package common

// Layout represents a complete display configuration that can be applied
type Layout struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Emoji       string       `json:"emoji,omitempty"`
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
	SourceIndex   int     `json:"source_index"`
	TargetIndex   int     `json:"target_index"`
	OutputTech    string  `json:"output_tech,omitempty"`
	Rotation      int     `json:"rotation,omitempty"`
	Scaling       string  `json:"scaling,omitempty"`
	RefreshRate   float64 `json:"refresh_rate"`
	ScanlineOrder string  `json:"scanline_order,omitempty"`
	IsPrimary     bool    `json:"is_primary,omitempty"`
}

// LayoutsConfig holds all available display layouts
type LayoutsConfig struct {
	Layouts []Layout `json:"layouts"`
}
