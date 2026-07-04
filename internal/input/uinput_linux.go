//go:build linux

package input

import (
	"unsafe"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

// This file implements mouse and keyboard control via the Linux uinput kernel
// interface. Unlike xdotool it works on Wayland, X11, and the bare console,
// because it injects events at the evdev layer with no compositor cooperation.
//
// One-time host setup is required (the deploy step prints it): load the uinput
// module and grant the agent user write access to /dev/uinput (input group +
// udev rule).

// evdev event types.
const (
	evSyn = 0x00
	evKey = 0x01
	evRel = 0x02
)

// evdev relative axes.
const (
	relX          = 0x00
	relY          = 0x01
	relHWheel     = 0x06
	relWheel      = 0x08
	relWheelHiRes = 0x0b
	relHWheelHi   = 0x0c
)

// SYN_REPORT.
const synReport = 0x00

// Mouse buttons.
const (
	btnLeft   = 0x110
	btnRight  = 0x111
	btnMiddle = 0x112
	btnSide   = 0x113
	btnExtra  = 0x114
)

// One wheel notch expressed in REL_WHEEL_HI_RES units.
const hiResPerNotch = 120

// Approx pixels of precise scroll per wheel notch.
const scrollPixelsPerNotch = 15.0

// uinput ioctl request numbers (see linux/uinput.h). Computed via the _IOC
// encoding so the intent stays readable.
const iocTypeUinput = 'U'

func ioc(dir, typ, nr, size uintptr) uintptr {
	return (dir << 30) | (size << 16) | (typ << 8) | nr
}

var (
	uiSetEvBit   = ioc(1, iocTypeUinput, 100, 4)
	uiSetKeyBit  = ioc(1, iocTypeUinput, 101, 4)
	uiSetRelBit  = ioc(1, iocTypeUinput, 102, 4)
	uiDevSetup   = ioc(1, iocTypeUinput, 3, unsafe.Sizeof(uinputSetup{}))
	uiDevCreate  = ioc(0, iocTypeUinput, 1, 0)
	uiDevDestroy = ioc(0, iocTypeUinput, 2, 0)
)

type inputID struct {
	Bustype uint16
	Vendor  uint16
	Product uint16
	Version uint16
}

type uinputSetup struct {
	ID           inputID
	Name         [80]byte
	FfEffectsMax uint32
}

// inputEvent mirrors struct input_event on 64-bit Linux (24 bytes).
type inputEvent struct {
	Sec   int64
	Usec  int64
	Type  uint16
	Code  uint16
	Value int32
}

// uinputDevice is an open virtual input device.
type uinputDevice struct {
	fd int
}

// Size helpers exposed for tests (the kernel rejects wrongly sized structs).
func sizeofInputEvent() uintptr  { return unsafe.Sizeof(inputEvent{}) }
func sizeofUinputSetup() uintptr { return unsafe.Sizeof(uinputSetup{}) }

func ioctl(fd int, req uintptr, arg uintptr) error {
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), req, arg); errno != 0 {
		return errno
	}
	return nil
}

// openUinput creates a virtual device with the given name and enabled event
// bits. evBits are enabled first; then each code in the provided maps.
func openUinput(name string, evBits []uintptr, keyCodes, relCodes []uintptr) (*uinputDevice, error) {
	fd, err := unix.Open("/dev/uinput", unix.O_WRONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, errors.Wrap(err, "open /dev/uinput (is the uinput module loaded and is the user in the input group?)")
	}

	d := &uinputDevice{fd: fd}
	cleanup := func(e error) (*uinputDevice, error) {
		unix.Close(fd)
		return nil, e
	}

	for _, ev := range evBits {
		if err := ioctl(fd, uiSetEvBit, ev); err != nil {
			return cleanup(errors.Wrap(err, "UI_SET_EVBIT"))
		}
	}
	for _, code := range keyCodes {
		if err := ioctl(fd, uiSetKeyBit, code); err != nil {
			return cleanup(errors.Wrap(err, "UI_SET_KEYBIT"))
		}
	}
	for _, code := range relCodes {
		if err := ioctl(fd, uiSetRelBit, code); err != nil {
			return cleanup(errors.Wrap(err, "UI_SET_RELBIT"))
		}
	}

	setup := uinputSetup{ID: inputID{Bustype: 0x03 /* BUS_USB */, Vendor: 0x1209, Product: 0x0777, Version: 1}}
	copy(setup.Name[:], name)
	if err := ioctl(fd, uiDevSetup, uintptr(unsafe.Pointer(&setup))); err != nil {
		return cleanup(errors.Wrap(err, "UI_DEV_SETUP"))
	}
	if err := ioctl(fd, uiDevCreate, 0); err != nil {
		return cleanup(errors.Wrap(err, "UI_DEV_CREATE"))
	}

	return d, nil
}

func (d *uinputDevice) emit(typ, code uint16, value int32) error {
	ev := inputEvent{Type: typ, Code: code, Value: value}
	buf := (*[unsafe.Sizeof(inputEvent{})]byte)(unsafe.Pointer(&ev))[:]
	if _, err := unix.Write(d.fd, buf); err != nil {
		return errors.Wrap(err, "write input event")
	}
	return nil
}

func (d *uinputDevice) syn() error {
	return d.emit(evSyn, synReport, 0)
}

func (d *uinputDevice) close() error {
	_ = ioctl(d.fd, uiDevDestroy, 0)
	return unix.Close(d.fd)
}

// --- Mouse ---

// UinputMouse controls the cursor via a virtual uinput pointer device.
type UinputMouse struct {
	dev          *uinputDevice
	fracX, fracY float64
	hiResAccY    float64
	hiResAccX    float64
}

func newUinputMouse() (*UinputMouse, error) {
	dev, err := openUinput("ottoman-mouse",
		[]uintptr{evKey, evRel},
		[]uintptr{btnLeft, btnRight, btnMiddle, btnSide, btnExtra},
		[]uintptr{relX, relY, relWheel, relHWheel, relWheelHiRes, relHWheelHi},
	)
	if err != nil {
		return nil, err
	}
	return &UinputMouse{dev: dev}, nil
}

func uinputButton(btn MouseButton) uint16 {
	switch btn {
	case MouseButtonLeft:
		return btnLeft
	case MouseButtonMiddle:
		return btnMiddle
	case MouseButtonRight:
		return btnRight
	case MouseButtonBack:
		return btnSide
	case MouseButtonForward:
		return btnExtra
	default:
		return btnLeft
	}
}

// MoveTo is not supported by a relative uinput device (absolute positioning
// would need an ABS device or the RemoteDesktop portal). The trackpad protocol
// is relative-only, so this is a no-op that reports the limitation.
func (m *UinputMouse) MoveTo(x, y int) error {
	return errors.New("absolute cursor positioning is not supported by the uinput backend")
}

// GetPosition is not knowable via uinput. Callers treat the error as "position
// unavailable" and simply skip the cursor-position overlay.
func (m *UinputMouse) GetPosition() (int, int, error) {
	return 0, 0, errors.New("cursor position is not available from the uinput backend")
}

func (m *UinputMouse) MoveRelative(dx, dy float64) error {
	m.fracX += dx
	m.fracY += dy
	intX := int32(m.fracX)
	intY := int32(m.fracY)
	if intX == 0 && intY == 0 {
		return nil
	}
	m.fracX -= float64(intX)
	m.fracY -= float64(intY)

	if intX != 0 {
		if err := m.dev.emit(evRel, relX, intX); err != nil {
			return err
		}
	}
	if intY != 0 {
		if err := m.dev.emit(evRel, relY, intY); err != nil {
			return err
		}
	}
	return m.dev.syn()
}

func (m *UinputMouse) Click(btn MouseButton) error {
	if err := m.ButtonDown(btn); err != nil {
		return err
	}
	return m.ButtonUp(btn)
}

func (m *UinputMouse) ButtonDown(btn MouseButton) error {
	if err := m.dev.emit(evKey, uinputButton(btn), 1); err != nil {
		return err
	}
	return m.dev.syn()
}

func (m *UinputMouse) ButtonUp(btn MouseButton) error {
	if err := m.dev.emit(evKey, uinputButton(btn), 0); err != nil {
		return err
	}
	return m.dev.syn()
}

// Scroll emits wheel events. evdev wheel sign is inverted from the trackpad
// protocol on the vertical axis (positive REL_WHEEL = up, but positive dy =
// down), so vertical values are negated; horizontal matches (positive = right).
func (m *UinputMouse) Scroll(dx, dy int, precise bool) error {
	if precise {
		return m.scrollPrecise(float64(dx), float64(dy))
	}
	return m.scrollLines(dx, dy)
}

func (m *UinputMouse) scrollLines(dx, dy int) error {
	if dy != 0 {
		if err := m.dev.emit(evRel, relWheel, int32(-dy)); err != nil {
			return err
		}
		if err := m.dev.emit(evRel, relWheelHiRes, int32(-dy*hiResPerNotch)); err != nil {
			return err
		}
	}
	if dx != 0 {
		if err := m.dev.emit(evRel, relHWheel, int32(dx)); err != nil {
			return err
		}
		if err := m.dev.emit(evRel, relHWheelHi, int32(dx*hiResPerNotch)); err != nil {
			return err
		}
	}
	if dx == 0 && dy == 0 {
		return nil
	}
	return m.dev.syn()
}

// scrollPrecise turns pixel deltas into high-resolution wheel events, emitting a
// coarse REL_WHEEL notch whenever a full notch's worth of hi-res units builds
// up (for apps that ignore the hi-res axis).
func (m *UinputMouse) scrollPrecise(dx, dy float64) error {
	emitted := false

	hiY := -dy * (hiResPerNotch / scrollPixelsPerNotch)
	if hiY != 0 {
		if err := m.dev.emit(evRel, relWheelHiRes, int32(hiY)); err != nil {
			return err
		}
		m.hiResAccY += hiY
		if notch := int32(m.hiResAccY / hiResPerNotch); notch != 0 {
			m.hiResAccY -= float64(notch * hiResPerNotch)
			if err := m.dev.emit(evRel, relWheel, notch); err != nil {
				return err
			}
		}
		emitted = true
	}

	hiX := dx * (hiResPerNotch / scrollPixelsPerNotch)
	if hiX != 0 {
		if err := m.dev.emit(evRel, relHWheelHi, int32(hiX)); err != nil {
			return err
		}
		m.hiResAccX += hiX
		if notch := int32(m.hiResAccX / hiResPerNotch); notch != 0 {
			m.hiResAccX -= float64(notch * hiResPerNotch)
			if err := m.dev.emit(evRel, relHWheel, notch); err != nil {
				return err
			}
		}
		emitted = true
	}

	if !emitted {
		return nil
	}
	return m.dev.syn()
}

// --- Keyboard ---

// UinputKeyboard controls keyboard input via a virtual uinput key device.
type UinputKeyboard struct {
	dev *uinputDevice
}

func newUinputKeyboard() (*UinputKeyboard, error) {
	dev, err := openUinput("ottoman-keyboard",
		[]uintptr{evKey},
		allKeyCodes(),
		nil,
	)
	if err != nil {
		return nil, err
	}
	return &UinputKeyboard{dev: dev}, nil
}

func (k *UinputKeyboard) KeyDown(key string, modifiers []string) error {
	return k.emitKey(key, modifiers, 1)
}

func (k *UinputKeyboard) KeyUp(key string, modifiers []string) error {
	return k.emitKey(key, modifiers, 0)
}

// emitKey resolves the browser key name to an evdev code and emits a press or
// release. Shift for shifted characters is expected to be supplied out of band
// by the caller's modifier handling (matching the X11 path), so this only
// presses the base key plus any explicit modifiers passed in.
func (k *UinputKeyboard) emitKey(key string, modifiers []string, value int32) error {
	code, ok := resolveKeyCode(key)
	if !ok {
		return nil // unknown key: ignore rather than error
	}

	// Explicit modifiers (rarely used on the trackpad path, which manages
	// modifier keys separately). Press before / release after the key.
	var mods []uint16
	for _, mod := range modifiers {
		if c, ok := modifierKeyCode(mod); ok {
			mods = append(mods, c)
		}
	}

	if value == 1 {
		for _, c := range mods {
			if err := k.dev.emit(evKey, c, 1); err != nil {
				return err
			}
		}
	}
	if err := k.dev.emit(evKey, code, value); err != nil {
		return err
	}
	if value == 0 {
		for _, c := range mods {
			if err := k.dev.emit(evKey, c, 0); err != nil {
				return err
			}
		}
	}
	return k.dev.syn()
}
