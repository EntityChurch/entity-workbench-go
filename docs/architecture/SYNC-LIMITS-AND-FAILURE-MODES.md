# Folder sync: measured limits and failure modes

What this actually does under load, what breaks, why, what an operator sees, and what to do
about it. Every number here is measured, and the command that produces it is named. Nothing in
this page is an estimate.

Reproduce: `make loadtest` (burst characterisation), `make perfreview ARGS="-run
TestFeatureCost"` (per-write feature cost), `make twopeer-sync` (two containers, real TCP).

---

## 1. The headline, in one table

| Question | Answer | Measured by |
|---|---|---|
| How fast does a file reach the other machine, live? | **~2.3 s fixed + ~0.36 ms/file** (≈2,800 files/s marginal) | `TestLoad_QueueDepthSweep`, 2000 in 3.0 s and 10,000 in 5.9 s |
| How fast does the catch-up path move files? | **~1,900 files/s** | `loadtest`, 1324 files in ~700 ms |
| What does a catch-up pass with nothing to do cost? | **0.24 ms/file** (2000 files in 472 ms) | `loadtest` |
| What does a shared file cost in the store? | **~3 entities** on the receiver, ~2 on the sender | `loadtest` |
| What does a file cost in DELIVERY QUEUE SLOTS? | **6.5–8.2 slots**, not 1 | `TestLoad_QueueDepthSweep` |
| What does change-recording cost per write? | **2 entities, ~1.2 KB, +0 latency growth** | `TestFeatureCost_HistoryRecording` |
| What does revision auto-versioning cost per write? | **~4.5 entities, and latency grows with store size** | `TestFeatureCost_RevisionAutoVersion` |
| Does a big directory copy complete? | **Only if the delivery ring is big enough for it, and no ring is big enough for every burst.** §3. | `loadtest` |

**Corrected 2026-09-07: this table used to claim "live delivery runs ~90 files/s" and "the
catch-up path is ~20× faster than the live path".** Both were wrong, in a way worth keeping on
the page rather than quietly deleting, because the mistake is easy to repeat. The 90 came from
a 200-file run, and a 200-file run is ~2.2 s of which essentially all is fixed setup — so it
divided an operation's fixed cost by its file count and called the result a rate. Measured
across two burst sizes, live delivery costs **~0.36 ms per file** at the margin, which is the
same order as the catch-up path, not twenty times worse.

**That retires the reason we gave for the design, and not the design.** Leaning on catch-up is
right because §5.5 of `EXTENSION-SUBSCRIPTION` makes notification delivery *best-effort* — a
subscriber that does not periodically reconcile will lose things, at any speed. It is not right
because catch-up is faster. The two arguments look alike and only one survives a measurement.

---

## 2. Change recording is linear and flat. Auto-versioning is not.

These are two different features and conflating them is the expensive mistake.

**Change recording** (`system/history`, what a shared folder turns on — see
`shellcmd/folder_history.go`):

| Writes | Entities | DB | p50 | p99 |
|---|---|---|---|---|
| 1,000 | 2,356 | 1.3 MiB | 141 µs | 415 µs |
| 25,000 | 50,356 | 29.3 MiB | 155 µs | 476 µs |
| 100,000 | 200,356 | 116.8 MiB | 165 µs | 640 µs |

**Latency does not grow.** 141 µs at a thousand writes, 165 µs at a hundred thousand. Cost is
a flat 2 entities and ~1.2 KB per write. This is affordable, and it is why recording is on for
a folder that receives.

**But it is unbounded.** Nothing prunes a chain. The steady-state cost is *per write*, not per
file, so the number that matters is your write RATE, not your file count:

| Workload | Writes/day | Growth |
|---|---|---|
| Documents folder, a few dozen saves a day | ~100 | ~120 KB/day |
| Active project directory | ~5,000 | ~6 MB/day |
| **A log file being appended** | **~1,000,000** | **~1.2 GB/day** |

**So: think before sharing a directory that contains a file something is continuously
rewriting.** A log stream, a database file, a `.sqlite` journal, an editor's swap file, a build
output directory. The failure is not dramatic — no crash, no error — the store simply grows
until the disk is full.

### The guard: recording STOPS for a runaway path, and says so

Since 2026-09-07 that growth is bounded per path rather than unbounded
(`shellcmd/history_budget.go`). Every recorded transition writes a head pointer at
`system/history/head/{tracked-path}`, which is an ordinary tree mutation, so a prefix watch
delivers **exactly one event per recorded transition** for the price of a map increment. When
one path passes `DefaultHistoryPathBudget` (**2,000** recorded versions, ~2.4 MB) the guard
writes a disabled exact-path `system/history/config` entity and a durable record at
`app/workbench/history-limits/{key}` saying which path, at what depth, against what budget, and
when.

**The exclusion is surgical because the kernel's own specificity rule makes it so.**
`EXTENSION-HISTORY` §6.2 orders configs by literal-segment count and `configCache.find` returns
nil when the most specific match is disabled, so an exact-path config outranks the folder's
`local/files/{root}/*` and stops that one path while every file beside it keeps recording.
Measured end to end against the real recorder in
`shellboot/history_budget_e2e_test.go`, with a control arm that runs the same workload with no
guard and asserts the chain keeps growing.

**State the trade rather than burying it: stopping keeps the OLDEST positions and loses the
newest, which is backwards for recovery.** That is acceptable for the workload it catches —
nobody wants the version history of a log file — and it is why a tripped limit is a *problem*
line on every status reading rather than a silent internal event. The two answers are the
operator's: take the file out of the shared folder, or re-enable the named config by hand. The
guard acts once per path and never re-applies, so an operator who overrides it is not undone by
the next pass.

**Why stopping and not pruning.** There is no mechanism below us that makes an existing chain
smaller — see the subsection below — and a transition is not in the location index at all, only
the head pointer is, so there is nothing to remove and no collector to remove it.

**What it does NOT bound: the aggregate.** A folder of N files can still reach N × budget,
because the guard acts on the shape that grows without limit in TIME and a per-file chain grows
with the data. That case is proportional to what the operator copied in. Surfaces: the `status`
verb prints the counter and every tripped path; the GUI's *Sharing Status* panel has the same
three lines. Turn it off with a negative `Config.HistoryPathBudget`, which is the only way to
re-measure the unbounded growth.

### The bound the substrate appears to offer does not work — measured, do not plan around it

`types.HistoryConfigData` carries `MaxDepth *uint64`, documented **"Max transitions per path;
nil = no limit"**, and the recorder calls `prune(path, maxDepth)` after every transition. That
reads like a bound we had simply not set, and it is the obvious first move on this whole
problem.

**It prunes nothing.** Measured (`shellboot/history_maxdepth_probe_test.go`): `max_depth = 3`,
twelve serialized writes to one path, **twelve transitions still reachable from the head**.
`prune` walks the chain to the `max_depth`'th transition and returns, having written nothing —
and it cannot easily do otherwise, because transitions are immutable content-addressed
entities, so severing a link means rewriting every retained transition and the head pointer. Its
comment says the older transitions become "no longer reachable" and that "GC handles cleanup";
neither holds, and there is no garbage collector in the cohort at all.

Setting `MaxDepth` today is a **pure cost**: an O(max_depth) content-store walk after every
recorded write, achieving nothing.

Routed as `docs/outbox/CORE-GO-HISTORY-MAXDEPTH-PRUNES-NOTHING-2026-09-07.md`. **The consequence
for planning is that the cheapest fix is not available**, so a workbench-tier guard is real work
rather than a config change — probably a per-folder budget that disables recording and *says
so*, since a silent disk-fill is the worse failure. The probe test is kept, and inverts: if
`max_depth` ever starts working it fails and tells you to take it.

**Revision auto-versioning is a different animal and must not be turned on for a mount:**

| Writes | Entities | p50 | p99 |
|---|---|---|---|
| 100 | 684 | 452 µs | 1 ms |
| 1,000 | 4,290 | 1 ms | 1 ms |
| 2,000 | 8,480 | 2 ms | 3 ms |
| 5,000 | 22,368 | **7 ms** | **15 ms** |

Latency grows **15× over 5,000 writes**, because each write recomputes a trie root over the
whole prefix. GC count went from 141 to 2,922 across the same run. Extrapolated, a 50,000-file
directory would be writing at tens of milliseconds per file — an hour-plus for a copy that
takes seconds otherwise.

**Conclusion, and it decides an API question: auto-versioning is not something to expose as a
per-folder switch.** It is correct, it is useful on a small curated prefix an operator chooses
deliberately, and it is a trap as a checkbox next to a shared folder. `revision` stays a verb
you point at a prefix on purpose.

---

## 3. FAILURE MODE 1 — a big directory copy stops part way, silently

**This is the one an operator hits first, and it was live until 2026-09-07.**

### What happens

Drop 2,000 files into a shared folder at once. Measured:

```
sender:   2000 files ingested, 2000 bound in its tree
receiver: 676 files
sender's dropped-delivery counter: 2327
status:   healthy.  syncs: healthy.  chain errors: none.
```

The copy stops and **stays** stopped. Nothing retries it. Nothing reports it.

### Why

The subscription engine's delivery queue is **bounded and drops on full** — deliberately, to
avoid a deadlock (`ext/subscription/engine.go`, `OnTreeChange`). The kernel counts every drop.

The drops happen on the **sending** peer, before anything reaches the network. So the receiver
is not "behind" — it was never told. There is no failed delivery to retry, no error to
surface, and no counter on the receiving side that moves. Both peers are, from their own point
of view, working correctly.

### What was wrong on our side — two things, and the second was the bigger one

**Nothing read the kernel's counter.** Not in shipped code, not in a test. The one number that
explains the failure was being maintained for us and thrown away.

**And we had sized the queue ourselves, wrongly, in a unit that was not the queue's.** The ring
is configurable; our SDK sets 4096 slots where core-go defaults to 65536, on a real memory
measurement (~20.3 MB per peer, eager, never released). 4096 was justified in our own source as
core-go's stated *"sized for 1000+-file mount bursts"* plus 4× margin — **but that counted
files, and the queue counts notifications.** One mounted file produces the watcher's file
entity, the ingest chain's document and the blob bindings.

Swept with the supervisor off (`TestLoad_QueueDepthSweep`, `LOAD_FILES` × `LOAD_QUEUES`):

| ring slots | 2000 files | 10,000 files |
|---|---|---|
| 4096 (old default) | **675**, 2350 dropped | — |
| 16384 | 2000 ✓ 3.0 s | **2949**, 12535 dropped |
| 65536 (core-go's) | 2000 ✓ 3.0 s | **9100**, 900 dropped |
| 262144 | 2000 ✓ 3.3 s | 10,000 ✓ 5.9 s |

Bracketing those rows puts real demand at **6.5–8.2 slots per file**. The four-fold margin was
short by most of an order of magnitude because the unit was wrong.

**Both readings are true and they pull in different directions.** The default was too small —
so `shellboot.DefaultDeliveryQueueSize` is now 65536, the application tier making the opposite
call to the library for the same reason it does about the registry extension. And no value
fixes it — core-go's own default still loses 900 of 10,000 — because the producer is a person
with a file manager and nothing in the delivery path can slow them down. The ring decides how
big a burst completes *at live speed*; the supervisor is what makes every larger one a delay
rather than a loss.

### What happens now

1. **The ring is sized for the application** and is reachable from a config field
   (`Config.DeliveryQueueSize`) rather than only from a doc comment. It was a documented
   mitigation no frontend could apply.
2. **The counter is surfaced.** `status` reports saturation on the peer that dropped
   (`shellcmd/delivery_health.go`). A silent stall is now a visible one.
3. **A catch-up supervisor closes the gap by itself** (`shellcmd/catchup.go`). It re-derives
   the truth by asking each sending peer what it actually holds, and pulls what is missing. It
   runs automatically in any long-running frontend. **The rate adapts** — see §3a.
4. **`catchup` is a verb**, so an operator can force a pass and see what had been lost.

Routed to architecture as
`docs/outbox/SUBSCRIPTION-SATURATION-AND-THE-LAYER-BOUNDARY-2026-09-07.md`, because the parts we
cannot fix here are the parts that matter most: a subscriber cannot discover a publisher's ring
size or observe its drop counter, so our adaptive rate is a blind timer standing in for a
feedback loop.

Gate: `shellboot/catchup_e2e_test.go`. With the supervisor off, a 1,500-file burst delivers
621 and stops. With it on, 1,500 of 1,500 arrive. The control arm runs **first**, and the test
skips rather than passing if the burst did not saturate on that machine — otherwise it would
certify nothing on a fast host.

### 3a. The rate adapts, and does not churn

A fixed period has to be wrong for one of the two regimes. A folder that has just taken a
2000-file burst is missing two thirds of itself and every second of delay is an operator
staring at an incomplete directory. A folder idle since yesterday needs a pass only to notice
something delivery dropped, which is rare. They are three orders of magnitude apart in urgency
and identical in cost.

The receiving peer cannot see the sender's drop counter, so it cannot detect "I am behind"
directly. But **it can read its own passes**: a pass that recovered files means delivery is
losing things right now; a pass that recovered nothing means it is not. Purely local, no
protocol change, and exactly as accurate as the use it is put to.

| Signal | Next interval |
|---|---|
| The pass recovered files | **5 s** — straight to the floor, in one step |
| The pass recovered nothing | **double**, up to a 10 min ceiling |
| Any pass | **never shorter than the pass itself** |

Three properties, each gated in `shellcmd/catchup_interval_test.go` (a pure function, so it is
tested without a clock — no sleeps, no flakes):

- **Recovery is asymmetric on purpose.** Back off gently; return to the floor in a *single*
  step. Symmetric ramps are what make a sync tool feel broken — it has relaxed to ten minutes,
  a burst arrives, and it takes six passes to get interested again. Being slow to notice a
  burst is a failure an operator feels; being slow to relax is not.
- **The ramp is a gradient, not a snap.** An earlier version clamped the way up at the
  configured rate, which jumped 5 s → 60 s the instant a burst ended and discarded every rate
  in between — exactly the useful ones while a copy is still trickling in. The configured rate
  sets where the ramp *starts*.
- **Anti-churn: the wait is never shorter than the pass that produced it.** On a folder where
  a pass takes 20 s, the loop would otherwise run back-to-back forever — burning the peer, and
  re-reading a moving target instead of letting the burst settle. Bounded at a 50% duty cycle,
  asserted as an invariant over a 50-pass sustained-load run rather than as a single row.

That last one is the important one, and it is why settling first is not merely cheaper but
*more correct*: a catch-up reads current state rather than replaying a change stream, so **one
pass over a settled folder gets everything**. Passing during a burst re-does most of its work
on the next pass.

`status` reports the current interval and whether the loop is actively recovering or settled,
because the two regimes produce an identical folder listing and an operator looking at a stale
folder needs to know which one they are in.

### A pass covers at most 20,000 entries, and RESUMES

`backfillWalkLimit` bounds what one catch-up pass enumerates from the far side. The cap is
deliberate and is not a performance tuning knob: a sync points at somebody else's machine, and
*"how many entries are under this prefix"* is **their** answer — an unbounded walk driven by a
remote response is a denial of service with our own CPU.

**`20,000` is UNMEASURED, and is labelled so deliberately.** It is a policy choice about how much
remote-driven work one pass may do, not a benchmark result, and nothing in this document should be
read as claiming a measurement behind it. (Every other figure in this file is measured and says
where. The distinction is the discipline the specification seat adopted after a cost claim of
theirs travelled three documents with no measurement anywhere.)

**What was wrong until 2026-09-11 is that the bound had no memory.** The walk restarted from the
base prefix every pass and the traversal is deterministic, so a folder over the cap converged to
a fixed, permanently incomplete prefix. Measured — 25,000 entities, two passes: **0 new paths on
the second, 5,000 unreachable** for as long as they did not change again, with truncation
honestly reported and every gate green. A folder now resumes after a cursor: 20,500 entries in
two passes, none unreachable.

Two consequences an operator can see. A truncated pass **says where the next one picks up**
rather than only that it stopped, and `resync` names the action because the supervisor only runs
on a long-running peer — a `resync` typed at a shell that is about to exit gets no second pass
unless you run one. And a truncated pass **holds the supervisor at its floor**: "recovered
nothing" and "did not look at all of it" are different facts, and a folder covered one segment
per pass with an hour between passes would be the old defect rebuilt on top of its own repair.

The cursor is process memory, so a restart re-walks from the top. That costs round trips and
loses nothing: every file in the re-walked segment short-circuits on the content-hash check.

### What it still does not do

The supervisor makes a dropped delivery a **delay**, not a loss — up to one interval, longer
for a big folder. It is not backpressure: the sender still discards work rather than slowing
down. Real backpressure belongs in the kernel's delivery queue and is a spec-adjacent
conversation, not a workbench change.

---

## 4. FAILURE MODE 2 — the first change after a restart is DELAYED, not lost

**Corrected 2026-09-08, and the correction is the important part.** This section used to open
*"Not a delay — the change is gone"*, and every other document in this repo repeated it. That
claim was made on 2026-09-03, **before the catch-up supervisor existed**, and nobody re-measured
it afterwards.

Measured (`make twopeer-gui` PHASE 13, two containers, real TCP, receiver restarts while the
sender stays up):

```
asym1.txt written on the sender, receiver just restarted
  not on the receiver's disk at 90s      ← what every gate had ever asked
  ON the receiver's disk at ~107s        ← no resync, no operator action
```

**So it converges on its own.** The mechanism predicts the number: the supervisor takes a pass
immediately at startup, then **doubles** its interval after any pass that recovers nothing
(`nextCatchUpInterval`), so from a restart the passes fall at roughly t=0, t≈120s, t≈360s — and a
90-second assertion window expires between the first two *by construction*. Every harness in this
repo waited 90 seconds and concluded the file was gone.

**What is still open, stated exactly:** live delivery of that one change does not happen, and the
cause is still unknown — the first question, whether the miss is on the send side or the receive
side, is still uninstrumented. What is **closed** is the characterisation. This is a latency
defect, not a data-loss defect, and the difference decides whether the product is usable
unattended.

**No bound is promised.** 107s is one measurement on one machine, and the recovery interval grows
with how long the peer has been idle (up to the 10-minute ceiling). `resync` / **Pull now** forces
it immediately and is the answer if you do not want to wait.

Background: `docs/outbox/FIRST-CHANGE-AFTER-RESTART-IS-LOST-2026-09-03.md`, whose title is now wrong
and which carries a correction banner; two hypotheses are refuted there and remain refuted.

---

## 5. Concurrent edits, and the conflict storm that must not follow

**Shipped 2026-09-07.** Before it, a delivery that landed on a file you had edited replaced it
silently. Nothing was destroyed — the tree kept both versions at distinct chain positions, which
`DOMAIN-LOCAL-FILES` §1.1a guarantees and `shellboot/concurrent_edit_baseline_test.go` measured
— but the replaced version sat at a chain position no surface rendered and no verb reached.
**A recoverable loss nobody is told about is an unrecoverable one.**

### What happens now

A delivery whose hash differs from what is on disk is classified before it is applied:

| The bytes here were last changed by | What happens |
|---|---|
| a **delivery** (we are simply behind) | materialize, as always. The overwhelmingly common case, and it stays free |
| **a local edit** of ours | materialize, and write a durable conflict record naming the version replaced |
| a local edit, and the folder declares `keep-both` | as above, plus write the replaced version to `{path}.keep-both-{hash8}` |
| a local edit, past the burst limit | **refuse the delivery.** Nothing is overwritten and nothing is written |
| a local edit the operator already chose to keep | decline the delivery — the decision is honoured |

Surfaces: `conflicts` lists, `resolve` decides, and the *Sharing Status* panel has the same list
with **Restore mine** / **Keep theirs** / **Keep both** per row.

### The default does not create files, and that is deliberate

`EXTENSION-REVISION` §2.3's `keep-both` keeps the local version at the path and writes the
incoming one to a sibling. That is right for a two-way folder and wrong as a default here:

```
one-way share, A owns and B receives.
A edits -> a.  B edits -> b.
keep-both at B: f = b, f.keep-both-{a8} = a.
A still holds f = a, and B does not publish, so A is never told.
The two peers DIVERGE at f, permanently and silently.
```

The default converges — the arriving version wins on disk, as it always has — and records what
it replaced. **Both versions are recoverable either way; the difference is whether both are
PRESENT.** `keep-both` stays the word for the sibling form only, because it is
EXTENSION-REVISION's word for exactly that and blurring it would make a cross-impl term mean two
things. Set per folder, **on the peer that OWNS the folder**:
`conflicts -folder <id> -policy keep-both`.

**A shared folder is one subject and names ONE reconciliation rule — the
owner's.** Until 2026-09-11 each peer read its own copy of the field, so two
peers could hold different rules for one folder and diverge with nothing
noticing; the receiving side's verb now refuses and names the machine to run
it on. The receiver obtains the rule by reading the owner's declaration on a
reconcile pass and records it as an OBSERVATION, kept apart from its own
declarations (`workbench/observed_state.go`) because hearsay merged into a
declaration is a field that means two different things depending on which
side of the folder you read it from.

**A rule we have never read holds a collision rather than guessing one.**
Deliveries continue; a delivery that lands on a local edit answers `409
conflict_rule_unknown`, overwrites nothing, and is re-derived by the next
catch-up pass once the owner is reachable. The state is reported on the
folder's row by `status` AND by a read-only panel, from one sentence with
one writer — a refusal an operator can only discover from a failed transfer
is the failure this whole document is about.

### The discriminator, and the version of it that did not work

The recorder writes who authored each chain position: a local edit reaches the tree through the
WATCHER (`local/files:watch`), a delivered one through blob-resolve (`local/files:write`).

**Reading that off the HEAD does not work.** Every delivered file acquires a `watch` transition
shortly after it lands, because the receiver's own watcher ingests the file blob-resolve just
wrote and the file entity carries an mtime the watcher reads from the filesystem rather than
from the write. Same bytes, different entity, real transition. So the head says `watch` for
every file in a received folder, and the first implementation flagged all of them — the storm
below, produced by the caution meant to prevent it.

The question that survives is *who last changed the BYTES*: walk the run of consecutive
transitions carrying the current content hash and read the operation of its OLDEST member. An
mtime-only echo lengthens the run and cannot change its oldest member.

**A path with no chain is NOT a conflict.** Recording is what the classification reads, and a
folder that predates the reconciler's history config has none — treating that as conflicted
would conflict an entire folder on the first pass after an upgrade. It is a stated blind spot,
carried on the record as `recoverable: false`, not a guess dressed as caution. A path whose
recording hit the growth budget (§2) is in the same position: detected, not recoverable.

### The storm rules, and how each is met

**The realistic cause is a detection bug, not operator behaviour.** A genuine concurrent edit
needs two people editing one file between two syncs, which is rare and self-limiting. A bug in
the comparison is not self-limiting: it fires on every file, every pass. So:

1. **A conflict pass is rate-limited and fails closed.** More than 10 conflicts in 2 minutes on
   a peer and the handler REFUSES further deliveries rather than resolving them — nothing
   overwritten, nothing written, the condition reported on `status`, on `conflicts` and in the
   panel. A refusal is a delay rather than a loss: the catch-up supervisor re-derives the truth
   on its next pass, by which time the window has expired. Sliding window, not a resetting
   counter — a counter that resets on a boundary lets 2×limit through across two adjacent
   windows.
2. **Conflicts are listable and bulk-resolvable, and shipped in the same change as the thing
   that produces them.** `conflicts`, `resolve --all [-folder <root>] -keep …`, and per-row
   buttons in the panel. `-keep` has no default: choosing one for the operator is the behaviour
   the feature exists to replace.
3. **Keep-both never destroys.** Nothing in this feature deletes. The default writes no file at
   all; `keep-both` adds one; `resolve` removes only a sibling it or the operator created.
4. **The detection is gated with an anti-vacuity arm.** `shellboot/conflict_e2e_test.go`
   delivers a change to a file the receiver edited AND one to a file it did not, in the same
   pass, and asserts **exactly one** conflict. Without that arm, `return conflict` passes.
   It is the arm that caught the head-provenance bug above.

### What `-keep mine` promises, and what it does not

It puts your version back and **declines that delivery** — the resolved record is keyed on
(path, mine, theirs), which is the identity of one collision, so a catch-up pass will not undo
it. Gated: the e2e test restores and then runs two `resync` passes.

It restores through the **filesystem**, not through `local/files:write`, and that is the part
that is easy to get subtly wrong. A restore dispatched through the handler records with a
DELIVERY's provenance, so the next pass reads that head, concludes the local copy is merely
stale, and overwrites the restore — silently, minutes after the operator asked for the
opposite. Writing into the mounted directory makes the watcher ingest it and record
`local/files:watch`, which is the truth.

In a folder that only RECEIVES, the sender's **next** change to that path wins again. That is
what receive-only means. What is promised is that the next overwrite is recorded rather than
silent.

### Still open here

- **Nothing propagates a conflict to the other peer.** They do not know their change landed on
  your edit, and there is no wire message for it. Both sides notice only their own. A conflict
  record lives at `app/workbench/conflicts/…`, which nothing replicates, and telling the *owner*
  of a one-way share needs a **receiver→owner channel that does not exist**. It is a real gap and
  it is a **small, separate** item.

  > **Corrected 2026-09-10; the error is kept visible because it is the reusable part.** This
  > entry used to end *"— the same missing piece that stops `Mode: both` working, so the two are
  > one piece of work rather than two."* That inference was wrong, and wrong in the direction that
  > costs the most: it bundled a small problem with a large one and made the small one look
  > blocked. **`Mode: both` did not need a channel.** It needed the owner to *read* the receiver's
  > own declaration — an authority the receiver has already granted, because the reverse leg
  > exists only when they publish — and it shipped that day along with the asymmetric-root defect
  > underneath it. The general lesson: *"these two need the same missing thing"* is a claim about
  > a design that does not exist yet, and it is the cheapest possible way to make work look bigger
  > than it is. (The sibling statement in `FILE-REPLICATION-LANDSCAPE.md` was corrected on the
  > day; **this one was missed for five days**, which is the failure mode of recording a
  > supersession in one document and not in the one a reader opens next.)

  **One form of it does already work, and it is the one every comparable product uses.** A
  `keep-both` sibling is a real file in the shared folder, so in a **two-way** folder it
  replicates to the other machine by the path that already exists — no new namespace, no new
  grant, no wire change. It carries no metadata beyond the content hash in its name, and in a
  one-way share it goes nowhere, which is exactly the case where the owner most needs to hear.
- **A conflict is per file, not per folder.** Ten files edited on both sides are ten rows.
- **`resolve` does not merge.** There is no three-way merge and none is planned at this tier;
  `EXTENSION-REVISION` is where merge semantics belong.

---

## 6. Known limitations, stated plainly

| | Status |
|---|---|
| Change recording is unbounded per path | **Guarded 2026-09-07** (§2). Recording STOPS for a path past 2,000 versions and says so; nothing prunes, so the cost of the trade is the recent history of that path. |
| Change recording is unbounded in AGGREGATE | **Open.** N files × 2,000 is still a large number. Proportional to the data rather than to time, so it does not run away on its own. |
| The substrate's `max_depth` bound is a no-op | **Open upstream, measured, routed 2026-09-07.** `prune` walks and mutates nothing; there is no GC. Setting it costs a walk per write and bounds nothing. |
| No backpressure; the sender drops under burst | **Open upstream.** Mitigated by catch-up and by a bigger ring; neither removes it. |
| A subscriber cannot discover the publisher's ring size, or see its drop counter | **Open upstream.** It is why the catch-up rate is a heuristic rather than a feedback loop. Routed 2026-09-07. |
| First change after a restart | **Characterisation corrected 2026-09-08: it is a DELAY, not a loss** (§4). Measured converging on its own in ~107 s with no operator action. Live delivery of that one change still misses, cause unknown; the recovery is the supervisor, and `resync` forces it. |
| Catch-up rate adapts to whether it is finding anything | **And, since 2026-09-08, to folder SIZE.** The ladder still doubles; what it may climb TO is now derived from what a pass costs (`settledCeiling`, ~1% duty), so a ~1,000-file folder rests at the 60 s base rate instead of climbing to a ten-minute blind spot, while a 100k-file folder still backs off to the 10-minute hard cap. Before it, four empty passes — about fourteen minutes of quiet — put every folder on a ten-minute check, whatever it cost to look. |
| A catch-up pass over a very large folder is O(files) | 0.24 ms/file — ~24 s for 100,000 files. Not bounded or chunked. |
| Concurrent-edit conflicts | **Detected, recorded, listable and resolvable 2026-09-07** (§5). Not propagated to the other peer, and not merged. |
| `Mode: both` does not sync receiver → owner | **CLOSED 2026-09-10.** Both legs run, and the two peers no longer have to name the directory the same thing. The 2026-09-08 cause was right about the mechanism and wrong about the remedy: acceptance still does not travel, and it never needed to — `offered` on a folder *we own* is our own act of sharing, and the receiver's root name is READ from their tree when the leg is built. **Both sides must declare `both`**; a receive-only counterpart publishes nothing, and the verb now says so and names the command to run on the far machine. Gates: `shellboot/mode_both_asymmetric_roots_test.go` (bytes on disk, asymmetric roots, symmetric control arm), `mode_both_reverse_leg_test.go`. |
| Receiver → owner propagation of anything OTHER than file changes | **Open.** Conflict records do not reach the other peer (§5). The 2026-09-08 write-up bundled this with `Mode: both` as one piece of work; that was wrong — the reverse leg needed a READ, not a channel — so this is smaller and separate now, and still owed. |
| A backfill pass covers at most 20,000 entries | **Resumable since 2026-09-11** (§3). Before that the bound had no memory and a folder over the cap was permanently incomplete: measured at 25,000 entities, 5,000 unreachable across identical passes. A truncated pass now says where the next resumes and holds the supervisor at its floor. |
| **The sync leg accepts a stale delivery — no rollback floor** | **OPEN, and measured 2026-09-11.** A delivery carrying an older version of a file is applied over a newer one: replayed v1 after v2, `status=200`, no error, newer file gone from disk. `fetch.Consumer.acceptSeq` refuses exactly this on the STATIC leg. What is measured is that the delivery handler has no ordering check, at the entry point a live delivery and a backfill both reach — so an **out-of-order live delivery lands here by accident, with no attacker**. What is NOT measured is whether an unauthorized party can trigger it; the probe does not cross the wire. A fix cannot key on content or on "older mtime" alone: a sender that genuinely reverts its own file must still be followed, and a restored backup carries an old mtime. `shellboot/sync_rollback_probe_test.go` pins the measurement. |
| A shared folder's reconciliation rule | **CLOSED 2026-09-11.** It was stored per peer and read locally by whichever side the collision hit, so two peers could hold different rules for one folder and diverge unnoticed. The rule is the OWNER's; the receiver reads it and records it as an observation, the receiving side's verb refuses, and a rule we have never read HOLDS the collision instead of guessing (§5). |
| Four or more peers, and triangles | **Untested.** |

---

## 7. Which surfaces run the supervisor, and which do not

The supervisor is what turns a dropped delivery into a delay instead of a loss, so *which
programs run it* is a correctness property, not a deployment detail. It is started in
`shellboot.Bootstrap` and defaults off `Config.ReconcileOnStart` — deliberately **not**
per-frontend, because a step every long-running frontend has to remember is a step one of them
forgets, and the forgetting is invisible (a share that stopped part way looks exactly like one
that is idle).

| Surface | Supervisor | Why |
|---|---|---|
| Avalonia GUI, default launch | **yes** | `Program.ApplyDefaults` sets `reconcile_on_start` |
| Avalonia GUI, `--ephemeral` | **no** | see the gap below |
| `entity-shell` REPL | **yes** | started in `shell/repl.go`; it reconciles by its own route and never sets the flag |
| `entity-shell -c` (one-shot) | **no** | correct: the process exits before a pass would matter |
| `entity-console`, `-storage sqlite` | **yes** | |
| `entity-console`, in-memory | **no** | see the gap below |

**That flag answers the wrong question, and the gap it leaves is real.**
`ReconcileOnStart` means *"I have durable declarations to re-establish"*. The supervisor needs
the answer to a different question: *"am I long-running?"* The two coincide for the default
configurations and diverge for in-memory ones — **a peer with an in-memory tree can still
accept a share during a session, take a burst, and lose files permanently**, and it is the
configuration in which nothing recovers them on the next launch either, because there is no
next launch.

It is not fixed by simply defaulting the loop on for every peer, and the reason is worth
recording: `Bootstrap` is what the test suites build peers with, so an unconditional default
would put a supervisor behind several hundred test peers and change the load profile of the
whole sweep — and this tree already has load-dependent failures
(`TestE2E_Bidirectional_BurstWrites_NoFS`). Decoupling the loop from the declaration flag needs
its own switch and its own control arm.

**The recording growth guard answers that question differently, and runs everywhere.** It hangs
off `Config.HistoryPathBudget` and not off `ReconcileOnStart`, because unbounded recording is
not *work a peer does on a schedule* — it is a consequence of a mount existing, accruing at
whatever rate something else is writing. A peer that runs for an hour with a log file in a
shared folder has written a gigabyte whether or not it intended to stay up. It costs one prefix
watch and a map increment per recorded transition, so there is no configuration in which paying
for it is worse than not having it. A negative budget turns it off, which is the only way to
re-measure the growth.

Gates: `shellboot/frontend_catchup_test.go` asserts the flag survives the bridge's JSON under
the name Go reads and that `PeerManager.Create` turns it into a running loop;
`avalonia/tests/.../FrontendConfigTests.cs` asserts the C# end of the same string. **A peer
that is destroyed stops its supervisor** — until 2026-09-07 it did not, and the loop kept
taking passes against a closed store for the life of the process, once per peer the operator
ever removed.

---

## 8. What to do when a share looks wrong

In order, because the order is the diagnosis:

1. **`status`** on both machines. It reports saturation, what you grant each peer, per-folder
   file counts, the last catch-up pass, and any path whose change recording hit its budget. A
   peer that dropped notifications says so. In the GUI the same three lines are the *Sharing
   Status* panel's "Delivery, catch-up and change recording" section, with **Catch up now**
   beside them — before 2026-09-07 every one of those facts was reachable from the shell and
   from no pixel, which is the wrong way round for the failure an operator is least able to
   diagnose.
2. **`catchup`** on the receiving machine. If it recovers files, the live path lost them and
   the cause was §3. If it reports everything already current, the folder is fine and the
   problem is elsewhere.
3. **`syncs`** — is the relationship even established? A restored binding lists as `[restored]`.
4. **`history query <path>`** — who last wrote this file. `local/files:watch` is a local edit;
   `local/files:write` is a copy that arrived from the peer.
5. **`forget <peer>`** on *both* machines, then set the share up again. This is the reset, and
   it deletes no files.

The two readings that are most often misread: an **empty** result from a surface is a claim
about the surface as much as about the tree, and a **healthy** `syncs` row says a relationship
exists, not that anything is flowing through it.
