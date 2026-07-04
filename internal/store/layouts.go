package store

import (
	"encoding/json"
	"os"

	"github.com/pkg/errors"
	"github.com/trolleyman/ottoman/internal/api"
)

// LayoutStore persists display layouts as JSON in the data directory.
type LayoutStore struct {
	path string
}

// NewLayoutStore returns a store backed by the given path. If path is empty,
// the default LayoutsPath() is used.
func NewLayoutStore(path string) *LayoutStore {
	if path == "" {
		path = LayoutsPath()
	}
	return &LayoutStore{path: path}
}

// Path returns the file path backing the store.
func (s *LayoutStore) Path() string {
	return s.path
}

// Exists reports whether the store file exists on disk.
func (s *LayoutStore) Exists() bool {
	_, err := os.Stat(s.path)
	return err == nil
}

// Load reads the stored layouts. A missing file is not an error and yields an
// empty slice.
func (s *LayoutStore) Load() ([]api.Layout, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []api.Layout{}, nil
		}
		return nil, errors.Wrap(err, "failed to read layouts store")
	}

	var layouts []api.Layout
	if len(data) > 0 {
		if err := json.Unmarshal(data, &layouts); err != nil {
			return nil, errors.Wrap(err, "failed to parse layouts store")
		}
	}
	if layouts == nil {
		layouts = []api.Layout{}
	}
	return layouts, nil
}

// Save writes the layouts atomically.
func (s *LayoutStore) Save(layouts []api.Layout) error {
	if layouts == nil {
		layouts = []api.Layout{}
	}
	data, err := json.MarshalIndent(layouts, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal layouts")
	}
	return writeAtomic(s.path, data)
}

// LoadWithMigration loads layouts from the store. If the store file does not
// yet exist and migrateFrom is non-empty (e.g. legacy agent.layouts from the
// config file), those layouts are imported into the store and returned. The
// config file is left untouched.
func (s *LayoutStore) LoadWithMigration(migrateFrom []api.Layout) ([]api.Layout, error) {
	if !s.Exists() {
		if len(migrateFrom) > 0 {
			if err := s.Save(migrateFrom); err != nil {
				return nil, errors.Wrap(err, "failed to migrate layouts into store")
			}
			return migrateFrom, nil
		}
		return []api.Layout{}, nil
	}
	return s.Load()
}
