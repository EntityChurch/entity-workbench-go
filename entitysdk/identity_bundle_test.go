package entitysdk_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"entity-workbench-go/entitysdk"
)

// TestIdentityBundle_BootstrapPersistAndReload is the load-bearing
// Cut 3 round-trip: bootstrap an identity-aware peer with persistence,
// close it, create a new peer bound to the same bundle name, and
// confirm the re-loaded peer reproduces identical content hashes.
//
// This verifies the deterministic-ceremony invariant — same inputs
// (controller keypair + member keypairs + properties) produce same
// outputs (quorum-id, controller-cert hash, peer→controller cap).
func TestIdentityBundle_BootstrapPersistAndReload(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Phase 1: bootstrap a fresh peer + persist.
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer (initial): %v", err)
	}
	res1, err := ap.BootstrapIdentity(context.Background(), entitysdk.BootstrapOpts{
		QuorumMembers:   3,
		QuorumThreshold: 2,
		QuorumName:      "round-trip-test",
		BundleName:      "alice",
	})
	if err != nil {
		t.Fatalf("BootstrapIdentity: %v", err)
	}
	if res1.BundleDir == "" {
		t.Fatalf("BundleDir empty in result")
	}
	if !directoryExists(res1.BundleDir) {
		t.Fatalf("bundle directory missing on disk: %s", res1.BundleDir)
	}
	originalPeerID := ap.PeerID()
	originalQuorumID := res1.QuorumID
	originalCertHash := res1.ControllerCertHash
	if err := ap.Close(); err != nil {
		t.Fatalf("Close (initial peer): %v", err)
	}

	// Phase 2: re-load via PeerConfig.Identity. Auto-detection
	// should see the directory and dispatch to identity-aware mode.
	ap2, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		Identity: &entitysdk.IdentityBindingConfig{Name: "alice"},
	})
	if err != nil {
		t.Fatalf("CreatePeer (reload): %v", err)
	}
	t.Cleanup(func() { _ = ap2.Close() })

	if ap2.PeerID() != originalPeerID {
		t.Errorf("reloaded peer-id = %s, want %s (controller keypair should round-trip)",
			ap2.PeerID(), originalPeerID)
	}
	if !ap2.Store().Has("system/identity/peer-config") {
		t.Errorf("peer-config not bound on reloaded peer")
	}

	// The re-minted quorum + controller-cert entities must have the
	// same content hashes as the original bootstrap (deterministic
	// ceremony invariant).
	quorumPath := "system/quorum/" + hexBytes(originalQuorumID.Bytes())
	if !ap2.Store().Has(quorumPath) {
		t.Errorf("re-minted quorum entity missing at %s", quorumPath)
	}
	certPath := "system/identity/internal/cert/" + hexBytes(originalCertHash.Bytes())
	if !ap2.Store().Has(certPath) {
		t.Errorf("re-minted controller cert missing at %s", certPath)
	}
}

// TestIdentityBundle_LoadWithoutBundle errors when the requested
// identity name doesn't exist as either a flat file or a bundle dir.
func TestIdentityBundle_LoadWithoutBundle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	_, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		Identity: &entitysdk.IdentityBindingConfig{Name: "missing"},
	})
	if err == nil {
		t.Fatal("expected error when identity does not exist on disk")
	}
	if entitysdk.StatusOf(err) != 404 {
		t.Errorf("expected status 404, got %d (%v)", entitysdk.StatusOf(err), err)
	}
}

// TestIdentityBundle_BootstrapTwiceRefuses guards against
// accidentally clobbering an existing bundle. The user has to
// explicitly remove the old one or pick a new name.
func TestIdentityBundle_BootstrapTwiceRefuses(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	if _, err := ap.BootstrapIdentity(context.Background(), entitysdk.BootstrapOpts{
		BundleName: "alice",
	}); err != nil {
		t.Fatalf("first bootstrap: %v", err)
	}
	_, err = ap.BootstrapIdentity(context.Background(), entitysdk.BootstrapOpts{
		BundleName: "alice",
	})
	if err == nil {
		t.Fatal("expected second bootstrap to fail (bundle already exists)")
	}
	if entitysdk.StatusOf(err) != 409 {
		t.Errorf("expected 409 conflict, got %d (%v)", entitysdk.StatusOf(err), err)
	}
}

// TestIdentityBundle_V7FlatStillWorks confirms the V7-only flat-
// keypair path still works through PeerConfig.Identity. Auto-
// detection picks the right mode based on file vs directory.
func TestIdentityBundle_V7FlatStillWorks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if _, err := entitysdk.CreateIdentity("flat-alice"); err != nil {
		t.Fatalf("CreateIdentity (V7 flat): %v", err)
	}

	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		Identity: &entitysdk.IdentityBindingConfig{Name: "flat-alice"},
	})
	if err != nil {
		t.Fatalf("CreatePeer (V7 reload): %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	// V7-only mode means no peer-config — the identity stack still
	// works (handlers are wired) but no ceremony was run.
	if ap.Store().Has("system/identity/peer-config") {
		t.Errorf("V7-only flat-keypair load should NOT have run identity bootstrap")
	}
}

func directoryExists(p string) bool {
	info, err := os.Stat(p)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func hexBytes(b []byte) string {
	const hexdigits = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, c := range b {
		out[i*2] = hexdigits[c>>4]
		out[i*2+1] = hexdigits[c&0x0f]
	}
	return string(out)
}

// silence linter for unused import path/filepath in some build configs
var _ = filepath.Join
