package workbench_test

import (
	"testing"

	"github.com/fxamacker/cbor/v2"
	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"

	"entity-workbench-go/entitysdk"
	wb "entity-workbench-go/workbench"
)

// encodeOfferEntity builds an entity of a given type from a raw field map.
// Deliberately does NOT go through any of our typed `ToEntity` helpers: the
// point is to encode what the other seat's emitter puts on the wire, so a
// rename of one of our struct tags has to fail here.
func encodeOfferEntity(t *testing.T, typeName string, data map[string]interface{}) entity.Entity {
	t.Helper()
	raw, err := ecf.Encode(data)
	if err != nil {
		t.Fatalf("encode %s: %v", typeName, err)
	}
	ent, err := entity.NewEntity(typeName, cbor.RawMessage(raw))
	if err != nil {
		t.Fatalf("NewEntity %s: %v", typeName, err)
	}
	return ent
}

// share_publication_crossimpl_test.go — the check `entity-browser-rust` asked
// back for in `ROUTING-2026-09-10-b-…` §1: *"a check that our `record` decodes
// at your reader. The rename is worthless if the body does not match."*
//
// Their alignment landed `app/share/record` **plus** the `app/share/publication`
// split arch ruled in A-5/A-5b, and the second half is the one that bit us:
// before this file, `workbench.ShareOffers` rejected a publication with
// *"unexpected type app/share/publication"* and reported the peer's conformant
// entity as a PROBLEM. Their public shares were invisible to us and the
// diagnosis pointed at them.
//
// **The bodies are built here from the CDDL and from their emitter's own field
// spellings** (`src/share.rs`: `tag`/`path`/`hash` at `:553-563`, `title`,
// `note`, `created_at`, `audience`) rather than by importing anything of ours,
// so a rename on our side fails this file instead of being invisible to it.
// This is the cheap arm; it is not the arm that would catch a divergence in
// *their* encoder, which needs their bytes — offered in the reply packet.

func publicationEntity(t *testing.T, data map[string]interface{}) entity.Entity {
	t.Helper()
	return encodeOfferEntity(t, entitysdk.TypeSharePublication, data)
}

func recordEntity(t *testing.T, data map[string]interface{}) entity.Entity {
	t.Helper()
	return encodeOfferEntity(t, entitysdk.TypeShareRecord, data)
}

func TestAPublicationDecodesAtOurReader(t *testing.T) {
	ent := publicationEntity(t, map[string]interface{}{
		"title": "Field notes",
		"target": map[string]interface{}{
			"tag":  "prefix",
			"path": "local/files/notes/",
		},
		"note":       "published to anyone who can reach us",
		"created_at": uint64(1_757_500_000_000),
	})

	offer, ok := wb.DecodeRemoteShareOffer("app/share/records/notes", ent)
	if !ok {
		t.Fatal("a conformant app/share/publication was refused by our reader — " +
			"this is the state that reported the other seat's entity as malformed")
	}
	if offer.Root != "notes" {
		t.Errorf("root: got %q, want notes", offer.Root)
	}
	if offer.Title != "Field notes" {
		t.Errorf("title: got %q", offer.Title)
	}
	if offer.TargetPrefix != "local/files/notes/" {
		t.Errorf("target prefix: got %q", offer.TargetPrefix)
	}
	if offer.CreatedAtMillis != 1_757_500_000_000 {
		t.Errorf("created_at: got %d", offer.CreatedAtMillis)
	}
	if !offer.Public {
		t.Error("a publication must read as Public")
	}
}

// The distinction the split exists for, and the one a shared struct would have
// destroyed. §2.2 gives an empty `audience` the meaning *"authored, no members
// yet"* — self-only — while §2.5's publication is reachable by anyone. They are
// opposites, and `SHARE-7` is the vector that fails on reading one as the other.
func TestSelfOnlyAndPublicAreNotTheSameOffer(t *testing.T) {
	target := map[string]interface{}{"tag": "prefix", "path": "local/files/notes/"}

	selfOnly, ok := wb.DecodeRemoteShareOffer("app/share/records/notes", recordEntity(t, map[string]interface{}{
		"title":      "Field notes",
		"target":     target,
		"audience":   []interface{}{},
		"created_at": uint64(1_757_500_000_000),
	}))
	if !ok {
		t.Fatal("a record with an empty audience is legal per §2.2 and was refused")
	}
	public, ok := wb.DecodeRemoteShareOffer("app/share/records/notes", publicationEntity(t, map[string]interface{}{
		"title":      "Field notes",
		"target":     target,
		"created_at": uint64(1_757_500_000_000),
	}))
	if !ok {
		t.Fatal("publication refused")
	}

	if selfOnly.Public {
		t.Error("an empty audience is self-only, never public (§2.2, SHARE-7)")
	}
	if !public.Public {
		t.Error("a publication is public")
	}

	// The consequence a surface acts on, which is the whole point of keeping
	// the two apart: a stranger may fetch one and not the other.
	const stranger = "2KLv2nhwtPrLFd4BZFQuNK1ujtE74q8cVg7y8cYdcZZ5BL"
	if selfOnly.OfferedTo(stranger) {
		t.Error("a self-only record is offered to nobody")
	}
	if !public.OfferedTo(stranger) {
		t.Error("a publication is offered to anyone who can reach us")
	}

	// Anti-vacuity: with `Public` derived from `len(Audience)` instead of from
	// the type, both of the above would report identically. Assert the two
	// offers differ on that field alone, so a future "simplification" that
	// drops the field fails here rather than silently.
	if selfOnly.Public == public.Public {
		t.Fatal("the two forms are indistinguishable — Public is not reading the type")
	}
}

// A publication carrying an `audience` is invalid per §2.5. We do not reject the
// entity at the reader (a permissive read is the right posture for in-flight
// interop, same call as the §5.4 legacy fields), but the field MUST NOT reach a
// surface as an audience — that would present a pull-only share as addressed.
func TestAnAudienceOnAPublicationDoesNotBecomeAnAudience(t *testing.T) {
	ent := publicationEntity(t, map[string]interface{}{
		"title":      "Field notes",
		"target":     map[string]interface{}{"tag": "prefix", "path": "local/files/notes/"},
		"audience":   []interface{}{map[string]interface{}{"grantee": "PEER-A", "added_at": uint64(1)}},
		"created_at": uint64(1_757_500_000_000),
	})
	offer, ok := wb.DecodeRemoteShareOffer("app/share/records/notes", ent)
	if !ok {
		t.Fatal("refused")
	}
	if len(offer.Audience) != 0 {
		t.Errorf("an invalid audience on a publication leaked to the surface as %v (§2.5: such a publication is invalid)", offer.Audience)
	}
	if !offer.Public {
		t.Error("still a publication")
	}
}
