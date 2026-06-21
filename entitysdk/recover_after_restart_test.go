package entitysdk_test

// Verifies RecoverAfterRestart composes Connect + RestorePriorSubscriptions
// + ReconcileSinceLastSeen into the documented one-call sequence per
// GUIDE-CONTINUATIONS-WORKBENCH §6.5 / HANDOFF-WORKBENCH-STAGE-5-FOLLOWUPS
// Lane 5.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"
	coretypes "go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

func TestRecoverAfterRestart_HappyPath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Alice is the publisher; persistent across the test.
	alice, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("alice: %v", err)
	}
	defer alice.Close()
	bringUpListener(t, ctx, alice, "alice")

	// Configure auto-version on alice's watched/ prefix so the
	// reconcile step has a revision head to diff against.
	yes := true
	revCfg := coretypes.RevisionConfigData{Prefix: "watched/", AutoVersion: &yes}
	if _, err := alice.Revision().Config(ctx, coretypes.RevisionConfigParamsData{
		Action: "set", Name: "recover-happy", Config: &revCfg,
	}); err != nil {
		t.Fatalf("alice auto-version config: %v", err)
	}

	// Bob has an on-disk identity + sqlite store so subscription
	// tracking sidecar survives close/reopen.
	dbPath := t.TempDir() + "/bob.db"
	t.Setenv("HOME", t.TempDir())
	if _, err := entitysdk.CreateIdentity("bob-recover"); err != nil {
		t.Fatalf("CreateIdentity: %v", err)
	}
	bringUpBob := func(label string) *entitysdk.AppPeer {
		bob, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
			ListenAddr: "127.0.0.1:0",
			Storage:    entitysdk.StorageConfig{Kind: "sqlite", Path: dbPath},
			Identity:   &entitysdk.IdentityBindingConfig{Name: "bob-recover"},
			RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
		})
		if err != nil {
			t.Fatalf("bob %s: %v", label, err)
		}
		bringUpListener(t, ctx, bob, "bob "+label)
		return bob
	}

	// --- Phase 1: bob v1 subscribes (creates tracking sidecar) ---
	bob1 := bringUpBob("v1")
	if _, err := bob1.Connect(ctx, alice.Addr().String()); err != nil {
		t.Fatalf("bob v1 → alice: %v", err)
	}
	if _, err := alice.Connect(ctx, bob1.Addr().String()); err != nil {
		t.Fatalf("alice → bob v1: %v", err)
	}

	sub, err := bob1.SubscribeAt(alice.PeerID(), "watched/*", entitysdk.SubscribeOpts{})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	// Drain the sub channel briefly so the substrate sees the subscription
	// as live before we close — closes the F6 race where the unsubscribe
	// would fire before any deliveries arrive.
	go func() {
		for range sub.Events() {
		}
	}()
	time.Sleep(200 * time.Millisecond)

	// Crash-simulate: close bob without closing the subscription. The
	// tracking sidecar persists since removeSubscriptionTracking only
	// fires from Subscription.Close.
	bob1.Close()

	// --- Phase 2: alice writes data bob missed ---
	const missed = 10
	for i := 0; i < missed; i++ {
		if _, err := alice.Put(fmt.Sprintf("watched/missed-%02d", i),
			"test/missed", i); err != nil {
			t.Fatalf("alice Put: %v", err)
		}
	}

	// --- Phase 3: bob v2 starts up and recovers in one call ---
	bob2 := bringUpBob("v2")
	defer bob2.Close()

	res, err := bob2.RecoverAfterRestart(ctx, alice.PeerID(), alice.Addr().String())
	if err != nil {
		t.Fatalf("RecoverAfterRestart returned hard error: %v", err)
	}

	t.Logf("recovery result: connected=%v restored=%d reconciled=%d errors=%d",
		res.Connected, len(res.Restored), len(res.Reconciled), len(res.Errors))
	for _, e := range res.Errors {
		t.Logf("  step=%s detail=%s err=%v", e.Step, e.Detail, e.Err)
	}
	for i, r := range res.Reconciled {
		t.Logf("  reconcile[%d]: prefix=%s entities-ingested=%d",
			i, r.Prefix, r.EntitiesIngested)
	}

	// Assertions: connect succeeded, one subscription restored against
	// alice, one reconcile result with non-zero entities.
	if !res.Connected {
		t.Fatalf("expected Connected=true, got false")
	}
	if len(res.Restored) != 1 {
		t.Fatalf("expected 1 restored subscription, got %d", len(res.Restored))
	}
	if got := res.Restored[0].RemotePeer; got != alice.PeerID() {
		t.Errorf("restored sub against unexpected peer: %s", got)
	}
	if got := res.Restored[0].Pattern; got != "watched/*" {
		t.Errorf("restored sub has unexpected pattern: %s", got)
	}
	if len(res.Reconciled) != 1 {
		t.Fatalf("expected 1 reconcile result, got %d", len(res.Reconciled))
	}
	if got := res.Reconciled[0].Prefix; got != "watched/" {
		t.Errorf("reconcile prefix unexpected: %s", got)
	}
	if res.Reconciled[0].EntitiesIngested == 0 {
		t.Errorf("expected entities ingested > 0 (alice wrote %d), got 0", missed)
	}
	if len(res.Errors) > 0 {
		t.Errorf("expected no per-step errors on happy path, got %d", len(res.Errors))
	}
}

func TestRecoverAfterRestart_ConnectFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Setenv("HOME", t.TempDir())
	bob, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("bob: %v", err)
	}
	defer bob.Close()
	bringUpListener(t, ctx, bob, "bob")

	// Use a clearly unreachable address.
	const badAddr = "127.0.0.1:1" // privileged port, will fail
	res, err := bob.RecoverAfterRestart(ctx, "bogus-peer-id", badAddr)
	if err == nil {
		t.Fatalf("expected hard error from RecoverAfterRestart, got nil; res=%+v", res)
	}
	if res.Connected {
		t.Errorf("expected Connected=false on connect failure")
	}
	if len(res.Errors) == 0 || res.Errors[0].Step != "connect" {
		t.Errorf("expected connect step error, got %+v", res.Errors)
	}
}

func TestRecoverAfterRestart_InvalidArgs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	t.Setenv("HOME", t.TempDir())
	bob, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("bob: %v", err)
	}
	defer bob.Close()

	if _, err := bob.RecoverAfterRestart(ctx, "", "127.0.0.1:9999"); err == nil {
		t.Errorf("expected error for empty publisherPeerID")
	}
	if _, err := bob.RecoverAfterRestart(ctx, "some-peer", ""); err == nil {
		t.Errorf("expected error for empty publisherAddr")
	}
}
