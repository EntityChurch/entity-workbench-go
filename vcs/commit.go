package vcs

import (
	"context"
	"fmt"

	"go.entitychurch.org/entity-core-go/core/hash"
)

// Commit captures the current state of the wt/ subtree as a
// revision entry. Returns the new version hash.
//
// First form: no message, no author, no timestamp. The revision
// entry is purely structural (Root + Parents per
// types.RevisionEntryData). Application-level metadata can be
// stored as a separate entity that references this version hash,
// once we decide where to bind it. See vcs design notes.
func Commit(ctx context.Context, r *Repo) (hash.Hash, error) {
	res, err := r.Peer.Revision().Commit(ctx, TreePrefix, "")
	if err != nil {
		return hash.Hash{}, fmt.Errorf("vcs commit: %w", err)
	}
	return res.Version, nil
}
