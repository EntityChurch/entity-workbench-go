package entitysdk_test

// Verifies ReconcileSinceLastSeen covers F3 (post-saturation catch-up)
// and F7 (post-restart catch-up) for collaborative workspace recovery.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/peer"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

func TestReconcileSinceLastSeen_FullClosure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// alice has revisioned data under `mirror/`; bob has nothing.
	// After Reconcile with base=zero, bob should have the same state.
	alice, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer alice.Close()
	bringUpListener(t, ctx, alice, "alice")

	bob, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer bob.Close()
	bringUpListener(t, ctx, bob, "bob")

	if _, err := bob.Connect(ctx, alice.Addr().String()); err != nil {
		t.Fatal(err)
	}
	if _, err := alice.Connect(ctx, bob.Addr().String()); err != nil {
		t.Fatal(err)
	}

	// Configure auto-version on alice for mirror/.
	autoTrue := true
	if _, err := alice.Revision().ConfigPut(ctx, "mirror", types.RevisionConfigData{
		Prefix:      "mirror/",
		AutoVersion: &autoTrue,
	}, nil); err != nil {
		t.Fatalf("ConfigPut: %v", err)
	}

	// Seed alice with 20 entities under mirror/.
	const seed = 20
	for i := 0; i < seed; i++ {
		path := fmt.Sprintf("mirror/file-%02d", i)
		if _, err := alice.Put(path, "test/file", map[string]interface{}{
			"i":    i,
			"body": fmt.Sprintf("content-%d", i),
		}); err != nil {
			t.Fatalf("alice seed %s: %v", path, err)
		}
	}

	// Wait for auto-version to settle on alice.
	time.Sleep(500 * time.Millisecond)

	// Pre-reconcile: bob should have NO mirror/ entries.
	preEntries, err := bob.List("mirror/")
	if err != nil {
		t.Fatalf("bob pre-list: %v", err)
	}
	if len(preEntries) != 0 {
		t.Errorf("bob already has %d mirror/ entries before reconcile", len(preEntries))
	}

	// Reconcile with base=zero → full closure pull.
	res, err := bob.ReconcileSinceLastSeen(ctx, alice.PeerID(), "mirror/", hash.Hash{})
	if err != nil {
		t.Fatalf("ReconcileSinceLastSeen: %v", err)
	}
	t.Logf("reconcile result: ingested=%d prefix=%s",
		res.EntitiesIngested, res.Prefix)
	if res.EntitiesIngested == 0 {
		t.Errorf("expected nonzero EntitiesIngested for full closure pull")
	}

	// Post-reconcile: bob should have the same 20 entries.
	postEntries, err := bob.List("mirror/")
	if err != nil {
		t.Fatalf("bob post-list: %v", err)
	}
	matched := 0
	for _, e := range postEntries {
		if !e.HasChildren {
			matched++
		}
	}
	t.Logf("bob post-reconcile mirror/ entries=%d (want >=%d)", matched, seed)
	if matched < seed {
		t.Errorf("post-reconcile bob has %d entries, want >= %d", matched, seed)
	}
}

// TestReconcileSinceLastSeen_AfterRestartDowntime is the F7 end-to-end
// scenario: bob subscribes to alice, receives some events, then closes.
// While bob is down, alice keeps writing. Bob restarts + reconciles +
// the previously-missed writes appear in bob's tree.
func TestReconcileSinceLastSeen_AfterRestartDowntime(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Setenv("HOME", t.TempDir())
	if _, err := entitysdk.CreateIdentity("alice-rc"); err != nil {
		t.Fatalf("CreateIdentity alice: %v", err)
	}
	if _, err := entitysdk.CreateIdentity("bob-rc"); err != nil {
		t.Fatalf("CreateIdentity bob: %v", err)
	}

	dir := t.TempDir()

	alice, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		Storage:    entitysdk.StorageConfig{Kind: "sqlite", Path: dir + "/alice.db"},
		Identity:   &entitysdk.IdentityBindingConfig{Name: "alice-rc"},
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer alice.Close()
	bringUpListener(t, ctx, alice, "alice")

	// Configure auto-version on alice for collab/ so the trie head
	// advances as alice writes.
	autoTrue := true
	if _, err := alice.Revision().ConfigPut(ctx, "collab", types.RevisionConfigData{
		Prefix: "collab/", AutoVersion: &autoTrue,
	}, nil); err != nil {
		t.Fatalf("ConfigPut: %v", err)
	}

	bobDB := dir + "/bob.db"
	bringUpBob := func(label string) *entitysdk.AppPeer {
		bob, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
			ListenAddr: "127.0.0.1:0",
			Storage:    entitysdk.StorageConfig{Kind: "sqlite", Path: bobDB},
			Identity:   &entitysdk.IdentityBindingConfig{Name: "bob-rc"},
			RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
		})
		if err != nil {
			t.Fatalf("bob %s: %v", label, err)
		}
		bringUpListener(t, ctx, bob, "bob "+label)
		if _, err := bob.Connect(ctx, alice.Addr().String()); err != nil {
			t.Fatalf("bob %s→alice: %v", label, err)
		}
		if _, err := alice.Connect(ctx, bob.Addr().String()); err != nil {
			t.Fatalf("alice→bob %s: %v", label, err)
		}
		return bob
	}

	// --- Phase 1: bob bootstraps via reconcile (base=zero), gets batch 1 ---
	bob := bringUpBob("v1")

	for i := 0; i < 10; i++ {
		path := fmt.Sprintf("collab/f-%02d", i)
		if _, err := alice.Put(path, "test/x", i); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(500 * time.Millisecond)

	// Bob bootstraps from full-closure reconcile.
	bootstrap, err := bob.ReconcileSinceLastSeen(ctx, alice.PeerID(), "collab/", hash.Hash{})
	if err != nil {
		t.Fatalf("bootstrap reconcile: %v", err)
	}
	t.Logf("phase1: bootstrap ingested=%d", bootstrap.EntitiesIngested)

	// Capture alice's current head — bob's "last seen" before going down.
	statusV1, err := alice.Revision().Status(ctx, "collab/")
	if err != nil {
		t.Fatalf("alice status: %v", err)
	}
	lastSeen := statusV1.Head
	t.Logf("phase1: alice head=%s after batch1", shortHashHex(lastSeen))

	// Sanity: bob has 10 entries after bootstrap.
	postBootstrap, _ := bob.List("collab/")
	bootstrapMatched := 0
	for _, e := range postBootstrap {
		if !e.HasChildren {
			bootstrapMatched++
		}
	}
	t.Logf("phase1: bob has %d collab/ entries after bootstrap", bootstrapMatched)
	if bootstrapMatched < 10 {
		t.Fatalf("bootstrap incomplete: bob has %d, want 10", bootstrapMatched)
	}

	// --- Phase 2: bob closes; alice continues writing batch 2 ---
	bob.Close()
	for i := 10; i < 25; i++ {
		path := fmt.Sprintf("collab/f-%02d", i)
		if _, err := alice.Put(path, "test/x", i); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(500 * time.Millisecond)

	statusV2, _ := alice.Revision().Status(ctx, "collab/")
	t.Logf("phase2: alice head advanced to %s during bob downtime",
		shortHashHex(statusV2.Head))

	// --- Phase 3: bob v2 reconciles incrementally ---
	bob = bringUpBob("v2")
	defer bob.Close()

	res, err := bob.ReconcileSinceLastSeen(ctx, alice.PeerID(), "collab/", lastSeen)
	if err != nil {
		t.Fatalf("ReconcileSinceLastSeen: %v", err)
	}
	t.Logf("phase3 incremental reconcile: ingested=%d (base=%s)",
		res.EntitiesIngested, shortHashHex(lastSeen))

	// Bob's tree should now have ALL 25 collab/ entries.
	postEntries, err := bob.List("collab/")
	if err != nil {
		t.Fatalf("bob post-list: %v", err)
	}
	matched := 0
	for _, e := range postEntries {
		if !e.HasChildren {
			matched++
		}
	}
	t.Logf("phase3 bob has %d collab/ entries (want 25)", matched)
	if matched < 25 {
		t.Errorf("post-reconcile bob has %d entries, want 25 (missed catch-up)", matched)
	}
}

func shortHashHex(h hash.Hash) string {
	s := h.String()
	if len(s) <= 12 {
		return s
	}
	return s[:12]
}
