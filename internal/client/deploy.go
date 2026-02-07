package client

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"
)

// InstallPaths returns the installation paths for the current platform
func InstallPaths() (binPath, configDir string) {
	switch runtime.GOOS {
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			localAppData = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
		}
		binPath = filepath.Join(localAppData, "ottoman", "ottoman.exe")
		configDir = filepath.Join(os.Getenv("APPDATA"), "ottoman")
	default: // linux, darwin
		home := os.Getenv("HOME")
		binPath = filepath.Join(home, ".local", "bin", "ottoman")
		configDir = filepath.Join(home, ".config", "ottoman")
	}
	return
}

// Install registers the ottoman client as a service for autostart.
// This is called after the binary has been deployed to the target location.
func Install() error {
	return InstallService()
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// Remove existing file first (in case it's in use on Windows)
	os.Remove(dst)

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

const linuxSystemdService = `[Unit]
Description=Ottoman Display Control Client
After=graphical.target

[Service]
Type=simple
ExecStart=%s client run
Restart=always
RestartSec=5
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

	serviceContent := fmt.Sprintf(linuxSystemdService, binPath, home)
	servicePath := filepath.Join(serviceDir, "ottoman-client.service")

	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return errors.Wrap(err, "failed to write service file")
	}

	// Reload and enable
	if err := exec.Command("systemctl", "--user", "daemon-reload").Run(); err != nil {
		return errors.Wrap(err, "failed to reload systemd")
	}

	if err := exec.Command("systemctl", "--user", "enable", "ottoman-client").Run(); err != nil {
		return errors.Wrap(err, "failed to enable service")
	}

	fmt.Println("Service installed successfully!")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  Start:   systemctl --user start ottoman-client")
	fmt.Println("  Stop:    systemctl --user stop ottoman-client")
	fmt.Println("  Status:  systemctl --user status ottoman-client")
	fmt.Println("  Logs:    journalctl --user -u ottoman-client -f")
	fmt.Println()
	fmt.Println("To start on login, run: loginctl enable-linger $USER")

	return nil
}

const windowsStartupVbs = `Set WshShell = CreateObject("WScript.Shell")
WshShell.Run """%s"" client run", 0, False
`

// installWindowsService creates a startup shortcut/script for Windows
func installWindowsService() error {
	// Get current binary path
	binPath, err := os.Executable()
	if err != nil {
		return errors.Wrap(err, "failed to get executable path")
	}
	binPath, err = filepath.EvalSymlinks(binPath)
	if err != nil {
		return errors.Wrap(err, "failed to resolve executable path")
	}

	// Get startup folder
	appData := os.Getenv("APPDATA")
	configDir := filepath.Join(appData, "ottoman")
	startupDir := filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "Startup")

	// Ensure config directory exists
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return errors.Wrap(err, "failed to create config directory")
	}

	// Create a VBS script to run hidden (no console window)
	vbsPath := filepath.Join(configDir, "ottoman-startup.vbs")
	vbsContent := fmt.Sprintf(windowsStartupVbs, binPath)

	if err := os.WriteFile(vbsPath, []byte(vbsContent), 0644); err != nil {
		return errors.Wrap(err, "failed to write startup script")
	}

	// Create shortcut in startup folder
	shortcutPath := filepath.Join(startupDir, "Ottoman Client.vbs")

	// Copy VBS to startup folder
	if err := copyFile(vbsPath, shortcutPath); err != nil {
		return errors.Wrap(err, "failed to create startup shortcut")
	}

	fmt.Println("Startup script installed successfully!")
	fmt.Printf("  Script: %s\n", shortcutPath)
	fmt.Println()
	fmt.Println("The client will start automatically at login (hidden).")
	fmt.Println()
	fmt.Println("To start now, run:")
	fmt.Printf("  \"%s\" client run\n", binPath)
	fmt.Println()
	fmt.Println("To remove, delete:")
	fmt.Printf("  %s\n", shortcutPath)

	return nil
}

// UninstallService removes the installed service
func UninstallService() error {
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
	appData := os.Getenv("APPDATA")
	startupDir := filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "Startup")
	shortcutPath := filepath.Join(startupDir, "Ottoman Client.vbs")

	if err := os.Remove(shortcutPath); err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, "failed to remove startup script")
	}

	// Also try to remove old schtasks task if it exists
	exec.Command("schtasks", "/delete", "/tn", "OttomanClient", "/f").Run()

	fmt.Println("Service uninstalled.")
	return nil
}
