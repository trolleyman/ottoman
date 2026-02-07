package client

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

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
	if err := run("systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}

	if err := run("systemctl", "--user", "enable", "ottoman-client"); err != nil {
		return err
	}

	fmt.Println("Service installed successfully!")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  Start:   systemctl --user start ottoman-client")
	fmt.Println("  Stop:    systemctl --user stop ottoman-client")
	fmt.Println("  Status:  systemctl --user status ottoman-client")
	fmt.Println("  Logs:    journalctl --user -u ottoman-client -f")
	fmt.Println()
	fmt.Println("To start on boot, run: loginctl enable-linger")

	return nil
}

const taskXmlTemplate = `<?xml version="1.0"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Author>Ottoman</Author>
    <Description>%s</Description>
  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>%s</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>false</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <IdleSettings>
      <StopOnIdleEnd>true</StopOnIdleEnd>
      <RestartOnIdle>false</RestartOnIdle>
    </IdleSettings>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>true</Hidden>
    <RunOnlyIfIdle>false</RunOnlyIfIdle>
    <WakeToRun>false</WakeToRun>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>7</Priority>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>%s</Command>
      <Arguments>%s</Arguments>
    </Exec>
  </Actions>
</Task>`

const autohotkeyScript = `#Requires AutoHotkey v2.0
#SingleInstance Force

; Define the function once
ApplyLayout(num) {
    Run(A_ScriptDir "\ottoman.exe client layout apply " num)
}

; Call the function for each hotkey
#^1::ApplyLayout(1)
#^2::ApplyLayout(2)
#^3::ApplyLayout(3)
#^4::ApplyLayout(4)
#^5::ApplyLayout(5)
#^6::ApplyLayout(6)
#^7::ApplyLayout(7)
#^8::ApplyLayout(8)
#^9::ApplyLayout(9)
`

const windowsAHKStartupVbs = `Set WshShell = CreateObject("WScript.Shell")
WshShell.Run """%s""", 1, False
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

	// Clean up previous installations (ignore errors)
	exec.Command("schtasks", "/End", "/TN", "OttomanClient").Run()
	exec.Command("schtasks", "/Delete", "/TN", "OttomanClient", "/F").Run()
	exec.Command("schtasks", "/End", "/TN", "OttomanHotkeys").Run()
	exec.Command("schtasks", "/Delete", "/TN", "OttomanHotkeys", "/F").Run()

	// --- Install Scheduled Task (Main Client) ---

	currentUser, err := user.Current()
	if err != nil {
		return errors.Wrap(err, "failed to get current user")
	}

	// Create XML definition
	xmlContent := fmt.Sprintf(taskXmlTemplate, "Ottoman Display Control Client", currentUser.Username, binPath, "client run")
	xmlPath := filepath.Join(configDir, "ottoman_task.xml")
	if err := os.WriteFile(xmlPath, []byte(xmlContent), 0644); err != nil {
		return errors.Wrap(err, "failed to write task XML")
	}
	defer os.Remove(xmlPath) // Cleanup temp file

	// Register Task
	// /F forces overwrite
	if err := runSchtasks("/Create", "/TN", "OttomanClient", "/XML", xmlPath, "/F"); err != nil {
		return err
	}

	fmt.Println("Task Scheduler task installed successfully!")
	fmt.Println()
	fmt.Println("The client will start automatically at login (hidden).")
	fmt.Println("To start now, run: schtasks /Run /TN OttomanClient")

	// --- Install AHK script ---

	// Write AHK script to binDir (where ottoman.exe is)
	binDir := filepath.Dir(binPath)
	ahkPath := filepath.Join(binDir, "ottoman.ahk")
	if err := os.WriteFile(ahkPath, []byte(autohotkeyScript), 0644); err != nil {
		return errors.Wrap(err, "failed to write AHK script")
	}

	// Create VBS launcher for AHK
	ahkVbsPath := filepath.Join(configDir, "ottoman-ahk.vbs")
	ahkVbsContent := fmt.Sprintf(windowsAHKStartupVbs, ahkPath)
	if err := os.WriteFile(ahkVbsPath, []byte(ahkVbsContent), 0644); err != nil {
		return errors.Wrap(err, "failed to write AHK startup script")
	}

	// Create Shortcut in Startup folder
	ahkShortcutPath := filepath.Join(startupDir, "Ottoman Hotkeys.lnk")
	if err := makeLink(ahkVbsPath, ahkShortcutPath); err != nil {
		return errors.Wrap(err, "failed to create AHK shortcut")
	}

	// Remove old AHK VBS shortcut if it exists (cleanup)
	oldAhkShortcutPath := filepath.Join(startupDir, "Ottoman Hotkeys.vbs")
	os.Remove(oldAhkShortcutPath)

	fmt.Println("AHK script installed as shortcut successfully!")
	fmt.Printf("  Script: %s\n", ahkPath)
	fmt.Printf("  Shortcut: %s\n", ahkShortcutPath)

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
	run("systemctl", "--user", "stop", "ottoman-client")
	run("systemctl", "--user", "disable", "ottoman-client")

	// Remove service file
	servicePath := filepath.Join(home, ".config/systemd/user/ottoman-client.service")
	os.Remove(servicePath)

	// Reload
	run("systemctl", "--user", "daemon-reload")

	fmt.Println("Service uninstalled.")
	return nil
}

func uninstallWindowsService() error {
	appData := os.Getenv("APPDATA")
	startupDir := filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "Startup")

	// Remove Scheduled Tasks
	runSchtasks("/Delete", "/TN", "OttomanClient", "/F")
	// runSchtasks("/Delete", "/TN", "OttomanHotkeys", "/F") // No longer used

	// Remove AHK shortcut
	ahkShortcutPath := filepath.Join(startupDir, "Ottoman Hotkeys.lnk")
	os.Remove(ahkShortcutPath)

	// Remove old client shortcut (cleanup)
	clientShortcutPath := filepath.Join(startupDir, "Ottoman Client.vbs")
	os.Remove(clientShortcutPath)

	fmt.Println("Service uninstalled.")
	return nil
}

// Quotes a string for display as a shell argument.
func shellQuoteForce(s string) string {
	containsDoubleQuote := strings.Contains(s, `"`)
	containsSingleQuote := strings.Contains(s, `'`)
	escaped := strings.ReplaceAll(s, "\t", `\t`)
	escaped = strings.ReplaceAll(s, `\`, `\\`)
	if !containsDoubleQuote {
		return `"` + escaped + `"`
	} else if !containsSingleQuote {
		return `'` + escaped + `'`
	} else {
		return `"` + strings.ReplaceAll(escaped, `"`, `\"`) + `"`
	}
}

// Quotes a string for display as a shell argument if necessary.
func shellQuote(s string) string {
	if s == "" {
		return `""`
	}
	containsDoubleQuote := strings.Contains(s, `"`)
	containsSingleQuote := strings.Contains(s, `'`)
	containsQuote := containsDoubleQuote || containsSingleQuote
	containsWhitespace := strings.ContainsAny(s, " \t")
	if containsQuote || containsWhitespace {
		return shellQuoteForce(s)
	}
	return s
}

// formatCmd formats a command and its arguments for display.
func formatCmd(cmd string, args ...string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, shellQuote(cmd))
	for _, a := range args {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

// run executes a command and prints it to stdout
func run(name string, args ...string) error {
	fmt.Printf("Running: %s\n", formatCmd(name, args...))
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return errors.Wrapf(err, "failed to run %s", name)
	}
	return nil
}

// runSchtasks runs schtasks and attempts elevation if access is denied
func runSchtasks(args ...string) error {
	name := "schtasks"
	fmt.Printf("Running: %s\n", formatCmd(name, args...))

	var stderr bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errStr := stderr.String()
		// Check for Access is denied
		if strings.Contains(errStr, "Access is denied") {
			fmt.Println("Access denied. Attempting to run with elevation...")
			return runElevated(name, args...)
		}
		fmt.Fprint(os.Stderr, errStr)
		return errors.Wrapf(err, "failed to run %s", name)
	}
	return nil
}

// runElevated runs a command with Administrator privileges using PowerShell
func runElevated(name string, args ...string) error {
	verb := "RunAs"
	exe, _ := exec.LookPath("powershell.exe")

	var psArgList []string
	for _, arg := range args {
		// Escape single quotes for PowerShell
		arg = strings.ReplaceAll(arg, "'", "''")
		// Wrap in single quotes
		psArgList = append(psArgList, fmt.Sprintf("'%s'", arg))
	}
	argsStr := strings.Join(psArgList, ", ")

	psCmd := fmt.Sprintf("Start-Process -FilePath '%s' -ArgumentList %s -Verb %s -Wait -WindowStyle Hidden", name, argsStr, verb)

	cmd := exec.Command(exe, "-NoProfile", "-NonInteractive", "-Command", psCmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
