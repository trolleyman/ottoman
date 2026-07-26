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

func TestRegistryReplaceTVEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	r, err := NewRegistry(path)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	// A non-TV entry that must survive the replace, plus a TV that must be dropped.
	if _, err := r.Update("DEL:U2717:9", func(e *MonitorEntry) { e.Backend = BackendDDC }); err != nil {
		t.Fatalf("Update ddc: %v", err)
	}
	if _, err := r.Update("LG:OLD:1", func(e *MonitorEntry) {
		e.Backend = BackendTV
		e.TV = &TVConn{Type: "webos", Host: "10.0.0.9"}
	}); err != nil {
		t.Fatalf("Update old tv: %v", err)
	}

	err = r.ReplaceTVEntries([]MonitorEntry{
		{Edid: "LG:NEW:2", FriendlyName: "Living Room", TV: &TVConn{Type: "webos", Host: "10.0.0.5", Mac: "aa:bb"}},
		{Edid: "", TV: &TVConn{Host: "skip.me"}}, // empty EDID is skipped
	})
	if err != nil {
		t.Fatalf("ReplaceTVEntries: %v", err)
	}

	// Reload from disk to confirm persistence + exact mirror semantics.
	r2, err := NewRegistry(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if _, ok := r2.Get("LG:OLD:1"); ok {
		t.Error("old TV entry should have been dropped")
	}
	if ddc, ok := r2.Get("DEL:U2717:9"); !ok || ddc.Backend != BackendDDC {
		t.Errorf("non-TV entry should survive: %+v ok=%v", ddc, ok)
	}
	newTV, ok := r2.Get("LG:NEW:2")
	if !ok {
		t.Fatal("new TV entry missing")
	}
	if newTV.Backend != BackendTV || newTV.TV == nil || newTV.TV.Host != "10.0.0.5" {
		t.Errorf("new TV entry wrong: %+v", newTV)
	}
	if got := r2.TVEntries(); len(got) != 1 {
		t.Errorf("TVEntries = %d, want 1 (%+v)", len(got), got)
	}
}
