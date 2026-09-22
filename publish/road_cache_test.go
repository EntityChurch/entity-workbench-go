package publish_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"entity-workbench-go/fetch"
	"entity-workbench-go/workbench"
)

// road_cache_test.go — the two roads to one publisher share proved
// bytes, and until this file nothing said so.
//
// # Why this needed its own gate
//
// AP100 moved the `seq` floor off [fetch.Consumer] because the chooser
// makes **two consumers for one publisher deliberate** — a publisher can
// be read live and statically, and a check whose memory is keyed by the
// object performing it is not one check. The cache is handed to both
// consumers for the same structural reason, one field over.
//
// Measured 2026-09-15, before this file existed: removing the shared
// cache from `BrowseModel.consumerFor` — `NewConsumerWithCache(layout,
// client, m.cache)` → `NewConsumer(layout, client)` — left **both the
// `workbench` and `publish` suites entirely green.** A reader that
// verified a page live and then fell back to the static road would
// re-fetch every byte it had just proved, with nothing to catch it.
//
// # Why the assertion is phrased over requests and not over the page
//
// Because the page is right either way. That is AP106: a second road
// that pays full price still produces the correct answer, so every
// assertion over the ANSWER passes against the defect. The only
// observable difference is what the origin was asked for — so that is
// what is asserted, and the control arm below exists because an
// assertion that "no content was fetched" is also satisfied by a road
// that fetched nothing at all.

// countingTransport records the paths a consumer asks an origin for.
type countingTransport struct {
	mu    sync.Mutex
	paths []string
	next  http.RoundTripper
}

func (c *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c.mu.Lock()
	c.paths = append(c.paths, r.URL.Path)
	c.mu.Unlock()
	next := c.next
	if next == nil {
		next = http.DefaultTransport
	}
	return next.RoundTrip(r)
}

func (c *countingTransport) take() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.paths
	c.paths = nil
	return out
}

func countingClient() (*http.Client, *countingTransport) {
	ct := &countingTransport{}
	return &http.Client{Transport: ct}, ct
}

// contentRequests returns the subset of paths that asked for
// content-addressed bytes rather than for the mutable pointer at the top
// of the chain.
//
// A published root MOVES and its signature moves with it, so a second
// road MUST re-read both — caching those would turn "verified as of now"
// into "verified as of whenever we first looked". Everything else on the
// chain is addressed by a hash of its own bytes, so the second read of
// one is free and paying for it is pure waste.
//
// **The manifest path is taken from the layout, not spelled here.** The
// static corridor serves the published root at `{out}/manifest` while
// the tree-path form is `system/peer/published-root`; a hard-coded
// classifier that knows only the second reports the first as a content
// re-fetch, which is a fixture-shaped false positive against a
// publisher behaving correctly.
func contentRequests(manifestPath string, paths []string) []string {
	var out []string
	for _, p := range paths {
		if p == manifestPath || strings.Contains(p, "/system/signature/") {
			continue
		}
		out = append(out, p)
	}
	return out
}

// manifestPathOf is the layout's manifest URL reduced to the path the
// origin will see.
func manifestPathOf(t *testing.T, l fetch.Layout) string {
	t.Helper()
	raw := l.ManifestURL()
	if raw == "" {
		t.Fatal("the layout advertises no manifest URL, so no read on this road can be classified")
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse manifest URL %q: %v", raw, err)
	}
	return u.Path
}

// TestBothRoadsToOnePublisherShareProvedBytes drives one published act
// down the live road and then down the static road, and requires the
// second road to pay for the freshness re-read and for nothing else.
func TestBothRoadsToOnePublisherShareProvedBytes(t *testing.T) {
	pair := newLivePair(t, readSiteGrants())
	ctx := context.Background()

	layout, err := fetch.LoadLayout(ctx, pair.originURL, nil)
	if err != nil {
		t.Fatalf("LoadLayout: %v", err)
	}
	manifestPath := manifestPathOf(t, layout)

	// ---- arm 1: one cache across both roads (the shipped wiring) ----
	shared := fetch.NewCache(0)
	live, err := workbench.NewPeerConsumer(pair.reader, pair.publisher.PeerID(), shared)
	if err != nil {
		t.Fatalf("NewPeerConsumer: %v", err)
	}
	liveChain := run(t, ctx, "live", live)

	client, ct := countingClient()
	staticChain := run(t, ctx, "static/shared", fetch.NewConsumerWithCache(layout, client, shared))
	sharedContent := contentRequests(manifestPath, ct.take())

	// ---- arm 2: the control — a cache per road ----
	//
	// Not optional. Without it, "the static leg fetched no content"
	// is equally satisfied by a static leg that did no work, by a
	// classifier that matches everything, and by a fixture with no
	// interior to walk.
	ownLive, err := workbench.NewPeerConsumer(pair.reader, pair.publisher.PeerID(), fetch.NewCache(0))
	if err != nil {
		t.Fatalf("NewPeerConsumer (control): %v", err)
	}
	run(t, ctx, "live/own", ownLive)

	client2, ct2 := countingClient()
	controlChain := run(t, ctx, "static/own", fetch.NewConsumerWithCache(layout, client2, fetch.NewCache(0)))
	controlContent := contentRequests(manifestPath, ct2.take())

	// ---- the control arm establishes that the measurement can move ----
	if len(controlContent) == 0 {
		t.Fatalf("the control arm — separate caches — fetched no content either, so this test " +
			"cannot tell a shared cache from an unshared one and proves nothing")
	}

	// ---- the property ----
	if len(sharedContent) != 0 {
		t.Errorf("the static road re-fetched %d content-addressed object(s) the live road had "+
			"already proved, out of the %d a cold road pays for: %v\n"+
			"Two consumers for one publisher is deliberate (the chooser), and they share a cache "+
			"so that the fallback road is not the full price again.",
			len(sharedContent), len(controlContent), sharedContent)
	}

	// ---- and the answer is the same on both arms (last, on purpose:
	//      this is the assertion the defect also passes) ----
	if liveChain.pageBody != staticChain.pageBody {
		t.Errorf("shared-cache arm: live and static served different bodies\n  live:   %q\n  static: %q",
			liveChain.pageBody, staticChain.pageBody)
	}
	if staticChain.pageBody != controlChain.pageBody {
		t.Errorf("the cache changed the ANSWER, which is the one thing it may never do\n"+
			"  shared: %q\n  own:    %q", staticChain.pageBody, controlChain.pageBody)
	}
	if staticChain.pageBody == "" {
		t.Fatal("anti-vacuity: the resolved body is empty, so the equalities above say nothing")
	}
	if staticChain.trieRoot != controlChain.trieRoot {
		t.Errorf("root_hash differs between the cached and uncached static reads: %s vs %s",
			staticChain.trieRoot, controlChain.trieRoot)
	}
}
