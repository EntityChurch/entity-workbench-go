//go:build perfreview

package perfreview

// Rust saturation sweep — push the publish rate up via parallel
// publishers and find where Rust starts dropping subscription
// notifications.
//
// Sequential probe (TestCrossImpl_ThroughputEnvelope) measured Rust
// at 581 writes/sec / 100% delivery — but that's just whatever wb-go
// can publish serially through the cross-impl dispatch path. This
// probe parallelizes the publish loop with K workers to push the
// effective rate up and characterize Rust's saturation curve.
//
// Stress framing: if Rust holds 100% delivery as we push N + K
// higher, the substrate is sound and we've empirically closed the
// Rust column. If we find a knee where delivery drops, that's a
// real cross-impl substrate finding.
//
// Run:
//   /tmp/peer-manager start --name rust1 --type rust
//   CROSSIMPL_TARGET_ADDR=127.0.0.1:NNNN CROSSIMPL_TARGET_IMPL=rust \
//     make perfreview ARGS="-run TestCrossImpl_RustSaturationSweep -v -timeout=10m"

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"
	coretypes "go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

type satRow struct {
	K           int
	N           int
	PublishWall time.Duration
	PublishRate float64
	Delivered   int64
	DeliveryPct float64
}

func TestCrossImpl_RustSaturationSweep(t *testing.T) {
	targetAddr := os.Getenv("CROSSIMPL_TARGET_ADDR")
	targetImpl := os.Getenv("CROSSIMPL_TARGET_IMPL")
	if targetAddr == "" {
		t.Skip("CROSSIMPL_TARGET_ADDR required")
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
		t.Fatalf("connect %s: %v", targetImpl, err)
	}
	remoteID := string(conn.ConnState().RemotePeerID)
	t.Logf("workbench-go hub: %s @ %s", ap.PeerID(), ap.Addr())
	t.Logf("target (%s): %s", targetImpl, remoteID)

	transportEnt, err := coretypes.TCPProfileData{
		PeerID:        ap.PeerID(),
		TransportType: "tcp",
		Endpoint:      coretypes.TransportEndpointURL{URL: "tcp://" + ap.Addr().String()},
		SupportedOps:  []string{coretypes.OpExecute},
	}.ToEntity()
	if err != nil {
		t.Fatalf("encode transport: %v", err)
	}
	if _, err := ap.PutEntity(
		fmt.Sprintf("/%s/system/peer/transport/%s", remoteID, ap.PeerID()),
		transportEnt,
	); err != nil {
		t.Fatalf("register transport: %v", err)
	}

	// Sweep: (workers, total writes).
	// Each step uses a fresh subscription path so deliveries don't
	// stack between steps.
	steps := []struct{ K, N int }{
		{1, 1000},   // sequential baseline (matches throughput envelope)
		{4, 4000},   // 4x parallel; expect ~4x rate
		{8, 8000},   // 8x parallel
		{16, 16000}, // 16x parallel
		{32, 32000}, // 32x parallel — where Rust's substrate likely stresses
	}

	var rows []satRow

	for _, step := range steps {
		// Fresh subscription per step.
		stepID := fmt.Sprintf("sat-step-%d-%d", step.K, step.N)
		pattern := stepID + "/*"
		sub, err := ap.SubscribeAt(remoteID, pattern, entitysdk.SubscribeOpts{})
		if err != nil {
			t.Fatalf("SubscribeAt step (K=%d): %v", step.K, err)
		}

		var delivered atomic.Int64
		drainerDone := make(chan struct{})
		go func() {
			for range sub.Events() {
				delivered.Add(1)
			}
			close(drainerDone)
		}()

		time.Sleep(200 * time.Millisecond)

		var publishWG sync.WaitGroup
		perWorker := step.N / step.K
		publishStart := time.Now()
		for w := 0; w < step.K; w++ {
			publishWG.Add(1)
			go func(workerID int) {
				defer publishWG.Done()
				for i := 0; i < perWorker; i++ {
					seq := workerID*perWorker + i
					path := fmt.Sprintf("/%s/%s/%07d", remoteID, stepID, seq)
					if _, err := ap.Put(path, "perfreview/sat",
						map[string]interface{}{"w": workerID, "i": i}); err != nil {
						t.Logf("Put failure step K=%d w=%d i=%d: %v", step.K, workerID, i, err)
						return
					}
				}
			}(w)
		}
		publishWG.Wait()
		publishWall := time.Since(publishStart)

		// Drain window scales with N.
		drainWait := time.Duration(step.N/200)*time.Second + 3*time.Second
		if drainWait > 30*time.Second {
			drainWait = 30 * time.Second
		}
		time.Sleep(drainWait)

		row := satRow{
			K:           step.K,
			N:           step.N,
			PublishWall: publishWall,
			PublishRate: float64(step.N) / publishWall.Seconds(),
			Delivered:   delivered.Load(),
			DeliveryPct: 100.0 * float64(delivered.Load()) / float64(step.N),
		}
		rows = append(rows, row)
		t.Logf("step K=%-2d N=%-5d wall=%s rate=%.0f/s delivered=%d (%.1f%%)",
			row.K, row.N, short(row.PublishWall), row.PublishRate,
			row.Delivered, row.DeliveryPct)

		// Close subscription before next step to avoid noise.
		_ = sub.Close()
		<-drainerDone
	}

	t.Logf("\n=== Rust saturation curve (parallel publish, single subscription) ===")
	t.Logf("%-4s %-6s %-10s %-8s %-10s %-10s",
		"K", "N", "wall", "rate/s", "delivered", "delivery%")
	for _, r := range rows {
		t.Logf("%-4d %-6d %-10s %-8.0f %-10d %.1f%%",
			r.K, r.N, short(r.PublishWall), r.PublishRate,
			r.Delivered, r.DeliveryPct)
	}

	// Pass/fail: every step must hold ≥95% delivery.
	for _, r := range rows {
		if r.DeliveryPct < 95 {
			t.Errorf("step K=%d N=%d delivery %.1f%% < 95%% — substrate saturated",
				r.K, r.N, r.DeliveryPct)
		}
	}
}
