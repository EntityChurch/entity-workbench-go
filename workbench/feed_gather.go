// Gathering — reading another peer's published feed and republishing what it
// carried, so the next reader does not have to gather it again.
//
// ⭐ **THIS IS NOT A FEED FEATURE.** `APP-CONVENTION-FEED` §6.0a hands authority
// to `SYSTEM-DATA-EXCHANGE` §2.5 in its first line and says the exchange wins on
// any disagreement. A gatherer is **any** peer that republishes what it
// obtained; a wiki or a forum on this substrate inherits the identical rules
// with a different noun. What is feed-specific below is the type tag and the
// entry key, and nothing else.
//
// `feed_mirror.go` in `entitysdk` is the VOCABULARY and its header says what it
// deliberately left out: §6.0a's real rules — *pages filled in gather order, a
// page sealed when its successor opens, a sealed page never rewritten* — are
// properties of a SEQUENCE of writes, and `ToEntity` sees one. This file is that
// sequence. It is the first place in this tree where `PutEntity` meets bytes we
// did not author.
//
// # THE ONE RULE EVERYTHING ELSE HANGS OFF: the bytes are not ours to touch
//
// §2.1 is a `[MUST]` with a named trap: *"an implementation MUST NOT re-encode
// it — including by decoding it through a type it does not fully declare"*, and
// *"the ordinary `put(path, type, data)` shape is the one a developer reaches
// for and it is the broken one."*
//
// The failure is **not a lost field**. An entity's address is the hash of its
// bytes and a detached signature binds at a pointer derived from that hash, so
// re-encoding moves the hash and **the signature stops naming the entity**. The
// output is a complete, correctly-walked, fully-verifiable publication *in which
// nobody wrote anything* — §2.1's own words, and the centralization failure in a
// different costume.
//
// So a gathered entry travels this file as an [entity.Entity] obtained from
// [fetch.Consumer.Blob] and is handed to [entitysdk.AppPeer.PutObtainedEntity]
// verbatim. **It is never decoded and re-encoded anywhere on the write path.**
// The reader decodes for rendering; the gatherer carries. [WriteMirror] asserts
// the hash came back unmoved, because a `MUST` with no check is a comment.
//
// ⚠ **And the realistic path to breaking it is not carelessness — it is the
// ordinary shape of the code.** Decoding a body into a local struct and binding
// it back is lossless over exactly the types this build fully declares, and **a
// gatherer aggregates types it did not write**. The case that breaks it is the
// normal case and it raises no error at any layer.
//
// # WHERE FOREIGN BYTES GO — under the AUTHOR, never under us
//
// §2.2 anchors authorship at the invariant pointer
// `/{signer_peer_id}/system/signature/{target_hash_hex}`, so a republished entry
// is reached at the address **its own author** would have bound it at. A mirror
// therefore writes `/{author}/app/feed/entries/{hex}` and
// `/{author}/system/signature/{hex}` into the gatherer's own tree, and only the
// mirror record and its pages land under the gatherer.
//
// `entity-browser-rust` reached the identical shape from the other side
// (`feed_mirror.rs`, `Carried{peer, key, entity}` — *"`peer` is the author's,
// never the gatherer's"*), which is what makes a mirror readable by the reader a
// consumer already has rather than by a second one written for mirrors.
//
// ⛔ **That is why [entitysdk.AppPeer.PutObtainedEntity] had to exist.** The owner
// self-cap's `Resources: ["*"]` is peer-LOCAL under §PR-8, so every `/{them}/…`
// put is a 403. [entitysdk.AppPeer.MintMirrorCapability] names the namespace.
//
// # WHAT A GATHERER MUST NOT PUBLISH, and it is not the obvious one
//
// `DX-R4` forbids publishing, under our own namespace, **an author's own
// set-layer object over content that author did not place there.** Read flatly
// it sends you looking for an `app/feed/index` under the *gatherer* — which
// would be the gatherer making a claim only the author can make, and
// unauthenticated besides, since the authorship instrument signs entries and not
// sets. So this file carries **entries and signatures and nothing else**: every
// key it emits is derived from an entry hash, which makes the rule hold by
// construction rather than by check. `TestGather_PublishesNoSetLayerObjectOfTheAuthors`
// is `DX-C6` and inspects what a plan binds, because *a sentence true at the
// layer everyone is thinking about and false at the layer nobody is reviews
// clean forever.*
//
// # THE THREE OUTCOMES FOR ONE ENTRY, and only one of them is a drop
//
//   - **Carried with its signature** — the ordinary case.
//   - **Carried WITHOUT one** (`DX-C5`). An entry whose detached signature is
//     unobtainable is carried and presented unattributed — *not dropped, and not
//     attributed to the republisher.* §2.3 rule 3 binds the READER to present it
//     as unattributed; it does not licence a gatherer to drop it, and dropping
//     would be the wrong repair because **a mirror's only lie is omission.**
//   - **Refused** — an entry the reader already rejected under `FEED-R1`,
//     claiming an author other than the namespace it was found under.
//     Republishing it would propagate a forgery we had just finished detecting.
//
// # WHAT THIS DOES NOT DO
//
//   - **No thread gather.** [entitysdk.MirrorSubject] has two arms and a verb
//     naming a *peer* can only produce the timeline one; a thread subject is a
//     pinned entity hash, which an operator has no way to type. A thread gather
//     belongs to whatever surface can show somebody a thread.
//   - **No pointer-body closure.** An `app/embed` payload over EMBED §3's
//     16 KiB ceiling is carried by reference, and the blobs beneath it are a
//     second closure this does not walk. The other seat shipped that hole first
//     and named it (`MirrorPlan::content`); ours is stated in
//     [MirrorPlan.Notes] at plan time rather than left to be discovered, and it
//     is the next rung.
package workbench

import (
	"context"
	"fmt"
	"strings"

	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/fetch"
)

// DefaultGatherLimit is how many entries one gather carries.
//
// Bounded because **a gather is a publish**: an unbounded walk of a stranger's
// archive lets them decide how large our own tree gets, from their side of the
// wire. §1.3 already makes a short view a legal one, so the bound costs
// conformance nothing. Matched to `entity-browser-rust`'s
// `DEFAULT_GATHER_LIMIT` for `DefaultFeedPageSize`'s reason: the value is free
// and a divergence costs a joint run an explanation.
const DefaultGatherLimit = 256

// CarriedKind says why one foreign entity is in a plan.
type CarriedKind string

const (
	// CarriedEntry is the republished entry itself.
	CarriedEntry CarriedKind = "entry"

	// CarriedSignature is its author's detached `FEED-R2` signature —
	// §2.3 rule 2's *travels with*.
	CarriedSignature CarriedKind = "signature"
)

// CarriedEntity is one foreign entity a mirror publish carries, **at the
// address its own author would have bound it at**.
//
// [CarriedEntity.Peer] is the author's, never the gatherer's. That is what lets
// a consumer reach it with the reader it already has, and it is the single fact
// that decides whether a mirror is usable by anybody but its maker.
type CarriedEntity struct {
	// Peer is the AUTHOR's peer-id.
	Peer string

	// Key is peer-relative — the key the author binds this at.
	Key string

	Kind CarriedKind

	// Entity is the obtained bytes, verbatim. Nothing on the write path
	// decodes this (§2.1).
	Entity entity.Entity
}

// TreePath is where this lands in the gatherer's own tree.
func (c CarriedEntity) TreePath() string { return "/" + c.Peer + "/" + c.Key }

// GatheredEntry is one entry as a gatherer obtained it, which is a different
// shape from one a *renderer* obtained: a renderer wants the decoded body and a
// gatherer wants the bytes and the evidence.
type GatheredEntry struct {
	Hash hash.Hash

	// Entity is the obtained form. Byte-identical to what the author
	// published, recomputed and matched by [fetch.Consumer.Blob].
	Entity entity.Entity

	// Signature is the author's detached signature entity, or nil when it
	// could not be obtained — which is `DX-C5`'s case and is carried, not
	// dropped. Never synthesized: §2.3 rule 2 says a republishing peer
	// **MUST NOT** supply one, and we could not if we wanted to.
	Signature *entity.Entity

	// CreatedAt feeds [GatheredClock] and nothing else.
	CreatedAt uint64
}

// SkippedEntry is one entry a gather refused, with the reason, because a plan
// that is quietly short is indistinguishable from a publisher who posted less.
type SkippedEntry struct {
	Hash hash.Hash
	Why  string
}

// MirrorPlan is everything one mirror publish emits.
type MirrorPlan struct {
	// Subject is §6.0's coordinate this view is OF.
	Subject entitysdk.MirrorSubject

	// Author and Gatherer are the two peers a mirror relates. They are
	// kept apart in the plan because every bug in this area is one being
	// used where the other belongs.
	Author   string
	Gatherer string

	// Record is the fixed-size head (§6.0a).
	Record entitysdk.FeedMirrorData

	// Pages are §6.0a's key-addressed pages in GATHER order, page 0 first,
	// including pages this run did not change.
	Pages []entitysdk.FeedMirrorPageData

	// Carried is the foreign bytes, under their author's namespace.
	Carried []CarriedEntity

	// Unattributed counts carried entries travelling with no signature —
	// reported, never required, and never a reason to drop (`DX-C5`).
	Unattributed int

	// Skipped is what this gather refused and why.
	Skipped []SkippedEntry

	// Notes are facts a surface would otherwise have to infer.
	Notes []string
}

// EntryCount is how many entries this mirror holds across every page.
func (p MirrorPlan) EntryCount() int {
	n := 0
	for _, pg := range p.Pages {
		n += len(pg.Entries)
	}
	return n
}

// GatherOpts parameterizes one gather.
type GatherOpts struct {
	// Limit bounds how many entries are carried. Zero means
	// [DefaultGatherLimit].
	Limit int

	// PageSize is §6.0a's page capacity. Zero means
	// [entitysdk.DefaultFeedPageSize].
	//
	// Changing it on a mirror that already has pages renumbers nothing —
	// §6.0a forbids that — so existing pages keep the capacity they were
	// filled at and only new pages use the new value.
	PageSize int
}

// GatheredClock is the instant a mirror record is stamped with: the **newest
// `created_at` in the gathered set**, not a wall clock.
//
// ⚠ **`time.Now()` is what the field's name asks for and it would make every
// gather of one unchanged feed produce a different record.** The stamp lands in
// the entity, the entity lands in the trie, and the signed root moves on every
// run — which is `EXTENSION-TREE` §3.2 determinism rule 3 (*a snapshot is pure
// structural data*), and it would invalidate every consumer's cached copy of a
// view whose content did not change. A high-water mark moves whenever the
// gathered content moves and never when it has not, which is everything a
// poller needs.
//
// ⚠ **§6 gives `gathered_at` no semantics at all** — three CDDL lines and no
// prose — so which reading is meant is the convention's question and not ours.
// `entity-browser-rust` made this same call in `gathered_clock` and routed it;
// **we match them deliberately**, for the reason `updated_at` was matched on the
// index page (arch `A-41`): one seat becomes the baseline either way, and a
// silent divergence costs a joint run an explanation. A ruling moves this one
// function.
func GatheredClock(gathered []GatheredEntry) uint64 {
	var newest uint64
	for _, g := range gathered {
		if g.CreatedAt > newest {
			newest = g.CreatedAt
		}
	}
	return newest
}

// PlanMirror turns a gathered set into the exact writes one mirror publish
// makes, given what this gatherer already published for the subject.
//
// **Pure, and separated from [GatherTimeline] on purpose**: §6.0a's rules are
// about a sequence of writes, so they are gateable without a network, a
// publisher or a clock — which is what makes the paging properties testable at
// all. `prior` is this gatherer's existing pages; nil is a first gather.
//
// # GATHER ORDER, and it is not a simplification of the author's order
//
// §6.0a `[MUST]`s that pages fill in **gather** order and that a page is sealed
// when its successor opens. **A gatherer backfills, routinely, because that is
// what gathering is** — so paging in the author's order would rewrite old pages
// on every round, which is exactly the archive-republishing cost §4.3 rule 1
// exists to prevent, moved onto the peer that can least afford it. Ordering was
// never the mirror's job (§6.2 sends a reader to the author's own succession),
// and the named cost is that a reader wanting *the author's newest 50* must read
// and sort.
//
// # A SEALED PAGE'S BYTES DO NOT MOVE, which is half of `DX-C8`
//
// Sealed pages are carried through untouched, `updated_at` included. Only the
// open page is appended to, and only its stamp moves. That is what makes a
// mirror cacheable — a reader that has read page 7 never re-reads page 7 — and
// it is why the head is fixed-size whatever the size of the view it heads.
func PlanMirror(gatherer, author string, subject entitysdk.MirrorSubject,
	gathered []GatheredEntry, prior []entitysdk.FeedMirrorPageData,
	pageSize int, gatheredAt uint64) (MirrorPlan, error) {

	if strings.TrimSpace(gatherer) == "" {
		return MirrorPlan{}, fmt.Errorf("a mirror names the key that assembled it; the gatherer is empty")
	}
	if strings.TrimSpace(author) == "" {
		return MirrorPlan{}, fmt.Errorf("a mirror of a timeline names whose timeline; the author is empty")
	}
	if author == gatherer {
		// Not a validation nicety. A mirror of ourselves would bind our own
		// entries under our own namespace at the keys they already occupy
		// and publish a view whose subject is its own publisher — the
		// closure property applied to a set of one party, which is the one
		// configuration §1.3's *"assembly as a by-product of
		// participation"* cannot mean.
		return MirrorPlan{}, fmt.Errorf(
			"%s cannot gather itself: a mirror republishes what it obtained from somebody else, and "+
				"this peer's own entries are already at these keys under its own name", author)
	}
	if pageSize <= 0 {
		pageSize = entitysdk.DefaultFeedPageSize
	}

	plan := MirrorPlan{
		Subject:  subject,
		Author:   author,
		Gatherer: gatherer,
		Pages:    append([]entitysdk.FeedMirrorPageData(nil), prior...),
	}

	// What this gatherer already holds. Deduping against PRIOR and not
	// against the plan-so-far is deliberate: a re-gather of an unchanged
	// feed must add nothing, and an entry that moved from one page to
	// another would be §6.0a's forbidden renumbering wearing a different
	// name.
	held := make(map[hash.Hash]bool)
	for _, pg := range prior {
		for _, r := range pg.Entries {
			if r.Hash != nil {
				held[*r.Hash] = true
			}
		}
	}

	if len(plan.Pages) == 0 {
		plan.Pages = []entitysdk.FeedMirrorPageData{{Page: 0}}
	}

	for _, g := range gathered {
		if g.Hash.IsZero() {
			continue
		}
		if held[g.Hash] {
			continue
		}
		held[g.Hash] = true

		open := len(plan.Pages) - 1
		if len(plan.Pages[open].Entries) >= pageSize {
			// The successor opens, so the predecessor is sealed from this
			// line on and is never touched again.
			plan.Pages = append(plan.Pages, entitysdk.FeedMirrorPageData{Page: uint64(open + 1)})
			open++
		}
		plan.Pages[open].Entries = append(plan.Pages[open].Entries,
			entitysdk.PinnedRef(author, g.Hash))
		plan.Pages[open].UpdatedAt = gatheredAt

		plan.Carried = append(plan.Carried, CarriedEntity{
			Peer:   author,
			Key:    entitysdk.FeedEntryKey(g.Hash),
			Kind:   CarriedEntry,
			Entity: g.Entity,
		})
		if g.Signature != nil {
			plan.Carried = append(plan.Carried, CarriedEntity{
				Peer:   author,
				Key:    types.LocalSignaturePath(g.Hash),
				Kind:   CarriedSignature,
				Entity: *g.Signature,
			})
		} else {
			plan.Unattributed++
		}
	}

	plan.Record = entitysdk.FeedMirrorData{
		Subject:    subject.Reference(),
		Current:    uint64(len(plan.Pages) - 1),
		GatheredAt: gatheredAt,
		GatheredBy: gatherer,
	}

	// `DX-R4` as a check as well as by construction. Every key above is
	// derived from an entry hash, so this cannot fire today — which is the
	// point: it fires on whoever adds the carried kind that would break it,
	// at the line that adds it, rather than in somebody else's reader.
	for _, c := range plan.Carried {
		if isSetLayerKey(c.Key) {
			return MirrorPlan{}, fmt.Errorf(
				"refusing to carry %s: `DX-R4` forbids publishing an author's own set-layer object "+
					"under a gatherer's namespace — a gatherer signs the mirror RECORD and cannot make "+
					"a claim about what %s's feed CONTAINS, because the authorship instrument signs "+
					"entries and not sets", c.Key, author)
		}
	}

	if plan.Unattributed > 0 {
		plan.Notes = append(plan.Notes, fmt.Sprintf(
			"%d of %d carried entries travel with no verified signature, and are carried anyway "+
				"(`DX-C5`): a reader MUST present them as unattributed, and dropping them would be "+
				"a mirror lying by omission about entries the author really published",
			plan.Unattributed, len(gathered)))
	}
	plan.Notes = append(plan.Notes,
		"the blob closure beneath a pointer-arm embed body is NOT carried by this gather — an entry "+
			"whose body exceeds EMBED §3's inline ceiling will republish with its body unreachable "+
			"from this mirror")

	return plan, nil
}

// isSetLayerKey reports whether a key is one of the author's own set-layer
// objects — the things `DX-R4` forbids a gatherer to republish under its own
// name.
//
// Segment-exact on the index root, because `app/feed/index` and
// `app/feed/index/7` are both set-layer and `app/feed/indexes-of-mine` is not a
// path this convention defines at all; a substring test would be a refusal
// nobody could predict.
//
// ⚠ **§5's `app/feed/collection` is a set-layer object too and is absent here
// because this tree does not implement it** — it is the seventh FEED type and
// the one we still owe. Whoever mints it adds it to this line, and the reason
// it is safe to omit today is measured rather than assumed: a collection has no
// encoder here, so a gather has no way to obtain one.
func isSetLayerKey(key string) bool {
	return key == entitysdk.FeedIndexPath ||
		strings.HasPrefix(key, entitysdk.FeedIndexPath+"/")
}

// LoadMirror reads back what this gatherer already published for a subject.
//
// ⭐ **The other seat cannot do this and named the gap**: `--gather` projects a
// directory in one shot, so their second gather of one subject re-pages from
// scratch and `FEED-R32` — a sealed page keeping its bytes across runs — is
// gated natively and not end to end. **We write into a tree, so the prior view
// is an ordinary local read**, and the cross-run property is reachable. It is
// the one place this seat is structurally better placed, and it is worth saying
// because it is not a better implementation, it is a different substrate.
//
// A missing head is a first gather and is not an error.
func LoadMirror(ap *entitysdk.AppPeer, subject entitysdk.MirrorSubject) (
	entitysdk.FeedMirrorData, []entitysdk.FeedMirrorPageData, bool, error) {

	if ap == nil {
		return entitysdk.FeedMirrorData{}, nil, false, fmt.Errorf("no peer to read a mirror from")
	}
	headPath, err := entitysdk.FeedMirrorTreePath(ap.PeerID(), subject)
	if err != nil {
		return entitysdk.FeedMirrorData{}, nil, false, err
	}
	ent, ok, err := ap.Get(headPath)
	if err != nil || !ok {
		return entitysdk.FeedMirrorData{}, nil, false, err
	}
	head, err := entitysdk.FeedMirrorFromEntity(ent)
	if err != nil {
		return entitysdk.FeedMirrorData{}, nil, false, err
	}

	pages := make([]entitysdk.FeedMirrorPageData, 0, head.Current+1)
	for p := uint64(0); p <= head.Current; p++ {
		path, err := entitysdk.FeedMirrorPageTreePath(ap.PeerID(), subject, p)
		if err != nil {
			return head, nil, false, err
		}
		pent, ok, err := ap.Get(path)
		if err != nil {
			return head, nil, false, err
		}
		if !ok {
			// A head naming a page that is not there is the shape §6.0a's
			// write order exists to prevent, so it is reported rather than
			// silently treated as empty — resuming from a mirror we cannot
			// fully read would seal pages over a gap.
			return head, nil, false, fmt.Errorf(
				"this peer's own mirror head names page %d and %s is not bound: refusing to extend a "+
					"view whose existing pages cannot all be read, because appending past a gap seals "+
					"it permanently", p, path)
		}
		pg, err := entitysdk.FeedMirrorPageFromEntity(pent)
		if err != nil {
			return head, nil, false, err
		}
		pages = append(pages, pg)
	}
	return head, pages, true, nil
}

// MirrorWrite is what one [WriteMirror] did.
type MirrorWrite struct {
	Carried  int
	Pages    []uint64
	HeadPath string
	HeadHash hash.Hash
}

// WriteMirror performs a plan: the foreign bytes, then the pages, then the head.
//
// # THE ORDER IS LOAD-BEARING and it is the author's rule one convention over
//
// A head naming a page whose entries are not bound yet is a view that reports
// more than it can serve, **and a reader cannot tell that from a withholding
// origin** — the worst available failure, because it accuses the wrong machine.
// So the head is written last and every failure before it leaves a mirror that
// is *shorter* than it could be rather than one that overstates itself. A
// carried entry with no page row is invisible and harmless; a page row with no
// entry is a broken promise.
//
// # THE BYTE CHECK IS HERE AND NOT IN THE PLANNER
//
// §2.1's `MUST` is about what lands in the store, so it is asserted against what
// the store handed back: a returned hash that differs from the obtained one
// means something re-encoded on the way down, and every downstream reader would
// report the *author* as having published an unattributable entry. A `MUST` with
// no check is a comment.
func WriteMirror(ctx context.Context, ap *entitysdk.AppPeer, plan MirrorPlan) (MirrorWrite, error) {
	if ap == nil {
		return MirrorWrite{}, fmt.Errorf("no peer to write a mirror to")
	}
	if ap.PeerID() != plan.Gatherer {
		return MirrorWrite{}, fmt.Errorf(
			"this plan was assembled by %s and is being written by %s: `gathered_by` names the key "+
				"that assembled the view, so writing it from a different peer would publish a record "+
				"attributing our own gather to somebody else", plan.Gatherer, ap.PeerID())
	}
	out := MirrorWrite{}

	// One capability for the author's namespace, minted once. §PR-8 makes
	// the owner self-cap peer-local, so without this every carried put is
	// a 403 — and a 403 here reads as *the author refused us*, which is
	// the opposite of true: it is our own tree refusing our own write.
	var mirrorCap entity.Entity
	if len(plan.Carried) > 0 {
		var err error
		mirrorCap, err = ap.MintMirrorCapability(plan.Author)
		if err != nil {
			return out, fmt.Errorf("minting authority to bind %s's bytes in our own tree: %w",
				plan.Author, err)
		}
	}

	for _, c := range plan.Carried {
		got, err := ap.PutObtainedEntity(mirrorCap, c.TreePath(), c.Entity)
		if err != nil {
			return out, fmt.Errorf("carrying %s %s: %w", c.Kind, c.TreePath(), err)
		}
		if !c.Entity.ContentHash.IsZero() && got != c.Entity.ContentHash {
			return out, fmt.Errorf(
				"§2.1 byte preservation failed carrying %s: obtained %s, stored %s. Something "+
					"re-encoded these bytes on the way into the tree — the detached signature binds at "+
					"a pointer derived from the obtained hash, so every reader of this mirror would "+
					"report %s as having published an entry nobody signed",
				c.TreePath(), c.Entity.ContentHash, got, plan.Author)
		}
		out.Carried++
	}

	for _, pg := range plan.Pages {
		path, err := entitysdk.FeedMirrorPageTreePath(ap.PeerID(), plan.Subject, pg.Page)
		if err != nil {
			return out, err
		}
		ent, err := pg.ToEntity()
		if err != nil {
			return out, fmt.Errorf("encoding mirror page %d: %w", pg.Page, err)
		}
		if _, err := ap.PutEntity(path, ent); err != nil {
			return out, fmt.Errorf("writing mirror page %d: %w", pg.Page, err)
		}
		out.Pages = append(out.Pages, pg.Page)
	}

	headPath, err := entitysdk.FeedMirrorTreePath(ap.PeerID(), plan.Subject)
	if err != nil {
		return out, err
	}
	headEnt, err := plan.Record.ToEntity()
	if err != nil {
		return out, fmt.Errorf("encoding the mirror head: %w", err)
	}
	h, err := ap.PutEntity(headPath, headEnt)
	if err != nil {
		return out, fmt.Errorf("writing the mirror head: %w", err)
	}
	out.HeadPath, out.HeadHash = headPath, h
	return out, nil
}

// GatherTimeline reads one author's published feed through the verifying
// consumer and plans a mirror of it.
//
// ⭐ **It reads through [BrowseModel.ReadFeedOf]'s road, not its own** — same
// chooser, same per-publisher consumer memo, same shared `seq` floor, same
// cache. That is `AP108`: a gate that builds its own transport cannot fail on
// the transport the product chooses, and the feed reader has already been caught
// once by exactly that. It also matters more here than for a reader, because
// **a gatherer that read the projection some other way would be republishing
// bytes nobody verified** — and the resulting mirror is durable, signed by us,
// and indistinguishable from a good one.
//
// The bytes come back out of [fetch.Consumer.Blob], which is the one door into
// the content store and the only place the cache is filled, both past the hash
// recomputation. So the entity this republishes is the entity that was proved.
func GatherTimeline(ctx context.Context, m *BrowseModel, author string, opts GatherOpts) (MirrorPlan, error) {
	if m == nil {
		return MirrorPlan{}, fmt.Errorf("no browser to gather through")
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = DefaultGatherLimit
	}

	c, err := m.feedConsumerFor(ctx, author)
	if err != nil {
		return MirrorPlan{}, err
	}
	read, err := ReadFeed(ctx, c, FeedReadOpts{Limit: limit})
	if err != nil {
		return MirrorPlan{}, fmt.Errorf("could not read %s's feed: %w", author, err)
	}

	gathered, skipped := obtainEntries(ctx, c, read)

	subject := entitysdk.MirrorSubjectTimeline(author)
	m.mu.Lock()
	ap := m.peer
	m.mu.Unlock()
	if ap == nil {
		return MirrorPlan{}, fmt.Errorf(
			"this browser holds no peer, so there is nowhere to bind what a gather obtains — " +
				"gathering is a publish, not a read")
	}

	var prior []entitysdk.FeedMirrorPageData
	if _, pages, ok, err := LoadMirror(ap, subject); err != nil {
		return MirrorPlan{}, err
	} else if ok {
		prior = pages
	}

	gatherer := ap.PeerID()
	plan, err := PlanMirror(gatherer, author, subject, gathered, prior,
		opts.PageSize, GatheredClock(gathered))
	if err != nil {
		return MirrorPlan{}, err
	}
	plan.Skipped = append(plan.Skipped, skipped...)
	if !read.Published {
		plan.Notes = append(plan.Notes,
			"this author has published no signed root, so there was nothing to verify a feed against "+
				"and this gather carries nothing")
	}
	return plan, nil
}

// obtainEntries turns a read into the bytes a republisher needs, applying the
// three outcomes in [MirrorPlan]'s header.
func obtainEntries(ctx context.Context, c *fetch.Consumer, read FeedRead) ([]GatheredEntry, []SkippedEntry) {
	var gathered []GatheredEntry
	var skipped []SkippedEntry

	for _, row := range read.Entries {
		if row.Rejected {
			skipped = append(skipped, SkippedEntry{Hash: row.Hash, Why: "`FEED-R1`: " + row.Problem +
				" — republishing it would carry a forgery we had just finished detecting"})
			continue
		}
		if row.Hash.IsZero() {
			skipped = append(skipped, SkippedEntry{Why: "the index named an entry with no resolvable hash"})
			continue
		}
		ent, err := c.Blob(ctx, row.Hash)
		if err != nil {
			skipped = append(skipped, SkippedEntry{Hash: row.Hash,
				Why: "its bytes were not served, so there is nothing to republish: " + err.Error()})
			continue
		}

		g := GatheredEntry{Hash: row.Hash, Entity: ent, CreatedAt: row.CreatedAt}

		// Re-resolved rather than threaded out of the read, and it is
		// cache-hot because the read just did it. The alternative is a
		// second attribution path, and two code paths for one trust
		// argument is how the weaker one ends up wearing the same UI.
		if row.Attributed {
			// `read.Subject` is the author, and it is the signer: the read
			// above rejects any entry whose `author` is not the namespace it
			// was found under (`FEED-R1`), so this is the checked value and
			// not an assumption about who signs.
			if sigEnt, _, _, err := c.SignatureEntityOver(ctx, "feed entry", read.Subject, ent); err == nil {
				s := sigEnt
				g.Signature = &s
			}
		}
		gathered = append(gathered, g)
	}
	return gathered, skipped
}
