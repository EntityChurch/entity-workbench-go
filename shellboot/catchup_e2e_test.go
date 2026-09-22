package shellboot_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"entity-workbench-go/shellcmd"
)

// The catch-up supervisor heals a share that the subscription path
// silently gave up on.
//
// Tier: real-session (TESTING-STRATEGY §4).
//
// # What this reproduces
//
// A burst of file writes saturates the SENDER's subscription delivery
// shards, which drop on full by design (ext/subscription/engine.go). The
// dropped notifications never reach the wire, so the receiver sees no
// error, no retry and no counter move — the copy simply stops part way
// and stays that way. Measured at 2000 files in
// bigcopy_load_test.go: 676 delivered, 2327 dropped, permanent.
//
// This test uses a burst big enough to provoke drops and asserts the
// supervisor closes the gap without anyone running a verb.
//
// # The control arm is the point
//
// WITHOUT the supervisor the same burst must stay incomplete. A
// positive-only test would pass on a run where the burst happened not to
// saturate at all — which is entirely possible on a fast machine — and
// would then be certifying nothing. So the control runs first, and if it
// does NOT stall the test says so and skips rather than claiming a
// result it did not earn.
func TestCatchUp_HealsASilentlyStalledShare(t *testing.T) {
	const burst = 1500

	// --- control arm: no supervisor -------------------------------------
	ctrlArrived, ctrlWant := runBurst(t, burst, 0)
	if ctrlArrived == ctrlWant {
		t.Skipf("the %d-file burst did not saturate delivery on this machine "+
			"(%d/%d arrived without a supervisor), so this run cannot "+
			"distinguish a working supervisor from a burst that never needed "+
			"one — raise the burst size to make it meaningful",
			burst, ctrlArrived, ctrlWant)
	}
	t.Logf("CONTROL: without the supervisor, %d of %d arrived and stayed that way",
		ctrlArrived, ctrlWant)

	// --- the real arm ---------------------------------------------------
	arrived, want := runBurst(t, burst, 2*time.Second)
	if arrived != want {
		t.Errorf("with the catch-up supervisor running, only %d of %d files arrived — "+
			"the supervisor did not close a gap the control arm proves exists",
			arrived, want)
	}
}

// runBurst shares a folder, drops n files into it at once, and reports
// how many reached the receiver. catchUp > 0 runs the supervisor on the
// receiver at that interval.
func runBurst(t *testing.T, n int, catchUp time.Duration) (arrived, want int) {
	t.Helper()

	senderDir := t.TempDir()
	receiverDir := t.TempDir()

	sender := newSyncTestPeer(t, "sender")
	receiver := newCatchUpPeer(t, "receiver", catchUp)
	connectPeers(t, sender, receiver)

	const folder = "burst"
	senderMount := filepath.Join(senderDir, folder)
	receiverMount := filepath.Join(receiverDir, folder)
	mkdirOrFail(t, senderMount)
	mkdirOrFail(t, receiverMount)
	mountOrFail(t, sender, senderMount, "archives/"+folder+"/")
	mountOrFail(t, receiver, receiverMount, "archives/"+folder+"/")

	if _, err := sender.ws.Share(shellcmd.ShareRequest{Root: folder, Peer: receiver.id}); err != nil {
		t.Fatalf("share: %v", err)
	}
	if _, err := receiver.ws.Accept(shellcmd.AcceptRequest{
		Peer: sender.id, Root: folder, Directory: receiverMount, AllowNonEmpty: true,
	}); err != nil {
		t.Fatalf("accept: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if _, err := receiver.ws.Reconcile(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	body := []byte("burst payload\n")
	for i := 0; i < n; i++ {
		p := filepath.Join(senderMount, fmt.Sprintf("b%05d.txt", i))
		if err := os.WriteFile(p, body, 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	// Settle on "no new file for a while", not on a fixed sleep: the
	// point is the STEADY state, and a fixed sleep measures whichever
	// moment it happens to land on.
	arrived = awaitQuiescence(t, receiverMount, n, len(body), 20*time.Second, 3*time.Minute)

	t.Logf("burst=%d catchUp=%v -> %d arrived · sender dropped=%d",
		n, catchUp, arrived, sender.ap.SubscriptionEngine().DroppedDeliveries())
	if catchUp > 0 {
		if res, ok := receiver.ws.LastCatchUp(); ok {
			t.Logf("  last catch-up: %s", res.Summary())
		} else {
			t.Log("  no catch-up pass has completed")
		}
	}
	receiver.ws.StopCatchUp()
	return arrived, n
}

// awaitQuiescence returns the count once it has stopped changing for
// `quiet`, or when everything arrived, or at the deadline.
func awaitQuiescence(t *testing.T, dir string, want, size int, quiet, timeout time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	last, lastChange := -1, time.Now()
	for time.Now().Before(deadline) {
		n := countComplete(dir, size)
		if n >= want {
			return n
		}
		if n != last {
			last, lastChange = n, time.Now()
		}
		if time.Since(lastChange) > quiet {
			return n
		}
		time.Sleep(200 * time.Millisecond)
	}
	return countComplete(dir, size)
}

func newCatchUpPeer(t *testing.T, name string, interval time.Duration) *syncTestPeer {
	t.Helper()
	p := newSyncTestPeer(t, name)
	if interval > 0 {
		p.ws.EnableCatchUp(interval)
		t.Cleanup(p.ws.StopCatchUp)
	}
	return p
}

// countComplete counts files at their full size. Size and not existence:
// local/files:write renames into place, but a partial file from any other
// path would otherwise count as arrived.
func countComplete(dir string, size int) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if info, err := e.Info(); err == nil && info.Size() == int64(size) {
			n++
		}
	}
	return n
}

// The supervisor BACKS OFF when there is nothing to do, and the rate is
// visible.
//
// Tier: real-session (TESTING-STRATEGY §4).
//
// nextCatchUpInterval is unit-tested as a pure function; this asserts the
// loop actually applies it, which is a different claim — a correct rule
// that the goroutine never calls is the shape of defect this repo keeps
// finding (a model with no caller). It runs against a peer with NO sync
// bindings, so every pass is trivially clean and the back-off is the only
// thing under test.
func TestCatchUp_BacksOffWhenThereIsNothingToRecover(t *testing.T) {
	p := newSyncTestPeer(t, "idle")

	// A short base so the back-off is observable in test time. The rule
	// is scale-free; the constants are not what is being asserted.
	p.ws.EnableCatchUp(200 * time.Millisecond)
	t.Cleanup(p.ws.StopCatchUp)

	// Let several passes happen.
	deadline := time.Now().Add(15 * time.Second)
	var grew bool
	var last time.Duration
	for time.Now().Before(deadline) {
		iv, running := p.ws.CatchUpInterval()
		if !running {
			t.Fatal("the supervisor reports itself not running immediately after EnableCatchUp")
		}
		if iv > last {
			last = iv
		}
		if iv >= 800*time.Millisecond {
			grew = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !grew {
		t.Errorf("after repeated clean passes the interval only reached %v — the loop "+
			"is not applying the back-off, so an idle peer keeps paying the busy rate",
			last)
	}
	t.Logf("idle peer backed off to %v", last)

	// And it reports a pass ran, so "backed off" is distinguishable from
	// "never started".
	if _, ok := p.ws.LastCatchUp(); !ok {
		t.Error("no catch-up pass was recorded, so the back-off above proves nothing")
	}

	// Stopping is observable too — a surface that cannot tell a stopped
	// supervisor from a slow one will caption a dead loop as a quiet one.
	p.ws.StopCatchUp()
	if _, running := p.ws.CatchUpInterval(); running {
		t.Error("the supervisor reports itself running after StopCatchUp")
	}
}
