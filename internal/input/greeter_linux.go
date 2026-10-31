//go:build linux

package input

// NewGreeterControllers uses kernel virtual devices exclusively. X11 helpers
// cannot control GDM's Wayland session and must not be selected as a fallback.
func NewGreeterControllers() (MouseController, KeyboardController, error) {
	mouse, err := newUinputMouse()
	if err != nil {
		return nil, nil, err
	}
	keyboard, err := newUinputKeyboard()
	if err != nil {
		mouse.dev.close()
		return nil, nil, err
	}
	return mouse, keyboard, nil
}
