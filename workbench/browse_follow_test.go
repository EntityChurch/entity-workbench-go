package workbench_test

import (
	"context"
	"testing"
)

// Follow is what a click on a link in a rendered page calls.
//
// These drive the frozen cross-impl federation, and the targets are the
// links the fixture's own pages actually contain — `demo-notes/index`
// carries `[First Entry](entries/first)` and
// `[back to the Demo](site:demo/index)`. Using the page's real links
// rather than invented ones is the point: an invented target tests the
// resolver against a convention nobody publishes.

// TestBrowseFollowInSiteLink: the ordinary case, a relative page link.
func TestBrowseFollowInSiteLink(t *testing.T) {
	m, _ := browserAt(t)
	ctx := context.Background()

	if err := m.Open(ctx, "docs.entitychurch.org"); err != nil {
		t.Fatalf("open: %v", err)
	}
	start := m.Render()
	if start.Page != "index" || start.Site != "demo-notes" {
		t.Fatalf("fixture precondition moved: site=%q page=%q", start.Site, start.Page)
	}

	if err := m.Follow(ctx, "entries/first"); err != nil {
		t.Fatalf("Follow(entries/first): %v", err)
	}
	out := m.Render()
	if out.Err != "" {
		t.Fatalf("following an in-site link failed: %s", out.Err)
	}
	if out.Page != "entries/first" {
		t.Errorf("page = %q, want entries/first", out.Page)
	}
	if out.Site != "demo-notes" {
		t.Errorf("site = %q — an in-site link must stay in the site", out.Site)
	}
	if out.Content.BodyMarkdown == "" {
		t.Error("navigated and rendered no body")
	}
}

// TestBrowseFollowInSiteLinkStripsMarkdownExtension is the defect a user
// hit on the live federation.
//
// billslab.com's front page links `[Support Bill's Lab](support.md)` and
// the signed root commits the page under `support`. We kept the `.md`,
// asked for a key that does not exist, and the walk correctly answered
// "page missing" — so every in-page link in the shipped browser was dead.
// The suffix is stripped from the final slug only, matching
// entity-browser-rust's resolve_in_site.
func TestBrowseFollowInSiteLinkStripsMarkdownExtension(t *testing.T) {
	m, _ := browserAt(t)
	ctx := context.Background()

	if err := m.Open(ctx, "docs.entitychurch.org"); err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := m.Follow(ctx, "entries/first.md"); err != nil {
		t.Fatalf("Follow(entries/first.md): %v", err)
	}
	out := m.Render()
	if out.Err != "" {
		t.Fatalf("a `.md` link must resolve to the extensionless slug; got: %s", out.Err)
	}
	if out.Page != "entries/first" {
		t.Errorf("page = %q, want entries/first (the .md must not survive into the slug)", out.Page)
	}
}

// TestBrowseFollowCrossSiteLink: `site:` hops to another site on the
// same publisher, inheriting the peer.
func TestBrowseFollowCrossSiteLink(t *testing.T) {
	m, _ := browserAt(t)
	ctx := context.Background()

	if err := m.Open(ctx, "docs.entitychurch.org"); err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := m.Follow(ctx, "site:demo/index"); err != nil {
		t.Fatalf("Follow(site:demo/index): %v", err)
	}
	out := m.Render()
	if out.Err != "" {
		t.Fatalf("cross-site link failed: %s", out.Err)
	}
	if out.Site != "demo" {
		t.Errorf("site = %q, want demo", out.Site)
	}
	if out.Host != "docs.entitychurch.org" {
		t.Errorf("host = %q — a cross-SITE link must not leave the publisher, and must keep the "+
			"name we arrived by so the chain is re-run through the registry", out.Host)
	}
}

// An external link must NOT navigate and must NOT clear the page.
//
// Routing it through Err would have been the easy thing and it is wrong:
// a renderer clears the page on Err (a refused navigation must never
// leave stale bytes under a failed chain), so clicking a link to
// someone's Ko-fi would blank the page being read. The live
// billslab.com support page has exactly that link.
func TestBrowseFollowExternalLinkDoesNotNavigateOrClearThePage(t *testing.T) {
	m, _ := browserAt(t)
	ctx := context.Background()

	if err := m.Open(ctx, "docs.entitychurch.org"); err != nil {
		t.Fatalf("open: %v", err)
	}
	before := m.Render()
	if before.Content.BodyMarkdown == "" {
		t.Fatal("fixture precondition: expected a page body on screen")
	}

	if err := m.Follow(ctx, "https://ko-fi.com/billslab"); err != nil {
		t.Fatalf("Follow(external) returned an error; it must be a no-op with a notice: %v", err)
	}
	after := m.Render()

	if after.Notice == "" {
		t.Error("an external link must SAY it was not followed — a click that does nothing and " +
			"reports nothing reads as a broken browser")
	}
	if after.Err != "" {
		t.Errorf("external link set Err = %q; Err clears the page, and nothing was refused", after.Err)
	}
	if after.Content.BodyMarkdown != before.Content.BodyMarkdown {
		t.Error("the page changed — a link out of the system must leave the reader where they were")
	}
	if after.Address != before.Address {
		t.Errorf("address moved to %q on an external link", after.Address)
	}
}

// TestBrowseFollowPushesHistory: a followed link is a history entry, so
// Back is how a reader undoes a click.
func TestBrowseFollowPushesHistory(t *testing.T) {
	m, _ := browserAt(t)
	ctx := context.Background()

	if err := m.Open(ctx, "docs.entitychurch.org"); err != nil {
		t.Fatalf("open: %v", err)
	}
	first := m.Render().Address

	if err := m.Follow(ctx, "entries/first"); err != nil {
		t.Fatalf("Follow: %v", err)
	}
	if !m.Render().CanBack {
		t.Fatal("a followed link must push history")
	}
	if err := m.Back(ctx); err != nil {
		t.Fatalf("Back: %v", err)
	}
	if got := m.Render().Address; got != first {
		t.Errorf("Back landed on %q, want %q", got, first)
	}
}

// TestBrowseFollowNoticeIsClearedByTheNextFollow: a stale notice beside
// a page it does not describe is its own small lie.
func TestBrowseFollowNoticeIsClearedByTheNextFollow(t *testing.T) {
	m, _ := browserAt(t)
	ctx := context.Background()

	if err := m.Open(ctx, "docs.entitychurch.org"); err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := m.Follow(ctx, "https://example.com"); err != nil {
		t.Fatalf("Follow(external): %v", err)
	}
	if m.Render().Notice == "" {
		t.Fatal("precondition: expected a notice")
	}
	if err := m.Follow(ctx, "entries/first"); err != nil {
		t.Fatalf("Follow(in-site): %v", err)
	}
	if n := m.Render().Notice; n != "" {
		t.Errorf("notice survived a successful navigation: %q", n)
	}
}
