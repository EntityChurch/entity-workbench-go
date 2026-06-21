package entitysdk_test

// Multi-step compute chain POC.
//
// Goal: demonstrate compute primitives composing INSIDE a continuation
// chain across multiple compute steps. ExpA_4 covered single-step
// (cont → compute → delivery); Scenario C covered same-expression
// composition (compute → Apply → compute). This test covers the
// gap between them: TWO distinct compute handlers wired in series
// through a continuation + routing trampoline, which is the realistic
// shape any "pipeline of derived aggregates" use case will hit.
//
// Chain shape:
//
//	┌──────────────┐        ┌─────────────────┐
//	│ Advance      │        │ compute1 (sum)  │
//	│ ContA  ──────┼───────►│ params.values   │
//	└──────────────┘        │  → uint64 sum   │
//	                        └────────┬────────┘
//	                                 │ DeliverTo
//	                                 ▼
//	                        ┌─────────────────┐
//	                        │ Trampoline      │
//	                        │ (Go handler)    │
//	                        │  - peel envelope│
//	                        │  - dispatch     │
//	                        │    compute2     │
//	                        └────────┬────────┘
//	                                 │ EXECUTE
//	                                 ▼
//	                        ┌─────────────────┐
//	                        │ compute2 (×2)   │
//	                        │ params.x        │
//	                        │  → x * 2        │
//	                        └────────┬────────┘
//	                                 │ trampoline forwards result
//	                                 ▼
//	                        ┌─────────────────┐
//	                        │ Observer        │
//	                        │ (test channel)  │
//	                        └─────────────────┘
//
// Why a trampoline (and not pure-continuation chaining)? The G1 finding
// from the re-grounding: ContinuationTransformData ops are
// {extract, select, resource_extract, target_extract, operation_extract}
// — pure projection. They can't do the path/string manipulation needed
// to forward a compute result into the next compute's params shape.
// The documented idiom (per the workbench's G1 closure addendum) is a
// one-step transform *handler* between chain steps. This test
// materializes that idiom: the trampoline IS the "transform handler"
// that routes data between compute1 and compute2.
//
// What this POC proves:
//   - compute handlers compose as steps inside a continuation chain.
//   - The transform-handler idiom works for compute → compute forwarding.
//   - The full chain (cont → compute → trampoline → compute → observer)
//     completes end-to-end with the expected derived value.
//
// What it does NOT do (deferred):
//   - Pure-continuation chaining without a trampoline — needs either
//     compute-as-transform-op (out of model per current spec) or
//     a richer DeliverySpec that carries Resource for direct
//     continuation-to-continuation advance (also not in current spec).
//   - Multi-peer (cross-peer) compute chain.

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/handler"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"

	"github.com/fxamacker/cbor/v2"
)

// TestMultiStepComputeChain — full end-to-end chain with two compute
// handlers wired via continuation + trampoline.
//
// Numeric check: values=[10,20,30] → compute1 sum=60 → compute2 ×2 → 120.
func TestMultiStepComputeChain(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	// --- Step 1: register compute1 (sum-over-values) -----------------
	const compute1Pattern = "app/multi-step/sum"
	c := ap.Compute()
	values := c.Field(c.LookupScope("params"), "values")
	sumExpr := entitysdk.LowerFold(c, values, c.Literal(uint64(0)),
		func(acc, elem *entitysdk.Builder) *entitysdk.Builder {
			return c.Arithmetic("add", acc, elem)
		})
	h1, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: compute1Pattern,
		Name:    "sum",
		Operations: map[string]types.HandlerOperationSpec{
			"compute": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, sumExpr)
	if err != nil {
		t.Fatalf("register compute1: %v", err)
	}
	t.Cleanup(func() { _ = h1.Close() })

	// --- Step 2: register compute2 (double-x) ------------------------
	const compute2Pattern = "app/multi-step/double"
	c2 := ap.Compute()
	doubleExpr := c2.Arithmetic("mul",
		c2.Field(c2.LookupScope("params"), "x"),
		c2.Literal(uint64(2)))
	h2, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: compute2Pattern,
		Name:    "double",
		Operations: map[string]types.HandlerOperationSpec{
			"compute": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, doubleExpr)
	if err != nil {
		t.Fatalf("register compute2: %v", err)
	}
	t.Cleanup(func() { _ = h2.Close() })

	// --- Step 3: observer (final result sink) ------------------------
	const observerPattern = "app/multi-step/observer"
	type finalResult struct {
		value  interface{}
		gotAt  time.Time
		envErr error
	}
	resultCh := make(chan finalResult, 1)
	obsHandle, err := ap.RegisterHandler(entitysdk.HandlerSpec{
		Pattern: observerPattern,
		Name:    "multi-step-observer",
		Operations: map[string]types.HandlerOperationSpec{
			// Observer receives the final compute2 result wrapped as
			// InboxDeliveryData (from the trampoline's deliver_to).
			"deliver": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, func(ctx context.Context, req *handler.Request) (*handler.Response, error) {
		// Observer also uses UnwrapChainStepDelivery — same shape as
		// the trampoline. The final compute2 result arrives as a
		// double-wrapped delivery; the helper peels both layers.
		d, err := entitysdk.UnwrapChainStepDelivery(req.Params)
		if err != nil {
			resultCh <- finalResult{envErr: err, gotAt: time.Now()}
			ack, _ := ecf.Encode(map[string]interface{}{"ack": false})
			ackEnt, _ := entity.NewEntity("primitive/any", cbor.RawMessage(ack))
			return &handler.Response{Status: 200, Result: ackEnt}, nil
		}
		resultCh <- finalResult{value: d.Value, gotAt: time.Now()}
		ack, _ := ecf.Encode(map[string]interface{}{"ack": true})
		ackEnt, _ := entity.NewEntity("primitive/any", cbor.RawMessage(ack))
		return &handler.Response{Status: 200, Result: ackEnt}, nil
	})
	if err != nil {
		t.Fatalf("register observer: %v", err)
	}
	t.Cleanup(func() { _ = obsHandle.Close() })

	// --- Step 4: trampoline (routes compute1 result into compute2) ----
	//
	// This is the "transform handler" idiom: a one-step handler in the
	// chain that does the string/shape work transform-ops can't (G1
	// finding). It peels the InboxDeliveryData envelope, extracts the
	// sum (compute1's result), shapes it as compute2's params, dispatches
	// compute2, and forwards compute2's result onward to the observer.
	const trampolinePattern = "app/multi-step/trampoline"
	var trampolineErr error
	var trampolineErrOnce sync.Once
	recordTrampolineErr := func(err error) {
		trampolineErrOnce.Do(func() { trampolineErr = err })
	}
	trampolineHandle, err := ap.RegisterHandler(entitysdk.HandlerSpec{
		Pattern: trampolinePattern,
		Name:    "multi-step-trampoline",
		Operations: map[string]types.HandlerOperationSpec{
			"deliver": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, func(ctx context.Context, req *handler.Request) (*handler.Response, error) {
		// The S7 helper (UnwrapChainStepDelivery) absorbs the double-wrap
		// envelope peel that used to live here. Chain-routing semantics
		// (peel InboxDeliveryData, peel entity envelope, return bare
		// inner value) are now in entitysdk — no impl-detail knowledge
		// required at the call site.
		d, err := entitysdk.UnwrapChainStepDelivery(req.Params)
		if err != nil {
			recordTrampolineErr(err)
			return &handler.Response{Status: 500}, err
		}
		if d.Status != 200 {
			recordTrampolineErr(NewStatusErr(d.Status, "compute1 non-200"))
			return &handler.Response{Status: 500}, nil
		}
		c2Params, err := entitysdk.PrimitiveAny(map[string]interface{}{"x": d.Value})
		if err != nil {
			recordTrampolineErr(err)
			return &handler.Response{Status: 500}, err
		}

		// Dispatch compute2 with the assembled params.
		c2Resp, err := ap.Executor().ExecuteWithParams(compute2Pattern, "compute", c2Params)
		if err != nil {
			recordTrampolineErr(err)
			return &handler.Response{Status: 500}, err
		}
		if c2Resp.Status != 200 {
			recordTrampolineErr(NewStatusErr(c2Resp.Status, "compute2 non-200"))
			return &handler.Response{Status: 500}, nil
		}

		// Forward compute2's result onward as a fresh delivery envelope
		// to the observer. (The dispatcher's deliver_to mechanism wraps
		// auto when a continuation step's DeliverTo fires; here we are
		// the trampoline, so we hand-wrap.)
		forwardDelivery := types.InboxDeliveryData{
			OriginalRequestID: d.OriginalRequestID,
			Status:            c2Resp.Status,
			Result:            c2Resp.Data,
		}
		fwdEnt, err := forwardDelivery.ToEntity()
		if err != nil {
			recordTrampolineErr(err)
			return &handler.Response{Status: 500}, err
		}
		if _, err := ap.Executor().ExecuteWithParams(observerPattern, "deliver", fwdEnt); err != nil {
			recordTrampolineErr(err)
			return &handler.Response{Status: 500}, err
		}

		ack, _ := ecf.Encode(map[string]interface{}{"ack": true})
		ackEnt, _ := entity.NewEntity("primitive/any", cbor.RawMessage(ack))
		return &handler.Response{Status: 200, Result: ackEnt}, nil
	})
	if err != nil {
		t.Fatalf("register trampoline: %v", err)
	}
	t.Cleanup(func() { _ = trampolineHandle.Close() })

	// --- Step 5: install ContA (chain entry) -------------------------
	//
	// ContA targets compute1 with static params {values:[10,20,30]} and
	// delivers the result to the trampoline. Single-shot.
	staticParams := map[string]interface{}{
		"values": []interface{}{uint64(10), uint64(20), uint64(30)},
	}
	staticRaw, _ := ecf.Encode(staticParams)
	contA := types.ContinuationData{
		Target:    compute1Pattern,
		Operation: "compute",
		Params:    cbor.RawMessage(staticRaw),
		DeliverTo: &types.DeliverySpec{
			URI:       trampolinePattern,
			Operation: "deliver",
		},
	}
	entitysdk.SetDefaultDispatchCap(ap.OwnerCapability().ContentHash, &contA)
	contAEnt, err := contA.ToEntity()
	if err != nil {
		t.Fatalf("build contA: %v", err)
	}
	const contAPath = "system/inbox/multi-step/step1"
	if _, err := ap.Continuation().Install(context.Background(), contAPath, contAEnt); err != nil {
		t.Fatalf("install contA: %v", err)
	}

	// --- Step 6: advance contA → triggers the chain ------------------
	emptyMap, _ := ecf.Encode(map[string]interface{}{})
	advReq := types.ContinuationAdvanceRequestData{Result: cbor.RawMessage(emptyMap)}
	advEnt, _ := advReq.ToEntity()
	advResp, err := ap.Executor().ExecuteOnResource(
		"system/continuation", "advance", advEnt,
		&types.ResourceTarget{Targets: []string{contAPath}},
	)
	if err != nil {
		t.Fatalf("advance contA: %v", err)
	}
	if advResp.Status != 200 {
		t.Fatalf("advance returned status %d (type=%s)", advResp.Status, advResp.Type)
	}

	// --- Step 7: observe final result --------------------------------
	select {
	case got := <-resultCh:
		if got.envErr != nil {
			t.Fatalf("observer envelope decode error: %v", got.envErr)
		}
		if !numEq(got.value, 120) {
			t.Fatalf("expected final value=120 (sum=60 × 2), got %v (%T)", got.value, got.value)
		}
		t.Logf("PASS multi-step chain: ContA → compute1 (sum [10,20,30]=60) → trampoline → compute2 (×2=120) → observer ✓")
	case <-time.After(3 * time.Second):
		if trampolineErr != nil {
			t.Fatalf("chain failed inside trampoline: %v", trampolineErr)
		}
		t.Fatal("observer never received the final result (3s timeout) — chain did not complete end-to-end")
	}
	if trampolineErr != nil {
		t.Errorf("trampoline reported error after PASS: %v", trampolineErr)
	}
}

// statusErr lets the trampoline thread non-zero compute-dispatch status
// back through the test harness without conflating with Go errors.
type statusErr struct {
	status uint
	what   string
}

func (e *statusErr) Error() string {
	return e.what
}

func NewStatusErr(status uint, what string) error {
	return &statusErr{status: status, what: what}
}
