package main

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	wb "entity-workbench-go/workbench"
)

// markdownFilesContent is the console renderer for the markdown
// files browser, now a folder tree (filtered to doc/markdown-file
// entities). Business logic lives in wb.MarkdownFilesModel; this
// file is pure tview I/O.
//
// Mirrors the structure of treeBrowserContent (tree_view.go) — the
// only differences are the underlying tree source (filtered, not
// raw peerCtx.Entries()) and the leaf labelling (title fallback).
//
// Selection wiring: navigating to a leaf publishes the file path to
// the shared SelectionState, which the markdown view (when present)
// reads to render the selected file. Navigating to a folder row
// publishes the folder path but no entity — markdown view shows
// nothing for it, which is the right behavior.
type markdownFilesContent struct {
	view *tview.TreeView
	ws   *workspace
	tree *wb.TreeNode

	peerCtx *wb.PeerContext
	model   *wb.MarkdownFilesModel

	expanded   map[string]bool
	refreshing bool
}

func newMarkdownFiles(ws *workspace, peerCtx *wb.PeerContext, state *wb.WorkspaceState, screenIdx int) *markdownFilesContent {
	mf := &markdownFilesContent{
		view:     tview.NewTreeView(),
		ws:       ws,
		peerCtx:  peerCtx,
		model:    wb.NewMarkdownFilesModel(peerCtx.Store(), wb.MarkdownFilesPrefix),
		expanded: make(map[string]bool),
	}
	mf.model.BindSelection(state, screenIdx)

	root := tview.NewTreeNode("markdown").SetColor(tcell.ColorWhite)
	mf.view.SetRoot(root).SetCurrentNode(root)
	mf.view.SetBorder(true).
		SetTitle(" Markdown Files ").
		SetTitleAlign(tview.AlignLeft).
		SetBorderColor(tcell.ColorDarkCyan)

	// Enter: toggle expand for folders, select for files.
	mf.view.SetSelectedFunc(func(node *tview.TreeNode) {
		if mf.refreshing {
			return
		}
		ref := node.GetReference()
		if ref == nil {
			return
		}
		wn := ref.(*wb.TreeNode)
		if len(wn.Children) > 0 {
			if mf.expanded[wn.FullPath] {
				delete(mf.expanded, wn.FullPath)
				node.SetExpanded(false)
			} else {
				mf.expanded[wn.FullPath] = true
				node.SetExpanded(true)
			}
		}
		// Selection writes the screen slot; subscribers (inspector,
		// markdown-view) re-render through OnSelectionChange. No
		// explicit refresh needed.
		mf.model.PublishSelection(wn.FullPath)
	})

	// Arrow keys: update selection state live.
	mf.view.SetChangedFunc(func(node *tview.TreeNode) {
		if mf.refreshing {
			return
		}
		ref := node.GetReference()
		if ref == nil {
			return
		}
		wn := ref.(*wb.TreeNode)
		mf.model.PublishSelection(wn.FullPath)
	})

	return mf
}

func (mf *markdownFilesContent) typeName() string             { return "markdown-files" }
func (mf *markdownFilesContent) widget() tview.Primitive      { return mf.view }
func (mf *markdownFilesContent) focusTarget() tview.Primitive { return mf.view }
func (mf *markdownFilesContent) handleEvent(e, v string) bool { return false }
func (mf *markdownFilesContent) setHighlight(m highlightMode) {
	mf.view.SetBorderColor(borderColorForMode(m))
}

func (mf *markdownFilesContent) refresh() {
	// Idempotent path: if the underlying filtered entry set hasn't
	// changed, leave tview alone so the user's keyboard navigation
	// state (current node, scroll position) survives the tick. The
	// console fires queueRefresh on every tree event — including the
	// 2s test/tick writes and revision-converge writes — so without
	// this guard the user can't navigate without their selection
	// being stomped.
	changed := mf.model.Refresh()
	if !changed && mf.tree != nil {
		return
	}

	selectedPath := mf.model.SelectedPath()

	mf.refreshing = true
	defer func() { mf.refreshing = false }()

	mf.tree = mf.model.Root()

	root := mf.view.GetRoot()
	root.ClearChildren()
	if mf.tree == nil {
		root.AddChild(tview.NewTreeNode("(no markdown files)").SetColor(tcell.ColorGray))
		return
	}
	wb.RestoreExpanded(mf.tree, mf.expanded)
	mf.addChildren(root, mf.tree)

	if selectedPath != "" {
		mf.selectByPath(root, selectedPath)
	}
}

func (mf *markdownFilesContent) selectByPath(node *tview.TreeNode, path string) bool {
	ref := node.GetReference()
	if ref != nil {
		if wn, ok := ref.(*wb.TreeNode); ok && wn.FullPath == path {
			mf.view.SetCurrentNode(node)
			return true
		}
	}
	for _, child := range node.GetChildren() {
		if mf.selectByPath(child, path) {
			return true
		}
	}
	return false
}

// addChildren walks the filtered tree and builds tview nodes. Leaves
// show "Segment — Title" when a title is resolved (title differs
// from filename); otherwise just the segment.
func (mf *markdownFilesContent) addChildren(parent *tview.TreeNode, wbNode *wb.TreeNode) {
	for _, child := range wbNode.Children {
		label := child.Segment
		color := tcell.ColorGray
		if child.HasEntry {
			color = tcell.ColorSkyblue
			if t := mf.model.TitleFor(child.FullPath); t != "" && t != child.Segment {
				label = fmt.Sprintf("%s — %s", child.Segment, t)
			}
		}
		if len(child.Children) > 0 && !child.HasEntry {
			label = fmt.Sprintf("%s (%d)", label, wb.CountLeaves(child))
		}

		node := tview.NewTreeNode(label).
			SetReference(child).
			SetColor(color).
			SetExpanded(child.Expanded).
			SetSelectable(true)

		if len(child.Children) > 0 {
			mf.addChildren(node, child)
		}
		parent.AddChild(node)
	}
}
