package workbench

import (
	"bytes"
	"testing"

	"go.entitychurch.org/entity-core-go/core/ecf"
)

// Round-trip a manifest through ecf.Encode → ecf.Decode. The on-disk
// path uses Store.Put which calls ecf internally; we go direct here to
// keep the format test focused on wire shape.
func TestSiteManifestRoundTripsThroughECF(t *testing.T) {
	m := NewSiteManifest(
		"church",
		"Entity Church Foundation",
		"index",
		[]NavNode{
			{Label: "Home", Target: "./index"},
			{Label: "About", Target: "./about"},
			{Label: "Labs", Target: "entity://PEERX/sites/labs/pages/intro"},
		},
	)
	raw, err := ecf.Encode(m)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var got SiteManifest
	if err := ecf.Decode(raw, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.SiteID != "church" {
		t.Errorf("site_id: got %q want %q", got.SiteID, "church")
	}
	if got.Title != "Entity Church Foundation" {
		t.Errorf("title: got %q", got.Title)
	}
	if len(got.Nav) != 3 {
		t.Fatalf("nav len: got %d want 3", len(got.Nav))
	}
	if got.Nav[2].Target != "entity://PEERX/sites/labs/pages/intro" {
		t.Errorf("nav[2].target: got %q", got.Nav[2].Target)
	}
	if got.Root() != "index" {
		t.Errorf("root: got %q want index", got.Root())
	}
}

// A manifest with no params.root resolves to the `index` convention
// rather than the empty string.
func TestSiteManifestRootFallsBackToIndex(t *testing.T) {
	var m SiteManifest
	if r := m.Root(); r != "index" {
		t.Errorf("default root: got %q want index", r)
	}
	m.Params = map[string]string{}
	if r := m.Root(); r != "index" {
		t.Errorf("empty params root: got %q want index", r)
	}
	m.Params = map[string]string{"root": ""}
	if r := m.Root(); r != "index" {
		t.Errorf("empty-string params.root: got %q want index", r)
	}
}

// A section nav entry round-trips its children (GAP3 — nesting).
func TestNestedNavRoundTrips(t *testing.T) {
	m := NewSiteManifest(
		"docs",
		"Docs",
		"index",
		[]NavNode{
			{Label: "Home", Target: "./index"},
			{
				Label:  "Guide",
				Target: "./guide/intro",
				Children: []NavNode{
					{Label: "Intro", Target: "./guide/intro"},
					{Label: "Install", Target: "./guide/install"},
					{
						Label:  "Advanced",
						Target: "./guide/advanced/internals",
						Children: []NavNode{
							{Label: "Internals", Target: "./guide/advanced/internals"},
						},
					},
				},
			},
		},
	)
	raw, err := ecf.Encode(m)
	if err != nil {
		t.Fatal(err)
	}
	var got SiteManifest
	if err := ecf.Decode(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Nav[1].Children) != 3 {
		t.Fatalf("guide children: got %d want 3", len(got.Nav[1].Children))
	}
	if got.Nav[1].Children[2].Children[0].Label != "Internals" {
		t.Errorf("deeply nested label lost: got %q", got.Nav[1].Children[2].Children[0].Label)
	}
}

// Back-compat: a flat nav must NOT emit a `children` key, so the wire
// bytes match the pre-nesting format. Pre-nesting readers don't know
// the key exists; emitting it would break the round-trip target.
func TestFlatNavOmitsChildrenKey(t *testing.T) {
	m := NewSiteManifest(
		"flat",
		"Flat",
		"index",
		[]NavNode{
			{Label: "Home", Target: "./index"},
			{Label: "About", Target: "./about"},
		},
	)
	raw, err := ecf.Encode(m)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("children")) {
		t.Errorf("flat nav must not emit a `children` key (wire back-compat)")
	}
}

// A nav node with empty target is a section header; its wire form
// carries no `target` key (spec nav-node.? target).
func TestSectionHeaderOmitsTarget(t *testing.T) {
	m := NewSiteManifest(
		"s",
		"Sectioned",
		"index",
		[]NavNode{
			{
				Label:    "Group",
				Children: []NavNode{{Label: "Leaf", Target: "./leaf"}},
			},
		},
	)
	raw, err := ecf.Encode(m)
	if err != nil {
		t.Fatal(err)
	}

	// The header has no target; the only `target` on the wire is the
	// leaf's. Round-trip preserves the empty header target.
	var got SiteManifest
	if err := ecf.Decode(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Nav[0].Target != "" {
		t.Errorf("section header should have empty target: got %q", got.Nav[0].Target)
	}
	if got.Nav[0].Children[0].Target != "./leaf" {
		t.Errorf("leaf target lost: got %q", got.Nav[0].Children[0].Target)
	}
}

func TestSitePageRoundTrips(t *testing.T) {
	p := NewMarkdownPage("Welcome", "# Hello\n\nSome **markdown** with a [link](./about).")
	raw, err := ecf.Encode(p)
	if err != nil {
		t.Fatal(err)
	}
	var got SitePage
	if err := ecf.Decode(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Title() != "Welcome" {
		t.Errorf("title: got %q", got.Title())
	}
	if got.Format != "markdown" {
		t.Errorf("format: got %q", got.Format)
	}
	if got.Body == "" {
		t.Errorf("body empty after round-trip")
	}
}

// A page entity carrying no `format` key decodes — the format field
// is empty, NOT "markdown" (we don't have struct-level defaults
// without a custom decode hook). Consumers should treat "" as the
// markdown default. This pins the decode behavior so a future change
// is intentional.
func TestSitePageMissingFormatDecodesEmpty(t *testing.T) {
	// Encode a minimal map missing the `format` key.
	raw, err := ecf.Encode(map[string]string{"body": "# Bare"})
	if err != nil {
		t.Fatal(err)
	}
	var got SitePage
	if err := ecf.Decode(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Format != "" {
		t.Errorf("missing format decodes to empty string, got %q", got.Format)
	}
	if got.Body != "# Bare" {
		t.Errorf("body: got %q", got.Body)
	}
}

// The v0.4.1 → v0.5 invariant: a manifest with a stray `pages: [...]`
// field decodes successfully (CBOR open-map ignores unknown fields)
// AND our re-encode does NOT include it. Mirrors the v0.5 erratum
// that killed the `pages` field.
func TestManifestStrayPagesFieldDroppedOnReencode(t *testing.T) {
	// A manifest with an unknown `pages` field — what a v0.4.1 publisher
	// might emit. fxamacker/cbor in default mode tolerates extra fields.
	src := map[string]interface{}{
		"site_id": "x",
		"title":   "X",
		"nav":     []interface{}{},
		"params":  map[string]string{"root": "index"},
		"pages":   []interface{}{"home", "about"},
	}
	raw, err := ecf.Encode(src)
	if err != nil {
		t.Fatal(err)
	}

	var m SiteManifest
	if err := ecf.Decode(raw, &m); err != nil {
		t.Fatalf("manifest with stray pages field should decode: %v", err)
	}

	// Re-encode and confirm `pages` is gone.
	reraw, err := ecf.Encode(m)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(reraw, []byte("pages")) {
		t.Errorf("re-encoded manifest must not contain the `pages` key (v0.5 invariant)")
	}
}

// Path helpers shape — pin v0.5 placement (bare /sites, NOT
// /content/sites).
func TestSitePathsBareSitesNotContentSites(t *testing.T) {
	if got := ManifestPath("PEER1", "church"); got != "/PEER1/sites/church/manifest" {
		t.Errorf("ManifestPath: got %q", got)
	}
	if got := PagePath("PEER1", "church", "about"); got != "/PEER1/sites/church/pages/about" {
		t.Errorf("PagePath: got %q", got)
	}
	if got := PagesPrefix("PEER1", "church"); got != "/PEER1/sites/church/pages/" {
		t.Errorf("PagesPrefix: got %q", got)
	}
	if got := SitesPrefix("PEER1"); got != "/PEER1/sites/" {
		t.Errorf("SitesPrefix: got %q", got)
	}
	// v0.5: the layer violation is gone — no site path touches the
	// CONTENT-extension namespace.
	for _, p := range []string{
		ManifestPath("P", "s"),
		PagePath("P", "s", "x"),
		SitePrefix("P", "s"),
		PagesPrefix("P", "s"),
		SitesPrefix("P"),
	} {
		if bytes.Contains([]byte(p), []byte("/content/")) {
			t.Errorf("path %q must not touch /content/ namespace (v0.5 §2)", p)
		}
	}
}

func TestPageFromPathRoundTrips(t *testing.T) {
	full := PagePath("PEER1", "church", "docs/intro")
	got, ok := PageFromPath("PEER1", "church", full)
	if !ok || got != "docs/intro" {
		t.Errorf("PageFromPath: got %q ok=%v want docs/intro true", got, ok)
	}
	// Wrong site → no match.
	if _, ok := PageFromPath("PEER1", "other", full); ok {
		t.Errorf("PageFromPath should reject wrong site")
	}
}
