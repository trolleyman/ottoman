package store

import (
	"path/filepath"
	"testing"
)

func TestRegistryEnsureAndPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")

	r, err := NewRegistry(path)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	e, err := r.Ensure("LG:TV:1", BackendTV)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if e.Backend != BackendTV {
		t.Fatalf("backend = %q, want tv", e.Backend)
	}

	// Reload from disk: the entry should persist.
	r2, err := NewRegistry(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	got, ok := r2.Get("LG:TV:1")
	if !ok || got.Backend != BackendTV {
		t.Fatalf("persisted entry missing/wrong: %+v ok=%v", got, ok)
	}
}

func TestRegistryUpdate(t *testing.T) {
	r, err := NewRegistry(filepath.Join(t.TempDir(), "registry.json"))
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	_, err = r.Update("DEL:U2717:9", func(e *MonitorEntry) {
		e.FriendlyName = "Dell 27\""
		e.Backend = BackendDDC
		e.Visibility = map[string]bool{ControlPower: false}
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	e, ok := r.Get("DEL:U2717:9")
	if !ok {
		t.Fatal("entry missing after update")
	}
	if e.FriendlyName != "Dell 27\"" || e.Backend != BackendDDC {
		t.Fatalf("unexpected entry: %+v", e)
	}
	if e.Visible(ControlPower) {
		t.Error("power should be hidden")
	}
	if !e.Visible(ControlBrightness) {
		t.Error("brightness should default to visible")
	}
}
