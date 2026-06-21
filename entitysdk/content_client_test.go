package entitysdk_test

import (
	"context"
	"testing"

	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/content"
	"go.entitychurch.org/entity-core-go/ext/content/chunker"

	"entity-workbench-go/entitysdk"
)

// TestContentClient_LocalGet_HappyPath exercises Get against the
// local peer: ingest a blob via the substrate, then fetch it back via
// the dispatcher-mediated ContentClient. The local-peer case is the
// foundation for the cross-peer case — the URI changes, the response
// shape doesn't.
func TestContentClient_LocalGet_HappyPath(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	defer ap.Close()

	body := []byte("# A small probe file\n\nfor ContentClient.Get round-trip.\n")
	ranges := chunker.ChunkFastCDC(body, types.DefaultChunkSize)
	blobHash, err := content.IngestBlob(body, ranges, types.ChunkingFastCDC, types.DefaultChunkSize,
		ap.RawContentStore())
	if err != nil {
		t.Fatalf("IngestBlob: %v", err)
	}

	cc := ap.Content()
	if got := cc.PeerID(); got != ap.PeerID() {
		t.Errorf("Local ContentClient targets %s, want %s", got, ap.PeerID())
	}

	resp, err := cc.Get(context.Background(), []hash.Hash{blobHash})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if _, ok := resp.Entities[blobHash]; !ok {
		t.Errorf("expected blob %s in resp.Entities, got %d entries (Found=%v Missing=%v)",
			blobHash.String(), len(resp.Entities), resp.Found, resp.Missing)
	}
	// F4 pin: Found/Missing are arrays of hashes.
	// Confirm the typed body shape carries the result symmetrically
	// with the Entities map.
	if len(resp.Found) != 1 || resp.Found[0] != blobHash {
		t.Errorf("Found = %v, want [%s]", resp.Found, blobHash)
	}
	if len(resp.Missing) != 0 {
		t.Errorf("Missing = %v, want []", resp.Missing)
	}
}

// TestContentClient_LocalGet_Missing covers the not-found case.
func TestContentClient_LocalGet_Missing(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	defer ap.Close()

	bogus := hash.Hash{}
	resp, err := ap.Content().Get(context.Background(), []hash.Hash{bogus})
	if err != nil {
		// system/content:get may legitimately return 404 for entirely-
		// missing hashes; either error or empty response is acceptable
		// per CONTENT §6.2. Asserting either-shape here would over-pin.
		t.Logf("Get(bogus) error (acceptable): %v", err)
		return
	}
	if _, ok := resp.Entities[bogus]; ok {
		t.Errorf("expected bogus hash absent from Entities, got present")
	}
}

// TestContentClient_FetchBlobClosure_LocalNoOp covers the
// already-resident case: ingest a blob locally, FetchBlobClosure
// should be a no-op (chunks present, blob present).
func TestContentClient_FetchBlobClosure_LocalNoOp(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	defer ap.Close()

	body := []byte("body for closure-fetch local-noop test\n")
	ranges := chunker.ChunkFastCDC(body, types.DefaultChunkSize)
	blobHash, err := content.IngestBlob(body, ranges, types.ChunkingFastCDC, types.DefaultChunkSize,
		ap.RawContentStore())
	if err != nil {
		t.Fatalf("IngestBlob: %v", err)
	}

	if err := ap.Content().FetchBlobClosure(context.Background(), blobHash); err != nil {
		t.Errorf("FetchBlobClosure should be no-op when blob+chunks are local: %v", err)
	}
}
