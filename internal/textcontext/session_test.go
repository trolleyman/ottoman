package textcontext

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/trolleyman/ottoman/internal/api"
)

func receive(t *testing.T, ch <-chan api.TrackpadMessageTextContext, predicate func(api.TrackpadMessageTextContext) bool) api.TrackpadMessageTextContext {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case v := <-ch:
			if predicate(v) {
				return v
			}
		case <-timeout:
			t.Fatal("timed out waiting for context")
			return api.TrackpadMessageTextContext{}
		}
	}
}
func TestSessionGuardedEdits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	backend := &Simulated{Current: Snapshot{Identity: "one", Value: "a😀b", Start: 2, End: 2, Available: true, Editable: true}}
	out := make(chan api.TrackpadMessageTextContext, 32)
	s := New(ctx, func(context.Context) (Backend, error) { return backend, nil }, func(v api.TrackpadMessageTextContext) error { out <- v; return nil })
	s.Watch(ctx, api.TrackpadMessageTextWatch{Enabled: true})
	first := receive(t, out, func(v api.TrackpadMessageTextContext) bool { return v.Available })
	s.Edit(ctx, api.TrackpadMessageTextEdit{FieldId: first.FieldId, BaseRevision: first.Revision, Start: 1, End: 2, Text: "👍", RequestId: 1})
	accepted := receive(t, out, func(v api.TrackpadMessageTextContext) bool { return v.RequestId != nil })
	if accepted.Value != "a👍b" || accepted.SelectionStart != 2 || accepted.Revision <= first.Revision || accepted.Rejected != nil {
		t.Fatalf("unexpected edit result: %#v", accepted)
	}
	s.Edit(ctx, api.TrackpadMessageTextEdit{FieldId: first.FieldId, BaseRevision: first.Revision, Start: 0, End: 1, Text: "wrong", RequestId: 2})
	stale := receive(t, out, func(v api.TrackpadMessageTextContext) bool { return v.RequestId != nil && *v.RequestId == 2 })
	if stale.Rejected == nil || !*stale.Rejected || stale.Value != "a👍b" {
		t.Fatal("stale edit was not rejected")
	}
	backend.Set(Snapshot{Identity: "two", Value: "different", Available: true, Editable: true})
	s.Edit(ctx, api.TrackpadMessageTextEdit{FieldId: accepted.FieldId, BaseRevision: accepted.Revision, Start: 0, End: 0, Text: "wrong", RequestId: 3})
	focused := receive(t, out, func(v api.TrackpadMessageTextContext) bool { return v.RequestId != nil && *v.RequestId == 3 })
	if focused.Rejected == nil || !*focused.Rejected || focused.Value != "different" || focused.FieldId == first.FieldId {
		t.Fatal("edit crossed a focus boundary")
	}
	backend.Set(Snapshot{Identity: "password", Value: "secret", Start: 3, End: 3, Available: true, Editable: true, Protected: true, Password: true})
	protected := receive(t, out, func(v api.TrackpadMessageTextContext) bool { return v.Protected })
	if !protected.Password || protected.MaskedLength != nil || protected.Value != "" || protected.FieldId == "" || protected.SelectionStart != 0 || protected.Available || protected.Editable {
		t.Fatal("protected context leaked")
	}
}

func TestSessionRequiresOptInAndClearsOnDisable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := make(chan struct{}, 2)
	out := make(chan api.TrackpadMessageTextContext, 16)
	s := New(ctx, func(context.Context) (Backend, error) {
		calls <- struct{}{}
		return &Simulated{Current: Snapshot{Identity: "one", Value: "hello", Available: true, Editable: true}}, nil
	}, func(v api.TrackpadMessageTextContext) error { out <- v; return nil })
	s.Edit(ctx, api.TrackpadMessageTextEdit{RequestId: 1})
	v := receive(t, out, func(v api.TrackpadMessageTextContext) bool { return v.RequestId != nil })
	if v.Rejected == nil || !*v.Rejected {
		t.Fatal("edit without opt-in accepted")
	}
	select {
	case <-calls:
		t.Fatal("native APIs used before opt-in")
	default:
	}
	s.Watch(ctx, api.TrackpadMessageTextWatch{Enabled: true})
	receive(t, out, func(v api.TrackpadMessageTextContext) bool { return v.Available })
	s.Watch(ctx, api.TrackpadMessageTextWatch{Enabled: false})
	v = receive(t, out, func(v api.TrackpadMessageTextContext) bool { return !v.Available })
	if v.Value != "" || v.FieldId != "" {
		t.Fatal("disable retained context")
	}
}

func TestSessionRejectsOutOfWindowAndOversizedEdits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := make(chan api.TrackpadMessageTextContext, 16)
	backend := &Simulated{Current: Snapshot{Identity: "one", Value: "abc", WindowStart: 20, Start: 21, End: 21, Available: true, Editable: true}}
	s := New(ctx, func(context.Context) (Backend, error) { return backend, nil }, func(v api.TrackpadMessageTextContext) error { out <- v; return nil })
	s.Watch(ctx, api.TrackpadMessageTextWatch{Enabled: true})
	v := receive(t, out, func(v api.TrackpadMessageTextContext) bool { return v.Available })
	for i, r := range [][2]int{{19, 20}, {20, 24}, {22, 21}} {
		s.Edit(ctx, api.TrackpadMessageTextEdit{FieldId: v.FieldId, BaseRevision: v.Revision, Start: r[0], End: r[1], Text: "x", RequestId: i + 1})
		result := receive(t, out, func(v api.TrackpadMessageTextContext) bool { return v.RequestId != nil })
		if result.Rejected == nil || !*result.Rejected || result.Value != "abc" {
			t.Fatal("invalid range accepted")
		}
	}
}

type closingBackend struct {
	Simulated
	closed chan struct{}
}

func (b *closingBackend) Close() error { close(b.closed); return nil }
func TestDisconnectStopsObserver(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	backend := &closingBackend{closed: make(chan struct{})}
	ready := make(chan struct{}, 1)
	s := New(ctx, func(context.Context) (Backend, error) { return backend, nil }, func(api.TrackpadMessageTextContext) error { ready <- struct{}{}; return nil })
	s.Watch(ctx, api.TrackpadMessageTextWatch{Enabled: true})
	<-ready
	cancel()
	select {
	case <-backend.closed:
	case <-time.After(time.Second):
		t.Fatal("disconnect left native observer running")
	}
}

func TestPasswordMaskMetadataIsSanitized(t *testing.T) {
	for _, tc := range []struct {
		name       string
		password   bool
		length     *int
		wantLength *int
	}{
		{"known password length", true, intPointer(6), intPointer(6)},
		{"unavailable password length", true, nil, nil},
		{"negative length", true, intPointer(-1), nil},
		{"bounded length", true, intPointer(10000), intPointer(4096)},
		{"lock state has no password metadata", false, intPointer(6), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			backend := &Simulated{Current: Snapshot{Identity: "private-field-name", Value: "must-never-be-mirrored", Start: 5, End: 8, WindowStart: 2, Available: true, Editable: true, Multiline: true, Protected: true, Password: tc.password, MaskedLength: tc.length}}
			out := make(chan api.TrackpadMessageTextContext, 8)
			s := New(ctx, func(context.Context) (Backend, error) { return backend, nil }, func(v api.TrackpadMessageTextContext) error { out <- v; return nil })
			s.Watch(ctx, api.TrackpadMessageTextWatch{Enabled: true})
			masked := receive(t, out, func(v api.TrackpadMessageTextContext) bool { return v.Protected })
			if masked.Value != "" || (tc.password && masked.FieldId == "") || (!tc.password && masked.FieldId != "") || masked.Available || masked.Editable || masked.Multiline || masked.SelectionStart != 0 || masked.SelectionEnd != 0 || masked.WindowStart != 0 || masked.Password != tc.password {
				t.Fatal("protected snapshot exposed text or editing metadata")
			}
			if (masked.MaskedLength == nil) != (tc.wantLength == nil) || (tc.wantLength != nil && *masked.MaskedLength != *tc.wantLength) {
				t.Fatal("incorrect mask length")
			}
			wire, err := json.Marshal(masked)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(wire), "must-never-be-mirrored") || strings.Contains(string(wire), "private-field-name") {
				t.Fatal("protected data appeared in the wire context")
			}
			s.Watch(ctx, api.TrackpadMessageTextWatch{Enabled: false})
			cleared := receive(t, out, func(v api.TrackpadMessageTextContext) bool { return !v.Protected })
			if cleared.Password || cleared.MaskedLength != nil {
				t.Fatal("disable retained password metadata")
			}
		})
	}
}
func intPointer(v int) *int { return &v }
func TestMaskMetadataEquality(t *testing.T) {
	a := Snapshot{Protected: true, Password: true, MaskedLength: intPointer(6)}
	b := Snapshot{Protected: true, Password: true, MaskedLength: intPointer(6)}
	if !sameSnapshot(a, b) {
		t.Fatal("identical mask counts would trigger repeated updates")
	}
	b.MaskedLength = intPointer(7)
	if sameSnapshot(a, b) {
		t.Fatal("password length change would not be published")
	}
	b.MaskedLength = nil
	if sameSnapshot(a, b) {
		t.Fatal("unknown length must not equal a known count")
	}
}

func TestPasswordFocusChangesWithoutExposingIdentity(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := Snapshot{Identity: "private-password-a", Protected: true, Password: true, MaskedLength: intPointer(6)}
	backend := &Simulated{Current: first}
	out := make(chan api.TrackpadMessageTextContext, 8)
	s := New(ctx, func(context.Context) (Backend, error) { return backend, nil }, func(v api.TrackpadMessageTextContext) error { out <- v; return nil })
	s.Watch(ctx, api.TrackpadMessageTextWatch{Enabled: true})
	a := receive(t, out, func(v api.TrackpadMessageTextContext) bool { return v.Password })
	first.Identity = "private-password-b"
	backend.Set(first)
	b := receive(t, out, func(v api.TrackpadMessageTextContext) bool { return v.Password && v.FieldId != a.FieldId })
	if b.FieldId == "" || b.Revision <= a.Revision || strings.Contains(b.FieldId, "private-password") {
		t.Fatal("password focus did not receive a new opaque token")
	}
	backend.Set(Snapshot{Protected: true})
	locked := receive(t, out, func(v api.TrackpadMessageTextContext) bool { return v.Protected && !v.Password })
	if locked.FieldId != "" || locked.MaskedLength != nil {
		t.Fatal("lock screen retained password metadata")
	}
}

// Model a renderer whose accessibility cache lags successful native writes.
type delayedTextBackend struct {
	Simulated
	old        Snapshot
	staleReads int
}

func (b *delayedTextBackend) Replace(ctx context.Context, before Snapshot, edit Edit) error {
	b.old = before
	if err := b.Simulated.Replace(ctx, before, edit); err != nil {
		return err
	}
	b.staleReads = 3
	return nil
}
func (b *delayedTextBackend) Snapshot(ctx context.Context, window int) (Snapshot, error) {
	if b.staleReads > 0 {
		b.staleReads--
		if b.staleReads > 0 {
			return b.old, nil
		}
		// Text arrives before its caret notification.
		next, err := b.Simulated.Snapshot(ctx, window)
		next.Start, next.End = b.old.Start, b.old.End
		return next, err
	}
	return b.Simulated.Snapshot(ctx, window)
}
func TestSessionWaitsForRendererBeforeAcknowledgingBurst(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	backend := &delayedTextBackend{Simulated: Simulated{Current: Snapshot{Identity: "one", Value: "", Available: true, Editable: true}}}
	out := make(chan api.TrackpadMessageTextContext, 32)
	s := New(ctx, func(context.Context) (Backend, error) { return backend, nil }, func(v api.TrackpadMessageTextContext) error { out <- v; return nil })
	s.Watch(ctx, api.TrackpadMessageTextWatch{Enabled: true})
	current := receive(t, out, func(v api.TrackpadMessageTextContext) bool { return v.Available })
	want := ""
	for i, char := range []string{"h", "e", "l", "l", "o", "😀"} {
		s.Edit(ctx, api.TrackpadMessageTextEdit{FieldId: current.FieldId, BaseRevision: current.Revision, Start: i, End: i, Text: char, RequestId: i + 1})
		want += char
		current = receive(t, out, func(v api.TrackpadMessageTextContext) bool { return v.RequestId != nil })
		if current.Rejected != nil || current.Value != want || current.SelectionStart != i+1 || current.SelectionEnd != i+1 {
			t.Fatal("acknowledged stale renderer text or caret")
		}
	}
}
func TestEditSettlementStopsAtPrivacyAndFocusBoundaries(t *testing.T) {
	before := Snapshot{Identity: "one", Value: "abc", Start: 3, End: 3, Available: true, Editable: true}
	for _, next := range []Snapshot{
		{Identity: "two", Value: "other", Available: true, Editable: true},
		{Identity: "one", Protected: true},
		{Identity: "one", Password: true, Protected: true},
	} {
		_, rejected := settleEdit(context.Background(), before, Edit{Start: 3, End: 3, Text: "x"}, 4096, func() Snapshot { return next })
		if !rejected {
			t.Fatal("accepted an edit across a focus or privacy boundary")
		}
	}
}
func TestEditSettlementRejectsUnconfirmedWrites(t *testing.T) {
	before := Snapshot{Identity: "one", Value: "abc", Start: 3, End: 3, Available: true, Editable: true}
	_, rejected := settleEdit(context.Background(), before, Edit{Start: 3, End: 3, Text: "x"}, 4096, func() Snapshot { return before })
	if !rejected {
		t.Fatal("accepted a native write that never appeared")
	}
}
