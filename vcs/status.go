package vcs

import (
	"context"
	"fmt"

	"go.entitychurch.org/entity-core-go/core/hash"
)

// StatusInfo is the first-form status snapshot.
type StatusInfo struct {
	Head      hash.Hash
	Pending   uint64
	Conflicts uint64
}

// Status returns the current revision state of wt/. First form
// surfaces head version + pending/conflict counts; the richer
// working-tree-vs-HEAD diff (per the design's §13.2) is deferred
// until we wire the localfiles bridge for working-tree scan.
func Status(ctx context.Context, r *Repo) (StatusInfo, error) {
	res, err := r.Peer.Revision().Status(ctx, TreePrefix)
	if err != nil {
		return StatusInfo{}, fmt.Errorf("vcs status: %w", err)
	}
	return StatusInfo{Head: res.Head, Pending: res.Pending, Conflicts: res.Conflicts}, nil
}
