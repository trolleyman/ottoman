package controller

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"
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
	if err := exec.Command("id", "ottoman").Run(); err != nil {
		fmt.Println("Creating ottoman user...")
		if err := exec.Command("useradd", "-r", "-s", "/bin/false", "ottoman").Run(); err != nil {
			fmt.Printf("Warning: failed to create ottoman user: %v\n", err)
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
		fmt.Printf("Config not found at %s\n", configPath)
		fmt.Println("Run 'ottoman config init controller' to create it.")
	}

	// Reload systemd
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return errors.Wrap(err, "failed to reload systemd")
	}

	// Enable service
	if err := exec.Command("systemctl", "enable", "ottoman-controller").Run(); err != nil {
		return errors.Wrap(err, "failed to enable service")
	}

	fmt.Println("Service installed successfully!")
	fmt.Println("Start with: sudo systemctl start ottoman-controller")
	fmt.Println("Check status: sudo systemctl status ottoman-controller")

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
	if err := exec.Command("systemctl", "--user", "daemon-reload").Run(); err != nil {
		return errors.Wrap(err, "failed to reload systemd")
	}

	if err := exec.Command("systemctl", "--user", "enable", "ottoman-controller").Run(); err != nil {
		return errors.Wrap(err, "failed to enable service")
	}

	fmt.Println("User service installed successfully!")
	fmt.Println("Start with: systemctl --user start ottoman-controller")
	fmt.Println("Check status: systemctl --user status ottoman-controller")
	fmt.Println("To start on boot, run: loginctl enable-linger")

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
	fmt.Println("Stopping service...")
	exec.Command("systemctl", "stop", "ottoman-controller").Run()

	// Disable service (ignore errors - might not be enabled)
	fmt.Println("Disabling service...")
	exec.Command("systemctl", "disable", "ottoman-controller").Run()

	// Remove service file
	servicePath := "/etc/systemd/system/ottoman-controller.service"
	if err := os.Remove(servicePath); err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, "failed to remove service file")
	}

	// Reload systemd
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return errors.Wrap(err, "failed to reload systemd")
	}

	fmt.Println("Service uninstalled successfully!")
	return nil
}

func uninstallUserService() error {
	home := os.Getenv("HOME")

	// Stop and disable
	exec.Command("systemctl", "--user", "stop", "ottoman-controller").Run()
	exec.Command("systemctl", "--user", "disable", "ottoman-controller").Run()

	// Remove service file
	servicePath := filepath.Join(home, ".config/systemd/user/ottoman-controller.service")
	if err := os.Remove(servicePath); err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, "failed to remove service file")
	}

	// Reload systemd
	if err := exec.Command("systemctl", "--user", "daemon-reload").Run(); err != nil {
		return errors.Wrap(err, "failed to reload systemd")
	}

	fmt.Println("User service uninstalled successfully!")
	return nil
}
