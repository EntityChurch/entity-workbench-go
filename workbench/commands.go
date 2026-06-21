package workbench

import "strings"

// ActionKind identifies what an action produces.
type ActionKind int

const (
	ActionNone           ActionKind = iota
	ActionSetContent                // system: change the focused window's content type
	ActionSplitH                    // system: split window horizontally (left | right)
	ActionSplitV                    // system: split window vertically (top / bottom)
	ActionCloseWindow               // system: close the focused window
	ActionToggleEditMode            // system: toggle edit mode
	ActionWindowEvent               // routed to the focused window's content
)

// Action is the result of executing a command or handling user input.
// The workspace interprets system actions directly. Window events are
// routed to the focused window's content for interpretation.
//
// This follows the Rust team's WindowEvent pattern: actions are data
// (kind + string event/value), serializable, and renderer-agnostic.
type Action struct {
	Kind        ActionKind
	ContentName string // for ActionSetContent: "tree-browser", "entity-detail", "empty"
	Event       string // for ActionWindowEvent: event name (e.g. "toggle_raw")
	Value       string // for ActionWindowEvent: event data
}

// ContentType describes an available panel content type.
// This is the single source of truth — command palette, picker UI,
// and startup configs all read from this list.
type ContentType struct {
	Name string // identifier used in factories and state (e.g. "tree-browser")
	Desc string // human-readable description
}

// ContentTypes lists all available panel content types.
var ContentTypes = []ContentType{
	{"tree-browser", "Browse entity tree with search and navigation"},
	{"entity-detail", "Inspect entity data with CBOR rendering"},
	{"entity-shell", "Interactive REPL for entity operations"},
	{"execute-console", "Handler discovery and execution"},
	{"log-viewer", "Real-time event log with level filtering"},
	{"peer-info", "Peer status and entity listing"},
	{"query-browser", "Query entities by type, path, or reference"},
	{"markdown-files", "Browse doc/markdown-file entities under any prefix"},
	{"markdown-view", "Read or edit a markdown file entity"},
}

// Command is a named action available from the command palette.
type Command struct {
	Name   string
	Desc   string
	Action Action
}

// Registry holds all available commands.
var Registry []Command

// Init populates the command registry from ContentTypes + structural commands.
func Init() {
	Registry = nil

	// Content type commands (generated from ContentTypes)
	for _, ct := range ContentTypes {
		Registry = append(Registry, Command{
			Name:   "new-" + ct.Name,
			Desc:   ct.Desc,
			Action: Action{Kind: ActionSetContent, ContentName: ct.Name},
		})
	}
	Registry = append(Registry, Command{
		"new-empty", "Reset to content picker",
		Action{Kind: ActionSetContent, ContentName: "empty"},
	})

	// Structural commands
	Registry = append(Registry, []Command{
		{"split-horizontal", "Split window left | right", Action{Kind: ActionSplitH}},
		{"split-vertical", "Split window top / bottom", Action{Kind: ActionSplitV}},
		{"close-window", "Close this window", Action{Kind: ActionCloseWindow}},
	}...)

	// Window events
	Registry = append(Registry, []Command{
		{"toggle-raw", "Toggle raw/rendered view", Action{Kind: ActionWindowEvent, Event: "toggle_raw"}},
		{"clear-log", "Clear event log", Action{Kind: ActionWindowEvent, Event: "clear"}},
	}...)
}

// FilterCommands returns indices into Registry matching the query.
func FilterCommands(query string) []int {
	q := strings.ToLower(query)
	var results []int
	for i, cmd := range Registry {
		if q == "" || strings.Contains(strings.ToLower(cmd.Name), q) || strings.Contains(strings.ToLower(cmd.Desc), q) {
			results = append(results, i)
		}
	}
	return results
}
