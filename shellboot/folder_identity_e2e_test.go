package shellboot_test

import (
	"os"
	"path/filepath"
	"testing"

	"entity-workbench-go/shellcmd"
	"entity-workbench-go/workbench"
)

// folder_identity_e2e_test.go — S6: a folder is ONE object across two
// peers, and direction is a property of it.
//
// Tier: real-session (TESTING-STRATEGY §4).
//
// # The defect
//
// A folder had no identity across peers. `share` wrote
// `app/workbench/folders/{root}`; `accept` wrote
// `app/workbench/folders/{owner}.{their-root}`. Different ids, different
// roots, no shared name, nothing joining them — so "this folder" was not
// a thing either side could name, and a bidirectional share could only
// be two unrelated one-way pipes pointed at different directories. The
// operator's report was *"bilateral transfer to different locations, but
// they don't have the same understanding"*, which is an exact
// description of the records.
//
// # What this gate asserts, and why it is the pair of them
//
// The id assertion alone is satisfiable by any two strings that happen
// to match, so it is paired with the direction assertion: the same
// record, on both sides, decides which way bytes flow. Together they are
// the claim "one object, two views".
//
// Note this file uses `OpenAccess: true` via newSyncTestPeer, so per
// AP63 it says NOTHING about the permission stage — the policy union is
// gated in `shellcmd/reconcile_test.go` and the no-wildcard flow in
// `flow_e2e_test.go`.
func TestFolderIdentity_IsTheSameStringOnBothPeers(t *testing.T) {
	alice, bob, root := shareOneFolder(t)

	aliceFolders, problems := workbench.LoadFolders(alice.ap.Store())
	if len(problems) > 0 {
		t.Fatalf("alice's folder records: %v", problems)
	}
	bobFolders, problems := workbench.LoadFolders(bob.ap.Store())
	if len(problems) > 0 {
		t.Fatalf("bob's folder records: %v", problems)
	}

	aliceIDs := folderIDs(aliceFolders)
	bobIDs := folderIDs(bobFolders)

	// The one string both sides must hold. Derived here from the facts
	// the test itself supplied rather than read from either peer, so a
	// bug that made both sides agree on the WRONG id still fails.
	want := workbench.FolderID(alice.id, root)

	if !contains(aliceIDs, want) {
		t.Errorf("the sharing peer does not hold the shared folder id %q.\n"+
			"  it has: %v\n"+
			"  (pre-S6 it wrote the bare root, which shares no string with the "+
			"receiver's record)", want, aliceIDs)
	}
	if !contains(bobIDs, want) {
		t.Errorf("the receiving peer does not hold the shared folder id %q.\n"+
			"  it has: %v", want, bobIDs)
	}

	// The claim stated as the operator would: there is an id in common.
	if !anyShared(aliceIDs, bobIDs) {
		t.Fatalf("the two peers share NO folder id — a folder is still not one "+
			"object across peers.\n  alice: %v\n  bob:   %v", aliceIDs, bobIDs)
	}

	// Anti-vacuity: the PRE-S6 id must be gone. Without this the
	// assertions above are also satisfied by a build that writes both
	// forms, which is the shape a careless migration produces — two
	// records describing one folder, agreeing until they diverge.
	if contains(aliceIDs, root) {
		t.Errorf("the bare-root id %q is still present alongside the shared one; "+
			"two records now describe one folder", root)
	}
}

// TestFolderMode_IsReadAndNotJustStored — the AP67 half.
//
// `FolderData.Mode` shipped with two writers, two readers, and nothing
// branching on it: both readers put it in a status DTO. It was stored
// faithfully, displayed faithfully, and consulted nowhere, so no test at
// any layer could fail on it — round-tripping a value is exactly what it
// did correctly. The tell worth carrying is **a field whose only readers
// are serializers**.
//
// So this asserts on an OBSERVABLE CONSEQUENCE of the mode rather than
// on the stored value: a receive-only folder must not put us in the set
// of peers we pull from, and a send-only one must not either. Reading
// the field back would pass against the version this test exists for.
func TestFolderMode_IsReadAndNotJustStored(t *testing.T) {
	alice, bob, root := shareOneFolder(t)
	id := workbench.FolderID(alice.id, root)

	// Alice owns it. Put her side send-only and the reconciler must not
	// subscribe back to bob.
	if _, err := alice.ws.SetFolderMode(shellcmd.SetFolderModeRequest{
		FolderID: id, Mode: workbench.FolderModeSend,
	}); err != nil {
		t.Fatalf("set send-only: %v", err)
	}
	f, ok := workbench.LoadFolder(alice.ap.Store(), id)
	if !ok {
		t.Fatalf("folder %q vanished", id)
	}
	if f.Receives() {
		t.Errorf("a send-only folder reports that it receives")
	}
	if !f.Publishes() {
		t.Errorf("a send-only folder reports that it does not publish")
	}

	// Control arm. Without it the assertions above pass against a
	// Receives() that returns false unconditionally — which is a real
	// way to get this wrong, and the more likely one.
	if _, err := alice.ws.SetFolderMode(shellcmd.SetFolderModeRequest{
		FolderID: id, Mode: workbench.FolderModeBoth,
	}); err != nil {
		t.Fatalf("set both: %v", err)
	}
	f, _ = workbench.LoadFolder(alice.ap.Store(), id)
	if !f.Receives() || !f.Publishes() {
		t.Errorf("control arm: a `both` folder does not report both directions "+
			"(publishes=%v receives=%v) — the assertions above prove nothing",
			f.Publishes(), f.Receives())
	}

	// And the mode is refused rather than silently coerced, because a
	// misspelling that became `both` would publish an operator's disk
	// when they asked for receive-only (AP33).
	if _, err := alice.ws.SetFolderMode(shellcmd.SetFolderModeRequest{
		FolderID: id, Mode: "sendish",
	}); err == nil {
		t.Errorf("an unrecognised mode was accepted instead of refused")
	}
	_ = bob
}

// TestFolderIDMigration_MovesAPreS6Record — the upgrade path.
//
// Every folder already on an operator's disk is keyed on the bare root.
// Left alone it would never join the receiver's record, so the defect
// would persist for exactly the people who already have data.
func TestFolderIDMigration_MovesAPreS6Record(t *testing.T) {
	alice := newSyncTestPeer(t, "alice")
	st := alice.ap.Store()

	// A pre-S6 record, written the way the old code wrote it.
	legacy := workbench.FolderData{
		ID: "photos", Label: "photos", Kind: "files",
		Root: "photos", Path: "/tmp/photos", Origin: "local",
	}
	if err := workbench.SaveFolder(st, legacy); err != nil {
		t.Fatal(err)
	}

	moved, problems := workbench.MigrateFolderIDs(st, alice.id)
	if len(problems) > 0 {
		t.Fatalf("migration problems: %v", problems)
	}
	if len(moved) != 1 {
		t.Fatalf("expected one folder to move, got %v", moved)
	}
	want := workbench.FolderID(alice.id, "photos")
	if _, ok := workbench.LoadFolder(st, want); !ok {
		t.Errorf("the migrated folder is not at its shared id %q", want)
	}
	if _, ok := workbench.LoadFolder(st, "photos"); ok {
		t.Errorf("the pre-S6 record is still there — two records now describe one folder")
	}

	// Idempotent: a second pass moves nothing. A migration that runs at
	// every bootstrap and is not idempotent rewrites an operator's
	// declarations forever.
	moved2, problems2 := workbench.MigrateFolderIDs(st, alice.id)
	if len(moved2) != 0 || len(problems2) != 0 {
		t.Errorf("second migration pass was not a no-op: moved=%v problems=%v", moved2, problems2)
	}

	// A migrated pre-S6 LOCAL record keeps publishing and does not start
	// receiving. Defaulting an absent mode to `both` — the first version
	// of this change — silently began publishing folders an operator had
	// only ever accepted.
	f, _ := workbench.LoadFolder(st, want)
	if !f.Publishes() {
		t.Errorf("a migrated local folder stopped publishing")
	}
	if f.Receives() {
		t.Errorf("a migrated local folder started receiving — an absent mode must "+
			"mean the pre-S6 behaviour, not `both` (mode is now %q)", f.Mode)
	}
}

// shareOneFolder is the two-peer fixture: alice mounts and shares a
// folder, bob accepts it into a directory of his own choosing.
func shareOneFolder(t *testing.T) (alice, bob *syncTestPeer, root string) {
	t.Helper()
	alice = newSyncTestPeer(t, "alice")
	bob = newSyncTestPeer(t, "bob")
	connectPeers(t, alice, bob)

	dir := filepath.Join(t.TempDir(), "photos")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "one.md"), []byte("# one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mounted, err := alice.ws.Mount(shellcmd.MountRequest{
		FilesystemDir: dir, TargetPrefix: "archives/photos/",
	})
	if err != nil {
		t.Fatalf("alice mount: %v", err)
	}
	root = mounted.RootName

	if _, err := alice.ws.Share(shellcmd.ShareRequest{Peer: bob.id, Root: root}); err != nil {
		t.Fatalf("alice share: %v", err)
	}
	// Bob receives into a DIFFERENTLY NAMED directory on purpose: the
	// two roots are only the same by coincidence, and a fixture where
	// they agree cannot fail on the field that exists for the case where
	// they do not (FolderData.LocalRoot).
	into := filepath.Join(t.TempDir(), "from-alice")
	if _, err := bob.ws.Accept(shellcmd.AcceptRequest{
		Peer: alice.id, Root: root, Directory: into,
	}); err != nil {
		t.Fatalf("bob accept: %v", err)
	}
	return alice, bob, root
}

func folderIDs(fs []workbench.FolderData) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.ID)
	}
	return out
}

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

func anyShared(a, b []string) bool {
	for _, x := range a {
		if contains(b, x) {
			return true
		}
	}
	return false
}
