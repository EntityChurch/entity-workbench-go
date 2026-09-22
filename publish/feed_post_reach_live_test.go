package publish_test

import (
	"context"
	"testing"
	"time"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/publish"
	"entity-workbench-go/workbench"
)

// feed_post_reach_live_test.go — what a VERIFYING LIVE reader sees when an
// author has posted and not re-minted.
//
// # Why this exists
//
// `shellboot/feed_reach_test.go` gates what *our own peer reports* about its
// published root going stale. It says nothing about what a reader on the other
// end actually gets, and those are different claims — the second is the one a
// composer is designed against.
//
// It was written to settle a cross-seat disagreement: that writing
// `app/feed/entry` into your own tree **is** publishing, and that "the live
// road needs nothing further" because a connected peer serves its tree like
// anything else.
//
// # What this test does and does not settle — corrected 2026-09-15
//
// An earlier version of this header said §7.6 "makes every conformant reader"
// root-anchored. **That is an overclaim and it is withdrawn.** §7.6 is titled
// *the light-client property* and §7.2's "verification is root-anchored" is
// unqualified prose; **no `FEED-Rn` row obliges a reader to anchor a read on a
// root** (checked, all 34). And §1.1 rules the other way on attribution: an
// entry "should verify alone … without a root that may be many publishes
// stale", the root answering the separate **anti-omission** question — *was
// this in their published tree, as of sequence N?*
//
// So the other seat is RIGHT that an entry needs no root to be attributable.
// What is root-anchored is **discovery**, and this test measures that it is
// root-anchored *as built, on both roads, in both implementations*:
//
//   - our index path walks the verified root;
//   - our `FEED-R13` rule-6 fallback enumerates "every key the signed root
//     commits to" (`workbench/feed_read.go:73-76`, `:470`) — so the fallback
//     that exists precisely so the index is never the authority **inherits the
//     same anchor and cannot rescue an un-minted post**;
//   - `entity-browser-rust` measured 34 via index / 0 via enumeration, their
//     `FeedSource::list` defaulting to *cannot enumerate*.
//
// A live reader has a second channel a static one does not — a `tree:list` at
// the author's own peer — and the convention does not say whether rule 6 may
// use it. **"The live road needs nothing further" is therefore undecided, not
// false:** it is achievable by a reader nobody has built. This test pins what
// happens with the readers that exist.
//
// # The shape that makes it worth a gate rather than a note
//
// The reader does not error, does not warn, and does not take the enumeration
// fallback. It returns the previous entry set **through the index**, which is
// byte-for-byte what an author who never posted looks like. That is the most
// confident wrong answer available, and it is invisible from the authoring
// side — which is exactly where a composer is built.
//
// Tier: integration (two real peers, real dispatch, real verification).

// TestFeedPostIsUnreachableLiveUntilTheRootIsReminted is the measurement.
//
// **Both arms are required.** Arm 1 alone is satisfied by any harness that
// cannot observe a fourth entry under any circumstances; arm 2 is what proves
// the reader sees it the moment — and only the moment — the root moves.
func TestFeedPostIsUnreachableLiveUntilTheRootIsReminted(t *testing.T) {
	pair := newLiveFeedPair(t, 3)
	ctx := context.Background()

	// A fresh consumer per read, on purpose: the question is what a reader
	// arriving now would see, not what one that had cached a walk sees.
	readLive := func(label string) workbench.FeedRead {
		t.Helper()
		c, err := workbench.NewPeerConsumer(pair.reader, pair.publisher.PeerID(), nil)
		if err != nil {
			t.Fatalf("[%s] NewPeerConsumer: %v", label, err)
		}
		read, err := workbench.ReadFeed(ctx, c, workbench.FeedReadOpts{})
		if err != nil {
			t.Fatalf("[%s] ReadFeed: %v", label, err)
		}
		return read
	}

	before := readLive("after the mint")
	if len(before.Entries) != len(pair.posts) {
		t.Fatalf("baseline: read %d entries, want the %d that were published",
			len(before.Entries), len(pair.posts))
	}

	// Author a fourth entry into the publisher's own tree. This is the whole
	// of "posting": the entry, its signature and the index head all land, and
	// `Post` returns success.
	f := pair.publisher.Feed()
	f.PageSize = 2
	posted, err := f.Post(entitysdk.PostRequest{
		Text: "posted after the root was minted",
		At:   time.UnixMilli(1_700_000_003_000),
	})
	if err != nil {
		t.Fatalf("post: %v", err)
	}

	// ---- ARM 1: the post is unreachable, and nothing says so -------------
	after := readLive("after a post with no re-mint")

	if len(after.Entries) != len(before.Entries) {
		t.Fatalf("a post with no re-mint changed what a live reader sees (%d -> %d). If this is now "+
			"reachable, the convention's §7.6 anchor has moved and AGENTS.md's "+
			"\"posting is not publishing\" paragraph is the thing to correct",
			len(before.Entries), len(after.Entries))
	}
	for _, e := range after.Entries {
		if e.Hash == posted.Hash {
			t.Fatalf("entry %s was posted after the mint and is visible to a verifying reader", posted.Hash)
		}
	}
	// The part that makes it dangerous rather than merely absent: no error,
	// no note, and the CHEAP path answered. A reader has nothing to render
	// that distinguishes this from an author who has not posted.
	if len(after.Notes) != 0 {
		t.Logf("note: the read carried notes, which is better than nothing: %v", after.Notes)
	}
	if after.Via != workbench.FeedViaIndex {
		t.Errorf("the read came back via %q rather than the index — if an uncommitted post makes the "+
			"index miss, the reader at least has a signal, and this test's premise needs re-reading",
			after.Via)
	}

	// ---- ARM 2: the control — the mint is what makes it reachable --------
	//
	// Without this, arm 1 passes against a harness that could never see a
	// fourth entry: a broken author, a page-size bug, a reader pinned to a
	// stale root forever.
	if _, err := publish.MintRoot(ctx, publish.MintOpts{
		Peer:   pair.publisher,
		Prefix: "app/feed/",
	}); err != nil {
		t.Fatalf("re-mint: %v", err)
	}

	remint := readLive("after the re-mint")
	if len(remint.Entries) != len(before.Entries)+1 {
		t.Fatalf("after re-minting, a live reader sees %d entries, want %d — arm 1 was measuring a "+
			"reader that cannot see a fourth entry at all, and proves nothing",
			len(remint.Entries), len(before.Entries)+1)
	}
	var found bool
	for _, e := range remint.Entries {
		found = found || e.Hash == posted.Hash
	}
	if !found {
		t.Errorf("the re-minted root does not carry entry %s, so the control arm is not measuring "+
			"the mint", posted.Hash)
	}

	t.Logf("measured: %d entries before the post, %d after the post with no re-mint, %d after the "+
		"re-mint — the mint is the act a reader can observe, on the live road as much as the static one",
		len(before.Entries), len(after.Entries), len(remint.Entries))
}
