package agent

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/trolleyman/ottoman/internal/api"
	"github.com/trolleyman/ottoman/internal/ddc"
	"github.com/trolleyman/ottoman/internal/store"
)

// ddcCacheTTL bounds how often we run the (slow) `ddcutil detect`.
const ddcCacheTTL = 60 * time.Second

// brightnessCacheTTL bounds how often we probe a monitor's brightness, so that
// polling GetMonitors doesn't spawn a ddcutil getvcp on every request.
const brightnessCacheTTL = 60 * time.Second

// monitorControl maps physical monitors (by EDID) to their control backend and
// dispatches brightness/power operations. DDC monitors are matched to an i2c
// bus via ddcutil; the TV backend is wired in separately.
type monitorControl struct {
	registry *store.Registry
	tv       *tvManager

	mu         sync.Mutex
	ddcCache   []ddc.Display
	ddcFetched time.Time
	brightness map[string]brightnessSample
}

type brightnessSample struct {
	value int
	at    time.Time
}

func newMonitorControl(reg *store.Registry) *monitorControl {
	return &monitorControl{
		registry:   reg,
		brightness: make(map[string]brightnessSample),
	}
}

// ddcDisplays returns the cached DDC display list, refreshing if stale.
func (c *monitorControl) ddcDisplays() []ddc.Display {
	if !ddc.Available() {
		return nil
	}
	c.mu.Lock()
	fresh := c.ddcCache != nil && time.Since(c.ddcFetched) < ddcCacheTTL
	cached := c.ddcCache
	c.mu.Unlock()
	if fresh {
		return cached
	}

	displays, err := ddc.Detect()
	if err != nil {
		log.Printf("ddcutil detect failed: %v", err)
		return cached
	}
	c.mu.Lock()
	c.ddcCache = displays
	c.ddcFetched = time.Now()
	c.mu.Unlock()
	return displays
}

// ddcBusFor returns the i2c bus for a monitor EDID, if a DDC display matches.
func (c *monitorControl) ddcBusFor(edid string) (int, bool) {
	for _, d := range c.ddcDisplays() {
		if ddcMatches(edid, d) {
			return d.Bus, true
		}
	}
	return 0, false
}

// backendFor resolves the control backend for a monitor: an explicit registry
// override wins, otherwise it's auto-detected (DDC match -> ddc, else none).
func (c *monitorControl) backendFor(edid string) string {
	if e, ok := c.registry.Get(edid); ok && e.Backend != "" {
		return e.Backend
	}
	if _, ok := c.ddcBusFor(edid); ok {
		return store.BackendDDC
	}
	return store.BackendNone
}

// ddcGetBrightness/ddcSetBrightness/ddcSetPower dispatch a DDC-family operation
// to either the ddcutil CLI (BackendDDC) or the direct-I2C transport
// (BackendI2C). Both drive the same monitors over the same i2c bus; direct I2C
// just skips the per-call ddcutil process spawn that makes dragging laggy.
func ddcGetBrightness(backend string, bus int) (int, error) {
	if backend == store.BackendI2C {
		return ddc.GetBrightnessDirect(bus)
	}
	return ddc.GetBrightness(bus)
}

func ddcSetBrightness(backend string, bus, percent int) error {
	if backend == store.BackendI2C {
		return ddc.SetBrightnessDirect(bus, percent)
	}
	return ddc.SetBrightness(bus, percent)
}

func ddcSetPower(backend string, bus int, on bool) error {
	if backend == store.BackendI2C {
		return ddc.SetPowerDirect(bus, on)
	}
	return ddc.SetPower(bus, on)
}

// capabilities reports which controls a monitor supports, based on its backend.
func (c *monitorControl) capabilities(edid string) api.MonitorCapabilities {
	switch c.backendFor(edid) {
	case store.BackendDDC, store.BackendI2C:
		// A DDC monitor that isn't currently on an i2c bus is disconnected (e.g.
		// a remembered layout monitor that's unplugged) — it can't be controlled
		// over DDC, so advertise no controls rather than switches that error.
		if _, ok := c.ddcBusFor(edid); !ok {
			return api.MonitorCapabilities{}
		}
		return api.MonitorCapabilities{Brightness: true, Power: true, Volume: false}
	case store.BackendTV:
		return api.MonitorCapabilities{Brightness: true, Power: true, Volume: true}
	default:
		return api.MonitorCapabilities{}
	}
}

// currentBrightness returns a monitor's brightness (0-100), using a throttled
// cache to avoid hammering the backend during polling. It live-reads DDC
// (getvcp) or the TV (getSystemSettings) once per TTL and, if that read fails
// or the firmware doesn't support it, falls back to the last value we set.
func (c *monitorControl) currentBrightness(edid string) (int, bool) {
	backend := c.backendFor(edid)
	if backend != store.BackendDDC && backend != store.BackendI2C && backend != store.BackendTV {
		return 0, false
	}

	c.mu.Lock()
	sample, cached := c.brightness[edid]
	fresh := cached && time.Since(sample.at) < brightnessCacheTTL
	c.mu.Unlock()
	if fresh {
		return sample.value, true
	}

	switch backend {
	case store.BackendDDC, store.BackendI2C:
		bus, ok := c.ddcBusFor(edid)
		if !ok {
			return 0, false
		}
		v, err := ddcGetBrightness(backend, bus)
		if err != nil {
			log.Printf("ddc GetBrightness(%s) failed: %v", edid, err)
			if cached {
				return sample.value, true // fall back to last-known
			}
			return 0, false
		}
		c.setBrightnessSample(edid, v)
		return v, true
	case store.BackendTV:
		if c.tv == nil {
			return 0, false
		}
		v, live := c.tv.Backlight(context.Background(), edid)
		if !live {
			// TV off/unreachable or firmware doesn't support the read — fall
			// back to the last value we set (if any).
			if cached {
				return sample.value, true
			}
			return 0, false
		}
		c.setBrightnessSample(edid, v)
		return v, true
	}
	return 0, false
}

func (c *monitorControl) setBrightnessSample(edid string, v int) {
	c.mu.Lock()
	c.brightness[edid] = brightnessSample{value: v, at: time.Now()}
	c.mu.Unlock()
}

// setBrightness sets brightness (0-100) on a monitor. For a TV-backed monitor
// this drives the OLED backlight.
func (c *monitorControl) setBrightness(edid string, percent int) error {
	backend := c.backendFor(edid)
	switch backend {
	case store.BackendDDC, store.BackendI2C:
		bus, ok := c.ddcBusFor(edid)
		if !ok {
			return errors.Errorf("no DDC display matches monitor %q", edid)
		}
		if err := ddcSetBrightness(backend, bus, percent); err != nil {
			return err
		}
		c.setBrightnessSample(edid, percent)
		return nil
	case store.BackendTV:
		if c.tv == nil {
			return errors.New("no TV controller configured")
		}
		if err := c.tv.SetBacklight(context.Background(), edid, percent); err != nil {
			return err
		}
		c.setBrightnessSample(edid, percent) // remember it — the read may not be supported
		return nil
	default:
		return errors.Errorf("brightness control not available for monitor %q", edid)
	}
}

// setPower turns a monitor on or off (standby / TV power).
func (c *monitorControl) setPower(edid string, on bool) error {
	backend := c.backendFor(edid)
	switch backend {
	case store.BackendDDC, store.BackendI2C:
		bus, ok := c.ddcBusFor(edid)
		if !ok {
			return errors.Errorf("no DDC display matches monitor %q", edid)
		}
		return ddcSetPower(backend, bus, on)
	case store.BackendTV:
		if c.tv == nil {
			return errors.New("no TV controller configured")
		}
		if on {
			return c.tv.PowerOn(edid)
		}
		return c.tv.PowerOff(context.Background(), edid)
	default:
		return errors.Errorf("power control not available for monitor %q", edid)
	}
}

// responding probes whether a monitor currently answers its control backend,
// which the UI treats as a proxy for "powered on": a DDC monitor in standby
// stops ACKing over i2c, and a TV that's off drops its network connection. Used
// to confirm a power toggle by polling until the state flips (there's no direct
// power-state read). The probe can block for the backend's own timeout.
func (c *monitorControl) responding(edid string) bool {
	backend := c.backendFor(edid)
	switch backend {
	case store.BackendDDC, store.BackendI2C:
		bus, ok := c.ddcBusFor(edid)
		if !ok {
			return false
		}
		v, err := ddcGetBrightness(backend, bus)
		if err != nil {
			return false
		}
		c.setBrightnessSample(edid, v) // a successful read doubles as a refresh
		return true
	case store.BackendTV:
		if c.tv == nil {
			return false
		}
		return c.tv.Reachable(context.Background(), edid)
	default:
		return false
	}
}

// enrich augments monitor entries with registry + control metadata for the UI.
func (c *monitorControl) enrich(ctx context.Context, monitors []api.Monitor) []api.Monitor {
	for i := range monitors {
		edid := monitors[i].Edid
		if edid == "" {
			continue
		}

		backend := c.backendFor(edid)
		// Persist a default registry entry the first time we see a monitor.
		if _, err := c.registry.Ensure(edid, backend); err != nil {
			log.Printf("registry ensure failed for %q: %v", edid, err)
		}
		entry, _ := c.registry.Get(edid)

		monitors[i].ControlBackend = strPtr(backend)
		caps := c.capabilities(edid)
		monitors[i].Capabilities = &caps

		if entry.FriendlyName != "" {
			monitors[i].FriendlyName = strPtr(entry.FriendlyName)
		}
		if entry.Visibility != nil {
			vis := entry.Visibility
			monitors[i].Visibility = &vis
		}
		if entry.TV != nil {
			monitors[i].Tv = &api.TVConn{
				Type: strPtr(entry.TV.Type),
				Host: strPtr(entry.TV.Host),
				Mac:  strPtr(entry.TV.Mac),
			}
		}
		if caps.Brightness {
			if v, ok := c.currentBrightness(edid); ok {
				monitors[i].Brightness = intPtr(v)
			} else {
				monitors[i].Brightness = intPtr(-1)
			}
		}
		// Live TV integration state (pairing, volume, panel power), so TV
		// control needs no separate endpoint.
		if backend == store.BackendTV && c.tv != nil {
			if st, ok := c.tv.StateFor(ctx, edid); ok {
				monitors[i].TvState = &st
			}
		}
	}
	return monitors
}

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

// splitEDID splits a "vendor:product:serial" identifier.
func splitEDID(edid string) (vendor, product, serial string) {
	parts := strings.SplitN(edid, ":", 3)
	if len(parts) > 0 {
		vendor = parts[0]
	}
	if len(parts) > 1 {
		product = parts[1]
	}
	if len(parts) > 2 {
		serial = parts[2]
	}
	return
}

// ddcMatches reports whether a DDC display corresponds to a mutter EDID
// ("vendor:product:serial"). Matching is heuristic (ddcutil and Mutter format
// the same EDID fields slightly differently): the manufacturer must match, then
// serial (if both known) or model name is used to disambiguate.
func ddcMatches(edid string, d ddc.Display) bool {
	vendor, product, serial := splitEDID(edid)
	if vendor != "" && d.Mfg != "" && !strings.EqualFold(strings.TrimSpace(d.Mfg), strings.TrimSpace(vendor)) {
		return false
	}
	if serial != "" && d.Serial != "" {
		return strings.EqualFold(strings.TrimSpace(d.Serial), strings.TrimSpace(serial))
	}
	if product != "" && d.Model != "" {
		p := strings.ToLower(strings.TrimSpace(product))
		m := strings.ToLower(strings.TrimSpace(d.Model))
		return p == m || strings.Contains(p, m) || strings.Contains(m, p)
	}
	// Manufacturer matched and nothing better to compare on.
	return true
}
