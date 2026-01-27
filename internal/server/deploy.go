package server

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pkg/errors"
)

const systemdService = `[Unit]
Description=Ottoman Home Automation Server
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/ottoman server run
Restart=always
RestartSec=5
User=ottoman
Group=ottoman

[Install]
WantedBy=multi-user.target
`

// InstallService installs the systemd service
func InstallService() error {
	if runtime.GOOS != "linux" {
		return errors.New("systemd service installation only supported on Linux")
	}

	// Check if running as root
	if os.Geteuid() != 0 {
		return errors.New("must run as root to install systemd service")
	}

	// Write service file
	servicePath := "/etc/systemd/system/ottoman-server.service"
	if err := os.WriteFile(servicePath, []byte(systemdService), 0644); err != nil {
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

	// Create default config if it doesn't exist
	configPath := filepath.Join(configDir, "server.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		config := DefaultConfig()
		if err := config.Save(configPath); err != nil {
			return errors.Wrap(err, "failed to create default config")
		}
		fmt.Printf("Created default config at %s\n", configPath)
		fmt.Println("Please edit the config file and set your MAC addresses and authentication.")
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

// Deploy deploys the server to a remote target via SSH
func Deploy(target string) error {
	if target == "" {
		return errors.New("target is required (e.g., pi@raspberrypi.local)")
	}

	// Parse target
	parts := strings.Split(target, "@")
	if len(parts) != 2 {
		return errors.New("target must be in format user@host")
	}

	// Build for Raspberry Pi (linux/arm)
	fmt.Println("Building ottoman for Raspberry Pi (linux/arm)...")

	binaryPath := filepath.Join(os.TempDir(), "ottoman-linux-arm")

	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/ottoman")
	buildCmd.Env = append(os.Environ(),
		"GOOS=linux",
		"GOARCH=arm",
		"GOARM=7",
	)
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr

	if err := buildCmd.Run(); err != nil {
		return errors.Wrap(err, "build failed")
	}

	// Copy to target
	fmt.Printf("Copying to %s...\n", target)
	scpCmd := exec.Command("scp", binaryPath, target+":/tmp/ottoman")
	scpCmd.Stdout = os.Stdout
	scpCmd.Stderr = os.Stderr

	if err := scpCmd.Run(); err != nil {
		return errors.Wrap(err, "scp failed")
	}

	// Install on target
	fmt.Println("Installing on target...")
	installScript := `
		sudo mv /tmp/ottoman /usr/local/bin/ottoman && \
		sudo chmod +x /usr/local/bin/ottoman && \
		sudo /usr/local/bin/ottoman server install && \
		sudo systemctl restart ottoman-server
	`

	sshCmd := exec.Command("ssh", target, installScript)
	sshCmd.Stdout = os.Stdout
	sshCmd.Stderr = os.Stderr

	if err := sshCmd.Run(); err != nil {
		return errors.Wrap(err, "remote installation failed")
	}

	fmt.Println("Deployment complete!")
	return nil
}
