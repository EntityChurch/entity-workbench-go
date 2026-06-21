package entitysdk_test

// Phase H.2 lowering-toolkit tests. Each test asserts both the
// translated IR shape (proving the lowering produces the canonical
// equivalent of the hand-written S1 form) and the runtime result
// (proving it evaluates the same).

import (
	"context"
	"testing"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

// --- LowerRecord -------------------------------------------------------

// TestLowerRecord_AllLiterals — every field is a raw Go value;
// LowerRecord auto-wraps each via Literal. The IR is a Construct
// whose every field-child is a compute/literal. Runtime is the
// expected record.
func TestLowerRecord_AllLiterals(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	pattern := "app/lower/record-all-literals"
	c := ap.Compute()
	expr := entitysdk.LowerRecord(c, "primitive/any", map[string]interface{}{
		"name": "alice",
		"age":  uint64(30),
		"flag": true,
	})

	h, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: pattern,
		Name:    "lower-record-all-literals",
		Operations: map[string]types.HandlerOperationSpec{
			"compute": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, expr)
	if err != nil {
		t.Fatalf("RegisterComputeHandler: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	// --- runtime ---
	params, _ := entitysdk.PrimitiveAny(map[string]interface{}{})
	resp, err := ap.Executor().ExecuteWithParams(pattern, "compute", params)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if resp.Status != 200 {
		t.Fatalf("status %d (type=%s)", resp.Status, resp.Type)
	}
	got := decodeAny(t, resp)
	assertRecordEquals(t, got, map[string]interface{}{
		"name": "alice",
		"age":  uint64(30),
		"flag": true,
	})

	// --- translation: root is Construct with three Literal children ---
	root := loadRoot(t, ap, pattern)
	if root.Type != types.TypeComputeConstruct {
		t.Fatalf("root: expected %q, got %q", types.TypeComputeConstruct, root.Type)
	}
	var cd types.ComputeConstructData
	mustDecode(t, root.Data, &cd)
	if cd.EntityType != "primitive/any" {
		t.Fatalf("Construct.EntityType: expected primitive/any, got %q", cd.EntityType)
	}
	if len(cd.Fields) != 3 {
		t.Fatalf("Construct.Fields: expected 3 entries, got %d", len(cd.Fields))
	}
	for fieldName, childHash := range cd.Fields {
		child, ok := ap.Store().GetByHash(childHash)
		if !ok {
			t.Fatalf("field %q child not found in store", fieldName)
		}
		if child.Type != types.TypeComputeLiteral {
			t.Fatalf("field %q child: expected %q, got %q", fieldName, types.TypeComputeLiteral, child.Type)
		}
	}
	t.Logf("PASS: LowerRecord(3 raw values) → Construct[primitive/any] with 3 Literal children; runtime returns expected record")
}

// TestLowerRecord_MixedLiteralAndComputed — some fields are raw Go
// values (auto-Literal); others are pre-built *Builders (passed
// through). The IR is a Construct whose Literal-fields are
// compute/literal and whose computed-fields are whatever the
// caller passed (here a compute/arithmetic).
func TestLowerRecord_MixedLiteralAndComputed(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	pattern := "app/lower/record-mixed"
	c := ap.Compute()
	expr := entitysdk.LowerRecord(c, "primitive/any", map[string]interface{}{
		"label": "result",
		"count": uint64(2),
		"sum":   c.Arithmetic("add", c.Literal(uint64(7)), c.Literal(uint64(8))),
	})

	h, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: pattern,
		Name:    "lower-record-mixed",
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
	assertRecordEquals(t, got, map[string]interface{}{
		"label": "result",
		"count": uint64(2),
		"sum":   uint64(15),
	})

	// --- translation: Construct has three fields, two Literal + one Arithmetic ---
	root := loadRoot(t, ap, pattern)
	var cd types.ComputeConstructData
	mustDecode(t, root.Data, &cd)
	if got, want := root.Type, types.TypeComputeConstruct; got != want {
		t.Fatalf("root type: expected %q, got %q", want, got)
	}
	sumChild, ok := ap.Store().GetByHash(cd.Fields["sum"])
	if !ok {
		t.Fatalf("sum child not found")
	}
	if sumChild.Type != types.TypeComputeArithmetic {
		t.Fatalf("sum child: expected %q (pass-through *Builder), got %q",
			types.TypeComputeArithmetic, sumChild.Type)
	}
	labelChild, ok := ap.Store().GetByHash(cd.Fields["label"])
	if !ok {
		t.Fatalf("label child not found")
	}
	if labelChild.Type != types.TypeComputeLiteral {
		t.Fatalf("label child: expected %q (auto-wrapped), got %q",
			types.TypeComputeLiteral, labelChild.Type)
	}
	t.Logf("PASS: LowerRecord(mixed) — Builders pass through, raw values auto-wrapped as Literal")
}

// TestLowerRecord_EquivalentToHandwrittenConstruct — proves the
// lowering is sugar, not transformation: building the same record
// two ways (LowerRecord vs cb.Construct + cb.Literal) yields
// identical content hashes. If this drifts, the lowering is no
// longer a pure sugar layer and the toolkit's "no semantic change"
// invariant is broken.
func TestLowerRecord_EquivalentToHandwrittenConstruct(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	c := ap.Compute()

	lowered := entitysdk.LowerRecord(c, "primitive/any", map[string]interface{}{
		"a": uint64(1),
		"b": "two",
	})
	handwritten := c.Construct("primitive/any", map[string]*entitysdk.Builder{
		"a": c.Literal(uint64(1)),
		"b": c.Literal("two"),
	})

	hLowered, err := lowered.Build(context.Background(), "app/lower/equiv/lowered")
	if err != nil {
		t.Fatalf("build lowered: %v", err)
	}
	hHandwritten, err := handwritten.Build(context.Background(), "app/lower/equiv/handwritten")
	if err != nil {
		t.Fatalf("build handwritten: %v", err)
	}
	if hLowered != hHandwritten {
		t.Fatalf("expected identical content hashes (lowering is sugar); got lowered=%s handwritten=%s",
			hLowered, hHandwritten)
	}
	t.Logf("PASS: LowerRecord produces byte-identical IR (hash %s) — confirms toolkit is sugar, not transformation", hLowered)
}

// TestLowerRecord_ErrorOnEmptyEntityType — guard against the
// easiest mistake (forgetting the entity type). Build-time error,
// not silent.
func TestLowerRecord_ErrorOnEmptyEntityType(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	c := ap.Compute()
	expr := entitysdk.LowerRecord(c, "", map[string]interface{}{
		"x": uint64(1),
	})
	_, err = expr.Build(context.Background(), "app/lower/empty-type/expr")
	if err == nil {
		t.Fatalf("expected build-time error for empty entityType, got nil")
	}
	t.Logf("PASS: empty entityType rejected at build time: %v", err)
}

// TestLowerRecord_ErrorOnNilFieldValue — guard against a Go nil
// silently encoding as CBOR null when the caller almost certainly
// meant a zero value or an omission. Build-time error.
func TestLowerRecord_ErrorOnNilFieldValue(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	c := ap.Compute()
	expr := entitysdk.LowerRecord(c, "primitive/any", map[string]interface{}{
		"present": uint64(1),
		"absent":  nil,
	})
	_, err = expr.Build(context.Background(), "app/lower/nil-field/expr")
	if err == nil {
		t.Fatalf("expected build-time error for nil field value, got nil")
	}
	t.Logf("PASS: nil field value rejected at build time: %v", err)
}

// --- LowerArithmetic / LowerCompare (numeric intent) ------------------

// TestLowerArithmetic_SignedDivIsBare — signed-intent div is the
// bare cb.Arithmetic — no cast wrapping. Asserts via content-hash
// equivalence (the load-bearing invariant: SignedIntent produces
// the same IR a frontend would write by hand).
func TestLowerArithmetic_SignedDivIsBare(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	c := ap.Compute()
	lowered := entitysdk.LowerArithmetic(c, entitysdk.SignedIntent, "div",
		c.Literal(int64(-10)), c.Literal(int64(3)))
	handwritten := c.Arithmetic("div", c.Literal(int64(-10)), c.Literal(int64(3)))

	hL, err := lowered.Build(context.Background(), "app/lower/arith-signed/lowered")
	if err != nil {
		t.Fatalf("build lowered: %v", err)
	}
	hH, err := handwritten.Build(context.Background(), "app/lower/arith-signed/handwritten")
	if err != nil {
		t.Fatalf("build handwritten: %v", err)
	}
	if hL != hH {
		t.Fatalf("expected identical hashes (SignedIntent should be bare); got lowered=%s handwritten=%s", hL, hH)
	}
	t.Logf("PASS: SignedIntent div produces bare Arithmetic (hash %s) — no surprise cast", hL)
}

// TestLowerArithmetic_UnsignedDivWrapsBothOperands — unsigned div
// inlines NumericCast on both operands at the operand site, per
// Rule 11 and the foundational guide §9 worked example. Asserts the
// IR shape directly: root is Arithmetic[div]; each child is
// NumericCast[primitive/uint] over a Literal.
func TestLowerArithmetic_UnsignedDivWrapsBothOperands(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	pattern := "app/lower/arith-unsigned-div"
	c := ap.Compute()
	// Use 10/2=5 (exact division). When div has a remainder, the
	// evaluator returns a float64 (eval.go:1250 for unsigned, :1256 for
	// signed). The test cares about IR shape + intent dispatch, not the
	// remainder edge case, so we pick a clean-division pair.
	expr := entitysdk.LowerArithmetic(c, entitysdk.UnsignedIntent, "div",
		c.Literal(uint64(10)), c.Literal(uint64(2)))

	h, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: pattern,
		Name:    "lower-arith-unsigned-div",
		Operations: map[string]types.HandlerOperationSpec{
			"compute": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, expr)
	if err != nil {
		t.Fatalf("RegisterComputeHandler: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	// --- runtime: 10 / 3 = 3 under unsigned (same as signed for these positive values) ---
	params, _ := entitysdk.PrimitiveAny(map[string]interface{}{})
	resp, err := ap.Executor().ExecuteWithParams(pattern, "compute", params)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if resp.Status != 200 {
		t.Fatalf("status %d (type=%s)", resp.Status, resp.Type)
	}
	got := decodeAny(t, resp)
	if !numEq(got, 5) {
		t.Fatalf("runtime: expected 5, got %v (%T)", got, got)
	}

	// --- translation: root Arithmetic[div]; each child NumericCast[uint] ---
	root := loadRoot(t, ap, pattern)
	if root.Type != types.TypeComputeArithmetic {
		t.Fatalf("root: expected %q, got %q", types.TypeComputeArithmetic, root.Type)
	}
	var ar types.ComputeArithmeticData
	mustDecode(t, root.Data, &ar)
	if ar.Op != "div" {
		t.Fatalf("Arithmetic.Op: expected div, got %q", ar.Op)
	}

	leftChild, ok := ap.Store().GetByHash(ar.Left)
	if !ok {
		t.Fatalf("left child not found")
	}
	if leftChild.Type != types.TypeComputeNumericCast {
		t.Fatalf("left child: expected %q, got %q (Rule 11 cast not inlined)", types.TypeComputeNumericCast, leftChild.Type)
	}
	var leftCast types.ComputeNumericCastData
	mustDecode(t, leftChild.Data, &leftCast)
	if leftCast.ToType != "primitive/uint" {
		t.Fatalf("left NumericCast.ToType: expected primitive/uint, got %q", leftCast.ToType)
	}

	rightChild, ok := ap.Store().GetByHash(ar.Right)
	if !ok {
		t.Fatalf("right child not found")
	}
	if rightChild.Type != types.TypeComputeNumericCast {
		t.Fatalf("right child: expected %q, got %q (Rule 11 cast not inlined on right)", types.TypeComputeNumericCast, rightChild.Type)
	}

	t.Logf("PASS: UnsignedIntent div = Arithmetic[div]→{NumericCast[uint]→Literal, NumericCast[uint]→Literal}; matches guide §9 worked example")
}

// TestLowerArithmetic_UnsignedAddIsBare — for sign-agnostic ops
// (add/sub/mul), UnsignedIntent should NOT add a cast — the bit-
// level semantics are identical for signed and unsigned, and
// adding a cast would be noise that breaks content-hash equality
// across intents. Asserts via hash equivalence to bare Arithmetic.
func TestLowerArithmetic_UnsignedAddIsBare(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	c := ap.Compute()
	unsignedAdd := entitysdk.LowerArithmetic(c, entitysdk.UnsignedIntent, "add",
		c.Literal(uint64(7)), c.Literal(uint64(8)))
	bare := c.Arithmetic("add", c.Literal(uint64(7)), c.Literal(uint64(8)))

	hU, err := unsignedAdd.Build(context.Background(), "app/lower/arith-add-unsigned/lowered")
	if err != nil {
		t.Fatalf("build unsigned: %v", err)
	}
	hB, err := bare.Build(context.Background(), "app/lower/arith-add-unsigned/bare")
	if err != nil {
		t.Fatalf("build bare: %v", err)
	}
	if hU != hB {
		t.Fatalf("expected identical hash (add is sign-agnostic; no cast needed); got unsigned=%s bare=%s", hU, hB)
	}
	t.Logf("PASS: UnsignedIntent add produces bare Arithmetic (sign-agnostic; hash %s)", hU)
}

// TestLowerArithmetic_SignedVsUnsignedDivDiffer — runtime proof
// that intent matters for div: a negative dividend interpreted as
// unsigned is a huge number, so the quotient flips drastically.
// This is the test that proves the toolkit's intent dispatch
// translates to actual behavioral difference at runtime.
func TestLowerArithmetic_SignedVsUnsignedDivDiffer(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	// Build two compute handlers: one signed div, one unsigned div,
	// both on (-2, 2). Signed: -2/2 = -1 (exact). Unsigned:
	// cast(-2, uint) = max-u64 - 1; (max-u64 - 1)/2 = (max-u64-1)/2.
	// Both divisions are exact so the evaluator returns integers
	// (eval.go:1248 unsigned, :1254 signed).
	c := ap.Compute()

	signedPattern := "app/lower/arith-cmp-signed"
	signedExpr := entitysdk.LowerArithmetic(c, entitysdk.SignedIntent, "div",
		c.Literal(int64(-2)), c.Literal(int64(2)))
	hSigned, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: signedPattern,
		Name:    "lower-arith-cmp-signed",
		Operations: map[string]types.HandlerOperationSpec{
			"compute": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, signedExpr)
	if err != nil {
		t.Fatalf("register signed: %v", err)
	}
	t.Cleanup(func() { _ = hSigned.Close() })

	unsignedPattern := "app/lower/arith-cmp-unsigned"
	unsignedExpr := entitysdk.LowerArithmetic(c, entitysdk.UnsignedIntent, "div",
		c.Literal(int64(-2)), c.Literal(int64(2)))
	hUnsigned, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: unsignedPattern,
		Name:    "lower-arith-cmp-unsigned",
		Operations: map[string]types.HandlerOperationSpec{
			"compute": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, unsignedExpr)
	if err != nil {
		t.Fatalf("register unsigned: %v", err)
	}
	t.Cleanup(func() { _ = hUnsigned.Close() })

	params, _ := entitysdk.PrimitiveAny(map[string]interface{}{})

	respSigned, err := ap.Executor().ExecuteWithParams(signedPattern, "compute", params)
	if err != nil {
		t.Fatalf("dispatch signed: %v", err)
	}
	if respSigned.Status != 200 {
		t.Fatalf("signed status %d", respSigned.Status)
	}
	gotSigned := decodeAny(t, respSigned)

	respUnsigned, err := ap.Executor().ExecuteWithParams(unsignedPattern, "compute", params)
	if err != nil {
		t.Fatalf("dispatch unsigned: %v", err)
	}
	if respUnsigned.Status != 200 {
		t.Fatalf("unsigned status %d", respUnsigned.Status)
	}
	gotUnsigned := decodeAny(t, respUnsigned)

	// Signed: -2 / 2 = -1 (exact int division).
	if !numEq(gotSigned, -1) {
		t.Fatalf("signed -2/2: expected -1, got %v (%T)", gotSigned, gotSigned)
	}
	// Unsigned: cast(-2, uint) = max-u64-1 = 0xFFFFFFFFFFFFFFFE;
	// /2 = 0x7FFFFFFFFFFFFFFF = 9223372036854775807.
	gotUnsignedU, ok := normalizeToUint64(gotUnsigned)
	if !ok {
		t.Fatalf("unsigned result not numeric: %v (%T)", gotUnsigned, gotUnsigned)
	}
	const expectedUnsigned = uint64(0x7FFFFFFFFFFFFFFF)
	if gotUnsignedU != expectedUnsigned {
		t.Fatalf("unsigned -2/2 (cast to uint = (max-u64-1)/2): expected %d, got %d", expectedUnsigned, gotUnsignedU)
	}

	t.Logf("PASS: signed div(-2,2)=-1; unsigned div(cast(-2,uint),2)=%d; intent dispatch flips runtime behavior as Rule 11 promises", gotUnsignedU)
}

// TestLowerCompare_UnsignedLtWrapsCasts — symmetric test for
// LowerCompare. Asserts the IR shape: Compare[lt]→{NumericCast,NumericCast}.
func TestLowerCompare_UnsignedLtWrapsCasts(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	c := ap.Compute()
	expr := entitysdk.LowerCompare(c, entitysdk.UnsignedIntent, "lt",
		c.Literal(int64(-1)), c.Literal(uint64(100)))

	_, err = expr.Build(context.Background(), "app/lower/cmp-unsigned/expr")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	root, _, err := ap.Get("app/lower/cmp-unsigned/expr")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if root.Type != types.TypeComputeCompare {
		t.Fatalf("root: expected %q, got %q", types.TypeComputeCompare, root.Type)
	}
	var cm types.ComputeCompareData
	mustDecode(t, root.Data, &cm)
	if cm.Op != "lt" {
		t.Fatalf("Compare.Op: expected lt, got %q", cm.Op)
	}
	left, ok := ap.Store().GetByHash(cm.Left)
	if !ok || left.Type != types.TypeComputeNumericCast {
		t.Fatalf("left: expected NumericCast, got %q (found=%v)", left.Type, ok)
	}
	right, ok := ap.Store().GetByHash(cm.Right)
	if !ok || right.Type != types.TypeComputeNumericCast {
		t.Fatalf("right: expected NumericCast, got %q (found=%v)", right.Type, ok)
	}
	t.Logf("PASS: UnsignedIntent compare = Compare[lt]→{NumericCast[uint], NumericCast[uint]}")
}

// normalizeToUint64 coerces numeric results out of the CBOR decoder
// to uint64 for comparison against large unsigned values where
// float conversion would lose precision.
func normalizeToUint64(v interface{}) (uint64, bool) {
	switch n := v.(type) {
	case uint64:
		return n, true
	case int64:
		if n < 0 {
			return uint64(n), true // two's-complement reinterpret; intentional
		}
		return uint64(n), true
	case float64:
		return uint64(n), true
	}
	return 0, false
}

// --- LowerFold (iteration over a collection) -------------------------

// TestLowerFold_SumList — the canonical use: sum params.numbers via
// fold(acc + elem, init=0). Runtime returns the expected sum; IR is
// BuiltinsCall("fold") with a Lambda child for fn.
func TestLowerFold_SumList(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	pattern := "app/lower/fold-sum-list"
	c := ap.Compute()
	expr := entitysdk.LowerFold(c,
		c.Field(c.LookupScope("params"), "numbers"),
		c.Literal(uint64(0)),
		func(acc, elem *entitysdk.Builder) *entitysdk.Builder {
			return c.Arithmetic("add", acc, elem)
		},
	)

	h, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: pattern,
		Name:    "lower-fold-sum",
		Operations: map[string]types.HandlerOperationSpec{
			"compute": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, expr)
	if err != nil {
		t.Fatalf("RegisterComputeHandler: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	// --- runtime: sum([1,2,3,4,5]) = 15 ---
	params, _ := entitysdk.PrimitiveAny(map[string]interface{}{
		"numbers": []interface{}{uint64(1), uint64(2), uint64(3), uint64(4), uint64(5)},
	})
	resp, err := ap.Executor().ExecuteWithParams(pattern, "compute", params)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if resp.Status != 200 {
		t.Fatalf("status %d (type=%s)", resp.Status, resp.Type)
	}
	got := decodeAny(t, resp)
	if !numEq(got, 15) {
		t.Fatalf("runtime: expected 15, got %v (%T)", got, got)
	}

	// --- translation: root is compute/apply (BuiltinsCall is sugar over
	//     Apply at system/compute/builtins/fold); args contain a Lambda. ---
	root := loadRoot(t, ap, pattern)
	if root.Type != types.TypeComputeApply {
		t.Fatalf("root: expected %q (BuiltinsCall is Apply), got %q", types.TypeComputeApply, root.Type)
	}
	var ad types.ComputeApplyData
	mustDecode(t, root.Data, &ad)
	if ad.Path != "system/compute/builtins/fold" {
		t.Fatalf("Apply.Path: expected fold path, got %q", ad.Path)
	}
	fnHash, ok := ad.Args["fn"]
	if !ok {
		t.Fatalf("Apply.Args missing 'fn'")
	}
	fnChild, ok := ap.Store().GetByHash(fnHash)
	if !ok {
		t.Fatalf("fn child not found")
	}
	if fnChild.Type != types.TypeComputeLambda {
		t.Fatalf("fn child: expected %q (pre-evaluated closure is non-portable per pitfall 4), got %q",
			types.TypeComputeLambda, fnChild.Type)
	}
	t.Logf("PASS: LowerFold(sum) sums [1..5] to 15; IR is Apply[system/compute/builtins/fold] with Lambda fn — matches guide §9 collection-iteration pattern")
}

// TestLowerFold_EmptyCollection — fold over [] returns the initial
// value unchanged. Edge case from N.1; confirms toolkit doesn't trip
// on the empty case.
func TestLowerFold_EmptyCollection(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	pattern := "app/lower/fold-empty"
	c := ap.Compute()
	expr := entitysdk.LowerFold(c,
		c.Field(c.LookupScope("params"), "xs"),
		c.Literal(uint64(42)),
		func(acc, elem *entitysdk.Builder) *entitysdk.Builder {
			return c.Arithmetic("add", acc, elem)
		},
	)
	h, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: pattern,
		Name:    "lower-fold-empty",
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
		t.Fatalf("status %d", resp.Status)
	}
	got := decodeAny(t, resp)
	if !numEq(got, 42) {
		t.Fatalf("runtime: expected init=42, got %v (%T)", got, got)
	}
	t.Logf("PASS: LowerFold([], init=42, +) = 42 (initial returned unchanged for empty)")
}

// TestLowerFold_ProductOfLiteralList — fold doesn't require params
// access; the collection can be a literal. Demonstrates fold over a
// hand-built array. Computes 2*3*4 = 24 with init=1.
func TestLowerFold_ProductOfLiteralList(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	pattern := "app/lower/fold-product"
	c := ap.Compute()
	expr := entitysdk.LowerFold(c,
		c.Literal([]interface{}{uint64(2), uint64(3), uint64(4)}),
		c.Literal(uint64(1)),
		func(acc, elem *entitysdk.Builder) *entitysdk.Builder {
			return c.Arithmetic("mul", acc, elem)
		},
	)
	h, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: pattern,
		Name:    "lower-fold-product",
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
		t.Fatalf("status %d", resp.Status)
	}
	got := decodeAny(t, resp)
	if !numEq(got, 24) {
		t.Fatalf("runtime: expected 24, got %v (%T)", got, got)
	}
	t.Logf("PASS: LowerFold(literal [2,3,4], init=1, *) = 24")
}

// --- LowerFilter / LowerMap -------------------------------------------

// TestLowerFilter_LiteralPredicate — filter to elements > 5,
// hardcoded threshold. Asserts runtime + IR shape (root is
// BuiltinsCall("filter") with a Lambda predicate under arg key
// "fn" per EXTENSION-COMPUTE v3.19 — F11 unified the lambda arg
// name across map/filter/fold).
func TestLowerFilter_LiteralPredicate(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	pattern := "app/lower/filter-literal"
	c := ap.Compute()
	expr := entitysdk.LowerFilter(c,
		c.Field(c.LookupScope("params"), "numbers"),
		func(elem *entitysdk.Builder) *entitysdk.Builder {
			return c.Compare("gt", elem, c.Literal(uint64(5)))
		},
	)
	h, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: pattern,
		Name:    "lower-filter-literal",
		Operations: map[string]types.HandlerOperationSpec{
			"compute": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, expr)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	params, _ := entitysdk.PrimitiveAny(map[string]interface{}{
		"numbers": []interface{}{uint64(1), uint64(3), uint64(7), uint64(10)},
	})
	resp, err := ap.Executor().ExecuteWithParams(pattern, "compute", params)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if resp.Status != 200 {
		t.Fatalf("status %d", resp.Status)
	}
	var got []interface{}
	if err := ecf.Decode(resp.Data, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 || !numEq(got[0], 7) || !numEq(got[1], 10) {
		t.Fatalf("expected [7, 10], got %v", got)
	}
	t.Logf("PASS: LowerFilter(>5) on [1,3,7,10] = [7,10]")
}

// TestLowerMap_DoubleEachElement — map(*2) over a list.
func TestLowerMap_DoubleEachElement(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	pattern := "app/lower/map-double"
	c := ap.Compute()
	expr := entitysdk.LowerMap(c,
		c.Field(c.LookupScope("params"), "numbers"),
		func(elem *entitysdk.Builder) *entitysdk.Builder {
			return c.Arithmetic("mul", elem, c.Literal(uint64(2)))
		},
	)
	h, err := ap.RegisterComputeHandler(context.Background(), entitysdk.HandlerSpec{
		Pattern: pattern,
		Name:    "lower-map-double",
		Operations: map[string]types.HandlerOperationSpec{
			"compute": {InputType: "primitive/any", OutputType: "primitive/any"},
		},
	}, expr)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	params, _ := entitysdk.PrimitiveAny(map[string]interface{}{
		"numbers": []interface{}{uint64(1), uint64(2), uint64(3)},
	})
	resp, err := ap.Executor().ExecuteWithParams(pattern, "compute", params)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	var got []interface{}
	if err := ecf.Decode(resp.Data, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 3 || !numEq(got[0], 2) || !numEq(got[1], 4) || !numEq(got[2], 6) {
		t.Fatalf("expected [2,4,6], got %v", got)
	}
	t.Logf("PASS: LowerMap(*2) on [1,2,3] = [2,4,6]")
}

// --- helpers ----------------------------------------------------------

// assertRecordEquals compares a decoded record value against an
// expected map. CBOR decode produces either map[string]interface{}
// or map[interface{}]interface{} depending on the decoder; this
// helper normalizes the comparison.
func assertRecordEquals(t *testing.T, got interface{}, want map[string]interface{}) {
	t.Helper()
	switch m := got.(type) {
	case map[string]interface{}:
		for k, wv := range want {
			gv, ok := m[k]
			if !ok {
				t.Fatalf("record missing field %q (got: %+v)", k, m)
			}
			if !recordValueEqual(gv, wv) {
				t.Fatalf("field %q: expected %v (%T), got %v (%T)", k, wv, wv, gv, gv)
			}
		}
	case map[interface{}]interface{}:
		for k, wv := range want {
			gv, ok := m[k]
			if !ok {
				t.Fatalf("record missing field %q (got: %+v)", k, m)
			}
			if !recordValueEqual(gv, wv) {
				t.Fatalf("field %q: expected %v (%T), got %v (%T)", k, wv, wv, gv, gv)
			}
		}
	default:
		t.Fatalf("expected a map, got %T: %v", got, got)
	}
}

// recordValueEqual handles the small set of value types that travel
// through CBOR + the integer normalization numEq does for numeric
// fields. Strings compare as strings; bools as bools; numerics via
// numEq.
func recordValueEqual(got, want interface{}) bool {
	switch w := want.(type) {
	case uint64:
		return numEq(got, float64(w))
	case int64:
		return numEq(got, float64(w))
	case float64:
		return numEq(got, w)
	case string:
		s, ok := got.(string)
		return ok && s == w
	case bool:
		b, ok := got.(bool)
		return ok && b == w
	}
	return got == want
}
