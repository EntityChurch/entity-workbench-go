package shellcmd_test

import (
	"strings"
	"testing"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/shellcmd"
)

// TestShell_PeerLs verifies the compact list output.
func TestShell_PeerLs(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	sh := shellcmd.NewShell(ap, "alice", "")
	reg := shellcmd.Default()

	res, err := reg.Dispatch(sh, "peer", []string{"ls"})
	if err != nil {
		t.Fatalf("peer ls: %v", err)
	}
	if res.Kind != shellcmd.KindLines {
		t.Fatalf("peer ls: expected KindLines, got %v", res.Kind)
	}
	if len(res.Lines) < 2 {
		t.Fatalf("peer ls: expected header + 1 row, got %d (%v)", len(res.Lines), res.Lines)
	}
	if !strings.Contains(res.Lines[0], "ALIAS") {
		t.Errorf("header missing ALIAS column: %q", res.Lines[0])
	}
	if !strings.Contains(res.Lines[1], "alice *") {
		t.Errorf("local row should have * marker: %q", res.Lines[1])
	}
	if !strings.Contains(res.Lines[1], "(local)") {
		t.Errorf("local row should show (local) address: %q", res.Lines[1])
	}
}

// TestShell_PeerRename retags the alias on the local peer.
func TestShell_PeerRenameLocal(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	sh := shellcmd.NewShell(ap, "alice", "")
	reg := shellcmd.Default()

	res, err := reg.Dispatch(sh, "peer", []string{"rename", "alice", "ace"})
	if err != nil {
		t.Fatalf("peer rename: %v", err)
	}
	if res.Kind != shellcmd.KindMessage {
		t.Fatalf("rename: expected KindMessage, got %v", res.Kind)
	}
	if !strings.Contains(res.Message, "alice → ace") {
		t.Errorf("rename message wrong: %q", res.Message)
	}
	if sh.Local.Alias != "ace" {
		t.Errorf("Local.Alias not updated: %q", sh.Local.Alias)
	}
	if sh.AliasFor(sh.Local.PeerID) != "ace" {
		t.Errorf("peerMap not updated; AliasFor returns %q", sh.AliasFor(sh.Local.PeerID))
	}
}

// TestShell_PeerInfo verifies the peer info subcommand delegates to
// cmdInfo and surfaces the same shape.
func TestShell_PeerInfo(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	sh := shellcmd.NewShell(ap, "alice", "")
	reg := shellcmd.Default()

	res, err := reg.Dispatch(sh, "peer", []string{"info", "alice"})
	if err != nil {
		t.Fatalf("peer info: %v", err)
	}
	// cmdInfo returns either KindInfo or KindLines depending on arg shape;
	// we just want to confirm it didn't error and returned something.
	if res.Kind == shellcmd.KindNone {
		t.Errorf("peer info returned empty result")
	}
}

// TestShell_PeerUsage covers argument-validation errors.
func TestShell_PeerUsage(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	sh := shellcmd.NewShell(ap, "alice", "")
	reg := shellcmd.Default()

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no args", []string{}, "usage"},
		{"unknown sub", []string{"warp"}, "unknown peer subcommand"},
		{"rename missing args", []string{"rename", "alice"}, "usage: peer rename"},
		{"rename unknown alias", []string{"rename", "bogus", "x"}, "unknown alias"},
		{"rename reserved target", []string{"rename", "alice", "self"}, "reserved"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := reg.Dispatch(sh, "peer", tc.args)
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}
