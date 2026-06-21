// Package vcs is the entity-system git-shaped binary's library half.
//
// Scope (first form):
//
//   - Repo lives in WorkingDir/.entity/
//   - Keypair is repo-local (.entity/keypair, PEM seed format)
//   - File content lands at /{peer_id}/wt/<repo-relative path>
//   - Revisions track the "wt/" subtree via the existing
//     system/revision extension (peer.Revision().Commit/Log/Status)
//   - No commit message metadata, no authors, no branches
//
// Read-only on the working tree — we ingest file BYTES into .entity/.
// The library never writes back to working-tree files. Safe to run
// against any working tree, but exercising it on a repo you care
// about is still inadvisable while the feature is this young.
//
// Deferred and tagged: .entityignore, commit metadata (message, author,
// timestamp stored alongside revisions as a separate application-typed
// entity), push/pull/clone, branches, checkout.
package vcs

import (
	"fmt"
	"os"
	"path/filepath"

	"go.entitychurch.org/entity-core-go/core/crypto"

	"entity-workbench-go/entitysdk"
)

// TreePrefix is the subtree under which working-tree files are
// bound. Choosing a dedicated prefix (rather than "/") keeps the
// peer's namespace open for non-vcs entities (capability grants,
// handler registrations, etc.) that bootstrap installs at peer
// construction time.
const TreePrefix = "wt/"

// Repo is an open entity-vcs working tree.
type Repo struct {
	Dir       string // working tree root (contains .entity/)
	EntityDir string // {Dir}/.entity
	Peer      *entitysdk.AppPeer
}

// Close releases the underlying peer's resources.
func (r *Repo) Close() error {
	if r == nil || r.Peer == nil {
		return nil
	}
	return r.Peer.Close()
}

// Init creates a new repo at dir.
//
// If keypair is non-nil, it's used as the repo identity (and saved
// to disk). If nil, a fresh ephemeral keypair is generated and
// saved. The keypair persists in .entity/keypair so subsequent
// `vcs add`/`vcs commit` invocations rebind the same peer-id and
// the revision chain stays coherent.
func Init(dir string, keypair *crypto.Keypair) (*Repo, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("vcs init: abs path: %w", err)
	}
	entDir := filepath.Join(abs, ".entity")
	if _, err := os.Stat(entDir); err == nil {
		return nil, fmt.Errorf("vcs init: %s already exists", entDir)
	}
	if err := os.MkdirAll(entDir, 0o755); err != nil {
		return nil, fmt.Errorf("vcs init: mkdir .entity: %w", err)
	}

	kp := keypair
	if kp == nil {
		gen, err := crypto.Generate()
		if err != nil {
			return nil, fmt.Errorf("vcs init: generate keypair: %w", err)
		}
		kp = &gen
	}
	if err := crypto.SaveIdentityToDir(entDir, "keypair", *kp); err != nil {
		return nil, fmt.Errorf("vcs init: save keypair: %w", err)
	}

	return openWithKeypair(abs, entDir, kp)
}

// Open loads an existing repo at dir.
func Open(dir string) (*Repo, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("vcs open: abs path: %w", err)
	}
	entDir := filepath.Join(abs, ".entity")
	if _, err := os.Stat(entDir); err != nil {
		return nil, fmt.Errorf("vcs open: not a repo (no .entity/ at %s)", abs)
	}
	kp, err := crypto.LoadIdentityFromFile(filepath.Join(entDir, "keypair"))
	if err != nil {
		return nil, fmt.Errorf("vcs open: load keypair: %w", err)
	}
	return openWithKeypair(abs, entDir, &kp)
}

func openWithKeypair(dir, entDir string, kp *crypto.Keypair) (*Repo, error) {
	storePath := filepath.Join(entDir, "store.db")
	cfg := entitysdk.PeerConfig{
		Keypair: kp,
		Storage: entitysdk.StorageConfig{Kind: "sqlite", Path: storePath},
	}
	ap, err := entitysdk.CreatePeer(cfg)
	if err != nil {
		return nil, fmt.Errorf("vcs: create peer: %w", err)
	}
	return &Repo{Dir: dir, EntityDir: entDir, Peer: ap}, nil
}
