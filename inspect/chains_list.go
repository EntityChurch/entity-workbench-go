package inspect

// Chain enumeration — answers "what chains have artifacts in this
// peer's store right now?" without needing the caller to know any
// chain_ids ahead of time.
//
// Composes from existing primitives (no new hooks): walks the
// location index for chain-error markers + continuation entities,
// extracts chain_ids, infers status per the v1.1 §9 #8 chain-
// participation invariants framing.

import (
	"sort"
	"strings"

	coretypes "go.entitychurch.org/entity-core-go/core/types"

	"github.com/fxamacker/cbor/v2"

	"entity-workbench-go/entitysdk"
)

// ChainStatus is the inferred lifecycle state of a chain based on
// the artifacts present in the local store. Pre-§9 #8 completion
// contracts, status is best-effort; with declarations in place the
// inference becomes mechanically authoritative.
//
// IMPORTANT (substrate honesty): chain_id is not embedded in installed
// continuation entities — it flows through dispatch context. The only
// substrate-discoverable chain_id sources today are (a) chain-error
// markers and (b) suspended continuation entities. Installed forward/
// join continuations are invisible to ListChains by chain_id; the
// operator's view of installed chains is via `continuation ls` (by
// path, not by chain_id).
type ChainStatus int

const (
	// ChainStatusUnknown — chain_id appears in some artifact but
	// status cannot be inferred from the artifact mix.
	ChainStatusUnknown ChainStatus = iota
	// ChainStatusSuspended — chain has a suspended continuation
	// awaiting resume.
	ChainStatusSuspended
	// ChainStatusFailed — chain-error marker present (at least one
	// step failed; chain may or may not still be active).
	ChainStatusFailed
	// ChainStatusMixed — both suspended continuation and error
	// marker present.
	ChainStatusMixed
)

func (s ChainStatus) String() string {
	switch s {
	case ChainStatusSuspended:
		return "suspended"
	case ChainStatusFailed:
		return "failed"
	case ChainStatusMixed:
		return "mixed"
	default:
		return "unknown"
	}
}

// ChainSummary is the per-chain enumeration entry returned by
// ListChains.
type ChainSummary struct {
	ChainID          string
	Status           ChainStatus
	MarkerCount      int
	SuspendedCount   int
	PathBindingCount int // total path bindings carrying this chain_id

	// LastReason is the {reason} segment of the most recently
	// observed chain-error marker, or empty if no markers.
	LastReason string

	// LastTimestamp is the most recent ChainErrorLostData.Timestamp
	// across markers for this chain (unix milli; 0 if no markers).
	LastTimestamp uint64
}

// ListChains enumerates distinct chain_ids that have at least one
// artifact in the local store. Walks the location index once;
// O(bindings) per peer.
//
// Status inference is best-effort per v1.1 §9 #8 — without declared
// completion contracts an entity is "succeeded" implicitly when its
// continuation has cleared with no marker. Adopt the §9 #8 audit's
// per-extension contracts to make this mechanical.
func ListChains(peer *entitysdk.AppPeer) []ChainSummary {
	byChain := map[string]*ChainSummary{}

	for _, e := range peer.Store().List("") {
		path := e.Path

		// Chain-error markers: path is .../system/runtime/chain-errors/{kind}/{chain_id}/{step}/{reason}/{hash}
		if idx := strings.Index(path, "system/runtime/chain-errors/"); idx >= 0 {
			tail := path[idx+len("system/runtime/chain-errors/"):]
			parts := strings.Split(tail, "/")
			if len(parts) >= 4 {
				chainID := parts[1]
				cs := getOrCreate(byChain, chainID)
				cs.MarkerCount++
				cs.PathBindingCount++

				// Decode the marker body for timestamp + reason.
				if ent, ok := peer.Store().GetByHash(e.Hash); ok {
					var body coretypes.ChainErrorLostData
					if err := cbor.Unmarshal(ent.Data, &body); err == nil {
						if body.Timestamp > cs.LastTimestamp {
							cs.LastTimestamp = body.Timestamp
							cs.LastReason = body.Reason
						}
					}
				}
				continue
			}
		}

		// Suspended continuations carry chain_id in their body (per
		// EXTENSION-CONTINUATION §6.x recovery semantics). Installed
		// forward/join continuations do NOT — chain_id flows through
		// dispatch context. ListChains is honest about this scope.
		if strings.Contains(path, "system/continuation/") {
			if ent, ok := peer.Store().GetByHash(e.Hash); ok {
				if ent.Type == coretypes.TypeContinuationSuspended {
					var body coretypes.ContinuationSuspendedData
					if err := cbor.Unmarshal(ent.Data, &body); err == nil && body.ChainID != "" {
						cs := getOrCreate(byChain, body.ChainID)
						cs.SuspendedCount++
						cs.PathBindingCount++
					}
				}
			}
		}
	}

	// Infer status per the artifact mix.
	out := make([]ChainSummary, 0, len(byChain))
	for _, cs := range byChain {
		if cs.MarkerCount > 0 && cs.SuspendedCount > 0 {
			cs.Status = ChainStatusMixed
		} else if cs.MarkerCount > 0 {
			cs.Status = ChainStatusFailed
		} else if cs.SuspendedCount > 0 {
			cs.Status = ChainStatusSuspended
		} else {
			cs.Status = ChainStatusUnknown
		}
		out = append(out, *cs)
	}

	// Default ordering: most recent first by timestamp (when known),
	// then by chain_id for stable order across chains with same
	// timestamp.
	sort.Slice(out, func(i, j int) bool {
		if out[i].LastTimestamp != out[j].LastTimestamp {
			return out[i].LastTimestamp > out[j].LastTimestamp
		}
		return out[i].ChainID < out[j].ChainID
	})

	return out
}

func getOrCreate(m map[string]*ChainSummary, chainID string) *ChainSummary {
	cs, ok := m[chainID]
	if !ok {
		cs = &ChainSummary{ChainID: chainID}
		m[chainID] = cs
	}
	return cs
}
