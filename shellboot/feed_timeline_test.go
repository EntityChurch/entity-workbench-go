package shellboot_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/shellboot"
	"entity-workbench-go/shellcmd"
	"entity-workbench-go/workbench"
)

// feed_timeline_test.go — one peer reads another peer's feed through the
// VERB, on two peers built by the real bootstrap.
//
// # What this adds to the gates beside it
//
// `feed_reach_test.go` is the author's own tree: `post` and `feed`, no
// root, no reader. `publish/feed_reader_live_test.go` is the reader's
// mechanism with a hand-built consumer. `publish/feed_road_wiring_test.go`
// is `BrowseModel.ReadFeedOf`, the road the model chooses. **Between the
// last of those and an operator there is still `ShellWorkspace.FeedTimeline`
// and the `timeline` verb, and neither had a test.**
//
// That layer is not glue. It holds four things nothing below it can:
// the follow set is where subjects come from, the cursor is durable
// reader state it persists, `FEED-R23`'s *declare what produced this view*
// is composed here, and an unreachable publisher has to become a named
// exclusion rather than an error that empties the screen.
//
// # Why it runs the VERB and not the method
//
// `shellcmd.Default().Dispatch(sh, "timeline", …)` goes through
// `bareBrowserOf`, which is what binds the local peer to the browser and
// is therefore what decides whether the live road is available at all.
// A test that built its own `BrowseModel` would be asserting about a
// wiring it had just performed itself — the failure `syncTestPeer`'s
// header records, where a fixture that reimplements the thing under test
// cannot fail when the thing under test changes.
//
// It also reaches the rendered lines, which matters because the
// discrimination this file is named for is only worth computing if it
// arrives: `Via` is set in `workbench`, carried through `TimelineSource`,
// and printed by `cmd_follow.go`. A field dropped anywhere along that run
// renders as *"everything is fine"* (AP49).
//
// # Deliberately NOT `OpenAccess`
//
// The publisher runs `publish -public`, which is the operator gesture,
// and the reader is authorized by the `default` policy row that gesture
// writes. A wildcard fixture would delete the permission stage from the
// test while everything downstream stayed green (AP63), and for this flow
// permission is load-bearing: §2.4 makes following pull-only and
// permissionless, so the *reader* needs nothing — and the *publisher*
// still has to have said yes to somebody.
//
// # What a green run does NOT claim
//
// Nothing cross-implementation. Nothing about mDNS, restart or the static
// road. And nothing about ordering ACROSS publishers — §9.3 says there is
// no cross-publisher order in the data, so the merged list here is
// per-source concatenation and this file does not assert otherwise.

// feedPeerPair is a publisher that has posted and published, and a reader
// connected to it, both through `shellboot.Bootstrap`.
type feedPeerPair struct {
	author, reader     *entitysdk.AppPeer
	authorWS, readerWS *shellcmd.ShellWorkspace
	posts              []string
}

func newFeedPeerPair(t *testing.T, texts ...string) feedPeerPair {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	author, authorWS, err := shellboot.Bootstrap(ctx, shellboot.Config{
		LocalAlias: "author", ListenAddr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("bootstrap author: %v", err)
	}
	t.Cleanup(func() { _ = author.Close() })

	reader, readerWS, err := shellboot.Bootstrap(ctx, shellboot.Config{LocalAlias: "reader"})
	if err != nil {
		t.Fatalf("bootstrap reader: %v", err)
	}
	t.Cleanup(func() { _ = reader.Close() })

	for _, text := range texts {
		if _, err := authorWS.Post(ctx, shellcmd.FeedPostRequest{Text: text}); err != nil {
			t.Fatalf("post %q: %v", text, err)
		}
	}

	// ⛔ **Publishing is the act a reader can observe, and posting is
	// not.** A root commits to the trie as it stood at mint time, so
	// every post above is invisible to every reader until this line runs
	// — which is the finding `publish/feed_post_reach_live_test.go`
	// measures, and the reason this harness cannot post afterwards
	// without re-minting.
	//
	// `-public` because AP63 assembles grants at HANDSHAKE: the row has
	// to exist before the dial below, not after it.
	if _, err := authorWS.Publish(ctx, shellcmd.PublishRequest{
		Prefix: shellcmd.FeedPrefix, Public: true,
	}); err != nil {
		t.Fatalf("publish -public: %v", err)
	}

	ready := make(chan struct{})
	listenErr := make(chan error, 1)
	go func() { listenErr <- author.ListenReady(ctx, ready) }()
	select {
	case <-ready:
	case err := <-listenErr:
		t.Fatalf("author listen: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("author not listening after 5s")
	}

	conn, err := reader.Connect(ctx, author.Addr().String())
	if err != nil {
		t.Fatalf("reader connect: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	return feedPeerPair{
		author: author, reader: reader,
		authorWS: authorWS, readerWS: readerWS,
		posts: texts,
	}
}

// browserFor returns the workspace's browser as the shipped path builds
// it — by running a shipped verb that builds one.
//
// `bareBrowserOf` (`shellcmd/cmd_browse.go:92`) is unexported, and it is
// where the local peer is bound to the browser: the step that decides
// whether the live road is available at all. Constructing a
// `workbench.BrowseModel` here and calling `SetPeer` on it would make
// this fixture perform the exact wiring it exists to check, which is the
// failure `syncTestPeer`'s header records.
//
// A plain `timeline` is the right verb to obtain it with because it
// touches no durable state — no `-new`, so no cursor is read or written
// — and therefore getting the browser cannot change what is measured
// afterwards.
func browserFor(t *testing.T, ws *shellcmd.ShellWorkspace) *workbench.BrowseModel {
	t.Helper()
	sh := shellcmd.NewShellInWorkspace(ws)
	if _, err := shellcmd.Default().Dispatch(sh, "timeline", nil); err != nil {
		t.Fatalf("building the workspace browser through the shipped verb: %v", err)
	}
	if ws.Browser == nil {
		t.Fatal("the `timeline` verb ran and left this workspace with no browser, so nothing here " +
			"can take the live road and every feed read is about to fail for the wrong reason")
	}
	return ws.Browser
}

// timelineVerb runs the shipped verb and returns its rendered lines.
func timelineVerb(t *testing.T, ws *shellcmd.ShellWorkspace, args ...string) []string {
	t.Helper()
	sh := shellcmd.NewShellInWorkspace(ws)
	res, err := shellcmd.Default().Dispatch(sh, "timeline", args)
	if err != nil {
		t.Fatalf("timeline %v: %v", args, err)
	}
	if len(res.Lines) == 0 && res.Message != "" {
		return []string{res.Message}
	}
	return res.Lines
}

// TestFeedTimeline_TheVerbReadsAFollowedPeerAndSaysWhichPathAnsweredIt is
// the whole chain: follow → read → render, and the assertion is on the
// path as much as on the entries.
func TestFeedTimeline_TheVerbReadsAFollowedPeerAndSaysWhichPathAnsweredIt(t *testing.T) {
	pair := newFeedPeerPair(t, "the first entry", "the second entry", "the third entry")
	ctx := context.Background()
	authorID := pair.author.PeerID()

	if _, err := pair.readerWS.FeedFollow(shellcmd.FollowRequest{
		Subject: authorID, Label: "the author", Via: authorID,
	}); err != nil {
		t.Fatalf("follow: %v", err)
	}

	out, err := pair.readerWS.FeedTimeline(ctx, shellcmd.TimelineRequest{},
		browserFor(t, pair.readerWS))
	if err != nil {
		t.Fatalf("FeedTimeline: %v", err)
	}
	if len(out.Sources) != 1 {
		t.Fatalf("the view was assembled from %d sources, want the one followed peer", len(out.Sources))
	}
	src := out.Sources[0]
	if src.Err != "" {
		t.Fatalf("the followed peer was excluded: %s", src.Err)
	}
	if src.Label != "the author" {
		t.Errorf("the source lost its petname (%q) — §2.4's label is the reader's own and the only "+
			"human-readable thing in the view", src.Label)
	}

	// ⭐ The discrimination, at the layer that renders it. A correct entry
	// list reached through rule 6's fallback is the defect wearing the
	// fix, and the count alone cannot tell the two apart — which is
	// exactly what `entity-browser-rust`'s corridor ① result showed from
	// the other side.
	if src.Read.Via != "index" {
		t.Fatalf("the verb read via %q with an index published, want the index\nnotes: %v",
			src.Read.Via, src.Read.Notes)
	}
	if len(out.Merged) != len(pair.posts) {
		t.Fatalf("the view carries %d entries, want %d\nnotes: %v",
			len(out.Merged), len(pair.posts), src.Read.Notes)
	}
	for i, e := range out.Merged {
		if !e.Entry.Listed {
			t.Errorf("merged entry %d is not listed on a read that reports itself as indexed", i)
		}
		if e.Subject != authorID {
			t.Errorf("merged entry %d is attributed to %s in the view, want %s", i, e.Subject, authorID)
		}
		if e.Entry.Problem != "" {
			t.Errorf("merged entry %d: %s", i, e.Entry.Problem)
		}
	}

	// The rendered half. `FEED-R23` is met by a *view that declares what
	// produced it*, and a declaration computed and not printed meets
	// nothing — so this asserts on the operator's screen and not on the
	// struct that feeds it.
	lines := strings.Join(timelineVerb(t, pair.readerWS), "\n")
	if !strings.Contains(lines, "what produced this view") {
		t.Errorf("the verb prints no source block, so the view does not declare what produced it:\n%s", lines)
	}
	if !strings.Contains(lines, "via the index") {
		t.Errorf("the verb does not say which path answered. That sentence is the only place an "+
			"operator can see rule 6's fallback rescuing a broken index lookup:\n%s", lines)
	}
	if !strings.Contains(lines, "the author") {
		t.Errorf("the rendered view does not carry the petname:\n%s", lines)
	}
}

// TestFeedTimeline_AnUnreachablePublisherIsExcludedAndNamed is `FEED-R23`
// in the state a real timeline is in most of the time.
//
// A timeline of two peers where one laptop is shut **is a timeline**. So
// the unreachable one must become a named exclusion and must not empty
// the view — and the reachable one's entries must still be there, which
// is the half that a `return err` on the first failure would take away.
func TestFeedTimeline_AnUnreachablePublisherIsExcludedAndNamed(t *testing.T) {
	pair := newFeedPeerPair(t, "posted by the peer that is up")
	ctx := context.Background()

	stranger, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { stranger.Close() })

	for _, f := range []shellcmd.FollowRequest{
		{Subject: pair.author.PeerID(), Label: "up", Via: pair.author.PeerID()},
		{Subject: stranger.PeerID(), Label: "shut", Via: stranger.PeerID()},
	} {
		if _, err := pair.readerWS.FeedFollow(f); err != nil {
			t.Fatalf("follow %s: %v", f.Label, err)
		}
	}

	out, err := pair.readerWS.FeedTimeline(ctx, shellcmd.TimelineRequest{},
		browserFor(t, pair.readerWS))
	if err != nil {
		t.Fatalf("FeedTimeline returned an error for a view in which one of two peers is "+
			"unreachable. That is the ordinary state of a timeline, and an error here empties a "+
			"screen that has something correct to show: %v", err)
	}
	if len(out.Sources) != 2 {
		t.Fatalf("the view was assembled from %d sources, want both follows — a dropped source is an "+
			"exclusion nobody is told about", len(out.Sources))
	}

	var excluded, contributed int
	for _, s := range out.Sources {
		if s.Err != "" {
			excluded++
			if s.Subject != stranger.PeerID() {
				t.Errorf("the excluded source is %s, want the peer nobody is in touch with", s.Subject)
			}
			continue
		}
		contributed += len(s.Read.Entries)
	}
	if excluded != 1 {
		t.Errorf("%d sources were excluded, want exactly the unreachable one", excluded)
	}
	if contributed != len(pair.posts) {
		t.Errorf("the reachable peer contributed %d entries, want %d — one unreachable publisher "+
			"took the whole view down with it", contributed, len(pair.posts))
	}

	lines := strings.Join(timelineVerb(t, pair.readerWS), "\n")
	if !strings.Contains(lines, "EXCLUDED") {
		t.Errorf("the rendered view does not mark the unreachable peer as excluded, so a reader "+
			"cannot tell a peer with nothing to say from a peer nobody asked:\n%s", lines)
	}
	if !strings.Contains(lines, "posted by the peer that is up") {
		t.Errorf("the reachable peer's entry is missing from the rendered view:\n%s", lines)
	}
}

// TestFeedTimeline_NewAdvancesThePositionOnceAndSaysSoOnlyWhenItMoved
// gates the one piece of DURABLE state this layer owns.
//
// Two rules live here and both are easy to get backwards. A plain
// `timeline` must not touch the cursor, which is what makes it safe to
// run twice. And `Advanced` must mean *a position MOVED*, not *the flag
// was passed* — a read that found nothing new and reported "positions
// were advanced" is a surface describing its own mode rather than what
// happened, in the confident direction, about state that persists.
func TestFeedTimeline_NewAdvancesThePositionOnceAndSaysSoOnlyWhenItMoved(t *testing.T) {
	pair := newFeedPeerPair(t, "one", "two")
	ctx := context.Background()
	authorID := pair.author.PeerID()

	if _, err := pair.readerWS.FeedFollow(shellcmd.FollowRequest{Subject: authorID, Via: authorID}); err != nil {
		t.Fatalf("follow: %v", err)
	}

	// A plain read, twice. Neither may move the position.
	for i := 0; i < 2; i++ {
		out, err := pair.readerWS.FeedTimeline(ctx, shellcmd.TimelineRequest{},
			browserFor(t, pair.readerWS))
		if err != nil {
			t.Fatalf("plain timeline %d: %v", i, err)
		}
		if out.Advanced {
			t.Fatalf("a plain `timeline` advanced a read position on pass %d — the verb is then not "+
				"safe to run twice, and an operator loses entries by looking at them", i)
		}
		if len(out.Merged) != len(pair.posts) {
			t.Fatalf("plain timeline %d read %d entries, want %d", i, len(out.Merged), len(pair.posts))
		}
	}

	// `-new` once: everything, and the position moves.
	first, err := pair.readerWS.FeedTimeline(ctx, shellcmd.TimelineRequest{New: true},
		browserFor(t, pair.readerWS))
	if err != nil {
		t.Fatalf("timeline -new: %v", err)
	}
	if len(first.Merged) != len(pair.posts) {
		t.Fatalf("the first `-new` read %d entries, want %d", len(first.Merged), len(pair.posts))
	}
	if !first.Advanced {
		t.Fatal("the first `-new` read every entry and reported no advance, so the next one re-reads " +
			"them all and §4.4's O(new) buys nothing")
	}

	// `-new` again with nothing posted since: no entries, and — the arm
	// that matters — no claim of an advance.
	second, err := pair.readerWS.FeedTimeline(ctx, shellcmd.TimelineRequest{New: true},
		browserFor(t, pair.readerWS))
	if err != nil {
		t.Fatalf("second timeline -new: %v — a feed with nothing new is an empty result and never "+
			"an error (`FEED-R19`)", err)
	}
	if len(second.Merged) != 0 {
		t.Errorf("the second `-new` returned %d entries with nothing posted since the first",
			len(second.Merged))
	}
	if second.Advanced {
		t.Error("the second `-new` reported that read positions were advanced while nothing moved. " +
			"`Advanced` is then a description of the flag rather than of what happened, and it is " +
			"the one thing here an operator cannot check for themselves")
	}
}
