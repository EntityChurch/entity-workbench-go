
# entity-workbench-go

Read **AGENTS-STANDARD.md** first. This file adds entity-workbench-go specifics.

## Overview

Application + performance layer for the entity ecosystem — **not** a conformance
implementation. Ships **entity-shell** (the primary CLI / leading edge of feature
development) and an **Avalonia** desktop frontend, plus a frozen **console** (tview)
renderer, all over the Go workbench stack / V7 protocol / `entity-core-go` store. The
in-tree `entitysdk/` is the **de facto reference SDK** (stewarded here until it spins out
— treat it as the authoritative Go SDK impl, not workbench-internal glue).

## How we work here — Disciplines

This repo runs the **Disciplines** layer of the entity-OS methodology (see
`AGENTS-STANDARD.md` §Methodology) for the Avalonia/.NET UI runtime — held where conformance
alone can't reach a GUI.
- **Disciplines** (invariants — the *what*): `docs/architecture/DISCIPLINE-CHARTER.md` —
  D1–D17, the nine review questions, the anti-pattern catalog.
- **Substrate model** (ground truth): `docs/architecture/MODEL-AVALONIA-RUNTIME.md` — what
  the Avalonia/.NET/Skia/X11 runtime actually does (stack diagram, lifecycle matrix, the
  six-boundary map, invariants). Read before any layout/lifetime/render work.
- **Recipes & conventions** (patterns that respect the rules):
  `docs/architecture/GUIDE-AVALONIA-PANEL-PATTERNS.md` (P0–P6, new panels lift these) +
  `TESTING-STRATEGY.md` + `LOGGING-CONVENTIONS.md`.

Session start: read the charter → the model. (Workbench runs the Disciplines layer; it does
not maintain the separate Feature/Audit Doctrine procedure docs that the browser/godot repos
carry.)

## Setup / environment

- **Go pinned to 1.25.1** (forced by core-go's `ext/go.mod` `go 1.25.0`) — the Makefile
  pins it. Per AGENTS-STANDARD, never set `GOTOOLCHAIN=` inline.
- **Sibling `../entity-core-go/` is required.** Every `go.mod` uses `replace` directives
  resolving to `../../entity-core-go/core` and `../../entity-core-go/ext`; without the
  sibling, `go build` fails at module resolution. README documents the layout.

## Build & test

`make` is the build interface (see AGENTS-STANDARD). Full target catalogue is in the
`Makefile` header.

- `make test` — full sweep (`-race -count=1`); ~50s total, `entitysdk`/`shellcmd` slowest;
  pin tests live in `entitysdk`.
- `make test ARGS="-run X -v"` — single test / forwarded flags (`ARGS=` is the only
  passthrough syntax `make` accepts).
- `make test-sdk` / `test-shell` / `test-shellcmd` / `test-workbench` — per-package suites.
- `make build` — all shipped Go binaries (entity-shell + entity-console).
- `make go ARGS="..."` — escape hatch; `ARGS` carries the subcommand (`vet ./...`,
  `mod tidy`, `env`, …).
- **Avalonia builds go through podman, always** — `cd avalonia && make build && make
  extract`, then `make host-run`. The host has no .NET and never needs it; never `dnf
  install dotnet`. Bridge-only smoke check: `cd avalonia/bridge && CGO_ENABLED=1 go build
  -buildmode=c-shared -o /tmp/libbridge-test.so .`.

## Code style

- **Read the source before asserting** path shape / addressing / namespace claims (see
  AGENTS-STANDARD). The whole tree is peer-id-namespaced, so **"peer-id keyed" is almost
  never a valid distinguishing claim** — if you reach for it to explain why something
  matters, you're probably about to mislead. Cite `file:line` in test comments and doc
  explanations.
- The project measures everything against the **17 disciplines (D1–D17)**, nine review
  questions, and anti-pattern catalog (AP1–AP8) in `docs/architecture/DISCIPLINE-CHARTER.md`.
- **Logging:** `PanelLog` breadcrumb discipline + category list per
  `docs/architecture/LOGGING-CONVENTIONS.md`; the pre-crash breadcrumb is the forensic surface.
- **Testing:** four tiers per `docs/architecture/TESTING-STRATEGY.md` — naming the tier is
  the discipline.

## Project structure

Architecture is a **dependency graph, not a strict stack** — five layers, where the panel
framework and the application are **siblings** (the panel framework could work without
entities):

1. **Entity Core** — peer, store, protocol (the `../entity-core-go` sibling).
2. **Entity Developer Framework** — Executor, PeerContext, Resolve, Format.
3. **Panel Framework** — panel, focus, actions, content contracts (entity-independent).
4. **Application** — content models, panel declarations, state persistence.
5. **Renderers** — medium-specific ordering + manifestation.

- Go packages: `entitysdk`, `workbench` (renderer-neutral models + business logic),
  `shellcmd` / `shell` (entity-shell verb-ops + REPL), `shellboot` (shared bootstrap +
  `PeerManager`/multi-peer lifecycle), `avalonia/` (bridge + C# frontend), `console/`.
  `ext/identity/` is the identity extension. Dep direction: `workbench → entitysdk`;
  `shellcmd → entitysdk, workbench`; `shellboot → entitysdk, shellcmd, workbench`.
  **`workbench` cannot import `shellcmd`.**
- `docs/architecture/` — canonical framework (charter, `MODEL-AVALONIA-RUNTIME.md`,
  `GUIDE-AVALONIA-PANEL-PATTERNS.md` recipes P0–P6, `TESTING-STRATEGY.md`,
  `LOGGING-CONVENTIONS.md`, `DEPLOYMENT-DIRECTION.md`, `SHELL-DIRECTION.md`,
  `REPOSITORY-WORKSPACE-ROADMAP.md`, active `PHASE-*-PLAN.md`). These are **undated /
  living** — edit in place.
  - `docs/status/` — the ephemeral status area: `STATUS.md` (the current self-contained
    thread) plus any dated `STATUS-YYYY-MM-DD.md` snapshots (immutable once published).
    (Moved here from the old top-level `status/`.)
  - `docs/architecture/reviews/` — dated cross-team exchanges (`reviews/{TOPIC}-{DATE}.md`);
    closed ones move to `reviews/archive/`.
- **Don't synthesize project state from `git log` or top-down code reading** — use the
  framework + latest status snapshot + roadmap. "What's next" lives in the roadmap, not in
  status docs; one direction doc per topic.

## Boundaries — do NOT modify

- **`../entity-core-go/` is a sibling dependency, not part of this repo.** Read it for
  protocol/store behavior; never edit it from here (route cross-impl changes via `reviews/`
  per AGENTS-STANDARD).
- **Status snapshots (`docs/status/STATUS-YYYY-MM-DD.md`) are immutable once published** — never
  re-open a closed snapshot to add work; write a new dated one.
- **Avalonia is podman-only** — don't touch the host package set for the .NET toolchain.

## Repo-specific gotchas

- **"identity" vs "keypair" — keep these distinct** (the word is overloaded across the
  codebase; full discussion `DEPLOYMENT-DIRECTION.md §3`):
  - **keypair** — the bare Ed25519 keypair (what `crypto.LoadIdentity/SaveIdentity`
    load/save — upstream misnomer; don't propagate it).
  - **identity bundle** — the on-disk directory shape
    (`entitysdk/identity_bundle.go::IdentityBundle`).
  - **identity entity** — the V7 hash-addressed public-key entity (`peer.Identity()`).
  - **identity extension** — the attestation + quorum + identity stack (`ext/identity/`).
- **workbench is the brain; renderers are thin I/O.** All business logic (entity
  resolution, CBOR/markdown formatting, handler discovery, tree/selection state, the
  content models — `tree_model`, `detail_model`, `shell_model`, `peer_info_model`,
  `log_model`, `handler_model`, …) lives renderer-neutral in `workbench/`; `Render()`
  returns a plain struct that any renderer drives. **Never reimplement model logic in C#
  or tview.** Treat `workbench` as the Go "standard library" for entity apps —
  protocol-first (execute / tree get-put), never direct store/index access from app code.
- **Multiple renderers are a discipline enforcer, not a parity obligation.** Console is
  kept (frozen, single-peer) purely to keep the renderer-neutral core honest; Avalonia
  drives all feature work and may outpace it. If a model-layer change breaks console,
  that's a signal the abstraction was wrong — fix the model, don't gate Avalonia on console
  parity. (The canvas/raylib renderer has been removed.)
- **DRY the integration, not the renderer.** Shared shell↔workspace wiring lives once in
  `shellcmd/integration.go` (e.g. `PersistAliases`, `PublishWDTo`); renderers add one line
  of wiring. Same closure in two renderers = extract it.
- **`shell.WD` is stored canonical `/{peerID}/...`, never `/@alias/...`.** The alias form is
  display-only (`shellpanel.Prompt()` applies `AliasFor` at render). Store-side surfaces
  (`Store.List`, `NamespacedIndex.canonicalize`) **panic** on a leading `@alias`. When
  seeding WD in a renderer/test, use `shellcmd.Path("/" + peerID + "/")`.
- **Path syntax migration owed:** `alias:path` ships today, but `@alias` is the pinned
  peer-id substitution sigil (`:` is reserved for `<handler-path>:<op>`), so prefer `@alias`
  in new user-facing docs/examples to avoid re-churn when the migration lands.
- **SDK shape:** `entitysdk.AppPeer` **is** a peer (always has a tree + full handler set +
  dispatcher + pool); `entitysdk.Client` is **not** (bare TCP wrapper, deferred). The
  keypair you operate under picks the surface; never open a fresh client connection to a
  peer your AppPeer already pooled under the same identity. Don't add `PeerSurface` /
  `*From` variants — URIs (`entity://{peer-id}/...`) encode the target.
- **Build the typed SDK clients against `core/types`** via `entitysdk/extdispatch.go`'s
  `extDispatch` — do **not** consume core-go's `ext/{identity,role}/sdk` proto-SDK (Go-only,
  no SDK error mapping; would puncture `*entitysdk.Error` predicates). **Layer-2 algorithm
  contract:** any operation whose bytes land in the tree (chunking params, slug/path
  canonicalization, CBOR canonical encoding, subscription pattern matching, …) must be
  byte-identical across impls — extract named constants + reference vectors. Layer-1
  ergonomics may vary freely.
- **Subscribe by prefix; never scan-and-filter.** Consumer refresh uses `Store.Watch`/
  `OnSelectionChange` (or `OnPrefixChange`) — listing the whole tree and filtering in-proc
  is a structural anti-pattern (O(store size) per render). Don't build a workspace-level
  polling dispatcher over the SDK; the SDK is the dispatcher.
- **App handler integration:** the app registers a handler at `workspace/app`; targeted
  refresh flows subscription engine → inbox → app handler → Go channel → UI (not
  "refresh-all on every tree event").
- **Perf measurement:** `modernc.org/sqlite` under `-race` is ~17× slower — perf benches
  must override the default flags (`GOTEST_FLAGS="-count=1"`); never trust SQL bench numbers
  taken with `-race`. To reconcile store/index count discrepancies, `SqliteStore.DB()` lets
  you `SELECT` the `entities` table directly.
- **Spec discipline:** workbench-application logic (workbench-owned handlers /
  `app/workbench/`, `archives/` namespaces) extends freely; **spec-adjacent** behavior
  (anything `system/*` or a documented domain handler) needs a spec read + cross-impl
  coordination first. To check whether an op is spec'd, read the `EXTENSION-*.md` section
  outline + manifest YAML (`pull: {input_type: ...}`) — grep with impl-style patterns misses
  unquoted YAML declarations.
- **Not the conformance team.** When a cross-impl wire bug surfaces during perf/feature
  work, capture `file:line` + reproducer and route it (Python encoder → Python team, spec
  ambiguity → arch, conformance test-gap → core-go) — don't extend the probe into a
  validation harness. That's core-go's `validate-peer`.
