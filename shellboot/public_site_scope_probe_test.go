package shellboot_test

// public_site_scope_probe_test.go — WHAT DOES PUBLISHING A SITE PUBLICLY
// ACTUALLY GRANT, AND TO WHOM?
//
// `share_scope_probe_test.go` asked this about a grant that names ONE
// peer, and the answer was that we were handing over the whole machine.
// This file asks it about the `default` row — the one grant in the system
// with no grantee — where the same mistake is the same mistake multiplied
// by every peer that can dial this one.
//
// # The four things measured here, and why each arm exists
//
//  1. **A stranger can read the site and VERIFY it.** Not "gets bytes":
//     the whole chain — signed published-root, its signature against the
//     key in the peer-id, the seq floor, a fail-closed CHAMP walk, then
//     the page. A grant that serves pages and cannot be verified is the
//     failure `publish/live_and_static_test.go` names as the most
//     misleading of the three ("this publisher has never published"),
//     because it accuses the publisher of something it did not do.
//  2. **The control arm: with no public grant, that read FAILS.** Without
//     it, arm 1 proves nothing — it would pass identically against a
//     kernel floor grant, or against a peer that authorizes everybody.
//  3. **The boundary: the same stranger gets NOTHING ELSE.** Not this
//     peer's devices, not its folder declarations, not a file in a folder
//     it never shared.
//  4. **A peer we DO share a folder with can read the site too.** That
//     arm exists because the kernel's policy resolution is first-match-
//     wins — see `desiredGrantsByPeer` — so the obvious implementation
//     publishes a site that every peer in the world can read except the
//     one the operator owns. It fails against the un-propagated build,
//     which is the only reason to believe it.
//
// # What is NOT asserted, and why it is a t.Log instead
//
// The `system/content:get` escalation. See the final function: with
// core-go's `handleGet` not consulting the §6.4.2 namespace binding, the
// public grant makes any blob whose hash a caller KNOWS fetchable by
// anybody. Asserting "it is served" would encode the defect as a premise
// and fail when somebody fixes it; asserting "it refuses" fails today.
// So it is measured and logged, with both outcomes phrased so the log
// stays true either way.
//
// No `OpenAccess` anywhere in this file. That is the whole experiment
// (AP63).

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/hash"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/shellboot"
	"entity-workbench-go/shellcmd"
	"entity-workbench-go/workbench"
)

const publicSiteID = "public-notes"
const publicPageBody = "# Public\n\nAnybody may read this.\n"

// seedPublicSite authors a site into a peer's tree and returns the
// prefix a publish would commit to.
func seedPublicSite(t *testing.T, ap *entitysdk.AppPeer) string {
	t.Helper()
	m := entitysdk.NewSiteManifest(publicSiteID, "Public Notes", "index", []entitysdk.NavItem{
		entitysdk.NewNavLeaf("Home", "/index"),
	})
	if _, err := ap.PutSiteManifest(publicSiteID, m); err != nil {
		t.Fatalf("put manifest: %v", err)
	}
	if _, err := ap.PutSitePage(publicSiteID, "index",
		entitysdk.NewMarkdownPage("Public", publicPageBody)); err != nil {
		t.Fatalf("put page: %v", err)
	}
	return workbench.SitesSubpath + "/"
}

// publicSitePeers stands up a publisher with a site, a private mounted
// folder it never shares, and a STRANGER peer that has no relationship
// with it of any kind.
//
// The stranger dials; the publisher does not dial back. That is the
// shape a reader actually arrives in — nobody introduced them — and it
// is also the shape in which a dial-by-address authorizes the dialer
// only (AP63), so nothing about this pair leaks authority sideways.
func publicSitePeers(t *testing.T, ctx context.Context) (
	pubAP, strangerAP *entitysdk.AppPeer,
	pubWS *shellcmd.ShellWorkspace,
	privateRoot string,
) {
	t.Helper()
	pAP, pWS, err := shellboot.Bootstrap(ctx, shellboot.Config{
		LocalAlias: "publisher", ListenAddr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("bootstrap publisher: %v", err)
	}
	t.Cleanup(func() { _ = pAP.Close() })

	sAP, _, err := shellboot.Bootstrap(ctx, shellboot.Config{LocalAlias: "stranger"})
	if err != nil {
		t.Fatalf("bootstrap stranger: %v", err)
	}
	t.Cleanup(func() { _ = sAP.Close() })

	seedPublicSite(t, pAP)

	// A folder the publisher never shares with anybody. Its presence is
	// what makes the negative arm a measurement rather than an assertion
	// about an empty tree.
	privateDir := filepath.Join(t.TempDir(), "private")
	if err := os.MkdirAll(privateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(privateDir, "secret.txt"),
		[]byte("tax-returns-and-recovery-phrases\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mounted, err := pWS.Mount(shellcmd.MountRequest{
		FilesystemDir: privateDir, TargetPrefix: "archives/private/",
	})
	if err != nil {
		t.Fatalf("publisher mount private: %v", err)
	}

	bringUpListener(t, ctx, pAP, "publisher")
	return pAP, sAP, pWS, mounted.RootName
}

// dialStranger opens the stranger's own outbound connection, which is
// the one its dispatches ride. Re-dialled after any policy change,
// because grants are assembled at handshake and `Connect` on a pooled
// peer returns the existing session without a new one (AP63).
func dialStranger(t *testing.T, ctx context.Context, stranger, publisher *entitysdk.AppPeer) {
	t.Helper()
	stranger.Disconnect(publisher.PeerID())
	if _, err := stranger.Connect(ctx, publisher.Addr().String()); err != nil {
		t.Fatalf("stranger dial publisher: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
}

// readSiteLive runs the whole verification chain from the stranger's
// side and returns the page body.
func readSiteLive(ctx context.Context, reader *entitysdk.AppPeer, publisherID string) (string, error) {
	c, err := workbench.NewPeerConsumer(reader, publisherID, nil)
	if err != nil {
		return "", err
	}
	root, err := c.VerifiedRoot(ctx)
	if err != nil {
		return "", err
	}
	walk, err := c.Walk(ctx, root.Data.RootHash)
	if err != nil {
		return "", err
	}
	res := workbench.NewRemoteSiteResolver(ctx, c, root, walk)
	out := res.ResolvePage(workbench.SiteRoot(publicSiteID))
	if !out.Ready || out.Page == nil {
		return "", fmt.Errorf("the page did not resolve (ready=%v, %v)", out.Ready, out.Err)
	}
	return out.Page.Page.Body, nil
}

// TestPublicSite_AStrangerVerifiesTheSiteAndGetsNothingElse is the gate,
// and the second half is the load-bearing one.
func TestPublicSite_AStrangerVerifiesTheSiteAndGetsNothingElse(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	pubAP, strangerAP, pubWS, privateRoot := publicSitePeers(t, ctx)

	out, err := pubWS.Publish(ctx, shellcmd.PublishRequest{Public: true})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if !out.Public.Present || !out.Public.Ours {
		t.Fatalf("publish -public did not write the public grant: %+v", out.Public)
	}
	dialStranger(t, ctx, strangerAP, pubAP)

	// ANTI-VACUITY. Everything below is a negative assertion, and a
	// negative assertion against a peer that authorizes nothing is
	// satisfied by the peer being switched off.
	body, err := readSiteLive(ctx, strangerAP, pubAP.PeerID())
	if err != nil {
		t.Fatalf("a stranger could not read the PUBLIC site: %v\n"+
			"  Every assertion below this line is vacuous until this passes — a peer\n"+
			"  that refuses everything satisfies all of them.", err)
	}
	if body != publicPageBody {
		t.Fatalf("public page body mismatch:\n got %q\nwant %q", body, publicPageBody)
	}

	// THE BOUNDARY. A peer-qualified read dispatches to that peer (AP11),
	// so each of these crosses the wire and hits the publisher's own
	// capability check — not a look at our own mirror, which would pass
	// whatever the grants said.
	for _, probe := range []struct {
		what string
		path string
		why  string
	}{
		{
			what: "a file in a folder that was never shared",
			path: "/" + pubAP.PeerID() + "/local/files/" + privateRoot + "/secret.txt",
			why: "the public grant's tree scope would have to cover local/files for this " +
				"to come back, and it must not — this is AP90 at the `default` row",
		},
		{
			what: "this peer's own device declarations",
			path: "/" + pubAP.PeerID() + "/" + workbench.DevicePrefix + strangerAP.PeerID(),
			why:  "who this peer is paired with is nobody's business but its own",
		},
		{
			what: "this peer's folder declarations",
			path: "/" + pubAP.PeerID() + "/" + workbench.FolderPrefix,
			why:  "what this peer shares, and with whom, is not part of a published site",
		},
	} {
		ent, ok, err := strangerAP.Get(probe.path)
		if err == nil && ok {
			t.Errorf("A STRANGER READ %s FROM A PEER THAT ONLY PUBLISHED A SITE.\n"+
				"  path: %s\n"+
				"  got %d bytes of %s back%s\n"+
				"  why this matters: %s",
				strings.ToUpper(probe.what), probe.path, len(ent.Data), ent.Type,
				previewSecret(string(ent.Data)), probe.why)
		}
	}
}

// TestPublicSite_WithoutTheGrantAStrangerGetsNothing is the control arm
// for the test above, and without it that test measures nothing.
//
// Identical setup, one flag removed. If a stranger can verify the site
// with no `default` row, then the row is not what authorized the read in
// the positive arm — a kernel floor grant is, or an open-access default
// somewhere — and the whole file is reporting on a mechanism it is not
// exercising.
func TestPublicSite_WithoutTheGrantAStrangerGetsNothing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	pubAP, strangerAP, pubWS, _ := publicSitePeers(t, ctx)

	// Published — signed, in the tree, complete — and authorized to
	// nobody. That is the default state of this verb and it is the
	// correct one.
	if _, err := pubWS.Publish(ctx, shellcmd.PublishRequest{}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	dialStranger(t, ctx, strangerAP, pubAP)

	if body, err := readSiteLive(ctx, strangerAP, pubAP.PeerID()); err == nil {
		t.Fatalf("A PEER THAT WAS GRANTED NOTHING READ A PUBLISHED SITE (%d bytes).\n"+
			"  `publish` with no -public writes no policy row at all, so this read had to\n"+
			"  be authorized by something else — and whatever that is, it means the\n"+
			"  positive arm in this file is not measuring the `default` row.", len(body))
	}
}

// TestPublicSite_APeerWeShareAFolderWithCanReadItToo is the arm that
// found the defect, and it is not a corner case.
//
// **The kernel resolves the policy table FIRST MATCH WINS**, not as a
// union: `readHandshakePolicyGrants` tries `hex(identityHash)`, then the
// Base58 peer-id, then `default`, and returns at the first hit. So a peer
// with a row of its own — which is every peer you have ever shared a
// folder with — never reaches `default`.
//
// Publishing a site publicly therefore made it readable by every peer in
// the world **except the operator's own other machine**, which is the
// only peer they have to test it with. The site would present as broken
// to the one reader available and fine to everybody unreachable.
//
// The fix is in `desiredGrantsByPeer`: the public site grant is unioned
// into every derived per-peer row. This test fails against the build
// without it — which is the only reason to believe the fix does anything.
func TestPublicSite_APeerWeShareAFolderWithCanReadItToo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	pubAP, friendAP, pubWS, _ := publicSitePeers(t, ctx)

	// Give the friend a policy row of its own, by the ordinary route: a
	// shared folder. Nothing here is about the site.
	sharedDir := filepath.Join(t.TempDir(), "shared")
	if err := os.MkdirAll(sharedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sharedDir, "holiday.txt"),
		[]byte("fine to share\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mounted, err := pubWS.Mount(shellcmd.MountRequest{
		FilesystemDir: sharedDir, TargetPrefix: "archives/shared/",
	})
	if err != nil {
		t.Fatalf("mount shared: %v", err)
	}
	if _, err := pubWS.Share(shellcmd.ShareRequest{
		Peer: friendAP.PeerID(), Root: mounted.RootName,
	}); err != nil {
		t.Fatalf("share: %v", err)
	}

	// Anti-vacuity for THIS arm: the friend must genuinely have a row of
	// its own, or it would reach `default` like any stranger and the test
	// would pass without exercising the shadowing at all.
	if _, ok := workbench.LoadAccessPolicy(pubAP.Store(), friendAP.PeerID()); !ok {
		t.Fatal("the friend has no per-peer policy row, so this test is not measuring " +
			"first-match-wins shadowing — it is measuring the stranger path again")
	}

	if _, err := pubWS.Publish(ctx, shellcmd.PublishRequest{Public: true}); err != nil {
		t.Fatalf("publish -public: %v", err)
	}
	dialStranger(t, ctx, friendAP, pubAP)

	body, err := readSiteLive(ctx, friendAP, pubAP.PeerID())
	if err != nil {
		t.Fatalf("A PEER WE SHARE A FOLDER WITH CANNOT READ OUR PUBLIC SITE: %v\n\n"+
			"  The kernel resolves system/capability/policy/{pattern} FIRST MATCH WINS —\n"+
			"  hex(identityHash), then Base58 peer-id, then `default` — and returns at the\n"+
			"  first one that exists. This peer has a row of its own from the share, so it\n"+
			"  never reaches `default`.\n\n"+
			"  Consequence: publishing publicly makes a site readable by every peer EXCEPT\n"+
			"  the ones the operator has paired with, which are the only ones they can test\n"+
			"  with. The fix is the publicSite union in desiredGrantsByPeer.", err)
	}
	if body != publicPageBody {
		t.Fatalf("public page body mismatch for a shared-with peer:\n got %q\nwant %q",
			body, publicPageBody)
	}
}

// TestPublicSite_ContentGetIsNotNamespaceScoped records what the public
// grant costs beyond the site, as a MEASUREMENT and deliberately not as
// an assertion.
//
// It is not asserted for `TestShareScope_WeRunTheSingleTrustDomainTopology`'s
// reason, one row further out: "content-get refuses an unbound hash"
// fails today, because core-go's `handleGet` is a bare store lookup with
// no §6.4.2 consult; "content-get serves any hash" would encode the
// defect as a premise and fail the day somebody fixes it.
//
// What is new here, and worth more than the restatement: with a `default`
// row the exposure stops being *one named peer who knows a hash* and
// becomes *anybody who can dial this machine and knows a hash*. The tree
// grant is still the only thing standing in the way, because the tree is
// what discloses hashes — which is why the arm above asserts the tree
// boundary and this one only reports.
func TestPublicSite_ContentGetIsNotNamespaceScoped(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	pubAP, strangerAP, pubWS, privateRoot := publicSitePeers(t, ctx)
	if _, err := pubWS.Publish(ctx, shellcmd.PublishRequest{Public: true}); err != nil {
		t.Fatalf("publish -public: %v", err)
	}
	dialStranger(t, ctx, strangerAP, pubAP)

	// The hash of a file in the UNSHARED folder, taken from the
	// publisher's own store. A real attacker has to come by it some other
	// way; the point of the probe is what happens once they have.
	secretPath := "local/files/" + privateRoot + "/secret.txt"
	ent, ok := pubAP.Store().Get(secretPath)
	if !ok {
		t.Skipf("the private file has not been ingested yet at %s; nothing to probe", secretPath)
	}

	res, err := strangerAP.ContentAt(pubAP.PeerID()).Get(ctx, []hash.Hash{ent.ContentHash})
	served := err == nil && len(res.Entities) > 0

	if served {
		t.Logf("MEASURED — the public grant's `system/content:get` is NOT namespace-scoped.\n"+
			"  A peer with no relationship to this one fetched %s, the content of a file in a\n"+
			"  folder that was never shared, purely by knowing its hash.\n"+
			"  EXTENSION-CONTENT §6.4.1 makes namespace-scoped topology a MUST for multi-party\n"+
			"  deployments; core-go's handleGet is a bare store lookup with no §6.4.2 consult,\n"+
			"  and `local/files` chunks mounted files without ever calling system/content:ingest,\n"+
			"  so nothing is bound at {namespace}/{hex(H)} for file bytes at all. Both halves are\n"+
			"  routed and neither is fixable at this tier.\n"+
			"  WHAT HOLDS THE LINE TODAY: the hash is undiscoverable, and the TREE grant is what\n"+
			"  discloses hashes — which is why the tree boundary above is an assertion and this\n"+
			"  is a log. A `default` row widens this from one named peer to anyone who can dial\n"+
			"  this machine.", ent.ContentHash)
	} else {
		t.Logf("MEASURED — `system/content:get` refused a hash outside the published site (%v).\n"+
			"  If that is because EXTENSION-CONTENT §6.4.1 namespace scoping is now enforced,\n"+
			"  this file's tree-grant argument has become a second line of defence rather than\n"+
			"  the only one, and the public grant can be narrowed to the site's namespace.\n"+
			"  Check before celebrating: a refusal for any other reason (no route, no capability\n"+
			"  at all) would read identically here.", err)
	}
}
