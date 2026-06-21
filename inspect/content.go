package inspect

// Content stream — every NEW entity Put fires the hook, captured with
// hash, type, data length, timestamp. Installed via peer.Option at
// peer-construction time; the NotifyingContentStore wraps the
// underlying store before any handler runs.
//
// Highest-leverage observability primitive in the entity system:
// every entity that flows — handler output, continuation advance,
// envelope ingest, tree::put, subscription delivery — passes through
// the content store. Hook it and you see everything.

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/peer"
	"go.entitychurch.org/entity-core-go/core/store"
)

// ContentStream captures every new content-store entity in real
// time. Construct with a zero value, attach via PeerOption() during
// peer creation, then read Events() after the run.
type ContentStream struct {
	mu         sync.Mutex
	events     []ContentEvent
	typeFilter atomic.Value // string
	stopped    atomic.Bool
}

// ContentEvent records one entity Put.
type ContentEvent struct {
	Seq       int64
	Timestamp time.Time
	Hash      hash.Hash
	Type      string
	DataLen   int
}

// PeerOption returns the peer.Option that installs the content
// hook. Apply at peer-construction time.
func (s *ContentStream) PeerOption() peer.Option {
	var counter atomic.Int64
	return peer.WithNamedContentHook("inspect-content-stream",
		func(evt store.ContentStoreEvent) *store.ContentConsumerResult {
			if s.stopped.Load() {
				return nil
			}
			filter, _ := s.typeFilter.Load().(string)
			if filter != "" && !strings.Contains(evt.Entity.Type, filter) {
				return nil
			}
			n := counter.Add(1)
			s.mu.Lock()
			s.events = append(s.events, ContentEvent{
				Seq:       n,
				Timestamp: time.Now(),
				Hash:      evt.Hash,
				Type:      evt.Entity.Type,
				DataLen:   len(evt.Entity.Data),
			})
			s.mu.Unlock()
			return nil
		})
}

// SetTypeFilter installs a substring filter on Entity.Type. Empty
// string captures everything.
func (s *ContentStream) SetTypeFilter(substr string) {
	s.typeFilter.Store(substr)
}

// Stop silently drops subsequent events without unregistering the
// hook (which can't be done after peer start).
func (s *ContentStream) Stop() {
	s.stopped.Store(true)
}

// Events returns a snapshot of captured content events.
func (s *ContentStream) Events() []ContentEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ContentEvent, len(s.events))
	copy(out, s.events)
	return out
}

// Count returns how many events have been captured.
func (s *ContentStream) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}

// CountByType returns a histogram of captured entity types.
func (s *ContentStream) CountByType() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]int{}
	for _, e := range s.events {
		out[e.Type]++
	}
	return out
}

// EntitiesOfType returns hashes of all captured entities of the
// given type.
func (s *ContentStream) EntitiesOfType(typeName string) []hash.Hash {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []hash.Hash
	for _, e := range s.events {
		if e.Type == typeName {
			out = append(out, e.Hash)
		}
	}
	return out
}
