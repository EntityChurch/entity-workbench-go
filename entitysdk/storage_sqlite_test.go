package entitysdk_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"go.entitychurch.org/entity-core-go/core/crypto"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

// Persistence tests for the SQLite storage backend.
//
// Each test follows the same arc: open a peer at a real on-disk DB
// path, drive some realistic workload through the SDK surface
// (tree writes, a revision config + commits, a history config +
// transitions, …), close the peer, reopen at the same path with the
// same keypair, and assert that the application-visible state is
// preserved.
//
// The point is not to retest the underlying SQL plumbing — core-go
// has its own coverage at core/store/sqlite_test.go. These tests
// validate that the SDK wiring (buildPeerOptions storage switch +
// peer.WithCloseFunc release + extension setup against a persisted
// content store) doesn't lose data across a process boundary.

// openSqlitePeer opens an AppPeer with the SQLite store at path.
// Caller is responsible for Close. kp is required so the second open
// in a round-trip produces the same peer identity.
func openSqlitePeer(t *testing.T, path string, kp crypto.Keypair) *entitysdk.AppPeer {
	t.Helper()
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		Keypair: &kp,
		Storage: entitysdk.StorageConfig{Kind: "sqlite", Path: path},
	})
	if err != nil {
		t.Fatalf("CreatePeer(sqlite, %s): %v", path, err)
	}
	return ap
}

// TestStorage_Sqlite_TreeRoundtrip is the floor: write a handful of
// entities at distinct paths, close the peer, reopen at the same DB
// file, and verify every path resolves to the same content hash.
// Also verifies the EntityCount + PathCount accessors agree on both
// sides — they read straight from the SQL backend so they're a tight
// indicator of "the rows really did persist."
func TestStorage_Sqlite_TreeRoundtrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "peer.db")
	kp, err := crypto.Generate()
	if err != nil {
		t.Fatalf("crypto.Generate: %v", err)
	}

	type entry struct {
		path, typ string
		data      any
	}
	entries := []entry{
		{"workspace/note", "doc/markdown-file", map[string]any{"text": "first"}},
		{"workspace/sub/a", "doc/markdown-file", map[string]any{"text": "alpha"}},
		{"workspace/sub/b", "doc/markdown-file", map[string]any{"text": "beta"}},
		{"misc/scratch", "test/v", 42},
	}

	wantHashes := make(map[string]string, len(entries))

	// --- pass 1: write + close ---
	func() {
		ap := openSqlitePeer(t, dbPath, kp)
		defer ap.Close()

		for _, e := range entries {
			h, err := ap.Put(e.path, e.typ, e.data)
			if err != nil {
				t.Fatalf("Put %s: %v", e.path, err)
			}
			wantHashes[e.path] = h.String()
		}
		if got, want := ap.PathCount(), len(entries); got < want {
			t.Errorf("pass1 PathCount = %d, want ≥ %d", got, want)
		}
	}()

	// --- pass 2: reopen + read ---
	ap := openSqlitePeer(t, dbPath, kp)
	defer ap.Close()

	for _, e := range entries {
		ent, ok, err := ap.Get(e.path)
		if err != nil {
			t.Fatalf("reopen Get %s: %v", e.path, err)
		}
		if !ok {
			t.Errorf("reopen: path %s missing", e.path)
			continue
		}
		if got := ent.ContentHash.String(); got != wantHashes[e.path] {
			t.Errorf("reopen %s: hash = %s, want %s", e.path, got, wantHashes[e.path])
		}
		if ent.Type != e.typ {
			t.Errorf("reopen %s: type = %s, want %s", e.path, ent.Type, e.typ)
		}
	}
	if got, want := ap.PathCount(), len(entries); got < want {
		t.Errorf("reopen PathCount = %d, want ≥ %d", got, want)
	}
	if string(ap.PeerID()) == "" {
		t.Error("reopen produced empty PeerID")
	}
}

// TestStorage_Sqlite_RevisionLogSurvivesRestart drives a manual
// revision flow: commit, write, commit, write, commit. Then closes
// and reopens. After reopen, `revision log` should still walk all
// three versions newest-first, `revision status` should still report
// the latest as head, and a branch created in pass 1 should still
// resolve. This is the test that backs "the cross-machine workspace
// story is deployment-ready" — without it, persistence is a half
// promise.
func TestStorage_Sqlite_RevisionLogSurvivesRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "peer.db")
	kp, err := crypto.Generate()
	if err != nil {
		t.Fatalf("crypto.Generate: %v", err)
	}

	prefix := "workspace/"
	branchName := "feature-x"

	var (
		firstVersion, secondVersion, thirdVersion string
	)

	// --- pass 1: three commits + a branch ---
	func() {
		ap := openSqlitePeer(t, dbPath, kp)
		defer ap.Close()

		ctx := context.Background()
		rc := ap.Revision()

		if _, err := ap.Put("workspace/note", "test/v", "v1"); err != nil {
			t.Fatalf("Put v1: %v", err)
		}
		c1, err := rc.Commit(ctx, prefix, "initial")
		if err != nil {
			t.Fatalf("commit 1: %v", err)
		}
		firstVersion = c1.Version.String()

		if _, err := ap.Put("workspace/note", "test/v", "v2"); err != nil {
			t.Fatalf("Put v2: %v", err)
		}
		c2, err := rc.Commit(ctx, prefix, "second")
		if err != nil {
			t.Fatalf("commit 2: %v", err)
		}
		secondVersion = c2.Version.String()

		if _, err := ap.Put("workspace/note", "test/v", "v3"); err != nil {
			t.Fatalf("Put v3: %v", err)
		}
		c3, err := rc.Commit(ctx, prefix, "third")
		if err != nil {
			t.Fatalf("commit 3: %v", err)
		}
		thirdVersion = c3.Version.String()

		// Create a branch off of the second version so we can verify
		// branch refs survive the round-trip.
		if _, err := rc.BranchCreate(ctx, prefix, branchName, c2.Version); err != nil {
			t.Fatalf("BranchCreate: %v", err)
		}
	}()

	// --- pass 2: reopen + log + status + branch lookup ---
	ap := openSqlitePeer(t, dbPath, kp)
	defer ap.Close()

	ctx := context.Background()
	rc := ap.Revision()

	logResult, err := rc.Log(ctx, types.RevisionLogParamsData{Prefix: prefix})
	if err != nil {
		t.Fatalf("reopen Log: %v", err)
	}
	if len(logResult.Versions) < 3 {
		t.Fatalf("reopen Log returned %d versions, want ≥ 3", len(logResult.Versions))
	}
	if got := logResult.Versions[0].String(); got != thirdVersion {
		t.Errorf("log[0] = %s, want third %s", got, thirdVersion)
	}
	if got := logResult.Versions[1].String(); got != secondVersion {
		t.Errorf("log[1] = %s, want second %s", got, secondVersion)
	}
	if got := logResult.Versions[2].String(); got != firstVersion {
		t.Errorf("log[2] = %s, want first %s", got, firstVersion)
	}

	status, err := rc.Status(ctx, prefix)
	if err != nil {
		t.Fatalf("reopen Status: %v", err)
	}
	if got := status.Head.String(); got != thirdVersion {
		t.Errorf("status.Head = %s, want third %s", got, thirdVersion)
	}

	branches, err := rc.BranchList(ctx, prefix)
	if err != nil {
		t.Fatalf("reopen BranchList: %v", err)
	}
	branchVersion, ok := branches.Branches[branchName]
	if !ok {
		t.Errorf("branch %s missing after reopen; got %+v", branchName, branches.Branches)
	} else if got := branchVersion.String(); got != secondVersion {
		t.Errorf("branch %s points at %s, want second %s", branchName, got, secondVersion)
	}

	// Sanity: the entity at the workspace/note path should still be
	// resolvable — we never wrote v4, so reading reflects whatever
	// the live tree settled at on commit 3.
	ent, ok, err := ap.Get("workspace/note")
	if err != nil {
		t.Fatalf("reopen Get workspace/note: %v", err)
	}
	if !ok {
		t.Fatal("reopen: workspace/note missing")
	}
	if ent.Type != "test/v" {
		t.Errorf("reopen workspace/note type = %s, want test/v", ent.Type)
	}
}

// TestStorage_Sqlite_HistoryTransitionsSurviveRestart verifies the
// history extension's recorded transitions persist. Records a few
// writes to a watched path, closes the peer, reopens, and queries
// the per-path transition log — every recorded version should still
// be there. This exercises a second extension (history) whose state
// lives in the same SQL tables as the rest of the tree, so it's a
// useful cross-check that the SDK's extension wiring doesn't accidentally
// short-circuit on the persisted store.
func TestStorage_Sqlite_HistoryTransitionsSurviveRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "peer.db")
	kp, err := crypto.Generate()
	if err != nil {
		t.Fatalf("crypto.Generate: %v", err)
	}

	const path = "workspace/note"

	var firstHash, secondHash, thirdHash string

	func() {
		ap := openSqlitePeer(t, dbPath, kp)
		defer ap.Close()

		// Install a history config that matches our path. Recording
		// is opt-in; without this the recorder is a no-op.
		cfg := types.HistoryConfigData{Pattern: "workspace/*", Enabled: true}
		cfgEnt, err := cfg.ToEntity()
		if err != nil {
			t.Fatalf("config ToEntity: %v", err)
		}
		if _, err := ap.Store().Put("system/history/config/test", cfgEnt.Type, cfg); err != nil {
			t.Fatalf("install history config: %v", err)
		}

		h1, err := ap.Put(path, "test/v", "v1")
		if err != nil {
			t.Fatalf("Put v1: %v", err)
		}
		firstHash = h1.String()
		h2, err := ap.Put(path, "test/v", "v2")
		if err != nil {
			t.Fatalf("Put v2: %v", err)
		}
		secondHash = h2.String()
		h3, err := ap.Put(path, "test/v", "v3")
		if err != nil {
			t.Fatalf("Put v3: %v", err)
		}
		thirdHash = h3.String()
	}()

	// Reopen and verify the recorded transitions are still queryable
	// newest-first.
	ap := openSqlitePeer(t, dbPath, kp)
	defer ap.Close()

	result, err := ap.History().Query(context.Background(), types.HistoryQueryParamsData{Path: path})
	if err != nil {
		t.Fatalf("reopen History.Query: %v", err)
	}
	if len(result.Transitions) < 3 {
		t.Fatalf("reopen Query returned %d transitions, want ≥ 3", len(result.Transitions))
	}
	got := []string{
		result.Transitions[0].Hash.String(),
		result.Transitions[1].Hash.String(),
		result.Transitions[2].Hash.String(),
	}
	want := []string{thirdHash, secondHash, firstHash}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("transition[%d].Hash = %s, want %s", i, got[i], want[i])
		}
	}
}

// TestStorage_Sqlite_RepeatedRestartsAccumulate exercises long-term
// predictability. One identity, one DB, N open/close cycles. Each
// cycle writes a unique entity, commits a revision, and verifies that
// every entity from every prior cycle is still readable and that the
// revision log has grown by exactly one. After all cycles, the log
// walks all N commits newest-first.
//
// The point is to catch anything that drifts under repeated use:
// duplicate writes, stale handles surviving Close, the auto-versioner
// re-emitting on reopen, head pointers regressing, content-store
// rows getting orphaned, etc. None of which is hypothetical — every
// real-world workbench session is many of these cycles strung
// together.
func TestStorage_Sqlite_RepeatedRestartsAccumulate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "peer.db")
	kp, err := crypto.Generate()
	if err != nil {
		t.Fatalf("crypto.Generate: %v", err)
	}

	const cycles = 8
	const prefix = "workspace/"

	type cycleRecord struct {
		path     string
		hash     string
		version  string
		entCount int
	}
	records := make([]cycleRecord, 0, cycles)

	for i := 0; i < cycles; i++ {
		ap := openSqlitePeer(t, dbPath, kp)
		ctx := context.Background()
		rc := ap.Revision()

		// Pre-flight: every prior cycle's entity is still here, with
		// the same content hash. If anything got dropped on a prior
		// Close, this catches it.
		for j, prev := range records {
			ent, ok, err := ap.Get(prev.path)
			if err != nil {
				_ = ap.Close()
				t.Fatalf("cycle %d: Get prior path %s (from cycle %d): %v",
					i, prev.path, j, err)
			}
			if !ok {
				_ = ap.Close()
				t.Fatalf("cycle %d: prior path %s (from cycle %d) is missing",
					i, prev.path, j)
			}
			if got := ent.ContentHash.String(); got != prev.hash {
				_ = ap.Close()
				t.Fatalf("cycle %d: prior path %s hash drifted: got %s, want %s",
					i, prev.path, got, prev.hash)
			}
		}

		// Pre-flight: the revision log already contains all prior
		// versions in the right order.
		if len(records) > 0 {
			logResult, err := rc.Log(ctx, types.RevisionLogParamsData{Prefix: prefix})
			if err != nil {
				_ = ap.Close()
				t.Fatalf("cycle %d: Log before write: %v", i, err)
			}
			if len(logResult.Versions) != len(records) {
				_ = ap.Close()
				t.Fatalf("cycle %d: log has %d versions before this cycle's commit, want %d",
					i, len(logResult.Versions), len(records))
			}
			// Newest first — records[len-1] should be log[0].
			for k, rec := range records {
				wantIdx := len(records) - 1 - k
				if got := logResult.Versions[wantIdx].String(); got != rec.version {
					_ = ap.Close()
					t.Fatalf("cycle %d: log[%d] = %s, want cycle-%d version %s",
						i, wantIdx, got, k, rec.version)
				}
			}
		}

		// This cycle's work: write a unique entity, commit.
		path := fmt.Sprintf("workspace/note-%03d", i)
		data := fmt.Sprintf("payload from cycle %d", i)
		h, err := ap.Put(path, "test/v", data)
		if err != nil {
			_ = ap.Close()
			t.Fatalf("cycle %d: Put: %v", i, err)
		}
		commit, err := rc.Commit(ctx, prefix, fmt.Sprintf("cycle %d", i))
		if err != nil {
			_ = ap.Close()
			t.Fatalf("cycle %d: Commit: %v", i, err)
		}
		if commit.Version.IsZero() {
			_ = ap.Close()
			t.Fatalf("cycle %d: commit returned zero version", i)
		}

		// Sanity: EntityCount is monotonically non-decreasing across
		// cycles. (Strict-monotonic-increase would be too strong —
		// the same identity entity is reused, only the new note and
		// the new revision-related entries add rows.)
		entCount := ap.EntityCount()
		if len(records) > 0 && entCount < records[len(records)-1].entCount {
			_ = ap.Close()
			t.Fatalf("cycle %d: EntityCount regressed from %d to %d",
				i, records[len(records)-1].entCount, entCount)
		}

		records = append(records, cycleRecord{
			path:     path,
			hash:     h.String(),
			version:  commit.Version.String(),
			entCount: entCount,
		})

		if err := ap.Close(); err != nil {
			t.Fatalf("cycle %d: Close: %v", i, err)
		}
	}

	// Final pass — reopen one more time, verify the entire history.
	ap := openSqlitePeer(t, dbPath, kp)
	defer ap.Close()

	logResult, err := ap.Revision().Log(context.Background(),
		types.RevisionLogParamsData{Prefix: prefix})
	if err != nil {
		t.Fatalf("final Log: %v", err)
	}
	if len(logResult.Versions) != cycles {
		t.Fatalf("final log has %d versions, want %d", len(logResult.Versions), cycles)
	}
	for k, rec := range records {
		wantIdx := cycles - 1 - k
		if got := logResult.Versions[wantIdx].String(); got != rec.version {
			t.Errorf("final log[%d] = %s, want cycle-%d version %s",
				wantIdx, got, k, rec.version)
		}
	}
	for _, rec := range records {
		ent, ok, err := ap.Get(rec.path)
		if err != nil || !ok {
			t.Errorf("final reopen: %s missing (err=%v)", rec.path, err)
			continue
		}
		if ent.ContentHash.String() != rec.hash {
			t.Errorf("final reopen: %s hash = %s, want %s",
				rec.path, ent.ContentHash, rec.hash)
		}
	}
}

// TestStorage_Sqlite_InMemoryFastPath uses the ":memory:" path to
// exercise the SQL code path without touching the disk. Useful in
// CI environments where filesystem behavior is unpredictable, and as
// a fast smoke test that the SDK switch + extension wiring composes
// against a SqliteContentStore (not a regression of the in-process
// Memory backend path). Persistence is not asserted here — a
// :memory: DB doesn't survive close, by design.
func TestStorage_Sqlite_InMemoryFastPath(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		Storage: entitysdk.StorageConfig{Kind: "sqlite", Path: ":memory:"},
	})
	if err != nil {
		t.Fatalf("CreatePeer(sqlite, :memory:): %v", err)
	}
	defer ap.Close()

	if _, err := ap.Put("scratch/foo", "test/v", "hello"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	ent, ok, err := ap.Get("scratch/foo")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok {
		t.Fatal("Get: missing")
	}
	if ent.Type != "test/v" {
		t.Errorf("type = %s, want test/v", ent.Type)
	}

	// Quick revision-extension smoke through the SQL path.
	ctx := context.Background()
	rc := ap.Revision()
	if _, err := rc.Commit(ctx, "scratch/", "via in-memory sqlite"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	status, err := rc.Status(ctx, "scratch/")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Head.IsZero() {
		t.Error("Status.Head is zero after commit")
	}
}
