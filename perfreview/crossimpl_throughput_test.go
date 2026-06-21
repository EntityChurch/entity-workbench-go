//go:build perfreview

package perfreview

// Cross-impl throughput envelope (subscription delivery stress).
//
// workbench-go (hub): writes N entities to a wide-flat prefix on the
// REMOTE target peer (Rust or Python or Go control). The TARGET PEER
// is the substrate-under-measurement — its tree handler writes the
// entities, its subscription engine matches the events, and its
// dispatch path delivers notifications back to wb-go's inbox.
//
// We measure: publish wall (hub-side per-Put cost) AND delivery rate
// (notifications received vs writes issued; deliveries that arrive
// during the drain window). Substrate stress: if the remote impl
// doesn't have shard-pool / grant-cache (Go's H-G1/G2/G3 fixes), the
// receiver-side write-amp will dominate and throughput will saturate
// at a lower ceiling than Go's 6500/sec aggregate.
//
// This is the cross-impl variant of TestRevision_HubSpoke_Throughput_OnVsOff
// from revision_paired_test.go (single-Go-process baseline).
//
// Run:
//
//   /tmp/peer-manager start --name rust1 --type rust
//   CROSSIMPL_TARGET_ADDR=127.0.0.1:NNNN CROSSIMPL_TARGET_IMPL=rust \
//     make perfreview ARGS="-run TestCrossImpl_ThroughputEnvelope -v -timeout=5m"

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"
	coretypes "go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

func ptrU64(v uint64) *uint64 { return &v }

func TestCrossImpl_ThroughputEnvelope(t *testing.T) {
	targetAddr := os.Getenv("CROSSIMPL_TARGET_ADDR")
	targetImpl := os.Getenv("CROSSIMPL_TARGET_IMPL")
	if targetAddr == "" {
		t.Skip("CROSSIMPL_TARGET_ADDR required; spawn target via peer-manager")
	}
	if targetImpl == "" {
		targetImpl = "unknown"
	}

	const writeCount = 1000

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
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
	t.Logf("workbench-go hub: %s @ %s", ap.PeerID(), ap.Addr())
	t.Logf("target (%s) spoke-substrate: %s @ %s", targetImpl, remoteID, targetAddr)

	// Register wb-go transport on target so dial-back works.
	transportEnt, err := coretypes.TCPProfileData{
		PeerID:        ap.PeerID(),
		TransportType: "tcp",
		Endpoint:      coretypes.TransportEndpointURL{URL: "tcp://" + ap.Addr().String()},
		SupportedOps:  []string{coretypes.OpExecute},
	}.ToEntity()
	if err != nil {
		t.Fatalf("encode transport: %v", err)
	}
	transportPath := fmt.Sprintf("/%s/system/peer/transport/%s", remoteID, ap.PeerID())
	if _, err := ap.PutEntity(transportPath, transportEnt); err != nil {
		t.Fatalf("register wb-go transport: %v", err)
	}

	// Subscribe to the prefix on the target. The target's subscription
	// engine will deliver notifications back to wb-go's inbox per
	// trigger event.
	//
	// Limits: probe substrate throughput, not server defaults. Per
	// EXTENSION-SUBSCRIPTION §2.4 the server MAY tighten — but
	// expressing the high-throughput intent on the subscription
	// request lets impls without restrictive defaults (Go, Rust)
	// see the subscriber's desired rate. Impls with restrictive
	// defaults (Python's hardcoded 60/min) still cap; that's a
	// separate cross-impl convergence finding.
	sub, err := ap.SubscribeAt(remoteID, "watched/*", entitysdk.SubscribeOpts{
		Limits: &coretypes.SubscriptionLimitsData{
			RateLimit: ptrU64(1_000_000),
		},
	})
	if err != nil {
		t.Fatalf("SubscribeAt: %v", err)
	}
	defer sub.Close()

	var delivered atomic.Int64
	go func() {
		for range sub.Events() {
			delivered.Add(1)
		}
	}()

	time.Sleep(300 * time.Millisecond)
	t.Logf("subscription installed on %s; starting %d writes…", targetImpl, writeCount)

	// Publish loop: wb-go writes N entities onto the target's tree at
	// the watched/ prefix. Each successful write triggers a notification
	// dispatch from the target back to wb-go.
	latencies := make([]time.Duration, 0, writeCount)
	publishStart := time.Now()
	for i := 0; i < writeCount; i++ {
		path := fmt.Sprintf("/%s/watched/%07d", remoteID, i)
		start := time.Now()
		if _, err := ap.Put(path, "perfreview/entity",
			map[string]interface{}{"tick": i}); err != nil {
			t.Fatalf("publish i=%d: %v", i, err)
		}
		latencies = append(latencies, time.Since(start))
	}
	publishWall := time.Since(publishStart)

	// Drain window: 3s after publish stops, deliveries should have
	// plateaued. (Matches the single-Go probe.)
	time.Sleep(3 * time.Second)
	deliveredCount := delivered.Load()

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	var sum time.Duration
	for _, d := range latencies {
		sum += d
	}
	mean := sum / time.Duration(len(latencies))
	p50 := latencies[len(latencies)*50/100]
	p95 := latencies[len(latencies)*95/100]
	p99 := latencies[len(latencies)*99/100]
	publishRate := float64(writeCount) / publishWall.Seconds()
	deliveryPct := 100.0 * float64(deliveredCount) / float64(writeCount)

	t.Logf("\n=== cross-impl throughput envelope: workbench-go → %s ===", targetImpl)
	t.Logf("publish-wall    %s    rate %.0f writes/sec",
		short(publishWall), publishRate)
	t.Logf("per-Put         mean=%s p50=%s p95=%s p99=%s",
		short(mean), short(p50), short(p95), short(p99))
	t.Logf("delivery        %d/%d (%.1f%%)", deliveredCount, writeCount, deliveryPct)
	t.Logf("")
	t.Logf("Single-Go baseline (TestRevision_HubSpoke_Throughput_OnVsOff, watched/, autoversion=off):")
	t.Logf("  4 spokes in-process: 323µs/Put, 3091/sec, 100%% delivery × 4 spokes")
	t.Logf("  1 spoke autoversion=on: 2ms/Put, 451/sec, 100%% delivery (Stage 7 v4.2 HAMT)")

	if deliveryPct < 90 {
		t.Errorf("delivery rate %.1f%% < 90%% — substrate dropping notifications under stress", deliveryPct)
	}
}
