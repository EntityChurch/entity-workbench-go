package publish_test

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/publish"
	"entity-workbench-go/workbench"
)

// feed_live_test.go — a peer authors a feed and another peer reads it back out
// of the signed root, over the wire, with no wildcard grant anywhere.
//
// # Why this is the gate that matters for the feed work
//
// The joint fixture established that two implementations' ENCODERS agree on
// authored input. It could not establish that anything a peer actually SERVES
// is reachable, because neither seat had published one: the producer wrote
// bytes into a fixture and the comparison was between two encoders. This runs
// the other half — author → sign → publish → verify → walk → fetch → decode —
// and the walk is the authority, so a key that is not in the signed root
// cannot be smuggled in by the connection.
//
// # What a green run claims
//
//   - A feed authored through `entitysdk.FeedAuthor` is committed by a real
//     signed root, and every key §4.2 pins is reachable from it.
//   - The entries decode, through OUR OWN decoder, to the bytes that were
//     authored — the index rows are pins, so this is a hash comparison and not
//     a field-by-field one.
//   - It works under a scoped per-peer grant, no `OpenAccess`.
//
// # ⚠ What it does NOT claim, and the second one is a finding
//
//   - **Nothing cross-implementation.** One encoder, one decoder, both ours.
//     The other seat's reader is what would make this evidence about the
//     format rather than about this tree.
//   - ⛔ **Nothing about attribution.** `TestFeedLive_TheSignaturesAreNotInTheSignedRoot`
//     below MEASURES that `FEED-R2`'s detached signatures are outside the
//     committed set, and that a live reader gets them only because the grant
//     names `system/signature/*` separately. A static reader has no such
//     second channel.

// seedLiveFeed authors a feed on ap and returns the entry hashes, oldest first.
func seedLiveFeed(t *testing.T, ap *entitysdk.AppPeer, n int) []entitysdk.PostedEntry {
	t.Helper()
	f := ap.Feed()
	f.PageSize = 2 // three pages out of five posts: the walk crosses pages
	var out []entitysdk.PostedEntry
	for i := 0; i < n; i++ {
		e, err := f.Post(entitysdk.PostRequest{
			Text: fmt.Sprintf("entry %d authored for the live gate", i),
			At:   time.UnixMilli(int64(1_700_000_000_000 + i*1000)),
		})
		if err != nil {
			t.Fatalf("post %d: %v", i, err)
		}
		out = append(out, e)
	}
	return out
}

// readPublishedGrants is the scoped grant a reader of a published prefix needs.
//
// It is `readSiteGrants` with one prefix changed, and the two `system/` rows
// unchanged — which is the point worth pinning. The verification evidence sits
// outside whatever prefix is published, so the shape of a "read my published
// X" grant does not depend on X at all.
//
// The prefix is a parameter rather than a second literal for that same reason:
// the two `system/` rows are the part that must not drift between two
// spellings of "a reader of this peer's published X", and a copy is how they
// would.
func readPublishedGrants(prefix string) []types.GrantEntry {
	// An empty prefix is the whole peer namespace. `workbench.PublicSiteGrants`
	// REFUSES to derive a public grant for it, deliberately and correctly, so
	// there is no product grant to copy here — this is the harness standing one
	// up in order to measure what a peer-root publish exposes
	// (`a36_peer_root_probe_test.go`). Spelled `*` rather than `/*` because the
	// latter is a pattern §PR-8 canonicalization does not make mean "everything",
	// and a grant that silently covers nothing would make the probe measure the
	// absence of authority instead of the presence of disclosure.
	treeResource := strings.TrimSuffix(prefix, "/") + "/*"
	if strings.Trim(prefix, "/") == "" {
		treeResource = "*"
	}
	return []types.GrantEntry{
		{
			Handlers:   types.CapabilityScope{Include: []string{"system/tree"}},
			Operations: types.CapabilityScope{Include: []string{"get"}},
			Resources: types.CapabilityScope{Include: []string{
				treeResource,
				"system/peer/published-root",
				"system/signature/*",
			}},
		},
		{
			Handlers:   types.CapabilityScope{Include: []string{"system/content"}},
			Operations: types.CapabilityScope{Include: []string{"get"}},
			Resources:  types.CapabilityScope{Include: []string{"system/content"}},
		},
	}
}

// liveFeedPair is newLivePair's feed twin: a publisher that has posted and
// published, and a reader connected to it.
type liveFeedPair struct {
	publisher *entitysdk.AppPeer
	reader    *entitysdk.AppPeer
	posts     []entitysdk.PostedEntry
}

func newLiveFeedPair(t *testing.T, posts int) liveFeedPair {
	t.Helper()
	return newLiveFeedPairUnder(t, posts, "app/feed/")
}

// newLiveFeedPairUnder is the same pair with the published prefix as a
// parameter.
//
// The live-reference gate needs a root that commits to more than the feed —
// §2.2.2's rows 3 and 4 are about a path DISAPPEARING from what a publisher
// commits to, and a feed is append-only, so there is nothing in `app/feed/`
// that can go away. Rather than stand up a second two-peer harness beside this
// one (which is how the four mutual-authorization facts in AP63 would come to
// live in two places), the prefix moved out into an argument.
func newLiveFeedPairUnder(t *testing.T, posts int, prefix string) liveFeedPair {
	t.Helper()
	return newLiveFeedPairSeeded(t, posts, prefix, nil)
}

// newLiveFeedPairSeeded is the same pair with a hook that runs on the
// publisher AFTER the feed is authored and BEFORE the root is minted.
//
// The ordering is the whole reason it is a hook rather than something a
// caller does to the returned pair: a root commits to the trie as it
// stood at mint time, so state written afterwards is absent from the
// commitment. A probe that seeded after the mint would measure an empty
// set and pass while reporting that nothing is exposed.
func newLiveFeedPairSeeded(t *testing.T, posts int, prefix string, seed func(*entitysdk.AppPeer)) liveFeedPair {
	t.Helper()
	return newLiveFeedPairContent(t, posts, prefix, seed, nil)
}

// newLiveFeedPairContent is the same pair with `A-38`'s binding set as a
// parameter — nil being the prefix scan this package did exclusively until
// the ruling.
//
// A PARAMETER RATHER THAN A SECOND HARNESS, for the reason the prefix became
// one two entries above: the four mutual-authorization facts in AP63 are
// established here, in the policy row and the dial below, and a forked
// harness is how they come to live in two places and drift. The content set
// is the only variable this fork would have added.
func newLiveFeedPairContent(t *testing.T, posts int, prefix string, seed func(*entitysdk.AppPeer), content *publish.ContentSet) liveFeedPair {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	publisher, err := entitysdk.CreatePeer(entitysdk.PeerConfig{ListenAddr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("publisher CreatePeer: %v", err)
	}
	t.Cleanup(func() { publisher.Close() })

	reader, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("reader CreatePeer: %v", err)
	}
	t.Cleanup(func() { reader.Close() })

	authored := seedLiveFeed(t, publisher, posts)
	if seed != nil {
		seed(publisher)
	}

	if _, err := publish.MintRoot(ctx, publish.MintOpts{
		Peer:    publisher,
		Prefix:  prefix,
		Content: content,
		// A harness measuring what a whole-peer publish exposes has to be
		// able to perform one. The guard it is opting out of is gated on
		// its own, in `a36_peer_root_probe_test.go`.
		AllowWholePeer: true,
	}); err != nil {
		t.Fatalf("MintRoot over %q: %v", prefix, err)
	}

	// AP63: assembled at handshake, so the row precedes the dial.
	policyPath := "system/capability/policy/" + reader.PeerID()
	if _, err := publisher.Store().Put(policyPath, types.TypeCapPolicyEntry,
		types.CapabilityPolicyEntryData{
			PeerPattern: reader.PeerID(),
			Grants:      readPublishedGrants(prefix),
			Notes:       "feed live gate: this peer may read the published feed and verify it",
		}); err != nil {
		t.Fatalf("write publisher policy row: %v", err)
	}

	ready := make(chan struct{})
	listenErr := make(chan error, 1)
	go func() { listenErr <- publisher.ListenReady(ctx, ready) }()
	select {
	case <-ready:
	case err := <-listenErr:
		t.Fatalf("publisher listen: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("publisher not listening after 5s")
	}

	conn, err := reader.Connect(ctx, publisher.Addr().String())
	if err != nil {
		t.Fatalf("reader Connect: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	return liveFeedPair{publisher: publisher, reader: reader, posts: authored}
}

func TestFeedLive_APublishedFeedIsReachableFromTheSignedRoot(t *testing.T) {
	pair := newLiveFeedPair(t, 5)
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

	// ⚠ **A WALK'S KEYS ARE RELATIVE TO THE PUBLISHED PREFIX**, and the first
	// version of this test asserted on the peer-relative form and failed
	// against a perfectly correct feed. §3.3a is explicit — a consumer
	// rebuilds absolute paths as `prefix + relative_key` — so a root
	// published over `app/feed/` commits to `index`, not to
	// `app/feed/index`, while §4.2 pins the address in its peer-relative
	// form. **Both are right and a reader has to hold both**: the
	// convention names where a key lives in a namespace, the publisher
	// chooses how much of that namespace the root commits to, and the
	// difference is the prefix. Left as the loud comment it is because a
	// reader implementation that gets this wrong sees an empty feed with a
	// valid signature over it.
	prefix := root.Data.Prefix
	committed := map[string]bool{}
	for _, k := range walk.Keys() {
		committed[prefix+k] = true
	}

	// §4.2's pinned keys: the head, and one page per two entries.
	for _, want := range []string{
		entitysdk.FeedIndexPath,
		entitysdk.FeedIndexPagePath(0),
		entitysdk.FeedIndexPagePath(1),
		entitysdk.FeedIndexPagePath(2),
	} {
		if !committed[want] {
			t.Errorf("the signed root does not commit to %q — a reader starting at the one pinned "+
				"address gets a verified answer that the feed is not there.\ncommitted: %v", want, walk.Keys())
		}
	}

	// Every entry, by the key the index's pins resolve to. The index rows
	// are pins, so reaching an entry is a hash lookup and a wrong body is a
	// hash mismatch rather than a field comparison.
	for i, p := range pair.posts {
		key := entitysdk.FeedEntryKey(p.Hash)
		if !committed[key] {
			t.Fatalf("entry %d (%s) is not in the committed set", i, hex.EncodeToString(p.Hash.Bytes()))
		}
		ent, err := c.Blob(ctx, p.Hash)
		if err != nil {
			t.Fatalf("entry %d: fetching the bytes the root commits to: %v", i, err)
		}
		data, err := entitysdk.FeedEntryFromEntity(ent)
		if err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
		if data.Author != pair.publisher.PeerID() {
			t.Errorf("entry %d claims author %s under namespace %s — §1.1 makes them equal",
				i, data.Author, pair.publisher.PeerID())
		}
		// §1.1's reader-side [MUST], run here for the first time in this
		// tree against bytes that arrived over a wire rather than out of a
		// fixture.
		if err := data.ValidateInNamespace(pair.publisher.PeerID()); err != nil {
			t.Errorf("entry %d: %v", i, err)
		}
	}

	// The head says what the pages say. Read through the walk rather than
	// out of the publisher's tree — the whole point is that the reader
	// learns this from the signed root.
	headHash, err := walk.Lookup(strings.TrimPrefix(entitysdk.FeedIndexPath, prefix))
	if err != nil {
		t.Fatalf("resolving the one pinned address against the committed set: %v", err)
	}
	headEnt, err := c.Blob(ctx, headHash)
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	head, err := entitysdk.FeedIndexHeadFromEntity(headEnt)
	if err != nil {
		t.Fatal(err)
	}
	if head.Current != 2 {
		t.Errorf("head says current=%d; five entries at page size 2 is three pages, 0..2", head.Current)
	}
}

// TestFeedLive_TheSignaturesAreNotInTheSignedRoot measures the attribution gap
// rather than asserting it in prose.
//
// **Both arms are load-bearing.** The first says the signature is outside the
// committed set — so a STATIC reader, whose only authority is the root, has no
// route to it at all. The second says a LIVE reader gets it anyway, because
// the grant names `system/signature/*` as a separate resource. Without the
// second arm this would read as "the signature is unreachable", which is false
// on the road we ship and would send somebody to fix the wrong thing.
func TestFeedLive_TheSignaturesAreNotInTheSignedRoot(t *testing.T) {
	pair := newLiveFeedPair(t, 2)
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

	entry := pair.posts[0]
	sigKey := types.LocalSignaturePath(entry.Hash)
	for _, k := range walk.Keys() {
		if k == sigKey {
			t.Fatalf("the committed set contains %q — if that is now true, the note on "+
				"FeedAuthor.SignatureCoverage and the problem line in `feed` are both wrong "+
				"and should be deleted rather than left to contradict the gate", sigKey)
		}
	}

	// The other arm: it IS reachable live, by dispatch, on the grant.
	got, found, err := pair.reader.Get("/" + pair.publisher.PeerID() + "/" + sigKey)
	if err != nil || !found {
		t.Fatalf("a live reader could not fetch the entry signature either (found=%v err=%v) — then the "+
			"note is understating the problem and the feed is unattributable on BOTH roads", found, err)
	}
	if got.Type != "" && got.Type != types.TypeSignature {
		t.Errorf("signature path holds a %q", got.Type)
	}
}
