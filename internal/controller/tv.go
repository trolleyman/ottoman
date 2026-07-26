package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/pkg/errors"
	"github.com/trolleyman/ottoman/internal/api"
	"github.com/trolleyman/ottoman/internal/store"
	"github.com/trolleyman/ottoman/internal/tv"
)

// tvSyncInterval bounds how often the controller re-mirrors the agent's TV
// registry + pairing keys. It's frequent enough that the mirror is fresh by the
// time the desktop is turned off (the moment local control is needed), but the
// data changes rarely so there's no need to poll hard.
const tvSyncInterval = 30 * time.Second

// startTVSync mirrors the agent's TV registry + pairing keys into the local
// store, once immediately and then on a ticker, so the controller can drive the
// TV directly the moment the agent (desktop) goes down. Best-effort: a failed
// sync (agent down, etc.) is logged at most once per transition and retried on
// the next tick, leaving the last-known mirror in place.
func (c *Controller) startTVSync(ctx context.Context) {
	if c.registry == nil {
		return // TV mirror unavailable (see New)
	}
	go func() {
		lastErr := true // force a one-time "sync ok" log on first success
		sync := func() {
			if err := c.syncTVFromAgent(ctx); err != nil {
				if !lastErr {
					log.Printf("TV sync from agent failed (using last-known mirror): %v", err)
				}
				lastErr = true
				return
			}
			if lastErr {
				log.Printf("TV sync from agent OK")
			}
			lastErr = false
		}
		sync()
		ticker := time.NewTicker(tvSyncInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sync()
			}
		}
	}()
}

// syncTVFromAgent fetches the agent's TV export and writes it into the local
// registry + pairing-key store.
func (c *Controller) syncTVFromAgent(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	url := fmt.Sprintf("http://%s/api/tv/export", c.getAgentAddr())
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	if c.config.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.AuthToken)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("agent returned status %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return errors.Wrap(err, "read TV export")
	}
	// Skip the store writes when nothing changed — this runs every 30s and the
	// controller often lives on an SD card, so needless rewrites are real wear.
	if bytes.Equal(raw, c.lastTVExport) {
		return nil
	}

	var exp tv.Export
	if err := json.Unmarshal(raw, &exp); err != nil {
		return errors.Wrap(err, "decode TV export")
	}

	entries := make([]store.MonitorEntry, 0, len(exp.TVs))
	for _, t := range exp.TVs {
		entries = append(entries, store.MonitorEntry{
			Edid:         t.Edid,
			FriendlyName: t.FriendlyName,
			Backend:      store.BackendTV,
			TV:           t.TV,
		})
	}
	if err := c.registry.ReplaceTVEntries(entries); err != nil {
		return errors.Wrap(err, "save mirrored TV registry")
	}
	for _, t := range exp.TVs {
		if t.PairingKey == "" {
			continue
		}
		if err := c.tvStore.SavePairingKey(t.Edid, t.PairingKey); err != nil {
			log.Printf("Failed to save mirrored pairing key for %s: %v", t.Edid, err)
		}
	}
	c.lastTVExport = raw
	return nil
}

// localTVEntry returns the mirrored registry entry for an EDID if it's a known
// TV the controller can drive locally, and whether it exists. Used by the
// monitor endpoints to decide whether to fall back to local control when the
// agent is unreachable.
func (c *Controller) localTVEntry(edid string) (store.MonitorEntry, bool) {
	if c.registry == nil || c.tv == nil || edid == "" {
		return store.MonitorEntry{}, false
	}
	e, ok := c.registry.Get(edid)
	if !ok || e.Backend != store.BackendTV || e.TV == nil || e.TV.Host == "" {
		return store.MonitorEntry{}, false
	}
	return e, true
}

// The following local* helpers drive a mirrored TV directly (WoL / SSAP) and
// are only reached from the monitor endpoints' agent-down fallback path. Each
// returns a user-facing message on success. Callers must have already confirmed
// the EDID is a known TV via localTVEntry.

// localSetPower turns a TV on via Wake-on-LAN or off via SSAP.
func (c *Controller) localSetPower(ctx context.Context, edid string, on bool) (string, error) {
	if on {
		if err := c.tv.PowerOn(edid); err != nil {
			return "", err
		}
		return "TV powering on (agent offline)", nil
	}
	if err := c.tv.PowerOff(ctx, edid); err != nil {
		return "", err
	}
	return "TV powering off (agent offline)", nil
}

// localSetVolume sets a TV's volume and/or mute over SSAP.
func (c *Controller) localSetVolume(ctx context.Context, edid string, volume *int, muted *bool) (string, error) {
	if volume != nil {
		if err := c.tv.SetVolume(ctx, edid, *volume); err != nil {
			return "", err
		}
	}
	if muted != nil {
		if err := c.tv.SetMute(ctx, edid, *muted); err != nil {
			return "", err
		}
	}
	return "volume updated (agent offline)", nil
}

// localSetBacklight sets a TV's OLED backlight over SSAP. The agent routes a
// brightness request for a TV-backed monitor to the backlight, so the offline
// fallback does the same.
func (c *Controller) localSetBacklight(ctx context.Context, edid string, brightness int) (string, error) {
	if err := c.tv.SetBacklight(ctx, edid, brightness); err != nil {
		return "", err
	}
	return "backlight updated (agent offline)", nil
}

// localTVMonitors synthesises the /api/monitors response from the mirrored TV
// registry, with each TV's live state, so the web UI still shows and controls
// the TVs when the agent is down. Returns nil if the mirror is unavailable.
func (c *Controller) localTVMonitors(ctx context.Context) []api.Monitor {
	if c.registry == nil || c.tv == nil {
		return nil
	}
	entries := c.registry.TVEntries()
	monitors := make([]api.Monitor, 0, len(entries))
	for _, e := range entries {
		name := e.FriendlyName
		if name == "" {
			name = "TV"
		}
		backend := store.BackendTV
		caps := &api.MonitorCapabilities{Brightness: true, Power: true, Volume: true}
		mon := api.Monitor{
			Edid:           e.Edid,
			Name:           name,
			ControlBackend: &backend,
			Capabilities:   caps,
		}
		if e.FriendlyName != "" {
			mon.FriendlyName = &e.FriendlyName
		}
		if e.TV != nil {
			mon.Tv = &api.TVConn{Type: &e.TV.Type, Host: &e.TV.Host, Mac: &e.TV.Mac}
		}
		if st, ok := c.tv.StateFor(ctx, e.Edid); ok {
			mon.TvState = &st
		}
		monitors = append(monitors, mon)
	}
	return monitors
}
