package shellcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"entity-workbench-go/workbench"
)

// status_two_way_honesty_test.go — WHAT THE FOLDER ROW IS ALLOWED TO SAY.
//
// Tier: model (TESTING-STRATEGY §2). No network: every claim under test is
// one a READ must be able to make from local state alone, which is the
// point — a status surface refreshes, and a pass that dials is not
// something a refresh may do.
//
// # Why this file exists
//
// An operator's report, 2026-09-10, in their words: *"showing two-way,
// showing no problem, meanwhile the whole files aren't getting
// delivered."* Every part of that is reproducible from local state, and
// each part is a separate defect:
//
//   - the row draws `↔ two-way` from OUR OWN declaration, which says
//     nothing about whether the other side agreed or whether any reverse
//     subscription exists;
//   - `FolderStatus.problems()` returns nil for every LOCAL folder that
//     has a mount, so an owner's folder cannot report a problem at all;
//   - an ABSENT mode renders as `both`, while the reconciler reads it as
//     the pre-S6 behaviour — the two disagree, in the direction that
//     flatters;
//   - and the reverse-leg lookup used OUR root name, so a two-way folder
//     between peers with different directory names reported "syncing with
//     nobody" while bytes were arriving.
//
// # The rule these tests enforce
//
// **A folder row states what is ESTABLISHED, not what was DECLARED — and
// where it states a declaration it says so.** A declaration is what the
// operator asked for; the whole purpose of a status surface is to say
// whether they got it. Rendering the ask as though it were the outcome
// makes the surface useless in exactly the case it exists for.
//
// This is D25's shape (a reading must say which operation produced it)
// pushed one level down: not *"was this verified by a pass"* but *"is
// this field an intention or an observation"*.

// twoWayFixture declares one folder we OWN, mounted, shared with one peer,
// set to `both` — and establishes NOTHING. That is the exact durable state
// after an operator shares a folder and presses "Make two-way", which is
// the state the report above was made in.
func twoWayFixture(t *testing.T) (*ShellWorkspace, *workbench.Store, string) {
	t.Helper()
	ws, st := mountableFixture(t)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mounted, err := ws.Mount(MountRequest{
		FilesystemDir: dir, TargetPrefix: "archives/shared/",
	})
	if err != nil {
		t.Fatalf("mount: %v", err)
	}

	id := workbench.FolderID(ws.Local.Peer.PeerID(), mounted.RootName)
	f := workbench.FolderData{
		ID: id, Label: mounted.RootName, Kind: "files",
		Root: mounted.RootName, Path: dir, Origin: "local",
		Mode: workbench.FolderModeBoth,
		SharedWith: []workbench.FolderPeerData{
			{PeerID: themPeer, State: workbench.FolderStateOffered, AtMillis: 1},
		},
	}
	if err := workbench.SaveFolder(st, f); err != nil {
		t.Fatalf("save folder: %v", err)
	}
	return ws, st, id
}

func folderStatusByID(t *testing.T, ws *ShellWorkspace, id string) FolderStatus {
	t.Helper()
	snap, _ := ws.StatusSnapshot()
	for _, f := range snap.Folders {
		if f.ID == id {
			return f
		}
	}
	t.Fatalf("no folder %q in the snapshot (%d folders)", id, len(snap.Folders))
	return FolderStatus{}
}

// TestStatus_TwoWayWithNoReverseLeg_IsAProblem is the operator's report,
// reduced to its smallest form.
//
// The folder is ours, mounted, declared `both`, shared with a peer, and
// nothing is established. A surface may say we ASKED for two-way. It may
// not say we HAVE it, and it must not be silent.
func TestStatus_TwoWayWithNoReverseLeg_IsAProblem(t *testing.T) {
	ws, _, id := twoWayFixture(t)

	fs := folderStatusByID(t, ws, id)
	if !fs.Local || !fs.Mounted {
		t.Fatalf("premise gone: expected a local, mounted folder, got local=%v mounted=%v",
			fs.Local, fs.Mounted)
	}
	if len(fs.SyncingWith) != 0 {
		t.Fatalf("premise gone: the fixture establishes nothing, yet the folder "+
			"reports syncing with %v", fs.SyncingWith)
	}

	probs := fs.problems()
	if len(probs) == 0 {
		t.Fatalf("a folder declared %q, shared with a peer, with NO reverse "+
			"subscription to anyone, reports no problem at all.\n"+
			"  This is the operator's report: \"showing two-way, showing no "+
			"problem, meanwhile the whole files aren't getting delivered.\"\n"+
			"  FolderStatus.problems() returns nil for every local folder that "+
			"has a mount, so an owner's folder can never say anything is wrong.",
			workbench.FolderModeBoth)
	}
	joined := strings.Join(probs, " ")
	if !strings.Contains(joined, themPeer) {
		t.Errorf("the problem does not name the peer it is about: %q", joined)
	}
}

// TestStatus_SendOnlyFolderIsNotAProblem is the CONTROL ARM, and without
// it the test above is satisfied by making every local folder complain.
//
// A send-only folder with no reverse subscription is CORRECT — that is
// what send-only means. A surface that flags it has invented an alarm,
// which trains an operator to ignore the one that matters.
func TestStatus_SendOnlyFolderIsNotAProblem(t *testing.T) {
	ws, st, id := twoWayFixture(t)

	f, ok := workbench.LoadFolder(st, id)
	if !ok {
		t.Fatalf("no folder %q", id)
	}
	f.Mode = workbench.FolderModeSend
	if err := workbench.SaveFolder(st, f); err != nil {
		t.Fatal(err)
	}

	fs := folderStatusByID(t, ws, id)
	if probs := fs.problems(); len(probs) != 0 {
		t.Fatalf("a %q folder with no reverse subscription reported a problem: %v\n"+
			"  Nothing is wrong: send-only means we publish and take nothing back.",
			workbench.FolderModeSend, probs)
	}
}

// TestStatus_AbsentModeIsNeverReportedAsTwoWay.
//
// `EffectiveMode` reads an absent Mode as the PRE-S6 behaviour — a local
// folder publishes — and AGENTS.md states plainly that defaulting it to
// `both` is the mistake that is not symmetric. The renderer defaulted a
// blank mode to `"both"` anyway, so a pre-S6 record drew `↔ two-way`
// while the reconciler treated it as send-only.
//
// The fix is that the STATUS carries the effective mode, so no renderer
// has to default anything. A default that lives in a renderer is a second
// answer to a question the model already answers.
func TestStatus_AbsentModeIsNeverReportedAsTwoWay(t *testing.T) {
	ws, st, id := twoWayFixture(t)

	f, ok := workbench.LoadFolder(st, id)
	if !ok {
		t.Fatalf("no folder %q", id)
	}
	// SaveFolder normalizes through EffectiveMode, so write the record
	// directly to reproduce what a pre-S6 store actually holds.
	f.Mode = ""
	if _, err := st.Put(workbench.FolderPrefix+f.ID, workbench.FolderType, f); err != nil {
		t.Fatalf("put pre-S6 record: %v", err)
	}
	if got, _ := workbench.LoadFolder(st, id); got.Mode != "" {
		t.Fatalf("premise gone: the record still carries mode %q", got.Mode)
	}

	fs := folderStatusByID(t, ws, id)
	if fs.Mode == workbench.FolderModeBoth {
		t.Fatalf("a folder record with NO declared mode reports %q.\n"+
			"  EffectiveMode reads an absent mode as the pre-S6 behaviour — a local "+
			"folder PUBLISHES — so this is the reconciler and the surface "+
			"disagreeing, in the direction that flatters.", fs.Mode)
	}
	if fs.Mode == "" {
		t.Fatalf("the status carries an EMPTY mode, which forces every renderer to " +
			"default it — and the renderer that did chose `both`. Carry the " +
			"effective mode so nobody downstream has to guess.")
	}
	if fs.Mode != workbench.FolderModeSend {
		t.Fatalf("an absent mode on a LOCAL folder should read as %q, got %q",
			workbench.FolderModeSend, fs.Mode)
	}
}

// TestStatus_ReverseLegIsSeenWhenTheirRootDiffers.
//
// The reverse-leg subscription is keyed on the SENDER's root — theirs —
// because that is the prefix being watched. `observeFolder` looked it up
// under `f.Root`, which is OURS. So a working two-way folder between two
// peers who named the directory differently reported syncing with nobody,
// while bytes arrived.
//
// This is the two-root-names trap landing on the STATUS surface after
// being fixed in the reconciler, which is the shape worth remembering: a
// fix that changes what a record is keyed on has to move every reader of
// that key, and the readers that merely DISPLAY are the ones nobody
// thinks of.
func TestStatus_ReverseLegIsSeenWhenTheirRootDiffers(t *testing.T) {
	ws, st, id := twoWayFixture(t)

	f, ok := workbench.LoadFolder(st, id)
	if !ok {
		t.Fatalf("no folder %q", id)
	}
	theirRoot := "their-different-name"
	if theirRoot == f.Root {
		t.Fatal("premise gone: the two roots must differ for this test to mean anything")
	}
	// Exactly what the reconciler's reverse leg writes: keyed on THEIR
	// root, targeting OUR mount.
	if err := workbench.SaveSyncBinding(st, workbench.SyncBindingData{
		RemotePeerID:   themPeer,
		Root:           theirRoot,
		SourcePrefix:   "local/files/" + theirRoot + "/",
		TargetPrefix:   "local/files/" + f.Root + "/",
		SubscriptionID: "sub-test-1",
	}); err != nil {
		t.Fatalf("save sync binding: %v", err)
	}

	fs := folderStatusByID(t, ws, id)
	found := false
	for _, p := range fs.SyncingWith {
		if p == themPeer {
			found = true
		}
	}
	if !found {
		t.Fatalf("the reverse leg IS established (a sync binding from %s targets our "+
			"mount) and the folder reports syncing with %v.\n"+
			"  The lookup is keyed on OUR root %q; the binding is keyed on THEIRS "+
			"%q. A working two-way folder therefore renders as connected to "+
			"nobody whenever the two machines named the directory differently — "+
			"which `accept` exists to allow.",
			themPeer, fs.SyncingWith, f.Root, theirRoot)
	}
	if probs := fs.problems(); len(probs) != 0 {
		t.Fatalf("the reverse leg is established and the folder still reports a "+
			"problem: %v", probs)
	}
}
