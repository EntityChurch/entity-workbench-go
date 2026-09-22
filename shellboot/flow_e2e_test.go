package shellboot_test

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
	"entity-workbench-go/workbench"
)

// THE WHOLE FLOW: discover → share → permission → mount → change → sync,
// two peers, through the shipped verbs, with NO OpenAccess anywhere.
//
// Tier: real-session (TESTING-STRATEGY §4).
//
// # Why "no OpenAccess" is the headline
//
// `peer.OpenAccessGrants()` is a wildcard that authorizes everything, and
// its own doc calls it development-only. Every cross-peer test in this
// repo has used it — including `TestM2_TwoPeers_OneFolder_ThroughTheVerbs`
// from yesterday, which reaches it through `shellboot.Config.OpenAccess`.
// A flow validated under a wildcard has validated the transport and told
// you nothing about permissions, which is the stage an operator actually
// has to perform.
//
// This test grants exactly what `share` and `accept` write, and nothing
// else. If a grant is missing the run fails; if a grant is unnecessary,
// removing it from the named sets makes this the test that says so.
//
// # The three facts it encodes
//
// Measured in `policy_probe_test.go` and carried here as behaviour:
//
//  1. Authorization is MUTUAL. `share` writes the sender's half, `accept`
//     writes the receiver's. Either alone yields an accepted subscription
//     and an empty folder.
//  2. The grant is assembled at HANDSHAKE, so both verbs re-establish the
//     connection. The assertions on Reconnected are what keep that from
//     silently regressing into a no-op.
//  3. The policy path is keyed on the Base58 peer-id, which is what an
//     operator gets from `peers`.
func TestFlow_DiscoverShareAuthorizeMountSync_NoOpenAccess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	alice := newFlowPeer(t, ctx, "alice")
	bob := newFlowPeer(t, ctx, "bob")

	// --- connect, THROUGH THE VERB ------------------------------------
	//
	// `connect` and not `ap.Connect`, because the verb is what records
	// the dialled address in the workspace — and that address is the only
	// dialable one we have. `ConnectedPeers()` reports the connection's
	// OBSERVED remote address, which for an inbound connection is the
	// dialer's ephemeral source port: it looks like an address and is
	// refused. Driving the SDK directly here would leave the workspace
	// without the address and make `share` unable to re-establish, which
	// is exactly how the first version of this test failed.
	//
	// mDNS announcement is link-local and this test must not depend on
	// the host's avahi, so reachability is established by dialling; what
	// is under test is that the flow works from a peer-id, which is what
	// discovery yields.
	connectVerb(t, bob, "alice", alice.ap.Addr().String())

	// --- discover -----------------------------------------------------
	peers, err := bob.ws.Peers()
	if err != nil {
		t.Fatalf("peers: %v", err)
	}
	if !containsPeer(peers, alice.ap.PeerID()) {
		t.Fatalf("`peers` does not list alice (%s); saw %+v", alice.ap.PeerID(), peers)
	}

	// --- mount, both sides --------------------------------------------
	const folder = "flowtest"
	aliceDir := filepath.Join(t.TempDir(), folder)
	bobDir := filepath.Join(t.TempDir(), folder)
	if err := os.MkdirAll(aliceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bobDir, 0o755); err != nil {
		t.Fatal(err)
	}

	aliceMount, err := alice.ws.Mount(shellcmd.MountRequest{
		FilesystemDir: aliceDir, TargetPrefix: "archives/" + folder + "/",
	})
	if err != nil {
		t.Fatalf("alice mount: %v", err)
	}
	if _, err := bob.ws.Mount(shellcmd.MountRequest{
		FilesystemDir: bobDir, TargetPrefix: "archives/" + folder + "/",
	}); err != nil {
		t.Fatalf("bob mount: %v", err)
	}
	root := aliceMount.RootName

	// --- share (alice authorizes bob, and says what is on offer) ------
	shareOut, err := alice.ws.Share(shellcmd.ShareRequest{
		Root: root, Peer: bob.ap.PeerID(), NowMillis: 1_756_000_000_000,
	})
	if err != nil {
		t.Fatalf("share: %v", err)
	}
	// NOT asserted as reconnected, and that is the realistic case rather
	// than a weakened assertion. Bob dialled alice, so alice holds an
	// INBOUND connection and no address she could dial back — her
	// ConnectedPeers() entry carries bob's ephemeral source port. The
	// operator who shares is frequently the one who cannot re-establish,
	// which is why the burden has to sit on `accept`.
	t.Logf("share reconnected=%v note=%q", shareOut.Reconnected, shareOut.ReconnectNote)

	// --- permission is a thing you can inspect ------------------------
	//
	// An authorization an operator cannot list is one they cannot audit
	// or revoke. Asserted as a surface, not just as a tree write.
	policies, problems := alice.ws.AccessPolicies()
	if len(problems) > 0 {
		t.Fatalf("access policies did not decode: %v", problems)
	}
	// The kernel seeds a wildcard entry for the peer ITSELF (V7 v7.74
	// §6.9a), so "exactly one row" is the wrong assertion and the first
	// version of this test made it. What must hold: exactly one row that
	// is not us, it is bob, and the self row is MARKED — an access listing
	// showing `*:*` against an unexplained hex string has invented a
	// security scare.
	var granted []workbench.AccessPolicy
	sawSelf := false
	for _, p := range policies {
		if p.IsSelf {
			sawSelf = true
			continue
		}
		granted = append(granted, p)
	}
	if !sawSelf {
		t.Error("the peer's own seed entry is not marked IsSelf — a surface listing " +
			"it would present a wildcard grant as a grant to a stranger")
	}
	if len(granted) != 1 || granted[0].PeerID != bob.ap.PeerID() {
		t.Fatalf("alice granted %+v, want exactly bob", granted)
	}
	if !strings.Contains(granted[0].Summary, "system/content:get") {
		t.Errorf("the grant summary does not mention the content read that makes "+
			"a sync work: %q", granted[0].Summary)
	}

	// --- the offer is READABLE FROM THE OTHER PEER --------------------
	//
	// This is the step that removes the out-of-band instruction. Without
	// it bob has to be told a root name by some other channel, which is
	// the difference between a share and a configuration exercise.
	offers, err := bob.ws.Offers(alice.ap.PeerID())
	if err != nil {
		t.Fatalf("offers: %v", err)
	}
	if len(offers) != 1 || offers[0].Root != root {
		t.Fatalf("bob sees offers %+v from alice, want one rooted at %q", offers, root)
	}

	// --- accept (bob authorizes alice's deliveries, then syncs) -------
	acceptOut, err := bob.ws.Accept(shellcmd.AcceptRequest{
		Peer: alice.ap.PeerID(), Root: offers[0].Root,
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if !acceptOut.Reconnected {
		t.Fatalf("accept did not re-establish the connection (%s)", acceptOut.ReconnectNote)
	}

	// --- the step neither verb can perform alone ----------------------
	//
	// A dial-by-address authorizes the DIALER only — `sendReciprocalGrant`
	// is gated on `EstablishedViaRendezvousKey()` and the kernel's own
	// comment says a dial-by-address "is asymmetric — one party requested
	// service". Bob dialling alice got bob the right to subscribe and
	// fetch. Delivery runs alice→bob, so ALICE must dial BOB once, after
	// bob's accept wrote the delivery grant.
	//
	// On a LAN this address comes from mDNS, which announces the port a
	// peer LISTENS on. Here it is passed directly, because a test must
	// not depend on the host's avahi.
	if acceptOut.PublisherMustDial == "" {
		t.Error("accept did not report the publisher-side dial — that step is " +
			"invisible from the receiving machine and the flow does not complete " +
			"without it")
	}
	connectVerb(t, alice, "bob", bob.ap.Addr().String())

	// --- change a file, and it crosses --------------------------------
	const body = "the whole flow, no wildcard grants\n"
	if err := os.WriteFile(filepath.Join(aliceDir, "note.md"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	landed := filepath.Join(bobDir, "note.md")
	if !awaitFileContent(landed, body, 45*time.Second) {
		got, rerr := os.ReadFile(landed)
		t.Fatalf("note.md never reached bob (read %q, err %v)\n"+
			"  alice granted bob: %s\n"+
			"  bob granted alice: %s",
			string(got), rerr, shareOut.GrantSummary, acceptOut.GrantSummary)
	}

	// A CHANGE, not just a create — the operator's actual loop is editing
	// a file that already synced, and an implementation that only handled
	// creates would pass everything above.
	const edited = "the whole flow, edited\n"
	if err := os.WriteFile(filepath.Join(aliceDir, "note.md"), []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	if !awaitFileContent(landed, edited, 45*time.Second) {
		t.Fatal("the file synced once and the EDIT never arrived")
	}

	// --- withdrawal is real, and states its limit ---------------------
	un, err := alice.ws.Unshare(root, bob.ap.PeerID())
	if err != nil {
		t.Fatalf("unshare: %v", err)
	}
	if !un.PolicyRemoved {
		t.Error("unshare left bob's access policy in place though he is in no other share")
	}
	if un.Caveat == "" {
		t.Error("unshare reports no caveat — withdrawal only affects the NEXT " +
			"handshake and a surface that does not say so is overclaiming")
	}
	if _, ok := workbench.LoadAccessPolicy(alice.ap.Store(), bob.ap.PeerID()); ok {
		t.Error("the policy entry survived unshare")
	}
	// The bytes bob already received are his.
	if _, err := os.Stat(landed); err != nil {
		t.Errorf("unshare deleted a file bob had already received: %v", err)
	}
}

// TestFlow_ShareAloneDeliversNothing is the control arm, and it is the
// most valuable test in this file.
//
// It pins the failure mode that a one-directional authorization produces:
// the subscription is ACCEPTED and no file ever arrives. If someone later
// "simplifies" `accept` into a plain sync, or drops the receiver-side
// grant as redundant, everything in the happy-path test still passes
// right up to the file assertion — and this test says why.
func TestFlow_ShareAloneDeliversNothing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	alice := newFlowPeer(t, ctx, "alice")
	bob := newFlowPeer(t, ctx, "bob")

	const folder = "halfshare"
	aliceDir := filepath.Join(t.TempDir(), folder)
	bobDir := filepath.Join(t.TempDir(), folder)
	if err := os.MkdirAll(aliceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	aliceMount, err := alice.ws.Mount(shellcmd.MountRequest{
		FilesystemDir: aliceDir, TargetPrefix: "archives/" + folder + "/",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bob.ws.Mount(shellcmd.MountRequest{
		FilesystemDir: bobDir, TargetPrefix: "archives/" + folder + "/",
	}); err != nil {
		t.Fatal(err)
	}

	// Share BEFORE connecting, so bob's very first handshake carries
	// alice's grant. That is both the simpler operator order and the one
	// that isolates the variable: with no reconnect in play, the only
	// thing missing is the RECEIVER's half.
	if _, err := alice.ws.Share(shellcmd.ShareRequest{
		Root: aliceMount.RootName, Peer: bob.ap.PeerID(), NowMillis: 1_756_000_000_000,
	}); err != nil {
		t.Fatalf("share: %v", err)
	}
	connectVerb(t, bob, "alice", alice.ap.Addr().String())
	connectVerb(t, alice, "bob", bob.ap.Addr().String())

	// Bob syncs WITHOUT accepting — no receiver-side delivery grant.
	if _, err := bob.ws.Sync(shellcmd.SyncRequest{
		Remote: alice.ap.PeerID(), Root: aliceMount.RootName,
	}); err != nil {
		t.Fatalf("the subscribe itself should be authorized by alice's share: %v", err)
	}

	const body = "should not arrive\n"
	if err := os.WriteFile(filepath.Join(aliceDir, "x.md"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if awaitFileContent(filepath.Join(bobDir, "x.md"), body, 12*time.Second) {
		t.Fatal("a file arrived with only the SENDER's half of the authorization — " +
			"if this is genuinely allowed then `accept`'s receiver-side grant is " +
			"unnecessary and both it and this test should go")
	}
}

// --- helpers --------------------------------------------------------

type flowPeer struct {
	ap *entitysdk.AppPeer
	ws *shellcmd.ShellWorkspace
}

func newFlowPeer(t *testing.T, ctx context.Context, name string) *flowPeer {
	t.Helper()
	// NOTE THE ABSENT FIELD: no OpenAccess. That omission is the test.
	ap, ws, err := shellboot.Bootstrap(ctx, shellboot.Config{
		LocalAlias: name,
		ListenAddr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("bootstrap %s: %v", name, err)
	}
	t.Cleanup(func() { _ = ap.Close() })
	bringUpListener(t, ctx, ap, name)
	return &flowPeer{ap: ap, ws: ws}
}

// connectVerb dials through the shipped `connect` verb, which is what
// registers the dialled address in the workspace.
func connectVerb(t *testing.T, from *flowPeer, alias, addr string) {
	t.Helper()
	sh := shellcmd.NewShellInWorkspace(from.ws)
	if _, err := shellcmd.Default().Dispatch(sh, "connect", []string{alias, addr}); err != nil {
		t.Fatalf("connect %s %s: %v", alias, addr, err)
	}
}

func containsPeer(peers []shellcmd.DiscoveredPeer, id string) bool {
	for _, p := range peers {
		if p.PeerID == id {
			return true
		}
	}
	return false
}
