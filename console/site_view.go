package main

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	wb "entity-workbench-go/workbench"
)

// siteViewContent is the console renderer for the SITE convention
// (`app/site-manifest` + `app/site-page`, v0.5). Business logic lives
// in wb.SiteModel; this file is pure tview I/O — text only, no
// markdown styling. The Avalonia renderer carries the rich rendering
// surface (P1 adaptive emit, breadcrumb chrome, sidebar widget); the
// TUI's role is the headless / SSH / single-peer use case where
// "show me the bytes" is enough.
//
// D18 (renderer-agnostic substrate): this file exists so a model
// contract change breaks the console build, not just the Avalonia
// build. The two-renderer compile gate is the cheapest enforcement
// we have.
//
// Minimum viable: site title + breadcrumb trail (one line) +
// sidebar (one column) + body as plain text. Polish (tab cycling,
// nav-bar key bindings, edit mode) deferred until the SITE feature
// matures past read-only v1.
type siteViewContent struct {
	view  *tview.TextView
	model *wb.SiteModel
}

// newSiteView builds a SiteView panel pointed at one site on the
// peerCtx's bound peer. siteID is required; an empty page slug
// resolves to the manifest's params.root.
//
// peerCtx must come from a real peer; tests stub via the
// PeerContext returned by testPeerContext.
func newSiteView(peerCtx *wb.PeerContext, siteID string) *siteViewContent {
	if peerCtx == nil {
		panic("console: newSiteView requires non-nil peerCtx")
	}
	if siteID == "" {
		panic("console: newSiteView requires non-empty siteID")
	}
	peerID := peerCtx.Executor().PeerID()
	resolver := wb.NewLocalTreeResolver(peerCtx.Store(), peerID)
	model := wb.NewSiteModel(resolver, wb.Location{SiteID: siteID})

	sv := &siteViewContent{
		view: tview.NewTextView().
			SetDynamicColors(true).
			SetScrollable(true).
			SetWordWrap(true),
		model: model,
	}
	sv.view.SetBorder(true).
		SetTitle(" Site ").
		SetTitleAlign(tview.AlignLeft).
		SetBorderColor(tcell.ColorDarkCyan)
	return sv
}

func (sv *siteViewContent) typeName() string             { return "site-view" }
func (sv *siteViewContent) widget() tview.Primitive      { return sv.view }
func (sv *siteViewContent) focusTarget() tview.Primitive { return sv.view }
func (sv *siteViewContent) handleEvent(e, v string) bool { return false }
func (sv *siteViewContent) setHighlight(m highlightMode) {
	sv.view.SetBorderColor(borderColorForMode(m))
}

func (sv *siteViewContent) refresh() {
	sv.view.Clear()
	out := sv.model.Render()

	if out.Error != "" {
		fmt.Fprintf(sv.view, "[red]error:[-] %s\n", tview.Escape(out.Error))
		return
	}
	if out.Loading {
		fmt.Fprintln(sv.view, "[gray]loading...[-]")
		return
	}

	fmt.Fprintf(sv.view, "[skyblue]%s[-]\n", tview.Escape(out.SiteTitle))

	if len(out.Breadcrumbs) > 0 {
		parts := make([]string, 0, len(out.Breadcrumbs))
		for _, c := range out.Breadcrumbs {
			parts = append(parts, tview.Escape(c.Label))
		}
		fmt.Fprintf(sv.view, "[gray]%s[-]\n", strings.Join(parts, " > "))
	}

	if len(out.Sidebar) > 0 {
		fmt.Fprintf(sv.view, "\n[yellow]Sections[-]\n")
		for _, s := range out.Sidebar {
			indent := strings.Repeat("  ", s.Depth)
			marker := "  "
			if s.Active {
				marker = "[orange]>[-] "
			}
			fmt.Fprintf(sv.view, "  %s%s%s\n", indent, marker, tview.Escape(s.Label))
		}
	}

	fmt.Fprintf(sv.view, "\n[white]%s[-]\n", tview.Escape(out.PageTitle))
	fmt.Fprintf(sv.view, "%s\n", tview.Escape(out.BodyMarkdown))
}
