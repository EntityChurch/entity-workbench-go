package shellcmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"
)

// status_test.go — the read/pass split, and what each half is allowed to
// do.
//
// The property under test is not "StatusSnapshot returns rows". It is
// that a snapshot **changes nothing** while a pass does, because that is
// the entire reason the two functions exist: a panel refreshes, and a
// reconcile pass dials every declared peer. A status surface that
// reconciled on every refresh would be a dialer with a table in it.
//
// Every test below that asserts a snapshot did NOT do something runs the
// reconcile pass as its control arm on the same fixture. Without that arm
// the assertion is satisfied by a StatusSnapshot that does nothing at
// all, which is the vacuous shape this area produced twice already
// (AP43).

const themPeer = "2KLf7osYcMLEdmSnYLx3Bg6TScaGJefLZJdKaL1tPNHAbR"

// mountableFixture is reconcileFixture plus the one handler a mount needs
// wired.
//
// `Mount` refuses without `ws.NotificationIngest`, which `shellboot`
// supplies in every shipped binary and which the bare workspace
// constructor does not. Two tests here mount for real, because the
// property under test — that a remount uses the path the DECLARATION
// remembers — is not observable against a stubbed mount: the whole point
// is that a real local-files root appears where the record said.
func mountableFixture(t *testing.T) (*ShellWorkspace, *workbench.Store) {
	t.Helper()
	ingest := workbench.NewNotificationIngestHandler(nil)
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		Handlers: []entitysdk.HandlerRegistration{
			{Pattern: workbench.NotificationIngestPattern, Handler: ingest},
			{Pattern: workbench.ChainErrorsPattern, Handler: workbench.NewChainErrorsHandler()},
		},
	})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })
	ws := NewShellWorkspace(ap, "self", "")
	ws.NotificationIngest = ingest
	return ws, ap.Store()
}

// TestStatusSnapshot_ReadsWithoutWritingPolicy is the load-bearing one.
//
// The declarations authorize a peer in both directions and no policy row
// exists yet. A snapshot must leave it that way; a pass must write it.
// The two halves are the same fixture and the same declared state, so the
// only variable is which function ran.
func TestStatusSnapshot_ReadsWithoutWritingPolicy(t *testing.T) {
	ws, st := reconcileFixture(t)
	declareBothDirections(t, st, themPeer)
	// The verbs declare a DEVICE alongside the folder; declareBothDirections
	// writes only the folders, so the device is added here to reproduce
	// the state `share` + `accept` actually leave behind.
	if err := workbench.SaveDevice(st, workbench.DeviceData{PeerID: themPeer, Label: "desk-2"}); err != nil {
		t.Fatal(err)
	}

	if _, ok := workbench.LoadAccessPolicy(st, themPeer); ok {
		t.Fatalf("premise broken: a policy row exists before anything ran")
	}

	snap, err := ws.StatusSnapshot()
	if err != nil {
		t.Fatalf("StatusSnapshot: %v", err)
	}
	if snap.Reconciled {
		t.Errorf("a snapshot reported Reconciled=true; a surface would caption a read as a verification")
	}
	if len(snap.Actions) != 0 {
		t.Errorf("a read produced actions: %v", snap.Actions)
	}
	if _, ok := workbench.LoadAccessPolicy(st, themPeer); ok {
		t.Fatalf("StatusSnapshot wrote a capability policy row — a read must not authorize anybody")
	}
	if len(snap.Devices) != 1 || len(snap.Folders) != 2 {
		t.Fatalf("snapshot saw %d device(s) and %d folder(s), want 1 and 2",
			len(snap.Devices), len(snap.Folders))
	}

	// Control arm. If this does not write the row, the assertion above is
	// satisfied by a function that cannot do anything, and the test is
	// measuring nothing.
	pass, err := ws.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if !pass.Reconciled {
		t.Errorf("a pass reported Reconciled=false")
	}
	if _, ok := workbench.LoadAccessPolicy(st, themPeer); !ok {
		t.Fatalf("control arm: the reconcile pass wrote no policy row either — " +
			"the snapshot assertion above proves nothing")
	}
}

// TestStatusSnapshot_ReportsTheSameProblemAsAPass pins the shared
// diagnostic. Both surfaces read the same records, so a folder with no
// mount must produce the same sentence whichever function an operator
// reached for — and the sentence names the action, because the loop
// deliberately will not take it.
func TestStatusSnapshot_ReportsTheSameProblemAsAPass(t *testing.T) {
	ws, st := reconcileFixture(t)
	if err := workbench.SaveFolder(st, workbench.FolderData{
		ID: "photos", Label: "photos", Kind: "files",
		Root: "photos", Path: "/tmp/photos", Origin: "local",
	}); err != nil {
		t.Fatal(err)
	}

	snap, err := ws.StatusSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	pass, err := ws.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Problems) != 1 || len(pass.Problems) != 1 {
		t.Fatalf("snapshot problems %v, pass problems %v — want exactly one each",
			snap.Problems, pass.Problems)
	}
	if snap.Problems[0] != pass.Problems[0] {
		t.Errorf("the read and the pass describe the same state differently:\n  read: %s\n  pass: %s",
			snap.Problems[0], pass.Problems[0])
	}
	if !strings.Contains(snap.Problems[0], "remount it") {
		t.Errorf("the problem does not name the action a surface should offer: %s", snap.Problems[0])
	}
}

// TestStatusSnapshot_FileCountsAreUnobservableWithoutAMount is AP59's
// rule at this surface: an absent mount must render as *unknown*, not as
// a confident zero. "Nothing has arrived" and "there is nowhere for it to
// arrive" are different claims, and the second is usually the whole
// problem.
func TestStatusSnapshot_FileCountsAreUnobservableWithoutAMount(t *testing.T) {
	ws, st := mountableFixture(t)
	// Named "photos" on disk, because a mount root is derived from the
	// directory's BASENAME — the same coupling that made a received
	// folder have to be named after the sender's.
	dir := filepath.Join(t.TempDir(), "photos")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := workbench.SaveFolder(st, workbench.FolderData{
		ID: "photos", Label: "photos", Kind: "files",
		Root: "photos", Path: dir, Origin: "local",
	}); err != nil {
		t.Fatal(err)
	}

	snap, err := ws.StatusSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Folders[0].FilesObservable {
		t.Errorf("a folder with no mount reported its file count as observable")
	}

	// Mount it, and the same folder becomes countable — with a real zero,
	// which is now a claim we can make.
	if _, err := ws.Mount(MountRequest{FilesystemDir: dir, TargetPrefix: "archives/photos/"}); err != nil {
		t.Fatalf("mount: %v", err)
	}
	snap, err = ws.StatusSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Folders[0].FilesObservable {
		t.Fatalf("a mounted folder still reports its file count as unobservable")
	}
	if !snap.Folders[0].Mounted {
		t.Errorf("a mounted folder reports Mounted=false")
	}
}

// TestSetDevicePaused_ChangesTheDeclarationAndTheLoopObeysIt.
//
// Pausing writes the DECLARATION, not the substrate. The gate is that a
// subsequent pass leaves the peer alone — with the un-paused run as the
// control arm, because a device whose address is dead produces a problem
// only when something actually tries to reach it.
func TestSetDevicePaused_ChangesTheDeclarationAndTheLoopObeysIt(t *testing.T) {
	ws, st := reconcileFixture(t)
	// 127.0.0.1:1 is a port nothing listens on, so an attempt to maintain
	// this peer fails and says so. That failure is the signal.
	if err := workbench.SaveDevice(st, workbench.DeviceData{
		PeerID: themPeer, Label: "desk-2", Addresses: []string{"127.0.0.1:1"},
	}); err != nil {
		t.Fatal(err)
	}

	// Control arm first: un-paused, the loop reaches for this peer.
	active, err := ws.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(active.Problems) == 0 {
		t.Fatalf("control arm: an unreachable declared peer produced no problem, " +
			"so the paused assertion below cannot distinguish anything")
	}

	if _, err := ws.SetDevicePaused(themPeer, true); err != nil {
		t.Fatalf("SetDevicePaused: %v", err)
	}
	d, ok := workbench.LoadDevice(st, themPeer)
	if !ok || !d.Paused {
		t.Fatalf("pausing did not reach the declaration (found=%v paused=%v)", ok, d.Paused)
	}

	paused, err := ws.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(paused.Problems) != 0 {
		t.Errorf("the loop still reached for a paused peer: %v", paused.Problems)
	}
	if len(paused.Devices) != 1 || !paused.Devices[0].Paused {
		t.Fatalf("the paused device is not reported as paused: %+v", paused.Devices)
	}

	// And it comes back. A pause that could not be undone would be a
	// forget with a friendlier name.
	if _, err := ws.SetDevicePaused(themPeer, false); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if d, _ := workbench.LoadDevice(st, themPeer); d.Paused {
		t.Errorf("resuming did not reach the declaration")
	}
}

// TestRemountFolder_UsesThePathTheDeclarationRemembers.
//
// The reconciler names this action and refuses to take it, because
// creating a mount writes to somebody's disk. The surface offers it — and
// the path comes from the record, never from a caller, which is the whole
// reason FolderData carries one.
func TestRemountFolder_UsesThePathTheDeclarationRemembers(t *testing.T) {
	ws, st := mountableFixture(t)
	dir := filepath.Join(t.TempDir(), "photos")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := workbench.SaveFolder(st, workbench.FolderData{
		ID: "photos", Label: "photos", Kind: "files",
		Root: "photos", Path: dir, Origin: "local",
	}); err != nil {
		t.Fatal(err)
	}

	// Premise: the folder is declared and unmounted, and the status
	// surface says so.
	before, err := ws.StatusSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if before.Folders[0].Mounted {
		t.Fatalf("premise broken: the folder is already mounted")
	}

	out, err := ws.RemountFolder("photos")
	if err != nil {
		t.Fatalf("RemountFolder: %v", err)
	}
	if out.FilesystemRoot != dir {
		t.Errorf("remounted %q, want the recorded path %q", out.FilesystemRoot, dir)
	}

	after, err := ws.StatusSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !after.Folders[0].Mounted {
		t.Errorf("the folder is still reported unmounted after a remount")
	}
	if len(after.Problems) != 0 {
		t.Errorf("the remount left problems behind: %v", after.Problems)
	}

	// Twice is a refusal, not a second mount. Remounting over a live
	// mount would replace a watcher that is working.
	if _, err := ws.RemountFolder("photos"); err == nil {
		t.Errorf("remounting an already-mounted folder was allowed")
	}
}

// TestRemountFolder_RefusesAFolderWithNoRecordedPath — a refusal rather
// than a guess (AP33). A record with no path is one whose mount was gone
// before the record learned where it was, and inventing a directory would
// bridge a path the operator never chose.
func TestRemountFolder_RefusesAFolderWithNoRecordedPath(t *testing.T) {
	ws, st := reconcileFixture(t)
	if err := workbench.SaveFolder(st, workbench.FolderData{
		ID: "photos", Label: "photos", Kind: "files", Root: "photos", Origin: "local",
	}); err != nil {
		t.Fatal(err)
	}
	_, err := ws.RemountFolder("photos")
	if err == nil {
		t.Fatalf("a folder with no recorded directory was remounted anyway")
	}
	if !strings.Contains(err.Error(), "does not record a directory") {
		t.Errorf("the refusal does not say why: %v", err)
	}
	if _, err := ws.RemountFolder("no-such-folder"); err == nil {
		t.Errorf("remounting an undeclared folder was allowed")
	}
}

// TestDeviceLabelFor_PrefersTheDeclarationOverTheConnectionAlias.
//
// `AliasFor` reads the connection map, which is empty until something
// dials. After a restart that is every row, so a status table built on it
// would show bare peer-ids for peers the operator has named. The declared
// label is durable, which is the point of declaring it.
func TestDeviceLabelFor_PrefersTheDeclarationOverTheConnectionAlias(t *testing.T) {
	ws, st := reconcileFixture(t)
	if ws.AliasFor(themPeer) != "" {
		t.Fatalf("premise broken: a connection alias exists with no connection")
	}
	if got := ws.DeviceLabelFor(themPeer); got != themPeer {
		t.Errorf("an unknown peer rendered as %q, want the peer-id", got)
	}
	if err := workbench.SaveDevice(st, workbench.DeviceData{PeerID: themPeer, Label: "desk-2"}); err != nil {
		t.Fatal(err)
	}
	if got := ws.DeviceLabelFor(themPeer); got != "desk-2" {
		t.Errorf("DeviceLabelFor = %q, want the declared label", got)
	}
}

// TestStatusSnapshot_OutboundIsStatedAndInboundIsNot.
//
// What we grant them is our own row and is exactly knowable. There is no
// inbound counterpart anywhere in the outcome, deliberately: their
// capability table is not readable from here, and a field claiming to
// know it would be wrong precisely when it mattered.
func TestStatusSnapshot_OutboundIsStatedAndInboundIsNot(t *testing.T) {
	ws, st := reconcileFixture(t)
	if err := workbench.SaveDevice(st, workbench.DeviceData{PeerID: themPeer, Label: "desk-2"}); err != nil {
		t.Fatal(err)
	}

	snap, err := ws.StatusSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if got := snap.Devices[0].OutboundGrant; got != "(nothing)" {
		t.Errorf("a peer we authorize for nothing reported %q, want an explicit nothing", got)
	}

	if err := workbench.SaveAccessPolicy(st, themPeer, workbench.SyncSenderGrants(), "test"); err != nil {
		t.Fatal(err)
	}
	snap, err = ws.StatusSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if got := snap.Devices[0].OutboundGrant; got == "" || got == "(nothing)" {
		t.Errorf("an authorized peer reported no outbound grant (%q)", got)
	}
}

// TestRememberDeviceAddress_UpdatesADeclarationAndNeverCreatesOne.
//
// The address an operator types is the one fact about a peer that
// nothing else can supply, and until 2026-09-03 it lived only in process
// memory: `connect` put it in `Conns` and in the kernel's pool, and
// nothing put it in the declaration. Restart the peer and the reconciler
// had nothing to dial with.
//
// The second half is the design constraint. Connecting is a MEANS, not a
// gesture — dialing a peer to look at its tree must not enroll it in a
// relationship the reconciler then maintains forever. So this updates an
// existing declaration and never creates one, and both arms are asserted
// because the create-anyway version passes the first arm on its own.
func TestRememberDeviceAddress_UpdatesADeclarationAndNeverCreatesOne(t *testing.T) {
	ws, st := reconcileFixture(t)

	// Arm 1: an undeclared peer stays undeclared.
	ws.RememberDeviceAddress(themPeer, "127.0.0.1:9402")
	if _, ok := workbench.LoadDevice(st, themPeer); ok {
		t.Fatalf("connecting to an undeclared peer created a device record — " +
			"the reconciler would then maintain a connection to a peer nobody paired with")
	}

	// Arm 2: a declared peer learns the address, durably.
	if err := workbench.SaveDevice(st, workbench.DeviceData{PeerID: themPeer, Label: "desk-2"}); err != nil {
		t.Fatal(err)
	}
	ws.RememberDeviceAddress(themPeer, "127.0.0.1:9402")
	d, ok := workbench.LoadDevice(st, themPeer)
	if !ok || d.PreferredAddress() != "127.0.0.1:9402" {
		t.Fatalf("the declaration did not learn the address (found=%v addr=%q)", ok, d.PreferredAddress())
	}

	// And the status surface reports it, which is how an operator can
	// tell "this will re-dial itself" from "by peer-id only".
	snap, err := ws.StatusSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Devices[0].Address != "127.0.0.1:9402" {
		t.Errorf("status reports address %q, want the declared one", snap.Devices[0].Address)
	}
}

// TestReconcile_OpensOurOwnOutboundRouteOncePerProcess.
//
// A connection has a direction for AUTHORITY and none for display: a
// dial-by-address authorizes the dialer only (AP63), so deliveries from
// us to them run over the connection WE opened. After a restart we have
// opened none — but theirs is still in our pool, so everything reports
// "connected" while nothing can be dispatched.
//
// The gate is on the once-per-process bound rather than on a successful
// dial, because the dial needs a live peer and the bound is the part that
// is easy to get wrong: dialing on EVERY pass would make the loop an
// outage generator, which is the same reason the policy write is
// conditional.
func TestReconcile_OpensOurOwnOutboundRouteOncePerProcess(t *testing.T) {
	ws, st := reconcileFixture(t)
	// 127.0.0.1:1 is a port nothing listens on: the dial fails, which is
	// what makes the attempt observable as a problem line.
	if err := workbench.SaveDevice(st, workbench.DeviceData{
		PeerID: themPeer, Label: "desk-2", Addresses: []string{"127.0.0.1:1"},
	}); err != nil {
		t.Fatal(err)
	}

	first, err := ws.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !mentions(first.Problems, "could not open our own connection") {
		t.Fatalf("the first pass of a process did not try to open our own outbound "+
			"connection: %v", first.Problems)
	}

	// A failed dial is NOT marked, so the loop keeps trying — a peer that
	// is switched off must not be given up on for the life of the
	// process.
	second, err := ws.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !mentions(second.Problems, "could not open our own connection") {
		t.Errorf("a failed dial was not retried on the next pass: %v", second.Problems)
	}
}

// TestReconcile_SaysNothingAboutDialingAPeerWeOnlyRECEIVEFrom.
//
// The outbound route matters for a folder we PUBLISH; for one we only
// receive, the other side dispatches to us and our own dial is not what
// carries it. Reporting a problem there would be noise on a working
// relationship, and noise on a status surface is how people learn to
// ignore it.
func TestReconcile_SaysNothingAboutDialingAPeerWeOnlyRECEIVEFrom(t *testing.T) {
	ws, st := reconcileFixture(t)
	if err := workbench.SaveDevice(st, workbench.DeviceData{PeerID: themPeer, Label: "desk-2"}); err != nil {
		t.Fatal(err)
	}
	// A received folder only — no address, nothing to dial with.
	id := workbench.ReceivedFolderID(themPeer, "notes")
	if err := workbench.SaveFolder(st, workbench.FolderData{
		ID: id, Label: "notes", Kind: "files", Root: "notes", Origin: themPeer,
	}.WithPeerState(themPeer, workbench.FolderStateAccepted, 1, "")); err != nil {
		t.Fatal(err)
	}
	out, err := ws.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if mentions(out.Problems, "we publish a folder to this peer") {
		t.Errorf("the loop complained about dialing a peer we only receive from: %v", out.Problems)
	}

	// Control arm: declare a folder we DO publish to them, and the same
	// pass now says so. Without this the assertion above passes against a
	// loop that never produces the message at all.
	if err := workbench.SaveFolder(st, workbench.FolderData{
		ID: "photos", Label: "photos", Kind: "files", Root: "photos", Origin: "local",
	}.WithPeerState(themPeer, workbench.FolderStateOffered, 1, "")); err != nil {
		t.Fatal(err)
	}
	out2, err := ws.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !mentions(out2.Problems, "we publish a folder to this peer") {
		t.Errorf("control arm: a published folder with no dialable address produced no "+
			"problem, so the assertion above proves nothing: %v", out2.Problems)
	}
}

func mentions(lines []string, sub string) bool {
	for _, l := range lines {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}
