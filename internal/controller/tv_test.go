package controller

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/trolleyman/ottoman/internal/api"
	"github.com/trolleyman/ottoman/internal/store"
)

func TestMirroredTVCommandsBypassAgent(t *testing.T) {
	var requests atomic.Int32
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer agent.Close()
	c := newTestController(t, agent.URL)
	if err := c.registry.ReplaceTVEntries([]store.MonitorEntry{{Edid: "tv", TV: &store.TVConn{Host: "127.0.0.1"}}}); err != nil {
		t.Fatal(err)
	}
	// An unpaired TV must report its own failure, even with an answering desktop.
	power, err := c.SetMonitorPower(t.Context(), api.SetMonitorPowerRequestObject{Body: &api.SetMonitorPowerJSONRequestBody{Edid: "tv", On: false}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := power.(api.SetMonitorPower500JSONResponse); !ok {
		t.Fatalf("power = %T, want local pairing error", power)
	}
	volume := 30
	audio, err := c.SetMonitorVolume(t.Context(), api.SetMonitorVolumeRequestObject{Body: &api.SetMonitorVolumeJSONRequestBody{Edid: "tv", Volume: &volume}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := audio.(api.SetMonitorVolume500JSONResponse); !ok {
		t.Fatalf("volume = %T, want local pairing error", audio)
	}
	brightness, err := c.SetMonitorBrightness(t.Context(), api.SetMonitorBrightnessRequestObject{Body: &api.SetMonitorBrightnessJSONRequestBody{Edid: "tv", Brightness: 40}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := brightness.(api.SetMonitorBrightness500JSONResponse); !ok {
		t.Fatalf("brightness = %T, want local pairing error", brightness)
	}
	state, err := c.GetMonitorPowerState(t.Context(), api.GetMonitorPowerStateRequestObject{Body: &api.GetMonitorPowerStateJSONRequestBody{Edid: "tv"}})
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := state.(api.GetMonitorPowerState200JSONResponse); !ok || got.Responding {
		t.Fatalf("power state = %#v, want not responding", state)
	}
	if requests.Load() != 0 {
		t.Fatalf("TV commands contacted agent %d times", requests.Load())
	}
	_, err = c.SetMonitorPower(t.Context(), api.SetMonitorPowerRequestObject{Body: &api.SetMonitorPowerJSONRequestBody{Edid: "desktop", On: true}})
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 {
		t.Fatal("desktop monitor did not use agent")
	}
}

func TestTVControlsSurviveControllerRestartWithDesktopOffline(t *testing.T) {
	c := newTestController(t, "http://127.0.0.1:1")
	if err := c.registry.ReplaceTVEntries([]store.MonitorEntry{{Edid: "tv", FriendlyName: "Living room", TV: &store.TVConn{Host: "127.0.0.1"}}}); err != nil {
		t.Fatal(err)
	}
	if err := c.tvStore.SavePairingKey("tv", "saved-key"); err != nil {
		t.Fatal(err)
	}
	restarted, err := New(c.config)
	if err != nil {
		t.Fatal(err)
	}
	response, err := restarted.GetMonitors(t.Context(), api.GetMonitorsRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	monitors, ok := response.(api.GetMonitors200JSONResponse)
	if !ok || len(monitors) != 1 {
		t.Fatalf("monitors = %#v", response)
	}
	if monitors[0].Name != "Living room" || monitors[0].TvState == nil || !monitors[0].TvState.Paired {
		t.Fatalf("lost mirrored TV: %#v", monitors[0])
	}
}
