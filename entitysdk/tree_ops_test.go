package entitysdk

import (
	"testing"

	"go.entitychurch.org/entity-core-go/core/hash"
)

func TestAppPeerPutThenGet(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	h, err := ap.Put("workspace/settings/theme", "app/state/setting",
		map[string]interface{}{"key": "theme", "value": "dark"})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if h.IsZero() {
		t.Fatal("Put returned zero hash")
	}

	ent, ok, err := ap.Get("workspace/settings/theme")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok {
		t.Fatal("Get: binding not found after Put")
	}
	if ent.Type != "app/state/setting" {
		t.Errorf("type = %q, want app/state/setting", ent.Type)
	}
}

func TestAppPeerGetMissing(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	_, ok, err := ap.Get("nowhere")
	if err != nil {
		t.Errorf("Get on missing path: want nil err, got %v", err)
	}
	if ok {
		t.Error("Get on missing path: want ok=false")
	}
}

func TestAppPeerPutCASCreatesWithZeroExpected(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	// With zero expected, dispatched put creates unconditionally
	// (core tree handler treats absent expected_hash as "no CAS").
	h, err := ap.PutCAS("first", "test/v", 1, hash.Hash{})
	if err != nil {
		t.Fatalf("PutCAS: %v", err)
	}
	if h.IsZero() {
		t.Fatal("PutCAS returned zero hash")
	}
}

func TestAppPeerPutCASMatchAndMismatch(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	h1, err := ap.Put("swap", "test/v", 1)
	if err != nil {
		t.Fatal(err)
	}

	h2, err := ap.PutCAS("swap", "test/v", 2, h1)
	if err != nil {
		t.Fatalf("matching PutCAS: %v", err)
	}
	if h2 == h1 {
		t.Error("updated hash equals original — different data should hash differently")
	}

	// Stale expected → handler returns 409 conflict.
	_, err = ap.PutCAS("swap", "test/v", 3, h1)
	if err == nil {
		t.Fatal("expected conflict on stale expected_hash")
	}
	if !IsConflict(err) {
		t.Errorf("want 409 conflict, got %v", err)
	}
}

// TestAppPeerPutGoesThroughHandler confirms the dispatched Put path
// actually invokes the system/tree handler by comparing against
// Store.Put — the two should produce the same persisted state, but
// the dispatched path goes through the handler registry. (If the
// handler weren't wired, dispatched Put would fail with 404 "no
// handler matched".)
func TestAppPeerPutGoesThroughHandler(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	// Dispatched write.
	if _, err := ap.Put("a/b", "test/v", 42); err != nil {
		t.Fatalf("dispatched Put: %v", err)
	}

	// Direct-store read of the same path must see the result —
	// confirms the handler wrote through the location index and
	// store, not some other side channel.
	if !ap.Store().Has("a/b") {
		t.Error("dispatched Put didn't reach the location index")
	}
}

func TestAppPeerHas(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	present, err := ap.Has("nowhere")
	if err != nil {
		t.Fatalf("Has(missing): %v", err)
	}
	if present {
		t.Error("Has(missing) = true, want false")
	}

	if _, err := ap.Put("here/it/is", "test/v", "x"); err != nil {
		t.Fatal(err)
	}
	present, err = ap.Has("here/it/is")
	if err != nil {
		t.Fatalf("Has(present): %v", err)
	}
	if !present {
		t.Error("Has(present) = false, want true")
	}
}

func TestAppPeerRemove(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	if _, err := ap.Put("gone/soon", "test/v", 1); err != nil {
		t.Fatal(err)
	}
	if !ap.Store().Has("gone/soon") {
		t.Fatal("setup: Put didn't land")
	}

	if err := ap.Remove("gone/soon"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if ap.Store().Has("gone/soon") {
		t.Error("Remove didn't unbind path")
	}

	// Second remove → 404.
	err = ap.Remove("gone/soon")
	if !IsNotFound(err) {
		t.Errorf("Remove of missing path: want 404, got %v", err)
	}
}

func TestAppPeerList(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	// Direct children of kb/.
	if _, err := ap.Put("kb/intro", "test/v", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := ap.Put("kb/index", "test/v", 2); err != nil {
		t.Fatal(err)
	}
	// Nested — should surface kb/nested as has_children=true, not a row per leaf.
	if _, err := ap.Put("kb/nested/a", "test/v", 3); err != nil {
		t.Fatal(err)
	}
	if _, err := ap.Put("kb/nested/b", "test/v", 4); err != nil {
		t.Fatal(err)
	}

	rows, err := ap.List("kb")
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("want 3 rows (index, intro, nested), got %d: %+v", len(rows), rows)
	}
	// Sorted by name.
	wantNames := []string{"index", "intro", "nested"}
	for i, w := range wantNames {
		if rows[i].Name != w {
			t.Errorf("row[%d].Name = %q, want %q", i, rows[i].Name, w)
		}
	}
	// Full path reconstruction.
	if rows[0].Path != "kb/index" {
		t.Errorf("row[0].Path = %q, want kb/index", rows[0].Path)
	}
	// Direct-child rows carry a content hash; parent-of-nested should not.
	if rows[0].ContentHash.IsZero() {
		t.Error("leaf row has zero content hash")
	}
	if rows[0].HasChildren {
		t.Error("leaf row has_children = true")
	}
	if !rows[2].HasChildren {
		t.Error("nested/ parent has_children = false")
	}
}

func TestAppPeerListTrailingSlashAccepted(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	if _, err := ap.Put("d/only", "test/v", 1); err != nil {
		t.Fatal(err)
	}

	a, err := ap.List("d")
	if err != nil {
		t.Fatal(err)
	}
	b, err := ap.List("d/")
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != len(b) || a[0].Path != b[0].Path {
		t.Errorf("List with and without trailing slash diverged: %+v vs %+v", a, b)
	}
}

func TestAppPeerSnapshotDiff(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	if _, err := ap.Put("snap/a", "test/v", 1); err != nil {
		t.Fatal(err)
	}
	base, err := ap.Snapshot("snap/")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if base.IsZero() {
		t.Fatal("Snapshot returned zero root")
	}

	if _, err := ap.Put("snap/b", "test/v", 2); err != nil {
		t.Fatal(err)
	}
	target, err := ap.Snapshot("snap/")
	if err != nil {
		t.Fatalf("second Snapshot: %v", err)
	}
	if target == base {
		t.Fatal("snapshots equal after adding a binding")
	}

	diff, err := ap.Diff(base, target)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(diff.Added) != 1 {
		t.Errorf("Diff.Added = %d, want 1 (snap/b): %+v", len(diff.Added), diff.Added)
	}
	if _, ok := diff.Added["b"]; !ok {
		t.Errorf("Diff.Added missing key 'b': %+v", diff.Added)
	}
}
