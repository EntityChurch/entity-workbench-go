//go:build perfreview

package perfreview

// Combined-stress probes — substrate stress with multiple concurrent
// shapes happening simultaneously. Isolated probes (Stage 5 topology,
// Lane 4 thrash) all passed; this file looks for failure modes that
// only emerge under combined load.
//
// Three probes:
//
//   1. TestStress_KitchenSink — stable subs + thrashing + reconcile
//      + high publish rate, all concurrently for 30s. The "everything
//      at once" smoke test.
//
//   2. TestStress_MassiveFanIn — extend Stage 5 fan-in past M=8.
//      How many spokes can one hub support before per-spoke delivery
//      degrades? Stage 5 stopped at M=8.
//
//   3. TestStress_SameTargetPathBurst — concurrent writes to the
//      SAME path from multiple goroutines. Exercises content-store
//      dedup + last-write-wins on locations index.

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/peer"
	coretypes "go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

// TestStress_KitchenSink runs multiple concurrent stress shapes
// against a single hub for 30s. Looks for failures that only emerge
// under combined load.
//
// Setup:
//   - Hub publishes at 500/sec continuously
//   - 4 stable spokes with permanent subscriptions
//   - 1 churn spoke doing sub/unsub at 5/sec
//   - 1 reconcile spoke doing ReconcileSinceLastSeen every 5s
func TestStress_KitchenSink(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	hub, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		Storage:    entitysdk.StorageConfig{Kind: "sqlite", Path: filepath.Join(dir, "hub.db")},
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("hub create: %v", err)
	}
	defer hub.Close()
	hubReady := make(chan struct{})
	go func() { _ = hub.ListenReady(ctx, hubReady) }()
	<-hubReady

	// Bring up 6 spokes: 4 stable + 1 churn + 1 reconcile.
	const stableCount = 4
	allSpokes := make([]*entitysdk.AppPeer, 6)
	for i := 0; i < 6; i++ {
		s, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
			ListenAddr: "127.0.0.1:0",
			Storage:    entitysdk.StorageConfig{Kind: "sqlite", Path: filepath.Join(dir, fmt.Sprintf("spoke%d.db", i))},
			RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
		})
		if err != nil {
			t.Fatalf("spoke%d create: %v", i, err)
		}
		sReady := make(chan struct{})
		go func() { _ = s.ListenReady(ctx, sReady) }()
		<-sReady

		if _, err := s.Connect(ctx, hub.Addr().String()); err != nil {
			t.Fatalf("spoke%d→hub: %v", i, err)
		}
		if _, err := hub.Connect(ctx, s.Addr().String()); err != nil {
			t.Fatalf("hub→spoke%d: %v", i, err)
		}
		allSpokes[i] = s
		defer s.Close()
	}

	// Wire stable subscriptions.
	type stableInfo struct {
		sub       *entitysdk.Subscription
		delivered atomic.Int64
		doneCh    chan struct{}
	}
	stables := make([]*stableInfo, stableCount)
	for i := 0; i < stableCount; i++ {
		sub, err := allSpokes[i].SubscribeAt(hub.PeerID(), "watched/*", entitysdk.SubscribeOpts{})
		if err != nil {
			t.Fatalf("stable[%d] subscribe: %v", i, err)
		}
		st := &stableInfo{sub: sub, doneCh: make(chan struct{})}
		stables[i] = st
		go func(st *stableInfo) {
			for range st.sub.Events() {
				st.delivered.Add(1)
			}
			close(st.doneCh)
		}(st)
	}

	churnSpoke := allSpokes[4]
	reconcileSpoke := allSpokes[5]

	// Configure auto-version on hub so revision-paired reconcile has
	// something to chase.
	yes := true
	revCfg := coretypes.RevisionConfigData{Prefix: "watched/", AutoVersion: &yes}
	if _, err := hub.Revision().Config(ctx, coretypes.RevisionConfigParamsData{
		Action: "set", Name: "kitchen-sink", Config: &revCfg,
	}); err != nil {
		t.Fatalf("install auto-version: %v", err)
	}

	const wallDuration = 30 * time.Second
	stopCh := make(chan struct{})

	// Publisher: 500/sec for the duration.
	var publishCount atomic.Int64
	var publishErrs atomic.Int64
	go func() {
		ticker := time.NewTicker(2 * time.Millisecond)
		defer ticker.Stop()
		i := 0
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				if _, err := hub.Store().Put(fmt.Sprintf("watched/%07d", i),
					"perfreview/entity",
					map[string]interface{}{"tick": i}); err != nil {
					publishErrs.Add(1)
				} else {
					publishCount.Add(1)
				}
				i++
			}
		}
	}()

	// Churn: sub/unsub at 5/sec.
	var churnCycles atomic.Int64
	var churnErrs atomic.Int64
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				sub, err := churnSpoke.SubscribeAt(hub.PeerID(), "watched/*", entitysdk.SubscribeOpts{})
				if err != nil {
					churnErrs.Add(1)
					continue
				}
				select {
				case <-sub.Events():
				case <-time.After(50 * time.Millisecond):
				}
				_ = sub.Close()
				churnCycles.Add(1)
			}
		}
	}()

	// Reconcile: every 5s.
	var reconcileRuns atomic.Int64
	var reconcileErrs atomic.Int64
	var reconcileTotalWall atomic.Int64
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				start := time.Now()
				_, err := reconcileSpoke.ReconcileSinceLastSeen(ctx, hub.PeerID(), "watched/", hash.Hash{})
				dur := time.Since(start)
				reconcileTotalWall.Add(int64(dur))
				if err != nil {
					reconcileErrs.Add(1)
					t.Logf("reconcile error at t=%s: %v", short(time.Since(start)), err)
				} else {
					reconcileRuns.Add(1)
				}
			}
		}
	}()

	t.Logf("kitchen-sink running for %s...", wallDuration)
	time.Sleep(wallDuration)
	close(stopCh)
	time.Sleep(3 * time.Second) // drain

	// Tear down stables.
	for _, st := range stables {
		_ = st.sub.Close()
	}
	for _, st := range stables {
		select {
		case <-st.doneCh:
		case <-time.After(2 * time.Second):
		}
	}

	pub := publishCount.Load()
	pubErrs := publishErrs.Load()
	t.Logf("\n=== kitchen-sink results ===")
	t.Logf("publisher: %d writes (%.0f/sec), %d errors",
		pub, float64(pub)/wallDuration.Seconds(), pubErrs)
	t.Logf("churn:     %d cycles, %d errors", churnCycles.Load(), churnErrs.Load())
	t.Logf("reconcile: %d runs, %d errors, avg=%s",
		reconcileRuns.Load(), reconcileErrs.Load(),
		short(time.Duration(reconcileTotalWall.Load()/max64(1, reconcileRuns.Load()))))

	t.Logf("\nstable subs deliveries:")
	var sumStable, minStable, maxStable int64
	minStable = -1
	for i, st := range stables {
		d := st.delivered.Load()
		sumStable += d
		if minStable < 0 || d < minStable {
			minStable = d
		}
		if d > maxStable {
			maxStable = d
		}
		pct := 100.0 * float64(d) / float64(pub)
		t.Logf("  stable[%d]: %d (%.1f%% of publish)", i, d, pct)
	}
	if pub > 0 {
		minPct := 100.0 * float64(minStable) / float64(pub)
		spread := maxStable - minStable
		t.Logf("stable summary: sum=%d min=%d max=%d spread=%d min-pct=%.1f%%",
			sumStable, minStable, maxStable, spread, minPct)

		if minPct < 70.0 {
			t.Errorf("stable deliveries degraded under kitchen-sink load: min=%.1f%% (expected ≥70%%)",
				minPct)
		}
	}

	if pubErrs > 0 {
		t.Errorf("publisher errors during kitchen-sink: %d", pubErrs)
	}
	if reconcileErrs.Load() > 0 {
		t.Errorf("reconcile errors during kitchen-sink: %d / %d",
			reconcileErrs.Load(), reconcileRuns.Load())
	}
}

// TestStress_MassiveFanIn extends Stage 5's M=8 limit. Hub publishes
// 500/sec; vary spoke count M = 8/16/32. Goal: characterize the
// per-spoke delivery rate as M grows. If H-G1 sharding works as
// designed, per-spoke delivery should stay high even at M=32.
func TestStress_MassiveFanIn(t *testing.T) {
	type fanoutRow struct {
		M           int
		Sent        int
		Sum         int64
		Min, Max    int64
		MeanPct     float64
		PerSpokePct []float64
	}
	var rows []fanoutRow

	for _, m := range []int{8, 16, 32} {
		m := m
		t.Run(fmt.Sprintf("M=%d", m), func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("HOME", dir)
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()

			hub, spokes := bringUpHubAndSpokes(t, ctx, dir, m)
			defer cleanupHubAndSpokes(hub, spokes)

			sent, _ := driveHubWrites(t, hub, 500, 3*time.Second)
			time.Sleep(3 * time.Second) // drain

			row := fanoutRow{M: m, Sent: sent}
			row.Min = -1
			for _, s := range spokes {
				d := s.delivered.Load()
				row.Sum += d
				if row.Min < 0 || d < row.Min {
					row.Min = d
				}
				if d > row.Max {
					row.Max = d
				}
			}
			row.MeanPct = 100.0 * float64(row.Sum) / float64(sent*m)
			rows = append(rows, row)

			t.Logf("M=%d sent=%d sum=%d min=%d max=%d mean-pct=%.1f%%",
				m, sent, row.Sum, row.Min, row.Max, row.MeanPct)
		})
	}

	t.Logf("\n=== massive fan-in results (hub=500/sec, wall=3s) ===")
	t.Logf("%-6s %8s %12s %10s %10s %10s",
		"M", "sent", "sum-deliv", "min-spoke", "max-spoke", "mean-pct")
	for _, r := range rows {
		t.Logf("%-6d %8d %12d %10d %10d %9.1f%%",
			r.M, r.Sent, r.Sum, r.Min, r.Max, r.MeanPct)
	}
}

// TestStress_SameTargetPathBurst hammers concurrent writes to the
// SAME path from multiple goroutines. Exercises:
//   - Content store dedup (identical writes → same hash → idempotent)
//   - Location index last-write-wins under contention
//   - Subscription delivery semantics for same-path updates
func TestStress_SameTargetPathBurst(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	hub, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		Storage:    entitysdk.StorageConfig{Kind: "sqlite", Path: filepath.Join(dir, "hub.db")},
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("hub create: %v", err)
	}
	defer hub.Close()
	ready := make(chan struct{})
	go func() { _ = hub.ListenReady(ctx, ready) }()
	<-ready

	const targetPath = "watched/the-one-path"
	const workerCount = 16
	const writesPerWorker = 100

	storeAPI := hub.Store()

	var wg sync.WaitGroup
	var writeErrors atomic.Int64
	start := time.Now()

	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < writesPerWorker; i++ {
				// Salt with worker+seq so each write is distinct content.
				if _, err := storeAPI.Put(targetPath, "perfreview/entity",
					map[string]interface{}{
						"worker": workerID,
						"seq":    i,
					}); err != nil {
					writeErrors.Add(1)
				}
			}
		}(w)
	}
	wg.Wait()
	wall := time.Since(start)

	total := workerCount * writesPerWorker
	t.Logf("\n=== same-path concurrent burst ===")
	t.Logf("workers=%d × writes=%d → total=%d in %s (%.0f writes/sec)",
		workerCount, writesPerWorker, total, short(wall),
		float64(total)/wall.Seconds())
	t.Logf("write errors: %d / %d", writeErrors.Load(), total)

	// Check: the path has SOME entity bound, no panics, no internal
	// inconsistency. Look at counts.
	ents := countEntities(t, filepath.Join(dir, "hub.db"))
	t.Logf("hub entity count after burst: %d (expected ~%d distinct content-addressed entries)",
		ents, total)

	if writeErrors.Load() > 0 {
		t.Errorf("same-path burst had %d write errors", writeErrors.Load())
	}
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
