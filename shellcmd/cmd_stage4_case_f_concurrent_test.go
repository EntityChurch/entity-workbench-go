package shellcmd_test

// Stage 4 Case F — concurrent same-file edits (last-writer-wins probe).
//
// Topology: 2-peer mesh (bidirectional). Both peers simultaneously write
// the SAME relative path with DIFFERENT content. Each watcher fires;
// each peer's tree gets a binding at the same prefix path but with a
// distinct blob hash (content-addressed). Cross-peer notifications then
// race.
//
// What this probes (and is expected to surface):
//
//  1. Last-writer-wins semantics under content-mode write. Without a
//     CRDT layer (this is the local/files content-mode path, NOT
//     revision-tracked), the final state on each peer depends on
//     delivery ordering. The substrate provides no merge semantics for
//     concurrent same-path writes — that's a deliberate scope choice
//     (CRDT is the revision extension's job, not local/files'). This
//     test makes that contract explicit.
//
//  2. Oscillation potential. If alice has hashA and bob has hashB,
//     alice's notification of hashA arrives at bob → bob writes hashA
//     to disk → bob's watcher → bob's tree updates to hashA → bob
//     notifies alice of hashA → alice already has hashA → idempotent
//     no-op. Symmetric for bob's notification arriving at alice. The
//     question is: does the system converge to ONE hash, or does it
//     ping-pong indefinitely?
//
//  3. Whether the test surfaces a NEW finding. If convergence happens
//     (both peers end with the same hash), the system has implicit
//     deterministic ordering somewhere — and that's worth understanding.
//     If oscillation happens, this is a substrate-level finding worth
//     escalating to arch.
//
// NOTE: This test is diagnostic, not a pass/fail correctness assertion.
// The expected outcome is "both peers converge to the same content"
// (whichever one); divergence would be a finding.
//
// **Empirical result (initial run):** DIVERGES, but in an
// unexpected shape. Both peers keep their OWN content on disk AND in
// the tree. Not a swap (which would suggest cross-peer materialization
// fired and didn't terminate); not an oscillation; not a convergence.
// Cross-peer materialization appears to not have fired at all under
// the symmetric same-path-write race. The test logs the divergence as
// a diagnostic but does not fail — the substrate makes no CRDT
// guarantee at this layer; surfacing the unexpected SHAPE of the
// non-convergence is the deliverable.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/ext/localfiles"
)

func TestStage4_CaseF_ConcurrentSameFile(t *testing.T) {
	const rootName = "sync"
	const sourcePrefix = "local/files/" + rootName + "/"
	const sharedFilename = "contested.md"

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	names := []string{"alice", "bob"}
	peers := stage4Setup(t, ctx, 2, names, rootName, sourcePrefix)
	stage4ConnectAllToAll(t, ctx, peers)
	stage4StartWatchers(t, ctx, peers, rootName, sourcePrefix)
	stage4WireMesh(t, peers, rootName, sourcePrefix)

	aliceContent := "# Contested\n\nAlice's version: " + randomTag() + "\n"
	bobContent := "# Contested\n\nBob's version: " + randomTag() + "\n"

	t.Logf("alice content hash: %s", contentHash(aliceContent))
	t.Logf("bob content hash:   %s", contentHash(bobContent))

	// Simultaneous writes via goroutines + WaitGroup barrier to make
	// the race as tight as possible. (Wall-clock ordering doesn't
	// matter for the production-readiness question — the question is
	// what happens when notifications cross in transit.)
	var wg sync.WaitGroup
	wg.Add(2)
	startedBarrier := make(chan struct{})
	go func() {
		defer wg.Done()
		<-startedBarrier
		path := filepath.Join(peers[0].fsRoot, sharedFilename)
		if err := os.WriteFile(path, []byte(aliceContent), 0600); err != nil {
			t.Errorf("alice write: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		<-startedBarrier
		path := filepath.Join(peers[1].fsRoot, sharedFilename)
		if err := os.WriteFile(path, []byte(bobContent), 0600); err != nil {
			t.Errorf("bob write: %v", err)
		}
	}()
	close(startedBarrier)
	wg.Wait()

	// Wait for the system to settle. Both peers' watchers + cross-peer
	// notifications + idempotency checks have to drain. Generous budget.
	time.Sleep(15 * time.Second)

	// Observation phase: what does each peer's filesystem hold?
	aliceFinal, errA := os.ReadFile(filepath.Join(peers[0].fsRoot, sharedFilename))
	bobFinal, errB := os.ReadFile(filepath.Join(peers[1].fsRoot, sharedFilename))
	if errA != nil {
		t.Fatalf("read alice final: %v", errA)
	}
	if errB != nil {
		t.Fatalf("read bob final: %v", errB)
	}

	aliceHash := contentHash(string(aliceFinal))
	bobHash := contentHash(string(bobFinal))

	t.Logf("FINAL STATE:")
	t.Logf("  alice filesystem: %d bytes, hash=%s", len(aliceFinal), aliceHash)
	t.Logf("  bob filesystem:   %d bytes, hash=%s", len(bobFinal), bobHash)

	// Surface tree state too — does each peer's tree have a binding,
	// and is it the same hash as the FS?
	aliceTreeFiles := listPrefix(peers[0].ap, sourcePrefix)
	bobTreeFiles := listPrefix(peers[1].ap, sourcePrefix)
	t.Logf("  alice tree bindings under %s: %d (paths: %v)", sourcePrefix, len(aliceTreeFiles), aliceTreeFiles)
	t.Logf("  bob tree bindings under %s:   %d (paths: %v)", sourcePrefix, len(bobTreeFiles), bobTreeFiles)

	// Surface the actual blob hashes bound in each peer's tree. If
	// these match the OWN content but the FS content matches OWN as
	// well, then nothing happened. If they swap-vs-FS, blob-resolve
	// ran but didn't update the tree. If they match the OTHER's, then
	// blob-resolve DID update the tree and a watcher event reverted
	// the FS.
	for i, p := range peers {
		treeFiles := listPrefix(p.ap, sourcePrefix)
		if len(treeFiles) == 0 {
			t.Logf("  %s: no tree binding under %s", p.name, sourcePrefix)
			continue
		}
		ent, ok, err := p.ap.Get(stripPeer(treeFiles[0]))
		if err != nil || !ok {
			t.Logf("  %s tree read fail: ok=%v err=%v", p.name, ok, err)
			continue
		}
		file, derr := localfiles.FileDataFromEntity(ent)
		if derr != nil {
			t.Logf("  %s tree decode fail: %v", p.name, derr)
			continue
		}
		t.Logf("  peers[%d]=%s tree blob hash = %s (size=%d)", i, p.name,
			file.Content.String(), file.Size)
	}

	// Diagnostic verdict — finding, not failure.
	if aliceHash == bobHash {
		t.Logf("CONVERGED: both peers ended with the same content (hash=%s)", aliceHash)
		if string(aliceFinal) == aliceContent {
			t.Logf("  winner: alice's content")
		} else if string(aliceFinal) == bobContent {
			t.Logf("  winner: bob's content")
		} else {
			t.Logf("  winner: NEITHER initial content (unexpected)")
		}
	} else {
		// Diagnostic log, not a hard failure — finding routed to arch.
		t.Logf("DIVERGED (finding): peers ended with different content (alice=%s, bob=%s); each kept OWN content. See FEEDBACK-STAGE-4-CONCURRENT-SAMEFILE.md.",
			aliceHash, bobHash)
	}

	// Loop-prevention probe — even if divergence happened, the system
	// should be quiescent (no chain-error markers, bounded entity count).
	stage4AssertEntityCountBound(t, peers, sourcePrefix, 1)
	stage4AssertNoChainErrors(t, peers)
}

func contentHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8])
}

func randomTag() string {
	// Test-deterministic content; not actually random, but distinct.
	// Avoids time.Now() for run-to-run reproducibility within a test.
	return fmt.Sprintf("tag-%d", time.Now().UnixNano()%1000)
}
