package entitysdk

import (
	"testing"

	"github.com/fxamacker/cbor/v2"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/handler"
	"go.entitychurch.org/entity-core-go/core/store"
)

// testPeerContext creates a PeerContext backed by in-memory store/index
// with a minimal handler registry. Returns the PeerContext and the raw
// store/index for seeding test data.
func testPeerContext(t *testing.T) (*PeerContext, store.ContentStore, store.LocationIndex) {
	t.Helper()
	s := store.NewMemoryContentStore()
	li := store.NewMemoryLocationIndex()
	reg := handler.NewRegistry()
	ex := NewExecutor(reg, s, li, "test-peer")
	st := NewStore(s, li)
	pc := NewPeerContext(ex, st)
	return pc, s, li
}

func seedStore(t *testing.T, s store.ContentStore, li store.LocationIndex, path, typ string, val interface{}) {
	t.Helper()
	raw, err := ecf.Encode(val)
	if err != nil {
		t.Fatal(err)
	}
	ent, err := entity.NewEntity(typ, cbor.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	h, err := s.Put(ent)
	if err != nil {
		t.Fatal(err)
	}
	li.Set(path, h)
}

func TestResolveEntity_Found(t *testing.T) {
	pc, s, li := testPeerContext(t)
	seedStore(t, s, li, "test/hello", "test/type", map[string]string{"greeting": "world"})

	r, ok := ResolveEntity(pc, "test/hello")
	if !ok {
		t.Fatal("expected to find entity")
	}
	if r.Path != "test/hello" {
		t.Errorf("path = %q, want test/hello", r.Path)
	}
	if r.Entity.Type != "test/type" {
		t.Errorf("type = %q, want test/type", r.Entity.Type)
	}
	if r.Decoded == nil {
		t.Error("expected decoded data")
	}
	if r.Hash.IsZero() {
		t.Error("expected non-zero hash")
	}
}

func TestResolveEntity_NotFound(t *testing.T) {
	pc, _, _ := testPeerContext(t)

	_, ok := ResolveEntity(pc, "nonexistent")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestResolveEntity_HashMissing(t *testing.T) {
	pc, s, li := testPeerContext(t)
	seedStore(t, s, li, "test/ephemeral", "test/type", "data")

	// Remove entity from store but keep the index entry
	h, _ := li.Get("test/ephemeral")
	s.Remove(h)

	_, ok := ResolveEntity(pc, "test/ephemeral")
	if ok {
		t.Fatal("expected not found when hash missing from store")
	}
}

func TestPeerContext_Resolve(t *testing.T) {
	pc, s, li := testPeerContext(t)
	seedStore(t, s, li, "test/direct", "test/type", "value")

	r, ok := pc.Resolve("test/direct")
	if !ok {
		t.Fatal("expected to resolve")
	}
	if r.Entity.Type != "test/type" {
		t.Errorf("type = %q", r.Entity.Type)
	}
}

func TestPeerContext_EntityCount(t *testing.T) {
	pc, s, li := testPeerContext(t)
	seedStore(t, s, li, "test/a", "test/type", "val1")
	seedStore(t, s, li, "test/b", "test/type", "val2")

	if pc.EntityCount() != 2 {
		t.Errorf("count = %d, want 2", pc.EntityCount())
	}
}

func TestListByPrefix(t *testing.T) {
	entries := []store.LocationEntry{
		{Path: "alpha/one"},
		{Path: "alpha/two"},
		{Path: "beta/three"},
	}

	result := ListByPrefix(entries, "alpha/")
	if len(result) != 2 {
		t.Fatalf("got %d entries, want 2", len(result))
	}

	result = ListByPrefix(entries, "beta/")
	if len(result) != 1 {
		t.Fatalf("got %d entries, want 1", len(result))
	}

	result = ListByPrefix(entries, "")
	if len(result) != 3 {
		t.Fatalf("got %d entries, want 3", len(result))
	}

	result = ListByPrefix(entries, "gamma/")
	if len(result) != 0 {
		t.Fatalf("got %d entries, want 0", len(result))
	}
}
