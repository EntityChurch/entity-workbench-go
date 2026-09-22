package shellboot_test

import (
	"context"
	"strings"
	"testing"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/shellboot"
	"entity-workbench-go/shellcmd"
)

// feed_reach_test.go — `post` and `feed` through the real bootstrap, and the
// regression gate for a defect that running the flow found and no unit test
// could have.
//
// **The defect:** `feed` reported *"the published root is BEHIND this tree"* on
// a peer that had published three seconds earlier and posted nothing. The
// staleness check rebuilt the trie over the published prefix with the trailing
// slash trimmed, and a trie's keys are relative to its prefix — so `app/feed`
// and `app/feed/` produce different roots over identical bytes. Every reading
// was stale forever, which is a permanent line in a problems list, which is how
// an operator learns to skip the problems list.
//
// **Both arms are required and the second is the one that would have been
// skipped.** A `Current` that is hard-wired true passes the first arm and is a
// worse defect than the one being fixed: it reports a feed as published while
// the reader sees a root that predates every post.

func feedPeer(t *testing.T, ctx context.Context) *shellcmd.ShellWorkspace {
	t.Helper()
	ap, ws, err := shellboot.Bootstrap(ctx, shellboot.Config{LocalAlias: "author"})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })
	return ws
}

func TestFeedReach_ThePublishedRootIsCurrentUntilSomethingIsPosted(t *testing.T) {
	ctx := context.Background()
	ws := feedPeer(t, ctx)

	if _, err := ws.Post(ctx, shellcmd.FeedPostRequest{Text: "the first entry"}); err != nil {
		t.Fatalf("post: %v", err)
	}

	before, err := ws.Feed(ctx, 0)
	if err != nil {
		t.Fatalf("feed: %v", err)
	}
	if before.Reach.Published {
		t.Error("a peer that has posted and not published reports a published root")
	}
	if len(before.Readout.Entries) != 1 {
		t.Fatalf("read %d entries after one post", len(before.Readout.Entries))
	}

	if _, err := ws.Publish(ctx, shellcmd.PublishRequest{Prefix: shellcmd.FeedPrefix}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// ARM 1 — the regression. Nothing has happened between the publish and
	// this read.
	after, err := ws.Feed(ctx, 0)
	if err != nil {
		t.Fatalf("feed: %v", err)
	}
	if !after.Reach.Published || !after.Reach.CoversFeed {
		t.Fatalf("after publishing %q: published=%v covers=%v prefix=%q",
			shellcmd.FeedPrefix, after.Reach.Published, after.Reach.CoversFeed, after.Reach.Prefix)
	}
	if !after.Reach.Current {
		t.Errorf("the root is reported as behind immediately after publishing, with nothing written "+
			"in between:\n  published %s\n  tree now  %s", after.Reach.RootHash, after.Reach.RootNow)
	}
	for _, p := range after.Problems {
		if strings.Contains(p, "does not commit to what is in the tree now") {
			t.Errorf("a freshly published feed carries the stale-root problem line: %s", p)
		}
	}

	// ARM 2 — the control. One post must move it, or arm 1 is measuring a
	// constant.
	if _, err := ws.Post(ctx, shellcmd.FeedPostRequest{Text: "a second entry, after the publish"}); err != nil {
		t.Fatalf("post: %v", err)
	}
	stale, err := ws.Feed(ctx, 0)
	if err != nil {
		t.Fatalf("feed: %v", err)
	}
	if stale.Reach.Current {
		t.Error("a post after the publish left the root reported as current — a reader would see " +
			"neither the entry nor any sign that one exists")
	}
	if stale.Reach.RootHash == stale.Reach.RootNow {
		t.Errorf("the published root and the tree's root agree after a post (%s) — then the check "+
			"cannot see a post at all", stale.Reach.RootHash)
	}
	var named bool
	for _, p := range stale.Problems {
		named = named || strings.Contains(p, "does not commit to what is in the tree now")
	}
	if !named {
		t.Errorf("nothing in the problem list says the root is behind: %v", stale.Problems)
	}
}

// A reply carries the conversation's root as well as its parent, and the root
// is INHERITED rather than assumed to be the parent — so a reply to a reply
// names the original.
func TestFeedReach_AReplyToAReplyNamesTheOriginalConversation(t *testing.T) {
	ctx := context.Background()
	ws := feedPeer(t, ctx)

	first, err := ws.Post(ctx, shellcmd.FeedPostRequest{Text: "the root of a conversation"})
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	second, err := ws.Post(ctx, shellcmd.FeedPostRequest{Text: "a reply", ReplyTo: first.Entry.Hash})
	if err != nil {
		t.Fatalf("reply: %v", err)
	}
	third, err := ws.Post(ctx, shellcmd.FeedPostRequest{Text: "a reply to the reply", ReplyTo: second.Entry.Hash})
	if err != nil {
		t.Fatalf("reply to reply: %v", err)
	}

	out, err := ws.Feed(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Readout.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(out.Readout.Entries))
	}
	if !out.Readout.Entries[0].IsReply || !out.Readout.Entries[1].IsReply {
		t.Error("the two replies do not read back as replies")
	}
	if out.Readout.Entries[2].IsReply {
		t.Error("the first entry reads back as a reply")
	}

	// The claim this test is named after, checked in the bytes rather than
	// in the readout, because `IsReply` is a boolean and the inheritance is
	// the part that can be wrong while the boolean is right.
	ent, found, err := ws.Local.Peer.Get(third.Entry.EntryPath)
	if err != nil || !found {
		t.Fatalf("reading back the third entry: found=%v err=%v", found, err)
	}
	data, err := entitysdk.FeedEntryFromEntity(ent)
	if err != nil {
		t.Fatal(err)
	}
	if data.Reply == nil || data.Reply.Root.Hash == nil || data.Reply.Parent.Hash == nil {
		t.Fatal("the third entry carries no complete reply term")
	}
	if *data.Reply.Parent.Hash != second.Entry.Hash {
		t.Errorf("parent is %s, want the second entry %s", data.Reply.Parent.Hash, second.Entry.Hash)
	}
	if *data.Reply.Root.Hash != first.Entry.Hash {
		t.Errorf("root is %s, want the conversation's first entry %s — `root` is INHERITED from the "+
			"parent, and assuming it IS the parent puts every deep reply in a conversation of its own",
			data.Reply.Root.Hash, first.Entry.Hash)
	}
}
