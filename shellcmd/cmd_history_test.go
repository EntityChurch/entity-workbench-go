package shellcmd_test

import (
	"strings"
	"testing"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/shellcmd"
)

// TestShell_HistoryCommands exercises the new history shell command
// surface end-to-end through the command registry: install a config,
// drive two writes at a tracked path, query the transition list,
// confirm rollback restores an earlier version.
func TestShell_HistoryCommands(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	sh := shellcmd.NewShell(ap, "local", "")
	reg := shellcmd.Default()

	// history config workspace/*
	res, err := reg.Dispatch(sh, "history", []string{"config", "workspace/*"})
	if err != nil {
		t.Fatalf("history config: %v", err)
	}
	if !strings.Contains(res.Message, "recording enabled") {
		t.Errorf("config result: %s", res.Message)
	}

	// Two writes at the tracked path.
	first, err := ap.Put("workspace/note", "test/v", "v1")
	if err != nil {
		t.Fatalf("first Put: %v", err)
	}
	second, err := ap.Put("workspace/note", "test/v", "v2")
	if err != nil {
		t.Fatalf("second Put: %v", err)
	}
	if first == second {
		t.Fatal("v1 and v2 produced the same hash")
	}

	// history query workspace/note → expect ≥2 transitions, newest-first.
	res, err = reg.Dispatch(sh, "history", []string{"query", "workspace/note"})
	if err != nil {
		t.Fatalf("history query: %v", err)
	}
	if res.Kind != shellcmd.KindLines {
		t.Fatalf("query: expected KindLines, got %v", res.Kind)
	}
	if len(res.Lines) < 2 {
		t.Fatalf("query: expected ≥2 lines, got %d (%v)", len(res.Lines), res.Lines)
	}
	if !strings.HasPrefix(res.Lines[0], "* ") {
		t.Errorf("first line should be HEAD-marked: %q", res.Lines[0])
	}

	// history rollback workspace/note <first-hex>.
	firstHex := hashToHex(first.Bytes())
	res, err = reg.Dispatch(sh, "history", []string{"rollback", "workspace/note", firstHex})
	if err != nil {
		t.Fatalf("history rollback: %v", err)
	}
	if !strings.Contains(res.Message, "rolled back") {
		t.Errorf("rollback message: %s", res.Message)
	}

	// Confirm the path now resolves to first via direct Get.
	got, ok, err := ap.Get("workspace/note")
	if err != nil {
		t.Fatalf("Get after rollback: %v", err)
	}
	if !ok || got.ContentHash != first {
		t.Errorf("after rollback path resolves to %s, want %s", got.ContentHash, first)
	}
}

// hashToHex is duplicated from cmd_role.go's shortHash sans the
// truncation, since we need the full hex form to round-trip back into
// the rollback command. Inlined here rather than exporting more
// surface area from the package.
func hashToHex(b []byte) string {
	const hexChars = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hexChars[v>>4]
		out[i*2+1] = hexChars[v&0x0f]
	}
	return string(out)
}
