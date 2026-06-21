package entitysdk

import (
	"context"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/role"

	"github.com/fxamacker/cbor/v2"
)

// RoleClient wraps system/role EXECUTE operations behind typed Go
// methods. Each RoleClient targets one peer (local or remote, selected
// at construction time via AppPeer.Role / AppPeer.RoleAt).
//
// Per SDK-EXTENSION-OPERATIONS §13 + EXTENSION-ROLE v2.0. Conformant
// in V7-only mode (caller cap is the local peer's standing grant);
// when the identity extension is wired in Cut 2, the typical caller
// cap becomes the local peer→controller cap per
// SDK-IDENTITY-INFRASTRUCTURE §7.
//
// Errors: non-200 statuses are returned as *entitysdk.Error with the
// status code preserved, distinct from transport / decode failures.
// Per SDK-OPERATIONS §12.3.
type RoleClient struct {
	ap      *AppPeer
	target  string // peer-id of the targeted peer (local or remote)
	roleURI string // "system/role" (local) or "entity://{peer-id}/system/role" (remote)
}

// Role returns a RoleClient targeting the local peer. Operations
// dispatch through the local peer's Executor, capability-checked under
// the peer's standing grant.
func (a *AppPeer) Role() *RoleClient {
	return &RoleClient{
		ap:      a,
		target:  a.PeerID(),
		roleURI: "system/role",
	}
}

// RoleAt returns a RoleClient targeting the named remote peer. Operations
// dispatch through the local peer's connection pool (the remote peer must
// be reachable via Connect or RegisterRemote).
func (a *AppPeer) RoleAt(peerID string) *RoleClient {
	return &RoleClient{
		ap:      a,
		target:  peerID,
		roleURI: extPeerURI(a.PeerID(), peerID, "system/role"),
	}
}

// PeerID returns the peer-id this RoleClient targets.
func (rc *RoleClient) PeerID() string { return rc.target }

// Define writes or mutates a role definition at
// system/role/{context}/{roleName}. Triggers a re-derive cascade if
// the role already exists (§5.5 IA11). metadata is opaque CBOR; pass
// nil for a role with no metadata.
func (rc *RoleClient) Define(
	ctx context.Context,
	contextStr, roleName string,
	grants []types.GrantEntry,
	metadata cbor.RawMessage,
) (types.RoleDefineResultData, error) {
	req := types.RoleDefineRequestData{Grants: grants, Metadata: metadata}
	ent, err := req.ToEntity()
	if err != nil {
		return types.RoleDefineResultData{}, WrapError(500, "encode_request", "encode RoleDefineRequest", err)
	}
	path := role.RoleDefinitionPath(contextStr, roleName)
	resultEnt, err := rc.execute(ctx, "define", path, ent)
	if err != nil {
		return types.RoleDefineResultData{}, err
	}
	result, err := types.RoleDefineResultDataFromEntity(resultEnt)
	if err != nil {
		return types.RoleDefineResultData{}, WrapError(500, "decode_result", "decode RoleDefineResult", err)
	}
	return result, nil
}

// Assign binds peerHash to roleName within contextStr and issues a
// role-derived capability token (§4.3 + §5.1). The minted cap inherits
// expiration per §5.3 v2.0 MIN_DEFINED(parent, role.ttl, caller_cap).
func (rc *RoleClient) Assign(
	ctx context.Context,
	contextStr string,
	peerHash hash.Hash,
	roleName string,
) (types.RoleAssignResultData, error) {
	req := types.RoleAssignRequestData{Role: roleName}
	ent, err := req.ToEntity()
	if err != nil {
		return types.RoleAssignResultData{}, WrapError(500, "encode_request", "encode RoleAssignRequest", err)
	}
	path := role.AssignmentPath(contextStr, peerHash, roleName)
	resultEnt, err := rc.execute(ctx, "assign", path, ent)
	if err != nil {
		return types.RoleAssignResultData{}, err
	}
	result, err := types.RoleAssignResultDataFromEntity(resultEnt)
	if err != nil {
		return types.RoleAssignResultData{}, WrapError(500, "decode_result", "decode RoleAssignResult", err)
	}
	return result, nil
}

// Unassign removes the assignment for (peerHash, roleName) in
// contextStr. If roleName == "", uses the all-roles form: drops the
// trailing role segment per §4.4 to remove every role for the peer in
// the context.
func (rc *RoleClient) Unassign(
	ctx context.Context,
	contextStr string,
	peerHash hash.Hash,
	roleName string,
) (types.RoleUnassignResultData, error) {
	var path string
	if roleName != "" {
		path = role.AssignmentPath(contextStr, peerHash, roleName)
	} else {
		path = "system/role/" + contextStr + "/assignment/" + role.HashHex(peerHash)
	}
	resultEnt, err := rc.execute(ctx, "unassign", path, emptyParamsEntity())
	if err != nil {
		return types.RoleUnassignResultData{}, err
	}
	result, err := types.RoleUnassignResultDataFromEntity(resultEnt)
	if err != nil {
		return types.RoleUnassignResultData{}, WrapError(500, "decode_result", "decode RoleUnassignResult", err)
	}
	return result, nil
}

// Exclude writes an exclusion entity for peerHash in contextStr and
// triggers the layer-1 sweep (§6.1) — fleet-wide revocation of the
// peer's role-derived caps in this context.
func (rc *RoleClient) Exclude(
	ctx context.Context,
	contextStr string,
	peerHash hash.Hash,
) (types.RoleExcludeResultData, error) {
	path := role.ExclusionPath(contextStr, peerHash)
	resultEnt, err := rc.execute(ctx, "exclude", path, emptyParamsEntity())
	if err != nil {
		return types.RoleExcludeResultData{}, err
	}
	result, err := types.RoleExcludeResultDataFromEntity(resultEnt)
	if err != nil {
		return types.RoleExcludeResultData{}, WrapError(500, "decode_result", "decode RoleExcludeResult", err)
	}
	return result, nil
}

// Unexclude removes the exclusion entity. Per §6.4, removing an
// exclusion does NOT auto-restore role-derived caps — re-assignment
// is required.
func (rc *RoleClient) Unexclude(
	ctx context.Context,
	contextStr string,
	peerHash hash.Hash,
) (types.RoleUnexcludeResultData, error) {
	path := role.ExclusionPath(contextStr, peerHash)
	resultEnt, err := rc.execute(ctx, "unexclude", path, emptyParamsEntity())
	if err != nil {
		return types.RoleUnexcludeResultData{}, err
	}
	result, err := types.RoleUnexcludeResultDataFromEntity(resultEnt)
	if err != nil {
		return types.RoleUnexcludeResultData{}, WrapError(500, "decode_result", "decode RoleUnexcludeResult", err)
	}
	return result, nil
}

// ReDerive walks every assignment of roleName in contextStr and
// re-issues role-derived caps (§5.5 IA9). Per §5.5 SI-15, assignees
// that fail RL2 mid-cascade appear in the result's SkippedGrantees
// rather than aborting the cascade.
func (rc *RoleClient) ReDerive(
	ctx context.Context,
	contextStr, roleName string,
) (types.RoleReDeriveResultData, error) {
	req := types.RoleReDeriveRequestData{Role: roleName}
	ent, err := req.ToEntity()
	if err != nil {
		return types.RoleReDeriveResultData{}, WrapError(500, "encode_request", "encode RoleReDeriveRequest", err)
	}
	path := role.RoleDefinitionPath(contextStr, roleName)
	resultEnt, err := rc.execute(ctx, "re-derive", path, ent)
	if err != nil {
		return types.RoleReDeriveResultData{}, err
	}
	result, err := types.RoleReDeriveResultDataFromEntity(resultEnt)
	if err != nil {
		return types.RoleReDeriveResultData{}, WrapError(500, "decode_result", "decode RoleReDeriveResult", err)
	}
	return result, nil
}

// Delegate is the IA22 member-to-member delegation op (§5.6). The
// caller (delegator) delegates a role they hold (or a literal subset
// of its grants via scope) to delegateHash. expiresAt is optional —
// pass nil for no explicit bound.
//
// Note: as of role v2.0 the Go handler returns 501 not_implemented.
// The signature is stable; callers detect 501 via IsNotSupported and
// the call site does not change once the impl lands.
func (rc *RoleClient) Delegate(
	ctx context.Context,
	contextStr, roleName string,
	delegator, delegate hash.Hash,
	scope []types.GrantEntry,
	expiresAt *uint64,
) (types.RoleDelegateResultData, error) {
	req := types.RoleDelegateRequestData{
		Delegate:  delegate,
		Context:   contextStr,
		Role:      roleName,
		Scope:     scope,
		ExpiresAt: expiresAt,
	}
	ent, err := req.ToEntity()
	if err != nil {
		return types.RoleDelegateResultData{}, WrapError(500, "encode_request", "encode RoleDelegateRequest", err)
	}
	// Per §4.2 the resource target is the delegator's assignment path;
	// the handler synthesizes the role-derived storage path internally.
	path := role.AssignmentPath(contextStr, delegator, roleName)
	resultEnt, err := rc.execute(ctx, "delegate", path, ent)
	if err != nil {
		return types.RoleDelegateResultData{}, err
	}
	result, err := types.RoleDelegateResultDataFromEntity(resultEnt)
	if err != nil {
		return types.RoleDelegateResultData{}, WrapError(500, "decode_result", "decode RoleDelegateResult", err)
	}
	return result, nil
}

// execute dispatches the role op via the shared extDispatch helper.
// Kept as a thin shim so call sites read role.execute(ctx, "define",
// path, ent) rather than threading the URI through every line.
func (rc *RoleClient) execute(
	_ context.Context,
	op, path string,
	params entity.Entity,
) (entity.Entity, error) {
	return extDispatch(rc.ap, rc.roleURI, op, path, params)
}

// emptyParamsEntity returns a primitive/any entity carrying an empty
// CBOR map. Used for ops whose request body is "no parameters" —
// unassign, exclude, unexclude all carry the resource path in the
// EXECUTE.resource field and have no body fields per §4.
func emptyParamsEntity() entity.Entity {
	raw, _ := ecf.Encode(map[string]interface{}{})
	ent, _ := entity.NewEntity("primitive/any", raw)
	return ent
}
