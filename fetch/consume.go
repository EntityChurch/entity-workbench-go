package fetch

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"entity-workbench-go/entitysdk/publishedroot"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"
)

// The verifying consume stack — the operation this package could not do
// before 2026-08-20, and the one that separates "hash-verified" from
// "not lied to".
//
// **What was here already, and why it was not enough.** `Fetch` resolves
// one path through the publisher's advertised tree-leaf URL and proves
// the body is the pre-image of the hash the leaf bound. That is a real
// gate against corruption and truncation and it is **not** a gate
// against a lying origin, because the origin supplied the pointer too
// (arch put it best in `ROUTING-2026-08-20-m` §1: *"hash-verified,
// therefore safe" is a true premise with a false inference*). Our other
// half — `entitysdk`'s `ReadPublishedRoot` — verifies a signature but
// reads out of the local store and never touches a wire.
//
// A [Consumer] is the two halves joined and the missing operation added:
//
//	manifest → signature → CHAMP trie walk from the SIGNED root → leaf
//
// Only the walk can see the defect the shape exists for. An origin that
// serves a well-formed, correctly-signed root whose closure it does not
// serve is indistinguishable from a small site at every other step —
// `entity-core-go` shipped exactly that for a week (a `0xC0C1C2…`
// literal root, fixed at `dabd076`), and no per-leaf fetch anywhere in
// this cohort noticed. Walking is what turns it into an error.
//
// Two deliberate departures from the obvious implementation, each
// load-bearing:
//
//  1. **The walk is ours, not `core/tree.CollectAllBindings`.** The
//     kernel helper takes a `store.ContentStore` and would have been a
//     three-line delegation, but it is documented best-effort: a node it
//     cannot load is `continue`, and `CollectNodeClosure` says so in as
//     many words ("Missing nodes are skipped"). Over a local store that
//     is fine — the store is ours. Over an HTTP origin it converts a
//     **withholding origin into a smaller site**, silently, which is the
//     precise defect the walk was added to catch. So [Consumer.Walk]
//     re-implements the traversal over the same kernel *types* (the
//     Layer-2 contract that must not vary) and fails closed on the first
//     unresolvable node. Routed to core-go as a helper-shape finding.
//  2. **The layout may be pinned instead of discovered.** `LoadLayout`
//     stays the front door, but `entity-core-go`'s federation origin
//     serves no `transport-profile` object at all, so a consumer that
//     required one could not reach it. See [PinnedLayout] — and arch's
//     R-28, which is this gap seen from the other side.

// Walk / consume failures. Each is distinguishable on purpose: an
// operator who cannot tell "the origin is down" from "the origin is
// withholding" from "that key does not exist" will read all three as the
// first one and retry forever.
var (
	// ErrIncompleteWalk is the §3.3a `tree/incomplete-walk` condition:
	// the signed root commits to a node the origin does not serve.
	ErrIncompleteWalk = errors.New("tree/incomplete-walk")

	// ErrAbsent means the enumerated key set does not contain the key —
	// a positive answer about a non-existent object, not a failure.
	ErrAbsent = errors.New("absent")

	// ErrEmptyEnumeration guards the absent-key control. An empty root
	// answers Absent to every question, so "absent" only carries
	// information once something has been enumerated.
	ErrEmptyEnumeration = errors.New("enumeration is empty, so Absent carries no information")

	// ErrContractMismatch is a leaf whose trie-routed hash disagrees
	// with the hash the publisher's own tree-leaf URL advertises.
	ErrContractMismatch = errors.New("trie-routed hash disagrees with the advertised leaf pointer")

	// errNoManifestPrefix is an HTTP publisher that advertises no
	// `manifest_url_prefix`, so this origin has no signed entry point.
	//
	// It is a sentinel rather than a formatted string because it now
	// crosses the [Source] seam: [HTTPSource.Root] is where the fact
	// lives and [Consumer.VerifiedRoot] is where it must NOT be wrapped
	// in "fetch manifest:", which would read as a network failure. §6.5.3
	// reserves that location and does not make it derivable, so there is
	// nothing to fall back to — and a derived manifest URL is
	// byte-identical to a withholding origin at the consumer (AP21).
	errNoManifestPrefix = errors.New("fetch: publisher advertises no manifest_url_prefix, " +
		"so this origin has no signed entry point (§6.5.3 reserves the location; it is not derivable)")

	// ErrSeqRollback is a published-root whose `seq` is below one this
	// consumer already accepted from the same publisher.
	//
	// It is only detectable by a consumer that OUTLIVES one read. A
	// client rebuilt per navigation compares each root against nothing,
	// so an origin can hand out `seq 5` for one page and `seq 3` for the
	// next — replaying a previous publish, one page at a time, with a
	// perfectly valid signature on every response. That is not a corner
	// case: it is the cheapest attack available to the §6a.1a fourth
	// actor, who may choose which signed artifact answers a read.
	ErrSeqRollback = errors.New("published-root seq went backwards (rollback)")
)

// PinnedLayout builds a Layout from a hand-supplied endpoint block,
// skipping profile discovery entirely.
//
// **This exists because discovery is not always available and pretending
// otherwise makes a consumer that works against one publisher.**
// `entity-core-go`'s federation origin (`scripts/federation-publish.sh`)
// serves a manifest, a tree and a content store, and **no**
// `transport-profile` — §6.5.4 makes profile distribution out-of-band in
// v1, so that is conformant, and a consumer handed only `(origin,
// peer_id)` has nowhere to learn `content_layout` from. It then guesses,
// and **a wrong layout guess and a withholding origin are byte-identical
// at the consumer**: every blob 404s either way. That is arch's R-28,
// filed 2026-08-20 from `entity-browser-rust`'s side of the same wall.
//
// Our position, unchanged and now with a second data point: the
// well-known object is the cheap fix and we ship it in `publish/`.
// Pinning is what an operator does in the meantime, and it is honest
// precisely because it is explicit — the layout came from a human, not
// from a convention the consumer invented.
func PinnedLayout(origin, peerID string, ep types.TransportEndpoint) (Layout, error) {
	if peerID == "" {
		return Layout{}, fmt.Errorf("fetch: PinnedLayout needs the publisher's peer-id — " +
			"it is the key the signature verifies against, so there is no useful pin without it")
	}
	if err := ep.Validate(); err != nil {
		return Layout{}, fmt.Errorf("fetch: pinned endpoint: %w", err)
	}
	return Layout{
		Origin:   strings.TrimRight(origin, "/"),
		PeerID:   peerID,
		Endpoint: ep,
	}, nil
}

// WalkConcurrency is how many CHAMP nodes a walk fetches at once.
//
// A trie walk is embarrassingly parallel and was serial: 51 nodes at
// ~90 ms is 4.6 seconds of a browser doing one thing at a time against a
// CDN built to do thousands. Nothing about the traversal needs an order
// — each node is fetched by hash and hash-verified independently, so
// concurrency cannot change what is admitted, only when.
//
// Bounded, and the bound is the point: an unbounded fan-out over a
// 966-key trie is a small denial-of-service pointed at someone else's
// origin, launched by a user clicking a link.
const WalkConcurrency = 8

// Consumer is a verifying reader bound to one publisher's layout.
//
// **It is meant to outlive one navigation.** The manifest is re-fetched
// on every [Consumer.VerifiedRoot] because it is the mutable pointer;
// everything under it is content-addressed and is held in [Cache]. See
// cache.go for the measurement and for the security property a
// per-navigation consumer silently gives up (the seq floor, below).
// **Its bytes come from a [Source] and its verification does not.** The
// seam is under the byte source and nowhere else — see source.go. Every
// check below (the recomputed hash, the two-hop signature, the seq
// floor, the fail-closed walk) is transport-neutral and runs identically
// whatever answered, because a second verification path would be two
// code paths for one trust argument with the weaker one wearing the same
// UI.
type Consumer struct {
	// src is the byte source. Unexported on purpose: a caller reaching
	// past the consumer to fetch from the source directly gets bytes that
	// nothing checked, which is the one way to use this package wrongly.
	src Source
	// Cache is shared, and sharing it across publishers is correct: the
	// key space is content hashes, so two publishers who committed the
	// same bytes name the same entry, and neither can put anything in it
	// that did not hash to its own key.
	Cache *Cache

	mu sync.Mutex
	// minSeq is the highest published-root `seq` this consumer has
	// accepted. §3-RES.4's freshness discipline is monotonic per peer,
	// and enforcing it needs a memory that spans navigations — which is
	// exactly what a consumer rebuilt per click does not have.
	minSeq     uint64
	haveMinSeq bool
}

// NewConsumerFromSource binds a consumer to any byte source.
//
// This is the general door. `NewConsumer` / `NewConsumerWithCache` are
// it with [HTTPSource] filled in and live in source.go, so that **this
// file names no transport at all** — which is the property the seam
// exists for and the one a reader should be able to check by looking at
// the imports.
func NewConsumerFromSource(src Source, cache *Cache) *Consumer {
	if cache == nil {
		cache = NewCache(0)
	}
	return &Consumer{src: src, Cache: cache}
}

// PeerID is the publisher this consumer reads — the key its signature
// checks verify against.
func (c *Consumer) PeerID() string { return c.src.PeerID() }

// VerifiedRoot is a published-root that verified over the wire.
//
// A value of this type only exists if the two-hop signature checked out
// against the key the publisher's peer-id itself carries — there is no
// `Verified bool`, for the reason `entitysdk/published_root.go` gives:
// a false one a caller forgets to check is how an unsigned root gets
// treated as a signed one.
type VerifiedRoot struct {
	Entity    entity.Entity
	Data      types.PublishedRootData
	Signature types.SignatureData

	ManifestURL  string
	SignatureURL string
}

// VerifiedRoot fetches the manifest and verifies it end to end.
//
//	GET {manifest_url_prefix}                       → published-root entity
//	   recompute its hash; check type, peer_id, §3.3a prefix
//	GET {tree-base}/system/signature/{hex}{suffix}  → the §5.2 pointer
//	GET {content}/{hex}                             → the signature entity
//	   verify over the RECOMPUTED hash, against the key in the peer-id
//
// The signature is resolved from the hash we recomputed, never from the
// one the origin served, so an origin cannot steer us at a signature it
// prepared for different bytes.
//
// **What this returns is "verified as of published_at", never
// "verified"** (§6.5.3.1, D6/D7): a quiet publisher and a withholding
// origin are indistinguishable from here, and no freshness field closes
// that. The caller decides what age it will accept.
func (c *Consumer) VerifiedRoot(ctx context.Context) (VerifiedRoot, error) {
	raw, manifestURL, err := c.src.Root(ctx)
	if err != nil {
		if errors.Is(err, errNoManifestPrefix) {
			return VerifiedRoot{}, err
		}
		return VerifiedRoot{}, fmt.Errorf("fetch manifest: %w", err)
	}
	var ent entity.Entity
	if err := ecf.Decode(raw, &ent); err != nil {
		return VerifiedRoot{}, fmt.Errorf("fetch: decode manifest %s: %w", manifestURL, err)
	}

	peerID := c.src.PeerID()
	ent, data, err := publishedroot.Check(peerID, manifestURL, ent)
	if err != nil {
		return VerifiedRoot{}, err
	}

	pub, keyType, err := publishedroot.DeriveKey(peerID)
	if err != nil {
		return VerifiedRoot{}, err
	}

	sigRel := publishedroot.SignatureRelPath(ent.ContentHash)
	sigEnt, sigURL, err := c.leafAt(ctx, sigRel)
	if err != nil {
		return VerifiedRoot{}, fmt.Errorf("fetch: resolving the §5.2 signature pointer at %s: %w — "+
			"a published-root whose signature cannot be reached is unverifiable, not unsigned", sigURL, err)
	}

	sig, err := publishedroot.VerifySignature(sigURL, ent, sigEnt, pub, keyType)
	if err != nil {
		return VerifiedRoot{}, err
	}

	if err := c.acceptSeq(data.Seq, manifestURL); err != nil {
		return VerifiedRoot{}, err
	}

	return VerifiedRoot{
		Entity:       ent,
		Data:         data,
		Signature:    sig,
		ManifestURL:  manifestURL,
		SignatureURL: sigURL,
	}, nil
}

// acceptSeq is the §3-RES.4 monotonicity floor, and it is one function
// so there is one copy of the comparison.
//
// **Called deliberately AFTER the signature check**: a rollback is a
// correctly-signed root being replayed, so refusing on `seq` before
// establishing the signer would be refusing on an unsigned number.
//
// Equal is accepted — a republish of one root is not a rollback — and
// only a strictly higher seq moves the floor.
func (c *Consumer) acceptSeq(seq uint64, locator string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.haveMinSeq && seq < c.minSeq {
		return fmt.Errorf("%w: %s served seq=%d and this session already accepted "+
			"seq=%d from the same publisher — both roots are validly signed, which is what makes "+
			"this a replay rather than a corruption",
			ErrSeqRollback, locator, seq, c.minSeq)
	}
	if !c.haveMinSeq || seq > c.minSeq {
		c.minSeq, c.haveMinSeq = seq, true
	}
	return nil
}

// Binding is one committed (key → content hash) pair, with key relative
// to the published-root's `prefix`.
type Binding struct {
	Key  string
	Hash hash.Hash
}

// WalkResult is the committed key set of a signed root, plus what the
// traversal cost. Bindings are sorted by key so a report is stable.
type WalkResult struct {
	Root     hash.Hash
	Bindings []Binding
	// NodeHashes is every distinct CHAMP node the walk resolved, in
	// **breadth-first visit order** — the interior structure a per-leaf
	// consumer never touches. Exposed because it is the served set
	// §6.5.3 obliges the publisher to cover, so it is the thing an
	// operator wants to see and the thing a test withholds one of.
	//
	// Breadth-first rather than the depth-first order this had before
	// 2026-08-31, because a level is the unit that can be fetched
	// concurrently. Deterministic either way: within a level, nodes
	// appear in the order their parents declared them.
	NodeHashes []hash.Hash
}

// Nodes is the number of distinct CHAMP nodes the walk resolved.
func (w WalkResult) Nodes() int { return len(w.NodeHashes) }

// Keys returns just the committed keys, sorted.
func (w WalkResult) Keys() []string {
	out := make([]string, len(w.Bindings))
	for i, b := range w.Bindings {
		out[i] = b.Key
	}
	return out
}

// Lookup resolves one key against the enumerated set.
//
// **Absence is only meaningful once the enumeration is non-empty**, so a
// lookup against an empty walk returns ErrEmptyEnumeration rather than
// ErrAbsent. An empty root answers "absent" to every question — which is
// exactly how core-go's pre-`dabd076` origin looked — and a control that
// passes against it is measuring nothing.
func (w WalkResult) Lookup(key string) (hash.Hash, error) {
	if len(w.Bindings) == 0 {
		return hash.Hash{}, ErrEmptyEnumeration
	}
	for _, b := range w.Bindings {
		if b.Key == key {
			return b.Hash, nil
		}
	}
	return hash.Hash{}, fmt.Errorf("%w: %q is not in the %d committed keys", ErrAbsent, key, len(w.Bindings))
}

// Walk traverses the CHAMP trie from root and returns every committed
// binding, failing closed on the first node the origin does not serve.
//
// Each node is fetched from the content store by hash and hash-verified
// before it is decoded, so a node body that is not its own pre-image is
// refused rather than parsed — a trie node is a routing decision, and
// trusting an unverified one hands the origin the choice of which
// subtree a consumer sees.
//
// See the package note for why this is not `tree.CollectAllBindings`.
//
// **Memoized on `root`, and that is sound rather than a shortcut.** A
// CHAMP trie rooted at H has exactly one key set for all time — every
// edge in it is a content hash — so re-walking an unchanged root is
// re-deriving a value that could not have moved. The freshness question
// lives one level up, in [Consumer.VerifiedRoot], which re-fetches the
// mutable manifest every time and hands us a possibly-different H. Only
// a walk that ran to completion is memoized; see [Cache.PutWalk].
//
// The traversal is breadth-first and fetches each level with
// [WalkConcurrency] in flight. It still fails closed on the first node
// the origin does not serve — the whole reason this is not the kernel's
// best-effort helper — and a failure anywhere in a level fails the walk,
// with the FIRST failure in declared order reported so the error is
// stable across runs rather than a race between goroutines.
func (c *Consumer) Walk(ctx context.Context, root hash.Hash) (WalkResult, error) {
	if w, ok := c.Cache.Walk(root); ok {
		return w, nil
	}

	res := WalkResult{Root: root}
	seen := map[hash.Hash]bool{root: true}
	out := map[string]hash.Hash{}

	frontier := []hash.Hash{root}
	for depth := 0; len(frontier) > 0; depth++ {
		res.NodeHashes = append(res.NodeHashes, frontier...)

		ents := make([]entity.Entity, len(frontier))
		errs := make([]error, len(frontier))
		var wg sync.WaitGroup
		sem := make(chan struct{}, WalkConcurrency)
		for i, h := range frontier {
			wg.Add(1)
			go func(i int, h hash.Hash) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				ents[i], errs[i] = c.Blob(ctx, h)
			}(i, h)
		}
		wg.Wait()

		var next []hash.Hash
		for i, h := range frontier {
			if errs[i] != nil {
				return res, fmt.Errorf("%w: the signed root commits to CHAMP node %s at depth %d, and "+
					"this origin does not serve it (§6.5.3 makes the closure of `root_hash` a "+
					"publish-side MUST): %w", ErrIncompleteWalk, h, depth, errs[i])
			}
			ent := ents[i]
			if ent.Type != types.TypeTreeSnapshotNode {
				return res, fmt.Errorf("%w: %s is type %q, want %s — the trie structure is not what "+
					"the root committed to", ErrIncompleteWalk, h, ent.Type, types.TypeTreeSnapshotNode)
			}
			node, err := types.SnapshotNodeDataFromEntity(ent)
			if err != nil {
				return res, fmt.Errorf("%w: decode CHAMP node %s: %w", ErrIncompleteWalk, h, err)
			}
			for _, e := range node.Data {
				if e.IsLink() {
					// CHAMP is a DAG: a shared subtree is visited once.
					if seen[*e.Link] {
						continue
					}
					seen[*e.Link] = true
					next = append(next, *e.Link)
					continue
				}
				for _, t := range e.Bucket {
					out[t.Key] = t.ValueHash
				}
			}
		}
		frontier = next
	}

	res.Bindings = make([]Binding, 0, len(out))
	for k, h := range out {
		res.Bindings = append(res.Bindings, Binding{Key: k, Hash: h})
	}
	sort.Slice(res.Bindings, func(i, j int) bool { return res.Bindings[i].Key < res.Bindings[j].Key })
	c.Cache.PutWalk(root, res)
	return res, nil
}

// Blob fetches one content-addressed body and proves it is the bytes h
// names. This is the only door into the content store — everything the
// consumer reads, trie nodes included, comes through it.
//
// **And it is therefore the only place [Cache] is filled**, on the far
// side of [decodeVerified]. Nothing enters the cache that has not been
// recomputed and matched against the hash it is filed under, so a later
// reader taking a hit stands on the same proof the first reader did.
func (c *Consumer) Blob(ctx context.Context, h hash.Hash) (entity.Entity, error) {
	if ent, ok := c.Cache.Blob(h); ok {
		return ent, nil
	}
	body, _, err := c.src.Blob(ctx, h)
	if err != nil {
		return entity.Entity{}, err
	}
	ent, err := decodeVerified(body, h)
	if err != nil {
		return entity.Entity{}, err
	}
	c.Cache.PutBlob(h, ent)
	return ent, nil
}

// leafAt resolves a peer-relative tree path the advertised way: the
// two-hop `system/hash` pointer at the leaf, then the content blob it
// names. This is `Fetch`'s path, reused — the tree-leaf surface is how
// the SIGNATURE is reached (§5.2 makes it an invariant pointer, not a
// trie key), so the consumer needs both resolution paths, not one.
//
// The locator is returned on the failure paths too, because the caller's
// error message names where it looked and a failed lookup is exactly
// when that matters.
func (c *Consumer) leafAt(ctx context.Context, treePath string) (entity.Entity, string, error) {
	raw, locator, err := c.src.Leaf(ctx, treePath)
	if err != nil {
		return entity.Entity{}, locator, err
	}
	h, err := crackPointer(raw)
	if err != nil {
		return entity.Entity{}, locator, fmt.Errorf("%s: %w", locator, err)
	}
	ent, err := c.Blob(ctx, h)
	return ent, locator, err
}

// AbsolutePrefix resolves a published root's **configured** `prefix`
// into the absolute form EXTENSION-TREE §3.3's reconstruction rule is
// stated over — the operand for `absolute_prefix + relative_key`.
//
// **The field on the wire is the configured prefix, not the absolute
// one, and the three admissible shapes resolve differently** (§3.3's
// table, `[MUST; ruled 2026-08-08]`):
//
//	"system/"      peer-relative   → /{peer_id}/system/
//	"/{peer_id}/"  peer-qualified  → /{peer_id}/            (already absolute)
//	"/"            universal tree  → ""  — the trim is a NO-OP, so the
//	                                 committed keys are fully qualified
//
// **Both live publishers in this cohort pick a different one of those
// three**: `entity-browser-rust` publishes the peer-qualified shape,
// our own `publish/` publishes the peer-relative shape (`docs/`). A
// consumer that concatenates the field verbatim is right for one of
// them and silently wrong for the other — and "silently" is the word,
// because a mis-joined path is a 404, which reads as a withholding
// origin rather than as a consumer bug. Measured 2026-08-20, building
// this consumer; the first version of it got the right answer for both
// by luck.
//
// The universal case returns the empty string on purpose. Trimming the
// local peer-id there would leave every *other* peer's keys qualified
// and produce a mixed key space — §3.3 rules that out in as many words.
func AbsolutePrefix(prefix, peerID string) string {
	switch {
	case prefix == "" || prefix == "/":
		return ""
	case strings.HasPrefix(prefix, "/"):
		return prefix
	default:
		return "/" + peerID + "/" + prefix
	}
}

// AbsolutePath reconstructs the absolute tree path of a committed key —
// §3.3's `absolute_prefix + relative_key`, with `prefix` resolved.
func AbsolutePath(prefix, peerID, key string) string {
	return AbsolutePrefix(prefix, peerID) + key
}

// PointerFor resolves the hash the publisher's own tree-leaf URL
// advertises for a committed key.
//
// `key` is relative to the published-root's `prefix` (the configured
// form — see [AbsolutePrefix]); the tree-leaf URL is built from a
// peer-relative path, so the key is reconstructed to absolute and the
// peer segment stripped back off.
func (c *Consumer) PointerFor(ctx context.Context, prefix, key string) (hash.Hash, string, error) {
	peerID := c.src.PeerID()
	rel := peerRelative(AbsolutePath(prefix, peerID, key), peerID)
	raw, locator, err := c.src.Leaf(ctx, rel)
	if err != nil {
		return hash.Hash{}, locator, err
	}
	h, err := crackPointer(raw)
	if err != nil {
		return hash.Hash{}, locator, fmt.Errorf("%s: %w", locator, err)
	}
	return h, locator, nil
}

// peerRelative turns an absolute `/{peer}/a/b` tree path into the
// `a/b` form the advertised tree-leaf URL is built from.
func peerRelative(abs, peerID string) string {
	rel := strings.TrimPrefix(abs, "/")
	rel = strings.TrimPrefix(rel, peerID)
	return strings.TrimPrefix(rel, "/")
}

// KeyResult is one key's verdict in a [Report].
type KeyResult struct {
	Key string
	// TrieHash is what the signed root committed to for this key.
	TrieHash hash.Hash
	// Bytes is the size of the verified body, or 0 if it was not
	// fetched (Opts.Bodies false).
	Bytes int
	// Type is the entity type of the verified body, when fetched.
	Type string
	// PointerHash is what the publisher's advertised tree-leaf URL says
	// for the same key, when reconciliation ran.
	PointerHash hash.Hash
	// Reconciled is true when PointerHash was fetched and matched.
	Reconciled bool
	// Err is this key's failure, if any. A per-key error does not stop
	// the run — one unreadable page is a fact about that page, and a
	// report that dies on the first one cannot say how much of the site
	// is good.
	Err error
}

// ConsumeOpts tunes a full consume run.
type ConsumeOpts struct {
	// Bodies fetches and hash-verifies every committed leaf. Off by
	// default: enumeration + the walk are the security-relevant part,
	// and a large site's bodies are the expensive part.
	Bodies bool
	// Reconcile additionally resolves each key through the publisher's
	// advertised tree-leaf URL and requires the two answers to agree.
	// This is the check only a consumer holding BOTH resolution paths
	// can make — see [Report].
	Reconcile bool
	// AbsentProbe is a key expected NOT to be committed. It runs only
	// after a non-empty enumeration.
	AbsentProbe string
}

// Report is the result of a full consume run — the structure a renderer
// draws and a gate asserts on.
//
// **The reconciliation field is the one nobody else in this cohort can
// produce.** `entity-browser-rust`'s consumer cross-checks the
// trie-routed hash against the *site contract* it fetched; a per-leaf
// consumer checks neither. We hold the trie walk and the advertised
// tree-leaf path in one process, so we can ask the question directly:
// does the origin's own leaf URL agree with the root it signed? An
// origin that answers differently on the two paths is serving two
// different trees and only one of them is signed.
type Report struct {
	Root VerifiedRoot
	Walk WalkResult
	Keys []KeyResult

	// AbsentProbe records the absent-key control: the key asked for and
	// whether it was correctly reported absent.
	AbsentProbe   string
	AbsentCorrect bool
	AbsentErr     error
}

// Failures returns the keys whose verification failed.
func (r Report) Failures() []KeyResult {
	var out []KeyResult
	for _, k := range r.Keys {
		if k.Err != nil {
			out = append(out, k)
		}
	}
	return out
}

// Consume runs the whole chain and reports it: pin → manifest →
// signature → trie walk → enumerate → (leaves) → (reconcile) → absent
// control.
//
// The first four steps are fatal — without a verified root there is
// nothing to report about. Per-key results are collected rather than
// thrown, so the report can say "217 of 218 verified" instead of
// stopping at the first bad body.
func (c *Consumer) Consume(ctx context.Context, opts ConsumeOpts) (Report, error) {
	root, err := c.VerifiedRoot(ctx)
	if err != nil {
		return Report{}, err
	}
	walk, err := c.Walk(ctx, root.Data.RootHash)
	if err != nil {
		return Report{Root: root}, err
	}
	return c.ConsumeFrom(ctx, root, walk, opts), nil
}

// ConsumeFrom runs the per-key half of the chain against a root and walk
// the caller already has.
//
// **It exists so a caller that wants to report each step as it happens
// does not have to run the chain twice.** The renderer-neutral model
// (`workbench.ConsumeModel`) drives the steps one at a time so a UI can
// name the link that broke; without this seam it called `VerifiedRoot`,
// `Walk`, and then `Consume` — which fetched the manifest a second time,
// verified a second signature, and walked a second trie. Wasteful, and
// worse than wasteful: two verifications were being run and only one
// reported, so a root that changed between them would have been resolved
// silently in favour of whichever the report happened to carry.
func (c *Consumer) ConsumeFrom(ctx context.Context, root VerifiedRoot, walk WalkResult,
	opts ConsumeOpts) Report {

	rep := Report{Root: root, Walk: walk}

	for _, b := range walk.Bindings {
		kr := KeyResult{Key: b.Key, TrieHash: b.Hash}
		if opts.Bodies {
			ent, err := c.Blob(ctx, b.Hash)
			if err != nil {
				kr.Err = err
			} else {
				kr.Bytes = len(ent.Data)
				kr.Type = ent.Type
			}
		}
		if opts.Reconcile && kr.Err == nil {
			ph, url, err := c.PointerFor(ctx, root.Data.Prefix, b.Key)
			switch {
			case err != nil:
				kr.Err = fmt.Errorf("reconcile %s: %w", url, err)
			case ph != b.Hash:
				kr.Err = fmt.Errorf("%w: %s — the signed root commits to %s, the leaf at %s says %s",
					ErrContractMismatch, b.Key, b.Hash, url, ph)
			default:
				kr.PointerHash, kr.Reconciled = ph, true
			}
		}
		rep.Keys = append(rep.Keys, kr)
	}

	if opts.AbsentProbe != "" {
		rep.AbsentProbe = opts.AbsentProbe
		_, err := walk.Lookup(opts.AbsentProbe)
		switch {
		case errors.Is(err, ErrAbsent):
			rep.AbsentCorrect = true
		case err == nil:
			rep.AbsentErr = fmt.Errorf("absent-key control: %q IS committed, so it is the wrong probe",
				opts.AbsentProbe)
		default:
			rep.AbsentErr = err
		}
	}
	return rep
}
