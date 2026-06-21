package entitysdk

import (
	"sort"
	"strings"

	"go.entitychurch.org/entity-core-go/core/store"
	"go.entitychurch.org/entity-core-go/core/types"
)

// HandlerInfo describes a discovered handler with its operations.
type HandlerInfo struct {
	Pattern    string
	Name       string
	Operations []string
	Specs      map[string]types.HandlerOperationSpec
}

// DiscoverHandlers lists the system/handler/* entries visible in the
// local peer's tree, resolves each through the PeerContext, and
// returns handler info sorted by pattern. Matches SDK-OPERATIONS §9.1.
func DiscoverHandlers(pc *PeerContext) []HandlerInfo {
	entries := pc.Store().List("system/handler/")
	return discoverHandlersIn(entries, pc.Resolve)
}

// DiscoverHandlersFromEntries is the lower-level variant that takes
// explicit entries and a resolver. Used by tests that don't have a
// full PeerContext. The entries are filtered to `system/handler/*`
// here (matching both bare and peer-qualified absolute paths), so
// callers can pass the full entry list without pre-filtering.
func DiscoverHandlersFromEntries(entries []store.LocationEntry, resolve func(path string) (ResolvedEntity, bool)) []HandlerInfo {
	filtered := make([]store.LocationEntry, 0, len(entries))
	for _, e := range entries {
		if isSystemHandlerPath(e.Path) {
			filtered = append(filtered, e)
		}
	}
	return discoverHandlersIn(filtered, resolve)
}

func discoverHandlersIn(entries []store.LocationEntry, resolve func(path string) (ResolvedEntity, bool)) []HandlerInfo {
	var handlers []HandlerInfo
	for _, entry := range entries {
		r, ok := resolve(entry.Path)
		if !ok {
			continue
		}
		data, err := types.HandlerInterfaceDataFromEntity(r.Entity)
		if err != nil {
			continue
		}
		ops := make([]string, 0, len(data.Operations))
		for op := range data.Operations {
			ops = append(ops, op)
		}
		sort.Strings(ops)

		handlers = append(handlers, HandlerInfo{
			Pattern:    data.Pattern,
			Name:       data.Name,
			Operations: ops,
			Specs:      data.Operations,
		})
	}
	sort.Slice(handlers, func(i, j int) bool {
		return handlers[i].Pattern < handlers[j].Pattern
	})
	return handlers
}

// isSystemHandlerPath accepts both the bare form ("system/handler/...")
// and the peer-qualified absolute form ("/{peerID}/system/handler/...").
func isSystemHandlerPath(path string) bool {
	if strings.HasPrefix(path, "system/handler/") {
		return true
	}
	return containsSegment(path, "system/handler/")
}

// containsSegment checks whether path contains segment as a path-
// segment match (preceded by "/" and not starting at position 0).
// Used to match peer-qualified paths like "/{pid}/system/{something}".
func containsSegment(path, segment string) bool {
	idx := strings.Index(path, "/"+segment)
	return idx >= 0
}
