package vcs

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"
)

// Diff returns the path-level diff between two revisions at the
// repo's tracked prefix. Wraps peer.Revision().Diff.
func Diff(ctx context.Context, r *Repo, base, target hash.Hash) (types.DiffData, error) {
	res, err := r.Peer.Revision().Diff(ctx, TreePrefix, base, target)
	if err != nil {
		return types.DiffData{}, fmt.Errorf("vcs diff: %w", err)
	}
	return res, nil
}

// ParseHash accepts either the canonical "ecf-sha256:<hex>" form or
// a bare 64-hex-char string. Returns the corresponding Hash.
//
// First-form scope — no short-prefix matching yet (callers supply the
// full 64-hex digest). Revision().Status / Log already give full
// hashes; copy/paste is the workflow.
func ParseHash(s string) (hash.Hash, error) {
	s = strings.TrimPrefix(s, "ecf-sha256:")
	if len(s) != 64 {
		return hash.Hash{}, fmt.Errorf("vcs: hash must be 64 hex chars, got %d", len(s))
	}
	digest, err := hex.DecodeString(s)
	if err != nil {
		return hash.Hash{}, fmt.Errorf("vcs: hex decode: %w", err)
	}
	// Wire form is [algorithm-byte || 32-byte digest]. SHA-256 = 0x00.
	wire := append([]byte{0x00}, digest...)
	return hash.FromBytes(wire)
}
