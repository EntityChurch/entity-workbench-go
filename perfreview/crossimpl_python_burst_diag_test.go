//go:build perfreview

package perfreview

// Diagnostic burst probe for Python F-CIMP-2 regression. Issues N
// writes one at a time with a small sleep between each, logs wb-go-
// side observed deliveries continuously. Distinguishes:
//
//  - Burst-overflow that recovers given time → eventually all arrive
//  - Persistent break after N=K → first K succeed, rest silently drop
//  - Fundamental break → 0 deliveries
//
// Per-write inter-arrival is parameterized so we can find the
// threshold where Python's pipeline stops keeping up.

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"
	coretypes "go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

func TestCrossImpl_PythonBurstDiag(t *testing.T) {
	targetAddr := os.Getenv("CROSSIMPL_PY_ADDR")
	if targetAddr == "" {
		t.Skip("CROSSIMPL_PY_ADDR required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	dbgLog := log.New(os.Stderr, "[wb-go-dbg] ", log.LstdFlags|log.Lmicroseconds)
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{
			peer.WithConnectionGrants(peer.OpenAccessGrants()),
			peer.WithDebugLog(dbgLog),
		},
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
		t.Fatalf("connect python: %v", err)
	}
	pyID := string(conn.ConnState().RemotePeerID)

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
		fmt.Sprintf("/%s/system/peer/transport/%s", pyID, ap.PeerID()),
		transportEnt,
	); err != nil {
		t.Fatalf("register transport: %v", err)
	}

	sub, err := ap.SubscribeAt(pyID, "burst/*", entitysdk.SubscribeOpts{})
	if err != nil {
		t.Fatalf("SubscribeAt: %v", err)
	}
	defer sub.Close()

	var delivered atomic.Int64
	doneCh := make(chan struct{})
	go func() {
		for range sub.Events() {
			delivered.Add(1)
		}
		close(doneCh)
	}()

	time.Sleep(300 * time.Millisecond)

	// Write N events with a sleep between each. Find where the pipeline
	// stops keeping up.
	N := 50
	interArrival := 0 * time.Millisecond
	if v := os.Getenv("BURST_N"); v != "" {
		fmt.Sscanf(v, "%d", &N)
	}
	if v := os.Getenv("BURST_INTERVAL_MS"); v != "" {
		var ms int
		fmt.Sscanf(v, "%d", &ms)
		interArrival = time.Duration(ms) * time.Millisecond
	}
	t.Logf("publishing %d events with %v inter-arrival to Python", N, interArrival)

	publishStart := time.Now()
	for i := 0; i < N; i++ {
		path := fmt.Sprintf("/%s/burst/%03d", pyID, i)
		if _, err := ap.Put(path, "burst/diag", map[string]any{"i": i}); err != nil {
			t.Fatalf("Put i=%d: %v", i, err)
		}
		if (i+1)%10 == 0 {
			t.Logf("publish progress: i=%d delivered=%d elapsed=%v",
				i+1, delivered.Load(), time.Since(publishStart))
		}
		time.Sleep(interArrival)
	}
	t.Logf("publish complete: %d writes in %v", N, time.Since(publishStart))

	// Long drain — let any pipeline-in-progress finish.
	drainStart := time.Now()
	for _, deadline := range []time.Duration{1, 3, 10, 20, 30} {
		sleepFor := deadline*time.Second - time.Since(drainStart)
		if sleepFor > 0 {
			time.Sleep(sleepFor)
		}
		t.Logf("drain @%ds: delivered=%d", deadline, delivered.Load())
	}

	finalDelivered := delivered.Load()
	t.Logf("FINAL: published=%d delivered=%d (%.0f%%)", N, finalDelivered,
		100.0*float64(finalDelivered)/float64(N))
}
