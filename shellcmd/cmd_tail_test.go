package shellcmd_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/shellcmd"
)

// TestShell_TailSingleEvent verifies the default tail shape: `tail
// <path>` waits for one event and returns it as a LinesResult.
func TestShell_TailSingleEvent(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	sh := shellcmd.NewShell(ap, "local", "")
	reg := shellcmd.Default()

	go func() {
		time.Sleep(50 * time.Millisecond)
		_, _ = ap.Put("workspace/note", "test/v", "v1")
	}()

	res, err := reg.Dispatch(sh, "tail", []string{"workspace/note", "-timeout", "2s"})
	if err != nil {
		t.Fatalf("tail: %v", err)
	}
	if res.Kind != shellcmd.KindLines {
		t.Fatalf("tail: expected KindLines, got %v (msg=%q)", res.Kind, res.Message)
	}
	if len(res.Lines) != 1 {
		t.Fatalf("tail: expected 1 line, got %d (%v)", len(res.Lines), res.Lines)
	}
	if !strings.Contains(res.Lines[0], "put") || !strings.Contains(res.Lines[0], "workspace/note") {
		t.Errorf("tail line should name put + path; got %q", res.Lines[0])
	}
}

// TestShell_TailMultipleEvents verifies `-n N` collects N events.
func TestShell_TailMultipleEvents(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	sh := shellcmd.NewShell(ap, "local", "")
	reg := shellcmd.Default()

	go func() {
		time.Sleep(50 * time.Millisecond)
		for i := 0; i < 3; i++ {
			_, _ = ap.Put("workspace/note", "test/v", fmt.Sprintf("v%d", i))
			time.Sleep(10 * time.Millisecond)
		}
	}()

	res, err := reg.Dispatch(sh, "tail", []string{"workspace/note", "-n", "3", "-timeout", "2s"})
	if err != nil {
		t.Fatalf("tail: %v", err)
	}
	if res.Kind != shellcmd.KindLines {
		t.Fatalf("tail: expected KindLines, got %v (msg=%q)", res.Kind, res.Message)
	}
	if len(res.Lines) != 3 {
		t.Fatalf("tail: expected 3 lines, got %d (%v)", len(res.Lines), res.Lines)
	}
}

// TestShell_TailPrefix verifies trailing-`*` prefix subscription
// catches events at descendant paths.
func TestShell_TailPrefix(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	sh := shellcmd.NewShell(ap, "local", "")
	reg := shellcmd.Default()

	go func() {
		time.Sleep(50 * time.Millisecond)
		_, _ = ap.Put("workspace/notes/a", "test/v", "a")
		_, _ = ap.Put("workspace/notes/b", "test/v", "b")
	}()

	res, err := reg.Dispatch(sh, "tail", []string{"workspace/notes/*", "-n", "2", "-timeout", "2s"})
	if err != nil {
		t.Fatalf("tail: %v", err)
	}
	if res.Kind != shellcmd.KindLines {
		t.Fatalf("tail: expected KindLines, got %v (msg=%q)", res.Kind, res.Message)
	}
	if len(res.Lines) != 2 {
		t.Fatalf("tail: expected 2 lines, got %d (%v)", len(res.Lines), res.Lines)
	}
}

// TestShell_TailTimeoutNoEvents verifies the no-event path returns a
// MessageResult, not an error.
func TestShell_TailTimeoutNoEvents(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	sh := shellcmd.NewShell(ap, "local", "")
	reg := shellcmd.Default()

	res, err := reg.Dispatch(sh, "tail", []string{"workspace/quiet", "-timeout", "100ms"})
	if err != nil {
		t.Fatalf("tail (no events): %v", err)
	}
	if res.Kind != shellcmd.KindMessage {
		t.Fatalf("tail: expected KindMessage for no-event timeout, got %v", res.Kind)
	}
	if !strings.Contains(res.Message, "no events") {
		t.Errorf("expected 'no events' in message, got %q", res.Message)
	}
}

// TestShell_TailUsage verifies argument-validation errors return
// usage-shaped messages.
func TestShell_TailUsage(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	sh := shellcmd.NewShell(ap, "local", "")
	reg := shellcmd.Default()

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no args", []string{}, "usage"},
		{"bad -n", []string{"path", "-n", "zero"}, "positive integer"},
		{"bad -timeout", []string{"path", "-timeout", "junk"}, "-timeout"},
		{"unknown flag", []string{"path", "-q"}, "unknown flag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := reg.Dispatch(sh, "tail", tc.args)
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}
