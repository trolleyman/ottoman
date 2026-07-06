package agent

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// Host-side, root-only prerequisites the Linux agent needs. Rather than emit a
// bash script for the user to run, the work is done natively in Go: the setup
// re-execs itself once via sudo and, running as root, writes the files and
// adjusts groups/modules directly — idempotently, with the sudoers file
// validated by visudo before it's installed.

const (
	uinputModuleConf = "/etc/modules-load.d/ottoman-uinput.conf"
	uinputUdevRule   = "/etc/udev/rules.d/99-ottoman-uinput.rules"
	i2cModuleConf    = "/etc/modules-load.d/ottoman-i2c.conf"
	grubSudoers      = "/etc/sudoers.d/ottoman-grub"
	grubDefaults     = "/etc/default/grub"

	uinputRuleContent = "KERNEL==\"uinput\", GROUP=\"input\", MODE=\"0660\", OPTIONS+=\"static_node=uinput\"\n"
)

const (
	// greeterRoot is a gdm-readable copy of the user's ottoman config + layouts.
	// The login-screen agent runs as gdm (which can't read the user's home), so
	// its HOME/XDG point here. Owned <user>:gdm, group-readable, setgid dirs so
	// the user's agent can keep it in sync and gdm can still read it.
	greeterRoot = "/var/lib/ottoman/greeter"
	// greeterBin is the greeter's copy of the ottoman binary. It lives inside
	// greeterRoot (owned <user>:gdm) rather than a root-owned path so a redeploy
	// can refresh it without sudo; it is deliberately not on anyone's $PATH.
	greeterBin = greeterRoot + "/bin/ottoman"
	// legacyGreeterBin is where pre-migration installs put the binary. Being
	// root-owned, it forced a sudo prompt on every redeploy just to refresh the
	// copy; a one-time `host-setup --greeter` re-run migrates away from it.
	legacyGreeterBin = "/usr/local/bin/ottoman"
	greeterDesktop   = "/usr/share/gdm/greeter/autostart/ottoman-greeter.desktop"
)

// greeterDesktopContent is the greeter autostart entry. Exec overrides
// HOME/XDG so the agent reads the gdm-readable copy under greeterRoot.
func greeterDesktopContent() string {
	return fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=Ottoman login-screen layout agent
Exec=env HOME=%[1]s XDG_CONFIG_HOME=%[1]s/.config XDG_DATA_HOME=%[1]s/.local/share %[2]s agent run --greeter
X-GNOME-Autostart-enabled=true
NoDisplay=true
`, greeterRoot, greeterBin)
}

// installGreeter deploys the login-screen layout agent: a system copy of the
// ottoman binary, a gdm-readable copy of the user's config + layouts under
// greeterRoot, and the greeter autostart entry that launches it. Returns
// whether it changed anything. Must run as root.
func installGreeter(username string) (bool, error) {
	u, err := user.Lookup(username)
	if err != nil {
		return false, errors.Wrapf(err, "no such user %q", username)
	}
	if _, err := user.LookupGroup("gdm"); err != nil {
		return false, errors.New("no 'gdm' group found — is GDM installed?")
	}

	// gdm-readable copy of config + layouts.
	cfgDst := filepath.Join(greeterRoot, ".config", "ottoman")
	dataDst := filepath.Join(greeterRoot, ".local", "share", "ottoman")
	if err := os.MkdirAll(cfgDst, 0750); err != nil {
		return false, errors.Wrapf(err, "failed to create %s", cfgDst)
	}
	if err := os.MkdirAll(dataDst, 0750); err != nil {
		return false, errors.Wrapf(err, "failed to create %s", dataDst)
	}
	// MkdirAll runs as root and creates any missing parents 0750 root-owned,
	// which would block the user from even traversing into greeterRoot — the
	// user-level binary refresh, staleness check, and layout mirroring all
	// need that. The parent holds nothing sensitive (protection lives on
	// greeterRoot itself, 2750 <user>:gdm), so open it up.
	if err := os.Chmod(filepath.Dir(greeterRoot), 0755); err != nil {
		return false, errors.Wrapf(err, "failed to chmod %s", filepath.Dir(greeterRoot))
	}
	copyTreeInto(filepath.Join(u.HomeDir, ".config", "ottoman"), cfgDst)
	copyTreeInto(filepath.Join(u.HomeDir, ".local", "share", "ottoman"), dataDst)

	// The greeter dir is read-only to gdm, so the agent must never need to write
	// during load. Ensure layouts.json exists so its migrate-on-missing path (a
	// write) can't trigger; an empty store is fine — the user's agent mirrors the
	// real layouts in on the next switch.
	if lj := filepath.Join(dataDst, "layouts.json"); !fileExists(lj) {
		if err := os.WriteFile(lj, []byte("[]\n"), 0640); err != nil {
			return false, errors.Wrapf(err, "failed to seed %s", lj)
		}
	}

	// Owner <user>:gdm; setgid dirs so files the user's agent later mirrors in
	// inherit the gdm group and stay group-readable; nothing readable by others.
	if err := run("chown", "-R", username+":gdm", greeterRoot); err != nil {
		return false, errors.Wrap(err, "failed to chown greeter dir")
	}
	if err := run("find", greeterRoot, "-type", "d", "-exec", "chmod", "2750", "{}", "+"); err != nil {
		return false, errors.Wrap(err, "failed to chmod greeter dirs")
	}
	if err := run("find", greeterRoot, "-type", "f", "-exec", "chmod", "0640", "{}", "+"); err != nil {
		return false, errors.Wrap(err, "failed to chmod greeter files")
	}
	fmt.Printf("  populated %s (owner %s, group gdm)\n", greeterRoot, username)

	// The binary copy (gdm can't exec the user's ~/.local/bin) is a user-owned
	// file inside greeterRoot — installed after the blanket chmods above so it
	// keeps its exec bit — letting a redeploy refresh it without root. Drop the
	// legacy root-owned copy, which forced a sudo prompt on every deploy.
	self, err := os.Executable()
	if err != nil {
		return false, errors.Wrap(err, "failed to find own executable")
	}
	if err := run("install", "-d", "-m", "2750", "-o", username, "-g", "gdm", filepath.Dir(greeterBin)); err != nil {
		return false, errors.Wrap(err, "failed to create greeter bin dir")
	}
	if err := run("install", "-m", "0750", "-o", username, "-g", "gdm", self, greeterBin); err != nil {
		return false, errors.Wrap(err, "failed to install greeter binary")
	}
	fmt.Printf("  installed %s\n", greeterBin)
	if err := os.Remove(legacyGreeterBin); err == nil {
		fmt.Printf("  removed legacy %s\n", legacyGreeterBin)
	}

	// Greeter autostart entry.
	changed, err := writeFileIfChanged(greeterDesktop, []byte(greeterDesktopContent()), 0644)
	if err != nil {
		return false, err
	}
	return changed, nil
}

// copyTreeInto copies the contents of src into dst (best-effort; a missing src
// is fine — the user may simply have no config/layouts yet).
func copyTreeInto(src, dst string) {
	if !fileExists(src) {
		return
	}
	if err := run("cp", "-a", src+"/.", dst); err != nil {
		fmt.Printf("  note: could not copy %s: %v\n", src, err)
	}
}

// grubSudoersContent is the NOPASSWD rule allowing the agent to set a one-shot
// GRUB next-boot entry (covers both grub-reboot and grub2-reboot paths).
func grubSudoersContent(username string) string {
	return username + " ALL=(root) NOPASSWD: /usr/sbin/grub-reboot *, /usr/bin/grub-reboot *, /usr/sbin/grub2-reboot *, /usr/bin/grub2-reboot *\n"
}

// HostSetup provisions the root-only host prerequisites for username. If not
// already running as root it re-execs itself once via sudo (so sudo can prompt
// on the terminal); as root it applies each step directly in Go. Passing an
// empty username resolves it from SUDO_USER / USER. When greeter is true it also
// installs the GDM login-screen layout agent.
func HostSetup(username string, greeter bool) error {
	if username == "" {
		username = setupTargetUser()
	}
	if username == "" || username == "root" {
		return errors.New("could not determine the non-root user to set up (pass --user)")
	}

	if os.Geteuid() != 0 {
		return elevateHostSetup(username, greeter)
	}
	return applyHostSetup(username, greeter)
}

// elevateHostSetup re-runs `ottoman agent host-setup --user <name>` under sudo.
func elevateHostSetup(username string, greeter bool) error {
	exe, err := os.Executable()
	if err != nil {
		return errors.Wrap(err, "failed to find own executable")
	}
	fmt.Println("Requesting root via sudo to apply host setup...")
	args := []string{exe, "agent", "host-setup", "--user", username}
	if greeter {
		args = append(args, "--greeter")
	}
	cmd := exec.Command("sudo", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "host setup (via sudo) failed")
	}
	return nil
}

// applyHostSetup performs the privileged work. It must run as root.
func applyHostSetup(username string, greeter bool) error {
	if os.Geteuid() != 0 {
		return errors.New("applyHostSetup must run as root")
	}
	if _, err := user.Lookup(username); err != nil {
		return errors.Wrapf(err, "no such user %q", username)
	}

	fmt.Printf("== Ottoman host setup for %s ==\n", username)
	var changed, relogin bool

	// --- uinput: virtual mouse/keyboard on Wayland ---
	fmt.Println("[uinput] virtual input device (Wayland mouse/keyboard)")
	tryModprobe("uinput")
	if c, err := writeFileIfChanged(uinputModuleConf, []byte("uinput\n"), 0644); err != nil {
		return err
	} else if c {
		changed = true
	}
	udevChanged, err := writeFileIfChanged(uinputUdevRule, []byte(uinputRuleContent), 0644)
	if err != nil {
		return err
	}
	changed = changed || udevChanged
	if added, err := ensureUserInGroup(username, "input"); err != nil {
		return err
	} else if added {
		changed, relogin = true, true
	}

	// --- i2c: DDC/CI monitor brightness & power ---
	fmt.Println("[i2c] i2c-dev for DDC/CI monitor brightness & power")
	tryModprobe("i2c-dev")
	if c, err := writeFileIfChanged(i2cModuleConf, []byte("i2c-dev\n"), 0644); err != nil {
		return err
	} else if c {
		changed = true
	}
	if err := ensureGroup("i2c"); err != nil {
		return err
	}
	if added, err := ensureUserInGroup(username, "i2c"); err != nil {
		return err
	} else if added {
		changed, relogin = true, true
	}

	// --- GRUB one-shot reboot (remote OS selection) ---
	fmt.Println("[grub] NOPASSWD sudoers rule for grub-reboot")
	if c, err := installSudoers(grubSudoers, grubSudoersContent(username)); err != nil {
		return err
	} else if c {
		changed = true
	}
	warnIfGrubDefaultNotSaved()

	// --- GDM login-screen layout agent (opt-in) ---
	if greeter {
		fmt.Println("[greeter] GDM login-screen layout agent (ottoman agent run --greeter)")
		if c, err := installGreeter(username); err != nil {
			return err
		} else if c {
			changed = true
		}
	}

	// Reload udev only if a rule changed.
	if udevChanged {
		fmt.Println("[udev] reloading rules")
		run("udevadm", "control", "--reload-rules")
		run("udevadm", "trigger")
	}

	fmt.Println()
	if !changed {
		fmt.Println("Everything was already in place — no changes made.")
	} else {
		fmt.Println("Host setup complete.")
		if relogin {
			fmt.Println("Log out and back in (or reboot) for new group memberships to take effect.")
		}
		if greeter {
			fmt.Println("Login-screen layout agent installed. Log out to test it on the GDM")
			fmt.Println("greeter. Future deploys refresh its binary copy automatically, no sudo.")
		}
	}
	return nil
}

// setupTargetUser resolves the real (non-root) user to configure. Under sudo
// that's SUDO_USER, otherwise the current user.
func setupTargetUser() string {
	if u := os.Getenv("SUDO_USER"); u != "" && u != "root" {
		return u
	}
	return currentUsername()
}

func currentUsername() string {
	if name := os.Getenv("USER"); name != "" && name != "root" {
		return name
	}
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return ""
}

// writeFileIfChanged writes content to path only if it differs from what's
// already there, reporting whether it wrote. Written atomically (temp+rename).
func writeFileIfChanged(path string, content []byte, perm os.FileMode) (bool, error) {
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, content) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return false, errors.Wrapf(err, "failed to create dir for %s", path)
	}
	if err := atomicWrite(path, content, perm); err != nil {
		return false, err
	}
	fmt.Printf("  wrote %s\n", path)
	return true, nil
}

// atomicWrite writes content to a temp file in the same dir then renames it
// over path, so readers never see a partial file.
func atomicWrite(path string, content []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp")
	if err != nil {
		return errors.Wrapf(err, "failed to create temp file for %s", path)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return errors.Wrapf(err, "failed to write %s", path)
	}
	if err := tmp.Close(); err != nil {
		return errors.Wrapf(err, "failed to close %s", path)
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return errors.Wrapf(err, "failed to chmod %s", path)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return errors.Wrapf(err, "failed to install %s", path)
	}
	return nil
}

// installSudoers writes a sudoers drop-in, validating it with `visudo -cf`
// before moving it into place so a bad rule can never break sudo.
func installSudoers(path, content string) (bool, error) {
	if existing, err := os.ReadFile(path); err == nil && string(existing) == content {
		return false, nil
	}
	tmp, err := os.CreateTemp("", "ottoman-sudoers-*")
	if err != nil {
		return false, errors.Wrap(err, "failed to create temp sudoers file")
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return false, errors.Wrap(err, "failed to write temp sudoers file")
	}
	tmp.Close()
	if err := os.Chmod(tmpName, 0440); err != nil {
		return false, errors.Wrap(err, "failed to chmod temp sudoers file")
	}

	if out, err := exec.Command("visudo", "-cf", tmpName).CombinedOutput(); err != nil {
		return false, errors.Wrapf(err, "sudoers validation failed: %s", strings.TrimSpace(string(out)))
	}
	if err := os.Rename(tmpName, path); err != nil {
		return false, errors.Wrapf(err, "failed to install %s", path)
	}
	fmt.Printf("  wrote %s (validated)\n", path)
	return true, nil
}

// ensureGroup creates a group if it doesn't already exist.
func ensureGroup(group string) error {
	if _, err := user.LookupGroup(group); err == nil {
		return nil
	}
	if err := run("groupadd", group); err != nil {
		return errors.Wrapf(err, "failed to create group %q", group)
	}
	fmt.Printf("  created group %s\n", group)
	return nil
}

// ensureUserInGroup adds username to group if not already a member, reporting
// whether it made a change.
func ensureUserInGroup(username, group string) (bool, error) {
	if userGroups(username)[group] {
		return false, nil
	}
	if err := run("usermod", "-aG", group, username); err != nil {
		return false, errors.Wrapf(err, "failed to add %s to group %q", username, group)
	}
	fmt.Printf("  added %s to group %s\n", username, group)
	return true, nil
}

// tryModprobe loads a kernel module, ignoring failure (it may be built in or
// load only after reboot; the modules-load.d entry covers the persistent case).
func tryModprobe(module string) {
	if err := run("modprobe", module); err != nil {
		fmt.Printf("  note: modprobe %s failed (will load on next boot): %v\n", module, err)
	}
}

// warnIfGrubDefaultNotSaved warns when GRUB isn't set to remember a saved entry,
// since grub-reboot (remote "boot into Windows") is a silent no-op otherwise.
func warnIfGrubDefaultNotSaved() {
	data, err := os.ReadFile(grubDefaults)
	if err != nil {
		return // no /etc/default/grub (non-GRUB system); nothing to warn about
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "GRUB_DEFAULT=") {
			continue
		}
		val := strings.Trim(strings.TrimPrefix(line, "GRUB_DEFAULT="), `"'`)
		if val != "saved" {
			fmt.Printf("  WARNING: GRUB_DEFAULT=%q (not \"saved\") in %s\n", val, grubDefaults)
			fmt.Println("           'Boot into Windows' (grub-reboot) will NOT work until you set")
			fmt.Println("           GRUB_DEFAULT=saved, then: sudo update-grub && sudo grub-set-default '<Linux entry>'")
		}
		return
	}
}

// run executes a command, surfacing combined output on failure.
func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		if trimmed := strings.TrimSpace(string(out)); trimmed != "" {
			return errors.Errorf("%v: %s", err, trimmed)
		}
		return err
	}
	return nil
}

// --- interactive install integration ---

// hostCheck is one host-setup requirement and whether it is already satisfied.
type hostCheck struct {
	label string // short human name
	hint  string // what it enables
	done  bool
}

// checkLinuxHostSetup reports the current state of each host requirement so the
// installer can tell the user what (if anything) still needs doing.
func checkLinuxHostSetup(username string) []hostCheck {
	groups := userGroups(username)
	return []hostCheck{
		{"input group + uinput udev rule", "mouse/keyboard control on Wayland", groups["input"] && fileExists(uinputUdevRule)},
		{"i2c group + i2c-dev module", "monitor brightness/power over DDC/CI", groups["i2c"] && fileExists(i2cModuleConf)},
		{"grub-reboot sudoers rule", "remote 'boot into Windows'", fileExists(grubSudoers)},
	}
}

// setUpLinuxHost is called at the end of `agent install`. It reports what host
// setup is still needed and, on an interactive terminal, offers to apply it
// (self-elevating via sudo). Non-interactive installs just print the command.
func setUpLinuxHost() {
	username := currentUsername()
	if username == "" {
		return
	}
	checks := checkLinuxHostSetup(username)
	pending := 0
	for _, c := range checks {
		if !c.done {
			pending++
		}
	}

	fmt.Println()
	fmt.Println("== One-time host setup (needs root) ==")
	fmt.Println("Some features need kernel access that must be granted once with sudo:")
	for _, c := range checks {
		mark := "needs setup"
		if c.done {
			mark = "ok"
		}
		fmt.Printf("  [%-11s] %s  (%s)\n", mark, c.label, c.hint)
	}

	greeterInstalled := fileExists(greeterDesktop)
	// Pre-migration installs keep the binary at legacyGreeterBin (root-owned);
	// one more sudo run moves it under greeterRoot, after which redeploys can
	// refresh the copy without root.
	greeterNeedsMigration := greeterInstalled && !greeterDesktopCurrent()
	greeterStale := greeterInstalled && !greeterNeedsMigration && greeterBinaryOutdated()

	// A stale binary in the current layout is user-writable — refresh it in
	// place, no root needed.
	if greeterStale {
		if err := refreshGreeterBinary(); err != nil {
			fmt.Printf("  note: could not refresh the login-screen agent binary: %v\n", err)
			fmt.Println("        retry with:  sudo ottoman agent host-setup --greeter")
		} else {
			fmt.Println("  refreshed the login-screen layout agent binary (no root needed)")
			greeterStale = false
		}
	}

	greeterMark := "needs setup"
	switch {
	case greeterNeedsMigration:
		greeterMark = "migrate"
	case greeterStale:
		greeterMark = "stale"
	case greeterInstalled:
		greeterMark = "ok"
	}
	fmt.Printf("  [%-11s] %s  (%s)\n", greeterMark, "GDM login-screen layout agent", "switch layouts on the login screen (opt-in)")

	if pending == 0 && greeterInstalled && !greeterStale && !greeterNeedsMigration {
		fmt.Println("\nEverything is already configured. Nothing to do.")
		return
	}

	if !stdinIsTerminal() {
		if pending > 0 {
			fmt.Println("\n  Run:  sudo ottoman agent host-setup")
			fmt.Println("Then log out and back in (group changes) and restart the agent.")
		}
		if !greeterInstalled {
			fmt.Println("  Login-screen layouts:  sudo ottoman agent host-setup --greeter")
		}
		if greeterNeedsMigration {
			fmt.Println("  Migrate the login-screen agent (one-time):  sudo ottoman agent host-setup --greeter")
		}
		return
	}

	// Base setup (uinput/i2c/grub) — one prompt.
	applyBase := false
	if pending > 0 {
		applyBase = promptYesNo(fmt.Sprintf("\nApply host setup now with sudo? (%d item(s) pending) [y/N] ", pending))
	}

	// Login-screen layout agent. Offer it as an opt-in when it isn't installed.
	// Binary staleness is handled above without root; sudo is only needed here
	// for a fresh install, the one-time migration, or a failed refresh.
	greeterWanted := false
	if !greeterInstalled {
		fmt.Println("\nOptional: a GDM login-screen agent lets you switch display layouts on")
		fmt.Println("the login screen and mirrors your session's layout there (runs as gdm).")
		greeterWanted = promptYesNo("Install the login-screen layout agent? [y/N] ")
	} else if greeterNeedsMigration {
		fmt.Println("\nThe login-screen layout agent needs a one-time migration: its binary")
		fmt.Println("moves into " + greeterRoot + " so future deploys can refresh")
		fmt.Println("it without sudo.")
		greeterWanted = promptYesNo("Migrate now with sudo? [y/N] ")
	} else if greeterStale {
		greeterWanted = promptYesNo("Refresh the login-screen agent binary with sudo? [y/N] ")
	}

	if !applyBase && !greeterWanted {
		fmt.Println("\nSkipped. Apply it later with:  sudo ottoman agent host-setup [--greeter]")
		return
	}
	fmt.Println()
	if err := HostSetup(username, greeterWanted); err != nil {
		fmt.Printf("\nHost setup did not complete: %v\n", err)
		fmt.Println("You can retry with:  sudo ottoman agent host-setup")
	}
}

// greeterDesktopCurrent reports whether the installed greeter autostart entry
// matches what we would write today; a mismatch means the install predates a
// layout change (e.g. the binary moving under greeterRoot) and needs a root
// re-run to migrate.
func greeterDesktopCurrent() bool {
	data, err := os.ReadFile(greeterDesktop)
	return err == nil && string(data) == greeterDesktopContent()
}

// refreshGreeterBinary copies the running (freshly deployed) binary over the
// greeter's copy. The copy is a user-owned file inside greeterRoot precisely
// so this needs no root; the setgid bin dir keeps the new file group-gdm.
func refreshGreeterBinary() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(self)
	if err != nil {
		return err
	}
	return atomicWrite(greeterBin, data, 0750)
}

// greeterBinaryOutdated reports whether the installed greeter binary copy differs
// from the currently running binary (the freshly deployed one), so a redeploy can
// refresh it. A missing copy counts as outdated.
func greeterBinaryOutdated() bool {
	self, err := os.Executable()
	if err != nil {
		return false
	}
	return !filesEqual(self, greeterBin)
}

// filesEqual reports whether two files have identical contents.
func filesEqual(a, b string) bool {
	da, err := os.ReadFile(a)
	if err != nil {
		return false
	}
	db, err := os.ReadFile(b)
	if err != nil {
		return false
	}
	return bytes.Equal(da, db)
}

// userGroups returns the set of groups the given user belongs to.
func userGroups(username string) map[string]bool {
	out := map[string]bool{}
	u, err := user.Lookup(username)
	if err != nil {
		return out
	}
	gids, err := u.GroupIds()
	if err != nil {
		return out
	}
	for _, gid := range gids {
		if g, err := user.LookupGroupId(gid); err == nil {
			out[g.Name] = true
		}
	}
	return out
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// stdinIsTerminal reports whether stdin is an interactive terminal.
func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// promptYesNo asks a yes/no question on stdin, defaulting to no.
func promptYesNo(question string) bool {
	fmt.Print(question)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}
