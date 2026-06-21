package workbench

import (
	"strings"
	"testing"
	"time"
)

// poll repeatedly invokes refresh+predicate until it returns true or
// the timeout elapses. Bench/model state is updated by an SDK-owned
// goroutine; tests poll instead of synchronizing.
func poll(t *testing.T, m *MarkdownFilesModel, predicate func() bool, timeout time.Duration, what string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		m.Refresh()
		if predicate() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s: predicate never satisfied within %s; final rows: %+v",
		what, timeout, m.Render().Rows)
}

// containsPath checks if any row in the model's render output has a
// path containing `needle` (string match — covers both bare and
// peer-id-qualified path forms).
func containsPath(m *MarkdownFilesModel, needle string) bool {
	for _, r := range m.Render().Rows {
		if r.HasEntry && strings.Contains(r.Path, needle) {
			return true
		}
	}
	return false
}

// hasEntryCount counts rows with HasEntry=true.
func hasEntryCount(m *MarkdownFilesModel) int {
	n := 0
	for _, r := range m.Render().Rows {
		if r.HasEntry {
			n++
		}
	}
	return n
}

// TestMarkdownFilesModel_SeedFromExistingEntries verifies entities put
// BEFORE model construction are picked up via the seed phase.
func TestMarkdownFilesModel_SeedFromExistingEntries(t *testing.T) {
	ap, err := NewAppPeer()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })
	st := ap.Store()

	for _, p := range []string{
		"docs/archives/notes/a.md",
		"docs/archives/notes/sub/b.md",
		"docs/intro.md",
	} {
		if _, err := st.Put(p, MarkdownFileType,
			map[string]interface{}{"content": "x"}); err != nil {
			t.Fatalf("seed put %s: %v", p, err)
		}
	}

	m := NewMarkdownFilesModel(st, "docs/")
	t.Cleanup(m.Close)

	// We don't assert tree shape (depends on path qualification) — we
	// assert all three leaves are reachable + the expected count.
	poll(t, m,
		func() bool {
			// Expand every collapsed node so all leaves become visible.
			for {
				expanded := false
				for i, r := range m.Render().Rows {
					if r.HasChildren && !r.Expanded {
						m.Expand(i)
						expanded = true
						break
					}
				}
				if !expanded {
					break
				}
			}
			return hasEntryCount(m) == 3
		},
		2*time.Second, "seed picks up 3 entries")

	for _, want := range []string{"archives/notes/a.md", "archives/notes/sub/b.md", "intro.md"} {
		if !containsPath(m, want) {
			t.Errorf("missing seeded path: %s", want)
		}
	}
}

// TestMarkdownFilesModel_FiltersByType verifies non-markdown entities
// under the same prefix are not surfaced.
func TestMarkdownFilesModel_FiltersByType(t *testing.T) {
	ap, err := NewAppPeer()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })
	st := ap.Store()

	if _, err := st.Put("docs/keep.md", MarkdownFileType,
		map[string]interface{}{"content": "yes"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Put("docs/skip.txt", "doc/text-file",
		map[string]interface{}{"content": "no"}); err != nil {
		t.Fatal(err)
	}

	m := NewMarkdownFilesModel(st, "docs/")
	t.Cleanup(m.Close)

	// Wait for keep.md to surface AND skip.txt to be absent.
	poll(t, m,
		func() bool {
			// Expand all
			for {
				expanded := false
				for i, r := range m.Render().Rows {
					if r.HasChildren && !r.Expanded {
						m.Expand(i)
						expanded = true
						break
					}
				}
				if !expanded {
					break
				}
			}
			return containsPath(m, "keep.md") && !containsPath(m, "skip.txt")
		},
		2*time.Second, "keep.md present + skip.txt absent")
}

// TestMarkdownFilesModel_LiveUpdate verifies a Put after model
// construction surfaces in the tree without an explicit refresh
// trigger.
func TestMarkdownFilesModel_LiveUpdate(t *testing.T) {
	ap, err := NewAppPeer()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })
	st := ap.Store()

	m := NewMarkdownFilesModel(st, "docs/")
	t.Cleanup(m.Close)

	// Empty initially (briefly — even seed-on-empty fires no events).
	m.Refresh()
	if hasEntryCount(m) != 0 {
		t.Fatalf("expected empty model, got %d entries", hasEntryCount(m))
	}

	// Add a file post-construction.
	if _, err := st.Put("docs/live.md", MarkdownFileType,
		map[string]interface{}{"title": "Live"}); err != nil {
		t.Fatal(err)
	}

	poll(t, m,
		func() bool {
			for {
				expanded := false
				for i, r := range m.Render().Rows {
					if r.HasChildren && !r.Expanded {
						m.Expand(i)
						expanded = true
						break
					}
				}
				if !expanded {
					break
				}
			}
			return containsPath(m, "live.md")
		},
		2*time.Second, "live put surfaces")
}

// TestMarkdownFilesModel_LiveRemove verifies a Remove drops the entry.
func TestMarkdownFilesModel_LiveRemove(t *testing.T) {
	ap, err := NewAppPeer()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })
	st := ap.Store()

	if _, err := st.Put("docs/gone.md", MarkdownFileType,
		map[string]interface{}{"content": "bye"}); err != nil {
		t.Fatal(err)
	}

	m := NewMarkdownFilesModel(st, "docs/")
	t.Cleanup(m.Close)

	expandAll := func() {
		for {
			expanded := false
			for i, r := range m.Render().Rows {
				if r.HasChildren && !r.Expanded {
					m.Expand(i)
					expanded = true
					break
				}
			}
			if !expanded {
				break
			}
		}
	}

	poll(t, m,
		func() bool { expandAll(); return hasEntryCount(m) == 1 },
		2*time.Second, "seed picks up gone.md")

	if !st.Remove("docs/gone.md") {
		t.Fatal("Remove returned false")
	}

	poll(t, m,
		func() bool { expandAll(); return hasEntryCount(m) == 0 },
		2*time.Second, "remove drops the entry")
}

// TestMarkdownFilesModel_IgnoresOutsidePrefix verifies entities outside
// the watched prefix are never surfaced.
func TestMarkdownFilesModel_IgnoresOutsidePrefix(t *testing.T) {
	ap, err := NewAppPeer()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })
	st := ap.Store()

	if _, err := st.Put("other/elsewhere.md", MarkdownFileType,
		map[string]interface{}{"content": "no"}); err != nil {
		t.Fatal(err)
	}

	m := NewMarkdownFilesModel(st, "docs/")
	t.Cleanup(m.Close)

	// Wait briefly to ensure no spurious delivery.
	time.Sleep(150 * time.Millisecond)
	m.Refresh()
	if hasEntryCount(m) != 0 {
		t.Errorf("entity outside prefix leaked into model: %+v", m.Render().Rows)
	}
}
