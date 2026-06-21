package entitysdk_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"

	"github.com/fxamacker/cbor/v2"
)

// TestTreeFollow_DeepTreeConvergence probes whether the entity-sync
// canonical 2-step chain (`tree:extract → tree:merge`) materializes
// a multi-level subtree on the follower in one round — directly
// answering the G3 question that the
// `revision:fetch → content:ingest → revision:merge` chain raised.
//
// `tree:extract` bundles the FULL subtree closure (every trie node +
// every leaf) per TREE §6.2 (see core/tree/operations.go::handleExtract:415-424).
// `tree:merge` ingests the envelope's `Included` + binds every path
// from the trie. So one round should close any tree size — provided
// the chain wiring is the same shape `cmd/entity-sync/main.go::setupSync`
// uses (proven prior art).
//
// If this passes, G3 isn't a continuation-primitive gap — it's
// "revision-follow picked the wrong op." The architecture team's
// existing tree-level pattern IS the answer; revision:fetch is the
// pagination-friendly variant for when bundling closure is too
// expensive, not the canonical mirror op.
func TestTreeFollow_DeepTreeConvergence(t *testing.T) {
	alice, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = alice.Close() })

	bob, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bob.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, ap := range []*entitysdk.AppPeer{alice, bob} {
		ready := make(chan struct{})
		errCh := make(chan error, 1)
		go func(p *entitysdk.AppPeer) { errCh <- p.ListenReady(ctx, ready) }(ap)
		select {
		case <-ready:
		case err := <-errCh:
			t.Fatalf("listen: %v", err)
		case <-time.After(2 * time.Second):
			t.Fatal("listen timeout")
		}
	}
	if _, err := bob.Connect(ctx, alice.Addr().String()); err != nil {
		t.Fatalf("bob connect: %v", err)
	}
	if _, err := alice.Connect(ctx, bob.Addr().String()); err != nil {
		t.Fatalf("alice connect: %v", err)
	}

	// Tree-level prefix must end with "/" per validatePrefix in
	// core/tree/operations.go.
	const prefix = "deep/"
	aliceID := alice.PeerID()
	bobID := bob.PeerID()

	localCap := bob.OwnerCapability().ContentHash

	// Cross-peer cap scoped to system/tree:extract on alice (vs the
	// revision-scoped one in the prior probe).
	//
	// Resources use the explicit /{remotePeerID}/* form. Bare `*` would
	// canonicalize against the leaf's granter (bob) per V7 v7.73 §PR-8,
	// not the dispatch target (alice). MintCrossPeerChainCapability also
	// rewrites bare `*` to this form, but the canonical shape is named
	// here so the test documents what's required.
	crossPeerGrants := []types.GrantEntry{{
		Handlers:   types.CapabilityScope{Include: []string{"system/tree"}},
		Operations: types.CapabilityScope{Include: []string{"extract"}},
		Resources:  types.CapabilityScope{Include: []string{fmt.Sprintf("/%s/*", aliceID)}},
	}}
	crossPeerCapEnt, err := bob.MintCrossPeerChainCapability(aliceID, crossPeerGrants, nil)
	if err != nil {
		t.Fatalf("mint cross-peer cap: %v", err)
	}
	crossPeerCap := crossPeerCapEnt.ContentHash

	extractPath := "system/inbox/treefollow/" + aliceID + "/deep/extract"
	mergePath := "system/inbox/treefollow/" + aliceID + "/deep/merge"

	// Step 2 (tree:merge). source_envelope=nil sentinel; ResultField
	// plumbs the extract result entity into the merge params. Per
	// entity-sync prior art (cmd/entity-sync/main.go:276-294):
	// extract's envelope-result IS the source_envelope payload
	// directly — no ResultTransform needed because the upstream
	// payload is the envelope entity, not an EXECUTE_RESPONSE shape.
	mergeParams, _ := cbor.Marshal(types.MergeRequestData{
		Strategy:     "source-wins",
		SourcePrefix: prefix,
		TargetPrefix: prefix,
	})
	mergeData := types.ContinuationData{
		Target:      "system/tree",
		Operation:   "merge",
		Resource:    &types.ResourceTarget{Targets: []string{prefix}},
		Params:      cbor.RawMessage(mergeParams),
		ResultField: "source_envelope",
	}

	// Step 1 (tree:extract on alice). Delivers result envelope to bob's
	// merge inbox; no params beyond the prefix in the resource target.
	extractData := types.ContinuationData{
		Target:    fmt.Sprintf("entity://%s/system/tree", aliceID),
		Operation: "extract",
		Resource:  &types.ResourceTarget{Targets: []string{prefix}},
		DeliverTo: &types.DeliverySpec{
			URI:       fmt.Sprintf("entity://%s/%s", bobID, mergePath),
			Operation: "receive",
		},
	}

	entitysdk.SetDefaultDispatchCap(localCap, &mergeData)
	entitysdk.SetDefaultDispatchCap(crossPeerCap, &extractData)

	mergeCont, _ := mergeData.ToEntity()
	if _, err := bob.Continuation().Install(ctx, mergePath, mergeCont); err != nil {
		t.Fatalf("install merge: %v", err)
	}
	extractCont, _ := extractData.ToEntity()
	if _, err := bob.Continuation().Install(ctx, extractPath, extractCont); err != nil {
		t.Fatalf("install extract: %v", err)
	}

	// Subscribe on alice for the head — same trigger as the
	// revision-follow chain. When alice commits, head changes, sub
	// fires, extract runs.
	headPattern := entitysdk.RevisionHeadPath(aliceID, prefix)
	deliverURI := fmt.Sprintf("entity://%s/%s", bobID, extractPath)
	rawSub, err := bob.SubscribeRawAt(aliceID, headPattern, deliverURI, "receive",
		entitysdk.SubscribeOpts{Events: []string{"created", "updated"}})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(func() { _ = rawSub.Close() })

	// Alice writes the same 50-leaf-across-5-subdirs tree the
	// revision-follow probe used.
	const numSubdirs = 5
	const leavesPerSub = 10
	const totalLeaves = numSubdirs * leavesPerSub
	for sub := 0; sub < numSubdirs; sub++ {
		for leaf := 0; leaf < leavesPerSub; leaf++ {
			path := fmt.Sprintf("deep/sub-%d/leaf-%02d", sub, leaf)
			body := fmt.Sprintf("alice's leaf at %s", path)
			if _, err := alice.Put(path, "test/note", body); err != nil {
				t.Fatalf("alice put %s: %v", path, err)
			}
		}
	}
	aliceCommit, err := alice.Revision().Commit(ctx, prefix, "deep")
	if err != nil {
		t.Fatalf("alice commit: %v", err)
	}
	t.Logf("alice committed %d leaves: head=%s", totalLeaves, aliceCommit.Version)

	// Wait for tree-level materialization on bob. The materialized
	// paths sit under the SOURCE namespace per V7 peer-id-keyed
	// addressing — i.e. bob mirrors alice's paths under
	// /{aliceID}/deep/, not under /{bobID}/deep/. That's the
	// follower's "view of alice's data" — same as revision-follow's
	// post-merge state.
	deadline := time.Now().Add(15 * time.Second)
	realMaterialized := 0
	for time.Now().Before(deadline) {
		realMaterialized = 0
		for sub := 0; sub < numSubdirs; sub++ {
			ents, _ := bob.List(fmt.Sprintf("/%s/deep/sub-%d/", aliceID, sub))
			realMaterialized += len(ents)
		}
		if realMaterialized >= totalLeaves {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Logf("bob materialized %d/%d leaves under /%s/deep/sub-*/", realMaterialized, totalLeaves, aliceID)

	if realMaterialized < totalLeaves {
		t.Fatalf("G3 NOT closed by tree:extract+tree:merge: only %d/%d leaves materialized", realMaterialized, totalLeaves)
	}
	t.Logf("G3 RESOLVED by canonical entity-sync pattern: tree:extract closure-bundles, tree:merge ingests + binds — all %d leaves in one chain round", totalLeaves)
}
