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
	Password                                  bool
	// MaskedLength is metadata only; never derive it by reading protected text.
	MaskedLength *int
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
		// Preserve only an explicit password marker and safe count metadata.
		// Locked/inaccessible sessions must not be presented as password fields.
		if next.Protected || next.Password {
			password := next.Password
			identity := next.Identity
			length := next.MaskedLength
			next = Snapshot{Protected: true, Password: password}
			if password {
				next.Identity = identity
			}
			if password && length != nil && *length >= 0 {
				bounded := min(4096, *length)
				next.MaskedLength = &bounded
			}
		} else {
			next.MaskedLength = nil
		}
		if !next.Available {
			next.Value = ""
			next.Start = 0
			next.End = 0
			next.WindowStart = 0
			next.Editable = false
		}
		changed := !sameSnapshot(next, previous)
		if next.Identity != previous.Identity || ((next.Available || next.Password) && field == "") {
			field = token()
		}
		if !next.Available && !next.Password {
			field = ""
		}
		if changed {
			revision++
		}
		previous = next
		if changed || force {
			v := api.TrackpadMessageTextContext{Type: api.Textcontext, FieldId: field, Revision: revision, Value: next.Value, SelectionStart: next.Start, SelectionEnd: next.End, WindowStart: next.WindowStart, Editable: next.Editable, Multiline: next.Multiline, Available: next.Available, Protected: next.Protected, Password: next.Password, MaskedLength: next.MaskedLength, RequestId: request}
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
				var result Snapshot
				if !rejected {
					before := previous
					edit := Edit{Start: e.Start, End: e.End, Text: e.Text, SelectionOnly: e.SelectionOnly != nil && *e.SelectionOnly}
					callCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
					err := backend.Replace(callCtx, before, edit)
					cancel()
					rejected = err != nil
					if !rejected {
						result, rejected = settleEdit(ctx, before, edit, window, read)
					} else {
						result = read()
					}
				} else {
					result = read()
				}
				publish(result, true, &e.RequestId, rejected)
			}
		}
		if sendFailed {
			previous = Snapshot{}
			return
		}
	}
}

var ErrUnavailable = errors.New("accessible text unavailable")

// Snapshot includes optional metadata pointers; compare their values so stable
// password counts do not cause repeated updates or revive dismissed previews.
func sameSnapshot(a, b Snapshot) bool {
	al, bl := a.MaskedLength, b.MaskedLength
	a.MaskedLength = nil
	b.MaskedLength = nil
	return a == b && ((al == nil && bl == nil) || (al != nil && bl != nil && *al == *bl))
}

// Native accessibility writes can return before the renderer publishes the new
// text and caret. Acknowledge only after observing both, without reapplying the
// mutation or publishing intermediate snapshots that could roll back phone input.
func settleEdit(ctx context.Context, before Snapshot, edit Edit, window int, read func() Snapshot) (Snapshot, bool) {
	want := before
	if edit.SelectionOnly {
		want.Start, want.End = edit.Start, edit.End
	} else {
		chars := []rune(before.Value)
		want.Value = string(chars[:edit.Start-before.WindowStart]) + edit.Text + string(chars[edit.End-before.WindowStart:])
		want.Start = edit.Start + utf8.RuneCountInString(edit.Text)
		want.End = want.Start
	}
	deadline := time.NewTimer(400 * time.Millisecond)
	defer deadline.Stop()
	for {
		next := read()
		if next.Identity != before.Identity || !next.Available || !next.Editable || next.Protected || next.Password {
			return next, true
		}
		// Compare the shared window: the provider may recenter or crop a full
		// 4096-character preview after an insertion near its edge.
		low := max(want.WindowStart, next.WindowStart)
		high := min(want.WindowStart+utf8.RuneCountInString(want.Value), next.WindowStart+utf8.RuneCountInString(next.Value))
		matches := low < high || (want.Value == "" && next.Value == "")
		if matches && low <= high {
			w, n := []rune(want.Value), []rune(next.Value)
			matches = string(w[low-want.WindowStart:high-want.WindowStart]) == string(n[low-next.WindowStart:high-next.WindowStart])
		}
		if next.WindowStart == want.WindowStart && utf8.RuneCountInString(before.Value) < window {
			matches = next.Value == want.Value
		}
		// Chrome's omnibox can publish a selected completion instead of the
		// plain inserted text. The user's prefix is committed in this case.
		// Acknowledge it so the next queued edit can replace that suffix.
		if inlineCompletion(want, next) {
			return next, false
		}
		if matches && next.Start == want.Start && next.End == want.End {
			return next, false
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return Snapshot{}, true
		case <-deadline.C:
			timer.Stop()
			return next, true
		case <-timer.C:
		}
	}
}

// Only accept an appended, selected suggestion at the exact intended caret.
// In particular this cannot hide changes to the text the user already typed.
func inlineCompletion(want, next Snapshot) bool {
	if want.Identity != next.Identity || want.WindowStart != next.WindowStart ||
		want.Start != want.End || next.Start != want.Start || next.Start < next.WindowStart || next.End <= next.Start ||
		!next.Available || !next.Editable || next.Protected || next.Password {
		return false
	}
	chars := []rune(next.Value)
	return next.End == next.WindowStart+len(chars) &&
		want.End == want.WindowStart+utf8.RuneCountInString(want.Value) &&
		string(chars[:next.Start-next.WindowStart]) == want.Value
}
