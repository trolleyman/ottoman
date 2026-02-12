package common

// TrackpadMessage is the WebSocket message format for trackpad communication.
// Message types:
//   - "s" (start): begin drag, Touch indicates touch vs mouse mode
//   - "m" (move): movement delta
//   - "e" (end): end drag, triggers inertia if touch mode
//   - "p" (position): cursor position update (server -> browser)
//   - "c" (click): mouse click
//   - "d" (down): mouse down
//   - "u" (up): mouse up
//   - "k" (keyboard): keyboard input
type TrackpadMessage struct {
	Type  string  `json:"t"`
	Touch *bool   `json:"touch,omitempty"`
	DX    float64 `json:"dx,omitempty"`
	DY    float64 `json:"dy,omitempty"`
	X     int     `json:"x"`
	Y     int     `json:"y"`
	Text  string  `json:"text,omitempty"`
}
