package workbench

// Site tree-path helpers. Mirrors egui-rust src/content_site/paths.rs.
//
// SITE convention v0.5 (APP-CONVENTION-SEMANTIC-CONTENT-SITE):
// a site is a free subgraph at a bare /{peer}/sites/{siteID}/ prefix —
// NOT under content/sites (that namespace belongs to the CONTENT
// extension; v0.5 §2 erratum closed that layer violation).

// SitesSubpath is the publisher convention's first segment under a peer.
const SitesSubpath = "sites"

// SitesPrefix returns the trailing-slash prefix that covers every site
// under a peer. Subscribing here covers any configured landing site
// without having to know which site that is at construction time.
func SitesPrefix(peerID string) string {
	return "/" + peerID + "/" + SitesSubpath + "/"
}

// SitePrefix returns the trailing-slash prefix for everything in one
// site. This subgraph root IS the site's capability scope (v0.5 §7).
func SitePrefix(peerID, siteID string) string {
	return "/" + peerID + "/" + SitesSubpath + "/" + siteID + "/"
}

// ManifestPath returns the path to a site's manifest entity.
func ManifestPath(peerID, siteID string) string {
	return "/" + peerID + "/" + SitesSubpath + "/" + siteID + "/manifest"
}

// PagesPrefix returns the trailing-slash prefix for a site's pages.
func PagesPrefix(peerID, siteID string) string {
	return "/" + peerID + "/" + SitesSubpath + "/" + siteID + "/pages/"
}

// PagePath returns the path to a single page entity within a site.
func PagePath(peerID, siteID, page string) string {
	return "/" + peerID + "/" + SitesSubpath + "/" + siteID + "/pages/" + page
}

// PageFromPath recovers a page slug from a full pages-prefixed path,
// if it belongs to the given site. Returns ("", false) otherwise.
func PageFromPath(peerID, siteID, fullPath string) (string, bool) {
	prefix := PagesPrefix(peerID, siteID)
	if len(fullPath) < len(prefix) || fullPath[:len(prefix)] != prefix {
		return "", false
	}
	return fullPath[len(prefix):], true
}
