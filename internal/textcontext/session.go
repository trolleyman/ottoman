// Package textcontext mirrors accessible focused text without persisting it.
package textcontext

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"
	"unicode/utf8"

	"github.com/trolleyman/ottoman/internal/api"
)

// Snapshot offsets are absolute Unicode code points, including WindowStart.
// Identity stays on the agent; only an opaque per-focus token crosses the wire.
type Snapshot struct {
	Identity                                  string
	Value                                     string
	Start, End, WindowStart                   int
	Editable, Multiline, Available, Protected bool
}

type Edit struct {
	Start, End    int
	Text          string
	SelectionOnly bool
}

// Backend is used only by the session worker, never by a WebSocket writer.
// Replace must recheck focus and protected state immediately before writing.
type Backend interface {
	Snapshot(context.Context, int) (Snapshot, error)
	Replace(context.Context, Snapshot, Edit) error
	Close() error
}

type command struct {
	watch *api.TrackpadMessageTextWatch
	edit  *api.TrackpadMessageTextEdit
}
type Session struct{ commands chan command }

func New(ctx context.Context, factory func(context.Context) (Backend, error), send func(api.TrackpadMessageTextContext) error) *Session {
	s := &Session{commands: make(chan command, 16)}
	go s.run(ctx, factory, send)
	return s
}
func (s *Session) Watch(ctx context.Context, v api.TrackpadMessageTextWatch) {
	select {
	case s.commands <- command{watch: &v}:
	case <-ctx.Done():
	}
}
func (s *Session) Edit(ctx context.Context, v api.TrackpadMessageTextEdit) {
	select {
	case s.commands <- command{edit: &v}:
	case <-ctx.Done():
	}
}

func token() string { var b [16]byte; _, _ = rand.Read(b[:]); return hex.EncodeToString(b[:]) }

func (s *Session) run(ctx context.Context, factory func(context.Context) (Backend, error), send func(api.TrackpadMessageTextContext) error) {
	var backend Backend
	defer func() {
		if backend != nil {
			backend.Close()
		}
	}()
	ticker := time.NewTicker(40 * time.Millisecond)
	defer ticker.Stop()
	var previous Snapshot
	var field string
	revision := 0
	window := 4096
	enabled := false
	var lastAttempt time.Time
	sendFailed := false
	nativeFailed := false
	publish := func(next Snapshot, force bool, request *int, rejected bool) {
		// A protected snapshot never carries even a name, identity, or selection.
		if next.Protected {
			next = Snapshot{Protected: true}
		}
		if !next.Available {
			next.Value = ""
			next.Start = 0
			next.End = 0
			next.WindowStart = 0
			next.Editable = false
		}
		changed := next != previous
		if next.Identity != previous.Identity || (next.Available && field == "") {
			field = token()
		}
		if !next.Available {
			field = ""
		}
		if changed {
			revision++
		}
		previous = next
		if changed || force {
			v := api.TrackpadMessageTextContext{Type: api.Textcontext, FieldId: field, Revision: revision, Value: next.Value, SelectionStart: next.Start, SelectionEnd: next.End, WindowStart: next.WindowStart, Editable: next.Editable, Multiline: next.Multiline, Available: next.Available, Protected: next.Protected, RequestId: request}
			if rejected {
				v.Rejected = &rejected
			}
			if send(v) != nil {
				sendFailed = true
				return
			}
		}
	}
	read := func() Snapshot {
		if enabled && backend == nil && time.Since(lastAttempt) >= 3*time.Second {
			lastAttempt = time.Now()
			var err error
			backend, err = factory(ctx)
			nativeFailed = err != nil
		}
		if backend == nil {
			return Snapshot{Protected: nativeFailed}
		}
		callCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
		defer cancel()
		v, err := backend.Snapshot(callCtx, window)
		if err != nil {
			backend.Close()
			backend = nil
			nativeFailed = true
			return Snapshot{Protected: true}
		}
		nativeFailed = false
		return v
	}
	for {
		select {
		case <-ctx.Done():
			previous = Snapshot{}
			return
		case <-ticker.C:
			if enabled {
				publish(read(), false, nil, false)
			}
		case c := <-s.commands:
			if c.watch != nil {
				enabled = c.watch.Enabled
				nativeFailed = false
				if c.watch.WindowSize != nil {
					window = max(128, min(4096, *c.watch.WindowSize))
				}
				if backend != nil {
					backend.Close()
					backend = nil
				}
				previous = Snapshot{}
				field = ""
				revision++
				if enabled {
					lastAttempt = time.Now()
					var err error
					backend, err = factory(ctx)
					nativeFailed = err != nil
					publish(read(), true, nil, false)
				} else {
					publish(Snapshot{}, true, nil, false)
				}
			}
			if c.edit != nil {
				e := c.edit
				publish(read(), false, nil, false)
				rejected := !enabled || backend == nil || !previous.Available || !previous.Editable || e.FieldId != field || e.BaseRevision != revision || e.Start < previous.WindowStart || e.End < e.Start || e.End > previous.WindowStart+utf8.RuneCountInString(previous.Value) || utf8.RuneCountInString(e.Text) > 4096
				if !rejected {
					callCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
					err := backend.Replace(callCtx, previous, Edit{Start: e.Start, End: e.End, Text: e.Text, SelectionOnly: e.SelectionOnly != nil && *e.SelectionOnly})
					cancel()
					rejected = err != nil
				}
				publish(read(), true, &e.RequestId, rejected)
			}
		}
		if sendFailed {
			previous = Snapshot{}
			return
		}
	}
}

var ErrUnavailable = errors.New("accessible text unavailable")
