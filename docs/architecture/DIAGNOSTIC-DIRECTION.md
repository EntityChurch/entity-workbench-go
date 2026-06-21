# Diagnostic direction — observe the entities, don't theorize about them

**Status:** Direction memo, living. Updated as the inspect facility grows.
**Origin:** Originated in a session with five attribution errors in one day from theorizing about code paths instead of observing the actual entities. Built the dispatch tap pattern (`perfreview/crossimpl_merge_tap_test.go`), found F-CIMP-7's actual root cause in one run, then productized as `perfreview/inspect.go`. The user's framing: *the whole point of an entity system is that everything is observable; the tools should make that ergonomic*.

---

## §0 The core architectural truth

Every dispatch, every continuation step, every chain-error marker, every wire frame — these are all entities, content-addressed, deterministic, observable. The system is **designed for inspection**. What's been missing is the ergonomic tooling to surface those entities to a debugging engineer.

Workbench-go is the right home for that tooling. Not entitysdk (which is the application interface), not core-go (which is the substrate), not the impls (which are conformance peers). The diagnostic layer lives above the SDK and below the application — a `inspect` package usable by tests, shell commands, and panels alike.

**What inspect IS NOT (per GUIDE-INSPECTABILITY v1.1 §7.1).** Inspect is *symptom localization*, not *root-cause analysis*. The path-tap that found F-CIMP-7 surfaced "merge inbox is receiving 19/20 status=400 base_not_a_version" — that's the symptom at the entity-system boundary. The actual root cause (Python's fetch-diff doesn't unwrap `system/hash` notification-form for the `base` field) was found by *reading Python's source* with the symptom as the navigation aid. Inspect made source-reading focused instead of speculative. **Inspect shortens the path to the right handler. It does not replace handler-body work or classical debugging.** A bug in a Rust extension's internal logic still needs gdb / lldb / source-reading to fix; inspect just makes "which handler" cheap so the debugger gets pointed at the right code on the first try.

This is the boundary discipline v1.1 codifies. Workbench-go's inspect surface delivers symptom localization at the entity-system frame; classical debuggers continue to own handler internals.

---

## §1 The primitives (post-v1.1)

Every diagnostic capability decomposes into combinations of these primitives. Build them; combine them; surface them through commands and panels.

This section reflects core-go having shipped the five v1.1 hook surfaces. Each primitive below names the consumer + the core-go hook it rides on + status.

### §1.1 Dispatch tap (built — `inspect.DispatchTap`)

Parallel observation of dispatcher↔handler events via `peer.WithDispatchHook` (core-go builder.go:215). Fires twice per dispatch (entry + exit) at the `invoke` closure in `core/protocol/execute.go`. Exit phase carries `ResponseStatus` + `ResponseHash` — the response_hash is load-bearing per the v1.1 review §7.1 invariant.

Construct with `inspect.NewDispatchTap(pathFilter)`; install via `tap.PeerOption()` at peer construction. `Exchanges()` correlates entry+exit by request_id. `CountByStatus()` histograms the exit-phase response statuses — the quick "did this chain converge?" check.

**Safe on production handler paths.** Replaces the legacy `inspect.InstallTap` (tap-as-handler shape, terminal observation). The legacy `Tap` is still present as a synthetic chain sink in tests where the chain is incomplete; for production observation, use DispatchTap.

Use cases:
- "Did every fetch-step succeed?" — `CountByStatus()` on a tap filtered to follow inbox paths
- Cross-impl probe: same workload, two Go-vs-Python runs, diff the status histograms
- Live dispatch latency telemetry by computing exit.Timestamp − entry.Timestamp

### §1.2 Content stream (built — `inspect.ContentStream`)

Parallel observation of every new-entity Put via `peer.WithNamedContentHook` (core-go builder.go:159). `ContentStoreEvent.IsNew bool` surfaces the invariant the call site already enforces (always true today; documents the contract per v1.1 §2.1 #1).

Filter by `Entity.Type` substring for focused traces. `CountByType()` histograms types seen; `EntitiesOfType(t)` returns hashes for chained lookup.

Use cases:
- Histogram of all entity types stored during a run (canonical F-CIMP-7 run: 117 run-window, 20 inbox deliveries, 20 notifications, etc.)
- Find all `system/protocol/error` entities for a debug session
- Find all `system/continuation/*` entities for a chain execution
- Audit: who wrote this entity? (cross-reference timestamp + content)

Limitation: hook is registered at peer construction; can't be added at runtime. Acceptable for debug-mode peers and standing-pin probes.

### §1.3 Binding stream (built — `inspect.BindingStream`)

Parallel observation of path/binding mutations via `peer.WithBindingHook` (core-go builder.go:172 — the observe-only alias for `WithNamedSyncHook`). Maps to v1.1 §2.1 #2: `(Path, PeerID, Hash, PreviousHash, ChangeType ∈ {Created, Modified, Deleted}, CascadeDepth)`.

Mirror of `ContentStream` for path-side observation. `CountByChangeType()` histograms create/modify/delete; substrings filter on path.

The dual primitive to content stream — content events fire on entity Put; binding events fire on location-index Set/Remove. v1.1 §2.4 calls them out as composing into the "binding stream" derived capability.

### §1.4 Wire recorder (built — `inspect.WireRecorder`)

Captures every inbound/outbound envelope as raw CBOR bytes via `peer.WithWireHook` (core-go builder.go:194 + `core/peer/wire_event.go`). Fires post-decode for inbound (so `RequestID` + `RootType` are populated) and post-write for outbound.

**Per the v1.1 security addendum §2:** FrameBytes carries the full wire envelope including capability tokens, signatures, identity entities. Recordings are sensitive artifacts. The recorder copies bytes at capture per the core-go hook contract (the underlying slice is owned by the I/O codepath). In-memory only — persisting to disk requires operator policy on the sink.

`FramesByRequest(id)` enables cross-peer correlation (same request_id observed on two peers' recorders). `CountByRootType()` histograms envelope shapes. `SetRootTypeFilter()` narrows to a specific envelope family.

Use cases:
- Cross-impl wire-shape diff: same operation against Go vs Python, byte-diff the encoded frames
- Replay (forward direction): re-dispatch a captured frame to reproduce a failure
- Audit trail: which peer sent which envelope when

### §1.5 Subscription tracer (built — `inspect.SubscriptionTracer`)

Emit + deliver as TWO distinct event classes per v1.1 §2.1 split — the conflation that hid F-CIMP-2 last cycle. Attached post-construction via `peer.SubscriptionEngine()` (new public accessor on AppPeer), then `tracer.Attach(engine)`.

Lives on the engine, not on the core peer builder, because the core/ext dependency DAG forbids core importing ext (per core-go CLAUDE.md). API: `Engine.AddEmitHook(name, fn)` + `Engine.AddDeliverHook(name, fn)` in `ext/subscription/inspect_hooks.go`.

Emit fires at notification-construction (engine.go OnTreeChange site). Deliver fires at the deliver loop wrapping `e.Deliver`. Status code surfaces on the deliver event — F-CIMP-2's distinguishing histogram row that v1.0's conflated formulation couldn't surface.

Use cases:
- "Did emit fire but deliver fail?" — distinguishes matcher-side from delivery-side failures
- F-CIMP-style cross-impl bug surfacing: compare emit:deliver ratio across impls
- Backpressure debugging: histogram of delivery statuses including 429/503

### §1.6 Entity browser (built — `inspect.DumpEntityAt`/`DumpEntityByHash`)

Given a path or hash, return the entity decoded as a human-readable structure. CBOR maps render as nested key-value trees; byte slices hex-encode with truncation. Zero hook plumbing required — pure substrate read.

Use cases:
- "What is at this path?" — `DumpEntityAt(peer, path)`
- "What does this hash hold?" — `DumpEntityByHash(peer, hash)`
- After finding a chain-error marker, dump the marker entity to read its body

Limitation: byte-keyed maps (CBOR maps with `hash.Hash` keys) render keys as garbled UTF-8. Need a special-case renderer for `map[hash.Hash]Entity` (envelope `Included` shape).

### §1.7 Location browse (built — `inspect.FindUnder`/`FindChainErrors`)

Enumerate path bindings matching a substring. Zero hook plumbing required — pure substrate read.

Use cases:
- "Did any chain-error markers get bound?" — `FindChainErrors(peer)`
- "What's under follow-errors/?" — `FindUnder(peer, "follow-errors")`

---

## §2 Composed capabilities

Built on the primitives above:

### §2.1 Chain trace (built — `inspect.TraceChain`)

Given a `chain_id`, walk:
- Subscription notification entity (start of chain)
- Continuation entities at each step
- Each step's dispatched EXECUTE + response
- Chain-error markers if any
- Final result entity

Returns `ChainTrace{ChainID, Errors, Continuations, PathBindings, ContentEvents}` — chain-error-lost markers decoded with v1.20 path-scheme breakdown, continuation entries, path bindings whose path contains the chain_id, optional content-stream events whose decoded body references chain_id.

Honesty caveat per v1.1 §9 #8 (chain participation invariants): without extensions declaring their completion contracts, `ChainTrace` cannot distinguish silent failure from "the chain never ran." Closing this requires per-extension completion-contract declarations, routed to arch.

### §2.2 Replay (NOT YET BUILT — design)

Take a stored dispatch entity (from a wire recorder, a tap, or a hand-constructed shape) and re-dispatch it against a target peer. Useful for:
- Reproducing a cross-impl wire-shape bug without re-running the full scenario
- Smoke-testing a fix: replay the failing dispatch, confirm new code accepts/rejects correctly
- Property tests: generate a corpus of dispatches, replay against multiple impls, diff responses

### §2.3 Time-travel (NOT YET BUILT — design)

The content-stream + tree-event-sink combination already captures every state change in entity form. With a recorder that timestamps + persists, we get a complete event log. From that:
- Step backwards through state — what did the tree look like 5 seconds ago?
- Reconstruct a peer's state at a past time
- Diff state between two timestamps (what changed in the last 30 sec?)

This is a workbench-go feature, not a substrate feature — the substrate is content-addressed and idempotent, the recorder is the diagnostic overlay.

### §2.4 Stepping debugger (NOT YET BUILT — exploratory)

For interactive debug: pause a peer at a handler boundary, inspect state, step to next dispatch, continue. Possible because every dispatch is a discrete entity boundary. Requires:
- A pause mechanism (hook returns a "wait for resume" signal)
- An interactive control surface (shell command? panel?)
- State snapshotting between pauses

Far future; sketch for when interactive use cases drive it.

---

## §3 Surface paths

How the primitives reach the engineer:

### §3.1 Test-time (built)

Direct Go-API usage in `_test.go` files. Pattern: probe with explicit setup, capture during execution, inspect+assert at end. This pattern *localized the symptom* in F-CIMP-7 (merge inbox receiving 19/20 status=400 base_not_a_version errors) — Python's actual root cause was then found by source-reading the fetch-diff base-unwrap path with the symptom as the navigation aid. **Inspect shortened the path; source-reading found the bug.** Per v1.1 §7.1 boundary discipline.

### §3.2 Shell commands (DESIGN — Phase G UI thread)

Surface inspect primitives through `entity-shell`:

```
entity-shell inspect tap <path>         # install a tap, print captures live
entity-shell inspect content [-type T]  # stream content-store events live
entity-shell inspect entity <path|@hash>  # decode + dump a single entity
entity-shell inspect chain <chain-id>   # walk a chain's full execution
entity-shell inspect errors             # enumerate chain-error markers
```

Builds on existing `cat`, `tree`, `find` commands. Lives in the same shellcmd package.

### §3.3 Panel surfaces (DESIGN — Phase G UI thread)

Workbench-go GUI panels for live observation:

- **Path tap panel** — interactive form: install a tap at any path, watch captures stream in
- **Content browser panel** — paginated list of all entities by type, drill into details
- **Chain trace panel** — given a chain_id, render the trace tree
- **Wire recorder panel** — show inbound/outbound frames in time order, drill into hex/decoded

Per the user's note: "you could do it in ways where you didn't really directly to the SDK" — these panels can live in their own package, not in entitysdk.

---

## §4 Standing pin test direction

The cross-impl probes that found F-CIMP-7 are currently `//go:build perfreview` — hand-run, not standing CI. Today's discovery was that v4.2 HAMT landed without a cross-impl chain probe running, so we found the bug as a "stress probe a session later" rather than at v4.2 sign-off.

Direction: promote cross-impl probes to a `crossimpl` build tag, run periodically (CI or scheduled). Workbench-go's role: maintain the conformance matrix; the probes are the conformance tests.

Items in scope for the matrix:
- `TestCrossImpl_SmokeWireAndSubscription` (wire + single-trigger sub delivery)
- `TestCrossImpl_RevisionFollowChain` (the canonical chain)
- `TestCrossImpl_SubscriptionDefaults` (server-default rate-limit divergence)
- `TestCrossImpl_ThroughputEnvelope` (high-rate delivery, opt-in Limits)
- `TestCrossImpl_InspectFullSurface` (diagnostic confirmation — pass means no unexpected entity types)

When any of these fails against any impl, the inspect surface tells operators why in one log.

---

## §5 Open questions

- **~~Lift inspect from perfreview to its own module?~~ DONE (commit `e2d1b11`).** Inspect package lifted, no longer behind perfreview build tag.
- **Should `inspect.ContentStream` be at the entitysdk level?** Still unresolved. Lean: keep at workbench-go level — it's diagnostic, not production application surface. Revisit when a non-workbench-go consumer needs it.
- **Cross-impl observability surface convergence.** v1.1 §8 ships `system/inspect/*` as v1.0 reference protocol shape. Workbench-go owns the L2 implementation; cross-impl agreement crystallizes when a second impl ships against it. See `system/inspect/*` builds in workbench-go's roadmap (post §9 #8 audit).
- **Replay safety.** Replaying a captured wire frame to a peer could trigger side effects. Needs "dry-run" or "isolated" mode where replays go to a sandboxed content store. Per v1.1 §10, L2 replay is in the forward direction; the replay/revert/edit-and-replay design space is deferred to a later cross-impl review.
- **Cross-peer propagation of inspect entities.** Open per workbench-go RESPONSE §8 and core-go SECURITY-ADDENDUM §3. Three resolutions tabulated (convention / substrate enforcement / operator policy); workbench-go's lean is convention + per-extension §9 #7 declaration. Routed to arch in closing memo.

---

## §6 Calibration link

This direction emerged from the discipline gap captured at `feedback_read_prior_art_before_routing_findings` and the broader pattern:

> Before forming an attribution, locate and observe the actual entity at the failure surface. A path-tap or store-dump takes minutes; an attribution memo takes hours; routing the wrong attribution to arch + multiple impl teams costs days.

The inspect facility makes "observe the actual entity" cheap — symptom localization at the entity-system boundary becomes routine instead of effortful. Per the v1.1 §7.1 boundary-discipline correction: this does NOT eliminate source-reading or classical debugging. **It shortens the path to the right handler.** Discipline is still required to read the handler's source once the symptom points at it. Inspect makes the source-reading focused instead of speculative.

---

## §7 Cross-references

**Workbench-go inspect package** (commits `4467abb`, `e2d1b11`, `74ab3b2`):
- `inspect/tap.go` — legacy Tap (handler-replacement); kept for test sinks
- `inspect/dispatch.go` — DispatchTap (parallel, safe-on-prod) — v1.1 hook consumer
- `inspect/content.go` — ContentStream
- `inspect/binding.go` — BindingStream — v1.1 hook consumer
- `inspect/wire.go` — WireRecorder — v1.1 hook consumer
- `inspect/subscription.go` — SubscriptionTracer (engine-side) — v1.1 hook consumer
- `inspect/dump.go` — EntityDump + FindUnder + FindChainErrors
- `inspect/chain.go` — TraceChain composed capability
- `perfreview/inspect_v11_test.go` — cross-impl probe validating all hooks

**Architecture canonical**: the governing spec is GUIDE-INSPECTABILITY v1.1, which this direction implements at the workbench-go L2 layer.

**Underlying core-go hooks consumed**:
- `core/peer/builder.go:159` (WithNamedContentHook)
- `core/peer/builder.go:172` (WithBindingHook — observe-only alias for WithNamedSyncHook)
- `core/peer/builder.go:194` (WithWireHook)
- `core/peer/builder.go:215` (WithDispatchHook)
- `ext/subscription/inspect_hooks.go:59,66` (Engine.AddEmitHook / AddDeliverHook)

**Lessons memory**: `[[feedback_read_prior_art_before_routing_findings]]`
