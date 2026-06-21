package main

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	wb "entity-workbench-go/workbench"
)

// queryBrowserContent is the console renderer for the query browser.
// Business logic lives in wb.QueryModel; this file is tview I/O.
type queryBrowserContent struct {
	flex       *tview.Flex
	typeInput  *tview.InputField
	pathInput  *tview.InputField
	resultView *tview.TextView
	model      *wb.QueryModel
	ws         *workspace

	// focus cycling: 0=type, 1=path, 2=results
	focusIdx int
}

func newQueryBrowser(ws *workspace, peerCtx *wb.PeerContext, state *wb.WorkspaceState, screenIdx int) *queryBrowserContent {
	q := &queryBrowserContent{
		model: wb.NewQueryModel(peerCtx),
		ws:    ws,
	}
	q.model.BindSelection(state, screenIdx)

	q.typeInput = tview.NewInputField().
		SetLabel("Type: ").
		SetFieldWidth(0)
	q.typeInput.SetDoneFunc(func(key tcell.Key) {
		q.model.TypeFilter = q.typeInput.GetText()
		switch key {
		case tcell.KeyEnter:
			q.executeQuery()
		case tcell.KeyTab:
			q.cycleFocus(1)
		case tcell.KeyBacktab:
			q.cycleFocus(-1)
		}
	})

	q.pathInput = tview.NewInputField().
		SetLabel("Path: ").
		SetFieldWidth(0)
	q.pathInput.SetDoneFunc(func(key tcell.Key) {
		q.model.PathPrefix = q.pathInput.GetText()
		switch key {
		case tcell.KeyEnter:
			q.executeQuery()
		case tcell.KeyTab:
			q.cycleFocus(1)
		case tcell.KeyBacktab:
			q.cycleFocus(-1)
		}
	})

	q.resultView = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true)
	q.resultView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyDown:
			q.model.SelectNext()
			q.drawResults()
			q.publishSelection()
			return nil
		case tcell.KeyUp:
			q.model.SelectPrev()
			q.drawResults()
			q.publishSelection()
			return nil
		case tcell.KeyTab:
			q.cycleFocus(1)
			return nil
		case tcell.KeyBacktab:
			q.cycleFocus(-1)
			return nil
		}
		switch event.Rune() {
		case 'n':
			if q.model.Render().HasMore {
				q.model.NextPage()
				q.drawResults()
				q.publishSelection()
			}
			return nil
		}
		return event
	})

	q.flex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(q.typeInput, 1, 0, true).
		AddItem(q.pathInput, 1, 0, false).
		AddItem(q.resultView, 0, 1, false)

	q.flex.SetBorder(true).SetTitle(" Query Browser ").SetTitleAlign(tview.AlignLeft)
	q.flex.SetBorderColor(tcell.ColorDarkCyan)

	return q
}

func (q *queryBrowserContent) typeName() string             { return "query-browser" }
func (q *queryBrowserContent) widget() tview.Primitive      { return q.flex }
func (q *queryBrowserContent) focusTarget() tview.Primitive { return q.focusedWidget() }
func (q *queryBrowserContent) handleEvent(e, v string) bool { return false }
func (q *queryBrowserContent) setHighlight(m highlightMode) {
	q.flex.SetBorderColor(borderColorForMode(m))
}

func (q *queryBrowserContent) refresh() {
	q.drawResults()
}

func (q *queryBrowserContent) executeQuery() {
	q.model.TypeFilter = q.typeInput.GetText()
	q.model.PathPrefix = q.pathInput.GetText()
	q.model.Execute()
	q.drawResults()
	q.publishSelection()
	// Move focus to results after executing
	q.focusIdx = 2
	q.ws.tviewApp.SetFocus(q.resultView)
}

func (q *queryBrowserContent) cycleFocus(dir int) {
	q.focusIdx = (q.focusIdx + dir + 3) % 3
	q.ws.tviewApp.SetFocus(q.focusedWidget())
}

func (q *queryBrowserContent) focusedWidget() tview.Primitive {
	switch q.focusIdx {
	case 1:
		return q.pathInput
	case 2:
		return q.resultView
	default:
		return q.typeInput
	}
}

func (q *queryBrowserContent) drawResults() {
	q.resultView.Clear()

	out := q.model.Render()

	fmt.Fprintf(q.resultView, "[skyblue]%s[-]\n\n", tview.Escape(out.Status))

	for i, match := range out.Matches {
		prefix := "  "
		if i == out.Selected {
			prefix = "[::r] "
		}
		path := match.Path
		if path == "" {
			path = "(no path)"
		}
		fmt.Fprintf(q.resultView, "%s[white]%s[-] [gray]%s[-]", prefix, tview.Escape(path), tview.Escape(match.Type))
		if i == out.Selected {
			fmt.Fprint(q.resultView, " [::-]")
		}
		fmt.Fprint(q.resultView, "\n")
	}

	if out.HasMore {
		fmt.Fprintf(q.resultView, "\n[yellow]press n for next page[-]")
	}
}

func (q *queryBrowserContent) publishSelection() {
	// Writes the screen slot; subscribers re-render via
	// OnSelectionChange. No explicit refresh needed.
	q.model.PublishSelection()
}
