# Landscape: what it would take for entity-workbench-go to be the thing you install on your machines

**Date:** 2026-09-01 · **Status:** research + direction, no code committed against it yet
**Question asked:** what does the file-sync landscape look like (Syncthing, rsync, the rest), and
what would it mean for workbench-go to be a filesystem replication tool you install on several
computers?

The short answer is that **we are much closer than the backlog implies, and the remaining gap is
not the one you would guess.** It is not chunking, not watching, not transport, not discovery —
all of that exists. It is *what happens when two machines edit the same file*, and today the
answer is silent data loss by design.

---

## 1. The landscape

Four architectures, and the line between them is not features — it is **what the tool does when
it cannot decide.**

| Tool | Model | Concurrent edit to one file | Notes |
|---|---|---|---|
| **rsync** | one-way delta transfer over SSH | not a question it asks — the destination is overwritten | rolling-checksum delta, ubiquitous, scriptable. A transfer tool, not a sync tool. |
| **Unison** | two-way, stateful reconciliation between two replicas | **detects** divergence, stops, asks the user | OCaml, GPLv3, stable 2.53.7 (Nov 2024). Correct and conservative; pairwise, not a mesh. |
| **Syncthing** | P2P mesh, Block Exchange Protocol | **detects** via version vectors, keeps both, renames loser to `.sync-conflict-…` | blocks 128 KiB–16 MiB, powers of two, fixed within a file, SHA-256 per block. Global model = union of device models, highest change version wins. |
| **git-annex** | content-addressed, git-tracked, location-aware | git's model — merge or conflict markers | tracks *where* large files live rather than putting every file everywhere. |

The thing to take from this table: **nobody merges.** Syncthing's version vectors are genuinely
good engineering — they *detect concurrency* rather than silently picking a winner on timestamp —
but the resolution is to write a second file and make it your problem. Unison stops and asks.
rsync never had the question. The state of the art in file sync is *conflict detection with
manual resolution*, and it has been for twenty years.

That is the opening, and it is where our substrate is unusual.

### Where Syncthing is technically weaker than what we already have

Two places, both structural rather than incidental:

- **Fixed-size blocks.** Syncthing's blocks are powers of two and constant within a file. Insert
  a byte at the front of a large file and every subsequent block boundary shifts, so every block
  changes hash and the whole file re-transfers. Content-defined chunking does not have this
  failure — boundaries are chosen by the content, so an insertion perturbs one chunk.
  **`ext/content/chunker/gear.go` is a Gear-hash / FastCDC chunker and is already the default**
  (`ChunkingFastCDC`, 1 MiB target, 64 KiB min, 8 MiB max — `core/types/content.go:46-53`).
- **Device-level trust.** Syncthing shares a *folder* with a *device ID*. There is an untrusted-
  device mode (encrypted-at-rest), but sharing is not scoped, delegable or revocable in the way a
  capability is. We already mint capabilities per mount —
  `system/capability/grants/chain/local-files/{root}` appears in `shellcmd/cmd_local_files.go:260`.

### Where Syncthing is far ahead

Honesty matters more than the pitch here. Syncthing has a decade of production exposure to the
part of this problem that is genuinely awful: case-insensitive filesystems, Unicode normalisation
on macOS, Windows path semantics, permission and ownership models, sparse files, extended
attributes, atomic rename guarantees, partial-write recovery, and the ignore-pattern semantics
users actually expect. **None of that is interesting and all of it is required.** Any estimate
that treats "sync a folder" as the hard part and these as details has the ratio backwards.

---

## 2. What we already have (the D20 audit)

Priced against the substrate, not against our own tree. Everything below is in
`../entity-core-go` today and we did not write it:

| Capability | Where | State |
|---|---|---|
| Content-defined chunking | `ext/content/chunker/gear.go`, `ext/content/builder.go` | FastCDC, 1 MiB target |
| Content-addressed blob store | `core/store` | the substrate's whole basis |
| File / directory / deletion entities | `ext/localfiles/types.go` — `FileData`, `DirectoryData`, `DirectoryEntryData`, `DeletedData` | typed, CBOR, spec'd |
| Filesystem → tree ingest | `ext/localfiles/operations.go`, `handler.go` | live |
| Tree → filesystem writeback | `ext/localfiles/reverse.go::StartReverseWrite` | live in the kernel — but **nothing in this repo starts it** (M1 finding; see §6) |
| Filesystem watching | `ext/localfiles/watcher.go` | fsnotify-based, debounced |
| Stat cache | `ext/localfiles/statcache.go` (+ `_linux`/`_darwin`/`_windows`) | implements **Git's racy-clean rule** and smudge-to-zero discipline |
| Loop prevention (write-back echo) | `ext/localfiles/reverse.go::reverseTracker` | 5 s window — see risks |
| Path containment / symlink defense | `containment_test.go`, `containment_wire_test.go`, spec §885 | leaf-symlink rejection is a MUST |
| Mount config + restart equivalence | `ext/localfiles/config.go`, `Handler.Load` | persisted |
| Three-way merge, branches, cherry-pick | `ext/revision/` — `ancestor.go`, `branch.go`, `commit.go`, `checkout.go`, `cherry_pick.go` | live, **separate extension** |
| Peer transport, discovery, registry | `core/protocol`, `ext/registry`, rendezvous | live |
| NAT traversal / relay | `ext/relay` | **in flight in their working tree right now** |
| Capabilities scoped to a mount | `system/capability/grants/chain/local-files/{root}` | already minted by our shell verb |

And a normative spec exists: **`specs/domains/DOMAIN-LOCAL-FILES.md`, 1022 lines.**

**What workbench-go has:** `shellcmd/cmd_local_files.go` (`mount`, `unmount`, `mount sweep`,
`mount include|exclude|filter`), `workbench/mount_sweep.go`, `workbench/mount_validate.go`, and
restart-equivalence wired in `shellboot/bootstrap.go:274`.

**What workbench-go does not have:** any renderer surface at all. `grep` for `LocalFiles` across
`avalonia/` and `console/` returns **nothing**. This is a D23 violation of the exact shape the
charter names three times — a complete model with no user-reachable path — and it is the single
cheapest thing on this page to fix.

---

## 2a. The affordance inventory — what a folder-sync tool EXPOSES

§1 and §2 compared architectures and substrate. Neither asked the question an operator actually
asks, which is *what can I do with a folder once I have one*. Added 2026-09-01 after the file
explorer landed, because the honest answer at the time was: mount it, and read a number.

Syncthing's per-folder surface, as its UI presents it:

| What it exposes | Ours | Note |
|---|---|---|
| Folder label + ID + path | partial | root name is derived from the basename; no label, no rename |
| **Folder type** — send&receive / send-only / receive-only / receive-encrypted | **none** | see below — this one is a real hole |
| Ignore patterns, editable, with negation and `#include` | partial | `-include` / `-exclude` globs, basename-matched, `mount filter` shows them. No negation, no file. |
| Ignore permissions | n/a | we do not sync permissions at all |
| Watch for changes + watch delay; rescan interval; full rescan interval | partial | the watcher is always on and its debounce is not configurable from here |
| **Folder status: state, global/local counts, OUT-OF-SYNC ITEMS, last scan, last file received** | **now partial** | the explorer reports total / ingested / not-ingested per file with a reason. Nothing yet reports *scan state* or *last activity*, and watcher liveness is not knowable from the tree at all. |
| File versioning: trash-can / simple / staggered / external | **adjacent** | `ext/revision` is strictly more powerful and is wired by a *separate* verb (`revision config put … -auto`), not by the mount. Not discoverable from any mount surface. |
| Sharing with specific devices; introducer; untrusted/encrypted | **different model** | we mint a capability per mount. Stronger in principle, and there is no sharing UI, so in practice it is unreachable. |
| Pull order, copiers/hashers, weak-hash threshold, sparse files, modtime window | none | genuinely advanced knobs; correctly absent for now |
| Minimum free disk space | none | a real operational guard we have no answer for |
| Per-device and global rate limits | none | transport-level, not mount-level |

**The one to fix first is folder type.** `localfiles.RootConfigData.ReadOnly` exists in the
kernel and is honoured by the writeback path — and **nothing in this repo can set it.**
`shellcmd/mount_op.go` hard-codes `ReadOnly: false`, the `mount` verb has no `-readonly` flag
(its flag set is `-include` / `-exclude` / `-force`), and the panel form has no checkbox. So
Syncthing's *"Send Only"* — the most-used non-default folder type, and the one an operator
reaches for the first time they mount a directory they do not want written to — is expressible
in the substrate and unreachable from every surface we ship. `PublishDescriptors` is in the same
position.

That is D23's shape one layer out, and it is the shape the M1 finding already named about
`StartReverseWrite`: **a capability that exists in a dependency is not a capability of your
product until something in your tree calls it.** The difference is that this one is a field
rather than a function, which is why the D23 sweep cannot see it — there is no unreached model
of *ours* to find.

**What we have that they do not, and should stop hiding.** Content-defined chunking (§1),
capability-scoped sharing, and `ext/revision`'s three-way merge. All three are real advantages
and none of them appear in any mount surface, which makes them advantages on paper.

---

## 3. The gap that actually decides this

`DOMAIN-LOCAL-FILES.md` §35, verbatim:

> Concurrent writes from multiple peers to the same `local/files/...` path are **NOT a CRDT use
> case for this domain**. The §5 reverse-write convergence model is substrate-level tree
> convergence (**last-arrival wins** under the §5.5 circuit-breaker; both writes hashed
> identically as blobs and recorded at distinct chain positions; **the filesystem reflects the
> last write to land**). This is intentional. **For concurrent collaborative edits, use
> `EXTENSION-REVISION` instead.**

And §39–43: *"No automatic merge — bytes are not three-way-merged at the substrate level.
Operators wanting merge semantics layer it via the revision extension."*

The implementation matches: `grep -i conflict ext/localfiles/*.go` returns **nothing**.

Read that as a product statement and it says: *if you edit a file on your laptop and your desktop
while both are offline, one of those edits disappears when they reconnect, and nothing tells
you.* Syncthing, which is the thing people would compare us to, has not lost that write since
version vectors landed — it keeps both and renames one.

**This is not a bug in the spec.** The spec is right that the *substrate* should not merge bytes,
and it explicitly names where merge semantics belong. It is a gap in the **product**, and the
product layer is us.

---

## 4. The thesis — what would actually be novel

The spec's own pointer is the design:

> **`local/files` gives you the filesystem surface. `ext/revision` gives you three-way merge,
> branches and history. Nobody has composed them.**

That composition is a file replicator where a concurrent edit produces **a real merge, or a
first-class versioned conflict with both parents addressable and history intact** — not a
`.sync-conflict-20260901-121500.txt` sitting next to the original with no relationship to it
that any tool understands.

Stack the rest of the substrate on top and the differentiated claim is:

1. **Content-defined chunking**, so an insert near the top of a large file costs one chunk, not
   the file. (Syncthing: fixed blocks, whole-file re-transfer.)
2. **Merge instead of surrender** on concurrent edit, via `ext/revision`. (Everyone else: detect
   and defer to the human.)
3. **Capability-scoped sharing** — share one subtree with one peer, revocably, delegably, rather
   than pairing a whole device with a whole folder.
4. **Signed roots**, so "this is what my other machine actually published" is verifiable rather
   than assumed from a device ID.
5. **Real history**, because the revision log already exists — not Syncthing's local-only,
   best-effort file-versioning bolt-on.

That is a genuinely new point in the design space and it is reachable, because four of the five
already exist and only #2 needs building.

---

## 5. Honest risks

- **The 5-second loop-prevention window is the scariest thing in the tree — and it is not scary
  for the reason first written here.** `reverseTracker.recentWriteWindow = 5 * time.Second`
  (`ext/localfiles/reverse.go`) suppresses a **tree change** event if the same path was written to
  disk recently. This section originally said it suppresses a *filesystem* event and therefore
  discards a user's own edit; **that was wrong**, and the M1 reproducer measured it — the watcher's
  ingest path never consults the tracker, so a user edit inside the window reaches the tree
  normally. What is lost is the other direction: a second genuine tree update inside five seconds
  never reaches disk, and a tree **delete** inside five seconds never reaches disk *and has no
  content check behind it*. Both are permanent — the event is discarded, not deferred. The real
  defect is worse than the one described here first: a lost operator edit is at least visible to
  the operator, a tree update that never lands is visible to nobody. Reproducer:
  `workbench/localfiles_reverse_window_test.go`. Routed upstream, with the correction attached.
- **The boring 80%** — case-insensitivity, Unicode normalisation, permissions, atomic rename,
  partial writes. Unglamorous, mandatory, and where the schedule actually goes.
- **We would be competing with a mature product on its home turf.** The differentiator has to be
  the merge story and capability-scoped sharing. On "sync a folder between two Linux boxes",
  Syncthing wins on maturity and will for a long time.
- **`ext/relay` is unlanded.** Cross-NAT sync without it means same-LAN or manual addressing.
  Their working tree has it dirty today; sequencing matters and is theirs.
- **Nothing here is conformance-gated yet.** Spec §993: conformance vectors for this domain
  "emerge as a Stage-4 byproduct" and Phase 4 has not started.

---

## 6. Proposed sequence

Ordered so each step produces something usable and the risky claim gets tested early.

**M0 — surface what we already have.** A `LocalFilesPanel` (mounts, their roots, filter patterns,
sweep results, watcher state) plus the `mount` verbs reachable from the GUI. Closes a standing
D23 violation, costs days not weeks, and makes every later step demonstrable.

**M1 — replace the clock with content. Our half is done; theirs is open.** The reproducer is
`workbench/localfiles_reverse_window_test.go` (six arms, ~8 s, no network), and the ask has been
routed upstream with it. The finding that makes it a small fix
rather than a design question: **the content check already exists**, twenty lines below the clock
in `reverseWrite`, and it stops the echo on its own — measured with the clock provably disarmed.
`markWritten` sits *downstream* of that check, so the clock cannot arm on an echo; it arms only
after a real tree→disk write, which is exactly when the next event is most likely to be a genuine
follow-up. Armed when harmful, disarmed when redundant.

Two things M1 turned up that change this page's own arithmetic:

- **`StartReverseWrite` has no caller in this repo** — one non-test caller in either tree, and it
  is core-go's `cmd/entity-peer/main.go`. Our peers replicate through the subscription chain into
  `local/files:write`. So the tree→disk direction that §2's table calls "live, **bidirectional**"
  is live *in the kernel* and **not started by anything workbench-go ships**. M2 has to wire it (or
  keep replicating through dispatch and say so), and that is a decision this page had not noticed
  it was making.
- The window defect is **dormant for us and live for `entity-peer`**, which is why routing it did
  not block on us adopting it.

**M2 — two machines, one folder, end to end.** Two peers, one mount each, capability-scoped, over
the live transport. Non-concurrent edits only. This is the first point at which the thing is
*real*, and it will surface the boring 80% in priority order rather than by guesswork.

**M3 — the novel claim, tested.** Concurrent edit to one path on two disconnected peers, then
reconnect. Today: one write vanishes. Target: `ext/revision` three-way merge where the file
merges, and a first-class versioned conflict — both parents addressable, history intact — where
it does not. **This is the milestone that is worth writing a paper about, and the only one that
is not catch-up.**

**M4 — the product.** Daemon/service install, per-OS packaging, ignore patterns matching user
expectation, and the filesystem edge cases enumerated rather than discovered.

M0 and M2 are catch-up. M3 is the reason to do it at all — so the honest sequencing question is
whether to pull a reduced M3 forward and prove the merge composition on two mounts *before*
investing in M4's packaging work.

---

## 7. Provenance

`entity-core-go` read read-only, `dev` at the time of writing, with its working tree dirty in
`ext/relay` — so line numbers here may drift and symbols are the durable citation.
`entity-system-architecture` read read-only. Landscape sources: Syncthing BEP v1 documentation,
Unison project documentation, and 2026 comparison roundups. No claim here about a competitor's
internals is sourced from memory alone.

§5 and §6 were corrected on 2026-09-01 by the M1 reproducer
(`workbench/localfiles_reverse_window_test.go`), which measured two claims this page had asserted
from a source reading and got one of them backwards. Both corrections are marked in place.
