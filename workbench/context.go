package workbench

// DataContext is the (now empty) marker for renderer data-access
// contexts.
//
// History: it once vended Entries / Resolve / EntityCount / MarkDirty
// (removed with the cache-then-filter UI-refresh
// anti-pattern) and then only Selection() (removed when
// SelectionState was deleted — the selection value-of-truth is the
// entity-tree slot, read/written via WorkspaceState, so renderers no
// longer need a shared in-memory selection accessor). Panel models now
// own their prefix + selection subscriptions and local state.
//
// The interface is retained as an empty marker only so the canvas
// `viewContext`/`commandContext` embedding chain and panel signatures
// stay unchanged; it can be deleted outright in a later pass that also
// collapses canvas's viewContext.
type DataContext interface{}
