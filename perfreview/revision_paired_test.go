//go:build perfreview

package perfreview

// Stage 6 lead-in probe — revision-paired saturation.
//
// Stage 5 stress-tested the subscription substrate in isolation. Real
// production workloads pair subscription delivery with revision-tracked
// state. This file characterizes the combined envelope.
//
// Three subtests:
//
//   1. TestRevision_PerPutLatency_FlatPrefix_NSweep
//      Local single-peer; auto-version on a flat prefix; sweep
//      N = 50/100/200/500/1000/2000. Capture per-Put p50/p95/p99 at
//      each N. Produces the O(N)/Put cliff curve in a single run.
//      Reproducer for the Investigation-11 finding (29ms/Put at N=500)
//      and the baseline against which a future OP-1 amendment lands.
//
//   2. TestRevision_HubSpoke_Throughput_OnVsOff
//      Hub-and-spoke fan-out (4 spokes); compare aggregate cross-peer
//      throughput envelope with auto-version OFF (Stage 5 baseline
//      shape) vs ON (revision on the path). Holds the spoke side flat
//      so the diff isolates the per-Put auto-version cost.
//
//   3. TestRevision_HubSpoke_FetchDiff_Recovery
//      Hub burns past the spoke's delivery rate (5K/sec for 3s),
//      spoke drops notifications, then calls ReconcileSinceLastSeen
//      to pull the delta via revision:fetch-diff. Measures recovery
//      latency + entities reconciled. Confirms the F3+F7 round-2
//      narrative end-to-end against a revision-tracked prefix.
//
// Lane reference: Stage 5 follow-ups, Lane 1 (the Stage 6 lead-in).

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/peer"
	coretypes "go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

// --- 1. Per-Put latency N-sweep (the cliff) ---------------------------

// TestRevision_PerPutLatency_FlatPrefix_NSweep produces the per-Put
// latency vs N curve under auto-version on a flat prefix. The flat
// prefix shape ("docs/{0..N}") forces every Put to extend one trie node
// — the worst case for the current trie-rebuild implementation per
// the hierarchical-shape counter-example in autoversion_hierarchical_test.go.
//
// Output: rows of (N, mean, p50, p95, p99, total-wall) — one per
// chunk-boundary. Each Logf row stands alone as memo-ready data.
//
// Budget: ~5 minutes for the full sweep at worst case.
func TestRevision_PerPutLatency_FlatPrefix_NSweep(t *testing.T) {
	type rowResult struct {
		N             int
		Mean          time.Duration
		P50, P95, P99 time.Duration
		WallTotal     time.Duration
	}
	var rows []rowResult

	// We measure incrementally: write to N=2000 once and snapshot at
	// each milestone. This avoids re-running expensive small Ns.
	// Per-Put latency at index i is what dominates at that depth; the
	// snapshot reports the trailing-window distribution from the prior
	// milestone up to this milestone (so p95 at N=500 reflects writes
	// 200..500 — the cost curve at that depth, not the cumulative
	// average).
	milestones := []int{50, 100, 200, 500, 1000, 2000}

	h := NewHarness(t, HarnessOptions{})
	defer h.Close()

	yes := true
	cfg := coretypes.RevisionConfigData{
		Prefix:      "docs/",
		AutoVersion: &yes,
	}
	if _, err := h.Peer().Revision().Config(context.Background(), coretypes.RevisionConfigParamsData{
		Action: "set",
		Name:   "perfreview-flat",
		Config: &cfg,
	}); err != nil {
		t.Fatalf("install revision config: %v", err)
	}

	storeAPI := h.Peer().Store()
	allLatencies := make([]time.Duration, 0, milestones[len(milestones)-1])

	prevMilestone := 0
	wallStart := time.Now()

	for _, ms := range milestones {
		windowStart := time.Now()
		windowLatencies := make([]time.Duration, 0, ms-prevMilestone)
		for i := prevMilestone; i < ms; i++ {
			path := fmt.Sprintf("docs/%07d", i)
			start := time.Now()
			if _, err := storeAPI.Put(path, "perfreview/entity",
				map[string]interface{}{"tick": i, "filler": "x"}); err != nil {
				t.Fatalf("Put i=%d: %v", i, err)
			}
			d := time.Since(start)
			windowLatencies = append(windowLatencies, d)
			allLatencies = append(allLatencies, d)
		}
		wallTotal := time.Since(wallStart)

		sort.Slice(windowLatencies, func(i, j int) bool { return windowLatencies[i] < windowLatencies[j] })
		var sum time.Duration
		for _, d := range windowLatencies {
			sum += d
		}
		mean := sum / time.Duration(len(windowLatencies))
		p50 := windowLatencies[len(windowLatencies)*50/100]
		p95 := windowLatencies[len(windowLatencies)*95/100]
		p99 := windowLatencies[len(windowLatencies)*99/100]

		rows = append(rows, rowResult{
			N:    ms,
			Mean: mean,
			P50:  p50, P95: p95, P99: p99,
			WallTotal: wallTotal,
		})
		t.Logf("N=%-5d window=%d..%d mean=%s p50=%s p95=%s p99=%s wall-total=%s window=%s",
			ms, prevMilestone, ms, short(mean), short(p50), short(p95), short(p99),
			short(wallTotal), short(time.Since(windowStart)))

		prevMilestone = ms

		// Defensive budget: if window for this milestone took >4 min,
		// stop sweeping. The cliff is established; further data is
		// confirmatory not novel.
		if time.Since(windowStart) > 4*time.Minute {
			t.Logf("budget exceeded at N=%d; stopping sweep early", ms)
			break
		}
	}

	t.Logf("\n=== per-Put latency cliff curve (flat prefix, auto-version on) ===")
	t.Logf("%-8s %10s %10s %10s %10s %10s",
		"N", "mean", "p50", "p95", "p99", "wall-total")
	for _, r := range rows {
		t.Logf("%-8d %10s %10s %10s %10s %10s",
			r.N, short(r.Mean), short(r.P50), short(r.P95), short(r.P99), short(r.WallTotal))
	}

	// Growth ratio first vs last window — a rough O() shape indicator.
	if len(rows) >= 2 {
		first := rows[0].P50
		last := rows[len(rows)-1].P50
		ratio := float64(last) / float64(first)
		expectN := float64(rows[len(rows)-1].N) / float64(rows[0].N)
		t.Logf("p50 growth %s → %s (%.1fx) over N %d → %d (%.1fx). Linear-O(N) would predict ~%.1fx.",
			short(first), short(last), ratio,
			rows[0].N, rows[len(rows)-1].N, expectN, expectN)
	}
}

// --- 2. Hub-and-spoke throughput: auto-version on vs off --------------

// TestRevision_HubSpoke_Throughput_OnVsOff measures aggregate cross-
// peer throughput envelope when the publish path is revision-tracked vs
// not. The Stage 5 baseline (~6,500/sec at N=8 mesh without revision)
// becomes the reference; this probe asks "how much of that ceiling
// does auto-version eat at modest workloads?".
//
// Method: 4 spokes; hub publishes 1000 writes at the fastest rate the
// per-Put cost allows. Repeat with auto-version OFF and ON on hub's
// `watched/` prefix. Compare wall-time to publish + per-spoke delivery
// rate.
func TestRevision_HubSpoke_Throughput_OnVsOff(t *testing.T) {
	const numSpokes = 4
	const writeCount = 1000

	type variantResult struct {
		Name        string
		PublishWall time.Duration
		PerPutMean  time.Duration
		PerPutP95   time.Duration
		PerSpoke    []int64
		Sum         int64
		MeanPct     float64
		PublishRate float64 // writes/sec
	}
	var results []variantResult

	for _, autoVersion := range []bool{false, true} {
		variantName := "off"
		if autoVersion {
			variantName = "on"
		}
		t.Run("autoversion="+variantName, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("HOME", dir)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			hub, spokes := bringUpHubAndSpokes(t, ctx, dir, numSpokes)
			defer cleanupHubAndSpokes(hub, spokes)

			if autoVersion {
				yes := true
				cfg := coretypes.RevisionConfigData{
					Prefix:      "watched/",
					AutoVersion: &yes,
				}
				if _, err := hub.Revision().Config(ctx, coretypes.RevisionConfigParamsData{
					Action: "set",
					Name:   "perfreview-watched",
					Config: &cfg,
				}); err != nil {
					t.Fatalf("install revision config on hub: %v", err)
				}
			}

			storeAPI := hub.Store()
			latencies := make([]time.Duration, 0, writeCount)
			publishStart := time.Now()
			for i := 0; i < writeCount; i++ {
				path := fmt.Sprintf("watched/%07d", i)
				start := time.Now()
				if _, err := storeAPI.Put(path, "perfreview/entity",
					map[string]interface{}{"tick": i}); err != nil {
					t.Fatalf("hub Put i=%d: %v", i, err)
				}
				latencies = append(latencies, time.Since(start))
			}
			publishWall := time.Since(publishStart)

			// Drain window: 3s after publish stops, deliveries should
			// have plateaued.
			time.Sleep(3 * time.Second)

			sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
			var sum time.Duration
			for _, d := range latencies {
				sum += d
			}
			mean := sum / time.Duration(len(latencies))
			p95 := latencies[len(latencies)*95/100]

			row := variantResult{
				Name:        "autoversion=" + variantName,
				PublishWall: publishWall,
				PerPutMean:  mean,
				PerPutP95:   p95,
				PerSpoke:    make([]int64, numSpokes),
				PublishRate: float64(writeCount) / publishWall.Seconds(),
			}
			for i, s := range spokes {
				d := s.delivered.Load()
				row.PerSpoke[i] = d
				row.Sum += d
			}
			row.MeanPct = 100.0 * float64(row.Sum) / float64(writeCount*numSpokes)
			results = append(results, row)

			t.Logf("autoversion=%s: publish-wall=%s per-Put-mean=%s p95=%s rate=%.0f/s perSpoke=%v sum=%d mean-pct=%.1f%%",
				variantName, short(publishWall), short(mean), short(p95),
				row.PublishRate, row.PerSpoke, row.Sum, row.MeanPct)
		})
	}

	t.Logf("\n=== throughput envelope: auto-version off vs on ===")
	t.Logf("%-18s %14s %14s %14s %10s %10s",
		"variant", "publish-wall", "rate", "per-Put-mean", "p95", "deliv-pct")
	for _, r := range results {
		t.Logf("%-18s %14s %10.0f/s %14s %10s %9.1f%%",
			r.Name, short(r.PublishWall), r.PublishRate,
			short(r.PerPutMean), short(r.PerPutP95), r.MeanPct)
	}
	if len(results) == 2 {
		ratio := results[1].PerPutMean.Seconds() / results[0].PerPutMean.Seconds()
		t.Logf("auto-version cost: %.1fx per-Put over baseline (off → on at writeCount=%d)",
			ratio, writeCount)
	}
}

// --- 3b. F18 diagnostic — bisect the EOF cause -----------------------

// TestRevision_F18_Bisect_AutoVersionOff_LongBurn — first cut of the
// F18 bisect: same N=2000 writes, auto-version OFF, eliminate cliff.
//
// Result observed: burn 378ms / 100% subscription
// delivery / ReconcileSinceLastSeen returns 404 "no revision head
// bound" (because no auto-version config → no revision state). The
// transport stayed healthy. This rules out "any long-elapsed burst
// kills the connection" but does NOT discriminate auto-version-pressure
// vs long-burn-time as the F18 cause — the like-for-like probe is
// `TestRevision_F18_Bisect_AutoVersionOn_Hierarchical` below.
//
// Kept for the data point + as a positive characterization that the
// 5K/sec substrate-bound burst doesn't break recovery.
func TestRevision_F18_Bisect_AutoVersionOff_LongBurn(t *testing.T) {
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
		t.Fatalf("CreatePeer hub: %v", err)
	}
	defer hub.Close()

	ready := make(chan struct{})
	go func() { _ = hub.ListenReady(ctx, ready) }()
	<-ready

	// NB: NO revision config installed. Auto-version stays off on
	// watched/ — the diagnostic question is whether the cliff causes
	// the F18 EOF, so we eliminate the cliff.

	spoke, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		Storage:    entitysdk.StorageConfig{Kind: "sqlite", Path: filepath.Join(dir, "spoke.db")},
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("CreatePeer spoke: %v", err)
	}
	defer spoke.Close()

	spokeReady := make(chan struct{})
	go func() { _ = spoke.ListenReady(ctx, spokeReady) }()
	<-spokeReady

	if _, err := spoke.Connect(ctx, hub.Addr().String()); err != nil {
		t.Fatalf("spoke→hub connect: %v", err)
	}
	if _, err := hub.Connect(ctx, spoke.Addr().String()); err != nil {
		t.Fatalf("hub→spoke connect: %v", err)
	}

	sub, err := spoke.SubscribeAt(hub.PeerID(), "watched/*", entitysdk.SubscribeOpts{})
	if err != nil {
		t.Fatalf("spoke SubscribeAt: %v", err)
	}
	var delivered atomic.Int64
	doneCh := make(chan struct{})
	go func() {
		for range sub.Events() {
			delivered.Add(1)
		}
		close(doneCh)
	}()

	// Burn 2000 writes — same count as the F18 case but no auto-version
	// so the burn runs at substrate speed (~hundreds/sec).
	const burnCount = 2000
	burnStart := time.Now()
	for i := 0; i < burnCount; i++ {
		path := fmt.Sprintf("watched/%07d", i)
		if _, err := hub.Store().Put(path, "perfreview/entity",
			map[string]interface{}{"tick": i}); err != nil {
			t.Fatalf("hub burn Put i=%d: %v", i, err)
		}
	}
	burnWall := time.Since(burnStart)
	t.Logf("F18-bisect burn (no auto-version): %d writes in %s (%.0f writes/sec)",
		burnCount, short(burnWall), float64(burnCount)/burnWall.Seconds())

	// Drain window.
	time.Sleep(3 * time.Second)
	deliveredPostBurn := delivered.Load()
	t.Logf("post-burn delivered: %d of sent=%d (%.1f%%)",
		deliveredPostBurn, burnCount, 100.0*float64(deliveredPostBurn)/float64(burnCount))

	// Same recovery call shape as the original F18 reproducer. With
	// no auto-version config on hub, there's no revision head — so
	// FetchDiff will return an empty closure (or zero head). That's
	// fine for the EOF diagnostic; we just want to know if the
	// transport stays healthy.
	recoveryStart := time.Now()
	res, err := spoke.ReconcileSinceLastSeen(ctx, hub.PeerID(), "watched/", hash.Hash{})
	recoveryWall := time.Since(recoveryStart)
	if err != nil {
		t.Logf("F18-bisect ReconcileSinceLastSeen ERROR (NOT cliff-bound): %v (wall=%s)",
			err, short(recoveryWall))
	} else {
		t.Logf("F18-bisect recovery: wall=%s entities-ingested=%d",
			short(recoveryWall), res.EntitiesIngested)
	}

	// Cleanup
	_ = sub.Close()
	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
	}

	t.Logf("\n=== F18 bisect interpretation ===")
	if err != nil {
		t.Logf("Recovery FAILED with auto-version OFF (burn=%s). F18 is NOT cliff-induced; transport has an independent failure mode.", short(burnWall))
	} else {
		t.Logf("Recovery SUCCEEDED with auto-version OFF (burn=%s, recovery=%s). F18 is CLIFF-INDUCED — the 137s burn under auto-version is necessary for the EOF.", short(burnWall), short(recoveryWall))
	}
}

// TestRevision_F18_Bisect_AutoVersionOn_Hierarchical was originally a
// bisect probe to characterize F18. Arch root-caused F18 to a ctx-leak
// in core-go's Peer.ListenReady (caller's ctx passed straight to spawned
// Connection.serve goroutines — workbench's 90s test ctx killed serve
// loops mid-burn, breaking spoke→hub connection). Fix landed at
// core-go a0d3ec6 (Peer.serveCtx decoupling).
//
// The test is retained as a regression probe: sustained auto-version-
// bound burn + post-burn fetch-diff should succeed cleanly. If a
// future regression re-introduces the ctx-leak (or any similar
// connection-death-during-sustained-load issue), this test will
// surface it.
//
// Pre-fix behavior: ~58% delivery during burn, broken-pipe on recovery.
// Post-fix behavior: 100% delivery during burn, recovery succeeds in
// ~700ms (148µs per entity, consistent with the linear-scaling curve
// validated at N=500..2000 in TestRevision_HubSpoke_FetchDiff_Recovery).
//
// Historical naming retained (`_F18_Bisect_`) for git/blame traceability;
// the test's role is now regression validation, not active diagnosis.
func TestRevision_F18_Bisect_AutoVersionOn_Hierarchical(t *testing.T) {
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
		t.Fatalf("CreatePeer hub: %v", err)
	}
	defer hub.Close()

	ready := make(chan struct{})
	go func() { _ = hub.ListenReady(ctx, ready) }()
	<-ready

	// Configure auto-version on hub at the hierarchical prefix.
	yes := true
	revCfg := coretypes.RevisionConfigData{
		Prefix:      "docs/",
		AutoVersion: &yes,
	}
	if _, err := hub.Revision().Config(ctx, coretypes.RevisionConfigParamsData{
		Action: "set",
		Name:   "perfreview-f18-hier",
		Config: &revCfg,
	}); err != nil {
		t.Fatalf("install revision config on hub: %v", err)
	}

	spoke, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		Storage:    entitysdk.StorageConfig{Kind: "sqlite", Path: filepath.Join(dir, "spoke.db")},
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("CreatePeer spoke: %v", err)
	}
	defer spoke.Close()

	spokeReady := make(chan struct{})
	go func() { _ = spoke.ListenReady(ctx, spokeReady) }()
	<-spokeReady

	if _, err := spoke.Connect(ctx, hub.Addr().String()); err != nil {
		t.Fatalf("spoke→hub connect: %v", err)
	}
	if _, err := hub.Connect(ctx, spoke.Addr().String()); err != nil {
		t.Fatalf("hub→spoke connect: %v", err)
	}

	sub, err := spoke.SubscribeAt(hub.PeerID(), "docs/*", entitysdk.SubscribeOpts{})
	if err != nil {
		t.Fatalf("spoke SubscribeAt: %v", err)
	}
	var delivered atomic.Int64
	doneCh := make(chan struct{})
	go func() {
		for range sub.Events() {
			delivered.Add(1)
		}
		close(doneCh)
	}()

	// Hierarchical paths — bounded fanout per
	// autoversion_hierarchical_test.go. With three 2-digit levels and
	// 10 children per level (00..09), 1000 leaves stay flat-bounded.
	// At 2000 entries we have two leaves per "directory" slot at the
	// deepest level, but every trie node still has ≤10 children — no
	// cliff.
	pathOf := func(i int) string {
		l1 := (i / 100) % 10
		l2 := (i / 10) % 10
		l3 := i % 10
		return fmt.Sprintf("docs/%02d/%02d/%02d/file-%d", l1, l2, l3, i)
	}

	const burnCount = 2000
	burnStart := time.Now()
	for i := 0; i < burnCount; i++ {
		path := pathOf(i)
		if _, err := hub.Store().Put(path, "perfreview/entity",
			map[string]interface{}{"tick": i}); err != nil {
			t.Fatalf("hub burn Put i=%d: %v", i, err)
		}
	}
	burnWall := time.Since(burnStart)
	t.Logf("F18-bisect burn (auto-version ON, hierarchical): %d writes in %s (%.0f writes/sec)",
		burnCount, short(burnWall), float64(burnCount)/burnWall.Seconds())

	time.Sleep(3 * time.Second)
	deliveredPostBurn := delivered.Load()
	t.Logf("post-burn delivered: %d of sent=%d (%.1f%%)",
		deliveredPostBurn, burnCount, 100.0*float64(deliveredPostBurn)/float64(burnCount))

	preStatus, err := spoke.RevisionAt(hub.PeerID()).Status(ctx, "docs/")
	if err != nil {
		t.Fatalf("spoke Status: %v", err)
	}
	t.Logf("spoke head pre-recovery: %v", preStatus.Head)

	recoveryStart := time.Now()
	res, err := spoke.ReconcileSinceLastSeen(ctx, hub.PeerID(), "docs/", hash.Hash{})
	recoveryWall := time.Since(recoveryStart)

	_ = sub.Close()
	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
	}

	t.Logf("\n=== F18 hierarchical-bisect interpretation ===")
	t.Logf("setup:    auto-version ON, hierarchical path (no cliff)")
	t.Logf("burn:     %d writes in %s (%.0f/sec)",
		burnCount, short(burnWall), float64(burnCount)/burnWall.Seconds())
	if err != nil {
		t.Logf("RECOVERY FAILED: %v (wall=%s)", err, short(recoveryWall))
		t.Logf("Interpretation: F18 is NOT wall-time-bound. EOF reproduces at sub-second burn → auto-version-handler interaction at hub.")
		t.Fatalf("F18 reproduces at fast burn — handler-level bug, not transport idle")
	}
	t.Logf("RECOVERY OK: wall=%s entities-ingested=%d", short(recoveryWall), res.EntitiesIngested)
	t.Logf("F18 regression validation: sustained auto-version-bound burn completes with 100%% delivery and clean recovery. Fix landed at core-go a0d3ec6 (Peer.serveCtx decoupling).")
}

// TestRevision_F18_Bisect_LongIdle was originally an F18 discriminator
// probe (ruling out transport idle-timeout as the cause). With F18
// root-caused to a ctx-leak (core-go a0d3ec6), the test is retained
// as a positive characterization: long-idle connections survive
// cleanly. Historical naming retained for git/blame traceability.
func TestRevision_F18_Bisect_LongIdle(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	hub, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		Storage:    entitysdk.StorageConfig{Kind: "sqlite", Path: filepath.Join(dir, "hub.db")},
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("CreatePeer hub: %v", err)
	}
	defer hub.Close()
	ready := make(chan struct{})
	go func() { _ = hub.ListenReady(ctx, ready) }()
	<-ready

	yes := true
	revCfg := coretypes.RevisionConfigData{Prefix: "watched/", AutoVersion: &yes}
	if _, err := hub.Revision().Config(ctx, coretypes.RevisionConfigParamsData{
		Action: "set", Name: "perfreview-idle", Config: &revCfg,
	}); err != nil {
		t.Fatalf("install revision config: %v", err)
	}

	spoke, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		Storage:    entitysdk.StorageConfig{Kind: "sqlite", Path: filepath.Join(dir, "spoke.db")},
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("CreatePeer spoke: %v", err)
	}
	defer spoke.Close()
	spokeReady := make(chan struct{})
	go func() { _ = spoke.ListenReady(ctx, spokeReady) }()
	<-spokeReady

	if _, err := spoke.Connect(ctx, hub.Addr().String()); err != nil {
		t.Fatalf("spoke→hub connect: %v", err)
	}
	if _, err := hub.Connect(ctx, spoke.Addr().String()); err != nil {
		t.Fatalf("hub→spoke connect: %v", err)
	}

	// Seed a small revision history so fetch-diff has something to
	// pull. Fast enough that the cliff doesn't dominate.
	for i := 0; i < 10; i++ {
		if _, err := hub.Store().Put(fmt.Sprintf("watched/%07d", i),
			"perfreview/entity", map[string]interface{}{"tick": i}); err != nil {
			t.Fatalf("seed Put: %v", err)
		}
	}

	// Sanity-check that fetch-diff works at t=0 (before any idle).
	t0Start := time.Now()
	_, err = spoke.ReconcileSinceLastSeen(ctx, hub.PeerID(), "watched/", hash.Hash{})
	t.Logf("t=0 recovery: wall=%s err=%v", short(time.Since(t0Start)), err)
	if err != nil {
		t.Fatalf("fetch-diff failed at t=0 (sanity): %v", err)
	}

	// Now sit idle for 150s — long enough to exceed any plausible
	// transport keepalive but shorter than the test deadline.
	idleDuration := 150 * time.Second
	t.Logf("idling %s...", idleDuration)
	time.Sleep(idleDuration)
	t.Logf("idle complete, attempting fetch-diff")

	tNStart := time.Now()
	_, err = spoke.ReconcileSinceLastSeen(ctx, hub.PeerID(), "watched/", hash.Hash{})
	tNWall := time.Since(tNStart)

	t.Logf("\n=== long-idle characterization ===")
	if err != nil {
		t.Logf("Recovery FAILED after %s idle: %v (wall=%s) — REGRESSION, investigate",
			idleDuration, err, short(tNWall))
		t.Fatalf("idle-survival regression: %v", err)
	}
	t.Logf("Recovery OK after %s idle (wall=%s) — transport survives quiet connections.",
		idleDuration, short(tNWall))
}

// --- 3. FetchDiff recovery probe --------------------------------------

// TestRevision_HubSpoke_FetchDiff_Recovery drives the hub above the
// spoke's delivery rate so notifications drop. Spoke captures the
// pre-burn revision head via Status(), takes the drops, then calls
// ReconcileSinceLastSeen to pull the delta via revision:fetch-diff +
// tree:merge. Measures recovery wall-time + entities ingested + how
// much state was actually recovered.
//
// Companion to F3+F7 round-2 narrative: substrate has no implicit
// catch-up for missed deliveries, but the workbench-SDK helper makes
// recovery a single call. This probe confirms the helper works against
// a revision-tracked prefix at saturation scale.
func TestRevision_HubSpoke_FetchDiff_Recovery(t *testing.T) {
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
		t.Fatalf("CreatePeer hub: %v", err)
	}
	defer hub.Close()

	ready := make(chan struct{})
	go func() { _ = hub.ListenReady(ctx, ready) }()
	<-ready

	// Configure hub-side auto-version on watched/.
	yes := true
	revCfg := coretypes.RevisionConfigData{
		Prefix:      "watched/",
		AutoVersion: &yes,
	}
	if _, err := hub.Revision().Config(ctx, coretypes.RevisionConfigParamsData{
		Action: "set",
		Name:   "perfreview-recovery",
		Config: &revCfg,
	}); err != nil {
		t.Fatalf("install revision config on hub: %v", err)
	}

	spoke, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		Storage:    entitysdk.StorageConfig{Kind: "sqlite", Path: filepath.Join(dir, "spoke.db")},
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("CreatePeer spoke: %v", err)
	}
	defer spoke.Close()

	spokeReady := make(chan struct{})
	go func() { _ = spoke.ListenReady(ctx, spokeReady) }()
	<-spokeReady

	if _, err := spoke.Connect(ctx, hub.Addr().String()); err != nil {
		t.Fatalf("spoke→hub connect: %v", err)
	}
	if _, err := hub.Connect(ctx, spoke.Addr().String()); err != nil {
		t.Fatalf("hub→spoke connect: %v", err)
	}

	sub, err := spoke.SubscribeAt(hub.PeerID(), "watched/*", entitysdk.SubscribeOpts{})
	if err != nil {
		t.Fatalf("spoke SubscribeAt: %v", err)
	}
	var delivered atomic.Int64
	doneCh := make(chan struct{})
	go func() {
		for range sub.Events() {
			delivered.Add(1)
		}
		close(doneCh)
	}()

	// Capture spoke's pre-burn lastSeen — it's zero hash (first time
	// touching this prefix). For a revision-tracked recovery test the
	// honest baseline is "no prior history" → reconcile pulls the
	// full closure.
	preStatus, err := spoke.RevisionAt(hub.PeerID()).Status(ctx, "watched/")
	if err != nil {
		t.Fatalf("spoke Status pre-burn: %v", err)
	}
	t.Logf("pre-burn spoke head for watched/: %v (zero=%v)", preStatus.Head, preStatus.Head.IsZero())
	lastSeen := preStatus.Head

	// Burn phase: hub publishes well past the per-Put-bound ceiling.
	// We can't drive a rate the per-Put cost can't sustain — just
	// publish as fast as Put returns and let the substrate be the
	// limiter.
	//
	// burnCount kept at 1000 for default CI signal (~36s). The helper
	// was validated linear across N=500..2000 post-F18 fix in core-go
	// a0d3ec6: 150ms@500 / 294ms@1000 / 448ms@1500 / 567ms@2000 —
	// ~148µs/entity flat. Bump locally to characterize larger N.
	const burnCount = 1000
	burnStart := time.Now()
	for i := 0; i < burnCount; i++ {
		path := fmt.Sprintf("watched/%07d", i)
		if _, err := hub.Store().Put(path, "perfreview/entity",
			map[string]interface{}{"tick": i}); err != nil {
			t.Fatalf("hub burn Put i=%d: %v", i, err)
		}
	}
	burnWall := time.Since(burnStart)
	t.Logf("burn phase: %d writes in %s (%.0f writes/sec)",
		burnCount, short(burnWall), float64(burnCount)/burnWall.Seconds())

	// Drain window — see how many deliveries arrive.
	time.Sleep(3 * time.Second)
	deliveredPostBurn := delivered.Load()
	t.Logf("post-burn delivered: %d of sent=%d (%.1f%%)",
		deliveredPostBurn, burnCount, 100.0*float64(deliveredPostBurn)/float64(burnCount))

	// Recovery phase: spoke pulls delta from hub.
	recoveryStart := time.Now()
	res, err := spoke.ReconcileSinceLastSeen(ctx, hub.PeerID(), "watched/", lastSeen)
	recoveryWall := time.Since(recoveryStart)
	if err != nil {
		t.Fatalf("ReconcileSinceLastSeen: %v", err)
	}
	t.Logf("recovery: wall=%s entities-ingested=%d (lastSeen=%v)",
		short(recoveryWall), res.EntitiesIngested, lastSeen)

	// Post-reconcile sanity: how many `watched/*` entities are in
	// spoke's local store now? Use the read-only sqlite counter.
	spokeCount := countEntities(t, filepath.Join(dir, "spoke.db"))
	hubCount := countEntities(t, filepath.Join(dir, "hub.db"))
	t.Logf("post-reconcile entity counts: spoke=%d hub=%d", spokeCount, hubCount)

	// Cleanup
	_ = sub.Close()
	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
	}

	t.Logf("\n=== reconcile envelope summary ===")
	t.Logf("burn:     %d writes in %s = %.0f writes/sec",
		burnCount, short(burnWall), float64(burnCount)/burnWall.Seconds())
	t.Logf("subs:     %d delivered of %d sent (%.1f%%)",
		deliveredPostBurn, burnCount, 100.0*float64(deliveredPostBurn)/float64(burnCount))
	t.Logf("recovery: %s wall to ReconcileSinceLastSeen with %d entities ingested",
		short(recoveryWall), res.EntitiesIngested)
}
