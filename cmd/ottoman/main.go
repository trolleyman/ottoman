package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"

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
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		return nil
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

var serverDeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy server to Pi target",
	RunE: func(cmd *cobra.Command, args []string) error {
		target, _ := cmd.Flags().GetString("target")
		return server.Deploy(target)
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

var clientDeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy client locally (alias for 'ottoman install')",
	RunE: func(cmd *cobra.Command, args []string) error {
		return client.Deploy()
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

// Deploy command (both)
var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy both server and client",
	RunE: func(cmd *cobra.Command, args []string) error {
		serverTarget, _ := cmd.Flags().GetString("server-target")
		if err := server.Deploy(serverTarget); err != nil {
			return errors.Wrap(err, "server deploy failed")
		}
		if err := client.Deploy(); err != nil {
			return errors.Wrap(err, "client deploy failed")
		}
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

Examples:
  ottoman config init client    # Initialize client configuration
  ottoman config init server    # Initialize server configuration`,
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

		// Check if file already exists
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("config file already exists: %s\nEdit it directly or delete it first", path)
		}

		// Initialize defaults
		config.Init("")

		reader := bufio.NewReader(os.Stdin)

		if mode == "client" {
			cfg := &config.ClientConfig{
				ListenAddr: ":8081",
			}

			// Prompt for auth token
			fmt.Print("Auth token (leave blank to generate): ")
			token, _ := reader.ReadString('\n')
			token = strings.TrimSpace(token)

			if token == "" {
				generated, err := config.GenerateToken()
				if err != nil {
					return errors.Wrap(err, "failed to generate token")
				}
				token = generated
				fmt.Printf("Generated token: %s\n", token)
			}
			cfg.AuthToken = token

			// Prompt for listen address
			fmt.Print("Listen address [:8081]: ")
			addr, _ := reader.ReadString('\n')
			addr = strings.TrimSpace(addr)
			if addr != "" {
				cfg.ListenAddr = addr
			}

			if err := config.SaveClient(cfg, path); err != nil {
				return errors.Wrap(err, "failed to save config")
			}

		} else { // server
			cfg := &config.ServerConfig{
				ListenAddr: ":8080",
				ClientAddr: "localhost:8081",
				DeviceID:   "ottoman",
			}

			// Prompt for auth token
			fmt.Print("Auth token (leave blank to generate): ")
			token, _ := reader.ReadString('\n')
			token = strings.TrimSpace(token)

			if token == "" {
				generated, err := config.GenerateToken()
				if err != nil {
					return errors.Wrap(err, "failed to generate token")
				}
				token = generated
				fmt.Printf("Generated token: %s\n", token)
			}
			cfg.AuthToken = token

			// Prompt for client address
			fmt.Print("Client address [localhost:8081]: ")
			clientAddr, _ := reader.ReadString('\n')
			clientAddr = strings.TrimSpace(clientAddr)
			if clientAddr != "" {
				cfg.ClientAddr = clientAddr
			}

			// Prompt for listen address
			fmt.Print("Listen address [:8080]: ")
			addr, _ := reader.ReadString('\n')
			addr = strings.TrimSpace(addr)
			if addr != "" {
				cfg.ListenAddr = addr
			}

			if err := config.SaveServer(cfg, path); err != nil {
				return errors.Wrap(err, "failed to save config")
			}
		}

		fmt.Printf("\nCreated config file: %s\n", path)
		return nil
	},
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "config file path")

	// Server commands
	serverCmd.AddCommand(serverRunCmd)
	serverCmd.AddCommand(serverInstallCmd)
	serverCmd.AddCommand(serverDeployCmd)
	serverDeployCmd.Flags().StringP("target", "t", "", "SSH target (user@host)")

	// Client commands
	clientCmd.AddCommand(clientRunCmd)
	clientCmd.AddCommand(clientDeployCmd)
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

	// Deploy command
	deployCmd.Flags().String("server-target", "", "SSH target for server deployment")

	// Status command
	statusCmd.Flags().String("server", "localhost:8080", "Server address")
	statusCmd.Flags().String("client", "localhost:8081", "Client address")

	// Config commands
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configPathsCmd)
	configCmd.AddCommand(configInitCmd)
	configInitCmd.Flags().StringP("output", "o", "", "output path for config file")

	// Add commands to root
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(clientCmd)
	rootCmd.AddCommand(deployCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(installCmd)
}
