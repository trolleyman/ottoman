//go:build linux

package input

import (
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// InitPlatform is a no-op on Linux.
func InitPlatform() {}

// LinuxMouse controls the OS cursor via xdotool.
type LinuxMouse struct {
	fracX, fracY float64
	scrollFracX  float64
	scrollFracY  float64
}

// NewMouseController creates a platform-specific mouse controller. It prefers
// the uinput backend (works on Wayland, X11 and the console) and falls back to
// xdotool (X11 only) if /dev/uinput isn't usable.
func NewMouseController() (MouseController, error) {
	if mouse, err := newUinputMouse(); err == nil {
		log.Printf("Input backend (mouse): uinput")
		return mouse, nil
	} else {
		log.Printf("uinput mouse unavailable, falling back to xdotool: %v", err)
	}
	if _, err := exec.LookPath("xdotool"); err != nil {
		return nil, errors.Wrap(err, "no usable mouse backend: uinput unavailable and xdotool not found")
	}
	log.Printf("Input backend (mouse): xdotool")
	return &LinuxMouse{}, nil
}

func (m *LinuxMouse) MoveTo(x, y int) error {
	cmd := exec.Command("xdotool", "mousemove", strconv.Itoa(x), strconv.Itoa(y))
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrapf(err, "xdotool mousemove failed: %s", string(out))
	}
	return nil
}

func (m *LinuxMouse) GetPosition() (int, int, error) {
	cmd := exec.Command("xdotool", "getmouselocation", "--shell")
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, errors.Wrap(err, "xdotool getmouselocation failed")
	}

	var x, y int
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "X="); ok {
			x, _ = strconv.Atoi(v)
		} else if v, ok := strings.CutPrefix(line, "Y="); ok {
			y, _ = strconv.Atoi(v)
		}
	}
	return x, y, nil
}

func (m *LinuxMouse) MoveRelative(dx, dy float64) error {
	m.fracX += dx
	m.fracY += dy

	intX := int(m.fracX)
	intY := int(m.fracY)

	if intX == 0 && intY == 0 {
		return nil
	}

	m.fracX -= float64(intX)
	m.fracY -= float64(intY)

	// xdotool mousemove_relative -- dx dy (-- needed for negative values)
	cmd := exec.Command("xdotool", "mousemove_relative", "--",
		fmt.Sprintf("%d", intX), fmt.Sprintf("%d", intY))
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrapf(err, "xdotool mousemove_relative failed: %s", string(out))
	}
	return nil
}

// xdotoolButton maps MouseButton to xdotool button number.
func xdotoolButton(btn MouseButton) string {
	switch btn {
	case MouseButtonLeft:
		return "1"
	case MouseButtonMiddle:
		return "2"
	case MouseButtonRight:
		return "3"
	case MouseButtonBack:
		return "8"
	case MouseButtonForward:
		return "9"
	default:
		return "1"
	}
}

func (m *LinuxMouse) Click(btn MouseButton) error {
	if err := exec.Command("xdotool", "click", xdotoolButton(btn)).Run(); err != nil {
		return errors.Wrapf(err, "xdotool click %s failed", btn)
	}
	return nil
}

func (m *LinuxMouse) ButtonDown(btn MouseButton) error {
	if err := exec.Command("xdotool", "mousedown", xdotoolButton(btn)).Run(); err != nil {
		return errors.Wrapf(err, "xdotool mousedown %s failed", btn)
	}
	return nil
}

func (m *LinuxMouse) ButtonUp(btn MouseButton) error {
	if err := exec.Command("xdotool", "mouseup", xdotoolButton(btn)).Run(); err != nil {
		return errors.Wrapf(err, "xdotool mouseup %s failed", btn)
	}
	return nil
}

func (m *LinuxMouse) Scroll(dx, dy int, precise bool) error {
	if precise {
		// Pixel-precise scrolling: accumulate and convert to click events
		const pixelsPerClick = 30
		m.scrollFracY += float64(dy)
		m.scrollFracX += float64(dx)

		clicksY := int(m.scrollFracY / pixelsPerClick)
		if clicksY != 0 {
			m.scrollFracY -= float64(clicksY * pixelsPerClick)
			if clicksY > 0 {
				for i := 0; i < clicksY; i++ {
					exec.Command("xdotool", "click", "5").Run() // scroll down
				}
			} else {
				for i := 0; i < -clicksY; i++ {
					exec.Command("xdotool", "click", "4").Run() // scroll up
				}
			}
		}

		clicksX := int(m.scrollFracX / pixelsPerClick)
		if clicksX != 0 {
			m.scrollFracX -= float64(clicksX * pixelsPerClick)
			if clicksX > 0 {
				for i := 0; i < clicksX; i++ {
					exec.Command("xdotool", "click", "7").Run() // scroll right
				}
			} else {
				for i := 0; i < -clicksX; i++ {
					exec.Command("xdotool", "click", "6").Run() // scroll left
				}
			}
		}
	} else {
		// Line-based scrolling: each unit = one click event
		if dy > 0 {
			for i := 0; i < dy; i++ {
				exec.Command("xdotool", "click", "5").Run() // scroll down
			}
		} else if dy < 0 {
			for i := 0; i < -dy; i++ {
				exec.Command("xdotool", "click", "4").Run() // scroll up
			}
		}
		if dx > 0 {
			for i := 0; i < dx; i++ {
				exec.Command("xdotool", "click", "7").Run() // scroll right
			}
		} else if dx < 0 {
			for i := 0; i < -dx; i++ {
				exec.Command("xdotool", "click", "6").Run() // scroll left
			}
		}
	}
	return nil
}

// LinuxKeyboard controls keyboard input via xdotool.
type LinuxKeyboard struct{}

// NewKeyboardController creates a platform-specific keyboard controller. It
// prefers the uinput backend and falls back to xdotool (X11 only).
func NewKeyboardController() (KeyboardController, error) {
	if kb, err := newUinputKeyboard(); err == nil {
		log.Printf("Input backend (keyboard): uinput")
		return kb, nil
	} else {
		log.Printf("uinput keyboard unavailable, falling back to xdotool: %v", err)
	}
	if _, err := exec.LookPath("xdotool"); err != nil {
		return nil, errors.Wrap(err, "no usable keyboard backend: uinput unavailable and xdotool not found")
	}
	log.Printf("Input backend (keyboard): xdotool")
	return &LinuxKeyboard{}, nil
}

// Browser event.key name -> xdotool key name mapping
var browserKeyToXdotool = map[string]string{
	"ArrowUp":     "Up",
	"ArrowDown":   "Down",
	"ArrowLeft":   "Left",
	"ArrowRight":  "Right",
	"Enter":       "Return",
	"Tab":         "Tab",
	"Escape":      "Escape",
	"Backspace":   "BackSpace",
	"Delete":      "Delete",
	"Home":        "Home",
	"End":         "End",
	"PageUp":      "Prior",
	"PageDown":    "Next",
	"Insert":      "Insert",
	" ":           "space",
	"F1":          "F1",
	"F2":          "F2",
	"F3":          "F3",
	"F4":          "F4",
	"F5":          "F5",
	"F6":          "F6",
	"F7":          "F7",
	"F8":          "F8",
	"F9":          "F9",
	"F10":         "F10",
	"F11":         "F11",
	"F12":         "F12",
	"PrintScreen": "Print",
	"ScrollLock":  "Scroll_Lock",
	"Pause":       "Pause",
	"NumLock":     "Num_Lock",
	"CapsLock":    "Caps_Lock",
}

var modifierToXdotool = map[string]string{
	"shift": "shift",
	"ctrl":  "ctrl",
	"alt":   "alt",
	"meta":  "super",
}

func getXdoKey(key string) (string, bool) {
	xdoKey, ok := browserKeyToXdotool[key]
	if !ok {
		// Single character keys pass through directly
		if len([]rune(key)) == 1 {
			return key, true
		} else {
			return "", false
		}
	}
	return xdoKey, true
}

func buildCombo(key string, modifiers []string) string {
	parts := make([]string, 0, len(modifiers)+1)
	for _, mod := range modifiers {
		if xdoMod, exists := modifierToXdotool[strings.ToLower(mod)]; exists {
			parts = append(parts, xdoMod)
		}
	}
	parts = append(parts, key)
	return strings.Join(parts, "+")
}

func (k *LinuxKeyboard) KeyDown(key string, modifiers []string) error {
	xdoKey, ok := getXdoKey(key)
	if !ok {
		return nil
	}

	combo := buildCombo(xdoKey, modifiers)
	if err := exec.Command("xdotool", "keydown", combo).Run(); err != nil {
		return errors.Wrapf(err, "xdotool keydown %q failed", combo)
	}
	return nil
}

func (k *LinuxKeyboard) KeyUp(key string, modifiers []string) error {
	xdoKey, ok := getXdoKey(key)
	if !ok {
		return nil
	}

	combo := buildCombo(xdoKey, modifiers)
	if err := exec.Command("xdotool", "keyup", combo).Run(); err != nil {
		return errors.Wrapf(err, "xdotool keyup %q failed", combo)
	}
	return nil
}
