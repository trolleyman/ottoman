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

func privateTextBus(t *testing.T) (*dbus.Conn, context.Context, string) {
	t.Helper()
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
	t.Cleanup(func() { server.Close() })
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return server, ctx, address
}

func exportTextObject(t *testing.T, server *dbus.Conn, v interface{}, path dbus.ObjectPath, iface string) {
	t.Helper()
	if err := server.Export(v, path, iface); err != nil {
		t.Fatal(err)
	}
}

func TestNativeATSPIFocusedTextAndProtectedField(t *testing.T) {
	server, ctx, _ := privateTextBus(t)
	export := func(v interface{}, path dbus.ObjectPath, iface string) { exportTextObject(t, server, v, path, iface) }
	root := &fakeAccessible{root: true}
	field := &fakeAccessible{value: "ab😀cd", start: 3, end: 3}
	export(root, "/org/a11y/atspi/accessible/root", accessible)
	export(root, "/org/a11y/atspi/registry", "org.a11y.atspi.Registry")
	for _, iface := range []string{accessible, textInterface, "org.a11y.atspi.EditableText", "org.freedesktop.DBus.Properties"} {
		export(field, "/field", iface)
	}
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
	if err != nil || !protected.Protected || !protected.Password || protected.Value != "" || protected.MaskedLength == nil || *protected.MaskedLength != 4096 {
		t.Fatal("protected field was mirrored")
	}
	field.mu.Lock()
	field.value = "abc"
	field.start = 0
	field.end = 3
	field.mu.Unlock()
	changed, err := backend.Snapshot(ctx, 128)
	if err != nil || changed.MaskedLength == nil || *changed.MaskedLength != 3 || changed.Start != 0 || changed.End != 0 {
		t.Fatal("password count change or selection redaction failed")
	}
	field.mu.Lock()
	defer field.mu.Unlock()
	if field.reads != reads {
		t.Fatal("password field text was read")
	}
}

type fakeRegistry struct {
	fakeAccessible
	children []reference
}

func (r *fakeRegistry) GetChildren() ([]reference, *dbus.Error) { return r.children, nil }

type testRelation struct {
	Kind    uint32
	Targets []reference
}
type lazyChrome struct {
	mu     sync.Mutex
	active bool
	bus    string
}

func (*lazyChrome) GetState() ([]uint32, *dbus.Error) { return []uint32{0, 0}, nil }
func (c *lazyChrome) GetChildren() ([]reference, *dbus.Error) {
	return []reference{{c.bus, "/frame"}}, nil
}
func (c *lazyChrome) GetRelationSet() ([]testRelation, *dbus.Error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.active = true
	return nil, nil
}

type lazyChromeFrame struct{ app *lazyChrome }

func (*lazyChromeFrame) GetState() ([]uint32, *dbus.Error) { return []uint32{1 << 12, 0}, nil }
func (*lazyChromeFrame) GetRole() (uint32, *dbus.Error)    { return 23, nil }
func (f *lazyChromeFrame) GetChildren() ([]reference, *dbus.Error) {
	f.app.mu.Lock()
	defer f.app.mu.Unlock()
	if !f.app.active {
		return nil, nil
	}
	return []reference{{f.app.bus, "/field"}}, nil
}

func TestChromeAccessibilityActivationAndFocusedWindowRecovery(t *testing.T) {
	registry, ctx, address := privateTextBus(t)
	chrome, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	defer chrome.Close()
	bus := chrome.Names()[0]
	app := &lazyChrome{bus: bus}
	root := &fakeRegistry{fakeAccessible: fakeAccessible{root: true}, children: []reference{{bus, "/org/a11y/atspi/accessible/root"}}}
	exportTextObject(t, registry, root, "/org/a11y/atspi/accessible/root", accessible)
	exportTextObject(t, registry, root, "/org/a11y/atspi/registry", "org.a11y.atspi.Registry")
	exportTextObject(t, chrome, app, "/org/a11y/atspi/accessible/root", accessible)
	exportTextObject(t, chrome, &lazyChromeFrame{app: app}, "/frame", accessible)
	field := &fakeAccessible{value: "Chrome field", start: 6, end: 6}
	for _, iface := range []string{accessible, textInterface, "org.a11y.atspi.EditableText", "org.freedesktop.DBus.Properties"} {
		exportTextObject(t, chrome, field, "/field", iface)
	}
	backend, err := Native(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	app.mu.Lock()
	active := app.active
	app.mu.Unlock()
	if active {
		t.Fatal("activated renderer before a text snapshot was requested")
	}
	snapshot, err := backend.Snapshot(ctx, 128)
	if err != nil || !snapshot.Available || snapshot.Value != "Chrome field" {
		t.Fatalf("did not activate the lazy Chrome tree and find its editable child: %v", err)
	}
	app.mu.Lock()
	active = app.active
	app.mu.Unlock()
	if !active {
		t.Fatal("no assistive-client activation request was made")
	}
	// A window focus signal must not pin the provider to a non-editable frame.
	provider := backend.(*atspi)
	provider.focused = reference{bus, "/frame"}
	if _, err = backend.Snapshot(ctx, 128); err != nil {
		t.Fatal(err)
	}
	recovered, err := backend.Snapshot(ctx, 128)
	if err != nil || !recovered.Available || recovered.Value != "Chrome field" {
		t.Fatal("window focus prevented later editable-field discovery")
	}
}

type fakeRichText struct {
	fakeAccessible
	links map[int32]int32
}

func (f *fakeRichText) GetLinkIndex(offset int32) (int32, *dbus.Error) {
	if f.links != nil {
		return f.links[offset], nil
	}
	return offset / 2, nil
}
func (*fakeRichText) GetLink(index int32) (reference, *dbus.Error) {
	if index == 0 {
		return reference{"org.ottoman.Test", "/link1"}, nil
	}
	return reference{"org.ottoman.Test", "/link2"}, nil
}

type fakeTextLink struct{ target reference }

func (f *fakeTextLink) GetObject(int32) (reference, *dbus.Error) { return f.target, nil }

func TestNativeRichEditorPreview(t *testing.T) {
	server, ctx, _ := privateTextBus(t)
	root := &fakeAccessible{root: true}
	editor := &fakeRichText{fakeAccessible: fakeAccessible{value: "\ufffc\n\ufffc", start: 2, end: 2}}
	first := &fakeAccessible{role: 73, value: "Hello", start: 5, end: 5}
	second := &fakeAccessible{role: 73, value: "world × €", start: 2, end: 2}
	exportTextObject(t, server, root, "/org/a11y/atspi/accessible/root", accessible)
	exportTextObject(t, server, root, "/org/a11y/atspi/registry", "org.a11y.atspi.Registry")
	for path, object := range map[dbus.ObjectPath]interface{}{"/field": editor, "/first": first, "/second": second} {
		for _, iface := range []string{accessible, textInterface, "org.freedesktop.DBus.Properties"} {
			exportTextObject(t, server, object, path, iface)
		}
	}
	exportTextObject(t, server, editor, "/field", "org.a11y.atspi.Hypertext")
	exportTextObject(t, server, &fakeTextLink{reference{"org.ottoman.Test", "/first"}}, "/link1", "org.a11y.atspi.Hyperlink")
	exportTextObject(t, server, &fakeTextLink{reference{"org.ottoman.Test", "/second"}}, "/link2", "org.a11y.atspi.Hyperlink")
	backend, err := Native(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	snapshot, err := backend.Snapshot(ctx, 128)
	if err != nil || !snapshot.Available || snapshot.Value != "Hello\nworld × €" || snapshot.Start != 8 || snapshot.End != 8 || snapshot.Editable {
		t.Fatalf("rich editor was not expanded safely: %#v, %v", snapshot, err)
	}
	// Chrome may omit parent separators for adjacent embedded paragraphs.
	editor.mu.Lock()
	editor.value, editor.start, editor.end = "\ufffc\ufffc", 1, 1
	editor.links = map[int32]int32{0: 0, 1: 1}
	editor.mu.Unlock()
	snapshot, err = backend.Snapshot(ctx, 128)
	if err != nil || snapshot.Value != "Hello\nworld × €" || snapshot.Start != 8 || snapshot.End != 8 {
		t.Fatalf("paragraph newline lost: %#v, %v", snapshot, err)
	}
	first.mu.Lock()
	first.value, first.start, first.end = "", 0, 0
	first.mu.Unlock()
	snapshot, err = backend.Snapshot(ctx, 128)
	if err != nil || snapshot.Value != "\nworld × €" || snapshot.Start != 3 {
		t.Fatalf("empty paragraph lost: %#v, %v", snapshot, err)
	}
	editor.mu.Lock()
	editor.value, editor.start, editor.end = "\ufffc\ufffc\ufffc", 2, 2
	editor.links = map[int32]int32{0: 0, 1: 0, 2: 1}
	editor.mu.Unlock()
	snapshot, err = backend.Snapshot(ctx, 128)
	if err != nil || snapshot.Value != "\n\nworld × €" || snapshot.Start != 4 {
		t.Fatalf("consecutive empty paragraphs lost: %#v, %v", snapshot, err)
	}
	if err := backend.Replace(ctx, snapshot, Edit{Start: 0, End: 5, Text: "changed"}); err == nil {
		t.Fatal("expanded preview offsets must never be written to the editor")
	}
	second.mu.Lock()
	second.role = 40
	reads := second.reads
	second.mu.Unlock()
	snapshot, _ = backend.Snapshot(ctx, 128)
	if snapshot.Available || snapshot.Value != "" {
		t.Fatal("protected embedded text was exposed")
	}
	second.mu.Lock()
	defer second.mu.Unlock()
	if second.reads != reads {
		t.Fatal("read a protected embedded field")
	}
}
