package entitysdk_test

// APP-CONVENTION-FEED §6 — the mirror vocabulary and §6.0.1's coordinate.
// Tier: cross-implementation conformance (TESTING-STRATEGY) for the coordinate
// arm, unit for the rest.
//
// ⭐ THE VECTOR IN `TestMirrorCoordinate_AgreesWithTheOtherImplementation` IS
// NOT OURS. It is `entity-browser-rust`'s `the_live_coordinate_is_pinned_to_a_literal`
// (`src/feed.rs`), whose expected value their file says is computed **with
// `hashlib` from the ECF framing rules, not with the function under test and not
// by calling the kernel**. So the agreement below is between two implementations,
// two languages and two kernels against one independently-derived literal —
// which per [ADR-0012] is evidence that is **not cohort-consistent**, the
// standard this tier is usually unable to meet.
//
// WHY IT MATTERS MORE THAN AN ORDINARY BYTE TEST. §6.0.1's coordinate is a
// `[derive-to-meet]` value: two peers compute it independently and must land on
// the same byte, with **nothing failing loudly** when they do not. A mismatch is
// not an error anywhere — the two simply write mirrors at different keys and
// never find each other's. That is `FEED-12`'s named failure mode, and a
// disagreement here is a WIRE EVENT rather than a test fix.
//
// ⚠ Their literal already moved once, on 2026-09-14: it was `00c11cf3…` under
// their `A-60` reading (`sha256(utf8(absolute))`) until `AT-110` ruled
// `prefix_hash` and `FEED-R33` made every other derivation a MUST NOT. It cost
// no migration because nothing had published a mirror. A later move will not be
// that cheap — check what is published first.

import (
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/ext/revision"

	"entity-workbench-go/entitysdk"
)

// Their fixture's peer id and expected coordinate, transcribed verbatim.
const (
	rustMirrorPeer  = "2AliceExamplePeerIdForKeyVectors"
	rustMirrorCoord = "004bba26751170e5329b232e70568e5b86e72d773532af2da7e7dd2ba14578856d"
)

func TestMirrorCoordinate_AgreesWithTheOtherImplementation(t *testing.T) {
	s := entitysdk.MirrorSubjectTimeline(rustMirrorPeer)

	got, err := s.Coordinate()
	if err != nil {
		t.Fatalf("coordinate: %v", err)
	}
	if got != rustMirrorCoord {
		t.Fatalf("THE LIVE-KEY DERIVATION DISAGREES ACROSS IMPLEMENTATIONS — this is a WIRE "+
			"event, not a test fix, and it is routed rather than locally corrected.\n"+
			"  ours   %s\n  theirs %s", got, rustMirrorCoord)
	}

	key, err := s.Key()
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	if want := "app/feed/mirrors/" + rustMirrorCoord; key != want {
		t.Errorf("key: got %q want %q (§6.0.1's prefix)", key, want)
	}

	// §3.1's shape claims, asserted rather than assumed: pinned to the
	// ECFv1-SHA-256 floor on every peer, whatever its home format.
	if len(got) != 66 {
		t.Errorf("§3.1 pins 66 characters; got %d", len(got))
	}
	if !strings.HasPrefix(got, "00") {
		t.Errorf("§3.1 pins the SHA-256 floor; got prefix %q", got[:2])
	}
}

// TestMirrorCoordinate_TheCallSiteHandsPrefixHashTheAbsolutePath is the arm that
// keeps the literal above attributable.
//
// The literal agreeing tells us the OUTPUT is right. It does not tell us WHY,
// and the thing most likely to drift is not the hash function — it is the string
// we hand it. Neuter the absolute-path assembly and a test spelled in terms of
// `PathCoordinate` still passes; this one reds.
func TestMirrorCoordinate_TheCallSiteHandsPrefixHashTheAbsolutePath(t *testing.T) {
	s := entitysdk.MirrorSubjectTimeline(rustMirrorPeer)
	got, err := s.Coordinate()
	if err != nil {
		t.Fatalf("coordinate: %v", err)
	}
	want := revision.PrefixHash("/" + rustMirrorPeer + "/app/feed/index")
	if got != want {
		t.Errorf("the coordinate is prefix_hash of the ABSOLUTE index path\n  got  %s\n  want %s",
			got, want)
	}
}

// TestMirrorCoordinate_ASlashInThePathIsNotDoubled pins the one piece of string
// handling `PathCoordinate` owns.
//
// A live reference stores its path with a leading slash (`refAbsolutePath`), the
// convention writes it without one (`app/feed/index`), and both spellings reach
// this function. They MUST derive the same coordinate: a gatherer that took one
// spelling and a reader that took the other would each be correct and would never
// meet, which is this whole file's failure mode arriving through a typo.
func TestMirrorCoordinate_ASlashInThePathIsNotDoubled(t *testing.T) {
	withSlash := entitysdk.PathCoordinate(rustMirrorPeer, "/app/feed/index")
	without := entitysdk.PathCoordinate(rustMirrorPeer, "app/feed/index")
	if withSlash != without {
		t.Errorf("the two spellings of one path derive different keys:\n  /app/… %s\n   app/… %s",
			withSlash, without)
	}
	if withSlash != rustMirrorCoord {
		t.Errorf("and neither is the ruled value: got %s want %s", withSlash, rustMirrorCoord)
	}
}

// TestMirrorSubject_DerivationReadsIdentifyingFieldsOnly is `FEED-R27`.
//
// ⭐ The compile-time half is the point. `entity-browser-rust` enforces this by
// naming every field of both reference arms in a destructuring match with the
// hints bound to `_`, so **adding a field to the atom is a compile error at the
// one site that decides what a coordinate is made of.** Go has no such match, so
// the unkeyed composite literal below stands in for it: an unkeyed literal must
// give every field positionally, and it stops compiling the moment `EntityRef`
// grows one. Whoever widens the atom lands here and is sent to
// `MirrorSubjectFromReference` to decide whether the new field is identifying.
//
// A `..`-shaped version of this — keyed fields, or no literal at all — is the
// version that silently starts ignoring a field somebody later decided mattered.
func TestMirrorSubject_DerivationReadsIdentifyingFieldsOnly(t *testing.T) {
	h := hashOfBytes(t, "a thread root")
	seen := hashOfBytes(t, "an expectation")

	// Positional: Tag, Peer, Hash, Path, Seen, At, Via.
	bare := entitysdk.EntityRef{entitysdk.RefTagLive, rustMirrorPeer, nil, "/app/feed/index", nil, nil, nil}
	hinted := entitysdk.EntityRef{entitysdk.RefTagLive, rustMirrorPeer, nil, "/app/feed/index", &seen,
		&entitysdk.RefAnchor{Field: []string{"body"}}, []entitysdk.RefHint{{Tag: "origin", Value: "x"}}}

	for _, c := range []struct {
		name string
		ref  entitysdk.EntityRef
	}{{"no hints", bare}, {"every hint set", hinted}} {
		s, err := entitysdk.MirrorSubjectFromReference(c.ref)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		got, err := s.Coordinate()
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got != rustMirrorCoord {
			t.Errorf("%s: an optional hint reached the derivation — got %s want %s",
				c.name, got, rustMirrorCoord)
		}
	}

	// And the pinned arm, whose key is the entity's own hash and NOT the peer's
	// — so two gatherers who learned of one thread from different peers meet.
	fromA, err := entitysdk.MirrorSubjectThread("peerA", h).Coordinate()
	if err != nil {
		t.Fatal(err)
	}
	fromB, err := entitysdk.MirrorSubjectThread("peerB", h).Coordinate()
	if err != nil {
		t.Fatal(err)
	}
	if fromA != fromB {
		t.Errorf("a thread's coordinate depends on who told us about it:\n  A %s\n  B %s", fromA, fromB)
	}
}

// TestMirrorPage_RefusesALiveEntry is §6.0's *entries is always PINNED*.
//
// The negative arm is the one that matters and the positive arm is its
// anti-vacuity control: without the pinned case passing, a `ToEntity` that
// refused everything would satisfy this test while making the type unusable.
func TestMirrorPage_RefusesALiveEntry(t *testing.T) {
	h := hashOfBytes(t, "an entry")

	ok := entitysdk.FeedMirrorPageData{
		Page:      0,
		Entries:   []entitysdk.EntityRef{entitysdk.PinnedRef(rustMirrorPeer, h)},
		UpdatedAt: 1_700_000_000_000,
	}
	if _, err := ok.ToEntity(); err != nil {
		t.Fatalf("a pinned entry must encode: %v", err)
	}

	bad := entitysdk.FeedMirrorPageData{
		Page:      0,
		Entries:   []entitysdk.EntityRef{entitysdk.LiveRef(rustMirrorPeer, "app/feed/entries/x", hash.Hash{})},
		UpdatedAt: 1_700_000_000_000,
	}
	if _, err := bad.ToEntity(); err == nil {
		t.Error("a LIVE entry was accepted onto a mirror page; §6.0 requires every mirrored " +
			"entry to be pinned, because a mirror carries exact bytes and a live reference " +
			"would mirror whatever is at that address now")
	}
}

// TestMirrorHead_HasNoCompletenessField is `DX-R14` / §6.1 rule 2, and it is a
// gate on the SHAPE rather than on a value.
//
// The rule is that a republication format MUST NOT PROVIDE A FIELD in which
// completeness can be claimed — so the thing to assert is that no such key
// reaches the wire, not that some flag is false. Asserted over the encoded bytes
// because that is where a field either exists or does not.
func TestMirrorHead_HasNoCompletenessField(t *testing.T) {
	s := entitysdk.MirrorSubjectTimeline(rustMirrorPeer)
	d := entitysdk.FeedMirrorData{
		Subject:    s.Reference(),
		Current:    2,
		GatheredAt: 1_700_000_000_000,
		GatheredBy: "2GathererPeerId",
	}
	ent, err := d.ToEntity()
	if err != nil {
		t.Fatalf("encode mirror head: %v", err)
	}
	for _, forbidden := range []string{"complete", "exhaustive", "total", "all"} {
		if strings.Contains(string(ent.Data), forbidden) {
			t.Errorf("the encoded mirror head carries a %q key; §6.1 rule 2 says the shape "+
				"gives a gatherer no way to claim completeness", forbidden)
		}
	}

	// Round trip, so the type is actually usable and this file is not asserting
	// the absence of a field on a type nobody can encode.
	back, err := entitysdk.FeedMirrorFromEntity(ent)
	if err != nil {
		t.Fatalf("decode mirror head: %v", err)
	}
	if back.Current != 2 || back.GatheredBy != "2GathererPeerId" {
		t.Errorf("round trip lost a field: %+v", back)
	}
}

// TestMirrorHead_RefusesAnOldestAboveCurrent mirrors the index head's check, for
// the same reason: a head that says the lowest page still published is above the
// highest in use describes no view, and the failure is otherwise a reader
// walking an empty range and reporting the gatherer as having gathered nothing.
func TestMirrorHead_RefusesAnOldestAboveCurrent(t *testing.T) {
	s := entitysdk.MirrorSubjectTimeline(rustMirrorPeer)
	oldest := uint64(5)
	d := entitysdk.FeedMirrorData{
		Subject:    s.Reference(),
		Current:    2,
		Oldest:     &oldest,
		GatheredAt: 1,
		GatheredBy: "g",
	}
	if _, err := d.ToEntity(); err == nil {
		t.Error("oldest=5 current=2 was accepted")
	}
}

// TestMirrorPageKey_HangsOffTheHeadKey pins §6.0a's addressing: decimal,
// key-addressed, and derived from the head so the two cannot drift.
func TestMirrorPageKey_HangsOffTheHeadKey(t *testing.T) {
	s := entitysdk.MirrorSubjectTimeline(rustMirrorPeer)
	head, err := s.Key()
	if err != nil {
		t.Fatal(err)
	}
	page, err := s.PageKey(7)
	if err != nil {
		t.Fatal(err)
	}
	if want := head + "/7"; page != want {
		t.Errorf("page key: got %q want %q", page, want)
	}
	if !strings.HasPrefix(page, "app/feed/mirrors/") {
		t.Errorf("page key left §6.0.1's prefix: %q", page)
	}
}

func hashOfBytes(t *testing.T, s string) hash.Hash {
	t.Helper()
	raw, err := ecf.Encode(s)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	h, err := hash.Compute("app/feed/entry", raw)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	return h
}
