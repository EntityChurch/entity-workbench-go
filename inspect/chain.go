package inspect

// Chain trace — given a chain_id, walk the related entities a chain
// produces: continuation entries, dispatched continuation step
// entities, chain-error-lost markers, history entries. Returns an
// ordered, decoded view so callers can answer "what happened to chain
// X" without manually composing path enumerate + entity dump.
//
// Composes from existing primitives (path enumerate + entity decode
// + optional content stream). No new substrate hooks.

import (
	"strings"

	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/store"
	coretypes "go.entitychurch.org/entity-core-go/core/types"

	"github.com/fxamacker/cbor/v2"

	"entity-workbench-go/entitysdk"
)

// ChainTrace is the assembled picture of one chain's footprint in a
// peer's content store + location index at the moment of the call.
type ChainTrace struct {
	ChainID string

	// Errors are chain-error-lost markers attributable to this chain,
	// in path order. v1.20 paths:
	// system/runtime/chain-errors/{kind}/{chain_id}/{step_index}/...
	Errors []ChainErrorMarker

	// Continuations are continuation entries whose body carries this
	// chain_id. Returned in the order their bindings were enumerated.
	Continuations []ContinuationEntry

	// PathBindings are *all* path bindings whose path contains the
	// chain_id as a segment. Catches anything not enumerated above —
	// runtime debug paths, custom extension bindings, etc.
	PathBindings []store.LocationEntry

	// ContentEvents are content-stream events whose decoded body has
	// chain_id matching, populated only when TraceChainWithStream was
	// used. Provides time-ordered traversal of every entity the chain
	// produced regardless of binding.
	ContentEvents []ChainContentRef
}

// ChainErrorMarker is a decoded chain-error-lost entry with the
// path-derived breakdown alongside the decoded body.
type ChainErrorMarker struct {
	// Path segments per v1.20 path scheme.
	Kind      string // "lost" / "rejected" / ...
	ChainID   string
	StepIndex string
	Reason    string

	Path string
	Hash string

	Body coretypes.ChainErrorLostData
}

// ContinuationEntry is a decoded continuation entity carrying the
// chain_id of interest.
type ContinuationEntry struct {
	Path string
	Hash string
	Body coretypes.ContinuationData
}

// ChainContentRef is a content-stream event whose decoded body refers
// to the chain_id of interest.
type ChainContentRef struct {
	Event ContentEvent
	// Field is the key under which chain_id was found ("chain_id",
	// "parent_chain_id", "bounds.chain_id" — recorded for diagnostic
	// transparency).
	Field string
}

// TraceChain assembles a ChainTrace for the given chain_id by walking
// the peer's location index + content store. Cheap on small stores;
// O(bindings) on large.
func TraceChain(peer *entitysdk.AppPeer, chainID string) *ChainTrace {
	return traceChain(peer, chainID, nil)
}

// TraceChainWithStream augments TraceChain with a time-ordered list
// of content-stream events whose decoded body references the chain_id.
// Requires the stream to have been installed at peer construction.
func TraceChainWithStream(peer *entitysdk.AppPeer, chainID string, stream *ContentStream) *ChainTrace {
	return traceChain(peer, chainID, stream)
}

func traceChain(peer *entitysdk.AppPeer, chainID string, stream *ContentStream) *ChainTrace {
	t := &ChainTrace{ChainID: chainID}

	// Path enumerate — store List returns paths with the peer-id
	// namespace prefix, so match by substring rather than HasPrefix.
	// chain_id appears as a path segment surrounded by '/'.
	chainSeg := "/" + chainID + "/"
	for _, e := range peer.Store().List("") {
		path := e.Path
		matchesChain := strings.Contains(path, chainSeg) ||
			strings.HasSuffix(path, "/"+chainID)
		if !matchesChain {
			continue
		}
		switch {
		case strings.Contains(path, "system/runtime/chain-errors/"):
			if m, ok := decodeChainErrorMarker(peer, e); ok {
				t.Errors = append(t.Errors, m)
			}
			t.PathBindings = append(t.PathBindings, e)
		case strings.Contains(path, "system/continuation/"):
			if c, ok := decodeContinuationAtPath(peer, e, chainID); ok {
				t.Continuations = append(t.Continuations, c)
			}
			t.PathBindings = append(t.PathBindings, e)
		default:
			t.PathBindings = append(t.PathBindings, e)
		}
	}

	if stream != nil {
		for _, evt := range stream.Events() {
			if ref, ok := streamEventMatchesChain(peer, evt, chainID); ok {
				t.ContentEvents = append(t.ContentEvents, ref)
			}
		}
	}

	return t
}

// DecodeChainErrorMarker decodes a chain-error-lost entity body into
// the typed marker shape. Returns false on type mismatch or decode
// failure. Lightweight variant used by streaming surfaces that already
// have the entity in hand (no store lookup needed). The path-derived
// segments (Kind, ChainID, StepIndex, Reason) are NOT populated by
// this entry point — callers that need them pass through the path
// alongside the entity.
func DecodeChainErrorMarker(ent entity.Entity) (ChainErrorMarker, bool) {
	var body coretypes.ChainErrorLostData
	if err := cbor.Unmarshal(ent.Data, &body); err != nil {
		return ChainErrorMarker{}, false
	}
	return ChainErrorMarker{
		Hash:      ent.ContentHash.String(),
		Body:      body,
		ChainID:   body.ChainID,
		StepIndex: body.StepIndex,
		Reason:    body.Reason,
	}, true
}

func decodeChainErrorMarker(peer *entitysdk.AppPeer, e store.LocationEntry) (ChainErrorMarker, bool) {
	ent, ok := peer.Store().GetByHash(e.Hash)
	if !ok {
		return ChainErrorMarker{}, false
	}
	var body coretypes.ChainErrorLostData
	if err := cbor.Unmarshal(ent.Data, &body); err != nil {
		return ChainErrorMarker{}, false
	}
	m := ChainErrorMarker{
		Path: e.Path,
		Hash: ent.ContentHash.String(),
		Body: body,
	}
	// Path: .../system/runtime/chain-errors/{kind}/{chain_id}/{step_index}/{reason}/{marker_hash}
	if idx := strings.Index(e.Path, "system/runtime/chain-errors/"); idx >= 0 {
		tail := e.Path[idx+len("system/runtime/chain-errors/"):]
		parts := strings.Split(tail, "/")
		if len(parts) >= 4 {
			m.Kind = parts[0]
			m.ChainID = parts[1]
			m.StepIndex = parts[2]
			m.Reason = parts[3]
		}
	}
	return m, true
}

func decodeContinuationAtPath(peer *entitysdk.AppPeer, e store.LocationEntry, chainID string) (ContinuationEntry, bool) {
	ent, ok := peer.Store().GetByHash(e.Hash)
	if !ok {
		return ContinuationEntry{}, false
	}
	body, err := coretypes.ContinuationDataFromEntity(ent)
	if err != nil {
		return ContinuationEntry{}, false
	}
	// ContinuationData carries chain_id at the top level.
	if !continuationCarriesChainID(body, chainID) {
		return ContinuationEntry{}, false
	}
	return ContinuationEntry{
		Path: e.Path,
		Hash: ent.ContentHash.String(),
		Body: body,
	}, true
}

// continuationCarriesChainID checks whether a continuation references
// the chain_id either directly or via its bounds.
func continuationCarriesChainID(_ coretypes.ContinuationData, _ string) bool {
	// ContinuationData itself has no top-level chain_id today (chain_id
	// is bound at install time + travels in dispatch context). We
	// surface continuation entries via path-segment match in the
	// caller — this hook is here so a future ContinuationData layout
	// that does embed chain_id can light it up without a caller change.
	return false
}

func streamEventMatchesChain(peer *entitysdk.AppPeer, evt ContentEvent, chainID string) (ChainContentRef, bool) {
	ent, ok := peer.Store().GetByHash(evt.Hash)
	if !ok {
		return ChainContentRef{}, false
	}
	// Best-effort generic decode + field scan. Covers chain_id /
	// parent_chain_id at any nesting depth.
	var body interface{}
	if err := cbor.Unmarshal(ent.Data, &body); err != nil {
		return ChainContentRef{}, false
	}
	if field, ok := findChainIDField(body, chainID, ""); ok {
		return ChainContentRef{Event: evt, Field: field}, true
	}
	return ChainContentRef{}, false
}

// findChainIDField walks a CBOR-decoded value looking for a string
// equal to chainID under any key whose name contains "chain_id".
// Returns the dotted path to the field for diagnostic surface.
func findChainIDField(v interface{}, chainID, prefix string) (string, bool) {
	switch x := v.(type) {
	case map[interface{}]interface{}:
		for k, vv := range x {
			ks, _ := k.(string)
			if ks == "" {
				continue
			}
			full := ks
			if prefix != "" {
				full = prefix + "." + ks
			}
			if strings.Contains(ks, "chain_id") {
				if s, ok := vv.(string); ok && s == chainID {
					return full, true
				}
			}
			if got, ok := findChainIDField(vv, chainID, full); ok {
				return got, true
			}
		}
	case map[string]interface{}:
		for k, vv := range x {
			full := k
			if prefix != "" {
				full = prefix + "." + k
			}
			if strings.Contains(k, "chain_id") {
				if s, ok := vv.(string); ok && s == chainID {
					return full, true
				}
			}
			if got, ok := findChainIDField(vv, chainID, full); ok {
				return got, true
			}
		}
	case []interface{}:
		for i, vv := range x {
			if got, ok := findChainIDField(vv, chainID, prefix); ok {
				_ = i
				return got, true
			}
		}
	}
	return "", false
}

// Summary returns a one-line summary suitable for test log output.
func (t *ChainTrace) Summary() string {
	var b strings.Builder
	b.WriteString("chain ")
	b.WriteString(t.ChainID)
	b.WriteString(": ")
	if len(t.Errors) > 0 {
		b.WriteString("errors=")
		writeInt(&b, len(t.Errors))
	}
	if len(t.Continuations) > 0 {
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		b.WriteString("continuations=")
		writeInt(&b, len(t.Continuations))
	}
	if len(t.PathBindings) > 0 {
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		b.WriteString("bindings=")
		writeInt(&b, len(t.PathBindings))
	}
	if len(t.ContentEvents) > 0 {
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		b.WriteString("content=")
		writeInt(&b, len(t.ContentEvents))
	}
	return b.String()
}

func writeInt(b *strings.Builder, n int) {
	if n == 0 {
		b.WriteByte('0')
		return
	}
	if n < 0 {
		b.WriteByte('-')
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	b.Write(buf[i:])
}
