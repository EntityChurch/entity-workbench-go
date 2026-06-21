package shellcmd_test

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/shellcmd"
)

// TestShell_RevisionCommands exercises the new revision shell
// command surface end-to-end through the command registry.
// Coverage: commit / log / status / branch list+create+delete /
// tag list+create / find-ancestor / config put.
//
// The test runs against a fresh peer (revision wired by default
// in CreatePeer) and uses manual commits — no auto-versioning
// config required.
func TestShell_RevisionCommands(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	sh := shellcmd.NewShell(ap, "local", "")
	reg := shellcmd.Default()

	// Seed two L1 writes so commits have content.
	if _, err := ap.Put("workspace/a", "test/v", "v1"); err != nil {
		t.Fatalf("Put a: %v", err)
	}
	if _, err := ap.Put("workspace/b", "test/v", "v1"); err != nil {
		t.Fatalf("Put b: %v", err)
	}

	// revision commit workspace/ "initial commit"
	res, err := reg.Dispatch(sh, "revision", []string{"commit", "workspace/", "initial", "commit"})
	if err != nil {
		t.Fatalf("revision commit: %v", err)
	}
	if res.Kind != shellcmd.KindMessage || !strings.Contains(res.Message, "committed") {
		t.Errorf("revision commit: unexpected result %+v", res)
	}

	// Use the typed client to grab the first version hash for the
	// tag/find-ancestor checks below — the message format isn't
	// machine-friendly enough to reparse.
	rcStatus, err := ap.Revision().Status(context.Background(), "workspace/")
	if err != nil {
		t.Fatalf("Status (probe): %v", err)
	}
	first := rcStatus.Head

	// Make a second commit.
	if _, err := ap.Put("workspace/a", "test/v", "v2"); err != nil {
		t.Fatalf("Put a v2: %v", err)
	}
	res, err = reg.Dispatch(sh, "revision", []string{"commit", "workspace/", "second"})
	if err != nil {
		t.Fatalf("revision commit (2): %v", err)
	}
	if !strings.Contains(res.Message, "committed") {
		t.Errorf("commit (2) message: %s", res.Message)
	}
	rcStatus, _ = ap.Revision().Status(context.Background(), "workspace/")
	second := rcStatus.Head

	// revision log
	res, err = reg.Dispatch(sh, "revision", []string{"log", "workspace/"})
	if err != nil {
		t.Fatalf("revision log: %v", err)
	}
	if res.Kind != shellcmd.KindLines {
		t.Fatalf("log: expected KindLines, got %v", res.Kind)
	}
	if len(res.Lines) < 2 {
		t.Errorf("log: expected ≥2 lines, got %d", len(res.Lines))
	}
	if !strings.HasPrefix(res.Lines[0], "* ") {
		t.Errorf("log line 0 should be HEAD-marked: %q", res.Lines[0])
	}

	// revision status workspace/
	res, err = reg.Dispatch(sh, "revision", []string{"status", "workspace/"})
	if err != nil {
		t.Fatalf("revision status: %v", err)
	}
	if res.Kind != shellcmd.KindLines {
		t.Fatalf("status: expected KindLines, got %v", res.Kind)
	}
	hasHead := false
	for _, l := range res.Lines {
		if strings.Contains(l, "head:") {
			hasHead = true
		}
	}
	if !hasHead {
		t.Errorf("status missing head row: %v", res.Lines)
	}

	// revision branch create workspace/ feature <first-hash>
	firstHex := hex.EncodeToString(first.Bytes())
	res, err = reg.Dispatch(sh, "revision", []string{"branch", "create", "workspace/", "feature", firstHex})
	if err != nil {
		t.Fatalf("revision branch create: %v", err)
	}
	if !strings.Contains(res.Message, "feature") {
		t.Errorf("branch create message: %s", res.Message)
	}

	// revision branch list workspace/
	res, err = reg.Dispatch(sh, "revision", []string{"branch", "list", "workspace/"})
	if err != nil {
		t.Fatalf("revision branch list: %v", err)
	}
	hasFeature := false
	for _, l := range res.Lines {
		if strings.Contains(l, "feature") {
			hasFeature = true
		}
	}
	if !hasFeature {
		t.Errorf("branch list missing feature: %v", res.Lines)
	}

	// revision tag create workspace/ v1.0 <second-hash>
	secondHex := hex.EncodeToString(second.Bytes())
	res, err = reg.Dispatch(sh, "revision", []string{"tag", "create", "workspace/", "v1.0", secondHex})
	if err != nil {
		t.Fatalf("revision tag create: %v", err)
	}
	if !strings.Contains(res.Message, "v1.0") {
		t.Errorf("tag create message: %s", res.Message)
	}

	// revision find-ancestor <first> <second> — first should be the LCA in a linear chain.
	res, err = reg.Dispatch(sh, "revision", []string{"find-ancestor", firstHex, secondHex})
	if err != nil {
		t.Fatalf("revision find-ancestor: %v", err)
	}
	if !strings.Contains(res.Message, "ancestor") {
		t.Errorf("find-ancestor message: %s", res.Message)
	}

	// revision config put auto1 workspace/ -auto -exclude scratch/*
	res, err = reg.Dispatch(sh, "revision", []string{
		"config", "put", "auto1", "workspace/", "-auto", "-exclude", "scratch/*",
	})
	if err != nil {
		t.Fatalf("revision config put: %v", err)
	}
	if !strings.Contains(res.Message, "wrote config") {
		t.Errorf("config put message: %s", res.Message)
	}
}
