package server

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"
)

const systemdServiceTemplate = `[Unit]
Description=Ottoman Home Automation Server
After=network.target

[Service]
Type=simple
ExecStart=%s server run
Restart=always
RestartSec=5
User=ottoman
Group=ottoman

[Install]
WantedBy=multi-user.target
`

const systemdUserServiceTemplate = `[Unit]
Description=Ottoman Home Automation Server
After=network.target

[Service]
Type=simple
ExecStart=%s server run
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
	servicePath := "/etc/systemd/system/ottoman-server.service"
	serviceContent := fmt.Sprintf(systemdServiceTemplate, binPath)
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
		fmt.Println("Run 'ottoman config init server' to create it.")
	}

	// Reload systemd
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return errors.Wrap(err, "failed to reload systemd")
	}

	// Enable service
	if err := exec.Command("systemctl", "enable", "ottoman-server").Run(); err != nil {
		return errors.Wrap(err, "failed to enable service")
	}

	fmt.Println("Service installed successfully!")
	fmt.Println("Start with: sudo systemctl start ottoman-server")
	fmt.Println("Check status: sudo systemctl status ottoman-server")

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

	serviceContent := fmt.Sprintf(systemdUserServiceTemplate, binPath)
	servicePath := filepath.Join(serviceDir, "ottoman-server.service")

	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return errors.Wrap(err, "failed to write service file")
	}

	// Reload and enable
	if err := exec.Command("systemctl", "--user", "daemon-reload").Run(); err != nil {
		return errors.Wrap(err, "failed to reload systemd")
	}

	if err := exec.Command("systemctl", "--user", "enable", "ottoman-server").Run(); err != nil {
		return errors.Wrap(err, "failed to enable service")
	}

	fmt.Println("User service installed successfully!")
	fmt.Println("Start with: systemctl --user start ottoman-server")
	fmt.Println("Check status: systemctl --user status ottoman-server")
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
	exec.Command("systemctl", "stop", "ottoman-server").Run()

	// Disable service (ignore errors - might not be enabled)
	fmt.Println("Disabling service...")
	exec.Command("systemctl", "disable", "ottoman-server").Run()

	// Remove service file
	servicePath := "/etc/systemd/system/ottoman-server.service"
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
	exec.Command("systemctl", "--user", "stop", "ottoman-server").Run()
	exec.Command("systemctl", "--user", "disable", "ottoman-server").Run()

	// Remove service file
	servicePath := filepath.Join(home, ".config/systemd/user/ottoman-server.service")
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
