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

// LocalFilesSourcePrefix is the watcher's source layer — every admitted
// file under every mount on this peer.
const LocalFilesSourcePrefix = "local/files/"

// ObservedPrefixes are the tree locations a sharing surface COUNTS as
// opposed to DECLARES, and they are the half this file originally
// missed.
//
// # The bug this closes, because it is the one the warning above
// predicted verbatim
//
// The comment on [DeclarationPrefixes] says a watcher that misses a
// prefix "produces a surface that is live for some changes and stale for
// others — which is worse than a Refresh button, since nothing tells the
// operator which kind they are looking at." That is exactly what shipped.
// [FolderStatus.FilesPresent] and [FolderStatus.FilesIngested] are counts
// over `local/files/{root}/` and a mount's target prefix — **neither of
// which is a declaration**, so no wake ever fired when a file arrived.
// The declared rows updated themselves and the numbers beside them did
// not.
//
// Measured: mount a directory of seven files and the panels report 1, or
// 2, or whatever the count happened to be at the instant the panel last
// read — while a second panel driven by an operator-pressed sweep reports
// 7. Two surfaces, two different numbers, both correct at the moment they
// were taken and neither labelled with when that was. The operator's
// report was "one on disc here, seven on disc there."
//
// # Why the set is computed and not constant
//
// A mount's TARGET prefix is chosen by the operator (`archives/photos/`,
// or anything else), so there is no fixed parent to watch. The source
// layer has one, and the two mount namespaces are watched so that
// creating or removing a mount is itself a wake — which is also what
// tells a caller its target set is out of date.
func ObservedPrefixes(st *Store) []string {
	out := []string{LocalFilesSourcePrefix, MountConfigPrefix, MountBindingPrefix}
	if st == nil {
		return out
	}
	bindings, _ := LoadMountBindings(st)
	seen := map[string]bool{}
	for _, b := range bindings {
		if b.TargetPrefix == "" || seen[b.TargetPrefix] {
			continue
		}
		seen[b.TargetPrefix] = true
		out = append(out, b.TargetPrefix)
	}
	return out
}

// SharingWatchPrefixes is everything a sharing surface displays: what
// this peer has DECLARED plus what it can OBSERVE about the result.
//
// One list because the distinction matters to a reader and not to a
// subscriber — a panel re-reads the whole state on any wake, so the only
// thing that can go wrong is a prefix being absent.
func SharingWatchPrefixes(st *Store) []string {
	return append(DeclarationPrefixes(), ObservedPrefixes(st)...)
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
	return WatchPrefixes(st, DeclarationPrefixes(), notify)
}

// WatchSharingState subscribes to everything a sharing surface displays —
// declarations AND the observed layers those surfaces count
// ([SharingWatchPrefixes]).
//
// **This is what a panel wants; [WatchDeclarations] is not.** A panel
// that watches declarations alone updates its rows and freezes its
// numbers, which is the failure [ObservedPrefixes] documents.
func WatchSharingState(st *Store, notify func()) *DeclarationWatcher {
	return WatchPrefixes(st, SharingWatchPrefixes(st), notify)
}

// WatchPrefixes subscribes to an explicit prefix list. The prefix set is
// a parameter rather than a constant because a mount's target prefix is
// operator-chosen and only knowable from the store.
func WatchPrefixes(st *Store, prefixes []string, notify func()) *DeclarationWatcher {
	w := &DeclarationWatcher{notify: notify}
	if st == nil || notify == nil {
		return w
	}
	// Subscribe OUTSIDE any lock of ours. Store.OnPrefixChange delivers
	// its seed SYNCHRONOUSLY on the calling goroutine when the store has
	// no watch hub, so attaching under the lock deadlocks on the first
	// seeded event — a hang with no panic and no race report (AP60).
	cancels := make([]func(), 0, len(prefixes))
	for _, prefix := range prefixes {
		if prefix == "" {
			continue
		}
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
