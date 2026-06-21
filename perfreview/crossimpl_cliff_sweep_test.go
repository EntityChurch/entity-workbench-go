//go:build perfreview

package perfreview

// Cross-impl cliff sweep — workbench-go (coordinator) drives the
// wide-flat per-Put N-sweep against a Rust or Python peer. Same probe
// shape as TestRevision_PerPutLatency_FlatPrefix_NSweep but writes go
// cross-peer via `ap.Put("/{remote_id}/docs/{i}", ...)` so we measure
// the remote substrate's per-Put cost under auto-version load.
//
// Stress-test framing (Cohort B): workbench-go is the metrics-bearing
// coordinator; the substrate-under-measurement is the remote impl. If
// the v4.2 HAMT win we measured today on Go (per-Put p50 collapses from
// 32ms→4ms at N=2000, growth shape linear→log) is structural, the
// remote impl should show the same shape. If it cliffs linearly, the
// remote substrate is missing v4.2 HAMT or OP-1, and we have a real
// cross-impl convergence finding to route.
//
// Skipped unless CROSSIMPL_TARGET_ADDR is set:
//
//   /tmp/peer-manager start --name rust1 --type rust
//   /tmp/peer-manager addr rust1
//   CROSSIMPL_TARGET_ADDR=127.0.0.1:NNNN CROSSIMPL_TARGET_IMPL=rust \
//     make perfreview ARGS="-run TestCrossImpl_CliffSweep -v -timeout=10m"

import (
	"context"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"
	coretypes "go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

func TestCrossImpl_CliffSweep(t *testing.T) {
	targetAddr := os.Getenv("CROSSIMPL_TARGET_ADDR")
	targetImpl := os.Getenv("CROSSIMPL_TARGET_IMPL")
	if targetAddr == "" {
		t.Skip("CROSSIMPL_TARGET_ADDR required; spawn target via peer-manager")
	}
	if targetImpl == "" {
		targetImpl = "unknown"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("create wb-go: %v", err)
	}
	defer ap.Close()
	ready := make(chan struct{})
	errCh := make(chan error, 1)
	go func() { errCh <- ap.ListenReady(ctx, ready) }()
	select {
	case <-ready:
	case err := <-errCh:
		t.Fatalf("wb-go listen: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("wb-go listen timeout")
	}

	conn, err := ap.Connect(ctx, targetAddr)
	if err != nil {
		t.Fatalf("connect %s @ %s: %v", targetImpl, targetAddr, err)
	}
	remoteID := string(conn.ConnState().RemotePeerID)
	t.Logf("workbench-go: %s @ %s", ap.PeerID(), ap.Addr())
	t.Logf("target (%s): %s @ %s", targetImpl, remoteID, targetAddr)

	// Install auto-version config on the REMOTE peer's revision handler
	// at "docs/" prefix. This is what makes the per-Put path go through
	// the substrate's tree-rebuild logic — the same code path the
	// single-peer Go probe exercises.
	yes := true
	cfg := coretypes.RevisionConfigData{
		Prefix:      "docs/",
		AutoVersion: &yes,
	}
	if _, err := ap.RevisionAt(remoteID).Config(ctx, coretypes.RevisionConfigParamsData{
		Action: "set",
		Name:   "crossimpl-cliff-sweep",
		Config: &cfg,
	}); err != nil {
		t.Fatalf("install revision config on %s: %v", targetImpl, err)
	}
	t.Logf("auto-version installed on %s at docs/", targetImpl)

	type rowResult struct {
		N             int
		Mean          time.Duration
		P50, P95, P99 time.Duration
		WallTotal     time.Duration
		WindowWall    time.Duration
	}
	var rows []rowResult
	milestones := []int{50, 100, 200, 500, 1000, 2000}

	prevMilestone := 0
	wallStart := time.Now()

	for _, ms := range milestones {
		windowStart := time.Now()
		windowLatencies := make([]time.Duration, 0, ms-prevMilestone)
		for i := prevMilestone; i < ms; i++ {
			path := fmt.Sprintf("/%s/docs/%07d", remoteID, i)
			start := time.Now()
			if _, err := ap.Put(path, "perfreview/entity",
				map[string]interface{}{"tick": i, "filler": "x"}); err != nil {
				t.Fatalf("cross-peer Put i=%d: %v", i, err)
			}
			windowLatencies = append(windowLatencies, time.Since(start))
		}
		wallTotal := time.Since(wallStart)
		windowWall := time.Since(windowStart)

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
			N: ms, Mean: mean, P50: p50, P95: p95, P99: p99,
			WallTotal: wallTotal, WindowWall: windowWall,
		})
		t.Logf("N=%-5d window=%d..%d mean=%s p50=%s p95=%s p99=%s wall-total=%s window=%s",
			ms, prevMilestone, ms, short(mean), short(p50), short(p95), short(p99),
			short(wallTotal), short(windowWall))

		prevMilestone = ms

		// Budget guard — wide-flat cliffs can grow viciously; bail at 4
		// min/window so a degenerate-cliff impl doesn't burn the whole
		// test timeout. Cliff shape is established by then.
		if windowWall > 4*time.Minute {
			t.Logf("budget exceeded at N=%d; stopping sweep early", ms)
			break
		}
	}

	t.Logf("\n=== cross-impl cliff curve (%s, flat prefix, auto-version on, cross-peer Put) ===", targetImpl)
	t.Logf("%-8s %10s %10s %10s %10s %10s",
		"N", "mean", "p50", "p95", "p99", "wall-total")
	for _, r := range rows {
		t.Logf("%-8d %10s %10s %10s %10s %10s",
			r.N, short(r.Mean), short(r.P50), short(r.P95), short(r.P99), short(r.WallTotal))
	}

	if len(rows) >= 2 {
		first := rows[0].P50
		last := rows[len(rows)-1].P50
		ratio := float64(last) / float64(first)
		expectN := float64(rows[len(rows)-1].N) / float64(rows[0].N)
		t.Logf("p50 growth %s → %s (%.1fx) over N %d → %d (%.1fx). Linear-O(N) would predict ~%.1fx.",
			short(first), short(last), ratio,
			rows[0].N, rows[len(rows)-1].N, expectN, expectN)
		t.Logf("Reference baselines (Go single-peer, this morning):")
		t.Logf("  pre-OP-1:  44x growth (linear)")
		t.Logf("  post-OP-1: 28x growth (sublinear)")
		t.Logf("  v4.2 HAMT:  6x growth (log shape)  ← Go-side win")
		t.Logf("Target (%s): %.1fx growth", targetImpl, ratio)
	}
}
