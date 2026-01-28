package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"github.com/trolleyman/ottoman/internal/common"
)

// GenerateToken creates a cryptographically random token
func GenerateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", errors.Wrap(err, "failed to generate random bytes")
	}
	return hex.EncodeToString(bytes), nil
}

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
	ListenAddr string          `mapstructure:"listen_addr"`
	AuthToken  string          `mapstructure:"auth_token"`
	Layouts    []common.Layout `mapstructure:"layouts"`
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
	v.SetConfigName("config")

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
	v.SetDefault("client.layouts", []common.Layout{})
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
			return filepath.Join(appData, "ottoman", "config.toml")
		}
	} else {
		if home := os.Getenv("HOME"); home != "" {
			return filepath.Join(home, ".config", "ottoman", "config.toml")
		}
	}
	return "config.toml"
}

// SystemConfigPath returns the system-wide config file path (Unix only)
func SystemConfigPath() string {
	if runtime.GOOS == "windows" {
		return DefaultConfigPath()
	}
	return "/etc/ottoman/config.toml"
}

// ensureConfigDir creates the config directory if needed
func ensureConfigDir(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.Wrap(err, "failed to create config directory")
	}
	return nil
}

// SaveClient writes client configuration to a file (client section only)
func SaveClient(cfg *ClientConfig, path string) error {
	if path == "" {
		path = DefaultConfigPath()
	}
	if err := ensureConfigDir(path); err != nil {
		return err
	}

	w := viper.New()
	w.SetConfigType("toml")

	w.Set("client.listen_addr", cfg.ListenAddr)
	w.Set("client.auth_token", cfg.AuthToken)

	if len(cfg.Layouts) > 0 {
		layouts := make([]map[string]interface{}, len(cfg.Layouts))
		for i, l := range cfg.Layouts {
			layout := map[string]interface{}{
				"id":   l.ID,
				"name": l.Name,
			}
			if l.Emoji != "" {
				layout["emoji"] = l.Emoji
			}
			if len(l.Aliases) > 0 {
				layout["aliases"] = l.Aliases
			}
			if len(l.Monitors) > 0 {
				monitors := make([]map[string]interface{}, len(l.Monitors))
				for j, m := range l.Monitors {
					monitors[j] = map[string]interface{}{
						"edid":         m.EDID,
						"port":         m.Port,
						"width":        m.Width,
						"height":       m.Height,
						"refresh_rate": m.RefreshRate,
						"position_x":   m.PositionX,
						"position_y":   m.PositionY,
						"primary":      m.Primary,
						"enabled":      m.Enabled,
					}
				}
				layout["monitors"] = monitors
			}
			layouts[i] = layout
		}
		w.Set("client.layouts", layouts)
	}

	if err := w.WriteConfigAs(path); err != nil {
		return errors.Wrap(err, "failed to write config file")
	}
	return nil
}

// SaveServer writes server configuration to a file (server section only)
func SaveServer(cfg *ServerConfig, path string) error {
	if path == "" {
		path = DefaultConfigPath()
	}
	if err := ensureConfigDir(path); err != nil {
		return err
	}

	w := viper.New()
	w.SetConfigType("toml")

	w.Set("server.listen_addr", cfg.ListenAddr)
	w.Set("server.auth_token", cfg.AuthToken)
	if cfg.Username != "" {
		w.Set("server.username", cfg.Username)
	}
	if cfg.PasswordHash != "" {
		w.Set("server.password_hash", cfg.PasswordHash)
	}
	w.Set("server.client_addr", cfg.ClientAddr)
	if cfg.PingURL != "" {
		w.Set("server.ping_url", cfg.PingURL)
	}
	w.Set("server.ping_interval", cfg.PingInterval.String())
	w.Set("server.device_id", cfg.DeviceID)

	if len(cfg.WakeTargets) > 0 {
		targets := make([]map[string]interface{}, len(cfg.WakeTargets))
		for i, t := range cfg.WakeTargets {
			target := map[string]interface{}{
				"name":        t.Name,
				"mac_address": t.MACAddress,
			}
			if t.IPAddress != "" {
				target["ip_address"] = t.IPAddress
			}
			if t.Port != 0 {
				target["port"] = t.Port
			}
			targets[i] = target
		}
		w.Set("server.wake_targets", targets)
	}

	if err := w.WriteConfigAs(path); err != nil {
		return errors.Wrap(err, "failed to write config file")
	}
	return nil
}

// Save writes both server and client configuration (deprecated, use SaveClient/SaveServer)
func Save(cfg *Config, path string) error {
	if path == "" {
		path = DefaultConfigPath()
	}
	if err := ensureConfigDir(path); err != nil {
		return err
	}

	w := viper.New()
	w.SetConfigType("toml")

	// Server values
	w.Set("server.listen_addr", cfg.Server.ListenAddr)
	w.Set("server.auth_token", cfg.Server.AuthToken)
	if cfg.Server.Username != "" {
		w.Set("server.username", cfg.Server.Username)
	}
	if cfg.Server.PasswordHash != "" {
		w.Set("server.password_hash", cfg.Server.PasswordHash)
	}
	w.Set("server.client_addr", cfg.Server.ClientAddr)
	if cfg.Server.PingURL != "" {
		w.Set("server.ping_url", cfg.Server.PingURL)
	}
	w.Set("server.ping_interval", cfg.Server.PingInterval.String())
	w.Set("server.device_id", cfg.Server.DeviceID)

	if len(cfg.Server.WakeTargets) > 0 {
		targets := make([]map[string]interface{}, len(cfg.Server.WakeTargets))
		for i, t := range cfg.Server.WakeTargets {
			target := map[string]interface{}{
				"name":        t.Name,
				"mac_address": t.MACAddress,
			}
			if t.IPAddress != "" {
				target["ip_address"] = t.IPAddress
			}
			if t.Port != 0 {
				target["port"] = t.Port
			}
			targets[i] = target
		}
		w.Set("server.wake_targets", targets)
	}

	// Client values
	w.Set("client.listen_addr", cfg.Client.ListenAddr)
	w.Set("client.auth_token", cfg.Client.AuthToken)

	if len(cfg.Client.Layouts) > 0 {
		layouts := make([]map[string]interface{}, len(cfg.Client.Layouts))
		for i, l := range cfg.Client.Layouts {
			layout := map[string]interface{}{
				"id":   l.ID,
				"name": l.Name,
			}
			if l.Emoji != "" {
				layout["emoji"] = l.Emoji
			}
			if len(l.Aliases) > 0 {
				layout["aliases"] = l.Aliases
			}
			if len(l.Monitors) > 0 {
				monitors := make([]map[string]interface{}, len(l.Monitors))
				for j, m := range l.Monitors {
					monitors[j] = map[string]interface{}{
						"edid":         m.EDID,
						"port":         m.Port,
						"width":        m.Width,
						"height":       m.Height,
						"refresh_rate": m.RefreshRate,
						"position_x":   m.PositionX,
						"position_y":   m.PositionY,
						"primary":      m.Primary,
						"enabled":      m.Enabled,
					}
				}
				layout["monitors"] = monitors
			}
			layouts[i] = layout
		}
		w.Set("client.layouts", layouts)
	}

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
		return errors.New("server.auth_token is required (run 'ottoman config init server' to configure)")
	}

	return nil
}

// ValidateClient checks that client configuration is valid
func (c *ClientConfig) Validate() error {
	if c.ListenAddr == "" {
		return errors.New("client.listen_addr is required")
	}

	if c.AuthToken == "" {
		return errors.New("client.auth_token is required (run 'ottoman config init client' to configure)")
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

	if len(cfg.Client.Layouts) > 0 {
		fmt.Println()
		for _, layout := range cfg.Client.Layouts {
			fmt.Println("[[client.layouts]]")
			fmt.Printf("id = %q\n", layout.ID)
			fmt.Printf("name = %q\n", layout.Name)
			if layout.Emoji != "" {
				fmt.Printf("emoji = %q\n", layout.Emoji)
			}
			if len(layout.Aliases) > 0 {
				fmt.Printf("aliases = %v\n", layout.Aliases)
			}
			fmt.Printf("# %d monitors\n", len(layout.Monitors))
		}
	}
}

// PrintPaths outputs the config search paths
func PrintPaths() {
	fmt.Println("Config search paths:")
	fmt.Println("  1. ./config.toml")

	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			fmt.Printf("  2. %s\n", filepath.Join(appData, "ottoman", "config.toml"))
		}
	} else {
		fmt.Println("  2. /etc/ottoman/config.toml")
		if home := os.Getenv("HOME"); home != "" {
			fmt.Printf("  3. %s\n", filepath.Join(home, ".config", "ottoman", "config.toml"))
		}
	}

	fmt.Println()
	fmt.Printf("Default config path: %s\n", DefaultConfigPath())
}
