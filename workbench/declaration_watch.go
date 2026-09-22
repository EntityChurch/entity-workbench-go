package workbench

// declaration_watch.go — the tree telling a surface that the declared
// sharing state changed, so nothing has to press Refresh.
//
// # Why this file exists
//
// Measured 2026-09-04, across the whole Avalonia frontend:
//
//	every panel except three   12 panels, 12 tree subscriptions
//	the three sharing panels    3 panels,  0 tree subscriptions
//
// `avalonia/bridge/share.go` had nine exports and no RegisterWake;
// `status.go` had four and none. SharePanel's only wake was one it
// borrowed from the PEER-CONNECTIONS model to keep its discovery list
// live. So the one job in the product whose whole state lives in
// watchable tree entities — `app/workbench/folders/`,
// `app/workbench/devices/`, `app/share/records/` — was the one job an
// operator had to refresh by hand, on five surfaces, while every other
// panel updated itself.
//
// It was not that we lacked the mechanism. Seven models in this package
// use OnPrefixChange. The sharing feature simply never adopted it, and
// each panel grew a Refresh button instead — locally reasonable, and in
// sum the thing an operator described as clicking refresh eight times.
//
// # What it deliberately does NOT do
//
// It does not carry WHAT changed. Every consumer re-reads the whole
// declared state anyway (it is a handful of entities, and the reconciler
// already reads all of them per pass), so a diff would be a second
// representation of the same facts that can disagree with the first.
// The signal is "something moved; read again".
//
// It also cannot see a peer's OFFERS to us: those live in THEIR tree and
// reaching them is a dispatched remote read (AP11). A surface that wants
// them fresh has to ask, and must say when it last asked rather than
// implying they are live.

import (
	"sync"
)

// DeclarationPrefixes are the tree locations that hold this peer's
// declared sharing state. Named once, here, because a watcher that
// misses one produces a surface that is live for some changes and stale
// for others — which is worse than a Refresh button, since nothing tells
// the operator which kind they are looking at.
func DeclarationPrefixes() []string {
	return []string{DevicePrefix, FolderPrefix, ShareOfferPrefix, SyncBindingPrefix}
}

// DeclarationWatcher fires a callback when any declared sharing state
// changes. Safe for concurrent use; Close is idempotent.
type DeclarationWatcher struct {
	mu      sync.Mutex
	cancels []func()
	closed  bool
	notify  func()
}

// WatchDeclarations subscribes to every declaration prefix and calls
// notify on any change.
//
// notify runs on an SDK-owned goroutine and MUST NOT touch a renderer —
// it exists to set a flag or signal a channel that a UI thread drains.
// It may also be called spuriously (each prefix delivers a seed at
// attach), which is deliberate: a consumer that re-reads on a spurious
// wake is correct, and one that assumes a wake means a specific change
// is not.
func WatchDeclarations(st *Store, notify func()) *DeclarationWatcher {
	w := &DeclarationWatcher{notify: notify}
	if st == nil || notify == nil {
		return w
	}
	// Subscribe OUTSIDE any lock of ours. Store.OnPrefixChange delivers
	// its seed SYNCHRONOUSLY on the calling goroutine when the store has
	// no watch hub, so attaching under the lock deadlocks on the first
	// seeded event — a hang with no panic and no race report (AP60).
	cancels := make([]func(), 0, len(DeclarationPrefixes()))
	for _, prefix := range DeclarationPrefixes() {
		cancels = append(cancels, st.OnPrefixChange(prefix, func(ChangeEvent) { w.fire() }))
	}

	w.mu.Lock()
	if w.closed {
		// Closed while we were attaching. Undo outside the lock below.
		w.mu.Unlock()
		for _, c := range cancels {
			if c != nil {
				c()
			}
		}
		return w
	}
	w.cancels = cancels
	w.mu.Unlock()
	return w
}

func (w *DeclarationWatcher) fire() {
	w.mu.Lock()
	notify, closed := w.notify, w.closed
	w.mu.Unlock()
	if closed || notify == nil {
		return
	}
	notify()
}

// Close detaches every subscription.
//
// The cancels are taken under the lock and CALLED OUTSIDE it. A cancel
// waits for its delivery goroutine to exit, and that goroutine may be
// inside fire() taking this same mutex — so cancelling under the lock
// deadlocks silently. That is AP60 exactly, and it once sat on the
// workbench suite for sixteen minutes with no panic and no race report.
func (w *DeclarationWatcher) Close() {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	cancels := w.cancels
	w.cancels = nil
	w.notify = nil
	w.mu.Unlock()

	for _, c := range cancels {
		if c != nil {
			c()
		}
	}
}
