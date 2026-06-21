package workbench

import "testing"

func TestFormatHexDump(t *testing.T) {
	data := make([]byte, 48)
	lines := FormatHexDump(data)
	// 48 bytes = 96 hex chars -> 2 lines (64 + 32).
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if len(lines[0]) != 64 {
		t.Errorf("first line len = %d, want 64", len(lines[0]))
	}
}

func TestFormatEntitySummary(t *testing.T) {
	got := FormatEntitySummary("workspace/x", "app/state/setting", 42)
	want := "workspace/x  app/state/setting  42 bytes"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
