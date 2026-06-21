package entitysdk

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/handler"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"
)

// HandlerSpec declares a dynamic handler's tree-visible identity plus
// its authority. It maps onto system/handler/register-request
// (V7 §3.12) and becomes a system/handler/manifest entity under the
// hood, decomposed per N5 into an interface entity (public contract)
// and a handler entity (dispatch target with the security config).
//
// Pattern is the bare pattern — no leading slash. The SDK qualifies
// it to the local peer's namespace internally.
type HandlerSpec struct {
	Pattern       string
	Name          string
	Operations    map[string]types.HandlerOperationSpec
	InternalScope []types.GrantEntry
}

// HandlerFunc is the callable body the SDK binds to a pattern when
// RegisterHandler succeeds. It runs under the same contract as a
// bootstrap handler's Handle method — receives a dispatch request,
// returns a response or an error.
type HandlerFunc func(ctx context.Context, req *handler.Request) (*handler.Response, error)

// HandlerHandle is the opaque handle returned by RegisterHandler.
// Close unregisters both the dispatch index binding and the tree
// entries in reverse order (§11.5.2). Close is idempotent; repeated
// calls are no-ops.
type HandlerHandle struct {
	pattern  string
	ap       *AppPeer
	hasGrant bool

	mu     sync.Mutex
	closed bool
}

// Pattern returns the bare pattern this handler was registered at.
func (r *HandlerHandle) Pattern() string { return r.pattern }

// Close tears down the registration per §11.5.2:
//  1. Dispatch index first — stop accepting dispatch immediately.
//  2. Tree entries in reverse write order — grant, handler, interface.
//
// Safe to call more than once. Returns nil once the teardown has run.
func (r *HandlerHandle) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	r.mu.Unlock()

	r.ap.peer.Registry().Unregister(r.pattern)

	li := r.ap.peer.LocationIndex()
	if r.hasGrant {
		li.Remove("system/capability/grants/" + r.pattern)
	}
	li.Remove(r.pattern)
	li.Remove("system/handler/" + r.pattern)
	return nil
}

// RegisterHandler installs a language-native handler at spec.Pattern
// per SDK-OPERATIONS §11.5. Four mutations run in order: interface
// entity → handler entity → (optional) grant entity → dispatch index.
// If any step fails the SDK compensates by removing the writes that
// succeeded, in reverse, and returns an Error.
//
// A nil InternalScope is taken literally — the handler gets no grant
// and cannot call other handlers from its body (§11.5.3). The SDK
// never silently installs a wildcard grant.
//
// The pattern in spec is authoritative. Any pattern the body might
// self-declare is ignored.
func (a *AppPeer) RegisterHandler(spec HandlerSpec, body HandlerFunc) (*HandlerHandle, error) {
	if err := validateHandlerSpec(spec); err != nil {
		return nil, err
	}
	if body == nil {
		return nil, NewError(400, "invalid_handler_spec", "body is nil")
	}

	pattern := spec.Pattern
	ifacePath := "system/handler/" + pattern
	grantPath := "system/capability/grants/" + pattern

	li := a.peer.LocationIndex()
	registry := a.peer.Registry()

	// Collision check — either side of the tree/registry pair constitutes
	// a prior registration. Silent overwrite is not permitted (§11.5.1).
	if li.Has(pattern) || li.Has(ifacePath) {
		return nil, NewError(409, "pattern_collision",
			"a handler is already declared at pattern "+pattern)
	}
	if _, exists := registry.Handlers()[pattern]; exists {
		return nil, NewError(409, "pattern_collision",
			"a handler is already registered at pattern "+pattern)
	}

	cs := a.peer.Store()
	manifest := types.HandlerManifestData{
		Pattern:       pattern,
		Name:          spec.Name,
		Operations:    spec.Operations,
		InternalScope: spec.InternalScope,
	}

	// Compensation ledger — appended to in write order, executed in
	// reverse if any subsequent write fails (§11.5.4).
	var compensators []func()
	compensate := func() {
		for i := len(compensators) - 1; i >= 0; i-- {
			compensators[i]()
		}
	}

	// Step 1: interface entity at system/handler/{pattern}.
	ifaceEnt, err := manifest.InterfaceData().ToEntity()
	if err != nil {
		return nil, WrapError(500, "partial_registration_failure",
			"build interface entity", err)
	}
	ifaceHash, err := cs.Put(ifaceEnt)
	if err != nil {
		return nil, WrapError(500, "partial_registration_failure",
			"store interface entity", err)
	}
	li.Set(ifacePath, ifaceHash)
	compensators = append(compensators, func() { li.Remove(ifacePath) })

	// Step 2: handler entity at {pattern}, referencing the interface by
	// path (N2 normalization — dispatch target carries security config
	// + an interface ref; the public contract lives on the interface).
	handlerData := types.HandlerData{
		Interface:     ifacePath,
		InternalScope: spec.InternalScope,
	}
	handlerEnt, err := handlerData.ToEntity()
	if err != nil {
		compensate()
		return nil, WrapError(500, "partial_registration_failure",
			"build handler entity", err)
	}
	handlerHash, err := cs.Put(handlerEnt)
	if err != nil {
		compensate()
		return nil, WrapError(500, "partial_registration_failure",
			"store handler entity", err)
	}
	li.Set(pattern, handlerHash)
	compensators = append(compensators, func() { li.Remove(pattern) })

	// Step 3: grant entity at system/capability/grants/{pattern}. Only
	// written when InternalScope is non-nil (§11.5.1, §11.5.3 — null
	// scope means no outbound calls, no grant).
	hasGrant := false
	if len(spec.InternalScope) > 0 {
		capHash, err := mintHandlerGrant(a, cs, spec.InternalScope)
		if err != nil {
			compensate()
			return nil, WrapError(500, "partial_registration_failure",
				"mint handler grant", err)
		}
		li.Set(grantPath, capHash)
		compensators = append(compensators, func() { li.Remove(grantPath) })
		hasGrant = true
	}

	// Step 4: dispatch index. Registry.Register cannot fail in the
	// current core, but we still sequence it last so tree state is
	// never ahead of the dispatch binding on a partial failure.
	registry.Register(pattern, &dynamicHandler{name: spec.Name, body: body})

	return &HandlerHandle{
		pattern:  pattern,
		ap:       a,
		hasGrant: hasGrant,
	}, nil
}

// validateHandlerSpec enforces the minimum shape spec §11.5 requires.
// Empty pattern, leading slash, and empty operations list are all 400s.
func validateHandlerSpec(spec HandlerSpec) error {
	if spec.Pattern == "" {
		return NewError(400, "invalid_handler_spec", "pattern is empty")
	}
	if strings.HasPrefix(spec.Pattern, "/") {
		return NewError(400, "invalid_handler_spec",
			"pattern must be bare (no leading slash)")
	}
	if len(spec.Operations) == 0 {
		return NewError(400, "invalid_handler_spec", "operations list is empty")
	}
	if spec.Name == "" {
		return NewError(400, "invalid_handler_spec", "name is empty")
	}
	return nil
}

// mintHandlerGrant mirrors the bootstrap flow (core/peer/peer.go
// createHandlerGrants): build a self-granted capability token
// attenuated to the declared scope, sign it, and return the token's
// content hash after storing both the token and its signature.
func mintHandlerGrant(a *AppPeer, cs interface {
	Put(entity.Entity) (hash.Hash, error)
}, scope []types.GrantEntry) (hash.Hash, error) {
	identity := a.peer.Identity()
	kp := a.peer.Keypair()

	capData := types.CapabilityTokenData{
		Grants:    scope,
		Granter:   types.SingleSigGranter(identity.ContentHash),
		Grantee:   identity.ContentHash,
		CreatedAt: uint64(time.Now().UnixMilli()),
	}
	capEntity, err := capData.ToEntity()
	if err != nil {
		return hash.Hash{}, fmt.Errorf("build cap entity: %w", err)
	}

	sig := kp.Sign(capEntity.ContentHash.Bytes())
	sigEntity, err := types.SignatureData{
		Target:    capEntity.ContentHash,
		Signer:    identity.ContentHash,
		Algorithm: "ed25519",
		Signature: sig,
	}.ToEntity()
	if err != nil {
		return hash.Hash{}, fmt.Errorf("build sig entity: %w", err)
	}

	capHash, err := cs.Put(capEntity)
	if err != nil {
		return hash.Hash{}, fmt.Errorf("store cap entity: %w", err)
	}
	if _, err := cs.Put(sigEntity); err != nil {
		return hash.Hash{}, fmt.Errorf("store sig entity: %w", err)
	}
	return capHash, nil
}

// dynamicHandler wraps a HandlerFunc in the handler.Handler interface
// for registry installation. Name is surfaced for the registry's debug
// output; Manifest is deliberately not implemented — the SDK wrote the
// tree entities itself, so ManifestProvider is unnecessary (and
// implementing it would duplicate the truth that already lives in the
// tree).
type dynamicHandler struct {
	name string
	body HandlerFunc
}

func (h *dynamicHandler) Name() string { return h.name }

func (h *dynamicHandler) Handle(ctx context.Context, req *handler.Request) (*handler.Response, error) {
	return h.body(ctx, req)
}
