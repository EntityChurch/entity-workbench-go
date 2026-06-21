package shellcmd_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/entity"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"
)

// TestSubstrateCostProbe_SingleFileIngest measures the per-file
// substrate cost in the v1.2+ shape: how many entities does one
// markdown-file ingest produce across the relevant prefixes? The
// breakdown gives concrete numbers for the v1.3 consumer-integration
// memo.
//
// We deliberately measure a small markdown file (<64 KiB) so the
// blob lands one chunk. Larger files would shift the chunk count
// per the FastCDC target_size (~4 MiB chunks); the cost shape per
// file is `1 file + 1 blob + N chunks + 1 doc/markdown-file`.
//
// This is a documentation test — the assertions are loose bounds
// (catch regressions if the substrate inflates dramatically) but
// the t.Logf breakdown is the load-bearing output. Run with -v to
// see the numbers.
func TestSubstrateCostProbe_SingleFileIngest(t *testing.T) {
	fsDir := t.TempDir()

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
	rootName := "probe"
	sourcePrefix := "local/files/" + rootName + "/"

	ingestHandler.RegisterMount(sourcePrefix, targetPrefix)

	// Capture pre-mount baseline so we can attribute substrate cost
	// vs. mount overhead. Mount overhead includes:
	//   - subscription entity
	//   - root-config entity
	//   - watcher-config entity (after StartWatching)
	//   - chain capability entity
	baseline := ap.EntityCount()
	t.Logf("baseline EntityCount (pre-mount):     %d", baseline)

	if err := installMountQ2(t, ap, fsDir, rootName, sourcePrefix, targetPrefix); err != nil {
		t.Fatalf("installMountQ2: %v", err)
	}

	afterMount := ap.EntityCount()
	t.Logf("after-mount EntityCount:              %d  (Δ from baseline: +%d)", afterMount, afterMount-baseline)

	mdContent := "# Probe File\n\nSubstrate cost measurement body.\n"
	mdPath := filepath.Join(fsDir, "probe.md")
	if err := os.WriteFile(mdPath, []byte(mdContent), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	wantSourcePath := sourcePrefix + "probe.md"
	wantTargetPath := targetPrefix + "probe.md"
	waitForBinding(t, ap, wantSourcePath, 5*time.Second, "FileData (localfiles watcher)")
	waitForBinding(t, ap, wantTargetPath, 5*time.Second, "doc/markdown-file (notification ingest)")

	// Settle: let any straggling chunk persistence complete. The
	// FastCDC chunker writes chunks before the blob, and the blob
	// before the file entity, so reaching wantSourcePath implies
	// the substrate is fully landed. Brief sleep to let any
	// subscription-driven dedup work finish.
	time.Sleep(100 * time.Millisecond)

	afterIngest := ap.EntityCount()
	t.Logf("after-ingest EntityCount:             %d  (Δ from mount: +%d)", afterIngest, afterIngest-afterMount)

	// Path-keyed counts under known prefixes. These show distribution
	// of where the new entities landed.
	fileEntities := listPrefix(ap, sourcePrefix)
	targetEntities := listPrefix(ap, targetPrefix)

	t.Logf("local/files/<root>/   path-bound entities: %d (%s)", len(fileEntities), short(fileEntities))
	t.Logf("archives/notes/       path-bound entities: %d (%s)", len(targetEntities), short(targetEntities))

	// Content-store-only entities (blobs + chunks) aren't necessarily
	// path-bound; iterate the location index to see which content
	// hashes were bound at content paths, and check the content store
	// directly for the rest.
	pc := ap.PeerContext()
	cs := pc.Store()

	allListings := cs.List("")
	contentBlobBound := 0
	contentChunkBound := 0
	for _, e := range allListings {
		bare := stripPeer(e.Path)
		switch {
		case strings.HasPrefix(bare, "system/content/blob/"):
			contentBlobBound++
		case strings.HasPrefix(bare, "system/content/chunk/"):
			contentChunkBound++
		}
	}
	t.Logf("system/content/blob/  path-bound entities: %d", contentBlobBound)
	t.Logf("system/content/chunk/ path-bound entities: %d", contentChunkBound)

	// The content store may also hold unbound entities (e.g. when the
	// content extension stores blobs/chunks under hash addressing only).
	// Sample by type from the path-bound set so we can see which
	// entity types appeared.
	typeCounts := map[string]int{}
	for _, e := range allListings {
		ent, ok, err := ap.Get(stripPeer(e.Path))
		if err != nil || !ok {
			continue
		}
		typeCounts[ent.Type]++
	}
	t.Logf("path-bound entity type breakdown:")
	logTypeBreakdown(t, typeCounts)

	// Verify the ingest worked end-to-end (round-trip + 503 partial-sync
	// case is covered separately in workbench/ingest_transform_test.go).
	tgt, ok, err := ap.Get(wantTargetPath)
	if err != nil || !ok {
		t.Fatalf("target binding missing: ok=%v err=%v", ok, err)
	}
	if tgt.Type != workbench.MarkdownFileType {
		t.Errorf("target Type = %s, want %s", tgt.Type, workbench.MarkdownFileType)
	}

	// Loose-bound regression catch: an order-of-magnitude jump in
	// per-file substrate cost is worth surfacing. For a single small
	// markdown file we expect on the order of single-digit entities
	// added by the ingest itself; the mount setup is separate.
	perIngest := afterIngest - afterMount
	if perIngest > 50 {
		t.Errorf("per-file ingest produced %d entities — substantial regression vs the expected order-of-single-digits", perIngest)
	}
}

// short renders a small list of paths abbreviated for log output.
func short(paths []string) string {
	if len(paths) <= 3 {
		return strings.Join(paths, ", ")
	}
	return strings.Join(paths[:3], ", ") + ", ..."
}

func logTypeBreakdown(t *testing.T, counts map[string]int) {
	t.Helper()
	type kv struct {
		Type  string
		Count int
	}
	rows := make([]kv, 0, len(counts))
	for k, v := range counts {
		rows = append(rows, kv{k, v})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		return rows[i].Type < rows[j].Type
	})
	for _, r := range rows {
		t.Logf("  %4d  %s", r.Count, r.Type)
	}
}

// Used by entity counting via PeerContext().Store().
var _ entity.Entity
