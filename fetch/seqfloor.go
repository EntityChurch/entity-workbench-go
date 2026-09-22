package fetch

import (
	"fmt"
	"sync"
)

// seqfloor.go — the §3-RES.4 monotonicity floor, extracted so that one
// publisher has ONE of them however many roads reach it.
//
// # Why this stopped being a field on Consumer
//
// The floor is a memory of the highest `seq` this session has accepted
// **from a publisher**. It was a `Consumer` field, which is the same
// thing for exactly as long as a publisher is reachable one way.
//
// A publisher reachable both statically and live is two [Source]s, hence
// two `Consumer`s, hence — before this file — two floors. And the split
// is silent and points the wrong way: a reader that takes the live road,
// sees seq=7, then falls back to a static origin serving a
// correctly-signed seq=3 gets a **fresh** floor on the static consumer
// and accepts the rollback without a word. The cheaper road is exactly
// the one a fallback reaches for, and a third party serving a stale root
// is precisely the case the static mode exists to confess.
//
// `consumerFor`'s own doc comment in `workbench/browse_model.go` had
// already named the hazard — *"two consumers for one peer would each hold
// their own seq floor, which is how a floor stops being one"* — and
// keyed defensively around a narrower version of it (a re-based layout
// and a discovered one naming the same peer). The chooser makes the
// two-consumers case deliberate rather than accidental, so the floor has
// to become shareable instead of merely avoided.
//
// **The floor is per PUBLISHER and not per road**, which is what
// §3-RES.4 says: monotonicity is a property of the peer's published-root
// sequence, and the transport that carried it is not part of that
// statement.

// SeqFloor is the highest published-root `seq` accepted for one
// publisher, shared by every [Consumer] reading that publisher.
//
// The zero value is a usable floor that has accepted nothing. Safe for
// concurrent use.
type SeqFloor struct {
	mu    sync.Mutex
	seq   uint64
	known bool
}

// NewSeqFloor is an empty floor for one publisher.
func NewSeqFloor() *SeqFloor { return &SeqFloor{} }

// Accept applies the floor to a root that has ALREADY had its signature
// checked, and raises the floor when the root is newer.
//
// **The caller must not call this before establishing the signer**: a
// rollback is a correctly-signed root being replayed, so refusing on
// `seq` first would be refusing on an unsigned number.
//
// Equal is accepted — a republish of one root is not a rollback — and
// only a strictly higher seq moves the floor.
func (f *SeqFloor) Accept(seq uint64, locator string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.known && seq < f.seq {
		return fmt.Errorf("%w: %s served seq=%d and this session already accepted "+
			"seq=%d from the same publisher — both roots are validly signed, which is what makes "+
			"this a replay rather than a corruption",
			ErrSeqRollback, locator, seq, f.seq)
	}
	if !f.known || seq > f.seq {
		f.seq, f.known = seq, true
	}
	return nil
}

// Seq is the floor's current value and whether anything has set it.
//
// Reported rather than inferred from a zero: **seq 0 is a legal
// published-root sequence**, so "the floor is 0" and "nothing has been
// accepted" are different states and a surface that collapses them would
// claim a floor it does not have.
func (f *SeqFloor) Seq() (uint64, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.seq, f.known
}
