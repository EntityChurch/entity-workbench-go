//go:build perfreview

package perfreview

import (
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"entity-workbench-go/entitysdk"
)

// TestSubscription_DeliverLatency measures the time per Deliver call
// by writing one entity at a time, waiting for the delivery, and
// repeating. Bypasses queue effects to characterize per-delivery cost.
//
// Findings drive theories on why ~2K/sec is the ceiling.
func TestSubscription_DeliverLatency(t *testing.T) {
	h := NewHarness(t, HarnessOptions{})
	defer h.Close()

	sub, err := h.Peer().Subscribe("watched/*", entitysdk.SubscribeOpts{})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Close()

	var delivered atomic.Int64
	ready := make(chan time.Time, 1024)
	done := make(chan struct{})
	go func() {
		for range sub.Events() {
			delivered.Add(1)
			select {
			case ready <- time.Now():
			default:
			}
		}
		close(done)
	}()

	const N = 200
	storeAPI := h.Peer().Store()

	// Warm up.
	for i := 0; i < 5; i++ {
		_, _ = storeAPI.Put("warmup/x", "perfreview/entity", map[string]interface{}{"i": i})
	}
	// Drain warmup deliveries (they don't match watched/*).
	time.Sleep(50 * time.Millisecond)

	endToEnd := make([]time.Duration, 0, N)
	putLatency := make([]time.Duration, 0, N)
	for i := 0; i < N; i++ {
		path := "watched/" + intStr(i)
		t0 := time.Now()
		if _, err := storeAPI.Put(path, "perfreview/entity",
			map[string]interface{}{"tick": i}); err != nil {
			t.Fatalf("Put: %v", err)
		}
		putElapsed := time.Since(t0)
		select {
		case deliverT := <-ready:
			endToEnd = append(endToEnd, deliverT.Sub(t0))
			putLatency = append(putLatency, putElapsed)
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("delivery timeout at i=%d", i)
		}
	}

	sort.Slice(endToEnd, func(i, j int) bool { return endToEnd[i] < endToEnd[j] })
	sort.Slice(putLatency, func(i, j int) bool { return putLatency[i] < putLatency[j] })

	t.Logf("Put latency           (N=%d): p50=%s p95=%s p99=%s",
		N,
		short(putLatency[len(putLatency)*50/100]),
		short(putLatency[len(putLatency)*95/100]),
		short(putLatency[len(putLatency)*99/100]))
	t.Logf("Put → delivery (e2e)  (N=%d): p50=%s p95=%s p99=%s",
		N,
		short(endToEnd[len(endToEnd)*50/100]),
		short(endToEnd[len(endToEnd)*95/100]),
		short(endToEnd[len(endToEnd)*99/100]))
}

func intStr(i int) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	buf := make([]byte, 0, 8)
	for i > 0 {
		buf = append([]byte{digits[i%10]}, buf...)
		i /= 10
	}
	return string(buf)
}
