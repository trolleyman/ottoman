//go:build mage

package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

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

// buildTarget compiles for a specific target.
func buildTarget(target BuildTarget, version string) error {
	fmt.Printf("Building for %s (%s/%s)...\n", target.Name, target.GOOS, target.GOARCH)

	ldflags := fmt.Sprintf("-X main.Version=%s", version)
	outputPath := filepath.Join(buildDir, target.Output)

	env := map[string]string{
		"GOOS":   target.GOOS,
		"GOARCH": target.GOARCH,
	}
	if target.GOARM != "" {
		env["GOARM"] = target.GOARM
	}

	err := sh.RunWith(env, "go", "build", "-ldflags", ldflags, "-o", outputPath, "./cmd/ottoman")
	if err != nil {
		return err
	}

	fmt.Printf("Built: %s\n", outputPath)
	return nil
}

// Build builds for the current platform.
func Build() error {
	mg.Deps(ensureBuildDir)

	version := getVersion()
	fmt.Printf("Version: %s\n\n", version)

	ldflags := fmt.Sprintf("-X main.Version=%s", version)
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	outputPath := filepath.Join(buildDir, binary+ext)

	err := sh.Run("go", "build", "-ldflags", ldflags, "-o", outputPath, "./cmd/ottoman")
	if err != nil {
		return err
	}

	fmt.Printf("Built: %s\n", outputPath)
	fmt.Println("\nBuild complete!")
	return nil
}

// BuildAll builds for all platforms.
func BuildAll() error {
	mg.Deps(ensureBuildDir)

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
	mg.Deps(ensureBuildDir)

	version := getVersion()
	fmt.Printf("Version: %s\n\n", version)

	return buildTarget(targets["pi"], version)
}

// BuildWindows builds for Windows (windows/amd64).
func BuildWindows() error {
	mg.Deps(ensureBuildDir)

	version := getVersion()
	fmt.Printf("Version: %s\n\n", version)

	return buildTarget(targets["windows"], version)
}

// BuildLinux builds for Linux desktop (linux/amd64).
func BuildLinux() error {
	mg.Deps(ensureBuildDir)

	version := getVersion()
	fmt.Printf("Version: %s\n\n", version)

	return buildTarget(targets["linux"], version)
}

// Clean removes build artifacts.
func Clean() error {
	fmt.Println("Cleaning build artifacts...")

	if err := sh.Rm(buildDir); err != nil {
		// Ignore error if directory doesn't exist
		if !os.IsNotExist(err) {
			return err
		}
	}

	if err := sh.Run("go", "clean"); err != nil {
		return err
	}

	fmt.Println("Clean complete!")
	return nil
}

// Test runs tests.
func Test() error {
	fmt.Println("Running tests...\n")

	err := sh.RunV("go", "test", "-v", "./...")
	if err != nil {
		return err
	}

	fmt.Println("\nTests complete!")
	return nil
}

// Lint runs the linter.
func Lint() error {
	fmt.Println("Running linter...\n")

	// Check if golangci-lint is available
	_, err := exec.LookPath("golangci-lint")
	if err != nil {
		fmt.Println("golangci-lint not found, using go vet instead...\n")
		err = sh.RunV("go", "vet", "./...")
	} else {
		err = sh.RunV("golangci-lint", "run")
	}

	if err != nil {
		return err
	}

	fmt.Println("\nLint complete!")
	return nil
}

// RunServer runs the server locally.
func RunServer() error {
	fmt.Println("Running server...\n")
	return sh.RunV("go", "run", "./cmd/ottoman", "server", "run")
}

// RunClient runs the client locally.
func RunClient() error {
	fmt.Println("Running client...\n")
	return sh.RunV("go", "run", "./cmd/ottoman", "client", "run")
}

// DeployPi deploys the server to Raspberry Pi.
// Usage: mage deploypi user@host
func DeployPi(host string) error {
	if host == "" {
		return fmt.Errorf("usage: mage deploypi user@raspberrypi.local")
	}

	fmt.Printf("Deploying to Raspberry Pi (%s)...\n\n", host)

	// Build for Pi first
	fmt.Println("Building for Raspberry Pi...")
	if err := BuildPi(); err != nil {
		return err
	}

	binaryPath := filepath.Join(buildDir, "ottoman-linux-arm")

	// Copy to target
	fmt.Printf("\nCopying to %s...\n", host)
	if err := sh.Run("scp", binaryPath, host+":/tmp/ottoman"); err != nil {
		return err
	}

	// Install on target
	fmt.Println("\nInstalling on target...")
	installScript := `sudo mv /tmp/ottoman /usr/local/bin/ottoman && sudo chmod +x /usr/local/bin/ottoman && sudo /usr/local/bin/ottoman server install && sudo systemctl restart ottoman-server`

	if err := sh.Run("ssh", host, installScript); err != nil {
		return err
	}

	fmt.Println("\nDeployment complete!")
	return nil
}

// DeployClient deploys the client locally (alias for Install).
func DeployClient() error {
	return Install()
}

// Install builds and installs ottoman to the system location.
// On Windows: %LOCALAPPDATA%\ottoman\ottoman.exe
// On Linux:   ~/.local/bin/ottoman
func Install() error {
	fmt.Println("Building and installing ottoman...\n")

	// Build for current platform
	if err := Build(); err != nil {
		return err
	}

	// Run install command
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	binaryPath := filepath.Join(buildDir, binary+ext)

	if err := sh.RunV(binaryPath, "install"); err != nil {
		return err
	}

	return nil
}

// Deps installs Go dependencies.
func Deps() error {
	fmt.Println("Installing Go dependencies...\n")

	if err := sh.Run("go", "mod", "tidy"); err != nil {
		return err
	}

	fmt.Println("\nDependencies installed!")
	return nil
}
