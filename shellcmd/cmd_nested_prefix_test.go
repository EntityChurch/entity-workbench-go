package shellcmd_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

// TestE2E_NestedPrefix_ConcurrentWritesConverge verifies the
// nested-prefix policy adopted per PROPOSAL-DELETION-MARKERS Amendment 2
// (option b): when two revision-tracked configs cover the same path
// (parent + nested), each config's DAG converges independently and
// eventually. The spec permits cross-config temporal partial-state in
// intermediate version entries — those entries reflect mid-flight
// tree state but the system converges given finite operations.
//
// Setup:
//   - Two peers, each configured with TWO auto-versioned revision configs:
//     archives/ (parent) and archives/notes/ (nested).
//   - Each peer follows the other peer's heads for BOTH prefixes.
//
// Workload:
//   - Concurrent burst of writes under archives/notes/X on both peers.
//   - Each write fires auto-version for BOTH configs (the parent
//     archives/ DAG and the nested archives/notes/ DAG).
//
// Expected (eventual-consistency contract, not strong-consistency):
//   - Both peers' live trees contain every written entry.
//   - Both peers' archives/ DAG heads converge to the same hash.
//   - Both peers' archives/notes/ DAG heads converge to the same hash.
//   - Each DAG may have intermediate versions reflecting partial states
//     during the burst window; what matters is the final convergent state.
//
// Per option (b): we do NOT assert anything about intermediate versions.
// We only assert eventual convergence on each DAG and live-tree completeness.
func TestE2E_NestedPrefix_ConcurrentWritesConverge(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const parentPrefix = "archives/"
	const nestedPrefix = "archives/notes/"
	a, b := bringUpNestedPair(t, ctx, parentPrefix, nestedPrefix)

	const burst = 5
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < burst; i++ {
			path := fmt.Sprintf("%sa-%d.md", nestedPrefix, i)
			if _, err := a.ap.Put(path, "doc/markdown-file", map[string]interface{}{
				"path": path, "content": fmt.Sprintf("# a %d\n", i),
			}); err != nil {
				t.Errorf("alice put a-%d: %v", i, err)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < burst; i++ {
			path := fmt.Sprintf("%sb-%d.md", nestedPrefix, i)
			if _, err := b.ap.Put(path, "doc/markdown-file", map[string]interface{}{
				"path": path, "content": fmt.Sprintf("# b %d\n", i),
			}); err != nil {
				t.Errorf("bob put b-%d: %v", i, err)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	wg.Wait()
	if t.Failed() {
		return
	}

	expectedPaths := make([]string, 0, burst*2)
	for i := 0; i < burst; i++ {
		expectedPaths = append(expectedPaths,
			fmt.Sprintf("%sa-%d.md", nestedPrefix, i),
			fmt.Sprintf("%sb-%d.md", nestedPrefix, i))
	}

	// Eventual-consistency assertion: both DAGs converge + live trees complete.
	converged := waitFor(30*time.Second, func() bool {
		// Each DAG's heads must match cross-peer.
		aParent, _ := a.ap.Revision().Status(ctx, parentPrefix)
		bParent, _ := b.ap.Revision().Status(ctx, parentPrefix)
		if aParent.Head.IsZero() || aParent.Head != bParent.Head {
			return false
		}
		aNested, _ := a.ap.Revision().Status(ctx, nestedPrefix)
		bNested, _ := b.ap.Revision().Status(ctx, nestedPrefix)
		if aNested.Head.IsZero() || aNested.Head != bNested.Head {
			return false
		}
		// Live trees complete on both peers.
		for _, p := range expectedPaths {
			if !a.ap.Store().Has(p) || !b.ap.Store().Has(p) {
				return false
			}
		}
		return true
	})
	if !converged {
		aParent, _ := a.ap.Revision().Status(ctx, parentPrefix)
		bParent, _ := b.ap.Revision().Status(ctx, parentPrefix)
		aNested, _ := a.ap.Revision().Status(ctx, nestedPrefix)
		bNested, _ := b.ap.Revision().Status(ctx, nestedPrefix)
		t.Logf("convergence diagnostic:")
		t.Logf("  archives/ heads      alice=%s bob=%s match=%v",
			aParent.Head, bParent.Head, aParent.Head == bParent.Head)
		t.Logf("  archives/notes/ heads alice=%s bob=%s match=%v",
			aNested.Head, bNested.Head, aNested.Head == bNested.Head)
		var aMissing, bMissing []string
		for _, p := range expectedPaths {
			if !a.ap.Store().Has(p) {
				aMissing = append(aMissing, p)
			}
			if !b.ap.Store().Has(p) {
				bMissing = append(bMissing, p)
			}
		}
		t.Logf("  alice missing: %v", aMissing)
		t.Logf("  bob missing:   %v", bMissing)
		t.Fatalf("nested-prefix concurrent burst did NOT converge within 30s — option (b) eventual-consistency contract violated")
	}
	t.Logf("nested-prefix burst converged: %d live entries on both peers, both DAGs head-equal", burst*2)
}

// TestE2E_NestedPrefix_DeletePropagatesAcrossBothDAGs verifies that a
// delete at a path covered by both nested configs propagates correctly
// in both DAGs:
//
//   - Both peers have entity E at archives/notes/X bound + converged.
//   - alice removes archives/notes/X.
//   - The remove fires auto-version for BOTH configs (archives/ and
//     archives/notes/), each emitting a deletion marker for archives/notes/X
//     in its respective DAG.
//   - Bob receives both DAGs' new heads, applies the marker translations,
//     and unbinds archives/notes/X from his live tree.
//
// This verifies the apply-translation path works uniformly across both
// covering configs.
func TestE2E_NestedPrefix_DeletePropagatesAcrossBothDAGs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const parentPrefix = "archives/"
	const nestedPrefix = "archives/notes/"
	const path = nestedPrefix + "to-delete-from-both.md"
	a, b := bringUpNestedPair(t, ctx, parentPrefix, nestedPrefix)

	// 1. Bind + converge.
	if _, err := a.ap.Put(path, "doc/markdown-file", map[string]interface{}{
		"path": path, "content": "# initial\n",
	}); err != nil {
		t.Fatalf("alice put: %v", err)
	}
	if !waitFor(10*time.Second, func() bool { return b.ap.Store().Has(path) }) {
		t.Fatalf("initial propagation failed")
	}
	if !waitFor(5*time.Second, func() bool {
		aP, _ := a.ap.Revision().Status(ctx, parentPrefix)
		bP, _ := b.ap.Revision().Status(ctx, parentPrefix)
		aN, _ := a.ap.Revision().Status(ctx, nestedPrefix)
		bN, _ := b.ap.Revision().Status(ctx, nestedPrefix)
		return !aP.Head.IsZero() && aP.Head == bP.Head &&
			!aN.Head.IsZero() && aN.Head == bN.Head
	}) {
		t.Fatalf("initial heads didn't converge across both DAGs")
	}
	t.Logf("step 1 OK: both peers have %s, both DAGs converged", path)

	// 2. Delete on alice.
	if err := a.ap.Remove(path); err != nil {
		t.Fatalf("alice remove: %v", err)
	}
	t.Logf("step 2 OK: alice removed %s", path)

	// 3. Wait for bob's live tree to unbind. Eventual consistency across
	// BOTH DAGs — bob's converge handlers (one per prefix) each pull
	// and apply; the apply translates the marker in whichever DAG's
	// merged trie carries it (both will, since both auto-versioners
	// emitted markers for the path).
	if !waitFor(15*time.Second, func() bool { return !b.ap.Store().Has(path) }) {
		t.Fatalf("bob's tree still has %s 15s after delete; propagation failed", path)
	}
	t.Logf("step 3 OK: bob's tree unbound %s after delete propagated through both DAGs", path)

	// Both DAGs converge on the post-delete state.
	if !waitFor(10*time.Second, func() bool {
		aP, _ := a.ap.Revision().Status(ctx, parentPrefix)
		bP, _ := b.ap.Revision().Status(ctx, parentPrefix)
		aN, _ := a.ap.Revision().Status(ctx, nestedPrefix)
		bN, _ := b.ap.Revision().Status(ctx, nestedPrefix)
		return !aP.Head.IsZero() && aP.Head == bP.Head &&
			!aN.Head.IsZero() && aN.Head == bN.Head
	}) {
		t.Fatalf("post-delete heads didn't converge across both DAGs")
	}
	t.Logf("nested-prefix delete propagation OK end-to-end across both DAGs")
}

// bringUpNestedPair stands up two peers with TWO nested
// auto-versioned revision configs: parentPrefix and nestedPrefix.
// Each peer follows the other on BOTH prefixes (two follow chains
// per direction = four total). This is the fixture for nested-prefix
// coordination tests.
func bringUpNestedPair(t *testing.T, ctx context.Context, parentPrefix, nestedPrefix string) (*bidiPeer, *bidiPeer) {
	t.Helper()
	a := newNestedPeer(t, "alice")
	b := newNestedPeer(t, "bob")
	t.Cleanup(func() { _ = a.ap.Close() })
	t.Cleanup(func() { _ = b.ap.Close() })

	for _, p := range []*bidiPeer{a, b} {
		ready := make(chan struct{})
		errCh := make(chan error, 1)
		go func(ap *entitysdk.AppPeer) {
			errCh <- ap.ListenReady(ctx, ready)
		}(p.ap)
		select {
		case <-ready:
		case err := <-errCh:
			t.Fatalf("%s listen: %v", p.rootName, err)
		case <-time.After(2 * time.Second):
			t.Fatalf("%s listen timeout", p.rootName)
		}
	}
	if _, err := b.ap.Connect(ctx, a.ap.Addr().String()); err != nil {
		t.Fatalf("bob→alice connect: %v", err)
	}
	if _, err := a.ap.Connect(ctx, b.ap.Addr().String()); err != nil {
		t.Fatalf("alice→bob connect: %v", err)
	}

	// Configure both revision-tracked prefixes on both peers.
	autoTrue := true
	for _, p := range []*bidiPeer{a, b} {
		if _, err := p.ap.Revision().ConfigPut(ctx, "archives", types.RevisionConfigData{
			Prefix:      parentPrefix,
			AutoVersion: &autoTrue,
		}, nil); err != nil {
			t.Fatalf("%s parent config: %v", p.rootName, err)
		}
		if _, err := p.ap.Revision().ConfigPut(ctx, "notes", types.RevisionConfigData{
			Prefix:      nestedPrefix,
			AutoVersion: &autoTrue,
		}, nil); err != nil {
			t.Fatalf("%s nested config: %v", p.rootName, err)
		}
	}
	// Each peer follows the other on BOTH prefixes (4 follow chains total
	// across the pair). The converge handler is keyed by remote-head URI,
	// which differs per prefix, so a single handler instance services
	// both prefixes' follow chains.
	a.installFollow(t, b, parentPrefix)
	a.installFollow(t, b, nestedPrefix)
	b.installFollow(t, a, parentPrefix)
	b.installFollow(t, a, nestedPrefix)
	return a, b
}

func newNestedPeer(t *testing.T, rootName string) *bidiPeer {
	t.Helper()
	cfg := entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	}
	ap, err := entitysdk.CreatePeer(cfg)
	if err != nil {
		t.Fatalf("%s create: %v", rootName, err)
	}
	return &bidiPeer{
		ap:       ap,
		rootName: rootName,
		id:       ap.PeerID(),
	}
}
