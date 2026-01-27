package common

// DisplayLayout represents a complete display configuration that can be applied
type DisplayLayout struct {
	Name        string       `json:"name"`
	SourceModes []SourceMode `json:"source_modes"`
	TargetModes []TargetMode `json:"target_modes"`
	Paths       []Path       `json:"paths"`
}

// SourceMode describes a display source configuration
type SourceMode struct {
	ID          string   `json:"id"`
	Adapter     string   `json:"adapter,omitempty"`
	GDIName     string   `json:"gdi_name,omitempty"`
	Width       int      `json:"width"`
	Height      int      `json:"height"`
	PixelFormat string   `json:"pixel_format,omitempty"`
	Position    Position `json:"position"`
}

// Position represents screen position
type Position struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// TargetMode describes a display target configuration
type TargetMode struct {
	ID               string      `json:"id"`
	OutputTechnology string      `json:"output_technology,omitempty"`
	EDIDManufacturer string      `json:"edid_manufacturer,omitempty"`
	EDIDProductCode  string      `json:"edid_product_code,omitempty"`
	ConnectorIndex   int         `json:"connector_index,omitempty"`
	MonitorDevice    string      `json:"monitor_device,omitempty"`
	PixelRate        int64       `json:"pixel_rate,omitempty"`
	HSyncFreq        float64     `json:"hsync_freq,omitempty"`
	VSyncFreq        float64     `json:"vsync_freq,omitempty"`
	ActiveSize       DisplaySize `json:"active_size"`
	TotalSize        DisplaySize `json:"total_size,omitempty"`
	VideoStandard    string      `json:"video_standard,omitempty"`
	ScanlineOrdering string      `json:"scanline_ordering,omitempty"`
}

// DisplaySize represents display dimensions
type DisplaySize struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Path connects a source to a target
type Path struct {
	SourceIndex    int        `json:"source_index"`
	TargetIndex    int        `json:"target_index"`
	OutputTech     string     `json:"output_tech,omitempty"`
	Rotation       int        `json:"rotation,omitempty"`
	Scaling        string     `json:"scaling,omitempty"`
	RefreshRate    float64    `json:"refresh_rate"`
	ScanlineOrder  string     `json:"scanline_order,omitempty"`
	IsPrimary      bool       `json:"is_primary,omitempty"`
}

// LayoutsConfig holds all available display layouts
type LayoutsConfig struct {
	Layouts []DisplayLayout `json:"layouts"`
}

// SimplifiedLayout is a user-friendly layout format for configuration
type SimplifiedLayout struct {
	Name     string           `json:"name"`
	Monitors []MonitorConfig  `json:"monitors"`
}

// MonitorConfig is a simplified monitor configuration
type MonitorConfig struct {
	Name        string  `json:"name"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	RefreshRate float64 `json:"refresh_rate"`
	PositionX   int     `json:"position_x"`
	PositionY   int     `json:"position_y"`
	Primary     bool    `json:"primary,omitempty"`
	Enabled     bool    `json:"enabled"`
}

