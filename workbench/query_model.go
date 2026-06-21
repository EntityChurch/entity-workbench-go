package workbench

import (
	"fmt"

	"go.entitychurch.org/entity-core-go/core/types"
)

var _ Model[QueryOutput] = (*QueryModel)(nil)

// QueryOutput is the renderer-neutral output of the query model.
// Includes both the user-input filter fields and the result state so
// renderers can draw the panel without reaching into the model.
type QueryOutput struct {
	TypeFilter  string
	PathPrefix  string
	Matches     []types.QueryMatchData
	Total       uint64
	HasMore     bool
	Selected    int
	Status      string
	HasExecuted bool
}

// QueryModel is the business logic for the query browser panel.
// It owns query construction, execution, result navigation, and
// selection routing. Renderers translate user input into method
// calls and draw the model's state.
type QueryModel struct {
	peerCtx *PeerContext

	// Query parameters (user-editable)
	TypeFilter string
	PathPrefix string

	// Results
	Matches     []types.QueryMatchData
	Total       uint64
	HasMore     bool
	Cursor      string
	Selected    int
	LastError   string
	HasExecuted bool

	// Selection slot binding (set via BindSelection). Query browser
	// only writes the selection (no read-back), so no subscription is
	// needed — just the bound state + screen index.
	selState  *WorkspaceState
	selScreen int
}

// BindSelection wires the model to a screen's selection slot. Nil
// state is a no-op for publish (unit tests / no presentation context).
func (m *QueryModel) BindSelection(state *WorkspaceState, screenIdx int) {
	m.selState = state
	m.selScreen = screenIdx
}

// NewQueryModel creates a query browser model.
func NewQueryModel(peerCtx *PeerContext) *QueryModel {
	return &QueryModel{
		peerCtx: peerCtx,
	}
}

// Execute runs the current query and populates results.
func (m *QueryModel) Execute() {
	m.LastError = ""
	m.HasExecuted = true

	typeFilter := m.TypeFilter
	if typeFilter == "" {
		typeFilter = "*"
	}

	expr := types.QueryExpressionData{
		TypeFilter: typeFilter,
		PathPrefix: m.PathPrefix,
	}
	limit := uint64(100)
	expr.Limit = &limit

	result, err := m.peerCtx.Executor().Query(expr)
	if err != nil {
		m.LastError = fmt.Sprintf("query: %s", err)
		m.Matches = nil
		m.Total = 0
		m.HasMore = false
		m.Cursor = ""
		return
	}

	m.Matches = result.Matches
	m.Total = result.Total
	m.HasMore = result.HasMore
	m.Cursor = result.Cursor
	m.Selected = 0
}

// NextPage fetches the next page of results using the cursor.
func (m *QueryModel) NextPage() {
	if !m.HasMore || m.Cursor == "" {
		return
	}

	m.LastError = ""

	expr := types.QueryExpressionData{
		TypeFilter: m.TypeFilter,
		PathPrefix: m.PathPrefix,
		Cursor:     m.Cursor,
	}
	limit := uint64(100)
	expr.Limit = &limit

	result, err := m.peerCtx.Executor().Query(expr)
	if err != nil {
		m.LastError = fmt.Sprintf("query next: %s", err)
		return
	}

	m.Matches = result.Matches
	m.Total = result.Total
	m.HasMore = result.HasMore
	m.Cursor = result.Cursor
	m.Selected = 0
}

// SelectMatch sets the selected result index.
func (m *QueryModel) SelectMatch(i int) {
	if i < 0 || i >= len(m.Matches) {
		return
	}
	m.Selected = i
}

// SelectNext moves selection down.
func (m *QueryModel) SelectNext() {
	if m.Selected < len(m.Matches)-1 {
		m.Selected++
	}
}

// SelectPrev moves selection up.
func (m *QueryModel) SelectPrev() {
	if m.Selected > 0 {
		m.Selected--
	}
}

// SelectedMatch returns the currently selected match, or nil.
func (m *QueryModel) SelectedMatch() *types.QueryMatchData {
	if m.Selected < 0 || m.Selected >= len(m.Matches) {
		return nil
	}
	return &m.Matches[m.Selected]
}

// PublishSelection writes the selected match's path to the screen
// selection slot (the value-of-truth in the tree).
func (m *QueryModel) PublishSelection() {
	match := m.SelectedMatch()
	if match == nil || m.selState == nil {
		return
	}
	m.selState.SaveSelection(m.selScreen, Selection{Path: match.Path, Type: "entity"})
}

// PeerCtx returns the peer context for cloning.
func (m *QueryModel) PeerCtx() *PeerContext { return m.peerCtx }

// Render produces the renderer-neutral query output snapshot.
func (m *QueryModel) Render() QueryOutput {
	return QueryOutput{
		TypeFilter:  m.TypeFilter,
		PathPrefix:  m.PathPrefix,
		Matches:     m.Matches,
		Total:       m.Total,
		HasMore:     m.HasMore,
		Selected:    m.Selected,
		Status:      m.StatusLine(),
		HasExecuted: m.HasExecuted,
	}
}

// StatusLine returns a summary string for display.
func (m *QueryModel) StatusLine() string {
	if m.LastError != "" {
		return "error: " + m.LastError
	}
	if !m.HasExecuted {
		return "Enter to query (type/path filters optional)"
	}
	if len(m.Matches) == 0 {
		return "no matches"
	}
	s := fmt.Sprintf("%d matches", m.Total)
	if m.HasMore {
		s += " (more available)"
	}
	return s
}
