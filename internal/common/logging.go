package common

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
)

// debugLogging enables verbose logging of subprocess invocations and their
// stdout. It's off by default because the audio and DDC pollers shell out every
// few seconds: those dumps accounted for ~93% of a typical log file, burying the
// entries that matter and pushing useful history out through rotation.
//
// HTTP request lines are deliberately not gated on this — they're one line each
// and are the clearest record of what the UI actually asked for.
var debugLogging atomic.Bool

// SetDebugLogging turns verbose logging on or off.
func SetDebugLogging(on bool) { debugLogging.Store(on) }

// DebugLogging reports whether verbose logging is enabled.
func DebugLogging() bool { return debugLogging.Load() }

// LoggingMiddleware logs HTTP requests with method, path, status, and duration
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := &logResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(lw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, lw.status, time.Since(start).Round(time.Microsecond))
	})
}

// HealthCORS adds a permissive CORS header to the unauthenticated /health
// endpoint. The SPA (served from e.g. ottoman.local) probes the desktop's LAN
// IP directly at /health to decide whether to redirect onto the local network;
// that fetch is cross-origin, so without this header the browser blocks the
// response even though the server answers 200.
func HealthCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

type logResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *logResponseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Hijack implements http.Hijacker so WebSocket upgrades work through the logging middleware.
func (w *logResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.ResponseWriter.(http.Hijacker).Hijack()
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
	return r.enforceBackups()
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

// ShellQuoteForce wraps a string in quotes for display as a shell argument.
func ShellQuoteForce(s string) string {
	containsDoubleQuote := strings.Contains(s, `"`)
	containsSingleQuote := strings.Contains(s, `'`)
	escaped := strings.ReplaceAll(s, "\t", `\t`)
	escaped = strings.ReplaceAll(s, `\`, `\\`)
	if !containsDoubleQuote {
		return `"` + escaped + `"`
	} else if !containsSingleQuote {
		return `'` + escaped + `'`
	}
	return `"` + strings.ReplaceAll(escaped, `"`, `\"`) + `"`
}

// ShellQuote wraps a string in quotes for display if it contains whitespace or quotes.
func ShellQuote(s string) string {
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, `"' `+"\t") {
		return ShellQuoteForce(s)
	}
	return s
}

// FormatCmd formats a command and its arguments for display.
func FormatCmd(cmd string, args ...string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, ShellQuote(cmd))
	for _, a := range args {
		parts = append(parts, ShellQuote(a))
	}
	return strings.Join(parts, " ")
}

// logRunning announces a subprocess, but only in debug mode. The audio and DDC
// pollers shell out every few seconds, so these lines are pure noise in normal
// operation; a failing command still surfaces its name via the wrapped error.
func logRunning(name string, args ...string) {
	if !DebugLogging() {
		return
	}
	log.Printf("Running: %s", FormatCmd(name, args...))
}

// logStdout logs a command's stdout, but only in debug mode. Subprocess dumps
// (wpctl inspect and friends) are by far the largest source of log volume and
// are only useful when actively debugging.
func logStdout(output string) {
	if !DebugLogging() {
		return
	}
	logOutput("[stdout]", output)
}

// logOutput logs each line of output with the given prefix.
func logOutput(prefix, output string) {
	out := strings.TrimRight(output, "\n")
	if out == "" {
		// log.Printf("  %s <no output>", prefix)
		return
	}
	for line := range strings.SplitSeq(out, "\n") {
		log.Printf("  %s %s", prefix, line)
	}
}

// RunCmd executes a command, logging it and piping stdout/stderr to the log.
func RunCmd(name string, args ...string) error {
	logRunning(name, args...)
	cmd := exec.Command(name, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	logStdout(stdout.String())
	logOutput("[stderr]", stderr.String())

	if err != nil {
		return errors.Wrapf(err, "failed to run %s", name)
	}
	return nil
}

// RunCmdOutput executes a command, logging it, and returns stdout.
// Stderr is logged. Returns stdout and any error.
func RunCmdOutput(name string, args ...string) (string, error) {
	logRunning(name, args...)
	cmd := exec.Command(name, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	logOutput("[stderr]", stderr.String())
	logStdout(stdout.String())

	if err != nil {
		return stdout.String(), errors.Wrapf(err, "failed to run %s", name)
	}
	return stdout.String(), nil
}

// RunCmdAllOutput executes a command, logging it, and returns stdout and stderr.
func RunCmdAllOutput(name string, args ...string) (string, string, error) {
	logRunning(name, args...)
	cmd := exec.Command(name, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	logOutput("[stderr]", stderr.String())
	logStdout(stdout.String())

	if err != nil {
		return stdout.String(), stderr.String(), errors.Wrapf(err, "failed to run %s", name)
	}
	return stdout.String(), stderr.String(), nil
}

// RunCmdSilent executes a command, logging it but ignoring errors.
// Useful for cleanup commands where failure is expected.
func RunCmdSilent(name string, args ...string) {
	logRunning(name, args...)
	cmd := exec.Command(name, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	logStdout(stdout.String())
	logOutput("[stderr]", stderr.String())

	if err != nil {
		log.Printf("  (ignored error: %v)", err)
	}
}
