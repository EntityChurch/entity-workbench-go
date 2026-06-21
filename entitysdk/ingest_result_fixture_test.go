package entitysdk_test

import (
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"

	"github.com/fxamacker/cbor/v2"
)

// TestContentIngestResult_FixtureForCrossImpl is the cross-impl
// fixture for the v3.4 ingest-result shape (after
// PROPOSAL-CONTENT-INGEST-PASS-THROUGH).
//
// Logs the Go canonical encoding of a known ContentIngestResultData
// value (with the new Root field populated). Rust + Python SDKs can
// construct the equivalent input, encode, and compare against the
// hex string below. Match → cross-impl content-addressing works.
func TestContentIngestResult_FixtureForCrossImpl(t *testing.T) {
	// Reference input — a FetchResultData wrapper that revision/fetch
	// would return as envelope.root. Using a deterministic hash so
	// the fixture is reproducible.
	versionHash, _ := hash.Compute("test/version-hash-marker", cbor.RawMessage{0xa0})
	if versionHash.IsZero() {
		t.Fatal("fixture: versionHash zero")
	}
	fetchResultEnt, err := types.RevisionFetchResultData{Head: versionHash}.ToEntity()
	if err != nil {
		t.Fatal(err)
	}

	// Build the ingest-result with the new Root pass-through field.
	rootCopy := fetchResultEnt
	ingestResult := types.ContentIngestResultData{
		Root:          &rootCopy,
		RootHash:      fetchResultEnt.ContentHash,
		IngestedCount: 5,
	}
	ingestResultEnt, err := ingestResult.ToEntity()
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("--- Cross-impl fixture: ContentIngestResultData v3.4 ---")
	t.Logf("input:")
	t.Logf("  Root.Type:           %s", rootCopy.Type)
	t.Logf("  Root.ContentHash:    %s", rootCopy.ContentHash)
	t.Logf("  Root.Data length:    %d bytes", len(rootCopy.Data))
	t.Logf("  RootHash:            %s", ingestResult.RootHash)
	t.Logf("  IngestedCount:       %d", ingestResult.IngestedCount)
	t.Logf("encoded:")
	t.Logf("  Entity type:         %s", ingestResultEnt.Type)
	t.Logf("  Entity content_hash: %s", ingestResultEnt.ContentHash)
	t.Logf("  Entity.Data length:  %d bytes", len(ingestResultEnt.Data))
	t.Logf("  Entity.Data (hex):   %s", hexFixture(ingestResultEnt.Data))

	// Determinism — encode twice, byte-identical.
	again, err := ingestResult.ToEntity()
	if err != nil {
		t.Fatal(err)
	}
	if again.ContentHash != ingestResultEnt.ContentHash {
		t.Errorf("non-deterministic encoding: %s != %s",
			again.ContentHash, ingestResultEnt.ContentHash)
	}

	// Round-trip check — decode and verify Root is preserved.
	var decoded types.ContentIngestResultData
	if err := ecf.Decode(ingestResultEnt.Data, &decoded); err != nil {
		t.Fatalf("decode round-trip: %v", err)
	}
	if decoded.Root == nil {
		t.Fatal("decoded Root is nil — pass-through dropped")
	}
	if decoded.Root.Type != fetchResultEnt.Type {
		t.Errorf("decoded Root.Type = %q, want %q",
			decoded.Root.Type, fetchResultEnt.Type)
	}
	if decoded.RootHash != ingestResult.RootHash {
		t.Errorf("decoded RootHash mismatch")
	}

	// Verify the Root entity's content hash matches when re-derived
	// (proves Root.Data round-tripped without corruption).
	rederivedHash, err := hash.Compute(decoded.Root.Type, decoded.Root.Data)
	if err != nil {
		t.Fatalf("rederive Root hash: %v", err)
	}
	if rederivedHash != fetchResultEnt.ContentHash {
		t.Errorf("Root.Data round-trip corrupted: rederived %s != original %s",
			rederivedHash, fetchResultEnt.ContentHash)
	}
	t.Logf("✓ Root pass-through round-trips with identical content hash")

	// Inhibits unused import warnings on entity.Entity for stricter linters.
	_ = entity.Entity{}
}

func hexFixture(b []byte) string {
	const digits = "0123456789abcdef"
	var sb strings.Builder
	sb.Grow(len(b) * 2)
	for _, c := range b {
		sb.WriteByte(digits[c>>4])
		sb.WriteByte(digits[c&0xf])
	}
	return sb.String()
}
