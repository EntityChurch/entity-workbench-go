package entitysdk_test

import (
	"context"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"

	"entity-workbench-go/entitysdk"
)

// TestSubscribeAt_CrossPeerNotification: bob subscribes on alice's
// "shared/*" prefix; alice writes; bob's channel receives a
// notification routed back across the wire.
//
// This is the primitive that makes Phase C revision-follow possible.
// The notification path: alice.Put → alice's subscription engine
// matches the pattern → engine dispatches `receive` to the deliver
// URI (entity://{bobID}/system/inbox/sdk-sub-{id}) → bob's inbox
// handler converts to ChangeEvent → bob's events channel.
func TestSubscribeAt_CrossPeerNotification(t *testing.T) {
	alice, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("alice: %v", err)
	}
	t.Cleanup(func() { _ = alice.Close() })

	bob, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("bob: %v", err)
	}
	t.Cleanup(func() { _ = bob.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Bring up both listeners and cross-connect so alice can dispatch
	// notifications back to bob through the connection pool.
	for _, p := range []struct {
		name string
		ap   *entitysdk.AppPeer
	}{{"alice", alice}, {"bob", bob}} {
		ready := make(chan struct{})
		errCh := make(chan error, 1)
		go func(name string, ap *entitysdk.AppPeer) {
			errCh <- ap.ListenReady(ctx, ready)
		}(p.name, p.ap)
		select {
		case <-ready:
		case err := <-errCh:
			t.Fatalf("%s listen: %v", p.name, err)
		case <-time.After(2 * time.Second):
			t.Fatalf("%s listen timeout", p.name)
		}
	}
	if _, err := bob.Connect(ctx, alice.Addr().String()); err != nil {
		t.Fatalf("bob→alice connect: %v", err)
	}
	if _, err := alice.Connect(ctx, bob.Addr().String()); err != nil {
		t.Fatalf("alice→bob connect: %v", err)
	}

	// Bob subscribes on alice's "shared/*" prefix.
	sub, err := bob.SubscribeAt(alice.PeerID(), "shared/*", entitysdk.SubscribeOpts{})
	if err != nil {
		t.Fatalf("SubscribeAt: %v", err)
	}
	t.Cleanup(func() { _ = sub.Close() })

	if sub.RemotePeer() != alice.PeerID() {
		t.Errorf("RemotePeer = %q, want %q", sub.RemotePeer(), alice.PeerID())
	}

	// Alice writes into shared/. Bob should receive an event.
	if _, err := alice.Put("shared/note", "test/note", "from alice"); err != nil {
		t.Fatalf("alice put: %v", err)
	}

	select {
	case evt := <-sub.Events():
		t.Logf("received: type=%v path=%q hash=%v", evt.EventType, evt.Path, evt.NewHash)
		if evt.Path == "" {
			t.Errorf("event path empty")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("did not receive cross-peer notification within 5s")
	}
}

// TestSubscribeAt_LocalDelegatesToSubscribe: when peerID == local,
// SubscribeAt behaves exactly like Subscribe (notification arrives
// without needing a connection).
func TestSubscribeAt_LocalDelegatesToSubscribe(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	sub, err := ap.SubscribeAt(ap.PeerID(), "local/*", entitysdk.SubscribeOpts{})
	if err != nil {
		t.Fatalf("SubscribeAt local: %v", err)
	}
	t.Cleanup(func() { _ = sub.Close() })

	if sub.RemotePeer() != ap.PeerID() {
		t.Errorf("RemotePeer should be local peer for local subscribe; got %q want %q",
			sub.RemotePeer(), ap.PeerID())
	}

	if _, err := ap.Put("local/x", "test/note", "hi"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-sub.Events():
	case <-time.After(2 * time.Second):
		t.Fatal("no local event")
	}
}
