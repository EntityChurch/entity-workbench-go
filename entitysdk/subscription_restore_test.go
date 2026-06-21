package entitysdk_test

// Verifies RestorePriorSubscriptions covers the §5.7 "subscriber-side
// restoration is application-level" gap measured by the Stage 5
// restart-mesh probe (F6).

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"

	"entity-workbench-go/entitysdk"
)

func TestRestorePriorSubscriptions_MeshRestart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	alice, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("alice: %v", err)
	}
	defer alice.Close()
	bringUpListener(t, ctx, alice, "alice")

	dbPath := t.TempDir() + "/bob.db"
	t.Setenv("HOME", t.TempDir())
	if _, err := entitysdk.CreateIdentity("bob-restore"); err != nil {
		t.Fatalf("CreateIdentity: %v", err)
	}
	bringUpBob := func(label string) *entitysdk.AppPeer {
		bob, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
			ListenAddr: "127.0.0.1:0",
			Storage:    entitysdk.StorageConfig{Kind: "sqlite", Path: dbPath},
			Identity:   &entitysdk.IdentityBindingConfig{Name: "bob-restore"},
			RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
		})
		if err != nil {
			t.Fatalf("bob %s: %v", label, err)
		}
		bringUpListener(t, ctx, bob, "bob "+label)
		if _, err := bob.Connect(ctx, alice.Addr().String()); err != nil {
			t.Fatalf("bob %s connect: %v", label, err)
		}
		if _, err := alice.Connect(ctx, bob.Addr().String()); err != nil {
			t.Fatalf("alice→bob %s: %v", label, err)
		}
		return bob
	}

	// --- Phase 1: bob v1 subscribes, receives some events, then closes ---
	bob := bringUpBob("v1")
	sub, err := bob.SubscribeAt(alice.PeerID(), "watched/*", entitysdk.SubscribeOpts{
		Events:         []string{"created", "updated"},
		IncludePayload: true,
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	var received1 atomic.Int64
	done1 := make(chan struct{})
	go func() {
		for range sub.Events() {
			received1.Add(1)
		}
		close(done1)
	}()

	const batch = 25
	for i := 0; i < batch; i++ {
		if _, err := alice.Put(fmt.Sprintf("watched/b1-%02d", i), "test/x", i); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(1 * time.Second)
	t.Logf("phase1: bob v1 received %d/%d", received1.Load(), batch)

	// Diagnostic: confirm sidecar visible BEFORE close.
	preCloseEntries, listErr := bob.List("sdk/subscription-tracking/")
	t.Logf("phase1 pre-close sidecar entries=%d listErr=%v", len(preCloseEntries), listErr)
	for _, e := range preCloseEntries {
		t.Logf("  - %s (hasChildren=%v)", e.Path, e.HasChildren)
	}
	// Also check raw location index — bypasses dispatch entirely.
	if liEntries := bob.RawLocationIndex().List("sdk/subscription-tracking/"); len(liEntries) > 0 {
		t.Logf("phase1 raw-li sees %d entries under sdk/subscription-tracking/:", len(liEntries))
		for _, e := range liEntries {
			t.Logf("  - rawli %s", e.Path)
		}
	} else {
		t.Logf("phase1 raw-li sees 0 entries under sdk/subscription-tracking/")
	}

	// Close subscription (but NOT remove the sidecar — that's what
	// Close on the live handle does, and we want restoration to find
	// the sidecar). Simulate crash: just close the peer, leaving the
	// SDK-tracked sidecar in place.
	bob.Close()

	// --- Phase 2: alice writes more while bob is offline ---
	for i := 0; i < batch; i++ {
		if _, err := alice.Put(fmt.Sprintf("watched/b2-%02d", i), "test/x", i); err != nil {
			t.Fatal(err)
		}
	}

	// --- Phase 3: bob v2 boots, restores subscriptions, receives new events ---
	bob = bringUpBob("v2")
	defer bob.Close()

	// Diagnostic: confirm sidecar survives reopen.
	postReopenEntries, listErr := bob.List("sdk/subscription-tracking/")
	t.Logf("phase3 post-reopen sidecar entries=%d listErr=%v", len(postReopenEntries), listErr)
	for _, e := range postReopenEntries {
		t.Logf("  - %s (hasChildren=%v)", e.Path, e.HasChildren)
	}
	if liEntries := bob.RawLocationIndex().List("sdk/subscription-tracking/"); len(liEntries) > 0 {
		t.Logf("phase3 raw-li sees %d entries:", len(liEntries))
		for _, e := range liEntries {
			t.Logf("  - rawli %s", e.Path)
		}
	} else {
		t.Logf("phase3 raw-li sees 0 entries")
	}
	// Also check raw store entity count
	t.Logf("phase3 raw bob entityCount=%d pathCount=%d", bob.EntityCount(), bob.PathCount())

	restored, errs := bob.RestorePriorSubscriptions()
	if len(errs) > 0 {
		t.Fatalf("restore errors: %v", errs)
	}
	if len(restored) != 1 {
		t.Fatalf("want 1 restored sub, got %d", len(restored))
	}
	if restored[0].RemotePeer != alice.PeerID() {
		t.Errorf("RemotePeer = %q, want %q", restored[0].RemotePeer, alice.PeerID())
	}
	if restored[0].Pattern != "watched/*" {
		t.Errorf("Pattern = %q, want %q", restored[0].Pattern, "watched/*")
	}
	if !restored[0].IncludePayload {
		t.Errorf("IncludePayload should round-trip true")
	}

	var received3 atomic.Int64
	done3 := make(chan struct{})
	go func() {
		for range restored[0].Sub.Events() {
			received3.Add(1)
		}
		close(done3)
	}()

	// --- Phase 4: alice writes new events; restored subscription delivers ---
	for i := 0; i < batch; i++ {
		if _, err := alice.Put(fmt.Sprintf("watched/b3-%02d", i), "test/x", i); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(1 * time.Second)
	got := received3.Load()
	t.Logf("phase4: restored sub delivered %d/%d new writes", got, batch)
	if got < int64(batch-2) {
		t.Errorf("post-restore delivery loss: %d of %d", got, batch)
	}

	// --- Phase 5: confirm the sidecar got re-keyed cleanly (no leak) ---
	// The original v1 sidecar is removed; a new v2 sidecar exists for
	// the restored subscription. Total: 1 active tracking entry.
	// (We can't easily count from the test without exporting a lister,
	// but the second-time-RestorePriorSubscriptions should produce
	// exactly one entry, restoring the v2 subscription itself.)
	_ = restored[0].Sub.Close()
	<-done3
}

// bringUpListener helper: identical to perfreview/restart_mesh_test's
// shape, copied here to avoid cross-package depends.
func bringUpListener(t *testing.T, ctx context.Context, ap *entitysdk.AppPeer, name string) {
	t.Helper()
	ready := make(chan struct{})
	errCh := make(chan error, 1)
	go func() { errCh <- ap.ListenReady(ctx, ready) }()
	select {
	case <-ready:
	case err := <-errCh:
		t.Fatalf("%s listen: %v", name, err)
	case <-time.After(5 * time.Second):
		t.Fatalf("%s listen timeout", name)
	}
}
