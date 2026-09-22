package shellboot_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"entity-workbench-go/shellboot"
	"entity-workbench-go/shellcmd"

	"go.entitychurch.org/entity-core-go/ext/localfiles"
)

// M2b: a delivered file is an ENTITY on the receiver, not only bytes.
//
// Tier: real-session (TESTING-STRATEGY §4).
//
// # Why this test exists
//
// Every sync test in this tree asserted on **bytes on disk**. Not one
// read the receiver's tree. So the entire tree-side half of the receive
// path — the half M3's conflict detection, `blob_resolve`'s F9
// already-current check and `revision` all depend on — had zero
// coverage, and a claim that it did not work at all was consistent with
// a fully green suite.
//
// That claim was made on 2026-09-06 ("the receiving peer binds no file
// entity") and it is FALSE; see the handoff of 2026-09-07 for the
// measurement that produced it. This test is the assertion whose absence
// let it stand for a day.
//
// # What it asserts, and why each arm is here
//
//   - The receiver's LOCATION INDEX has a binding at the target path,
//     read from the live index rather than from any surface.
//   - The bound entity is a local-files file entity whose blob hash
//     EQUALS the sender's. That equality is the comparison point M3's
//     conflict detection is built on (blob_resolve.go's F9 branch), so
//     asserting existence alone would leave the interesting property
//     untested.
//   - The `ls` verb reports the same file. The podman harness cannot
//     read a peer's store directly, so it has to ask the shell; gating
//     the surface against the index here is what makes that assertion
//     trustworthy over there.
//
// The four arms vary the two things that differ between this test and
// the containerised run: the storage backend (memory vs. sqlite), and
// whether the two peers named the folder the same (`accept` takes a
// directory, so a received folder has two root names — AGENTS.md).
func TestM2b_TheReceiverBindsTheDeliveredFile(t *testing.T) {
	for _, tc := range []struct {
		name       string
		storage    string
		recvFolder string // "" means the same name as the sender's
	}{
		{"memory_symmetric", "", ""},
		{"memory_asymmetric", "", "received"},
		{"sqlite_symmetric", "sqlite", ""},
		{"sqlite_asymmetric", "sqlite", "received"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertReceiverBinds(t, tc.storage, tc.recvFolder)
		})
	}
}

func assertReceiverBinds(t *testing.T, storage, recvFolder string) {
	senderDir := t.TempDir()
	receiverDir := t.TempDir()

	sender := newStorageTestPeer(t, "sender", storage)
	receiver := newStorageTestPeer(t, "receiver", storage)
	connectPeers(t, sender, receiver)

	const folder = "shared"
	if recvFolder == "" {
		recvFolder = folder
	}
	senderMount := filepath.Join(senderDir, folder)
	receiverMount := filepath.Join(receiverDir, recvFolder)
	mkdirOrFail(t, senderMount)
	mkdirOrFail(t, receiverMount)

	senderOut := mountOrFail(t, sender, senderMount, "archives/"+folder+"/")
	recvOut := mountOrFail(t, receiver, receiverMount, "archives/"+recvFolder+"/")
	root, localRoot := senderOut.RootName, recvOut.RootName

	if _, err := receiver.ws.Sync(shellcmd.SyncRequest{
		Remote:     sender.id,
		Root:       root,
		TargetRoot: localRoot,
	}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	const body = "# M2b\n\nThe receiver stores this, not only writes it.\n"
	const name = "note.md"
	if err := os.WriteFile(filepath.Join(senderMount, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write on sender: %v", err)
	}
	if !awaitFileContent(filepath.Join(receiverMount, name), body, 30*time.Second) {
		t.Fatal("precondition failed: the file never reached the receiver's disk")
	}

	// The binding, from the live index. The index and not a surface,
	// because the surface is what this test exists to certify.
	//
	// A store copied out from under a running peer is NOT the store:
	// file-backed SqliteStore opens WAL (core/store/sqlite.go,
	// buildSqliteDSN defaults JournalMode to "WAL"), so writes since the
	// last checkpoint live in the `-wal` sidecar. Reading the main file
	// alone returns zero rows and no error, which is how "the receiver
	// binds nothing" came to be reported. Measured: 0 rows against the
	// copied main file, 2 against main+WAL, same instant, same store.
	qualified := "/" + receiver.id + "/local/files/" + localRoot + "/" + name
	h, bound := receiver.ap.RawLocationIndex().Get(qualified)
	if !bound {
		t.Fatalf("the receiver has the bytes on disk and no binding at %s\n"+
			"  receiver bindings under local/files: %v",
			qualified, bindingPaths(receiver, "/"+receiver.id+"/local/files"))
	}

	ent, ok := receiver.ap.RawContentStore().Get(h)
	if !ok {
		t.Fatalf("the receiver bound %s to a hash its own store does not hold", qualified)
	}
	if ent.Type != localfiles.TypeFile {
		t.Fatalf("the receiver bound %s to a %s, want %s", qualified, ent.Type, localfiles.TypeFile)
	}
	recvFile, err := localfiles.FileDataFromEntity(ent)
	if err != nil {
		t.Fatalf("decode the receiver's file entity: %v", err)
	}

	// The blob hashes must MATCH. This is the comparison F9 makes, and
	// the one M3's conflict detection will branch on: equal means the
	// receiver is current, different means somebody edited. A test that
	// stopped at "a binding exists" would pass with the two sides
	// pointing at unrelated content.
	senderQualified := "/" + sender.id + "/local/files/" + root + "/" + name
	sh, sbound := sender.ap.RawLocationIndex().Get(senderQualified)
	if !sbound {
		t.Fatalf("control arm broken: the sender has no binding at %s", senderQualified)
	}
	sent, _ := sender.ap.RawContentStore().Get(sh)
	sendFile, err := localfiles.FileDataFromEntity(sent)
	if err != nil {
		t.Fatalf("decode the sender's file entity: %v", err)
	}
	if recvFile.Content != sendFile.Content {
		t.Fatalf("blob hash differs across the two peers: receiver %s, sender %s — "+
			"the F9 already-current check can never fire and conflict detection "+
			"has no comparison point", recvFile.Content, sendFile.Content)
	}

	// And the surface agrees. The containerised harness has no way to
	// read a peer's store — the alpine image ships no sqlite3, and a
	// copied WAL store lies — so it asks the shell instead. That is only
	// sound if the shell and the index cannot disagree.
	if !listingNames(t, receiver, "local/files/"+localRoot+"/")[name] {
		t.Errorf("`ls local/files/%s/` does not report %s, which the location "+
			"index has bound — the surface the podman harness relies on "+
			"disagrees with the store", localRoot, name)
	}
}

// --- helpers -------------------------------------------------------

// newStorageTestPeer is newSyncTestPeer with the storage backend as a
// parameter. Every other in-process test runs memory while the
// containerised runs use sqlite, so the backend is a live variable
// between the two and has to be controlled rather than assumed
// irrelevant.
func newStorageTestPeer(t *testing.T, name, storage string) *syncTestPeer {
	t.Helper()
	if storage != "sqlite" {
		return newSyncTestPeer(t, name)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	// A per-peer HOME, so a run never touches the developer's ~/.entity.
	home := t.TempDir()
	t.Setenv("HOME", home)

	ap, ws, err := shellboot.Bootstrap(ctx, shellboot.Config{
		Identity:       name,
		CreateIdentity: true,
		LocalAlias:     name,
		ListenAddr:     "127.0.0.1:0",
		OpenAccess:     true,
		StorageKind:    "sqlite",
		StoragePath:    filepath.Join(home, name+".db"),
	})
	if err != nil {
		t.Fatalf("Bootstrap %s (sqlite): %v", name, err)
	}
	t.Cleanup(func() { _ = ap.Close() })
	if ws.BlobResolve == nil {
		t.Fatalf("%s: shellboot did not wire the blob-resolve handler", name)
	}
	return &syncTestPeer{ap: ap, ws: ws, id: ap.PeerID()}
}

func mkdirOrFail(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
}

func mountOrFail(t *testing.T, p *syncTestPeer, dir, target string) shellcmd.MountOutcome {
	t.Helper()
	out, err := p.ws.Mount(shellcmd.MountRequest{FilesystemDir: dir, TargetPrefix: target})
	if err != nil {
		t.Fatalf("mount %s: %v", dir, err)
	}
	return out
}

func bindingPaths(p *syncTestPeer, prefix string) []string {
	var out []string
	for _, e := range p.ap.RawLocationIndex().List(prefix) {
		out = append(out, e.Path)
	}
	return out
}

// listingNames drives the REAL `ls` verb — through Registry.Dispatch,
// the same route the REPL takes — and returns the names it reported.
//
// Reading p.ap.Store().List here instead would be the fake this repo
// keeps finding (AP58, AP61): it would certify the model while claiming
// to certify the surface, and the podman harness's assertion depends on
// the surface specifically.
//
// WD is seeded canonical, never `/@alias/...` — store-side surfaces
// panic on a leading alias (AGENTS.md).
func listingNames(t *testing.T, p *syncTestPeer, relPrefix string) map[string]bool {
	t.Helper()
	sh := shellcmd.NewShellInWorkspace(p.ws)
	sh.WD = shellcmd.Path("/" + p.id + "/")

	res, err := shellcmd.Default().Dispatch(sh, "ls", []string{relPrefix})
	if err != nil {
		t.Fatalf("`ls %s` on the receiver: %v", relPrefix, err)
	}
	names := map[string]bool{}
	for _, row := range res.Listing {
		names[row.Name] = true
	}
	if len(names) == 0 {
		t.Logf("`ls %s` reported no rows", relPrefix)
	}
	return names
}
