package entitysdk

import (
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
)

// embed_fallback_test.go — `C-6`: the read side enforces §3's mandatory
// `fallback`, and it tells MISSING from EMPTY.
//
// Two things were wrong and only one of them was on the tracker.
//
//   - The tracker's item: `EmbedData.Validate` refused an absent
//     fallback and **`EmbedNodeFromEntity` never called it**, so the rule
//     held against embeds this tree authored and against nobody else's.
//     *A read side more permissive than the write side does not make a
//     system tolerant — it leaves the write-side check untested against
//     the only inputs it exists to catch.*
//   - Not on the tracker, found while fixing it: the feed reader's
//     non-inline branch renders `fallback` and nothing else, so a
//     missing one produced **a blank row carrying no problem** — which
//     reads as an author who posted nothing.
//
// The split is `entity-browser-rust`'s and we are the side that moved.

// rawEmbed encodes an embed-data map directly, so a test can omit a key
// rather than set it to its zero value.
//
// **This is the whole point of the file.** `EmbedData{Fallback: ""}`
// cannot express *"the key is not on the wire"* — Go has one zero value
// for a string and CBOR has two encodings — so a fixture built from the
// struct can only ever produce the EMPTY case, and the MISSING case
// would stay untestable while looking covered.
func rawEmbed(t *testing.T, withFallback bool, fallback string) []byte {
	t.Helper()
	m := map[string]any{
		"payload": map[string]any{
			"tag":   "inline",
			"bytes": []byte("hello"),
		},
	}
	if withFallback {
		m["fallback"] = fallback
	}
	b, err := ecf.Encode(m)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return b
}

func embedEntity(t *testing.T, withFallback bool, fallback string) entity.Entity {
	t.Helper()
	return entity.Entity{
		Type: EmbedType("text/plain"),
		Data: rawEmbed(t, withFallback, fallback),
	}
}

func TestEmbedFallback_PresenceIsReadFromTheBytesNotTheValue(t *testing.T) {
	if EmbedFallbackPresence(rawEmbed(t, false, "")) {
		t.Error("a map with no `fallback` key reports the key as present")
	}
	if !EmbedFallbackPresence(rawEmbed(t, true, "")) {
		t.Error("a map with `fallback` present and EMPTY reports the key as absent — " +
			"then the two cases are indistinguishable and the split is decorative")
	}
	if !EmbedFallbackPresence(rawEmbed(t, true, "a caption")) {
		t.Error("a map with a real fallback reports the key as absent")
	}
}

func TestEmbedFallback_TheReadSideRefusesBothShapes(t *testing.T) {
	cases := []struct {
		name         string
		withFallback bool
		fallback     string
		wantWord     string
	}{
		{"missing key", false, "", "no `fallback` key"},
		{"present and empty", true, "", "EMPTY `fallback`"},
		{"present and blank", true, "   ", "EMPTY `fallback`"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := EmbedNodeFromEntity(embedEntity(t, tc.withFallback, tc.fallback))
			if err == nil {
				t.Fatal("the read side accepted an embed with no usable fallback")
			}
			if !strings.Contains(err.Error(), tc.wantWord) {
				t.Errorf("refusal does not name which of the two faults occurred\n  want substring: %q\n  got: %v",
					tc.wantWord, err)
			}
		})
	}
}

// TestEmbedFallback_AConformantEmbedStillDecodes is the control arm.
//
// Without it every assertion above is satisfied by a read side that
// refuses everything, which would be a far worse regression than the
// one being fixed and would present identically in a red/green summary.
func TestEmbedFallback_AConformantEmbedStillDecodes(t *testing.T) {
	node, err := EmbedNodeFromEntity(embedEntity(t, true, "a caption"))
	if err != nil {
		t.Fatalf("a conformant embed was refused: %v", err)
	}
	if node.Data.Fallback != "a caption" {
		t.Errorf("fallback round-tripped as %q", node.Data.Fallback)
	}
}

// TestEmbedFallback_NestedPresenceDoesNotAnswerFromTheEnclosure guards
// the trap this fix walked into: a feed entry carries the node at
// `body`, and probing the ENTRY's bytes for a top-level `fallback` finds
// none and reports MISSING for every entry, conformant ones included.
func TestEmbedFallback_NestedPresenceDoesNotAnswerFromTheEnclosure(t *testing.T) {
	enclosing, err := ecf.Encode(map[string]any{
		"author": "somebody",
		"body": map[string]any{
			"type": EmbedType("text/plain"),
			"data": map[string]any{
				"payload":  map[string]any{"tag": "inline", "bytes": []byte("hi")},
				"fallback": "a caption",
			},
		},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	if !EmbedNestedFallbackPresence(enclosing, "body") {
		t.Error("a conformant nested embed reports its fallback as MISSING — " +
			"this is the exact wrong answer the nested form exists to avoid")
	}
	// And the flat probe on the same bytes gets it wrong, which is why
	// the nested form has to exist rather than the caller passing the
	// enclosure to EmbedFallbackPresence.
	if EmbedFallbackPresence(enclosing) {
		t.Error("the flat probe found a top-level `fallback` in an enclosing entity — " +
			"then this test is not demonstrating the hazard it documents")
	}
}
