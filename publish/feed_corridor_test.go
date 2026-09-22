package publish_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"entity-workbench-go/fetch"
	"entity-workbench-go/workbench"
)

// feed_corridor_test.go — corridor ①, read back by our own reader before
// it is handed to anybody else's.
//
// `cmd/crossimpl-feed` emits the two cuts `entity-browser-rust` asked for
// as their `B-13` and arch adopted whole (`ROUTING-2026-09-15-a` §2): 34
// entries against a 32-entry page size, published once over the peer root
// and once over `app/feed/`. This drives **our** static reader at both.
//
// # Why a fixture gets a gate on the emitting side
//
// Because the value of the corridor is that a READER on the other side
// reads it, and the cheapest way to waste that is to hand over bytes we
// never read ourselves. That is not hypothetical: writing this found a
// live defect in our own reader — see
// `TestFeedCorridor_PeerRootCutIsReadable`.
//
// ⚠ **This is not cross-implementation evidence and must never be cited
// as any.** One encoder and one decoder, both ours. It establishes that
// the fixture is walkable and that the two cuts differ in the way
// `FEED-14` says they must; the evidence [ADR-0012] cares about arrives
// when their reader runs.

// corridorCut emits the fixture and returns a consumer pointed at one of
// its two cuts, served locally.
//
// **The server is started BEFORE the emit**, which looks backwards and is
// not. The emitted transport profile carries the origin the directory
// will be served from (§6.5.3 Amendment 5), so a consumer entering
// through it dials whatever was baked in at emit time — and the shipped
// fixture bakes `https://go-arm.example`, which is a real DNS lookup and
// a real attempt to leave this machine. **No suite in this repo may
// reach the public internet**: a suite whose result depends on a remote
// host is not measuring this tree and fails for someone else's reasons.
// Emitting against the test server's own URL is what keeps this local.
//
// The bytes below the transport profile are unaffected — the origin
// appears in the profile's three URL prefixes and nowhere in the trie —
// so this still reads the fixture's real emission.
func corridorCut(t *testing.T, cut string) *fetch.Consumer {
	t.Helper()
	out := t.TempDir()
	srv := httptest.NewServer(http.FileServer(http.Dir(filepath.Join(out, cut))))
	t.Cleanup(srv.Close)

	cmd := exec.Command("go", "run", "./cmd/crossimpl-feed", "-out", out, "-origin", srv.URL)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("emitting the corridor fixture: %v\n%s", err, b)
	}

	client := fetch.NewHTTPClient(30 * time.Second)
	layout, err := fetch.LoadLayout(context.Background(), srv.URL, client)
	if err != nil {
		t.Fatalf("LoadLayout for %s: %v", cut, err)
	}
	return fetch.NewConsumer(layout, client)
}

// TestFeedCorridor_FeedOnlyCutIsReadableAndUnattributed drives the narrow
// cut: 34 entries across two pages, every one honestly unattributed
// because `FEED-R2`'s signatures are outside what the root commits to.
func TestFeedCorridor_FeedOnlyCutIsReadableAndUnattributed(t *testing.T) {
	c := corridorCut(t, "feed-only")

	read, err := workbench.ReadFeed(context.Background(), c, workbench.FeedReadOpts{Limit: 100})
	if err != nil {
		t.Fatalf("ReadFeed: %v", err)
	}
	if len(read.Entries) != 34 {
		t.Fatalf("read %d entries, want 34 — the multi-page property is the whole reason for 34",
			len(read.Entries))
	}

	// Same assertion on the narrow cut. It passed before the fix and it
	// is kept, because it is the control that says `Listed` can be true
	// at all — an `assertAllListed` that never sees a true value would be
	// satisfied by a field nothing ever sets.
	assertAllListed(t, read, "feed-only")

	// The property no single-page fixture on either seat could falsify:
	// the entries span more than one index page.
	pages := map[uint64]bool{}
	for _, e := range read.Entries {
		pages[e.Page] = true
	}
	if len(pages) < 2 {
		t.Errorf("all 34 entries report one index page (%v) — then FEED-R12 is still unfalsifiable "+
			"and 34-against-32 bought nothing", pages)
	}

	for i, e := range read.Entries {
		if e.Attributed {
			t.Fatalf("entry %d is attributed on the feed-only cut — the signatures are not in the "+
				"committed set here, so attributing one means the reader found it by some route "+
				"other than the signed root", i)
		}
		if e.Attribution == "" {
			t.Errorf("entry %d is unattributed and says nothing about why; "+
				"\"unattributed\" alone reads as a defect in the reader", i)
		}
	}
}

// TestFeedCorridor_PeerRootCutIsReadable drives the wide cut, and it is
// the arm that found a defect.
//
// ⛔ **`ReadFeed` reconstructed §3.3a's keys as `prefix + key` verbatim,
// and the universal tree is the one prefix where that is wrong** — it is
// spelled `"/"` and every other prefix is not, so the reconstruction
// yielded `/app/feed/index` where §4.2 pins `app/feed/index`. Nothing
// matched.
//
// ⭐ **And the reader still returned all 34 entries, which is why nothing
// had caught it.** §4.3 rule 6 says an index is an optimization and its
// absence is a cost rather than an answer, so the reader fell back to
// enumeration exactly as `FEED-R13` requires and produced the same set —
// right answer, no error, nothing a caller would read as a defect.
// **Measured before the fix: 0 of 34 by index and 34 by fallback, where
// the narrow cut is 34 and 0.**
//
// So the transferable rule is not *"get the prefix join right"* — it is
// that **a conformance fallback built to survive a hostile publisher will
// equally survive your own broken primary path, and hide it.** Assert
// which path answered. `FeedEntryRead.Listed` is the field that says, and
// it existed the whole time.
//
// The defect was unreachable until this fixture existed, because every
// feed this tree had ever read was published over `app/feed/`. **A
// fixture that models one value of a parameter cannot fail on the
// others** — AP44's one-peer-per-origin and AP58's wrapper-less store,
// again.
func TestFeedCorridor_PeerRootCutIsReadable(t *testing.T) {
	c := corridorCut(t, "peer-root")

	read, err := workbench.ReadFeed(context.Background(), c, workbench.FeedReadOpts{Limit: 100})
	if err != nil {
		t.Fatalf("ReadFeed: %v", err)
	}
	if len(read.Entries) != 34 {
		t.Fatalf("read %d entries from the peer-root cut, want 34 — a root over the peer namespace "+
			"commits to the feed keys as surely as one over `app/feed/`; if this is 0, the reader "+
			"is reconstructing §3.3a's keys with a leading slash", len(read.Entries))
	}

	// ⭐ The assertion the entry count cannot make. Without it this test
	// passes against the defect it was written for.
	assertAllListed(t, read, "peer-root")

	// The reason this cut exists: `FEED-R2`'s signatures ARE committed
	// here, so a STATIC reader with no second channel attributes every
	// entry. Together with the feed-only arm this is `FEED-14`.
	attributed := 0
	for _, e := range read.Entries {
		if e.Attributed {
			attributed++
		}
	}
	if attributed != 34 {
		t.Errorf("%d/34 entries attributed on the peer-root cut. This is the half of FEED-14 that "+
			"says widening the publish scope actually buys attribution — if it is 0, either the "+
			"signatures are not committed (check the emitter's own 34/34 line) or the reader is "+
			"not looking for them in the committed set", attributed)
	}
}

// assertAllListed requires every entry to have been found through §4.2's
// index rather than through §4.3 rule 6's enumeration fallback.
//
// Both are conformant ways to read a feed and the fallback is REQUIRED to
// exist — that is `FEED-R13` and it has its own gate. What must not
// happen is falling back *silently on a feed whose index is committed*,
// because then the fallback is not a safety net, it is a mask over
// whatever broke the index path. The reader also records the reason in
// `Notes`, which is printed here so a failure says what it thought was
// wrong with the index rather than only that it gave up on it.
func assertAllListed(t *testing.T, read workbench.FeedRead, cut string) {
	t.Helper()
	listed := 0
	for _, e := range read.Entries {
		if e.Listed {
			listed++
		}
	}
	if listed != len(read.Entries) {
		t.Errorf("%s: %d of %d entries were found by ENUMERATION, not by the committed index. "+
			"Both are conformant and the answer may well be right — that is exactly the problem, "+
			"because a silent fallback on a feed whose index IS committed is hiding a defect in "+
			"the index path.\n  reader notes: %v",
			cut, len(read.Entries)-listed, len(read.Entries), read.Notes)
	}
}
