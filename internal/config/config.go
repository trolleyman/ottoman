package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

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
	ListenAddr   string       `mapstructure:"listen_addr"`
	AuthToken    string       `mapstructure:"auth_token"`
	Username     string       `mapstructure:"username"`
	PasswordHash string       `mapstructure:"password_hash"`
	WakeTargets  []WakeTarget `mapstructure:"wake_targets"`
	ClientAddr   string       `mapstructure:"client_addr"`
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
	v.SetDefault("server.listen_addr", ":17293")
	v.SetDefault("server.client_addr", "localhost:17294")
	v.SetDefault("server.wake_targets", []WakeTarget{})

	// Client defaults
	v.SetDefault("client.listen_addr", ":17294")
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
						"width":        m.Width,
						"height":       m.Height,
						"refresh_rate": m.RefreshRate,
						"position_x":   m.PositionX,
						"position_y":   m.PositionY,
						"primary":      m.Primary,
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
						"width":        m.Width,
						"height":       m.Height,
						"refresh_rate": m.RefreshRate,
						"position_x":   m.PositionX,
						"position_y":   m.PositionY,
						"primary":      m.Primary,
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

// Print outputs the config file contents to stdout
func Print() error {
	if configPath == "" {
		fmt.Println("No config file found.")
		fmt.Println()
		fmt.Printf("Default path: %s\n", DefaultConfigPath())
		fmt.Println("Run 'ottoman config init client' or 'ottoman config init server' to create one.")
		return nil
	}

	fmt.Printf("# %s\n", configPath)
	fmt.Println()

	content, err := os.ReadFile(configPath)
	if err != nil {
		return errors.Wrap(err, "failed to read config file")
	}

	fmt.Print(string(content))
	return nil
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
