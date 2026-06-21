package entitysdk

// ActionEvent names the canonical user-action vocabulary defined in
// GUIDE-ENTITY-WORKBENCH-APP.md §5.3 + §6. Action events are the
// **application-agnostic** pub-sub channel — `navigate`, `select`, and
// friends mean the same thing in any application that adopts the
// convention (a knowledge-base app and a calculator app would emit
// the same names with the same propagation defaults).
//
// Action events are distinct from **commands** (workbench-specific
// shell verbs: cd, ls, connect, mount, exec, …) which live in
// shellcmd/. A command may emit one or more action events as a side
// effect — `cd <path>` produces a navigate(path) — but the action
// vocabulary itself is shared across applications and render contexts.
// See `SHELL-DIRECTION.md §8.4` ("Two vocabularies, one render
// context") for the design discussion.
//
// The list below is **not closed** — content-types may register new
// events as needed. New events should be named and registered
// explicitly, never absorbed into a generic catch-all (the guide
// deprecates `set_field` for exactly this reason: replay/auditing
// require named actions).
type ActionEvent string

const (
	// EventNavigate — "I'm attending to this path." Value: text (path).
	// Typical producer: tree-browser cursor move, shell `cd`. Default
	// propagation: context (other panels in the same presentation
	// context may co-orient).
	EventNavigate ActionEvent = "navigate"

	// EventSelect — "I have chosen this path." Value: text (path).
	// Typical producer: tree-browser enter, query-console result click.
	// Default propagation: context.
	EventSelect ActionEvent = "select"

	// EventSubmit — "execute the buffered command/query." Value: empty.
	// Typical producer: execute-console, query-console. Default
	// propagation: panel (local — submission is a within-panel act).
	EventSubmit ActionEvent = "submit"

	// EventClear — "clear my output / buffer." Value: empty. Typical
	// producer: event-log clear hotkey, execute-console clear.
	// Default propagation: panel.
	EventClear ActionEvent = "clear"

	// EventSetFilter — "filter view by this." Value: text (filter
	// expression — often a level name for event-log). Default
	// propagation: panel.
	EventSetFilter ActionEvent = "set_filter"

	// EventToggleRaw — "toggle raw/decoded view." Value: empty.
	// Typical producer: entity-detail. Default propagation: panel
	// (purely a view-state change).
	EventToggleRaw ActionEvent = "toggle_raw"
)

// Propagation describes whether an action event propagates beyond the
// panel that emitted it.
type Propagation uint8

const (
	// PropPanel — action stays local to the emitting panel; no other
	// panel observes it through the per-context channel.
	PropPanel Propagation = iota

	// PropContext — action is written to the presentation-context-level
	// channel (per-screen selection slot today); other panels in the
	// same context may subscribe and co-orient.
	PropContext
)

// DefaultPropagation returns the canonical default propagation for an
// action event per guide §5.3. Unknown events default to PropPanel —
// the safe choice; a content-type that wants context propagation must
// declare it explicitly.
//
// Per-panel-instance overrides layer on top of these defaults via
// content-type metadata at registration time; see SHELL-DIRECTION §8.4
// Stage 4 item 5.
func DefaultPropagation(event ActionEvent) Propagation {
	switch event {
	case EventNavigate, EventSelect:
		return PropContext
	case EventSubmit, EventClear, EventSetFilter, EventToggleRaw:
		return PropPanel
	default:
		return PropPanel
	}
}
