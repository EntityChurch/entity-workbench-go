package shellboot_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/shellboot"
	"entity-workbench-go/shellcmd"
	"entity-workbench-go/workbench"
)

// AP68, gated at the VERBS rather than at the reconciler.
//
// `shellcmd/reconcile_test.go` already proves the reconciler computes the
// policy row as a union across both directions. It does so by SEEDING the
// row the way the verbs leave it — which was honest, and which quietly
// records that the verbs themselves were still writing their own half.
// They were, and the reconciler only healed it on its next pass: at the
// following `status`, or the next launch.
//
// That window is not theoretical. It is what an operator with a laptop
// and a desktop lands in by performing the two gestures the product
// exists for, in the same session:
//
//	share photos with them     -> row := sender grants
//	accept them notes          -> row := receiver grants   (revokes the above)
//
// From there, `photos` is authorized in neither peer's favour until
// something re-runs the loop, and nothing on either machine says so.
//
// This test drives the two SHIPPED verbs and asserts on the row they
// leave behind. Every cheaper form passes against the defect: asserting
// the reconciler's output tests the healer rather than the verbs, and
// asserting after a `status` call tests that the heal works — which was
// never in doubt.
//
// Tier: real-session (TESTING-STRATEGY §4).
func TestShareThenAccept_LeavesBothHalvesAuthorized(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	home := t.TempDir()
	t.Setenv("HOME", home)

	shareDir := filepath.Join(home, "photos")
	if err := os.MkdirAll(shareDir, 0o755); err != nil {
		t.Fatal(err)
	}

	ap, ws, err := shellboot.Bootstrap(ctx, shellboot.Config{LocalAlias: "self"})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	defer func() { _ = ap.Close() }()

	// A peer-id we never connect to. The permission row is written before
	// any connection is required, which is the whole reason a grant can be
	// stale — so an unreachable peer is the right fixture here, not a
	// weakened one.
	const them = "2KLf7osYcMLEdmSnYLx3Bg6TScaGJefLZJdKaL1tPNHAbR"

	mounted, err := ws.Mount(shellcmd.MountRequest{
		FilesystemDir: shareDir, TargetPrefix: "archives/photos/",
	})
	if err != nil {
		t.Fatalf("mount: %v", err)
	}

	// --- gesture 1: share a folder OUT -------------------------------
	if _, err := ws.Share(shellcmd.ShareRequest{
		Root: mounted.RootName, Peer: them, NowMillis: 1_756_000_000_000,
	}); err != nil {
		t.Fatalf("share: %v", err)
	}
	pol, ok := workbench.LoadAccessPolicy(ap.Store(), them)
	if !ok || !grantsMention(pol.Grants, "system/subscription") {
		t.Fatalf("premise broken: after `share` the peer cannot subscribe (%+v)", pol.Grants)
	}

	// --- gesture 2: accept a DIFFERENT folder IN, from the same peer ---
	//
	// A directory is given, which is the shape an operator uses now: the
	// accept creates and mounts it. The error afterwards is expected and
	// is not what is under test — there is no second peer, so the sync
	// leg fails, and `Accept` documents that it leaves the delivery grant
	// in place on that path. That is exactly the state being measured.
	if _, err := ws.Accept(shellcmd.AcceptRequest{
		Peer: them, Root: "notes", Directory: filepath.Join(home, "from-them"),
	}); err == nil {
		t.Log("accept unexpectedly succeeded with no remote peer; the assertions below still hold")
	}

	pol, ok = workbench.LoadAccessPolicy(ap.Store(), them)
	if !ok {
		t.Fatal("no policy row at all after share-then-accept")
	}

	// Both halves, asserted independently — they fail independently and a
	// fix for one does not imply the other (AP15).
	if !grantsMention(pol.Grants, "system/subscription") {
		t.Errorf("REGRESSION (AP68): after `accept`, the peer we SHARE `%s` to can no longer "+
			"subscribe — the sender half of the row was replaced rather than unioned, so the "+
			"folder we offered them is authorized in no direction and nothing says so.\n"+
			"  grants now: %+v", mounted.RootName, pol.Grants)
	}
	if !grantsMention(pol.Grants, workbench.BlobResolvePattern) {
		t.Errorf("after `accept`, the peer we accepted `notes` from cannot deliver — "+
			"the receiver half is missing.\n  grants now: %+v", pol.Grants)
	}
}

// grantsMention reports whether any grant entry names a handler pattern.
// Compared on the rendered summary rather than field-by-field, so it does
// not have to track the shape of GrantEntry.
func grantsMention(grants []types.GrantEntry, handler string) bool {
	return strings.Contains(workbench.SummarizeGrants(grants), handler)
}
