package shellcmd_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"
	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/localfiles"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"
)

// TestStage3_F9_BisectDualWatch isolates whether F9's bob-side
// silent-failure is (a) a fundamental dual-watcher bug — bob's watcher
// never works when alice also has a watcher — or (b) post-materialize
// state corruption — bob's watcher works in isolation but stops after
// bob processes a materialization from alice.
//
// Setup matches case 2: both peers AddRoot + StartWatching, both peers
// subscribe to each other's source prefix, both peers have blob-resolve
// handlers wired. Then the test runs three phases in order:
//
//   - Phase 0 (pre-materialize): bob writes a file BEFORE alice writes
//     anything. If bob's watcher binds this to bob's tree, dual-watch
//     works in isolation (rules out hypothesis a).
//   - Phase 1: alice writes a file, bob materializes it via the
//     subscription chain. This is the known-working α→β direction.
//   - Phase 2 (post-materialize): bob writes a NEW file (different
//     name). If bob's tree binds it, the watcher survives the
//     materialize cycle. If not — hypothesis (b) confirmed: bob's
//     watcher is corrupted by handling a local/files:write content-mode
//     dispatch.
//
// Round 6 update to F9 routing: original Round 4 diagnosis assumed the
// subscription engine was at fault. This bisect re-routes the finding
// to localfiles watcher state interaction with content-mode writes if
// Phase 0 binds + Phase 2 doesn't.
//
// Test is NOT skipped — we want this diagnostic data captured in CI
// every run until F9 closes. Test logs all four observable states
// (bob's tree pre-phase0, post-phase0, post-phase1, post-phase2) so
// a core-team diagnoser can correlate against their own instrumentation.
func TestStage3_F9_BisectDualWatch(t *testing.T) {
	aliceDir := t.TempDir()
	bobDir := t.TempDir()
	const rootName = "sync"
	const sourcePrefix = "local/files/" + rootName + "/"

	aliceBR := workbench.NewBlobResolveHandler()
	aliceBR.RegisterMount(sourcePrefix, sourcePrefix)
	bobBR := workbench.NewBlobResolveHandler()
	bobBR.RegisterMount(sourcePrefix, sourcePrefix)

	alice, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
		Handlers: []entitysdk.HandlerRegistration{
			{Pattern: workbench.BlobResolvePattern, Handler: aliceBR},
			{Pattern: workbench.ChainErrorsPattern, Handler: workbench.NewChainErrorsHandler()},
		},
	})
	if err != nil {
		t.Fatalf("CreatePeer alice: %v", err)
	}
	t.Cleanup(func() { _ = alice.Close() })

	bob, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
		Handlers: []entitysdk.HandlerRegistration{
			{Pattern: workbench.BlobResolvePattern, Handler: bobBR},
			{Pattern: workbench.ChainErrorsPattern, Handler: workbench.NewChainErrorsHandler()},
		},
	})
	if err != nil {
		t.Fatalf("CreatePeer bob: %v", err)
	}
	t.Cleanup(func() { _ = bob.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	bringUpListener(t, ctx, alice, "alice")
	bringUpListener(t, ctx, bob, "bob")
	if _, err := alice.Connect(ctx, bob.Addr().String()); err != nil {
		t.Fatalf("alice→bob connect: %v", err)
	}
	if _, err := bob.Connect(ctx, alice.Addr().String()); err != nil {
		t.Fatalf("bob→alice connect: %v", err)
	}

	aliceID := alice.PeerID()
	bobID := bob.PeerID()

	for _, p := range []struct {
		name   string
		ap     *entitysdk.AppPeer
		fsRoot string
	}{
		{"alice", alice, aliceDir},
		{"bob", bob, bobDir},
	} {
		lf := p.ap.LocalFilesHandler()
		if err := lf.AddRoot(rootName, localfiles.RootConfigData{
			Prefix:         sourcePrefix,
			FilesystemRoot: p.fsRoot,
		}, p.ap.RawContentStore(), p.ap.RawLocationIndex()); err != nil {
			t.Fatalf("%s AddRoot: %v", p.name, err)
		}
		if err := lf.StartWatching(ctx, rootName, p.ap.RawContentStore(),
			p.ap.RawLocationIndex(), p.ap.IdentityHash()); err != nil {
			t.Fatalf("%s StartWatching: %v", p.name, err)
		}
	}

	for _, p := range []struct {
		name   string
		ap     *entitysdk.AppPeer
		other  *entitysdk.AppPeer
		thisID string
	}{
		{"alice", alice, bob, aliceID},
		{"bob", bob, alice, bobID},
	} {
		chainGrants := []types.GrantEntry{{
			Handlers:   types.CapabilityScope{Include: []string{workbench.BlobResolvePattern}},
			Operations: types.CapabilityScope{Include: []string{"receive"}},
		}}
		if _, err := p.ap.MintChainCapabilityBound(chainGrants,
			"system/capability/grants/chain/blob-resolve/"+rootName); err != nil {
			t.Fatalf("%s mint chain cap: %v", p.name, err)
		}
		otherID := p.other.PeerID()
		deliverURI := fmt.Sprintf("entity://%s/%s", p.thisID, workbench.BlobResolvePattern)
		if _, err := p.ap.SubscribeRawAt(otherID, sourcePrefix+"*", deliverURI, "receive",
			entitysdk.SubscribeOpts{
				Events:         []string{"created", "updated"},
				IncludePayload: true,
			}); err != nil {
			t.Fatalf("%s subscribe to %s: %v", p.name, otherID, err)
		}
	}

	logBobTree := func(label string) {
		entries := listPrefix(bob, sourcePrefix)
		t.Logf("%s — bob tree under %s: %d entries", label, sourcePrefix, len(entries))
		for _, e := range entries {
			t.Logf("    %s", e)
		}
	}

	// ============== PHASE 0: bob writes pre-materialize ==============
	logBobTree("PHASE 0 PRE: before any writes")
	phase0Content := "phase 0: bob writes before alice has done anything\n"
	phase0Path := filepath.Join(bobDir, "phase0-bob.md")
	if err := os.WriteFile(phase0Path, []byte(phase0Content), 0600); err != nil {
		t.Fatalf("phase 0 write: %v", err)
	}

	phase0Bound := pollBoundDeadline(bob, "phase0-bob.md", sourcePrefix, 10*time.Second)
	if phase0Bound {
		t.Logf("PHASE 0 RESULT: bob's watcher bound phase0-bob.md ✓ — dual-watch works in isolation")
	} else {
		t.Logf("PHASE 0 RESULT: bob's watcher did NOT bind phase0-bob.md within 10s ✗")
	}
	logBobTree("PHASE 0 POST")

	// ============== PHASE 1: alice writes, bob materializes ==============
	phase1Content := "phase 1: alice writes; bob materializes\n"
	phase1Path := filepath.Join(aliceDir, "phase1-alice.md")
	if err := os.WriteFile(phase1Path, []byte(phase1Content), 0600); err != nil {
		t.Fatalf("phase 1 write: %v", err)
	}

	phase1MaterializedOnBob := pollFSDeadline(filepath.Join(bobDir, "phase1-alice.md"), 15*time.Second)
	if phase1MaterializedOnBob {
		t.Logf("PHASE 1 RESULT: bob's FS received phase1-alice.md ✓ — α→β subscription chain works")
	} else {
		t.Logf("PHASE 1 RESULT: bob's FS did NOT receive phase1-alice.md within 15s ✗")
	}
	logBobTree("PHASE 1 POST")

	// Give bob's watcher a moment to settle from the materialization
	// before we test if it still works.
	time.Sleep(2 * time.Second)

	// ============== PHASE 2: bob writes post-materialize ==============
	logBobTree("PHASE 2 PRE: about to test watcher post-materialize")
	phase2Content := "phase 2: bob writes after bob materialized alice's file\n"
	phase2Path := filepath.Join(bobDir, "phase2-bob.md")
	if err := os.WriteFile(phase2Path, []byte(phase2Content), 0600); err != nil {
		t.Fatalf("phase 2 write: %v", err)
	}

	phase2Bound := pollBoundDeadline(bob, "phase2-bob.md", sourcePrefix, 10*time.Second)
	if phase2Bound {
		t.Logf("PHASE 2 RESULT: bob's watcher bound phase2-bob.md ✓ — watcher survives materialize")
	} else {
		t.Logf("PHASE 2 RESULT: bob's watcher did NOT bind phase2-bob.md within 10s ✗ — POST-MATERIALIZE CORRUPTION confirmed")
	}
	logBobTree("PHASE 2 POST")

	// Diagnostic summary block — read by the human reviewer.
	t.Logf("F9 BISECT SUMMARY:")
	t.Logf("  Phase 0 (bob writes pre-materialize):  bound=%v", phase0Bound)
	t.Logf("  Phase 1 (bob materializes alice):      received=%v", phase1MaterializedOnBob)
	t.Logf("  Phase 2 (bob writes post-materialize): bound=%v", phase2Bound)
	t.Logf("INTERPRETATION:")
	switch {
	case phase0Bound && phase1MaterializedOnBob && phase2Bound:
		t.Logf("  All three phases succeed — F9 CLOSED (core-go handleWrite markWritten + workbench blob-resolve idempotency).")
		t.Logf("  Regression-block: this test fails CI if any phase breaks again.")
	case !phase0Bound:
		t.Logf("  Phase 0 failed — bob's watcher fundamentally broken when both peers watch.")
		t.Logf("  Owner: localfiles watcher / dual-mount interaction.")
		t.Errorf("F9 regression: Phase 0 (watcher in isolation) broken")
	case !phase1MaterializedOnBob:
		t.Logf("  Phase 1 failed — α→β subscription chain broken. Could indicate")
		t.Logf("  the F9 loop is back and saturating CPU before the chain can complete.")
		t.Errorf("F9 regression: Phase 1 (α→β materialization) broken")
	case !phase2Bound:
		t.Logf("  Phase 2 failed — bob's watcher stopped working after the α→β materialization.")
		t.Logf("  Original F9 mode: dispatch-driven writes feeding back through watcher / subscription engine.")
		t.Errorf("F9 regression: Phase 2 (watcher post-materialize) broken")
	}

	// This test is INTENTIONALLY not Fail/Pass on Phase outcomes —
	// it's a diagnostic data capture. Asserting failure would just
	// duplicate case-2's assertion. The value here is the diagnostic
	// trace, which a core-team diagnoser reads to route the fix.
}

// pollBoundDeadline waits until the named file appears in the local
// peer's tree under the prefix.
func pollBoundDeadline(ap *entitysdk.AppPeer, fileName, prefix string, budget time.Duration) bool {
	deadline := time.Now().Add(budget)
	wantSuffix := "/" + prefix + fileName
	for time.Now().Before(deadline) {
		entries := listPrefix(ap, prefix)
		for _, e := range entries {
			if len(e) >= len(wantSuffix) && e[len(e)-len(wantSuffix):] == wantSuffix {
				return true
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// pollFSDeadline waits until the named filesystem path exists.
func pollFSDeadline(fsPath string, budget time.Duration) bool {
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(fsPath); err == nil {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}
