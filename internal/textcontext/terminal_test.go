package textcontext

import (
	"context"
	"testing"

	"github.com/trolleyman/ottoman/internal/api"
)

func TestCompactTerminal(t *testing.T) {
	for _, tc := range []struct {
		name, value, want string
		caret, low        int
	}{
		{"inline prompt", "old output\nuser@host:~$ echo 😀\nafter", "user@host:~$ echo 😀", 28, 0},
		{"two line prompt", "old output\n~/code/ottoman on main\n❯ echo 😀", "~/code/ottoman on main\n❯ echo 😀", 40, 0},
		{"cursor in command", "old\n❯ echo hello world\nnext", "❯ echo hello world", 11, 100},
		{"empty line", "old\n", "", 4, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caret := min(tc.caret, len([]rune(tc.value))) + tc.low
			v := compactTerminal(Snapshot{Terminal: true, Value: tc.value, WindowStart: tc.low, Start: caret, End: caret, TerminalColors: []api.TerminalColor{{Start: tc.low, End: tc.low + len([]rune(tc.value)), Color: "#00ff00"}}})
			if v.Value != tc.want || v.Start != caret || v.End != caret {
				t.Fatalf("unexpected preview: %#v", v)
			}
			if v.WindowStart+len([]rune(v.Value)) < caret {
				t.Fatal("caret outside preview")
			}
			for _, run := range v.TerminalColors {
				if run.Start < v.WindowStart || run.End > v.WindowStart+len([]rune(v.Value)) {
					t.Fatal("colour outside preview")
				}
			}
		})
	}
}

func TestTerminalScrollbackOptIn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	buffer := "previous output\n~/code/ottoman on main\n❯ echo 😀"
	end := len([]rune(buffer))
	backend := &Simulated{Current: Snapshot{Identity: "terminal", Available: true, Multiline: true, Terminal: true, Value: buffer, Start: end, End: end}}
	out := make(chan api.TrackpadMessageTextContext, 32)
	session := New(ctx, func(context.Context) (Backend, error) { return backend, nil }, func(v api.TrackpadMessageTextContext) error { out <- v; return nil })
	session.Watch(ctx, api.TrackpadMessageTextWatch{Enabled: true})
	compact := receive(t, out, func(v api.TrackpadMessageTextContext) bool { return v.Available })
	if compact.Value != "~/code/ottoman on main\n❯ echo 😀" || compact.WindowStart != 16 || !compact.Terminal {
		t.Fatalf("unexpected compact preview: %#v", compact)
	}
	session.Watch(ctx, api.TrackpadMessageTextWatch{Enabled: true, TerminalScrollback: true})
	full := receive(t, out, func(v api.TrackpadMessageTextContext) bool { return v.Available && v.Value == buffer })
	if full.WindowStart != 0 {
		t.Fatal("scrollback offsets changed")
	}
	changed := backend.Current
	changed.TerminalColors = []api.TerminalColor{{Start: end - 1, End: end, Color: "#00ff00"}}
	backend.Set(changed)
	receive(t, out, func(v api.TrackpadMessageTextContext) bool {
		return len(v.TerminalColors) > 0 && v.Revision > full.Revision
	})
}
