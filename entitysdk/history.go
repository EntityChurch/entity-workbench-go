package entitysdk

import (
	"context"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"
)

// HistoryClient wraps system/history EXECUTE operations behind typed
// Go methods. Each HistoryClient targets one peer (local or remote,
// selected at construction time via AppPeer.History / AppPeer.HistoryAt).
//
// Per SDK-EXTENSION-OPERATIONS §5 + EXTENSION-HISTORY. The history
// recorder runs as a sync hook on the targeted peer's emit pipeline
// (default-on via CreatePeer), so transitions are recorded on every
// tree mutation that occurs on that peer.
//
// Path canonicalization: Query and Rollback accept either bare paths
// ("workspace/foo") or absolute peer-qualified paths
// ("/{peerID}/workspace/foo"). Bare paths are canonicalized against
// the targeted peer's local-peer-id at handler dispatch — for cross-
// peer calls that means the remote peer's id, not the caller's.
type HistoryClient struct {
	ap         *AppPeer
	target     string
	historyURI string
}

// History returns a HistoryClient targeting the local peer.
func (a *AppPeer) History() *HistoryClient {
	return &HistoryClient{
		ap:         a,
		target:     a.PeerID(),
		historyURI: "system/history",
	}
}

// HistoryAt returns a HistoryClient targeting the named remote peer.
// Operations dispatch through the local peer's connection pool.
func (a *AppPeer) HistoryAt(peerID string) *HistoryClient {
	return &HistoryClient{
		ap:         a,
		target:     peerID,
		historyURI: extPeerURI(a.PeerID(), peerID, "system/history"),
	}
}

// PeerID returns the peer-id this HistoryClient targets.
func (hc *HistoryClient) PeerID() string { return hc.target }

// Query returns the recorded transition chain for params.Path, most
// recent first. Filters: Limit (default 50), Since (exclusive walk
// stop), Before (timestamp ceiling), Events (event-type allowlist).
//
// The handler wraps the result in a system/envelope with the per-
// transition entities in the included map; the wrapper unwraps the
// envelope and returns the typed result. Included transition
// entities are dropped — they're already inlined in
// HistoryQueryResultData.Transitions.
func (hc *HistoryClient) Query(
	ctx context.Context,
	params types.HistoryQueryParamsData,
) (types.HistoryQueryResultData, error) {
	paramEnt, err := params.ToEntity()
	if err != nil {
		return types.HistoryQueryResultData{}, WrapError(500, "encode_request",
			"encode HistoryQueryParams", err)
	}

	resultEnt, err := extDispatch(hc.ap, hc.historyURI, "query", "", paramEnt)
	if err != nil {
		return types.HistoryQueryResultData{}, err
	}

	if resultEnt.Type != "system/envelope" {
		return types.HistoryQueryResultData{}, NewError(500, "unexpected_result_type",
			"history query: expected system/envelope, got "+resultEnt.Type)
	}
	var env entity.Envelope
	if err := ecf.Decode(resultEnt.Data, &env); err != nil {
		return types.HistoryQueryResultData{}, WrapError(500, "decode_failed",
			"history envelope decode", err)
	}
	result, err := types.HistoryQueryResultDataFromEntity(env.Root)
	if err != nil {
		return types.HistoryQueryResultData{}, WrapError(500, "decode_result",
			"decode HistoryQueryResult", err)
	}
	return result, nil
}

// Rollback rebinds path to targetHash. The handler verifies that
// targetHash appears in path's recorded history (404 not_in_history
// otherwise) and that the entity is still present in the content
// store (404 entity_not_found otherwise) before issuing the bind.
//
// The bind goes through the normal tree write path, which records a
// new "rollback" transition on the chain — rollbacks are themselves
// historical events.
func (hc *HistoryClient) Rollback(
	ctx context.Context,
	path string,
	targetHash hash.Hash,
) (types.HistoryRollbackResultData, error) {
	params := types.HistoryRollbackParamsData{Path: path, TargetHash: targetHash}
	paramEnt, err := params.ToEntity()
	if err != nil {
		return types.HistoryRollbackResultData{}, WrapError(500, "encode_request",
			"encode HistoryRollbackParams", err)
	}
	resultEnt, err := extDispatch(hc.ap, hc.historyURI, "rollback", "", paramEnt)
	if err != nil {
		return types.HistoryRollbackResultData{}, err
	}
	result, err := types.HistoryRollbackResultDataFromEntity(resultEnt)
	if err != nil {
		return types.HistoryRollbackResultData{}, WrapError(500, "decode_result",
			"decode HistoryRollbackResult", err)
	}
	return result, nil
}
