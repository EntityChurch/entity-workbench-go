@AGENTS-STANDARD.md

# entity-workbench-go

**Our addressable name is `entity-workbench-go`** — that is this repo's directory name, and it is
what other seats put in a packet's `To:` line. Address us by it and nothing else.

Read **AGENTS-STANDARD.md** first (imported above); it carries the ecosystem-wide conventions.
This file adds entity-workbench-go specifics and **stays small on purpose** — the accumulated
findings live in `docs/agents/memory/`, indexed below.

> **Reference an ADR; do not copy one.** `[ADR-NNNN]` unqualified means the *ecosystem* ADR —
> read it at its source rather than keeping a copy here, because a copy goes stale silently.
> Cite this repo's own as `[<repo>-ADR-NNNN]`. Two that change daily work: **[ADR-0031]**
> (`docs/status/` publishes nothing — so it is written for the next session, with no scrub
> obligation) and **[ADR-0012] Am. 1** (in a *canonical* doc, cite by content or a release tag,
> never a branch SHA, because published history is authored fresh at the release boundary and an
> internal SHA resolves to nothing for a reader).
>
> **`git add -A` is not safe in this tree.** `AGENTS-STANDARD.md` and `METHODOLOGY.md` are shared
> files maintained outside this repo and updated in place, so they can change underneath you
> mid-session — and a blanket `add -A` then sweeps thousands of lines you did not write into a
> commit about something else. It has happened here. **Stage explicit paths, or read
> `git status` before staging.** The same habit generalizes: when a tool or a teammate tells you
> what landed in your tree, measure it in your tree before repeating the number.

## Overview

Application + performance layer for the entity ecosystem — **not** a conformance
implementation. Ships **entity-shell** (the primary CLI / leading edge of feature
development) and an **Avalonia** desktop frontend, plus a frozen **console** (tview)
renderer, all over the Go workbench stack / V7 protocol / `entity-core-go` store. The
in-tree `entitysdk/` is the **de facto reference SDK** (stewarded here until it spins out
— treat it as the authoritative Go SDK impl, not workbench-internal glue).

### Our public surface — what "breaking" is measured against

**In:** the command surface of the shipped binaries — `entity-shell`, `entity-console`,
`entity-publish`, `entity-vcs`, `entity-fetch` — meaning their verbs, flags, exit codes and
`-json` output shape; the **exported Go API of `entitysdk/`**; and the shapes we *author* that
something else reads back — tree paths and prefixes, the static publish directory layout, and
persisted application state.

**Out:** every other package is internal and is refactored freely (`workbench/`, `shellcmd/`,
`shellboot/`, `programs/`, `fetch/`, `inspect/`, `console/` internals, and the Avalonia C#
frontend). Human-readable console prose is not a format; panel layout and the GUI's visual
arrangement are not promises; `perfreview/`, the harness scripts and every `make` target beyond
the standard verbs are development instruments. Published documents are addressed to a reader,
not pinned by a caller — correcting one is never a breaking change.

*The protocol wire format is not ours to break or to promise; it belongs to the spec and to
`entity-core-go`. Ours is what we build on top of it.*

## Where knowledge lives — read this before adding anything to this file

Three homes, and one question decides it: *would a competent newcomer need this **before** their
first change, or only when they hit the thing it describes?*

| home | answers | lifecycle |
|---|---|---|
| **`AGENTS.md`** (this file) | how do I work here today? | living, **bounded**, edited in place |
| **`docs/agents/memory/`** | what would I otherwise rediscover the hard way? | living, **indexed** |
| **`docs/status/`** | where are we this week? | dated, written once, ages out |

**This file used to be all three.** It reached 279 KB — nine times the 30 KiB an agent context
budget assumes, and past the point where several agents silently truncate it. The findings were
not the problem; the filing was. They moved to `docs/agents/memory/` on 2026-09-17, **verbatim
and byte-identical**, and `docs/agents/memory/INDEX.md` is the map.

**When a session ends holding something worth keeping, it goes in a memory topic file, not
here** — unless it changes how the repo is *operated*, in which case it belongs in this file and
something else probably leaves. And the rule that bounds the whole thing:

> **An entry that could become a check SHOULD become one — and then it is deleted from memory.**
> Memory is where a finding waits *while it is still only prose*. It is not where findings retire.

### The memory index

Open the topic when its subject is your subject; do not read them all. Full list and one-line
summaries in `docs/agents/memory/INDEX.md`.

| file | open it when |
|---|---|
| `BUILD-AND-ENVIRONMENT.md` | the build behaves oddly, or you are about to measure with a binary |
| `TESTING-AND-SWEEPS.md` | **before quoting any failure count**, and before adding a gate |
| `AVALONIA-GUI.md` | any panel, control, template, or render-path work |
| `CRASH-FORENSICS-AND-SIGNALS.md` | a crash, a signal, or anything below the managed runtime |
| `FOLDER-SHARING-AND-SYNC.md` | share / accept / direction / conflicts / delivery / catch-up |
| `MOUNTS-AND-LOCAL-FILES.md` | mounts, the watcher, ingest, history recording |
| `CAPABILITIES-AND-GRANTS.md` | anything that grants, denies, or is denied |
| `SDK-AND-STORE.md` | addressing, paths, dispatch targets, derived indexes, SDK shape |
| `PUBLISHING-AND-CONSUMING.md` | publish, fetch, registry, transports, the two roads |
| `FEED-AND-SOCIAL-CONVENTIONS.md` | EMBED / FEED / data-exchange work |
| `PROGRAMS-AND-COMPUTE.md` | `programs/`, the compute host, Axis-1 |
| `CROSS-REPO-ROUTING.md` | **before opening a packet or citing another repo** |
| `DOCS-AND-PUBLICATION.md` | touching `CANONICAL-DOCS.toml` or anything that publishes |

## How we work here — Disciplines & Doctrines · tier **FULL**

This repo runs the entity-OS methodology at the **Full** tier for the Avalonia/.NET UI runtime
— held where conformance alone can't reach a GUI. The framework is `METHODOLOGY.md` (maintained
upstream, identical in every repo, **carried but not loaded** — open it by trigger); the charter
below carries the local grounding, and **this repo is one of the worked instances the framework
was reconciled from** — D1–D11 there are inherited verbatim, D12–D27 here are ours, earned on the
eight crash-hunt commits, two feedback episodes, the 2026-08-18 publisher/connectivity pair, the
v1.13 adoption trio, the 2026-08-19 cross-impl consume run, the 2026-08-20 reachability audit,
and the 2026-08-21 crash hunt that found a month-old fatal bug the moment an instrument could
reach it.

- **Disciplines** (invariants — the *what*): `docs/architecture/DISCIPLINE-CHARTER.md` —
  D1–D27, the ten review questions, the anti-pattern catalog AP1–AP117, and the promotion
  criteria (§5) that the ecosystem ladder generalizes.
- **Substrate model** (ground truth): `docs/architecture/MODEL-AVALONIA-RUNTIME.md` — what
  the Avalonia/.NET/Skia/X11 runtime actually does (stack diagram, lifecycle matrix, the
  seven-boundary map — **Boundary G is the POSIX signal layer** — and the invariants). Read
  before any layout/lifetime/render/**crash** work.
- **Recipes & conventions** (patterns that respect the rules):
  `docs/architecture/GUIDE-AVALONIA-PANEL-PATTERNS.md` (P0–P7, new panels lift these) +
  `TESTING-STRATEGY.md` + `LOGGING-CONVENTIONS.md`.

Session start: read the charter → the substrate model → the memory topic for your area →
anything newer than the point `docs/STATUS.md` records as last read, and update that marker. A
specification change that moves a *table*, a *default*, or a **MUST** is read the same session it
is found, **before feature work** — it has twice been the case here that a landed spec change sat
unread while we shipped past it, once leaving a validator rejecting a configuration that had
become legal.

Task start: open the matching doctrine. Every feature/audit ends by feeding its lessons back into
the disciplines — the ratchet (*a feature must make us stronger, not weaker*).

**Doctrines — one exists now.** `docs/architecture/DOCTRINE-CRASH-FORENSICS.md` is this
repo's first, earned on the 2026-08-21 hunt: the procedure for a crash that leaves no managed
dump, in the order that actually converges (reach before forensics; first signal before any
dump; `si_code` before `si_addr`). Open it at the *start* of any crash investigation — its
whole point is that the intuitive order wastes days.
The rest are still owed: for Feature / Audit / Foundation work, run the procedures as
`METHODOLOGY.md` §7 states them and codify the substrate-native steps here when a run surfaces
one — name the recurring cycle first, then let each step own one lever of it.

**Work that crosses a repo boundary has its own rules and they were each earned the hard way** —
delivery versus intention, the counterpart trackers, packet form, and marking which sentences are
measured. They are in `docs/agents/memory/CROSS-REPO-ROUTING.md`; **open it before writing to
another seat**, not after.

*(Sibling repositories named anywhere in this repo sit beside it under a shared parent. A `../`
path is relative to the repo root; the `../../` form is relative to a Go module directory.)*
**When you read another repo, its git is read-only** — `git status` first, stage specific paths,
never `git add -A` outside your own working directory.

## Setup / environment

- **Go pinned to 1.25.1** (forced by core-go's `ext/go.mod` `go 1.25.0`) — the Makefile
  pins it. Per AGENTS-STANDARD, never set `GOTOOLCHAIN=` inline.
- **Sibling `../entity-core-go/` is required.** Every `go.mod` uses `replace` directives
  resolving to `../../entity-core-go/core` and `../../entity-core-go/ext`; without the
  sibling, `go build` fails at module resolution. `README.md` documents the layout, and
  `make preflight` refuses early with a named cause rather than module-resolution spew.
- **Avalonia builds go through podman, always.** The host needs `make` + `podman` and never
  .NET; never `dnf install dotnet`.

## Build & test

`make` is the build interface (see AGENTS-STANDARD). `make help` lists everything; the full
target catalogue is in the `Makefile` header. **The detail — which targets lie to you, the
podman collider, the load-dependent failures — is in `docs/agents/memory/TESTING-AND-SWEEPS.md`
and `BUILD-AND-ENVIRONMENT.md`. Read the first one before quoting any failure count.**

| target | what it is for |
|---|---|
| `make doctor` | prerequisites + the sibling kernel — **run this first on a strange machine** |
| `make build` | all shipped Go binaries (entity-shell + entity-console) |
| `make test-each` | **the target that tells you the state of the tree** — every suite to completion, pass/fail table, logs in `.test-logs/`, ~12 min |
| `make test` | full sweep (`-race -count=1`). **Stops at the first failing package** |
| `make test-<pkg>` | per-package: `sdk` `shell` `shellcmd` `shellboot` `workbench` `programs` `publish` `fetch` `bridge` `inspect` `shellpanel` |
| `make lint` / `fmt` | `go vet` only / gofmt. **`lint` does not check formatting**; run `fmt` as its own commit |
| `make check` | lint + test |
| `make run` | build + REPL (`ARGS=` for one-shot) |
| `make gui` / `gui-run` | Avalonia frontend: with / without an image rebuild |
| `make demo` | scripted CLI tour in a throwaway HOME — fastest end-to-end validation |
| `make reachability` | the D23 sweep: does every model have a surface a user can reach |

**One environment rule matters more than the rest: nothing else may run a podman target while a
sweep is running.** A second `:Z` bind mount revokes the first container's access mid-run, and
the sweep then reports red for reasons outside the tree. The tell is a suite that "failed" in
**0s**, or a missing `.test-logs/<suite>.log`. Start a sweep, then keep your hands off podman
until it prints its table.

Targets deliberately **outside** `test-native` because they need a network, a sibling checkout,
or the public internet — a sweep target that can go red for a neighbour's reasons teaches people
to ignore the sweep: `crossimpl-go`, `consume-live`, `twopeer-sync`, `twopeer-gui`,
`threepeer-sync`, `gui-drive`, `loadtest`, `perfreview`, and `make -C avalonia crash-hunt`. Each
script's header states what a green run claims and what it does not; **do not restate either
looser anywhere else.**

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
  `programs` (the entity-native programs track), `shellcmd` / `shell` (entity-shell verb-ops
  + REPL), `shellboot` (shared bootstrap + `PeerManager`/multi-peer lifecycle), `avalonia/`
  (bridge + C# frontend), `console/`. `ext/identity/` is the identity extension.
  Dep direction: `workbench → entitysdk`; `programs → entitysdk`;
  `shellcmd → entitysdk, workbench`; `shellboot → entitysdk, shellcmd, workbench`.
  **`workbench` cannot import `shellcmd`.**
- **`programs/` is a sibling of `workbench`, not a layer of it** — it depends on `entitysdk`
  **only** and must never import `workbench` (that dependency was zero at extraction; keep it
  zero).
- **workbench is the brain; renderers are thin I/O.** All business logic lives renderer-neutral
  in `workbench/`; `Render()` returns a plain struct any renderer drives. **Never reimplement
  model logic in C# or tview.** Detail in `docs/agents/memory/AVALONIA-GUI.md`.

### Where documents live

| path | what it is |
|---|---|
| `docs/architecture/` | the canonical framework — charter, substrate model, guides, direction docs. **Undated / living**, edit in place |
| `docs/agents/memory/` | the accumulated findings, by topic. Living, indexed, **published** |
| `docs/STATUS.md` | the rolling status log. **Outside `docs/status/` on purpose** — the path is the declaration. **It publishes**: write it for the next session, but know a stranger can read it |
| `docs/status/` | dated `STATUS-YYYY-MM-DD.md` snapshots (immutable once published), `HANDOFF-*`, and the `TRACKER-*` files. **Never published, no scrub obligation** |
| `docs/outbox/` | packets we have **sent**. Nothing else lives here. **Never declared in `CANONICAL-DOCS.toml`** |
| `docs/archive/outbox/` | packets whose recipients have acknowledged them |

**`CANONICAL-DOCS.toml` is a published artifact, not configuration** — it is a **keep-list**, so
an omission is an act of deletion against anything already public. Read
`docs/agents/memory/DOCS-AND-PUBLICATION.md` before touching it.

**Don't synthesize project state from `git log` or top-down code reading** — use the framework +
`docs/STATUS.md` + the newest `docs/status/HANDOFF-*`. **There is no roadmap doc and there has
never been one.** "What's next" lives in the newest handoff's recommended-order section and in
`STATUS.md`'s "Waiting on"; one direction doc per topic covers the rest.

## Boundaries — do NOT modify

- **You work on `dev`. You do not touch `master`, ever.** `master` is the **public canonical
  mirror** and is republished at each release — it is not a branch this repo's working sessions
  advance, propose advancing, or reason about. Promotion `dev → master` is the **release act**,
  performed by maintainers; [ADR-0015] and its amendment are the authority, [ADR-0022] covers the
  pipeline. **`dev` being ahead of `master` is the normal, expected steady state** — not a pending
  decision, not a status-file row, simply unreleased work. This file used to carry *"the one
  decision left: `dev` is N commits ahead of `master`"* as an open item across sessions, which
  presented someone else's act as our decision and invited a future session to act on it.
  **A session that finds itself weighing a merge to `master` has already gone wrong; there is
  nothing to weigh.**
- **Do not push to any remote unless you were asked to, by name, in this session.** Publication
  is the release act and it is not a session's to perform.
- **`../entity-core-go/` is a sibling dependency, not part of this repo.** Read it for
  protocol/store behavior; never edit it from here (route cross-impl changes through
  `docs/outbox/` per `docs/agents/memory/CROSS-REPO-ROUTING.md`).
- **Status snapshots (`docs/status/STATUS-YYYY-MM-DD.md`) are immutable once published** — never
  re-open a closed snapshot to add work; write a new dated one.
- **Avalonia is podman-only** — don't touch the host package set for the .NET toolchain.
- **Not the conformance team.** When a cross-impl wire bug surfaces during perf/feature work,
  capture `file:line` + a reproducer and route it — don't extend the probe into a validation
  harness. That is core-go's `validate-peer`.

## Two conventions that live in the code, not in a document

- **Logging:** `PanelLog` breadcrumb discipline + category list per
  `docs/architecture/LOGGING-CONVENTIONS.md`; the pre-crash breadcrumb is the forensic surface.
- **Testing:** four tiers per `docs/architecture/TESTING-STRATEGY.md` — **naming the tier is the
  discipline.**
