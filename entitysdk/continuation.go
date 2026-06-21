package entitysdk

import (
	"context"
	"strings"

	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"
)

// ValidateContinuation runs the same structural lints that the
// continuation handler will run server-side on install. Use it
// client-side to fail fast on malformed continuations without a
// round-trip. The rules mirror ext/continuation/handler.go:
//
//   - Target and Operation must be non-empty
//   - DispatchCapability must be set
//   - If ResultField is set, Params must also be set
//   - DeliverTo and OnError, when present, must have non-empty URI
//
// This does NOT validate the authority chain on the cap, nor that
// the target handler exists or accepts the operation — those are
// runtime checks the handler performs.
func ValidateContinuation(cont types.ContinuationData) error {
	if cont.Target == "" {
		return NewError(400, "invalid_continuation", "target is empty")
	}
	if cont.Operation == "" {
		return NewError(400, "invalid_continuation", "operation is empty")
	}
	if cont.DispatchCapability.IsZero() {
		return NewError(400, "missing_dispatch_capability",
			"continuation requires dispatch_capability")
	}
	if cont.ResultField != "" && len(cont.Params) == 0 {
		return NewError(400, "invalid_continuation",
			"result_field specified without params")
	}
	if cont.DeliverTo != nil && cont.DeliverTo.URI == "" {
		return NewError(400, "invalid_continuation",
			"deliver_to set but URI is empty")
	}
	if cont.OnError != nil && cont.OnError.URI == "" {
		return NewError(400, "invalid_continuation",
			"on_error set but URI is empty")
	}
	return nil
}

// ValidateContinuationJoin runs structural lints on a join
// continuation. Same rules as ValidateContinuation plus: Expected
// must be non-empty (a join with no expected slots is a programming
// error — it would fire immediately on the first delivery).
func ValidateContinuationJoin(cont types.ContinuationJoinData) error {
	if len(cont.Expected) == 0 {
		return NewError(400, "invalid_continuation",
			"join continuation requires at least one expected slot")
	}
	if cont.Target == "" {
		return NewError(400, "invalid_continuation", "target is empty")
	}
	if cont.Operation == "" {
		return NewError(400, "invalid_continuation", "operation is empty")
	}
	if cont.DispatchCapability.IsZero() {
		return NewError(400, "missing_dispatch_capability",
			"continuation requires dispatch_capability")
	}
	if cont.ResultField != "" && len(cont.Params) == 0 {
		return NewError(400, "invalid_continuation",
			"result_field specified without params")
	}
	if cont.DeliverTo != nil && cont.DeliverTo.URI == "" {
		return NewError(400, "invalid_continuation",
			"deliver_to set but URI is empty")
	}
	return nil
}

// InboxPath is a convention helper that builds canonical install
// paths for SDK-installed continuation chains. The convention is
// `system/inbox/{purpose}/{instance}/{step}` — purpose names the
// chain class ("follow", "sync", "ingest"), instance is a unique
// identifier within that class (typically the remote peer-id +
// prefix for follow chains), step is the chain position ("fetch",
// "merge", "extract").
//
// This is a *convention*, not a requirement. Callers may install at
// any path — the helper exists for discoverability (so listing tools
// can enumerate SDK-installed chains by walking the canonical
// prefix). Empty segments are dropped so InboxPath("follow", "", "fetch")
// returns "system/inbox/follow/fetch".
func InboxPath(purpose, instance, step string) string {
	parts := []string{"system/inbox"}
	for _, p := range []string{purpose, instance, step} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, "/")
}

// ContinuationClient wraps system/continuation EXECUTE operations
// behind typed Go methods. Each client targets one peer (local or
// remote, selected via AppPeer.Continuation / AppPeer.ContinuationAt).
//
// Per EXTENSION-CONTINUATION.md the handler exposes four operations:
// install, advance, resume, abandon. All four use V7 §3.2
// path-as-resource — the continuation tree path is carried in
// resource.targets[0] (and *not* in params).
//
// The continuation handler is wired default-on by CreatePeer
// (app.go), so any AppPeer can install / advance / resume / abandon
// against its local handler. Cross-peer continuation operations
// (e.g. installing a continuation on a remote peer) require a
// connection to the remote and use the cross-peer dispatch path.
//
// Caps required to live in hctx.Included (dispatch_capability for
// install, etc.) can be passed via the *Included variants. The
// local-targeted simple form looks for caps in the local content
// store as a fallback, which suffices for caps the SDK has minted
// and stored locally before calling Install.
type ContinuationClient struct {
	ap              *AppPeer
	target          string
	continuationURI string
}

// Continuation returns a ContinuationClient targeting the local peer.
func (a *AppPeer) Continuation() *ContinuationClient {
	return &ContinuationClient{
		ap:              a,
		target:          a.PeerID(),
		continuationURI: "system/continuation",
	}
}

// ContinuationAt returns a ContinuationClient targeting the named
// remote peer. Operations dispatch through the local peer's
// connection pool. Cap entities needed by the remote handler must be
// passed via the *Included variants of each method — the remote
// won't see entities only present in the local store.
func (a *AppPeer) ContinuationAt(peerID string) *ContinuationClient {
	return &ContinuationClient{
		ap:              a,
		target:          peerID,
		continuationURI: extPeerURI(a.PeerID(), peerID, "system/continuation"),
	}
}

// PeerID returns the peer-id this client targets.
func (cc *ContinuationClient) PeerID() string { return cc.target }

// Install creates a continuation entity (forward or join) bound at
// the given install path. The continuation's dispatch_capability is
// resolved either from the local content store (local target) or
// from the caller-supplied included map (cross-peer or when caller
// hasn't pre-staged the cap locally).
//
// Per EXTENSION-CONTINUATION.md §3.2, the install handler performs
// R1 creator-authorization on the embedded dispatch_capability — the
// caller (this peer's identity) must chain to the cap's granter.
//
// Returns the install path that the handler bound the entity at
// (echoes the input path on success).
func (cc *ContinuationClient) Install(ctx context.Context, installPath string, contEntity entity.Entity) (string, error) {
	return cc.InstallWithIncluded(ctx, installPath, contEntity, nil)
}

// InstallWithIncluded is Install with optional additional Included
// entities. Use when targeting a remote ContinuationClient (the
// remote can't see the local content store), or when the caller
// hasn't put the dispatch_capability + signature into the local
// store. Pass nil for the local-store-only path (equivalent to
// Install).
func (cc *ContinuationClient) InstallWithIncluded(ctx context.Context, installPath string, contEntity entity.Entity, included map[hash.Hash]entity.Entity) (string, error) {
	if installPath == "" {
		return "", NewError(400, "invalid_path", "install path is empty")
	}
	switch contEntity.Type {
	case types.TypeContinuation:
		cont, err := types.ContinuationDataFromEntity(contEntity)
		if err != nil {
			return "", WrapError(400, "decode_continuation",
				"decode forward continuation payload", err)
		}
		if err := ValidateContinuation(cont); err != nil {
			return "", err
		}
	case types.TypeContinuationJoin:
		cont, err := types.ContinuationJoinDataFromEntity(contEntity)
		if err != nil {
			return "", WrapError(400, "decode_continuation",
				"decode join continuation payload", err)
		}
		if err := ValidateContinuationJoin(cont); err != nil {
			return "", err
		}
	default:
		return "", NewError(400, "invalid_params",
			"install expects system/continuation or system/continuation/join entity, got "+contEntity.Type)
	}
	resultEnt, err := extDispatchWithIncluded(cc.ap, cc.continuationURI, "install", installPath, contEntity, included)
	if err != nil {
		return "", err
	}
	result, err := types.ContinuationInstallResultDataFromEntity(resultEnt)
	if err != nil {
		return "", WrapError(500, "decode_result", "decode ContinuationInstallResult", err)
	}
	return result.Path, nil
}

// Advance dispatches the continuation at path with a result + status
// (per EXTENSION-CONTINUATION.md §3.3). Any handler can trigger
// advance — the inbox handler does it on delivery, but tests and
// administrative tools can call it directly. result may be a zero
// Entity (no value).
func (cc *ContinuationClient) Advance(ctx context.Context, path string, result entity.Entity, status uint) error {
	if path == "" {
		return NewError(400, "invalid_path", "advance path is empty")
	}
	req := types.ContinuationAdvanceRequestData{}
	if !result.ContentHash.IsZero() || len(result.Data) > 0 {
		req.Result = result.Data
	}
	if status != 0 {
		req.Status = &status
	}
	reqEnt, err := req.ToEntity()
	if err != nil {
		return WrapError(500, "encode_request", "encode AdvanceRequest", err)
	}
	_, err = extDispatch(cc.ap, cc.continuationURI, "advance", path, reqEnt)
	return err
}

// Resume reconstructs an EXECUTE from a suspended continuation at
// path and dispatches it (per EXTENSION-CONTINUATION.md §3.7). The
// resolution argument is the data payload to inject as the resumed
// EXECUTE's params (encoded form; pass nil to resume with empty
// params).
func (cc *ContinuationClient) Resume(ctx context.Context, path string) error {
	if path == "" {
		return NewError(400, "invalid_path", "resume path is empty")
	}
	reqEnt, err := types.ContinuationResumeRequestData{}.ToEntity()
	if err != nil {
		return WrapError(500, "encode_request", "encode ResumeRequest", err)
	}
	_, err = extDispatch(cc.ap, cc.continuationURI, "resume", path, reqEnt)
	return err
}

// Abandon deletes the suspended continuation at path (per
// EXTENSION-CONTINUATION.md §3.8). No-op semantics are
// implementation-defined; the handler returns success if no
// continuation exists at the path.
func (cc *ContinuationClient) Abandon(ctx context.Context, path string) error {
	if path == "" {
		return NewError(400, "invalid_path", "abandon path is empty")
	}
	reqEnt, err := types.ContinuationAbandonRequestData{}.ToEntity()
	if err != nil {
		return WrapError(500, "encode_request", "encode AbandonRequest", err)
	}
	_, err = extDispatch(cc.ap, cc.continuationURI, "abandon", path, reqEnt)
	return err
}

// SetDefaultDispatchCap sets DispatchCapability on each continuation
// whose cap is currently zero. Continuations with an already-set cap
// are left alone (per-step override wins).
//
// Pattern: callers build chains step-by-step as plain
// ContinuationData / ContinuationJoinData structs, then apply a
// single root cap to all steps in one call. Collapses N repetitions
// of `DispatchCapability: capHash` to one helper call — the
// pipeline-cap-default ergonomic flagged by the cross-impl Rust SDK
// proposal without introducing a fluent multi-step builder. Mutates
// in place.
func SetDefaultDispatchCap(capHash hash.Hash, conts ...*types.ContinuationData) {
	for _, c := range conts {
		if c == nil {
			continue
		}
		if c.DispatchCapability.IsZero() {
			c.DispatchCapability = capHash
		}
	}
}

// SetDefaultDispatchCapJoin is SetDefaultDispatchCap for join
// continuations. Same semantics, different entity type.
func SetDefaultDispatchCapJoin(capHash hash.Hash, conts ...*types.ContinuationJoinData) {
	for _, c := range conts {
		if c == nil {
			continue
		}
		if c.DispatchCapability.IsZero() {
			c.DispatchCapability = capHash
		}
	}
}
