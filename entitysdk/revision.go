package entitysdk

import (
	"context"
	"fmt"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"
)

// RevisionClient wraps system/revision EXECUTE operations behind
// typed Go methods. Each RevisionClient targets one peer (local or
// remote, selected at construction time via AppPeer.Revision /
// AppPeer.RevisionAt).
//
// Per SDK-EXTENSION-OPERATIONS §4 + EXTENSION-REVISION v2.1. The
// revision extension is wired default-on by CreatePeer (handler +
// auto-versioner + root tracker — see app.go). Per-prefix tracking
// remains opt-in via RevisionConfigData entries at
// system/revision/config/{name}; without a matching config the
// auto-versioner is a no-op and Commit returns 404 prefix_not_tracked.
//
// Three operations return system/envelope-wrapped results: Log,
// Fetch, FetchEntities. The wrapper unwraps the envelope and
// returns the typed result; included entities (referenced via the
// envelope) are dropped — accessors that need them will be added
// when a caller surfaces the requirement.
type RevisionClient struct {
	ap          *AppPeer
	target      string
	revisionURI string
}

// Revision returns a RevisionClient targeting the local peer.
func (a *AppPeer) Revision() *RevisionClient {
	return &RevisionClient{
		ap:          a,
		target:      a.PeerID(),
		revisionURI: "system/revision",
	}
}

// RevisionAt returns a RevisionClient targeting the named remote peer.
// Operations dispatch through the local peer's connection pool.
func (a *AppPeer) RevisionAt(peerID string) *RevisionClient {
	return &RevisionClient{
		ap:          a,
		target:      peerID,
		revisionURI: extPeerURI(a.PeerID(), peerID, "system/revision"),
	}
}

// PeerID returns the peer-id this RevisionClient targets.
func (rc *RevisionClient) PeerID() string { return rc.target }

// --- Per-prefix queries ---

// Status returns the working state at prefix — head version, remotes,
// conflicts, pending changes, and any keep-both paths from the last
// merge. Returns 400 invalid_params if prefix is empty.
func (rc *RevisionClient) Status(ctx context.Context, prefix string) (types.RevisionStatusData, error) {
	params := types.RevisionStatusParamsData{Prefix: prefix}
	paramEnt, err := params.ToEntity()
	if err != nil {
		return types.RevisionStatusData{}, WrapError(500, "encode_request", "encode RevisionStatusParams", err)
	}
	resultEnt, err := extDispatch(rc.ap, rc.revisionURI, "status", "", paramEnt)
	if err != nil {
		return types.RevisionStatusData{}, err
	}
	result, err := types.RevisionStatusDataFromEntity(resultEnt)
	if err != nil {
		return types.RevisionStatusData{}, WrapError(500, "decode_result", "decode RevisionStatus", err)
	}
	return result, nil
}

// Commit creates a new revision version capturing the current state
// of prefix. A tracking config (Configs.Put) is *not* required —
// manual commits work on any prefix; configs only drive the
// auto-versioner. message is optional commit metadata.
func (rc *RevisionClient) Commit(ctx context.Context, prefix string, message string) (types.RevisionCommitResultData, error) {
	params := types.RevisionCommitParamsData{Prefix: prefix}
	if message != "" {
		params.Message = &message
	}
	paramEnt, err := params.ToEntity()
	if err != nil {
		return types.RevisionCommitResultData{}, WrapError(500, "encode_request", "encode RevisionCommitParams", err)
	}
	resultEnt, err := extDispatch(rc.ap, rc.revisionURI, "commit", "", paramEnt)
	if err != nil {
		return types.RevisionCommitResultData{}, err
	}
	result, err := types.RevisionCommitResultDataFromEntity(resultEnt)
	if err != nil {
		return types.RevisionCommitResultData{}, WrapError(500, "decode_result", "decode RevisionCommitResult", err)
	}
	return result, nil
}

// Log walks the revision chain at prefix newest-first. Pass nil for
// limit to use the handler default (typically 50). since, when set,
// stops the walk once that hash is reached (exclusive). Result is
// envelope-wrapped server-side; included entities are dropped.
func (rc *RevisionClient) Log(ctx context.Context, params types.RevisionLogParamsData) (types.RevisionLogResultData, error) {
	paramEnt, err := params.ToEntity()
	if err != nil {
		return types.RevisionLogResultData{}, WrapError(500, "encode_request", "encode RevisionLogParams", err)
	}
	resultEnt, err := extDispatch(rc.ap, rc.revisionURI, "log", "", paramEnt)
	if err != nil {
		return types.RevisionLogResultData{}, err
	}
	if resultEnt.Type != "system/envelope" {
		return types.RevisionLogResultData{}, NewError(500, "unexpected_result_type",
			"revision:log expected system/envelope, got "+resultEnt.Type)
	}
	var env entity.Envelope
	if err := ecf.Decode(resultEnt.Data, &env); err != nil {
		return types.RevisionLogResultData{}, WrapError(500, "decode_failed", "log envelope decode", err)
	}
	result, err := types.RevisionLogResultDataFromEntity(env.Root)
	if err != nil {
		return types.RevisionLogResultData{}, WrapError(500, "decode_result", "decode RevisionLogResult", err)
	}
	return result, nil
}

// FindAncestor returns the lowest common ancestor of two version
// hashes, or zero hash when none exists.
func (rc *RevisionClient) FindAncestor(ctx context.Context, versionA, versionB hash.Hash) (hash.Hash, error) {
	params := types.RevisionAncestorParamsData{VersionA: versionA, VersionB: versionB}
	paramEnt, err := params.ToEntity()
	if err != nil {
		return hash.Hash{}, WrapError(500, "encode_request", "encode RevisionAncestorParams", err)
	}
	resultEnt, err := extDispatch(rc.ap, rc.revisionURI, "find-ancestor", "", paramEnt)
	if err != nil {
		return hash.Hash{}, err
	}
	result, err := types.RevisionAncestorResultDataFromEntity(resultEnt)
	if err != nil {
		return hash.Hash{}, WrapError(500, "decode_result", "decode RevisionAncestorResult", err)
	}
	return result.Ancestor, nil
}

// Diff compares two version hashes at prefix, returning added / removed
// / changed paths plus the unchanged subtree count for telemetry.
func (rc *RevisionClient) Diff(ctx context.Context, prefix string, base, target hash.Hash) (types.DiffData, error) {
	params := types.RevisionDiffParamsData{Prefix: prefix, Base: base, Target: target}
	paramEnt, err := params.ToEntity()
	if err != nil {
		return types.DiffData{}, WrapError(500, "encode_request", "encode RevisionDiffParams", err)
	}
	resultEnt, err := extDispatch(rc.ap, rc.revisionURI, "diff", "", paramEnt)
	if err != nil {
		return types.DiffData{}, err
	}
	var diff types.DiffData
	if err := ecf.Decode(resultEnt.Data, &diff); err != nil {
		return types.DiffData{}, WrapError(500, "decode_result", "decode DiffData", err)
	}
	return diff, nil
}

// --- Merge / conflict resolution ---

// Merge combines remoteVersion into prefix's tracked head per the
// configured strategy ("auto", "ours", "theirs", "keep-both").
// dryRun=true reports the would-be outcome without writing. Result
// status is one of "merged" / "conflict" / "no-op".
func (rc *RevisionClient) Merge(ctx context.Context, params types.RevisionMergeParamsData) (types.RevisionMergeResultData, error) {
	paramEnt, err := params.ToEntity()
	if err != nil {
		return types.RevisionMergeResultData{}, WrapError(500, "encode_request", "encode RevisionMergeParams", err)
	}
	resultEnt, err := extDispatch(rc.ap, rc.revisionURI, "merge", "", paramEnt)
	if err != nil {
		return types.RevisionMergeResultData{}, err
	}
	result, err := types.RevisionMergeResultDataFromEntity(resultEnt)
	if err != nil {
		return types.RevisionMergeResultData{}, WrapError(500, "decode_result", "decode RevisionMergeResult", err)
	}
	return result, nil
}

// Resolve fixes one conflicted path within prefix. resolved=nil
// drops the conflict (delete-on-resolve); a non-nil hash binds the
// conflicted path to the named entity. Returns the count of
// remaining conflicts so callers can detect "merge complete".
func (rc *RevisionClient) Resolve(ctx context.Context, prefix, path string, resolved *hash.Hash) (types.RevisionResolveResultData, error) {
	params := types.RevisionResolveParamsData{Prefix: prefix, Path: path, Resolved: resolved}
	paramEnt, err := params.ToEntity()
	if err != nil {
		return types.RevisionResolveResultData{}, WrapError(500, "encode_request", "encode RevisionResolveParams", err)
	}
	resultEnt, err := extDispatch(rc.ap, rc.revisionURI, "resolve", "", paramEnt)
	if err != nil {
		return types.RevisionResolveResultData{}, err
	}
	result, err := types.RevisionResolveResultDataFromEntity(resultEnt)
	if err != nil {
		return types.RevisionResolveResultData{}, WrapError(500, "decode_result", "decode RevisionResolveResult", err)
	}
	return result, nil
}

// --- Branch (multi-action: list / create / delete / switch) ---

// Branch dispatches a generic branch action — used when the action
// is supplied dynamically (e.g. from CLI args). Most callers prefer
// the action-specific helpers BranchList / BranchCreate / etc.
func (rc *RevisionClient) Branch(ctx context.Context, params types.RevisionBranchParamsData) (types.RevisionBranchResultData, error) {
	paramEnt, err := params.ToEntity()
	if err != nil {
		return types.RevisionBranchResultData{}, WrapError(500, "encode_request", "encode RevisionBranchParams", err)
	}
	resultEnt, err := extDispatch(rc.ap, rc.revisionURI, "branch", "", paramEnt)
	if err != nil {
		return types.RevisionBranchResultData{}, err
	}
	result, err := types.RevisionBranchResultDataFromEntity(resultEnt)
	if err != nil {
		return types.RevisionBranchResultData{}, WrapError(500, "decode_result", "decode RevisionBranchResult", err)
	}
	return result, nil
}

// BranchList returns all branches at prefix plus the active branch.
func (rc *RevisionClient) BranchList(ctx context.Context, prefix string) (types.RevisionBranchResultData, error) {
	return rc.Branch(ctx, types.RevisionBranchParamsData{Prefix: prefix, Action: "list"})
}

// BranchCreate creates a branch at prefix anchored at from (zero hash
// = current head).
func (rc *RevisionClient) BranchCreate(ctx context.Context, prefix, name string, from hash.Hash) (types.RevisionBranchResultData, error) {
	return rc.Branch(ctx, types.RevisionBranchParamsData{
		Prefix: prefix, Action: "create", Name: name, From: from,
	})
}

// BranchDelete drops a branch reference. The underlying versions
// remain in the content store; only the branch pointer is removed.
func (rc *RevisionClient) BranchDelete(ctx context.Context, prefix, name string) (types.RevisionBranchResultData, error) {
	return rc.Branch(ctx, types.RevisionBranchParamsData{
		Prefix: prefix, Action: "delete", Name: name,
	})
}

// BranchSwitch moves the active branch pointer at prefix to name.
// Use Checkout to swap working state to a branch's head; Switch is
// the lightweight pointer move.
func (rc *RevisionClient) BranchSwitch(ctx context.Context, prefix, name string) (types.RevisionBranchResultData, error) {
	return rc.Branch(ctx, types.RevisionBranchParamsData{
		Prefix: prefix, Action: "switch", Name: name,
	})
}

// --- Checkout (move working state) ---

// Checkout swaps the prefix's tracked head to the named branch or a
// specific version hash. Pass branch="" + version=<hash> to check out
// a detached version, or branch=<name> + version=zero to follow the
// branch. Returns any cascade warnings from the underlying tree
// writes.
func (rc *RevisionClient) Checkout(ctx context.Context, params types.RevisionCheckoutParamsData) (types.RevisionCheckoutResultData, error) {
	paramEnt, err := params.ToEntity()
	if err != nil {
		return types.RevisionCheckoutResultData{}, WrapError(500, "encode_request", "encode RevisionCheckoutParams", err)
	}
	resultEnt, err := extDispatch(rc.ap, rc.revisionURI, "checkout", "", paramEnt)
	if err != nil {
		return types.RevisionCheckoutResultData{}, err
	}
	result, err := types.RevisionCheckoutResultDataFromEntity(resultEnt)
	if err != nil {
		return types.RevisionCheckoutResultData{}, WrapError(500, "decode_result", "decode RevisionCheckoutResult", err)
	}
	return result, nil
}

// --- Tag (multi-action: list / create / delete) ---

// Tag dispatches a generic tag action. Callers prefer the
// action-specific helpers below.
func (rc *RevisionClient) Tag(ctx context.Context, params types.RevisionTagParamsData) (types.RevisionTagResultData, error) {
	paramEnt, err := params.ToEntity()
	if err != nil {
		return types.RevisionTagResultData{}, WrapError(500, "encode_request", "encode RevisionTagParams", err)
	}
	resultEnt, err := extDispatch(rc.ap, rc.revisionURI, "tag", "", paramEnt)
	if err != nil {
		return types.RevisionTagResultData{}, err
	}
	result, err := types.RevisionTagResultDataFromEntity(resultEnt)
	if err != nil {
		return types.RevisionTagResultData{}, WrapError(500, "decode_result", "decode RevisionTagResult", err)
	}
	return result, nil
}

// TagList returns all tags at prefix.
func (rc *RevisionClient) TagList(ctx context.Context, prefix string) (types.RevisionTagResultData, error) {
	return rc.Tag(ctx, types.RevisionTagParamsData{Prefix: prefix, Action: "list"})
}

// TagCreate names a version. Tags are immutable references; calling
// create with an existing name returns an error.
func (rc *RevisionClient) TagCreate(ctx context.Context, prefix, name string, version hash.Hash) (types.RevisionTagResultData, error) {
	return rc.Tag(ctx, types.RevisionTagParamsData{
		Prefix: prefix, Action: "create", Name: name, Version: version,
	})
}

// TagDelete drops a tag reference.
func (rc *RevisionClient) TagDelete(ctx context.Context, prefix, name string) (types.RevisionTagResultData, error) {
	return rc.Tag(ctx, types.RevisionTagParamsData{
		Prefix: prefix, Action: "delete", Name: name,
	})
}

// --- Cherry-pick / revert ---

// CherryPick replays a single version onto the current head at
// prefix. parent narrows the operation when version has multiple
// parents (zero hash = unambiguous case). Conflicts in the result
// must be Resolved before another revision op succeeds.
func (rc *RevisionClient) CherryPick(ctx context.Context, prefix string, version, parent hash.Hash) (types.RevisionCherryPickResultData, error) {
	params := types.RevisionCherryPickParamsData{Prefix: prefix, Version: version, Parent: parent}
	paramEnt, err := params.ToEntity()
	if err != nil {
		return types.RevisionCherryPickResultData{}, WrapError(500, "encode_request", "encode RevisionCherryPickParams", err)
	}
	resultEnt, err := extDispatch(rc.ap, rc.revisionURI, "cherry-pick", "", paramEnt)
	if err != nil {
		return types.RevisionCherryPickResultData{}, err
	}
	result, err := types.RevisionCherryPickResultDataFromEntity(resultEnt)
	if err != nil {
		return types.RevisionCherryPickResultData{}, WrapError(500, "decode_result", "decode RevisionCherryPickResult", err)
	}
	return result, nil
}

// Revert applies the inverse of version onto the current head at
// prefix. Same parent-disambiguation semantics as CherryPick.
func (rc *RevisionClient) Revert(ctx context.Context, prefix string, version, parent hash.Hash) (types.RevisionRevertResultData, error) {
	params := types.RevisionRevertParamsData{Prefix: prefix, Version: version, Parent: parent}
	paramEnt, err := params.ToEntity()
	if err != nil {
		return types.RevisionRevertResultData{}, WrapError(500, "encode_request", "encode RevisionRevertParams", err)
	}
	resultEnt, err := extDispatch(rc.ap, rc.revisionURI, "revert", "", paramEnt)
	if err != nil {
		return types.RevisionRevertResultData{}, err
	}
	result, err := types.RevisionRevertResultDataFromEntity(resultEnt)
	if err != nil {
		return types.RevisionRevertResultData{}, WrapError(500, "decode_result", "decode RevisionRevertResult", err)
	}
	return result, nil
}

// --- Cross-peer transfer (fetch / fetch-entities / push) ---

// Fetch retrieves remote prefix versions newer than since (zero =
// from initial). Result is envelope-wrapped server-side; the
// included version-entry + trie-node entities from the envelope are
// **copied into the calling peer's content store** so the local
// peer can resolve the fetched versions in subsequent merge / log /
// status ops. Without this side effect, merge of a freshly-fetched
// remote head would 404.
//
// Use FetchEntities to materialize the snapshot's leaf data
// entities (the trie's content, not just structure).
func (rc *RevisionClient) Fetch(ctx context.Context, params types.RevisionFetchParamsData) (types.RevisionFetchResultData, error) {
	paramEnt, err := params.ToEntity()
	if err != nil {
		return types.RevisionFetchResultData{}, WrapError(500, "encode_request", "encode RevisionFetchParams", err)
	}
	resultEnt, err := extDispatch(rc.ap, rc.revisionURI, "fetch", "", paramEnt)
	if err != nil {
		return types.RevisionFetchResultData{}, err
	}
	if resultEnt.Type != "system/envelope" {
		return types.RevisionFetchResultData{}, NewError(500, "unexpected_result_type",
			"revision:fetch expected system/envelope, got "+resultEnt.Type)
	}
	var env entity.Envelope
	if err := ecf.Decode(resultEnt.Data, &env); err != nil {
		return types.RevisionFetchResultData{}, WrapError(500, "decode_failed", "fetch envelope decode", err)
	}

	// Copy fetched entities (version entries + trie nodes) into the
	// local content store so the local peer can resolve them. Best
	// effort — Put rejects only on encode errors, which would
	// indicate a malformed envelope from the remote.
	for _, ent := range env.Included {
		if _, err := rc.ap.peer.Store().Put(ent); err != nil {
			return types.RevisionFetchResultData{}, WrapError(500, "ingest_failed",
				fmt.Sprintf("write fetched entity %s into local store", ent.Type), err)
		}
	}

	result, err := types.RevisionFetchResultDataFromEntity(env.Root)
	if err != nil {
		return types.RevisionFetchResultData{}, WrapError(500, "decode_result", "decode RevisionFetchResult", err)
	}
	return result, nil
}

// FetchEntities materializes the named hashes from a remote
// snapshot's trie. Returned entities are **copied into the calling
// peer's content store** (matching the Fetch wrapper's ingest
// semantics) so subsequent local lookups resolve. Result reports
// which hashes were found vs missing.
func (rc *RevisionClient) FetchEntities(ctx context.Context, params types.RevisionFetchEntitiesParamsData) (types.RevisionFetchEntitiesResultData, error) {
	paramEnt, err := params.ToEntity()
	if err != nil {
		return types.RevisionFetchEntitiesResultData{}, WrapError(500, "encode_request", "encode RevisionFetchEntitiesParams", err)
	}
	resultEnt, err := extDispatch(rc.ap, rc.revisionURI, "fetch-entities", "", paramEnt)
	if err != nil {
		return types.RevisionFetchEntitiesResultData{}, err
	}
	if resultEnt.Type != "system/envelope" {
		return types.RevisionFetchEntitiesResultData{}, NewError(500, "unexpected_result_type",
			"revision:fetch-entities expected system/envelope, got "+resultEnt.Type)
	}
	var env entity.Envelope
	if err := ecf.Decode(resultEnt.Data, &env); err != nil {
		return types.RevisionFetchEntitiesResultData{}, WrapError(500, "decode_failed", "fetch-entities envelope decode", err)
	}

	// Ingest fetched entities into the local content store.
	for _, ent := range env.Included {
		if _, err := rc.ap.peer.Store().Put(ent); err != nil {
			return types.RevisionFetchEntitiesResultData{}, WrapError(500, "ingest_failed",
				fmt.Sprintf("write fetched entity %s into local store", ent.Type), err)
		}
	}

	result, err := types.RevisionFetchEntitiesResultDataFromEntity(env.Root)
	if err != nil {
		return types.RevisionFetchEntitiesResultData{}, WrapError(500, "decode_result", "decode RevisionFetchEntitiesResult", err)
	}
	return result, nil
}

// FetchDiff is the content-delta sibling of Fetch (REVISION v3.4
// §4.4.19). Bundles the trie-node + leaf-entity closure that
// changed between the caller-supplied base version and the remote
// peer's current head, returning the raw `system/envelope` entity
// directly so callers can hand it to `tree:merge` as a
// `source_envelope` without re-marshalling.
//
// Side effect: ingests `env.Included` into the local content store
// (symmetric with Fetch/FetchEntities). The envelope's Root is a
// `system/tree/snapshot` whose `Root` field is the remote's current
// trie root for the prefix — useful for callers that want to merge
// it directly via `Store().Put` + tree:merge.
//
// `Base = zero` means "diff against the empty trie" — i.e. the full
// current closure, used for bootstrap (see GUIDE-REVISION-AUTO-VERSION
// §4.3).
func (rc *RevisionClient) FetchDiff(ctx context.Context, params types.RevisionFetchDiffParamsData) (entity.Entity, error) {
	paramEnt, err := params.ToEntity()
	if err != nil {
		return entity.Entity{}, WrapError(500, "encode_request", "encode RevisionFetchDiffParams", err)
	}
	resultEnt, err := extDispatch(rc.ap, rc.revisionURI, "fetch-diff", "", paramEnt)
	if err != nil {
		return entity.Entity{}, err
	}
	if resultEnt.Type != "system/envelope" {
		return entity.Entity{}, NewError(500, "unexpected_result_type",
			"revision:fetch-diff expected system/envelope, got "+resultEnt.Type)
	}
	var env entity.Envelope
	if err := ecf.Decode(resultEnt.Data, &env); err != nil {
		return entity.Entity{}, WrapError(500, "decode_failed", "fetch-diff envelope decode", err)
	}
	for _, ent := range env.Included {
		if _, err := rc.ap.peer.Store().Put(ent); err != nil {
			return entity.Entity{}, WrapError(500, "ingest_failed",
				fmt.Sprintf("write fetched entity %s into local store", ent.Type), err)
		}
	}
	return resultEnt, nil
}

// Sync is a one-shot pull: fetch the remote peer's current head for
// prefix, retrieve any missing leaf entities the trie references,
// integrate the result into the local DAG via Merge. Returns the
// merge result.
//
// Implements the "Pull" convenience described in GUIDE-REVISION
// §11.3 (which the protocol doesn't expose as a single op):
//
//	fetch → walk trie → fetch-entities for missing leaves → merge
//
// Per GUIDE-REVISION-AUTO-VERSION §4.1, this is the right primitive
// for CI, manual catch-up, or scheduled pull. For automatic
// convergence on every change, wire a subscription + continuation
// chain on top (Phase C).
//
// Pre-conditions:
//   - The local peer must have an open connection to remoteID
//     (AppPeer.Connect).
//   - Both peers must agree on the prefix.
//
// Returns the merge status: "fast_forward" (remote was ahead, head
// just advanced), "merged" (clean merge + new commit),
// "merged_with_conflicts" (paths conflicted; resolve via Resolve).
// Pull is the SDK-EXTENSION-OPERATIONS §4 "pull orchestration":
// one-shot fetch + iterative fetch-entities + local merge.
// Renamed from the earlier "Sync" name to (a) align with the spec
// term and (b) free "Sync" for the standing reactive composition
// (subscription + continuation chain — see Phase C).
//
// Pull blocks until convergence (or the iterative-fetch loop hits
// the safety ceiling). For background / on-write convergence, use
// the follow chain (revision follow / unfollow shell verbs).
func (rc *RevisionClient) Pull(ctx context.Context, prefix, remoteID string) (types.RevisionMergeResultData, error) {
	// 1. Fetch the remote's current head. Side effect: env.Included
	//    (version entries + trie nodes) lands in our local content
	//    store via the Fetch wrapper's ingest logic.
	remoteRC := rc.ap.RevisionAt(remoteID)
	fetchRes, err := remoteRC.Fetch(ctx, types.RevisionFetchParamsData{Prefix: prefix})
	if err != nil {
		return types.RevisionMergeResultData{}, WrapError(500, "fetch_failed",
			fmt.Sprintf("revision/fetch from %s", remoteID), err)
	}
	if fetchRes.Head.IsZero() {
		return types.RevisionMergeResultData{}, NewError(404, "remote_empty",
			fmt.Sprintf("remote %s has no versions at prefix %s", remoteID, prefix))
	}

	// 2. Walk the trie to find missing entities (subtree nodes +
	//    leaf data), iteratively fetch-entities until everything
	//    resolves. The fetch handler only includes the root trie
	//    node — subtree nodes and leaves are pulled on demand.
	//    Loop is bounded by depth + safety counter to avoid infinite
	//    trips on a malicious / inconsistent remote.
	versionEnt, ok := rc.ap.peer.Store().Get(fetchRes.Head)
	if !ok {
		return types.RevisionMergeResultData{}, NewError(500, "version_missing",
			"version entity missing after fetch (envelope ingest issue)")
	}
	versionData, err := types.RevisionEntryDataFromEntity(versionEnt)
	if err != nil {
		return types.RevisionMergeResultData{}, WrapError(500, "decode_version",
			"decode RevisionEntryData", err)
	}
	const maxRounds = 32 // depth ceiling — deeper trees would force more rounds
	for round := 0; round < maxRounds; round++ {
		missing := collectMissingLeafHashes(rc.ap, versionData.Root)
		if len(missing) == 0 {
			break
		}
		fetched, err := remoteRC.FetchEntities(ctx, types.RevisionFetchEntitiesParamsData{
			Prefix:   prefix,
			Snapshot: versionData.Root,
			Hashes:   missing,
		})
		if err != nil {
			return types.RevisionMergeResultData{}, WrapError(500, "fetch_entities_failed",
				fmt.Sprintf("revision/fetch-entities round %d", round+1), err)
		}
		if len(fetched.Found) == 0 {
			// Avoid infinite loop if remote keeps reporting missing.
			break
		}
	}

	// 3. Merge the fetched head into our local DAG. Default strategy
	//    (auto / deterministic per merge_order) is fine for v1.
	return rc.Merge(ctx, types.RevisionMergeParamsData{
		Prefix:        prefix,
		RemoteVersion: fetchRes.Head,
	})
}

// collectMissingLeafHashes walks the trie at root (using the local
// content store, which holds the trie nodes after Fetch) and
// returns the set of leaf-binding hashes that the local store
// doesn't yet have. Used by Sync to drive a targeted FetchEntities
// call rather than fetching everything blindly.
//
// Walks Entries (subtree children, recurse) and Binding (leaf
// entity, check presence). Trie nodes themselves are already local
// after Fetch's envelope ingest.
func collectMissingLeafHashes(ap *AppPeer, root hash.Hash) []hash.Hash {
	if root.IsZero() {
		return nil
	}
	cs := ap.peer.Store()
	seen := map[hash.Hash]bool{}
	var missing []hash.Hash
	var visit func(h hash.Hash)
	visit = func(h hash.Hash) {
		if h.IsZero() || seen[h] {
			return
		}
		seen[h] = true
		nodeEnt, ok := cs.Get(h)
		if !ok {
			// A trie node we don't have. Add to missing — fetch-entities
			// will retrieve it. Note: trie nodes SHOULD already be
			// local after Fetch; missing here means an incomplete
			// envelope ingest.
			missing = append(missing, h)
			return
		}
		if nodeEnt.Type != types.TypeTreeSnapshotNode {
			// Not a trie node — treat as leaf. Skip child walk.
			return
		}
		var nd types.SnapshotNodeData
		if err := ecf.Decode(nodeEnt.Data, &nd); err != nil {
			return
		}
		// EXTENSION-TREE v4.0: bucket entries carry leaf value_hashes
		// directly; link entries point at sub-nodes.
		for _, entry := range nd.Data {
			if entry.IsBucket() {
				for _, tup := range entry.Bucket {
					if tup.ValueHash.IsZero() {
						continue
					}
					if !cs.Has(tup.ValueHash) {
						missing = append(missing, tup.ValueHash)
					}
				}
			} else {
				visit(*entry.Link)
			}
		}
	}
	visit(root)
	return missing
}

// Push uploads versions to remote at the named prefix. Empty
// versions list pushes the local head and any reachable ancestors.
func (rc *RevisionClient) Push(ctx context.Context, params types.RevisionPushParamsData) (types.RevisionPushResultData, error) {
	paramEnt, err := params.ToEntity()
	if err != nil {
		return types.RevisionPushResultData{}, WrapError(500, "encode_request", "encode RevisionPushParams", err)
	}
	resultEnt, err := extDispatch(rc.ap, rc.revisionURI, "push", "", paramEnt)
	if err != nil {
		return types.RevisionPushResultData{}, err
	}
	result, err := types.RevisionPushResultDataFromEntity(resultEnt)
	if err != nil {
		return types.RevisionPushResultData{}, WrapError(500, "decode_result", "decode RevisionPushResult", err)
	}
	return result, nil
}

// --- Config (multi-action: write / delete) ---

// Config dispatches a config action. Callers prefer ConfigPut /
// ConfigDelete.
func (rc *RevisionClient) Config(ctx context.Context, params types.RevisionConfigParamsData) (types.RevisionConfigResultData, error) {
	paramEnt, err := params.ToEntity()
	if err != nil {
		return types.RevisionConfigResultData{}, WrapError(500, "encode_request", "encode RevisionConfigParams", err)
	}
	resultEnt, err := extDispatch(rc.ap, rc.revisionURI, "config", "", paramEnt)
	if err != nil {
		return types.RevisionConfigResultData{}, err
	}
	var result types.RevisionConfigResultData
	if err := ecf.Decode(resultEnt.Data, &result); err != nil {
		return types.RevisionConfigResultData{}, WrapError(500, "decode_result", "decode RevisionConfigResult", err)
	}
	return result, nil
}

// ConfigPut writes (or replaces) a tracking config entry at name.
// expectedHash, when non-nil, gates the write CAS-style — pass the
// current config_hash to assert "no concurrent change since I last
// looked." Pass nil for the first write or "force overwrite" semantics.
//
// Wire-level action name is "set" per EXTENSION-REVISION; the SDK
// uses "Put" since this isn't a CAS-only operation.
func (rc *RevisionClient) ConfigPut(ctx context.Context, name string, cfg types.RevisionConfigData, expectedHash *hash.Hash) (types.RevisionConfigResultData, error) {
	return rc.Config(ctx, types.RevisionConfigParamsData{
		Name: name, Action: "set", Config: &cfg, ExpectedHash: expectedHash,
	})
}

// ConfigDelete removes a tracking config entry. expectedHash gates
// the delete CAS-style; pass nil for unconditional delete.
func (rc *RevisionClient) ConfigDelete(ctx context.Context, name string, expectedHash *hash.Hash) (types.RevisionConfigResultData, error) {
	return rc.Config(ctx, types.RevisionConfigParamsData{
		Name: name, Action: "delete", ExpectedHash: expectedHash,
	})
}
