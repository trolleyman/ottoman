package common

// Layout represents a display layout configuration
type Layout struct {
	ID       string    `json:"id" mapstructure:"id"`                     // Required: user-defined or slug of name
	Name     string    `json:"name" mapstructure:"name"`                 // Required: display name
	Emoji    string    `json:"emoji,omitempty" mapstructure:"emoji"`     // Optional: emoji for UI
	Aliases  []string  `json:"aliases,omitempty" mapstructure:"aliases"` // Optional: alternative names/shortcuts
	Monitors []Monitor `json:"monitors" mapstructure:"monitors"`         // Monitor configurations
}

// Monitor represents a monitor configuration within a layout
type Monitor struct {
	// Identification by EDID (manufacturer:product code)
	// Format: "MANUFACTURER:PRODUCT" e.g., "DEL:D0A2", "SAM:0C4E", "AOC:B403"
	EDID string `json:"edid" mapstructure:"edid"`

	// Port is the output name (e.g., "HDMI-1", "DP-1" on Linux)
	// Used by xrandr on Linux for display configuration
	Port string `json:"port,omitempty" mapstructure:"port"`

	// Name is the human-readable name of the monitor
	Name string `json:"name,omitempty" mapstructure:"name"`

	// Display configuration
	Width       int     `json:"width" mapstructure:"width"`
	Height      int     `json:"height" mapstructure:"height"`
	RefreshRate float64 `json:"refresh_rate" mapstructure:"refresh_rate"`
	PositionX   int     `json:"position_x" mapstructure:"position_x"`
	PositionY   int     `json:"position_y" mapstructure:"position_y"`
	Primary     bool    `json:"primary,omitempty" mapstructure:"primary"`
	Enabled     bool    `json:"enabled" mapstructure:"enabled"`
}

// LayoutsConfig holds all available display layouts (for file format)
type LayoutsConfig struct {
	Layouts []Layout `json:"layouts"`
}
