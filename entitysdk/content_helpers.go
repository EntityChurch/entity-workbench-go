package entitysdk

import (
	"fmt"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/store"
	"go.entitychurch.org/entity-core-go/core/types"
)

// ReassembleBlob loads a blob's chunks from the content store and
// returns the concatenated payload. Mirrors the package-private
// reassembleBlob in ext/localfiles.
//
// IMPORTANT — capability discipline (L10 outcome, Amendment 2):
//
// Arch resolved that reassemble is an algorithm reference in
// CONTENT §3.4, NOT a public
// substrate primitive. Capability-checked access continues through
// dispatcher-mediated handler ops: local/files:read for tree-path-scoped
// access (narrow); system/content:get for namespace-scoped access (broad).
// Direct ContentStore reads stay handler-private; substrate-direct access
// is a trusted-context-only operation.
//
// This helper is workbench-internal. It implements the published algorithm
// and is opt-in to workbench-go's own threat model: we accept that calling
// it bypasses tree-path cap discipline. Workbench code that holds a
// ContentStore reference at this layer is trusted code (the peer's own
// substrate machinery, internal handlers, chain transforms operating
// under the chain dispatch capability).
//
// Code that crosses workbench's trust boundary (e.g., a chain step
// resolving a file entity that arrived via subscription, particularly
// cross-peer) SHOULD dispatch through local/files:read instead. The
// dispatched path returns the file entity with the blob and (for files
// under the 64 KiB inline threshold) the chunks via response envelope
// `included` — so substrate reassembly happens under handler authority,
// not via this helper. See workbench/blob_resolve.go for the canonical
// dispatch-mediated chain step.
//
// Per CONTENT v3.5 §3.4, reassembly is byte-concatenation in chunks-list
// order — FastCDC is symmetric and produces no boundary metadata to
// interpret on the read side.
//
// Returns an error if the blob payload won't decode, if a chunk is
// missing from the local content store (caller should treat this as
// L12 blob_pending_sync — retry on next sync event per
// DOMAIN-LOCAL-FILES §5.3 + Amendment 2), or if the reassembled size
// doesn't match the blob's declared TotalSize.
func ReassembleBlob(cs store.ContentStore, blobEnt entity.Entity) ([]byte, error) {
	var blob types.ContentBlobData
	if err := ecf.Decode(blobEnt.Data, &blob); err != nil {
		return nil, fmt.Errorf("decode blob: %w", err)
	}
	buf := make([]byte, 0, blob.TotalSize)
	for i, chunkHash := range blob.Chunks {
		ent, ok := cs.Get(chunkHash)
		if !ok {
			return nil, fmt.Errorf("chunk %d (%x) missing from content store", i, chunkHash.Digest[:8])
		}
		var chunk types.ContentChunkData
		if err := ecf.Decode(ent.Data, &chunk); err != nil {
			return nil, fmt.Errorf("decode chunk %d: %w", i, err)
		}
		buf = append(buf, chunk.Payload...)
	}
	if uint64(len(buf)) != blob.TotalSize {
		return nil, fmt.Errorf("reassembled size %d does not match blob total_size %d", len(buf), blob.TotalSize)
	}
	return buf, nil
}

// ResolveBlobBytes is the convenience form that takes a blob hash,
// looks it up in the content store, and reassembles. Returns
// (nil, false, nil) when the blob entity itself is missing — callers
// distinguish "not yet arrived" from real errors. Returns
// (nil, true, err) when the blob is present but reassembly fails
// (chunk missing, decode error, size mismatch).
func ResolveBlobBytes(cs store.ContentStore, blobHash hash.Hash) ([]byte, bool, error) {
	blobEnt, ok := cs.Get(blobHash)
	if !ok {
		return nil, false, nil
	}
	bytes, err := ReassembleBlob(cs, blobEnt)
	return bytes, true, err
}
