package entitysdk

import (
	"strings"
	"testing"
	"time"
)

func TestScopeResolvesPaths(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	scope := ap.Scope("app/workbench/workspace")
	if scope.Prefix() != "app/workbench/workspace/" {
		t.Errorf("prefix not normalized: %q", scope.Prefix())
	}

	h, err := scope.Put("settings/theme", "app/state/setting",
		map[string]interface{}{"key": "theme", "value": "dark"})
	if err != nil {
		t.Fatalf("scope.Put: %v", err)
	}
	if h.IsZero() {
		t.Fatal("zero hash")
	}

	// The write must land at the scoped path, readable via AppPeer.Store.
	ent, ok := ap.Store().Get("app/workbench/workspace/settings/theme")
	if !ok {
		t.Fatal("scoped put did not land at prefix+relPath")
	}
	if ent.Type != "app/state/setting" {
		t.Errorf("type = %q, want app/state/setting", ent.Type)
	}

	// Read back through the scope round-trips.
	if !scope.Has("settings/theme") {
		t.Error("scope.Has missed the entry we just wrote")
	}
}

func TestScopeListIsScoped(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	scope := ap.Scope("app/workbench/workspace/")
	paths := []string{"a", "b/c", "b/d"}
	for _, p := range paths {
		if _, err := scope.Put(p, "test/v", 1); err != nil {
			t.Fatal(err)
		}
	}

	// List everything under the scope.
	all := scope.List("")
	if len(all) != 3 {
		t.Errorf("scope.List(\"\") returned %d, want 3", len(all))
	}

	// Sub-list inside scope.
	bOnly := scope.List("b/")
	if len(bOnly) != 2 {
		t.Errorf("scope.List(\"b/\") returned %d, want 2", len(bOnly))
	}
}

func TestScopeRejectsAbsolutePaths(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	scope := ap.Scope("app/workbench/")
	_, err = scope.Put("/other/peer/foo", "test/v", 1)
	if err == nil {
		t.Fatal("expected absolute-path rejection")
	}
	if !IsClientError(err) {
		t.Errorf("want 400 client error, got %v", err)
	}
}

func TestScopeWatchIsScoped(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	scope := ap.Scope("app/workbench/")
	w, err := scope.Watch("workspace/*")
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	// Write under scope → seen.
	if _, err := scope.Put("workspace/theme", "test/v", 1); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-w.Events():
		if !strings.Contains(ev.Path, "/app/workbench/workspace/theme") {
			t.Errorf("scoped watch saw unexpected path %q", ev.Path)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("scoped watch never received event")
	}

	// Write outside scope → not seen.
	if _, err := ap.Store().Put("other/x", "test/v", 1); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-w.Events():
		t.Errorf("scoped watch received out-of-scope event: %q", ev.Path)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestScopeCloseCancelsWatches(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	scope := ap.Scope("app/workbench/")
	w, err := scope.Watch("workspace/*")
	if err != nil {
		t.Fatal(err)
	}

	scope.Close()

	select {
	case _, ok := <-w.Events():
		if ok {
			t.Error("watch channel not closed after Scope.Close")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("watch channel did not close after Scope.Close")
	}
}

func TestScopeEmptyPrefix(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	scope := ap.Scope("")
	if scope.Prefix() != "" {
		t.Errorf("empty scope got prefix %q", scope.Prefix())
	}
	// Behaves like AppPeer.Store for path resolution.
	if _, err := scope.Put("bare/path", "test/v", 1); err != nil {
		t.Fatal(err)
	}
	if !ap.Store().Has("bare/path") {
		t.Error("empty-prefix scope didn't write to bare path")
	}
}
