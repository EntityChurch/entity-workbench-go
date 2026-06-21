package inspect

// Subscription tracer — captures emit + deliver events from the
// subscription engine. Two distinct event classes per v1.1 §2.1 #5
// (emit) + #6 (deliver) — the conflation hid F-CIMP-2 last cycle.
// Built on subscription.Engine.AddEmitHook / AddDeliverHook
// (entity-core-go/ext/subscription/inspect_hooks.go:59,66).
//
// Unlike ContentStream / BindingStream / DispatchTap / WireRecorder,
// the engine hooks are attached post-construction via the engine
// accessor on AppPeer (the core/ext dependency DAG forbids putting
// these on the core peer builder). Usage:
//
//   tracer := &inspect.SubscriptionTracer{}
//   peer, _ := entitysdk.CreatePeer(...)
//   tracer.Attach(peer.SubscriptionEngine())
//   // ... do stuff ...
//   for _, e := range tracer.Emits() { ... }

import (
	"sync"
	"sync/atomic"
	"time"

	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/ext/subscription"
)

// SubscriptionTracer captures emit and deliver events. Attach to the
// engine before traffic begins; engine registration races with active
// OnTreeChange goroutines are not synchronized per core-go's hook
// contract (inspect_hooks.go:53).
type SubscriptionTracer struct {
	mu       sync.Mutex
	emits    []EmitEvent
	delivers []DeliverEvent
	stopped  atomic.Bool

	emitSeq    atomic.Int64
	deliverSeq atomic.Int64
}

// EmitEvent records one subscription-match emit. Maps to
// subscription.EmitEvent plus observer-stamped seq.
type EmitEvent struct {
	Seq              int64
	Timestamp        time.Time
	SubscriptionID   string
	SourceChangeURI  string
	NotificationHash hash.Hash
}

// DeliverEvent records one delivery attempt outcome. Maps to
// subscription.DeliverEvent.
type DeliverEvent struct {
	Seq              int64
	Timestamp        time.Time
	SubscriptionID   string
	NotificationHash hash.Hash
	DeliverURI       string
	Status           uint
	ErrorCode        string
}

// Attach registers the tracer's emit + deliver hooks against the
// engine. Safe to call before peer.ListenReady; not safe to call
// concurrently with OnTreeChange activity.
func (s *SubscriptionTracer) Attach(engine *subscription.Engine) {
	if engine == nil {
		return
	}
	engine.AddEmitHook("inspect-subscription-emit", func(evt subscription.EmitEvent) {
		if s.stopped.Load() {
			return
		}
		n := s.emitSeq.Add(1)
		s.mu.Lock()
		s.emits = append(s.emits, EmitEvent{
			Seq:              n,
			Timestamp:        time.Now(),
			SubscriptionID:   evt.SubscriptionID,
			SourceChangeURI:  evt.SourceChangeURI,
			NotificationHash: evt.NotificationHash,
		})
		s.mu.Unlock()
	})
	engine.AddDeliverHook("inspect-subscription-deliver", func(evt subscription.DeliverEvent) {
		if s.stopped.Load() {
			return
		}
		n := s.deliverSeq.Add(1)
		s.mu.Lock()
		s.delivers = append(s.delivers, DeliverEvent{
			Seq:              n,
			Timestamp:        time.Now(),
			SubscriptionID:   evt.SubscriptionID,
			NotificationHash: evt.NotificationHash,
			DeliverURI:       evt.DeliverURI,
			Status:           evt.Status,
			ErrorCode:        evt.ErrorCode,
		})
		s.mu.Unlock()
	})
}

// Stop silently drops subsequent events.
func (s *SubscriptionTracer) Stop() {
	s.stopped.Store(true)
}

// Emits returns a snapshot of captured emit events.
func (s *SubscriptionTracer) Emits() []EmitEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]EmitEvent, len(s.emits))
	copy(out, s.emits)
	return out
}

// Delivers returns a snapshot of captured deliver events.
func (s *SubscriptionTracer) Delivers() []DeliverEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]DeliverEvent, len(s.delivers))
	copy(out, s.delivers)
	return out
}

// CountEmits / CountDelivers return event counts without copy.
func (s *SubscriptionTracer) CountEmits() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.emits)
}

func (s *SubscriptionTracer) CountDelivers() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.delivers)
}

// CountByDeliveryStatus returns a histogram of delivery status codes.
// The shape that surfaces F-CIMP-2-style conflations (emit succeeds,
// deliver fails — both in one histogram row, distinct).
func (s *SubscriptionTracer) CountByDeliveryStatus() map[uint]int {
	out := map[uint]int{}
	for _, d := range s.Delivers() {
		out[d.Status]++
	}
	return out
}
