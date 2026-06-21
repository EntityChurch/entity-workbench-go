package entitysdk

// S2 — RegisterComputeHandler. Mirrors RegisterHandler shape (returns
// *HandlerHandle for symmetric lifecycle), but takes a compute-
// expression Builder instead of a Go func body. Internally:
//
//   1. Build the expression — writes the IR tree at a stable per-pattern
//      sub-path; root hash captured.
//   2. Hand-build HandlerManifestData with expression_path pointing at
//      that sub-path.
//   3. Dispatch system/handler:register (R0 atomic). NEVER tree:put the
//      manifest — that bypasses R0 grant-entity creation and produces
//      403 fail-closed on subsequent dispatches (SDK-OPERATIONS §11.2A).
//
// The expression's tree path is rooted under
//   {pattern}/_compute_expr
// to avoid colliding with the manifest entity at {pattern} and the
// interface entity at system/handler/{pattern}.

import (
	"context"
	"strconv"

	"go.entitychurch.org/entity-core-go/core/types"
)

// IsHandlerRegistered reports whether a handler manifest is currently
// bound at pattern. Probes the registration's persisted side-effect
// without exposing the internal expression-path layout.
//
// This is the S6b ergonomic helper — promoted to E7.2
// SHOULD-provide in SDK-EXTENSION-OPERATIONS. Callers were probing
// `{pattern}/_compute_expr` directly, coupling the verb-level code
// to RegisterComputeHandler's internal expression-path convention.
// The query stays internal to the SDK, so the layout can change
// without breaking callers.
//
//	if !ap.IsHandlerRegistered("app/compute/files-stats") {
//	    _, _ = ap.RegisterComputeHandler(ctx, spec, expr)
//	}
//
// Detection mechanism: the install dispatches system/handler:register
// (R0 atomic) which binds the handler manifest at the pattern path
// and the interface entity at system/handler/{pattern}. We probe the
// interface entity since it's the public surface; the manifest
// expression sub-path stays a private implementation detail.
func (a *AppPeer) IsHandlerRegistered(pattern string) bool {
	if a == nil || pattern == "" {
		return false
	}
	// The interface entity at system/handler/{pattern} is the
	// public registration-evidence. R0 register binds it together
	// with the manifest — if it exists, registration succeeded.
	_, found, err := a.Get("system/handler/" + pattern)
	if err != nil {
		return false
	}
	return found
}

// RegisterComputeHandler installs a compute-backed handler at spec.Pattern.
// The expression DAG rooted at expr is materialized via expr.Build(), and
// the resulting expression-tree root path becomes the manifest's
// expression_path.
//
// Returns the same *HandlerHandle as RegisterHandler. Closing the handle
// tears down the registration; the expression-tree entities are not
// reaped (content-addressed; safe to leak, and may be referenced by
// other manifests).
//
// The kernel-vs-handler principle (SDK-OPERATIONS §11.2A) means we MUST
// dispatch system/handler:register here, never tree:put the manifest
// directly — the register operation is what creates the grant entity at
// system/capability/grants/{pattern}; without it, dispatches return 403.
func (a *AppPeer) RegisterComputeHandler(ctx context.Context, spec HandlerSpec, expr *Builder) (*HandlerHandle, error) {
	if expr == nil {
		return nil, NewError(400, "invalid_handler_spec", "compute expression is nil")
	}

	pattern := spec.Pattern
	exprPath := pattern + "/_compute_expr"

	// Materialize the IR tree at exprPath; intermediates at
	// exprPath/_expr/{hashPrefix}. The rest of the registration is
	// shared with RegisterComputeHandlerAtExpressionPath.
	if _, err := expr.Build(ctx, exprPath); err != nil {
		return nil, NewError(400, "compute_build_failed", err.Error())
	}
	return a.RegisterComputeHandlerAtExpressionPath(ctx, spec, exprPath)
}

// RegisterComputeHandlerAtExpressionPath is the "register against an
// existing expression" variant. The expression tree at exprPath
// must already be populated (typically by a previous .Build call,
// or by another tool that wrote IR entities). This is the surface
// the shell `compute register <pattern> <expr-path>` verb wraps —
// users authored or imported the expression separately and now want
// to dispatch a handler against it.
//
// The exprPath becomes the manifest's expression_path verbatim;
// nothing is re-built or re-stored. The R0-atomic
// system/handler:register dispatch is identical to the Builder
// variant.
func (a *AppPeer) RegisterComputeHandlerAtExpressionPath(ctx context.Context, spec HandlerSpec, exprPath string) (*HandlerHandle, error) {
	if err := validateHandlerSpec(spec); err != nil {
		return nil, err
	}
	if exprPath == "" {
		return nil, NewError(400, "invalid_handler_spec", "expression path is empty")
	}

	pattern := spec.Pattern

	// Default scope wildcard if caller didn't specify — same convention
	// as RegisterHandler / the existing test harnesses. Callers
	// explicitly narrow when they need to.
	scope := spec.InternalScope
	if scope == nil {
		scope = []types.GrantEntry{{
			Operations: types.CapabilityScope{Include: []string{"*"}},
			Handlers:   types.CapabilityScope{Include: []string{"*"}},
			Resources:  types.CapabilityScope{Include: []string{"*"}},
		}}
	}
	manifest := types.HandlerManifestData{
		Pattern:        pattern,
		Name:           spec.Name,
		Operations:     spec.Operations,
		ExpressionPath: exprPath,
		InternalScope:  scope,
	}
	req := types.RegisterRequestData{
		Manifest:       manifest,
		RequestedScope: scope,
	}
	reqEnt, err := req.ToEntity()
	if err != nil {
		return nil, NewError(500, "register_encode_failed", err.Error())
	}

	resp, err := a.Executor().ExecuteOnResource(
		"system/handler", "register", reqEnt,
		&types.ResourceTarget{Targets: []string{"system/handler/" + pattern}},
	)
	if err != nil {
		return nil, err
	}
	if resp.Status != 200 {
		return nil, NewError(resp.Status, "register_failed",
			"system/handler:register returned status "+strconv.FormatUint(uint64(resp.Status), 10)+" (type="+resp.Type+")")
	}

	return &HandlerHandle{
		pattern:  pattern,
		ap:       a,
		hasGrant: scope != nil,
	}, nil
}
