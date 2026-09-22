package publish_test

import (
	"context"
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/hash"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"
)

// feed_reader_live_test.go — one peer READS another peer's feed, through
// the signed root, with every reader-side rule the convention puts on it.
//
// # What this is against what already existed
//
// `feed_live_test.go` established that a published feed is REACHABLE: the
// pinned keys are in the committed set and the entries decode. That is a
// statement about the publisher. This is the reader: `workbench.ReadFeed`
// resolving an index, attributing entries, refusing a forged one, falling
// back when the index is not there, and resuming when the position it was
// holding has been removed.
//
// # The rules driven here, each by the condition that defines it
//
//	FEED-R1   an entry whose `author` is not the namespace → REJECTED
//	FEED-R4   an entry with no verified signature → UNATTRIBUTED, not hidden
//	FEED-R13  the index is not the authority: remove it, get the same entries
//	FEED-R14  the cursor's `applied` is gone → resume from `page`
//
// # What a green run does NOT claim
//
// Nothing cross-implementation: one reader, ours, against one publisher,
// ours. Nothing about a subscription — following is pull-only by §2.4 and
// this reads. And nothing about a STATIC corridor: attribution here works
// because a live grant carries `system/signature/*`, which no
// feed-shaped published prefix contains (see
// `TestFeedLive_TheSignaturesAreNotInTheSignedRoot`).

func TestFeedReader_ReadsAnotherPeersFeedThroughTheSignedRoot(t *testing.T) {
	pair := newLiveFeedPair(t, 5)
	ctx := context.Background()

	c, err := workbench.NewPeerConsumer(pair.reader, pair.publisher.PeerID(), nil)
	if err != nil {
		t.Fatalf("NewPeerConsumer: %v", err)
	}
	read, err := workbench.ReadFeed(ctx, c, workbench.FeedReadOpts{})
	if err != nil {
		t.Fatalf("ReadFeed: %v", err)
	}

	if read.Via != workbench.FeedViaIndex {
		t.Fatalf("read via %q, want the index — the fallback is correct and costs O(all), so taking it "+
			"when an index is present means the cheap path is broken and nothing says so", read.Via)
	}
	if len(read.Entries) != len(pair.posts) {
		t.Fatalf("read %d entries, want %d\nnotes: %v", len(read.Entries), len(pair.posts), read.Notes)
	}
	// Newest first, across pages. The publisher posted oldest-first and
	// pages FILL oldest-first while entries within a page are listed
	// newest-first, so the concatenation a reader sees is newest-first
	// overall — the arithmetic a harness that only re-reads its own index
	// cannot see.
	newest := pair.posts[len(pair.posts)-1]
	if read.Entries[0].Hash != newest.Hash {
		t.Errorf("first entry is %s, want the newest post %s", read.Entries[0].Hash, newest.Hash)
	}
	for i, e := range read.Entries {
		if e.Problem != "" {
			t.Errorf("entry %d: %s", i, e.Problem)
		}
		if !e.Attributed {
			// FEED-R4 the other way round: these entries ARE signed, so an
			// unattributed one here means the reader cannot reach the
			// signature it should be able to reach.
			t.Errorf("entry %d is unattributed and its author signed it: %s", i, e.Attribution)
		}
		if !e.Listed {
			t.Errorf("entry %d did not come from the index", i)
		}
		if e.Text == "" {
			t.Errorf("entry %d has no body text", i)
		}
	}
	if read.Cursor.Applied != newest.Hash {
		t.Errorf("cursor applied=%s, want the newest entry %s — a cursor that does not advance makes "+
			"every later read O(all)", read.Cursor.Applied, newest.Hash)
	}
	if read.Freshness == "" {
		t.Error("the read carries no freshness sentence, so a surface has nothing to say about what " +
			"this reading is worth")
	}
}

// TestFeedReader_ResumesFromTheCursorAndFromItsPageWhenTheEntryIsGone is
// §4.3 rule 4 and `FEED-R14` in one run, because the second is only
// meaningful against a cursor the first established.
func TestFeedReader_ResumesFromTheCursorAndFromItsPageWhenTheEntryIsGone(t *testing.T) {
	pair := newLiveFeedPair(t, 5)
	ctx := context.Background()

	c, err := workbench.NewPeerConsumer(pair.reader, pair.publisher.PeerID(), nil)
	if err != nil {
		t.Fatalf("NewPeerConsumer: %v", err)
	}
	first, err := workbench.ReadFeed(ctx, c, workbench.FeedReadOpts{})
	if err != nil {
		t.Fatalf("first read: %v", err)
	}
	cursor := first.Cursor

	// Nothing has been posted since. A read from that position is empty,
	// and **empty is not an error** (`FEED-R19`): a publisher who has not
	// posted is a publisher in their ordinary state.
	again, err := workbench.ReadFeed(ctx, c, workbench.FeedReadOpts{Cursor: cursor})
	if err != nil {
		t.Fatalf("read from the cursor: %v", err)
	}
	if len(again.Entries) != 0 {
		t.Fatalf("a read from the cursor returned %d entries, want none — O(new) is the whole reason "+
			"§4.4 exists", len(again.Entries))
	}
	if again.Resumed {
		t.Error("the read reported resuming from a page number while its cursor's entry was still there")
	}

	// Two more posts, then republish. Only the new ones come back.
	f := pair.publisher.Feed()
	f.PageSize = 2
	for i := 0; i < 2; i++ {
		if _, err := f.Post(entitysdk.PostRequest{Text: "posted after the reader stopped"}); err != nil {
			t.Fatalf("post: %v", err)
		}
	}
	republish(t, pair.publisher, "app/feed/")

	fresh, err := workbench.ReadFeed(ctx, c, workbench.FeedReadOpts{Cursor: cursor})
	if err != nil {
		t.Fatalf("read after two posts: %v", err)
	}
	if len(fresh.Entries) != 2 {
		t.Fatalf("read %d entries since the cursor, want 2\nnotes: %v", len(fresh.Entries), fresh.Notes)
	}

	// **The intermediate case first, because it is the one that must NOT
	// resume.** Unbind the entry's BYTES and leave its index row: the
	// reader can still tell where it stopped, because a position is a
	// listing and not a fetch. It reads everything above the row, stops
	// there, and reports nothing unusual — resuming here would re-show
	// entries on the strength of a publisher withholding one body.
	if !pair.publisher.Store().Remove(entitysdk.FeedEntryTreePath(pair.publisher.PeerID(), cursor.Applied)) {
		t.Fatal("unbinding the cursor's entry did nothing")
	}
	republish(t, pair.publisher, "app/feed/")
	withheld, err := workbench.ReadFeed(ctx, c, workbench.FeedReadOpts{Cursor: cursor})
	if err != nil {
		t.Fatalf("read with the cursor's bytes withheld: %v", err)
	}
	if withheld.Resumed {
		t.Error("the read resumed from a page number because the cursor entry's BYTES were not served. " +
			"Its row is still in the index, so the position is intact — §4.4 is about the row being " +
			"gone, and treating a withheld body as a lost position hands a publisher a way to make " +
			"every reader re-show old entries")
	}
	if len(withheld.Entries) != 2 {
		t.Errorf("read %d entries with the position intact, want the 2 newer ones", len(withheld.Entries))
	}

	// `FEED-R14`: now REMOVE the entry as an author removes one — §4.3
	// rule 3, a page that loses an entry is a page with fewer entries —
	// so the hash this reader is holding is in no page at all.
	removeEntryFromIndex(t, pair.publisher, cursor.Applied)
	republish(t, pair.publisher, "app/feed/")

	resumed, err := workbench.ReadFeed(ctx, c, workbench.FeedReadOpts{Cursor: cursor})
	if err != nil {
		t.Fatalf("read after the cursor's entry was removed: %v", err)
	}
	if len(resumed.Entries) < 2 {
		t.Errorf("read %d entries after the position was removed, want at least the 2 newer ones — a "+
			"reader that loses its place must not lose the feed", len(resumed.Entries))
	}
	if !resumed.Resumed {
		t.Error("FEED-R14 did not fire: the entry this reader held as its position no longer resolves, " +
			"and the read did not report resuming from the page number. Silence here means a reader " +
			"cannot tell a resumed read from an ordinary one, and some of these entries are ones it " +
			"has already shown")
	}
}

// TestFeedReader_FallsBackToEnumerationWhenTheIndexIsGone is §4.3 rule 6,
// a floor obligation: the index is an optimization and MUST NOT be the
// authority.
//
// **The arm that matters is the comparison, not the count.** *Slower, same
// answer* is the rule, so the test asserts the enumerated set equals the
// indexed one — a fallback that returns fewer entries would satisfy a
// "did not error" check while leaving the publisher able to lie by
// omission, which is the exact hole the rule closes.
func TestFeedReader_FallsBackToEnumerationWhenTheIndexIsGone(t *testing.T) {
	pair := newLiveFeedPair(t, 5)
	ctx := context.Background()

	c, err := workbench.NewPeerConsumer(pair.reader, pair.publisher.PeerID(), nil)
	if err != nil {
		t.Fatalf("NewPeerConsumer: %v", err)
	}
	indexed, err := workbench.ReadFeed(ctx, c, workbench.FeedReadOpts{})
	if err != nil {
		t.Fatalf("read with the index present: %v", err)
	}

	// Remove the head AND every page: an index is the head plus its pages,
	// and leaving the pages would leave a reader a route that rule 6 is
	// not about.
	pubID := pair.publisher.PeerID()
	if !pair.publisher.Store().Remove(entitysdk.FeedIndexTreePath(pubID)) {
		t.Fatal("unbinding the index head did nothing")
	}
	for p := uint64(0); p < 3; p++ {
		pair.publisher.Store().Remove(entitysdk.FeedIndexPageTreePath(pubID, p))
	}
	republish(t, pair.publisher, "app/feed/")

	fallback, err := workbench.ReadFeed(ctx, c, workbench.FeedReadOpts{})
	if err != nil {
		t.Fatalf("read with no index: %v — rule 6 makes a missing index a cost, not an answer", err)
	}
	if fallback.Via != workbench.FeedViaEnumeration {
		t.Fatalf("read via %q with no index present, want enumeration", fallback.Via)
	}
	if len(fallback.Entries) != len(indexed.Entries) {
		t.Fatalf("enumeration found %d entries, the index found %d — *slower, same answer* is the rule, "+
			"and a shorter answer is the publisher lying by omission with our help\nnotes: %v",
			len(fallback.Entries), len(indexed.Entries), fallback.Notes)
	}
	got := map[string]bool{}
	for _, e := range fallback.Entries {
		got[e.Hash.String()] = true
		if e.Listed {
			t.Errorf("entry %s claims it came from the index, and there is no index", e.Hash)
		}
		if !e.Attributed {
			t.Errorf("entry %s lost its attribution in the fallback: %s", e.Hash, e.Attribution)
		}
	}
	for _, e := range indexed.Entries {
		if !got[e.Hash.String()] {
			t.Errorf("entry %s was reachable through the index and is missing from the enumeration", e.Hash)
		}
	}
	// AP93's discipline, asserted rather than trusted: a bounded sweep
	// that stopped must say so.
	if fallback.Truncated {
		t.Errorf("the enumeration truncated on a five-entry feed, which means the budget is not what "+
			"this test thinks it is: %v", fallback.Notes)
	}
}

// TestFeedReader_RejectsAnEntryClaimingAnotherAuthorAndKeepsTheRow is
// `FEED-R1`, the forgery gate.
//
// The entry is authored by a THIRD peer and bound into the publisher's own
// entry prefix — which is what a publisher serving somebody else's bytes
// under their own namespace looks like. §1.1 makes the namespace the
// authorship claim, so the reader must refuse it; and it must keep the row,
// because a silently dropped entry makes the reader's list disagree with
// the index it came from and hides the interesting fact.
func TestFeedReader_RejectsAnEntryClaimingAnotherAuthorAndKeepsTheRow(t *testing.T) {
	pair := newLiveFeedPair(t, 2)
	ctx := context.Background()

	stranger, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { stranger.Close() })

	// A well-formed entry whose `author` is the stranger, planted in the
	// publisher's tree and named by the publisher's own index.
	forged := entitysdk.FeedEntryData{
		Author:    stranger.PeerID(),
		CreatedAt: 1_700_000_500_000,
		Body:      textEmbedFor(t, "planted under somebody else's namespace"),
	}
	ent, err := forged.ToEntity()
	if err != nil {
		t.Fatalf("encode the planted entry: %v", err)
	}
	pubID := pair.publisher.PeerID()
	if _, err := pair.publisher.Store().Put(
		entitysdk.FeedEntryTreePath(pubID, ent.ContentHash), ent.Type, forged); err != nil {
		t.Fatalf("plant the entry: %v", err)
	}
	// Name it from page 0, which is where the publisher's own oldest
	// entries are. Read-modify-write of one page is exactly what §4.3
	// rule 1's key addressing makes cheap.
	pageEnt, ok := pair.publisher.Store().Get(entitysdk.FeedIndexPageTreePath(pubID, 0))
	if !ok {
		t.Fatal("page 0 is not in the publisher's tree")
	}
	page, err := entitysdk.FeedIndexPageFromEntity(pageEnt)
	if err != nil {
		t.Fatal(err)
	}
	page.Entries = append([]entitysdk.EntityRef{entitysdk.PinnedRef(pubID, ent.ContentHash)}, page.Entries...)
	if _, err := pair.publisher.Store().Put(
		entitysdk.FeedIndexPageTreePath(pubID, 0), entitysdk.TypeFeedIndexPage, page); err != nil {
		t.Fatalf("rewrite page 0: %v", err)
	}
	republish(t, pair.publisher, "app/feed/")

	c, err := workbench.NewPeerConsumer(pair.reader, pubID, nil)
	if err != nil {
		t.Fatalf("NewPeerConsumer: %v", err)
	}
	read, err := workbench.ReadFeed(ctx, c, workbench.FeedReadOpts{})
	if err != nil {
		t.Fatalf("ReadFeed: %v", err)
	}

	var rejected, rendered int
	for _, e := range read.Entries {
		if e.Hash != ent.ContentHash {
			continue
		}
		rendered++
		if !e.Rejected {
			t.Errorf("the reader accepted an entry claiming author %s under namespace %s — §1.1 makes "+
				"the namespace the authorship claim, and without this check a publisher can serve an "+
				"entry in anybody's name", stranger.PeerID(), pubID)
		}
		if e.Text != "" {
			t.Error("a rejected entry's body was rendered; the row is kept so the fact is visible, " +
				"not so the content is")
		}
		if !strings.Contains(e.Problem, stranger.PeerID()) {
			t.Errorf("the rejection does not name the author it claims: %q", e.Problem)
		}
		rejected++
	}
	if rendered == 0 {
		t.Fatal("the planted entry is in no row at all. Dropping it silently makes this list disagree " +
			"with the index it was read from, and the dropped row is the interesting one")
	}
	if rejected == 0 {
		t.Fatal("FEED-R1 did not fire")
	}
}

// textEmbedFor builds the one embed shape a post carries, so this file
// plants an entry the same way the author would.
func textEmbedFor(t *testing.T, text string) entitysdk.EmbedNode {
	t.Helper()
	node := entitysdk.NewEmbedNode(entitysdk.DefaultPostMediaType,
		entitysdk.InlinePayload([]byte(text)), text)
	if err := node.Validate(); err != nil {
		t.Fatalf("build embed: %v", err)
	}
	return node
}

// removeEntryFromIndex is §7.3's removal as an author performs it: the
// entry leaves the page that named it, and the page is rewritten in
// place. **Nothing is renumbered** (§4.3 rule 3) — a page that loses an
// entry is a page with fewer entries — which is what keeps every other
// reader's cursor valid.
func removeEntryFromIndex(t *testing.T, ap *entitysdk.AppPeer, entry hash.Hash) {
	t.Helper()
	pubID := ap.PeerID()
	headEnt, ok := ap.Store().Get(entitysdk.FeedIndexTreePath(pubID))
	if !ok {
		t.Fatal("no index head to remove an entry from")
	}
	head, err := entitysdk.FeedIndexHeadFromEntity(headEnt)
	if err != nil {
		t.Fatal(err)
	}
	for p := int64(head.Current); p >= 0; p-- {
		page := uint64(p)
		ent, ok := ap.Store().Get(entitysdk.FeedIndexPageTreePath(pubID, page))
		if !ok {
			continue
		}
		data, err := entitysdk.FeedIndexPageFromEntity(ent)
		if err != nil {
			t.Fatal(err)
		}
		kept := make([]entitysdk.EntityRef, 0, len(data.Entries))
		found := false
		for _, r := range data.Entries {
			if r.Hash != nil && *r.Hash == entry {
				found = true
				continue
			}
			kept = append(kept, r)
		}
		if !found {
			continue
		}
		data.Entries = kept
		if _, err := ap.Store().Put(entitysdk.FeedIndexPageTreePath(pubID, page),
			entitysdk.TypeFeedIndexPage, data); err != nil {
			t.Fatalf("rewrite page %d: %v", page, err)
		}
		ap.Store().Remove(entitysdk.FeedEntryTreePath(pubID, entry))
		return
	}
	t.Fatalf("entry %s is in no index page, so nothing was removed", entry)
}

// TestFeedReader_AnUnsignedEntryIsUnattributedAndStillShown is `FEED-R4`'s
// negative arm, and without it the positive one passes against a reader
// that simply sets `Attributed` to true.
//
// The entry is authored by the publisher, under the publisher's own
// namespace, and **nobody minted its detached signature** — which is what
// an entry written before that path existed looks like, and what a
// re-serializing mirror produces. §1.1.1: *found under a namespace* and
// *signed by that peer* are different facts. It is SHOWN, because an
// unsigned entry is a real entry and hiding it makes the reader's list
// disagree with the index it came from; it is not attributed, because the
// one thing that could attribute it is missing.
func TestFeedReader_AnUnsignedEntryIsUnattributedAndStillShown(t *testing.T) {
	pair := newLiveFeedPair(t, 2)
	ctx := context.Background()
	pubID := pair.publisher.PeerID()

	unsigned := entitysdk.FeedEntryData{
		Author:    pubID,
		CreatedAt: 1_700_000_900_000,
		Body:      textEmbedFor(t, "authored here, signed by nobody"),
	}
	ent, err := unsigned.ToEntity()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if _, err := pair.publisher.Store().Put(
		entitysdk.FeedEntryTreePath(pubID, ent.ContentHash), ent.Type, unsigned); err != nil {
		t.Fatalf("plant: %v", err)
	}
	prependToPage(t, pair.publisher, 0, entitysdk.PinnedRef(pubID, ent.ContentHash))
	republish(t, pair.publisher, "app/feed/")

	c, err := workbench.NewPeerConsumer(pair.reader, pubID, nil)
	if err != nil {
		t.Fatalf("NewPeerConsumer: %v", err)
	}
	read, err := workbench.ReadFeed(ctx, c, workbench.FeedReadOpts{})
	if err != nil {
		t.Fatalf("ReadFeed: %v", err)
	}

	var found bool
	for _, e := range read.Entries {
		if e.Hash != ent.ContentHash {
			continue
		}
		found = true
		if e.Attributed {
			t.Error("an entry with no detached signature came back ATTRIBUTED. `FEED-R4` is the rule " +
				"that a hash matching does not mean an author wrote it, and a reader that credits " +
				"whoever served the bytes is the failure it names")
		}
		if e.Attribution == "" {
			t.Error("unattributed with no reason given, which reads as a defect in the reader rather " +
				"than a fact about the entry")
		}
		if e.Rejected {
			t.Error("an unsigned entry was REJECTED. It is authored under the namespace it was found " +
				"in, so `FEED-R1` has nothing to say about it — the two rules are separate")
		}
		if e.Text == "" {
			t.Error("the unsigned entry's body was withheld; it is a real entry and the list has to " +
				"agree with the index it was read from")
		}
	}
	if !found {
		t.Fatal("the unsigned entry is in no row")
	}
	// The signed entries beside it must still be attributed, or this test
	// passes against a reader that attributes nothing.
	for _, e := range read.Entries {
		if e.Hash != ent.ContentHash && !e.Attributed {
			t.Errorf("entry %s lost its attribution too: %s", e.Hash, e.Attribution)
		}
	}
}

// prependToPage puts a row at the newest position of one index page.
func prependToPage(t *testing.T, ap *entitysdk.AppPeer, page uint64, ref entitysdk.EntityRef) {
	t.Helper()
	key := entitysdk.FeedIndexPageTreePath(ap.PeerID(), page)
	ent, ok := ap.Store().Get(key)
	if !ok {
		t.Fatalf("page %d is not in the publisher's tree", page)
	}
	data, err := entitysdk.FeedIndexPageFromEntity(ent)
	if err != nil {
		t.Fatal(err)
	}
	data.Entries = append([]entitysdk.EntityRef{ref}, data.Entries...)
	if _, err := ap.Store().Put(key, entitysdk.TypeFeedIndexPage, data); err != nil {
		t.Fatalf("rewrite page %d: %v", page, err)
	}
}
