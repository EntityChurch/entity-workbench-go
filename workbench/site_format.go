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
//
// ⚠ THAT SENTENCE WAS FALSE ABOUT `nav` UNTIL 2026-09-16 and is kept
// visible rather than quietly corrected: `Nav` was tagged `cbor:"nav"`
// with no `omitempty`, directly under a comment saying optional fields
// have it. A doc comment claiming a property is the sentence the next
// reader checks the behaviour against, which is how this survived.
//
// ⭐ THESE TYPES ARE A SECOND COPY. `entitysdk.SiteManifest` /
// `entitysdk.SitePage` / `entitysdk.NavItem` are the same three shapes,
// and until the fix above the two copies encoded ONE authored manifest to
// TWO different content hashes. Nothing compared them —
// `fetch/site_entity_crossimpl_test.go`, the gate that proves our site
// bytes match another implementation's, drives the **entitysdk** copy,
// while every resolver in this package drives **this** one. That is AP96
// exactly (the `SitesSubpath` divergence), at the struct instead of the
// path constant, and the same answer applies: the two are held together by
// `site_type_agreement_test.go`, which asserts BOTH against spelled-out
// literal bytes so neither can pass by agreeing with itself.
//
// ⛔ Collapsing them to one definition is owed and deliberately NOT done
// here — it touches every resolver, the console renderer and the bridge,
// and it deserves its own change with its own gate rather than riding a
// one-tag fix. Until then the agreement test is what stands in for it.

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
// `nav` carries `omitempty` because SITE §4 declares it `? nav` and the
// convention says title-only is conformant. Without it, decoding an ordinary
// one-page manifest and re-encoding it ADDS `nav: []` — measured, 25 B -> 30 B,
// a moved content hash on an entity carrying no unknown field at all. See
// TestSiteTypeCopies_AgreeByteForByte, and AP111 in the charter.
type SiteManifest struct {
	SiteID string            `cbor:"site_id"`
	Title  string            `cbor:"title"`
	Nav    []NavNode         `cbor:"nav,omitempty"`
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
