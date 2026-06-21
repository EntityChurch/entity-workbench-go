//go:build perfreview

package perfreview

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

// TestContinuation_InstallCost measures the cost of installing N
// continuations. Each install puts a continuation entity into the
// store (the continuation is "suspended" until something fires it via
// dispatch).
//
// Method: install N forward continuations, measure per-install latency
// and heap growth.
func TestContinuation_InstallCost(t *testing.T) {
	h := NewHarness(t, HarnessOptions{})
	defer h.Close()

	ctx := context.Background()
	cc := h.Peer().Continuation()

	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	heapBefore := ms.HeapInuse

	const N = 500
	latencies := make([]time.Duration, 0, N)
	t0 := time.Now()
	for i := 0; i < N; i++ {
		installPath := fmt.Sprintf("system/inbox/perfreview-cont-%05d", i)
		ownerCap := h.Peer().OwnerCapability()
		cont := types.ContinuationData{
			Target:             "system/clock",
			Operation:          "now",
			DispatchCapability: ownerCap.ContentHash,
		}
		contEnt, err := cont.ToEntity()
		if err != nil {
			t.Fatalf("ToEntity: %v", err)
		}
		start := time.Now()
		if _, err := cc.Install(ctx, installPath, contEnt); err != nil {
			t.Fatalf("install %d: %v", i, err)
		}
		latencies = append(latencies, time.Since(start))
	}
	totalDur := time.Since(t0)

	runtime.GC()
	runtime.ReadMemStats(&ms)
	heapAfter := ms.HeapInuse

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	t.Logf("install %d continuations: total=%s p50=%s p95=%s p99=%s heap+%.1fKiB (%.0fB/cont)",
		N, short(totalDur),
		short(latencies[len(latencies)*50/100]),
		short(latencies[len(latencies)*95/100]),
		short(latencies[len(latencies)*99/100]),
		float64(int64(heapAfter)-int64(heapBefore))/1024,
		float64(int64(heapAfter)-int64(heapBefore))/float64(N))

	m := h.Snapshot("post-install", 0, 0, 0, 0, 0)
	t.Logf("post-install state: entities=%d locs=%d (per cont: ents=%.1f locs=%.1f)",
		m.EntityCount, m.LocationCount,
		float64(m.EntityCount-244)/float64(N), float64(m.LocationCount-271)/float64(N))
}

// TestContinuation_DispatchLatency is intentionally a baseline-only
// measurement — dispatching system/clock 'now' directly (no chain in
// the path). The "true" continuation-dispatch measurement needs an op
// that ENDS with a result-transform pointing at the continuation
// chain, which is more setup than makes sense here. See
// entitysdk/continuation_observe.go for the full chain shape.
//
// This baseline gives the "what's the cheapest possible dispatch
// through the executor" number for comparison against compute
// dispatch + cross-peer dispatch.
func TestContinuation_DispatchLatency(t *testing.T) {
	h := NewHarness(t, HarnessOptions{})
	defer h.Close()

	ctx := context.Background()
	ap := h.Peer()

	_ = ctx
	const N = 500
	latencies := make([]time.Duration, 0, N)
	// Trigger the continuation by writing to its inbox path with a
	// signal. The clock op runs and the chain resumes.
	params, _ := entitysdk.PrimitiveAny(map[string]interface{}{})
	for i := 0; i < N; i++ {
		start := time.Now()
		if _, err := ap.Executor().ExecuteWithParams("system/clock", "now", params); err != nil {
			t.Fatalf("dispatch %d: %v", i, err)
		}
		latencies = append(latencies, time.Since(start))
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	t.Logf("clock 'now' dispatch (no continuation in path) (N=%d): p50=%s p95=%s p99=%s",
		N,
		short(latencies[len(latencies)*50/100]),
		short(latencies[len(latencies)*95/100]),
		short(latencies[len(latencies)*99/100]))
}
