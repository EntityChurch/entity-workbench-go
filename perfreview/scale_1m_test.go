//go:build perfreview

package perfreview

import (
	"fmt"
	"testing"
)

// TestScale_PushTo1M extends investigation 1 to higher N: 250K, 500K,
// 1M, 2M. Does anything cliff? Where does write latency start to hurt?
//
// Investigation 6 of PRODUCTION-READINESS-REVIEW.
func TestScale_PushTo1M(t *testing.T) {
	h := NewHarness(t, HarnessOptions{})
	defer h.Close()

	steps := []int{250_000, 500_000, 1_000_000, 2_000_000}
	rows := make([]Metrics, 0, len(steps)+1)
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
