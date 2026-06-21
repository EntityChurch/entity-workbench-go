package workbench

import "testing"

func TestTreeBrowserModel_Refresh(t *testing.T) {
	pc, s, li := testPeerContext(t)
	seedStore(t, s, li, "test/alpha", "test/type", "a")
	seedStore(t, s, li, "test/beta", "test/type", "b")
	_ = pc // cache removed

	m := NewTreeBrowserModel(pc)
	m.Refresh()

	if m.Root == nil {
		t.Fatal("expected root")
	}
	if len(m.VisibleRows) == 0 {
		t.Fatal("expected visible rows")
	}
}

func TestTreeBrowserModel_SearchPath(t *testing.T) {
	pc, s, li := testPeerContext(t)
	seedStore(t, s, li, "test/alpha", "test/type", "a")
	seedStore(t, s, li, "test/beta", "test/type", "b")
	seedStore(t, s, li, "other/gamma", "other/type", "c")
	_ = pc // cache removed

	m := NewTreeBrowserModel(pc)
	m.Refresh()

	// Search for "alpha"
	m.SetSearch("alpha")
	if m.MatchCount != 1 {
		t.Errorf("expected 1 match, got %d", m.MatchCount)
	}
	if len(m.VisibleRows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(m.VisibleRows))
	}
	if m.VisibleRows[0].Node.FullPath != "test/alpha" {
		t.Errorf("expected test/alpha, got %s", m.VisibleRows[0].Node.FullPath)
	}

	// Search for "test" — matches 2
	m.SetSearch("test")
	if m.MatchCount != 2 {
		t.Errorf("expected 2 matches, got %d", m.MatchCount)
	}

	// Clear search
	m.ClearSearch()
	if m.SearchText != "" {
		t.Error("expected empty search")
	}
	// Should have all nodes visible (tree view, not flat)
	if len(m.VisibleRows) == 0 {
		t.Error("expected visible rows after clear")
	}
}

func TestTreeBrowserModel_SearchType(t *testing.T) {
	pc, s, li := testPeerContext(t)
	seedStore(t, s, li, "test/alpha", "doc/paper", "a")
	seedStore(t, s, li, "test/beta", "doc/paper", "b")
	seedStore(t, s, li, "other/gamma", "test/config", "c")
	_ = pc // cache removed

	m := NewTreeBrowserModel(pc)
	m.Refresh()

	// Type filter
	m.SetSearch("type:doc")
	if m.MatchCount != 2 {
		t.Errorf("expected 2 doc matches, got %d", m.MatchCount)
	}

	m.SetSearch("type:config")
	if m.MatchCount != 1 {
		t.Errorf("expected 1 config match, got %d", m.MatchCount)
	}

	m.SetSearch("type:nonexistent")
	if m.MatchCount != 0 {
		t.Errorf("expected 0 matches, got %d", m.MatchCount)
	}
}

func TestTreeBrowserModel_SearchCaseInsensitive(t *testing.T) {
	pc, s, li := testPeerContext(t)
	seedStore(t, s, li, "Test/Alpha", "test/type", "a")
	_ = pc // cache removed

	m := NewTreeBrowserModel(pc)
	m.Refresh()

	m.SetSearch("alpha")
	if m.MatchCount != 1 {
		t.Errorf("case-insensitive: expected 1, got %d", m.MatchCount)
	}

	m.SetSearch("ALPHA")
	if m.MatchCount != 1 {
		t.Errorf("uppercase: expected 1, got %d", m.MatchCount)
	}
}

func TestTreeBrowserModel_ToggleExpand(t *testing.T) {
	pc, s, li := testPeerContext(t)
	seedStore(t, s, li, "a/one", "t", "v")
	seedStore(t, s, li, "a/two", "t", "v")
	seedStore(t, s, li, "b/three", "t", "v")
	_ = pc // cache removed

	m := NewTreeBrowserModel(pc)
	m.Refresh()

	initial := len(m.VisibleRows)

	// Find the "a" node and collapse it
	for i, row := range m.VisibleRows {
		if row.Node.Segment == "a" && len(row.Node.Children) > 0 {
			m.ToggleExpand(i)
			break
		}
	}

	if len(m.VisibleRows) >= initial {
		t.Error("collapsing should reduce visible rows")
	}
}

func TestTreeBrowserModel_SyncFromSelection(t *testing.T) {
	pc, s, li := testPeerContext(t)
	seedStore(t, s, li, "deep/nested/path", "t", "v")
	_ = pc // cache removed

	m := NewTreeBrowserModel(pc)
	m.Refresh()

	state := NewWorkspaceState(pc.Store())
	state.SaveSelection(0, Selection{Path: "deep/nested/path", Type: "entity"})
	m.BindSelection(state, 0)

	idx := m.SyncFromSelection()
	if idx < 0 {
		t.Error("expected to find synced path in visible rows")
	}
	if idx >= 0 && m.VisibleRows[idx].Node.FullPath != "deep/nested/path" {
		t.Errorf("synced to wrong row: %s", m.VisibleRows[idx].Node.FullPath)
	}
}

func TestTreeBrowserModel_SyncClearsSearch(t *testing.T) {
	pc, s, li := testPeerContext(t)
	seedStore(t, s, li, "test/alpha", "t", "v")
	seedStore(t, s, li, "test/beta", "t", "v")
	_ = pc // cache removed

	m := NewTreeBrowserModel(pc)
	m.Refresh()
	m.SetSearch("alpha")

	if m.MatchCount != 1 {
		t.Fatalf("search should match 1, got %d", m.MatchCount)
	}

	// External selection change clears search
	state := NewWorkspaceState(pc.Store())
	state.SaveSelection(0, Selection{Path: "test/beta", Type: "entity"})
	m.BindSelection(state, 0)
	m.SyncFromSelection()

	if m.SearchText != "" {
		t.Error("sync should clear search")
	}
}

func TestTreeBrowserModel_PublishSelection(t *testing.T) {
	pc, s, li := testPeerContext(t)
	seedStore(t, s, li, "test/hello", "t", "v")
	_ = pc // cache removed

	m := NewTreeBrowserModel(pc)
	m.Refresh()

	state := NewWorkspaceState(pc.Store())
	m.BindSelection(state, 0)

	// Find the leaf node
	for i, row := range m.VisibleRows {
		if row.Node.HasEntry {
			m.PublishSelection(i)
			break
		}
	}

	if sel, ok := state.ReadSelection(0); !ok || sel.Path == "" {
		t.Error("expected selection to be published")
	}
}

func TestFilterEntries(t *testing.T) {
	pc, s, li := testPeerContext(t)
	seedStore(t, s, li, "test/alpha", "doc/paper", "a")
	seedStore(t, s, li, "test/beta", "test/config", "b")
	_ = pc // cache removed

	entries := pc.Store().List("")

	// Path filter
	result := FilterEntries(entries, "alpha", pc.Resolve)
	if len(result) != 1 {
		t.Errorf("path filter: expected 1, got %d", len(result))
	}

	// Type filter
	result = FilterEntries(entries, "type:doc", pc.Resolve)
	if len(result) != 1 {
		t.Errorf("type filter: expected 1, got %d", len(result))
	}

	// No match
	result = FilterEntries(entries, "nonexistent", pc.Resolve)
	if len(result) != 0 {
		t.Errorf("no match: expected 0, got %d", len(result))
	}
}
