// Embed — APP-CONVENTION-EMBED v0.2.3 §3, the INPUT surface.
//
// This file builds §3 and only §3: the `Embed` entity, the `embed-node` inline
// form, `embed-data`, and the three-arm tagged payload union. It deliberately
// does NOT build §4's `EmbedOutput`.
//
// **Why the output surface is absent, stated so the gap is a decision and not
// an oversight.** §4 is the *handler's* output — the thing a renderer draws —
// and §3's own warning is that a consuming convention which stores output
// "publishes the wrong half of the model": the rendition choice would be fixed
// at authoring time for every reader forever, §5's `{media_type → handler}`
// dispatch would have nothing left to run, and §6's ladder nothing left to
// degrade. An entry stores what was AUTHORED and the handler runs at the
// READER. So the input surface is what a producer needs, and an output
// vocabulary with no renderer behind it would be a model with no shipped
// surface (D23) carrying a closed enum we would then have to keep.
//
// **What this file is for.** `APP-CONVENTION-FEED` §2.3 types an entry's `body`
// as an `embed-node`, so nothing in `feed.go` can be written until this exists.
// That ordering is forced, not chosen: reference → embed → feed, because EMBED's
// `child` arm carries an `entity-ref` and FEED's `body` carries an embed.
//
// **v0.2 is PASSIVE-ONLY and that is enforced here, not documented here.**
// §3's last normative note makes a consumer refuse to RENDER any embed carrying
// a non-empty `requires` or `sandbox`; RefusesToRender is that predicate. The
// fields are still carried losslessly, because §3 also says decoders MUST
// tolerate unknown keys and a re-serializer that dropped them would rewrite
// somebody else's entity.
package entitysdk

import (
	"fmt"
	"strings"

	"github.com/fxamacker/cbor/v2"
	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
)

// EmbedTypePrefix is §3's dispatch key prefix. **The media type lives in the
// TYPE TAG and there is no `data.media_type` field** — it would be a redundant
// second source of truth, dropped per the convention's cross-team S-4.
const EmbedTypePrefix = "app/embed/"

// Payload tags (§3). TAGGED: a decoder MUST reject an untagged or ambiguous
// payload, which is the silent-divergence risk G-PIN-2 closes.
const (
	// EmbedPayloadInline carries the bytes in the entity. Icons and SVG only.
	EmbedPayloadInline = "inline"
	// EmbedPayloadPointer names a blob in the RESOLVING PEER'S OWN content
	// store. Same-peer by design — the implied-authority form of REFERENCE
	// §2.1's pinned atom (§3.4), the same rung `site:` occupies.
	EmbedPayloadPointer = "pointer"
	// EmbedPayloadChild is entity-native transclusion of a sibling Embed, and
	// it is **the only payload that may cross a peer boundary**, which is why
	// it is the only one carrying the authority term.
	EmbedPayloadChild = "child"
)

// EmbedInlineMaxBytes is §3's `.size (1..16384)` ceiling on an inline payload.
//
// **It is a property of the payload, not of the addressing**, so it applies
// unchanged to an inline node carried inside another entity's field. Above it,
// use a pointer: inline bytes inflate trie nodes and `.list` outputs, and this
// repo's own >10 MB inline-failure history is cited in the convention as the
// reason the ceiling exists (it supersedes the site convention's earlier
// 256 KiB; the 16–256 KiB range is a single-chunk pointer).
const EmbedInlineMaxBytes = 16384

// EmbedType builds the dispatch key for a media type.
func EmbedType(mediaType string) string { return EmbedTypePrefix + mediaType }

// EmbedMediaType recovers the media type from a dispatch key. The second
// result is false for a type tag that is not an embed at all.
func EmbedMediaType(typeTag string) (string, bool) {
	if !strings.HasPrefix(typeTag, EmbedTypePrefix) {
		return "", false
	}
	mt := strings.TrimPrefix(typeTag, EmbedTypePrefix)
	if mt == "" {
		return "", false
	}
	return mt, true
}

// EmbedPayload is §3's tagged union — the content itself.
//
// One struct with `omitempty` on the arm-specific fields encodes
// byte-identically to any of the three arms, exactly as EntityRef does for
// REFERENCE §2.1's two. Validate is what enforces that one arm and only one arm
// is inhabited: the discipline is a check because the type system cannot carry
// it, and a check that is never called is not a discipline.
//
// **Never infer the arm from which field is populated.** The tag is the
// discriminator, so a malformed payload fails on its own terms rather than only
// at a reader that happens to implement the presence rule.
type EmbedPayload struct {
	Tag   string     `cbor:"tag"`
	Bytes []byte     `cbor:"bytes,omitempty"` // inline
	Hash  *hash.Hash `cbor:"hash,omitempty"`  // pointer
	Ref   *EntityRef `cbor:"ref,omitempty"`   // child
}

// InlinePayload carries bytes in the entity. See EmbedInlineMaxBytes.
func InlinePayload(b []byte) EmbedPayload {
	return EmbedPayload{Tag: EmbedPayloadInline, Bytes: b}
}

// PointerPayload names a blob in the resolving peer's own content store.
func PointerPayload(h hash.Hash) EmbedPayload {
	return EmbedPayload{Tag: EmbedPayloadPointer, Hash: &h}
}

// ChildPayload transcludes a sibling Embed entity. The reference is an
// `entity-ref` and may name another peer — the one payload that can.
func ChildPayload(ref EntityRef) EmbedPayload {
	return EmbedPayload{Tag: EmbedPayloadChild, Ref: &ref}
}

// Validate enforces the tagged-union discipline and the inline ceiling.
func (p EmbedPayload) Validate() error {
	extra := func(name string, present bool) error {
		if !present {
			return nil
		}
		return NewError(400, "invalid_embed_payload",
			fmt.Sprintf("%s-payload also carries %s; the payload is a tagged union with exactly one arm "+
				"(APP-CONVENTION-EMBED §3)", p.Tag, name))
	}
	switch p.Tag {
	case EmbedPayloadInline:
		// `.size (1..16384)` — the LOWER bound is normative too. A zero-byte
		// inline payload is a payload that carries nothing while claiming to
		// carry the content, which is the one state `fallback` cannot cover.
		if len(p.Bytes) == 0 {
			return NewError(400, "invalid_embed_payload",
				"inline-payload carries no bytes; §3 pins `.size (1..16384)` (APP-CONVENTION-EMBED §3)")
		}
		if len(p.Bytes) > EmbedInlineMaxBytes {
			return NewError(400, "invalid_embed_payload",
				fmt.Sprintf("inline-payload carries %d bytes, over §3's %d-byte ceiling; use a pointer-payload "+
					"(inline bytes inflate trie nodes and .list output)", len(p.Bytes), EmbedInlineMaxBytes))
		}
		if err := extra("a hash", p.Hash != nil); err != nil {
			return err
		}
		return extra("a ref", p.Ref != nil)
	case EmbedPayloadPointer:
		if p.Hash == nil || p.Hash.IsZero() {
			return NewError(400, "invalid_embed_payload",
				"pointer-payload carries no hash (APP-CONVENTION-EMBED §3)")
		}
		if err := extra("bytes", len(p.Bytes) > 0); err != nil {
			return err
		}
		return extra("a ref", p.Ref != nil)
	case EmbedPayloadChild:
		if p.Ref == nil {
			return NewError(400, "invalid_embed_payload",
				"child-payload carries no ref (APP-CONVENTION-EMBED §3)")
		}
		if err := p.Ref.Validate(); err != nil {
			return fmt.Errorf("child-payload ref: %w", err)
		}
		if err := extra("bytes", len(p.Bytes) > 0); err != nil {
			return err
		}
		return extra("a hash", p.Hash != nil)
	case "":
		// G-PIN-2, and the reason it is its own arm: an untagged payload is
		// the shape that would otherwise be guessed at from field presence by
		// each implementation independently.
		return NewError(400, "invalid_embed_payload",
			"payload carries no tag; §3's union is TAGGED and an untagged payload MUST be rejected "+
				"(APP-CONVENTION-EMBED §3, G-PIN-2)")
	default:
		return NewError(400, "invalid_embed_payload",
			fmt.Sprintf("payload tag %q is none of %q, %q, %q (APP-CONVENTION-EMBED §3)",
				p.Tag, EmbedPayloadInline, EmbedPayloadPointer, EmbedPayloadChild))
	}
}

// EmbedRendition is one responsive/format variant (§5.3), lazily dereferenced.
type EmbedRendition struct {
	Pointer        hash.Hash `cbor:"pointer"`
	MediaType      string    `cbor:"media_type"`
	CapabilityTags []string  `cbor:"capability_tags"`
}

// EmbedSandbox is §3's substrate-NEUTRAL containment declaration. Deliberately
// carries no `iframe` / `wasm` literals — naming a substrate here is what the
// convention's S-3 removed.
//
// v0.2 consumers refuse to RENDER an embed carrying one; see RefusesToRender.
type EmbedSandbox struct {
	ReadOnlyWithin       string `cbor:"read_only_within,omitempty"`
	DenyExternalHandlers *bool  `cbor:"deny_external_handlers,omitempty"`
}

// EmbedData is §3's `embed-data`.
//
// `Requires` is `[]cbor.RawMessage` ON PURPOSE. §3's CDDL references
// `capability-decl` and **the document never defines it**; inventing a struct
// for it would be minting a shape whose referent exists in no document (AP20).
// Carrying the raw elements satisfies the two obligations that DO exist — the
// forward-compatibility rule ("decoders MUST tolerate unknown keys") and the
// lossless round trip — while leaving the shape to whoever defines it.
type EmbedData struct {
	Payload    EmbedPayload           `cbor:"payload"`
	Fallback   string                 `cbor:"fallback"`
	Params     map[string]interface{} `cbor:"params,omitempty"`
	Renditions []EmbedRendition       `cbor:"renditions,omitempty"`
	Requires   []cbor.RawMessage      `cbor:"requires,omitempty"`
	Sandbox    *EmbedSandbox          `cbor:"sandbox,omitempty"`
}

// Validate enforces §3's shape.
//
// **`fallback` is mandatory and non-empty**, and it is the anti-graveyard
// contract's sixth clause (§8) rather than a nicety: it is the one rung of §6's
// ladder that is always available, so an embed without one is an embed that can
// become invisible on any substrate that lacks its handler.
func (d EmbedData) Validate() error { return d.validate(EmbedFallbackStateFor(d.Fallback, true)) }

// EmbedFallbackState is §3's `fallback` rule split into the two ways it
// is broken, because they name different faults by different parties.
//
// **MISSING** is a producer that has not implemented §3 — the key is not
// on the wire at all, so nothing there was trying to be a fallback.
// **EMPTY** is a producer that implemented it and shipped an empty
// string, which is a bug in one authoring path rather than an absent
// feature. An operator who meets the first should go and read the other
// implementation's emitter; one who meets the second should report a
// single broken entry.
//
// C-6: `entity-browser-rust` splits these and we did not — theirs is the
// absent-vs-withheld principle at the smallest available scale, which is
// the same distinction `fetch.ErrEmptyEnumeration` draws about a whole
// signed root and `RefNotCommitted` draws about one key.
type EmbedFallbackState int

const (
	// EmbedFallbackPresent is the conformant case.
	EmbedFallbackPresent EmbedFallbackState = iota
	// EmbedFallbackMissing means the key was absent from the encoded map.
	EmbedFallbackMissing
	// EmbedFallbackEmpty means the key was present and blank.
	EmbedFallbackEmpty
)

// EmbedFallbackStateFor classifies a fallback given whether its key was
// present on the wire.
//
// `present` is a parameter rather than something derived from the string
// because **a decoded Go string cannot answer it**: `ecf.Decode` yields
// `""` for both an absent key and an empty one, which is exactly why
// this distinction has to be taken from the raw bytes and cannot be
// recovered afterwards. [EmbedFallbackPresence] is what reads it.
func EmbedFallbackStateFor(fallback string, present bool) EmbedFallbackState {
	switch {
	case !present:
		return EmbedFallbackMissing
	case strings.TrimSpace(fallback) == "":
		return EmbedFallbackEmpty
	default:
		return EmbedFallbackPresent
	}
}

// EmbedFallbackPresence reports whether an encoded `embed-data` carries
// a `fallback` key at all, independent of its value.
//
// Best-effort by construction: a body that does not decode as a map has
// bigger problems, and this reports `true` for it so the caller's real
// decode error is the one that surfaces rather than being pre-empted by
// a misleading "no fallback".
func EmbedFallbackPresence(raw []byte) bool {
	if len(raw) == 0 {
		return true
	}
	var probe struct {
		Fallback *string `cbor:"fallback"`
	}
	if err := ecf.Decode(raw, &probe); err != nil {
		return true
	}
	return probe.Fallback != nil
}

// ValidateDecoded is [EmbedData.Validate] for the READ side, where the
// raw bytes are still in hand and the two failures can be told apart.
//
// `raw` is the encoded `embed-data` map — the same bytes this value was
// decoded from, NOT an enclosing entity. See
// [EmbedNestedFallbackPresence] for a node carried as a field of
// something else.
func (d EmbedData) ValidateDecoded(raw []byte) error {
	return d.validate(EmbedFallbackStateFor(d.Fallback, EmbedFallbackPresence(raw)))
}

// EmbedNestedFallbackPresence answers [EmbedFallbackPresence] for an
// embed node carried as a named field of an enclosing entity — a feed
// entry's `body`, most importantly.
//
// It exists because the presence question **cannot be asked of a decoded
// value**, and a caller holding an enclosing entity has raw bytes for
// the enclosure and none for the nested node. Passing the enclosure's
// bytes to [EmbedFallbackPresence] is not a near-miss: it looks for
// `fallback` at the top level, does not find it, and reports MISSING for
// every entry including conformant ones — a confidently wrong answer on
// a surface whose whole job is to say which of two faults occurred.
func EmbedNestedFallbackPresence(enclosing []byte, field string) bool {
	if len(enclosing) == 0 {
		return true
	}
	var probe map[string]struct {
		Data struct {
			Fallback *string `cbor:"fallback"`
		} `cbor:"data"`
	}
	if err := ecf.Decode(enclosing, &probe); err != nil {
		return true
	}
	node, ok := probe[field]
	if !ok {
		return true
	}
	return node.Data.Fallback != nil
}

// ValidateDecodedNested is [EmbedData.ValidateDecoded] for a node
// carried at `field` of `enclosing`.
func (d EmbedData) ValidateDecodedNested(enclosing []byte, field string) error {
	return d.validate(EmbedFallbackStateFor(d.Fallback, EmbedNestedFallbackPresence(enclosing, field)))
}

func (d EmbedData) validate(state EmbedFallbackState) error {
	if err := d.Payload.Validate(); err != nil {
		return err
	}
	switch state {
	case EmbedFallbackMissing:
		return NewError(400, "invalid_embed",
			"embed carries no `fallback` key; it is MANDATORY and non-empty (APP-CONVENTION-EMBED §3, §6, "+
				"§8 clause 6) — it is the rung of the degradation ladder that is always available, and a "+
				"producer omitting the key entirely has not implemented §3 rather than filled it in badly")
	case EmbedFallbackEmpty:
		return NewError(400, "invalid_embed",
			"embed carries an EMPTY `fallback`; the key is present, so §3 is implemented and one authoring "+
				"path filled it with nothing — it is MANDATORY and non-empty (APP-CONVENTION-EMBED §3, §6, "+
				"§8 clause 6), and an embed without one becomes invisible on any substrate lacking its handler")
	}
	for k := range d.Params {
		if k == "" {
			return NewError(400, "invalid_embed",
				"embed params carries an empty key; §3 makes params keys STRINGS ONLY and a key names an attribute")
		}
	}
	for i, r := range d.Renditions {
		if r.Pointer.IsZero() {
			return NewError(400, "invalid_embed",
				fmt.Sprintf("rendition %d carries no pointer (APP-CONVENTION-EMBED §5.3)", i))
		}
		if r.MediaType == "" {
			return NewError(400, "invalid_embed",
				fmt.Sprintf("rendition %d carries no media_type; it is what selection reads (§5.3)", i))
		}
	}
	return nil
}

// RefusesToRender reports whether a v0.2 consumer must refuse to render this
// embed, and why.
//
// **This is a RENDER refusal, never a decode refusal**, and the distinction is
// the whole point: §3 requires decoders to tolerate unknown keys, so an embed
// declaring capabilities is a well-formed entity we must carry and hand on. It
// is *drawing* it that is forbidden, because a partial-honour implementation
// would render with declared-but-unenforced capabilities — which is the state
// a reader cannot detect and the one §7's gate G1 exists to keep from shipping.
//
// This convention names workbench-go by name in that note. The caller's correct
// response is §6's ladder: show the authored `fallback`.
func (d EmbedData) RefusesToRender() (bool, string) {
	switch {
	case len(d.Requires) > 0:
		return true, "this embed declares capability requirements (`requires`), and active embeds are deferred " +
			"behind gate G1 — a v0.2 consumer MUST refuse to render it rather than draw it with declared-but-" +
			"unenforced capabilities (APP-CONVENTION-EMBED §3, §7). Showing the authored fallback is the " +
			"conformant response"
	case d.Sandbox != nil:
		return true, "this embed declares a sandbox constraint, and active embeds are deferred behind gate G1 — " +
			"a v0.2 consumer MUST refuse to render it (APP-CONVENTION-EMBED §3, §7). Showing the authored " +
			"fallback is the conformant response"
	}
	return false, ""
}

// EmbedNode is §3's inline form — the SAME `(type, data)` pair, carried inside
// another entity's field rather than addressed separately.
//
// **A node and an entity differ in ADDRESSING ONLY** (§3.1), and the convention
// makes that a MUST: a handler that accepts one accepts the other, and an
// implementation MUST NOT make behaviour depend on which form an embed arrived
// in. That is why `ToEntity` and `Node` below are two views of one value rather
// than two types with two code paths.
//
// Inline is correct where the embed IS the field's value and is never
// referenced independently — `APP-CONVENTION-FEED` §2.3's `body` is the
// reference case, and it is why this type exists here.
type EmbedNode struct {
	Type string    `cbor:"type"`
	Data EmbedData `cbor:"data"`
}

// NewEmbedNode builds an inline node for a media type.
func NewEmbedNode(mediaType string, payload EmbedPayload, fallback string) EmbedNode {
	return EmbedNode{
		Type: EmbedType(mediaType),
		Data: EmbedData{Payload: payload, Fallback: fallback},
	}
}

// WithParams returns a copy carrying the open attribute bag.
//
// Values are ECF values (canonical CBOR, V7 §1.3): a Go string encodes as a
// text value and a Go unsigned integer as an unsigned integer, and the two are
// different bytes. An implementation that stringifies numbers on the way in is
// caught by a hash rather than by review — the joint fixture carries one of
// each for exactly that reason.
func (n EmbedNode) WithParams(params map[string]interface{}) EmbedNode {
	n.Data.Params = params
	return n
}

// WithRenditions returns a copy carrying §5.3's variants.
func (n EmbedNode) WithRenditions(r ...EmbedRendition) EmbedNode {
	n.Data.Renditions = append(append([]EmbedRendition(nil), n.Data.Renditions...), r...)
	return n
}

// Validate enforces the node's own shape plus §3's data rules.
func (n EmbedNode) Validate() error {
	mt, ok := EmbedMediaType(n.Type)
	if !ok {
		return NewError(400, "invalid_embed",
			fmt.Sprintf("embed type %q is not %q + a media type; §3 makes the TYPE TAG the dispatch key and "+
				"there is no data.media_type field", n.Type, EmbedTypePrefix))
	}
	if strings.ContainsAny(mt, " \t") {
		return NewError(400, "invalid_embed",
			fmt.Sprintf("embed media type %q carries whitespace", mt))
	}
	return n.Data.Validate()
}

// MediaType returns the dispatch key's media type.
func (n EmbedNode) MediaType() string {
	mt, _ := EmbedMediaType(n.Type)
	return mt
}

// ToEntity addresses this node as a standalone `Embed` entity. §3.1: the two
// forms differ in addressing only, so this is a view and not a conversion.
func (n EmbedNode) ToEntity() (entity.Entity, error) {
	if err := n.Validate(); err != nil {
		return entity.Entity{}, err
	}
	return encodeAsEntity(n.Type, n.Data)
}

// EmbedNodeFromEntity is the other direction — read an addressed `Embed` as the
// inline node it is equivalent to.
func EmbedNodeFromEntity(e entity.Entity) (EmbedNode, error) {
	if _, ok := EmbedMediaType(e.Type); !ok {
		return EmbedNode{}, NewError(400, "invalid_embed",
			fmt.Sprintf("entity type %q is not an embed", e.Type))
	}
	var d EmbedData
	if err := ecf.Decode(e.Data, &d); err != nil {
		return EmbedNode{}, err
	}
	// C-6. Until 2026-09-15 this returned here: `ToEntity` refused an
	// embed with no fallback and this accepted one, so the rule held
	// against embeds WE authored and against nobody else's. A read side
	// that is more permissive than the write side does not make the
	// system tolerant — it makes the write-side check untested against
	// the only inputs it exists to catch.
	if err := d.ValidateDecoded(e.Data); err != nil {
		return EmbedNode{}, err
	}
	return EmbedNode{Type: e.Type, Data: d}, nil
}
