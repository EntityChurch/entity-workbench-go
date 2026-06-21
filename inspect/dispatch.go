package inspect

// Dispatch tap — parallel observation of dispatcher↔handler events.
// Replaces the old tap-as-handler shape (terminal observation) with
// safe-on-prod-paths parallel observation. Built on
// peer.WithDispatchHook (entity-core-go/core/peer/builder.go:215),
// landed per the inspectability cycle.
//
// Per v1.1 §2.1 #3: hook fires twice per dispatch (entry + exit) at
// the invoke closure (core/protocol/execute.go:319). Out-of-band sink,
// recursion-safe by construction.

import (
	"strings"
	"sync"
	"sync/atomic"

	"go.entitychurch.org/entity-core-go/core/handler"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/peer"
)

// DispatchTap captures dispatch events whose target_uri contains
// PathFilter (empty = match all). Install via PeerOption() at peer
// construction time. Stop()-able mid-run.
type DispatchTap struct {
	pathFilter string

	mu       sync.Mutex
	captures []DispatchCapture
	stopped  atomic.Bool
}

// DispatchCapture records one entry or exit of a matched dispatch.
type DispatchCapture struct {
	Seq            int64
	Phase          string // "entry" or "exit"
	TargetURI      string
	Operation      string
	ParamsHash     hash.Hash
	RequestID      string
	ResponseStatus uint      // zero at entry
	ResponseHash   hash.Hash // zero at entry
	Timestamp      int64     // unix nano
}

// DispatchExchange pairs an entry/exit observation for the same
// request_id. Either side may be nil if only one fired (saturation:
// pool-acquire failure produces entry without exit per core-go §3 of
// the v1.1 review).
type DispatchExchange struct {
	Entry *DispatchCapture
	Exit  *DispatchCapture
}

// NewDispatchTap returns a tap that filters by substring match on
// target_uri. Empty pathFilter matches all dispatches.
func NewDispatchTap(pathFilter string) *DispatchTap {
	return &DispatchTap{pathFilter: pathFilter}
}

// PeerOption returns the peer.Option that installs the dispatch hook.
// Apply at peer construction. The hook is process-local and operator-
// bounded by construction per the v1.1 security addendum §1.
func (t *DispatchTap) PeerOption() peer.Option {
	var counter atomic.Int64
	return peer.WithDispatchHook("inspect-dispatch-tap", func(evt handler.DispatchEvent) {
		if t.stopped.Load() {
			return
		}
		if t.pathFilter != "" && !strings.Contains(evt.TargetURI, t.pathFilter) {
			return
		}
		n := counter.Add(1)
		phase := "entry"
		if evt.Phase == handler.DispatchExit {
			phase = "exit"
		}
		c := DispatchCapture{
			Seq:            n,
			Phase:          phase,
			TargetURI:      evt.TargetURI,
			Operation:      evt.Operation,
			ParamsHash:     evt.ParamsHash,
			RequestID:      evt.RequestID,
			ResponseStatus: evt.ResponseStatus,
			ResponseHash:   evt.ResponseHash,
			Timestamp:      evt.Timestamp.UnixNano(),
		}
		t.mu.Lock()
		t.captures = append(t.captures, c)
		t.mu.Unlock()
	})
}

// Stop silently drops subsequent events.
func (t *DispatchTap) Stop() {
	t.stopped.Store(true)
}

// Captures returns a snapshot of recorded captures in fire order.
func (t *DispatchTap) Captures() []DispatchCapture {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]DispatchCapture, len(t.captures))
	copy(out, t.captures)
	return out
}

// Count returns how many captures the tap has recorded.
func (t *DispatchTap) Count() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.captures)
}

// Exchanges returns entry+exit pairs correlated by request_id. An
// exchange with Exit == nil indicates entry-without-exit (handler
// panic, pool-saturation refusal, or in-flight at snapshot time).
func (t *DispatchTap) Exchanges() []DispatchExchange {
	captures := t.Captures()
	byReq := map[string]*DispatchExchange{}
	var order []string
	for i := range captures {
		c := &captures[i]
		ex, ok := byReq[c.RequestID]
		if !ok {
			ex = &DispatchExchange{}
			byReq[c.RequestID] = ex
			order = append(order, c.RequestID)
		}
		if c.Phase == "entry" {
			ex.Entry = c
		} else {
			ex.Exit = c
		}
	}
	out := make([]DispatchExchange, 0, len(order))
	for _, id := range order {
		out = append(out, *byReq[id])
	}
	return out
}

// CountByStatus returns a histogram of exit-phase ResponseStatus
// values across all exchanges. Useful for "did this chain converge?"
// quick checks.
func (t *DispatchTap) CountByStatus() map[uint]int {
	out := map[uint]int{}
	for _, c := range t.Captures() {
		if c.Phase == "exit" {
			out[c.ResponseStatus]++
		}
	}
	return out
}
