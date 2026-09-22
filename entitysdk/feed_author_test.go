package entitysdk

import (
	"crypto/ed25519"
	"strings"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"
)

// feed_author_test.go — the gates for the production path.
//
// The load-bearing one is TestFeedAuthor_AppendEqualsFullRebuild. The shipped
// path appends to one page and never touches a sealed one; BuildFeedIndex is
// what the cross-implementation fixture measures. If those two ever disagree,
// the thing the joint run established stops describing what a peer serves —
// and nothing else in this tree would notice, because the fixture drives the
// builder directly.

// postAt posts one entry at a pinned instant, so the entry hash is reproducible
// and the arithmetic below does not depend on the clock.
func postAt(t *testing.T, f *FeedAuthor, text string, atMillis int64) PostedEntry {
	t.Helper()
	out, err := f.Post(PostRequest{Text: text, At: time.UnixMilli(atMillis)})
	if err != nil {
		t.Fatalf("post %q: %v", text, err)
	}
	return out
}

func TestFeedAuthor_AppendEqualsFullRebuild(t *testing.T) {
	ap, err := NewAppPeer()
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	f := ap.Feed()
	f.PageSize = 4 // small on purpose: three pages and a partial last one

	// 10 posts over a page size of 4 → pages 0,1 full and page 2 partial.
	// One page would pass with the two orderings swapped (the neuter the
	// joint fixture caught), so the boundary has to be crossed twice.
	const n = 10
	var entries []FeedIndexEntry
	for i := 0; i < n; i++ {
		at := int64(1_700_000_000_000 + i*1000)
		out := postAt(t, f, "post number "+string(rune('a'+i)), at)
		entries = append(entries, FeedIndexEntry{Hash: out.Hash, CreatedAt: out.CreatedAt})
	}

	// The reference: one full build over the same entries, OLDEST FIRST,
	// with the feed's high-water mark as the head stamp — exactly what the
	// joint fixture does.
	var highWater uint64
	for _, e := range entries {
		if e.CreatedAt > highWater {
			highWater = e.CreatedAt
		}
	}
	want, err := BuildFeedIndex(ap.PeerID(), entries, f.PageSize, highWater)
	if err != nil {
		t.Fatal(err)
	}

	for key, wantHash := range want.Bindings() {
		got, found, err := ap.Get("/" + ap.PeerID() + "/" + key)
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		if !found {
			t.Fatalf("%s: the appended index bound nothing here, and the reference build has it", key)
		}
		if got.ContentHash != wantHash {
			t.Errorf("%s: appended index disagrees with a full BuildFeedIndex\n  appended %s\n  rebuilt  %s",
				key, got.ContentHash, wantHash)
		}
	}
	if want.PageCount != 3 {
		t.Fatalf("expected 3 pages from %d entries at page size %d, got %d — the arithmetic this test "+
			"is about did not happen", n, f.PageSize, want.PageCount)
	}
}

// A sealed page's BINDING must not move when a later page is written — that is
// §4.3 rule 1's whole cost argument, and it is what a reader's cache and a
// re-projection actually depend on.
//
// ⚠ **What this cannot see: a rebuild that produces identical bytes.** It
// compares content hashes, so an implementation that re-encoded every page on
// every post would pass here while costing O(archive) per post. That is a
// performance property and not a correctness one — identical bytes invalidate
// no cache — but the distinction belongs in the name rather than in a claim
// this test does not make.
func TestFeedAuthor_SealedPageBindingsDoNotMoveWhenANewPageOpens(t *testing.T) {
	ap, err := NewAppPeer()
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	f := ap.Feed()
	f.PageSize = 3

	for i := 0; i < 3; i++ {
		postAt(t, f, "first page "+string(rune('a'+i)), int64(1_700_000_000_000+i*1000))
	}
	page0 := mustPageHash(t, ap, 0)
	head0 := mustHeadHash(t, ap)

	// One more post opens page 1. Page 0 is full and must not move — §4.3
	// rule 1's whole cost argument is that rewriting page 12 changes page 12
	// and nothing else.
	out := postAt(t, f, "second page a", 1_700_000_100_000)
	if !out.NewPage || out.Page != 1 {
		t.Fatalf("expected the 4th post at page size 3 to open page 1, got page=%d new=%v", out.Page, out.NewPage)
	}
	if got := mustPageHash(t, ap, 0); got != page0 {
		t.Errorf("page 0's binding moved when a post landed on page 1:\n  before %s\n  after  %s", page0, got)
	}
	// The control arm: something DID move, so the test above is not passing
	// because nothing happened at all.
	if got := mustHeadHash(t, ap); got == head0 {
		t.Errorf("the head did not move across a page boundary — this test cannot see a rewrite either")
	}
}

func TestFeedAuthor_EntrySignatureVerifiesAgainstThePeersOwnKey(t *testing.T) {
	ap, err := NewAppPeer()
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	out := postAt(t, ap.Feed(), "signed", 1_700_000_000_000)

	sigEnt, found, err := ap.Get(out.SignaturePath)
	if err != nil || !found {
		t.Fatalf("no signature bound at %s (found=%v err=%v) — FEED-R2 makes it REQUIRED, and an entry "+
			"without one is unattributable the moment it leaves this tree", out.SignaturePath, found, err)
	}
	var sig types.SignatureData
	if err := ecf.Decode(sigEnt.Data, &sig); err != nil {
		t.Fatal(err)
	}
	if sig.Target != out.Hash {
		t.Errorf("signature targets %s, entry is %s", sig.Target, out.Hash)
	}
	// The trap this asserts: `signer` is the identity ENTITY's content hash
	// and not the peer-id string. An implementer reading FEED alone puts a
	// Base58 id in this slot and produces something no verifier can use.
	if sig.Signer != ap.RawPeer().Identity().ContentHash {
		t.Errorf("signer is %s, identity entity hash is %s — §1.1's `signer = author` means the author "+
			"expressed in system/signature's own field, which is a hash", sig.Signer, ap.RawPeer().Identity().ContentHash)
	}
	pub := ed25519.PublicKey(ap.RawPeer().Keypair().PublicKey)
	if !ed25519.Verify(pub, out.Hash.Bytes(), sig.Signature) {
		t.Error("the detached signature does not verify against this peer's own public key")
	}
}

func TestFeedAuthor_RefusesABodyOverTheInlineBound(t *testing.T) {
	ap, err := NewAppPeer()
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	_, err = ap.Feed().Post(PostRequest{Text: strings.Repeat("x", EmbedInlineMaxBytes+1)})
	if err == nil {
		t.Fatal("a body over EMBED §3's inline bound was accepted; the payload is declared .size (1..16384) " +
			"and silently promoting it to a pointer is a different act")
	}
	// The bound itself is the control: one byte under must pass, or the
	// refusal above is just "posts do not work".
	if _, err := ap.Feed().Post(PostRequest{Text: strings.Repeat("x", EmbedInlineMaxBytes)}); err != nil {
		t.Fatalf("a body exactly at the bound was refused: %v", err)
	}
}

func TestFeedAuthor_ReadIsNewestFirstAcrossPages(t *testing.T) {
	ap, err := NewAppPeer()
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	f := ap.Feed()
	f.PageSize = 2

	texts := []string{"one", "two", "three", "four", "five"}
	for i, s := range texts {
		postAt(t, f, s, int64(1_700_000_000_000+i*1000))
	}

	got, err := f.Read(0)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Published {
		t.Fatal("Read reports no feed after five posts")
	}
	if len(got.Entries) != len(texts) {
		t.Fatalf("read %d entries, posted %d", len(got.Entries), len(texts))
	}
	// Newest first, across the page boundary as well as inside a page —
	// which is the pair of orderings the joint fixture's neuter 2 showed a
	// self-referential harness cannot see. This one asserts against the
	// AUTHORED sequence, which is outside the index.
	for i, want := range []string{"five", "four", "three", "two", "one"} {
		if got.Entries[i].Text != want {
			t.Errorf("position %d: got %q, want %q (newest first)", i, got.Entries[i].Text, want)
		}
	}
	if !got.Entries[0].Signed {
		t.Error("the newest entry reads as unsigned")
	}
	if got.Pages != 3 {
		t.Errorf("five entries at page size 2 should be 3 pages, head says %d", got.Pages)
	}

	lim, err := f.Read(2)
	if err != nil {
		t.Fatal(err)
	}
	if !lim.Truncated || len(lim.Entries) != 2 {
		t.Errorf("a limited read must say it is limited: truncated=%v n=%d", lim.Truncated, len(lim.Entries))
	}
}

func TestFeedAuthor_AnEmptyFeedAndAnUnpublishedOneAreDifferentAnswers(t *testing.T) {
	ap, err := NewAppPeer()
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	got, err := ap.Feed().Read(0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Published {
		t.Error("a peer that has never posted reports a published feed")
	}
	if len(got.Entries) != 0 {
		t.Errorf("a peer that has never posted returned %d entries", len(got.Entries))
	}
}

// The attribution fact, pinned as a test rather than left in prose: a root over
// a feed prefix does not commit to FEED-R2's signatures, and the surface has to
// say so. If somebody makes this pass by widening the prefix, the whole tree is
// public and the other arm catches it.
func TestFeedAuthor_SignatureCoverageIsFalseForEveryFeedPrefix(t *testing.T) {
	ap, err := NewAppPeer()
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()
	f := ap.Feed()

	for _, prefix := range []string{"app/feed/", "app/feed", "app/", "sites/"} {
		covered, note := f.SignatureCoverage(prefix)
		if covered {
			t.Errorf("%q reports the entry signatures as committed; V7 §3.5 puts them at "+
				"system/signature/{hex}, outside it", prefix)
		}
		if !strings.Contains(note, "system/signature") {
			t.Errorf("%q: the note does not name where the signatures live: %q", prefix, note)
		}
	}
	if covered, _ := f.SignatureCoverage(""); !covered {
		t.Error("the whole tree does contain system/signature/*, and saying otherwise would send an " +
			"operator looking for a prefix that cannot exist")
	}
}

func mustPageHash(t *testing.T, ap *AppPeer, page uint64) hash.Hash {
	t.Helper()
	return mustHashAt(t, ap, FeedIndexPageTreePath(ap.PeerID(), page))
}

func mustHeadHash(t *testing.T, ap *AppPeer) hash.Hash {
	t.Helper()
	return mustHashAt(t, ap, FeedIndexTreePath(ap.PeerID()))
}

func mustHashAt(t *testing.T, ap *AppPeer, path string) hash.Hash {
	t.Helper()
	ent, found, err := ap.Get(path)
	if err != nil || !found {
		t.Fatalf("nothing bound at %s (found=%v err=%v)", path, found, err)
	}
	return ent.ContentHash
}
