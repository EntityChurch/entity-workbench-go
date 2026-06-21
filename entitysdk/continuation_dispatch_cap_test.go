package entitysdk_test

import (
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"

	"github.com/fxamacker/cbor/v2"
)

// TestSetDefaultDispatchCap exercises the per-step cap default
// helper — collapses N repetitions of `DispatchCapability: capHash`
// to one call. Pre-existing per-step caps are preserved (override).
func TestSetDefaultDispatchCap(t *testing.T) {
	rootCap, _ := hash.Compute("test/cap-root", cbor.RawMessage{0xa0})
	overrideCap, _ := hash.Compute("test/cap-override", cbor.RawMessage{0xa0})
	if rootCap == overrideCap || rootCap.IsZero() || overrideCap.IsZero() {
		t.Fatalf("fixture hashes invalid: root=%v override=%v", rootCap, overrideCap)
	}

	// Three continuations: first two have no cap; third has an override.
	a := types.ContinuationData{Target: "h/a", Operation: "op"}
	b := types.ContinuationData{Target: "h/b", Operation: "op"}
	c := types.ContinuationData{Target: "h/c", Operation: "op", DispatchCapability: overrideCap}

	entitysdk.SetDefaultDispatchCap(rootCap, &a, &b, &c)

	if a.DispatchCapability != rootCap {
		t.Errorf("a: cap = %v, want root", a.DispatchCapability)
	}
	if b.DispatchCapability != rootCap {
		t.Errorf("b: cap = %v, want root", b.DispatchCapability)
	}
	if c.DispatchCapability != overrideCap {
		t.Errorf("c: cap = %v, want override (per-step preserved)", c.DispatchCapability)
	}
}

// TestSetDefaultDispatchCap_NilSafe: nil entries in the variadic
// shouldn't panic. Idempotent for completeness.
func TestSetDefaultDispatchCap_NilSafe(t *testing.T) {
	rootCap, _ := hash.Compute("test/cap", cbor.RawMessage{0xa0})
	a := types.ContinuationData{Target: "h", Operation: "op"}
	entitysdk.SetDefaultDispatchCap(rootCap, nil, &a, nil)
	if a.DispatchCapability != rootCap {
		t.Errorf("cap not applied through nil entries")
	}
}

// TestSetDefaultDispatchCapJoin: join variant has the same semantics
// on ContinuationJoinData.
func TestSetDefaultDispatchCapJoin(t *testing.T) {
	rootCap, _ := hash.Compute("test/cap", cbor.RawMessage{0xa0})
	a := types.ContinuationJoinData{Target: "h/a", Operation: "op", Expected: []string{"slot"}}
	b := types.ContinuationJoinData{Target: "h/b", Operation: "op", Expected: []string{"slot"}}
	entitysdk.SetDefaultDispatchCapJoin(rootCap, &a, &b)
	if a.DispatchCapability != rootCap || b.DispatchCapability != rootCap {
		t.Errorf("join cap defaults not applied")
	}
}

// TestContinuationEntity_DeterministicEncoding is the cross-impl
// byte-equality probe: the same ContinuationData inputs must encode
// to byte-identical CBOR every time. This is the necessary condition
// for cross-impl entity content-addressing — if Go's encoder is
// deterministic AND Rust's encoder produces the same canonical
// shape (CBOR canonical encoding per RFC 8949 §4.2.1), then both
// SDKs will emit byte-equal continuation entities for equivalent
// inputs.
//
// Half the cross-impl contract is verified here. The other half
// (Rust-side equivalence) is for the Rust SDK team to verify
// against the hex string this test logs.
//
// Reference fixture: the byte string emitted here is the canonical
// Go encoding for the documented input, recorded for cross-team
// verification.
func TestContinuationEntity_DeterministicEncoding(t *testing.T) {
	// Use known hash values so the fixture is reproducible.
	dispatchCap, _ := hash.Compute("test/dispatch-cap", cbor.RawMessage{0xa0})

	// Build a representative ContinuationData covering the fields
	// commonly used in chain composition.
	cont := types.ContinuationData{
		Target:    "system/revision",
		Operation: "merge",
		Params: cbor.RawMessage([]byte{
			// Minimal CBOR map {prefix: "shared/", strategy: "auto"}.
			0xa2, // map(2)
			0x66, 'p', 'r', 'e', 'f', 'i', 'x',
			0x67, 's', 'h', 'a', 'r', 'e', 'd', '/',
			0x68, 's', 't', 'r', 'a', 't', 'e', 'g', 'y',
			0x64, 'a', 'u', 't', 'o',
		}),
		ResultField:        "source_envelope",
		DispatchCapability: dispatchCap,
	}

	ent1, err := cont.ToEntity()
	if err != nil {
		t.Fatalf("ToEntity: %v", err)
	}
	ent2, err := cont.ToEntity()
	if err != nil {
		t.Fatalf("ToEntity (second): %v", err)
	}

	// Determinism: byte-identical across two encodings of same input.
	if !bytesEqual(ent1.Data, ent2.Data) {
		t.Errorf("non-deterministic encoding: ent1.Data != ent2.Data\n  ent1: %x\n  ent2: %x", ent1.Data, ent2.Data)
	}
	if ent1.ContentHash != ent2.ContentHash {
		t.Errorf("non-deterministic content hash: %s != %s", ent1.ContentHash, ent2.ContentHash)
	}

	// Log fixture for cross-impl verification by Rust SDK team.
	t.Logf("Go canonical encoding of ContinuationData{merge, source_envelope, ...}:")
	t.Logf("  type:         %s", ent1.Type)
	t.Logf("  content_hash: %s", ent1.ContentHash)
	t.Logf("  data (hex):   %s", hex(ent1.Data))
	t.Logf("  data length:  %d bytes", len(ent1.Data))

	// Round-trip: decode the entity back to ContinuationData and
	// verify the fields survive untouched.
	decoded, err := types.ContinuationDataFromEntity(ent1)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Target != cont.Target {
		t.Errorf("Target round-trip: got %q, want %q", decoded.Target, cont.Target)
	}
	if decoded.Operation != cont.Operation {
		t.Errorf("Operation round-trip: got %q, want %q", decoded.Operation, cont.Operation)
	}
	if decoded.ResultField != cont.ResultField {
		t.Errorf("ResultField round-trip: got %q, want %q", decoded.ResultField, cont.ResultField)
	}
	if decoded.DispatchCapability != cont.DispatchCapability {
		t.Errorf("DispatchCapability round-trip mismatch")
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func hex(b []byte) string {
	const digits = "0123456789abcdef"
	var sb strings.Builder
	sb.Grow(len(b) * 2)
	for _, c := range b {
		sb.WriteByte(digits[c>>4])
		sb.WriteByte(digits[c&0xf])
	}
	return sb.String()
}
