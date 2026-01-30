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
	// Identification by EDID (manufacturer:product code)
	// Format: "MANUFACTURER:PRODUCT" e.g., "DEL:D0A2", "SAM:0C4E", "AOC:B403"
	EDID string `json:"edid"`

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
