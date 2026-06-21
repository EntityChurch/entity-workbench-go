package entitysdk

import (
	"fmt"
	"os"
	"path/filepath"
)

// PeersDir returns the absolute path to ~/.entity/peers/, the
// directory containing per-peer state (one subdirectory per named
// peer; see GUIDE-PERSISTENCE.md §1.1).
func PeersDir() (string, error) { return peersDir() }

func peersDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".entity", "peers"), nil
}

// DefaultPeerStoragePath returns the spec-conventional SQLite path
// for a named peer: ~/.entity/peers/{name}/store.db. The filename
// `store.db` matches GUIDE-PERSISTENCE.md §1.1 / §3.
//
// Returns an error if name is empty (no identity → no default
// location).
func DefaultPeerStoragePath(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("no peer name; cannot derive default storage path")
	}
	dir, err := peersDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name, "store.db"), nil
}

// EnsurePeerStorageDir creates the parent directory of path with
// 0700 permissions. Idempotent; safe to call on every startup.
func EnsurePeerStorageDir(path string) error {
	if path == "" || path == ":memory:" {
		return nil
	}
	return os.MkdirAll(filepath.Dir(path), 0o700)
}
