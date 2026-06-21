package main

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	wb "entity-workbench-go/workbench"
)

// inputMode tracks whether the user is navigating windows or
// interacting with a window's content.
type inputMode int

const (
	modeNormal inputMode = iota // navigate windows, split/close, switch screens
	modeActive                  // interact with focused window's content
)

// screen is an independent layout with its own window tree, focus,
// and selection. Selection is per-presentation-context per the
// workbench convention — each screen has independent navigation
// state.
type screen struct {
	layout  *layoutNode
	focused *consoleWindow
}

// workspace is the window system. Manages screens, layout, focus,
// and action routing. Knows nothing about peers or data.
type workspace struct {
	tviewApp *tview.Application
	bridge   appBridge
	overlay  *tview.Pages
	root     *tview.Flex
	status   *tview.TextView

	screens      [wb.MaxScreens]*screen
	activeScreen int
	nextID       uint32
	mode         inputMode
}

func newWorkspace(bridge appBridge) *workspace {
	ws := &workspace{
		tviewApp: tview.NewApplication(),
		bridge:   bridge,
	}

	ws.status = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)

	ws.root = tview.NewFlex().SetDirection(tview.FlexRow)
	ws.overlay = tview.NewPages().AddPage("main", ws.root, true, true)

	ws.tviewApp.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		return ws.handleGlobalInput(event)
	})

	return ws
}

func (ws *workspace) active() *screen {
	if ws.screens[ws.activeScreen] == nil {
		win := ws.newWindow()
		ws.screens[ws.activeScreen] = &screen{
			layout:  leafNode(win),
			focused: win,
		}
	}
	return ws.screens[ws.activeScreen]
}

func (ws *workspace) switchScreen(idx int) {
	if idx < 0 || idx >= wb.MaxScreens || idx == ws.activeScreen {
		return
	}
	ws.mode = modeNormal
	ws.activeScreen = idx
	ws.rebuildFlexTree()
}

// --- Window management ---

func (ws *workspace) newWindow() *consoleWindow {
	ws.nextID++
	w := &consoleWindow{id: ws.nextID}
	ec := newEmptyContent(ws)
	ec.win = w
	w.content = ec
	return w
}

func (ws *workspace) initLayout() {
	ws.active()
	ws.rebuildFlexTree()
}

func (ws *workspace) rebuildFlexTree() {
	s := ws.active()
	ws.root.Clear()
	content := buildFlex(s.layout)
	ws.root.AddItem(content, 0, 1, true)
	ws.root.AddItem(ws.status, 1, 0, false)

	ws.updateWindowBorders()
	ws.updateStatus()
}

func (ws *workspace) setFocus(win *consoleWindow) {
	ws.active().focused = win
	ws.updateWindowBorders()
	ws.updateStatus()
}

func (ws *workspace) activateWindow() {
	s := ws.active()
	if s.focused == nil {
		return
	}
	ws.mode = modeActive
	ws.tviewApp.SetFocus(s.focused.content.focusTarget())
	ws.updateWindowBorders()
	ws.updateStatus()
}

func (ws *workspace) deactivateWindow() {
	ws.mode = modeNormal
	ws.updateWindowBorders()
	ws.updateStatus()
}

// highlightMode controls window border appearance.
type highlightMode int

const (
	highlightDefault highlightMode = iota
	highlightFocused
	highlightActive
)

func (ws *workspace) updateWindowBorders() {
	s := ws.active()
	for _, w := range s.layout.AllWindows() {
		if w == s.focused {
			if ws.mode == modeActive {
				w.content.setHighlight(highlightActive)
			} else {
				w.content.setHighlight(highlightFocused)
			}
		} else {
			w.content.setHighlight(highlightDefault)
		}
	}
}

// --- Content management ---

func (ws *workspace) setWindowContent(win *consoleWindow, name string) {
	if win == nil {
		return
	}
	if c, ok := win.content.(interface{ close() }); ok {
		c.close()
	}
	content := ws.bridge.createWindowContent(win, name, ws.activeScreen)
	if content == nil {
		return
	}
	win.content = content
	ws.rebuildFlexTree()
}

func (ws *workspace) allWindowsAllScreens() []*consoleWindow {
	var all []*consoleWindow
	for _, s := range ws.screens {
		if s != nil {
			all = append(all, s.layout.AllWindows()...)
		}
	}
	return all
}

// --- Split / Close ---

func (ws *workspace) splitFocused(dir wb.SplitDir) {
	s := ws.active()
	if s.focused == nil {
		return
	}
	r := wb.FindRect(s.layout, s.focused)
	if r != nil {
		const minSplitProportion = 0.15
		switch dir {
		case wb.SplitH:
			if r.W2*0.5 < minSplitProportion {
				return
			}
		case wb.SplitV:
			if r.H*0.5 < minSplitProportion {
				return
			}
		}
	}
	newWin := ws.newWindow()
	s.layout.Split(s.focused, dir, newWin)
	s.focused = newWin
	ws.rebuildFlexTree()
}

func (ws *workspace) closeFocused() {
	s := ws.active()
	if s.focused == nil {
		return
	}
	if len(s.layout.AllWindows()) <= 1 {
		return
	}
	sibling, ok := s.layout.Close(s.focused)
	if ok && sibling != nil {
		s.focused = sibling
	}
	ws.rebuildFlexTree()
}

// --- Focus navigation ---

func (ws *workspace) navigateWindows(dir wb.NavDir) {
	s := ws.active()
	if s.focused == nil {
		return
	}
	if best, ok := wb.Navigate(s.layout, s.focused, dir); ok {
		ws.setFocus(best)
	}
}

func (ws *workspace) cycleFocus() {
	s := ws.active()
	windows := s.layout.AllWindows()
	if len(windows) <= 1 {
		return
	}
	for i, w := range windows {
		if w == s.focused {
			next := windows[(i+1)%len(windows)]
			ws.setFocus(next)
			return
		}
	}
	ws.setFocus(windows[0])
}

// --- Input handling ---

func (ws *workspace) handleGlobalInput(event *tcell.EventKey) *tcell.EventKey {
	if name, _ := ws.overlay.GetFrontPage(); name == "palette" {
		return event
	}

	if event.Key() == tcell.KeyCtrlP {
		ws.openPalette()
		return nil
	}

	if event.Key() == tcell.KeyCtrlC {
		ws.tviewApp.Stop()
		return nil
	}

	switch ws.mode {
	case modeActive:
		return ws.handleActiveInput(event)
	default:
		return ws.handleNormalInput(event)
	}
}

func (ws *workspace) handleNormalInput(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyLeft:
		ws.navigateWindows(wb.NavLeft)
	case tcell.KeyRight:
		ws.navigateWindows(wb.NavRight)
	case tcell.KeyUp:
		ws.navigateWindows(wb.NavUp)
	case tcell.KeyDown:
		ws.navigateWindows(wb.NavDown)
	case tcell.KeyEnter:
		ws.activateWindow()
	case tcell.KeyTab:
		ws.cycleFocus()
	case tcell.KeyCtrlE:
		ws.executeAction(wb.Action{Kind: wb.ActionSetContent, ContentName: "empty"})
	case tcell.KeyRune:
		switch event.Rune() {
		case '\\':
			ws.splitFocused(wb.SplitH)
		case '-':
			ws.splitFocused(wb.SplitV)
		case 'x':
			ws.closeFocused()
		}
		if event.Rune() >= '1' && event.Rune() <= '9' {
			ws.switchScreen(int(event.Rune() - '1'))
		}
	}
	return nil
}

func (ws *workspace) handleActiveInput(event *tcell.EventKey) *tcell.EventKey {
	if event.Key() == tcell.KeyEscape {
		ws.deactivateWindow()
		return nil
	}
	return event
}

// --- Palette ---

func (ws *workspace) openPalette() {
	palette := newCommandPalette(ws)
	ws.overlay.AddPage("palette", palette, true, true)
	ws.tviewApp.SetFocus(palette.input)
}

func (ws *workspace) closePalette() {
	ws.overlay.RemovePage("palette")
	if ws.mode == modeActive {
		s := ws.active()
		if s.focused != nil {
			ws.tviewApp.SetFocus(s.focused.content.focusTarget())
		}
	}
}

// executeAction is the single dispatch point for all actions.
func (ws *workspace) executeAction(action wb.Action) {
	switch action.Kind {
	case wb.ActionSplitH:
		ws.splitFocused(wb.SplitH)
	case wb.ActionSplitV:
		ws.splitFocused(wb.SplitV)
	case wb.ActionCloseWindow:
		ws.closeFocused()
	default:
		ws.bridge.handleAction(action)
	}
}

// --- Status ---

func (ws *workspace) updateStatus() {
	ws.status.Clear()

	modeStr := "[green]NORMAL[-]"
	if ws.mode == modeActive {
		modeStr = "[orange]ACTIVE[-]"
	}

	var screenInd string
	for i := 0; i < wb.MaxScreens; i++ {
		if ws.screens[i] != nil || i == ws.activeScreen {
			if i == ws.activeScreen {
				screenInd += fmt.Sprintf("[white][%d][-]", i+1)
			} else {
				screenInd += fmt.Sprintf("[gray]%d[-]", i+1)
			}
		}
	}

	s := ws.active()
	windowCount := len(s.layout.AllWindows())
	contentType := ""
	if s.focused != nil {
		contentType = s.focused.content.typeName()
	}

	appInfo := ws.bridge.statusInfo()

	hints := "[gray]arrows:nav  Enter:activate  \\:hsplit  -:vsplit  x:close  1-9:screen[-]"
	if ws.mode == modeActive {
		hints = "[gray]Esc:exit  Ctrl+P:commands[-]"
	}

	fmt.Fprintf(ws.status, " %s  %s  [gray]|[-]  [skyblue]%s[-]  [gray]|[-]  %s  [gray]|[-]  [gray]w:%d[-]  [gray]|[-]  %s",
		modeStr, screenInd, contentType, appInfo, windowCount, hints)
}

// --- Run ---

func (ws *workspace) run() error {
	ws.tviewApp.SetRoot(ws.overlay, true)
	for _, w := range ws.allWindowsAllScreens() {
		w.content.refresh()
	}
	ws.updateStatus()
	return ws.tviewApp.Run()
}
