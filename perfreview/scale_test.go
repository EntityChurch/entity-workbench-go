//go:build perfreview

package perfreview

import (
	"fmt"
	"testing"
)

// TestScale_HeartbeatBaseline drives the scaling fixture with the same
// shape as the canvas/console heartbeat writer (2-3 field payload, one
// path per write under a monotonically-growing prefix) but in batches
// at log-spaced cumulative N. Captures heap, goroutines, sqlite size,
// write latency at each checkpoint.
//
// What it answers: "is the peer (storage + dispatch) the bottleneck at
// scale, or is it something downstream of writes?" If write latency,
// heap, and goroutines stay flat (or grow benignly), the freeze is
// render-driven. If they explode here, the freeze is peer-driven and
// the canvas is just the messenger.
//
// Investigation 1 of the production-readiness review.
func TestScale_HeartbeatBaseline(t *testing.T) {
	h := NewHarness(t, HarnessOptions{})
	defer h.Close()

	steps := LogScaleSteps()
	rows := make([]Metrics, 0, len(steps)+1)

	// Baseline snapshot before any workload writes — bootstrap-only
	// state. Tells us what the SDK + sqlite-init cost on their own.
	rows = append(rows, h.Snapshot("bootstrap", 0, 0, 0, 0, 0))

	prevTotal := 0
	for _, target := range steps {
		batchSize := target - prevTotal
		writeDur, p50, p95, p99 := h.Workload("bench", prevTotal, batchSize)
		prevTotal = target

		label := fmt.Sprintf("N=%d", target)
		rows = append(rows, h.Snapshot(label, batchSize, writeDur, p50, p95, p99))
	}

	t.Logf("\n%s", FormatMetricsTable(rows))
}
