# Entity Application Framework: The Missing Standard Library

An exploration of what sits between entity-core-go (the kernel) and
applications like the workbench. We keep reaching into kernel internals
to build application-level things. There's a missing layer.

**Status**: Exploration — figuring out what we're building

---

## 0. Amendment: What Building Two Renderers Revealed

The original document identified three layers: kernel → framework →
application. That was correct but incomplete. Building the canvas
renderer (raylib) alongside the console renderer (tview) made the
internal structure of "application" visible.

### 0.1 The Problem We Found

After migrating the canvas to the workbench API, both renderers
compiled clean with content panels importing only workbench. The
architecture validated — at the framework layer. But then we started
adding screens, content types, and interaction modes to the canvas
and discovered we were rebuilding the console line by line:

- Same screen management (9 screens, switchScreen, active screen)
- Same layout tree (split, close, allWindows, navigate)
- Same input modes (NORMAL/ACTIVE/EDIT state machine)
- Same content factory (name → content type mapping)
- Same shell commands (ls, get, exec, count, clear — identical logic)
- Same handler discovery (fingerprinting, selection, spec display)
- Same log filtering (level cycling, entry filtering)
- Same state persistence patterns (TreePut at workspace/* paths)
- Same startup sequences (action-based screen configuration)

The only thing that differed was how each line of output was drawn:
`fmt.Fprintf(view, "[skyblue]%s[-]")` vs `f.draw(text, x, y, 12, colorPath)`.

This means the console and canvas aren't two applications. They're
two **renderers** for the same application. The application logic —
windows, screens, content models, shell commands, state persistence —
is the application. The renderers translate between that application
state and a UI toolkit.

### 0.2 The Five-Layer Model

What we originally called "application" actually contains three
distinct layers:

```
┌─────────────────────────────────────────────────────┐
│  5. Renderers                                       │
│     console (tview) │ canvas (raylib) │ future...    │
│     Pure I/O: draw state, translate input → actions  │
├─────────────────────────────────────────────────────┤
│  4. Application: Entity Workbench                    │
│     Content models: shell commands, handler browser, │
│     log filter, detail model, state persistence.     │
│     "The specific thing we're building."            │
├─────────────────────────────────────────────────────┤
│  3. Windowing Framework                              │
│     Screens, windows, layout trees, modes, focus,    │
│     commands/actions, split/close/navigate.           │
│     "Could build any panel app with this."           │
├─────────────────────────────────────────────────────┤
│  2. Entity Developer Framework (stdlib)              │
│     Executor, PeerContext, Resolve, Format,          │
│     handler discovery, event log.                    │
│     "Standard library for entity applications."     │
├─────────────────────────────────────────────────────┤
│  1. Entity Core (kernel)                             │
│     Peer, Store, Index, Handlers, Protocol.          │
│     "The entity system itself."                     │
└─────────────────────────────────────────────────────┘
```

Layers 1 and 2 are what the original document describes — kernel and
framework. They're clean and validated. The workbench API migration
proved they work for multiple renderers.

Layers 3, 4, and 5 were previously collapsed into "application."
Building two renderers forced the separation:

**Layer 3 — Windowing Framework**: Screens, windows, layout trees,
input modes, command/action system, focus navigation. This is a
general-purpose panel framework. You could build any windowed
application on it — it has no knowledge of entities, handlers, or
the entity protocol. It's similar in concept to a tiling window
manager or a framework like Decker.

**Layer 4 — Application**: The entity workbench specifically. Shell
commands (ls, get, exec), handler browser (discovery, fingerprinting),
log filter (level cycling), detail model (resolve, raw/rendered
toggle), state persistence (save/read workspace settings). This is
the business logic — it uses the windowing framework for structure
and the entity framework for data access.

**Layer 5 — Renderers**: Console (tview) and canvas (raylib) are
thin adapters. They receive the application's state model and draw
it. They translate user input (terminal events, mouse clicks) into
windowing actions and application operations.

### 0.3 Two Frameworks, Not One

The original document describes one missing layer — the entity
developer framework. We now see there are two:

**Entity Developer Framework** (Layer 2): Standard library for
building applications that use the entity system. Protocol-level
access (Executor, PeerContext), entity resolution, CBOR formatting,
handler discovery. Any entity application uses this.

**Windowing Framework** (Layer 3): Standard library for building
paneled, multi-screen applications with layout trees and command
palettes. This is independent of the entity system — you could use
it for an application that has nothing to do with entities.

These are separate concerns with separate consumers:
- A headless entity application uses Layer 2 but not Layer 3.
- A non-entity panel application uses Layer 3 but not Layer 2.
- The entity workbench uses both.

### 0.4 The Content Model Split

Each content type (tree browser, entity shell, execute console, etc.)
has both application logic and rendering code. These should separate:

**Application model** (Layer 4): The shell command parser, the handler
discovery and fingerprinting, the log level state machine, the entity
resolution and format selection. These produce renderer-neutral
output — lists of lines with text and kind tags, lists of handlers
with operations, filtered log entries.

**Renderer view** (Layer 5): How to draw those lines on screen. For
the shell: tview writes `[skyblue]>[-] %s` vs raylib draws
`f.draw("> "+text, x, y, 12, colorPath)`. For the handler list:
tview builds `tview.List` items vs raylib draws rows with selection
highlights.

The console's `writeFormattedLine` and the canvas's
`drawFormattedLine` do the same thing differently — map a
`wb.FormattedLine` to their UI framework. The model that produces
the `FormattedLine` should exist once.

### 0.5 Where Things Currently Live vs Should Live

| Concept | Currently In | Should Be In |
|---------|-------------|-------------|
| Screen management | console/workspace + canvas/workspace | Layer 3 (windowing) |
| Layout tree operations | console/layout + canvas/window | Layer 3 (windowing) |
| Input mode state machine | console/workspace + canvas/workspace | Layer 3 (windowing) |
| Command/action system | workbench/commands | Layer 3 — already correct |
| Selection + history | workbench/selection | Layer 3 — already correct |
| Shell command parsing | console/entity_shell + canvas/entity_shell | Layer 4 (application) |
| Handler fingerprinting | console/execute_console + canvas/execute_console | Layer 4 (application) |
| Log level cycling | console/log_viewer + canvas/log_viewer | Layer 4 (application) |
| State persistence | console/application | Layer 4 (application) |
| Content factory | console/application + canvas/workspace | Layer 4 (application) |
| Entity resolution/format | workbench/ | Layer 2 — already correct |
| Executor, PeerContext | workbench/ | Layer 2 — already correct |
| tview draw calls | console/ | Layer 5 (renderer) |
| raylib draw calls | canvas/ | Layer 5 (renderer) |

### 0.6 What This Changes About the Original Document

**Still correct**:
- Protocol-first principle (Section 2) — validated by canvas migration
- Standard application state types (Section 3.2) — path conventions
  are shared, SaveSetting/ReadSetting should be in the framework
- Universal entity operations (Section 3.3) — still needed
- Type-aware formatting (Section 3.4) — still the next capability win
- Handler interaction (Section 3.5) — still needed
- Bootstrap path (Section 6) — still the right direction
- AppPeer design (Section 9) — still the right target

**Needs revision**:
- The three-layer diagram (Section 1) → becomes five layers
- "What we have vs need" (Section 5) → needs windowing framework row
- "What to build first" (Section 9) → windowing extraction is now a
  practical priority, not just a future idea

**New insight not in original**:
- The windowing framework as a distinct, entity-independent layer
- Content types have model/view separation
- The application layer owns business logic, not the renderers
- State persistence mechanism (SaveSetting/ReadSetting) belongs in
  the entity framework; orchestration (when/what to save) belongs
  in the application

### 0.7 The Bootstrap Path (Revised)

```
DONE: Workbench API migration
      Canvas builds clean on workbench. Content panels import
      only workbench + raylib. Framework layer (2) validated.
      ↓
NOW:  Understand the layer boundaries
      Map what's duplicated vs renderer-specific.
      This document.
      ↓
NEXT: Extract the windowing framework (Layer 3)
      Screens, layout trees, modes, commands → shared code.
      Both renderers consume the same window model.
      ↓
      Extract application models (Layer 4)
      Shell, handler browser, log filter, detail model →
      shared code that produces renderer-neutral output.
      ↓
      Renderers become thin (Layer 5)
      Console = "draw this model with tview"
      Canvas = "draw this model with raylib"
      ↓
      Add entity-backed state (via framework)
      SaveSetting/ReadSetting in workbench.
      Application decides when/what to save.
      ↓
      Add AppPeer lifecycle (original Section 9)
      Protocol-first state management.
      ↓
      Type-aware formatting
      Read system/type/*, structural display.
      ↓
      Connection management
      Browse real Go/Rust/Python peers.
```

### 0.8 What We're Not Changing

This amendment doesn't invalidate the protocol-first architecture,
the handler integration model, or the convergence spectrum. Those
are about how the application relates to the entity system (Layers
1-2). This amendment is about how the application relates to its
UI (Layers 3-5).

Both dimensions are real and orthogonal:
- **Vertical**: How deep into the entity protocol? (convergence spectrum)
- **Horizontal**: How separated from the renderer? (model/view split)

The workbench is currently at Level 1 (cached client) vertically
and partially separated horizontally. The path forward advances
both: Level 2 (handler registration) vertically, and full
model/view separation horizontally.

### 0.9 Not Five Layers — A Dependency Graph

The five-layer stack is useful but misleading if read as strictly
hierarchical. The actual relationships are:

```
                    ┌──────────┐
                    │ Renderers│  (tview, raylib, HTML, screen reader)
                    └────┬─────┘
                         │ draws/translates
              ┌──────────┴──────────┐
              │                     │
        ┌─────▼──────┐    ┌────────▼────────┐
        │   Panel     │    │  Application    │
        │  Framework  │◄───│  (workbench)    │
        │             │    │                 │
        └─────┬───────┘    └───────┬─────────┘
              │                    │
              │         ┌──────────▼──────────┐
              │         │  Entity Developer   │
              │         │    Framework        │
              │         └──────────┬──────────┘
              │                    │
              │         ┌──────────▼──────────┐
              └────────►│    Entity Core      │
                        └─────────────────────┘
```

The panel framework and the application are siblings, not stacked.
The application uses the panel framework for structure and the entity
framework for data. The panel framework is entity-independent — it
could work without the entity system. The entity framework is panel-
independent — a headless application uses it without panels.

The renderer sits above both — it draws the application's content
models using the panel framework's structure into a specific medium.

This means:
- Panel framework depends on: nothing (or entity core for types only)
- Entity framework depends on: entity core
- Application depends on: both panel framework and entity framework
- Renderers depend on: application + panel framework + medium toolkit

### 0.10 The Fractal Panel Structure

The same layout-of-content pattern repeats at two scales:

**Outer**: The panel framework arranges panels in the viewport.
A split tree, a flex flow, floating windows, tabs — whatever the
renderer's medium supports. Each panel is a rectangular region
with a content type.

**Inner**: Each panel arranges content within itself. The entity
shell has an output area and an input line. The execute console
has a handler list, operation selector, and output. The detail
view has a header and a body that's either raw or rendered.

The inner layout is content-model-specific. The shell model says
"I have output lines and an input buffer." The renderer decides
if that's a tview Flex with TextView + InputField, or a raylib
panel with a divider line and a cursor. The model doesn't know.

This is the same abstraction at different scales:
- Viewport → panels (outer layout, renderer-managed)
- Panel → content regions (inner layout, content-model-defined)
- Content region → formatted elements (manifestation, model-defined)

Composite panels — a panel containing sub-panels — are just the
outer pattern nested. The execute console could be a composite
of a handler browser sub-panel and an output sub-panel. This is
what the Godot project was exploring with its scene tree approach.

The framework should support this nesting without special cases.
A panel's content model can declare sub-regions, and the renderer
decides how to manifest them in its medium.

### 0.11 Content Model Contracts

Each content type has a model that produces renderer-neutral output.
Here's what they look like for what we've built:

**Entity Shell Model**
```
State:   input string, output lines, command history, history index
Input:   submit(text), history-prev, history-next, clear
Output:  []OutputLine{text, kind}  (kind: prompt, result, error, info)
Needs:   PeerContext (for ls, get, count), DispatchFunc (for exec)
```

**Entity Detail Model**
```
State:   selected path, raw/rendered toggle
Input:   select(path), toggle-raw, scroll, navigate-back/forward
Output:  EntityHeader + (FormattedLines | HexDump+DiagnosticLines)
Needs:   PeerContext (for Resolve)
```

**Handler Browser Model** (execute console)
```
State:   handlers[], selected handler, selected operation
Input:   select-handler(idx), select-op(idx), execute
Output:  []HandlerInfo with specs, execution results as OutputLines
Needs:   PeerContext (for DiscoverHandlers), DispatchFunc
Refresh: fingerprint system/handler/* for changes
```

**Log Filter Model**
```
State:   display level, auto-scroll flag
Input:   cycle-level, scroll
Output:  filtered []LogEntry (timestamp + message + level)
Needs:   EventLog reference
```

**Tree Browser Model**
```
State:   tree root, expanded set, search text, selected index
Input:   expand/collapse, select, search, navigate up/down
Output:  []VisibleRow{node, depth}  (already in workbench)
Needs:   entry list (from PeerContext), SelectionState (shared)
Refresh: rebuild tree on entry list change
```

**Peer Info Model**
```
State:   (none — derived from PeerContext)
Input:   scroll
Output:  entity count, path count, path list
Needs:   PeerContext
```

The pattern: State + Input + Output + Needs. The state is owned
by the model. Input comes from the renderer translating user
actions. Output is renderer-neutral data the renderer manifests.
Needs are the data bindings.

The renderer's job for each model: translate medium-specific input
(keystrokes, mouse clicks) into model Input, call the model, take
the model's Output, and manifest it in the medium (tview markup,
raylib draw calls, HTML elements).

### 0.12 Panel Bindings and Connected Interest

The tree browser and entity detail are connected: selecting in the
tree updates the detail. Today this works through shared
SelectionState — both panels read/write the same Go struct.

In the entity-backed model, this becomes tree subscriptions:
- Tree browser writes `workspace/selection/current` when selection
  changes
- Detail view subscribes to `workspace/selection/current` and
  re-renders when it changes
- Any other panel interested in selection does the same

This is the UI Landscape's Section 25.3 insight: "emit is the only
coordination you need." Panels don't talk to each other. They share
state through the tree. The selection path IS the coordination point.

**Multiple bindings are possible.** You could have two tree browsers
with independent selections, each with its own detail view. The
binding is which selection path they share:
- Browser A writes `workspace/selection/a`
- Detail A subscribes to `workspace/selection/a`
- Browser B writes `workspace/selection/b`
- Detail B subscribes to `workspace/selection/b`

The framework manages the wiring. The application declares the
binding. The model just says "I need a selection source" — it
doesn't care which path.

### 0.13 Local Subscriptions vs Cross-Peer

For the local app peer, the full inbox/continuation delivery
pipeline is unnecessary overhead. The subscription engine matches
patterns and delivers — but for local delivery, you can tap
directly into the peer's notification mechanism:

```
Local:   locIndex.Set → NotifyingLocationIndex → fan-out → callback
Remote:  locIndex.Set → subscription engine → inbox → EXECUTE over wire
```

The framework should abstract this. From the application's
perspective, it's the same call:

```go
framework.Subscribe("workspace/selection/*", func(path, event) {
    // panel refreshes
})
```

Internally, for a local peer: direct fan-out subscription.
For a remote peer: subscription engine + inbox + delivery chain.
Same API, different wiring. The application doesn't know or care.

This is the same pattern inside entity-core-go — the subscription
infrastructure already handles both local and remote delivery.
The framework just exposes it as a single function call in the
language's native concurrency model (Go channel, Rust mpsc,
Python asyncio queue).

### 0.15 Research Validation

The academic team published an "architecture-implementation" note
set at
`entity-core-papers/papers/shared/notes/architecture-implementation/`.
Two of the seven documents directly address this layer model:

- **`exploration-sdk-and-ergonomics.md`** independently arrives at
  the same layering and explicitly calls the workbench's
  `entitysdk/` + `workbench/` + renderers separation exemplary.
  It recommends the Rust SDK converge on this structure (Rust
  currently mixes business logic into view code, the way our
  console did before the model extraction).

- **`ui-projects-convergence-review.md`** names console + canvas
  sharing `workbench/` models as "the projection pattern in
  practice" — the clearest existing example of the multi-projection
  conclusion from the UI landscape work.

The five-layer model in §0.2 is unchanged by this validation. The
research adds three forward-looking workstreams (Phases F, G, H in
`WORKBENCH-STATUS-AND-NEXT.md`) but does not revise the layer
structure itself. Specifically the research adds the distinction
between **Layer 1 (per-language facade, variation allowed)** and
**Layer 2 (shared algorithms, must match)** within what we call
the entity developer framework — see `ENTITY-SDK-API.md` "Layer 2
Algorithm Contract."

### 0.14 Converging with the UI Landscape

The UI Landscape (entity-core-go) mapped the theoretical space.
Building two renderers for the same application showed us where
we're standing in it:

| UI Landscape Concept | What We Built | What We Learned |
|---------------------|---------------|-----------------|
| Projection = Selection + Ordering + Manifestation | Each panel is a projection | The model (selection + manifestation) is shared; ordering is renderer-specific |
| Layout is medium-specific | Split trees + screens (tiled 2D) | Screens aren't fundamental — they're one medium's grouping strategy |
| The five UI primitives (Scope, Arrangement, Form, Mode, Exchange) | Panel types + modes + actions | These map to our content model contracts (State + Input + Output) |
| Panels don't talk to each other — they share tree state | SelectionState, entity-backed state | Confirmed. The tree IS the coordination bus |
| The codebook travels with content | Entity types at system/type/* | Type awareness is the next unlock |
| Multiple projections composed | Tree + detail + shell | Connected through shared selection, not direct coupling |

The theory was right. Building it showed us how to apply it:
the content model IS the projection (scope + manifestation), the
renderer IS the ordering, and the entity tree IS the coordination
substrate.

What the theory didn't make clear (because we hadn't built it yet):
the content model needs an explicit contract — State, Input, Output,
Needs — so that any renderer can implement it. And the panel
framework is a sibling to the application, not a layer above or
below it.

---

```
┌─────────────────────────────────────────────────────┐
│  Applications                                        │
│  Workbench, text editor, game, e-commerce, whatever  │
│  Built on the framework. Bespoke UI + domain logic.  │
└──────────────────────┬──────────────────────────────┘
                       │ imports
┌──────────────────────▼──────────────────────────────┐
│  Entity Application Framework  (THE MISSING LAYER)   │
│  Standard library for building entity-system apps.   │
│  App peer lifecycle, state management, type-aware    │
│  operations, universal entity operations, UI state   │
│  conventions. Language-specific (Go, Rust, Python).  │
└──────────────────────┬──────────────────────────────┘
                       │ imports
┌──────────────────────▼──────────────────────────────┐
│  Entity Core  (THE KERNEL)                           │
│  Peer, Store, LocationIndex, Handler, Protocol,      │
│  CBOR/ECF, Hash, Crypto, Types.                      │
│  Raw system-level primitives.                        │
└─────────────────────────────────────────────────────┘
```

entity-core-go is the kernel. It gives you peers, stores, handlers,
the wire protocol. It's correct and complete for what it does. But
building an application directly on it is like building a web app
directly on syscalls.

The workbench module is becoming the standard library by accident.
We keep adding functions to it — ResolveEntity, FormatCBOR, DiscoverHandlers,
Executor — because the console and canvas both need them. But it's
designed as "shared code for two UIs," not as "the Go standard library
for entity-system applications."

The difference matters. A standard library has:
- Opinions about how you structure an application
- Conventions that all applications follow
- Patterns that make common things easy
- An API surface designed for external consumers, not just two siblings

---

## 2. The Protocol Boundary Principle

Before defining the API: the most important architectural decision.

### 2.1 The Problem with Direct Access

The code we wrote today for AppPeer state management looks like this:

```go
data, _ := ecf.Encode(map[string]interface{}{"path": selectedPath, "has_entry": true})
ent, _ := entity.NewEntity("app/state/selection", cbor.RawMessage(data))
h, _ := store.Put(ent)
locIndex.Set("workspace/ui/selection", h)
```

This bypasses the protocol. We're reaching directly into the store and
location index. This is kernel-level implementation detail — the kind
of thing entity-core-go does internally, not what an application should
do.

Why it matters:
- **Handlers don't fire.** A tree put handler might need to process
  the write. Direct `locIndex.Set` skips it.
- **Emit channels might not fire.** NotifyingLocationIndex fires, but
  only if we're using the right wrapped index.
- **Capability grants are bypassed.** No authorization check.
- **It's implementation-specific.** This code only works with Go peers.
  Can't use it to talk to a Rust or Python peer.

### 2.2 Protocol-First Standard Library

The standard library should interact with peers through the protocol,
even the internal app peer. "Protocol" doesn't mean TCP — it means
using the execute/tree operations that handlers process:

```go
// Standard library uses protocol operations, not direct store access:
appPeer.Execute("workspace/ui", "put", SelectionState{Path: selectedPath})
// This goes through: handler resolution → capability check → handler logic
//                     → store.Put → locIndex.Set → emit → tree events
```

The developer never sees stores, indexes, or hashes. They see paths
and operations. The standard library translates between "I want to
save my selection" and the protocol operations that accomplish it.

### 2.3 Why This Changes the Design

If the standard library uses the protocol:

1. **Same code path for local and remote peers.** The app peer is just
   another peer. Connecting to an external Go/Rust/Python peer uses the
   same API.

2. **Handlers always fire.** Writes go through tree put handlers.
   Indexes update. Emit channels fire. Subscriptions trigger.
   Everything works the way the system designed it to work.

3. **Capability grants are enforced.** Even on the app peer. This means
   the security model is always active, which matters when app state
   syncs to other machines.

4. **Multiple peers are natural.** An application might use several
   peers for different concerns — one for app state, one for user data,
   connections to remote peers. The standard library treats them all
   the same because the protocol is the same.

5. **Memory isolation.** Each peer is its own concurrent process with
   its own emit chain. No explosion of cross-cutting subscriptions.
   You monitor activity per-peer.

### 2.4 When to Drop Below Protocol

An implementer CAN reach into internals — it's Go, the types are
there. But it's a conscious choice with tradeoffs:

- **Protocol level**: security enforced, handlers fire, emits fire,
  indexes updated, works with any peer. This is the standard library.
- **Kernel level**: faster, more control, but you own the consequences.
  Bypassed handlers, missed emits, possible security gaps. This is for
  entity-core-go contributors, not application developers.

The standard library should make protocol-level access so convenient
that dropping to kernel level is rarely needed. Performance optimization
should be data-driven, not assumed — especially for the Go workbench
which is management-oriented, not performance-critical.

### 2.5 Implications for Standard Library Design

The standard library API wraps protocol operations, not store/index
operations:

```
// NOT this (kernel level):
store.Put(entity)
locIndex.Set(path, hash)

// THIS (protocol level):
peer.Execute("system/tree", "put", TreePutRequest{Path: path, Entity: entity})

// OR BETTER (standard library level):
appPeer.SetState("workspace/ui/selection", selectionData)
// which internally does the execute
```

This means the standard library depends on the peer having appropriate
handlers registered (tree handler at minimum). The app peer ships with
these. External peers are expected to have them (they're part of core
protocol).

---

## 3. What the Standard Library Provides

Working from what we keep building and rebuilding, the standard library
handles these concerns:

### 3.1 App Peer Lifecycle

Every entity-system application needs an internal peer for its own
state. The standard library provides this:

```
AppPeer
├── Create(opts) → peer with standard handlers + app state types
├── State(path) → read state via tree get
├── SetState(path, type, data) → write state via tree put
├── OnChange(callback) → subscribe via tree events
├── Connect(addr) → connect to external peer
├── Execute(path, op, params) → dispatch via protocol
└── Close() → persist and shut down
```

The app peer is:
- Created on startup with a deterministic or stored keypair
- Pre-seeded with type definitions for standard application state types
- Not exposed as a network listener by default (debug flag to expose)
- The place where ALL application state lives — no Go struct state
  that isn't backed by an entity
- Accessed through the protocol, not through direct store/index

This is what we keep trying to build in `application.go` but as
one-off code.

### 3.2 Standard Application State Types

The standard library defines entity types for common application state.
These type definitions ship with the library and are seeded into the
app peer on creation:

```
app/state/selection     → {path: "...", has_entry: bool}
app/state/layout        → {direction: "h"|"v", ratio: 0.5, children: [...]}
app/state/window        → {content_type: "tree-browser", bindings: {...}}
app/state/connection    → {addr: "...", auto_connect: bool, status: "..."}
app/state/setting       → {key: "...", value: ...}
app/state/history       → {entries: [...], position: int}
```

Because these are entity types with definitions at `system/type/app/state/*`,
any application that imports the standard library interprets them the
same way. Console, canvas, future web UI — all read the same state
format.

### 3.3 Universal Entity Operations

Operations that apply to ANY entity at ANY path. These are currently
scattered across our code or not implemented at all:

```
EntityOps
├── Resolve(path) → entity + decoded data + metadata
├── Save(path, type, data) → create entity, put, set index
├── Delete(path) → remove from index
├── History(path) → list revision entries if they exist
├── Capabilities(path) → what grants authorize access here
├── Related(entity) → follow type refs, hash links
├── TypeInfo(typeName) → field specs from type definition
└── Search(prefix, typeFilter) → find entities by path + type
```

These exist partially in workbench today (ResolveEntity, ListByPrefix,
DiscoverHandlers). But they're ad hoc. The standard library makes them
a coherent API.

### 3.4 Type-Aware Formatting

What we built today in `format.go` — but designed as a proper API:

```
TypeFormat
├── FormatEntity(entity, typeInfo) → []FormattedLine with field names
├── FormatValue(value) → FormattedValue with kind tag
├── FormatStructural(entity, typeDefs) → named fields from type definition
├── FormatRaw(entity) → hex dump + hash diagnostic
└── EditableFields(entity, typeInfo) → fields with editor hints
```

The last one is new: given an entity and its type definition, return
a description of what fields exist, their types, whether they're
editable, and what kind of editor to use (text, number, toggle, hash
picker, etc.). This is what enables generic editing without per-type
code.

### 3.5 Handler Interaction

What the execute console does, but as a library:

```
HandlerOps
├── Discover(entries) → []HandlerInfo
├── Execute(path, operation, params) → response
├── OperationsFor(handlerInfo) → operation specs with types
└── MatchHandler(path) → which handler serves this path
```

Again, partially built. `DiscoverHandlers()` and `Executor` exist.
But `MatchHandler` (given a path, find the handler whose pattern matches)
is what would enable contextual comprehension: "I'm looking at
`data/files/readme.md` — is there a handler for `data/files/*`?"

### 3.6 Connection Management

Managing connections to external peers:

```
PeerManager
├── Connect(addr) → connection with tree access
├── Disconnect(addr)
├── ListConnections() → active peers
├── TreeFor(peerID) → PeerContext for browsing remote tree
└── OnTreeChange(peerID, callback) → subscribe to remote changes
```

The workbench does this manually in `main.go`. The standard library
makes it a pattern.

---

## 4. What's Language-Specific vs Implementation-Agnostic

Two very different things live in this system:

### Language-specific standard library

Go applications import the Go library. Rust imports the Rust library.
Python imports the Python library. Same patterns, same conventions,
different implementations. These are the APIs in section 2 above.

The Go standard library is what `workbench/` is becoming. A Rust
equivalent would live in the Rust entity system project. They don't
share code — they share conventions and type definitions.

### Implementation-agnostic content

This is what lives in the entity tree and is interpreted by any
language's library:

- **Type definitions** at `system/type/*` — field specs, extends chains
- **Handler manifests** at `system/handler/*` — operations, input/output types
- **Application state** at `workspace/*` or `app/*` — selection, layout, settings
- **UI descriptions** (future) — panel configurations, editor layouts, rendering hints
- **Content** — documents, data, whatever the application manages

This content is the same bytes regardless of which language reads it.
CBOR encoding, content-addressed, syncable via protocol. The type
definitions tell any reader how to interpret the data.

### The interface between them

The standard library reads the implementation-agnostic content and
provides language-native APIs:

```
Entity tree (CBOR bytes)
    → Standard library reads type definitions
    → Provides Go structs / Rust structs / Python dicts
    → Application uses native types
    → Application writes back through standard library
    → Entity tree (CBOR bytes)
```

The standard library is the translator between the entity world (CBOR,
paths, hashes, types) and the application world (native types, function
calls, callbacks).

---

## 5. What We Have vs What We Need

### What workbench/ currently provides (the accidental standard library)

| Concern | What Exists | Status |
|---------|-------------|--------|
| Entity resolution | `ResolveEntity()`, `DecodeEntityData()` | Built, tested |
| CBOR formatting | `FormatCBOR()`, `FormatValue()`, `FormattedLine` | Built, tested |
| Tree navigation | `TreeNode`, `BuildTree`, `FlattenVisible` | Built, tested |
| Selection state | `SelectionState` (Go struct, not entity-backed) | Built, tested |
| Handler discovery | `DiscoverHandlers()` | Built, tested |
| Handler dispatch | `Executor`, `DispatchFunc` | Built |
| Event logging | `EventLog` | Built |
| Data caching | `PeerContext` with dirty flag | Built |
| Commands | `Registry`, `Action`, `FilterCommands` | Built, tested |

### What's missing for a proper standard library

| Concern | What's Needed | Priority |
|---------|--------------|----------|
| App peer lifecycle | Create, seed types, state read/write | High — everything depends on this |
| Type-aware formatting | Read `system/type/*`, show named fields | High — biggest usability improvement |
| Entity-backed state | Selection/layout/connections as entities | High — persistence, transferability |
| Connection management | Connect to external peers, browse trees | High — makes it a real tool |
| Universal operations | Save, delete, history, capabilities | Medium — needed for editing |
| Handler matching | Given a path, find its handler | Medium — enables contextual actions |
| Editable field specs | Type definition → editor hints | Medium — enables generic editing |
| DSL / convenience API | Less cumbersome entity creation | Low — ergonomics, not capability |

### The gap

The gap is mostly in the **first column**: app peer lifecycle and
entity-backed state. Once the workbench stores its own state as entities,
the patterns for reading, writing, and subscribing to entity state
become the standard library API. We're discovering the API by using it.

---

## 6. The DSL Question

Working with the entity system today is cumbersome. The kernel API
requires four steps to write one piece of state. But the answer isn't
just a convenience wrapper — it's the protocol.

**Kernel level** (what we do today — wrong for applications):

```go
data, _ := ecf.Encode(map[string]interface{}{"path": selectedPath})
ent, _ := entity.NewEntity("app/state/selection", cbor.RawMessage(data))
h, _ := store.Put(ent)
locIndex.Set("workspace/ui/selection", h)
```

This bypasses handlers, skips capability checks, may miss emits.

**Protocol level** (correct — goes through the system):

```go
appPeer.Execute("system/tree", "put", TreePutRequest{
    Path: "workspace/ui/selection",
    Entity: selectionEntity,
})
```

One operation. Handlers fire. Emits fire. Grants checked.

**Standard library level** (what we want — hides protocol ceremony):

```go
app.SetState("workspace/ui/selection", SelectionState{
    Path:     selectedPath,
    HasEntry: true,
})
```

Or even:

```go
app.Selection.Set(selectedPath, true)
```

Where `app.Selection` is a typed accessor that knows the path
convention, entity type, and translates to the protocol operation.

The standard library doesn't bypass the protocol — it wraps it. The
ceremony is hidden, but the semantics are preserved: handler dispatch,
capability enforcement, tree events.

### Would an actual DSL help?

Maybe eventually. But it's premature — we don't know enough about the
patterns to design a good DSL. Build the Go standard library API first.
The DSL (if we need one) falls out of the patterns we discover.

### The peer capability dimension

The standard library's capabilities depend on what the peer supports.
A minimal peer with only core protocol can do tree get/put and execute.
A peer with system extensions can do extract, merge, subscriptions,
compute. The standard library needs to handle both:

```go
// Works with any peer (core protocol only):
app.Execute("system/tree", "get", path)
app.Execute("system/tree", "put", path, entity)

// Requires system extensions:
app.Execute("system/tree", "extract", subtree)
app.Execute("system/subscription", "create", spec)
```

The standard library documents which operations require which peer
capabilities. Applications choose their peer configuration based on
what they need.

---

## 6. The Bootstrap Path

How we get from where we are to where we want to be:

```
NOW:  workbench/ is shared code for two UIs
      ↓
      Recognize it as the embryonic standard library
      ↓
NEXT: Add app peer lifecycle + entity-backed state
      ↓
      State reads/writes become the core API pattern
      ↓
      Add type-aware operations (read type defs, structural formatting)
      ↓
      Add connection management (browse real peers)
      ↓
      Standard library API stabilizes from usage
      ↓
THEN: Factor out into proper importable library
      ↓
      Rust/Python teams implement equivalent libraries
      ↓
      Implementation-agnostic content (type defs, state types,
      UI descriptions) is shared across languages
      ↓
GOAL: Any entity-system application imports the standard library,
      gets app peer + state management + type awareness + connections.
      UI framework is a choice (tview, raylib, web, egui).
      Application state and content portable across implementations.
```

Some of the code we write now IS the standard library (ResolveEntity,
FormatCBOR, TreeNode). Some is scaffolding (manual PeerContext wiring,
direct Store/Index access). The scaffolding gets replaced as the
standard library API matures.

---

## 8. Relationship to the Scenarios Document

The scenarios (WORKBENCH-SCENARIOS.md) trace what happens in concrete
use cases. This document identifies the API layer that makes those
scenarios work:

| Scenario | Standard Library API Used |
|----------|-------------------------|
| View doc/paper | TypeFormat.FormatStructural, EntityOps.TypeInfo |
| Edit and save | EntityOps.Save, TypeFormat.EditableFields |
| Execute handler | HandlerOps.Execute, HandlerOps.OperationsFor |
| Persist state | AppPeer.SetState, AppPeer.OnStateChange |
| Browse remote peer | PeerManager.Connect, PeerManager.TreeFor |
| Two workbenches, same state | AppPeer sync via protocol |

Each scenario that's hard today is hard because the standard library
API doesn't exist yet. We're doing everything at the kernel level.

---

## 9. What to Build First

The minimum standard library that makes the workbench meaningfully better.
All state access goes through the protocol, not direct store/index.

### 9.1 AppPeer (the foundation)

```go
// workbench/app_peer.go

type AppPeer struct {
    peer *peer.Peer      // internal, not exposed to application code
}

// NewAppPeer creates an internal peer for application state.
// The peer has standard handlers registered (tree, connect).
// Application state type definitions are seeded automatically.
func NewAppPeer() (*AppPeer, error)

// State reads application state via protocol (tree get).
func (ap *AppPeer) State(path string) (ResolvedEntity, bool)

// SetState writes application state via protocol (tree put).
// Handlers fire. Emits fire. Grants enforced.
func (ap *AppPeer) SetState(path string, typeName string, data interface{}) error

// Execute dispatches any operation via protocol.
func (ap *AppPeer) Execute(handlerPath, operation string, params interface{}) (*handler.Response, error)

// Connect reaches an external peer via protocol.
func (ap *AppPeer) Connect(addr string) error

// TreeEvents returns change notifications from all connected peers.
func (ap *AppPeer) TreeEvents() <-chan store.TreeChangeEvent

func (ap *AppPeer) Close() error
```

The application never sees `peer.Store()` or `peer.LocationIndex()`.
It uses `State`, `SetState`, and `Execute`. These go through the
protocol. The AppPeer is just another peer the application talks to —
it happens to be local, but the API is the same as for remote peers.

### 9.2 Typed State Accessors

```go
// workbench/app_state.go

type AppState struct {
    app *AppPeer
}

func (s *AppState) Selection() *SelectionState
func (s *AppState) SetSelection(sel *SelectionState)
func (s *AppState) Connections() []ConnectionInfo
func (s *AppState) AddConnection(addr string, autoConnect bool)
```

Typed accessors that call `ap.SetState` with the right path and type.
These ARE the standard library API for application state.

### 9.3 Type-Aware Formatting

```go
// workbench/type_aware.go

func FormatTyped(entity, typeStore) []FormattedLine
// Reads system/type/{typeName} via protocol (tree get)
// Returns named fields from type definition
// Falls back to FormatCBOR when no type definition exists
```

One function. No registry infrastructure. Uses the protocol to read
type definitions. Enhances the existing formatting pipeline.

### 9.4 The Migration

The current workbench uses direct store/index access everywhere.
Migrating to AppPeer is incremental:

1. Create AppPeer — wraps the existing peer creation in main.go
2. Add State/SetState — the first protocol-level accessors
3. Migrate console/application.go to use AppPeer instead of manual
   peer wiring
4. Replace PeerContext (direct store/index wrapper) with protocol-based
   data access
5. Remove direct entity-core-go imports from application.go

Each step preserves functionality. The scaffolding (direct access)
gets replaced with the standard library (protocol access) piece by
piece.

---

## 10. Open Questions

1. **Should the standard library be a separate module?** Currently it's
   `workbench/`. Should it become `entity-app-go/` or similar? Probably
   not yet — let the API stabilize first, then factor out.

2. **How much ceremony does entity-backed state add?** Every state change
   goes through the protocol. Is this too slow for interactive UI?
   Probably not for meaningful state (selection, layout). Definitely too
   slow for ephemeral state (scroll position, hover). The line between
   "entity-backed" and "Go struct" is important to get right.

3. **Does the tree handler support what we need?** The tree handler does
   get, put, list, snapshot, diff, merge, extract. Is that sufficient
   for all standard library operations? Or do we need additional
   handlers for application state management (e.g., a workspace handler
   that understands layout semantics)?

4. **What about testing?** The standard library is testable because it
   wraps the kernel's in-memory implementations. AppPeer in tests uses
   a real peer with in-memory stores. No network, no disk. The protocol
   operations work identically.

5. **When do Rust/Python teams need their versions?** When they build
   applications beyond CLIs. The standard library conventions (state
   path conventions, type definitions for UI state) should be documented
   as cross-language specs, not just Go code.

6. **Multiple peers per application.** The user identified that
   applications may use multiple peers for different concerns — app
   state, user data, external connections. Each peer is its own
   concurrent process with isolated state and emit chains. The standard
   library should make multi-peer applications natural, not special.

7. **Where does the standard library live relative to entity-core-go?**
   Some of this might belong in entity-core-go as part of the peer
   implementation. The standard library contract (API conventions, state
   types) could be standardized cross-language. But the implementation
   depends on the peer implementation. This boundary needs clarity.
