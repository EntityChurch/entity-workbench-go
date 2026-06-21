package entitysdk

// Compute-expression builder (S1) — typed Go API that produces
// EXTENSION-COMPUTE v3.18 expression entities bottom-up via a single
// .Build() call.
//
// Entry point: ap.Compute() returns a *ComputeBuilder. Each node
// constructor returns *Builder; nesting composes the DAG. .Build(ctx,
// path) topologically puts every node and returns the root hash.
//
// Four design pitfalls (all enforced at build-time):
//
//   1. Rule 11 — compute/numeric-cast is eager and does NOT flow
//      through compute/let. Let(...) rejects bindings whose value is
//      a NumericCast Builder; cast must appear inline at the
//      operand site.
//
//   2. compute/apply Args canonical order — handled transparently by
//      ecf.Encode (CoreDetEncOptions = CTAP2 canonical sort).
//      Identical expressions in different builder-insertion orders
//      produce identical content hashes.
//
//   3. Lambda is always an expression, never a closure. b.Lambda(...)
//      returns a Builder whose materialized type is compute/lambda;
//      compute/closure is a runtime value, not a builder constructor.
//
//   4. F5 (capability ⇒ resource) — Apply(... WithCapability(c) ...)
//      without WithResource(r) returns a build-time error. Matches
//      PROPOSAL-COMPUTE-APPLY-RESOURCE-CEILING §F5.
//
// Storage: .Build(ctx, rootPath) writes the root expression at rootPath
// and intermediates at rootPath + "/_expr/" + hashPrefix. The evaluator
// resolves nodes by content hash, so tree-path layout below the root
// is internal to the builder.

import (
	"context"
	"encoding/hex"
	"fmt"

	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"
)

// ComputeBuilder is the entry point. Obtain one via ap.Compute().
// All node constructors are methods so the builder can capture the
// owning AppPeer for the final .Build() put.
type ComputeBuilder struct {
	ap *AppPeer
}

// Builder is one node in the in-memory expression DAG. Construct via
// ComputeBuilder methods (cb.Literal, cb.Field, ...). Compose by
// passing one Builder into another. Materialize the whole DAG by
// calling .Build(ctx, rootPath) on any node.
type Builder struct {
	cb *ComputeBuilder

	// kind names this node's logical role (for error messages, Rule 11
	// detection, and the small set of build-time checks below). Mirrors
	// typeName but easier to compare than the full type string.
	kind     builderKind
	typeName string

	// children holds direct child Builders in deterministic order. The
	// .Build() topological pass resolves each child to a hash before
	// invoking materialize on this node.
	children []*Builder

	// materialize produces the concrete entity given resolved hashes for
	// children (in the same order as children). Closure captures
	// non-Builder fields (literal value, lookup name, etc.).
	materialize func(childHashes []hash.Hash) (entity.Entity, error)

	// buildErr is a build-time error captured during construction (e.g.
	// F5 violation, Rule 11 violation). Surfaces from .Build().
	buildErr error
}

type builderKind int

const (
	kindUnknown builderKind = iota
	kindLiteral
	kindLookupScope
	kindLookupTree
	kindLookupHash
	kindField
	kindIndex
	kindLength
	kindNumericCast
	kindArithmetic
	kindCompare
	kindLogic
	kindConstruct
	kindApply
	kindBuiltinsCall
	kindIf
	kindLet
	kindLambda
)

// Compute returns the compute-expression builder for this peer. The
// returned ComputeBuilder is stateless; create one per authoring task
// or hold one for the lifetime of the AppPeer — either works.
func (a *AppPeer) Compute() *ComputeBuilder { return &ComputeBuilder{ap: a} }

// --- Leaf nodes --------------------------------------------------------

// Literal wraps a Go value as compute/literal. The value is CBOR-
// encoded by ecf.Encode; primitive types (uint64, int64, float64,
// string, bool, []byte) map directly. Collections (map, slice) are
// encoded as CBOR; the evaluator can index/field them per N.1.
func (cb *ComputeBuilder) Literal(v interface{}) *Builder {
	return &Builder{
		cb:       cb,
		kind:     kindLiteral,
		typeName: types.TypeComputeLiteral,
		materialize: func(_ []hash.Hash) (entity.Entity, error) {
			return types.ComputeLiteralData{Value: v}.ToEntity()
		},
	}
}

// LookupScope resolves a name from the dispatch scope at eval time.
// For entity-native handlers the scope carries {operation, params,
// resource, caller_capability}; cb.LookupScope("params") is the
// canonical way to reach the dispatched EXECUTE's params.
func (cb *ComputeBuilder) LookupScope(name string) *Builder {
	return &Builder{
		cb:       cb,
		kind:     kindLookupScope,
		typeName: types.TypeComputeLookupScope,
		materialize: func(_ []hash.Hash) (entity.Entity, error) {
			return types.ComputeLookupScopeData{Name: name}.ToEntity()
		},
	}
}

// LookupTree resolves a tree path at eval time. Impure — produces a
// reactive dependency edge in the gradient. Use relative=true to
// resolve under the handler's logical path.
func (cb *ComputeBuilder) LookupTree(path string, relative bool) *Builder {
	return &Builder{
		cb:       cb,
		kind:     kindLookupTree,
		typeName: types.TypeComputeLookupTree,
		materialize: func(_ []hash.Hash) (entity.Entity, error) {
			return types.ComputeLookupTreeData{Path: path, Relative: relative}.ToEntity()
		},
	}
}

// LookupTreeLocal is sugar over LookupTree for the common case of a
// local-peer dependency. It auto-qualifies path to "/{peerID}/{path}"
// so the resulting dep stored in LookupTreeData.Path matches the
// canonicalized path the store dispatches in TreeChangeEvent.
//
// This is the S8 ergonomic helper from FEEDBACK-COMPUTE-FOUNDATION-CLOSED
// — promoted to E7.2 SHOULD-provide in SDK-EXTENSION-OPERATIONS. The
// path-qualification asymmetry it closes: `compute/lookup/tree` stores
// Path verbatim (so the reactive dep index keys on the literal string),
// but the tree layer's NamespacedIndex.canonicalize auto-qualifies
// bare paths to /{localNS}/. Without LookupTreeLocal, authors hit a
// silent reactivity miss when LookupTree("app/foo", false) is paired
// with PutEntity("app/foo", ...) — the dep is "app/foo" but the
// event fires "/peerID/app/foo", and the index lookup misses.
//
// Use LookupTreeLocal for any compute that reads tree state on the
// SAME peer the compute is registered against (the common case).
// Keep the explicit LookupTree(path, relative) for cross-peer cases
// where you already know the full qualified shape, OR for transferable
// subgraphs that need §9.4 relative-path semantics.
//
//	// Common reactive aggregate over local entities:
//	expr := c.Arithmetic("add",
//	    c.Field(c.LookupTreeLocal("app/inputs/a"), "size"),
//	    c.Field(c.LookupTreeLocal("app/inputs/b"), "size"))
//
// The spec-level question (should LookupTreeData.Path default to
// local-relative for unqualified paths?) is tracked by arch as a
// proposal-pending question — it collides with §9.4 transferable
// relative-path semantics. The SDK helper closes the workbench
// authoring case without prejudging the spec resolution.
func (cb *ComputeBuilder) LookupTreeLocal(path string) *Builder {
	qualified := "/" + cb.ap.PeerID() + "/" + path
	return cb.LookupTree(qualified, false)
}

// LookupHash resolves a content hash at eval time. Pure — the hash
// pins the content. Optional path field is a hint for tree-recovery
// if the content store evicts.
func (cb *ComputeBuilder) LookupHash(h hash.Hash, pathHint string, relative bool) *Builder {
	return &Builder{
		cb:       cb,
		kind:     kindLookupHash,
		typeName: types.TypeComputeLookupHash,
		materialize: func(_ []hash.Hash) (entity.Entity, error) {
			return types.ComputeLookupHashData{Hash: h, Path: pathHint, Relative: relative}.ToEntity()
		},
	}
}

// --- Unary / binary inline ops -----------------------------------------

// Field reads a named field from an entity-typed value. String-keyed
// only (per the core-go evaluator); CBOR arrays use Index instead.
func (cb *ComputeBuilder) Field(target *Builder, name string) *Builder {
	return &Builder{
		cb:       cb,
		kind:     kindField,
		typeName: types.TypeComputeField,
		children: []*Builder{target},
		materialize: func(hs []hash.Hash) (entity.Entity, error) {
			return types.ComputeFieldData{Name: name, Entity: hs[0]}.ToEntity()
		},
	}
}

// Index reads an element from a CBOR-array value. Out-of-range or
// non-array targets produce compute/error per N.1 edge cases.
func (cb *ComputeBuilder) Index(array, index *Builder) *Builder {
	return &Builder{
		cb:       cb,
		kind:     kindIndex,
		typeName: types.TypeComputeIndex,
		children: []*Builder{array, index},
		materialize: func(hs []hash.Hash) (entity.Entity, error) {
			return types.ComputeIndexData{Array: hs[0], Index: hs[1]}.ToEntity()
		},
	}
}

// Length returns the length of a CBOR-array value. Empty → 0;
// non-array → compute/error type_mismatch per N.1.
func (cb *ComputeBuilder) Length(array *Builder) *Builder {
	return &Builder{
		cb:       cb,
		kind:     kindLength,
		typeName: types.TypeComputeLength,
		children: []*Builder{array},
		materialize: func(hs []hash.Hash) (entity.Entity, error) {
			return types.ComputeLengthData{Array: hs[0]}.ToEntity()
		},
	}
}

// NumericCast reinterprets a numeric value as a different numeric
// type (intra-numeric: int/uint/float). Per Rule 11 the cast is
// eager and consumed by the immediately-following op; binding a cast
// via Let drops the cast effect (Let enforces this with a build error).
//
// toType is the destination compute primitive type (e.g.
// "primitive/uint", "primitive/int", "primitive/float").
func (cb *ComputeBuilder) NumericCast(value *Builder, toType string) *Builder {
	return &Builder{
		cb:       cb,
		kind:     kindNumericCast,
		typeName: types.TypeComputeNumericCast,
		children: []*Builder{value},
		materialize: func(hs []hash.Hash) (entity.Entity, error) {
			return types.ComputeNumericCastData{Value: hs[0], ToType: toType}.ToEntity()
		},
	}
}

// Arithmetic op = "add" | "sub" | "mul" | "div" | "mod". add/sub/mul
// are sign-agnostic (overflow wraps mod 2^64); div/mod/compare are
// signed-default unless an immediate-upstream NumericCast switches
// the interpretation (Rule 11).
func (cb *ComputeBuilder) Arithmetic(op string, left, right *Builder) *Builder {
	return &Builder{
		cb:       cb,
		kind:     kindArithmetic,
		typeName: types.TypeComputeArithmetic,
		children: []*Builder{left, right},
		materialize: func(hs []hash.Hash) (entity.Entity, error) {
			return types.ComputeArithmeticData{Op: op, Left: hs[0], Right: hs[1]}.ToEntity()
		},
	}
}

// Compare op = "eq" | "ne" | "lt" | "le" | "gt" | "ge". Signed-default
// per the v3.18 integer model.
func (cb *ComputeBuilder) Compare(op string, left, right *Builder) *Builder {
	return &Builder{
		cb:       cb,
		kind:     kindCompare,
		typeName: types.TypeComputeCompare,
		children: []*Builder{left, right},
		materialize: func(hs []hash.Hash) (entity.Entity, error) {
			return types.ComputeCompareData{Op: op, Left: hs[0], Right: hs[1]}.ToEntity()
		},
	}
}

// Logic op = "and" | "or" (binary) | "not" (unary; pass nil for right).
func (cb *ComputeBuilder) Logic(op string, left *Builder, right *Builder) *Builder {
	children := []*Builder{left}
	if right != nil {
		children = append(children, right)
	}
	return &Builder{
		cb:       cb,
		kind:     kindLogic,
		typeName: types.TypeComputeLogic,
		children: children,
		materialize: func(hs []hash.Hash) (entity.Entity, error) {
			data := types.ComputeLogicData{Op: op, Left: hs[0]}
			if len(hs) > 1 {
				r := hs[1]
				data.Right = &r
			}
			return data.ToEntity()
		},
	}
}

// --- Complex nodes -----------------------------------------------------

// Construct builds a typed entity from named compute sub-values.
// fields maps field-name → Builder; the resolved hashes become the
// field values in the constructed entity.
func (cb *ComputeBuilder) Construct(entityType string, fields map[string]*Builder) *Builder {
	// Stable child order: sort fields by name. canonical CBOR will
	// re-sort the map by length-then-lex on encode; this ordering is
	// just for deterministic child resolution.
	keys := sortedMapKeys(fields)
	children := make([]*Builder, len(keys))
	for i, k := range keys {
		children[i] = fields[k]
	}
	return &Builder{
		cb:       cb,
		kind:     kindConstruct,
		typeName: types.TypeComputeConstruct,
		children: children,
		materialize: func(hs []hash.Hash) (entity.Entity, error) {
			out := make(map[string]hash.Hash, len(keys))
			for i, k := range keys {
				out[k] = hs[i]
			}
			return types.ComputeConstructData{EntityType: entityType, Fields: out}.ToEntity()
		},
	}
}

// ApplyOption configures a compute/apply node.
type ApplyOption func(*applyConfig)

type applyConfig struct {
	capability *Builder
	resource   *Builder
}

// WithCapability sets the capability field on compute/apply. The
// capability overrides the handler's grant for this dispatch (dual-
// check applies). Per F5, MUST be paired with WithResource — Apply
// returns a build-time error otherwise.
func WithCapability(b *Builder) ApplyOption {
	return func(c *applyConfig) { c.capability = b }
}

// WithResource sets the resource field on compute/apply. Required
// when WithCapability is set (F5); optional otherwise.
func WithResource(b *Builder) ApplyOption {
	return func(c *applyConfig) { c.resource = b }
}

// Apply dispatches an EXECUTE to a registered handler at path with
// the named operation. args become the EXECUTE's params, expressed
// as a primitive/any with the named keys; see GUIDE-COMPUTE-PROGRAMMING
// §3 for the args→params convention.
//
// Default behavior (no options): handler runs under its own grant —
// the simplest, safest pattern. Use WithCapability/WithResource only
// for proxy-style "act on caller's behalf" handlers; F5 requires both
// together.
//
// SA-4: when path resolves to a language-native / compiled handler
// returning primitive/any, the result is wrapped as compute/result
// {value, expression}. Use Field(result, "value") to extract the
// inner value for downstream compute use, or call UnwrapComputeResult
// at the SDK boundary.
func (cb *ComputeBuilder) Apply(path, operation string, args map[string]*Builder, opts ...ApplyOption) *Builder {
	var cfg applyConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	// F5 — capability without resource is a structural violation. Catch
	// at build time so users see a Go-side error before the install/eval
	// dispatch surfaces invalid_expression.
	if cfg.capability != nil && cfg.resource == nil {
		return errBuilder(cb, kindApply, types.TypeComputeApply,
			fmt.Errorf("compute.Apply: WithCapability requires WithResource (F5: PROPOSAL-COMPUTE-APPLY-RESOURCE-CEILING)"))
	}

	// Children order: args (sorted by name) + optional capability + optional resource.
	argKeys := sortedMapKeys(args)
	children := make([]*Builder, 0, len(argKeys)+2)
	for _, k := range argKeys {
		children = append(children, args[k])
	}
	var capIdx, resIdx = -1, -1
	if cfg.capability != nil {
		capIdx = len(children)
		children = append(children, cfg.capability)
	}
	if cfg.resource != nil {
		resIdx = len(children)
		children = append(children, cfg.resource)
	}

	return &Builder{
		cb:       cb,
		kind:     kindApply,
		typeName: types.TypeComputeApply,
		children: children,
		materialize: func(hs []hash.Hash) (entity.Entity, error) {
			outArgs := make(map[string]hash.Hash, len(argKeys))
			for i, k := range argKeys {
				outArgs[k] = hs[i]
			}
			data := types.ComputeApplyData{
				Path:      path,
				Operation: operation,
				Args:      outArgs,
			}
			if capIdx >= 0 {
				data.Capability = hs[capIdx]
			}
			if resIdx >= 0 {
				data.Resource = hs[resIdx]
			}
			return data.ToEntity()
		},
	}
}

// ApplyClosure builds a closure-mode compute/apply — used for
// self-reference (recursion), invoking stored lambdas by hash/path,
// and any "call this expression-as-function" pattern. The fnNode
// argument is any expression node that evaluates to a compute/closure
// — typically LookupHash(lambdaHash) or LookupTree(lambdaPath).
//
// This is the closure-mode counterpart to Apply (which is handler-mode).
// Per EXTENSION-COMPUTE §2.2, compute/apply MUST have either `path`
// (handler) or `fn` (closure), never both — this constructor emits the
// `fn` shape. The evaluator's tail-call optimization (§4.x) makes
// closure-mode self-application iterate without depth growth, so this
// is the substrate for LowerRecurse.
//
// Per PROPOSAL-COMPUTE-RECURSION-AND-SUM-TYPES §2 / SDK-EXTENSION-
// OPERATIONS §8 E7: the constructor is the only spec-side gap that
// was open; closure mode + TCO already exist in core-go. Ships now.
//
//	// Build a stored lambda; the body LookupTrees its own path for self-reference.
//	const factPath = "app/recurse/factorial"
//	body := /* build a lambda whose body references LookupTree(factPath) */
//	lambda.Build(ctx, factPath)
//	expr := c.ApplyClosure(c.LookupTree(factPath, false), map[string]*Builder{
//	    "n": c.Literal(uint64(5)),
//	})
//
// Frontends will typically use LowerRecurse (the toolkit layer) rather
// than wiring ApplyClosure by hand.
func (cb *ComputeBuilder) ApplyClosure(fnNode *Builder, args map[string]*Builder) *Builder {
	if fnNode == nil {
		return errBuilder(cb, kindApply, types.TypeComputeApply,
			fmt.Errorf("compute.ApplyClosure: fnNode is nil"))
	}

	// Children order: args (sorted by name) + fnNode.
	argKeys := sortedMapKeys(args)
	children := make([]*Builder, 0, len(argKeys)+1)
	for _, k := range argKeys {
		children = append(children, args[k])
	}
	fnIdx := len(children)
	children = append(children, fnNode)

	return &Builder{
		cb:       cb,
		kind:     kindApply,
		typeName: types.TypeComputeApply,
		children: children,
		materialize: func(hs []hash.Hash) (entity.Entity, error) {
			outArgs := make(map[string]hash.Hash, len(argKeys))
			for i, k := range argKeys {
				outArgs[k] = hs[i]
			}
			return types.ComputeApplyData{
				Fn:   hs[fnIdx],
				Args: outArgs,
			}.ToEntity()
		},
	}
}

// BuiltinsCall is sugar for Apply against a system/compute/builtins/*
// handler. The collection stdlib (map/filter/fold/collect_keys) and
// store live there; all are MUST-given-COMPUTE per the v3.18 floor.
//
// builtin is the suffix (e.g. "map", "filter", "fold", "store").
// args follows the per-builtin schema (see system/compute/{builtin}-args
// types in EXTENSION-COMPUTE).
func (cb *ComputeBuilder) BuiltinsCall(builtin string, args map[string]*Builder) *Builder {
	path := "system/compute/builtins/" + builtin
	// Builtins use "eval" as the operation per core-go validate driver
	// (`cmd/internal/validate/compute.go::computeEvalBuiltinFold`).
	// evalApplyHandler shortcuts on IsBuiltinPath without examining the
	// operation name; "eval" is the cross-impl convention.
	return cb.Apply(path, "eval", args)
}

// If conditionally evaluates then or elseExpr based on cond. elseExpr
// may be nil (degenerate — evaluator may produce null when cond is false).
func (cb *ComputeBuilder) If(cond, thenExpr, elseExpr *Builder) *Builder {
	children := []*Builder{cond, thenExpr}
	if elseExpr != nil {
		children = append(children, elseExpr)
	}
	return &Builder{
		cb:       cb,
		kind:     kindIf,
		typeName: types.TypeComputeIf,
		children: children,
		materialize: func(hs []hash.Hash) (entity.Entity, error) {
			data := types.ComputeIfData{Condition: hs[0], Then: hs[1]}
			if len(hs) > 2 {
				e := hs[2]
				data.Else = &e
			}
			return data.ToEntity()
		},
	}
}

// Let binds named values into a local scope and evaluates body in
// that scope. The map's iteration order is normalized (sorted by
// binding name) so two Let constructions with identical bindings
// produce identical hashes.
//
// **Rule 11 enforcement:** Let refuses bindings whose value is a
// NumericCast. The cast effect is consumed by the immediately-
// following arithmetic op; binding via Let drops the effect. Express
// the cast inline at the use site instead. (See
// EXPLORATION-COMPUTE-FRONTEND-WORKBENCH-GO.md §7.2 for the worked
// example of the trap.)
func (cb *ComputeBuilder) Let(bindings map[string]*Builder, body *Builder) *Builder {
	keys := sortedMapKeys(bindings)
	for _, k := range keys {
		if bindings[k] != nil && bindings[k].kind == kindNumericCast {
			return errBuilder(cb, kindLet, types.TypeComputeLet,
				fmt.Errorf("compute.Let: binding %q is a NumericCast — Rule 11 says the cast effect does not flow through let; inline the cast at the use site instead", k))
		}
	}
	children := make([]*Builder, 0, len(keys)+1)
	for _, k := range keys {
		children = append(children, bindings[k])
	}
	children = append(children, body)
	bodyIdx := len(children) - 1
	return &Builder{
		cb:       cb,
		kind:     kindLet,
		typeName: types.TypeComputeLet,
		children: children,
		materialize: func(hs []hash.Hash) (entity.Entity, error) {
			bs := make([]types.ComputeLetBinding, len(keys))
			for i, k := range keys {
				bs[i] = types.ComputeLetBinding{Name: k, Value: hs[i]}
			}
			return types.ComputeLetData{Bindings: bs, Body: hs[bodyIdx]}.ToEntity()
		},
	}
}

// Lambda constructs a compute/lambda *expression* — never a compute/closure
// (which is a runtime value produced by evaluating a lambda). When passed
// as the fn argument of a builtin Apply, the evaluator evaluates the
// lambda expression to a closure on demand. Pre-evaluating to a closure
// and storing it is non-portable (per COMPUTE-V314-CROSS-IMPL-DIVERGENCES).
func (cb *ComputeBuilder) Lambda(params []string, body *Builder) *Builder {
	return &Builder{
		cb:       cb,
		kind:     kindLambda,
		typeName: types.TypeComputeLambda,
		children: []*Builder{body},
		materialize: func(hs []hash.Hash) (entity.Entity, error) {
			return types.ComputeLambdaData{Params: params, Body: hs[0]}.ToEntity()
		},
	}
}

// --- Build: topological put ---------------------------------------------

// Build materializes the DAG rooted at b. The root entity is written at
// rootPath; intermediate nodes go at rootPath + "/_expr/{hashPrefix}".
// Returns the root hash.
//
// Build is idempotent across multiple calls — the in-memory DAG is not
// mutated, and re-putting the same content yields the same content hash
// at the same paths.
func (b *Builder) Build(ctx context.Context, rootPath string) (hash.Hash, error) {
	if b == nil {
		return hash.Hash{}, fmt.Errorf("compute.Build: nil Builder")
	}
	// Walk the DAG; collect nodes in post-order (children-before-parent)
	// while deduplicating by pointer identity. The topological order is
	// the order entities must be PutEntity'd (each parent depends on
	// child hashes existing in the content store, though hash derivation
	// itself doesn't require prior puts).
	visited := make(map[*Builder]bool)
	var order []*Builder
	var walk func(n *Builder) error
	walk = func(n *Builder) error {
		if n == nil {
			return fmt.Errorf("compute.Build: nil child in DAG")
		}
		if visited[n] {
			return nil
		}
		visited[n] = true
		if n.buildErr != nil {
			return n.buildErr
		}
		for _, c := range n.children {
			if err := walk(c); err != nil {
				return err
			}
		}
		order = append(order, n)
		return nil
	}
	if err := walk(b); err != nil {
		return hash.Hash{}, err
	}

	// Materialize + put in topological order.
	resolved := make(map[*Builder]hash.Hash, len(order))
	intermediatePath := rootPath + "/_expr"
	rootIdx := len(order) - 1

	for i, n := range order {
		childHashes := make([]hash.Hash, len(n.children))
		for j, c := range n.children {
			h, ok := resolved[c]
			if !ok {
				// Should be impossible given post-order traversal.
				return hash.Hash{}, fmt.Errorf("compute.Build: child not resolved (kind=%v) — internal bug", c.kind)
			}
			childHashes[j] = h
		}
		ent, err := n.materialize(childHashes)
		if err != nil {
			return hash.Hash{}, fmt.Errorf("compute.Build: materialize kind=%v: %w", n.kind, err)
		}
		var path string
		if i == rootIdx {
			path = rootPath
		} else {
			// Use a short hash-prefix as the leaf name — deterministic and
			// readable in a tree dump.
			path = intermediatePath + "/" + hex.EncodeToString(ent.ContentHash.Digest[:])[:16]
		}
		h, err := n.cb.ap.PutEntity(path, ent)
		if err != nil {
			return hash.Hash{}, fmt.Errorf("compute.Build: put at %q: %w", path, err)
		}
		resolved[n] = h
	}

	return resolved[b], nil
}

// --- helpers ------------------------------------------------------------

// errBuilder returns a Builder whose .Build surfaces err. Used for
// build-time constraint violations (Rule 11, F5, ...).
func errBuilder(cb *ComputeBuilder, k builderKind, typeName string, err error) *Builder {
	return &Builder{cb: cb, kind: k, typeName: typeName, buildErr: err}
}

// sortedMapKeys returns the keys of m in lexicographic order. Used
// for deterministic child-Builder enumeration in Construct/Apply/Let.
// (Canonical CBOR re-sorts on encode; this is for in-memory order.)
func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Lex sort is fine for in-memory determinism; CBOR encoder applies
	// length-first canonical sort independently.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
