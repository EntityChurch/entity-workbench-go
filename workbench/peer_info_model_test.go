package workbench

import (
	"reflect"
	"testing"
)

func TestPeerInfoModel_NilPeerCtxAndCloseIdempotent(t *testing.T) {
	m := NewPeerInfoModel(nil)
	if out := m.Render(); !reflect.DeepEqual(out, PeerInfoOutput{}) {
		t.Fatalf("nil peerCtx Render = %+v, want zero value", out)
	}
	// Close with no subscription must be safe and idempotent.
	m.Close()
	m.Close()
}

func TestPeerInfoModel_SeedFromExistingPaths(t *testing.T) {
	pc, _, _ := testPeerContext(t)
	// Distinct content per path so each is a distinct entity.
	for _, p := range []string{"docs/c.md", "docs/a.md", "docs/b.md"} {
		if _, err := pc.Store().Put(p, MarkdownFileType,
			map[string]interface{}{"content": p}); err != nil {
			t.Fatal(err)
		}
	}
	// Scaffold stores have no watch hub: OnPrefixChange("") seeds the
	// model synchronously from pre-existing paths at construction.
	m := NewPeerInfoModel(pc)
	defer m.Close()

	out := m.Render()
	if out.PathCount != 3 || len(out.Paths) != 3 {
		t.Fatalf("seed: PathCount=%d Paths=%v, want 3", out.PathCount, out.Paths)
	}
	for i := 1; i < len(out.Paths); i++ {
		if out.Paths[i-1] > out.Paths[i] {
			t.Fatalf("Paths not sorted: %v", out.Paths)
		}
	}
	if out.EntityCount != 3 {
		t.Fatalf("EntityCount=%d, want 3 (one distinct entity per path)", out.EntityCount)
	}
}

func TestPeerInfoModel_IncrementalEvents(t *testing.T) {
	pc, _, _ := testPeerContext(t)
	m := NewPeerInfoModel(pc) // no seed paths
	defer m.Close()

	if out := m.Render(); out.PathCount != 0 {
		t.Fatalf("empty model PathCount=%d, want 0", out.PathCount)
	}

	// Put adds; duplicate Put is deduped (count stays 2).
	m.onEvent(ChangeEvent{EventType: ChangePut, Path: "z/one"})
	m.onEvent(ChangeEvent{EventType: ChangePut, Path: "a/two"})
	m.onEvent(ChangeEvent{EventType: ChangePut, Path: "z/one"})
	out := m.Render()
	if out.PathCount != 2 {
		t.Fatalf("after 2 distinct + 1 dup Put: PathCount=%d, want 2", out.PathCount)
	}
	if !reflect.DeepEqual(out.Paths, []string{"a/two", "z/one"}) {
		t.Fatalf("Paths=%v, want sorted [a/two z/one]", out.Paths)
	}

	// Remove drops a member; removing a non-member is a no-op.
	m.onEvent(ChangeEvent{EventType: ChangeRemove, Path: "z/one"})
	m.onEvent(ChangeEvent{EventType: ChangeRemove, Path: "never/existed"})
	out = m.Render()
	if !reflect.DeepEqual(out.Paths, []string{"a/two"}) {
		t.Fatalf("after remove: Paths=%v, want [a/two]", out.Paths)
	}
}

func TestPeerInfoModel_RenderReturnsCopy(t *testing.T) {
	pc, _, _ := testPeerContext(t)
	m := NewPeerInfoModel(pc)
	defer m.Close()
	m.onEvent(ChangeEvent{EventType: ChangePut, Path: "p/x"})

	out := m.Render()
	if len(out.Paths) != 1 {
		t.Fatalf("precondition: Paths=%v", out.Paths)
	}
	out.Paths[0] = "MUTATED" // caller mutation must not corrupt the cache

	if again := m.Render(); again.Paths[0] != "p/x" {
		t.Fatalf("Render leaked its cache: got %q after caller mutation", again.Paths[0])
	}
}
