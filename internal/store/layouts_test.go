package store

import (
	"path/filepath"
	"testing"

	"github.com/trolleyman/ottoman/internal/api"
)

func layout(id string) api.Layout {
	return api.Layout{Id: id, Name: id, Aliases: []string{}}
}

func TestLayoutStoreSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "layouts.json")
	s := NewLayoutStore(path)

	want := []api.Layout{layout("a"), layout("b")}
	if err := s.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d layouts, want %d", len(got), len(want))
	}
	if got[0].Id != "a" || got[1].Id != "b" {
		t.Fatalf("unexpected layouts: %+v", got)
	}
}

func TestLayoutStoreLoadMissingIsEmpty(t *testing.T) {
	s := NewLayoutStore(filepath.Join(t.TempDir(), "nope.json"))
	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty, got %+v", got)
	}
}

func TestLoadWithMigrationImportsOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "layouts.json")
	s := NewLayoutStore(path)

	legacy := []api.Layout{layout("legacy")}

	// First run: store missing -> migrate from config.
	got, err := s.LoadWithMigration(legacy)
	if err != nil {
		t.Fatalf("LoadWithMigration: %v", err)
	}
	if len(got) != 1 || got[0].Id != "legacy" {
		t.Fatalf("expected migrated layout, got %+v", got)
	}
	if !s.Exists() {
		t.Fatal("store file should exist after migration")
	}

	// Second run: store now exists, so config layouts are ignored even if they
	// differ (simulating a stale/removed config value).
	got2, err := s.LoadWithMigration([]api.Layout{layout("different")})
	if err != nil {
		t.Fatalf("LoadWithMigration (2): %v", err)
	}
	if len(got2) != 1 || got2[0].Id != "legacy" {
		t.Fatalf("store should win over config after migration, got %+v", got2)
	}
}

func TestLoadWithMigrationNoLegacyIsEmpty(t *testing.T) {
	s := NewLayoutStore(filepath.Join(t.TempDir(), "layouts.json"))
	got, err := s.LoadWithMigration(nil)
	if err != nil {
		t.Fatalf("LoadWithMigration: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty, got %+v", got)
	}
	if s.Exists() {
		t.Fatal("store file should not be created when there is nothing to migrate")
	}
}
