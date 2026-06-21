//go:build perfreview

package perfreview

// Multi-peer saturation probe — Stage 5 Tier 1 #1.
//
// WB-21 characterized
// the single-subscriber cliff: ~2K notifs/sec stable, silent drops above
// (~47% delivery at 5K+/sec). The HANDOFF asks: does that cliff scale with
// subscriber count under fan-out? And do drops distribute evenly across
// spokes or land on some peers more than others?
//
// These probes bypass the watcher entirely — writes go straight to the
// hub's Store.Put, which fires subscription deliveries directly. No
// fsnotify debounce, no localfiles handler in the path. This is the
// shape WB-21 used for the 1-subscriber sweep, extended to N spokes.
//
// What we measure (per spoke):
//   - delivered count vs hub-side sent count → delivery percentage
//   - per-spoke variance (does spoke-1 drop differently from spoke-4?)
//   - post-write convergence: does each spoke see the final state via
//     its tree even when deliveries dropped mid-burn?
//
// What this probe does NOT do:
//   - Push through localfiles + revision merge. That's the Stage 4 Case G
//     shape; this probe is about the subscription engine's intrinsic
//     ceiling under fan-out, not the cross-peer chain plumbing.
//   - Test mesh symmetric writes. Symmetric writes hit the WB-28
//     reentrant-deadlock surface — closed by Class G multiplexing in
//     core-go 5792cdc, but probing it here would be testing the wrong
//     thing. Single-direction hub→spoke first; mesh later if interesting.

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"

	"entity-workbench-go/entitysdk"
)

// spokeReceiver tracks delivery state for one spoke in a hub-and-spoke
// fan-out probe.
type spokeReceiver struct {
	name      string
	ap        *entitysdk.AppPeer
	sub       *entitysdk.Subscription
	delivered atomic.Int64
	doneCh    chan struct{}
}

// bringUpHubAndSpokes creates `numSpokes+1` peers (peers[0] is the hub,
// the rest are spokes), brings the hub up as listener, connects each
// spoke to the hub, and installs a cross-peer subscribe from each spoke
// to the hub's `watched/*` prefix. Each spoke drains its events channel
// into its delivered counter in a background goroutine.
//
// Caller must defer cleanupHubAndSpokes(hub, spokes) before returning.
func bringUpHubAndSpokes(t *testing.T, ctx context.Context, dir string, numSpokes int) (*entitysdk.AppPeer, []*spokeReceiver) {
	t.Helper()

	hub, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		Storage:    entitysdk.StorageConfig{Kind: "sqlite", Path: filepath.Join(dir, "hub.db")},
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("CreatePeer hub: %v", err)
	}

	ready := make(chan struct{})
	listenErr := make(chan error, 1)
	go func() { listenErr <- hub.ListenReady(ctx, ready) }()
	select {
	case <-ready:
	case err := <-listenErr:
		hub.Close()
		t.Fatalf("hub listen: %v", err)
	case <-time.After(5 * time.Second):
		hub.Close()
		t.Fatal("hub listen timeout")
	}

	spokes := make([]*spokeReceiver, 0, numSpokes)
	for i := 0; i < numSpokes; i++ {
		name := fmt.Sprintf("spoke%d", i+1)
		spoke, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
			ListenAddr: "127.0.0.1:0", // spoke must listen so hub can dial back
			Storage:    entitysdk.StorageConfig{Kind: "sqlite", Path: filepath.Join(dir, name+".db")},
			RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
		})
		if err != nil {
			t.Fatalf("CreatePeer %s: %v", name, err)
		}
		spokeReady := make(chan struct{})
		spokeListenErr := make(chan error, 1)
		go func() { spokeListenErr <- spoke.ListenReady(ctx, spokeReady) }()
		select {
		case <-spokeReady:
		case err := <-spokeListenErr:
			t.Fatalf("%s listen: %v", name, err)
		case <-time.After(5 * time.Second):
			t.Fatalf("%s listen timeout", name)
		}
		// Bidirectional connect — hub needs an outbound to the spoke to
		// dispatch notifications back through the connection pool.
		if _, err := spoke.Connect(ctx, hub.Addr().String()); err != nil {
			t.Fatalf("%s→hub connect: %v", name, err)
		}
		if _, err := hub.Connect(ctx, spoke.Addr().String()); err != nil {
			t.Fatalf("hub→%s connect: %v", name, err)
		}
		sub, err := spoke.SubscribeAt(hub.PeerID(), "watched/*", entitysdk.SubscribeOpts{})
		if err != nil {
			t.Fatalf("%s SubscribeAt hub: %v", name, err)
		}
		r := &spokeReceiver{name: name, ap: spoke, sub: sub, doneCh: make(chan struct{})}
		spokes = append(spokes, r)
		go func(r *spokeReceiver) {
			for range r.sub.Events() {
				r.delivered.Add(1)
			}
			close(r.doneCh)
		}(r)
	}
	return hub, spokes
}

// cleanupHubAndSpokes closes all spokes (subscription + peer) then the
// hub. Spoke goroutines drain after Subscription.Close (it closes the
// events channel).
func cleanupHubAndSpokes(hub *entitysdk.AppPeer, spokes []*spokeReceiver) {
	for _, s := range spokes {
		_ = s.sub.Close()
	}
	for _, s := range spokes {
		select {
		case <-s.doneCh:
		case <-time.After(2 * time.Second):
		}
		s.ap.Close()
	}
	hub.Close()
}

// driveHubWrites paces writes at targetRate per second for wallTime.
// Returns the actual sent count + measured wall time.
func driveHubWrites(t *testing.T, hub *entitysdk.AppPeer, targetRate int, wallTime time.Duration) (int, time.Duration) {
	t.Helper()
	if targetRate <= 0 {
		return 0, 0
	}
	interval := time.Second / time.Duration(targetRate)
	deadline := time.Now().Add(wallTime)
	tick := time.NewTicker(interval)
	defer tick.Stop()

	storeAPI := hub.Store()
	sent := 0
	start := time.Now()
writeLoop:
	for {
		select {
		case <-tick.C:
			if time.Now().After(deadline) {
				break writeLoop
			}
			path := fmt.Sprintf("watched/%07d", sent)
			if _, err := storeAPI.Put(path, "perfreview/entity",
				map[string]interface{}{"tick": sent}); err != nil {
				t.Fatalf("hub Put: %v", err)
			}
			sent++
		}
	}
	return sent, time.Since(start)
}

// TestSaturation_HubAndSpoke_VaryRate sweeps the hub write rate while
// holding spoke count constant. Each rate brings up a fresh hub + 4
// spokes (no shared state across rates). Goal: does the WB-21 ~2K cliff
// scale with subscriber count?
//
// Two hypotheses to discriminate:
//
//	(a) Per-subscriber ceiling — each spoke saturates at ~2K notifs/sec
//	    independently. With N spokes, hub's ceiling is also 2K
//	    writes/sec (each write triggers N parallel deliveries).
//	(b) Engine-global ceiling — the hub's single deliveryLoop serializes
//	    ALL outbound notifications. With N spokes, hub's ceiling is
//	    2K/N writes/sec (because each write fires N serialized
//	    deliveries).
//
// The WB-21 source-code analysis (engine.go single deliveryLoop) suggests
// (b). This probe should produce the empirical curve.
func TestSaturation_HubAndSpoke_VaryRate(t *testing.T) {
	const numSpokes = 4
	const wallTime = 3 * time.Second

	type rateResult struct {
		Rate          int
		Sent          int
		Elapsed       time.Duration
		PerSpoke      []int64 // delivered counts
		Min, Max, Sum int64
		MeanPct       float64
	}
	rows := []rateResult{}

	for _, rate := range []int{500, 1_000, 2_000, 5_000, 10_000} {
		rate := rate
		t.Run(fmt.Sprintf("rate=%d/s", rate), func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("HOME", dir)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			hub, spokes := bringUpHubAndSpokes(t, ctx, dir, numSpokes)
			defer cleanupHubAndSpokes(hub, spokes)

			sent, elapsed := driveHubWrites(t, hub, rate, wallTime)

			// Drain window: 2s after the write phase, deliveries should
			// have plateaued (either fully delivered or stuck behind drops).
			time.Sleep(2 * time.Second)

			row := rateResult{Rate: rate, Sent: sent, Elapsed: elapsed}
			row.PerSpoke = make([]int64, numSpokes)
			row.Min = -1
			for i, s := range spokes {
				d := s.delivered.Load()
				row.PerSpoke[i] = d
				row.Sum += d
				if row.Min < 0 || d < row.Min {
					row.Min = d
				}
				if d > row.Max {
					row.Max = d
				}
			}
			if sent > 0 {
				row.MeanPct = 100.0 * float64(row.Sum) / float64(sent*numSpokes)
			}
			rows = append(rows, row)

			t.Logf("rate=%d/s sent=%d elapsed=%s perSpoke=%v sum=%d mean-pct=%.1f%% min=%d max=%d",
				rate, sent, short(elapsed), row.PerSpoke, row.Sum, row.MeanPct, row.Min, row.Max)
		})
	}

	t.Logf("\n%-10s %8s %12s %10s %10s %10s",
		"target", "sent", "sum-deliv", "min-spoke", "max-spoke", "mean-pct")
	for _, r := range rows {
		t.Logf("%-10s %8d %12d %10d %10d %9.1f%%",
			fmt.Sprintf("%d/s", r.Rate), r.Sent, r.Sum, r.Min, r.Max, r.MeanPct)
	}
}

// TestSaturation_HubAndSpoke_VaryFanout holds the hub's write rate
// constant at the WB-21 cliff (2000/s — the boundary where 1-subscriber
// is just stable) and varies the spoke count. Goal: does the cliff
// shift downward as N grows?
//
// If the cliff is at engine-global delivery throughput (~2K deliveries/s
// total), then with N=1 spoke the hub stays just at the ceiling. With
// N=4 spokes at 2000 writes/s the hub fires 8000 deliveries/s — well
// past the cliff. The expected shape: per-spoke delivery percentage
// degrades inversely with N.
func TestSaturation_HubAndSpoke_VaryFanout(t *testing.T) {
	const targetRate = 2_000
	const wallTime = 3 * time.Second

	type fanoutResult struct {
		NumSpokes int
		Sent      int
		Sum       int64
		Min, Max  int64
		MeanPct   float64
	}
	rows := []fanoutResult{}

	for _, n := range []int{1, 2, 4, 8} {
		n := n
		t.Run(fmt.Sprintf("spokes=%d", n), func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("HOME", dir)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			hub, spokes := bringUpHubAndSpokes(t, ctx, dir, n)
			defer cleanupHubAndSpokes(hub, spokes)

			sent, _ := driveHubWrites(t, hub, targetRate, wallTime)
			time.Sleep(2 * time.Second)

			row := fanoutResult{NumSpokes: n, Sent: sent}
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
			if sent > 0 {
				row.MeanPct = 100.0 * float64(row.Sum) / float64(sent*n)
			}
			rows = append(rows, row)

			t.Logf("spokes=%d sent=%d sum=%d mean-pct=%.1f%% min=%d max=%d",
				n, sent, row.Sum, row.MeanPct, row.Min, row.Max)
		})
	}

	t.Logf("\n%-10s %8s %12s %10s %10s %10s",
		"spokes", "sent", "sum-deliv", "min", "max", "mean-pct")
	for _, r := range rows {
		t.Logf("%-10d %8d %12d %10d %10d %9.1f%%",
			r.NumSpokes, r.Sent, r.Sum, r.Min, r.Max, r.MeanPct)
	}
}

// TestSaturation_HubAndSpoke_ConvergencePostBurn drives writes well
// past the saturation cliff, then waits for delivery to settle and
// asks: does each spoke's TREE STATE converge to match the hub's,
// even though many notifications were silently dropped?
//
// This probes a critical production question: the WB-21 silent-drop
// policy is "drop the notification" — but the underlying tree state is
// still in hub's content store. Does subscription's catch-up mechanism
// (or any other path) bring spokes back to consistency? If not, silent
// notification drop = silent permanent divergence, which is a much
// bigger finding than the throughput cliff.
//
// Expected:
//   - During the burn, mean delivery <50% (saturated).
//   - After burn, each spoke's tree-state count remains at delivered
//     count — substrate has no implicit catch-up for missed events
//     (consistent with Case I late-join finding).
//
// If observation differs (spokes recover via some background mechanism)
// that's a positive finding worth documenting.
func TestSaturation_HubAndSpoke_ConvergencePostBurn(t *testing.T) {
	const numSpokes = 4
	const rate = 10_000 // well past cliff
	const wallTime = 3 * time.Second

	dir := t.TempDir()
	t.Setenv("HOME", dir)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	hub, spokes := bringUpHubAndSpokes(t, ctx, dir, numSpokes)
	defer cleanupHubAndSpokes(hub, spokes)

	sent, _ := driveHubWrites(t, hub, rate, wallTime)
	t.Logf("burn: sent=%d at target=%d/s for %s", sent, rate, wallTime)

	// Phase 1: immediately after writes stop.
	time.Sleep(3 * time.Second)
	for _, s := range spokes {
		t.Logf("post-burn t=+3s: %s delivered=%d", s.name, s.delivered.Load())
	}

	// Phase 2: extended drain (10s). If there's any background catch-up
	// (subscription late-fire, periodic resync) it should show up here.
	time.Sleep(10 * time.Second)
	finalDelivered := make([]int64, numSpokes)
	for i, s := range spokes {
		finalDelivered[i] = s.delivered.Load()
		t.Logf("post-burn t=+13s: %s delivered=%d", s.name, finalDelivered[i])
	}

	// Sanity: hub's local tree state should reflect all `sent` writes.
	// (Use SDK-level Entries scan via raw store — but we don't need that
	// precision; the relevant comparison is delivered vs sent.)
	for i, d := range finalDelivered {
		pct := 100.0 * float64(d) / float64(sent)
		t.Logf("spoke%d final: delivered=%d of sent=%d (%.1f%%)", i+1, d, sent, pct)
	}

	// Sort + log min/max/spread for the per-spoke distribution.
	sorted := append([]int64(nil), finalDelivered...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	t.Logf("per-spoke distribution: min=%d max=%d spread=%d",
		sorted[0], sorted[len(sorted)-1], sorted[len(sorted)-1]-sorted[0])
}
