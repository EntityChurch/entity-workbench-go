package shellboot_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/localfiles"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/shellboot"
	"entity-workbench-go/shellcmd"
	"entity-workbench-go/workbench"
)

// AUDIT PROBE — does a mount survive a restart in the form the WRITE
// path needs, not just the form the CHECK path reads?
//
// The operator's report: accept succeeded, reported the right target
// prefix, and then every file failed with
//
//	local/files:write returned status 404
//
// 404 from `local/files:write` is the kernel's `no_root_mapping`
// (ext/localfiles/operations.go handleWrite → findRootMapping). So the
// receiving peer had a mount by every measure the product consults, and
// the kernel handler that has to put bytes on disk could not see it.
//
// There are TWO sources of truth for "is this folder mounted" and they
// are not the same store:
//
//   - `shellcmd.hasLocalRoot`, which Accept/Sync gate on, reads the TREE
//     (`system/config/local/files/{root}` entities). Durable.
//   - `Handler.findRootMapping`, which the write goes through, reads
//     `h.roots` — an IN-MEMORY map, rebuilt at startup by `Handler.Load`.
//
// Every existing test mounts and writes inside ONE process, where
// `AddRoot` populates the in-memory map directly, so `Load` is never on
// the path being exercised. The operator restarted the app. That is the
// difference between the suite and the machine, and it is the same shape
// as AP62/D26: a derived runtime structure over a persistent store, where
// the read path that still works hides the one that does not.
//
// This test asserts on the DERIVED structure across a process boundary,
// which is the only form that can fail — re-reading the tree entity
// passes against the broken build.
func TestMount_SurvivesRestart_ForTheWritePath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// HOME is redirected so the identity Bootstrap creates for a sqlite
	// peer lands in the test's own directory and not the developer's
	// ~/.entity. Go reads HOME at call time, so t.Setenv reaches it.
	home := t.TempDir()
	t.Setenv("HOME", home)
	dbPath := filepath.Join(home, "store.db")
	shareDir := filepath.Join(home, "downloads")
	if err := os.MkdirAll(shareDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Identity deliberately EMPTY: under sqlite, Bootstrap substitutes
	// the default identity and creates it if absent, which is what the
	// shipped app does. A per-run ephemeral keypair would namespace the
	// tree differently on each run and the restart would be measuring
	// the wrong thing.
	cfg := shellboot.Config{
		LocalAlias:  "restarter",
		StorageKind: "sqlite",
		StoragePath: dbPath,
	}

	// --- first run: mount ---------------------------------------------
	ap1, ws1, err := shellboot.Bootstrap(ctx, cfg)
	if err != nil {
		t.Fatalf("bootstrap 1: %v", err)
	}
	out, err := ws1.Mount(shellcmd.MountRequest{
		FilesystemDir: shareDir,
		TargetPrefix:  "archives/downloads/",
	})
	if err != nil {
		_ = ap1.Close()
		t.Fatalf("mount: %v", err)
	}
	root := out.RootName
	if err := ap1.Close(); err != nil {
		t.Fatalf("close 1: %v", err)
	}

	// --- second run: the same store, a fresh process image ------------
	ap2, ws2, err := shellboot.Bootstrap(ctx, cfg)
	if err != nil {
		t.Fatalf("bootstrap 2: %v", err)
	}
	defer func() { _ = ap2.Close() }()

	// The check the PRODUCT makes. Reads the tree; expected to pass, and
	// its passing is exactly what makes the failure below invisible —
	// Accept gets this far and reports success.
	mountedByTree := false
	for _, e := range ap2.Store().List(workbench.MountConfigPrefix) {
		name, under := workbench.RelativeUnder(e.Path, workbench.MountConfigPrefix)
		if under && name == root {
			mountedByTree = true
		}
	}
	if !mountedByTree {
		t.Fatalf("after restart the tree has no mount config for %q — a different "+
			"defect from the one under test, and it would make Accept refuse rather "+
			"than half-succeed", root)
	}

	// What the kernel's rehydrate path actually sees. Handler.Load does
	// `strings.TrimPrefix(entry.Path, "system/config/local/files/")` and
	// skips any remainder containing a "/", so the SHAPE of these paths
	// decides whether any root is restored at all.
	for _, e := range ap2.RawLocationIndex().List("system/config/local/files/") {
		t.Logf("raw index entry: %q", e.Path)
	}

	// The check the KERNEL makes, reached the only way it can be from
	// out here: perform the write the sync performs.
	//
	// A 404 here IS the operator's bug, reproduced with no network, no
	// second peer, and no GUI.
	target := "local/files/" + root + "/probe.txt"
	wroteOK := true
	err = writeThroughLocalFiles(ctx, ap2, target, []byte("audit probe\n"))
	if err != nil {
		wroteOK = false
		// Errorf, not Fatalf: the watcher assertion below is an
		// INDEPENDENT half of the same defect, and stopping here would
		// report one failure where there are two (AP15).
		t.Errorf("REPRODUCED: local/files:write to %q failed after a restart: %v\n"+
			"  the tree says this folder is mounted and the write path cannot see it.\n"+
			"  Accept gates on the tree, so it reports success and then every file 404s —\n"+
			"  which is exactly what the operator saw.", target, err)
	}

	// And the bytes are actually on disk under the mount root, because a
	// write that returns 200 and lands nowhere would satisfy the above.
	if wroteOK {
		landed := filepath.Join(shareDir, "probe.txt")
		got, rerr := os.ReadFile(landed)
		if rerr != nil {
			t.Errorf("write reported success but %s does not exist: %v", landed, rerr)
		} else if string(got) != "audit probe\n" {
			t.Errorf("wrote %q to %s, want the probe body", string(got), landed)
		}
	}

	// --- the OTHER half of the same skipped loop ----------------------
	//
	// `StartWatching` is called inside the loop that silently skips every
	// root, so a restart also leaves the disk→tree direction dead. The
	// operator's second complaint — "I go to Local Files and there are no
	// files" — is this: they add a file to the shared folder and nothing
	// in the tree ever hears about it, so every surface shows an empty
	// mount and the folder they are sharing publishes nothing.
	//
	// Asserted separately from the write above because they fail
	// independently and a fix for one does not imply the other.
	newFile := filepath.Join(shareDir, "after-restart.txt")
	if err := os.WriteFile(newFile, []byte("watcher probe\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	watched := "local/files/" + root + "/after-restart.txt"
	if !awaitTreePath(ap2, watched, 20*time.Second) {
		t.Errorf("after a restart, a file added to the mounted directory never reached "+
			"the tree at %q — the watcher did not restart, so this peer ingests nothing "+
			"and the folder it is sharing looks empty from every surface", watched)
	}

	_ = ws2
}

// awaitTreePath polls for a tree path to appear. Polling and not a
// subscription on purpose: what is under test is whether the WATCHER
// produced anything at all, and a subscription would be a second thing
// that could be broken.
func awaitTreePath(ap *entitysdk.AppPeer, path string, within time.Duration) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if _, ok := ap.Store().Get(path); ok {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// writeThroughLocalFiles performs the same dispatch the sync's
// blob-resolve step performs: bytes mode rather than content mode, since
// the point is the ROOT LOOKUP that precedes both, not blob plumbing.
func writeThroughLocalFiles(ctx context.Context, ap *entitysdk.AppPeer,
	treePath string, body []byte) error {
	req := localfiles.WriteRequestData{Bytes: body, CreateDirs: true}
	raw, err := ecf.Encode(req)
	if err != nil {
		return fmt.Errorf("encode write request: %w", err)
	}
	ent, err := entity.NewEntity(localfiles.TypeWriteRequest, cbor.RawMessage(raw))
	if err != nil {
		return fmt.Errorf("build write request: %w", err)
	}
	resp, err := ap.Executor().ExecuteOnResource("local/files", "write", ent,
		&types.ResourceTarget{Targets: []string{treePath}})
	if err != nil {
		return err
	}
	if resp == nil || resp.Status != 200 {
		status := uint(0)
		if resp != nil {
			status = resp.Status
		}
		return fmt.Errorf("local/files:write returned status %d", status)
	}
	return nil
}
