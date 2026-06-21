# Entity Shell — Revision and History walkthroughs

**Owner:** workbench team.
**Companion to:** `USAGE-SHELL.md` (general shell usage) and the
typed clients in `entitysdk/{revision,history}.go`.

This doc walks through working scenarios for the two versioning
extensions exposed by the shell:

- **`history`** — per-path transition log + rollback. One entry per
  recorded mutation. Linear chain per path.
- **`revision`** — versioned tree snapshots scoped to a prefix. One
  version captures the whole subtree; commits, branches, tags, diffs.

Both are recorded **opt-in** — wired by default but dormant until
you install a config that matches the path/prefix you care about.

Every command sequence below has been run through `make shell` and
confirmed working at the time of writing. When something doesn't
work as documented, that's a regression — file it.

---

## 1. Quick mental model

| | `history` | `revision` |
|-|-----------|------------|
| Granularity | One path | One prefix (subtree) |
| Records on | Every mutation to that path | Every mutation to any path under the prefix (auto), or on explicit `commit` (manual) |
| Output of "log" | List of transitions: timestamp + event + content hash | List of version hashes; each version is a structural snapshot |
| Rollback unit | Single path → earlier hash | Whole prefix → earlier version (`checkout`), or single version replayed (`cherry-pick` / `revert`) |
| Branches/tags? | No | Yes |
| Cross-peer transfer? | No | Yes (push / fetch — deferred in shell) |

**Pick history when** you want "show me how this one document
changed over time" or "undo this one path."

**Pick revision when** you want "snapshot this whole subtree as a
unit," "branch this workspace to try something," or "show me the
state of all articles as of last Tuesday."

You can use both together — they don't conflict, and the workbench
default-on configuration wires both.

### 1.1 Revision has two modes — pick one deliberately

Revision is the same handler with the same primitives in both
modes; what changes is when versions get created.

**Manual mode (`auto_version: false`, the default).** Git-style.
You run `revision commit` when you reach a checkpoint worth
preserving. Branches, tags, cherry-pick, revert, merge — same
semantics as git. Use this for **authoring workflows** where a
human decides "this is a useful snapshot." This is the common case
for the workbench; everything in §3 below assumes this mode unless
called out otherwise.

**Auto-version mode (`auto_version: true`).** CRDT-style
collaboration. Every tree write under the prefix produces a
version entry automatically — including writes from checkout,
merge, cherry-pick, revert, and incoming sync from other peers.
The model assumes multiple peers are editing the same subtree
concurrently and converges them through deterministic merges
(`merge_order: "deterministic"` is the default and the only
sensible choice for multi-peer). Use this for **collaborative
real-time editing** (Figma / Google Docs style), append-only
audit logs, or any case where you want the DAG to record every
state transition without manual intervention.

**Don't mix the two on the same prefix without thinking it
through.** Calling `revision commit` while `auto_version: true`
typically produces a redundant entry (same trie root as the
auto-versioned head). The handler has dedup checks that suppress
these, but it's a sign you have the wrong mental model — under
auto-version the versions ARE the per-write entries, not separate
checkpoints.

**The canonical guides are the source of truth:**
- `entity-core-architecture/.../guides/GUIDE-REVISION.md` — manual
  mode, branches, merges, conflicts.
- `entity-core-architecture/.../guides/GUIDE-REVISION-AUTO-VERSION.md`
  — auto-version, sync composition (subscription + continuation),
  merge ordering rationale.

This doc walks through scenarios; refer to those guides when you
need spec-level detail on what a primitive does.

---

## 2. History walkthrough

### 2.1 Recording is off by default

```
$ make shell ARGS="cd local: && put articles/intro test/note '\"v1\"'"
$ make shell ARGS="cd local: && history query articles/intro"
(no transitions recorded for articles/intro — is a config installed?)
```

The recorder is wired but matches no paths until you add a config.

### 2.2 Install a config and start recording

Patterns are exact path or `prefix/*`:

```
> history config articles/*
recording enabled for "articles/*" (config default)
```

The config name (`default` here) is what shows up at
`system/history/config/{name}`. Pass a second arg to use a different
name if you want multiple configs in the same peer.

### 2.3 Drive some writes, see them in the log

```
> put articles/intro test/note '"first"'
> put articles/intro test/note '"second"'
> put articles/intro test/note '"third"'
> history query articles/intro
* rev-3  updated   00d3e1f4a8b2...
  rev-2  updated   009de5e2b687...
  rev-1  updated   0075f97cb4a8...
```

The `*` marks the most recent transition (head). Hashes are short —
the full hex is what `cat -diag` emits and what rollback needs.

The first write counts as `created`, subsequent writes are
`updated`, deletes are `deleted`. You can filter with the typed SDK
client (`HistoryQueryParamsData.Events`) but the shell doesn't
surface that flag yet.

### 2.4 Rollback to an earlier version

You need the full hash. Best source: `cat -diag <path>` against the
path right after each write — its `claimed_hash` field is in the
`ecf-sha256:HEX` form the shell parsers accept verbatim:

```
> cat -diag articles/intro
  type:         "test/note"
  claimed_hash: ecf-sha256:0075f97cb4a8e8d2e8903584ffed3fc42f6264bc2b4d288108dcf7302e1cba222617
  ...

> history rollback articles/intro ecf-sha256:0075f97cb4a8e8d2e8903584ffed3fc42f6264bc2b4d288108dcf7302e1cba222617
rolled back /local/articles/intro → 000075f97cb4...
```

The rollback itself is a recorded transition (event type
`rollback`), so the chain stays complete and you can re-roll
forward by rolling back to the post-rollback hash.

**Hash format note:** the shell accepts three equivalent forms
wherever a hash is expected — `ecf-sha256:HEX64` (the
`cat -diag` form), `00HEX64` (the algorithm-prefixed 33-byte
form `revision log` would emit if untruncated), or bare
`HEX64` (which the shell prepends `00` to). The short form
ending in `...` is rejected — it's display-only.

### 2.5 What's recorded vs not

- Writes via `put` (L1 dispatched) → recorded.
- Writes via `Store.Put` (L0 direct) → recorded (the location index
  fires the event regardless of who issued the bind).
- Writes to system paths (`system/...`) → recorded if the pattern
  matches. The recorder has its own recursion guard for
  `system/history/...` paths (won't record its own writes).

---

## 3. Revision walkthrough

### 3.1 Manual commits work without any config

Useful when you want a snapshot but don't want every write to
trigger one.

```
> put articles/intro test/note '"v1"'
> put articles/about test/note '"about"'
> revision commit articles/ "initial articles snapshot"
committed 0061f2d309b8... @ root 0083fc4a1192...
```

`Version` is the hash you'll feed back to `branch create`,
`checkout`, `cherry-pick` etc. `Root` is the trie root for the
snapshot — content-addressed, so two commits with identical content
share the same root.

### 3.2 Log + status

```
> revision log articles/
* 0061f2d309b8...

> revision status articles/
prefix:    /local/articles/
head:      0061f2d309b8...
conflicts: 0
pending:   0
```

Status reports the current head, conflict count (non-zero means
there's a merge-in-progress), and pending changes (non-zero means
the working tree has writes since the last commit).

### 3.3 Auto-version on every write (CRDT collaboration mode)

`auto_version: true` is the **collaborative editing mode** —
designed for multiple peers editing the same subtree concurrently
with deterministic convergence. **Don't enable it for solo
authoring; manual commits are the right tool there.**

```
> revision config put articles_auto articles/ -auto
wrote config "articles_auto" at system/revision/...

> put articles/intro test/note '"v2"'
> put articles/about test/note '"about v2"'
> revision log articles/
* 0099aabbcc...
  0083fc4a11...
  0061f2d309b8...
```

After the config write, every tree mutation under `articles/`
produces a version entry — including writes from checkout, merge,
cherry-pick, revert, and incoming sync. This is the CRDT
contract: every state transition gets recorded so peers converge
through merge.

**Configuration knobs that matter for auto-version:**
- `merge_order: "deterministic"` — the only safe choice for
  multi-peer. Empty config defaults to deterministic; the
  alternative `"caller-perspective"` causes silent divergence in
  P2P topologies (see GUIDE-REVISION-AUTO-VERSION §3).
- `exclude` — required when the prefix overlaps with system paths
  (`/`, anything containing `system/...`). The config-set handler
  rejects auto-version configs missing the required excludes.
  For a prefix like `articles/` no excludes are required.
- `exclude_types` — entity types to skip even when bound at a
  tracked path.

**Cross-peer sync.** The revision extension provides primitives
(`fetch`, `push`, `merge`); cross-peer sync is a separate
composition of `subscription` + `continuation` that orchestrates
when those primitives fire. Not yet wired in the shell.

**Why intermediate versions appear during checkout / merge.**
Under auto-version, a `checkout` rewrites paths via the normal
tree-write path. The auto-versioner sees those writes and creates
version entries for each. This is intentional — it preserves the
exact state-transition trail so other peers can converge on the
identical DAG. If you don't want this, you're using the wrong
mode; switch to manual.

### 3.4 Branches

**Heads-up:** revision has **no implicit `main` branch.** Until
you call `branch create`, `revision branch list articles/` is
empty even though you have a live head. This differs from git;
the design treats branches as named bookmarks rather than the
primary identity of the head.

```
> revision branch list articles/
(no branches at articles/)

> revision branch create articles/ feature 0061f2d309b8...
created branch "feature" @ 0061f2d309b8...

> revision branch list articles/
  feature              0061f2d309b8...

> revision branch switch articles/ feature
switched to branch "feature"
```

`branch create` from a hash anchors the branch at that historical
version. `branch switch` is a pointer move — it doesn't change the
working tree. Use `revision checkout articles/ feature` to also
swap the tree state.

If you want a "you're always on `main`" feel, create one
explicitly at the start of the project:

```
> put articles/intro test/note '"first"'
> revision commit articles/ "initial"
committed 0061f2d309b8... @ root ...
> revision branch create articles/ main 0061f2d309b8...
```

Then `main` will appear in `branch list` and you can `branch switch`
to it later.

### 3.5 Diff between two versions

```
> revision diff articles/ 0061f2d309b8... 0099aabbcc...
base:      0061f2d309b8...
target:    0099aabbcc...
added:     0
removed:   0
changed:   2
unchanged: 0 (subtree count)
  ~ /local/articles/intro
  ~ /local/articles/about
```

Diff is structural — uses the trie's hash equality to skip
unchanged subtrees. Performance is O(changed paths), not O(tree
size).

### 3.6 Tags for stable references

```
> revision tag create articles/ v1.0 0061f2d309b8...
tagged 0061f2d309b8... as "v1.0"

> revision tag list articles/
  v1.0                 0061f2d309b8...
```

Tags are immutable. To re-point: delete + create.

### 3.7 Checkout = swap the working tree

```
> revision checkout articles/ 0061f2d309b8...
checked out 0061f2d309b8...
```

After checkout, `cat articles/intro` returns the entity from that
version. Cascade warnings (handlers that halt on the rebind, e.g.
auto-versioner deciding "this would create a cycle") are reported
but non-fatal.

To follow a branch: pass the branch name instead of a hash.

```
> revision checkout articles/ feature
checked out 0061f2d309b8... (branch "feature")
```

### 3.8 Cherry-pick and revert

Replays a single version onto the current head:

```
> revision cherry-pick articles/ 0099aabbcc...
cherry-pick 0099aabbcc... → 00aabbccdd... [success]
```

Inverse:

```
> revision revert articles/ 0099aabbcc...
reverted 0099aabbcc... → 00aabbccdd... [success]
```

Both can produce conflicts (same trie path written by two
different ancestors). The result reports `conflicts: N` and lists
the paths; resolve with `revision resolve <prefix> <path> [<hash>]`.

---

### 3.9 Refs everywhere

Anywhere a hash is accepted (`branch create`, `tag create`,
`checkout`, `cherry-pick`, `revert`, `diff`, `show`), the shell
accepts any of:

- `HEAD` — current head pointer for the prefix.
- A branch name.
- A tag name.
- A full hash (`ecf-sha256:HEX64`, `00HEX64`, or bare `HEX64`).
- A short hex prefix (≥4 chars) that uniquely matches one version
  in the log.

Use `revision show <prefix> <ref>` to verify a ref resolves where
you expect:

```
> revision show docs/ HEAD
ref:      HEAD (HEAD)
version:  ecf-sha256:bdc577e8...
root:     ecf-sha256:9de456a4...
parent[0]: ecf-sha256:3fb09656...
```

Show also reports any branches and tags pointing at the resolved
version, useful for "where exactly am I."

---

## 4. Branching scenarios — preserving divergent history

This is the part that makes revision worth using. Without
explicit branches, divergent commits become unreachable from the
current head and disappear from `revision log` (still in content
store, but you'd need to remember the hash).

### 4.1 Always create a branch before you might lose the head

```
> put docs/a test/v 1
> put docs/a test/v 2
> put docs/a test/v 3
> revision branch create docs/ trunk HEAD     # save the tip
> revision branch list docs/
  trunk                00ce869c6c68...
```

`branch create … HEAD` is the equivalent of "remember where I am
right now." Do this before any operation that moves the head
backwards (checkout, rollback, cherry-pick onto a different base).

### 4.2 Diverge, preserve, return

```
> revision checkout docs/ <initial-hash>      # detached, head moves back
> put docs/b test/v 99                         # new commit on a divergent line
> revision branch create docs/ feature HEAD    # name it before we lose it
> revision checkout docs/ trunk                # back to the original tip
> cat docs/a
3
> revision log docs/                           # trunk's history is back
```

Now `feature` and `trunk` both exist as branches, with
divergent commits underneath each. Without those branch creates,
the divergent commits would be in the content store but
unreachable from any visible log.

### 4.3 Cherry-pick across branches

```
> revision checkout docs/ trunk                # I'm on trunk
> revision cherry-pick docs/ feature           # bring feature's tip onto trunk
cherry-pick 007a8dd580bf... → 003971ac521c... [cherry_picked]
> cat docs/b
99
```

The change at `docs/b` from feature is now present in trunk's
history without dragging feature's parent chain along.

### 4.4 Diff branch tips to see what changes

```
> revision diff docs/ trunk feature
base:      ...
target:    ...
added:     1
removed:   0
changed:   0
unchanged: 0 (subtree count)
  + /local/docs/b
```

Useful for "what's on this branch that isn't on that one."

---

### 4.5 Branch operations + auto-version don't mix

If your prefix has `auto_version: true`, every checkout / merge /
cherry-pick / revert produces additional version entries because
those operations issue tree writes and the auto-versioner records
each one. This is **intentional** under the CRDT contract — the
DAG records every state transition so peers converge identically.

The implication for users: **don't use auto-version for prefixes
where you want to do manual branch surgery.** Branching is an
authoring affordance; auto-version is a collaboration affordance.
Pick one mode per prefix and stick with it.

If you're authoring (manual commits, branches, checkouts), keep
`auto_version: false`. If you turn auto-version on for sync, treat
the prefix as no-touch except through the sync flow.

---

## 6. Combined workflow — versioned authoring

A realistic loop:

```
# Set up
> history config articles/*                     # per-path log
> revision config put articles_auto articles/ -auto   # auto-snapshot subtree

# Author a doc
> put articles/intro test/note '"draft"'
> put articles/intro test/note '"first revision"'
> put articles/intro test/note '"final"'

# See per-path history
> history query articles/intro
* ...  updated   00final...
  ...  updated   00first...
  ...  created   00draft...

# See subtree versions
> revision log articles/
* 00v3...
  00v2...
  00v1...

# Tag a stable release
> revision tag create articles/ v1 00v3...

# Mistake found, roll back ONE doc to the first revision
> history rollback articles/intro 00first...
# revision auto-snapshots that, so:
> revision log articles/
* 00v4...   # the rollback recorded as a new version
  00v3...
  00v2...
  00v1...

# Diff to confirm only intro changed:
> revision diff articles/ 00v3... 00v4...
changed: 1
  ~ /local/articles/intro

# Tag still points at v3:
> revision tag list articles/
  v1                   00v3...
```

This is the canonical "authoring with safety net" loop. History
gives you per-doc undo; revision gives you whole-subtree
snapshots; tags pin known-good states.

---

## 7. Known rough edges (today)

These are real. Filing them here as a punch list.

### 7.1 Hash copy-paste is awkward

The shell prints short hashes (12 hex chars + `...`) for readability,
but rollback / cherry-pick / etc. need the full hex. The shell's
hash parser accepts the `ecf-sha256:HEX` form `cat -diag` emits, so
the workflow is:

1. Write the entity.
2. `cat -diag <path>` to capture the full hash.
3. Paste the `ecf-sha256:...` line into the rollback / branch /
   checkout command.

Still awkward — there's no shell sugar for "show me the full hash
from the most recent commit" or "the third entry from the log."

**Fix candidates:**
- `revision log -full` flag → emit full hashes.
- A `revision show <prefix> <ref>` command where `ref` accepts
  short prefixes / branch names / tags / `HEAD~N`.
- A clipboard-style register: `$1 / $2 / ...` referring to recent
  output hashes.

### 7.2 Versioned content is opaque to `ls` / `cat`

`revision log` shows version hashes, but you can't `cat <hash>` to
see what's in a version. The data lives at
`system/revision/{prefix-hash}/...` — discoverable but ugly.

**Fix candidates:**
- A `revision show <prefix> <version>` command that lists the
  version's tree contents.
- A virtual path scheme like `<prefix>@<version>/foo` so `cat`
  works against historical state.

### 7.3 Cross-peer transfer not in shell

The typed `RevisionClient` covers `Push` / `Fetch` /
`FetchEntities` but the shell command surface intentionally
defers them. They need a "remote" abstraction (named remote ↔
peer-id mapping) and a `revision remote add ...` UX.

### 7.4 No version timestamps in `revision log`

Versions are content-addressed and don't carry wall-clock
timestamps in the result type. Auto-version commits do carry the
clock state in the underlying entity, but the typed
`RevisionLogResultData` only surfaces hashes. To get timestamps,
you'd `cat` each version entity individually.

**Fix candidate:** extend the typed client to optionally fetch
included version entities (the envelope already carries them).

### 7.5 Compute extension is wired but has no shell verb

`system/compute` is reachable via `exec`, but there's no
`compute eval / install / uninstall` shell command. Add one when
the first real expression-driven workflow needs it.

### 7.6 `revision status` doesn't expose the active branch

Per GUIDE-REVISION §3.4 the active branch is tracked at
`system/revision/active-branch/{prefix}` and `commit` advances
both the head pointer and the active-branch pointer. But
`RevisionStatusData` (the response shape) doesn't include an
`active_branch` field — to know what branch you're on you have
to call `revision branch list` and look at the `Active` field.

This is a real spec/impl gap, not a workbench bug. The most
natural place for "what branch am I on" is `status`. Filed for
core-go feedback (small spec extension to add `active_branch` to
`RevisionStatusData`).

**Workaround:** call `revision branch list <prefix>` and look at
the `*` marker on the active branch. Or `revision show <prefix>
HEAD` and cross-reference the branch list.

### 7.7 No "reflog" — detached commits are hard to recover

Once you switch away from a detached commit without naming it
(`branch create`), it's still in the content store but
unreachable from `revision log` (which walks from current head).
There's no `revision reflog` that lists "all heads I've recently
been at."

This is consistent with the spec — content-addressed DAG entries
are immutable, and per GUIDE-REVISION §12 garbage collection is
explicitly listed as "Not yet specified." So orphaned versions
aren't lost (they remain in the content store), just unfindable
unless you remember the hash.

**Workaround:** always `branch create … HEAD` before any operation
that moves the head backwards. Branches are single-pointer writes
— effectively free.

### 7.8 Local files

There's no shell-level surface for ingesting from / exporting to
the local filesystem. The KB ingest path in
`workbench/kb_ingest.go` does it programmatically; surfacing it
through the shell is a separate piece of work.

---

## 8. Validating this doc

The behaviors documented here are also pinned by Go end-to-end CLI
tests at `shell/e2e_test.go`. Each test builds the actual
`entity-shell` binary, pipes a REPL script via stdin, and asserts
on the captured output. Run them via:

```sh
cd shell && go test -race -run TestE2E -v ./...
```

The tests exercise:

| Scenario                                           | Test                                           |
|----------------------------------------------------|------------------------------------------------|
| Basic tree ops (sanity floor)                      | `TestE2E_BasicTreeOps`                         |
| Manual-mode commit/log/status (no auto-version)    | `TestE2E_RevisionManualMode`                   |
| Manual-mode checkout doesn't create extra versions | `TestE2E_RevisionManualMode_CheckoutNoExtraVersions` |
| Branch-as-bookmark divergence + return             | `TestE2E_RevisionBranchPreservation`           |
| Universal ref resolution (HEAD/branch/tag/short)   | `TestE2E_RevisionRefResolution`                |
| History config + query                             | `TestE2E_HistoryConfigAndQuery`                |
| History rollback via ecf-sha256: form              | `TestE2E_HistoryRollback`                      |
| Auto-version produces per-write versions           | `TestE2E_AutoVersionMode`                      |

If any of these regress, the corresponding section in this doc has
rotted — fix the test or fix the doc.

For ad-hoc manual exploration, the same scripts also run through:

```sh
make shell <<'EOF'
cd local:
revision config put articles_auto articles/ -auto
put articles/intro test/note '"draft"'
revision log articles/
exit
EOF
```

---

## 9. Reference

- Typed SDK: `entitysdk/revision.go`, `entitysdk/history.go`
- Shell commands: `shellcmd/cmd_revision.go`, `shellcmd/cmd_history.go`
- Spec: `entity-core-architecture/docs/architecture/v7.0-core-revision/sdk-domain/specs/SDK-EXTENSION-OPERATIONS.md` §4 (revision), §5 (history)
- Handler impls: `entity-core-go/ext/revision/`, `entity-core-go/ext/history/`
