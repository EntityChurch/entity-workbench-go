// Feed — APP-CONVENTION-FEED v0.3 §2.3, §2.4 and §4.2.
//
// Four types: `app/feed/entry`, `app/feed/index-head`, `app/feed/index-page`
// and `app/feed/follow`. Not `collection` (§5) and not `mirror` (§6) — those
// are stage 2, and §6 in particular needs a byte-fidelity republication path
// rather than a codec.
//
// BUILD ORDER. reference → embed → feed, and it is forced: §2.3 types an
// entry's `body` as an `embed-node`, and EMBED's `child` arm carries an
// `entity-ref`. The peer seat measured the sizing trap for us and said so —
// *the site convention imports no atoms and FEED imports two* — which is why
// pricing this by analogy with the site work was wrong.
//
// FOUR ENCODINGS ARE MATCHED, NOT RE-DERIVED. `entity-browser-rust` shipped
// FEED first, so where the specification is silent their choice is the
// baseline: whatever publishes first becomes the corpus. Cited here so the
// coupling is greppable if arch moves any of them.
//
//  1. **`signer` is the identity ENTITY'S CONTENT HASH, not the peer-id
//     string.** §1.1's shorthand is *"signer = author"* and `system/signature`
//     is the KERNEL's type, whose own field is a hash. An implementer reading
//     FEED alone puts a peer id in a slot that wants a hash and produces
//     something no verifier can use. MintEntrySignature is the one place.
//  2. **Pages fill oldest-first; entries within a page read newest-first**
//     (§4.5). The two run against each other by design and **getting BOTH
//     backwards round-trips perfectly**, so a harness that only re-reads its
//     own index cannot see it. See BuildFeedIndex.
//  3. **A page's `updated_at` is the MAXIMUM `created_at` of the entries it
//     carries** — a witness, not the publish instant, so a page nobody touched
//     reproduces its own stamp forever. §4.2 gives the field no semantics;
//     this is THEIR reading, routed to arch as `A-41`, and we match it
//     deliberately rather than silently — one of us becomes the baseline
//     either way.
//  4. **The head omits `oldest` when nothing has been dropped** — §4.2
//     declares 0 as the default, so a publisher who has dropped nothing emits
//     no key. Absent and zero are the same fact and only one is fewer bytes.
//
// WHAT IS NOT HERE, and each absence is a decision:
//
//   - **No cursor field on a follow record.** §2.4 makes its absence normative
//     in v0.3: a reader's position is `{page, applied}` LOCAL state (§4.4), and
//     v0.2's `? cursor: content-hash` was simultaneously the declared field and
//     the wrong shape. The peer seat's ask `A-36` and their own
//     `[OPEN-FEED-1]` vote point in opposite directions and **both cannot be
//     built**; they have put it on hold and their `Follow.cursor` is
//     constructed `None` everywhere. We emit no such field, which costs
//     nothing and commits to neither answer.
//
//     ⚠ **The seat is named because IDS ARE PER-SEAT AND COLLIDE ACROSS
//     THEM** — arch, 2026-09-13, on two seats using `A-35` for different
//     items. This line read "arch's `A-36`", which names the seat an ask was
//     routed TO rather than the one that numbered it, and our own tracker now
//     carries a different `A-36`.
//
//   - **No reader/collection/mirror surface.** Deliberately last. They have the
//     reader; duplicating it produces cohort-consistent evidence at best.
package entitysdk

import (
	hexenc "encoding/hex"
	"fmt"
	"sort"
	"strconv"

	"go.entitychurch.org/entity-core-go/core/crypto"
	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"
)

// The `app/feed/*` type vocabulary (§2). As with `app/share/*`, **the type tag
// is the cross-impl contract and the path is not** (§2) — cross-peer
// aggregation is a `type_filter` query with no peer filter, so the tag is the
// index key.
const (
	TypeFeedEntry     = "app/feed/entry"
	TypeFeedIndexHead = "app/feed/index-head"
	TypeFeedIndexPage = "app/feed/index-page"
	TypeFeedFollow    = "app/feed/follow"
)

// FeedIndexPath is §4.2's pinned head path, peer-relative:
// `/{peer}/app/feed/index`.
//
// **This and FeedIndexPagePath are the ONLY paths this convention pins.**
// A reader holding no reference has to start somewhere and a reader resuming
// has to jump straight to where it left off, so these two are addresses rather
// than a local choice. Everything else — including where an entry lives — is
// ours, which is why the joint fixture has two roots and only the index one is
// a comparand.
const FeedIndexPath = "app/feed/index"

// FeedEntryPrefix is where THIS implementation puts entries. It is **not**
// normative and no other seat has to match it: §2 makes the type tag the
// contract. Named as a constant anyway so the choice has one home.
const FeedEntryPrefix = "app/feed/entries/"

// FeedIndexPagePath is §4.2's page path. The page number is a decimal uint and
// **MUST equal the `page` field inside the entity** — the key and the body say
// the same thing so a page that is moved is detectably moved.
func FeedIndexPagePath(page uint64) string {
	return FeedIndexPath + "/" + strconv.FormatUint(page, 10)
}

// FeedEntryKey is this implementation's key for one entry.
func FeedEntryKey(entryHash hash.Hash) string {
	return FeedEntryPrefix + hexenc.EncodeToString(entryHash.Bytes())
}

// The four functions below are the same keys PEER-QUALIFIED — `/{peer}/…` —
// which is what a tree write takes and what a dispatched read of somebody
// else's feed takes.
//
// **The two forms are kept apart deliberately.** The bare key is the BINDING,
// and the binding is the cross-impl comparand: a joint fixture compares
// `app/feed/index/2`, never `/{some-peer}/app/feed/index/2`, because the peer
// segment is whoever happens to hold the bytes. Collapsing them would put a
// peer id into the one string two implementations have to agree on.
func FeedIndexTreePath(peerID string) string {
	return "/" + peerID + "/" + FeedIndexPath
}

// FeedIndexPageTreePath is §4.2's page, peer-qualified.
func FeedIndexPageTreePath(peerID string, page uint64) string {
	return "/" + peerID + "/" + FeedIndexPagePath(page)
}

// FeedEntryTreePath is where this implementation binds one entry.
func FeedEntryTreePath(peerID string, entryHash hash.Hash) string {
	return "/" + peerID + "/" + FeedEntryKey(entryHash)
}

// FeedSignatureTreePath is V7 §3.5's invariant pointer for an entry's detached
// signature, peer-qualified.
//
// ⚠ **It is NOT under `app/feed/`**, and that is a fact about the whole
// convention rather than about this function: `system/signature/{hex}` is the
// kernel's location for a signature over any target, so the `FEED-R2`
// attribution for an entry lives outside every prefix a feed publish would
// name. See [FeedAuthor]'s header — a publisher has to carry that, and until
// something does, a statically published feed reaches a reader with no way to
// attribute a single entry.
func FeedSignatureTreePath(peerID string, entryHash hash.Hash) string {
	return "/" + peerID + "/" + types.LocalSignaturePath(entryHash)
}

// FeedEntryFromEntity decodes an entry, refusing anything that is not one.
//
// The type check is not a formality: §2 makes the TYPE TAG the cross-impl
// contract, so an entity at an entry's key whose tag says otherwise is not an
// entry that moved, it is a different thing at that address.
func FeedEntryFromEntity(e entity.Entity) (FeedEntryData, error) {
	if e.Type != TypeFeedEntry {
		return FeedEntryData{}, NewError(400, "invalid_feed_entry",
			fmt.Sprintf("entity type %q is not %q; §2 makes the type tag the contract", e.Type, TypeFeedEntry))
	}
	var d FeedEntryData
	if err := ecf.Decode(e.Data, &d); err != nil {
		return FeedEntryData{}, WrapError(400, "invalid_feed_entry", "decode feed entry", err)
	}
	return d, nil
}

// FeedIndexHeadFromEntity decodes §4.2's head.
func FeedIndexHeadFromEntity(e entity.Entity) (FeedIndexHeadData, error) {
	if e.Type != TypeFeedIndexHead {
		return FeedIndexHeadData{}, NewError(400, "invalid_feed_index",
			fmt.Sprintf("entity type %q is not %q", e.Type, TypeFeedIndexHead))
	}
	var d FeedIndexHeadData
	if err := ecf.Decode(e.Data, &d); err != nil {
		return FeedIndexHeadData{}, WrapError(400, "invalid_feed_index", "decode feed index head", err)
	}
	return d, nil
}

// FeedIndexPageFromEntity decodes §4.2's page.
func FeedIndexPageFromEntity(e entity.Entity) (FeedIndexPageData, error) {
	if e.Type != TypeFeedIndexPage {
		return FeedIndexPageData{}, NewError(400, "invalid_feed_index",
			fmt.Sprintf("entity type %q is not %q", e.Type, TypeFeedIndexPage))
	}
	var d FeedIndexPageData
	if err := ecf.Decode(e.Data, &d); err != nil {
		return FeedIndexPageData{}, WrapError(400, "invalid_feed_index", "decode feed index page", err)
	}
	return d, nil
}

// FeedReply is §2.3's reply term: present means this entry IS a reply.
//
// **Both terms are PINNED references and §2.2.1 does not widen** — a reply must
// not become as trustworthy as whatever currently answers a location, and a
// parent must not be editable underneath its replies. Validate refuses a live
// atom here.
//
// **`root` is load-bearing and is not redundant with `parent`.** With `parent`
// alone, assembling a conversation is a hop-by-hop walk and one unreachable
// author truncates everything below them. `root` lets any holder of any entry
// name the whole conversation in one step. There is no genesis entity: the root
// entry IS the conversation and its hash is the conversation's identity.
type FeedReply struct {
	Root   EntityRef `cbor:"root"`
	Parent EntityRef `cbor:"parent"`
}

// FeedEntryData is §2.3's entry.
//
// Every optional key is emitted only when it is carried: an absent `reply` and
// an empty one are different facts and **only the first is representable**.
// That is what the `omitempty` tags buy, and it is the first thing a joint
// fixture's floor row measures.
type FeedEntryData struct {
	// Author MUST equal the authoring namespace (§1.1). An entry found under
	// one peer's namespace claiming a different author is INVALID and a
	// conformant reader MUST reject it — ValidateInNamespace is that check,
	// separate because this struct does not know where it was read from.
	Author string `cbor:"author"`
	// CreatedAt is the author's own clock and is a DISPLAY HEURISTIC (§2.3.2).
	// A reader MUST NOT rely on it for correctness and MUST NOT reject an entry
	// for an implausible timestamp: the cost of getting that wrong — entries
	// silently dropped as "too old" — is worse than a display artifact.
	CreatedAt uint64 `cbor:"created_at"`
	// Body is an embed-node carried INLINE (EMBED §3.1, the INPUT surface).
	// A photo post and a text post are one shape with a different embed inside.
	Body EmbedNode `cbor:"body"`
	// Reply present => this entry is a reply. Both terms pinned.
	Reply *FeedReply `cbor:"reply,omitempty"`
	// Context is what this is PART OF — never who it is for, and never a
	// `reply`. Collapsing the two makes "replied to" unrenderable. Either atom.
	Context *EntityRef `cbor:"context,omitempty"`
	// Prev is §2.3.1's OPT-IN append-only commitment, and it is a BARE HASH
	// rather than a reference atom — wrapping it in one produces different
	// bytes. It is NOT navigation: navigation is by key (§4).
	Prev *hash.Hash `cbor:"prev,omitempty"`
	// Attachments are REFERENCED, never inlined (§1.2). Either atom.
	Attachments []EntityRef `cbor:"attachments,omitempty"`
}

// Validate enforces §2.3's shape.
func (d FeedEntryData) Validate() error {
	if d.Author == "" {
		return NewError(400, "invalid_feed_entry",
			"entry names no author; §1.1 makes the author the namespace and it is REQUIRED")
	}
	if err := d.Body.Validate(); err != nil {
		return fmt.Errorf("entry body: %w", err)
	}
	if d.Reply != nil {
		// §2.2.1: `reference` only at both sites. The pin is the reason the
		// field exists.
		if err := requirePinned("reply.root", d.Reply.Root); err != nil {
			return err
		}
		if err := requirePinned("reply.parent", d.Reply.Parent); err != nil {
			return err
		}
	}
	if d.Context != nil {
		if err := d.Context.Validate(); err != nil {
			return fmt.Errorf("entry context: %w", err)
		}
	}
	if d.Prev != nil && d.Prev.IsZero() {
		return NewError(400, "invalid_feed_entry",
			"entry carries an empty `prev`; §2.3.1 makes it an append-only commitment to a SPECIFIC "+
				"predecessor, so a zero hash commits to nothing while claiming to commit")
	}
	for i, a := range d.Attachments {
		if err := a.Validate(); err != nil {
			return fmt.Errorf("entry attachment %d: %w", i, err)
		}
	}
	return nil
}

// ValidateInNamespace is §1.1's [MUST], and it is separate from Validate on
// purpose: the rule is about where the entry was READ, which the bytes cannot
// know. A reader that skips this accepts entries authored in somebody else's
// name.
func (d FeedEntryData) ValidateInNamespace(peerID string) error {
	if err := d.Validate(); err != nil {
		return err
	}
	if d.Author != peerID {
		return NewError(400, "invalid_feed_entry",
			fmt.Sprintf("entry under namespace %s claims author %s; §1.1 makes them equal and a conformant "+
				"reader MUST reject the entry", peerID, d.Author))
	}
	return nil
}

func requirePinned(where string, r EntityRef) error {
	if err := r.Validate(); err != nil {
		return fmt.Errorf("%s: %w", where, err)
	}
	if !r.IsPinned() {
		return NewError(400, "invalid_feed_entry",
			fmt.Sprintf("%s is a %q reference; APP-CONVENTION-FEED §2.2.1 admits a PIN only here — a reply "+
				"must not become as trustworthy as whatever currently answers a location, and a parent must "+
				"not be editable underneath its replies", where, r.Tag))
	}
	return nil
}

// ToEntity encodes the entry.
func (d FeedEntryData) ToEntity() (entity.Entity, error) {
	if err := d.Validate(); err != nil {
		return entity.Entity{}, err
	}
	return encodeAsEntity(TypeFeedEntry, d)
}

// FeedIndexHeadData is §4.2's head, at the pinned key `app/feed/index`.
type FeedIndexHeadData struct {
	// Current is the highest page number in use.
	Current uint64 `cbor:"current"`
	// Oldest is the lowest page still published. **Omitted when nothing has
	// been dropped** — §4.2 declares 0 as the default, so emitting it would be
	// spending bytes to say what the default already says.
	Oldest    *uint64 `cbor:"oldest,omitempty"`
	UpdatedAt uint64  `cbor:"updated_at"`
}

// ToEntity encodes the head.
func (d FeedIndexHeadData) ToEntity() (entity.Entity, error) {
	if d.Oldest != nil && *d.Oldest > d.Current {
		return entity.Entity{}, NewError(400, "invalid_feed_index",
			fmt.Sprintf("index head says oldest=%d current=%d; the lowest page still published cannot be "+
				"above the highest in use", *d.Oldest, d.Current))
	}
	return encodeAsEntity(TypeFeedIndexHead, d)
}

// FeedIndexPageData is §4.2's page, at `app/feed/index/{page}`.
type FeedIndexPageData struct {
	// Page MUST equal this page's own key.
	Page uint64 `cbor:"page"`
	// Entries is NEWEST FIRST within the page, and the order is AUTHORED
	// rather than derived (§4.5) — whatever sequence the publisher wrote is
	// what a reader renders.
	Entries []EntityRef `cbor:"entries"`
	// UpdatedAt — see the file header, item 3.
	UpdatedAt uint64 `cbor:"updated_at"`
}

// ToEntity encodes the page.
func (d FeedIndexPageData) ToEntity() (entity.Entity, error) {
	for i, r := range d.Entries {
		if err := r.Validate(); err != nil {
			return entity.Entity{}, fmt.Errorf("index page %d entry %d: %w", d.Page, i, err)
		}
	}
	return encodeAsEntity(TypeFeedIndexPage, d)
}

// FeedFollowData is §2.4's follow — **the reader's PRIVATE data**. Nothing in
// the convention publishes it and nothing requires a publisher to learn who
// follows them; publishing a follow list is a separate, voluntary act that is
// not specified.
//
// **This is a distinct type from `app/share/follow` and the discriminator is
// the SUBJECT**: a share-follow follows a GRANT (one titled record with an
// audience the publisher authorized, so the publisher knows the follower
// exists); a feed-follow follows a NAMESPACE (public, pull-only, no grant, and
// the publisher does not know). Two authorization models cannot share a record
// without one lying about what it grants — which is how `B-4`'s *"settle on one
// record + verb"* was answered by the convention declining the premise.
type FeedFollowData struct {
	// Subject is whose feed this follows — a namespace, not a record.
	Subject string `cbor:"subject"`
	// Label is the follower's own petname: local, chosen by the reader, never
	// authoritative, and never transmitted as a claim about anyone. It is the
	// answer to "I cannot read a public key" that needs no naming authority.
	Label string `cbor:"label,omitempty"`
	// Via records the identifier as typed or scanned, for provenance display.
	Via string `cbor:"via,omitempty"`
	// Since is when this follow was created, ms since epoch.
	Since uint64 `cbor:"since"`

	// NOTE: there is deliberately no `cursor` field. See the file header.
}

// ToEntity encodes the follow record.
func (d FeedFollowData) ToEntity() (entity.Entity, error) {
	if d.Subject == "" {
		return entity.Entity{}, NewError(400, "invalid_feed_follow",
			"follow names no subject; §2.4 makes it the peer namespace being followed")
	}
	return encodeAsEntity(TypeFeedFollow, d)
}

// FeedIndex is a built index — the head plus every page, with the tree key each
// belongs at.
type FeedIndex struct {
	Head      entity.Entity
	HeadKey   string
	Pages     []entity.Entity
	PageKeys  []string
	PageCount uint64
}

// Bindings returns key → content hash for everything §4.2 pins, which is
// exactly the set a cross-impl root comparison is over.
func (ix FeedIndex) Bindings() map[string]hash.Hash {
	out := make(map[string]hash.Hash, len(ix.Pages)+1)
	out[ix.HeadKey] = ix.Head.ContentHash
	for i, p := range ix.Pages {
		out[ix.PageKeys[i]] = p.ContentHash
	}
	return out
}

// FeedIndexEntry is one entry as the index needs to see it.
type FeedIndexEntry struct {
	Hash      hash.Hash
	CreatedAt uint64
}

// BuildFeedIndex pages entries per §4.2/§4.3/§4.5.
//
// **`entries` is given OLDEST FIRST and is not sorted here.** Ordering is the
// publisher's (§4.5: the order is authored, not derived) and `created_at` is
// explicitly not an ordering authority (§2.3.2), so sorting by it inside the
// index builder would make an unverifiable clock decide the archive's layout.
//
// ⚠ **THE TWO DIRECTIONS RUN AGAINST EACH OTHER AND THAT IS THE POINT.** Pages
// FILL oldest-first — page 0 holds the oldest `pageSize` entries — and entries
// WITHIN a page are listed newest-first. Fill newest-first instead and every
// publish shifts every entry one slot, so the whole archive is rewritten and
// every reader's cursor dies: §4.3 rule 1's cost property defeated by the
// ordinary act of posting.
//
// **A harness that only re-reads its own index passes with BOTH reversed**,
// which is why the joint fixture is three pages with a partial last one rather
// than a single page, and why the gate asserts the arithmetic rather than the
// round trip.
//
// Pages are never renumbered, merged or compacted (§4.3 rule 3): a page that
// loses an entry is a page with fewer entries. Nothing here can renumber, and
// that is by construction rather than by check.
func BuildFeedIndex(author string, entries []FeedIndexEntry, pageSize int, headUpdatedAt uint64) (FeedIndex, error) {
	if pageSize <= 0 {
		// §4.3 rule 5 forbids an unbounded page. A zero page size is the
		// unbounded case spelled a different way.
		return FeedIndex{}, NewError(400, "invalid_feed_index",
			"page size must be positive; §4.3 rule 5 forbids an unbounded page (the right VALUE is "+
				"deliberately unspecified — what is normative is the shape, not the arithmetic)")
	}
	if author == "" {
		return FeedIndex{}, NewError(400, "invalid_feed_index", "index names no author")
	}

	ix := FeedIndex{HeadKey: FeedIndexPath}
	for start := 0; start < len(entries); start += pageSize {
		end := start + pageSize
		if end > len(entries) {
			end = len(entries)
		}
		chunk := entries[start:end]
		pageNo := uint64(start / pageSize)

		// Newest first WITHIN the page. The slice arrives oldest-first, so
		// this is a reversal and not a sort — a sort would consult created_at,
		// which §2.3.2 forbids relying on.
		refs := make([]EntityRef, 0, len(chunk))
		var updatedAt uint64
		for i := len(chunk) - 1; i >= 0; i-- {
			refs = append(refs, PinnedRef(author, chunk[i].Hash))
		}
		for _, e := range chunk {
			if e.CreatedAt > updatedAt {
				updatedAt = e.CreatedAt
			}
		}

		pageEnt, err := FeedIndexPageData{Page: pageNo, Entries: refs, UpdatedAt: updatedAt}.ToEntity()
		if err != nil {
			return FeedIndex{}, err
		}
		ix.Pages = append(ix.Pages, pageEnt)
		ix.PageKeys = append(ix.PageKeys, FeedIndexPagePath(pageNo))
	}

	if len(ix.Pages) == 0 {
		// A feed with no entries still has a head: a reader holding no
		// reference has to start somewhere, and "page 0, empty" is a different
		// and more useful answer than a 404 on the one pinned key.
		empty, err := FeedIndexPageData{Page: 0, Entries: []EntityRef{}}.ToEntity()
		if err != nil {
			return FeedIndex{}, err
		}
		ix.Pages = append(ix.Pages, empty)
		ix.PageKeys = append(ix.PageKeys, FeedIndexPagePath(0))
	}
	ix.PageCount = uint64(len(ix.Pages))

	head, err := FeedIndexHeadData{
		Current:   ix.PageCount - 1,
		UpdatedAt: headUpdatedAt,
		// Oldest omitted: nothing has been dropped. See the field's doc.
	}.ToEntity()
	if err != nil {
		return FeedIndex{}, err
	}
	ix.Head = head
	return ix, nil
}

// MintEntrySignature mints §1.1's REQUIRED detached signature over one entry,
// and returns the signature entity plus the invariant-pointer key it belongs at.
//
// ⚠ **`signer` is the CONTENT HASH OF THE SIGNER'S IDENTITY ENTITY, never the
// peer-id string.** §1.1's shorthand reads *"signer = author"* and
// `system/signature` is the kernel's type whose `signer` field is a hash — so
// an implementer reading FEED alone puts a Base58 string into a slot that wants
// a hash and produces something no verifier can use. This function is the one
// place that decision is made.
//
// **The signature is NOT part of the entity**, and that is the point most
// likely to be implemented wrongly: an entity is `(type, data)` and its hash,
// no field of it names a signer, and anyone holding the bytes reconstructs an
// identical entity — which is exactly why bytes cannot establish who wrote
// them. Root-anchored verification works while you are reading the author's
// tree and **not at all once an entry travels**, which is the whole reason this
// is required rather than optional.
//
// **It is an obligation on the COMPOSER, at authoring time** (§1.1.1). A mirror
// cannot supply it; it can only carry one the author already minted. Entries
// authored before an implementation adopted the rule can never be attributed
// once mirrored — a permanent boundary in the data, not a migration window.
func MintEntrySignature(kp *crypto.Keypair, identity entity.Entity, entryHash hash.Hash) (entity.Entity, string, error) {
	if kp == nil {
		return entity.Entity{}, "", NewError(400, "invalid_signature", "no keypair to sign with")
	}
	if identity.ContentHash.IsZero() {
		return entity.Entity{}, "", NewError(400, "invalid_signature",
			"signer identity entity has no content hash; §1.1's `signer` is that hash and NOT the peer-id string")
	}
	if entryHash.IsZero() {
		return entity.Entity{}, "", NewError(400, "invalid_signature", "nothing to sign")
	}
	sig := kp.Sign(entryHash.Bytes())
	ent, err := types.SignatureData{
		Target:    entryHash,
		Signer:    identity.ContentHash,
		Algorithm: "ed25519",
		Signature: sig,
	}.ToEntity()
	if err != nil {
		return entity.Entity{}, "", WrapError(500, "encode_signature", "encode entry signature", err)
	}
	// V7 §3.5's invariant pointer, through the kernel's own single source of
	// that path rather than a second spelling of it.
	return ent, types.LocalSignaturePath(entryHash), nil
}

// FeedPage is one page of a read, in the order a renderer shows it.
type FeedPage struct {
	Page    uint64
	Entries []EntityRef
}

// FeedCursor is §4.4's reader position: the page reached and the newest entry
// hash taken from it.
//
// **It is LOCAL READER STATE and nothing publishes it.** It carries a NUMBER as
// well as a hash because an author may remove the very entry a reader was
// holding as its position: §4.4's [MUST] is that if `Applied` no longer
// resolves, the reader resumes from `Page`. A cursor that cannot survive that
// is a cursor that breaks on edit.
type FeedCursor struct {
	Page    uint64
	Applied hash.Hash
}

// SortedIndexKeys returns an index's keys in a stable order, for reporting.
// **Per-key before any root comparison** — the one thing the peer seat asked
// back for when they accepted the producer/consumer split, because a bare root
// mismatch costs a session to localize.
func (ix FeedIndex) SortedIndexKeys() []string {
	keys := make([]string, 0, len(ix.PageKeys)+1)
	keys = append(keys, ix.HeadKey)
	keys = append(keys, ix.PageKeys...)
	sort.Strings(keys)
	return keys
}
