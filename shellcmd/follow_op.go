package shellcmd

import (
	"context"
	"fmt"
	"time"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"
)

// follow_op.go — following somebody's feed, and reading what they posted.
//
// # Following establishes nothing at the far end, and that is the design
//
// `APP-CONVENTION-FEED` §2.4 makes a feed-follow a follow of a
// **namespace**: *public, pull-only, requiring no grant and no permission,
// and the publisher does not know the follower exists.* That is the whole
// discriminator against `app/share/follow`, which follows a **grant** and
// therefore tells the publisher who you are.
//
// ⚠ **THIS CORRECTS THE PLAN OF RECORD, WHICH WAS WRITTEN AGAINST AN
// EARLIER DRAFT.** `docs/architecture/LIVE-PEER-DIRECTION.md` §3 obligation
// 3 says *"`app/feed/follow` as a subscription, not a poll"*, quoting the
// vocabulary table's one-line role — *"a reader's durable subscription to
// a peer's feed"* — and proposing the kernel subscription engine as the
// mechanism. A kernel subscription is registered AT the publisher: it
// needs a grant, and it tells them a follower exists. Building a follow on
// it would make the feed-follow the thing §2.4 says it is not, and would
// collapse the distinction the convention draws between its two follow
// types. §7.6 states the reader loop this convention actually specifies —
// *fetch the published root, verify one signature, read the index head,
// read down to your cursor and stop* — which is a bounded PULL.
//
// So: a follow is a durable local declaration; a read derives from it; and
// a subscription-based arrival remains a legitimate OPTIMIZATION for a
// publisher who has granted one, which nothing here requires and which
// must never be described as how following works.
//
// # A read and a catch-up are different operations
//
// `timeline` reads and does not move the cursor. `timeline -new` reads
// what is new since the cursor and advances it. Same split as `status`
// versus the reconcile pass, for the same reason: a surface somebody
// refreshes must not quietly change durable state, and a reader who wants
// `O(new)` is asking for the other operation.

// FollowRequest is one follow as an operator asked for it.
type FollowRequest struct {
	// Subject is the peer whose feed to follow.
	Subject string
	// Label is §2.4's petname — local, chosen by the reader, never
	// authoritative and never transmitted as a claim about anyone.
	Label string
	// Via records the identifier as typed, for provenance display.
	Via string
}

// FollowOutcome is what a follow did.
type FollowOutcome struct {
	Follow  entitysdk.FeedFollowData
	Updated bool // an existing follow was relabelled rather than created

	// Reachable reports whether this peer can currently reach the
	// subject, and Why says what is in the way. **Not a precondition** —
	// §2.4 requires no permission and no relationship, so a follow of a
	// peer who is asleep is a perfectly good follow. Reported so the
	// operator is not left wondering why `timeline` shows nothing.
	Reachable bool
	Why       string
}

// FeedFollow records a follow. It dials nobody and asks nobody.
func (ws *ShellWorkspace) FeedFollow(req FollowRequest) (FollowOutcome, error) {
	st, err := ws.feedStore()
	if err != nil {
		return FollowOutcome{}, err
	}
	if req.Subject == ws.Local.Peer.PeerID() {
		return FollowOutcome{}, fmt.Errorf("this peer IS %s — `feed` reads its own posts, and it reads "+
			"them out of its own tree rather than through a published root", req.Subject)
	}
	f := entitysdk.FeedFollowData{
		Subject: req.Subject,
		Label:   req.Label,
		Via:     req.Via,
		Since:   uint64(time.Now().UnixMilli()),
	}
	out := FollowOutcome{}
	if prior, ok := workbench.LoadFeedFollow(st, req.Subject); ok {
		// The relationship is the same one; only the petname moved.
		f.Since = prior.Since
		if f.Via == "" {
			f.Via = prior.Via
		}
		out.Updated = true
	}
	if err := workbench.SaveFeedFollow(st, f); err != nil {
		return FollowOutcome{}, err
	}
	out.Follow = f
	out.Reachable, out.Why = ws.canReachSubject(req.Subject)
	return out, nil
}

// FeedUnfollow drops a follow and the read position that went with it.
func (ws *ShellWorkspace) FeedUnfollow(subject string) (bool, error) {
	st, err := ws.feedStore()
	if err != nil {
		return false, err
	}
	return workbench.RemoveFeedFollow(st, subject), nil
}

// FollowsOutcome is the `follows` reading.
type FollowsOutcome struct {
	Rows     []FollowRow
	Problems []string
}

// FollowRow is one follow with the facts a surface needs ON THE ROW.
//
// Reachability used to exist only as a line in `Problems`, spelled
// `subject + ": " + why`. That is fine for a terminal and unusable for
// anything else: a renderer with a table of follows would have to parse a
// prose string back into a key to find out which row it was about, and a
// petname containing `": "` would make it parse wrong. The GUI needed the
// fact, so the fact moved onto the row.
//
// **`Problems` keeps the same sentences**, because the two are different
// questions — *"is there anything wrong here"* is answered by a list, and
// *"can I read THIS one"* is answered by a row — and because a surface
// that renders only the rows would otherwise silently drop §2.4's privacy
// sentence, which is about the peer and belongs to no row at all.
// Produced in one place either way, so the two cannot disagree.
type FollowRow struct {
	workbench.FeedFollowRow

	// Reachable reports whether this peer can currently reach the
	// subject, and Why says what is in the way.
	//
	// ⚠ **Not a precondition, and a surface must not render it as an
	// error.** §2.4 requires no permission and no relationship, so a
	// follow of a peer who is asleep is a perfectly good follow. It is
	// reported so an operator is not left wondering why `timeline` shows
	// nothing.
	Reachable bool
	Why       string
}

// FeedFollows lists what this peer follows. A read: nothing is dialed.
func (ws *ShellWorkspace) FeedFollows(ctx context.Context) (FollowsOutcome, error) {
	st, err := ws.feedStore()
	if err != nil {
		return FollowsOutcome{}, err
	}
	loaded, problems := workbench.LoadFeedFollows(st)
	out := FollowsOutcome{Rows: make([]FollowRow, 0, len(loaded)), Problems: problems}

	// §2.4's privacy sentence, checked against what this peer actually
	// publishes rather than asserted. Nothing else in the chain will
	// mention it: the records are well-formed and the publish is correct.
	if pr, err := ws.Local.Peer.ReadPublishedRoot(ctx, ws.Local.Peer.PeerID()); err == nil {
		if p := workbench.FeedPrivacyProblem(pr.Data.Prefix); p != "" {
			out.Problems = append(out.Problems, p)
		}
	}
	for _, r := range loaded {
		ok, why := ws.canReachSubject(r.Follow.Subject)
		out.Rows = append(out.Rows, FollowRow{FeedFollowRow: r, Reachable: ok, Why: why})
		if !ok {
			out.Problems = append(out.Problems, r.Follow.Subject+": "+why)
		}
	}
	return out, nil
}

// TimelineRequest is one read across the followed set.
type TimelineRequest struct {
	// Subject, when set, reads exactly one peer instead of every follow.
	// It does NOT have to be followed — reading a feed requires nothing
	// of the reader either.
	Subject string

	// Limit caps entries per subject. 0 means everything the read reaches.
	Limit int

	// New reads from the stored cursor and ADVANCES it — §4.3 rule 4's
	// `O(new)`. Without it the cursor is neither read nor written, which
	// is what makes plain `timeline` safe to run twice.
	New bool
}

// TimelineOutcome is a multi-publisher view, and `FEED-R23` is why it
// carries its sources rather than only its entries.
type TimelineOutcome struct {
	// Sources is every subject this view was assembled from, in the order
	// they were read, each with what it contributed and what went wrong.
	// **A view of more than one publisher MUST declare what produced it**
	// — sources, mirrors, exclusions — and a list of entries with no
	// provenance is exactly the view that MUST NOT exist.
	Sources []TimelineSource

	// Advanced reports that this read moved the stored cursors.
	Advanced bool

	// Merged is every entry, grouped by source in Sources order.
	// **Deliberately not interleaved by time**: §9.3 says there is no
	// cross-publisher order in the data and this convention does not
	// invent one, and `created_at` is an unverifiable clock (§2.3.2)
	// which sorting by would quietly promote to an authority.
	Merged []TimelineEntry
}

// TimelineSource is one publisher's contribution to a view.
type TimelineSource struct {
	Subject string
	Label   string
	Read    workbench.FeedRead
	// Err is why this source contributed nothing, when it did not. A
	// source that could not be reached is an EXCLUSION and is named as
	// one; dropping the row would make the view claim completeness it
	// does not have.
	Err string
}

// TimelineEntry is one entry with the publisher it came from attached.
type TimelineEntry struct {
	Subject string
	Label   string
	Entry   workbench.FeedEntryRead
}

// FeedTimeline reads the followed set (or one subject) through the
// verified road.
func (ws *ShellWorkspace) FeedTimeline(ctx context.Context, req TimelineRequest, browser *workbench.BrowseModel) (TimelineOutcome, error) {
	st, err := ws.feedStore()
	if err != nil {
		return TimelineOutcome{}, err
	}
	if browser == nil {
		return TimelineOutcome{}, fmt.Errorf("no browser: a feed is read through the same verifying " +
			"reader a page is, so there is no second road to fall back on")
	}

	type target struct{ subject, label string }
	var targets []target
	switch {
	case req.Subject != "":
		label := ""
		if f, ok := workbench.LoadFeedFollow(st, req.Subject); ok {
			label = f.Label
		}
		targets = append(targets, target{req.Subject, label})
	default:
		rows, _ := workbench.LoadFeedFollows(st)
		for _, r := range rows {
			targets = append(targets, target{r.Follow.Subject, r.Follow.Label})
		}
	}

	out := TimelineOutcome{}
	for _, t := range targets {
		src := TimelineSource{Subject: t.subject, Label: t.label}
		opts := workbench.FeedReadOpts{Limit: req.Limit}
		if req.New {
			if cur, _, ok := workbench.LoadFeedCursor(st, t.subject); ok {
				opts.Cursor = cur
			}
		}
		before := opts.Cursor
		read, err := browser.ReadFeedOf(ctx, t.subject, opts)
		if err != nil {
			// Named, never dropped: an unreachable publisher is an
			// exclusion from this view and `FEED-R23` makes the view say
			// so. It is also NOT an error for the verb — a timeline of
			// five peers where one laptop is shut is a timeline.
			src.Err = err.Error()
			out.Sources = append(out.Sources, src)
			continue
		}
		src.Read = read
		out.Sources = append(out.Sources, src)
		for _, e := range read.Entries {
			out.Merged = append(out.Merged, TimelineEntry{Subject: t.subject, Label: t.label, Entry: e})
		}
		// **Advanced means a position MOVED, not that the flag was
		// passed.** A read that found nothing new and reported "positions
		// were advanced" is a surface describing its own mode rather than
		// what happened, which is the confident-direction error in one
		// line — and this one is about durable state.
		if req.New && !read.Cursor.Applied.IsZero() && read.Cursor != before {
			if err := workbench.SaveFeedCursor(st, t.subject, read.Cursor, uint64(time.Now().UnixMilli())); err != nil {
				src.Err = "read, but the new position was not recorded: " + err.Error()
				out.Sources[len(out.Sources)-1] = src
				continue
			}
			out.Advanced = true
		}
	}
	return out, nil
}

// canReachSubject answers §2.4's one practical precondition: following
// needs no permission, and reading needs a route.
func (ws *ShellWorkspace) canReachSubject(subject string) (bool, string) {
	ap := ws.Local.Peer
	if subject == ap.PeerID() {
		return true, ""
	}
	for _, p := range ap.ConnectedPeers() {
		if p.PeerID == subject {
			return true, ""
		}
	}
	// The verb's own spelling, checked by running it: `connect` takes an
	// alias AND an address. A guidance line that names a command form the
	// program refuses is worse than none — it is the sentence the operator
	// types next (AP71).
	return false, "no connection to this peer, and a peer-id carries no address to dial — " +
		"`connect <alias> <host:port>` or discovery has to have reached them before their feed can " +
		"be read. The follow itself is fine: it needs nothing from them"
}

// feedStore is the one nil-check these operations share.
func (ws *ShellWorkspace) feedStore() (*workbench.Store, error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return nil, fmt.Errorf("no local peer")
	}
	return ws.Local.Peer.Store(), nil
}
