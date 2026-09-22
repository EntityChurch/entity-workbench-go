package fetch

import (
	"context"
	"fmt"
	"net/http"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
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
// # The unit is an ENTITY, and it was `[]byte` until the second
// implementation existed
//
// **A seam with one implementation is a hypothesis.** This interface
// shipped on 2026-09-11 with all three primitives returning `raw
// []byte`, asserted to be transport-neutral, and W1 said in as many
// words that the assertion was untested until something else
// implemented it. It was wrong in all three, in the same way, and a
// dispatched read is what showed it:
//
//	Root   raw bytes → entity.Entity   a dispatched tree:get hands back a
//	                                   decoded entity; the wire bytes are
//	                                   the protocol's framing and never
//	                                   reach this package
//	Blob   raw bytes → entity.Entity   same, via system/content:get
//	Leaf   raw bytes → hash.Hash       over HTTP a tree leaf is an
//	                                   Amendment 6 *pointer*; over
//	                                   dispatch tree:get returns the
//	                                   entity the binding NAMES
//
// So `ecf.Decode` was never a check — it was HTTP's *framing*, sitting
// above the seam because HTTP was the only thing below it. The checks
// are what stayed: [verifyEntity] recomputes the hash over (type, data)
// and requires it to be the one the tree bound, the two-hop signature
// verifies against the key in the peer-id, the seq floor holds, and the
// walk fails closed. None of those moved, and none of them can be
// satisfied by a Source.
//
// **`Leaf` returning a hash is the one worth arguing about**, because it
// moves a conformance check below the seam. EXTENSION-NETWORK
// Amendment 6 makes an HTTP tree leaf the bound hash pointer and *not*
// the dereferenced entity — but that is a rule about the HTTP
// projection, and a dispatched `system/tree:get` returning the entity is
// the protocol behaving correctly rather than a one-hop publisher
// cutting a corner. A check that fires on conformant behaviour on the
// other transport is not a stricter check, it is a false refusal, and
// this repository has a catalogue entry for those (AP44). So
// [crackPointer] moved into [HTTPSource.Leaf], where it is a statement
// about the transport that has the obligation. The question both callers
// above the seam were actually asking — *what hash does this publisher
// bind at this path* — is transport-neutral and is what the method now
// answers.
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

	// Describe names the kind of party that answers here, and it exists
	// for exactly one downstream decision: the freshness sentence.
	//
	// W1 left this method out on the ground that it *"would have no
	// reader until"* the mode mattered, which was right then and is not
	// now — see freshness.go for what the two modes may and may not
	// claim. **It reports a KIND, never a verdict**: there is no field
	// here for a Source to say its bytes are good, for the reason the
	// file note gives.
	Describe() Description

	// Root returns the publisher's current published-root entity — the
	// MUTABLE pointer, re-read on every navigation because it is the only
	// thing under a publisher that can change.
	//
	// It returns the entity as served and nothing more: the type check,
	// the recomputed content hash, the peer-id check, the §3.3a prefix
	// check, the two-hop signature and the seq floor all live above the
	// seam. A Source is never asked whether this root is good.
	Root(ctx context.Context) (ent entity.Entity, locator string, err error)

	// Leaf returns the content hash this publisher binds at a
	// peer-relative tree path.
	//
	// **It answers the binding, not the body**, and the caller fetches
	// the body through [Consumer.Blob] so that the one door into the
	// content store stays one door. How the binding is obtained is the
	// transport's business and the two differ: HTTP serves an
	// Amendment 6 pointer to be cracked, a dispatched `system/tree:get`
	// serves the bound entity and the hash is recomputed from it. See the
	// file note for why that asymmetry belongs below the seam rather than
	// above it.
	Leaf(ctx context.Context, treePath string) (h hash.Hash, locator string, err error)

	// Blob returns the content-store entity for h.
	//
	// **It must not check the hash**, however easy that would be here.
	// [Consumer.Blob] is the only door into the content store and the
	// only place [Cache] is filled, both on the far side of
	// [verifyEntity]; a Source that verified too would make the real
	// check look optional to the next implementer.
	Blob(ctx context.Context, h hash.Hash) (ent entity.Entity, locator string, err error)
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

// Describe implements [Source]: an origin answers here, and it is a
// different party from the publisher whose signature is being checked.
// That gap is the whole content of the static freshness sentence.
func (s *HTTPSource) Describe() Description {
	return Description{Mode: ModeStaticOrigin, Authority: s.Layout.Origin}
}

// Root implements [Source]: GET {manifest_url_prefix}.
//
// The refusal when the publisher advertises no prefix is deliberate and
// belongs here rather than above: §6.5.3 *reserves* that location and
// does not make it derivable, so there is no fallback to invent. A
// derived manifest URL and a withholding origin are byte-identical at
// the consumer — both 404 — which is the shape this package refuses to
// get into (AP21).
func (s *HTTPSource) Root(ctx context.Context) (entity.Entity, string, error) {
	url := s.Layout.ManifestURL()
	if url == "" {
		return entity.Entity{}, "", errNoManifestPrefix
	}
	raw, err := httpGet(ctx, s.Client, url)
	if err != nil {
		return entity.Entity{}, url, err
	}
	var ent entity.Entity
	if err := ecf.Decode(raw, &ent); err != nil {
		return entity.Entity{}, url, fmt.Errorf("decode manifest: %w", err)
	}
	return ent, url, nil
}

// Leaf implements [Source]: GET {tree-base}/{path}{suffix}, then crack
// the Amendment 6 pointer it serves.
//
// **The pointer check is HTTP's and lives here**, because Amendment 6 is
// an obligation on this projection: a leaf URL MUST serve the bound hash
// and not the entity it names, and a one-hop publisher's leaf decodes
// perfectly well as *some* entity, so the type check is the only thing
// between a consumer and silently losing the dedup invariant. It is not
// a rule a dispatched read breaks — see [crackPointer] and the file
// note.
func (s *HTTPSource) Leaf(ctx context.Context, treePath string) (hash.Hash, string, error) {
	url := s.Layout.TreeLeafURL(treePath)
	raw, err := httpGet(ctx, s.Client, url)
	if err != nil {
		return hash.Hash{}, url, err
	}
	h, err := crackPointer(raw)
	if err != nil {
		return hash.Hash{}, url, err
	}
	return h, url, nil
}

// Blob implements [Source]: GET {content_url_prefix}/{layout}/{hex(H)}.
//
// The hex is of the full wire form including the format-code byte; see
// [contentURL] for the MUST and for why this is not core-go's
// `types.BuildContentURL`.
//
// The decode is framing and the hash check is not: this returns whatever
// the origin served, and [Consumer.Blob] is where it is proved to be the
// pre-image of h.
func (s *HTTPSource) Blob(ctx context.Context, h hash.Hash) (entity.Entity, string, error) {
	url, err := s.Layout.ContentURL(h)
	if err != nil {
		return entity.Entity{}, "", err
	}
	raw, err := httpGet(ctx, s.Client, url)
	if err != nil {
		return entity.Entity{}, url, err
	}
	var ent entity.Entity
	if err := ecf.Decode(raw, &ent); err != nil {
		return entity.Entity{}, url, fmt.Errorf("fetch: decode entity: %w", err)
	}
	return ent, url, nil
}
