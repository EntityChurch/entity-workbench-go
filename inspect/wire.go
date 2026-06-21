package inspect

// Wire recorder — captures every inbound/outbound envelope frame as
// raw CBOR bytes for replay, cross-impl byte-diffing, and forensic
// post-mortem. Built on peer.WithWireHook (entity-core-go/core/peer/
// builder.go:194).
//
// Security note (v1.1 security addendum §2): FrameBytes carries the
// full wire envelope including capability tokens, signatures, and
// payload entities. Recordings are sensitive artifacts. The recorder
// here is in-memory only; persisting to disk requires operator
// policy on the sink.

import (
	"strings"
	"sync"
	"sync/atomic"

	"go.entitychurch.org/entity-core-go/core/peer"
)

// WireRecorder captures wire frames in fire order. Install via
// PeerOption() at peer construction. The recorder copies FrameBytes
// at capture time per the core-go hook contract (FrameBytes slice
// lifetime is bounded by the hook's return).
type WireRecorder struct {
	mu      sync.Mutex
	frames  []WireFrame
	stopped atomic.Bool

	rootFilter atomic.Value // string — substring filter on RootType
}

// WireFrame records one envelope traversal.
type WireFrame struct {
	Seq         int64
	Direction   string // "in" or "out"
	Bytes       []byte // copied at capture; caller owns
	PeerAddress string
	RequestID   string
	RootType    string
	Timestamp   int64 // unix nano
}

// PeerOption returns the peer.Option that installs the wire hook.
// Apply at peer construction.
func (r *WireRecorder) PeerOption() peer.Option {
	var counter atomic.Int64
	return peer.WithWireHook("inspect-wire-recorder", func(evt peer.WireEvent) {
		if r.stopped.Load() {
			return
		}
		filter, _ := r.rootFilter.Load().(string)
		if filter != "" && !strings.Contains(evt.RootType, filter) {
			return
		}
		n := counter.Add(1)
		dir := "in"
		if evt.Direction == peer.WireOutbound {
			dir = "out"
		}
		// Copy bytes — the hook contract is that the slice is owned by
		// the read/write codepath and not retained past hook return.
		buf := make([]byte, len(evt.FrameBytes))
		copy(buf, evt.FrameBytes)
		f := WireFrame{
			Seq:         n,
			Direction:   dir,
			Bytes:       buf,
			PeerAddress: evt.PeerAddress,
			RequestID:   evt.RequestID,
			RootType:    evt.RootType,
			Timestamp:   evt.Timestamp.UnixNano(),
		}
		r.mu.Lock()
		r.frames = append(r.frames, f)
		r.mu.Unlock()
	})
}

// SetRootTypeFilter narrows the recorder to frames whose RootType
// contains substr. Useful for cross-impl wire-shape debugging.
func (r *WireRecorder) SetRootTypeFilter(substr string) {
	r.rootFilter.Store(substr)
}

// Stop silently drops subsequent frames.
func (r *WireRecorder) Stop() {
	r.stopped.Store(true)
}

// Frames returns a snapshot of recorded frames in fire order.
func (r *WireRecorder) Frames() []WireFrame {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]WireFrame, len(r.frames))
	copy(out, r.frames)
	return out
}

// Count returns how many frames have been captured.
func (r *WireRecorder) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.frames)
}

// FramesByRequest returns all frames matching a request_id. Useful
// for cross-peer correlation: same request_id observed on two peers'
// recorders.
func (r *WireRecorder) FramesByRequest(id string) []WireFrame {
	var out []WireFrame
	for _, f := range r.Frames() {
		if f.RequestID == id {
			out = append(out, f)
		}
	}
	return out
}

// CountByRootType returns a histogram of envelope root types.
func (r *WireRecorder) CountByRootType() map[string]int {
	out := map[string]int{}
	for _, f := range r.Frames() {
		out[f.RootType]++
	}
	return out
}
