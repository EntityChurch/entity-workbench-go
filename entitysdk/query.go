package entitysdk

import (
	"fmt"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/types"
)

// Query executes a find operation against the system/query handler.
// Returns matching entities filtered by the expression criteria.
//
// The query handler wraps its result in a system/envelope (root =
// QueryResultData, included = domain entities when requested). We
// decode the envelope and return the root; included domain entities
// are dropped for now — surface them if a caller needs IncludeEntities.
func (ex *Executor) Query(expr types.QueryExpressionData) (types.QueryResultData, error) {
	paramEnt, err := expr.ToEntity()
	if err != nil {
		return types.QueryResultData{}, WrapError(400, "encode_failed", "query encode", err)
	}

	resp, err := ex.ExecuteWithParams("system/query", "find", paramEnt)
	if err != nil {
		return types.QueryResultData{}, err
	}

	if resp.Type != "system/envelope" {
		return types.QueryResultData{}, NewError(500, "unexpected_result_type",
			"query find: expected system/envelope, got "+resp.Type)
	}
	var env entity.Envelope
	if err := ecf.Decode(resp.Data, &env); err != nil {
		return types.QueryResultData{}, WrapError(500, "decode_failed",
			"query envelope decode", err)
	}
	result, err := types.QueryResultDataFromEntity(env.Root)
	if err != nil {
		return types.QueryResultData{}, WrapError(500, "decode_failed", "query decode", err)
	}

	if ex.log != nil {
		ex.log.Verbosef("query find → %d matches (%d total)", len(result.Matches), result.Total)
	}
	return result, nil
}

// QueryCount executes a count operation against the system/query handler.
// Returns the number of matching entities.
func (ex *Executor) QueryCount(expr types.QueryExpressionData) (uint64, error) {
	paramEnt, err := expr.ToEntity()
	if err != nil {
		return 0, WrapError(400, "encode_failed", "query encode", err)
	}

	resp, err := ex.ExecuteWithParams("system/query", "count", paramEnt)
	if err != nil {
		return 0, err
	}

	// Count returns a primitive/uint — decode from the result entity data
	var count uint64
	if err := decodeEntityField(resp.Data, &count); err != nil {
		return 0, WrapError(500, "decode_failed", "query count decode", err)
	}

	if ex.log != nil {
		ex.log.Verbosef("query count → %d", count)
	}
	return count, nil
}

// decodeEntityField decodes a single CBOR value from entity data.
func decodeEntityField(data []byte, v interface{}) error {
	if len(data) == 0 {
		return fmt.Errorf("empty data")
	}
	return ecf.Decode(data, v)
}
