package vcs

import (
	"context"
	"fmt"

	"go.entitychurch.org/entity-core-go/core/hash"
)

// SnapshotResult bundles the Add and Commit outputs.
type SnapshotResult struct {
	AddResult
	Version hash.Hash
}

// Snapshot is the headline ergonomic verb: re-scan the working tree
// from the repo root, ingest everything non-ignored (Put overwrites
// existing bindings — same path with new bytes → new hash, location
// index points to new), then make a revision.
//
// One-shot "save current state as a new version." For deltas on
// existing files: Store.Put is content-addressed, so re-Put'ing
// identical bytes yields the same hash and is effectively a no-op.
// Re-Put'ing changed bytes yields a new hash; the location index
// rebinds; revision:commit captures the new trie root.
//
// message is accepted but not yet stored (revision entries per V7
// §4.4.8 carry only Root + Parents; commit-metadata-as-companion-
// entity is the deferred application-tags-along pattern).
func Snapshot(ctx context.Context, r *Repo, message string) (SnapshotResult, error) {
	add, err := Add(r, r.Dir)
	if err != nil {
		return SnapshotResult{}, fmt.Errorf("vcs snapshot: add: %w", err)
	}
	v, err := Commit(ctx, r)
	if err != nil {
		return SnapshotResult{AddResult: add}, fmt.Errorf("vcs snapshot: commit: %w", err)
	}
	return SnapshotResult{AddResult: add, Version: v}, nil
}
