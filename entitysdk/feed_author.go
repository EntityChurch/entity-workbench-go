// Feed authoring — the production path for APP-CONVENTION-FEED §1.1, §2.3 and
// §4, i.e. the half `feed.go` deliberately left out.
//
// # Why this exists, and it is the same item from two directions
//
// `feed.go` shipped the vocabulary and the index builder and **nothing that
// writes one into a tree**. That is D23's violation — a model with no shipped
// surface is not shipped — and the other application seat had the mirror image:
// a per-entry signing capability called by nothing but tests. Arch's `AZ-s4`
// and our own surface question are one item seen from two sides, and the same
// authoring path closes both. Until something publishes a feed, the joint
// fixture's five-of-five is **producer evidence between two encoders** and no
// reader anywhere has been pointed at bytes a peer actually serves.
//
// # A POST APPENDS; IT DOES NOT REBUILD
//
// §4.3 rule 1 makes pages key-addressed rather than hash-chained so that
// *"rewriting page 12 changes page 12's binding and nothing else"*, and rule 3
// forbids renumbering, merging and compaction. Those two only pay if a new
// entry lands on the **last** page — which is why pages fill oldest-first while
// entries read newest-first *within* a page (`BuildFeedIndex`'s header). So
// [FeedAuthor.Post] touches exactly two index keys: the current page, and the
// head. **A full page is sealed and is never read, re-encoded or rewritten
// again**, which is the rule holding by construction rather than by check.
//
// [BuildFeedIndex] stays the reference: `TestFeedAuthor_AppendEqualsFullRebuild`
// asserts the appended index is **byte-identical** to a full build over the same
// entries, across a page boundary, so the shipped path cannot drift from the one
// the cross-implementation fixture measures.
//
// # WRITE ORDER IS LOAD-BEARING: entry, signature, page, head
//
// The other seat's phase-2b write-up puts it best — *the index and the entries
// are one publish, or the index is a liar*. A head naming a page whose entries
// are not bound yet is a feed that reports more than it can serve, and a reader
// cannot tell that from a withholding origin. So the head is written **last**
// and every failure before it leaves a feed that is smaller than it could be
// rather than one that overstates itself. An entry bound with no index row is
// invisible and harmless; an index row with no entry is a broken promise.
//
// # TWO THINGS THIS DOES NOT DO, both named rather than hidden
//
//   - **No `prev`.** §2.3.1's append-only commitment is opt-in, and opting in
//     makes removing an entry break every successor's chain. A publisher who
//     wants tamper-evidence more than editability sets it; the default should
//     not decide that for them.
//   - **No reader.** Still theirs. [FeedAuthor.Read] reads **our own** feed back
//     out of our own tree to render it, and is not an implementation of §4.3's
//     walk, §2.2.2's four resolution outcomes or `FEED-R4`'s attribution. Those
//     four reader-side `[MUST]`s remain ungated on this seat and this file does
//     not change that.
//
// # ⚠ THE ATTRIBUTION LIVES OUTSIDE THE PREFIX, AND THAT IS NOT A LOCAL CHOICE
//
// `FEED-R2` requires a detached `system/signature` per entry, and V7 §3.5 fixes
// where it goes: `system/signature/{hex(entry_hash)}`. A feed publish commits to
// `app/feed/`. **The signature is therefore outside the signed root in every
// topology**, and the two roads answer differently — a live reader is covered by
// `workbench.PublicSiteGrants`'s `system/signature/*` entry, a static reader is
// not covered by anything, because the static corridor emits the closure of the
// trie root and those keys are not in it. [FeedAuthor.SignatureCoverage]
// computes the fact so a surface can say it; it is not this file's to fix.
package entitysdk

import (
	"fmt"
	"strings"
	"time"

	"go.entitychurch.org/entity-core-go/core/hash"
)

// DefaultFeedPageSize is how many entries this implementation puts on a page.
//
// §4.3 rule 5 forbids an unbounded page and **deliberately does not pick a
// number** — what is normative is the shape. 32 is the other implementation's
// `DEFAULT_PAGE_SIZE`, matched for the same reason the four encodings in
// `feed.go` are matched: the value is free, a divergence costs a joint run an
// explanation, and whoever published first is the baseline.
const DefaultFeedPageSize = 32

// The embed media types this cohort's feed bodies carry today.
//
// **They live here, beside the author, because a media type is a producer
// and a consumer agreeing** — it is the `app/embed/{media_type}` dispatch key
// (EMBED §3), so the writer's spelling and the reader's switch are one
// contract. A reader holding its own copy is how one seat renders a body and
// the other renders its alt text with neither able to see it
// (`ROUTING-2026-09-17-a` §4).
const (
	// FeedMediaPlain is prose the author did not mean as markup. A reader
	// MUST NOT run a markdown parser over it.
	FeedMediaPlain = "text/plain"

	// FeedMediaMarkdown is a body drawn by the same code that draws a site
	// page — `SITE` §3.1/§3.2 and `FEED` §2.3 already share EMBED as the
	// vocabulary, so this needs no spec change and never did.
	FeedMediaMarkdown = "text/markdown"
)

// DefaultPostMediaType is the embed media type a bare text post carries.
//
// It stays `text/plain` deliberately. Defaulting a bare `post` to markdown
// would silently reinterpret every apostrophe-and-asterisk post an operator
// has already written, and the one thing a default may not do is change what
// existing bytes mean.
const DefaultPostMediaType = FeedMediaPlain

// FeedAuthor writes this peer's own feed.
//
// **It can only ever write this peer's feed**, and that is structural rather
// than a check: §1.1 makes the author equal to the namespace, [PostRequest] has
// no author field, and every path is built from `ap.PeerID()`. An author who
// could name someone else would be an author who could forge.
type FeedAuthor struct {
	ap *AppPeer

	// PageSize is §4.2's page capacity. Changing it on a feed that already
	// has pages does NOT renumber anything — rule 3 forbids that — so the
	// existing pages keep whatever capacity they were filled at and only
	// new pages use the new value. A reader is unaffected: nothing in §4
	// says pages are the same size, and a reader that assumed so would
	// break on the last page anyway.
	PageSize int
}

// Feed returns the authoring surface for this peer's own feed.
func (a *AppPeer) Feed() *FeedAuthor {
	return &FeedAuthor{ap: a, PageSize: DefaultFeedPageSize}
}

// PostRequest is one authored entry.
//
// There is no `Author` field on purpose — see [FeedAuthor].
type PostRequest struct {
	// Text is the body, carried INLINE as an `app/embed/{media_type}`
	// payload (EMBED §3.1's input surface). A photo post and a text post
	// are one shape with a different embed inside, which is why this is an
	// embed rather than a string field.
	Text string

	// MediaType defaults to [DefaultPostMediaType].
	MediaType string

	// Fallback is EMBED §3's mandatory degradation rung. Empty means Text,
	// which is right for text and wrong for anything else — an image post
	// whose fallback is its own bytes has no fallback.
	Fallback string

	// Reply, when set, makes this entry a reply. Both terms MUST be pinned
	// references (§2.2.1) and [FeedEntryData.Validate] refuses otherwise.
	Reply *FeedReply

	// Context is what this is PART OF — never who it is for.
	Context *EntityRef

	// Attachments are REFERENCED, never inlined (§1.2).
	Attachments []EntityRef

	// At pins `created_at`. Zero means now.
	//
	// It exists for reproducible emissions, the same reason
	// `publish.Opts.At` does: an entry's timestamp is inside the hashed
	// bytes, so a fresh clock moves the entry hash, which moves its
	// signature's key, its index row and the root.
	At time.Time
}

// PostedEntry is what one post did, in terms a surface can render.
type PostedEntry struct {
	// Hash is the entry's content hash, which is also its identity: §2.2.1
	// makes every reference to an entry a pin.
	Hash hash.Hash

	// CreatedAt is the stamp that went into the bytes, ms since epoch.
	CreatedAt uint64

	// EntryPath, SignaturePath, PagePath and HeadPath are the four tree
	// keys this post wrote, in the order it wrote them.
	EntryPath     string
	SignaturePath string
	PagePath      string
	HeadPath      string

	// Page is the index page this entry landed on, and NewPage reports
	// whether the post started it. A surface that shows the operator "post
	// 33 opened page 1" is showing them the one cost model §4.3 has.
	Page    uint64
	NewPage bool

	// PageEntries is how many entries that page carries now.
	PageEntries int

	// Entries is the feed's total after this post.
	Entries int
}

// Post authors one entry: the entry, its `FEED-R2` detached signature, the
// index page it lands on and the head — in that order, and see the file header
// for why the order is not an implementation detail.
func (f *FeedAuthor) Post(req PostRequest) (PostedEntry, error) {
	if f == nil || f.ap == nil {
		return PostedEntry{}, NewError(400, "invalid_request", "no peer to author a feed on")
	}
	peerID := f.ap.PeerID()
	pageSize := f.PageSize
	if pageSize <= 0 {
		pageSize = DefaultFeedPageSize
	}

	node, err := postBody(req)
	if err != nil {
		return PostedEntry{}, err
	}
	createdAt := uint64(req.At.UnixMilli())
	if req.At.IsZero() {
		createdAt = uint64(time.Now().UnixMilli())
	}

	data := FeedEntryData{
		Author:      peerID,
		CreatedAt:   createdAt,
		Body:        node,
		Reply:       req.Reply,
		Context:     req.Context,
		Attachments: req.Attachments,
	}
	// §1.1's [MUST] applied at the EMITTING side, which is where it is
	// cheapest and where it is a tautology worth asserting anyway: the
	// author field is built from the namespace two lines above, so this
	// fires only if that ever stops being true. Until this call existed
	// `ValidateInNamespace` had no caller outside its own test — the rule
	// was implemented and unreached, which is the shape of everything else
	// in this file.
	if err := data.ValidateInNamespace(peerID); err != nil {
		return PostedEntry{}, err
	}
	ent, err := data.ToEntity()
	if err != nil {
		return PostedEntry{}, err
	}

	out := PostedEntry{
		Hash:          ent.ContentHash,
		CreatedAt:     createdAt,
		EntryPath:     FeedEntryTreePath(peerID, ent.ContentHash),
		SignaturePath: FeedSignatureTreePath(peerID, ent.ContentHash),
		HeadPath:      FeedIndexTreePath(peerID),
	}

	// 1. The entry.
	if _, err := f.ap.PutEntity(out.EntryPath, ent); err != nil {
		return PostedEntry{}, WrapError(500, "post_failed", "bind feed entry", err)
	}

	// 2. Its detached signature. An entry without one is unattributable the
	//    moment it leaves this tree (§1.1.1: the obligation is the
	//    COMPOSER's, at authoring time, and a mirror can only carry one that
	//    already exists), so a post that cannot sign is a failed post rather
	//    than an unsigned one.
	kp := f.ap.RawPeer().Keypair()
	sigEnt, sigKey, err := MintEntrySignature(&kp, f.ap.RawPeer().Identity(), ent.ContentHash)
	if err != nil {
		return PostedEntry{}, err
	}
	if sigPath := "/" + peerID + "/" + sigKey; sigPath != out.SignaturePath {
		// Two spellings of V7 §3.5's invariant pointer, one from the kernel
		// through MintEntrySignature and one built here for the surface to
		// print. They cannot disagree, so say so where it would be found
		// rather than leaving two sources of one path.
		return PostedEntry{}, NewError(500, "post_failed",
			fmt.Sprintf("signature pointer disagrees: %s vs %s", sigPath, out.SignaturePath))
	}
	if _, err := f.ap.PutEntity(out.SignaturePath, sigEnt); err != nil {
		return PostedEntry{}, WrapError(500, "post_failed", "bind entry signature", err)
	}

	// 3. The page, and 4. the head.
	ix, err := f.appendToIndex(peerID, ent.ContentHash, createdAt, pageSize)
	if err != nil {
		return PostedEntry{}, err
	}
	out.Page = ix.page
	out.NewPage = ix.newPage
	out.PageEntries = ix.pageEntries
	out.Entries = ix.total
	out.PagePath = FeedIndexPageTreePath(peerID, ix.page)
	return out, nil
}

// postBody builds §2.3's `body` — an inline embed node, never a bare string.
func postBody(req PostRequest) (EmbedNode, error) {
	text := req.Text
	if strings.TrimSpace(text) == "" {
		return EmbedNode{}, NewError(400, "invalid_post",
			"a post with no body; §2.3 makes `body` required and EMBED §3 makes its `fallback` non-empty")
	}
	if len(text) > EmbedInlineMaxBytes {
		// EMBED §3's `.size (1..16384)` on the inline arm. Refused rather
		// than silently promoted to a pointer payload: a pointer needs the
		// bytes ingested into the content store under a hash, which is a
		// different act with a different failure mode, and an operator whose
		// long post quietly became a content blob has lost the ability to
		// reason about what their feed contains.
		return EmbedNode{}, NewError(400, "invalid_post",
			fmt.Sprintf("post body is %d bytes and APP-CONVENTION-EMBED §3 bounds an INLINE payload at %d; "+
				"a larger body belongs behind a pointer payload, which is a different act (ingest the bytes, "+
				"embed the hash) and is not done implicitly", len(text), EmbedInlineMaxBytes))
	}
	mediaType := req.MediaType
	if mediaType == "" {
		mediaType = DefaultPostMediaType
	}
	fallback := req.Fallback
	if fallback == "" {
		fallback = text
	}
	node := NewEmbedNode(mediaType, InlinePayload([]byte(text)), fallback)
	if err := node.Validate(); err != nil {
		return EmbedNode{}, err
	}
	return node, nil
}

// indexAppend is what appending did, for [PostedEntry].
type indexAppend struct {
	page        uint64
	newPage     bool
	pageEntries int
	total       int
}

// appendToIndex puts one entry on the current page — or opens the next one —
// and re-stamps the head.
//
// **`updated_at` is a WITNESS OF THE ENTRIES, never the instant of the write**,
// on the page and on the head alike. The other seat shipped the publish-instant
// version first and their own gate could not see it, because it published twice
// at one instant: stamping now means every page's bytes move whenever anything
// is posted, which re-projects the whole archive and invalidates every cached
// page — §4.3 rule 1's cost argument defeated by the ordinary act of posting.
// A witness reproduces itself forever.
func (f *FeedAuthor) appendToIndex(peerID string, entryHash hash.Hash, createdAt uint64, pageSize int) (indexAppend, error) {
	headPath := FeedIndexTreePath(peerID)
	headEnt, headFound, err := f.ap.Get(headPath)
	if err != nil {
		return indexAppend{}, WrapError(500, "post_failed", "read feed index head", err)
	}

	var (
		page     FeedIndexPageData
		pageNo   uint64
		newPage  bool
		priorAll int
	)
	if headFound {
		head, err := FeedIndexHeadFromEntity(headEnt)
		if err != nil {
			return indexAppend{}, err
		}
		pageNo = head.Current
		pageEnt, pageFound, err := f.ap.Get(FeedIndexPageTreePath(peerID, pageNo))
		if err != nil {
			return indexAppend{}, WrapError(500, "post_failed", "read current index page", err)
		}
		if !pageFound {
			// The head names a page that is not bound. Refused rather than
			// healed: silently starting a fresh page would renumber nothing
			// and lose nothing, but it would also erase the only evidence
			// that something removed a page out from under the index, and
			// the next post would do it again.
			return indexAppend{}, NewError(409, "feed_index_broken",
				fmt.Sprintf("the feed index head names page %d and nothing is bound at %s — the index is "+
					"describing a page this peer does not hold", pageNo, FeedIndexPageTreePath(peerID, pageNo)))
		}
		page, err = FeedIndexPageFromEntity(pageEnt)
		if err != nil {
			return indexAppend{}, err
		}
		if page.Page != pageNo {
			// §4.2: a page's own number MUST equal its key. The two saying
			// different things is exactly how a moved page is detectable,
			// which only pays if somebody checks.
			return indexAppend{}, NewError(409, "feed_index_broken",
				fmt.Sprintf("page bound at key %d says it is page %d; §4.2 makes them equal so that a page "+
					"which has been moved is detectably moved", pageNo, page.Page))
		}
		priorAll = int(pageNo)*pageSize + len(page.Entries)
		if len(page.Entries) >= pageSize {
			pageNo++
			newPage = true
			page = FeedIndexPageData{Page: pageNo}
		}
	} else {
		// A feed with no head is a feed with no entries. Page 0 starts here.
		newPage = true
		page = FeedIndexPageData{Page: 0}
	}

	// Newest FIRST within the page (§4.5), so the new entry goes at the
	// front. This is a prepend and never a sort — a sort would consult
	// `created_at`, which §2.3.2 forbids relying on for correctness.
	page.Entries = append([]EntityRef{PinnedRef(peerID, entryHash)}, page.Entries...)
	if createdAt > page.UpdatedAt {
		page.UpdatedAt = createdAt
	}
	pageEnt, err := page.ToEntity()
	if err != nil {
		return indexAppend{}, err
	}
	if _, err := f.ap.PutEntity(FeedIndexPageTreePath(peerID, pageNo), pageEnt); err != nil {
		return indexAppend{}, WrapError(500, "post_failed", "bind index page", err)
	}

	headData := FeedIndexHeadData{Current: pageNo, UpdatedAt: page.UpdatedAt}
	newHead, err := headData.ToEntity()
	if err != nil {
		return indexAppend{}, err
	}
	if _, err := f.ap.PutEntity(headPath, newHead); err != nil {
		return indexAppend{}, WrapError(500, "post_failed", "bind index head", err)
	}

	return indexAppend{
		page:        pageNo,
		newPage:     newPage,
		pageEntries: len(page.Entries),
		total:       priorAll + 1,
	}, nil
}

// ReplyTo builds §2.3's reply term for an entry this peer holds.
//
// **`root` is inherited, never assumed to be the parent.** With `parent` alone
// a conversation is a hop-by-hop walk that one unreachable author truncates;
// `root` lets any holder of any entry name the whole thing in one step. So
// replying to a reply carries that reply's own root, and replying to a
// top-level entry makes that entry the root — there is no genesis entity, the
// first entry IS the conversation.
//
// ⚠ **It refuses a parent this peer does not hold, and that is a real limit
// rather than a check.** The conversation's root is a field INSIDE the parent,
// so a reply to an entry we have never read would have to guess it — and a
// wrong `root` puts the reply in a conversation nobody else can see. Reaching a
// foreign entry needs its key on the author's peer, which §2 leaves to the
// author, so a cross-peer reply needs the reader half (§4.3's walk) that this
// seat has not built.
func (f *FeedAuthor) ReplyTo(parent hash.Hash) (*FeedReply, error) {
	if f == nil || f.ap == nil {
		return nil, NewError(400, "invalid_request", "no peer")
	}
	if parent.IsZero() {
		return nil, NewError(400, "invalid_request", "no parent entry to reply to")
	}
	peerID := f.ap.PeerID()
	path := FeedEntryTreePath(peerID, parent)
	ent, found, err := f.ap.Get(path)
	if err != nil {
		return nil, WrapError(500, "reply_failed", "read the entry being replied to", err)
	}
	if !found {
		return nil, NewError(404, "parent_not_held",
			fmt.Sprintf("this peer holds no feed entry %s — a reply pins the CONVERSATION's root as well as "+
				"its parent (§2.3), and that root is a field inside the parent, so replying to an entry we "+
				"have never read would mean guessing which conversation it belongs to", parent))
	}
	data, err := FeedEntryFromEntity(ent)
	if err != nil {
		return nil, err
	}
	root := parent
	if data.Reply != nil && data.Reply.Root.Hash != nil {
		root = *data.Reply.Root.Hash
	}
	return &FeedReply{
		Root:   PinnedRef(data.Author, root),
		Parent: PinnedRef(data.Author, parent),
	}, nil
}

// FeedReadout is this peer's own feed as a surface renders it: newest first,
// with the index's own arithmetic beside it.
type FeedReadout struct {
	PeerID string

	// Published reports whether an index head exists at all. FALSE and an
	// empty Entries list are different facts — *this peer has never posted*
	// versus *this peer has a feed and it is empty* — and only the first is
	// a reason to say "nothing here yet".
	Published bool

	// Pages is the head's `current` + 1, and Entries counts what was read.
	Pages   uint64
	Entries []FeedReadEntry

	// Truncated reports that a limit stopped the read before the oldest
	// page. A surface that renders a limited list as the whole feed is
	// telling the operator their older posts are gone.
	Truncated bool
}

// FeedReadEntry is one entry of our own, decoded.
type FeedReadEntry struct {
	Hash      hash.Hash
	CreatedAt uint64
	Page      uint64

	// Text is the inline embed's payload when it has one, else the
	// authored fallback — EMBED §6's ladder, one rung.
	Text      string
	MediaType string

	// Signed reports whether the `FEED-R2` detached signature is bound.
	// **Unsigned is rendered, never hidden**: an entry authored before this
	// path existed is a real entry, and dropping it from the list would
	// make the feed disagree with its own index.
	Signed bool

	// IsReply is §2.3's reply term being present.
	IsReply bool
}

// Read reads this peer's own feed back, newest first, walking pages downward
// from the head's `current`.
//
// **This is not §4.3's reader.** It reads our own tree with our own authority
// and resolves nothing: no live references (§2.2.2), no attribution
// (`FEED-R4`), no enumeration fallback (§4.3 rule 6), no resume (§4.4). It
// exists so the operator who posts can see what they posted, and calling it a
// reader would be claiming four `[MUST]`s this seat has not built.
func (f *FeedAuthor) Read(limit int) (FeedReadout, error) {
	if f == nil || f.ap == nil {
		return FeedReadout{}, NewError(400, "invalid_request", "no peer to read a feed on")
	}
	peerID := f.ap.PeerID()
	out := FeedReadout{PeerID: peerID}

	headEnt, found, err := f.ap.Get(FeedIndexTreePath(peerID))
	if err != nil {
		return FeedReadout{}, WrapError(500, "read_failed", "read feed index head", err)
	}
	if !found {
		return out, nil
	}
	head, err := FeedIndexHeadFromEntity(headEnt)
	if err != nil {
		return FeedReadout{}, err
	}
	out.Published = true
	out.Pages = head.Current + 1

	oldest := uint64(0)
	if head.Oldest != nil {
		oldest = *head.Oldest
	}
	for p := int64(head.Current); p >= int64(oldest); p-- {
		pageNo := uint64(p)
		pageEnt, pageFound, err := f.ap.Get(FeedIndexPageTreePath(peerID, pageNo))
		if err != nil {
			return FeedReadout{}, WrapError(500, "read_failed", "read index page", err)
		}
		if !pageFound {
			// Reported by absence rather than refused: a READ of a feed with
			// a gap in it should still show what is there. The write path
			// refuses the same condition, which is where it can still be
			// acted on.
			continue
		}
		page, err := FeedIndexPageFromEntity(pageEnt)
		if err != nil {
			return FeedReadout{}, err
		}
		for _, ref := range page.Entries {
			if limit > 0 && len(out.Entries) >= limit {
				out.Truncated = true
				return out, nil
			}
			re, err := f.readEntry(peerID, ref, pageNo)
			if err != nil {
				return FeedReadout{}, err
			}
			out.Entries = append(out.Entries, re)
		}
	}
	return out, nil
}

func (f *FeedAuthor) readEntry(peerID string, ref EntityRef, page uint64) (FeedReadEntry, error) {
	if !ref.IsPinned() || ref.Hash == nil {
		return FeedReadEntry{}, NewError(400, "invalid_feed_index",
			fmt.Sprintf("index page %d carries a %q reference; §4.2 pins every entry row", page, ref.Tag))
	}
	entryHash := *ref.Hash
	re := FeedReadEntry{Hash: entryHash, Page: page}
	ent, found, err := f.ap.Get(FeedEntryTreePath(peerID, entryHash))
	if err != nil {
		return FeedReadEntry{}, WrapError(500, "read_failed", "read feed entry", err)
	}
	if !found {
		// The index names an entry this peer does not hold. Kept in the
		// list, empty: a surface that drops it silently makes the feed's
		// own count wrong, and the missing row is the interesting one.
		return re, nil
	}
	data, err := FeedEntryFromEntity(ent)
	if err != nil {
		return FeedReadEntry{}, err
	}
	re.CreatedAt = data.CreatedAt
	re.MediaType = data.Body.MediaType()
	re.IsReply = data.Reply != nil
	if data.Body.Data.Payload.Tag == EmbedPayloadInline {
		re.Text = string(data.Body.Data.Payload.Bytes)
	} else {
		re.Text = data.Body.Data.Fallback
	}
	if ok, err := f.ap.Has(FeedSignatureTreePath(peerID, entryHash)); err == nil {
		re.Signed = ok
	}
	return re, nil
}

// SignatureCoverage answers whether a signed root over `prefix` commits to the
// detached signatures this feed's entries carry.
//
// **The answer is always no for any feed-shaped prefix, and the point is to say
// so rather than to compute it.** V7 §3.5 fixes a signature at
// `system/signature/{hex}`; a feed publish names `app/feed/`. There is no
// prefix that contains both except the whole tree, which `PublicSiteGrants`
// refuses for a public read and rightly. So:
//
//   - a LIVE reader is fine — the public grant carries `system/signature/*`
//     alongside the prefix, for the same reason it carries the published root;
//   - a STATIC reader is not — the corridor emits the closure of the trie root,
//     and these keys are not in the trie at all, so every entry arrives
//     unattributable and the reader cannot tell that from an author who never
//     signed.
//
// Returned as a fact for a surface to print, because the operator who publishes
// is the only party who can act on it, and nothing else in the chain will
// mention it.
func (f *FeedAuthor) SignatureCoverage(prefix string) (covered bool, note string) {
	p := strings.Trim(strings.TrimSpace(prefix), "/")
	if p == "" {
		// The whole tree. Legal to sign, and the one prefix that does cover
		// the signatures — which is why the note says what it costs.
		return true, "this root commits to the whole tree, so the entries' signatures are inside it — " +
			"and so is everything else this peer holds"
	}
	if strings.HasPrefix("system/signature", p) {
		return true, ""
	}
	return false, "a reader that fetches this feed from a STATIC directory cannot attribute any entry: " +
		"FEED-R2's detached signatures live at `system/signature/{hex}` (V7 §3.5) and a root over " +
		"\"" + prefix + "\" does not commit to them. A reader that dials this peer is unaffected — the " +
		"public grant covers them."
}
