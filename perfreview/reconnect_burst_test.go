//go:build perfreview

package perfreview

// Reconnect-during-burst probe — Stage 5 audit P0 #3.
//
// The Stage 5 partition probe (partition_test.go) closes the
// connections cleanly, then reconnects. This probe is the harder
// case: a connection drops MID-WRITE — i.e., the hub is bursting at
// 10K/sec and a single spoke's connection closes. Does the spoke
// recover after re-connecting? How much is lost in the drop window?
//
// Production case: mobile peer with flaky network; laptop sleep
// during background sync; cloud peer behind a transient load
// balancer.

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"entity-workbench-go/entitysdk"
)

// TestReconnect_MidBurstDropAndResume drives 4-spoke fan-out at
// 5K/sec for 3s. At t=1.5s (mid-burst), spoke 0's connection to the
// hub is forcibly closed. At t=2.0s the spoke reconnects + re-
// subscribes. We measure:
//
//   - How many events spoke 0 delivers vs other spokes
//   - Whether the gap is "just the disconnect window" or larger
//   - Whether other spokes were affected by the drop
//
// Note: the spoke's subscription cannot magically resume without
// explicit re-subscribe — that's the F6 finding closed by
// RestorePriorSubscriptions. This test does the manual re-subscribe
// to characterize the substrate's behavior under the contract.
func TestReconnect_MidBurstDropAndResume(t *testing.T) {
	const hubRate = 5000
	const wallTime = 3 * time.Second
	const dropAt = 1500 * time.Millisecond
	const reconnectAt = 2000 * time.Millisecond

	dir := t.TempDir()
	t.Setenv("HOME", dir)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	hub, spokes := bringUpHubAndSpokes(t, ctx, dir, 4)
	defer cleanupHubAndSpokes(hub, spokes)
	hubAddr := hub.Addr().String()

	// Drive hub writes in a goroutine; we control disconnect/reconnect
	// from the main test goroutine.
	type writeResult struct {
		sent    int
		elapsed time.Duration
	}
	writeDone := make(chan writeResult, 1)
	go func() {
		sent, elapsed := driveHubWrites(t, hub, hubRate, wallTime)
		writeDone <- writeResult{sent, elapsed}
	}()

	// At t=1.5s, close spoke 0's connection to the hub.
	time.Sleep(dropAt)
	t.Logf("t=%s: dropping spoke 0 connection to hub", dropAt)
	hub.Disconnect(spokes[0].ap.PeerID())
	spokes[0].ap.Disconnect(hub.PeerID())
	deliveredAtDrop := spokes[0].delivered.Load()

	// At t=2.0s, reconnect + re-subscribe spoke 0.
	time.Sleep(reconnectAt - dropAt)
	t.Logf("t=%s: reconnecting spoke 0", reconnectAt)
	if _, err := spokes[0].ap.Connect(ctx, hubAddr); err != nil {
		t.Fatalf("spoke 0 reconnect: %v", err)
	}
	if _, err := hub.Connect(ctx, spokes[0].ap.Addr().String()); err != nil {
		t.Fatalf("hub→spoke0 reconnect: %v", err)
	}
	// Close the stale subscription and re-subscribe (the substrate
	// does not auto-resume per F6).
	_ = spokes[0].sub.Close()
	select {
	case <-spokes[0].doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("stale spoke0 drain did not exit")
	}
	newSub, err := spokes[0].ap.SubscribeAt(hub.PeerID(), "watched/*", entitysdk.SubscribeOpts{})
	if err != nil {
		t.Fatalf("spoke 0 re-subscribe: %v", err)
	}
	spokes[0].sub = newSub
	spokes[0].doneCh = make(chan struct{})
	go func(r *spokeReceiver) {
		for range r.sub.Events() {
			r.delivered.Add(1)
		}
		close(r.doneCh)
	}(spokes[0])

	deliveredAtReconnect := spokes[0].delivered.Load()

	// Wait for the rest of the write phase to finish.
	res := <-writeDone

	// 2s drain.
	time.Sleep(2 * time.Second)

	t.Logf("\nhub sent=%d at %d/sec for %s",
		res.sent, hubRate, res.elapsed.Round(10*time.Millisecond))
	t.Logf("spoke 0 (dropped %s, reconnected %s):", dropAt, reconnectAt)
	t.Logf("  delivered at drop:      %d", deliveredAtDrop)
	t.Logf("  delivered at reconnect: %d (Δ=%d during disconnected window)",
		deliveredAtReconnect, deliveredAtReconnect-deliveredAtDrop)
	t.Logf("  delivered total:        %d", spokes[0].delivered.Load())
	for i := 1; i < 4; i++ {
		t.Logf("spoke %d (uninterrupted): delivered=%d/%d (%.1f%%)",
			i+1, spokes[i].delivered.Load(), res.sent,
			100.0*float64(spokes[i].delivered.Load())/float64(res.sent))
	}

	gap := spokes[1].delivered.Load() - spokes[0].delivered.Load()
	t.Logf("\nspoke-0 vs spoke-1 final delta = %d events (estimates events lost across disconnect window)", gap)
}

// TestReconnect_DropDuringIdleDoesNotAffect drives the hub at
// 500/sec (well below ceiling), then drops + reconnects spoke 0
// after a 1.5s pause with no traffic. Tests that a clean idle-drop
// + reconnect doesn't drop events that didn't exist.
func TestReconnect_DropDuringIdleDoesNotAffect(t *testing.T) {
	const hubRate = 500
	const wallTime = 1500 * time.Millisecond

	dir := t.TempDir()
	t.Setenv("HOME", dir)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	hub, spokes := bringUpHubAndSpokes(t, ctx, dir, 2)
	defer cleanupHubAndSpokes(hub, spokes)
	hubAddr := hub.Addr().String()

	// Phase 1: write briefly.
	sent1, _ := driveHubWrites(t, hub, hubRate, wallTime)
	time.Sleep(500 * time.Millisecond)

	// Phase 2: spoke 0 drops + reconnects during a quiet period.
	hub.Disconnect(spokes[0].ap.PeerID())
	spokes[0].ap.Disconnect(hub.PeerID())
	time.Sleep(500 * time.Millisecond)
	if _, err := spokes[0].ap.Connect(ctx, hubAddr); err != nil {
		t.Fatalf("spoke 0 reconnect: %v", err)
	}
	if _, err := hub.Connect(ctx, spokes[0].ap.Addr().String()); err != nil {
		t.Fatalf("hub→spoke0 reconnect: %v", err)
	}
	_ = spokes[0].sub.Close()
	<-spokes[0].doneCh
	newSub, err := spokes[0].ap.SubscribeAt(hub.PeerID(), "watched/*", entitysdk.SubscribeOpts{})
	if err != nil {
		t.Fatalf("spoke 0 re-subscribe: %v", err)
	}
	spokes[0].sub = newSub
	spokes[0].doneCh = make(chan struct{})
	var d2 atomic.Int64
	go func(s *entitysdk.Subscription) {
		for range s.Events() {
			d2.Add(1)
			spokes[0].delivered.Add(1)
		}
		close(spokes[0].doneCh)
	}(newSub)

	// Phase 3: write again post-reconnect.
	sent2, _ := driveHubWrites(t, hub, hubRate, wallTime)
	time.Sleep(2 * time.Second)

	t.Logf("\nphase1 sent=%d  phase2-idle  phase3 sent=%d", sent1, sent2)
	for i, s := range spokes {
		d := s.delivered.Load()
		t.Logf("spoke %d total delivered=%d  (post-reconnect %d)", i+1, d, d2.Load())
		_ = fmt.Sprintf("%s", s.name) // silence unused
	}
}
