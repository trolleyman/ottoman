package display

import (
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
	// Identification
	EDID string `json:"edid,omitempty"` // EDID "MANUFACTURER:PRODUCT" e.g., "DEL:D0A2"
	Port string `json:"port"`           // Port/connector name e.g., "HDMI-1", "DP-1"

	// Display info
	Name         string `json:"name,omitempty"`
	Manufacturer string `json:"manufacturer,omitempty"`
	Model        string `json:"model,omitempty"`

	// Current configuration
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	RefreshRate float64 `json:"refresh_rate"`
	PositionX   int     `json:"position_x"`
	PositionY   int     `json:"position_y"`
	Primary     bool    `json:"primary"`
	Connected   bool    `json:"connected"`
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

// NewLayoutsFromSlice creates a Layouts store from a slice of layouts
func NewLayoutsFromSlice(layouts []common.Layout) *Layouts {
	s := &Layouts{
		layouts: make(map[string]common.Layout),
	}
	for _, layout := range layouts {
		s.layouts[layout.ID] = layout
	}
	return s
}

// ToSlice returns all layouts as a slice (for saving to config)
func (s *Layouts) ToSlice() []common.Layout {
	layouts := make([]common.Layout, 0, len(s.layouts))
	for _, layout := range s.layouts {
		layouts = append(layouts, layout)
	}
	return layouts
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

// FindByIDOrAlias returns layouts matching the given ID, name, or alias
func (s *Layouts) FindByIDOrAlias(query string) []common.Layout {
	var matches []common.Layout
	for _, layout := range s.layouts {
		// Check ID
		if layout.ID == query {
			matches = append(matches, layout)
			continue
		}
		// Check name
		if layout.Name == query {
			matches = append(matches, layout)
			continue
		}
		// Check aliases
		for _, alias := range layout.Aliases {
			if alias == query {
				matches = append(matches, layout)
				break
			}
		}
	}
	return matches
}

// AddAlias adds an alias to a layout
func (s *Layouts) AddAlias(id, alias string) bool {
	layout, ok := s.layouts[id]
	if !ok {
		return false
	}
	// Check if alias already exists
	for _, a := range layout.Aliases {
		if a == alias {
			return true // Already exists
		}
	}
	layout.Aliases = append(layout.Aliases, alias)
	s.layouts[id] = layout
	return true
}

// RemoveAlias removes an alias from a layout
func (s *Layouts) RemoveAlias(id, alias string) bool {
	layout, ok := s.layouts[id]
	if !ok {
		return false
	}
	for i, a := range layout.Aliases {
		if a == alias {
			layout.Aliases = append(layout.Aliases[:i], layout.Aliases[i+1:]...)
			s.layouts[id] = layout
			return true
		}
	}
	return false
}

// NewManager creates a platform-specific display manager
// This is implemented in platform-specific files (windows.go, linux.go)
func NewManager(store *Layouts) (Manager, error) {
	return newPlatformManager(store)
}
