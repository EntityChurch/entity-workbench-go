//go:build perfreview

package perfreview

import (
	"fmt"
	"testing"
)

// TestSteady_OverwriteSamePathset writes N entities, then OVERWRITES
// the same N paths multiple times. With a content-addressed store,
// overwriting at the same path creates a new content-addressed entity
// each time, but locations don't grow.
//
// What this answers: does memory grow proportional to writes (good —
// bounded by N), or proportional to total Put count (bad — leak)?
// If heap grows across rounds, something is retaining per-write state.
//
// Investigation 3 of PRODUCTION-READINESS-REVIEW.
func TestSteady_OverwriteSamePathset(t *testing.T) {
	h := NewHarness(t, HarnessOptions{})
	defer h.Close()

	const N = 50_000
	const rounds = 4

	rows := make([]Metrics, 0, rounds+2)
	rows = append(rows, h.Snapshot("bootstrap", 0, 0, 0, 0, 0))

	// Initial seed.
	dur, p50, p95, p99 := h.Workload("steady", 0, N)
	rows = append(rows, h.Snapshot("seed", N, dur, p50, p95, p99))

	// Rounds of overwrite at the same paths. Locations stay at N;
	// entity count grows by N each round (immutable content-store
	// keeps each version's content entity).
	for round := 1; round <= rounds; round++ {
		dur, p50, p95, p99 := h.WorkloadAtSamePaths("steady", N, round)
		label := fmt.Sprintf("rewrite-%d", round)
		rows = append(rows, h.Snapshot(label, N, dur, p50, p95, p99))
	}

	t.Logf("\n%s", FormatMetricsTable(rows))
}
