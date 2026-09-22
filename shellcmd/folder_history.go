package shellcmd

import (
	"fmt"
	"strings"

	"entity-workbench-go/workbench"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/types"
)

// folder_history.go — the chain a conflict is recovered from, made a
// derived output of the control loop rather than something an operator
// has to know to turn on.
//
// # Why this exists
//
// `DOMAIN-LOCAL-FILES` §1.1a — this repo's own WB-25 closure — rules
// that concurrent same-path writes are last-arrival-wins at the
// filesystem surface, with **both writes recorded in the tree at
// distinct chain positions**. The substrate delivers that in full and
// always has. It just was not switched on: history recording is opt-in
// per path (`ext/history/config.go`, `configCache.find`), and nothing in
// the mount / sync / share path ever installed a config. With none, a
// query at the path returns EMPTY AND NO ERROR — which reads exactly
// like "the tree kept nothing", and did, for a session.
//
// So a received folder's overwritten bytes were unreachable: correct
// per the letter of last-arrival-wins, and missing the half of the
// ruling that makes the loss recoverable.
//
// # Why the reconciler owns it, and not `mount`
//
// Same argument as the policy row, the subscription and the sync
// binding (see reconcile.go): it is DERIVED from a declaration, so a
// verb that wrote it directly would be undone by the next pass, and a
// restart would leave a folder receiving deliveries with no chain
// behind them. Writing it here makes it idempotent, restart-safe, and
// automatically correct for a folder whose direction changes later.
//
// # Why only folders that RECEIVE
//
// A chain position is only worth its storage where a write can arrive
// from somewhere else. A send-only folder has exactly one writer — the
// operator at the keyboard — so recording every one of their saves
// forever buys nothing and costs an entity per save, on mounts that are
// routinely thousands of files.
//
// `Receives()` and not `IsLocal()`, and the distinction is not academic:
// `share` declares the OWNER's folder `both` (declare.go), so in a
// shared folder either side can be overwritten by the other and both
// sides record. Keying on IsLocal() would leave the owner — the peer
// whose files these actually are — as the one side with no chain. That
// is AGENTS.md's standing rule (read direction through
// Publishes()/Receives(), never through IsLocal()) landing on a case
// where the two genuinely differ.
//
// This is deliberately NOT a general "history on" switch. `history
// config` stays available for an operator who wants one.

// folderHistoryConfigName is the config entity name for a mount root.
// One per root, so the write is idempotent and a later Unmount can drop
// exactly its own.
func folderHistoryConfigName(localRoot string) string {
	// A root name is a directory basename; keep the path segment safe
	// without inventing an encoding nothing else reads.
	safe := strings.NewReplacer("/", "-", " ", "-").Replace(localRoot)
	return "folder-" + safe
}

// FolderHistoryPattern is the recording pattern for a mount root.
//
// Short form on purpose: the recorder canonicalizes an unrooted pattern
// against the LOCAL peer id (`ext/history/config.go`,
// canonicalizePattern), so this records our own tree and cannot be
// mistaken for a claim about anyone else's.
func FolderHistoryPattern(localRoot string) string {
	return "local/files/" + localRoot + "/*"
}

// ensureFolderHistory installs the recording config for a receiving
// folder's mount root, and reports whether it wrote one.
//
// Writes only when absent or different. A control loop that rewrites a
// declaration on every pass is a different and worse thing than one
// that reconciles substrate to it — and here it would also append a
// transition to the config's own chain on every startup.
func (ws *ShellWorkspace) ensureFolderHistory(localRoot string) (wrote bool, err error) {
	if localRoot == "" {
		return false, fmt.Errorf("no mount root")
	}
	st := ws.Local.Peer.Store()
	path := "system/history/config/" + folderHistoryConfigName(localRoot)
	want := types.HistoryConfigData{
		Pattern: FolderHistoryPattern(localRoot),
		Enabled: true,
	}

	if ent, ok := st.Get(path); ok {
		var have types.HistoryConfigData
		// A config that exists and does not decode is an ERROR, not a
		// reason to overwrite (AP33) — it may be a newer shape written
		// by a build that knows more than this one.
		if derr := ecf.Decode(ent.Data, &have); derr != nil {
			return false, fmt.Errorf("history config at %s does not decode: %w", path, derr)
		}
		if have.Pattern == want.Pattern && have.Enabled == want.Enabled {
			return false, nil
		}
	}

	if _, err := st.Put(path, "system/history/config", want); err != nil {
		return false, fmt.Errorf("install history config for %s: %w", localRoot, err)
	}
	return true, nil
}

// reconcileFolderHistory is the reconciler's call site. Kept separate
// from ensureFolderHistory so the decision (which folders) and the
// mechanism (write if changed) are readable apart.
func (ws *ShellWorkspace) reconcileFolderHistory(f workbench.FolderData, fs FolderStatus, out *ReconcileOutcome) {
	if !f.Receives() || !fs.Mounted {
		return
	}
	wrote, err := ws.ensureFolderHistory(fs.LocalRoot)
	if err != nil {
		out.Problems = append(out.Problems,
			fmt.Sprintf("folder %q: %v — a delivery that overwrites a local edit "+
				"will not be recoverable", fs.Label, err))
		return
	}
	if wrote {
		out.Actions = append(out.Actions,
			fmt.Sprintf("folder %q: recording changes at %s, so an overwritten local "+
				"edit stays recoverable", fs.Label, FolderHistoryPattern(fs.LocalRoot)))
	}
}
