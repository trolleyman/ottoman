package textcontext

import (
	"context"
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
	backend.Set(Snapshot{Identity: "password", Value: "secret", Start: 3, End: 3, Available: true, Editable: true, Protected: true})
	protected := receive(t, out, func(v api.TrackpadMessageTextContext) bool { return v.Protected })
	if protected.Value != "" || protected.FieldId != "" || protected.SelectionStart != 0 || protected.Available || protected.Editable {
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
