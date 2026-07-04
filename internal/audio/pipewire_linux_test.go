//go:build linux

package audio

import "testing"

// Representative `wpctl status` output (trimmed to the Audio block).
const sampleStatus = `PipeWire 'pipewire-0' [1.0.5, callum@ottoman, cookie:12345]
 └─ Clients:
        32. WirePlumber

Audio
 ├─ Devices:
 │      45. Built-in Audio
 │
 ├─ Sinks:
 │  *   55. HDA NVidia Digital Stereo (HDMI) [vol: 0.65]
 │      61. Logi Z407 Analogue Stereo        [vol: 1.00]
 │
 ├─ Sources:
 │      62. Built-in Microphone              [vol: 0.90]
 │
 ├─ Filters:
 │
 └─ Streams:

Video
 ├─ Devices:
`

func TestParseSinkSection(t *testing.T) {
	sinks := parseSinkSection(sampleStatus)
	if len(sinks) != 2 {
		t.Fatalf("expected 2 sinks, got %d: %+v", len(sinks), sinks)
	}

	if sinks[0].ID != 55 {
		t.Errorf("sink[0].ID = %d, want 55", sinks[0].ID)
	}
	if !sinks[0].Default {
		t.Errorf("sink[0] should be default")
	}
	if sinks[0].Description != "HDA NVidia Digital Stereo (HDMI)" {
		t.Errorf("sink[0].Description = %q", sinks[0].Description)
	}

	if sinks[1].ID != 61 {
		t.Errorf("sink[1].ID = %d, want 61", sinks[1].ID)
	}
	if sinks[1].Default {
		t.Errorf("sink[1] should not be default")
	}
	if sinks[1].Description != "Logi Z407 Analogue Stereo" {
		t.Errorf("sink[1].Description = %q", sinks[1].Description)
	}
}

func TestParseSinkSectionStopsAtSources(t *testing.T) {
	// The microphone (id 62) is a Source and must not be parsed as a sink.
	for _, s := range parseSinkSection(sampleStatus) {
		if s.ID == 62 {
			t.Fatalf("source id 62 leaked into sinks")
		}
	}
}

func TestParseVolume(t *testing.T) {
	cases := []struct {
		in    string
		vol   float64
		muted bool
	}{
		{"Volume: 0.65\n", 0.65, false},
		{"Volume: 1.00 [MUTED]\n", 1.00, true},
		{"Volume: 0.00\n", 0.0, false},
	}
	for _, c := range cases {
		vol, muted, err := parseVolume(c.in)
		if err != nil {
			t.Errorf("parseVolume(%q): %v", c.in, err)
			continue
		}
		if vol != c.vol || muted != c.muted {
			t.Errorf("parseVolume(%q) = (%v,%v), want (%v,%v)", c.in, vol, muted, c.vol, c.muted)
		}
	}

	if _, _, err := parseVolume("garbage"); err == nil {
		t.Error("expected error for unparseable volume")
	}
}

func TestInspectRegexes(t *testing.T) {
	inspect := `id 55, type PipeWire:Interface:Node
    node.name = "alsa_output.pci-0000_01_00.1.hdmi-stereo"
    node.description = "HDA NVidia Digital Stereo (HDMI)"
`
	if m := nodeNameRe.FindStringSubmatch(inspect); m == nil || m[1] != "alsa_output.pci-0000_01_00.1.hdmi-stereo" {
		t.Fatalf("node.name parse failed: %v", m)
	}
	if m := nodeDescRe.FindStringSubmatch(inspect); m == nil || m[1] != "HDA NVidia Digital Stereo (HDMI)" {
		t.Fatalf("node.description parse failed: %v", m)
	}
}
