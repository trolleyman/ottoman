package agent

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// hostSetupScriptName is the file written into the config dir. It gathers every
// root-only, one-time host requirement the agent has (uinput access, i2c for
// DDC brightness, GRUB reboot sudoers, ...) into a single idempotent script.
// `agent install` offers to run it directly with sudo; it's also left on disk
// so it can be inspected or re-run by hand.
const hostSetupScriptName = "ottoman-host-setup.sh"

// paths touched by the setup script; also used to detect what's already done.
const (
	uinputUdevRule = "/etc/udev/rules.d/99-ottoman-uinput.rules"
	grubSudoers    = "/etc/sudoers.d/ottoman-grub"
	grubDefaults   = "/etc/default/grub"
)

// buildLinuxHostSetupScript returns an idempotent bash script that provisions
// the host-side prerequisites for the given user.
func buildLinuxHostSetupScript(username string) string {
	var b strings.Builder
	b.WriteString(`#!/usr/bin/env bash
# Ottoman one-time host setup. Run with sudo:  sudo bash ` + hostSetupScriptName + `
# Safe to re-run (idempotent).
set -euo pipefail

USER_NAME="` + username + `"

echo "== Ottoman host setup for $USER_NAME =="

`)

	// uinput: virtual mouse/keyboard on Wayland (plan §2b).
	b.WriteString(`# --- uinput: virtual input device (Wayland mouse/keyboard) ---
echo "[uinput] loading module + granting access"
modprobe uinput || true
echo uinput > /etc/modules-load.d/ottoman-uinput.conf
cat > ` + uinputUdevRule + ` <<'RULE'
KERNEL=="uinput", GROUP="input", MODE="0660", OPTIONS+="static_node=uinput"
RULE
usermod -aG input "$USER_NAME"

`)

	// i2c: DDC/CI monitor brightness + power (plan §3a).
	b.WriteString(`# --- i2c: DDC/CI monitor brightness & power ---
echo "[i2c] loading i2c-dev + granting access"
modprobe i2c-dev || true
echo i2c-dev > /etc/modules-load.d/ottoman-i2c.conf
# i2c group is created by the i2c-tools/ddcutil packages; create it if missing.
getent group i2c >/dev/null || groupadd i2c
usermod -aG i2c "$USER_NAME"

`)

	// grub-reboot sudoers for remote OS selection (plan §7).
	b.WriteString(`# --- GRUB one-shot reboot (remote OS selection) ---
echo "[grub] installing NOPASSWD sudoers rule for grub-reboot"
cat > ` + grubSudoers + ` <<RULE
$USER_NAME ALL=(root) NOPASSWD: /usr/sbin/grub-reboot *, /usr/bin/grub-reboot *, /usr/sbin/grub2-reboot *, /usr/bin/grub2-reboot *
RULE
chmod 440 ` + grubSudoers + `
# grub-reboot only takes effect when GRUB reads the saved next_entry, i.e. with
# GRUB_DEFAULT=saved. Warn if the box is pinned to a fixed default instead.
if [ -f ` + grubDefaults + ` ] && ! grep -Eq '^[[:space:]]*GRUB_DEFAULT=saved' ` + grubDefaults + `; then
  echo "[grub] WARNING: GRUB_DEFAULT is not 'saved' in ` + grubDefaults + `."
  echo "       'Boot into Windows' (grub-reboot) will NOT work until you set:"
  echo "         GRUB_DEFAULT=saved"
  echo "       then: sudo update-grub && sudo grub-set-default '<Linux entry>'"
fi

`)

	b.WriteString(`echo
echo "== Done. Reloading udev rules... =="
udevadm control --reload-rules && udevadm trigger || true
echo "Log out and back in (or reboot) for the new group memberships to take effect."
`)

	return b.String()
}

// writeLinuxHostSetup writes the host-setup script into configDir and returns
// its path along with the resolved username.
func writeLinuxHostSetup(configDir string) (path, username string, err error) {
	username = currentUsername()
	if username == "" {
		return "", "", errors.New("could not determine current username for host setup")
	}

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", "", errors.Wrap(err, "failed to create config directory")
	}

	path = filepath.Join(configDir, hostSetupScriptName)
	script := buildLinuxHostSetupScript(username)
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		return "", "", errors.Wrap(err, "failed to write host setup script")
	}
	return path, username, nil
}

func currentUsername() string {
	if name := os.Getenv("USER"); name != "" {
		return name
	}
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return ""
}

// hostCheck is one host-setup requirement and whether it is already satisfied.
type hostCheck struct {
	label string // short human name
	hint  string // what it enables / how it's granted
	done  bool
}

// checkLinuxHostSetup reports the current state of each host requirement so the
// installer can tell the user what (if anything) still needs doing.
func checkLinuxHostSetup(username string) []hostCheck {
	groups := userGroups(username)
	return []hostCheck{
		{
			label: "input group + uinput udev rule",
			hint:  "mouse/keyboard control on Wayland",
			done:  groups["input"] && fileExists(uinputUdevRule),
		},
		{
			label: "i2c group + i2c-dev module",
			hint:  "monitor brightness/power over DDC/CI",
			done:  groups["i2c"],
		},
		{
			label: "grub-reboot sudoers rule",
			hint:  "remote 'boot into Windows'",
			done:  fileExists(grubSudoers),
		},
	}
}

// setUpLinuxHost is called at the end of `agent install`. It writes the
// host-setup script, then — if anything is missing and we're on an interactive
// terminal — offers to run it with sudo right away. Non-interactive installs
// (e.g. scripted deploys) just get the script path and a hint, unchanged.
func setUpLinuxHost() {
	_, configDir := InstallPaths()
	path, username, err := writeLinuxHostSetup(configDir)
	if err != nil {
		fmt.Printf("Warning: could not write host setup script: %v\n", err)
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

	// Non-interactive install: leave the script and a hint, don't block.
	if !stdinIsTerminal() {
		fmt.Printf("\n  Run:  sudo bash %q\n", path)
		fmt.Println("Then log out and back in (group changes) and restart the agent.")
		return
	}

	fmt.Printf("\nThe script that grants these is at:\n  %s\n", path)
	if !promptYesNo(fmt.Sprintf("Run it now with sudo? (%d item(s) pending) [y/N] ", pending)) {
		fmt.Printf("\nSkipped. Run it yourself later:\n  sudo bash %q\n", path)
		return
	}

	fmt.Println()
	cmd := exec.Command("sudo", "bash", path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("\nHost setup did not complete: %v\n", err)
		fmt.Printf("You can re-run it manually:\n  sudo bash %q\n", path)
		return
	}
	fmt.Println("\nHost setup complete. Log out and back in for group changes to take effect.")
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

// stdinIsTerminal reports whether stdin is an interactive terminal (so we can
// prompt) rather than a pipe/file (scripted install).
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
