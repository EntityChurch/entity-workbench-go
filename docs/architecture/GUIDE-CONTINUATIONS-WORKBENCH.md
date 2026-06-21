# Continuations — Workbench Programming Guide

**Owner:** workbench team. Written from real SDK usage.
**Companion to:** `EXTENSION-CONTINUATION.md` (spec, normative) and
the `entitysdk.ContinuationClient` source.

This guide is for workbench developers building real flows. It
describes the surface as it exists today in `entitysdk/`,
`shellcmd/`, and the shell — not what the spec might one day allow.

---

## 1. The model in one screenful

A **continuation** is a tree-bound entity that says "when something
delivers a result here, run this dispatch." It's a deferred function
call, persistent across restart, capability-scoped.

Three kinds (per `EXTENSION-CONTINUATION.md` §2):

- **Forward** (`system/continuation`) — single deferred dispatch
- **Join** (`system/continuation/join`) — accumulate N results, then dispatch
- **Suspended** (`system/continuation/suspended`) — created by the runtime
  when an EXECUTE can't complete (resource missing, etc.); resumable or
  abandonable

Four operations on the handler:

| Op | Purpose |
|---|---|
| `install`  | Bind a forward/join continuation entity at a tree path |
| `advance`  | Deliver a result + status; runs the dispatch |
| `resume`   | Re-dispatch a suspended continuation with fresh state |
| `abandon`  | Delete a suspended continuation |

In the workbench SDK these are surfaced behind a typed client:

```go
peer.Continuation()            // local
peer.ContinuationAt(remoteID)  // cross-peer

cc.Install(ctx, path, contEnt)
cc.Advance(ctx, path, resultEntity, status)
cc.Resume(ctx, path)
cc.Abandon(ctx, path)
```

Plus client-side helpers (`continuation.go`):

```go
entitysdk.ValidateContinuation(cont)        // structural lints
entitysdk.ValidateContinuationJoin(cont)
entitysdk.InboxPath(purpose, instance, step) // canonical install-path helper
```

And an observability surface (`continuation_observe.go`):

```go
cc.ListSuspended(ctx)
cc.ListAt(ctx, pathPrefix)
cc.Inspect(ctx, path)
```

---

## 2. Building a continuation, end to end

The simplest possible install + advance round-trip. Local peer; the
dispatch_capability is the peer's owner self-cap (already in the
local content store, so no cross-peer cap setup needed).

```go
peer, _ := entitysdk.CreatePeer(entitysdk.PeerConfig{})
defer peer.Close()

cont := types.ContinuationData{
    Target:             "system/tree",
    Operation:          "get",
    Resource:           &types.ResourceTarget{Targets: []string{"observed/marker"}},
    DispatchCapability: peer.OwnerCapability().ContentHash,
}
contEnt, _ := cont.ToEntity()

path := entitysdk.InboxPath("demo", "probe", "fetch")
// → "system/inbox/demo/probe/fetch"

_, err := peer.Continuation().Install(ctx, path, contEnt)
```

That installs a forward continuation at the inbox path. To trigger
it, dispatch something at that path — either via a subscription
delivery, or directly:

```go
result, _ := entity.NewEntity("test/note", someData)
err = peer.Continuation().Advance(ctx, path, result, 200)
```

`Advance` invokes the continuation handler's advancement algorithm,
which dispatches the target+operation pair using the params
captured at install time.

For the spec-compliant rules on what shape a continuation can take,
see `ValidateContinuation` in `entitysdk/continuation.go` — it
enforces the same structural rules the handler will run server-side
(target/operation non-empty, dispatch_capability set, `result_field`
requires `params`, etc.). The wrapper calls Validate before
dispatching install, so malformed continuations fail fast without
a round-trip.

---

## 3. Wiring a 2-step chain (the canonical pattern)

The most useful chain shape: subscribe to a path; on every change,
dispatch a fetch; on the fetch result, dispatch a merge. This is
how the entity-sync prior art (`entity-core-go/cmd/entity-sync/main.go`)
mirrors tree state across peers, adapted here for revision-level
follow.

### 3.1 The pieces

You need three primitives:

1. **`SubscribeRawAt(remoteID, pattern, deliverURI, "receive", opts)`**
   (entitysdk/subscription.go). Subscribes on the remote peer; tells
   its engine to dispatch notifications to `deliverURI`. Unlike
   `Subscribe` / `SubscribeAt`, this does *not* register a Go-channel
   inbox handler — the caller is responsible for ensuring the deliver
   URI has a continuation entity bound at it. Returns a
   `*RawSubscription` whose `Close()` cancels the subscription
   remotely.

2. **`Continuation().Install(ctx, path, contEnt)`** (twice). One
   continuation at the path the subscription delivers to (step 1 of
   the chain); a second one that the first delivers to (step 2).

3. **Caller-supplied dispatch_capability**. For now,
   `peer.OwnerCapability().ContentHash` is the workbench's
   wildcard self-cap and is the simplest choice for local-only
   chains. Cross-peer chains need a cap rooted at the remote's
   authority — see §5.

### 3.2 The follow chain (revision auto-sync)

`shell/cmd_revision_follow.go` builds this chain. Logical shape:

```
on bob (the follower):
  raw sub  → pattern  = system/revision/{prefix-hash}/head   (on alice)
             deliver  = entity://{bobID}/system/inbox/follow/{aliceID}/{prefix}/fetch

  continuation @ system/inbox/follow/{aliceID}/{prefix}/fetch:
    target     = entity://{aliceID}/system/revision
    operation  = fetch
    params     = {prefix: "..."}
    deliver_to = entity://{bobID}/system/inbox/follow/{aliceID}/{prefix}/merge

  continuation @ system/inbox/follow/{aliceID}/{prefix}/merge:
    target           = system/revision
    operation        = merge
    params           = {prefix: "...", strategy: "auto"}
    result_field     = remote_version
    result_transform = extract: "head"   (pulls .head out of fetch result,
                                          injects as remote_version into
                                          merge params)
```

To use it:

```sh
> connect laptopA 192.168.1.10:9001
> revision follow shared/ laptopA
following shared/ @ laptopA (sub=...)
  fetch inbox: system/inbox/follow/{aliceID}/shared/fetch
  merge inbox: system/inbox/follow/{aliceID}/shared/merge
  pattern:     system/revision/{prefix-hash}/head
```

`revision unfollow shared/ laptopA` tears it down.

**End-to-end convergence WORKS.** The cap-chain gap closed when the Phase C
revision-follow chain landed.
Both Form 1 (`subscribe head → revision:fetch-diff → tree:merge`) and
Form 2 (`subscribe head → revision:pull`) compose declaratively. See
`shell/cmd_revision_follow.go` for the production chain; the
bidirectional E2E test (`shellcmd/cmd_local_files_bidirectional_test.go`)
runs both peers writing concurrently and asserts convergence.

---

## 3.3 Substrate resolution as a handler step (the L11 idiom)

**Established at Stage 3 close (v2 closure-completion).** When a
chain step needs to pull entities cross-peer to make a subsequent step's
inputs locally complete — the workbench/blob-resolve case — substrate
resolution lives in a **handler step**, not a transform.

### Why a handler step, not a transform

Transforms (`ResultTransform.Extract`, `Select`) project data within an
envelope. They run synchronously, in-process, without dispatching. A
substrate-resolve step needs to *dispatch* `system/content:get`
cross-peer, run the cap-checked sequencer to drain the closure, then
hand control to the next chain step. That's a handler's job.

Per `GUIDE-EXTENSION-DEVELOPMENT.md §4.10` (arch's L11 idiom): chain
dispatches a handler that calls `content.EnsureClosure` under its
`internal_scope`; next chain step proceeds with closure local. Cap
discipline stays in the existing V7 §6.8 handler-grant model — no
implicit cap inheritance from chains.

### The SDK shape

```go
import "entity-core-go/ext/content"

// In your handler:
disp := content.AtPeer(hctx, crypto.PeerID(sourcePeerID))
if err := content.EnsureClosure(ctx, disp, blobHash, "system/content"); err != nil {
    var se *content.StatusError
    if errors.As(err, &se) {
        return handler.NewErrorResponse(se.Status, se.Code, ...)
    }
    return handler.NewErrorResponse(503, "blob_pending_sync", ...)
}
// Closure is now locally complete. Next step (e.g. local/files:write
// content-mode) reads from the local store; no bytes traverse the wire.
```

`content.AtPeer` returns a `handler.Dispatcher` that aims dispatch at
the source peer while the cap-check still flows through the inner
HandlerContext dispatcher. `content.EnsureClosure` is the cap-checked
sequencer over `system/content:get` with §7.4 sender batching +
503 partial-sync retry built in.

### Worked example: cross-peer file sync chain

`workbench/blob_resolve.go` is the production reference:

```
on bob (the receiver):
  raw sub  → pattern  = system/files/sync/* (on alice)
             deliver  = entity://{bobID}/workbench/blob-resolve
             opts     = {include_payload: true}

  workbench/blob-resolve:receive handler:
    1. Unwrap inbox delivery → tree-change notification
    2. Decode the file entity (delivered via include_payload)
    3. F9 idempotency short-circuit: skip if local tree already has the
       same blob hash at the target path (terminates loops in symmetric
       bidirectional topologies — see §7)
    4. content.EnsureClosure via content.AtPeer against alice — drains
       the blob's full closure (manifest + every chunk) into bob's
       local content store
    5. Dispatch local/files:write content-mode locally — atomic disk
       write + tree bind, no bytes on the wire

  Result: bob's filesystem mirrors alice's, content-addressed, with
  full §7.x dedup (unchanged chunks not re-fetched on modify)
```

### When to use this vs revision-follow

| Use case | Shape |
|---|---|
| Repository-level cross-peer mirror (multiple files, CRDT merge) | `subscribe head → revision:fetch-diff → tree:merge` (§3.2) |
| Single-prefix file sync with content-mode atomic writes | `subscribe local/files/* → blob-resolve` (§3.3, this section) |
| Custom substrate-bearing pipeline | Wrap your own handler that calls `content.EnsureClosure` + dispatches downstream |

Revision-follow operates on tree-shaped state with revision-graph
merge semantics. Blob-resolve operates on file-shaped state with
filesystem semantics. They compose with the same primitives.

---

## 4. Observability — the "ps" for continuations

Continuations are entities in the tree. They have no spec-level
`list` op (per EXTENSION-CONTINUATION.md). The workbench provides
listing on top of `tree:list` + `tree:get`:

```go
// Forward + join continuations under a path prefix:
views, _ := peer.Continuation().ListAt(ctx, "system/inbox/")

// Suspended continuations (canonical prefix):
suspended, _ := peer.Continuation().ListSuspended(ctx)

// Detailed view of a single continuation:
view, ok, _ := peer.Continuation().Inspect(ctx, path)
```

`ContinuationView` is a typed projection with the kind-discriminated
fields:

```go
type ContinuationView struct {
    Path                string
    Hash                hash.Hash
    Kind                ContinuationKind  // forward / join / suspended
    Target, Operation   string
    Resource            *types.ResourceTarget
    ResultField         string
    ResultTransform     *types.ContinuationTransformData
    DeliverTo, OnError  *types.DeliverySpec
    RemainingExecutions *uint64           // nil = standing
    DispatchCapability  hash.Hash
    // join-only:
    Expected            []string
    Received            map[string]struct{}
    // suspended-only:
    Reason, ChainID     string
    OriginalAuthor      hash.Hash
    SuspendedAt         uint64
}
```

Shell verbs over the same surface:

```sh
> continuation ls [path-prefix]   # default: system/inbox/
> continuation suspended          # suspended continuations
> continuation inspect <path>     # detailed view
> continuation abandon <path>     # drop a suspended continuation
> continuation resume <path>      # re-dispatch a suspended continuation
```

Same shape for subscriptions:

```go
peer.Subscriptions().List(ctx)
peer.Subscriptions().Inspect(ctx, path-or-id)
```

```sh
> subscription ls
> subscription inspect <path|id>
> subscription rm <id>
```

These are the surface the user described as "DPS [ps] of the shell."
Use them to answer: *what processes are installed on this peer? are
any stuck? what was the last thing each one did?*

---

## 5. The cap-chain question (cross-peer continuations)

`EXTENSION-CONTINUATION.md` §3.2 requires that the continuation's
`dispatch_capability` chain-walks to the writer's authority. For
**local** continuations the peer's owner self-cap satisfies this
trivially.

For **cross-peer** continuations — where the dispatch fires an op
against a remote handler — the cap shape is more involved. The
workbench's owner self-cap is rooted at the local peer's identity
and authorizes operations in the local namespace. To dispatch to a
remote handler, the cap needs to chain to the remote's authority
(role grant or similar).

The cap-chain works end-to-end for both revision-follow (Phase C) and the
§3.3 substrate-resolution idiom (Stage 3). Use
`MintCrossPeerChainCapability(remoteID, grants, parentCap)` to mint a
chain-shaped cross-peer cap; attach via `SetDefaultDispatchCap` on the
ContinuationData.

For chains that include a §3.3 substrate-resolution handler step, the
cross-peer cap-check is also exercised when the handler calls
`content.EnsureClosure` against the source peer's namespace. The
workbench Stage 3 cap-delegation test
(`shellcmd/cmd_stage3_cap_delegation_test.go`) validates both the
positive case (chain converges under scoped grants) and the negative
case (chain stalls when alice's grants don't authorize bob's
`system/content:get`).

See `shellcmd/cmd_stage3_case1_5_subscription_test.go` for the production
shape; see `entitysdk/cross_peer_cap_mint_test.go::TestMintCrossPeerChainCapability_Shape`
for the cap-bundle structural contract.

---

## 6. Path conventions

The SDK uses (and `InboxPath` produces) the convention:

```
system/inbox/{purpose}/{instance}/{step}
```

For follow chains this resolves to:

```
system/inbox/follow/{remote-peer-id}/{prefix-slug}/{fetch|merge}
```

The convention is not load-bearing — callers can install at any
path. `continuation ls` defaults to `system/inbox/` because that's
where SDK-installed chains live by convention, but you can pass any
prefix.

---

## 6.5 Post-restart recovery (Stage 5 helpers)

Subscription state on a peer is in-memory: when the process restarts,
the substrate has no record of what the peer was previously
subscribed to. Per Stage 5 round-2 cycle, workbench-go ships two SDK
helpers in `entitysdk/` that codify the recovery pattern.

The canonical recovery shape after any peer restart:

```go
ap, _ := entitysdk.NewAppPeer(cfg)
ap.Connect(ctx, remoteAddr)                          // 1. re-establish
if err := ap.RestorePriorSubscriptions(ctx); err != nil {
    // F6 close — recovers subscriptions tracked in sidecar entities
}
since, _ := ap.LastSeenFor(remoteID, prefix)
delta, _ := ap.ReconcileSinceLastSeen(ctx, remoteID, prefix, since)
// F3/F7 close — pulls writes that landed during downtime
```

**Three steps, decoupled:**

1. **`Connect`** re-establishes the transport-level pool entry. Pure
   network plumbing; no protocol-level state.
2. **`RestorePriorSubscriptions(ctx)`** (`entitysdk/subscription_restore.go`)
   scans sidecar tracking entities the SDK persists when a subscription
   is registered, and replays the registration. Subscriptions whose
   sidecar persisted survive process restart; those whose sidecar did
   not (sub registered without `Persist`) do not.
3. **`ReconcileSinceLastSeen(ctx, peerID, prefix, sinceHash)`**
   (`entitysdk/reconcile.go`) composes `revision:fetch-diff` +
   `tree:merge` with the last-seen-hash threading. Pulls every write
   the remote committed under `prefix` after `sinceHash` and merges
   them into the local tree. Idempotent; safe to call on every boot
   even when nothing changed.

**Why three calls, not one — and the one-call sugar.** Each step has
a clear failure mode: re-connect can hang on bad DNS; subscription-
restore can fail if the sidecar's grant chain no longer chains (role
rotated, peer demoted); reconcile can fail if the remote dropped the
closure. Splitting lets the app surface "we're disconnected" vs "we
recovered, but lost some sub state" vs "we're up but behind on data."

**The one-call helper (Lane 5):**
`AppPeer.RecoverAfterRestart(ctx, publisherPeerID, publisherAddr)`
composes the three steps and returns a `RecoveryResult` with per-step
error fan-in. Use this for the typical "I have one publisher I care
about reconnecting to" case:

```go
ap, _ := entitysdk.NewAppPeer(cfg)
res, err := ap.RecoverAfterRestart(ctx, publisherPeerID, publisherAddr)
if err != nil {
    // Connect failed — short-circuit; no subsequent steps ran.
}
for _, sub := range res.Restored {
    go func(s *entitysdk.Subscription) {
        for evt := range s.Events() { /* dispatch */ }
    }(sub.Sub)
}
log.Printf("restored=%d reconciled=%d soft-errors=%d",
    len(res.Restored), len(res.Reconciled), len(res.Errors))
```

Notes on the helper:
- **Best-effort:** failures in step 2 (restore) or step 3 (reconcile)
  don't halt the rest of the sweep — each is collected into
  `RecoveryResult.Errors` with `Step` ("connect" / "restore" / "reconcile")
  and `Detail` (the specific item that failed) so the app can surface
  partial-recovery diagnostics.
- **404-soft:** when the publisher hasn't auto-versioned the prefix,
  reconcile returns `no_local_state`. The helper treats this as a
  zero-entity success (not an error), so apps using non-versioned
  subscription patterns aren't punished.
- **Multi-publisher:** the v1 helper handles one publisher per call.
  Apps with subscriptions against multiple publishers should call
  `RecoverAfterRestart` once per publisher; each call's
  `RestorePriorSubscriptions` sweep is idempotent.

The three-step form remains the underlying contract and is what tests
assert; the helper is sugar over it, not a replacement for it.

**Stage 5 reference data.** Cross-impl applicability:
`RestorePriorSubscriptions` proves F6 (HIGH) closes; the saturation probe
confirms a restored sub delivers 25/25 post-restart vs F6
baseline 0/25. `ReconcileSinceLastSeen` proves F3+F7 close; bootstrap
pulls 41 entities for 20 leaves; incremental delta pulls 31 for 15
new leaves. Both helpers are workbench-go BLESSED promotion candidates —
the shapes will travel to the other-language SDKs.

**What's NOT recovered automatically.** Per-subscription "what did I
last process" — that's app-level state. Apps that need exactly-once
semantics across restart should persist their own per-subscription
high-water-mark and resume from there. The SDK helpers cover the
substrate-level "what state of the world am I tracking"; downstream
processing watermarks remain the app's responsibility.

---

## 7. What's deferred

### Closed since the original draft

| Item | Resolution |
|---|---|
| Cross-peer continuation convergence (Phase C proof) | Closed — both Form 1 (`subscribe → revision:fetch-diff → tree:merge`) and Form 2 (`subscribe → revision:pull`) compose declaratively. Bidirectional E2E test asserts convergence. |
| End-to-end substrate-bearing chain (Stage 3 case 1.5) | Closed — `workbench/blob_resolve.go` + L11 idiom (§3.3). |
| Cap-chain visibility (silent rejection) | Closed — core-go's F11 visibility fix logs `"F11: lost-error marker bind FAILED at X: <err>"` when chain-error binding fails. |
| Compute (`system/compute`) typed client | Closed — Phase H foundation; see `entitysdk/compute_*.go` and `shellcmd/cmd_compute.go`. |

### Still deferred

| Item | Reason |
|---|---|
| Multi-step fluent chain builder | Per user direction: defer until real usage demands it. Today: plain `types.ContinuationData` struct + helpers. |
| Shape-level validation (does step N's output match step N+1's expected input?) | Needs the type extension's schema surface, which the SDK doesn't expose yet. `ValidateContinuation` covers structural lints only. |
| Iterative fetch-entities as a standing chain step | The one-shot `Pull` does this in Go; making it work as a chain step needs either a "loop-until-stable" continuation primitive or a smarter merge handler. |
| `Gate`/`Branch`/`For-each` utility handlers | Per the exploration doc, these can be SDK-level Go composition over the existing primitives. Not yet built — wait for a real use case. |
| TreeSet idempotency at the tree layer (F12) | Architectural decision pending — would eliminate the workbench-side idempotency check in blob-resolve (§3.3 step 3). Round 7+ scope; consumer-layer defense-in-depth works for now. |
| Wire-layer transient-heap reduction at scale (F10) | Peak 845 MiB on 128 MiB transfer; acceptable for desktop/server, scoping question for IoT/constrained. Round 7+ scope. |

---

## 8. References

- **Spec** — `entity-core-architecture/docs/architecture/v7.0-core-revision/core-protocol-domain/specs/extensions/standard-peer-extensions/EXTENSION-CONTINUATION.md`
- **SDK ops** — `.../sdk-domain/specs/SDK-EXTENSION-OPERATIONS.md` §2 (continuation ops), §3 (subscription), §11 (closure-completion: `content.EnsureClosure` + `content.AtPeer` + Dispatcher)
- **Substrate-resolution idiom (L11)** — `GUIDE-EXTENSION-DEVELOPMENT.md §4.10` (arch)
- **Wire references** — `entity-core-go/cmd/entity-sync/main.go` (canonical 2-step tree chain); `workbench/blob_resolve.go` (production §3.3 worked example)
- **SDK source** — `entitysdk/continuation.go`, `continuation_observe.go`, `subscription.go` (`SubscribeRawAt`), `content_client.go` (`FetchBlobClosure` SDK wrapper)
- **Shell source** — `shellcmd/cmd_continuation.go`, `cmd_subscription.go`, `cmd_revision_follow.go`
