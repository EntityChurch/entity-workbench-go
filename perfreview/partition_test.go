//go:build perfreview

package perfreview

// Transient network-partition probe — Stage 5 follow-up.
//
// Sits between Stage 4 baseline (always-connected) and the Stage 5
// restart probe (full peer close + reopen). What happens when the
// connection between two peers transiently drops but BOTH peers keep
// running and the subscription entity + inbox handler remain
// registered?
//
// What needs to be true for production-readiness:
//   - On reconnect, subscription resumes without explicit re-subscribe
//     (since the subscription entity + inbox handler both persist).
//   - Writes that arrive at the publisher during the partition are
//     either delivered after reconnect, OR the substrate surfaces a
//     gap signal so the subscriber can decide to catch up.
//
// What we expect to observe (based on Stage 4 + Stage 5 priors):
//   - Subscription DOES resume on reconnect (the binding state is
//     intact on both sides).
//   - Writes during the partition: delivery attempts during partition
//     likely fail at the wire; what does the engine do — retry,
//     queue, or drop? This is the load-bearing question.
//
// If deliveries silently drop during partition: same Finding 3 shape
// as the saturation probe, now with partition as the trigger.
// If they queue and replay: positive finding — but bounded by engine
// queue depth.

import (
	"context"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"

	"entity-workbench-go/entitysdk"
)

// TestPartition_TransientDisconnect probes the partition-heal cycle.
//
// 6-phase scenario:
//  1. alice + bob connect; bob subscribes to alice's `watched/*`.
//  2. Batch 1 (50 writes) — bob receives all.
//  3. Close all connections (both alice's view of bob + bob's view
//     of alice). Both peers still running.
//  4. Batch 2 (50 writes) — alice's engine attempts delivery to a
//     now-disconnected bob.
//  5. Reconnect both directions.
//  6. Batch 3 (50 writes) — does delivery flow now? And does the
//     partition-period batch 2 ever arrive?
func TestPartition_TransientDisconnect(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	alice, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		Storage:    entitysdk.StorageConfig{Kind: "sqlite", Path: filepath.Join(dir, "alice.db")},
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("alice: %v", err)
	}
	defer alice.Close()
	bringUpListenerProbe(t, ctx, alice, "alice")

	bob, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		Storage:    entitysdk.StorageConfig{Kind: "sqlite", Path: filepath.Join(dir, "bob.db")},
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("bob: %v", err)
	}
	defer bob.Close()
	bringUpListenerProbe(t, ctx, bob, "bob")

	if _, err := bob.Connect(ctx, alice.Addr().String()); err != nil {
		t.Fatalf("bob→alice: %v", err)
	}
	if _, err := alice.Connect(ctx, bob.Addr().String()); err != nil {
		t.Fatalf("alice→bob: %v", err)
	}

	sub, err := bob.SubscribeAt(alice.PeerID(), "watched/*", entitysdk.SubscribeOpts{})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Close()

	var delivered atomic.Int64
	go func() {
		for range sub.Events() {
			delivered.Add(1)
		}
	}()

	// --- Phase 1: batch 1 with active connections ---
	const batchSize = 50
	for i := 0; i < batchSize; i++ {
		path := fmt.Sprintf("watched/p1-%03d", i)
		if _, err := alice.Put(path, "test/note", i); err != nil {
			t.Fatalf("p1 put: %v", err)
		}
	}
	time.Sleep(2 * time.Second)
	p1Delivered := delivered.Load()
	t.Logf("phase1: delivered=%d/%d", p1Delivered, batchSize)
	if p1Delivered < int64(batchSize-2) {
		t.Errorf("phase1: pre-partition delivery loss (%d/%d) — saturation, not partition", p1Delivered, batchSize)
	}

	// --- Phase 2: partition — close all connections between alice and bob ---
	// Closing connections from one side closes both endpoints (TCP). We
	// close from both sides for symmetry.
	for _, c := range alice.RawPeer().Connections() {
		_ = c.Close()
	}
	for _, c := range bob.RawPeer().Connections() {
		_ = c.Close()
	}
	t.Logf("phase2: partition established (closed alice's %d + bob's %d connections)",
		len(alice.RawPeer().Connections()), len(bob.RawPeer().Connections()))
	// Brief settle so the close propagates.
	time.Sleep(200 * time.Millisecond)

	// --- Phase 3: writes during partition ---
	for i := 0; i < batchSize; i++ {
		path := fmt.Sprintf("watched/p2-%03d", i)
		if _, err := alice.Put(path, "test/note", i); err != nil {
			t.Fatalf("p2 put: %v", err)
		}
	}
	time.Sleep(2 * time.Second)
	p2Delivered := delivered.Load() - p1Delivered
	t.Logf("phase3: during-partition delivered=%d/%d (cumulative=%d)", p2Delivered, batchSize, delivered.Load())

	// --- Phase 4: heal partition ---
	if _, err := bob.Connect(ctx, alice.Addr().String()); err != nil {
		t.Fatalf("phase4 bob→alice: %v", err)
	}
	if _, err := alice.Connect(ctx, bob.Addr().String()); err != nil {
		t.Fatalf("phase4 alice→bob: %v", err)
	}
	t.Logf("phase4: connections re-established")

	// --- Phase 5: post-heal drain — do batch 2 writes show up now? ---
	time.Sleep(3 * time.Second)
	postHealCumulative := delivered.Load()
	p2RecoveredDelta := postHealCumulative - (p1Delivered + p2Delivered)
	t.Logf("phase5 post-heal drain: cumulative=%d (recovered %d batch-2 writes via reconnect)",
		postHealCumulative, p2RecoveredDelta)

	// --- Phase 6: new writes post-heal ---
	preP3 := delivered.Load()
	for i := 0; i < batchSize; i++ {
		path := fmt.Sprintf("watched/p3-%03d", i)
		if _, err := alice.Put(path, "test/note", i); err != nil {
			t.Fatalf("p3 put: %v", err)
		}
	}
	time.Sleep(2 * time.Second)
	p3Delivered := delivered.Load() - preP3
	t.Logf("phase6 post-heal new writes: delivered=%d/%d", p3Delivered, batchSize)

	// --- Summary ---
	totalSent := 3 * batchSize
	totalDelivered := delivered.Load()
	t.Logf("\nSUMMARY:")
	t.Logf("  total sent:      %d", totalSent)
	t.Logf("  total delivered: %d", totalDelivered)
	t.Logf("  pre-partition:   %d / %d delivered", p1Delivered, batchSize)
	t.Logf("  during partition (no recovery): %d / %d delivered", p2Delivered, batchSize)
	t.Logf("  post-heal recovery of partition writes: %d", p2RecoveredDelta)
	t.Logf("  post-heal new writes: %d / %d delivered", p3Delivered, batchSize)

	// Production-readiness checks. The load-bearing observation is
	// p3Delivered: does subscription resume after reconnect WITHOUT
	// requiring explicit re-subscribe?
	if p3Delivered < int64(batchSize-2) {
		t.Errorf("FINDING: subscription did NOT auto-resume after partition heal (p3 delivered %d of %d)",
			p3Delivered, batchSize)
	} else {
		t.Logf("OBSERVATION: subscription DID auto-resume after partition heal — connection-level recovery is transparent to subscription layer")
	}

	// Partition-period recovery: this is the more interesting finding.
	// If p2RecoveredDelta + p2Delivered >= batchSize, the engine
	// successfully queued during partition. If less, those writes are
	// silently lost.
	totalP2Recovered := p2Delivered + p2RecoveredDelta
	if totalP2Recovered < int64(batchSize-2) {
		t.Logf("FINDING: %d of %d batch-2 writes silently lost during partition (consistent with Stage 5 F3: no implicit catch-up; partition = transient version of restart)",
			int64(batchSize)-totalP2Recovered, batchSize)
	} else {
		t.Logf("OBSERVATION: all batch-2 writes recovered post-heal — engine queues during partition")
	}
}

// bringUpListenerProbe is the shared listener-bringup helper for
// partition + adjacent probes. Named with `Probe` suffix to avoid
// collision with other tests' helpers in this package.
func bringUpListenerProbe(t *testing.T, ctx context.Context, ap *entitysdk.AppPeer, name string) {
	t.Helper()
	ready := make(chan struct{})
	errCh := make(chan error, 1)
	go func() { errCh <- ap.ListenReady(ctx, ready) }()
	select {
	case <-ready:
	case err := <-errCh:
		t.Fatalf("%s listen: %v", name, err)
	case <-time.After(5 * time.Second):
		t.Fatalf("%s listen timeout", name)
	}
}
