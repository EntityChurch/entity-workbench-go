package entitysdk

import (
	"testing"

	"go.entitychurch.org/entity-core-go/core/types"
)

// seedTypeDef writes a system/type entity at system/type/{name}.
func seedTypeDef(t *testing.T, pc *PeerContext, td types.TypeDefinition) {
	t.Helper()
	ent, err := td.ToEntity()
	if err != nil {
		t.Fatalf("ToEntity: %v", err)
	}
	h, err := pc.Store().content.Put(ent)
	if err != nil {
		t.Fatalf("content.Put: %v", err)
	}
	pc.Store().locationIndex.Set(td.TreePath(), h)
}

func TestDiscoverTypes_Empty(t *testing.T) {
	pc, _, _ := testPeerContext(t)
	_ = pc // cache removed

	got := DiscoverTypes(pc)
	if len(got) != 0 {
		t.Fatalf("got %d types, want 0", len(got))
	}
}

func TestDiscoverTypes_FindsAndSortsByName(t *testing.T) {
	pc, _, _ := testPeerContext(t)

	seedTypeDef(t, pc, types.TypeDefinition{
		Name: "example/zeta",
		Fields: map[string]types.FieldSpec{
			"title": {TypeRef: "primitive/string"},
		},
	})
	seedTypeDef(t, pc, types.TypeDefinition{
		Name:    "example/alpha",
		Extends: "example/base",
		Fields: map[string]types.FieldSpec{
			"count": {TypeRef: "primitive/uint"},
		},
	})
	_ = pc // cache removed

	got := DiscoverTypes(pc)
	if len(got) != 2 {
		t.Fatalf("got %d types, want 2", len(got))
	}
	if got[0].Name != "example/alpha" || got[1].Name != "example/zeta" {
		t.Errorf("sort order: got %q, %q; want example/alpha, example/zeta",
			got[0].Name, got[1].Name)
	}
	if got[0].Extends != "example/base" {
		t.Errorf("Extends = %q, want example/base", got[0].Extends)
	}
	if _, ok := got[0].Fields["count"]; !ok {
		t.Errorf("example/alpha missing field 'count'")
	}
}

func TestDiscoverTypes_IgnoresNonTypePaths(t *testing.T) {
	pc, s, li := testPeerContext(t)
	seedStore(t, s, li, "data/not-a-type", "example/thing", "hello")
	seedTypeDef(t, pc, types.TypeDefinition{Name: "example/one"})
	_ = pc // cache removed

	got := DiscoverTypes(pc)
	if len(got) != 1 || got[0].Name != "example/one" {
		t.Errorf("want [example/one], got %+v", got)
	}
}

func TestDiscoverTypes_WorksOnRealAppPeer(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	pc := ap.PeerContext()
	_ = pc // cache removed

	got := DiscoverTypes(pc)
	if len(got) == 0 {
		t.Fatal("expected bootstrap types to be discoverable via real AppPeer, got 0")
	}
	// Bootstrap writes types like system/type, system/handler, system/tree/*,
	// etc. We don't assert the exact set — just that the namespace-qualified
	// path matching works end-to-end.
	foundSystemType := false
	for _, ti := range got {
		if ti.Name == "system/type" {
			foundSystemType = true
			break
		}
	}
	if !foundSystemType {
		t.Errorf("expected to discover system/type definition; got %d types, none matched", len(got))
	}
}
