package shellboot_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"entity-workbench-go/shellcmd"
	"entity-workbench-go/workbench"
)

// mode_both_asymmetric_roots_test.go — THE OPERATOR'S SCENARIO, recreated:
// two machines, one two-way folder, and the two directories are NOT
// called the same thing.
//
// Tier: real-session (TESTING-STRATEGY §4). Real listeners, real TCP, real
// grants — `newFlowPeer` sets no OpenAccess, so unlike every other
// mode=both gate in this tree this one also crosses the permission stage
// (AP63). Every file assertion is on BYTES ON DISK at the far end.
//
// # Why this file exists
//
// "The two peers do not have to name the directory the same thing" was a
// stated requirement, and it was not met. `accept` has taken a directory
// since S5 and records the operator's choice in `FolderData.LocalRoot`;
// the reverse leg of a `Mode: both` folder then subscribed to
// `local/files/{OUR root}/*` on a peer whose files are under
// `local/files/{THEIR root}/*`. That subscription is accepted, reports
// healthy, and delivers nothing for as long as it exists.
//
// It was a comment. `shellcmd/reconcile.go` carried it as a KNOWN LIMIT
// with the fix written out in prose — *"a dispatched remote read of their
// folder record would work (AP11; they already grant us
// `system/tree:get`)"* — and nothing failed, because
// `TestModeBoth_OwnerEstablishesTheReverseLeg` asserts that a sync BINDING
// appears and a binding appears whether or not it can carry a byte. That
// is this repo's oldest shape: **a gate on the existence of a mechanism
// rather than on the arrival of the thing the mechanism is for.**
//
// # The control arm is the whole design
//
// `SymmetricRoots` runs the identical scenario with the two directories
// named the same. If the asymmetric arm fails and the symmetric arm
// passes, the root NAME is isolated as the variable and no hypothesis
// about grants, connections, modes or ordering survives. If both fail,
// the reverse leg is broken for a reason that has nothing to do with
// names and this file says so on the first run rather than after a day of
// bisecting.
//
// # What a green run does NOT claim
//
// Two peers, one folder, one file each way, in one process on loopback.
// It says nothing about a restart (`reconcile_restart_test.go`), about
// burst (`queue_depth_load_test.go`), or about the GUI (`twopeer-gui`).

// twoWayAsymmetric is the operator's setup, parameterised on the one
// thing under test: whether the receiving directory has the same basename
// as the sending one.
//
// Returns the two peers, the two on-disk directories, and the folder id —
// which is THE SAME STRING on both peers by construction (S6,
// workbench.FolderID), and is the only reason the owner can name the
// receiver's record at all.
func twoWayAsymmetric(t *testing.T, ctx context.Context, receivingDirName string) (
	alice, bob *flowPeer, aliceDir, bobDir, root, folderID string,
) {
	t.Helper()

	alice = newFlowPeer(t, ctx, "alice")
	bob = newFlowPeer(t, ctx, "bob")

	// Alice owns `entity-share` and mounts it.
	aliceDir = filepath.Join(t.TempDir(), "entity-share")
	if err := os.MkdirAll(aliceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mounted, err := alice.ws.Mount(shellcmd.MountRequest{
		FilesystemDir: aliceDir, TargetPrefix: "archives/entity-share/",
	})
	if err != nil {
		t.Fatalf("alice mount: %v", err)
	}
	root = mounted.RootName

	if _, err := alice.ws.Share(shellcmd.ShareRequest{
		Root: root, Peer: bob.ap.PeerID(), NowMillis: 1_757_000_000_000,
	}); err != nil {
		t.Fatalf("alice share: %v", err)
	}

	// Both dial. A dial-by-address authorizes the DIALER only (AP63), and
	// a two-way folder needs dispatch in both directions — the reverse
	// leg is alice subscribing to bob, which runs over the connection
	// ALICE opened.
	connectVerb(t, bob, "alice", alice.ap.Addr().String())
	connectVerb(t, alice, "bob", bob.ap.Addr().String())

	// Bob accepts into a directory HE names. This is the whole variable:
	// `receivingDirName` is either the same basename as alice's or not.
	bobDir = filepath.Join(t.TempDir(), receivingDirName)
	if _, err := bob.ws.Accept(shellcmd.AcceptRequest{
		Peer: alice.ap.PeerID(), Root: root, Directory: bobDir,
	}); err != nil {
		t.Fatalf("bob accept into %s: %v", bobDir, err)
	}

	folderID = workbench.FolderID(alice.ap.PeerID(), root)

	// BOTH sides declare two-way. Direction is a per-peer property of one
	// folder (S6), so both have to ask for it — alice declaring `both`
	// alone means "I will accept bob's changes" and says nothing about
	// whether bob publishes his.
	for _, p := range []struct {
		name string
		peer *flowPeer
	}{{"alice", alice}, {"bob", bob}} {
		if _, err := p.peer.ws.SetFolderMode(shellcmd.SetFolderModeRequest{
			FolderID: folderID, Mode: workbench.FolderModeBoth,
		}); err != nil {
			t.Fatalf("%s could not declare mode=both on %q: %v", p.name, folderID, err)
		}
	}

	// A mode change rewrites the policy row, and a grant is assembled at
	// HANDSHAKE — so re-dial, in both directions, before expecting either
	// leg to authorize. `SetFolderMode` says so in its caveat and does not
	// do it for you; the peer that DISPATCHES is the peer that must
	// reconnect, and here both dispatch.
	reconnectVerb(t, bob, "alice", alice.ap.Addr().String())
	reconnectVerb(t, alice, "bob", bob.ap.Addr().String())

	for _, p := range []struct {
		name string
		peer *flowPeer
	}{{"alice", alice}, {"bob", bob}} {
		if _, err := p.peer.ws.ReconcileWithTimeout(60 * time.Second); err != nil {
			t.Fatalf("%s reconcile: %v", p.name, err)
		}
	}
	return alice, bob, aliceDir, bobDir, root, folderID
}

// reconnectVerb drops the alias and re-dials, which is what forces a new
// handshake and therefore a re-assembly of the grants. `connect` refuses
// an alias that is already in use rather than silently re-pointing it, so
// the disconnect is not optional.
func reconnectVerb(t *testing.T, from *flowPeer, alias, addr string) {
	t.Helper()
	sh := shellcmd.NewShellInWorkspace(from.ws)
	if _, err := shellcmd.Default().Dispatch(sh, "disconnect", []string{alias}); err != nil {
		t.Fatalf("disconnect %s: %v", alias, err)
	}
	connectVerb(t, from, alias, addr)
}

// assertBothDirections writes one file on each side and requires both to
// land. Forward first, because a forward failure means the pair is broken
// for a reason that is not this test's subject and saying so is worth
// more than a reverse-leg verdict nobody can trust.
func assertBothDirections(t *testing.T, aliceDir, bobDir string, reverseMustWork bool) {
	t.Helper()

	const fwd = "written by the owner\n"
	if err := os.WriteFile(filepath.Join(aliceDir, "from-alice.md"), []byte(fwd), 0o600); err != nil {
		t.Fatal(err)
	}
	if !awaitFileContent(filepath.Join(bobDir, "from-alice.md"), fwd, 60*time.Second) {
		t.Fatalf("FORWARD leg is broken: the owner's file never reached the receiver.\n"+
			"  Nothing about the reverse leg can be concluded from this run.\n"+
			"  alice %s -> bob %s", aliceDir, bobDir)
	}

	const rev = "written by the receiver\n"
	if err := os.WriteFile(filepath.Join(bobDir, "from-bob.md"), []byte(rev), 0o600); err != nil {
		t.Fatal(err)
	}
	landed := filepath.Join(aliceDir, "from-bob.md")
	got := awaitFileContent(landed, rev, 60*time.Second)
	if reverseMustWork && !got {
		t.Fatalf("REVERSE leg delivered nothing: the receiver's file never reached the owner.\n"+
			"  Both peers declared mode=both, both dialled, both reconciled.\n"+
			"  bob %s -> alice %s", bobDir, aliceDir)
	}
	if !reverseMustWork && got {
		t.Fatalf("the reverse leg DELIVERED in the arm documented as broken — "+
			"re-measure before trusting either arm (%s)", landed)
	}
}

// TestModeBoth_SymmetricRoots_ReverseLegDelivers is the CONTROL ARM.
//
// Same scenario, same verbs, same order — the receiving directory just
// happens to have the same basename. If this passes and the asymmetric
// arm below fails, the directory name is the variable and nothing else.
func TestModeBoth_SymmetricRoots_ReverseLegDelivers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	_, _, aliceDir, bobDir, _, _ := twoWayAsymmetric(t, ctx, "entity-share")
	assertBothDirections(t, aliceDir, bobDir, true)
}

// TestModeBoth_AsymmetricRoots_ReverseLegDelivers is the operator's live
// configuration: `entity-share` on one machine, a directory of the
// receiver's own choosing on the other.
//
// Two peers not having to agree on a filesystem path is a REQUIREMENT, not
// a nicety — `accept` takes a directory precisely so the receiver never
// has to be told a name out of band. A reverse leg that only works when
// the names collide has not met it.
func TestModeBoth_AsymmetricRoots_ReverseLegDelivers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	alice, bob, aliceDir, bobDir, root, folderID := twoWayAsymmetric(t, ctx, "entity-downloads")

	// PIN THE PREMISE. If the two roots ever agree, this test is the
	// control arm wearing the asymmetric arm's name and would pass
	// against the defect it exists for (AP58's shape at the level of a
	// value: a fixture that cannot express the case it is named after).
	bobFolder, ok := workbench.LoadFolder(bob.ap.Store(), folderID)
	if !ok {
		t.Fatalf("the receiver holds no record for folder %q", folderID)
	}
	if bobFolder.ReceivingRoot() == root {
		t.Fatalf("premise gone: the receiver's root is %q and the owner's is %q — "+
			"they AGREE, so this run measures the symmetric case",
			bobFolder.ReceivingRoot(), root)
	}
	t.Logf("owner root %q, receiver root %q, folder id %q",
		root, bobFolder.ReceivingRoot(), folderID)

	// And pin that the owner really is trying to receive, so a failure
	// below is about the ROOT NAME rather than about the leg being
	// refused one step earlier.
	aliceFolder, ok := workbench.LoadFolder(alice.ap.Store(), folderID)
	if !ok {
		t.Fatalf("the owner holds no record for folder %q", folderID)
	}
	if !aliceFolder.Receives() {
		t.Fatalf("the owner's folder does not report Receives() (mode %q)",
			aliceFolder.EffectiveMode())
	}

	assertBothDirections(t, aliceDir, bobDir, true)
}
