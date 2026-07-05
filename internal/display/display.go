package display

import (
	"slices"

	"github.com/trolleyman/ottoman/internal/api"
)

// Manager handles display configuration switching
type Manager interface {
	ListMonitors() ([]api.Monitor, error)
	ApplyLayoutConfig(layout api.Layout) error
}

// Layouts manages display layout configurations
type Layouts struct {
	layouts map[string]api.Layout
}

func NewLayouts() *Layouts {
	return &Layouts{
		layouts: make(map[string]api.Layout),
	}
}

// NewLayoutsFromSlice creates a Layouts store from a slice of layouts
func NewLayoutsFromSlice(layouts []api.Layout) *Layouts {
	s := &Layouts{
		layouts: make(map[string]api.Layout),
	}
	for _, layout := range layouts {
		s.layouts[layout.Id] = layout
	}
	return s
}

// ToSlice returns all layouts as a slice (for saving to config)
func (s *Layouts) ToSlice() []api.Layout {
	layouts := make([]api.Layout, 0, len(s.layouts))
	for _, layout := range s.layouts {
		layouts = append(layouts, layout)
	}
	return layouts
}

// Get returns a layout by id
func (s *Layouts) Get(id string) (api.Layout, bool) {
	layout, ok := s.layouts[id]
	return layout, ok
}

// List returns all layouts
func (s *Layouts) List() []api.Layout {
	var layouts = make([]api.Layout, 0, len(s.layouts))
	for _, layout := range s.layouts {
		layouts = append(layouts, layout)
	}
	return layouts
}

// Set adds or updates a layout
func (s *Layouts) Set(layout api.Layout) {
	s.layouts[layout.Id] = layout
}

// Delete removes a layout
func (s *Layouts) Delete(id string) {
	delete(s.layouts, id)
}

// FindByIDOrAlias returns the layout matching the given ID, or a list of layouts with that alias
func (s *Layouts) FindByIDOrAlias(query string) []api.Layout {
	var matches []api.Layout
	for _, layout := range s.layouts {
		// Check ID
		if layout.Id == query {
			return []api.Layout{layout}
		}
		// Check aliases
		if slices.Contains(layout.Aliases, query) {
			matches = append(matches, layout)
		}
	}
	return matches
}

// GetClosest returns the layout that matches the provided monitors
func (s *Layouts) GetClosest(monitors []api.Monitor) (string, bool) {
	for _, layout := range s.layouts {
		if matches(monitors, layout) {
			return layout.Id, true
		}
	}
	return "", false
}

func matches(monitors []api.Monitor, layout api.Layout) bool {
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
			if lm.Edid != "" {
				if lm.Edid != m.Edid {
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

// AliasOwner returns the ID of the layout that already claims the given alias,
// ignoring the layout identified by exceptID. Empty string means the alias
// (or matching layout ID) is free to use.
func (s *Layouts) AliasOwner(alias, exceptID string) string {
	for id, layout := range s.layouts {
		if id == exceptID {
			continue
		}
		if id == alias || slices.Contains(layout.Aliases, alias) {
			return id
		}
	}
	return ""
}

// UpdateMeta updates a layout's editable metadata (name, emoji, aliases) in
// place, preserving its monitors. Any nil field is left unchanged; a non-nil
// emoji of "" clears it. Returns the updated layout and whether it existed.
func (s *Layouts) UpdateMeta(id string, name, emoji *string, aliases *[]string) (api.Layout, bool) {
	layout, ok := s.layouts[id]
	if !ok {
		return api.Layout{}, false
	}
	if name != nil {
		layout.Name = *name
	}
	if emoji != nil {
		layout.Emoji = emoji
	}
	if aliases != nil {
		layout.Aliases = *aliases
	}
	s.layouts[id] = layout
	return layout, true
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
