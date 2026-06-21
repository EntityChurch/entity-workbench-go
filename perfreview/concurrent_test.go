//go:build perfreview

package perfreview

import (
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"
)

// TestConcurrent_Writers drives N parallel writer goroutines, each
// writing through the same AppPeer.Store(). Measures aggregate
// throughput, per-write latency, and looks for lock contention.
//
// Real workbench scenarios with parallel writes: multi-panel shells
// each driving REPL commands, mount + heartbeat racing each other,
// concurrent revision commits.
//
// Investigation 7 of PRODUCTION-READINESS-REVIEW.
func TestConcurrent_Writers(t *testing.T) {
	const totalWrites = 100_000

	for _, nWriters := range []int{1, 4, 16, 64} {
		nWriters := nWriters
		t.Run(fmt.Sprintf("writers=%d", nWriters), func(t *testing.T) {
			h := NewHarness(t, HarnessOptions{})
			defer h.Close()

			perWriter := totalWrites / nWriters
			storeAPI := h.Peer().Store()

			// Collect latencies from each worker into a per-worker slice;
			// merge after the workload to compute global percentiles.
			workerLats := make([][]time.Duration, nWriters)

			var wg sync.WaitGroup
			t0 := time.Now()
			for w := 0; w < nWriters; w++ {
				w := w
				wg.Add(1)
				go func() {
					defer wg.Done()
					lats := make([]time.Duration, 0, perWriter)
					prefix := fmt.Sprintf("conc/%02d", w)
					for i := 0; i < perWriter; i++ {
						path := fmt.Sprintf("%s/%07d", prefix, i)
						payload := map[string]interface{}{
							"tick":   i,
							"worker": w,
							"time":   "t",
						}
						start := time.Now()
						if _, err := storeAPI.Put(path, "perfreview/entity", payload); err != nil {
							t.Errorf("worker %d Put: %v", w, err)
							return
						}
						lats = append(lats, time.Since(start))
					}
					workerLats[w] = lats
				}()
			}
			wg.Wait()
			totalDur := time.Since(t0)

			// Merge latencies + compute global stats.
			all := make([]time.Duration, 0, totalWrites)
			for _, lats := range workerLats {
				all = append(all, lats...)
			}
			sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })

			p50 := all[len(all)*50/100]
			p95 := all[len(all)*95/100]
			p99 := all[len(all)*99/100]
			throughput := float64(totalWrites) / totalDur.Seconds()

			m := h.Snapshot(fmt.Sprintf("w=%d", nWriters), totalWrites, totalDur, p50, p95, p99)
			t.Logf("\nwriters=%d totalDur=%s throughput=%.0f/s p50=%s p95=%s p99=%s heap=%.1fMiB",
				nWriters, short(totalDur), throughput,
				short(p50), short(p95), short(p99),
				float64(m.HeapInUseBytes)/1024/1024)
		})
	}
}
