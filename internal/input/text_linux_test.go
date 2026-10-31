//go:build linux

package input

import "testing"

func TestTextKeyUsesRemoteBritishLayout(t *testing.T) {
	for _, test := range []struct {
		r     rune
		code  uint16
		shift bool
	}{
		{'"', 3, true}, {'@', 40, true}, {'#', 43, false}, {'£', 4, true},
		{'~', 43, true}, {'\\', 86, false}, {'|', 86, true}, {'\'', 40, false},
		{'A', 30, true}, {'a', 30, false}, {'?', 53, true},
	} {
		code, mods, ok := textKey(test.r, "gb")
		if !ok || code != test.code || (len(mods) > 0) != test.shift {
			t.Errorf("British %q: code=%d mods=%v ok=%v", test.r, code, mods, ok)
		}
	}
	for _, test := range []struct {
		r    rune
		code uint16
	}{{'"', 40}, {'@', 3}, {'#', 4}, {'\\', 43}} {
		code, _, ok := textKey(test.r, "us")
		if !ok || code != test.code {
			t.Errorf("US %q: code=%d ok=%v", test.r, code, ok)
		}
	}
	enabled := false
	for _, code := range allKeyCodes() {
		if code == 86 {
			enabled = true
		}
	}
	if !enabled {
		t.Fatal("ISO backslash key must be enabled on virtual keyboard")
	}
}
