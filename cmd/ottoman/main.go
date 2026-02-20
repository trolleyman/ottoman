package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/trolleyman/ottoman/internal/agent"
	"github.com/trolleyman/ottoman/internal/api"
	"github.com/trolleyman/ottoman/internal/common"
	"github.com/trolleyman/ottoman/internal/config"
	"github.com/trolleyman/ottoman/internal/controller"
	"github.com/trolleyman/ottoman/internal/display"
	"github.com/trolleyman/ottoman/internal/input"
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
	input.InitPlatform()
	setupLogging()
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

// Controller commands
var controllerCmd = &cobra.Command{
	Use:   "controller",
	Short: "Controller commands (runs on Raspberry Pi)",
}

var controllerRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the controller",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := controller.LoadConfig(configFile)
		if err != nil {
			return errors.Wrap(err, "failed to load config")
		}
		return controller.Run(cfg)
	},
}

var controllerSimulateCmd = &cobra.Command{
	Use:   "simulate",
	Short: "Run simulated controller with mock agent (for frontend testing)",
	Long: `Run a simulated controller that serves the real web frontend with mocked API
endpoints. The simulated agent starts offline — use Wake-on-LAN in the UI
to trigger a simulated boot sequence. Layouts and monitors are loaded from
a agent config file.

Admin endpoints (no auth):
  POST /api/sim/reset      Reset agent to offline
  GET  /api/sim/state      Get current simulated state
  POST /api/sim/set-state  Set state directly (offline/booting/online)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		agentConfigFile, _ := cmd.Flags().GetString("agent-config")
		bootDelay, _ := cmd.Flags().GetDuration("boot-delay")
		startOnline, _ := cmd.Flags().GetBool("start-online")

		// Load controller config
		controllerCfg, err := controller.LoadConfig(configFile)
		if err != nil {
			return errors.Wrap(err, "failed to load controller config")
		}

		// Load agent config for layout/monitor data
		if agentConfigFile == "" {
			return errors.New("--agent-config is required")
		}
		agentCfg, err := agent.LoadConfig(agentConfigFile)
		if err != nil {
			return errors.Wrap(err, "failed to load agent config")
		}

		return controller.RunSimulatedController(controllerCfg, agentCfg, bootDelay, startOnline)
	},
}

var controllerInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install systemd service for controller",
	RunE: func(cmd *cobra.Command, args []string) error {
		return controller.InstallService()
	},
}

var controllerUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall systemd service for controller",
	RunE: func(cmd *cobra.Command, args []string) error {
		return controller.UninstallService()
	},
}

// Agent commands
var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Agent commands (runs on desktop)",
}

var agentRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the agent",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := agent.LoadConfig(configFile)
		if err != nil {
			return errors.Wrap(err, "failed to load config")
		}
		return agent.Run(cfg)
	},
}

var agentInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install autostart service (systemd on Linux, startup script on Windows)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return agent.InstallService()
	},
}

var agentUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove autostart service",
	RunE: func(cmd *cobra.Command, args []string) error {
		return agent.UninstallService()
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
		return agent.Install()
	},
}

// Monitor commands
var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Monitor management commands",
}

var monitorListCmd = &cobra.Command{
	Use:   "list",
	Short: "List connected monitors with detailed info",
	RunE: func(cmd *cobra.Command, args []string) error {
		layouts := display.NewLayouts()
		mgr, err := display.NewManager(layouts)
		if err != nil {
			return errors.Wrap(err, "failed to create display manager")
		}

		monitors, err := mgr.ListMonitors()
		if err != nil {
			return errors.Wrap(err, "failed to list monitors")
		}

		if len(monitors) == 0 {
			log.Println("No monitors detected")
			return nil
		}

		for _, m := range monitors {
			status := "inactive"
			if m.Active != nil {
				status = "active"
			}
			primary := ""
			if m.Active != nil && m.Active.Primary {
				primary = " [PRIMARY]"
			}
			log.Printf("%s (%s) - %s%s\n", m.Edid, m.Name, status, primary)
			log.Printf("  Port:       %s\n", m.Port)
			if m.Active != nil {
				log.Printf("  Resolution: %dx%d @ %.0fHz\n", m.Active.Width, m.Active.Height, m.Active.RefreshRate)
				log.Printf("  Position:   (%d, %d)\n", m.Active.PositionX, m.Active.PositionY)
				if m.Active.Model != "" {
					log.Printf("  Model:      %s\n", m.Active.Model)
				}
			}
		}
		return nil
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

		layouts := display.NewLayoutsFromSlice(fullCfg.Agent.Layouts)

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
		var monitorConfigs []api.LayoutMonitor
		for _, m := range monitors {
			if m.Active != nil {
				monitorConfigs = append(monitorConfigs, api.LayoutMonitor{
					Edid:        m.Edid,
					Name:        m.Name,
					Port:        m.Port,
					Width:       m.Active.Width,
					Height:      m.Active.Height,
					RefreshRate: m.Active.RefreshRate,
					PositionX:   m.Active.PositionX,
					PositionY:   m.Active.PositionY,
					Primary:     m.Active.Primary,
				})
			}
		}

		name := args[0]
		layout := api.Layout{
			Id:       slugify(name),
			Name:     name,
			Aliases:  []string{},
			Monitors: monitorConfigs,
		}
		if len(args) > 1 {
			emoji := args[1]
			layout.Emoji = &emoji
		}

		layouts.Set(layout)
		fullCfg.Agent.Layouts = layouts.ToSlice()
		if err := config.SaveAgent(&fullCfg.Agent, config.ConfigPath()); err != nil {
			return errors.Wrap(err, "failed to save config")
		}

		log.Printf("Added layout %q (%s)\n", layout.Name, layout.Id)
		for _, m := range monitorConfigs {
			primary := ""
			if m.Primary {
				primary = " [PRIMARY]"
			}
			log.Printf("  - %q EDID=%q Port=%q (%vx%v @ %.0fHz) @ %v,%v%s\n", m.Name, m.Edid, m.Port, m.Width, m.Height, m.RefreshRate, m.PositionX, m.PositionY, primary)
		}
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

		if len(cfg.Agent.Layouts) == 0 {
			log.Println("No layouts configured")
			return nil
		}

		for _, l := range cfg.Agent.Layouts {
			emoji := ""
			if l.Emoji != nil && *l.Emoji != "" {
				emoji = *l.Emoji + " "
			}
			aliases := ""
			if len(l.Aliases) > 0 {
				aliases = fmt.Sprintf(" (aliases: %v)", l.Aliases)
			}
			log.Printf("%s%s [%s]%s - %d monitors\n", emoji, l.Name, l.Id, aliases, len(l.Monitors))
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
			log.Println("No monitors detected")
			return nil
		}

		log.Println("Current display configuration:")
		for _, m := range monitors {
			if m.Active == nil {
				continue
			}
			primary := ""
			if m.Active.Primary {
				primary = " [PRIMARY]"
			}
			log.Printf("  %s (%s)%s\n", m.Edid, m.Name, primary)
			log.Printf("    Resolution: %dx%d @ %.0fHz\n", m.Active.Width, m.Active.Height, m.Active.RefreshRate)
			log.Printf("    Position:   (%d, %d)\n", m.Active.PositionX, m.Active.PositionY)
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

		layouts := display.NewLayoutsFromSlice(fullCfg.Agent.Layouts)

		if !layouts.AddAlias(args[0], args[1]) {
			return fmt.Errorf("layout %q not found", args[0])
		}

		fullCfg.Agent.Layouts = layouts.ToSlice()
		if err := config.SaveAgent(&fullCfg.Agent, config.ConfigPath()); err != nil {
			return errors.Wrap(err, "failed to save config")
		}

		log.Printf("Added alias %q to layout %q\n", args[1], args[0])
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

		layouts := display.NewLayoutsFromSlice(fullCfg.Agent.Layouts)

		if !layouts.RemoveAlias(args[0], args[1]) {
			return fmt.Errorf("layout %q not found or alias %q doesn't exist", args[0], args[1])
		}

		fullCfg.Agent.Layouts = layouts.ToSlice()
		if err := config.SaveAgent(&fullCfg.Agent, config.ConfigPath()); err != nil {
			return errors.Wrap(err, "failed to save config")
		}

		log.Printf("Removed alias %q from layout %q\n", args[1], args[0])
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

		layouts := display.NewLayoutsFromSlice(cfg.Agent.Layouts)

		matches := layouts.FindByIDOrAlias(args[0])
		if len(matches) == 0 {
			return fmt.Errorf("no layout found matching %q", args[0])
		}
		if len(matches) > 1 {
			log.Printf("Multiple layouts match %q:\n", args[0])
			for _, l := range matches {
				log.Printf("  - %s [%s]\n", l.Name, l.Id)
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

		log.Printf("Applied layout %q (%s)\n", layout.Name, layout.Id)
		return nil
	},
}

// Status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check if both controller and agent are running and reachable",
	RunE: func(cmd *cobra.Command, args []string) error {
		controllerAddr, _ := cmd.Flags().GetString("controller")
		agentAddr, _ := cmd.Flags().GetString("agent")

		log.Println("Checking ottoman status...")
		log.Println()

		controllerStatus := controller.CheckStatus(controllerAddr)
		agentStatus := agent.CheckStatus(agentAddr)

		log.Printf("Controller (%s): %s\n", controllerAddr, controllerStatus)
		log.Printf("Agent      (%s): %s\n", agentAddr, agentStatus)

		if controllerStatus != "OK" || agentStatus != "OK" {
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
	Use:   "init <agent|controller>",
	Short: "Create a configuration file for agent or controller",
	Long: `Create a configuration file with required settings.
If the file already exists, it will be displayed and you will be asked
whether to keep it or reconfigure.

Examples:
  ottoman config init agent                             # Initialize agent configuration
  ottoman config init controller                        # Initialize controller configuration
  ottoman config init controller --output config.toml   # Write to specific path`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mode := args[0]
		if mode != "agent" && mode != "controller" {
			return fmt.Errorf("invalid mode %q: must be 'agent' or 'controller'", mode)
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
			log.Printf("=== Existing config (%s) ===\n", path)
			log.Println(string(content))
			log.Println("===========================")

			answer, err := promptInput(reader, "Use this configuration? [Y/n]", "")
			if err != nil {
				return err
			}
			if answer == "" || strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes") {
				log.Println("Keeping existing configuration.")
				return nil
			}

			// Load existing values as defaults
			config.Init(path)
		} else {
			config.Init("")
		}

		if mode == "agent" {
			cfg, err := initAgentConfig(reader)
			if err != nil {
				return err
			}
			if err := config.SaveAgent(cfg, path); err != nil {
				return errors.Wrap(err, "failed to save config")
			}
		} else { // controller
			cfg, err := initControllerConfig(reader)
			if err != nil {
				return err
			}
			if err := config.SaveController(cfg, path); err != nil {
				return errors.Wrap(err, "failed to save config")
			}
		}

		log.Printf("\nCreated config file: %s\n", path)
		return nil
	},
}

// promptInput asks for user input with an optional default value
func promptInput(reader *bufio.Reader, question, defaultVal string) (string, error) {
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", question, defaultVal)
	} else {
		fmt.Printf("%s: ", question)
	}
	answer, err := reader.ReadString('\n')
	if err != nil {
		return "", errors.Wrap(err, "failed to read input")
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return defaultVal, nil
	}
	return answer, nil
}

// promptInteger asks for user input with an optional default value
func promptInteger(reader *bufio.Reader, question string, defaultVal *int) (int, error) {
	defaultString := ""
	if defaultVal != nil {
		defaultString = fmt.Sprintf("%d", *defaultVal)
	}
	answer, err := promptInput(reader, question, defaultString)
	if err != nil {
		return 0, err
	}
	if answer == "" {
		return *defaultVal, nil
	}
	val, err := strconv.Atoi(answer)
	if err != nil {
		return 0, errors.Wrapf(err, "invalid integer: %q", answer)
	}
	return val, nil
}

// promptToken asks for an auth token, generating one if left blank
func promptToken(reader *bufio.Reader, label, defaultVal string) (string, error) {
	if defaultVal != "" {
		token, err := promptInput(reader, label, defaultVal)
		return token, err
	}
	token, err := promptInput(reader, label+" (leave blank to generate)", "")
	if err != nil {
		return "", err
	}
	if token == "" {
		generated, err := config.GenerateToken()
		if err != nil {
			return "", errors.Wrap(err, "failed to generate token")
		}
		log.Printf("Generated token: %s\n", generated)
		return generated, nil
	}
	return token, nil
}

// initAgentConfig interactively creates an agent config
func initAgentConfig(reader *bufio.Reader) (*config.AgentConfig, error) {
	// Try to load existing values
	existing, _ := config.Load()
	cfg := &config.AgentConfig{
		ListenAddress: ":17294",
	}
	if existing != nil {
		cfg = &existing.Agent
	}

	var err error
	cfg.AuthToken, err = promptToken(reader, "Auth token", cfg.AuthToken)
	if err != nil {
		return nil, err
	}
	cfg.ListenAddress, err = promptInput(reader, "Listen address", cfg.ListenAddress)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

// initControllerConfig interactively creates a controller config
func initControllerConfig(reader *bufio.Reader) (*config.ControllerConfig, error) {
	// Try to load existing values
	existing, _ := config.Load()
	cfg := &config.ControllerConfig{
		ListenAddress: ":17293",
		Agent:         config.AgentControllerConfig{IPAddress: "127.0.0.1", Port: 17294},
	}
	if existing != nil {
		cfg = &existing.Controller
	}

	// Smart defaults from local network
	localIP := getLocalIP()
	if localIP != "" && cfg.Agent.IPAddress == "127.0.0.1" {
		cfg.Agent.IPAddress = localIP
	}

	var err error
	cfg.AuthToken, err = promptToken(reader, "Auth token", cfg.AuthToken)
	if err != nil {
		return nil, err
	}
	cfg.ListenAddress, err = promptInput(reader, "Listen address", cfg.ListenAddress)
	if err != nil {
		return nil, err
	}
	cfg.Agent.IPAddress, err = promptInput(reader, "Agent IP address", cfg.Agent.IPAddress)
	if err != nil {
		return nil, err
	}
	cfg.Agent.Port, err = promptInteger(reader, "Agent port", &cfg.Agent.Port)
	if err != nil {
		return nil, err
	}

	if cfg.Agent.MACAddress == "" {
		localMAC, err := getLocalMAC()
		if err != nil {
			return nil, errors.Wrap(err, "failed to get local MAC address")
		}
		if localMAC != "" {
			cfg.Agent.MACAddress = localMAC
		}
	}
	cfg.Agent.MACAddress, err = promptInput(reader, "Agent MAC address", cfg.Agent.MACAddress)
	if err != nil {
		return nil, err
	}

	return cfg, nil
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

// getLocalMAC returns the MAC address of the primary network interface. "" if no interface was found with a MAC address
func getLocalMAC() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", errors.Wrap(err, "failed to get local MAC address")
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
			return iface.HardwareAddr.String(), nil
		}
	}
	return "", nil
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "config file path")

	// Controller commands
	controllerCmd.AddCommand(controllerRunCmd)
	controllerSimulateCmd.Flags().String("agent-config", "", "path to agent config for layout/monitor data (required)")
	controllerSimulateCmd.Flags().Duration("boot-delay", 1*time.Second, "simulated boot delay after WoL")
	controllerSimulateCmd.Flags().Bool("start-online", false, "start with simulated agent already online")
	controllerCmd.AddCommand(controllerSimulateCmd)
	controllerCmd.AddCommand(controllerInstallCmd)
	controllerCmd.AddCommand(controllerUninstallCmd)

	// Agent commands
	agentCmd.AddCommand(agentRunCmd)
	agentCmd.AddCommand(layoutCmd)
	agentCmd.AddCommand(monitorCmd)
	agentCmd.AddCommand(agentInstallCmd)
	agentCmd.AddCommand(agentUninstallCmd)

	// Monitor commands
	monitorCmd.AddCommand(monitorListCmd)

	// Layout commands
	layoutCmd.AddCommand(layoutAddCmd)
	layoutCmd.AddCommand(layoutListCmd)
	layoutCmd.AddCommand(layoutShowCmd)
	layoutCmd.AddCommand(layoutApplyCmd)
	layoutCmd.AddCommand(layoutAliasCmd)
	layoutAliasCmd.AddCommand(layoutAliasAddCmd)
	layoutAliasCmd.AddCommand(layoutAliasRemoveCmd)

	// Status command
	statusCmd.Flags().String("controller", "localhost:17293", "Controller address")
	statusCmd.Flags().String("agent", "localhost:17294", "Agent address")

	// Config commands
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configPathsCmd)
	configCmd.AddCommand(configInitCmd)
	configInitCmd.Flags().StringP("output", "o", "", "output path for config file")

	// Add commands to root
	rootCmd.AddCommand(controllerCmd)
	rootCmd.AddCommand(agentCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(installCmd)
}

func setupLogging() {
	var logDir string
	home, err := os.UserHomeDir()
	if err != nil {
		return // Can't find home dir, skip file logging
	}

	// Determine log directory based on OS
	if os.Getenv("OS") == "Windows_NT" {
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			localAppData = filepath.Join(home, "AppData", "Local")
		}
		logDir = filepath.Join(localAppData, "ottoman", "logs")
	} else {
		logDir = filepath.Join(home, ".local", "share", "ottoman", "logs")
	}

	logPath := filepath.Join(logDir, "ottoman.log")
	rl, err := common.NewRotatingLogger(logPath, 5*1024*1024, 5) // 5MB, 5 backups
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup file logging: %v\n", err)
		return
	}

	// Log to both stderr and file
	log.SetOutput(io.MultiWriter(os.Stderr, rl))
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
}
