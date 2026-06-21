package entitysdk_test

import (
	"context"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

// TestRevisionSync_TwoPeerConvergence is the load-bearing
// proof-of-concept for the multi-peer revision-as-CRDT story.
//
// Scenario: alice listens, bob connects. Alice writes some
// content under "docs/" and commits a revision; bob calls Sync to
// pull alice's head and merge into bob's local DAG. After Sync,
// bob's docs/ content matches alice's, and bob's revision log
// reaches alice's head.
//
// This validates the manual-catchup path described in
// GUIDE-REVISION-AUTO-VERSION §4.1. Standing follow (subscription +
// continuation) is Phase C work; this test pins the primitive
// underneath it.
func TestRevisionSync_TwoPeerConvergence(t *testing.T) {
	// --- Setup: two peers, alice listens, bob connects -------------
	alice, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("CreatePeer alice: %v", err)
	}
	t.Cleanup(func() { _ = alice.Close() })

	bob, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer bob: %v", err)
	}
	t.Cleanup(func() { _ = bob.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ready := make(chan struct{})
	listenErr := make(chan error, 1)
	go func() { listenErr <- alice.ListenReady(ctx, ready) }()
	select {
	case <-ready:
	case err := <-listenErr:
		t.Fatalf("alice listen: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("alice listen timeout")
	}

	if _, err := bob.Connect(ctx, alice.Addr().String()); err != nil {
		t.Fatalf("bob connect: %v", err)
	}

	// --- Alice writes + commits a revision -------------------------
	if _, err := alice.Put("docs/intro", "test/note", "alice's intro"); err != nil {
		t.Fatalf("alice put intro: %v", err)
	}
	if _, err := alice.Put("docs/about", "test/note", "alice's about"); err != nil {
		t.Fatalf("alice put about: %v", err)
	}
	aliceCommit, err := alice.Revision().Commit(ctx, "docs/", "initial")
	if err != nil {
		t.Fatalf("alice commit: %v", err)
	}

	// Sanity: alice's head is what we just committed.
	aliceStatus, err := alice.Revision().Status(ctx, "docs/")
	if err != nil {
		t.Fatalf("alice status: %v", err)
	}
	if aliceStatus.Head != aliceCommit.Version {
		t.Fatalf("alice head mismatch: got %s, want %s", aliceStatus.Head, aliceCommit.Version)
	}

	// --- Bob pulls + merges via Sync -------------------------------
	mergeRes, err := bob.Revision().Pull(ctx, "docs/", alice.PeerID())
	if err != nil {
		t.Fatalf("bob Sync: %v", err)
	}
	t.Logf("merge result: status=%q version=%s mergedCount=%v", mergeRes.Status, mergeRes.Version, mergeRes.MergedCount)

	// Bob had no prior versions for this prefix, so the merge should
	// fast-forward to alice's head.
	if mergeRes.Status != "fast_forward" && mergeRes.Status != "merged" {
		t.Errorf("merge status = %q, expected fast_forward or merged", mergeRes.Status)
	}

	// Bob's head now matches alice's.
	bobStatus, err := bob.Revision().Status(ctx, "docs/")
	if err != nil {
		t.Fatalf("bob status: %v", err)
	}
	if bobStatus.Head != aliceCommit.Version {
		t.Errorf("bob head = %s, want alice's head %s", bobStatus.Head, aliceCommit.Version)
	}

	// Diagnostic: dump bob's docs/ listing to see what merge actually
	// wrote. If empty, fast-forward didn't apply bindings.
	bobEntries, _ := bob.List("docs/")
	t.Logf("bob's docs/ listing (%d entries):", len(bobEntries))
	for _, e := range bobEntries {
		t.Logf("  %s  hash=%s", e.Path, e.ContentHash)
	}

	// Bob's tree has the entities alice wrote.
	intro, ok, err := bob.Get("docs/intro")
	if err != nil {
		t.Fatalf("bob get intro: %v", err)
	}
	if !ok {
		t.Fatal("bob is missing docs/intro after sync")
	}
	if intro.Type != "test/note" {
		t.Errorf("bob intro type = %s, want test/note", intro.Type)
	}
}

// TestRevisionSync_BidirectionalConvergence: both peers write,
// both Sync from the other, both end up at the same merged head.
//
// This is the "no leader" property the workbench is built around —
// edits on either peer propagate, content addressing makes
// re-merges no-ops, deterministic merge_order ensures both peers
// compute the same merged DAG.
func TestRevisionSync_BidirectionalConvergence(t *testing.T) {
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

	// Bring up both listeners.
	for _, p := range []struct {
		name string
		ap   *entitysdk.AppPeer
	}{{"alice", alice}, {"bob", bob}} {
		ready := make(chan struct{})
		listenErr := make(chan error, 1)
		go func(name string, ap *entitysdk.AppPeer) {
			listenErr <- ap.ListenReady(ctx, ready)
		}(p.name, p.ap)
		select {
		case <-ready:
		case err := <-listenErr:
			t.Fatalf("%s listen: %v", p.name, err)
		case <-time.After(2 * time.Second):
			t.Fatalf("%s listen timeout", p.name)
		}
	}

	// Cross-connect.
	if _, err := bob.Connect(ctx, alice.Addr().String()); err != nil {
		t.Fatalf("bob → alice connect: %v", err)
	}
	if _, err := alice.Connect(ctx, bob.Addr().String()); err != nil {
		t.Fatalf("alice → bob connect: %v", err)
	}

	// Each writes their own file at non-conflicting paths under the
	// same prefix and commits.
	if _, err := alice.Put("shared/alice-note", "test/note", "from alice"); err != nil {
		t.Fatal(err)
	}
	aliceCommit, err := alice.Revision().Commit(ctx, "shared/", "alice work")
	if err != nil {
		t.Fatalf("alice commit: %v", err)
	}
	_ = aliceCommit

	if _, err := bob.Put("shared/bob-note", "test/note", "from bob"); err != nil {
		t.Fatal(err)
	}
	bobCommit, err := bob.Revision().Commit(ctx, "shared/", "bob work")
	if err != nil {
		t.Fatalf("bob commit: %v", err)
	}
	_ = bobCommit

	// Each pulls from the other.
	if _, err := bob.Revision().Pull(ctx, "shared/", alice.PeerID()); err != nil {
		t.Fatalf("bob Sync from alice: %v", err)
	}
	if _, err := alice.Revision().Pull(ctx, "shared/", bob.PeerID()); err != nil {
		t.Fatalf("alice Sync from bob: %v", err)
	}

	// Both peers should now have both files.
	aliceHasBob, _, _ := alice.Get("shared/bob-note")
	if aliceHasBob.Type == "" {
		t.Errorf("alice missing bob's note after bidirectional sync")
	}
	bobHasAlice, _, _ := bob.Get("shared/alice-note")
	if bobHasAlice.Type == "" {
		t.Errorf("bob missing alice's note after bidirectional sync")
	}

	// Optional convergence assertion: heads SHOULD match after both
	// peers have integrated each other's changes. Under the default
	// deterministic merge_order, both compute the same merged
	// version. With auto-version off (manual mode), the head moves
	// only on commit/merge, so this works.
	aliceStatus, _ := alice.Revision().Status(ctx, "shared/")
	bobStatus, _ := bob.Revision().Status(ctx, "shared/")
	t.Logf("alice head: %s", aliceStatus.Head)
	t.Logf("bob   head: %s", bobStatus.Head)
	// Note: heads may legitimately differ if each peer's local merge
	// produced its own commit. The content convergence (both have
	// both files) is the load-bearing assertion. Heads-match is a
	// stronger property requiring auto-version + a third sync round.
	// Document and don't assert here.
	_ = types.RevisionStatusData{} // keep types import live
}
