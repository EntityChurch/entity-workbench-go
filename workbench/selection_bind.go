package workbench

// bindSelectionWatch subscribes a handler to a screen's selection slot
// and returns its cancel func (nil when no subscription was made).
//
// It mirrors the graceful degradation Store.OnPrefixChange already
// provides for watch-hub-less stores (test scaffolding / a peer with
// no presentation context): WorkspaceState.OnSelectionChange panics in
// that case by design, so here we degrade to seed-only (no live
// updates) rather than crash. Production peers (constructed via
// CreatePeer) always have a watch hub, so this is exercised only by
// scaffold/no-presentation paths where seed-only is the correct
// behaviour anyway.
func bindSelectionWatch(state *WorkspaceState, screenIdx int, handler func(Selection)) (cancel func()) {
	if state == nil {
		return nil
	}
	defer func() { _ = recover() }()
	return state.OnSelectionChange(state.ScreenSelectionPath(screenIdx), handler)
}
