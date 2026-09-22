package entitysdk_test

import (
	"context"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"

	"entity-workbench-go/entitysdk"
)

// connection_direction_test.go — `PeerInfo.Direction` is real, not populated.
//
// `Direction` was declared and left empty from the day `connected_peers.go`
// was written; core-go's row 13 (`Connection.IsOutbound()` / `Direction()`)
// is what filled it. This gates the adoption.
//
// ⚠ **The assertion that matters is the pair, not either half.** A field
// hard-coded to "outbound" satisfies any test that only dials out, and the
// consequence of getting it wrong is not cosmetic: a dial-by-address
// authorizes the DIALER only (AP63), so an inbound-only session is a peer
// that can reach us while we cannot dispatch to them — the state in which
// sharing is half-broken and the one a flat "connected" reads as healthy.
//
// So both arms run against ONE pair of peers in ONE pool: bob dials alice, so
// bob's view of that connection must be outbound and alice's view of the same
// wire must be inbound. Nothing that reports a constant can pass both.

func TestConnectedPeers_DirectionIsTheRealDirectionOnBothSides(t *testing.T) {
	alice, bob, _ := listeningPairForDirection(t)

	// bob dialled, so bob holds an OUTBOUND connection to alice.
	bobView := connectionTo(t, bob, string(alice.PeerID()))
	if bobView.Direction != entitysdk.DirectionOutbound {
		t.Errorf("bob dialled alice, so bob's Direction = %q, want %q",
			bobView.Direction, entitysdk.DirectionOutbound)
	}
	if !bobView.Outbound() {
		t.Error("bob.Outbound() must be true for a connection bob opened")
	}

	// ⭐ The other half of the same wire. This is the arm a constant fails.
	aliceView := connectionTo(t, alice, string(bob.PeerID()))
	if aliceView.Direction != entitysdk.DirectionInbound {
		t.Errorf("alice was dialled, so alice's Direction = %q, want %q — "+
			"a direction that reads the same on both ends of one connection is not a direction",
			aliceView.Direction, entitysdk.DirectionInbound)
	}
	if aliceView.Outbound() {
		t.Error("alice.Outbound() must be false: she did not dial, so a dial-by-address " +
			"authorized bob only (AP63) and she cannot dispatch over this session")
	}
}

// HasOutboundConnection is the question every caller was actually asking.
// Asymmetric by construction, and the negative arm is the load-bearing one:
// alice HAS a live session to bob and still cannot dispatch over it.
func TestHasOutboundConnection_IsAsymmetric(t *testing.T) {
	alice, bob, _ := listeningPairForDirection(t)

	if !bob.HasOutboundConnection(string(alice.PeerID())) {
		t.Error("bob dialled alice; HasOutboundConnection must be true")
	}
	if alice.HasOutboundConnection(string(bob.PeerID())) {
		t.Error("alice never dialled bob. Reporting true here is the defect the field " +
			"exists to prevent: an inbound-only session rendering as a working route")
	}
	// Anti-vacuity: alice really is connected, so the false above is about
	// DIRECTION and not about an empty pool.
	if connectionTo(t, alice, string(bob.PeerID())).PeerID == "" {
		t.Fatal("premise not met: alice holds no connection to bob at all, so the " +
			"assertion above measures an empty pool rather than a direction")
	}
}

func connectionTo(t *testing.T, a *entitysdk.AppPeer, peerID string) entitysdk.PeerInfo {
	t.Helper()
	for _, info := range a.ConnectedPeers() {
		if info.PeerID == peerID {
			return info
		}
	}
	return entitysdk.PeerInfo{}
}

// listeningPairForDirection stands up two listening peers and has bob dial
// alice — once, in one direction only, which is what makes the asymmetry
// above measurable.
func listeningPairForDirection(t *testing.T) (alice, bob *entitysdk.AppPeer, ctx context.Context) {
	t.Helper()

	mk := func(name string) *entitysdk.AppPeer {
		p, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
			ListenAddr: "127.0.0.1:0",
			RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
		})
		if err != nil {
			t.Fatalf("CreatePeer %s: %v", name, err)
		}
		t.Cleanup(func() { _ = p.Close() })
		return p
	}
	alice, bob = mk("alice"), mk("bob")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)

	for _, p := range []*entitysdk.AppPeer{alice, bob} {
		ready := make(chan struct{})
		errCh := make(chan error, 1)
		go func(ap *entitysdk.AppPeer) { errCh <- ap.ListenReady(ctx, ready) }(p)
		select {
		case <-ready:
		case err := <-errCh:
			t.Fatalf("listen: %v", err)
		case <-time.After(2 * time.Second):
			t.Fatal("listen timeout")
		}
	}

	if _, err := bob.Connect(ctx, alice.Addr().String()); err != nil {
		t.Fatalf("bob.Connect(alice): %v", err)
	}
	// alice's side of the wire is registered by her accept loop, which runs
	// independently of bob's Connect returning.
	deadline := time.Now().Add(3 * time.Second)
	for connectionTo(t, alice, string(bob.PeerID())).PeerID == "" {
		if time.Now().After(deadline) {
			t.Fatal("alice never registered the inbound connection")
		}
		time.Sleep(25 * time.Millisecond)
	}
	return alice, bob, ctx
}
