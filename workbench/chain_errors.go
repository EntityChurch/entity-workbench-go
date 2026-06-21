package workbench

import (
	"context"
	"fmt"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/handler"

	"github.com/fxamacker/cbor/v2"
)

// ChainErrorsPattern is the URI prefix under which the workbench's
// chain-errors handler registers. Per
// `sdk-domain/guides/GUIDE-PEER-CONCERNS-AND-NAMESPACES.md §4.1`,
// `system/runtime/` is the namespace for "runtime-instantiated
// machinery that doesn't fit any single extension's namespace."
// Chain errors are exactly that — per-call, ephemeral, system-
// privileged, unfitted-elsewhere observations of failed chain
// steps.
//
// `on_error` deliveries from `system/continuation:advance` route
// here when the chain author sets
// `on_error.uri = entity://{peer}/system/runtime/chain-errors/...`.
// The handler binds the delivery payload at a unique sub-path so
// operators / tooling can list errors by chain.
const ChainErrorsPattern = "system/runtime/chain-errors"

// ChainErrorsHandler is the workbench's on-error sink for
// continuation chains. The behavior is intentionally minimal:
// take the delivery entity in params, bind it at
// `{resource-target}/{request-id}`, return 200.
//
// This is the simplest shape that gives chain errors an observable
// home in the tree. Active error handling (auto-retry, escalation,
// metric emission) is left to higher-level tooling that watches
// the chain-errors subtree via subscription.
//
// Why not just route on_error to a `system/inbox/...` path and use
// the stock inbox handler? Two reasons. (1) The namespace guide is
// explicit that runtime-instantiated machinery lives under
// `system/runtime/`, not in the inbox namespace. (2) The stock
// inbox handler has continuation-advance logic baked in — it
// inspects whether a continuation is bound at the inbox path and
// delegates `advance` if so. That coupling is appropriate for
// inbox semantics but inappropriate for an error sink: an error
// shouldn't accidentally drive another chain step.
type ChainErrorsHandler struct{}

// NewChainErrorsHandler returns the singleton chain-errors handler.
// Stateless; pure dispatch.
func NewChainErrorsHandler() *ChainErrorsHandler {
	return &ChainErrorsHandler{}
}

func (h *ChainErrorsHandler) Name() string { return "workbench-chain-errors" }

// Handle accepts the `receive` operation. The handler is bound at
// pattern `ChainErrorsPattern`; longer paths under that prefix
// (e.g. `system/runtime/chain-errors/local-files/notes/transform`)
// match via longest-prefix and arrive here with the resource path
// in the resource-target list.
func (h *ChainErrorsHandler) Handle(_ context.Context, req *handler.Request) (*handler.Response, error) {
	if req.Operation != "receive" {
		return handler.NewErrorResponse(400, "unknown_operation",
			fmt.Sprintf("chain-errors handler does not support operation %q", req.Operation))
	}
	hctx := req.Context
	if hctx == nil || hctx.Store == nil || hctx.LocationIndex == nil {
		return handler.NewErrorResponse(500, "internal_error",
			"chain-errors handler requires store + location index in context")
	}

	bindPath := hctx.ExtractResourcePath()
	if bindPath == "" {
		return handler.NewErrorResponse(400, "missing_resource",
			"chain-errors receive requires a resource target path")
	}

	// Persist the delivery payload. The cap check is the caller's
	// responsibility — the chain's dispatch_capability must include
	// a grant for system/runtime/chain-errors:receive on the target
	// resource. We don't double-check here because the dispatch
	// chain already validated the cap before invoking us.
	storedHash, err := hctx.Store.Put(req.Params)
	if err != nil {
		return handler.NewErrorResponse(500, "store_failed",
			"persist error delivery: "+err.Error())
	}

	// Bind at {target}/{request-id} so multiple errors at the same
	// chain step don't clobber each other. RequestID is unique per
	// dispatch.
	key := hctx.RequestID
	if key == "" {
		key = storedHash.String()
	}
	storagePath := bindPath + "/" + key
	hctx.TreeSet(storagePath, storedHash, "receive")

	resultRaw, _ := ecf.Encode(map[string]interface{}{
		"path":         storagePath,
		"content_hash": storedHash.Bytes(),
	})
	resultEntity, _ := entity.NewEntity("workbench/chain-errors/receive-result", cbor.RawMessage(resultRaw))
	return &handler.Response{Status: 200, Result: resultEntity}, nil
}
