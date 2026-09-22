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
| Tree → filesystem writeback | `ext/localfiles/reverse.go::StartReverseWrite` | live in the kernel; **we deliberately do not start it** — M2 replicates through the subscription chain into `local/files:write` instead (§6) |
| Cross-peer materialization | `workbench/blob_resolve.go` (**ours**) | live, and reachable from `sync` as of 2026-09-02 — before that, twelve test files and no registration in any binary |
| Subscription rehydration at open | `ext/subscription/engine.go::Load` | live in the kernel; **we did not call it until 2026-09-02**, so every subscription — and therefore every mount — was dead after a restart (AP62) |
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

**M2 — two peers, one folder, end to end. DONE 2026-09-02 for two peers on one host over the
real transport;** two physical machines is the remaining half. `sync <peer> <root>` /
`unsync` / `syncs`, the durable binding at `app/workbench/syncs/{peerID}.{root}`, and
`shellboot/sync_e2e_test.go` — both peers built through the real bootstrap, connected over TCP, a
file written on one appearing on the other's disk, then a second file to distinguish "the first
delivery worked" from "the relationship is live".

It cost far less than this page priced it, for a reason worth recording: **the engine was already
built and already tested, and nothing could reach it.** `workbench.BlobResolveHandler` — the whole
cross-peer materialization chain — had twelve test files behind it (one-way, bidirectional, burst,
4 MB file, cap delegation, late join, self-loop) and **no registration outside them**. `shellboot`
registered two handlers and this was not one; `subscription` is `ls|inspect|rm` with no create. So
M2 was a wiring job, not a build. That is D23 at the handler layer, where `make reachability`
does not look because it asks whether a *model* has a surface.

Two corrections to this page's own arithmetic fell out of doing it:

- **The tree→disk direction question is settled, and the answer is "not `StartReverseWrite`".**
  We replicate through the subscription chain into `local/files:write`, which is what all twelve
  tests exercise and what the shipped `sync` verb now does. The kernel's reverse-write loop stays
  uncalled here, deliberately.
- **Bidirectional sync was never the blocker this page implied.** F9 — the asymmetry that made
  "only one-way mirror topologies work cleanly" — was closed by core-go at `8ad52bc` and the skip
  came off `TestStage3_Case2_Bidirectional`. That test's own header still said
  *"CURRENTLY SKIPPED"* and *"fully symmetric peer-to-peer does not [work]"* while passing every
  night, which is how the constraint survived in our planning long after it stopped being true.
  Corrected in place.

**What M2 does NOT cover, named rather than implied:** two physical machines (this is loopback
TCP; the addressing and NAT story is untouched and `ext/relay` is unlanded upstream), any GUI
affordance (the verbs are shell-only — the Local Files panel manages mounts and says nothing about
syncs), and history replay (a sync delivers changes from the moment it is established; files
already sitting in the remote mount arrive when they next change or when that peer remounts). The
verb prints that last one rather than letting an operator watch an empty directory and conclude it
is broken.

**M2a — the flow, and the permission stage nobody had tested. DONE 2026-09-02.**
`peers` / `share` / `offers` / `accept` / `access` / `unshare`, validated end to end across two
peers with **no wildcard grants** (`shellboot/flow_e2e_test.go`), operator recipe in
`USAGE-SHARE-A-FOLDER.md`.

The finding that reframes §2a's affordance table: **every cross-peer test in this repo ran under
`peer.OpenAccessGrants()`**, so the sync work to this point had validated the transport and
nothing about authorization. Turning the wildcard off surfaced four constraints — a sync is
*mutual* authorization; the grant is assembled at handshake; the peer that dispatches is the one
that must reconnect; and a dial-by-address authorizes the dialer only, so both peers must dial.
None are visible under a wildcard and each is a lost afternoon on real hardware. AP63.

Two rows in §2a's table move as a result. **"Sharing with specific devices"** is no longer
"different model, unreachable in practice" — it is `share`/`accept` over the kernel's per-peer
policy table, which is genuinely finer-grained than pairing a device with a folder. **"Folder
status"** gains `access`, `shares` and `syncs` as inspectable surfaces.

**M2b — "the receiver binds nothing". WITHDRAWN 2026-09-07; there was no such defect.**
It was inserted on 2026-09-06 ahead of M3 on a measurement that read a `podman cp` of a live
SQLite store — WAL, sidecar not copied, so the receiver's recent bindings were simply absent
from the file being queried and came back as zero rows with no error (AP76). The receiver
binds correctly in all four configurations that matter (memory/sqlite × symmetric/asymmetric
root names), gated now by `shellboot/receive_binds_e2e_test.go`. Four conclusions that rested
on it are void with it: F9 is *not* dead code, `resync`'s already-current *does* fire (an
existing green gate asserts `AlreadyCurrent == 4`), and neither `Mode: both` nor
first-change-after-restart is explained by it — both are open again on their own terms.

What was real, and is the reason the row is kept rather than deleted: **every sync test in
this tree asserted bytes on disk and none read the receiver's tree.** The tree-side half of
the receive path had zero coverage, which is why a claim that it did not work at all was
consistent with a fully green suite.

**M3 — the novel claim, tested. RESCOPED 2026-09-07, and it is much smaller than this page
priced it.** The measurement nobody had run
(`shellboot/concurrent_edit_baseline_test.go`): with history recording enabled on the mount
prefix, a concurrent same-path edit already produces exactly what `DOMAIN-LOCAL-FILES` §1.1a
rules — last arrival wins on disk, **both writes at distinct chain positions**, and the
overwritten bytes byte-recoverable from the chain:

```
[0] updated  local/files:write   ← the delivered edit; won on disk
[1] updated  local/files:watch   ← the receiver's own edit, preserved
[2] created  local/files:write   ← the seed
```

Two things follow. **The tree-side guarantee is already met by the substrate** — it had simply
never been switched on, because history recording is opt-in per path
(`ext/history/config.go`, `configCache.find`) and nothing in the mount / sync / share path
installs a config. With none, a query returns empty *and no error*, which reads as "the tree
kept nothing". And **the provenance discriminator conflict detection needs already exists on
every transition**: a local edit arrives through the WATCHER (`local/files:watch`), a
delivered one through `blob_resolve`'s dispatch (`local/files:write`), so "they edited this"
and "I am behind" are distinguishable today at zero cost.

So M3 is not "build a merge engine". In order: **(1)** install a history config for a mount
prefix at mount time, so the chain exists for the namespace whose whole point it is;
**(2)** branch at `blob_resolve.go`'s existing F9 *different* arm — equal means current,
different plus a local `:watch` position since the last delivery means conflict; **(3)** write
a keep-both copy on conflict under the spec's own `{path}.keep-both-{hash8}` naming, which is
what Syncthing / Dropbox / OneDrive / iCloud all do; **(4)** layer `EXTENSION-REVISION` for
real three-way merge, whose commit/log/status half is already proven to work over a
`local/files` prefix. Each strategy has a distinct observable outcome, so gate each on bytes
on disk **plus** `revision status`'s conflict count, with an anti-vacuity arm distinguishing a
strategy that resolves from one that degrades.

**The paper-worthy claim survives the rescoping** — a content-addressed tree makes both
parents of a conflict permanently addressable with no side-car format, which is the thing no
comparable product offers. What changed is that we are wiring it up rather than inventing it.

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
