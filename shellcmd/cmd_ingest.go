package shellcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"entity-workbench-go/workbench"
)

// cmdIngest dispatches `ingest <subcommand> [args...]`.
//
// Subcommands:
//
//	ingest tree <src-dir> <tree-prefix>   — walk src-dir, write each .md
//	                                        file at tree-prefix/<relpath>
//	                                        preserving folder structure.
//
// Flat-slug knowledge-base ingest (workbench.IngestMarkdownDirectory)
// is not surfaced through the shell — it predates the structured
// form and is currently driven from the renderer's main() at startup
// via $KB_SEED_DIR. Surface it as a subcommand here when there's a
// caller need.
func cmdIngest(sh *Shell, args []string) (Result, error) {
	if len(args) < 1 {
		return Result{}, fmt.Errorf("usage: ingest <tree> [args]")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "tree":
		return cmdIngestTree(sh, rest)
	default:
		return Result{}, fmt.Errorf("unknown ingest subcommand: %s", sub)
	}
}

func cmdIngestTree(sh *Shell, args []string) (Result, error) {
	if len(args) < 2 {
		return Result{}, fmt.Errorf("usage: ingest tree <src-dir> <tree-prefix>")
	}
	srcDir, treePrefix := args[0], args[1]

	// Expand a leading ~ for the user's convenience.
	if strings.HasPrefix(srcDir, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			srcDir = filepath.Join(home, srcDir[2:])
		}
	}

	start := time.Now()
	res, err := workbench.IngestMarkdownTree(sh.Local.Peer.Store(), srcDir, treePrefix)
	if err != nil {
		return Result{}, fmt.Errorf("ingest tree: %w", err)
	}
	dur := time.Since(start)

	lines := []string{
		fmt.Sprintf("source:   %s", res.SrcRoot),
		fmt.Sprintf("prefix:   %s", res.Prefix),
		fmt.Sprintf("created:  %d", res.Created),
		fmt.Sprintf("skipped:  %d (non-md or read errors)", res.Skipped),
		fmt.Sprintf("bytes:    %d", res.BytesIn),
		fmt.Sprintf("elapsed:  %s", dur.Round(time.Millisecond)),
	}
	if len(res.Errors) > 0 {
		lines = append(lines, fmt.Sprintf("errors:   %d", len(res.Errors)))
		// Cap printed errors to keep the shell output legible —
		// callers wanting all of them can re-run with verbose
		// logging or look at the result programmatically.
		const maxShow = 5
		shown := len(res.Errors)
		if shown > maxShow {
			shown = maxShow
		}
		for _, e := range res.Errors[:shown] {
			lines = append(lines, "  - "+e)
		}
		if len(res.Errors) > maxShow {
			lines = append(lines, fmt.Sprintf("  ... %d more errors elided", len(res.Errors)-maxShow))
		}
	}
	return LinesResult(lines), nil
}
