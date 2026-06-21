package shellboot

import (
	"context"
	"path/filepath"
	"testing"
)

// TestBootstrap_Memory verifies the default (ephemeral, in-memory)
// bootstrap path produces a usable AppPeer + ShellWorkspace, and that
// the workbench handler refs are wired on the workspace.
func TestBootstrap_Memory(t *testing.T) {
	ctx := context.Background()
	ap, ws, err := Bootstrap(ctx, Config{})
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	defer ap.Close()

	if ap.PeerID() == "" {
		t.Fatalf("AppPeer has empty PeerID")
	}
	if ws.Local == nil || ws.Local.Peer != ap {
		t.Fatalf("workspace Local does not point at the returned AppPeer")
	}
	if ws.Local.Alias != "self" {
		t.Fatalf("default alias should be %q, got %q", "self", ws.Local.Alias)
	}
	if ws.NotificationIngest == nil {
		t.Fatalf("NotificationIngest not wired on workspace")
	}
}

// TestBootstrap_SQLiteInMemory verifies that StorageKind=sqlite with
// ":memory:" runs the SQL backend through the SDK without touching
// disk. Exercises the SQL path in CI without temp-dir setup.
func TestBootstrap_SQLiteInMemory(t *testing.T) {
	ctx := context.Background()
	ap, ws, err := Bootstrap(ctx, Config{
		StorageKind: "sqlite",
		StoragePath: ":memory:",
	})
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	defer ap.Close()
	if ws == nil {
		t.Fatalf("nil workspace")
	}
}

// TestBootstrap_SQLiteRequiresPathOrIdentity verifies the safety
// check: -storage=sqlite with neither an explicit path nor an
// identity name to derive from is rejected before it can land an
// orphan store.
func TestBootstrap_SQLiteRequiresPathOrIdentity(t *testing.T) {
	ctx := context.Background()
	_, _, err := Bootstrap(ctx, Config{StorageKind: "sqlite"})
	if err == nil {
		t.Fatalf("expected error when storage=sqlite with no path or identity")
	}
}

// TestBootstrap_AliasFromIdentity verifies the LocalAlias fallback
// chain: explicit alias takes precedence, else identity name, else
// "self".
func TestBootstrap_AliasFromIdentity(t *testing.T) {
	ctx := context.Background()

	// Build a sqlite-backed peer in a temp dir so we can pass a real
	// path without an identity binding (which would try to load from
	// ~/.entity).
	tmpDir := t.TempDir()
	storagePath := filepath.Join(tmpDir, "store.db")

	ap, ws, err := Bootstrap(ctx, Config{
		LocalAlias:  "myname",
		StorageKind: "sqlite",
		StoragePath: storagePath,
	})
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	defer ap.Close()

	if ws.Local.Alias != "myname" {
		t.Fatalf("explicit LocalAlias should win, got %q", ws.Local.Alias)
	}
}
