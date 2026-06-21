package entitysdk

import (
	"sort"
	"strings"

	"go.entitychurch.org/entity-core-go/core/store"
	"go.entitychurch.org/entity-core-go/core/types"
)

// TypeInfo describes a type definition visible on the local peer.
// Matches the SDK-OPERATIONS §9.2 PeerInfo-style shape for types.
type TypeInfo struct {
	Name    string
	Extends string
	Fields  map[string]types.FieldSpec
}

// DiscoverTypes lists the system/type/* entries visible in the local
// peer's tree, decodes each as a TypeDefinition, and returns type
// info sorted by name. Matches SDK-OPERATIONS §9.2 (SHOULD).
func DiscoverTypes(pc *PeerContext) []TypeInfo {
	entries := pc.Store().List("system/type/")
	return discoverTypesIn(entries, pc.Resolve)
}

// DiscoverTypesFromEntries is the lower-level variant that takes
// explicit entries and a resolver. See DiscoverHandlersFromEntries
// for the intended use.
func DiscoverTypesFromEntries(entries []store.LocationEntry, resolve func(path string) (ResolvedEntity, bool)) []TypeInfo {
	filtered := make([]store.LocationEntry, 0, len(entries))
	for _, e := range entries {
		if isSystemTypePath(e.Path) {
			filtered = append(filtered, e)
		}
	}
	return discoverTypesIn(filtered, resolve)
}

func discoverTypesIn(entries []store.LocationEntry, resolve func(path string) (ResolvedEntity, bool)) []TypeInfo {
	var out []TypeInfo
	for _, entry := range entries {
		r, ok := resolve(entry.Path)
		if !ok {
			continue
		}
		// TypeDefinition has no dedicated FromEntity helper — decode
		// the data payload directly.
		var td types.TypeDefinition
		if err := decodeEntityField(r.Entity.Data, &td); err != nil {
			continue
		}
		out = append(out, TypeInfo{
			Name:    td.Name,
			Extends: td.Extends,
			Fields:  td.Fields,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func isSystemTypePath(path string) bool {
	if strings.HasPrefix(path, "system/type/") {
		return true
	}
	return containsSegment(path, "system/type/")
}
