//go:build perfreview

package perfreview

// Cross-impl smoke test — does workbench-go's SDK actually drive Rust + Python
// peers over the wire, and does cross-impl subscription delivery work end-to-end?
//
// This test does NOT spawn the sibling peers — it expects them already running
// via peer-manager (cmd/peer-manager in core-go). Pass addresses via env vars:
//
//   CROSSIMPL_RUST_ADDR=127.0.0.1:NNNN
//   CROSSIMPL_PY_ADDR=127.0.0.1:NNNN
//
// If either env var is unset, the test SKIPs — this keeps it out of CI sweep
// runs while still being runnable on-demand.
//
// Workflow:
//
//   cd ../entity-core-go && go build -o /tmp/peer-manager ./cmd/peer-manager
//   /tmp/peer-manager start --name rust1 --type rust
//   /tmp/peer-manager start --name py1   --type python
//   /tmp/peer-manager list   # → grab the addresses
//   cd ../entity-workbench-go
//   CROSSIMPL_RUST_ADDR=127.0.0.1:44345 CROSSIMPL_PY_ADDR=127.0.0.1:33683 \
//     make perfreview ARGS="-run TestCrossImpl_SmokeWireAndSubscription -v -timeout=2m"

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/crypto"
	"go.entitychurch.org/entity-core-go/core/peer"
	coretypes "go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

func TestCrossImpl_SmokeWireAndSubscription(t *testing.T) {
	rustAddr := os.Getenv("CROSSIMPL_RUST_ADDR")
	pyAddr := os.Getenv("CROSSIMPL_PY_ADDR")
	if rustAddr == "" || pyAddr == "" {
		t.Skip("CROSSIMPL_RUST_ADDR + CROSSIMPL_PY_ADDR required; spawn via peer-manager")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		// Open-access connection grants on wb-go's listener — Rust/Python
		// peers dial back here to deliver subscription notifications, and
		// the inbox-handler dispatch needs an inbound cap broad enough to
		// reach `system/inbox/*`. This mirrors what TestSubscribeAt_CrossPeerNotification
		// uses for the Go↔Go path. peer-manager applies the equivalent
		// (--debug-grants, --open-access) when spawning Rust + Python.
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("create workbench peer: %v", err)
	}
	defer ap.Close()
	// Start wb-go's listener so Rust + Python can dial back when
	// delivering subscription notifications.
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
	t.Logf("workbench-go coordinator peer: %s @ %s", ap.PeerID(), ap.Addr())

	// --- Wire layer probes ---
	rustConn, err := ap.Connect(ctx, rustAddr)
	if err != nil {
		t.Fatalf("connect rust @ %s: %v", rustAddr, err)
	}
	rustID := rustConn.ConnState().RemotePeerID
	t.Logf("connected rust:   %s @ %s", rustID, rustAddr)

	pyConn, err := ap.Connect(ctx, pyAddr)
	if err != nil {
		t.Fatalf("connect python @ %s: %v", pyAddr, err)
	}
	pyID := pyConn.ConnState().RemotePeerID
	t.Logf("connected python: %s @ %s", pyID, pyAddr)

	// --- Cross-impl Put: workbench-go writes on Rust's tree, then Python's ---
	rustPath := fmt.Sprintf("/%s/scratch/from-wb-go-%d", rustID, time.Now().UnixNano())
	rustHash, err := ap.Put(rustPath, "smoke/test", map[string]any{"msg": "cross-impl", "ts": time.Now().UnixNano()})
	if err != nil {
		t.Fatalf("cross-peer Put on rust: %v", err)
	}
	t.Logf("rust accepted Put: %s → %s", rustPath, rustHash.String())

	pyPath := fmt.Sprintf("/%s/scratch/from-wb-go-%d", pyID, time.Now().UnixNano())
	pyHash, err := ap.Put(pyPath, "smoke/test", map[string]any{"msg": "cross-impl", "ts": time.Now().UnixNano()})
	if err != nil {
		t.Fatalf("cross-peer Put on python: %v", err)
	}
	t.Logf("python accepted Put: %s → %s", pyPath, pyHash.String())

	// --- ECF byte-identity: same payload → same content hash? ---
	// (Note: payloads above have different ts/path; this is a separate check
	// using a deterministic payload below.)
	const stableMsg = "deterministic-payload"
	rustStableHash, err := ap.Put(
		fmt.Sprintf("/%s/scratch/stable", rustID),
		"smoke/test",
		map[string]any{"msg": stableMsg},
	)
	if err != nil {
		t.Fatalf("stable Put on rust: %v", err)
	}
	pyStableHash, err := ap.Put(
		fmt.Sprintf("/%s/scratch/stable", pyID),
		"smoke/test",
		map[string]any{"msg": stableMsg},
	)
	if err != nil {
		t.Fatalf("stable Put on python: %v", err)
	}
	if rustStableHash.String() != pyStableHash.String() {
		t.Errorf("ECF DIVERGED across rust/python on identical payload: rust=%s python=%s",
			rustStableHash.String(), pyStableHash.String())
	} else {
		t.Logf("ECF byte-identical across rust+python on stable payload: %s", rustStableHash.String())
	}

	// --- Wire wb-go's transport address into Rust's tree so Rust's ---
	// --- subscription engine can dial back to deliver notifications ---
	// Pattern lifted from entity-core-go/cmd/entity-sync/main.go::registerTransport.
	// Without this, Rust matches the trigger event and queues a delivery,
	// then fails with "no transport address for peer" because Rust's
	// connection pool looks for system/peer/transport/{wb_go_id} on its
	// own tree and finds nothing — even though Rust has wb-go's inbound
	// connection open, the dispatch path requires a registered transport.
	wbListenAddr := ap.Addr().String()
	transportEnt, err := coretypes.TCPProfileData{
		PeerID:        ap.PeerID(),
		TransportType: "tcp",
		Endpoint:      coretypes.TransportEndpointURL{URL: "tcp://" + wbListenAddr},
		SupportedOps:  []string{coretypes.OpExecute},
	}.ToEntity()
	if err != nil {
		t.Fatalf("encode transport: %v", err)
	}
	// v7.64 transport cutover: profiles live at
	// system/peer/transport/{identity-hash-hex}/{profile-id}, not the old
	// flat .../transport/{base58-peer-id}. The hex segment is the canonical
	// content_hash hex of the peer's system/peer entity (core/peer/remote.go
	// transportProfilePrefix); profile-id "primary" is Go's TCP default.
	wbHash, err := coretypes.ComputePeerIdentityHashFromPeerID(crypto.PeerID(ap.PeerID()))
	if err != nil {
		t.Fatalf("compute wb identity hash: %v", err)
	}
	transportPath := fmt.Sprintf("/%s/system/peer/transport/%s/primary", rustID, coretypes.PeerIdentityHashHex(wbHash))
	if _, err := ap.PutEntity(transportPath, transportEnt); err != nil {
		t.Fatalf("register wb-go transport with rust: %v", err)
	}
	t.Logf("registered wb-go transport on rust: %s → %s", ap.PeerID(), wbListenAddr)

	// --- Cross-impl subscription: workbench-go subscribes to a pattern on Rust ---
	sub, err := ap.SubscribeAt(string(rustID), "scratch/*", entitysdk.SubscribeOpts{})
	if err != nil {
		t.Fatalf("SubscribeAt rust: %v", err)
	}
	t.Logf("cross-impl subscription installed: id=%s pattern=%s remote=%s", sub.ID(), sub.Pattern(), sub.RemotePeer())

	delivered := make(chan string, 16)
	go func() {
		for evt := range sub.Events() {
			delivered <- fmt.Sprintf("path=%s event=%s newhash=%s", evt.Path, evt.EventType, evt.NewHash.String())
		}
	}()

	time.Sleep(300 * time.Millisecond)

	// Trigger a write on Rust that should match the subscription pattern.
	triggerPath := fmt.Sprintf("/%s/scratch/sub-trigger-%d", rustID, time.Now().UnixNano())
	if _, err := ap.Put(triggerPath, "smoke/sub", map[string]any{"trigger": true}); err != nil {
		t.Fatalf("trigger Put on rust: %v", err)
	}
	t.Logf("trigger written: %s", triggerPath)

	// Wait for delivery (back across the wire).
	select {
	case msg := <-delivered:
		t.Logf("*** SUBSCRIPTION DELIVERED from rust: %s ***", msg)
	case <-time.After(5 * time.Second):
		t.Errorf("NO subscription delivery within 5s — rust did not route the change to wb-go's inbox")
	}

	if err := sub.Close(); err != nil {
		t.Logf("sub.Close: %v (non-fatal)", err)
	}
	t.Log("-- smoke test complete --")
}
