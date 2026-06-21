package vcs

import (
	"context"
	"fmt"

	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"
)

// Log walks back from HEAD up to limit versions. limit=0 ⇒ handler
// default (typically 50). Returned slice is newest-first.
//
// First form: returns hashes only. The revision entry (per V7
// §4.4.8) doesn't carry message/author/timestamp fields — that
// metadata belongs to separate entities that the application can
// bind alongside revisions. Once we wire commit metadata as its
// own entity type, Log will fan out hash → metadata for display.
func Log(ctx context.Context, r *Repo, limit int) ([]hash.Hash, error) {
	params := types.RevisionLogParamsData{Prefix: TreePrefix}
	if limit > 0 {
		u := uint64(limit)
		params.Limit = &u
	}
	res, err := r.Peer.Revision().Log(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("vcs log: %w", err)
	}
	return res.Versions, nil
}
