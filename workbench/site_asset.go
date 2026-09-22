package workbench

import "strings"

// site_asset.go — the site convention's **assets** subgraph, which this
// repo did not implement at all until 2026-08-31.
//
// # What was missing, and how invisible it was
//
// A site is `/{peer}/sites/{site}/` with `manifest` and `pages/` under
// it. It also has `assets/`, and we had neither the path helpers nor a
// resolver branch nor a renderer for it. On the live federation that is
// not a rounding error: billslab.com's methodology site commits **966
// keys, 665 of which are `sites/billslab-methodology/assets/figures/*`**
// — the figure corpus is two thirds of the site and none of it was
// reachable. An operator looking for the images reported not being able
// to find any image links, which is exactly right: there were none,
// because the images are not links (see site_embed.go).
//
// # AssetNameFromRef is a SECURITY GATE, not a path helper
//
// It decides whether a string written in someone else's page body may
// cause this process to fetch something. Everything it rejects is a way
// to point a renderer at a URL the publisher did not commit:
//
//	https://tracker/x.png   an arbitrary origin — a read receipt for
//	                        every reader of the page
//	//tracker/x.png         the same, protocol-relative
//	/etc/passwd             an absolute path, outside the site subgraph
//	data:...                inline bytes bypassing the tree entirely
//	assets/../../secret     a parent escape out of the site's subgraph
//	figures/x.png           not under assets/ — outside the convention
//
// It is byte-identical with `entity-browser-rust`'s
// `content_site/paths.rs::asset_name_from_ref` by obligation, for the
// same reason [ClassifyTarget] is: a slug that selects bytes in a
// content-addressed tree is Layer-2 algorithm contract, and two impls
// that disagree resolve the same page differently. Its vectors are
// lifted verbatim in site_asset_test.go — **a failure there is routed,
// not locally corrected.**

// AssetsSubpath is the segment under a site that holds its assets.
const AssetsSubpath = "assets"

// AssetsPrefix returns the trailing-slash prefix covering every asset in
// one site.
func AssetsPrefix(peerID, siteID string) string {
	return "/" + peerID + "/" + SitesSubpath + "/" + siteID + "/" + AssetsSubpath + "/"
}

// AssetPath returns the tree path of one named asset. `name` is the
// suffix [AssetNameFromRef] produces — `figures/x.png`, not
// `assets/figures/x.png`.
func AssetPath(peerID, siteID, name string) string {
	return AssetsPrefix(peerID, siteID) + name
}

// AssetNameFromRef maps an embed's `ref` to the site-local asset name,
// or reports false if the ref is not a resolvable site-local asset.
//
// See the file note: this is the gate that stops a page body from
// steering the renderer at an arbitrary URL. It refuses rather than
// sanitizes — there is no useful repair of `https://tracker/x.png` into
// something the publisher committed, and a "cleaned up" version of a
// hostile ref is a hostile ref that now looks legitimate (AP33).
func AssetNameFromRef(ref string) (string, bool) {
	r := strings.TrimSpace(ref)
	if r == "" ||
		strings.Contains(r, "://") ||
		strings.HasPrefix(r, "//") ||
		strings.HasPrefix(r, "/") ||
		strings.HasPrefix(r, "data:") {
		return "", false
	}
	for _, seg := range strings.Split(r, "/") {
		if seg == ".." {
			return "", false
		}
	}
	name, ok := strings.CutPrefix(r, AssetsSubpath+"/")
	if !ok || name == "" {
		return "", false
	}
	return name, true
}

// SiteAsset is one embedded file in a site's assets subgraph.
//
// The bytes are held as-is: an asset is opaque to this layer, which is
// the point of it being content-addressed. `MediaType` is the
// publisher's declaration and is a HINT for a renderer choosing a
// decoder — it is not verified against the bytes and must not be treated
// as if it were.
type SiteAsset struct {
	MediaType string `cbor:"media_type"`
	Bytes     []byte `cbor:"bytes"`
}

// TypeSiteAsset is the entity type an asset is stored under.
const TypeSiteAsset = "site/asset"

// MediaTypeForName is the extension→media-type table, used when a
// publisher declared none. Deliberately small and closed: a renderer
// that guesses widely will eventually guess a type it then hands to a
// decoder, and "we could not identify this" is a better answer than a
// confident wrong one.
func MediaTypeForName(name string) string {
	i := strings.LastIndex(name, ".")
	if i < 0 {
		return ""
	}
	switch strings.ToLower(name[i+1:]) {
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	case "bmp":
		return "image/bmp"
	case "svg":
		return "image/svg+xml"
	default:
		return ""
	}
}
