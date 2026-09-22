package shellcmd

// backfill_cursor.go — A BOUNDED WALK THAT CANNOT RESUME IS A CEILING ON
// THE FOLDER, NOT A BOUND ON THE PASS.
//
// # The defect this closes (C15, measured before it was fixed)
//
// `walkRemoteFiles` restarted from the base prefix on every pass and
// stopped at `backfillWalkLimit`. The traversal is deterministic, so a
// folder larger than the cap converged to a FIXED, permanently
// incomplete prefix. Measured (`TestProbe_BoundedWalkMakesNoProgress`,
// 25,000 entries against a 20,000 cap):
//
//	pass 1: 20,000 paths, truncated
//	pass 2: 20,000 paths, truncated
//	paths pass 2 reached that pass 1 did not: 0
//	unreachable: 5,000
//
// Every gate green, truncation honestly reported, and the loop making no
// progress for as long as those 5,000 files do not change again. The
// specification seat wrote the corresponding requirement — *repeated
// passes MUST make progress* — from our reasoning and cited it as
// unperformed. It is performed, and this is the fix.
//
// ⚠ **The cap is not the bug and is not raised.** A sync points at
// somebody else's machine and "how many entries are under this prefix"
// is their answer, not ours — an unbounded walk driven by a remote
// response is a denial of service with our own CPU. What was wrong is
// that the bound had no memory, so it bounded the FOLDER rather than the
// PASS.
//
// # Two properties the resumable walk needs, and one of them was missing
//
//  1. **A total order on leaves that the traversal respects.** The old
//     walk was breadth-first, so leaves arrived level by level and
//     "everything before X" was not expressible. It is now a sorted
//     depth-first walk whose emission order is exactly ascending order of
//     the full path — see `walkRemoteFilesAfter` for the one subtlety
//     that makes that true (a directory sorts under its own name plus a
//     separator, not under its bare name).
//  2. **A cursor.** Held here, in process memory, keyed by (peer,
//     prefix).
//
// # Why the cursor is process memory and not a tree write
//
// It is derived progress state about a loop this workspace owns — the
// same category as `catchUpState` and `historyBudgetState`, and it is
// kept the same way for the same reason. Persisting it would mean an
// entity write per truncated pass on a prefix the declaration watches,
// i.e. a wake on every pass for every oversized folder, which is the
// churn AP73's rule exists to keep out of the tree.
//
// **The limit that comes with that choice, stated rather than hidden:** a
// restart resets the cursor, so a folder mid-cycle starts again from the
// top. That costs a re-walk whose files all short-circuit on F9 as
// already-current — expensive in round trips, wrong in nothing. It is a
// one-field change to persist it in the sync binding if a folder is ever
// large enough that the re-walk matters.
//
// # What advancing the cursor past a FAILED file means
//
// The cursor advances over everything the pass PROCESSED, including
// files that failed to materialize. So a failure is not retried until the
// cursor wraps and the cycle comes round again. That is deliberate: the
// alternative — hold the cursor at the first failure — turns one
// permanently unreadable file into a wall the folder never gets past,
// which is the same shape as the defect above with a different cause.

import (
	"sort"
	"strings"
	"sync"

	"entity-workbench-go/entitysdk"
)

// backfillCursorState is the workspace's memory of where each folder's
// walk stopped. Guarded because the catch-up supervisor runs on its own
// goroutine while verbs run on the caller's.
type backfillCursorState struct {
	mu sync.Mutex
	// after maps a folder to the last path the previous pass processed.
	// An absent or empty entry means "start from the top", which is both
	// the initial state and the state a completed cycle returns to.
	after map[string]string
}

func backfillCursorKey(remotePeerID, sourcePrefix string) string {
	return remotePeerID + "\x00" + sourcePrefix
}

// backfillCursor reads where the next pass over this folder should
// resume. Empty means from the top.
func (ws *ShellWorkspace) backfillCursor(remotePeerID, sourcePrefix string) string {
	if ws == nil {
		return ""
	}
	ws.backfillCursors.mu.Lock()
	defer ws.backfillCursors.mu.Unlock()
	return ws.backfillCursors.after[backfillCursorKey(remotePeerID, sourcePrefix)]
}

// setBackfillCursor records where the next pass resumes. Passing "" wraps
// the folder back to the top, which is what a COMPLETE pass does.
func (ws *ShellWorkspace) setBackfillCursor(remotePeerID, sourcePrefix, after string) {
	if ws == nil {
		return
	}
	ws.backfillCursors.mu.Lock()
	defer ws.backfillCursors.mu.Unlock()
	key := backfillCursorKey(remotePeerID, sourcePrefix)
	if after == "" {
		delete(ws.backfillCursors.after, key)
		return
	}
	if ws.backfillCursors.after == nil {
		ws.backfillCursors.after = make(map[string]string)
	}
	ws.backfillCursors.after[key] = after
}

// nextBackfillCursor is where the next pass over this folder resumes,
// given what this one covered.
//
// A pure function for the same reason `nextCatchUpInterval` is one: it is
// the whole rule, it is two lines, and a gate that re-implements it
// inside the test is asserting against its own copy. `backfill` has the
// only call site.
//
// A pass that was NOT truncated covered the rest of the folder, so the
// cycle wraps to the top — which is what makes `resync` idempotent in the
// operator-visible sense, and that idempotence is this flow's only
// positive confirmation that anything worked.
func nextBackfillCursor(paths []string, truncated bool) string {
	if truncated && len(paths) > 0 {
		return paths[len(paths)-1]
	}
	return ""
}

// walkRemoteFilesAfter enumerates leaf paths under a remote prefix in
// ascending path order, skipping everything at or before `after`, and
// stopping at `backfillWalkLimit`.
//
// Returns the paths (always sorted ascending, by construction rather than
// by a trailing sort) and whether the walk was cut short. A `true` here
// now means "there is more, and the next pass resumes after the last path
// returned" — where before it meant "there is more, and no pass will ever
// reach it".
//
// # The ordering subtlety, which is the whole reason a cursor works
//
// Leaves under a directory `d` all begin with `d + "/"`. So a directory
// must sort under THAT key and not under its bare name, or the emission
// order is not ascending path order and the cursor comparison is wrong at
// exactly the directory boundaries. Concretely, with entries `a` (a
// directory) and `a.txt` (a file), sorting by bare name visits `a` first
// and emits `a/x` before `a.txt` — but `a.txt` < `a/x`, because `.` (0x2E)
// sorts below `/` (0x2F). Sorting the directory as `a/` puts it after
// `a.txt`, which is where every one of its leaves belongs.
//
// # Why a stack and not recursion
//
// Unchanged from the walk this replaces: the depth is chosen by the
// remote peer, and a recursive walk over a remote-supplied tree is a
// stack overflow with someone else's finger on the trigger.
func (ws *ShellWorkspace) walkRemoteFilesAfter(
	local *entitysdk.AppPeer, remotePeerID, sourcePrefix, after string,
) ([]string, bool, error) {
	base := "/" + remotePeerID + "/" + sourcePrefix

	type item struct {
		path string
		dir  bool
	}
	stack := []item{{path: base, dir: true}}
	seen := map[string]bool{base: true}
	var out []string

	for len(stack) > 0 {
		if len(out) >= backfillWalkLimit {
			return out, true, nil
		}
		it := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if !it.dir {
			out = append(out, it.path)
			continue
		}

		entries, err := local.List(it.path)
		if err != nil {
			// The BASE prefix is special: failing it means we never saw
			// their folder, and a caller that reports that as "zero files"
			// states a fact about their machine that it does not have. A
			// SUBDIRECTORY that fails mid-walk is different — the rest of
			// the walk is still real — so it contributes nothing and does
			// not abort.
			if it.path == base {
				return nil, false, err
			}
			continue
		}

		type child struct {
			sortKey string
			path    string
			dir     bool
		}
		kids := make([]child, 0, len(entries))
		for _, e := range entries {
			// Normalize to the peer-qualified form explicitly rather than
			// trusting whatever shape List happened to return. This is
			// AP58's neighbourhood: entries come back qualified today, but
			// a RELATIVE path handed to `local.Get` would read our OWN tree
			// instead of theirs — a local read wearing a remote read's
			// clothes, which would "succeed", materialize nothing new, and
			// report every file as already-current.
			p := qualifyTo(remotePeerID, e.Path)
			if p == "" {
				continue
			}
			if e.HasChildren {
				d := strings.TrimSuffix(p, "/") + "/"
				if seen[d] {
					continue
				}
				seen[d] = true
				// Prune: every leaf under d begins with d, so if `after`
				// is past d and does not live under it, the whole subtree
				// is already behind the cursor. This is what makes a
				// resumed pass cheap in ROUND TRIPS and not just in
				// results — without it the walk would re-list every
				// directory it has already finished with.
				if after != "" && after > d && !strings.HasPrefix(after, d) {
					continue
				}
				kids = append(kids, child{sortKey: d, path: d, dir: true})
				continue
			}
			if seen[p] {
				continue
			}
			seen[p] = true
			if after != "" && p <= after {
				continue
			}
			kids = append(kids, child{sortKey: p, path: p})
		}

		sort.Slice(kids, func(i, j int) bool { return kids[i].sortKey < kids[j].sortKey })
		// Pushed in reverse so they pop in ascending order.
		for i := len(kids) - 1; i >= 0; i-- {
			stack = append(stack, item{path: kids[i].path, dir: kids[i].dir})
		}
	}

	return out, false, nil
}
