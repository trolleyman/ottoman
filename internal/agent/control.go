package agent

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/trolleyman/ottoman/internal/api"
	"github.com/trolleyman/ottoman/internal/ddc"
	"github.com/trolleyman/ottoman/internal/store"
)

// ddcCacheTTL bounds how often we run display detection (native EDID reads over
// i2c, see ddc.DetectDirect). It's long because the display topology rarely
// changes (a DDC power-off keeps the monitor on the bus) and probing still does
// some bus i/o. Stale refreshes run in the background, off the request path, so
// a poll never waits on — or is gated behind — a detect.
const ddcCacheTTL = 10 * time.Minute

// brightnessCacheTTL bounds how often we probe a monitor's brightness. Live DDC
// getvcp / TV reads jitter the display bus and stutter the compositor, so this
// is long: our own writes keep the sample authoritative (see setBrightnessSample),
// and this cache only lags an external change made via the monitor's own OSD.
const brightnessCacheTTL = 1 * time.Hour

// tvStateCacheTTL bounds how often we query a webOS TV's live state (power,
// volume) over the network. Like the brightness cache it keeps that I/O off the
// request path: a powered-off TV takes up to tvDialTimeout (5s) to fail a dial,
// which must never block a /api/monitors poll — so we refresh in the background
// and serve the last-known state meanwhile.
const tvStateCacheTTL = 10 * time.Second

// ddcMissCooldown rate-limits the re-detect kicked when a monitor we're asked
// about isn't in the cached topology. Without it, a genuinely absent monitor
// (a remembered layout entry that's unplugged) would re-detect on every poll.
const ddcMissCooldown = 30 * time.Second

// ddcRefreshWedged bounds how long the single in-flight detect slot may be held
// before we assume the goroutine holding it is stuck and claim it anyway. An
// i2c read blocks in the kernel with no timeout of its own, so a wedged bus must
// not be able to freeze display detection for the life of the process.
const ddcRefreshWedged = 2 * time.Minute

// monitorControl maps physical monitors (by EDID) to their control backend and
// dispatches brightness/power operations. DDC monitors are matched to an i2c
// bus via ddcutil; the TV backend is wired in separately.
type monitorControl struct {
	registry *store.Registry
	tv       *tvManager

	mu                   sync.Mutex
	ddcCache             []ddc.Display
	ddcFetched           time.Time
	ddcRefreshing        bool      // a background detect is in flight
	ddcRefreshAt         time.Time // when that detect claimed the slot
	ddcSummary           string    // last logged topology, to log only on change
	ddcBusMemory         map[string]int
	ddcMissAt            map[string]time.Time
	brightness           map[string]brightnessSample
	brightnessRefreshing map[string]bool // per-EDID background read in flight
	tvState              map[string]tvStateSample
	tvStateRefreshing    map[string]bool // per-EDID background TV query in flight
}

type brightnessSample struct {
	value int
	at    time.Time
	ok    bool // whether the last read succeeded (false = negative-cached failure)
}

type tvStateSample struct {
	state api.MonitorTVState
	at    time.Time
}

func newMonitorControl(reg *store.Registry) *monitorControl {
	return &monitorControl{
		registry:             reg,
		ddcBusMemory:         make(map[string]int),
		ddcMissAt:            make(map[string]time.Time),
		brightness:           make(map[string]brightnessSample),
		brightnessRefreshing: make(map[string]bool),
		tvState:              make(map[string]tvStateSample),
		tvStateRefreshing:    make(map[string]bool),
	}
}

// ddcDisplays returns the cached DDC display list, refreshing if stale.
func (c *monitorControl) ddcDisplays() []ddc.Display {
	if !ddc.Available() {
		return nil
	}
	c.mu.Lock()
	cached := c.ddcCache
	fresh := cached != nil && time.Since(c.ddcFetched) < ddcCacheTTL
	if fresh {
		c.mu.Unlock()
		return cached
	}
	if cached == nil {
		// Nothing known yet: detect once synchronously so the first poll after
		// startup has real topology. Recurring refreshes go to the background.
		c.mu.Unlock()
		return c.detectAndStore()
	}
	// Stale but known: refresh off the request path (detect churns the i2c bus
	// and stutters the compositor, and can take a while) and serve the
	// last-known topology now. If a prior detect has held the in-flight slot
	// past ddcRefreshWedged it's presumed stuck on a dead bus — claim it anyway
	// so detection can't wedge permanently.
	if !c.ddcRefreshing || time.Since(c.ddcRefreshAt) > ddcRefreshWedged {
		c.startDetectLocked()
	}
	c.mu.Unlock()
	return cached
}

// startDetectLocked launches a background detect and marks the in-flight slot.
// Must be called with c.mu held.
func (c *monitorControl) startDetectLocked() {
	c.ddcRefreshing = true
	c.ddcRefreshAt = time.Now()
	go c.detectAndStore()
}

// detectAndStore runs display detection and updates the cache. Safe to call in
// a background goroutine; it clears the in-flight guard on the way out.
//
// Two sources are merged. ddc.Detect reads EDIDs live over i2c and yields a bus
// we can address for DDC/CI; ddc.SysfsDisplays reads the kernel's cached EDIDs
// and keeps naming a monitor even when its live i2c read has gone quiet (see
// SysfsDisplays). A monitor present in sysfs but missing a usable bus is matched
// to a bus we've addressed it on before (ddcBusMemory), so a monitor that drops
// off i2c mid-session stays controllable on its last-known bus.
func (c *monitorControl) detectAndStore() []ddc.Display {
	direct, err := ddc.Detect()
	if err != nil {
		// A live-detect error is the degraded case this merge exists for: the
		// i2c reads failed and the ddcutil fallback failed too. Don't discard
		// sysfs over it — carry on with no direct entries and let sysfs +
		// remembered buses keep known monitors addressable.
		log.Printf("live display detect failed (using sysfs + last-known buses): %v", err)
		direct = nil
	}
	sysfs := ddc.SysfsDisplays()

	c.mu.Lock()
	defer c.mu.Unlock()
	c.ddcRefreshing = false

	displays := c.mergeDisplays(direct, sysfs)
	c.ddcFetched = time.Now()
	if len(displays) == 0 && len(c.ddcCache) > 0 {
		// Everything came back empty (a transient total failure); keep the last
		// known-good topology rather than blanking every monitor's controls. A
		// specific control request still forces recovery via kickRedetect.
		return c.ddcCache
	}
	c.ddcCache = displays
	if summary := summarizeDisplays(displays); summary != c.ddcSummary {
		log.Printf("DDC topology: %s", summary)
		c.ddcSummary = summary
	}
	return displays
}

// mergeDisplays folds the live-i2c and sysfs display lists into one entry per
// physical monitor, preferring a live bus and otherwise a last-known one. Must
// be called with c.mu held.
func (c *monitorControl) mergeDisplays(direct, sysfs []ddc.Display) []ddc.Display {
	byKey := map[string]ddc.Display{}
	order := []string{}
	add := func(d ddc.Display) {
		k := displayKey(d)
		if existing, ok := byKey[k]; ok {
			// Keep whichever entry carries a usable bus (direct reads first).
			if existing.Bus < 0 && d.Bus >= 0 {
				byKey[k] = d
			}
			return
		}
		byKey[k] = d
		order = append(order, k)
	}
	// Direct entries first so their live bus wins over a sysfs -1.
	for _, d := range direct {
		add(d)
	}
	for _, d := range sysfs {
		add(d)
	}

	out := make([]ddc.Display, 0, len(order))
	for _, k := range order {
		d := byKey[k]
		if d.Bus >= 0 {
			c.ddcBusMemory[k] = d.Bus // remember where we can reach it
		} else if bus, ok := c.ddcBusMemory[k]; ok {
			d.Bus = bus // fall back to the last bus we addressed it on
		}
		out = append(out, d)
	}
	return out
}

// ddcBusFor returns the i2c bus for a monitor EDID, if a DDC display matches and
// we know a bus we can address it on. A match with no usable bus (a monitor
// seen only in sysfs, never on i2c) is treated as a miss, but kicks a
// rate-limited re-detect so a monitor that (re)appears is picked up well before
// the long cache TTL lapses.
func (c *monitorControl) ddcBusFor(edid string) (int, bool) {
	displays := c.ddcDisplays()
	for _, d := range displays {
		if ddcMatches(edid, d) && d.Bus >= 0 {
			return d.Bus, true
		}
	}
	c.kickRedetect(edid)
	return 0, false
}

// kickRedetect launches a background detect when we're asked for a monitor we
// can't currently address, at most once per ddcMissCooldown per monitor.
func (c *monitorControl) kickRedetect(edid string) {
	if !ddc.Available() {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ddcRefreshing && time.Since(c.ddcRefreshAt) <= ddcRefreshWedged {
		return
	}
	if last, ok := c.ddcMissAt[edid]; ok && time.Since(last) < ddcMissCooldown {
		return
	}
	c.ddcMissAt[edid] = time.Now()
	c.startDetectLocked()
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

// currentBrightness returns a monitor's brightness (0-100) from a cache, never
// touching the bus or network on the request path: whenever the entry isn't
// fresh it kicks a single background read (see refreshBrightness) and serves the
// last-known value immediately. A live DDC getvcp jitters the display bus and a
// TV backlight read goes over the network, so neither must block (or stutter) a
// poll. Returns ok=false only until the first read has populated the cache (or
// when it permanently fails — negative-cached so it isn't re-probed each poll).
func (c *monitorControl) currentBrightness(edid string) (int, bool) {
	backend := c.backendFor(edid)
	if backend != store.BackendDDC && backend != store.BackendI2C && backend != store.BackendTV {
		return 0, false
	}

	c.mu.Lock()
	sample, have := c.brightness[edid]
	fresh := have && time.Since(sample.at) < brightnessCacheTTL
	if !fresh && !c.brightnessRefreshing[edid] {
		c.brightnessRefreshing[edid] = true
		go c.refreshBrightness(edid, backend)
	}
	c.mu.Unlock()
	if have {
		return sample.value, sample.ok
	}
	return 0, false // cold: nothing known yet; the background read will populate it
}

// readBrightnessLive performs the actual (bus-touching) brightness read for a
// monitor. It does no caching; callers cache the result via storeBrightness.
func (c *monitorControl) readBrightnessLive(edid, backend string) (int, bool) {
	switch backend {
	case store.BackendDDC, store.BackendI2C:
		bus, ok := c.ddcBusFor(edid)
		if !ok {
			return 0, false
		}
		v, err := ddcGetBrightness(backend, bus)
		if err != nil {
			log.Printf("ddc GetBrightness(%s) failed: %v", edid, err)
			return 0, false
		}
		return v, true
	case store.BackendTV:
		if c.tv == nil {
			return 0, false
		}
		return c.tv.Backlight(context.Background(), edid)
	}
	return 0, false
}

// refreshBrightness reads brightness in the background and updates the cache,
// clearing the per-EDID in-flight guard when done.
func (c *monitorControl) refreshBrightness(edid, backend string) {
	v, ok := c.readBrightnessLive(edid, backend)
	c.mu.Lock()
	c.brightnessRefreshing[edid] = false
	c.mu.Unlock()
	c.storeBrightness(edid, v, ok)
}

// storeBrightness records a read result. A failed read keeps the last-known
// value (so a transient hiccup doesn't blank the UI) but still refreshes the
// timestamp, so the failure is negative-cached and won't be retried until the
// TTL lapses.
func (c *monitorControl) storeBrightness(edid string, v int, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !ok {
		if prev, had := c.brightness[edid]; had {
			v, ok = prev.value, prev.ok
		}
	}
	c.brightness[edid] = brightnessSample{value: v, at: time.Now(), ok: ok}
}

// setBrightnessSample records a brightness we just wrote, as an authoritative
// (ok) sample — so a subsequent poll serves it without a bus-touching read.
func (c *monitorControl) setBrightnessSample(edid string, v int) {
	c.mu.Lock()
	c.brightness[edid] = brightnessSample{value: v, at: time.Now(), ok: true}
	c.mu.Unlock()
}

// tvStateFor returns a TV-backed monitor's live state (power, volume, pairing)
// from a background-refreshed cache. Querying it dials the TV over the network,
// which for a powered-off set can take several seconds, so — like brightness —
// it never runs on the request path: a non-fresh entry triggers a single
// background refresh and the last-known state is served immediately. ok is false
// only until the first refresh has populated the cache.
func (c *monitorControl) tvStateFor(edid string) (api.MonitorTVState, bool) {
	if c.tv == nil {
		return api.MonitorTVState{}, false
	}
	c.mu.Lock()
	sample, have := c.tvState[edid]
	fresh := have && time.Since(sample.at) < tvStateCacheTTL
	if !fresh && !c.tvStateRefreshing[edid] {
		c.tvStateRefreshing[edid] = true
		go c.refreshTVState(edid)
	}
	c.mu.Unlock()
	return sample.state, have
}

// refreshTVState queries a TV's live state in the background and caches it,
// clearing the per-EDID in-flight guard when done.
func (c *monitorControl) refreshTVState(edid string) {
	st, ok := c.tv.StateFor(context.Background(), edid)
	c.mu.Lock()
	c.tvStateRefreshing[edid] = false
	if ok {
		c.tvState[edid] = tvStateSample{state: st, at: time.Now()}
	}
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
		// Live TV integration state (pairing, volume, panel power), so TV
		// control needs no separate endpoint. Served from a background-refreshed
		// cache so a poll never blocks on a TV network dial.
		if backend == store.BackendTV && c.tv != nil {
			if st, ok := c.tvStateFor(edid); ok {
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

// displayKey is a stable identity for a display across the two detection
// sources, so the same physical monitor read live over i2c and from sysfs folds
// to one entry. Bus is deliberately excluded — it's the thing that changes.
func displayKey(d ddc.Display) string {
	return strings.ToLower(strings.TrimSpace(d.Mfg)) + ":" +
		strings.ToLower(strings.TrimSpace(d.Model)) + ":" +
		strings.ToLower(strings.TrimSpace(d.Serial))
}

// summarizeDisplays renders the detected topology for a log line, so a change
// (a monitor gaining or losing a live bus) is visible without dumping on every
// refresh.
func summarizeDisplays(displays []ddc.Display) string {
	if len(displays) == 0 {
		return "no displays"
	}
	parts := make([]string, len(displays))
	for i, d := range displays {
		bus := "no-bus"
		if d.Bus >= 0 {
			bus = "i2c-" + strconv.Itoa(d.Bus)
		}
		parts[i] = fmt.Sprintf("%s/%s@%s", strings.TrimSpace(d.Mfg), strings.TrimSpace(d.Model), bus)
	}
	return strings.Join(parts, ", ")
}
