//go:build linux

package media

import (
	"github.com/godbus/dbus/v5"
	"testing"
)

func TestCapabilitiesAndMetadata(t *testing.T) {
	props := map[string]dbus.Variant{"CanControl": dbus.MakeVariant(true), "CanPlay": dbus.MakeVariant(true), "CanPause": dbus.MakeVariant(false), "CanGoNext": dbus.MakeVariant(true), "PlaybackStatus": dbus.MakeVariant("Playing"), "Metadata": dbus.MakeVariant(map[string]dbus.Variant{"mpris:artUrl": dbus.MakeVariant("file:///tmp/album.png"), "xesam:title": dbus.MakeVariant("A title"), "xesam:artist": dbus.MakeVariant([]string{"One", "Two"})})}
	st := parse("org.mpris.MediaPlayer2.test", props)
	if !st.Available || !st.Playing || st.CanToggle || !st.CanNext || st.CanPrevious || st.Title != "A title" || st.Artist != "One, Two" {
		t.Fatalf("unexpected state: %+v", st)
	}
	if st.Artwork == "" || st.ArtworkSource != "file:///tmp/album.png" {
		t.Fatal("missing current artwork URL")
	}
	props["PlaybackStatus"] = dbus.MakeVariant("Paused")
	if !parse("test", props).CanToggle {
		t.Fatal("paused player can play")
	}
	props["CanControl"] = dbus.MakeVariant(false)
	st = parse("test", props)
	if st.CanToggle || st.CanNext {
		t.Fatal("read-only player exposed controls")
	}
}
