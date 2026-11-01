//go:build linux

package input

import (
	"encoding/binary"
	"os"
	"testing"
	"time"
)

func TestTextKeyUsesRemoteBritishLayout(t *testing.T) {
	for _, test := range []struct {
		r     rune
		code  uint16
		shift bool
	}{
		{'"', 3, true}, {'@', 40, true}, {'#', 43, false}, {'£', 4, true},
		{'~', 43, true}, {'\\', 86, false}, {'|', 86, true}, {'\'', 40, false},
		{'A', 30, true}, {'a', 30, false}, {'?', 53, true},
	} {
		code, mods, ok := textKey(test.r, "gb")
		if !ok || code != test.code || (len(mods) > 0) != test.shift {
			t.Errorf("British %q: code=%d mods=%v ok=%v", test.r, code, mods, ok)
		}
	}
	for _, test := range []struct {
		r    rune
		code uint16
	}{{'"', 40}, {'@', 3}, {'#', 4}, {'\\', 43}} {
		code, _, ok := textKey(test.r, "us")
		if !ok || code != test.code {
			t.Errorf("US %q: code=%d ok=%v", test.r, code, ok)
		}
	}
	enabled := false
	for _, code := range allKeyCodes() {
		if code == 86 {
			enabled = true
		}
	}
	if !enabled {
		t.Fatal("ISO backslash key must be enabled on virtual keyboard")
	}
}

func TestUnicodeTextEmitsInputMethodSequence(t *testing.T) {
	for _, text := range []string{"€", "·", "•", "×", "😀"} {
		t.Run(text, func(t *testing.T) {
			file, err := os.CreateTemp(t.TempDir(), "events")
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			keyboard := &UinputKeyboard{dev: &uinputDevice{fd: int(file.Fd())}, layout: "gb", layoutRead: time.Now()}
			if err := keyboard.TypeText(text); err != nil {
				t.Fatal(err)
			}
			if _, err := file.Seek(0, 0); err != nil {
				t.Fatal(err)
			}
			var keys []inputEvent
			for {
				var event inputEvent
				if err := binary.Read(file, binary.LittleEndian, &event); err != nil {
					break
				}
				if event.Type == evKey {
					keys = append(keys, event)
				}
			}
			if len(keys) < 12 || keys[0].Code != keyLeftCtrl || keys[1].Code != keyLeftShift || keys[2].Code != 22 {
				t.Fatalf("missing Ctrl+Shift+U prefix: %#v", keys)
			}
			if keys[3].Value != 0 || keys[4].Value != 0 || keys[5].Value != 0 {
				t.Fatal("Unicode prefix left a key held")
			}
			digits := ""
			for _, event := range keys[6 : len(keys)-2] {
				if event.Value != 1 {
					continue
				}
				for _, candidate := range "0123456789abcdef" {
					code, _, _ := textKey(candidate, "gb")
					if code == event.Code {
						digits += string(candidate)
					}
				}
			}
			expected := map[string]string{"€": "20ac", "·": "b7", "•": "2022", "×": "d7", "😀": "1f600"}[text]
			if digits != expected {
				t.Fatalf("Unicode digits %q, want %q", digits, expected)
			}
			if keys[len(keys)-2].Code != keyEnter || keys[len(keys)-1].Code != keyEnter || keys[len(keys)-1].Value != 0 {
				t.Fatal("Unicode entry was not committed")
			}
		})
	}
}
