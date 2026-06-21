# Entity System Reference

This document describes the core abstractions of the entity system as implemented
in `entity-core-go/core`. Understanding these concepts is essential for working on
the workbench, console, or canvas modules.

## Entity

An **Entity** is an immutable, typed data unit. It has three fields:

| Field | Type | Description |
|-------|------|-------------|
| `Type` | `string` | Type identifier, e.g. `"system/handler/interface"` |
| `Data` | `cbor.RawMessage` | CBOR-encoded payload, never re-encoded |
| `ContentHash` | `hash.Hash` | SHA-256 hash of `{type, data}` in ECF canonical form |

Entities are **content-addressed**: two entities with identical type and data
produce the same hash. The `Data` field is stored as raw bytes to preserve
hash fidelity — it is never decoded and re-encoded during storage or forwarding.

**Key functions:**
- `entity.NewEntity(type, data)` — create and compute hash
- `entity.Validate()` — verify hash integrity
- `entity.DiagnoseHash()` — human-readable hash diagnostics

## ContentStore

The **ContentStore** is an immutable, content-addressed store mapping `Hash → Entity`.

```
Put(entity) → hash         // store entity, return computed hash
Get(hash)   → entity, ok   // retrieve by hash
Has(hash)   → bool         // check existence
Remove(hash) → bool        // delete
Len()       → int          // count
```

The store **recomputes the hash on Put** — it never trusts the entity's claimed
ContentHash. This guarantees integrity.

**Implementation:** `store.MemoryContentStore` (thread-safe, in-memory map).

## LocationIndex

The **LocationIndex** is a mutable namespace mapping `path → Hash`. This is
the "directory" layer that gives entities human-readable locations.

```
Set(path, hash)              // bind path to hash
Get(path) → hash, ok         // look up
Has(path) → bool             // check
Remove(path) → hash, ok      // unbind
List(prefix) → []LocationEntry  // list by prefix
```

A `LocationEntry` is a `{Path, Hash}` pair.

**Implementation:** `store.MemoryLocationIndex` (thread-safe, in-memory map).

## Paths and Peer Namespaces

All paths in the canonical tree are **fully qualified** with a peer ID prefix:

```
{peerID}/bare/path
```

Examples:
```
Qm8nJemDqTPGT8s5fZYpXA3pXv2WScbejV5YvjkVoN1t4VrTqd/data/myfile
Qm8nJemDqTPGT8s5fZYpXA3pXv2WScbejV5YvjkVoN1t4VrTqd/system/handler/tree
```

The peer ID is the first path segment and is detected by a heuristic:
a segment of 40+ alphanumeric characters before the first `/`.

**Index layers** (innermost to outermost):
1. `MemoryLocationIndex` — stores raw qualified paths
2. `NotifyingLocationIndex` — wraps inner index, emits `TreeChangeEvent` on mutations
3. `NamespacedIndex` — auto-prefixes bare paths with local peer ID

When a handler or local code writes to `"data/foo"`, the NamespacedIndex
stores it as `"{localPeerID}/data/foo"`. This allows a single tree to hold
data from multiple peers.

## Handlers

A **Handler** is a named processor registered at a pattern URI. Handlers
implement the `handler.Handler` interface:

```go
Handle(ctx, *Request) → (*Response, error)
Name() string
```

**Request** contains: `Path`, `Operation`, `Params` (entity), `Context`.

**Response** contains: `Status` (uint), `Result` (entity), `Included` (map of extra entities).

**HandlerContext** provides the handler with access to:
- `Store` and `LocationIndex` for data access
- `Author` and `LocalPeerID` for identity
- `Execute` callback for invoking other handlers
- Capability tokens for authorization

**Handler Registry** uses longest-prefix-match routing. A handler at
`"system/tree"` matches requests to `"system/tree"`, `"system/tree/foo"`, etc.

**Handler metadata** is stored in the tree at `system/handler/{pattern}` as
entities of type `system/handler/interface`. These contain:
- `Pattern` — the URI pattern
- `Name` — human-readable name
- `Operations` — map of operation name → `{InputType, OutputType}`

**Built-in handlers:**
- `system/protocol/connect` — peer handshake
- `system/tree` — get, put, list, snapshot, diff, merge, extract

## TreeEvents

A `TreeChangeEvent` is emitted whenever the LocationIndex is mutated:

```go
TreeChangeEvent {
    Path         string     // qualified path (with peer ID)
    PeerID       string     // extracted peer ID
    Hash         hash.Hash  // current hash (zero for deletes)
    PreviousHash hash.Hash  // previous hash (zero for creates)
    ChangeType   ChangeType // Created | Modified | Deleted
}
```

Events flow through:
1. `NotifyingLocationIndex` emits on Set/Remove
2. Fan-out distributes to multiple sinks
3. `Peer.TreeEvents()` channel exposes to external subscribers

The workbench uses TreeEvents to set the dirty flag on PeerContext,
triggering a UI refresh on the next frame.

## Peer

A **Peer** is the top-level construct that ties everything together:

```
Peer
├── Identity (keypair, peerID, identity entity)
├── ContentStore (hash → entity)
├── LocationIndex (path → hash, wrapped in Notifying + Namespaced)
├── Handler Registry (pattern → handler)
├── Dispatcher (envelope validation, auth, routing)
├── TreeEvents channel
└── Connections (to remote peers)
```

**Lifecycle:**
1. Create keypair and derive identity
2. Initialize store and index
3. Register handlers (built-in + custom)
4. Seed system tree (type definitions, handler specs, capability grants)
5. Listen for connections or connect to remote peers

## CBOR and ECF

All entity data is encoded in **CBOR** (RFC 8949). The system uses
**ECF (Entity Canonical Form)** — deterministic CBOR per RFC 8949 Section 4.2:

- Map keys sorted in bytewise lexicographic order
- Minimal-length integer encoding
- No indefinite-length items

This ensures that identical data always produces identical bytes, which is
essential for content-addressed hashing.

**Hash computation:**
1. CBOR-encode `{data: <raw>, type: <string>}` using ECF
2. SHA-256 hash the resulting bytes
3. Prepend algorithm byte (`0x00` for SHA-256)
4. Result: 33 bytes (1 algorithm + 32 digest)

## System Path Conventions

```
system/
├── handler/          # handler metadata (HandlerInterfaceData entities)
│   ├── system/tree
│   ├── system/protocol/connect
│   └── {custom-pattern}
├── capability/
│   └── grants/       # capability tokens for handler authorization
├── identity          # peer identity entity
└── type/             # type definitions
data/                 # application data (user-defined)
```

## Workbench Abstractions

The `workbench` module provides shared logic consumed by both the
console (tview) and canvas (raylib) UIs:

| Abstraction | Purpose |
|-------------|---------|
| `PeerContext` | Wraps store + index with dirty-flag caching |
| `SelectionState` | Current path selection with back/forward history |
| `TreeNode` | Hierarchical path tree for navigation UIs |
| `Command` / `Action` | Renderer-agnostic command palette and action system |
| `Executor` | Handler dispatch for a peer (builds request, calls registry) |
| `DispatchFunc` | Callback type for panels to execute handler operations |
| `EventLog` | Thread-safe ring buffer for application events |
| `ResolveEntity()` | Path → decoded entity lookup |
| `DecodeEntityData()` | CBOR bytes → decoded interface{} |
| `FormatCBOR()` | Decoded CBOR → renderer-neutral formatted lines |
| `FormatValue()` | Single value → kind-tagged text |
| `DiscoverHandlers()` | Scan tree for handler metadata |
| `FormatHexDump()` | Raw bytes → hex line display |
| `ListByPrefix()` | Filter entries by path prefix |

UIs consume these abstractions and apply their own rendering (tview color
tags, raylib draw calls). Business logic stays in workbench; UIs are
thin renderers that translate actions back to workbench operations.
