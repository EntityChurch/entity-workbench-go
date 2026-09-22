package shellboot_test

import (
	"testing"
	"time"

	"entity-workbench-go/shellcmd"
	"entity-workbench-go/workbench"
)

// mode_both_cause_test.go — WHY `Mode: both` does not run receiver → owner.
//
// Tier: real-session (TESTING-STRATEGY §4).
//
// # The symptom, and why it needed a cause
//
// `make threepeer-sync` PHASE 5 measures `Mode: both` three ways — one
// side declaring it, both sides declaring it, and both sides with
// SYMMETRIC root names — and the receiver's write comes back in none of
// them. The third arm is the discriminator and it refutes the obvious
// hypothesis: the two-root-names trap is not the cause, because
// asymmetric and symmetric behave identically. What that harness could
// say was that the declaration TAKES (the owner reports `mode=both`) and
// that the owner holds no sync binding naming the receiver. It could not
// say why, and "start at receiveFromPeers" is where it left the next
// session.
//
// # What this measures, and what it deliberately does not
//
// It measures the DISCRIMINATING FACT: on the owner's side, the peer
// state for the receiver after a completed share-and-accept. If that
// state is anything other than `accepted`, `receiveFromPeers`
// (shellcmd/reconcile.go) cannot return the receiver, no reverse
// subscription is ever created, and the symptom is fully explained
// without appealing to the network, the grant, the mount or the roots.
//
// It does NOT assert the absence of the delivery — that is what the
// three-peer harness over a real network is for, and re-asserting it
// here in-process would be a weaker copy of a stronger gate.
//
// # The mechanism, stated so a failure here is legible
//
// Acceptance is recorded by `declareAcceptedFolder` (shellcmd/declare.go)
// in the RECEIVER's own tree. There is no message back to the owner, so
// the owner's record — written by `declareLocalShare` with the peer in
// state `offered` — has nothing that can ever advance it. Both peers hold
// a record with the SAME folder id (S6/AP72) and DIFFERENT peer states,
// and only the receiver's says `accepted`.
//
// # If this test starts failing
//
// It fails when the owner's state becomes `accepted`, which means
// something now carries acceptance back — the receiver→owner channel that
// both `Mode: both` and conflict propagation need
// (`reviews/CONFLICT-PROPAGATION-OPTIONS-2026-09-08.md` §8). At that
// point this file is no longer a cause, it is a regression guard pointed
// the wrong way: delete it and gate the reverse leg instead. Do not
// "fix" it by relaxing the assertion.
//
// Uses `OpenAccess: true` via newSyncTestPeer, so per AP63 it says
// NOTHING about the permission stage — which is the point. The cause
// reproduces with authorization entirely out of the picture, so
// authorization is not it.
func TestModeBoth_OwnerNeverLearnsTheReceiverAccepted(t *testing.T) {
	alice, bob, root := shareOneFolder(t)
	id := workbench.FolderID(alice.id, root)

	// Control arm, and it is not a formality: it establishes that the
	// accept ACTUALLY HAPPENED. Without it, an assertion that the owner
	// does not see `accepted` is satisfied by a fixture where nobody
	// accepted anything, which is the vacuous pass this whole class of
	// test is prone to.
	bobFolder, ok := workbench.LoadFolder(bob.ap.Store(), id)
	if !ok {
		t.Fatalf("the receiver holds no record for folder %q — the accept did not "+
			"happen, so nothing below measures what it claims to", id)
	}
	bobState, ok := bobFolder.PeerState(alice.id)
	if !ok {
		t.Fatalf("the receiver's record for %q names no peer state for the owner %s",
			id, alice.id)
	}
	if bobState.State != workbench.FolderStateAccepted {
		t.Fatalf("the receiver's own record says %q, not %q — the accept did not "+
			"complete and the measurement below is void",
			bobState.State, workbench.FolderStateAccepted)
	}

	// The owner declares `both`. This is the operator gesture the whole
	// feature is, and it succeeds — which is half of why the failure is
	// hard to see from outside.
	if _, err := alice.ws.SetFolderMode(shellcmd.SetFolderModeRequest{
		FolderID: id, Mode: workbench.FolderModeBoth,
	}); err != nil {
		t.Fatalf("owner could not declare mode=both on %q: %v", id, err)
	}

	aliceFolder, ok := workbench.LoadFolder(alice.ap.Store(), id)
	if !ok {
		t.Fatalf("the owner holds no record for folder %q", id)
	}
	if got := aliceFolder.EffectiveMode(); got != workbench.FolderModeBoth {
		t.Fatalf("the declaration did not take: owner's mode is %q, want %q",
			got, workbench.FolderModeBoth)
	}
	if !aliceFolder.Receives() {
		t.Fatalf("a folder declared %q does not report Receives() — the reverse leg "+
			"is refused one step earlier than this test is about",
			workbench.FolderModeBoth)
	}

	// THE MEASUREMENT. The owner's view of the receiver.
	aliceState, ok := aliceFolder.PeerState(bob.id)
	if !ok {
		t.Fatalf("the owner's record for %q names no peer state for the receiver %s "+
			"— the share did not record one", id, bob.id)
	}
	if aliceState.State == workbench.FolderStateAccepted {
		t.Fatalf("the owner's record now says %q for the receiver.\n"+
			"  Something carries acceptance back to the owner, which did not exist "+
			"when this test was written.\n"+
			"  That is the receiver->owner channel `Mode: both` needs: this file has "+
			"served its purpose and the reverse leg itself should be gated instead.",
			workbench.FolderStateAccepted)
	}

	// Run the loop. The owner should now establish the receive leg, and
	// the point is that it cannot: `receiveFromPeers` filters on the
	// state asserted above.
	if _, err := alice.ws.ReconcileWithTimeout(30 * time.Second); err != nil {
		t.Fatalf("owner reconcile: %v", err)
	}

	if _, ok := workbench.LoadSyncBinding(alice.ap.Store(), bob.id, root); ok {
		t.Fatalf("the owner now HAS a sync binding naming the receiver after a "+
			"reconcile, while its peer state is %q.\n"+
			"  The three-peer harness measured the opposite. Either the cause moved "+
			"or the reverse leg was built; check PHASE 5 before trusting this.",
			aliceState.State)
	}

	t.Logf("cause: owner's peer state for the receiver is %q, and "+
		"receiveFromPeers admits only %q — so no reverse subscription is created, "+
		"and the receiver's writes have nothing to travel on.\n"+
		"  The receiver's own record for the same folder id says %q. Acceptance is "+
		"written in the receiver's tree and never travels.",
		aliceState.State, workbench.FolderStateAccepted, bobState.State)
}
