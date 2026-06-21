// entity-vcs — git-shaped CLI over the entity-system's revision
// extension.
//
// First form. Subcommands: init / add / commit / log /
// status. Repo state lives in ./.entity/. Read-only on working-tree
// files; we never write back.
//
// Run this on a SCRATCH directory while the feature is young. Building
// the binary in workbench-go is fine; exercising it on workbench-go's
// own tree is not.
//
// Like entity-publish, this binary's go.mod is deliberately slim — it
// depends only on entity-workbench-go/vcs (which itself pulls just
// entitysdk + core-go). Easy to extract to its own repo when the
// corridor closes.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"entity-workbench-go/vcs"
)

const usage = `Usage:
  entity-vcs <command> [args]

Commands:
  init [dir]            Create .entity/ in dir (default: cwd) with a
                        fresh ephemeral keypair.
  add <path>...         Ingest files (or directory trees) into the
                        repo. Honors .gitignore.
  snapshot [-m MSG]     Re-scan the working tree + commit in one
                        shot. MSG accepted but not yet persisted.
  commit                Capture current wt/ state as a new revision.
  log [-n N]            Print revision hashes newest-first (default 50).
  status                Print HEAD revision + pending/conflict counts.
  diff <base> <target>  Show path-level diff between two revision hashes.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	ctx := context.Background()

	switch cmd {
	case "init":
		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}
		r, err := vcs.Init(dir, nil)
		check(err)
		defer r.Close()
		fmt.Printf("initialized repo at %s\npeer-id: %s\n", r.EntityDir, r.Peer.PeerID())

	case "add":
		if len(args) == 0 {
			fmt.Fprint(os.Stderr, "usage: entity-vcs add <path>...\n")
			os.Exit(2)
		}
		r, err := vcs.Open(".")
		check(err)
		defer r.Close()
		res, err := vcs.Add(r, args...)
		check(err)
		fmt.Printf("added %d files (%d bytes), skipped %d\n", res.Added, res.Bytes, res.Skipped)

	case "commit":
		r, err := vcs.Open(".")
		check(err)
		defer r.Close()
		v, err := vcs.Commit(ctx, r)
		check(err)
		fmt.Printf("committed %s\n", v)

	case "snapshot":
		fs := flag.NewFlagSet("snapshot", flag.ExitOnError)
		msg := fs.String("m", "", "message (not yet persisted)")
		_ = fs.Parse(args)
		r, err := vcs.Open(".")
		check(err)
		defer r.Close()
		res, err := vcs.Snapshot(ctx, r, *msg)
		check(err)
		fmt.Printf("snapshot: %d files (%d bytes), skipped %d\nversion: %s\n",
			res.Added, res.Bytes, res.Skipped, res.Version)

	case "log":
		fs := flag.NewFlagSet("log", flag.ExitOnError)
		n := fs.Int("n", 50, "max entries")
		_ = fs.Parse(args)
		r, err := vcs.Open(".")
		check(err)
		defer r.Close()
		versions, err := vcs.Log(ctx, r, *n)
		check(err)
		for _, v := range versions {
			fmt.Println(v)
		}

	case "status":
		r, err := vcs.Open(".")
		check(err)
		defer r.Close()
		s, err := vcs.Status(ctx, r)
		check(err)
		head := "<none>"
		if !s.Head.IsZero() {
			head = s.Head.String()
		}
		fmt.Printf("head:      %s\npending:   %d\nconflicts: %d\n", head, s.Pending, s.Conflicts)

	case "diff":
		if len(args) != 2 {
			fmt.Fprint(os.Stderr, "usage: entity-vcs diff <base> <target>\n")
			os.Exit(2)
		}
		base, err := vcs.ParseHash(args[0])
		check(err)
		target, err := vcs.ParseHash(args[1])
		check(err)
		r, err := vcs.Open(".")
		check(err)
		defer r.Close()
		d, err := vcs.Diff(ctx, r, base, target)
		check(err)
		fmt.Printf("base:      %s\ntarget:    %s\nadded:     %d\nremoved:   %d\nchanged:   %d\nunchanged: %d\n",
			d.Base, d.Target, len(d.Added), len(d.Removed), len(d.Changed), d.Unchanged)
		if len(d.Added) > 0 {
			fmt.Println("\n  added:")
			for p := range d.Added {
				fmt.Printf("    + %s\n", p)
			}
		}
		if len(d.Removed) > 0 {
			fmt.Println("\n  removed:")
			for p := range d.Removed {
				fmt.Printf("    - %s\n", p)
			}
		}
		if len(d.Changed) > 0 {
			fmt.Println("\n  changed:")
			for p := range d.Changed {
				fmt.Printf("    ~ %s\n", p)
			}
		}

	default:
		fmt.Fprintf(os.Stderr, "entity-vcs: unknown command %q\n", cmd)
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "entity-vcs:", err)
		os.Exit(1)
	}
}
