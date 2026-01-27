package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"github.com/trolleyman/ottoman/internal/common"
)

// Config holds all server configuration
type Config struct {
	ListenAddr   string              `json:"listen_addr"`
	AuthToken    string              `json:"auth_token,omitempty"`
	Username     string              `json:"username,omitempty"`
	PasswordHash string              `json:"password_hash,omitempty"`
	WakeTargets  []common.WakeTarget `json:"wake_targets"`
	ClientAddr   string              `json:"client_addr"`
	PingURL      string              `json:"ping_url,omitempty"`
	PingInterval time.Duration       `json:"ping_interval,omitempty"`
	DeviceID     string              `json:"device_id,omitempty"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		ListenAddr: ":8080",
		ClientAddr: "hades:8081",
		WakeTargets: []common.WakeTarget{
			{
				Name:       "hades",
				MACAddress: "00:00:00:00:00:00", // User must configure
			},
		},
		PingInterval: 5 * time.Minute,
		DeviceID:     "ottoman-pi",
	}
}

// LoadConfig loads configuration from a file or returns defaults
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		// Try default locations
		candidates := []string{
			"ottoman-server.json",
			"/etc/ottoman/server.json",
			filepath.Join(os.Getenv("HOME"), ".config/ottoman/server.json"),
		}

		for _, candidate := range candidates {
			if _, err := os.Stat(candidate); err == nil {
				path = candidate
				break
			}
		}

		if path == "" {
			// Return default config if no file found
			return DefaultConfig(), nil
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read config file")
	}

	config := DefaultConfig()
	if err := json.Unmarshal(data, config); err != nil {
		return nil, errors.Wrap(err, "failed to parse config file")
	}

	return config, nil
}

// Save writes the configuration to a file
func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal config")
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.Wrap(err, "failed to create config directory")
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return errors.Wrap(err, "failed to write config file")
	}

	return nil
}

// Validate checks that the configuration is valid
func (c *Config) Validate() error {
	if c.ListenAddr == "" {
		return errors.New("listen_addr is required")
	}

	if c.ClientAddr == "" {
		return errors.New("client_addr is required")
	}

	// Check that at least one auth method is configured
	if c.AuthToken == "" && c.Username == "" {
		fmt.Println("Warning: No authentication configured. API will be open.")
	}

	return nil
}
