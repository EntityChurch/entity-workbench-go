package shellboot_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/shellboot"
	"entity-workbench-go/shellcmd"
)

// PROBE — is `system/capability/policy/{peer}` enough to authorize a sync
// with NO OpenAccess anywhere?
//
// Everything in this repo that has ever run a cross-peer sync ran it under
// `peer.OpenAccessGrants()` — a wildcard that authorizes everything and
// therefore measures nothing about permissions. The one exception,
// `TestStage3_CapDelegation_Positive`, passes a scoped set to
// `WithConnectionGrants`, which is a PROCESS-WIDE floor applied to every
// inbound connection: it proves the grant set is sufficient, and says
// nothing about granting it to ONE named peer.
//
// The per-peer mechanism is the V7 v7.62 §8 policy table, read at
// authenticate-response by `readHandshakePolicyGrants` and UNIONED with the
// floor. Nothing in this repo has ever written one. This probe establishes
// whether the seamless flow can stand on it before any verb is built on top
// — D19: route the measurement, not the argument.
//
// Two things it is specifically designed to find out:
//
//  1. Does the peer-id-keyed form work? The reader tries
//     hex(identityHash), then the Base58 peer-id, then `default`. Only the
//     middle one is usable by an operator who has a peer-id from discovery
//     and nothing else, so if it does not work the whole UX changes.
//  2. WHEN must the policy exist? It is consulted during the handshake, so
//     a policy written after two peers are already connected plausibly does
//     nothing until they reconnect. That is an ordering constraint the
//     verbs have to either enforce or state.
func TestProbe_PolicyTableAuthorizesSync_NoOpenAccess(t *testing.T) {
	// The grant set TestStage3_CapDelegation_Positive established as the
	// minimum a blob-resolve sync needs from the SENDING peer. Copied
	// deliberately rather than shared: if this probe and that test drift,
	// the difference is the finding.
	syncGrants := []types.GrantEntry{
		{
			Handlers:   types.CapabilityScope{Include: []string{"system/subscription"}},
			Operations: types.CapabilityScope{Include: []string{"*"}},
			Resources:  types.CapabilityScope{Include: []string{"*"}},
		},
		{
			Handlers:   types.CapabilityScope{Include: []string{"system/content"}},
			Operations: types.CapabilityScope{Include: []string{"get"}},
			Resources:  types.CapabilityScope{Include: []string{"system/content"}},
		},
		{
			Handlers:   types.CapabilityScope{Include: []string{"local/files"}},
			Operations: types.CapabilityScope{Include: []string{"read"}},
			Resources:  types.CapabilityScope{Include: []string{"*"}},
		},
		{
			Handlers:   types.CapabilityScope{Include: []string{"system/tree"}},
			Operations: types.CapabilityScope{Include: []string{"get"}},
			Resources:  types.CapabilityScope{Include: []string{"*"}},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// NO OpenAccess on either side. This is the whole point.
	senderAP, senderWS, err := shellboot.Bootstrap(ctx, shellboot.Config{
		LocalAlias: "sender", ListenAddr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("bootstrap sender: %v", err)
	}
	defer senderAP.Close()

	receiverAP, receiverWS, err := shellboot.Bootstrap(ctx, shellboot.Config{
		LocalAlias: "receiver", ListenAddr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("bootstrap receiver: %v", err)
	}
	defer receiverAP.Close()

	// A SYNC IS MUTUAL AUTHORIZATION, NOT A ONE-WAY SHARE. Established by
	// this probe's first run, which granted only the sender→receiver
	// direction: the subscribe was accepted and no file ever arrived.
	//
	// The reason is the chain's shape. The receiver dispatches INTO the
	// sender to subscribe and to fetch blobs — that is the direction the
	// grant set above covers. But delivery runs the other way: the
	// SENDER's subscription engine dispatches the notification into
	// `entity://{receiver}/workbench/blob-resolve`, and that crosses the
	// receiver's capability boundary. The delivery token the receiver
	// minted authorizes the engine to deliver; it does not substitute for
	// the sender holding a capability at the receiver's edge.
	//
	// Every prior cross-peer test in this repo hid this, because the
	// receiver always had `OpenAccessGrants()` — including the one test
	// written specifically to close the cap-delegation gap, which scopes
	// alice and leaves bob wildcard on purpose. So the half of the
	// authorization an operator has to perform on the RECEIVING device
	// has never been exercised anywhere.
	policyPath := "system/capability/policy/" + receiverAP.PeerID()
	if _, err := senderAP.Store().Put(policyPath, types.TypeCapPolicyEntry,
		types.CapabilityPolicyEntryData{
			PeerPattern: receiverAP.PeerID(),
			Grants:      syncGrants,
			Notes:       "probe: sender authorizes this peer to subscribe and fetch",
		}); err != nil {
		t.Fatalf("write sender-side policy entry: %v", err)
	}

	deliveryGrants := []types.GrantEntry{
		{
			Handlers:   types.CapabilityScope{Include: []string{"workbench/blob-resolve"}},
			Operations: types.CapabilityScope{Include: []string{"receive"}},
			Resources:  types.CapabilityScope{Include: []string{"*"}},
		},
	}
	revPolicyPath := "system/capability/policy/" + senderAP.PeerID()
	if _, err := receiverAP.Store().Put(revPolicyPath, types.TypeCapPolicyEntry,
		types.CapabilityPolicyEntryData{
			PeerPattern: senderAP.PeerID(),
			Grants:      deliveryGrants,
			Notes:       "probe: receiver authorizes this peer to deliver notifications",
		}); err != nil {
		t.Fatalf("write receiver-side policy entry: %v", err)
	}

	bringUpListener(t, ctx, senderAP, "sender")
	bringUpListener(t, ctx, receiverAP, "receiver")
	if _, err := receiverAP.Connect(ctx, senderAP.Addr().String()); err != nil {
		t.Fatalf("receiver to sender connect: %v", err)
	}
	if _, err := senderAP.Connect(ctx, receiverAP.Addr().String()); err != nil {
		t.Fatalf("sender to receiver connect: %v", err)
	}

	// Both mount a directory of the same basename.
	const folder = "policyprobe"
	senderDir := filepath.Join(t.TempDir(), folder)
	receiverDir := filepath.Join(t.TempDir(), folder)
	if err := os.MkdirAll(senderDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(receiverDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sOut, err := senderWS.Mount(shellcmd.MountRequest{
		FilesystemDir: senderDir, TargetPrefix: "archives/" + folder + "/",
	})
	if err != nil {
		t.Fatalf("sender mount: %v", err)
	}
	if _, err := receiverWS.Mount(shellcmd.MountRequest{
		FilesystemDir: receiverDir, TargetPrefix: "archives/" + folder + "/",
	}); err != nil {
		t.Fatalf("receiver mount: %v", err)
	}

	if _, err := receiverWS.Sync(shellcmd.SyncRequest{
		Remote: senderAP.PeerID(), Root: sOut.RootName,
	}); err != nil {
		t.Fatalf("sync refused under the policy table: %v\n"+
			"  => the policy entry did not authorize the subscribe. Either the "+
			"peer-id-keyed form is not consulted, or the grant set is short.", err)
	}

	const body = "authorized by policy, not by wildcard\n"
	if err := os.WriteFile(filepath.Join(senderDir, "p.txt"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if !awaitFileContent(filepath.Join(receiverDir, "p.txt"), body, 45*time.Second) {
		t.Fatal("the subscribe was accepted but no file arrived — the policy " +
			"authorized the trigger and not the fetch, which is the failure " +
			"mode worth knowing about: it looks like a working share.")
	}
}

// The control arm. Same setup, NO policy entry — if this also succeeds,
// the probe above proves nothing, because the default connection grants
// were sufficient all along and the policy table is decoration.
func TestProbe_WithoutPolicy_SyncIsRefusedOrSilent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	senderAP, senderWS, err := shellboot.Bootstrap(ctx, shellboot.Config{
		LocalAlias: "sender", ListenAddr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("bootstrap sender: %v", err)
	}
	defer senderAP.Close()

	receiverAP, receiverWS, err := shellboot.Bootstrap(ctx, shellboot.Config{
		LocalAlias: "receiver", ListenAddr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("bootstrap receiver: %v", err)
	}
	defer receiverAP.Close()

	bringUpListener(t, ctx, senderAP, "sender")
	bringUpListener(t, ctx, receiverAP, "receiver")
	if _, err := receiverAP.Connect(ctx, senderAP.Addr().String()); err != nil {
		t.Fatalf("receiver to sender connect: %v", err)
	}
	if _, err := senderAP.Connect(ctx, receiverAP.Addr().String()); err != nil {
		t.Fatalf("sender to receiver connect: %v", err)
	}

	const folder = "nopolicy"
	senderDir := filepath.Join(t.TempDir(), folder)
	receiverDir := filepath.Join(t.TempDir(), folder)
	if err := os.MkdirAll(senderDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(receiverDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sOut, err := senderWS.Mount(shellcmd.MountRequest{
		FilesystemDir: senderDir, TargetPrefix: "archives/" + folder + "/",
	})
	if err != nil {
		t.Fatalf("sender mount: %v", err)
	}
	if _, err := receiverWS.Mount(shellcmd.MountRequest{
		FilesystemDir: receiverDir, TargetPrefix: "archives/" + folder + "/",
	}); err != nil {
		t.Fatalf("receiver mount: %v", err)
	}

	_, syncErr := receiverWS.Sync(shellcmd.SyncRequest{
		Remote: senderAP.PeerID(), Root: sOut.RootName,
	})

	const body = "unauthorized\n"
	if err := os.WriteFile(filepath.Join(senderDir, "p.txt"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	arrived := awaitFileContent(filepath.Join(receiverDir, "p.txt"), body, 15*time.Second)

	t.Logf("CONTROL ARM — no policy entry: sync err = %v, file arrived = %v", syncErr, arrived)
	if syncErr == nil && arrived {
		t.Fatal("a file synced across with NO policy entry and NO OpenAccess — " +
			"the default connection grants already authorize cross-peer file " +
			"reads, which is a much bigger finding than this probe was looking " +
			"for and makes the policy table decorative here")
	}
}

// PROBE — does a policy written AFTER the peers are already connected
// take effect, or must the connection be re-established?
//
// This decides the shape of the verbs. The policy table is consulted at
// authenticate-response, i.e. during the handshake, and an operator's
// natural order is connect first, then share. If the grant is baked into
// the connection, `share` has to force a reconnect or say plainly that it
// does not take effect until one — and a share that silently does nothing
// until the next restart is the worst possible version of this feature.
func TestProbe_PolicyWrittenAfterConnect(t *testing.T) {
	syncGrants := []types.GrantEntry{
		{
			Handlers:   types.CapabilityScope{Include: []string{"system/subscription"}},
			Operations: types.CapabilityScope{Include: []string{"*"}},
			Resources:  types.CapabilityScope{Include: []string{"*"}},
		},
		{
			Handlers:   types.CapabilityScope{Include: []string{"system/content"}},
			Operations: types.CapabilityScope{Include: []string{"get"}},
			Resources:  types.CapabilityScope{Include: []string{"system/content"}},
		},
		{
			Handlers:   types.CapabilityScope{Include: []string{"local/files"}},
			Operations: types.CapabilityScope{Include: []string{"read"}},
			Resources:  types.CapabilityScope{Include: []string{"*"}},
		},
		{
			Handlers:   types.CapabilityScope{Include: []string{"system/tree"}},
			Operations: types.CapabilityScope{Include: []string{"get"}},
			Resources:  types.CapabilityScope{Include: []string{"*"}},
		},
	}
	deliveryGrants := []types.GrantEntry{{
		Handlers:   types.CapabilityScope{Include: []string{"workbench/blob-resolve"}},
		Operations: types.CapabilityScope{Include: []string{"receive"}},
		Resources:  types.CapabilityScope{Include: []string{"*"}},
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	senderAP, senderWS, err := shellboot.Bootstrap(ctx, shellboot.Config{
		LocalAlias: "sender", ListenAddr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("bootstrap sender: %v", err)
	}
	defer senderAP.Close()
	receiverAP, receiverWS, err := shellboot.Bootstrap(ctx, shellboot.Config{
		LocalAlias: "receiver", ListenAddr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("bootstrap receiver: %v", err)
	}
	defer receiverAP.Close()

	bringUpListener(t, ctx, senderAP, "sender")
	bringUpListener(t, ctx, receiverAP, "receiver")

	// CONNECT FIRST — the operator's natural order.
	if _, err := receiverAP.Connect(ctx, senderAP.Addr().String()); err != nil {
		t.Fatalf("receiver to sender connect: %v", err)
	}
	if _, err := senderAP.Connect(ctx, receiverAP.Addr().String()); err != nil {
		t.Fatalf("sender to receiver connect: %v", err)
	}

	// THEN write both policies.
	if _, err := senderAP.Store().Put(
		"system/capability/policy/"+receiverAP.PeerID(), types.TypeCapPolicyEntry,
		types.CapabilityPolicyEntryData{PeerPattern: receiverAP.PeerID(), Grants: syncGrants},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := receiverAP.Store().Put(
		"system/capability/policy/"+senderAP.PeerID(), types.TypeCapPolicyEntry,
		types.CapabilityPolicyEntryData{PeerPattern: senderAP.PeerID(), Grants: deliveryGrants},
	); err != nil {
		t.Fatal(err)
	}

	const folder = "afterconnect"
	senderDir := filepath.Join(t.TempDir(), folder)
	receiverDir := filepath.Join(t.TempDir(), folder)
	if err := os.MkdirAll(senderDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(receiverDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sOut, err := senderWS.Mount(shellcmd.MountRequest{
		FilesystemDir: senderDir, TargetPrefix: "archives/" + folder + "/",
	})
	if err != nil {
		t.Fatalf("sender mount: %v", err)
	}
	if _, err := receiverWS.Mount(shellcmd.MountRequest{
		FilesystemDir: receiverDir, TargetPrefix: "archives/" + folder + "/",
	}); err != nil {
		t.Fatalf("receiver mount: %v", err)
	}

	_, syncErr := receiverWS.Sync(shellcmd.SyncRequest{
		Remote: senderAP.PeerID(), Root: sOut.RootName,
	})
	const body = "written after connect\n"
	if err := os.WriteFile(filepath.Join(senderDir, "a.txt"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	arrived := awaitFileContent(filepath.Join(receiverDir, "a.txt"), body, 20*time.Second)

	t.Logf("POLICY-AFTER-CONNECT: sync err = %v, file arrived = %v", syncErr, arrived)
	if syncErr != nil || !arrived {
		t.Log("=> the grant is fixed at handshake. `share` MUST re-establish the " +
			"connection, or state that it does not take effect until one.")
	} else {
		t.Log("=> the policy is consulted late enough that connect order does not " +
			"matter. The verbs can stay simple.")
	}
}
