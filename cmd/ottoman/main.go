package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/trolleyman/ottoman/internal/client"
	"github.com/trolleyman/ottoman/internal/common"
	"github.com/trolleyman/ottoman/internal/config"
	"github.com/trolleyman/ottoman/internal/display"
	"github.com/trolleyman/ottoman/internal/server"
)

// slugify converts a string into a URL-friendly slug
func slugify(input string) string {
	slug := regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(strings.ToLower(input), "-")
	return strings.Trim(slug, "-")
}

var (
	// Version is set at build time
	Version = "dev"

	// Config file path
	configFile string
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "ottoman",
	Short: "Home automation system for desktop control",
	Long: `Ottoman is a home automation system for controlling a desktop computer
from a Raspberry Pi. It provides wake-on-LAN, display switching, and
remote management capabilities.`,
	Version: Version,
	// Silence usage after args validation passes (show usage for arg errors, not runtime errors)
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		cmd.SilenceUsage = true
	},
}

// Server commands
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Server commands (runs on Raspberry Pi)",
}

var serverRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the server",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := server.LoadConfig(configFile)
		if err != nil {
			return errors.Wrap(err, "failed to load config")
		}
		return server.Run(cfg)
	},
}

var serverInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install systemd service for server",
	RunE: func(cmd *cobra.Command, args []string) error {
		return server.InstallService()
	},
}

var serverUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall systemd service for server",
	RunE: func(cmd *cobra.Command, args []string) error {
		return server.UninstallService()
	},
}

// Client commands
var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "Client commands (runs on desktop)",
}

var clientRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the client agent",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := client.LoadConfig(configFile)
		if err != nil {
			return errors.Wrap(err, "failed to load config")
		}
		return client.Run(cfg)
	},
}

// Service commands
var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage the ottoman client service (autostart)",
}

var serviceInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install autostart service (systemd on Linux, startup script on Windows)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return client.InstallService()
	},
}

var serviceUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove autostart service",
	RunE: func(cmd *cobra.Command, args []string) error {
		return client.UninstallService()
	},
}

// Install command (root level)
var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install ottoman to system location and create config",
	Long: `Install copies the ottoman binary to the appropriate system location:
  - Windows: %LOCALAPPDATA%\ottoman\ottoman.exe
  - Linux:   ~/.local/bin/ottoman

It also creates a default configuration file if one doesn't exist.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return client.Install()
	},
}

// Layout commands
var layoutCmd = &cobra.Command{
	Use:   "layout",
	Short: "Manage display layouts",
}

var layoutAddCmd = &cobra.Command{
	Use:   "add <name> [emoji]",
	Short: "Add a new layout from current display configuration",
	Long:  `Add a new layout capturing the current display configuration. The ID is auto-generated from the name.`,
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		config.Init(configFile)
		fullCfg, err := config.Load()
		if err != nil {
			return errors.Wrap(err, "failed to load config")
		}

		layouts := display.NewLayoutsFromSlice(fullCfg.Client.Layouts)

		mgr, err := display.NewManager(layouts)
		if err != nil {
			return errors.Wrap(err, "failed to create display manager")
		}

		// Get current monitor state
		monitors, err := mgr.ListMonitors()
		if err != nil {
			return errors.Wrap(err, "failed to get monitors")
		}

		// Convert to layout monitors
		var monitorConfigs []common.Monitor
		for _, m := range monitors {
			if m.Connected {
				monitorConfigs = append(monitorConfigs, common.Monitor{
					EDID:        m.EDID,
					Width:       m.Width,
					Height:      m.Height,
					RefreshRate: m.RefreshRate,
					PositionX:   m.PositionX,
					PositionY:   m.PositionY,
					Primary:     m.Primary,
					Enabled:     true,
				})
			}
		}

		name := args[0]
		layout := common.Layout{
			ID:       slugify(name),
			Name:     name,
			Monitors: monitorConfigs,
		}
		if len(args) > 1 {
			layout.Emoji = args[1]
		}

		layouts.Set(layout)
		fullCfg.Client.Layouts = layouts.ToSlice()
		if err := config.SaveClient(&fullCfg.Client, config.ConfigPath()); err != nil {
			return errors.Wrap(err, "failed to save config")
		}

		fmt.Printf("Added layout %q (%s)\n", layout.Name, layout.ID)
		return nil
	},
}

var layoutListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all layouts",
	RunE: func(cmd *cobra.Command, args []string) error {
		config.Init(configFile)
		cfg, err := config.Load()
		if err != nil {
			return errors.Wrap(err, "failed to load config")
		}

		if len(cfg.Client.Layouts) == 0 {
			fmt.Println("No layouts configured")
			return nil
		}

		for _, l := range cfg.Client.Layouts {
			emoji := ""
			if l.Emoji != "" {
				emoji = l.Emoji + " "
			}
			aliases := ""
			if len(l.Aliases) > 0 {
				aliases = fmt.Sprintf(" (aliases: %v)", l.Aliases)
			}
			fmt.Printf("%s%s [%s]%s - %d monitors\n", emoji, l.Name, l.ID, aliases, len(l.Monitors))
		}
		return nil
	},
}

var layoutShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current display configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		layouts := display.NewLayouts()
		mgr, err := display.NewManager(layouts)
		if err != nil {
			return errors.Wrap(err, "failed to create display manager")
		}

		monitors, err := mgr.ListMonitors()
		if err != nil {
			return errors.Wrap(err, "failed to get monitors")
		}

		if len(monitors) == 0 {
			fmt.Println("No monitors detected")
			return nil
		}

		fmt.Println("Current display configuration:")
		for _, m := range monitors {
			if !m.Connected {
				continue
			}
			primary := ""
			if m.Primary {
				primary = " [PRIMARY]"
			}
			fmt.Printf("  %s (%s)%s\n", m.EDID, m.Name, primary)
			fmt.Printf("    Resolution: %dx%d @ %.0fHz\n", m.Width, m.Height, m.RefreshRate)
			fmt.Printf("    Position:   (%d, %d)\n", m.PositionX, m.PositionY)
		}
		return nil
	},
}

var layoutAliasCmd = &cobra.Command{
	Use:   "alias",
	Short: "Manage layout aliases",
}

var layoutAliasAddCmd = &cobra.Command{
	Use:   "add <id> <alias>",
	Short: "Add an alias to a layout",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		config.Init(configFile)
		fullCfg, err := config.Load()
		if err != nil {
			return errors.Wrap(err, "failed to load config")
		}

		layouts := display.NewLayoutsFromSlice(fullCfg.Client.Layouts)

		if !layouts.AddAlias(args[0], args[1]) {
			return fmt.Errorf("layout %q not found", args[0])
		}

		fullCfg.Client.Layouts = layouts.ToSlice()
		if err := config.SaveClient(&fullCfg.Client, config.ConfigPath()); err != nil {
			return errors.Wrap(err, "failed to save config")
		}

		fmt.Printf("Added alias %q to layout %q\n", args[1], args[0])
		return nil
	},
}

var layoutAliasRemoveCmd = &cobra.Command{
	Use:   "remove <id> <alias>",
	Short: "Remove an alias from a layout",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		config.Init(configFile)
		fullCfg, err := config.Load()
		if err != nil {
			return errors.Wrap(err, "failed to load config")
		}

		layouts := display.NewLayoutsFromSlice(fullCfg.Client.Layouts)

		if !layouts.RemoveAlias(args[0], args[1]) {
			return fmt.Errorf("layout %q not found or alias %q doesn't exist", args[0], args[1])
		}

		fullCfg.Client.Layouts = layouts.ToSlice()
		if err := config.SaveClient(&fullCfg.Client, config.ConfigPath()); err != nil {
			return errors.Wrap(err, "failed to save config")
		}

		fmt.Printf("Removed alias %q from layout %q\n", args[1], args[0])
		return nil
	},
}

var layoutApplyCmd = &cobra.Command{
	Use:   "apply <id-or-alias>",
	Short: "Apply a layout by ID, name, or alias",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		config.Init(configFile)
		cfg, err := config.Load()
		if err != nil {
			return errors.Wrap(err, "failed to load config")
		}

		layouts := display.NewLayoutsFromSlice(cfg.Client.Layouts)

		matches := layouts.FindByIDOrAlias(args[0])
		if len(matches) == 0 {
			return fmt.Errorf("no layout found matching %q", args[0])
		}
		if len(matches) > 1 {
			fmt.Printf("Multiple layouts match %q:\n", args[0])
			for _, l := range matches {
				fmt.Printf("  - %s [%s]\n", l.Name, l.ID)
			}
			return fmt.Errorf("ambiguous layout reference")
		}

		layout := matches[0]

		mgr, err := display.NewManager(layouts)
		if err != nil {
			return errors.Wrap(err, "failed to create display manager")
		}

		if err := mgr.ApplyLayoutConfig(layout); err != nil {
			return errors.Wrap(err, "failed to apply layout")
		}

		fmt.Printf("Applied layout %q (%s)\n", layout.Name, layout.ID)
		return nil
	},
}

// Status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check if both server and client are running and reachable",
	RunE: func(cmd *cobra.Command, args []string) error {
		serverAddr, _ := cmd.Flags().GetString("server")
		clientAddr, _ := cmd.Flags().GetString("client")

		fmt.Println("Checking ottoman status...")
		fmt.Println()

		serverStatus := server.CheckStatus(serverAddr)
		clientStatus := client.CheckStatus(clientAddr)

		fmt.Printf("Server (%s): %s\n", serverAddr, serverStatus)
		fmt.Printf("Client (%s): %s\n", clientAddr, clientStatus)

		if serverStatus != "OK" || clientStatus != "OK" {
			return errors.New("one or more components are not reachable")
		}
		return nil
	},
}

// Config commands
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration management commands",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration file contents",
	RunE: func(cmd *cobra.Command, args []string) error {
		config.Init(configFile)
		// Load to find the config path
		if _, err := config.Load(); err != nil {
			return errors.Wrap(err, "failed to load config")
		}
		return config.Print()
	},
}

var configPathsCmd = &cobra.Command{
	Use:   "paths",
	Short: "Show configuration file search paths",
	Run: func(cmd *cobra.Command, args []string) {
		config.PrintPaths()
	},
}

var configInitCmd = &cobra.Command{
	Use:   "init <client|server>",
	Short: "Create a configuration file for client or server",
	Long: `Create a configuration file with required settings.
If the file already exists, it will be displayed and you will be asked
whether to keep it or reconfigure.

Examples:
  ottoman config init client                        # Initialize client configuration
  ottoman config init server                        # Initialize server configuration
  ottoman config init server --output server.toml   # Write to specific path`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mode := args[0]
		if mode != "client" && mode != "server" {
			return fmt.Errorf("invalid mode %q: must be 'client' or 'server'", mode)
		}

		path, _ := cmd.Flags().GetString("output")
		if path == "" {
			path = config.DefaultConfigPath()
		}

		reader := bufio.NewReader(os.Stdin)

		// If file already exists, show it and ask whether to keep
		if _, err := os.Stat(path); err == nil {
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return errors.Wrap(readErr, "failed to read existing config")
			}
			fmt.Printf("=== Existing config (%s) ===\n", path)
			fmt.Println(string(content))
			fmt.Println("===========================")

			answer := promptInput(reader, "Use this configuration? [Y/n]", "")
			if answer == "" || strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes") {
				fmt.Println("Keeping existing configuration.")
				return nil
			}

			// Load existing values as defaults
			config.Init(path)
		} else {
			config.Init("")
		}

		if mode == "client" {
			cfg, err := initClientConfig(reader)
			if err != nil {
				return err
			}
			if err := config.SaveClient(cfg, path); err != nil {
				return errors.Wrap(err, "failed to save config")
			}
		} else {
			cfg, err := initServerConfig(reader)
			if err != nil {
				return err
			}
			if err := config.SaveServer(cfg, path); err != nil {
				return errors.Wrap(err, "failed to save config")
			}
		}

		fmt.Printf("\nCreated config file: %s\n", path)
		return nil
	},
}

// promptInput asks for user input with an optional default value
func promptInput(reader *bufio.Reader, question, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", question, defaultVal)
	} else {
		fmt.Printf("%s: ", question)
	}
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return defaultVal
	}
	return answer
}

// promptToken asks for an auth token, generating one if left blank
func promptToken(reader *bufio.Reader, label, defaultVal string) (string, error) {
	if defaultVal != "" {
		token := promptInput(reader, label, defaultVal)
		return token, nil
	}
	token := promptInput(reader, label+" (leave blank to generate)", "")
	if token == "" {
		generated, err := config.GenerateToken()
		if err != nil {
			return "", errors.Wrap(err, "failed to generate token")
		}
		fmt.Printf("Generated token: %s\n", generated)
		return generated, nil
	}
	return token, nil
}

// initClientConfig interactively creates a client config
func initClientConfig(reader *bufio.Reader) (*config.ClientConfig, error) {
	// Try to load existing values
	existing, _ := config.Load()
	cfg := &config.ClientConfig{
		ListenAddr: ":17294",
	}
	if existing != nil {
		cfg = &existing.Client
	}

	var err error
	cfg.AuthToken, err = promptToken(reader, "Auth token", cfg.AuthToken)
	if err != nil {
		return nil, err
	}
	cfg.ListenAddr = promptInput(reader, "Listen address", cfg.ListenAddr)

	return cfg, nil
}

// initServerConfig interactively creates a server config
func initServerConfig(reader *bufio.Reader) (*config.ServerConfig, error) {
	// Try to load existing values
	existing, _ := config.Load()
	cfg := &config.ServerConfig{
		ListenAddr: ":17293",
		ClientAddr: "localhost:17294",
		DeviceID:   "ottoman",
	}
	if existing != nil {
		cfg = &existing.Server
	}

	// Smart defaults from local network
	localIP := getLocalIP()
	if cfg.ClientAddr == "localhost:17294" && localIP != "" {
		cfg.ClientAddr = localIP + ":17294"
	}

	var err error
	cfg.AuthToken, err = promptToken(reader, "Auth token", cfg.AuthToken)
	if err != nil {
		return nil, err
	}
	cfg.ListenAddr = promptInput(reader, "Listen address", cfg.ListenAddr)
	cfg.ClientAddr = promptInput(reader, "Client address", cfg.ClientAddr)
	cfg.DeviceID = promptInput(reader, "Device ID", cfg.DeviceID)

	// Ping (optional)
	cfg.Ping.URL = promptInput(reader, "Ping URL (optional)", cfg.Ping.URL)
	if cfg.Ping.URL != "" {
		if cfg.Ping.Interval == 0 {
			cfg.Ping.Interval = 5 * time.Minute
		}
		intervalStr := promptInput(reader, "Ping interval", cfg.Ping.Interval.String())
		// Store as string and let Viper parse it on load; for saving, parse it here
		parsed, parseErr := parseDuration(intervalStr)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid ping interval %q: %w", intervalStr, parseErr)
		}
		cfg.Ping.Interval = parsed
		cfg.Ping.AuthToken = promptInput(reader, "Ping auth token (optional)", cfg.Ping.AuthToken)
	}

	// Wake target
	fmt.Println("\n--- Wake-on-LAN Target ---")
	if len(cfg.WakeTargets) == 0 {
		cfg.WakeTargets = []config.WakeTarget{{Name: "desktop"}}
	}
	wt := &cfg.WakeTargets[0]
	wt.Name = promptInput(reader, "Wake target name", wt.Name)

	localMAC := getLocalMAC()
	if wt.MACAddress == "" && localMAC != "" {
		wt.MACAddress = localMAC
	}
	wt.MACAddress = promptInput(reader, "MAC address", wt.MACAddress)

	if wt.IPAddress == "" && localIP != "" {
		wt.IPAddress = localIP
	}
	wt.IPAddress = promptInput(reader, "IP address", wt.IPAddress)

	return cfg, nil
}

// parseDuration parses a duration string like "5m", "1h30m", etc.
func parseDuration(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	return d, nil
}

// getLocalIP returns the local machine's IP address
func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}

// getLocalMAC returns the MAC address of the primary network interface
func getLocalMAC() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback != 0 || len(iface.HardwareAddr) == 0 {
			continue
		}
		name := strings.ToLower(iface.Name)
		if strings.HasPrefix(name, "veth") || strings.HasPrefix(name, "docker") ||
			strings.HasPrefix(name, "br-") || strings.HasPrefix(name, "virbr") {
			continue
		}
		if iface.Flags&net.FlagUp != 0 {
			return iface.HardwareAddr.String()
		}
	}
	return ""
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "config file path")

	// Server commands
	serverCmd.AddCommand(serverRunCmd)
	serverCmd.AddCommand(serverInstallCmd)
	serverCmd.AddCommand(serverUninstallCmd)

	// Client commands
	clientCmd.AddCommand(clientRunCmd)
	clientCmd.AddCommand(layoutCmd)
	clientCmd.AddCommand(serviceCmd)

	// Service commands
	serviceCmd.AddCommand(serviceInstallCmd)
	serviceCmd.AddCommand(serviceUninstallCmd)

	// Layout commands
	layoutCmd.AddCommand(layoutAddCmd)
	layoutCmd.AddCommand(layoutListCmd)
	layoutCmd.AddCommand(layoutShowCmd)
	layoutCmd.AddCommand(layoutApplyCmd)
	layoutCmd.AddCommand(layoutAliasCmd)
	layoutAliasCmd.AddCommand(layoutAliasAddCmd)
	layoutAliasCmd.AddCommand(layoutAliasRemoveCmd)

	// Status command
	statusCmd.Flags().String("server", "localhost:17293", "Server address")
	statusCmd.Flags().String("client", "localhost:17294", "Client address")

	// Config commands
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configPathsCmd)
	configCmd.AddCommand(configInitCmd)
	configInitCmd.Flags().StringP("output", "o", "", "output path for config file")

	// Add commands to root
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(clientCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(installCmd)
}
