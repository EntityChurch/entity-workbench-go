package workbench

import (
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/store"
)

const testPeerID = "PEER1"

// seedSite plants a small fixture site (manifest + pages) into the
// store. Returns the resolver bound to testPeerID.
func seedSite(t *testing.T) *LocalTreeResolver {
	t.Helper()
	cs := store.NewMemoryContentStore()
	li := store.NewMemoryLocationIndex()
	st := NewStore(cs, li)

	manifest := NewSiteManifest(
		"church",
		"Entity Church Foundation",
		"index",
		[]NavNode{
			{Label: "Home", Target: "./index"},
			{Label: "About", Target: "./about"},
		},
	)
	if _, err := st.Put(ManifestPath(testPeerID, "church"), SiteManifestType, manifest); err != nil {
		t.Fatalf("put manifest: %v", err)
	}
	if _, err := st.Put(
		PagePath(testPeerID, "church", "index"),
		SitePageType,
		NewMarkdownPage("Home", "# Welcome\n\nSee [About](./about)."),
	); err != nil {
		t.Fatalf("put index: %v", err)
	}
	if _, err := st.Put(
		PagePath(testPeerID, "church", "about"),
		SitePageType,
		NewMarkdownPage("About", "# About us"),
	); err != nil {
		t.Fatalf("put about: %v", err)
	}
	return NewLocalTreeResolver(st, testPeerID)
}

// Round-trip pin: seed manifest + pages, resolve root, get expected
// title + body. Tier 1 of plan §7.
func TestLocalResolverResolvesRootPage(t *testing.T) {
	r := seedSite(t)
	out := r.ResolvePage(Location{SiteID: "church"})
	if !out.Ready || out.Page == nil {
		t.Fatalf("expected Ready+Page, got Ready=%v Err=%s", out.Ready, out.Err)
	}
	if out.Page.Location.Page != "index" {
		t.Errorf("empty page should resolve to manifest root, got %q", out.Page.Location.Page)
	}
	if out.Page.Manifest.Title != "Entity Church Foundation" {
		t.Errorf("manifest title: got %q", out.Page.Manifest.Title)
	}
	if got := out.Page.Page.Title(); got != "Home" {
		t.Errorf("page title: got %q want Home", got)
	}
	if !strings.Contains(out.Page.Page.Body, "Welcome") {
		t.Errorf("page body lost: %q", out.Page.Page.Body)
	}
}

func TestLocalResolverResolvesNamedPage(t *testing.T) {
	r := seedSite(t)
	out := r.ResolvePage(Location{SiteID: "church", Page: "about"})
	if !out.Ready || out.Page == nil {
		t.Fatalf("expected Ready+Page, got Err=%s", out.Err)
	}
	if got := out.Page.Page.Title(); got != "About" {
		t.Errorf("page title: got %q want About", got)
	}
}

func TestLocalResolverMissingManifestErrors(t *testing.T) {
	r := seedSite(t)
	out := r.ResolvePage(Location{SiteID: "ghost"})
	if !out.Ready {
		t.Fatalf("expected Ready, got Ready=%v", out.Ready)
	}
	if out.Err != ManifestMissing {
		t.Errorf("expected ManifestMissing, got %s", out.Err)
	}
}

func TestLocalResolverMissingPageErrors(t *testing.T) {
	r := seedSite(t)
	out := r.ResolvePage(Location{SiteID: "church", Page: "ghost"})
	if !out.Ready {
		t.Fatalf("expected Ready")
	}
	if out.Err != PageMissing {
		t.Errorf("expected PageMissing, got %s", out.Err)
	}
}

// Lex-order pin: pages named `a-second`, `b-first`, `c-third` inserted
// in arbitrary order come back from ListChildren sorted byte-wise on
// slug. v0.4.2 §4.2 ordering floor. Tier 2 of plan §7.
func TestListChildrenLexOrderFloor(t *testing.T) {
	cs := store.NewMemoryContentStore()
	li := store.NewMemoryLocationIndex()
	st := NewStore(cs, li)

	if _, err := st.Put(ManifestPath(testPeerID, "s"),
		SiteManifestType,
		NewSiteManifest("s", "S", "a-second", nil)); err != nil {
		t.Fatal(err)
	}
	// Insertion order intentionally non-lex.
	for _, slug := range []string{"c-third", "a-second", "b-first"} {
		if _, err := st.Put(
			PagePath(testPeerID, "s", slug),
			SitePageType,
			NewMarkdownPage(slug, "# "+slug),
		); err != nil {
			t.Fatal(err)
		}
	}
	r := NewLocalTreeResolver(st, testPeerID)
	got := r.ListChildren(Location{SiteID: "s"}, "")
	if len(got) != 3 {
		t.Fatalf("len: got %d want 3, entries=%+v", len(got), got)
	}
	want := []string{"a-second", "b-first", "c-third"}
	for i, w := range want {
		if got[i].Name != w {
			t.Errorf("[%d]: got %q want %q (full order: %+v)", i, got[i].Name, w, got)
		}
		if !got[i].IsPage {
			t.Errorf("[%d] %s: expected IsPage=true", i, got[i].Name)
		}
	}
}

// Section-index synthesis: navigating to a slug that's a SECTION
// (children exist, no page entity) returns a synthesized page listing
// children. Mirrors egui discovery.rs parity. Tier 6 of plan §7.
func TestSectionIndexSynthesis(t *testing.T) {
	cs := store.NewMemoryContentStore()
	li := store.NewMemoryLocationIndex()
	st := NewStore(cs, li)

	if _, err := st.Put(ManifestPath(testPeerID, "docs"),
		SiteManifestType,
		NewSiteManifest("docs", "Docs", "index", nil)); err != nil {
		t.Fatal(err)
	}
	// Note: no page entity at `guide` itself — guide is a pure section.
	for _, slug := range []string{"index", "guide/intro", "guide/install"} {
		if _, err := st.Put(
			PagePath(testPeerID, "docs", slug),
			SitePageType,
			NewMarkdownPage(slug, "# "+slug),
		); err != nil {
			t.Fatal(err)
		}
	}
	r := NewLocalTreeResolver(st, testPeerID)
	out := r.ResolvePage(Location{SiteID: "docs", Page: "guide"})
	if !out.Ready || out.Page == nil {
		t.Fatalf("section navigation should resolve a synthesized index, got Err=%s", out.Err)
	}
	body := out.Page.Page.Body
	if !strings.Contains(body, "intro") || !strings.Contains(body, "install") {
		t.Errorf("synthesized section-index missing children, got body=%q", body)
	}
	// Title humanized from last segment.
	if title := out.Page.Page.Title(); title != "Guide" {
		t.Errorf("section title: got %q want Guide", title)
	}
}

// One name can be both is_page AND is_section (a section with its own
// index page at the exact slug). The discovery walk must not lose
// either flag.
func TestListChildrenPageAndSectionSimultaneously(t *testing.T) {
	cs := store.NewMemoryContentStore()
	li := store.NewMemoryLocationIndex()
	st := NewStore(cs, li)

	if _, err := st.Put(ManifestPath(testPeerID, "s"),
		SiteManifestType,
		NewSiteManifest("s", "S", "guide", nil)); err != nil {
		t.Fatal(err)
	}
	// `guide` exists as a page AND has children.
	for _, slug := range []string{"guide", "guide/intro"} {
		if _, err := st.Put(
			PagePath(testPeerID, "s", slug),
			SitePageType,
			NewMarkdownPage(slug, "# "+slug),
		); err != nil {
			t.Fatal(err)
		}
	}
	r := NewLocalTreeResolver(st, testPeerID)
	got := r.ListChildren(Location{SiteID: "s"}, "")
	if len(got) != 1 || got[0].Name != "guide" {
		t.Fatalf("expected single child `guide`, got %+v", got)
	}
	if !got[0].IsPage {
		t.Error("guide should be IsPage=true (entity exists at exact slug)")
	}
	if !got[0].IsSection {
		t.Error("guide should be IsSection=true (descendants exist)")
	}
}

// Humanize converts segment kebab/snake to spaced title case — pin
// the rendering convention used in breadcrumbs + synthesized titles.
func TestHumanize(t *testing.T) {
	cases := map[string]string{
		"getting-started": "Getting started",
		"my_section":      "My section",
		"guide":           "Guide",
		"":                "",
	}
	for in, want := range cases {
		if got := humanize(in); got != want {
			t.Errorf("humanize(%q): got %q want %q", in, got, want)
		}
	}
}
