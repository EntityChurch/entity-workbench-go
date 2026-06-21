# Entity Shell — Direction

The canvas/raylib renderer has been removed. References to
`canvas/...` in this doc are historical; the renderer pair is now
console+Avalonia. The shell-as-primary-interface direction is
unchanged — it's the GUI consumer set that shifted.

**Repo:** `entity-workbench-go`. **Owner:** workbench team.

This document establishes the entity shell as a **primary user
interface** for the entity-systems ecosystem and pulls together the
positioning, current state, and open design questions in one place.
It supersedes the original Stream C "prototype text-mode renderer"
framing.

The shell is positioned to become the **leading edge of feature
development** for peer/identity/capability management and other
cross-cutting concerns that don't fit naturally into a GUI panel
shape. CLI features ship; GUI panels follow as presentation layers
over the same command vocabulary.

This is a design doc, not a proposal. Specific items (path-aware
routing, fan-out syntax, pipe semantics) are identified as candidates
for formal proposals — coordinated with the architecture team where
their concerns extend beyond the workbench-go boundary.

---

## 1. Positioning

### 1.1 Why the shell matters

The entity protocol is fundamentally about **peer-to-peer
coordination**: connections, identity, capabilities, entity
exchange, dispatch. Many of these operations are inherently
shell-shaped:

- They take args and produce structured results.
- They compose via piping, scripting, automation.
- They benefit from non-interactive use (CI, headless servers).
- They bridge naturally to the entity-programming model where
  programs are entity graphs in the tree.

A GUI panel surfaces a curated subset of capability — the shell
surfaces all of it. As the system grows toward identity management,
peer spawn/management, network topology, and continuation-driven
async flows, the shell is the natural surface for feature delivery.

### 1.2 Shell-first development

Going forward, new SDK features should land at the CLI level first:

1. SDK adds a handler (or refines a peer-level capability).
2. Shell command exposes it via the existing `exec` + dedicated verb.
3. GUI panel (when wanted) wraps the same `shellcmd` Result.

Benefits:
- Immediate value from SDK work.
- One command vocabulary; multiple presentation backends.
- The CLI is testable headlessly; the GUI can lag without blocking
  feature iteration.
- Consistent with the **DUtils** prior art (§2.3) and the
  cross-implementation CLI analysis.

### 1.3 Relationship to the GUI workbench

The GUI workbench (canvas / console) keeps an **embedded shell
panel** as one of its content types. Long-term, that panel becomes
a thin presentation adapter over `shellcmd` — same command
vocabulary, panel-shaped output (`OutputLine` slice instead of
stdout). Shell-first work pays out for both surfaces.

---

## 2. Prior art and convergence

### 2.1 The DUtils naming convention

The cross-impl CLI tree-tools convention (v5.0 proposal) defines:

- **One-shot mode (outside the shell):** `d-` prefix to disambiguate
  from system commands. `dls`, `dcat`, `dcp`, `dmv`, `drm`, `dgrep`,
  `dfind`, `dtree`, `dsync`, `dsh`.
- **Interactive mode (inside the shell):** unprefixed. Already in
  entity context, prefix is redundant.

**Current implementation** matches this for interactive mode
(unprefixed). One-shot mode currently uses `entity-shell ls /path`;
installing `dls` etc. as aliases is a packaging decision deferred to
distribution time.

### 2.2 The 9-command core

Three independent CLI implementations (Go, Python, Rust) converged
on the same operation set:

| Command | Op |
|---|---|
| `connect` | establish session with a peer (handshake → capabilities) |
| `disconnect` | close a session |
| `cd` | change working directory (peer + path) |
| `pwd` | print working directory |
| `ls` | list children at a path |
| `cat` | display the entity bound at a path |
| `tree` | recursive listing |
| `exec` | dispatch a handler operation (the universal escape hatch) |
| `info` | show connection details |

This is what we ported into `shellcmd/` in v1. Open questions about
naming (`put` vs `write`, `cat` vs `read`) are flagged in §11.

### 2.3 Continuations vs. shell pipes — they're different concepts

This is the most commonly conflated pair. They're separate.

**Continuations:**
- A protocol-level async chaining primitive.
- A continuation is an **entity** stored at a path.
- It says: "when an async result arrives, dispatch it to handler X
  with op Y, injecting the result into field Z."
- One-shot continuations self-delete; standing continuations fire on
  every delivery (subscriptions).
- Used for async fetch-then-store, subscription delivery, callback
  patterns.
- Already in production: `entity-core-go/cmd/entity-sync` sets up
  cross-peer continuations to drive bidirectional sync.

**Shell pipes** (proposed in v5.0 §6.3, **not yet implemented**):
- A CLI-level **typed entity flow**.
- Each item in the pipe is an entity (with a hash, type, decoded
  data) — not a byte stream.
- Conceptually:
  ```
  dls peer-c:/local/media --type=audio
    | dgrep --field=metadata.artist "Miles Davis"
    | exec peer-a:/apps/music/player --op=enqueue
  ```
- No consensus on syntax or semantics yet.

**Open question:** do shell pipes translate down to continuations
under the hood, or are they a separate CLI-only construct? See §11.

### 2.4 Entity-sync as a reference for cross-peer flows

`entity-core-go/cmd/entity-sync` (already shipping) shows the
cross-peer continuation pattern in production:

- A "from" peer emits tree changes.
- A continuation entity stored at the "to" peer dispatches incoming
  deliveries into local `system/tree put` operations.
- Bidirectional mode sets up the symmetric pair.
- Strategies: `source-wins`, `target-wins`, `no-overwrite`.

The shell's eventual `dsync` should be a thin wrapper over the same
mechanism — possibly invoking the existing entity-sync logic
directly once it's accessible from `entitysdk`.

---

## 3. Current state (v1, this session)

### 3.1 What shipped

Two new modules in `entity-workbench-go`:

```
shellcmd/                  command vocabulary, presentation-neutral
├── path.go                Path type with peer-alias support (+tests)
├── result.go              sum-typed Result (none/message/path/listing/
│                          entity/tree/dispatch/info/lines)
├── commands.go            Registry + Default()
├── shell.go               Shell state (conns, wd, identity)
├── cmd_nav.go             ls/cd/pwd/tree
├── cmd_entity.go          cat/exec
├── cmd_connect.go         connect/disconnect (v1: stubs for remote)
└── cmd_info.go            info/help

shell/                     standalone binary entity-shell
├── cmd/entity-shell/main.go
├── app.go                 App (peer bootstrap + Shell + Registry)
├── repl.go                REPL loop, prompt, arg parsing
└── format.go              text + JSON formatters for Result
```

### 3.2 What works

- `entity-shell` (no args) → REPL mode with persistent local in-process
  peer (spawned via `entitysdk.CreatePeer`).
- `entity-shell <cmd> [args...]` → one-shot mode.
- `entity-shell --json <cmd>` → structured JSON output.
- All 9 core commands operate on the local peer's tree, which
  auto-registers `system/tree` (and via PeerConfig, optional
  query/subscription extensions).
- `pwd`, `cd`, `ls` (with peer-aware path semantics), `cat`, `tree
  -depth N -v`, `exec handler op [resource] [json-params]`, `info`,
  `help` — all functional.
- `Result` is renderer-neutral; both stdout text and JSON formatters
  consume it.

### 3.3 What's stubbed

- `connect <alias> <host:port>` — returns "not yet supported" with
  explanation. Reason: see §4.
- Remote-peer commands (`ls /{remote-peer-id}/path`, etc.) — return
  "remote-peer X not yet supported." Same reason.
- `workbench/shell_model.go` (panel shell) — still uses its own
  micro-vocabulary. Refactor blocked on canvas/console migrating to
  `entitysdk.CreatePeer`. See §10.

---

## 4. Multi-peer model

### 4.1 The mental model

Every peer owns its own tree. The shell's local peer is one such
peer; remote peers reached over connections are others. The shell
provides a unified navigation surface across all of them via
**peer-qualified paths**:

```
local peer's tree:        /{local-peer-id}/system/...
remote peer's tree:       /{remote-peer-id}/local/files/...
via alias:                local:/system/...   (or)   nas:/local/files/...
```

`ls /` at the shell root lists **connected peers** (each is a row
with alias, address, peer-id). `cd nas:` jumps to the remote peer's
tree root. From there, navigation is filesystem-shaped — but every
operation is capability-mediated by the remote peer's authority.

### 4.2 What the local peer is

The local peer is **always there** — spawned at shell startup by
`entitysdk.CreatePeer`. It's listed under the alias `local` (or
whatever `-alias` flag overrides). It serves three roles:

1. **The shell's own identity** — operations the shell originates
   are signed by the local peer's keypair.
2. **The scratch tree** — `cd local:` and use it like a working
   directory.
3. **The dispatch surface** — all operations, local or remote,
   route through the local peer's executor and connection pool.

### 4.3 Capability-mediated cross-peer view

There is **no synced-view abstraction**. When the shell navigates
into a remote peer's tree, every `ls` / `cat` / `exec` is an op
dispatched to that peer; the peer enforces its capability tokens on
every call. You see what its capabilities let you see — the
"perspective" is the remote peer's, mediated by what it's granted
the shell's local peer.

This is consistent with the protocol-level model (capability tokens
in handshake → enforced on each EXECUTE) and avoids inventing a
new "view" abstraction.

### 4.4 What's missing — `entitysdk.Executor` bypasses the remote-routing path

Cross-peer routing is **already implemented at the protocol layer in
core-go.** `peer.Peer.RegisterRemote(peerID, addr)` plus
`peer.remoteExecute(uri, ...)` (core/peer/remote.go:59) plus the
dispatcher's `RemoteExecute` callback (core/peer/peer.go:203) means
that dispatching to a URI of the form `entity://{remote-peer-id}/...`
*automatically* routes through a pooled connection — pulling from
`peer.Peer.remote.conns` or dialing on first use. The connection pool
lives at the peer level, where it belongs.

The actual SDK gap is that `entitysdk.Executor` calls
`Registry.Dispatch` directly (executor.go:121) — local handlers only.
The remote-routing logic lives in `protocol.Dispatcher`, never reached
by Executor today. So every `AppPeer.Get/Put/List/...` call is
local-only, by construction.

The minimal fix has three parts:

1. In `Executor`, detect non-local URIs and route through the peer's
   `Dispatcher.RemoteExecute` (or call the connection's `Execute`
   directly via the pool) instead of `Registry.Dispatch`. Local URIs
   keep the existing path.
2. In `AppPeer.Connect(ctx, addr)`, after handshake, also call
   `peer.RegisterRemote(remotePeerID, addr)` so the address is in the
   local tree for resolution and the dispatcher's remote path can find
   it.
3. In `shellcmd`, pass peer-qualified URIs to dispatch calls
   (`entity://{peerID}/system/tree`) instead of bare paths; drop the
   `pc.Address != ""` stub branches.

Once this lands:

- The shell's `connect` becomes a real implementation.
- All existing commands work for remote peers without code changes.
- Any future entitysdk consumer (canvas, console, future Godot
  workbench) gets the same behavior for free.

**Estimated cost:** ~half day SDK + half day shell wiring + tests.
**This is still the single highest-leverage piece of work in the
shell roadmap** — but it is reconnecting wires that already exist,
not building a new abstraction.

### 4.5 Two SDK surfaces — AppPeer and Client

A process is either a peer or it isn't. The SDK reflects that with
two distinct surfaces:

| | `entitysdk.Client` | `entitysdk.AppPeer` |
|---|---|---|
| Is a peer? | no | yes |
| Has keypair | yes | yes |
| Has tree (store + location index) | no | yes (always) |
| Has handlers | no | yes (always — `system/tree`, `system/protocol/connect`, etc., seeded at construction) |
| Has dispatcher | no | yes |
| Has a peer-id-as-tree-namespace | no | yes |
| Has connection state | one connection it owns | pool of N pooled conns |
| Can receive callbacks (continuations, subscriptions) | no | yes |

**`entitysdk.AppPeer`:** a peer. Real `peer.Peer` underneath. Always
has a tree (the data model demands it — every peer's tree is seeded
at startup with `system/type/*`, `system/handler/*`,
`system/capability/grants/*`). Always has the standard handler set.
Required for anything that wants to receive responses through a
peer's machinery, run long-lived, or be addressed by other peers.

**`entitysdk.Client` (new primitive):** a TCP wrapper that speaks the
protocol but is *not* a peer. Has a keypair (for signing handshake
and EXECUTEs), wraps one `peer.Connection`, does handshake + Execute
+ Close. No store, no handlers, no dispatcher, no pool. The thing
that makes requests, but doesn't participate in the network as an
addressable node. Core-go's `cmd/internal/validate.PeerClient` is
the existing reference shape; entitysdk should re-expose it as a
first-class surface.

The line between them is about **API surface and lifetime**, not
category. A `Client` is plausibly implemented as a thin facade that
spins up a minimal `peer.Peer` for one connection and tears it down
on Close — same substrate as `AppPeer`, just shorter-lived and
narrower API. The wire protocol can't tell them apart; the remote
peer just sees a signed EXECUTE from some peer-id. What we're
exposing in the SDK is two ergonomic patterns:

- The "stay alive, hold many connections, be addressed" pattern
  (AppPeer).
- The "spin up, send one or a few requests, exit" pattern (Client).

The earlier "minimal AppPeer with no tree" framing was an attempt
to invent a middle ground; not needed.

#### 4.5.1 Routing rule — which surface to use

**The keypair you're operating under determines the surface:**

- Same keypair as a live AppPeer in the process → route through that
  AppPeer. Reuses the pooled connection, signs as the peer.
- Different keypair (acting "as someone else") → use a `Client`. (Or,
  in v2 multi-identity, another AppPeer for that identity.)

Why this rule matters: if a process has a live AppPeer connected to
remote peer R under identity X, and *also* opens a throwaway client
connection to R *also under identity X*, R now holds two TCP
connections from the same logical entity. That confuses connection
state, capability bookkeeping, and any per-connection rate limiting.
Avoid it.

Corollary: **don't use a `Client` against a remote peer your AppPeer
has already pooled a connection to under the same keypair.** Either
go through the AppPeer or operate under a different identity.

#### 4.5.2 When each surface fits

| Use case | Surface |
|---|---|
| Long-running REPL with persistent identity | AppPeer |
| Subscriptions / continuations / callback delivery | AppPeer (Client can't receive) |
| One-shot CLI invocation, no further state needed | Client (or AppPeer if already in a session) |
| Fan-out across a peer group (parallel reads, aggregate, exit) | Client (separate ephemeral conns; doesn't pollute the AppPeer's pool) |
| Acting under a non-default identity for one operation | Client with that identity |

The shell's REPL session uses AppPeer. One-shot mode (`entity-shell
ls peerA:/path`) can use Client for lower overhead. Future fan-out
commands use Client per target peer. These are deliberate per-call
choices, not implicit fallback behavior.

#### 4.5.3 Open question — connection sharing across surfaces

If a single process holds an AppPeer (identity X) *and* wants to
make a `Client` call under the same identity X to a remote peer R —
should the SDK transparently route that through the AppPeer's
pooled connection rather than opening a fresh one?

v1 stance: **no transparent sharing.** Callers respect the routing
rule themselves. Keeps Client simple, keeps AppPeer's pool semantics
clean.

v2 (if profiling motivates): a `Client` could opportunistically
borrow a pooled connection from a registered AppPeer when the
keypair matches. Optimization, not core design. Tracked in §11.

### 4.6 AppPeer's configurable axes

An AppPeer always has a tree, a store, the standard handler set, and
a dispatcher — those are not configurable, they're what being a peer
*is*. What *is* configurable matches the real axes already exposed
by `entitysdk.PeerConfig` (or candidates for it):

| Axis | Config | Effect |
|---|---|---|
| Identity source | `Keypair` field, or auto-generated | shell flag `--identity NAME` loads from `~/.entity/identities/NAME/` |
| Listener | `ListenAddr` field; empty = no listener | reachable to other peers if set; outbound-only otherwise |
| Storage | `Storage.Kind` (only `memory` today, persistent spec'd) | shell flag `--persistent` selects `~/.entity/peers/shell/` once persistent storage lands |
| Extensions | `Extensions.Subscription`, etc. | opt in to extra handlers (subscription, history, …) |

The shell's three positions on these axes:

- **Default** — ephemeral keypair, no listener, in-memory storage,
  default extensions only. The shell is a peer but isn't reachable
  and doesn't survive restart. Suitable for one-shot REPL sessions.
- **`--identity NAME`** — load keypair from disk; tree still
  in-memory unless combined with `--persistent`.
- **`--persistent`** — disk-backed storage; shell session state
  survives restart. Implies persistence work in the SDK before this
  is real.

There is no "shell starts without a tree" mode; that would be Client
mode (§4.5), which is a different surface entirely.

### 4.7 Identity and capability flow at-a-glance

The shell's identity is a **keypair**, period. It comes from one of:

- A stored identity (`--identity alice` → load from
  `~/.entity/identities/alice/`).
- Auto-generation (ephemeral, default — useful only against
  open-grant peers).

The **user → identity binding lives outside the shell.** Whatever
unlocks the key file (passphrase prompt, hardware token, OS
keychain) happens before or during shell startup. The shell never
runs an interactive auth flow of its own; it gets a keypair and uses
it. This keeps the shell's job small — it does not authenticate
users, it acts on behalf of an already-resolved identity.

**Permissions are not enforced by the shell.** Every EXECUTE the
shell sends is signed by the active keypair; the *remote peer*
checks grants:

1. At handshake (`system/protocol/connect` AUTHENTICATE step), the
   remote peer's connect-handler validates whatever capability tokens
   the connecting identity presents. The connection state carries
   the resulting grant.
2. On every EXECUTE the dispatcher checks the operation against the
   grant attached to the connection state.

The shell observes 403 responses and surfaces them — it does not
make policy decisions. Why dev environments "just work" today: most
dev peers are configured with `WithConnectionGrants(OpenAccessGrants())`
at startup, which is non-enforcing. Production peers configure a
real `GrantResolver` and connections are gated on actual tokens.

**The shell holds tokens.** Issuance (`cap grant`) is itself an
EXECUTE against a peer that has the authority to issue. Inspection
(`cap list`) reads tokens out of the local tree
(`system/capability/grants/...` per the existing convention,
peer.go:514). These are future commands (§5.3, §11.7); they sit on
the same dispatch path as everything else.

### 4.8 Multi-identity — deferred to v2

What if the user wants to run as alice for some ops and bob for
others within one shell session?

The protocol's identity-per-connection assumption forbids "juggling
keys on one peer" — a connection that completed AUTHENTICATE under
alice's keypair cannot send a bob-signed EXECUTE through it.

The clean answer is therefore: **multi-identity = multiple AppPeers
in one shell process, one per active identity.** The shell holds a
map `identity-name → AppPeer`; `identity use bob` switches the
active one (lazily spawning bob's AppPeer on first use). Each
AppPeer has its own keypair, its own connection pool, its own (if
enabled) local tree.

This is genuinely cheap per identity (a keypair plus minimal handler
set is small in memory) and avoids any new abstraction — each
AppPeer is just another instance of the same thing.

Deferred from Phase 2 because:

- v1 use cases are all single-identity.
- It surfaces design questions about cross-identity local-tree
  visibility (does alice see bob's scratch? probably not) that
  benefit from concrete usage to answer.

Tracked for v2 once `--identity` lands and the single-identity case
is exercised.

---

## 5. Command vocabulary direction

### 5.1 v1 commands (shipping)

The 9 core commands from §2.2, ported and operating on the local
peer. Their shape doesn't change as remote-routing lands —
remoteness becomes transparent.

### 5.2 Near-term additions

These are natural extensions of the v1 vocabulary; each maps to an
existing or near-existing SDK surface:

| Command | Maps to | Status |
|---|---|---|
| `put <path> <type> <data>` | `appPeer.Put` | SDK ready; command not yet added |
| `rm <path>` | `appPeer.Remove` | SDK ready; not yet added |
| `cp <src> <dst>` | `Get` + `Put` | trivial composition |
| `has <path>` | `appPeer.Has` | SDK ready; not yet added |

### 5.3 Identity / peer / capability management

This is the **leading-edge for SDK feature development.** Direction:

- `identity create [name]` — generate a keypair, store under
  `~/.entity/identities/{name}`.
- `identity list` — enumerate stored identities.
- `identity use <name>` — switch the shell's active identity.
- `peer spawn [config]` — start a new local peer process, optionally
  with listen address.
- `peer list` — list known peers (configured + connected).
- `peer kill <alias>` — terminate a peer the shell spawned.
- `cap grant <peer> <pattern> <ops>` — issue a capability to another
  peer.
- `cap list [peer]` — list capabilities granted/received.
- `cap revoke <id>` — revoke a previously-granted capability.

These are sketches — each command needs SDK support to land. The
shell ships a stub when the SDK isn't ready (clear "not yet
supported" message) so the vocabulary stabilizes ahead of
implementation.

### 5.4 Tree-flow commands (medium-term)

Once path-aware routing lands and pipes are designed (§6):

| Command | Purpose |
|---|---|
| `cp <src> <dst>` | copy single entity (works cross-peer once routing lands) |
| `mv <src> <dst>` | copy + remove on source |
| `sync <peer-a:/path> <peer-b:/path>` | tree-level sync via continuations (wraps entity-sync) |
| `fetch <remote:path> [<local:path>]` | one-shot directional pull |
| `push <local:path> <remote:path>` | one-shot directional push |
| `grep <pattern> [path]` | search entity content |
| `find <predicate> [path]` | path/type search |
| `watch <path>` | subscribe to tree changes (continuation-backed) |
| `diff <snapshot-a> <snapshot-b>` | structural diff via `system/tree diff` |

### 5.5 Compute / programming commands (long-term)

The architecture papers describe an **entity programming model**
where programs are entity graphs in the tree (Layer 4 in
`ENTITY-COMPUTATION-PROGRAMMING.md`). Shell commands eventually
include:

- `eval <entity>` — evaluate a computation entity, return result.
- `deps <entity>` — show the entity's dependency graph.
- `purity <entity>` — check whether subgraph is pure (AOT-compilable).

**Scope decision needed:** how deep do we go into compute /
continuation territory in the shell? See §11.

---

## 6. Piping and flows

### 6.1 The two distinct concepts

Repeating §2.3 because it's load-bearing:

1. **Continuations** — protocol-level async chaining primitive. Used
   by entity-sync today. Will be used by the shell's eventual
   `watch`, `sync`, `fetch` commands. *Not* what the user types as
   a pipe.
2. **Shell pipes** — CLI-level typed entity flow. Each item is an
   entity (hash + type + data). Not yet implemented in any CLI.

The user might *eventually* see pipes that **compile down to
continuation graphs** for execution — but that's a translation, not
an identity. The shell pipe syntax is the user-facing concept; the
continuation graph is the runtime mechanism.

### 6.2 Pipe semantics — open questions

What flows through a pipe? The most defensible model:

- **Each pipe stage receives a stream of typed entities.**
- **Each stage emits a stream of typed entities** (possibly
  filtered, transformed, or new ones).
- **Type metadata flows with the entity** so downstream stages can
  predicate on it.
- **Errors / status** flow as a separate stderr-equivalent
  channel (or out-of-band).

Concrete questions for a future proposal (§11):

- Does each stage emit on completion (batch) or as it processes
  (streaming)? Streaming is more useful but harder to specify.
- Is the pipe always linear, or do we support fan-out / fan-in?
- Are pipes evaluated lazily (downstream demand pulls upstream) or
  eagerly?
- What does `cat` emit when piped — a single entity payload? The
  decoded content? The raw entity?
- What does `ls` emit — listing-rows or full entities (one per
  child)?

### 6.3 Fan-out across peer groups

User-stated requirement: "I may want to list a common path across
this group of peers." Not specified anywhere in the architecture
yet — this is genuinely new design.

Sketch of possible syntax (none of these are decided):

```
# Explicit list:
dls --peers=peer-a,peer-b,peer-c common/path

# Group:
dpeer group create devs peer-a peer-b peer-c
dls --group=devs common/path

# Glob over connected:
dls --peers='*' common/path
```

Output shape questions:

- Per-peer output blocks (separated by peer-id headers)?
- Aggregated single listing (with provenance per row)?
- Configurable via flag?
- What about disagreements (peer A says path exists, peer B says it
  doesn't)?

This deserves a focused proposal — the design space is real and
has implications for piping (a fan-out producer feeding a
single-stream consumer needs aggregation semantics).

---

## 7. Entity transfer

### 7.1 Semantics — copy, not move

Entities are content-addressed and immutable. "Transferring an
entity" means the destination peer gets a copy in its store; the
source still has it. This is structural — same content hash can
exist in multiple peers without contradiction.

### 7.2 Shell verbs

| Verb | Mechanism | Scope |
|---|---|---|
| `cp src dst` | GET + PUT | single entity |
| `mv src dst` | GET + PUT + REMOVE on source | single entity (atomicity caveats) |
| `sync` | continuations on both sides | tree subset, ongoing |
| `fetch remote local` | one-shot directional GET + PUT local | single or subtree |
| `push local remote` | one-shot directional PUT remote | single or subtree |

`mv` raises atomicity questions (what if PUT succeeds but REMOVE
fails?). For v1 we treat `mv` as `cp` + warning the user to verify;
a transactional version is a future concern.

### 7.3 Continuation-backed transfers

The advanced cases (`sync`, `watch`, `fetch` with follow-on
processing) are continuation-driven. The shell command sets up the
continuation entity and, where bidirectional, the symmetric pair.
Entity-sync's existing implementation is the reference.

---

## 8. Embedding inside canvas / console

### 8.1 The embedded shell panel

Canvas and console embed `shellpanel/` — a thin presentation adapter
over `shellcmd`. Each panel owns its own `*shellcmd.Shell` over a
single `*shellcmd.ShellWorkspace` (shared across all panels in one
process). On submit, the panel dispatches to `shellcmd.Default()` —
same registry the standalone REPL uses — and renders the resulting
`shellcmd.Result` as `[]workbench.OutputLine` via
`shellpanel.RenderResult` (faithful sibling of `shell/format.go::FormatText`).

The full 25+ verb set is available in the embedded panel. Aliases
added in one panel are visible from any other panel; per-panel state
is WD + command history only.

### 8.2 Sharing op logic with non-shell panels — the verb-op pattern

The embedded shell panel is one consumer of shellcmd. The other —
and the more important one architecturally — is **non-shell panels
that drive the same operations the CLI verbs do**: the execute-
console panel (handler dispatch), future revision/mount/conn
panels, etc.

**The rule: write the op once, reuse it twice.**

Each shellcmd command splits into two layers:

1. **The exported op** — a function in shellcmd that takes
   structured arguments (typed parameters, not strings), does the
   work, returns the SDK-shaped result. Example:
   `shellcmd.Exec(pc *PeerConn, handler, op string, resource *types.ResourceTarget, params map[string]interface{}) (*entitysdk.Response, error)`.
2. **The thin verb** — `cmdExec(sh *Shell, args []string) (Result, error)`
   parses the CLI args, calls the op, projects the result into the
   `shellcmd.Result` shape for text rendering.

Panels skip the thin-verb layer entirely. They call the op directly
with structured inputs, getting back the raw SDK Response (or
whatever typed shape the op returns). No fake CLI argument
assembly, no `Dispatch("verb", sh, args)` round-trip, no
`Result → Response` adapter.

**Why this shape:**

- The panel doesn't pretend to be the terminal. Its inputs are
  structured (selected handler from a list, op picked from a spec,
  resource picked from a path tree), and its outputs flow into a
  structured renderer. Forcing it through string-args + Result-kind
  decoding is paying for a CLI tax the panel doesn't owe.
- The op layer is the natural extension point. Adding a new verb
  means writing the op, then writing the thin verb. Adding a new
  panel that needs the same operation means importing the op. No
  duplication, no drift between what the CLI does and what the
  panel does.
- The `cmdFoo → Foo` split is enforced by file structure: ops live
  next to their verbs in shellcmd, but the op is exported and the
  verb is package-private. Panel code grepping for what to call
  finds the exported symbol.

`shellcmd.Exec` is extracted out of `cmdExec`, and canvas + console's
panel dispatchExecute call it. Both panels route through the canonical
op; the hand-rolled `peerCtx.Executor().Execute(path, op)` path is gone.

**Standing convention going forward:** when adding a new
shellcmd verb that's a candidate for panel reuse (anything
revision-, mount-, capability-, or peer-mgmt-shaped),
extract the op first, then write the thin verb around it. Migrate
existing verbs to this shape as panels need them — no big-bang
refactor.

**Candidates for the next extractions** (in rough order of expected
panel-reuse demand):

- **`mount` / `unmount` / `mounts`** — `localfiles` admin panel
  (list current mounts with their filters, add/remove without
  retyping flags). Today's `cmdMount` is ~80 LOC of arg parsing
  before it hits the workbench-side install transaction; the op
  shape would be `Mount(pc, fsDir, targetPrefix, include, exclude)
  → MountResult`.
- **`revision config put` / `revision commit` / `revision log` /
  `revision follow`** — version-tracked-prefix panel, DAG viewer,
  remote-status panel. These already have typed clients
  (`entitysdk.RevisionClient`) so the "op" is mostly already there;
  the shellcmd verbs just wrap it with arg parsing. The extraction
  here is moving the verb body into a shared helper near
  `cmdRevisionConfig` so panels can call without re-parsing args.
- **`capability mint` / role verbs** — capability inspector panel,
  cap-minting flow for new peers.
- **`connect` / `disconnect` / `info`** — peer-management panel
  (alias list, addresses, grant inspection).

The pattern (op as exported helper, verb as thin parser) is the
same in each case. We don't migrate them all at once — we wait for
the matching panel to need it, so each migration ships with a
concrete consumer rather than as speculative refactor.

### 8.3 Open follow-ups

- **Custom URI / resource path in execute-console.** The panel's
  custom-URI mode lets the user type a `path` field that overrides
  the handler URI — today the panel collapses both into the
  `path → Execute(path, op)` call, which has subtly different
  semantics from `Exec(handler, op, resource, ...)`. When that mode
  earns its keep, reshape the panel UI to pass handler + resource as
  separate structured fields, then route through `shellcmd.Exec`'s
  resource argument directly.
- **Article-view Save through `AppPeer.Put`.** Markdown-view today
  writes via L0 `Store.Put` (round-trips through the mount transform
  fine). Routing through `AppPeer.Put` for cap-checked writes is the
  proper-shape version; deferred until it earns its keep.

### 8.4 Cross-panel coordination — implement the convention, don't redesign it

The architecture team has pinned the selection / action / propagation
model in the entity-workbench-app guide, based on three-impl consensus
across the Go workbench + egui-Rust + Godot. The workbench design
conversation here re-derived essentially the same shape — a useful
confirmation, but also a flag that we should be reading the guide
instead of rediscovering.

**Read these sections of the guide before implementing Stage 3:**

- **§5 Selection scope** — per-panel local + per-presentation-context
  propagated. Exactly the two-layer model the workbench conversation
  arrived at independently.
- **§5.3 Default propagation per action** — `navigate`/`select` →
  context, `submit`/`clear`/`set_filter`/`toggle_raw` → panel.
  Per-panel-instance overrides via factory-time content-type metadata.
- **§5.4 Selection schema** — `{path, paths?, peer_id?, content_type?, source_window?, updated_at}`.
  Our current `SaveSelection` writes `{path, has_entry}` — incomplete.
- **§6 Action wire shape** — `(window_id, event, value)` triple with
  canonical event vocabulary.
- **§8 Persistence interaction** — selection is ephemeral by default
  per the guide; we currently persist it always. Worth reconciling.

#### Where we already align with the guide

- `app/{app-id}/workspace/...` namespace pattern (with `app-id =
  "workbench"`); see `entitysdk.DefaultAppID`.
- Type names `app/state/window`, `app/state/selection`,
  `app/state/setting`.
- Bundled per-window state as CBOR map (one entity per window).
- Per-screen selection at `app/{app-id}/workspace/screens/{idx}/selection`.
- Model → output → renderer pattern (TEA-shaped); same `workbench/`
  models drive both raylib canvas and tview console renderers.
- Three render contexts (canvas/console/shell) as the §10 guide
  pattern names "CLI as render context."

#### Where we have gaps to close as part of Stage 3

These are the state-in-tree audit items plus the divergences from
the guide:

1. **Selection schema is incomplete.** Add `paths`, `peer_id`,
   `content_type`, `source_window`, `updated_at` to the `SaveSelection`
   payload to match guide §5.4. `peer_id` matters most — without it,
   multi-peer apps can't disambiguate which peer's tree a path refers
   to.
2. **No formal action-event vocabulary.** Add canonical events
   (`navigate`, `select`, `submit`, `clear`, `set_filter`,
   `toggle_raw`) with default-propagation metadata per guide §5.3.
   Our current `wb.Action` enum is workspace-management actions
   (`ActionSetContent`, `ActionSplitH`, etc.) — orthogonal to the
   guide's action vocabulary; both coexist.
3. **No per-panel propagation override.** Today each panel hard-codes
   `ws.refreshViewsForSelection(sel)`. Guide says this should come
   from content-type metadata. Mechanism: panels register a
   propagation-default-overrides map at construction time.
4. **`Shell.WD` is in-memory, not tree-resident.** Goes to
   `app/workbench/workspace/shells/{shell-id}/selection` (or just
   participates in the existing per-screen selection slot — TBD).
5. **Connection aliases in-memory.** Should persist; either at
   `app/workbench/workspace/shells/aliases/{alias}` or folded into
   `system/peer/transport/{peer-id}` as an `alias` field.
6. **Panel expand state (markdown-files, tree-browser) in-memory.**
   Goes to `app/workbench/workspace/windows/{id}/expand` (or as a
   field in the bundled window-state map).
7. **Window split layout** is recomputed from `DefaultScreens()` each
   session. Custom splits are lost; only active-screen-index
   persists.
8. **Markdown view editor drafts in-memory.** Independent bug: edits
   lost on panel reload. Separate small fix.
9. **Selection-as-ephemeral.** Guide §8 says selection is ephemeral
   by default; we currently persist always. Worth a decision —
   probably move to ephemeral and let apps opt in to persistence
   when meaningful.

#### Implementation order

The audit + guide together give a clear order. Items 1–4 are done;
items 5–7 are open and item 5 has a corrected design (see below — an
earlier design tangled the model; the correction is what should be
implemented next).

1. ✓ **Expand `SaveSelection` schema to guide §5.4.** The
   `Selection` struct carries `Path`, `Paths`, `PeerID`,
   `ContentType`, `SourceWindow`, `UpdatedAt`. Auto-fills UpdatedAt;
   tolerant read of legacy records. See `entitysdk/workspace_state.go`.
   - **Note:** `ContentType` + `SourceWindow` fields are
     load-bearing under the *original* design (shared per-context
     slot with multiple publishers identifying themselves). Under
     the corrected design below (one slot per publisher path),
     these fields become unused. They're harmless to keep; remove
     when convenient or leave as debug aids.
2. ✓ **Ephemeral-vs-persistent default for selection.** Resolved
   as a no-op for our single-peer-everything-in-one-tree shape:
   the peer's storage choice (`-storage memory` vs `-storage sqlite`)
   determines durability uniformly; selection follows. Revisit if
   multi-peer-per-app composition (guide §9.1) lands.
3. ✓ **Tree-resident shell WD + alias persistence.**
   `Shell.SetWD` fires `OnWDChanged`; shellcmd helpers
   `(*ShellWorkspace).PersistAliases(state)` and
   `PublishWDTo(state, activeScreen)` provide shared wiring across
   canvas, console, standalone shell. Connection aliases persist
   at `app/workbench/workspace/shells/aliases/{alias}`.
   - **Note under corrected design:** `PublishWDTo` currently
     writes into the per-screen aggregate slot
     (`app/workbench/.../screens/{i}/selection`). Under the
     corrected design, shell publishes belong in their own slot
     (per shell-panel window), not the aggregate. Implementation
     of item 5 will redirect.
4. ✓ **Action-event vocabulary.** Lives in
   `entitysdk/action_event.go`. Canonical names + default
   propagation. Application-agnostic; standing reference.
5. **OPEN — Reactive pub-sub for cross-panel coordination.** See
   "Corrected design" below.
6. Panel expand state in tree (audit item; closes the expand-state
   gap).
7. Window split layout in tree (audit item; closes the layout gap).

#### Design for item 5 — reactive pub-sub on selection state

**The model.** Each panel that has a selection has its own selection
slot in the tree. Each screen has an aggregate selection slot. By
default a panel publishes to *both* its own slot and its screen's
aggregate; by default a consuming panel watches its screen's
aggregate. Both ends are per-panel configurable: a panel can skip
publishing to the aggregate, and a panel can watch a specific other
panel's slot instead of the aggregate. Subscribers receive callbacks
via an SDK wrapper around `Store.Watch`.

There is no "propagation defaults" registry, no per-panel-instance
override metadata, no shared filter API. Wiring is just per-panel
configuration.

**Terminology.** Use "panel" throughout — it's the abstraction the
user works with. The renderer maintains a panel-id internally
(currently called `windowID` in code; rename as a separate cleanup
or as part of this work).

**Slot layout.**

| Slot | Writer | Default consumer |
|---|---|---|
| `app/workbench/workspace/panels/{panel_id}/selection` | the panel rendered here | nothing by default; subscribers opt in |
| `app/workbench/workspace/screens/{screen_idx}/selection` | every panel in the screen that publishes (default) | every consuming panel in the screen (default) |

Schema for both (type `app/state/selection`):

```
path        text    # the selected path
type        text    # type of the selected thing (today always "entity")
peer_id     text?   # which peer's tree the path lives in
updated_at  uint    # epoch ms; staleness signal
```

**On `type`.** Renamed from the current `content_type` field, which
was overloaded with panel content-type. The field names the *type
of the selected thing* — today every selection is an entity
selection. When other selection types are introduced (e.g. a query
result selection, an event-log row selection), this field
disambiguates and lets consumers like the inspector ignore selection
types they don't render.

**Per-panel-slot subscribers** never need to filter — the slot is
single-publisher single-type. **Aggregate-slot subscribers**
optionally filter on `type` once multiple types exist; until then
filtering code is unnecessary.

**SDK callback wrapper.**

```go
// In entitysdk/workspace_state.go:
func (ws *WorkspaceState) OnSelectionChange(
    path string,
    handler func(Selection),
) (cancel func())
```

Wraps `Store.Watch(path)`. Internally drains the channel on an SDK
goroutine, decodes the entity, calls `handler(sel)`. The cancel
func stops the goroutine and closes the watch. Panels mutex their
state (canvas) or wrap the handler body in `app.QueueUpdateDraw`
(console) — standard Go threading. The channel never leaves the
SDK; panels see callbacks only.

**Panel configuration.** Today panels are constructed in code with
fixed wiring (canvas/workspace.go, console/application.go). The
config surface lives at construction:

```go
type PanelConfig struct {
    PanelID       uint32
    SubscribesTo  string  // tree path; empty = screen aggregate
    PublishesToAggregate bool  // default true
}
```

Eventually this becomes tree-resident at
`app/workbench/workspace/panels/{panel_id}/config` so the layout
system can drive it. For the migration, constructor params are
enough.

**Migration order** (each step independently shippable):

1. **Add `OnSelectionChange` to `entitysdk.WorkspaceState`.** Wraps
   `Store.Watch` as a callback.
2. **Schema cleanup on `Selection`.** Rename `ContentType` → `Type`.
   Drop `SourceWindow` and `Paths` (vestigial — not in this
   model). Update existing publishers and tests.
3. **Per-panel publish.** Add panel-id to publish sites. Every
   publishing panel writes both slots (its own + the screen
   aggregate). Existing publishers: tree-browser, markdown-files,
   query-browser, shell-panel. Each writes `Type: "entity"` today.
4. **Migrate entity-detail (inspector).** Subscribe to the screen
   aggregate via `OnSelectionChange`. Handler updates a mutex'd
   `currentPath` (canvas) or queues a redraw with the new path
   (console). Drop the `wb.SelectionState` constructor param.
   Visible payoff: `cd` in the shell drives the inspector,
   tree-browser clicks drive the inspector — both via the same
   subscription.
5. **Migrate markdown-view.** Same shape.
6. **Remove `wb.SelectionState`.** Once subscribers are migrated
   and publishers write to slots directly, the in-memory state
   is dead. The `syncStateToTree` mirror logic in canvas/console
   also goes away — publishers write the slots themselves.
7. **Optional later: workspace-level aggregate.** A slot at
   `app/workbench/workspace/selection` carrying "the latest
   selection across all screens." Useful for cross-screen tools
   (status bar, "go to most recent" hotkey). Not in this stage.

**What we'd revise from the landed work.**

- `Selection.ContentType` → `Selection.Type` (step 2). Drop
  `SourceWindow`, `Paths`.
- `shellcmd.PublishWDTo` currently writes only the screen aggregate.
  Step 3 changes shell publishes to write both its panel slot and
  the aggregate.
- `syncStateToTree` mirror logic in canvas/console is removed once
  publishers write slots directly (step 6).

**Threading note.** SDK callbacks run on the SDK's goroutine.
Canvas panels use `sync.Mutex` around their callback-mutated
fields and the render-side reads. Console panels wrap handler
bodies in `app.QueueUpdateDraw` (tview's existing main-thread
marshal). Cancel funcs from `OnSelectionChange` are called on
panel teardown to stop the SDK goroutine.

**On guide alignment.** Implements guide §5.1 (per-panel slot)
and §5.2 (per-context aggregate) together. The guide's "default
propagation per action" metadata (§5.3) becomes uninteresting
because publishing-to-aggregate is just a per-panel config bool,
not an event-typed registry. Worth raising with the architecture
team as a simplification.

#### Two vocabularies, one render context

Designing Stage 3/4 surfaced a useful distinction the architecture guide
doesn't explicitly draw but probably should — captured here as a POC
finding to take back to the architecture team:

| Domain | What it is | Scope |
|---|---|---|
| **Action events** (guide §5.3, §6) | Pub-sub: "user attended to X", "user submitted." Wire shape `(window_id, event, value)`. Named vocabulary (`navigate`, `select`, `submit`, `clear`, `set_filter`, `toggle_raw`). | **Application-agnostic.** A `knowledge-base` app and a `calculator` app would use the same action names with the same propagation semantics. |
| **Commands** (this doc §5) | Imperative ops: `cd`, `ls`, `connect`, `mount`, `exec`, `revision config put`. Args + typed `Result`. Dispatched through `shellcmd.Registry`. | **Application-directed.** A KB app has different verbs than a workbench. Each app's CLI render context has its own command vocabulary. |

How they intersect:

- **A command may emit one or more actions as a side effect.** `cd /peerA/notes/` (command) produces a `navigate` action with `path=/peerA/notes/`. The command returns a `Result`; the action publishes to subscribers. Stage 3a already wired this: `Shell.SetWD` fires `OnWDChanged`, which writes the navigate into the per-context selection slot.
- **Not every command emits an action.** `mount`, `connect`, `revision config put` are pure configuration writes — their observable consequence is on the entity tree's event stream, not the action channel. Forcing 1:1 would over-design.
- **Not every action has a command counterpart.** `toggle_raw` (entity-detail) and `set_filter` (event-log) are panel-internal gestures. They *could* gain a verb (`panel entity-detail toggle-raw`) but the value is low until panels are shell-addressable.
- **The user can be a publisher from either side.** Per guide §10 ("CLI as render context"), typing `cd` in the shell and pressing arrow keys in tree-browser are equivalent inputs to the same `navigate` event surface.

Implication for the action-vocabulary work in this plan:

- Action event names belong in `entitysdk/` (application-agnostic, cross-app constants).
- Commands stay in `shellcmd/` (workbench-specific).
- A panel content-type declares its action surface (which events it produces, which it consumes) at registration time — content-types are the cross-app contract.
- Per-panel propagation overrides (Stage 4 item 5 above) are how a panel says "I subscribe to *tree-browser*'s `select`, not the *shell*'s `navigate` — even though both default to context-level propagation."

The architecture guide §5 + §10 treat action vocabulary as canonical
across render contexts but don't explicitly contrast it with the
per-app command vocabulary. Once Stage 3/4 land, a refinement note
back to the architecture team would close that loop.

#### What this guide does NOT bind

The guide §11 calls out what's deferred: T3 outputs, cross-peer
rendering latency regimes, compute-backed UIs, multi-app capability
boundary. None of those block Stage 3. Multi-peer-per-app
composition (§9.1) is permitted by the convention but we currently
run one peer with everything in one tree — that's a worthwhile
future structure but not a prerequisite.

### 8.5 Panel data subscriptions — subscribe by prefix, never scan-and-filter

**Anti-pattern (since removed):** the workbench previously
maintained a shared `PeerContext` cache that listed the entire tree
into a slice on every refresh tick; every panel filtered that slice
for its own prefix. With a populated store (14K paths in the field)
each coalesced refresh blocked the renderer main goroutine for ~22ms.
Heartbeats and other background writes kept the UI in refresh.

**Correct shape — what every new panel model must do:**

1. **Subscribe to your prefix.** Panel models call `store.OnPrefixChange(prefix, handler)`
   at construction. The SDK delivers a `ChangePut` for every existing
   path under the prefix (seed phase) and a live `ChangePut` /
   `ChangeRemove` for every subsequent mutation. The handler is the
   panel's exclusive read path for matching events.
2. **Maintain your own local view-state.** Inside the handler, update
   a per-panel map / tree / slice — whatever shape the panel needs to
   render. Never read shared cache state. Never re-enumerate the
   global tree.
3. **Treat events as "current state of path = X", not deltas.** The
   seed phase + live drain can deliver the same path's ChangePut twice
   (once from seed, once from a buffered live event). Handlers must
   be idempotent.
4. **Off-thread.** The handler runs on an SDK-owned goroutine. Marshal
   to your renderer's UI thread inside the handler if you need to
   touch render state directly (tview's `QueueUpdateDraw`, raylib's
   wake-render-loop pattern, etc).
5. **Cancel on close.** `OnPrefixChange` returns a cancel func; the
   panel model's `Close()` invokes it. Idempotent.

**Reference implementation:** `entitysdk/workspace_state.go::OnSelectionChange`
is the prototype (per-path); `entitysdk/store.go::OnPrefixChange` is
the generalization (per-prefix). Both follow the same lifecycle
shape. Migrated panel models (template: `workbench/markdown_files_model.go`)
show the consumer side.

**When a panel legitimately needs "show everything":** tree-browser
and peer-info display the whole peer tree by design. They call
`store.List("")` on each render directly — no cache. The cost is
bounded by the renderer's refresh tick rate (driven by `TreeEvents`),
not by tree event rate. Long-term, both panels should migrate to
incremental local model + `OnPrefixChange("/")` (or similar
"watch this peer's whole tree" primitive — not built yet).

**What was deleted with the cache:**
- `PeerContext.Entries / MarkDirty / RefreshIfDirty` — gone.
- `DataContext.Entries / Resolve / EntityCount / MarkDirty` interface
  methods — gone (only `Selection()` remains).
- The 100ms refresh-throttle band-aid in `console/main.go` — gone.

**If you write a new panel and find yourself reaching for "give me
every path then I'll filter":** stop. That's the anti-pattern. Add a
prefix subscription instead. The feedback memo's "Migration guidance"
section enumerates the steps.

---

## 9. State and persistence

### 9.1 Persistent local peer

The local peer is no longer ephemeral by default-capability. Pair
`-storage sqlite -storage-path PATH` with `-identity NAME` and the
peer survives close+reopen with tree, revisions, history, and
identity state all intact:

```
entity-shell -identity peerA \
             -storage sqlite \
             -storage-path ~/.entity/peers/peerA/peer.db
```

Without those flags, the local peer is in-memory and per-invocation,
which is still the right default for shell experiments and one-shot
dispatches. Full walkthrough in `USAGE-SHELL.md §1.3`; operational
posture (what to back up, breaking-change recovery) in
`DEPLOYMENT-DIRECTION.md`.

The Phase D landing also surfaced two findings worth tracking:

- **Identity-aware peers leak ~4 entities per re-bootstrap.** Bounded
  per restart but linear over the peer's lifetime. Tracked as
  roadmap item 5a-followup; see `DEPLOYMENT-DIRECTION.md §7`.
- **core-go's `SqliteStore.init()` rewrites `PRAGMA user_version`
  unconditionally.** Forward-compat hazard. The workbench has a
  pre-flight version check (`storage_sqlite_check.go`) that refuses
  to open a DB written by a future binary; core-go's init still needs
  a fix to not clobber future versions. Cross-team item.

### 9.2 Still-future — shell session state

Persistent peer state landed. **Shell session state** has not. The
following are still future direction:

- `~/.entity/shell/aliases.toml` — alias → host:port mappings
  surviving across invocations (today: rebuilt per launch).
- `~/.entity/shell/history` — REPL command history.
- `~/.entity/shell/connections.toml` — last-known peer connections,
  with optional auto-reconnect on startup.

For a long-running deployed peer, the connection-state question
(`system/peer/transport/{peer-id}` entries today rebuilt per launch)
is operationally relevant — tracked in `DEPLOYMENT-DIRECTION.md §7`.

### 9.3 Cross-app shared peers — still open

The persistent shell peer can be the same kind of peer the canvas /
console use. In principle they could share a backing store and open
cross-app coordination (one peer, multiple front-ends). The
concurrent-open question (two processes, same SQLite file) is
flagged but undecided — see `DEPLOYMENT-DIRECTION.md §7`.

---

## 10. Implementation plan — phased

> **Note:** the phases below are the *original* shell-direction phasing.
> They've been superseded by the repository-workspace track, which is the
> canonical "what's next" plan today. The sub-sections below stay as a
> historical record of how the shell got to its current shape.

### 10.1 Phase 1 — completed (this session)

Standalone `entity-shell` binary. 9-command core. REPL + one-shot
+ JSON. Local-peer-only. Module skeleton (`shellcmd/` + `shell/`).

### 10.2 Phase 2 — AppPeer remote dispatch + (optional) Client primitive

Two parallel tracks. Track A is the load-bearing one for the REPL.
Track B unlocks one-shot mode and fan-out cleanly; do it when those
become priorities.

#### Track A — AppPeer mode remote dispatch — **landed**

All three surgical changes shipped:

1. **`entitysdk.Executor` URI-aware dispatch** (executor.go) —
   `RemoteExecuteFunc`, `SetRemoteExecute`, `isRemote(uri)` check;
   local-qualified URIs are normalized via `entity.ExtractHandlerPath`
   before hitting the registry's prefix matcher.
2. **`entitysdk.AppPeer.Connect`** (connection.go) — calls
   `peer.RegisterRemote(remotePeerID, addr)` after handshake.
3. **`entitysdk.AppPeer.{Get,Put,Has,Remove,List}`** (tree_ops.go) —
   `resolveDispatchTarget` accepts bare paths, peer-qualified
   `/{peerID}/...` paths, and full `entity://` URIs; routing decision
   propagates to the executor's URI-aware branch.
4. **`shellcmd`** — `cmd_connect.go` real implementation;
   `cmd_nav.go` / `cmd_entity.go` drop the `pc.Address != ""` stubs
   and pass peer-qualified paths through to the SDK.

Tests in place:
- `entitysdk.TestExecutor_RemoteDispatch` — low-level executor
  routing two in-process peers.
- `entitysdk.TestAppPeer_RemoteGetPutList` — high-level surface
  Put → Has-on-server-store → Get → List.
- `entitysdk.TestExecutor_NoRemoteWiringRejects` — clean error if
  unwired.
- `shellcmd.TestShell_RemoteConnectAndDispatch` — end-to-end
  `connect → cd → ls → cat → disconnect` against an in-process
  remote peer.

**Track A follow-ups — closed:**

- **Dual-dial on connect — fixed.** Added
  `peer.Peer.AddRemoteConnection(peerID, conn)` in core-go
  (core/peer/remote.go) and a dedup pass in `peer.Peer.Connections`
  so the same conn isn't double-counted between the flat list and
  the pool. `entitysdk.AppPeer.Connect` now caches the
  freshly-handshaked connection in the pool, so subsequent
  dispatched ops reuse it — no second TCP dial.
- **Disconnect evicts the pool — fixed.** Added
  `entitysdk.AppPeer.Disconnect(peerID)` wrapping
  `peer.RemoveRemote`; `cmdDisconnect` calls it before dropping
  the alias.
- **`alias:path` syntax — supported.** New `Shell.Resolve(input)`
  method in `shellcmd/shell.go` interprets `alias:` and
  `alias:foo/bar` against the shell's connection table; existing
  command sites updated to use it. Aliases that don't match a
  connection fall through to the package-level `Resolve` so a typo
  surfaces as "no connection for path …" downstream.

  > **Migration to the `@alias` substitution sigil is owed.** The
  > shell-framing convention pins `@alias` as a peer-id substitution
  > sigil composing with canonical
  > `/{peer_id}/path` and `entity://{peer_id}/path`. `:` is reserved
  > for the protocol's universal `<handler-path>:<op>` op-naming
  > convention. The `alias:path` form shipped here is being migrated
  > to `@alias` with a one-release deprecation window (both forms
  > accepted; `alias:` emits a deprecation warning). Tracked as
  > follow-through; not blocking.

Coverage: `peer.TestAddRemoteConnection_Inserts` /
`TestAddRemoteConnection_RejectsUnestablished` (core-go);
`shellcmd.TestShell_Phase3Commands` exercises alias-path syntax,
disconnect's pool eviction is naturally validated by re-connect
working.

#### Track B — Client primitive in entitysdk

Add a single-connection bare-client surface (§4.5):

```go
// entitysdk/client.go (sketch)
type Client struct { /* keypair, conn, session */ }

func NewClient(addr string, opts ...ClientOption) (*Client, error)
func (c *Client) Connect(ctx context.Context) error  // handshake
func (c *Client) Execute(ctx context.Context, uri, op string,
    params entity.Entity, resource *types.ResourceTarget) (*Response, error)
func (c *Client) Close() error
```

Wraps `peer.Connection` (`core/peer/connection.go:277` already has
`Execute`). Brings its own keypair via `ClientOption` (or
auto-generates an ephemeral one). No pool, no dispatcher, no `peer.Peer`.

Used by the shell's one-shot mode (lower spinup cost) and any
future fan-out command. Callers must obey the routing rule (§4.5.1).

**Estimated: half day SDK work + tests. Independent of Track A.**

The architecture team may eventually want this formalized as an SDK
pattern (sits naturally alongside SDK-OPERATIONS §7-§8); flag for
proposal once we have concrete usage.

### 10.3 Phase 3 — basic put/cp/rm/has commands — **landed**

Added in `shellcmd/cmd_tree.go`:

- `put <path> <type> <json-data>` — JSON-decoded data through
  `AppPeer.Put`; literal-string fallback if the data isn't valid JSON.
- `rm <path>` — `AppPeer.Remove`.
- `has <path>` — `AppPeer.Has`; reports yes/no via a Result message.
- `cp <src> <dst>` — `AppPeer.Get(src)` + new `AppPeer.PutEntity(dst, ent)`
  (preserves content hash). Cross-peer-capable because both endpoints
  dispatch through the local AppPeer's pool.

All four accept the alias-path syntax (`put serv:demo/value …`,
`cp serv:foo local:bar`). Coverage: `shellcmd.TestShell_Phase3Commands`.

### 10.4 Phase 4 — identity scaffolding

`identity create / list / use` + `~/.entity/identities/` integration
(consume what entity-core-go already provides for keypair files).
Wire the `--identity` flag end-to-end.

**Estimated: half day. Coordinates with whatever the architecture
team eventually says about identity-management surface.**

### 10.5 Phase 5 — peer / capability commands (sketches → real)

`peer spawn / list / kill`, `cap grant / list / revoke`. Each
depends on SDK support that may not yet exist; those commands ship
as stubs documenting the intended behavior.

### 10.6 Phase 6 — fan-out + piping

Both need formal proposals (§11). Phase-6 work follows their
resolution. Reference implementations possibly precede the proposal
to feed it with concrete experience.

### 10.7 Cross-cutting: panel-shell refactor

After canvas/console migrate to `entitysdk.AppPeer`,
`workbench/shell_model.go` becomes a thin adapter. Tracked
separately; not blocking.

---

## 11. Open questions for architecture-team coordination

These are decisions that extend beyond the workbench-go boundary —
they affect cross-impl alignment (Rust egui, Godot) and/or core
protocol semantics. Each is a candidate for a formal proposal once
local design is stable.

**Readiness state (after Phase 3 + Track A follow-ups):**

| § | Topic | Readiness |
|---|---|---|
| 11.3 | Command-naming convergence | **Ready to formalize.** We've shipped `put`/`rm`/`cat`/`has`/`cp` matching the v5.0 proposal; can write the convergence proposal now. |
| 11.5 | Persistent shell peer shape | **Ready to formalize.** AppPeer's configurable axes (§4.6) clarify the shape; proposal can pin down `~/.entity/peers/shell/` vs `~/.entity/peers/{name}/` naming. |
| 11.7 | Identity / capability vocabulary | **Sharpen during Phase 4.** §5.3 sketches the verbs; concrete shape comes when `--identity` and key-loading land. |
| 11.1 | Pipe semantics + continuation translation | **Defer.** Benefits from concrete shell experience before formalizing. |
| 11.2 | Fan-out / aggregation | **Defer.** Same — needs experience with multi-peer commands. |
| 11.4 | Prefixed vs unprefixed | **Defer to packaging.** Not blocking. |
| 11.6 | Compute / programming command depth | **Long-term.** Wait for entity-computation extension to stabilize. |
| — | New: pool-insertion API (`peer.Peer.AddRemoteConnection`) | **Already shipped in core-go.** Worth flagging to other-language SDK teams (Rust, Godot, Python) so they expose a parallel surface. |
| — | New: SDK shape for AppPeer-vs-Client (§4.5) | **Worth surfacing when Track B lands.** Two-surface model with identity-routing rule (§4.5.1) is novel; cross-impl alignment would benefit from a proposal once we have concrete Client usage. |

### 11.1 Pipe semantics and continuation translation

Should shell pipes have a defined translation to continuation
graphs? Or are they a separate CLI-level mechanism with no protocol
implications? §6 gives the open-question list.

**Coordination needed with:** core protocol (continuations spec),
all CLI implementations.

### 11.2 Fan-out / aggregation syntax and semantics

§6.3 — no prior art across the architecture. Needs a focused
proposal.

**Coordination needed with:** SDK (multi-peer dispatch), all CLI
implementations.

### 11.3 Command-naming convergence

Cross-impl analysis showed disagreements:

- Write: Go/Rust `put`, Python `write`.
- Delete: Python `rm`, others implicit.
- Browse: Go/Python `cat`, Rust no client.

**Recommendation:** stabilize on `put`, `rm`, `cat` per the v5.0
proposal and CLI analysis. Coordinate to harmonize the other
impls.

### 11.4 Prefixed vs unprefixed commands

Should the binary install `dls`, `dcat`, etc. as one-shot aliases?
Packaging decision; deferred until distribution work.

### 11.5 Persistent shell peer shape

Where does `~/.entity/shell/` sit relative to `~/.entity/peers/`?
Does the shell's persistent peer share state with the workbench's
persistent peer? §9.2-9.3.

**Coordination needed with:** persistence guide, peer-namespace
convention.

### 11.6 Compute / programming command depth

`eval`, `deps`, `purity` — these touch the entity-computation VM
described in `ENTITY-COMPUTATION-PROGRAMMING.md`. We don't yet know
how deeply the shell engages with that layer.

**Recommendation:** defer until compute extension stabilizes;
revisit when entity-computation has a usable surface.

### 11.7 Identity / capability command vocabulary

§5.3 sketches `identity create / list / use`, `cap grant / list /
revoke`. These need SDK and protocol support; the verbs we choose
will travel.

**Coordination needed with:** identity spec, capability spec.

---

## 12. Action items (this team)

- [ ] **Path-aware routing in entitysdk** — Phase 2, §10.2. The
      single highest-leverage piece. Half-day SDK + half-day
      shell wiring.
- [ ] **Basic put/rm/cp/has commands** — Phase 3, §10.3. Trivial.
- [ ] **Identity scaffolding** — Phase 4, §10.4. Wires the
      `--identity` flag end-to-end.
- [ ] **Promote sections of this doc to formal proposals** as the
      open questions in §11 mature. Likely candidates first:
      pipe semantics, fan-out, command-naming.
- [ ] **Panel-shell refactor** — once canvas/console use
      `entitysdk.CreatePeer`. §8.

---

## 13. References

**Prior art** (informing the design, in the broader ecosystem):
- The CLI entity-tree-tools proposal — DUtils command set, prefix
  convention, multi-peer navigation, piping sketches.
- The cross-impl CLI analysis — three-impl convergence on the 9-command core.
- The continuation-model exploration — the continuation primitive
  (distinct from shell pipes).
- The entity-workbench-app and persistence guides — application-layer
  convention (shell as another presentation backend) and the `~/.entity/`
  per-peer persistence layout.
- `entity-core-go/cmd/entity-shell/` — the Go shell we ported from
  (sibling repo).
- `entity-core-go/cmd/entity-sync/` — production cross-peer continuation
  example (sibling repo).
- The entity-programming layered model (Layer 1-4).

**This repo:**
- `shellcmd/` — command vocabulary.
- `shell/` — standalone binary.
- `docs/architecture/USAGE-SHELL.md` — user-facing usage and
  scenarios for the shell.
- `docs/architecture/ARCHITECTURE.md` — workbench architecture.

---

## 14. Memory / status notes

- Current implementation status: Phase 1, Phase 2 Track A, and
  Phase 3 all complete (§10.1-§10.3). Track A follow-ups all
  closed (§10.2). REPL has working
  `connect/disconnect/cd/pwd/ls/cat/tree/exec/put/rm/has/cp/info/help`
  with full alias-path syntax, end-to-end against remote peers.
- Next candidates: Phase 2 Track B (`Client` primitive — only when
  one-shot mode performance or fan-out becomes a priority); Phase 4
  identity scaffolding; refining the panel-shell adapter
  (§8/§10.7); promoting design sections to formal proposals as
  they mature (§11).
- All cross-team coordination items are §11; raise to architecture
  team when each design has matured locally.
- Shell-first development principle (§1.2) is now load-bearing for
  feature priority — record in workbench memory.

### 14.1 Decisions log

- **Every AppPeer has a tree (§4.5, §4.6).** Earlier
  framing of a "minimal AppPeer with no tree handler" was wrong:
  every `peer.Peer` in core-go has a tree (store + location index)
  seeded at startup with `system/*` entities, and `system/tree` is
  registered unconditionally (peer.go:138). The honest distinction
  is binary — `Client` (not a peer; TCP wrapper) vs `AppPeer` (peer,
  always with tree). AppPeer's real configurable axes are identity
  source, listener, storage, and extensions; "tree-less peer" was
  fiction.
- **Phase 2 reframed.** The original §4.4 sketch
  (`splitPeerPath`, `listOverConnection`, etc.) overstated the work.
  Cross-peer routing already exists in `core/peer/remote.go` and the
  dispatcher's `RemoteExecute` callback. The actual gap is
  `entitysdk.Executor` calling `Registry.Dispatch` directly,
  bypassing the dispatcher's remote path (§4.4 revised).
- **Multi-identity = multi-AppPeer, deferred.** Settled
  on "one AppPeer per active identity" rather than key-juggling on
  a single peer (§4.8); v1 stays single-identity, v2 picks this up
  once `--identity` is exercised.
- **Capability is enforced remotely.** Shell signs and
  propagates only; remote peers validate grants at handshake and
  per-EXECUTE (§4.7). No policy logic in the shell.
- **Two SDK surfaces, identity-routed.** AppPeer and
  Client are distinct surfaces, not modes of the same thing
  (§4.5). The keypair you operate under picks the surface: same
  keypair as a live AppPeer → route through the AppPeer; different
  keypair → use a Client. Avoids duplicate TCP connections to the
  same remote peer from the same logical entity. Connection sharing
  across surfaces deferred to v2 (§4.5.3). Architecture team may
  formalize as an SDK pattern once concrete usage exists.
- **Track A landed.** Phase 2 Track A (AppPeer mode
  remote dispatch + shell wiring) is in. Shell's `connect`/`cd`/
  `ls`/`cat`/`tree`/`exec`/`disconnect` work against remote peers
  end-to-end.
- **Phase 3 landed.** `put`/`rm`/`has`/`cp` shell
  commands shipped (§10.3). New `entitysdk.AppPeer.PutEntity`
  preserves an entity verbatim (content-hash stable) so `cp` works
  cross-peer.
- **Track A follow-ups closed.** (1) Dual-dial fixed
  via new core-go `peer.Peer.AddRemoteConnection` + dedup in
  `Connections()`; AppPeer.Connect caches in the pool. (2)
  `entitysdk.AppPeer.Disconnect(peerID)` shim added; cmdDisconnect
  evicts the pool. (3) `Shell.Resolve(input)` supports
  `alias:path` and `alias:` syntax; commands updated to use it.
  Both core-go test and workbench tests cover the changes.
- **REPL ergonomics + path-resolution fixes.** Added
  `peterh/liner` for line editing (Home/End/arrows/history/tab
  completion); persistent history at `~/.entity/shell/history`.
  Falls back to a line scanner when stdin isn't a TTY. Fixed
  `Shell.Resolve` so absolute paths with an alias as the first
  segment (`/local/foo`, `/serv/system/handler`) expand to the
  actual peer-id — previously they were treated as literal peer-ids
  and produced "no connection" errors. Also rewrote the help
  command's path-navigation section to reflect actual path forms
  (the prior text falsely advertised peer-less absolute paths).
- **Tab-completion walks the tree on demand.** New
  `shell/complete.go` provides context-aware completion: command
  verbs at the start of a line; path arguments via live
  `AppPeer.List` against whichever directory the user is mid-typing
  into. One dispatched listing per TAB (free for local peers, single
  round-trip for remote). No tree caching or background fetching —
  the SDK's existing per-level listing is what makes this trivial.
  Handles alias prefixes (`local:scratch/`), absolute alias forms
  (`/local/scratch/`), relative paths after `cd alias:`, and
  shell-root alias suggestions. Tested in `shell/complete_test.go`.
- **Shell-framing guide absorbed cross-impl.**
  `GUIDE-SHELL-FRAMING.md` (the normative cross-impl artifact) was
  absorbed alongside the egui and Godot reviews. Three
  workbench-relevant outcomes:
  1. **Path syntax → `@alias` substitution sigil** (§3.4). `@alias`
     composes with canonical `/{peer_id}/path` and
     `entity://{peer_id}/path`; `:` is reserved for the protocol's
     `<handler-path>:<op>` op-naming convention. Workbench-go's
     `alias:path` form is migrating with a one-release deprecation
     window.
  2. **Verb-op seam now normative** (§8.1 four-layer factoring).
     The seam between verb-parsers (thin CLI string → call op →
     project Result) and verb-ops (typed callable) is a MUST-factor
     pin. Workbench-go's `InstallRevisionFollowChain` (in
     `shellcmd/cmd_revision_follow.go`) is the canonical example.
     Most other verbs treat the SDK as the op layer (acceptable);
     multi-step verbs are opportunistic extraction targets.
  3. **Tier C is now ten-core** — `help` added.
