package publish_test

import (
	"context"
	"strings"
	"testing"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"
)

// feed_road_wiring_test.go — the feed reader on the road a SURFACE takes,
// and the field that says which path answered.
//
// # The gap this closes
//
// Every other reader gate in this tree builds its own [fetch.Consumer] —
// `NewPeerConsumer` in `feed_reader_live_test.go`, `corridorCut` in
// `feed_corridor_test.go` — and hands it to `workbench.ReadFeed`. Those
// establish the MECHANISM: an index resolves, rule 6 falls back, a forged
// entry is refused, a removed position resumes. **Nothing drove
// `BrowseModel.ReadFeedOf`**, which is the only entry point a shipped
// surface has: the `timeline` verb reaches it through
// `ShellWorkspace.FeedTimeline` (`shellcmd/follow_op.go:272`) and the
// Avalonia feed panel reaches the same call across the bridge. So the road
// chooser, the per-publisher consumer memo, the shared `seq` floor and the
// `canReachLive` gate in front of all of it were reached by no test.
//
// That is AP107 exactly — *a gate for the mechanism and none for the
// wiring* — in its feed-shaped half.
//
// # Why the assertion is on WHICH PATH and not on the entries
//
// ⭐ `FEED-R13`'s rule-6 fallback is built to survive a publisher that
// withholds an index, and **it survives our own broken primary path
// identically.** That is not hypothetical here. It hid `AP106` on the
// corridor cut, where **0 of 34 entries were found by index and 34 by
// fallback** — right answer, no error, and no note any caller would read
// as a defect (`workbench/feed_read.go`, the `committed` map's comment
// records it). `entity-browser-rust` reported the same shape from the
// other side: their corridor ① result is 34 via index / **0 via
// enumeration**, and their `0` is *structural* — `FeedSource::list`
// defaults to `cannot enumerate` — rather than a finding about the bytes
// we sent them.
//
// So a count is not evidence of anything. [workbench.FeedRead.Via] and
// [workbench.FeedEntryRead.Listed] are the two fields that discriminate,
// and the control arm below exists so that asserting on them cannot be
// satisfied by a reader which hard-wires either one.
//
// # What a green run does NOT claim
//
// Nothing cross-implementation: one publisher and one reader, both ours.
// Nothing about the static road — there is no origin in this harness for
// one to be taken. Nothing about `FeedTimeline`'s merge, its cursor
// persistence or its exclusion rows, which are `shellcmd`'s and are gated
// in `shellboot/feed_timeline_test.go`. And nothing about a feed reached
// over a transport profile: `ReadFeedOf` is the peer-id case by
// construction (§2.4 makes the subject a namespace), so the road it
// chooses is the one `canReachLive` admits and no other.

// TestFeedRoad_TheShippedReaderAnswersViaTheIndexAndSaysWhich drives
// `BrowseModel.ReadFeedOf` — the call every feed surface in this repo
// makes — and asserts the path, not the answer.
func TestFeedRoad_TheShippedReaderAnswersViaTheIndexAndSaysWhich(t *testing.T) {
	pair := newLiveFeedPair(t, 5)
	ctx := context.Background()
	pubID := pair.publisher.PeerID()

	m := workbench.NewBrowseModel(nil)
	m.SetPeer(pair.reader)

	read, err := m.ReadFeedOf(ctx, pubID, workbench.FeedReadOpts{})
	if err != nil {
		t.Fatalf("ReadFeedOf: %v", err)
	}

	// The road first. There is no origin anywhere in this harness, so a
	// static read is structurally impossible — asserting the sentence
	// anyway puts the claim in the artifact instead of in the setup, and
	// it is the discriminator
	// `TestBrowseModel_PeerIDAddressTakesTheLiveRoadWithNoOrigin` already
	// uses for a page. A feed that described its road differently from
	// the way a page describes the same road would be the drift
	// `fetch/freshness.go` exists to make impossible.
	if !strings.Contains(read.Freshness, "the publisher answered for itself") {
		t.Errorf("the shipped road did not take the live one:\n  %s", read.Freshness)
	}
	if !read.Published {
		t.Fatalf("the publisher has a signed root and the read says it has none\nnotes: %v", read.Notes)
	}

	// ⭐ The discrimination. An entry list that is correct and was reached
	// through the fallback is the defect wearing the fix.
	if read.Via != workbench.FeedViaIndex {
		t.Fatalf("read via %q with an index published, want %q. Rule 6 costs O(all) and returns the same "+
			"answer, which is exactly why taking it silently is how a broken index lookup reports as "+
			"healthy\nnotes: %v", read.Via, workbench.FeedViaIndex, read.Notes)
	}
	if len(read.Entries) != len(pair.posts) {
		t.Fatalf("read %d entries, want %d\nnotes: %v", len(read.Entries), len(pair.posts), read.Notes)
	}
	for i, e := range read.Entries {
		if !e.Listed {
			t.Errorf("entry %d came back unlisted on a read that reports itself as indexed — `Via` and "+
				"`Listed` are set in different places and a surface renders both", i)
		}
		if e.Problem != "" {
			t.Errorf("entry %d: %s", i, e.Problem)
		}
		if !e.Attributed {
			// The live road carries `system/signature/*` on the same grant
			// that carries the root's own signature, so these are
			// attributable and an unattributed one means the reader cannot
			// reach a signature it can reach.
			t.Errorf("entry %d is unattributed on the live road: %s", i, e.Attribution)
		}
	}

	// ── The control arm, and it is the one that makes the above mean
	// something ──
	//
	// Remove the index and read again THROUGH THE SAME MODEL. Two
	// properties are established at once and both are load-bearing:
	//
	//  1. `Via` and `Listed` can take their other value on this road. Each
	//     is set at a single line in a single branch
	//     (`feed_read.go:392` and `:515`), so without this arm every
	//     assertion above is satisfied by a reader that hard-wires
	//     `Listed: true` and never enumerates at all.
	//  2. The consumer is memoized per publisher for the life of the
	//     browser (`consumerForRoad`), so a re-read after the publisher
	//     re-minted must see the NEW root. A memo that froze the first
	//     verified root would answer every later read about a tree that
	//     has moved — and it would be indistinguishable from a publisher
	//     who had not republished, which is the failure
	//     `feed_post_reach_live_test.go` measures from the other end.
	if !pair.publisher.Store().Remove(entitysdk.FeedIndexTreePath(pubID)) {
		t.Fatal("unbinding the index head did nothing")
	}
	for p := uint64(0); p < 3; p++ {
		pair.publisher.Store().Remove(entitysdk.FeedIndexPageTreePath(pubID, p))
	}
	republish(t, pair.publisher, "app/feed/")

	fallback, err := m.ReadFeedOf(ctx, pubID, workbench.FeedReadOpts{})
	if err != nil {
		t.Fatalf("ReadFeedOf with no index: %v — rule 6 makes a missing index a cost and not an answer", err)
	}
	if fallback.Via != workbench.FeedViaEnumeration {
		t.Fatalf("read via %q with the index unbound, want %q. If this says `index`, the memoized "+
			"consumer is answering about the root it verified first and the re-mint reached nothing",
			fallback.Via, workbench.FeedViaEnumeration)
	}
	if len(fallback.Entries) != len(read.Entries) {
		t.Fatalf("enumeration found %d entries where the index found %d — *slower, same answer* is the "+
			"rule, and a shorter answer is a publisher lying by omission with our help\nnotes: %v",
			len(fallback.Entries), len(read.Entries), fallback.Notes)
	}
	indexed := map[string]bool{}
	for _, e := range read.Entries {
		indexed[e.Hash.String()] = true
	}
	for _, e := range fallback.Entries {
		if e.Listed {
			t.Errorf("entry %s reports that the index named it, and there is no index", e.Hash)
		}
		if !indexed[e.Hash.String()] {
			t.Errorf("the enumeration returned %s, which the index never named", e.Hash)
		}
	}
}

// TestFeedRoad_AReaderWithNoPeerRefusesAndSaysWhy is the wiring's own
// anti-vacuity arm.
//
// `ReadFeedOf` is the one feed entry point that chooses a road, and the
// road it can choose is live or nothing: a subject is a peer-id and
// carries no origin. So the failure to guard against is not a wrong road,
// it is a *silent local* one — a reader that quietly answers out of its
// own store would return an empty feed with no error, which reads as a
// publisher who has posted nothing.
//
// Two peers exist in this harness and the publisher's entries are in the
// publisher's tree, so a local read here returns nothing rather than
// something wrong. That is precisely why the assertion is on the REFUSAL
// and its sentence rather than on the entry count: *empty* is the answer
// both the correct refusal and the silent local read would produce, and
// only one of them says so.
func TestFeedRoad_AReaderWithNoPeerRefusesAndSaysWhy(t *testing.T) {
	pair := newLiveFeedPair(t, 3)
	ctx := context.Background()

	// No `SetPeer`. This is `entity-fetch`'s configuration, which reads
	// static origins correctly and completely and cannot dispatch.
	m := workbench.NewBrowseModel(nil)

	read, err := m.ReadFeedOf(ctx, pair.publisher.PeerID(), workbench.FeedReadOpts{})
	if err == nil {
		t.Fatalf("a browser holding no peer read a feed anyway: via=%q entries=%d published=%v",
			read.Via, len(read.Entries), read.Published)
	}
	if !strings.Contains(err.Error(), "holds no peer") {
		t.Errorf("the refusal does not name the missing peer, so an operator is told a feed is "+
			"unreachable and not which of their own two configurations caused it: %v", err)
	}
	// §2.4 is the sentence that makes this the operator's own problem and
	// not the publisher's: following needs no permission from them, so a
	// refusal here must never read as *they did not let you*.
	if !strings.Contains(err.Error(), "§2.4") {
		t.Errorf("the refusal does not say that following requires no permission from the subject, "+
			"which is the fact that tells the operator which machine to fix: %v", err)
	}

	// The same model, given the peer, reads it — or the arm above is
	// measuring a subject that was never readable.
	m.SetPeer(pair.reader)
	ok, err := m.ReadFeedOf(ctx, pair.publisher.PeerID(), workbench.FeedReadOpts{})
	if err != nil {
		t.Fatalf("the same read with a peer set: %v", err)
	}
	if len(ok.Entries) != len(pair.posts) {
		t.Fatalf("read %d entries with the peer set, want %d — then the refusal above was not the "+
			"only thing standing between this reader and the feed\nnotes: %v",
			len(ok.Entries), len(pair.posts), ok.Notes)
	}
}

// TestFeedRoad_AnUnreachableSubjectIsNamedRatherThanReportedEmpty is the
// third state, and it is the one a timeline meets most often.
//
// A peer we hold no connection to is not a peer with an empty feed. Both
// produce no entries, and only one of them is anybody's problem — so the
// read must REFUSE rather than return an empty `FeedRead`, because
// `FeedTimeline` turns an error into an EXCLUDED row (`FEED-R23`: a
// multi-publisher view declares what produced it) and turns an empty read
// into a publisher who posted nothing.
func TestFeedRoad_AnUnreachableSubjectIsNamedRatherThanReportedEmpty(t *testing.T) {
	pair := newLiveFeedPair(t, 2)
	ctx := context.Background()

	// A third peer nobody has dialled. It is a real peer-id, so this is
	// not a parsing failure — it is the ordinary case of a laptop that is
	// shut.
	stranger, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { stranger.Close() })

	m := workbench.NewBrowseModel(nil)
	m.SetPeer(pair.reader)

	read, err := m.ReadFeedOf(ctx, stranger.PeerID(), workbench.FeedReadOpts{})
	if err == nil {
		t.Fatalf("reading a feed from a peer this reader has never been in touch with returned a "+
			"result instead of a refusal: via=%q entries=%d published=%v",
			read.Via, len(read.Entries), read.Published)
	}
	if !strings.Contains(err.Error(), "holds no connection") {
		t.Errorf("the refusal does not say the reader is not in touch with the subject: %v", err)
	}
	if !strings.Contains(err.Error(), "carries no address") {
		t.Errorf("the refusal does not say a peer-id carries no address to dial, which is the fact "+
			"that stops an operator waiting for a retry that cannot happen: %v", err)
	}
}
