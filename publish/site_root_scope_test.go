package publish

// The invariant `G-PIN-4` rests on, and which nothing else in either tree
// states: a site's subtree root is INDEPENDENT OF THE PUBLISHING PEER.
//
// `APP-CONVENTION-SEMANTIC-CONTENT-SITE` §2 makes a site's identity "the
// root hash of its subtree" and §9's G-PIN-4 asks two publishers to emit
// an identical root from one fixture. That is only well-posed if the root
// is a function of the CONTENT and not of who published it — and this
// tree is peer-id-namespaced everywhere, so the opposite is the thing to
// expect until measured. It holds because `tree.BuildTrieForPrefix`
// (core/tree/trie_update.go:20) trims the qualified prefix off every
// entry before binding, so the trie keys are RELATIVE and the peer-id
// never enters the hash.
//
// If this test ever fails, G-PIN-4 is impossible as specified and the
// gate is the thing to route, not our publisher.
//
// It also pins the two consequences a joint run has to be designed
// around, both of which would otherwise surface as an unexplained red:
//   - the comparison must be scoped to the SITE SUBTREE, because peer-scoped
//     roots commit the placement as part of the key. **This bullet used to
//     read "our placement (`content/sites/{id}/`) and theirs (`sites/{id}/`)
//     put the same content under different keys" — filed as a fixture-scoping
//     note, which is how a four-month non-conformance stayed invisible.**
//     It was not a difference to design a joint run around: SITE v0.5 §2
//     drops `content/sites` by name and ours was the wrong one. Fixed
//     2026-09-12; the placements now agree, and the scoping rule below
//     stands on its own (`sites/` is a publisher-CHOSEN path under v0.5, so
//     two publishers may still legitimately differ). AP45: a paragraph
//     explaining a difference closes the question permanently, where a
//     `TODO` would have invited the work.
//   - `site_id` DOES enter the root, via the manifest body. Two publishers
//     using different slugs for "the same" fixture diverge on a field
//     neither is likely to consider part of the content.

import (
	"testing"

	"go.entitychurch.org/entity-core-go/core/crypto"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/tree"

	"entity-workbench-go/entitysdk"
)

// seedSiteRoot authors one fixed site under a seeded identity and returns
// the peer-id together with the trie root scoped to the site subtree.
func seedSiteRoot(t *testing.T, seedByte byte, siteID string) (string, hash.Hash) {
	t.Helper()
	var seed [32]byte
	for i := range seed {
		seed[i] = seedByte
	}
	kp := crypto.FromSeed(seed)
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{Keypair: &kp})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { ap.Close() })

	m := entitysdk.NewSiteManifest(siteID, "Entity Demo Site", "index", []entitysdk.NavItem{
		entitysdk.NewNavLeaf("Home", "/index"),
		entitysdk.NewNavSection("Guide", "/guide/intro", []entitysdk.NavItem{
			entitysdk.NewNavLeaf("Intro", "/guide/intro"),
		}),
	})
	if _, err := ap.PutSiteManifest(siteID, m); err != nil {
		t.Fatalf("put manifest: %v", err)
	}
	// Sorted by construction below, so seeding order cannot be what makes
	// two runs agree — CHAMP insert is permutation-invariant and this
	// keeps the test from depending on that being true.
	pages := []struct{ slug, body string }{
		{"guide/intro", "# Intro\n\nFirst page of the guide.\n"},
		{"index", "# Welcome\n\nHello from the fixture.\n"},
	}
	for _, p := range pages {
		if _, err := ap.PutSitePage(siteID, p.slug, entitysdk.NewMarkdownPage(p.slug, p.body)); err != nil {
			t.Fatalf("put page %s: %v", p.slug, err)
		}
	}

	root, err := tree.BuildTrieForPrefix(ap.RawContentStore(), ap.RawLocationIndex(),
		crypto.PeerID(ap.PeerID()), entitysdk.SitePrefix(ap.PeerID(), siteID))
	if err != nil {
		t.Fatalf("BuildTrieForPrefix: %v", err)
	}
	if root.IsZero() {
		t.Fatalf("anti-vacuity: empty root — nothing was bound under the site prefix")
	}
	return ap.PeerID(), root
}

func TestSiteRootIsIndependentOfThePublishingPeer(t *testing.T) {
	idA, rootA := seedSiteRoot(t, 0xA1, "demo")
	idB, rootB := seedSiteRoot(t, 0xB2, "demo")

	// Anti-vacuity: without this the test passes trivially if both arms
	// were somehow the same peer, which is the only way it could be green
	// for the wrong reason.
	if idA == idB {
		t.Fatalf("anti-vacuity: both arms ran as %s — the variable under test never varied", idA)
	}
	if rootA != rootB {
		t.Fatalf("site root is peer-dependent, so G-PIN-4 is impossible as specified\n  %s -> %s\n  %s -> %s",
			idA, rootA, idB, rootB)
	}
	t.Logf("two identities, one site root: %s", rootA)
}

func TestSiteIDEntersTheSiteScopedRoot(t *testing.T) {
	_, demo := seedSiteRoot(t, 0xC3, "demo")
	_, notes := seedSiteRoot(t, 0xC3, "notes")
	if demo == notes {
		t.Fatalf("site_id does not reach the root; the manifest carries it (entitysdk/site.go:78), "+
			"so this is a change in what the root commits to — root %s", demo)
	}
	t.Logf("site_id reaches the root, so a joint fixture must pin it: demo=%s notes=%s", demo, notes)
}
