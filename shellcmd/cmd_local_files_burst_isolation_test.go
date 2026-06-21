package shellcmd_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"
	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/localfiles"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"
)

// TestE2E_BurstIsolation_LocalIngest verifies whether the local
// Phase E ingest pipeline handles a burst of 5 file writes on a
// SINGLE peer (no cross-peer activity, no follow chain, no merges).
//
// If this passes: ingest is fine, the burst data loss in
// TestE2E_Bidirectional_BurstWrites is genuinely from the
// cross-peer merge cycle (Finding 10 as described).
//
// If this FAILS: the data loss is local — fsnotify coalescing or
// watcher debounce dropping events. Finding 10 would need rewriting
// because the failure mode is upstream of the merge.
func TestE2E_BurstIsolation_LocalIngest(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	fsDir := t.TempDir()
	const targetPrefix = "archives/notes/"
	const burst = 5
	const rootName = "burst-iso"
	sourcePrefix := "local/files/" + rootName + "/"

	ingest := workbench.NewNotificationIngestHandler(nil)
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
		Handlers: []entitysdk.HandlerRegistration{
			{Pattern: workbench.NotificationIngestPattern, Handler: ingest},
			{Pattern: workbench.ChainErrorsPattern, Handler: workbench.NewChainErrorsHandler()},
		},
	})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	// Install mount (single-peer, no follow).
	grants := []types.GrantEntry{{
		Handlers:   types.CapabilityScope{Include: []string{workbench.NotificationIngestPattern}},
		Operations: types.CapabilityScope{Include: []string{"receive"}},
	}}
	if _, err := ap.MintChainCapabilityBound(grants,
		"system/capability/grants/chain/local-files/"+rootName); err != nil {
		t.Fatalf("mint cap: %v", err)
	}
	ingest.RegisterMount(sourcePrefix, targetPrefix)
	lf := ap.LocalFilesHandler()
	if err := lf.AddRoot(rootName, localfiles.RootConfigData{
		Prefix:         sourcePrefix,
		FilesystemRoot: fsDir,
	}, ap.RawContentStore(), ap.RawLocationIndex()); err != nil {
		t.Fatalf("AddRoot: %v", err)
	}
	if err := lf.StartWatching(ctx, rootName, ap.RawContentStore(),
		ap.RawLocationIndex(), ap.IdentityHash()); err != nil {
		t.Fatalf("StartWatching: %v", err)
	}
	deliverURI := fmt.Sprintf("entity://%s/%s", ap.PeerID(), workbench.NotificationIngestPattern)
	if _, err := ap.SubscribeRawAt(ap.PeerID(), sourcePrefix+"*", deliverURI, "receive",
		entitysdk.SubscribeOpts{Events: []string{"created", "updated"}}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	// Auto-version on the target prefix.
	autoTrue := true
	if _, err := ap.Revision().ConfigPut(ctx, "notes", types.RevisionConfigData{
		Prefix:      targetPrefix,
		AutoVersion: &autoTrue,
	}, nil); err != nil {
		t.Fatalf("auto-version config: %v", err)
	}

	// Burst writes. Same shape as the bidi test but no cross-peer
	// pressure.
	for i := 0; i < burst; i++ {
		if err := os.WriteFile(filepath.Join(fsDir, fmt.Sprintf("a-%d.md", i)),
			[]byte(fmt.Sprintf("# a %d\n", i)), 0600); err != nil {
			t.Fatalf("write a-%d.md: %v", i, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for all 5 to land at archives/notes/.
	deadline := time.Now().Add(20 * time.Second)
	allLanded := func() bool {
		for i := 0; i < burst; i++ {
			if !ap.Store().Has(fmt.Sprintf("%sa-%d.md", targetPrefix, i)) {
				return false
			}
		}
		return true
	}
	for time.Now().Before(deadline) {
		if allLanded() {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !allLanded() {
		t.Logf("--- local ingest burst diagnostics (files missing) ---")
		for i := 0; i < burst; i++ {
			srcPath := fmt.Sprintf("%sa-%d.md", sourcePrefix, i)
			tgtPath := fmt.Sprintf("%sa-%d.md", targetPrefix, i)
			t.Logf("  a-%d: source(watcher)=%v target(ingest)=%v",
				i, ap.Store().Has(srcPath), ap.Store().Has(tgtPath))
		}
		t.Fatalf("local ingest dropped %d-burst writes on single peer", burst)
	}

	// All 5 files in tree. Now check the REVISION LOG — did
	// auto-version commit one revision per file, or batch them?
	// Also confirm the latest commit's state reflects all 5 files.
	time.Sleep(2 * time.Second) // settle for the auto-versioner to finish
	logResult, err := ap.Revision().Log(ctx, types.RevisionLogParamsData{Prefix: targetPrefix})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	t.Logf("local ingest handled all %d files; revision log has %d commits",
		burst, len(logResult.Versions))
	// If auto-version captured all 5 writes via N commits where each
	// commit includes at least one new file, the cumulative state at
	// the latest commit should have all 5 files. We can't easily walk
	// historical commits without the diff API; the present-tree
	// check is enough to confirm "single-peer burst → all files
	// land + at least one commit fires".
	if len(logResult.Versions) == 0 {
		t.Errorf("auto-versioner produced zero commits despite %d tree writes", burst)
	}
}
