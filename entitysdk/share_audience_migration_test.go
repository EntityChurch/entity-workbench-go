package entitysdk_test

// The `audience` encoding fix and its migration arm. Tier: unit
// (TESTING-STRATEGY).
//
// `share_crossimpl_test.go` establishes that we now agree with the other seat
// on the conformant shape. This file covers the half their fixture cannot
// reach: **what happens to the records this SDK already wrote.**
//
// Every `app/share/record` authored by this product before 2026-09-13 carries
// `audience` as an array of bare `data` maps — the non-conformant shape — and
// those bytes are in real trees on real peers because we put them there. So the
// reader takes both arms and the writer takes one, which is the same posture as
// the `content-type` → `content_type` window-state fix: the new spelling is
// written, the old one is read and then gone at the next save.
//
// **That is a migration, not leniency** (cf. AP33). The two shapes have
// disjoint key sets, so the discrimination is exact rather than heuristic, and
// an element matching NEITHER is an error — turning one into an entry with an
// empty grantee would put a member nobody named into an audience.

import (
	"strings"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"

	"entity-workbench-go/entitysdk"
)

// encodeLegacyRecord builds the shape this SDK used to write: `audience` as an
// array of BARE data maps, with no `{type, data}` envelope. Built from a raw
// map on purpose — going through our own types would encode the new shape and
// the test would measure nothing.
func encodeLegacyRecord(t *testing.T) entity.Entity {
	t.Helper()
	raw, err := ecf.Encode(map[string]interface{}{
		"title": "quarterly-figures.pdf",
		"target": map[string]interface{}{
			"tag":  "prefix",
			"path": "local/files/figures/",
		},
		"audience": []interface{}{
			map[string]interface{}{
				"grantee":  "2KLv2nhwtPrLFd4BZFQuNK1ujtE74q8cVg7y8cYdcZZ5BL",
				"via":      "direct",
				"added_at": uint64(1_757_462_400_000),
			},
			map[string]interface{}{
				"grantee":  "2KHPSRBHu13dCYEo4AQW8rqZ1yLVE2Hmu3faXJUy3zcs8L",
				"added_at": uint64(1_757_462_401_000),
			},
		},
		"created_at": uint64(1_757_462_400_000),
	})
	if err != nil {
		t.Fatalf("encode legacy body: %v", err)
	}
	ent, err := entity.NewEntity(entitysdk.TypeShareRecord, cbor.RawMessage(raw))
	if err != nil {
		t.Fatalf("legacy entity: %v", err)
	}
	return ent
}

// The migration itself: a record written by the old encoder still reads, with
// every field intact, including an ABSENT `via` staying absent.
func TestALegacyAudienceStillReads(t *testing.T) {
	var rec entitysdk.ShareRecordData
	if err := ecf.Decode(encodeLegacyRecord(t).Data, &rec); err != nil {
		t.Fatalf("a record this SDK itself wrote no longer decodes: %v\n"+
			"every share an operator has authored is in that shape", err)
	}
	if len(rec.Audience) != 2 {
		t.Fatalf("audience: got %d entries, want 2", len(rec.Audience))
	}
	if rec.Audience[0].Grantee != "2KLv2nhwtPrLFd4BZFQuNK1ujtE74q8cVg7y8cYdcZZ5BL" {
		t.Errorf("grantee[0]: %q", rec.Audience[0].Grantee)
	}
	if rec.Audience[0].Via != entitysdk.AudienceOriginDirect {
		t.Errorf("via[0]: %q", rec.Audience[0].Via)
	}
	if rec.Audience[1].Via != "" {
		t.Errorf("via[1]: %q — an absent via must stay absent, never defaulted to %q",
			rec.Audience[1].Via, entitysdk.AudienceOriginDirect)
	}
	if rec.Audience[1].AddedAt != 1_757_462_401_000 {
		t.Errorf("added_at[1]: %d", rec.Audience[1].AddedAt)
	}
}

// READ-ONLY: re-encoding a legacy record emits the CONFORMANT shape. This is
// what makes the old spelling gone at the next save rather than carried
// forever, and it is the assertion that would fail if somebody "fixed" the
// migration by making the writer symmetric with the reader.
func TestALegacyRecordIsRewrittenInTheConformantShape(t *testing.T) {
	legacy := encodeLegacyRecord(t)
	var rec entitysdk.ShareRecordData
	if err := ecf.Decode(legacy.Data, &rec); err != nil {
		t.Fatal(err)
	}
	round, err := rec.ToEntity()
	if err != nil {
		t.Fatal(err)
	}
	if round.ContentHash == legacy.ContentHash {
		t.Fatal("re-encoding a legacy record reproduced the legacy bytes — the writer is still emitting " +
			"the bare-data shape, so nothing migrates")
	}

	// And the re-encoded form is the entity shape, checked on the WIRE rather
	// than through our own types (which would agree with themselves whatever
	// they do).
	var wire struct {
		Audience []struct {
			Type    string `cbor:"type"`
			Grantee string `cbor:"grantee"`
		} `cbor:"audience"`
	}
	if err := ecf.Decode(round.Data, &wire); err != nil {
		t.Fatal(err)
	}
	for i, e := range wire.Audience {
		if e.Type != entitysdk.TypeShareAudience {
			t.Errorf("re-encoded audience[%d] carries type %q, want %q", i, e.Type, entitysdk.TypeShareAudience)
		}
		if e.Grantee != "" {
			t.Errorf("re-encoded audience[%d] still carries `grantee` at the top level", i)
		}
	}
}

// An element matching NEITHER arm is an error. The alternative — decoding it to
// a zero-valued entry — would put a member with an empty grantee into an
// audience, and `Validate` would then refuse the record for a reason that names
// the wrong thing.
func TestAnUnrecognizableAudienceElementIsRefused(t *testing.T) {
	raw, err := ecf.Encode(map[string]interface{}{
		"title":      "x",
		"target":     map[string]interface{}{"tag": "prefix", "path": "local/files/x/"},
		"audience":   []interface{}{map[string]interface{}{"who": "somebody"}},
		"created_at": uint64(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	var rec entitysdk.ShareRecordData
	err = ecf.Decode(raw, &rec)
	if err == nil {
		t.Fatalf("an audience element in neither shape was accepted, yielding %d entries", len(rec.Audience))
	}
	if !strings.Contains(err.Error(), "audience element 0") {
		t.Errorf("the refusal should name the element: %v", err)
	}
}

// SHARE-7's state, on the wire and in both directions: an empty audience is
// PRESENT AND EMPTY, never absent and never null. Absent is a publication's
// shape and means the opposite thing, so a nil slice that encoded as `null`
// would turn *"nobody yet"* into *"anyone"* at a reader keying on presence.
func TestAnEmptyAudienceIsPresentAndEmpty(t *testing.T) {
	rec := entitysdk.ShareRecordData{
		Title:     "draft-notes",
		Target:    entitysdk.PrefixTarget("local/files/drafts/"),
		Audience:  nil, // the nil case, deliberately — not an empty literal
		CreatedAt: 1_757_466_000_000,
	}
	ent, err := rec.ToEntity()
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]cbor.RawMessage
	if err := ecf.Decode(ent.Data, &wire); err != nil {
		t.Fatal(err)
	}
	got, present := wire["audience"]
	if !present {
		t.Fatal("`audience` is absent from a record with no members; §2.2 makes it present and empty, " +
			"and an absent key is §2.5's publication shape — the opposite meaning")
	}
	if len(got) != 1 || got[0] != 0x80 {
		t.Errorf("`audience` encoded as % x, want an empty ARRAY (0x80) — a CBOR null would be a third state", got)
	}
}
