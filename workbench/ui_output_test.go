package workbench

import "testing"

// Only LevelName/ParseLevelName are pinned here: they are the durable
// contract behind persisted log-display-level settings (saved to and
// reloaded from the tree via WorkspaceState.SaveWindowSetting /
// ParseLevelName). The Flatten* helpers are deliberately NOT tested —
// they are exact-string presentation glue with no stable contract, and
// asserting their formatting would be brittle without catching real
// bugs.

func TestLevelNameRoundTrip(t *testing.T) {
	for _, lvl := range []LogLevel{LogVerbose, LogDebug, LogInfo} {
		if got := ParseLevelName(LevelName(lvl)); got != lvl {
			t.Fatalf("round-trip failed: %v → %q → %v", lvl, LevelName(lvl), got)
		}
	}
}

func TestLevelNameMapping(t *testing.T) {
	if LevelName(LogVerbose) != "verbose" || LevelName(LogDebug) != "debug" || LevelName(LogInfo) != "info" {
		t.Fatalf("LevelName mapping: verbose=%q debug=%q info=%q",
			LevelName(LogVerbose), LevelName(LogDebug), LevelName(LogInfo))
	}
	// Unknown names fall back to info (the safe default for a
	// persisted setting written by an older/newer build).
	if ParseLevelName("not-a-level") != LogInfo {
		t.Fatalf("ParseLevelName(unknown) = %v, want LogInfo", ParseLevelName("not-a-level"))
	}
}
