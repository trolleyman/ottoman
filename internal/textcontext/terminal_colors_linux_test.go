//go:build linux

package textcontext

import (
	"github.com/godbus/dbus/v5"
	"strings"
	"testing"
	"unicode/utf8"
)

type colourPart struct{ text, color string }
type byteColourRun struct {
	start, end int32
	color      string
}
type byteColourTerminal struct {
	fakeTerminal
	runs      []byteColourRun
	exclusive bool
}

func (f *byteColourTerminal) Get(iface, name string) (dbus.Variant, *dbus.Error) {
	if name == "CharacterCount" && !f.exclusive {
		return dbus.MakeVariant(int32(len(f.value))), nil
	}
	return f.fakeTerminal.Get(iface, name)
}
func (f *byteColourTerminal) GetText(start, end int32) (string, *dbus.Error) {
	chars := []rune(f.value)
	start = max(0, min(start, int32(len(chars))))
	end = max(start, min(end, int32(len(chars))))
	return string(chars[start:end]), nil
}
func (f *byteColourTerminal) GetAttributeRun(offset int32, _ bool) (map[string]string, int32, int32, *dbus.Error) {
	for _, run := range f.runs {
		if offset >= run.start && ((!f.exclusive && offset <= run.end) || (f.exclusive && offset < run.end)) {
			return map[string]string{"fg-color": run.color}, run.start, run.end, nil
		}
	}
	return nil, 0, 0, dbus.NewError("org.freedesktop.DBus.Error.InvalidArgs", nil)
}
func TestTerminalColourOffsetConventions(t *testing.T) {
	for _, tc := range []struct {
		name      string
		prefix    string
		parts     []colourPart
		window    int
		exclusive bool
	}{
		{"Unicode prompt", "", []colourPart{{"~/code/ottoman ", "0,52685,52685"}, {"on ", "57825,57825,57825"}, {"\ue0a0 main", "52685,0,52685"}, {" via ", "57825,57825,57825"}, {"🐹 v1.27.3", "0,52685,52685"}, {" took ", "57825,57825,57825"}, {"12s", "52685,52685,0"}, {"\n", "57825,57825,57825"}, {"❯ ", "0,52685,0"}, {"git ", "57825,57825,57825"}, {"push && ", "0,52685,52685"}, {"mage ", "57825,57825,57825"}, {"deployAll\n", "0,52685,52685"}}, 4096, false},
		{"window with Unicode prefix", strings.Repeat("output 🐹\n", 50), []colourPart{{"❯ ", "0,52685,0"}, {"echo ", "57825,57825,57825"}, {"'test'\n", "52685,52685,0"}}, 128, false},
		{"standard exclusive Unicode runs", "", []colourPart{{"🐹", "0,52685,52685"}, {"echo ", "57825,57825,57825"}, {"❯", "0,52685,0"}, {"x\n", "52685,52685,0"}}, 128, true},
		{"ASCII single-character runs", "", []colourPart{{"a", "0,52685,52685"}, {"b", "52685,0,52685"}, {"c", "0,52685,0"}, {"\n", "57825,57825,57825"}}, 128, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, ctx, _ := privateTextBus(t)
			parts := append([]colourPart{}, tc.parts...)
			if tc.prefix != "" {
				parts = append([]colourPart{{tc.prefix, "57825,57825,57825"}}, parts...)
			}
			field := &byteColourTerminal{exclusive: tc.exclusive, fakeTerminal: fakeTerminal{fakeAccessible: fakeAccessible{role: terminalRole}}}
			expected := map[int]string{}
			position := 0
			for _, part := range parts {
				begin := len(field.value)
				field.value += part.text
				end := len(field.value) - 1
				if tc.exclusive {
					begin = utf8.RuneCountInString(field.value[:begin])
					end = utf8.RuneCountInString(field.value)
				}
				field.runs = append(field.runs, byteColourRun{int32(begin), int32(end), part.color})
				color, _ := terminalColor(part.color)
				for range part.text {
					expected[position] = color
					position++
				}
			}
			field.start = int32(utf8.RuneCountInString(field.value) - 1)
			for _, entry := range []struct {
				path  dbus.ObjectPath
				iface string
				value interface{}
			}{
				{"/org/a11y/atspi/accessible/root", accessible, &fakeAccessible{root: true}},
				{"/org/a11y/atspi/registry", "org.a11y.atspi.Registry", &fakeAccessible{root: true}},
				{"/field", accessible, field}, {"/field", textInterface, field}, {"/field", "org.freedesktop.DBus.Properties", field},
			} {
				exportTextObject(t, server, entry.value, entry.path, entry.iface)
			}
			backend, err := Native(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer backend.Close()
			snapshot, err := backend.Snapshot(ctx, tc.window)
			if err != nil || !snapshot.Available {
				t.Fatalf("missing terminal snapshot: %+v %v", snapshot, err)
			}
			actual := map[int]string{}
			for _, run := range snapshot.TerminalColors {
				for offset := run.Start; offset < run.End; offset++ {
					actual[offset] = run.Color
				}
			}
			for offset := snapshot.WindowStart; offset < snapshot.WindowStart+utf8.RuneCountInString(snapshot.Value); offset++ {
				if actual[offset] != expected[offset] {
					t.Fatalf("colour at Unicode offset %d: got %q want %q; window start %d", offset, actual[offset], expected[offset], snapshot.WindowStart)
				}
			}
		})
	}
}
