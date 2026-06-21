//go:build perfreview

package perfreview

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestMixed_ReadDuringWrite drives concurrent readers while the writer
// loop fills the store. Real workbench scenario: a panel showing data
// (Get calls) while the heartbeat / mount-watcher / revision-commit
// writes happen in parallel. Looks for read-latency degradation under
// write pressure, and verifies the read-pool-split (multiple SQLite
// connections for reads) is doing its job.
//
// Methodology: pre-seed N entities. Then run a writer goroutine doing
// continuous Puts AND R reader goroutines doing continuous Gets against
// known-existent paths. Sample read latency throughout. Run for a fixed
// wall-time window (no early termination).
//
// Investigation 9 of PRODUCTION-READINESS-REVIEW.
func TestMixed_ReadDuringWrite(t *testing.T) {
	const seedSize = 50_000
	const window = 5 * time.Second

	for _, nReaders := range []int{1, 4, 16} {
		nReaders := nReaders
		t.Run(fmt.Sprintf("readers=%d", nReaders), func(t *testing.T) {
			h := NewHarness(t, HarnessOptions{})
			defer h.Close()

			// Seed.
			h.Workload("mixed", 0, seedSize)
			storeAPI := h.Peer().Store()

			ctx, cancel := context.WithTimeout(context.Background(), window)
			defer cancel()

			// Writer goroutine.
			var writeCount atomic.Int64
			var writeLatTotal atomic.Int64
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := seedSize; ; i++ {
					select {
					case <-ctx.Done():
						return
					default:
					}
					path := fmt.Sprintf("mixed/%07d", i)
					payload := map[string]interface{}{"tick": i, "time": "x"}
					start := time.Now()
					if _, err := storeAPI.Put(path, "perfreview/entity", payload); err != nil {
						t.Errorf("writer Put: %v", err)
						return
					}
					writeLatTotal.Add(int64(time.Since(start)))
					writeCount.Add(1)
				}
			}()

			// Reader goroutines.
			readerLatsCh := make(chan []time.Duration, nReaders)
			for r := 0; r < nReaders; r++ {
				r := r
				wg.Add(1)
				go func() {
					defer wg.Done()
					lats := make([]time.Duration, 0, 100_000)
					// Read against pre-seeded paths so Get always succeeds.
					for i := uint64(r); ; i = (i + uint64(nReaders)) % seedSize {
						select {
						case <-ctx.Done():
							readerLatsCh <- lats
							return
						default:
						}
						path := fmt.Sprintf("mixed/%07d", i)
						start := time.Now()
						_, _ = storeAPI.Get(path)
						lats = append(lats, time.Since(start))
					}
				}()
			}
			wg.Wait()
			close(readerLatsCh)

			all := make([]time.Duration, 0)
			for lats := range readerLatsCh {
				all = append(all, lats...)
			}
			sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })

			readCount := len(all)
			readP50 := all[readCount*50/100]
			readP95 := all[readCount*95/100]
			readP99 := all[readCount*99/100]

			wcnt := writeCount.Load()
			wlatAvg := time.Duration(0)
			if wcnt > 0 {
				wlatAvg = time.Duration(writeLatTotal.Load() / wcnt)
			}

			t.Logf("\nreaders=%d window=%s writes=%d (%.0f/s, avg %s)  reads=%d (%.0f/s) p50=%s p95=%s p99=%s",
				nReaders, window, wcnt, float64(wcnt)/window.Seconds(), short(wlatAvg),
				readCount, float64(readCount)/window.Seconds(),
				short(readP50), short(readP95), short(readP99))
		})
	}
}
