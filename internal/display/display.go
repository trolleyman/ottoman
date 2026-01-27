package display

import (
	"encoding/json"
	"os"

	"github.com/pkg/errors"
	"github.com/trolleyman/ottoman/internal/common"
)

// Manager handles display configuration switching
type Manager interface {
	// ListMonitors returns information about connected monitors
	ListMonitors() ([]MonitorInfo, error)

	// GetCurrentLayout attempts to identify the current layout
	GetCurrentLayout() (string, error)

	// ApplyLayout applies a named layout configuration
	ApplyLayout(name string) error

	// ApplyLayoutConfig applies a specific layout configuration
	ApplyLayoutConfig(layout common.SimplifiedLayout) error
}

// MonitorInfo contains information about a connected monitor
type MonitorInfo struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Manufacturer string  `json:"manufacturer,omitempty"`
	Model        string  `json:"model,omitempty"`
	Width        int     `json:"width"`
	Height       int     `json:"height"`
	RefreshRate  float64 `json:"refresh_rate"`
	PositionX    int     `json:"position_x"`
	PositionY    int     `json:"position_y"`
	Primary      bool    `json:"primary"`
	Connected    bool    `json:"connected"`
}

// LayoutStore manages display layout configurations
type LayoutStore struct {
	layouts map[string]common.SimplifiedLayout
	file    string
}

// NewLayoutStore creates a new layout store from a file
func NewLayoutStore(file string) (*LayoutStore, error) {
	store := &LayoutStore{
		layouts: make(map[string]common.SimplifiedLayout),
		file:    file,
	}

	if err := store.Load(); err != nil {
		return nil, err
	}

	return store, nil
}

// Load reads layouts from the configuration file
func (s *LayoutStore) Load() error {
	data, err := os.ReadFile(s.file)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty store if file doesn't exist
			return nil
		}
		return errors.Wrap(err, "failed to read layouts file")
	}

	var layouts []common.SimplifiedLayout
	if err := json.Unmarshal(data, &layouts); err != nil {
		return errors.Wrap(err, "failed to parse layouts file")
	}

	for _, layout := range layouts {
		s.layouts[layout.Name] = layout
	}

	return nil
}

// Save writes layouts to the configuration file
func (s *LayoutStore) Save() error {
	layouts := make([]common.SimplifiedLayout, 0, len(s.layouts))
	for _, layout := range s.layouts {
		layouts = append(layouts, layout)
	}

	data, err := json.MarshalIndent(layouts, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal layouts")
	}

	if err := os.WriteFile(s.file, data, 0644); err != nil {
		return errors.Wrap(err, "failed to write layouts file")
	}

	return nil
}

// Get returns a layout by name
func (s *LayoutStore) Get(name string) (common.SimplifiedLayout, bool) {
	layout, ok := s.layouts[name]
	return layout, ok
}

// List returns all layout names
func (s *LayoutStore) List() []string {
	names := make([]string, 0, len(s.layouts))
	for name := range s.layouts {
		names = append(names, name)
	}
	return names
}

// Set adds or updates a layout
func (s *LayoutStore) Set(layout common.SimplifiedLayout) {
	s.layouts[layout.Name] = layout
}

// Delete removes a layout
func (s *LayoutStore) Delete(name string) {
	delete(s.layouts, name)
}

// NewManager creates a platform-specific display manager
// This is implemented in platform-specific files (windows.go, linux.go)
func NewManager(store *LayoutStore) (Manager, error) {
	return newPlatformManager(store)
}
