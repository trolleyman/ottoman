//go:build linux

package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/trolleyman/ottoman/internal/api"
	"github.com/trolleyman/ottoman/internal/config"
	"github.com/trolleyman/ottoman/internal/store"
)

func TestGreeterStartupDoesNotRestoreSavedTVLayout(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_SESSION_TYPE", "x11")
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("PATH", dir)
	marker := filepath.Join(dir, "display-command")
	t.Setenv("OTTOMAN_TEST_DISPLAY_COMMAND", marker)
	// The old startup path would query xrandr and then attempt the saved TV
	// layout. Neither probing nor applying a layout belongs in construction.
	if err := os.WriteFile(filepath.Join(dir, "xrandr"), []byte("#!/bin/sh\necho called >> \"$OTTOMAN_TEST_DISPLAY_COMMAND\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCurrentLayout("tv"); err != nil {
		t.Fatal(err)
	}
	a, err := NewGreeter(&config.AgentConfig{
		ListenAddress: "127.0.0.1:0", AuthToken: "test",
		Layouts: []api.Layout{{Id: "tv", Monitors: []api.LayoutMonitor{{Port: "HDMI-2", Width: 3840, Height: 2160}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("startup invoked a display command: %v", err)
	}
	if a.currentLayout != "" {
		t.Fatalf("startup claimed saved layout %q was active", a.currentLayout)
	}
	if got := store.LoadCurrentLayout(); got != "tv" {
		t.Fatalf("startup rewrote saved choice to %q", got)
	}
}
