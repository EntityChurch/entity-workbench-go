package entitysdk_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/crypto"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

// TestStorage_Sqlite_LargeCorpusSurvivesRestart is the workload-shape
// persistence test: generate a ~500-file synthetic corpus, ingest it
// into a SQLite-backed peer, commit a revision, close, reopen, and
// verify every entity's content hash matches what we recorded — i.e.
// the whole corpus round-trips byte-equal through SQLite.
//
// Why synthetic and not the workbench's own docs directory: that's
// what TestE2E_IngestTree exercises (~24 files, real-shaped folders,
// good for "ingest mechanics work end-to-end"). This test wants
// scale to catch behaviors that only emerge with hundreds-to-thousands
// of entities: SQL query plan changes, index scans, write-amplification
// patterns on commit-time root recomputation. Synthetic also keeps CI
// deterministic — no dependency on parent-directory shape.
//
// Coverage:
//
//   - 500 entities × 4 metadata fields × ~2KB content each ≈ 1MB of
//     payload through the SQL content store.
//   - Per-path content-hash byte-equality across close+reopen.
//   - Revision log walks the single pass-1 commit on reopen.
//   - `List(prefix)` returns the right cardinality at sub-tree level.
//   - A fresh write + commit on the reopened peer pushes the log to
//     2 versions, proving the auto-version + root-tracker state
//     rehydrates cleanly post-reopen.
//   - Content survives intact for a sampled set of paths (grep-shape
//     check — paths contain a known marker string we wrote at ingest
//     time).
func TestStorage_Sqlite_LargeCorpusSurvivesRestart(t *testing.T) {
	srcDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "peer.db")

	kp, err := crypto.Generate()
	if err != nil {
		t.Fatalf("crypto.Generate: %v", err)
	}

	// Synthesize a deterministic corpus on disk so the test is
	// hermetic. 10 subdirectories × 50 files each = 500 entities.
	// File names are zero-padded so iteration order is stable; we
	// rely on that for the sample probes.
	type fileSpec struct {
		treePath string // workspace/dir03/file017.md
		content  string
	}
	const (
		numDirs       = 10
		filesPerDir   = 50
		treePrefix    = "workspace/"
		markerPattern = "MARKER-d%02d-f%03d" // grep-checked downstream
	)
	specs := make([]fileSpec, 0, numDirs*filesPerDir)

	for d := 0; d < numDirs; d++ {
		dirName := fmt.Sprintf("dir%02d", d)
		if err := os.MkdirAll(filepath.Join(srcDir, dirName), 0700); err != nil {
			t.Fatalf("mkdir %s: %v", dirName, err)
		}
		for f := 0; f < filesPerDir; f++ {
			marker := fmt.Sprintf(markerPattern, d, f)
			content := strings.Repeat("Lorem ipsum dolor sit amet. ", 60) // ≈1.7 KB
			content += "\n\n" + marker + "\n"
			fname := fmt.Sprintf("file%03d.md", f)
			diskPath := filepath.Join(srcDir, dirName, fname)
			if err := os.WriteFile(diskPath, []byte(content), 0600); err != nil {
				t.Fatalf("write %s: %v", diskPath, err)
			}
			specs = append(specs, fileSpec{
				treePath: treePrefix + dirName + "/" + fname,
				content:  content,
			})
		}
	}
	t.Logf("synthesized corpus: %d files across %d dirs (~%d KB total payload)",
		len(specs), numDirs, len(specs)*len(specs[0].content)/1024)

	wantHashes := make(map[string]string, len(specs))
	var firstCommitVersion string

	// --- Pass 1: bulk ingest + commit ----------------------------------
	func() {
		ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
			Keypair: &kp,
			Storage: entitysdk.StorageConfig{Kind: "sqlite", Path: dbPath},
		})
		if err != nil {
			t.Fatalf("pass1 CreatePeer: %v", err)
		}
		defer ap.Close()

		for _, s := range specs {
			data := map[string]any{
				"path":    strings.TrimPrefix(s.treePath, treePrefix),
				"title":   filepath.Base(s.treePath),
				"content": s.content,
				"size":    int64(len(s.content)),
			}
			h, err := ap.Put(s.treePath, "doc/markdown-file", data)
			if err != nil {
				t.Fatalf("pass1 Put %s: %v", s.treePath, err)
			}
			wantHashes[s.treePath] = h.String()
		}
		if got, want := ap.PathCount(), len(specs); got < want {
			t.Errorf("pass1 PathCount = %d, want ≥ %d", got, want)
		}

		// Commit the corpus as a revision snapshot.
		ctx := context.Background()
		commit, err := ap.Revision().Commit(ctx, treePrefix, "ingest pass 1")
		if err != nil {
			t.Fatalf("pass1 Commit: %v", err)
		}
		if commit.Version.IsZero() {
			t.Fatal("pass1 commit returned zero version")
		}
		firstCommitVersion = commit.Version.String()
	}()

	// Sanity: the SQLite file actually got written substantially.
	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat DB: %v", err)
	}
	if info.Size() < int64(100*1024) {
		t.Errorf("DB file is only %d bytes after ingesting %d entities; expected substantially larger",
			info.Size(), len(specs))
	}
	t.Logf("DB size after pass 1: %d KB", info.Size()/1024)

	// --- Pass 2: reopen + verify entire corpus + revision log ---------
	ap := openSqlitePeer(t, dbPath, kp)
	defer ap.Close()

	// Byte-equality check: every path returns the same content hash.
	// We don't compare every entity's full payload (too slow), just
	// the hash — which is over the canonical CBOR encoding of the
	// payload, so equal hashes ⇒ equal payloads.
	missing := 0
	mismatched := 0
	for _, s := range specs {
		ent, ok, err := ap.Get(s.treePath)
		if err != nil {
			t.Fatalf("reopen Get %s: %v", s.treePath, err)
		}
		if !ok {
			missing++
			continue
		}
		if ent.ContentHash.String() != wantHashes[s.treePath] {
			mismatched++
		}
	}
	if missing > 0 {
		t.Errorf("reopen: %d/%d entities missing after restart", missing, len(specs))
	}
	if mismatched > 0 {
		t.Errorf("reopen: %d/%d entities have mismatched content hashes after restart",
			mismatched, len(specs))
	}

	// Subtree listing: pick a directory at random-but-deterministic
	// and verify it has exactly filesPerDir children.
	probeDir := treePrefix + "dir05/"
	entries, err := ap.List(probeDir)
	if err != nil {
		t.Fatalf("reopen List %s: %v", probeDir, err)
	}
	if len(entries) != filesPerDir {
		t.Errorf("reopen: %s has %d entries, want %d", probeDir, len(entries), filesPerDir)
	}

	// Content-survival check (grep-shape): for a sampled set of
	// paths, verify the marker string is still in the entity's
	// content field. Catches scenarios where the entity hash matches
	// but the content somehow truncated or got re-encoded on storage.
	for _, sampleIdx := range []int{0, 50, 123, 256, 333, 449} {
		s := specs[sampleIdx]
		ent, ok, err := ap.Get(s.treePath)
		if err != nil || !ok {
			t.Errorf("reopen sample-probe Get %s failed (ok=%v, err=%v)", s.treePath, ok, err)
			continue
		}
		// The content field is the raw CBOR payload of the entity.
		// Easiest hermetic check: decode the entity's data map and
		// look for the marker. The MarkdownFileType in workbench
		// uses a flat map shape so the marker is reachable via the
		// "content" key.
		dataStr := string(ent.Data)
		expectMarker := fmt.Sprintf("MARKER-d%02d-f%03d", sampleIdx/filesPerDir, sampleIdx%filesPerDir)
		if !strings.Contains(dataStr, expectMarker) {
			t.Errorf("reopen sample %s: expected marker %q not in entity payload",
				s.treePath, expectMarker)
		}
	}

	// Revision log: should walk exactly one version (the pass-1
	// commit), unchanged across the restart.
	ctx := context.Background()
	rc := ap.Revision()
	logResult, err := rc.Log(ctx, types.RevisionLogParamsData{Prefix: treePrefix})
	if err != nil {
		t.Fatalf("reopen Log: %v", err)
	}
	if len(logResult.Versions) != 1 {
		t.Errorf("reopen Log returned %d versions, want 1", len(logResult.Versions))
	}
	if len(logResult.Versions) > 0 && logResult.Versions[0].String() != firstCommitVersion {
		t.Errorf("reopen log[0] = %s, want pass-1 commit %s",
			logResult.Versions[0], firstCommitVersion)
	}

	// Fresh write + commit on the reopened peer. The auto-version +
	// root-tracker state must rehydrate correctly from the persisted
	// tree — otherwise the second commit either fails or produces
	// the same hash as the first.
	newPath := treePrefix + "dir05/file-after-restart.md"
	if _, err := ap.Put(newPath, "doc/markdown-file", map[string]any{
		"path":    "dir05/file-after-restart.md",
		"title":   "after restart",
		"content": "fresh write after reopen",
		"size":    int64(24),
	}); err != nil {
		t.Fatalf("reopen fresh Put: %v", err)
	}
	secondCommit, err := rc.Commit(ctx, treePrefix, "post-restart commit")
	if err != nil {
		t.Fatalf("reopen Commit: %v", err)
	}
	if secondCommit.Version.IsZero() {
		t.Fatal("reopen Commit returned zero version")
	}
	if secondCommit.Version.String() == firstCommitVersion {
		t.Fatal("reopen Commit returned same hash as pass-1 commit — corpus snapshot didn't shift")
	}

	// The log should now show 2 versions, second-newest-first.
	logResult2, err := rc.Log(ctx, types.RevisionLogParamsData{Prefix: treePrefix})
	if err != nil {
		t.Fatalf("reopen Log (after second commit): %v", err)
	}
	if len(logResult2.Versions) != 2 {
		t.Errorf("after second commit: log has %d versions, want 2", len(logResult2.Versions))
	}
	if len(logResult2.Versions) >= 2 {
		if got := logResult2.Versions[0].String(); got != secondCommit.Version.String() {
			t.Errorf("log[0] = %s, want second commit %s", got, secondCommit.Version)
		}
		if got := logResult2.Versions[1].String(); got != firstCommitVersion {
			t.Errorf("log[1] = %s, want first commit %s", got, firstCommitVersion)
		}
	}
}
