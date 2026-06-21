# Entity Workbench Architecture

Renderers share an SDK and application layer. The Entity SDK (entitysdk/)
provides reusable infrastructure for any entity-native Go application. The
workbench (workbench/) is one specific application built on the SDK.
Renderers are pure I/O.

**Status**: Architecture validated — SDK extracted, renderers functional.

---

## Project Structure

```
entity-workbench-go/
├── go.work                        workspace linking all modules
├── entitysdk/                     Entity SDK (reusable for any entity app)
│   ├── go.mod                     module: entity-workbench-go/entitysdk
│   │
│   │  Protocol access
│   ├── executor.go                Executor: protocol-level peer access
│   ├── peer_context.go            PeerContext: cached data access
│   ├── resolve.go                 ResolveEntity, DecodeEntityData, ListByPrefix
│   │
│   │  Formatting
│   ├── format.go                  FormatCBOR, FormatValue, FormattedLine, ValueKind
│   ├── output.go                  OutputLine, FlattenFormattedLine, LevelName
│   │
│   │  Tree + layout
│   ├── tree.go                    TreeNode, BuildTree, FlattenVisible
│   ├── layout.go                  LayoutNode[W] generic split tree, Navigate
│   │
│   │  Infrastructure
│   ├── workspace_state.go         WorkspaceState: entity-backed state persistence
│   ├── event_log.go               EventLog: thread-safe ring buffer
│   ├── handlers.go                DiscoverHandlers from system/handler/* entries
│   │
│   └── *_test.go                  tests
│
├── workbench/                     application layer (no UI dependencies)
│   ├── go.mod                     module: entity-workbench-go/workbench
│   │
│   │  Application glue
│   ├── compat.go                  type aliases re-exporting entitysdk for renderers
│   ├── context.go                 DataContext interface (no store/index exposure)
│   │
│   │  Content models (business logic for each panel type)
│   ├── shell_model.go             ShellModel: REPL command parsing + execution
│   ├── handler_model.go           HandlerBrowserModel: handler discovery + execution
│   ├── log_model.go               LogFilterModel: level filtering + auto-persistence
│   ├── detail_model.go            DetailModel: entity resolution + raw/rendered
│   ├── peer_info_model.go         PeerInfoModel: peer statistics
│   ├── tree_model.go              TreeBrowserModel: tree state + search/filter
│   │
│   │  Application state
│   ├── startup.go                 ScreenConfig, DefaultScreens()
│   ├── commands.go                ContentTypes registry, Command palette, Actions
│   ├── selection.go               SelectionState + navigation history
│   │
│   └── *_test.go                  tests
│
├── console/                       TUI renderer (tview + tcell, no CGo)
│   ├── main.go                    peer setup, shared startup, event loop
│   ├── application.go             peer management, content factory, state sync
│   ├── workspace.go               screens, layout, focus, input modes, actions
│   ├── layout.go                  type alias for wb.LayoutNode, buildFlex
│   ├── tree_view.go               tview.TreeView renderer
│   ├── detail_view.go             tview.TextView entity detail renderer
│   ├── entity_shell.go            tview shell REPL renderer
│   ├── execute_console.go         tview handler browser renderer
│   ├── log_viewer.go              tview log viewer renderer
│   ├── peer_info.go               tview peer info renderer
│   ├── empty_view.go              placeholder panel
│   ├── palette.go                 command palette overlay
│   ├── tview_format.go            FormattedLine → tview color tags
│   └── tview_demo.go              widget exploration (non-production)
│
├── canvas/                        graphical renderer (raylib + GLFW, CGo)
│   ├── main.go                    peer setup, shared startup, render loop
│   ├── workspace.go               screens, layout, focus, input modes, actions
│   ├── window.go                  layoutNode (with pixel bounds), split tree
│   ├── tree_view.go               raylib tree browser renderer
│   ├── detail_view.go             raylib entity detail + hash links
│   ├── entity_shell.go            raylib shell REPL renderer
│   ├── execute_console.go         raylib handler browser renderer
│   ├── log_viewer.go              raylib log viewer renderer
│   ├── peer_info.go               raylib peer info renderer
│   ├── empty_content.go           content type picker (reads wb.ContentTypes)
│   ├── commands.go                command palette overlay
│   ├── panel.go                   render texture per window
│   ├── type_renderer.go           per-type renderer registry
│   ├── font.go                    SDF/bitmap font rendering
│   ├── hidpi.go                   Wayland HiDPI workaround (GLFW direct)
│   ├── colors.go                  color definitions
│   ├── view_context.go            viewContext interface
│   └── wake.go                    GLFW event post for render wake
│
├── docs/architecture/
└── Makefile                       build targets for all modules
```

## Module Dependency Flow

```
entity-core-go/core              protocol library (14 packages)
       ↑                         no UI, no workbench knowledge
       │
entity-workbench-go/entitysdk   Entity SDK (reusable infrastructure)
       ↑                         no UI, no application knowledge
       │
entity-workbench-go/workbench    application layer + content models
       ↑                         no UI framework dependencies
      ╱ ╲
console/  canvas/                pure I/O renderers
(tview)   (raylib)               translate input → model, draw model → screen
```

---

## The Application Layer (workbench/)

### Content Models

Each panel type has a model that owns its business logic. Renderers
call model methods and draw model state — no business logic in
renderers.

| Model | What It Owns |
|-------|-------------|
| `ShellModel` | Command parsing (ls/get/exec/count/clear/help), history, output lines |
| `HandlerBrowserModel` | Handler discovery, fingerprinting, selection, operation execution |
| `LogFilterModel` | Display level filtering, level cycling, auto-persistence via BindState() |
| `DetailModel` | Entity resolution, raw/rendered toggle, hash navigation |
| `PeerInfoModel` | Peer statistics (entity count, path count, path list) |
| `TreeBrowserModel` | Tree structure, search/filter (path + `type:` prefix), selection sync |

**Pattern**: each model has State + Methods → Output. The model
produces renderer-neutral data. The renderer translates toolkit
input into model method calls and draws the model's output.

### Entity-Backed State (WorkspaceState)

All application state lives in the entity tree, not in Go structs.
Both renderers write to the same paths via `WorkspaceState`.

| Path | Content |
|------|---------|
| `workspace/window/{id}/content-type` | What a window shows |
| `workspace/window/{id}/screen` | Which screen a window belongs to |
| `workspace/window/{id}/{key}` | Per-window settings (e.g. log-display-level) |
| `workspace/screen/active` | Active screen index |
| `workspace/selection/current` | Selected entity path + has_entry flag |
| `workspace/settings/{key}` | Global settings (e.g. log-collection-level) |

### Layout Tree

`LayoutNode[W comparable]` — generic binary split tree using Go generics.

- `Split`, `Close`, `AllWindows` — tree mutation operations
- `ComputeRects`, `Navigate` — spatial navigation (find nearest window in direction)
- `SplitDir` (SplitH, SplitV), `NavDir` (NavLeft/Right/Up/Down)

Console uses a type alias: `type layoutNode = wb.LayoutNode[*consoleWindow]`.
Canvas keeps its own struct (adds ratio, pixel bounds, drag state) but uses
shared `SplitDir`/`NavDir` and converts to generic tree for navigation.

### Shared Startup

`DefaultScreens()` returns `[]*ScreenConfig` — a declarative tree of
splits and content types. Both renderers translate this to their own
layout tree type via `buildScreenFromConfig()`. Same screens, same
content, same entity-tree state.

### Content Type Registry

`ContentTypes` is the single source of truth for available panel types.
The command palette, canvas picker, and startup configs all derive from it.

```go
var ContentTypes = []ContentType{
    {"tree-browser", "Browse entity tree with search and navigation"},
    {"entity-detail", "Inspect entity data with CBOR rendering"},
    {"entity-shell", "Interactive REPL for entity operations"},
    {"execute-console", "Handler discovery and execution"},
    {"log-viewer", "Real-time event log with level filtering"},
    {"peer-info", "Peer status and entity listing"},
}
```

---

## How to Add a New Panel Type

Adding a content type (e.g. "connection-manager"):

### 1. Create the model (workbench/)

```
workbench/connection_model.go       — ConnectionModel + methods
workbench/connection_model_test.go  — tests
```

Follow the pattern: struct with state, methods that produce
renderer-neutral output, PeerCtx()/DispatchFn() accessors for clone().

### 2. Register the content type (ONE place)

In `workbench/commands.go`, add to `ContentTypes`:

```go
{"connection-manager", "Manage peer connections"},
```

This automatically creates the command palette entry, the canvas
picker entry, and makes the name available for startup configs.

### 3. Add factory case in each renderer

**canvas/workspace.go** `setWindowContent`:
```go
case "connection-manager":
    w.content = newConnectionManager(ws.peerCtx)
```

**console/application.go** `createWindowContent`:
```go
case "connection-manager":
    return newConnectionManager(app.ws, win.peerCtx)
```

### 4. Write the renderer files (thin I/O)

```
canvas/connection_manager.go   — implements windowContent (draw + handleInput)
console/connection_manager.go  — implements windowContent (widget + refresh)
```

Each file translates toolkit input → model method calls and draws
model state. No business logic.

### 5. (Optional) Add to default screens

In `workbench/startup.go` `DefaultScreens()`:

```go
Leaf("connection-manager"),
```

---

## Console (tview)

**Binary**: ~7 MB, pure Go, no CGo, runs in any terminal, works over SSH.

- `tview.Application` manages the event loop
- `tview.Pages` for main content + command palette overlay
- `tview.Flex` tree built from `wb.LayoutNode` via `buildFlex()`
- Input modes: Normal (arrow nav) and Active (keyboard to content)
- Thread-safe updates via `app.QueueUpdateDraw()`

| Key | Action |
|-----|--------|
| Arrow keys | Navigate between windows |
| Enter | Activate focused window |
| Escape | Deactivate window |
| Tab | Cycle focus |
| Ctrl+P | Command palette |
| Ctrl+E | Reset to content picker |
| \\ | Horizontal split |
| - | Vertical split |
| x | Close window |
| 1-9 | Switch screen |

## Canvas (raylib)

**Binary**: ~9.4 MB, CGo required, builds in podman container.

- Immediate-mode rendering via raylib draw calls
- `layoutNode` split tree with pixel bounds + divider dragging
- `panel` — off-screen render textures per window
- Input modes: Normal, Active (keyboard capture), Edit (window management)
- Event-driven: `EnableEventWaiting` + CGo GLFW wake from goroutines

| Key | Action |
|-----|--------|
| s | Activate search in tree |
| e | Toggle edit mode |
| Enter | Activate / command palette |
| Escape | Deactivate |
| \\ | Horizontal split (edit mode) |
| - | Vertical split (edit mode) |
| x | Close window (edit mode) |
| 1-9 | Switch screen |

### HiDPI / Wayland

Raylib 5.5 has bugs with Wayland fractional scaling. `hidpi.go`
queries GLFW directly for framebuffer size, window size, and cursor
position each frame, then corrects the GL viewport and projection
matrix.

---

## Build System

```
make workbench-test     # test shared core (no deps, fast)
make console-build      # build TUI (no CGo, fast)
make console-run        # build + run TUI
make canvas-build       # build in podman (CGo, first build slow)
make canvas-run         # build + run canvas
make test               # all tests
```

Canvas builds in a podman container (Fedora 43 + devel packages).
Go module and build caches are mounted for fast incremental builds.
