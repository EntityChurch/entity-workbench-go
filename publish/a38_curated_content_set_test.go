package publish_test

import (
	"context"
	"sort"
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/crypto"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/tree"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/publish"
	"entity-workbench-go/workbench"
)

// a38_curated_content_set_test.go — `A-38`'s ruling (D), gated.
//
// # What was ruled
//
// `entity-system-architecture`, `ROUTING-2026-09-16-a-…-both-a38s-are-ruled-
// the-prefix-is-a-bound-and-the-disclosure-is-not-a-cost-of-the-ruling` §1.
// We filed three candidate answers and all three were rejected, on landed
// text, because all three rested on the premise that widening the `prefix`
// widens what is published. `EXTENSION-TREE` §3.3a denies that three separate
// times, and the completeness `[MUST]` that would have made it true landed in
// v4.1 and was withdrawn in full.
//
//	⇒ Publish at `prefix: "/{peer_id}/"`, with a binding set of exactly the
//	  entries, the index head and pages, and the `system/signature` entities
//	  held for those entries.
//
// The 4 → 386 we measured in `a36_peer_root_probe_test.go` is the cost of
// deriving the content set from the prefix, not a cost of the ruling.
//
// # The two things this file has to establish, and why neither is enough alone
//
//   - **Nothing existing moved.** Every publish in this tree before the
//     ruling was a prefix scan, and the scan is still the default. If the
//     curated path changed what a scanned publish commits to, every fixture,
//     every corridor cut and the cross-impl trie invariant move with it —
//     silently, because a trie root is one hash and a wrong one looks exactly
//     like a right one. `TestA38_ACuratedSetOverTheWholePrefixIsStillTheScan`
//     is that assertion, made against `tree.BuildTrieForPrefix` — the
//     function this package no longer calls — so it cannot pass by agreeing
//     with itself.
//
//   - **`FEED-15`, which arch named as the new discriminating check**, and
//     whose anti-vacuity arm is the whole point: *"a fixture whose peer holds
//     nothing else passes `FEED-R38` by having nothing to leak, and measures
//     nothing. Seed the private bindings, then assert the committed key set is
//     unchanged."* So every arm below runs on a peer carrying real private
//     declarations, and asserts they are there before asserting they are out.

// curatedFeedPublish mints a peer-root root over `FeedContent`'s set on a peer
// that also holds private declarations, and returns the committed key set as a
// VERIFYING READER computes it.
//
// Read through the reader rather than out of the publisher's own structs on
// purpose: the question `A-38` is about is what a stranger can reach, and a
// publisher-side list of what it meant to commit to is the one artifact that
// cannot answer it.
func curatedFeedPublish(t *testing.T, posts int) (pair liveFeedPair, committed map[string]bool, private []string) {
	t.Helper()

	pair = newLiveFeedPairContent(t, posts, "", func(ap *entitysdk.AppPeer) {
		private = seedPrivateState(t, ap)
	}, publish.FeedContent())

	ctx := context.Background()
	c, err := workbench.NewPeerConsumer(pair.reader, pair.publisher.PeerID(), nil)
	if err != nil {
		t.Fatalf("NewPeerConsumer: %v", err)
	}
	root, err := c.VerifiedRoot(ctx)
	if err != nil {
		t.Fatalf("VerifiedRoot: %v", err)
	}
	walk, err := c.Walk(ctx, root.Data.RootHash)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	committed = map[string]bool{}
	for _, k := range walk.Keys() {
		committed[rejoin(root.Data.Prefix, k)] = true
	}
	return pair, committed, private
}

// TestA38_ACuratedSetOverTheWholePrefixIsStillTheScan is the no-regression
// arm, and it is first because everything else in this package depends on it.
//
// `mintSignedRoot` used to call `tree.BuildTrieForPrefix`; it now calls
// `tree.BuildTrie` over a binding set it is handed. When that set is the whole
// prefix — which is what a nil ContentSet means, and what every existing
// caller passes — the two MUST produce the same root, or this refactor
// silently re-cut every fixture and broke the cross-impl invariant that a trie
// is a function of its binding set.
//
// ⚠ **Asserted against `BuildTrieForPrefix` itself**, the function the
// publisher no longer uses. Re-deriving the expectation through our own
// `trieBindings` would compare the new code to itself and pass for any pair of
// consistent-but-wrong relative keys — which is exactly AP96's shape, where
// each package composed the expected path from its own copy of the constant
// and the two agreed with each other indefinitely.
func TestA38_ACuratedSetOverTheWholePrefixIsStillTheScan(t *testing.T) {
	for _, prefix := range []string{"app/feed/", ""} {
		t.Run("prefix="+prefix, func(t *testing.T) {
			// ⚠ THE EXPECTATION IS TAKEN BEFORE THE MINT, and the first
			// draft of this test took it after and failed on the peer
			// root only. The mint BINDS two entities — the published-root
			// and its signature — so a scan taken afterwards contains two
			// keys that did not exist when the root was built. That is
			// `mintSignedRoot` behaving correctly (an entity cannot appear
			// in its own preimage, which is why `entries` is snapshotted
			// first), and it is invisible under `app/feed/` because both
			// new bindings land under `system/`.
			var want hash.Hash
			pair := newLiveFeedPairSeeded(t, 3, prefix, func(ap *entitysdk.AppPeer) {
				seedPrivateState(t, ap)
				var err error
				want, err = tree.BuildTrieForPrefix(ap.RawContentStore(), ap.RawLocationIndex(),
					crypto.PeerID(ap.PeerID()), prefix)
				if err != nil {
					t.Fatalf("BuildTrieForPrefix: %v", err)
				}
			})
			ap := pair.publisher

			pr, err := ap.ReadPublishedRoot(context.Background(), ap.PeerID())
			if err != nil {
				t.Fatalf("ReadPublishedRoot: %v", err)
			}
			if pr.Data.RootHash != want {
				t.Fatalf("a scanned publish over %q no longer agrees with BuildTrieForPrefix:\n"+
					"  published %s\n  scan      %s\n"+
					"The binding set is the same set, so the roots must be the same hash — a "+
					"divergence here means the relative keys moved, which re-cuts every fixture "+
					"in this tree and breaks the cross-impl invariant that a trie is a function "+
					"of its bindings.", prefix, pr.Data.RootHash, want)
			}
		})
	}
}

// TestA38_TheCuratedSetCommitsToEverySignatureAndNoPrivateKey is `FEED-15`.
//
// Both halves in one test on purpose: they are the two directions of a single
// property, and splitting them invites one to be read as passing while the
// other is deleted. The set is *exactly* the entries, the index, and the
// signatures — so the interesting assertion is not that it contains what it
// should, it is that the peer's other 380-odd bindings did not come along.
func TestA38_TheCuratedSetCommitsToEverySignatureAndNoPrivateKey(t *testing.T) {
	pair, committed, private := curatedFeedPublish(t, 3)

	// ANTI-VACUITY FIRST. A peer that holds no private state passes the
	// exclusion arm by having nothing to leak — arch named this as the way
	// `FEED-15` measures nothing — so establish the bindings exist before
	// establishing they are out of the committed set.
	if len(private) == 0 {
		t.Fatal("seedPrivateState wrote nothing, so the exclusion arm below measures nothing")
	}
	for _, key := range private {
		if _, ok := pair.publisher.Store().Get(key); !ok {
			t.Fatalf("anti-vacuity: %q is not in the publisher's tree, so asserting it is "+
				"absent from the committed set says nothing about the content set", key)
		}
	}

	// `A-36`: the attribution fix, which is the reason any of this happened.
	for i, p := range pair.posts {
		sigKey := types.LocalSignaturePath(p.Hash)
		if !committed[sigKey] {
			t.Errorf("entry %d's FEED-R2 signature is not in the committed key set (%s) — "+
				"a static reader cannot attribute it, which is the whole of A-36", i, sigKey)
		}
		// The RELATIVE key: `committed` is keyed by §3.3a reconstruction,
		// and `PostedEntry.EntryPath` is the peer-qualified tree path.
		if entryKey := entitysdk.FeedEntryKey(p.Hash); !committed[entryKey] {
			t.Errorf("entry %d is not in the committed key set (%s)", i, entryKey)
		}
	}
	if !committed[entitysdk.FeedIndexPath] {
		t.Errorf("the index head is not in the committed key set (%s)", entitysdk.FeedIndexPath)
	}

	// `A-38`: and nothing else came with it.
	for _, key := range private {
		if committed[key] {
			t.Errorf("the committed key set contains %q, which is a private declaration this "+
				"peer holds and never published — the curated set is supposed to be exactly "+
				"the entries, the index and their signatures", key)
		}
	}

	// The strict form: enumerate what IS committed and refuse anything that
	// is not one of the three categories. The loop above only catches the
	// four keys the fixture happens to seed; this catches a selector that
	// widened to a category nobody thought to seed.
	for key := range committed {
		switch {
		case key == entitysdk.FeedIndexPath,
			strings.HasPrefix(key, entitysdk.FeedIndexPath+"/"),
			strings.HasPrefix(key, entitysdk.FeedEntryPrefix),
			strings.HasPrefix(key, "system/signature/"):
		default:
			t.Errorf("the committed key set contains %q, which is none of "+
				"(index head, index page, entry, entry signature) — the ruled set is those "+
				"three categories and nothing else", key)
		}
	}
}

// TestA38_TheCuratedStaticEmitCarriesNoPrivateBytes is the same fact in the
// form that is not arguable: bytes in the directory an operator uploads.
//
// The mint arm above is about what a root COMMITS to. This is about what
// `Publish` WRITES, and they are different failures with the same cause — a
// commitment discloses because the walk is content-addressed, and a file
// discloses because it is a file. `a36_peer_root_probe_test.go`'s third arm
// is this test's negative twin and it still measures the scan.
func TestA38_TheCuratedStaticEmitCarriesNoPrivateBytes(t *testing.T) {
	pair := newLiveFeedPairContent(t, 2, "", func(ap *entitysdk.AppPeer) {
		seedPrivateState(t, ap)
	}, publish.FeedContent())

	out := t.TempDir()
	res, err := publish.Publish(context.Background(), publish.Opts{
		Peer:      pair.publisher,
		Prefix:    "",
		OutputDir: out,
		OriginURL: "https://example.invalid/feed",
		Content:   publish.FeedContent(),
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// Anti-vacuity: the emit has to have produced something, or grepping it
	// for absent needles is a test that passes on an empty directory.
	if res.Entities == 0 {
		t.Fatal("the emit wrote no entities, so the absence assertions below measure nothing")
	}

	// The needles are the values `seedPrivateState` wrote, not the paths:
	// a path could be absent while the bytes are present under a content
	// hash, which is the shape of disclosure that matters on a CDN.
	for _, needle := range []string{
		"/home/operator/private/notes",
		"192.168.1.44:9110",
		"the contents of a file in a folder nobody shared",
		"who this peer has granted access to, and to what",
	} {
		if n := grepDir(t, out, needle); n != 0 {
			t.Errorf("the upload directory contains %q in %d file(s) — a curated feed publish "+
				"emits the entries, the index and their signatures, and this is none of those",
				needle, n)
		}
	}

	// And the positive control, so the grep is known to be able to find
	// something in this directory at all.
	if n := grepDir(t, out, "authored for the live gate"); n == 0 {
		t.Error("the upload directory does not contain the entry text either, so the four " +
			"absence assertions above cannot be distinguished from a grep that never matched")
	}
}

// TestA38_TheCuratedSetNeedsNoWholePeerAcknowledgement pins the guard's new
// boundary.
//
// `disclosureAcrossSystem` refuses a peer-root publish unless the operator
// says `-whole-peer`, because a SCAN sweeps along every key they did not name.
// A curated set sweeps nothing — every member is there because the selector
// named it — so requiring the acknowledgement would be asking an operator to
// accept a cost the ruling removed, and would make the ruled fix reachable
// only through the flag that means "publish my private tree".
//
// The control arm is the same peer-root publish WITHOUT the content set,
// which must still refuse. Without it this test passes against a build where
// the guard was deleted outright.
func TestA38_TheCuratedSetNeedsNoWholePeerAcknowledgement(t *testing.T) {
	pair := newLiveFeedPairSeeded(t, 2, "app/feed/", func(ap *entitysdk.AppPeer) {
		seedPrivateState(t, ap)
	})
	ctx := context.Background()

	if _, err := publish.MintRoot(ctx, publish.MintOpts{
		Peer:    pair.publisher,
		Prefix:  "",
		Content: publish.FeedContent(),
	}); err != nil {
		t.Fatalf("a curated peer-root publish was refused without -whole-peer: %v\n"+
			"The acknowledgement exists for the keys a SCAN sweeps along; a curated set "+
			"sweeps none, so this is the ruled fix being reachable only through the flag "+
			"that means \"publish my private tree\".", err)
	}

	// Control: the same prefix, scanned, must still refuse.
	_, err := publish.MintRoot(ctx, publish.MintOpts{
		Peer:   pair.publisher,
		Prefix: "",
	})
	if err == nil {
		t.Fatal("a SCANNED peer-root publish was accepted without -whole-peer — the guard is " +
			"gone rather than scoped, and the arm above no longer distinguishes anything")
	}
	if !strings.Contains(err.Error(), "spans the system boundary") {
		t.Fatalf("the scanned peer-root publish was refused for the wrong reason: %v", err)
	}
}

// TestA38_AFeedContentSetOnAPeerWithNoFeedRefuses is the empty-set refusal.
//
// AP97 one route over: the prefix holds bindings, and the SET chose none of
// them. Signing that would emit a valid, correctly-signed origin that answers
// "absent" to every key — `fetch.ErrEmptyEnumeration`'s whole reason — and the
// operator's actual situation is that they have not posted yet, which is a
// sentence rather than a hash.
func TestA38_AFeedContentSetOnAPeerWithNoFeedRefuses(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { ap.Close() })
	seedPrivateState(t, ap)

	_, err = publish.MintRoot(context.Background(), publish.MintOpts{
		Peer:    ap,
		Prefix:  "",
		Content: publish.FeedContent(),
	})
	if err == nil {
		t.Fatal("a feed publish on a peer with no feed was accepted — that signs a root " +
			"committing to nothing, which a consumer cannot tell from a feed it asked the " +
			"wrong question of")
	}
	if !strings.Contains(err.Error(), "selected none of them") {
		t.Fatalf("refused for the wrong reason: %v", err)
	}
}

// TestA38_TheCuratedSetIsSmallerThanTheScanAndSaysSo is the number, pinned.
//
// Not decoration: `A-38` was filed as a measurement (4 keys → 386) and the
// ruling's answer is that the 386 was never the ruling's cost. A test that
// asserts the categories but not the magnitude would pass on a "curated" set
// that quietly re-scanned, since a scan also contains every entry and every
// signature.
// ⚠ **On ONE peer.** The first draft compared the curated set against
// `publishedOverPeerRoot`'s scan, which stands up a SECOND peer — different
// keypair, different posts, therefore different entry hashes, therefore two
// key sets that can never be compared for containment. It failed loudly and
// it would have been worse if it had passed: a subset assertion across two
// fixtures is satisfied by any two sets that happen not to overlap.
func TestA38_TheCuratedSetIsSmallerThanTheScanAndSaysSo(t *testing.T) {
	pair, curated, _ := curatedFeedPublish(t, 3)

	// What the prefix BOUNDS, on this peer: every binding under the peer
	// root. That is what a scan would have committed to, and it is the set
	// the curated one has to be a subset of.
	bounded := map[string]bool{}
	for _, e := range entitysdk.ListEntriesSorted(pair.publisher.RawLocationIndex(), "") {
		bounded[peerRelativeForTest(pair.publisher.PeerID(), e.Path)] = true
	}

	if len(curated) >= len(bounded) {
		t.Fatalf("the curated set committed to %d keys and the peer root bounds %d — a curated "+
			"set that is not smaller than the scan over the same prefix is a scan",
			len(curated), len(bounded))
	}
	var outside []string
	for k := range curated {
		if !bounded[k] {
			outside = append(outside, k)
		}
	}
	sort.Strings(outside)
	if len(outside) != 0 {
		t.Fatalf("the curated set committed to keys the prefix does not bound: %v — a curated "+
			"set is a SUBSET of what the prefix bounds, and anything else has no relative_key "+
			"under §3.3a", outside)
	}
	t.Logf("A-38 (D): curated %d keys of the %d the peer-root prefix bounds",
		len(curated), len(bounded))
}

// peerRelativeForTest is the package's own `peerRelative` — duplicated here
// rather than exported, because it is one line and exporting a path helper
// from `publish` would invite a second caller to use it as a path API.
func peerRelativeForTest(peerID, path string) string {
	if strings.HasPrefix(path, "/"+peerID+"/") {
		return strings.TrimPrefix(path, "/"+peerID+"/")
	}
	return path
}

