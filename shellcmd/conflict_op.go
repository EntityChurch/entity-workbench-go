package shellcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"entity-workbench-go/workbench"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/content"
	"go.entitychurch.org/entity-core-go/ext/localfiles"

	"github.com/fxamacker/cbor/v2"
)

// conflict_op.go — listing what a delivery replaced, and putting it back.
//
// # Why this ships in the same change as the thing that detects it
//
// `SYNC-LIMITS-AND-FAILURE-MODES` §5 rule 2, and it is the rule that
// answers the operator's actual question. A tool that can PRODUCE
// conflicts and cannot enumerate or undo them leaves *"what do I even do
// now?"* with no answer, and the answer is needed most in exactly the
// case the tool is most likely to be wrong — a detection bug, which fires
// on every file rather than on one.
//
// So: `conflicts` lists, `resolve` acts, both take a whole folder, and
// neither existed one commit later than the detection.
//
// # Why `-keep mine` writes through the FILESYSTEM
//
// This is the part that is easy to get subtly wrong, and the wrong
// version passes every test that stops at "the bytes are back on disk".
//
// Restoring through `local/files:write` — the obvious route, and the one
// blob-resolve itself uses — records the restore on the chain as
// `local/files:write`, which is the provenance of a DELIVERY. The next
// catch-up pass then reads that head, concludes the local copy arrived
// from the sender and is merely stale, and overwrites the restore
// silently. The operator's choice would survive until the next pass and
// no longer, with nothing said.
//
// Writing the bytes into the mounted directory makes the watcher ingest
// them and record `local/files:watch`, which is the truth: the operator
// chose these bytes. It is not a forged provenance, it is the correct
// one. And the resolved record additionally suppresses re-delivery of
// that exact (mine, theirs) pair, so the choice is not merely re-detected
// as a conflict on every pass.
//
// # What `-keep mine` does NOT promise
//
// In a folder that only RECEIVES, the sender's next change to that path
// wins again — that is what receive-only means, and the verb says so
// rather than implying a merge. What it does promise is that the next
// overwrite will be recorded rather than silent.

// ConflictKeepChoice is what an operator decided about a conflict.
type ConflictKeepChoice = string

// ResolveOutcome is what resolving one conflict did.
type ResolveOutcome struct {
	Key  string
	Path string
	Kept string
	// Note is the operator-facing sentence, composed here so the shell
	// and the GUI cannot describe the same resolution differently.
	Note string
	// Restored is whether bytes were written back to disk.
	Restored bool
	// SiblingRemoved / SiblingWritten name the keep-both file that was
	// dropped or created, or "".
	SiblingRemoved string
	SiblingWritten string
}

// Conflicts lists every recorded conflict, newest first.
func (ws *ShellWorkspace) Conflicts() ([]workbench.ConflictData, []string) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return nil, nil
	}
	return workbench.LoadConflicts(ws.Local.Peer.Store())
}

// ConflictHealth reports the burst limiter's state, or a zero value when
// no blob-resolve handler is wired.
//
// Zero and not an error: a peer with no handler has no conflicts and no
// limiter, which is a real configuration rather than a fault. The caller
// distinguishes it by Limit == 0.
func (ws *ShellWorkspace) ConflictHealth() workbench.ConflictHealth {
	if ws == nil || ws.BlobResolve == nil {
		return workbench.ConflictHealth{}
	}
	return ws.BlobResolve.ConflictHealth()
}

// ResolveConflict acts on one record.
//
// keep is one of workbench.ConflictKeptMine / KeptTheirs / KeptBoth.
func (ws *ShellWorkspace) ResolveConflict(key, keep string) (ResolveOutcome, error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return ResolveOutcome{}, fmt.Errorf("workspace has no local peer")
	}
	st := ws.Local.Peer.Store()
	c, ok := workbench.LoadConflict(st, key)
	if !ok {
		return ResolveOutcome{}, fmt.Errorf("no conflict recorded under %q — "+
			"run `conflicts` to list them", key)
	}
	return ws.resolveOne(c, keep)
}

// ResolveConflicts acts on every UNRESOLVED conflict, optionally narrowed
// to one folder root.
//
// Bulk is rule 2's other half: a storm produces conflicts by the thousand
// and an operator resolving them one key at a time has no way back. It
// reports per-item failures rather than stopping at the first, because a
// bulk operation that aborts part way leaves a folder in a state neither
// the operator nor the tool can describe (AP15's shape at the level of an
// operation rather than a test).
func (ws *ShellWorkspace) ResolveConflicts(root, keep string) (outcomes []ResolveOutcome, problems []string) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return nil, []string{"workspace has no local peer"}
	}
	conflicts, probs := workbench.UnresolvedConflicts(ws.Local.Peer.Store())
	problems = append(problems, probs...)
	for _, c := range conflicts {
		if root != "" && c.Root != root {
			continue
		}
		out, err := ws.resolveOne(c, keep)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", c.Path, err))
			continue
		}
		outcomes = append(outcomes, out)
	}
	sort.Slice(outcomes, func(i, j int) bool { return outcomes[i].Path < outcomes[j].Path })
	return outcomes, problems
}

// ClearConflicts drops records. resolvedOnly keeps the ones an operator
// has not decided yet, which is the safe default for a verb whose whole
// job is to make a list shorter.
func (ws *ShellWorkspace) ClearConflicts(resolvedOnly bool) (int, error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return 0, fmt.Errorf("workspace has no local peer")
	}
	st := ws.Local.Peer.Store()
	all, _ := workbench.LoadConflicts(st)
	n := 0
	for _, c := range all {
		if resolvedOnly && c.Unresolved() {
			continue
		}
		if workbench.RemoveConflict(st, c.Key()) {
			n++
		}
	}
	return n, nil
}

func (ws *ShellWorkspace) resolveOne(c workbench.ConflictData, keep string) (ResolveOutcome, error) {
	st := ws.Local.Peer.Store()
	out := ResolveOutcome{Key: c.Key(), Path: c.Path, Kept: keep}

	switch keep {
	case workbench.ConflictKeptTheirs:
		// Their version is already on disk — this records the DECISION
		// and tidies up, it does not move bytes.
		if c.KeepBothPath != "" {
			if err := ws.deleteTreeFile(c.KeepBothPath); err != nil {
				return out, fmt.Errorf("remove %s: %w", c.KeepBothPath, err)
			}
			out.SiblingRemoved = c.KeepBothPath
		}
		out.Note = "kept the version that arrived; nothing on disk changed"

	case workbench.ConflictKeptMine:
		if !c.Recoverable {
			return out, fmt.Errorf("the replaced version at %s is not recoverable — "+
				"this path had no change recording when it was overwritten, so the "+
				"bytes were never kept. `-keep theirs` records the decision", c.Path)
		}
		if err := ws.restoreThroughFilesystem(c); err != nil {
			return out, err
		}
		out.Restored = true
		if c.KeepBothPath != "" {
			if err := ws.deleteTreeFile(c.KeepBothPath); err == nil {
				out.SiblingRemoved = c.KeepBothPath
			}
		}
		out.Note = "your version is back on disk. Deliveries of the version it " +
			"replaced are now declined, so a catch-up pass will not undo this"

	case workbench.ConflictKeptBoth:
		sibling, err := ws.writeSibling(c)
		if err != nil {
			return out, err
		}
		c.KeepBothPath = sibling
		out.SiblingWritten = sibling
		out.Note = "both kept — your version is at " + sibling

	default:
		return out, fmt.Errorf("keep must be one of mine, theirs, both (got %q)", keep)
	}

	c.Kept = keep
	c.ResolvedAtMillis = uint64(time.Now().UnixMilli())
	if err := workbench.SaveConflict(st, c); err != nil {
		return out, fmt.Errorf("record the resolution: %w", err)
	}
	return out, nil
}

// restoreThroughFilesystem writes the replaced version back into the
// mounted directory, so the WATCHER ingests it as the local edit it is.
//
// See this file's header for why not local/files:write. The short form: a
// restore that records as a delivery is undone by the next catch-up pass,
// silently, because the pass reads that provenance and concludes the copy
// is merely stale.
func (ws *ShellWorkspace) restoreThroughFilesystem(c workbench.ConflictData) error {
	fsRoot, rel, err := ws.mountLocationOf(c.Path, c.Root)
	if err != nil {
		return err
	}
	bytes, err := content.Reassemble(ws.Local.Peer.RawContentStore(), c.MineHash)
	if err != nil {
		return fmt.Errorf("the replaced version's bytes are no longer in this "+
			"peer's content store: %w", err)
	}
	target := filepath.Join(fsRoot, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(target), err)
	}
	if err := os.WriteFile(target, bytes, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}
	return nil
}

// writeSibling materializes the replaced version at the keep-both path
// through local/files:write.
//
// Through the handler and not the filesystem, which is the opposite
// choice to restoreThroughFilesystem and for the reason that makes both
// correct: the sibling is a NEW file whose provenance nobody will read,
// while the original path's provenance is what the next delivery
// classifies on.
func (ws *ShellWorkspace) writeSibling(c workbench.ConflictData) (string, error) {
	if c.MineHash.IsZero() {
		return "", fmt.Errorf("no replaced version recorded for %s", c.Path)
	}
	sibling := workbench.KeepBothPath(workbench.TreeRelative(c.Path), c.MineHash)
	h := c.MineHash
	req := localfiles.WriteRequestData{Content: &h, CreateDirs: true}
	raw, err := ecf.Encode(req)
	if err != nil {
		return "", err
	}
	ent, err := entity.NewEntity(localfiles.TypeWriteRequest, cbor.RawMessage(raw))
	if err != nil {
		return "", err
	}
	resp, err := ws.Local.Peer.Executor().ExecuteOnResource("local/files", "write", ent,
		&types.ResourceTarget{Targets: []string{sibling}})
	if err != nil {
		return "", fmt.Errorf("write %s: %w", sibling, err)
	}
	if resp == nil || resp.Status >= 400 {
		status := uint(500)
		if resp != nil {
			status = resp.Status
		}
		return "", fmt.Errorf("write %s: status %d", sibling, status)
	}
	return sibling, nil
}

// deleteTreeFile removes a file through the handler, so the mount, the
// tree and the disk stay in step.
func (ws *ShellWorkspace) deleteTreeFile(treePath string) error {
	bare := workbench.TreeRelative(treePath)
	// A zero Entity for params, which is the form blob_resolve.go's own
	// delete arm uses — `delete` declares no InputType in the handler's
	// manifest, so a constructed params entity is a shape nothing reads
	// and a shape nothing else in this tree sends.
	resp, err := ws.Local.Peer.Executor().ExecuteOnResource("local/files", "delete",
		entity.Entity{}, &types.ResourceTarget{Targets: []string{bare}})
	if err != nil {
		return err
	}
	if resp != nil && resp.Status >= 400 && resp.Status != 404 {
		return fmt.Errorf("status %d", resp.Status)
	}
	return nil
}

// mountLocationOf resolves a tree path to (filesystem root, relative
// path) through the mount's own config.
//
// The kernel's RootConfigData is the one place that knows where a mount
// root lives on disk, and reading it here rather than remembering it is
// the same rule the whole reconciler runs on: a second copy of a fact
// goes stale the moment the first one moves.
func (ws *ShellWorkspace) mountLocationOf(qualifiedPath, root string) (fsRoot, rel string, err error) {
	bare := workbench.TreeRelative(qualifiedPath)
	prefix := workbench.LocalFilesSourcePrefix + root + "/"
	if !strings.HasPrefix(bare, prefix) {
		return "", "", fmt.Errorf("%s is not under mount root %q", qualifiedPath, root)
	}
	rel = strings.TrimPrefix(bare, prefix)

	ent, ok := ws.Local.Peer.Store().Get(workbench.MountConfigPrefix + root)
	if !ok {
		return "", "", fmt.Errorf("mount %q has no config on this peer — it may have "+
			"been unmounted since the conflict was recorded", root)
	}
	cfg, decErr := localfiles.RootConfigDataFromEntity(ent)
	if decErr != nil {
		return "", "", fmt.Errorf("mount %q config does not decode: %w", root, decErr)
	}
	if cfg.FilesystemRoot == "" {
		return "", "", fmt.Errorf("mount %q records no filesystem root", root)
	}
	return cfg.FilesystemRoot, rel, nil
}

// ConflictSummaryLines renders the list for a text surface.
func ConflictSummaryLines(conflicts []workbench.ConflictData) []string {
	if len(conflicts) == 0 {
		return []string{"no conflicts recorded — no delivery has replaced an edit of yours"}
	}
	lines := make([]string, 0, len(conflicts)*2)
	for _, c := range conflicts {
		when := time.UnixMilli(int64(c.AtMillis)).Format("2006-01-02 15:04:05")
		mark := "!"
		if !c.Unresolved() {
			mark = " "
		}
		lines = append(lines, fmt.Sprintf("%s %s  %s", mark, when, c.Summary()))
		lines = append(lines, fmt.Sprintf("    key %s   yours %s   theirs %s",
			c.Key(), shortHash(c.MineHash), shortHash(c.TheirsHash)))
	}
	return lines
}

// SetFolderConflictPolicyResult is what changing a folder's policy did.
type SetFolderConflictPolicyResult struct {
	FolderID string
	Label    string
	Previous string
	Policy   string
	Changed  bool
	// Caveat names a consequence the operator would otherwise meet later.
	Caveat string
}

// SetFolderConflictPolicy writes the folder's declared conflict policy.
//
// A DECLARATION and nothing else, which is the rule the whole flow runs
// on (reconcile.go): the handler reads this record when a collision
// happens, so there is no substrate to reconcile and nothing to
// re-establish. Writing a policy anywhere other than the declaration
// would be undone by the next pass — that is how `unshare` once reversed
// itself at the next launch.
func (ws *ShellWorkspace) SetFolderConflictPolicy(folderID, policy string) (SetFolderConflictPolicyResult, error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return SetFolderConflictPolicyResult{}, fmt.Errorf("workspace has no local peer")
	}
	p, err := workbench.ParseConflictPolicy(policy)
	if err != nil {
		return SetFolderConflictPolicyResult{}, err
	}
	st := ws.Local.Peer.Store()
	f, ok := ws.findFolderByIDOrRoot(folderID)
	if !ok {
		return SetFolderConflictPolicyResult{}, fmt.Errorf(
			"no folder %q is declared on this peer — `status` lists the ids", folderID)
	}
	res := SetFolderConflictPolicyResult{
		FolderID: f.ID,
		Label:    f.DisplayLabel(),
		Previous: f.ConflictPolicy(),
		Policy:   p,
	}
	if f.ConflictPolicy() == p {
		return res, nil
	}
	f.Conflict = p
	if err := workbench.SaveFolder(st, f); err != nil {
		return res, err
	}
	res.Changed = true
	if p == workbench.ConflictPolicyKeepBoth {
		res.Caveat = "a collision here will now leave a second file beside the " +
			"original, and this folder stops converging with the other peer until " +
			"you resolve it"
	} else {
		res.Caveat = "a collision here will let the arriving version win on disk; " +
			"yours stays recoverable with `resolve <key> -keep mine`"
	}
	return res, nil
}

// FolderConflictPolicy reads one folder's declared policy.
func (ws *ShellWorkspace) FolderConflictPolicy(folderID string) (string, bool) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return "", false
	}
	f, ok := workbench.LoadFolder(ws.Local.Peer.Store(), folderID)
	if !ok {
		return "", false
	}
	return f.ConflictPolicy(), true
}

// noteConflicts reports unresolved conflicts on an outcome — the ONE
// place either entry point learns about them.
//
// Shared by StatusSnapshot and Reconcile for observeDevice's reason: a
// read and a pass must not be able to describe the same condition
// differently. Reported through Problems because an unresolved conflict
// is a standing state that stays true until an operator acts, and a line
// that appears once and goes quiet is how the whole class became
// invisible.
func (ws *ShellWorkspace) noteConflicts(out *ReconcileOutcome) {
	conflicts, problems := ws.Conflicts()
	for _, c := range conflicts {
		if c.Unresolved() {
			out.Conflicts = append(out.Conflicts, c)
		}
	}
	for _, p := range problems {
		out.Problems = append(out.Problems,
			fmt.Sprintf("conflict record %s — a replaced edit may be unfindable", p))
	}
	if n := len(out.Conflicts); n > 0 {
		out.Problems = append(out.Problems, fmt.Sprintf(
			"%d file(s) where a delivery replaced an edit of yours and you have not "+
				"decided yet — run `conflicts`", n))
	}
	out.ConflictHealth = ws.ConflictHealth()
	if out.ConflictHealth.Storming {
		out.Problems = append(out.Problems, fmt.Sprintf(
			"DELIVERIES ARE BEING REFUSED on this peer: more than %d conflicts in "+
				"%.0fs, so conflict resolution has stopped and %d deliveries were turned "+
				"away. Nothing was overwritten and delivery resumes by itself. A burst "+
				"this size is more likely to be a fault here than someone else's editing",
			out.ConflictHealth.Limit, out.ConflictHealth.WindowSeconds,
			out.ConflictHealth.Refused))
	}
}

// findFolderByIDOrRoot accepts either spelling of "the folder".
//
// A received folder's id is `{owner-peer-id}.{their-root}` and the
// directory the operator actually chose has a different name, so an id is
// built out of a root they never typed. Both work here, because a flag
// that means "the folder" and accepts only one of the two names is a flag
// that is wrong exactly when the two differ — which is the case the
// LocalRoot field exists for.
func (ws *ShellWorkspace) findFolderByIDOrRoot(nameOrID string) (workbench.FolderData, bool) {
	st := ws.Local.Peer.Store()
	if f, ok := workbench.LoadFolder(st, nameOrID); ok {
		return f, true
	}
	folders, _ := workbench.LoadFolders(st)
	for _, f := range folders {
		// ReceivingRoot and never Root — right in the symmetric case and
		// silently wrong in the one that matters.
		if f.ReceivingRoot() == nameOrID || f.Root == nameOrID || f.Label == nameOrID {
			return f, true
		}
	}
	return workbench.FolderData{}, false
}
