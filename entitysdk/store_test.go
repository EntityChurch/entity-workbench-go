package entitysdk

import (
	"testing"

	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/store"
)

func newTestStore() *Store {
	return NewStore(store.NewMemoryContentStore(), store.NewMemoryLocationIndex())
}

func TestStorePutGet(t *testing.T) {
	st := newTestStore()

	h, err := st.Put("workspace/settings/theme", "app/state/setting",
		map[string]interface{}{"key": "theme", "value": "dark"})
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if h.IsZero() {
		t.Fatal("put returned zero hash")
	}

	ent, ok := st.Get("workspace/settings/theme")
	if !ok {
		t.Fatal("get: not found after put")
	}
	if ent.Type != "app/state/setting" {
		t.Errorf("got type %q, want app/state/setting", ent.Type)
	}
	if ent.ContentHash != h {
		t.Errorf("entity hash %s != put hash %s", ent.ContentHash, h)
	}
}

func TestStoreGetMissing(t *testing.T) {
	st := newTestStore()
	if _, ok := st.Get("nope"); ok {
		t.Error("get on empty store returned ok")
	}
}

func TestStoreHas(t *testing.T) {
	st := newTestStore()
	if st.Has("absent") {
		t.Error("Has reported present for missing path")
	}
	if _, err := st.Put("present", "test/v", 1); err != nil {
		t.Fatal(err)
	}
	if !st.Has("present") {
		t.Error("Has reported missing for present path")
	}
}

func TestStoreRemove(t *testing.T) {
	st := newTestStore()

	if st.Remove("missing") {
		t.Error("Remove on missing path returned true")
	}

	if _, err := st.Put("doomed", "test/v", 1); err != nil {
		t.Fatal(err)
	}
	if !st.Remove("doomed") {
		t.Error("Remove on present path returned false")
	}
	if st.Has("doomed") {
		t.Error("path still present after Remove")
	}
}

func TestStoreList(t *testing.T) {
	st := newTestStore()
	paths := []string{"a/x", "a/y", "b/z"}
	for _, p := range paths {
		if _, err := st.Put(p, "test/v", 1); err != nil {
			t.Fatal(err)
		}
	}

	all := st.List("")
	if len(all) != 3 {
		t.Errorf("List(\"\") returned %d, want 3", len(all))
	}

	aOnly := st.List("a/")
	if len(aOnly) != 2 {
		t.Errorf("List(\"a/\") returned %d, want 2", len(aOnly))
	}
}

func TestStorePutCASCreateOnly(t *testing.T) {
	st := newTestStore()

	h, err := st.PutCAS("once", "test/v", 1, hash.Hash{})
	if err != nil {
		t.Fatalf("create-only CAS on empty path: %v", err)
	}
	if h.IsZero() {
		t.Fatal("got zero hash on successful create")
	}

	_, err = st.PutCAS("once", "test/v", 2, hash.Hash{})
	if err == nil {
		t.Fatal("create-only CAS should have failed on existing path")
	}
	if !IsConflict(err) {
		t.Errorf("want 409 conflict, got %v", err)
	}
}

func TestStorePutCASExpectedMatch(t *testing.T) {
	st := newTestStore()

	// Missing path with non-zero expected → conflict.
	nonZero := hash.Hash{Algorithm: hash.AlgorithmSHA256}
	nonZero.Digest[0] = 1
	if _, err := st.PutCAS("nope", "test/v", 1, nonZero); !IsConflict(err) {
		t.Errorf("expected 409 for CAS on missing path, got %v", err)
	}

	h1, err := st.Put("swap", "test/v", 1)
	if err != nil {
		t.Fatal(err)
	}

	h2, err := st.PutCAS("swap", "test/v", 2, h1)
	if err != nil {
		t.Fatalf("matching CAS failed: %v", err)
	}
	if h2 == h1 {
		t.Error("updated hash equals original — different data should produce different hash")
	}

	// Stale expected → conflict.
	if _, err := st.PutCAS("swap", "test/v", 3, h1); !IsConflict(err) {
		t.Errorf("expected 409 for stale expected, got %v", err)
	}
}

func TestStoreEntityCount(t *testing.T) {
	st := newTestStore()
	if n := st.EntityCount(); n != 0 {
		t.Errorf("empty store count %d, want 0", n)
	}
	for i := 0; i < 3; i++ {
		if _, err := st.Put("p", "test/v", i); err != nil {
			t.Fatal(err)
		}
	}
	if n := st.EntityCount(); n != 3 {
		t.Errorf("after 3 distinct puts count %d, want 3", n)
	}
}

func TestStorePathCount(t *testing.T) {
	st := newTestStore()
	if n := st.PathCount(); n != 0 {
		t.Errorf("empty store path count %d, want 0", n)
	}

	// Three distinct paths sharing the same entity content — PathCount
	// must count paths, not entities (otherwise it'd collapse to 1).
	for _, p := range []string{"a", "b", "c"} {
		if _, err := st.Put(p, "test/v", 42); err != nil {
			t.Fatal(err)
		}
	}
	if n := st.EntityCount(); n != 1 {
		t.Errorf("entity count %d, want 1 (single content)", n)
	}
	if n := st.PathCount(); n != 3 {
		t.Errorf("path count %d, want 3 (three paths)", n)
	}

	// Removing a path drops the path count but content is still
	// referenced (only one path removed of three).
	if !st.Remove("a") {
		t.Fatal("remove returned false")
	}
	if n := st.PathCount(); n != 2 {
		t.Errorf("path count after remove %d, want 2", n)
	}
}
