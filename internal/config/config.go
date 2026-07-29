package config

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
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

	// RequireLocalAuth withdraws the loopback exemption, so a browser on this
	// machine has to present the token too. Turn it on when a TLS front-end on
	// this host forwards outside traffic in - `tailscale serve` and reverse
	// proxies dial us from 127.0.0.1, so with the default exemption every
	// proxied request looks local and is waved straight through.
	RequireLocalAuth bool `json:"require_local_auth,omitempty"`
}

// AgentControllerConfig holds the configuration for how to contact the agent.
type AgentControllerConfig struct {
	MACAddress string `json:"mac_address"`

	// URL is the agent's base URL, scheme included. The scheme decides how the
	// controller talks to the agent: "https://host:port" when a TLS front-end
	// (tailscale serve, a reverse proxy) fronts it, "http://host:port" for a
	// direct connection over an already-trusted link such as a tailnet. It also
	// decides the trackpad WebSocket scheme (wss vs ws).
	URL string `json:"url,omitempty"`

	// IPAddress and Port are the pre-URL spelling. They are kept so existing
	// configs keep working: when URL is empty they are folded into
	// "http://ip:port". Prefer URL in new configs.
	IPAddress string `json:"ip_address,omitempty"`
	Port      int    `json:"port,omitempty"`
}

// AgentConfig holds agent configuration
type AgentConfig struct {
	ListenAddress string         `json:"listen_address"`
	AuthToken     string         `json:"auth_token"`
	Layouts       []api.Layout   `json:"layouts"`
	Trackpad      TrackpadConfig `json:"trackpad"`
	Boot          BootConfig     `json:"boot"`

	// RequireLocalAuth withdraws the loopback exemption - see the field of the
	// same name on ControllerConfig.
	RequireLocalAuth bool `json:"require_local_auth,omitempty"`
}

// DefaultAgentPort is the agent's listen port, and the port assumed when a
// legacy ip_address is given without one.
const DefaultAgentPort = 17294

// defaultAgentURL is where the controller looks for the agent when nothing is
// configured at all.
const defaultAgentURL = "http://127.0.0.1:17294"

// resolvedURL returns the agent base URL, folding in the legacy ip_address /
// port pair when url is unset. It is deliberately total - callers get a usable
// string whether or not Normalize has run - so there is no ordering dependency
// between loading, validating and using the config.
func (a *AgentControllerConfig) resolvedURL() string {
	if a.URL != "" {
		return a.URL
	}
	if a.IPAddress != "" {
		port := a.Port
		if port == 0 {
			port = DefaultAgentPort
		}
		return "http://" + net.JoinHostPort(a.IPAddress, strconv.Itoa(port))
	}
	return defaultAgentURL
}

// BaseURL parses the agent's base URL, rejecting anything the controller can't
// actually dial.
func (a *AgentControllerConfig) BaseURL() (*url.URL, error) {
	raw := a.resolvedURL()
	u, err := url.Parse(raw)
	if err != nil {
		return nil, errors.Wrapf(err, "controller.agent.url %q is not a valid URL", raw)
	}
	switch u.Scheme {
	case "http", "https":
	default:
		return nil, errors.Errorf("controller.agent.url %q must use the http or https scheme", raw)
	}
	if u.Host == "" {
		return nil, errors.Errorf("controller.agent.url %q is missing a host", raw)
	}
	return u, nil
}

// Normalize collapses the legacy spelling into URL so a config written back out
// carries only the modern key.
func (a *AgentControllerConfig) Normalize() {
	a.URL = a.resolvedURL()
	a.IPAddress = ""
	a.Port = 0
}

// BootConfig holds GRUB dual-boot entry names for remote OS selection. The GRUB
// default should be the Linux entry (GRUB_DEFAULT=saved); "boot into Windows"
// uses grub-reboot for a one-shot next boot.
type BootConfig struct {
	LinuxEntry   string `json:"linux_entry"`   // GRUB menuentry name for Linux
	WindowsEntry string `json:"windows_entry"` // GRUB menuentry name for Windows
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
	// controller.agent.url is deliberately not defaulted: an unset legacy
	// ip_address has to stay empty so resolvedURL can tell "nothing configured"
	// (use the default URL) from "configured the old way" (fold it into one).
	v.SetDefault("controller.agent.url", "")

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
	if cfg.RequireLocalAuth {
		w.Set("agent.require_local_auth", true)
	}

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
	if cfg.RequireLocalAuth {
		w.Set("controller.require_local_auth", true)
	}
	w.Set("controller.agent.mac_address", cfg.Agent.MACAddress)
	// Write the modern key only: a config round-tripped through `config init`
	// comes back with url and no ip_address/port to disagree with it.
	w.Set("controller.agent.url", cfg.Agent.resolvedURL())
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

	if _, err := c.Agent.BaseURL(); err != nil {
		return err
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
