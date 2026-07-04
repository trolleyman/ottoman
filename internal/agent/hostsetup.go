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

// grubSudoersContent is the NOPASSWD rule allowing the agent to set a one-shot
// GRUB next-boot entry (covers both grub-reboot and grub2-reboot paths).
func grubSudoersContent(username string) string {
	return username + " ALL=(root) NOPASSWD: /usr/sbin/grub-reboot *, /usr/bin/grub-reboot *, /usr/sbin/grub2-reboot *, /usr/bin/grub2-reboot *\n"
}

// HostSetup provisions the root-only host prerequisites for username. If not
// already running as root it re-execs itself once via sudo (so sudo can prompt
// on the terminal); as root it applies each step directly in Go. Passing an
// empty username resolves it from SUDO_USER / USER.
func HostSetup(username string) error {
	if username == "" {
		username = setupTargetUser()
	}
	if username == "" || username == "root" {
		return errors.New("could not determine the non-root user to set up (pass --user)")
	}

	if os.Geteuid() != 0 {
		return elevateHostSetup(username)
	}
	return applyHostSetup(username)
}

// elevateHostSetup re-runs `ottoman agent host-setup --user <name>` under sudo.
func elevateHostSetup(username string) error {
	exe, err := os.Executable()
	if err != nil {
		return errors.Wrap(err, "failed to find own executable")
	}
	fmt.Println("Requesting root via sudo to apply host setup...")
	cmd := exec.Command("sudo", exe, "agent", "host-setup", "--user", username)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "host setup (via sudo) failed")
	}
	return nil
}

// applyHostSetup performs the privileged work. It must run as root.
func applyHostSetup(username string) error {
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

	if pending == 0 {
		fmt.Println("\nEverything is already configured. Nothing to do.")
		return
	}

	if !stdinIsTerminal() {
		fmt.Println("\n  Run:  sudo ottoman agent host-setup")
		fmt.Println("Then log out and back in (group changes) and restart the agent.")
		return
	}

	if !promptYesNo(fmt.Sprintf("\nApply host setup now with sudo? (%d item(s) pending) [y/N] ", pending)) {
		fmt.Println("\nSkipped. Apply it later with:  sudo ottoman agent host-setup")
		return
	}
	fmt.Println()
	if err := HostSetup(username); err != nil {
		fmt.Printf("\nHost setup did not complete: %v\n", err)
		fmt.Println("You can retry with:  sudo ottoman agent host-setup")
	}
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
