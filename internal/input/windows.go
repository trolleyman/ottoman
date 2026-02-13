//go:build windows

package input

import (
	"runtime"
	"strings"
	"unicode/utf16"
	"unsafe"

	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

var (
	user32                              = windows.NewLazySystemDLL("user32.dll")
	procSetCursorPos                    = user32.NewProc("SetCursorPos")
	procGetCursorPos                    = user32.NewProc("GetCursorPos")
	procSendInput                       = user32.NewProc("SendInput")
	procSetProcessDpiAwarenessContext   = user32.NewProc("SetProcessDpiAwarenessContext")
)

type point struct {
	X, Y int32
}

const (
	inputMouse    = 0
	inputKeyboard = 1
)

// Mouse event flags
const (
	mouseEventFLeftDown   = 0x0002
	mouseEventFLeftUp     = 0x0004
	mouseEventFRightDown  = 0x0008
	mouseEventFRightUp    = 0x0010
	mouseEventFMiddleDown = 0x0020
	mouseEventFMiddleUp   = 0x0040
	mouseEventFXDown      = 0x0080
	mouseEventFXUp        = 0x0100
	mouseEventFWheel      = 0x0800
	mouseEventFHWheel     = 0x1000
)

// Keyboard event flags
const (
	keyEventFExtendedKey = 0x0001
	keyEventFKeyUp       = 0x0002
	keyEventFUnicode     = 0x0004
)

// XBUTTON values
const (
	xButton1 = 1 // Back
	xButton2 = 2 // Forward
)

// DPI_AWARENESS_CONTEXT values
const (
	dpiAwarenessContextPerMonitorAwareV2 = ^uintptr(3) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = -4
)

type keybdInput struct {
	wVk         uint16
	wScan       uint16
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

type mouseEvent struct {
	flags     uint32
	mouseData int32
}

// InitPlatform sets per-monitor DPI awareness on Windows.
// Must be called before any display/cursor operations.
func InitPlatform() {
	if err := procSetProcessDpiAwarenessContext.Find(); err != nil {
		// API not available (older Windows); skip
		return
	}
	procSetProcessDpiAwarenessContext.Call(dpiAwarenessContextPerMonitorAwareV2)
	// Ignore errors — may already be set by manifest or prior call
}

// WindowsMouse controls the OS cursor via user32.dll.
type WindowsMouse struct {
	fracX, fracY       float64
	scrollFracX        float64
	scrollFracY        float64
}

// NewMouseController creates a platform-specific mouse controller.
func NewMouseController() (MouseController, error) {
	if err := procSetCursorPos.Find(); err != nil {
		return nil, errors.Wrap(err, "SetCursorPos not available")
	}
	if err := procGetCursorPos.Find(); err != nil {
		return nil, errors.Wrap(err, "GetCursorPos not available")
	}
	if err := procSendInput.Find(); err != nil {
		return nil, errors.Wrap(err, "SendInput not available")
	}
	return &WindowsMouse{}, nil
}

func (m *WindowsMouse) MoveTo(x, y int) error {
	ret, _, err := procSetCursorPos.Call(uintptr(x), uintptr(y))
	if ret == 0 {
		return errors.Wrap(err, "SetCursorPos failed")
	}
	return nil
}

func (m *WindowsMouse) GetPosition() (int, int, error) {
	var pt point
	ret, _, err := procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	if ret == 0 {
		return 0, 0, errors.Wrap(err, "GetCursorPos failed")
	}
	return int(pt.X), int(pt.Y), nil
}

func (m *WindowsMouse) MoveRelative(dx, dy float64) error {
	m.fracX += dx
	m.fracY += dy

	intX := int(m.fracX)
	intY := int(m.fracY)

	if intX == 0 && intY == 0 {
		return nil
	}

	m.fracX -= float64(intX)
	m.fracY -= float64(intY)

	curX, curY, err := m.GetPosition()
	if err != nil {
		return err
	}
	return m.MoveTo(curX+intX, curY+intY)
}

// buttonFlags returns the down and up flags for a given mouse button, plus mouseData for XButtons.
func buttonFlags(btn MouseButton) (down, up uint32, mouseData int32) {
	switch btn {
	case MouseButtonLeft:
		return mouseEventFLeftDown, mouseEventFLeftUp, 0
	case MouseButtonRight:
		return mouseEventFRightDown, mouseEventFRightUp, 0
	case MouseButtonMiddle:
		return mouseEventFMiddleDown, mouseEventFMiddleUp, 0
	case MouseButtonBack:
		return mouseEventFXDown, mouseEventFXUp, xButton1
	case MouseButtonForward:
		return mouseEventFXDown, mouseEventFXUp, xButton2
	default:
		return mouseEventFLeftDown, mouseEventFLeftUp, 0
	}
}

func (m *WindowsMouse) Click(btn MouseButton) error {
	downFlag, upFlag, data := buttonFlags(btn)
	return m.sendMouseEvents([]mouseEvent{
		{flags: downFlag, mouseData: data},
		{flags: upFlag, mouseData: data},
	})
}

func (m *WindowsMouse) ButtonDown(btn MouseButton) error {
	downFlag, _, data := buttonFlags(btn)
	return m.sendMouseEvents([]mouseEvent{
		{flags: downFlag, mouseData: data},
	})
}

func (m *WindowsMouse) ButtonUp(btn MouseButton) error {
	_, upFlag, data := buttonFlags(btn)
	return m.sendMouseEvents([]mouseEvent{
		{flags: upFlag, mouseData: data},
	})
}

func (m *WindowsMouse) Scroll(dx, dy int, precise bool) error {
	var events []mouseEvent

	if precise {
		// Pixel-precise scrolling (trackpads): accumulate fractional wheel ticks
		// Use a smaller threshold for smoother scrolling
		const pixelsPerTick = 30
		m.scrollFracY += float64(-dy)
		m.scrollFracX += float64(dx)

		ticksY := int(m.scrollFracY / pixelsPerTick)
		if ticksY != 0 {
			m.scrollFracY -= float64(ticksY * pixelsPerTick)
			events = append(events, mouseEvent{
				flags:     mouseEventFWheel,
				mouseData: int32(ticksY) * 120,
			})
		}

		ticksX := int(m.scrollFracX / pixelsPerTick)
		if ticksX != 0 {
			m.scrollFracX -= float64(ticksX * pixelsPerTick)
			events = append(events, mouseEvent{
				flags:     mouseEventFHWheel,
				mouseData: int32(ticksX) * 120,
			})
		}
	} else {
		// Line-based scrolling (mouse wheels): each unit = one line = WHEEL_DELTA
		if dy != 0 {
			events = append(events, mouseEvent{
				flags:     mouseEventFWheel,
				mouseData: int32(-dy) * 120,
			})
		}
		if dx != 0 {
			events = append(events, mouseEvent{
				flags:     mouseEventFHWheel,
				mouseData: int32(dx) * 120,
			})
		}
	}

	if len(events) == 0 {
		return nil
	}
	return m.sendMouseEvents(events)
}

func (m *WindowsMouse) sendMouseEvents(events []mouseEvent) error {
	size, unionOffset := getInputLayout()
	count := len(events)
	buffer := make([]byte, size*count)

	for i, ev := range events {
		*(*uint32)(unsafe.Pointer(&buffer[i*size])) = inputMouse
		// MOUSEINPUT: dx(4) dy(4) mouseData(4) dwFlags(4) time(4) dwExtraInfo(ptr)
		// mouseData at union offset + 8
		*(*int32)(unsafe.Pointer(&buffer[i*size+unionOffset+8])) = ev.mouseData
		// dwFlags at union offset + 12
		*(*uint32)(unsafe.Pointer(&buffer[i*size+unionOffset+12])) = ev.flags
	}

	ret, _, err := procSendInput.Call(
		uintptr(count),
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(size),
	)
	if ret != uintptr(count) {
		return errors.Wrap(err, "SendInput failed")
	}
	return nil
}

// WindowsKeyboard controls keyboard input via user32.dll SendInput.
type WindowsKeyboard struct{}

// NewKeyboardController creates a platform-specific keyboard controller.
func NewKeyboardController() (KeyboardController, error) {
	if err := procSendInput.Find(); err != nil {
		return nil, errors.Wrap(err, "SendInput not available")
	}
	return &WindowsKeyboard{}, nil
}

func (k *WindowsKeyboard) Type(text string) error {
	utf16Chars := utf16.Encode([]rune(text))
	size, unionOffset := getInputLayout()

	for _, char := range utf16Chars {
		buffer := make([]byte, size*2)

		// Key Down
		*(*uint32)(unsafe.Pointer(&buffer[0])) = inputKeyboard
		kiDown := (*keybdInput)(unsafe.Pointer(&buffer[unionOffset]))
		kiDown.wScan = char
		kiDown.dwFlags = keyEventFUnicode

		// Key Up
		*(*uint32)(unsafe.Pointer(&buffer[size])) = inputKeyboard
		kiUp := (*keybdInput)(unsafe.Pointer(&buffer[size+unionOffset]))
		kiUp.wScan = char
		kiUp.dwFlags = keyEventFUnicode | keyEventFKeyUp

		ret, _, err := procSendInput.Call(
			uintptr(2),
			uintptr(unsafe.Pointer(&buffer[0])),
			uintptr(size),
		)
		if ret != 2 {
			return errors.Wrap(err, "SendInput failed")
		}
	}
	return nil
}

// Browser event.key name -> Windows VK code mapping
type vkMapping struct {
	vk       uint16
	extended bool
}

var browserKeyToVK = map[string]vkMapping{
	"ArrowUp":    {0x26, true},
	"ArrowDown":  {0x28, true},
	"ArrowLeft":  {0x25, true},
	"ArrowRight": {0x27, true},
	"Enter":      {0x0D, false},
	"Tab":        {0x09, false},
	"Escape":     {0x1B, false},
	"Backspace":  {0x08, false},
	"Delete":     {0x2E, true},
	"Home":       {0x24, true},
	"End":        {0x23, true},
	"PageUp":     {0x21, true},
	"PageDown":   {0x22, true},
	"Insert":     {0x2D, true},
	" ":          {0x20, false},
	"F1":         {0x70, false},
	"F2":         {0x71, false},
	"F3":         {0x72, false},
	"F4":         {0x73, false},
	"F5":         {0x74, false},
	"F6":         {0x75, false},
	"F7":         {0x76, false},
	"F8":         {0x77, false},
	"F9":         {0x78, false},
	"F10":        {0x79, false},
	"F11":        {0x7A, false},
	"F12":        {0x7B, false},
	"PrintScreen": {0x2C, false},
	"ScrollLock":  {0x91, false},
	"Pause":       {0x13, false},
	"NumLock":     {0x90, true},
	"CapsLock":    {0x14, false},
}

var modifierVK = map[string]uint16{
	"shift": 0x10, // VK_SHIFT
	"ctrl":  0x11, // VK_CONTROL
	"alt":   0x12, // VK_MENU
	"meta":  0x5B, // VK_LWIN
}

func (k *WindowsKeyboard) KeyPress(key string, modifiers []string) error {
	mapping, ok := browserKeyToVK[key]
	if !ok {
		// Try single character -> VK code
		runes := []rune(key)
		if len(runes) == 1 {
			r := runes[0]
			switch {
			case r >= 'a' && r <= 'z':
				mapping = vkMapping{vk: uint16(r - 'a' + 0x41), extended: false}
				ok = true
			case r >= 'A' && r <= 'Z':
				mapping = vkMapping{vk: uint16(r - 'A' + 0x41), extended: false}
				ok = true
			case r >= '0' && r <= '9':
				mapping = vkMapping{vk: uint16(r), extended: false}
				ok = true
			default:
				// For unknown single chars with modifiers, try using the char code directly
				if len(modifiers) > 0 {
					mapping = vkMapping{vk: uint16(r), extended: false}
					ok = true
				} else {
					// No modifiers, just type the character
					return k.Type(key)
				}
			}
		}
		if !ok {
			return nil // Unknown key, skip
		}
	}

	size, unionOffset := getInputLayout()

	// Count valid modifiers
	validMods := make([]uint16, 0, len(modifiers))
	for _, mod := range modifiers {
		if vk, exists := modifierVK[strings.ToLower(mod)]; exists {
			validMods = append(validMods, vk)
		}
	}

	// Build event list: modifier downs, key down, key up, modifier ups
	totalEvents := 2 + len(validMods)*2
	buffer := make([]byte, size*totalEvents)
	idx := 0

	// Modifier key downs
	for _, vk := range validMods {
		*(*uint32)(unsafe.Pointer(&buffer[idx*size])) = inputKeyboard
		ki := (*keybdInput)(unsafe.Pointer(&buffer[idx*size+unionOffset]))
		ki.wVk = vk
		idx++
	}

	// Key down
	*(*uint32)(unsafe.Pointer(&buffer[idx*size])) = inputKeyboard
	ki := (*keybdInput)(unsafe.Pointer(&buffer[idx*size+unionOffset]))
	ki.wVk = mapping.vk
	if mapping.extended {
		ki.dwFlags = keyEventFExtendedKey
	}
	idx++

	// Key up
	*(*uint32)(unsafe.Pointer(&buffer[idx*size])) = inputKeyboard
	ki = (*keybdInput)(unsafe.Pointer(&buffer[idx*size+unionOffset]))
	ki.wVk = mapping.vk
	ki.dwFlags = keyEventFKeyUp
	if mapping.extended {
		ki.dwFlags |= keyEventFExtendedKey
	}
	idx++

	// Modifier key ups (reverse order)
	for i := len(validMods) - 1; i >= 0; i-- {
		*(*uint32)(unsafe.Pointer(&buffer[idx*size])) = inputKeyboard
		ki := (*keybdInput)(unsafe.Pointer(&buffer[idx*size+unionOffset]))
		ki.wVk = validMods[i]
		ki.dwFlags = keyEventFKeyUp
		idx++
	}

	ret, _, err := procSendInput.Call(
		uintptr(idx),
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(size),
	)
	if ret != uintptr(idx) {
		return errors.Wrap(err, "SendInput KeyPress failed")
	}
	return nil
}

func getInputLayout() (size int, unionOffset int) {
	if runtime.GOARCH == "amd64" {
		return 40, 8
	}
	return 28, 4
}
