//go:build linux

package textcontext

import (
	"context"
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"
)

const accessible = "org.a11y.atspi.Accessible"
const textInterface = "org.a11y.atspi.Text"

type reference struct {
	Bus  string
	Path dbus.ObjectPath
}
type atspi struct {
	window       int
	session, bus *dbus.Conn
	signals      chan *dbus.Signal
	focused      reference
	epoch        uint64
	searched     time.Time
}

func Native(ctx context.Context) (Backend, error) {
	session, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	var address string
	err = session.Object("org.a11y.Bus", "/org/a11y/bus").CallWithContext(ctx, "org.a11y.Bus.GetAddress", 0).Store(&address)
	if err != nil {
		session.Close()
		return nil, err
	}
	bus, err := dbus.Connect(address)
	if err != nil {
		session.Close()
		return nil, err
	}
	a := &atspi{session: session, bus: bus, signals: make(chan *dbus.Signal, 256)}
	bus.Signal(a.signals)
	err = bus.AddMatchSignal(dbus.WithMatchInterface("org.a11y.atspi.Event.Object"), dbus.WithMatchMember("StateChanged"))
	if err == nil {
		err = bus.Object("org.a11y.atspi.Registry", "/org/a11y/atspi/registry").CallWithContext(ctx, "org.a11y.atspi.Registry.RegisterEvent", 0, "object:state-changed:focused", []string{}, "").Err
	}
	if err != nil {
		a.Close()
		return nil, err
	}
	return a, nil
}
func (a *atspi) Close() error                      { a.bus.RemoveSignal(a.signals); a.bus.Close(); return a.session.Close() }
func (a *atspi) object(r reference) dbus.BusObject { return a.bus.Object(r.Bus, r.Path) }
func (a *atspi) states(ctx context.Context, r reference) ([]uint32, error) {
	var states []uint32
	err := a.object(r).CallWithContext(ctx, accessible+".GetState", 0).Store(&states)
	return states, err
}
func state(v []uint32, bit uint) bool { return int(bit/32) < len(v) && v[bit/32]&(1<<(bit%32)) != 0 }
func (a *atspi) locked(ctx context.Context) bool {
	var locked bool
	if a.session.Object("org.gnome.ScreenSaver", "/org/gnome/ScreenSaver").CallWithContext(ctx, "org.gnome.ScreenSaver.GetActive", 0).Store(&locked) == nil {
		return locked
	}
	// A session without an observable lock state is not safe to mirror.
	return true
}
func (a *atspi) discover(ctx context.Context) reference {
	queue := []reference{{"org.a11y.atspi.Registry", "/org/a11y/atspi/accessible/root"}}
	for count := 0; len(queue) > 0 && count < 2048 && ctx.Err() == nil; count++ {
		r := queue[0]
		queue = queue[1:]
		states, err := a.states(ctx, r)
		if err != nil {
			continue
		}
		if state(states, 12) {
			return r
		} // ATSPI_STATE_FOCUSED
		var children []reference
		if a.object(r).CallWithContext(ctx, accessible+".GetChildren", 0).Store(&children) == nil {
			queue = append(queue, children...)
		}
	}
	return reference{}
}
func (a *atspi) property(ctx context.Context, r reference, name string) (int, error) {
	var v dbus.Variant
	err := a.object(r).CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, textInterface, name).Store(&v)
	n, ok := v.Value().(int32)
	if !ok && err == nil {
		return 0, ErrUnavailable
	}
	return int(n), err
}
func (a *atspi) Snapshot(ctx context.Context, window int) (Snapshot, error) {
	a.window = window
	for draining := true; draining; {
		select {
		case sig := <-a.signals:
			if sig.Name == "org.a11y.atspi.Event.Object.StateChanged" && len(sig.Body) >= 2 && sig.Body[0] == "focused" {
				a.epoch++
				if sig.Body[1] == int32(1) {
					a.focused = reference{sig.Sender, sig.Path}
				} else if a.focused.Path == sig.Path && a.focused.Bus == sig.Sender {
					a.focused = reference{}
				}
			}
		default:
			draining = false
		}
	}
	if a.locked(ctx) {
		a.focused = reference{}
		a.epoch++
		return Snapshot{Protected: true}, nil
	}
	if a.focused.Bus == "" {
		if time.Since(a.searched) < time.Second {
			return Snapshot{}, nil
		}
		a.searched = time.Now()
		a.focused = a.discover(ctx)
		a.epoch++
	}
	r := a.focused
	if r.Bus == "" {
		return Snapshot{}, nil
	}
	states, err := a.states(ctx, r)
	if err != nil || !state(states, 12) {
		a.focused = reference{}
		return Snapshot{}, nil
	}
	var role uint32
	if err = a.object(r).CallWithContext(ctx, accessible+".GetRole", 0).Store(&role); err != nil {
		return Snapshot{}, err
	}
	if role == 40 {
		return Snapshot{Protected: true}, nil
	} // ATSPI_ROLE_PASSWORD_TEXT, before reading text
	if !state(states, 7) || !state(states, 8) || !state(states, 24) {
		return Snapshot{}, nil
	} // ATSPI_STATE_EDITABLE
	var interfaces []string
	if err = a.object(r).CallWithContext(ctx, accessible+".GetInterfaces", 0).Store(&interfaces); err != nil {
		return Snapshot{}, err
	}
	writable := false
	hasText := false
	for _, v := range interfaces {
		if v == textInterface {
			hasText = true
		}
		if v == "org.a11y.atspi.EditableText" {
			writable = true
		}
	}
	if !hasText {
		return Snapshot{}, nil
	}
	count, err := a.property(ctx, r, "CharacterCount")
	if err != nil {
		return Snapshot{}, err
	}
	start, err := a.property(ctx, r, "CaretOffset")
	if err != nil || start < 0 || start > count {
		return Snapshot{}, ErrUnavailable
	}
	end := start
	var selections int32
	if a.object(r).CallWithContext(ctx, textInterface+".GetNSelections", 0).Store(&selections) == nil && selections > 0 {
		var x, y int32
		if a.object(r).CallWithContext(ctx, textInterface+".GetSelection", 0, int32(0)).Store(&x, &y) == nil {
			start = int(x)
			end = int(y)
		}
	}
	low := max(0, min(start, end)-window/2)
	high := min(count, low+window)
	low = max(0, high-window)
	if start < low || end < start || end > high {
		return Snapshot{}, nil
	} // Do not expose partial selections as editable ranges.
	var value string
	if err = a.object(r).CallWithContext(ctx, textInterface+".GetText", 0, int32(low), int32(high)).Store(&value); err != nil {
		return Snapshot{}, err
	}
	// Recheck privacy/focus after the read. Events racing this snapshot are handled next tick.
	now, err := a.states(ctx, r)
	if err != nil || !state(now, 12) || a.locked(ctx) {
		return Snapshot{}, ErrUnavailable
	}
	return Snapshot{Identity: fmt.Sprintf("%s:%s:%d", r.Bus, r.Path, a.epoch), Value: value, Start: start, End: end, WindowStart: low, Available: true, Editable: writable, Multiline: state(states, 17)}, nil
}
func (a *atspi) Replace(ctx context.Context, expected Snapshot, e Edit) error {
	current, err := a.Snapshot(ctx, a.window)
	if err != nil || current != expected {
		return ErrUnavailable
	}
	obj := a.object(a.focused)
	call := func(method string, args ...interface{}) error {
		var ok bool
		err := obj.CallWithContext(ctx, method, 0, args...).Store(&ok)
		if err != nil {
			return err
		}
		if !ok {
			return ErrUnavailable
		}
		return nil
	}
	if e.SelectionOnly {
		if e.Start == e.End {
			return call(textInterface+".SetCaretOffset", int32(e.Start))
		}
		var n int32
		if err := obj.CallWithContext(ctx, textInterface+".GetNSelections", 0).Store(&n); err != nil {
			return err
		}
		if n > 0 {
			return call(textInterface+".SetSelection", int32(0), int32(e.Start), int32(e.End))
		}
		return call(textInterface+".AddSelection", int32(e.Start), int32(e.End))
	}
	if e.Start != e.End {
		if err := call("org.a11y.atspi.EditableText.DeleteText", int32(e.Start), int32(e.End)); err != nil {
			return err
		}
	}
	if e.Text != "" {
		if err := call("org.a11y.atspi.EditableText.InsertText", int32(e.Start), e.Text, int32(len(e.Text))); err != nil {
			return err
		}
	}
	return call(textInterface+".SetCaretOffset", int32(e.Start+len([]rune(e.Text))))
}
