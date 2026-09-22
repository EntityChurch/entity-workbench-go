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

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/types"
)

// handler_grant_migration_test.go — the INSTALLED grant, not the declared
// one.
//
// Tier: real-session (TESTING-STRATEGY §4).
//
// # What this is for
//
// On 2026-09-10 the kernel made the executing handler's own grant the gate
// on outbound sub-dispatch (0.8.2.19 Delta E1 / F67). blob-resolve's
// cross-peer `system/content:get` is a sub-dispatch, so a manifest
// declaring no peers dimension stopped being able to fetch a byte —
// measured as `make twopeer-sync` going from green to 18 of 35 checks
// failing, every file assertion among them, in both directions, with and
// without wildcard grants.
//
// Widening the manifest fixes a peer CONSTRUCTED after the change. It does
// nothing for one that already exists, because handler grants are
// install-once: `createHandlerGrants` skips a pattern whose grant is
// already bound, deliberately (Class I, canonical for the identity's
// lifetime). Its own comment says so.
//
// And here is what makes this a test file rather than a comment: **every
// other cross-peer test in this repo runs on a memory store**, which has
// nothing at the grant path and therefore always mints fresh. So the
// manifest fix alone is green across the entire tree and inert on every
// machine an operator runs. A suite that cannot tell the declared scope
// from the installed one is not testing this at all.
//
// # Both tests cross a boundary the product crosses and the suite does not
//
// The control arm crosses PEERS: it installs the pre-E1 scope and measures
// that the fetch genuinely fails, so the migration below cannot pass
// against a build where the scope has no effect — which was the state of
// the world the day before E1 landed, for the entire prior life of this
// handler.
//
// The migration arm crosses a PROCESS: sqlite, close, re-open. That is
// AP39's rule (a derived-state fix has to be gated across a restart,
// because in-process it is green either way) and it is also the operator's
// actual upgrade — they do not swap a capability under a running peer,
// they quit and start the new build.

// preE1BlobResolveScope is blob-resolve's internal scope exactly as it
// shipped before E1: no peers dimension, and a resource scope naming only
// our own content store.
//
// A LITERAL, on purpose, and one of the few places where retyping a
// constant is right: it has to keep meaning "what used to be installed"
// as `BlobResolveInternalScope` moves. If anyone makes this call the live
// function, both tests below go vacuous in the same instant.
func preE1BlobResolveScope() []types.GrantEntry {
	return []types.GrantEntry{
		{
			Handlers:   types.CapabilityScope{Include: []string{"local/files"}},
			Operations: types.CapabilityScope{Include: []string{"write", "delete"}},
			Resources:  types.CapabilityScope{Include: []string{"*"}},
		},
		{
			Handlers:   types.CapabilityScope{Include: []string{"system/content"}},
			Operations: types.CapabilityScope{Include: []string{"get"}},
			Resources:  types.CapabilityScope{Include: []string{"system/content"}},
		},
	}
}

// TestHandlerGrantMigration_PreE1ScopeCannotFetch is the CONTROL ARM.
//
// It asserts the pre-E1 grant genuinely breaks the transfer, which is what
// gives the migration test something to have repaired. It measures through
// `resync` rather than through a live delivery on purpose: a backfill
// reports its per-file errors as a value, so this arm fails with the
// capability refusal in the message instead of as a timeout, and it does
// not depend on any timing the test does not control.
func TestHandlerGrantMigration_PreE1ScopeCannotFetch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	alice := newFlowPeer(t, ctx, "alice")
	bob := newFlowPeer(t, ctx, "bob")

	// The RECEIVER is the peer that fetches: blob-resolve runs on the side
	// taking delivery and dispatches OUT to pull the closure.
	changed, err := bob.ap.RemintHandlerGrant(
		workbench.BlobResolvePattern, preE1BlobResolveScope())
	if err != nil {
		t.Fatalf("could not install the pre-E1 scope on the receiver: %v", err)
	}
	if !changed {
		t.Fatal("installing the pre-E1 scope reported NO CHANGE, so the scope this " +
			"build declares IS the old one — the fix is not in the tree and both " +
			"tests in this file are measuring nothing")
	}

	aliceDir, bobDir, root := shareOneFolderBetween(t, alice, bob, "pree1")

	const body = "written while the receiver holds the pre-E1 grant\n"
	if err := os.WriteFile(filepath.Join(aliceDir, "blocked.md"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	out := awaitBackfill(t, bob, alice.ap.PeerID(), root, 30*time.Second)
	if out.Scanned == 0 {
		t.Fatalf("the receiver enumerated NOTHING from the sender, so this arm says "+
			"nothing about fetching (unreachable=%v listError=%q)",
			out.Unreachable, out.ListError)
	}
	if out.Failed == 0 {
		t.Fatalf("the receiver materialized %d file(s) while holding the PRE-E1 handler "+
			"grant.\n"+
			"  Either the kernel no longer gates outbound sub-dispatch on the executing "+
			"handler's grant, or the scope this build declares is not what makes the "+
			"fetch work. Either way the migration test is measuring nothing and this "+
			"file must be re-derived before it is trusted.", out.Materialized)
	}
	joined := strings.Join(out.Errors, " ")
	if !strings.Contains(joined, "capability") {
		t.Errorf("the fetch failed for a reason that is not a capability refusal, so "+
			"this arm may be pinning an unrelated breakage: %v", out.Errors)
	}
	if _, serr := os.Stat(filepath.Join(bobDir, "blocked.md")); serr == nil {
		t.Fatal("the file reached the receiver's disk despite the backfill reporting " +
			"a failure — the counters and the disk disagree")
	}
	t.Logf("pre-E1 scope refused the fetch as expected: %v", out.Errors)
}

// TestHandlerGrantMigration_RestartMigratesTheInstalledGrant is the real
// operator upgrade: a peer whose store already holds the old grant is
// quit, and started again on the new build.
//
// The assertion is on the INSTALLED scope read back after the second
// bootstrap, and then on bytes crossing between two peers. Reading the
// manifest back would pass against a build that migrates nothing.
func TestHandlerGrantMigration_RestartMigratesTheInstalledGrant(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := shellboot.Config{
		LocalAlias:  "upgrader",
		ListenAddr:  "127.0.0.1:0",
		StorageKind: "sqlite",
		StoragePath: filepath.Join(home, "store.db"),
	}

	// --- first run: put the store in the state an old build left it in --
	ap1, _, err := shellboot.Bootstrap(ctx, cfg)
	if err != nil {
		t.Fatalf("bootstrap 1: %v", err)
	}
	if _, err := ap1.RemintHandlerGrant(
		workbench.BlobResolvePattern, preE1BlobResolveScope()); err != nil {
		t.Fatalf("install pre-E1 scope: %v", err)
	}
	installed, err := ap1.HandlerGrantScope(workbench.BlobResolvePattern)
	if err != nil {
		t.Fatalf("read back the installed scope: %v", err)
	}
	if !sameScope(t, installed, preE1BlobResolveScope()) {
		t.Fatalf("the pre-E1 scope did not take: %+v", installed)
	}
	if err := ap1.Close(); err != nil {
		t.Fatalf("close 1: %v", err)
	}

	// --- second run: same store, new build ------------------------------
	ap2, ws2, err := shellboot.Bootstrap(ctx, cfg)
	if err != nil {
		t.Fatalf("bootstrap 2: %v", err)
	}
	defer func() { _ = ap2.Close() }()

	// THE MEASUREMENT. The kernel will not have re-minted — the grant was
	// already bound, so `createHandlerGrants` skipped it — which means
	// anything true here is true because shellboot migrated it.
	after, err := ap2.HandlerGrantScope(workbench.BlobResolvePattern)
	if err != nil {
		t.Fatalf("read the installed scope after restart: %v", err)
	}
	if !sameScope(t, after, workbench.BlobResolveInternalScope()) {
		t.Fatalf("the restarted peer is STILL on the old handler grant.\n"+
			"  installed: %+v\n"+
			"  declared:  %+v\n"+
			"  Handler grants are install-once, so a manifest change alone reaches "+
			"new peers only. Start at RemintHandlerGrant's call site in "+
			"shellboot/bootstrap.go.", after, workbench.BlobResolveInternalScope())
	}

	// --- and a third run must change NOTHING ---------------------------
	//
	// The mint embeds a CreatedAt, so a migration that re-mints
	// unconditionally moves the grant's content hash at every launch —
	// and a capability whose root moves every launch is worse than the
	// bug being fixed.
	hashBefore, err := grantHashOf(ap2, workbench.BlobResolvePattern)
	if err != nil {
		t.Fatalf("read grant hash: %v", err)
	}
	if err := ap2.Close(); err != nil {
		t.Fatalf("close 2: %v", err)
	}
	ap3, ws3, err := shellboot.Bootstrap(ctx, cfg)
	if err != nil {
		t.Fatalf("bootstrap 3: %v", err)
	}
	defer func() { _ = ap3.Close() }()
	hashAfter, err := grantHashOf(ap3, workbench.BlobResolvePattern)
	if err != nil {
		t.Fatalf("read grant hash after third boot: %v", err)
	}
	if hashBefore != hashAfter {
		t.Fatalf("the handler grant's content hash changed on a launch that had "+
			"nothing to migrate (%s -> %s) — the migration is not idempotent",
			hashBefore, hashAfter)
	}
	_ = ws2

	// --- END TO END: the migrated peer can now receive a file -----------
	li3, err := shellboot.BringUpListener(ctx, ap3, cfg)
	if err != nil {
		t.Fatalf("listener: %v", err)
	}
	defer li3.Cancel()

	sender := newFlowPeer(t, ctx, "sender")
	receiver := &flowPeer{ap: ap3, ws: ws3}

	aliceDir, bobDir, _ := shareOneFolderBetween(t, sender, receiver, "upgraded")

	const body = "written after the peer restarted onto the new build\n"
	if err := os.WriteFile(filepath.Join(aliceDir, "works.md"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if !awaitFileContent(filepath.Join(bobDir, "works.md"), body, 60*time.Second) {
		t.Fatalf("the migrated peer still cannot receive a file.\n"+
			"  installed scope: %+v", after)
	}
}

// --- helpers --------------------------------------------------------

// shareOneFolderBetween runs the shipped share/accept flow between two
// flowPeers and returns the two directories plus the sender's root. The
// receiving directory is deliberately named differently: two peers are not
// required to agree on a filesystem path, and a fixture where they happen
// to agree cannot fail on the field that exists for the case where they do
// not (FolderData.LocalRoot).
func shareOneFolderBetween(t *testing.T, sender, receiver *flowPeer, name string) (senderDir, receiverDir, root string) {
	t.Helper()

	senderDir = filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(senderDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mounted, err := sender.ws.Mount(shellcmd.MountRequest{
		FilesystemDir: senderDir, TargetPrefix: "archives/" + name + "/",
	})
	if err != nil {
		t.Fatalf("sender mount: %v", err)
	}
	root = mounted.RootName

	if _, err := sender.ws.Share(shellcmd.ShareRequest{
		Root: root, Peer: receiver.ap.PeerID(), NowMillis: 1_757_000_000_000,
	}); err != nil {
		t.Fatalf("share: %v", err)
	}
	connectVerb(t, receiver, "sender-"+name, sender.ap.Addr().String())

	receiverDir = filepath.Join(t.TempDir(), name+"-in")
	if _, err := receiver.ws.Accept(shellcmd.AcceptRequest{
		Peer: sender.ap.PeerID(), Root: root, Directory: receiverDir,
	}); err != nil {
		t.Fatalf("accept into %s: %v", receiverDir, err)
	}

	// THE SENDER DIALS LAST, AND THE ORDER IS THE WHOLE POINT.
	//
	// Delivery runs sender → receiver, over the connection the SENDER
	// opened, and the grant carried on that connection is assembled at its
	// handshake. `accept` is what writes the receiver's delivery grant, so
	// a sender that dialled before the accept holds a connection that
	// predates the authority it needs. An earlier version of this helper
	// dialled both sides up front and every live delivery in this file
	// silently never arrived — which reads as a product bug and was the
	// fixture.
	connectVerb(t, sender, "receiver-"+name, receiver.ap.Addr().String())
	return senderDir, receiverDir, root
}

// awaitBackfill polls `resync` until the sender's tree actually holds
// something to enumerate, and returns the last result.
//
// A backfill enumerates the SENDER's tree, and a file written a moment ago
// is on disk before it is in the tree — the watcher has to ingest it
// first. Resyncing once immediately after the write therefore measures the
// watcher's latency and reports it as "their folder is empty", which is
// the confident-wrong-answer shape this repo already named about empty
// registries.
func awaitBackfill(t *testing.T, p *flowPeer, peerID, root string, d time.Duration) shellcmd.BackfillResult {
	t.Helper()
	deadline := time.Now().Add(d)
	var last shellcmd.BackfillResult
	for {
		out, err := p.ws.Resync(peerID, root)
		if err == nil {
			last = out
			if out.Scanned > 0 {
				return out
			}
		}
		if time.Now().After(deadline) {
			return last
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// sameScope compares two grant sets by their canonical encoding, which is
// the same comparison the migration itself makes. Comparing by eye — or by
// checking one field — would let the two drift.
func sameScope(t *testing.T, a, b []types.GrantEntry) bool {
	t.Helper()
	ea, err := ecf.Encode(a)
	if err != nil {
		t.Fatalf("encode scope: %v", err)
	}
	eb, err := ecf.Encode(b)
	if err != nil {
		t.Fatalf("encode scope: %v", err)
	}
	return string(ea) == string(eb)
}

func grantHashOf(ap *entitysdk.AppPeer, pattern string) (string, error) {
	ent, ok, err := ap.Get("system/capability/grants/" + pattern)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", os.ErrNotExist
	}
	return ent.ContentHash.String(), nil
}
