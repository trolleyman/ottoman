//go:build mage

package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

var (
	buildDir = getEnv("BUILD_DIR", "build")
	binary   = "ottoman"
)

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// BuildTarget represents a cross-compilation target.
type BuildTarget struct {
	Name   string
	GOOS   string
	GOARCH string
	GOARM  string
	Output string
}

var targets = map[string]BuildTarget{
	"pi": {
		Name:   "Raspberry Pi",
		GOOS:   "linux",
		GOARCH: "arm",
		GOARM:  "7",
		Output: binary + "-linux-arm",
	},
	"windows": {
		Name:   "Windows",
		GOOS:   "windows",
		GOARCH: "amd64",
		Output: binary + "-windows-amd64.exe",
	},
	"linux": {
		Name:   "Linux",
		GOOS:   "linux",
		GOARCH: "amd64",
		Output: binary + "-linux-amd64",
	},
}

// Logging configuration
var (
	logDir        = getDefaultLogDir()
	logMaxSize    = int64(5 * 1024 * 1024) // 5MB per file
	logMaxBackups = 5
)

// getDefaultLogDir returns the preferred log directory depending on OS.
func getDefaultLogDir() string {
	if runtime.GOOS == "windows" {
		if v := os.Getenv("LOCALAPPDATA"); v != "" {
			return filepath.Join(v, "ottoman", "logs")
		}
		if v := os.Getenv("TMP"); v != "" {
			return filepath.Join(v, "ottoman", "logs")
		}
	}
	if v := os.Getenv("XDG_CACHE_HOME"); v != "" {
		return filepath.Join(v, "ottoman", "logs")
	}
	if h := os.Getenv("HOME"); h != "" {
		return filepath.Join(h, ".cache", "ottoman", "logs")
	}
	return filepath.Join(".", "logs")
}

// RotatingLogger is a simple size-based rotating logger.
type RotatingLogger struct {
	mu         sync.Mutex
	path       string
	maxSize    int64
	maxBackups int
	file       *os.File
}

// NewRotatingLogger creates or opens the log file and returns a RotatingLogger.
func NewRotatingLogger(path string, maxSize int64, maxBackups int) (*RotatingLogger, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	return &RotatingLogger{path: path, maxSize: maxSize, maxBackups: maxBackups, file: f}, nil
}

func (r *RotatingLogger) Write(p []byte) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	fi, err := r.file.Stat()
	if err != nil {
		return 0, err
	}
	if fi.Size()+int64(len(p)) > r.maxSize {
		if err := r.rotate(); err != nil {
			return 0, err
		}
	}
	return r.file.Write(p)
}

func (r *RotatingLogger) rotate() error {
	// Close current
	if r.file != nil {
		r.file.Close()
	}
	// Rename with timestamp
	ts := time.Now().UTC().Format("20060102T150405Z")
	newName := fmt.Sprintf("%s.%s", r.path, ts)
	if err := os.Rename(r.path, newName); err != nil {
		// If rename fails because file doesn't exist, ignore
		if !os.IsNotExist(err) {
			return err
		}
	}
	// Recreate current log file
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	r.file = f

	// Enforce backups limit
	if err := r.enforceBackups(); err != nil {
		return err
	}
	return nil
}

func (r *RotatingLogger) enforceBackups() error {
	dir := filepath.Dir(r.path)
	base := filepath.Base(r.path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	// collect rotated files matching base.
	var candidates []os.DirEntry
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, base+".") {
			candidates = append(candidates, e)
		}
	}
	if len(candidates) <= r.maxBackups {
		return nil
	}
	// Sort by name (timestamps) ascending
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].Name() < candidates[j].Name() })
	toRemove := len(candidates) - r.maxBackups
	for i := 0; i < toRemove; i++ {
		_ = os.Remove(filepath.Join(dir, candidates[i].Name()))
	}
	return nil
}

// getVersion returns the version from git describe.
func getVersion() string {
	out, err := sh.Output("git", "describe", "--tags", "--always", "--dirty")
	if err != nil {
		return "dev"
	}
	v := strings.TrimSpace(out)
	if v == "" {
		return "dev"
	}
	return v
}

// ensureBuildDir creates the build directory if it doesn't exist.
func ensureBuildDir() error {
	return os.MkdirAll(buildDir, 0755)
}

// --- Pretty-printing helpers for commands, copies, and moves ---

// ANSI color codes for pretty-printing.
const (
	colorReset  = "\033[0m"
	colorCyan   = "\033[36m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBold   = "\033[1m"
)

// Quotes a string for display as a shell argument.
func shellQuoteForce(s string) string {
	containsDoubleQuote := strings.Contains(s, `"`)
	containsSingleQuote := strings.Contains(s, `'`)
	escaped := strings.ReplaceAll(s, "\t", `\t`)
	escaped = strings.ReplaceAll(s, `\`, `\\`)
	if !containsDoubleQuote {
		return `"` + escaped + `"`
	} else if !containsSingleQuote {
		return `'` + escaped + `'`
	} else {
		return `"` + strings.ReplaceAll(escaped, `"`, `\"`) + `"`
	}
}

// Quotes a string for display as a shell argument if necessary.
// Args with whitespace or quotes are wrapped in double quotes; embedded " and ' are escaped.
func shellQuote(s string) string {
	if s == "" {
		return `""`
	}
	containsDoubleQuote := strings.Contains(s, `"`)
	containsSingleQuote := strings.Contains(s, `'`)
	containsQuote := containsDoubleQuote || containsSingleQuote
	containsWhitespace := strings.ContainsAny(s, " \t")
	if containsQuote || containsWhitespace {
		return shellQuoteForce(s)
	}
	return s
}

// formatCmd formats a command and its arguments for display.
func formatCmd(cmd string, args ...string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, shellQuote(cmd))
	for _, a := range args {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

// displayPath returns a path suitable for display.
// Paths inside cwd are shown as relative; paths outside are shown as absolute.
func displayPath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	cwd, err := os.Getwd()
	if err != nil {
		return abs
	}
	rel, err := filepath.Rel(cwd, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return abs
	}
	return filepath.ToSlash(rel)
}

// formatPathPair formats a source and destination path pair for display.
// If paths share a common directory, shows as dir/{src -> dst}.
func formatPathPair(src, dst string) string {
	ds := displayPath(src)
	dd := displayPath(dst)
	dirS := filepath.Dir(ds)
	dirD := filepath.Dir(dd)
	if dirS == dirD && dirS != "." {
		return fmt.Sprintf("%s/{%s -> %s}", dirS, filepath.Base(ds), filepath.Base(dd))
	}
	return fmt.Sprintf("%s -> %s", ds, dd)
}

// run runs a command silently (no stdout/stderr forwarding), printing "Running: ..." first.
func run(cmd string, args ...string) error {
	fmt.Printf("%s%sRunning:%s %s\n", colorBold, colorCyan, colorReset, formatCmd(cmd, args...))
	if err := sh.Run(cmd, args...); err != nil {
		return fmt.Errorf("failed to run %q: %w", cmd, err)
	}
	return nil
}

// start starts a comand in the background, with no stdout/stderr forwarding, printing "Starting..." first.
func start(cmd string, args ...string) error {
	fmt.Printf("%s%sStarting:%s %s\n", colorBold, colorCyan, colorReset, formatCmd(cmd, args...))
	if err := exec.Command(cmd, args...).Start(); err != nil {
		return fmt.Errorf("failed to start %q: %w", cmd, err)
	}
	return nil
}

// runV runs a command with stdout/stderr forwarded, printing "Running: ..." first.
func runV(cmd string, args ...string) error {
	fmt.Printf("%s%sRunning:%s %s\n", colorBold, colorCyan, colorReset, formatCmd(cmd, args...))
	if err := sh.RunV(cmd, args...); err != nil {
		return fmt.Errorf("failed to run %q: %w", cmd, err)
	}
	return nil
}

// runWithEnv runs a command with environment variables set, printing "Running: ..." first.
func runWithEnv(env map[string]string, cmd string, args ...string) error {
	fmt.Printf("%s%sRunning:%s %s\n", colorBold, colorCyan, colorReset, formatCmd(cmd, args...))
	if err := sh.RunWith(env, cmd, args...); err != nil {
		return fmt.Errorf("failed to run %q: %w", cmd, err)
	}
	return nil
}

// runInDir runs a command in a specific directory, printing "Running: ... (in dir)" first.
func runInDir(dir string, cmd string, args ...string) error {
	fmt.Printf("%s%sRunning:%s %s (in %s)\n", colorBold, colorCyan, colorReset, formatCmd(cmd, args...), displayPath(dir))
	c := exec.Command(cmd, args...)
	c.Dir = dir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("failed to run %q in %q: %w", cmd, dir, err)
	}
	return nil
}

// copyFile copies a file from src to dst, printing "Copying: ..." first.
// Creates destination directories as needed and removes existing dst on Windows.
func copyFile(src, dst string) error {
	fmt.Printf("%s%sCopying:%s %s\n", colorBold, colorGreen, colorReset, formatPathPair(src, dst))

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	// Remove existing file first (in case it's in use on Windows)
	os.Remove(dst)

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// moveFile moves (renames) a file from src to dst, printing "Moving: ..." first.
// Creates destination directories as needed.
func moveFile(src, dst string) error {
	fmt.Printf("%s%sMoving:%s %s\n", colorBold, colorYellow, colorReset, formatPathPair(src, dst))

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	return os.Rename(src, dst)
}

// Build the web files (bun install + bun run build).
func buildWebFiles() error {
	webDir := "web"

	// Run bun install if node_modules doesn't exist
	if _, err := os.Stat(filepath.Join(webDir, "node_modules")); os.IsNotExist(err) {
		if err := runInDir(webDir, "bun", "install"); err != nil {
			return fmt.Errorf("bun install failed: %w", err)
		}
	}

	if err := runInDir(webDir, "bun", "run", "build"); err != nil {
		return fmt.Errorf("bun run build failed: %w", err)
	}
	return nil
}

func BuildWeb() error {
	return buildWebFiles()
}

// buildTarget compiles for a specific target.
func buildTarget(target BuildTarget, version string) error {
	ldflags := fmt.Sprintf("-X main.Version=%s", version)
	outputPath := filepath.Join(buildDir, target.Output)

	env := map[string]string{
		"GOOS":   target.GOOS,
		"GOARCH": target.GOARCH,
	}
	if target.GOARM != "" {
		env["GOARM"] = target.GOARM
	}

	return runWithEnv(env, "go", "build", "-ldflags", ldflags, "-o", outputPath, "./cmd/ottoman")
}

func buildDeps() {
	mg.SerialDeps(ensureBuildDir, buildWebFiles)
}

// Build builds the client for the current platform.
func Build() error {
	buildDeps()

	version := getVersion()
	ldflags := fmt.Sprintf("-X main.Version=%s", version)
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	outputPath := filepath.Join(buildDir, binary+ext)

	return run("go", "build", "-ldflags", ldflags, "-o", outputPath, "./cmd/ottoman")
}

// BuildAll builds for all platforms.
func BuildAll() error {
	buildDeps()

	version := getVersion()
	fmt.Printf("Version: %s\n\n", version)

	for _, target := range targets {
		if err := buildTarget(target, version); err != nil {
			return err
		}
	}

	fmt.Println("\nBuild complete!")
	return nil
}

// BuildPi builds for Raspberry Pi (linux/arm).
func BuildPi() error {
	buildDeps()

	version := getVersion()
	fmt.Printf("Version: %s\n\n", version)

	return buildTarget(targets["pi"], version)
}

// BuildWindows builds for Windows (windows/amd64).
func BuildWindows() error {
	buildDeps()

	version := getVersion()
	fmt.Printf("Version: %s\n\n", version)

	return buildTarget(targets["windows"], version)
}

// BuildLinux builds for Linux desktop (linux/amd64).
func BuildLinux() error {
	buildDeps()

	version := getVersion()
	fmt.Printf("Version: %s\n\n", version)

	return buildTarget(targets["linux"], version)
}

// Clean removes build artifacts.
func Clean() error {
	if err := sh.Rm(buildDir); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	}

	if err := sh.Rm(filepath.Join("web", "client", "dist")); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	}

	return run("go", "clean")
}

// Test runs tests.
func Test() error {
	return runV("go", "test", "-v", "./...")
}

// Lint runs the linter.
func Lint() error {
	// Check if golangci-lint is available
	if _, err := exec.LookPath("golangci-lint"); err != nil {
		return runV("go", "vet", "./...")
	}
	return runV("golangci-lint", "run")
}

// RunController runs the controller locally.
func RunController() error {
	mg.Deps(buildWebFiles)
	controllerConfigFile := filepath.Join("magefiles", "dev_controller.toml")
	_, err := os.Stat(controllerConfigFile)
	if os.IsNotExist(err) {
		err = runV("go", "run", "./cmd/ottoman", "config", "init", "controller", "--output", controllerConfigFile)
	} else if err != nil {
		return fmt.Errorf("failed to read %q: %w", controllerConfigFile, err)
	} else {
		fmt.Printf("Loading existing config: %s\n", controllerConfigFile)
	}
	return runV("go", "run", "./cmd/ottoman", "--config", controllerConfigFile, "controller", "run")
}

// RunAgent runs the agent locally.
func RunAgent() error {
	mg.Deps(buildWebFiles)
	agentConfigFile := filepath.Join("magefiles", "dev_agent.toml")
	_, err := os.Stat(agentConfigFile)
	if os.IsNotExist(err) {
		err = runV("go", "run", "./cmd/ottoman", "config", "init", "agent", "--output", agentConfigFile)
		if err != nil {
			return fmt.Errorf("failed to run config init agent: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to read %q: %w", agentConfigFile, err)
	} else {
		fmt.Printf("Loading existing config: %s\n", agentConfigFile)
	}
	return runV("go", "run", "./cmd/ottoman", "--config", agentConfigFile, "agent", "run")
}

// RunSimulated runs a simulated controller for frontend WoL testing.
func RunSimulated() error {
	mg.Deps(buildWebFiles)

	controllerConfigFile := filepath.Join("magefiles", "dev_controller.toml")
	agentConfigFile := filepath.Join("magefiles", "dev_agent.toml")

	// Ensure controller config exists
	if _, err := os.Stat(controllerConfigFile); os.IsNotExist(err) {
		if err := runV("go", "run", "./cmd/ottoman", "config", "init", "controller", "--output", controllerConfigFile); err != nil {
			return fmt.Errorf("failed to init controller config: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to read %q: %w", controllerConfigFile, err)
	}

	// Ensure agent config exists
	if _, err := os.Stat(agentConfigFile); os.IsNotExist(err) {
		if err := runV("go", "run", "./cmd/ottoman", "config", "init", "agent", "--output", agentConfigFile); err != nil {
			return fmt.Errorf("failed to init agent config: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to read %q: %w", agentConfigFile, err)
	}

	return runV("go", "run", "./cmd/ottoman",
		"--config", controllerConfigFile,
		"controller", "simulate",
		"--agent-config", agentConfigFile)
}

// DeployConfig holds deployment configuration
type DeployConfig struct {
	Agent      AgentDeployConfig      `toml:"agent"`
	Controller ControllerDeployConfig `toml:"controller"`
}

// AgentDeployConfig holds agent deployment settings
type AgentDeployConfig struct {
	BinaryPath string `toml:"binary_path"`
}

// ControllerDeployConfig holds controller deployment settings
type ControllerDeployConfig struct {
	SSHTarget  string `toml:"ssh_target"`
	DeployPath string `toml:"deploy_path"`
	ConfigPath string `toml:"config_path"`
}

var deployConfigPath = filepath.Join("magefiles", "deploy.toml")

// loadDeployConfig loads the deployment configuration
func loadDeployConfig() (*DeployConfig, error) {
	cfg := &DeployConfig{}
	if _, err := os.Stat(deployConfigPath); os.IsNotExist(err) {
		return cfg, nil
	}
	if _, err := toml.DecodeFile(deployConfigPath, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse deploy.toml: %w", err)
	}
	return cfg, nil
}

// saveDeployConfig saves the deployment configuration
func saveDeployConfig(cfg *DeployConfig) error {
	f, err := os.Create(deployConfigPath)
	if err != nil {
		return fmt.Errorf("failed to create deploy.toml: %w", err)
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	if err := encoder.Encode(cfg); err != nil {
		return fmt.Errorf("failed to write deploy.toml: %w", err)
	}
	return nil
}

// prompt asks for user input with a default value
func prompt(reader *bufio.Reader, question, defaultVal string) string {
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

// fileExists checks if a file exists and is not a directory
func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// hasFlag checks if a flag is present in os.Args
func hasFlag(name string) bool {
	for _, arg := range os.Args {
		if arg == name {
			return true
		}
	}
	return false
}

// defaultClientBinaryPath returns the default client binary path for the current platform
func defaultClientBinaryPath() string {
	switch runtime.GOOS {
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			localAppData = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
		}
		return filepath.Join(localAppData, "ottoman", "ottoman.exe")
	default:
		home := os.Getenv("HOME")
		return filepath.Join(home, ".local", "bin", "ottoman")
	}
}

// DeployAgent builds and deploys the agent locally.
// Interactively asks for settings and saves them to magefiles/deploy.toml.
func DeployAgent() error {
	fmt.Println("=== Ottoman Agent Deployment ===\n")

	agentConfigPath := filepath.Join("magefiles", "deploy_agent.toml")
	reconfigure := hasFlag("--config")
	deployConfigExists := fileExists(deployConfigPath)
	agentConfigExists := fileExists(agentConfigPath)

	// Load existing config
	cfg, err := loadDeployConfig()
	if err != nil {
		fmt.Printf("Warning: %v\n", err)
		cfg = &DeployConfig{}
	}

	if !reconfigure && deployConfigExists && agentConfigExists {
		fmt.Printf("Using existing deployment config: %s\n", deployConfigPath)
		if content, err := os.ReadFile(deployConfigPath); err == nil {
			fmt.Println(string(content))
		}
	} else {
		reader := bufio.NewReader(os.Stdin)

		// Get binary path
		defaultPath := cfg.Agent.BinaryPath
		if defaultPath == "" {
			defaultPath = defaultClientBinaryPath()
		}
		cfg.Agent.BinaryPath = prompt(reader, "Binary install path", defaultPath)

		// Save deploy config
		if err := saveDeployConfig(cfg); err != nil {
			return err
		}
		fmt.Printf("\nSaved deploy config to %s\n", deployConfigPath)

		// Generate agent config via config init
		if err := runV("go", "run", "./cmd/ottoman", "config", "init", "agent", "--output", agentConfigPath); err != nil {
			return fmt.Errorf("config init failed: %w", err)
		}
	}

	// Stop existing service/process to allow binary overwrite
	if runtime.GOOS == "windows" {
		run("schtasks", "/End", "/TN", "OttomanAgent")
		// Force kill to ensure file is released
		run("taskkill", "/F", "/IM", "ottoman.exe")
	} else {
		run("systemctl", "--user", "stop", "ottoman-agent")
	}

	// Build for current platform
	if err := Build(); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	// Get built binary path
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	builtBinary := filepath.Join(buildDir, binary+ext)

	// Copy binary to target location
	if err := copyFile(builtBinary, cfg.Agent.BinaryPath); err != nil {
		return fmt.Errorf("failed to copy binary: %w", err)
	}

	// Make executable on Unix
	if runtime.GOOS != "windows" {
		if err := os.Chmod(cfg.Agent.BinaryPath, 0755); err != nil {
			return fmt.Errorf("failed to make binary executable: %w", err)
		}
	}

	// Copy config to actual config location
	configDst := defaultConfigPath()
	if err := copyFile(agentConfigPath, configDst); err != nil {
		return fmt.Errorf("failed to copy config: %w", err)
	}

	// Run install command to register service
	if err := runV(cfg.Agent.BinaryPath, "agent", "install"); err != nil {
		return fmt.Errorf("failed to register service: %w", err)
	}

	// Start services on Windows
	if runtime.GOOS == "windows" {
		fmt.Println("Starting OttomanAgent task...")
		if err := run("schtasks", "/Run", "/TN", "OttomanAgent"); err != nil {
			fmt.Printf("Warning: failed to start task: %v\n", err)
		}

		fmt.Println("Starting AHK script...")
		if appData := os.Getenv("APPDATA"); appData != "" {
			ahkVbsPath := filepath.Join(appData, "ottoman", "ottoman-ahk.vbs")
			if err := exec.Command("wscript", "//nologo", ahkVbsPath).Start(); err != nil {
				fmt.Printf("Warning: failed to start AHK script: %v\n", err)
			}
		}
	}

	fmt.Println("\n=== Agent deployment complete! ===")
	return nil
}

// defaultConfigPath returns the default ottoman config path for the current platform
func defaultConfigPath() string {
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

// DeployController deploys the controller to a Raspberry Pi via SSH.
// Interactively asks for deployment settings (saved to magefiles/deploy.toml)
// and delegates controller config creation to `ottoman config init controller`.
func DeployController() error {
	fmt.Println("=== Ottoman Controller Deployment ===\n")

	controllerConfigPath := filepath.Join("magefiles", "deploy_controller.toml")
	reconfigure := hasFlag("--config")
	deployConfigExists := fileExists(deployConfigPath)
	controllerConfigExists := fileExists(controllerConfigPath)

	// Load existing deploy config
	cfg, err := loadDeployConfig()
	if err != nil {
		fmt.Printf("Warning: %v\n", err)
		cfg = &DeployConfig{}
	}

	if !reconfigure && deployConfigExists && controllerConfigExists {
		fmt.Printf("Using existing deployment config: %s\n", deployConfigPath)
		if content, err := os.ReadFile(deployConfigPath); err == nil {
			fmt.Println(string(content))
		}
	} else {
		reader := bufio.NewReader(os.Stdin)

		// Prompt for deployment settings
		fmt.Println("--- Deployment Settings ---")

		if cfg.Controller.SSHTarget == "" {
			cfg.Controller.SSHTarget = prompt(reader, "SSH target (user@host)", "")
		} else {
			cfg.Controller.SSHTarget = prompt(reader, "SSH target (user@host)", cfg.Controller.SSHTarget)
		}
		if cfg.Controller.SSHTarget == "" {
			return fmt.Errorf("SSH target is required")
		}

		if cfg.Controller.DeployPath == "" {
			cfg.Controller.DeployPath = "~/.local/share/ottoman/ottoman"
		}
		cfg.Controller.DeployPath = prompt(reader, "Remote binary path", cfg.Controller.DeployPath)

		if cfg.Controller.ConfigPath == "" {
			cfg.Controller.ConfigPath = "~/.config/ottoman/config.toml"
		}
		cfg.Controller.ConfigPath = prompt(reader, "Remote config path", cfg.Controller.ConfigPath)

		// Save deploy config
		if err := saveDeployConfig(cfg); err != nil {
			return err
		}
		fmt.Printf("\nSaved deploy config to %s\n", deployConfigPath)

		// Generate controller config via config init
		if err := runV("go", "run", "./cmd/ottoman", "config", "init", "controller", "--output", controllerConfigPath); err != nil {
			return fmt.Errorf("config init failed: %w", err)
		}
	}

	// Build for Raspberry Pi
	if err := BuildPi(); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	binaryPath := filepath.Join(buildDir, "ottoman-linux-arm")

	// Create directories on remote (use path.Dir for Unix paths, not filepath.Dir)
	deployDir := path.Dir(expandPath(cfg.Controller.DeployPath))
	configDir := path.Dir(expandPath(cfg.Controller.ConfigPath))

	if err := run("ssh", cfg.Controller.SSHTarget, fmt.Sprintf("mkdir -p '%s'", deployDir)); err != nil {
		return fmt.Errorf("failed to create deploy directory: %w", err)
	}

	if err := run("ssh", cfg.Controller.SSHTarget, fmt.Sprintf("mkdir -p '%s'", configDir)); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Stop service if running (ignore errors as it might not exist yet)
	_ = run("ssh", cfg.Controller.SSHTarget, "systemctl --user stop ottoman-controller")

	// Remove binary if it exists
	if err := run("ssh", cfg.Controller.SSHTarget, fmt.Sprintf("rm -f '%s'", cfg.Controller.DeployPath)); err != nil {
		return fmt.Errorf("failed to remove existing binary: %w", err)
	}

	// Copy binary (use scpPath to handle ~ properly)
	if err := run("scp", binaryPath, fmt.Sprintf("%s:%s", cfg.Controller.SSHTarget, scpPath(cfg.Controller.DeployPath))); err != nil {
		return fmt.Errorf("failed to copy binary: %w", err)
	}

	// Make executable
	if err := run("ssh", cfg.Controller.SSHTarget, fmt.Sprintf("chmod +x %s", cfg.Controller.DeployPath)); err != nil {
		return fmt.Errorf("failed to chmod: %w", err)
	}

	// Write config file
	if err := run("scp", controllerConfigPath, fmt.Sprintf("%s:%s", cfg.Controller.SSHTarget, scpPath(cfg.Controller.ConfigPath))); err != nil {
		return fmt.Errorf("failed to copy config: %w", err)
	}

	// Install systemd service
	installCmd := fmt.Sprintf("%s controller install", cfg.Controller.DeployPath)
	if err := run("ssh", cfg.Controller.SSHTarget, installCmd); err != nil {
		return fmt.Errorf("failed to install service: %w", err)
	}

	// Enable lingering to ensure service starts on boot
	if err := run("ssh", cfg.Controller.SSHTarget, "loginctl enable-linger"); err != nil {
		fmt.Printf("Warning: failed to enable lingering: %v\n", err)
	}

	// Restart service
	if err := run("ssh", cfg.Controller.SSHTarget, "systemctl --user restart ottoman-controller"); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	// Checking status
	fmt.Println("\nDeployed - checking status:")
	if err := run("ssh", cfg.Controller.SSHTarget, "systemctl --user status ottoman-controller"); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	fmt.Println("\n=== Controller deployment complete! ===")
	return nil
}

func DeployAll() error {
	mg.SerialDeps(DeployController, DeployAgent)
	return nil
}

// expandPath keeps ~ for shell commands (ssh) - the remote shell expands it
func expandPath(path string) string {
	// Keep ~ as-is - remote shell will expand it
	return path
}

// scpPath converts ~/path to relative path for scp (scp defaults to home dir)
func scpPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		return path[2:] // Remove "~/" - scp uses home dir by default
	}
	return path
}

// Deps installs Go dependencies.
func Deps() error {
	return run("go", "mod", "tidy")
}

type Generate mg.Namespace

// All runs both Go and TypeScript generation
func (Generate) All() {
	mg.Deps(Generate.Go, Generate.TypeScript)
}

// Go generates the Go server interface and types
func (Generate) Go() error {
	fmt.Println("🚀 Generating Go API...")

	// Ensure internal/api exists
	if err := os.MkdirAll("internal/api", 0755); err != nil {
		return err
	}

	// Generate the Server Interface and Types
	// We use "server" generation because both the Controller (Pi)
	// and Agent (Desktop) implement this API to some degree.
	return run("go", "generate", "./...")
}

// TypeScript generates the React client
func (Generate) TypeScript() error {
	fmt.Println("🚀 Generating TypeScript Client...")

	// Ensure web/src/api exists
	if err := os.MkdirAll("web/src/api", 0755); err != nil {
		return err
	}

	// uses openapi-typescript-codegen
	// --client fetch: Uses the native Fetch API (lightweight, no axios)
	// --name OttomanClient: The name of the client class
	return run("bun", "x", "openapi-typescript-codegen",
		"--input", "api/openapi.yaml",
		"--output", "web/src/api",
		"--client", "fetch",
		"--name", "OttomanClient",
	)
}
