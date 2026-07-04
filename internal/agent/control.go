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
	tv       *tvController

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

// capabilities reports which controls a monitor supports, based on its backend.
func (c *monitorControl) capabilities(edid string) api.MonitorCapabilities {
	switch c.backendFor(edid) {
	case store.BackendDDC:
		return api.MonitorCapabilities{Brightness: true, Power: true, Volume: false}
	case store.BackendTV:
		return api.MonitorCapabilities{Brightness: true, Power: true, Volume: true}
	default:
		return api.MonitorCapabilities{}
	}
}

// currentBrightness returns a monitor's brightness (0-100), using a throttled
// cache to avoid hammering ddcutil during polling.
func (c *monitorControl) currentBrightness(edid string) (int, bool) {
	if c.backendFor(edid) != store.BackendDDC {
		return 0, false
	}

	c.mu.Lock()
	sample, ok := c.brightness[edid]
	fresh := ok && time.Since(sample.at) < brightnessCacheTTL
	c.mu.Unlock()
	if fresh {
		return sample.value, true
	}

	bus, ok := c.ddcBusFor(edid)
	if !ok {
		return 0, false
	}
	v, err := ddc.GetBrightness(bus)
	if err != nil {
		log.Printf("ddc GetBrightness(%s) failed: %v", edid, err)
		if ok {
			return sample.value, true // fall back to last-known
		}
		return 0, false
	}
	c.setBrightnessSample(edid, v)
	return v, true
}

func (c *monitorControl) setBrightnessSample(edid string, v int) {
	c.mu.Lock()
	c.brightness[edid] = brightnessSample{value: v, at: time.Now()}
	c.mu.Unlock()
}

// setBrightness sets brightness (0-100) on a monitor. For a TV-backed monitor
// this drives the OLED backlight.
func (c *monitorControl) setBrightness(edid string, percent int) error {
	switch c.backendFor(edid) {
	case store.BackendDDC:
		bus, ok := c.ddcBusFor(edid)
		if !ok {
			return errors.Errorf("no DDC display matches monitor %q", edid)
		}
		if err := ddc.SetBrightness(bus, percent); err != nil {
			return err
		}
		c.setBrightnessSample(edid, percent)
		return nil
	case store.BackendTV:
		if c.tv == nil {
			return errors.New("no TV controller configured")
		}
		return c.tv.SetBacklight(context.Background(), percent)
	default:
		return errors.Errorf("brightness control not available for monitor %q", edid)
	}
}

// setPower turns a monitor on or off (standby / TV power).
func (c *monitorControl) setPower(edid string, on bool) error {
	switch c.backendFor(edid) {
	case store.BackendDDC:
		bus, ok := c.ddcBusFor(edid)
		if !ok {
			return errors.Errorf("no DDC display matches monitor %q", edid)
		}
		return ddc.SetPower(bus, on)
	case store.BackendTV:
		if c.tv == nil {
			return errors.New("no TV controller configured")
		}
		if on {
			return c.tv.PowerOn()
		}
		return c.tv.PowerOff(context.Background())
	default:
		return errors.Errorf("power control not available for monitor %q", edid)
	}
}

// enrich augments monitor entries with registry + control metadata for the UI.
func (c *monitorControl) enrich(monitors []api.Monitor) []api.Monitor {
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
