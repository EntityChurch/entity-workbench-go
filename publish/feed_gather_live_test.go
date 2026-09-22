// feed_gather_live_test.go — a real peer gathers a real peer's published
// feed, and the bytes land in the gatherer's own tree.
//
// # What this measures that `workbench/feed_gather_test.go` cannot
//
// The planner gates are pure, so they establish the RULES: byte fidelity
// against a fixture our encoder would not emit, gather-order paging, a sealed
// page that does not move, an unsigned entry carried rather than dropped. Every
// one of them hands [workbench.PlanMirror] a set somebody built.
//
// This is the **wiring**, and it is a different failure class — `AP108`, the one
// that cost this arc a session already. A gather has to cross the road chooser,
// a verifying consumer, a minted capability over somebody else's namespace, and
// the tree handler's put pre-check. Each of those can be wrong while every pure
// gate stays green, and the resulting mirror is durable, signed by us, and
// indistinguishable from a good one.
//
// # What a green run does NOT claim
//
// Nothing cross-implementation: one gatherer, ours, against one publisher,
// ours. Nothing about a THIRD party reading the mirror — that is
// `TestGatherLive_WhatAThirdPartyCanActuallyReach`, which measures the reach
// rather than asserting it, because the answer is the finding.
package publish_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"
)

// gatherLive stands the publisher/gatherer pair up and runs one gather
// through the shipped road.
func gatherLive(t *testing.T, posts int, opts workbench.GatherOpts) (
	liveFeedPair, *workbench.BrowseModel, workbench.MirrorPlan) {
	t.Helper()

	pair := newLiveFeedPair(t, posts)
	m := workbench.NewBrowseModel(nil)
	m.SetPeer(pair.reader)

	plan, err := workbench.GatherTimeline(context.Background(), m, pair.publisher.PeerID(), opts)
	if err != nil {
		t.Fatalf("GatherTimeline: %v", err)
	}
	return pair, m, plan
}

// TestGatherLive_RepublishesTheAuthorsExactBytesUnderTheAuthorsName is the
// first two hops of `DX-C1`: published at A, republished at B, **every entity
// hash byte-identical at both hops**.
//
// The two assertions are deliberately separate. The hash matching is what a
// consumer checks; the BYTES matching is what §2.1 requires, and §6.1's own
// hazard note says why they are not the same check — *"if an implementation's
// entity equality compares content hash only — the right default nearly
// everywhere — it is the wrong granularity here and hides this entire class."*
func TestGatherLive_RepublishesTheAuthorsExactBytesUnderTheAuthorsName(t *testing.T) {
	ctx := context.Background()

	// The publisher authors three entries through our own encoder AND one
	// our encoder would never emit. The fourth is the only one that can
	// fail this test, and it is the reason the test is not vacuous.
	var planted hash.Hash
	pair := newLiveFeedPairSeeded(t, 3, "app/feed/", func(pub *entitysdk.AppPeer) {
		planted = undeclaredFieldEntry(t, pub)
	})
	m := workbench.NewBrowseModel(nil)
	m.SetPeer(pair.reader)
	plan, err := workbench.GatherTimeline(ctx, m, pair.publisher.PeerID(), workbench.GatherOpts{})
	if err != nil {
		t.Fatalf("GatherTimeline: %v", err)
	}

	author := pair.publisher.PeerID()
	gatherer := pair.reader.PeerID()

	want := len(pair.posts) + 1
	if plan.EntryCount() != want {
		t.Fatalf("the gather planned %d entries and the author published %d\nnotes: %v\nskipped: %v",
			plan.EntryCount(), want, plan.Notes, plan.Skipped)
	}
	if plan.Unattributed != 0 {
		t.Errorf("%d entries were gathered with no signature, and this publisher signed all of "+
			"them — so the gatherer could not reach evidence it should have reached", plan.Unattributed)
	}

	write, err := workbench.WriteMirror(ctx, pair.reader, plan)
	if err != nil {
		t.Fatalf("WriteMirror: %v", err)
	}
	if write.Carried != len(plan.Carried) {
		t.Fatalf("wrote %d of %d carried entities", write.Carried, len(plan.Carried))
	}

	// ⭐ The planted entry first, because it is the only one whose bytes a
	// re-encoding gatherer would move. Asserted by name rather than folded
	// into the loop below so that a failure says which property broke.
	plantedPath := entitysdk.FeedEntryTreePath(author, planted)
	origPlanted, ok := pair.publisher.Store().Get(plantedPath)
	if !ok {
		t.Fatalf("the publisher does not hold the planted entry at %s", plantedPath)
	}
	mirrPlanted, ok := pair.reader.Store().Get(plantedPath)
	if !ok {
		t.Fatalf("the gatherer did not carry the planted entry (%s)", plantedPath)
	}
	if !bytes.Equal(mirrPlanted.Data, origPlanted.Data) {
		t.Errorf("§2.1: the entry carrying an undeclared field was RE-ENCODED on republication.\n"+
			"  published    %x\n  republished  %x\n"+
			"The field this build does not declare was dropped, the hash moved, and the author's "+
			"detached signature no longer names these bytes — so this mirror serves a verifiable "+
			"entry that nobody wrote.", origPlanted.Data, mirrPlanted.Data)
	}

	// Hop 2: what is in the GATHERER's tree, read back out of it, against
	// what the AUTHOR published. Read through the store rather than through
	// a dispatch, because a peer-qualified dispatched read of `/{author}/…`
	// goes to the author (`AP11`) and would compare the author's tree with
	// itself — green whether or not a single byte was ever republished.
	for _, post := range pair.posts {
		entPath := entitysdk.FeedEntryTreePath(author, post.Hash)

		original, ok := pair.publisher.Store().Get(entPath)
		if !ok {
			t.Fatalf("reading the author's own %s: not bound", entPath)
		}
		mirrored, ok := pair.reader.Store().Get(entPath)
		if !ok {
			t.Fatalf("the gatherer did not bind %s — foreign bytes live under their AUTHOR's "+
				"namespace, so this is the address a consumer's own reader will ask for", entPath)
		}

		if mirrored.ContentHash != original.ContentHash {
			t.Errorf("%s: hash moved across the hop, %s -> %s", entPath,
				original.ContentHash, mirrored.ContentHash)
		}
		if !bytes.Equal(mirrored.Data, original.Data) {
			t.Errorf("%s: the republished body is not byte-identical to the published one.\n"+
				"  published    %x\n  republished  %x", entPath, original.Data, mirrored.Data)
		}

		// §2.3 rule 2: the entry travelled with its author's detached
		// signature, at the invariant pointer under the AUTHOR.
		sigPath := entitysdk.FeedSignatureTreePath(author, post.Hash)
		origSig, ok := pair.publisher.Store().Get(sigPath)
		if !ok {
			t.Fatalf("the author has no signature at %s", sigPath)
		}
		mirrSig, ok := pair.reader.Store().Get(sigPath)
		if !ok {
			t.Fatalf("the entry was republished without its signature (%s): a mirror that carries "+
				"integrity and not authorship makes every entry unattributable, and a reader "+
				"cannot tell that from an author who never signed", sigPath)
		}
		if mirrSig.ContentHash != origSig.ContentHash {
			t.Errorf("%s: the carried signature is not the author's own bytes", sigPath)
		}
	}

	// `DX-R13`: nothing was rebound under the gatherer's own name.
	for _, post := range pair.posts {
		underUs := entitysdk.FeedEntryTreePath(gatherer, post.Hash)
		if _, ok := pair.reader.Store().Get(underUs); ok {
			t.Errorf("the gatherer bound %s under its OWN namespace — an entry found under a peer's "+
				"namespace is that peer's authorship claim (§1.1), so this republishes somebody "+
				"else's post as ours", underUs)
		}
	}
}

// TestGatherLive_TheMirrorIsBoundAtTheDerivedCoordinate checks the half a
// reader depends on: a mirror is found by **computing** its address from the
// subject, never by discovering it.
//
// §6.0.1 makes the coordinate derive-to-meet, and `FEED-12`'s failure mode is
// that a drift fails nothing — two peers simply write at different keys and
// never find each other. So the address is recomputed here from the subject
// alone, the way a stranger would, rather than read back off the plan.
func TestGatherLive_TheMirrorIsBoundAtTheDerivedCoordinate(t *testing.T) {
	pair, _, plan := gatherLive(t, 3, workbench.GatherOpts{})
	ctx := context.Background()

	if _, err := workbench.WriteMirror(ctx, pair.reader, plan); err != nil {
		t.Fatalf("WriteMirror: %v", err)
	}

	subject := entitysdk.MirrorSubjectTimeline(pair.publisher.PeerID())
	headPath, err := entitysdk.FeedMirrorTreePath(pair.reader.PeerID(), subject)
	if err != nil {
		t.Fatalf("deriving the mirror head path: %v", err)
	}
	ent, ok := pair.reader.Store().Get(headPath)
	if !ok {
		t.Fatalf("no mirror head at the derived coordinate %s", headPath)
	}
	head, err := entitysdk.FeedMirrorFromEntity(ent)
	if err != nil {
		t.Fatalf("decoding the mirror head: %v", err)
	}
	if head.GatheredBy != pair.reader.PeerID() {
		t.Errorf("gathered_by=%q, want the gatherer %q", head.GatheredBy, pair.reader.PeerID())
	}

	// The subject round-trips back to the peer it is of. A mirror whose
	// subject cannot be read back is one no second gatherer can merge with.
	got, err := entitysdk.MirrorSubjectFromReference(head.Subject)
	if err != nil {
		t.Fatalf("the head's subject does not read back as one: %v", err)
	}
	if !got.IsTimeline() || got.Peer != pair.publisher.PeerID() {
		t.Errorf("the head's subject is %+v, want a timeline of %s", got, pair.publisher.PeerID())
	}

	for p := uint64(0); p <= head.Current; p++ {
		path, err := entitysdk.FeedMirrorPageTreePath(pair.reader.PeerID(), subject, p)
		if err != nil {
			t.Fatalf("page path: %v", err)
		}
		pent, ok := pair.reader.Store().Get(path)
		if !ok {
			t.Fatalf("the head names page %d and %s is not bound", p, path)
		}
		pg, err := entitysdk.FeedMirrorPageFromEntity(pent)
		if err != nil {
			t.Fatalf("decoding page %d: %v", p, err)
		}
		if pg.Page != p {
			t.Errorf("page at key %d carries page=%d; §6.0a MUSTs the field equal its key", p, pg.Page)
		}
	}
}

// TestGatherLive_ASecondGatherOfAnUnchangedFeedIsANoOp is the **cross-run**
// half of `FEED-R32`, and it is the one property the other seat's gatherer
// cannot reach.
//
// `entity-browser-rust`'s `--gather` projects a directory in one shot and
// passes `&[]` for the prior pages — named in their source as a stated bound —
// so a second gather of one subject re-pages from scratch there. We write into
// a tree, so [workbench.LoadMirror] reads the prior view back and the gather is
// resumable. That is not a better implementation; it is a different substrate,
// and this is the gate that proves the difference is real rather than assumed.
func TestGatherLive_ASecondGatherOfAnUnchangedFeedIsANoOp(t *testing.T) {
	pair, m, first := gatherLive(t, 3, workbench.GatherOpts{})
	ctx := context.Background()

	if _, err := workbench.WriteMirror(ctx, pair.reader, first); err != nil {
		t.Fatalf("first WriteMirror: %v", err)
	}
	subject := entitysdk.MirrorSubjectTimeline(pair.publisher.PeerID())
	headPath, _ := entitysdk.FeedMirrorTreePath(pair.reader.PeerID(), subject)
	before, ok := pair.reader.Store().Get(headPath)
	if !ok {
		t.Fatal("the mirror head is not bound after the first write")
	}

	second, err := workbench.GatherTimeline(ctx, m, pair.publisher.PeerID(), workbench.GatherOpts{})
	if err != nil {
		t.Fatalf("second GatherTimeline: %v", err)
	}
	if len(second.Carried) != 0 {
		t.Errorf("the second gather carries %d entities for a feed that did not change — the prior "+
			"view was not loaded, so every run re-binds the whole archive", len(second.Carried))
	}
	if second.EntryCount() != first.EntryCount() {
		t.Errorf("the entry count moved %d -> %d across two gathers of one unchanged feed",
			first.EntryCount(), second.EntryCount())
	}

	if _, err := workbench.WriteMirror(ctx, pair.reader, second); err != nil {
		t.Fatalf("second WriteMirror: %v", err)
	}
	after, ok := pair.reader.Store().Get(headPath)
	if !ok {
		t.Fatal("the mirror head vanished across the second write")
	}
	if before.ContentHash != after.ContentHash {
		t.Errorf("the mirror head moved (%s -> %s) with nothing gathered. It is bound in our tree, "+
			"so a moved head moves our published root, which republishes an unchanged view to "+
			"every consumer holding it", before.ContentHash, after.ContentHash)
	}
}

// TestGatherLive_ARefusedEntryIsNotRepublished is `FEED-R1` carried through to
// the republish decision, and it is the arm that separates *reading* a forgery
// from *propagating* one.
//
// The reader already refuses an entry claiming an author other than the
// namespace it was found under, and keeps the row. A gatherer that carried
// that row would be binding, under the claimed author's namespace, bytes that
// author never wrote — which is the forgery succeeding one hop later, with our
// signed mirror record vouching for the view that holds it.
func TestGatherLive_ARefusedEntryIsNotRepublished(t *testing.T) {
	pair := newLiveFeedPairSeeded(t, 2, "app/feed/", func(pub *entitysdk.AppPeer) {
		seedForeignAuthoredEntry(t, pub)
	})
	ctx := context.Background()

	m := workbench.NewBrowseModel(nil)
	m.SetPeer(pair.reader)
	plan, err := workbench.GatherTimeline(ctx, m, pair.publisher.PeerID(), workbench.GatherOpts{})
	if err != nil {
		t.Fatalf("GatherTimeline: %v", err)
	}

	if len(plan.Skipped) == 0 {
		t.Fatal("nothing was refused, so either the forged entry was not reached or it was " +
			"republished — and this test cannot tell those apart without the skip")
	}
	var forgery bool
	for _, s := range plan.Skipped {
		if strings.Contains(s.Why, "FEED-R1") {
			forgery = true
		}
	}
	if !forgery {
		t.Errorf("entries were skipped but none for `FEED-R1`: %+v", plan.Skipped)
	}

	// The anti-vacuity half: the honest entries still came through. A
	// gatherer that refused everything would satisfy the assertion above.
	if plan.EntryCount() != len(pair.posts) {
		t.Errorf("the gather carried %d entries and the author honestly published %d — refusing a "+
			"forgery must not shorten the view of everything around it",
			plan.EntryCount(), len(pair.posts))
	}
}

// seedForeignAuthoredEntry plants a well-formed entry authored by a THIRD peer
// into the publisher's own entry prefix and names it from the publisher's index
// — which is what a publisher serving somebody else's bytes under its own
// namespace looks like on the wire.
//
// Planted from the seed hook so it is inside the minted root: a forgery the
// signed root does not commit to is one the reader never reaches, and the test
// would pass by never meeting the case.
func seedForeignAuthoredEntry(t *testing.T, pub *entitysdk.AppPeer) {
	t.Helper()

	stranger, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { stranger.Close() })

	forged := entitysdk.FeedEntryData{
		Author:    stranger.PeerID(),
		CreatedAt: 1_700_000_500_000,
		Body:      textEmbedFor(t, "planted under somebody else's namespace"),
	}
	ent, err := forged.ToEntity()
	if err != nil {
		t.Fatalf("encode the planted entry: %v", err)
	}
	pubID := pub.PeerID()
	if _, err := pub.Store().Put(
		entitysdk.FeedEntryTreePath(pubID, ent.ContentHash), ent.Type, forged); err != nil {
		t.Fatalf("plant the entry: %v", err)
	}

	pageEnt, ok := pub.Store().Get(entitysdk.FeedIndexPageTreePath(pubID, 0))
	if !ok {
		t.Fatal("page 0 is not in the publisher's tree")
	}
	page, err := entitysdk.FeedIndexPageFromEntity(pageEnt)
	if err != nil {
		t.Fatal(err)
	}
	page.Entries = append([]entitysdk.EntityRef{entitysdk.PinnedRef(pubID, ent.ContentHash)},
		page.Entries...)
	if _, err := pub.Store().Put(
		entitysdk.FeedIndexPageTreePath(pubID, 0), entitysdk.TypeFeedIndexPage, page); err != nil {
		t.Fatalf("rewrite page 0: %v", err)
	}
}

// undeclaredFieldEntry is an entry the publisher authors **carrying a field
// this build does not declare**, planted into their own tree and index.
//
// ⛔ **The live leg needs this and did not have it, and the omission made the
// byte-fidelity assertion measure nothing.** Measured: with the publisher's
// entries all authored by [entitysdk.FeedAuthor], a gatherer mutated to decode
// each entry through `FeedEntryData` and bind the re-encoding back **passed the
// whole live suite** — because decode-and-re-encode is lossless over exactly
// the bytes our own encoder produced.
//
// That is `DX-R9` in as many words — *a check exercising §2.1 MUST use input
// the implementation's own encoder would not emit* — and `DX-C2a` exists as its
// own conformance row because this check *"has a documented history of passing
// while measuring nothing"*. It has one more instance now, found here, in the
// gate written to honour it.
//
// The entry is authored and signed by the publisher, so `FEED-R1` and `FEED-R4`
// are satisfied and the only thing unusual about it is the extra field.
func undeclaredFieldEntry(t *testing.T, pub *entitysdk.AppPeer) hash.Hash {
	t.Helper()
	pubID := pub.PeerID()

	raw, err := ecf.Encode(struct {
		Author    string              `cbor:"author"`
		CreatedAt uint64              `cbor:"created_at"`
		Body      entitysdk.EmbedNode `cbor:"body"`
		Lang      string              `cbor:"lang"`
	}{
		Author:    pubID,
		CreatedAt: 1_700_000_900_000,
		Body:      textEmbedFor(t, "an entry carrying a field this build has never heard of"),
		Lang:      "cy",
	})
	if err != nil {
		t.Fatalf("encoding the undeclared-field entry: %v", err)
	}
	ent, err := entity.NewEntity(entitysdk.TypeFeedEntry, raw)
	if err != nil {
		t.Fatalf("building the undeclared-field entry: %v", err)
	}

	// The premise, asserted at the point of planting: our own encoder would
	// not emit these bytes. Without it the fixture silently degrades into
	// one that proves nothing, which is the whole hazard.
	decoded, err := entitysdk.FeedEntryFromEntity(ent)
	if err != nil {
		t.Fatalf("the planted entry must still decode as an entry: %v", err)
	}
	renormalized, err := decoded.ToEntity()
	if err != nil {
		t.Fatalf("re-encoding: %v", err)
	}
	if renormalized.ContentHash == ent.ContentHash {
		t.Fatal("the planted entry round-trips through our own type unchanged, so it does not " +
			"carry anything we fail to declare and every byte-preservation assertion in this file " +
			"is vacuous (`DX-R9`)")
	}

	if _, err := pub.PutEntity(entitysdk.FeedEntryTreePath(pubID, ent.ContentHash), ent); err != nil {
		t.Fatalf("plant the entry: %v", err)
	}
	kp := pub.RawPeer().Keypair()
	sigEnt, _, err := entitysdk.MintEntrySignature(&kp, pub.RawPeer().Identity(), ent.ContentHash)
	if err != nil {
		t.Fatalf("mint the planted entry's signature: %v", err)
	}
	if _, err := pub.PutEntity(entitysdk.FeedSignatureTreePath(pubID, ent.ContentHash), sigEnt); err != nil {
		t.Fatalf("bind the planted entry's signature: %v", err)
	}

	pageEnt, ok := pub.Store().Get(entitysdk.FeedIndexPageTreePath(pubID, 0))
	if !ok {
		t.Fatal("page 0 is not in the publisher's tree")
	}
	page, err := entitysdk.FeedIndexPageFromEntity(pageEnt)
	if err != nil {
		t.Fatal(err)
	}
	page.Entries = append([]entitysdk.EntityRef{entitysdk.PinnedRef(pubID, ent.ContentHash)},
		page.Entries...)
	if _, err := pub.Store().Put(
		entitysdk.FeedIndexPageTreePath(pubID, 0), entitysdk.TypeFeedIndexPage, page); err != nil {
		t.Fatalf("rewrite page 0: %v", err)
	}
	return ent.ContentHash
}
