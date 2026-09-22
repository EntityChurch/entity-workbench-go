# Mounts & local files

The two layers of a mount, what persists where, watcher liveness, history recording, and the tree-to-disk direction we do not run.

> Memory, per `AGENTS.md`. Every entry below was paid for by a defect that shipped or
> a measurement that contradicted what we believed. Find by symptom; the mechanism is
> stated before the fix, and `file:line` references point at the code, not at prose.

---

- **A mount has TWO layers and a single count reads the one that cannot fail** (AP59). The
  watcher writes `local/files/{root}/{rel}` for every admitted file; the ingest chain lifts each
  into a `doc/*` at the target prefix. The mount row's `FileCount` read the *source* layer, so a
  directory of 400 photographs and one README displayed "401 entities in tree" with exactly one
  openable document. Report **both sides of a lossy stage** — `FileExplorerModel` carries
  `TotalFiles` / `Ingested` / `NotIngested` — and make a per-item absence carry its **reason**,
  never a blank column.
  **What a file becomes is `workbench/doc_types.go`**, an extension→type registry:
  `doc/markdown-file` (unchanged and byte-identical to what it always was — see
  `doc_file_data.go` for why markdown keeps its own struct), plus `doc/text-file`,
  `doc/code-file`, `doc/image-file`, and `doc/binary-file` as the honest fallthrough. There is no
  "unhandled" outcome any more. Classification is by **name only, never by content**: reading a
  4 GB video's first chunk to learn what its extension already said is a cost with no answer at
  the end of it. `ValidateMountTarget` must be passed `DocEntityTypes()` — the whole set — or a
  remount conflicts with its own predecessor's output.
- **The workbench half of a mount is persisted at `app/workbench/mounts/{root}`**
  (`workbench/mount_binding.go`) and restored by `shellboot` at startup. The kernel persists the
  RootConfig and rehydrates the watcher; the **source→target mapping the ingest handler routes on**
  is ours, and until this landed it existed only in `NotificationIngestHandler`'s memory. After a
  restart the watcher came back, wrote its file entities, and every delivery answered
  `404 no_mount_for_uri` — a mount that listed as healthy and had silently stopped producing
  documents. `Mount` writes it, `Unmount` removes it (or the unmount undoes itself at the next
  launch), and every failure path in between unwinds it.
- ⛔ **A GATE THAT ASSERTS AN ABSENCE CANNOT TELL THE BLOCKER FROM YOUR OWN FAILURE TO ADOPT THE
  FIX** (2026-09-17, on core-go row 3 — watcher liveness). This file already carries the rule that
  *a blocked-state gate that keeps passing after the blocker lifts is the one failure it must not
  have*, earned on the row-22 tripwire. **That tripwire worked**: it pinned a `403`, so when the
  blocker lifted the `403` stopped arriving and the test went red on the same day, exactly as
  designed. This one did not, and the difference is the whole entry.
  `TestLocalFilesModel_DoesNotClaimWatcherLiveness` asserted `WatcherObservable == false`, with a
  message explaining that watcher state *"is a `watch` response and is never written to a tree
  path"*. core-go made it a tree fact — `Handler.persistWatcherState` at
  `system/config/local/files/watch/{root}`, on **every** start/stop/error path, which is the
  better half of what we asked for. The field stayed `false`, because nothing taught the model to
  read the record. **So the gate went on passing, and it was passing on OUR inaction.**
  ⭐ **The discriminator: pin a fact the OTHER side controls, never a field YOU control.** A
  refusal, a status code, a missing key — those stop happening when somebody fixes them. A
  boolean you set to `false` yourself is invariant under the fix, so the gate keeps reporting
  *"still blocked"* forever and reads, to the next session, as evidence that it is. Three
  documents inherited the false sentence from the test's own message, including a published one.
  **When you must pin a field you own, assert the UPSTREAM CONDITION beside it** — here, *the tree
  carries no watch record for this root* — because that arm fails the day the record appears.
  Fixed: the model reads it, and the replacement gate's load-bearing arm is the one asserting a
  row **does** report a status when the tree carries one (mutation-checked both ways).
  Two things the adoption cost that are worth knowing. **`cmdMounts` was counting entities in the
  config namespace**, so the new watch records made a peer with two mounts report *"mounted roots:
  4"* while listing two — the per-row loop already filtered nested paths, so the count and the list
  disagreed with each other, which reads as a tree problem rather than a counting one. It goes
  through `LocalFilesModel` now; **two surfaces answering one question had two copies of the
  namespace filter and only one of them learned**, which is this file's own *DRY the integration,
  not the renderer* rule arriving late. And **there are THREE states, not two**: absent record is
  where a mount whose watcher never started lands, and it is not `stopped` — collapsing them sends
  an operator to the wrong machine, so `WatcherStatus` is empty exactly when `WatcherObservable` is
  false and no renderer may default it.
  ⚠ `ext/localfiles` exports `RootConfigDataFromEntity` and **no** `WatcherConfigDataFromEntity`
  beside it (the type was only ever an operation response), so the decode is ours — their idiom
  verbatim, named as a small ask rather than worked around in silence.
- **HISTORY RECORDING IS OPT-IN PER PATH, AND NOTHING IN THE MOUNT/SYNC/SHARE PATH TURNS IT
  ON** — which is the whole distance between where M3 is and where it needs to be. The
  recorder tracks a path only when a `system/history/config` entity matches it
  (`ext/history/config.go`, `configCache.find`); with no config a query returns **empty and no
  error**, which reads exactly like "the tree kept nothing". `DOMAIN-LOCAL-FILES` §1.1a — our
  own WB-25 closure — rules that a concurrent same-path write is last-arrival-wins at the FS
  surface with **both writes recorded at distinct chain positions**, and *the substrate already
  delivers that in full the moment recording is enabled.* Measured
  (`shellboot/concurrent_edit_baseline_test.go`), receiver's chain after a concurrent edit:
  `[0] updated local/files:write` (the delivery, which won on disk) · `[1] updated
  local/files:watch` (the receiver's own edit, **preserved and byte-recoverable**) ·
  `[2] created local/files:write` (the seed). **The `handler`/`operation` on a transition is
  the provenance discriminator conflict detection needs** — a local edit arrives through the
  WATCHER, a delivered one through `blob_resolve`'s dispatch. So M3 is **not** "build a merge
  engine": the tree-side guarantee is already met and had simply never been switched on.
  **BUT THAT READING IS TRUE AT THE INSTANT OF DELIVERY AND ERASED SHORTLY AFTERWARDS, AND THE
  PARAGRAPH ABOVE USED TO SAY OTHERWISE.** Every delivered file acquires a `local/files:watch`
  transition a moment later: the receiver's own watcher ingests the file `blob_resolve` just
  wrote to disk, and a file entity carries `modified_at`, which the watcher reads from the
  filesystem rather than from the write — same bytes, different entity, real transition. So the
  HEAD says `watch` for **every** file in a received folder, and the obvious detector built on
  it flagged all of them. The baseline test's `trans[0].Operation == "write"` assertion passes
  because it reads within a second of the delivery; it is a measurement taken at one moment,
  read as a property. **The question that survives is *who last changed the BYTES***:
  `workbench.localEditAwaitsDelivery` walks the run of consecutive transitions carrying the
  current content hash and reads the OLDEST member's operation, because an mtime-only echo
  lengthens the run and cannot change its oldest member. Known limit, unchanged: a local caller
  dispatching `local/files:write` directly records as a delivery — which is why `resolve -keep
  mine` writes through the **filesystem**. The generalisation is the part to carry: **a fact
  established by reading a mutable structure once is a fact about that instant**, and the tell
  is a test that reads immediately after the event it is about. **`shellcmd/folder_history.go` switches
  it on**, as a derived output of the reconciler — a folder that `Receives()` and is mounted
  gets a config for its mount prefix. `Receives()` and **not** `IsLocal()`, and here the two
  genuinely differ: `share` declares the OWNER's folder `both`, so either side of a shared
  folder can be overwritten and both record, while keying on `IsLocal()` would leave the owner
  — whose files these actually are — as the one side with no chain. A **send-only** folder
  gets none, because one writer means an entity per save forever answering no question. What
  is left of M3 after 2026-09-07 is `EXTENSION-REVISION`, and nothing else in this list. This is
  D20 aimed at the kernel one more time: **grep `../entity-core-go` for the mechanism before
  pricing the build.**
- **Nothing in this repo starts the tree→disk direction of a `local/files` mount** (M1 finding,
  2026-09-01). `localfiles.Handler.StartReverseWrite` has **one** non-test caller in either tree
  and it is the kernel's own `cmd/entity-peer`. Our peers replicate through the subscription chain
  into `local/files:write` instead, so a mount here is watcher-ingest plus dispatched writes — not
  the kernel's reverse-write loop. Do not read the kernel's *"bidirectional"* as a statement about
  us; it was, in a doc of ours, for a day. **A capability that exists in a dependency is not a
  capability of your product until something in your tree calls it** — D23 aimed one layer out,
  and the D23 sweep does not look for it because there is no unreached model of *ours* to find.
  ✅ **The reverse-write loop's five-second clock is GONE, fixed by core-go 2026-09-17** (tracker
  row 2), and this bullet described it as a live defect until then. `reverseTracker` is removed
  **entirely**: the content check is the sole echo authority, which is exactly what our packet
  argued — *the check the clock was standing in for already existed, and `markWritten` sat
  downstream of it, so the clock could only arm when it was harmful.* They verified against our own
  F9 self-loop repro (bounded at 4 entities against ~2200 unfixed).
  `workbench/localfiles_reverse_window_test.go` is **no longer a reproducer**: its three
  defect-asserting tests fired on the fix and were **replaced with the property** (a genuine second
  update lands; it lands promptly; a delete inside a burst lands), with the content-identity arm
  re-cast as their anti-vacuity control — *"a second write lands"* is satisfied just as well by a
  build with the echo guard deleted, which is a peer in a write loop with itself. Read the file
  header; it keeps the whole history and the reason the tripwires were replaced rather than deleted.
