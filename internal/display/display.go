package display

import (
	"encoding/json"
	"os"

	"github.com/pkg/errors"
	"github.com/trolleyman/ottoman/internal/common"
)

// Manager handles display configuration switching
type Manager interface {
	ListMonitors() ([]MonitorInfo, error)
	GetCurrentLayout(layouts *Layouts) (string, error)
	ApplyLayoutConfig(layout common.Layout) error
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

// Layouts manages display layout configurations
type Layouts struct {
	layouts map[string]common.Layout
}

func NewLayouts() *Layouts {
	return &Layouts{
		layouts: make(map[string]common.Layout),
	}
}

// Load reads layouts from the configuration file
func (s *Layouts) Load(file string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty store if file doesn't exist
			return nil
		}
		return errors.Wrap(err, "failed to read layouts file")
	}

	var layouts []common.Layout
	if err := json.Unmarshal(data, &layouts); err != nil {
		return errors.Wrap(err, "failed to parse layouts file")
	}

	for _, layout := range layouts {
		s.layouts[layout.Name] = layout
	}

	return nil
}

// Save writes layouts to the configuration file
func (s *Layouts) Save(file string) error {
	layouts := make([]common.Layout, 0, len(s.layouts))
	for _, layout := range s.layouts {
		layouts = append(layouts, layout)
	}

	data, err := json.MarshalIndent(layouts, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal layouts")
	}

	if err := os.WriteFile(file, data, 0644); err != nil {
		return errors.Wrap(err, "failed to write layouts file")
	}

	return nil
}

// Get returns a layout by id
func (s *Layouts) Get(id string) (common.Layout, bool) {
	layout, ok := s.layouts[id]
	return layout, ok
}

// List returns all layouts
func (s *Layouts) List() []common.Layout {
	var layouts []common.Layout
	for _, layout := range s.layouts {
		layouts = append(layouts, layout)
	}
	return layouts
}

// Set adds or updates a layout
func (s *Layouts) Set(layout common.Layout) {
	s.layouts[layout.ID] = layout
}

// Delete removes a layout
func (s *Layouts) Delete(id string) {
	delete(s.layouts, id)
}

// NewManager creates a platform-specific display manager
// This is implemented in platform-specific files (windows.go, linux.go)
func NewManager(store *Layouts) (Manager, error) {
	return newPlatformManager(store)
}
