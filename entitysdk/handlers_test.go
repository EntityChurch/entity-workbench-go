package entitysdk

import (
	"testing"

	"github.com/fxamacker/cbor/v2"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/store"
	"go.entitychurch.org/entity-core-go/core/types"
)

func seedHandlerInterface(t *testing.T, s store.ContentStore, li store.LocationIndex, pattern, name string, ops map[string]types.HandlerOperationSpec) {
	t.Helper()
	data := types.HandlerInterfaceData{
		Pattern:    pattern,
		Name:       name,
		Operations: ops,
	}
	raw, err := ecf.Encode(data)
	if err != nil {
		t.Fatal(err)
	}
	ent, err := entity.NewEntity(types.TypeHandlerInterface, cbor.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	h, err := s.Put(ent)
	if err != nil {
		t.Fatal(err)
	}
	li.Set("system/handler/"+pattern, h)
}

func TestDiscoverHandlers_Empty(t *testing.T) {
	pc, _, _ := testPeerContext(t)
	_ = pc // cache removed

	handlers := DiscoverHandlers(pc)
	if len(handlers) != 0 {
		t.Fatalf("got %d handlers, want 0", len(handlers))
	}
}

func TestDiscoverHandlers_FindsHandlers(t *testing.T) {
	pc, s, li := testPeerContext(t)

	seedHandlerInterface(t, s, li, "system/tree", "Tree Handler", map[string]types.HandlerOperationSpec{
		"get":  {InputType: "system/tree/get-request", OutputType: ""},
		"list": {InputType: "", OutputType: "system/tree/listing"},
		"put":  {InputType: "system/tree/put-request", OutputType: ""},
	})

	seedHandlerInterface(t, s, li, "data/files", "File Handler", map[string]types.HandlerOperationSpec{
		"read":  {OutputType: "data/file"},
		"write": {InputType: "data/file"},
	})

	_ = pc // cache removed
	handlers := DiscoverHandlers(pc)

	if len(handlers) != 2 {
		t.Fatalf("got %d handlers, want 2", len(handlers))
	}

	// Should be sorted by pattern
	if handlers[0].Pattern != "data/files" {
		t.Errorf("first handler = %q, want data/files", handlers[0].Pattern)
	}
	if handlers[1].Pattern != "system/tree" {
		t.Errorf("second handler = %q, want system/tree", handlers[1].Pattern)
	}

	// Operations should be sorted
	tree := handlers[1]
	if len(tree.Operations) != 3 {
		t.Fatalf("tree ops = %d, want 3", len(tree.Operations))
	}
	if tree.Operations[0] != "get" || tree.Operations[1] != "list" || tree.Operations[2] != "put" {
		t.Errorf("ops not sorted: %v", tree.Operations)
	}

	// Specs should be accessible
	if tree.Specs["get"].InputType != "system/tree/get-request" {
		t.Errorf("get input type = %q", tree.Specs["get"].InputType)
	}
}

func TestDiscoverHandlers_IgnoresNonHandlerPaths(t *testing.T) {
	pc, s, li := testPeerContext(t)

	// Add a non-handler entry
	seedStore(t, s, li, "data/some-entity", "test/type", "value")
	// Add a handler entry
	seedHandlerInterface(t, s, li, "system/tree", "Tree", map[string]types.HandlerOperationSpec{
		"list": {},
	})

	_ = pc // cache removed
	handlers := DiscoverHandlers(pc)

	if len(handlers) != 1 {
		t.Fatalf("got %d handlers, want 1", len(handlers))
	}
}
