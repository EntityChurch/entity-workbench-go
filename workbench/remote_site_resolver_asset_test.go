package workbench

import (
	"testing"

	"go.entitychurch.org/entity-core-go/core/hash"
)

// remote_site_resolver_asset_test.go — Tier 1 gate on the WIRING, which
// the grammar tests in site_ref_test.go deliberately do not cover.
//
// [ClassifyAssetRef] decides where an asset would be; this asserts the
// resolver then looks it up at that address, out of the committed key
// set, for every one of the reference forms. Those are two different
// failures — a correct classification routed to the wrong key resolves
// to nothing and reports as *the publisher did not commit this*, which
// is a diagnosis pointing at the wrong machine.
//
// The resolver is built by hand rather than through
// [NewRemoteSiteResolver] so the key set is the fixture. Bodies are
// seeded into the blob cache, which is the one path that does not need a
// [fetch.Consumer] — the network is not what is under test here.

func assetFixtureResolver(t *testing.T, peer string, bodies map[string][]byte) *RemoteSiteResolver {
	t.Helper()
	r := &RemoteSiteResolver{
		peerID: peer,
		keys:   map[string]hash.Hash{},
		cache:  map[hash.Hash][]byte{},
	}
	for rel, body := range bodies {
		h, err := hash.OfBytes(hash.AlgorithmSHA256, body)
		if err != nil {
			t.Fatalf("hashing the fixture body for %q: %v", rel, err)
		}
		r.keys[rel] = h
		r.cache[h] = body
		r.sorted = append(r.sorted, rel)
	}
	return r
}

// TestRemoteResolveAsset_EveryReferenceFormReachesTheCommittedBytes is
// the end of W0: four spellings, one figure, bytes out of the signed key
// set.
//
// It also pins the boundary the resolver enforces, which is NOT the site
// — it is the publisher. `r.keys` is one peer's completed walk and it
// covers every site under that peer's root, so a same-peer cross-site
// ref is inside the signature the chain on screen already describes.
func TestRemoteResolveAsset_EveryReferenceFormReachesTheCommittedBytes(t *testing.T) {
	const peer = "PUBLISHER"
	figure := []byte("\x89PNG\r\n\x1a\n-- pretend this is a figure --")
	sibling := []byte("\x89PNG\r\n\x1a\n-- a figure in the other site --")

	r := assetFixtureResolver(t, peer, map[string][]byte{
		relPath(AssetPath(peer, "lab", "figures/x.png"), peer):        figure,
		relPath(AssetPath(peer, "other-site", "figures/y.png"), peer): sibling,
	})
	cur := Location{PeerID: peer, SiteID: "lab", Page: "research/model/grounding"}

	for _, c := range []struct {
		name string
		ref  string
		want []byte
	}{
		{"relative", "assets/figures/x.png", figure},
		{"root-absolute", "/assets/figures/x.png", figure},
		{"site: — a sibling site under the same signed root", "site:other-site/assets/figures/y.png", sibling},
		{"entity+ref:// naming this publisher", refScheme() + peer + "/sites/lab/assets/figures/x.png", figure},
		{"entity:// legacy form", "entity://" + peer + "/sites/lab/assets/figures/x.png", figure},
	} {
		t.Run(c.name, func(t *testing.T) {
			a, ok := r.ResolveAsset(cur, c.ref)
			if !ok {
				t.Fatalf("ResolveAsset(%q) found nothing — the key set commits it", c.ref)
			}
			if string(a.Bytes) != string(c.want) {
				t.Errorf("got %d bytes, want %d — the ref routed to the wrong key",
					len(a.Bytes), len(c.want))
			}
			// The media type is derived from the NAME, so a wrong name
			// that happened to hit a committed key would still show here.
			if a.MediaType != "image/png" {
				t.Errorf("media type = %q; want image/png", a.MediaType)
			}
		})
	}
}

// TestRemoteResolveAsset_RefusesWhatItCannotVouchFor is the negative
// half, and the two entries are refusals for DIFFERENT reasons that must
// both keep firing.
func TestRemoteResolveAsset_RefusesWhatItCannotVouchFor(t *testing.T) {
	const peer = "PUBLISHER"
	figure := []byte("\x89PNG\r\n\x1a\nfigure")
	r := assetFixtureResolver(t, peer, map[string][]byte{
		relPath(AssetPath(peer, "lab", "figures/x.png"), peer): figure,
	})
	cur := Location{PeerID: peer, SiteID: "lab", Page: "index"}

	// Another publisher. This resolver is bound to one completed walk
	// and will not answer for a root it never verified — a reachability
	// answer, not a judgement about the reference.
	if _, ok := r.ResolveAsset(cur, refScheme()+"SOMEONE-ELSE/sites/lab/assets/figures/x.png"); ok {
		t.Error("resolved an asset for a peer this resolver has not verified a root for")
	}
	// A tracking URL. Still refused, now by the grammar rather than by
	// the leading-slash arm that used to be catching legal forms with it.
	if _, ok := r.ResolveAsset(cur, "https://tracker.example/x.png"); ok {
		t.Error("resolved an external URL — a page body must not be able to make this process fetch one")
	}
	// Committed, and not addressed by this ref: the key set is the
	// authority, so a well-formed ref to something nobody published
	// resolves to nothing rather than to the nearest match.
	if _, ok := r.ResolveAsset(cur, "assets/figures/not-published.png"); ok {
		t.Error("resolved an asset the signed key set does not commit")
	}
}
