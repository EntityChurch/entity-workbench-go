//go:build perfreview

package perfreview

// Lane 3 — bimodal workload (heartbeat + bursts). Canvas realism:
// real workloads alternate low-rate idempotent heartbeats with
// high-rate distinct-content bursts. Tests H-G3 dedup leverage
// and burst behavior after heartbeat regime.

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

// TestBimodal_HeartbeatPlusBursts simulates a canvas-class workload:
//   - Heartbeat regime: 20/sec same-content writes for 5s (idempotent;
//     H-G3 dedup should suppress downstream deliveries)
//   - Burst regime: 2000/sec distinct-content writes for 500ms (drag
//     operation; substrate-bound)
//   - Alternation: 4 cycles of heartbeat+burst over ~22s total
//
// Measures:
//   - Aggregate delivery count vs distinct-content publishes (dedup
//     leverage: how many heartbeats produced 0 downstream events)
//   - Per-cycle burst delivery percentage (burst regime should be
//     similar to steady-state hub-and-spoke)
//   - Inter-cycle drift (do later cycles degrade?)
func TestBimodal_HeartbeatPlusBursts(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
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

	spoke, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		Storage:    entitysdk.StorageConfig{Kind: "sqlite", Path: filepath.Join(dir, "spoke.db")},
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("spoke create: %v", err)
	}
	defer spoke.Close()
	spokeReady := make(chan struct{})
	go func() { _ = spoke.ListenReady(ctx, spokeReady) }()
	<-spokeReady

	if _, err := spoke.Connect(ctx, hub.Addr().String()); err != nil {
		t.Fatalf("spoke→hub: %v", err)
	}
	if _, err := hub.Connect(ctx, spoke.Addr().String()); err != nil {
		t.Fatalf("hub→spoke: %v", err)
	}

	sub, err := spoke.SubscribeAt(hub.PeerID(), "watched/*", entitysdk.SubscribeOpts{})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	var delivered atomic.Int64
	doneCh := make(chan struct{})
	go func() {
		for range sub.Events() {
			delivered.Add(1)
		}
		close(doneCh)
	}()

	type cycleStats struct {
		Cycle              int
		HeartbeatPublished int
		HeartbeatDelivered int64 // delivery count gained during heartbeat phase
		BurstPublished     int
		BurstDelivered     int64 // delivery count gained during burst phase
		BurstWall          time.Duration
	}
	var cycles []cycleStats

	const cycleCount = 4
	const heartbeatRate = 20 // writes/sec
	const heartbeatPhase = 5 * time.Second
	const burstRate = 2000 // writes/sec
	const burstPhase = 500 * time.Millisecond

	storeAPI := hub.Store()
	heartbeatIdx := 0

	for c := 0; c < cycleCount; c++ {
		preCycleDelivered := delivered.Load()
		hbCount := 0
		hbDeadline := time.Now().Add(heartbeatPhase)
		hbTick := time.NewTicker(time.Second / time.Duration(heartbeatRate))

		// Heartbeat regime: same path, same payload (idempotent).
		for time.Now().Before(hbDeadline) {
			<-hbTick.C
			path := fmt.Sprintf("watched/heartbeat/peer-%07d", heartbeatIdx%4)
			// Same-content per heartbeat slot. H-G3 dedup should
			// suppress downstream delivery after the first instance.
			if _, err := storeAPI.Put(path, "perfreview/heartbeat",
				map[string]interface{}{"slot": heartbeatIdx % 4}); err != nil {
				t.Logf("heartbeat write err c=%d i=%d: %v", c, hbCount, err)
			}
			hbCount++
			heartbeatIdx++
		}
		hbTick.Stop()
		heartbeatDelivered := delivered.Load() - preCycleDelivered

		// Burst regime: distinct-content paths.
		burstStart := time.Now()
		preBurstDelivered := delivered.Load()
		burstCount := 0
		burstDeadline := time.Now().Add(burstPhase)
		burstTick := time.NewTicker(time.Second / time.Duration(burstRate))
		for time.Now().Before(burstDeadline) {
			<-burstTick.C
			path := fmt.Sprintf("watched/burst/c%d-i%07d", c, burstCount)
			if _, err := storeAPI.Put(path, "perfreview/burst",
				map[string]interface{}{"c": c, "i": burstCount}); err != nil {
				t.Logf("burst write err c=%d i=%d: %v", c, burstCount, err)
			}
			burstCount++
		}
		burstTick.Stop()
		burstWall := time.Since(burstStart)
		// Brief drain to let in-flight deliveries land.
		time.Sleep(500 * time.Millisecond)
		burstDelivered := delivered.Load() - preBurstDelivered

		cycles = append(cycles, cycleStats{
			Cycle:              c,
			HeartbeatPublished: hbCount,
			HeartbeatDelivered: heartbeatDelivered,
			BurstPublished:     burstCount,
			BurstDelivered:     burstDelivered,
			BurstWall:          burstWall,
		})

		t.Logf("cycle %d: hb=%d→%d (dedup-suppressed=%d, %.1f%% suppressed); burst=%d→%d in %s (%.1f%% delivered)",
			c, hbCount, heartbeatDelivered,
			int64(hbCount)-heartbeatDelivered,
			100.0*float64(int64(hbCount)-heartbeatDelivered)/float64(hbCount),
			burstCount, burstDelivered, short(burstWall),
			100.0*float64(burstDelivered)/float64(burstCount))
	}

	// Cleanup
	_ = sub.Close()
	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
	}

	// Aggregate stats
	var totalHb, totalBurst int
	var totalHbDelivered, totalBurstDelivered int64
	for _, c := range cycles {
		totalHb += c.HeartbeatPublished
		totalBurst += c.BurstPublished
		totalHbDelivered += c.HeartbeatDelivered
		totalBurstDelivered += c.BurstDelivered
	}

	t.Logf("\n=== bimodal workload summary ===")
	t.Logf("%-6s %12s %12s %14s %12s %12s %12s",
		"cycle", "hb-pub", "hb-deliv", "hb-suppressed", "burst-pub", "burst-deliv", "burst-wall")
	for _, c := range cycles {
		t.Logf("%-6d %12d %12d %13d (%.1f%%) %12d %12d %12s",
			c.Cycle, c.HeartbeatPublished, c.HeartbeatDelivered,
			int64(c.HeartbeatPublished)-c.HeartbeatDelivered,
			100.0*float64(int64(c.HeartbeatPublished)-c.HeartbeatDelivered)/float64(c.HeartbeatPublished),
			c.BurstPublished, c.BurstDelivered, short(c.BurstWall))
	}

	hbSuppressedPct := 100.0 * float64(int64(totalHb)-totalHbDelivered) / float64(totalHb)
	burstDeliveryPct := 100.0 * float64(totalBurstDelivered) / float64(totalBurst)
	t.Logf("\noverall: hb-published=%d hb-delivered=%d (suppressed=%.1f%%); burst-published=%d burst-delivered=%d (%.1f%%)",
		totalHb, totalHbDelivered, hbSuppressedPct,
		totalBurst, totalBurstDelivered, burstDeliveryPct)

	// Late-cycle drift: compare cycle 0 burst rate to cycle (N-1) burst rate
	if len(cycles) >= 2 {
		first := cycles[0]
		last := cycles[len(cycles)-1]
		firstPct := 100.0 * float64(first.BurstDelivered) / float64(first.BurstPublished)
		lastPct := 100.0 * float64(last.BurstDelivered) / float64(last.BurstPublished)
		t.Logf("\nfirst-vs-last burst delivery: %.1f%% → %.1f%% (drift=%+.1f pp)",
			firstPct, lastPct, lastPct-firstPct)
		if lastPct < firstPct-20 {
			t.Errorf("burst delivery degraded substantially over cycles: %+.1f pp drift",
				lastPct-firstPct)
		}
	}
}
