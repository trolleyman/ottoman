package agent

import (
	"testing"

	"github.com/trolleyman/ottoman/internal/ddc"
)

func TestDDCMatches(t *testing.T) {
	lg := ddc.Display{Bus: 4, Mfg: "GSM", Model: "LG ULTRAGEAR", Serial: "1234ABCD"}
	dell := ddc.Display{Bus: 7, Mfg: "DEL", Model: "DELL U2717D"}

	cases := []struct {
		name string
		edid string
		disp ddc.Display
		want bool
	}{
		{"serial match", "GSM:LG ULTRAGEAR:1234ABCD", lg, true},
		{"model match no serial", "DEL:DELL U2717D:", dell, true},
		{"wrong mfg", "AUS:LG ULTRAGEAR:1234ABCD", lg, false},
		{"serial mismatch same mfg", "GSM:LG ULTRAGEAR:9999", lg, false},
		{"mfg only", "DEL::", dell, true},
		{"model substring", "DEL:DELL U2717D 27inch:", dell, true},
	}
	for _, c := range cases {
		if got := ddcMatches(c.edid, c.disp); got != c.want {
			t.Errorf("%s: ddcMatches(%q, %+v) = %v, want %v", c.name, c.edid, c.disp, got, c.want)
		}
	}
}

// newTestControl builds a monitorControl with just the maps mergeDisplays
// touches, avoiding a real registry.
func newTestControl() *monitorControl {
	return &monitorControl{ddcBusMemory: map[string]int{}}
}

func busFor(displays []ddc.Display, edid string) (int, bool) {
	for _, d := range displays {
		if ddcMatches(edid, d) && d.Bus >= 0 {
			return d.Bus, true
		}
	}
	return 0, false
}

func TestMergeDisplaysPrefersLiveBus(t *testing.T) {
	c := newTestControl()
	direct := []ddc.Display{{Bus: 6, Mfg: "AOC", Model: "Q27G4", Serial: "2S6R6HA023228"}}
	sysfs := []ddc.Display{
		{Bus: -1, Mfg: "AOC", Model: "Q27G4", Serial: "2S6R6HA023228"},
		{Bus: -1, Mfg: "PHL", Model: "PHL 243V7", Serial: "0x00001910"},
	}
	got := c.mergeDisplays(direct, sysfs)

	if bus, ok := busFor(got, "AOC:Q27G4:2S6R6HA023228"); !ok || bus != 6 {
		t.Fatalf("AOC bus = %d,%v; want 6,true", bus, ok)
	}
	// The live read taught us the AOC's bus; it should be remembered.
	if c.ddcBusMemory[displayKey(direct[0])] != 6 {
		t.Errorf("bus memory not recorded: %v", c.ddcBusMemory)
	}
	// PHL is sysfs-only with no bus and no memory: unaddressable, not a bogus 0.
	if _, ok := busFor(got, "PHL:PHL 243V7:0x00001910"); ok {
		t.Error("PHL reported addressable with no known bus")
	}
}

// TestMergeDisplaysFallsBackToMemory is the crux of the fix: a monitor that
// drops off live i2c mid-session (present only in sysfs, no bus) stays
// controllable on the bus we last addressed it on.
func TestMergeDisplaysFallsBackToMemory(t *testing.T) {
	c := newTestControl()

	// First detect: AOC answers live on bus 6.
	c.mergeDisplays([]ddc.Display{{Bus: 6, Mfg: "AOC", Model: "Q27G4", Serial: "2S6R6HA023228"}}, nil)

	// Later detect: live i2c read fails, AOC only shows up in sysfs with no bus.
	got := c.mergeDisplays(nil, []ddc.Display{{Bus: -1, Mfg: "AOC", Model: "Q27G4", Serial: "2S6R6HA023228"}})

	if bus, ok := busFor(got, "AOC:Q27G4:2S6R6HA023228"); !ok || bus != 6 {
		t.Fatalf("AOC bus after i2c dropout = %d,%v; want 6,true (from memory)", bus, ok)
	}
}

func TestSummarizeDisplays(t *testing.T) {
	got := summarizeDisplays([]ddc.Display{
		{Bus: 6, Mfg: "AOC", Model: "Q27G4"},
		{Bus: -1, Mfg: "PHL", Model: "PHL 243V7"},
	})
	want := "AOC/Q27G4@i2c-6, PHL/PHL 243V7@no-bus"
	if got != want {
		t.Errorf("summarizeDisplays = %q, want %q", got, want)
	}
	if s := summarizeDisplays(nil); s != "no displays" {
		t.Errorf("empty summary = %q", s)
	}
}

func TestSplitEDID(t *testing.T) {
	v, p, s := splitEDID("GSM:LG ULTRAGEAR:1234")
	if v != "GSM" || p != "LG ULTRAGEAR" || s != "1234" {
		t.Fatalf("split = %q/%q/%q", v, p, s)
	}
	v, p, s = splitEDID("GSM")
	if v != "GSM" || p != "" || s != "" {
		t.Fatalf("split single = %q/%q/%q", v, p, s)
	}
}

func TestCharNeedsShift(t *testing.T) {
	shift := []rune{'A', 'Z', '!', '@', '#', '$', '%', '^', '&', '*', '(', ')',
		'_', '+', '{', '}', '|', ':', '"', '~', '<', '>', '?'}
	for _, r := range shift {
		if !charNeedsShift(r) {
			t.Errorf("charNeedsShift(%q) = false, want true", r)
		}
	}
	noShift := []rune{'a', 'z', '1', '0', '-', '=', '[', ']', ';', '\'', ',', '.', '/', ' '}
	for _, r := range noShift {
		if charNeedsShift(r) {
			t.Errorf("charNeedsShift(%q) = true, want false", r)
		}
	}
}
