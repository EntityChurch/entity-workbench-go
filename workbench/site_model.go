package workbench

import (
	"strings"
	"sync"
)

// SiteModel is the renderer-neutral model for one site-view surface.
// Mirrors egui-rust src/views/content_site/model.rs.
//
// State ownership:
//   - The model owns the current Location, the session back-history,
//     and the last-rendered SiteRenderOutput.
//   - Renderers call Navigate / GoBack to mutate location; the model
//     fires OnChange so the bridge can wake the UI thread to re-Render.
//   - Render is a pull-based snapshot that always returns the latest
//     fully-built output. Calling Render multiple times is cheap when
//     nothing changed (the output is cached behind a dirty flag).
//
// D18: this model lives in workbench/ and both renderers (console +
// Avalonia) consume it. Presentation choices (debounce, virtualization,
// panel layout) live in the renderer.
//
// Nav safety (SITE v0.5 §4.1 v1-blocking): the model's nav-walk caps
// recursion depth and tracks a visited-set so a self-referencing or
// pathological nav tree cannot stall or stack-overflow the renderer.
type SiteModel struct {
	resolver ContentResolver

	mu        sync.Mutex
	current   Location
	history   []Location
	output    SiteRenderOutput
	dirty     bool
	listeners []func()
}

// Maximum depth of the nav-walk; SITE v0.5 §4.1 v1-blocking. The
// breadcrumb + sidebar walks share the same cap.
const SiteNavMaxDepth = 32

// NavLink is one entry in the manifest nav. The renderer renders it
// with `Active` set when the current page is within the link's section.
type NavLink struct {
	Label  string
	Target string
	Active bool
	// Kind is a renderer hint: "page" (clickable leaf), "section-header"
	// (no target, just label), or "external" (link out of the system).
	Kind string
}

// Crumb is one step in the breadcrumb trail. Target empty = the
// current "you are here" segment (a label, not a link).
type Crumb struct {
	Label  string
	Target string
}

// SectionLink is one entry in the tree-driven section sidebar.
// Depth 0 = top-level; depth 1 = expanded child of the active section.
type SectionLink struct {
	Label     string
	Target    string
	Active    bool
	Depth     int
	IsSection bool
}

// SiteRenderOutput is everything the renderer needs for one frame.
type SiteRenderOutput struct {
	SiteTitle   string
	PageTitle   string
	Nav         []NavLink
	Breadcrumbs []Crumb
	Sidebar     []SectionLink
	// BodyMarkdown is the raw markdown body (or HTML for format=html).
	// The renderer converts to its display medium — Avalonia parses to
	// inlines via MarkdownView's pipeline; tview prints as plain text.
	BodyMarkdown string
	BodyFormat   string

	// Current location, exposed so the renderer can classify relative
	// links and so cross-pattern tests can pin nav state.
	PeerID      string
	SiteID      string
	CurrentPage string

	Error     string
	Loading   bool
	CanGoBack bool
}

// NewSiteModel builds a model bound to a resolver and an initial
// location. The initial location is the starting point; it is NOT
// pushed to history (back at the first location has nothing to pop).
func NewSiteModel(resolver ContentResolver, initial Location) *SiteModel {
	if resolver == nil {
		panic("workbench: NewSiteModel requires non-nil resolver")
	}
	return &SiteModel{
		resolver: resolver,
		current:  initial,
		dirty:    true,
	}
}

// Current returns the location the model is currently pointed at.
func (m *SiteModel) Current() Location {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current
}

// NavigateTarget classifies a raw link target (as written in markdown
// or a nav menu) relative to the current location and Navigates. The
// renderer passes targets as opaque strings; classification happens
// here so every renderer inherits the same link semantics. Returns
// the classified Location and ok=true if navigation fired; ok=false
// means the target was external (renderer should hand off to OS) or
// malformed (no-op).
func (m *SiteModel) NavigateTarget(target string) (Location, bool) {
	cur := m.Current()
	loc, kind, ok := ClassifyTarget(target, cur)
	if !ok || kind == LinkExternal {
		return Location{}, false
	}
	m.Navigate(loc)
	return loc, true
}

// LinkKind classifies a raw link target.
type LinkKind int

const (
	// LinkInSite — a page within the current site.
	LinkInSite LinkKind = iota
	// LinkCrossSite — a page in a different site on the same peer.
	LinkCrossSite
	// LinkCrossPeer — a page in a site on another peer.
	LinkCrossPeer
	// LinkExternal — a link out of the entity system.
	LinkExternal
)

// ClassifyTarget turns a raw link string into a Location + kind. The
// `current` location supplies the inherited peer + site for relative
// links. Returns ok=false for malformed entity:// URIs (caller should
// treat as external).
//
// Mirrors egui-rust src/content_site/location.rs::classify_link.
func ClassifyTarget(target string, current Location) (Location, LinkKind, bool) {
	t := strings.TrimSpace(target)
	if t == "" {
		return Location{}, LinkExternal, false
	}
	if strings.HasPrefix(t, "http://") ||
		strings.HasPrefix(t, "https://") ||
		strings.HasPrefix(t, "mailto:") {
		return Location{}, LinkExternal, true
	}
	if rest, ok := strings.CutPrefix(t, "entity://"); ok {
		return parseEntityURI(rest, current)
	}
	if rest, ok := strings.CutPrefix(t, "site:"); ok {
		siteID, page := splitFirstSlash(rest)
		return Location{
			PeerID: current.PeerID,
			SiteID: siteID,
			Page:   page,
		}, LinkCrossSite, true
	}
	// In-site link — normalize to root-relative slug. v0: strip leading
	// ./, /, and naive ../ collapse (root-relative has no current dir).
	p := normalizeInSitePage(t)
	return Location{
		PeerID: current.PeerID,
		SiteID: current.SiteID,
		Page:   p,
	}, LinkInSite, true
}

// parseEntityURI parses the part after `entity://` into a cross-peer
// location. Tolerant: locates the `sites` + `pages` segments rather
// than assuming offsets, so the legacy `{peer}/content/sites/...`
// form still resolves. Returns ok=false on malformed.
func parseEntityURI(rest string, _ Location) (Location, LinkKind, bool) {
	segs := splitNonEmpty(rest, "/")
	if len(segs) < 2 {
		return Location{}, LinkExternal, false
	}
	peerID := segs[0]
	sitesIdx := -1
	for i, s := range segs {
		if s == "sites" {
			sitesIdx = i
			break
		}
	}
	if sitesIdx < 0 || sitesIdx+1 >= len(segs) {
		return Location{}, LinkExternal, false
	}
	siteID := segs[sitesIdx+1]
	page := ""
	for i, s := range segs {
		if s == "pages" && i+1 < len(segs) {
			page = strings.Join(segs[i+1:], "/")
			break
		}
	}
	return Location{PeerID: peerID, SiteID: siteID, Page: page}, LinkCrossPeer, true
}

// normalizeInSitePage strips leading ./ , /, and ../ to root-relative.
func normalizeInSitePage(p string) string {
	for {
		switch {
		case strings.HasPrefix(p, "./"):
			p = p[2:]
		case strings.HasPrefix(p, "/"):
			p = p[1:]
		case strings.HasPrefix(p, "../"):
			p = p[3:]
		default:
			return p
		}
	}
}

func splitFirstSlash(s string) (string, string) {
	if i := strings.IndexByte(s, '/'); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

func splitNonEmpty(s, sep string) []string {
	parts := strings.Split(s, sep)
	out := parts[:0]
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Navigate sets the current location and pushes the prior location
// onto the back-history. Idempotent against the same target (rapid
// double-clicks don't double-push). Marks the output dirty and fires
// the change handler.
func (m *SiteModel) Navigate(loc Location) {
	m.mu.Lock()
	if loc == m.current {
		m.mu.Unlock()
		return
	}
	m.history = append(m.history, m.current)
	m.current = loc
	m.dirty = true
	listeners := append([]func(){}, m.listeners...)
	m.mu.Unlock()
	fireAll(listeners)
}

// GoBack pops the back-history. Returns true if anything was popped.
func (m *SiteModel) GoBack() bool {
	m.mu.Lock()
	if len(m.history) == 0 {
		m.mu.Unlock()
		return false
	}
	last := len(m.history) - 1
	prev := m.history[last]
	m.history = m.history[:last]
	m.current = prev
	m.dirty = true
	listeners := append([]func(){}, m.listeners...)
	m.mu.Unlock()
	fireAll(listeners)
	return true
}

// CanGoBack reports whether there is anything in the back-history.
func (m *SiteModel) CanGoBack() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.history) > 0
}

// OnChange registers a handler called when the model's output may
// have changed (after Navigate, GoBack, or an external Invalidate).
// The handler runs on whatever goroutine fired the change — the
// renderer is responsible for marshaling to its UI thread. Returns a
// cancel func; idempotent.
func (m *SiteModel) OnChange(h func()) func() {
	if h == nil {
		return func() {}
	}
	m.mu.Lock()
	m.listeners = append(m.listeners, h)
	idx := len(m.listeners) - 1
	m.mu.Unlock()
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if idx < len(m.listeners) {
			m.listeners[idx] = nil
		}
	}
}

// Invalidate marks the cached output dirty without changing location.
// Used by the bridge when an external event (subscription update,
// cross-peer fetch landing) means the underlying tree may have
// changed.
func (m *SiteModel) Invalidate() {
	m.mu.Lock()
	m.dirty = true
	listeners := append([]func(){}, m.listeners...)
	m.mu.Unlock()
	fireAll(listeners)
}

// Render returns the latest output snapshot. Cheap on a clean cache;
// re-resolves through ContentResolver when the output is dirty.
func (m *SiteModel) Render() SiteRenderOutput {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.dirty {
		return m.output
	}
	m.output = m.buildOutputLocked()
	m.dirty = false
	return m.output
}

// buildOutputLocked resolves the current location and assembles a
// fresh SiteRenderOutput. Caller holds m.mu.
func (m *SiteModel) buildOutputLocked() SiteRenderOutput {
	out := SiteRenderOutput{
		PeerID:      m.current.PeerID,
		SiteID:      m.current.SiteID,
		CurrentPage: m.current.Page,
		CanGoBack:   len(m.history) > 0,
	}

	outcome := m.resolver.ResolvePage(m.current)
	if !outcome.Ready {
		out.Loading = true
		return out
	}
	if outcome.Err != ResolveOK {
		out.Error = outcome.Err.String()
		return out
	}
	rp := outcome.Page

	out.SiteTitle = rp.Manifest.Title
	out.PageTitle = rp.Page.Title()
	out.CurrentPage = rp.Location.Page
	out.PeerID = rp.Location.PeerID
	out.BodyMarkdown = rp.Page.Body
	out.BodyFormat = rp.Page.Format

	rootSlug := rp.Manifest.Root()
	cur := rp.Location.Page

	// Nav: walk manifest.Nav with depth + visited safety.
	out.Nav = buildNavLinks(rp.Manifest.Nav, cur, rootSlug)

	// Breadcrumbs: site-root → ancestor sections → current page label.
	out.Breadcrumbs = buildBreadcrumbs(rp.Manifest.Title, rootSlug, cur, rp.Page.Title())

	// Sidebar: tree-driven from .list (one level + expand active section).
	out.Sidebar = buildSidebar(m.resolver, rp.Location, cur)

	return out
}

// buildNavLinks walks the manifest nav with a depth cap + a visited
// set so a pathological tree (cycle, depth >32) cannot stall the
// renderer. The visited set uses pointer identity; identical-valued
// siblings are not collapsed.
//
// v1 flattens nested nav for the top-bar — children are exposed in
// the sidebar via .list, not via nested NavLinks. So this only walks
// the top level. The depth/visited machinery is in shape ready for a
// future nested-nav rollout.
func buildNavLinks(nav []NavNode, currentPage, rootSlug string) []NavLink {
	out := make([]NavLink, 0, len(nav))
	visited := make(map[*NavNode]bool)
	for i := range nav {
		out = append(out, walkNavNode(&nav[i], currentPage, rootSlug, visited, 0)...)
	}
	return out
}

// walkNavNode emits one NavLink for the node + (currently disabled)
// its children. Defensive: returns empty if visited or over depth.
func walkNavNode(n *NavNode, currentPage, rootSlug string, visited map[*NavNode]bool, depth int) []NavLink {
	if depth >= SiteNavMaxDepth {
		return nil
	}
	if visited[n] {
		return nil
	}
	visited[n] = true

	kind := "page"
	switch {
	case n.Target == "":
		kind = "section-header"
	case strings.HasPrefix(n.Target, "http://"),
		strings.HasPrefix(n.Target, "https://"),
		strings.HasPrefix(n.Target, "mailto:"):
		kind = "external"
	}

	link := NavLink{
		Label:  n.Label,
		Target: n.Target,
		Kind:   kind,
		Active: navActive(n.Target, currentPage, rootSlug),
	}
	out := []NavLink{link}
	// v1: do not flatten children into the top nav. Cap reserved for
	// when nested-nav rendering lands; the safety machinery exercises
	// in tests against pathological inputs.
	_ = depth
	return out
}

// navActive returns true when current is at-or-under the section the
// target points to. Strips ./ , entity:// prefixes naively for v1.
func navActive(target, currentPage, rootSlug string) bool {
	t := strings.TrimPrefix(target, "./")
	t = strings.TrimPrefix(t, "/")
	// Treat the root link as active on the root page.
	if t == "" || t == rootSlug {
		return currentPage == "" || currentPage == rootSlug
	}
	if t == currentPage {
		return true
	}
	// Section trail: current within the same first segment.
	ts := firstSeg(t)
	cs := firstSeg(currentPage)
	return ts != "" && ts == cs
}

func firstSeg(p string) string {
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return p
}

// buildBreadcrumbs assembles the trail to currentPage. Empty on the
// site's root page (no trail to show). Intermediate segments are
// clickable section links; the last segment is the page title with
// empty Target (the "you are here" crumb).
func buildBreadcrumbs(siteTitle, rootSlug, currentPage, pageTitle string) []Crumb {
	if currentPage == "" || currentPage == rootSlug {
		return nil
	}
	out := []Crumb{{Label: siteTitle, Target: "./"}}
	segs := strings.Split(currentPage, "/")
	for i, seg := range segs {
		if seg == "" {
			continue
		}
		if i == len(segs)-1 {
			out = append(out, Crumb{Label: pageTitle, Target: ""})
		} else {
			path := strings.Join(segs[:i+1], "/")
			out = append(out, Crumb{Label: humanize(seg), Target: "./" + path})
		}
	}
	return out
}

// buildSidebar derives the section sidebar from the resolver's
// list_children: top-level entries plus a one-level expansion of the
// active section. Empty for a flat site or a remote whose .list isn't
// wired — the renderer falls back to the simple single-pane layout.
func buildSidebar(resolver ContentResolver, loc Location, currentPage string) []SectionLink {
	top := resolver.ListChildren(loc, "")
	if len(top) == 0 {
		return nil
	}
	curSection := firstSeg(currentPage)
	out := make([]SectionLink, 0, len(top))
	for _, e := range top {
		out = append(out, SectionLink{
			Label:     humanize(e.Name),
			Target:    "./" + e.Name,
			Active:    navActive("./"+e.Name, currentPage, ""),
			Depth:     0,
			IsSection: e.IsSection,
		})
		// Expand the active top-level section one level — depth-bounded
		// (we cap one level here, not the navigation walk).
		if e.IsSection && curSection != "" && e.Name == curSection {
			for _, k := range resolver.ListChildren(loc, e.Name+"/") {
				kpath := e.Name + "/" + k.Name
				active := currentPage == kpath ||
					strings.HasPrefix(currentPage, kpath+"/")
				out = append(out, SectionLink{
					Label:     humanize(k.Name),
					Target:    "./" + kpath,
					Active:    active,
					Depth:     1,
					IsSection: k.IsSection,
				})
			}
		}
	}
	return out
}

// fireAll calls each non-nil listener. Used after we drop the model
// mutex, so a slow handler can't block other Navigate calls.
func fireAll(listeners []func()) {
	for _, h := range listeners {
		if h != nil {
			h()
		}
	}
}
