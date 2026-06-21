package workbench

import "fmt"

var _ Model[LogOutput] = (*LogFilterModel)(nil)

// LogOutput is the renderer-neutral output of the log filter model.
// Entries is the filtered list (display-level applied). Title is the
// pre-formatted panel title. Renderers needing incremental updates
// can additionally call NewEntries on the model — Render gives the
// full snapshot.
type LogOutput struct {
	Title           string
	Entries         []LogEntry
	DisplayLevel    LogLevel
	CollectionLevel LogLevel
}

// LogFilterModel is the business logic for the log viewer panel.
// It owns display level filtering and provides filtered entry access.
// When WorkspaceState is configured, level changes auto-persist to
// the entity tree.
type LogFilterModel struct {
	eventLog     *EventLog
	DisplayLevel LogLevel
	lastSeq      uint64

	// Optional entity-backed persistence. Set via BindState.
	state    *WorkspaceState
	windowID uint32
}

// NewLogFilterModel creates a log filter that shows all levels.
func NewLogFilterModel(eventLog *EventLog) *LogFilterModel {
	return &LogFilterModel{
		eventLog:     eventLog,
		DisplayLevel: LogDebug,
	}
}

// BindState enables entity-backed persistence for this model.
// When bound, level changes are saved to the tree and the constructor
// restores previous state. Call after NewLogFilterModel.
func (m *LogFilterModel) BindState(state *WorkspaceState, windowID uint32) {
	m.state = state
	m.windowID = windowID
	m.restoreState()
}

// CycleDisplayLevel advances the display filter to the next level.
func (m *LogFilterModel) CycleDisplayLevel() {
	m.DisplayLevel = (m.DisplayLevel + 1) % 3
	m.saveDisplayLevel()
}

// CycleCollectionLevel advances the global collection level.
func (m *LogFilterModel) CycleCollectionLevel() {
	current := m.eventLog.Level()
	next := (current + 1) % 3
	m.eventLog.SetLevel(next)
	m.saveCollectionLevel()
}

// FilteredEntries returns all log entries that pass the display filter.
func (m *LogFilterModel) FilteredEntries() []LogEntry {
	entries := m.eventLog.Entries()
	var visible []LogEntry
	for _, e := range entries {
		if e.Level <= m.DisplayLevel {
			visible = append(visible, e)
		}
	}
	return visible
}

// NewEntries returns entries added since the last call to NewEntries,
// filtered by display level. Useful for incremental rendering.
func (m *LogFilterModel) NewEntries() []LogEntry {
	entries := m.eventLog.Entries()
	var result []LogEntry
	for _, e := range entries {
		if e.Seq <= m.lastSeq {
			continue
		}
		m.lastSeq = e.Seq
		if e.Level <= m.DisplayLevel {
			result = append(result, e)
		}
	}
	return result
}

// ResetSequence resets the last-seen sequence number, causing the next
// call to NewEntries to return all matching entries. Use after changing
// the display level to force a full re-render.
func (m *LogFilterModel) ResetSequence() {
	m.lastSeq = 0
}

// Title returns a formatted title string showing collection and display levels.
func (m *LogFilterModel) Title() string {
	collect := m.eventLog.LevelName()
	display := LevelName(m.DisplayLevel)
	return fmt.Sprintf("Event Log [collect:%s display:%s]", collect, display)
}

// DisplayLevelName returns the display name of the current display level.
func (m *LogFilterModel) DisplayLevelName() string {
	return LevelName(m.DisplayLevel)
}

// CollectionLevelName returns the display name of the current collection level.
func (m *LogFilterModel) CollectionLevelName() string {
	return m.eventLog.LevelName()
}

// EventLog returns the underlying event log.
func (m *LogFilterModel) EventLog() *EventLog {
	return m.eventLog
}

// Render produces the renderer-neutral log output snapshot.
func (m *LogFilterModel) Render() LogOutput {
	return LogOutput{
		Title:           m.Title(),
		Entries:         m.FilteredEntries(),
		DisplayLevel:    m.DisplayLevel,
		CollectionLevel: m.eventLog.Level(),
	}
}

// --- Entity-backed persistence ---

func (m *LogFilterModel) restoreState() {
	if m.state == nil {
		return
	}
	// Restore global collection level
	if v := m.state.ReadSetting("log-collection-level"); v != "" {
		m.eventLog.SetLevel(ParseLevelName(v))
	}
	// Restore per-window display level
	if v := m.state.ReadWindowSetting(m.windowID, "log-display-level"); v != "" {
		m.DisplayLevel = ParseLevelName(v)
	}
}

func (m *LogFilterModel) saveDisplayLevel() {
	if m.state == nil {
		return
	}
	m.state.SaveWindowSetting(m.windowID, "log-display-level", LevelName(m.DisplayLevel))
}

func (m *LogFilterModel) saveCollectionLevel() {
	if m.state == nil {
		return
	}
	m.state.SaveSetting("log-collection-level", m.eventLog.LevelName())
}
