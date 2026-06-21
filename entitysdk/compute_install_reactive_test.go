package entitysdk_test

// Reactive compute install POC — `system/compute:install`.
//
// Goal: prove the reactive subgraph story end-to-end. compute:install
// registers an expression that reads from specific tree paths; when
// any of those paths change, the engine re-evaluates and writes the
// result to result_path. This is the compute primitive that makes
// "derived state in the tree" possible — a panel can subscribe to
// result_path and see live updates whenever the inputs change.
//
// What this POC proves:
//   - compute:install accepts an expression with LookupTree deps and
//     registers their paths for reactive triggering.
//   - When a dependency entity is mutated via tree:put, the engine's
//     OnTreeChange sync hook fires, reEvaluate runs, and the result
//     is written to result_path with the new value.
//   - The contract holds for our LowerArithmetic-built expression
//     (i.e., the H.2 toolkit composes through install just like it
//     composes through register).
//
// What it does NOT do (deferred):
//   - Install via a typed SDK helper (no InstallComputeSubgraph yet —
//     would-be S6 surface). This test dispatches system/compute:install
//     manually.
//   - Cascade tracking across nested installs.
//   - Cross-peer reactive installs.
//   - Subscribe to result_path via system/subscription (a panel-level
//     hookup) — for the POC we just Get the result entity directly.

import (
	"context"
	"testing"

	"entity-workbench-go/entitysdk"
)

// TestComputeInstall_ReactiveAggregateOverFixedPaths — install a
// subgraph that sums the `size` fields of three FileData entities at
// fixed paths. Initial install establishes dependencies; mutating a
// dep triggers a reactive re-eval and writes the new sum to result_path.
//
// Numeric checks (each mutation changes content-hash, triggering the
// sync hook):
//   - Initial files: a=100, b=200, c=300. Install. (No initial eval —
//     engine doesn't write result_path eagerly; first mutation drives it.)
//   - Mutate a: 100 → 150 → sum should be 650 (150+200+300).
//   - Mutate b: 200 → 250 → sum should be 700 (150+250+300).
//   - Mutate c: 300 → 500 → sum should be 900 (150+250+500).
func TestComputeInstall_ReactiveAggregateOverFixedPaths(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	// --- Step 1: put three FileData-shaped entities at fixed paths ----
	//
	// (Post-refactor: this test now exercises LookupTreeLocal + the
	// typed InstallComputeSubgraph helper — both shipped after the
	// arch team answered Q1 and Q2 in
	// FEEDBACK-COMPUTE-FOUNDATION-CLOSED. The path-qualification
	// dance the original test had to do by hand is now absorbed by
	// LookupTreeLocal; the manual `system/compute:install` dispatch
	// is now `ap.InstallComputeSubgraph`.)
	pathA := "app/reactive/files/a"
	pathB := "app/reactive/files/b"
	pathC := "app/reactive/files/c"
	putFile := func(path string, size uint64) {
		t.Helper()
		ent, err := entitysdk.PrimitiveAny(map[string]interface{}{
			"path": path,
			"size": size,
		})
		if err != nil {
			t.Fatalf("primitive/any %s: %v", path, err)
		}
		if _, err := ap.PutEntity(path, ent); err != nil {
			t.Fatalf("put %s: %v", path, err)
		}
	}
	putFile(pathA, 100)
	putFile(pathB, 200)
	putFile(pathC, 300)

	// --- Step 2: build a reactive expression using LookupTreeLocal ----
	// total = field(LookupTreeLocal(a), "size")
	//       + field(LookupTreeLocal(b), "size")
	//       + field(LookupTreeLocal(c), "size")
	//
	// LookupTreeLocal auto-qualifies the bare paths to /{peerID}/path,
	// matching the canonical path shape the store dispatches in
	// TreeChangeEvent.Path. The original test had to do `q := func(p)
	// string { return "/" + peerID + "/" + p }` by hand for every dep.
	c := ap.Compute()
	sizeOf := func(barePath string) *entitysdk.Builder {
		return c.Field(c.LookupTreeLocal(barePath), "size")
	}
	totalExpr := c.Arithmetic("add", sizeOf(pathA),
		c.Arithmetic("add", sizeOf(pathB), sizeOf(pathC)))

	// --- Step 3: install via the typed helper -------------------------
	// InstallComputeSubgraph builds the expression tree under a stable
	// per-result-path sub-path, dispatches system/compute:install
	// (audit + grant + dependency registration atomic), and returns a
	// typed SubgraphHandle. Manual `ExecuteOnResource("system/compute",
	// "install", ...)` machinery is gone.
	resultPath := "app/reactive/result/total-bytes"
	h, err := ap.InstallComputeSubgraph(context.Background(), totalExpr, resultPath)
	if err != nil {
		t.Fatalf("InstallComputeSubgraph: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	t.Logf("install ok: subgraph=%s result_path=%s", h.SubgraphPath(), h.ResultPath())
	t.Logf("install audit: impure_operations=%+v", h.ImpureOperations)

	// --- Step 4: mutate a (100 → 150) → reactive update ---------------
	// Install registers deps but doesn't eagerly evaluate (per engine.go
	// HandleInstall — no initial recompute; result_path stays empty
	// until OnTreeChange fires for a registered dep). The first mutation
	// to ANY dep triggers the engine to evaluate the full expression
	// (using current state of ALL deps), so this single mutation drives
	// the initial visible value.
	putFile(pathA, 150)

	// --- Step 5: read result via SubgraphHandle.ReadResult ------------
	// ReadResult peels the SA-4 envelope automatically — no manual
	// "is it bare value or compute/result-shaped" branching at the
	// call site.
	readSum := func() interface{} {
		t.Helper()
		v, found, err := h.ReadResult()
		if err != nil {
			t.Fatalf("ReadResult: %v", err)
		}
		if !found {
			t.Fatalf("no entity at result_path %s (re-eval didn't fire?)", resultPath)
		}
		return v
	}

	got := readSum()
	if !numEq(got, 650) {
		t.Fatalf("after a: 100→150: expected 650, got %v (%T)", got, got)
	}
	t.Logf("STAGE 1 PASS: install + mutate a 100→150 → result = %v (sum 150+200+300 = 650)", got)

	// --- Step 6: mutate b (200 → 250) → reactive update --------------
	putFile(pathB, 250)
	got = readSum()
	if !numEq(got, 700) {
		t.Fatalf("after b: 200→250: expected 700, got %v", got)
	}
	t.Logf("STAGE 2 PASS: mutate b 200→250 → result = %v (sum 150+250+300 = 700)", got)

	// --- Step 7: mutate c (300 → 500) → reactive update --------------
	putFile(pathC, 500)
	got = readSum()
	if !numEq(got, 900) {
		t.Fatalf("after c: 300→500: expected 900, got %v", got)
	}
	t.Logf("STAGE 3 PASS: mutate c 300→500 → result = %v (sum 150+250+500 = 900)", got)

	t.Logf("FULL POC PASS: compute:install reactive subgraph re-evaluates on dep mutation; result_path tracks the latest sum across three independent updates")
}
