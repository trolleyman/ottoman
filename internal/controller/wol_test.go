package controller

import (
	"strings"
	"testing"
)

func TestBuildMagicPacket(t *testing.T) {
	mac := []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}
	packet := buildMagicPacket(mac)
	if len(packet) != 102 {
		t.Fatalf("packet length = %d, want 102", len(packet))
	}
	for i := 0; i < 6; i++ {
		if packet[i] != 0xFF {
			t.Fatalf("byte %d = %#x, want 0xFF", i, packet[i])
		}
	}
	// MAC repeated 16 times after the 6-byte preamble.
	for rep := 0; rep < 16; rep++ {
		off := 6 + rep*6
		for j := 0; j < 6; j++ {
			if packet[off+j] != mac[j] {
				t.Fatalf("rep %d byte %d = %#x, want %#x", rep, j, packet[off+j], mac[j])
			}
		}
	}
}

func TestParseMACSeparators(t *testing.T) {
	for _, in := range []string{"AA:BB:CC:DD:EE:FF", "aa-bb-cc-dd-ee-ff", "AABB.CCDD.EEFF", "aabbccddeeff"} {
		mac, err := parseMAC(in)
		if err != nil {
			t.Fatalf("parseMAC(%q): %v", in, err)
		}
		if len(mac) != 6 {
			t.Fatalf("parseMAC(%q) len = %d, want 6", in, len(mac))
		}
	}
	if _, err := parseMAC("not-a-mac"); err == nil {
		t.Fatal("expected error for invalid MAC")
	}
}

func TestValidateWakeConfig(t *testing.T) {
	if w := ValidateWakeConfig(""); w != nil {
		t.Fatalf("empty MAC should yield no warnings, got %v", w)
	}
	w := ValidateWakeConfig("zz:zz:zz:zz:zz:zz")
	if len(w) == 0 || !strings.Contains(w[0], "invalid") {
		t.Fatalf("expected invalid-MAC warning, got %v", w)
	}
}
