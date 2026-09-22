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

// M3 step 1: the chain exists because the loop established it, not
// because a test turned it on.
//
// Tier: real-session (TESTING-STRATEGY §4).
//
// `TestM3Baseline_ConcurrentEditLeavesBothWritesOnTheChain` had to
// install a `system/history/config` itself to ask its question at all —
// which was the finding, not the fixture: recording is opt-in per path
// and nothing in the mount / sync / share path installed one, so a
// received folder's overwritten bytes were unreachable in the shipped
// product. This asserts the reconciler now derives it.
//
// The control arm is the load-bearing half. A SEND-ONLY folder must NOT
// get a config: a folder with one writer records an entity per save
// forever and buys nothing, and a positive-only test would pass against
// a build that switched recording on for every mount on the machine.
func TestM3_TheReconcilerEstablishesTheChainForAReceivingFolder(t *testing.T) {
	senderDir := t.TempDir()
	receiverDir := t.TempDir()

	sender := newSyncTestPeer(t, "sender")
	receiver := newSyncTestPeer(t, "receiver")
	connectPeers(t, sender, receiver)

	const folder = "shared"
	senderMount := filepath.Join(senderDir, folder)
	receiverMount := filepath.Join(receiverDir, folder)
	mkdirOrFail(t, senderMount)
	mkdirOrFail(t, receiverMount)

	mountOrFail(t, sender, senderMount, "archives/"+folder+"/")
	mountOrFail(t, receiver, receiverMount, "archives/"+folder+"/")

	// Declare and reconcile through the shipped verbs, so what is under
	// test is the loop and not a hand-assembled arrangement of it.
	if _, err := sender.ws.Share(shellcmd.ShareRequest{
		Root: folder, Peer: receiver.id,
	}); err != nil {
		t.Fatalf("share: %v", err)
	}
	if _, err := receiver.ws.Accept(shellcmd.AcceptRequest{
		Peer: sender.id, Root: folder, Directory: receiverMount, AllowNonEmpty: true,
	}); err != nil {
		t.Fatalf("accept: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := receiver.ws.Reconcile(ctx); err != nil {
		t.Fatalf("receiver reconcile: %v", err)
	}

	// The RECEIVER, which takes deliveries, has a chain.
	cfgPath := "system/history/config/folder-" + folder
	if !receiver.ap.Store().Has(cfgPath) {
		t.Fatalf("the reconciler established no recording config at %s — a delivery "+
			"that overwrites a local edit is unrecoverable", cfgPath)
	}

	// And it actually records, which is the property that matters. A
	// config entity that exists and matches nothing would pass the
	// assertion above and leave the defect in place.
	const body = "recorded\n"
	const name = "tracked.md"
	if err := os.WriteFile(filepath.Join(senderMount, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write on sender: %v", err)
	}
	if !awaitFileContent(filepath.Join(receiverMount, name), body, 30*time.Second) {
		t.Fatal("precondition: the file never reached the receiver")
	}
	treePath := "/" + receiver.id + "/local/files/" + folder + "/" + name
	deadline := time.Now().Add(15 * time.Second)
	var n int
	for time.Now().Before(deadline) {
		if n = len(historyAt(t, receiver, treePath, 50)); n > 0 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if n == 0 {
		t.Fatalf("the config at %s exists and records nothing at %s", cfgPath, treePath)
	}
	t.Logf("receiver chain at %s: %d transitions", treePath, n)

	// The OWNER records too, and that is not an oversight. `share`
	// declares the owner's folder `both` (shellcmd/declare.go), so in a
	// shared folder either side's edit can be overwritten by the other's
	// delivery — which makes the chain worth its storage on both.
	// Asserted rather than assumed, because it is surprising and because
	// the first version of this test asserted the opposite and was wrong.
	if _, err := sender.ws.Reconcile(ctx); err != nil {
		t.Fatalf("sender reconcile: %v", err)
	}
	if !sender.ap.Store().Has(cfgPath) {
		t.Errorf("the owner's folder is declared %q and so takes deliveries too, "+
			"but no chain was established at %s", workbench.FolderModeBoth, cfgPath)
	}
}

// TestM3_ASendOnlyFolderGetsNoChain is the control arm, and it is what
// stops the rule from being "switch recording on for every mount".
//
// A folder that only publishes has exactly one writer — the operator at
// the keyboard — so a chain records an entity per save, forever, on
// mounts that are routinely thousands of files, and answers no question
// anyone can ask. The predicate is Receives(), and this is the case that
// distinguishes it from "is mounted".
//
// It sets the mode EXPLICITLY rather than relying on a default: `share`
// declares `both`, so a test that just skipped the share would be
// asserting about an unshared folder and would pass against a build that
// keyed on the wrong thing.
func TestM3_ASendOnlyFolderGetsNoChain(t *testing.T) {
	senderDir := t.TempDir()
	receiverDir := t.TempDir()

	sender := newSyncTestPeer(t, "sender")
	receiver := newSyncTestPeer(t, "receiver")
	connectPeers(t, sender, receiver)

	const folder = "shared"
	senderMount := filepath.Join(senderDir, folder)
	receiverMount := filepath.Join(receiverDir, folder)
	mkdirOrFail(t, senderMount)
	mkdirOrFail(t, receiverMount)
	mountOrFail(t, sender, senderMount, "archives/"+folder+"/")
	mountOrFail(t, receiver, receiverMount, "archives/"+folder+"/")

	if _, err := sender.ws.Share(shellcmd.ShareRequest{Root: folder, Peer: receiver.id}); err != nil {
		t.Fatalf("share: %v", err)
	}
	if _, err := sender.ws.SetFolderMode(shellcmd.SetFolderModeRequest{
		FolderID: workbench.FolderID(sender.id, folder),
		Mode:     workbench.FolderModeSend,
	}); err != nil {
		t.Fatalf("set mode send: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := sender.ws.Reconcile(ctx); err != nil {
		t.Fatalf("sender reconcile: %v", err)
	}

	cfgPath := "system/history/config/folder-" + folder
	if sender.ap.Store().Has(cfgPath) {
		t.Errorf("a send-only folder got a recording config at %s — nothing can "+
			"overwrite a local edit here, so every transition it writes is storage "+
			"spent on a question that cannot be asked", cfgPath)
	}
}

// TestM3_TheChainConfigIsNotRewrittenOnEveryPass pins the idempotence.
//
// The reconciler runs at startup, after any change, and from the
// `status` verb. A pass that rewrote this config would append a
// transition to the config's OWN chain each time and make a settled
// system report an action forever — which is how an operator learns to
// stop reading the loop's output.
func TestM3_TheChainConfigIsNotRewrittenOnEveryPass(t *testing.T) {
	senderDir := t.TempDir()
	receiverDir := t.TempDir()

	sender := newSyncTestPeer(t, "sender")
	receiver := newSyncTestPeer(t, "receiver")
	connectPeers(t, sender, receiver)

	const folder = "shared"
	senderMount := filepath.Join(senderDir, folder)
	receiverMount := filepath.Join(receiverDir, folder)
	mkdirOrFail(t, senderMount)
	mkdirOrFail(t, receiverMount)
	mountOrFail(t, sender, senderMount, "archives/"+folder+"/")
	mountOrFail(t, receiver, receiverMount, "archives/"+folder+"/")

	if _, err := sender.ws.Share(shellcmd.ShareRequest{Root: folder, Peer: receiver.id}); err != nil {
		t.Fatalf("share: %v", err)
	}
	if _, err := receiver.ws.Accept(shellcmd.AcceptRequest{
		Peer: sender.id, Root: folder, Directory: receiverMount, AllowNonEmpty: true,
	}); err != nil {
		t.Fatalf("accept: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	first, err := receiver.ws.Reconcile(ctx)
	if err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	if !mentionsRecording(first.Actions) {
		t.Fatalf("the first pass did not report establishing the chain; actions: %v",
			first.Actions)
	}

	second, err := receiver.ws.Reconcile(ctx)
	if err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if mentionsRecording(second.Actions) {
		t.Errorf("a settled pass re-established the chain: %v", second.Actions)
	}
}

func mentionsRecording(actions []string) bool {
	for _, a := range actions {
		if strings.Contains(a, "recording changes at") {
			return true
		}
	}
	return false
}
