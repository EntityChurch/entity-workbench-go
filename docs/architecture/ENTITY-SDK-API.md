# Entity SDK — Go API Surface

Status: Design exploration

The Entity SDK wraps entity-core-go (the kernel) to provide an
ergonomic interface for building entity-native applications in Go.
This document maps the developer-facing API — what a Go developer
sees, not how it's implemented.

The SDK is not hiding the entity system. Everything it does is
visible in the tree — subscriptions, connections, handlers, state.
The SDK makes common patterns convenient so you don't construct
execute messages for every tree operation.

## SDK vs Workbench

These are different things:

| | Entity SDK | Workbench |
|--|-----------|-----------|
| What | Go library for building entity applications | A specific application built on the SDK |
| Module | `entity-sdk-go` | `entity-workbench-go` |
| Audience | Any Go developer building on the entity system | Users of the workbench tool |
| Contains | Protocol access, subscriptions, connections, formatting, tree ops | Content models, panels, screen configs, command palette |
| Depends on | entity-core-go (kernel) | Entity SDK + tview/raylib |

The workbench is the SDK's first consumer, the same way egui-entity-core-rust
is the first consumer of the Rust SDK. The SDK was discovered by building
the workbench — patterns that proved reusable across both renderers
(console + canvas) are SDK material.

## Design Principles

1. **Go-native idioms.** Channels for event streams. Interfaces for
   extension points. Context for cancellation. Goroutines handle
   concurrency internally — the developer's API is synchronous where
   possible, channel-based where events flow.

2. **Cross-language in spirit.** The Rust, Go, Python, and JS SDKs
   share the same concepts with language-specific surfaces. Go gets
   channels and interfaces where Rust gets async/await and traits.
   The concepts map across: tree operations, subscriptions,
   connections, handler registration.

3. **Protocol-first.** All operations go through the entity protocol.
   The SDK never exposes internal storage or indexes directly.
   This ensures handlers fire, emits propagate, capabilities are
   checked, and the same code works for local and remote peers.

4. **The kernel is always available.** Developers who need
   entity-core-go directly can use it. The SDK is a layer on top,
   not a replacement. Custom engines, store backends, and transport
   implementations need the kernel.

5. **Correctness first, optimize later.** The initial SDK routes
   everything through the protocol. Once the API is stable, hot
   paths (local tree reads, local subscriptions) can be optimized
   internally without changing the surface.

## Core Type

```go
package entitysdk

// App is the primary interface for entity-native application
// development in Go.
//
// It wraps an entity-core-go Peer with ergonomic methods for tree
// operations, subscriptions, connections, and handler management.
// All operations go through the entity protocol — the SDK never
// exposes internal storage or indexes directly.
type App struct {
    // internal: peer, subscription state, connection state
    // none of this is public
}
```

## Lifecycle

```go
// -- Construction --

app, err := entitysdk.New(entitysdk.GenerateKeypair())

// With options:
app, err := entitysdk.New(
    entitysdk.WithKeypair(kp),
    entitysdk.WithConfig(cfg),     // kernel config passthrough
)

// -- Identity --

peerID := app.PeerID()

// -- Shutdown --
// App cleans up on Close: stops engines, closes connections.
app.Close()

// Typical pattern with defer:
app, err := entitysdk.New(entitysdk.GenerateKeypair())
if err != nil {
    log.Fatal(err)
}
defer app.Close()
```

## Tree Operations

The core data operations. These work on local and remote paths
transparently. The SDK routes based on the path:

- `"my/local/path"` → local peer's tree
- `"entity://{remote_pid}/some/path"` → remote peer via connection

```go
// -- Read --

ent, ok := app.TreeGet(ctx, "path/to/entity")

// -- Write --

err := app.TreePut(ctx, "path/to/entity", "doc/paper", data)

// -- List --

entries, err := app.TreeList(ctx, "path/prefix/")

// -- Delete --

err := app.TreeDelete(ctx, "path/to/entity")

// -- Check existence --

exists := app.TreeHas(ctx, "path/to/entity")
```

### TreeEntry

```go
// An entry in a tree listing.
type TreeEntry struct {
    Path string
    Hash crypto.Hash
}
```

### Entity construction

```go
// From structured data (CBOR-encoded automatically):
err := app.TreePut(ctx,
    "workspace/settings/theme",
    "app/setting",
    map[string]interface{}{"value": "dark"},
)

// From raw bytes:
err := app.TreePutRaw(ctx,
    "data/imported/file",
    "data/blob",
    rawBytes,
)
```

## Subscriptions

Subscribe to tree changes. The SDK handles the wiring — whether
that's a local broadcast, a subscription engine chain, or a remote
subscription. The developer gets a Go channel.

```go
// -- Channel style (idiomatic Go) --

events, cancel := app.OnChange("system/type/**")
defer cancel()

for event := range events {
    fmt.Printf("changed: %s (%s)\n", event.Path, event.Type)
}

// -- Non-blocking select --

events, cancel := app.OnChange("workspace/ui/**")
defer cancel()

select {
case event := <-events:
    handleChange(event)
case <-ctx.Done():
    return
}

// -- Multiple patterns --

typeEvents, cancel1 := app.OnChange("system/type/**")
handlerEvents, cancel2 := app.OnChange("system/handler/**")
defer cancel1()
defer cancel2()

// -- Remote subscriptions (same API) --

events, cancel := app.OnChange("entity://{remote_pid}/data/**")
// SDK internally: checks connection, wires subscription chain,
// delivers events through the same channel interface.
```

### ChangeEvent

```go
// A tree change notification.
type ChangeEvent struct {
    Path         string
    Hash         crypto.Hash
    PreviousHash crypto.Hash  // zero if created
    Type         ChangeType   // Created, Modified, Deleted
}

type ChangeType int
const (
    Created ChangeType = iota
    Modified
    Deleted
)
```

### Wake function

For applications that need a "something changed, redraw" signal
rather than per-event processing. Coalesces: 1000 changes between
frames = 1 wake call.

```go
// Register a wake function — called when ANY subscribed path changes.
app.SetWakeFunc(func() {
    // tview:
    tviewApp.QueueUpdateDraw(func() {})

    // raylib/GLFW:
    glfw.PostEmptyEvent()

    // generic:
    wakeCh <- struct{}{}
})
```

The wake function is the render-framework bridge. Set once.
Everything else (subscriptions, remote updates, connection events)
flows through it.

## Connections

```go
// -- Connect to a remote peer --

remotePID, err := app.Connect(ctx, "ws://192.168.1.5:4041")

// -- Listen for incoming connections --

err := app.Listen(ctx, "0.0.0.0:4041")
// Accepts connections in background goroutines.
// New peers appear in the tree and are accessible via entity:// URIs.

// -- List connected peers --

peers := app.ConnectedPeers()

// -- Connection events (channel style) --

events, cancel := app.OnPeerEvent()
defer cancel()

for event := range events {
    switch event.Type {
    case PeerConnected:
        log.Printf("connected: %s at %s", event.PeerID, event.Addr)
    case PeerDisconnected:
        log.Printf("disconnected: %s (%s)", event.PeerID, event.Reason)
    }
}
```

### PeerInfo

```go
type PeerInfo struct {
    PeerID    string
    Addr      string     // empty if not applicable
    Direction Direction  // Inbound or Outbound
}

type Direction int
const (
    Inbound Direction = iota
    Outbound
)
```

## Execute (Power Tool)

Direct handler execution for operations not covered by convenience
methods. This is the "drop down a level" API — you're constructing
protocol operations directly.

```go
resp, err := app.Execute(ctx, "system/tree", "get", params)

// With options:
resp, err := app.ExecuteWith(ctx, "custom/handler", "process", params,
    ExecuteOptions{
        Resource: &ResourceTarget{...},
    },
)
```

## Handler Discovery

Read what handlers are available — local or remote.

```go
// -- Local handlers --

handlers, err := app.DiscoverHandlers(ctx)
for _, h := range handlers {
    fmt.Printf("%s: %s (ops: %v)\n", h.Pattern, h.Name, h.Operations)
}

// -- Remote handlers --

handlers, err := app.DiscoverHandlersOn(ctx, remotePID)
```

### HandlerInfo

```go
type HandlerInfo struct {
    Pattern    string
    Name       string
    Operations []string
}
```

## Handler Registration

Register application handlers on the local peer. This is how your
application becomes a participant in the entity protocol — other
peers can execute operations on your handlers.

```go
// -- Register a handler --

err := app.RegisterHandler(myHandler)

// Where myHandler implements the kernel's handler.Handler interface.
// The SDK doesn't wrap this — handlers are a kernel concept.
// The SDK makes registration convenient.

// -- Closure-based handler (no interface needed) --

err := app.Handle("app/echo", []string{"echo"}, func(ctx HandlerContext) (*Response, error) {
    return &Response{Status: 200, Result: ctx.Params}, nil
})

// -- Unregister --

err := app.UnregisterHandler("app/echo")
```

## Higher-Level Helpers

These are patterns built on the core API. They live in the SDK
because they're useful across applications, not specific to any one.

### PeerContext (Cached Tree View)

Wraps an App with caching and dirty-tracking. Multiple panels or
goroutines can share a PeerContext — they all see the same cached
data and refresh together.

```go
pc := entitysdk.NewPeerContext(app)

// Cached entry list with dirty-tracking:
pc.MarkDirty()
if pc.RefreshIfDirty() {
    entries := pc.Entries()  // sorted, cached
}

// Entity resolution through cache:
resolved, ok := pc.Resolve("path/to/entity")
// resolved.Path, resolved.Hash, resolved.Entity, resolved.Decoded

count := pc.EntityCount()
```

This is the primary data access pattern for UI applications. The
App provides raw tree operations; PeerContext adds the caching
layer that makes frame-rate access efficient.

### AppState (Entity-Backed Persistence)

Typed access to application state stored in the entity tree. All
state lives as entities — no Go structs holding ephemeral state.
Multiple renderers can share the same state.

```go
state := entitysdk.NewAppState(app)

// Read/write settings:
state.SaveSetting("theme", "dark")
theme := state.ReadSetting("theme")

// Scoped state (e.g., per-window settings):
state.SaveScoped("window", windowID, "content-type", "tree-browser")
ct := state.ReadScoped("window", windowID, "content-type")
```

Path conventions (customizable, these are defaults):

| Path | Content |
|------|---------|
| `{prefix}/settings/{key}` | Global settings |
| `{prefix}/{scope}/{id}/{key}` | Scoped settings |

### Tree Building

Construct hierarchical tree structures from flat entry lists.
Used by any application that displays entity paths as a tree.

```go
root := entitysdk.BuildTree(entries)
entitysdk.SortTree(root)
entitysdk.ExpandToDepth(root, 2)

visible := entitysdk.FlattenVisible(root)
for _, row := range visible {
    fmt.Printf("%s%s\n", strings.Repeat("  ", row.Depth), row.Node.Segment)
}
```

### Formatting Pipeline

Renderer-neutral formatting for entity data. Produces structured
output that any UI framework can consume.

```go
// CBOR data → structured lines:
lines := entitysdk.FormatCBOR(decoded)
for _, line := range lines {
    // line.Indent, line.Key, line.Value (with Kind: String, Number, Hash, etc.)
}

// Entity header:
header := entitysdk.HeaderFromResolved(resolved)
// header.Path, header.Type, header.Hash, header.Size

// Plain text fallback:
text := entitysdk.RenderPlainText(lines)
```

### Layout Engine

Generic binary split tree for windowed applications. Go generics
make it type-safe for any window type.

```go
// Build a layout:
layout := entitysdk.SplitNode[*MyWindow](
    entitysdk.SplitH,
    entitysdk.LeafNode(leftWindow),
    entitysdk.LeafNode(rightWindow),
)

// Split a window:
layout.Split(targetWindow, entitysdk.SplitV, newWindow)

// Spatial navigation:
next, ok := entitysdk.Navigate(layout, currentWindow, entitysdk.NavRight)

// Compute pixel/cell rects for rendering:
rects := entitysdk.ComputeRects(layout, 0, 0, width, height)
```

### Event Log

Thread-safe, leveled event logging with generation tracking for
efficient incremental reads.

```go
log := entitysdk.NewEventLog(1000) // ring buffer

log.Append("connected to peer %s", peerID)
log.Verbose("tree refresh: %d entries", count)
log.Debug("raw response: %v", resp)

// Generation-based reads (only get new entries):
gen := log.Gen()
// ... later ...
if log.Gen() != gen {
    entries := log.Entries()
}
```

## What the SDK Does NOT Expose

These are kernel internals. Application code should not need them:

| Kernel concept | Why hidden | SDK alternative |
|---------------|-----------|-----------------|
| `store.ContentStore` | Direct storage bypass | `TreeGet`, `TreePut` |
| `store.LocationIndex` | Direct index bypass | `TreeList`, `TreeHas` |
| `handler.Registry` | Internal dispatch | `Execute()` |
| `peer.PeerShared` | Internal wiring | Not needed |
| `handler.Request` | Internal message type | `Execute()` builds these |
| `broadcast.Receiver` | Notification plumbing | `OnChange` channels |

If you need these, use entity-core-go directly. The SDK doesn't
prevent it:

```go
// Escape hatch: access the kernel peer
peer := app.Peer()

// For advanced use: custom engines, store backends,
// transport implementations, debugging.
```

## Cross-Language API Mapping

The same concepts appear in every language SDK:

| Concept | Go | Rust | Python | JS |
|---------|-----|------|--------|-----|
| Construction | `entitysdk.New(opts...)` | `EntitySDK::builder().build()` | `EntitySDK(**opts)` | `new EntitySDK(opts)` |
| Tree get | `app.TreeGet(ctx, path)` | `.tree_get(path).await?` | `await sdk.tree_get(path)` | `await sdk.treeGet(path)` |
| Subscribe | `app.OnChange(pat)` → `<-chan` | `.on_change(pat, \|e\| {...})?` | `sdk.on_change(pat, fn)` | `sdk.onChange(pat, fn)` |
| Connect | `app.Connect(ctx, addr)` | `.connect(addr).await?` | `await sdk.connect(addr)` | `await sdk.connect(addr)` |
| Execute | `app.Execute(ctx, h, op, p)` | `.execute(h, op, p).await?` | `await sdk.execute(h, op, p)` | `await sdk.execute(h, op, p)` |
| Wake signal | `app.SetWakeFunc(fn)` | `.set_wake_fn(\|\| {...})` | `sdk.set_wake_fn(fn)` | `sdk.setWakeFn(fn)` |

Go uses `context.Context` for cancellation where other languages use
async cancellation. Go subscriptions return channels where Rust
returns streams and JS returns async iterators.

## Layer 2 Algorithm Contract

The cross-language API mapping above describes **Layer 1**: the
per-language facade. Layer 1 is allowed to vary — Go uses
generics and channels, Rust uses traits and async/await, Python
uses context managers, JS uses promises. The point of Layer 1
is ergonomics in the host language.

There is also a **Layer 2** under the same SDK name: the shared
algorithms that must produce byte-identical entities across all
implementations. If Go chunks content at 4KB boundaries and Rust
at 8KB boundaries, deduplication breaks across implementations
even though both call themselves "the entity SDK." Layer 2 is
the contract that says: given the same high-level call, every
SDK produces the same tree state.

This section enumerates the Layer 2 operations the Go SDK
implements (or will implement) and treats them as normative for
other-language SDKs.

### Layer 2 Operations

| Operation | What must be deterministic |
|-----------|---------------------------|
| **Content chunking** | Chunk size, boundary algorithm (rolling hash parameters), inline-vs-chunked threshold |
| **Slug / path canonicalization** | Lowercasing, ASCII fold, separator choice, length cap, collision handling |
| **CBOR canonical encoding** | Map key ordering, integer width selection, float canonicalization |
| **Pipeline construction** | Continuation entity layout, callback path generation, deliver_to linkage, capability scoping |
| **Bridge sync semantics** | Change detection, conflict resolution policy, idempotency rules |
| **Type validation** | Constraint checking against `system/type/*`, error reporting structure |
| **Subscription pattern matching** | Glob semantics, wildcard expansion, ordering of multi-pattern matches |

### Status

Most Layer 2 operations in the Go SDK are **implicit** — they exist
as code but are not documented as a contract. The work to make them
normative is to:

1. Walk the `entitysdk/` package and identify each Layer 2 operation
2. Extract its parameters into named constants where currently inline
3. Add a normative reference for each operation in this section
4. Provide reference test vectors so other-language SDKs can verify

### Why this matters now

The workbench is the validating implementation for the Go peer.
That makes us the de facto reference for what the Go SDK does. If
we don't document Layer 2 explicitly, the Rust and Python teams
will reverse-engineer our behavior from the validation failures —
which encodes Go's bugs as the spec. Documenting Layer 2 lets us
say "this is intentional" or "this is a Go bug" cleanly.

### What stays in Layer 1

Anything that doesn't affect the bytes that land in the tree:

- Method naming (`TreeGet` vs `tree_get`)
- Error type shapes (Go errors vs Rust `Result` vs Python exceptions)
- Async pattern (channels vs streams vs async iterators)
- Context/cancellation (`context.Context` vs `tokio::CancellationToken`)
- Builder vs functional-options vs keyword arguments

These are facade choices. Differences here are not portability
bugs.

## Relationship to Current Code

The workbench package (entity-workbench-go/workbench/) is the
current home for SDK code. It mixes SDK-material with
workbench-specific application code. The extraction path is clear
because the boundary is already visible:

### Becomes SDK (entity-sdk-go)

| Current file | SDK equivalent | Notes |
|-------------|---------------|-------|
| `executor.go` | Internal to `App` | App provides the same methods |
| `peer_context.go` | `PeerContext` | Uses App instead of Executor |
| `workspace_state.go` | `AppState` | Generalized path prefix |
| `context.go` | `DataContext` interface | May evolve |
| `resolve.go` | `ResolveEntity`, `DecodeEntityData` | Unchanged |
| `tree.go` | `BuildTree`, `FlattenVisible`, etc. | Unchanged |
| `format.go` | `FormatCBOR`, `FormatValue`, etc. | Unchanged |
| `output.go` | `OutputLine`, `FlattenFormattedLine` | Unchanged |
| `handlers.go` | `DiscoverHandlers`, `HandlerInfo` | Unchanged |
| `event_log.go` | `EventLog` | Unchanged |
| `layout.go` | `LayoutNode[W]`, `Navigate` | Unchanged |

### Stays in Workbench (application)

| Current file | Why it stays |
|-------------|-------------|
| `commands.go` | Workbench command palette, action types, content type registry |
| `selection.go` | Workbench navigation history |
| `startup.go` | Workbench default screen configurations |
| `shell_model.go` | Workbench REPL panel |
| `handler_model.go` | Workbench execute console panel |
| `log_model.go` | Workbench log viewer panel |
| `detail_model.go` | Workbench entity detail panel |
| `peer_info_model.go` | Workbench peer info panel |
| `tree_model.go` | Workbench tree browser panel |

The content models are workbench-specific — they encode decisions
about what panels exist, how they behave, what commands they support.
A different application would have different panels.

## Refactoring Path

The SDK doesn't need to be extracted immediately. The workbench
package already has clean internal boundaries. The path:

### Phase 1: Stabilize the API (current)

What exists today maps cleanly. The main work:

- **Unify Executor into App**: `Executor` becomes the internal
  implementation of `App`. The method signatures (`TreeGet`,
  `TreeList`, `TreePut`, `Execute`) are already the right shape.
  `NewExecutor(registry, store, index, peerID)` becomes
  `entitysdk.New(opts...)` — the kernel wiring moves inside.

- **Add context.Context**: Tree operations should accept `ctx` for
  cancellation and timeouts. Currently `Executor` creates its own
  context internally.

- **Generalize AppState**: `WorkspaceState` uses hardcoded
  `workspace/` prefix. `AppState` should accept a configurable
  prefix so different applications can use their own namespace.

### Phase 2: Subscriptions

The biggest missing piece. Current state: renderers poll via
`PeerContext.RefreshIfDirty()` on every frame/event.

Target: `app.OnChange(pattern)` returns a channel. The SDK wires
this through the kernel's subscription engine. PeerContext can
internally subscribe and mark itself dirty on relevant changes
instead of being marked dirty externally.

This also enables the wake function — the SDK coalesces subscription
events and calls the wake function, which triggers the render
framework's event loop.

### Phase 3: Connections

`app.Connect(ctx, addr)` wraps `peer.Connect()`. Remote tree data
appears through the same `TreeGet`/`TreeList` API via entity:// URIs.
`OnChange` works for remote paths — the SDK manages the subscription
chain.

### Phase 4: Handler registration

`app.RegisterHandler(h)` and the closure-based `app.Handle(...)`.
The workbench's application handler (at `workspace/app`) becomes
a normal handler registered through this API.

### Phase 5: Extract module

Move SDK files to `entity-sdk-go/` with its own `go.mod`. The
workbench imports it. The API surface is the module boundary.

This is the same progression as the Rust SDK (entity-sdk-rust
Phase 5: "extract to crate"). Both projects build the SDK inside
the first consumer, then extract when the API stabilizes.

## What a Workbench Built on the SDK Looks Like

```go
func main() {
    // SDK handles peer lifecycle
    app, _ := entitysdk.New(entitysdk.GenerateKeypair())
    defer app.Close()

    // Higher-level helpers
    pc := entitysdk.NewPeerContext(app)
    state := entitysdk.NewAppState(app)
    log := entitysdk.NewEventLog(1000)

    // Wake function bridges SDK events to render framework
    app.SetWakeFunc(func() {
        pc.MarkDirty()
        tviewApp.QueueUpdateDraw(func() {})
    })

    // Subscribe to tree changes
    events, cancel := app.OnChange("workspace/**")
    defer cancel()
    go func() {
        for range events {
            // wake function handles the redraw signal
        }
    }()

    // Application-specific: build screens, create panels, run UI
    screens := workbench.DefaultScreens()
    // ... renderer setup using pc, state, log ...
}
```

The SDK provides the entity-system plumbing. The workbench
application provides the panels, commands, and screen layouts.
A different application (monitoring dashboard, data pipeline
manager, IoT controller) would use the same SDK with completely
different application-level code.

## Type System Access (Future)

Reading and working with entity type definitions. This is how
applications understand what entities mean, not just what data
they contain.

```go
// -- Read a type definition --

typeDef, ok := app.TypeGet(ctx, "doc/paper")
if ok {
    fmt.Printf("extends: %s\n", typeDef.Extends)
    for _, field := range typeDef.Fields {
        fmt.Printf("  %s: %s\n", field.Name, field.Type)
    }
}

// -- List known types --

types, err := app.TypeList(ctx)

// -- Check type hierarchy --

isDoc := app.TypeExtends(ctx, "doc/paper", "doc/base")
```

This tier connects to the workbench's "type awareness" vision:
rendering + comprehension driven by type definitions in the tree.
The SDK provides raw access to type data; applications (like the
workbench) build type-driven panels on top.

## References

- Entity Core Go (kernel): `entity-core-go/`
- Workbench architecture: `docs/architecture/ARCHITECTURE.md`
- Application framework: `docs/architecture/ENTITY-APPLICATION-FRAMEWORK.md`
- Handler integration: `docs/architecture/APPLICATION-HANDLER-INTEGRATION.md`
