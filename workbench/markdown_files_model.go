package workbench

import (
	"sort"
	"sync"

	"go.entitychurch.org/entity-core-go/core/store"
)

var _ Model[MarkdownFilesOutput] = (*MarkdownFilesModel)(nil)

// MarkdownFileRow is one visible row in the tree-shaped markdown
// files panel. Folder rows have HasEntry=false; file rows have
// HasEntry=true plus a resolved Title. Renderers consume Rows in
// order (already flattened from the tree with expand state applied)
// and indent by Depth.
type MarkdownFileRow struct {
	Path        string
	Segment     string
	Depth       int
	HasChildren bool
	Expanded    bool
	HasEntry    bool
	LeafCount   int
	Title       string // file rows only — title field from decoded entity, or filename
}

// MarkdownFilesOutput is the renderer-neutral output of the model.
type MarkdownFilesOutput struct {
	Rows  []MarkdownFileRow
	Error string
}

// MarkdownFilesModel is a type-driven, tree-shaped browser for
// doc/markdown-file entities under a configured prefix.
//
// The model OWNS a prefix-scoped subscription on the underlying
// WorkspaceState and maintains its own incremental snapshot of
// matching entries + titles. It does NOT iterate the whole tree on
// every refresh — that anti-pattern was the cause of the 22ms
// per-refresh-tick UI lag.
//
// Lifecycle:
//   - NewMarkdownFilesModel attaches the subscription.
//   - The SDK-owned watch goroutine drains tree events into m.onEvent,
//     which updates the local entries+titles maps with mutex.
//   - Refresh() rebuilds the tree from the local maps — O(matching N),
//     not O(whole-DB).
//   - Close() cancels the subscription.
//
// Tree state (expand state) persists across Refresh calls so the
// renderer's navigation cursor survives ambient updates.
type MarkdownFilesModel struct {
	store  *Store
	prefix string

	Type string

	// Subscription handle.
	cancel func()

	mu      sync.Mutex
	entries map[string]store.LocationEntry // qualified path → entry
	titles  map[string]string              // qualified path → resolved title
	dirty   bool                           // local snapshot changed since last Refresh

	root            *TreeNode
	visibleRows     []VisibleRow
	expanded        map[string]bool
	lastFingerprint string

	LastError string

	// Selection slot binding (set via BindSelection). The markdown
	// files panel both writes (navigate) and reads (highlight current)
	// the selection, so the read side is a cached path kept current by
	// an OnSelectionChange subscription — not a per-refresh store Get.
	selState   *WorkspaceState
	selScreen  int
	selCancel  func()
	extSelPath string
}

// PeerCtx is retained as a no-op deprecation shim — the model no
// longer holds or needs a PeerContext.
//
// Deprecated: PeerContext is being removed. Returns nil.
func (m *MarkdownFilesModel) PeerCtx() *PeerContext { return nil }

// Root returns the filtered tree root for renderers that want to walk
// the *TreeNode graph directly (e.g. tview.TreeView). Returns nil
// when no entities matched. Renderers using the flat Render() API
// don't need this.
func (m *MarkdownFilesModel) Root() *TreeNode { return m.root }

// TitleFor returns the resolved title for an entity at the given
// path, or empty string if the path is not a markdown-file entity in
// the current snapshot. Mirrors the per-row Title field exposed via
// Render() so tree-walking renderers can label leaves consistently.
func (m *MarkdownFilesModel) TitleFor(path string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.titles[path]
}

// NewMarkdownFilesModel creates a model that watches `prefix` and
// surfaces matching MarkdownFileType ("doc/markdown-file") entities.
// `prefix` is a tree-relative or qualified prefix (e.g. "docs/");
// the model appends `*` internally for the underlying Store.Watch
// pattern.
//
// Panics on empty prefix or nil store — programmer errors.
func NewMarkdownFilesModel(st *Store, prefix string) *MarkdownFilesModel {
	if st == nil {
		panic("workbench: NewMarkdownFilesModel requires non-nil Store")
	}
	if prefix == "" {
		panic("workbench: NewMarkdownFilesModel requires a non-empty prefix")
	}
	m := &MarkdownFilesModel{
		store:    st,
		prefix:   prefix,
		Type:     MarkdownFileType,
		entries:  make(map[string]store.LocationEntry),
		titles:   make(map[string]string),
		expanded: make(map[string]bool),
		dirty:    true,
	}
	m.cancel = st.OnPrefixChange(prefix, m.onEvent)
	return m
}

// Close cancels the underlying subscriptions. Idempotent.
func (m *MarkdownFilesModel) Close() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	if m.selCancel != nil {
		m.selCancel()
		m.selCancel = nil
	}
}

// BindSelection wires the model to a screen's selection slot — the
// authoritative selection value (in the tree) replaces the old
// in-memory SelectionState. SelectedPath() observes a cached path
// kept current by an OnSelectionChange subscription; PublishSelection
// writes the slot. Nil state (unit tests / no presentation context)
// is a no-op for publish and an empty SelectedPath. Idempotent.
func (m *MarkdownFilesModel) BindSelection(state *WorkspaceState, screenIdx int) {
	m.mu.Lock()
	if m.selCancel != nil {
		m.selCancel()
		m.selCancel = nil
	}
	m.selState = state
	m.selScreen = screenIdx
	if state != nil {
		if sel, ok := state.ReadSelection(screenIdx); ok {
			m.extSelPath = sel.Path
		}
	}
	m.mu.Unlock()
	m.selCancel = bindSelectionWatch(state, screenIdx, func(sel Selection) {
		m.mu.Lock()
		m.extSelPath = sel.Path
		m.mu.Unlock()
	})
}

// SelectedPath returns the currently-selected path as observed from
// the screen selection slot (cached; no store read on the hot path).
func (m *MarkdownFilesModel) SelectedPath() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.extSelPath
}

// PublishSelection writes the given path to the screen selection slot
// (the value-of-truth in the tree). Empty state is a no-op.
func (m *MarkdownFilesModel) PublishSelection(path string) {
	if m.selState == nil {
		return
	}
	m.selState.SaveSelection(m.selScreen, Selection{Path: path, Type: "entity"})
}

// onEvent is the SDK watch callback. Runs on an SDK-owned goroutine;
// must not call back into tview / raylib directly. Renderers learn
// about state changes via the next Refresh() call (driven by the
// renderer's tick loop).
func (m *MarkdownFilesModel) onEvent(ev ChangeEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch ev.EventType {
	case ChangePut:
		ent, ok := m.store.Get(ev.Path)
		if !ok {
			// Hash present but entity gone — concurrent remove.
			// Treat as removal of any prior cached state.
			if _, was := m.entries[ev.Path]; was {
				delete(m.entries, ev.Path)
				delete(m.titles, ev.Path)
				m.dirty = true
			}
			return
		}
		if ent.Type != m.Type {
			// Path holds an entity of a different type. Ensure we don't
			// carry stale state for this path (it may have changed type
			// from markdown-file to something else).
			if _, was := m.entries[ev.Path]; was {
				delete(m.entries, ev.Path)
				delete(m.titles, ev.Path)
				m.dirty = true
			}
			return
		}
		m.entries[ev.Path] = store.LocationEntry{Path: ev.Path, Hash: ev.NewHash}
		decoded, _ := DecodeEntityData(ent.Data)
		if title, ok := titleFromDecoded(decoded); ok && title != "" {
			m.titles[ev.Path] = title
		} else {
			delete(m.titles, ev.Path)
		}
		m.dirty = true

	case ChangeRemove:
		if _, was := m.entries[ev.Path]; was {
			delete(m.entries, ev.Path)
			delete(m.titles, ev.Path)
			m.dirty = true
		}
	}
}

// Refresh rebuilds the folder tree from the local incremental
// snapshot, if the snapshot has changed since the last call. Expand
// state is preserved by path.
//
// Returns true if the tree was rebuilt; false otherwise. Renderers
// should skip their widget-tree rebuild when this returns false to
// preserve keyboard navigation state.
func (m *MarkdownFilesModel) Refresh() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LastError = ""

	if !m.dirty && m.root != nil {
		return false
	}
	m.dirty = false

	entries := make([]store.LocationEntry, 0, len(m.entries))
	for _, e := range m.entries {
		entries = append(entries, e)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})

	fp := fingerprintEntries(entries)
	if m.root != nil && fp == m.lastFingerprint {
		return false
	}
	m.lastFingerprint = fp
	m.root = BuildTree(entries)
	RestoreExpanded(m.root, m.expanded)
	m.rebuildVisible()
	return true
}

// fingerprintEntries returns a stable string identifying the
// (sorted) entry set. Uses path + content hash so a content edit at
// the same path counts as a change but a no-op refresh doesn't.
func fingerprintEntries(entries []store.LocationEntry) string {
	var b []byte
	for _, e := range entries {
		b = append(b, e.Path...)
		b = append(b, '@')
		b = append(b, e.Hash.String()...)
		b = append(b, ';')
	}
	return string(b)
}

// rebuildVisible re-flattens the tree using the current expand state.
// Caller holds m.mu.
func (m *MarkdownFilesModel) rebuildVisible() {
	if m.root == nil {
		m.visibleRows = nil
		return
	}
	m.visibleRows = FlattenVisible(m.root)
}

// Render produces the renderer-neutral output.
func (m *MarkdownFilesModel) Render() MarkdownFilesOutput {
	m.mu.Lock()
	defer m.mu.Unlock()
	rows := make([]MarkdownFileRow, len(m.visibleRows))
	for i, vr := range m.visibleRows {
		node := vr.Node
		row := MarkdownFileRow{
			Path:        node.FullPath,
			Segment:     node.Segment,
			Depth:       vr.Depth,
			HasChildren: len(node.Children) > 0,
			Expanded:    node.Expanded,
			HasEntry:    node.HasEntry,
		}
		if node.HasEntry {
			if t, ok := m.titles[node.FullPath]; ok && t != "" {
				row.Title = t
			} else {
				row.Title = node.Segment
			}
		}
		if !node.HasEntry && len(node.Children) > 0 {
			row.LeafCount = CountLeaves(node)
		}
		rows[i] = row
	}
	return MarkdownFilesOutput{Rows: rows, Error: m.LastError}
}

// FindByPath returns the index of the visible row at the given path,
// or -1 if the path is not currently visible (e.g. inside a collapsed
// ancestor). Use ExpandToPath first to make a path visible.
func (m *MarkdownFilesModel) FindByPath(path string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, vr := range m.visibleRows {
		if vr.Node.FullPath == path {
			return i
		}
	}
	return -1
}

// ToggleExpand flips the expand state of the node at the given visible
// row index. No-op for leaf nodes (no children).
func (m *MarkdownFilesModel) ToggleExpand(index int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if index < 0 || index >= len(m.visibleRows) {
		return
	}
	node := m.visibleRows[index].Node
	if len(node.Children) == 0 {
		return
	}
	node.Expanded = !node.Expanded
	m.expanded[node.FullPath] = node.Expanded
	if !node.Expanded {
		delete(m.expanded, node.FullPath)
	}
	m.rebuildVisible()
}

// Expand opens the node at the given visible row index.
func (m *MarkdownFilesModel) Expand(index int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if index < 0 || index >= len(m.visibleRows) {
		return
	}
	node := m.visibleRows[index].Node
	if len(node.Children) == 0 || node.Expanded {
		return
	}
	node.Expanded = true
	m.expanded[node.FullPath] = true
	m.rebuildVisible()
}

// Collapse closes the node at the given visible row index.
func (m *MarkdownFilesModel) Collapse(index int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if index < 0 || index >= len(m.visibleRows) {
		return
	}
	node := m.visibleRows[index].Node
	if !node.Expanded {
		return
	}
	node.Expanded = false
	delete(m.expanded, node.FullPath)
	m.rebuildVisible()
}

// ExpandToPath ensures every ancestor of path is expanded so the
// target row becomes visible. Returns true if the path exists in the
// tree, false otherwise.
func (m *MarkdownFilesModel) ExpandToPath(path string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.root == nil || path == "" {
		return false
	}
	ExpandAncestors(m.root, path)
	for p := range CollectExpanded(m.root) {
		m.expanded[p] = true
	}
	m.rebuildVisible()
	return true
}

// lastSegment returns the substring after the final "/" in path, or
// the whole string if there's no slash. Kept in this file for
// historical reasons — markdown_view_model.go uses it too.
func lastSegment(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}

// titleFromDecoded extracts the optional "title" string field from
// the decoded data of a markdown-file entity. Tolerates both
// map[string]interface{} and map[interface{}]interface{} shapes that
// CBOR decoders may produce.
func titleFromDecoded(decoded interface{}) (string, bool) {
	if decoded == nil {
		return "", false
	}
	if m, ok := decoded.(map[string]interface{}); ok {
		if t, ok := m["title"].(string); ok {
			return t, true
		}
		return "", false
	}
	if m, ok := decoded.(map[interface{}]interface{}); ok {
		if t, ok := m["title"].(string); ok {
			return t, true
		}
	}
	return "", false
}
