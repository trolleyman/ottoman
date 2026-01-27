package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

// Config holds the complete ottoman configuration
type Config struct {
	Server ServerConfig `mapstructure:"server"`
	Client ClientConfig `mapstructure:"client"`
}

// ServerConfig holds server configuration
type ServerConfig struct {
	ListenAddr   string        `mapstructure:"listen_addr"`
	AuthToken    string        `mapstructure:"auth_token"`
	Username     string        `mapstructure:"username"`
	PasswordHash string        `mapstructure:"password_hash"`
	WakeTargets  []WakeTarget  `mapstructure:"wake_targets"`
	ClientAddr   string        `mapstructure:"client_addr"`
	PingURL      string        `mapstructure:"ping_url"`
	PingInterval time.Duration `mapstructure:"ping_interval"`
	DeviceID     string        `mapstructure:"device_id"`
}

// ClientConfig holds client configuration
type ClientConfig struct {
	ListenAddr  string `mapstructure:"listen_addr"`
	AuthToken   string `mapstructure:"auth_token"`
	LayoutsFile string `mapstructure:"layouts_file"`
}

// WakeTarget represents a device that can be woken via WoL
type WakeTarget struct {
	Name       string `mapstructure:"name" json:"name"`
	MACAddress string `mapstructure:"mac_address" json:"mac_address"`
	IPAddress  string `mapstructure:"ip_address" json:"ip_address,omitempty"`
	Port       int    `mapstructure:"port" json:"port,omitempty"`
}

var (
	v          *viper.Viper
	configFile string
	configPath string
)

// Init initializes the configuration system
func Init(cfgFile string) {
	v = viper.New()
	configFile = cfgFile

	v.SetConfigType("toml")
	v.SetConfigName("ottoman")

	// Set defaults
	setDefaults()

	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
		configPath = cfgFile
	} else {
		// Add config search paths
		addConfigPaths()
	}

	// Enable environment variable overrides
	v.SetEnvPrefix("OTTOMAN")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
}

func setDefaults() {
	// Server defaults
	v.SetDefault("server.listen_addr", ":8080")
	v.SetDefault("server.client_addr", "localhost:8081")
	v.SetDefault("server.ping_interval", 5*time.Minute)
	v.SetDefault("server.device_id", "ottoman")
	v.SetDefault("server.wake_targets", []WakeTarget{})

	// Client defaults
	v.SetDefault("client.listen_addr", ":8081")
	v.SetDefault("client.layouts_file", defaultLayoutsFile())
}

func defaultLayoutsFile() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("APPDATA"), "ottoman", "layouts.toml")
	}
	return filepath.Join(os.Getenv("HOME"), ".config", "ottoman", "layouts.toml")
}

func addConfigPaths() {
	// Current directory
	v.AddConfigPath(".")

	if runtime.GOOS == "windows" {
		// Windows: %APPDATA%/ottoman/
		if appData := os.Getenv("APPDATA"); appData != "" {
			v.AddConfigPath(filepath.Join(appData, "ottoman"))
		}
	} else {
		// Unix: /etc/ottoman/ and ~/.config/ottoman/
		v.AddConfigPath("/etc/ottoman")
		if home := os.Getenv("HOME"); home != "" {
			v.AddConfigPath(filepath.Join(home, ".config", "ottoman"))
		}
	}
}

// Load reads the configuration from file
func Load() (*Config, error) {
	if v == nil {
		Init("")
	}

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found, use defaults
			configPath = ""
		} else {
			return nil, errors.Wrap(err, "failed to read config file")
		}
	} else {
		configPath = v.ConfigFileUsed()
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal config")
	}

	return &cfg, nil
}

// GetServer returns just the server configuration
func GetServer() (*ServerConfig, error) {
	cfg, err := Load()
	if err != nil {
		return nil, err
	}
	return &cfg.Server, nil
}

// GetClient returns just the client configuration
func GetClient() (*ClientConfig, error) {
	cfg, err := Load()
	if err != nil {
		return nil, err
	}
	return &cfg.Client, nil
}

// ConfigPath returns the path of the loaded config file, or empty if using defaults
func ConfigPath() string {
	return configPath
}

// DefaultConfigPath returns the default config file path for the current platform
func DefaultConfigPath() string {
	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "ottoman", "ottoman.toml")
		}
	} else {
		if home := os.Getenv("HOME"); home != "" {
			return filepath.Join(home, ".config", "ottoman", "ottoman.toml")
		}
	}
	return "ottoman.toml"
}

// SystemConfigPath returns the system-wide config file path (Unix only)
func SystemConfigPath() string {
	if runtime.GOOS == "windows" {
		return DefaultConfigPath()
	}
	return "/etc/ottoman/ottoman.toml"
}

// Save writes the configuration to a file
func Save(cfg *Config, path string) error {
	if path == "" {
		path = DefaultConfigPath()
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.Wrap(err, "failed to create config directory")
	}

	// Create a new viper instance for writing
	w := viper.New()
	w.SetConfigType("toml")

	// Set all values
	w.Set("server", cfg.Server)
	w.Set("client", cfg.Client)

	if err := w.WriteConfigAs(path); err != nil {
		return errors.Wrap(err, "failed to write config file")
	}

	return nil
}

// ValidateServer checks that server configuration is valid
func (c *ServerConfig) Validate() error {
	if c.ListenAddr == "" {
		return errors.New("server.listen_addr is required")
	}

	if c.ClientAddr == "" {
		return errors.New("server.client_addr is required")
	}

	if c.AuthToken == "" && c.Username == "" {
		fmt.Println("Warning: No authentication configured. API will be open.")
	}

	return nil
}

// ValidateClient checks that client configuration is valid
func (c *ClientConfig) Validate() error {
	if c.ListenAddr == "" {
		return errors.New("client.listen_addr is required")
	}

	if c.LayoutsFile == "" {
		return errors.New("client.layouts_file is required")
	}

	return nil
}

// Print outputs the current configuration to stdout
func Print(cfg *Config) {
	fmt.Println("# Ottoman Configuration")
	if configPath != "" {
		fmt.Printf("# Loaded from: %s\n", configPath)
	} else {
		fmt.Println("# Using defaults (no config file found)")
	}
	fmt.Println()

	fmt.Println("[server]")
	fmt.Printf("listen_addr = %q\n", cfg.Server.ListenAddr)
	if cfg.Server.AuthToken != "" {
		fmt.Printf("auth_token = %q\n", cfg.Server.AuthToken)
	}
	if cfg.Server.Username != "" {
		fmt.Printf("username = %q\n", cfg.Server.Username)
	}
	if cfg.Server.PasswordHash != "" {
		fmt.Printf("password_hash = %q\n", cfg.Server.PasswordHash)
	}
	fmt.Printf("client_addr = %q\n", cfg.Server.ClientAddr)
	if cfg.Server.PingURL != "" {
		fmt.Printf("ping_url = %q\n", cfg.Server.PingURL)
	}
	fmt.Printf("ping_interval = %q\n", cfg.Server.PingInterval.String())
	fmt.Printf("device_id = %q\n", cfg.Server.DeviceID)

	if len(cfg.Server.WakeTargets) > 0 {
		fmt.Println()
		for _, target := range cfg.Server.WakeTargets {
			fmt.Println("[[server.wake_targets]]")
			fmt.Printf("name = %q\n", target.Name)
			fmt.Printf("mac_address = %q\n", target.MACAddress)
			if target.IPAddress != "" {
				fmt.Printf("ip_address = %q\n", target.IPAddress)
			}
			if target.Port != 0 {
				fmt.Printf("port = %d\n", target.Port)
			}
		}
	}

	fmt.Println()
	fmt.Println("[client]")
	fmt.Printf("listen_addr = %q\n", cfg.Client.ListenAddr)
	if cfg.Client.AuthToken != "" {
		fmt.Printf("auth_token = %q\n", cfg.Client.AuthToken)
	}
	fmt.Printf("layouts_file = %q\n", cfg.Client.LayoutsFile)
}

// PrintPaths outputs the config search paths
func PrintPaths() {
	fmt.Println("Config search paths:")
	fmt.Println("  1. ./ottoman.toml")

	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			fmt.Printf("  2. %s\n", filepath.Join(appData, "ottoman", "ottoman.toml"))
		}
	} else {
		fmt.Println("  2. /etc/ottoman/ottoman.toml")
		if home := os.Getenv("HOME"); home != "" {
			fmt.Printf("  3. %s\n", filepath.Join(home, ".config", "ottoman", "ottoman.toml"))
		}
	}

	fmt.Println()
	fmt.Printf("Default config path: %s\n", DefaultConfigPath())
}
