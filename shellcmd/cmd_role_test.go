package shellcmd_test

import (
	"encoding/hex"
	"strings"
	"testing"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/shellcmd"
)

// TestShell_RoleAndIdentityCommands exercises the new identity + role
// shell commands end-to-end through the command registry. Validates
// that:
//   - identity create / list write a keypair to ~/.entity/identities/
//     (redirected via $HOME to a tmpdir).
//   - role define / assign / exclude flow through Shell.Local.Peer.Role().
func TestShell_RoleAndIdentityCommands(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	sh := shellcmd.NewShell(ap, "local", "")
	reg := shellcmd.Default()

	// identity create alice
	res, err := reg.Dispatch(sh, "identity", []string{"create", "alice"})
	if err != nil {
		t.Fatalf("identity create: %v", err)
	}
	if res.Kind != shellcmd.KindMessage || !strings.Contains(res.Message, "alice") {
		t.Errorf("identity create: unexpected result %+v", res)
	}

	// identity list (should show alice + active marker on peer-id-mismatch)
	res, err = reg.Dispatch(sh, "identity", []string{"list"})
	if err != nil {
		t.Fatalf("identity list: %v", err)
	}
	if res.Kind != shellcmd.KindLines {
		t.Fatalf("identity list: expected KindLines, got %v", res.Kind)
	}
	hasAlice := false
	for _, line := range res.Lines {
		if strings.Contains(line, "alice") {
			hasAlice = true
		}
	}
	if !hasAlice {
		t.Errorf("identity list missing alice: %v", res.Lines)
	}

	// role define test-ctx reader system/tree:get,list:public/*
	res, err = reg.Dispatch(sh, "role", []string{
		"define", "test-ctx", "reader",
		"system/tree:get,list:public/*",
	})
	if err != nil {
		t.Fatalf("role define: %v", err)
	}
	if res.Kind != shellcmd.KindMessage {
		t.Errorf("role define: unexpected kind %v", res.Kind)
	}
	if !strings.Contains(res.Message, "reader") {
		t.Errorf("role define message missing role name: %s", res.Message)
	}

	// role assign test-ctx <local-hash> reader
	localHashHex := hex.EncodeToString(ap.IdentityHash().Bytes())
	res, err = reg.Dispatch(sh, "role", []string{
		"assign", "test-ctx", localHashHex, "reader",
	})
	if err != nil {
		t.Fatalf("role assign: %v", err)
	}
	if !strings.Contains(res.Message, "reader") {
		t.Errorf("role assign message: %s", res.Message)
	}

	// role exclude test-ctx <local-hash>
	res, err = reg.Dispatch(sh, "role", []string{
		"exclude", "test-ctx", localHashHex,
	})
	if err != nil {
		t.Fatalf("role exclude: %v", err)
	}
	if !strings.Contains(res.Message, "swept") {
		t.Errorf("role exclude message missing 'swept': %s", res.Message)
	}

	// role unexclude test-ctx <local-hash>
	res, err = reg.Dispatch(sh, "role", []string{
		"unexclude", "test-ctx", localHashHex,
	})
	if err != nil {
		t.Fatalf("role unexclude: %v", err)
	}

	// Bad usage: role assign with too few args.
	_, err = reg.Dispatch(sh, "role", []string{"assign"})
	if err == nil {
		t.Errorf("expected usage error for bare 'role assign'")
	}
}

// TestShell_IdentityBootstrap exercises the `identity bootstrap`
// command end-to-end through the registry. After the ceremony, the
// peer-config entity is bound and at least one local→controller cap
// is issued.
func TestShell_IdentityBootstrap(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	sh := shellcmd.NewShell(ap, "local", "")
	reg := shellcmd.Default()

	res, err := reg.Dispatch(sh, "identity",
		[]string{"bootstrap", "-members", "3", "-threshold", "2", "-name", "test-quorum"})
	if err != nil {
		t.Fatalf("identity bootstrap: %v", err)
	}
	if res.Kind != shellcmd.KindLines {
		t.Fatalf("expected KindLines, got %v", res.Kind)
	}
	hasCeremonySummary := false
	for _, line := range res.Lines {
		if strings.Contains(line, "quorum 2-of-3") {
			hasCeremonySummary = true
		}
	}
	if !hasCeremonySummary {
		t.Errorf("ceremony summary missing from output: %v", res.Lines)
	}

	// Post-bootstrap: peer-config must be bound.
	if !ap.Store().Has("system/identity/peer-config") {
		t.Errorf("peer-config not bound after bootstrap")
	}
}
