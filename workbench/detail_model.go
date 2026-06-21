package workbench

var _ Model[DetailOutput] = (*DetailModel)(nil)

// DetailModel is the business logic for the entity detail panel.
// It owns the currently displayed path, entity resolution, the
// raw/rendered toggle, and output generation. Renderers update the
// model's path via LoadPath (typically off the screen selection slot,
// observed via WorkspaceState.OnSelectionChange) and then call Render.
type DetailModel struct {
	peerCtx *PeerContext
	Path    string
	RawView bool
}

// DetailOutput is the renderer-neutral output of the detail model.
type DetailOutput struct {
	// Path is the entity path being displayed.
	Path string
	// Empty is true when no path is selected.
	Empty bool
	// Directory is true when the path has no entity (directory node).
	Directory bool

	// Header fields (only set when entity exists).
	Header EntityHeader
	// RawView indicates which view mode is active.
	RawView bool

	// Rendered view (when RawView == false):
	Lines []FormattedLine

	// Raw view (when RawView == true):
	DiagHash string
	HexLines []string

	// The resolved entity (for renderer-specific needs like hash links).
	Resolved *ResolvedEntity
}

// PeerCtx returns the underlying PeerContext.
func (m *DetailModel) PeerCtx() *PeerContext { return m.peerCtx }

// NewDetailModel creates a detail model.
func NewDetailModel(peerCtx *PeerContext) *DetailModel {
	return &DetailModel{peerCtx: peerCtx}
}

// ToggleRaw switches between raw and rendered view.
func (m *DetailModel) ToggleRaw() {
	m.RawView = !m.RawView
}

// LoadPath sets the path the detail panel will display. The next
// call to Render resolves the entity at this path.
func (m *DetailModel) LoadPath(path string) {
	m.Path = path
}

// Render produces the detail output for the currently loaded path.
func (m *DetailModel) Render() DetailOutput {
	if m.Path == "" {
		return DetailOutput{Empty: true}
	}

	resolved, ok := ResolveEntity(m.peerCtx, m.Path)
	if !ok {
		return DetailOutput{Path: m.Path, Directory: true}
	}

	out := DetailOutput{
		Path:     m.Path,
		Header:   HeaderFromResolved(resolved),
		RawView:  m.RawView,
		Resolved: &resolved,
	}

	if m.RawView {
		out.DiagHash = FormatDiagnoseHash(resolved)
		out.HexLines = FormatHexDump(resolved.Entity.Data)
	} else {
		if resolved.Decoded != nil {
			out.Lines = FormatCBOR(resolved.Decoded)
		}
	}

	return out
}

// NavigateToHash finds an entry whose hash matches and returns its path.
// Returns ("", false) if no match found.
//
// Iterates the store via List("") — this is a click-time operation (hash
// link in the inspector) not a per-event hot path, so the O(N) cost is
// bounded by user interaction rate. Long-term, a reverse-hash-index
// (query.ReverseHashIndex) would make this O(1).
func (m *DetailModel) NavigateToHash(hashStr string) (string, bool) {
	if m.peerCtx == nil {
		return "", false
	}
	for _, entry := range m.peerCtx.Store().List("") {
		if entry.Hash.String() == hashStr {
			return entry.Path, true
		}
	}
	return "", false
}
