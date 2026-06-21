package entitysdk

// Lowering toolkit (Phase H.2) — the reusable functional decompositions
// the foundational guide names as the layer ABOVE the IR-assembly SDK.
//
// Per GUIDE-CORE-COMPUTATIONAL-ARCHITECTURE.md §8: the build past the
// SDK is *not* a language. It is the lowering toolkit — the small,
// shared functional patterns every frontend will compile down to when
// targeting compute. Loop→TCO, record access, signed/unsigned-intent
// via Rule 11, eventually compute/match. Each decomposition lives here
// as a free function over *ComputeBuilder; together they form the
// "machinery many frontends share."
//
// Style note: these are free functions, not methods on ComputeBuilder.
// The toolkit is a layer above S1; mixing the two would blur the seam
// the guide §8 draws. If the toolkit grows large enough that namespace
// matters, introduce a Lower wrapper type — until then, free functions
// keep call sites short and the layering honest.

import (
	"context"
	"fmt"
	"sort"
)

// NumericIntent is the lowering toolkit's representation of signed-
// versus-unsigned numeric semantics. Per v3.18 integer model: add/sub/
// mul are sign-agnostic (wraparound semantics are the same for both),
// so SignedIntent and UnsignedIntent produce identical IR for those
// ops. For div/mod/compare (signed-default), UnsignedIntent inlines a
// numeric-cast to primitive/uint at the operand site per Rule 11.
//
// Frontends use this to thread their source-language type's signedness
// through to the IR without writing the cast machinery themselves.
type NumericIntent int

const (
	// SignedIntent — bare arithmetic / compare with no cast wrapping.
	// Matches the v3.18 signed-default behavior.
	SignedIntent NumericIntent = iota

	// UnsignedIntent — for div/mod/compare, wraps each operand in
	// NumericCast(_, "primitive/uint") at the operand site so Rule 11
	// makes the operation unsigned. For sign-agnostic ops (add/sub/mul)
	// emits the bare op since the bit-level semantics are identical.
	UnsignedIntent
)

func (i NumericIntent) String() string {
	switch i {
	case SignedIntent:
		return "signed"
	case UnsignedIntent:
		return "unsigned"
	default:
		return "unknown"
	}
}

// signedDefaultOps lists the ops whose default interpretation is
// signed and where UnsignedIntent therefore requires the Rule 11
// numeric-cast inlining. add/sub/mul are sign-agnostic and absent
// from this set.
var signedDefaultArithmeticOps = map[string]bool{
	"div": true,
	"mod": true,
}

// LowerArithmetic builds compute/arithmetic with the cast machinery
// for the given intent inlined per Rule 11. Op is the arithmetic
// operation ("add", "sub", "mul", "div", "mod"). For sign-agnostic
// ops, the function emits a bare cb.Arithmetic regardless of intent
// (the bit-level behavior is identical and adding a cast would be
// noise). For div/mod under UnsignedIntent, each operand is wrapped
// in a NumericCast to primitive/uint at the operand site — the cast
// is the *direct operand* of the arithmetic per Rule 11.
//
// The point of this function: a frontend lowering a host-language
// arithmetic site says intent + op + operands; the toolkit decides
// when Rule 11's cast is needed and emits the canonical IR. The
// caller never has to know Rule 11 to write correct unsigned
// arithmetic.
//
//	// Source (Rust-style):  let y: u32 = x / 2;
//	// Toolkit call:
//	expr := LowerArithmetic(cb, UnsignedIntent, "div", xBuilder, cb.Literal(uint64(2)))
//	// IR: Arithmetic("div", NumericCast(xBuilder, "primitive/uint"),
//	//                       NumericCast(cb.Literal(2), "primitive/uint"))
func LowerArithmetic(cb *ComputeBuilder, intent NumericIntent, op string, left, right *Builder) *Builder {
	if cb == nil {
		return nil
	}
	if left == nil || right == nil {
		return errBuilder(cb, kindArithmetic, "compute/arithmetic",
			fmt.Errorf("LowerArithmetic: nil operand (left=%v right=%v)", left, right))
	}
	if intent == UnsignedIntent && signedDefaultArithmeticOps[op] {
		left = cb.NumericCast(left, "primitive/uint")
		right = cb.NumericCast(right, "primitive/uint")
	}
	return cb.Arithmetic(op, left, right)
}

// LowerFold builds an iteration over a collection — the practical
// "loop" pattern for compute. Per the foundational guide §9
// ("Collection iteration → the stdlib"), iteration over a CBOR array
// lowers to `system/compute/builtins/fold` with a (acc, elem)
// lambda. This is the canonical loop-over-collection shape for any
// frontend that doesn't need self-referential recursion (which is
// the rarer pattern, parked as future work).
//
// Parameters:
//
//   - collection — a *Builder producing the CBOR array to iterate
//   - initial    — a *Builder producing the starting accumulator
//   - step       — a Go callback that receives placeholder *Builders
//     for acc and elem and returns the body expression
//     for one iteration. The toolkit wraps the body in
//     a Lambda(["acc","elem"], body); the lambda is then
//     invoked per element by fold.
//
// Example — summing a list of params.numbers:
//
//	expr := LowerFold(
//	    cb,
//	    cb.Field(cb.LookupScope("params"), "numbers"),
//	    cb.Literal(uint64(0)),
//	    func(acc, elem *Builder) *Builder {
//	        return cb.Arithmetic("add", acc, elem)
//	    },
//	)
//
// The IR is `BuiltinsCall("fold", {collection, initial, fn:Lambda})`;
// the fold builtin's evaluator (`ext/compute/builtins.go::builtinFold`)
// threads initial through fn(acc, element) left-to-right.
//
// For the rarer self-referential recursive pattern (loops not over a
// pre-built collection — e.g., factorial via lambda-self-call), see
// the future LowerRecurse / LowerTCO when those decompositions land.
// The guide §9 names "loops → recursion + TCO" as the shape; this
// fold-based LowerLoop covers the common iteration case without
// the complexity of self-reference plumbing.
func LowerFold(cb *ComputeBuilder, collection, initial *Builder, step func(acc, elem *Builder) *Builder) *Builder {
	if cb == nil {
		return nil
	}
	if collection == nil || initial == nil {
		return errBuilder(cb, kindBuiltinsCall, "compute/apply",
			fmt.Errorf("LowerFold: nil collection or initial"))
	}
	if step == nil {
		return errBuilder(cb, kindBuiltinsCall, "compute/apply",
			fmt.Errorf("LowerFold: nil step function"))
	}
	accRef := cb.LookupScope("acc")
	elemRef := cb.LookupScope("elem")
	body := step(accRef, elemRef)
	if body == nil {
		return errBuilder(cb, kindBuiltinsCall, "compute/apply",
			fmt.Errorf("LowerFold: step returned nil body"))
	}
	lam := cb.Lambda([]string{"acc", "elem"}, body)
	return cb.BuiltinsCall("fold", map[string]*Builder{
		"collection": collection,
		"initial":    initial,
		"fn":         lam,
	})
}

// LowerFilter builds a filter over a collection — selects elements
// for which the predicate returns truthy. Counterpart to LowerFold
// for the single-param-lambda case.
//
// Parameters:
//   - collection — a *Builder producing the CBOR array to filter
//   - predicate  — a Go callback that receives a placeholder *Builder
//     for `elem` and returns the boolean-shaped expression
//     that decides whether to keep this element. The
//     toolkit wraps it in Lambda(["elem"], body).
//
// Hides F11 friction (BuiltinsCall arg-name inconsistency: filter
// uses "predicate", fold/map use "fn"). Authors writing
// LowerFilter / LowerMap / LowerFold see a uniform shape; the
// underlying spec-arg-name asymmetry is invisible.
//
// Example — filter to elements > 5:
//
//	expr := LowerFilter(
//	    cb,
//	    cb.Field(cb.LookupScope("params"), "numbers"),
//	    func(elem *Builder) *Builder {
//	        return cb.Compare("gt", elem, cb.Literal(uint64(5)))
//	    },
//	)
//
// **F9 secondary manifestation:** if `predicate` references scope-
// typed values via closure capture (e.g., `cb.LookupScope("params")`
// inside the body), the runtime fails — the captured params loses
// its entity.Entity type on the CBOR scope round-trip, and the
// lambda body's Field on it hits evalField's bare-map rejection.
// Hardcoded predicates work; closures over scope don't. Closes
// when F9 lands in core-go.
func LowerFilter(cb *ComputeBuilder, collection *Builder, predicate func(elem *Builder) *Builder) *Builder {
	if cb == nil {
		return nil
	}
	if collection == nil {
		return errBuilder(cb, kindBuiltinsCall, "compute/apply",
			fmt.Errorf("LowerFilter: nil collection"))
	}
	if predicate == nil {
		return errBuilder(cb, kindBuiltinsCall, "compute/apply",
			fmt.Errorf("LowerFilter: nil predicate"))
	}
	elemRef := cb.LookupScope("elem")
	body := predicate(elemRef)
	if body == nil {
		return errBuilder(cb, kindBuiltinsCall, "compute/apply",
			fmt.Errorf("LowerFilter: predicate returned nil body"))
	}
	lam := cb.Lambda([]string{"elem"}, body)
	return cb.BuiltinsCall("filter", map[string]*Builder{
		"collection": collection,
		// Post-v3.19 (F11): map/filter/fold all use "fn" as the
		// lambda arg name. Prior to v3.19 filter used "predicate".
		"fn": lam,
	})
}

// LowerMap builds a map over a collection — transforms each element.
// Counterpart to LowerFilter / LowerFold for the single-param
// "transform per element" case.
//
// Parameters:
//   - collection — a *Builder producing the CBOR array to map over
//   - transform  — a Go callback that receives a placeholder *Builder
//     for `elem` and returns the body that produces the
//     transformed element. The toolkit wraps it in
//     Lambda(["elem"], body).
//
// Example — double each element:
//
//	expr := LowerMap(
//	    cb,
//	    cb.Field(cb.LookupScope("params"), "numbers"),
//	    func(elem *Builder) *Builder {
//	        return cb.Arithmetic("mul", elem, cb.Literal(uint64(2)))
//	    },
//	)
//
// Same F9 secondary manifestation as LowerFilter for closure-
// captured scope values.
func LowerMap(cb *ComputeBuilder, collection *Builder, transform func(elem *Builder) *Builder) *Builder {
	if cb == nil {
		return nil
	}
	if collection == nil {
		return errBuilder(cb, kindBuiltinsCall, "compute/apply",
			fmt.Errorf("LowerMap: nil collection"))
	}
	if transform == nil {
		return errBuilder(cb, kindBuiltinsCall, "compute/apply",
			fmt.Errorf("LowerMap: nil transform"))
	}
	elemRef := cb.LookupScope("elem")
	body := transform(elemRef)
	if body == nil {
		return errBuilder(cb, kindBuiltinsCall, "compute/apply",
			fmt.Errorf("LowerMap: transform returned nil body"))
	}
	lam := cb.Lambda([]string{"elem"}, body)
	return cb.BuiltinsCall("map", map[string]*Builder{
		"collection": collection,
		"fn":         lam,
	})
}

// LowerCompare builds compute/compare with the Rule 11 cast inlined
// for UnsignedIntent. Compare ops ("eq", "ne", "lt", "le", "gt",
// "ge") are signed-default per the v3.18 integer model; the
// unsigned variant casts each operand to primitive/uint at the
// operand site. SignedIntent produces a bare cb.Compare.
//
// "eq" and "ne" are technically sign-agnostic (the bit patterns
// either match or don't), but for consistency with div/mod we still
// emit the cast under UnsignedIntent — it's a no-op for those ops
// at runtime and keeps the toolkit's behavior uniform. (Frontends
// that want bare equality can call cb.Compare directly.)
//
//	// Source: if (x: u32) < 100u32 { ... }
//	// Toolkit call:
//	cond := LowerCompare(cb, UnsignedIntent, "lt", xBuilder, cb.Literal(uint64(100)))
func LowerCompare(cb *ComputeBuilder, intent NumericIntent, op string, left, right *Builder) *Builder {
	if cb == nil {
		return nil
	}
	if left == nil || right == nil {
		return errBuilder(cb, kindCompare, "compute/compare",
			fmt.Errorf("LowerCompare: nil operand (left=%v right=%v)", left, right))
	}
	if intent == UnsignedIntent {
		left = cb.NumericCast(left, "primitive/uint")
		right = cb.NumericCast(right, "primitive/uint")
	}
	return cb.Compare(op, left, right)
}

// LowerRecurse builds a recursive lambda + its first invocation via
// closure-mode apply (ApplyClosure). The pure-hash fixpoint pattern
// from GUIDE-CORE-COMPUTATIONAL-ARCHITECTURE §8.2/§9: store the
// lambda at a stable tree path; the body references that same path
// via LookupTree to call itself; the evaluator's TCO trampolines
// tail-position self-calls without consuming evaluator depth.
//
// The toolkit handles the fixpoint-bootstrap knot ("need the hash to
// build the body, need the body to build the hash") mechanically.
// Resolves the gap left open in workbench's original H.2 toolkit per
// PROPOSAL-COMPUTE-RECURSION-AND-SUM-TYPES §2.
//
// Parameters:
//   - ctx        — context for the lambda's Build call
//   - lambdaPath — tree path where the recursive lambda is materialized;
//     `self` inside body resolves to LookupTree(lambdaPath, false).
//     Caller chooses the path; it must be stable across builds for
//     the recursion to be reachable.
//   - params     — names of the lambda's parameters
//   - body       — Go callback receiving (self, params...) builders;
//     returns the body expression. `self` is a LookupTree pointer at
//     lambdaPath; call it via cb.ApplyClosure(self, args).
//
// Returns: a *Builder for an ApplyClosure invoking the recursive
// lambda. Caller decides how to invoke (provide initial-args via the
// returned builder's invocation context, or compose into a larger IR).
// The lambda itself is materialized at lambdaPath as a side-effect
// of LowerRecurse — the toolkit handles the "build the body, then
// store the lambda" sequencing.
//
// Example — factorial:
//
//	expr := entitysdk.LowerRecurse(ctx, ap, "app/recurse/factorial",
//	    []string{"n"},
//	    func(self, n *entitysdk.Builder) *entitysdk.Builder {
//	        return c.If(
//	            c.Compare("eq", n, c.Literal(uint64(0))),
//	            c.Literal(uint64(1)),
//	            c.Arithmetic("mul", n,
//	                c.ApplyClosure(self, map[string]*entitysdk.Builder{
//	                    "n": c.Arithmetic("sub", n, c.Literal(uint64(1))),
//	                })),
//	        )
//	    })
//
// Invocation at the lambdaPath site (e.g. via compute aggregate or as
// a registered handler's expression_path) supplies the initial param.
func LowerRecurse(ctx context.Context, ap *AppPeer, lambdaPath string, params []string, body func(self *Builder, args ...*Builder) *Builder) (*Builder, error) {
	if ap == nil {
		return nil, fmt.Errorf("LowerRecurse: nil AppPeer")
	}
	if lambdaPath == "" {
		return nil, fmt.Errorf("LowerRecurse: empty lambdaPath")
	}
	if len(params) == 0 {
		return nil, fmt.Errorf("LowerRecurse: at least one param required")
	}
	if body == nil {
		return nil, fmt.Errorf("LowerRecurse: nil body function")
	}
	cb := ap.Compute()

	// Build the lambda whose body references LookupTree(lambdaPath)
	// for self-recursion. The body callback receives `self` as that
	// LookupTree builder + placeholder LookupScope builders for each
	// param name (the lambda's local scope).
	self := cb.LookupTree(lambdaPath, false)
	paramRefs := make([]*Builder, len(params))
	for i, p := range params {
		paramRefs[i] = cb.LookupScope(p)
	}
	bodyExpr := body(self, paramRefs...)
	if bodyExpr == nil {
		return nil, fmt.Errorf("LowerRecurse: body callback returned nil")
	}
	lambda := cb.Lambda(params, bodyExpr)
	if _, err := lambda.Build(ctx, lambdaPath); err != nil {
		return nil, fmt.Errorf("LowerRecurse: build lambda at %s: %w", lambdaPath, err)
	}

	// Return a closure-mode Apply that the caller will invoke with
	// initial args. The caller composes this into the surrounding
	// expression (typically inside a RegisterComputeHandler whose
	// scope.params supplies the initial values).
	//
	// The returned builder is ApplyClosure(LookupTree(lambdaPath),
	// args) — caller binds args by composing on top, e.g.:
	//   wrap = c.ApplyClosure(c.LookupTree(lambdaPath, false), map[string]*Builder{
	//       "n": c.Field(c.LookupScope("params"), "n"),
	//   })
	// LowerRecurse returns a placeholder for that pattern; the caller
	// can either use it directly with a closure (for a top-level
	// invocation with literal args) or rebuild the ApplyClosure with
	// the args they need.
	//
	// Most flexible: return the lambda's self-reference (a LookupTree
	// builder). Caller wraps with ApplyClosure(returned, args).
	return cb.LookupTree(lambdaPath, false), nil
}

// LowerMatch builds a sum-type discriminator: lowers a `match value
// { tag1 => body1, tag2 => body2, ..., _ => default }` pattern to a
// right-nested if/eq chain on `field(value, tagField)`. The matched
// value is bound for each arm so the arm body can navigate into it.
//
// Per PROPOSAL-COMPUTE-RECURSION-AND-SUM-TYPES §3, this is the
// entity-native sum-type pattern (no spec change needed). A variant
// is lowered to an entity with a discriminant field inside `.data`;
// LowerMatch reads that field via Field(value, tagField) and branches.
// Closes the standing 4-B gap; unblocks Rust enum/Result/Option
// lowering immediately without v3.20 spec work.
//
// Parameters:
//   - value     — the *Builder producing the value to discriminate.
//   - tagField  — the field name inside the value's data holding the
//     discriminant (typically "$variant" or similar).
//   - arms      — map of tag-value → arm-body builder. Each arm-body
//     receives the matched value as its only Builder argument so it
//     can do further Field nav (e.g. `Some(x) => x + 1` reads `value`).
//   - defaultArm — optional; receives the matched value. If nil and no
//     tag matches at eval time, the IR returns a compute/error via
//     a default `compute/error/no_match` value (see body for shape).
//     Frontends with static exhaustiveness can pass nil safely; frontends
//     that need runtime match defaults pass a real defaultArm.
//
// Limitations (vs a hypothetical v3.20 `compute/match` primitive):
//   - No static exhaustiveness checking — the runtime knows nothing
//     about which tags the value's type CAN have; a missing arm is a
//     runtime no-match, not a build error.
//   - Discrimination is on a `.data` tag, not the entity's `.type`. If
//     `compute/field` someday gets a `type-of`-style reader, that
//     becomes the cleaner shape (no redundant tag); until then this
//     pattern is the canonical workbench-side answer.
//
// Example — Option<T>:
//
//	expr := entitysdk.LowerMatch(c, value, "$variant", map[string]func(*entitysdk.Builder) *entitysdk.Builder{
//	    "some": func(v *entitysdk.Builder) *entitysdk.Builder {
//	        return c.Arithmetic("add", c.Field(v, "value"), c.Literal(uint64(1)))
//	    },
//	    "none": func(_ *entitysdk.Builder) *entitysdk.Builder {
//	        return c.Literal(uint64(0))
//	    },
//	}, nil /* no default arm */)
func LowerMatch(cb *ComputeBuilder, value *Builder, tagField string,
	arms map[string]func(matched *Builder) *Builder,
	defaultArm func(matched *Builder) *Builder) *Builder {
	if cb == nil {
		return nil
	}
	if value == nil {
		return errBuilder(cb, kindIf, "compute/if",
			fmt.Errorf("LowerMatch: nil value"))
	}
	if tagField == "" {
		return errBuilder(cb, kindIf, "compute/if",
			fmt.Errorf("LowerMatch: empty tagField"))
	}
	if len(arms) == 0 && defaultArm == nil {
		return errBuilder(cb, kindIf, "compute/if",
			fmt.Errorf("LowerMatch: must provide at least one arm or a defaultArm"))
	}

	// Right-nested if-chain on Field(value, tagField).
	//
	// Sort tags for canonical IR shape (same tag-arm set → same hash).
	// This mirrors the canonicalization Construct does on field-keys.
	tagOrder := make([]string, 0, len(arms))
	for tag := range arms {
		tagOrder = append(tagOrder, tag)
	}
	sort.Strings(tagOrder)

	tag := cb.Field(value, tagField)

	// Default arm — innermost else. Either the user-provided default,
	// or a compute/error literal flagging the runtime no-match.
	var defaultExpr *Builder
	if defaultArm != nil {
		defaultExpr = defaultArm(value)
		if defaultExpr == nil {
			return errBuilder(cb, kindIf, "compute/if",
				fmt.Errorf("LowerMatch: defaultArm returned nil"))
		}
	} else {
		// No default: emit a Literal compute/error-shaped record. The
		// evaluator will return it as a compute/error (per F10 / v3.19c
		// — the IR layer just builds the value; the eval boundary wraps
		// it as compute/error semantics).
		defaultExpr = cb.Literal(map[string]interface{}{
			"$compute_error": "no_match",
			"code":           "no_match",
			"message":        "LowerMatch: no arm matched tag",
		})
	}

	// Build right-nested if chain: for tags T1, T2, ..., Tn:
	//   if eq(tag, T1) then arm1(value)
	//   else if eq(tag, T2) then arm2(value)
	//   ...
	//   else defaultExpr
	expr := defaultExpr
	// Iterate in reverse so the leftmost tag becomes the outermost if.
	for i := len(tagOrder) - 1; i >= 0; i-- {
		t := tagOrder[i]
		arm := arms[t]
		body := arm(value)
		if body == nil {
			return errBuilder(cb, kindIf, "compute/if",
				fmt.Errorf("LowerMatch: arm %q returned nil body", t))
		}
		cond := cb.Compare("eq", tag, cb.Literal(t))
		expr = cb.If(cond, body, expr)
	}
	return expr
}

// LowerRecord builds a compute/construct expression for a record with
// the given entity type. Each field value may be either:
//
//   - a *Builder — passed through as that field's computed expression
//   - any other Go value — wrapped via cb.Literal at the field site
//
// The IR produced is identical to the equivalent hand-written
// cb.Construct(...); LowerRecord is sugar that eliminates the most
// common authoring rote (writing cb.Literal(...) at every field
// site for record values that are mostly constants with a few
// computed cells).
//
//	// Before (S1 only):
//	expr := cb.Construct("primitive/any", map[string]*Builder{
//	    "name":  cb.Literal("alice"),
//	    "age":   cb.Literal(uint64(30)),
//	    "score": cb.Arithmetic("add", computedX, computedY),
//	})
//
//	// After (with LowerRecord):
//	expr := LowerRecord(cb, "primitive/any", map[string]interface{}{
//	    "name":  "alice",
//	    "age":   uint64(30),
//	    "score": cb.Arithmetic("add", computedX, computedY),
//	})
//
// Field-key canonicalization (length-then-lex sort on CBOR encode) is
// handled by cb.Construct as usual; LowerRecord does not interact with
// that ordering.
func LowerRecord(cb *ComputeBuilder, entityType string, fields map[string]interface{}) *Builder {
	if cb == nil {
		// Bare *Builder we can't even build a useful error against.
		// Return nil; downstream Build will surface a clean error.
		return nil
	}
	if entityType == "" {
		return errBuilder(cb, kindConstruct, "compute/construct",
			fmt.Errorf("LowerRecord: empty entityType"))
	}
	wrapped := make(map[string]*Builder, len(fields))
	for k, v := range fields {
		switch val := v.(type) {
		case *Builder:
			wrapped[k] = val
		case nil:
			return errBuilder(cb, kindConstruct, "compute/construct",
				fmt.Errorf("LowerRecord: field %q has nil value (pass a zero-value of the intended type, or omit the field)", k))
		default:
			wrapped[k] = cb.Literal(val)
		}
	}
	return cb.Construct(entityType, wrapped)
}
