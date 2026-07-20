//go:build linux

package display

import (
	"testing"

	"github.com/godbus/dbus/v5"
	"github.com/trolleyman/ottoman/internal/api"
)

func TestStateMatchesIntent(t *testing.T) {
	intent := map[string]intentMonitor{
		"DP-6": {x: 0, y: 360, scale: 1},
		"DP-4": {x: 1920, y: 0, scale: 1},
	}
	match := []mutterLogicalMonitor{
		{X: 0, Y: 360, Scale: 1, Monitors: []monitorSpec{{Connector: "DP-6"}}},
		{X: 1920, Y: 0, Scale: 1, Monitors: []monitorSpec{{Connector: "DP-4"}}},
	}
	if !stateMatchesIntent(match, intent) {
		t.Error("identical state should match")
	}

	// A monitor enabled that the layout wanted off (the TV hijack case).
	extra := append(append([]mutterLogicalMonitor{}, match...),
		mutterLogicalMonitor{X: 0, Y: 0, Scale: 2, Monitors: []monitorSpec{{Connector: "HDMI-2"}}})
	if stateMatchesIntent(extra, intent) {
		t.Error("an unexpected enabled monitor must not match")
	}

	// Only some of the intended monitors are on.
	if stateMatchesIntent(match[:1], intent) {
		t.Error("a missing monitor must not match")
	}

	// Right monitors, wrong scale — the 200%-not-applied case.
	wrongScale := []mutterLogicalMonitor{
		{X: 0, Y: 360, Scale: 2, Monitors: []monitorSpec{{Connector: "DP-6"}}},
		{X: 1920, Y: 0, Scale: 1, Monitors: []monitorSpec{{Connector: "DP-4"}}},
	}
	if stateMatchesIntent(wrongScale, intent) {
		t.Error("a differing scale must not match")
	}

	// Right monitors, wrong position.
	wrongPos := []mutterLogicalMonitor{
		{X: 0, Y: 0, Scale: 1, Monitors: []monitorSpec{{Connector: "DP-6"}}},
		{X: 1920, Y: 0, Scale: 1, Monitors: []monitorSpec{{Connector: "DP-4"}}},
	}
	if stateMatchesIntent(wrongPos, intent) {
		t.Error("a differing position must not match")
	}
}

func TestLayoutApplyOutcomeOk(t *testing.T) {
	ok := []LayoutApplyOutcome{OutcomeApplied, OutcomeAlreadyActive, OutcomeUnverified}
	for _, o := range ok {
		if !o.Ok() {
			t.Errorf("%s should be Ok", o)
		}
	}
	for _, o := range []LayoutApplyOutcome{OutcomeRolledBack, OutcomeMismatch} {
		if o.Ok() {
			t.Errorf("%s should not be Ok", o)
		}
	}
}

func TestLogicalCoordRoundTrip(t *testing.T) {
	cases := []struct {
		name       string
		physical   int32
		scale      float64
		mode       uint32
		wantLogic  int
		wantBackTo int32
	}{
		{"origin", 0, 2.0, layoutModePhysical, 0, 0},
		{"physical 200%", 3840, 2.0, layoutModePhysical, 1920, 3840},
		{"physical 100%", 2560, 1.0, layoutModePhysical, 2560, 2560},
		{"logical mode passthrough", 1920, 2.0, layoutModeLogical, 1920, 1920},
	}
	for _, c := range cases {
		gotLogic := toLogicalCoord(c.physical, c.scale, c.mode)
		if gotLogic != c.wantLogic {
			t.Errorf("%s: toLogicalCoord = %d, want %d", c.name, gotLogic, c.wantLogic)
		}
		gotBack := fromLogicalCoord(gotLogic, c.scale, c.mode)
		if gotBack != c.wantBackTo {
			t.Errorf("%s: fromLogicalCoord = %d, want %d", c.name, gotBack, c.wantBackTo)
		}
	}
}

func TestIsFractionalScale(t *testing.T) {
	cases := []struct {
		scale float64
		want  bool
	}{
		{0, false},   // unset
		{1, false},   // 100%
		{2, false},   // 200%
		{1.25, true}, // fractional
		{1.5, true},
		{1.7518248558044434, true}, // Mutter's exact 175% value
	}
	for _, c := range cases {
		if got := isFractionalScale(c.scale); got != c.want {
			t.Errorf("isFractionalScale(%v) = %v, want %v", c.scale, got, c.want)
		}
	}
}

func TestLayoutNeedsFractional(t *testing.T) {
	cases := []struct {
		name   string
		scales []float64
		want   bool
	}{
		{"uniform 100%", []float64{1, 1}, false},
		{"single monitor at 200%", []float64{2}, false},
		{"uniform 200%", []float64{2, 2}, false},
		{"unset scales", []float64{0, 0}, false},
		// The legacy physical mode has one global scale factor, so a 200% TV
		// beside 100% monitors renders everything at one factor: it reports 200%
		// while looking 100%. Only logical mode can express this.
		{"mixed integer scales", []float64{2, 1}, true},
		{"unset alongside 200%", []float64{0, 2}, true},
		{"fractional", []float64{1.5, 1.5}, true},
		{"fractional among integers", []float64{2, 1.5}, true},
	}
	for _, c := range cases {
		mons := make([]api.LayoutMonitor, len(c.scales))
		for i, s := range c.scales {
			mons[i] = api.LayoutMonitor{Scale: s}
		}
		if got := layoutNeedsFractional(api.Layout{Monitors: mons}); got != c.want {
			t.Errorf("%s: layoutNeedsFractional(%v) = %v, want %v", c.name, c.scales, got, c.want)
		}
	}
}

func TestPickScale(t *testing.T) {
	mode := &mutterMode{PreferredScale: 1.0, SupportedScales: []float64{1.0, 1.25, 1.5, 2.0}}

	// Unset scale falls back to the mode's preferred scale.
	if got := pickScale(mode, 0); got != 1.0 {
		t.Errorf("pickScale(unset) = %v, want 1.0 (preferred)", got)
	}
	// Exact supported value passes through.
	if got := pickScale(mode, 1.5); got != 1.5 {
		t.Errorf("pickScale(1.5) = %v, want 1.5", got)
	}
	// A near-miss snaps to the closest supported value.
	if got := pickScale(mode, 1.4); got != 1.5 {
		t.Errorf("pickScale(1.4) = %v, want 1.5 (nearest)", got)
	}
	// An unsupported high value snaps down to the max supported.
	if got := pickScale(mode, 3.0); got != 2.0 {
		t.Errorf("pickScale(3.0) = %v, want 2.0 (nearest)", got)
	}
	// With no supported list, the request is trusted as-is.
	if got := pickScale(&mutterMode{}, 1.5); got != 1.5 {
		t.Errorf("pickScale(no supported list) = %v, want 1.5", got)
	}
}

func TestParseGSettingsStringArray(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"@as []\n", nil},
		{"['scale-monitor-framebuffer']\n", []string{"scale-monitor-framebuffer"}},
		{"['a', 'b', 'scale-monitor-framebuffer']\n", []string{"a", "b", "scale-monitor-framebuffer"}},
	}
	for _, c := range cases {
		got := parseGSettingsStringArray(c.in)
		if len(got) != len(c.want) {
			t.Errorf("parseGSettingsStringArray(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("parseGSettingsStringArray(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

func TestMonitorEDID(t *testing.T) {
	if got := monitorEDID(monitorSpec{Vendor: "GSM", Product: "LG TV", Serial: "0x01"}); got != "GSM:LG TV:0x01" {
		t.Fatalf("monitorEDID = %q", got)
	}
	if got := monitorEDID(monitorSpec{}); got != "" {
		t.Fatalf("empty spec should give empty EDID, got %q", got)
	}
}

func TestMonitorName(t *testing.T) {
	withName := mutterMonitor{
		Spec:       monitorSpec{Connector: "DP-1", Product: "27GL"},
		Properties: map[string]dbus.Variant{"display-name": dbus.MakeVariant("LG UltraGear")},
	}
	if got := monitorName(withName); got != "LG UltraGear" {
		t.Fatalf("expected display-name, got %q", got)
	}

	noName := mutterMonitor{Spec: monitorSpec{Connector: "DP-1", Product: "27GL"}}
	if got := monitorName(noName); got != "27GL" {
		t.Fatalf("expected product fallback, got %q", got)
	}

	bare := mutterMonitor{Spec: monitorSpec{Connector: "HDMI-1"}}
	if got := monitorName(bare); got != "HDMI-1" {
		t.Fatalf("expected connector fallback, got %q", got)
	}
}

func TestPickMode(t *testing.T) {
	mon := &mutterMonitor{
		Modes: []mutterMode{
			{ID: "2560x1440@60", Width: 2560, Height: 1440, RefreshRate: 59.95},
			{ID: "2560x1440@144", Width: 2560, Height: 1440, RefreshRate: 143.91},
			{ID: "1920x1080@60", Width: 1920, Height: 1080, RefreshRate: 60.0,
				Properties: map[string]dbus.Variant{"is-preferred": dbus.MakeVariant(true)}},
		},
	}

	// Exact resolution + closest refresh.
	got := pickMode(mon, api.LayoutMonitor{Width: 2560, Height: 1440, RefreshRate: 144})
	if got == nil || got.ID != "2560x1440@144" {
		t.Fatalf("expected 144Hz mode, got %+v", got)
	}

	// No refresh preference -> preferred mode wins.
	got = pickMode(mon, api.LayoutMonitor{Width: 1920, Height: 1080})
	if got == nil || got.ID != "1920x1080@60" {
		t.Fatalf("expected preferred mode, got %+v", got)
	}

	// No matching resolution.
	if got := pickMode(mon, api.LayoutMonitor{Width: 800, Height: 600}); got != nil {
		t.Fatalf("expected nil for unmatched resolution, got %+v", got)
	}
}

func TestCurrentMode(t *testing.T) {
	mon := mutterMonitor{
		Modes: []mutterMode{
			{ID: "a"},
			{ID: "b", Properties: map[string]dbus.Variant{"is-current": dbus.MakeVariant(true)}},
		},
	}
	if got := currentMode(mon); got == nil || got.ID != "b" {
		t.Fatalf("expected mode b, got %+v", got)
	}

	none := mutterMonitor{Modes: []mutterMode{{ID: "a"}}}
	if got := currentMode(none); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestResolveMonitor(t *testing.T) {
	monA := &mutterMonitor{Spec: monitorSpec{Connector: "DP-1", Vendor: "LG", Product: "27", Serial: "1"}}
	monB := &mutterMonitor{Spec: monitorSpec{Connector: "HDMI-1"}}
	byEDID := map[string]*mutterMonitor{monitorEDID(monA.Spec): monA}
	byConnector := map[string]*mutterMonitor{"DP-1": monA, "HDMI-1": monB}

	// EDID match preferred.
	if got := resolveMonitor(api.LayoutMonitor{Edid: "LG:27:1", Port: "HDMI-1"}, byEDID, byConnector); got != monA {
		t.Fatalf("expected EDID match to monA")
	}
	// Fall back to port when EDID missing.
	if got := resolveMonitor(api.LayoutMonitor{Port: "HDMI-1"}, byEDID, byConnector); got != monB {
		t.Fatalf("expected port match to monB")
	}
	// No match.
	if got := resolveMonitor(api.LayoutMonitor{Port: "VGA-1"}, byEDID, byConnector); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}
