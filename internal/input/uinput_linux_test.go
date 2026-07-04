//go:build linux

package input

import "testing"

// TestIoctlNumbers pins the computed uinput ioctl request numbers to the values
// the kernel expects (from linux/uinput.h with _IOC encoding on x86_64). If the
// _IOC math is wrong the device setup silently fails, so lock it down here.
func TestIoctlNumbers(t *testing.T) {
	cases := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"UI_SET_EVBIT", uiSetEvBit, 0x40045564},
		{"UI_SET_KEYBIT", uiSetKeyBit, 0x40045565},
		{"UI_SET_RELBIT", uiSetRelBit, 0x40045566},
		{"UI_DEV_CREATE", uiDevCreate, 0x5501},
		{"UI_DEV_DESTROY", uiDevDestroy, 0x5502},
		// UI_DEV_SETUP = _IOW('U', 3, sizeof(struct uinput_setup)=92)
		{"UI_DEV_SETUP", uiDevSetup, 0x405c5503},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %#x, want %#x", c.name, c.got, c.want)
		}
	}
}

func TestResolveKeyCode(t *testing.T) {
	cases := []struct {
		key  string
		want uint16
		ok   bool
	}{
		{"a", 30, true},
		{"A", 30, true}, // shift supplied out of band
		{"z", 44, true},
		{"1", 2, true},
		{"!", 2, true}, // same physical key as '1'
		{" ", keySpace, true},
		{"Enter", keyEnter, true},
		{"ArrowUp", keyUp, true},
		{"F5", keyF1 + 4, true},
		{"Shift", keyLeftShift, true},
		{"Unknown", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		got, ok := resolveKeyCode(c.key)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("resolveKeyCode(%q) = (%d,%v), want (%d,%v)", c.key, got, ok, c.want, c.ok)
		}
	}
}

func TestInputEventSize(t *testing.T) {
	// struct input_event must be 24 bytes on 64-bit Linux or writes are rejected.
	if sz := sizeofInputEvent(); sz != 24 {
		t.Fatalf("sizeof(inputEvent) = %d, want 24", sz)
	}
}

func TestUinputSetupSize(t *testing.T) {
	if sz := sizeofUinputSetup(); sz != 92 {
		t.Fatalf("sizeof(uinputSetup) = %d, want 92", sz)
	}
}
