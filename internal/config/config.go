package config

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"github.com/trolleyman/ottoman/internal/api"
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
	Controller ControllerConfig `json:"controller"`
	Agent      AgentConfig      `json:"agent"`
}

// ControllerConfig holds controller configuration
type ControllerConfig struct {
	ListenAddress string                `json:"listen_address"`
	AuthToken     string                `json:"auth_token"`
	Agent         AgentControllerConfig `json:"agent"`
}

// AgentControllerConfig holds the configuration for how to contact the agent
type AgentControllerConfig struct {
	MACAddress string `json:"mac_address"`
	IPAddress  string `json:"ip_address,omitempty"`
	Port       int    `json:"port,omitempty"`
}

// AgentConfig holds agent configuration
type AgentConfig struct {
	ListenAddress string         `json:"listen_address"`
	AuthToken     string         `json:"auth_token"`
	Layouts       []api.Layout   `json:"layouts"`
	Trackpad      TrackpadConfig `json:"trackpad"`
}

// TrackpadConfig holds trackpad configuration
type TrackpadConfig struct {
	Sensitivity float64 `json:"sensitivity"`
	Friction    float64 `json:"friction"`
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
	v.SetDefault("controller.listen_address", ":17293")
	v.SetDefault("controller.agent.ip_address", "127.0.0.1")
	v.SetDefault("controller.agent.port", "17294")

	v.SetDefault("agent.listen_address", ":17294")
	v.SetDefault("agent.layouts", []api.Layout{})
	v.SetDefault("agent.trackpad_sensitivity", 1.5)
	v.SetDefault("agent.trackpad_friction", 0.92)
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
	if err := v.Unmarshal(&cfg, func(c *mapstructure.DecoderConfig) {
		c.TagName = "json"
	}); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal config")
	}

	return &cfg, nil
}

// GetController returns just the controller configuration
func GetController() (*ControllerConfig, error) {
	cfg, err := Load()
	if err != nil {
		return nil, err
	}
	return &cfg.Controller, nil
}

// GetAgent returns just the agent configuration
func GetAgent() (*AgentConfig, error) {
	cfg, err := Load()
	if err != nil {
		return nil, err
	}
	return &cfg.Agent, nil
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

func setAgent(w *viper.Viper, cfg *AgentConfig) {
	w.Set("agent.listen_address", cfg.ListenAddress)
	w.Set("agent.auth_token", cfg.AuthToken)

	// Preserve trackpad tuning so re-running `config init` over an existing
	// config doesn't silently drop it.
	if cfg.Trackpad.Sensitivity != 0 {
		w.Set("agent.trackpad.sensitivity", cfg.Trackpad.Sensitivity)
	}
	if cfg.Trackpad.Friction != 0 {
		w.Set("agent.trackpad.friction", cfg.Trackpad.Friction)
	}

	if len(cfg.Layouts) > 0 {
		layouts := make([]map[string]any, len(cfg.Layouts))
		for i, l := range cfg.Layouts {
			layout := map[string]any{
				"id":   l.Id,
				"name": l.Name,
			}
			if l.Emoji != nil && *l.Emoji != "" {
				layout["emoji"] = *l.Emoji
			}
			if len(l.Aliases) > 0 {
				layout["aliases"] = l.Aliases
			}
			if len(l.Monitors) > 0 {
				monitors := make([]map[string]any, len(l.Monitors))
				for j, m := range l.Monitors {
					monitors[j] = map[string]any{
						"name":         m.Name,
						"edid":         m.Edid,
						"port":         m.Port,
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
		w.Set("agent.layouts", layouts)
	}
}

// SaveAgent writes agent configuration to a file
func SaveAgent(cfg *AgentConfig, path string) error {
	if path == "" {
		path = DefaultConfigPath()
	}
	if err := ensureConfigDir(path); err != nil {
		return err
	}

	w := viper.New()
	w.SetConfigType("toml")

	setAgent(w, cfg)

	if err := w.WriteConfigAs(path); err != nil {
		return errors.Wrap(err, "failed to write config file")
	}
	return nil
}

func setController(w *viper.Viper, cfg *ControllerConfig) {
	w.Set("controller.listen_address", cfg.ListenAddress)
	w.Set("controller.auth_token", cfg.AuthToken)
	w.Set("controller.agent.mac_address", cfg.Agent.MACAddress)
	w.Set("controller.agent.ip_address", cfg.Agent.IPAddress)
	w.Set("controller.agent.port", cfg.Agent.Port)
}

// SaveController writes controller configuration to a file
func SaveController(cfg *ControllerConfig, path string) error {
	if path == "" {
		path = DefaultConfigPath()
	}
	if err := ensureConfigDir(path); err != nil {
		return err
	}

	w := viper.New()
	w.SetConfigType("toml")

	setController(w, cfg)

	if err := w.WriteConfigAs(path); err != nil {
		return errors.Wrap(err, "failed to write config file")
	}
	return nil
}

// Save writes both controller and agent configuration
func Save(cfg *Config, path string) error {
	if path == "" {
		path = DefaultConfigPath()
	}
	if err := ensureConfigDir(path); err != nil {
		return err
	}

	w := viper.New()
	w.SetConfigType("toml")

	setAgent(w, &cfg.Agent)
	setController(w, &cfg.Controller)

	if err := w.WriteConfigAs(path); err != nil {
		return errors.Wrap(err, "failed to write config file")
	}
	return nil
}

// Validate checks that controller configuration is valid
func (c *ControllerConfig) Validate() error {
	if c.ListenAddress == "" {
		return errors.New("controller.listen_address is required")
	}

	if c.Agent.IPAddress == "" {
		return errors.New("controller.agent.ip_address is required")
	}
	if c.Agent.Port == 0 {
		return errors.New("controller.agent.port is required")
	}

	if c.AuthToken == "" {
		return errors.New("controller.auth_token is required (run 'ottoman config init controller' to configure)")
	}

	return nil
}

// Validate checks that agent configuration is valid
func (c *AgentConfig) Validate() error {
	if c.ListenAddress == "" {
		return errors.New("agent.listen_address is required")
	}

	if c.AuthToken == "" {
		return errors.New("agent.auth_token is required (run 'ottoman config init agent' to configure)")
	}

	return nil
}

// Print outputs the config file contents to stdout
func Print() error {
	if configPath == "" {
		log.Println("No config file found.")
		log.Println()
		log.Printf("Default path: %s\n", DefaultConfigPath())
		log.Println("Run 'ottoman config init controller' or 'ottoman config init agent' to create one.")
		return nil
	}

	log.Printf("# %s\n", configPath)
	log.Println()

	content, err := os.ReadFile(configPath)
	if err != nil {
		return errors.Wrap(err, "failed to read config file")
	}

	log.Print(string(content))
	return nil
}

// PrintPaths outputs the config search paths
func PrintPaths() {
	log.Println("Config search paths:")
	log.Println("  1. ./config.toml")

	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			log.Printf("  2. %s\n", filepath.Join(appData, "ottoman", "config.toml"))
		}
	} else {
		log.Println("  2. /etc/ottoman/config.toml")
		if home := os.Getenv("HOME"); home != "" {
			log.Printf("  3. %s\n", filepath.Join(home, ".config", "ottoman", "config.toml"))
		}
	}

	log.Println()
	log.Printf("Default config path: %s\n", DefaultConfigPath())
}
