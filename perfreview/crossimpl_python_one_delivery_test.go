//go:build perfreview

package perfreview

// Single-delivery probe against Python — F-CIMP-2 regression check.
// After Python's Class H F1b fix landed (e395813), the throughput
// envelope shows 0/1000 deliveries with empty error messages in
// Python's log. This probe issues ONE write, waits long enough for
// even a slow pipeline to drain, and reports — distinguishes "delivery
// fundamentally broken" (0 delivered) from "delivery slow under load"
// (eventually delivers).

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"
	coretypes "go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

func TestCrossImpl_PythonOneDelivery(t *testing.T) {
	targetAddr := os.Getenv("CROSSIMPL_PY_ADDR")
	if targetAddr == "" {
		t.Skip("CROSSIMPL_PY_ADDR required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
		t.Fatalf("connect python: %v", err)
	}
	pyID := string(conn.ConnState().RemotePeerID)
	t.Logf("wb-go: %s @ %s", ap.PeerID(), ap.Addr())
	t.Logf("python: %s @ %s", pyID, targetAddr)

	transportEnt, err := coretypes.TCPProfileData{
		PeerID:        ap.PeerID(),
		TransportType: "tcp",
		Endpoint:      coretypes.TransportEndpointURL{URL: "tcp://" + ap.Addr().String()},
		SupportedOps:  []string{coretypes.OpExecute},
	}.ToEntity()
	if err != nil {
		t.Fatalf("encode transport: %v", err)
	}
	transportPath := fmt.Sprintf("/%s/system/peer/transport/%s", pyID, ap.PeerID())
	if _, err := ap.PutEntity(transportPath, transportEnt); err != nil {
		t.Fatalf("register transport: %v", err)
	}
	t.Logf("transport registered on python")

	sub, err := ap.SubscribeAt(pyID, "single/*", entitysdk.SubscribeOpts{})
	if err != nil {
		t.Fatalf("SubscribeAt python: %v", err)
	}
	defer sub.Close()
	t.Logf("subscription installed on python: %s", sub.ID())

	time.Sleep(300 * time.Millisecond)

	triggerPath := fmt.Sprintf("/%s/single/probe-%d", pyID, time.Now().UnixNano())
	if _, err := ap.Put(triggerPath, "smoke/single", map[string]any{"probe": true}); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	t.Logf("trigger written: %s", triggerPath)

	select {
	case evt := <-sub.Events():
		t.Logf("*** DELIVERED: path=%s event=%s newhash=%s ***",
			evt.Path, evt.EventType, evt.NewHash.String())
	case <-time.After(10 * time.Second):
		t.Errorf("NO delivery within 10s — Python delivery fundamentally broken (not just slow under load)")
	}
}
