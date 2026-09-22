package shellboot_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"entity-workbench-go/shellcmd"
	"entity-workbench-go/workbench"
)

// S3 — accept takes a DIRECTORY, and that deletes two steps of the flow.
//
// Tier: real-session (TESTING-STRATEGY §4). No OpenAccess on either side.
//
// # What this is gating
//
// The nine-step flow had the receiver mount a directory of their own
// before accepting, because `sync` refuses without a local mount — a
// refusal that is correct (a sync with no mount 404s on every delivery
// while reporting itself healthy) and whose cost landed on the operator.
// Worse, the mount root is derived from the directory's BASENAME and the
// sync was keyed on a single shared root name, so the receiver had to
// create a directory named exactly what the sender happened to call
// theirs. Nothing said so. Picking a sensible local name silently
// produced a relationship that could not deliver.
//
// So this test does the thing an operator would do — receive `alicework`
// into a directory called something else entirely, having mounted
// nothing — and requires the bytes to arrive.
//
// # Why it asserts on the FILE and not on the outcome struct
//
// Every intermediate signal here is one this repo has already shipped a
// green version of while the folder stayed empty: a sync row that lists
// as healthy over a dead mount, an accept that reports the right target
// prefix and then 404s, a subscription with no backfill behind it. The
// only assertion that cannot be satisfied by a broken build is bytes on
// disk under the directory the operator named.
func TestAccept_WithADirectory_MountsAndReceives(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	alice := newFlowPeer(t, ctx, "alice")
	bob := newFlowPeer(t, ctx, "bob")

	connectVerb(t, bob, "alice", alice.ap.Addr().String())

	// Alice mounts and shares a folder that ALREADY HAS A FILE IN IT.
	// Pre-existing content is the case AP65 was about: a subscription is
	// a future tense, so without the backfill this arrives as nothing at
	// all and every surface reports success.
	const folder = "alicework"
	aliceDir := filepath.Join(t.TempDir(), folder)
	if err := os.MkdirAll(aliceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(aliceDir, "report.md"),
		[]byte("# already here\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	aliceMount, err := alice.ws.Mount(shellcmd.MountRequest{
		FilesystemDir: aliceDir, TargetPrefix: "archives/" + folder + "/",
	})
	if err != nil {
		t.Fatalf("alice mount: %v", err)
	}
	root := aliceMount.RootName

	if _, err := alice.ws.Share(shellcmd.ShareRequest{
		Root: root, Peer: bob.ap.PeerID(), NowMillis: 1_756_000_000_000,
	}); err != nil {
		t.Fatalf("alice share: %v", err)
	}

	// --- the step under test ------------------------------------------
	//
	// Bob has mounted NOTHING. The directory does not exist yet, and its
	// name deliberately does not match alice's root — the old flow could
	// not express this at all.
	bobDir := filepath.Join(t.TempDir(), "from-alice")
	acceptOut, err := bob.ws.Accept(shellcmd.AcceptRequest{
		Peer: alice.ap.PeerID(), Root: root, Directory: bobDir,
	})
	if err != nil {
		t.Fatalf("bob accept with a directory: %v", err)
	}

	if acceptOut.Mounted == nil {
		t.Fatal("accept did not report creating a mount, but bob had none — " +
			"if it reused one, the fixture is not testing what it claims")
	}
	if acceptOut.Mounted.FilesystemRoot != bobDir {
		t.Errorf("accept mounted %q, want the directory it was given (%q)",
			acceptOut.Mounted.FilesystemRoot, bobDir)
	}
	if acceptOut.LocalRoot == root {
		t.Errorf("LocalRoot is %q, the SENDER's root name — this test exists because the "+
			"two must be allowed to differ, so a run where they match measures nothing",
			acceptOut.LocalRoot)
	}
	if acceptOut.Sync.TargetRoot != acceptOut.LocalRoot {
		t.Errorf("the sync writes into root %q but the accept mounted %q — the bytes will "+
			"land somewhere the operator did not choose, or nowhere",
			acceptOut.Sync.TargetRoot, acceptOut.LocalRoot)
	}

	// --- the only assertion that cannot be faked ----------------------
	landed := filepath.Join(bobDir, "report.md")
	if !awaitFile(landed, "# already here\n", 30*time.Second) {
		got, rerr := os.ReadFile(landed)
		t.Fatalf("the file alice was already sharing never arrived at %s (read: %q, %v)\n"+
			"  backfill: %+v (skipped=%v)",
			landed, string(got), rerr, acceptOut.Sync.Backfill, acceptOut.Sync.BackfillSkipped)
	}

	// The declaration must carry the local root, or the reconciler looks
	// for a mount named after ALICE's folder on the next pass, does not
	// find one, and reports a healthy sync as broken.
	id := workbench.ReceivedFolderID(alice.ap.PeerID(), root)
	f, ok := workbench.LoadFolder(bob.ap.Store(), id)
	if !ok {
		t.Fatal("no folder declaration was written for the accepted folder")
	}
	if f.ReceivingRoot() != acceptOut.LocalRoot {
		t.Errorf("the declaration says the bytes land in %q; they actually land in %q",
			f.ReceivingRoot(), acceptOut.LocalRoot)
	}

	// And one reconcile pass must find it settled rather than reporting a
	// missing mount — the pass runs at every startup, so a false problem
	// here is what the operator sees on every launch forever.
	res, err := bob.ws.Reconcile(ctx)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	for _, p := range res.Problems {
		if strings.Contains(p, "no mount") {
			t.Errorf("reconcile reports a missing mount for a folder that is receiving "+
				"files right now: %s", p)
		}
	}
}

// TestAccept_RefusesANonEmptyDirectoryUnlessTold.
//
// Incoming files overwrite same-named local ones and remote deletes
// propagate, so pointing an accept at a directory that already has
// contents is destructive. The operator's mental model at that moment is
// "choose somewhere to put these", not "choose something to merge with",
// and the gap between those two is somebody's documents folder.
//
// The refusal carries the count as STRUCTURE, so a panel can say what is
// in the way instead of printing a sentence at them.
func TestAccept_RefusesANonEmptyDirectoryUnlessTold(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	bob := newFlowPeer(t, ctx, "bob")

	occupied := filepath.Join(t.TempDir(), "my-documents")
	if err := os.MkdirAll(occupied, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"taxes.md", "novel.md"} {
		if err := os.WriteFile(filepath.Join(occupied, name), []byte("mine\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	const them = "2KLf7osYcMLEdmSnYLx3Bg6TScaGJefLZJdKaL1tPNHAbR"
	_, err := bob.ws.Accept(shellcmd.AcceptRequest{
		Peer: them, Root: "theirfolder", Directory: occupied,
	})
	if err == nil {
		t.Fatal("accepting into a directory with two files in it was allowed")
	}
	ne, ok := shellcmd.AsNonEmptyDirectory(err)
	if !ok {
		t.Fatalf("the refusal is not a typed NonEmptyDirectory, so a panel cannot offer to "+
			"proceed and has to parse prose: %v", err)
	}
	if ne.Entries != 2 {
		t.Errorf("NonEmptyDirectory.Entries = %d, want 2", ne.Entries)
	}

	// Nothing may have been established by a refused accept. This is the
	// half that matters: the check runs BEFORE any durable write, so a
	// refusal leaves no grant, no declaration and no mount behind.
	if _, ok := workbench.LoadAccessPolicy(bob.ap.Store(), them); ok {
		t.Error("a refused accept still granted the peer a delivery policy")
	}
	if _, ok := workbench.LoadFolder(bob.ap.Store(),
		workbench.ReceivedFolderID(them, "theirfolder")); ok {
		t.Error("a refused accept still declared the folder")
	}

	// And with permission it proceeds. The sync leg fails — there is no
	// such peer — but the directory decision is the thing under test and
	// it must be past by then.
	_, err = bob.ws.Accept(shellcmd.AcceptRequest{
		Peer: them, Root: "theirfolder", Directory: occupied, AllowNonEmpty: true,
	})
	if _, still := shellcmd.AsNonEmptyDirectory(err); still {
		t.Error("AllowNonEmpty did not override the refusal")
	}
	if _, ok := workbench.LoadAccessPolicy(bob.ap.Store(), them); !ok {
		t.Error("with AllowNonEmpty the accept should have got as far as writing the grant")
	}
}

// awaitFile polls for a file to appear with the expected contents.
// Polling, not a subscription: what is under test is whether the whole
// chain produced bytes, and a subscription would be a second thing that
// could be broken.
func awaitFile(path, want string, within time.Duration) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil && string(b) == want {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}
