package common

// Layout represents a display layout configuration
type Layout struct {
	ID       string    `json:"id"`                // Required: user-defined or slug of name
	Name     string    `json:"name"`              // Required: display name
	Emoji    string    `json:"emoji,omitempty"`   // Optional: emoji for UI
	Aliases  []string  `json:"aliases,omitempty"` // Optional: alternative names/shortcuts
	Monitors []Monitor `json:"monitors"`          // Monitor configurations
}

// Monitor represents a monitor configuration within a layout
type Monitor struct {
	// Identification - EDID preferred, Port as fallback
	// EDID format: "MANUFACTURER:PRODUCT" e.g., "DEL:D0A2", "SAM:0C4E"
	// Port format: connector name e.g., "HDMI-1", "DP-1", "eDP-1"
	EDID string `json:"edid,omitempty"` // EDID manufacturer:product (preferred, portable)
	Port string `json:"port,omitempty"` // Fallback: port/connector name

	// Display configuration
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	RefreshRate float64 `json:"refresh_rate"`
	PositionX   int     `json:"position_x"`
	PositionY   int     `json:"position_y"`
	Primary     bool    `json:"primary,omitempty"`
	Enabled     bool    `json:"enabled"`
}

// LayoutsConfig holds all available display layouts (for file format)
type LayoutsConfig struct {
	Layouts []Layout `json:"layouts"`
}
