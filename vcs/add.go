package vcs

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// AddResult summarises an Add call.
type AddResult struct {
	Added   int
	Skipped int
	Bytes   int64
}

// Add ingests file bytes from paths into the repo's tree.
//
// Each path may be a single file or a directory; directories are
// walked recursively. Entries matching the ignore set are skipped.
// File bytes land at {Repo.Peer.PeerID()}/wt/{rel-path} with type
// "vcs/blob" — sketch-level type sufficient for the revision system
// to capture (path → hash) bindings.
//
// First form: read all bytes into memory and Put in one shot. No
// chunked blobs, no streaming. Files larger than a few MB will be
// inefficient.
func Add(r *Repo, paths ...string) (AddResult, error) {
	ignore := LoadIgnore(r.Dir)
	res := AddResult{}

	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return res, fmt.Errorf("vcs add: abs %s: %w", p, err)
		}
		// Reject paths outside the repo.
		rel, err := filepath.Rel(r.Dir, abs)
		if err != nil || strings.HasPrefix(rel, "..") {
			return res, fmt.Errorf("vcs add: %s is outside the repo", p)
		}

		walkErr := filepath.WalkDir(abs, func(walkPath string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			relW, err := filepath.Rel(r.Dir, walkPath)
			if err != nil {
				return err
			}
			if relW == "." {
				return nil
			}
			if ignore(filepath.ToSlash(relW)) {
				if d.IsDir() {
					return fs.SkipDir
				}
				res.Skipped++
				return nil
			}
			if d.IsDir() {
				return nil
			}
			bytes, err := os.ReadFile(walkPath)
			if err != nil {
				return fmt.Errorf("read %s: %w", walkPath, err)
			}
			treePath := TreePrefix + filepath.ToSlash(relW)
			if _, err := r.Peer.Store().Put(treePath, "vcs/blob", map[string]any{
				"bytes": bytes,
			}); err != nil {
				return fmt.Errorf("put %s: %w", treePath, err)
			}
			res.Added++
			res.Bytes += int64(len(bytes))
			return nil
		})
		if walkErr != nil {
			return res, fmt.Errorf("vcs add: walk %s: %w", p, walkErr)
		}
	}
	return res, nil
}
