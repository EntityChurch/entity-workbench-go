package entitysdk_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"go.entitychurch.org/entity-core-go/core/crypto"

	"entity-workbench-go/entitysdk"
)

// withTempHome redirects $HOME to a tmpdir so identity persistence
// tests don't touch the real ~/.entity/. Returns a cleanup callback.
func withTempHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return dir
}

func TestLoadIdentity_NotFound(t *testing.T) {
	withTempHome(t)
	_, err := entitysdk.LoadIdentity("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing identity")
	}
	if entitysdk.StatusOf(err) != 404 {
		t.Errorf("expected status 404, got %d (%v)", entitysdk.StatusOf(err), err)
	}
	if !errors.Is(err, entitysdk.ErrIdentityNotFound) {
		t.Errorf("expected wrapping ErrIdentityNotFound, got %v", err)
	}
}

func TestCreateIdentity_RoundTrip(t *testing.T) {
	home := withTempHome(t)

	id, err := entitysdk.CreateIdentity("alice")
	if err != nil {
		t.Fatalf("CreateIdentity: %v", err)
	}
	if id.Name != "alice" {
		t.Errorf("Name = %q, want alice", id.Name)
	}
	if id.PeerID == "" {
		t.Errorf("PeerID empty")
	}

	// File must exist on disk.
	keyfile := filepath.Join(home, ".entity", "identities", "alice")
	if _, err := os.Stat(keyfile); err != nil {
		t.Errorf("keypair file missing: %v", err)
	}

	// LoadIdentity returns the same keypair.
	loaded, err := entitysdk.LoadIdentity("alice")
	if err != nil {
		t.Fatalf("LoadIdentity: %v", err)
	}
	if loaded.PeerID != id.PeerID {
		t.Errorf("loaded peer-id %s != created %s", loaded.PeerID, id.PeerID)
	}
}

func TestCreateIdentity_AlreadyExists(t *testing.T) {
	withTempHome(t)
	if _, err := entitysdk.CreateIdentity("alice"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := entitysdk.CreateIdentity("alice")
	if err == nil {
		t.Fatal("expected error on duplicate create")
	}
	if entitysdk.StatusOf(err) != 409 {
		t.Errorf("expected 409 conflict, got %d (%v)", entitysdk.StatusOf(err), err)
	}
	if !errors.Is(err, entitysdk.ErrIdentityExists) {
		t.Errorf("expected wrapping ErrIdentityExists, got %v", err)
	}
}

func TestListIdentities_EmptyAndPopulated(t *testing.T) {
	withTempHome(t)

	// Empty home — no identities directory yet.
	got, err := entitysdk.ListIdentities()
	if err != nil {
		t.Fatalf("ListIdentities (empty): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 identities, got %d", len(got))
	}

	// Create two and re-list.
	for _, name := range []string{"alice", "bob"} {
		if _, err := entitysdk.CreateIdentity(name); err != nil {
			t.Fatalf("CreateIdentity %s: %v", name, err)
		}
	}
	got, err = entitysdk.ListIdentities()
	if err != nil {
		t.Fatalf("ListIdentities (populated): %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 identities, got %d", len(got))
	}
	names := map[string]bool{}
	for _, id := range got {
		names[id.Name] = true
		if id.PeerID == "" {
			t.Errorf("identity %s: empty PeerID", id.Name)
		}
	}
	for _, expected := range []string{"alice", "bob"} {
		if !names[expected] {
			t.Errorf("identity %s missing from list", expected)
		}
	}
}

// TestCreatePeer_WithLoadedIdentity ensures the loaded keypair is
// honored by CreatePeer — the peer-id derived from the V7Identity
// matches the peer-id reported by AppPeer.
func TestCreatePeer_WithLoadedIdentity(t *testing.T) {
	withTempHome(t)
	id, err := entitysdk.CreateIdentity("alice")
	if err != nil {
		t.Fatalf("CreateIdentity: %v", err)
	}
	// Verify the keypair we got is functional — it should sign
	// non-empty data without panic.
	sig := id.Keypair.Sign([]byte("test"))
	if len(sig) == 0 {
		t.Errorf("keypair Sign returned empty signature")
	}
	_ = crypto.PeerID(id.PeerID) // force-typed for clarity
	loaded, err := entitysdk.LoadIdentity("alice")
	if err != nil {
		t.Fatalf("LoadIdentity: %v", err)
	}
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{Keypair: &loaded.Keypair})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })
	if ap.PeerID() != id.PeerID {
		t.Errorf("AppPeer.PeerID = %s, want %s (from loaded identity)",
			ap.PeerID(), id.PeerID)
	}
}
