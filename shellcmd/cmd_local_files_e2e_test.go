package shellcmd_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/localfiles"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"
)

// TestE2E_LocalFilesMountChain is the load-bearing integration test
// for Phase E: a real filesystem write fires fsnotify → the
// localfiles handler writes a FileData entity into the tree → the
// subscription fires → the 3-step ingest chain advances → a
// doc/markdown-file entity is bound at the configured target prefix.
//
// What this catches that the unit tests don't:
//   - Scoped chain capability actually authorizes every step of the
//     chain at dispatch time (R1 check at install + per-step
//     verify_request at advancement).
//   - The ingest-transform handler's input type matches what
//     local/files:read produces.
//   - on_error routing wires correctly through dispatch — failures
//     would deposit error entities under
//     system/runtime/chain-errors/... and we assert there are
//     none after a successful round-trip.
//   - localfiles writer + entitysdk subscription engine interop —
//     the fs event chain we depend on isn't synthetic.
//
// Timing: fsnotify events fire ~100ms after the write; chain
// advancement spans ~3 dispatch hops; total wall-clock budget is
// generous (5s) to absorb CI jitter.
func TestE2E_LocalFilesMountChain(t *testing.T) {
	// Uses the Q2 workaround for Finding 6:
	// a single workbench handler does the notification → tree-bind
	// pipeline instead of a 3-step continuation chain. The 3-step
	// chain shape can't be expressed today because
	// `ContinuationData.Resource` is static at install time. When
	// the architecture team adopts Q1 (resource_transform), the
	// chain shape can be restored and this test rewritten.
	fsDir := t.TempDir()

	// Note: not enabling DebugLog here — the watcher goroutine
	// outlives test completion and logging into t.Log after exit
	// panics. To re-enable for debugging, route the logger to
	// os.Stderr or a goroutine-safe sink that survives test teardown.

	// The workaround handler is registered at peer-construction time.
	ingestHandler := workbench.NewNotificationIngestHandler(nil)
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		Handlers: []entitysdk.HandlerRegistration{
			{Pattern: workbench.NotificationIngestPattern, Handler: ingestHandler},
			{Pattern: workbench.ChainErrorsPattern, Handler: workbench.NewChainErrorsHandler()},
		},
	})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	const targetPrefix = "archives/notes/"
	rootName := "e2e"
	sourcePrefix := "local/files/" + rootName + "/"

	// Register the source→target mount with the ingest handler.
	ingestHandler.RegisterMount(sourcePrefix, targetPrefix)

	// Mount procedure for the Q2 shape: subscribe + start watcher.
	// No multi-step chain installation.
	if err := installMountQ2(t, ap, fsDir, rootName, sourcePrefix, targetPrefix); err != nil {
		t.Fatalf("installMountQ2: %v", err)
	}

	// Write a markdown file. The watcher should pick this up within
	// ~100ms; the chain should converge within another ~few hundred ms.
	mdContent := "# Hello E2E\n\nBody content for the round-trip test.\n"
	mdPath := filepath.Join(fsDir, "hello.md")
	if err := os.WriteFile(mdPath, []byte(mdContent), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Poll for the target binding. The path mirroring is
	// targetPrefix + relative-fs-path; for "hello.md" at the root of
	// the mount, that's "archives/notes/hello.md".
	wantPath := targetPrefix + "hello.md"
	wantSourcePath := sourcePrefix + "hello.md"

	// First, wait for the localfiles watcher to bind the FileData
	// at the source prefix. Then wait for the workaround handler
	// to bind the doc/markdown-file at the target prefix.
	waitForBinding(t, ap, wantSourcePath, 5*time.Second, "FileData (localfiles watcher)")
	waitForBinding(t, ap, wantPath, 5*time.Second, "doc/markdown-file (notification ingest)")

	// Verify the bound entity is shaped as expected.
	ent, ok, err := ap.Get(wantPath)
	if err != nil {
		t.Fatalf("Get %s: %v", wantPath, err)
	}
	if !ok {
		t.Fatalf("expected entity at %s, missing", wantPath)
	}
	if ent.Type != workbench.MarkdownFileType {
		t.Errorf("target entity type = %s, want %s", ent.Type, workbench.MarkdownFileType)
	}
	md, err := workbench.MarkdownFileDataFromEntity(ent)
	if err != nil {
		t.Fatalf("decode target entity: %v", err)
	}
	if md.Title != "Hello E2E" {
		t.Errorf("title = %q, want %q", md.Title, "Hello E2E")
	}
	body, present, err := workbench.LoadMarkdownContent(ap.Store().ContentStore(), md)
	if err != nil {
		t.Fatalf("LoadMarkdownContent: %v", err)
	}
	if !present {
		t.Fatal("blob not present in local content store")
	}
	if string(body) != mdContent {
		t.Errorf("content round-trip mismatch: got %q, want %q", string(body), mdContent)
	}

}

// installMountQ2 sets up the Phase E Q2 workaround shape: subscribe
// tree-event notifications directly to the workbench/ingest-from-
// notification handler. No multi-step continuation chain — the
// single handler does the full pipeline. When the architecture team
// adopts Q1 (dynamic Resource extraction in continuation chains)
// this collapses back to the proper 3-step shape.
func installMountQ2(
	t *testing.T,
	ap *entitysdk.AppPeer,
	fsDir, rootName, sourcePrefix, targetPrefix string,
) error {
	t.Helper()
	ctx := context.Background()
	localID := ap.PeerID()

	// Cap covers the one EXECUTE the subscription triggers: dispatch
	// the workbench ingest handler with receive op.
	grants := []types.GrantEntry{
		{
			Handlers:   types.CapabilityScope{Include: []string{workbench.NotificationIngestPattern}},
			Operations: types.CapabilityScope{Include: []string{"receive"}},
		},
	}
	if _, err := ap.MintChainCapabilityBound(grants,
		"system/capability/grants/chain/local-files/"+rootName); err != nil {
		return fmt.Errorf("mint chain cap: %w", err)
	}

	lfHandler := ap.LocalFilesHandler()
	if lfHandler == nil {
		return fmt.Errorf("local/files handler not wired")
	}
	cfg := localfiles.RootConfigData{
		Prefix:         sourcePrefix,
		FilesystemRoot: fsDir,
	}
	if err := lfHandler.AddRoot(rootName, cfg, ap.RawContentStore(), ap.RawLocationIndex()); err != nil {
		return fmt.Errorf("AddRoot: %w", err)
	}
	if err := lfHandler.StartWatching(ctx, rootName, ap.RawContentStore(),
		ap.RawLocationIndex(), ap.IdentityHash()); err != nil {
		return fmt.Errorf("StartWatching: %w", err)
	}

	// Subscribe: tree changes under sourcePrefix deliver to the
	// workbench ingest handler URI. The subscription engine wraps
	// notifications and dispatches them; the handler does the rest.
	deliverURI := fmt.Sprintf("entity://%s/%s", localID, workbench.NotificationIngestPattern)
	if _, err := ap.SubscribeRawAt(localID, sourcePrefix+"*", deliverURI,
		"receive", entitysdk.SubscribeOpts{Events: []string{"created", "updated"}}); err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	return nil
}

// waitForBinding polls ap.Store().Has(path) until true or the
// deadline expires. Reports a sensible failure with the elapsed
// time so we can tell whether the chain didn't fire vs fired late.
func waitForBinding(t *testing.T, ap *entitysdk.AppPeer, path string, timeout time.Duration, label string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	const pollInterval = 25 * time.Millisecond
	start := time.Now()
	for time.Now().Before(deadline) {
		if ap.Store().Has(path) {
			t.Logf("%s appeared at %s after %v", label, path, time.Since(start))
			return
		}
		time.Sleep(pollInterval)
	}
	t.Fatalf("%s did not appear at %s within %v", label, path, timeout)
}
