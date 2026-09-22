package main

import "sync/atomic"

// wake_pump.go — A CANCEL THAT DOES NOT WAIT IS NOT A CANCEL, AND HERE IT
// ABORTS THE PROCESS.
//
// # The crash
//
// Every wake surface in this bridge had the same loop, written ten times:
//
//	go func() {
//	    for {
//	        select {
//	        case <-h.doneCh:
//	            return
//	        case <-h.wakeCh:
//	            C.invoke_tree_wake_x(cb, handle)   // calls into .NET
//	        }
//	    }
//	}()
//
// …and an unregister export that did `close(h.doneCh)` and **returned
// immediately**. On the far side of that P/Invoke, the panel's detach
// handler then does `_wakeHandle.Free()` — which is the only thing
// keeping the .NET delegate alive.
//
// Two races, both real:
//
//  1. **Both channels ready.** Go's `select` picks uniformly at random
//     among ready cases, so a pending wake can be chosen in the same
//     round the done channel closes, and the callback fires after the
//     unregister returned.
//  2. **Already inside the call.** The pump can be mid-`invoke` when
//     unregister returns and the caller frees the handle, so the call in
//     flight lands on a delegate that has just been collected.
//
// The result is not an exception a panel could catch. It is
// `Process terminated. A callback was made on a garbage collected
// delegate` — the runtime aborts, and every panel, every peer, and any
// unsaved state goes with it.
//
// # How it was found, which is the part worth keeping
//
// `make -C avalonia test` **aborts partway through and has been aborting**:
// `Passed: 160 … Total tests: Unknown … Test Run Aborted`. That is AP15's
// shape at the level of a whole suite — a pass count with no denominator
// is a lower bound, and an unknown number of GUI tests at the tail had
// simply not been running. The abort was read as noise because the number
// before it was large and green.
//
// # The fix
//
// One pump, with the two properties the hand-written loops lacked:
//
//   - **it re-checks `done` after taking a wake**, so a cancelled pump
//     cannot invoke even when both channels were ready;
//   - **`stop()` WAITS for the goroutine to exit** before returning, so a
//     caller that frees a delegate the instant the export returns is
//     safe by construction.
//
// This is AP60 (*"a cancel that waits is the whole point"*) inverted:
// there the cancel waited and deadlocked because it was called under a
// lock; here it did not wait at all. Both fail silently in their own
// direction, and the rule that covers both is: **call stop() OUTSIDE any
// mutex the callback path takes, and let it block.**

// wakeChans is the channel triple every wake surface embeds.
//
// Embedded rather than passed, so existing `h.wakeCh` / `h.doneCh` field
// accesses keep working and a site that is converted half way does not
// compile — which is what stops this becoming nine slightly different
// fixes again.
type wakeChans struct {
	// wakeCh is depth-1 and coalescing: a burst of tree events becomes
	// one redraw rather than a queue of them that outlives the burst.
	wakeCh chan struct{}
	// doneCh is closed by stop().
	doneCh chan struct{}
	// exited is closed by the pump as it returns. This is the channel
	// whose absence was the bug: without something to wait ON, a cancel
	// can only ever be a request.
	exited chan struct{}
	// started is set by run(). **stop() must not wait unless a pump was
	// actually started**, and getting this wrong turns the crash into a
	// hang, which is not an improvement.
	//
	// Every handle in this bridge allocates its channels at construction,
	// but a wake is registered LATER and often not at all — a panel that
	// opens a handle, reads once and closes it never calls run(). Waiting
	// on `exited` there blocks forever, on the UI thread, inside a
	// P/Invoke: the window simply stops. Measured the first time this
	// pump shipped — the headless suite went from ABORTING at test 160 to
	// HANGING after it, which is a worse failure wearing a better number.
	//
	// A pointer because wakeChans is embedded BY VALUE and its methods
	// take value receivers, so a plain bool would be set on a copy and
	// read as false by every caller.
	started *atomic.Bool
}

// newWakeChans builds the triple.
func newWakeChans() wakeChans {
	return wakeChans{
		wakeCh:  make(chan struct{}, 1),
		doneCh:  make(chan struct{}),
		exited:  make(chan struct{}),
		started: new(atomic.Bool),
	}
}

// signal requests one wake, coalescing when one is already pending.
//
// Never blocks and never fails: a dropped signal is correct here, because
// the consumer re-reads whole state on any wake and two wakes with no
// read between them are indistinguishable from one.
func (w wakeChans) signal() {
	select {
	case w.wakeCh <- struct{}{}:
	default:
	}
}

// run starts the pump. invoke is called on a Go-owned goroutine and must
// not assume a UI thread.
//
// The `default:` re-check after a wake is load-bearing, not defensive: it
// is what closes race (1) above, where `select` may pick a ready wake in
// the same round done became ready.
func (w wakeChans) run(invoke func()) {
	if w.started != nil {
		w.started.Store(true)
	}
	go func() {
		defer close(w.exited)
		for {
			select {
			case <-w.doneCh:
				return
			case <-w.wakeCh:
				// Cancelled between the signal and here? Then the caller
				// may already have freed the delegate this would call.
				select {
				case <-w.doneCh:
					return
				default:
				}
				invoke()
			}
		}
	}()
}

// stop cancels the pump and BLOCKS until it has exited.
//
// **Call it outside every mutex the callback path takes.** The pump may
// be inside `invoke` — i.e. inside .NET — and that call can re-enter this
// bridge; waiting for it while holding a lock it needs is AP60's deadlock,
// which presents as a frozen window rather than as an error.
//
// Idempotent: calling it twice is safe, which matters because a panel's
// detach can run more than once.
func (w wakeChans) stop() {
	if w.doneCh == nil {
		return
	}
	select {
	case <-w.doneCh: // already stopped
	default:
		close(w.doneCh)
	}
	// Only wait for a pump that was actually started — see `started`.
	if w.exited != nil && w.started != nil && w.started.Load() {
		<-w.exited
	}
}
