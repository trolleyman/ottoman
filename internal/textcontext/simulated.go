package textcontext

import (
	"context"
	"sync"
)

// Simulated is an in-memory focused field. Set can model local edits, focus
// changes, protected controls and permission loss without native APIs.
type Simulated struct {
	mu      sync.Mutex
	Current Snapshot
}

func (s *Simulated) Set(v Snapshot) { s.mu.Lock(); defer s.mu.Unlock(); s.Current = v }
func (s *Simulated) Snapshot(context.Context, int) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Current, nil
}
func (s *Simulated) Replace(_ context.Context, expected Snapshot, e Edit) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Current != expected || !expected.Available || !expected.Editable || expected.Protected {
		return ErrUnavailable
	}
	if e.SelectionOnly {
		s.Current.Start = e.Start
		s.Current.End = e.End
		return nil
	}
	chars := []rune(s.Current.Value)
	start, end := e.Start-s.Current.WindowStart, e.End-s.Current.WindowStart
	if start < 0 || end < start || end > len(chars) {
		return ErrUnavailable
	}
	s.Current.Value = string(chars[:start]) + e.Text + string(chars[end:])
	s.Current.Start = e.Start + len([]rune(e.Text))
	s.Current.End = s.Current.Start
	return nil
}
func (*Simulated) Close() error { return nil }
