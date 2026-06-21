package workbench

import (
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/types"
)

func TestHandlerBrowserModel_Empty(t *testing.T) {
	pc, _, _ := testPeerContext(t)
	_ = pc // cache removed

	m := NewHandlerBrowserModel(pc, nil)
	if len(m.Handlers) != 0 {
		t.Fatalf("got %d handlers, want 0", len(m.Handlers))
	}
	if m.SelectedHandlerInfo() != nil {
		t.Error("expected nil handler info")
	}
	if m.SelectedOpName() != "" {
		t.Error("expected empty op name")
	}
}

func TestHandlerBrowserModel_Discovery(t *testing.T) {
	pc, s, li := testPeerContext(t)
	seedHandlerInterface(t, s, li, "system/tree", "Tree Handler", map[string]types.HandlerOperationSpec{
		"get":  {InputType: "system/tree/get-request"},
		"list": {OutputType: "system/tree/listing"},
	})
	seedHandlerInterface(t, s, li, "data/files", "File Handler", map[string]types.HandlerOperationSpec{
		"read": {OutputType: "data/file"},
	})
	_ = pc // cache removed

	m := NewHandlerBrowserModel(pc, nil)
	if len(m.Handlers) != 2 {
		t.Fatalf("got %d handlers, want 2", len(m.Handlers))
	}
	// Sorted by pattern
	if m.Handlers[0].Pattern != "data/files" {
		t.Errorf("first = %q, want data/files", m.Handlers[0].Pattern)
	}
}

func TestHandlerBrowserModel_Selection(t *testing.T) {
	pc, s, li := testPeerContext(t)
	seedHandlerInterface(t, s, li, "system/tree", "Tree", map[string]types.HandlerOperationSpec{
		"get":  {},
		"list": {},
		"put":  {},
	})
	seedHandlerInterface(t, s, li, "data/files", "Files", map[string]types.HandlerOperationSpec{
		"read":  {},
		"write": {},
	})
	_ = pc // cache removed

	m := NewHandlerBrowserModel(pc, nil)

	// Initial selection
	if m.SelectedHandler != 0 {
		t.Errorf("initial handler = %d, want 0", m.SelectedHandler)
	}

	// Navigate handlers
	m.SelectHandlerNext()
	if m.SelectedHandler != 1 {
		t.Errorf("after next = %d, want 1", m.SelectedHandler)
	}
	m.SelectHandlerNext() // at end, should not move
	if m.SelectedHandler != 1 {
		t.Errorf("past end = %d, want 1", m.SelectedHandler)
	}
	m.SelectHandlerPrev()
	if m.SelectedHandler != 0 {
		t.Errorf("after prev = %d, want 0", m.SelectedHandler)
	}

	// Navigate operations
	m.SelectHandler(1) // system/tree (sorted: data/files=0, system/tree=1)
	h := m.SelectedHandlerInfo()
	if h == nil || h.Pattern != "system/tree" {
		t.Fatalf("expected system/tree, got %v", h)
	}
	if m.SelectedOp != 0 {
		t.Errorf("initial op = %d, want 0", m.SelectedOp)
	}
	m.SelectOpNext()
	if m.SelectedOp != 1 {
		t.Errorf("op after next = %d, want 1", m.SelectedOp)
	}
	m.SelectOpPrev()
	if m.SelectedOp != 0 {
		t.Errorf("op after prev = %d, want 0", m.SelectedOp)
	}
}

func TestHandlerBrowserModel_SpecLine(t *testing.T) {
	pc, s, li := testPeerContext(t)
	seedHandlerInterface(t, s, li, "system/tree", "Tree", map[string]types.HandlerOperationSpec{
		"get": {InputType: "system/tree/get-request", OutputType: ""},
	})
	_ = pc // cache removed

	m := NewHandlerBrowserModel(pc, nil)
	spec := m.SpecLine()
	if !strings.Contains(spec, "system/tree") || !strings.Contains(spec, "get") {
		t.Errorf("spec = %q, expected handler pattern and op", spec)
	}
	if !strings.Contains(spec, "in:system/tree/get-request") {
		t.Errorf("spec = %q, expected input type", spec)
	}
}

func TestHandlerBrowserModel_Refresh(t *testing.T) {
	pc, s, li := testPeerContext(t)
	seedHandlerInterface(t, s, li, "system/tree", "Tree", map[string]types.HandlerOperationSpec{
		"list": {},
	})
	_ = pc // cache removed

	m := NewHandlerBrowserModel(pc, nil)
	if len(m.Handlers) != 1 {
		t.Fatalf("initial: got %d, want 1", len(m.Handlers))
	}

	// Add another handler
	seedHandlerInterface(t, s, li, "data/files", "Files", map[string]types.HandlerOperationSpec{
		"read": {},
	})
	_ = pc // cache removed

	changed := m.Refresh()
	if !changed {
		t.Error("expected refresh to detect changes")
	}
	if len(m.Handlers) != 2 {
		t.Fatalf("after refresh: got %d, want 2", len(m.Handlers))
	}

	// No change
	changed = m.Refresh()
	if changed {
		t.Error("expected no change on second refresh")
	}
}

func TestHandlerBrowserModel_RefreshPreservesSelection(t *testing.T) {
	pc, s, li := testPeerContext(t)
	seedHandlerInterface(t, s, li, "a/handler", "A", map[string]types.HandlerOperationSpec{"op1": {}})
	seedHandlerInterface(t, s, li, "b/handler", "B", map[string]types.HandlerOperationSpec{"op1": {}, "op2": {}})
	_ = pc // cache removed

	m := NewHandlerBrowserModel(pc, nil)
	m.SelectHandler(1) // b/handler
	m.SelectOpNext()   // op2

	// Add handler, refresh
	seedHandlerInterface(t, s, li, "c/handler", "C", map[string]types.HandlerOperationSpec{"op1": {}})
	_ = pc // cache removed
	m.Refresh()

	// Selection should be preserved
	if m.SelectedHandler != 1 {
		t.Errorf("handler = %d, want 1", m.SelectedHandler)
	}
	if m.SelectedOp != 1 {
		t.Errorf("op = %d, want 1", m.SelectedOp)
	}
}
