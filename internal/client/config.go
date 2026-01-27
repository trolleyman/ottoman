package client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"
)

// Config holds all client configuration
type Config struct {
	ListenAddr  string `json:"listen_addr"`
	AuthToken   string `json:"auth_token,omitempty"`
	LayoutsFile string `json:"layouts_file"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	layoutsFile := "layouts.json"
	if runtime.GOOS == "windows" {
		layoutsFile = filepath.Join(os.Getenv("APPDATA"), "ottoman", "layouts.json")
	} else {
		layoutsFile = filepath.Join(os.Getenv("HOME"), ".config/ottoman/layouts.json")
	}

	return &Config{
		ListenAddr:  ":8081",
		LayoutsFile: layoutsFile,
	}
}

// LoadConfig loads configuration from a file or returns defaults
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		// Try default locations
		var candidates []string

		if runtime.GOOS == "windows" {
			candidates = []string{
				"ottoman-client.json",
				filepath.Join(os.Getenv("APPDATA"), "ottoman", "client.json"),
			}
		} else {
			candidates = []string{
				"ottoman-client.json",
				"/etc/ottoman/client.json",
				filepath.Join(os.Getenv("HOME"), ".config/ottoman/client.json"),
			}
		}

		for _, candidate := range candidates {
			if _, err := os.Stat(candidate); err == nil {
				path = candidate
				break
			}
		}

		if path == "" {
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

	if c.LayoutsFile == "" {
		return errors.New("layouts_file is required")
	}

	return nil
}
