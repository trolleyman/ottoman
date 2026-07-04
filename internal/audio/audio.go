// Package audio provides control over the desktop's audio sinks (output
// devices). On Linux this wraps PipeWire's wpctl; other platforms are not yet
// supported.
package audio

// Sink is one audio output device.
type Sink struct {
	// ID is the current PipeWire node id. It is NOT stable across reboots, so
	// it must not be persisted — match on Name instead.
	ID uint32 `json:"id"`
	// Name is the stable node.name (e.g. alsa_output.pci-...hdmi-stereo).
	Name string `json:"name"`
	// Description is the human-friendly name (e.g. "HDA NVidia Digital Stereo").
	Description string `json:"description"`
	// Volume is 0.0..1.0+ (PipeWire allows >1.0 boost).
	Volume float64 `json:"volume"`
	// Muted reports whether the sink is muted.
	Muted bool `json:"muted"`
	// Default reports whether this is the default output sink.
	Default bool `json:"default"`
}

// Controller controls audio sinks. All mutating operations take a stable sink
// Name (node.name) rather than an id.
type Controller interface {
	// ListSinks returns all output sinks.
	ListSinks() ([]Sink, error)
	// SetVolume sets a sink's volume (0.0..1.5).
	SetVolume(name string, volume float64) error
	// SetMute sets a sink's mute state.
	SetMute(name string, muted bool) error
	// SetDefault makes a sink the default output.
	SetDefault(name string) error
}

// NewController returns the platform-specific audio controller, or an error if
// audio control is unavailable on this platform/host.
func NewController() (Controller, error) {
	return newPlatformController()
}
