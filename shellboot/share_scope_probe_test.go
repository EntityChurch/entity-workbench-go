package shellboot_test

// share_scope_probe_test.go — WHAT DOES SHARING ONE FOLDER ACTUALLY GRANT?
//
// The operator's question, asked while looking at `system/content:get` in
// the design review: *"we just need to make sure it's not having access to
// stuff outside the scope, because we can't be exposing the full content
// store."*
//
// It was the right thing to look at and the answer was worse than the
// question. `workbench.SyncSenderGrants()` — written verbatim into
// `system/capability/policy/{peer}` by the reconciler, with no per-folder
// narrowing anywhere between — grants the receiving peer:
//
//	system/subscription : *   on *
//	system/content      : get on system/content
//	local/files         : read on *
//	system/tree         : get on *
//
// Three of those four are `*`. So sharing ONE folder with a peer grants
// that peer read access to EVERYTHING: every entity in the tree, every
// mounted file, and a live subscription to any prefix they choose.
//
// # Why the existing coverage could not see it
//
// The grant set's own doc comment says it "is not invented — it is the
// minimum established by cmd_stage3_cap_delegation_test.go", and that is
// true of the HANDLER list and false of everything else. That test's
// negative arm drops `system/content:get` **entirely** and confirms
// materialization fails. Dropping a whole handler establishes that the
// handler is necessary. It says nothing about whether its RESOURCES are
// minimal, and nobody ever narrowed them.
//
// That is the transferable shape: **a grant minimized along one axis reads
// as a minimized grant.** "Each entry is load-bearing by measurement" was
// written about the rows and got read as being about the cells.
//
// # The boundary that holds TODAY, and the one the spec actually wants
//
// An earlier version of this comment claimed `system/content` "cannot be
// scoped per-folder by anything, in any implementation, ever". **That was
// wrong**, and `EXTENSION-CONTENT` §6.4 says so directly: the dispatch's
// resource target is a NAMESPACE PATH, §6.4.2 binds each hash into the
// tree at `{namespace}/{hex(H)}`, and §6.4.1 makes namespace-scoped
// topology — where get "consults the tree binding and serves only when the
// hash is bound under the requested namespace" — a **MUST for any
// multi-party deployment**. The flat mode is single-trust-domain,
// explicitly opt-in, and running it multi-party is called "out-of-spec and
// security-defective" in the spec's own words.
//
// We run the flat mode: bare `"system/content"` namespace, no
// `system/content:ingest` call anywhere, so nothing is bound under any
// namespace in our tree. That is tracked, not fixed here — and it cannot
// be fixed here alone, because core-go implements the ingest half
// (`bindHashTreePresence`) and not the get half (`handleGet` is a bare
// store lookup with no namespace consult).
//
// So the arms below measure the boundary that is load-bearing RIGHT NOW:
// with content-get unenforced, an unshared file is confidential because its
// hash is undiscoverable, and the tree is what discloses hashes. Narrow the
// tree grant and you close the disclosure path. That is a true statement
// about this build and NOT a statement about the architecture.

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

// scopeProbePair brings up two peers with listeners and mutual dials, and
// returns them ready to share. No OpenAccess: a wildcard fixture deletes
// the permission stage from the suite (AP63), which is the whole subject
// here.
func scopeProbePair(t *testing.T, ctx context.Context) (
	ownerAP, guestAP *entitysdk.AppPeer,
	ownerWS, guestWS *shellcmd.ShellWorkspace,
) {
	t.Helper()
	sAP, sWS, err := shellboot.Bootstrap(ctx, shellboot.Config{
		LocalAlias: "owner", ListenAddr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("bootstrap owner: %v", err)
	}
	t.Cleanup(func() { _ = sAP.Close() })

	rAP, rWS, err := shellboot.Bootstrap(ctx, shellboot.Config{
		LocalAlias: "guest", ListenAddr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("bootstrap guest: %v", err)
	}
	t.Cleanup(func() { _ = rAP.Close() })

	bringUpListener(t, ctx, sAP, "owner")
	bringUpListener(t, ctx, rAP, "guest")
	if _, err := rAP.Connect(ctx, sAP.Addr().String()); err != nil {
		t.Fatalf("guest dial owner: %v", err)
	}
	if _, err := sAP.Connect(ctx, rAP.Addr().String()); err != nil {
		t.Fatalf("owner dial guest: %v", err)
	}
	return sAP, rAP, sWS, rWS
}

// TestShareScope_APrivateFolderIsNotReadableByAPeerWeSharedSomethingElseWith
// is the boundary.
//
// The owner mounts TWO folders and shares exactly ONE. The guest then
// reads a path inside the OTHER one, over the wire.
//
// A peer-qualified read dispatches to that peer (AP11), so this is a real
// remote read against the owner's capability check — not a look at our own
// mirror, which would pass whatever the grants said.
func TestShareScope_APrivateFolderIsNotReadableByAPeerWeSharedSomethingElseWith(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	ownerAP, guestAP, ownerWS, _ := scopeProbePair(t, ctx)

	sharedDir := filepath.Join(t.TempDir(), "shared")
	privateDir := filepath.Join(t.TempDir(), "private")
	for _, d := range []string{sharedDir, privateDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	const secret = "tax-returns-and-recovery-phrases\n"
	if err := os.WriteFile(filepath.Join(privateDir, "secret.txt"), []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sharedDir, "holiday.txt"), []byte("fine to share\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	sharedOut, err := ownerWS.Mount(shellcmd.MountRequest{
		FilesystemDir: sharedDir, TargetPrefix: "archives/shared/",
	})
	if err != nil {
		t.Fatalf("owner mount shared: %v", err)
	}
	privateOut, err := ownerWS.Mount(shellcmd.MountRequest{
		FilesystemDir: privateDir, TargetPrefix: "archives/private/",
	})
	if err != nil {
		t.Fatalf("owner mount private: %v", err)
	}

	// Share ONE folder. This is the entire operator gesture.
	if _, err := ownerWS.Share(shellcmd.ShareRequest{
		Peer: guestAP.PeerID(), Root: sharedOut.RootName,
	}); err != nil {
		t.Fatalf("share: %v", err)
	}
	if _, err := ownerWS.Reconcile(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	// Grants are assembled at handshake (AP63), so the policy just written
	// is inert until the connection is REPLACED — and `Connect` on a peer
	// already in the pool returns the pooled session without a new
	// handshake, so dialling again is not enough. Disconnect first, which
	// is exactly what `refreshGrantConnection` does and for this reason.
	//
	// The GUEST is the peer that must do it: the owner assembles inbound
	// grants for whoever dialled in, so the connection that carries the new
	// authority is the guest's own outbound one. A reconnect by the granter
	// refreshes nothing the grantee dispatches over.
	guestAP.Disconnect(ownerAP.PeerID())
	if _, err := guestAP.Connect(ctx, ownerAP.Addr().String()); err != nil {
		t.Fatalf("guest re-dial: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	// Anti-vacuity: the guest MUST be able to read the shared folder. If
	// this fails, the negative below proves nothing — it would pass
	// against a peer that authorized nothing at all.
	sharedPath := "/" + ownerAP.PeerID() + "/local/files/" + sharedOut.RootName + "/holiday.txt"
	if _, ok, err := guestAP.Get(sharedPath); err != nil || !ok {
		t.Fatalf("the guest cannot read the folder we DID share (ok=%v err=%v).\n"+
			"  The negative assertion below is vacuous until this passes.", ok, err)
	}

	// The boundary.
	privatePath := "/" + ownerAP.PeerID() + "/local/files/" + privateOut.RootName + "/secret.txt"
	ent, ok, err := guestAP.Get(privatePath)
	if err == nil && ok {
		body := string(ent.Data)
		t.Fatalf("A PEER WE SHARED ONE FOLDER WITH READ A FILE FROM A FOLDER WE DID NOT SHARE.\n"+
			"  path: %s\n"+
			"  got %d bytes back%s\n\n"+
			"  workbench.SyncSenderGrants() grants `system/tree:get` and\n"+
			"  `local/files:read` on Resources `*`, and the reconciler writes it into\n"+
			"  system/capability/policy/{peer} verbatim. So the operator gesture\n"+
			"  \"share this folder\" actually authorizes a read of the ENTIRE tree and\n"+
			"  every mounted file on this machine.\n\n"+
			"  Narrowing the tree grant is the fix, and it is the ONLY fix available:\n"+
			"  content addressing is flat, so system/content:get can never be scoped\n"+
			"  per folder. The tree grant is what discloses the hashes, so the tree\n"+
			"  grant is the boundary.",
			privatePath, len(body), previewSecret(body))
	}
}

// TestShareScope_WeRunTheSingleTrustDomainTopology records a KNOWN
// DEVIATION, deliberately as a log and not as an assertion.
//
// It is not asserted because the assertion would have to be one of two
// things and both are wrong today. "Content-get refuses an unbound hash"
// fails, because core-go's `handleGet` does not consult the §6.4.2
// binding. "Content-get serves any hash" would ENCODE THE DEFECT AS A
// PREMISE — the AP65 mistake this repo has made before, where a documented
// limitation became a test that would fail when someone fixed it.
//
// So it states the position and names what has to change, on both sides.
func TestShareScope_WeRunTheSingleTrustDomainTopology(t *testing.T) {
	t.Log("KNOWN DEVIATION — EXTENSION-CONTENT §6.4.1.\n" +
		"  We pass the bare `system/content` default namespace and never call\n" +
		"  system/content:ingest, so no hash is bound at {namespace}/{hex(H)} in\n" +
		"  our tree. §6.4.1 makes namespace-scoped topology a MUST for multi-party\n" +
		"  deployments and says running the flat one multi-party is 'out-of-spec\n" +
		"  and security-defective'.\n" +
		"  OURS: ingest shared content into a per-folder namespace and request THAT\n" +
		"  namespace from EnsureClosure.\n" +
		"  THEIRS: handleGet is a bare store lookup — the §6.4.1 'consults the tree\n" +
		"  binding' half is unimplemented, so scoping our grant alone narrows which\n" +
		"  label we may claim and not which bytes we may get.\n" +
		"  Until both land, the tree grant asserted above is the operative boundary.")
}

func previewSecret(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	if len(body) > 40 {
		body = body[:40] + "…"
	}
	return " — content begins: " + body
}
