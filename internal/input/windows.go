//go:build windows

package input

import (
	"unsafe"

	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

var (
	user32          = windows.NewLazySystemDLL("user32.dll")
	procSetCursorPos = user32.NewProc("SetCursorPos")
	procGetCursorPos = user32.NewProc("GetCursorPos")
)

type point struct {
	X, Y int32
}

// WindowsMouse controls the OS cursor via user32.dll.
type WindowsMouse struct {
	fracX, fracY float64
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
