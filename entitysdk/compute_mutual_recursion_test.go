package entitysdk_test

// Gate-1 probe for the native-compute peer step D: path-mediated
// mutual recursion across two compute handlers.
//
// Arch's `EXPLORATION-STAGE-0-FLOOR-AND-GENOME-BOOTSTRAP.md §4 gate 1`
// posed the question:
//
//   "are mutual references (handler A's body calls B, B's calls A)
//   permitted, and if so how are they bootstrapped? Content-addressing
//   forbids a literal cycle in the *hash* graph (a hash can't contain
//   itself), so mutual reference must go through a *path*
//   (`lookup/tree`) indirection — which means the two handlers register
//   independently and resolve each other at dispatch, not at load.
//   Tentative: path-mediated mutual reference is fine (register both,
//   resolve at eval); hash-mediated cycles are impossible by construction.
//   Worth confirming against a worked two-handler-mutual-recursion genome."
//
// This test is that worked confirmation. Two compute lambdas — isEven
// and isOdd — call each other via path-mediated LookupTree. The
// path resolution happens at eval time, so build-order doesn't matter:
// we can build isEven first (whose body references isOdd's path, which
// doesn't yet exist) and isOdd second; both resolve at the first
// dispatch.
//
// Pattern: classical Scheme-style mutual recursion.
//
//	isEven(n) = if n == 0 then 1 else isOdd(n - 1)
//	isOdd(n)  = if n == 0 then 0 else isEven(n - 1)
//
// (Returns 1/0 instead of true/false since compute deals in numeric
// values; treat 1 as "even", 0 as "odd".)
//
// What this proves:
//   - Path-mediated mutual recursion works exactly as arch hypothesized.
//   - Build order doesn't matter for path-resolved references: a lambda
//     can be built referencing a path that won't be populated until
//     after it. Eval-time resolution closes the gap.
//   - The bootstrap's "topological-order DAG" load model (per §3 step 3
//     of the bootstrap) is not the only viable order — at the IR/handler
//     layer, path-mediated mutual reference admits parallel/either-order
//     loading too. The DAG order is required only for HASH-mediated
//     references (compute/apply with `fn` = LookupHash of a stored
//     closure); path-mediated references float.
//   - The §6.9 native bootstrap of `system/handler` is what's load-bearing:
//     once system/handler is up, BOTH compute handlers can register and
//     mutually reference even though one ref will be unresolvable at
//     register-time. That ordering invariant generalizes from
//     "bootstrap can't register itself" to "mutual handlers register
//     independently."

import (
	"context"
	"testing"

	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

// TestMutualRecursion_PathMediated_EvenOdd — isEven and isOdd reference
// each other via LookupTree(partner-path). Confirms arch gate-1's
// tentative claim concretely with shipped workbench primitives.
func TestMutualRecursion_PathMediated_EvenOdd(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	c := ap.Compute()

	// Lambda paths — chosen before building either lambda so each body
	// can LookupTree the partner.
	const isEvenPath = "app/mutual-rec/is-even/lambda"
	const isOddPath = "app/mutual-rec/is-odd/lambda"

	// --- Build isEven's lambda ---------------------------------------
	// λ(n) -> if n == 0 then 1 else ApplyClosure(LookupTree(isOddPath), {n: n-1})
	//
	// At build time, isOddPath has NO entity bound to it. That's fine —
	// LookupTree is resolved at EVAL time, not build time. The build
	// only stores the LookupTree IR node (which contains the path
	// string); the resolution happens during evaluate().
	isEvenN := c.LookupScope("n")
	isEvenBody := c.If(
		c.Compare("eq", isEvenN, c.Literal(uint64(0))),
		c.Literal(uint64(1)),
		c.ApplyClosure(
			c.LookupTree(isOddPath, false),
			map[string]*entitysdk.Builder{
				"n": c.Arithmetic("sub", isEvenN, c.Literal(uint64(1))),
			},
		),
	)
	isEvenLambda := c.Lambda([]string{"n"}, isEvenBody)
	if _, err := isEvenLambda.Build(context.Background(), isEvenPath); err != nil {
		t.Fatalf("build isEven: %v", err)
	}

	// --- Build isOdd's lambda ----------------------------------------
	// Symmetric to isEven. Note: at THIS build moment, isEven's path IS
	// populated — but that's incidental; the build doesn't actually
	// resolve through LookupTree even if it could.
	isOddN := c.LookupScope("n")
	isOddBody := c.If(
		c.Compare("eq", isOddN, c.Literal(uint64(0))),
		c.Literal(uint64(0)),
		c.ApplyClosure(
			c.LookupTree(isEvenPath, false),
			map[string]*entitysdk.Builder{
				"n": c.Arithmetic("sub", isOddN, c.Literal(uint64(1))),
			},
		),
	)
	isOddLambda := c.Lambda([]string{"n"}, isOddBody)
	if _, err := isOddLambda.Build(context.Background(), isOddPath); err != nil {
		t.Fatalf("build isOdd: %v", err)
	}

	// --- Register isEven as a dispatchable handler --------------------
	// Wrapper expression: pull n from scope.params, invoke isEven.
	const isEvenPattern = "app/mutual-rec/is-even"
	wrapper := c.ApplyClosure(
		c.LookupTree(isEvenPath, false),
		map[string]*entitysdk.Builder{
			"n": c.Field(c.LookupScope("params"), "n"),
		},
	)
	h, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: isEvenPattern,
		Name:    "is-even",
		Operations: map[string]types.HandlerOperationSpec{
			"compute": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, wrapper)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	// --- Dispatch + verify -------------------------------------------
	// is-even(0) = 1, is-even(1) = 0, is-even(4) = 1, is-even(7) = 0.
	cases := []struct {
		n       uint64
		isEven  uint64 // expected isEven result: 1 if even, 0 if odd
		comment string
	}{
		{0, 1, "0 is even (base case isEven)"},
		{1, 0, "1 is odd (isEven → isOdd → base case)"},
		{2, 1, "2 is even (isEven → isOdd → isEven → base)"},
		{4, 1, "4 is even (4 mutual-rec hops)"},
		{7, 0, "7 is odd (7 mutual-rec hops)"},
		{10, 1, "10 is even (10 hops — non-trivial mutual-rec depth)"},
	}
	for _, tc := range cases {
		params, _ := entitysdk.PrimitiveAny(map[string]interface{}{"n": tc.n})
		resp, err := ap.Executor().ExecuteWithParams(isEvenPattern, "compute", params)
		if err != nil {
			t.Fatalf("dispatch n=%d: %v", tc.n, err)
		}
		if resp.Status != 200 {
			t.Fatalf("n=%d status %d (type=%s, data=%s) — mutual-rec failed",
				tc.n, resp.Status, resp.Type, string(respData(resp)))
		}
		got := decodeBare(resp.Data)
		if !numEq(got, float64(tc.isEven)) {
			t.Errorf("isEven(%d) = %v, want %d (%s)", tc.n, got, tc.isEven, tc.comment)
		}
	}
	t.Logf("PASS mutual-rec gate 1: isEven/isOdd resolve each other via path-mediated LookupTree; mutual recursion at depths 0..10 returns correct parity")
	t.Logf("CONFIRMS arch tentative answer (STAGE-0-FLOOR-AND-GENOME-BOOTSTRAP §4 gate 1): path-mediated mutual reference is fine; build/register order doesn't matter because LookupTree resolves at eval time")
}

// TestMutualRecursion_BuildOrderIndependence — re-runs the same shape
// with REVERSED build order to make the build-order-independence claim
// explicit. isOdd is built first (referencing an as-yet-nonexistent
// isEvenPath), then isEven.
func TestMutualRecursion_BuildOrderIndependence(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	c := ap.Compute()
	const isEvenPath = "app/mutual-rec-rev/is-even/lambda"
	const isOddPath = "app/mutual-rec-rev/is-odd/lambda"

	// Build isOdd FIRST — isEvenPath doesn't exist yet at this moment.
	isOddN := c.LookupScope("n")
	isOddBody := c.If(
		c.Compare("eq", isOddN, c.Literal(uint64(0))),
		c.Literal(uint64(0)),
		c.ApplyClosure(
			c.LookupTree(isEvenPath, false),
			map[string]*entitysdk.Builder{
				"n": c.Arithmetic("sub", isOddN, c.Literal(uint64(1))),
			},
		),
	)
	isOddLambda := c.Lambda([]string{"n"}, isOddBody)
	if _, err := isOddLambda.Build(context.Background(), isOddPath); err != nil {
		t.Fatalf("build isOdd (reversed order): %v", err)
	}

	// Now isEven (which references the already-built isOdd path — but
	// the symmetric "isOdd built first" pre-state is the load-bearing
	// claim).
	isEvenN := c.LookupScope("n")
	isEvenBody := c.If(
		c.Compare("eq", isEvenN, c.Literal(uint64(0))),
		c.Literal(uint64(1)),
		c.ApplyClosure(
			c.LookupTree(isOddPath, false),
			map[string]*entitysdk.Builder{
				"n": c.Arithmetic("sub", isEvenN, c.Literal(uint64(1))),
			},
		),
	)
	isEvenLambda := c.Lambda([]string{"n"}, isEvenBody)
	if _, err := isEvenLambda.Build(context.Background(), isEvenPath); err != nil {
		t.Fatalf("build isEven (reversed order): %v", err)
	}

	// Register isEven (the dispatch entry). Same wrapper shape.
	const isEvenPattern = "app/mutual-rec-rev/is-even"
	wrapper := c.ApplyClosure(
		c.LookupTree(isEvenPath, false),
		map[string]*entitysdk.Builder{
			"n": c.Field(c.LookupScope("params"), "n"),
		},
	)
	h, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: isEvenPattern,
		Name:    "is-even-rev",
		Operations: map[string]types.HandlerOperationSpec{
			"compute": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, wrapper)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	// Single dispatch — the reversed-order proof point.
	params, _ := entitysdk.PrimitiveAny(map[string]interface{}{"n": uint64(5)})
	resp, err := ap.Executor().ExecuteWithParams(isEvenPattern, "compute", params)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if resp.Status != 200 {
		t.Fatalf("status %d — reversed build order broke mutual-rec resolution", resp.Status)
	}
	got := decodeBare(resp.Data)
	if !numEq(got, float64(0)) {
		t.Errorf("isEven(5) = %v, want 0 (5 is odd)", got)
	}
	t.Logf("PASS reversed-build-order: isOdd built first (referencing non-existent isEvenPath) then isEven; both resolve at dispatch time")
}
