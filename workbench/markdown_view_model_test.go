package workbench

import (
	"testing"

	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/content"
	"go.entitychurch.org/entity-core-go/ext/content/chunker"
)

// seedMarkdown writes a doc/markdown-file entity at path via the same
// Store.Put surface MarkdownViewModel.Save uses, so tests exercise the
// real round-trip (Put → Resolve), not a hand-encoded fixture. Content
// is chunked into the content store; the typed entity carries only the
// blob hash (CONTENT v3.6 substrate shape).
func seedMarkdown(t *testing.T, pc *PeerContext, path, title, body string) {
	t.Helper()
	raw := []byte(body)
	ranges := chunker.ChunkFastCDC(raw, types.DefaultChunkSize)
	blobHash, err := content.IngestBlob(raw, ranges, types.ChunkingFastCDC, types.DefaultChunkSize, pc.Store().ContentStore())
	if err != nil {
		t.Fatalf("seed %s: ingest blob: %v", path, err)
	}
	md := MarkdownFileData{
		Path:    path,
		Title:   title,
		Content: blobHash,
		Size:    int64(len(raw)),
	}
	if _, err := pc.Store().Put(path, MarkdownFileType, md); err != nil {
		t.Fatalf("seed %s: %v", path, err)
	}
}

func TestMarkdownViewModel_EmptyAndNotFound(t *testing.T) {
	pc, _, _ := testPeerContext(t)
	m := NewMarkdownViewModel(pc)

	if out := m.Render(); !out.Empty || out.NotFound {
		t.Fatalf("fresh model: want Empty, got %+v", out)
	}
	if m.Path() != "" {
		t.Fatalf("fresh Path() = %q, want empty", m.Path())
	}

	m.Load("docs/missing.md")
	out := m.Render()
	if out.Empty || !out.NotFound {
		t.Fatalf("missing path: want NotFound, got %+v", out)
	}
	if out.Path != "docs/missing.md" {
		t.Fatalf("NotFound Path = %q", out.Path)
	}
}

func TestMarkdownViewModel_WrongTypeIsNotFound(t *testing.T) {
	pc, _, _ := testPeerContext(t)
	if _, err := pc.Store().Put("docs/note.md", "doc/text-file",
		map[string]interface{}{"content": "x"}); err != nil {
		t.Fatal(err)
	}
	m := NewMarkdownViewModel(pc)
	m.Load("docs/note.md")
	if out := m.Render(); !out.NotFound {
		t.Fatalf("non-markdown entity: want NotFound, got %+v", out)
	}
}

func TestMarkdownViewModel_RendersSavedAndTitleFallback(t *testing.T) {
	pc, _, _ := testPeerContext(t)
	seedMarkdown(t, pc, "docs/intro.md", "Intro", "hello body")
	seedMarkdown(t, pc, "docs/untitled.md", "", "no title here")

	m := NewMarkdownViewModel(pc)
	m.Load("docs/intro.md")
	out := m.Render()
	if out.Empty || out.NotFound || out.Editing {
		t.Fatalf("loaded md: unexpected flags %+v", out)
	}
	if out.Title != "Intro" || out.Content != "hello body" {
		t.Fatalf("loaded md: got title=%q content=%q", out.Title, out.Content)
	}

	// Empty title falls back to the path's last segment.
	m.Load("docs/untitled.md")
	if out := m.Render(); out.Title != "untitled.md" {
		t.Fatalf("title fallback: got %q, want %q", out.Title, "untitled.md")
	}
}

func TestMarkdownViewModel_LoadFromPath(t *testing.T) {
	pc, _, _ := testPeerContext(t)
	seedMarkdown(t, pc, "docs/a.md", "A", "a")
	m := NewMarkdownViewModel(pc)

	if m.LoadFromPath("") {
		t.Fatal("LoadFromPath(\"\") should return false")
	}
	if m.Path() != "" {
		t.Fatalf("empty LoadFromPath mutated path to %q", m.Path())
	}
	if !m.LoadFromPath("docs/a.md") {
		t.Fatal("LoadFromPath(non-empty) should return true")
	}
	if m.Path() != "docs/a.md" {
		t.Fatalf("Path() = %q after LoadFromPath", m.Path())
	}
}

func TestMarkdownViewModel_EnterEditNoOpWhenEmptyOrNotFound(t *testing.T) {
	pc, _, _ := testPeerContext(t)
	m := NewMarkdownViewModel(pc)

	m.EnterEdit() // no path
	if m.IsEditing() {
		t.Fatal("EnterEdit with no path should be a no-op")
	}
	m.Load("docs/missing.md")
	m.EnterEdit() // path set but NotFound
	if m.IsEditing() {
		t.Fatal("EnterEdit on NotFound path should be a no-op")
	}
}

func TestMarkdownViewModel_EditLifecycle(t *testing.T) {
	pc, _, _ := testPeerContext(t)
	seedMarkdown(t, pc, "docs/edit.md", "Saved Title", "saved content")
	m := NewMarkdownViewModel(pc)
	m.Load("docs/edit.md")

	m.EnterEdit()
	if !m.IsEditing() || m.IsDirty() {
		t.Fatalf("after EnterEdit: editing=%v dirty=%v (want true,false)", m.IsEditing(), m.IsDirty())
	}
	if out := m.Render(); !out.Editing || out.Title != "Saved Title" || out.Content != "saved content" {
		t.Fatalf("EnterEdit should seed draft from saved: %+v", out)
	}

	// Setting the same value is not a modification.
	m.UpdateTitle("Saved Title")
	if m.IsDirty() {
		t.Fatal("UpdateTitle with unchanged value should not dirty")
	}
	m.UpdateContent("saved content")
	if m.IsDirty() {
		t.Fatal("UpdateContent with unchanged value should not dirty")
	}

	// Real changes dirty the draft and Render reflects the draft.
	m.UpdateTitle("New Title")
	m.UpdateContent("new content")
	if !m.IsDirty() {
		t.Fatal("draft change should set dirty")
	}
	if out := m.Render(); out.Title != "New Title" || out.Content != "new content" || !out.Dirty {
		t.Fatalf("editing Render should reflect draft: %+v", out)
	}

	// ExitEdit discards the draft, reverting to the saved entity.
	m.ExitEdit()
	if m.IsEditing() || m.IsDirty() {
		t.Fatalf("after ExitEdit: editing=%v dirty=%v", m.IsEditing(), m.IsDirty())
	}
	if out := m.Render(); out.Editing || out.Title != "Saved Title" || out.Content != "saved content" {
		t.Fatalf("ExitEdit should revert to saved: %+v", out)
	}

	// Update* are no-ops when not editing.
	m.UpdateTitle("ignored")
	m.UpdateContent("ignored")
	if out := m.Render(); out.Title != "Saved Title" || out.Content != "saved content" {
		t.Fatalf("Update* outside edit mode must be no-ops: %+v", out)
	}
}

func TestMarkdownViewModel_LoadDiscardsDraftOnPathChange(t *testing.T) {
	pc, _, _ := testPeerContext(t)
	seedMarkdown(t, pc, "docs/a.md", "A", "a-body")
	seedMarkdown(t, pc, "docs/b.md", "B", "b-body")
	m := NewMarkdownViewModel(pc)
	m.Load("docs/a.md")
	m.EnterEdit()
	m.UpdateContent("unsaved edit")
	if !m.IsDirty() {
		t.Fatal("precondition: draft should be dirty")
	}

	// Load(samePath) is an early-return no-op: the dirty draft survives.
	m.Load("docs/a.md")
	if !m.IsEditing() || !m.IsDirty() {
		t.Fatal("Load(samePath) must not discard the in-progress draft")
	}

	// Load(differentPath) silently discards the draft (documented v1
	// limitation) and shows the new file's saved content.
	m.Load("docs/b.md")
	if m.IsEditing() || m.IsDirty() {
		t.Fatalf("Load(newPath): editing=%v dirty=%v, want false,false", m.IsEditing(), m.IsDirty())
	}
	if out := m.Render(); out.Title != "B" || out.Content != "b-body" {
		t.Fatalf("after path change: %+v", out)
	}
}

func TestMarkdownViewModel_SavePersists(t *testing.T) {
	pc, _, _ := testPeerContext(t)
	seedMarkdown(t, pc, "docs/save.md", "Old", "old body")
	m := NewMarkdownViewModel(pc)
	m.Load("docs/save.md")

	// Save outside edit mode is a no-op (nil) and must not mutate.
	if err := m.Save(); err != nil {
		t.Fatalf("Save() not editing: %v", err)
	}
	m.Load("docs/save.md")
	if out := m.Render(); out.Title != "Old" || out.Content != "old body" {
		t.Fatalf("no-op Save mutated the entity: %+v", out)
	}

	m.EnterEdit()
	m.UpdateTitle("New")
	m.UpdateContent("new body")
	if err := m.Save(); err != nil {
		t.Fatalf("Save() = %v", err)
	}
	if m.IsEditing() || m.IsDirty() {
		t.Fatalf("after Save: editing=%v dirty=%v, want false,false", m.IsEditing(), m.IsDirty())
	}
	if out := m.Render(); out.Editing || out.Title != "New" || out.Content != "new body" {
		t.Fatalf("post-Save Render: %+v", out)
	}

	// Durability: a fresh model loading the same path sees the write.
	fresh := NewMarkdownViewModel(pc)
	fresh.Load("docs/save.md")
	if out := fresh.Render(); out.Title != "New" || out.Content != "new body" {
		t.Fatalf("Save did not persist for a fresh model: %+v", out)
	}
}
