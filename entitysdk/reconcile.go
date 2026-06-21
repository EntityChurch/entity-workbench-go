package entitysdk

// State catch-up after silent saturation drops or peer-restart downtime.
// Wraps the documented `revision:fetch-diff + tree:merge` chain from
// GUIDE-CONTINUATIONS-WORKBENCH §5 + REVISION v3.4 §4.4.19 as a single
// SDK call.
//
// Use cases:
//   1. After a saturation burst: subscription dropped some notifications;
//      caller wants to make sure local tree state matches publisher.
//   2. After a peer restart: caller called RestorePriorSubscriptions but
//      still missed writes that happened during downtime.
//   3. Periodic reconciliation: belt-and-suspenders for long-lived
//      collaborative workspaces.

import (
	"context"
	"fmt"

	"github.com/fxamacker/cbor/v2"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"
)

// ReconcileResult summarizes a reconciliation pass.
type ReconcileResult struct {
	// Prefix that was reconciled.
	Prefix string

	// RemotePeerID we reconciled against.
	RemotePeerID string

	// BaseHash supplied to FetchDiff (zero = full closure).
	BaseHash hash.Hash

	// EntitiesIngested counts entities pulled across the wire (trie
	// nodes + leaves combined). A rough measure of "how much state
	// changed since the base."
	EntitiesIngested int
}

// ReconcileSinceLastSeen pulls the delta between the caller-supplied
// base hash and the remote peer's current head for prefix, applying
// the result via tree:merge. Wraps the chain documented in
// GUIDE-CONTINUATIONS-WORKBENCH §5.
//
// Pre-conditions:
//   - Local peer has an open connection to remotePeerID
//     (AppPeer.Connect).
//   - Both peers agree on prefix.
//
// `lastSeen = hash.Hash{}` (zero) reconciles against the full current
// closure — equivalent to a bootstrap pull. Pass the last-seen
// revision hash for an incremental sync.
//
// Closes Stage 5 findings F3 (no implicit catch-up after saturation
// drops) and F7 (missed writes recoverable but only via explicit
// pull) from the consumer-side.
func (a *AppPeer) ReconcileSinceLastSeen(ctx context.Context, remotePeerID, prefix string, lastSeen hash.Hash) (ReconcileResult, error) {
	if remotePeerID == "" {
		return ReconcileResult{}, NewError(400, "invalid_peer", "remotePeerID is empty")
	}
	if prefix == "" {
		return ReconcileResult{}, NewError(400, "invalid_prefix", "prefix is empty")
	}

	res := ReconcileResult{
		Prefix:       prefix,
		RemotePeerID: remotePeerID,
		BaseHash:     lastSeen,
	}

	// 1. revision:fetch-diff against the remote. Side effect:
	//    env.Included is already ingested into the local content
	//    store by the FetchDiff wrapper.
	envEnt, err := a.RevisionAt(remotePeerID).FetchDiff(ctx, types.RevisionFetchDiffParamsData{
		Prefix: prefix,
		Base:   lastSeen,
	})
	if err != nil {
		return res, WrapError(500, "fetch_diff_failed",
			fmt.Sprintf("revision:fetch-diff %s prefix=%s", remotePeerID, prefix), err)
	}

	// Decode the envelope shape just for the metric — duplicates a
	// small amount of work FetchDiff already did. The wrapper doesn't
	// expose the count so we re-decode rather than alter its surface.
	var env entity.Envelope
	if err := ecf.Decode(envEnt.Data, &env); err == nil {
		res.EntitiesIngested = len(env.Included)
	}

	// 2. tree:merge the envelope into local prefix. Same shape as
	//    shellcmd/cmd_revision_follow.go::bootstrapFollow.
	envEntRaw, err := cbor.Marshal(envEnt)
	if err != nil {
		return res, WrapError(500, "encode_envelope", "marshal fetch-diff envelope", err)
	}
	mergeReq := types.MergeRequestData{
		Strategy:       "source-wins",
		SourcePrefix:   prefix,
		TargetPrefix:   prefix,
		SourceEnvelope: cbor.RawMessage(envEntRaw),
	}
	mergeParamEnt, err := mergeReq.ToEntity()
	if err != nil {
		return res, WrapError(500, "encode_merge_params", "encode tree:merge params", err)
	}
	resp, err := a.executor.ExecuteWithParams("system/tree", "merge", mergeParamEnt)
	if err != nil {
		return res, WrapError(500, "tree_merge_failed", "tree:merge failed", err)
	}
	if resp == nil {
		return res, NewError(500, "nil_merge_response", "tree:merge returned nil response")
	}
	if resp.Status >= 400 {
		return res, NewError(resp.Status, "tree_merge_status",
			fmt.Sprintf("tree:merge status=%d", resp.Status))
	}
	return res, nil
}
