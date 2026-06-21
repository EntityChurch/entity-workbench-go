package entitysdk_test

// Phase H.1 — translation test sweep.
//
// Per the arch handback: the area "most wanted nailed down with real
// tests before calling the foundation closed" is CBOR indexing +
// data-structure interaction +
// translation. The S1 builder is the natural place to grow this; each
// test below builds an IR via the builder, asserts the runtime result,
// and walks the stored entities by content hash to confirm the
// translated IR shape matches what was authored.
//
// Two-axis coverage:
//
//   *Runtime* — does the evaluator produce the expected value for this
//   shape of structured input? Covers v3.18 N.1 (index/length on CBOR
//   arrays + edge cases), Field on record values, NumericCast at
//   boundaries (Rule 11), Construct composition.
//
//   *Translation* — does the builder produce the IR a human author
//   would expect for this expression? Read back via Store().GetByHash,
//   decode the data structs, walk children. If the builder's
//   construction ever drifts from a one-to-one mapping with the spec's
//   data types, these tests catch it before a frontend hits the
//   wrong-shape surprise.
//
// Tests are named TestComputeTranslation_{Scenario} so they cluster
// in test output and can be selected with `make test-sdk
// ARGS="-run TestComputeTranslation"`.

import (
	"context"
	"testing"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

// --- scenario 1: nested record field-chain -----------------------------

// TestComputeTranslation_NestedRecordFieldChain — the canonical
// "first concrete unit" from the Phase H sketch:
//
//	field(field(scope.params, "user"), "name")
//
// Post-F9 (EXTENSION-COMPUTE v3.19 N.5): runtime returns "alice"; the
// inner Field's bare-map return is accepted by the outer Field per the
// composing-navigation pin (core-go fix at ext/compute/eval.go:632).
// Translation: IR is Field→Field→LookupScope("params").
func TestComputeTranslation_NestedRecordFieldChain(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	pattern := "app/tr/nested-record"
	c := ap.Compute()
	expr := c.Field(c.Field(c.LookupScope("params"), "user"), "name")

	h, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: pattern,
		Name:    "tr-nested-record",
		Operations: map[string]types.HandlerOperationSpec{
			"compute": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, expr)
	if err != nil {
		t.Fatalf("RegisterComputeHandler: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	// --- runtime ---
	params, _ := entitysdk.PrimitiveAny(map[string]interface{}{
		"user": map[string]interface{}{"name": "alice", "age": uint64(30)},
		"misc": "ignored",
	})
	resp, err := ap.Executor().ExecuteWithParams(pattern, "compute", params)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if resp.Status != 200 {
		t.Fatalf("status %d (type=%s)", resp.Status, resp.Type)
	}
	got := decodeAny(t, resp)
	if got != "alice" {
		t.Fatalf("expected %q, got %v (%T)", "alice", got, got)
	}

	// --- translation: walk root → child → grandchild by content hash ---
	root := loadRoot(t, ap, pattern)
	if root.Type != types.TypeComputeField {
		t.Fatalf("root: expected %q, got %q", types.TypeComputeField, root.Type)
	}
	var outer types.ComputeFieldData
	mustDecode(t, root.Data, &outer)
	if outer.Name != "name" {
		t.Fatalf("outer Field.Name: expected %q, got %q", "name", outer.Name)
	}

	mid, ok := ap.Store().GetByHash(outer.Entity)
	if !ok {
		t.Fatalf("mid entity not found")
	}
	if mid.Type != types.TypeComputeField {
		t.Fatalf("mid: expected %q, got %q", types.TypeComputeField, mid.Type)
	}
	var inner types.ComputeFieldData
	mustDecode(t, mid.Data, &inner)
	if inner.Name != "user" {
		t.Fatalf("inner Field.Name: expected %q, got %q", "user", inner.Name)
	}

	leaf, ok := ap.Store().GetByHash(inner.Entity)
	if !ok {
		t.Fatalf("leaf entity not found")
	}
	if leaf.Type != types.TypeComputeLookupScope {
		t.Fatalf("leaf: expected %q, got %q", types.TypeComputeLookupScope, leaf.Type)
	}
	var ls types.ComputeLookupScopeData
	mustDecode(t, leaf.Data, &ls)
	if ls.Name != "params" {
		t.Fatalf("leaf LookupScope.Name: expected %q, got %q", "params", ls.Name)
	}

	t.Logf("PASS (v3.19 N.5): runtime → %q; IR shape Field[name]→Field[user]→LookupScope[params]", got)
}

// --- scenario 2: deep CBOR-array index-chain (2D matrix) ---------------

// TestComputeTranslation_DeepIndexChain — addresses an element of a
// 2D matrix:
//
//	index(index(field(scope.params, "matrix"), lit 0), lit 1)
//
// matrix=[[1,2,3],[4,5,6]] → matrix[0][1] = 2. Exercises N.1 nested
// index on a CBOR array of CBOR arrays.
func TestComputeTranslation_DeepIndexChain(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	pattern := "app/tr/deep-index"
	c := ap.Compute()
	expr := c.Index(
		c.Index(
			c.Field(c.LookupScope("params"), "matrix"),
			c.Literal(uint64(0)),
		),
		c.Literal(uint64(1)),
	)

	h, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: pattern,
		Name:    "tr-deep-index",
		Operations: map[string]types.HandlerOperationSpec{
			"compute": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, expr)
	if err != nil {
		t.Fatalf("RegisterComputeHandler: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	params, _ := entitysdk.PrimitiveAny(map[string]interface{}{
		"matrix": []interface{}{
			[]interface{}{uint64(1), uint64(2), uint64(3)},
			[]interface{}{uint64(4), uint64(5), uint64(6)},
		},
	})
	resp, err := ap.Executor().ExecuteWithParams(pattern, "compute", params)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if resp.Status != 200 {
		t.Fatalf("status %d (type=%s)", resp.Status, resp.Type)
	}
	got := decodeAny(t, resp)
	if !numEq(got, 2) {
		t.Fatalf("runtime: expected 2, got %v (%T)", got, got)
	}

	// --- translation: root is Index over Index over Field ---
	root := loadRoot(t, ap, pattern)
	if root.Type != types.TypeComputeIndex {
		t.Fatalf("root: expected %q, got %q", types.TypeComputeIndex, root.Type)
	}
	var outer types.ComputeIndexData
	mustDecode(t, root.Data, &outer)

	mid, ok := ap.Store().GetByHash(outer.Array)
	if !ok {
		t.Fatalf("mid entity not found")
	}
	if mid.Type != types.TypeComputeIndex {
		t.Fatalf("mid: expected %q, got %q", types.TypeComputeIndex, mid.Type)
	}
	var inner types.ComputeIndexData
	mustDecode(t, mid.Data, &inner)

	leaf, ok := ap.Store().GetByHash(inner.Array)
	if !ok {
		t.Fatalf("leaf entity not found")
	}
	if leaf.Type != types.TypeComputeField {
		t.Fatalf("leaf: expected %q, got %q", types.TypeComputeField, leaf.Type)
	}
	t.Logf("PASS: matrix[0][1] = 2; IR shape Index→Index→Field")
}

// --- scenario 3: record-of-arrays + index ------------------------------

// TestComputeTranslation_RecordOfArrays — the common shape: a record
// whose values are arrays, index into one:
//
//	index(field(scope.params, "items"), lit 2)
//
// Combines Field (record access) + Index (array access) in one
// expression. params={"items":[10,20,30,40]} → 30.
func TestComputeTranslation_RecordOfArrays(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	pattern := "app/tr/record-of-arrays"
	c := ap.Compute()
	expr := c.Index(
		c.Field(c.LookupScope("params"), "items"),
		c.Literal(uint64(2)),
	)

	h, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: pattern,
		Name:    "tr-record-of-arrays",
		Operations: map[string]types.HandlerOperationSpec{
			"compute": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, expr)
	if err != nil {
		t.Fatalf("RegisterComputeHandler: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	params, _ := entitysdk.PrimitiveAny(map[string]interface{}{
		"items": []interface{}{uint64(10), uint64(20), uint64(30), uint64(40)},
	})
	resp, err := ap.Executor().ExecuteWithParams(pattern, "compute", params)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if resp.Status != 200 {
		t.Fatalf("status %d (type=%s)", resp.Status, resp.Type)
	}
	got := decodeAny(t, resp)
	if !numEq(got, 30) {
		t.Fatalf("runtime: expected 30, got %v (%T)", got, got)
	}

	root := loadRoot(t, ap, pattern)
	if root.Type != types.TypeComputeIndex {
		t.Fatalf("root: expected %q, got %q", types.TypeComputeIndex, root.Type)
	}
	t.Logf("PASS: items[2] = 30; IR root is Index over Field")
}

// --- scenario 4: length on empty array ---------------------------------

// TestComputeTranslation_LengthEmpty — N.1 edge case: length of an
// empty CBOR array is 0, not an error.
func TestComputeTranslation_LengthEmpty(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	pattern := "app/tr/length-empty"
	c := ap.Compute()
	expr := c.Length(c.Field(c.LookupScope("params"), "xs"))

	h, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: pattern,
		Name:    "tr-length-empty",
		Operations: map[string]types.HandlerOperationSpec{
			"compute": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, expr)
	if err != nil {
		t.Fatalf("RegisterComputeHandler: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	params, _ := entitysdk.PrimitiveAny(map[string]interface{}{
		"xs": []interface{}{},
	})
	resp, err := ap.Executor().ExecuteWithParams(pattern, "compute", params)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if resp.Status != 200 {
		t.Fatalf("status %d (type=%s)", resp.Status, resp.Type)
	}
	got := decodeAny(t, resp)
	if !numEq(got, 0) {
		t.Fatalf("runtime: expected 0, got %v (%T)", got, got)
	}
	t.Logf("PASS: length([]) = 0 (N.1 empty-array edge case holds)")
}

// --- scenario 5: length on heterogeneous array -------------------------

// TestComputeTranslation_LengthHeterogeneous — Length doesn't care
// about element types, only count. Mixed array of uint/string/bool
// has length 3.
func TestComputeTranslation_LengthHeterogeneous(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	pattern := "app/tr/length-hetero"
	c := ap.Compute()
	expr := c.Length(c.Field(c.LookupScope("params"), "mix"))

	h, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: pattern,
		Name:    "tr-length-hetero",
		Operations: map[string]types.HandlerOperationSpec{
			"compute": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, expr)
	if err != nil {
		t.Fatalf("RegisterComputeHandler: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	params, _ := entitysdk.PrimitiveAny(map[string]interface{}{
		"mix": []interface{}{uint64(42), "hello", true},
	})
	resp, err := ap.Executor().ExecuteWithParams(pattern, "compute", params)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if resp.Status != 200 {
		t.Fatalf("status %d (type=%s)", resp.Status, resp.Type)
	}
	got := decodeAny(t, resp)
	if !numEq(got, 3) {
		t.Fatalf("runtime: expected 3, got %v (%T)", got, got)
	}
	t.Logf("PASS: length([42,\"hello\",true]) = 3 (Length is element-agnostic)")
}

// --- scenario 6: index out-of-range produces compute/error -------------

// TestComputeTranslation_IndexOutOfRange — N.1 edge case: indexing
// past the end of an array.
//
// Post-F10 (EXTENSION-COMPUTE v3.19): evaluated compute/error returns
// at status 200 with the error entity as the result (per the §1.5
// error-as-value model). resp.Type == "compute/error" with code
// == "index_out_of_range". core-go fix at handler.go:99/:345.
func TestComputeTranslation_IndexOutOfRange(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	pattern := "app/tr/index-oor"
	c := ap.Compute()
	expr := c.Index(
		c.Field(c.LookupScope("params"), "xs"),
		c.Literal(uint64(99)),
	)

	h, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: pattern,
		Name:    "tr-index-oor",
		Operations: map[string]types.HandlerOperationSpec{
			"compute": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, expr)
	if err != nil {
		t.Fatalf("RegisterComputeHandler: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	params, _ := entitysdk.PrimitiveAny(map[string]interface{}{
		"xs": []interface{}{uint64(10), uint64(20)},
	})
	resp, err := ap.Executor().ExecuteWithParams(pattern, "compute", params)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if resp.Status != 200 {
		t.Fatalf("expected status 200 with compute/error body per v3.19, got status %d type=%s", resp.Status, resp.Type)
	}
	if resp.Type != types.TypeComputeError {
		t.Fatalf("expected resp.Type=%q, got %q", types.TypeComputeError, resp.Type)
	}
	var ed types.ComputeErrorData
	mustDecode(t, resp.Data, &ed)
	if ed.Code != "index_out_of_range" {
		t.Fatalf("expected code=index_out_of_range, got %q", ed.Code)
	}

	// IR-shape translation.
	root := loadRoot(t, ap, pattern)
	if root.Type != types.TypeComputeIndex {
		t.Fatalf("translation: root: expected %q, got %q", types.TypeComputeIndex, root.Type)
	}
	t.Logf("PASS (v3.19 F10): OOR returns status 200 with compute/error{code=%q, message=%q}; IR root is Index", ed.Code, ed.Message)
}

// --- scenario 7: NumericCast inline at use site (Rule 11 success) ------

// TestComputeTranslation_NumericCastInline — Rule 11 in its positive
// form: cast appears inline at the operand site (not bound via Let)
// and threads through. add(cast(-1 → uint), 1) wraps to 0 because
// uint addition is mod 2^64.
//
// This is the worked example from GUIDE-CORE-COMPUTATIONAL-ARCHITECTURE
// §9 (lowering patterns) — the cast is consumed by the arithmetic that
// immediately consumes its result.
func TestComputeTranslation_NumericCastInline(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	pattern := "app/tr/numeric-cast-inline"
	c := ap.Compute()
	// add(cast(-1 → uint), 1) — under unsigned 64-bit, MAX+1 wraps to 0.
	expr := c.Arithmetic("add",
		c.NumericCast(c.Literal(int64(-1)), "primitive/uint"),
		c.Literal(uint64(1)),
	)

	h, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: pattern,
		Name:    "tr-cast-inline",
		Operations: map[string]types.HandlerOperationSpec{
			"compute": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, expr)
	if err != nil {
		t.Fatalf("RegisterComputeHandler: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	params, _ := entitysdk.PrimitiveAny(map[string]interface{}{})
	resp, err := ap.Executor().ExecuteWithParams(pattern, "compute", params)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if resp.Status != 200 {
		t.Fatalf("status %d (type=%s)", resp.Status, resp.Type)
	}
	got := decodeAny(t, resp)
	// Expect 0 (uint wraparound) — confirms unsigned interpretation
	// propagated through the cast to the add.
	if !numEq(got, 0) {
		t.Fatalf("runtime: expected 0 (unsigned wraparound), got %v (%T)", got, got)
	}

	// --- translation: root is Arithmetic, left child is NumericCast,
	//     right child is Literal. The NumericCast appears in the IR
	//     (not bound via Let). ---
	root := loadRoot(t, ap, pattern)
	if root.Type != types.TypeComputeArithmetic {
		t.Fatalf("root: expected %q, got %q", types.TypeComputeArithmetic, root.Type)
	}
	var ar types.ComputeArithmeticData
	mustDecode(t, root.Data, &ar)
	if ar.Op != "add" {
		t.Fatalf("Arithmetic.Op: expected add, got %q", ar.Op)
	}

	left, ok := ap.Store().GetByHash(ar.Left)
	if !ok {
		t.Fatalf("left child not found")
	}
	if left.Type != types.TypeComputeNumericCast {
		t.Fatalf("left child: expected %q, got %q", types.TypeComputeNumericCast, left.Type)
	}
	var nc types.ComputeNumericCastData
	mustDecode(t, left.Data, &nc)
	if nc.ToType != "primitive/uint" {
		t.Fatalf("NumericCast.ToType: expected %q, got %q", "primitive/uint", nc.ToType)
	}
	t.Logf("PASS: add(cast(-1→uint), 1) = 0 (wraparound); IR shape Arithmetic[add]→{NumericCast[primitive/uint], Literal}")
}

// --- scenario 8: Construct with mixed literal and computed fields ------

// TestComputeTranslation_ConstructMixed — Construct composes a typed
// entity from sub-values that may be Literal or computed. Here:
//
//	construct("primitive/any", {
//	  "sum":   arithmetic("add", lit 7, lit 8),
//	  "label": lit "result",
//	})
//
// The runtime result is a primitive/any whose body has both fields.
// The IR root is Construct; one field-child is Arithmetic, the other
// is Literal.
func TestComputeTranslation_ConstructMixed(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	pattern := "app/tr/construct-mixed"
	c := ap.Compute()
	expr := c.Construct("primitive/any", map[string]*entitysdk.Builder{
		"sum":   c.Arithmetic("add", c.Literal(uint64(7)), c.Literal(uint64(8))),
		"label": c.Literal("result"),
	})

	h, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: pattern,
		Name:    "tr-construct-mixed",
		Operations: map[string]types.HandlerOperationSpec{
			"compute": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, expr)
	if err != nil {
		t.Fatalf("RegisterComputeHandler: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	params, _ := entitysdk.PrimitiveAny(map[string]interface{}{})
	resp, err := ap.Executor().ExecuteWithParams(pattern, "compute", params)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if resp.Status != 200 {
		t.Fatalf("status %d (type=%s)", resp.Status, resp.Type)
	}
	got := decodeAny(t, resp)
	m, ok := got.(map[interface{}]interface{})
	if !ok {
		// CBOR decode may produce map[string]interface{} for string-keyed
		// maps depending on decoder options; accept either.
		if mm, okk := got.(map[string]interface{}); okk {
			if !numEq(mm["sum"], 15) {
				t.Fatalf("runtime: expected sum=15, got %v", mm["sum"])
			}
			if mm["label"] != "result" {
				t.Fatalf("runtime: expected label=\"result\", got %v", mm["label"])
			}
		} else {
			t.Fatalf("runtime: expected map, got %T (%v)", got, got)
		}
	} else {
		if !numEq(m["sum"], 15) {
			t.Fatalf("runtime: expected sum=15, got %v", m["sum"])
		}
		if m["label"] != "result" {
			t.Fatalf("runtime: expected label=\"result\", got %v", m["label"])
		}
	}

	// --- translation: root is Construct["primitive/any"] with fields
	//     {label: Literal, sum: Arithmetic} ---
	root := loadRoot(t, ap, pattern)
	if root.Type != types.TypeComputeConstruct {
		t.Fatalf("root: expected %q, got %q", types.TypeComputeConstruct, root.Type)
	}
	var cd types.ComputeConstructData
	mustDecode(t, root.Data, &cd)
	if cd.EntityType != "primitive/any" {
		t.Fatalf("Construct.EntityType: expected %q, got %q", "primitive/any", cd.EntityType)
	}
	if len(cd.Fields) != 2 {
		t.Fatalf("Construct.Fields: expected 2 entries, got %d", len(cd.Fields))
	}
	sumHash, ok := cd.Fields["sum"]
	if !ok {
		t.Fatalf("Construct.Fields missing \"sum\"")
	}
	labelHash, ok := cd.Fields["label"]
	if !ok {
		t.Fatalf("Construct.Fields missing \"label\"")
	}

	sumEnt, ok := ap.Store().GetByHash(sumHash)
	if !ok {
		t.Fatalf("sum child not found")
	}
	if sumEnt.Type != types.TypeComputeArithmetic {
		t.Fatalf("sum child: expected %q, got %q", types.TypeComputeArithmetic, sumEnt.Type)
	}
	labelEnt, ok := ap.Store().GetByHash(labelHash)
	if !ok {
		t.Fatalf("label child not found")
	}
	if labelEnt.Type != types.TypeComputeLiteral {
		t.Fatalf("label child: expected %q, got %q", types.TypeComputeLiteral, labelEnt.Type)
	}
	t.Logf("PASS: Construct{sum=15, label=\"result\"}; IR Construct[primitive/any]→{sum:Arithmetic, label:Literal}")
}

// --- scenario 9: deep field-chain (3 levels) ---------------------------

// TestComputeTranslation_DeepFieldChain — three-level Field nesting:
//
//	field(field(field(scope.params, "outer"), "middle"), "inner")
//
// Post-v3.19 N.5: composes to arbitrary depth.
func TestComputeTranslation_DeepFieldChain(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	pattern := "app/tr/deep-field"
	c := ap.Compute()
	expr := c.Field(
		c.Field(
			c.Field(c.LookupScope("params"), "outer"),
			"middle",
		),
		"inner",
	)

	h, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: pattern,
		Name:    "tr-deep-field",
		Operations: map[string]types.HandlerOperationSpec{
			"compute": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, expr)
	if err != nil {
		t.Fatalf("RegisterComputeHandler: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	params, _ := entitysdk.PrimitiveAny(map[string]interface{}{
		"outer": map[string]interface{}{
			"middle": map[string]interface{}{
				"inner": uint64(123),
			},
		},
	})
	resp, err := ap.Executor().ExecuteWithParams(pattern, "compute", params)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if resp.Status != 200 {
		t.Fatalf("status %d (type=%s)", resp.Status, resp.Type)
	}
	got := decodeAny(t, resp)
	if !numEq(got, 123) {
		t.Fatalf("expected 123, got %v (%T)", got, got)
	}

	// Walk three levels of Field, then expect LookupScope at the base.
	depth := 0
	cur := loadRoot(t, ap, pattern)
	wantNames := []string{"inner", "middle", "outer"}
	for cur.Type == types.TypeComputeField {
		var fd types.ComputeFieldData
		mustDecode(t, cur.Data, &fd)
		if depth >= len(wantNames) {
			t.Fatalf("walk too deep: unexpected %dth Field", depth+1)
		}
		if fd.Name != wantNames[depth] {
			t.Fatalf("level %d: expected Field.Name=%q, got %q", depth, wantNames[depth], fd.Name)
		}
		depth++
		next, found := ap.Store().GetByHash(fd.Entity)
		if !found {
			t.Fatalf("level %d child not found", depth)
		}
		cur = next
	}
	if depth != 3 {
		t.Fatalf("expected 3 Field levels, walked %d", depth)
	}
	if cur.Type != types.TypeComputeLookupScope {
		t.Fatalf("base: expected %q, got %q", types.TypeComputeLookupScope, cur.Type)
	}
	t.Logf("PASS (v3.19 N.5): outer.middle.inner = 123; IR shape Field[inner]→Field[middle]→Field[outer]→LookupScope[params]")
}

// --- helpers -----------------------------------------------------------

// decodeAny pulls the inner value out of a dispatch response, handling
// the three response shapes the evaluator may produce for these
// expressions (none of which are handler-mode Apply, so SA-4 doesn't
// apply): compute/error (fails test), primitive/any (most common —
// the bare value), or anything else (rare; returned as-is for the
// caller to inspect).
func decodeAny(t *testing.T, resp *entitysdk.Response) interface{} {
	t.Helper()
	switch resp.Type {
	case types.TypeComputeError:
		var d types.ComputeErrorData
		mustDecode(t, resp.Data, &d)
		t.Fatalf("compute error: code=%q message=%q", d.Code, d.Message)
		return nil
	case "primitive/any":
		var v interface{}
		mustDecode(t, resp.Data, &v)
		return v
	default:
		// Some non-Apply paths may return the bare CBOR of the result
		// without an envelope. Try decoding to interface{} as a last
		// resort.
		var v interface{}
		if err := ecf.Decode(resp.Data, &v); err == nil {
			return v
		}
		return resp.Data
	}
}

// mustDecode decodes CBOR into dst or fatals the test. Keeps the test
// bodies free of repetitive error-check boilerplate.
func mustDecode(t *testing.T, data []byte, dst interface{}) {
	t.Helper()
	if err := ecf.Decode(data, dst); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

// respStatus returns resp.Status or 0 if resp is nil. Used by the
// F9/F10 documentation tests so the diagnostic log line works
// uniformly whether the dispatch path returned a response or just an
// error.
func respStatus(resp *entitysdk.Response) int {
	if resp == nil {
		return 0
	}
	return int(resp.Status)
}

// respType returns resp.Type or empty if resp is nil. Same purpose as
// respStatus.
func respType(resp *entitysdk.Response) string {
	if resp == nil {
		return ""
	}
	return resp.Type
}

// loadRoot resolves the expression root entity for a compute-backed
// handler registered at pattern. RegisterComputeHandler stores the
// root at {pattern}/_compute_expr (see register_compute_handler.go);
// this helper centralizes that knowledge so the tests stay readable.
func loadRoot(t *testing.T, ap *entitysdk.AppPeer, pattern string) entity.Entity {
	t.Helper()
	ent, found, err := ap.Get(pattern + "/_compute_expr")
	if err != nil {
		t.Fatalf("loadRoot: get %s: %v", pattern+"/_compute_expr", err)
	}
	if !found {
		t.Fatalf("loadRoot: no entity at %s", pattern+"/_compute_expr")
	}
	return ent
}
