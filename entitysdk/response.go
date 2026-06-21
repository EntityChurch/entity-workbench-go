package entitysdk

import (
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/handler"
	"go.entitychurch.org/entity-core-go/core/hash"

	"github.com/fxamacker/cbor/v2"
)

// Response is the return value of Executor.Execute*, matching the
// shape in SDK-OPERATIONS §4.1:
//
//	Response := {
//	  status:   uint       ; HTTP-style status code
//	  type:     string     ; Response entity type
//	  data:     any        ; Response payload (CBOR-encoded)
//	  hash:     hash       ; Content hash of the response entity
//	  included: [Entity]?  ; Supporting entities (signatures,
//	                        capabilities, multi-entity results)
//	}
//
// Data is the CBOR payload — use `ecf.Decode(resp.Data, &v)` to
// unmarshal into a typed value, or `resp.Entity()` to recompose the
// response as an `entity.Entity` for `types.FooFromEntity(...)`
// decoders.
type Response struct {
	Status   uint
	Type     string
	Data     cbor.RawMessage
	Hash     hash.Hash
	Included map[hash.Hash]entity.Entity
}

// Entity reassembles the response's result as an entity.Entity,
// convenient for passing into type-specific decoders
// (`types.QueryResultDataFromEntity`, etc.) that expect an entity.
func (r *Response) Entity() entity.Entity {
	return entity.Entity{Type: r.Type, Data: r.Data, ContentHash: r.Hash}
}

// responseFromHandler converts the core handler.Response into the
// SDK-owned Response shape. Used at the Executor boundary so the
// handler package is no longer leaked across the SDK API.
func responseFromHandler(resp *handler.Response) *Response {
	if resp == nil {
		return nil
	}
	return &Response{
		Status:   resp.Status,
		Type:     resp.Result.Type,
		Data:     resp.Result.Data,
		Hash:     resp.Result.ContentHash,
		Included: resp.Included,
	}
}
