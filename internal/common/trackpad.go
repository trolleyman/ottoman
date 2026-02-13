package common

// TrackpadMessage is the WebSocket message format for trackpad communication.
// Message types:
//   - "m" (move): movement delta
//   - "p" (position): cursor position update (server -> browser)
//   - "c" (click): mouse click (Button specifies which: "left","right","middle","back","forward")
//   - "d" (down): mouse button down (Button specifies which)
//   - "u" (up): mouse button up (Button specifies which)
//   - "k" (keyboard): keyboard text input
//   - "sc" (scroll): scroll by DX (horizontal) and DY (vertical), Precise indicates pixel vs line mode
//   - "key" (keypress): special key press with Key name and Modifiers
//   - "a" (absolute): move cursor to absolute position
type TrackpadMessage struct {
	Type      string   `json:"t"`
	Touch     *bool    `json:"touch,omitempty"`
	DX        float64  `json:"dx,omitempty"`
	DY        float64  `json:"dy,omitempty"`
	X         int      `json:"x"`
	Y         int      `json:"y"`
	Text      string   `json:"text,omitempty"`
	Button    string   `json:"btn,omitempty"`     // "left" (default), "right", "middle", "back", "forward"
	Key       string   `json:"key,omitempty"`     // browser event.key name for "key" messages
	Modifiers []string `json:"mod,omitempty"`     // modifier keys: "shift", "ctrl", "alt", "meta"
	Precise   *bool    `json:"precise,omitempty"` // scroll precision: true = pixel (trackpads), false/nil = line (mouse wheels)
}
