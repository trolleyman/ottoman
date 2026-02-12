//go:build windows

package input

import (
	"unicode/utf16"
	"unsafe"

	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

var (
	user32           = windows.NewLazySystemDLL("user32.dll")
	procSetCursorPos = user32.NewProc("SetCursorPos")
	procGetCursorPos = user32.NewProc("GetCursorPos")
	procMouseEvent   = user32.NewProc("mouse_event")
	procSendInput    = user32.NewProc("SendInput")
)

type point struct {
	X, Y int32
}

// WindowsMouse controls the OS cursor via user32.dll.
type WindowsMouse struct {
	fracX, fracY float64
}

const (
	mouseEventFLeftDown = 0x0002
	mouseEventFLeftUp   = 0x0004
)

const (
	inputKeyboard    = 1
	keyEventFKeyUp   = 0x0002
	keyEventFUnicode = 0x0004
)

type input struct {
	type_ uint32
	// On 64-bit, the union is 32 bytes (max of MOUSEINPUT and KEYBDINPUT + padding)
	// We only need KEYBDINPUT for SendInput here.
	// KEYBDINPUT: wVk(2), wScan(2), dwFlags(4), time(4), dwExtraInfo(8) = 20 bytes.
	// But we need to match the C union size/alignment.
	// Let's use a byte array large enough to cover the union.
	// 32 bytes should be safe for x64 (40 bytes total struct size).
	padding [32]byte
}

type keybdInput struct {
	wVk         uint16
	wScan       uint16
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

// NewMouseController creates a platform-specific mouse controller.
func NewMouseController() (MouseController, error) {
	// Verify the procs are available
	if err := procSetCursorPos.Find(); err != nil {
		return nil, errors.Wrap(err, "SetCursorPos not available")
	}
	if err := procGetCursorPos.Find(); err != nil {
		return nil, errors.Wrap(err, "GetCursorPos not available")
	}
	if err := procMouseEvent.Find(); err != nil {
		return nil, errors.Wrap(err, "mouse_event not available")
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

func (m *WindowsMouse) LeftClick() error {
	// mouse_event is deprecated but simpler and sufficient for basic clicks
	procMouseEvent.Call(
		uintptr(mouseEventFLeftDown),
		0, 0, 0, 0,
	)
	procMouseEvent.Call(
		uintptr(mouseEventFLeftUp),
		0, 0, 0, 0,
	)
	return nil
}

func (m *WindowsMouse) Type(text string) error {
	// Use SendInput for Unicode support
	utf16Chars := utf16.Encode([]rune(text))

	for _, char := range utf16Chars {
		var inputs [2]input

		// Key Down
		inputs[0].type_ = inputKeyboard
		kiDown := (*keybdInput)(unsafe.Pointer(&inputs[0].padding[0]))
		kiDown.wScan = char
		kiDown.dwFlags = keyEventFUnicode

		// Key Up
		inputs[1].type_ = inputKeyboard
		kiUp := (*keybdInput)(unsafe.Pointer(&inputs[1].padding[0]))
		kiUp.wScan = char
		kiUp.dwFlags = keyEventFUnicode | keyEventFKeyUp

		// Send both events
		// We need to pass the size of the INPUT structure.
		// On 64-bit Go, unsafe.Sizeof(input{}) should be 40 (4 + 4 padding + 32).
		// On 32-bit Go, it might be smaller (4 + 28 = 32).
		// Let's rely on Go's struct layout.
		cbSize := unsafe.Sizeof(inputs[0])

		ret, _, err := procSendInput.Call(
			uintptr(2),
			uintptr(unsafe.Pointer(&inputs[0])),
			uintptr(cbSize),
		)
		if ret != 2 {
			return errors.Wrap(err, "SendInput failed")
		}
	}
	return nil
}
