package webos

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// VolumeState is the TV's current audio state.
type VolumeState struct {
	Volume int  `json:"volume"`
	Muted  bool `json:"muted"`
}

// GetVolume returns the TV's current volume and mute state.
func (c *Client) GetVolume(ctx context.Context) (VolumeState, error) {
	raw, err := c.request(ctx, "ssap://audio/getVolume", nil)
	if err != nil {
		return VolumeState{}, err
	}
	return parseVolume(raw)
}

// parseVolume handles the two payload shapes webOS versions use: flat
// volume/muted, or a nested volumeStatus object.
func parseVolume(raw json.RawMessage) (VolumeState, error) {
	var flat struct {
		Volume       *int  `json:"volume"`
		Muted        *bool `json:"muted"`
		VolumeStatus *struct {
			Volume     int  `json:"volume"`
			MuteStatus bool `json:"muteStatus"`
		} `json:"volumeStatus"`
	}
	if err := json.Unmarshal(raw, &flat); err != nil {
		return VolumeState{}, err
	}
	st := VolumeState{}
	if flat.VolumeStatus != nil {
		st.Volume = flat.VolumeStatus.Volume
		st.Muted = flat.VolumeStatus.MuteStatus
	}
	if flat.Volume != nil {
		st.Volume = *flat.Volume
	}
	if flat.Muted != nil {
		st.Muted = *flat.Muted
	}
	return st, nil
}

// SetVolume sets the absolute volume (0-100).
func (c *Client) SetVolume(ctx context.Context, volume int) error {
	if volume < 0 {
		volume = 0
	}
	if volume > 100 {
		volume = 100
	}
	_, err := c.request(ctx, "ssap://audio/setVolume", map[string]int{"volume": volume})
	return err
}

// SetMute sets the mute state.
func (c *Client) SetMute(ctx context.Context, muted bool) error {
	_, err := c.request(ctx, "ssap://audio/setMute", map[string]bool{"mute": muted})
	return err
}

// TurnOff powers the TV off (it can be woken again via Wake-on-LAN).
func (c *Client) TurnOff(ctx context.Context) error {
	_, err := c.request(ctx, "ssap://system/turnOff", nil)
	return err
}

// Input is an external input source.
type Input struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// ListInputs returns the TV's external inputs.
func (c *Client) ListInputs(ctx context.Context) ([]Input, error) {
	raw, err := c.request(ctx, "ssap://tv/getExternalInputList", nil)
	if err != nil {
		return nil, err
	}
	var p struct {
		Devices []Input `json:"devices"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	return p.Devices, nil
}

// SwitchInput switches to the given external input id.
func (c *Client) SwitchInput(ctx context.Context, inputID string) error {
	_, err := c.request(ctx, "ssap://tv/switchInput", map[string]string{"inputId": inputID})
	return err
}

// lunaRequest invokes an internal luna:// service method. webOS does not
// expose luna services over the SSAP websocket (requesting one directly gets
// "404 no such service or method"), so this uses the established workaround
// (as in bscpylgtv / Home Assistant): create a blank system alert whose
// button and close handlers point at the luna call, then immediately close
// it — closing fires the handler, which executes the call with system
// privileges. Fire-and-forget: the luna call's own result is not observable,
// only the createAlert/closeAlert round-trips.
func (c *Client) lunaRequest(ctx context.Context, uri string, params any) error {
	call := map[string]any{"uri": uri, "params": params}
	payload := map[string]any{
		"message": " ",
		"buttons": []map[string]any{{"label": "", "onClick": uri, "params": params}},
		"onclose": call,
		"onfail":  call,
	}
	raw, err := c.request(ctx, "ssap://system.notifications/createAlert", payload)
	if err != nil {
		return err
	}
	var p struct {
		AlertID string `json:"alertId"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	if p.AlertID == "" {
		return errors.Errorf("createAlert for %s returned no alertId", uri)
	}
	_, err = c.request(ctx, "ssap://system.notifications/closeAlert", map[string]string{"alertId": p.AlertID})
	return err
}

// SetBacklight sets the OLED panel backlight (0-100) via the picture Luna
// settings — the control that actually governs OLED panel brightness, distinct
// from the "brightness" picture control. Writes have no SSAP endpoint, so this
// goes through the lunaRequest alert workaround.
func (c *Client) SetBacklight(ctx context.Context, value int) error {
	if value < 0 {
		value = 0
	}
	if value > 100 {
		value = 100
	}
	payload := map[string]any{
		"category": "picture",
		"settings": map[string]any{"backlight": value},
	}
	return c.lunaRequest(ctx, "luna://com.webos.settingsservice/setSystemSettings", payload)
}

// GetBacklight reads the OLED panel backlight (0-100) — the read mirror of
// SetBacklight. Unlike writes, reads have a real SSAP endpoint
// (ssap://settings/getSystemSettings, gated by the manifest's READ_SETTINGS).
// Some firmwares don't expose the picture category, so callers should treat an
// error as "unknown" and fall back to the last value they set.
func (c *Client) GetBacklight(ctx context.Context) (int, error) {
	payload := map[string]any{
		"category": "picture",
		"keys":     []string{"backlight"},
	}
	raw, err := c.request(ctx, "ssap://settings/getSystemSettings", payload)
	if err != nil {
		return 0, err
	}
	var p struct {
		Settings struct {
			Backlight json.RawMessage `json:"backlight"`
		} `json:"settings"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return 0, err
	}
	return parseBacklight(p.Settings.Backlight)
}

// parseBacklight handles webOS returning the value as either a JSON number (80)
// or a string ("80").
func parseBacklight(raw json.RawMessage) (int, error) {
	if len(raw) == 0 {
		return 0, errors.New("no backlight in picture settings")
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		return n, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		v, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			return 0, errors.Wrapf(err, "backlight %q not an integer", s)
		}
		return v, nil
	}
	return 0, errors.Errorf("unrecognized backlight value %s", raw)
}
