package entitysdk_test

import (
	refhex "encoding/hex"
	"errors"
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/hash"

	"entity-workbench-go/entitysdk"
)

// reference_test.go — `APP-CONVENTION-REFERENCE` §6.2's eleven required checks,
// one test per vector, named after it.
//
// §6.2's own words: *"the fixtures, the bytes and the run are the
// implementations' and the conformance oracle's … the convention is authored; it
// is validated when these have been exercised."* So this file is the validation
// artifact for our half, and `REF-V3`, `REF-V7` and `REF-V9` are called out
// there as **the three that would not be written by someone implementing from
// the prose alone** — which is why each of them carries its own note below
// rather than being folded into a table-driven sweep.
//
// A mixed-case peer id is used throughout ON PURPOSE (REF-V3). Base58 excludes
// `0`, `O`, `I` and `l`, so this is a plausible id shape and not a synthetic one.
const refPeer = "2KLv2nhwtPrLFd4BZFQuNK1ujtE74q8cVg7y8cYdcZZ5BL"

func refHash(t *testing.T, seed string) hash.Hash {
	t.Helper()
	h, err := hash.OfBytes(hash.AlgorithmSHA256, []byte(seed))
	if err != nil {
		t.Fatalf("OfBytes: %v", err)
	}
	return h
}

// wireHex is what the string form must carry: the FULL wire form including the
// format-code byte, not the digest-only form. Computed here independently of
// the implementation so a change to `refHashParam` fails rather than agreeing
// with itself.
func wireHex(h hash.Hash) string { return refhex.EncodeToString(h.Bytes()) }

// --- REF-V1 / REF-V2 — round-tripping is normative (REF-R8) ---

func TestREF_V1_PinnedReferenceRoundTripsBothWays(t *testing.T) {
	h := refHash(t, "a")
	ref := entitysdk.PinnedRef(refPeer, h)

	uri, err := ref.URI()
	if err != nil {
		t.Fatalf("URI: %v", err)
	}
	want := "entity+ref://" + refPeer + "/?hash=" + wireHex(h)
	if uri != want {
		t.Fatalf("uri:\n got %q\nwant %q", uri, want)
	}

	back, err := entitysdk.ParseRefURI(uri)
	if err != nil {
		t.Fatalf("ParseRefURI: %v", err)
	}
	if !refEqual(back, ref) {
		t.Errorf("atom → string → atom differs:\n got %+v\nwant %+v", back, ref)
	}
	// The other direction of §3.2's MUST: string → atom → string is
	// byte-identical.
	again, err := back.URI()
	if err != nil {
		t.Fatalf("re-serialize: %v", err)
	}
	if again != uri {
		t.Errorf("string → atom → string not byte-identical:\n got %q\nwant %q", again, uri)
	}
}

// REF-V2 exists because "the query component carries three different kinds of
// term and is where they diverge" — an identity expectation (`seen`), advisory
// hints (`via`), and the fragment's anchor all in one string.
func TestREF_V2_LiveReferenceWithSeenViaAndAnchorRoundTrips(t *testing.T) {
	seen := refHash(t, "seen")
	ref := entitysdk.LiveRef(refPeer, "/content/sites/labs/pages/intro", seen).
		WithHints(
			entitysdk.RefHint{Tag: entitysdk.RefHintOrigin, Value: "https://example.test/"},
			entitysdk.RefHint{Tag: entitysdk.RefHintPeer, Value: refPeer},
		).
		WithAnchor("body", "sections")

	uri, err := ref.URI()
	if err != nil {
		t.Fatalf("URI: %v", err)
	}

	// Pinned against the inherited encodings (see reference.go's header): query
	// order is seen-then-via, `via` is `{tag}:{value}` with each half escaped,
	// the anchor is `/`-joined. Written out in full rather than asserted
	// piecewise, because byte-identity with the other seat is the requirement
	// and a piecewise assertion cannot see a reordering.
	want := "entity+ref://" + refPeer + "/content/sites/labs/pages/intro" +
		"?seen=" + wireHex(seen) +
		"&via=origin:https%3A%2F%2Fexample.test%2F" +
		"&via=peer:" + refPeer +
		"#body/sections"
	if uri != want {
		t.Fatalf("uri:\n got %q\nwant %q", uri, want)
	}

	back, err := entitysdk.ParseRefURI(uri)
	if err != nil {
		t.Fatalf("ParseRefURI: %v", err)
	}
	if !refEqual(back, ref) {
		t.Errorf("round trip differs:\n got %+v\nwant %+v", back, ref)
	}
	again, err := back.URI()
	if err != nil {
		t.Fatalf("re-serialize: %v", err)
	}
	if again != uri {
		t.Errorf("not byte-identical on re-serialize:\n got %q\nwant %q", again, uri)
	}
}

// --- REF-V3 — the highest-value vector in the set (REF-R12) ---

// §6.2 calls this "the highest-value vector here", and §3.3 calls the mistake it
// catches "the single most likely implementation error": every general URL
// library lowercases the authority by default, and a peer id that survives such
// a parser NAMES A DIFFERENT PEER. It then fails as a clean 404 at a well-formed
// address, which reads as *not found* rather than as a bug.
//
// This is also why `reference.go` hand-rolls its escaping instead of using
// `net/url` — round-tripping through `url.Parse`/`URL.String()` would lowercase
// the host here.
func TestREF_V3_MixedCasePeerIDSurvivesParseAndReserializeUnchanged(t *testing.T) {
	h := refHash(t, "a")
	uri := "entity+ref://" + refPeer + "/?hash=" + wireHex(h)

	back, err := entitysdk.ParseRefURI(uri)
	if err != nil {
		t.Fatalf("ParseRefURI: %v", err)
	}
	if back.Peer != refPeer {
		t.Fatalf("peer id was normalized:\n got %q\nwant %q", back.Peer, refPeer)
	}
	again, err := back.URI()
	if err != nil {
		t.Fatalf("URI: %v", err)
	}
	if again != uri {
		t.Errorf("re-serialize changed the authority:\n got %q\nwant %q", again, uri)
	}

	// Anti-vacuity: the fixture must actually contain both cases, or a
	// lowercasing implementation would pass this.
	if refPeer == strings.ToLower(refPeer) || refPeer == strings.ToUpper(refPeer) {
		t.Fatal("the fixture peer id is single-case, so this vector proves nothing")
	}

	// REF-R11's other half: the SCHEME is case-insensitive on parse, while the
	// authority beside it is not. Both properties live in one string, which is
	// exactly why they are easy to conflate.
	upper := "ENTITY+REF://" + refPeer + "/?hash=" + wireHex(h)
	fromUpper, err := entitysdk.ParseRefURI(upper)
	if err != nil {
		t.Fatalf("uppercase scheme refused: %v", err)
	}
	if fromUpper.Peer != refPeer {
		t.Errorf("authority normalized on the uppercase-scheme path: %q", fromUpper.Peer)
	}
	emitted, err := fromUpper.URI()
	if err != nil {
		t.Fatalf("URI: %v", err)
	}
	if !strings.HasPrefix(emitted, "entity+ref://") {
		t.Errorf("scheme MUST be emitted lowercase (REF-R11): %q", emitted)
	}
}

// --- REF-V4 / REF-V5 — the discriminator is a rule, not a convention ---

func TestREF_V4_BothHashAndPathIsRefused(t *testing.T) {
	h := refHash(t, "a")
	_, err := entitysdk.ParseRefURI("entity+ref://" + refPeer + "/sites/labs?hash=" + wireHex(h))
	if !errors.Is(err, entitysdk.ErrRefBothIdentityTerms) {
		t.Fatalf("want ErrRefBothIdentityTerms, got %v", err)
	}
}

func TestREF_V5_NeitherHashNorPathIsRefused(t *testing.T) {
	for _, s := range []string{
		"entity+ref://" + refPeer + "/",
		"entity+ref://" + refPeer,
	} {
		if _, err := entitysdk.ParseRefURI(s); !errors.Is(err, entitysdk.ErrRefNoIdentityTerm) {
			t.Errorf("%q: want ErrRefNoIdentityTerm, got %v", s, err)
		}
	}
}

// --- REF-V6 — the most common real content case (REF-R14, REF-R13) ---

// "a page slug with a space or a `#`" — §6.2. The `#` is the sharp one: unescaped
// it would terminate the path and become a fragment, so the round trip would
// produce a well-formed reference to a different thing.
func TestREF_V6_ReservedCharactersInAPathSegmentRoundTripPercentEncoded(t *testing.T) {
	ref := entitysdk.LiveRef(refPeer, "/content/sites/labs/pages/a b#c", hash.Hash{})
	uri, err := ref.URI()
	if err != nil {
		t.Fatalf("URI: %v", err)
	}
	if !strings.Contains(uri, "/a%20b%23c") {
		t.Fatalf("segment not encoded: %q", uri)
	}
	// Uppercase hex escapes, per §6.2.2.1 and matching the other seat's
	// emitter. `%20` and `%23` are digit-only and cannot show this, so the
	// assertion needs a byte whose hex has letters in it — a non-ASCII title
	// character, which is also a real content case.
	accented, err := entitysdk.LiveRef(refPeer, "/pages/caf\u00e9", hash.Hash{}).URI()
	if err != nil {
		t.Fatalf("URI: %v", err)
	}
	if !strings.Contains(accented, "%C3%A9") {
		t.Errorf("escapes must be UPPERCASE hex (§6.2.2.1): %q", accented)
	}
	if strings.Contains(accented, "%c3%a9") {
		t.Errorf("lowercase escapes emitted: %q", accented)
	}
	back, err := entitysdk.ParseRefURI(uri)
	if err != nil {
		t.Fatalf("ParseRefURI: %v", err)
	}
	// REF-R13: the atom holds the DECODED form; comparison is after decoding.
	if back.Path != "/content/sites/labs/pages/a b#c" {
		t.Errorf("path did not decode: %q", back.Path)
	}
	again, err := back.URI()
	if err != nil {
		t.Fatalf("re-serialize: %v", err)
	}
	if again != uri {
		t.Errorf("not byte-identical:\n got %q\nwant %q", again, uri)
	}

	// The `/` delimiter is never encoded when it is one (§3.3), so the segment
	// count survives. Without this a component escaper applied to the whole
	// path would pass every assertion above and collapse the path to one
	// segment.
	if got := strings.Count(back.Path, "/"); got != 5 {
		t.Errorf("segment structure changed: %d slashes in %q", got, back.Path)
	}
}

// --- REF-V7 — droppability measured directly (REF-R5, REF-R7) ---

// One of the three §6.2 says nobody writes from the prose alone. **An unknown
// hint tag must be IGNORED, not refused** — a reader that rejects an atom
// because it carries a hint kind it does not know has made an advisory term
// load-bearing, which is the failure §2.3 exists to prevent.
//
// The subtlety, and the reason the atom KEEPS what resolution ignores: dropping
// the hint at parse would satisfy REF-R7 and break REF-R8's lossless round trip
// in the same move. So this asserts both — the unknown hint survives the round
// trip AND is absent from the actionable set.
func TestREF_V7_UnknownHintTagResolvesIdenticallyAndIsNotDropped(t *testing.T) {
	h := refHash(t, "a")
	plain := entitysdk.PinnedRef(refPeer, h)
	withUnknown := entitysdk.PinnedRef(refPeer, h).
		WithHints(entitysdk.RefHint{Tag: "carrier-pigeon", Value: "coop-3"})

	// REF-R5: the same answer with the hints as without them. The identity term
	// is what resolution turns on, and a hint cannot change it.
	if *plain.Hash != *withUnknown.Hash || plain.Peer != withUnknown.Peer {
		t.Fatal("a via hint changed an identity term")
	}
	if got := entitysdk.RefActionableHints(withUnknown.Via); len(got) != 0 {
		t.Errorf("an unknown hint kind reached the actionable set: %+v", got)
	}

	// REF-R8: and it is still there.
	uri, err := withUnknown.URI()
	if err != nil {
		t.Fatalf("URI: %v", err)
	}
	if !strings.Contains(uri, "via=carrier-pigeon:coop-3") {
		t.Fatalf("unknown hint not emitted: %q", uri)
	}
	back, err := entitysdk.ParseRefURI(uri)
	if err != nil {
		t.Fatalf("an unknown hint kind was REFUSED, which makes an advisory term load-bearing: %v", err)
	}
	if len(back.Via) != 1 || back.Via[0].Tag != "carrier-pigeon" || back.Via[0].Value != "coop-3" {
		t.Errorf("unknown hint was dropped or mangled: %+v", back.Via)
	}
	if back.Via[0].Known() {
		t.Error("carrier-pigeon should not be a known hint kind")
	}
}

// --- REF-V8 / REF-V9 — the refusal, and the arm against over-applying it ---

func TestREF_V8_DotSegmentInAnAbsoluteFormPathIsRefused(t *testing.T) {
	for _, p := range []string{
		"/content/../secrets/key",
		"/content/./pages/intro",
		"/../etc/passwd",
	} {
		uri := "entity+ref://" + refPeer + p
		if _, err := entitysdk.ParseRefURI(uri); !errors.Is(err, entitysdk.ErrRefDotSegment) {
			t.Errorf("%q: want ErrRefDotSegment, got %v", p, err)
		}
		// And the emitter refuses it too, so an atom built in memory cannot
		// project into a string §3.3 forbids.
		ref := entitysdk.EntityRef{Tag: entitysdk.RefTagLive, Peer: refPeer, Path: p}
		if _, err := ref.URI(); !errors.Is(err, entitysdk.ErrRefDotSegment) {
			t.Errorf("%q: emitter allowed a dot segment, got %v", p, err)
		}
	}
}

// The second of the three §6.2 flags. **REF-V9 is the arm that catches
// over-application of REF-V8**, which would reject ordinary correct links:
// §3.4's relative form is directory-relative, so `..` is meaningful and expected
// there, and the refusal is scoped to the ABSOLUTE form only.
//
// Relative resolution is `workbench`'s (`ClassifyTarget` / `resolveInSitePage`,
// byte-identical to the reference implementation by obligation). What is asserted
// here is the boundary this file owns: a relative string is **not** an
// `entity+ref://` reference, so it must come back as a classification — *this is
// leaving the reference grammar* — and never as a dot-segment refusal.
func TestREF_V9_RelativeLinkWithDotDotIsNotRefusedByTheAbsoluteFormRule(t *testing.T) {
	for _, s := range []string{
		"../sibling/page",
		"../../up/two",
		"./here",
		"support.md",
		"/root-absolute/page",
		"site:labs/intro",
	} {
		_, err := entitysdk.ParseRefURI(s)
		if errors.Is(err, entitysdk.ErrRefDotSegment) {
			t.Errorf("%q: an UNRESOLVED relative string was refused for a dot segment — "+
				"REF-R15 is scoped to the absolute form and this rejects ordinary correct links", s)
		}
		if !errors.Is(err, entitysdk.ErrRefNotAReferenceURI) {
			t.Errorf("%q: want the REF-R20 classification (not a reference URI), got %v", s, err)
		}
	}

	// And the resolved product of such a link is accepted: relative resolution
	// CONSUMES the dot segments, so what it produces is an absolute reference in
	// which none survive.
	resolved := entitysdk.LiveRef(refPeer, "/content/sites/labs/pages/sibling/page", hash.Hash{})
	if _, err := resolved.URI(); err != nil {
		t.Errorf("the resolved form of a relative link was refused: %v", err)
	}
}

// --- REF-V10 — the two spellings of an identity (REF-R3) ---

// §2.1.1 exists because the wrong answer is reachable by analogy: `content-hash`
// is a `bstr` and self-describing, a peer id is also self-describing, so the two
// look like the same kind of term. They are not — one is bytes the system
// hashes, the other is an identifier the system SPELLS.
//
// A byte-string peer id has no representation in this Go type at all (`Peer` is
// a `string`), which is the strongest form of the check but also means the test
// has to assert something else: that the WIRE tag is `tstr`, and that the
// canonical Base58 spelling is what lands in it rather than a digest re-hexed.
func TestREF_V10_PeerIDIsTextInCanonicalBase58NotRawDigestBytes(t *testing.T) {
	h := refHash(t, "a")
	ref := entitysdk.PinnedRef(refPeer, h)

	data, err := refEncodeAtom(ref)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	// The Base58 text must appear literally in the encoded atom. A `bstr`
	// encoding of the same identity would not contain it.
	if !strings.Contains(string(data), refPeer) {
		t.Fatalf("the canonical Base58 peer id is not present as text in the encoded atom")
	}
	// And a hash IS bytes, so its hex spelling must NOT appear — the two terms
	// are encoded differently and this is the assertion that says so.
	if strings.Contains(string(data), wireHex(h)) {
		t.Error("the content hash was encoded as hex text; it is a bstr (REF-R4)")
	}

	// REF-R4's other half: no fixed width. The wire form's length is implied by
	// its leading format byte and is never assumed.
	if got := len(h.Bytes()); got != 33 {
		t.Errorf("SHA-256 wire hash: got %d bytes, want 33 (1 format + 32 digest)", got)
	}
	if !strings.HasPrefix(wireHex(h), "00") {
		t.Errorf("wire hex must carry the format code: %q", wireHex(h))
	}
}

// --- REF-V11 — a 404 at a hint proves nothing (REF-R21, REF-R5) ---

// "a reader that gives up at the hinted location makes a hint load-bearing,
// which is the whole failure `via` is bounded to prevent."
//
// There is no resolver in this package, so what is asserted is the property a
// resolver has to preserve and the shape that makes it possible: the identity
// term is intact with the hints removed, and the hint set is a droppable
// suffix. **A `path` hint on a pinned reference is only ever a hint** — the
// bytes validate against the hash from any source at all, so failing at one
// location proves nothing about existence.
func TestREF_V11_APathHintIsDroppableAndTheIdentityTermSurvivesWithoutIt(t *testing.T) {
	h := refHash(t, "a")
	hinted := entitysdk.PinnedRef(refPeer, h).WithHints(
		entitysdk.RefHint{Tag: entitysdk.RefHintPath, Value: "/content/blobs/moved-away"},
		entitysdk.RefHint{Tag: entitysdk.RefHintMirror, Value: "https://second.test/"},
	)

	// Drop every hint — the operation a resolver performs after the hinted
	// location 404s — and the reference is still fully resolvable.
	stripped := hinted
	stripped.Via = nil
	if err := stripped.Validate(); err != nil {
		t.Fatalf("a reference with no hints must be valid: %v", err)
	}
	if *stripped.Hash != h {
		t.Fatal("the identity term did not survive dropping the hints")
	}

	// And there is a second source to try, which is what REF-V11 measures: the
	// hint list is ordered and exhaustion is not proof of absence.
	rest := entitysdk.RefActionableHints(hinted.Via[1:])
	if len(rest) != 1 || rest[0].Tag != entitysdk.RefHintMirror {
		t.Errorf("hints are not an ordered droppable list: %+v", rest)
	}

	// §5's floor, asserted because it is a claim about what a valid participant
	// is: an atom with no `via` and no `at` is the WHOLE floor, and a peer that
	// resolves only pinned references and ignores every hint is a valid
	// participant, not a degraded one.
	floor := entitysdk.PinnedRef(refPeer, h)
	if floor.At != nil || len(floor.Via) != 0 {
		t.Error("the floor atom should carry neither at nor via")
	}
	if err := floor.Validate(); err != nil {
		t.Errorf("the floor is not valid: %v", err)
	}
}

// --- shape discipline beyond the numbered vectors ---

// REF-R1 / REF-R2, plus the schema violations we refuse in step with the other
// seat and keep APART from a malformed decode (their `W-5` third item).
func TestARefWhoseTagAndShapeDisagreeIsRefused(t *testing.T) {
	h := refHash(t, "a")
	cases := []struct {
		name string
		ref  entitysdk.EntityRef
		want error
	}{
		{"untagged", entitysdk.EntityRef{Peer: refPeer, Hash: &h}, entitysdk.ErrRefUnknownTag},
		{"unknown tag", entitysdk.EntityRef{Tag: "snapshot", Peer: refPeer, Hash: &h}, entitysdk.ErrRefUnknownTag},
		{"pin without hash", entitysdk.EntityRef{Tag: entitysdk.RefTagPin, Peer: refPeer}, entitysdk.ErrRefTagMismatch},
		{"live without path", entitysdk.EntityRef{Tag: entitysdk.RefTagLive, Peer: refPeer}, entitysdk.ErrRefTagMismatch},
		{"live with only a root path", entitysdk.EntityRef{Tag: entitysdk.RefTagLive, Peer: refPeer, Path: "/"}, entitysdk.ErrRefTagMismatch},
		{"no peer", entitysdk.EntityRef{Tag: entitysdk.RefTagPin, Hash: &h}, entitysdk.ErrRefMalformed},
		// The schema judgements. A `path` beside a `hash` is the field-name
		// collision §2.3 refuses on principle: one name meaning *the address of
		// record* in one shape and *a guess* in the other is a discriminator a
		// reader would have to know the shape to interpret.
		{"pin carrying path", entitysdk.EntityRef{Tag: entitysdk.RefTagPin, Peer: refPeer, Hash: &h, Path: "/x"}, entitysdk.ErrRefShapeExcludes},
		{"pin carrying seen", entitysdk.EntityRef{Tag: entitysdk.RefTagPin, Peer: refPeer, Hash: &h, Seen: &h}, entitysdk.ErrRefShapeExcludes},
		{"live carrying hash", entitysdk.EntityRef{Tag: entitysdk.RefTagLive, Peer: refPeer, Path: "/x", Hash: &h}, entitysdk.ErrRefShapeExcludes},
	}
	for _, c := range cases {
		if err := c.ref.Validate(); !errors.Is(err, c.want) {
			t.Errorf("%s: want %v, got %v", c.name, c.want, err)
		}
	}
}

// REF-R16: an absent parameter and one present with an empty value are
// different, and the second is malformed.
func TestAnEmptyQueryParameterValueIsRefused(t *testing.T) {
	for _, s := range []string{
		"entity+ref://" + refPeer + "/?hash=",
		"entity+ref://" + refPeer + "/pages/intro?seen=",
		"entity+ref://" + refPeer + "/pages/intro?via=",
	} {
		if _, err := entitysdk.ParseRefURI(s); !errors.Is(err, entitysdk.ErrRefEmptyParam) {
			t.Errorf("%q: want ErrRefEmptyParam, got %v", s, err)
		}
	}
}

// REF-R4 through hash.ParseHex's fail-closed behaviour: the 64-char digest-only
// form is not a content hash, because no allocated format code implies a 32-byte
// wire hash. This is the form a reader is most likely to be handed by mistake.
func TestTheDigestOnlyHexFormIsRefused(t *testing.T) {
	h := refHash(t, "a")
	digestOnly := refhex.EncodeToString(h.Bytes()[1:])
	if len(digestOnly) != 64 {
		t.Fatalf("fixture: expected a 64-char digest, got %d", len(digestOnly))
	}
	_, err := entitysdk.ParseRefURI("entity+ref://" + refPeer + "/?hash=" + digestOnly)
	if !errors.Is(err, entitysdk.ErrRefBadHash) {
		t.Errorf("want ErrRefBadHash for the digest-only form, got %v", err)
	}
}

// REF-R18 / REF-R19: the producer MUST NOT emit `entity://` in a link position;
// a consumer SHOULD keep resolving it and SHOULD say that it did. This asserts
// the predicate a consumer needs in order to say so — without it, "surface that
// you did" has nothing to test.
func TestTheLegacyDispatchSchemeIsRecognisedInALinkPosition(t *testing.T) {
	if !entitysdk.RefIsLegacyDispatchURI("entity://" + refPeer + "/content/x") {
		t.Error("the wire dispatch scheme was not recognised")
	}
	if !entitysdk.RefIsLegacyDispatchURI("ENTITY://" + refPeer + "/content/x") {
		t.Error("scheme matching must be case-insensitive")
	}
	if entitysdk.RefIsLegacyDispatchURI("entity+ref://" + refPeer + "/?hash=00") {
		t.Error("the reference scheme is not the dispatch scheme")
	}
	// Nothing in this package emits `entity://`, which is REF-R18 satisfied by
	// construction. The assertion that can fail is that our own emitter's
	// scheme is the reference one.
	h := refHash(t, "a")
	uri, err := entitysdk.PinnedRef(refPeer, h).URI()
	if err != nil {
		t.Fatalf("URI: %v", err)
	}
	if entitysdk.RefIsLegacyDispatchURI(uri) {
		t.Errorf("our emitter produced a dispatch-scheme string: %q", uri)
	}
}

// A malformed escape is refused rather than passed through, because a tolerant
// decode invents a value and cannot report that it did.
func TestAMalformedPercentEscapeIsRefused(t *testing.T) {
	for _, s := range []string{
		"entity+ref://" + refPeer + "/pages/a%2",
		"entity+ref://" + refPeer + "/pages/a%zz",
		"entity+ref://" + refPeer + "/pages/intro#a%2",
	} {
		if _, err := entitysdk.ParseRefURI(s); !errors.Is(err, entitysdk.ErrRefBadEscape) {
			t.Errorf("%q: want ErrRefBadEscape, got %v", s, err)
		}
	}
}

// An anchor field name containing the `/` delimiter must survive, or a field
// path of one name becomes a field path of two — a different atom that decodes
// without complaint.
func TestAnAnchorFieldNameContainingTheDelimiterRoundTrips(t *testing.T) {
	h := refHash(t, "a")
	ref := entitysdk.PinnedRef(refPeer, h).WithAnchor("odd/name", "second")
	uri, err := ref.URI()
	if err != nil {
		t.Fatalf("URI: %v", err)
	}
	if !strings.Contains(uri, "#odd%2Fname/second") {
		t.Fatalf("anchor not escaped per component: %q", uri)
	}
	back, err := entitysdk.ParseRefURI(uri)
	if err != nil {
		t.Fatalf("ParseRefURI: %v", err)
	}
	if back.At == nil || len(back.At.Field) != 2 ||
		back.At.Field[0] != "odd/name" || back.At.Field[1] != "second" {
		t.Errorf("anchor did not round-trip: %+v", back.At)
	}
}

// A `#` before a `?` makes the `?` part of the fragment (RFC 3986 §3), so the
// fragment MUST be split off before the query. Splitting on `?` first moves an
// anchor's tail into the query, where it decodes cleanly into a different atom
// with nothing to report.
//
// Note this input is NOT in emitted form — our emitter escapes a `?` in an
// anchor field to `%3F`, since `?` is outside the unreserved set. So this
// asserts the PARSE ordering only and makes no REF-R8 claim about a string we
// would never produce.
func TestTheFragmentIsSplitOffBeforeTheQuery(t *testing.T) {
	back, err := entitysdk.ParseRefURI("entity+ref://" + refPeer + "/pages/intro#a?b")
	if err != nil {
		t.Fatalf("ParseRefURI: %v", err)
	}
	if back.Path != "/pages/intro" {
		t.Errorf("path: got %q, want /pages/intro", back.Path)
	}
	if back.At == nil || len(back.At.Field) != 1 || back.At.Field[0] != "a?b" {
		t.Fatalf("fragment mis-split — the query split ran first: %+v", back.At)
	}
	if back.Seen != nil {
		t.Error("nothing in that string is a seen parameter")
	}

	// And the emitted form of the same atom escapes it, which is why the input
	// above is a parse fixture rather than a round-trip one.
	uri, err := back.URI()
	if err != nil {
		t.Fatalf("URI: %v", err)
	}
	if !strings.HasSuffix(uri, "#a%3Fb") {
		t.Errorf("emitter should escape a ? in an anchor field: %q", uri)
	}
	again, err := entitysdk.ParseRefURI(uri)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if !refEqual(again, back) {
		t.Error("the emitted form does not re-parse to the same atom")
	}
}

// --- helpers ---

// refEqual compares two atoms structurally. Written out rather than using
// reflect.DeepEqual because the hash fields are pointers and DeepEqual on them
// compares what they point at correctly but says nothing legible when it fails.
func refEqual(a, b entitysdk.EntityRef) bool {
	if a.Tag != b.Tag || a.Peer != b.Peer || a.Path != b.Path {
		return false
	}
	if !refHashPtrEqual(a.Hash, b.Hash) || !refHashPtrEqual(a.Seen, b.Seen) {
		return false
	}
	if (a.At == nil) != (b.At == nil) {
		return false
	}
	if a.At != nil {
		if len(a.At.Field) != len(b.At.Field) {
			return false
		}
		for i := range a.At.Field {
			if a.At.Field[i] != b.At.Field[i] {
				return false
			}
		}
	}
	if len(a.Via) != len(b.Via) {
		return false
	}
	for i := range a.Via {
		if a.Via[i] != b.Via[i] {
			return false
		}
	}
	return true
}

func refHashPtrEqual(a, b *hash.Hash) bool {
	if (a == nil) != (b == nil) {
		return false
	}
	return a == nil || *a == *b
}

// refEncodeAtom encodes the atom the way a carrying convention would — canonical
// ECF, which is what has to agree across implementations.
func refEncodeAtom(r entitysdk.EntityRef) ([]byte, error) {
	return ecf.Encode(r)
}

// --- cross-impl: THEIR literals, verbatim ---

// The strings below are transcribed from `entity-browser-rust`'s own
// `src/entity_ref.rs` test module, which shipped first. Per the standing rule
// for a shared shape, **a failure here is ROUTED, not locally corrected** — a
// silent local fix converts a cohort disagreement into a divergence with our
// name on it.
//
// Transcribed from (symbol, path) rather than a SHA, at their `dd9d032`:
//
//	a_pinned_reference_round_trips_atom_to_string_to_atom → "{SCHEME}{PEER}/?hash={to_hex}"
//	the_path_delimiter_is_not_encoded                     → "{SCHEME}{PEER}/a/b/c"
//	a_path_segment_with_reserved_characters_…             → contains "/a%20note%20%232"
//	a_string_with_both_identity_terms_or_neither_…        → BothIdentityTerms / NoIdentityTerm
//
// ⚠ **What is NOT pinned on either side until this file existed: the QUERY
// PARAMETER ORDER and the `via` spelling.** Their `REF-V2` asserts a round trip
// and does not assert the literal string, so the `hash`/`seen`-then-`via`
// ordering their emitter implements — which we adopted because they shipped
// first (`W-5`) — is gated by nobody. §3.2's losslessness MUST names no order,
// so a refactor on either side that reorders the query breaks byte-identity
// while every test in both trees stays green. Our `REF-V2` pins the literal;
// theirs should too, and that is in the packet rather than fixed here.
func TestTheirPinnedLiteralsAreOurs(t *testing.T) {
	h := refHash(t, "a")

	pinned, err := entitysdk.PinnedRef(refPeer, h).URI()
	if err != nil {
		t.Fatalf("URI: %v", err)
	}
	if want := "entity+ref://" + refPeer + "/?hash=" + wireHex(h); pinned != want {
		t.Errorf("pinned form diverges from theirs — ROUTE this, do not correct it locally:\n got %q\nwant %q", pinned, want)
	}

	// The delimiter is never encoded. Their arm against an encoder that escapes
	// the whole path as one blob.
	plain, err := entitysdk.LiveRef(refPeer, "/a/b/c", hash.Hash{}).URI()
	if err != nil {
		t.Fatalf("URI: %v", err)
	}
	if want := "entity+ref://" + refPeer + "/a/b/c"; plain != want {
		t.Errorf("delimiter handling diverges:\n got %q\nwant %q", plain, want)
	}

	// Their REF-V6 fixture, character for character.
	slug, err := entitysdk.LiveRef(refPeer, "/sites/labs/pages/a note #2", hash.Hash{}).URI()
	if err != nil {
		t.Fatalf("URI: %v", err)
	}
	if !strings.Contains(slug, "/a%20note%20%232") {
		t.Errorf("their REF-V6 fixture does not encode the same way: %q", slug)
	}
	// And their assertion beside it: a `#` inside a segment is not a fragment.
	if strings.Contains(slug, "#") {
		t.Errorf("an unescaped # made the segment tail a fragment: %q", slug)
	}
}
