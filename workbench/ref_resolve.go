package workbench

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/fetch"
)

// ref_resolve.go — resolving one `APP-CONVENTION-REFERENCE` atom, and
// saying WHICH of `APP-CONVENTION-FEED` §2.2.2's outcomes came back.
//
// # The deliverable is the outcome, not the entity
//
// §2.2.2 gives a live reference four outcomes and **[MUST]s that a reader
// be able to tell which one it got** (`FEED-R7`). A `(entity, error)`
// signature cannot carry that: it collapses rows 2, 3 and 4 into
// *"something went wrong"*, and **row 2 is not a failure at all** — it is
// the ordinary case of a document having evolved since somebody linked to
// it. So this returns a typed [RefOutcome] and reserves its error return
// for faults that are about the PUBLISHER rather than about the reference
// (unreachable, unverifiable, withholding).
//
// The normative half is the *ability to tell*, not the policy. Strict
// (refuse on mismatch) and lenient (render current) are both legitimate
// and both are the caller's choice — which is exactly why the choice is
// not made here. What is required is that the fact reaches the view:
// [RefOutcome.Moved] and [RefOutcome.Note] are that fact, and a renderer
// that drops them satisfies nothing however correct the bytes are (D23 at
// field granularity; AP49 is the same defect one boundary further out,
// where an undeclared DTO field is discarded in silence and renders as
// *"everything is fine"*).
//
// # Resolution goes through the verified walk, never a bare tree:get
//
// A dispatched `system/tree:get` at the publisher would answer this
// question in one round trip and prove the wrong thing: **an
// authenticated connection proves WHO, not WHAT** (`peer_source.go`). The
// signed root is the authority for what a publisher committed to, so a
// live reference resolves against the committed key set and nowhere else.
// A second verification path is the thing that must not exist — said in
// W1, said again in W2, and this is where it was tempting.
//
// # Row 3 is the elegant one and it is safe from anywhere
//
// `seen` is a content hash, so the bytes are self-validating: any store
// that has them satisfies it and no store can substitute for it. That is
// what §2.2.2 means by *"the publisher's declared content origin or any
// reachable source that has it"*, and it is why a live reference survives
// its author unpublishing the path **with no link database anywhere**.
// This peer's own store is tried first — it is a source, it is reachable,
// and it costs no round trip.
//
// # The fifth outcome, which §2.2.2 does not have a row for
//
// A reference names a `(peer, path)`; a published root commits to a
// PREFIX. A path outside that prefix is not row 3 — the publisher has not
// unpublished anything, it has committed to a different region of its
// tree and this root says nothing whatever about the path. Folding that
// into *"dangling"* would report a true absence where the honest answer
// is *"asked the wrong root"*, and the two send an operator to different
// places. [RefNotCommitted] keeps them apart.

// RefRow is which of §2.2.2's outcomes a resolution produced.
//
// A string rather than an int so it survives a bridge, a log line and a
// JSON envelope without a lookup table on the far side — the value IS the
// name, which is the property that made `Reconciled` worth carrying in
// `shellcmd/status.go`'s outcome rather than remembering at the surface.
type RefRow string

const (
	// RefPinned is a pin that resolved: these exact bytes, from whoever
	// had them. Not one of §2.2.2's rows — that table is about live
	// references — and carried here because `reply.root` and
	// `reply.parent` are pins (§2.2.1), so a resolver that handled only
	// live references would refuse every reference this convention
	// actually emits today.
	RefPinned RefRow = "pinned"

	// RefCurrent is row 1: the path resolves, and either it matches
	// `seen` or the linker offered no expectation.
	RefCurrent RefRow = "current"

	// RefMoved is row 2: the path resolves to something OTHER than what
	// the linker saw. **The ordinary case, and not an error** — the
	// document evolved. `FEED-R7`'s MUST is about this row reaching the
	// view.
	RefMoved RefRow = "moved"

	// RefFellBack is row 3: the path is not in the committed set, and
	// the bytes the linker saw were obtained anyway, by hash.
	RefFellBack RefRow = "fell-back-to-seen"

	// RefDangling is row 4: nothing resolves and nothing falls back —
	// either no `seen` was offered or no reachable source has it. The
	// honest failure; nothing to hide.
	RefDangling RefRow = "dangling"

	// RefNotCommitted is the fifth state (see the file note): the
	// publisher's signed root commits to a prefix that does not contain
	// this path, so it answers neither "here" nor "gone".
	RefNotCommitted RefRow = "not-committed"
)

// RefOutcome is one resolution, with the comparison result carried as
// information rather than as an error.
type RefOutcome struct {
	// Ref is what was asked for, verbatim.
	Ref entitysdk.EntityRef

	// Row is which outcome this is. Always set.
	Row RefRow

	// Entity is the body a view should render, when there is one. Check
	// Have rather than the zero value — an entity type is a string and
	// an empty one is a decode fault, not an absence.
	Entity entity.Entity
	Have   bool

	// Resolved is what `(peer, path)` resolves to NOW, zero when it does
	// not resolve. For a pin it is the pin.
	Resolved hash.Hash

	// Seen is what the linker saw, zero when they offered no
	// expectation. **An expectation and never the identity** (§2.2).
	Seen hash.Hash

	// Moved is `FEED-R7`'s fact: the path resolved, and to something
	// other than `seen`. **This is the field the MUST is about.** It is
	// separate from Row on purpose — a caller switching on Row can
	// forget a case; a caller rendering provenance reads one boolean.
	Moved bool

	// Provenance is where the bytes in Entity came from, in a phrase. A
	// view that names its own provenance is debuggable; one that does
	// not is indistinguishable from a bug (§2.2.2).
	Provenance string

	// Prefix is what the publisher's signed root commits to, as the root
	// spells it (§3.3a's configured form). Empty for a pin, which needs
	// no root at all.
	Prefix string

	// Note is the one sentence a surface renders. Written for the person
	// looking at the screen, and it names the next thing to do wherever
	// there is one.
	Note string
}

// RefResolveOpts is the optional half of a resolution.
type RefResolveOpts struct {
	// Local is a peer whose own content store may already hold bytes
	// addressed by hash — row 3's first and cheapest leg, and the pin's.
	//
	// Optional: a resolver with no local peer is the `entity-fetch`
	// configuration and is correct, it just always pays a round trip.
	Local *entitysdk.AppPeer
}

// ResolveRef resolves one reference atom against a verifying reader.
//
// `c` must be bound to the publisher the reference names. That is checked
// rather than assumed: a reference is routable precisely BECAUSE it names
// its publisher (§1), so resolving one against a different peer would
// verify the wrong key and answer confidently about the wrong tree.
//
// The error return is reserved for faults about the publisher — it is
// unreachable, it has published nothing, its root does not verify, its
// walk is incomplete. Every outcome that is about the REFERENCE comes
// back as a [RefOutcome] with `err == nil`.
func ResolveRef(ctx context.Context, c *fetch.Consumer, ref entitysdk.EntityRef, opts RefResolveOpts) (RefOutcome, error) {
	if c == nil {
		return RefOutcome{}, fmt.Errorf("workbench: resolving a reference needs a verifying reader")
	}
	if err := ref.Validate(); err != nil {
		return RefOutcome{}, err
	}
	if ref.Peer != c.PeerID() {
		return RefOutcome{}, fmt.Errorf(
			"workbench: this reference names peer %s and the reader is bound to %s — a reference is "+
				"routable because it names its publisher, so resolving it against another one checks "+
				"the wrong key and answers about the wrong tree", ref.Peer, c.PeerID())
	}

	out := RefOutcome{Ref: ref}
	if ref.Seen != nil {
		out.Seen = *ref.Seen
	}
	if ref.IsPinned() {
		return resolvePinnedRef(ctx, c, out, opts), nil
	}
	return resolveLiveRef(ctx, c, out, opts)
}

// resolvePinnedRef answers a pin, which needs no signed root.
//
// **That is a property worth naming rather than an implementation
// shortcut: a pin survives its author never having published at all.**
// The hash is the whole argument — [fetch.Consumer.Blob] recomputes it
// before anything is returned — so the publisher's commitments are not
// consulted because they add nothing.
func resolvePinnedRef(ctx context.Context, c *fetch.Consumer, out RefOutcome, opts RefResolveOpts) RefOutcome {
	h := *out.Ref.Hash
	out.Resolved = h
	ent, from, err := bytesForHash(ctx, c, h, opts)
	if err != nil {
		out.Row = RefDangling
		out.Note = fmt.Sprintf("dangling — this pin names bytes (%s) that nothing reachable holds. "+
			"A pin can never resolve to anything else, so this is a statement about who has them, "+
			"not about whether they were right", shortRefHash(h))
		return out
	}
	out.Row, out.Entity, out.Have, out.Provenance = RefPinned, ent, true, from
	out.Note = fmt.Sprintf("pinned — these exact bytes, from %s. A pin is self-verifying: any holder "+
		"satisfies it and the answer can never change", from)
	return out
}

// resolveLiveRef answers a live reference, which is §2.2.2's table.
func resolveLiveRef(ctx context.Context, c *fetch.Consumer, out RefOutcome, opts RefResolveOpts) (RefOutcome, error) {
	root, err := c.VerifiedRoot(ctx)
	if err != nil {
		// About the publisher, not about the reference: unreachable,
		// never published, or a root that did not verify. Passed through
		// unwrapped so `fetch.ErrNoPublishedRoot` stays distinguishable —
		// "committed to nothing" is a third state and a surface says so.
		return RefOutcome{}, err
	}
	walk, err := c.Walk(ctx, root.Data.RootHash)
	if err != nil {
		return RefOutcome{}, err
	}
	out.Prefix = root.Data.Prefix

	abs := "/" + out.Ref.Peer + out.Ref.Path
	key, inside := refKeyUnderPrefix(abs, fetch.AbsolutePrefix(root.Data.Prefix, out.Ref.Peer))
	if !inside {
		out.Row = RefNotCommitted
		out.Note = fmt.Sprintf("this publisher's signed root commits to %q, which does not contain %q — "+
			"so the root says nothing about this path either way. That is NOT the same as the path "+
			"being gone: a peer has exactly one published root, and this one was minted over a "+
			"different part of the tree", root.Data.Prefix, out.Ref.Path)
		return out, nil
	}

	h, lookupErr := walk.Lookup(key)
	switch {
	case lookupErr == nil:
		ent, err := c.Blob(ctx, h)
		if err != nil {
			// The signed root commits to this key and the publisher does
			// not serve its bytes. That is §6.5.3's withholding case and
			// it is a fault about the publisher, so it is an error here
			// and not a row — the reference was fine.
			return RefOutcome{}, fmt.Errorf("the signed root commits %q to %s and its bytes were not "+
				"served: %w", out.Ref.Path, h, err)
		}
		out.Resolved, out.Entity, out.Have = h, ent, true
		out.Provenance = "the publisher's signed root"
		switch {
		case out.Seen.IsZero():
			out.Row = RefCurrent
			out.Note = "current — the path resolves, and the link carried no `seen` hash, so there is " +
				"no earlier version to have differed from"
		case out.Seen == h:
			out.Row = RefCurrent
			out.Note = "current — what this publisher commits to at that path is exactly what the " +
				"linker saw"
		default:
			out.Row, out.Moved = RefMoved, true
			out.Note = fmt.Sprintf("MOVED — this is the current document and it is not the one the link "+
				"was made against: the linker saw %s, the path now resolves to %s. The version they "+
				"saw is still fetchable by its hash", shortRefHash(out.Seen), shortRefHash(h))
		}
		return out, nil

	case errors.Is(lookupErr, fetch.ErrEmptyEnumeration):
		// An empty root answers "absent" to every key, so absence here
		// carries no information at all — reporting it as row 3 or row 4
		// would be manufacturing a finding about the path out of a fact
		// about the root.
		out.Row = RefNotCommitted
		out.Note = "this publisher's signed root commits to no keys at all, so it answers \"absent\" " +
			"to every path and that answer carries no information about this one"
		return out, nil

	case errors.Is(lookupErr, fetch.ErrAbsent):
		return fallBackToSeen(ctx, c, out, opts), nil

	default:
		return RefOutcome{}, lookupErr
	}
}

// fallBackToSeen is rows 3 and 4: the path is inside what the publisher
// commits to and is not in the committed set.
//
// **The publisher not committing to the path is not a failure to report
// up.** §2.2.2 row 3 is a normal outcome with a normal answer, and the
// answer is the hash the linker recorded.
func fallBackToSeen(ctx context.Context, c *fetch.Consumer, out RefOutcome, opts RefResolveOpts) RefOutcome {
	if out.Seen.IsZero() {
		out.Row = RefDangling
		out.Note = fmt.Sprintf("dangling — %q is not in what this publisher commits to, and the link "+
			"carried no `seen` hash, so there is nothing to fall back to", out.Ref.Path)
		return out
	}
	ent, from, err := bytesForHash(ctx, c, out.Seen, opts)
	if err != nil {
		out.Row = RefDangling
		out.Note = fmt.Sprintf("dangling — %q is gone from what this publisher commits to, and nothing "+
			"reachable holds %s, which is what the linker saw. Any store that has those bytes would "+
			"satisfy this link", out.Ref.Path, shortRefHash(out.Seen))
		return out
	}
	out.Row, out.Entity, out.Have, out.Provenance = RefFellBack, ent, true, from
	out.Note = fmt.Sprintf("the path is gone from what this publisher commits to — showing what the "+
		"linker saw (%s), from %s. The hash validates the bytes whoever serves them, which is why "+
		"this link survives the author unpublishing it", shortRefHash(out.Seen), from)
	return out
}

// bytesForHash obtains content-addressed bytes from any reachable source,
// nearest first, and proves they are the bytes the hash names.
//
// **Every leg is self-validating, which is what makes "any source" safe.**
// The local leg re-derives the hash rather than trusting the store's own
// key (the store does key by a hash it computes, and this is the one leg
// with no signed root behind it, so the hash IS the whole argument); the
// remote leg goes through [fetch.Consumer.Blob], which is the single door
// into the content store and recomputes before it returns.
func bytesForHash(ctx context.Context, c *fetch.Consumer, h hash.Hash, opts RefResolveOpts) (entity.Entity, string, error) {
	if opts.Local != nil {
		if ent, ok := localBytesForHash(ctx, opts.Local, h); ok {
			return ent, "this peer's own store", nil
		}
	}
	ent, err := c.Blob(ctx, h)
	if err != nil {
		return entity.Entity{}, "", err
	}
	return ent, "the publisher's content store", nil
}

// localBytesForHash asks this peer's own content store, and answers
// (zero, false) for every failure — a miss and a fault are the same thing
// to a caller that is about to try somewhere else.
func localBytesForHash(ctx context.Context, ap *entitysdk.AppPeer, h hash.Hash) (entity.Entity, bool) {
	res, err := ap.Content().Get(ctx, []hash.Hash{h})
	if err != nil {
		return entity.Entity{}, false
	}
	ent, ok := res.Entities[h]
	if !ok {
		return entity.Entity{}, false
	}
	got, err := recomputeServedHash(ent)
	if err != nil || got != h {
		return entity.Entity{}, false
	}
	return ent, true
}

// refKeyUnderPrefix turns an absolute tree path into the key a published
// root's walk would carry it under, and reports whether the root covers
// it at all.
//
// §3.3a makes reconstruction pure concatenation — `absolute_prefix +
// relative_key` — so the inverse is a string trim and not a path join.
// **The boundary test is segment-exact anyway**, for the case a prefix
// does not end in a separator: `app/feed` would otherwise swallow
// `app/feedback/…`, which is the substring bug that is right on every
// example anybody thinks of (`prefixCoversFeed` and `Layout.RebaseTo`
// are written this way for the same reason).
//
// An empty absolute prefix is §3.3's universal tree, where the committed
// keys are already fully qualified and every path is covered.
func refKeyUnderPrefix(abs, absPrefix string) (string, bool) {
	if absPrefix == "" {
		return abs, true
	}
	if !strings.HasPrefix(abs, absPrefix) {
		return "", false
	}
	rest := abs[len(absPrefix):]
	if strings.HasSuffix(absPrefix, "/") || rest == "" || strings.HasPrefix(rest, "/") {
		return rest, true
	}
	return "", false
}

// shortRefHash is a hash at reading length. The full form is in the
// outcome's fields; a sentence carrying 66 hex characters is a sentence
// nobody finishes.
func shortRefHash(h hash.Hash) string {
	s := h.String()
	if len(s) <= 20 {
		return s
	}
	return s[:20] + "…"
}

// ResolveReference resolves one reference through the browser's own road
// chooser, consumer cache and `seq` floor.
//
// **The road is the live one and that is forced, not preferred.** A
// reference names a peer and carries no origin (§2.1's four terms are
// who / what / which part / where to look first, and `via` is advisory),
// so there is no static corridor to choose — the same position
// `BrowseModel.goTo` is in for a peer-id address. `canReachLive` is the
// predicate: it is us, or we already hold a connection. It never dials a
// guess, because inventing an address is the one move that can put a
// stranger's bytes behind a peer-id somebody typed.
//
// Going through [BrowseModel.consumerForRoad] rather than building a
// consumer here is what keeps the `seq` floor one per publisher (AP100):
// a resolution and a navigation against the same peer share a floor, so a
// root this browser has already seen cannot be replayed at one of them.
func (m *BrowseModel) ResolveReference(ctx context.Context, ref entitysdk.EntityRef) (RefOutcome, error) {
	if err := ref.Validate(); err != nil {
		return RefOutcome{}, err
	}
	m.mu.Lock()
	ap := m.peer
	m.mu.Unlock()

	if why, ok := m.canReachLive(ref.Peer); !ok {
		return RefOutcome{}, fmt.Errorf(
			"cannot resolve a reference to %s: %s. A reference names a publisher and no origin, so the "+
				"only road to it is asking them", ref.Peer, why)
	}
	c, err := m.consumerForRoad(ctx, browseRoad{
		Class:     fetch.ClassLive,
		Candidate: fetch.TransportCandidate{Class: fetch.ClassLive, PeerID: ref.Peer},
	})
	if err != nil {
		return RefOutcome{}, err
	}
	return ResolveRef(ctx, c, ref, RefResolveOpts{Local: ap})
}
