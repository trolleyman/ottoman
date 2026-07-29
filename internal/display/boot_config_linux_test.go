//go:build linux

package display

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// tvSpec/deskSpec stand in for the two monitors that matter: an HDMI TV and a
// DisplayPort desk monitor.
var (
	tvSpec   = monitorSpec{Connector: "HDMI-1", Vendor: "LGE", Product: "OLED", Serial: "TV1"}
	deskSpec = monitorSpec{Connector: "DP-1", Vendor: "DEL", Product: "U2720Q", Serial: "D1"}
	sideSpec = monitorSpec{Connector: "DP-2", Vendor: "DEL", Product: "U2720Q", Serial: "D2"}
)

// excludeTV is the predicate the agent installs: the TV, by EDID.
func excludeTV(edid string) bool { return edid == monitorEDID(tvSpec) }

func mon(spec monitorSpec, x, y int32, primary bool) persistLogicalMonitor {
	return persistLogicalMonitor{
		spec: spec, x: x, y: y, width: 3840, height: 2160, rate: 60, scale: 1, primary: primary,
	}
}

// readMonitorsXML returns the file the manager wrote.
func readMonitorsXML(t *testing.T) string {
	t.Helper()
	path, err := monitorsXMLPath()
	if err != nil {
		t.Fatalf("monitorsXMLPath: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// newBootConfigTest redirects the config dir into the test's temp dir and
// returns a manager excluding the TV.
func newBootConfigTest(t *testing.T) *MutterManager {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := &MutterManager{}
	m.ExcludeFromBootConfig(excludeTV)
	return m
}

func TestBootConfigLeavesOutTheTV(t *testing.T) {
	m := newBootConfigTest(t)
	connected := []mutterMonitor{{Spec: tvSpec}, {Spec: deskSpec}}

	// A layout spanning the TV and a desk monitor: the TV drives the session
	// now, but must not be what the machine comes back up on.
	err := m.persistBootConfig([]persistLogicalMonitor{
		mon(tvSpec, 0, 0, true),
		mon(deskSpec, 3840, 0, false),
	}, connected)
	if err != nil {
		t.Fatalf("persistBootConfig: %v", err)
	}

	xml := readMonitorsXML(t)
	// The TV may appear once, under <disabled> - never as a logical monitor,
	// which is what the display server would come up on.
	cut := strings.Index(xml, "<disabled>")
	if cut < 0 {
		t.Fatalf("no <disabled> section at all:\n%s", xml)
	}
	if strings.Contains(xml[:cut], "TV1") {
		t.Errorf("the TV is enabled in the boot configuration:\n%s", xml)
	}
	if !strings.Contains(xml, "D1</serial>") {
		t.Errorf("the desk monitor is missing from the boot configuration:\n%s", xml)
	}
	// It must still be listed as connected hardware, or Mutter's spec-set match
	// against the real machine fails and it ignores the block entirely.
	if !strings.Contains(xml[cut:], "TV1</serial>") {
		t.Errorf("the TV is not listed as disabled, so Mutter won't match this block:\n%s", xml)
	}
}

func TestBootConfigNormalisesAfterDroppingTheTV(t *testing.T) {
	m := newBootConfigTest(t)
	connected := []mutterMonitor{{Spec: tvSpec}, {Spec: deskSpec}}

	// The TV sat at the origin and held primary; the desk monitor was to its
	// right. Dropping the TV would leave the survivor at x=3840 with no primary,
	// which Mutter refuses.
	if err := m.persistBootConfig([]persistLogicalMonitor{
		mon(tvSpec, 0, 0, true),
		mon(deskSpec, 3840, 0, false),
	}, connected); err != nil {
		t.Fatalf("persistBootConfig: %v", err)
	}

	xml := readMonitorsXML(t)
	if !strings.Contains(xml, "<x>0</x>") {
		t.Errorf("the surviving monitor was not moved back to the origin:\n%s", xml)
	}
	if strings.Contains(xml, "<x>3840</x>") {
		t.Errorf("the surviving monitor kept the offset left by the TV:\n%s", xml)
	}
	if !strings.Contains(xml, "<primary>yes</primary>") {
		t.Errorf("no monitor is primary after the TV was dropped:\n%s", xml)
	}
}

func TestBootConfigKeepsTheLastGoodOneWhenOnlyTheTVIsLeft(t *testing.T) {
	m := newBootConfigTest(t)

	// First, an ordinary desk configuration.
	if err := m.persistBootConfig([]persistLogicalMonitor{mon(deskSpec, 0, 0, true)},
		[]mutterMonitor{{Spec: deskSpec}, {Spec: tvSpec}}); err != nil {
		t.Fatalf("persistBootConfig: %v", err)
	}
	before := readMonitorsXML(t)
	if before == "" {
		t.Fatal("nothing was persisted for the desk configuration")
	}

	// Now switch to the TV alone. There would be nothing left to write, and an
	// empty configuration means a machine that comes up on no screen at all.
	if err := m.persistBootConfig([]persistLogicalMonitor{mon(tvSpec, 0, 0, true)},
		[]mutterMonitor{{Spec: deskSpec}, {Spec: tvSpec}}); err != nil {
		t.Fatalf("persistBootConfig: %v", err)
	}
	if after := readMonitorsXML(t); after != before {
		t.Errorf("the TV-only layout changed the boot configuration:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestBootConfigWithoutAnExclusionPersistsEverything(t *testing.T) {
	// A manager nobody handed a predicate to behaves exactly as before.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := &MutterManager{}

	if err := m.persistBootConfig([]persistLogicalMonitor{mon(tvSpec, 0, 0, true)},
		[]mutterMonitor{{Spec: tvSpec}}); err != nil {
		t.Fatalf("persistBootConfig: %v", err)
	}
	if xml := readMonitorsXML(t); !strings.Contains(xml, "TV1</serial>") {
		t.Errorf("the TV was dropped with no exclusion set:\n%s", xml)
	}
}

func TestBootConfigSkipsAnIdenticalRewrite(t *testing.T) {
	m := newBootConfigTest(t)
	connected := []mutterMonitor{{Spec: deskSpec}, {Spec: tvSpec}}
	enabled := []persistLogicalMonitor{mon(deskSpec, 0, 0, true)}

	if err := m.persistBootConfig(enabled, connected); err != nil {
		t.Fatalf("persistBootConfig: %v", err)
	}
	path, err := monitorsXMLPath()
	if err != nil {
		t.Fatalf("monitorsXMLPath: %v", err)
	}
	first, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	// The boot configuration is reconciled on every agent start; rewriting a
	// file Mutter watches, to the same bytes, is pure churn.
	if err := os.Chtimes(path, first.ModTime().Add(-time.Hour), first.ModTime().Add(-time.Hour)); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	stale, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if err := m.persistBootConfig(enabled, connected); err != nil {
		t.Fatalf("persistBootConfig: %v", err)
	}
	again, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !again.ModTime().Equal(stale.ModTime()) {
		t.Error("an identical boot configuration was rewritten")
	}
}

func TestLiveBootMonitorsReadsTheCurrentArrangement(t *testing.T) {
	// What ReconcileBootConfig feeds to persistBootConfig: the arrangement as
	// the display server currently has it.
	monitors := []mutterMonitor{
		{Spec: deskSpec, Modes: []mutterMode{{ID: "m1", Width: 3840, Height: 2160, RefreshRate: 60,
			Properties: currentModeProps()}}},
		{Spec: sideSpec, Modes: []mutterMode{{ID: "m2", Width: 2560, Height: 1440, RefreshRate: 144,
			Properties: currentModeProps()}}},
	}
	logical := []mutterLogicalMonitor{
		{X: 0, Y: 0, Scale: 1, Primary: true, Monitors: []monitorSpec{deskSpec}},
		{X: 3840, Y: 0, Scale: 1, Monitors: []monitorSpec{sideSpec}},
	}

	got := liveBootMonitors(monitors, logical)
	if len(got) != 2 {
		t.Fatalf("liveBootMonitors returned %d monitors, want 2", len(got))
	}
	if got[0].spec != deskSpec || !got[0].primary || got[0].width != 3840 {
		t.Errorf("first monitor = %+v, want the primary desk monitor at its current mode", got[0])
	}
	if got[1].spec != sideSpec || got[1].x != 3840 || got[1].rate != 144 {
		t.Errorf("second monitor = %+v, want the side monitor where it sits", got[1])
	}
}

func TestBootConfigPreservesOtherHardwareBlocks(t *testing.T) {
	m := newBootConfigTest(t)

	// A configuration for a different set of monitors - the laptop-only case,
	// say - must survive a rewrite of the current one.
	if err := m.persistBootConfig([]persistLogicalMonitor{mon(sideSpec, 0, 0, true)},
		[]mutterMonitor{{Spec: sideSpec}}); err != nil {
		t.Fatalf("persistBootConfig: %v", err)
	}
	if err := m.persistBootConfig([]persistLogicalMonitor{mon(deskSpec, 0, 0, true)},
		[]mutterMonitor{{Spec: deskSpec}, {Spec: tvSpec}}); err != nil {
		t.Fatalf("persistBootConfig: %v", err)
	}

	xml := readMonitorsXML(t)
	if !strings.Contains(xml, "D2</serial>") {
		t.Errorf("the other hardware's block was dropped:\n%s", xml)
	}
	if !strings.Contains(xml, "D1</serial>") {
		t.Errorf("the current block is missing:\n%s", xml)
	}
	if got := strings.Count(xml, "<configuration>"); got != 2 {
		t.Errorf("%d configuration blocks, want 2 (current + preserved)\n%s", got, xml)
	}
}

// currentModeProps marks a mode as the one in use, the way Mutter reports it.
func currentModeProps() map[string]dbus.Variant {
	return map[string]dbus.Variant{"is-current": dbus.MakeVariant(true)}
}
