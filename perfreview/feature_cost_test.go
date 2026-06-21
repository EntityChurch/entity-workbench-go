//go:build perfreview

package perfreview

import (
	"context"
	"fmt"
	"testing"
	"time"

	coretypes "go.entitychurch.org/entity-core-go/core/types"
)

// TestFeatureCost_HistoryRecording measures the per-write cost of
// enabling the history extension's per-path transition recording on
// the workload prefix.
//
// Mechanism: install a HistoryConfigData entity at
// system/history/config/perfreview matching the workload pattern.
// The recorder hot-reloads its config cache; subsequent Puts on
// matching paths emit a transition entity.
//
// Investigation 10 of PRODUCTION-READINESS-REVIEW.
func TestFeatureCost_HistoryRecording(t *testing.T) {
	runFeatureCost(t, "history", func(h *Harness) {
		cfg := coretypes.HistoryConfigData{
			Pattern: "bench/*",
			Enabled: true,
		}
		if _, err := h.Peer().Store().Put(
			"system/history/config/perfreview",
			"system/history/config",
			cfg,
		); err != nil {
			t.Fatalf("install history config: %v", err)
		}
	})
}

// TestFeatureCost_RevisionAutoVersion measures the per-write cost of
// auto-versioning a prefix. Each Put triggers a trie root recompute
// + revision entry creation when the prefix is configured.
//
// Investigation 11 of PRODUCTION-READINESS-REVIEW.
func TestFeatureCost_RevisionAutoVersion(t *testing.T) {
	runFeatureCost(t, "revision-auto", func(h *Harness) {
		yes := true
		cfg := coretypes.RevisionConfigData{
			Prefix:      "bench/",
			AutoVersion: &yes,
		}
		// Use the typed RevisionClient — the underlying path is
		// system/revision/{prefix_hash}/config (hashed), so we can't
		// just Store.Put a hand-crafted path.
		params := coretypes.RevisionConfigParamsData{
			Action: "set",
			Name:   "perfreview",
			Config: &cfg,
		}
		if _, err := h.Peer().Revision().Config(context.Background(), params); err != nil {
			t.Fatalf("install revision config: %v", err)
		}
	}, 100, 500, 1_000, 2_000, 5_000)
}

// TestFeatureCost_HistoryPlusRevision measures the combined cost of
// having both history recording AND auto-version active on the same
// workload prefix.
//
// Investigation 12 of PRODUCTION-READINESS-REVIEW.
func TestFeatureCost_HistoryPlusRevision(t *testing.T) {
	runFeatureCost(t, "history+revision", func(h *Harness) {
		histCfg := coretypes.HistoryConfigData{
			Pattern: "bench/*",
			Enabled: true,
		}
		if _, err := h.Peer().Store().Put(
			"system/history/config/perfreview",
			"system/history/config",
			histCfg,
		); err != nil {
			t.Fatalf("install history config: %v", err)
		}

		yes := true
		revCfg := coretypes.RevisionConfigData{
			Prefix:      "bench/",
			AutoVersion: &yes,
		}
		params := coretypes.RevisionConfigParamsData{
			Action: "set",
			Name:   "perfreview",
			Config: &revCfg,
		}
		if _, err := h.Peer().Revision().Config(context.Background(), params); err != nil {
			t.Fatalf("install revision config: %v", err)
		}
	}, 100, 500, 1_000, 2_000, 5_000)
}

// TestFeatureCost_HistoryRecording_Diagnostic mirrors the auto-version
// diagnostic for direct comparison. Drives 500 Puts with history
// recording on, logs per-Put latency every 50 Puts to show whether
// latency is bounded or grows with N.
func TestFeatureCost_HistoryRecording_Diagnostic(t *testing.T) {
	h := NewHarness(t, HarnessOptions{})
	defer h.Close()

	cfg := coretypes.HistoryConfigData{
		Pattern: "bench/*",
		Enabled: true,
	}
	if _, err := h.Peer().Store().Put(
		"system/history/config/perfreview",
		"system/history/config",
		cfg,
	); err != nil {
		t.Fatalf("install history config: %v", err)
	}

	storeAPI := h.Peer().Store()
	const N = 500
	t.Logf("starting %d Puts with history recording on bench/*", N)
	for i := 0; i < N; i++ {
		path := fmt.Sprintf("bench/%07d", i)
		start := time.Now()
		if _, err := storeAPI.Put(path, "perfreview/entity",
			map[string]interface{}{"tick": i, "time": "x"}); err != nil {
			t.Fatalf("Put %d: %v", i, err)
		}
		elapsed := time.Since(start)
		if i%50 == 0 {
			m := h.Snapshot("", 0, 0, 0, 0, 0)
			t.Logf("put %3d: %s (entities=%d locs=%d heap=%.1fMiB)",
				i, short(elapsed), m.EntityCount, m.LocationCount,
				float64(m.HeapInUseBytes)/1024/1024)
		}
	}
}

// TestFeatureCost_RevisionAutoVersion_Diagnostic does a tiny 100-Put
// run with auto-version on to characterize per-Put cost when scaling
// out times out. Logs progress every 10 Puts so we can see if/where
// latency cliffs.
func TestFeatureCost_RevisionAutoVersion_Diagnostic(t *testing.T) {
	h := NewHarness(t, HarnessOptions{})
	defer h.Close()

	yes := true
	cfg := coretypes.RevisionConfigData{
		Prefix:      "bench/",
		AutoVersion: &yes,
	}
	params := coretypes.RevisionConfigParamsData{
		Action: "set",
		Name:   "perfreview",
		Config: &cfg,
	}
	if _, err := h.Peer().Revision().Config(context.Background(), params); err != nil {
		t.Fatalf("install revision config: %v", err)
	}

	storeAPI := h.Peer().Store()
	const N = 500
	t.Logf("starting %d Puts with auto-version on bench/ prefix", N)
	for i := 0; i < N; i++ {
		path := fmt.Sprintf("bench/%07d", i)
		start := time.Now()
		if _, err := storeAPI.Put(path, "perfreview/entity",
			map[string]interface{}{"tick": i, "time": "x"}); err != nil {
			t.Fatalf("Put %d: %v", i, err)
		}
		elapsed := time.Since(start)
		if i%50 == 0 {
			m := h.Snapshot("", 0, 0, 0, 0, 0)
			t.Logf("put %3d: %s (entities=%d locs=%d heap=%.1fMiB)",
				i, short(elapsed), m.EntityCount, m.LocationCount,
				float64(m.HeapInUseBytes)/1024/1024)
		}
	}
}

// runFeatureCost drives a shared scaling workload after applying a
// per-feature configure hook. Same workload as the scale baseline so
// the cost is directly comparable. The optional `steps` slice lets
// individual investigations scale down when a feature is too expensive
// to run at baseline scale (e.g., revision auto-version is O(depth)
// trie work per write, much more expensive than plain Puts).
func runFeatureCost(t *testing.T, label string, configure func(*Harness), steps ...int) {
	if len(steps) == 0 {
		steps = []int{1_000, 5_000, 25_000, 50_000, 100_000}
	}
	h := NewHarness(t, HarnessOptions{})
	defer h.Close()

	configure(h)

	rows := make([]Metrics, 0, len(steps)+1)
	rows = append(rows, h.Snapshot("bootstrap", 0, 0, 0, 0, 0))

	prevTotal := 0
	for _, target := range steps {
		batchSize := target - prevTotal
		writeDur, p50, p95, p99 := h.Workload("bench", prevTotal, batchSize)
		prevTotal = target

		row := h.Snapshot(fmt.Sprintf("N=%d", target), batchSize, writeDur, p50, p95, p99)
		rows = append(rows, row)
	}

	t.Logf("\n=== feature: %s ===\n%s", label, FormatMetricsTable(rows))
}
