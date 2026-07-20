package display

import (
	"math"
	"slices"
	"sort"

	"github.com/trolleyman/ottoman/internal/api"
)

// Manager handles display configuration switching
type Manager interface {
	ListMonitors() ([]api.Monitor, error)
	ApplyLayoutConfig(layout api.Layout) error
}

// LayoutApplyOutcome describes what actually happened to the display. A display
// server accepting an apply request is not proof the layout stuck: Mutter can
// report success and then roll the configuration back a second or two later, and
// it can also report a layout as already active while the screen shows something
// else. Reporting a bare "success" in those cases is actively misleading.
type LayoutApplyOutcome string

const (
	// OutcomeApplied means the display changed and now matches the layout.
	OutcomeApplied LayoutApplyOutcome = "applied"
	// OutcomeAlreadyActive means the display server already considered this
	// layout active, so nothing changed. If the screen disagrees, the display
	// server's state has drifted from reality.
	OutcomeAlreadyActive LayoutApplyOutcome = "already-active"
	// OutcomeRolledBack means the layout applied and was then reverted by the
	// display server.
	OutcomeRolledBack LayoutApplyOutcome = "rolled-back"
	// OutcomeMismatch means the request was accepted but the display never
	// matched the layout.
	OutcomeMismatch LayoutApplyOutcome = "mismatch"
	// OutcomeUnverified means the resulting state could not be read back.
	OutcomeUnverified LayoutApplyOutcome = "unverified"
)

// Ok reports whether the outcome means the layout is actually on screen.
func (o LayoutApplyOutcome) Ok() bool {
	return o == OutcomeApplied || o == OutcomeAlreadyActive || o == OutcomeUnverified
}

// LayoutApplyResult reports the verified outcome of applying a layout.
type LayoutApplyResult struct {
	Outcome LayoutApplyOutcome
	Detail  string
}

// VerifyingManager is implemented by display backends that can confirm what
// actually happened to the display after an apply, rather than just reporting
// that the request was accepted.
type VerifyingManager interface {
	ApplyLayoutConfigVerified(layout api.Layout) (LayoutApplyResult, error)
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

// GetClosest returns the layout matching the provided monitors.
//
// Several layouts can legitimately match at once (two that differ only in a
// field the display can't distinguish). Iterating the layout map directly made
// the winner depend on Go's randomised map order, so the reported current layout
// could differ between calls. When there's a tie, prefer is returned if it's
// among the matches — the layout the user actually switched to is the right
// answer — and otherwise the lowest ID wins so the result is at least stable.
func (s *Layouts) GetClosest(monitors []api.Monitor, prefer string) (string, bool) {
	var found []string
	for id, layout := range s.layouts {
		if matches(monitors, layout) {
			found = append(found, id)
		}
	}
	if len(found) == 0 {
		return "", false
	}
	if prefer != "" && slices.Contains(found, prefer) {
		return prefer, true
	}
	sort.Strings(found)
	return found[0], true
}

// sameScale compares scale factors, treating an unset (0) scale as 100% so
// layouts saved before scale was recorded still match an unscaled display.
func sameScale(a, b float64) bool {
	if a <= 0 {
		a = 1
	}
	if b <= 0 {
		b = 1
	}
	return math.Abs(a-b) < 1e-6
}

// sameRefreshRate compares refresh rates loosely. Modes report rates like
// 59.93939208984375, and a layout may have been applied via the closest
// available mode, so an exact comparison would spuriously fail to match. The
// tolerance is wide enough to absorb that but still separates 60Hz from 144Hz.
func sameRefreshRate(a, b float64) bool {
	if a <= 0 || b <= 0 {
		return true // unknown on either side: don't let it veto a match
	}
	return math.Abs(a-b) < 0.5
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
			// Geometry alone doesn't identify a layout: two layouts can place the
			// same monitors identically and differ only in which one is primary,
			// their scale, or their refresh rate. Without these, such layouts are
			// indistinguishable and the wrong one gets reported as current.
			if lm.Primary != m.Active.Primary {
				continue
			}
			if !sameScale(lm.Scale, m.Active.Scale) {
				continue
			}
			if !sameRefreshRate(lm.RefreshRate, m.Active.RefreshRate) {
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
