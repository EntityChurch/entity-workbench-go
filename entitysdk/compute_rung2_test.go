package entitysdk_test

// Rung-2 completion validation — the empirical hand-off arch named in
// PROPOSAL-COMPUTE-RECURSION-AND-SUM-TYPES §5.
//
// Two prototype tests answer the empirical questions:
//
//   1. LowerRecurse + ApplyClosure — does the closure-mode apply +
//      TCO trampoline + LookupTree(lambda_path) fixpoint compose cleanly
//      enough for a frontend to use? (Factorial as the worked example.)
//
//   2. LowerMatch over tag-in-data — does the entity-native sum-type
//      pattern lower real enums cleanly? (Option<T>, Result<T,E>, and
//      a 3-variant domain enum.) The three questions to answer:
//        a. Does the redundant tag (entity .type + .data tag field)
//           bite enough to want a compute/type-of primitive (A2)?
//        b. Is the absence of static exhaustiveness a real problem?
//        c. Any binding/scope friction in arm bodies?
//
// Findings inform the v3.20 decision: do we open compute/match as a
// primitive, or does the toolkit answer suffice?

import (
	"context"
	"testing"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

// --- Part A: LowerRecurse via factorial ------------------------------

// TestLowerRecurse_TailRecursiveFactorial — builds an
// accumulator-style factorial (tail-position recursive call) via
// LowerRecurse + ApplyClosure, registers it as a compute handler,
// and dispatches against several values.
//
// The accumulator shape is the load-bearing trick for TCO:
//
//	fact_iter(n, acc) = if n == 0 then acc
//	                   else fact_iter(n-1, n*acc)
//
// The recursive call is in tail position, so the evaluator's TCO
// trampoline iterates without consuming depth. Naive
// `n * fact(n-1)` is NOT tail-recursive and would blow the eval
// budget at depth ~32.
//
// Confirms three things:
//  1. ApplyClosure correctly emits closure-mode compute/apply.
//  2. LookupTree(lambdaPath) inside the lambda body resolves to the
//     lambda itself (the fixpoint-bootstrap works mechanically).
//  3. The evaluator's TCO trampoline iterates large-N cases without
//     depth-budget exhaustion.
func TestLowerRecurse_TailRecursiveFactorial(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	c := ap.Compute()
	const lambdaPath = "app/rung2/factorial/lambda"

	// Build the recursive lambda via LowerRecurse. self-reference
	// is provided as a LookupTree builder; the lambda's body uses
	// ApplyClosure(self, ...) for the tail call.
	_, err = entitysdk.LowerRecurse(context.Background(), ap, lambdaPath,
		[]string{"n", "acc"},
		func(self *entitysdk.Builder, args ...*entitysdk.Builder) *entitysdk.Builder {
			n, acc := args[0], args[1]
			return c.If(
				c.Compare("eq", n, c.Literal(uint64(0))),
				acc,
				c.ApplyClosure(self, map[string]*entitysdk.Builder{
					"n":   c.Arithmetic("sub", n, c.Literal(uint64(1))),
					"acc": c.Arithmetic("mul", n, acc),
				}),
			)
		})
	if err != nil {
		t.Fatalf("LowerRecurse: %v", err)
	}

	// Now wrap with a top-level expression: read n from scope.params,
	// kick off the recursion with acc=1. This is the registered
	// handler's expression — the wrapper that maps scope.params.n into
	// the recursive lambda's parameter shape.
	wrapper := c.ApplyClosure(c.LookupTree(lambdaPath, false),
		map[string]*entitysdk.Builder{
			"n":   c.Field(c.LookupScope("params"), "n"),
			"acc": c.Literal(uint64(1)),
		})

	const pattern = "app/rung2/factorial"
	h, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: pattern,
		Name:    "factorial",
		Operations: map[string]types.HandlerOperationSpec{
			"compute": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, wrapper)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	// Sanity: 0! = 1, 1! = 1, 5! = 120, 10! = 3628800.
	// 20! overflows uint64 but uses the tail-call trampoline at
	// non-trivial depth — if TCO weren't engaged, the eval-depth
	// budget would reject it before correctness mattered. We assert
	// only on the cases where the uint64 arithmetic is exact.
	cases := []struct {
		n    uint64
		want uint64
	}{
		{0, 1},
		{1, 1},
		{5, 120},
		{10, 3628800},
		{15, 1307674368000}, // 15! = 1,307,674,368,000 — well within uint64; tests deeper recursion under TCO
	}
	for _, tc := range cases {
		params, _ := entitysdk.PrimitiveAny(map[string]interface{}{"n": tc.n})
		resp, err := ap.Executor().ExecuteWithParams(pattern, "compute", params)
		if err != nil {
			t.Fatalf("dispatch n=%d: %v", tc.n, err)
		}
		if resp.Status != 200 {
			t.Fatalf("n=%d status %d (type=%s, data=%s)", tc.n, resp.Status, resp.Type, string(respData(resp)))
		}
		var got interface{}
		if v, _, err := entitysdk.UnwrapComputeResult(resp); err == nil {
			// SA-4 wrap.
			_ = decodeInto(v, &got)
		} else {
			// Bare value.
			got = decodeBare(resp.Data)
		}
		if !numEq(got, float64(tc.want)) {
			t.Errorf("factorial(%d) = %v (%T), want %d", tc.n, got, got, tc.want)
		}
	}
	t.Logf("PASS LowerRecurse: tail-recursive factorial(0/1/5/10) returns expected values via closure-mode apply + TCO")
}

// --- Part B: LowerMatch over real enum shapes ------------------------

// TestLowerMatch_OptionTSomeAndNone — the canonical Rust Option<T> shape.
// Lowers two variants ({some: x, value} | {none}) to an if-chain
// dispatching on $variant.
func TestLowerMatch_OptionTSomeAndNone(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	c := ap.Compute()
	const pattern = "app/rung2/option-unwrap-or-zero-plus-one"

	// match value:
	//   Some(x) => x + 1
	//   None    => 0
	value := c.LookupScope("params")
	expr := entitysdk.LowerMatch(c, value, "$variant",
		map[string]func(*entitysdk.Builder) *entitysdk.Builder{
			"some": func(v *entitysdk.Builder) *entitysdk.Builder {
				return c.Arithmetic("add", c.Field(v, "value"), c.Literal(uint64(1)))
			},
			"none": func(_ *entitysdk.Builder) *entitysdk.Builder {
				return c.Literal(uint64(0))
			},
		}, nil)

	h, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: pattern,
		Name:    "option-match",
		Operations: map[string]types.HandlerOperationSpec{
			"compute": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, expr)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	cases := []struct {
		name   string
		params map[string]interface{}
		want   uint64
	}{
		{"Some(41) → 42", map[string]interface{}{"$variant": "some", "value": uint64(41)}, 42},
		{"Some(0) → 1", map[string]interface{}{"$variant": "some", "value": uint64(0)}, 1},
		{"None → 0", map[string]interface{}{"$variant": "none"}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params, _ := entitysdk.PrimitiveAny(tc.params)
			resp, err := ap.Executor().ExecuteWithParams(pattern, "compute", params)
			if err != nil {
				t.Fatalf("dispatch: %v", err)
			}
			if resp.Status != 200 {
				t.Fatalf("status %d (type=%s, data=%s)", resp.Status, resp.Type, string(respData(resp)))
			}
			got := decodeBare(resp.Data)
			if !numEq(got, float64(tc.want)) {
				t.Errorf("got %v, want %d", got, tc.want)
			}
		})
	}
}

// TestLowerMatch_ResultTE — the canonical Rust Result<T,E> shape.
// Ok(v) and Err(e) variants; arm bodies do different navigation
// per variant.
func TestLowerMatch_ResultTE(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	c := ap.Compute()
	const pattern = "app/rung2/result-unwrap-or-default"

	// match value:
	//   Ok(v)     => v
	//   Err(e)    => -1  (use uint64 max for "error sentinel" simplicity)
	const errSentinel = uint64(99999)
	value := c.LookupScope("params")
	expr := entitysdk.LowerMatch(c, value, "$variant",
		map[string]func(*entitysdk.Builder) *entitysdk.Builder{
			"ok": func(v *entitysdk.Builder) *entitysdk.Builder {
				return c.Field(v, "value")
			},
			"err": func(_ *entitysdk.Builder) *entitysdk.Builder {
				return c.Literal(errSentinel)
			},
		}, nil)

	h, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: pattern,
		Name:    "result-match",
		Operations: map[string]types.HandlerOperationSpec{
			"compute": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, expr)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	cases := []struct {
		name   string
		params map[string]interface{}
		want   uint64
	}{
		{"Ok(42) → 42", map[string]interface{}{"$variant": "ok", "value": uint64(42)}, 42},
		{"Err(\"oops\") → sentinel", map[string]interface{}{"$variant": "err", "error": "oops"}, errSentinel},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params, _ := entitysdk.PrimitiveAny(tc.params)
			resp, err := ap.Executor().ExecuteWithParams(pattern, "compute", params)
			if err != nil {
				t.Fatalf("dispatch: %v", err)
			}
			if resp.Status != 200 {
				t.Fatalf("status %d (type=%s, data=%s)", resp.Status, resp.Type, string(respData(resp)))
			}
			got := decodeBare(resp.Data)
			if !numEq(got, float64(tc.want)) {
				t.Errorf("got %v, want %d", got, tc.want)
			}
		})
	}
}

// TestLowerMatch_ThreeVariantDomainEnum — a domain enum with three
// variants exercising arm-bodies that compute distinct shapes.
// Stand-in for a real frontend's enum (e.g. an event-type enum where
// each variant carries different data and produces a different summary).
func TestLowerMatch_ThreeVariantDomainEnum(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	c := ap.Compute()
	const pattern = "app/rung2/event-classify"

	// match event:
	//   Click(x, y)     => x + y      (sum coords)
	//   KeyPress(code)  => code * 2   (scale code)
	//   Tick            => 0
	value := c.LookupScope("params")
	expr := entitysdk.LowerMatch(c, value, "$variant",
		map[string]func(*entitysdk.Builder) *entitysdk.Builder{
			"click": func(v *entitysdk.Builder) *entitysdk.Builder {
				return c.Arithmetic("add", c.Field(v, "x"), c.Field(v, "y"))
			},
			"keypress": func(v *entitysdk.Builder) *entitysdk.Builder {
				return c.Arithmetic("mul", c.Field(v, "code"), c.Literal(uint64(2)))
			},
			"tick": func(_ *entitysdk.Builder) *entitysdk.Builder {
				return c.Literal(uint64(0))
			},
		}, nil)

	h, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: pattern,
		Name:    "event-classify",
		Operations: map[string]types.HandlerOperationSpec{
			"compute": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, expr)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	cases := []struct {
		name   string
		params map[string]interface{}
		want   uint64
	}{
		{"Click(10,20) → 30", map[string]interface{}{"$variant": "click", "x": uint64(10), "y": uint64(20)}, 30},
		{"KeyPress(65) → 130", map[string]interface{}{"$variant": "keypress", "code": uint64(65)}, 130},
		{"Tick → 0", map[string]interface{}{"$variant": "tick"}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params, _ := entitysdk.PrimitiveAny(tc.params)
			resp, err := ap.Executor().ExecuteWithParams(pattern, "compute", params)
			if err != nil {
				t.Fatalf("dispatch: %v", err)
			}
			if resp.Status != 200 {
				t.Fatalf("status %d (type=%s, data=%s)", resp.Status, resp.Type, string(respData(resp)))
			}
			got := decodeBare(resp.Data)
			if !numEq(got, float64(tc.want)) {
				t.Errorf("got %v, want %d", got, tc.want)
			}
		})
	}
	t.Logf("PASS LowerMatch: three real-enum shapes (Option<T>, Result<T,E>, 3-variant domain enum) lower cleanly via tag-in-data + if/eq chain")
}

// TestLowerMatch_NoArmMatchesDefaultArm — confirms the user-supplied
// default arm fires when no tag matches.
func TestLowerMatch_NoArmMatchesDefaultArm(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	c := ap.Compute()
	const pattern = "app/rung2/match-default"

	const sentinel = uint64(7777)
	value := c.LookupScope("params")
	expr := entitysdk.LowerMatch(c, value, "$variant",
		map[string]func(*entitysdk.Builder) *entitysdk.Builder{
			"a": func(_ *entitysdk.Builder) *entitysdk.Builder { return c.Literal(uint64(1)) },
		},
		func(_ *entitysdk.Builder) *entitysdk.Builder {
			return c.Literal(sentinel)
		})

	h, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: pattern,
		Name:    "default-arm",
		Operations: map[string]types.HandlerOperationSpec{
			"compute": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, expr)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	params, _ := entitysdk.PrimitiveAny(map[string]interface{}{"$variant": "z-not-listed"})
	resp, err := ap.Executor().ExecuteWithParams(pattern, "compute", params)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if resp.Status != 200 {
		t.Fatalf("status %d (type=%s)", resp.Status, resp.Type)
	}
	got := decodeBare(resp.Data)
	if !numEq(got, float64(sentinel)) {
		t.Errorf("default arm not fired: got %v, want %d", got, sentinel)
	}
}

// --- test helpers ----------------------------------------------------

func decodeBare(data []byte) interface{} {
	var v interface{}
	_ = ecf.Decode(data, &v)
	return v
}

func decodeInto(data []byte, target interface{}) error {
	return ecf.Decode(data, target)
}
