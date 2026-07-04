package agent

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// hostSetupScriptName is the file written into the config dir for the user to
// run once with sudo. It gathers every root-only, one-time host requirement the
// agent has (uinput access, i2c for DDC brightness, GRUB reboot sudoers, ...)
// into a single idempotent script.
const hostSetupScriptName = "ottoman-host-setup.sh"

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
cat > /etc/udev/rules.d/99-ottoman-uinput.rules <<'RULE'
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
cat > /etc/sudoers.d/ottoman-grub <<RULE
$USER_NAME ALL=(root) NOPASSWD: /usr/sbin/grub-reboot *, /usr/bin/grub-reboot *, /usr/sbin/grub2-reboot *, /usr/bin/grub2-reboot *
RULE
chmod 440 /etc/sudoers.d/ottoman-grub
echo "[grub] NOTE: also set GRUB_DEFAULT=saved and keep GRUB_TIMEOUT=5 in"
echo "       /etc/default/grub, then: sudo update-grub && sudo grub-set-default '<linux entry>'"

`)

	b.WriteString(`echo
echo "== Done. Reloading udev rules... =="
udevadm control --reload-rules && udevadm trigger || true
echo "Log out and back in (or reboot) for the new group memberships to take effect."
`)

	return b.String()
}

// writeLinuxHostSetup writes the host-setup script into configDir and returns
// its path.
func writeLinuxHostSetup(configDir string) (string, error) {
	username := os.Getenv("USER")
	if username == "" {
		if u, err := user.Current(); err == nil {
			username = u.Username
		}
	}
	if username == "" {
		return "", errors.New("could not determine current username for host setup")
	}

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", errors.Wrap(err, "failed to create config directory")
	}

	path := filepath.Join(configDir, hostSetupScriptName)
	script := buildLinuxHostSetupScript(username)
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		return "", errors.Wrap(err, "failed to write host setup script")
	}
	return path, nil
}

// printLinuxHostSetupHint writes the host-setup script and logs how to run it.
func printLinuxHostSetupHint() {
	_, configDir := InstallPaths()
	path, err := writeLinuxHostSetup(configDir)
	if err != nil {
		fmt.Printf("Warning: could not write host setup script: %v\n", err)
		return
	}
	fmt.Println()
	fmt.Println("== One-time host setup required (needs root) ==")
	fmt.Println("Some features need kernel access that must be granted once with sudo:")
	fmt.Println("  - uinput      -> mouse/keyboard control on Wayland")
	fmt.Println("  - i2c-dev     -> monitor brightness/power over DDC/CI")
	fmt.Println("  - grub-reboot -> remote 'boot into Windows' (sudoers rule)")
	fmt.Printf("\n  Run:  sudo bash %q\n\n", path)
	fmt.Println("Then log out and back in (group changes) and restart the agent.")
}
