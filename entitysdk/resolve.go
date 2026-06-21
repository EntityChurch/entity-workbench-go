package entitysdk

import (
	"github.com/fxamacker/cbor/v2"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/store"
)

// ResolvedEntity is the result of looking up an entity by path.
// It bundles the path, hash, entity, and decoded CBOR data together
// so callers don't have to repeat the multi-step lookup.
type ResolvedEntity struct {
	Path    string
	Hash    hash.Hash
	Entity  entity.Entity
	Decoded interface{} // nil if CBOR decode failed
}

// ResolveEntity looks up an entity by path through a PeerContext.
// This is the standard library way to resolve entities — applications
// use this instead of accessing store/index directly.
func ResolveEntity(pc *PeerContext, path string) (ResolvedEntity, bool) {
	return pc.Resolve(path)
}

// DecodeEntityData decodes CBOR entity data into a generic interface{}.
// Returns (decoded, true) on success, or (nil, false) on decode failure.
func DecodeEntityData(data []byte) (interface{}, bool) {
	if len(data) == 0 {
		return nil, false
	}
	var decoded interface{}
	if err := ecf.Decode(data, &decoded); err != nil {
		return nil, false
	}
	return decoded, true
}

// decodeEntityData is the internal helper used by PeerContext.Resolve.
func decodeEntityData(data []byte, v *interface{}) error {
	return ecf.Decode(data, v)
}

// encodeData encodes a value to CBOR raw message.
func encodeData(v interface{}) (cbor.RawMessage, error) {
	raw, err := ecf.Encode(v)
	if err != nil {
		return nil, err
	}
	return cbor.RawMessage(raw), nil
}

// ListByPrefix returns all entries whose path starts with the given prefix.
// An empty prefix returns all entries.
func ListByPrefix(entries []store.LocationEntry, prefix string) []store.LocationEntry {
	if prefix == "" {
		return entries
	}
	var result []store.LocationEntry
	for _, e := range entries {
		if len(e.Path) >= len(prefix) && e.Path[:len(prefix)] == prefix {
			result = append(result, e)
		}
	}
	return result
}
