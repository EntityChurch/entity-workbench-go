package entitysdk

// Compute SDK ergonomic helpers (S3 + S5) — bookend the construct/
// consume sides of the compute boundary.
//
//   PrimitiveAny (S3) — wrap a Go value as a primitive/any entity for
//     use as EXECUTE params. Replaces the F3 friction of
//     ecf.Encode + entity.NewEntity by hand at every call site.
//
//   UnwrapComputeResult (S5) — strip the SA-4 compute/result envelope
//     when the caller wants the inner value. Counterpart to F8.

import (
	"fmt"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"

	"github.com/fxamacker/cbor/v2"
)

// PrimitiveAny wraps a Go value as a primitive/any entity. The value
// is CBOR-encoded via ecf.Encode (canonical/deterministic).
//
// Use case: the dispatch surface (Executor.ExecuteWithParams) takes an
// entity for params; most application-level params are Go maps or
// structs that want primitive/any wrapping.
//
//	params, err := entitysdk.PrimitiveAny(map[string]interface{}{
//	    "numbers": []uint64{1, 2, 3, 4, 5},
//	})
//	resp, err := ap.Executor().ExecuteWithParams("app/mypattern", "compute", params)
func PrimitiveAny(v interface{}) (entity.Entity, error) {
	raw, err := ecf.Encode(v)
	if err != nil {
		return entity.Entity{}, fmt.Errorf("PrimitiveAny: encode: %w", err)
	}
	return entity.NewEntity("primitive/any", cbor.RawMessage(raw))
}

// UnwrapComputeResultAsMap decodes a compute response's Data into a
// map[string]interface{}, absorbing the CBOR-decoder's two-shape
// inconsistency (some encodings yield map[string]interface{}, others
// map[interface{}]interface{}). Returns the normalized map.
//
// This is the S4 ergonomic helper (now E7.2 in
// SDK-EXTENSION-OPERATIONS). Wraps the boilerplate three call sites
// were duplicating after every compute eval that returned a record.
//
// Accepts either:
//   - a bare primitive/any record return (e.g. Scenario B's
//     {count, sum, average}) — Data is the CBOR-encoded map directly,
//   - a compute/result envelope (when handler-mode compute/apply
//     wraps the inner primitive/any per SA-4) — peels .value first.
//
// Errors if resp is nil, if the bytes decode as neither map shape, or
// if a non-string key appears in the map[interface{}]interface{}
// fallback (such a shape is structurally invalid for a record).
func UnwrapComputeResultAsMap(resp *Response) (map[string]interface{}, error) {
	if resp == nil {
		return nil, fmt.Errorf("UnwrapComputeResultAsMap: nil Response")
	}
	data := resp.Data
	if resp.Type == types.TypeComputeResult {
		inner, _, err := UnwrapComputeResult(resp)
		if err != nil {
			return nil, fmt.Errorf("UnwrapComputeResultAsMap: %w", err)
		}
		data = inner
	}
	return decodeAsStringKeyMap(data, "UnwrapComputeResultAsMap")
}

// UnwrapComputeResultAsList decodes a compute response's Data into a
// []interface{}, peeling the SA-4 envelope if present. Counterpart
// to UnwrapComputeResultAsMap for array-shaped returns (e.g.
// LowerFilter / LowerMap results).
func UnwrapComputeResultAsList(resp *Response) ([]interface{}, error) {
	if resp == nil {
		return nil, fmt.Errorf("UnwrapComputeResultAsList: nil Response")
	}
	data := resp.Data
	if resp.Type == types.TypeComputeResult {
		inner, _, err := UnwrapComputeResult(resp)
		if err != nil {
			return nil, fmt.Errorf("UnwrapComputeResultAsList: %w", err)
		}
		data = inner
	}
	var out []interface{}
	if err := ecf.Decode(data, &out); err != nil {
		return nil, fmt.Errorf("UnwrapComputeResultAsList: decode: %w", err)
	}
	return out, nil
}

// UnwrapChainStepDelivery extracts the bare inner value from a
// continuation chain step delivery. Chain delivery double-wraps:
// `req.Params.Data` is the CBOR encoding of `InboxDeliveryData{
// status, result}`, and `result` is the CBOR encoding of the **full
// entity envelope** `{type, data, content_hash}` of the prior
// step's response. The bare inner value (what the trampoline
// actually wants) is at `result.data`.
//
// This is the S7 ergonomic helper — promoted to E7.2
// SHOULD-provide in SDK-EXTENSION-OPERATIONS. Removes the manual
// envelope-peel-twice dance every continuation-chain transform handler
// would otherwise re-implement.
//
//	func myTrampoline(ctx context.Context, req *handler.Request) (*handler.Response, error) {
//	    d, err := entitysdk.UnwrapChainStepDelivery(req.Params)
//	    if err != nil { ... }
//	    if d.Status != 200 { ... }
//	    // d.Value is the bare inner value from the prior chain step
//	    // (e.g. uint64(60) if prior step's compute returned sum=60).
//	    // d.OriginalRequestID lets you correlate forwarded deliveries.
//	    ...
//	}
//
// The function is permissive about the inner-result shape:
//   - If the result decodes as an entity-envelope map (has "data" key),
//     returns `["data"]` (the canonical case — entity-native dispatch
//     wrapping primitive/any).
//   - If the result decodes as an SA-4 compute/result envelope (has
//     "value" key), returns `["value"]` (the compute/apply handler-mode
//     return shape).
//   - Otherwise returns the decoded value verbatim.
//
// Doc gap also flagged for arch (E7.2 footnote): GUIDE-CROSS-PEER-MESSAGING
// should clarify the double-wrap wire shape; this helper papers over
// that gap until they do.
// ChainStepDelivery is the unwrapped view of a chain step delivery.
type ChainStepDelivery struct {
	// OriginalRequestID propagates from the chain origin — useful for
	// correlating forwarded deliveries when the trampoline emits its
	// own onward delivery.
	OriginalRequestID string
	// Status is the prior step's response status (200 = success).
	Status uint
	// Value is the bare inner value after double-wrap peel:
	// InboxDeliveryData.Result → entity-envelope → .data (or .value
	// for SA-4 compute/result shape).
	Value interface{}
}

func UnwrapChainStepDelivery(params entity.Entity) (*ChainStepDelivery, error) {
	var delivery types.InboxDeliveryData
	if err := ecf.Decode(params.Data, &delivery); err != nil {
		return nil, fmt.Errorf("UnwrapChainStepDelivery: decode InboxDeliveryData: %w", err)
	}
	out := &ChainStepDelivery{
		OriginalRequestID: delivery.OriginalRequestID,
		Status:            delivery.Status,
	}

	// The InboxDeliveryData.Result holds CBOR-encoded bytes of the
	// previous step's full response entity. Try the entity-envelope
	// shape first (the entity-native dispatch wrap).
	envelope, mapErr := decodeAsStringKeyMap(delivery.Result, "UnwrapChainStepDelivery.envelope")
	if mapErr == nil {
		if v, ok := envelope["data"]; ok {
			out.Value = v
			return out, nil
		}
		if v, ok := envelope["value"]; ok {
			// SA-4 compute/result shape.
			out.Value = v
			return out, nil
		}
		// Map shape but no "data" or "value" key — hand back as-is.
		out.Value = envelope
		return out, nil
	}

	// Not a map shape — decode as a bare value.
	var bare interface{}
	if err := ecf.Decode(delivery.Result, &bare); err != nil {
		return nil, fmt.Errorf("UnwrapChainStepDelivery: decode inner result: %w", err)
	}
	out.Value = bare
	return out, nil
}

// decodeAsStringKeyMap normalizes CBOR's two map shapes
// (map[string]interface{} vs map[interface{}]interface{}) into one.
// Internal helper; also reused by chain-step delivery unwrap.
func decodeAsStringKeyMap(data []byte, ctx string) (map[string]interface{}, error) {
	var direct map[string]interface{}
	if err := ecf.Decode(data, &direct); err == nil && direct != nil {
		return direct, nil
	}
	var alt map[interface{}]interface{}
	if err := ecf.Decode(data, &alt); err != nil {
		return nil, fmt.Errorf("%s: decode (both map shapes failed): %w", ctx, err)
	}
	out := make(map[string]interface{}, len(alt))
	for k, v := range alt {
		ks, ok := k.(string)
		if !ok {
			return nil, fmt.Errorf("%s: non-string key %v (%T) in map", ctx, k, k)
		}
		out[ks] = v
	}
	return out, nil
}

// UnwrapComputeResult strips the SA-4 compute/result envelope returned
// by handler-mode compute/apply when the inner handler returned
// primitive/any. Returns the inner value (as raw CBOR ready for decode),
// the apply expression's hash (provenance), and an error if resp's
// type is not compute/result.
//
// Callers that want the inner value as a typed Go value should follow
// with ecf.Decode on the returned raw bytes:
//
//	value, exprHash, err := entitysdk.UnwrapComputeResult(resp)
//	if err != nil { ... }
//	var decoded map[string]interface{}
//	_ = ecf.Decode(value, &decoded)
//
// Callers that want a typed entity should reach into resp.Data
// directly and call ComputeResultDataFromEntity (when it exists in
// core-go types) — the helper here is for the most common case where
// the inner value is what you actually want.
func UnwrapComputeResult(resp *Response) (cbor.RawMessage, hash.Hash, error) {
	if resp == nil {
		return nil, hash.Hash{}, fmt.Errorf("UnwrapComputeResult: nil Response")
	}
	if resp.Type != types.TypeComputeResult {
		return nil, hash.Hash{}, fmt.Errorf("UnwrapComputeResult: expected type=%s, got type=%s",
			types.TypeComputeResult, resp.Type)
	}
	var wrap struct {
		Value      cbor.RawMessage `cbor:"value"`
		Expression hash.Hash       `cbor:"expression"`
	}
	if err := ecf.Decode(resp.Data, &wrap); err != nil {
		return nil, hash.Hash{}, fmt.Errorf("UnwrapComputeResult: decode envelope: %w", err)
	}
	return wrap.Value, wrap.Expression, nil
}
