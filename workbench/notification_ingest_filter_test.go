package workbench

import "testing"

// TestPassesMountFilter_NoFilters — empty filters pass everything.
func TestPassesMountFilter_NoFilters(t *testing.T) {
	if !passesMountFilter("notes/x.md", nil, nil) {
		t.Error("empty filter should pass")
	}
}

// TestPassesMountFilter_IncludeAllowList — non-empty include keeps
// only matching basenames; non-matching rejected.
func TestPassesMountFilter_IncludeAllowList(t *testing.T) {
	include := []string{"*.md", "*.markdown"}
	if !passesMountFilter("a.md", include, nil) {
		t.Error("a.md should match *.md")
	}
	if !passesMountFilter("deep/folder/b.markdown", include, nil) {
		t.Error("b.markdown should match *.markdown (basename only)")
	}
	if passesMountFilter("x.txt", include, nil) {
		t.Error("x.txt should be excluded by include allowlist")
	}
}

// TestPassesMountFilter_ExcludeWins — exclude always rejects, even
// when include would have matched.
func TestPassesMountFilter_ExcludeWins(t *testing.T) {
	include := []string{"*.md"}
	exclude := []string{"draft.md", "*~"}
	if !passesMountFilter("note.md", include, exclude) {
		t.Error("note.md should pass (matches include, no exclude)")
	}
	if passesMountFilter("draft.md", include, exclude) {
		t.Error("draft.md should be excluded")
	}
	if passesMountFilter("backup.md~", include, exclude) {
		t.Error("backup.md~ should be excluded by *~")
	}
}

// TestSetMountFilters_RoundTrips — exercises the public API of
// NotificationIngestHandler for filter mutation + readback.
func TestSetMountFilters_RoundTrips(t *testing.T) {
	h := NewNotificationIngestHandler(nil)
	h.RegisterMount("local/files/r/", "archives/notes/")

	// Defaults: no filters.
	inc, exc, ok := h.MountFilters("local/files/r/")
	if !ok {
		t.Fatal("MountFilters should find the registered mount")
	}
	if len(inc) != 0 || len(exc) != 0 {
		t.Errorf("initial filters not empty: inc=%v exc=%v", inc, exc)
	}

	// Set include + exclude.
	if !h.SetMountFilters("local/files/r/", []string{"*.md"}, []string{"draft.md"}) {
		t.Fatal("SetMountFilters returned false on a registered mount")
	}
	inc, exc, _ = h.MountFilters("local/files/r/")
	if len(inc) != 1 || inc[0] != "*.md" {
		t.Errorf("include = %v, want [*.md]", inc)
	}
	if len(exc) != 1 || exc[0] != "draft.md" {
		t.Errorf("exclude = %v, want [draft.md]", exc)
	}

	// Re-register preserves filters (peer-restart path simulates).
	h.RegisterMount("local/files/r/", "archives/notes/")
	inc, exc, _ = h.MountFilters("local/files/r/")
	if len(inc) != 1 || inc[0] != "*.md" {
		t.Errorf("after re-register: include lost: %v", inc)
	}
	if len(exc) != 1 || exc[0] != "draft.md" {
		t.Errorf("after re-register: exclude lost: %v", exc)
	}

	// Unknown mount: returns false.
	if h.SetMountFilters("local/files/nope/", nil, nil) {
		t.Error("SetMountFilters on unknown prefix should return false")
	}
}

// TestRegisterMount_NormalizeTrailingSlash — verifies both
// sourcePrefix and targetPrefix get a trailing slash regardless of
// input shape.
func TestRegisterMount_NormalizeTrailingSlash(t *testing.T) {
	h := NewNotificationIngestHandler(nil)
	h.RegisterMount("local/files/r", "archives/notes")
	_, _, ok := h.MountFilters("local/files/r/")
	if !ok {
		t.Error("source prefix should be normalized with trailing slash")
	}
	if got := h.LookupMount("local/files/r/"); got != "archives/notes/" {
		t.Errorf("target prefix = %q, want %q", got, "archives/notes/")
	}
}
