//go:build perfreview

// Package perfreview is the production-readiness measurement harness.
//
// All files in this package are gated by `//go:build perfreview` so
// they don't run in the default `make test` sweep. Invoke via
// `make perfreview` (no -race; race detector imposes ~17× slowdown
// on modernc.org/sqlite per feedback_race_detector_vs_sqlite memo).
//
// The harness boots an entitysdk peer with sqlite storage, drives a
// configurable write workload, and snapshots metrics (heap, goroutines,
// SQLite file size, write latency) at log-spaced checkpoints. The
// shared fixture is reused across investigations 1–5 documented in
// the production-readiness review.
package perfreview

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"entity-workbench-go/entitysdk"
)

// Metrics captures the observable state of a peer at one moment.
type Metrics struct {
	Label          string
	EntityCount    int
	LocationCount  int
	HeapInUseBytes uint64
	HeapAllocBytes uint64
	NumGC          uint32
	Goroutines     int
	SQLiteBytes    int64
	WriteCount     int
	WriteDur       time.Duration
	P50, P95, P99  time.Duration
	WallSinceStart time.Duration
}

// HarnessOptions controls the shared fixture.
type HarnessOptions struct {
	DBPath        string // empty → temp dir + peer.db
	PayloadFields int    // default 3 (matches heartbeat shape)
	// IdentityName, when set, binds the peer to a named on-disk
	// identity. The identity is auto-created if it doesn't exist. Use
	// this for tests that close + reopen the peer and need a stable
	// peer-id across the cycle.
	IdentityName string
}

// Harness owns the peer + workdir + measurement state across an
// investigation. One per Test* function.
type Harness struct {
	t       testing.TB
	peer    *entitysdk.AppPeer
	dbPath  string
	workDir string
	start   time.Time
	opts    HarnessOptions

	roDB *sql.DB // read-only handle for entity-count queries
}

// NewHarness boots a sqlite-backed peer + returns the Harness handle.
// Caller must defer h.Close(); harness destroys the workdir on Close.
func NewHarness(t testing.TB, opts HarnessOptions) *Harness {
	t.Helper()

	if opts.PayloadFields <= 0 {
		opts.PayloadFields = 3
	}
	if opts.DBPath == "" {
		dir := t.TempDir()
		opts.DBPath = filepath.Join(dir, "peer.db")
	}

	if os.Getenv("HOME") == "" || !filepath.IsAbs(os.Getenv("HOME")) {
		t.Setenv("HOME", t.TempDir())
	}

	cfg := entitysdk.PeerConfig{
		Storage: entitysdk.StorageConfig{Kind: "sqlite", Path: opts.DBPath},
	}
	if opts.IdentityName != "" {
		// Auto-create if absent. CreateIdentity wraps ErrIdentityExists
		// in an SDK error; unwrap with errors.Is for the restart case
		// (same on-disk keypair reused across runs).
		_, err := entitysdk.CreateIdentity(opts.IdentityName)
		if err != nil && !errors.Is(err, entitysdk.ErrIdentityExists) {
			t.Fatalf("perfreview: CreateIdentity %s: %v", opts.IdentityName, err)
		}
		cfg.Identity = &entitysdk.IdentityBindingConfig{Name: opts.IdentityName}
	}
	ap, err := entitysdk.CreatePeer(cfg)
	if err != nil {
		t.Fatalf("perfreview: CreatePeer: %v", err)
	}

	// Read-only handle for queries that bypass the SDK's writer. WAL
	// mode allows concurrent readers without blocking the writer.
	roDB, err := sql.Open("sqlite", "file:"+opts.DBPath+"?mode=ro")
	if err != nil {
		ap.Close()
		t.Fatalf("perfreview: open read-only sqlite: %v", err)
	}

	return &Harness{
		t:       t,
		peer:    ap,
		dbPath:  opts.DBPath,
		workDir: filepath.Dir(opts.DBPath),
		start:   time.Now(),
		opts:    opts,
		roDB:    roDB,
	}
}

// Close releases the peer + read handle.
func (h *Harness) Close() {
	if h.roDB != nil {
		h.roDB.Close()
		h.roDB = nil
	}
	if h.peer != nil {
		h.peer.Close()
		h.peer = nil
	}
}

// Peer exposes the AppPeer for tests that need to register handlers or
// drive workloads beyond the standard write path.
func (h *Harness) Peer() *entitysdk.AppPeer { return h.peer }

// DBPath returns the on-disk SQLite path.
func (h *Harness) DBPath() string { return h.dbPath }

// Workload writes n entities under `<prefix>/{startIdx + k}` with a
// fixed-shape payload. Caller supplies startIdx so multiple batches
// produce unique paths (matching the heartbeat shape: monotonically-
// incrementing tick = monotonically-incrementing path). Returns wall
// time + percentile latencies. Metrics snapshot is taken by the caller
// around the workload via h.Snapshot.
func (h *Harness) Workload(prefix string, startIdx, n int) (writeDur time.Duration, p50, p95, p99 time.Duration) {
	h.t.Helper()

	latencies := make([]time.Duration, 0, n)
	now := time.Now()
	storeAPI := h.peer.Store()

	t0 := time.Now()
	for i := 0; i < n; i++ {
		path := fmt.Sprintf("%s/%07d", prefix, startIdx+i)
		payload := map[string]interface{}{
			"tick": i,
			"time": now.Format(time.RFC3339Nano),
		}
		for j := 0; j < h.opts.PayloadFields-2; j++ {
			payload[fmt.Sprintf("filler%d", j)] = fmt.Sprintf("filler-value-%d", j)
		}
		writeStart := time.Now()
		if _, err := storeAPI.Put(path, "perfreview/entity", payload); err != nil {
			h.t.Fatalf("perfreview: Put %s: %v", path, err)
		}
		latencies = append(latencies, time.Since(writeStart))
	}
	writeDur = time.Since(t0)

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p50 = latencies[len(latencies)*50/100]
	p95 = latencies[len(latencies)*95/100]
	p99 = latencies[len(latencies)*99/100]
	return
}

// Snapshot captures Metrics at the current peer state. runtime.GC()
// runs first so heap measurements reflect retained (not garbage) state.
// Entity + location counts come from a read-only SQLite handle.
func (h *Harness) Snapshot(label string, writeCount int, writeDur, p50, p95, p99 time.Duration) Metrics {
	h.t.Helper()

	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	dbSize := int64(0)
	if info, err := os.Stat(h.dbPath); err == nil {
		dbSize = info.Size()
	}

	entityCount, locationCount := h.sqliteCounts()

	return Metrics{
		Label:          label,
		EntityCount:    entityCount,
		LocationCount:  locationCount,
		HeapInUseBytes: ms.HeapInuse,
		HeapAllocBytes: ms.HeapAlloc,
		NumGC:          ms.NumGC,
		Goroutines:     runtime.NumGoroutine(),
		SQLiteBytes:    dbSize,
		WriteCount:     writeCount,
		WriteDur:       writeDur,
		P50:            p50,
		P95:            p95,
		P99:            p99,
		WallSinceStart: time.Since(h.start),
	}
}

// countEntities opens a fresh read-only sqlite handle and reports the
// entity-row count. Used by tests that don't use the Harness.
func countEntities(t testing.TB, dbPath string) int {
	t.Helper()
	roDB, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		t.Logf("countEntities open: %v", err)
		return -1
	}
	defer roDB.Close()
	var count int
	if err := roDB.QueryRow("SELECT count(*) FROM entities").Scan(&count); err != nil {
		t.Logf("countEntities scan: %v", err)
		return -1
	}
	return count
}

// sqliteCounts reads entity + location counts via the read-only handle.
// Returns -1 on error (logged, not fatal — keeps the harness running).
func (h *Harness) sqliteCounts() (int, int) {
	var entCount, locCount int
	if err := h.roDB.QueryRow("SELECT count(*) FROM entities").Scan(&entCount); err != nil {
		h.t.Logf("perfreview: entity count: %v", err)
		entCount = -1
	}
	if err := h.roDB.QueryRow("SELECT count(*) FROM locations").Scan(&locCount); err != nil {
		h.t.Logf("perfreview: location count: %v", err)
		locCount = -1
	}
	return entCount, locCount
}

// FormatMetricsTable formats a slice of Metrics as a tab-padded table.
// Sized to fit comfortably in an 180-column status doc.
func FormatMetricsTable(rows []Metrics) string {
	header := fmt.Sprintf("%-18s %9s %9s %9s %9s %4s %6s %9s %8s %8s %8s %8s %8s",
		"label", "entities", "locs", "heap-MiB", "alloc-MiB", "GC", "goroso",
		"db-MiB", "writes", "wr-dur", "p50", "p95", "p99")
	out := header + "\n"
	out += "------------------ --------- --------- --------- --------- ---- ------ --------- -------- -------- -------- -------- --------\n"
	for _, m := range rows {
		out += fmt.Sprintf("%-18s %9d %9d %9.1f %9.1f %4d %6d %9.1f %8d %8s %8s %8s %8s\n",
			truncate(m.Label, 18), m.EntityCount, m.LocationCount,
			float64(m.HeapInUseBytes)/1024/1024,
			float64(m.HeapAllocBytes)/1024/1024,
			m.NumGC,
			m.Goroutines,
			float64(m.SQLiteBytes)/1024/1024,
			m.WriteCount,
			short(m.WriteDur),
			short(m.P50),
			short(m.P95),
			short(m.P99))
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// short renders a duration in a compact, table-friendly form.
func short(d time.Duration) string {
	switch {
	case d >= time.Second:
		return fmt.Sprintf("%.2fs", d.Seconds())
	case d >= time.Millisecond:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d >= time.Microsecond:
		return fmt.Sprintf("%dµs", d.Microseconds())
	default:
		return fmt.Sprintf("%dns", d.Nanoseconds())
	}
}

// WorkloadAtSamePaths overwrites the existing paths under `prefix`
// with a fresh payload, salted by `round` so each round produces
// distinct content hashes (otherwise the content-store would dedupe).
// Used by steady-state tests that want to measure write churn without
// growing the path namespace.
func (h *Harness) WorkloadAtSamePaths(prefix string, n, round int) (writeDur, p50, p95, p99 time.Duration) {
	h.t.Helper()

	latencies := make([]time.Duration, 0, n)
	storeAPI := h.peer.Store()

	t0 := time.Now()
	for i := 0; i < n; i++ {
		path := fmt.Sprintf("%s/%07d", prefix, i)
		payload := map[string]interface{}{
			"tick":  i,
			"round": round,
			"time":  fmt.Sprintf("round-%d-tick-%d", round, i),
		}
		for j := 0; j < h.opts.PayloadFields-2; j++ {
			payload[fmt.Sprintf("filler%d", j)] = fmt.Sprintf("filler-value-%d", j)
		}
		writeStart := time.Now()
		if _, err := storeAPI.Put(path, "perfreview/entity", payload); err != nil {
			h.t.Fatalf("perfreview: rewrite Put %s: %v", path, err)
		}
		latencies = append(latencies, time.Since(writeStart))
	}
	writeDur = time.Since(t0)

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p50 = latencies[len(latencies)*50/100]
	p95 = latencies[len(latencies)*95/100]
	p99 = latencies[len(latencies)*99/100]
	return
}

// LogScaleSteps returns log-spaced N values for scaling investigations.
// Default sequence: 1K, 2K, 5K, 10K, 25K, 50K, 100K, 200K.
func LogScaleSteps() []int {
	return []int{1_000, 2_000, 5_000, 10_000, 25_000, 50_000, 100_000, 200_000}
}
