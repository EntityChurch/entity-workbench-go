//go:build perfreview

package perfreview

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"entity-workbench-go/entitysdk"
)

// TestSubscription_FindThreshold sweeps write rates with 1 subscriber
// to find where the delivery loop saturates. Identifies the
// stable-vs-degraded boundary.
//
// Method: install 1 subscription on watched/*. At each target write
// rate, drive writes with paced sleeps. Measure delivery completion
// rate (delivered / sent) and engine-reported drop counter.
func TestSubscription_FindThreshold(t *testing.T) {
	type result struct {
		Rate        int
		Sent        int
		Delivered   int64
		Dropped     uint64
		DeliveryPct float64
		Elapsed     time.Duration
	}
	rows := []result{}

	rates := []int{100, 500, 1_000, 2_000, 5_000, 10_000, 20_000} // writes per second targeted

	for _, rate := range rates {
		rate := rate
		t.Run(fmt.Sprintf("rate=%d/s", rate), func(t *testing.T) {
			h := NewHarness(t, HarnessOptions{})
			defer h.Close()

			sub, err := h.Peer().Subscribe("watched/*", entitysdk.SubscribeOpts{})
			if err != nil {
				t.Fatalf("subscribe: %v", err)
			}
			defer sub.Close()

			var delivered atomic.Int64
			done := make(chan struct{})
			go func() {
				for range sub.Events() {
					delivered.Add(1)
				}
				close(done)
			}()

			// Run for 3 seconds at the target rate.
			const wallTime = 3 * time.Second
			interval := time.Second / time.Duration(rate)
			sent := 0
			storeAPI := h.Peer().Store()
			deadline := time.Now().Add(wallTime)
			tick := time.NewTicker(interval)
			defer tick.Stop()

		writeLoop:
			for {
				select {
				case <-tick.C:
					if time.Now().After(deadline) {
						break writeLoop
					}
					path := fmt.Sprintf("watched/%07d", sent)
					if _, err := storeAPI.Put(path, "perfreview/entity",
						map[string]interface{}{"tick": sent}); err != nil {
						t.Fatalf("Put: %v", err)
					}
					sent++
				default:
					if time.Now().After(deadline) {
						break writeLoop
					}
				}
			}

			// Allow delivery to drain.
			time.Sleep(2 * time.Second)

			rows = append(rows, result{
				Rate:        rate,
				Sent:        sent,
				Delivered:   delivered.Load(),
				DeliveryPct: 100 * float64(delivered.Load()) / float64(sent),
				Elapsed:     wallTime,
			})
		})
	}

	// Final summary table.
	t.Logf("\n%-12s %10s %12s %12s", "target", "sent", "delivered", "delivery-pct")
	for _, r := range rows {
		t.Logf("%-12s %10d %12d %12.1f%%",
			fmt.Sprintf("%d/s", r.Rate), r.Sent, r.Delivered, r.DeliveryPct)
	}
}
