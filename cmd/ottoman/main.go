package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
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
	"github.com/trolleyman/ottoman/internal/store"
)

// loadLayoutStore opens the data-dir layout store, migrating any legacy layouts
// still present in the config file on first use. Returns the store and a
// Layouts view over its contents so CLI commands stay consistent with the
// running agent.
func loadLayoutStore(cfg *config.Config) (*store.LayoutStore, *display.Layouts, error) {
	ls := store.NewLayoutStore("")
	loaded, err := ls.LoadWithMigration(cfg.Agent.Layouts)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to load layouts store")
	}
	return ls, display.NewLayoutsFromSlice(loaded), nil
}

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

var agentRunGreeter bool

var agentRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the agent",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := agent.LoadConfig(configFile)
		if err != nil {
			return errors.Wrap(err, "failed to load config")
		}
		if agentRunGreeter {
			return agent.RunGreeter(cfg)
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

var hostSetupUser string
var hostSetupGreeter bool

var agentHostSetupCmd = &cobra.Command{
	Use:   "host-setup",
	Short: "Grant one-time root host access (uinput, i2c, grub-reboot); self-elevates via sudo",
	RunE: func(cmd *cobra.Command, args []string) error {
		return agent.HostSetup(hostSetupUser, hostSetupGreeter)
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

		layoutStore, layouts, err := loadLayoutStore(fullCfg)
		if err != nil {
			return err
		}

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
		if err := layoutStore.Save(layouts.ToSlice()); err != nil {
			return errors.Wrap(err, "failed to save layouts")
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

		_, layouts, err := loadLayoutStore(cfg)
		if err != nil {
			return err
		}
		list := layouts.List()

		if len(list) == 0 {
			log.Println("No layouts configured")
			return nil
		}

		for _, l := range list {
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

		layoutStore, layouts, err := loadLayoutStore(fullCfg)
		if err != nil {
			return err
		}

		if !layouts.AddAlias(args[0], args[1]) {
			return fmt.Errorf("layout %q not found", args[0])
		}

		if err := layoutStore.Save(layouts.ToSlice()); err != nil {
			return errors.Wrap(err, "failed to save layouts")
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

		layoutStore, layouts, err := loadLayoutStore(fullCfg)
		if err != nil {
			return err
		}

		if !layouts.RemoveAlias(args[0], args[1]) {
			return fmt.Errorf("layout %q not found or alias %q doesn't exist", args[0], args[1])
		}

		if err := layoutStore.Save(layouts.ToSlice()); err != nil {
			return errors.Wrap(err, "failed to save layouts")
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

		_, layouts, err := loadLayoutStore(cfg)
		if err != nil {
			return err
		}

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

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a single configuration value non-interactively",
	Long: `Set one configuration value, validating it before it is written.

The value is checked against what the key actually accepts, so a mistake is
refused here rather than surfacing as a service that won't start. Only the
named key changes; everything else in the file is preserved (though comments
and key order are not - the file is rewritten from its parsed contents).

Writes the file named by --config, or the config file that would be loaded,
or the platform default path.

Examples:
  ottoman config set agent.listen_address 127.0.0.1:17294
  ottoman config set agent.require_local_auth true
  ottoman config set controller.agent.url https://hades.tail1234.ts.net
  ottoman config set -c /etc/ottoman/config.toml controller.listen_address :17293

Settable keys:
` + strings.Join(config.SettableKeys(), "\n"),
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		config.Init(configFile)
		// Locate the file that would be loaded; a missing one is fine, `set`
		// creates it.
		_, _ = config.Load()

		path, value, err := config.SetValue(configFile, args[0], args[1])
		if err != nil {
			return err
		}
		log.Printf("%s: %s = %v\n", path, args[0], value)
		mirrorConfigToGreeter(path)
		log.Println(restartHint(args[0]))
		return nil
	},
}

var configRotateTokenCmd = &cobra.Command{
	Use:   "rotate-token",
	Short: "Generate a new auth token and write it to every copy that must match",
	Long: `Rotate the shared auth token.

The agent and the controller must present the same token, and on this machine
it can live in up to three files: the config, the gdm greeter's copy of it (if
the login-screen agent is installed) and the controller's config on the Pi.
Editing them by hand is how they drift apart, so this generates the token once
and writes every copy it can reach.

Both auth_token keys already present in the local config file are updated;
--push additionally sets controller.auth_token on a remote host over SSH.

Examples:
  ottoman config rotate-token
  ottoman config rotate-token --token "$(cat token.txt)"
  ottoman config rotate-token --push pi@ottoman.home`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		token, _ := cmd.Flags().GetString("token")
		push, _ := cmd.Flags().GetString("push")
		remoteBinary, _ := cmd.Flags().GetString("remote-binary")
		remoteConfig, _ := cmd.Flags().GetString("remote-config")

		config.Init(configFile)
		if _, err := config.Load(); err != nil {
			return errors.Wrap(err, "failed to load config")
		}

		rotation, err := config.RotateToken(configFile, token)
		if err != nil {
			return err
		}
		log.Printf("%s: rotated %s\n", rotation.Path, strings.Join(rotation.Keys, ", "))
		log.Printf("New token: %s\n", rotation.Token)
		mirrorConfigToGreeter(rotation.Path)

		if push != "" {
			if err := pushToken(push, remoteBinary, remoteConfig, rotation.Token); err != nil {
				// The local half is already written, so say what is now out of
				// step rather than pretending nothing happened.
				return errors.Wrapf(err, "local config rotated, but %s was not updated - it will reject the new token until it is", push)
			}
			log.Printf("Pushed controller.auth_token to %s\n", push)
		} else {
			log.Println("Not pushed anywhere: set controller.auth_token on the Pi too, or re-run with --push <user@host>.")
		}

		log.Println("Restart both components to pick it up (systemctl --user restart ottoman-agent, and ottoman-controller on the Pi).")
		return nil
	},
}

// pushToken sets controller.auth_token on a remote host and restarts its
// controller, using the remote ottoman binary's own `config set` so the value
// gets the same validation it would locally.
//
// The token rides in the remote command line, so it is visible in that host's
// process list for the moment the command runs. Acceptable here: the target is
// a single-user Pi you already have shell on, and the alternative (feeding it
// over stdin) needs a remote shell snippet that is harder to read than the risk
// it removes.
func pushToken(target, binary, remoteConfig, token string) error {
	remote := remotePathArg(binary) + " config set"
	if remoteConfig != "" {
		remote += " --config " + remotePathArg(remoteConfig)
	}
	remote += " controller.auth_token " + shellQuote(token)
	// Best-effort restart: a controller that isn't running as a user service
	// (or isn't installed yet) shouldn't fail the rotation.
	remote += "; systemctl --user restart ottoman-controller || true"

	c := exec.Command("ssh", target, remote)
	c.Stdout = os.Stderr
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	return c.Run()
}

// shellQuote wraps s for a remote /bin/sh command line.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// remotePathArg quotes a remote path, expanding a leading ~/ through $HOME:
// tilde expansion only happens on an unquoted ~, so a quoted "~/..." would be
// taken literally, while $HOME does expand inside double quotes.
func remotePathArg(p string) string {
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		return `"$HOME/` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", "$", `\$`).Replace(rest) + `"`
	}
	return shellQuote(p)
}

// mirrorConfigToGreeter pushes a just-written config to the login-screen
// agent's copy, so a CLI edit doesn't leave the greeter on a stale token.
func mirrorConfigToGreeter(path string) {
	copied, err := agent.MirrorConfigToGreeter(path)
	if err != nil {
		log.Printf("Warning: failed to mirror config to the greeter copy: %v\n", err)
		return
	}
	if copied {
		log.Println("Mirrored to the login-screen agent's copy.")
	}
}

// restartHint says which component has to be restarted for a key to take
// effect, since nothing rereads the config while running.
func restartHint(key string) string {
	if strings.HasPrefix(key, "controller.") {
		return "Restart the controller to pick it up: systemctl --user restart ottoman-controller"
	}
	return "Restart the agent to pick it up: systemctl --user restart ottoman-agent"
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
	agentRunCmd.Flags().BoolVar(&agentRunGreeter, "greeter", false, "run in GDM greeter mode: display/layouts only, applies the last-used layout on start")
	agentCmd.AddCommand(agentRunCmd)
	agentCmd.AddCommand(layoutCmd)
	agentCmd.AddCommand(monitorCmd)
	agentCmd.AddCommand(agentInstallCmd)
	agentCmd.AddCommand(agentUninstallCmd)
	agentHostSetupCmd.Flags().StringVar(&hostSetupUser, "user", "", "user to grant access to (default: invoking user)")
	agentHostSetupCmd.Flags().BoolVar(&hostSetupGreeter, "greeter", false, "also install the GDM login-screen layout agent")
	agentCmd.AddCommand(agentHostSetupCmd)

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
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configRotateTokenCmd)
	configRotateTokenCmd.Flags().String("token", "", "use this token instead of generating one")
	configRotateTokenCmd.Flags().String("push", "", "also set controller.auth_token on this SSH target (user@host)")
	configRotateTokenCmd.Flags().String("remote-binary", "~/.local/share/ottoman/ottoman", "path to the ottoman binary on the --push target")
	configRotateTokenCmd.Flags().String("remote-config", "", "config path on the --push target (default: whatever it would load)")

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

	// Verbose subprocess logging is opt-in via OTTOMAN_DEBUG=1 — left on, it
	// accounts for ~93% of the log volume and buries the entries that matter.
	// HTTP requests are always logged regardless.
	if v := os.Getenv("OTTOMAN_DEBUG"); v != "" && v != "0" && v != "false" {
		common.SetDebugLogging(true)
		log.Printf("Debug logging enabled (OTTOMAN_DEBUG=%s)", v)
	}
}
