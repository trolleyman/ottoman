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
