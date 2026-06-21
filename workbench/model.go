package workbench

// Model is the contract every workbench panel model satisfies.
//
// A panel is structured as model -> Output struct -> renderer. The
// model owns business logic and per-panel state; Render produces a
// renderer-neutral Output that any renderer (tview console, raylib
// canvas, future text-mode shell) can consume without reaching into
// model internals. The Output type is the cross-impl contract; the
// model's internal shape is per-impl idiomatic.
//
// State that depends on external inputs (e.g. the currently selected
// path for an entity-detail panel) is set via dedicated setter
// methods on the concrete model — Render itself takes no arguments.
// This keeps Render a pure function of model state and matches the
// TEA-shaped pattern called out in the application-layer convention
// (see GUIDE-ENTITY-WORKBENCH-APP.md).
type Model[Out any] interface {
	Render() Out
}
