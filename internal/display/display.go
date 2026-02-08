package display

import (
	"slices"

	"github.com/trolleyman/ottoman/internal/common"
)

// Manager handles display configuration switching
type Manager interface {
	ListMonitors() ([]MonitorInfo, error)
	ApplyLayoutConfig(layout common.Layout) error
}

// MonitorInfo contains information about a monitor (connected or not)
type MonitorInfo struct {
	// Identification
	EDID string `json:"edid,omitempty"` // EDID "MANUFACTURER:PRODUCT" e.g., "DEL:D0A2"
	Port string `json:"port"`           // Port/connector name e.g., "HDMI-1", "DP-1"

	// Display info
	Name         string `json:"name,omitempty"`
	Manufacturer string `json:"manufacturer,omitempty"`

	// Active configuration (nil if monitor is not active/connected)
	Active *ConnectedInfo `json:"active,omitempty"`
}

// ConnectedInfo contains the configuration of an active monitor
type ConnectedInfo struct {
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	RefreshRate float64 `json:"refresh_rate"`
	PositionX   int     `json:"position_x"`
	PositionY   int     `json:"position_y"`
	Primary     bool    `json:"primary"`
	Model       string  `json:"model,omitempty"`
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

// FindByIDOrAlias returns the layout matching the given ID, or a list of layouts with that alias
func (s *Layouts) FindByIDOrAlias(query string) []common.Layout {
	var matches []common.Layout
	for _, layout := range s.layouts {
		// Check ID
		if layout.ID == query {
			return []common.Layout{layout}
		}
		// Check aliases
		if slices.Contains(layout.Aliases, query) {
			matches = append(matches, layout)
		}
	}
	return matches
}

// GetClosest returns the layout that matches the provided monitors
func (s *Layouts) GetClosest(monitors []MonitorInfo) (string, bool) {
	for _, layout := range s.layouts {
		if matches(monitors, layout) {
			return layout.ID, true
		}
	}
	return "", false
}

func matches(monitors []MonitorInfo, layout common.Layout) bool {
	// Count active monitors (connected and configured)
	activeMonitorsCount := 0
	for _, m := range monitors {
		if m.Active != nil {
			activeMonitorsCount++
		}
	}

	if len(layout.Monitors) != activeMonitorsCount {
		return false
	}

	used := make([]bool, len(monitors))

	// Check each layout monitor matches a physical monitor
	for _, lm := range layout.Monitors {
		found := false
		for i, m := range monitors {
			if m.Active == nil || used[i] {
				continue
			}

			// Match by EDID first, then by port
			if lm.EDID != "" {
				if lm.EDID != m.EDID {
					continue
				}
			} else if lm.Port != "" {
				if lm.Port != m.Port {
					continue
				}
			}

			// Check geometry
			if lm.Width != m.Active.Width || lm.Height != m.Active.Height {
				continue
			}
			if lm.PositionX != m.Active.PositionX || lm.PositionY != m.Active.PositionY {
				continue
			}

			used[i] = true
			found = true
			break
		}
		if !found {
			return false
		}
	}

	return true
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
