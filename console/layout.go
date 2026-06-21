package main

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	wb "entity-workbench-go/workbench"
)

// borderColorForMode returns the border color for a window's highlight state.
func borderColorForMode(mode highlightMode) tcell.Color {
	switch mode {
	case highlightFocused:
		return tcell.ColorSkyblue
	case highlightActive:
		return tcell.ColorOrange
	default:
		return tcell.ColorDarkCyan
	}
}

// --- Window Content Interface ---

// windowContent is the interface for panel renderers.
type windowContent interface {
	typeName() string
	widget() tview.Primitive
	focusTarget() tview.Primitive
	refresh()
	setHighlight(mode highlightMode)
	handleEvent(event string, value string) bool
}

// --- Console Window ---

// consoleWindow is a leaf in the layout tree.
type consoleWindow struct {
	id      uint32
	content windowContent

	// Data bindings — content types receive these at creation time.
	peerCtx *wb.PeerContext
}

// --- Layout Tree (uses generic workbench.LayoutNode) ---

// layoutNode is the console's layout tree, parameterized on consoleWindow.
type layoutNode = wb.LayoutNode[*consoleWindow]

// leafNode creates a leaf layout node.
func leafNode(w *consoleWindow) *layoutNode {
	return wb.LeafNode(w)
}

// --- Build tview Flex Tree ---

// buildFlex recursively creates a tview widget tree from the layout tree.
func buildFlex(n *layoutNode) tview.Primitive {
	if n.IsLeaf() {
		return n.Win.content.widget()
	}

	var direction int
	switch n.Dir {
	case wb.SplitH:
		direction = tview.FlexColumn
	case wb.SplitV:
		direction = tview.FlexRow
	}

	flex := tview.NewFlex().SetDirection(direction)
	flex.AddItem(buildFlex(n.First), 0, 1, false)
	flex.AddItem(buildFlex(n.Second), 0, 1, false)
	return flex
}
