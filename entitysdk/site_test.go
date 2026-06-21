package entitysdk

import (
	"bytes"
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
)

// Round-trip: encode a SiteManifest → decode back → DeepEqual.
// This is the basic "did the cbor tags wire up?" check.
func TestSiteManifest_RoundTrip(t *testing.T) {
	in := NewSiteManifest(
		"workbench",
		"Workbench Notes",
		"index",
		[]NavItem{
			NewNavLeaf("Home", "./index"),
			NewNavLeaf("About", "./about"),
			NewNavLeaf("Labs",
				"entity://PEERX/content/sites/labs/pages/intro"),
		},
	)

	raw, err := ecf.Encode(in)
	if err != nil {
		t.Fatalf("encode manifest: %v", err)
	}

	var out SiteManifest
	if err := ecf.Decode(raw, &out); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}

	if out.SiteID != in.SiteID || out.Title != in.Title {
		t.Fatalf("identity fields lost: in=%+v out=%+v", in, out)
	}
	if len(out.Nav) != len(in.Nav) {
		t.Fatalf("nav length mismatch: in=%d out=%d", len(in.Nav), len(out.Nav))
	}
	if out.Root() != "index" {
		t.Fatalf("Root() lost via params.root: got %q", out.Root())
	}
}

// Round-trip: nested nav (section + children) survives. Mirrors egui's
// `nested_nav_round_trips_through_entity`.
func TestSiteManifest_NestedNavRoundTrip(t *testing.T) {
	in := NewSiteManifest("docs", "Docs", "index", []NavItem{
		NewNavLeaf("Home", "./index"),
		NewNavSection("Guide", "./guide/intro", []NavItem{
			NewNavLeaf("Intro", "./guide/intro"),
			NewNavLeaf("Install", "./guide/install"),
			NewNavSection("Advanced", "./guide/advanced/internals", []NavItem{
				NewNavLeaf("Internals", "./guide/advanced/internals"),
			}),
		}),
	})

	raw, err := ecf.Encode(in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var out SiteManifest
	if err := ecf.Decode(raw, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Nav) != 2 {
		t.Fatalf("top-level nav: got %d, want 2", len(out.Nav))
	}
	if len(out.Nav[1].Children) != 3 {
		t.Fatalf("section children: got %d, want 3", len(out.Nav[1].Children))
	}
	if out.Nav[1].Children[2].Children[0].Label != "Internals" {
		t.Fatalf("deep nested label lost: %+v", out.Nav[1].Children[2])
	}
}

// Wire back-compat: a flat nav MUST NOT emit a `children` key — egui
// readers depend on this (`flat_nav_is_wire_compatible_with_pre_nesting_format`).
func TestSiteManifest_FlatNavOmitsChildrenKey(t *testing.T) {
	in := NewSiteManifest("flat", "Flat", "index", []NavItem{
		NewNavLeaf("Home", "./index"),
		NewNavLeaf("About", "./about"),
	})
	raw, err := ecf.Encode(in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if strings.Contains(string(raw), "children") {
		t.Fatalf("flat nav emitted a `children` key; wire bytes: %x", raw)
	}
}

// Section headers carry no target — verifies omitempty on Target works
// for the empty string. Mirrors egui's `section_header_omits_target`.
func TestSiteManifest_SectionHeaderOmitsTarget(t *testing.T) {
	in := NewSiteManifest("s", "Sectioned", "index", []NavItem{
		NewNavSection("Group", "", []NavItem{NewNavLeaf("Leaf", "./leaf")}),
	})
	raw, err := ecf.Encode(in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	// The header has no target; the only `target` on the wire belongs
	// to the leaf entry. The substring count of "target" must be 1.
	if n := bytes.Count(raw, []byte("target")); n != 1 {
		t.Fatalf("expected one `target` key (leaf only); got %d", n)
	}
	var out SiteManifest
	if err := ecf.Decode(raw, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Nav[0].Target != "" {
		t.Fatalf("section header target should round-trip empty; got %q", out.Nav[0].Target)
	}
	if out.Nav[0].Children[0].Target != "./leaf" {
		t.Fatalf("leaf target lost: got %q", out.Nav[0].Children[0].Target)
	}
}

// Default landing-page fallback: an empty-params manifest resolves
// Root() to "index" by convention (egui's `manifest_root_falls_back_to_index`).
func TestSiteManifest_RootFallsBackToIndex(t *testing.T) {
	if (SiteManifest{}).Root() != DefaultRootPage {
		t.Fatalf("Root() should default to %q on an empty manifest", DefaultRootPage)
	}
}

// SitePage round-trip + title accessor.
func TestSitePage_RoundTrip(t *testing.T) {
	in := NewMarkdownPage("Welcome",
		"# Hello\n\nSome **markdown** with a [link](./about).")
	raw, err := ecf.Encode(in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var out SitePage
	if err := ecf.Decode(raw, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Title() != "Welcome" {
		t.Fatalf("title lost: got %q", out.Title())
	}
	if out.Format != DefaultPageFormat {
		t.Fatalf("format lost: got %q", out.Format)
	}
	if out.Body != in.Body {
		t.Fatalf("body lost")
	}
}

// Type tag is correctly assigned on the wrapping Entity.
func TestSiteTypes_EntityTags(t *testing.T) {
	mRaw, err := ecf.Encode(SiteManifest{SiteID: "x", Title: "X"})
	if err != nil {
		t.Fatal(err)
	}
	mEnt, err := entity.NewEntity(TypeSiteManifest, mRaw)
	if err != nil {
		t.Fatal(err)
	}
	if mEnt.Type != TypeSiteManifest {
		t.Fatalf("manifest entity type: got %q want %q", mEnt.Type, TypeSiteManifest)
	}

	pRaw, err := ecf.Encode(NewMarkdownPage("T", "# T"))
	if err != nil {
		t.Fatal(err)
	}
	pEnt, err := entity.NewEntity(TypeSitePage, pRaw)
	if err != nil {
		t.Fatal(err)
	}
	if pEnt.Type != TypeSitePage {
		t.Fatalf("page entity type: got %q want %q", pEnt.Type, TypeSitePage)
	}

	if TypeSiteManifest != "app/site-manifest" || TypeSitePage != "app/site-page" {
		t.Fatalf("type tags drifted from v0.4.2 §4: manifest=%q page=%q",
			TypeSiteManifest, TypeSitePage)
	}
}

// Deterministic encoding: byte-stable across encodes (CoreDet sorts map
// keys length-then-lex). Critical for content-hash stability cross-impl.
func TestSiteManifest_DeterministicEncoding(t *testing.T) {
	m := NewSiteManifest("det", "Determinism", "index", []NavItem{
		NewNavLeaf("Home", "./index"),
		NewNavLeaf("Posts", "./posts"),
	})
	a, err := ecf.Encode(m)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ecf.Encode(m)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("encode not deterministic: a=%x b=%x", a, b)
	}
}

// Path helpers — sanity-check the egui-aligned tree layout.
func TestSitePaths(t *testing.T) {
	cases := []struct{ got, want string }{
		{SitePrefix("PEER1", "blog"), "/PEER1/content/sites/blog/"},
		{SiteManifestPath("PEER1", "blog"), "/PEER1/content/sites/blog/manifest"},
		{SitePagesPrefix("PEER1", "blog"), "/PEER1/content/sites/blog/pages/"},
		{SitePagePath("PEER1", "blog", "about"), "/PEER1/content/sites/blog/pages/about"},
		{SitePagePath("PEER1", "blog", "docs/intro"), "/PEER1/content/sites/blog/pages/docs/intro"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("path mismatch: got %q want %q", c.got, c.want)
		}
	}
}
