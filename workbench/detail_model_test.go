package workbench

import "testing"

func TestDetailModel_Empty(t *testing.T) {
	pc, _, _ := testPeerContext(t)
	_ = pc // cache removed

	m := NewDetailModel(pc)
	out := m.Render()
	if !out.Empty {
		t.Error("expected empty output for empty path")
	}
}

func TestDetailModel_Directory(t *testing.T) {
	pc, _, _ := testPeerContext(t)
	_ = pc // cache removed

	m := NewDetailModel(pc)
	m.LoadPath("nonexistent/path")
	out := m.Render()
	if !out.Directory {
		t.Error("expected directory output for missing entity")
	}
	if out.Path != "nonexistent/path" {
		t.Errorf("path = %q", out.Path)
	}
}

func TestDetailModel_Rendered(t *testing.T) {
	pc, s, li := testPeerContext(t)
	seedStore(t, s, li, "test/hello", "test/type", map[string]string{"greeting": "world"})
	_ = pc // cache removed

	m := NewDetailModel(pc)
	m.LoadPath("test/hello")
	out := m.Render()

	if out.Empty || out.Directory {
		t.Fatal("expected entity output")
	}
	if out.Header.Path != "test/hello" {
		t.Errorf("path = %q", out.Header.Path)
	}
	if out.Header.Type != "test/type" {
		t.Errorf("type = %q", out.Header.Type)
	}
	if out.RawView {
		t.Error("expected rendered view by default")
	}
	if len(out.Lines) == 0 {
		t.Error("expected formatted lines")
	}
	if out.Resolved == nil {
		t.Error("expected resolved entity")
	}
}

func TestDetailModel_Raw(t *testing.T) {
	pc, s, li := testPeerContext(t)
	seedStore(t, s, li, "test/hello", "test/type", "value")
	_ = pc // cache removed

	m := NewDetailModel(pc)
	m.ToggleRaw()
	m.LoadPath("test/hello")

	out := m.Render()
	if !out.RawView {
		t.Error("expected raw view after toggle")
	}
	if out.DiagHash == "" {
		t.Error("expected diagnostic hash")
	}
	if len(out.HexLines) == 0 {
		t.Error("expected hex lines")
	}
}

func TestDetailModel_Toggle(t *testing.T) {
	pc, _, _ := testPeerContext(t)
	m := NewDetailModel(pc)

	if m.RawView {
		t.Error("should start in rendered mode")
	}
	m.ToggleRaw()
	if !m.RawView {
		t.Error("should be raw after first toggle")
	}
	m.ToggleRaw()
	if m.RawView {
		t.Error("should be rendered after second toggle")
	}
}

func TestDetailModel_NavigateToHash(t *testing.T) {
	pc, s, li := testPeerContext(t)
	seedStore(t, s, li, "test/hello", "test/type", "world")
	_ = pc // cache removed

	m := NewDetailModel(pc)

	// Get the hash of the stored entity
	entries := pc.Store().List("")
	if len(entries) == 0 {
		t.Fatal("no entries")
	}
	hashStr := entries[0].Hash.String()

	path, ok := m.NavigateToHash(hashStr)
	if !ok {
		t.Fatal("expected to find entity by hash")
	}
	if path != "test/hello" {
		t.Errorf("path = %q, want test/hello", path)
	}

	// Non-existent hash
	_, ok = m.NavigateToHash("nonexistent")
	if ok {
		t.Error("expected not found for bogus hash")
	}
}
