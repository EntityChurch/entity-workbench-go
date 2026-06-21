package entitysdk

import (
	"context"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/types"

	"github.com/fxamacker/cbor/v2"
)

// ClockClient wraps system/clock EXECUTE operations behind typed Go
// methods. Each ClockClient targets one peer (local or remote, selected
// at construction time via AppPeer.Clock / AppPeer.ClockAt).
//
// Per SDK-EXTENSION-OPERATIONS §10 + EXTENSION-CLOCK. The clock
// extension is wired default-on by CreatePeer; SetupAdvancement runs
// in assembleAppPeer so Now reflects the configured mode (wall /
// logical / vector / hlc) without further wiring.
//
// Tick is intentionally not wrapped: it delegates to system/subscription
// internally, and the subscription bridge (AppPeer.Subscribe) is the
// idiomatic surface for periodic-event streams.
type ClockClient struct {
	ap       *AppPeer
	target   string
	clockURI string
}

// Clock returns a ClockClient targeting the local peer.
func (a *AppPeer) Clock() *ClockClient {
	return &ClockClient{
		ap:       a,
		target:   a.PeerID(),
		clockURI: "system/clock",
	}
}

// ClockAt returns a ClockClient targeting the named remote peer.
// Operations dispatch through the local peer's connection pool.
func (a *AppPeer) ClockAt(peerID string) *ClockClient {
	return &ClockClient{
		ap:       a,
		target:   peerID,
		clockURI: extPeerURI(a.PeerID(), peerID, "system/clock"),
	}
}

// PeerID returns the peer-id this ClockClient targets.
func (cc *ClockClient) PeerID() string { return cc.target }

// Now returns the current clock state per the targeted peer's
// configured mode (system/clock/config). The returned ClockStateData
// always includes Mode; Timestamp / Logical / Vector / HLC are
// populated according to the configured mode (Timestamp is included
// for any mode unless explicitly disabled via config.WallClock=false).
func (cc *ClockClient) Now(ctx context.Context) (types.ClockStateData, error) {
	resultEnt, err := extDispatch(cc.ap, cc.clockURI, "now", "", emptyParamsEntity())
	if err != nil {
		return types.ClockStateData{}, err
	}
	state, err := types.ClockStateDataFromEntity(resultEnt)
	if err != nil {
		return types.ClockStateData{}, WrapError(500, "decode_result", "decode ClockState", err)
	}
	return state, nil
}

// Compare orders two clock values. a and b are CBOR-encoded clock
// values of the same kind — Timestamp, Logical, Vector, or HLC.
// The handler detects the kind from CBOR map keys; mismatched kinds
// return a 400 invalid_params error.
//
// Returns one of "before", "after", "equal", or "concurrent" (only
// vector clocks can yield "concurrent").
func (cc *ClockClient) Compare(ctx context.Context, a, b cbor.RawMessage) (string, error) {
	params := types.ClockCompareParamsData{A: a, B: b}
	paramEnt, err := encodeAsEntity(types.TypeClockCompareParams, params)
	if err != nil {
		return "", WrapError(500, "encode_request", "encode ClockCompareParams", err)
	}
	resultEnt, err := extDispatch(cc.ap, cc.clockURI, "compare", "", paramEnt)
	if err != nil {
		return "", err
	}
	var result types.ClockCompareResultData
	if err := decodeEntityField(resultEnt.Data, &result); err != nil {
		return "", WrapError(500, "decode_result", "decode ClockCompareResult", err)
	}
	return result.Order, nil
}

// encodeAsEntity is a small wrapper around an explicit-type CBOR encode +
// entity.NewEntity call. ClockCompareParamsData has no ToEntity helper
// in core-go because the handler only ever reads it; the SDK is the
// first writer.
func encodeAsEntity(typeName string, v interface{}) (entity.Entity, error) {
	raw, err := ecf.Encode(v)
	if err != nil {
		return entity.Entity{}, err
	}
	return entity.NewEntity(typeName, cbor.RawMessage(raw))
}
