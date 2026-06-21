package workbench

import "testing"

// bindSelectionWatch exists only to make selection-slot subscription
// degrade gracefully where WorkspaceState.OnSelectionChange would panic
// (nil state, or a store with no watch hub — test scaffolding / a peer
// with no presentation context). These pin that degradation: it must
// return a nil cancel and never panic, so callers fall back to
// seed-only with no live updates.

func TestBindSelectionWatch_NilStateReturnsNilCancel(t *testing.T) {
	called := false
	cancel := bindSelectionWatch(nil, 0, func(Selection) { called = true })
	if cancel != nil {
		t.Fatal("nil state must yield a nil cancel")
	}
	if called {
		t.Fatal("handler must not be invoked when state is nil")
	}
}

func TestBindSelectionWatch_HublessStoreDegradesToSeedOnly(t *testing.T) {
	pc, _, _ := testPeerContext(t)
	// testPeerContext's Store has no watch hub; OnSelectionChange
	// panics there by design. bindSelectionWatch must recover and
	// degrade rather than crash the caller.
	state := NewWorkspaceState(pc.Store())

	called := false
	cancel := bindSelectionWatch(state, 0, func(Selection) { called = true })
	if cancel != nil {
		t.Fatal("hub-less store must yield a nil cancel (seed-only degrade)")
	}
	if called {
		t.Fatal("handler must not fire without a watch hub")
	}
	// Reaching here without a panic propagating is the core contract.
}
