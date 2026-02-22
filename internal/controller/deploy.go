package controller

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"
	"github.com/trolleyman/ottoman/internal/common"
)

const systemdControllerTemplate = `[Unit]
Description=Ottoman Home Automation Controller
After=network.target

[Service]
Type=simple
ExecStart=%s controller run
Restart=always
RestartSec=5
User=ottoman
Group=ottoman

[Install]
WantedBy=multi-user.target
`

const systemdUserControllerTemplate = `[Unit]
Description=Ottoman Home Automation Controller
After=network.target

[Service]
Type=simple
ExecStart=%s controller run
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
`

// InstallService installs the systemd service
func InstallService() error {
	if runtime.GOOS != "linux" {
		return errors.New("systemd service installation only supported on Linux")
	}

	// Check if running as root
	if os.Geteuid() != 0 {
		return installUserService()
	}

	// Get current binary path
	binPath, err := os.Executable()
	if err != nil {
		return errors.Wrap(err, "failed to get executable path")
	}
	binPath, err = filepath.EvalSymlinks(binPath)
	if err != nil {
		return errors.Wrap(err, "failed to resolve executable path")
	}

	// Write service file
	servicePath := "/etc/systemd/system/ottoman-controller.service"
	serviceContent := fmt.Sprintf(systemdControllerTemplate, binPath)
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return errors.Wrap(err, "failed to write service file")
	}

	// Create ottoman user if it doesn't exist
	if err := common.RunCmd("id", "ottoman"); err != nil {
		log.Println("Creating ottoman user...")
		if err := common.RunCmd("useradd", "-r", "-s", "/bin/false", "ottoman"); err != nil {
			log.Printf("Warning: failed to create ottoman user: %v", err)
		}
	}

	// Create config directory
	configDir := "/etc/ottoman"
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return errors.Wrap(err, "failed to create config directory")
	}

	// Check if config exists
	configPath := filepath.Join(configDir, "config.toml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Printf("Config not found at %s", configPath)
		log.Println("Run 'ottoman config init controller' to create it.")
	}

	// Reload systemd
	if err := common.RunCmd("systemctl", "daemon-reload"); err != nil {
		return err
	}

	// Enable service
	if err := common.RunCmd("systemctl", "enable", "ottoman-controller"); err != nil {
		return err
	}

	log.Println("Service installed successfully!")
	log.Println("Start with: sudo systemctl start ottoman-controller")
	log.Println("Check status: sudo systemctl status ottoman-controller")

	return nil
}

// installUserService installs the systemd service for the current user
func installUserService() error {
	home := os.Getenv("HOME")
	if home == "" {
		return errors.New("HOME environment variable must be set")
	}

	// Get current binary path
	binPath, err := os.Executable()
	if err != nil {
		return errors.Wrap(err, "failed to get executable path")
	}
	binPath, err = filepath.EvalSymlinks(binPath)
	if err != nil {
		return errors.Wrap(err, "failed to resolve executable path")
	}

	// User-level systemd service
	serviceDir := filepath.Join(home, ".config/systemd/user")
	if err := os.MkdirAll(serviceDir, 0755); err != nil {
		return errors.Wrap(err, "failed to create systemd user directory")
	}

	serviceContent := fmt.Sprintf(systemdUserControllerTemplate, binPath)
	servicePath := filepath.Join(serviceDir, "ottoman-controller.service")

	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return errors.Wrap(err, "failed to write service file")
	}

	// Reload and enable
	if err := common.RunCmd("systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}

	if err := common.RunCmd("systemctl", "--user", "enable", "ottoman-controller"); err != nil {
		return err
	}

	// Enable lingering so user services start on boot
	if err := common.RunCmd("loginctl", "enable-linger"); err != nil {
		log.Printf("Warning: failed to enable linger: %v", err)
	}

	// Restart the service
	if err := common.RunCmd("systemctl", "--user", "restart", "ottoman-controller"); err != nil {
		return err
	}

	// Print status
	common.RunCmdSilent("systemctl", "--user", "status", "ottoman-controller")

	log.Println("User service installed successfully!")
	log.Println()
	log.Println("Commands:")
	log.Println("  Stop:    systemctl --user stop ottoman-controller")
	log.Println("  Status:  systemctl --user status ottoman-controller")
	log.Println("  Logs:    journalctl --user -u ottoman-controller -f")

	return nil
}

// UninstallService removes the systemd service
func UninstallService() error {
	if runtime.GOOS != "linux" {
		return errors.New("systemd service uninstallation only supported on Linux")
	}

	// Check if running as root
	if os.Geteuid() != 0 {
		return uninstallUserService()
	}

	// Stop service (ignore errors - might not be running)
	common.RunCmdSilent("systemctl", "stop", "ottoman-controller")

	// Disable service (ignore errors - might not be enabled)
	common.RunCmdSilent("systemctl", "disable", "ottoman-controller")

	// Remove service file
	servicePath := "/etc/systemd/system/ottoman-controller.service"
	if err := os.Remove(servicePath); err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, "failed to remove service file")
	}

	// Reload systemd
	if err := common.RunCmd("systemctl", "daemon-reload"); err != nil {
		return err
	}

	log.Println("Service uninstalled successfully!")
	return nil
}

func uninstallUserService() error {
	home := os.Getenv("HOME")

	// Stop and disable
	common.RunCmdSilent("systemctl", "--user", "stop", "ottoman-controller")
	common.RunCmdSilent("systemctl", "--user", "disable", "ottoman-controller")

	// Remove service file
	servicePath := filepath.Join(home, ".config/systemd/user/ottoman-controller.service")
	if err := os.Remove(servicePath); err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, "failed to remove service file")
	}

	// Reload systemd
	if err := common.RunCmd("systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}

	log.Println("User service uninstalled successfully!")
	return nil
}
