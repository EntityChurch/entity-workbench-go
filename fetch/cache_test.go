package fetch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	cbor "github.com/fxamacker/cbor/v2"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"
)

// cache_test.go — Tier 2 (component). The cache's two claims, each with
// its own failure direction:
//
//  1. A body already proved is not re-fetched. This is the win.
//  2. A body is only ever served from the cache under the hash it
//     verified against, and a walk that did NOT complete is never
//     memoized. These are the ways a cache turns into a lie.

// blobServer serves a content store from an in-memory map, counting the
// requests for each hash so "was it fetched again" is a number.
type blobServer struct {
	t     *testing.T
	blobs map[string][]byte
	hits  map[string]*int64
	srv   *httptest.Server
	gone  map[string]bool
}

func newBlobServer(t *testing.T) *blobServer {
	b := &blobServer{t: t, blobs: map[string][]byte{}, hits: map[string]*int64{}, gone: map[string]bool{}}
	b.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Path
		if c, ok := b.hits[key]; ok {
			atomic.AddInt64(c, 1)
		} else {
			var n int64 = 1
			b.hits[key] = &n
		}
		if b.gone[key] {
			http.NotFound(w, r)
			return
		}
		body, ok := b.blobs[key]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(b.srv.Close)
	return b
}

func (b *blobServer) layout() Layout {
	return Layout{
		Origin: b.srv.URL,
		PeerID: "2KFNrGARQBkx3d9WQtkeuT3HbQ3oXSS7PBggWt1szT9hzb",
		Endpoint: types.TransportEndpoint{
			ContentURLPrefix: "/content",
			ContentLayout:    types.ContentLayoutFlat,
			TreeURLPrefix:    "/tree",
			TreeLeafSuffix:   ".bin",
		},
	}
}

// put stores an entity and returns the hash the content store files it
// under — the same computation decodeVerified re-does on the way back.
func (b *blobServer) put(t *testing.T, typ string, data cbor.RawMessage) hash.Hash {
	t.Helper()
	h, err := hash.Compute(typ, data)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := ecf.Encode(entity.Entity{Type: typ, Data: data})
	if err != nil {
		t.Fatal(err)
	}
	b.blobs["/content/"+hexOf(h)] = raw
	return h
}

func (b *blobServer) count(h hash.Hash) int64 {
	if c, ok := b.hits["/content/"+hexOf(h)]; ok {
		return atomic.LoadInt64(c)
	}
	return 0
}

// enc CBOR-encodes a value into the `data` position of an entity. Every
// entity body is CBOR; hash.Compute re-parses it, so a raw Go []byte is
// not a body, it is a parse error.
func enc(t *testing.T, v any) cbor.RawMessage {
	t.Helper()
	b, err := ecf.Encode(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func hexOf(h hash.Hash) string {
	return fmt.Sprintf("%x", h.Bytes())
}

// TestCache_ProvedBytesAreNotRefetched is the win, stated as a count.
//
// Before the cache, `BrowseModel` re-fetched the same 51 CHAMP nodes on
// every navigation against the live federation (measured 2026-08-31: 61
// requests to open a page, 60 to click a link in it). The property is
// not "it is faster" — it is that a content hash is an identity, so the
// second read of one is free.
func TestCache_ProvedBytesAreNotRefetched(t *testing.T) {
	b := newBlobServer(t)
	h := b.put(t, "test/doc", enc(t, "hello"))
	c := NewConsumer(b.layout(), b.srv.Client())
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		ent, err := c.Blob(ctx, h)
		if err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
		var got string
		if err := ecf.Decode(ent.Data, &got); err != nil {
			t.Fatal(err)
		}
		if got != "hello" {
			t.Fatalf("read %d returned %q", i, got)
		}
	}
	if got := b.count(h); got != 1 {
		t.Fatalf("five reads of one content hash cost %d fetches, want 1 — the cache is not "+
			"holding, and every navigation pays for the last one again", got)
	}
	if s := c.Cache.Stats(); s.Hits != 4 || s.Misses != 1 {
		t.Fatalf("stats say %d hits / %d misses, want 4/1", s.Hits, s.Misses)
	}
}

// TestCache_NeverServesUnverifiedBytes pins the door.
//
// The cache is filled in exactly one place — [Consumer.Blob], on the far
// side of decodeVerified — so a body the origin corrupted must never
// reach it. If it did, the first reader would get an error and every
// later reader would get the corrupt body reported as verified, which is
// strictly worse than having no cache at all.
func TestCache_NeverServesUnverifiedBytes(t *testing.T) {
	b := newBlobServer(t)
	h := b.put(t, "test/doc", enc(t, "hello"))
	// Replace the body with different bytes at the same URL — a lying
	// origin, or a corrupt CDN edge.
	bad, err := ecf.Encode(entity.Entity{Type: "test/doc", Data: enc(t, "goodbye")})
	if err != nil {
		t.Fatal(err)
	}
	b.blobs["/content/"+hexOf(h)] = bad

	c := NewConsumer(b.layout(), b.srv.Client())
	ctx := context.Background()
	if _, err := c.Blob(ctx, h); err == nil {
		t.Fatal("a body that is not its own pre-image was accepted")
	}
	if s := c.Cache.Stats(); s.Blobs != 0 {
		t.Fatalf("a refused body left %d entries in the cache — every later reader would be "+
			"served it as verified", s.Blobs)
	}
	// And again, to prove the refusal is not itself memoized into a
	// success.
	if _, err := c.Blob(ctx, h); err == nil {
		t.Fatal("second read of a corrupt body succeeded")
	}
}

// TestCache_IncompleteWalkIsNotMemoized is the failure direction that
// would be invisible.
//
// A walk that fails closed is the whole reason this package does not use
// the kernel's best-effort helper. If a partial traversal were cached,
// one transient 404 during a walk would pin a SHORTER key set for the
// rest of the session — manufacturing, locally and permanently, the
// withholding origin the walk exists to detect.
func TestCache_IncompleteWalkIsNotMemoized(t *testing.T) {
	b := newBlobServer(t)

	// A two-level trie: root links to a child, child holds one binding.
	leafHash := b.put(t, "test/page", enc(t, "body"))
	childData, err := ecf.Encode(types.SnapshotNodeData{
		Map: []byte{0, 0, 0, 1},
		Data: []types.NodeEntry{{Bucket: []types.BucketTuple{
			{Key: "docs/a", ValueHash: leafHash},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	childHash := b.put(t, types.TypeTreeSnapshotNode, childData)
	rootData, err := ecf.Encode(types.SnapshotNodeData{
		Map:  []byte{0, 0, 0, 1},
		Data: []types.NodeEntry{{Link: &childHash}},
	})
	if err != nil {
		t.Fatal(err)
	}
	rootHash := b.put(t, types.TypeTreeSnapshotNode, rootData)

	c := NewConsumer(b.layout(), b.srv.Client())
	ctx := context.Background()

	// Withhold the child before the first walk ever runs.
	b.gone["/content/"+hexOf(childHash)] = true
	if _, err := c.Walk(ctx, rootHash); !errors.Is(err, ErrIncompleteWalk) {
		t.Fatalf("withheld interior node walked as %v, want ErrIncompleteWalk", err)
	}
	if s := c.Cache.Stats(); s.Walks != 0 {
		t.Fatalf("a failed walk was memoized (%d entries) — the shortened key set would now "+
			"outlive the condition that produced it", s.Walks)
	}

	// The origin recovers. A cache that had memoized the failure would
	// still be answering with the short set.
	b.gone["/content/"+hexOf(childHash)] = false
	w, err := c.Walk(ctx, rootHash)
	if err != nil {
		t.Fatalf("walk after recovery: %v", err)
	}
	if len(w.Bindings) != 1 || w.Bindings[0].Key != "docs/a" {
		t.Fatalf("recovered walk returned %v, want the one committed key", w.Keys())
	}
	if s := c.Cache.Stats(); s.Walks != 1 {
		t.Fatalf("a completed walk was not memoized (%d entries)", s.Walks)
	}
}

// TestConsumer_SeqFloorIsPerPublisherNotPerRoad is the security property
// a per-ROAD floor does not have.
//
// # What used to be here, and why it is gone
//
// A test of the same name re-implemented the floor comparison inside its
// own body, and said so: when it was written the comparison was six
// lines inline in `VerifiedRoot` and there was nothing else to call.
// `TestAcceptSeqIsTheRealFloor` (source_test.go) was added later to run
// the real one over the same four cases, and the reimplementation was
// deliberately left alone as the net for that refactor.
//
// It is retired now because **its stated reason expired**: the floor is
// [SeqFloor], an ordinary callable, so a copy of the rule sitting in a
// test can only fail to notice when the rule changes. What replaces it
// is the case nothing covered — two consumers, one publisher.
//
// # The property
//
// A publisher reachable both live and statically is two [Source]s and
// therefore two [Consumer]s. If each keeps its own floor, a reader that
// verified seq=6 on one road and then falls back to the other starts
// from **no memory**, and a correctly-signed seq=3 replayed by a static
// origin is accepted in silence. The fallback road is the one a chooser
// reaches for when the preferred one fails, and a third party serving a
// stale root is exactly what the static mode exists to confess — so the
// split points the wrong way in the case that matters.
func TestConsumer_SeqFloorIsPerPublisherNotPerRoad(t *testing.T) {
	live := NewConsumerFromSource(NewHTTPSource(Layout{PeerID: srcTestPeer}, nil), nil)
	static := NewConsumerFromSource(NewHTTPSource(Layout{PeerID: srcTestPeer}, nil), nil)
	static.ShareFloorWith(live)

	if err := live.acceptSeq(6, "entity://publisher/system/peer/published-root"); err != nil {
		t.Fatalf("first root refused: %v", err)
	}
	if err := static.acceptSeq(6, "https://origin.test/manifest"); err != nil {
		t.Fatalf("the SAME seq on the other road was refused: %v — one publisher republishing "+
			"one root is not a rollback, whichever road carried it", err)
	}
	err := static.acceptSeq(3, "https://origin.test/manifest")
	if !errors.Is(err, ErrSeqRollback) {
		t.Fatalf("seq 3 on the static road was accepted (%v) after seq 6 on the live road — "+
			"a floor that resets when the road changes is not a floor", err)
	}

	// Anti-vacuity: the same two consumers WITHOUT the shared floor must
	// take the rollback. Without this arm the assertion above passes
	// against any build where `ShareFloorWith` does nothing at all, which
	// is the one way this fix can silently not be applied.
	loneLive := NewConsumerFromSource(NewHTTPSource(Layout{PeerID: srcTestPeer}, nil), nil)
	loneStatic := NewConsumerFromSource(NewHTTPSource(Layout{PeerID: srcTestPeer}, nil), nil)
	if err := loneLive.acceptSeq(6, "entity://publisher/system/peer/published-root"); err != nil {
		t.Fatalf("control arm: first root refused: %v", err)
	}
	if err := loneStatic.acceptSeq(3, "https://origin.test/manifest"); err != nil {
		t.Fatalf("control arm: an unshared floor refused seq 3 (%v) — the two consumers are "+
			"sharing state they were not given, so the positive arm above proves nothing", err)
	}
}

// TestShareFloorWithRefusesToDiscardAMemory pins the one way this API can
// be used to open the window it closes.
//
// Installing a floor is a construction-time act. Doing it after a
// consumer has accepted something replaces the memory with an empty one,
// so the call that was supposed to close a rollback window opens one.
// The guard is that the OTHER consumer's floor is what gets installed —
// so a caller who wires it late still ends up sharing rather than
// resetting, and a nil argument is ignored outright.
func TestShareFloorWithRefusesToDiscardAMemory(t *testing.T) {
	a := NewConsumerFromSource(NewHTTPSource(Layout{PeerID: srcTestPeer}, nil), nil)
	if err := a.acceptSeq(9, "entity://publisher/root"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	a.ShareFloorWith(nil)
	a.UseFloor(nil)
	if err := a.acceptSeq(4, "entity://publisher/root"); !errors.Is(err, ErrSeqRollback) {
		t.Fatalf("a nil floor was installed and wiped the memory (%v)", err)
	}
	if seq, known := a.Floor().Seq(); !known || seq != 9 {
		t.Fatalf("floor reports (%d, %v), want (9, true)", seq, known)
	}
}
