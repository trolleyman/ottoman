//go:build windows

package display

import (
	"fmt"
	"syscall"
	"unsafe"

	"github.com/pkg/errors"
	"github.com/trolleyman/ottoman/internal/common"
	"golang.org/x/sys/windows"
)

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procGetDisplayConfigBufferSizes = user32.NewProc("GetDisplayConfigBufferSizes")
	procQueryDisplayConfig          = user32.NewProc("QueryDisplayConfig")
	procSetDisplayConfig            = user32.NewProc("SetDisplayConfig")
	procDisplayConfigGetDeviceInfo  = user32.NewProc("DisplayConfigGetDeviceInfo")
)

// Query Display Config flags
const (
	QDC_ALL_PATHS           = 0x00000001
	QDC_ONLY_ACTIVE_PATHS   = 0x00000002
	QDC_DATABASE_CURRENT    = 0x00000004
	QDC_VIRTUAL_MODE_AWARE  = 0x00000010
)

// Set Display Config flags
const (
	SDC_TOPOLOGY_INTERNAL          = 0x00000001
	SDC_TOPOLOGY_CLONE             = 0x00000002
	SDC_TOPOLOGY_EXTEND            = 0x00000004
	SDC_TOPOLOGY_EXTERNAL          = 0x00000008
	SDC_APPLY                      = 0x00000080
	SDC_NO_OPTIMIZATION            = 0x00000100
	SDC_SAVE_TO_DATABASE           = 0x00000200
	SDC_ALLOW_CHANGES              = 0x00000400
	SDC_PATH_PERSIST_IF_REQUIRED   = 0x00000800
	SDC_USE_SUPPLIED_DISPLAY_CONFIG = 0x00000020
	SDC_VALIDATE                   = 0x00000040
	SDC_FORCE_MODE_ENUMERATION     = 0x00001000
	SDC_ALLOW_PATH_ORDER_CHANGES   = 0x00002000
)

// Path flags
const (
	DISPLAYCONFIG_PATH_ACTIVE = 0x00000001
)

// Mode info type
const (
	DISPLAYCONFIG_MODE_INFO_TYPE_SOURCE  = 1
	DISPLAYCONFIG_MODE_INFO_TYPE_TARGET  = 2
	DISPLAYCONFIG_MODE_INFO_TYPE_DESKTOP_IMAGE = 3
)

// Device info type for DisplayConfigGetDeviceInfo
const (
	DISPLAYCONFIG_DEVICE_INFO_GET_SOURCE_NAME       = 1
	DISPLAYCONFIG_DEVICE_INFO_GET_TARGET_NAME       = 2
	DISPLAYCONFIG_DEVICE_INFO_GET_TARGET_PREFERRED_MODE = 3
	DISPLAYCONFIG_DEVICE_INFO_GET_ADAPTER_NAME      = 4
	DISPLAYCONFIG_DEVICE_INFO_SET_TARGET_PERSISTENCE = 5
	DISPLAYCONFIG_DEVICE_INFO_GET_TARGET_BASE_TYPE  = 6
	DISPLAYCONFIG_DEVICE_INFO_GET_SUPPORT_VIRTUAL_RESOLUTION = 7
	DISPLAYCONFIG_DEVICE_INFO_SET_SUPPORT_VIRTUAL_RESOLUTION = 8
	DISPLAYCONFIG_DEVICE_INFO_GET_ADVANCED_COLOR_INFO = 9
	DISPLAYCONFIG_DEVICE_INFO_SET_ADVANCED_COLOR_STATE = 10
	DISPLAYCONFIG_DEVICE_INFO_GET_SDR_WHITE_LEVEL   = 11
)

// Output technology types (using int32 to match Windows API)
const (
	DISPLAYCONFIG_OUTPUT_TECHNOLOGY_OTHER                int32 = -1
	DISPLAYCONFIG_OUTPUT_TECHNOLOGY_HD15                 int32 = 0
	DISPLAYCONFIG_OUTPUT_TECHNOLOGY_SVIDEO               int32 = 1
	DISPLAYCONFIG_OUTPUT_TECHNOLOGY_COMPOSITE_VIDEO      int32 = 2
	DISPLAYCONFIG_OUTPUT_TECHNOLOGY_COMPONENT_VIDEO      int32 = 3
	DISPLAYCONFIG_OUTPUT_TECHNOLOGY_DVI                  int32 = 4
	DISPLAYCONFIG_OUTPUT_TECHNOLOGY_HDMI                 int32 = 5
	DISPLAYCONFIG_OUTPUT_TECHNOLOGY_LVDS                 int32 = 6
	DISPLAYCONFIG_OUTPUT_TECHNOLOGY_D_JPN                int32 = 8
	DISPLAYCONFIG_OUTPUT_TECHNOLOGY_SDI                  int32 = 9
	DISPLAYCONFIG_OUTPUT_TECHNOLOGY_DISPLAYPORT_EXTERNAL int32 = 10
	DISPLAYCONFIG_OUTPUT_TECHNOLOGY_DISPLAYPORT_EMBEDDED int32 = 11
	DISPLAYCONFIG_OUTPUT_TECHNOLOGY_UDI_EXTERNAL         int32 = 12
	DISPLAYCONFIG_OUTPUT_TECHNOLOGY_UDI_EMBEDDED         int32 = 13
	DISPLAYCONFIG_OUTPUT_TECHNOLOGY_SDTVDONGLE           int32 = 14
	DISPLAYCONFIG_OUTPUT_TECHNOLOGY_MIRACAST             int32 = 15
	DISPLAYCONFIG_OUTPUT_TECHNOLOGY_INDIRECT_WIRED       int32 = 16
	DISPLAYCONFIG_OUTPUT_TECHNOLOGY_INDIRECT_VIRTUAL     int32 = 17
	// DISPLAYCONFIG_OUTPUT_TECHNOLOGY_INTERNAL uses bit 31, treat as -2147483648 in int32
	DISPLAYCONFIG_OUTPUT_TECHNOLOGY_INTERNAL int32 = -2147483648
)

// LUID represents a locally unique identifier
type LUID struct {
	LowPart  uint32
	HighPart int32
}

// DISPLAYCONFIG_RATIONAL represents a fractional value
type DISPLAYCONFIG_RATIONAL struct {
	Numerator   uint32
	Denominator uint32
}

// DISPLAYCONFIG_2DREGION represents a 2D region
type DISPLAYCONFIG_2DREGION struct {
	Cx uint32
	Cy uint32
}

// POINTL represents a point
type POINTL struct {
	X int32
	Y int32
}

// DISPLAYCONFIG_PATH_SOURCE_INFO contains source info for a path
type DISPLAYCONFIG_PATH_SOURCE_INFO struct {
	AdapterId   LUID
	Id          uint32
	ModeInfoIdx uint32 // Union with cloneGroupId
	StatusFlags uint32
}

// DISPLAYCONFIG_PATH_TARGET_INFO contains target info for a path
type DISPLAYCONFIG_PATH_TARGET_INFO struct {
	AdapterId        LUID
	Id               uint32
	ModeInfoIdx      uint32 // Union with desktopModeInfoIdx
	OutputTechnology int32
	Rotation         uint32
	Scaling          uint32
	RefreshRate      DISPLAYCONFIG_RATIONAL
	ScanLineOrdering uint32
	TargetAvailable  int32
	StatusFlags      uint32
}

// DISPLAYCONFIG_PATH_INFO contains display path information
type DISPLAYCONFIG_PATH_INFO struct {
	SourceInfo DISPLAYCONFIG_PATH_SOURCE_INFO
	TargetInfo DISPLAYCONFIG_PATH_TARGET_INFO
	Flags      uint32
}

// DISPLAYCONFIG_VIDEO_SIGNAL_INFO contains video signal info
type DISPLAYCONFIG_VIDEO_SIGNAL_INFO struct {
	PixelRate          uint64
	HSyncFreq          DISPLAYCONFIG_RATIONAL
	VSyncFreq          DISPLAYCONFIG_RATIONAL
	ActiveSize         DISPLAYCONFIG_2DREGION
	TotalSize          DISPLAYCONFIG_2DREGION
	VideoStandardPacked uint32 // Union containing videoStandard and vSyncFreqDivider
	ScanLineOrdering   uint32
}

// DISPLAYCONFIG_SOURCE_MODE contains source mode info
type DISPLAYCONFIG_SOURCE_MODE struct {
	Width       uint32
	Height      uint32
	PixelFormat uint32
	Position    POINTL
}

// DISPLAYCONFIG_TARGET_MODE contains target mode info
type DISPLAYCONFIG_TARGET_MODE struct {
	TargetVideoSignalInfo DISPLAYCONFIG_VIDEO_SIGNAL_INFO
}

// DISPLAYCONFIG_MODE_INFO contains mode information
// Note: This is a union in C, we use the largest variant
type DISPLAYCONFIG_MODE_INFO struct {
	InfoType   uint32
	Id         uint32
	AdapterId  LUID
	ModeUnion  [64]byte // Union of source/target mode, using bytes for flexibility
}

// DISPLAYCONFIG_DEVICE_INFO_HEADER is the header for device info requests
type DISPLAYCONFIG_DEVICE_INFO_HEADER struct {
	Type      uint32
	Size      uint32
	AdapterId LUID
	Id        uint32
}

// DISPLAYCONFIG_SOURCE_DEVICE_NAME contains source device name
type DISPLAYCONFIG_SOURCE_DEVICE_NAME struct {
	Header         DISPLAYCONFIG_DEVICE_INFO_HEADER
	ViewGdiDeviceName [32]uint16
}

// DISPLAYCONFIG_TARGET_DEVICE_NAME contains target device name
type DISPLAYCONFIG_TARGET_DEVICE_NAME struct {
	Header                   DISPLAYCONFIG_DEVICE_INFO_HEADER
	Flags                    uint32
	OutputTechnology         uint32
	EdidManufactureId        uint16
	EdidProductCodeId        uint16
	ConnectorInstance        uint32
	MonitorFriendlyDeviceName [64]uint16
	MonitorDevicePath         [128]uint16
}

// WindowsManager implements display management on Windows using native APIs
type WindowsManager struct {
	store *Layouts
}

func newPlatformManager(store *Layouts) (Manager, error) {
	return &WindowsManager{store: store}, nil
}

// getDisplayConfigBufferSizes returns the required buffer sizes
func getDisplayConfigBufferSizes(flags uint32) (numPaths, numModes uint32, err error) {
	ret, _, _ := procGetDisplayConfigBufferSizes.Call(
		uintptr(flags),
		uintptr(unsafe.Pointer(&numPaths)),
		uintptr(unsafe.Pointer(&numModes)),
	)
	if ret != 0 {
		return 0, 0, fmt.Errorf("GetDisplayConfigBufferSizes failed: %d", ret)
	}
	return numPaths, numModes, nil
}

// queryDisplayConfig queries the current display configuration
func queryDisplayConfig(flags uint32) ([]DISPLAYCONFIG_PATH_INFO, []DISPLAYCONFIG_MODE_INFO, error) {
	numPaths, numModes, err := getDisplayConfigBufferSizes(flags)
	if err != nil {
		return nil, nil, err
	}

	if numPaths == 0 {
		return nil, nil, nil
	}

	paths := make([]DISPLAYCONFIG_PATH_INFO, numPaths)
	modes := make([]DISPLAYCONFIG_MODE_INFO, numModes)

	var pathsPtr, modesPtr unsafe.Pointer
	if numPaths > 0 {
		pathsPtr = unsafe.Pointer(&paths[0])
	}
	if numModes > 0 {
		modesPtr = unsafe.Pointer(&modes[0])
	}

	ret, _, _ := procQueryDisplayConfig.Call(
		uintptr(flags),
		uintptr(unsafe.Pointer(&numPaths)),
		uintptr(pathsPtr),
		uintptr(unsafe.Pointer(&numModes)),
		uintptr(modesPtr),
		0, // currentTopologyId (optional)
	)
	if ret != 0 {
		return nil, nil, fmt.Errorf("QueryDisplayConfig failed: %d", ret)
	}

	return paths[:numPaths], modes[:numModes], nil
}

// getTargetDeviceName gets the device name for a target
func getTargetDeviceName(adapterId LUID, targetId uint32) (*DISPLAYCONFIG_TARGET_DEVICE_NAME, error) {
	var name DISPLAYCONFIG_TARGET_DEVICE_NAME
	name.Header.Type = DISPLAYCONFIG_DEVICE_INFO_GET_TARGET_NAME
	name.Header.Size = uint32(unsafe.Sizeof(name))
	name.Header.AdapterId = adapterId
	name.Header.Id = targetId

	ret, _, _ := procDisplayConfigGetDeviceInfo.Call(uintptr(unsafe.Pointer(&name)))
	if ret != 0 {
		return nil, fmt.Errorf("DisplayConfigGetDeviceInfo failed: %d", ret)
	}
	return &name, nil
}

// getSourceDeviceName gets the device name for a source
func getSourceDeviceName(adapterId LUID, sourceId uint32) (*DISPLAYCONFIG_SOURCE_DEVICE_NAME, error) {
	var name DISPLAYCONFIG_SOURCE_DEVICE_NAME
	name.Header.Type = DISPLAYCONFIG_DEVICE_INFO_GET_SOURCE_NAME
	name.Header.Size = uint32(unsafe.Sizeof(name))
	name.Header.AdapterId = adapterId
	name.Header.Id = sourceId

	ret, _, _ := procDisplayConfigGetDeviceInfo.Call(uintptr(unsafe.Pointer(&name)))
	if ret != 0 {
		return nil, fmt.Errorf("DisplayConfigGetDeviceInfo (source) failed: %d", ret)
	}
	return &name, nil
}

// utf16ToString converts a null-terminated UTF16 slice to string
func utf16ToString(s []uint16) string {
	for i, v := range s {
		if v == 0 {
			return syscall.UTF16ToString(s[:i])
		}
	}
	return syscall.UTF16ToString(s)
}

// outputTechnologyToString converts output technology to a readable string
func outputTechnologyToString(tech int32) string {
	switch tech {
	case DISPLAYCONFIG_OUTPUT_TECHNOLOGY_HD15:
		return "VGA"
	case DISPLAYCONFIG_OUTPUT_TECHNOLOGY_DVI:
		return "DVI"
	case DISPLAYCONFIG_OUTPUT_TECHNOLOGY_HDMI:
		return "HDMI"
	case DISPLAYCONFIG_OUTPUT_TECHNOLOGY_DISPLAYPORT_EXTERNAL:
		return "DP"
	case DISPLAYCONFIG_OUTPUT_TECHNOLOGY_DISPLAYPORT_EMBEDDED:
		return "eDP"
	case DISPLAYCONFIG_OUTPUT_TECHNOLOGY_INTERNAL:
		return "Internal"
	default:
		return fmt.Sprintf("Unknown(%d)", tech)
	}
}

// ListMonitors returns information about connected monitors using Windows Display Config API
func (m *WindowsManager) ListMonitors() ([]MonitorInfo, error) {
	paths, modes, err := queryDisplayConfig(QDC_ONLY_ACTIVE_PATHS)
	if err != nil {
		return nil, errors.Wrap(err, "failed to query display config")
	}

	var monitors []MonitorInfo

	for _, path := range paths {
		if path.Flags&DISPLAYCONFIG_PATH_ACTIVE == 0 {
			continue
		}

		// Get target device name (contains EDID info)
		targetName, err := getTargetDeviceName(path.TargetInfo.AdapterId, path.TargetInfo.Id)
		if err != nil {
			continue
		}

		// Get source device name (GDI device name like \\.\DISPLAY1)
		sourceName, err := getSourceDeviceName(path.SourceInfo.AdapterId, path.SourceInfo.Id)
		if err != nil {
			continue
		}

		// Build EDID string from manufacturer ID and product code
		edid := ""
		if targetName.EdidManufactureId != 0 {
			// EDID manufacturer ID is encoded as 3 5-bit characters
			mfg := decodeEdidManufacturer(targetName.EdidManufactureId)
			edid = fmt.Sprintf("%s:%04X", mfg, targetName.EdidProductCodeId)
		}

		// Get friendly name
		friendlyName := utf16ToString(targetName.MonitorFriendlyDeviceName[:])

		// Get GDI device name
		gdiName := utf16ToString(sourceName.ViewGdiDeviceName[:])

		// Build port/connector string
		port := fmt.Sprintf("%s-%d", outputTechnologyToString(int32(targetName.OutputTechnology)), targetName.ConnectorInstance)

		// Get resolution from source mode
		var width, height int
		var posX, posY int
		if path.SourceInfo.ModeInfoIdx != 0xFFFFFFFF && int(path.SourceInfo.ModeInfoIdx) < len(modes) {
			modeInfo := modes[path.SourceInfo.ModeInfoIdx]
			if modeInfo.InfoType == DISPLAYCONFIG_MODE_INFO_TYPE_SOURCE {
				sourceMode := (*DISPLAYCONFIG_SOURCE_MODE)(unsafe.Pointer(&modeInfo.ModeUnion[0]))
				width = int(sourceMode.Width)
				height = int(sourceMode.Height)
				posX = int(sourceMode.Position.X)
				posY = int(sourceMode.Position.Y)
			}
		}

		// Get refresh rate from target mode
		var refreshRate float64
		if path.TargetInfo.ModeInfoIdx != 0xFFFFFFFF && int(path.TargetInfo.ModeInfoIdx) < len(modes) {
			modeInfo := modes[path.TargetInfo.ModeInfoIdx]
			if modeInfo.InfoType == DISPLAYCONFIG_MODE_INFO_TYPE_TARGET {
				targetMode := (*DISPLAYCONFIG_TARGET_MODE)(unsafe.Pointer(&modeInfo.ModeUnion[0]))
				if targetMode.TargetVideoSignalInfo.VSyncFreq.Denominator > 0 {
					refreshRate = float64(targetMode.TargetVideoSignalInfo.VSyncFreq.Numerator) /
						float64(targetMode.TargetVideoSignalInfo.VSyncFreq.Denominator)
				}
			}
		}

		// Check if this is the primary display (position 0,0 is typically primary)
		primary := posX == 0 && posY == 0

		monitors = append(monitors, MonitorInfo{
			EDID:         edid,
			Port:         port,
			Name:         friendlyName,
			Manufacturer: decodeEdidManufacturer(targetName.EdidManufactureId),
			Model:        gdiName,
			Width:        width,
			Height:       height,
			RefreshRate:  refreshRate,
			PositionX:    posX,
			PositionY:    posY,
			Primary:      primary,
			Connected:    true,
		})
	}

	return monitors, nil
}

// decodeEdidManufacturer decodes the EDID manufacturer ID to a 3-letter string
func decodeEdidManufacturer(id uint16) string {
	// EDID manufacturer ID is big-endian, need to swap bytes
	id = (id >> 8) | (id << 8)

	// 5 bits per character, 'A' = 1
	c1 := ((id >> 10) & 0x1F) + 'A' - 1
	c2 := ((id >> 5) & 0x1F) + 'A' - 1
	c3 := (id & 0x1F) + 'A' - 1

	return string([]byte{byte(c1), byte(c2), byte(c3)})
}

// GetCurrentLayout attempts to identify the current layout
func (m *WindowsManager) GetCurrentLayout(layouts *Layouts) (string, error) {
	monitors, err := m.ListMonitors()
	if err != nil {
		return "", err
	}

	// Try to match current state to a known layout
	for _, layout := range layouts.List() {
		if m.matchesLayout(monitors, layout) {
			return layout.ID, nil
		}
	}

	return "", nil
}

// matchesLayout checks if current monitors match a layout
func (m *WindowsManager) matchesLayout(monitors []MonitorInfo, layout common.Layout) bool {
	enabledCount := 0
	for _, lm := range layout.Monitors {
		if lm.Enabled {
			enabledCount++
		}
	}

	connectedCount := 0
	for _, mon := range monitors {
		if mon.Connected && mon.Width > 0 {
			connectedCount++
		}
	}

	if connectedCount != enabledCount {
		return false
	}

	// Check each layout monitor matches a physical monitor
	for _, lm := range layout.Monitors {
		if !lm.Enabled {
			continue
		}
		found := false
		for _, mon := range monitors {
			// Match by EDID first (preferred), then by port
			if lm.EDID != "" && lm.EDID == mon.EDID {
				if mon.Width == lm.Width && mon.Height == lm.Height {
					found = true
					break
				}
			} else if lm.Port != "" && lm.Port == mon.Port {
				if mon.Width == lm.Width && mon.Height == lm.Height {
					found = true
					break
				}
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// ApplyLayoutConfig applies a display configuration using SetDisplayConfig
func (m *WindowsManager) ApplyLayoutConfig(layout common.Layout) error {
	// Get current config to use as base
	paths, modes, err := queryDisplayConfig(QDC_ALL_PATHS)
	if err != nil {
		return errors.Wrap(err, "failed to query current display config")
	}

	// Count enabled monitors in layout
	enabledCount := 0
	for _, mon := range layout.Monitors {
		if mon.Enabled {
			enabledCount++
		}
	}

	// Simple topologies via SDC flags
	if enabledCount == 1 {
		// Find which display to use
		for _, lm := range layout.Monitors {
			if lm.Enabled {
				// Check if it's the internal display or external
				if lm.Port == "Internal" || lm.Port == "eDP-1" {
					return setDisplayTopology(SDC_TOPOLOGY_INTERNAL)
				}
				return setDisplayTopology(SDC_TOPOLOGY_EXTERNAL)
			}
		}
	}

	// For multi-monitor, we need to set up paths properly
	return m.applyMultiMonitorLayout(layout, paths, modes)
}

// setDisplayTopology sets a simple display topology
func setDisplayTopology(topology uint32) error {
	ret, _, _ := procSetDisplayConfig.Call(
		0,     // numPathArrayElements
		0,     // pathArray
		0,     // numModeInfoArrayElements
		0,     // modeInfoArray
		uintptr(topology|SDC_APPLY),
	)
	if ret != 0 {
		return fmt.Errorf("SetDisplayConfig topology failed: %d", ret)
	}
	return nil
}

// applyMultiMonitorLayout applies a complex multi-monitor layout
func (m *WindowsManager) applyMultiMonitorLayout(layout common.Layout, paths []DISPLAYCONFIG_PATH_INFO, modes []DISPLAYCONFIG_MODE_INFO) error {
	// Get all monitors to build a mapping
	monitors, err := m.ListMonitors()
	if err != nil {
		return err
	}

	// Build mapping from EDID/Port to monitor index
	monitorMap := make(map[string]int)
	for i, mon := range monitors {
		if mon.EDID != "" {
			monitorMap[mon.EDID] = i
		}
		if mon.Port != "" {
			monitorMap[mon.Port] = i
		}
	}

	// Modify paths to match layout
	// For now, just enable extend mode and let Windows handle positioning
	// A full implementation would modify the source modes with positions

	// Use extend topology as base
	err = setDisplayTopology(SDC_TOPOLOGY_EXTEND)
	if err != nil {
		return errors.Wrap(err, "failed to set extend topology")
	}

	// TODO: For precise positioning, we'd need to:
	// 1. Query the new configuration after extend
	// 2. Modify source mode positions
	// 3. Call SetDisplayConfig with SDC_USE_SUPPLIED_DISPLAY_CONFIG | SDC_APPLY

	return nil
}
