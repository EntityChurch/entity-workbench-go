package workbench

import (
	"sort"
	"strings"

	"entity-workbench-go/entitysdk"
)

// MountValidationResult reports what's already bound under a
// candidate mount's source + target prefixes. Empty result =
// fresh mount.
type MountValidationResult struct {
	SourcePrefix string
	TargetPrefix string

	// TargetTotal is the total count of bindings under TargetPrefix.
	TargetTotal int

	// TargetExpected is the count of bindings whose type matches one
	// of the expected types (doc/markdown-file today; extensible as
	// the per-type viewer registry grows).
	TargetExpected int

	// TargetForeign maps each unexpected entity type to its
	// occurrence count under TargetPrefix. Sorted alphabetically
	// by type when listed via ForeignTypeOrder.
	TargetForeign map[string]int

	// SourceTotal is the total count of bindings under SourcePrefix
	// (i.e. previous-mount residue at the localfiles namespace).
	SourceTotal int
}

// HasConflict reports whether any non-expected bindings sit under
// TargetPrefix. Callers refuse the mount when this is true unless
// the operator has supplied -force.
func (r MountValidationResult) HasConflict() bool {
	return len(r.TargetForeign) > 0
}

// ForeignTypeOrder returns the foreign types sorted alphabetically
// for stable operator-facing output.
func (r MountValidationResult) ForeignTypeOrder() []string {
	types := make([]string, 0, len(r.TargetForeign))
	for t := range r.TargetForeign {
		types = append(types, t)
	}
	sort.Strings(types)
	return types
}

// ValidateMountTarget walks the candidate mount's source + target
// prefixes and reports what's already bound. Phase E v2 §7.4 —
// pure read-only inspection on the local peer's Level 0 store.
//
// `expectedTypes` is the set of entity types the workbench owns at
// the target prefix. Today that's just doc/markdown-file; when the
// ingest layer generalizes to a per-extension type registry (item
// 16 of the roadmap), the caller passes the expanded set.
func ValidateMountTarget(ap *entitysdk.AppPeer, sourcePrefix, targetPrefix string, expectedTypes []string) MountValidationResult {
	res := MountValidationResult{
		SourcePrefix:  sourcePrefix,
		TargetPrefix:  targetPrefix,
		TargetForeign: map[string]int{},
	}
	if !strings.HasSuffix(sourcePrefix, "/") {
		sourcePrefix += "/"
		res.SourcePrefix = sourcePrefix
	}
	if !strings.HasSuffix(targetPrefix, "/") {
		targetPrefix += "/"
		res.TargetPrefix = targetPrefix
	}

	expectedSet := map[string]struct{}{}
	for _, t := range expectedTypes {
		expectedSet[t] = struct{}{}
	}

	st := ap.Store()
	for _, e := range st.List(targetPrefix) {
		res.TargetTotal++
		ent, ok := st.GetByHash(e.Hash)
		if !ok {
			continue
		}
		if _, isExpected := expectedSet[ent.Type]; isExpected {
			res.TargetExpected++
			continue
		}
		res.TargetForeign[ent.Type]++
	}
	res.SourceTotal = len(st.List(sourcePrefix))
	return res
}
