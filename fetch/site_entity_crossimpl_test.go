package fetch_test

// Cross-impl gate for the two layers `G-PIN-4` rests on — the site ENTITY
// bytes and the CHAMP trie root — measured against entity-browser-rust's
// own frozen emission (testdata/crossimpl-rust-site/, their `dev` @
// fbc2c5c). Tier: cross-implementation conformance (TESTING-STRATEGY).
//
// WHY THIS EXISTS. `entitysdk/site.go`'s header says we "hold to
// byte-equivalence with their `to_entity` output". Until this file, that
// was an ASSERTION, not a measurement: the only byte test in the area was
// `TestSiteManifest_DeterministicEncoding` (entitysdk/site_test.go:189),
// which encodes twice and compares — self-consistency, which is green for
// any encoder that is merely stable. The two files that DO carry their
// vectors verbatim (workbench/site_link_crossimpl_test.go,
// site_asset_crossimpl_test.go) cover link classification and asset refs,
// not entity bytes. So the one claim `APP-CONVENTION-SEMANTIC-CONTENT-SITE`
// §9 turns into a ratification gate was the one nothing here measured.
//
// WHAT A GREEN RUN CLAIMS, AND WHAT IT DOES NOT. It claims: given THEIR
// entity, our types reproduce THEIR bytes; and given THEIR binding set,
// our trie builder reproduces THEIR root. It does NOT claim that one
// SOURCE fixture (markdown + frontmatter) lowers to the same entity on
// both sides — that hop has no fixture on either side and is the one
// unmeasured link in G-PIN-4. It also exercises NO chunking: their only
// asset is a 732-byte inline tree entity, well under the §6.1 / A1 16 KiB
// threshold, so the "chunker agreement" caveat in §2 is untouched by
// anything either seat currently holds.
//
// A failure here is ROUTED, not locally corrected (the standing rule for
// their vectors — correcting our side to match theirs without settling
// which is right converts a cohort disagreement into a silent divergence
// with our name on it).

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/store"
	"go.entitychurch.org/entity-core-go/core/tree"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

const (
	rustFixture = "testdata/crossimpl-rust-site"
	rustContent = rustFixture + "/content"
)

func rustTree() string { return filepath.Join(rustFixture, rustPeer) }

// loadContentStore fills a memory content store from their sharded-2-4
// content directory. Their layout, read as-is: content/{hex[0:2]}/{hex[2:4]}/{hex}.
func loadContentStore(t *testing.T) *store.MemoryContentStore {
	t.Helper()
	cs := store.NewMemoryContentStore()
	n := 0
	err := filepath.Walk(rustContent, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		raw, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		var ent entity.Entity
		if derr := ecf.Decode(raw, &ent); derr != nil {
			return nil // not an entity; the fixture holds only entities, but be tolerant
		}
		if _, perr := cs.Put(ent); perr != nil {
			return perr
		}
		n++
		return nil
	})
	if err != nil {
		t.Fatalf("walk their content store: %v", err)
	}
	if n == 0 {
		t.Fatalf("anti-vacuity: loaded zero entities from %s", rustContent)
	}
	return cs
}

// resolveLeaf follows the tree-leaf indirection: the `.bin` served at a
// tree path is a `system/hash` entity whose data is the content hash of
// the real entity. Reading the leaf as the entity is the trap here — it
// decodes cleanly and yields type "system/hash" with a 35-byte body.
func resolveLeaf(t *testing.T, leafPath string) (entity.Entity, bool) {
	t.Helper()
	raw, err := os.ReadFile(leafPath)
	if err != nil {
		t.Errorf("read leaf %s: %v", leafPath, err)
		return entity.Entity{}, false
	}
	var leaf entity.Entity
	if err := ecf.Decode(raw, &leaf); err != nil {
		t.Errorf("decode leaf %s: %v", leafPath, err)
		return entity.Entity{}, false
	}
	var hb []byte
	if err := ecf.Decode(leaf.Data, &hb); err != nil {
		t.Errorf("decode leaf hash %s: %v", leafPath, err)
		return entity.Entity{}, false
	}
	hx := hex.EncodeToString(hb)
	cp := filepath.Join(rustContent, hx[0:2], hx[2:4], hx)
	craw, err := os.ReadFile(cp)
	if err != nil {
		t.Errorf("read content %s (for %s): %v", cp, leafPath, err)
		return entity.Entity{}, false
	}
	var ent entity.Entity
	if err := ecf.Decode(craw, &ent); err != nil {
		t.Errorf("decode content %s: %v", cp, err)
		return entity.Entity{}, false
	}
	return ent, true
}

// TestSiteEntityBytes_MatchRustEmission decodes every site entity in their
// emission through OUR types and re-encodes it, asserting byte equality.
//
// The round trip is its own anti-vacuity arm: `ecf.Decode` into a Go
// struct silently drops any field the struct does not declare (AP49's
// shape), so a dropped field cannot survive the re-encode — it comes back
// as a byte difference, loudly. What it cannot catch is a field BOTH sides
// omit, which is why the source-lowering hop above is called out as
// unmeasured rather than implied.
//
// Collect-then-report, not Fatalf-in-a-loop (AP15): the count a
// short-circuiting sweep yields is a lower bound.
func TestSiteEntityBytes_MatchRustEmission(t *testing.T) {
	var manifests, pages, assets, skipped int
	var failures []string

	err := filepath.Walk(rustTree(), func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".bin") {
			return err
		}
		// system/signature/*.bin is a signature, not a site entity.
		if strings.Contains(p, string(filepath.Separator)+"system"+string(filepath.Separator)) {
			return nil
		}
		ent, ok := resolveLeaf(t, p)
		if !ok {
			return nil
		}
		rel, _ := filepath.Rel(rustTree(), p)

		var re []byte
		switch ent.Type {
		case entitysdk.TypeSiteManifest:
			var m entitysdk.SiteManifest
			if derr := ecf.Decode(ent.Data, &m); derr != nil {
				failures = append(failures, rel+": decode into SiteManifest: "+derr.Error())
				return nil
			}
			b, eerr := ecf.Encode(m)
			if eerr != nil {
				failures = append(failures, rel+": re-encode manifest: "+eerr.Error())
				return nil
			}
			re, manifests = b, manifests+1
		case entitysdk.TypeSitePage:
			var pg entitysdk.SitePage
			if derr := ecf.Decode(ent.Data, &pg); derr != nil {
				failures = append(failures, rel+": decode into SitePage: "+derr.Error())
				return nil
			}
			b, eerr := ecf.Encode(pg)
			if eerr != nil {
				failures = append(failures, rel+": re-encode page: "+eerr.Error())
				return nil
			}
			re, pages = b, pages+1
		default:
			// `app/site-asset` is theirs alone — arch confirmed it is a
			// Stage-2 input and NOT a divergence (entitysdk/site.go
			// reserves `assets/{name}` for the post-v1 passive-Embed
			// work). Counted, not asserted on, so its arrival is visible
			// here the day we do model it.
			if ent.Type == "app/site-asset" {
				assets++
			} else {
				skipped++
			}
			return nil
		}

		if !bytes.Equal(re, ent.Data) {
			failures = append(failures,
				rel+" ("+ent.Type+"): theirs "+hex.EncodeToString(ent.Data)+" / ours "+hex.EncodeToString(re))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk their tree: %v", err)
	}

	// Anti-vacuity: the fixture carries 3 sites (3 manifests) and 11
	// pages. A walk that matched nothing would otherwise report a clean
	// pass — which is exactly how this area went unmeasured for a month.
	if manifests == 0 || pages == 0 {
		t.Fatalf("anti-vacuity: matched %d manifests and %d pages in %s — the walk found no site entities",
			manifests, pages, rustTree())
	}
	t.Logf("their emission: %d %s, %d %s, %d app/site-asset (theirs alone), %d other",
		manifests, entitysdk.TypeSiteManifest, pages, entitysdk.TypeSitePage, assets, skipped)

	for _, f := range failures {
		t.Errorf("site entity bytes diverge — ROUTE, do not correct locally: %s", f)
	}
}

// TestTrieRoot_RebuildsTheirPublishedRoot is the trie half. It walks THEIR
// signed root with our reader, then rebuilds the root from the collected
// bindings in a FRESH content store — so the rebuild constructs every
// CHAMP node from scratch rather than re-finding theirs.
//
// EXTENSION-TREE v4.0 §3.3 states this as the cross-impl convergence
// guarantee (core/tree/trie.go:20 — "byte-identical to any other peer's
// BuildTrie over the same binding set"). This is that guarantee measured
// against a genuinely foreign trie rather than restated.
func TestTrieRoot_RebuildsTheirPublishedRoot(t *testing.T) {
	cs := loadContentStore(t)

	raw, err := os.ReadFile(filepath.Join(rustTree(), "system", "peer", "published-root"))
	if err != nil {
		t.Fatalf("read their published-root: %v", err)
	}
	var ent entity.Entity
	if err := ecf.Decode(raw, &ent); err != nil {
		t.Fatalf("decode published-root: %v", err)
	}
	var prd types.PublishedRootData
	if err := ecf.Decode(ent.Data, &prd); err != nil {
		t.Fatalf("decode PublishedRootData: %v", err)
	}

	bindings := tree.CollectAllBindings(cs, prd.RootHash, "")
	if len(bindings) == 0 {
		t.Fatalf("anti-vacuity: our reader collected zero bindings from their root %s", prd.RootHash)
	}

	// The binding count must equal the number of tree leaves they emitted.
	// Without this, a walk that silently stopped early would still rebuild
	// *a* root and only fail the comparison for an unexplained reason.
	var leaves int
	_ = filepath.Walk(rustTree(), func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if strings.HasSuffix(p, ".bin") && !strings.Contains(p, string(filepath.Separator)+"system"+string(filepath.Separator)) {
			leaves++
		}
		return nil
	})
	if leaves != len(bindings) {
		keys := make([]string, 0, len(bindings))
		for k := range bindings {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		t.Errorf("walked %d bindings but their tree carries %d leaves; collected: %v",
			len(bindings), leaves, keys)
	}

	fresh := store.NewMemoryContentStore()
	bs := make([]tree.Binding, 0, len(bindings))
	for k, h := range bindings {
		bs = append(bs, tree.Binding{Path: k, Hash: h})
	}
	rebuilt, err := tree.BuildTrie(fresh, bs)
	if err != nil {
		t.Fatalf("rebuild trie: %v", err)
	}
	if rebuilt != prd.RootHash {
		t.Errorf("CHAMP root diverges — ROUTE, do not correct locally\n  theirs %s\n  ours   %s",
			prd.RootHash, rebuilt)
		return
	}
	t.Logf("rebuilt their root from %d bindings in a fresh store: %s", len(bindings), rebuilt)
}
