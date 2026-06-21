package workbench

// Demo site fixture. Mirrors egui-rust's
// src/views/content_site/mod.rs::ensure_demo_site so the two
// renderers can be eyeballed side-by-side against the same content.
//
// The demo is a genuinely deep site (2- and 3-level page paths under
// a Guide section) so the panel exercises nested navigation +
// breadcrumb trail + section sidebar — not just flat pages.
//
// "Preload" mechanic: the Avalonia bridge calls EnsureDemoSite on
// SiteOpen when siteID == DemoSiteID, so a fresh peer with no site
// yet still surfaces something usable. Idempotent — gated on the
// manifest's presence.

// DemoSiteID is the slug for the bundled demo site.
const DemoSiteID = "demo"

// EnsureDemoSite seeds the bundled demo site into peerID's tree
// under SitesPrefix if it isn't there yet. Idempotent: returns
// quickly when the manifest already exists.
//
// Synchronous L0 writes through the entitysdk Store — the first
// SiteModel render after this call resolves immediately.
func EnsureDemoSite(store *Store, peerID string) error {
	if store == nil {
		return nil
	}
	if _, ok := store.Get(ManifestPath(peerID, DemoSiteID)); ok {
		return nil
	}

	manifest := NewSiteManifest(
		DemoSiteID,
		"Entity Demo Site",
		"index",
		[]NavNode{
			{Label: "Home", Target: "./index"},
			// Guide is a section. Top nav points at its intro;
			// active-trail keeps Guide highlighted across the whole
			// section subtree.
			{Label: "Guide", Target: "./guide/intro"},
			{Label: "About", Target: "./about"},
			{Label: "Theory", Target: "./theory"},
		},
	)

	pages := []struct {
		slug, title, body string
	}{
		{
			"index",
			"Welcome",
			"# Welcome to the Entity Demo Site\n\nThis page is a **content-addressed entity** rendered from the local tree — you're browsing it inside a full entity peer, but it looks like any other site.\n\n- It's just markdown stored in the tree.\n- Links navigate within the entity system.\n- The Site panel resolves pages without a network.\n\nStart with the [Guide](./guide/intro), read [About](./about) or the [Theory](./theory), or visit [the web](https://example.com).\n",
		},
		{
			"guide/intro",
			"Guide — Intro",
			"# Guide: Intro\n\nThis page lives at `guide/intro` — a **nested** content entity. The *Guide* nav item stays highlighted across the whole section (active-trail).\n\nNext: [Install](./guide/install), or jump straight to the [Internals](./guide/advanced/internals).\n\nBack to [Home](./index).\n",
		},
		{
			"guide/install",
			"Guide — Install",
			"# Guide: Install\n\nStill in the Guide section (`guide/install`). Notice *Guide* is still the active nav item.\n\nBack to the [Intro](./guide/intro), or deeper to [Internals](./guide/advanced/internals).\n",
		},
		{
			"guide/advanced/internals",
			"Guide — Internals",
			"# Guide: Internals\n\nThree levels deep (`guide/advanced/internals`) and still resolving from the tree by path. The *Guide* section nav stays lit the whole way down.\n\nBack to the [Intro](./guide/intro).\n",
		},
		{
			"about",
			"About",
			"# About\n\nThe Entity Demo Site is a tiny showcase of **Site Mode**: content-addressed static sites served from the local tree.\n\n```\nsites/demo/\n  manifest\n  pages/{index,about,theory}\n  pages/guide/{intro,install}\n  pages/guide/advanced/internals\n```\n\nBack to [Home](./index).\n",
		},
		{
			"theory",
			"Theory",
			"# Theory\n\nA *site* is a content subgraph rooted at a signed manifest. Pages are markdown entities; links are entity-native and resolve across sites and peers.\n\n> Format ⊥ transport: the same page renders from the local tree, a peer, or a CDN.\n\nBack to [Home](./index).\n",
		},
	}

	if _, err := store.Put(ManifestPath(peerID, DemoSiteID), SiteManifestType, manifest); err != nil {
		return err
	}
	for _, p := range pages {
		if _, err := store.Put(
			PagePath(peerID, DemoSiteID, p.slug),
			SitePageType,
			NewMarkdownPage(p.title, p.body),
		); err != nil {
			return err
		}
	}
	return nil
}
