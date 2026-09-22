// Content-site entity types — `app/site-manifest` and `app/site-page`.
//
// Cross-impl contract: APP-CONVENTION-SEMANTIC-CONTENT-SITE — the
// entity bodies are v0.4.2's (locked); the PLACEMENT is v0.5's, which
// this file was four months late to (see SitesSubpath).
// Wire-bytes peer is `entity-browser-rust`'s `src/content_site/format.rs`;
// we hold to byte-equivalence with their `to_entity` output so a site
// published from workbench-go renders identically when fetched by their
// consumer over HTTP-poll. **That claim is measured rather than asserted
// as of 2026-09-09** — `fetch/site_entity_crossimpl_test.go` decodes
// their frozen emission through these types and re-encodes it
// byte-identically. It sat here as prose for months (AP83).
//
// Encoding: ECF (deterministic CBOR, RFC 8949 §4.2, length-then-lex map
// key order). Optional fields use `,omitempty` so the wire matches the
// "emit only when present" discipline both impls follow:
//   - SiteManifest.Nav / Params — omitted when empty
//   - SitePage.Frontmatter      — omitted when empty
//   - NavItem.Target            — omitted when "" (section header, per
//     spec `nav-node.? target`)
//   - NavItem.Children          — omitted when empty (back-compat with
//     flat-nav wire shape)
//
// Path layout — a free subgraph at the convention's reserved first
// segment (v0.5 §2; peer-id substituted):
//
//	/{peer_id}/sites/{site_id}/manifest
//	/{peer_id}/sites/{site_id}/pages/{slug}
//
// See SitesSubpath for what this used to be and what it cost. The
// helpers are SitePrefix / SiteManifestPath / SitePagePath below;
// `assets/{name}` is reserved for the post-v1 passive-Embed work and is
// not exposed here yet.
package entitysdk

import (
	"fmt"

	"go.entitychurch.org/entity-core-go/core/hash"
)

// Type tags — final per v0.4.2 §4 / F-8.
const (
	TypeSiteManifest = "app/site-manifest"
	TypeSitePage     = "app/site-page"
)

// DefaultRootPage is the landing-page slug used when a manifest declares
// no `params.root`. Convention default; see SiteManifest.Root.
const DefaultRootPage = "index"

// DefaultPageFormat is the page-body base format used when SitePage.Format
// is unset on decode (post-v1 readers MUST tolerate a missing format and
// treat it as markdown, per egui's `page_format_defaults_to_markdown`).
const DefaultPageFormat = "markdown"

// NavItem is one navigation entry — see spec §4 `nav-node`. `Target` is
// optional (empty = section header with no link). `Children` is optional
// (empty = leaf). Both fields use `omitempty` to keep flat-nav wire
// shape compatible with pre-nesting readers.
type NavItem struct {
	Label    string    `cbor:"label"`
	Target   string    `cbor:"target,omitempty"`
	Children []NavItem `cbor:"children,omitempty"`
}

// NewNavLeaf returns a leaf entry (no sub-menu).
func NewNavLeaf(label, target string) NavItem {
	return NavItem{Label: label, Target: target}
}

// NewNavSection returns a section entry with a sub-menu of children.
// An empty target marks a pure section header (spec `nav-node.? target`).
func NewNavSection(label, target string, children []NavItem) NavItem {
	return NavItem{Label: label, Target: target, Children: children}
}

// SiteManifest is the site's cover — stable identity, title, the
// (optional) human nav menu, and an open params bag. Per spec §4.2 the
// manifest carries NO page-collection field; page discovery is lazy
// `.list` over `pages/`.
//
// Landing page = Params["root"] (workbench convention matching egui's
// `params.root`; falls back to DefaultRootPage when unset).
type SiteManifest struct {
	SiteID string            `cbor:"site_id"`
	Title  string            `cbor:"title"`
	Nav    []NavItem         `cbor:"nav,omitempty"`
	Params map[string]string `cbor:"params,omitempty"`
}

// NewSiteManifest builds a manifest with the landing page recorded in
// `params.root`. `root` may be empty to omit the key (callers relying on
// the convention fall back to DefaultRootPage).
func NewSiteManifest(siteID, title, root string, nav []NavItem) SiteManifest {
	m := SiteManifest{SiteID: siteID, Title: title, Nav: nav}
	if root != "" {
		m.Params = map[string]string{"root": root}
	}
	return m
}

// Root returns the landing-page slug — `params.root`, or DefaultRootPage
// when unset. Matches egui's `SiteManifest::root`.
func (m SiteManifest) Root() string {
	if r, ok := m.Params["root"]; ok && r != "" {
		return r
	}
	return DefaultRootPage
}

// SitePage is one page entity: a base-format body + frontmatter. v1
// floor is `format="markdown"` + raw markdown in `body`; renderers
// translate at the last minute (§3.1).
//
// `Frontmatter["title"]` is the one well-known key. The spec allows
// frontmatter to be omitted entirely and the title derived from the
// first H1; we keep both shapes round-trip-clean.
type SitePage struct {
	Format      string            `cbor:"format"`
	Body        string            `cbor:"body"`
	Frontmatter map[string]string `cbor:"frontmatter,omitempty"`
}

// NewMarkdownPage builds a markdown page with `frontmatter.title` set.
func NewMarkdownPage(title, body string) SitePage {
	return SitePage{
		Format:      DefaultPageFormat,
		Body:        body,
		Frontmatter: map[string]string{"title": title},
	}
}

// Title returns the page title — `frontmatter.title`, or "" if unset.
func (p SitePage) Title() string {
	return p.Frontmatter["title"]
}

// --- path layout ---

// SitesSubpath is the per-peer subpath under which all sites live.
//
// **It was `content/sites` until 2026-09-12, which is the placement
// `APP-CONVENTION-SEMANTIC-CONTENT-SITE` v0.5 DROPS by name.**
// `system/content/*` is the CONTENT extension's namespace — the leaf
// there is always `{hex(H)}` (EXTENSION-CONTENT §6.4.2) — and an L5
// application subgraph has no business in it. v0.5 makes a site a free
// subgraph at a publisher-chosen path and registers `sites` as the
// convention's reserved first segment; `entity-browser-rust` and this
// repo's own `workbench` resolvers have both been on `sites/` since
// then.
//
// **So for as long as it was wrong, this SDK — the surface an
// application developer reaches for — wrote sites where neither of our
// own site resolvers looks.** A site seeded through `entity-seed-site`
// was invisible to the Local Site panel and to the Browser panel, and
// nothing failed: the remote resolver is only ever exercised against
// `entity-browser-rust`'s frozen fixture, which uses the correct path,
// and `entitysdk`'s own tests round-trip through this constant, so both
// halves agreed with themselves. Found 2026-09-12 by pointing our own
// writer at our own reader for the first time (W2) — the same shape as
// the 2026-08-19 publish/fetch corridor, where two green halves were not
// a working corridor either.
//
// Worth more than the fix: **the divergence had already been noticed and
// filed as a coordination detail.** `publish/site_root_scope_test.go`
// says in a comment that "our placement (`content/sites/{id}/`) and
// theirs (`sites/{id}/`) put the same content under different keys", as
// a note about how to scope a joint fixture — not as a finding about
// which of the two is conformant. AP45's shape: a paragraph explaining a
// difference closes the question permanently, where a `TODO` would have
// invited the work.
//
// No migration is owed. Anything written at the old placement was
// already unreadable by every surface that renders a site.
//
// [workbench.SitesSubpath] is this constant — one definition, so the two
// halves cannot drift apart again.
const SitesSubpath = "sites"

// SitePrefix is the tree prefix that contains the entire site (manifest
// + pages + assets). Trailing slash; safe to pass to entity-publish via
// the -prefix flag.
func SitePrefix(peerID, siteID string) string {
	return fmt.Sprintf("/%s/%s/%s/", peerID, SitesSubpath, siteID)
}

// SiteManifestPath is the bound path of a site's manifest entity.
func SiteManifestPath(peerID, siteID string) string {
	return fmt.Sprintf("/%s/%s/%s/manifest", peerID, SitesSubpath, siteID)
}

// SitePagesPrefix is the tree prefix that contains a site's pages.
// Trailing slash.
func SitePagesPrefix(peerID, siteID string) string {
	return fmt.Sprintf("/%s/%s/%s/pages/", peerID, SitesSubpath, siteID)
}

// SitePagePath is the bound path of a single page entity within a site.
// `slug` may itself contain "/" segments (nested pages, e.g. "docs/intro").
func SitePagePath(peerID, siteID, slug string) string {
	return fmt.Sprintf("/%s/%s/%s/pages/%s", peerID, SitesSubpath, siteID, slug)
}

// --- put helpers (AppPeer-bound) ---

// PutSiteManifest stores m at the conventional manifest path for the
// caller's peer. Returns the entity's content hash.
func (a *AppPeer) PutSiteManifest(siteID string, m SiteManifest) (hash.Hash, error) {
	return a.Put(SiteManifestPath(a.PeerID(), siteID), TypeSiteManifest, m)
}

// PutSitePage stores p at the conventional page path for the caller's
// peer. `slug` is the page identifier ("index", "about", "docs/intro").
func (a *AppPeer) PutSitePage(siteID, slug string, p SitePage) (hash.Hash, error) {
	return a.Put(SitePagePath(a.PeerID(), siteID, slug), TypeSitePage, p)
}
