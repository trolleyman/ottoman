//go:build linux

package display

import (
	"testing"

	"github.com/godbus/dbus/v5"
	"github.com/trolleyman/ottoman/internal/api"
)

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
