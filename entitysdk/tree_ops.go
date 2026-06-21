package entitysdk

import (
	"sort"
	"strings"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/tree"
	"go.entitychurch.org/entity-core-go/core/types"
)

// The Level 1 dispatched tree operations in this file go through the
// system/tree handler. Every call is capability-checked, fires the
// full emit pathway, and works the same whether the target is local
// or remote. Per SDK-OPERATIONS §2.7 / §3, these are what application
// code SHOULD use by default.
//
// Path forms accepted by Get/Put/Has/Remove/List:
//
//   - bare path             ("foo/bar")          → local
//   - "entity://{id}/foo"   (URI form)           → local if {id} is this
//                                                  peer, remote otherwise
//   - "/{id}/foo"           (peer-qualified)     → same routing rule
//
// For remote forms, the executor's URI-aware branch (executor.go)
// dispatches via peer.Dispatcher.RemoteExecute over the local peer's
// connection pool. AppPeer.Connect must have set up the connection
// (and seeded the transport address via RegisterRemote) first.
//
// For direct-store access that bypasses dispatch entirely, use
// AppPeer.Store() — see store.go. The two surfaces are intentionally
// distinct so the security boundary is visible at every call site.

// resolveDispatchTarget interprets a caller-supplied path and returns
// the handler URI to dispatch (peer-qualified for remote, bare for
// local) plus the resource path to embed in the request.
//
// Routing rule: if the input names a peer-id different from this
// AppPeer's own, return a remote-dispatch tuple; otherwise return
// a local-dispatch tuple. See SHELL-DIRECTION.md §4.4 / §4.5.1.
func (a *AppPeer) resolveDispatchTarget(path string) (handlerURI, resourcePath string) {
	const localHandler = "system/tree"
	localID := a.PeerID()

	if u, err := entity.ParseURI(path); err == nil && u.PeerID != "" {
		if u.PeerID == localID {
			return localHandler, u.Path
		}
		return "entity://" + u.PeerID + "/" + localHandler, u.Path
	}

	if strings.HasPrefix(path, "/") {
		rest := strings.TrimPrefix(path, "/")
		if idx := strings.Index(rest, "/"); idx > 0 {
			peerID := rest[:idx]
			barePath := rest[idx+1:]
			if peerID == localID {
				return localHandler, barePath
			}
			return "entity://" + peerID + "/" + localHandler, barePath
		}
	}

	return localHandler, path
}

// Get retrieves the entity bound at path via the system/tree handler.
// Returns (zero, false) if no binding exists. Path may be local or
// peer-qualified (see resolveDispatchTarget).
func (a *AppPeer) Get(path string) (entity.Entity, bool, error) {
	handlerURI, resourcePath := a.resolveDispatchTarget(path)
	getReq, resource, err := tree.CreateGetRequest(resourcePath, "entity")
	if err != nil {
		return entity.Entity{}, false, WrapError(400, "invalid_request", "build get request", err)
	}

	resp, err := a.executor.ExecuteOnResource(handlerURI, "get", getReq, resource)
	if err != nil {
		if IsNotFound(err) {
			return entity.Entity{}, false, nil
		}
		return entity.Entity{}, false, err
	}
	return resp.Entity(), true, nil
}

// Put stores an entity at path via the system/tree handler. Returns
// the content hash of the stored entity.
func (a *AppPeer) Put(path, typeName string, data interface{}) (hash.Hash, error) {
	return a.putDispatched(path, typeName, data, nil)
}

// PutCAS is a compare-and-swap Put dispatched through system/tree.
// The write succeeds only if the current binding matches expected.
// Pass a zero hash.Hash to mean "path must not exist" — and in that
// case the handler creates without a CAS check (no equivalent of
// "create-only" at the handler layer today; use the Store L0 form if
// you need that exact semantic).
func (a *AppPeer) PutCAS(path, typeName string, data interface{}, expected hash.Hash) (hash.Hash, error) {
	var expectedPtr *hash.Hash
	if !expected.IsZero() {
		expectedPtr = &expected
	}
	return a.putDispatched(path, typeName, data, expectedPtr)
}

func (a *AppPeer) putDispatched(path, typeName string, data interface{}, expected *hash.Hash) (hash.Hash, error) {
	raw, err := encodeData(data)
	if err != nil {
		return hash.Hash{}, WrapError(400, "encode_failed", "put encode", err)
	}
	ent, err := entity.NewEntity(typeName, raw)
	if err != nil {
		return hash.Hash{}, WrapError(400, "invalid_entity", "put entity", err)
	}

	handlerURI, resourcePath := a.resolveDispatchTarget(path)
	putReq, resource, err := tree.CreatePutRequestCAS(resourcePath, &ent, expected)
	if err != nil {
		return hash.Hash{}, WrapError(400, "invalid_request", "build put request", err)
	}

	resp, err := a.executor.ExecuteOnResource(handlerURI, "put", putReq, resource)
	if err != nil {
		return hash.Hash{}, err
	}

	// The put handler's result entity contains `{content_hash: ...}` —
	// decode it out so callers get the stored hash back per spec §3.2.
	var result struct {
		ContentHash []byte `cbor:"content_hash"`
	}
	if err := ecf.Decode(resp.Data, &result); err == nil && len(result.ContentHash) > 0 {
		if h, herr := hash.FromBytes(result.ContentHash); herr == nil {
			return h, nil
		}
	}
	return ent.ContentHash, nil
}

// PutEntity stores ent verbatim at path via the system/tree handler.
// Use this when copying or replicating an existing entity: the
// content hash is preserved because no re-encoding occurs. For
// typical writes from typed Go data, use Put.
func (a *AppPeer) PutEntity(path string, ent entity.Entity) (hash.Hash, error) {
	handlerURI, resourcePath := a.resolveDispatchTarget(path)
	putReq, resource, err := tree.CreatePutRequest(resourcePath, &ent)
	if err != nil {
		return hash.Hash{}, WrapError(400, "invalid_request", "build put request", err)
	}
	resp, err := a.executor.ExecuteOnResource(handlerURI, "put", putReq, resource)
	if err != nil {
		return hash.Hash{}, err
	}

	var result struct {
		ContentHash []byte `cbor:"content_hash"`
	}
	if err := ecf.Decode(resp.Data, &result); err == nil && len(result.ContentHash) > 0 {
		if h, herr := hash.FromBytes(result.ContentHash); herr == nil {
			return h, nil
		}
	}
	return ent.ContentHash, nil
}

// Has reports whether path is currently bound. Wraps a dispatched
// get with mode=hash per SDK-OPERATIONS §3.5 ("MAY implement as
// get(path) != null"). Returns false on 404, propagates other errors.
func (a *AppPeer) Has(path string) (bool, error) {
	handlerURI, resourcePath := a.resolveDispatchTarget(path)
	getReq, resource, err := tree.CreateGetRequest(resourcePath, "hash")
	if err != nil {
		return false, WrapError(400, "invalid_request", "build get request", err)
	}
	_, err = a.executor.ExecuteOnResource(handlerURI, "get", getReq, resource)
	if err != nil {
		if IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Remove unbinds path via the system/tree handler. Implemented as a
// dispatched put with an empty entity payload — the tree handler
// treats that as an unbind (core/tree/handler.go handlePut).
//
// Returns a 404 SDK Error when path is not bound. Per SDK-OPERATIONS
// §3.4 implementations MAY silently succeed on 404; callers that want
// that behavior should check IsNotFound and discard.
func (a *AppPeer) Remove(path string) error {
	handlerURI, resourcePath := a.resolveDispatchTarget(path)
	putReq, resource, err := tree.CreatePutRequest(resourcePath, nil)
	if err != nil {
		return WrapError(400, "invalid_request", "build put request", err)
	}
	_, err = a.executor.ExecuteOnResource(handlerURI, "put", putReq, resource)
	return err
}

// Entry is a single row returned by List. Matches SDK-OPERATIONS §3.3.
type Entry struct {
	Name        string
	Path        string
	ContentHash hash.Hash
	HasChildren bool
}

// List returns the direct children of prefix via a dispatched trailing-
// slash get (core routes that to handleListing). Rows are sorted by
// name. SDK-OPERATIONS §3.3.
func (a *AppPeer) List(prefix string) ([]Entry, error) {
	handlerURI, resourcePath := a.resolveDispatchTarget(prefix)
	listingPath := resourcePath
	if listingPath != "" && !strings.HasSuffix(listingPath, "/") {
		listingPath += "/"
	}

	getReq, resource, err := tree.CreateGetRequest(listingPath, "entity")
	if err != nil {
		return nil, WrapError(400, "invalid_request", "build list request", err)
	}
	resp, err := a.executor.ExecuteOnResource(handlerURI, "get", getReq, resource)
	if err != nil {
		return nil, err
	}

	// Decode directly into a typed shape. The wire form is
	// map[name]{hash, has_children}; the SDK surfaces it as typed
	// Entry rows with Name/Path/ContentHash/HasChildren.
	type wireEntry struct {
		Hash        []byte `cbor:"hash"`
		HasChildren bool   `cbor:"has_children"`
	}
	var decoded struct {
		Path    string               `cbor:"path"`
		Entries map[string]wireEntry `cbor:"entries"`
	}
	if err := ecf.Decode(resp.Data, &decoded); err != nil {
		return nil, WrapError(500, "decode_failed", "decode listing", err)
	}

	names := make([]string, 0, len(decoded.Entries))
	for name := range decoded.Entries {
		names = append(names, name)
	}
	sort.Strings(names)

	pathPrefix := strings.TrimSuffix(prefix, "/")
	rows := make([]Entry, 0, len(names))
	for _, name := range names {
		we := decoded.Entries[name]
		var h hash.Hash
		if len(we.Hash) > 0 {
			if parsed, herr := hash.FromBytes(we.Hash); herr == nil {
				h = parsed
			}
		}
		full := name
		if pathPrefix != "" {
			full = pathPrefix + "/" + name
		}
		rows = append(rows, Entry{
			Name:        name,
			Path:        full,
			ContentHash: h,
			HasChildren: we.HasChildren,
		})
	}
	return rows, nil
}

// Snapshot captures the tree under prefix as a content-addressed Merkle
// trie, persists the snapshot envelope entity, and returns its content
// hash — the handle Diff / Merge accept. SDK-OPERATIONS §3.6.
//
// The core handler computes the trie root but intentionally does not
// persist the wrapping snapshot entity (callers that want to transmit
// it via an Extract envelope, for example, may not want it in the
// local store). The SDK persists by default so the typical
// Snapshot→Diff flow composes without the caller managing entity
// lifecycle.
func (a *AppPeer) Snapshot(prefix string) (hash.Hash, error) {
	reqEntity, err := types.SnapshotRequestData{Prefix: prefix}.ToEntity()
	if err != nil {
		return hash.Hash{}, WrapError(400, "invalid_request", "build snapshot request", err)
	}
	resp, err := a.executor.ExecuteWithParams("system/tree", "snapshot", reqEntity)
	if err != nil {
		return hash.Hash{}, err
	}
	h, perr := a.peer.Store().Put(resp.Entity())
	if perr != nil {
		return hash.Hash{}, WrapError(500, "store_failed", "persist snapshot entity", perr)
	}
	return h, nil
}

// Diff returns the structural difference between two snapshot roots.
// SDK-OPERATIONS §3.7.
func (a *AppPeer) Diff(base, target hash.Hash) (types.DiffData, error) {
	reqEntity, err := types.DiffRequestData{Base: base, Target: target}.ToEntity()
	if err != nil {
		return types.DiffData{}, WrapError(400, "invalid_request", "build diff request", err)
	}
	resp, err := a.executor.ExecuteWithParams("system/tree", "diff", reqEntity)
	if err != nil {
		return types.DiffData{}, err
	}
	diff, derr := types.DiffDataFromEntity(resp.Entity())
	if derr != nil {
		return types.DiffData{}, WrapError(500, "decode_failed", "decode diff", derr)
	}
	return diff, nil
}

// Merge applies source onto the target tree using the supplied strategy.
// SDK-OPERATIONS §3.8. Callers build MergeRequestData directly so the
// full set of merge knobs (strategy, dry_run, source/target prefix,
// envelope ingest) remains available without wrapping every field.
func (a *AppPeer) Merge(req types.MergeRequestData) (types.MergeResultData, error) {
	reqEntity, err := req.ToEntity()
	if err != nil {
		return types.MergeResultData{}, WrapError(400, "invalid_request", "build merge request", err)
	}
	resp, err := a.executor.ExecuteWithParams("system/tree", "merge", reqEntity)
	if err != nil {
		return types.MergeResultData{}, err
	}
	result, derr := types.MergeResultDataFromEntity(resp.Entity())
	if derr != nil {
		return types.MergeResultData{}, WrapError(500, "decode_failed", "decode merge result", derr)
	}
	return result, nil
}

// Extract returns a portable envelope of the tree under prefix from the
// given snapshot. SDK-OPERATIONS §3.9.
func (a *AppPeer) Extract(snapshot hash.Hash, prefix string, paths []string) (entity.Entity, error) {
	reqEntity, err := types.ExtractRequestData{Prefix: prefix, Paths: paths}.ToEntity()
	if err != nil {
		return entity.Entity{}, WrapError(400, "invalid_request", "build extract request", err)
	}
	resp, err := a.executor.ExecuteWithParams("system/tree", "extract", reqEntity)
	if err != nil {
		return entity.Entity{}, err
	}
	return resp.Entity(), nil
}
