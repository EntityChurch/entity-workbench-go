package shellcmd

import (
	"fmt"
	"strings"

	"entity-workbench-go/workbench"
)

// cmd_conflicts.go — the two verbs SYNC-LIMITS §5 rule 2 requires to
// exist before conflicts can be created.
//
// Not "before they are common" and not "before they are a problem":
// before they are CREATABLE. The rule is written that way because the
// case the verbs are needed for is a detection bug, which produces
// conflicts on every file on every pass, and an operator meeting that
// with no way to enumerate or undo has no way back at all.

func cmdConflicts(sh *Shell, args []string) (Result, error) {
	if sh.Local == nil || sh.Local.Peer == nil {
		return Result{}, fmt.Errorf("no local peer")
	}
	ws := sh.ShellWorkspace

	clear := false
	root := ""
	policy := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-clear", "--clear":
			clear = true
		case "-folder", "--folder":
			if i+1 >= len(args) {
				return Result{}, fmt.Errorf("-folder requires a root name")
			}
			root = args[i+1]
			i++
		case "-policy", "--policy":
			if i+1 >= len(args) {
				return Result{}, fmt.Errorf("-policy requires record or keep-both")
			}
			policy = args[i+1]
			i++
		default:
			return Result{}, fmt.Errorf("unknown flag: %s", args[i])
		}
	}

	// Setting the policy lives on THIS verb and not on `direction`,
	// because an operator reaches for it while looking at a conflict —
	// which is the only moment the choice means anything to them. It
	// takes a FOLDER-ID, the same identifier `direction` takes, and not a
	// mount root: a folder is one object across two peers and its id is
	// what names it (S6).
	if policy != "" {
		if root == "" {
			return Result{}, fmt.Errorf(
				"-policy needs -folder <folder-id> — run `status` to see the ids")
		}
		res, err := ws.SetFolderConflictPolicy(root, policy)
		if err != nil {
			return Result{}, err
		}
		if !res.Changed {
			return MessageResult(fmt.Sprintf("%s is already %q — nothing changed. %s",
				res.Label, res.Policy, res.Caveat)), nil
		}
		return MessageResult(fmt.Sprintf("%s: conflicts are now handled as %q. %s",
			res.Label, res.Policy, res.Caveat)), nil
	}

	if clear {
		// Resolved records only. Clearing an UNRESOLVED one throws away
		// the hash that makes the replaced version recoverable, which is
		// a destructive act wearing the word "clear" — so the verb that
		// tidies a list is not also the verb that loses data.
		n, err := ws.ClearConflicts(true)
		if err != nil {
			return Result{}, err
		}
		return MessageResult(fmt.Sprintf(
			"cleared %d resolved conflict record(s). Unresolved ones are kept — "+
				"they carry the only handle on the version that was replaced", n)), nil
	}

	conflicts, problems := ws.Conflicts()
	if root != "" {
		filtered := conflicts[:0]
		for _, c := range conflicts {
			if c.Root == root {
				filtered = append(filtered, c)
			}
		}
		conflicts = filtered
	}

	lines := ConflictSummaryLines(conflicts)
	for _, p := range problems {
		lines = append(lines, "problem: conflict record "+p)
	}

	// The limiter's state, always — including when it is quiet. A peer
	// that has stopped materializing deliveries because it hit the burst
	// limit is neither healthy nor broken, and no other number on any
	// surface would say so.
	if h := ws.ConflictHealth(); h.Limit > 0 {
		if h.Storming {
			lines = append(lines, "",
				fmt.Sprintf("REFUSING DELIVERIES: more than %d conflicts in %.0fs on this "+
					"peer, so conflict resolution has stopped and %d deliveries have been "+
					"turned away. Nothing was overwritten. A burst this size is more likely "+
					"to be a fault here than someone else's editing; delivery resumes by "+
					"itself once it subsides.",
					h.Limit, h.WindowSeconds, h.Refused))
		} else if h.Detected > 0 || h.Refused > 0 {
			lines = append(lines, "",
				fmt.Sprintf("(%d conflict(s) detected and %d delivery(ies) refused in this "+
					"session; the limit is %d per %.0fs)",
					h.Detected, h.Refused, h.Limit, h.WindowSeconds))
		}
	}

	if len(conflicts) > 0 {
		lines = append(lines, "",
			"resolve <key> -keep mine|theirs|both, or resolve --all -keep theirs")
	}
	return LinesResult(lines), nil
}

func cmdResolve(sh *Shell, args []string) (Result, error) {
	if sh.Local == nil || sh.Local.Peer == nil {
		return Result{}, fmt.Errorf("no local peer")
	}
	ws := sh.ShellWorkspace

	target := ""
	all := false
	root := ""
	keep := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--all", "-all":
			all = true
		case "-keep", "--keep":
			if i+1 >= len(args) {
				return Result{}, fmt.Errorf("-keep requires mine, theirs or both")
			}
			keep = strings.ToLower(args[i+1])
			i++
		case "-folder", "--folder":
			if i+1 >= len(args) {
				return Result{}, fmt.Errorf("-folder requires a root name")
			}
			root = args[i+1]
			all = true
			i++
		default:
			if strings.HasPrefix(args[i], "-") {
				return Result{}, fmt.Errorf("unknown flag: %s", args[i])
			}
			target = args[i]
		}
	}

	switch keep {
	case workbench.ConflictKeptMine, workbench.ConflictKeptTheirs, workbench.ConflictKeptBoth:
	case "":
		// REQUIRED, with no default. Every candidate default is wrong for
		// somebody's folder in a way they only discover afterwards, and
		// the whole feature exists because a choice was being made
		// silently on the operator's behalf.
		return Result{}, fmt.Errorf(
			"resolve needs -keep mine, -keep theirs or -keep both — there is no " +
				"default, because choosing one for you is the thing this verb exists to stop")
	default:
		return Result{}, fmt.Errorf("-keep must be mine, theirs or both (got %q)", keep)
	}

	if !all && target == "" {
		return Result{}, fmt.Errorf(
			"usage: resolve <key> -keep mine|theirs|both  |  resolve --all [-folder <root>] -keep ...")
	}

	if !all {
		out, err := ws.ResolveConflict(target, keep)
		if err != nil {
			return Result{}, err
		}
		return MessageResult(resolveLine(out)), nil
	}

	outcomes, problems := ws.ResolveConflicts(root, keep)
	lines := make([]string, 0, len(outcomes)+len(problems)+1)
	for _, o := range outcomes {
		lines = append(lines, resolveLine(o))
	}
	for _, p := range problems {
		lines = append(lines, "could not resolve "+p)
	}
	if len(outcomes) == 0 && len(problems) == 0 {
		scope := "any folder"
		if root != "" {
			scope = "folder " + root
		}
		return MessageResult("no unresolved conflicts in " + scope), nil
	}
	lines = append(lines, "",
		fmt.Sprintf("%d resolved, %d could not be", len(outcomes), len(problems)))
	return LinesResult(lines), nil
}

func resolveLine(o ResolveOutcome) string {
	s := o.Path + ": " + o.Note
	if o.SiblingRemoved != "" {
		s += " (removed " + o.SiblingRemoved + ")"
	}
	return s
}
