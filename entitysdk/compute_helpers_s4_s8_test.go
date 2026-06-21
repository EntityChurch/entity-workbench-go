package entitysdk_test

// Tests for the S4–S8 ergonomic helpers per
// SDK-EXTENSION-OPERATIONS E7.2 (post-FEEDBACK-COMPUTE-FOUNDATION-CLOSED).
//
// Each helper gets a focused smoke test exercising its happy path
// plus a couple of representative edge cases. The refactored
// scenario/POC tests further exercise them in realistic flows.

import (
	"context"
	"testing"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"

	"github.com/fxamacker/cbor/v2"
)

// --- S4: UnwrapComputeResultAsMap / AsList ---------------------------

// TestUnwrapComputeResultAsMap_RecordReturn — happy path: a compute
// handler returns a primitive/any record; the helper decodes it into
// map[string]interface{} normalizing the CBOR two-shape decode.
func TestUnwrapComputeResultAsMap_RecordReturn(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	pattern := "app/helpers/s4/record"
	c := ap.Compute()
	expr := entitysdk.LowerRecord(c, "primitive/any", map[string]interface{}{
		"sum":   c.Literal(uint64(60)),
		"count": c.Literal(uint64(3)),
	})
	h, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: pattern,
		Name:    "s4-record",
		Operations: map[string]types.HandlerOperationSpec{
			"compute": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, expr)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	params, _ := entitysdk.PrimitiveAny(map[string]interface{}{})
	resp, err := ap.Executor().ExecuteWithParams(pattern, "compute", params)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	rec, err := entitysdk.UnwrapComputeResultAsMap(resp)
	if err != nil {
		t.Fatalf("UnwrapComputeResultAsMap: %v", err)
	}
	if !numEq(rec["sum"], 60) {
		t.Errorf("sum: got %v, want 60", rec["sum"])
	}
	if !numEq(rec["count"], 3) {
		t.Errorf("count: got %v, want 3", rec["count"])
	}
}

// TestUnwrapComputeResultAsList_FilterReturn — happy path: a filter
// returns a primitive/any array; the helper decodes it into
// []interface{}.
func TestUnwrapComputeResultAsList_FilterReturn(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	pattern := "app/helpers/s4/list"
	c := ap.Compute()
	expr := entitysdk.LowerFilter(c,
		c.Field(c.LookupScope("params"), "numbers"),
		func(elem *entitysdk.Builder) *entitysdk.Builder {
			return c.Compare("gt", elem, c.Literal(uint64(5)))
		})
	h, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: pattern,
		Name:    "s4-list",
		Operations: map[string]types.HandlerOperationSpec{
			"compute": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, expr)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	params, _ := entitysdk.PrimitiveAny(map[string]interface{}{
		"numbers": []interface{}{uint64(3), uint64(7), uint64(2), uint64(10)},
	})
	resp, err := ap.Executor().ExecuteWithParams(pattern, "compute", params)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	got, err := entitysdk.UnwrapComputeResultAsList(resp)
	if err != nil {
		t.Fatalf("UnwrapComputeResultAsList: %v", err)
	}
	if len(got) != 2 || !numEq(got[0], 7) || !numEq(got[1], 10) {
		t.Fatalf("expected [7,10], got %v", got)
	}
}

// --- S6: InstallComputeSubgraph -------------------------------------

// TestInstallComputeSubgraph_HappyPath — typed install of a reactive
// subgraph; ensure SubgraphHandle exposes the right metadata and
// Close() uninstalls cleanly.
func TestInstallComputeSubgraph_HappyPath(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	// Put a single FileData entity at a known path.
	bare := "app/helpers/s6/input"
	ent, _ := entitysdk.PrimitiveAny(map[string]interface{}{"size": uint64(42)})
	if _, err := ap.PutEntity(bare, ent); err != nil {
		t.Fatalf("put input: %v", err)
	}

	// Build expr that reads the size field via the local LookupTree.
	c := ap.Compute()
	expr := c.Field(c.LookupTreeLocal(bare), "size")

	resultPath := "app/helpers/s6/result"
	h, err := ap.InstallComputeSubgraph(context.Background(), expr, resultPath)
	if err != nil {
		t.Fatalf("InstallComputeSubgraph: %v", err)
	}
	if h.ResultPath() != resultPath {
		t.Errorf("ResultPath: got %q, want %q", h.ResultPath(), resultPath)
	}
	if h.SubgraphPath() == "" {
		t.Errorf("SubgraphPath empty")
	}

	// Mutate input — should fire reactive eval, result_path populated.
	ent2, _ := entitysdk.PrimitiveAny(map[string]interface{}{"size": uint64(99)})
	if _, err := ap.PutEntity(bare, ent2); err != nil {
		t.Fatalf("mutate input: %v", err)
	}
	v, found, err := h.ReadResult()
	if err != nil || !found {
		t.Fatalf("ReadResult: err=%v found=%v", err, found)
	}
	if !numEq(v, 99) {
		t.Fatalf("expected 99 at result_path, got %v", v)
	}

	// Close uninstalls.
	if err := h.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Idempotency: second close is a no-op.
	if err := h.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// --- S6b: IsHandlerRegistered ---------------------------------------

func TestIsHandlerRegistered_BeforeAndAfterRegister(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	pattern := "app/helpers/s6b/probe"

	if ap.IsHandlerRegistered(pattern) {
		t.Errorf("pre-register: IsHandlerRegistered returned true (no handler exists)")
	}

	c := ap.Compute()
	h, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: pattern,
		Name:    "s6b",
		Operations: map[string]types.HandlerOperationSpec{
			"compute": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, c.Literal(uint64(7)))
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	if !ap.IsHandlerRegistered(pattern) {
		t.Errorf("post-register: IsHandlerRegistered returned false")
	}
	if ap.IsHandlerRegistered("app/nonexistent/pattern") {
		t.Errorf("nonexistent pattern: IsHandlerRegistered returned true")
	}
}

// --- S7: UnwrapChainStepDelivery ------------------------------------

func TestUnwrapChainStepDelivery_EntityEnvelopeShape(t *testing.T) {
	// Build the "what a chain step receives" shape: an entity wrapping
	// InboxDeliveryData, where Result is the CBOR encoding of an
	// entity-envelope-shaped map {type, data, content_hash}.
	innerMap, _ := ecf.Encode(map[string]interface{}{
		"type":         "primitive/any",
		"data":         uint64(60),
		"content_hash": []byte{0x01, 0x02, 0x03},
	})
	delivery := types.InboxDeliveryData{
		OriginalRequestID: "test-req-1",
		Status:            200,
		Result:            cbor.RawMessage(innerMap),
	}
	deliveryRaw, _ := ecf.Encode(delivery)
	params, _ := entity.NewEntity("primitive/any", cbor.RawMessage(deliveryRaw))

	d, err := entitysdk.UnwrapChainStepDelivery(params)
	if err != nil {
		t.Fatalf("UnwrapChainStepDelivery: %v", err)
	}
	if d.Status != 200 {
		t.Errorf("status: got %d, want 200", d.Status)
	}
	if !numEq(d.Value, 60) {
		t.Errorf("inner value: got %v, want 60", d.Value)
	}
	if d.OriginalRequestID != "test-req-1" {
		t.Errorf("original_request_id: got %q, want test-req-1", d.OriginalRequestID)
	}
}

func TestUnwrapChainStepDelivery_SA4ResultShape(t *testing.T) {
	// SA-4 compute/result envelope: {value, expression}.
	innerMap, _ := ecf.Encode(map[string]interface{}{
		"value":      uint64(120),
		"expression": []byte{0x10, 0x20},
	})
	delivery := types.InboxDeliveryData{
		Status: 200,
		Result: cbor.RawMessage(innerMap),
	}
	deliveryRaw, _ := ecf.Encode(delivery)
	params, _ := entity.NewEntity("primitive/any", cbor.RawMessage(deliveryRaw))

	d, err := entitysdk.UnwrapChainStepDelivery(params)
	if err != nil {
		t.Fatalf("UnwrapChainStepDelivery (SA-4): %v", err)
	}
	if d.Status != 200 {
		t.Errorf("status: got %d, want 200", d.Status)
	}
	if !numEq(d.Value, 120) {
		t.Errorf("SA-4 inner value: got %v, want 120", d.Value)
	}
}

// --- S8: LookupTreeLocal --------------------------------------------

func TestLookupTreeLocal_QualifiesWithPeerID(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	// Put an entity at a bare path; LookupTreeLocal of that bare path
	// should produce a LookupTree IR node whose Path matches what the
	// store's TreeChangeEvent fires (the qualified shape).
	bare := "app/helpers/s8/input"
	ent, _ := entitysdk.PrimitiveAny(map[string]interface{}{"size": uint64(5)})
	if _, err := ap.PutEntity(bare, ent); err != nil {
		t.Fatalf("put: %v", err)
	}

	// Build and install a subgraph using LookupTreeLocal. If
	// qualification is wrong, the dep won't match the change event
	// and the reactive write to result_path won't fire on mutation.
	c := ap.Compute()
	expr := c.Field(c.LookupTreeLocal(bare), "size")
	resultPath := "app/helpers/s8/result"
	h, err := ap.InstallComputeSubgraph(context.Background(), expr, resultPath)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	// Mutate input — auto-qualified dep should match the event.
	ent2, _ := entitysdk.PrimitiveAny(map[string]interface{}{"size": uint64(42)})
	if _, err := ap.PutEntity(bare, ent2); err != nil {
		t.Fatalf("mutate: %v", err)
	}

	v, found, err := h.ReadResult()
	if err != nil || !found {
		t.Fatalf("ReadResult: err=%v found=%v — LookupTreeLocal failed to qualify (dep mismatch)", err, found)
	}
	if !numEq(v, 42) {
		t.Fatalf("expected 42 at result_path, got %v", v)
	}
	t.Logf("PASS: LookupTreeLocal correctly auto-qualifies; reactive eval fired on mutation; result=%v", v)
}
