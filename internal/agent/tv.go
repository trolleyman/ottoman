package agent

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/trolleyman/ottoman/internal/api"
	"github.com/trolleyman/ottoman/internal/store"
	"github.com/trolleyman/ottoman/internal/tv/webos"
)

const (
	// tvDialTimeout bounds a single connection attempt. A powered-off TV drops
	// packets rather than refusing, so an unbounded dial would block on the OS
	// TCP timeout (tens of seconds) — long enough to trip the controller's 10s
	// proxy timeout and 500 the request.
	tvDialTimeout = 5 * time.Second
	// tvDialBackoff suppresses re-dialing after a recent failure, so repeated
	// polls of /api/monitors fail fast instead of each queuing behind another
	// full dial while holding the shared mutex.
	tvDialBackoff = 15 * time.Second
)

// tvManager hands out one tvController per TV-backed monitor (keyed by EDID),
// so multiple network TVs can be driven independently, each with its own SSAP
// connection and pairing key. A TV's transport (host/mac/type) lives on its
// monitor registry entry (backend "tv"), alongside the rest of that monitor's
// settings.
type tvManager struct {
	reg   *store.Registry
	store *store.TVStore

	mu          sync.Mutex
	controllers map[string]*tvController
}

func newTVManager(reg *store.Registry, st *store.TVStore) *tvManager {
	return &tvManager{reg: reg, store: st, controllers: make(map[string]*tvController)}
}

// conn resolves a TV monitor's transport from its registry entry.
func (m *tvManager) conn(edid string) (store.TVConn, bool) {
	e, ok := m.reg.Get(edid)
	if !ok || e.Backend != store.BackendTV || e.TV == nil || e.TV.Host == "" {
		return store.TVConn{}, false
	}
	return *e.TV, true
}

// controller returns the controller for a TV-backed monitor, creating it on
// first use. Fails if the monitor isn't a configured network TV.
func (m *tvManager) controller(edid string) (*tvController, error) {
	if _, ok := m.conn(edid); !ok {
		return nil, errors.Errorf("monitor %q is not a configured network TV", edid)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.controllers[edid]
	if !ok {
		key, _ := m.store.LoadPairingKey(edid)
		c = &tvController{edid: edid, manager: m, key: key}
		m.controllers[edid] = c
	}
	return c, nil
}

// StateFor reports the live TV integration state of a TV-backed monitor, for
// embedding in its /api/monitors entry. ok is false for non-TV monitors.
func (m *tvManager) StateFor(ctx context.Context, edid string) (api.MonitorTVState, bool) {
	c, err := m.controller(edid)
	if err != nil {
		return api.MonitorTVState{}, false
	}
	return c.State(ctx), true
}

// StartPairing kicks off the on-screen pairing flow for a TV-backed monitor.
func (m *tvManager) StartPairing(edid string) error {
	c, err := m.controller(edid)
	if err != nil {
		return err
	}
	return c.StartPairing()
}

// PowerOn wakes a TV via Wake-on-LAN.
func (m *tvManager) PowerOn(edid string) error {
	conn, ok := m.conn(edid)
	if !ok {
		return errors.Errorf("monitor %q is not a configured network TV", edid)
	}
	if conn.Mac == "" {
		return errors.New("no TV MAC configured for Wake-on-LAN")
	}
	return webos.PowerOn(conn.Mac, conn.Host)
}

// PowerOff turns a TV off via SSAP.
func (m *tvManager) PowerOff(ctx context.Context, edid string) error {
	c, err := m.controller(edid)
	if err != nil {
		return err
	}
	return c.withClient(ctx, func(cl *webos.Client) error { return cl.TurnOff(ctx) })
}

// Reachable reports whether a TV's panel is actually on.
func (m *tvManager) Reachable(ctx context.Context, edid string) bool {
	c, err := m.controller(edid)
	if err != nil {
		return false
	}
	return c.Reachable(ctx)
}

// SetVolume sets a TV's absolute volume (0-100).
func (m *tvManager) SetVolume(ctx context.Context, edid string, v int) error {
	c, err := m.controller(edid)
	if err != nil {
		return err
	}
	return c.withClient(ctx, func(cl *webos.Client) error { return cl.SetVolume(ctx, v) })
}

// SetMute sets a TV's mute state.
func (m *tvManager) SetMute(ctx context.Context, edid string, muted bool) error {
	c, err := m.controller(edid)
	if err != nil {
		return err
	}
	return c.withClient(ctx, func(cl *webos.Client) error { return cl.SetMute(ctx, muted) })
}

// SetBacklight sets a TV's OLED backlight (0-100).
func (m *tvManager) SetBacklight(ctx context.Context, edid string, v int) error {
	c, err := m.controller(edid)
	if err != nil {
		return err
	}
	return c.withClient(ctx, func(cl *webos.Client) error { return cl.SetBacklight(ctx, v) })
}

// Backlight reads a TV's current panel backlight (0-100). ok is false if the
// TV is unreachable or the firmware doesn't support the read, so the caller
// can fall back to the last value it set.
func (m *tvManager) Backlight(ctx context.Context, edid string) (int, bool) {
	c, err := m.controller(edid)
	if err != nil {
		return 0, false
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	var v int
	err = c.withClient(ctx, func(cl *webos.Client) error {
		got, e := cl.GetBacklight(ctx)
		v = got
		return e
	})
	return v, err == nil
}

// SwitchInput switches a TV's external input.
func (m *tvManager) SwitchInput(ctx context.Context, edid, input string) error {
	c, err := m.controller(edid)
	if err != nil {
		return err
	}
	return c.withClient(ctx, func(cl *webos.Client) error { return cl.SwitchInput(ctx, input) })
}

// tvController manages the connection to one network-controlled TV (LG webOS).
// The SSAP connection is established lazily and reused; power-on uses
// Wake-on-LAN and needs no connection.
type tvController struct {
	edid    string
	manager *tvManager

	mu         sync.Mutex
	client     *webos.Client
	clientHost string // host the current client is connected to
	key        string
	pairing    bool
	pairErr    string
	dialErr    error     // last failed connect, for a short negative cache
	dialAt     time.Time // when dialErr was recorded
}

// ensure returns a connected client, dialing (and reusing) as needed. It fails
// if the TV isn't paired yet.
func (t *tvController) ensure() (*webos.Client, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	conn, ok := t.manager.conn(t.edid)
	if !ok {
		return nil, errors.New("no TV configured")
	}
	// Drop a stale client if the configured host changed.
	if t.client != nil && t.clientHost != conn.Host {
		t.client.Close()
		t.client = nil
	}
	if t.client != nil {
		return t.client, nil
	}
	if t.key == "" {
		return nil, errors.New("TV is not paired yet — pair it first (POST /api/monitors/pair)")
	}
	// Negative cache: after a recent failed dial, fail fast rather than block
	// this (and every other) caller behind another full dial timeout.
	if t.dialErr != nil && time.Since(t.dialAt) < tvDialBackoff {
		return nil, t.dialErr
	}

	c := webos.New(conn.Host)
	// Bound the dial so an unreachable/off TV can't hang the request (and, via
	// the shared mutex, every other TV/monitor request) until the OS TCP
	// timeout. The client's read loop uses its own background context, so the
	// persistent connection outlives this deadline once established.
	dialCtx, cancel := context.WithTimeout(context.Background(), tvDialTimeout)
	defer cancel()
	newKey, err := c.Connect(dialCtx, t.key)
	if err != nil {
		t.dialErr = errors.Wrap(err, "connect to TV")
		t.dialAt = time.Now()
		return nil, t.dialErr
	}
	t.dialErr = nil
	if newKey != "" && newKey != t.key {
		t.key = newKey
		_ = t.manager.store.SavePairingKey(t.edid, newKey)
	}
	t.client = c
	t.clientHost = conn.Host
	return c, nil
}

// withClient runs fn against a connected client, reconnecting once if the
// connection has gone stale.
func (t *tvController) withClient(ctx context.Context, fn func(*webos.Client) error) error {
	c, err := t.ensure()
	if err != nil {
		return err
	}
	if err := fn(c); err != nil {
		// Drop the (possibly dead) connection and try once more.
		t.mu.Lock()
		if t.client == c {
			t.client.Close()
			t.client = nil
		}
		t.mu.Unlock()

		c2, err2 := t.ensure()
		if err2 != nil {
			return err
		}
		return fn(c2)
	}
	return nil
}

// StartPairing kicks off the on-screen pairing flow in the background (it can
// take up to a minute waiting for the user to accept on the TV). Progress is
// reported via State().
func (t *tvController) StartPairing() error {
	t.mu.Lock()
	conn, ok := t.manager.conn(t.edid)
	if !ok {
		t.mu.Unlock()
		return errors.New("no TV configured")
	}
	if t.pairing {
		t.mu.Unlock()
		return errors.New("pairing already in progress")
	}
	t.pairing = true
	t.pairErr = ""
	host := conn.Host
	key := t.key
	if t.client != nil {
		t.client.Close()
		t.client = nil
	}
	t.mu.Unlock()

	go func() {
		c := webos.New(host)
		newKey, err := c.Connect(context.Background(), key)

		t.mu.Lock()
		defer t.mu.Unlock()
		t.pairing = false
		if err != nil {
			t.pairErr = err.Error()
			log.Printf("TV pairing failed: %v", err)
			return
		}
		t.key = newKey
		t.client = c
		t.clientHost = host
		if err := t.manager.store.SavePairingKey(t.edid, newKey); err != nil {
			log.Printf("Failed to persist TV pairing key: %v", err)
		}
		log.Printf("TV paired successfully")
	}()
	return nil
}

// screenOn maps a webOS power state to "the panel is on". Standby states count
// as off — with Quick Start+ the TV keeps its network stack up and answers
// SSAP while the screen is dark. Unknown states count as on (mirroring Home
// Assistant's mapping): an answering TV in an unrecognised state is more
// likely on than not.
func screenOn(state string) bool {
	switch state {
	case "Suspend", "Active Standby", "Power Off":
		return false
	}
	return true
}

// State returns the TV's live integration state, querying volume and power
// state if connected.
func (t *tvController) State(ctx context.Context) api.MonitorTVState {
	t.mu.Lock()
	st := api.MonitorTVState{Paired: t.key != "", Pairing: t.pairing}
	if t.pairErr != "" {
		st.Error = strPtr(t.pairErr)
	}
	t.mu.Unlock()

	if !st.Paired || st.Pairing {
		return st
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	if err := t.withClient(ctx, func(c *webos.Client) error {
		vol, err := c.GetVolume(ctx)
		if err != nil {
			return err
		}
		st.Volume = vol.Volume
		st.Muted = vol.Muted
		// The volume read answering only proves the network stack is up —
		// Quick Start+ keeps it answering in standby. Ask the power service
		// whether the panel is actually on; firmwares without the endpoint
		// fall back to answered == on.
		st.On = true
		if state, perr := c.GetPowerState(ctx); perr == nil {
			st.On = screenOn(state)
		}
		return nil
	}); err != nil {
		st.Error = strPtr(err.Error())
	}
	return st
}

// Reachable reports whether the TV's panel is actually on. It asks the power
// service for the real state (a Quick Start+ TV answers SSAP in standby, so
// merely connecting proves nothing); firmwares without that endpoint fall back
// to "answers a lightweight request == on".
func (t *tvController) Reachable(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	on := false
	err := t.withClient(ctx, func(c *webos.Client) error {
		state, perr := c.GetPowerState(ctx)
		if perr != nil {
			if _, verr := c.GetVolume(ctx); verr != nil {
				return verr
			}
			on = true
			return nil
		}
		on = screenOn(state)
		return nil
	})
	return err == nil && on
}

// --- API handlers ---

// PairMonitor implements api.StrictServerInterface. It starts on-screen
// pairing for a TV-backed monitor.
func (a *Agent) PairMonitor(ctx context.Context, request api.PairMonitorRequestObject) (api.PairMonitorResponseObject, error) {
	if request.Body == nil || request.Body.Edid == "" {
		return api.PairMonitor400JSONResponse{Code: 400, Error: "edid is required"}, nil
	}
	if err := a.tv.StartPairing(request.Body.Edid); err != nil {
		return api.PairMonitor500JSONResponse{Code: 500, Error: err.Error()}, nil
	}
	msg := "Pairing started — accept the prompt on the TV"
	return api.PairMonitor200JSONResponse{Success: true, Message: &msg}, nil
}

// SetMonitorVolume implements api.StrictServerInterface. Volume control is
// only available on TV-backed monitors.
func (a *Agent) SetMonitorVolume(ctx context.Context, request api.SetMonitorVolumeRequestObject) (api.SetMonitorVolumeResponseObject, error) {
	if request.Body == nil || request.Body.Edid == "" {
		return api.SetMonitorVolume400JSONResponse{Code: 400, Error: "edid is required"}, nil
	}
	edid := request.Body.Edid
	if request.Body.Volume != nil {
		if err := a.tv.SetVolume(ctx, edid, *request.Body.Volume); err != nil {
			return api.SetMonitorVolume500JSONResponse{Code: 500, Error: err.Error()}, nil
		}
	}
	if request.Body.Muted != nil {
		if err := a.tv.SetMute(ctx, edid, *request.Body.Muted); err != nil {
			return api.SetMonitorVolume500JSONResponse{Code: 500, Error: err.Error()}, nil
		}
	}
	msg := "volume updated"
	return api.SetMonitorVolume200JSONResponse{Success: true, Message: &msg}, nil
}

// SetMonitorInput implements api.StrictServerInterface. It switches a
// TV-backed monitor's external input.
func (a *Agent) SetMonitorInput(ctx context.Context, request api.SetMonitorInputRequestObject) (api.SetMonitorInputResponseObject, error) {
	if request.Body == nil || request.Body.Edid == "" || request.Body.Input == "" {
		return api.SetMonitorInput400JSONResponse{Code: 400, Error: "edid and input are required"}, nil
	}
	if err := a.tv.SwitchInput(ctx, request.Body.Edid, request.Body.Input); err != nil {
		return api.SetMonitorInput500JSONResponse{Code: 500, Error: err.Error()}, nil
	}
	msg := "input switched"
	return api.SetMonitorInput200JSONResponse{Success: true, Message: &msg}, nil
}
