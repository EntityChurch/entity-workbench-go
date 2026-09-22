package shellcmd

import (
	"context"
	"fmt"
	"strings"

	"go.entitychurch.org/entity-core-go/core/hash"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/publish"
)

// feed_op.go — posting and reading this peer's own feed, extracted from the
// verb so a panel shares it (AP57).
//
// # WHAT THIS ADDS TO THE SDK, and it is one sentence
//
// `entitysdk.FeedAuthor` writes the convention. What it cannot know is
// **whether anybody can read what it wrote**, because that is three facts it
// has no business holding: is a root published, does its prefix cover the feed,
// and does it still commit to what the tree holds now. All three are invisible
// from inside the act of posting, and all three are the operator's to fix —
// which is AP84's shape exactly, so they are carried in the outcome and
// rendered beside the success line rather than instead of it.
//
// # POSTING IS NOT PUBLISHING, and the gap is silent in both directions
//
// A published root commits to a trie root taken when it was minted. A post
// binds an entry, a signature, a page and a head — **and moves none of them
// into the commitment.** So after a post:
//
//   - a live reader verifying against the published root cannot see the new
//     entry, because the root does not commit to it;
//   - a static reader gets whatever was in the directory at emit time;
//   - and both of those are indistinguishable, from the reader's side, from an
//     author who simply has not posted.
//
// Nothing in the substrate will mention it. `publish.RootNow` is the check, and
// **the binding count is not** — one post rewrites the index head in place,
// which changes no count at all, so a surface comparing counts would report a
// feed as published while serving a stale page set.
//
// **Re-publishing is not done implicitly.** Publishing is signing, and a verb
// that signs as a side effect of another verb is a publisher claiming a release
// it was not asked for — the same argument that keeps `PublishStatus` from
// minting. `post` says the root is behind; the operator runs `publish`.

// FeedPostRequest is one authored post as an operator asked for it.
type FeedPostRequest struct {
	// Text is the body. Carried inline as an embed (EMBED §3.1).
	Text string

	// ReplyTo, when non-zero, is the entry being replied to. The
	// conversation's root is INHERITED from it and never assumed to be it
	// — see entitysdk.FeedAuthor.ReplyTo.
	ReplyTo hash.Hash
}

// FeedPostOutcome is what one post did and what it did not.
type FeedPostOutcome struct {
	Entry entitysdk.PostedEntry
	Text  string

	// ReplyTo echoes the entry this one replies to, zero when it is not a
	// reply.
	//
	// Carried so the surface can say what a reply does NOT do. `FEED-R9`
	// is a **MUST NOT** on describing replies as notifying the author
	// absent a delivery grant, and a verb that says nothing at all is one
	// reasonable reading away from the thing the rule exists to prevent:
	// "reply" means notification everywhere else a person has used the
	// word. Inbound delivery is granted, not ambient (§3).
	ReplyTo hash.Hash

	// Reach is the same reading `feed` renders, taken after the post, so
	// the operator sees the consequence in the same breath as the act.
	Reach FeedReach
}

// FeedOutcome is `feed`: this peer's own feed plus who can read it.
type FeedOutcome struct {
	PeerID   string
	Readout  entitysdk.FeedReadout
	Reach    FeedReach
	Problems []string
}

// FeedReach is whether anything this peer posted can be read by anyone else.
//
// Three separate facts, deliberately not collapsed into one boolean: a feed
// that is unpublished, one published under a prefix that does not cover it, and
// one published-and-behind are three different operator actions, and the middle
// one is the quietest — everything looks published and the feed keys are not in
// the root.
type FeedReach struct {
	// Published reports that this peer has a signed root at all.
	Published bool

	// Prefix is what that root commits to.
	Prefix string

	// CoversFeed reports whether Prefix contains `app/feed/`.
	CoversFeed bool

	// Current reports whether the published root still commits to the tree
	// as it stands. False means posted-since-published.
	Current bool

	// RootHash is what is published; RootNow is what a root minted this
	// instant would commit to. Both printed when they differ, because
	// "your root is behind" is a claim an operator should be able to check
	// rather than believe.
	RootHash string
	RootNow  string

	// Public is the `default` policy row, reused from the publish surface
	// so the two verbs cannot describe one row differently.
	Public PublicGrantState

	// SignatureNote is the FEED-R2 attribution caveat — see
	// entitysdk.FeedAuthor.SignatureCoverage. Non-empty means a STATIC
	// reader of this feed cannot attribute a single entry.
	SignatureNote string
}

// FeedPrefix is the tree prefix a feed publish commits to.
//
// Both of this convention's pinned keys and this implementation's entry keys
// live under it: `app/feed/index`, `app/feed/index/{n}` and
// `app/feed/entries/{hex}`. It is NOT the whole story — see SignatureNote.
const FeedPrefix = "app/feed/"

// Post authors one entry into this peer's own feed.
func (ws *ShellWorkspace) Post(ctx context.Context, req FeedPostRequest) (FeedPostOutcome, error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return FeedPostOutcome{}, fmt.Errorf("post: no local peer")
	}
	author := ws.Local.Peer.Feed()

	post := entitysdk.PostRequest{Text: req.Text}
	if !req.ReplyTo.IsZero() {
		reply, err := author.ReplyTo(req.ReplyTo)
		if err != nil {
			return FeedPostOutcome{}, err
		}
		post.Reply = reply
	}

	entry, err := author.Post(post)
	if err != nil {
		return FeedPostOutcome{}, err
	}
	return FeedPostOutcome{
		Entry:   entry,
		Text:    req.Text,
		ReplyTo: req.ReplyTo,
		Reach:   ws.feedReach(ctx, author),
	}, nil
}

// Feed reads this peer's own feed back. limit 0 means everything.
func (ws *ShellWorkspace) Feed(ctx context.Context, limit int) (FeedOutcome, error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return FeedOutcome{}, fmt.Errorf("feed: no local peer")
	}
	author := ws.Local.Peer.Feed()
	readout, err := author.Read(limit)
	if err != nil {
		return FeedOutcome{}, err
	}
	out := FeedOutcome{
		PeerID:  ws.Local.Peer.PeerID(),
		Readout: readout,
		Reach:   ws.feedReach(ctx, author),
	}
	out.Problems = feedProblems(out.Readout, out.Reach)
	return out, nil
}

// feedReach answers the three questions the SDK cannot.
//
// It is a READ throughout: it reads the published root, rebuilds the trie over
// the prefix in memory, and reads the policy table. It mints nothing, dials
// nobody and writes nothing, which is what makes it safe on a panel refresh.
func (ws *ShellWorkspace) feedReach(ctx context.Context, author *entitysdk.FeedAuthor) FeedReach {
	ap := ws.Local.Peer
	var out FeedReach

	pr, err := ap.ReadPublishedRoot(ctx, ap.PeerID())
	if err == nil {
		out.Published = true
		out.Prefix = pr.Data.Prefix
		out.RootHash = pr.Data.RootHash.String()
		out.CoversFeed = prefixCoversFeed(pr.Data.Prefix)

		// Rebuilt over the PUBLISHED prefix rather than over the feed's —
		// the question is whether the root describes the tree, and the root
		// is the one that chose the prefix.
		if now, err := publish.RootNow(ap, trimPublishedPrefix(pr.Data.Prefix)); err == nil {
			out.RootNow = now.String()
			out.Current = now == pr.Data.RootHash
		}
	}
	out.Public = ws.publicGrantState(out.Prefix)
	_, out.SignatureNote = author.SignatureCoverage(FeedPrefix)
	return out
}

// prefixCoversFeed reports whether a published prefix contains the feed's keys.
//
// Segment-exact, never substring: `app/feedback/` starts with `app/feed` and
// contains nothing of this convention. Same discriminator the ref classifier
// and the layout rebaser use, and the reason both are written this way is that
// the substring version is right on every example anybody thinks of.
func prefixCoversFeed(publishedPrefix string) bool {
	p := strings.Trim(strings.TrimSpace(trimPublishedPrefix(publishedPrefix)), "/")
	if p == "" {
		return true // the whole tree
	}
	want := strings.Trim(FeedPrefix, "/")
	return p == want || strings.HasPrefix(want+"/", p+"/")
}

// trimPublishedPrefix converts §3.3a's stored form to the peer-relative form
// the trie builder takes — which is the SAME STRING THE MINT USED, trailing
// slash and all.
//
// ⚠ **The trailing slash is load-bearing and dropping it was a real defect,
// found by running the flow rather than by reading it.** A trie's keys are
// relative to its prefix, so `app/feed` and `app/feed/` produce different
// relative keys (`/index` versus `index`) and therefore different roots over
// identical bytes. The first version trimmed both ends, so `feed` reported
// *"the published root is BEHIND this tree"* on a peer that had published
// three seconds earlier and posted nothing — a permanent problem line, which
// is how an operator learns to skip the problem list.
//
// This is `currentPublishPrefix`'s conversion and it stays identical to it on
// purpose: two spellings of "what prefix is published" that disagree by one
// character is exactly what produced the false alarm.
func trimPublishedPrefix(prefix string) string {
	if prefix == "/" {
		// §3.3a spells the universal tree "/" inside the root; the
		// peer-relative form the builder lists on is "".
		return ""
	}
	return prefix
}

// feedProblems is the "what is not done" list, in the order an operator would
// act on it. Each line names the command that fixes it, on the machine they are
// already looking at.
func feedProblems(readout entitysdk.FeedReadout, reach FeedReach) []string {
	var out []string
	if !readout.Published {
		// Nothing posted. Every line below would be about a feed that does
		// not exist, and a list of four problems is how an operator learns
		// to skip the list.
		return nil
	}
	switch {
	case !reach.Published:
		out = append(out, "this peer has published no signed root, so nothing can read this feed and verify "+
			"it — `publish -prefix "+FeedPrefix+"` signs one")
	case !reach.CoversFeed:
		out = append(out, fmt.Sprintf(
			"the published root commits to %q, which does not contain %q — a reader following this peer "+
				"gets a verified answer that the feed is not there. A peer has exactly ONE published root, "+
				"so `publish -prefix %s` REPLACES what is published now",
			reach.Prefix, FeedPrefix, FeedPrefix))
	case !reach.Current:
		out = append(out, "the published root does not commit to what is in the tree now — posts made since "+
			"the last `publish` are invisible to every reader, on both roads, and look exactly like not "+
			"having posted. Run `publish` again")
	}
	if reach.Published && reach.CoversFeed && !reach.Public.Present {
		out = append(out, "no peer is authorized to read this feed: it is signed and in the tree and "+
			"`publish -public` is what opens it to any peer that can dial this one")
	}
	if reach.SignatureNote != "" && reach.Published && reach.CoversFeed {
		out = append(out, reach.SignatureNote)
	}
	return out
}
