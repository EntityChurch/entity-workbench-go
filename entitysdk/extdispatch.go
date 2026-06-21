package entitysdk

import (
	"errors"
	"fmt"

	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"
)

// extDispatch dispatches an EXECUTE for an extension op through the
// AppPeer's Executor. Used by the typed extension clients (Identity,
// Role, Attestation, Quorum) — same pattern, different URIs.
//
// Local vs remote routing is handled by the executor's URI-aware
// path: bare handler URIs ("system/role") dispatch locally; URIs of
// the form "entity://{peer-id}/system/role" route through the
// connection pool. Non-200 statuses are mapped to *entitysdk.Error
// so callers can use the standard IsNotFound / IsForbidden / etc.
// predicates.
//
// Returns the response's result entity (zero entity on no-data
// 2xx responses).
func extDispatch(
	ap *AppPeer,
	handlerURI, op, resourcePath string,
	params entity.Entity,
) (entity.Entity, error) {
	var resource *types.ResourceTarget
	if resourcePath != "" {
		resource = &types.ResourceTarget{Targets: []string{resourcePath}}
	}
	resp, err := ap.executor.ExecuteOnResource(handlerURI, op, params, resource)
	if err != nil {
		// Surface already-typed SDK errors (handler returned a non-2xx
		// response; registry not-found; etc.) so callers can use the
		// IsNotFound / IsForbidden / etc. predicates against the real
		// status.
		var sdkErr *Error
		if errors.As(err, &sdkErr) {
			return entity.Entity{}, sdkErr
		}
		return entity.Entity{}, WrapError(500, "execute_failed",
			fmt.Sprintf("%s:%s", handlerURI, op), err)
	}
	if resp == nil {
		return entity.Entity{}, NewError(500, "no_response",
			fmt.Sprintf("%s:%s: no response", handlerURI, op))
	}
	if resp.Status >= 400 {
		if e := ErrorFromResponse(resp); e != nil {
			return entity.Entity{}, e
		}
		return entity.Entity{}, NewError(resp.Status, "ext_op_failed",
			fmt.Sprintf("%s:%s returned status %d", handlerURI, op, resp.Status))
	}
	if len(resp.Data) == 0 {
		return entity.Entity{}, nil
	}
	return resp.Entity(), nil
}

// extPeerURI returns the handler URI to use for ext ops targeting
// peerID. Local peer-id selects the bare form; remote peer-ids
// select the entity:// URI form so the executor's URI-aware routing
// dispatches through the connection pool.
func extPeerURI(localPeerID, targetPeerID, handlerPath string) string {
	if localPeerID == targetPeerID {
		return handlerPath
	}
	return fmt.Sprintf("entity://%s/%s", targetPeerID, handlerPath)
}

// extDispatchWithIncluded is extDispatch plus a caller-supplied
// hctx.Included map. Used by ops whose handler reads params-
// referenced entities from Included (e.g. continuation:install's
// dispatch_capability). For cross-peer dispatches the included
// entities are packed onto the EXECUTE envelope via the protocol
// layer's Extras field (see CORE-GO-FEEDBACK-CROSS-PEER-INCLUDED-EXTRAS).
//
// Pass nil for included to match extDispatch's behavior (handler
// falls back to local content store for cap resolution).
func extDispatchWithIncluded(
	ap *AppPeer,
	handlerURI, op, resourcePath string,
	params entity.Entity,
	included map[hash.Hash]entity.Entity,
) (entity.Entity, error) {
	var resource *types.ResourceTarget
	if resourcePath != "" {
		resource = &types.ResourceTarget{Targets: []string{resourcePath}}
	}
	resp, err := ap.executor.ExecuteWithIncluded(handlerURI, op, params, resource, included)
	if err != nil {
		var sdkErr *Error
		if errors.As(err, &sdkErr) {
			return entity.Entity{}, sdkErr
		}
		return entity.Entity{}, WrapError(500, "execute_failed",
			fmt.Sprintf("%s:%s", handlerURI, op), err)
	}
	if resp == nil {
		return entity.Entity{}, NewError(500, "no_response",
			fmt.Sprintf("%s:%s: no response", handlerURI, op))
	}
	if resp.Status >= 400 {
		if e := ErrorFromResponse(resp); e != nil {
			return entity.Entity{}, e
		}
		return entity.Entity{}, NewError(resp.Status, "ext_op_failed",
			fmt.Sprintf("%s:%s returned status %d", handlerURI, op, resp.Status))
	}
	if len(resp.Data) == 0 {
		return entity.Entity{}, nil
	}
	return resp.Entity(), nil
}
