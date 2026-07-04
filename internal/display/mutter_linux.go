//go:build linux

package display

import (
	"math"
	"strings"

	"github.com/godbus/dbus/v5"
	"github.com/pkg/errors"
	"github.com/trolleyman/ottoman/internal/api"
)

// Mutter's org.gnome.Mutter.DisplayConfig D-Bus interface. This is the native
// display-control API for GNOME on Wayland (and X11); unlike xrandr it can
// reconfigure real outputs under Wayland and exposes vendor/product/serial per
// connector.
const (
	mutterBusName    = "org.gnome.Mutter.DisplayConfig"
	mutterObjectPath = "/org/gnome/Mutter/DisplayConfig"
	mutterInterface  = "org.gnome.Mutter.DisplayConfig"

	// ApplyMonitorsConfig methods.
	mutterMethodVerify     = 0
	mutterMethodTemporary  = 1
	mutterMethodPersistent = 2
)

// --- GetCurrentState reply types (must match the D-Bus signatures exactly) ---

// monitorSpec is the (ssss) tuple identifying a physical monitor.
type monitorSpec struct {
	Connector string
	Vendor    string
	Product   string
	Serial    string
}

// mutterMode is one available mode: (siiddada{sv}).
type mutterMode struct {
	ID              string
	Width           int32
	Height          int32
	RefreshRate     float64
	PreferredScale  float64
	SupportedScales []float64
	Properties      map[string]dbus.Variant
}

// mutterMonitor is one physical monitor: ((ssss)a(siiddada{sv})a{sv}).
type mutterMonitor struct {
	Spec       monitorSpec
	Modes      []mutterMode
	Properties map[string]dbus.Variant
}

// mutterLogicalMonitor is one logical monitor: (iiduba(ssss)a{sv}).
type mutterLogicalMonitor struct {
	X          int32
	Y          int32
	Scale      float64
	Transform  uint32
	Primary    bool
	Monitors   []monitorSpec
	Properties map[string]dbus.Variant
}

// --- ApplyMonitorsConfig request types: a(iiduba(ssa{sv})) ---

type applyMonitor struct {
	Connector  string
	Mode       string
	Properties map[string]dbus.Variant
}

type applyLogicalMonitor struct {
	X         int32
	Y         int32
	Scale     float64
	Transform uint32
	Primary   bool
	Monitors  []applyMonitor
}

// MutterManager implements display management via the GNOME Mutter D-Bus API.
type MutterManager struct {
	store *Layouts
	conn  *dbus.Conn
}

// newMutterManager connects to the session bus and verifies that the Mutter
// DisplayConfig interface is answering before returning a usable manager.
func newMutterManager(store *Layouts) (*MutterManager, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to session bus")
	}

	m := &MutterManager{store: store, conn: conn}

	// Sanity check: make sure GetCurrentState actually responds.
	if _, _, _, err := m.getCurrentState(); err != nil {
		conn.Close()
		return nil, errors.Wrap(err, "mutter DisplayConfig not available")
	}

	return m, nil
}

// getCurrentState calls GetCurrentState and returns the serial plus decoded
// monitors and logical monitors.
func (m *MutterManager) getCurrentState() (uint32, []mutterMonitor, []mutterLogicalMonitor, error) {
	obj := m.conn.Object(mutterBusName, dbus.ObjectPath(mutterObjectPath))
	call := obj.Call(mutterInterface+".GetCurrentState", 0)
	if call.Err != nil {
		return 0, nil, nil, errors.Wrap(call.Err, "GetCurrentState failed")
	}

	var serial uint32
	var monitors []mutterMonitor
	var logical []mutterLogicalMonitor
	var props map[string]dbus.Variant
	if err := call.Store(&serial, &monitors, &logical, &props); err != nil {
		return 0, nil, nil, errors.Wrap(err, "failed to decode GetCurrentState reply")
	}
	return serial, monitors, logical, nil
}

// ListMonitors returns information about connected monitors.
func (m *MutterManager) ListMonitors() ([]api.Monitor, error) {
	_, monitors, logical, err := m.getCurrentState()
	if err != nil {
		return nil, err
	}

	// Index logical monitors by connector so we can tell which physical
	// monitors are active and where they sit.
	logicalByConnector := make(map[string]*mutterLogicalMonitor)
	for i := range logical {
		for _, spec := range logical[i].Monitors {
			logicalByConnector[spec.Connector] = &logical[i]
		}
	}

	result := make([]api.Monitor, 0, len(monitors))
	for _, mon := range monitors {
		apiMon := api.Monitor{
			Edid:         monitorEDID(mon.Spec),
			Manufacturer: mon.Spec.Vendor,
			Name:         monitorName(mon),
			Port:         mon.Spec.Connector,
		}

		if lm, ok := logicalByConnector[mon.Spec.Connector]; ok {
			cur := currentMode(mon)
			active := &api.ActiveMonitor{
				PositionX: int(lm.X),
				PositionY: int(lm.Y),
				Primary:   lm.Primary,
				Model:     mon.Spec.Product,
			}
			if cur != nil {
				active.Width = int(cur.Width)
				active.Height = int(cur.Height)
				active.RefreshRate = cur.RefreshRate
			}
			apiMon.Active = active
		}

		result = append(result, apiMon)
	}

	SortMonitors(result)
	return result, nil
}

// ApplyLayoutConfig applies a display configuration via ApplyMonitorsConfig.
func (m *MutterManager) ApplyLayoutConfig(layout api.Layout) error {
	serial, monitors, _, err := m.getCurrentState()
	if err != nil {
		return err
	}

	byConnector := make(map[string]*mutterMonitor)
	for i := range monitors {
		byConnector[monitors[i].Spec.Connector] = &monitors[i]
	}
	byEDID := make(map[string]*mutterMonitor)
	for i := range monitors {
		byEDID[monitorEDID(monitors[i].Spec)] = &monitors[i]
	}

	var logicals []applyLogicalMonitor
	for _, lm := range layout.Monitors {
		mon := resolveMonitor(lm, byEDID, byConnector)
		if mon == nil {
			return errors.Errorf("layout monitor %q (edid=%q port=%q) is not connected", lm.Name, lm.Edid, lm.Port)
		}

		mode := pickMode(mon, lm)
		if mode == nil {
			return errors.Errorf("no matching mode %dx%d@%.2f for monitor %q", lm.Width, lm.Height, lm.RefreshRate, mon.Spec.Connector)
		}

		logicals = append(logicals, applyLogicalMonitor{
			X:         int32(lm.PositionX),
			Y:         int32(lm.PositionY),
			Scale:     1.0,
			Transform: 0,
			Primary:   lm.Primary,
			Monitors: []applyMonitor{{
				Connector:  mon.Spec.Connector,
				Mode:       mode.ID,
				Properties: map[string]dbus.Variant{},
			}},
		})
	}

	if len(logicals) == 0 {
		return errors.New("layout has no applicable monitors")
	}

	obj := m.conn.Object(mutterBusName, dbus.ObjectPath(mutterObjectPath))
	call := obj.Call(mutterInterface+".ApplyMonitorsConfig", 0,
		serial,
		uint32(mutterMethodPersistent),
		logicals,
		map[string]dbus.Variant{},
	)
	if call.Err != nil {
		return errors.Wrap(call.Err, "ApplyMonitorsConfig failed")
	}
	return nil
}

// resolveMonitor finds the connected monitor matching a layout monitor,
// preferring a stable EDID match and falling back to the connector/port.
func resolveMonitor(lm api.LayoutMonitor, byEDID, byConnector map[string]*mutterMonitor) *mutterMonitor {
	if lm.Edid != "" {
		if mon, ok := byEDID[lm.Edid]; ok {
			return mon
		}
	}
	if lm.Port != "" {
		if mon, ok := byConnector[lm.Port]; ok {
			return mon
		}
	}
	return nil
}

// pickMode chooses the mode best matching the layout's resolution and refresh
// rate. Resolution must match exactly; among those, the closest refresh rate
// wins (falling back to the current/preferred mode when the layout has none).
func pickMode(mon *mutterMonitor, lm api.LayoutMonitor) *mutterMode {
	var best *mutterMode
	bestDelta := math.MaxFloat64
	for i := range mon.Modes {
		mode := &mon.Modes[i]
		if int(mode.Width) != lm.Width || int(mode.Height) != lm.Height {
			continue
		}
		delta := math.Abs(mode.RefreshRate - lm.RefreshRate)
		if lm.RefreshRate == 0 {
			// No refresh preference: prefer the preferred mode, else first match.
			if modeIsPreferred(mode) {
				return mode
			}
			if best == nil {
				best = mode
			}
			continue
		}
		if delta < bestDelta {
			best = mode
			bestDelta = delta
		}
	}
	return best
}

func modeIsPreferred(mode *mutterMode) bool {
	if v, ok := mode.Properties["is-preferred"]; ok {
		if b, ok := v.Value().(bool); ok {
			return b
		}
	}
	return false
}

func currentMode(mon mutterMonitor) *mutterMode {
	for i := range mon.Modes {
		if v, ok := mon.Modes[i].Properties["is-current"]; ok {
			if b, ok := v.Value().(bool); ok && b {
				return &mon.Modes[i]
			}
		}
	}
	return nil
}

// monitorEDID builds a stable identifier from vendor/product/serial. Mutter
// does not expose raw EDID bytes, but this triple is stable across ports and
// reboots, which is all the layout matcher and monitor registry need.
func monitorEDID(spec monitorSpec) string {
	if spec.Vendor == "" && spec.Product == "" && spec.Serial == "" {
		return ""
	}
	return strings.Join([]string{spec.Vendor, spec.Product, spec.Serial}, ":")
}

// monitorName prefers the human-readable display-name property, falling back to
// the product code and then the connector.
func monitorName(mon mutterMonitor) string {
	if v, ok := mon.Properties["display-name"]; ok {
		if s, ok := v.Value().(string); ok && s != "" {
			return s
		}
	}
	if mon.Spec.Product != "" {
		return mon.Spec.Product
	}
	return mon.Spec.Connector
}
