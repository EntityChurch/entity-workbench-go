package shellboot_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/shellboot"
	"entity-workbench-go/shellcmd"
	"entity-workbench-go/workbench"
)

// TestRelationship_SurvivesRestart_WithNoOperatorAction is the gate on
// the complaint that started this work: *"why, after I've connected to a
// peer, do I have to do it again?"*
//
// # What used to happen
//
// Nothing in this repo called `system/network:maintain-peer` — ten
// references, nine inside its own SDK wrapper and one in its own test —
// so a connection was one `AppPeer.Connect` and nothing more. A restart
// left no record of the relationship, nothing to re-establish it from,
// and no code that would have re-established it. The operator reconnected
// by hand, on both machines, every launch. That is not a bug in a button;
// there was no object in the system that meant "this peer and I have a
// relationship".
//
// # Why the assertion has to be shaped like this
//
// **Across a process boundary, on the DERIVED structure, with a
// before-and-after.** Every one of those three is load-bearing and this
// repo has been caught by the absence of each:
//
//   - In-process, the connection never dropped, so nothing is measured.
//   - Re-reading the device entity passes against a build that restores
//     nothing — the tree is fine, it is always fine, and that is exactly
//     what made three restart defects invisible (D26/AP62).
//   - Asserting only the end state cannot tell "the reconciler
//     re-established it" from "it was never disconnected". So the test
//     asserts NOT connected immediately after the restart, and connected
//     after one reconcile pass, which is the only pair of facts that
//     names the cause.
func TestRelationship_SurvivesRestart_WithNoOperatorAction(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	home := t.TempDir()
	t.Setenv("HOME", home)

	// The peer that stays up. Ephemeral store is fine — it is the other
	// side's restart being measured — but it must LISTEN, because the
	// whole point is that A finds its way back to it unaided.
	remoteCfg := shellboot.Config{LocalAlias: "remote", ListenAddr: "127.0.0.1:0"}
	remoteAP, _, err := shellboot.Bootstrap(ctx, remoteCfg)
	if err != nil {
		t.Fatalf("bootstrap remote: %v", err)
	}
	defer func() { _ = remoteAP.Close() }()
	remoteLi, err := shellboot.BringUpListener(ctx, remoteAP, remoteCfg)
	if err != nil {
		t.Fatalf("remote listener: %v", err)
	}
	defer remoteLi.Cancel()
	remoteID := remoteAP.PeerID()

	// The peer that restarts. sqlite + the default identity, which is what
	// the shipped app now does — a per-run keypair would namespace the
	// tree differently on every run and the restart would be measuring
	// nothing.
	localCfg := shellboot.Config{
		LocalAlias:  "restarter",
		StorageKind: "sqlite",
		StoragePath: filepath.Join(home, "store.db"),
	}

	// --- first run: declare the peer and reconcile --------------------
	ap1, ws1, err := shellboot.Bootstrap(ctx, localCfg)
	if err != nil {
		t.Fatalf("bootstrap 1: %v", err)
	}
	if err := workbench.SaveDevice(ap1.Store(), workbench.DeviceData{
		PeerID:        remoteID,
		Label:         "remote",
		Addresses:     []string{remoteLi.BoundAddr},
		AddedAtMillis: 1,
	}); err != nil {
		t.Fatalf("declare device: %v", err)
	}
	out1, err := ws1.Reconcile(ctx)
	if err != nil {
		t.Fatalf("reconcile 1: %v", err)
	}
	if !maintained(out1, remoteID) {
		t.Fatalf("first reconcile did not maintain the peer: %+v problems=%v", out1.Devices, out1.Problems)
	}
	if !connectedTo(ap1, remoteID) {
		t.Fatalf("first reconcile reported maintained but the peer is not connected; problems=%v", out1.Problems)
	}
	if err := ap1.Close(); err != nil {
		t.Fatalf("close 1: %v", err)
	}

	// --- second run: same store, fresh process image ------------------
	ap2, ws2, err := shellboot.Bootstrap(ctx, localCfg)
	if err != nil {
		t.Fatalf("bootstrap 2: %v", err)
	}
	defer func() { _ = ap2.Close() }()

	// The declaration is durable. This passes against a build that
	// re-establishes nothing, which is precisely why it is not the
	// assertion — it is the premise for the one that follows.
	if _, ok := workbench.LoadDevice(ap2.Store(), remoteID); !ok {
		t.Fatal("the device declaration did not survive the restart")
	}

	// BEFORE: nothing is connected. Bootstrap does not dial, and if this
	// ever fails the test below stops being able to attribute anything.
	if connectedTo(ap2, remoteID) {
		t.Fatal("the restarted peer was already connected before any reconcile — " +
			"this test can no longer distinguish re-establishment from a connection that never dropped")
	}

	// AFTER: one pass, no operator action, no address typed.
	out2, err := ws2.Reconcile(ctx)
	if err != nil {
		t.Fatalf("reconcile 2: %v", err)
	}
	if !maintained(out2, remoteID) {
		t.Fatalf("after a restart the relationship was not re-established: %+v problems=%v",
			out2.Devices, out2.Problems)
	}
	if !connectedTo(ap2, remoteID) {
		t.Fatalf("reconcile reported the peer maintained but no connection exists; problems=%v", out2.Problems)
	}
}

// TestReconcile_WithoutADeclarationConnectsNothing is the control arm.
//
// Without it, the test above could be satisfied by a bootstrap that dials
// every peer it has ever heard of — a different behaviour, arguably a
// worse one, reported as a pass.
func TestReconcile_WithoutADeclarationConnectsNothing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	remoteCfg := shellboot.Config{LocalAlias: "remote2", ListenAddr: "127.0.0.1:0"}
	remoteAP, _, err := shellboot.Bootstrap(ctx, remoteCfg)
	if err != nil {
		t.Fatalf("bootstrap remote: %v", err)
	}
	defer func() { _ = remoteAP.Close() }()
	li, err := shellboot.BringUpListener(ctx, remoteAP, remoteCfg)
	if err != nil {
		t.Fatalf("remote listener: %v", err)
	}
	defer li.Cancel()

	ap, ws, err := shellboot.Bootstrap(ctx, shellboot.Config{LocalAlias: "bare"})
	if err != nil {
		t.Fatalf("bootstrap local: %v", err)
	}
	defer func() { _ = ap.Close() }()

	out, err := ws.Reconcile(ctx)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(out.Devices) != 0 {
		t.Fatalf("reconcile produced device state with nothing declared: %+v", out.Devices)
	}
	if connectedTo(ap, remoteAP.PeerID()) {
		t.Fatal("a peer nobody declared was connected to")
	}
}

func maintained(out shellcmd.ReconcileOutcome, peerID string) bool {
	for _, d := range out.Devices {
		if d.PeerID == peerID {
			return d.Maintained
		}
	}
	return false
}

// connectedTo asks the connection POOL, not the tree.
//
// Deliberate: `system/peer/status` is the authority on liveness and is
// transition-written, so a status entity left behind by the previous
// process would read as "connected" on a peer that has just started and
// dialled nothing — which is the exact false pass this test exists to
// avoid. The pool cannot lie about a socket that does not exist.
func connectedTo(ap *entitysdk.AppPeer, peerID string) bool {
	for _, c := range ap.ConnectedPeers() {
		if c.PeerID == peerID {
			return true
		}
	}
	return false
}
