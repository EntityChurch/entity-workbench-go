package workbench

import "testing"

func TestLogFilterModel_FilteredEntries(t *testing.T) {
	el := NewEventLog(100)
	el.Append("info msg")
	el.Verbose("verbose msg")
	el.Debug("debug msg")

	m := NewLogFilterModel(el)

	// Default: show everything
	entries := m.FilteredEntries()
	if len(entries) != 3 {
		t.Fatalf("debug filter: got %d, want 3", len(entries))
	}

	// Verbose: show info + verbose
	m.DisplayLevel = LogVerbose
	entries = m.FilteredEntries()
	if len(entries) != 2 {
		t.Fatalf("verbose filter: got %d, want 2", len(entries))
	}

	// Info only
	m.DisplayLevel = LogInfo
	entries = m.FilteredEntries()
	if len(entries) != 1 {
		t.Fatalf("info filter: got %d, want 1", len(entries))
	}
	if entries[0].Message != "info msg" {
		t.Errorf("expected 'info msg', got %q", entries[0].Message)
	}
}

func TestLogFilterModel_CycleDisplayLevel(t *testing.T) {
	el := NewEventLog(100)
	m := NewLogFilterModel(el)

	if m.DisplayLevel != LogDebug {
		t.Fatalf("initial = %d, want LogDebug", m.DisplayLevel)
	}

	m.CycleDisplayLevel()
	if m.DisplayLevel != LogInfo {
		t.Errorf("after first cycle = %d, want LogInfo", m.DisplayLevel)
	}

	m.CycleDisplayLevel()
	if m.DisplayLevel != LogVerbose {
		t.Errorf("after second cycle = %d, want LogVerbose", m.DisplayLevel)
	}

	m.CycleDisplayLevel()
	if m.DisplayLevel != LogDebug {
		t.Errorf("after third cycle = %d, want LogDebug", m.DisplayLevel)
	}
}

func TestLogFilterModel_CycleCollectionLevel(t *testing.T) {
	el := NewEventLog(100)
	m := NewLogFilterModel(el)

	// Default collection level is LogDebug
	if el.Level() != LogDebug {
		t.Fatalf("initial = %d, want LogDebug", el.Level())
	}

	m.CycleCollectionLevel()
	if el.Level() != LogInfo {
		t.Errorf("after cycle = %d, want LogInfo", el.Level())
	}
}

func TestLogFilterModel_NewEntries(t *testing.T) {
	el := NewEventLog(100)
	m := NewLogFilterModel(el)

	el.Append("msg1")
	el.Append("msg2")

	entries := m.NewEntries()
	if len(entries) != 2 {
		t.Fatalf("first call: got %d, want 2", len(entries))
	}

	// No new entries
	entries = m.NewEntries()
	if len(entries) != 0 {
		t.Fatalf("second call: got %d, want 0", len(entries))
	}

	// Add more
	el.Append("msg3")
	entries = m.NewEntries()
	if len(entries) != 1 {
		t.Fatalf("third call: got %d, want 1", len(entries))
	}
	if entries[0].Message != "msg3" {
		t.Errorf("expected msg3, got %q", entries[0].Message)
	}
}

func TestLogFilterModel_NewEntriesFiltered(t *testing.T) {
	el := NewEventLog(100)
	m := NewLogFilterModel(el)
	m.DisplayLevel = LogInfo

	el.Append("info")
	el.Debug("debug")

	entries := m.NewEntries()
	if len(entries) != 1 {
		t.Fatalf("got %d, want 1 (only info)", len(entries))
	}
}

func TestLogFilterModel_ResetSequence(t *testing.T) {
	el := NewEventLog(100)
	m := NewLogFilterModel(el)

	el.Append("msg1")
	el.Append("msg2")
	m.NewEntries() // consume

	m.ResetSequence()
	entries := m.NewEntries()
	if len(entries) != 2 {
		t.Fatalf("after reset: got %d, want 2", len(entries))
	}
}

func TestLogFilterModel_StatePersistence(t *testing.T) {
	el := NewEventLog(100)
	ws, _ := testWorkspaceState(t)

	// Create model with state binding
	m := NewLogFilterModel(el)
	m.BindState(ws, 42)

	// Cycle display level — should persist
	m.CycleDisplayLevel() // debug → info
	if v := ws.ReadWindowSetting(42, "log-display-level"); v != "info" {
		t.Errorf("persisted display level = %q, want info", v)
	}

	// Cycle collection level — should persist
	m.CycleCollectionLevel() // debug → info
	if v := ws.ReadSetting("log-collection-level"); v != "info" {
		t.Errorf("persisted collection level = %q, want info", v)
	}

	// Create a new model that should restore from state
	m2 := NewLogFilterModel(el)
	m2.BindState(ws, 42)
	if m2.DisplayLevel != LogInfo {
		t.Errorf("restored display level = %d, want LogInfo", m2.DisplayLevel)
	}
	if el.Level() != LogInfo {
		t.Errorf("restored collection level = %d, want LogInfo", el.Level())
	}
}

func TestLogFilterModel_Title(t *testing.T) {
	el := NewEventLog(100)
	m := NewLogFilterModel(el)

	title := m.Title()
	if title != "Event Log [collect:debug display:debug]" {
		t.Errorf("title = %q", title)
	}

	m.DisplayLevel = LogInfo
	title = m.Title()
	if title != "Event Log [collect:debug display:info]" {
		t.Errorf("title = %q", title)
	}
}
