package workbench

import (
	"context"
	"fmt"
	"strings"

	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/store"
	"go.entitychurch.org/entity-core-go/ext/localfiles"
)

// RestoreLocalFilesRoots rebuilds the localfiles handler's IN-MEMORY root
// table from the mount configs in the tree, at startup.
//
// # Why this exists, and why it is here and not in the kernel
//
// `localfiles.Handler` keeps its roots in `h.roots`, a plain map. Every
// path that puts bytes on disk goes through `findRootMapping`, which
// reads that map and nothing else. The map is rebuilt at startup by the
// kernel's own `Handler.Load` — and `Load` restores nothing, for a reason
// this repo has a name for.
//
// `Load` enumerates `li.List("system/config/local/files/")` and then does
//
//	rel := strings.TrimPrefix(entry.Path, configPathPrefix)
//	if rel == "" || strings.Contains(rel, "/") { continue }
//
// The location index returns PEER-QUALIFIED paths —
// `/{peer-id}/system/config/local/files/downloads` — so the TrimPrefix
// matches nothing, `rel` keeps its slashes, and every root is skipped.
// `loaded` stays 0, so even the "rehydrated N root(s)" line never prints.
// That is **AP58** exactly: `TrimPrefix` with a relative prefix against a
// peer-qualified path, giving a confidently wrong answer.
//
// The consequences are the whole product, and both halves are silent:
//
//   - **No root mapping**, so `local/files:write` answers 404
//     `no_root_mapping`. A received file cannot land. The operator sees
//     a share that accepts cleanly and then fails every file.
//   - **No watcher**, because `StartWatching` is inside the same skipped
//     loop. Local edits never reach the tree, so the folder the operator
//     is sharing looks empty from every surface.
//
// A mount therefore works until the app is restarted and is dead
// afterwards, while `mounts`, the Local Files panel and Accept's own
// precondition all keep reporting it as healthy — they read the TREE,
// which is intact. Two sources of truth, only one of which is rebuilt.
//
// This lives here because `../entity-core-go` is a sibling dependency and
// is not edited from this repo; the finding is routed to core-go rather
// than patched in place. Until it lands there, this restores the same
// table from the same entities, using `RelativeUnder` — which is
// idempotent and peer-qualification-safe, and is the fix AP58 prescribes.
//
// It is deliberately a SEPARATE pass rather than a replacement: if the
// kernel's Load starts working, the roots it restored are already present
// and `AddRoot` reports an overlap, which is treated as "already there"
// rather than as an error.
func RestoreLocalFilesRoots(
	ctx context.Context,
	st *Store,
	h *localfiles.Handler,
	cs store.ContentStore,
	li store.LocationIndex,
	identityHash hash.Hash,
) (restored int, problems []string) {
	if st == nil || h == nil || cs == nil || li == nil {
		return 0, nil
	}

	for _, e := range st.List(MountConfigPrefix) {
		root, under := RelativeUnder(e.Path, MountConfigPrefix)
		if !under || root == "" || strings.Contains(root, "/") {
			// A nested path under our namespace is something else's
			// business — the watch/ sub-namespace lives here too.
			continue
		}

		ent, ok := st.Get(e.Path)
		if !ok {
			problems = append(problems,
				fmt.Sprintf("%s: config listed but not resolvable", root))
			continue
		}
		cfg, err := localfiles.RootConfigDataFromEntity(ent)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: config did not decode: %v", root, err))
			continue
		}

		if err := h.AddRoot(root, cfg, cs, li); err != nil {
			// An overlap means this root is already mapped — either the
			// kernel's Load worked, or we have already run. Not a
			// problem, and reporting it as one would train the operator
			// to ignore this line.
			if strings.Contains(err.Error(), "overlaps with existing root") {
				continue
			}
			problems = append(problems, fmt.Sprintf("%s: %v", root, err))
			continue
		}

		// The watcher is the disk→tree half and is skipped by the same
		// kernel loop. Without it the mount accepts writes and never
		// notices a file the operator adds, which is the half that makes
		// a shared folder look empty on the machine that owns it.
		if err := h.StartWatching(ctx, root, cs, li, identityHash); err != nil {
			problems = append(problems,
				fmt.Sprintf("%s: root restored but watcher did not start: %v", root, err))
		}
		restored++
	}
	return restored, problems
}
