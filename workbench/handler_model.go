package workbench

import (
	"encoding/hex"
	"fmt"
	"sync"

	"go.entitychurch.org/entity-core-go/core/types"
)

// HandlerBrowserPrefix is the canonical prefix where handler-interface
// entities are registered. Used as the HandlerBrowserModel subscription
// prefix.
const HandlerBrowserPrefix = "system/handler/"

var _ Model[HandlerBrowserOutput] = (*HandlerBrowserModel)(nil)

// HandlerBrowserOutput is the renderer-neutral output of the handler
// browser model. It exposes the discovered handlers, selection
// state, the resolved selected-handler info, and the execute output
// log so renderers can draw without reaching into the model.
type HandlerBrowserOutput struct {
	Handlers        []HandlerInfo
	SelectedHandler int
	SelectedOp      int
	Selected        *HandlerInfo
	SelectedOpName  string
	SpecLine        string
	Output          []OutputLine
}

// HandlerBrowserModel is the business logic for the handler browser /
// execute console. It owns handler discovery, selection, execution,
// and output. Renderers translate user input into method calls and
// draw the model's state.
//
// The model subscribes to the system/handler/ prefix and only
// rediscovers when an event arrives — no full-tree scan per refresh.
type HandlerBrowserModel struct {
	peerCtx  *PeerContext
	dispatch DispatchFunc

	Handlers        []HandlerInfo
	SelectedHandler int
	SelectedOp      int
	Output          []OutputLine

	mu     sync.Mutex
	dirty  bool // set when a system/handler/* event arrives
	cancel func()
}

// NewHandlerBrowserModel creates a handler browser and performs
// initial handler discovery.
//
// The model subscribes to the system/handler/ prefix on the underlying
// store; cancel via Close. The peerCtx still carries Executor +
// Resolve(); handler discovery needs Resolve to decode entity bodies.
func NewHandlerBrowserModel(peerCtx *PeerContext, dispatch DispatchFunc) *HandlerBrowserModel {
	m := &HandlerBrowserModel{
		peerCtx:  peerCtx,
		dispatch: dispatch,
		dirty:    true,
	}
	if peerCtx != nil && peerCtx.Store() != nil {
		m.cancel = peerCtx.Store().OnPrefixChange(HandlerBrowserPrefix, m.onEvent)
	}
	m.discoverHandlers()
	return m
}

// Close cancels the subscription. Idempotent.
func (m *HandlerBrowserModel) Close() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
}

// onEvent marks the model dirty on every system/handler/* tree change.
// Discovery is deferred to Refresh so we don't decode entities on the
// SDK goroutine — keeps the handler fast.
func (m *HandlerBrowserModel) onEvent(_ ChangeEvent) {
	m.mu.Lock()
	m.dirty = true
	m.mu.Unlock()
}

// Refresh rediscovers handlers if the subscription has signaled a
// change. Returns true if the handler list was updated.
func (m *HandlerBrowserModel) Refresh() bool {
	m.mu.Lock()
	if !m.dirty {
		m.mu.Unlock()
		return false
	}
	m.dirty = false
	m.mu.Unlock()

	prev := m.SelectedHandler
	prevOp := m.SelectedOp
	m.discoverHandlers()
	m.SelectedHandler = prev
	m.SelectedOp = prevOp
	if m.SelectedHandler >= len(m.Handlers) {
		m.SelectedHandler = 0
	}
	return true
}

// SelectHandler sets the selected handler index and resets op selection.
func (m *HandlerBrowserModel) SelectHandler(i int) {
	if i < 0 || i >= len(m.Handlers) {
		return
	}
	m.SelectedHandler = i
	m.SelectedOp = 0
}

// SelectOperation sets the selected operation index.
func (m *HandlerBrowserModel) SelectOperation(i int) {
	h := m.SelectedHandlerInfo()
	if h == nil {
		return
	}
	if i < 0 || i >= len(h.Operations) {
		return
	}
	m.SelectedOp = i
}

// SelectHandlerPrev moves handler selection up by one.
func (m *HandlerBrowserModel) SelectHandlerPrev() {
	if m.SelectedHandler > 0 {
		m.SelectedHandler--
		m.SelectedOp = 0
	}
}

// SelectHandlerNext moves handler selection down by one.
func (m *HandlerBrowserModel) SelectHandlerNext() {
	if m.SelectedHandler < len(m.Handlers)-1 {
		m.SelectedHandler++
		m.SelectedOp = 0
	}
}

// SelectOpPrev moves operation selection left by one.
func (m *HandlerBrowserModel) SelectOpPrev() {
	if m.SelectedOp > 0 {
		m.SelectedOp--
	}
}

// SelectOpNext moves operation selection right by one.
func (m *HandlerBrowserModel) SelectOpNext() {
	h := m.SelectedHandlerInfo()
	if h == nil {
		return
	}
	if m.SelectedOp < len(h.Operations)-1 {
		m.SelectedOp++
	}
}

// SelectedHandlerInfo returns the currently selected handler, or nil.
func (m *HandlerBrowserModel) SelectedHandlerInfo() *HandlerInfo {
	if m.SelectedHandler < 0 || m.SelectedHandler >= len(m.Handlers) {
		return nil
	}
	return &m.Handlers[m.SelectedHandler]
}

// SelectedOpName returns the name of the selected operation, or "".
func (m *HandlerBrowserModel) SelectedOpName() string {
	h := m.SelectedHandlerInfo()
	if h == nil || m.SelectedOp < 0 || m.SelectedOp >= len(h.Operations) {
		return ""
	}
	return h.Operations[m.SelectedOp]
}

// SelectedOpSpec returns the spec for the selected operation.
func (m *HandlerBrowserModel) SelectedOpSpec() (types.HandlerOperationSpec, bool) {
	h := m.SelectedHandlerInfo()
	op := m.SelectedOpName()
	if h == nil || op == "" {
		return types.HandlerOperationSpec{}, false
	}
	spec, ok := h.Specs[op]
	return spec, ok
}

// SpecLine returns a formatted spec summary for the selected operation.
func (m *HandlerBrowserModel) SpecLine() string {
	h := m.SelectedHandlerInfo()
	op := m.SelectedOpName()
	if h == nil || op == "" {
		return ""
	}
	spec := h.Specs[op]
	line := h.Pattern + " " + op
	if spec.InputType != "" {
		line += "  in:" + spec.InputType
	}
	if spec.OutputType != "" {
		line += "  out:" + spec.OutputType
	}
	return line
}

// Execute runs the selected handler operation and appends output.
func (m *HandlerBrowserModel) Execute() {
	h := m.SelectedHandlerInfo()
	if h == nil {
		return
	}
	op := m.SelectedOpName()
	if op == "" {
		return
	}

	m.appendLine(fmt.Sprintf("> %s %s", h.Pattern, op), KindPath)

	resp, err := m.dispatch(h.Pattern, op)
	if err != nil {
		m.appendLine("ERROR "+err.Error(), KindError)
		return
	}

	m.appendLine(fmt.Sprintf("%d %s", resp.Status, resp.Type), KindString)

	if len(resp.Data) > 0 {
		if decoded, ok := DecodeEntityData(resp.Data); ok {
			for _, line := range FormatCBOR(decoded) {
				m.Output = append(m.Output, FlattenFormattedLine(line, 1))
			}
		} else {
			m.appendLine(hex.EncodeToString(resp.Data), KindBytes)
		}
	}
	if len(resp.Included) > 0 {
		m.appendLine(fmt.Sprintf("+%d included", len(resp.Included)), KindNull)
	}
	m.appendLine("", KindNull)
}

// ExecuteCustom runs a custom handler operation (with explicit URI, op, resource).
func (m *HandlerBrowserModel) ExecuteCustom(uri, op, resource string) {
	if uri == "" || op == "" {
		m.appendLine("need URI and operation", KindError)
		return
	}

	path := uri
	if resource != "" {
		path = resource
	}

	m.appendLine(fmt.Sprintf("> %s %s", uri, op), KindPath)

	resp, err := m.dispatch(path, op)
	if err != nil {
		m.appendLine("ERROR "+err.Error(), KindError)
		return
	}

	m.appendLine(fmt.Sprintf("%d %s", resp.Status, resp.Type), KindString)

	if len(resp.Data) > 0 {
		if decoded, ok := DecodeEntityData(resp.Data); ok {
			for _, line := range FormatCBOR(decoded) {
				m.Output = append(m.Output, FlattenFormattedLine(line, 1))
			}
		} else {
			m.appendLine(hex.EncodeToString(resp.Data), KindBytes)
		}
	}
	if len(resp.Included) > 0 {
		m.appendLine(fmt.Sprintf("+%d included", len(resp.Included)), KindNull)
	}
}

// Render produces the renderer-neutral handler browser output.
func (m *HandlerBrowserModel) Render() HandlerBrowserOutput {
	return HandlerBrowserOutput{
		Handlers:        m.Handlers,
		SelectedHandler: m.SelectedHandler,
		SelectedOp:      m.SelectedOp,
		Selected:        m.SelectedHandlerInfo(),
		SelectedOpName:  m.SelectedOpName(),
		SpecLine:        m.SpecLine(),
		Output:          m.Output,
	}
}

// PeerCtx returns the underlying PeerContext.
func (m *HandlerBrowserModel) PeerCtx() *PeerContext { return m.peerCtx }

// DispatchFn returns the underlying DispatchFunc.
func (m *HandlerBrowserModel) DispatchFn() DispatchFunc { return m.dispatch }

func (m *HandlerBrowserModel) appendLine(text string, kind ValueKind) {
	m.Output = append(m.Output, OutputLine{Text: text, Kind: kind})
}

func (m *HandlerBrowserModel) discoverHandlers() {
	m.Handlers = DiscoverHandlers(m.peerCtx)
	if m.SelectedHandler >= len(m.Handlers) {
		m.SelectedHandler = 0
	}
	m.SelectedOp = 0
}
