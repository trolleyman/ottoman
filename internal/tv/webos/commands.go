package webos

import (
	"context"
	"encoding/json"
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

// SetBacklight sets the OLED panel backlight (0-100) via the picture Luna
// settings — the control that actually governs OLED panel brightness, distinct
// from the "brightness" picture control.
//
// NOTE: this drives a luna:// system-settings call over SSAP, which is the
// least portable command here; it's proven on webOS 6 (Home Assistant's
// webostv / bscpylgtv) but may need adjustment on other firmwares.
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
	_, err := c.request(ctx, "luna://com.webos.settingsservice/setSystemSettings", payload)
	return err
}
