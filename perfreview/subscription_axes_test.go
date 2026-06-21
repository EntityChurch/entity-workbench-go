//go:build perfreview

package perfreview

import (
	"fmt"
	"runtime"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"entity-workbench-go/entitysdk"
)

// TestSubscription_PerInstalledCost characterizes subscription's actual
// scaling dimension: cost per INSTALLED subscription (regardless of
// whether anyone writes to the subscribed path).
//
// Method: install N subscriptions, no writes. Measure heap delta from
// peer-bootstrap. This isolates "what does each subscription itself
// cost" from "what does the engine bootstrap cost."
//
// Corrects the prior naive disable-matrix finding ("subscription costs
// 14 MiB heap") — that 14 MiB is the engine's preallocated 65K-entry
// delivery queue, NOT per-subscription cost. See
// ext/subscription/engine.go:88-93 for the constant.
func TestSubscription_PerInstalledCost(t *testing.T) {
	h := NewHarness(t, HarnessOptions{})
	defer h.Close()

	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	heapBefore := ms.HeapInuse
	t.Logf("post-bootstrap heap (engine queue included): %.1f MiB", float64(heapBefore)/1024/1024)

	type result struct {
		N         int
		HeapMiB   float64
		DeltaKiB  float64
		PerSubKiB float64
	}
	rows := []result{}

	subs := make([]*entitysdk.Subscription, 0, 1000)
	defer func() {
		for _, s := range subs {
			s.Close()
		}
	}()

	for _, n := range []int{10, 100, 500, 1000} {
		for len(subs) < n {
			pattern := fmt.Sprintf("watched/%05d/*", len(subs))
			s, err := h.Peer().Subscribe(pattern, entitysdk.SubscribeOpts{})
			if err != nil {
				t.Fatalf("Subscribe(%d): %v", len(subs), err)
			}
			subs = append(subs, s)
		}
		runtime.GC()
		runtime.ReadMemStats(&ms)
		delta := int64(ms.HeapInuse) - int64(heapBefore)
		rows = append(rows, result{
			N:         n,
			HeapMiB:   float64(ms.HeapInuse) / 1024 / 1024,
			DeltaKiB:  float64(delta) / 1024,
			PerSubKiB: float64(delta) / 1024 / float64(n),
		})
	}

	t.Logf("\n%-8s %12s %12s %14s", "N-subs", "heap-MiB", "delta-KiB", "per-sub-KiB")
	for _, r := range rows {
		t.Logf("%-8d %12.1f %12.0f %14.2f", r.N, r.HeapMiB, r.DeltaKiB, r.PerSubKiB)
	}
}

// TestSubscription_DispatchCostWhenSubscribed characterizes the OTHER
// subscription axis: per-write cost when the written path is matched
// by N installed subscriptions. Each Put triggers fanout to all
// matching subscribers.
//
// Method: install N subscriptions on a prefix, drive M writes to that
// prefix, measure per-Put latency at the writer side. Also count the
// notification deliveries received.
func TestSubscription_DispatchCostWhenSubscribed(t *testing.T) {
	for _, nSubs := range []int{0, 1, 10, 100} {
		nSubs := nSubs
		t.Run(fmt.Sprintf("subs=%d", nSubs), func(t *testing.T) {
			h := NewHarness(t, HarnessOptions{})
			defer h.Close()

			var delivered atomic.Int64
			subs := make([]*entitysdk.Subscription, 0, nSubs)
			defer func() {
				for _, s := range subs {
					s.Close()
				}
			}()

			for i := 0; i < nSubs; i++ {
				s, err := h.Peer().Subscribe("watched/*", entitysdk.SubscribeOpts{})
				if err != nil {
					t.Fatalf("subscribe %d: %v", i, err)
				}
				subs = append(subs, s)
				// Drain the events chan in a goroutine to avoid back-pressure.
				go func(sub *entitysdk.Subscription) {
					for range sub.Events() {
						delivered.Add(1)
					}
				}(s)
			}

			const N = 5_000
			storeAPI := h.Peer().Store()
			latencies := make([]time.Duration, 0, N)
			t0 := time.Now()
			for i := 0; i < N; i++ {
				path := fmt.Sprintf("watched/%07d", i)
				start := time.Now()
				if _, err := storeAPI.Put(path, "perfreview/entity",
					map[string]interface{}{"tick": i, "time": "x"}); err != nil {
					t.Fatalf("Put: %v", err)
				}
				latencies = append(latencies, time.Since(start))
			}
			writeDur := time.Since(t0)

			sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
			p50 := latencies[len(latencies)*50/100]
			p99 := latencies[len(latencies)*99/100]

			// Give async deliveries a moment.
			time.Sleep(200 * time.Millisecond)

			t.Logf("subs=%d writes=%d dur=%s p50=%s p99=%s delivered=%d (expected up to %d)",
				nSubs, N, short(writeDur), short(p50), short(p99),
				delivered.Load(), int64(nSubs)*int64(N))
		})
	}
}
