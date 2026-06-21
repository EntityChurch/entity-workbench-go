//go:build perfreview

package perfreview

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"entity-workbench-go/entitysdk"
)

// TestFanout_SubscribersPerEvent measures dispatch cost as a function
// of subscriber count. N subscribers × M writes — does per-write cost
// scale linearly in N (good), or super-linearly (bad)?
//
// Each subscriber is a no-op handler that just increments a counter.
// Eliminates per-handler work as a confound so we're measuring the
// hub's fan-out cost in isolation.
//
// What this answers: if you wire up 10 panels (tree, markdown-files,
// inspector, log, peer-info, query, …) is the dispatch O(N panels)
// per event (acceptable) or worse?
//
// Investigation 5 of PRODUCTION-READINESS-REVIEW.
func TestFanout_SubscribersPerEvent(t *testing.T) {
	for _, nSubs := range []int{0, 1, 4, 16, 64} {
		nSubs := nSubs
		t.Run(fmt.Sprintf("subs=%d", nSubs), func(t *testing.T) {
			h := NewHarness(t, HarnessOptions{})
			defer h.Close()

			var counter atomic.Int64
			cancels := make([]func(), 0, nSubs)
			for i := 0; i < nSubs; i++ {
				cancel := h.Peer().Store().OnPrefixChange("", func(ev entitysdk.ChangeEvent) {
					counter.Add(1)
				})
				cancels = append(cancels, cancel)
			}
			defer func() {
				for _, c := range cancels {
					c()
				}
			}()

			const N = 50_000
			dur, p50, p95, p99 := h.Workload("fanout", 0, N)

			// Wait for fan-out to drain.
			expected := int64(nSubs) * int64(N+243) // 243 = bootstrap entity count seeded to each sub
			deadline := time.Now().Add(30 * time.Second)
			for counter.Load() < expected && time.Now().Before(deadline) {
				time.Sleep(50 * time.Millisecond)
			}

			t.Logf("subs=%d N=%d: write-dur=%s p50=%s p95=%s p99=%s delivered=%d/%d",
				nSubs, N, short(dur), short(p50), short(p95), short(p99),
				counter.Load(), expected)
		})
	}
}
