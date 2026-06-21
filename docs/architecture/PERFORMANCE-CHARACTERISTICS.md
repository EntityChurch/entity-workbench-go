# Performance Characteristics & Proven Capabilities

Canonical reference for what `entity-workbench-go` has **measured** and
**proven** about the entity system as a running application. Living doc — each
results block is stamped with the machine and date it was measured on; re-run
the harness and edit in place.

This is the workbench's answer to "does it actually work, and how fast?" — not
projections, but numbers from the in-tree measurement harness, plus the
capabilities the test suite demonstrates end-to-end.

---

## 1. What this measures, and how to reproduce it

All numbers come from the **`perfreview`** harness (`perfreview/`, gated by the
`//go:build perfreview` tag so it never runs in the default `make test` sweep).
It boots a real `entitysdk` peer on **SQLite** storage, drives configurable
workloads, and snapshots metrics (write latency percentiles, heap, goroutines,
SQLite file size, delivery counts) at log-spaced checkpoints.

```bash
make perfreview                          # full suite
make perfreview ARGS="-run TestScale"    # one investigation
```

**No `-race`.** The race detector imposes ~17× slowdown on `modernc.org/sqlite`
(pure-Go SQL), which would distort every measurement; the harness runs without
it deliberately. (`make test` keeps `-race` on for the correctness sweep.)

**Measurement environment (unless a block says otherwise):**

| | |
|---|---|
| CPU | 12 cores |
| RAM | 46 GiB |
| Go | go1.25.0 (toolchain go1.25.1) |
| Storage | `modernc.org/sqlite` (pure-Go), WAL |
| Build | `-tags=perfreview`, no `-race` |

44 investigations passed in the first captured run (which then hit the suite's
20-minute wall on a slow-consumer test — §5). The scale category was re-run
separately: `TestScale_PushTo1M` passed (1M writes) and `TestScale_OneMillionWrites`
captured flat-latency checkpoints to 400K before its 8-minute budget expired
(§2.1). The standalone subscription delivery-latency sweep did not get its own
budget this pass; delivery throughput is still well-characterized in §2.3. See
§5 for the two flagged core-go issues.

---

## 2. Measured results

### 2.1 Write throughput & latency (single peer, SQLite)

Per-Put latency stays in the **hundreds-of-µs** range and grows **sub-linearly**
as the tree fills (`revision_paired_test.go`, `scale_test.go`):

| Writes done | per-Put p50 | Notes |
|---|---|---|
| 0 | 618 µs | cold (first put, page-cache warm-up) |
| 50–300 | ~1 ms | steady |
| 450 | ~2 ms | growth 0→450 = **3.43×** (sub-linear vs 9× tree growth) |

- **Steady overwrite** (same path-set, N sweep): N=50 mean 720 µs; N=500 p95
  2 ms, p99 3 ms — overwrite cost is flat, dominated by content-hash + SQL upsert.
- **Concurrent writers:** 16 workers × 100 writes = 1600 writes in 193 ms →
  **8,280 writes/sec** with no lost writes (`concurrent_test.go`).
- **Bulk write p50 under load:** ~140 µs across a 50,000-write run
  (`feature_cost_test.go`, all-extensions-on).

**Scale to 1,000,000 writes** (`scale_test.go`, `scale_million_writes_test.go`):

- `TestScale_PushTo1M` **completed 1,000,000 writes** to a single SQLite peer
  (368.7 s wall, including periodic GC/heap sampling) — **passed**.
- `TestScale_OneMillionWrites` (hub + spoke subscriber) shows **flat write
  latency as the store grows**: wp50 held **192 → 203 µs** from 100K → 400K
  writes, hub DB grew linearly (~0.32 MiB per 1K writes; 31.7 → 127 MiB), heap
  112 → 243 MiB, goroutines steady at ~36. The run was cut at the 400K
  checkpoint by the **8-minute measurement budget** (PushTo1M consumed 368 s of
  the same window) — a budget cap, not a failure or a latency wall.

  | Checkpoint | wall | write p50 | write p95 | hub DB | heap |
  |---|---|---|---|---|---|
  | 100K | 22.1 s | 192 µs | 354 µs | 31.7 MiB | 111.8 MiB |
  | 200K | 46.9 s | 199 µs | 498 µs | 63.5 MiB | 149.9 MiB |
  | 300K | 73.1 s | 197 µs | 610 µs | 95.3 MiB | 188.8 MiB |
  | 400K | 100.9 s | 203 µs | 680 µs | 127.0 MiB | 242.9 MiB |

### 2.2 Capability / signature cost (Ed25519)

The cryptographic floor for signed delivery (`delivery_breakdown_test.go`):

| Op | per-op | throughput |
|---|---|---|
| Ed25519 sign | 19.35 µs | 51,684 ops/sec |
| Ed25519 verify | 41.98 µs | 23,819 ops/sec |
| `findSignatureFor` inner sign | 18.26 µs | (cacheable — see below) |

Caching the per-delivery signature would save ~18.3 µs/delivery.

### 2.3 Subscription delivery throughput

Signed delivery scales with worker parallelism (`delivery_breakdown_test.go`):

| Workers | deliveries/sec | with cached signature |
|---|---|---|
| 1 | 12,466 | 16,451 |
| 2 | 23,422 | 32,461 |
| 4 | 42,013 | 61,621 |
| 8 | 70,305 | **82,934** |

**Fanout barely touches write latency** (`subscription_axes_test.go`): at
**64 subscribers**, a 50,000-write run delivered **3,221,768** events while the
writer's p50 held at 146 µs (vs 142 µs at 0 subs) — subscriptions do not
back-pressure the writer on this workload.

### 2.4 Continuations (the deferred-process model)

`continuation_axes_test.go`:

- **Install:** 500 continuations in 131 ms → p50 250 µs, p99 435 µs, **3,031 B
  per continuation** of heap.
- **Dispatch (clock `now`, no continuation in path):** p50 74 µs, p95 92 µs,
  p99 118 µs — the deferred-process machinery is not in the hot path when unused.

### 2.5 Compute substrate

`compute_cost_test.go`:

- **Dispatch** (N=1000): p50 149 µs, p95 181 µs, p99 227 µs.
- **Registration:** 200 compute handlers in 231 ms (p50 1 ms each); resident
  cost 35.2 MiB heap / 1,127 entities after 200 handlers.

### 2.6 Cross-peer (two in-process peers over TCP)

`crosspeer_test.go`:

- Local `Get` p50 101 µs vs remote `Get` p50 700 µs → **6.9× cross-peer
  overhead** (p50), dominated by the wire round-trip + verify.

### 2.7 Feature-cost matrix (what each extension costs)

`feature_cost_test.go` / `disabled_extensions_test.go` — 50,000 writes, toggling
one extension off at a time:

| Configuration | write p50 | goroutines | Observation |
|---|---|---|---|
| all-default | 140 µs | 16 | baseline |
| minimal | 102 µs | 8 | floor |
| no-subscription | 139 µs | **8** | subscription owns the extra goroutines |
| no-identity-stack | 117 µs | 16 | identity stack ≈ 23 µs p50 |
| no-revision | 138 µs | 16 | revision near-free when idle |

**Auto-version is the one expensive feature when active**
(`saturation_multipeer_test.go`): hub-spoke publish at `autoversion=off` ran
**3,139 writes/sec** (per-Put 318 µs) vs `autoversion=on` at **441 writes/sec**
(per-Put 2 ms) — roughly **7× write cost** when every put cuts a version. Both
delivered 100% to all spokes.

### 2.8 Storage lifecycle (durability + reclaim)

`lifecycle_test.go`:

- **Restart preserves state**; **peer-id is stable across restart**
  (identity continuity proven on real SQLite).
- **VACUUM reclaims orphans:** after deleting 200,003 orphan rows (79.9% of the
  store), the DB stayed at 50.9 MiB until `VACUUM`, which dropped it to
  **16.5 MiB (32.4% of pre-GC — 67.6% reclaimed)**. Deletion alone does not
  shrink the file; VACUUM does. (Deletion is not erasure — reclaim is a separate
  explicit step.)
- Known: a small **+2 entity-count drift across restart** (cleanable via VACUUM)
  — see §5.

### 2.9 Resilience (partition / reconnect)

`partition_test.go`, `reconnect_burst_test.go`, `restart_mesh_test.go`:

- **Subscriptions auto-resume after a partition heals** — connection-level
  recovery is transparent to the subscription layer; the engine queues during
  the partition and drains on heal (49/50 delivered during partition, full
  recovery post-heal).
- Mesh subscriptions re-establish after a peer restart; mid-burst connection
  drop-and-resume loses no committed writes.

### 2.10 Dedup under bimodal load

`bimodal_workload_test.go`: a heartbeat-plus-burst workload suppressed **80–97%
of redundant heartbeat events** (content-addressed dedup) while delivering
**91.8–99.7% of burst writes** within the 500 ms window.

---

## 2.11 Cross-implementation interop (Go ↔ Rust ↔ Python, live peers)

Live multi-peer run: workbench-go's SDK drove **real Rust and Python peers**
(launched via core-go `peer-manager` as podman containers) over TCP
(`crossimpl_smoke_test.go`). Proven:

- **Wire + handshake** to both Rust and Python; peer-ids resolve.
- **Cross-impl writes** — Rust and Python both accept entity Puts from wb-go.
- **ECF byte-identical across all three impls** — the same payload produced the
  identical content hash (`ecf-sha256:e6a7ac…`) on Go, Rust, and Python. This is
  the deterministic-encoding interop guarantee, demonstrated end-to-end.
- **Subscription delivery from Go and Python publishers** to a wb-go subscriber
  via §6.11 inbound-reentry — **works** (PASS).

One gap found and isolated (see §5): a **Rust** publisher's reentry subscription
delivery to the same subscriber does **not** complete.

## 3. Proven capabilities (end-to-end, test-cited)

Beyond raw speed, these are behaviors the workbench has demonstrated **working**
against the Go kernel (and, where noted, across implementations):

- **Cross-peer continuation / revision-follow** — a follower tracks a source
  peer's tree across commits via `tree:extract` + `tree:merge`; works at any
  tree size (`entitysdk/tree_incremental_test.go`,
  `entitysdk/poc_incremental_sync_test.go`). Incremental sync transfers only the
  changed sub-tree, not the whole tree.
- **Identity ceremony on persisted storage** — bootstrap an identity-aware peer,
  drive role.Define / role.Assign, close, reopen against the same bundle + DB,
  and the peer is the *same* identity with prior role/cap state intact and able
  to dispatch fresh identity-extension ops
  (`entitysdk/storage_sqlite_identity_test.go::...RoundtripPreservesIdentity`).
- **The `resolve()` seam** — pluggable name→entity resolution proven in
  `entitysdk/resolve_chain.go`.
- **Inspect surface** — operator diagnostics (`entity-shell inspect …`) over
  live peers, exercised at scale in `perfreview/inspect_*_test.go`.
- **Filesystem ↔ tree bridge** — the shell `mount` verb round-trips a directory
  into a revision-tracked tree prefix and back to disk.

---

## 4. The tools that produced this (and that ship)

| Tool | What it is | Docs |
|---|---|---|
| `entity-shell` | Primary CLI — REPL + one-shot; peer/identity/role/revision/continuation/compute/inspect verbs | `USAGE-SHELL.md` |
| `entity-console` | TUI renderer (tcell+tview), frozen discipline-enforcer | README |
| `entity-avalonia` | Desktop GUI (Avalonia/.NET + Go c-shared bridge) | `avalonia/README.md` |
| `entity-publish` / `entity-vcs` / `entity-fetch` | CDN-corridor: publish / version / fetch a peer's tree over HTTP | README, `USAGE-*` |
| `make perfreview` | The measurement harness that produced this document | this doc, §1 |

---

## 5. Flagged issues (honest caveats)

Two items surfaced while producing this data. Both are characterized, neither
blocks the preview:

1. **Identity rebootstrap leak (core-go).** Re-applying an identity bundle to an
   already-populated store grows it by a constant per restart (+1 path / +4
   entities). Root cause is core-go's `ext/identity` ceremony, not workbench.
   Test waived for the preview; a handoff is filed with core-go. The §2.8
   "+2 drift across restart" is the same family.
2. **Subscription per-shard head-of-line block under a slow consumer.** A
   perfreview run timed out on the slow-consumer investigation. **Cause
   (verified with core-go):** the producer queue is already bounded
   with drop-on-full (`engine.go:475–481`); the gap is the **consumer-side
   synchronous `Deliver` call with no per-delivery deadline** (`engine.go:595`).
   A slow spoke that holds its delivery response pins one shard worker inside
   `Deliver`, head-of-line-blocking that shard only — the other shards' delivery
   loops keep idling (those are the "parked" goroutines in the dump, not the
   wedge). Re-running the suite excluding that one investigation completes
   cleanly; the normal delivery path is healthy (§2.3). Fix is a per-delivery
   deadline on core-go's side; a handoff is filed with core-go.

3. **Rust §6.11 reentry subscription delivery never completes.** A wb-go (core-go)
   subscriber on a **Rust** publisher never receives reentry-delivered
   notifications — Rust logs `reentry: connection closed before response`. Same
   subscriber receives fine from **Go and Python** publishers (§2.11), so the
   subscriber is exonerated; the issue is Rust's inbound-reentry `EXECUTE` shape.
   Deterministic over 3 runs. Rust-side; a handoff is filed with the Rust
   team. The cross-impl smoke test is left failing against Rust on purpose as
   a regression detector.

Also recorded: an **F18 revision-recovery diagnostic** — hub-spoke fetch-diff
recovery with auto-version **off** logged an independent transport failure mode
(`revision_paired_test.go`), distinct from the rate-cliff. Captured as an
observation; the test passes.
