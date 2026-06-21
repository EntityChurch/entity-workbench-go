# Entity Workbench (Go)

A **reference application** built on the [Entity Core Protocol](https://github.com/EntityChurch/entity-core-protocol).
It demonstrates one coherent way to build an entity-native application — it is
**a reference paradigm, not a mandate**. The protocol requires none of these
shapes; the workbench is an opinionated, worked example you can learn from,
lift from, or ignore.

## Where this sits in the stack

```
entity-core-architecture   the protocol specification
        │
entity-core-go             the Go reference implementation (the kernel)
        │
entity-workbench-go        ← this repo: bindings / apps layer
```

Workbench **depends on** `entity-core-go` (core + ext). **Nothing depends on
the workbench** — it is a leaf. It ships:

- **`entitysdk/`** — a Go SDK over the kernel (typed wrappers, storage,
  identity bundles, revision/continuation helpers).
- **`shell/` + `shellcmd/`** — `entity-shell`, the primary CLI (REPL +
  one-shot) for peer / identity / capability / tree management.
- **`console/`** — `entity-console`, a TUI renderer (tcell + tview, pure Go,
  no CGo). Kept as a frozen discipline-enforcer.
- **`avalonia/`** — the primary desktop GUI (Avalonia/.NET, driven by a Go
  c-shared bridge). Built and tested in a podman container.
- **`workbench/` + `shellboot/` + `shellpanel/`** — the shared,
  renderer-neutral application library the frontends wire into.
- **CDN corridor** — `entity-publish`, `entity-vcs`, `entity-fetch`: small
  binaries for publishing / fetching a peer's tree over HTTP.

For design orientation start with `docs/architecture/` — the canonical surface
is enumerated in [`CANONICAL-DOCS.toml`](CANONICAL-DOCS.toml) (the discipline
charter, the Avalonia runtime model, the panel patterns, and the testing +
logging conventions, plus the usage guides).

---

## Build & run — everything goes through `make` + `podman`

The native Go binaries need only a Go toolchain (pinned by the Makefile to
**go 1.25.1** via `GOTOOLCHAIN`; you do not set it yourself). The Avalonia GUI
needs only **`make` + `podman`** on the host — the .NET SDK, Go, and Skia
dependencies all live inside the container, never on your machine.

```bash
# Native (Go) — no container
make test                 # full race-enabled test sweep across all modules
make build                # entity-shell + entity-console + CDN corridor tools
make shell                # go run the entity-shell REPL
make shell ARGS="info"    # one-shot shell command
make console-run          # build + run the TUI

# Avalonia desktop GUI — podman only
make -C avalonia build    # build the multi-stage image
make -C avalonia up       # build + extract + launch on the host
make -C avalonia test     # headless UI tests (Avalonia.Headless.XUnit)
```

The `entity-shell`, `entity-console`, and Avalonia frontends share the same
identity + storage flags:

```bash
entity-shell   -identity peerA -storage sqlite -storage-path ~/.entity/peerA.db
entity-console -identity peerA -storage sqlite -storage-path ~/.entity/peerA.db
```

Markdown and other files enter the workbench via the shell's `mount` verb,
which bridges a filesystem directory to a tree prefix; GUI edits round-trip
back to disk. See `docs/architecture/USAGE-SHELL.md` and
`docs/architecture/USAGE-PROTOTYPE-FILESYSTEM-SYNC.md`.

### Resource caps (podman)

Every podman build/run is fenced with hard memory ceilings (zero swap) so a
build cannot take the host down. The committed defaults live in
[`caps.mk`](caps.mk) (memory sized to the Avalonia build's measured peak +
headroom). Override per machine **without editing the tracked file** via an
env var (`CAP_MEM=8g make -C avalonia build`) or a gitignored `caps.local.mk`.

---

## Repository layout (sibling dependency)

This repo currently builds against `entity-core-go` as a **sibling directory**:

```
entity-systems/
├── entity-workbench-go/        ← this repo
└── entity-core-go/             ← required sibling (core/ + ext/)
```

Each `go.mod` requires the kernel by its canonical module path
(`go.entitychurch.org/entity-core-go/{core,ext}`) and resolves it through a
local `replace` to `../../entity-core-go/{core,ext}`. This is the **in-between
zone**: the published vanity module path is wired up, but resolution is still
local — no network fetch, no tag required. The final cutover (when the vanity
path is actually published) is a one-line change per module: drop the
`replace`, and the existing `require … @v0.8.0` fetches from the network.

If `../entity-core-go/` is missing, the build fails at module resolution.

The repo-root `go.work` composes the in-repo modules for editor / language
server convenience; cross-repo resolution to the kernel happens via the
per-`go.mod` `replace` directives above.

---

## Versioning

Module / app version is **0.8.0** (preview). The repo is not git-tagged yet —
tagging is a freeze action reserved for the release cut.

---

## Supporting the project

This project is developed in the open. If it's useful to you, the best support is
to use it, report issues, and contribute back — see
[CONTRIBUTING.md](CONTRIBUTING.md).

To support the work directly, see the project's funding page.
