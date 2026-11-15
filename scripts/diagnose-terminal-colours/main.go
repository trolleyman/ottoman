// A read-only colour-attribute probe. Default mode never calls GetText.
// --offsets reads a bounded focused-terminal sample to print only byte/rune counts.
// Neither mode prints terminal text or changes focus.
package main

import (
	"context"
	"fmt"
	"github.com/godbus/dbus/v5"
	"os"
	"sort"
	"time"
	"unicode/utf8"
)

const accessible = "org.a11y.atspi.Accessible"
const text = "org.a11y.atspi.Text"

type reference struct {
	Bus  string
	Path dbus.ObjectPath
}

func state(v []uint32, bit uint) bool { return int(bit/32) < len(v) && v[bit/32]&(1<<(bit%32)) != 0 }
func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	session, err := dbus.ConnectSessionBus()
	if err != nil {
		return err
	}
	defer session.Close()
	var locked bool
	if err := session.Object("org.gnome.ScreenSaver", "/org/gnome/ScreenSaver").CallWithContext(ctx, "org.gnome.ScreenSaver.GetActive", 0).Store(&locked); err != nil || locked {
		return fmt.Errorf("desktop locked or lock state unavailable")
	}
	var address string
	if err := session.Object("org.a11y.Bus", "/org/a11y/bus").CallWithContext(ctx, "org.a11y.Bus.GetAddress", 0).Store(&address); err != nil {
		return err
	}
	bus, err := dbus.Connect(address)
	if err != nil {
		return err
	}
	defer bus.Close()
	active := func(r reference) bool {
		seen := map[reference]bool{}
		for depth := 0; depth < 64 && r.Bus != "" && !seen[r]; depth++ {
			seen[r] = true
			obj := bus.Object(r.Bus, r.Path)
			var states []uint32
			if obj.CallWithContext(ctx, accessible+".GetState", 0).Store(&states) != nil {
				return false
			}
			if state(states, 1) {
				return true
			}
			var parent dbus.Variant
			if obj.CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, accessible, "Parent").Store(&parent) != nil {
				return false
			}
			if dbus.Store([]interface{}{parent.Value()}, &r) != nil {
				return false
			}
		}
		return false
	}
	queue := []reference{{"org.a11y.atspi.Registry", "/org/a11y/atspi/accessible/root"}}
	for count := 0; len(queue) > 0 && count < 4096 && ctx.Err() == nil; count++ {
		r := queue[0]
		queue = queue[1:]
		obj := bus.Object(r.Bus, r.Path)
		var states []uint32
		if obj.CallWithContext(ctx, accessible+".GetState", 0).Store(&states) != nil {
			continue
		}
		var role uint32
		if state(states, 12) && obj.CallWithContext(ctx, accessible+".GetRole", 0).Store(&role) == nil && role == 60 && active(r) {
			var interfaces []string
			obj.CallWithContext(ctx, accessible+".GetInterfaces", 0).Store(&interfaces)
			fmt.Printf("Focused terminal interfaces: %v\n", interfaces)
			props := map[string]int32{}
			for _, name := range []string{"CharacterCount", "CaretOffset"} {
				var value dbus.Variant
				if obj.CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, text, name).Store(&value) == nil {
					if n, ok := value.Value().(int32); ok {
						props[name] = n
					}
				}
			}
			fmt.Printf("Offsets: %v\n", props)
			if len(os.Args) > 1 && os.Args[1] == "--offsets" && props["CharacterCount"] <= 65536 {
				var value string
				if err := obj.CallWithContext(ctx, text+".GetText", 0, int32(0), props["CharacterCount"]).Store(&value); err != nil {
					fmt.Printf("Offset sample error: %v\n", err)
				} else {
					fmt.Printf("Offset units: CharacterCount=%d UTF8Bytes=%d UnicodeCharacters=%d\n", props["CharacterCount"], len(value), utf8.RuneCountInString(value))
				}
			}

			for _, offset := range []int32{max(0, props["CaretOffset"]-1), max(0, props["CharacterCount"]-1), max(0, props["CaretOffset"]-60)} {
				for _, method := range []string{"GetAttributeRun", "GetAttributes"} {
					var attrs map[string]string
					var start, end int32
					var err error
					if method == "GetAttributeRun" {
						err = obj.CallWithContext(ctx, text+"."+method, 0, offset, true).Store(&attrs, &start, &end)
					} else {
						err = obj.CallWithContext(ctx, text+"."+method, 0, offset).Store(&attrs, &start, &end)
					}
					if err != nil {
						fmt.Printf("%s offset %d: %v\n", method, offset, err)
						continue
					}
					keys := []string{}
					for key := range attrs {
						keys = append(keys, key)
					}
					sort.Strings(keys)
					fmt.Printf("%s offset %d: run %d..%d keys=%v fg-color=%q bg-color=%q\n", method, offset, start, end, keys, attrs["fg-color"], attrs["bg-color"])
					if len(os.Args) > 1 && os.Args[1] == "--offsets" && method == "GetAttributeRun" && end >= 0 && end < props["CharacterCount"] {
						var edge map[string]string
						var edgeStart, edgeEnd int32
						if obj.CallWithContext(ctx, text+".GetAttributeRun", 0, end, true).Store(&edge, &edgeStart, &edgeEnd) == nil {
							fmt.Printf("End-boundary probe %d: run %d..%d sameColour=%v\n", end, edgeStart, edgeEnd, edge["fg-color"] == attrs["fg-color"])
						}
					}

				}
			}
			return nil
		}
		var children []reference
		if obj.CallWithContext(ctx, accessible+".GetChildren", 0).Store(&children) == nil {
			queue = append(queue, children...)
		}
	}
	return fmt.Errorf("no focused terminal found; keep the terminal focused on the desktop")
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
