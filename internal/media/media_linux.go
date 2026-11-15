//go:build linux

package media

import (
	"context"
	"errors"
	"github.com/godbus/dbus/v5"
	"sort"
	"strings"
)

const iface = "org.mpris.MediaPlayer2.Player"

func names(ctx context.Context, c *dbus.Conn) ([]string, error) {
	var all []string
	if err := c.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.ListNames", 0).Store(&all); err != nil {
		return nil, err
	}
	var found []string
	for _, name := range all {
		if strings.HasPrefix(name, "org.mpris.MediaPlayer2.") {
			found = append(found, name)
		}
	}
	sort.Strings(found)
	return found, nil
}
func stateFor(ctx context.Context, c *dbus.Conn, name string) (State, error) {
	var props map[string]dbus.Variant
	if err := c.Object(name, "/org/mpris/MediaPlayer2").CallWithContext(ctx, "org.freedesktop.DBus.Properties.GetAll", 0, iface).Store(&props); err != nil {
		return State{}, err
	}
	return parse(name, props), nil
}
func parse(name string, props map[string]dbus.Variant) State {
	boolean := func(key string) bool { value, _ := props[key].Value().(bool); return value }
	status, _ := props["PlaybackStatus"].Value().(string)
	st := State{Available: status == "Playing" || status == "Paused", Player: name, Playing: status == "Playing", CanNext: boolean("CanControl") && boolean("CanGoNext"), CanPrevious: boolean("CanControl") && boolean("CanGoPrevious")}
	st.CanToggle = boolean("CanControl") && ((st.Playing && boolean("CanPause")) || (!st.Playing && boolean("CanPlay")))
	meta, _ := props["Metadata"].Value().(map[string]dbus.Variant)
	st.Title, _ = meta["xesam:title"].Value().(string)
	artists, _ := meta["xesam:artist"].Value().([]string)
	st.Artist = strings.Join(artists, ", ")
	st.Title = string([]rune(st.Title)[:min(len([]rune(st.Title)), 500)])
	st.Artist = string([]rune(st.Artist)[:min(len([]rune(st.Artist)), 500)])
	return st
}
func Read(ctx context.Context) (State, error) {
	c, err := dbus.ConnectSessionBus()
	if err != nil {
		return State{}, err
	}
	defer c.Close()
	players, err := names(ctx, c)
	if err != nil {
		return State{}, err
	}
	var chosen State
	for _, name := range players {
		st, err := stateFor(ctx, c, name)
		if err != nil {
			continue
		}
		if !chosen.Available {
			chosen = st
		}
		if st.Playing {
			return st, nil
		}
	}
	return chosen, nil
}
func Control(ctx context.Context, player, action string) error {
	c, err := dbus.ConnectSessionBus()
	if err != nil {
		return err
	}
	defer c.Close()
	players, err := names(ctx, c)
	if err != nil {
		return err
	}
	known := false
	for _, name := range players {
		if name == player {
			known = true
		}
	}
	if !known {
		return errors.New("media player unavailable")
	}
	st, err := stateFor(ctx, c, player)
	if err != nil {
		return err
	}
	method := ""
	switch action {
	case "toggle":
		if st.CanToggle {
			method = "PlayPause"
		}
	case "next":
		if st.CanNext {
			method = "Next"
		}
	case "previous":
		if st.CanPrevious {
			method = "Previous"
		}
	}
	if method == "" {
		return errors.New("media action unavailable")
	}
	return c.Object(player, "/org/mpris/MediaPlayer2").CallWithContext(ctx, iface+"."+method, 0).Err
}
