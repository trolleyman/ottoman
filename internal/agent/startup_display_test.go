package agent

import (
	"testing"

	"github.com/trolleyman/ottoman/internal/api"
)

func activeMonitor(edid, name string, w, h, x, y int, primary bool) api.Monitor {
	return api.Monitor{
		Edid: edid,
		Name: name,
		Active: &api.ActiveMonitor{
			Width: w, Height: h, PositionX: x, PositionY: y, Primary: primary, Scale: 1, RefreshRate: 60,
		},
	}
}

// Dropping the primary TV should promote the leftmost survivor and normalise the
// origin so the remaining monitors start at (0,0).
func TestSynthesizeWithoutPromotesPrimaryAndNormalises(t *testing.T) {
	monitors := []api.Monitor{
		activeMonitor("LG", "LG TV", 3840, 2160, 0, 0, true),       // primary, off
		activeMonitor("AOC", "AOC", 2560, 1440, 3840, 0, false),    // to the right
		activeMonitor("DELL", "Dell", 2560, 1440, 6400, 0, false),  // further right
	}
	off := map[string]bool{"LG": true}

	layout, ok := synthesizeWithout(monitors, off)
	if !ok {
		t.Fatal("synthesizeWithout returned ok=false with survivors present")
	}
	if len(layout.Monitors) != 2 {
		t.Fatalf("got %d monitors; want 2 (TV dropped)", len(layout.Monitors))
	}

	var aoc, dell *api.LayoutMonitor
	for i := range layout.Monitors {
		switch layout.Monitors[i].Edid {
		case "AOC":
			aoc = &layout.Monitors[i]
		case "DELL":
			dell = &layout.Monitors[i]
		case "LG":
			t.Fatal("powered-off TV should have been dropped")
		}
	}
	if aoc == nil || dell == nil {
		t.Fatal("expected AOC and DELL to survive")
	}
	if aoc.PositionX != 0 {
		t.Errorf("leftmost survivor not normalised to x=0: got %d", aoc.PositionX)
	}
	if dell.PositionX != 2560 {
		t.Errorf("second survivor should shift to x=2560: got %d", dell.PositionX)
	}
	if !aoc.Primary || dell.Primary {
		t.Errorf("leftmost survivor should become primary: aoc=%v dell=%v", aoc.Primary, dell.Primary)
	}
}

// A survivor that was already primary keeps it; nothing gets promoted.
func TestSynthesizeWithoutKeepsExistingPrimary(t *testing.T) {
	monitors := []api.Monitor{
		activeMonitor("AOC", "AOC", 2560, 1440, 0, 0, true),
		activeMonitor("LG", "LG TV", 3840, 2160, 2560, 0, false),
	}
	layout, ok := synthesizeWithout(monitors, map[string]bool{"LG": true})
	if !ok || len(layout.Monitors) != 1 {
		t.Fatalf("expected one surviving monitor, got ok=%v n=%d", ok, len(layout.Monitors))
	}
	if layout.Monitors[0].Edid != "AOC" || !layout.Monitors[0].Primary {
		t.Errorf("surviving AOC should stay primary: %+v", layout.Monitors[0])
	}
}

// If every monitor is powered off there's nothing to synthesize.
func TestSynthesizeWithoutEmpty(t *testing.T) {
	monitors := []api.Monitor{activeMonitor("LG", "LG TV", 3840, 2160, 0, 0, true)}
	if _, ok := synthesizeWithout(monitors, map[string]bool{"LG": true}); ok {
		t.Error("synthesizeWithout should return ok=false when no monitor survives")
	}
}
