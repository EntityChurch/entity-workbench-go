package workbench

import (
	"context"
	"fmt"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/fetch"

	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"
)

// peer_source.go — the second [fetch.Source]: read a publisher's bytes
// by dispatching at the machine that wrote them.
//
// # What this is not
//
// **It is not a second verification path, and there is deliberately
// nowhere in this file for one to go.** Everything that decides whether
// bytes are admissible — the recomputed content hash, the two-hop
// published-root signature against the key carried in the peer-id, the
// monotonic `seq` floor, the fail-closed CHAMP walk — lives in
// [fetch.Consumer] and runs identically whichever Source answered. See
// `fetch/source.go` for why that asymmetry is the point rather than a
// simplification.
//
// The reason it matters more here than on the static corridor:
// **an authenticated connection proves WHO, not WHAT.** The handshake
// establishes that the far end holds the key its peer-id names. It says
// nothing about whether the bytes it hands back are bytes it ever
// committed to — a peer can serve whatever it likes over its own
// connection, and a consumer that took the connection as the proof would
// have swapped a trust argument for a freshness one and called it an
// upgrade.
//
// # Why it lives in `workbench` and not in `fetch`
//
// `fetch` is deliberately peer-free: `entity-fetch` is a standalone CLI
// that links a content decoder and an HTTP client and no peer, no store
// and no location index — which is also why `fetch.Registry` exists
// rather than calling core-go's resolver. A `Source` that needs an
// `entitysdk.AppPeer` would pull the whole peer stack into that binary.
// `workbench` already imports both, so the second implementation lands
// on this side of the line and `fetch` keeps its one-way dependency.
//
// # Adoption, not construction
//
// Both primitives were already here. `AppPeer.Get` on a peer-qualified
// path is a *remote* read that routes to that peer's tree (AP11), and
// `AppPeer.ContentAt(peer).Get` is the same cross-peer `system/content:get`
// the sync leg has used since M2. Nothing in this file is new mechanism;
// D20 came back positive for the fifth time.

// PeerSource reads one publisher's bytes by dispatch.
//
// The caller must have connected to the publisher first
// (`AppPeer.Connect`, or the reconciler's dial ladder) **and the
// publisher must have authorized the read**: two operations,
// `system/tree:get` and `system/content:get`, over the site prefix *and*
// over `system/peer/published-root` and `system/signature/*`. That last
// part is easy to get wrong and is not obvious from the site's own
// paths — the signed root and its signature live outside the prefix they
// commit to, so a grant scoped to the site alone yields a peer that
// serves every page and cannot be verified at all.
//
// Safe for concurrent use: [fetch.Consumer.Walk] fetches a whole trie
// level at once, and each node becomes its own dispatch.
type PeerSource struct {
	ap     *entitysdk.AppPeer
	peerID string
}

// NewPeerSource binds a source to one publisher reachable by dispatch.
//
// peerID may be the local peer — a peer reading its own published site
// through the same stack is the cheapest control arm there is, and it
// takes the identical path because the read is peer-qualified either
// way.
func NewPeerSource(ap *entitysdk.AppPeer, peerID string) (*PeerSource, error) {
	if ap == nil {
		return nil, fmt.Errorf("workbench: PeerSource needs a peer to dispatch from")
	}
	if peerID == "" {
		return nil, fmt.Errorf("workbench: PeerSource needs the publisher's peer-id — " +
			"it is the key the signature verifies against, so there is no useful source without it")
	}
	return &PeerSource{ap: ap, peerID: peerID}, nil
}

// NewPeerConsumer binds a verifying [fetch.Consumer] to a live peer.
//
// The cache is shared across publishers on purpose (its key space is
// content hashes), and the consumer is meant to outlive one navigation —
// that is what gives the `seq` floor something to compare against. A
// consumer rebuilt per click cannot see a rollback at all.
func NewPeerConsumer(ap *entitysdk.AppPeer, peerID string, cache *fetch.Cache) (*fetch.Consumer, error) {
	src, err := NewPeerSource(ap, peerID)
	if err != nil {
		return nil, err
	}
	return fetch.NewConsumerFromSource(src, cache), nil
}

// PeerID implements [fetch.Source].
func (s *PeerSource) PeerID() string { return s.peerID }

// Describe implements [fetch.Source]: the publisher answers for itself
// here.
//
// **That is one fewer party, not a stronger check.** Nothing about the
// verification changes — an authenticated connection proves who, not
// what, which is the first thing this file says. What it removes is the
// origin: on the static corridor a third party stands between the
// publisher and the reader and can serve a correctly-signed root that is
// arbitrarily old, and here there is nobody in that position. A publisher
// that has simply not republished is still indistinguishable from a
// current one, in both modes. See `fetch/freshness.go`.
func (s *PeerSource) Describe() fetch.Description {
	return fetch.Description{Mode: fetch.ModeLivePeer, Authority: "entity://" + s.peerID}
}

// Locator is the address form this source reports for a tree path — the
// dispatched analogue of a URL, so the chain an operator reads names
// where each step looked whichever mode answered.
func (s *PeerSource) Locator(treePath string) string {
	return "entity://" + s.peerID + "/" + treePath
}

// Root implements [fetch.Source]: a dispatched tree:get at the
// publisher's canonical published-root storage path.
//
// **A peer that answers and has never published is its own state and is
// reported as one.** It is not "unreachable" (it answered) and not
// "withholding" (there is nothing to withhold): it is a peer that has
// committed to nothing, which is a real and common thing to be. See
// [fetch.ErrNoPublishedRoot] for why v1 refuses to read one through this
// stack rather than falling back to an unsigned chain.
func (s *PeerSource) Root(ctx context.Context) (entity.Entity, string, error) {
	rel := types.PublishedRootStoragePath()
	locator := s.Locator(rel)
	ent, found, err := s.ap.Get("/" + s.peerID + "/" + rel)
	if err != nil {
		return entity.Entity{}, locator, fmt.Errorf("tree:get %s: %w", locator, err)
	}
	if !found {
		return entity.Entity{}, locator, fmt.Errorf("%w: %s", fetch.ErrNoPublishedRoot, locator)
	}
	return ent, locator, nil
}

// Leaf implements [fetch.Source]: the content hash the publisher binds
// at a peer-relative tree path.
//
// **Over a dispatch there is no Amendment 6 pointer to crack.** That
// rule binds the HTTP tree-leaf projection — a leaf URL MUST serve the
// bound hash and not the dereferenced entity — and `system/tree:get`
// returning the entity is the protocol behaving correctly, not a
// publisher cutting a corner. So the binding is recovered the only way
// it can be here: recompute the hash of what came back.
//
// The claimed `content_hash` is never returned as the answer. It is used
// only to pick the algorithm (v7.67 §2.3 makes the hash format
// process-global to the *author*, so a consumer cannot assume its own),
// and then it has to agree with the bytes or this refuses — the same
// discipline `publishedroot.Check` applies one layer up. A publisher
// that lies about either is caught above the seam anyway, because the
// value is about to be compared with what the *signed* root committed.
func (s *PeerSource) Leaf(ctx context.Context, treePath string) (hash.Hash, string, error) {
	locator := s.Locator(treePath)
	ent, found, err := s.ap.Get("/" + s.peerID + "/" + treePath)
	if err != nil {
		return hash.Hash{}, locator, fmt.Errorf("tree:get %s: %w", locator, err)
	}
	if !found {
		return hash.Hash{}, locator, fmt.Errorf("%s: nothing bound at this path", locator)
	}
	h, err := recomputeServedHash(ent)
	if err != nil {
		return hash.Hash{}, locator, fmt.Errorf("%s: %w", locator, err)
	}
	return h, locator, nil
}

// Blob implements [fetch.Source]: a cross-peer `system/content:get`.
//
// The hash is NOT checked here, though it would be one line: the check
// belongs to [fetch.Consumer.Blob], which is the only door into the
// content store and the only place the cache is filled. A Source that
// verified too would make the real check look optional to the next
// implementer.
//
// **Side effect worth knowing about:** the SDK's content client ingests
// what it fetches into the local content store, symmetric with the
// §7.2 transfer pattern. Reading a live site therefore leaves its blobs
// behind on this peer. That is safe — the store keys by a hash it
// computes itself, so nothing can be filed under a name it is not — but
// it is a write, and a reader expecting a read to be inert should know.
//
// Batching is left on the table: `Source.Blob` asks for one hash and
// `content:get` takes an array, so a 51-node walk is 51 dispatches where
// it could be a handful. The interface would have to grow a plural
// primitive to fix it, and no measurement yet says it is worth the
// widening.
func (s *PeerSource) Blob(ctx context.Context, h hash.Hash) (entity.Entity, string, error) {
	locator := "entity://" + s.peerID + "/system/content/" + h.String()

	cc := s.ap.ContentAt(s.peerID)
	if s.peerID == s.ap.PeerID() {
		cc = s.ap.Content()
	}
	res, err := cc.Get(ctx, []hash.Hash{h})
	if err != nil {
		return entity.Entity{}, locator, fmt.Errorf("content:get %s: %w", locator, err)
	}
	ent, ok := res.Entities[h]
	if !ok {
		// Distinguished on purpose: a publisher that answers and does not
		// hold a hash its own signed root commits to is the withholding
		// case §6.5.3 makes a publish-side MUST, and the walk above turns
		// it into `tree/incomplete-walk`. Reporting it as a transport
		// failure would send an operator to retry a condition that
		// retrying cannot change.
		return entity.Entity{}, locator, fmt.Errorf("this peer does not hold %s", h)
	}
	return ent, locator, nil
}

// recomputeServedHash is the served-entity half of the discipline
// `fetch.verifyEntity` applies to served bytes: the hash is derived, and
// a self-claimed one is checked rather than believed.
func recomputeServedHash(ent entity.Entity) (hash.Hash, error) {
	if ent.Type == "" {
		return hash.Hash{}, fmt.Errorf("served entity carries no type")
	}
	alg := ent.ContentHash.Algorithm
	if ent.ContentHash.IsZero() {
		alg = hash.AlgorithmSHA256
	}
	computed, err := hash.ComputeFormat(alg, ent.Type, ent.Data)
	if err != nil {
		return hash.Hash{}, fmt.Errorf("recompute served content hash: %w", err)
	}
	if !ent.ContentHash.IsZero() && ent.ContentHash != computed {
		return hash.Hash{}, fmt.Errorf("served entity claims hash %s but its bytes are %s",
			ent.ContentHash, computed)
	}
	return computed, nil
}
