package main

import (
	"fmt"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/shellboot"
	"entity-workbench-go/shellcmd"
	wb "entity-workbench-go/workbench"
)

// appBridge is the contract between workspace (UI system) and
// application (state + content). The workspace never imports peer
// or knows about entity data — it calls through this interface.
type appBridge interface {
	createWindowContent(win *consoleWindow, name string, screenIdx int) windowContent
	handleAction(action wb.Action)
	queueRefresh()
	statusInfo() string
}

// application is the outermost layer. It owns application state
// (managed peers, data contexts), determines available actions,
// and creates content for windows with the right data bindings.
//
// Phase I §13: the application now sources its peers from a
// shellboot.PeerManager (the same renderer-neutral type the Avalonia
// bridge consumes per PLAN §12). The local `peers` slice mirrors the
// manager's roster for the console's own use; OnPeerDestroyed keeps
// the mirror in sync when a peer goes through the manager.
//
// Today the console's multi-peer UX is minimal — it boots ONE peer
// from argv flags and that's the active peer for life. The picker
// modal / multi-peer switch UI is a tview-design follow-up; the
// refactor here is what proves the renderer-neutral discipline is
// real (PeerManager has two consumers; the model layer doesn't leak
// into either renderer).
type application struct {
	ws *workspace

	// PeerManager (Phase I §12). Owns peer lifecycle; the application
	// reads + listens via OnPeerDestroyed. Set in main.go before
	// addManagedPeer is called.
	manager *shellboot.PeerManager

	// Managed peers — local mirror of the manager's roster, indexed
	// for fast lookup by the application's UI code.
	peers []*managedPeer

	// Active data bindings for new windows
	activePeerCtx *wb.PeerContext

	// Application-wide event log
	eventLog *wb.EventLog

	// Entity-backed workspace state
	state *wb.WorkspaceState

	// shWs is the shellcmd ShellWorkspace shared across all embedded
	// shell panels — bound at the first addManagedPeer call. One per
	// process per active identity (v1 single-identity; multi-peer
	// picker UI will swap this when active peer changes).
	shWs *shellcmd.ShellWorkspace

	// Track last synced active-screen index to avoid redundant tree writes
	lastSyncedScreen int
}

// managedPeer is a peer the application drives. AppPeer is the SDK
// entry point (handlers, executor, event log are all built-in and
// shared with the shellboot workspace).
//
// Phase I §13: `handle` is the shellboot.PeerManager handle so we can
// route destroy + lookups through the manager. Stays as a lightweight
// local mirror — we don't want to thread a HostedPeer pointer through
// every panel ctor that historically took peerCtx.
type managedPeer struct {
	handle  int64
	name    string
	app     *entitysdk.AppPeer
	peerCtx *wb.PeerContext
}

// newApplication constructs the console application with a fresh
// PeerManager. main.go bootstraps the first peer through the manager,
// then calls addManagedPeer to register it with the application.
func newApplication() *application {
	app := &application{
		manager: shellboot.NewPeerManager(consoleAppID),
	}
	app.ws = newWorkspace(app)
	// Cascade hook: when the manager destroys a peer (today only on
	// process exit via ShutdownAll; future via shell verbs or a
	// picker action), drop the local mirror entry. Panels bound to
	// the destroyed peerCtx will stop receiving events when the
	// underlying AppPeer closes — they don't need to be torn down by
	// the application itself.
	app.manager.OnPeerDestroyed(func(h int64) {
		app.dropManagedPeer(h)
	})
	return app
}

// consoleAppID is the namespace the console writes its roster under.
// Matches the cross-impl convention (godot: "godot-workbench"; egui:
// "entity-browser"; avalonia: "entity-workbench"). Shared with the
// Avalonia bridge so a workbench-go user running both can see one
// roster across launches — same identity, same peer-ids.
const consoleAppID = "entity-workbench"

// addManagedPeer registers a peer the application's PeerManager just
// minted. Mirrors the manager's HostedPeer into the application's own
// peers slice for fast UI-side lookup. The first peer added becomes
// the active data binding (sets activePeerCtx + eventLog + state +
// shWs).
//
// Phase I §13: replaces the old addPeer(name, ap, shWs) signature.
// main.go now calls manager.Create(cfg) + addManagedPeer(hp); the
// roster + cascade cleanup live in the manager.
func (app *application) addManagedPeer(hp *shellboot.HostedPeer) *managedPeer {
	mp := &managedPeer{
		handle:  hp.Handle,
		name:    hp.Workspace.Local.Alias,
		app:     hp.AppPeer,
		peerCtx: hp.AppPeer.PeerContext(),
	}
	app.peers = append(app.peers, mp)

	if app.activePeerCtx == nil {
		app.activePeerCtx = mp.peerCtx
		app.eventLog = hp.AppPeer.EventLog()
		app.state = wb.NewWorkspaceState(hp.AppPeer.Store())
		app.shWs = hp.Workspace

		// Shared shell↔workspace integration: alias persistence.
		// The per-shell-panel WD publisher is built per-panel inside
		// createWindowContent so it knows the panel-id.
		hp.Workspace.PersistAliases(app.state)
	}

	if app.eventLog != nil {
		app.eventLog.Appendf("peer added: %s (handle=%d)", mp.name, mp.handle)
	}
	return mp
}

// dropManagedPeer removes a peer from the local mirror. Called from
// the OnPeerDestroyed hook the application registers with the manager
// in newApplication. Does NOT itself close the AppPeer — the manager
// owns that (and has already done it by the time this hook fires).
//
// If the destroyed peer was the active binding, activePeerCtx is
// cleared. Today nothing automatically promotes a replacement; the
// future picker UI / first new addManagedPeer will rebind.
func (app *application) dropManagedPeer(handle int64) {
	out := app.peers[:0]
	var dropped *managedPeer
	for _, mp := range app.peers {
		if mp.handle == handle {
			dropped = mp
			continue
		}
		out = append(out, mp)
	}
	app.peers = out
	if dropped == nil {
		return
	}
	if app.activePeerCtx == dropped.peerCtx {
		app.activePeerCtx = nil
		// shWs + state + eventLog still point at the destroyed peer's
		// objects. They'll be rebound on the next addManagedPeer.
	}
	if app.eventLog != nil {
		app.eventLog.Appendf("peer removed: %s (handle=%d)", dropped.name, handle)
	}
}

// dispatchExecute runs a handler operation on the active peer. Routes
// through shellcmd.Exec so the panel shares the canonical exec op
// with the CLI's `exec` verb — qualification, params encoding, and
// resource handling live in one place.
func (app *application) dispatchExecute(path, operation string) (*entitysdk.Response, error) {
	if app.shWs == nil {
		return nil, fmt.Errorf("no peer available")
	}
	resp, err := shellcmd.Exec(app.shWs.Local, path, operation, nil, nil)
	if err != nil {
		app.eventLog.Appendf("execute error: %s %s: %s", path, operation, err)
		return nil, err
	}
	app.eventLog.Appendf("execute: %s %s → %d", path, operation, resp.Status)
	return resp, nil
}

// runActions executes a sequence of actions.
func (app *application) runActions(actions []wb.Action) {
	for _, action := range actions {
		app.logAction(action)
		app.ws.executeAction(action)
	}
}

// buildScreenFromConfig creates a layout tree from a shared
// ScreenConfig. Each panel binds its model to the screen's selection
// slot (screenIdx) at construction; selection is per-screen so windows
// on different screens are independent.
func (app *application) buildScreenFromConfig(cfg *wb.ScreenConfig, screenIdx int) *layoutNode {
	if cfg.IsLeaf() {
		win := app.ws.newWindow()
		content := app.createWindowContent(win, cfg.Content, screenIdx)
		if content != nil {
			win.content = content
		}
		// Apply per-window settings
		for k, v := range cfg.Settings {
			if app.state != nil {
				app.state.SaveWindowSetting(win.id, k, v)
			}
			if k == "log-display-level" {
				if lv, ok := win.content.(*logViewerContent); ok {
					lv.model.DisplayLevel = wb.ParseLevelName(v)
					lv.updateTitle()
				}
			}
		}
		return wb.LeafNode(win)
	}
	first := app.buildScreenFromConfig(cfg.First, screenIdx)
	second := app.buildScreenFromConfig(cfg.Second, screenIdx)
	return wb.SplitNode(cfg.Dir, first, second)
}

// setupDefaultScreens builds all screens from the shared default config.
func (app *application) setupDefaultScreens() {
	screens := wb.DefaultScreens()
	for i, cfg := range screens {
		layout := app.buildScreenFromConfig(cfg, i)
		wins := layout.AllWindows()
		focused := wins[0]
		app.ws.screens[i] = &screen{
			layout:  layout,
			focused: focused,
		}
	}
	app.ws.activeScreen = 0
	app.ws.rebuildFlexTree()
}

// setLogViewerDisplay sets the display level on a log viewer window.
func (app *application) setLogViewerDisplay(win *consoleWindow, level wb.LogLevel) {
	if win == nil {
		return
	}
	if lv, ok := win.content.(*logViewerContent); ok {
		lv.model.DisplayLevel = level
		lv.updateTitle()
		lv.fullRerender()
		// Persistence happens through model.BindState — no manual save needed
	}
}

func (app *application) logAction(action wb.Action) {
	switch action.Kind {
	case wb.ActionSetContent:
		// logged in handleAction with window ID
	case wb.ActionSplitH:
		app.eventLog.Appendf("action split-horizontal")
	case wb.ActionSplitV:
		app.eventLog.Appendf("action split-vertical")
	case wb.ActionCloseWindow:
		app.eventLog.Appendf("action close-window")
	case wb.ActionWindowEvent:
		app.eventLog.Appendf("action window-event %s=%s", action.Event, action.Value)
	}
}

// --- Workspace State (entity-backed via WorkspaceState) ---

func (app *application) saveWindowState(win *consoleWindow) {
	if app.state == nil || win == nil {
		return
	}
	app.state.SaveWindowContent(win.id, win.content.typeName())
}

func (app *application) saveScreenState() {
	if app.state == nil {
		return
	}
	app.state.SaveActiveScreen(app.ws.activeScreen)
}

func (app *application) saveAllWindowStates() {
	if app.state == nil {
		return
	}
	for i, s := range app.ws.screens {
		if s == nil {
			continue
		}
		for _, win := range s.layout.AllWindows() {
			app.state.SaveWindowContent(win.id, win.content.typeName())
			app.state.SaveWindowScreen(win.id, i)
		}
	}
}

// --- appBridge implementation ---

func (app *application) createWindowContent(win *consoleWindow, name string, screenIdx int) windowContent {
	switch name {
	case "tree-browser":
		if app.activePeerCtx == nil {
			return nil
		}
		win.peerCtx = app.activePeerCtx
		return newTreeBrowser(app.ws, win.peerCtx, app.state, screenIdx)

	case "entity-detail":
		if app.activePeerCtx == nil {
			return nil
		}
		win.peerCtx = app.activePeerCtx
		return newEntityDetail(app.ws, win.peerCtx, app.state, screenIdx)

	case "peer-info":
		if app.activePeerCtx == nil {
			return nil
		}
		win.peerCtx = app.activePeerCtx
		return newPeerInfo(win.peerCtx)

	case "log-viewer":
		lv := newLogViewer(app.eventLog, win.id)
		if app.state != nil {
			lv.model.BindState(app.state, win.id)
		}
		return lv

	case "entity-shell":
		if app.activePeerCtx == nil || app.shWs == nil {
			return nil
		}
		win.peerCtx = app.activePeerCtx
		publishWD := shellcmd.PublishWDForPanel(app.state, win.id, func() int { return app.ws.activeScreen })
		return newEntityShell(app.shWs, publishWD)

	case "execute-console":
		if app.activePeerCtx == nil {
			return nil
		}
		win.peerCtx = app.activePeerCtx
		return newExecuteConsole(app.ws, win.peerCtx, app.eventLog, app.dispatchExecute)

	case "query-browser":
		if app.activePeerCtx == nil {
			return nil
		}
		win.peerCtx = app.activePeerCtx
		return newQueryBrowser(app.ws, win.peerCtx, app.state, screenIdx)

	case "markdown-files":
		if app.activePeerCtx == nil {
			return nil
		}
		win.peerCtx = app.activePeerCtx
		return newMarkdownFiles(app.ws, win.peerCtx, app.state, screenIdx)

	case "markdown-view":
		if app.activePeerCtx == nil {
			return nil
		}
		win.peerCtx = app.activePeerCtx
		return newMarkdownView(app.ws, win.peerCtx, app.eventLog, app.state, screenIdx)

	case "tview-demo":
		return newTviewDemo(app.ws)

	case "empty":
		ec := newEmptyContent(app.ws)
		ec.win = win
		return ec
	}
	return nil
}

func (app *application) handleAction(action wb.Action) {
	s := app.ws.active()
	switch action.Kind {
	case wb.ActionSetContent:
		if s.focused != nil {
			app.eventLog.Appendf("action set-content → %s (window %d)", action.ContentName, s.focused.id)
			app.ws.setWindowContent(s.focused, action.ContentName)
			app.saveWindowState(s.focused)
		}
	case wb.ActionWindowEvent:
		if s.focused != nil {
			s.focused.content.handleEvent(action.Event, action.Value)
		}
	}
}

func (app *application) queueRefresh() {
	app.ws.tviewApp.QueueUpdateDraw(func() {
		// Panel models that subscribe to their own prefix maintain
		// their state independently of this refresh tick. Panels
		// without subscriptions (tree-browser, peer-info) re-query the
		// store on each refresh — the per-panel cost is what they pay,
		// not a shared full-tree enumeration.
		for _, w := range app.ws.allWindowsAllScreens() {
			w.content.refresh()
		}
		app.ws.updateStatus()
		app.syncActiveScreenToTree()
	})
}

// syncActiveScreenToTree persists the active-screen pointer when it
// changes. Selection itself is now auto-published by SelectionState
// directly, so this function is only responsible for the active-screen
// signal.
func (app *application) syncActiveScreenToTree() {
	if app.state == nil {
		return
	}
	screen := app.ws.activeScreen
	if screen != app.lastSyncedScreen {
		app.lastSyncedScreen = screen
		app.saveScreenState()
	}
}

func (app *application) statusInfo() string {
	if app.activePeerCtx == nil {
		return "[gray]no peer[-]"
	}
	nEntities := app.activePeerCtx.EntityCount()
	nPaths := app.activePeerCtx.PathCount()
	return fmt.Sprintf("[gray]entities: %d  paths: %d[-]", nEntities, nPaths)
}
