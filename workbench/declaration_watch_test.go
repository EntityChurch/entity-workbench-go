package workbench

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// declaration_watch_test.go — tier: model (TESTING-STRATEGY §2).
//
// The thing being gated is that the sharing surfaces no longer need a
// Refresh button, so the assertion has to be that a WRITE produces a
// WAKE. Asserting that WatchDeclarations returns a non-nil watcher would
// pass against a watcher that never fires, which is the version this
// test exists to prevent.

func TestWatchDeclarations_WakesOnEveryDeclarationPrefix(t *testing.T) {
	for _, tc := range []struct {
		name  string
		write func(st *Store) error
	}{
		{"a device", func(st *Store) error {
			return SaveDevice(st, DeviceData{PeerID: "2KLf7osYcMLEdmSnYLx3Bg6TScaGJefLZJdKaL1tPNHAbR"})
		}},
		{"a folder", func(st *Store) error {
			return SaveFolder(st, FolderData{ID: "p.photos", Root: "photos", Origin: "local"})
		}},
		{"an offer", func(st *Store) error {
			return SaveShareOffer(st, ShareOffer{
				Root: "photos", Title: "photos", TargetPrefix: "archives/photos/",
			})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := liveStore(t)

			var woke atomic.Int64
			w := WatchDeclarations(st, func() { woke.Add(1) })
			defer w.Close()

			// The attach seed may already have fired; measure from here.
			before := woke.Load()
			if err := tc.write(st); err != nil {
				t.Fatalf("write: %v", err)
			}
			if !waitFor(func() bool { return woke.Load() > before }) {
				t.Errorf("writing %s produced no wake — the surface over this prefix "+
					"still needs a Refresh button", tc.name)
			}
		})
	}
}

// TestWatchDeclarations_CloseUnderChurnDoesNotDeadlock is the AP60 gate.
//
// Store.OnPrefixChange's cancel waits for its delivery goroutine to
// exit, and that goroutine is inside the callback. A Close that holds
// the watcher's own mutex across the cancel therefore deadlocks with no
// panic and no race report — it presents as a suite that sits there,
// which is how it cost sixteen minutes on the workbench suite once
// already.
//
// LOOPED, because one close under churn does not reliably catch the
// deliverer mid-contention: the first version of the equivalent gate
// elsewhere in this repo passed against the deadlocking code.
func TestWatchDeclarations_CloseUnderChurnDoesNotDeadlock(t *testing.T) {
	for i := 0; i < 40; i++ {
		st := liveStore(t)
		w := WatchDeclarations(st, func() {
			// Contend for the watcher's lock from the delivery
			// goroutine, which is what makes the deadlock reachable.
			time.Sleep(time.Microsecond)
		})

		var wg sync.WaitGroup
		stop := make(chan struct{})
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; ; n++ {
				select {
				case <-stop:
					return
				default:
				}
				_ = SaveDevice(st, DeviceData{
					PeerID:        "2KLf7osYcMLEdmSnYLx3Bg6TScaGJefLZJdKaL1tPNHAbR",
					AddedAtMillis: uint64(n),
				})
			}
		}()

		done := make(chan struct{})
		go func() { w.Close(); close(done) }()

		select {
		case <-done:
		case <-time.After(20 * time.Second):
			t.Fatalf("iteration %d: Close deadlocked under churn (AP60)", i)
		}
		close(stop)
		wg.Wait()
	}
}

// TestWatchDeclarations_CloseIsIdempotent — Close runs from a panel's
// teardown, which in this frontend can happen more than once.
func TestWatchDeclarations_CloseIsIdempotent(t *testing.T) {
	st := liveStore(t)
	w := WatchDeclarations(st, func() {})
	w.Close()
	w.Close()
}

func waitFor(cond func() bool) bool {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return cond()
}
