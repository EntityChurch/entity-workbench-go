package shellcmd_test

// Stage 4 round-3 — WB-27 cross-cutting reproducer.
//
// The arch handoff (PROPOSAL-STAGE-4-TRANSPORT-AND-OBSERVABILITY §6.X)
// asks workbench-go to write the cross-cutting WB-27 reproducer once at
// least one impl lands. Core-go landed v1.19 + v1.20 + WB-27 in commit
// 5792cdc. This test is the cross-peer wire-level verification: a chain
// step (Bounds.ChainID present) dispatched from peer A to peer B where
// B's connection grants don't authorize the target handler MUST produce:
//
//  1. A 403 response from B with ErrorData.code = "capability_denied"
//     (V7 §3.3 canonical code, NOT the deprecated `cap_denied` invented
//     by the original v1.18 proposal — confirmed by Amendment 2 →
//     v1.19 unified vocab).
//
//  2. A receiver-side `rejected` chain-error marker bound on B's tree
//     at `system/runtime/chain-errors/rejected/{chain_id}/{step_index}/
//     capability_denied/{marker_hash_hex}` per v1.20 path-scheme. The
//     terminal {marker_hash} segment is V7 §3.5 hex form (66 lowercase
//     hex chars including format-code prefix).
//
//  3. The response's `ErrorData.RejectedMarker` field populated with
//     the receiver-side marker's content hash — the cross-peer mirror
//     pointer per v1.20 §3.10.4.
//
//  4. The marker body (ChainErrorLostData) carrying the spec-defined
//     fields: Reason=capability_denied, ChainID, StepIndex, AttemptedURI,
//     RequestingPeerID, Timestamp.
//
// Core-go has its own unit-level pin
// (TestWB27_ChainCapDeniedBindsReceiverSideRejectedMarker) using a
// synthetic dispatcher call. The workbench reproducer's contribution is
// the CROSS-PEER variant: real network, real connection grants, real
// peer-to-peer wire round-trip of ErrorData.RejectedMarker.
//
// The sender-side `lost` marker binding (per §3.10.4 mirror-pointer
// SHOULD) lives in the continuation engine's chain-advancement code and
// is core-go's pin territory (TestForwardDispatchHandlerNon2xxIsCompleted
// + family); this reproducer pins the receiver side + wire round-trip
// because that's the consumer-observable cross-peer surface.

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/peer"
	"go.entitychurch.org/entity-core-go/core/protocol"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"

	"github.com/fxamacker/cbor/v2"
)

func TestStage4_WB27_CrossPeerChainCapRejection(t *testing.T) {
	// Bob's restricted connection grants: include the test handler URI
	// but NOT the operation we're about to call. Same shape as the
	// Stage 4 Case H "minimum-grant-set + missing chain handler" gap,
	// but here we want the rejection to actually fire, so we keep
	// "ping" out of the allowed ops.
	bobGrants := []types.GrantEntry{
		{
			Handlers:   types.CapabilityScope{Include: []string{"*"}},
			Resources:  types.CapabilityScope{Include: []string{"*"}},
			Operations: types.CapabilityScope{Include: []string{"get"}}, // "ping" NOT here
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Two peers. Alice has open grants (so bob can subscribe / connect
	// fine); bob has restricted grants. Alice is the chain-step sender,
	// bob is the cap-rejector.
	alice, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("CreatePeer alice: %v", err)
	}
	t.Cleanup(func() { _ = alice.Close() })

	bob, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(bobGrants)},
	})
	if err != nil {
		t.Fatalf("CreatePeer bob: %v", err)
	}
	t.Cleanup(func() { _ = bob.Close() })

	bringUpListener(t, ctx, alice, "alice")
	bringUpListener(t, ctx, bob, "bob")
	if _, err := alice.Connect(ctx, bob.Addr().String()); err != nil {
		t.Fatalf("alice→bob connect: %v", err)
	}

	// Dispatch a synthetic chain step from alice to bob:
	//   target  = entity://{bob}/system/peer/transport (any URI works;
	//             we'll target a handler bob has, with an op he
	//             doesn't authorize).
	//   op      = "ping" (NOT in bob's allowed operations → 403)
	//   bounds  = ChainID set (per v1.20 §3.10.3 — only chain
	//             dispatches trigger the rejected marker)
	const chainID = "test-chain-wb27-cross"
	// Op MUST exist on system/tree (else V7 §6.2 / v7.62 dispatcher
	// 501-wins-over-403 fires first — "the caller's authority is
	// irrelevant"). system/tree publishes {get, put, snapshot, diff,
	// merge, extract}; bob's grants allow only "get", so "put" routes
	// past op-existence into cap-check, which then 403s + binds the
	// rejected-marker (the v1.20 §3.10.4 surface we're pinning).
	const targetOp = "put"

	bobURI := "entity://" + bob.PeerID() + "/system/tree"
	paramsEntity, _ := entity.NewEntity("test/params", cbor.RawMessage{0xf6}) // null
	cascadeDepth := uint64(1)

	resp, err := alice.RawPeer().Dispatcher().RemoteExecute(ctx,
		bobURI,
		targetOp,
		paramsEntity,
		nil, // no resource
		&protocol.AsyncDelivery{
			Bounds: &types.BoundsData{
				ChainID:      chainID,
				CascadeDepth: &cascadeDepth,
			},
		},
	)
	if err != nil {
		t.Fatalf("remote execute (expected 403, got transport error): %v", err)
	}
	if resp == nil {
		t.Fatal("nil response from RemoteExecute")
	}
	if resp.Status != 403 {
		t.Fatalf("expected 403 cap-rejected, got status %d (full response: %+v)", resp.Status, resp)
	}

	// Decode ErrorData from the response result.
	if resp.Result.Type == "" {
		t.Fatal("response.Result.Type empty — expected system/protocol/error")
	}
	var ed types.ErrorData
	if err := ecf.Decode(resp.Result.Data, &ed); err != nil {
		t.Fatalf("decode response result.data as ErrorData: %v", err)
	}

	// Assertion 1: V7 §3.3 canonical 403 code.
	if ed.Code != "capability_denied" {
		t.Errorf("ErrorData.code: got %q, want %q (V7 §3.3 line 736 canonical 403 code; v1.19 unified vocab)",
			ed.Code, "capability_denied")
	}

	// Assertion 2: v1.20 §3.10.4 — mirror-pointer populated for chain
	// dispatches.
	if ed.RejectedMarker.IsZero() {
		t.Fatal("v1.20 §3.10.4: ErrorData.RejectedMarker MUST be populated for chain-dispatch cap-rejection (Bounds.ChainID present); got zero hash")
	}

	// Assertion 3: receiver-side marker bound on bob's tree at the
	// canonical v1.20 path. Walk bob's location index directly via the
	// raw accessors.
	expectedPathPrefix := "/" + bob.PeerID() + "/system/runtime/chain-errors/rejected/" +
		chainID + "/"

	rawLI := bob.RawLocationIndex()
	allBobPaths := []string{}
	for _, e := range bob.RawLocationIndex().List("") {
		allBobPaths = append(allBobPaths, e.Path)
	}

	matchingPaths := []string{}
	for _, p := range allBobPaths {
		if strings.HasPrefix(p, expectedPathPrefix) {
			matchingPaths = append(matchingPaths, p)
		}
	}
	if len(matchingPaths) != 1 {
		t.Fatalf("v1.20 §3.10.3: expected exactly 1 receiver-side rejected marker under %s on bob, got %d (paths: %v)",
			expectedPathPrefix, len(matchingPaths), matchingPaths)
	}

	markerPath := matchingPaths[0]
	segs := strings.Split(markerPath, "/")

	// Path shape:
	// /{bob_id}/system/runtime/chain-errors/rejected/{chain_id}/{step_index}/{reason}/{marker_hash_hex}
	if len(segs) < 9 {
		t.Fatalf("path segments: got %d (%v), want >=9 — v1.20 path has 8 segments after leading slash",
			len(segs), segs)
	}
	reasonSeg := segs[len(segs)-2]
	terminal := segs[len(segs)-1]

	if reasonSeg != "capability_denied" {
		t.Errorf("path reason segment: got %q, want %q (v1.19 unified vocab; result.data.code verbatim)",
			reasonSeg, "capability_denied")
	}

	// Assertion 4: terminal segment is V7 §3.5 hex form.
	if len(terminal) != 66 {
		t.Errorf("terminal {marker_hash} segment length: got %d, want 66 (V7 §3.5 hex form, 64 hex chars + 2 format-code prefix)",
			len(terminal))
	}
	if strings.Contains(terminal, ":") {
		t.Error("terminal segment contains ':' — that's V7 §1.2 UI-only Hash.String() form. v1.20 requires V7 §3.5 hex form on paths")
	}
	expectedHex := hex.EncodeToString(ed.RejectedMarker.Bytes())
	if terminal != expectedHex {
		t.Errorf("terminal hex %q != hex(ErrorData.RejectedMarker.Bytes()) %q — cross-peer mirror-pointer mismatch",
			terminal, expectedHex)
	}

	// Assertion 5: hash bound at the path matches the response's mirror.
	boundHash, ok := rawLI.Get(markerPath)
	if !ok {
		t.Fatalf("location-index lookup at %s returned no binding", markerPath)
	}
	if boundHash != ed.RejectedMarker {
		t.Errorf("bound hash %s != ErrorData.RejectedMarker %s",
			boundHash, ed.RejectedMarker)
	}

	// Assertion 6: marker entity body shape per §3.10.6.
	markerEnt, ok := bob.RawContentStore().Get(ed.RejectedMarker)
	if !ok {
		t.Fatalf("marker entity not present in bob's content store")
	}
	if markerEnt.Type != types.TypeChainErrorLost {
		t.Errorf("marker entity type: got %q, want %q",
			markerEnt.Type, types.TypeChainErrorLost)
	}
	var body types.ChainErrorLostData
	if err := ecf.Decode(markerEnt.Data, &body); err != nil {
		t.Fatalf("decode marker body: %v", err)
	}
	if body.Reason != "capability_denied" {
		t.Errorf("body.reason: got %q, want %q", body.Reason, "capability_denied")
	}
	if body.ChainID != chainID {
		t.Errorf("body.chain_id: got %q, want %q", body.ChainID, chainID)
	}
	if body.AttemptedURI != bobURI {
		t.Errorf("body.attempted_uri: got %q, want %q", body.AttemptedURI, bobURI)
	}
	aliceIDExpected := alice.PeerID()
	if body.RequestingPeerID != aliceIDExpected {
		t.Errorf("body.requesting_peer_id: got %q, want %q", body.RequestingPeerID, aliceIDExpected)
	}
	if body.Timestamp == 0 {
		t.Error("body.timestamp: MUST be set (§3.10.6 timestamp-capture discipline)")
	}

	t.Logf("WB-27 cross-peer reproducer ✓ — receiver-side marker bound at %s; "+
		"hash=%s; body reason=%q; cross-peer mirror-pointer round-trip verified",
		markerPath, ed.RejectedMarker.String(), body.Reason)
}

// TestStage4_WB27_OrdinaryEXECUTEDoesNotBindRejectedMarker pins the
// §3.10.3 scope rule: ordinary (non-chain) EXECUTEs that get 403 do NOT
// trigger the receiver-side rejected marker. The caller sees the 403 in
// the synchronous response — no fire-and-forget observability gap to
// close.
//
// Cross-peer variant of core-go's TestWB27_OrdinaryEXECUTECapDeniedDoesNotBindRejectedMarker.
func TestStage4_WB27_OrdinaryEXECUTEDoesNotBindRejectedMarker(t *testing.T) {
	bobGrants := []types.GrantEntry{
		{
			Handlers:   types.CapabilityScope{Include: []string{"*"}},
			Resources:  types.CapabilityScope{Include: []string{"*"}},
			Operations: types.CapabilityScope{Include: []string{"get"}},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	alice, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("CreatePeer alice: %v", err)
	}
	t.Cleanup(func() { _ = alice.Close() })
	bob, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(bobGrants)},
	})
	if err != nil {
		t.Fatalf("CreatePeer bob: %v", err)
	}
	t.Cleanup(func() { _ = bob.Close() })

	bringUpListener(t, ctx, alice, "alice")
	bringUpListener(t, ctx, bob, "bob")
	if _, err := alice.Connect(ctx, bob.Addr().String()); err != nil {
		t.Fatalf("alice→bob connect: %v", err)
	}

	bobURI := "entity://" + bob.PeerID() + "/system/tree"
	paramsEntity, _ := entity.NewEntity("test/params", cbor.RawMessage{0xf6})

	// Same as the chain-dispatched sibling: use "put" (exists on
	// system/tree, not in bob's grants) to route past V7 §6.2's
	// 501-wins-over-403 op-existence gate into cap-check.
	// NO Bounds.ChainID — this is an ordinary EXECUTE.
	resp, err := alice.RawPeer().Dispatcher().RemoteExecute(ctx,
		bobURI, "put", paramsEntity, nil,
		// no AsyncDelivery → no Bounds
	)
	if err != nil {
		t.Fatalf("remote execute: %v", err)
	}
	if resp == nil || resp.Status != 403 {
		var status uint
		if resp != nil {
			status = resp.Status
		}
		t.Fatalf("expected 403 cap-rejected, got status %d", status)
	}

	var ed types.ErrorData
	if err := ecf.Decode(resp.Result.Data, &ed); err != nil {
		t.Fatalf("decode error data: %v", err)
	}
	if ed.Code != "capability_denied" {
		t.Errorf("ErrorData.code: got %q, want %q", ed.Code, "capability_denied")
	}
	// §3.10.3 scope rule: ordinary EXECUTE → no rejected marker → no mirror.
	if !ed.RejectedMarker.IsZero() {
		t.Errorf("v1.20 §3.10.3: ordinary EXECUTE (no Bounds.ChainID) MUST NOT trigger rejected marker; ErrorData.RejectedMarker should be zero, got %s", ed.RejectedMarker)
	}

	// Verify no marker bound under bob's rejected/ prefix at all.
	for _, e := range bob.RawLocationIndex().List("") {
		if strings.Contains(e.Path, "system/runtime/chain-errors/rejected/") {
			t.Errorf("v1.20 §3.10.3: ordinary EXECUTE produced a rejected marker at %s — scope rule violated", e.Path)
		}
	}

	t.Logf("§3.10.3 scope rule ✓ — ordinary EXECUTE cap-rejection produces no marker, no mirror; caller sees 403 in response only")
}
