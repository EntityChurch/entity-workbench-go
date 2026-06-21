// entity-seed-site — author a small content site into a peer's store.
//
// Mints the SiteManifest + a handful of SitePage entities at the
// canonical paths (`/{peer}/content/sites/{site_id}/manifest` +
// `/{peer}/content/sites/{site_id}/pages/{slug}`), so a subsequent
// `entity-publish -prefix content/sites/{site_id}/` emits a static dir
// that an egui-rust consumer can fetch via HTTP-poll.
//
// The site here is workbench-go's own — short, voice-y, with a section
// header in the nav and a cross-peer placeholder link. NOT a copy of
// egui's seed site (the cross-impl proof is that distinct content
// round-trips through identical wire bytes, not that we re-publish
// theirs).
package main

import (
	"flag"
	"fmt"
	"os"

	"go.entitychurch.org/entity-core-go/core/crypto"

	"entity-workbench-go/entitysdk"
)

const defaultSiteID = "workbench-notes"

const usage = `Usage:
  entity-seed-site [flags]

Flags:
  -identity NAME      Named identity under ~/.entity/identities/.
                      Required when -storage=sqlite and -storage-path empty.
  -keypair PATH       Explicit keypair (PEM seed) — overrides -identity.
  -storage KIND       Storage backend: "memory" or "sqlite" (default sqlite).
  -storage-path PATH  SQLite DB path. Defaults to
                      ~/.entity/peers/{identity}/store.db when -identity set.
  -site-id ID         Site slug (default "%s"). The published URL will
                      address /{peer}/content/sites/{site-id}/manifest.
  -create-identity    If -identity NAME is given but no identity by that
                      name exists yet under ~/.entity/identities/, mint a
                      fresh Ed25519 keypair and save it. Off by default
                      so a typo doesn't silently create a second identity.

Re-running is idempotent in spirit but not in bytes — manifest/page
entities get re-put on each run; if the content is unchanged the content
hash is stable, but the tree's revision history grows.
`

func main() {
	identity := flag.String("identity", "", "named identity")
	keypairPath := flag.String("keypair", "", "explicit keypair file")
	storage := flag.String("storage", "sqlite", "storage backend")
	storagePath := flag.String("storage-path", "", "sqlite DB path")
	siteID := flag.String("site-id", defaultSiteID, "site slug")
	createIdentity := flag.Bool("create-identity", false, "mint identity if missing")
	flag.Usage = func() { fmt.Fprintf(os.Stderr, usage, defaultSiteID) }
	flag.Parse()

	if *createIdentity && *identity != "" {
		if err := ensureIdentity(*identity); err != nil {
			fmt.Fprintf(os.Stderr, "entity-seed-site: %v\n", err)
			os.Exit(1)
		}
	}

	ap, err := buildPeer(*storage, *storagePath, *identity, *keypairPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "entity-seed-site: %v\n", err)
		os.Exit(1)
	}
	defer ap.Close()

	manifest, pages := buildSite()
	mHash, err := ap.PutSiteManifest(*siteID, manifest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "entity-seed-site: put manifest: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("manifest  %s  → %s\n",
		entitysdk.SiteManifestPath(ap.PeerID(), *siteID), mHash)

	for _, p := range pages {
		h, err := ap.PutSitePage(*siteID, p.slug, p.page)
		if err != nil {
			fmt.Fprintf(os.Stderr, "entity-seed-site: put page %q: %v\n", p.slug, err)
			os.Exit(1)
		}
		fmt.Printf("page %-10s %s  → %s\n", p.slug,
			entitysdk.SitePagePath(ap.PeerID(), *siteID, p.slug), h)
	}

	fmt.Println()
	fmt.Printf("seeded site %q (%d pages) under peer %s\n",
		*siteID, len(pages), ap.PeerID())
	fmt.Printf("publish via:\n  entity-publish -identity %s -prefix content/sites/%s/ -origin <URL>\n",
		coalesce(*identity, "<identity>"), *siteID)
}

// ensureIdentity mints + saves a fresh Ed25519 keypair under
// ~/.entity/identities/{name} if no identity by that name exists yet.
// No-op if the identity file is already present. Detection is via
// LoadIdentity — no path-shape assumption (the on-disk layout varies
// between flat keypair files and identity-aware directory bundles).
func ensureIdentity(name string) error {
	if _, err := crypto.LoadIdentity(name); err == nil {
		return nil // already exists
	}
	kp, err := crypto.Generate()
	if err != nil {
		return fmt.Errorf("mint identity %q: %w", name, err)
	}
	if err := crypto.SaveIdentity(name, kp); err != nil {
		return fmt.Errorf("save identity %q: %w", name, err)
	}
	fmt.Printf("created identity %q\n", name)
	return nil
}

func buildPeer(storageKind, storagePath, identity, keypairPath string) (*entitysdk.AppPeer, error) {
	path := storagePath
	if storageKind == "sqlite" {
		if path == "" {
			if identity == "" {
				return nil, fmt.Errorf("-storage=sqlite requires -storage-path or -identity")
			}
			p, err := entitysdk.DefaultPeerStoragePath(identity)
			if err != nil {
				return nil, fmt.Errorf("resolve storage path: %w", err)
			}
			path = p
		}
		if err := entitysdk.EnsurePeerStorageDir(path); err != nil {
			return nil, fmt.Errorf("prepare storage dir: %w", err)
		}
	}

	cfg := entitysdk.PeerConfig{
		Storage: entitysdk.StorageConfig{Kind: storageKind, Path: path},
	}
	if keypairPath != "" {
		kp, err := crypto.LoadIdentityFromFile(keypairPath)
		if err != nil {
			return nil, fmt.Errorf("load keypair: %w", err)
		}
		cfg.Keypair = &kp
	} else if identity != "" {
		cfg.Identity = &entitysdk.IdentityBindingConfig{Name: identity}
	}
	return entitysdk.CreatePeer(cfg)
}

func coalesce(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

type seedPage struct {
	slug string
	page entitysdk.SitePage
}

// buildSite returns the manifest and the seed pages. Pages are listed
// in nav order; the actual on-wire order is whatever `.list` returns
// (hash-order; renderers sort lex-by-name per v0.4.2 §4.2).
func buildSite() (entitysdk.SiteManifest, []seedPage) {
	nav := []entitysdk.NavItem{
		entitysdk.NewNavLeaf("Home", "./index"),
		entitysdk.NewNavLeaf("About", "./about"),
		entitysdk.NewNavSection("Notes", "", []entitysdk.NavItem{
			entitysdk.NewNavLeaf("Publish pipeline", "./notes/publish"),
			entitysdk.NewNavLeaf("Cross-impl proof", "./notes/cross-impl"),
		}),
		entitysdk.NewNavLeaf("Status", "./status"),
	}

	manifest := entitysdk.NewSiteManifest(
		"workbench-notes",
		"Workbench Notes",
		"index",
		nav,
	)

	pages := []seedPage{
		{"index", entitysdk.NewMarkdownPage("Workbench Notes", indexBody)},
		{"about", entitysdk.NewMarkdownPage("About", aboutBody)},
		{"notes/publish", entitysdk.NewMarkdownPage("Publish pipeline", notesPublishBody)},
		{"notes/cross-impl", entitysdk.NewMarkdownPage("Cross-impl proof", notesCrossImplBody)},
		{"status", entitysdk.NewMarkdownPage("Status", statusBody)},
	}
	return manifest, pages
}

const indexBody = `# Workbench Notes

Short notes from the Go workbench — the side of the entity-systems
family that ships the cross-impl test rig, the publish pipeline,
and the TUI/desktop shells.

This page is the landing page, served as an ` + "`app/site-page`" + `
entity at ` + "`/{peer}/content/sites/workbench-notes/pages/index`" + `.

## What's around

- [About](./about) — what this site is and why it exists
- The **Notes** section in the menu has two short pieces:
  - [Publish pipeline](./notes/publish) — how this site got served
  - [Cross-impl proof](./notes/cross-impl) — why we're publishing at all
- [Status](./status) — where the workbench is right now

If you're an egui-rust reader fetching this over HTTP-poll, this is
the page your resolver hits first via ` + "`params.root`" + `.
`

const aboutBody = `# About

This is a small content site authored against
**APP-CONVENTION-SEMANTIC-CONTENT-SITE v0.4.2** — the locked L5
convention that lets a site travel as plain tree entities and render
identically across substrates.

It's not a copy of [the egui seed site](./index) — workbench-go is the
native-feasibility seat of the cohort, so the proof that matters is:
*the same wire format renders the same site whether the entities were
authored in Go or in Rust.*

## What it's made of

- One ` + "`app/site-manifest`" + ` at the site root (this page links to
  it indirectly via the menu).
- Five ` + "`app/site-page`" + ` entities (this is one of them).
- Markdown bodies — no embeds, no compute closure. v1 floor.

## What it's NOT

No assets, no embeds, no install-audited handlers. Those are v1.1+
work — see [Cross-impl proof](./notes/cross-impl) for what we ARE
trying to prove with this first cut.
`

const notesPublishBody = `# Publish pipeline

This page was authored by ` + "`entity-seed-site`" + ` (a small tool in
the workbench-go repo), stored as a CBOR-encoded entity at a known tree
path, and emitted as a flat static directory by ` + "`entity-publish`" + `
following the **EXTENSION-NETWORK Amendment 5** layout.

The pipeline is three stages:

1. **Author** — ` + "`entity-seed-site`" + ` constructs ` + "`SiteManifest`" + ` and
   ` + "`SitePage`" + ` Go structs, encodes them via ECF (deterministic
   CBOR, RFC 8949 §4.2), and ` + "`Put`" + `s them at:
   - ` + "`/{peer}/content/sites/workbench-notes/manifest`" + `
   - ` + "`/{peer}/content/sites/workbench-notes/pages/{slug}`" + `

2. **Publish** — ` + "`entity-publish`" + ` walks the tree under the
   ` + "`content/sites/workbench-notes/`" + ` prefix, emits a flat dir
   with the Amendment-5 manifest at the root, per-prefix ` + "`.list`" + `
   files, and content-addressed blob shards under ` + "`/content/`" + `.

3. **Serve** — any CORS-enabled static HTTP server in front of the
   ` + "`publish-out/`" + ` dir is sufficient. No live peer needed on the
   origin side — that's the whole point of the http-poll profile.

## Why this is interesting

The cross-impl test isn't "two implementations can talk to each other"
(we have a websocket profile for that). It's: **can a wire format
travel through a dumb static origin and arrive on the consumer side
indistinguishable from a live peer's tree?** That's the
HTTP-poll cohort claim.
`

const notesCrossImplBody = `# Cross-impl proof

The reason this site exists.

The content-site convention is locked at v0.4.2 across three teams
(architecture, egui-rust, workbench-go). The format is settled. What
we haven't done yet is the *cross-impl byte-for-byte proof* — author
on substrate A, transport over dumb HTTP, render on substrate B.

## Roles

- **workbench-go** authored these pages, encoded them as
  ` + "`app/site-manifest`" + ` + ` + "`app/site-page`" + ` ECF entities,
  and shipped them through ` + "`entity-publish`" + ` to a static dir.
- **egui-entity-core-rust** fetches the manifest + pages via its
  HTTP-poll resolver and renders them through its existing markdown
  view. No special handling for "Go-authored" content — if the wire
  bytes match, the renderer doesn't care.

## What success looks like

- Manifest decodes; ` + "`params.root`" + ` resolves to ` + "`index`" + `.
- ` + "`.list`" + ` of the ` + "`pages/`" + ` prefix returns the five page
  slugs in some order; the renderer sorts them
  lexicographically per the v0.4.2 §4.2 floor.
- In-site links (` + "`./about`" + `, ` + "`./notes/publish`" + `) navigate
  without leaving the site.
- The section-header **Notes** in the nav renders as a header with no
  link (target omitted on the wire, per ` + "`nav-node.? target`" + `).

If we can also point a link at
` + "`entity://<their-peer>/content/sites/<their-site>/pages/<slug>`" + `
and have it cross-peer-resolve — that's the bonus level.
`

const statusBody = `# Status

As of authoring:

- v0.4.2 locked across all three teams; vector sprint queued.
- workbench-go ships ` + "`SiteManifest`" + ` / ` + "`SitePage`" + ` /
  ` + "`NavItem`" + ` in ` + "`entitysdk/site.go`" + `; round-trip + flat-nav
  back-compat tests are green.
- ` + "`entity-publish`" + ` emits the Amendment-5 layout that the
  HTTP-poll consumer profile expects.
- Embeds, the install-audit shell-inspectable surface, and the
  ` + "`.entsite`" + ` bundle helper are NOT in this first cut — they're
  workbench-owned, but post-proof.

[Back to home](./index).
`
