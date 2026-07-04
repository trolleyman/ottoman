package agent

import (
	"context"
	"log"
	"sync"

	"github.com/pkg/errors"
	"github.com/trolleyman/ottoman/internal/api"
	"github.com/trolleyman/ottoman/internal/config"
	"github.com/trolleyman/ottoman/internal/store"
	"github.com/trolleyman/ottoman/internal/tv/webos"
)

// tvController manages the connection to a network-controlled TV (LG webOS).
// The SSAP connection is established lazily and reused; power-on uses
// Wake-on-LAN and needs no connection.
//
// The TV's transport (host/mac/type) is resolved from the monitor registry
// entry whose backend is "tv" — so a TV's config lives alongside the rest of
// that monitor's settings. A legacy top-level [agent.tv] config, if present, is
// used as a fallback so existing installs keep working until the TV is
// configured on its monitor.
type tvController struct {
	reg    *store.Registry
	legacy config.TVConfig
	store  *store.TVStore

	mu         sync.Mutex
	client     *webos.Client
	clientHost string // host the current client is connected to
	key        string
	pairing    bool
	pairErr    string
}

func newTVController(reg *store.Registry, legacy config.TVConfig, st *store.TVStore) *tvController {
	key, _ := st.LoadPairingKey()
	return &tvController{reg: reg, legacy: legacy, store: st, key: key}
}

// conn resolves the current TV transport: the registry entry first, then the
// legacy [agent.tv] config as a migration fallback.
func (t *tvController) conn() (store.TVConn, bool) {
	if e, ok := t.reg.TVEntry(); ok {
		return *e.TV, true
	}
	if t.legacy.Host != "" {
		return store.TVConn{Type: t.legacy.Type, Host: t.legacy.Host, Mac: t.legacy.Mac}, true
	}
	return store.TVConn{}, false
}

// Configured reports whether a network TV is configured (on a monitor or via
// the legacy config).
func (t *tvController) Configured() bool {
	_, ok := t.conn()
	return ok
}

// ensure returns a connected client, dialing (and reusing) as needed. It fails
// if the TV isn't paired yet.
func (t *tvController) ensure(ctx context.Context) (*webos.Client, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	conn, ok := t.conn()
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
		return nil, errors.New("TV is not paired yet — pair it first (POST /api/tv/pair)")
	}

	c := webos.New(conn.Host)
	// Use a background context so the persistent read loop outlives the request.
	newKey, err := c.Connect(context.Background(), t.key)
	if err != nil {
		return nil, err
	}
	if newKey != "" && newKey != t.key {
		t.key = newKey
		_ = t.store.SavePairingKey(newKey)
	}
	t.client = c
	t.clientHost = conn.Host
	return c, nil
}

// withClient runs fn against a connected client, reconnecting once if the
// connection has gone stale.
func (t *tvController) withClient(ctx context.Context, fn func(*webos.Client) error) error {
	c, err := t.ensure(ctx)
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

		c2, err2 := t.ensure(ctx)
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
	conn, ok := t.conn()
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
		if err := t.store.SavePairingKey(newKey); err != nil {
			log.Printf("Failed to persist TV pairing key: %v", err)
		}
		log.Printf("TV paired successfully")
	}()
	return nil
}

// TVState is the reported state of the TV integration.
type TVState struct {
	Configured bool   `json:"configured"`
	Paired     bool   `json:"paired"`
	Pairing    bool   `json:"pairing"`
	Host       string `json:"host"`
	Volume     int    `json:"volume"`
	Muted      bool   `json:"muted"`
	Error      string `json:"error,omitempty"`
}

// State returns the current TV integration state, querying live volume if
// connected.
func (t *tvController) State(ctx context.Context) TVState {
	conn, configured := t.conn()
	t.mu.Lock()
	st := TVState{
		Configured: configured,
		Paired:     t.key != "",
		Pairing:    t.pairing,
		Host:       conn.Host,
		Error:      t.pairErr,
	}
	t.mu.Unlock()

	if st.Configured && st.Paired && !st.Pairing {
		if err := t.withClient(ctx, func(c *webos.Client) error {
			vol, err := c.GetVolume(ctx)
			if err != nil {
				return err
			}
			st.Volume = vol.Volume
			st.Muted = vol.Muted
			return nil
		}); err != nil {
			st.Error = err.Error()
		}
	}
	return st
}

// PowerOn wakes the TV via Wake-on-LAN.
func (t *tvController) PowerOn() error {
	conn, ok := t.conn()
	if !ok || conn.Mac == "" {
		return errors.New("no TV MAC configured for Wake-on-LAN")
	}
	return webos.PowerOn(conn.Mac, conn.Host)
}

// PowerOff turns the TV off via SSAP.
func (t *tvController) PowerOff(ctx context.Context) error {
	return t.withClient(ctx, func(c *webos.Client) error { return c.TurnOff(ctx) })
}

// SetVolume sets the TV's absolute volume (0-100).
func (t *tvController) SetVolume(ctx context.Context, v int) error {
	return t.withClient(ctx, func(c *webos.Client) error { return c.SetVolume(ctx, v) })
}

// SetMute sets the TV mute state.
func (t *tvController) SetMute(ctx context.Context, muted bool) error {
	return t.withClient(ctx, func(c *webos.Client) error { return c.SetMute(ctx, muted) })
}

// SetBacklight sets the OLED backlight (0-100).
func (t *tvController) SetBacklight(ctx context.Context, v int) error {
	return t.withClient(ctx, func(c *webos.Client) error { return c.SetBacklight(ctx, v) })
}

// SwitchInput switches the TV's external input.
func (t *tvController) SwitchInput(ctx context.Context, input string) error {
	return t.withClient(ctx, func(c *webos.Client) error { return c.SwitchInput(ctx, input) })
}

// --- API handlers ---

// GetTVState implements api.StrictServerInterface.
func (a *Agent) GetTVState(ctx context.Context, request api.GetTVStateRequestObject) (api.GetTVStateResponseObject, error) {
	st := a.tv.State(ctx)
	resp := api.GetTVState200JSONResponse{
		Configured: st.Configured,
		Paired:     st.Paired,
		Pairing:    st.Pairing,
		Host:       st.Host,
		Volume:     st.Volume,
		Muted:      st.Muted,
	}
	if st.Error != "" {
		resp.Error = &st.Error
	}
	return resp, nil
}

// PairTV implements api.StrictServerInterface.
func (a *Agent) PairTV(ctx context.Context, request api.PairTVRequestObject) (api.PairTVResponseObject, error) {
	if err := a.tv.StartPairing(); err != nil {
		return api.PairTV500JSONResponse{Code: 500, Error: err.Error()}, nil
	}
	msg := "Pairing started — accept the prompt on the TV"
	return api.PairTV200JSONResponse{Success: true, Message: &msg}, nil
}

// SetTVPower implements api.StrictServerInterface.
func (a *Agent) SetTVPower(ctx context.Context, request api.SetTVPowerRequestObject) (api.SetTVPowerResponseObject, error) {
	if request.Body == nil {
		return api.SetTVPower400JSONResponse{Code: 400, Error: "body required"}, nil
	}
	var err error
	if request.Body.On {
		err = a.tv.PowerOn()
	} else {
		err = a.tv.PowerOff(ctx)
	}
	if err != nil {
		return api.SetTVPower500JSONResponse{Code: 500, Error: err.Error()}, nil
	}
	msg := "TV power updated"
	return api.SetTVPower200JSONResponse{Success: true, Message: &msg}, nil
}

// SetTVVolume implements api.StrictServerInterface.
func (a *Agent) SetTVVolume(ctx context.Context, request api.SetTVVolumeRequestObject) (api.SetTVVolumeResponseObject, error) {
	if request.Body == nil {
		return api.SetTVVolume400JSONResponse{Code: 400, Error: "body required"}, nil
	}
	if request.Body.Volume != nil {
		if err := a.tv.SetVolume(ctx, *request.Body.Volume); err != nil {
			return api.SetTVVolume500JSONResponse{Code: 500, Error: err.Error()}, nil
		}
	}
	if request.Body.Muted != nil {
		if err := a.tv.SetMute(ctx, *request.Body.Muted); err != nil {
			return api.SetTVVolume500JSONResponse{Code: 500, Error: err.Error()}, nil
		}
	}
	msg := "TV volume updated"
	return api.SetTVVolume200JSONResponse{Success: true, Message: &msg}, nil
}

// SetTVInput implements api.StrictServerInterface.
func (a *Agent) SetTVInput(ctx context.Context, request api.SetTVInputRequestObject) (api.SetTVInputResponseObject, error) {
	if request.Body == nil || request.Body.Input == "" {
		return api.SetTVInput400JSONResponse{Code: 400, Error: "input is required"}, nil
	}
	if err := a.tv.SwitchInput(ctx, request.Body.Input); err != nil {
		return api.SetTVInput500JSONResponse{Code: 500, Error: err.Error()}, nil
	}
	msg := "TV input switched"
	return api.SetTVInput200JSONResponse{Success: true, Message: &msg}, nil
}
