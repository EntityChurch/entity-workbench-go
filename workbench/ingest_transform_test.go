package workbench

import (
	"context"
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/handler"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/store"
	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/content"
	"go.entitychurch.org/entity-core-go/ext/content/chunker"
	"go.entitychurch.org/entity-core-go/ext/localfiles"

	"github.com/fxamacker/cbor/v2"
)

// ingestSubstrate is the test fixture that materializes a file body
// as a v1.2+ substrate blob — chunks + blob entity in the content
// store, returning the blob hash to embed in FileData.Content. This
// is the minimum dance every consumer test now has to perform; the
// shape is itself a v1.3-consumer-experience finding (see
// reviews/FEEDBACK-LOCAL-FILES-V1.3-CONSUMER-INTEGRATION §3).
func ingestSubstrate(t *testing.T, body string) (store.ContentStore, hash.Hash) {
	t.Helper()
	cs := store.NewMemoryContentStore()
	ranges := chunker.ChunkFastCDC([]byte(body), types.DefaultChunkSize)
	blobHash, err := content.IngestBlob([]byte(body), ranges, types.ChunkingFastCDC, types.DefaultChunkSize, cs)
	if err != nil {
		t.Fatalf("IngestBlob: %v", err)
	}
	return cs, blobHash
}

// TestIngestTransform_HappyPath covers the canonical case:
// localfiles.FileData (v1.2 substrate shape) with markdown content
// and a leading H1 produces a doc/markdown-file entity whose title
// is the heading text.
func TestIngestTransform_HappyPath(t *testing.T) {
	body := "# My Note\n\nSome body content.\n"
	cs, blobHash := ingestSubstrate(t, body)

	file := localfiles.FileData{
		Path:    "notes/test.md",
		Size:    uint64(len(body)),
		Content: blobHash,
	}
	ent, err := file.ToEntity()
	if err != nil {
		t.Fatalf("FileData.ToEntity: %v", err)
	}

	h := NewIngestTransformHandler()
	resp, err := h.Handle(context.Background(), &handler.Request{
		Params:  ent,
		Context: &handler.HandlerContext{Store: cs},
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if resp.Status != 200 {
		t.Fatalf("Status = %d, want 200; body=%q", resp.Status, string(resp.Result.Data))
	}
	if resp.Result.Type != MarkdownFileType {
		t.Errorf("Result.Type = %s, want %s", resp.Result.Type, MarkdownFileType)
	}

	md, err := MarkdownFileDataFromEntity(resp.Result)
	if err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if md.Title != "My Note" {
		t.Errorf("title = %q, want %q", md.Title, "My Note")
	}
	if md.Path != "notes/test.md" {
		t.Errorf("path = %q, want %q", md.Path, "notes/test.md")
	}
	if md.Content != blobHash {
		t.Errorf("content hash mismatch: got %s, want %s", md.Content, blobHash)
	}
	if md.Size != int64(len(body)) {
		t.Errorf("size = %d, want %d", md.Size, len(body))
	}
}

// TestIngestTransform_TitleFallsBackToFilename covers content with
// no H1 heading — the title comes from the file's base name (minus
// extension).
func TestIngestTransform_TitleFallsBackToFilename(t *testing.T) {
	body := "No heading here, just body text.\n"
	cs, blobHash := ingestSubstrate(t, body)

	file := localfiles.FileData{
		Path:    "notes/README.md",
		Size:    uint64(len(body)),
		Content: blobHash,
	}
	ent, _ := file.ToEntity()

	resp, err := NewIngestTransformHandler().Handle(
		context.Background(),
		&handler.Request{Params: ent, Context: &handler.HandlerContext{Store: cs}},
	)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	md, err := MarkdownFileDataFromEntity(resp.Result)
	if err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if md.Title != "README" {
		t.Errorf("title = %q, want %q", md.Title, "README")
	}
}

// TestIngestTransform_RejectsWrongInputType is the 400 case: chain
// composition must route only `local/files/file` entities into this
// handler. A bogus type is a chain-wiring bug; we surface it loudly.
func TestIngestTransform_RejectsWrongInputType(t *testing.T) {
	body := []byte{0xa0} // empty CBOR map
	ent, err := entity.NewEntity("doc/something-else", cbor.RawMessage(body))
	if err != nil {
		t.Fatalf("entity.NewEntity: %v", err)
	}

	resp, err := NewIngestTransformHandler().Handle(
		context.Background(), &handler.Request{Params: ent})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if resp.Status != 400 {
		t.Errorf("Status = %d, want 400", resp.Status)
	}
	if !strings.Contains(string(resp.Result.Data), "invalid_input_type") {
		t.Errorf("expected invalid_input_type in error body, got %q", string(resp.Result.Data))
	}
}

// TestIngestTransform_BlobPendingSync covers the partial-sync condition:
// a file entity references a blob that hasn't reached the local content
// store yet. Per DOMAIN-LOCAL-FILES §5.3 + L12 (Amendment 2 of v1.3)
// the canonical name is blob_pending_sync; consumers receiving this MUST
// treat the underlying operation as eligible for retry without operator
// intervention.
func TestIngestTransform_BlobPendingSync(t *testing.T) {
	cs := store.NewMemoryContentStore()
	// Construct a plausible-looking but absent blob hash by ingesting
	// then immediately re-using its hash against a fresh empty store.
	ghostStore, blobHash := ingestSubstrate(t, "body that lives only in the other peer's store\n")
	_ = ghostStore

	file := localfiles.FileData{
		Path:    "notes/missing.md",
		Size:    100,
		Content: blobHash,
	}
	ent, _ := file.ToEntity()

	resp, err := NewIngestTransformHandler().Handle(
		context.Background(),
		&handler.Request{Params: ent, Context: &handler.HandlerContext{Store: cs}},
	)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if resp.Status != 503 {
		t.Errorf("Status = %d, want 503 (blob not arrived); body=%q", resp.Status, string(resp.Result.Data))
	}
	if !strings.Contains(string(resp.Result.Data), "blob_pending_sync") {
		t.Errorf("expected blob_pending_sync in error body, got %q", string(resp.Result.Data))
	}
}
