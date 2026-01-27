package client

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"
)

const linuxSystemdService = `[Unit]
Description=Ottoman Display Control Client
After=graphical.target

[Service]
Type=simple
ExecStart=/usr/local/bin/ottoman client run
Restart=always
RestartSec=5
User=%s
Environment=DISPLAY=:0
Environment=XAUTHORITY=%s/.Xauthority

[Install]
WantedBy=graphical.target
`

// InstallService installs the appropriate service for the current platform
func InstallService() error {
	switch runtime.GOOS {
	case "linux":
		return installLinuxService()
	case "windows":
		return installWindowsService()
	default:
		return errors.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// installLinuxService installs a systemd user service
func installLinuxService() error {
	user := os.Getenv("USER")
	home := os.Getenv("HOME")

	if user == "" || home == "" {
		return errors.New("USER and HOME environment variables must be set")
	}

	// User-level systemd service
	serviceDir := filepath.Join(home, ".config/systemd/user")
	if err := os.MkdirAll(serviceDir, 0755); err != nil {
		return errors.Wrap(err, "failed to create systemd user directory")
	}

	serviceContent := fmt.Sprintf(linuxSystemdService, user, home)
	servicePath := filepath.Join(serviceDir, "ottoman-client.service")

	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return errors.Wrap(err, "failed to write service file")
	}

	// Create config directory
	configDir := filepath.Join(home, ".config/ottoman")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return errors.Wrap(err, "failed to create config directory")
	}

	// Create default config if it doesn't exist
	configPath := filepath.Join(configDir, "client.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		config := DefaultConfig()
		if err := config.Save(configPath); err != nil {
			return errors.Wrap(err, "failed to create default config")
		}
		fmt.Printf("Created default config at %s\n", configPath)
	}

	// Create default layouts file if it doesn't exist
	layoutsPath := filepath.Join(configDir, "layouts.json")
	if _, err := os.Stat(layoutsPath); os.IsNotExist(err) {
		if err := os.WriteFile(layoutsPath, []byte("[]"), 0644); err != nil {
			return errors.Wrap(err, "failed to create layouts file")
		}
		fmt.Printf("Created empty layouts file at %s\n", layoutsPath)
	}

	// Reload and enable
	if err := exec.Command("systemctl", "--user", "daemon-reload").Run(); err != nil {
		return errors.Wrap(err, "failed to reload systemd")
	}

	if err := exec.Command("systemctl", "--user", "enable", "ottoman-client").Run(); err != nil {
		return errors.Wrap(err, "failed to enable service")
	}

	fmt.Println("Service installed successfully!")
	fmt.Println("Start with: systemctl --user start ottoman-client")
	fmt.Println("Check status: systemctl --user status ottoman-client")
	fmt.Println()
	fmt.Println("To start on login, run: loginctl enable-linger", user)

	return nil
}

// installWindowsService installs a Windows service using NSSM or Task Scheduler
func installWindowsService() error {
	// Get the path to the current executable
	exePath, err := os.Executable()
	if err != nil {
		return errors.Wrap(err, "failed to get executable path")
	}

	// Create config directory
	configDir := filepath.Join(os.Getenv("APPDATA"), "ottoman")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return errors.Wrap(err, "failed to create config directory")
	}

	// Create default config if it doesn't exist
	configPath := filepath.Join(configDir, "client.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		config := DefaultConfig()
		if err := config.Save(configPath); err != nil {
			return errors.Wrap(err, "failed to create default config")
		}
		fmt.Printf("Created default config at %s\n", configPath)
	}

	// Create default layouts file if it doesn't exist
	layoutsPath := filepath.Join(configDir, "layouts.json")
	if _, err := os.Stat(layoutsPath); os.IsNotExist(err) {
		if err := os.WriteFile(layoutsPath, []byte("[]"), 0644); err != nil {
			return errors.Wrap(err, "failed to create layouts file")
		}
		fmt.Printf("Created empty layouts file at %s\n", layoutsPath)
	}

	// Use Task Scheduler to run at login
	taskName := "OttomanClient"
	taskCmd := fmt.Sprintf(`schtasks /create /tn "%s" /tr "\"%s\" client run" /sc onlogon /rl highest /f`,
		taskName, exePath)

	cmd := exec.Command("cmd", "/c", taskCmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "failed to create scheduled task\nOutput: %s", string(output))
	}

	fmt.Println("Scheduled task created successfully!")
	fmt.Printf("Task name: %s\n", taskName)
	fmt.Println()
	fmt.Println("The client will start automatically at login.")
	fmt.Println("To start now, run: ottoman client run")
	fmt.Println("Or start the scheduled task manually.")

	return nil
}

// Deploy installs the client on the local system
func Deploy() error {
	fmt.Println("Deploying ottoman client...")

	// Install service
	if err := InstallService(); err != nil {
		return errors.Wrap(err, "service installation failed")
	}

	return nil
}

// Uninstall removes the installed service
func Uninstall() error {
	switch runtime.GOOS {
	case "linux":
		return uninstallLinuxService()
	case "windows":
		return uninstallWindowsService()
	default:
		return errors.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func uninstallLinuxService() error {
	home := os.Getenv("HOME")

	// Stop and disable
	exec.Command("systemctl", "--user", "stop", "ottoman-client").Run()
	exec.Command("systemctl", "--user", "disable", "ottoman-client").Run()

	// Remove service file
	servicePath := filepath.Join(home, ".config/systemd/user/ottoman-client.service")
	os.Remove(servicePath)

	// Reload
	exec.Command("systemctl", "--user", "daemon-reload").Run()

	fmt.Println("Service uninstalled.")
	return nil
}

func uninstallWindowsService() error {
	taskName := "OttomanClient"
	cmd := exec.Command("schtasks", "/delete", "/tn", taskName, "/f")
	cmd.Run()

	fmt.Println("Scheduled task removed.")
	return nil
}
