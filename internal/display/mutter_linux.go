//go:build linux

package display

import (
	"log"
	"math"
	"os/exec"
	"strings"
	"time"

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
	if _, _, _, _, err := m.getCurrentState(); err != nil {
		conn.Close()
		return nil, errors.Wrap(err, "mutter DisplayConfig not available")
	}

	return m, nil
}

// getCurrentState calls GetCurrentState and returns the serial, the decoded
// monitors and logical monitors, and the layout mode (which tells us whether the
// logical monitors' positions are in logical or physical pixels).
func (m *MutterManager) getCurrentState() (uint32, []mutterMonitor, []mutterLogicalMonitor, uint32, error) {
	obj := m.conn.Object(mutterBusName, dbus.ObjectPath(mutterObjectPath))
	call := obj.Call(mutterInterface+".GetCurrentState", 0)
	if call.Err != nil {
		return 0, nil, nil, 0, errors.Wrap(call.Err, "GetCurrentState failed")
	}

	var serial uint32
	var monitors []mutterMonitor
	var logical []mutterLogicalMonitor
	var props map[string]dbus.Variant
	if err := call.Store(&serial, &monitors, &logical, &props); err != nil {
		return 0, nil, nil, 0, errors.Wrap(err, "failed to decode GetCurrentState reply")
	}

	// "layout-mode" (uint32) is absent on setups that don't support changing it;
	// default to logical, which is what Wayland uses with fractional scaling.
	layoutMode := layoutModeLogical
	if v, ok := props["layout-mode"]; ok {
		if lm, ok := v.Value().(uint32); ok {
			layoutMode = lm
		}
	}
	return serial, monitors, logical, layoutMode, nil
}

// ListMonitors returns information about connected monitors.
func (m *MutterManager) ListMonitors() ([]api.Monitor, error) {
	_, monitors, logical, layoutMode, err := m.getCurrentState()
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
				// Report positions in logical pixels regardless of the current
				// layout mode, so a scaled monitor's stored position matches its
				// logical size (physical / scale) and layouts render correctly.
				PositionX: toLogicalCoord(lm.X, lm.Scale, layoutMode),
				PositionY: toLogicalCoord(lm.Y, lm.Scale, layoutMode),
				Primary:   lm.Primary,
				Model:     mon.Spec.Product,
				Scale:     lm.Scale,
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
	_, err := m.ApplyLayoutConfigVerified(layout)
	return err
}

// ApplyLayoutConfigVerified applies a layout and then confirms what actually
// happened to the display, rather than trusting that an accepted request means
// the layout stuck. See verifyApplied for why that distinction matters.
func (m *MutterManager) ApplyLayoutConfigVerified(layout api.Layout) (LayoutApplyResult, error) {
	intent, preMatched, err := m.applyLayout(layout)
	if err != nil {
		return LayoutApplyResult{Outcome: OutcomeUnverified, Detail: err.Error()}, err
	}
	return m.verifyApplied(intent, preMatched), nil
}

// applyLayout performs the apply and returns the configuration it asked for
// (keyed by connector) plus whether the display already matched it beforehand.
func (m *MutterManager) applyLayout(layout api.Layout) (map[string]intentMonitor, bool, error) {
	// GNOME only offers fractional scales (1.25, 1.5, …) while the
	// "scale-monitor-framebuffer" experimental feature is enabled; with it off,
	// only integer scales are available and Mutter uses the sharper physical
	// layout mode. Enable the feature exactly when this layout needs it, and turn
	// it back off otherwise so integer-only layouts don't pay the fractional
	// rendering cost. This may change the serial and the modes' supported-scale
	// lists, so it must happen before we read state and build the request.
	wantFractional := layoutNeedsFractional(layout)
	changed, err := setFractionalScaling(wantFractional)
	if err != nil {
		log.Printf("Failed to set fractional scaling to %v: %v", wantFractional, err)
	}
	if changed {
		// Mutter processes the gsettings change asynchronously; wait for it to
		// take effect so the state we read next carries a fresh serial (a stale
		// one makes ApplyMonitorsConfig fail) and the updated supported-scale
		// lists (needed to snap the scale correctly).
		m.waitForFractionalScales(wantFractional)
	}

	serial, monitors, preLogical, targetMode, err := m.getCurrentState()
	if err != nil {
		return nil, false, err
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
	var persist []persistLogicalMonitor
	intent := make(map[string]intentMonitor, len(layout.Monitors))
	for _, lm := range layout.Monitors {
		mon := resolveMonitor(lm, byEDID, byConnector)
		if mon == nil {
			return nil, false, errors.Errorf("layout monitor %q (edid=%q port=%q) is not connected", lm.Name, lm.Edid, lm.Port)
		}

		mode := pickMode(mon, lm)
		if mode == nil {
			return nil, false, errors.Errorf("no matching mode %dx%d@%.2f for monitor %q", lm.Width, lm.Height, lm.RefreshRate, mon.Spec.Connector)
		}

		// Snap the layout's saved scale to one the picked mode actually supports;
		// Mutter rejects an ApplyMonitorsConfig whose scale isn't in the list.
		scale := pickScale(mode, lm.Scale)

		// Layouts store logical positions; convert them into whatever coordinate
		// space the target layout mode expects (physical pixels when fractional
		// scaling is off). monitors.xml uses the same space, so persist it too.
		x := fromLogicalCoord(lm.PositionX, scale, targetMode)
		y := fromLogicalCoord(lm.PositionY, scale, targetMode)

		logicals = append(logicals, applyLogicalMonitor{
			X:         x,
			Y:         y,
			Scale:     scale,
			Transform: 0,
			Primary:   lm.Primary,
			Monitors: []applyMonitor{{
				Connector:  mon.Spec.Connector,
				Mode:       mode.ID,
				Properties: map[string]dbus.Variant{},
			}},
		})
		persist = append(persist, persistLogicalMonitor{
			spec:    mon.Spec,
			x:       x,
			y:       y,
			width:   mode.Width,
			height:  mode.Height,
			rate:    mode.RefreshRate,
			scale:   scale,
			primary: lm.Primary,
		})
		intent[mon.Spec.Connector] = intentMonitor{x: x, y: y, scale: scale}
	}

	if len(logicals) == 0 {
		return nil, false, errors.New("layout has no applicable monitors")
	}

	// Whether the display server already considered this layout active. If so the
	// apply below is a no-op, which is worth surfacing: it means a "successful"
	// switch changed nothing, and any disagreement with the screen is drift in the
	// display server's own state.
	preMatched := stateMatchesIntent(preLogical, intent)

	obj := m.conn.Object(mutterBusName, dbus.ObjectPath(mutterObjectPath))
	// Apply with the TEMPORARY method, not PERSISTENT. PERSISTENT makes Mutter
	// apply the config and then emit confirm-display-change, which triggers
	// GNOME Shell's "Keep these display settings?" dialog with a countdown that
	// auto-reverts to the previous layout if not confirmed in time. TEMPORARY
	// applies the switch immediately with no confirmation prompt; we then persist
	// it ourselves by writing monitors.xml (below) so it still survives a reboot.
	call := obj.Call(mutterInterface+".ApplyMonitorsConfig", 0,
		serial,
		uint32(mutterMethodTemporary),
		logicals,
		map[string]dbus.Variant{},
	)
	if call.Err != nil {
		return nil, false, errors.Wrap(call.Err, "ApplyMonitorsConfig failed")
	}

	// Best-effort persistence: the layout is already applied, so a failure to
	// write monitors.xml only means it won't be restored after a reboot — don't
	// fail the switch over it.
	if err := writeMonitorsXML(persist, monitors); err != nil {
		log.Printf("Applied layout but failed to persist to monitors.xml: %v", err)
	}
	return intent, preMatched, nil
}

// intentMonitor is the placement we asked Mutter for, used to check afterwards
// whether the display actually ended up that way.
type intentMonitor struct {
	x, y  int32
	scale float64
}

// How long to keep watching the display after an apply, and how often to sample.
// Mutter has been observed accepting a configuration, applying it, and then
// silently rolling it back roughly two seconds later, so a single check straight
// after the call reports success for a layout that does not survive. The window
// must comfortably outlast that rollback.
const (
	verifySettleWindow = 3 * time.Second
	verifyPollInterval = 250 * time.Millisecond
)

// stateMatchesIntent reports whether the display server's logical monitors match
// the configuration we asked for: exactly the same set of connectors, each at the
// requested position and scale.
func stateMatchesIntent(logical []mutterLogicalMonitor, intent map[string]intentMonitor) bool {
	seen := 0
	for _, lm := range logical {
		for _, spec := range lm.Monitors {
			want, ok := intent[spec.Connector]
			if !ok {
				return false // a monitor is enabled that the layout wanted off
			}
			if lm.X != want.x || lm.Y != want.y || math.Abs(lm.Scale-want.scale) > 1e-6 {
				return false
			}
			seen++
		}
	}
	return seen == len(intent)
}

// verifyApplied watches the display for a settle window after an apply and
// reports what actually happened. ApplyMonitorsConfig returning success only
// means the request was accepted — it is not proof the layout stuck, so this
// keeps sampling rather than trusting a single post-apply read.
func (m *MutterManager) verifyApplied(intent map[string]intentMonitor, preMatched bool) LayoutApplyResult {
	deadline := time.Now().Add(verifySettleWindow)
	everMatched := preMatched
	matched := preMatched
	readOK := false

	for {
		if _, _, logical, _, err := m.getCurrentState(); err == nil {
			readOK = true
			matched = stateMatchesIntent(logical, intent)
			if matched {
				everMatched = true
			}
		}
		if !time.Now().Before(deadline) {
			break
		}
		time.Sleep(verifyPollInterval)
	}

	switch {
	case !readOK:
		return LayoutApplyResult{OutcomeUnverified, "could not read display state back"}
	case matched && preMatched:
		return LayoutApplyResult{OutcomeAlreadyActive,
			"the display server already reported this layout as active, so nothing changed"}
	case matched:
		return LayoutApplyResult{OutcomeApplied, "layout is active"}
	case everMatched:
		return LayoutApplyResult{OutcomeRolledBack,
			"layout was applied but the display server reverted it within " + verifySettleWindow.String()}
	default:
		return LayoutApplyResult{OutcomeMismatch,
			"the request was accepted but the display never matched the layout"}
	}
}

// Mutter logical-monitor layout modes (org.gnome.Mutter.DisplayConfig). They
// determine the coordinate space of logical-monitor positions: LOGICAL uses
// scaled pixels (Wayland with fractional scaling), PHYSICAL uses device pixels
// (integer scaling with fractional scaling off).
const (
	layoutModeLogical  uint32 = 1
	layoutModePhysical uint32 = 2
)

// toLogicalCoord converts a position component as reported by Mutter into
// logical pixels. In physical layout mode positions are device pixels, so they
// are divided by the monitor's scale; in logical mode they are already logical.
func toLogicalCoord(v int32, scale float64, mode uint32) int {
	if mode == layoutModePhysical && scale > 0 {
		return int(math.Round(float64(v) / scale))
	}
	return int(v)
}

// fromLogicalCoord is the inverse of toLogicalCoord: it converts a stored
// logical position into the coordinate space Mutter expects for the target
// layout mode (multiplying by scale in physical mode).
func fromLogicalCoord(v int, scale float64, mode uint32) int32 {
	if mode == layoutModePhysical && scale > 0 {
		return int32(math.Round(float64(v) * scale))
	}
	return int32(v)
}

// mutterFractionalScalingFeature is the org.gnome.mutter experimental feature
// that unlocks non-integer display scales (and switches Mutter to the logical
// framebuffer layout mode).
const mutterFractionalScalingFeature = "scale-monitor-framebuffer"

// layoutNeedsFractional reports whether any monitor in the layout uses a
// non-integer scale, which GNOME only honours with fractional scaling enabled.
func layoutNeedsFractional(layout api.Layout) bool {
	for _, lm := range layout.Monitors {
		if isFractionalScale(lm.Scale) {
			return true
		}
	}
	return false
}

// isFractionalScale reports whether s is a set, non-integer scale.
func isFractionalScale(s float64) bool {
	return s > 0 && math.Abs(s-math.Round(s)) > 1e-6
}

// pickScale snaps the layout's saved scale to the nearest value the chosen mode
// actually supports. An unset scale (0, e.g. a layout saved before scale was
// captured) falls back to the mode's preferred scale, else 1.0.
func pickScale(mode *mutterMode, want float64) float64 {
	if want <= 0 {
		if mode.PreferredScale > 0 {
			return mode.PreferredScale
		}
		return 1.0
	}
	if len(mode.SupportedScales) == 0 {
		return want
	}
	best := mode.SupportedScales[0]
	bestDelta := math.Abs(best - want)
	for _, s := range mode.SupportedScales[1:] {
		if d := math.Abs(s - want); d < bestDelta {
			best, bestDelta = s, d
		}
	}
	return best
}

// setFractionalScaling enables or disables GNOME's fractional-scaling
// experimental feature, preserving any other experimental features already set.
// It reports whether it actually changed the setting. Best-effort: a missing
// gsettings binary or mutter schema surfaces as an error for the caller to log.
func setFractionalScaling(enable bool) (bool, error) {
	features, err := getMutterExperimentalFeatures()
	if err != nil {
		return false, err
	}
	has := false
	out := make([]string, 0, len(features)+1)
	for _, f := range features {
		if f == mutterFractionalScalingFeature {
			has = true
			continue // re-added below iff enabling
		}
		out = append(out, f)
	}
	if enable == has {
		return false, nil // already in the desired state
	}
	if enable {
		out = append(out, mutterFractionalScalingFeature)
	}
	return true, writeMutterExperimentalFeatures(out)
}

// waitForFractionalScales blocks (briefly) until Mutter has processed a
// fractional-scaling toggle, i.e. until the connected monitors' modes advertise
// (or stop advertising) fractional scales. This guarantees the next
// GetCurrentState carries a fresh serial and the up-to-date supported-scale list.
func (m *MutterManager) waitForFractionalScales(want bool) {
	for i := 0; i < 20; i++ {
		if _, monitors, _, _, err := m.getCurrentState(); err == nil && anyModeHasFractional(monitors) == want {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func anyModeHasFractional(monitors []mutterMonitor) bool {
	for i := range monitors {
		for j := range monitors[i].Modes {
			for _, s := range monitors[i].Modes[j].SupportedScales {
				if isFractionalScale(s) {
					return true
				}
			}
		}
	}
	return false
}

// getMutterExperimentalFeatures reads org.gnome.mutter's experimental-features
// key via gsettings, returning the currently-enabled feature names.
func getMutterExperimentalFeatures() ([]string, error) {
	out, err := exec.Command("gsettings", "get", "org.gnome.mutter", "experimental-features").Output()
	if err != nil {
		return nil, errors.Wrap(err, "gsettings get experimental-features")
	}
	return parseGSettingsStringArray(string(out)), nil
}

func writeMutterExperimentalFeatures(features []string) error {
	quoted := make([]string, len(features))
	for i, f := range features {
		quoted[i] = "'" + f + "'"
	}
	val := "[" + strings.Join(quoted, ", ") + "]"
	if err := exec.Command("gsettings", "set", "org.gnome.mutter", "experimental-features", val).Run(); err != nil {
		return errors.Wrap(err, "gsettings set experimental-features")
	}
	return nil
}

// parseGSettingsStringArray extracts the single-quoted tokens from a gsettings
// array literal such as "['scale-monitor-framebuffer']" or "@as []".
func parseGSettingsStringArray(s string) []string {
	var res []string
	for {
		i := strings.IndexByte(s, '\'')
		if i < 0 {
			break
		}
		s = s[i+1:]
		j := strings.IndexByte(s, '\'')
		if j < 0 {
			break
		}
		res = append(res, s[:j])
		s = s[j+1:]
	}
	return res
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
