package shellboot_test

import (
	"context"
	"strings"
	"testing"

	"entity-workbench-go/shellcmd"
	"entity-workbench-go/workbench"
)

// feed_publish_curated_test.go — `publish -feed` through the real bootstrap.
//
// # Why this exists beside the gates in `publish/`
//
// `publish/a38_curated_content_set_test.go` gates the MECHANISM: it builds the
// content set itself, hands it to `MintRoot`, and asserts on the committed key
// set. That is AP108's shape exactly — *a test that constructs the thing under
// test's input cannot fail on the input the product chooses* — and AP108 was
// earned in this very corridor, where every feed-reader gate was green while
// the road a user takes was wired to the wrong transport.
//
// So this file drives the **verb**, and the thing it is really about is the
// half that lives nowhere near the trie: a curated root and a scanned root are
// two hashes over one prefix, the published root records which it was NOWHERE,
// and so the publisher keeps its own note (`workbench.PublishRecordPath`).
// Every re-derivation reads that note back. Get it wrong in either direction
// and nothing errors — `feed` simply reports *"the published root is BEHIND
// this tree"* on a root that is exactly current, forever, which is the
// identical symptom the trailing-slash defect produced and the reason
// `feed_reach_test.go` exists.
//
// **The mechanism tests cannot see that.** They never call the verb, never
// write the record and never re-derive anything.

// TestFeedPublishCurated_TheVerbCommitsToTheSignaturesAndStaysCurrent is the
// wiring gate, and the `Current` assertion is the load-bearing one.
func TestFeedPublishCurated_TheVerbCommitsToTheSignaturesAndStaysCurrent(t *testing.T) {
	ctx := context.Background()
	ws := feedPeer(t, ctx)

	for _, text := range []string{"the first entry", "the second entry"} {
		if _, err := ws.Post(ctx, shellcmd.FeedPostRequest{Text: text}); err != nil {
			t.Fatalf("post %q: %v", text, err)
		}
	}

	out, err := ws.Publish(ctx, shellcmd.PublishRequest{Feed: true})
	if err != nil {
		t.Fatalf("publish -feed: %v", err)
	}

	// `-feed` publishes at the peer root, because that is the only prefix
	// containing both `app/feed/…` and `system/signature/…`.
	if strings.Trim(out.Prefix, "/") != "" {
		t.Errorf("publish -feed committed to prefix %q, want the peer root", out.Prefix)
	}
	if out.ContentSet != workbench.PublishContentFeed {
		t.Errorf("publish -feed reported content set %q, want %q — the surface has no other way "+
			"to tell a 9-key curated root from a 399-key scan over the same prefix",
			out.ContentSet, workbench.PublishContentFeed)
	}

	// THE NUMBER. Two posts is 2 entries + 2 signatures + 1 head + at least
	// one page, and nowhere near what the peer root bounds. An upper bound
	// rather than an exact count because the page size is the author's and
	// this test is not the place to pin it — but it has to be small enough
	// that a scan cannot pass, and a bootstrapped peer carries well over a
	// hundred `system/` bindings.
	if out.Bindings == 0 || out.Bindings > 20 {
		t.Errorf("publish -feed committed to %d keys; a curated feed set for two posts is a "+
			"handful, and a number this size means the selector re-scanned the prefix",
			out.Bindings)
	}

	// ARM 1 — the regression the record exists to prevent. Nothing has
	// happened between the publish and this read, so the root describes the
	// tree. It can only report so if `feedReach` re-derived with the SAME
	// set the verb minted with.
	after, err := ws.Feed(ctx, 0)
	if err != nil {
		t.Fatalf("feed: %v", err)
	}
	if !after.Reach.Published {
		t.Fatal("after publish -feed the peer reports no published root")
	}
	if after.Reach.ContentSet != workbench.PublishContentFeed {
		t.Errorf("feed read back content set %q, want %q — the publisher's note did not survive "+
			"the round trip, so every re-derivation is about a different publish",
			after.Reach.ContentSet, workbench.PublishContentFeed)
	}
	if !after.Reach.Current {
		t.Errorf("the root is reported as behind immediately after `publish -feed`, with nothing "+
			"written in between:\n  published %s\n  tree now  %s\n"+
			"That is the staleness check re-deriving with the prefix SCAN while the root was "+
			"minted from the curated set — a permanent false line in the problems list.",
			after.Reach.RootHash, after.Reach.RootNow)
	}

	// ARM 2 — `A-36` closed, and the caveat with it. The signatures are in
	// the committed set now, so the standing note must stop being printed;
	// a caveat that outlives its defect tells an operator who did the right
	// thing that it did not work.
	if after.Reach.SignatureNote != "" {
		t.Errorf("after publish -feed the attribution caveat is still printed: %q\n"+
			"The published root commits to every entry's FEED-R2 signature, which is exactly "+
			"what that sentence says is impossible.", after.Reach.SignatureNote)
	}

	// ARM 3 — the anti-vacuity arm, and without it arms 1 and 2 are
	// satisfied by a build where `Current` is hard-wired true and the note
	// is hard-wired empty. A narrow publish must STILL say both.
	if _, err := ws.Publish(ctx, shellcmd.PublishRequest{Prefix: shellcmd.FeedPrefix}); err != nil {
		t.Fatalf("publish -prefix %s: %v", shellcmd.FeedPrefix, err)
	}
	narrow, err := ws.Feed(ctx, 0)
	if err != nil {
		t.Fatalf("feed after narrow publish: %v", err)
	}
	if narrow.Reach.ContentSet != workbench.PublishContentScan {
		t.Errorf("a narrow publish left the content set at %q — the record tracks the LIVE root, "+
			"so re-publishing another way must move it back", narrow.Reach.ContentSet)
	}
	if narrow.Reach.SignatureNote == "" {
		t.Error("a root over `app/feed/` prints no attribution caveat — it does not commit to " +
			"`system/signature/…`, so a static reader cannot attribute a single entry, and " +
			"arm 2 above is not distinguishing anything")
	}
	if !narrow.Reach.Current {
		t.Error("the narrow publish reports itself behind, so the re-derivation followed the " +
			"record in one direction only")
	}
}

// TestFeedPublishCurated_ANarrowerPrefixWithFeedIsRefused pins the refusal.
//
// An operator who typed both `-feed` and `-prefix app/feed/` told us two
// things: commit to the evidence, and commit to a prefix that structurally
// cannot contain it. Silently widening would be doing something they did not
// ask for with their disclosure; silently narrowing would publish a feed
// nobody can attribute while reporting success.
func TestFeedPublishCurated_ANarrowerPrefixWithFeedIsRefused(t *testing.T) {
	ctx := context.Background()
	ws := feedPeer(t, ctx)
	if _, err := ws.Post(ctx, shellcmd.FeedPostRequest{Text: "an entry"}); err != nil {
		t.Fatalf("post: %v", err)
	}

	_, err := ws.Publish(ctx, shellcmd.PublishRequest{
		Feed:   true,
		Prefix: shellcmd.FeedPrefix,
	})
	if err == nil {
		t.Fatal("publish -feed -prefix app/feed/ was accepted; one of the two instructions was " +
			"silently discarded and the operator is not told which")
	}
	if !strings.Contains(err.Error(), "peer root") {
		t.Fatalf("refused for the wrong reason: %v", err)
	}
}
