package shellcmd

import (
	"context"
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"
)

// reconcile_test.go — the union defect, and the loop's idempotence.
//
// # The defect these tests exist for
//
// `system/capability/policy/{peer}` is ONE row per peer, and
// `SaveAccessPolicy` REPLACES rather than merges — correctly, by its own
// documented contract, which assumes one caller with one intent. But two
// verbs write it with different intents:
//
//	Share(root, peer)   -> SyncSenderGrants     (they may subscribe + pull)
//	Accept(peer, root)  -> SyncReceiverGrants   (they may deliver to us)
//
// So on a peer that both shares a folder TO someone and accepts one FROM
// them, the second write silently revokes the first, and one direction
// stops authorizing at the next handshake. That is two-way sharing
// between one pair of machines — the thing an operator with a laptop and
// a desktop wants first.
//
// It was invisible because `shellboot/sync_twoway_e2e_test.go`, the test
// written specifically for two-way, bootstraps with `OpenAccess: true`
// and says so in its own header. Under a wildcard the policy row is not
// consulted, so the clobber cannot be observed. AP63, one instance on
// from the one that named AP63.

func reconcileFixture(t *testing.T) (*ShellWorkspace, *workbench.Store) {
	t.Helper()
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })
	return NewShellWorkspace(ap, "self", ""), ap.Store()
}

// outgoingFolderID is the shared id of the fixture's outgoing "photos"
// folder, as workbench.FolderID computes it for THIS peer.
func outgoingFolderID(ws *ShellWorkspace) string {
	return workbench.FolderID(ws.Local.Peer.PeerID(), "photos")
}

// declareBothDirections writes the two folder records an operator gets
// from sharing one folder out and accepting one back, with no substrate
// established — which is exactly the durable state after those two verbs
// have run.
//
// The outgoing folder's id is the SHARED one — workbench.FolderID(us,
// root), the same string this folder has on their peer. Writing the bare
// root here (which is what it used to be) made the unwind and the
// withdrawal look up an id nothing had written, so both silently found
// no folder and did nothing.
func declareBothDirections(t *testing.T, ws *ShellWorkspace, st *workbench.Store, them string) {
	t.Helper()
	out := workbench.FolderData{
		ID: outgoingFolderID(ws), Label: "photos", Kind: "files",
		Root: "photos", Path: "/tmp/photos", Origin: "local",
		Mode: workbench.FolderModeBoth,
	}.WithPeerState(them, workbench.FolderStateOffered, 1, "")
	if err := workbench.SaveFolder(st, out); err != nil {
		t.Fatal(err)
	}

	inID := workbench.ReceivedFolderID(them, "notes")
	in := workbench.FolderData{
		ID: inID, Label: "notes", Kind: "files",
		Root: "notes", Path: "/tmp/notes", Origin: them,
		Mode: workbench.FolderModeReceive,
	}.WithPeerState(them, workbench.FolderStateAccepted, 2, "")
	if err := workbench.SaveFolder(st, in); err != nil {
		t.Fatal(err)
	}
}

// TestReconcile_PolicyIsTheUnionOfBothDirections is the fix.
func TestReconcile_PolicyIsTheUnionOfBothDirections(t *testing.T) {
	ws, st := reconcileFixture(t)
	const them = "2KLf7osYcMLEdmSnYLx3Bg6TScaGJefLZJdKaL1tPNHAbR"
	declareBothDirections(t, ws, st, them)

	// Seed the row the way the verbs leave it: Share first, then Accept
	// over the top of it. This is not a contrived starting point — it is
	// the byte-for-byte state `share` followed by `accept` produces.
	if err := workbench.SaveAccessPolicy(st, them, workbench.SyncSenderGrants([]workbench.SharedScope{{LocalRoot: "probe", FolderID: "probe-id"}}), "share"); err != nil {
		t.Fatal(err)
	}
	if err := workbench.SaveAccessPolicy(st, them, workbench.SyncReceiverGrants(), "accept"); err != nil {
		t.Fatal(err)
	}
	if pol, ok := workbench.LoadAccessPolicy(st, them); !ok || grantsInclude(pol.Grants, "system/subscription") {
		// Guard the premise. If this ever passes, SaveAccessPolicy has
		// started merging and the defect below no longer exists — in
		// which case this test is measuring nothing and must be revisited
		// rather than quietly staying green.
		t.Fatalf("premise broken: after share-then-accept the row still carries the sender grants (%+v)", pol.Grants)
	}

	if _, err := ws.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	pol, ok := workbench.LoadAccessPolicy(st, them)
	if !ok {
		t.Fatal("no policy row after reconcile")
	}
	if !grantsInclude(pol.Grants, "system/subscription") {
		t.Error("the peer we share a folder TO cannot subscribe — the sender half of the union is missing")
	}
	if !grantsInclude(pol.Grants, workbench.BlobResolvePattern) {
		t.Error("the peer we accepted a folder FROM cannot deliver — the receiver half of the union is missing")
	}
}

// TestReconcile_IsIdempotent is the property that makes the loop safe to
// run at startup, after every change, and on a timer.
//
// The failure it guards is not cosmetic. A second pass that decides the
// policy differs rewrites the row, and reconcilePolicies then RECONNECTS
// every peer whose row it rewrote, because a grant is only assembled at a
// handshake. A loop that reports "settled" while dropping and
// re-establishing every connection on every pass is an outage generator
// wearing a status message — and the natural implementation has exactly
// that bug, because the union is built from map iteration and comes out
// in a different order each time.
func TestReconcile_IsIdempotent(t *testing.T) {
	ws, st := reconcileFixture(t)
	const them = "2KLf7osYcMLEdmSnYLx3Bg6TScaGJefLZJdKaL1tPNHAbR"
	declareBothDirections(t, ws, st, them)

	first, err := ws.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("first pass: %v", err)
	}
	if len(first.Actions) == 0 {
		t.Fatal("the first pass over an unestablished declaration changed nothing — it had a policy to write")
	}

	// Several passes: once is not enough to catch an ordering-dependent
	// comparison, which can agree by luck.
	for i := 0; i < 5; i++ {
		again, err := ws.Reconcile(context.Background())
		if err != nil {
			t.Fatalf("pass %d: %v", i+2, err)
		}
		for _, a := range again.Actions {
			if strings.Contains(a, "authorization") || strings.Contains(a, "reconnected") {
				t.Fatalf("pass %d rewrote authorization on a settled system: %q", i+2, a)
			}
		}
	}
}

// TestReconcile_OnlyAcceptedFoldersAuthorizeDelivery — an offer is a
// label, not an authority (APP-CONVENTION-SHARE §2.2). A folder somebody
// has offered us and we have not accepted must not grant them the right
// to write into our blob-resolve handler; if it did, "accept" would be a
// button with no meaning.
func TestReconcile_OnlyAcceptedFoldersAuthorizeDelivery(t *testing.T) {
	ws, st := reconcileFixture(t)
	const them = "2KLf7osYcMLEdmSnYLx3Bg6TScaGJefLZJdKaL1tPNHAbR"

	id := workbench.ReceivedFolderID(them, "notes")
	f := workbench.FolderData{
		ID: id, Label: "notes", Kind: "files", Root: "notes",
		Origin: them, Mode: workbench.FolderModeReceive,
	}.WithPeerState(them, workbench.FolderStateOffered, 1, "")
	if err := workbench.SaveFolder(st, f); err != nil {
		t.Fatal(err)
	}

	if _, err := ws.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if pol, ok := workbench.LoadAccessPolicy(st, them); ok && grantsInclude(pol.Grants, workbench.BlobResolvePattern) {
		t.Error("an UNACCEPTED offer authorized the sender to deliver into this peer")
	}
}

// TestReconcile_ReportsAFolderWithNoMountRatherThanCreatingOne.
//
// Two halves, both load-bearing. The loop must NOT silently create a
// mount: that means writing into a directory on somebody's disk on the
// strength of a remembered path, which is a decision an operator makes.
// And it must not stay silent either — a share whose mount is gone is the
// exact failure this product shipped twice, where every tree-reading
// surface reports the folder as healthy while nothing can arrive.
func TestReconcile_ReportsAFolderWithNoMountRatherThanCreatingOne(t *testing.T) {
	ws, st := reconcileFixture(t)
	const them = "2KLf7osYcMLEdmSnYLx3Bg6TScaGJefLZJdKaL1tPNHAbR"
	declareBothDirections(t, ws, st, them)

	out, err := ws.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(out.Folders) != 2 {
		t.Fatalf("reported %d folders, want 2", len(out.Folders))
	}
	for _, f := range out.Folders {
		if f.Mounted {
			t.Errorf("folder %q reports mounted, but nothing ever mounted it", f.Label)
		}
	}
	var named int
	for _, p := range out.Problems {
		if strings.Contains(p, "mount") {
			named++
		}
	}
	if named < 2 {
		t.Errorf("only %d of 2 mountless folders were reported as problems; problems=%v", named, out.Problems)
	}
}

func grantsInclude(grants []types.GrantEntry, handler string) bool {
	for _, g := range grants {
		for _, h := range g.Handlers.Include {
			if h == handler {
				return true
			}
		}
	}
	return false
}
