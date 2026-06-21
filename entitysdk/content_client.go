package entitysdk

import (
	"context"
	"errors"
	"fmt"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/handler"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/store"
	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/content"
)

// ContentClient wraps system/content EXECUTE operations behind typed
// Go methods. Each ContentClient targets one peer (local or remote,
// selected at construction time via AppPeer.Content / AppPeer.ContentAt).
//
// Per EXTENSION-CONTENT v3.5 §6 (system content handler). The system
// content handler exposes capability-gated hash-addressed read access
// to the content store; it is the canonical cross-peer chunk-fetch
// surface for content not bound to a specific domain handler's
// tree-path namespace.
//
// Stage 3 use case: the cross-peer file-sync chain composes
// `local/files:read` (tree-path-scoped, narrow) with this client's
// `Get` op (namespace-scoped, broad) — local/files for the small-file
// inline-include happy path; system/content for files above the
// 64 KiB inline threshold where chunk-batch fetch is needed.
//
// **Response shape (post-F4 pin):** Per the post-A2 cycle
// close-out, CONTENT v3.6 + Go's impl converged on the spec-typed
// shape: `Found []hash.Hash` / `Missing []hash.Hash` in the response
// body, with the actual fetched entities arriving via the EXECUTE
// envelope's `Included` map. ContentClient.Get returns a wrapper
// (`ContentGetResult`) that surfaces both: the typed arrays for
// status checks, the entities map for downstream consumption. Fetched
// entities are also ingested into the local content store as a side
// effect (symmetric with RevisionClient.Fetch).
type ContentClient struct {
	ap         *AppPeer
	target     string
	contentURI string
}

// Content returns a ContentClient targeting the local peer's
// system/content handler. Reads under local-peer dispatch — used
// during workbench-internal compositions where cap discipline
// is provided by the chain dispatch capability that triggered
// the composition.
func (a *AppPeer) Content() *ContentClient {
	return &ContentClient{
		ap:         a,
		target:     a.PeerID(),
		contentURI: "system/content",
	}
}

// ContentAt returns a ContentClient targeting the named remote
// peer's system/content handler. Operations dispatch through the
// local peer's connection pool. The Stage 3 cross-peer chunk-fetch
// surface uses this.
func (a *AppPeer) ContentAt(peerID string) *ContentClient {
	return &ContentClient{
		ap:         a,
		target:     peerID,
		contentURI: extPeerURI(a.PeerID(), peerID, "system/content"),
	}
}

// PeerID returns the peer-id this ContentClient targets.
func (cc *ContentClient) PeerID() string { return cc.target }

// ContentGetResult is the workbench-side wrapper for a system/content:get
// dispatch. It carries the spec-typed response body (Found/Missing
// hash arrays) and the actual entities (from the EXECUTE envelope's
// Included map) in one struct so callers can both check status and
// consume the fetched entities.
type ContentGetResult struct {
	// Found is the array of hashes the sender had locally. Per
	// CONTENT v3.6 §6.2 (post-F4 pin).
	Found []hash.Hash

	// Missing is the array of hashes the sender did NOT have. Caller
	// treats this as L12 retry-eligible when sync-state visibility is
	// active on the sender, otherwise as terminal 404.
	Missing []hash.Hash

	// Entities maps each Found hash to its entity, as delivered via
	// the EXECUTE envelope's Included map. Mirrors what's been
	// ingested into the local content store; provided here for
	// callers that want direct access without re-reading the store.
	Entities map[hash.Hash]entity.Entity
}

// Get dispatches a batched hash-addressed read against the target
// peer's system/content handler. Returns Found/Missing arrays + the
// entities themselves (from envelope.Included).
//
// Entities returned are also written to the local content store as a
// side effect — symmetric with RevisionClient.Fetch and the §7.2
// transfer pattern. Callers can re-read them via
// ap.PeerContext().Store().Get(hash) after this returns; the Entities
// map on ContentGetResult is provided for direct access.
//
// L12: when the sender has sync-state visibility (active subscription
// + inbox-feeding-store predicate) and the hash is missing, the
// response surfaces 503 blob_pending_sync — the caller treats the
// fetch as retry-eligible. Without sync-state visibility the response
// is 404 (won't ever arrive); both are conformant per spec.
func (cc *ContentClient) Get(ctx context.Context, hashes []hash.Hash) (ContentGetResult, error) {
	params := types.ContentGetRequestData{Hashes: hashes}
	paramEnt, err := params.ToEntity()
	if err != nil {
		return ContentGetResult{}, WrapError(500, "encode_request",
			"encode ContentGetRequest", err)
	}
	// CONTENT v3.6 §6.2 path_required MUST: dispatch must carry a
	// resource targeting the namespace path. For the default
	// (no-namespace) ContentClient we target "system/content"; per-
	// namespace clients would target "system/content/{namespace}".
	// Bypass extDispatch because we need the response's Included map
	// (where the fetched entities actually live). extDispatch only
	// returns the result entity; system/content:get's result carries
	// the Found/Missing arrays while the entities themselves arrive
	// via the envelope.
	resource := &types.ResourceTarget{Targets: []string{"system/content"}}
	rawResp, err := cc.ap.executor.ExecuteOnResource(cc.contentURI, "get", paramEnt, resource)
	if err != nil {
		return ContentGetResult{}, err
	}
	if rawResp == nil {
		return ContentGetResult{}, NewError(500, "no_response",
			fmt.Sprintf("%s:get: no response", cc.contentURI))
	}
	if rawResp.Status >= 400 {
		if e := ErrorFromResponse(rawResp); e != nil {
			return ContentGetResult{}, e
		}
		return ContentGetResult{}, NewError(rawResp.Status, "ext_op_failed",
			fmt.Sprintf("%s:get returned status %d", cc.contentURI, rawResp.Status))
	}
	resultEnt := rawResp.Entity()
	if resultEnt.Type != types.TypeContentGetResponse {
		return ContentGetResult{}, NewError(500, "unexpected_result_type",
			fmt.Sprintf("system/content:get expected %s, got %s",
				types.TypeContentGetResponse, resultEnt.Type))
	}
	var typed types.ContentGetResponseData
	if err := decodeContentGetResponse(resultEnt, &typed); err != nil {
		return ContentGetResult{}, WrapError(500, "decode_failed",
			"decode ContentGetResponse", err)
	}

	// Surface the envelope's Included map alongside the typed
	// response arrays. Per CONTENT v3.6 §6.2 the result body carries
	// Found/Missing as hash arrays; the entities themselves arrive
	// via the EXECUTE envelope's Included map.
	entities := make(map[hash.Hash]entity.Entity, len(rawResp.Included))
	for h, ent := range rawResp.Included {
		entities[h] = ent
	}

	// Symmetric with RevisionClient.Fetch: ingest the included
	// entities into the local content store so subsequent lookups
	// resolve.
	for _, ent := range entities {
		if _, putErr := cc.ap.peer.Store().Put(ent); putErr != nil {
			return ContentGetResult{}, WrapError(500, "ingest_failed",
				fmt.Sprintf("write fetched entity %s into local store", ent.Type), putErr)
		}
	}
	return ContentGetResult{
		Found:    typed.Found,
		Missing:  typed.Missing,
		Entities: entities,
	}, nil
}

// FetchBlobClosure drives content.EnsureClosure against the target
// peer's system/content handler. Returns nil when the blob and all its
// chunks are present locally; returns an *Error with the partial-sync
// status (403 / 404 / 503) on failure.
//
// Per PROPOSAL-CONTENT-MATERIALIZATION v2 closure-think reframe +
// SDK-EXTENSION-OPERATIONS v0.8 §11: the §7.2 algorithm + §7.4 sender
// batching + 503 partial-sync retry all live inside the SDK helper
// now. This wrapper just builds the executor-backed Dispatcher and
// translates content.StatusError → entitysdk.Error.
//
// Idempotent + resumable: chunks already in the local content store
// are skipped (the receiver checks local presence before requesting).
func (cc *ContentClient) FetchBlobClosure(ctx context.Context, blobHash hash.Hash) error {
	disp := &executorDispatcher{
		executor:   cc.ap.executor,
		contentURI: cc.contentURI,
		cs:         cc.ap.peer.Store(),
	}
	if err := content.EnsureClosure(ctx, disp, blobHash, "system/content"); err != nil {
		var se *content.StatusError
		if errors.As(err, &se) {
			return NewError(se.Status, se.Code,
				fmt.Sprintf("blob %s on peer %s: %s", blobHash.String(), cc.target, se.Message))
		}
		return WrapError(500, "ensure_closure",
			fmt.Sprintf("blob %s on peer %s", blobHash.String(), cc.target), err)
	}
	return nil
}

// executorDispatcher adapts the AppPeer's Executor to handler.Dispatcher
// for content.EnsureClosure. The executor handles local/remote routing
// by URI authority; we rewrite the helper's bare "system/content" URI
// to the ContentClient's pre-resolved contentURI (local or cross-peer).
//
// Satisfies handler.Dispatcher + the structural storeCarrier interface
// EnsureClosure uses to read local-presence (see ext/content/sequencer.go).
type executorDispatcher struct {
	executor   *Executor
	contentURI string
	cs         store.ContentStore
}

func (d *executorDispatcher) Store() store.ContentStore { return d.cs }

func (d *executorDispatcher) Execute(ctx context.Context, req handler.ExecuteRequest) (handler.ExecuteResponse, error) {
	uri := req.URI
	if uri == "system/content" {
		uri = d.contentURI
	}
	var resp *Response
	var err error
	if req.Resource != nil {
		resp, err = d.executor.ExecuteOnResource(uri, req.Operation, req.Params, req.Resource)
	} else {
		resp, err = d.executor.ExecuteWithParams(uri, req.Operation, req.Params)
	}
	if err != nil {
		return handler.ExecuteResponse{}, err
	}
	if resp == nil {
		return handler.ExecuteResponse{}, nil
	}
	return handler.ExecuteResponse{
		Status:   resp.Status,
		Result:   resp.Entity(),
		Included: resp.Included,
	}, nil
}

// decodeContentGetResponse decodes the response entity into its
// typed data shape. Wrapped here so we have one decode site and a
// single place to migrate when the spec/impl shape divergence on
// Found/Missing resolves (today: counters; spec: arrays).
func decodeContentGetResponse(ent entity.Entity, out *types.ContentGetResponseData) error {
	return ecf.Decode(ent.Data, out)
}

// decodeBlob decodes a system/content/blob entity into its typed shape.
func decodeBlob(ent entity.Entity, out *types.ContentBlobData) error {
	return ecf.Decode(ent.Data, out)
}
