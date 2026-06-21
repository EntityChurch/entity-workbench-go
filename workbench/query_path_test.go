package workbench

import (
	"testing"

	"go.entitychurch.org/entity-core-go/core/peer"
	"go.entitychurch.org/entity-core-go/core/store"
	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/query"
)

// TestQueryPaths checks what paths the query handler returns through a real peer.
func TestQueryPaths(t *testing.T) {
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
	defer p.Close()

	ex := NewExecutor(p.Registry(), p.Store(), p.LocationIndex(), p.PeerID())
	st := NewStore(p.Store(), p.LocationIndex())

	if _, err := st.Put("test/hello", "test/type", "value"); err != nil {
		t.Fatal(err)
	}

	// Check what the local store lists at prefix "".
	entries := st.List("")
	t.Logf("store.List paths:")
	for _, e := range entries {
		t.Logf("  %s", e.Path)
	}

	// Check what query returns
	result, err := ex.Query(types.QueryExpressionData{TypeFilter: "test/type"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	t.Logf("Query match paths:")
	for _, m := range result.Matches {
		t.Logf("  %s (type=%s)", m.Path, m.Type)
	}

	// Check if query path resolves
	if len(result.Matches) > 0 {
		path := result.Matches[0].Path
		_, ok := st.Get(path)
		t.Logf("store.Get(%q) ok=%v", path, ok)
	}
}
