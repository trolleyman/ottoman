package main

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/trolleyman/ottoman/internal/client"
	"github.com/trolleyman/ottoman/internal/config"
	"github.com/trolleyman/ottoman/internal/server"
)

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

var clientInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install service for client (systemd on Linux, Windows Service on Windows)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return client.InstallService()
	},
}

var clientDeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy client locally",
	RunE: func(cmd *cobra.Command, args []string) error {
		return client.Deploy()
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
	Short: "Show current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		config.Init(configFile)
		cfg, err := config.Load()
		if err != nil {
			return errors.Wrap(err, "failed to load config")
		}
		config.Print(cfg)
		return nil
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
	Use:   "init",
	Short: "Create a default configuration file",
	RunE: func(cmd *cobra.Command, args []string) error {
		path, _ := cmd.Flags().GetString("output")
		if path == "" {
			path = config.DefaultConfigPath()
		}

		// Check if file already exists
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("config file already exists: %s", path)
		}

		// Load defaults and save
		config.Init("")
		cfg, err := config.Load()
		if err != nil {
			return errors.Wrap(err, "failed to load defaults")
		}

		if err := config.Save(cfg, path); err != nil {
			return errors.Wrap(err, "failed to save config")
		}

		fmt.Printf("Created config file: %s\n", path)
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
	clientCmd.AddCommand(clientInstallCmd)
	clientCmd.AddCommand(clientDeployCmd)

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
}
