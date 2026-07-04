//go:build linux

package input

// evdev key codes (subset of linux/input-event-codes.h) needed to type text and
// handle the special keys the trackpad UI sends. Codes are positional scancodes,
// not alphabetical.
const (
	keyEsc        = 1
	keyBackspace  = 14
	keyTab        = 15
	keyEnter      = 28
	keyLeftCtrl   = 29
	keyLeftShift  = 42
	keyLeftAlt    = 56
	keySpace      = 57
	keyCapsLock   = 58
	keyF1         = 59
	keyNumLock    = 69
	keyScrollLock = 70
	keyF11        = 87
	keyF12        = 88
	keySysrq      = 99
	keyRightCtrl  = 97
	keyRightAlt   = 100
	keyHome       = 102
	keyUp         = 103
	keyPageUp     = 104
	keyLeft       = 105
	keyRight      = 106
	keyEnd        = 107
	keyDown       = 108
	keyPageDown   = 109
	keyInsert     = 110
	keyDelete     = 111
	keyPause      = 119
	keyLeftMeta   = 125
)

// charToKey maps a printable ASCII character to its US-QWERTY base scancode.
// Shift for the shifted glyph is supplied out of band via modifier keys (the
// browser reports Shift alongside capitals/symbols), so both a character and its
// shifted variant map to the same physical key.
var charToKey = map[rune]uint16{
	// letters
	'a': 30, 'b': 48, 'c': 46, 'd': 32, 'e': 18, 'f': 33, 'g': 34, 'h': 35,
	'i': 23, 'j': 36, 'k': 37, 'l': 38, 'm': 50, 'n': 49, 'o': 24, 'p': 25,
	'q': 16, 'r': 19, 's': 31, 't': 20, 'u': 22, 'v': 47, 'w': 17, 'x': 45,
	'y': 21, 'z': 44,
	// digits and their shifted symbols
	'1': 2, '!': 2, '2': 3, '@': 3, '3': 4, '#': 4, '4': 5, '$': 5,
	'5': 6, '%': 6, '6': 7, '^': 7, '7': 8, '&': 8, '8': 9, '*': 9,
	'9': 10, '(': 10, '0': 11, ')': 11,
	// punctuation
	'-': 12, '_': 12, '=': 13, '+': 13,
	'[': 26, '{': 26, ']': 27, '}': 27, '\\': 43, '|': 43,
	';': 39, ':': 39, '\'': 40, '"': 40, '`': 41, '~': 41,
	',': 51, '<': 51, '.': 52, '>': 52, '/': 53, '?': 53,
	' ': keySpace,
}

// namedKeys maps browser event.key names for non-character keys to scancodes.
var namedKeys = map[string]uint16{
	"Enter":       keyEnter,
	"Tab":         keyTab,
	"Backspace":   keyBackspace,
	"Delete":      keyDelete,
	"Escape":      keyEsc,
	"ArrowUp":     keyUp,
	"ArrowDown":   keyDown,
	"ArrowLeft":   keyLeft,
	"ArrowRight":  keyRight,
	"Home":        keyHome,
	"End":         keyEnd,
	"PageUp":      keyPageUp,
	"PageDown":    keyPageDown,
	"Insert":      keyInsert,
	" ":           keySpace,
	"CapsLock":    keyCapsLock,
	"NumLock":     keyNumLock,
	"ScrollLock":  keyScrollLock,
	"Pause":       keyPause,
	"PrintScreen": keySysrq,
	"F1":          keyF1,
	"F2":          keyF1 + 1,
	"F3":          keyF1 + 2,
	"F4":          keyF1 + 3,
	"F5":          keyF1 + 4,
	"F6":          keyF1 + 5,
	"F7":          keyF1 + 6,
	"F8":          keyF1 + 7,
	"F9":          keyF1 + 8,
	"F10":         keyF1 + 9,
	"F11":         keyF11,
	"F12":         keyF12,
	// Modifier keys can arrive as their own key events.
	"Shift":   keyLeftShift,
	"Control": keyLeftCtrl,
	"Alt":     keyLeftAlt,
	"Meta":    keyLeftMeta,
}

// resolveKeyCode maps a browser event.key value to an evdev scancode.
func resolveKeyCode(key string) (uint16, bool) {
	if code, ok := namedKeys[key]; ok {
		return code, true
	}
	r := []rune(key)
	if len(r) == 1 {
		return charToKeyCode(r[0])
	}
	return 0, false
}

func charToKeyCode(r rune) (uint16, bool) {
	// Normalise uppercase letters to their lowercase key (shift is external).
	if r >= 'A' && r <= 'Z' {
		r = r - 'A' + 'a'
	}
	code, ok := charToKey[r]
	return code, ok
}

// modifierKeyCode maps a modifier name (as sent in the message's modifier list)
// to a scancode.
func modifierKeyCode(mod string) (uint16, bool) {
	switch mod {
	case "shift", "Shift":
		return keyLeftShift, true
	case "ctrl", "control", "Control", "Ctrl":
		return keyLeftCtrl, true
	case "alt", "Alt":
		return keyLeftAlt, true
	case "meta", "Meta", "super", "Super":
		return keyLeftMeta, true
	}
	return 0, false
}

// allKeyCodes returns every scancode the keyboard device must enable via
// UI_SET_KEYBIT.
func allKeyCodes() []uintptr {
	seen := make(map[uint16]bool)
	var codes []uintptr
	add := func(c uint16) {
		if !seen[c] {
			seen[c] = true
			codes = append(codes, uintptr(c))
		}
	}
	for _, c := range charToKey {
		add(c)
	}
	for _, c := range namedKeys {
		add(c)
	}
	// Modifiers used for out-of-band shifting / combos.
	for _, c := range []uint16{keyLeftShift, keyLeftCtrl, keyLeftAlt, keyLeftMeta, keyRightCtrl, keyRightAlt} {
		add(c)
	}
	return codes
}
