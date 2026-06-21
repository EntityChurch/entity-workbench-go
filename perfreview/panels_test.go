//go:build perfreview

package perfreview

import (
	"fmt"
	"testing"
	"time"

	"entity-workbench-go/workbench"
)

// TestPanels_TreeBrowserAtScale wires a TreeBrowserModel — the panel
// that subscribes to OnPrefixChange("") (every mutation under this
// peer) — to the same scaling fixture as the baseline. Measures whether
// the tree-browser's per-event work crosses a usability threshold as
// N grows.
//
// Suspect surfaced by code reading:
//
//	workbench/tree_model.go:160 — onEvent's "walk up to expand depth-0"
//	loop calls parentOf(m.Root, n) per ancestor level. parentOf is a
//	full recursive tree scan: at N=150K with many siblings under one
//	prefix, finding a leaf's parent walks all its siblings — O(N).
//	The docstring claim "O(depth) per event" is incorrect.
//
// What this test answers: does the tree-browser model's onEvent
// latency degrade with N? If yes, parentOf is the bug. If no, the
// freeze is elsewhere (channel backpressure / render thread / etc.).
//
// Investigation 2 of the production-readiness review.
func TestPanels_TreeBrowserAtScale(t *testing.T) {
	h := NewHarness(t, HarnessOptions{})
	defer h.Close()

	pctx := h.Peer().PeerContext()
	tree := workbench.NewTreeBrowserModel(pctx)
	defer tree.Close()

	steps := LogScaleSteps()
	rows := make([]Metrics, 0, len(steps)+1)
	rows = append(rows, h.Snapshot("bootstrap", 0, 0, 0, 0, 0))

	prevTotal := 0
	for _, target := range steps {
		batchSize := target - prevTotal
		writeDur, p50, p95, p99 := h.Workload("bench", prevTotal, batchSize)
		prevTotal = target

		// Drain pending tree-browser events so the snapshot reflects
		// the model's state AFTER processing the batch (not in the
		// middle of catching up). We poll for known-size convergence
		// with a soft timeout — if it doesn't drain, that's itself a
		// finding (the panel can't keep up with the writer).
		drained := waitForTreeConvergence(tree, prevTotal+243 /* bootstrap base */, 5*time.Second)
		label := fmt.Sprintf("N=%d", target)
		if !drained {
			label += "!drain"
		}

		rows = append(rows, h.Snapshot(label, batchSize, writeDur, p50, p95, p99))
	}

	t.Logf("\n%s", FormatMetricsTable(rows))
}

// waitForTreeConvergence polls until the tree-browser's known-set count
// approximately matches the expected entity count, or timeout. Returns
// true if convergence reached.
func waitForTreeConvergence(tree *workbench.TreeBrowserModel, expectedAtLeast int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		// TreeBrowserModel doesn't expose known-count directly, but
		// Refresh() returns true when there's pending work. When it
		// stays false through a few polls, we're drained.
		if !tree.Refresh() {
			// Double-check after a brief settle.
			time.Sleep(10 * time.Millisecond)
			if !tree.Refresh() {
				return true
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}
