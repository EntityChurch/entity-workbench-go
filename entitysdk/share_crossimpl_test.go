package entitysdk_test

// Cross-impl gate for `app/share/*` — OUR reader and OUR encoder measured
// against entity-browser-rust's own frozen emission
// (testdata/crossimpl-rust-share/, their `dev` @ 2e24636, built at bd84bcf).
// Tier: cross-implementation conformance (TESTING-STRATEGY).
//
// WHY THIS EXISTS, AND WHY THE TEST THAT ALREADY EXISTED COULD NOT DO IT.
// workbench/share_publication_crossimpl_test.go builds its bodies by hand
// from the CDDL and from their emitter's field spellings. That catches a
// rename on OUR side — and it caught nothing here, because our hand-built
// audience and our decoder were wrong in the SAME direction. A test
// population you generated cannot contain the shape you are missing; the
// only cure is bytes that crossed the boundary. This is ask `B-7`, which
// we filed for exactly this reason and which they discharged 2026-09-12.
//
// WHAT A GREEN RUN CLAIMS: given THEIR canonical bodies, our reader decodes
// every field to the value they say it means, and our encoder reproduces
// THEIR bytes exactly. It does NOT claim anything about a served tree, a
// publisher, a keypair or a signed root — none of those appear in these
// bytes, on purpose (§4 makes the namespace the publisher).
//
// A DIVERGENCE IS ROUTED, NOT LOCALLY CORRECTED — the standing rule for
// their vectors. The ONE exception already exercised: where the convention
// decides the question outright and we are simply non-conformant, we fix it
// and tell them. That is what happened to `audience` on arrival; see
// TestAudienceElementsAreWholeEntities, which is the pin for the reading
// that justified changing our bytes.

import (
	hexenc "encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"

	"entity-workbench-go/entitysdk"
)

const rustShareFixture = "testdata/crossimpl-rust-share"

// theirExpected mirrors EXPECTED.json. Only the fields we assert on are
// declared; an undeclared field is dropped in silence (AP49), so anything
// added here later must be read as well as declared.
type theirExpected struct {
	Publisher string `json:"publisher"`
	Entities  []struct {
		Name        string `json:"name"`
		Type        string `json:"type"`
		BodyFile    string `json:"body_file"`
		ContentHash string `json:"content_hash"`
		Why         string `json:"why"`
		DecodesTo   struct {
			Title     string  `json:"title"`
			Note      *string `json:"note"`
			CreatedAt uint64  `json:"created_at"`
			From      string  `json:"from"`
			Target    struct {
				Tag  string `json:"tag"`
				Hash string `json:"hash"`
				Path string `json:"path"`
			} `json:"target"`
			// nil means "no audience key at all" (a publication); an empty
			// slice means "present and empty" (self-only). Three states, and
			// the pointer is what keeps two of them apart.
			Audience *[]struct {
				Grantee string  `json:"grantee"`
				Via     *string `json:"via"`
				AddedAt uint64  `json:"added_at"`
			} `json:"audience"`
			AudienceIsSelfOnly bool `json:"audience_is_self_only"`
		} `json:"decodes_to"`
	} `json:"entities"`
}

func loadTheirShareFixture(t *testing.T) theirExpected {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(rustShareFixture, "EXPECTED.json"))
	if err != nil {
		t.Fatalf("read their EXPECTED.json: %v", err)
	}
	var exp theirExpected
	if err := json.Unmarshal(raw, &exp); err != nil {
		t.Fatalf("decode their EXPECTED.json: %v", err)
	}
	if len(exp.Entities) == 0 {
		t.Fatal("anti-vacuity: their fixture declares zero entities")
	}
	return exp
}

// Step 2 of their protocol, and it is a stop condition: a body must re-hash
// to the filename it is filed under. If this fails the fixture is corrupt in
// transit and nothing below means anything.
func TestTheirShareBodiesReHashToTheirFilenames(t *testing.T) {
	exp := loadTheirShareFixture(t)
	for _, row := range exp.Entities {
		raw, err := os.ReadFile(filepath.Join(rustShareFixture, row.BodyFile))
		if err != nil {
			t.Fatalf("%s: read body: %v", row.Name, err)
		}
		claimed, err := hash.ParseHex(row.ContentHash)
		if err != nil {
			t.Fatalf("%s: parse their content_hash: %v", row.Name, err)
		}
		got, err := hash.OfBytes(claimed.Algorithm, raw)
		if err != nil {
			t.Fatalf("%s: hash body: %v", row.Name, err)
		}
		if got != claimed {
			t.Errorf("%s: body re-hashes to %s, filed under %s — the fixture is corrupt in transit",
				row.Name, got.String(), claimed.String())
		}
	}
}

// Steps 3 and 4: decode with our reader, field by field, BEFORE any hash
// comparison — a bare hash mismatch names the entity and not the field.
func TestTheirShareBodiesDecodeAtOurReader(t *testing.T) {
	exp := loadTheirShareFixture(t)
	seen := map[string]bool{}

	for _, row := range exp.Entities {
		row := row
		t.Run(row.Name, func(t *testing.T) {
			ent := readTheirBody(t, row.BodyFile)
			seen[ent.Type] = true
			if ent.Type != row.Type {
				t.Fatalf("type: got %q, want %q", ent.Type, row.Type)
			}

			want := row.DecodesTo
			switch ent.Type {
			case entitysdk.TypeShareRecord:
				var rec entitysdk.ShareRecordData
				if err := ecf.Decode(ent.Data, &rec); err != nil {
					t.Fatalf("our reader refused their conformant record: %v", err)
				}
				if rec.Title != want.Title {
					t.Errorf("title: got %q, want %q", rec.Title, want.Title)
				}
				if got, w := rec.Note, deref(want.Note); got != w {
					t.Errorf("note: got %q, want %q", got, w)
				}
				if rec.CreatedAt != want.CreatedAt {
					t.Errorf("created_at: got %d, want %d", rec.CreatedAt, want.CreatedAt)
				}
				assertTarget(t, rec.Target, want.Target.Tag, want.Target.Hash, want.Target.Path)

				if want.Audience == nil {
					t.Fatalf("their fixture gives a record no audience key; that is a publication's shape")
				}
				if len(rec.Audience) != len(*want.Audience) {
					t.Fatalf("audience: got %d entries, want %d — %s",
						len(rec.Audience), len(*want.Audience), row.Why)
				}
				// ORDER IS AUTHORED, not sorted. A seat that sorts here
				// produces a record whose bytes cannot be reproduced.
				for i, w := range *want.Audience {
					got := rec.Audience[i]
					if got.Grantee != w.Grantee {
						t.Errorf("audience[%d].grantee: got %q, want %q", i, got.Grantee, w.Grantee)
					}
					if got.Via != deref(w.Via) {
						t.Errorf("audience[%d].via: got %q, want %q (absent must stay absent, never defaulted to %q)",
							i, got.Via, deref(w.Via), entitysdk.AudienceOriginDirect)
					}
					if got.AddedAt != w.AddedAt {
						t.Errorf("audience[%d].added_at: got %d, want %d", i, got.AddedAt, w.AddedAt)
					}
				}
				// SHARE-7: empty is self-only, never public. The only place
				// this fixture can tell us we got it backwards.
				if selfOnly := len(rec.Audience) == 0; selfOnly != want.AudienceIsSelfOnly {
					t.Errorf("self-only: got %v, want %v — %s", selfOnly, want.AudienceIsSelfOnly, row.Why)
				}

			case entitysdk.TypeSharePublication:
				var pub entitysdk.SharePublicationData
				if err := ecf.Decode(ent.Data, &pub); err != nil {
					t.Fatalf("our reader refused their conformant publication: %v", err)
				}
				if pub.Title != want.Title {
					t.Errorf("title: got %q, want %q", pub.Title, want.Title)
				}
				if got, w := pub.Note, deref(want.Note); got != w {
					t.Errorf("note: got %q, want %q", got, w)
				}
				if pub.CreatedAt != want.CreatedAt {
					t.Errorf("created_at: got %d, want %d", pub.CreatedAt, want.CreatedAt)
				}
				assertTarget(t, pub.Target, want.Target.Tag, want.Target.Hash, want.Target.Path)
				if want.Audience != nil {
					t.Errorf("their fixture gives a publication an audience key; §2.5 makes that invalid")
				}
				if err := pub.Validate(); err != nil {
					t.Errorf("their conformant publication failed our Validate: %v", err)
				}

			default:
				t.Fatalf("unexpected type %q", ent.Type)
			}
		})
	}

	// Anti-vacuity on the population, not on one row: a fixture that quietly
	// stopped carrying publications would leave every assertion above green.
	for _, want := range []string{entitysdk.TypeShareRecord, entitysdk.TypeSharePublication} {
		if !seen[want] {
			t.Errorf("their fixture carried no %s — the population no longer covers the type split", want)
		}
	}
}

// Step 5, and the one a round trip against our own encoder cannot see: decode
// THEIR body with our types, re-encode with OUR encoder, and require the bytes
// back. A decode that agrees and a re-encode that does not is the most
// interesting outcome this fixture can produce.
//
// This is the assertion that was RED on arrival: our `audience` elements were
// the bare `data` map where §2.3 makes each one a whole entity.
func TestOurEncoderReproducesTheirShareBytes(t *testing.T) {
	exp := loadTheirShareFixture(t)
	for _, row := range exp.Entities {
		row := row
		t.Run(row.Name, func(t *testing.T) {
			if row.Type != entitysdk.TypeShareRecord {
				// We author no publications on purpose (offering a folder to
				// anyone who can reach us is a product decision, not a codec
				// convenience), so there is no ToEntity to measure. Named
				// rather than skipped silently.
				t.Skip("we read app/share/publication and deliberately do not emit it — see TypeSharePublication's doc comment")
			}
			ent := readTheirBody(t, row.BodyFile)

			var rec entitysdk.ShareRecordData
			if err := ecf.Decode(ent.Data, &rec); err != nil {
				t.Fatalf("decode: %v", err)
			}
			round, err := rec.ToEntity()
			if err != nil {
				t.Fatalf("our encoder refused a body we had just decoded: %v", err)
			}

			theirs, err := os.ReadFile(filepath.Join(rustShareFixture, row.BodyFile))
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			ours, err := ecf.EncodeHashable(round.Type, round.Data)
			if err != nil {
				t.Fatalf("re-encode: %v", err)
			}
			if hexenc.EncodeToString(ours) != hexenc.EncodeToString(theirs) {
				t.Errorf("re-encode diverged.\n ours: %s\ntheirs: %s\n%s",
					hexenc.EncodeToString(ours), hexenc.EncodeToString(theirs), row.Why)
			}
			if round.ContentHash.String() != mustParse(t, row.ContentHash).String() {
				t.Errorf("content hash: got %s, want %s", round.ContentHash, row.ContentHash)
			}
		})
	}
}

// THE READING THAT JUSTIFIED CHANGING OUR BYTES, pinned so it is greppable if
// it is ever wrong.
//
// `APP-CONVENTION-SHARE` decides this one CDDL block apart, and the contrast is
// the whole argument:
//
//	share-target  = blob-target / prefix-target
//	blob-target   = { tag: "blob", hash: content-hash }        <- BARE inline map
//	audience-entry = { type: "app/share/audience-entry",
//	                   data: { grantee, ? via, added_at } }    <- WHOLE entity
//
// So the convention does distinguish an inline structure from an inlined
// entity, and `audience` is the second kind. We encoded it as the first for as
// long as the type has existed. This is OUR non-conformance, not a cohort
// disagreement, which is why it was corrected here rather than routed — the
// same call as the `sites/` placement (AP96).
func TestAudienceElementsAreWholeEntities(t *testing.T) {
	exp := loadTheirShareFixture(t)
	var row = -1
	for i, e := range exp.Entities {
		if e.Name == "record-one-member" {
			row = i
		}
	}
	if row < 0 {
		t.Fatal("anti-vacuity: their fixture no longer carries record-one-member")
	}
	ent := readTheirBody(t, exp.Entities[row].BodyFile)

	// Read the audience array WITHOUT our share types, so this asserts on the
	// wire shape rather than on whatever our struct currently does with it.
	var wire struct {
		Audience []struct {
			Type string         `cbor:"type"`
			Data ecfRawAudience `cbor:"data"`
			// The flat arm's own keys, declared so a body carrying them
			// instead is visible here rather than decoding to a zero value.
			Grantee string `cbor:"grantee"`
		} `cbor:"audience"`
	}
	if err := ecf.Decode(ent.Data, &wire); err != nil {
		t.Fatalf("decode their record's audience as raw wire: %v", err)
	}
	if len(wire.Audience) != 1 {
		t.Fatalf("expected one audience entry, got %d", len(wire.Audience))
	}
	e := wire.Audience[0]
	if e.Type != entitysdk.TypeShareAudience {
		t.Errorf("an audience element carries type %q; §2.3 makes it a whole %s entity",
			e.Type, entitysdk.TypeShareAudience)
	}
	if e.Grantee != "" {
		t.Errorf("an audience element carries `grantee` at the TOP level (%q) — that is the bare-data "+
			"shape we used to emit, and it is what this gate exists to fail on", e.Grantee)
	}
	if e.Data.Grantee == "" {
		t.Error("an audience element's `data` carries no grantee")
	}
}

type ecfRawAudience struct {
	Grantee string `cbor:"grantee"`
	Via     string `cbor:"via"`
	AddedAt uint64 `cbor:"added_at"`
}

func readTheirBody(t *testing.T, name string) entity.Entity {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(rustShareFixture, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var ent entity.Entity
	if err := ecf.Decode(raw, &ent); err != nil {
		t.Fatalf("decode %s as an entity: %v", name, err)
	}
	return ent
}

func assertTarget(t *testing.T, got entitysdk.ShareTarget, tag, wantHash, wantPath string) {
	t.Helper()
	if got.Tag != tag {
		t.Errorf("target tag: got %q, want %q", got.Tag, tag)
	}
	switch tag {
	case entitysdk.ShareTargetBlob:
		if got.Hash == nil {
			t.Fatal("blob-target decoded with no hash")
		}
		// Their hashes are the SELF-DESCRIBING wire form (format varint then
		// digest), never a fixed 32 bytes — SHARE-5.
		if got.Hash.String() != mustParse(t, wantHash).String() {
			t.Errorf("target hash: got %s, want %s", got.Hash.String(), wantHash)
		}
		if got.Path != "" {
			t.Errorf("blob-target also carries a path %q; the union has one arm", got.Path)
		}
	case entitysdk.ShareTargetPrefix:
		if got.Path != wantPath {
			t.Errorf("target path: got %q, want %q", got.Path, wantPath)
		}
		if got.Hash != nil && !got.Hash.IsZero() {
			t.Errorf("prefix-target also carries a hash; the union has one arm")
		}
	}
}

func mustParse(t *testing.T, hexStr string) hash.Hash {
	t.Helper()
	h, err := hash.ParseHex(hexStr)
	if err != nil {
		t.Fatalf("parse hash %q: %v", hexStr, err)
	}
	return h
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
