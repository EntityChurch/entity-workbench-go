package shellcmd_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/ext/localfiles"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"
)

// TestMountSweep_RemovesStaleBindings is the operator-tool path:
// after some files have been mounted, the test simulates "offline
// drift" by removing files from disk WITHOUT the watcher noticing
// (we suspend the watcher's perspective by sleeping past its
// debounce window and then re-querying). SweepMount removes the
// stale tree bindings.
//
// Real-world trigger: peer was off when files were deleted. On
// next boot the tree still has those bindings; operator runs
// `mount sweep <root>` to reconcile.
//
// For the test we don't actually go offline — we just sweep
// without telling the watcher. The watcher will eventually fire
// a Remove event for the missing files (and our sweep is
// idempotent against that), but the sweep runs synchronously
// and the assertions can fire before the watcher catches up.
func TestMountSweep_RemovesStaleBindings(t *testing.T) {
	const rootName = "swept"
	const sourcePrefix = "local/files/" + rootName + "/"
	const targetPrefix = "archives/notes/"

	fsDir := t.TempDir()

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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ingest.RegisterMount(sourcePrefix, targetPrefix)
	lf := ap.LocalFilesHandler()
	if err := lf.AddRoot(rootName, localfiles.RootConfigData{
		Prefix:         sourcePrefix,
		FilesystemRoot: fsDir,
	}, ap.RawContentStore(), ap.RawLocationIndex()); err != nil {
		t.Fatalf("AddRoot: %v", err)
	}
	if err := lf.StartWatching(ctx, rootName, ap.RawContentStore(), ap.RawLocationIndex(), ap.IdentityHash()); err != nil {
		t.Fatalf("StartWatching: %v", err)
	}
	// Seed three files; wait for the watcher to bind them.
	for _, name := range []string{"a.md", "b.md", "c.md"} {
		if err := os.WriteFile(filepath.Join(fsDir, name), []byte("# "+name+"\n\nbody\n"), 0600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if !waitFor(10*time.Second, func() bool {
		return ap.Store().Has(sourcePrefix+"a.md") &&
			ap.Store().Has(sourcePrefix+"b.md") &&
			ap.Store().Has(sourcePrefix+"c.md")
	}) {
		t.Fatal("watcher did not bind seed files")
	}

	// Sanity: target prefix won't be populated because we haven't
	// wired a subscription for notification-ingest in this test —
	// SweepMount should still handle the (empty) target side without
	// error.

	// Simulate offline drift: remove b.md from disk + then stop the
	// fs watcher's chance to react before sweep runs. We do this by
	// removing the file and immediately calling sweep — the
	// watcher's debounce window means it may not have fired yet.
	// (In reality the watcher would catch this within the debounce
	// window anyway; the sweep is the safety net for the case
	// where the peer was offline.)
	if err := os.Remove(filepath.Join(fsDir, "b.md")); err != nil {
		t.Fatalf("remove b.md: %v", err)
	}

	res, err := workbench.SweepMount(ap, ingest, rootName)
	if err != nil {
		t.Fatalf("SweepMount: %v", err)
	}

	// One of two outcomes is acceptable:
	//   (a) The watcher already caught the Remove before sweep ran,
	//       in which case sweep sees b.md gone from both fs and tree
	//       → no removal needed; SourcePresent should be 2 not 3.
	//   (b) Sweep beat the watcher, in which case it removed b.md
	//       directly → SourceRemoved contains "b.md".
	// Either way: after sweep, b.md must be gone from the tree
	// AND a.md / c.md must remain.
	if ap.Store().Has(sourcePrefix + "b.md") {
		t.Errorf("after sweep, b.md still bound at %s", sourcePrefix+"b.md")
	}
	if !ap.Store().Has(sourcePrefix + "a.md") {
		t.Errorf("after sweep, a.md missing (sweep removed too much)")
	}
	if !ap.Store().Has(sourcePrefix + "c.md") {
		t.Errorf("after sweep, c.md missing (sweep removed too much)")
	}

	t.Logf("sweep result: fs=%d tree=%d removed=%v",
		res.FilesystemFiles, res.SourcePresent, res.SourceRemoved)
}

// TestMountSweep_AddMode covers the inverse: files exist on disk
// with no tree binding. -add re-ingests them. Simulates "watcher
// missed the create" or "operator wants to baseline from disk."
func TestMountSweep_AddMode(t *testing.T) {
	const rootName = "addsweep"
	const sourcePrefix = "local/files/" + rootName + "/"
	const targetPrefix = "archives/notes/"

	fsDir := t.TempDir()
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

	ingest.RegisterMount(sourcePrefix, targetPrefix)
	lf := ap.LocalFilesHandler()
	if err := lf.AddRoot(rootName, localfiles.RootConfigData{
		Prefix:         sourcePrefix,
		FilesystemRoot: fsDir,
	}, ap.RawContentStore(), ap.RawLocationIndex()); err != nil {
		t.Fatalf("AddRoot: %v", err)
	}
	// Intentionally do NOT call StartWatching — we want files on disk
	// with no tree bindings, which is what -add mode reconciles.

	for _, name := range []string{"x.md", "y.md"} {
		if err := os.WriteFile(filepath.Join(fsDir, name), []byte("# "+name+"\n\nbody\n"), 0600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	// Pre-sweep: nothing in the tree.
	if ap.Store().Has(sourcePrefix + "x.md") {
		t.Fatal("pre-sweep: x.md should not be bound (no watcher running)")
	}

	added, errs, err := workbench.IngestMissingFiles(ap, rootName)
	if err != nil {
		t.Fatalf("IngestMissingFiles: %v", err)
	}
	if len(errs) > 0 {
		t.Fatalf("IngestMissingFiles errors: %v", errs)
	}
	if added != 2 {
		t.Errorf("added = %d, want 2", added)
	}

	// Post-sweep: both files are bound.
	for _, name := range []string{"x.md", "y.md"} {
		if !ap.Store().Has(sourcePrefix + name) {
			t.Errorf("after add-sweep, %s%s not bound", sourcePrefix, name)
		}
		ent, ok := ap.Store().Get(sourcePrefix + name)
		if !ok {
			continue
		}
		if ent.Type != localfiles.TypeFile {
			t.Errorf("%s type = %s, want %s", name, ent.Type, localfiles.TypeFile)
		}
		file, err := localfiles.FileDataFromEntity(ent)
		if err != nil {
			t.Errorf("decode %s: %v", name, err)
			continue
		}
		if file.Path != name {
			t.Errorf("file.Path = %q, want %q", file.Path, name)
		}
		if file.Size == 0 {
			t.Errorf("file.Size = 0")
		}
	}
}
