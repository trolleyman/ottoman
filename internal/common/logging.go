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
	"time"

	"github.com/pkg/errors"
)

// LoggingMiddleware logs HTTP requests with method, path, status, and duration
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := &logResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(lw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, lw.status, time.Since(start).Round(time.Microsecond))
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
	log.Printf("Running: %s", FormatCmd(name, args...))
	cmd := exec.Command(name, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	logOutput("[stdout]", stdout.String())
	logOutput("[stderr]", stderr.String())

	if err != nil {
		return errors.Wrapf(err, "failed to run %s", name)
	}
	return nil
}

// RunCmdOutput executes a command, logging it, and returns stdout.
// Stderr is logged. Returns stdout and any error.
func RunCmdOutput(name string, args ...string) (string, error) {
	log.Printf("Running: %s", FormatCmd(name, args...))
	cmd := exec.Command(name, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	logOutput("[stderr]", stderr.String())
	logOutput("[stdout]", stdout.String())

	if err != nil {
		return stdout.String(), errors.Wrapf(err, "failed to run %s", name)
	}
	return stdout.String(), nil
}

// RunCmdAllOutput executes a command, logging it, and returns stdout and stderr.
func RunCmdAllOutput(name string, args ...string) (string, string, error) {
	log.Printf("Running: %s", FormatCmd(name, args...))
	cmd := exec.Command(name, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	logOutput("[stderr]", stderr.String())
	logOutput("[stdout]", stdout.String())

	if err != nil {
		return stdout.String(), stderr.String(), errors.Wrapf(err, "failed to run %s", name)
	}
	return stdout.String(), stderr.String(), nil
}

// RunCmdSilent executes a command, logging it but ignoring errors.
// Useful for cleanup commands where failure is expected.
func RunCmdSilent(name string, args ...string) {
	log.Printf("Running: %s", FormatCmd(name, args...))
	cmd := exec.Command(name, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	logOutput("[stdout]", stdout.String())
	logOutput("[stderr]", stderr.String())

	if err != nil {
		log.Printf("  (ignored error: %v)", err)
	}
}
