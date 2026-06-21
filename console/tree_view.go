package main

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	wb "entity-workbench-go/workbench"
)

// treeBrowserContent wraps tview.TreeView and implements windowContent.
//
// State lives in wb.TreeBrowserModel, which subscribes to the peer's
// whole-tree change stream and maintains its internal *TreeNode
// incrementally — no per-refresh enumeration. The tview side mirrors
// the model's tree shape on each refresh tick.
type treeBrowserContent struct {
	view *tview.TreeView
	ws   *workspace // structural ops only (refresh notifications)

	// Data bindings — received at creation, not owned.
	peerCtx *wb.PeerContext
	model   *wb.TreeBrowserModel

	// UI state — owned by this struct, drives what tview shows.
	expanded   map[string]bool
	refreshing bool
}

func newTreeBrowser(ws *workspace, peerCtx *wb.PeerContext, state *wb.WorkspaceState, screenIdx int) *treeBrowserContent {
	tb := &treeBrowserContent{
		view:     tview.NewTreeView(),
		ws:       ws,
		peerCtx:  peerCtx,
		model:    wb.NewTreeBrowserModel(peerCtx),
		expanded: make(map[string]bool),
	}
	tb.model.BindSelection(state, screenIdx)

	root := tview.NewTreeNode("entities").SetColor(tcell.ColorWhite)
	tb.view.SetRoot(root).SetCurrentNode(root)
	tb.view.SetBorder(true).SetTitle(" Tree Browser ").SetTitleAlign(tview.AlignLeft)
	tb.view.SetBorderColor(tcell.ColorDarkCyan)

	// Enter key: toggle expand (updates struct state), select entity.
	tb.view.SetSelectedFunc(func(node *tview.TreeNode) {
		if tb.refreshing {
			return
		}
		ref := node.GetReference()
		if ref == nil {
			return
		}
		wn := ref.(*wb.TreeNode)
		if len(wn.Children) > 0 {
			if tb.expanded[wn.FullPath] {
				delete(tb.expanded, wn.FullPath)
				node.SetExpanded(false)
			} else {
				tb.expanded[wn.FullPath] = true
				// Lazy-build real children on first expand. If the
				// tview node has only a placeholder, swap it for real
				// nodes built from the wb subtree.
				hydrateChildren(node, wn)
				node.SetExpanded(true)
			}
		}
		// Selection publishes to the per-screen slot; inspector +
		// markdown-view re-render via their OnSelectionChange
		// subscriptions. No explicit refreshViewsForSelection needed
		// (that would cause a double-render).
		tb.model.PublishSelectionPath(wn.FullPath)
	})

	// Arrow keys: update selection state.
	tb.view.SetChangedFunc(func(node *tview.TreeNode) {
		if tb.refreshing {
			return
		}
		ref := node.GetReference()
		if ref == nil {
			return
		}
		wn := ref.(*wb.TreeNode)
		tb.model.PublishSelectionPath(wn.FullPath)
	})

	return tb
}

func (tb *treeBrowserContent) typeName() string {
	return "tree-browser"
}

func (tb *treeBrowserContent) widget() tview.Primitive {
	return tb.view
}

func (tb *treeBrowserContent) focusTarget() tview.Primitive {
	return tb.view
}

func (tb *treeBrowserContent) refresh() {
	selectedPath := tb.model.SelectedPath()

	// Skip the tview rebuild entirely if the model hasn't changed
	// since the last refresh AND the tview tree is already populated.
	// Selection changes still need selectByPath; handle that below.
	modelChanged := tb.model.Refresh()
	if !modelChanged && tb.view.GetRoot() != nil && len(tb.view.GetRoot().GetChildren()) > 0 {
		if selectedPath != "" {
			tb.selectByPath(tb.view.GetRoot(), selectedPath)
		}
		return
	}

	tb.refreshing = true
	defer func() { tb.refreshing = false }()

	wbRoot := tb.model.Root
	if wbRoot != nil {
		wb.RestoreExpanded(wbRoot, tb.expanded)
	}

	root := tb.view.GetRoot()
	root.ClearChildren()
	if wbRoot != nil {
		addChildren(root, wbRoot)
	}

	if selectedPath != "" {
		tb.selectByPath(root, selectedPath)
	}
}

// close cancels the model's subscription on window teardown.
func (tb *treeBrowserContent) close() {
	if tb.model != nil {
		tb.model.Close()
	}
}

func (tb *treeBrowserContent) selectByPath(node *tview.TreeNode, path string) bool {
	ref := node.GetReference()
	if ref != nil {
		if wn, ok := ref.(*wb.TreeNode); ok && wn.FullPath == path {
			tb.view.SetCurrentNode(node)
			return true
		}
	}
	for _, child := range node.GetChildren() {
		if tb.selectByPath(child, path) {
			return true
		}
	}
	return false
}

func (tb *treeBrowserContent) handleEvent(event string, value string) bool {
	return false
}

func (tb *treeBrowserContent) setHighlight(mode highlightMode) {
	tb.view.SetBorderColor(borderColorForMode(mode))
}

// addChildren mirrors wbNode's children into tview, but ONLY recurses
// into expanded subtrees. Collapsed folders with wb children get a
// placeholder tview child so tview still shows the expand indicator
// — the placeholder is replaced with real children on first expand
// (lazy hydration via SetSelectedFunc).
//
// This keeps each refresh O(currently-visible-rows) instead of O(total
// paths). With a 14K-path store + a typical "few folders expanded"
// session, that's a ~280× cut in per-refresh work.
func addChildren(parent *tview.TreeNode, wbNode *wb.TreeNode) {
	for _, child := range wbNode.Children {
		label := child.Segment
		color := tcell.ColorGray
		if child.HasEntry {
			color = tcell.ColorSkyblue
		}

		hasWBChildren := len(child.Children) > 0
		if hasWBChildren && !child.HasEntry {
			count := wb.CountLeaves(child)
			label = fmt.Sprintf("%s (%d)", label, count)
		}

		node := tview.NewTreeNode(label).
			SetReference(child).
			SetColor(color).
			SetExpanded(child.Expanded).
			SetSelectable(true)

		if hasWBChildren {
			if child.Expanded {
				addChildren(node, child)
			} else {
				// Placeholder — tview needs at least one child to
				// render the [+] expand indicator. Replaced with real
				// children on first expand. Reference is nil so the
				// SetSelected handler can distinguish.
				node.AddChild(tview.NewTreeNode("…").SetSelectable(false))
			}
		}
		parent.AddChild(node)
	}
}

// hydrateChildren replaces a placeholder under `tviewNode` with real
// tview children built from the corresponding wbNode. No-op if the
// node already has real children (reference != nil on first child).
func hydrateChildren(tviewNode *tview.TreeNode, wbNode *wb.TreeNode) {
	children := tviewNode.GetChildren()
	if len(children) > 0 && children[0].GetReference() != nil {
		return // already hydrated
	}
	tviewNode.ClearChildren()
	addChildren(tviewNode, wbNode)
}
