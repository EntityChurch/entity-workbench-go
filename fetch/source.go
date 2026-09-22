package fetch

import (
	"context"
	"net/http"

	"go.entitychurch.org/entity-core-go/core/hash"
)

// source.go — the seam between *where a consumer's bytes come from* and
// *what makes them trustworthy*.
//
// # Why there is a seam at all
//
// [Consumer] bundled three things and only one of them was about HTTP:
//
//	byte source    Layout + *http.Client + httpGet      HTTP-specific
//	verification   decodeVerified, the two-hop root signature,
//	               the seq floor, the CHAMP walk         not
//	caching        Cache, keyed by content hash          not
//
// So the seam goes under the byte source and nowhere else. Everything
// above it — every check that decides whether bytes are admissible — has
// exactly one implementation and keeps it. **That is the point rather
// than a tidiness argument.** The alternative is two code paths for one
// trust argument, and the weaker one would wear the same UI.
//
// # The trust property a second Source must not quietly weaken
//
// **An authenticated connection proves WHO, not WHAT.** A handshake
// establishes that the far end holds the key its peer-id names; it says
// nothing about whether the bytes it hands back are bytes it ever
// committed to. A peer can serve whatever it likes over its own
// connection. So a Source is a *byte source only*: it is never asked
// whether bytes are good, it cannot report that they are, and there is
// no field on any of these methods for it to say so. The hash check, the
// signature, the seq floor and the walk run above it, identically,
// whatever answered.
//
// # Three primitives, and the direction doc said two
//
// `LIVE-PEER-DIRECTION.md` §4.1 sketched this interface with two methods
// — content-addressed bytes, and the invariant-pointer leaves that live
// outside the trie — on the reading that those are the two resolution
// paths V7 §5.2 forces a consumer to hold. That is true of the *tree*,
// and it is one short, because the **published-root manifest is reached
// a third way**: over HTTP it is served as an entity at
// `manifest_url_prefix`, which is neither a content hash nor a tree-leaf
// pointer. Building the seam is what surfaced it; the doc is corrected
// rather than the count being quietly rounded up here.
//
// # Every method returns a LOCATOR, and it is not decoration
//
// The chain an operator reads names *where each step looked*, and a
// verification layer that cannot see a URL cannot name one. So each
// primitive returns the transport's own name for the thing it fetched —
// a URL here, an `entity://` address for a peer — and the layer above
// puts it in [VerifiedRoot] and in its errors without knowing what kind
// of string it is.

// Source is where a [Consumer]'s bytes come from.
//
// A Source answers *"give me these bytes"* and nothing else. It performs
// no verification, and none of its methods can express a verdict; see
// the file note for why that asymmetry is load-bearing rather than a
// simplification.
//
// Implementations must be safe for concurrent use: [Consumer.Walk]
// fetches a whole trie level at once.
type Source interface {
	// PeerID is the publisher this source reads.
	//
	// **It is the key the signature verifies against**, not a routing
	// detail — which is why it is on the seam rather than left inside the
	// transport. A Source that cannot name its publisher cannot be
	// verified against anything.
	PeerID() string

	// Root returns the raw published-root body — the MUTABLE pointer,
	// re-read on every navigation because it is the only thing under a
	// publisher that can change.
	//
	// It returns bytes, not a decoded root: the type check, the peer-id
	// check, the §3.3a prefix check and the two-hop signature all live
	// above the seam.
	Root(ctx context.Context) (raw []byte, locator string, err error)

	// Leaf returns the raw body at a peer-relative tree path — the §5.2
	// invariant-pointer leaf itself, NOT the entity it names.
	//
	// The dereference is the caller's, because cracking the pointer is a
	// conformance check (a one-hop publisher's leaf decodes perfectly
	// well as *some* entity) and conformance checks live above the seam.
	Leaf(ctx context.Context, treePath string) (raw []byte, locator string, err error)

	// Blob returns the raw content-store body for h.
	//
	// **It must not check the hash**, however easy that would be here.
	// [Consumer.Blob] is the only door into the content store and the
	// only place [Cache] is filled, both on the far side of
	// [decodeVerified]; a Source that verified too would make the real
	// check look optional to the next implementer.
	Blob(ctx context.Context, h hash.Hash) (raw []byte, locator string, err error)
}

// HTTPSource reads a publisher's bytes over HTTP, out of the layout that
// publisher advertised.
//
// This is the byte source this package has always had, moved behind the
// interface with no change to what it fetches or in what order. It is
// the only implementation as of this commit — a seam with one side is a
// refactor, and it is justified by what goes on the other side rather
// than by itself.
type HTTPSource struct {
	// Layout is the publisher's advertised endpoint. Every URL is built
	// from it and none is derived by convention — see [Layout].
	Layout Layout
	// Client is the transport. Nil is http.DefaultClient, which is
	// correct and slow for a concurrent walk; see [NewHTTPClient].
	Client *http.Client
}

// NewHTTPSource binds a source to one publisher's advertised layout.
// A nil client means http.DefaultClient.
func NewHTTPSource(layout Layout, client *http.Client) *HTTPSource {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPSource{Layout: layout, Client: client}
}

// NewConsumer binds a consumer to a layout over HTTP, with its own cache.
//
// **These two live here, beside [HTTPSource], rather than in consume.go
// beside the type they build.** That is the one mechanical check a
// reader can run on whether the seam actually holds: consume.go — the
// whole verification stack — imports no transport package and names no
// client. A constructor taking an `*http.Client` sitting on top of it
// would leave the file looking HTTP-shaped while the type underneath had
// stopped being so, and the next person deciding where to put a
// transport detail would follow the example rather than the rule.
func NewConsumer(layout Layout, client *http.Client) *Consumer {
	return NewConsumerWithCache(layout, client, NewCache(0))
}

// NewConsumerWithCache binds a consumer to a layout over HTTP, over a
// shared cache. See [NewConsumer] for why it is in this file.
func NewConsumerWithCache(layout Layout, client *http.Client, cache *Cache) *Consumer {
	return NewConsumerFromSource(NewHTTPSource(layout, client), cache)
}

// PeerID implements [Source].
func (s *HTTPSource) PeerID() string { return s.Layout.PeerID }

// Root implements [Source]: GET {manifest_url_prefix}.
//
// The refusal when the publisher advertises no prefix is deliberate and
// belongs here rather than above: §6.5.3 *reserves* that location and
// does not make it derivable, so there is no fallback to invent. A
// derived manifest URL and a withholding origin are byte-identical at
// the consumer — both 404 — which is the shape this package refuses to
// get into (AP21).
func (s *HTTPSource) Root(ctx context.Context) ([]byte, string, error) {
	url := s.Layout.ManifestURL()
	if url == "" {
		return nil, "", errNoManifestPrefix
	}
	raw, err := httpGet(ctx, s.Client, url)
	return raw, url, err
}

// Leaf implements [Source]: GET {tree-base}/{path}{suffix}.
func (s *HTTPSource) Leaf(ctx context.Context, treePath string) ([]byte, string, error) {
	url := s.Layout.TreeLeafURL(treePath)
	raw, err := httpGet(ctx, s.Client, url)
	return raw, url, err
}

// Blob implements [Source]: GET {content_url_prefix}/{layout}/{hex(H)}.
//
// The hex is of the full wire form including the format-code byte; see
// [contentURL] for the MUST and for why this is not core-go's
// `types.BuildContentURL`.
func (s *HTTPSource) Blob(ctx context.Context, h hash.Hash) ([]byte, string, error) {
	url, err := s.Layout.ContentURL(h)
	if err != nil {
		return nil, "", err
	}
	raw, err := httpGet(ctx, s.Client, url)
	return raw, url, err
}
