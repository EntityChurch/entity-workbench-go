package fetch_test

// DX byte-preservation probe — `SYSTEM-DATA-EXCHANGE` v0.2 §2.1 / §2.1.1,
// requirements `DX-R6`–`DX-R9`, check `DX-C2a`.
// Tier: cross-implementation conformance (TESTING-STRATEGY).
//
// WHY THIS EXISTS. §2.1 makes one operation normative and names the trap by
// hand:
//
//	[MUST] A republished entity MUST be bound byte-identically to the form in
//	which it was obtained. An implementation MUST NOT re-encode it — INCLUDING
//	BY DECODING IT THROUGH A TYPE IT DOES NOT FULLY DECLARE.
//
//	[MUST] An implementation MUST provide, and republication MUST use, an
//	operation that binds OBTAINED BYTES rather than DATA. The ordinary
//	put(path, type, data) shape is the one a developer reaches for and it is
//	the broken one.
//
// §2.1.1 then makes the FIXTURE obligation normative (`DX-R9`): a check
// exercising §2.1 MUST use input the implementation's own encoder would not
// emit, because "a round trip through bytes your own encoder produced proves
// nothing — decode-and-re-encode is lossless over exactly that input."
//
// That is precisely the hole in the gate next door.
// `TestSiteEntityBytes_MatchRustEmission` round-trips another implementation's
// real site entities and asserts byte equality — and it is GREEN, which tells
// us their emission contains no key we drop. It says nothing whatever about
// the case §2.1 is about, and it cannot: its fixture is the agreement.
//
// WHAT A GREEN RUN CLAIMS. That decoding a foreign entity through our Go types
// MOVES ITS ADDRESS whenever it carries a key we do not model — measured, with
// the byte deltas printed — and that carrying `entity.Entity` does not. It is a
// measurement of the HAZARD, not of a defect: this repo republishes nothing
// today (see docs/status/AUDIT-2026-09-16-a-…), so there is no live §2.1
// violation to catch. The file exists so that the day a gathered view is built,
// the number is already on the record and the shape of the write is not a
// matter of opinion.
//
// WHAT IT DOES NOT CLAIM. It is a MECHANISM gate and there is deliberately no
// WIRING gate beside it (AP107/AP108) — a wiring gate would assert that no
// durable write of a foreign entity goes through the decoded view, which is
// VACUOUSLY TRUE while no such write exists anywhere in the tree. Writing it
// now would be a gate that cannot fail, counted as coverage. It is owed by the
// first republish path, not by this file.

import (
	"bytes"
	"testing"

	"github.com/fxamacker/cbor/v2"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"

	"entity-workbench-go/entitysdk"
)

// dxEntity builds an entity from a data map, the way a foreign publisher's
// encoder would: the map IS the authored shape, and nothing here goes through
// one of our declared structs.
func dxEntity(t *testing.T, typeName string, data map[string]interface{}) entity.Entity {
	t.Helper()
	raw, err := ecf.Encode(data)
	if err != nil {
		t.Fatalf("encode %s: %v", typeName, err)
	}
	ent, err := entity.NewEntity(typeName, cbor.RawMessage(raw))
	if err != nil {
		t.Fatalf("new entity %s: %v", typeName, err)
	}
	return ent
}

// dxRoundTrip is the broken shape, written out in full because naming it is
// half the point: decode the obtained bytes into a type we declare, then bind
// the DECODED VALUE. Every durable write in this repo's application tier has
// this shape (`Store.Put(path, type, data)`), which is correct for data we
// authored and is what §2.1 forbids for data we obtained.
func dxRoundTrip(t *testing.T, ent entity.Entity, into interface{}) entity.Entity {
	t.Helper()
	if err := ecf.Decode(ent.Data, into); err != nil {
		t.Fatalf("decode through our type: %v", err)
	}
	raw, err := ecf.Encode(into)
	if err != nil {
		t.Fatalf("re-encode our view: %v", err)
	}
	out, err := entity.NewEntity(ent.Type, cbor.RawMessage(raw))
	if err != nil {
		t.Fatalf("re-bind: %v", err)
	}
	return out
}

// TestDX_R9_AConventionDeclaredFieldWeDoNotModelMovesTheAddress is the arm that
// matters, because its input is not exotic.
//
// `APP-CONVENTION-SEMANTIC-CONTENT-SITE` §4's CDDL for `site-page` declares
// FOUR keys:
//
//	site-page = { format, body, ? frontmatter, ? embeds: [* (path / content-hash)] }
//
// `entitysdk.SitePage` declares three. `embeds` — the sibling Embed entities a
// page transcludes in child mode — is not modelled anywhere in this tree. So a
// publisher using a documented, optional, conformant field of the convention we
// implement produces a page whose address we cannot preserve.
//
// This is NOT an unknown-field forward-compatibility case. It is a field the
// specification we build against declares today.
func TestDX_R9_AConventionDeclaredFieldWeDoNotModelMovesTheAddress(t *testing.T) {
	theirs := dxEntity(t, entitysdk.TypeSitePage, map[string]interface{}{
		"format": "markdown",
		"body":   "# Notes\n\n::embed[a figure]{ref=assets/figures/demo.svg}\n",
		"embeds": []interface{}{"assets/figures/demo.svg"},
	})

	var ours entitysdk.SitePage
	ours2 := dxRoundTrip(t, theirs, &ours)

	t.Logf("site-page with `embeds`: %d B -> %d B", len(theirs.Data), len(ours2.Data))

	if theirs.ContentHash.String() == ours2.ContentHash.String() {
		t.Fatal("ANTI-VACUITY: the address did NOT move, so either `embeds` is now modelled " +
			"(good — delete this arm and say so) or this probe stopped reaching the decode path")
	}
	if len(ours2.Data) >= len(theirs.Data) {
		t.Errorf("expected the re-encoding to be SHORTER (a dropped field); got %d -> %d",
			len(theirs.Data), len(ours2.Data))
	}

	// And the consequence, stated where it is measured rather than in prose:
	// a detached signature binds at a pointer derived from the hash (§2.2), so
	// once the address moves the signature no longer names the entity and a
	// conformant renderer MUST present the entry as unattributed (§2.1).
	var check map[string]interface{}
	if err := ecf.Decode(ours2.Data, &check); err != nil {
		t.Fatalf("decode our re-encoding: %v", err)
	}
	if _, present := check["embeds"]; present {
		t.Error("`embeds` survived the round trip; this arm is measuring the wrong thing")
	}
}

// TestDX_R9_AnUnknownFieldMovesTheAddress is §2.1.1's literal case — "an entry
// carrying a field this build has never heard of, which the core protocol
// obliges it to ignore and preserve" ([ADR-0002]: unknowns are MUST-ignore).
//
// Kept separate from the arm above even though the mechanism is identical,
// because the two answer different questions and only one of them can be closed
// by modelling a field. A gatherer aggregates types it did not write (§2.1's
// own warning), so this arm can never be retired.
func TestDX_R9_AnUnknownFieldMovesTheAddress(t *testing.T) {
	theirs := dxEntity(t, entitysdk.TypeSiteManifest, map[string]interface{}{
		"site_id":    "demo",
		"title":      "Demo",
		"theme_hint": "dark",
	})

	var ours entitysdk.SiteManifest
	ours2 := dxRoundTrip(t, theirs, &ours)

	t.Logf("site-manifest with an unknown key: %d B -> %d B", len(theirs.Data), len(ours2.Data))

	if theirs.ContentHash.String() == ours2.ContentHash.String() {
		t.Fatal("the address did not move on an unknown field; ecf.Decode has started " +
			"preserving undeclared keys, which would be a substantial change worth reading before " +
			"deleting this arm")
	}
}

// TestDX_OurRoundTripIsFaithfulOverExactlyTheFieldsWeModel is the anti-vacuity
// arm for the whole file, and it is not optional.
//
// Without it, every assertion above is satisfied by an encoder that is simply
// broken — "the hash moved" is the expected outcome of a corrupt round trip
// too. This arm establishes that the round trip IS lossless over the input
// §2.1.1 warns is worthless as a fixture, which is what makes the other arms
// attributable to the dropped field rather than to the mechanism.
func TestDX_OurRoundTripIsFaithfulOverExactlyTheFieldsWeModel(t *testing.T) {
	theirs := dxEntity(t, entitysdk.TypeSitePage, map[string]interface{}{
		"format":      "markdown",
		"body":        "# Notes\n",
		"frontmatter": map[string]interface{}{"title": "Notes"},
	})

	var ours entitysdk.SitePage
	ours2 := dxRoundTrip(t, theirs, &ours)

	if theirs.ContentHash.String() != ours2.ContentHash.String() {
		t.Fatalf("round trip over exactly our own fields moved the address (%d B -> %d B) — "+
			"the other arms in this file are measuring an encoder fault, not a dropped field",
			len(theirs.Data), len(ours2.Data))
	}
}

// TestDX_R8_CarryingTheEntityPreservesTheAddress is the fix, measured at the
// layer where it is decidable: `entity.Entity` holds `Data` as a
// `cbor.RawMessage`, so carrying the entity carries the obtained bytes and
// re-binding is a no-op on the address — for exactly the same input the arms
// above destroy.
//
// `entitysdk.AppPeer.PutEntity(path, ent)` is this repo's `DX-R8` operation and
// takes precisely this carrier. Note what is NOT asserted here: that any
// application-tier code path uses it. It does not — all twenty callers are in
// `programs/`, writing their own locally-authored state.
func TestDX_R8_CarryingTheEntityPreservesTheAddress(t *testing.T) {
	theirs := dxEntity(t, entitysdk.TypeSitePage, map[string]interface{}{
		"format": "markdown",
		"body":   "# Notes\n",
		"embeds": []interface{}{"assets/figures/demo.svg"},
	})

	// The carry: nothing is decoded, so nothing can be dropped.
	carried, err := entity.NewEntity(theirs.Type, theirs.Data)
	if err != nil {
		t.Fatalf("re-bind obtained bytes: %v", err)
	}

	if theirs.ContentHash.String() != carried.ContentHash.String() {
		t.Fatalf("carrying the obtained bytes moved the address: %x -> %x",
			theirs.ContentHash.Bytes(), carried.ContentHash.Bytes())
	}
	if !bytes.Equal(theirs.Data, carried.Data) {
		t.Fatal("carrying the obtained bytes changed them")
	}

	// The discriminator, in one file: same input, two shapes, opposite outcomes.
	var ours entitysdk.SitePage
	if dropped := dxRoundTrip(t, theirs, &ours); dropped.ContentHash.String() == theirs.ContentHash.String() {
		t.Fatal("the decoded-view shape preserved the address, so this test no longer " +
			"discriminates between the two shapes and proves nothing")
	}
}

// TestDX_OurAttributeBagsAreNarrowerThanTheConvention is a finding rather than
// a hazard probe, and it fails LOUDER than the arms above because its outcome
// is not a moved address — it is a refusal to read a conformant publisher at
// all.
//
// `site-manifest` declares `? params: { * tstr => any }` and `site-page`
// declares `? frontmatter: { ? title: tstr, * tstr => any }`. Both value types
// are `any`. Ours are `map[string]string` in both places
// (entitysdk/site.go:89, :122).
//
// So a conformant manifest carrying a non-string attribute — a number, a bool,
// a nested map — does not merely lose the key. The whole decode fails, and the
// operator is told the publisher's site is malformed.
func TestDX_OurAttributeBagsAreNarrowerThanTheConvention(t *testing.T) {
	theirs := dxEntity(t, entitysdk.TypeSiteManifest, map[string]interface{}{
		"site_id": "demo",
		"title":   "Demo",
		"params":  map[string]interface{}{"root": "index", "weight": uint64(3)},
	})

	var ours entitysdk.SiteManifest
	err := ecf.Decode(theirs.Data, &ours)

	if err == nil {
		t.Logf("decoded a non-string `params` value without error; params=%v — "+
			"the narrowing is silent rather than fatal, which is the better half of "+
			"this finding and should be recorded as such", ours.Params)
		return
	}
	t.Logf("MEASURED: a conformant `params: {* tstr => any}` is UNDECODABLE by our type: %v", err)
	t.Log("consequence: the surface reports the publisher's manifest as malformed, " +
		"which is a false accusation against the other machine (the AP44 shape)")
}
