//go:build linux

package textcontext

import (
	"bufio"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

type fakeAccessible struct {
	mu         sync.Mutex
	root       bool
	role       uint32
	value      string
	start, end int32
	reads      int
}

func (f *fakeAccessible) GetState() ([]uint32, *dbus.Error) {
	if f.root {
		return []uint32{0}, nil
	}
	return []uint32{1<<7 | 1<<8 | 1<<12 | 1<<17 | 1<<24}, nil
}
func (f *fakeAccessible) GetChildren() ([]reference, *dbus.Error) {
	if f.root {
		return []reference{{"org.ottoman.Test", "/field"}}, nil
	}
	return nil, nil
}
func (f *fakeAccessible) GetRole() (uint32, *dbus.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.role, nil
}
func (f *fakeAccessible) GetInterfaces() ([]string, *dbus.Error) {
	return []string{accessible, textInterface, "org.a11y.atspi.EditableText"}, nil
}
func (f *fakeAccessible) Get(_ string, name string) (dbus.Variant, *dbus.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if name == "CharacterCount" {
		return dbus.MakeVariant(int32(len([]rune(f.value)))), nil
	}
	return dbus.MakeVariant(f.start), nil
}
func (f *fakeAccessible) GetNSelections() (int32, *dbus.Error) { return 1, nil }
func (f *fakeAccessible) GetSelection(int32) (int32, int32, *dbus.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.start, f.end, nil
}
func (f *fakeAccessible) GetText(start, end int32) (string, *dbus.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	return string([]rune(f.value)[start:end]), nil
}
func (f *fakeAccessible) DeleteText(start, end int32) (bool, *dbus.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v := []rune(f.value)
	f.value = string(v[:start]) + string(v[end:])
	return true, nil
}
func (f *fakeAccessible) InsertText(start int32, text string, length int32) (bool, *dbus.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v := []rune(f.value)
	f.value = string(v[:start]) + text + string(v[start:])
	return length == int32(len(text)), nil
}
func (f *fakeAccessible) SetCaretOffset(offset int32) (bool, *dbus.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.start = offset
	f.end = offset
	return true, nil
}
func (*fakeAccessible) RegisterEvent(string, []string, string) *dbus.Error { return nil }

type fakeBus struct{ Address string }

func (b fakeBus) GetAddress() (string, *dbus.Error) { return b.Address, nil }
func (fakeBus) GetActive() (bool, *dbus.Error)      { return false, nil }

func TestNativeATSPIFocusedTextAndProtectedField(t *testing.T) {
	path, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("dbus-daemon is unavailable")
	}
	address := "unix:path=" + filepath.Join(t.TempDir(), "bus")
	cmd := exec.Command(path, "--session", "--nofork", "--address="+address, "--print-address=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	if _, err = bufio.NewReader(stdout).ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", address)
	server, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	for _, name := range []string{"org.a11y.Bus", "org.a11y.atspi.Registry", "org.ottoman.Test", "org.gnome.ScreenSaver"} {
		if _, err = server.RequestName(name, dbus.NameFlagDoNotQueue); err != nil {
			t.Fatal(err)
		}
	}
	export := func(v interface{}, path dbus.ObjectPath, iface string) {
		t.Helper()
		if err := server.Export(v, path, iface); err != nil {
			t.Fatal(err)
		}
	}
	export(fakeBus{address}, "/org/a11y/bus", "org.a11y.Bus")
	export(fakeBus{}, "/org/gnome/ScreenSaver", "org.gnome.ScreenSaver")
	root := &fakeAccessible{root: true}
	field := &fakeAccessible{value: "ab😀cd", start: 3, end: 3}
	export(root, "/org/a11y/atspi/accessible/root", accessible)
	export(root, "/org/a11y/atspi/registry", "org.a11y.atspi.Registry")
	for _, iface := range []string{accessible, textInterface, "org.a11y.atspi.EditableText", "org.freedesktop.DBus.Properties"} {
		export(field, "/field", iface)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	backend, err := Native(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	first, err := backend.Snapshot(ctx, 128)
	if err != nil || !first.Available || first.Value != "ab😀cd" || first.Start != 3 {
		t.Fatalf("bad native snapshot: %#v, %v", first, err)
	}
	if err = backend.Replace(ctx, first, Edit{Start: 2, End: 3, Text: "👍"}); err != nil {
		t.Fatal(err)
	}
	next, err := backend.Snapshot(ctx, 128)
	if err != nil || next.Value != "ab👍cd" || next.Start != 3 {
		t.Fatalf("bad native replacement: %#v, %v", next, err)
	}
	field.mu.Lock()
	field.value = strings.Repeat("😀", 10000)
	field.start = 9000
	field.end = 9000
	field.mu.Unlock()
	bounded, err := backend.Snapshot(ctx, 128)
	if err != nil || len([]rune(bounded.Value)) != 128 || bounded.WindowStart != 8936 || bounded.Start != 9000 {
		t.Fatal("large context was not bounded around the caret")
	}
	field.mu.Lock()
	field.role = 40
	reads := field.reads
	field.mu.Unlock()
	protected, err := backend.Snapshot(ctx, 128)
	if err != nil || !protected.Protected || protected.Value != "" {
		t.Fatal("protected field was mirrored")
	}
	field.mu.Lock()
	defer field.mu.Unlock()
	if field.reads != reads {
		t.Fatal("password field text was read")
	}
}
