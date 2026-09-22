package workbench

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/fetch"
)

// feed_read.go — reading SOMEBODY ELSE'S feed, through the signed root
// they published.
//
// # What this is, against what was here before
//
// `entitysdk.FeedAuthor.Read` reads **our own** tree with our own
// authority and says so in its doc comment: no attribution, no
// enumeration fallback, no resume. This is the other one. Everything it
// reads comes out of a [fetch.Consumer] — verified root, two-hop
// signature, `seq` floor, fail-closed CHAMP walk — so the subject peer
// cannot serve a reader anything its own signed root did not commit to.
//
// # The four reader rules this implements, and why each is not optional
//
//   - **`FEED-R1` (§1.1) — reject an entry whose `author` differs from
//     the namespace it was found under.** A namespace is the forgery
//     gate: without the check, a publisher can serve an entry claiming
//     anybody's authorship and a reader renders it under that name.
//   - **`FEED-R4` (§1.1.1) — an entry whose detached signature is absent
//     is UNATTRIBUTED, not attributed.** *Found under a namespace* and
//     *signed by that peer* are different facts, and the first is what a
//     mirror can copy. Withholding a signature is the most a hostile
//     party can do (see [fetch.Consumer.SignatureOver]), so this is a
//     rendering state and never an error.
//   - **`FEED-R13` (§4.3 rule 6) — the index is an optimization and MUST
//     NOT be the authority.** A reader that cannot fetch it falls back to
//     enumerating the prefix: *slower, same answer*. Were the index
//     authoritative it would be a place for a publisher to lie by
//     omission about their own posts, with no way for a reader to tell an
//     omission from an absence.
//   - **`FEED-R14` (§4.4) — if the cursor's `applied` hash no longer
//     resolves, resume from `page`.** An author may remove the very entry
//     a reader was holding as its position, and a cursor that cannot
//     survive that is a cursor that breaks on edit.
//
// Two more that are absences rather than code, and are easy to
// reintroduce: **`FEED-R8`** — nothing here reads `created_at` for
// anything but display, and no entry is refused for an implausible one —
// and **`FEED-R19`** — a feed with nothing new is an empty result and
// never an error, because staleness is the publisher's ordinary state.
//
// # What it deliberately does not do
//
// It does not merge publishers. `FEED-R23` makes a multi-publisher view
// declare what produced it, and §9.3 says there is no cross-publisher
// order in the data — so merging is a surface decision and is made at the
// surface (`shellcmd/follow_op.go`), with the sources named.

// FeedReadVia is how the entry list was obtained.
type FeedReadVia string

const (
	// FeedViaIndex is §4.3's cheap path: the head, then pages downward
	// from `current`, stopping at the cursor. `O(new)`.
	FeedViaIndex FeedReadVia = "index"

	// FeedViaEnumeration is rule 6's fallback: every key the signed root
	// commits to, filtered by entity type. **Slower, same answer** — and
	// the reason the index cannot lie by omission.
	FeedViaEnumeration FeedReadVia = "enumeration"
)

// DefaultFeedEnumerationBudget bounds the rule-6 fallback.
//
// The fallback fetches committed bodies to find out which are entries,
// which is `O(all)` by construction — that is what the rule trades for
// not trusting the index. Bounded because an unbounded walk driven by a
// remote publisher's key count is our CPU spent at their discretion
// (AP93), and **what was dropped is always disclosed**: a truncated view
// reported as a complete one is the omission this rule exists to catch,
// arriving from our own side instead.
const DefaultFeedEnumerationBudget = 512

// FeedReadOpts tunes one read.
type FeedReadOpts struct {
	// Limit caps entries returned. 0 means everything the read reaches.
	Limit int

	// Cursor is §4.4's position: the page reached and the newest entry
	// taken from it. Zero means "read from the head".
	//
	// **It is local reader state and nothing publishes it** — `[v0.3]`
	// removed the field from the follow record for exactly that reason.
	Cursor entitysdk.FeedCursor

	// EnumerationBudget overrides [DefaultFeedEnumerationBudget].
	EnumerationBudget int
}

// FeedEntryRead is one entry as a reader sees it — which is not the same
// shape as one an author sees, because half of these fields are the
// reader's own findings about it.
type FeedEntryRead struct {
	Hash hash.Hash
	Page uint64

	// Listed reports that the index named this entry. False means it was
	// found by enumeration, which `FEED-R13` makes just as valid.
	Listed bool

	CreatedAt uint64
	Text      string
	MediaType string
	IsReply   bool

	// Attributed is `FEED-R4`: a detached `system/signature` over these
	// exact bytes verified against the key this peer-id carries.
	//
	// **Not the same question as "did it arrive under their namespace".**
	// A mirror republishes other people's entries verbatim, so the
	// namespace an entry is found under says who is serving it and the
	// signature says who wrote it.
	Attributed bool

	// Attribution says why not, when not. Always populated when
	// Attributed is false, because *"unattributed"* on its own reads as a
	// defect in the reader.
	Attribution string

	// Rejected is `FEED-R1`: the entry claims an author other than the
	// namespace it was found under. The row is KEPT and its body is not
	// rendered — dropping it silently would make this list disagree with
	// the index it came from, and the rejected row is the interesting one.
	Rejected bool

	// Problem is a per-entry fault: it did not decode, or its bytes were
	// not served. One bad entry is a fact about that entry and must not
	// end the read.
	Problem string
}

// FeedRead is one subject's feed as this reader obtained it.
type FeedRead struct {
	Subject string

	// Published reports that the subject has a signed root at all. It is
	// distinct from an empty Entries list: *never published*, *published
	// and committing to no feed*, and *a feed with nothing new since your
	// cursor* are three different things and only the first two are
	// anybody's problem.
	Published bool

	// Prefix is what that root commits to, as the root spells it.
	Prefix string

	Via     FeedReadVia
	Entries []FeedEntryRead

	// Pages is `current` + 1 from the head, 0 when the head was not read.
	Pages uint64

	// Cursor is where a next read should resume from. Unchanged from the
	// caller's when nothing was read.
	Cursor entitysdk.FeedCursor

	// Resumed is `FEED-R14` having fired: the cursor's `applied` hash was
	// not found in the range read, so the read resumed from its page. Not
	// an error — an author removed an entry, which they are allowed to do.
	Resumed bool

	// Truncated reports that a limit or the enumeration budget stopped the
	// read before it reached the end of its range.
	Truncated bool

	// Freshness is the one sentence about what this reading is worth,
	// composed by [fetch.VerifiedRoot.Freshness] so a feed cannot describe
	// a root differently from the way a page does.
	Freshness string

	// Notes are facts about this read an operator would otherwise have to
	// infer: the fallback firing, the budget stopping, a page the root
	// does not commit to.
	Notes []string
}

// NewestFirst is the order a renderer shows a single publisher's feed in:
// the order the publisher authored (§4.5), which is what the pages
// already carry. It exists so a caller never reaches for a sort.
func (r FeedRead) NewestFirst() []FeedEntryRead { return r.Entries }

// ReadFeed reads one publisher's feed through their signed root.
//
// The error return is for the publisher being unreachable, unpublished or
// unverifiable. **Everything else is a field**: a missing index, a page
// the root does not commit to, an entry that will not decode, an entry
// nobody signed, and an entry claiming somebody else's authorship are all
// findings, and a read that turns any of them into an error tells the
// operator their own machine is broken.
func ReadFeed(ctx context.Context, c *fetch.Consumer, opts FeedReadOpts) (FeedRead, error) {
	if c == nil {
		return FeedRead{}, fmt.Errorf("workbench: reading a feed needs a verifying reader")
	}
	subject := c.PeerID()
	out := FeedRead{Subject: subject, Cursor: opts.Cursor}

	root, err := c.VerifiedRoot(ctx)
	if err != nil {
		return FeedRead{}, err
	}
	walk, err := c.Walk(ctx, root.Data.RootHash)
	if err != nil {
		return FeedRead{}, err
	}
	out.Published, out.Prefix, out.Freshness = true, root.Data.Prefix, root.Freshness()

	// The committed key set, in the peer-relative form §4.2 pins its two
	// addresses in. A walk's keys are relative to the PUBLISHED PREFIX and
	// §4.2's addresses are not (§3.3a: `prefix + relative_key`), and a
	// reader that gets this wrong sees an empty feed with a valid
	// signature over it — the most confident wrong answer available.
	//
	// ⛔ **This used to be `root.Data.Prefix + b.Key` and that is wrong for
	// one of the three admissible prefix shapes.** §3.3a spells the
	// universal tree `"/"` and the peer-relative form without a leading
	// slash, so verbatim concatenation yields `/app/feed/index` for a
	// peer-root publish and `app/feed/index` for every other — and §4.2
	// pins the second. `fetch.AbsolutePath` is the one place that resolves
	// all three, its doc comment records this exact lesson from the
	// consumer's own build, and this function reimplemented the join
	// instead of calling it.
	//
	// ⭐ **The reason nothing caught it is the part to keep: §4.3 rule 6
	// hid it completely.** The index lookup missed, the reader fell back
	// to enumeration exactly as `FEED-R13` requires, and returned *the
	// same 34 entries* — right answer, no error, no note a caller would
	// read as a defect. Measured on the corridor fixture, which is the
	// first feed in this tree published over anything but `app/feed/`:
	// **0 of 34 found by index and 34 by fallback**, where the narrow cut
	// is 34 and 0. A conformance fallback designed to survive a
	// withholding publisher will equally survive your own broken primary
	// path, so **assert WHICH path answered**, not just the answer —
	// `publish/feed_corridor_test.go` does, on `Listed`.
	committed := make(map[string]hash.Hash, len(walk.Bindings))
	absPeer := "/" + subject + "/"
	for _, b := range walk.Bindings {
		abs := fetch.AbsolutePath(root.Data.Prefix, subject, b.Key)
		committed[strings.TrimPrefix(abs, absPeer)] = b.Hash
	}

	head, haveHead := readIndexHead(ctx, c, committed, &out)
	if !haveHead {
		// §4.3 rule 6. The index is an optimization; its absence is a cost
		// and never an answer.
		out.Via = FeedViaEnumeration
		enumerateFeed(ctx, c, walk, subject, opts, &out)
		return out, nil
	}

	out.Via = FeedViaIndex
	out.Pages = head.Current + 1
	readIndexPages(ctx, c, committed, head, subject, opts, &out)
	return out, nil
}

// readIndexHead resolves §4.2's one pinned address.
func readIndexHead(ctx context.Context, c *fetch.Consumer, committed map[string]hash.Hash,
	out *FeedRead) (entitysdk.FeedIndexHeadData, bool) {

	h, ok := committed[entitysdk.FeedIndexPath]
	if !ok {
		out.Notes = append(out.Notes, fmt.Sprintf(
			"this publisher's signed root (%q) does not commit to %q, so there is no index to read — "+
				"enumerating instead, which is slower and reaches the same entries (§4.3 rule 6)",
			out.Prefix, entitysdk.FeedIndexPath))
		return entitysdk.FeedIndexHeadData{}, false
	}
	ent, err := c.Blob(ctx, h)
	if err != nil {
		out.Notes = append(out.Notes, "the index head is committed and its bytes were not served ("+
			err.Error()+") — enumerating instead")
		return entitysdk.FeedIndexHeadData{}, false
	}
	head, err := entitysdk.FeedIndexHeadFromEntity(ent)
	if err != nil {
		out.Notes = append(out.Notes, "the index head did not decode ("+err.Error()+") — enumerating instead")
		return entitysdk.FeedIndexHeadData{}, false
	}
	return head, true
}

// readIndexPages is §4.3 rules 2 and 4: jump to the cursor's page, read
// downward from `current`, stop.
func readIndexPages(ctx context.Context, c *fetch.Consumer, committed map[string]hash.Hash,
	head entitysdk.FeedIndexHeadData, subject string, opts FeedReadOpts, out *FeedRead) {

	floor := uint64(0)
	if head.Oldest != nil {
		floor = *head.Oldest
	}
	// The cursor's page is the floor of THIS read — rule 2's jump. A
	// reader holding page 12 fetches page 12, it does not walk down from
	// the head to reach it.
	resuming := !opts.Cursor.Applied.IsZero()
	if resuming && opts.Cursor.Page > floor {
		floor = opts.Cursor.Page
	}
	if head.Current < floor {
		// The publisher's whole index is below where we resumed from —
		// pages were dropped past our position. Rule 3 forbids
		// renumbering, so this is removal, and the honest read is the
		// whole remaining index.
		out.Notes = append(out.Notes, fmt.Sprintf(
			"this reader's position was page %d and the index now ends at page %d — entries have been "+
				"removed, so the read restarted at the oldest page still published",
			opts.Cursor.Page, head.Current))
		floor, resuming = 0, false
		out.Resumed = true
	}

	reachedCursor := false
	for p := int64(head.Current); p >= int64(floor); p-- {
		page := uint64(p)
		key := entitysdk.FeedIndexPagePath(page)
		h, ok := committed[key]
		if !ok {
			// Reported rather than refused: a gap in an index is a fact
			// about the publisher, and a read that dies on it shows the
			// operator nothing at all.
			out.Notes = append(out.Notes, fmt.Sprintf("the root does not commit to %q, so that page's "+
				"entries are not in this view", key))
			continue
		}
		ent, err := c.Blob(ctx, h)
		if err != nil {
			out.Notes = append(out.Notes, fmt.Sprintf("index page %d is committed and was not served (%v)", page, err))
			continue
		}
		pageData, err := entitysdk.FeedIndexPageFromEntity(ent)
		if err != nil {
			out.Notes = append(out.Notes, fmt.Sprintf("index page %d did not decode (%v)", page, err))
			continue
		}
		if pageData.Page != page {
			// §4.2: the page's own number MUST equal its key, which is what
			// makes a moved page detectably moved.
			out.Notes = append(out.Notes, fmt.Sprintf("the page bound at %q says it is page %d — §4.2 "+
				"makes the key and the body agree, so this page has been moved", key, pageData.Page))
		}

		// **No page size is assumed** (`FEED-R12`): the page says how many
		// entries it has by having them.
		for _, ref := range pageData.Entries {
			if resuming && ref.Hash != nil && *ref.Hash == opts.Cursor.Applied {
				// Everything from here down has been seen.
				reachedCursor = true
				break
			}
			if opts.Limit > 0 && len(out.Entries) >= opts.Limit {
				out.Truncated = true
				return
			}
			out.Entries = append(out.Entries, readIndexedEntry(ctx, c, subject, ref, page))
		}
		if reachedCursor {
			break
		}
	}

	if resuming && !reachedCursor {
		// `FEED-R14`. The entry this reader was holding as its position is
		// gone from the pages it covers, so the read resumed from the page
		// number — which is the whole reason the cursor carries one.
		out.Resumed = true
		out.Notes = append(out.Notes, fmt.Sprintf(
			"the entry this reader was holding as its position (%s) is no longer in page %d — the author "+
				"removed it, which they may do. Resumed from the page number instead, so some entries "+
				"here may have been seen before", shortRefHash(opts.Cursor.Applied), opts.Cursor.Page))
	}
	advanceCursor(out, head.Current)
}

// readIndexedEntry fetches and checks one row of a page.
func readIndexedEntry(ctx context.Context, c *fetch.Consumer, subject string,
	ref entitysdk.EntityRef, page uint64) FeedEntryRead {

	e := FeedEntryRead{Page: page, Listed: true}
	if !ref.IsPinned() || ref.Hash == nil {
		// §2.2.1: an index row is a pin. A live row would make an archive
		// editable underneath the readers holding positions in it.
		e.Problem = fmt.Sprintf("index page %d carries a %q row; §4.2 pins every entry", page, ref.Tag)
		e.Attribution = "not fetched"
		return e
	}
	e.Hash = *ref.Hash
	ent, err := c.Blob(ctx, e.Hash)
	if err != nil {
		e.Problem = "the index names this entry and its bytes were not served: " + err.Error()
		e.Attribution = "not fetched"
		return e
	}
	fillEntry(ctx, c, subject, ent, &e)
	return e
}

// fillEntry decodes one entry and applies `FEED-R1` and `FEED-R4`.
func fillEntry(ctx context.Context, c *fetch.Consumer, subject string, ent entity.Entity, e *FeedEntryRead) {
	e.Hash = ent.ContentHash
	data, err := entitysdk.FeedEntryFromEntity(ent)
	if err != nil {
		e.Problem = "this entry did not decode: " + err.Error()
		e.Attribution = "undecodable"
		return
	}
	if err := data.ValidateInNamespace(subject); err != nil {
		// `FEED-R1`, the forgery gate. Kept in the list and not rendered.
		e.Rejected = true
		e.Problem = err.Error()
		e.Attribution = "rejected before attribution was considered"
		return
	}
	e.CreatedAt = data.CreatedAt
	e.MediaType = data.Body.MediaType()
	e.IsReply = data.Reply != nil
	if data.Body.Data.Payload.Tag == entitysdk.EmbedPayloadInline {
		e.Text = string(data.Body.Data.Payload.Bytes)
	} else {
		// C-6. `fallback` is EMBED §3's mandatory rung and this is the
		// branch that renders it — a `child` or `ref` payload has nothing
		// else to show a reader who cannot run its handler. Before the
		// check, a missing or empty fallback here produced **a blank row
		// with no problem on it**, which reads as an author posting
		// nothing rather than as a producer omitting a mandatory field.
		//
		// The entry is KEPT and not dropped, for `FEED-R1`'s reason: the
		// author wrote it, the index names it, and a list that quietly
		// disagrees with the index it came from is the worse artifact.
		// State the fault on the row instead.
		if err := data.Body.Data.ValidateDecodedNested(ent.Data, "body"); err != nil {
			e.Problem = "this entry's body is not renderable: " + err.Error()
		}
		e.Text = data.Body.Data.Fallback
	}
	attribute(ctx, c, ent, e)
}

// attribute is `FEED-R4`, and its failure arm is the normal one.
//
// The signature lives at `system/signature/{hex}`, which no feed-shaped
// published prefix contains — so on a STATIC corridor this is always
// unreachable and every entry is honestly unattributed, while a reader
// that dialled the publisher gets it on the same grant that carries the
// root's own signature. Both are correct readings of what was available.
func attribute(ctx context.Context, c *fetch.Consumer, ent entity.Entity, e *FeedEntryRead) {
	if _, where, err := c.SignatureOver(ctx, "feed entry", ent); err != nil {
		e.Attributed = false
		e.Attribution = "no verified signature for these bytes at " + where + " (" + err.Error() +
			"). Found under this peer's namespace is not the same fact as written by them — §1.1 makes " +
			"the detached signature what attributes an entry once it leaves its author's tree"
		return
	}
	e.Attributed = true
}

// enumerateFeed is §4.3 rule 6's fallback: every key the root commits to,
// filtered by TYPE.
//
// **By type and not by key prefix**, and that is the whole reason it
// answers the same question: `app/feed/entries/…` is where *this*
// implementation puts entries and is not normative (§2 makes the type tag
// the contract), so a prefix scan would find another implementation's
// feed empty and report it as an absence.
func enumerateFeed(ctx context.Context, c *fetch.Consumer, walk fetch.WalkResult, subject string,
	opts FeedReadOpts, out *FeedRead) {

	budget := opts.EnumerationBudget
	if budget <= 0 {
		budget = DefaultFeedEnumerationBudget
	}

	// Sorted by key, which is what the walk already guarantees. There is
	// no authored order to recover here — §4.5's ordering contract lives
	// on an index page — so this is deterministic rather than meaningful,
	// and the note says so.
	bindings := append([]fetch.Binding(nil), walk.Bindings...)
	sort.Slice(bindings, func(i, j int) bool { return bindings[i].Key < bindings[j].Key })

	looked := 0
	for _, b := range bindings {
		if looked >= budget {
			out.Truncated = true
			out.Notes = append(out.Notes, fmt.Sprintf(
				"stopped after reading %d of this publisher's %d committed keys — entries beyond that "+
					"point are NOT in this view. Raise the enumeration budget to see them",
				looked, len(bindings)))
			break
		}
		if opts.Limit > 0 && len(out.Entries) >= opts.Limit {
			out.Truncated = true
			break
		}
		looked++
		ent, err := c.Blob(ctx, b.Hash)
		if err != nil {
			continue
		}
		if ent.Type != entitysdk.TypeFeedEntry {
			continue
		}
		e := FeedEntryRead{Listed: false}
		fillEntry(ctx, c, subject, ent, &e)
		out.Entries = append(out.Entries, e)
	}
	out.Notes = append(out.Notes, fmt.Sprintf(
		"read by enumeration: %d entries found among %d committed keys, in key order. **That order is "+
			"not the publisher's** — §4.5's newest-first contract lives on an index page, and there is "+
			"no index here", len(out.Entries), len(bindings)))
	// No cursor is advanced by an enumeration: a cursor is a position in
	// an index, and there is no index. Left exactly as the caller passed
	// it, so a later read with an index present resumes correctly.
}

// advanceCursor records where a next read should resume.
//
// The position is the newest entry of the newest page — which is what
// `applied` means — and the page number travels with it so §4.4's resume
// works when that entry is later removed.
func advanceCursor(out *FeedRead, current uint64) {
	for _, e := range out.Entries {
		if e.Hash.IsZero() || e.Problem != "" {
			continue
		}
		out.Cursor = entitysdk.FeedCursor{Page: current, Applied: e.Hash}
		return
	}
}

// feedConsumerFor is the one road a feed is read over, extracted so that
// **reading a feed and GATHERING one cannot take different transports**.
//
// `AP108` is the reason it is a function rather than two call sites: the
// feed reader was once wired to the static road while every gate stayed
// green, because each gate built its own consumer. A gatherer picking its
// own road would be the same defect with a durable, signed artifact at the
// end of it — [GatherTimeline] republishes what this returns.
func (m *BrowseModel) feedConsumerFor(ctx context.Context, subject string) (*fetch.Consumer, error) {
	if why, ok := m.canReachLive(subject); !ok {
		return nil, fmt.Errorf("cannot read %s's feed: %s. Following requires no permission from "+
			"them (§2.4) and it does require a route to them", subject, why)
	}
	return m.consumerForRoad(ctx, browseRoad{
		Class:     fetch.ClassLive,
		Candidate: fetch.TransportCandidate{Class: fetch.ClassLive, PeerID: subject},
	})
}

// ReadFeedOf reads a subject's feed through the browser's road chooser,
// consumer cache and `seq` floor.
//
// Same road rule as [BrowseModel.ResolveReference] and for the same
// reason: a subject is a peer-id and carries no origin, so the road is
// the live one — this peer must already hold a connection, or be the
// subject itself. It never dials a guess.
func (m *BrowseModel) ReadFeedOf(ctx context.Context, subject string, opts FeedReadOpts) (FeedRead, error) {
	if strings.TrimSpace(subject) == "" {
		return FeedRead{}, fmt.Errorf("no subject: a feed is read from a peer, and §2.4 makes the " +
			"subject a namespace")
	}
	c, err := m.feedConsumerFor(ctx, subject)
	if err != nil {
		return FeedRead{}, err
	}
	read, err := ReadFeed(ctx, c, opts)
	if err != nil && errors.Is(err, fetch.ErrNoPublishedRoot) {
		// A peer that has never published is a third state, and for a feed
		// it is the commonest one of all: somebody posted and has not run
		// `publish`. Returned as the read it is, rather than as a failure
		// of ours, so a timeline can show the row and say what is wrong.
		return FeedRead{Subject: subject, Notes: []string{
			"this peer has published no signed root, so there is nothing to verify a feed against. " +
				"Posting is not publishing: what they posted is in their tree and in no reader's view",
		}}, nil
	}
	return read, err
}
