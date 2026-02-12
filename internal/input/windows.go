//go:build windows

package input

import (
	"runtime"
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
	inputMouse    = 0
	inputKeyboard = 1
)

const (
	mouseEventFLeftDown = 0x0002
	mouseEventFLeftUp   = 0x0004
)

const (
	keyEventFKeyUp   = 0x0002
	keyEventFUnicode = 0x0004
)

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
	// Use SendInput for mouse click as well for consistency
	return m.sendMouseInput(mouseEventFLeftDown | mouseEventFLeftUp)
}

func (m *WindowsMouse) LeftDown() error {
	return m.sendMouseInput(mouseEventFLeftDown)
}

func (m *WindowsMouse) LeftUp() error {
	return m.sendMouseInput(mouseEventFLeftUp)
}

func (m *WindowsMouse) sendMouseInput(flags uint32) error {
	size, unionOffset := getInputLayout()
	buffer := make([]byte, size*2) // Enough for up to 2 events, though we might only use 1 or 2

	// We'll send Down and Up separately if flags has both, or together?
	// mouse_event allows ORing, SendInput usually expects separate events for clarity,
	// but let's just send two events if it's a click (Down | Up).

	count := 0
	if flags&mouseEventFLeftDown != 0 {
		*(*uint32)(unsafe.Pointer(&buffer[count*size])) = inputMouse
		// Offset to union (MOUSEINPUT)
		// MOUSEINPUT: dx, dy, mouseData, dwFlags, time, dwExtraInfo
		// We only care about dwFlags at offset 12 (3 * 4 bytes) inside the union
		*(*uint32)(unsafe.Pointer(&buffer[count*size+unionOffset+12])) = mouseEventFLeftDown
		count++
	}
	if flags&mouseEventFLeftUp != 0 {
		*(*uint32)(unsafe.Pointer(&buffer[count*size])) = inputMouse
		*(*uint32)(unsafe.Pointer(&buffer[count*size+unionOffset+12])) = mouseEventFLeftUp
		count++
	}

	if count > 0 {
		ret, _, err := procSendInput.Call(
			uintptr(count),
			uintptr(unsafe.Pointer(&buffer[0])),
			uintptr(size),
		)
		if ret != uintptr(count) {
			return errors.Wrap(err, "SendInput failed")
		}
	}
	return nil
}

func (m *WindowsMouse) Type(text string) error {
	// Use SendInput for Unicode support
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

func getInputLayout() (size int, unionOffset int) {
	if runtime.GOARCH == "amd64" {
		return 40, 8
	}
	return 28, 4
}
