//go:build perfreview

package perfreview

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

// TestComputeCost_RegisterAndDispatch measures register-handler and
// per-dispatch latency for compute-backed handlers. Compute is one of
// the heavier extensions per its design (IR evaluation per call); this
// gives operators a sense of what each dispatch through the compute
// path actually costs.
//
// Methodology:
//  1. Register a single compute handler with a moderate-complexity
//     expression (literal record with 3 fields).
//  2. Dispatch against it N times; measure per-dispatch latency.
//
// Investigation 14 of PRODUCTION-READINESS-REVIEW.
func TestComputeCost_RegisterAndDispatch(t *testing.T) {
	h := NewHarness(t, HarnessOptions{})
	defer h.Close()

	ap := h.Peer()
	ctx := context.Background()

	pattern := "perfreview/compute-handler"
	c := ap.Compute()
	expr := entitysdk.LowerRecord(c, "primitive/any", map[string]interface{}{
		"sum":   c.Literal(uint64(60)),
		"count": c.Literal(uint64(3)),
	})

	t0 := time.Now()
	handle, err := ap.RegisterComputeHandler(ctx, entitysdk.HandlerSpec{
		Pattern: pattern,
		Name:    "perfreview-compute",
		Operations: map[string]types.HandlerOperationSpec{
			"compute": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, expr)
	if err != nil {
		t.Fatalf("RegisterComputeHandler: %v", err)
	}
	defer handle.Close()
	registerDur := time.Since(t0)
	t.Logf("RegisterComputeHandler: %s", short(registerDur))

	const N = 1000
	params, _ := entitysdk.PrimitiveAny(map[string]interface{}{})
	latencies := make([]time.Duration, 0, N)
	for i := 0; i < N; i++ {
		start := time.Now()
		if _, err := ap.Executor().ExecuteWithParams(pattern, "compute", params); err != nil {
			t.Fatalf("dispatch: %v", err)
		}
		latencies = append(latencies, time.Since(start))
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	t.Logf("compute dispatch (N=%d): p50=%s p95=%s p99=%s",
		N,
		short(latencies[len(latencies)*50/100]),
		short(latencies[len(latencies)*95/100]),
		short(latencies[len(latencies)*99/100]))
}

// TestComputeCost_RegisterManyHandlers measures the cost of bulk-
// registering N compute handlers (each at its own path). Tells
// operators what it costs to set up a workspace with many reactive
// computations.
func TestComputeCost_RegisterManyHandlers(t *testing.T) {
	h := NewHarness(t, HarnessOptions{})
	defer h.Close()

	ap := h.Peer()
	ctx := context.Background()
	c := ap.Compute()
	expr := c.Literal(uint64(42))

	const N = 200
	latencies := make([]time.Duration, 0, N)
	handles := make([]*entitysdk.HandlerHandle, 0, N)
	t0 := time.Now()
	for i := 0; i < N; i++ {
		pattern := fmt.Sprintf("perfreview/handler-%03d", i)
		start := time.Now()
		handle, err := ap.RegisterComputeHandler(ctx, entitysdk.HandlerSpec{
			Pattern: pattern,
			Name:    fmt.Sprintf("h-%03d", i),
			Operations: map[string]types.HandlerOperationSpec{
				"compute": {InputType: "primitive/any", OutputType: "primitive/any"},
			},
		}, expr)
		if err != nil {
			t.Fatalf("register %d: %v", i, err)
		}
		latencies = append(latencies, time.Since(start))
		handles = append(handles, handle)
	}
	totalDur := time.Since(t0)
	defer func() {
		for _, h := range handles {
			h.Close()
		}
	}()

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	t.Logf("register %d compute handlers: total=%s p50=%s p95=%s p99=%s",
		N, short(totalDur),
		short(latencies[len(latencies)*50/100]),
		short(latencies[len(latencies)*95/100]),
		short(latencies[len(latencies)*99/100]))

	m := h.Snapshot("after-register", 0, 0, 0, 0, 0)
	t.Logf("after registering %d handlers: entities=%d locs=%d heap=%.1fMiB",
		N, m.EntityCount, m.LocationCount, float64(m.HeapInUseBytes)/1024/1024)
}
