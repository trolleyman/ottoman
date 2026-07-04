package agent

import (
	"context"
	"log"
	"os/exec"
	"runtime"
	"time"

	"github.com/trolleyman/ottoman/internal/api"
)

// Boot implements api.StrictServerInterface. It reboots the machine, optionally
// into a specific OS via GRUB. "windows" uses grub-reboot to set a one-shot
// next-boot entry (requires GRUB_DEFAULT=saved), so the machine boots Windows
// once and the default (Linux) is unchanged. "linux" (or empty) is a plain
// reboot, which lands in the GRUB default.
func (a *Agent) Boot(ctx context.Context, request api.BootRequestObject) (api.BootResponseObject, error) {
	if request.Body == nil {
		return api.Boot400JSONResponse{Code: 400, Error: "target is required"}, nil
	}
	target := request.Body.Target

	if runtime.GOOS != "linux" {
		return api.Boot500JSONResponse{Code: 500, Error: "boot-target selection is only supported on Linux"}, nil
	}

	switch target {
	case "windows":
		entry := a.config.Boot.WindowsEntry
		if entry == "" {
			return api.Boot400JSONResponse{Code: 400, Error: "no Windows GRUB entry configured ([agent.boot] windows_entry)"}, nil
		}
		// grub-reboot needs root; a NOPASSWD sudoers rule is installed by deploy.
		if out, err := exec.Command("sudo", "-n", "grub-reboot", entry).CombinedOutput(); err != nil {
			return api.Boot500JSONResponse{Code: 500, Error: "grub-reboot failed: " + string(out)}, nil
		}
		log.Printf("Boot: set one-shot GRUB next-boot to %q; rebooting", entry)
		rebootAfterDelay()
		msg := "Rebooting into Windows"
		return api.Boot200JSONResponse{Success: true, Message: &msg}, nil

	case "linux", "":
		log.Printf("Boot: rebooting into the default (Linux)")
		rebootAfterDelay()
		msg := "Rebooting"
		return api.Boot200JSONResponse{Success: true, Message: &msg}, nil

	default:
		return api.Boot400JSONResponse{Code: 400, Error: "unknown target (expected linux or windows)"}, nil
	}
}

// rebootAfterDelay reboots after a short delay so the HTTP response can flush.
func rebootAfterDelay() {
	go func() {
		time.Sleep(1 * time.Second)
		if out, err := exec.Command("systemctl", "reboot").CombinedOutput(); err != nil {
			log.Printf("reboot command failed: %v: %s", err, string(out))
		}
	}()
}
