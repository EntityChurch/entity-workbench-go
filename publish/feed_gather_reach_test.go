// feed_gather_reach_test.go — `DX-C1`'s THIRD hop: publish at A, republish at
// B, **consume at C**.
//
// The other two hops are `feed_gather_live_test.go` and they are assertions.
// This one is deliberately a **measurement**, and the distinction is this
// repo's own rule: a measurement dressed as a check is how an undesigned
// behaviour gets recorded as a passing requirement. What C can reach through B
// is not something this seat has ruled — it is something nobody has run, which
// is what `DX-C1`'s own conformance row says in as many words: *"closure is
// asserted and never run."*
//
// So each leg is measured and reported by name, and a leg is asserted only
// where the answer is a rule this tree already owes.
//
// **Three peers is the whole point and two cannot express it.** With two, the
// peer serving the bytes is the peer that wrote them, so every question about
// detachment answers itself. C has never spoken to A and never will: that is
// §6.2's proposition — *one check instead of 500* — and it is the configuration
// in which a mirror either carries authorship or only carries integrity.
//
// # ⛔ WHAT THIS RUN FOUND, measured 2026-09-16
//
//	LEG 1 ok        — addressable at the derived coordinate
//	LEG 2 ok        — 3 entries named, every one referencing A and not B
//	LEG 3 ok        — B served all 3 of A's entries by hash
//	LEG 4           — 0 of 3 attributable by C
//	LEG 4 control   — 3 of 3 attributable read DIRECTLY from A
//
// **So a mirror this tree produces carries integrity and not authorship**,
// which is the exact state §2.2 says MUST NOT be presented as attributed. The
// bytes are right, the references are right, the view is addressable and
// complete — and the thing the whole tier exists for does not survive the hop.
//
// There are **two causes and they are different bugs**, which is why the run
// measures the structural one separately rather than stopping at leg 4:
//
//  1. **Ours.** [fetch.Consumer.SignatureOver] derives the §2.2 invariant
//     pointer under `c.src.PeerID()` — *the peer being read from* — where §2.2
//     names `/{signer_peer_id}/…`. Those coincide on a direct read and diverge
//     on exactly this one, so the consumer asked B for a signature bound under
//     A and got *"nothing bound at this path"*. **Measured: B HOLDS it** (the
//     gather bound it correctly), so nothing is missing — it is being looked
//     for in the wrong place.
//
//  2. **Not ours alone, and it is why (1) is not a one-line fix.** Asking
//     correctly needs a dispatched read at B for a resource under A's
//     namespace, and `AppPeer.Get` routes by peer segment, so it would
//     dispatch to **A** — the read-direction twin of the gap that made
//     [entitysdk.AppPeer.PutObtainedEntity] necessary on the write side, found
//     the same afternoon from the other end. `Source.Leaf` also prepends the
//     serving peer, and B's grant to C names `system/signature/*`, which §PR-8
//     canonicalizes peer-locally to B's own namespace.
//
// ⚠ **Measured and NOT to be confused with a third cause:** B's signed root
// commits to **0 keys under A's namespace**. That is expected and is *not* the
// blocker — a detached signature is deliberately read OUTSIDE the committed set
// (see [fetch.Consumer.SignatureOver]'s own header), because it is
// self-verifying. It is recorded because it rules out *"carry it in the root"*
// as the fix, which is the first thing a reader of this file will reach for and
// is `A-36`'s answer one level further than that answer can go.
//
// Routed rather than patched here: the fix changes the verification path, and a
// second way to locate a signature is a second trust argument.
package publish_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/fetch"
	"entity-workbench-go/publish"
	"entity-workbench-go/workbench"
)

// mirrorTrio is A (author), B (gatherer/republisher) and C (a stranger to A).
type mirrorTrio struct {
	author   *entitysdk.AppPeer
	gatherer *entitysdk.AppPeer
	consumer *entitysdk.AppPeer
	posts    []entitysdk.PostedEntry
	plan     workbench.MirrorPlan
}

func newMirrorTrio(t *testing.T, posts int) mirrorTrio {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	author := mustListeningPeer(t)
	gatherer := mustListeningPeer(t)
	consumer, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("consumer CreatePeer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	// A authors and publishes.
	authored := seedLiveFeed(t, author, posts)
	if _, err := publish.MintRoot(ctx, publish.MintOpts{Peer: author, Prefix: "app/feed/"}); err != nil {
		t.Fatalf("A MintRoot: %v", err)
	}
	grantAndDial(t, author, gatherer, readPublishedGrants("app/feed/"))

	// B gathers A through the shipped road and republishes.
	m := workbench.NewBrowseModel(nil)
	m.SetPeer(gatherer)
	plan, err := workbench.GatherTimeline(ctx, m, author.PeerID(), workbench.GatherOpts{})
	if err != nil {
		t.Fatalf("GatherTimeline: %v", err)
	}
	if _, err := workbench.WriteMirror(ctx, gatherer, plan); err != nil {
		t.Fatalf("WriteMirror: %v", err)
	}
	if _, err := publish.MintRoot(ctx, publish.MintOpts{Peer: gatherer, Prefix: "app/feed/"}); err != nil {
		t.Fatalf("B MintRoot over its own mirror: %v", err)
	}
	grantAndDial(t, gatherer, consumer, readPublishedGrants("app/feed/"))

	return mirrorTrio{author: author, gatherer: gatherer, consumer: consumer,
		posts: authored, plan: plan}
}

func mustListeningPeer(t *testing.T) *entitysdk.AppPeer {
	t.Helper()
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{ListenAddr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { ap.Close() })
	return ap
}

// grantAndDial writes the §8 policy row and dials from the client.
//
// AP63: grants are assembled AT HANDSHAKE, so the row precedes the dial — or
// the connection carries no authority and every read is a 403 that reads as a
// refusal rather than as a harness ordering bug.
func grantAndDial(t *testing.T, server, client *entitysdk.AppPeer, grants []types.GrantEntry) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	if _, err := server.Store().Put(
		"system/capability/policy/"+client.PeerID(), types.TypeCapPolicyEntry,
		types.CapabilityPolicyEntryData{
			PeerPattern: client.PeerID(),
			Grants:      grants,
			Notes:       "DX-C1: this peer may read the published view and verify it",
		}); err != nil {
		t.Fatalf("write policy row: %v", err)
	}

	ready := make(chan struct{})
	listenErr := make(chan error, 1)
	go func() { listenErr <- server.ListenReady(ctx, ready) }()
	select {
	case <-ready:
	case err := <-listenErr:
		t.Fatalf("listen: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("not listening after 5s")
	}

	conn, err := client.Connect(ctx, server.Addr().String())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
}

// TestGatherReach_WhatAThirdPartyCanReachThroughAMirror walks the four legs a
// consumer takes when it reads a gathered view, one at a time, and says which
// answer.
//
// **The legs are separated on purpose.** A single *"can C read the mirror"*
// assertion collapses four independent questions — is the view addressable, are
// its pages committed, are the republished bytes servable, is the author's
// evidence reachable — and the first three can be right while the fourth is the
// one that decides whether a mirror carries authorship or only integrity.
func TestGatherReach_WhatAThirdPartyCanReachThroughAMirror(t *testing.T) {
	ctx := context.Background()
	trio := newMirrorTrio(t, 3)

	c, err := workbench.NewPeerConsumer(trio.consumer, trio.gatherer.PeerID(), nil)
	if err != nil {
		t.Fatalf("NewPeerConsumer: %v", err)
	}

	// ---- Leg 1: is the view addressable from the subject alone? ----
	//
	// C computes the coordinate from A's peer-id. §6.0.1 makes deriving it
	// the requirement rather than a convenience, because a reader holding
	// the subject must compute the address rather than discover it.
	subject := entitysdk.MirrorSubjectTimeline(trio.author.PeerID())
	key, err := subject.Key()
	if err != nil {
		t.Fatalf("deriving the coordinate: %v", err)
	}
	root, err := c.VerifiedRoot(ctx)
	if err != nil {
		t.Fatalf("C cannot verify B's root: %v", err)
	}
	walk, err := c.Walk(ctx, root.Data.RootHash)
	if err != nil {
		t.Fatalf("C cannot walk B's root: %v", err)
	}
	committed := map[string]hash.Hash{}
	for _, b := range walk.Bindings {
		abs := fetch.AbsolutePath(root.Data.Prefix, trio.gatherer.PeerID(), b.Key)
		committed[abs] = b.Hash
	}
	headAbs := "/" + trio.gatherer.PeerID() + "/" + key
	headHash, ok := committed[headAbs]
	if !ok {
		t.Fatalf("LEG 1 FAILED: B's signed root does not commit to the mirror head at the derived "+
			"coordinate %s. A view nobody can address from the subject is not a source leg", headAbs)
	}
	t.Logf("LEG 1 ok — the mirror head is committed at the derived coordinate %s", key)

	// ---- Leg 2: are the pages committed, and do they name A's entries? ----
	headEnt, err := c.Blob(ctx, headHash)
	if err != nil {
		t.Fatalf("LEG 2 FAILED: C cannot fetch the head it just proved is committed: %v", err)
	}
	head, err := entitysdk.FeedMirrorFromEntity(headEnt)
	if err != nil {
		t.Fatalf("LEG 2 FAILED: the mirror head does not decode: %v", err)
	}
	var mirrored []entitysdk.EntityRef
	for p := uint64(0); p <= head.Current; p++ {
		pk, err := subject.PageKey(p)
		if err != nil {
			t.Fatal(err)
		}
		ph, ok := committed["/"+trio.gatherer.PeerID()+"/"+pk]
		if !ok {
			t.Fatalf("LEG 2 FAILED: the head names page %d and B's root does not commit to it", p)
		}
		pent, err := c.Blob(ctx, ph)
		if err != nil {
			t.Fatalf("LEG 2 FAILED: page %d is committed and not served: %v", p, err)
		}
		pg, err := entitysdk.FeedMirrorPageFromEntity(pent)
		if err != nil {
			t.Fatalf("LEG 2 FAILED: page %d does not decode: %v", p, err)
		}
		mirrored = append(mirrored, pg.Entries...)
	}
	if len(mirrored) != len(trio.posts) {
		t.Fatalf("LEG 2 FAILED: the mirror names %d entries, A published %d",
			len(mirrored), len(trio.posts))
	}
	for _, r := range mirrored {
		if r.Peer != trio.author.PeerID() {
			t.Errorf("a mirrored entry names peer %q, want the AUTHOR %q — `DX-R13` forbids "+
				"attributing a republished entry to its republisher", r.Peer, trio.author.PeerID())
		}
	}
	t.Logf("LEG 2 ok — %d entries named, every one referencing A and not B", len(mirrored))

	// ---- Leg 3: will B serve A's bytes? ----
	//
	// The content-addressed leg, and the one with no right to care whose
	// bytes they are: a pinned reference is a hash, the bytes prove
	// themselves against it, and B holds them because the gather bound them.
	served := 0
	for _, r := range mirrored {
		if r.Hash == nil {
			continue
		}
		if _, err := c.Blob(ctx, *r.Hash); err != nil {
			t.Errorf("LEG 3: B does not serve mirrored entry %s: %v", *r.Hash, err)
			continue
		}
		served++
	}
	if served != len(mirrored) {
		t.Fatalf("LEG 3 FAILED: B served %d of %d entries it names — a view naming entries it "+
			"cannot serve reports more than it holds, and a reader cannot tell that from a "+
			"withholding origin", served, len(mirrored))
	}
	t.Logf("LEG 3 ok — B served all %d of A's entries by hash, from its own content store", served)

	// ---- Leg 4: can C ATTRIBUTE them? ----
	//
	// §2.2's whole proposition is evidence of authorship that **survives
	// detachment from the author's signed root**. C has never spoken to A.
	// If this does not answer, the mirror carries integrity without
	// authorship — which §2.2 says MUST NOT be presented as attributed.
	//
	// **Reported, not asserted.** Where a signature pointer resolves when
	// the serving peer is not the signing peer is precisely the fact this
	// run exists to establish, and asserting either answer would be writing
	// down a ruling nobody has made.
	attributed := 0
	var lastWhy string
	for _, r := range mirrored {
		if r.Hash == nil {
			continue
		}
		ent, err := c.Blob(ctx, *r.Hash)
		if err != nil {
			continue
		}
		if _, _, err := c.SignatureOver(ctx, "mirrored feed entry", ent); err != nil {
			lastWhy = err.Error()
			continue
		}
		attributed++
	}
	t.Logf("LEG 4 — %d of %d mirrored entries attributable by C", attributed, len(mirrored))
	if lastWhy != "" {
		t.Logf("LEG 4 detail — %s", lastWhy)
	}

	// The control arm, and without it leg 4 is uninterpretable: the same
	// entries read STRAIGHT FROM A. If those do not attribute either, the
	// measurement is about this harness and not about mirrors.
	direct, err := workbench.NewPeerConsumer(trio.gatherer, trio.author.PeerID(), nil)
	if err != nil {
		t.Fatalf("direct consumer: %v", err)
	}
	directOK := 0
	for _, r := range mirrored {
		if r.Hash == nil {
			continue
		}
		ent, err := direct.Blob(ctx, *r.Hash)
		if err != nil {
			continue
		}
		if _, _, err := direct.SignatureOver(ctx, "feed entry", ent); err == nil {
			directOK++
		}
	}
	t.Logf("LEG 4 control — %d of %d attributable when read DIRECTLY from A",
		directOK, len(mirrored))

	// ---- Why leg 4 answered the way it did, measured rather than reasoned ----
	//
	// Two candidate causes, and they are not the same bug: the CONSUMER
	// could be asking at the wrong address, or B's root could be unable to
	// commit to the right one. The second is structural and decides whether
	// the first is worth fixing, so it is counted here.
	underAuthor := 0
	for abs := range committed {
		if strings.HasPrefix(abs, "/"+trio.author.PeerID()+"/") {
			underAuthor++
		}
	}
	t.Logf("STRUCTURE — B's signed root commits to %d keys in total, %d of them under A's "+
		"namespace. The §2.2 invariant pointer is /{signer}/system/signature/{hex}, so A's "+
		"namespace is where every carried signature is bound in B's tree",
		len(committed), underAuthor)

	sigKey := "/" + trio.author.PeerID() + "/" + types.LocalSignaturePath(*mirrored[0].Hash)
	_, sigCommitted := committed[sigKey]
	t.Logf("STRUCTURE — is %s in B's committed key set? %v", sigKey, sigCommitted)

	// B holds it — the gather bound it — so this separates "not carried"
	// from "carried and not committable", which are different findings
	// pointing at different seats.
	_, held := trio.gatherer.Store().Get(sigKey)
	t.Logf("STRUCTURE — does B HOLD it in its own tree? %v", held)
	if !held {
		t.Fatal("B does not hold the carried signature at all, so the structural question above is " +
			"moot and the gather itself is what failed")
	}
	if directOK != len(mirrored) {
		t.Fatalf("the control arm failed: only %d of %d entries attribute when read straight from "+
			"their author, so leg 4 measures this harness rather than the mirror",
			directOK, len(mirrored))
	}
}
