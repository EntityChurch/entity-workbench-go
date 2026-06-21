package workbench

import (
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/store"
)

func TestEnsureDemoSiteIsIdempotent(t *testing.T) {
	cs := store.NewMemoryContentStore()
	li := store.NewMemoryLocationIndex()
	st := NewStore(cs, li)

	if err := EnsureDemoSite(st, testPeerID); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	first := st.EntityCount()
	if first == 0 {
		t.Fatalf("expected entities after first seed, got 0")
	}
	if err := EnsureDemoSite(st, testPeerID); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if got := st.EntityCount(); got != first {
		t.Errorf("second seed should be a no-op (idempotent), entity count moved %d → %d", first, got)
	}
}

// Eyeball the demo through the resolver: every nav target listed in
// the manifest must resolve to something (a page entity OR a
// synthesized section-index). No dead links.
func TestDemoSiteNavTargetsAllResolve(t *testing.T) {
	cs := store.NewMemoryContentStore()
	li := store.NewMemoryLocationIndex()
	st := NewStore(cs, li)
	if err := EnsureDemoSite(st, testPeerID); err != nil {
		t.Fatal(err)
	}
	r := NewLocalTreeResolver(st, testPeerID)
	m := NewSiteModel(r, Location{SiteID: DemoSiteID})
	out := m.Render()
	if out.Error != "" {
		t.Fatalf("demo render error: %s", out.Error)
	}
	if len(out.Nav) == 0 {
		t.Fatalf("demo manifest must declare a nav")
	}
	for _, nl := range out.Nav {
		if nl.Kind == "external" || nl.Target == "" {
			continue
		}
		loc, _, ok := ClassifyTarget(nl.Target, Location{SiteID: DemoSiteID})
		if !ok {
			t.Errorf("nav target %q failed to classify", nl.Target)
			continue
		}
		// Within-site only — cross-site/cross-peer targets aren't seeded.
		if loc.SiteID != DemoSiteID {
			continue
		}
		out := r.ResolvePage(loc)
		if !out.Ready || out.Page == nil {
			t.Errorf("nav target %q (page=%q) failed to resolve: err=%s",
				nl.Target, loc.Page, out.Err)
		}
	}
}

// The Guide page links to install + internals. After navigating to
// guide/intro, the body markdown should contain those references and
// the breadcrumbs should show the section trail.
func TestDemoSiteGuideHasBreadcrumbAndLinks(t *testing.T) {
	cs := store.NewMemoryContentStore()
	li := store.NewMemoryLocationIndex()
	st := NewStore(cs, li)
	if err := EnsureDemoSite(st, testPeerID); err != nil {
		t.Fatal(err)
	}
	r := NewLocalTreeResolver(st, testPeerID)
	m := NewSiteModel(r, Location{SiteID: DemoSiteID})
	m.Navigate(Location{SiteID: DemoSiteID, Page: "guide/intro"})
	out := m.Render()
	if out.Error != "" {
		t.Fatalf("guide render: %s", out.Error)
	}
	if !strings.Contains(out.BodyMarkdown, "Install") || !strings.Contains(out.BodyMarkdown, "Internals") {
		t.Errorf("guide intro body lost link copy: %q", out.BodyMarkdown)
	}
	if len(out.Breadcrumbs) < 2 {
		t.Errorf("guide intro should have a breadcrumb trail (site → guide → intro), got %+v", out.Breadcrumbs)
	}
}
