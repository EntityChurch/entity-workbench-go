package publish

import (
	"fmt"
	"strings"

	"go.entitychurch.org/entity-core-go/core/store"
	"go.entitychurch.org/entity-core-go/core/tree"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

// content_set.go — WHICH of a peer's bindings a signed root commits to,
// as a decision separate from the PREFIX the root declares.
//
// # A-38, ruled (D): move the PREFIX, do not move the CONTENT
//
// `A-36` required a feed's `FEED-R2` detached signatures to be inside the
// committed key set, and the only prefix containing both `app/feed/…` and
// `system/signature/…` is the peer's own namespace. We built that, measured
// it, and did not ship it: the committed set went from **4 keys to 386** and
// the static emit from 7 entities to 400, putting an operator's mount paths,
// another peer's LAN address and an unshared document's body into the
// directory whose next step is an upload (`a36_peer_root_probe_test.go`).
//
// We filed that as `A-38` offering three answers. Arch ruled **none of them**,
// on landed text, and the ruling is better than the ask
// (`entity-system-architecture` `ROUTING-2026-09-16-a-…-both-a38s-are-ruled`
// §1):
//
//	⇒ Publish at `prefix: "/{peer_id}/"`, and publish a binding set of
//	  exactly the entries, the index head and pages, and the
//	  `system/signature` entities held for those entries.
//
// **Our three answers all rested on one premise — that widening the prefix
// widens what is published — and `EXTENSION-TREE` §3.3a says otherwise three
// separate times:** the prefix *"bounds the publication's scope; it is NOT a
// completeness claim"*, a publisher *"MAY declare `/{peer_id}/` and publish a
// small subset"*, and the negative-scoping `[MUST]` lets one *"serve different
// subsets to different audiences from the same prefix"*. A completeness `[MUST]`
// that would have made our premise true landed in v4.1 and was **withdrawn in
// full**.
//
// ⭐ **So the 4 → 386 was never a cost of the ruling. It was the cost of
// DERIVING THE CONTENT SET FROM THE PREFIX**, which is what this file stops
// doing.
//
// # Why the defect survived review for as long as it did
//
// Nothing in this package ever chose a binding set. `tree.BuildTrieForPrefix`
// takes a prefix and lists the location index under it, and that is the only
// thing the reference publish helper does — `tree.BuildTrie(cs, bindings)`
// takes an explicit list and is exported, but core-go's publish path and its
// `root_tracker` both reach for the scanning form. **The easy helper
// implements the reading the ruling rejects**, which is the strongest
// available evidence that the missing thing was a sentence rather than care.
// Arch routed that to `entity-core-go` in the same round
// (`ROUTING-2026-09-16-b-entity-core-go-…`); it is not ours to fix.
//
// # The shape
//
// A [ContentSet] selects from the bindings **under the declared prefix** and
// never from outside it. That is structural rather than checked: a key outside
// the prefix has no `relative_key` under §3.3a, so a selector that could reach
// one would be able to build a trie whose keys reconstruct to paths the root
// does not describe. Selecting from a list that is already bounded makes that
// unrepresentable.
//
// A nil ContentSet means **every binding under the prefix** — the scan, which
// is what a site publish wants and what this package did exclusively until
// this ruling. It is still the right default: a site's prefix *is* its
// content, and the two only come apart when the artifact and its evidence are
// placed by different authorities.

// ContentSet chooses which of the publisher's own bindings the signed root
// commits to, within the bound its prefix declares.
//
// A nil *ContentSet is the prefix scan. See the file header for why that
// stays the default rather than becoming a legacy mode.
type ContentSet struct {
	// Name is what a surface calls this set when it reports a publish.
	// An operator reading `publish` output has no other way to tell a
	// curated set from a scan, and the difference is the whole of what
	// the root discloses.
	Name string

	// Select narrows the bindings under the prefix to the ones the root
	// will commit to. It receives them sorted and must return a subset;
	// returning an entry that was not offered is a programming error and
	// is refused by [applyContentSet] rather than silently published.
	Select func(peer *entitysdk.AppPeer, underPrefix []store.LocationEntry) ([]store.LocationEntry, error)
}

// FeedContent is Ruling (D)'s binding set for a feed publish: the entries,
// the index head and its pages, and the `system/signature` entity held for
// each entry — and nothing else the peer happens to hold.
//
// # Why the members are chosen by PATH here and by TYPE in a reader
//
// `workbench.ReadFeed`'s rule-6 fallback filters by **type tag**, deliberately
// and for `APP-CONVENTION-FEED` §2's stated reason: where entries live is an
// implementation's choice, so a reader that scans a path prefix finds another
// implementation's feed empty and calls that an absence.
//
// **This is the author, and the author is the authority on where it put its
// own artifacts.** `FeedIndexPath` and `FeedEntryPrefix` are this
// implementation's own constants; selecting by them asks a question that
// cannot be wrong, where decoding 386 entities to re-derive a fact we already
// hold would cost a store read per binding to learn something we wrote.
//
// The signatures are the half that cannot be chosen by either path or type
// without the entries first: `system/signature/{hex(H)}` is keyed on the
// **entry's** content hash (V7 §3.5), so the set is derived from the entry
// bindings and from nothing else. That is also what keeps it honest — a
// signature over anything this feed does not publish is not reachable from
// here, so the selector cannot widen by accident.
func FeedContent() *ContentSet {
	return &ContentSet{
		Name: "feed (entries, index, and the signature held for each entry)",
		Select: func(peer *entitysdk.AppPeer, underPrefix []store.LocationEntry) ([]store.LocationEntry, error) {
			peerID := peer.PeerID()

			// Index the offered set by its peer-relative key so the
			// signature lookup is a map hit rather than a second pass,
			// and — the part that matters — so a signature is admitted
			// ONLY when it is already under the declared prefix.
			byKey := make(map[string]store.LocationEntry, len(underPrefix))
			for _, e := range underPrefix {
				byKey[peerRelative(peerID, e.Path)] = e
			}

			var out []store.LocationEntry
			var entries []store.LocationEntry

			for _, e := range underPrefix {
				key := peerRelative(peerID, e.Path)
				switch {
				case key == entitysdk.FeedIndexPath,
					strings.HasPrefix(key, entitysdk.FeedIndexPath+"/"):
					// The head and its pages. Note the separator is
					// load-bearing: a bare-name prefix test would also
					// admit `app/feed/index-of-something-else`.
					out = append(out, e)
				case strings.HasPrefix(key, entitysdk.FeedEntryPrefix):
					out = append(out, e)
					entries = append(entries, e)
				}
			}

			// FEED-R2's detached signature, per entry. A missing one is
			// NOT an error here: `FEED-R4` makes an unattributed entry a
			// state a reader must be able to present, so refusing to
			// publish would convert a soft reader-side outcome into a
			// hard publisher-side one for an entry that is otherwise
			// perfectly valid.
			for _, e := range entries {
				if sig, ok := byKey[types.LocalSignaturePath(e.Hash)]; ok {
					out = append(out, sig)
				}
			}

			return out, nil
		},
	}
}

// applyContentSet narrows the prefix scan to the set the caller chose, and
// refuses a selector that returned something it was not offered.
//
// The refusal is not defensive tidiness. A binding outside the declared
// prefix has no `relative_key`, so it would be committed under a key that
// reconstructs to a path the root does not describe — a consumer following
// §3.3a would rebuild an absolute path that names bytes nobody published,
// with a valid signature over it. That is unrepresentable by construction for
// every selector in this file and would stop being so the moment one of them
// started reading the index itself.
func applyContentSet(peer *entitysdk.AppPeer, set *ContentSet, underPrefix []store.LocationEntry) ([]store.LocationEntry, error) {
	if set == nil || set.Select == nil {
		return underPrefix, nil
	}
	chosen, err := set.Select(peer, underPrefix)
	if err != nil {
		return nil, fmt.Errorf("publish: select content set %q: %w", set.Name, err)
	}
	offered := make(map[string]bool, len(underPrefix))
	for _, e := range underPrefix {
		offered[e.Path] = true
	}
	for _, e := range chosen {
		if !offered[e.Path] {
			return nil, fmt.Errorf(
				"publish: content set %q selected %q, which is not under the declared prefix — "+
					"a key outside the prefix has no relative_key under EXTENSION-TREE §3.3a, so "+
					"committing to it would sign a trie whose keys reconstruct to paths the root "+
					"does not describe", set.Name, e.Path)
		}
	}
	return chosen, nil
}

// peerRelative strips this peer's namespace from a path the location index
// returned, and is safe on a path that never carried one.
//
// AP58 is the reason this is not `strings.TrimPrefix`: `Store.List` returns
// PEER-QUALIFIED paths on a namespaced index and bare relative ones on the
// `NewStore(memory, memory)` scaffolding every model test uses, so a trim
// against either spelling is silently a no-op against the other — and the
// arithmetic after it gets a confidently wrong answer rather than an error.
// Splitting on the namespace answers correctly for both shapes.
func peerRelative(peerID, path string) string {
	ns, bare := store.SplitNamespace(path)
	if ns == peerID {
		return bare
	}
	return path
}

// trieBindings turns location entries into the (relative_key → hash) pairs
// `tree.BuildTrie` commits to.
//
// ⚠ **This mirrors `core/tree/trie_update.go`'s `BuildTrieForPrefix` exactly,
// including the cases that look like bugs**, because the equivalence between
// the two is the property `TestMint_ACuratedSetOverTheWholePrefixIsTheScan`
// asserts — and that test is what says this refactor changed no existing
// publish. Two specifics worth naming so nobody "fixes" them into a
// divergence:
//
//   - the prefix is qualified with the local peer-id only when it is
//     RELATIVE; an already-absolute prefix is used as given;
//   - an entry whose relative key comes out empty is SKIPPED, which is how
//     the binding at the prefix path itself stays out of its own trie.
//
// The trie is permutation-invariant (CHAMP-canonical insert), so the order
// this returns cannot move the root hash — which is what lets a curated set
// append its signatures after its entries without sorting them back in.
func trieBindings(peerID, prefix string, entries []store.LocationEntry) []tree.Binding {
	qualified := prefix
	if !strings.HasPrefix(prefix, "/") {
		qualified = store.QualifyPath(peerID, prefix)
	}
	var out []tree.Binding
	for _, e := range entries {
		rel := strings.TrimPrefix(e.Path, qualified)
		if rel == "" {
			continue
		}
		out = append(out, tree.Binding{Path: rel, Hash: e.Hash})
	}
	return out
}
