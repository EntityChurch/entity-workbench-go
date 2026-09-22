package workbench_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"entity-workbench-go/workbench"
)

// browse_cache_test.go — the WIRING gate for AP48's cache, written
// because the mechanism was gated and the wiring was not.
//
// `fetch/cache_test.go::TestCache_ProvedBytesAreNotRefetched` proves the
// cache holds: five reads of one content hash cost one fetch. It says
// nothing about whether [workbench.BrowseModel] — the thing every
// shipped surface drives — actually reaches that cache across two
// navigations. A model that built a fresh consumer per navigation, or
// keyed its memo so that no second navigation ever hit it, would render
// every page correctly and pay the pre-AP48 cost forever, with that test
// green.
//
// That is AP100's rule ("two gates, because one cannot see the other's
// failure") and AP106's ("assert which path produced the answer"). The
// assertions below are phrased over the PATH, never over the page: a
// test phrased over the page passes against a browser with no cache at
// all, which is precisely how this went ungated.
//
// Tier: integration (real HTTP, real verification, another impl's bytes).

// countingFederation serves the frozen fixture and records what was
// asked for, so a navigation can be described by the requests it made
// rather than by how long it took.
type countingFederation struct {
	mu    sync.Mutex
	paths []string
	h     http.Handler
}

func (c *countingFederation) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	c.paths = append(c.paths, r.URL.Path)
	c.mu.Unlock()
	c.h.ServeHTTP(w, r)
}

// take returns the requests since the last take and resets the log.
func (c *countingFederation) take() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.paths
	c.paths = nil
	return out
}

// isFreshnessRead reports whether a path is a MUTABLE POINTER or the
// signature over one — the two things a re-visit must still fetch.
//
// This is AP48's distinction, and getting it backwards is the defect the
// cache was built to fix in one direction and a much worse one in the
// other: a content hash is an identity, so re-fetching proved bytes is
// pure waste; a published root is a pointer that MOVES, so caching one
// would turn "verified as of now" into "verified as of whenever we first
// looked", silently.
func isFreshnessRead(path string) bool {
	return strings.Contains(path, "/system/peer/published-root") ||
		strings.Contains(path, "/system/signature/")
}

// countingBrowserAt is browserAt with the origin instrumented.
func countingBrowserAt(t *testing.T) (*workbench.BrowseModel, *countingFederation) {
	t.Helper()
	cf := &countingFederation{h: http.FileServer(http.Dir(fedRoot))}
	srv := httptest.NewServer(cf)
	t.Cleanup(srv.Close)

	m := workbench.NewBrowseModel(nil)
	m.Now = func() time.Time { return time.UnixMilli(0) }
	ctx := context.Background()
	if err := m.PinRegistry(ctx, srv.URL, fedRegistryPeer, registryPin()); err != nil {
		t.Fatalf("PinRegistry: %v", err)
	}
	if err := m.RefreshNames(ctx); err != nil {
		t.Fatalf("RefreshNames: %v", err)
	}
	return m, cf
}

// TestBrowseRevisitRefetchesNoProvedByte is the wiring gate.
//
// `Back` deliberately re-runs the whole verification chain rather than
// replaying a stored page (browse_model.go's history comment: "a cached
// page is a claim about a moment"). So a re-visit is the strongest
// available measurement of whether the cache is reached: every content
// hash on that chain has already been proved once, and not one of them
// may be asked for again — while the mutable pointer at the top of it
// MUST be.
//
// Measured on the frozen fixture at the time of writing: opening `docs`
// cold costs 10 requests and 8 blob misses; going Back to it costs 3
// requests, 0 new misses and 7 hits. The three are the registry's root
// signature, the publisher's published-root and its signature — all
// three freshness reads, no content.
func TestBrowseRevisitRefetchesNoProvedByte(t *testing.T) {
	m, cf := countingBrowserAt(t)
	ctx := context.Background()

	cf.take()
	if err := m.Open(ctx, "docs.entitychurch.org"); err != nil {
		t.Fatalf("open docs: %v\nsteps: %s", err, stepDump(m))
	}
	cold := m.CacheStats()
	coldReqs := cf.take()

	// Anti-vacuity, first half: the cache started cold and its counters
	// move at all. Without this the whole test is satisfied by a model
	// that fetches nothing because it is doing nothing.
	if cold.Hits != 0 {
		t.Errorf("the first navigation reported %d cache hits — the fixture is not cold, so "+
			"'no new misses on the re-visit' would prove nothing", cold.Hits)
	}
	if cold.Misses == 0 {
		t.Fatalf("the first navigation missed the cache 0 times over %d requests — either the "+
			"counters are dead or nothing went through Consumer.Blob, and every assertion "+
			"below is then vacuous", len(coldReqs))
	}

	if err := m.Open(ctx, "lab.entitychurch.org"); err != nil {
		t.Fatalf("open lab: %v", err)
	}
	warm := m.CacheStats()
	cf.take()

	// 1. The model's ONE cache is reached by the reader it built for
	//    EACH publisher. Two publishers have been read, so two traversals
	//    are held here; a model whose per-publisher consumers each kept
	//    their own cache holds only the registry's.
	//
	//    Measured 2026-09-15 with the cache unshared in `consumerFor`
	//    (`NewConsumerWithCache(…, m.cache)` → `NewConsumer(…)`): 1 walk
	//    instead of 2, and the whole `workbench` and `publish` suites
	//    still green. This assertion and assertion 3 are the two that
	//    move under that mutation.
	if warm.Walks < 2 {
		t.Errorf("after reading two publishers the model's cache holds %d walk(s): the consumers "+
			"it builds per publisher are not reaching it, so each publisher's trie is being "+
			"traversed into a cache that dies with the read", warm.Walks)
	}

	// ---- the re-visit -------------------------------------------------
	if err := m.Back(ctx); err != nil {
		t.Fatalf("back to docs: %v", err)
	}
	revisit := m.CacheStats()
	reqs := cf.take()

	// 2. Not one proved byte was asked for again.
	if revisit.Misses != warm.Misses {
		t.Errorf("the re-visit took %d NEW cache misses (%d -> %d): a page whose every content "+
			"hash was already proved is being re-fetched, which is the pre-AP48 cost with the "+
			"cache sitting right there. Requests made: %v",
			revisit.Misses-warm.Misses, warm.Misses, revisit.Misses, reqs)
	}

	// 3. ...and the cached TRAVERSAL is what answered. This is the
	//    "which path produced it" half (AP106), and it is phrased over
	//    walk hits rather than blob hits deliberately: the cache is
	//    shared with the registry reader, so blob hits alone are
	//    satisfied by registry traffic while every site read goes
	//    uncached — which is exactly what the mutation above produces.
	//    The walk is also the entry the cache's own doc comment calls
	//    "the one whose absence costs 51 round-trips".
	if revisit.WalkHits <= warm.WalkHits {
		t.Errorf("the re-visit served no walk from the cache (%d -> %d): the page's trie was "+
			"traversed again from the origin, so assertion 2 passed on bytes that were never "+
			"asked for rather than on bytes that were already held",
			warm.WalkHits, revisit.WalkHits)
	}

	// 4. Every request it DID make is a freshness read, named. This is
	//    what stops assertion 2 from being satisfiable by skipping the
	//    verification chain: a re-visit still re-establishes the root,
	//    and the honest cost of that is exactly these.
	for _, p := range reqs {
		if !isFreshnessRead(p) {
			t.Errorf("the re-visit fetched %q, which is neither a published root nor a signature "+
				"over one — a content-addressed byte is an identity and the second read of one "+
				"is free", p)
		}
	}
	if len(reqs) == 0 {
		t.Error("the re-visit made no requests at all: the published root is a MUTABLE pointer, " +
			"so a navigation that re-fetches none of it is not re-verifying freshness, it is " +
			"replaying a claim about an earlier moment")
	}

	// 5. The answer is still right. Last on purpose: this is the ONLY
	//    assertion here that a browser with a dead cache also passes,
	//    which is the entire lesson of AP106.
	if got := m.Render().Address; !strings.Contains(got, "docs.entitychurch.org") {
		t.Errorf("back landed on %q, want the docs page", got)
	}
}
