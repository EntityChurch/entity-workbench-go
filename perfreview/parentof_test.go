//go:build perfreview

package perfreview

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"entity-workbench-go/workbench"
)

// TestParentOf_LatencyAtScale isolates the suspicion that
// workbench/tree_model.go:177's parentOf() is the O(N) site that
// degrades the tree-browser panel's per-event work as the local tree
// grows.
//
// Methodology: attach a TreeBrowserModel, drive a workload, then
// measure per-Put completion latency observed by the panel — i.e. the
// time between calling Store.Put and observing the event in the
// model's known-set via Refresh(). This is the user-visible
// "event ingestion lag" that compounds with the render rate.
//
// Scale: 5K → 200K. Per-checkpoint we drive an extra 500 single-write
// "probe" Puts and measure their median + p99 ingest latency.
//
// Investigation 2 (continued) of PRODUCTION-READINESS-REVIEW.
func TestParentOf_LatencyAtScale(t *testing.T) {
	h := NewHarness(t, HarnessOptions{})
	defer h.Close()

	pctx := h.Peer().PeerContext()
	tree := workbench.NewTreeBrowserModel(pctx)
	defer tree.Close()

	checkpoints := []int{5_000, 25_000, 50_000, 100_000, 200_000}
	const probeCount = 500

	type row struct {
		Label             string
		Total             int
		BulkWriteDur      time.Duration
		BulkP99           time.Duration
		ProbeMedian       time.Duration
		ProbeP95          time.Duration
		ProbeP99          time.Duration
		PanelInsertMedian time.Duration
	}
	results := []row{}

	prevTotal := 0
	for _, target := range checkpoints {
		batch := target - prevTotal
		bulkDur, _, _, bulkP99 := h.Workload("bench", prevTotal, batch)
		prevTotal = target

		// Drain.
		waitForTreeConvergence(tree, prevTotal, 30*time.Second)

		// Probe phase: write 500 single entities under a new prefix,
		// measure both Put latency (writer-side) and the panel's
		// onEvent processing latency (consumer-side, via Refresh
		// signal).
		probePrefix := fmt.Sprintf("probe-%d", target)
		probePutLatencies := make([]time.Duration, 0, probeCount)
		panelLatencies := make([]time.Duration, 0, probeCount)
		storeAPI := h.Peer().Store()

		for i := 0; i < probeCount; i++ {
			path := fmt.Sprintf("%s/%05d", probePrefix, i)
			payload := map[string]interface{}{"tick": i, "time": "x"}

			t0 := time.Now()
			if _, err := storeAPI.Put(path, "perfreview/probe", payload); err != nil {
				t.Fatalf("probe Put: %v", err)
			}
			putElapsed := time.Since(t0)
			probePutLatencies = append(probePutLatencies, putElapsed)

			// Spin until Refresh signals the panel has processed events,
			// up to a generous timeout. The latency we want is "when did
			// the model observe this path?" — Refresh going from
			// not-dirty → dirty → not-dirty captures it.
			panelT0 := time.Now()
			for time.Since(panelT0) < 100*time.Millisecond {
				if tree.Refresh() {
					break
				}
				time.Sleep(50 * time.Microsecond)
			}
			panelLatencies = append(panelLatencies, time.Since(panelT0))
		}

		sort.Slice(probePutLatencies, func(i, j int) bool { return probePutLatencies[i] < probePutLatencies[j] })
		sort.Slice(panelLatencies, func(i, j int) bool { return panelLatencies[i] < panelLatencies[j] })

		results = append(results, row{
			Label:             fmt.Sprintf("N=%d", target),
			Total:             prevTotal,
			BulkWriteDur:      bulkDur,
			BulkP99:           bulkP99,
			ProbeMedian:       probePutLatencies[len(probePutLatencies)*50/100],
			ProbeP95:          probePutLatencies[len(probePutLatencies)*95/100],
			ProbeP99:          probePutLatencies[len(probePutLatencies)*99/100],
			PanelInsertMedian: panelLatencies[len(panelLatencies)*50/100],
		})
	}

	t.Logf("\n%-12s %10s %10s %10s %12s %12s %12s %14s",
		"label", "total", "bulk-dur", "bulk-p99", "put-p50", "put-p95", "put-p99", "panel-ingest")
	for _, r := range results {
		t.Logf("%-12s %10d %10s %10s %12s %12s %12s %14s",
			r.Label, r.Total,
			short(r.BulkWriteDur), short(r.BulkP99),
			short(r.ProbeMedian), short(r.ProbeP95), short(r.ProbeP99),
			short(r.PanelInsertMedian))
	}
}
