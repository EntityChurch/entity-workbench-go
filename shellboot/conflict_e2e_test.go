package shellboot_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"entity-workbench-go/shellcmd"
	"entity-workbench-go/workbench"
)

// M3 step 2: does a delivery that lands on a local edit get NOTICED,
// RECORDED, and UNDONE — and does an ordinary delivery stay free?
//
// Tier: real-session (TESTING-STRATEGY §4). Two peers built by the real
// shellboot.Bootstrap, a real TCP connection, real files on disk.
//
// # What the baseline established, and what this adds
//
// `TestM3Baseline_ConcurrentEditLeavesBothWritesOnTheChain` measured that
// the substrate already meets `DOMAIN-LOCAL-FILES` §1.1a in full: last
// arrival wins on disk, both writes sit at distinct chain positions, and
// the chain says WHICH SIDE authored each — a local edit through the
// watcher (`local/files:watch`), a delivered one through blob-resolve
// (`local/files:write`).
//
// So M3 was never "build a merge engine". It was that the losing version
// sat at a chain position no surface rendered and no verb reached, which
// makes a recoverable loss an unrecoverable one.
//
// # The anti-vacuity arm is the whole test
//
// `SYNC-LIMITS-AND-FAILURE-MODES` §5 rule 4: the detection must be gated
// with an arm proving a strategy that RESOLVES is distinguishable from
// one that merely degrades. A conflict detector that fires on every
// differing hash "detects" every conflict there is and is worthless —
// worse than worthless, because §5's realistic failure is exactly that
// bug, and it is not self-limiting.
//
// So the second half of this test delivers an ordinary change to a file
// the receiver has NOT touched and asserts nothing was recorded. Without
// that arm, `return conflictRecord` unconditionally passes the first half.
func TestM3_ADeliveryThatReplacesALocalEditIsRecordedAndUndoable(t *testing.T) {
	senderDir := t.TempDir()
	receiverDir := t.TempDir()

	sender := newSyncTestPeer(t, "cx-sender")
	receiver := newSyncTestPeer(t, "cx-receiver")
	connectPeers(t, sender, receiver)

	const folder = "contested"
	senderMount := filepath.Join(senderDir, folder)
	receiverMount := filepath.Join(receiverDir, folder)
	mkdirOrFail(t, senderMount)
	mkdirOrFail(t, receiverMount)

	senderOut := mountOrFail(t, sender, senderMount, "archives/"+folder+"/")
	mountOrFail(t, receiver, receiverMount, "archives/"+folder+"/")
	root := senderOut.RootName

	if _, err := receiver.ws.Sync(shellcmd.SyncRequest{Remote: sender.id, Root: root}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Detection reads the chain, so recording has to be on. The
	// reconciler installs this for a folder that receives; installed
	// explicitly here because this test drives Sync directly rather than
	// the whole share flow.
	enableHistoryFor(t, receiver, "local/files/*")

	const contested = "contested.md"
	const untouched = "untouched.md"
	senderContested := filepath.Join(senderMount, contested)
	receiverContested := filepath.Join(receiverMount, contested)
	senderUntouched := filepath.Join(senderMount, untouched)
	receiverUntouched := filepath.Join(receiverMount, untouched)

	// --- 1. Both peers agree on both files. -----------------------------
	const agreed = "shared starting point\n"
	writeOrFail(t, senderContested, agreed)
	writeOrFail(t, senderUntouched, agreed)
	if !awaitFileContent(receiverContested, agreed, 30*time.Second) ||
		!awaitFileContent(receiverUntouched, agreed, 30*time.Second) {
		t.Fatal("precondition: the seed never reached the receiver")
	}

	// --- 2. The receiver edits ONE of them. -----------------------------
	const mine = "the receiver's own edit\n"
	writeOrFail(t, receiverContested, mine)
	if !awaitChainDepth(t, receiver, root, contested, 2, 20*time.Second) {
		t.Fatal("precondition: the receiver's own edit never reached its tree")
	}

	// --- 3. The sender changes BOTH. ------------------------------------
	// One lands on a local edit (a conflict) and one does not (not a
	// conflict). Same delivery path, same pass, different answers — which
	// is the distinction the whole milestone turns on.
	// Both chains, before the sender's edit. The classification reads the
	// HEAD of each, so this is the input to the decision under test.
	logChain(t, receiver, root, contested)
	logChain(t, receiver, root, untouched)

	const theirs = "the sender's edit\n"
	writeOrFail(t, senderContested, theirs)
	writeOrFail(t, senderUntouched, theirs)

	if !awaitFileContent(receiverContested, theirs, 30*time.Second) {
		got, _ := os.ReadFile(receiverContested)
		t.Fatalf("the delivery never landed on the contested file (on disk: %q)", got)
	}
	if !awaitFileContent(receiverUntouched, theirs, 30*time.Second) {
		t.Fatal("the delivery never landed on the untouched file")
	}

	conflicts := awaitConflicts(t, receiver, 1, 20*time.Second)
	if len(conflicts) == 0 {
		t.Fatal("a delivery replaced a local edit and NOTHING was recorded — " +
			"the loss is recoverable from the chain and no surface can say so, " +
			"which is the whole state M3 exists to leave behind")
	}

	// THE ANTI-VACUITY ARM (§5 rule 4). Exactly one conflict, and it is
	// the contested file. A detector that fires on every differing hash
	// would record two here, and would record thousands on any real
	// folder — §5's storm, which is not self-limiting because it is our
	// bug rather than somebody's editing.
	if len(conflicts) != 1 {
		var paths []string
		for _, c := range conflicts {
			paths = append(paths, c.Path)
		}
		t.Fatalf("%d conflicts recorded, want exactly 1 — a delivery to a file "+
			"the receiver never touched was classified as a conflict, which is "+
			"the detection bug SYNC-LIMITS §5 is about:\n  %s",
			len(conflicts), strings.Join(paths, "\n  "))
	}
	c := conflicts[0]
	if !strings.HasSuffix(c.Path, contested) {
		t.Fatalf("the recorded conflict is at %s, want the contested file", c.Path)
	}
	if !c.Recoverable {
		t.Error("the conflict is recorded as NOT recoverable while recording was on")
	}
	if c.RemotePeerID != sender.id {
		t.Errorf("conflict names peer %s, want the sender %s", c.RemotePeerID, sender.id)
	}
	if c.KeepBothPath != "" {
		t.Errorf("a sibling was written at %s under the default policy — the "+
			"default must not change what is in the folder", c.KeepBothPath)
	}
	if entries, _ := os.ReadDir(receiverMount); len(entries) != 2 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("the receiving folder has %d files, want 2: %v — the default "+
			"policy created something", len(entries), names)
	}

	// The surfaces say so, both of them, because a read and a pass must
	// not describe the same condition differently.
	snap, err := receiver.ws.StatusSnapshot()
	if err != nil {
		t.Fatalf("status snapshot: %v", err)
	}
	if len(snap.Conflicts) != 1 {
		t.Errorf("StatusSnapshot carries %d conflicts, want 1", len(snap.Conflicts))
	}
	if !anyContains(snap.Problems, "replaced an edit of yours") {
		t.Errorf("status problems do not mention the conflict:\n  %s",
			strings.Join(snap.Problems, "\n  "))
	}

	// --- 4. Undo it. ----------------------------------------------------
	out, err := receiver.ws.ResolveConflict(c.Key(), workbench.ConflictKeptMine)
	if err != nil {
		t.Fatalf("resolve -keep mine: %v", err)
	}
	if !out.Restored {
		t.Fatal("resolve reported no restore")
	}
	if !awaitFileContent(receiverContested, mine, 20*time.Second) {
		got, _ := os.ReadFile(receiverContested)
		t.Fatalf("the receiver's version was not restored (on disk: %q)", got)
	}

	// --- 5. AND IT STICKS. ----------------------------------------------
	//
	// This is the assertion the naive implementation fails. Restoring
	// through `local/files:write` records the restore with a DELIVERY's
	// provenance, so the next catch-up pass reads that head, concludes
	// the local copy is merely stale, and overwrites the restore —
	// silently, five minutes after the operator asked for the opposite.
	//
	// A catch-up pass is the exact operation that does it, so the test
	// runs one rather than waiting for the supervisor.
	if _, err := receiver.ws.Resync(sender.id, root); err != nil {
		t.Fatalf("resync: %v", err)
	}
	// A second one, because the first could pass by luck of ordering.
	if _, err := receiver.ws.Resync(sender.id, root); err != nil {
		t.Fatalf("second resync: %v", err)
	}
	if got, _ := os.ReadFile(receiverContested); string(got) != mine {
		t.Errorf("a catch-up pass undid the operator's choice: on disk %q, want %q. "+
			"The restore was recorded with a delivery's provenance, or the resolved "+
			"record is not declining the re-delivery", got, mine)
	}

	// The untouched file is still theirs — the resolution touched one
	// file, not the folder.
	if got, _ := os.ReadFile(receiverUntouched); string(got) != theirs {
		t.Errorf("the untouched file is now %q, want %q", got, theirs)
	}

	// The record carries the decision, so a later reader can answer "what
	// happened to this file" rather than only "nothing is wrong now".
	after, _ := receiver.ws.Conflicts()
	if len(after) != 1 || after[0].Kept != workbench.ConflictKeptMine {
		t.Errorf("after resolving, the record set is %v — a resolved conflict is "+
			"KEPT with its decision, not deleted", after)
	}
	t.Logf("MEASURED: 1 conflict on 2 delivered files; restored and survived "+
		"2 catch-up passes; folder still has %d files", 2)
}

// The keep-both policy, which is opt-in and changes what is in the folder.
//
// Separate test because it is a different product behaviour, and running
// it in the same peer as the default would make "the default writes no
// sibling" unassertable.
func TestM3_KeepBothPolicyLeavesBothVersionsInTheFolder(t *testing.T) {
	senderDir := t.TempDir()
	receiverDir := t.TempDir()

	sender := newSyncTestPeer(t, "kb-sender")
	receiver := newSyncTestPeer(t, "kb-receiver")
	connectPeers(t, sender, receiver)

	const folder = "kept"
	senderMount := filepath.Join(senderDir, folder)
	receiverMount := filepath.Join(receiverDir, folder)
	mkdirOrFail(t, senderMount)
	mkdirOrFail(t, receiverMount)

	senderOut := mountOrFail(t, sender, senderMount, "archives/"+folder+"/")
	mountOrFail(t, receiver, receiverMount, "archives/"+folder+"/")
	root := senderOut.RootName

	if _, err := receiver.ws.Sync(shellcmd.SyncRequest{Remote: sender.id, Root: root}); err != nil {
		t.Fatalf("sync: %v", err)
	}
	enableHistoryFor(t, receiver, "local/files/*")

	// The DECLARATION the handler reads. Written through the workspace
	// operation and not by hand, because a test that writes the field
	// directly cannot fail when the operation stops writing it.
	if err := declareReceivedFolder(t, receiver, sender.id, root); err != nil {
		t.Fatalf("declare folder: %v", err)
	}
	folderID := workbench.FolderID(sender.id, root)
	res, err := receiver.ws.SetFolderConflictPolicy(folderID, workbench.ConflictPolicyKeepBoth)
	if err != nil {
		t.Fatalf("set conflict policy: %v", err)
	}
	if !res.Changed {
		t.Fatalf("policy was already %q — this test cannot see what it is named after",
			res.Policy)
	}

	const name = "contested.md"
	senderPath := filepath.Join(senderMount, name)
	receiverPath := filepath.Join(receiverMount, name)

	const agreed = "shared starting point\n"
	writeOrFail(t, senderPath, agreed)
	if !awaitFileContent(receiverPath, agreed, 30*time.Second) {
		t.Fatal("precondition: the seed never arrived")
	}

	const mine = "the receiver's own edit\n"
	writeOrFail(t, receiverPath, mine)
	if !awaitChainDepth(t, receiver, root, name, 2, 20*time.Second) {
		t.Fatal("precondition: the local edit never reached the tree")
	}

	const theirs = "the sender's edit\n"
	writeOrFail(t, senderPath, theirs)
	if !awaitFileContent(receiverPath, theirs, 30*time.Second) {
		t.Fatal("the delivery never landed")
	}

	conflicts := awaitConflicts(t, receiver, 1, 20*time.Second)
	if len(conflicts) != 1 {
		t.Fatalf("%d conflicts recorded, want 1", len(conflicts))
	}
	c := conflicts[0]
	if c.KeepBothPath == "" {
		t.Fatal("keep-both is declared and no sibling was recorded")
	}

	// The sibling is on DISK, under EXTENSION-REVISION §2.3's name, and
	// carries the replaced bytes. Bytes on disk at the far end, not a
	// view model — a sibling that exists in the tree and not in the
	// folder is invisible to the operator it is for.
	sibling := filepath.Join(receiverMount, name+workbench.KeepBothSuffix(c.MineHash))
	if !awaitFileContent(sibling, mine, 20*time.Second) {
		got, err := os.ReadFile(sibling)
		t.Fatalf("the keep-both sibling at %s does not carry the replaced version "+
			"(read %q, err %v)", sibling, got, err)
	}
	if got, _ := os.ReadFile(receiverPath); string(got) != theirs {
		t.Errorf("the original path is %q, want the delivered version — "+
			"EXTENSION-REVISION keeps local at the path, but this pipeline "+
			"delivers to it and preserves local at the sibling", got)
	}

	// A sibling is a real file and must never become a conflict candidate
	// itself — that is how one collision breeds on every pass.
	writeOrFail(t, senderPath, theirs+"again\n")
	if !awaitFileContent(receiverPath, theirs+"again\n", 30*time.Second) {
		t.Fatal("the follow-up delivery never landed")
	}
	all, _ := receiver.ws.Conflicts()
	for _, x := range all {
		if workbench.IsKeepBothPath(x.Path) {
			t.Errorf("a keep-both sibling was itself recorded as a conflict at %s", x.Path)
		}
	}
	// Resolving `theirs` REMOVES the sibling — the only delete in the
	// whole feature, and it removes a file this feature created rather
	// than one the operator put there.
	//
	// Exercised here because `deleteTreeFile` is the one path in
	// conflict_op.go that reaches a handler operation nothing else in
	// this repo's app tier dispatches, and an untested delete is the kind
	// that fails at the moment an operator is tidying up.
	before, _ := receiver.ws.Conflicts()
	var target workbench.ConflictData
	for _, x := range before {
		if x.KeepBothPath != "" && x.Unresolved() {
			target = x
		}
	}
	if target.Path == "" {
		t.Fatal("no unresolved conflict with a sibling to resolve")
	}
	out, err := receiver.ws.ResolveConflict(target.Key(), workbench.ConflictKeptTheirs)
	if err != nil {
		t.Fatalf("resolve -keep theirs: %v", err)
	}
	if out.SiblingRemoved != target.KeepBothPath {
		t.Errorf("resolve reported SiblingRemoved %q, want %q",
			out.SiblingRemoved, target.KeepBothPath)
	}
	if !awaitGone(sibling, 20*time.Second) {
		t.Errorf("the keep-both sibling is still on disk at %s after "+
			"`-keep theirs` — the one delete in this feature does not work", sibling)
	}

	t.Logf("MEASURED: keep-both left %d conflict record(s) and a sibling at %s; "+
		"resolving theirs removed it", len(all), sibling)
}

// awaitGone waits for a path to disappear from disk.
func awaitGone(path string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

// --- helpers -------------------------------------------------------

func writeOrFail(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func awaitConflicts(t *testing.T, p *syncTestPeer, want int, timeout time.Duration) []workbench.ConflictData {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last []workbench.ConflictData
	for time.Now().Before(deadline) {
		last, _ = p.ws.Conflicts()
		if len(last) >= want {
			// Settle briefly so an over-count is CAUGHT rather than raced
			// past: returning at the first sight of `want` would make the
			// anti-vacuity arm pass against a detector that records a
			// second conflict a moment later.
			time.Sleep(1500 * time.Millisecond)
			last, _ = p.ws.Conflicts()
			return last
		}
		time.Sleep(200 * time.Millisecond)
	}
	return last
}

// declareReceivedFolder writes the folder declaration a real `accept`
// would have written, so the handler has a policy to read.
func declareReceivedFolder(t *testing.T, p *syncTestPeer, ownerPeerID, root string) error {
	t.Helper()
	return workbench.SaveFolder(p.ap.Store(), workbench.FolderData{
		ID:     workbench.FolderID(ownerPeerID, root),
		Label:  root,
		Kind:   "files",
		Root:   root,
		Origin: ownerPeerID,
		Mode:   workbench.FolderModeReceive,
	})
}

// logChain prints a path's transition chain, most recent first.
func logChain(t *testing.T, p *syncTestPeer, root, name string) {
	t.Helper()
	treePath := "/" + p.id + "/local/files/" + root + "/" + name
	trans := historyAt(t, p, treePath, 50)
	t.Logf("chain at %s — %d transitions:", name, len(trans))
	for i, tr := range trans {
		t.Logf("  [%d] event=%-8s handler=%s:%s", i, tr.Event, tr.Handler, tr.Operation)
	}
}
