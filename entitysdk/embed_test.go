package entitysdk_test

// Unit gates for APP-CONVENTION-EMBED §3, the input surface. Tier: unit
// (TESTING-STRATEGY).
//
// These are OURS against OURS and cannot establish cross-impl agreement —
// `feed_crossimpl_test.go` is the only gate here that can, because it drives
// these types from another seat's authored input and compares their bytes.
// What this file covers is the set of cases §9 names as *"what an
// implementation must discriminate"* for the input surface: one vector per
// payload tag, the untagged reject, the mandatory fallback, and the
// passive-only refusal.
//
// §9's OUTPUT-surface cases (the five `EmbedOutput` kinds, `raw`-dropped-clean,
// unknown-`layout`, unknown-`kind`, rendition selection and decline) are NOT
// here, because §4 is not built — see `embed.go`'s header for why that is a
// decision rather than a gap.

import (
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/hash"

	"entity-workbench-go/entitysdk"
)

func embedTestHash(t *testing.T) hash.Hash {
	t.Helper()
	h, err := hash.ParseHex("00b10b000000000000000000000000000000000000000000000000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// §9: "an Embed of each payload tag (inline / pointer / child) with a mandatory
// fallback". One vector each, and each ROUND TRIPS — the re-encode is its own
// anti-vacuity arm, because `ecf.Decode` into a Go struct drops an undeclared
// field in silence (AP49's shape) and a dropped field comes back as a byte
// difference rather than as nothing at all.
func TestEmbedPayloadArmsRoundTrip(t *testing.T) {
	ref := entitysdk.PinnedRef("2K42FX8pASWDrXaAVsGXMNJbAkCVBuVwXf4RuwFNnmyYis", embedTestHash(t))

	cases := []struct {
		name    string
		media   string
		payload entitysdk.EmbedPayload
	}{
		{"inline", "text/plain", entitysdk.InlinePayload([]byte("hello"))},
		{"pointer", "image/png", entitysdk.PointerPayload(embedTestHash(t))},
		{"child", "text/markdown", entitysdk.ChildPayload(ref)},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			node := entitysdk.NewEmbedNode(c.media, c.payload, "the authored fallback")
			ent, err := node.ToEntity()
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			if ent.Type != "app/embed/"+c.media {
				t.Errorf("type: got %q — §3 makes the TYPE TAG the dispatch key", ent.Type)
			}

			back, err := entitysdk.EmbedNodeFromEntity(ent)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if back.Data.Payload.Tag != c.payload.Tag {
				t.Errorf("payload tag: got %q, want %q", back.Data.Payload.Tag, c.payload.Tag)
			}
			// §3.1's MUST: a node and an entity differ in ADDRESSING only, so
			// the round trip through the addressed form must change nothing.
			re, err := back.ToEntity()
			if err != nil {
				t.Fatalf("re-encode: %v", err)
			}
			if re.ContentHash != ent.ContentHash {
				t.Errorf("round trip changed the bytes: %s -> %s", ent.ContentHash, re.ContentHash)
			}
		})
	}
}

// §9: "a payload-tag-ambiguity reject vector (untagged payload MUST be
// rejected — G-PIN-2)", plus the two-arms-at-once case the tagged union exists
// to make unrepresentable.
func TestEmbedRefusesAnUntaggedOrDoubleArmedPayload(t *testing.T) {
	t.Run("untagged", func(t *testing.T) {
		n := entitysdk.EmbedNode{
			Type: "app/embed/text/plain",
			Data: entitysdk.EmbedData{
				Payload:  entitysdk.EmbedPayload{Bytes: []byte("hi")}, // no tag
				Fallback: "x",
			},
		}
		err := n.Validate()
		if err == nil {
			t.Fatal("an untagged payload was accepted; G-PIN-2 requires it be rejected")
		}
		if !strings.Contains(err.Error(), "TAGGED") {
			t.Errorf("the refusal should name the rule: %v", err)
		}
	})

	t.Run("two arms", func(t *testing.T) {
		h := embedTestHash(t)
		n := entitysdk.EmbedNode{
			Type: "app/embed/text/plain",
			Data: entitysdk.EmbedData{
				Payload:  entitysdk.EmbedPayload{Tag: "inline", Bytes: []byte("hi"), Hash: &h},
				Fallback: "x",
			},
		}
		if err := n.Validate(); err == nil {
			t.Fatal("a payload carrying both bytes and a hash was accepted; the union has exactly one arm")
		}
	})
}

// §3, §6, §8 clause 6: `fallback` is MANDATORY and non-empty. It is the rung of
// the degradation ladder that is always available, so an embed without one can
// become invisible on any substrate lacking its handler — which is the
// anti-graveyard contract's whole subject.
func TestEmbedRefusesAMissingFallback(t *testing.T) {
	for _, fb := range []string{"", "   ", "\t\n"} {
		n := entitysdk.NewEmbedNode("text/plain", entitysdk.InlinePayload([]byte("hi")), fb)
		if err := n.Validate(); err == nil {
			t.Errorf("fallback %q was accepted; §3 makes it mandatory and NON-EMPTY", fb)
		}
	}
	// Anti-vacuity: the same node with a fallback must pass, or the assertion
	// above is satisfied by everything failing.
	ok := entitysdk.NewEmbedNode("text/plain", entitysdk.InlinePayload([]byte("hi")), "a fallback")
	if err := ok.Validate(); err != nil {
		t.Fatalf("a conformant embed was refused: %v", err)
	}
}

// §3's `.size (1..16384)`. BOTH bounds: a zero-byte inline payload carries
// nothing while claiming to carry the content, which is the one state
// `fallback` cannot cover.
func TestEmbedInlineCeilingAndFloor(t *testing.T) {
	mk := func(n int) error {
		return entitysdk.NewEmbedNode("image/svg+xml",
			entitysdk.InlinePayload(make([]byte, n)), "fallback").Validate()
	}
	if err := mk(0); err == nil {
		t.Error("a zero-byte inline payload was accepted; §3 pins `.size (1..16384)`")
	}
	if err := mk(entitysdk.EmbedInlineMaxBytes); err != nil {
		t.Errorf("exactly the ceiling was refused: %v", err)
	}
	if err := mk(entitysdk.EmbedInlineMaxBytes + 1); err == nil {
		t.Error("one byte over the ceiling was accepted; above it the payload must be a pointer")
	}
}

// §9: "a non-empty-requires/sandbox refuse vector (v0.2 consumer refuses to
// render — §3)".
//
// THE DISTINCTION THIS GATE EXISTS FOR: the refusal is on RENDERING, never on
// decoding. §3 requires decoders to tolerate unknown keys, so the entity is
// well-formed and must be carried and handed on; it is drawing it that is
// forbidden, because a partial-honour implementation renders with
// declared-but-unenforced capabilities and a reader cannot detect that state.
func TestEmbedRefusesToRenderAnActiveEmbedButStillDecodesIt(t *testing.T) {
	rawReq, err := ecf.Encode(map[string]interface{}{"handler": "something"})
	if err != nil {
		t.Fatal(err)
	}
	n := entitysdk.NewEmbedNode("text/plain", entitysdk.InlinePayload([]byte("hi")), "fallback text")
	n.Data.Requires = append(n.Data.Requires, rawReq)

	// It is VALID — that is the half that is easy to get backwards.
	if err := n.Validate(); err != nil {
		t.Fatalf("an embed declaring capabilities is a well-formed entity and must decode: %v", err)
	}
	refuses, why := n.Data.RefusesToRender()
	if !refuses {
		t.Fatal("a v0.2 consumer MUST refuse to RENDER an embed carrying a non-empty `requires` (§3, §7)")
	}
	if !strings.Contains(why, "fallback") {
		t.Errorf("the refusal should name the conformant response — show the authored fallback: %q", why)
	}

	// And it round-trips: a re-serializer that dropped `requires` would be
	// rewriting somebody else's entity, which §3's forward-compat rule forbids.
	ent, err := n.ToEntity()
	if err != nil {
		t.Fatal(err)
	}
	back, err := entitysdk.EmbedNodeFromEntity(ent)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Data.Requires) != 1 {
		t.Fatalf("`requires` did not survive the round trip: %d elements", len(back.Data.Requires))
	}
	re, err := back.ToEntity()
	if err != nil {
		t.Fatal(err)
	}
	if re.ContentHash != ent.ContentHash {
		t.Error("round trip changed the bytes of an embed carrying capability declarations")
	}

	// Anti-vacuity: the same embed WITHOUT the declaration must render.
	plain := entitysdk.NewEmbedNode("text/plain", entitysdk.InlinePayload([]byte("hi")), "fallback text")
	if refuses, _ := plain.Data.RefusesToRender(); refuses {
		t.Error("a passive embed was refused; v0.2's floor is passive embeds rendering")
	}
	// The sandbox arm, separately — it is a different field and a different
	// `case` in the predicate.
	sb := plain
	sb.Data.Sandbox = &entitysdk.EmbedSandbox{}
	if refuses, _ := sb.Data.RefusesToRender(); !refuses {
		t.Error("an embed declaring a sandbox constraint must also be refused for rendering")
	}
}

// A type tag that is not an embed at all, and one with no media type after the
// prefix. The second is the interesting one: `app/embed/` alone would make the
// dispatch key empty, and §3's whole point is that the media type lives there.
func TestEmbedTypeTagIsTheDispatchKey(t *testing.T) {
	if _, ok := entitysdk.EmbedMediaType("app/feed/entry"); ok {
		t.Error("a non-embed type was read as an embed")
	}
	if _, ok := entitysdk.EmbedMediaType("app/embed/"); ok {
		t.Error("`app/embed/` with no media type was accepted; the media type IS the dispatch key")
	}
	mt, ok := entitysdk.EmbedMediaType("app/embed/image/png")
	if !ok || mt != "image/png" {
		t.Errorf("media type: got %q ok=%v", mt, ok)
	}
	n := entitysdk.EmbedNode{
		Type: "app/embed/",
		Data: entitysdk.EmbedData{Payload: entitysdk.InlinePayload([]byte("x")), Fallback: "f"},
	}
	if err := n.Validate(); err == nil {
		t.Error("a node with an empty media type validated")
	}
}
