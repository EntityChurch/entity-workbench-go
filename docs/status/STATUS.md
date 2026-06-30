# entity-workbench-go — status

_Updated: 2026-06-30 · public: v0.8.0 (master)_

## Where it is

The Go **reference application** built on the Entity Core Protocol — an opinionated,
worked example of an entity-native app, **not** a conformance implementation and not a
mandate. It is a leaf in the stack (depends on the `entity-core-go` kernel — `core` + `ext`
— via local `replace`; nothing depends on it). It ships the in-tree `entitysdk/` (the de
facto reference Go SDK: typed wrappers, storage, identity bundles, revision/continuation/
discovery helpers), **`entity-shell`** (the primary CLI and leading edge of feature work),
an **Avalonia/.NET desktop GUI** driven through a Go c-shared bridge (podman-only build),
a frozen **`console`** (tview) TUI kept as a renderer-neutrality enforcer, and a small CDN
corridor (`entity-publish` / `entity-vcs` / `entity-fetch`, plus `entity-seed-site` /
`entity-serve-cors`). Maturity: **v0.8.0 research preview** — feature-complete enough to
demonstrate the paradigm, with a full race-enabled test sweep green across all modules
(~200 test files) save the consciously-waived items below. APIs and the path-syntax /
module-resolution story are still settling.

## Where we left off

v0.8.0 is the public research-preview release; no code or protocol changes are in flight
since the cut. The next substantive work is to pick the next feature thread — either close
the last console→Avalonia parity gap with the **handler-browser panel**, or run the
research-first SDK-helpers (S4–S8) session that's owed before any compute-ergonomics pass.

The last substantive **feature** thread before that pause was the peer-connections / LAN
discovery work. **mDNS "Nearby Peers" closed end-to-end** — discovery, dial, age-out + reap
of stale candidates, and an empty-state ("Searching…" → "No peers found on the LAN.") UX,
all proven with live 2-peer tests. Just ahead of it, the **peer-connections panel** landed
WebSocket scheme routing in `AppPeer.Connect`, auto-`Listen` wiring, and async dial-out;
and a **universal-resolution `resolve()` seam** was built and proven in
`entitysdk/resolve_chain.go` (its first concrete consumer is still open). These are present
in v0.8.0; they're the leading edge to pick back up from.

## Backlog

**Release / dependency cutover**
- **Module-path cutover.** Every `go.mod` requires the kernel by its vanity path
  (`go.entitychurch.org/entity-core-go/{core,ext}` @ v0.8.0) but resolves it through a
  local `replace` to the sibling `../../entity-core-go/{core,ext}` (offline; no network, no
  tag). When the vanity path is actually published + tagged, the cutover is **one line per
  module**: drop the `replace`, let `require … @v0.8.0` fetch from the network. Then run the
  deferred no-siblings, clone-fresh `make build` to prove it.
- **Path-syntax migration.** User-facing surfaces still use `alias:path`; the pinned
  substitution sigil is `@alias` (`:` is reserved for `<handler-path>:<op>`). Prefer
  `@alias` in new docs/examples now to minimize churn when the code change lands.
- **`entitysdk` spin-out.** Stewarded in-tree as the authoritative Go SDK; intended to
  eventually become its own module.

**Known waivers / kernel-blocked (not workbench bugs — sibling-impl rule)**
- **Identity-rebootstrap storage leak.** The bounded-rebootstrap pin in `entitysdk/` is
  currently `t.Skip`'d with its assertion intact. Re-applying an identity bundle leaks
  ~+1 path / +4 entities **per reload** (linear, unbounded). Root cause is the kernel's
  identity *ceremony* re-issuing the local-peer→controller cap + sibling signature with
  ceremony-time-varying material instead of reproducing prior content hashes. Delete the
  Skip to re-arm the regression the moment the kernel's re-apply is idempotent.
- **Subscription slow-consumer head-of-line block.** The producer queue is bounded with
  drop-on-full; the gap is a missing **per-delivery deadline** on the consumer-side
  synchronous `Deliver`, which pins one shard worker when a consumer stalls. Isolated to the
  slow-consumer path — normal delivery is healthy. Kernel-side fix.
- **Subscription delivery saturation.** 100% delivery below ~2K notifs/sec; a cliff to
  ~47–49% at 5K+/sec, dropped silently (drop-on-full). Typical workbench heartbeat is far
  below saturation, so usable, but mount-burst / K-Put/sec scenarios drop. Endorsed fix is
  **parallel delivery workers** (K-worker pool; near-linear speedup) — kernel-side after
  cross-impl alignment; workbench is committed to that option.
- **Revision auto-version is O(N)/Put.** Per-Put latency under auto-version grows linearly
  in the existing path count under the prefix, *regardless of trie shape*, because the trie
  is rebuilt from scratch on every Put. Fix is incremental update (cache node hashes; walk
  leaf→root recomputing only the ancestor chain) — a kernel/spec concern.
- **Content-store GC contract.** Path-overwrite accumulates orphaned content entities
  indefinitely; a naive "delete unreferenced + VACUUM" sweep is unsafe because hash
  references are encoded across many typed sites (revision trie nodes, conflict versions,
  continuation dispatch caps, identity cert chains, multi-granter signers, …). Awaiting a
  cross-team reachability/GC contract built on the kernel's reverse-hash index; workbench
  ships nothing until it lands.

**Tooling / CI**
- **perfreview rot guard.** The `perfreview` module is build-tagged out of `make test` and
  silently rotted against a prior kernel surface change. Add a compile-only gate
  (`go vet -tags=perfreview ./perfreview/...`) so it can't rot unnoticed again.

**Discovery follow-ups (post-"Nearby Peers" close)**
- N-panel × M-peer scan multiplier — one scan loop per discovery handle, no dedup by handle;
  bounded, but amplifies multicast in dashboards / test rigs.
- TXT-pair parsing in the Avalonia bridge duplicates the kernel's private parser (drift risk).
- Scan interval is hardcoded ~5s; no "Scan now" affordance.
- No headless test exercises a *populated* nearby list (would need mDNS in the test
  container); same-process 2-peer tests can't catch cross-machine IP-selection bugs.

**UI / renderer**
- **Console multi-peer UX** (deferred): peer-picker modal, status bar showing the current
  peer, `peer create` / `peer destroy` verbs, peer-switch.
- **Handler-browser panel** — the one surface where `console` is still ahead of Avalonia;
  closing it ends the console→Avalonia parity gap.
- **Manifest-driven panel registration** (deferred from the multi-peer plan).
- Avalonia drives feature work and may outpace the frozen `console` renderer; console-parity
  is explicitly *not* an obligation (console exists to keep the app renderer-neutral).

**SDK ergonomics / compute**
- **SDK ergonomic helpers (compute "S4–S8").** Owed a research-first session before any
  implementation pass.
- **Compute DSL parser.** Deferred; built *on top of* the S4–S8 helpers, only once an
  authoring workflow actually needs one.
- **Wire a real consumer of the `resolve()` seam** (`entitysdk/resolve_chain.go`) — the seam
  is proven but has no first concrete consumer yet.

**Hardening / cleanup**
- Revision-recovery diagnostic: hub-spoke fetch-diff recovery with auto-version *off* logs
  an independent transport failure mode; captured as an observation, the test passes.
- Selection-state reader hardening: replace the silent legacy-tolerance path in
  `entitysdk/workspace_state.go` with log-on-violation (default) or reject-on-decode.

## Waiting on

- **`entity-core-go` kernel:**
  - published + tagged vanity module path → unblocks the `replace`-removal cutover;
  - an idempotent identity-ceremony re-apply → re-arm the rebootstrap-leak regression;
  - a per-delivery deadline + parallel delivery workers → subscription slow-consumer block
    and the saturation cliff;
  - incremental revision-trie update → the auto-version O(N)/Put cost.
- **Cross-team:** the content-store GC / reachability contract.

## Done recently

- Cut and published the **v0.8.0** initial public research-preview release,
  including the release-readiness hardening: full `make build` + `make test` + Avalonia
  podman build/headless-test green; `caps.mk` podman resource fencing (hard memory ceilings,
  zero swap, sized to the measured Avalonia build peak); the canonical doc surface declared
  in `CANONICAL-DOCS.toml`; and measured, citable performance numbers captured in
  `docs/architecture/PERFORMANCE-CHARACTERISTICS.md` (1M-write scale test flat at scale,
  signed delivery ~70–83k/sec, etc.).
- **mDNS "Nearby Peers" closed end-to-end** — age-out + dedup + reap of stale candidates
  (`entitysdk/discovery.go`: `DiscoveryCandidateMaxAge`, `ReapStaleDiscoveredCandidates`),
  empty-state UX in `PeerConnectionsPanel`, live 2-peer age-out regression test.
- **Peer-connections panel** (Avalonia): WebSocket scheme routing in `AppPeer.Connect`,
  auto-`Listen` wiring, async dial-out.
- Built + proved the **universal `resolve()` seam** (`entitysdk/resolve_chain.go`).
- **SITE v0.5 read-projection** substrate + Avalonia `SiteViewPanel` (+ `console/site_view.go`
  stub) + headless tests.
- **Multi-panel layout** — `PanelStack` with per-panel add/close and viewport-bound sizing.
- Shipped **SDK restart-recovery helpers**: `RestorePriorSubscriptions`
  (`entitysdk/subscription_restore.go`) + `ReconcileSinceLastSeen` (`entitysdk/reconcile.go`,
  `entitysdk/recover_after_restart.go`).
- Landed shell verbs **`tail`**, **`query`/`count`**, **`peer`**
  (`shellcmd/cmd_tail.go`, `cmd_query.go`, `cmd_peer.go`).
- Established the **Disciplines** framework — charter D1–D17 + nine review questions +
  anti-pattern catalog (`docs/architecture/DISCIPLINE-CHARTER.md`), the Avalonia runtime
  model (`MODEL-AVALONIA-RUNTIME.md`), panel patterns, and the testing + logging conventions.

## Next

1. **Pick the next feature thread** — either close the last console→Avalonia parity gap with
   the **handler-browser panel** (+ manifest-driven panel registration), or run the
   **research-first SDK-helpers (S4–S8)** session that's owed before any compute-ergonomics
   implementation.
2. **Track the kernel's vanity-path publication;** when it lands, execute the
   `replace`-removal module cutover, run the no-siblings clone-fresh `make build`, and add the
   `go vet -tags=perfreview` gate so the perf module can't silently rot again.
