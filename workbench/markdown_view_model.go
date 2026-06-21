package workbench

import (
	"fmt"

	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/content"
	"go.entitychurch.org/entity-core-go/ext/content/chunker"
)

var _ Model[MarkdownViewOutput] = (*MarkdownViewModel)(nil)

// MarkdownViewModel is the read/edit model for a single
// doc/markdown-file entity at any path. Unlike the retired article
// model, the identity here is the full path — there is no slug
// concept, no fixed prefix, no slug→path translation. The path is
// whatever the file was mounted at (or written to via shell `put`).
//
// State ownership:
//   - The model owns the currently loaded path.
//   - The model owns the edit-mode draft buffer.
//   - Renderers detect external selection changes and call
//     LoadFromPath / Load to update the model.
//
// v1 limitation: Load() during an unsaved edit discards the draft
// silently. Renderers should warn the user to save before navigating
// away. Promoting the draft to a durable workspace entity is a future
// concern.
type MarkdownViewModel struct {
	peerCtx *PeerContext

	path string // empty when no file loaded

	editing bool
	draft   markdownDraft
	dirty   bool
}

type markdownDraft struct {
	title   string
	content string
}

// MarkdownViewOutput is the renderer-neutral output.
type MarkdownViewOutput struct {
	// Empty is true when no file is loaded (selection has not pointed
	// at a doc/markdown-file path yet).
	Empty bool
	// NotFound is true when a path is loaded but the entity does not
	// exist or is not a doc/markdown-file.
	NotFound bool

	Path    string
	Title   string
	Content string

	// Editing is true when the model is in edit mode. Title and
	// Content reflect the draft buffer instead of the saved entity.
	Editing bool
	// Dirty is true when the draft has been modified since EnterEdit
	// (or since the last save).
	Dirty bool
}

// PeerCtx returns the underlying PeerContext.
func (m *MarkdownViewModel) PeerCtx() *PeerContext { return m.peerCtx }

// NewMarkdownViewModel creates a markdown view model.
func NewMarkdownViewModel(peerCtx *PeerContext) *MarkdownViewModel {
	return &MarkdownViewModel{peerCtx: peerCtx}
}

// Path returns the currently loaded path, or "" if none.
func (m *MarkdownViewModel) Path() string { return m.path }

// Load sets the currently loaded path. The next Render resolves the
// entity at that path.
//
// v1 limitation: silently discards an unsaved draft. Renderers should
// call IsDirty() first if they want to warn the user.
func (m *MarkdownViewModel) Load(path string) {
	if path == m.path {
		return
	}
	m.path = path
	m.editing = false
	m.draft = markdownDraft{}
	m.dirty = false
}

// LoadFromPath is an alias for Load. Kept as a separate method to
// match the renderer-side wiring contract (renderers may want a
// pre-check or different behavior on the "from selection" path
// later — the indirection is cheap).
func (m *MarkdownViewModel) LoadFromPath(path string) bool {
	if path == "" {
		return false
	}
	m.Load(path)
	return true
}

// IsEditing returns true when the model is in edit mode.
func (m *MarkdownViewModel) IsEditing() bool { return m.editing }

// IsDirty returns true when the draft has unsaved changes.
func (m *MarkdownViewModel) IsDirty() bool { return m.dirty }

// EnterEdit copies the currently rendered file into the draft buffer
// and switches to edit mode. No-op if no file is loaded or the path
// points at a non-markdown entity.
func (m *MarkdownViewModel) EnterEdit() {
	if m.path == "" {
		return
	}
	out := m.renderSaved()
	if out.NotFound {
		return
	}
	m.draft = markdownDraft{
		title:   out.Title,
		content: out.Content,
	}
	m.editing = true
	m.dirty = false
}

// ExitEdit discards the draft and returns to read-only display.
func (m *MarkdownViewModel) ExitEdit() {
	m.editing = false
	m.draft = markdownDraft{}
	m.dirty = false
}

// UpdateTitle replaces the draft's title. No-op if not editing.
func (m *MarkdownViewModel) UpdateTitle(title string) {
	if !m.editing {
		return
	}
	if m.draft.title != title {
		m.draft.title = title
		m.dirty = true
	}
}

// UpdateContent replaces the draft's content. No-op if not editing.
func (m *MarkdownViewModel) UpdateContent(content string) {
	if !m.editing {
		return
	}
	if m.draft.content != content {
		m.draft.content = content
		m.dirty = true
	}
}

// Save writes the draft back as a doc/markdown-file entity at the
// loaded path. On success, exits edit mode and marks the peer context
// dirty so the files list refreshes. No-op (returns nil) if not editing.
//
// The write uses the peer's Level 0 direct-store surface — the panel
// runs under the peer owner's authority. When a mount is bound at
// this path, the write is round-tripped to disk by the
// notification-ingest → fsnotify path (Phase E).
func (m *MarkdownViewModel) Save() error {
	if !m.editing {
		return nil
	}
	if m.path == "" {
		return fmt.Errorf("no path loaded")
	}
	// Chunk the draft via FastCDC, persist blob + chunks to the content
	// store, then bind a doc/markdown-file entity referring to the new
	// blob hash. Mirrors the localfiles handler's read/write paths and
	// matches the ingest_transform output shape — large drafts no longer
	// re-encode the whole payload inside the typed entity.
	body := []byte(m.draft.content)
	ranges := chunker.ChunkFastCDC(body, types.DefaultChunkSize)
	blobHash, err := content.IngestBlob(body, ranges, types.ChunkingFastCDC, types.DefaultChunkSize, m.peerCtx.Store().ContentStore())
	if err != nil {
		return fmt.Errorf("ingest blob: %w", err)
	}
	md := MarkdownFileData{
		Path:    m.path,
		Title:   m.draft.title,
		Content: blobHash,
		Size:    int64(len(body)),
	}
	if _, err := m.peerCtx.Store().Put(m.path, MarkdownFileType, md); err != nil {
		return err
	}
	m.editing = false
	m.draft = markdownDraft{}
	m.dirty = false
	return nil
}

// Render produces the view output. When editing, Title/Content reflect
// the draft; otherwise the saved entity.
func (m *MarkdownViewModel) Render() MarkdownViewOutput {
	out := m.renderSaved()
	if m.editing {
		out.Editing = true
		out.Dirty = m.dirty
		out.Title = m.draft.title
		out.Content = m.draft.content
	}
	return out
}

func (m *MarkdownViewModel) renderSaved() MarkdownViewOutput {
	if m.path == "" {
		return MarkdownViewOutput{Empty: true}
	}
	resolved, ok := m.peerCtx.Resolve(m.path)
	if !ok {
		return MarkdownViewOutput{Path: m.path, NotFound: true}
	}
	if resolved.Entity.Type != MarkdownFileType {
		return MarkdownViewOutput{Path: m.path, NotFound: true}
	}
	md, err := MarkdownFileDataFromEntity(resolved.Entity)
	if err != nil {
		return MarkdownViewOutput{Path: m.path, NotFound: true}
	}
	title := md.Title
	if title == "" {
		title = lastSegment(m.path)
	}
	// Reassemble bytes from the local content store. Missing blob means
	// the typed entity is bound but the content chunks haven't arrived
	// yet (e.g. mid-subscription on a cross-peer sync); render empty
	// rather than NotFound so the panel can show the title + size while
	// content settles.
	cs := m.peerCtx.Store().ContentStore()
	body, present, err := LoadMarkdownContent(cs, md)
	content := ""
	if err == nil && present {
		content = string(body)
	}
	return MarkdownViewOutput{
		Path:    m.path,
		Title:   title,
		Content: content,
	}
}
