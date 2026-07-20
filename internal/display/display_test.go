package display

import (
	"testing"

	"github.com/trolleyman/ottoman/internal/api"
)

// activeMonitor builds a connected, active monitor for matching tests.
func activeMonitor(edid string, w, h, x, y int, primary bool, scale, rate float64) api.Monitor {
	return api.Monitor{
		Edid: edid,
		Active: &api.ActiveMonitor{
			Width: w, Height: h, PositionX: x, PositionY: y,
			Primary: primary, Scale: scale, RefreshRate: rate,
		},
	}
}

func layoutMonitor(edid string, w, h, x, y int, primary bool, scale, rate float64) api.LayoutMonitor {
	return api.LayoutMonitor{
		Edid: edid, Width: w, Height: h, PositionX: x, PositionY: y,
		Primary: primary, Scale: scale, RefreshRate: rate,
	}
}

// Two layouts placing the same monitors identically but differing in which one
// is primary must not be confused for each other.
func TestMatchesDistinguishesPrimary(t *testing.T) {
	monitors := []api.Monitor{
		activeMonitor("AOC", 2560, 1440, 0, 0, false, 1, 60),
		activeMonitor("LG", 3840, 2160, 2560, 0, true, 2, 60),
	}
	tvPrimary := api.Layout{Id: "with-tv-primary", Monitors: []api.LayoutMonitor{
		layoutMonitor("AOC", 2560, 1440, 0, 0, false, 1, 60),
		layoutMonitor("LG", 3840, 2160, 2560, 0, true, 2, 60),
	}}
	aocPrimary := api.Layout{Id: "with-tv", Monitors: []api.LayoutMonitor{
		layoutMonitor("AOC", 2560, 1440, 0, 0, true, 1, 60),
		layoutMonitor("LG", 3840, 2160, 2560, 0, false, 2, 60),
	}}

	if !matches(monitors, tvPrimary) {
		t.Error("layout with the same primary should match")
	}
	if matches(monitors, aocPrimary) {
		t.Error("layout with a different primary must not match")
	}
}

func TestMatchesDistinguishesScaleAndRefresh(t *testing.T) {
	monitors := []api.Monitor{activeMonitor("LG", 3840, 2160, 0, 0, true, 2, 60)}

	same := api.Layout{Id: "tv-200", Monitors: []api.LayoutMonitor{
		layoutMonitor("LG", 3840, 2160, 0, 0, true, 2, 60)}}
	otherScale := api.Layout{Id: "tv-100", Monitors: []api.LayoutMonitor{
		layoutMonitor("LG", 3840, 2160, 0, 0, true, 1, 60)}}
	otherRate := api.Layout{Id: "tv-120", Monitors: []api.LayoutMonitor{
		layoutMonitor("LG", 3840, 2160, 0, 0, true, 2, 120)}}

	if !matches(monitors, same) {
		t.Error("identical layout should match")
	}
	if matches(monitors, otherScale) {
		t.Error("a layout at a different scale must not match")
	}
	if matches(monitors, otherRate) {
		t.Error("a layout at a different refresh rate must not match")
	}
}

// Layouts saved before scale was recorded (scale 0) must still match an
// unscaled display, and float noise in refresh rates must not break matching.
func TestMatchesToleratesUnsetScaleAndRateNoise(t *testing.T) {
	monitors := []api.Monitor{activeMonitor("PHL", 1920, 1080, 0, 0, true, 1, 59.93939208984375)}
	legacy := api.Layout{Id: "old", Monitors: []api.LayoutMonitor{
		layoutMonitor("PHL", 1920, 1080, 0, 0, true, 0, 59.939)}}

	if !matches(monitors, legacy) {
		t.Error("a pre-scale layout with a slightly different rate should still match")
	}
}

// When several layouts match, the answer must be stable and must respect the
// layout the user actually switched to.
func TestGetClosestPrefersCurrentAndIsDeterministic(t *testing.T) {
	// Two layouts that are genuinely indistinguishable from the display.
	a := api.Layout{Id: "aaa", Monitors: []api.LayoutMonitor{
		layoutMonitor("LG", 3840, 2160, 0, 0, true, 2, 60)}}
	b := api.Layout{Id: "zzz", Monitors: []api.LayoutMonitor{
		layoutMonitor("LG", 3840, 2160, 0, 0, true, 2, 60)}}
	store := NewLayoutsFromSlice([]api.Layout{a, b})
	monitors := []api.Monitor{activeMonitor("LG", 3840, 2160, 0, 0, true, 2, 60)}

	if got, ok := store.GetClosest(monitors, "zzz"); !ok || got != "zzz" {
		t.Errorf("GetClosest should honour the preferred layout, got %q", got)
	}
	// With no preference the result must still be stable across many calls,
	// rather than following Go's randomised map iteration order.
	for i := 0; i < 50; i++ {
		got, ok := store.GetClosest(monitors, "")
		if !ok || got != "aaa" {
			t.Fatalf("GetClosest should be deterministic, got %q on iteration %d", got, i)
		}
	}
	// A preference that doesn't match is ignored.
	if got, ok := store.GetClosest(monitors, "nonexistent"); !ok || got != "aaa" {
		t.Errorf("an unmatched preference should fall back to the stable choice, got %q", got)
	}
}

// Re-capturing a layout replaces its geometry but must preserve its identity
// and metadata — the whole point of updating in place rather than re-saving.
func TestSetMonitorsPreservesMetadata(t *testing.T) {
	emoji := "📺"
	original := api.Layout{
		Id: "with-tv", Name: "With TV", Emoji: &emoji, Aliases: []string{"3"},
		Monitors: []api.LayoutMonitor{layoutMonitor("LG", 3840, 2160, 0, 0, true, 1, 60)},
	}
	store := NewLayoutsFromSlice([]api.Layout{original})

	fresh := []api.LayoutMonitor{
		layoutMonitor("LG", 3840, 2160, 0, 0, false, 2, 120),
		layoutMonitor("AOC", 2560, 1440, 1920, 0, true, 1, 60),
	}
	got, ok := store.SetMonitors("with-tv", fresh)
	if !ok {
		t.Fatal("SetMonitors should find the layout")
	}
	if len(got.Monitors) != 2 || got.Monitors[0].Scale != 2 || !got.Monitors[1].Primary {
		t.Errorf("monitors were not replaced: %+v", got.Monitors)
	}
	if got.Id != "with-tv" || got.Name != "With TV" || got.Emoji == nil || *got.Emoji != "📺" {
		t.Errorf("identity/metadata not preserved: %+v", got)
	}
	if len(got.Aliases) != 1 || got.Aliases[0] != "3" {
		t.Errorf("aliases not preserved: %+v", got.Aliases)
	}
	// The change must be persisted in the store, not just returned.
	if stored, _ := store.Get("with-tv"); len(stored.Monitors) != 2 {
		t.Errorf("store not updated: %+v", stored.Monitors)
	}
	if _, ok := store.SetMonitors("nope", fresh); ok {
		t.Error("SetMonitors should report a missing layout")
	}
}
