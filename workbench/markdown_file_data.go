package workbench

import (
	"fmt"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/store"
	"go.entitychurch.org/entity-core-go/core/types"

	"github.com/fxamacker/cbor/v2"
)

// MarkdownFileData is the workbench-local typed payload for a
// `doc/markdown-file` entity.
//
// Content lives in the CONTENT v3.6 substrate as a blob + chunks; the
// entity itself carries only metadata + a hash reference. This mirrors
// the DOMAIN-LOCAL-FILES v1.3 §2.1 file-entity shape: `FileData.Content`
// is a hash into `system/content/blob`; bytes are reassembled on read
// via ResolveBlobBytes (local-only) or content.EnsureClosure (cross-peer
// sync-aware).
//
// Closes the >10MB silent failure cliff that the inline-string shape
// produced — ingest-transform no longer materializes payload bytes into
// the typed entity; large files round-trip through the substrate.
//
// Size is the total payload size in bytes (mirrors blob.TotalSize). Kept
// in the typed entity so listings + UI can show file sizes without
// loading the blob.
type MarkdownFileData struct {
	Path    string    `cbor:"path"`
	Title   string    `cbor:"title"`
	Content hash.Hash `cbor:"content"` // hash of system/content/blob
	Size    int64     `cbor:"size"`
}

// ToEntity encodes the typed payload as an entity of type
// MarkdownFileType ("doc/markdown-file").
func (d MarkdownFileData) ToEntity() (entity.Entity, error) {
	raw, err := ecf.Encode(d)
	if err != nil {
		return entity.Entity{}, fmt.Errorf("encode doc/markdown-file: %w", err)
	}
	return entity.NewEntity(MarkdownFileType, cbor.RawMessage(raw))
}

// MarkdownFileDataFromEntity decodes a doc/markdown-file entity body.
// Rejects entities of any other type.
func MarkdownFileDataFromEntity(ent entity.Entity) (MarkdownFileData, error) {
	if ent.Type != MarkdownFileType {
		return MarkdownFileData{}, fmt.Errorf("expected %s, got %s", MarkdownFileType, ent.Type)
	}
	var d MarkdownFileData
	if err := ecf.Decode(ent.Data, &d); err != nil {
		return MarkdownFileData{}, fmt.Errorf("decode doc/markdown-file: %w", err)
	}
	return d, nil
}

// LoadMarkdownContent reassembles the content blob for a doc/markdown-file
// entity from the local content store. Returns (bytes, true, nil) when
// the blob is locally complete, (nil, false, nil) when the blob is not
// yet present (chain-retry territory — caller surfaces 503
// blob_pending_sync), or (nil, true, err) for reassembly errors.
//
// For >64MiB blobs callers SHOULD stream chunks one-at-a-time via
// LoadMarkdownContentStream instead — this helper buffers the full
// payload. (Mirrors DOMAIN-LOCAL-FILES §4.3 streaming SHOULD at 64MiB.)
func LoadMarkdownContent(cs store.ContentStore, d MarkdownFileData) ([]byte, bool, error) {
	blobEnt, ok := cs.Get(d.Content)
	if !ok {
		return nil, false, nil
	}
	var blob types.ContentBlobData
	if err := ecf.Decode(blobEnt.Data, &blob); err != nil {
		return nil, true, fmt.Errorf("decode blob: %w", err)
	}
	buf := make([]byte, 0, blob.TotalSize)
	for i, chunkHash := range blob.Chunks {
		ent, ok := cs.Get(chunkHash)
		if !ok {
			return nil, true, fmt.Errorf("chunk %d missing from local content store", i)
		}
		var chunk types.ContentChunkData
		if err := ecf.Decode(ent.Data, &chunk); err != nil {
			return nil, true, fmt.Errorf("decode chunk %d: %w", i, err)
		}
		buf = append(buf, chunk.Payload...)
	}
	if uint64(len(buf)) != blob.TotalSize {
		return nil, true, fmt.Errorf("reassembled size %d != blob total_size %d", len(buf), blob.TotalSize)
	}
	return buf, true, nil
}

// LoadMarkdownFirstChunk returns the bytes of the first chunk of the
// content blob, without materializing the full payload. Useful for
// metadata extraction (e.g. first-heading title scan) on arbitrarily
// large files. Returns (nil, false, nil) when the blob is missing,
// (chunkBytes, true, nil) on success, or (nil, true, err) on decode
// failure / empty blob (zero chunks).
func LoadMarkdownFirstChunk(cs store.ContentStore, blobHash hash.Hash) ([]byte, bool, error) {
	blobEnt, ok := cs.Get(blobHash)
	if !ok {
		return nil, false, nil
	}
	var blob types.ContentBlobData
	if err := ecf.Decode(blobEnt.Data, &blob); err != nil {
		return nil, true, fmt.Errorf("decode blob: %w", err)
	}
	if len(blob.Chunks) == 0 {
		return []byte{}, true, nil
	}
	chunkEnt, ok := cs.Get(blob.Chunks[0])
	if !ok {
		return nil, true, fmt.Errorf("first chunk missing from local content store")
	}
	var chunk types.ContentChunkData
	if err := ecf.Decode(chunkEnt.Data, &chunk); err != nil {
		return nil, true, fmt.Errorf("decode first chunk: %w", err)
	}
	return chunk.Payload, true, nil
}
