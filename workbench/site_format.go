package workbench

// Site entity types — SITE convention v0.5
// (APP-CONVENTION-SEMANTIC-CONTENT-SITE, ratified).
//
// Mirrors egui-rust src/content_site/format.rs. Type tags are final
// (app/site-manifest, app/site-page). CBOR encoding goes through
// entitysdk.Store.Put which uses ecf (deterministic CBOR per RFC 8949
// §4.2). Optional fields use `omitempty` so wire bytes for a flat-nav
// manifest stay back-compatible with pre-nesting readers (the
// `children` key is emitted only when non-empty); same for section
// headers (empty `target`).

// SiteManifestType is the type tag for site manifests.
const SiteManifestType = "app/site-manifest"

// SitePageType is the type tag for site pages.
const SitePageType = "app/site-page"

// DefaultRootPage is the landing page slug used when a manifest
// declares no params.root.
const DefaultRootPage = "index"

// DefaultPageFormat is the default base format for a page body.
const DefaultPageFormat = "markdown"

// NavNode is one navigation entry. `Target` empty = section header
// with no link (spec nav-node.? target). `Children` empty = leaf.
// Cycle-safe + max-depth-32 walk enforced by the consuming model
// (SITE v0.5 §4.1 v1-blocking).
type NavNode struct {
	Label    string    `cbor:"label"`
	Target   string    `cbor:"target,omitempty"`
	Children []NavNode `cbor:"children,omitempty"`
}

// SiteManifest is a site's cover: identity + title + curated nav menu
// + an open params attribute bag (params.root names the landing page,
// our reasonable v1 choice in the absence of a spec top-level root).
//
// Per v0.5 §4 the manifest holds NO page-collection field (the killed
// pages field); discovery is lazy `.list`.
type SiteManifest struct {
	SiteID string            `cbor:"site_id"`
	Title  string            `cbor:"title"`
	Nav    []NavNode         `cbor:"nav"`
	Params map[string]string `cbor:"params,omitempty"`
}

// Root returns the landing page slug — params.root, defaulting to
// DefaultRootPage if unset or empty.
func (m *SiteManifest) Root() string {
	if r, ok := m.Params["root"]; ok && r != "" {
		return r
	}
	return DefaultRootPage
}

// SitePage is a single page: a base-format body + frontmatter map.
// `frontmatter.title` is the conventional title key.
type SitePage struct {
	Format      string            `cbor:"format"`
	Body        string            `cbor:"body"`
	Frontmatter map[string]string `cbor:"frontmatter,omitempty"`
}

// Title returns frontmatter.title, or empty string if unset.
func (p *SitePage) Title() string {
	if p.Frontmatter == nil {
		return ""
	}
	return p.Frontmatter["title"]
}

// NewMarkdownPage builds a markdown page with frontmatter.title set.
func NewMarkdownPage(title, body string) SitePage {
	return SitePage{
		Format:      DefaultPageFormat,
		Body:        body,
		Frontmatter: map[string]string{"title": title},
	}
}

// NewSiteManifest builds a manifest with the landing page recorded in
// params.root.
func NewSiteManifest(siteID, title, root string, nav []NavNode) SiteManifest {
	return SiteManifest{
		SiteID: siteID,
		Title:  title,
		Nav:    nav,
		Params: map[string]string{"root": root},
	}
}
