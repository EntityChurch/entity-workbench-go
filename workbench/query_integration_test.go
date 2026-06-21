package workbench

import (
	"testing"

	"go.entitychurch.org/entity-core-go/core/peer"
	"go.entitychurch.org/entity-core-go/core/store"
	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/query"
)

// TestQueryIntegration_RealPeer tests query through a real peer with
// the query handler registered — same setup as console/main.go.
func TestQueryIntegration_RealPeer(t *testing.T) {
	cs := store.NewMemoryContentStore()
	qm := query.NewIndexMaintainer(cs)
	qh := query.NewHandler(qm.TypeIndex(), qm.ReverseHashIndex(), qm.PathLinkIndex(), cs)

	p, err := peer.New(
		peer.WithStore(cs),
		peer.WithNamedSyncHook("query", qm.OnTreeChange),
		peer.WithHandler("system/query", qh),
	)
	if err != nil {
		t.Fatalf("peer: %v", err)
	}
	defer p.Close()

	ex := NewExecutor(p.Registry(), p.Store(), p.LocationIndex(), p.PeerID())
	st := NewStore(p.Store(), p.LocationIndex())
	pc := NewPeerContext(ex, st)

	// Seed via Level 0 store — writes fire emits so the query
	// extension sees them.
	if _, err := st.Put("test/alpha", "doc/paper", map[string]string{"title": "Alpha"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := st.Put("test/beta", "doc/paper", map[string]string{"title": "Beta"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := st.Put("other/gamma", "test/config", map[string]string{"key": "val"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Query by type
	result, err := ex.Query(types.QueryExpressionData{TypeFilter: "doc/paper"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(result.Matches) != 2 {
		t.Fatalf("got %d matches, want 2", len(result.Matches))
	}

	// Query by type + path prefix
	result, err = ex.Query(types.QueryExpressionData{TypeFilter: "*", PathPrefix: "test/"})
	if err != nil {
		t.Fatalf("query path prefix: %v", err)
	}
	if len(result.Matches) != 2 {
		t.Fatalf("got %d matches for test/ prefix, want 2", len(result.Matches))
	}

	// Query count
	count, err := ex.QueryCount(types.QueryExpressionData{TypeFilter: "doc/paper"})
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}

	// QueryModel on real peer
	_ = pc // cache removed
	m := NewQueryModel(pc)
	m.TypeFilter = "doc/paper"
	m.Execute()
	if m.LastError != "" {
		t.Fatalf("model query error: %s", m.LastError)
	}
	if len(m.Matches) != 2 {
		t.Fatalf("model got %d matches, want 2", len(m.Matches))
	}
}
