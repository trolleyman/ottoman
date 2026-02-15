//go:build windows

package display

import (
	"fmt"
	"log"
	"syscall"
	"unsafe"

	"github.com/pkg/errors"
	"github.com/trolleyman/ottoman/internal/api"
	"golang.org/x/sys/windows"
)

var (
	user32                          = windows.NewLazySystemDLL("user32.dll")
	procGetDisplayConfigBufferSizes = user32.NewProc("GetDisplayConfigBufferSizes")
	procQueryDisplayConfig          = user32.NewProc("QueryDisplayConfig")
	procSetDisplayConfig            = user32.NewProc("SetDisplayConfig")
	procDisplayConfigGetDeviceInfo  = user32.NewProc("DisplayConfigGetDeviceInfo")
	procEnumDisplaySettingsW        = user32.NewProc("EnumDisplaySettingsW")
)

const ENUM_CURRENT_SETTINGS = 0xFFFFFFFF

// DEVMODEW structure for EnumDisplaySettings
type DEVMODEW struct {
	DmDeviceName         [32]uint16
	DmSpecVersion        uint16
	DmDriverVersion      uint16
	DmSize               uint16
	DmDriverExtra        uint16
	DmFields             uint32
	DmPositionX          int32 // Union: dmPosition.x or dmOrientation
	DmPositionY          int32 // Union: dmPosition.y
	DmDisplayOrientation uint32
	DmDisplayFixedOutput uint32
	DmColor              int16
	DmDuplex             int16
	DmYResolution        int16
	DmTTOption           int16
	DmCollate            int16
	DmFormName           [32]uint16
	DmLogPixels          uint16
	DmBitsPerPel         uint32
	DmPelsWidth          uint32
	DmPelsHeight         uint32
	DmDisplayFlags       uint32 // Union with dmNup
	DmDisplayFrequency   uint32
	// Additional fields omitted - we only need up to dmDisplayFrequency
}

// Query Display Config flags
const (
	QDC_ALL_PATHS          = 0x00000001
	QDC_ONLY_ACTIVE_PATHS  = 0x00000002
	QDC_DATABASE_CURRENT   = 0x00000004
	QDC_VIRTUAL_MODE_AWARE = 0x00000010
)

// Set Display Config flags
const (
	SDC_TOPOLOGY_INTERNAL           = 0x00000001
	SDC_TOPOLOGY_CLONE              = 0x00000002
	SDC_TOPOLOGY_EXTEND             = 0x00000004
	SDC_TOPOLOGY_EXTERNAL           = 0x00000008
	SDC_APPLY                       = 0x00000080
	SDC_NO_OPTIMIZATION             = 0x00000100
	SDC_SAVE_TO_DATABASE            = 0x00000200
	SDC_ALLOW_CHANGES               = 0x00000400
	SDC_PATH_PERSIST_IF_REQUIRED    = 0x00000800
	SDC_USE_SUPPLIED_DISPLAY_CONFIG = 0x00000020
	SDC_VALIDATE                    = 0x00000040
	SDC_FORCE_MODE_ENUMERATION      = 0x00001000
	SDC_ALLOW_PATH_ORDER_CHANGES    = 0x00002000
)

// Path flags
const (
	DISPLAYCONFIG_PATH_ACTIVE = 0x00000001
)

// Mode info type
const (
	DISPLAYCONFIG_MODE_INFO_TYPE_SOURCE        = 1
	DISPLAYCONFIG_MODE_INFO_TYPE_TARGET        = 2
	DISPLAYCONFIG_MODE_INFO_TYPE_DESKTOP_IMAGE = 3
)

// Device info type for DisplayConfigGetDeviceInfo
const (
	DISPLAYCONFIG_DEVICE_INFO_GET_SOURCE_NAME                = 1
	DISPLAYCONFIG_DEVICE_INFO_GET_TARGET_NAME                = 2
	DISPLAYCONFIG_DEVICE_INFO_GET_TARGET_PREFERRED_MODE      = 3
	DISPLAYCONFIG_DEVICE_INFO_GET_ADAPTER_NAME               = 4
	DISPLAYCONFIG_DEVICE_INFO_SET_TARGET_PERSISTENCE         = 5
	DISPLAYCONFIG_DEVICE_INFO_GET_TARGET_BASE_TYPE           = 6
	DISPLAYCONFIG_DEVICE_INFO_GET_SUPPORT_VIRTUAL_RESOLUTION = 7
	DISPLAYCONFIG_DEVICE_INFO_SET_SUPPORT_VIRTUAL_RESOLUTION = 8
	DISPLAYCONFIG_DEVICE_INFO_GET_ADVANCED_COLOR_INFO        = 9
	DISPLAYCONFIG_DEVICE_INFO_SET_ADVANCED_COLOR_STATE       = 10
	DISPLAYCONFIG_DEVICE_INFO_GET_SDR_WHITE_LEVEL            = 11
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
	AdapterId   windows.LUID
	Id          uint32
	ModeInfoIdx uint32 // Union with cloneGroupId
	StatusFlags uint32
}

// DISPLAYCONFIG_PATH_TARGET_INFO contains target info for a path
type DISPLAYCONFIG_PATH_TARGET_INFO struct {
	AdapterId        windows.LUID
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
	PixelRate           uint64
	HSyncFreq           DISPLAYCONFIG_RATIONAL
	VSyncFreq           DISPLAYCONFIG_RATIONAL
	ActiveSize          DISPLAYCONFIG_2DREGION
	TotalSize           DISPLAYCONFIG_2DREGION
	VideoStandardPacked uint32 // Union containing videoStandard and vSyncFreqDivider
	ScanLineOrdering    uint32
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
	InfoType  uint32
	Id        uint32
	AdapterId windows.LUID
	ModeUnion [48]byte // Union of source/target mode (48 bytes = 64 total - 16 header)
}

// DISPLAYCONFIG_DEVICE_INFO_HEADER is the header for device info requests
type DISPLAYCONFIG_DEVICE_INFO_HEADER struct {
	Type      uint32
	Size      uint32
	AdapterId windows.LUID
	Id        uint32
}

// DISPLAYCONFIG_SOURCE_DEVICE_NAME contains source device name
type DISPLAYCONFIG_SOURCE_DEVICE_NAME struct {
	Header            DISPLAYCONFIG_DEVICE_INFO_HEADER
	ViewGdiDeviceName [32]uint16
}

// DISPLAYCONFIG_TARGET_DEVICE_NAME contains target device name
type DISPLAYCONFIG_TARGET_DEVICE_NAME struct {
	Header                    DISPLAYCONFIG_DEVICE_INFO_HEADER
	Flags                     uint32
	OutputTechnology          uint32
	EdidManufactureId         uint16
	EdidProductCodeId         uint16
	ConnectorInstance         uint32
	MonitorFriendlyDeviceName [64]uint16
	MonitorDevicePath         [128]uint16
}

// DISPLAYCONFIG_TARGET_PREFERRED_MODE contains preferred mode info
type DISPLAYCONFIG_TARGET_PREFERRED_MODE struct {
	Header     DISPLAYCONFIG_DEVICE_INFO_HEADER
	Width      uint32
	Height     uint32
	TargetMode DISPLAYCONFIG_TARGET_MODE
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
func getTargetDeviceName(adapterId windows.LUID, targetId uint32) (*DISPLAYCONFIG_TARGET_DEVICE_NAME, error) {
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
func getSourceDeviceName(adapterId windows.LUID, sourceId uint32) (*DISPLAYCONFIG_SOURCE_DEVICE_NAME, error) {
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

// getTargetPreferredMode gets the preferred mode for a target
func getTargetPreferredMode(adapterId windows.LUID, targetId uint32) (*DISPLAYCONFIG_TARGET_PREFERRED_MODE, error) {
	var mode DISPLAYCONFIG_TARGET_PREFERRED_MODE
	mode.Header.Type = DISPLAYCONFIG_DEVICE_INFO_GET_TARGET_PREFERRED_MODE
	mode.Header.Size = uint32(unsafe.Sizeof(mode))
	mode.Header.AdapterId = adapterId
	mode.Header.Id = targetId

	ret, _, _ := procDisplayConfigGetDeviceInfo.Call(uintptr(unsafe.Pointer(&mode)))
	if ret != 0 {
		return nil, fmt.Errorf("DisplayConfigGetDeviceInfo (preferred mode) failed: %d", ret)
	}
	return &mode, nil
}

// getDisplaySettings gets current display settings using EnumDisplaySettingsW
func getDisplaySettings(deviceName string) (*DEVMODEW, error) {
	deviceNameUTF16, err := syscall.UTF16PtrFromString(deviceName)
	if err != nil {
		return nil, err
	}

	var devMode DEVMODEW
	devMode.DmSize = uint16(unsafe.Sizeof(devMode))

	ret, _, _ := procEnumDisplaySettingsW.Call(
		uintptr(unsafe.Pointer(deviceNameUTF16)),
		ENUM_CURRENT_SETTINGS,
		uintptr(unsafe.Pointer(&devMode)),
	)
	if ret == 0 {
		return nil, fmt.Errorf("EnumDisplaySettingsW failed for %s", deviceName)
	}
	return &devMode, nil
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
		return fmt.Sprintf("Unknown(%x)", tech)
	}
}

// ListMonitors returns information about all monitors (active and inactive) using Windows Display Config API
func (m *WindowsManager) ListMonitors() ([]api.Monitor, error) {
	// Query all paths to discover every target (connected or not)
	allPaths, allModes, err := queryDisplayConfig(QDC_ALL_PATHS)
	if err != nil {
		return nil, errors.Wrap(err, "failed to query all display paths")
	}

	// Build map of all monitors keyed by target identity, deduplicating by EDID
	type targetKey struct {
		AdapterId windows.LUID
		Id        uint32
	}
	monitors := make(map[targetKey]api.Monitor)

	for _, path := range allPaths {
		if path.TargetInfo.TargetAvailable == 0 {
			continue
		}

		key := targetKey{
			AdapterId: path.TargetInfo.AdapterId,
			Id:        path.TargetInfo.Id,
		}

		// Skip if we already have this target
		if _, ok := monitors[key]; ok {
			continue
		}

		// Get target device name (contains EDID info)
		targetName, err := getTargetDeviceName(path.TargetInfo.AdapterId, path.TargetInfo.Id)
		if err != nil || targetName.EdidManufactureId == 0 {
			continue
		}

		edid := fmt.Sprintf("%s:%04X", decodeEdidManufacturer(targetName.EdidManufactureId), targetName.EdidProductCodeId)
		friendlyName := utf16ToString(targetName.MonitorFriendlyDeviceName[:])
		port := fmt.Sprintf("%s-%d", outputTechnologyToString(int32(targetName.OutputTechnology)), targetName.ConnectorInstance)

		monitors[key] = api.Monitor{
			Edid:         edid,
			Port:         port,
			Name:         friendlyName,
			Manufacturer: decodeEdidManufacturer(targetName.EdidManufactureId),
			Active:       nil,
		}
	}

	// Go through active paths and populate api.ActiveMonitor from source modes
	for _, path := range allPaths {
		if path.Flags&DISPLAYCONFIG_PATH_ACTIVE == 0 {
			continue
		}

		key := targetKey{
			AdapterId: path.TargetInfo.AdapterId,
			Id:        path.TargetInfo.Id,
		}

		// Find or create the monitor monitor (it should already exist from pass 1)
		monitor, ok := monitors[key]
		if !ok {
			continue
		}

		// Get source device name for EnumDisplaySettings
		sourceName, err := getSourceDeviceName(path.SourceInfo.AdapterId, path.SourceInfo.Id)
		if err != nil {
			continue
		}
		gdiName := utf16ToString(sourceName.ViewGdiDeviceName[:])

		// Get resolution, refresh rate, and position from EnumDisplaySettings (most reliable)
		var width, height int
		var posX, posY int
		var refreshRate float64

		if devMode, err := getDisplaySettings(gdiName); err == nil {
			width = int(devMode.DmPelsWidth)
			height = int(devMode.DmPelsHeight)
			posX = int(devMode.DmPositionX)
			posY = int(devMode.DmPositionY)
			refreshRate = float64(devMode.DmDisplayFrequency)
		}

		// Fallback to DisplayConfig mode info
		if width == 0 || height == 0 {
			targetModeIdx := path.TargetInfo.ModeInfoIdx & 0xFFFF
			if targetModeIdx != 0xFFFF && int(targetModeIdx) < len(allModes) {
				modeInfo := allModes[targetModeIdx]
				if modeInfo.InfoType == DISPLAYCONFIG_MODE_INFO_TYPE_TARGET {
					targetMode := (*DISPLAYCONFIG_TARGET_MODE)(unsafe.Pointer(&modeInfo.ModeUnion[0]))
					width = int(targetMode.TargetVideoSignalInfo.ActiveSize.Cx)
					height = int(targetMode.TargetVideoSignalInfo.ActiveSize.Cy)
					if refreshRate == 0 && targetMode.TargetVideoSignalInfo.VSyncFreq.Denominator > 0 {
						refreshRate = float64(targetMode.TargetVideoSignalInfo.VSyncFreq.Numerator) /
							float64(targetMode.TargetVideoSignalInfo.VSyncFreq.Denominator)
					}
				}
			}
		}

		// Final fallback: preferred mode
		if width == 0 || height == 0 {
			if prefMode, err := getTargetPreferredMode(path.TargetInfo.AdapterId, path.TargetInfo.Id); err == nil {
				width = int(prefMode.Width)
				height = int(prefMode.Height)
			}
		}

		primary := posX == 0 && posY == 0

		monitor.Active = &api.ActiveMonitor{
			Width:       width,
			Height:      height,
			RefreshRate: refreshRate,
			PositionX:   posX,
			PositionY:   posY,
			Primary:     primary,
			Model:       gdiName,
		}
		monitors[key] = monitor
	}

	// Collect results
	var monitorsList []api.Monitor
	for _, monitor := range monitors {
		monitorsList = append(monitorsList, monitor)
	}

	SortMonitors(monitorsList)

	return monitorsList, nil
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

// Status flags
const (
	DISPLAYCONFIG_SOURCE_IN_USE = 0x00000001
	DISPLAYCONFIG_TARGET_IN_USE = 0x00000001
)

// displayPathInfo holds all the info needed to build a path
type displayPathInfo struct {
	edid            string
	friendlyName    string
	sourceAdapterId windows.LUID
	sourceId        uint32
	targetAdapterId windows.LUID
	targetId        uint32
	targetMode      DISPLAYCONFIG_TARGET_MODE
	outputTech      int32
	rotation        uint32
	scaling         uint32
	refreshRate     DISPLAYCONFIG_RATIONAL
	scanLineOrder   uint32
}

// getDisplayPathsFromConfig queries display config and extracts path info with target modes
func getDisplayPathsFromConfig() ([]displayPathInfo, error) {
	// Query ALL paths to get all possible connections
	allPaths, allModes, err := queryDisplayConfig(QDC_ALL_PATHS)
	if err != nil {
		return nil, err
	}

	var result []displayPathInfo
	seen := make(map[string]bool)

	for _, path := range allPaths {
		// Get target device name (contains EDID)
		targetName, err := getTargetDeviceName(path.TargetInfo.AdapterId, path.TargetInfo.Id)
		if err != nil || targetName.EdidManufactureId == 0 {
			continue
		}

		edid := fmt.Sprintf("%s:%04X", decodeEdidManufacturer(targetName.EdidManufactureId), targetName.EdidProductCodeId)
		friendlyName := utf16ToString(targetName.MonitorFriendlyDeviceName[:])

		// Deduplicate by EDID
		if seen[edid] {
			continue
		}
		seen[edid] = true

		// Get target mode from the modes array if available
		var targetMode DISPLAYCONFIG_TARGET_MODE
		tgtModeIdx := path.TargetInfo.ModeInfoIdx & 0xFFFF
		if tgtModeIdx != 0xFFFF && int(tgtModeIdx) < len(allModes) {
			modeInfo := allModes[tgtModeIdx]
			if modeInfo.InfoType == DISPLAYCONFIG_MODE_INFO_TYPE_TARGET {
				targetMode = *(*DISPLAYCONFIG_TARGET_MODE)(unsafe.Pointer(&modeInfo.ModeUnion[0]))
			}
		}

		// If no mode found, get preferred mode
		if targetMode.TargetVideoSignalInfo.ActiveSize.Cx == 0 {
			if prefMode, err := getTargetPreferredMode(path.TargetInfo.AdapterId, path.TargetInfo.Id); err == nil {
				targetMode = prefMode.TargetMode
			}
		}

		result = append(result, displayPathInfo{
			edid:            edid,
			friendlyName:    friendlyName,
			sourceAdapterId: path.SourceInfo.AdapterId,
			sourceId:        path.SourceInfo.Id,
			targetAdapterId: path.TargetInfo.AdapterId,
			targetId:        path.TargetInfo.Id,
			targetMode:      targetMode,
			outputTech:      path.TargetInfo.OutputTechnology,
			rotation:        path.TargetInfo.Rotation,
			scaling:         path.TargetInfo.Scaling,
			refreshRate:     path.TargetInfo.RefreshRate,
			scanLineOrder:   path.TargetInfo.ScanLineOrdering,
		})
	}

	return result, nil
}

// ApplyLayoutConfig applies a display configuration using SetDisplayConfig
func (m *WindowsManager) ApplyLayoutConfig(layout api.Layout) error {
	log.Printf("Applying layout: %s (%s)", layout.Id, layout.Name)

	// Build map of EDIDs that should be enabled with their config
	monitorsByEdid := make(map[string]api.LayoutMonitor)
	for _, mon := range layout.Monitors {
		monitorsByEdid[mon.Edid] = mon
	}

	// Get all display path info
	pathInfos, err := getDisplayPathsFromConfig()
	if err != nil {
		return errors.Wrap(err, "failed to get display paths")
	}

	// Collect mode data for each enabled monitor
	type monitorModeData struct {
		edid      string
		layoutMon api.LayoutMonitor
		info      *displayPathInfo
		sourceId  uint32
	}
	var monitorData []monitorModeData
	sourceIdCounter := uint32(0)

	for edid, layoutMon := range monitorsByEdid {
		// Find the path info for this EDID
		var info *displayPathInfo
		for i := range pathInfos {
			if pathInfos[i].edid == edid {
				info = &pathInfos[i]
				break
			}
		}

		if info == nil {
			log.Printf("WARNING: No path found for EDID %s", edid)
			continue
		}

		monitorData = append(monitorData, monitorModeData{
			edid:      edid,
			layoutMon: layoutMon,
			info:      info,
			sourceId:  sourceIdCounter,
		})
		sourceIdCounter++
	}

	numMonitors := uint32(len(monitorData))

	// Build target modes first, then source modes
	var targetModes []DISPLAYCONFIG_MODE_INFO
	var sourceModes []DISPLAYCONFIG_MODE_INFO
	var paths []DISPLAYCONFIG_PATH_INFO

	for i, md := range monitorData {
		// Create target mode
		var targetMode DISPLAYCONFIG_MODE_INFO
		targetMode.InfoType = DISPLAYCONFIG_MODE_INFO_TYPE_TARGET
		targetMode.Id = md.info.targetId
		targetMode.AdapterId = md.info.targetAdapterId

		tgtMode := (*DISPLAYCONFIG_TARGET_MODE)(unsafe.Pointer(&targetMode.ModeUnion[0]))
		*tgtMode = md.info.targetMode

		targetModes = append(targetModes, targetMode)

		// Create source mode
		var sourceMode DISPLAYCONFIG_MODE_INFO
		sourceMode.InfoType = DISPLAYCONFIG_MODE_INFO_TYPE_SOURCE
		sourceMode.Id = md.sourceId
		sourceMode.AdapterId = md.info.targetAdapterId

		srcMode := (*DISPLAYCONFIG_SOURCE_MODE)(unsafe.Pointer(&sourceMode.ModeUnion[0]))
		srcMode.Width = uint32(md.layoutMon.Width)
		srcMode.Height = uint32(md.layoutMon.Height)
		srcMode.PixelFormat = 4 // DISPLAYCONFIG_PIXELFORMAT_32BPP
		srcMode.Position.X = int32(md.layoutMon.PositionX)
		srcMode.Position.Y = int32(md.layoutMon.PositionY)

		sourceModes = append(sourceModes, sourceMode)

		// Calculate indices - target modes are at 0..numMonitors-1, source modes at numMonitors..2*numMonitors-1
		targetModeIdx := uint32(i)
		sourceModeIdx := numMonitors + uint32(i)

		// Create the path
		var path DISPLAYCONFIG_PATH_INFO
		path.SourceInfo.AdapterId = md.info.targetAdapterId
		path.SourceInfo.Id = md.sourceId
		path.SourceInfo.ModeInfoIdx = sourceModeIdx
		path.SourceInfo.StatusFlags = DISPLAYCONFIG_SOURCE_IN_USE

		path.TargetInfo.AdapterId = md.info.targetAdapterId
		path.TargetInfo.Id = md.info.targetId
		path.TargetInfo.ModeInfoIdx = targetModeIdx
		path.TargetInfo.OutputTechnology = md.info.outputTech
		path.TargetInfo.Rotation = md.info.rotation
		path.TargetInfo.Scaling = md.info.scaling
		path.TargetInfo.RefreshRate = md.info.refreshRate // Note: Windows API uses rational, we might need conversion if we were setting it from layoutMon, but here we use md.info which comes from current config query.
		path.TargetInfo.ScanLineOrdering = md.info.scanLineOrder
		path.TargetInfo.TargetAvailable = 1 // TRUE
		path.TargetInfo.StatusFlags = DISPLAYCONFIG_TARGET_IN_USE

		path.Flags = DISPLAYCONFIG_PATH_ACTIVE

		paths = append(paths, path)
	}

	if len(paths) == 0 {
		return errors.New("no matching monitors found for layout")
	}

	// Combine modes: target modes first, then source modes (matching Windows active config order)
	modes := append(targetModes, sourceModes...)

	// Apply the configuration
	flags := uint32(SDC_APPLY | SDC_USE_SUPPLIED_DISPLAY_CONFIG | SDC_SAVE_TO_DATABASE)
	ret, _, _ := procSetDisplayConfig.Call(
		uintptr(len(paths)),
		uintptr(unsafe.Pointer(&paths[0])),
		uintptr(len(modes)),
		uintptr(unsafe.Pointer(&modes[0])),
		uintptr(flags),
	)

	if ret != 0 {
		return fmt.Errorf("SetDisplayConfig failed: %d", ret)
	}

	return nil
}
