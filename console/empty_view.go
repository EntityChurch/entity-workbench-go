package main

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// emptyContent is a blank panel. Use Ctrl+P to set content.
type emptyContent struct {
	view *tview.TextView
	ws   *workspace
	win  *consoleWindow
}

func newEmptyContent(ws *workspace) *emptyContent {
	ec := &emptyContent{
		view: tview.NewTextView().
			SetDynamicColors(true).
			SetTextAlign(tview.AlignCenter),
		ws: ws,
	}

	ec.view.SetText("\n\n[gray]Ctrl+P to set content[-]")
	ec.view.SetBorder(true).SetTitle(" Empty ").SetTitleAlign(tview.AlignLeft)
	ec.view.SetBorderColor(tcell.ColorDarkCyan)

	return ec
}

func (ec *emptyContent) typeName() string {
	return "empty"
}

func (ec *emptyContent) widget() tview.Primitive {
	return ec.view
}

func (ec *emptyContent) focusTarget() tview.Primitive {
	return ec.view
}

func (ec *emptyContent) refresh() {}

func (ec *emptyContent) handleEvent(event string, value string) bool {
	return false
}

func (ec *emptyContent) setHighlight(mode highlightMode) {
	ec.view.SetBorderColor(borderColorForMode(mode))
}
