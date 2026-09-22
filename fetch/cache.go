package fetch

import (
	"container/list"
	"sync"

	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
)

// cache.go — the thing this package spent its first year refusing to
// build, for a reason that turns out to apply to exactly one object.
//
// # The sentence that cost 75% of every click
//
// [Consumer] carried this, and it was right about the manifest and wrong
// about everything the manifest points at:
//
//	"It holds no state between calls beyond the HTTP client: every
//	 verification starts from the manifest, because a cached root is a
//	 freshness claim nobody made."
//
// A published-root IS a mutable pointer — it is the one object at a
// stable URL whose bytes change under you, and re-fetching it on every
// navigation is correct and stays. **Everything below it is
// content-addressed.** `{content}/00/8d/008deffb…` is a URL whose path
// *is* the SHA-256 of the body it returns; the body cannot change while
// the URL stays the same, and [decodeVerified] recomputes the hash and
// refuses a mismatch before the bytes leave this package. Holding that
// body in memory under its own hash is not a freshness claim. It is the
// identity the substrate is built on, and refusing to use it means
// re-downloading and re-hashing bytes we have already proved.
//
// **Measured against the live federation, 2026-08-31**, browsing
// billslab.com through `BrowseModel`:
//
//	open billslab.com   7.3 s   61 requests
//	click support.md    6.2 s   60 requests   ← same 51 CHAMP nodes again
//	Back                5.9 s   60 requests   ← and again
//
// 51 of those 60 are the trie walk, sequential, ~90 ms each. The same 51
// URLs every time. The publisher's own transport profile even declares
// `freshness: "static-immutable+signed-pointer"` — the origin was
// telling us, in a field we parse into [Layout.Freshness] and never
// read.
//
// # Why the WALK is cacheable too, which is the bigger half
//
// A [WalkResult] is a pure function of the root hash it was walked from:
// every edge is a content hash, so a trie rooted at H has exactly one
// key set, forever. Caching it under H is not caching an *answer about
// the world*, it is memoizing a traversal of an immutable structure.
// The freshness question is answered one level up, where it belongs —
// we re-fetch the manifest, and if `root_hash` moved we walk again.
//
// So the cache is bounded by what it is allowed to hold, never by time:
// there is no TTL here and there must not be one, because a TTL would
// imply these bytes could go stale, and a content hash is the one thing
// in this system that cannot.
//
// # The security property this BUYS (not the one it trades away)
//
// `entity-browser-rust`'s `SignedSession` names it, and we did not have
// it: a client that is rebuilt per navigation enforces root monotonicity
// only within its own lifetime, so **an origin can serve `seq 5` for one
// page and `seq 3` for the next — a rollback, page by page — and nothing
// notices.** Ours was rebuilt in `goTo` on every single navigation, so
// our seq floor was one navigation deep, i.e. absent. Reusing a
// [Consumer] across a session is what makes [Consumer.minSeq] mean
// anything. Re-verifying from scratch each click was not the paranoid
// option; it was strictly weaker and much slower.

// DefaultCacheBytes bounds the body cache.
//
// It is a byte budget rather than an entry count because the corpus is
// wildly non-uniform: billslab commits 966 keys of which 665 are figure
// blobs (tens to hundreds of KB each) and one HTML page is 8.27 MB. An
// entry-count bound sized for the trie would be blown by one paper; one
// sized for the papers would hold the whole figure set in memory.
const DefaultCacheBytes = 64 << 20

// Cache holds content-addressed bodies and the traversals derived from
// them. Safe for concurrent use.
//
// **Every entry is keyed by a hash the entry's own bytes produce.** That
// is the whole invariant, and it is what makes eviction free of
// correctness consequences: dropping an entry costs a re-fetch and can
// never yield a different answer.
type Cache struct {
	mu sync.Mutex

	maxBytes int64
	bytes    int64

	blobs map[hash.Hash]*list.Element
	lru   *list.List // front = most recently used

	// walks is keyed by root hash. Not byte-bounded: a WalkResult is
	// key strings plus hashes, small next to the bodies, and it is the
	// entry whose absence costs 51 round-trips.
	walks map[hash.Hash]WalkResult

	hits, misses, walkHits, walkMisses int64
}

type cacheItem struct {
	h   hash.Hash
	ent entity.Entity
	sz  int64
}

// NewCache builds a cache with the given byte budget; <= 0 means
// [DefaultCacheBytes].
func NewCache(maxBytes int64) *Cache {
	if maxBytes <= 0 {
		maxBytes = DefaultCacheBytes
	}
	return &Cache{
		maxBytes: maxBytes,
		blobs:    map[hash.Hash]*list.Element{},
		lru:      list.New(),
		walks:    map[hash.Hash]WalkResult{},
	}
}

// Blob returns a cached body, if held.
func (c *Cache) Blob(h hash.Hash) (entity.Entity, bool) {
	if c == nil {
		return entity.Entity{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.blobs[h]
	if !ok {
		c.misses++
		return entity.Entity{}, false
	}
	c.hits++
	c.lru.MoveToFront(el)
	return el.Value.(*cacheItem).ent, true
}

// PutBlob records a body that has already been hash-verified against h.
//
// The caller's obligation is exactly that — this is the door
// [Consumer.Blob] holds open after [decodeVerified] has run, and nothing
// else may call it. A body admitted here without that check would be
// served to every later reader as verified.
func (c *Cache) PutBlob(h hash.Hash, ent entity.Entity) {
	if c == nil {
		return
	}
	sz := int64(len(ent.Data) + len(ent.Type) + 64)
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.blobs[h]; ok {
		c.lru.MoveToFront(el)
		return
	}
	if sz > c.maxBytes {
		// One body larger than the whole budget. Serve it, do not hold
		// it — caching it would evict everything else to store a thing
		// we would then evict on the next insert anyway.
		return
	}
	el := c.lru.PushFront(&cacheItem{h: h, ent: ent, sz: sz})
	c.blobs[h] = el
	c.bytes += sz
	for c.bytes > c.maxBytes {
		back := c.lru.Back()
		if back == nil {
			break
		}
		it := back.Value.(*cacheItem)
		c.lru.Remove(back)
		delete(c.blobs, it.h)
		c.bytes -= it.sz
	}
}

// Walk returns a cached traversal of the trie rooted at h.
func (c *Cache) Walk(h hash.Hash) (WalkResult, bool) {
	if c == nil {
		return WalkResult{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	w, ok := c.walks[h]
	if ok {
		c.walkHits++
	} else {
		c.walkMisses++
	}
	return w, ok
}

// PutWalk records a COMPLETED traversal.
//
// Only a walk that ran to completion may be recorded. A partial one is
// the exact artifact [Consumer.Walk] fails closed to avoid producing,
// and caching one would turn a transient network failure into a
// permanently short key set — a withholding origin, manufactured
// locally, outliving the condition that caused it.
func (c *Cache) PutWalk(h hash.Hash, w WalkResult) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.walks[h] = w
}

// CacheStats is what the cache did, for a surface that wants to say why
// a navigation was fast.
type CacheStats struct {
	Blobs      int
	Bytes      int64
	MaxBytes   int64
	Hits       int64
	Misses     int64
	Walks      int
	WalkHits   int64
	WalkMisses int64
}

// Stats snapshots the counters.
func (c *Cache) Stats() CacheStats {
	if c == nil {
		return CacheStats{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return CacheStats{
		Blobs: len(c.blobs), Bytes: c.bytes, MaxBytes: c.maxBytes,
		Hits: c.hits, Misses: c.misses,
		Walks: len(c.walks), WalkHits: c.walkHits, WalkMisses: c.walkMisses,
	}
}
