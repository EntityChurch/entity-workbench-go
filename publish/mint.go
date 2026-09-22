package publish

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/store"
	"go.entitychurch.org/entity-core-go/core/tree"

	"entity-workbench-go/entitysdk"
)

// mint.go — the publishing ACT, separated from the static PROJECTION.
//
// # Why this file exists
//
// Until 2026-09-12 the only way to mint a signed published-root in this
// tree was [Publish], which also walks a closure and writes a directory.
// That was fine while the CDN corridor was the only consumer. It stopped
// being fine the moment a peer could serve its own site by dispatch
// (W2, `workbench.PeerSource`): a live-only publisher needs the signed
// root in its tree and has no use for an output directory, and the
// cheapest way to give it one would have been a second mint.
//
// **A second way to produce the artifact the whole trust argument hangs
// off is the thing that must not exist.** `publish/live_and_static_test.go`
// says so in its header and it is the reason this is an extraction rather
// than a new function: [MintRoot] and [Publish] run the same code, past
// the same refusal, and the only difference is whether a directory comes
// out of the other end.
//
// # The refusal is shared on purpose
//
// [prepareMint] carries the empty-prefix guard. It was in `Publish` when
// it landed (AP97 — `mintSignedRoot`'s own guard could never fire), and
// leaving it there while adding a second entry point would have re-opened
// exactly the defect it was written for, on the newer of the two roads.
// A guard that protects one caller of two is a guard with a hole in it.

// MintOpts is the publishing act's whole input.
//
// Deliberately not [Opts]: the emit-side fields (OutputDir, OriginURL,
// the filter hooks) have no meaning here, and a shared struct whose
// fields are ignored by half its callers is how a filter ends up
// silently not applied.
type MintOpts struct {
	// Peer is the publisher. Its keypair signs the root and its
	// peer-id is the key a consumer verifies against.
	Peer *entitysdk.AppPeer

	// Prefix is the peer-relative tree prefix the root will commit to,
	// e.g. `sites/`. Empty means the whole tree, which is legal and is
	// almost never what an operator means.
	Prefix string

	// At pins `published_at`. Zero means time.Now(). See Opts.At for
	// why a caller producing a reproducible emission must set it.
	At time.Time

	// AllowWholePeer is the operator's explicit acknowledgement that
	// this publish commits to `system/` keys as well as application
	// ones. See [disclosureAcrossSystem] — absent it, a SCANNED prefix
	// that spans the boundary is REFUSED rather than published quietly.
	//
	// It has no meaning for a curated [Content] set and is not consulted
	// for one; see [disclosureAcrossSystem] for why the guard is about
	// scanning rather than about the prefix's spelling.
	AllowWholePeer bool

	// Content chooses WHICH bindings under Prefix the root commits to.
	// nil is the prefix scan — see content_set.go, and `A-38`'s ruling
	// (D) for why a prefix is a bound rather than a content set.
	Content *ContentSet
}

// MintRoot performs the publishing act and stops there: build the trie
// over the prefix, sign a `system/peer/published-root` over its root
// hash, bind the signature and then the root.
//
// This is what a LIVE publisher needs and all it needs. After this
// returns, any peer authorized to read `system/peer/published-root`,
// `system/signature/*` and the prefix can run the identical verification
// chain a static consumer runs — same signature, same seq floor, same
// fail-closed walk. What it does NOT do is make anybody authorized to do
// that, or make this peer reachable; both are the caller's, because both
// are decisions about disclosure rather than steps in an encoding.
//
// [Publish] calls the same two helpers in the same order and then emits
// the static corridor on top.
func MintRoot(ctx context.Context, opts MintOpts) (SignedRoot, error) {
	entries, err := prepareMint(opts.Peer, opts.Prefix, opts.AllowWholePeer, opts.Content)
	if err != nil {
		return SignedRoot{}, err
	}
	at := opts.At
	if at.IsZero() {
		at = time.Now()
	}
	signed, err := mintSignedRoot(opts.Peer, opts.Prefix, entries, at)
	if err != nil {
		return SignedRoot{}, err
	}
	signed.Bindings = len(entries)
	signed.ContentSet = contentSetName(opts.Content)
	return signed, nil
}

// contentSetName is "" for the prefix scan and the set's name otherwise.
func contentSetName(set *ContentSet) string {
	if set == nil {
		return ""
	}
	return set.Name
}

// RootNow computes what a root over `prefix` WOULD commit to if it were
// minted this instant, and signs, binds and publishes nothing.
//
// # Why a surface needs this
//
// A published root commits to a trie root taken at the moment of the
// publish. Every write under the prefix afterwards is invisible to it —
// **not stale by a clock, but absent from the commitment** — so a
// consumer on either road gets the old set and cannot tell that from a
// publisher who has not posted. `publish status` re-derives the BINDING
// COUNT for that reason and the count is the weaker signal: it moves
// only when keys are added or removed, and says nothing when an existing
// key's bytes change (an edited page, a rewritten index head).
//
// Comparing this against [types.PublishedRootData.RootHash] is the whole
// question *"does what I published still describe what I have"*, and it
// is the fact a feed needs most: one post rewrites the index head, which
// changes no count at all.
//
// It is a pure read — the same call [mintSignedRoot] makes — so a status
// surface may run it on a refresh where minting would be a publisher
// claiming a release every time somebody looks.
// ⚠ **`content` MUST be the set the live root was minted with**, or this
// answers a question about a different publish. A curated root and a scanned
// root over the same prefix have different hashes by design, so re-deriving
// with the wrong one reports *"your root is behind"* on a root that is exactly
// current — permanently, since nothing the operator does can make the two
// agree. That is a standing false line in a problems list, which is how an
// operator learns to stop reading the list. The published root does **not**
// record how its set was chosen, so the caller has to; `shellcmd` keeps it in
// [ShellWorkspace.currentPublishContentSet].
func RootNow(ap *entitysdk.AppPeer, prefix string, content *ContentSet) (hash.Hash, error) {
	if ap == nil {
		return hash.Hash{}, fmt.Errorf("publish: Peer required")
	}
	entries := entitysdk.ListEntriesSorted(ap.RawLocationIndex(), prefix)
	chosen, err := applyContentSet(ap, content, entries)
	if err != nil {
		return hash.Hash{}, err
	}
	root, err := tree.BuildTrie(ap.RawContentStore(), trieBindings(ap.PeerID(), prefix, chosen))
	if err != nil {
		return hash.Hash{}, fmt.Errorf("publish: build trie for prefix %q: %w", prefix, err)
	}
	return root, nil
}

// prepareMint lists what the prefix binds and refuses to sign nothing.
//
// AN EMPTY PREFIX PUBLISHES A SIGNED ROOT THAT COMMITS TO NOTHING, and
// until 2026-09-12 this is exactly what it did.
//
// `mintSignedRoot` has a refusal for this case — `if trieRoot.IsZero()`
// — and it **cannot fire**: `tree.BuildTrieForPrefix` over a prefix with
// no bindings returns the hash of the canonical *empty* CHAMP node,
// which is a perfectly good non-zero hash (measured: the same
// `ecf-sha256:6a22fe73…` under two different identities, since it is
// content-addressed and the peer-id never enters a trie key). So a
// mistyped `-prefix` emitted a well-formed, correctly-signed, completely
// empty origin.
//
// **The failure mode is the one this package works hardest to avoid.**
// An empty root answers "absent" to every key with a valid signature
// over it, which is indistinguishable from a large site nobody asked the
// right question of — `fetch.ErrEmptyEnumeration` exists precisely
// because absence carries no information until something has been
// enumerated. The publisher is the one party that can tell the two apart
// for free, because it knows it bound nothing.
//
// Checked on the binding count rather than on the root hash: the count
// is the fact, and a guard that compares against a magic empty-node hash
// would be a second thing to keep true.
//
// # The refusal NAMES WHAT IS THERE, and that is not politeness
//
// "prefix %q has no bindings" is true and is a confident wrong diagnosis
// in the case an operator actually hits. Measured by running the flow
// (AP71): `entity-shell` defaults to an in-memory store, so
// `-identity NAME` loads the keypair, produces the right peer-id, and
// opens an EMPTY tree — and a message that ends *"check the prefix"*
// sends someone to re-read a prefix that was correct, on a peer whose
// whole tree is empty. AP44's shape: a refusal that asserts a fact about
// the world reads as rigor and leaves no wrong answer to catch.
//
// Reporting the tree's own top-level segments separates the two cases at
// a glance — *"you have `archives/` and `doc/`, not `sites/`"* is a
// prefix mistake, and *"this peer's tree has no bindings at all"* is a
// different machine or a different store. Same discipline as
// `fetch.NameSet.Diagnosis` and `TransportOptions.Offered`: when you
// refuse, say what was on offer.
func prepareMint(peer *entitysdk.AppPeer, prefix string, allowWholePeer bool, content *ContentSet) ([]store.LocationEntry, error) {
	if peer == nil {
		return nil, fmt.Errorf("publish: Peer required")
	}
	entries := entitysdk.ListEntriesSorted(peer.RawLocationIndex(), prefix)
	if len(entries) == 0 {
		return nil, fmt.Errorf(
			"publish: prefix %q has no bindings, so there is nothing to publish — "+
				"signing a root over it would emit a valid, correctly-signed origin that answers "+
				"\"absent\" to every key, which a consumer cannot tell from a site it asked the "+
				"wrong question of. %s",
			prefix, treeShape(peer))
	}

	// THE DISCLOSURE GUARD IS ABOUT SCANNING, NOT ABOUT THE PREFIX, and
	// `A-38`'s ruling is what makes the distinction expressible. It exists
	// because deriving the content set from the prefix SWEEPS ALONG every
	// key the operator did not name — which for a peer-root publish is
	// their mount paths, their peers' addresses and their documents. A
	// curated set sweeps nothing: every member is there because the
	// selector named it, so there is no second category to disclose and
	// `-whole-peer` would be asking an operator to acknowledge a cost the
	// ruling removed.
	//
	// Ordered before the selector on purpose: the guard reads the SCAN,
	// because the scan is the thing it is about.
	if !allowWholePeer && content == nil {
		if d := disclosureAcrossSystem(peer, entries); d != "" {
			return nil, fmt.Errorf("publish: %s", d)
		}
	}

	chosen, err := applyContentSet(peer, content, entries)
	if err != nil {
		return nil, err
	}
	if len(chosen) == 0 {
		// Same refusal as the empty prefix and for the identical reason —
		// a root over nothing answers "absent" to every key under a valid
		// signature. Reached by a different route: the prefix holds
		// bindings and the set chose none of them, which on a feed means
		// the operator has not posted yet.
		return nil, fmt.Errorf(
			"publish: prefix %q holds %d bindings and the %s content set selected none of them, "+
				"so there is nothing to publish — signing a root over an empty set would emit a "+
				"valid, correctly-signed origin that answers \"absent\" to every key, which a "+
				"consumer cannot tell from one it asked the wrong question of",
			prefix, len(entries), content.Name)
	}
	return chosen, nil
}

// disclosureAcrossSystem returns a refusal when a prefix spans the
// system boundary, or "" when it does not.
//
// # Why this guard exists and why it is not `prefix == ""`
//
// `A-36` (arch, ruled 2026-09-15) says a feed's detached per-entry
// signatures have to be inside the committed key set for a STATIC reader
// to attribute anything, and the prescribed fix is to publish over the
// peer root — because `system/signature/…` and `app/feed/…` share no
// shorter prefix. That fix works (`a36_peer_root_probe_test.go`). It
// also takes this peer's committed key set from 4 to 386 and its static
// emit from 7 entities to 400, and the emitted directory then contains
// the operator's mount paths, other peers' addresses and the bodies of
// documents from folders nobody shared. **On the static road there is no
// grant between that directory and the public** — the operator's next
// action is an upload.
//
// So the refusal is not *"don't do that"*. It is **say it out loud
// first**: a publish that discloses the peer's private declarations is a
// legitimate thing to want and an illegitimate thing to do by typing an
// empty string. `AllowWholePeer` is the operator saying it.
//
// **Keyed on spanning the boundary rather than on the prefix's spelling**
// for the reason AP97 taught: `mintSignedRoot`'s empty-prefix guard was
// written against the root hash, which is never zero, so it could never
// fire. Guard on the fact — *this publish commits to `system/` keys AND
// to application keys* — not on a proxy for it. A deliberate
// `-prefix system/config/` names one side and is not caught; nothing is
// being protected from an operator who typed the thing they meant.
func disclosureAcrossSystem(peer *entitysdk.AppPeer, entries []store.LocationEntry) string {
	bySeg := map[string]int{}
	for _, e := range entries {
		for _, s := range strings.Split(e.Path, "/") {
			if s == "" || s == peer.PeerID() {
				continue
			}
			bySeg[s]++
			break
		}
	}
	system := bySeg["system"]
	if system == 0 {
		return ""
	}
	var app []string
	appKeys := 0
	for seg, n := range bySeg {
		if seg == "system" {
			continue
		}
		app = append(app, fmt.Sprintf("%s (%d)", seg, n))
		appKeys += n
	}
	if len(app) == 0 {
		// Only `system/`. The operator named it; there is no second
		// category being swept along, so there is nothing to disclose
		// that the prefix did not already say.
		return ""
	}
	sort.Strings(app)

	return fmt.Sprintf(
		"this prefix spans the system boundary, so the signed root would commit to %d keys under "+
			"`system/` as well as %d application keys (%s) — and every one of them is emitted into "+
			"the output directory, where the next step is an upload.\n\n"+
			"On this peer that means the published set includes whatever these hold: peer "+
			"declarations and their last-known addresses, folder declarations and their local "+
			"filesystem paths, ingested documents from mounted folders, and the capability policy "+
			"table naming who has been granted what.\n\n"+
			"A verifying reader also needs no further authorization for any of it: the walk is "+
			"content-addressed, so committing to a key discloses its bytes.\n\n"+
			"If that is what you mean, say so — `-whole-peer` — and it will publish. If you are "+
			"here because a feed's per-entry signatures are not in the committed set, that is "+
			"`A-36`, it is real, and widening the prefix is the ruled fix whose cost this message "+
			"is; the narrow prefix is still `-prefix app/feed/`",
		system, appKeys, strings.Join(app, ", "))
}

// treeShape describes what the peer's tree DOES hold, as the second half
// of the refusal above.
//
// Application-level segments first and `system/` counted rather than
// listed: every peer has a populated `system/` from construction, so
// leading with it would make an empty application tree look full —
// which is precisely the distinction this sentence exists to draw.
func treeShape(peer *entitysdk.AppPeer) string {
	all := entitysdk.ListEntriesSorted(peer.RawLocationIndex(), "")
	seen := map[string]bool{}
	var segs []string
	system := 0
	for _, e := range all {
		// The index returns peer-qualified paths on a namespaced store and
		// bare ones otherwise, so take the first segment that is neither
		// empty nor this peer's id rather than assuming either shape.
		for _, s := range strings.Split(e.Path, "/") {
			if s == "" || s == peer.PeerID() {
				continue
			}
			if s == "system" {
				system++
			} else if !seen[s] {
				seen[s] = true
				segs = append(segs, s)
			}
			break
		}
	}
	switch {
	case len(all) == 0:
		return "This peer's tree is EMPTY — not just the prefix. Most likely this process is " +
			"running on a different store than the one that holds the site: `entity-shell` " +
			"defaults to an in-memory store, so `-identity NAME` alone gives you the right " +
			"peer-id and a blank tree. Try `-storage sqlite`."
	case len(segs) == 0:
		return fmt.Sprintf("This peer's tree holds %d bindings and every one of them is under "+
			"`system/` — nothing has been authored on it yet.", system)
	default:
		sort.Strings(segs)
		return fmt.Sprintf("This peer's tree holds these top-level prefixes: %s (plus %d under "+
			"`system/`). The prefix is peer-relative, e.g. %q.",
			strings.Join(segs, ", "), system, entitysdk.SitesSubpath+"/")
	}
}
