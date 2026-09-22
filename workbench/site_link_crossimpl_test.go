package workbench

import "testing"

// Cross-impl gate for in-site link resolution.
//
// Turning a markdown href into a page slug is **Layer-2 algorithm
// contract** under AGENTS.md: the result addresses bytes in a
// content-addressed tree, so two implementations that disagree do not
// merely render differently — they fetch different keys, and one of them
// gets `page missing` on content the other reads fine.
//
// The vectors below are lifted VERBATIM from the reference
// implementation's own unit tests:
// `entity-browser-rust/src/content_site/location.rs`, the `tests` module
// (`in_site_links_resolve_from_root_page` and
// `in_site_links_resolve_dir_relative_from_nested_page`). They are not
// re-derived here and must not be "corrected" locally — if this test
// fails, either our resolver drifted or theirs did, and the next step is
// a `reviews/` packet, not an edit to the expectations.
//
// # What this caught
//
// Everything below except the first four cases failed before 2026-08-31.
// Our `normalizeInSitePage` stripped leading `./` `/` `../` in a loop and
// returned the rest: it ignored the current page's directory and never
// stripped `.md`. On the live federation billslab.com's front page links
// `[Support Bill's Lab](support.md)`, which we resolved to the page
// `support.md` — a key the signed root does not commit — so the walk
// reported `page missing` and **every in-page link in the browser was
// dead**. Measured against the live origin, not inferred.
func TestResolveInSitePage_MatchesRustReference(t *testing.T) {
	cases := []struct {
		name        string
		href        string
		currentPage string
		want        string
	}{
		// --- in_site_links_resolve_from_root_page (current page "index",
		// so dir = "") ---
		{"root/dot-slash", "./about", "index", "about"},
		{"root/bare", "about", "index", "about"},
		{"root/absolute", "/docs/intro", "index", "docs/intro"},
		{"root/dotdot clamps", "../theory", "index", "theory"},

		// --- in_site_links_resolve_dir_relative_from_nested_page
		// (current page "research/model/grounding", so dir =
		// "research/model") ---
		{
			name:        "nested/papers-corpus canonical case",
			href:        "../notes/abstract/analysis-abstract-bridge.md",
			currentPage: "research/model/grounding",
			want:        "research/notes/abstract/analysis-abstract-bridge",
		},
		{"nested/bare is sibling", "sibling.md", "research/model/grounding", "research/model/sibling"},
		{"nested/dot-slash is no-op", "./sibling", "research/model/grounding", "research/model/sibling"},
		{"nested/two pops to root", "../../top.md", "research/model/grounding", "top"},
		{"nested/dotdot clamps at root", "../../../../escape", "research/model/grounding", "escape"},
		{"nested/leading slash ignores dir", "/abs/page.md", "research/model/grounding", "abs/page"},
		{"nested/trailing slash is dir target", "notes/", "research/model/grounding", "research/model/notes"},
		{"nested/fragment dropped and .md stripped", "page.md#section", "research/model/grounding", "research/model/page"},
		{"nested/site root is empty slug", "/", "research/model/grounding", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := resolveInSitePage(c.href, c.currentPage)
			if got != c.want {
				t.Errorf("resolveInSitePage(%q, current=%q) = %q; want %q\n"+
					"vector is from entity-browser-rust location.rs — route a divergence, do not edit the expectation",
					c.href, c.currentPage, got, c.want)
			}
		})
	}
}

// TestResolveInSitePage_LiveBillslabLink pins the exact link that was
// dead in the shipped browser, as its own named case.
//
// A table entry can be lost in a refactor; the defect that put a user in
// front of a page whose every link did nothing deserves a test with its
// own name and its own failure message.
func TestResolveInSitePage_LiveBillslabLink(t *testing.T) {
	// billslab.com/billslab-main/index links to `support.md`; the signed
	// root commits the page under `support`. Verified against the live
	// origin on 2026-08-31:
	//   `open billslab.com/billslab-main/support.md` -> page missing
	//   `open billslab.com/billslab-main/support`    -> 535 bytes
	if got := resolveInSitePage("support.md", "index"); got != "support" {
		t.Fatalf("the live billslab.com front-page link still does not resolve: "+
			"resolveInSitePage(%q, %q) = %q; want %q", "support.md", "index", got, "support")
	}
}

// TestClassifyTarget_SiteSchemeFromLivePage covers the other live link
// form on the same page — `[...](site:billslab-entity-system)` — which
// must inherit the current peer and cross to a different site.
func TestClassifyTarget_SiteSchemeFromLivePage(t *testing.T) {
	cur := Location{PeerID: "PEER", SiteID: "billslab-main", Page: "index"}
	loc, kind, ok := ClassifyTarget("site:billslab-entity-system", cur)
	if !ok || kind != LinkCrossSite {
		t.Fatalf("ClassifyTarget(site:...) = kind %v ok %v; want LinkCrossSite true", kind, ok)
	}
	if loc.PeerID != "PEER" {
		t.Errorf("cross-site link must inherit the current peer; got %q", loc.PeerID)
	}
	if loc.SiteID != "billslab-entity-system" {
		t.Errorf("site id = %q; want billslab-entity-system", loc.SiteID)
	}
	if loc.Page != "" {
		t.Errorf("page = %q; want empty (the site's declared root page)", loc.Page)
	}
}
