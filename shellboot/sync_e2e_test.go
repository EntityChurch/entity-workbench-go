package shellboot_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/shellboot"
	"entity-workbench-go/shellcmd"
)

// M2: two peers, one folder, end to end — THROUGH THE SHIPPED VERBS.
//
// Tier: real-session (TESTING-STRATEGY §4).
//
// # Why this test is not a duplicate of the Stage 3 cases
//
// `cmd_stage3_case1_5_subscription_test.go` and its siblings already
// prove the cross-peer chain works. They also each hand-assemble it:
// construct the handler, call `RegisterMount`, mint the chain capability,
// build the deliver URI, call `SubscribeRawAt`. Every one of those
// registrations lives in a `_test.go`, which is exactly how the whole
// pipeline came to be built, tested, and reachable from nothing a user
// could run — `shellboot` registered two handlers and this was not one.
//
// So the assertion here is deliberately not "the chain works". It is
// **an operator can cause the chain to exist**: mount on both sides with
// the `mount` verb, `sync` on the receiver, write a file on the sender,
// and find it on the receiver's disk. Nothing in the body reaches past a
// shipped surface into the plumbing.
//
// Verified to fail before it was trusted: removing the blob-resolve
// registration from shellboot — the pre-M2 state — fails this at
// newSyncTestPeer with "shellboot did not wire the blob-resolve
// handler", which is the honest reproduction of "the engine exists and
// nothing can reach it".
func TestM2_TwoPeers_OneFolder_ThroughTheVerbs(t *testing.T) {
	senderDir := t.TempDir()
	receiverDir := t.TempDir()

	sender := newSyncTestPeer(t, "sender")
	receiver := newSyncTestPeer(t, "receiver")

	connectPeers(t, sender, receiver)

	// Both sides mount, through the verb. The root name is derived from
	// the directory basename, so the two temp dirs are renamed to share
	// one — which is also the realistic case (the same folder name on
	// two machines).
	const folder = "shared"
	senderMount := filepath.Join(senderDir, folder)
	receiverMount := filepath.Join(receiverDir, folder)
	if err := os.MkdirAll(senderMount, 0o755); err != nil {
		t.Fatalf("mkdir sender mount: %v", err)
	}
	if err := os.MkdirAll(receiverMount, 0o755); err != nil {
		t.Fatalf("mkdir receiver mount: %v", err)
	}

	senderOut, err := sender.ws.Mount(shellcmd.MountRequest{
		FilesystemDir: senderMount,
		TargetPrefix:  "archives/" + folder + "/",
	})
	if err != nil {
		t.Fatalf("sender mount: %v", err)
	}
	receiverOut, err := receiver.ws.Mount(shellcmd.MountRequest{
		FilesystemDir: receiverMount,
		TargetPrefix:  "archives/" + folder + "/",
	})
	if err != nil {
		t.Fatalf("receiver mount: %v", err)
	}
	if senderOut.RootName != receiverOut.RootName {
		t.Fatalf("the two mounts derived different root names (%q vs %q); this test's "+
			"premise is that they match", senderOut.RootName, receiverOut.RootName)
	}
	root := senderOut.RootName

	// The verb under test.
	syncOut, err := receiver.ws.Sync(shellcmd.SyncRequest{
		Remote: sender.id,
		Root:   root,
	})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if syncOut.SubscriptionID == "" {
		t.Fatal("sync reported no subscription id")
	}

	// It shows up as established, and says which peer it is from. A
	// relationship a surface cannot list is one an operator cannot
	// undo.
	rows, problems := receiver.ws.Syncs()
	if len(problems) > 0 {
		t.Fatalf("syncs reported undecodable bindings: %v", problems)
	}
	if len(rows) != 1 {
		t.Fatalf("syncs = %d rows, want 1", len(rows))
	}
	if rows[0].RemotePeerID != sender.id || rows[0].Root != root {
		t.Fatalf("sync row = %+v, want remote %s root %s", rows[0], sender.id, root)
	}
	if !rows[0].Live {
		t.Error("a sync established in this process reports Live=false")
	}

	// The actual claim: a file written on one machine appears on the
	// other. Written AFTER the sync, because the subscription fires on
	// change and does not replay history — which is the thing the verb's
	// own output says out loud.
	const body = "# M2\n\nOne folder, two peers, through the shipped verbs.\n"
	if err := os.WriteFile(filepath.Join(senderMount, "note.md"), []byte(body), 0o600); err != nil {
		t.Fatalf("write on sender: %v", err)
	}

	landed := filepath.Join(receiverMount, "note.md")
	if !awaitFileContent(landed, body, 30*time.Second) {
		got, rerr := os.ReadFile(landed)
		t.Fatalf("note.md never arrived at %s (read: %q, err: %v)\n"+
			"  sender source prefix:   %s\n"+
			"  receiver target prefix: %s\n"+
			"  subscription:           %s",
			landed, string(got), rerr,
			syncOut.SourcePrefix, syncOut.TargetPrefix, syncOut.SubscriptionID)
	}

	// A second file, to distinguish "the first delivery worked" from "the
	// relationship is live". A one-shot chain that fires once and stops
	// would pass everything above.
	const body2 = "second\n"
	if err := os.WriteFile(filepath.Join(senderMount, "again.txt"), []byte(body2), 0o600); err != nil {
		t.Fatalf("write second file on sender: %v", err)
	}
	if !awaitFileContent(filepath.Join(receiverMount, "again.txt"), body2, 30*time.Second) {
		t.Fatal("the second file never arrived — the first delivery worked and the " +
			"relationship did not stay live")
	}

	// Teardown is reachable too, and does not delete what arrived.
	if _, err := receiver.ws.Unsync(sender.id, root); err != nil {
		t.Fatalf("unsync: %v", err)
	}
	if rows, _ := receiver.ws.Syncs(); len(rows) != 0 {
		t.Errorf("after unsync, syncs = %d rows, want 0", len(rows))
	}
	if _, err := os.Stat(landed); err != nil {
		t.Errorf("unsync removed a file that had already arrived: %v", err)
	}
}

// TestM2_SyncRefusesWithoutALocalMount pins the refusal, because the
// alternative is the failure this repo has now shipped twice: a
// relationship that establishes cleanly and then 404s on every delivery,
// listing as healthy the whole time.
func TestM2_SyncRefusesWithoutALocalMount(t *testing.T) {
	sender := newSyncTestPeer(t, "sender")
	receiver := newSyncTestPeer(t, "receiver")
	connectPeers(t, sender, receiver)

	_, err := receiver.ws.Sync(shellcmd.SyncRequest{
		Remote: sender.id,
		Root:   "nothing-is-mounted-here",
	})
	if err == nil {
		t.Fatal("sync succeeded with no local mount to receive into")
	}
	if !strings.Contains(err.Error(), "no local mount") {
		t.Errorf("refusal does not name the cause: %v", err)
	}
}

// --- helpers -------------------------------------------------------

// syncTestPeer is a peer built by the REAL bootstrap.
//
// This is the load-bearing detail of the whole file, and the first
// version of it got this wrong: it constructed its own handler list
// "the way shellboot builds one", which meant deleting the blob-resolve
// registration from shellboot would have left this test green. That is
// the same defect the test exists to prevent, one layer up — a fixture
// that reimplements the thing under test cannot fail when the thing
// under test changes. It calls shellboot.Bootstrap now, so the
// assertion is about the shipped wiring and nothing else.
type syncTestPeer struct {
	ap *entitysdk.AppPeer
	ws *shellcmd.ShellWorkspace
	id string
}

func newSyncTestPeer(t *testing.T, name string) *syncTestPeer {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	ap, ws, err := shellboot.Bootstrap(ctx, shellboot.Config{
		LocalAlias: name,
		ListenAddr: "127.0.0.1:0",
		OpenAccess: true,
	})
	if err != nil {
		t.Fatalf("Bootstrap %s: %v", name, err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	// The registration this whole milestone is about. Asserted here
	// rather than only implied by the sync succeeding, so a regression
	// names itself instead of surfacing as a mysterious timeout.
	if ws.BlobResolve == nil {
		t.Fatalf("%s: shellboot did not wire the blob-resolve handler — the "+
			"cross-peer file pipeline is unreachable again", name)
	}

	return &syncTestPeer{ap: ap, ws: ws, id: ap.PeerID()}
}

func connectPeers(t *testing.T, a, b *syncTestPeer) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	bringUpListener(t, ctx, a.ap, "a")
	bringUpListener(t, ctx, b.ap, "b")
	if _, err := a.ap.Connect(ctx, b.ap.Addr().String()); err != nil {
		t.Fatalf("a to b connect: %v", err)
	}
	if _, err := b.ap.Connect(ctx, a.ap.Addr().String()); err != nil {
		t.Fatalf("b to a connect: %v", err)
	}
}

// bringUpListener starts the peer's listener and waits for ready.
func bringUpListener(t *testing.T, ctx context.Context, ap *entitysdk.AppPeer, name string) {
	t.Helper()
	ready := make(chan struct{})
	errCh := make(chan error, 1)
	go func() { errCh <- ap.ListenReady(ctx, ready) }()
	select {
	case <-ready:
	case err := <-errCh:
		t.Fatalf("%s listen: %v", name, err)
	case <-time.After(5 * time.Second):
		t.Fatalf("%s listen timeout", name)
	}
}

// awaitFileContent polls for a file to exist with the expected bytes.
// Content, not existence: local/files:write writes atomically via a
// rename, but settling on existence alone would assert the weaker thing
// and the interesting failure is a file that arrives truncated.
func awaitFileContent(path, want string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil && string(b) == want {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}
