package workbench

import (
	"testing"

	"go.entitychurch.org/entity-core-go/core/peer"
	"go.entitychurch.org/entity-core-go/core/store"
	"go.entitychurch.org/entity-core-go/ext/query"
)

func testQueryModel(t *testing.T) (*QueryModel, store.ContentStore, store.LocationIndex) {
	t.Helper()

	// Use a real peer so namespaced paths, query handler, and sync hooks
	// all work the same as in the actual application.
	cs := store.NewMemoryContentStore()
	qm := query.NewIndexMaintainer(cs)
	qh := query.NewHandler(qm.TypeIndex(), qm.ReverseHashIndex(), qm.PathLinkIndex(), cs)

	p, err := peer.New(
		peer.WithStore(cs),
		peer.WithNamedSyncHook("query", qm.OnTreeChange),
		peer.WithHandler("system/query", qh),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close() })

	ex := NewExecutor(p.Registry(), p.Store(), p.LocationIndex(), p.PeerID())
	st := NewStore(p.Store(), p.LocationIndex())
	pc := NewPeerContext(ex, st)

	// Seed test data via the peer's Level 0 store — writes route
	// through the notifying index so the query extension's sync
	// hook fires and sees the new entities.
	if _, err := st.Put("test/alpha", "doc/paper", map[string]string{"title": "Alpha"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Put("test/beta", "doc/paper", map[string]string{"title": "Beta"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Put("other/gamma", "test/config", map[string]string{"key": "val"}); err != nil {
		t.Fatal(err)
	}
	_ = pc // cache removed

	m := NewQueryModel(pc)
	return m, cs, p.LocationIndex()
}

func TestQueryModel_EmptyQuery_ReturnsAll(t *testing.T) {
	m, _, _ := testQueryModel(t)

	m.Execute()
	if m.LastError != "" {
		t.Fatalf("query error: %s", m.LastError)
	}
	// Real peer has system entities (types, handlers, grants) plus our 3 test entities
	if len(m.Matches) < 3 {
		t.Fatalf("got %d matches, want at least 3", len(m.Matches))
	}
}

func TestQueryModel_TypeFilter(t *testing.T) {
	m, _, _ := testQueryModel(t)

	m.TypeFilter = "doc/paper"
	m.Execute()

	if m.LastError != "" {
		t.Fatalf("query error: %s", m.LastError)
	}
	if len(m.Matches) != 2 {
		t.Fatalf("got %d matches, want 2", len(m.Matches))
	}
	// Results should be the two doc/paper entities
	for _, match := range m.Matches {
		if match.Type != "doc/paper" {
			t.Errorf("match type = %q, want doc/paper", match.Type)
		}
	}
}

func TestQueryModel_TypeAndPathPrefix(t *testing.T) {
	m, _, _ := testQueryModel(t)

	m.TypeFilter = "*"
	m.PathPrefix = "test/"
	m.Execute()

	if m.LastError != "" {
		t.Fatalf("query error: %s", m.LastError)
	}
	if len(m.Matches) != 2 {
		t.Fatalf("got %d matches, want 2 (test/alpha and test/beta)", len(m.Matches))
	}
}

func TestQueryModel_PathPrefixOnly(t *testing.T) {
	m, _, _ := testQueryModel(t)

	m.PathPrefix = "test/"
	m.Execute()

	if m.LastError != "" {
		t.Fatalf("query error: %s", m.LastError)
	}
	if len(m.Matches) != 2 {
		t.Fatalf("got %d matches, want 2", len(m.Matches))
	}
}

func TestQueryModel_NoMatches(t *testing.T) {
	m, _, _ := testQueryModel(t)

	m.TypeFilter = "nonexistent/type"
	m.Execute()

	if m.LastError != "" {
		t.Fatalf("query error: %s", m.LastError)
	}
	if len(m.Matches) != 0 {
		t.Fatalf("got %d matches, want 0", len(m.Matches))
	}
	if m.StatusLine() != "no matches" {
		t.Errorf("status = %q", m.StatusLine())
	}
}

func TestQueryModel_Selection(t *testing.T) {
	m, _, _ := testQueryModel(t)

	m.TypeFilter = "doc/paper"
	m.Execute()

	if m.Selected != 0 {
		t.Errorf("initial selection = %d, want 0", m.Selected)
	}

	m.SelectNext()
	if m.Selected != 1 {
		t.Errorf("after next: selection = %d, want 1", m.Selected)
	}

	m.SelectNext() // at end, should not go past
	if m.Selected != 1 {
		t.Errorf("past end: selection = %d, want 1", m.Selected)
	}

	m.SelectPrev()
	if m.Selected != 0 {
		t.Errorf("after prev: selection = %d, want 0", m.Selected)
	}

	match := m.SelectedMatch()
	if match == nil {
		t.Fatal("expected non-nil match")
	}
	if match.Type != "doc/paper" {
		t.Errorf("selected match type = %q", match.Type)
	}
}

func TestQueryModel_PublishSelection(t *testing.T) {
	m, _, _ := testQueryModel(t)

	m.TypeFilter = "doc/paper"
	m.Execute()

	state := NewWorkspaceState(m.PeerCtx().Store())
	m.BindSelection(state, 0)
	m.PublishSelection()

	if sel, ok := state.ReadSelection(0); !ok || sel.Path == "" {
		t.Error("expected selection to be published")
	}
}

func TestQueryModel_StatusLine(t *testing.T) {
	m, _, _ := testQueryModel(t)

	// Before execute
	if s := m.StatusLine(); s != "Enter to query (type/path filters optional)" {
		t.Errorf("initial status = %q", s)
	}

	// After execute
	m.TypeFilter = "doc/paper"
	m.Execute()
	if s := m.StatusLine(); s != "2 matches" {
		t.Errorf("after execute status = %q", s)
	}
}
