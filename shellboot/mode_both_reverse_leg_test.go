package shellboot_test

import (
	"strings"
	"testing"
	"time"

	"entity-workbench-go/shellcmd"
	"entity-workbench-go/workbench"
)

// mode_both_reverse_leg_test.go — `Mode: both` establishes the receiver →
// owner leg.
//
// Tier: real-session (TESTING-STRATEGY §4).
//
// # What this replaces, and why the replacement was mandatory
//
// `mode_both_cause_test.go` measured why the reverse leg never ran: the
// owner's peer state for the receiver is `offered`, acceptance is recorded
// by `declareAcceptedFolder` in the RECEIVER's tree, no channel carries it
// back, and `receiveFromPeers` admitted `accepted` only — so the owner
// never created a reverse subscription on any pair of peers, ever. That
// file carried a standing instruction: *when the reverse leg is built,
// delete this and gate the leg instead; do not relax the assertion.* This
// is that gate.
//
// # What actually changed, and what deliberately did NOT
//
// **Acceptance still does not travel.** The owner's state for the receiver
// is still `offered`, and the first assertion below pins that — the fix is
// not a new channel, it is the recognition that the owner never needed one
// for THIS decision. `offered` on a folder we own means *we offered it to
// them*: our own act, recorded by our own `declareLocalShare`. Requiring
// `accepted` there was requiring a fact that structurally cannot arrive,
// which is a different thing from requiring a fact that has not arrived
// yet.
//
// The receiver→owner channel is still owed, and conflict propagation still
// needs it (`docs/outbox/CONFLICT-PROPAGATION-OPTIONS-2026-09-08.md` §8).
// Nothing here closes that.
//
// # The control arm is the load-bearing half
//
// `Establishes` alone would pass against a build that subscribes to every
// peer it has ever heard of. `DoesNotEstablish` is what makes it a gate:
// a `send`-only folder must NOT acquire a reverse binding, because the
// whole failure this fix repairs is a direction being run that was not
// declared — and over-applying the correction turns every one-way share
// into a two-way one silently, on somebody else's disk.
//
// Uses `OpenAccess: true` via newSyncTestPeer, so per AP63 it says nothing
// about the permission stage. The grant half is asserted separately in
// TestModeBoth_OwnerGrantsTheReceiverTheDeliveryHandler below, which reads
// the derived policy rather than the wire.

// TestModeBoth_OwnerEstablishesTheReverseLeg is the gate the cause test
// asked for: when both sides declare `both`, a reconcile pass creates the
// owner's sync binding naming the receiver.
//
// # This test used to declare `both` on ONE side, and that was wrong
//
// Until 2026-09-10 it set `both` on the owner only, left the receiver on
// the `receive` that `accept` writes, and asserted a binding appeared. A
// binding did appear, and it could not have carried a byte: a receive-only
// peer does not publish, so `desiredGrantsByPeer` writes it no sender
// grants, and the owner's subscribe answers `403 capability_denied:
// insufficient capability for handler scope` — observed verbatim in a
// `make twopeer-sync` run.
//
// So the gate was asserting **that a mechanism exists**, on a case where
// the mechanism provably cannot work. That is this repo's most-repeated
// failure shape, and it is exactly what the reconciler now refuses to
// build (see `remoteRootForReverseLeg`). The requirement it should have
// encoded all along is the one an operator has to satisfy: direction is a
// per-peer property of one folder, so BOTH sides ask for two-way.
//
// # It also now covers ASYMMETRIC ROOTS for free
//
// `shareOneFolder` has bob accept into `from-alice` while alice's root is
// `photos`, so the binding asserted below is keyed on the RECEIVER's root.
// With the owner's root it does not exist, which is the defect
// `mode_both_asymmetric_roots_test.go` measures end to end in bytes.
func TestModeBoth_OwnerEstablishesTheReverseLeg(t *testing.T) {
	alice, bob, root := shareOneFolder(t)
	id := workbench.FolderID(alice.id, root)

	for _, p := range []struct {
		name string
		peer *syncTestPeer
	}{{"owner", alice}, {"receiver", bob}} {
		if _, err := p.peer.ws.SetFolderMode(shellcmd.SetFolderModeRequest{
			FolderID: id, Mode: workbench.FolderModeBoth,
		}); err != nil {
			t.Fatalf("%s could not declare mode=both on %q: %v", p.name, id, err)
		}
	}

	aliceFolder, ok := workbench.LoadFolder(alice.ap.Store(), id)
	if !ok {
		t.Fatalf("the owner holds no record for folder %q", id)
	}

	// PIN THE PREMISE. If this ever becomes `accepted`, somebody built the
	// receiver→owner PUSH channel — good news, but this gate then stops
	// testing what it claims to: it would be passing because acceptance
	// arrived, not because `offered` is sufficient. Re-read the fix before
	// trusting a green run.
	//
	// Note what the fix did instead: the owner LOOKS. Acceptance never had
	// to travel for this decision, and the receiver's root — which cannot
	// be inferred at all — is read from their tree in the same pass.
	aliceState, ok := aliceFolder.PeerState(bob.id)
	if !ok {
		t.Fatalf("the owner's record for %q names no peer state for the receiver %s", id, bob.id)
	}
	if aliceState.State != workbench.FolderStateOffered {
		t.Fatalf("premise moved: the owner's state for the receiver is %q, not %q.\n"+
			"  This gate asserts that `offered` is SUFFICIENT for the reverse leg. If "+
			"acceptance now travels, the leg may be establishing for a different reason "+
			"and this test no longer measures the fix.",
			aliceState.State, workbench.FolderStateOffered)
	}
	if !aliceFolder.Receives() {
		t.Fatalf("a folder declared %q does not report Receives() — the reverse leg is "+
			"refused one step earlier than this test is about", workbench.FolderModeBoth)
	}

	bobFolder, ok := workbench.LoadFolder(bob.ap.Store(), id)
	if !ok {
		t.Fatalf("the receiver holds no record for folder %q", id)
	}
	bobRoot := bobFolder.ReceivingRoot()
	if bobRoot == root {
		t.Fatalf("premise gone: the two roots agree (%q) — this fixture is supposed to "+
			"have the receiver accept into a differently named directory, and with "+
			"matching names the assertion below cannot fail on the root", root)
	}

	if _, err := alice.ws.ReconcileWithTimeout(30 * time.Second); err != nil {
		t.Fatalf("owner reconcile: %v", err)
	}

	// THE MEASUREMENT — keyed on the RECEIVER's root, which is the whole
	// point: it is a fact in their tree and the owner has to go and read
	// it (shellcmd/remote_declaration.go).
	if _, ok := workbench.LoadSyncBinding(alice.ap.Store(), bob.id, bobRoot); !ok {
		_, wrong := workbench.LoadSyncBinding(alice.ap.Store(), bob.id, root)
		t.Fatalf("the owner has NO sync binding naming the receiver %s at THEIR root %q "+
			"after both sides declared %q and the owner reconciled.\n"+
			"  A binding at the OWNER's root %q: %v — if that is true, the reverse leg "+
			"is subscribing to a prefix that does not exist on the far side, which "+
			"establishes cleanly and delivers nothing forever.\n"+
			"  Start at remoteRootForReverseLeg (shellcmd/remote_declaration.go).",
			bob.id, bobRoot, workbench.FolderModeBoth, root, wrong)
	}
}

// TestModeBoth_ReceiveOnlyCounterpartGetsNoReverseLeg is the second control
// arm, and it is the one that stops the fix from being re-broken by
// "helpfully" falling back to the owner's own root name.
//
// The owner declares `both`; the receiver does not. There is nothing to
// receive — a receive-only peer publishes nothing and grants no sender
// authority — so the correct outcome is NO binding and a reason an
// operator can act on. The old version of this file asserted the opposite,
// and the binding it asserted was dead on arrival.
//
// The `problems` assertion is the load-bearing half. Refusing silently
// would be a regression dressed as a fix: an operator whose two-way folder
// is quiet needs to be told that the other machine has not asked for
// two-way, on the machine they are looking at.
func TestModeBoth_ReceiveOnlyCounterpartGetsNoReverseLeg(t *testing.T) {
	alice, bob, root := shareOneFolder(t)
	id := workbench.FolderID(alice.id, root)

	// ONLY the owner declares two-way. The receiver keeps what `accept`
	// wrote, which is what an operator who set this up on one machine has.
	if _, err := alice.ws.SetFolderMode(shellcmd.SetFolderModeRequest{
		FolderID: id, Mode: workbench.FolderModeBoth,
	}); err != nil {
		t.Fatalf("owner could not declare mode=both on %q: %v", id, err)
	}
	bobFolder, ok := workbench.LoadFolder(bob.ap.Store(), id)
	if !ok {
		t.Fatalf("the receiver holds no record for folder %q", id)
	}
	if bobFolder.Publishes() {
		t.Fatalf("premise gone: the receiver's folder reports Publishes() at mode %q — "+
			"this arm is about a counterpart that has NOT asked for two-way",
			bobFolder.EffectiveMode())
	}

	out, err := alice.ws.ReconcileWithTimeout(30 * time.Second)
	if err != nil {
		t.Fatalf("owner reconcile: %v", err)
	}

	for _, r := range []string{root, bobFolder.ReceivingRoot()} {
		if _, ok := workbench.LoadSyncBinding(alice.ap.Store(), bob.id, r); ok {
			t.Fatalf("the owner built a reverse sync binding at root %q against a "+
				"receive-only peer.\n"+
				"  That leg cannot authorize — the receiver publishes nothing and "+
				"therefore grants no sender authority — so it establishes, reports "+
				"healthy and delivers nothing for as long as it exists.", r)
		}
	}

	// AND IT MUST SAY SO. A refusal nobody is told about is the same
	// silence, one layer down.
	said := false
	for _, p := range out.Problems {
		if strings.Contains(p, bob.id) {
			said = true
			t.Logf("reported: %s", p)
		}
	}
	if !said {
		t.Fatalf("the owner declined to build the reverse leg and reported NOTHING "+
			"naming %s.\n"+
			"  problems: %v\n"+
			"  An operator whose two-way folder is quiet has no way to learn that the "+
			"other machine never asked for two-way.", bob.id, out.Problems)
	}
}

// TestModeBoth_SendOnlyFolderGetsNoReverseLeg is the control arm. Without
// it the test above passes against a build that subscribes to everyone.
func TestModeBoth_SendOnlyFolderGetsNoReverseLeg(t *testing.T) {
	alice, bob, root := shareOneFolder(t)
	id := workbench.FolderID(alice.id, root)

	// `send` is what a plain `share` means, and it is also the pre-S6
	// default for a local folder (EffectiveMode). Declared explicitly so
	// the arm cannot pass by accident of a default moving.
	if _, err := alice.ws.SetFolderMode(shellcmd.SetFolderModeRequest{
		FolderID: id, Mode: workbench.FolderModeSend,
	}); err != nil {
		t.Fatalf("owner could not declare mode=send on %q: %v", id, err)
	}
	if _, err := alice.ws.ReconcileWithTimeout(30 * time.Second); err != nil {
		t.Fatalf("owner reconcile: %v", err)
	}

	if _, ok := workbench.LoadSyncBinding(alice.ap.Store(), bob.id, root); ok {
		t.Fatalf("a folder declared %q acquired a reverse sync binding naming %s.\n"+
			"  The owner is now pulling a peer's writes on a share the operator declared "+
			"ONE-WAY. That is somebody else's files arriving on this disk without a "+
			"declaration asking for them — a worse failure than the one the `both` fix "+
			"repaired, and in the opposite direction.",
			workbench.FolderModeSend, bob.id)
	}
}

// TestModeBoth_OwnerGrantsTheReceiverTheDeliveryHandler covers the half a
// subscription alone does not: the owner must also grant the receiver the
// blob-resolve handler, or the reverse leg subscribes successfully and
// then 403s every delivery it pulls. A half-built leg is worse than none,
// because it fails at the last hop with a permission error that reads as a
// security problem rather than as a missing grant.
func TestModeBoth_OwnerGrantsTheReceiverTheDeliveryHandler(t *testing.T) {
	alice, bob, root := shareOneFolder(t)
	id := workbench.FolderID(alice.id, root)

	if _, err := alice.ws.SetFolderMode(shellcmd.SetFolderModeRequest{
		FolderID: id, Mode: workbench.FolderModeBoth,
	}); err != nil {
		t.Fatalf("owner could not declare mode=both on %q: %v", id, err)
	}
	out, err := alice.ws.ReconcileWithTimeout(30 * time.Second)
	if err != nil {
		t.Fatalf("owner reconcile: %v", err)
	}

	var dev *shellcmd.DeviceStatus
	for i := range out.Devices {
		if out.Devices[i].PeerID == bob.id {
			dev = &out.Devices[i]
			break
		}
	}
	if dev == nil {
		t.Fatalf("the owner's reconcile outcome names no device for the receiver %s", bob.id)
	}

	// Derived from the grant set itself rather than typed as a literal:
	// a constant retyped in a test pins the author's memory, not the
	// product (the rule browser-rust took from our catch-up work).
	want := workbench.BlobResolvePattern
	if want == "" {
		t.Fatal("BlobResolvePattern is empty — this test can no longer say anything")
	}
	if !strings.Contains(dev.OutboundGrant, want) {
		t.Fatalf("the owner does not grant the receiver %q.\n"+
			"  outbound grant: %q\n"+
			"  The reverse subscription will establish and then refuse every delivery it "+
			"pulls. See desiredGrantsByPeer (shellcmd/reconcile.go).",
			want, dev.OutboundGrant)
	}
}
