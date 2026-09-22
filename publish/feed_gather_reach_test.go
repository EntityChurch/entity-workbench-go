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
// # ⛔ WHAT THIS RUN FOUND, measured 2026-09-16 and re-measured 2026-09-17
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
// # ⭐ THE COUNT IS UNCHANGED AND THE CAUSE IS NOT — read the leg 4 detail line
//
// On 2026-09-16 there were two candidate causes and the first was ours.
// `SYSTEM-DATA-EXCHANGE` v0.3 §2.2.1 ruled it (arch `ROUTING-2026-09-17-a` §1,
// on `ENTITY-CORE-PROTOCOL` §1.4's authority) and **it is fixed**: the consumer
// derives the pointer under the SIGNER, passes the already-absolute path
// through unchanged, and names the serving peer as the handler. Gated on its
// own in `feed_gather_signer_test.go`, which removes the transport by
// construction — it passes, and it fails on the pre-fix consumer.
//
// **Leg 4 still reports 0, and that is not the same 0.** It used to read
//
//	nothing bound at this path        ← we asked the wrong peer's namespace
//
// and it now reads
//
//	.../system/tree?resource=/{A}/system/signature/{hex}: 403 capability_denied
//
// — the right question, refused. ⇒ **a second wall, which nobody had measured
// and which is not ours.** A republisher cannot authorize a read of its own
// copy of the author's namespace at all:
//
//   - the §3 advertisement discipline keeps a policy grant entry only if this
//     peer's advertised served-scope COVERS it (`AssembleInboundGrants` →
//     `filterAdvertisedGrants`, *"an uncovered entry is DROPPED, not
//     narrowed"*), and `advertisedServedScope` gives `system/tree` a bare `*`,
//     which §PR-8 makes the peer's OWN namespace;
//   - so adding arch's §1.3 row does not widen the grant, it **deletes the
//     entry the row was added to** — measured next door, with a control arm;
//   - and B cannot read its own carried copy either: `403 capability_denied`
//     on a LOCAL `tree:get`, for the same reason
//     [entitysdk.AppPeer.PutObtainedEntity] needed
//     [entitysdk.AppPeer.MintMirrorCapability] on the write side.
//
// ⭐ core-go's own `defaultHandlerSelfGrant` documents this exact class forty
// lines from `advertisedServedScope` — bare `*` is own-namespace-only, the
// cross-peer form is `/*/*` — and records fixing it there *because* a peer
// *"could no longer write the foreign-namespace subtrees its store legitimately
// holds under V7 §1.4's universal address space"*. The advertised scope is the
// same ceiling facing outward and still carries the old spelling. Routed as
// core-go row 22; **not shimmed here**, because a local workaround hides a cohort-wide
// question and republication is the first operation that needs this.
//
// ⚠ **Measured and NOT to be confused with a further cause:** B's signed root
// commits to **0 keys under A's namespace**. That is expected and is *not* the
// blocker — a detached signature is deliberately read OUTSIDE the committed set
// (see [fetch.Consumer.SignatureOver]'s own header), because it is
// self-verifying. It is recorded because it rules out *"carry it in the root"*
// as the fix, which is the first thing a reader of this file will reach for and
// is `A-36`'s answer one level further than that answer can go.
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

// mirrorReaderGrants is what a republisher grants a reader of its mirror: the
// view itself, plus the AUTHOR's evidence.
//
// `readPublishedGrants` alone covers only the view — B's own `app/feed/`
// prefix, its published root and its own signatures. It does **not** reach A's
// detached signatures, because a bare `system/signature/*` is peer-relative
// under §PR-8 and canonicalizes to the GRANTER's namespace, so at B it means
// *B's* signatures and covers none of A's. Naming the author —
// `/{A}/system/signature/*`, arch's `ROUTING-2026-09-17-a` §1.3 — is what
// carries authorship across the hop.
//
// ⚠ **For two days this function existed to be MEASURED rather than used**,
// because adding that row deleted the grant entry it was added to: the
// advertised served-scope covered only the granter's own namespace, and an
// uncovered entry is dropped rather than narrowed. Fixed upstream in core-go
// (`3df98f6`, our tracker row 22) and it is an ordinary grant helper now. The
// gate that pins both halves is
// `TestGatherReach_TheAuthorNamespacedGrantWidensRatherThanDeleting`.
func mirrorReaderGrants(prefix string, authors ...string) []types.GrantEntry {
	g := readPublishedGrants(prefix)
	for _, a := range authors {
		g[0].Resources.Include = append(g[0].Resources.Include, "/"+a+"/system/signature/*")
	}
	return g
}

func newMirrorTrio(t *testing.T, posts int) mirrorTrio {
	t.Helper()
	return newMirrorTrioGranting(t, posts, nil)
}

// newMirrorTrioGranting builds the trio with B's grant to C supplied by the
// caller, so a negative arm can remove exactly one row and nothing else.
// A nil grantToConsumer means the conformant one.
func newMirrorTrioGranting(t *testing.T, posts int,
	grantToConsumer func(author string) []types.GrantEntry) mirrorTrio {
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
	if grantToConsumer == nil {
		// ⚠ NOT `mirrorReaderGrants` — see its header. The author-namespaced
		// row does not widen this grant, it DELETES it, so the conformant
		// trio is the one that can express the least.
		grantToConsumer = func(string) []types.GrantEntry {
			return readPublishedGrants("app/feed/")
		}
	}
	grantAndDial(t, gatherer, consumer, grantToConsumer(author.PeerID()))

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
	trio := newMirrorTrioGranting(t, 3, func(author string) []types.GrantEntry {
		return mirrorReaderGrants("app/feed/", author)
	})

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
	// **ASSERTED as of 2026-09-17-b. It was a measurement for two days and the
	// history is why this comment is long: the leg needed TWO walls down, in
	// two different trees, and each was invisible from the other side.**
	//
	//	the consumer asks at the wrong address     OURS      fixed 2026-09-17-a
	//	the republisher cannot AUTHORIZE the read   core-go   fixed by their 3df98f6
	//
	// Ours was `SYSTEM-DATA-EXCHANGE` v0.3 §2.2.1 — bind at the signer-rooted
	// absolute path, resolve it against the peer SERVING the object, MUST NOT
	// re-qualify. Theirs was `advertisedServedScope` spelling its resources
	// axis as a bare `*`, which §PR-8 makes own-namespace-only, so a grant
	// naming the AUTHOR's namespace was dropped rather than narrowed and the
	// row deleted the entry it was added to.
	//
	// ⚠ **The grant below is what closes it, and it is load-bearing.** C reads
	// A's evidence out of B's tree only because B's policy row names
	// `/{A}/system/signature/*`; `readPublishedGrants` alone leaves this leg at
	// 0 of 3 with a 403, which is the shape a reader sees against any peer
	// still carrying the old advertised scope. That is the anti-vacuity arm in
	// `TestGatherReach_TheAuthorNamespacedGrantWidensRatherThanDeleting`.
	//
	// The consumer half is ALSO gated on its own in `feed_gather_signer_test.go`,
	// which removes the transport by construction — deliberately two gates,
	// because a single end-to-end one cannot say which wall is standing.
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
		// The signer is A — read off the mirrored reference, which leg 2
		// has just asserted names the AUTHOR and not the republisher. C
		// never speaks to A to learn it.
		if _, _, err := c.SignatureOver(ctx, "mirrored feed entry", r.Peer, ent); err != nil {
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
		if _, _, err := direct.SignatureOver(ctx, "feed entry", trio.author.PeerID(), ent); err == nil {
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

	// ---- Leg 4, asserted LAST, and the order is the point ----
	//
	// The control arm above is checked FIRST so a broken harness can never
	// present as a regression in the mirror. Only once "these entries attribute
	// when read straight from A" is established does "and they attribute
	// through B" mean anything.
	if attributed != len(mirrored) {
		t.Errorf("LEG 4 FAILED: %d of %d mirrored entries attributable by C, and the control arm "+
			"passed — so the entries are fine and the MIRROR is what stopped carrying authorship. "+
			"Two candidate causes, in two trees: this consumer resolving the §2.2 pointer at the "+
			"wrong address (ours — see feed_gather_signer_test.go, which fails first if so), or the "+
			"republisher unable to authorize the read (core-go's advertised served-scope regressing "+
			"to a bare `*`, their row 22). Detail: %s", attributed, len(mirrored), lastWhy)
	}
}

// TestGatherReach_TheAuthorNamespacedGrantWidensRatherThanDeleting is what
// arch's `ROUTING-2026-09-17-a` §6 experiment turned into once both walls came
// down — *"present a grant carrying `/{A}/system/signature/*` to a conformant
// peer and confirm it is accepted."*
//
// **It is accepted now. For two days it was not, and the way it failed is worth
// keeping in the comment**: the row did not fail to widen the grant, it DELETED
// the entry it was added to. `AssembleInboundGrants` ends in
// `filterAdvertisedGrants`, which keeps a policy entry only if this peer's
// advertised served-scope covers it — *"an uncovered entry is DROPPED, not
// narrowed"* — and `advertisedServedScope` gave `system/tree` a bare `*`, which
// §PR-8 makes own-namespace-only. So an operator widening a grant made it
// strictly narrower, silently, and the symptom landed on a read that used to
// work. Fixed in core-go by their `3df98f6` (our tracker row 22): the derived
// resources axis is the cross-peer `/*/*`.
//
// The grammar half of §1.3 was never in doubt and is still exercised here: the
// leading slash signals universal-tree scope, and `/{granter}/system/signature/*`
// was accepted throughout.
//
// This gate REPLACED a tripwire that asserted the blocked state, and the swap is
// the discipline rather than bookkeeping. A blocked-state gate must go red when
// the blocker lifts — that one did, and said so in its message — but a gate that
// only ever says *"still blocked"* cannot then protect the fix. What pins the fix
// is the property, with a control arm underneath it.
func TestGatherReach_TheAuthorNamespacedGrantWidensRatherThanDeleting(t *testing.T) {
	ctx := context.Background()

	// ---- The property: the row widens, and costs nothing that worked ----
	trio := newMirrorTrioGranting(t, 2, func(author string) []types.GrantEntry {
		return mirrorReaderGrants("app/feed/", author)
	})
	c, err := workbench.NewPeerConsumer(trio.consumer, trio.gatherer.PeerID(), nil)
	if err != nil {
		t.Fatalf("NewPeerConsumer: %v", err)
	}

	// The ORDINARY read first — B's own published root, covered by a row this
	// test did not touch. This is the half the deletion pathology broke, and it
	// is checked separately because "the widening did not take" and "the
	// widening destroyed the entry" are different failures pointing at
	// different fixes.
	if _, err := c.VerifiedRoot(ctx); err != nil {
		t.Fatalf("⛔ the author-namespaced grant row cost the reader its ORDINARY access, which is "+
			"the core-go row 22 pathology returning: adding a resource row deleted the entry it was "+
			"added to. Check `advertisedServedScope` in the sibling kernel for a resources axis that "+
			"has gone back to a bare `*` — it must be the cross-peer `/*/*`: %v", err)
	}

	// And the widening itself: A's evidence, out of B's tree, read by a peer
	// that has never spoken to A.
	carried := firstCarriedEntry(t, trio.plan)
	if _, _, err := c.SignatureOver(ctx, "mirrored feed entry", carried.Peer, carried.Entity); err != nil {
		t.Fatalf("the row is retained but carries nothing: C still cannot resolve A's detached "+
			"signature out of B's tree, so `/{A}/system/signature/*` is being accepted and not "+
			"honoured: %v", err)
	}

	// ---- The anti-vacuity arm, and without it this test proves nothing ----
	//
	// The SAME trio with exactly that row removed. If the signature still
	// resolved here, the positive arm above would be passing because everything
	// is open — a harness with no authorization in it at all — rather than
	// because the grant names the author's namespace.
	control := newMirrorTrioGranting(t, 2, func(string) []types.GrantEntry {
		return readPublishedGrants("app/feed/")
	})
	cc, err := workbench.NewPeerConsumer(control.consumer, control.gatherer.PeerID(), nil)
	if err != nil {
		t.Fatalf("control NewPeerConsumer: %v", err)
	}
	if _, err := cc.VerifiedRoot(ctx); err != nil {
		t.Fatalf("the CONTROL arm cannot make its ordinary read either, so this test is measuring a "+
			"broken harness rather than the grant row: %v", err)
	}
	cCarried := firstCarriedEntry(t, control.plan)
	_, _, err = cc.SignatureOver(ctx, "mirrored feed entry", cCarried.Peer, cCarried.Entity)
	if err == nil {
		t.Fatal("⚠ the control arm RESOLVED A's signature with no author-namespaced row in the " +
			"grant, so this harness authorizes the read some other way and the positive arm above " +
			"is vacuous. Find what is granting it before trusting either arm")
	}
	if !strings.Contains(err.Error(), "capability_denied") {
		t.Fatalf("the control arm failed for a reason that is not authorization, so it is not the "+
			"control this test needs: %v", err)
	}
	t.Logf("control — with the row removed and nothing else changed, A's evidence is refused: %v", err)
}

// firstCarriedEntry returns the first republished ENTRY in a plan — never a
// signature, which is the thing being resolved rather than the thing resolved
// over.
func firstCarriedEntry(t *testing.T, plan workbench.MirrorPlan) workbench.CarriedEntity {
	t.Helper()
	for _, ce := range plan.Carried {
		if ce.Kind == workbench.CarriedEntry {
			return ce
		}
	}
	t.Fatal("the plan carries no entry at all, so the gather is what failed and every arm below " +
		"would be measuring that instead")
	return workbench.CarriedEntity{}
}
