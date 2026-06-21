package workbench

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"go.entitychurch.org/entity-core-go/core/store"
)

// Tier 3 of plan §7: nav cycle pin. A manifest with a self-referencing
// nav-node renders without infinite-looping. visited-set + depth cap
// engage. SITE v0.5 §4.1 v1-blocking contract.
func TestNavWalkSurvivesPathologicalDepth(t *testing.T) {
	// Build a nav node with deeply nested children — depth 50,
	// well over the 32 cap. The walk must NOT stack-overflow.
	deep := []NavNode{{Label: "leaf", Target: "./leaf"}}
	for i := 0; i < 50; i++ {
		deep = []NavNode{{Label: "n", Target: "./n", Children: deep}}
	}
	out := buildNavLinks(deep, "any", "index")
	if len(out) == 0 {
		t.Errorf("nav walk should still emit the top level")
	}
	// We don't fan into children in v1, but the safety cap is the
	// real pin — the walk does not panic or stall.
}

// Visited-set engages on a self-referencing nav (constructed via
// pointer aliasing). The walk must terminate.
func TestNavWalkVisitedSetTerminates(t *testing.T) {
	// Constructed by aliasing — naive value-typed NavNode trees from
	// CBOR decode can't cycle, but a programmatic aliased construction
	// can. Exercise the defensive path.
	n := NavNode{Label: "self", Target: "./self"}
	visited := make(map[*NavNode]bool)

	// Walk twice with same pointer; the second walk returns empty
	// per the visited-set guard.
	links1 := walkNavNode(&n, "self", "index", visited, 0)
	links2 := walkNavNode(&n, "self", "index", visited, 0)

	if len(links1) != 1 {
		t.Errorf("first walk should emit one link, got %d", len(links1))
	}
	if len(links2) != 0 {
		t.Errorf("re-walk of same pointer should be visited-suppressed, got %d", len(links2))
	}
}

// Tier 5 of plan §7: empty-page navigation resolves to the manifest's
// root (params.root, or "index" default).
func TestModelEmptyPageResolvesToRoot(t *testing.T) {
	r := seedSite(t)
	m := NewSiteModel(r, Location{SiteID: "church"})
	out := m.Render()
	if out.Error != "" {
		t.Fatalf("render error: %s", out.Error)
	}
	if out.CurrentPage != "index" {
		t.Errorf("empty page should resolve to manifest root, got %q", out.CurrentPage)
	}
	if out.PageTitle != "Home" {
		t.Errorf("page title: got %q want Home", out.PageTitle)
	}
}

// Tier 6 of plan §7: section navigation surfaces a synthesized index.
func TestModelSectionIndexFlowsThroughRender(t *testing.T) {
	cs := store.NewMemoryContentStore()
	li := store.NewMemoryLocationIndex()
	st := NewStore(cs, li)

	if _, err := st.Put(ManifestPath(testPeerID, "docs"),
		SiteManifestType,
		NewSiteManifest("docs", "Docs", "index", nil)); err != nil {
		t.Fatal(err)
	}
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
	m := NewSiteModel(r, Location{SiteID: "docs", Page: "guide"})
	out := m.Render()
	if out.Error != "" {
		t.Fatalf("render error: %s", out.Error)
	}
	if !strings.Contains(out.BodyMarkdown, "intro") || !strings.Contains(out.BodyMarkdown, "install") {
		t.Errorf("section-index body lost children, got %q", out.BodyMarkdown)
	}
}

func TestModelNavigateAndGoBack(t *testing.T) {
	r := seedSite(t)
	m := NewSiteModel(r, Location{SiteID: "church"})
	_ = m.Render()
	if m.CanGoBack() {
		t.Errorf("fresh model should have no back-history")
	}

	m.Navigate(Location{SiteID: "church", Page: "about"})
	out := m.Render()
	if out.CurrentPage != "about" {
		t.Errorf("after Navigate, current should be about, got %q", out.CurrentPage)
	}
	if !out.CanGoBack {
		t.Errorf("after Navigate, CanGoBack should be true")
	}

	if !m.GoBack() {
		t.Errorf("GoBack should pop")
	}
	out = m.Render()
	if out.CurrentPage != "index" {
		t.Errorf("after GoBack, current should resolve to root (index), got %q", out.CurrentPage)
	}
	if out.CanGoBack {
		t.Errorf("after sole GoBack, CanGoBack should be false")
	}
}

// Idempotency: Navigate to the same Location is a no-op (does NOT
// stack-up history entries on rapid clicks). The bridge-side debounce
// handles the rest.
func TestModelNavigateSameLocationIdempotent(t *testing.T) {
	r := seedSite(t)
	m := NewSiteModel(r, Location{SiteID: "church"})
	loc := Location{SiteID: "church", Page: "about"}
	m.Navigate(loc)
	m.Navigate(loc)
	m.Navigate(loc)
	// One history entry only (the pre-Navigate root).
	if got := m.HistoryDepthForTest(); got != 1 {
		t.Errorf("history depth after 3× same Navigate: got %d want 1", got)
	}
}

// OnChange fires after Navigate. Bridge wires this to its wakeCh.
func TestModelOnChangeFiresOnNavigate(t *testing.T) {
	r := seedSite(t)
	m := NewSiteModel(r, Location{SiteID: "church"})
	var fired int32
	cancel := m.OnChange(func() { atomic.AddInt32(&fired, 1) })
	defer cancel()
	m.Navigate(Location{SiteID: "church", Page: "about"})
	if got := atomic.LoadInt32(&fired); got != 1 {
		t.Errorf("OnChange fire count: got %d want 1", got)
	}
	cancel()
	m.Navigate(Location{SiteID: "church"})
	if got := atomic.LoadInt32(&fired); got != 1 {
		t.Errorf("OnChange fire after cancel: got %d want 1", got)
	}
}

// Render is concurrency-safe — multiple goroutines can Render without
// data races (covered by -race).
func TestModelRenderConcurrent(t *testing.T) {
	r := seedSite(t)
	m := NewSiteModel(r, Location{SiteID: "church"})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				_ = m.Render()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 10; j++ {
			m.Navigate(Location{SiteID: "church", Page: "about"})
			m.GoBack()
		}
	}()
	wg.Wait()
}

// Breadcrumbs trail through ancestor sections, with the last segment
// rendered as the page title and a non-clickable target.
func TestBreadcrumbsTrail(t *testing.T) {
	crumbs := buildBreadcrumbs("Docs", "index", "guide/advanced/caching", "Caching")
	if len(crumbs) != 4 {
		t.Fatalf("crumb count: got %d want 4 (site, guide, advanced, current)", len(crumbs))
	}
	want := []struct {
		label, target string
	}{
		{"Docs", "./"},
		{"Guide", "./guide"},
		{"Advanced", "./guide/advanced"},
		{"Caching", ""},
	}
	for i, w := range want {
		if crumbs[i].Label != w.label || crumbs[i].Target != w.target {
			t.Errorf("crumb[%d]: got %+v want %+v", i, crumbs[i], w)
		}
	}
}

// On the root page, breadcrumbs are empty (nothing to trail).
func TestBreadcrumbsEmptyOnRoot(t *testing.T) {
	if got := buildBreadcrumbs("Site", "index", "", "Home"); got != nil {
		t.Errorf("breadcrumbs on empty page should be nil, got %+v", got)
	}
	if got := buildBreadcrumbs("Site", "index", "index", "Home"); got != nil {
		t.Errorf("breadcrumbs on root page should be nil, got %+v", got)
	}
}

// Link target classification.
func TestClassifyTarget(t *testing.T) {
	cur := Location{PeerID: "HOME", SiteID: "church", Page: "index"}
	cases := []struct {
		name   string
		target string
		want   Location
		kind   LinkKind
		ok     bool
	}{
		{"in-site dot-slash", "./about", Location{PeerID: "HOME", SiteID: "church", Page: "about"}, LinkInSite, true},
		{"in-site bare", "about", Location{PeerID: "HOME", SiteID: "church", Page: "about"}, LinkInSite, true},
		{"in-site root-slash", "/docs/intro", Location{PeerID: "HOME", SiteID: "church", Page: "docs/intro"}, LinkInSite, true},
		{"in-site parent-ref", "../theory", Location{PeerID: "HOME", SiteID: "church", Page: "theory"}, LinkInSite, true},
		{"cross-site", "site:labs/intro", Location{PeerID: "HOME", SiteID: "labs", Page: "intro"}, LinkCrossSite, true},
		{"cross-peer", "entity://PEERX/sites/labs/pages/post1", Location{PeerID: "PEERX", SiteID: "labs", Page: "post1"}, LinkCrossPeer, true},
		{"legacy content/sites", "entity://PEERX/content/sites/labs/pages/post1", Location{PeerID: "PEERX", SiteID: "labs", Page: "post1"}, LinkCrossPeer, true},
		{"external https", "https://example.com", Location{}, LinkExternal, true},
		{"external mailto", "mailto:a@b.c", Location{}, LinkExternal, true},
		{"malformed entity", "entity://nope", Location{}, LinkExternal, false},
	}
	for _, c := range cases {
		got, kind, ok := ClassifyTarget(c.target, cur)
		if got != c.want || kind != c.kind || ok != c.ok {
			t.Errorf("%s: ClassifyTarget(%q) = %+v %d %v; want %+v %d %v",
				c.name, c.target, got, kind, ok, c.want, c.kind, c.ok)
		}
	}
}

func TestNavigateTargetExternalIsNoOp(t *testing.T) {
	r := seedSite(t)
	m := NewSiteModel(r, Location{SiteID: "church"})
	_, ok := m.NavigateTarget("https://example.com")
	if ok {
		t.Errorf("external link should not Navigate")
	}
	if m.CanGoBack() {
		t.Errorf("external link must not push history")
	}
}

// HistoryDepthForTest exposes the back-history length for tests.
// Production callers use CanGoBack.
func (m *SiteModel) HistoryDepthForTest() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.history)
}
