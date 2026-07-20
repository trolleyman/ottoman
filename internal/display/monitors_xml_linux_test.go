//go:build linux

package display

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormatScaleXML(t *testing.T) {
	cases := []struct {
		scale float64
		want  string
	}{
		{0, "1"},     // unset defaults to 1
		{1, "1"},     // integer, no decimal point
		{2, "2"},     // integer
		{1.5, "1.5"}, // fractional at precision
		{1.25, "1.25"},
	}
	for _, c := range cases {
		if got := formatScaleXML(c.scale); got != c.want {
			t.Errorf("formatScaleXML(%v) = %q, want %q", c.scale, got, c.want)
		}
	}
}

func specA() monitorSpec {
	return monitorSpec{Connector: "DP-1", Vendor: "LG", Product: "27GL", Serial: "S1"}
}
func specB() monitorSpec {
	return monitorSpec{Connector: "HDMI-1", Vendor: "DEL", Product: "U2415", Serial: "S2"}
}

// wrapMonitors renders a full monitors.xml around one or more <configuration>
// blocks, mirroring writeMonitorsXML's framing.
func wrapMonitors(blocks ...string) string {
	return "<monitors version=\"2\">\n" + strings.Join(blocks, "") + "</monitors>\n"
}

// writeTempMonitors writes a monitors.xml into a temp dir and returns its path.
func writeTempMonitors(t *testing.T, blocks ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "monitors.xml")
	if err := os.WriteFile(path, []byte(wrapMonitors(blocks...)), 0o644); err != nil {
		t.Fatalf("writing temp monitors.xml: %v", err)
	}
	return path
}

func edidSet(specs ...monitorSpec) map[string]bool {
	set := make(map[string]bool, len(specs))
	for _, s := range specs {
		set[specEDIDKey(s)] = true
	}
	return set
}

// The same monitors on renumbered connectors must be recognised as the current
// configuration and dropped, not preserved as an orphan. This is the case that
// let blocks pile up: DisplayPort renumbers DP-6 to DP-3 when a monitor sleeps.
func TestPreservedConfigsDropsRenumberedConnectors(t *testing.T) {
	// Same physical monitors as specA/specB, but on different connectors.
	renamedA := monitorSpec{Connector: "DP-9", Vendor: "LG", Product: "27GL", Serial: "S1"}
	renamedB := monitorSpec{Connector: "HDMI-7", Vendor: "DEL", Product: "U2415", Serial: "S2"}
	block := buildConfigurationXML(
		[]persistLogicalMonitor{{spec: renamedA, width: 2560, height: 1440, rate: 60}},
		[]monitorSpec{renamedB},
	)
	path := writeTempMonitors(t, block)

	got := preservedConfigs(path, edidSet(specA(), specB()))
	if len(got) != 0 {
		t.Errorf("block for the same monitors on renumbered connectors should be replaced, kept %d", len(got))
	}
}

// A block describing genuinely different hardware must survive.
func TestPreservedConfigsKeepsOtherHardware(t *testing.T) {
	other := monitorSpec{Connector: "DP-1", Vendor: "AOC", Product: "Q27G4", Serial: "S9"}
	block := buildConfigurationXML(
		[]persistLogicalMonitor{{spec: other, width: 2560, height: 1440, rate: 60}}, nil)
	path := writeTempMonitors(t, block)

	if got := preservedConfigs(path, edidSet(specA(), specB())); len(got) != 1 {
		t.Errorf("a block for different monitors should be preserved, kept %d", len(got))
	}
}

// Several blocks describing the same monitors collapse to the first: Mutter only
// ever applies the first match, so the rest are dead weight that can later hijack
// the display when they match a transient hardware state.
func TestPreservedConfigsDeduplicatesEquivalentBlocks(t *testing.T) {
	other := monitorSpec{Connector: "DP-1", Vendor: "AOC", Product: "Q27G4", Serial: "S9"}
	sameOnAnotherPort := monitorSpec{Connector: "DP-5", Vendor: "AOC", Product: "Q27G4", Serial: "S9"}
	first := buildConfigurationXML(
		[]persistLogicalMonitor{{spec: other, width: 2560, height: 1440, rate: 60}}, nil)
	dup := buildConfigurationXML(
		[]persistLogicalMonitor{{spec: sameOnAnotherPort, width: 1920, height: 1080, rate: 60}}, nil)
	path := writeTempMonitors(t, first, dup)

	got := preservedConfigs(path, edidSet(specA(), specB()))
	if len(got) != 1 {
		t.Fatalf("equivalent blocks should collapse to one, kept %d", len(got))
	}
	if !strings.Contains(got[0], "2560") {
		t.Errorf("the first block should be the one kept, got:\n%s", got[0])
	}
}

// A generated block must parse back into the spec sets used to match hardware.
func TestBuildConfigurationRoundTrips(t *testing.T) {
	enabled := []persistLogicalMonitor{{
		spec: specA(), x: 0, y: 0, width: 2560, height: 1440, rate: 59.951, primary: true,
	}}
	block := buildConfigurationXML(enabled, []monitorSpec{specB()})

	var parsed monitorsFileXML
	if err := xml.Unmarshal([]byte(wrapMonitors(block)), &parsed); err != nil {
		t.Fatalf("generated XML did not parse: %v", err)
	}
	if len(parsed.Configs) != 1 {
		t.Fatalf("want 1 configuration, got %d", len(parsed.Configs))
	}
	cfg := parsed.Configs[0]
	if len(cfg.LogicalSpecs) != 1 || cfg.LogicalSpecs[0].Connector != "DP-1" {
		t.Errorf("logical spec not parsed: %+v", cfg.LogicalSpecs)
	}
	if len(cfg.DisabledSpecs) != 1 || cfg.DisabledSpecs[0].Connector != "HDMI-1" {
		t.Errorf("disabled spec not parsed: %+v", cfg.DisabledSpecs)
	}
	if !strings.Contains(block, "<primary>yes</primary>") {
		t.Errorf("primary flag missing:\n%s", block)
	}
	if !strings.Contains(block, "<rate>59.951</rate>") {
		t.Errorf("rate not rendered as expected:\n%s", block)
	}
}

// A non-primary monitor must not emit a <primary> element (Mutter reads its
// absence as "no").
func TestBuildConfigurationOmitsNonPrimary(t *testing.T) {
	block := buildConfigurationXML([]persistLogicalMonitor{{spec: specA(), width: 1920, height: 1080, rate: 60}}, nil)
	if strings.Contains(block, "<primary>") {
		t.Errorf("non-primary monitor should not emit <primary>:\n%s", block)
	}
}

// preservedConfigs must drop the block matching the current hardware set and
// keep unrelated ones verbatim.
func TestPreservedConfigsReplacesMatchingKeepsOthers(t *testing.T) {
	// Block for current hardware {A (enabled), B (disabled)}.
	current := buildConfigurationXML(
		[]persistLogicalMonitor{{spec: specA(), width: 2560, height: 1440, rate: 60, primary: true}},
		[]monitorSpec{specB()},
	)
	// Block for a different hardware set {C}, carrying a marker to prove verbatim preservation.
	other := "  <configuration>\n    <logicalmonitor>\n      <x>0</x>\n      <y>0</y>\n" +
		"      <scale>2</scale>\n      <primary>yes</primary>\n      <monitor>\n" +
		"        <monitorspec>\n          <connector>DP-2</connector>\n          <vendor>SAM</vendor>\n" +
		"          <product>LC49</product>\n          <serial>S3</serial>\n        </monitorspec>\n" +
		"        <mode>\n          <width>5120</width>\n          <height>1440</height>\n" +
		"          <rate>120</rate>\n        </mode>\n      </monitor>\n    </logicalmonitor>\n  </configuration>\n"

	dir := t.TempDir()
	path := filepath.Join(dir, "monitors.xml")
	if err := os.WriteFile(path, []byte(wrapMonitors(current, other)), 0o644); err != nil {
		t.Fatal(err)
	}

	preserved := preservedConfigs(path, edidSet(specA(), specB()))

	if len(preserved) != 1 {
		t.Fatalf("want 1 preserved config, got %d", len(preserved))
	}
	if !strings.Contains(preserved[0], "<scale>2</scale>") || !strings.Contains(preserved[0], "LC49") {
		t.Errorf("preserved config not kept verbatim: %q", preserved[0])
	}
}

// A missing or unparseable file must preserve nothing rather than error.
func TestPreservedConfigsMissingFile(t *testing.T) {
	if got := preservedConfigs(filepath.Join(t.TempDir(), "nope.xml"), map[string]bool{}); got != nil {
		t.Errorf("missing file should preserve nothing, got %v", got)
	}
}
