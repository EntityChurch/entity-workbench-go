package publish_test

import (
	"context"
	"testing"

	"go.entitychurch.org/entity-core-go/core/hash"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/publish"
	"entity-workbench-go/workbench"
)

// ref_outcomes_live_test.go — `APP-CONVENTION-FEED` §2.2.2's four live
// reference outcomes, against a real publisher over a real connection, plus the
// fifth state the table has no row for.
//
// # Why this cannot be a fixture test, which is why it is here
//
// Rows 2 and 3 are **structurally unreachable from a frozen fixture**: a
// publisher who never changes cannot produce a mismatch, and a static bundle
// cannot stop committing to a key it committed to. They need a publisher that
// republishes between two reads, and that is what `newLiveFeedPairUnder` is.
//
// # What a green run claims
//
//   - Each of §2.2.2's four rows is produced by the condition that defines it,
//     and comes back NAMED (`FEED-R7`: the normative half is the reader's
//     ability to tell, not which policy it picks).
//   - Row 2 is not an error and row 3 is not a failure. If either came back as
//     `err != nil` the MUST is unmet however correct the bytes are.
//   - Row 3 works twice over, from two different sources, because `seen` is a
//     hash: from this peer's own store, and from the publisher's content store
//     across the wire after the path is gone from the signed root.
//   - Resolution goes through the verified walk. Nothing here asks the
//     publisher what is at a path.
//
// # What it does NOT claim
//
//   - **Nothing about a renderer.** The outcome reaching a view is the other
//     half of `FEED-R7` and it is the `ref` verb's; a gate on this side cannot
//     see a surface dropping the field.
//   - Nothing cross-implementation: one resolver, ours.

// scratchType is a free-form application type. Deliberately not a feed type —
// these documents exist to be EDITED and REMOVED, and a feed is append-only, so
// using an entry here would model a thing the convention says cannot happen.
const scratchType = "app/workbench/scratch-note"

func putScratch(t *testing.T, ap *entitysdk.AppPeer, path, text string) hash.Hash {
	t.Helper()
	h, err := ap.Store().Put(path, scratchType, map[string]any{"text": text})
	if err != nil {
		t.Fatalf("put %s: %v", path, err)
	}
	return h
}

// republish re-mints the signed root over the same prefix. Every call moves
// `seq` forward, so the reader's floor is satisfied and each read below sees a
// genuinely newer commitment rather than a cached one.
func republish(t *testing.T, ap *entitysdk.AppPeer, prefix string) {
	t.Helper()
	if _, err := publish.MintRoot(context.Background(), publish.MintOpts{Peer: ap, Prefix: prefix}); err != nil {
		t.Fatalf("re-mint over %q: %v", prefix, err)
	}
}

func TestRefOutcomes_TheFourLiveRowsAndTheFifthState(t *testing.T) {
	const prefix = "app/"
	pair := newLiveFeedPairUnder(t, 2, prefix)
	ctx := context.Background()
	pubID := pair.publisher.PeerID()

	c, err := workbench.NewPeerConsumer(pair.reader, pubID, nil)
	if err != nil {
		t.Fatalf("NewPeerConsumer: %v", err)
	}
	// The reader's own store is row 3's near leg. Passing the reader peer is
	// also what a real surface does (`BrowseModel.ResolveReference`).
	opts := workbench.RefResolveOpts{Local: pair.reader}
	resolve := func(ref entitysdk.EntityRef) workbench.RefOutcome {
		t.Helper()
		out, err := workbench.ResolveRef(ctx, c, ref, opts)
		if err != nil {
			t.Fatalf("resolving %v: an outcome about the REFERENCE came back as an error about the "+
				"publisher: %v", ref.Path, err)
		}
		return out
	}

	// Four documents, each shaped for one row. Bound before the re-mint so
	// the root that follows commits to all of them.
	noteV1 := putScratch(t, pair.publisher, "app/refs/note", "v1 — what the linker saw")
	goneV1 := putScratch(t, pair.publisher, "app/refs/gone", "read once, then unpublished")
	unreadV1 := putScratch(t, pair.publisher, "app/refs/unread", "never read by this reader")
	ghostV1 := putScratch(t, pair.publisher, "app/refs/ghost", "bytes that will exist nowhere")
	republish(t, pair.publisher, prefix)

	// ---------------------------------------------------------------
	// Row 1 — the path resolves and it is what the linker saw
	// ---------------------------------------------------------------
	out := resolve(entitysdk.LiveRef(pubID, "/app/refs/note", noteV1))
	if out.Row != workbench.RefCurrent || out.Moved {
		t.Fatalf("row 1: got %q moved=%v, want %q moved=false — %s",
			out.Row, out.Moved, workbench.RefCurrent, out.Note)
	}
	if out.Resolved != noteV1 || !out.Have {
		t.Errorf("row 1: resolved %s have=%v, want %s with a body", out.Resolved, out.Have, noteV1)
	}

	// Row 1's other arm: no `seen` at all. **A reference with no expectation
	// can never be row 2**, so a resolver that defaulted a missing `seen` to
	// the zero hash and compared it would report every such link as moved.
	if out := resolve(entitysdk.LiveRef(pubID, "/app/refs/note", hash.Hash{})); out.Row != workbench.RefCurrent || out.Moved {
		t.Errorf("row 1 with no seen: got %q moved=%v — an absent expectation is not a differing one",
			out.Row, out.Moved)
	}

	// Read `gone` once, so its bytes are in the reader's own store when the
	// publisher stops committing to it further down. This is the ordinary
	// sequence and not a contrivance: you read a document, and afterwards the
	// author unpublishes it.
	if out := resolve(entitysdk.LiveRef(pubID, "/app/refs/gone", goneV1)); out.Row != workbench.RefCurrent {
		t.Fatalf("reading `gone` while it is still published: got %q — %s", out.Row, out.Note)
	}

	// ---------------------------------------------------------------
	// Row 2 — the path resolves to something ELSE (FEED-R7)
	// ---------------------------------------------------------------
	noteV2 := putScratch(t, pair.publisher, "app/refs/note", "v2 — the document evolved")
	if noteV2 == noteV1 {
		t.Fatal("the edit did not move the content hash, so this arm cannot measure row 2")
	}
	republish(t, pair.publisher, prefix)

	out = resolve(entitysdk.LiveRef(pubID, "/app/refs/note", noteV1))
	if out.Row != workbench.RefMoved {
		t.Fatalf("row 2: got %q, want %q — a document that evolved is the ORDINARY case and must "+
			"not be an error: %s", out.Row, workbench.RefMoved, out.Note)
	}
	if !out.Moved {
		t.Error("row 2: Moved is false. That field IS `FEED-R7` — the fact a view must be able to " +
			"surface — and a Row a caller forgets to switch on is why it is carried separately")
	}
	if out.Resolved != noteV2 || out.Seen != noteV1 {
		t.Errorf("row 2: resolved %s seen %s, want %s / %s — the current version is what is rendered "+
			"and the linker's is what it is compared against", out.Resolved, out.Seen, noteV2, noteV1)
	}
	if !out.Have {
		t.Error("row 2: no body. Rendering current is the reasonable behaviour §2.2.2 names; " +
			"withholding it turns an evolution into an outage")
	}

	// ---------------------------------------------------------------
	// Row 3 — 404 at the path, and `seen` answers anyway
	// ---------------------------------------------------------------
	// Both legs, because "any reachable source that has it" is the whole
	// reason this row does not need a link database, and one leg passing
	// says nothing about the other.
	if !pair.publisher.Store().Remove("app/refs/gone") {
		t.Fatal("unbinding `gone` did nothing, so the row-3 condition was never created")
	}
	if !pair.publisher.Store().Remove("app/refs/unread") {
		t.Fatal("unbinding `unread` did nothing")
	}
	republish(t, pair.publisher, prefix)

	out = resolve(entitysdk.LiveRef(pubID, "/app/refs/gone", goneV1))
	if out.Row != workbench.RefFellBack {
		t.Fatalf("row 3 (near leg): got %q, want %q — %s", out.Row, workbench.RefFellBack, out.Note)
	}
	if !out.Have || out.Provenance != "this peer's own store" {
		t.Errorf("row 3 (near leg): have=%v provenance=%q — this reader read those bytes while the "+
			"path was still published, so the near leg is the one that should have answered. A view "+
			"that names its own provenance is debuggable; one that does not is indistinguishable "+
			"from a bug", out.Have, out.Provenance)
	}
	if got, err := hash.ComputeFormat(goneV1.Algorithm, out.Entity.Type, out.Entity.Data); err != nil || got != goneV1 {
		t.Errorf("row 3 (near leg): the bytes served back hash to %s, not to the %s the linker saw "+
			"(err=%v) — the hash is the entire argument on this leg", got, goneV1, err)
	}
	if out.Moved {
		t.Error("row 3: Moved is set. Nothing resolved at the path, so there is no comparison to " +
			"have failed — `FEED-R7` is about a resolved hash differing")
	}

	// The far leg: bytes this reader has never held, fetched by hash from the
	// publisher's content store AFTER the signed root stopped committing to
	// the path. This is the arm that proves the fallback is not just the
	// consumer's cache answering.
	out = resolve(entitysdk.LiveRef(pubID, "/app/refs/unread", unreadV1))
	if out.Row != workbench.RefFellBack || !out.Have {
		t.Fatalf("row 3 (far leg): got %q have=%v — %s", out.Row, out.Have, out.Note)
	}
	if out.Provenance != "the publisher's content store" {
		t.Errorf("row 3 (far leg): provenance %q — these bytes were never on this reader, so the "+
			"near leg answering would mean the two legs are not distinguishable and one of them "+
			"is untested", out.Provenance)
	}
	if got, _ := hash.ComputeFormat(unreadV1.Algorithm, out.Entity.Type, out.Entity.Data); got != unreadV1 {
		t.Errorf("row 3 (far leg): served bytes hash to %s, want %s", got, unreadV1)
	}

	// ---------------------------------------------------------------
	// Row 4 — nothing resolves and nothing falls back
	// ---------------------------------------------------------------
	// 4a: no `seen` was ever offered.
	out = resolve(entitysdk.LiveRef(pubID, "/app/refs/gone", hash.Hash{}))
	if out.Row != workbench.RefDangling {
		t.Errorf("row 4a: got %q, want %q — %s", out.Row, workbench.RefDangling, out.Note)
	}
	if out.Have {
		t.Error("row 4a: a body came back for a reference that resolves to nothing")
	}

	// 4b: `seen` was offered and no reachable source has those bytes. The
	// interesting arm — 4a and 4b are the same row and different situations,
	// and only 4b exercises the fallback failing.
	if !pair.publisher.Store().Remove("app/refs/ghost") {
		t.Fatal("unbinding `ghost` did nothing")
	}
	if !pair.publisher.RawContentStore().Remove(ghostV1) {
		t.Fatal("removing ghost's bytes did nothing, so `seen` is still obtainable and this arm " +
			"measures row 3 under row 4's name")
	}
	republish(t, pair.publisher, prefix)

	out = resolve(entitysdk.LiveRef(pubID, "/app/refs/ghost", ghostV1))
	if out.Row != workbench.RefDangling || out.Have {
		t.Errorf("row 4b: got %q have=%v, want %q with no body — %s",
			out.Row, out.Have, workbench.RefDangling, out.Note)
	}
	if out.Seen != ghostV1 {
		t.Errorf("row 4b: the outcome dropped `seen` (%s). It is what somebody else would need in "+
			"order to satisfy this link", out.Seen)
	}

	// ---------------------------------------------------------------
	// The fifth state — the root commits somewhere else entirely
	// ---------------------------------------------------------------
	// Not row 4. The publisher has unpublished nothing; this root was minted
	// over a different part of the tree and says nothing about the path
	// either way. Folding the two together would report a true absence where
	// the honest answer is "you asked the wrong root", and they send an
	// operator to different places.
	out = resolve(entitysdk.LiveRef(pubID, "/system/config/anything", hash.Hash{}))
	if out.Row != workbench.RefNotCommitted {
		t.Errorf("the fifth state: got %q, want %q — %s", out.Row, workbench.RefNotCommitted, out.Note)
	}
	if out.Prefix != prefix {
		t.Errorf("the fifth state: prefix %q, want %q — the sentence is only actionable if it names "+
			"what the root DOES commit to", out.Prefix, prefix)
	}

	// ---------------------------------------------------------------
	// The pinned arm — every reference this convention emits today
	// ---------------------------------------------------------------
	// §2.2.1 makes `reply.root` and `reply.parent` pins, so a resolver that
	// handled only live references would refuse every reference a feed
	// actually carries.
	out = resolve(entitysdk.PinnedRef(pubID, unreadV1))
	if out.Row != workbench.RefPinned || !out.Have {
		t.Errorf("pin: got %q have=%v — a pin survives its path being unpublished, and this one's "+
			"path is gone from the root: %s", out.Row, out.Have, out.Note)
	}
	out = resolve(entitysdk.PinnedRef(pubID, ghostV1))
	if out.Row != workbench.RefDangling {
		t.Errorf("pin to bytes nobody holds: got %q, want %q", out.Row, workbench.RefDangling)
	}
}
