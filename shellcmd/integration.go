package shellcmd

import "entity-workbench-go/entitysdk"

// Integration helpers — workspace-state wiring that's identical
// across render contexts (canvas, console, standalone REPL). Lives in
// shellcmd because it bridges shellcmd primitives (ShellWorkspace,
// PeerConn, Path) to entitysdk's WorkspaceState slots. Without these
// helpers each renderer would re-implement the same closures; with
// them every render context wires "shell behaves as a workbench
// citizen" in one call.
//
// Render contexts integrate by:
//
//	shWs.PersistAliases(state)                                                    // connect/disconnect → tree
//	sh.OnWDChanged = shellcmd.PublishWDForPanel(state, panelID, activeScreenFn)  // cd → navigate publish

// PersistAliases wires this workspace's connection hooks to persist
// every connect/disconnect under the given WorkspaceState. Replaces
// any existing OnConnAdded/OnConnRemoved. Pass a nil state to detach.
//
// The persisted record is `{alias, peer_id, address}` at
// app/workbench/workspace/shells/aliases/{alias}. Today the slot is
// write-only (no startup restoration); future features ("restore last
// session", "list known peers") read from it.
func (ws *ShellWorkspace) PersistAliases(state *entitysdk.WorkspaceState) {
	if state == nil {
		ws.OnConnAdded = nil
		ws.OnConnRemoved = nil
		return
	}
	ws.OnConnAdded = func(pc *PeerConn) {
		state.SaveAlias(pc.Alias, pc.PeerID, pc.Address)
	}
	ws.OnConnRemoved = func(alias string) {
		state.RemoveAlias(alias)
	}
}

// PublishWDForPanel returns an OnWDChanged callback that publishes the
// new working directory into both:
//
//   - the shell panel's own per-panel selection slot
//     (app/{app-id}/workspace/panels/{panelID}/selection), and
//   - the active screen's aggregate selection slot
//     (app/{app-id}/workspace/screens/{screen}/selection).
//
// Per the per-panel-slot + per-context-aggregate model in
// SHELL-DIRECTION.md §8.4: publisher panels write both by default;
// subscribers default to watching the aggregate. activeScreen is
// called per publish so multi-screen renderers always target their
// current screen.
//
// Returns nil when state is nil — callers can assign unconditionally.
//
// Wire at shell-panel construction:
//
//	publishWD := PublishWDForPanel(state, win.id, func() int { return ws.activeScreen })
//	sh := NewShellInWorkspace(shWs)
//	sh.OnWDChanged = publishWD
//
// The standalone REPL has no presentation context to coordinate with
// and should leave OnWDChanged nil.
func PublishWDForPanel(state *entitysdk.WorkspaceState, panelID uint32, activeScreen func() int) func(prev, next Path) {
	if state == nil {
		return nil
	}
	return func(_, next Path) {
		sel := entitysdk.Selection{
			Path:   string(next),
			Type:   "entity",
			PeerID: next.PeerID(),
		}
		state.SavePanelSelection(panelID, sel)
		state.SaveSelection(activeScreen(), sel)
	}
}
