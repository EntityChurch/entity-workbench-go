package publish_test

// What a RETURNING reader pays to answer "what is new?" at feed scale.
//
// # The question this settles
//
// Two landed/proposed texts disagree. `APP-CONVENTION-FEED` §4.1 says
// "discovering what is new under a prefix costs the whole tree, because the
// trie is keyed by hash bits rather than by path locality", and concludes
// the index "is therefore a prerequisite of the reader loop rather than a
// later optimization". arch's replication proposal (D6) narrows that index
// to authored order, because under comparison-primary currency a walk
// obtains its delta from content addressing.
//
// Both cannot be right, and nobody had measured it — every trie in this
// tree is a 51-node site.
//
// # Measured 2026-09-11, and it settles it
//
//	entries   cold walk        no-op pass   1-entry delta
//	  1,000    56 requests      2 requests    5 requests
//	 10,000  1,057 requests     2 requests    6 requests
//	 50,000  3,345 requests     2 requests    7 requests
//
// The returning reader pays the ROOT-TO-LEAF PATH, not the tree: request
// count grows with log(n), so a 50× larger feed costs two more requests.
// The no-op pass is 2 requests at every scale — the manifest and its
// signature, i.e. exactly the fixed-key-mutable-pointer check.
//
// **The cost that IS O(n) is local traversal of already-cached nodes** —
// 63 ms over 3,343 nodes at 50k entries, ~19 µs/node — because a
// [fetch.WalkResult] is memoized per root hash and a moved root re-walks.
// Network is what a reader is charged for; the live federation runs ~90 ms
// per request, so the difference is 7 requests against 3,345.
//
// # Why this is a gate and not a note
//
// The whole result rests on the blob cache holding trie nodes by hash
// across a root move (AP48). Break that and the delta silently returns to
// the cold number while every other test in this tree stays green — a 51-
// node fixture cannot tell 5 requests from 56. The bounds below are
// generous on purpose: they catch a regression in KIND, not in degree.
//
// Run: go test ./publish -run TestFeedScaleProbe -v -count=1

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/fetch"
	"entity-workbench-go/publish"
)

type countingFS struct {
	h    http.Handler
	reqs atomic.Int64
}

func (c *countingFS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.reqs.Add(1)
	c.h.ServeHTTP(w, r)
}

func TestFeedScaleProbe(t *testing.T) {
	for _, n := range []int{1000, 10000, 50000} {
		t.Run(fmt.Sprintf("entries=%d", n), func(t *testing.T) {
			probeAtScale(t, n)
		})
	}
}

func probeAtScale(t *testing.T, n int) {
	ctx := context.Background()

	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	defer ap.Close()

	// A feed shape: flat, growing, write-once. One entry per key.
	seed := time.Now()
	for i := 0; i < n; i++ {
		p := fmt.Sprintf("feed/entry-%06d", i)
		if _, err := ap.Store().Put(p, "app/feed/entry", map[string]any{
			"author": "probe",
			"body":   fmt.Sprintf("entry number %d, long enough to be a realistic post body rather than a token", i),
			"at":     i,
		}); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
	t.Logf("seeded %d entries in %s", n, time.Since(seed).Round(time.Millisecond))

	dir := t.TempDir()
	counter := &countingFS{h: http.FileServer(http.Dir(dir))}
	srv := httptest.NewServer(counter)
	defer srv.Close()

	pubStart := time.Now()
	if _, err := publish.Publish(ctx, publish.Opts{
		Peer: ap, Prefix: "feed/", OutputDir: dir, OriginURL: srv.URL,
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	t.Logf("published in %s", time.Since(pubStart).Round(time.Millisecond))

	layout, err := fetch.LoadLayout(ctx, srv.URL, nil)
	if err != nil {
		t.Fatalf("LoadLayout: %v", err)
	}

	// A reader that keeps its cache across navigations -- the AP48 shape,
	// which is what ships today.
	cache := fetch.NewCache(512 << 20)
	c := fetch.NewConsumerWithCache(layout, fetch.NewHTTPClient(30*time.Second), cache)

	// ---- COLD: first read, nothing held.
	counter.reqs.Store(0)
	t0 := time.Now()
	rep, err := c.Consume(ctx, fetch.ConsumeOpts{})
	if err != nil {
		t.Fatalf("cold Consume: %v", err)
	}
	coldReqs := counter.reqs.Load()
	coldDur := time.Since(t0)
	t.Logf("COLD   : %6d keys · %5d nodes · %6d requests · %s",
		len(rep.Walk.Bindings), rep.Walk.Nodes(), coldReqs, coldDur.Round(time.Millisecond))

	// ---- NO-OP: root unmoved, warm cache. The "am I current?" pass.
	counter.reqs.Store(0)
	t1 := time.Now()
	if _, err := c.Consume(ctx, fetch.ConsumeOpts{}); err != nil {
		t.Fatalf("warm no-op Consume: %v", err)
	}
	noOpReqs := counter.reqs.Load()
	t.Logf("NO-OP  : %6s      %5s      %6d requests · %s",
		"", "", noOpReqs, time.Since(t1).Round(time.Millisecond))

	// ---- ONE NEW ENTRY: the publisher posts once; the root moves.
	if _, err := ap.Store().Put(fmt.Sprintf("feed/entry-%06d", n), "app/feed/entry", map[string]any{
		"author": "probe", "body": "the one new post", "at": n,
	}); err != nil {
		t.Fatalf("post: %v", err)
	}
	if _, err := publish.Publish(ctx, publish.Opts{
		Peer: ap, Prefix: "feed/", OutputDir: dir, OriginURL: srv.URL,
	}); err != nil {
		t.Fatalf("republish: %v", err)
	}

	counter.reqs.Store(0)
	t2 := time.Now()
	rep2, err := c.Consume(ctx, fetch.ConsumeOpts{})
	if err != nil {
		t.Fatalf("warm delta Consume: %v", err)
	}
	deltaReqs := counter.reqs.Load()
	deltaDur := time.Since(t2)
	t.Logf("DELTA+1: %6d keys · %5d nodes · %6d requests · %s",
		len(rep2.Walk.Bindings), rep2.Walk.Nodes(), deltaReqs, deltaDur.Round(time.Millisecond))

	t.Logf("VERDICT: cold %d reqs → returning-reader %d reqs (%.2f%% of cold) for a 1-entry delta over %d entries",
		coldReqs, deltaReqs, 100*float64(deltaReqs)/float64(coldReqs), n)

	// Anti-vacuity. Without this the bounds below are satisfied by a walk
	// that found nothing, which is the failure mode that would make this
	// test read as a pass while measuring an empty tree.
	if len(rep2.Walk.Bindings) != n+1 {
		t.Fatalf("walk committed %d keys, want %d — the measurement below is about nothing",
			len(rep2.Walk.Bindings), n+1)
	}
	if coldReqs < int64(n)/32 {
		t.Fatalf("cold walk took %d requests over %d entries — too few for this to be a real trie",
			coldReqs, n)
	}

	// The two properties the replication mechanism's affordability rests on.
	// Bounds are deliberately loose: a break here is a lost cache, which
	// moves the delta from single digits to thousands.
	if noOpReqs > 4 {
		t.Errorf("no-op currency check took %d requests, want <= 4 (manifest + signature); "+
			"a fixed-key pointer compare is not supposed to touch the trie", noOpReqs)
	}
	if deltaReqs > 32 {
		t.Errorf("a 1-entry delta over %d entries took %d requests, want <= 32 (root-to-leaf path); "+
			"the node cache is not surviving a root move", n, deltaReqs)
	}

	// CONTROL ARM. The bound above is only a measurement of the cache if a
	// reader WITHOUT one fails it. A reader that holds nothing walks the
	// same one-entry delta and must pay the whole trie — without this, the
	// bound is satisfiable by a walk that is cheap for some other reason,
	// and the finding routed from these numbers would be unearned.
	cold := fetch.NewConsumerWithCache(layout, fetch.NewHTTPClient(30*time.Second), fetch.NewCache(512<<20))
	counter.reqs.Store(0)
	if _, err := cold.Consume(ctx, fetch.ConsumeOpts{}); err != nil {
		t.Fatalf("control Consume: %v", err)
	}
	controlReqs := counter.reqs.Load()
	t.Logf("CONTROL: cacheless reader, same 1-entry delta · %6d requests", controlReqs)
	if controlReqs <= 32 {
		t.Errorf("a cacheless reader paid %d requests for the same delta — the bound above "+
			"is not measuring the cache", controlReqs)
	}
}
