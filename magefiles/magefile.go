//go:build mage

package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
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

func init() {
	// Ensure log directory exists and initialize logger
	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Printf("warning: could not create log dir %s: %v\n", logDir, err)
		return
	}
	rl, err := NewRotatingLogger(filepath.Join(logDir, "ottoman.log"), logMaxSize, logMaxBackups)
	if err != nil {
		fmt.Printf("warning: could not create rotating logger: %v\n", err)
		return
	}
	log.SetOutput(io.MultiWriter(os.Stderr, rl))
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
}

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

// shellQuote quotes a string for display as a shell argument.
// Args with spaces are wrapped in double quotes; embedded " and ' are escaped.
func shellQuote(s string) string {
	if s == "" {
		return `""`
	}
	if !strings.ContainsAny(s, " \t\"'\\") {
		return s
	}
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	escaped = strings.ReplaceAll(escaped, `'`, `\'`)
	return `"` + escaped + `"`
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
	return sh.Run(cmd, args...)
}

// runV runs a command with stdout/stderr forwarded, printing "Running: ..." first.
func runV(cmd string, args ...string) error {
	fmt.Printf("%s%sRunning:%s %s\n", colorBold, colorCyan, colorReset, formatCmd(cmd, args...))
	return sh.RunV(cmd, args...)
}

// runWithEnv runs a command with environment variables set, printing "Running: ..." first.
func runWithEnv(env map[string]string, cmd string, args ...string) error {
	fmt.Printf("%s%sRunning:%s %s\n", colorBold, colorCyan, colorReset, formatCmd(cmd, args...))
	return sh.RunWith(env, cmd, args...)
}

// runInDir runs a command in a specific directory, printing "Running: ... (in dir)" first.
func runInDir(dir string, cmd string, args ...string) error {
	fmt.Printf("%s%sRunning:%s %s (in %s)\n", colorBold, colorCyan, colorReset, formatCmd(cmd, args...), displayPath(dir))
	c := exec.Command(cmd, args...)
	c.Dir = dir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
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
func buildWebFiles(webDir string) error {
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

func getWebClientPath() string {
	return filepath.Join("web", "client")
}

func getWebServerPath() string {
	return filepath.Join("web", "server")
}

func buildWebClientFiles() error {
	return buildWebFiles(getWebClientPath())
}

func buildWebServerFiles() error {
	return buildWebFiles(getWebServerPath())
}

func BuildWebClient() error {
	return buildWebClientFiles()
}

func BuildWebServer() error {
	return buildWebServerFiles()
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
	mg.SerialDeps(ensureBuildDir, buildWebClientFiles, buildWebServerFiles)
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

// RunServer runs the server locally.
func RunServer() error {
	mg.Deps(buildWebServerFiles)
	return runV("go", "run", "./cmd/ottoman", "server", "run")
}

// RunClient runs the client locally.
func RunClient() error {
	mg.Deps(buildWebClientFiles)
	return runV("go", "run", "./cmd/ottoman", "client", "run")
}

// DeployConfig holds deployment configuration
type DeployConfig struct {
	Client ClientDeployConfig `toml:"client"`
	Server ServerDeployConfig `toml:"server"`
}

// ClientDeployConfig holds client deployment settings
type ClientDeployConfig struct {
	BinaryPath string `toml:"binary_path"`
}

// ServerDeployConfig holds server deployment settings
type ServerDeployConfig struct {
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

// DeployClient builds and deploys the client locally.
// Interactively asks for settings and saves them to magefiles/deploy.toml.
func DeployClient() error {
	fmt.Println("=== Ottoman Client Deployment ===\n")

	// Load existing config
	cfg, err := loadDeployConfig()
	if err != nil {
		fmt.Printf("Warning: %v\n", err)
		cfg = &DeployConfig{}
	}

	reader := bufio.NewReader(os.Stdin)

	// Get binary path
	defaultPath := cfg.Client.BinaryPath
	if defaultPath == "" {
		defaultPath = defaultClientBinaryPath()
	}
	cfg.Client.BinaryPath = prompt(reader, "Binary install path", defaultPath)

	// Save deploy config
	if err := saveDeployConfig(cfg); err != nil {
		return err
	}
	fmt.Printf("\nSaved deploy config to %s\n", deployConfigPath)

	// Generate client config via config init
	clientConfigPath := filepath.Join("magefiles", "deploy_client.toml")
	if err := runV("go", "run", "./cmd/ottoman", "config", "init", "client", "--output", clientConfigPath); err != nil {
		return fmt.Errorf("config init failed: %w", err)
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
	if err := copyFile(builtBinary, cfg.Client.BinaryPath); err != nil {
		return fmt.Errorf("failed to copy binary: %w", err)
	}

	// Make executable on Unix
	if runtime.GOOS != "windows" {
		if err := os.Chmod(cfg.Client.BinaryPath, 0755); err != nil {
			return fmt.Errorf("failed to make binary executable: %w", err)
		}
	}

	// Copy config to actual config location
	configDst := defaultConfigPath()
	if err := copyFile(clientConfigPath, configDst); err != nil {
		return fmt.Errorf("failed to copy config: %w", err)
	}

	// Run install command to register service
	if err := runV(cfg.Client.BinaryPath, "client", "install"); err != nil {
		return fmt.Errorf("failed to register service: %w", err)
	}

	fmt.Println("\n=== Client deployment complete! ===")
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

// DeployServer deploys the server to a Raspberry Pi via SSH.
// Interactively asks for deployment settings (saved to magefiles/deploy.toml)
// and delegates server config creation to `ottoman config init server`.
func DeployServer() error {
	fmt.Println("=== Ottoman Server Deployment ===\n")

	// Load existing deploy config
	cfg, err := loadDeployConfig()
	if err != nil {
		fmt.Printf("Warning: %v\n", err)
		cfg = &DeployConfig{}
	}

	reader := bufio.NewReader(os.Stdin)

	// Prompt for deployment settings
	fmt.Println("--- Deployment Settings ---")

	if cfg.Server.SSHTarget == "" {
		cfg.Server.SSHTarget = prompt(reader, "SSH target (user@host)", "")
	} else {
		cfg.Server.SSHTarget = prompt(reader, "SSH target (user@host)", cfg.Server.SSHTarget)
	}
	if cfg.Server.SSHTarget == "" {
		return fmt.Errorf("SSH target is required")
	}

	if cfg.Server.DeployPath == "" {
		cfg.Server.DeployPath = "~/.local/share/ottoman/ottoman"
	}
	cfg.Server.DeployPath = prompt(reader, "Remote binary path", cfg.Server.DeployPath)

	if cfg.Server.ConfigPath == "" {
		cfg.Server.ConfigPath = "~/.config/ottoman/config.toml"
	}
	cfg.Server.ConfigPath = prompt(reader, "Remote config path", cfg.Server.ConfigPath)

	// Save deploy config
	if err := saveDeployConfig(cfg); err != nil {
		return err
	}
	fmt.Printf("\nSaved deploy config to %s\n", deployConfigPath)

	// Generate server config via config init
	serverConfigPath := filepath.Join("magefiles", "deploy_server.toml")
	if err := runV("go", "run", "./cmd/ottoman", "config", "init", "server", "--output", serverConfigPath); err != nil {
		return fmt.Errorf("config init failed: %w", err)
	}

	// Read generated config
	serverConfig, err := os.ReadFile(serverConfigPath)
	if err != nil {
		return fmt.Errorf("failed to read generated config: %w", err)
	}

	// Build for Raspberry Pi
	if err := BuildPi(); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	binaryPath := filepath.Join(buildDir, "ottoman-linux-arm")

	// Create directories on remote (use path.Dir for Unix paths, not filepath.Dir)
	deployDir := path.Dir(expandPath(cfg.Server.DeployPath))
	configDir := path.Dir(expandPath(cfg.Server.ConfigPath))

	if err := run("ssh", cfg.Server.SSHTarget, "mkdir -p "+deployDir); err != nil {
		return fmt.Errorf("failed to create deploy directory: %w", err)
	}

	if err := run("ssh", cfg.Server.SSHTarget, "mkdir -p "+configDir); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Copy binary (use scpPath to handle ~ properly)
	if err := run("scp", binaryPath, cfg.Server.SSHTarget+":"+scpPath(cfg.Server.DeployPath)); err != nil {
		return fmt.Errorf("failed to copy binary: %w", err)
	}

	// Make executable
	if err := run("ssh", cfg.Server.SSHTarget, "chmod +x "+cfg.Server.DeployPath); err != nil {
		return fmt.Errorf("failed to chmod: %w", err)
	}

	// Write config file
	configCmd := fmt.Sprintf("cat > %s << 'OTTOMAN_CONFIG_EOF'\n%sOTTOMAN_CONFIG_EOF", cfg.Server.ConfigPath, string(serverConfig))
	if err := run("ssh", cfg.Server.SSHTarget, configCmd); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	// Install systemd service
	installCmd := fmt.Sprintf("sudo %s server install", cfg.Server.DeployPath)
	if err := run("ssh", cfg.Server.SSHTarget, installCmd); err != nil {
		return fmt.Errorf("failed to install service: %w", err)
	}

	// Restart service
	if err := run("ssh", cfg.Server.SSHTarget, "sudo systemctl restart ottoman-server"); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	fmt.Println("\n=== Server deployment complete! ===")
	fmt.Printf("\nTo check status: ssh %s 'sudo systemctl status ottoman-server'\n", cfg.Server.SSHTarget)
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
