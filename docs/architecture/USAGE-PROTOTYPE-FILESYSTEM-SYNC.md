# Prototype: Filesystem-synced peers

**Status:** Working prototype — Phase E v1 + Phase C follow + SQLite
persistence. Demonstrates the deployment shape end-to-end with two
known limitations (called out below). The system is not yet what
this guide describes as the long-term claim — read §6 before
believing the prototype proves anything operational.

This guide walks through getting two `entity-shell` peers running
on different hosts (or two terminals on one host), pointing each
at a filesystem directory, and watching changes propagate from
one to the other. The goal is the user's framing: **"safe to just
turn on, reads from the filesystem, tracks changes, syncs trees."**

What the prototype proves today:
- File save on peer A → entity in A's tree → revision auto-version
  → revision head propagates to peer B over the network → content
  materializes at the matching path on B's tree.
- Both peers persist their state across restarts.
- The capability grants for the chain are scoped (not owner-cap),
  so the security boundary is real.
- `ls archives/notes/` on B shows the same files A sees, with same
  content via `cat`. End-to-end round-trip confirmed in
  `shellcmd/cmd_local_files_multipeer_test.go::TestE2E_MultiPeer_FsSavePropagates`.

---

## 1. Prereqs

- This repo (`entity-workbench-go/`).
- Sibling checkout `../entity-core-go/`. Required by the workspace
  module resolution (see top-level README).
- Go 1.25.1 via toolchain. `GOTOOLCHAIN=go1.25.1` is what the
  Makefile uses.
- Two terminals (or two hosts if testing over a real network).

Build the binary once:

```
GOTOOLCHAIN=go1.25.1 make shell-build
```

This produces `./entity-shell` with the build version stamped in.
Copy it to both hosts, or use it from both terminals.

## 2. Create named identities

Each peer needs a stable identity so its peer-id survives restarts.
On each host (or terminal):

```
./entity-shell identity create alice
./entity-shell identity create bob
```

The keypairs land at `~/.entity/identities/{name}`. Back these up —
they're the only thing that MUST survive a wipe. See
`DEPLOYMENT-DIRECTION.md §2`.

## 3. Start peer A (the file-watching source)

In terminal A:

```
mkdir -p ~/notes
./entity-shell -identity alice \
               -storage sqlite \
               -listen 127.0.0.1:9101 \
               -open-access
```

What this does:
- Binds the `alice` identity (stable peer-id across restarts).
- Persistent SQLite at `~/.entity/peers/alice/store.db` (the default
  derived from the identity name per GUIDE-PERSISTENCE.md §1.1; the
  parent directory is auto-created).
- Listens for inbound peers on `127.0.0.1:9101`.
- `-open-access` grants connecting peers wildcard caps. **Dev only**
  — production needs scoped grants via the role extension.

Pass `-storage-path` explicitly to override the default — useful when
you want to test different tree contents against the same identity,
or when running multiple peers from the same identity in different
environments (e.g. `-storage-path ./scratch/peer.db`).

You're now at the entity-shell REPL.

## 4. Mount a filesystem directory on A

Inside A's REPL:

```
mount ~/notes archives/notes/ -include "*.md"
revision config put notes archives/notes/ -auto
```

Line 1 bridges `~/notes` to `archives/notes/` via the workbench
ingest pipeline. With `-include "*.md"`, only markdown files reach
the watcher → tree → ingest path. Each admitted `.md` file gets a
`doc/markdown-file` entity at `archives/notes/{relpath}`.

Line 2 enables auto-versioning on the `archives/notes/` prefix.
Every entity write under that prefix produces a new revision —
which is what the follow chain on B will track. Mount and revision
config are deliberately separate verbs: "filesystem-sync this
prefix" and "version-track this prefix" are orthogonal concerns.

**Filter defaults — important.** Mount applies a built-in exclude
list (`.git`, `node_modules`, `target`, `dist`, `build`, `vendor`,
`.venv`, `__pycache__`, `*.exe`, `*.bin`, `*.so`, `*.dylib`,
`*.dll`, `*.a`, `*.o`, `*.class`, `*.pyc`) unless you pass
`-exclude` (which **replaces** rather than extends the defaults).
Include defaults to empty = admit everything not excluded. Without
`-include "*.md"`, the watcher reads every text/source file in the
tree, builds a FileData entity per file, and the ingest handler
then skips non-markdown entries — fine for small trees but
needlessly memory-hungry on big ones. Use `-include` to scope the
watcher to what you actually want ingested.

**POC stance: markdown only at the typed layer.** The ingest
handler today produces `doc/markdown-file` entities exclusively;
non-markdown files admitted by include/exclude are accepted into
the FileData layer but no typed `doc/*` entity is built and no
tree:put fires. The long-term direction is per-extension type
discrimination (text → `doc/text-file`, code → `doc/code-file`,
binary → `doc/binary-file`) paired with a type-aware viewer panel.
For now, scope mounts to markdown explicitly via `-include`.

## 5. Start peer B (the follower)

In terminal B:

```
./entity-shell -identity bob \
               -storage sqlite \
               -listen 127.0.0.1:9102 \
               -open-access
```

Same default-path derivation as peer A: SQLite lives at
`~/.entity/peers/bob/store.db`.

Then connect to A and install the follow chain:

```
connect alice 127.0.0.1:9101
revision follow archives/notes/ alice
```

The first line dials A and handshakes (open-access on both sides
means the handshake succeeds without explicit role grants).

The second installs the 3-step continuation chain on B that:
1. Subscribes to A's revision head pointer for `archives/notes/`.
2. On head change, fetches A's revision content (fetch step).
3. Ingests the fetched envelope into B's content store.
4. Merges the new head into B's local revision state.

The chain uses a **scoped** capability — narrowed to fetch on A,
content:ingest locally, revision:merge locally, and chain-error
delivery. Not owner-cap.

## 6. Try it

In terminal A's shell:

```
exec sh -c 'echo "# Hello World\n\nFirst note" > ~/notes/hello.md'
```

(Or just open `~/notes/hello.md` in another terminal and write to
it directly. The fsnotify watcher picks up either path.)

Wait ~3 seconds (fsnotify debounce ≈ 2s + chain dispatch). Then:

**On peer A:**
```
ls archives/notes/                  # → hello.md
cat archives/notes/hello.md         # → doc/markdown-file with content + title
revision log archives/notes/        # → one version
```

**On peer B:**
```
revision log archives/notes/        # → same version hash as A
revision status archives/notes/     # → head matches A's head
ls archives/notes/                  # → hello.md
cat archives/notes/hello.md         # → same content A has
```

Both peers see the same content at the same path. Full round-trip
from A's filesystem to B's tree.

## 7. Inspecting state

Useful commands on either peer:

| Want to see | Command |
|---|---|
| Connected peers | `info` |
| Local tree | `ls`, `cat <path>`, `tree -depth N` |
| Tree-search | `find <prefix> <substring>`, `grep <prefix> <regex>` |
| Revision log | `revision log <prefix>` |
| Active subscriptions | `subscription ls` |
| Continuation chains | `continuation ls system/inbox/` |
| Mounted filesystems | `mounts` |
| Identity bundle path | `info` (current peer's identity hash) |
| Binary version | `entity-shell -version` (one-shot, before the REPL) |

## 8. Persistence + restart

Stop both shells with Ctrl-D or `exit`. Restart with the same flags
— same `-identity`, same `-storage` (and same `-storage-path` if you
overrode the default). Both peers come back up with their state
intact:

- Identity-bound peer-id unchanged.
- Tree contents unchanged.
- Revision history unchanged.
- Mount config persists; the workbench reloads watchers on startup
  via the localfiles handler's `Engine.Load()` (called from
  `shell.App.New`).

You should NOT need to re-run `mount`, `revision config put`, or
`revision follow` after restart — the configs and chain entities
are tree-resident.

## 9. Known limitations to be aware of

In rough order of "you'll hit this":

1. **Concurrent edit-vs-delete favors the edit by default.**
   The deletion-markers Amendment 4 sets the default
   `deletion_resolution` strategy to
   `preserve-on-conflict`: if peer A deletes a path while peer B
   concurrently edits it, both peers converge to the entity (B's
   edit wins; A's delete is silently dropped). For workflows
   that prefer "delete intent is sticky," configure
   `deletion_resolution: deletion-wins` per-prefix via a
   `system/revision/merge-config` entity. Both defaults silently
   drop one signal — neither is "lossless." Pick based on
   workload bias.

   Finding 10 (burst-write data loss) is **closed**. The F10
   investigation produced seven rounds of fixes (parts 1-7) plus
   the deletion-marker spec amendment; the merge math now
   satisfies CRDT add-monotonicity. Burst tests pass 50/50.

2. **on_error failures are silently lost** (Finding 7). If
   something in the chain breaks, you may see no error surface at
   all. The first thing to try when something doesn't propagate is
   to enable debug logging by rebuilding with a logger plumbed
   through `PeerConfig.DebugLog` (we don't currently expose this
   as a CLI flag; it's an SDK-level field). The `subscription ls`
   and `continuation ls system/inbox/` commands also help.

3. **Identity-aware peers leak ~4 entities per restart** (Finding
   1). Bounded per restart but accumulates. Tracked spec-side as
   PROPOSAL-RESTART-EQUIVALENCE §5.4. For the prototype this is a
   non-issue; for long-running deployed peers it becomes one.

4. **Subscription unmount is incomplete** (TODO in `cmdUnmount`).
   The mount verb's `unmount` clears the workbench's mount
   registration but doesn't cancel the subscription that drives
   it. Restart the peer to fully clear.

5. **No version flag on the binary by default** unless built via
   `make shell-build`. Plain `go run` produces an "unstamped dev
   build" — fine for prototyping, not for production.

## 10. Viewing the tree

There's no graphical tree-browser tied into this prototype yet. The
TUI (`console/`) and canvas (`canvas/`) applications exist but run
on their own peers — they're not pointed at the entity-shell binary
today. For the prototype, `ls`, `tree`, `cat`, `find`, `grep`, and
the `revision log/status/diff` family inside `entity-shell` are the
viewers.

For direct SQLite inspection (debugging):

```
sqlite3 ~/.entity/peers/bob/peer.db \
  "SELECT path FROM locations WHERE path LIKE '%/archives/notes/%'"
```

This is useful when something's off and you want to confirm what
the location index says vs. what the shell verbs report.

## 11. If you're writing new handlers

The prototype works, including bidirectional concurrent sync, but
there's an architectural footgun in the protocol layer worth
knowing about if you extend the system with new handlers.

**The rule:** any handler invoked by subscription delivery that
needs to dispatch an outbound EXECUTE (cross-peer or even a
nested local dispatch that itself crosses peers) MUST do the
outbound work in a goroutine. Return 200 to the caller
immediately; the dispatcher's serve goroutine has to be freed to
process the next incoming frame.

**Why:** the core protocol's serve loop is single-threaded per
connection. A handler that synchronously dispatches outbound
blocks the serve loop. In symmetric peer-to-peer scenarios (both
peers' handlers firing on each other simultaneously) this
deadlocks at the connection layer. Both sides time out at 15s
with no convergence. Tracked as Finding 9b — pending a core
protocol fix.

**Examples in this codebase that do it right:**

- `workbench.RevisionConvergeHandler.Handle` — returns 200, runs
  `Pull` in a goroutine.
- `inbox.Handler.handleReceive` — returns 200, runs
  `continuation:advance` in a goroutine.

**Examples that are safe synchronously** (because they don't
dispatch outbound):

- `workbench.NotificationIngestHandler.Handle` — reads + writes
  the local tree only.
- `workbench.IngestTransformHandler.Handle` — pure transform; no
  dispatch.
- `workbench.ChainErrorsHandler.Handle` — local tree bind only.
- The core-go `revision:fetch`, `revision:merge`, `content:ingest`
  handlers — local operations that respond synchronously to remote
  callers.

**If your handler dispatches outbound:** copy the
`RevisionConvergeHandler.Handle` pattern. Decode the inputs,
return 200 immediately, do the heavy work in a goroutine with its
own context. If the goroutine fails, log via `DebugLog` — there's
no in-band way to report failure once you've returned 200.

This is a discipline issue today; when the core protocol fixes
the single-threaded serve dispatch (Finding 9b), the discipline
becomes a performance optimization rather than a correctness
requirement.

## 12. Where to read more

- `DEPLOYMENT-DIRECTION.md` — operational posture; what the
  prototype is heading toward.
- `USAGE-SHELL.md` — full shell command reference.
- `USAGE-REVISION-HISTORY.md` — revision/history feature reference.

## 13. If something doesn't work

Order of suspicion based on what we've hit:

1. **Both peers running `-open-access`?** Without it the connect
   handshake fails. Look for "connection refused" or auth errors.
2. **Identities created on both sides?** `~/.entity/identities/{name}`
   must exist. Use `identity create <name>` if missing.
3. **Listener port available?** `-listen 127.0.0.1:NNNN` will
   fail-fast if the port is taken.
4. **fsnotify debounce.** Allow ~2-3 seconds after the file change
   before checking. `localfiles` uses a 2000ms debounce by default.
5. **Revision config installed?** `revision config put notes
   archives/notes/ -auto` must succeed before changes will produce
   versions. Check `revision log archives/notes/` returns a list.
6. **Subscription on B?** `subscription ls` should show one
   entry pointing at A's revision head pattern.
7. **Chain installed on B?** `continuation ls system/inbox/follow/`
   should show three entries (fetch, ingest, merge).
8. **Capability bound?** `ls system/capability/grants/chain/`
   should show your mounted root + follow chain.
