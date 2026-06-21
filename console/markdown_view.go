package main

import (
	"fmt"
	"strings"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	wb "entity-workbench-go/workbench"
)

// markdownViewContent is the console renderer for the markdown view
// panel. Business logic lives in wb.MarkdownViewModel; this file is
// pure tview I/O.
//
// The panel has two display modes:
//
//   - Read mode: title heading + path subtitle + content as plain
//     text. Press 'e' to enter edit mode.
//   - Edit mode: InputField for title + TextArea for content.
//     Ctrl+S saves, Esc discards and exits edit.
//
// Selection wiring: each render, read the selection path. If it
// points at a doc/markdown-file entity, load that path into the model
// and render its content.
type markdownViewContent struct {
	ws       *workspace
	model    *wb.MarkdownViewModel
	eventLog *wb.EventLog
	peerCtx  *wb.PeerContext

	root *tview.Flex

	readHeader *tview.TextView
	readBody   *tview.TextView

	editHeader *tview.Flex
	editTitle  *tview.InputField
	editBody   *tview.TextArea

	mountedEditing bool

	mu              sync.Mutex
	currentPath     string
	cancelSelection func()
	cancelContent   func()
}

func newMarkdownView(ws *workspace, peerCtx *wb.PeerContext, eventLog *wb.EventLog, state *wb.WorkspaceState, screenIdx int) *markdownViewContent {
	av := &markdownViewContent{
		ws:       ws,
		model:    wb.NewMarkdownViewModel(peerCtx),
		eventLog: eventLog,
		peerCtx:  peerCtx,
	}
	if state != nil {
		if sel, ok := state.ReadSelection(screenIdx); ok {
			av.currentPath = sel.Path
			av.rebindContentWatch(sel.Path)
		}
		av.cancelSelection = state.OnSelectionChange(state.ScreenSelectionPath(screenIdx), func(sel wb.Selection) {
			ws.tviewApp.QueueUpdateDraw(func() {
				av.mu.Lock()
				av.currentPath = sel.Path
				av.mu.Unlock()
				av.rebindContentWatch(sel.Path)
				av.render()
			})
		})
	}

	av.readHeader = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true)

	av.readBody = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWordWrap(true)

	av.readBody.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyRune && event.Rune() == 'e' {
			av.enterEdit()
			return nil
		}
		return event
	})

	av.editTitle = tview.NewInputField().
		SetLabel("[skyblue]title:[-] ").
		SetLabelColor(tcell.ColorSkyblue).
		SetFieldBackgroundColor(tcell.ColorBlack)

	av.editTitle.SetChangedFunc(func(text string) {
		av.model.UpdateTitle(text)
	})
	av.editTitle.SetInputCapture(av.editKeyCapture)

	av.editBody = tview.NewTextArea()
	av.editBody.SetChangedFunc(func() {
		av.model.UpdateContent(av.editBody.GetText())
	})
	av.editBody.SetInputCapture(av.editKeyCapture)

	av.editHeader = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(av.editTitle, 1, 0, true).
		AddItem(tview.NewTextView().
			SetDynamicColors(true).
			SetText("[gray]ctrl-s save  esc cancel  tab next field[-]"),
			1, 0, false)

	av.root = tview.NewFlex().SetDirection(tview.FlexRow)
	av.root.SetBorder(true).
		SetTitle(" Markdown ").
		SetTitleAlign(tview.AlignLeft).
		SetBorderColor(tcell.ColorDarkCyan)

	av.mountReadWidgets()

	return av
}

func (av *markdownViewContent) typeName() string             { return "markdown-view" }
func (av *markdownViewContent) widget() tview.Primitive      { return av.root }
func (av *markdownViewContent) handleEvent(e, v string) bool { return false }
func (av *markdownViewContent) setHighlight(m highlightMode) {
	av.root.SetBorderColor(borderColorForMode(m))
}

func (av *markdownViewContent) focusTarget() tview.Primitive {
	if av.model.IsEditing() {
		return av.editTitle
	}
	return av.readBody
}

func (av *markdownViewContent) editKeyCapture(event *tcell.EventKey) *tcell.EventKey {
	switch {
	case event.Key() == tcell.KeyEscape:
		av.model.ExitEdit()
		av.mountReadWidgets()
		return nil
	case event.Key() == tcell.KeyCtrlS:
		if err := av.model.Save(); err != nil {
			if av.eventLog != nil {
				av.eventLog.Appendf("markdown save failed: %s", err)
			}
			return nil
		}
		if av.eventLog != nil {
			av.eventLog.Appendf("markdown saved: %s", av.model.Path())
		}
		av.mountReadWidgets()
		av.ws.bridge.queueRefresh()
		return nil
	case event.Key() == tcell.KeyTab:
		current := av.ws.tviewApp.GetFocus()
		if current == av.editTitle {
			av.ws.tviewApp.SetFocus(av.editBody)
		} else {
			av.ws.tviewApp.SetFocus(av.editTitle)
		}
		return nil
	}
	return event
}

func (av *markdownViewContent) enterEdit() {
	av.model.EnterEdit()
	if !av.model.IsEditing() {
		return
	}
	av.mountEditWidgets()
	out := av.model.Render()
	av.editTitle.SetText(out.Title)
	av.editBody.SetText(out.Content, false)
	av.ws.tviewApp.SetFocus(av.editTitle)
}

func (av *markdownViewContent) mountReadWidgets() {
	av.root.Clear()
	av.root.
		AddItem(av.readHeader, 2, 0, false).
		AddItem(av.readBody, 0, 1, true)
	av.mountedEditing = false
	av.render()
}

func (av *markdownViewContent) mountEditWidgets() {
	av.root.Clear()
	av.root.
		AddItem(av.editHeader, 2, 0, true).
		AddItem(av.editBody, 0, 1, false)
	av.mountedEditing = true
}

// refresh is a no-op: render is driven by OnSelectionChange (path
// changed) + a Store.Watch on currentPath (entity content changed).
// queueRefresh ticks for unrelated paths (heartbeats etc) no longer
// trigger a render here.
func (av *markdownViewContent) refresh() {}

// rebindContentWatch cancels the previous content-watch and registers
// a new one on path. Fires render() when the entity at path is mutated.
func (av *markdownViewContent) rebindContentWatch(path string) {
	if av.cancelContent != nil {
		av.cancelContent()
		av.cancelContent = nil
	}
	if path == "" || av.peerCtx == nil || av.peerCtx.Store() == nil {
		return
	}
	w, err := av.peerCtx.Store().Watch(path)
	if err != nil {
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range w.Events() {
			av.ws.tviewApp.QueueUpdateDraw(func() { av.render() })
		}
	}()
	av.cancelContent = func() { w.Close(); <-done }
}

func (av *markdownViewContent) close() {
	if av.cancelSelection != nil {
		av.cancelSelection()
		av.cancelSelection = nil
	}
	if av.cancelContent != nil {
		av.cancelContent()
		av.cancelContent = nil
	}
}

func (av *markdownViewContent) render() {
	if !av.model.IsEditing() {
		av.mu.Lock()
		path := av.currentPath
		av.mu.Unlock()
		if path != "" {
			av.model.LoadFromPath(path)
		}
	}

	out := av.model.Render()

	if out.Editing && !av.mountedEditing {
		av.mountEditWidgets()
	} else if !out.Editing && av.mountedEditing {
		av.mountReadWidgets()
	}

	if out.Editing {
		dirtyMark := ""
		if out.Dirty {
			dirtyMark = " *"
		}
		av.root.SetTitle(fmt.Sprintf(" Markdown (editing%s) ", dirtyMark))
		return
	}

	av.root.SetTitle(" Markdown ")

	av.readHeader.Clear()
	av.readBody.Clear()

	if out.Empty {
		fmt.Fprintf(av.readHeader, "[gray]No file selected[-]")
		return
	}
	if out.NotFound {
		fmt.Fprintf(av.readHeader, "[gray]File not found:[-] [skyblue]%s[-]", tview.Escape(out.Path))
		return
	}

	fmt.Fprintf(av.readHeader, "[white::b]%s[-::-]  [gray]%s[-]",
		tview.Escape(out.Title), tview.Escape(out.Path))
	for _, line := range strings.Split(out.Content, "\n") {
		fmt.Fprintln(av.readBody, tview.Escape(line))
	}
	fmt.Fprintf(av.readBody, "\n[gray]── press 'e' to edit ──[-]\n")
}
