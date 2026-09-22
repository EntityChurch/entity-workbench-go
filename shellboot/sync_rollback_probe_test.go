package shellboot_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/shellcmd"
	"entity-workbench-go/workbench"
)

// Does the SYNC leg have a rollback floor, and should it?
//
// # Why this probe exists and why it is a probe
//
// `fetch.Consumer.acceptSeq` is the §3-RES.4 monotonicity floor on the
// STATIC leg: a published root whose `seq` went backwards is refused
// (`fetch.ErrSeqRollback`), authentic bytes and all. The sync leg — a
// receiver pulling files out of a sender's tree — has no equivalent.
// Same repository, same threat, one leg defended.
//
// We routed that as a SOURCE READING, explicitly unperformed, and the
// specification seat then narrowed its floor row to a per-leg rule partly
// on the strength of it (their `A-31` ruling: *"a monotonic floor is a
// valid witness over a single writer's leg, whatever the subject's
// authority class"*). Under the blanket row that preceded it, our gap was
// conformant; under the narrowing it is a defect.
//
// ⚠ **So the reading is now load-bearing for somebody else's text, and it
// has still never been run.** That is the condition this repository keeps
// getting burned by — *"a 'no change needed' claim about another layer is
// a hypothesis until the operation has been run end to end"* (D19) — and
// the direction it burns us is that a correct source reading misses a
// layer underneath. This performs it.
//
// # What a "rollback" is on this leg, and what it is NOT
//
// A sender is authoritative for the folder it publishes. If it genuinely
// reverts a file, the receiver SHOULD follow — that is convergence, not
// an attack, and arm 2 exists so a fix cannot be "refuse anything that
// looks older". The threat is a STALE DELIVERY ARRIVING LATE: an old,
// authentic, correctly-authorized (entity, hash) pair replayed after a
// newer one has already landed. Out-of-order delivery produces it by
// accident; a compromised or malicious sender produces it on purpose.
//
// # MEASURED 2026-09-11 — CONFIRMED. And what it does NOT establish.
//
//	REPLAY: dispatch status=200, err=nil
//	file on disk after the replay: "version one — the original"
//
// The stale delivery was applied over the newer one, silently, and the
// control arm passed in the same run.
//
// ⚠ **What is measured is that the HANDLER has no ordering check**: given
// an authentic (path, entity, hash) triple it writes it, whatever it
// already holds for that path. The replay is dispatched in-process at
// `workbench/blob-resolve:receive`, which is the same entry point a live
// delivery and a backfill both reach and the identical shape
// `shellcmd.materializeOne` builds — so an out-of-order LIVE delivery
// lands here by accident with no attacker at all.
//
// ⚠ **What is NOT measured: that an unauthorized remote party can trigger
// it.** This probe does not cross the wire and says nothing about who may
// dispatch. Do not let that sentence travel in the grammar of the
// measured one — a replay by an authorized sender and a replay by a
// stranger are different claims and only the first is performed here.
//
// Tier: real-session (TESTING-STRATEGY §4) — two peers through the real
// `shellboot.Bootstrap`, a real TCP connection, assertions on bytes on
// disk at the receiving end.
func TestProbe_SyncLegHasNoRollbackFloor(t *testing.T) {
	senderDir := t.TempDir()
	receiverDir := t.TempDir()

	sender := newSyncTestPeer(t, "rb-sender")
	receiver := newSyncTestPeer(t, "rb-receiver")
	connectPeers(t, sender, receiver)

	const folder = "rollback"
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

	const name = "minutes.md"
	senderFile := filepath.Join(senderMount, name)
	receiverFile := filepath.Join(receiverMount, name)
	treePath := "/" + sender.id + "/local/files/" + root + "/" + name

	// --- v1 ------------------------------------------------------------
	const v1 = "version one — the original\n"
	writeOrFail(t, senderFile, v1)
	if !awaitFileContent(receiverFile, v1, 30*time.Second) {
		t.Fatal("precondition: v1 never reached the receiver")
	}

	// Capture the v1 delivery while it is still current. This is exactly
	// what an attacker capturing a notification holds, and exactly what an
	// out-of-order delivery carries: an authentic entity and its hash.
	v1Ent, v1Hash, ok := captureFileEntity(t, sender, treePath)
	if !ok {
		t.Fatalf("could not read the sender's own file entity at %s", treePath)
	}

	// --- v2 ------------------------------------------------------------
	const v2 = "version two — the one that must survive\n"
	writeOrFail(t, senderFile, v2)
	if !awaitFileContent(receiverFile, v2, 30*time.Second) {
		t.Fatal("precondition: v2 never reached the receiver")
	}

	// --- ARM 1: replay the captured v1 delivery -------------------------
	//
	// Dispatched at the receiver's own blob-resolve handler, which is the
	// same entry point a live delivery and a backfill both arrive at —
	// `shellcmd.materializeOne` builds this identical shape. Nothing here
	// is privileged: it is the message the sender already sent once.
	subID := subscriptionIDFor(t, receiver, sender.id, root)
	replayStatus, replayErr := deliverEntity(t, receiver, subID, treePath, v1Ent, v1Hash)

	settled := ""
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(receiverFile)
		if err == nil {
			settled = string(b)
			if settled == v1 {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Logf("REPLAY: dispatch status=%d err=%v · file on disk after the replay: %q",
		replayStatus, replayErr, settled)

	rolledBack := settled == v1
	if rolledBack {
		t.Logf("CONFIRMED — THE SYNC LEG HAS NO ROLLBACK FLOOR. A replayed v1 delivery "+
			"overwrote v2 on disk at the receiver. The static leg refuses exactly this "+
			"(fetch.ErrSeqRollback); this leg accepted it with status %d and no error. "+
			"The routed prediction is performed and it holds.", replayStatus)
	} else {
		t.Logf("REFUTED — the replay did NOT land (file is still %q). Something on this "+
			"leg already rejects a stale delivery; find it and name it before anyone "+
			"builds a floor that duplicates it, and RETRACT the routed claim.", settled)
	}

	// The measurement is PINNED, so the day it changes somebody has to
	// say so rather than absorb it. A silent flip in either direction is
	// the failure this probe exists to prevent: the claim is already
	// carried in another repository's text.
	if !rolledBack {
		t.Errorf("this probe pins a MEASURED absence and the measurement just changed: "+
			"the stale replay no longer lands (file %q).\n"+
			"  If a rollback floor was added, this probe is now the GATE — invert the "+
			"assertion, keep the control arm, and correct the routed claim at the "+
			"architecture tracker (A-31) and in STATUS.md.\n"+
			"  If nothing was added, something else changed underneath this leg and "+
			"that is the more interesting finding.", settled)
	}

	// --- ARM 2: the control, and the reason a naive fix is wrong --------
	//
	// The sender GENUINELY reverts. The receiver must follow: a sender is
	// authoritative for its own folder, so this is convergence and not an
	// attack. Any floor built on arm 1 has to keep this arm passing, which
	// rules out "refuse content we have seen before" and "refuse an older
	// mtime" — both of which would pass arm 1 and break the product.
	const reverted = "version one — the original\n" // byte-identical to v1
	writeOrFail(t, senderFile, reverted)
	if !awaitFileContent(receiverFile, reverted, 30*time.Second) {
		got, _ := os.ReadFile(receiverFile)
		t.Fatalf("CONTROL ARM FAILED: the sender deliberately reverted its own file and "+
			"the receiver did not follow (on disk: %q). A sync must converge on what the "+
			"authoritative side actually holds — if this fails because a rollback floor "+
			"was added, the floor is wrong, not this arm", got)
	}
	t.Logf("CONTROL: a genuine revert by the sender converged normally — "+
		"a floor must distinguish this from arm 1, and %q is byte-identical to v1, "+
		"so content alone cannot be the discriminator", reverted)
}

// captureFileEntity reads a peer's own file entity and its content hash.
func captureFileEntity(t *testing.T, p *syncTestPeer, treePath string) (entity.Entity, hash.Hash, bool) {
	t.Helper()
	ent, found, err := p.ap.Get(treePath)
	if err != nil || !found {
		t.Logf("capture %s: found=%v err=%v", treePath, found, err)
		return entity.Entity{}, hash.Hash{}, false
	}
	return ent, ent.ContentHash, true
}

// subscriptionIDFor finds the receiver's sync binding for this folder.
func subscriptionIDFor(t *testing.T, p *syncTestPeer, remotePeerID, root string) string {
	t.Helper()
	bindings, _ := workbench.LoadSyncBindings(p.ap.Store())
	for _, b := range bindings {
		if b.RemotePeerID == remotePeerID && b.Root == root {
			return b.SubscriptionID
		}
	}
	t.Fatalf("no sync binding for %s/%s — the probe cannot synthesize a delivery", remotePeerID, root)
	return ""
}

// deliverEntity dispatches one synthesized delivery at the receiver's
// blob-resolve handler, in the shape `shellcmd.materializeOne` builds.
func deliverEntity(
	t *testing.T, p *syncTestPeer, subID, uri string, ent entity.Entity, h hash.Hash,
) (uint, error) {
	t.Helper()
	notif := types.SubscriptionNotificationData{
		SubscriptionID: subID,
		Event:          "updated",
		URI:            uri,
		Hash:           h,
	}
	notifEnt, err := notif.ToEntity()
	if err != nil {
		t.Fatalf("build notification: %v", err)
	}
	resp, err := p.ap.Executor().ExecuteWithIncluded(
		workbench.BlobResolvePattern, "receive", notifEnt, nil,
		map[hash.Hash]entity.Entity{h: ent},
	)
	if err != nil {
		return 0, err
	}
	if resp == nil {
		return 0, nil
	}
	return resp.Status, nil
}
