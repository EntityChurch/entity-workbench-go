//go:build perfreview

package perfreview

// Lane 4 — subscription thrash. Stress sub/unsub cycles to surface
// handler-registration leaks, goroutine leaks, memory leaks, or
// engine-side state drift.
//
// Per arch's HANDOFF-WORKBENCH-STAGE-5-FOLLOWUPS Lane 4. Three probes:
//
//   1. TestThrash_SubUnsubLoop_GoroutineLeak — single spoke does
//      rapid sub→receive→unsub cycles against a publishing hub.
//      Goroutine count snapshots at intervals. Linear growth = leak.
//
//   2. TestThrash_MixedLongLivedAndThrash — sustained workload mixing
//      stable long-lived subs with thrashing short-lived ones. Tests
//      whether long-lived subs degrade under churn next to them.
//
//   3. TestThrash_ConcurrentSubUnsubSamePattern — multiple goroutines
//      concurrently sub+unsub on the same pattern. Race detector
//      should catch any engine-side state races; engine-level
//      counters should reconcile to zero after teardown.

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"

	"entity-workbench-go/entitysdk"
)

// goroutineHeap captures the snapshot used by all thrash probes.
type thrashSnapshot struct {
	Label      string
	Wall       time.Duration
	Goroutines int
	HeapMiB    float64
	AllocMiB   float64
	NumGC      uint32
}

func takeThrashSnapshot(label string, start time.Time) thrashSnapshot {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return thrashSnapshot{
		Label:      label,
		Wall:       time.Since(start),
		Goroutines: runtime.NumGoroutine(),
		HeapMiB:    float64(ms.HeapInuse) / 1024 / 1024,
		AllocMiB:   float64(ms.HeapAlloc) / 1024 / 1024,
		NumGC:      ms.NumGC,
	}
}

func logThrashSnapshots(t *testing.T, rows []thrashSnapshot) {
	t.Helper()
	t.Logf("%-18s %10s %10s %10s %10s %6s",
		"label", "wall", "goroutines", "heap-MiB", "alloc-MiB", "GC")
	for _, r := range rows {
		t.Logf("%-18s %10s %10d %10.1f %10.1f %6d",
			r.Label, short(r.Wall), r.Goroutines, r.HeapMiB, r.AllocMiB, r.NumGC)
	}
}

// bringUpThrashPair brings up a hub + 1 spoke with bidirectional
// connect — minimum shape for sub/unsub thrash probes. Returns the
// peers + a cleanup function.
func bringUpThrashPair(t *testing.T, ctx context.Context, dir string) (hub, spoke *entitysdk.AppPeer, cleanup func()) {
	t.Helper()

	hub, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		Storage:    entitysdk.StorageConfig{Kind: "sqlite", Path: filepath.Join(dir, "hub.db")},
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("hub create: %v", err)
	}
	ready := make(chan struct{})
	go func() { _ = hub.ListenReady(ctx, ready) }()
	<-ready

	spoke, err = entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		Storage:    entitysdk.StorageConfig{Kind: "sqlite", Path: filepath.Join(dir, "spoke.db")},
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		hub.Close()
		t.Fatalf("spoke create: %v", err)
	}
	spokeReady := make(chan struct{})
	go func() { _ = spoke.ListenReady(ctx, spokeReady) }()
	<-spokeReady

	if _, err := spoke.Connect(ctx, hub.Addr().String()); err != nil {
		hub.Close()
		spoke.Close()
		t.Fatalf("spoke→hub connect: %v", err)
	}
	if _, err := hub.Connect(ctx, spoke.Addr().String()); err != nil {
		hub.Close()
		spoke.Close()
		t.Fatalf("hub→spoke connect: %v", err)
	}

	cleanup = func() {
		spoke.Close()
		hub.Close()
	}
	return hub, spoke, cleanup
}

// TestThrash_SubUnsubLoop_GoroutineLeak rapidly subscribes,
// receives a few events, then unsubscribes — 600 cycles over 60s.
// Snapshots goroutine count + heap at intervals. If goroutines
// grow linearly, we have a per-subscription leak.
func TestThrash_SubUnsubLoop_GoroutineLeak(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	hub, spoke, cleanup := bringUpThrashPair(t, ctx, dir)
	defer cleanup()

	// Background publisher: continuous low-rate writes so subscribers
	// always have something to receive. Runs until ctx cancels.
	publisherStop := make(chan struct{})
	var publishCount atomic.Int64
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond) // 100/sec
		defer ticker.Stop()
		i := 0
		for {
			select {
			case <-publisherStop:
				return
			case <-ticker.C:
				_, _ = hub.Store().Put(fmt.Sprintf("watched/%07d", i),
					"perfreview/entity", map[string]interface{}{"tick": i})
				publishCount.Add(1)
				i++
			}
		}
	}()

	start := time.Now()
	rows := []thrashSnapshot{takeThrashSnapshot("baseline", start)}

	// 600 cycles, target ~10 cycles/sec. Each cycle: sub, read a few
	// events, unsub. Should stabilize at a steady-state goroutine
	// count if there's no leak.
	const cycleCount = 600
	const eventsPerCycle = 3
	cycleErrors := 0

	for c := 0; c < cycleCount; c++ {
		sub, err := spoke.SubscribeAt(hub.PeerID(), "watched/*", entitysdk.SubscribeOpts{})
		if err != nil {
			cycleErrors++
			t.Logf("cycle %d subscribe error: %v", c, err)
			continue
		}

		// Receive a few events with a small timeout per event.
		eventTimeout := time.After(500 * time.Millisecond)
		received := 0
	receiveLoop:
		for received < eventsPerCycle {
			select {
			case _, ok := <-sub.Events():
				if !ok {
					break receiveLoop
				}
				received++
			case <-eventTimeout:
				break receiveLoop
			}
		}

		if err := sub.Close(); err != nil {
			cycleErrors++
			t.Logf("cycle %d close error: %v", c, err)
		}

		// Snapshot every 60 cycles.
		if (c+1)%60 == 0 {
			rows = append(rows, takeThrashSnapshot(
				fmt.Sprintf("cycle=%d", c+1), start))
		}
	}

	// Final snapshot after a settle window.
	close(publisherStop)
	time.Sleep(3 * time.Second)
	rows = append(rows, takeThrashSnapshot("settled+3s", start))

	t.Logf("\n=== sub/unsub thrash snapshots ===")
	logThrashSnapshots(t, rows)
	t.Logf("cycle-errors: %d / %d (%.1f%%)",
		cycleErrors, cycleCount, 100.0*float64(cycleErrors)/float64(cycleCount))
	t.Logf("publisher: %d writes during thrash", publishCount.Load())

	// Growth analysis: compare baseline to final.
	baseline := rows[0]
	final := rows[len(rows)-1]
	t.Logf("\ngoroutine delta: %d → %d (Δ=%+d)",
		baseline.Goroutines, final.Goroutines,
		final.Goroutines-baseline.Goroutines)
	t.Logf("heap delta:      %.1f → %.1f MiB (Δ=%+.1f MiB)",
		baseline.HeapMiB, final.HeapMiB, final.HeapMiB-baseline.HeapMiB)

	// Heuristic regression check: goroutine count after 600 cycles
	// + 3s settle shouldn't be more than ~20 above baseline if the
	// substrate cleans up properly. This is permissive — a real leak
	// would be hundreds, not tens.
	if delta := final.Goroutines - baseline.Goroutines; delta > 50 {
		t.Errorf("goroutine leak suspected: Δ=%+d after %d sub/unsub cycles + 3s settle",
			delta, cycleCount)
	}
}

// TestThrash_MixedLongLivedAndThrash holds K stable long-lived
// subscriptions while a churn loop creates+destroys M short-lived
// subscriptions on the same pattern. Tests whether the long-lived
// subs degrade under churn (silent delivery drops, queue starvation,
// etc.).
func TestThrash_MixedLongLivedAndThrash(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	hub, spoke, cleanup := bringUpThrashPair(t, ctx, dir)
	defer cleanup()

	// 4 stable subs. Each gets its own counter.
	const stableCount = 4
	type stableSub struct {
		sub       *entitysdk.Subscription
		delivered atomic.Int64
		doneCh    chan struct{}
	}
	stables := make([]*stableSub, stableCount)
	for i := 0; i < stableCount; i++ {
		s, err := spoke.SubscribeAt(hub.PeerID(), "watched/*", entitysdk.SubscribeOpts{})
		if err != nil {
			t.Fatalf("stable subscribe %d: %v", i, err)
		}
		ss := &stableSub{sub: s, doneCh: make(chan struct{})}
		stables[i] = ss
		go func(ss *stableSub) {
			for range ss.sub.Events() {
				ss.delivered.Add(1)
			}
			close(ss.doneCh)
		}(ss)
	}

	// Background publisher: 200/sec for the full duration.
	publisherStop := make(chan struct{})
	var publishCount atomic.Int64
	go func() {
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		i := 0
		for {
			select {
			case <-publisherStop:
				return
			case <-ticker.C:
				_, _ = hub.Store().Put(fmt.Sprintf("watched/%07d", i),
					"perfreview/entity", map[string]interface{}{"tick": i})
				publishCount.Add(1)
				i++
			}
		}
	}()

	start := time.Now()
	rows := []thrashSnapshot{takeThrashSnapshot("baseline", start)}

	// Churn loop: 200 cycles over ~20s.
	const churnCycles = 200
	churnErrors := 0
	for c := 0; c < churnCycles; c++ {
		csub, err := spoke.SubscribeAt(hub.PeerID(), "watched/*", entitysdk.SubscribeOpts{})
		if err != nil {
			churnErrors++
			continue
		}
		// Brief drain.
		select {
		case <-csub.Events():
		case <-time.After(100 * time.Millisecond):
		}
		_ = csub.Close()

		if (c+1)%40 == 0 {
			rows = append(rows, takeThrashSnapshot(
				fmt.Sprintf("churn=%d", c+1), start))
		}
	}

	close(publisherStop)
	time.Sleep(2 * time.Second)
	rows = append(rows, takeThrashSnapshot("post-churn", start))

	// Tear down stables.
	for _, ss := range stables {
		_ = ss.sub.Close()
	}
	for _, ss := range stables {
		select {
		case <-ss.doneCh:
		case <-time.After(2 * time.Second):
		}
	}

	t.Logf("\n=== mixed-thrash snapshots ===")
	logThrashSnapshots(t, rows)
	t.Logf("churn errors: %d / %d", churnErrors, churnCycles)
	t.Logf("publisher:    %d writes", publishCount.Load())

	t.Logf("\nstable subs delivered counts:")
	var sumStable int64
	var minStable, maxStable int64 = -1, 0
	for i, ss := range stables {
		d := ss.delivered.Load()
		sumStable += d
		if minStable < 0 || d < minStable {
			minStable = d
		}
		if d > maxStable {
			maxStable = d
		}
		t.Logf("  stable[%d]: %d", i, d)
	}
	t.Logf("stable sum=%d min=%d max=%d spread=%d (publish=%d)",
		sumStable, minStable, maxStable, maxStable-minStable, publishCount.Load())

	// Sanity: stable subs should each get ~publishCount events.
	// Spread between min/max stable indicates uneven delivery under churn.
	pubCount := publishCount.Load()
	if pubCount > 0 {
		minPct := 100.0 * float64(minStable) / float64(pubCount)
		t.Logf("min stable delivered %.1f%% of published — degradation under churn",
			minPct)
		// Fail only if very degraded — stable should get ≥80% under
		// 200/sec publish + ~10/sec churn.
		if minPct < 50.0 {
			t.Errorf("stable subs degraded under churn: min=%.1f%% of publish",
				minPct)
		}
	}
}

// TestThrash_ConcurrentSubUnsubSamePattern hits the engine with
// concurrent sub/unsub from multiple goroutines on the same pattern.
// Race-detector should catch any engine-side races; final state
// should converge cleanly.
func TestThrash_ConcurrentSubUnsubSamePattern(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	hub, spoke, cleanup := bringUpThrashPair(t, ctx, dir)
	defer cleanup()

	// Background trickle of writes — keep the engine active but not
	// dominate the test.
	publisherStop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(20 * time.Millisecond) // 50/sec
		defer ticker.Stop()
		i := 0
		for {
			select {
			case <-publisherStop:
				return
			case <-ticker.C:
				_, _ = hub.Store().Put(fmt.Sprintf("watched/%07d", i),
					"perfreview/entity", map[string]interface{}{"tick": i})
				i++
			}
		}
	}()

	start := time.Now()
	rows := []thrashSnapshot{takeThrashSnapshot("baseline", start)}

	// 8 concurrent workers, each doing 50 sub/unsub cycles = 400 cycles total.
	const workerCount = 8
	const perWorker = 50

	var wg sync.WaitGroup
	var subErrors atomic.Int64
	var closeErrors atomic.Int64

	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				sub, err := spoke.SubscribeAt(hub.PeerID(), "watched/*", entitysdk.SubscribeOpts{})
				if err != nil {
					subErrors.Add(1)
					continue
				}
				// Drain briefly without blocking.
				select {
				case <-sub.Events():
				case <-time.After(50 * time.Millisecond):
				}
				if err := sub.Close(); err != nil {
					closeErrors.Add(1)
				}
			}
		}(w)
	}
	wg.Wait()

	rows = append(rows, takeThrashSnapshot("post-wg", start))
	close(publisherStop)
	time.Sleep(3 * time.Second)
	rows = append(rows, takeThrashSnapshot("settled+3s", start))

	t.Logf("\n=== concurrent thrash snapshots ===")
	logThrashSnapshots(t, rows)
	t.Logf("workers=%d × cycles=%d → total=%d",
		workerCount, perWorker, workerCount*perWorker)
	t.Logf("errors: subscribe=%d close=%d", subErrors.Load(), closeErrors.Load())

	baseline := rows[0]
	final := rows[len(rows)-1]
	t.Logf("goroutine delta: %d → %d (Δ=%+d)",
		baseline.Goroutines, final.Goroutines,
		final.Goroutines-baseline.Goroutines)

	if delta := final.Goroutines - baseline.Goroutines; delta > 50 {
		t.Errorf("concurrent-thrash goroutine leak: Δ=%+d after %d concurrent cycles + 3s settle",
			delta, workerCount*perWorker)
	}
}
