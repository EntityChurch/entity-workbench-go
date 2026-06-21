package shellcmd_test

import (
	"strings"
	"testing"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/shellcmd"
)

// TestShell_QueryBasic verifies `query <prefix>` returns matching paths
// + types and reports the total in the header.
func TestShell_QueryBasic(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	// Seed three entities under workspace/.
	for i, p := range []string{"workspace/a", "workspace/b", "workspace/c"} {
		if _, err := ap.Put(p, "test/v", string(rune('0'+i))); err != nil {
			t.Fatalf("seed %s: %v", p, err)
		}
	}

	sh := shellcmd.NewShell(ap, "local", "")
	reg := shellcmd.Default()

	res, err := reg.Dispatch(sh, "query", []string{"workspace"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if res.Kind != shellcmd.KindLines {
		t.Fatalf("query: expected KindLines, got %v (msg=%q)", res.Kind, res.Message)
	}
	// Header + blank + 3 match lines.
	if len(res.Lines) < 5 {
		t.Fatalf("query: expected ≥5 lines (header+blank+3 matches), got %d (%v)", len(res.Lines), res.Lines)
	}
	if !strings.Contains(res.Lines[0], "3 match") {
		t.Errorf("header should report 3 matches, got %q", res.Lines[0])
	}
	matchSection := strings.Join(res.Lines[2:], "\n")
	for _, p := range []string{"workspace/a", "workspace/b", "workspace/c"} {
		if !strings.Contains(matchSection, p) {
			t.Errorf("expected match for %q in output:\n%s", p, matchSection)
		}
	}
}

// TestShell_QueryWithTypeFilter narrows to a single type.
func TestShell_QueryWithTypeFilter(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	_, _ = ap.Put("workspace/a", "test/note", "x")
	_, _ = ap.Put("workspace/b", "test/draft", "y")
	_, _ = ap.Put("workspace/c", "test/note", "z")

	sh := shellcmd.NewShell(ap, "local", "")
	reg := shellcmd.Default()

	res, err := reg.Dispatch(sh, "query", []string{"workspace", "-type", "test/note"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if res.Kind != shellcmd.KindLines {
		t.Fatalf("query: expected KindLines, got %v (msg=%q)", res.Kind, res.Message)
	}
	if !strings.Contains(res.Lines[0], "2 match") {
		t.Errorf("expected 2 matches for type=test/note, got header %q (lines: %v)", res.Lines[0], res.Lines)
	}
}

// TestShell_QueryEmptyPrefix returns the no-matches message rather than
// an error.
func TestShell_QueryNoMatches(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	sh := shellcmd.NewShell(ap, "local", "")
	reg := shellcmd.Default()

	res, err := reg.Dispatch(sh, "query", []string{"nothing"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if res.Kind != shellcmd.KindMessage {
		t.Fatalf("query: expected KindMessage for no-matches, got %v", res.Kind)
	}
	if !strings.Contains(res.Message, "no matches") {
		t.Errorf("expected 'no matches' in message, got %q", res.Message)
	}
}

// TestShell_CountBasic verifies `count <prefix>` returns the cardinality
// as a message.
func TestShell_CountBasic(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	for i, p := range []string{"workspace/a", "workspace/b", "workspace/c", "workspace/d"} {
		if _, err := ap.Put(p, "test/v", string(rune('0'+i))); err != nil {
			t.Fatalf("seed %s: %v", p, err)
		}
	}

	sh := shellcmd.NewShell(ap, "local", "")
	reg := shellcmd.Default()

	res, err := reg.Dispatch(sh, "count", []string{"workspace"})
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if res.Kind != shellcmd.KindMessage {
		t.Fatalf("count: expected KindMessage, got %v", res.Kind)
	}
	if res.Message != "4" {
		t.Errorf("count: expected 4, got %q", res.Message)
	}
}

// TestShell_CountWithTypeFilter narrows count by type.
func TestShell_CountWithTypeFilter(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	_, _ = ap.Put("workspace/a", "test/note", "x")
	_, _ = ap.Put("workspace/b", "test/draft", "y")
	_, _ = ap.Put("workspace/c", "test/note", "z")

	sh := shellcmd.NewShell(ap, "local", "")
	reg := shellcmd.Default()

	res, err := reg.Dispatch(sh, "count", []string{"workspace", "-type", "test/note"})
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if res.Message != "2" {
		t.Errorf("count: expected 2, got %q", res.Message)
	}
}

// TestShell_QueryCountUsage verifies argument-validation errors.
func TestShell_QueryCountUsage(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	sh := shellcmd.NewShell(ap, "local", "")
	reg := shellcmd.Default()

	for _, tc := range []struct {
		verb string
		args []string
		want string
	}{
		{"query", []string{}, "usage"},
		{"query", []string{"prefix", "-type"}, "-type"},
		{"query", []string{"prefix", "-field", "novalue"}, "-field expects F=V"},
		{"query", []string{"prefix", "-limit", "junk"}, "positive integer"},
		{"query", []string{"prefix", "-q"}, "unknown flag"},
		{"count", []string{}, "usage"},
	} {
		t.Run(tc.verb+"_"+tc.want, func(t *testing.T) {
			_, err := reg.Dispatch(sh, tc.verb, tc.args)
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}
