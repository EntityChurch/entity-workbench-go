// The gathering loop's own rules — `SYSTEM-DATA-EXCHANGE` §2.1, §2.3 and
// §2.5, driven against [PlanMirror], which is pure.
//
// **Pure on purpose.** §6.0a's rules are properties of a SEQUENCE of writes, so
// the thing that has to be gated is the sequence, and a harness needing two
// peers and a network to ask *"did page 3's bytes move"* would ask it once. The
// end-to-end leg is `publish/feed_gather_live_test.go` and it measures a
// different thing: that the road, the capability and the store agree.
package workbench

import (
	"bytes"
	"testing"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"

	"entity-workbench-go/entitysdk"
)

const (
	gatherTestAuthor   = "2AliceExamplePeerIdForKeyVectors"
	gatherTestGatherer = "2BobExampleGathererPeerIdVector"
)

// foreignEntry is an `app/feed/entry` **carrying a field this build has never
// heard of** — which is exactly the fixture `DX-R9` makes mandatory.
//
// §2.1.1: *"a check exercising §2.1 MUST use input the implementation's own
// encoder would not emit"*, because **a round trip through bytes your own
// encoder produced proves nothing** — decode-and-re-encode is lossless over
// exactly that input, so a closure gate built from self-generated fixtures
// stays green with byte preservation removed, and is then worse than no gate
// because it is counted.
//
// `lang` is legal content the core protocol obliges us to ignore and preserve.
// It is the realistic shape too: a gatherer aggregates types it did not write,
// so an unrecognized field is the normal case and not an exotic one.
type foreignEntry struct {
	Author    string              `cbor:"author"`
	CreatedAt uint64              `cbor:"created_at"`
	Body      entitysdk.EmbedNode `cbor:"body"`
	Lang      string              `cbor:"lang"`
}

func mustForeignEntry(t *testing.T, author, text string, createdAt uint64) entity.Entity {
	t.Helper()
	raw, err := ecf.Encode(foreignEntry{
		Author:    author,
		CreatedAt: createdAt,
		Body: entitysdk.NewEmbedNode("text/plain",
			entitysdk.InlinePayload([]byte(text)), text),
		Lang: "cy",
	})
	if err != nil {
		t.Fatalf("encoding the foreign fixture: %v", err)
	}
	ent, err := entity.NewEntity(entitysdk.TypeFeedEntry, raw)
	if err != nil {
		t.Fatalf("building the foreign fixture: %v", err)
	}
	return ent
}

// TestGather_TheFixtureIsOneOurOwnEncoderWouldNotEmit is **`DX-C2a`, and it
// runs before anything else on purpose.**
//
// The exchange document flags this as the check most likely to be built wrong
// and gives it its own conformance row rather than a note inside `DX-C2`:
// *"`DX-C2` passes with byte preservation removed, and is then worse than no
// check because it is counted."* So the guard asserts the premise the whole
// byte-fidelity argument rests on — that **normalizing this input moves the
// hash** — and every assertion in the next test is vacuous without it.
func TestGather_TheFixtureIsOneOurOwnEncoderWouldNotEmit(t *testing.T) {
	foreign := mustForeignEntry(t, gatherTestAuthor, "bore da", 1700000001)

	decoded, err := entitysdk.FeedEntryFromEntity(foreign)
	if err != nil {
		t.Fatalf("the fixture must still be a legal entry our reader accepts, or it is measuring "+
			"malformedness rather than an undeclared field: %v", err)
	}
	renormalized, err := decoded.ToEntity()
	if err != nil {
		t.Fatalf("re-encoding the decoded entry: %v", err)
	}

	if renormalized.ContentHash == foreign.ContentHash {
		t.Fatal("decoding this fixture through our own type and re-encoding it produced the SAME " +
			"hash, so our encoder WOULD emit these bytes — the fixture does not carry anything we " +
			"fail to declare, and every byte-preservation assertion built on it is vacuous. " +
			"`DX-R9` exists for exactly this: pick input this build cannot round-trip")
	}
	if bytes.Equal(renormalized.Data, foreign.Data) {
		t.Fatal("the re-encoded body is byte-identical while the hashes differ, which cannot happen — " +
			"the fixture is not measuring what it claims")
	}
}

// TestGather_CarriesObtainedBytesAndNeverAReEncoding is **`DX-C2`**, the check
// `SYSTEM-DATA-EXCHANGE` exists for.
//
// §2.1: a republished entity MUST be bound byte-identically to the form in
// which it was obtained, and an implementation MUST NOT re-encode it
// *"including by decoding it through a type it does not fully declare."* The
// failure is not a lost field — it is that the hash moves, the detached
// signature stops naming the entity, and the output is a complete, verifiable
// publication **in which nobody wrote anything**.
func TestGather_CarriesObtainedBytesAndNeverAReEncoding(t *testing.T) {
	foreign := mustForeignEntry(t, gatherTestAuthor, "bore da", 1700000001)
	sig := mustSignatureStandIn(t, foreign.ContentHash)

	plan, err := PlanMirror(gatherTestGatherer, gatherTestAuthor,
		entitysdk.MirrorSubjectTimeline(gatherTestAuthor),
		[]GatheredEntry{{
			Hash: foreign.ContentHash, Entity: foreign, Signature: &sig, CreatedAt: 1700000001,
		}}, nil, 0, 1700000001)
	if err != nil {
		t.Fatalf("PlanMirror: %v", err)
	}

	var carried *CarriedEntity
	for i := range plan.Carried {
		if plan.Carried[i].Kind == CarriedEntry {
			carried = &plan.Carried[i]
		}
	}
	if carried == nil {
		t.Fatal("the plan carries no entry at all")
	}

	if !bytes.Equal(carried.Entity.Data, foreign.Data) {
		t.Errorf("the carried body is not the obtained body:\n obtained %x\n carried  %x",
			foreign.Data, carried.Entity.Data)
	}
	if carried.Entity.ContentHash != foreign.ContentHash {
		t.Errorf("the carried hash is %s and the obtained hash was %s — a moved hash means the "+
			"author's detached signature no longer names this entity, so every reader of this "+
			"mirror reports the AUTHOR as having published something nobody signed",
			carried.Entity.ContentHash, foreign.ContentHash)
	}

	// The undeclared field survived, which is §2.3 rule 4 — and it survives
	// *automatically*, because bytes were republished rather than
	// re-serialized. Asserted directly rather than inferred from the hash so
	// that a failure says which property broke.
	var back foreignEntry
	if err := ecf.Decode(carried.Entity.Data, &back); err != nil {
		t.Fatalf("decoding the carried body: %v", err)
	}
	if back.Lang != "cy" {
		t.Errorf("the undeclared field did not survive republication: lang=%q, want %q", back.Lang, "cy")
	}

	// And the entry is pinned on the page. §6.0 makes that structural: a
	// live reference would mirror whatever is at that address now, which is
	// not a mirror.
	if got := plan.EntryCount(); got != 1 {
		t.Fatalf("plan holds %d entries, want 1", got)
	}
	ref := plan.Pages[0].Entries[0]
	if !ref.IsPinned() {
		t.Errorf("the mirrored entry is a %q reference; §6.0 requires PINNED", ref.Tag)
	}
	if ref.Peer != gatherTestAuthor {
		t.Errorf("the mirrored entry names peer %q, want the AUTHOR %q — a mirror that renamed the "+
			"peer would attribute the entry to its republisher, which `DX-R13` forbids",
			ref.Peer, gatherTestAuthor)
	}
}

// mustSignatureStandIn builds an entity in the shape a detached signature
// arrives in. It is never verified here — [PlanMirror] is handed signatures
// that [GatherTimeline] already verified through the consumer, and the
// verification is gated where it happens (`publish/feed_gather_live_test.go`),
// not re-asserted against a hand-built double that could only agree with
// itself.
func mustSignatureStandIn(t *testing.T, target hash.Hash) entity.Entity {
	t.Helper()
	raw, err := ecf.Encode(map[string]any{"target": target.Bytes()})
	if err != nil {
		t.Fatalf("encoding the signature stand-in: %v", err)
	}
	ent, err := entity.NewEntity("system/signature", raw)
	if err != nil {
		t.Fatalf("building the signature stand-in: %v", err)
	}
	return ent
}

// TestGather_AnEntryWithNoSignatureIsCarriedAndNotDropped is **`DX-C5`**, and
// it needs both arms.
//
// §2.3 rule 3 binds the READER to present an entry with no surviving
// authorship evidence as unattributed. It does **not** licence a gatherer to
// drop it, and dropping is the wrong repair: **a mirror's only lie is
// omission**, so a gatherer that silently shortened its view would be making
// the one claim §6.1 rule 2 says the format must give it no way to make.
//
// The positive arm alone passes against a gatherer that drops everything
// unsigned and happens to have been handed a signed set.
func TestGather_AnEntryWithNoSignatureIsCarriedAndNotDropped(t *testing.T) {
	signed := mustForeignEntry(t, gatherTestAuthor, "signed", 1700000001)
	unsigned := mustForeignEntry(t, gatherTestAuthor, "unsigned", 1700000002)
	sig := mustSignatureStandIn(t, signed.ContentHash)

	plan, err := PlanMirror(gatherTestGatherer, gatherTestAuthor,
		entitysdk.MirrorSubjectTimeline(gatherTestAuthor),
		[]GatheredEntry{
			{Hash: signed.ContentHash, Entity: signed, Signature: &sig, CreatedAt: 1700000001},
			{Hash: unsigned.ContentHash, Entity: unsigned, CreatedAt: 1700000002},
		}, nil, 0, 1700000002)
	if err != nil {
		t.Fatalf("PlanMirror: %v", err)
	}

	if got := plan.EntryCount(); got != 2 {
		t.Fatalf("the mirror holds %d entries, want 2 — an entry whose signature could not be "+
			"obtained was DROPPED, which makes this mirror short about entries the author really "+
			"published and is the one thing a republisher must not do silently", got)
	}
	if plan.Unattributed != 1 {
		t.Errorf("plan reports %d unattributed, want 1", plan.Unattributed)
	}

	// The signed one travels with its signature; the unsigned one travels
	// with nothing. Nothing is synthesized — §2.3 rule 2 says a republishing
	// peer MUST NOT supply a signature, and we could not: we do not hold the
	// author's key.
	sigs := 0
	for _, c := range plan.Carried {
		if c.Kind == CarriedSignature {
			sigs++
		}
	}
	if sigs != 1 {
		t.Errorf("the plan carries %d signatures for 2 entries, want exactly 1 — a second one could "+
			"only have been invented, and a gatherer cannot author in the author's name", sigs)
	}
}

// TestGather_PublishesNoSetLayerObjectOfTheAuthors is **`DX-C6`** and it
// inspects what a plan BINDS.
//
// `DX-R4` forbids publishing, under our own namespace, an author's own
// set-layer object over content that author did not place there. The failure
// it guards is not obvious from the flat sentence: a gatherer that republished
// the author's `app/feed/index` would be asserting what that author's feed
// CONTAINS — a claim only the author can make and one the authorship
// instrument cannot carry, because signatures sign entries and not sets.
func TestGather_PublishesNoSetLayerObjectOfTheAuthors(t *testing.T) {
	a := mustForeignEntry(t, gatherTestAuthor, "one", 1700000001)
	b := mustForeignEntry(t, gatherTestAuthor, "two", 1700000002)

	plan, err := PlanMirror(gatherTestGatherer, gatherTestAuthor,
		entitysdk.MirrorSubjectTimeline(gatherTestAuthor),
		[]GatheredEntry{
			{Hash: a.ContentHash, Entity: a, CreatedAt: 1700000001},
			{Hash: b.ContentHash, Entity: b, CreatedAt: 1700000002},
		}, nil, 0, 1700000002)
	if err != nil {
		t.Fatalf("PlanMirror: %v", err)
	}

	for _, c := range plan.Carried {
		if isSetLayerKey(c.Key) {
			t.Errorf("the plan binds %s, which is one of the author's own set-layer objects", c.Key)
		}
		if c.Peer != gatherTestAuthor {
			t.Errorf("carried %s is bound under %q; foreign bytes live under their AUTHOR's "+
				"namespace, never the gatherer's", c.Key, c.Peer)
		}
	}
	// The gatherer's own output is the record and its pages, and those are
	// the gatherer's to sign. Asserted as the positive half so the test
	// cannot pass by a plan that binds nothing at all.
	if plan.Record.GatheredBy != gatherTestGatherer {
		t.Errorf("the mirror record says gathered_by=%q, want %q",
			plan.Record.GatheredBy, gatherTestGatherer)
	}
	if len(plan.Carried) == 0 {
		t.Error("the plan carries nothing, so the refusals above were not exercised")
	}
}

// TestGather_PagesFillInGatherOrderAndASealedPageDoesNotMove is **`DX-C8`'s
// writer half**, and it is the property no single-publisher fixture can see.
//
// §6.0a: pages are filled in gather order, a page is sealed when its successor
// opens, and a sealed page is never rewritten to insert an entry discovered
// later. Together with §2.5's *bounded head plus key-addressed pages* that is
// what makes a mirror incrementally readable — a reader that has read page 7
// never re-reads page 7 — and a head whose size grows with the member count
// makes *"one check instead of 500"* into a different expensive read.
//
// The measurement that matters is the **byte** comparison of page 0 across a
// second gather, not its length: a page rewritten with the same entries in the
// same order but a new `updated_at` has moved, invalidates every cache holding
// it, and would pass a length check.
func TestGather_PagesFillInGatherOrderAndASealedPageDoesNotMove(t *testing.T) {
	const pageSize = 2
	subject := entitysdk.MirrorSubjectTimeline(gatherTestAuthor)

	mk := func(n int, from uint64) []GatheredEntry {
		var out []GatheredEntry
		for i := 0; i < n; i++ {
			at := from + uint64(i)
			e := mustForeignEntry(t, gatherTestAuthor, string(rune('a'+i))+"-post", at)
			out = append(out, GatheredEntry{Hash: e.ContentHash, Entity: e, CreatedAt: at})
		}
		return out
	}

	first := mk(3, 1700000001)
	round1, err := PlanMirror(gatherTestGatherer, gatherTestAuthor, subject,
		first, nil, pageSize, GatheredClock(first))
	if err != nil {
		t.Fatalf("first gather: %v", err)
	}
	if len(round1.Pages) != 2 {
		t.Fatalf("3 entries at page size 2 produced %d pages, want 2 — a view that never opens a "+
			"second page cannot measure sealing at all", len(round1.Pages))
	}
	for i, pg := range round1.Pages {
		if pg.Page != uint64(i) {
			t.Errorf("page at index %d carries page=%d; §6.0a MUSTs the field equal its key, so a "+
				"page that is moved is detectably moved", i, pg.Page)
		}
	}
	if len(round1.Pages[0].Entries) != pageSize {
		t.Errorf("page 0 holds %d entries, want it filled to %d before its successor opened",
			len(round1.Pages[0].Entries), pageSize)
	}

	sealed, err := round1.Pages[0].ToEntity()
	if err != nil {
		t.Fatalf("encoding sealed page 0: %v", err)
	}

	// A second gather that discovers two more entries. §6.0a's rule is that
	// the ones discovered later go on the CURRENT page — never inserted into
	// a sealed one, however much a post-ordered view would prefer it.
	second := append(append([]GatheredEntry{}, first...), mk(2, 1700000010)...)
	round2, err := PlanMirror(gatherTestGatherer, gatherTestAuthor, subject,
		second, round1.Pages, pageSize, GatheredClock(second))
	if err != nil {
		t.Fatalf("second gather: %v", err)
	}

	resealed, err := round2.Pages[0].ToEntity()
	if err != nil {
		t.Fatalf("re-encoding page 0: %v", err)
	}
	if resealed.ContentHash != sealed.ContentHash {
		t.Errorf("page 0's bytes MOVED when the view was extended (%s -> %s). A sealed page is "+
			"immutable: it is what lets a reader keep a cursor and a cache across rounds, and "+
			"moving it makes every reader re-read the whole archive to learn nothing",
			sealed.ContentHash, resealed.ContentHash)
	}
	if round2.EntryCount() != 5 {
		t.Errorf("the extended view holds %d entries, want 5", round2.EntryCount())
	}

	// The head is fixed-size whatever the size of the view it heads (§2.5).
	// Measured as bytes against a one-entry view, because *"the head has no
	// entries field"* is true of a head that grows for any other reason too.
	small, err := PlanMirror(gatherTestGatherer, gatherTestAuthor, subject,
		first[:1], nil, pageSize, GatheredClock(first[:1]))
	if err != nil {
		t.Fatalf("one-entry gather: %v", err)
	}
	bigHead, err := round2.Record.ToEntity()
	if err != nil {
		t.Fatalf("encoding the large head: %v", err)
	}
	smallHead, err := small.Record.ToEntity()
	if err != nil {
		t.Fatalf("encoding the small head: %v", err)
	}
	if len(bigHead.Data) != len(smallHead.Data) {
		t.Errorf("a 5-entry view's head is %d bytes and a 1-entry view's is %d — the head is a "+
			"function of the member count, which is §2.5's failure: the cheap leg becomes the "+
			"expensive one and no single-publisher fixture can see it",
			len(bigHead.Data), len(smallHead.Data))
	}
	if round2.Record.Current != 2 {
		t.Errorf("head says current=%d, want 2", round2.Record.Current)
	}
}

// TestGather_ReGatheringAnUnchangedFeedChangesNothing is the idempotence that
// makes a gather safe to run on a schedule.
//
// Two things are asserted and they fail for different reasons: the entry set
// does not grow (an entry re-added under a second reference would be a
// duplicate the reader has no way to collapse), and **the record's bytes do not
// move** — which is [GatheredClock]'s whole argument. A wall-clock stamp passes
// the first and fails the second, silently moving the signed root on every run
// and invalidating every consumer's cached copy of a view whose content did not
// change.
func TestGather_ReGatheringAnUnchangedFeedChangesNothing(t *testing.T) {
	subject := entitysdk.MirrorSubjectTimeline(gatherTestAuthor)
	a := mustForeignEntry(t, gatherTestAuthor, "one", 1700000001)
	b := mustForeignEntry(t, gatherTestAuthor, "two", 1700000002)
	set := []GatheredEntry{
		{Hash: a.ContentHash, Entity: a, CreatedAt: 1700000001},
		{Hash: b.ContentHash, Entity: b, CreatedAt: 1700000002},
	}

	one, err := PlanMirror(gatherTestGatherer, gatherTestAuthor, subject, set, nil, 0, GatheredClock(set))
	if err != nil {
		t.Fatalf("first gather: %v", err)
	}
	two, err := PlanMirror(gatherTestGatherer, gatherTestAuthor, subject, set, one.Pages, 0, GatheredClock(set))
	if err != nil {
		t.Fatalf("second gather: %v", err)
	}

	if one.EntryCount() != two.EntryCount() {
		t.Errorf("re-gathering an unchanged feed moved the entry count %d -> %d",
			one.EntryCount(), two.EntryCount())
	}
	if len(two.Carried) != 0 {
		t.Errorf("the second gather carries %d entities for a feed that did not change — bytes "+
			"already in our tree are re-bound at the keys they already occupy", len(two.Carried))
	}

	oneHead, err := one.Record.ToEntity()
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}
	twoHead, err := two.Record.ToEntity()
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}
	if oneHead.ContentHash != twoHead.ContentHash {
		t.Errorf("two gathers of one unchanged feed produced two records (%s, %s) — the stamp is "+
			"reading a clock, so the signed root moves on every run and every consumer's cached "+
			"copy of an unchanged view is invalidated", oneHead.ContentHash, twoHead.ContentHash)
	}
	if one.Record.GatheredAt == 0 {
		t.Error("the stamp is zero, which would make the comparison above vacuous")
	}
	if one.Record.GatheredAt != 1700000002 {
		t.Errorf("gathered_at=%d, want the gathered set's high-water mark 1700000002",
			one.Record.GatheredAt)
	}
}

// TestGather_RefusesToGatherItself names the one configuration the closure
// property cannot mean.
//
// A mirror of ourselves would bind our own entries under our own namespace at
// the keys they already occupy, and publish a view whose subject is its own
// publisher. §6.2's *assembly as a by-product of participation* is about
// carrying what somebody ELSE published.
func TestGather_RefusesToGatherItself(t *testing.T) {
	e := mustForeignEntry(t, gatherTestGatherer, "mine", 1700000001)
	_, err := PlanMirror(gatherTestGatherer, gatherTestGatherer,
		entitysdk.MirrorSubjectTimeline(gatherTestGatherer),
		[]GatheredEntry{{Hash: e.ContentHash, Entity: e, CreatedAt: 1700000001}}, nil, 0, 1)
	if err == nil {
		t.Fatal("a peer gathered itself")
	}
}
