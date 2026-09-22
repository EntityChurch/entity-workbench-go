package entitysdk_test

import (
	"testing"

	"entity-workbench-go/entitysdk"
)

// GUIDE-ENTITY-WORKBENCH-APP §8's persist arm: an application that persists
// per-window state MUST be able to determine at startup which window each
// persisted entity belongs to, satisfied either by an `app/state/window-index`
// or by sweeping `app/{app-id}/workspace/windows/` before allocating an id.
// entity-browser-rust read our source and reported that we satisfied neither
// (their W-3, 2026-09-01); they were right.
//
// These run on a REAL peer store, not on `NewStore(memory, memory)`, and that
// is the point. A bare memory store has no `NamespacedIndex`, so `Store.List`
// returns relative paths and any prefix arithmetic works; a peer's index
// returns PEER-QUALIFIED paths and a `TrimPrefix` with the relative prefix we
// passed in removes nothing (AP58). The production object always has the
// wrapper, so a fixture without it cannot fail on anything the wrapper changes.

func TestHighestPersistedWindowID_ReadsQualifiedPathsFromARealPeer(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	defer ap.Close()

	ws := entitysdk.NewWorkspaceState(ap.Store())

	if got := ws.HighestPersistedWindowID(); got != 0 {
		t.Fatalf("empty tree: got %d, want 0", got)
	}

	// Not in ascending order, and not contiguous — the answer is the max, not
	// the count and not the last one written.
	ws.SaveWindowContent(2, "tree-browser")
	ws.SaveWindowContent(7, "log-viewer")
	ws.SaveWindowContent(3, "shell")

	if got := ws.HighestPersistedWindowID(); got != 7 {
		t.Fatalf("got %d, want 7", got)
	}

	// Anti-vacuity for the parse itself: the paths really are qualified, so a
	// prefix-trim implementation would have measured nothing above.
	entries := ap.Store().List("app/workbench/workspace/windows/")
	if len(entries) == 0 {
		t.Fatal("no entries under the windows prefix — the sweep above proved nothing")
	}
	qualified := false
	for _, e := range entries {
		if len(e.Path) > 0 && e.Path[0] == '/' {
			qualified = true
			break
		}
	}
	if !qualified {
		t.Errorf("expected peer-qualified paths from a real peer's index, got %q — "+
			"if this store stopped qualifying, this test no longer covers AP58's shape",
			entries[0].Path)
	}
}

// The failure the obligation exists to prevent, stated as an assertion: a
// second session must not allocate an id that already carries state. This is
// the collision, not the sweep — it fails against a zero-seeded counter.
func TestASecondSessionDoesNotAllocateAWindowIDThatAlreadyHasState(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	defer ap.Close()

	// Session one: three windows, each with state a later session could
	// mis-read. `log-display-level` is the real one — workbench.LogModel binds
	// it by ordinal.
	first := entitysdk.NewWorkspaceState(ap.Store())
	for id := uint32(1); id <= 3; id++ {
		first.SaveWindowContent(id, "log-viewer")
		first.SaveWindowSetting(id, "log-display-level", "verbose")
	}

	// Session two, same tree, fresh counter — the shape console.workspace has.
	second := entitysdk.NewWorkspaceState(ap.Store())
	nextID := second.HighestPersistedWindowID()

	nextID++
	if got := second.ReadWindowContent(nextID); got != "" {
		t.Errorf("window %d already carries content %q — the next session would restore a stranger's state", nextID, got)
	}
	if got := second.ReadWindowSetting(nextID, "log-display-level"); got != "" {
		t.Errorf("window %d already carries a display level %q — this is the §8 failure", nextID, got)
	}

	// Control arm: without the sweep the very same read hits session one's
	// window. Without this the test above passes on any tree that happens to
	// be empty at the ordinal it probes.
	if got := second.ReadWindowSetting(1, "log-display-level"); got != "verbose" {
		t.Fatalf("control arm: window 1 should still hold session one's state, got %q", got)
	}
}
