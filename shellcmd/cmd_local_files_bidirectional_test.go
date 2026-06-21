package shellcmd_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/peer"
	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/localfiles"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"

	"github.com/fxamacker/cbor/v2"
)

// dumpRevisionDAG walks the version DAG from head, printing each
// version's parents + root + tree-binding count under prefix. Helper
// for diagnosing convergence failures — surfaces whether two peers
// share a common ancestor and what versions diverged.
func dumpRevisionDAG(t *testing.T, label string, ap *entitysdk.AppPeer, head hash.Hash, prefix string) {
	t.Helper()
	cs := ap.RawContentStore()
	li := ap.RawLocationIndex()

	t.Logf("[%s] HEAD = %s", label, head)

	bindings := li.List(prefix)
	t.Logf("[%s] live tree under %s: %d bindings", label, prefix, len(bindings))
	for _, e := range bindings {
		rel := strings.TrimPrefix(e.Path, "/")
		if idx := strings.Index(rel, "/"); idx >= 0 {
			rel = rel[idx+1:]
		}
		t.Logf("[%s]   %s -> %s", label, rel, e.Hash)
	}

	// Walk ancestors up to depth 20.
	seen := map[hash.Hash]bool{}
	frontier := []hash.Hash{head}
	for depth := 0; depth < 20 && len(frontier) > 0; depth++ {
		next := []hash.Hash{}
		for _, h := range frontier {
			if seen[h] || h.IsZero() {
				continue
			}
			seen[h] = true
			ent, ok := cs.Get(h)
			if !ok {
				t.Logf("[%s] V@%d %s — NOT IN STORE", label, depth, h)
				continue
			}
			ver, err := types.RevisionEntryDataFromEntity(ent)
			if err != nil {
				t.Logf("[%s] V@%d %s — decode err: %v", label, depth, h, err)
				continue
			}
			parentsStr := "none"
			if len(ver.Parents) > 0 {
				ps := make([]string, len(ver.Parents))
				for i, p := range ver.Parents {
					ps[i] = p.String()[:24]
				}
				parentsStr = strings.Join(ps, ", ")
			}
			t.Logf("[%s] V@%d %s root=%s parents=[%s]",
				label, depth, h.String()[:24], ver.Root.String()[:24], parentsStr)
			next = append(next, ver.Parents...)
		}
		frontier = next
	}
}

// bidiPeer bundles a peer + its workbench handler refs so the
// bidirectional tests can stand up two of them without inline
// duplication. The fs dir, source/target prefixes, and root name
// are per-peer (each peer mounts its own dir into the SAME
// target tree prefix; the followers merge each other's revisions).
//
// Follow is wired as a continuation chain `subscribe head →
// revision:pull` (REVISION §4.4.8). Earlier versions of this helper
// used `workbench.RevisionConvergeHandler` because `revision:pull`
// wasn't implemented in core-go; once the op landed
// the chain shape replaces RCH wholesale.
type bidiPeer struct {
	ap        *entitysdk.AppPeer
	ingest    *workbench.NotificationIngestHandler
	fsDir     string
	rootName  string
	sourcePfx string
	id        string
}

// newBidiPeer constructs a workbench-handlers-wired peer with
// listener + open-access + sqlite-in-memory storage (we don't need
// persistence for these tests; in-memory keeps them fast).
func newBidiPeer(t *testing.T, fsDir, rootName string) *bidiPeer {
	t.Helper()
	ingest := workbench.NewNotificationIngestHandler(nil)
	// stderr logger — safe even if watcher goroutines log post-exit
	// (unlike t.Log). Useful for diagnosing concurrent test failures.
	logger := log.New(os.Stderr, "["+rootName+"] ", 0)
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		DebugLog:   logger,
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
		Handlers: []entitysdk.HandlerRegistration{
			{Pattern: workbench.NotificationIngestPattern, Handler: ingest},
			{Pattern: workbench.ChainErrorsPattern, Handler: workbench.NewChainErrorsHandler()},
		},
	})
	if err != nil {
		t.Fatalf("CreatePeer %s: %v", rootName, err)
	}
	return &bidiPeer{
		ap:        ap,
		ingest:    ingest,
		fsDir:     fsDir,
		rootName:  rootName,
		sourcePfx: "local/files/" + rootName + "/",
		id:        ap.PeerID(),
	}
}

// installMountQ2Local installs the Phase E mount (source dir →
// target tree prefix) on this peer.
func (p *bidiPeer) installMountQ2Local(t *testing.T, ctx context.Context, targetPrefix string) {
	t.Helper()
	grants := []types.GrantEntry{{
		Handlers:   types.CapabilityScope{Include: []string{workbench.NotificationIngestPattern}},
		Operations: types.CapabilityScope{Include: []string{"receive"}},
	}}
	if _, err := p.ap.MintChainCapabilityBound(grants,
		"system/capability/grants/chain/local-files/"+p.rootName); err != nil {
		t.Fatalf("mint mount cap (%s): %v", p.rootName, err)
	}
	p.ingest.RegisterMount(p.sourcePfx, targetPrefix)
	lf := p.ap.LocalFilesHandler()
	if err := lf.AddRoot(p.rootName, localfiles.RootConfigData{
		Prefix:         p.sourcePfx,
		FilesystemRoot: p.fsDir,
	}, p.ap.RawContentStore(), p.ap.RawLocationIndex()); err != nil {
		t.Fatalf("AddRoot (%s): %v", p.rootName, err)
	}
	if err := lf.StartWatching(ctx, p.rootName, p.ap.RawContentStore(),
		p.ap.RawLocationIndex(), p.ap.IdentityHash()); err != nil {
		t.Fatalf("StartWatching (%s): %v", p.rootName, err)
	}
	deliverURI := fmt.Sprintf("entity://%s/%s", p.id, workbench.NotificationIngestPattern)
	if _, err := p.ap.SubscribeRawAt(p.id, p.sourcePfx+"*", deliverURI, "receive",
		entitysdk.SubscribeOpts{Events: []string{"created", "updated"}}); err != nil {
		t.Fatalf("subscribe mount (%s): %v", p.rootName, err)
	}
}

// installFollow installs a `subscribe head → revision:pull` follow
// chain on this peer for the given remote peer's targetPrefix.
// Per REVISION §4.4.8: pull folds fetch + iterative fetch-entities
// + local merge into one handler op, so the chain is one step with
// no dynamic field (notification just triggers; params are entirely
// static).
//
// Replaces the prior `workbench.RevisionConvergeHandler.RegisterFollow`
// wiring. RCH was the workbench-side workaround for "pull spec'd at
// §4.4.8 but not implemented in core-go" — once the op landed, the
// chain expresses the same orchestration declaratively.
func (p *bidiPeer) installFollow(t *testing.T, remote *bidiPeer, targetPrefix string) {
	t.Helper()
	localCap := p.ap.OwnerCapability().ContentHash

	pullPath := "system/inbox/follow-pull/" + remote.id + "/" + strings.Trim(targetPrefix, "/")
	pullParamsRaw, err := cbor.Marshal(types.RevisionFetchParamsData{
		Prefix: targetPrefix,
		Remote: remote.id,
	})
	if err != nil {
		t.Fatalf("encode pull params (%s→%s): %v", p.rootName, remote.rootName, err)
	}
	pullData := types.ContinuationData{
		Target:    "system/revision",
		Operation: "pull",
		Resource:  &types.ResourceTarget{Targets: []string{targetPrefix}},
		Params:    cbor.RawMessage(pullParamsRaw),
	}
	entitysdk.SetDefaultDispatchCap(localCap, &pullData)

	pullCont, err := pullData.ToEntity()
	if err != nil {
		t.Fatalf("encode pull continuation (%s→%s): %v", p.rootName, remote.rootName, err)
	}
	if _, err := p.ap.Continuation().Install(context.Background(), pullPath, pullCont); err != nil {
		t.Fatalf("install pull continuation (%s→%s): %v", p.rootName, remote.rootName, err)
	}

	headPath := entitysdk.RevisionHeadPath(remote.id, targetPrefix)
	deliverURI := fmt.Sprintf("entity://%s/%s", p.id, pullPath)
	if _, err := p.ap.SubscribeRawAt(remote.id, headPath, deliverURI, "receive",
		entitysdk.SubscribeOpts{Events: []string{"created", "updated"}}); err != nil {
		t.Fatalf("follow subscribe (%s→%s): %v", p.rootName, remote.rootName, err)
	}
}

// bringUp listens, connects, mounts, follows. Helper used by all
// bidirectional tests.
func bringUpBidiPair(t *testing.T, ctx context.Context, targetPrefix string) (*bidiPeer, *bidiPeer) {
	t.Helper()
	dirA := t.TempDir()
	dirB := t.TempDir()
	a := newBidiPeer(t, dirA, "alice")
	b := newBidiPeer(t, dirB, "bob")
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

	// Both peers: mount their own fs dir, auto-version the prefix,
	// follow the other's prefix.
	autoTrue := true
	for _, p := range []*bidiPeer{a, b} {
		p.installMountQ2Local(t, ctx, targetPrefix)
		if _, err := p.ap.Revision().ConfigPut(ctx, "notes", types.RevisionConfigData{
			Prefix:      targetPrefix,
			AutoVersion: &autoTrue,
		}, nil); err != nil {
			t.Fatalf("%s auto-version config: %v", p.rootName, err)
		}
	}
	a.installFollow(t, b, targetPrefix)
	b.installFollow(t, a, targetPrefix)
	return a, b
}

// waitFor polls predicate every 100ms up to timeout. Returns true
// when satisfied, false on timeout.
func waitFor(timeout time.Duration, predicate func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if predicate() {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return predicate()
}

// TestE2E_Bidirectional_AThenB validates the simplest bidirectional
// sync shape: A writes → B sees it; then B writes → A sees it.
// Sequential, not concurrent. If this fails the multi-peer follow
// setup is broken at the basic level.
func TestE2E_Bidirectional_AThenB(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const targetPrefix = "archives/notes/"
	a, b := bringUpBidiPair(t, ctx, targetPrefix)

	// Phase 1: A writes; both peers should see it.
	contentA := "# From Alice\n\nFirst note.\n"
	if err := os.WriteFile(filepath.Join(a.fsDir, "from-alice.md"), []byte(contentA), 0600); err != nil {
		t.Fatalf("write from-alice.md: %v", err)
	}
	wantA := targetPrefix + "from-alice.md"
	if !waitFor(15*time.Second, func() bool {
		return a.ap.Store().Has(wantA) && b.ap.Store().Has(wantA)
	}) {
		t.Fatalf("from-alice.md did not converge on both peers within 15s\n"+
			"  alice has %v\n  bob has %v", a.ap.Store().Has(wantA), b.ap.Store().Has(wantA))
	}
	t.Logf("phase 1 OK: %s on both peers", wantA)

	// Phase 2: B writes; both peers should see it (and still have A's).
	contentB := "# From Bob\n\nReply note.\n"
	if err := os.WriteFile(filepath.Join(b.fsDir, "from-bob.md"), []byte(contentB), 0600); err != nil {
		t.Fatalf("write from-bob.md: %v", err)
	}
	wantB := targetPrefix + "from-bob.md"
	if !waitFor(15*time.Second, func() bool {
		return a.ap.Store().Has(wantA) && a.ap.Store().Has(wantB) &&
			b.ap.Store().Has(wantA) && b.ap.Store().Has(wantB)
	}) {
		t.Fatalf("from-bob.md did not converge on both peers within 15s\n"+
			"  alice has from-alice=%v from-bob=%v\n  bob has from-alice=%v from-bob=%v",
			a.ap.Store().Has(wantA), a.ap.Store().Has(wantB),
			b.ap.Store().Has(wantA), b.ap.Store().Has(wantB))
	}
	t.Logf("phase 2 OK: both files on both peers")
}

// TestE2E_Bidirectional_ConcurrentDisjoint exercises the case the
// user flagged: A and B both write at the same time, to different
// paths. The expected outcome is that both peers converge to a
// state containing BOTH files. This was the source of past
// non-convergence issues with entity-sync; the workbench prototype
// should handle it cleanly because each peer's auto-version
// produces deterministic content-addressed revisions that the
// other side merges.
func TestE2E_Bidirectional_ConcurrentDisjoint(t *testing.T) {
	// Blocked by Finding 9:
	// `core-go/peer/connection.go:282` increments `session.requestSeq`
	// OUTSIDE the connection mutex. Two concurrent outbound
	// EXECUTEs on the same Connection produce duplicate request IDs;
	// the response-routing layer can't disambiguate; one Pull's
	// Fetch silently drops. End-to-end effect: bidirectional sync
	// fails under concurrent commit pressure. Manual Pull recovery
	// works because by then the connection is quiescent.
	//
	// Workbench-side workaround would require serializing Pulls
	// per-connection at the SDK layer, which is more invasive than
	// the one-line core-go fix (move `requestSeq++` under the
	// existing mutex). Skipping pending the core-go fix.
	// t.Skip — Finding 9 fix landed. Unskipped.

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const targetPrefix = "archives/notes/"
	a, b := bringUpBidiPair(t, ctx, targetPrefix)

	// Brief settle delay for cross-peer subscription registration —
	// installFollow dispatches the subscribe op over the wire async;
	// committing before it lands means the first commit's
	// notification has nowhere to go.
	time.Sleep(500 * time.Millisecond)

	// Concurrent writes from both peers. Goroutines so the fs writes
	// happen as close to simultaneously as the scheduler allows.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = os.WriteFile(filepath.Join(a.fsDir, "concurrent-a.md"),
			[]byte("# A simultaneous\n"), 0600)
	}()
	go func() {
		defer wg.Done()
		_ = os.WriteFile(filepath.Join(b.fsDir, "concurrent-b.md"),
			[]byte("# B simultaneous\n"), 0600)
	}()
	wg.Wait()

	wantA := targetPrefix + "concurrent-a.md"
	wantB := targetPrefix + "concurrent-b.md"
	if !waitFor(20*time.Second, func() bool {
		return a.ap.Store().Has(wantA) && a.ap.Store().Has(wantB) &&
			b.ap.Store().Has(wantA) && b.ap.Store().Has(wantB)
	}) {
		// Diagnostic: walk each peer's revision log to understand
		// what happened. Each peer should have at least one own
		// commit + zero or more merges from the other.
		t.Logf("--- divergence diagnostics ---")
		for _, p := range []*bidiPeer{a, b} {
			s, _ := p.ap.Revision().Status(ctx, targetPrefix)
			t.Logf("%s revision head=%s", p.rootName, s.Head)
			logResult, _ := p.ap.Revision().Log(ctx, types.RevisionLogParamsData{Prefix: targetPrefix})
			for i, v := range logResult.Versions {
				t.Logf("  %s log[%d]: %s", p.rootName, i, v)
			}
			entries := p.ap.Store().List(targetPrefix)
			t.Logf("  %s entries at %s: %d", p.rootName, targetPrefix, len(entries))
			for _, e := range entries {
				t.Logf("    %s", e.Path)
			}
			subs := p.ap.Store().List("system/subscription/")
			t.Logf("  %s subscriptions: %d", p.rootName, len(subs))
		}
		// Diagnostic: can a manual Pull recover?
		t.Logf("--- attempting manual Pull recovery ---")
		if _, err := a.ap.Revision().Pull(ctx, targetPrefix, b.id); err != nil {
			t.Logf("alice manual Pull from bob: %v", err)
		} else {
			t.Logf("alice manual Pull from bob: OK; alice now has b-file=%v",
				a.ap.Store().Has(wantB))
		}
		if _, err := b.ap.Revision().Pull(ctx, targetPrefix, a.id); err != nil {
			t.Logf("bob manual Pull from alice: %v", err)
		} else {
			t.Logf("bob manual Pull from alice: OK; bob now has a-file=%v",
				b.ap.Store().Has(wantA))
		}
		t.Fatalf("concurrent disjoint writes did not converge within 20s\n"+
			"  alice: a=%v b=%v\n  bob:   a=%v b=%v",
			a.ap.Store().Has(wantA), a.ap.Store().Has(wantB),
			b.ap.Store().Has(wantA), b.ap.Store().Has(wantB))
	}
	t.Logf("concurrent disjoint OK: both files on both peers")
}

// TestE2E_Bidirectional_ConcurrentSamePath is the stress case the
// user flagged. Both peers write DIFFERENT content to the SAME
// relative path concurrently. With deterministic merge_order
// (content-addressed CRDT), both peers should converge to the
// same winner — picked by hash ordering, identical on both sides.
//
// Tolerated outcomes:
//   - Both peers have the SAME entity bound at the target path
//     (deterministic winner picked).
//
// NOT tolerated:
//   - Peers disagree on which entity is bound at the path
//     (non-convergence).
//   - Path is unbound on either peer.
//
// If this test reveals divergence, that's a real CRDT-merge bug to
// flag to the architecture team.
func TestE2E_Bidirectional_ConcurrentSamePath(t *testing.T) {
	// t.Skip — Finding 9 fix landed. Unskipped.

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const targetPrefix = "archives/notes/"
	a, b := bringUpBidiPair(t, ctx, targetPrefix)

	// Both write to hello.md concurrently with DIFFERENT content.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = os.WriteFile(filepath.Join(a.fsDir, "hello.md"),
			[]byte("# alice's hello\n"), 0600)
	}()
	go func() {
		defer wg.Done()
		_ = os.WriteFile(filepath.Join(b.fsDir, "hello.md"),
			[]byte("# bob's hello\n"), 0600)
	}()
	wg.Wait()

	wantPath := targetPrefix + "hello.md"

	// Wait for both peers to have *something* at the target path,
	// then check convergence.
	if !waitFor(25*time.Second, func() bool {
		return a.ap.Store().Has(wantPath) && b.ap.Store().Has(wantPath)
	}) {
		t.Fatalf("neither peer bound hello.md within 25s")
	}

	// Give the chain a little more time to settle — after both
	// peers see *some* binding, both follow chains may still be
	// pulling each other's later commits.
	time.Sleep(3 * time.Second)

	aEnt, aOk, _ := a.ap.Get(wantPath)
	bEnt, bOk, _ := b.ap.Get(wantPath)
	if !aOk || !bOk {
		t.Fatalf("post-settle: peer missing binding (a=%v, b=%v)", aOk, bOk)
	}
	if aEnt.ContentHash != bEnt.ContentHash {
		t.Errorf("CONVERGENCE FAILURE: peers disagree at %s\n  alice: %s\n  bob:   %s",
			wantPath, aEnt.ContentHash, bEnt.ContentHash)
	} else {
		t.Logf("converged to deterministic winner at %s: %s", wantPath, aEnt.ContentHash)
	}
}

// TestE2E_Bidirectional_BurstWrites is the "stress test it a
// little" case. Each peer writes 5 files in quick succession; both
// peers should converge to a state containing all 10 files.
// TestE2E_Bidirectional_BurstThenTrigger validates the
// eventually-consistent hypothesis: after a burst leaves peers
// divergent (Finding 10), does a single subsequent write on either
// peer trigger a cascading sync that converges?
//
// The reasoning: if alice and bob have divergent heads but neither
// has anything more to say, both subscriptions are in "no-event"
// steady state — they only fire on `created` / `updated` of the
// head path, and the head isn't changing. A single new commit
// from EITHER peer should resume the propagation chain.
//
// This test is exploratory — not part of the prototype's
// "must-pass" suite. If it passes, the system is eventually-
// consistent under burst load (just not instantaneously). If it
// fails, the merge bug is worse than Finding 10 describes.
func TestE2E_Bidirectional_BurstThenTrigger(t *testing.T) {
	// Surfaces the same incomplete-merge bug as BurstWrites.
	// Stays unskipped so the failure motivates the core-go fix; once
	// pull.go bails out instead of merging on incomplete state, this
	// will converge or surface a different signal.
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	const targetPrefix = "archives/notes/"
	const burst = 5
	a, b := bringUpBidiPair(t, ctx, targetPrefix)
	time.Sleep(500 * time.Millisecond)

	// Phase 1: the burst that triggers Finding 10.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < burst; i++ {
			_ = os.WriteFile(filepath.Join(a.fsDir, fmt.Sprintf("a-%d.md", i)),
				[]byte(fmt.Sprintf("# a %d\n", i)), 0600)
			time.Sleep(10 * time.Millisecond)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < burst; i++ {
			_ = os.WriteFile(filepath.Join(b.fsDir, fmt.Sprintf("b-%d.md", i)),
				[]byte(fmt.Sprintf("# b %d\n", i)), 0600)
			time.Sleep(10 * time.Millisecond)
		}
	}()
	wg.Wait()

	// Let the system settle to whatever divergent state it reaches.
	time.Sleep(20 * time.Second)
	aEntries := len(a.ap.Store().List(targetPrefix))
	bEntries := len(b.ap.Store().List(targetPrefix))
	t.Logf("post-burst settle: alice=%d entries, bob=%d entries", aEntries, bEntries)

	// Phase 2: trigger write from alice. The auto-versioner should
	// commit a new revision (incorporating alice's current state).
	// Bob's subscription on alice's head should fire, bob pulls,
	// the cascade continues until convergence.
	_ = os.WriteFile(filepath.Join(a.fsDir, "trigger.md"),
		[]byte("# trigger\n"), 0600)

	// Build expected paths.
	expectedPaths := []string{targetPrefix + "trigger.md"}
	for i := 0; i < burst; i++ {
		expectedPaths = append(expectedPaths,
			fmt.Sprintf("%sa-%d.md", targetPrefix, i),
			fmt.Sprintf("%sb-%d.md", targetPrefix, i))
	}

	allConverged := func() bool {
		for _, p := range expectedPaths {
			if !a.ap.Store().Has(p) || !b.ap.Store().Has(p) {
				return false
			}
		}
		return true
	}

	if waitFor(60*time.Second, allConverged) {
		t.Logf("CONVERGED after trigger: system is eventually-consistent")
		return
	}

	// Report final state.
	aMissing := 0
	bMissing := 0
	for _, p := range expectedPaths {
		if !a.ap.Store().Has(p) {
			aMissing++
		}
		if !b.ap.Store().Has(p) {
			bMissing++
		}
	}
	t.Logf("DID NOT CONVERGE after trigger: alice missing %d / bob missing %d (of %d)",
		aMissing, bMissing, len(expectedPaths))
	t.Logf("system is NOT eventually-consistent under burst + trigger; Finding 10 is more severe than documented")
	t.Fail()
}

func TestE2E_Bidirectional_BurstWrites(t *testing.T) {
	// Skipped — surfaces Finding 10 in a precise form: peers DO
	// converge to the same revision head (eventual head
	// consistency works), but the converged state can be missing
	// individual local commits that were written to disk and
	// ingested locally. Data loss during the merge wipe-and-
	// replace race.
	//
	// Verified empirically: 5 writes per peer with 10ms gaps
	// converge to the same head on both peers within ~5 seconds,
	// but the head has 7-9 of the 10 written files; the rest were
	// lost during interleaved local commits + remote merges.
	//
	// Architecture-team finding — merge needs to be additive or
	// the merge needs to operate on a stable snapshot that
	// includes in-flight local state. See Finding 10.
	//
	// Follow-up: Go impl merge is verified additive
	// (`ext/revision/merge.go::performMerge:277-285`); the
	// "wipe-and-replace" diagnosis was wrong for Go. F9b async
	// dispatch fix has landed in core-go. Empirical post-fix
	// result with -count=3: 1 PASS in 2.3s, 2 FAIL with HEAD
	// CONVERGENCE FAILED (different terminal heads on each peer).
	// New failure mode: not data loss within a converged head;
	// failure to reach consensus under sustained burst. Simpler
	// tests (AThenB, ConcurrentDisjoint, ConcurrentSamePath) are
	// solid (15/15 across 5 iters of each).
	//
	// Re-skipped because the failure is flaky (~67%) and we don't
	// want it red on master sweeps. Captured as a core-go follow-up.
	// Burst still flaky after F10 fastForward fix — cross-peer merge cascade
	// during sustained burst loses one peer's writes asymmetrically. Local
	// burst (single peer) passes; isolation experiment pending.

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	const targetPrefix = "archives/notes/"
	const burst = 5
	a, b := bringUpBidiPair(t, ctx, targetPrefix)

	// Writes are sequenced WITHIN each peer (small gap between
	// successive files on the same fs root) but the two peers
	// write CONCURRENTLY. The within-peer sequencing avoids
	// fsnotify event coalescing under rapid same-directory writes
	// — when 5 CREATE events arrive within microseconds, fsnotify
	// may merge them into a single batch that the watcher's
	// debouncer flushes as one event, losing individual file
	// triggers. That's fsnotify behavior, not a sync issue.
	//
	// We're testing whether the SYNC layer handles a stream of
	// commits from both peers concurrently. The 10ms gap between
	// writes on each peer is well below the fsnotify debounce
	// window but enough that each file gets its own kernel-level
	// inotify event.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < burst; i++ {
			_ = os.WriteFile(filepath.Join(a.fsDir, fmt.Sprintf("a-%d.md", i)),
				[]byte(fmt.Sprintf("# a %d\n", i)), 0600)
			time.Sleep(10 * time.Millisecond)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < burst; i++ {
			_ = os.WriteFile(filepath.Join(b.fsDir, fmt.Sprintf("b-%d.md", i)),
				[]byte(fmt.Sprintf("# b %d\n", i)), 0600)
			time.Sleep(10 * time.Millisecond)
		}
	}()
	wg.Wait()

	expectedPaths := make([]string, 0, burst*2)
	for i := 0; i < burst; i++ {
		expectedPaths = append(expectedPaths,
			fmt.Sprintf("%sa-%d.md", targetPrefix, i),
			fmt.Sprintf("%sb-%d.md", targetPrefix, i))
	}

	allConverged := func() bool {
		for _, p := range expectedPaths {
			if !a.ap.Store().Has(p) || !b.ap.Store().Has(p) {
				return false
			}
		}
		return true
	}
	// Two distinct things to assert:
	// 1) HEAD CONVERGENCE — both peers settle to the same revision
	//    head hash (eventual consistency at the head level).
	// 2) DATA COMPLETENESS — that head hash represents a state
	//    containing every file actually written to disk.
	//
	// Under burst load, (1) holds but (2) may not — the merge can
	// lose individual local commits to the wipe-and-replace race
	// described in Finding 10. We assert (1) and *report* on (2).
	headConverged := waitFor(120*time.Second, func() bool {
		aH, _ := a.ap.Revision().Status(ctx, targetPrefix)
		bH, _ := b.ap.Revision().Status(ctx, targetPrefix)
		return !aH.Head.IsZero() && aH.Head == bH.Head
	})
	if !headConverged {
		aHead, _ := a.ap.Revision().Status(ctx, targetPrefix)
		bHead, _ := b.ap.Revision().Status(ctx, targetPrefix)
		// Walk each peer's version DAG to characterize the divergence.
		dumpRevisionDAG(t, "alice", a.ap, aHead.Head, targetPrefix)
		dumpRevisionDAG(t, "bob  ", b.ap, bHead.Head, targetPrefix)
		t.Fatalf("HEAD CONVERGENCE FAILED — alice=%s bob=%s after 120s",
			aHead.Head, bHead.Head)
	}

	// Head is converged. Wait for each peer's live tree to actually
	// reflect that head's bindings. Per EXTENSION-REVISION §4.4.4: head
	// publication and binding-apply are intentionally NOT atomic; the
	// head pointer CASes to V_merge while the binding-apply loop is
	// still mid-flight. Asserting on the live tree the instant heads
	// match races the in-flight apply. The race window is bounded by
	// merge's binding-apply runtime (typically <100ms); a short polled
	// wait closes it.
	//
	// Spec acknowledges this race and tells implementations to "filter
	// at the subscriber layer" (§4.4.4 line 1149) but doesn't define a
	// canonical primitive. For now we use the test's existing
	// path-by-path predicate in a polling wait — it's what the test
	// wants to assert anyway.
	if !waitFor(10*time.Second, allConverged) {
		missing := make([]string, 0)
		for _, p := range expectedPaths {
			if !a.ap.Store().Has(p) {
				missing = append(missing, "alice missing "+p)
			}
			if !b.ap.Store().Has(p) {
				missing = append(missing, "bob missing "+p)
			}
		}
		aHead, _ := a.ap.Revision().Status(ctx, targetPrefix)
		bHead, _ := b.ap.Revision().Status(ctx, targetPrefix)
		t.Logf("HEAD CONVERGED to %s but DATA INCOMPLETE", aHead.Head)
		t.Logf("alice has %d / %d expected entries; bob has %d / %d",
			len(a.ap.Store().List(targetPrefix)), len(expectedPaths),
			len(b.ap.Store().List(targetPrefix)), len(expectedPaths))
		t.Logf("missing %d:\n  %s", len(missing), strings.Join(missing, "\n  "))
		dumpRevisionDAG(t, "alice", a.ap, aHead.Head, targetPrefix)
		dumpRevisionDAG(t, "bob  ", b.ap, bHead.Head, targetPrefix)
		t.Fatalf("burst writes lost data during merge — Finding 10")
	}
	t.Logf("burst OK: %d files (%d on each peer) converged on both peers", burst*2, burst)
}
