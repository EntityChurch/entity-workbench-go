package inspect

// Binding stream — mirror of ContentStream for path/binding events.
// Built on peer.WithBindingHook (entity-core-go/core/peer/builder.go:172),
// the observe-only alias for WithNamedSyncHook landed per
// the inspectability cycle.

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/peer"
	"go.entitychurch.org/entity-core-go/core/store"
)

// BindingStream captures every binding mutation. Install via
// PeerOption() at peer construction. Maps to v1.1 §2.1 #2.
type BindingStream struct {
	mu         sync.Mutex
	events     []BindingEvent
	pathFilter atomic.Value // string
	stopped    atomic.Bool
}

// BindingEvent records one location-index mutation.
type BindingEvent struct {
	Seq          int64
	Timestamp    time.Time
	Path         string
	PeerID       string
	Hash         hash.Hash
	PreviousHash hash.Hash
	ChangeType   store.ChangeType // Created / Modified / Deleted
	CascadeDepth uint64
}

// PeerOption returns the peer.Option that installs the binding hook.
// Apply at peer construction.
func (s *BindingStream) PeerOption() peer.Option {
	var counter atomic.Int64
	return peer.WithBindingHook("inspect-binding-stream", func(evt store.TreeChangeEvent) {
		if s.stopped.Load() {
			return
		}
		filter, _ := s.pathFilter.Load().(string)
		if filter != "" && !strings.Contains(evt.Path, filter) {
			return
		}
		n := counter.Add(1)
		var depth uint64
		if evt.Context != nil && evt.Context.CascadeDepth != nil {
			depth = *evt.Context.CascadeDepth
		}
		s.mu.Lock()
		s.events = append(s.events, BindingEvent{
			Seq:          n,
			Timestamp:    time.Now(),
			Path:         evt.Path,
			PeerID:       evt.PeerID,
			Hash:         evt.Hash,
			PreviousHash: evt.PreviousHash,
			ChangeType:   evt.ChangeType,
			CascadeDepth: depth,
		})
		s.mu.Unlock()
	})
}

// SetPathFilter narrows the stream to paths containing substr.
func (s *BindingStream) SetPathFilter(substr string) {
	s.pathFilter.Store(substr)
}

// Stop silently drops subsequent events.
func (s *BindingStream) Stop() {
	s.stopped.Store(true)
}

// Events returns a snapshot of captured events.
func (s *BindingStream) Events() []BindingEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]BindingEvent, len(s.events))
	copy(out, s.events)
	return out
}

// Count returns how many events have been captured.
func (s *BindingStream) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}

// CountByChangeType returns a histogram of change types.
func (s *BindingStream) CountByChangeType() map[store.ChangeType]int {
	out := map[store.ChangeType]int{}
	for _, e := range s.Events() {
		out[e.ChangeType]++
	}
	return out
}
