package entitysdk_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/peer"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"

	"github.com/fxamacker/cbor/v2"
)

// continuation_merge_mode_test.go — POC for
// PROPOSAL-CONTINUATION-MERGE-ASSEMBLY (the §9 ask).
//
// Three checks per the proposal:
//   1. multi-field op assembles flat ({prefix, base, target} dispatched)
//   2. result_merge + result_field install-rejects (400 invalid_continuation)
//   3. merge_value_not_map marker binds on non-map post-transform value
//
// The POC reuses the standing-chain harness from the revision:fetch-diff
// validation: subscribe alice's head → continuation that uses merge-mode
// → dispatch revision:diff(prefix, base, target). revision:diff is the
// canonical "static scaffold + two dynamic fields" op that target-
// implicit dodged.

// TestContinuationMergeMode_MultiFieldFlatAssembly is check (1): a
// standing chain assembles {prefix, base, target} flat into a dispatched
// revision:diff via select + result_merge.
func TestContinuationMergeMode_MultiFieldFlatAssembly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	alice, bob := setupPairForMergeMode(t, ctx)

	const prefix = "merge-poc/"
	aliceID := alice.PeerID()
	bobID := bob.PeerID()

	// Two commits on alice so previous_hash (commit X) and hash
	// (commit Y) are both non-zero version entry hashes.
	for i := 0; i < 5; i++ {
		path := fmt.Sprintf("%sleaf-%02d", prefix, i)
		if _, err := alice.Put(path, "test/note", "v1"); err != nil {
			t.Fatalf("alice put: %v", err)
		}
	}
	if _, err := alice.Revision().Commit(ctx, prefix, "X"); err != nil {
		t.Fatalf("alice commit X: %v", err)
	}
	if _, err := bob.Revision().Pull(ctx, prefix, aliceID); err != nil {
		t.Fatalf("bob initial pull: %v", err)
	}

	// Bob installs a chain: subscribe alice's head → revision:diff
	// (cross-peer to alice). The diff result lands in bob's local inbox
	// where we capture it via the chain's DeliverTo.
	captureInbox := "system/inbox/mergepoc/" + aliceID + "/diff/capture"

	// We dispatch directly to a no-op terminator: the inbox handler
	// without a continuation at the path stores the message as a
	// mailbox entry. That lets us inspect what alice returned via
	// the chain's downstream delivery.
	// Resources use the explicit /{remotePeerID}/* form. Bare `*` would
	// canonicalize against the leaf's granter (bob) per V7 v7.73 §PR-8,
	// not the dispatch target (alice). MintCrossPeerChainCapability also
	// rewrites bare `*` to this form, but the canonical shape is named
	// here so the test documents what's required.
	crossPeerGrants := []types.GrantEntry{{
		Handlers:   types.CapabilityScope{Include: []string{"system/revision"}},
		Operations: types.CapabilityScope{Include: []string{"diff"}},
		Resources:  types.CapabilityScope{Include: []string{fmt.Sprintf("/%s/*", aliceID)}},
	}}
	crossPeerCapEnt, err := bob.MintCrossPeerChainCapability(aliceID, crossPeerGrants, nil)
	if err != nil {
		t.Fatalf("mint cap: %v", err)
	}
	crossPeerCap := crossPeerCapEnt.ContentHash

	diffPath := "system/inbox/mergepoc/" + aliceID + "/diff"
	diffParams, _ := cbor.Marshal(types.RevisionDiffParamsData{Prefix: prefix})
	diffData := types.ContinuationData{
		Target:    fmt.Sprintf("entity://%s/system/revision", aliceID),
		Operation: "diff",
		Resource:  &types.ResourceTarget{Targets: []string{prefix}},
		Params:    cbor.RawMessage(diffParams), // static scaffold {prefix}
		ResultTransform: &types.ContinuationTransformData{
			Select: map[string]string{
				"base":   "previous_hash",
				"target": "hash",
			},
		},
		ResultMerge: true, // merge {base, target} flat into {prefix}
		DeliverTo: &types.DeliverySpec{
			URI:       fmt.Sprintf("entity://%s/%s", bobID, captureInbox),
			Operation: "receive",
		},
	}
	entitysdk.SetDefaultDispatchCap(crossPeerCap, &diffData)

	diffCont, _ := diffData.ToEntity()
	if _, err := bob.Continuation().Install(ctx, diffPath, diffCont); err != nil {
		t.Fatalf("install diff continuation: %v", err)
	}

	// Subscribe alice's head → bob's diff continuation.
	headPattern := entitysdk.RevisionHeadPath(aliceID, prefix)
	deliverURI := fmt.Sprintf("entity://%s/%s", bobID, diffPath)
	rawSub, err := bob.SubscribeRawAt(aliceID, headPattern, deliverURI, "receive",
		entitysdk.SubscribeOpts{Events: []string{"created", "updated"}})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(func() { _ = rawSub.Close() })

	// Alice's second commit → fires the chain → bob's diff cont
	// dispatches revision:diff(prefix, base, target) on alice → result
	// (DiffData entity) lands in bob's capture inbox as a mailbox msg.
	for i := 0; i < 3; i++ {
		path := fmt.Sprintf("%sleaf-%02d", prefix, i)
		if _, err := alice.Put(path, "test/note", "v2"); err != nil {
			t.Fatalf("alice update %s: %v", path, err)
		}
	}
	commitY, err := alice.Revision().Commit(ctx, prefix, "Y")
	if err != nil {
		t.Fatalf("alice commit Y: %v", err)
	}

	// Wait for the diff result to land in bob's capture inbox. The
	// mailbox path layout: {captureInbox}/messages/{hash}.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		entries := bob.RawLocationIndex().List(captureInbox)
		for _, e := range entries {
			if !strings.HasPrefix(e.Path, "/"+bobID+"/"+captureInbox) {
				continue
			}
			ent, ok := bob.Store().GetByHash(e.Hash)
			if !ok {
				continue
			}
			// Inbox wraps the delivery; the delivery's Result is the
			// CBOR of the result entity (wrapped); decode that to get
			// at the DiffData.
			var diffData []byte
			if ent.Type == types.TypeInboxDelivery {
				del, derr := types.InboxDeliveryDataFromEntity(ent)
				if derr != nil {
					continue
				}
				var resultEnt entity.Entity
				if err := ecf.Decode(del.Result, &resultEnt); err != nil {
					continue
				}
				if resultEnt.Type != types.TypeTreeDiff {
					t.Logf("delivery wraps unexpected type: %s", resultEnt.Type)
					continue
				}
				diffData = resultEnt.Data
			} else if ent.Type == types.TypeTreeDiff {
				diffData = ent.Data
			} else {
				continue
			}
			var diff types.DiffData
			if err := ecf.Decode(diffData, &diff); err != nil {
				continue
			}
			// Y commit changed 3 leaves; expect non-empty Changed.
			if len(diff.Changed) == 0 && len(diff.Added) == 0 {
				continue
			}
			t.Logf("OK: revision:diff dispatched with flat {prefix, base, target}; "+
				"alice returned diff with added=%d changed=%d",
				len(diff.Added), len(diff.Changed))
			_ = commitY
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("did not see captured diff result within deadline")
}

// TestContinuationMergeMode_InstallRejectsBothMergeAndResultField is
// check (2): result_merge: true + result_field set → 400 invalid_continuation.
func TestContinuationMergeMode_InstallRejectsBothMergeAndResultField(t *testing.T) {
	bob, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bob.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	localCap := bob.OwnerCapability().ContentHash
	contData := types.ContinuationData{
		Target:      "system/tree",
		Operation:   "merge",
		Resource:    &types.ResourceTarget{Targets: []string{"x/"}},
		Params:      cbor.RawMessage([]byte{0xa0}), // empty CBOR map
		ResultField: "source_envelope",
		ResultMerge: true,
	}
	entitysdk.SetDefaultDispatchCap(localCap, &contData)
	contEnt, _ := contData.ToEntity()
	_, err = bob.Continuation().Install(ctx, "system/inbox/mergepoc/reject-test", contEnt)
	if err == nil {
		t.Fatal("expected 400 invalid_continuation; install succeeded")
	}
	if !strings.Contains(err.Error(), "invalid_continuation") ||
		!strings.Contains(err.Error(), "result_merge") {
		t.Fatalf("expected 400 invalid_continuation with result_merge in message; got: %v", err)
	}
	t.Logf("OK: install rejected with: %v", err)
}

// TestContinuationMergeMode_MarkerFiresOnNonMapValue is check (3):
// result_merge: true meets a non-map post-transform value → §3.4 marker
// binds with reason=merge_value_not_map; dispatch still proceeds.
//
// Drives the chain via continuation:advance directly (no inbox layer)
// so the test is focused on the assembly step's behavior rather than
// the surrounding delivery plumbing.
func TestContinuationMergeMode_MarkerFiresOnNonMapValue(t *testing.T) {
	bob, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bob.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	localCap := bob.OwnerCapability().ContentHash

	// A continuation with extract (scalar producer) + result_merge:
	// post-transform value is a string, not a map → marker fires.
	// Dispatch is to system/tree:list — read-only, accepts arbitrary
	// params (decode-tolerant: unknown fields ignored), returns 200.
	// Important: the §3.4 lost-error marker path is shared across
	// reasons (`{chain_id}/{step_index}` — latest writer wins). If the
	// dispatch ALSO returns non-2xx, the forward_dispatch_non2xx
	// marker overwrites the merge_value_not_map one. We pick a safe
	// target so only the merge marker survives.
	contPath := "system/inbox/mergepoc/marker-test"
	staticParams := map[string]interface{}{"static_only": "yes"}
	staticRaw, _ := ecf.Encode(staticParams)
	contData := types.ContinuationData{
		Target:    "system/tree",
		Operation: "list",
		Resource:  &types.ResourceTarget{Targets: []string{"app/"}},
		Params:    cbor.RawMessage(staticRaw),
		ResultTransform: &types.ContinuationTransformData{
			Extract: "scalar_field", // produces a STRING — non-map
		},
		ResultMerge: true,
	}
	entitysdk.SetDefaultDispatchCap(localCap, &contData)
	contEnt, _ := contData.ToEntity()
	if _, err := bob.Continuation().Install(ctx, contPath, contEnt); err != nil {
		t.Fatalf("install: %v", err)
	}

	// Drive advance with a result payload whose extract path resolves
	// to a scalar (string), not a map.
	resultMap := map[string]interface{}{"scalar_field": "im-a-string-not-a-map"}
	resultRaw, _ := ecf.Encode(resultMap)
	advReq := types.ContinuationAdvanceRequestData{Result: cbor.RawMessage(resultRaw)}
	advEnt, _ := advReq.ToEntity()
	if _, err := bob.Executor().ExecuteOnResource("system/continuation", "advance", advEnt,
		&types.ResourceTarget{Targets: []string{contPath}}); err != nil {
		t.Fatalf("advance: %v", err)
	}

	// v1.20 per-occurrence path: markers at
	// `{chain_id}/{step_index}/{reason}/{marker_hash_hex}` — the {reason}
	// segment is no longer terminal, so we substring-match instead of
	// suffix-match. Multiple occurrences of the same reason coexist at
	// distinct {marker_hash_hex} terminals; v1.16's per-reason isolation
	// is preserved by the path segment one level up.
	deadline := time.Now().Add(5 * time.Second)
	reasonSegment := "/" + types.ChainErrorLostReasonMergeValueNotMap + "/"
	for time.Now().Before(deadline) {
		entries := bob.RawLocationIndex().List("system/runtime/chain-errors/lost/")
		for _, e := range entries {
			if !strings.Contains(e.Path, reasonSegment) {
				continue
			}
			ent, ok := bob.Store().GetByHash(e.Hash)
			if !ok {
				continue
			}
			var m types.ChainErrorLostData
			if err := ecf.Decode(ent.Data, &m); err != nil {
				continue
			}
			if m.Reason == types.ChainErrorLostReasonMergeValueNotMap {
				t.Logf("OK: merge_value_not_map marker bound at %s", e.Path)
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("merge_value_not_map marker did not bind within deadline")
}

func setupPairForMergeMode(t *testing.T, ctx context.Context) (*entitysdk.AppPeer, *entitysdk.AppPeer) {
	t.Helper()
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
	return alice, bob
}
