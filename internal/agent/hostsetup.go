package agent

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
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

	// GDM config lives at different paths depending on distro (Debian/Ubuntu
	// package the daemon as "gdm3"; Fedora/Arch/openSUSE as "gdm").
	gdmConfDebian = "/etc/gdm3/custom.conf"
	gdmConfOther  = "/etc/gdm/custom.conf"

	// autostartLockRel is the per-user autostart entry (relative to $HOME) that
	// locks the screen right after autologin.
	autostartLockRel = ".config/autostart/ottoman-lock.desktop"
	lockScriptRel    = ".config/ottoman/lock-on-login.sh"

	uinputRuleContent = "KERNEL==\"uinput\", GROUP=\"input\", MODE=\"0660\", OPTIONS+=\"static_node=uinput\"\n"
)

// lockOnLoginScript locks the GNOME screen after autologin so the desktop stays
// password-protected while the agent runs behind the lock.
//
// GNOME's ScreenSaver interface may not be on the bus the instant autostart
// fires, so we wait for it rather than poll on a timer: `gdbus wait` blocks
// until the name is owned (i.e. the shell is ready) and then we lock once, so
// the lock fires the moment it can rather than after an arbitrary delay. The
// --timeout is only a hang backstop. On glib too old for `gdbus wait` (<2.72)
// we fall back to a bounded retry — still preferring "keep trying" over
// leaving the screen unlocked.
const lockOnLoginScript = `#!/bin/sh
# Installed by ottoman. Locks the GNOME screen right after autologin so the
# desktop stays password-protected while the ottoman agent runs behind it.
lock() {
	gdbus call --session \
		--dest org.gnome.ScreenSaver \
		--object-path /org/gnome/ScreenSaver \
		--method org.gnome.ScreenSaver.Lock >/dev/null 2>&1
}

if gdbus wait --session --timeout 120 org.gnome.ScreenSaver 2>/dev/null; then
	lock
else
	# Old glib without "gdbus wait", or the name never appeared: keep trying.
	i=0
	while [ "$i" -lt 120 ]; do
		lock && break
		i=$((i + 1))
		sleep 1
	done
fi
`

// lockOnLoginDesktop is the autostart entry that runs lockOnLoginScript. The
// %s is the absolute path to the installed script.
const lockOnLoginDesktop = `[Desktop Entry]
Type=Application
Name=Ottoman Lock On Login
Comment=Lock the screen after autologin so the desktop stays password-protected
Exec=%s
X-GNOME-Autostart-enabled=true
NoDisplay=true
`

const (
	// greeterRoot is a gdm-readable copy of the user's ottoman config + layouts.
	// The login-screen agent runs as gdm (which can't read the user's home), so
	// its HOME/XDG point here. Owned <user>:gdm, group-readable, setgid dirs so
	// the user's agent can keep it in sync and gdm can still read it.
	greeterRoot    = "/var/lib/ottoman/greeter"
	greeterBin     = "/usr/local/bin/ottoman"
	greeterDesktop = "/usr/share/gdm/greeter/autostart/ottoman-greeter.desktop"
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

	// System copy of the binary (gdm can't exec the user's ~/.local/bin).
	self, err := os.Executable()
	if err != nil {
		return false, errors.Wrap(err, "failed to find own executable")
	}
	if err := run("install", "-Dm0755", self, greeterBin); err != nil {
		return false, errors.Wrap(err, "failed to install greeter binary")
	}
	fmt.Printf("  installed %s\n", greeterBin)

	// gdm-readable copy of config + layouts.
	cfgDst := filepath.Join(greeterRoot, ".config", "ottoman")
	dataDst := filepath.Join(greeterRoot, ".local", "share", "ottoman")
	if err := os.MkdirAll(cfgDst, 0750); err != nil {
		return false, errors.Wrapf(err, "failed to create %s", cfgDst)
	}
	if err := os.MkdirAll(dataDst, 0750); err != nil {
		return false, errors.Wrapf(err, "failed to create %s", dataDst)
	}
	copyTreeInto(filepath.Join(u.HomeDir, ".config", "ottoman"), cfgDst)
	copyTreeInto(filepath.Join(u.HomeDir, ".local", "share", "ottoman"), dataDst)

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
	var changed, relogin, autologin bool

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

	// --- GDM autologin into a locked screen (agent works after Wake-on-LAN) ---
	// Autologin starts a graphical session at boot so the agent's display/audio
	// backends come up without a manual login; the lock-on-login autostart then
	// locks it immediately, so the desktop still needs the password to be used.
	fmt.Println("[autologin] GDM automatic login into a locked screen")
	if c, err := enableGdmAutologin(username); err != nil {
		return err
	} else if c {
		changed, autologin = true, true
	}
	if c, err := installAutostartLock(username); err != nil {
		return err
	} else if c {
		changed = true
	}

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
		if autologin {
			fmt.Println("Autologin enabled: the machine now boots straight into your session (locked).")
			fmt.Println("Reboot to test. To undo, restore the backed-up GDM config and delete")
			fmt.Printf("  ~/%s\n", autostartLockRel)
		}
		if greeter {
			fmt.Println("Login-screen layout agent installed. Log out to test it on the GDM")
			fmt.Println("greeter. Re-run with --greeter after updating ottoman to refresh the")
			fmt.Println("system binary copy.")
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

// gdmConfigPath returns the GDM custom.conf path present on this system, or ""
// if neither known location exists (e.g. a non-GDM display manager).
func gdmConfigPath() string {
	for _, p := range []string{gdmConfDebian, gdmConfOther} {
		if fileExists(p) {
			return p
		}
	}
	return ""
}

// enableGdmAutologin turns on GDM automatic login for username by editing the
// [daemon] section of custom.conf in place, preserving every other setting. It
// backs the file up once before the first change. Returns whether it changed
// anything; if no GDM config is found it warns and makes no change.
func enableGdmAutologin(username string) (bool, error) {
	path := gdmConfigPath()
	if path == "" {
		fmt.Printf("  note: no GDM config found (%s or %s)\n", gdmConfDebian, gdmConfOther)
		fmt.Println("        skipping autologin — is this system using GDM?")
		return false, nil
	}
	orig, err := os.ReadFile(path)
	if err != nil {
		return false, errors.Wrapf(err, "failed to read %s", path)
	}
	updated := setGdmAutologin(string(orig), username)
	if updated == string(orig) {
		return false, nil
	}
	backup := path + ".ottoman-bak"
	if !fileExists(backup) {
		if err := atomicWrite(backup, orig, 0644); err != nil {
			return false, errors.Wrap(err, "failed to back up GDM config")
		}
		fmt.Printf("  backed up %s -> %s\n", path, backup)
	}
	if err := atomicWrite(path, []byte(updated), 0644); err != nil {
		return false, err
	}
	fmt.Printf("  enabled autologin for %s in %s\n", username, path)
	return true, nil
}

// gdmAutologinEnabled reports whether GDM autologin is already configured for
// username (i.e. applying setGdmAutologin would be a no-op).
func gdmAutologinEnabled(username string) bool {
	path := gdmConfigPath()
	if path == "" {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return string(data) == setGdmAutologin(string(data), username)
}

// setGdmAutologin returns content with AutomaticLoginEnable=true and
// AutomaticLogin=<username> set inside the [daemon] section, preserving all
// other lines. Existing copies of those keys (including commented-out ones) are
// replaced in place; missing keys are inserted into [daemon], and the section
// is created if it is absent.
func setGdmAutologin(content, username string) string {
	want := map[string]string{
		"AutomaticLoginEnable": "true",
		"AutomaticLogin":       username,
	}
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines)+4)
	section := ""
	seen := map[string]bool{}
	daemonSeen := false

	flushMissing := func() {
		if !seen["AutomaticLoginEnable"] {
			out = append(out, "AutomaticLoginEnable="+want["AutomaticLoginEnable"])
			seen["AutomaticLoginEnable"] = true
		}
		if !seen["AutomaticLogin"] {
			out = append(out, "AutomaticLogin="+want["AutomaticLogin"])
			seen["AutomaticLogin"] = true
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			if section == "daemon" {
				flushMissing() // finish [daemon] before starting the next section
			}
			section = strings.ToLower(strings.TrimSpace(trimmed[1 : len(trimmed)-1]))
			if section == "daemon" {
				daemonSeen = true
			}
			out = append(out, line)
			continue
		}
		if section == "daemon" {
			if key := matchedDaemonKey(trimmed); key != "" {
				if !seen[key] {
					out = append(out, key+"="+want[key])
					seen[key] = true
				}
				continue // drop the original (and any duplicate/commented copies)
			}
		}
		out = append(out, line)
	}
	if section == "daemon" {
		flushMissing()
	}
	if !daemonSeen {
		out = append(out, "[daemon]", "AutomaticLoginEnable="+want["AutomaticLoginEnable"], "AutomaticLogin="+want["AutomaticLogin"])
	}
	return strings.Join(out, "\n")
}

// matchedDaemonKey returns the canonical autologin key a line sets (ignoring a
// leading comment marker), or "" if it sets neither. "AutomaticLoginEnable" is
// checked first since "AutomaticLogin" is a prefix of it.
func matchedDaemonKey(line string) string {
	s := strings.TrimLeft(strings.TrimSpace(line), "#; ")
	for _, k := range []string{"AutomaticLoginEnable", "AutomaticLogin"} {
		if rest := strings.TrimSpace(strings.TrimPrefix(s, k)); strings.HasPrefix(s, k) && strings.HasPrefix(rest, "=") {
			return k
		}
	}
	return ""
}

// installAutostartLock installs the per-user lock-on-login script and autostart
// entry into username's home, owned by that user. Returns whether it changed
// anything. Runs as root, so every file/dir it creates is chowned back.
func installAutostartLock(username string) (bool, error) {
	u, err := user.Lookup(username)
	if err != nil {
		return false, errors.Wrapf(err, "no such user %q", username)
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return false, errors.Wrapf(err, "bad uid for %s", username)
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return false, errors.Wrapf(err, "bad gid for %s", username)
	}
	if u.HomeDir == "" {
		return false, errors.Errorf("no home directory for %s", username)
	}

	scriptPath := filepath.Join(u.HomeDir, lockScriptRel)
	desktopPath := filepath.Join(u.HomeDir, autostartLockRel)
	changed := false

	if err := ensureUserDir(filepath.Dir(scriptPath), uid, gid); err != nil {
		return false, err
	}
	if c, err := writeFileIfChanged(scriptPath, []byte(lockOnLoginScript), 0755); err != nil {
		return false, err
	} else if c {
		changed = true
	}
	if err := os.Chown(scriptPath, uid, gid); err != nil {
		return false, errors.Wrapf(err, "failed to chown %s", scriptPath)
	}

	if err := ensureUserDir(filepath.Dir(desktopPath), uid, gid); err != nil {
		return false, err
	}
	if c, err := writeFileIfChanged(desktopPath, []byte(fmt.Sprintf(lockOnLoginDesktop, scriptPath)), 0644); err != nil {
		return false, err
	} else if c {
		changed = true
	}
	if err := os.Chown(desktopPath, uid, gid); err != nil {
		return false, errors.Wrapf(err, "failed to chown %s", desktopPath)
	}
	return changed, nil
}

// ensureUserDir creates dir (if needed) and chowns it to uid:gid so files the
// root process writes under it end up owned by the target user.
func ensureUserDir(dir string, uid, gid int) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.Wrapf(err, "failed to create %s", dir)
	}
	if err := os.Chown(dir, uid, gid); err != nil {
		return errors.Wrapf(err, "failed to chown %s", dir)
	}
	return nil
}

// userHome returns username's home directory, or "" if it can't be resolved.
func userHome(username string) string {
	if u, err := user.Lookup(username); err == nil {
		return u.HomeDir
	}
	return ""
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
	autologinDone := gdmAutologinEnabled(username) && fileExists(filepath.Join(userHome(username), autostartLockRel))
	return []hostCheck{
		{"input group + uinput udev rule", "mouse/keyboard control on Wayland", groups["input"] && fileExists(uinputUdevRule)},
		{"i2c group + i2c-dev module", "monitor brightness/power over DDC/CI", groups["i2c"] && fileExists(i2cModuleConf)},
		{"grub-reboot sudoers rule", "remote 'boot into Windows'", fileExists(grubSudoers)},
		{"GDM autologin + lock-on-login", "agent works after Wake-on-LAN; desktop stays locked", autologinDone},
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
	fmt.Printf("  [%-11s] %s  (%s)\n", markDone(greeterInstalled), "GDM login-screen layout agent", "switch layouts on the login screen (opt-in)")

	if pending == 0 && greeterInstalled {
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
		return
	}

	// Base setup (uinput/i2c/grub/autologin) — one prompt.
	applyBase := false
	if pending > 0 {
		applyBase = promptYesNo(fmt.Sprintf("\nApply host setup now with sudo? (%d item(s) pending) [y/N] ", pending))
	}

	// Login-screen layout agent — separate opt-in prompt.
	installGreeter := false
	if !greeterInstalled {
		fmt.Println("\nOptional: a GDM login-screen agent lets you switch display layouts on")
		fmt.Println("the login screen and mirrors your session's layout there (runs as gdm).")
		installGreeter = promptYesNo("Install the login-screen layout agent? [y/N] ")
	}

	if !applyBase && !installGreeter {
		fmt.Println("\nSkipped. Apply it later with:  sudo ottoman agent host-setup [--greeter]")
		return
	}
	fmt.Println()
	if err := HostSetup(username, installGreeter); err != nil {
		fmt.Printf("\nHost setup did not complete: %v\n", err)
		fmt.Println("You can retry with:  sudo ottoman agent host-setup")
	}
}

// markDone returns the checklist status label for a boolean state.
func markDone(done bool) string {
	if done {
		return "ok"
	}
	return "needs setup"
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
