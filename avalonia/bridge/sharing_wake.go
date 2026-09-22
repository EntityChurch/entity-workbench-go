package main

// sharing_wake.go — the tree telling the sharing surfaces that something
// changed, so nothing has to press Refresh.
//
// # What was measured
//
// 2026-09-04, across the whole frontend:
//
//	12 panels, 12 tree subscriptions   — everything else
//	 3 panels,  0 tree subscriptions   — Shared Folders, Sharing Status,
//	                                     Local Files
//
// `share.go` had nine exports and no RegisterWake; `status.go` had four
// and none. SharePanel's only wake was `ConnectionsRegisterWake`, which
// it borrows from the PEER-CONNECTIONS model to keep its discovery list
// live — so the one thing that updated itself on that panel was the one
// thing that was not about sharing.
//
// The state behind all three panels is `app/workbench/folders/`,
// `app/workbench/devices/`, `app/workbench/syncs/` and
// `app/share/records/` — ordinary tree entities that
// `Store.OnPrefixChange` has always been able to watch. Seven models in
// `workbench/` already do exactly this. The sharing feature simply never
// adopted it, and each panel grew a Refresh button instead: individually
// reasonable, and in sum the thing an operator described as clicking
// refresh eight times to watch a share establish.
//
// # Why this hangs off the PEER handle
//
// `StatusRender` and `ShareRender` already take a peer handle and are
// already wake-safe — they read records and observe the substrate, and
// they neither write nor dial. So the reactive fix needs no new handle
// type and no new model: one subscription per peer, fanned out to every
// registered callback.
//
// **Only the READ is wired to the wake.** `StatusReconcile` dials every
// declared device; wiring a pass to a wake or a timer turns a status
// surface into a dialer an operator leaves running overnight, and the
// reconciler writes to the tree, so it would also wake itself forever.
// That distinction is the whole reason Render and Reconcile are separate
// exports and it must not be eroded here.

/*
#include <stdlib.h>
#include <stdint.h>

// Local copy of invoke_tree_wake — see site.go for the same pattern.
static inline void invoke_tree_wake_sharing(void* cb, int64_t handle) {
    if (cb != NULL) {
        ((void(*)(int64_t))cb)(handle);
    }
}
*/
import "C"

import (
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"

	wb "entity-workbench-go/workbench"
)

// sharingWakeSub is one registered callback.
type sharingWakeSub struct {
	id     int64
	cb     unsafe.Pointer
	wakeCh chan struct{}
	doneCh chan struct{}
}

// sharingWakeHub is the single declaration subscription for one peer,
// plus every callback registered against it.
//
// One watcher per PEER rather than per registration: three panels on the
// same peer would otherwise hold three subscriptions to the same four
// prefixes and each deliver the same event, which is three times the
// work to produce one redraw.
type sharingWakeHub struct {
	watcher *wb.DeclarationWatcher
	subs    map[int64]*sharingWakeSub
}

var (
	sharingWakeMu      sync.Mutex
	sharingWakeHubs    = map[int64]*sharingWakeHub{}
	sharingWakeCounter int64
)

// SharingRegisterWake asks to be told when this peer's declared sharing
// state changes: devices, folders, sync bindings, or our own offers.
//
// Returns a registration id for SharingUnregisterWake. The callback is
// invoked with the PEER handle, because that is what the Render exports
// take — a panel receiving it calls StatusRender or ShareRender with the
// same value it already holds.
//
// It is NOT told what changed. Every consumer re-reads the whole
// declared state anyway, so a diff would be a second representation of
// the same facts that can disagree with the first. Spurious wakes are
// expected (each prefix delivers a seed at attach) and a consumer that
// re-reads on one is correct.
//
//export SharingRegisterWake
func SharingRegisterWake(peerHandle C.int64_t, cb unsafe.Pointer) (result *C.char) {
	defer recoverToErrorEnvelope("SharingRegisterWake", &result)
	_, hp, errEnv := shareWorkspace(peerHandle)
	if errEnv != "" {
		return C.CString(errEnv)
	}

	handleID := int64(peerHandle)
	sub := &sharingWakeSub{
		id:     atomic.AddInt64(&sharingWakeCounter, 1),
		cb:     cb,
		wakeCh: make(chan struct{}, 1),
		doneCh: make(chan struct{}),
	}

	sharingWakeMu.Lock()
	hub, existed := sharingWakeHubs[handleID]
	if !existed {
		hub = &sharingWakeHub{subs: map[int64]*sharingWakeSub{}}
		sharingWakeHubs[handleID] = hub
	}
	hub.subs[sub.id] = sub
	sharingWakeMu.Unlock()

	// Attach the store subscription OUTSIDE our lock. OnPrefixChange
	// delivers its seed synchronously on the calling goroutine when the
	// store has no watch hub, and the callback takes this same mutex —
	// attaching under it is AP60's deadlock, which is silent.
	if !existed {
		// Sharing state, NOT declarations alone: these panels display
		// file counts over `local/files/{root}/` and a mount's target
		// prefix, and a declaration-only watcher leaves those numbers
		// frozen while the rows beside them update. See
		// workbench.ObservedPrefixes.
		watcher := wb.WatchSharingState(hp.AppPeer.Store(), func() {
			fanOutSharingWake(handleID)
		})
		sharingWakeMu.Lock()
		// A concurrent Close may have removed the hub while we attached.
		if live, ok := sharingWakeHubs[handleID]; ok && live == hub {
			hub.watcher = watcher
			watcher = nil
		}
		sharingWakeMu.Unlock()
		if watcher != nil {
			watcher.Close()
		}
	}

	// One goroutine per registration, coalescing on a depth-1 channel:
	// a burst of tree events becomes one redraw rather than a queue of
	// them that outlives the burst.
	go func() {
		for {
			select {
			case <-sub.doneCh:
				return
			case <-sub.wakeCh:
				C.invoke_tree_wake_sharing(sub.cb, C.int64_t(handleID))
			}
		}
	}()

	return C.CString(fmt.Sprintf(`{"ok":true,"registration":%d}`, sub.id))
}

// SharingUnregisterWake drops one registration and, when it was the
// last, the peer's subscription with it.
//
//export SharingUnregisterWake
func SharingUnregisterWake(peerHandle C.int64_t, registration C.int64_t) (result *C.char) {
	defer recoverToErrorEnvelope("SharingUnregisterWake", &result)
	handleID := int64(peerHandle)
	regID := int64(registration)

	sharingWakeMu.Lock()
	hub, ok := sharingWakeHubs[handleID]
	if !ok {
		sharingWakeMu.Unlock()
		return C.CString(`{"ok":true}`)
	}
	sub := hub.subs[regID]
	delete(hub.subs, regID)
	var watcher *wb.DeclarationWatcher
	if len(hub.subs) == 0 {
		watcher = hub.watcher
		delete(sharingWakeHubs, handleID)
	}
	sharingWakeMu.Unlock()

	if sub != nil {
		close(sub.doneCh)
	}
	// Outside the lock: Close waits for the delivery goroutine, which
	// may be in fanOutSharingWake taking this mutex (AP60).
	if watcher != nil {
		watcher.Close()
	}
	return C.CString(`{"ok":true}`)
}

// fanOutSharingWake signals every registration for one peer.
//
// Runs on an SDK-owned goroutine. It takes the mutex only to COPY the
// subscriber list, and signals outside it — invoking a .NET callback
// while holding a lock that Unregister also takes is a deadlock between
// the UI thread and the store's deliverer.
func fanOutSharingWake(handleID int64) {
	sharingWakeMu.Lock()
	hub, ok := sharingWakeHubs[handleID]
	if !ok {
		sharingWakeMu.Unlock()
		return
	}
	subs := make([]*sharingWakeSub, 0, len(hub.subs))
	for _, s := range hub.subs {
		subs = append(subs, s)
	}
	sharingWakeMu.Unlock()

	for _, s := range subs {
		select {
		case s.wakeCh <- struct{}{}:
		default: // already pending; coalesce
		}
	}
}
