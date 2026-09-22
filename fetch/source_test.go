package fetch

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/types"
)

// source_test.go — Tier 1, and deliberately short.
//
// **The gate for the Source extraction is the EXISTING suites**, which
// run against `entity-browser-rust`'s frozen federation and against
// httptest origins, and which were not edited for it: a refactor that
// changes a byte fails them. Verified by breaking the seam on purpose —
// routing `Leaf` at the listing prefix instead of the tree-leaf prefix
// produced six failures including the cross-impl fixture — because *"the
// tests are the net"* is a claim about the tests, and it is worth one
// minute to find out whether the net can catch anything.
//
// What is here instead is the one path the extraction changed the
// SEMANTICS of rather than the shape of, and which nothing covered.

// srcTestPeer is an identity-form peer-id. Its own copy rather than
// crossimpl_test.go's `rustPeer`, which lives in the EXTERNAL test
// package (`package fetch_test`) — this file is in `package fetch`
// because the two things it covers, `errNoManifestPrefix` and
// `acceptSeq`, are unexported on purpose.
const srcTestPeer = "2KEE55MMWBvXE8ozrm45kwTchzFaQP1dzBnU9rifGTdmUa"

// TestHTTPSource_NoManifestPrefixIsARefusalNotANetworkFailure pins the
// distinction the extraction had to preserve across a new boundary.
//
// `manifest_url_prefix` is §6.5.3-reserved and NOT derivable, so a
// publisher advertising none has no signed entry point and there is
// nothing to fall back to. That was one `if` at the top of
// `Consumer.VerifiedRoot` before the seam; it now lives in
// [HTTPSource.Root], on the far side of an error return that
// `VerifiedRoot` wraps with `"fetch manifest: …"`.
//
// **If it were wrapped, a publisher misconfiguration would read as a
// transport failure** — which sends an operator to check their network
// for a condition that no amount of retrying can change, and which is
// the exact class of confusion the error variables in consume.go are
// split up to prevent. So it is a sentinel and `VerifiedRoot` passes it
// through untouched, and this asserts both halves.
func TestHTTPSource_NoManifestPrefixIsARefusalNotANetworkFailure(t *testing.T) {
	layout := Layout{
		Origin: "https://example.test",
		PeerID: srcTestPeer,
		Endpoint: types.TransportEndpoint{
			// Everything EXCEPT the manifest prefix, so the refusal is
			// about the missing field and not about an empty layout.
			TreeURLPrefix:     "https://example.test/tree",
			ContentURLPrefix:  "https://example.test/content",
			ContentLayout:     types.ContentLayoutFlat,
			TreeLeafSuffix:    ".bin",
			TreeListingSuffix: ".list",
		},
	}

	src := NewHTTPSource(layout, nil)
	_, _, err := src.Root(context.Background())
	if !errors.Is(err, errNoManifestPrefix) {
		t.Fatalf("HTTPSource.Root err = %v; want errNoManifestPrefix", err)
	}

	// The half that matters: it survives the layer above unwrapped. A
	// `fmt.Errorf("fetch manifest: %w", …)` here would still satisfy
	// errors.Is, so the assertion is on the TEXT the operator reads.
	_, err = NewConsumerFromSource(src, nil).VerifiedRoot(context.Background())
	if !errors.Is(err, errNoManifestPrefix) {
		t.Fatalf("VerifiedRoot err = %v; want errNoManifestPrefix", err)
	}
	if strings.Contains(err.Error(), "fetch manifest:") {
		t.Errorf("a publisher with no signed entry point was reported as a fetch failure:\n  %v\n"+
			"that sends an operator to check a network for a condition retrying cannot change", err)
	}
	if !strings.Contains(err.Error(), "manifest_url_prefix") {
		t.Errorf("the refusal does not name the missing field: %v", err)
	}

	// Anti-vacuity: the same layout WITH the prefix gets past the
	// refusal and fails on the network instead. Without this arm the
	// test above passes against a Root that refuses unconditionally.
	layout.Endpoint.ManifestURLPrefix = "https://127.0.0.1:1/manifest"
	_, _, err = NewHTTPSource(layout, nil).Root(context.Background())
	if err == nil {
		t.Fatal("a manifest fetch at a dead address succeeded")
	}
	if errors.Is(err, errNoManifestPrefix) {
		t.Errorf("a publisher that DOES advertise a prefix was refused as if it did not: %v", err)
	}
}

// TestConsumerReadsItsPublisherFromTheSource is the small contract the
// verification layer now depends on.
//
// `PeerID` is on the seam rather than inside the transport because it is
// **the key every signature check verifies against** — `publishedroot`
// derives the public key from it — not a routing detail. A Source that
// could not name its publisher would leave `VerifiedRoot` with nothing
// to check the signature against, and the failure would be a verification
// that silently had no subject.
func TestConsumerReadsItsPublisherFromTheSource(t *testing.T) {
	layout := Layout{PeerID: srcTestPeer, Origin: "https://example.test"}
	c := NewConsumer(layout, nil)
	if got := c.PeerID(); got != srcTestPeer {
		t.Errorf("Consumer.PeerID() = %q; want %q", got, srcTestPeer)
	}
}

// TestAcceptSeqIsTheRealFloor calls the function the product calls.
//
// It was written beside `TestConsumer_SeqFloorRefusesARollback`, which
// re-implemented the comparison inside its own body because the floor
// was six lines inline in `VerifiedRoot` and there was nothing else to
// call. **That test is gone**: the floor is now [SeqFloor], so the copy
// had nothing left justifying it, and its slot in cache_test.go is held
// by `TestConsumer_SeqFloorIsPerPublisherNotPerRoad` — the case a copy
// of the rule could never have reached. This one stays as the four-case
// sweep over the real function.
func TestAcceptSeqIsTheRealFloor(t *testing.T) {
	c := NewConsumerFromSource(NewHTTPSource(Layout{PeerID: srcTestPeer}, nil), nil)
	for _, step := range []struct {
		seq      uint64
		wantRoll bool
		why      string
	}{
		{5, false, "the first root sets the floor"},
		{5, false, "a republish of one root is not a rollback"},
		{6, false, "forward is always fine"},
		{3, true, "seq 3 after seq 6 is a replay of a validly-signed root"},
	} {
		err := c.acceptSeq(step.seq, "https://example.test/manifest")
		if got := errors.Is(err, ErrSeqRollback); got != step.wantRoll {
			t.Fatalf("acceptSeq(%d) rollback=%v, want %v — %s (err: %v)",
				step.seq, got, step.wantRoll, step.why, err)
		}
	}
	// The refusal has to be readable: an operator seeing it needs to
	// know both numbers and that both roots were validly signed, or it
	// reads as corruption and they retry.
	err := c.acceptSeq(1, "https://example.test/manifest")
	for _, want := range []string{"seq=1", "seq=6", "validly signed"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("rollback error does not mention %q: %v", want, err)
		}
	}
}
