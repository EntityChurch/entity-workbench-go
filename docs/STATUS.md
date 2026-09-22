# entity-workbench-go — status

_Updated: 2026-09-03 · public: 0.9.0 (master) · working branch: `dev` (ahead of `master`)_

> **Start here:** **§0V — driving the flow for real, and what it found**, then
> **§0U — the control loop got a face**, then
> **§0T — accept takes a directory**, then
> **§0S — the loop was right and three verbs had not been told**, then
> **§0R — four reasons nothing survived a restart, and the reconnect engine we
> never called**, then **§0Q** (the file share that shared no files, and the
> kernel line nobody read), **§0P** (the whole flow, and the wildcard that was hiding the
> permission stage), **§0O** (M2, and the subscription that was dead after every
> restart), **§0N** (the panel threw on every file, and the forensics worked),
> **§0M** (a mount you can actually use), **§0L** (the GUI can mount a directory, and it
> remembers your workspace) and **§0J** (it was not a stack overflow, and the reason we said it
> was is the real finding). Everything after those is the running history. **§0I is superseded by
> §0J and is kept only so the error is legible.**
>
> **What this file is.** The rolling engineering log for entity-workbench-go — our tree, our
> defects, our decisions — and it is **published**. Working memory that is not about this
> project's own state lives in `docs/status/`, which publishes nothing: dated snapshots,
> handoffs, and cross-team coordination. Write here for the next session, but a stranger reads
> it.

## §0V NEW (2026-09-03) — we drove the whole flow for real, and it found three things

S4 shipped green. Then we ran the two-machine share end to end with the shipped binary — two
peers, separate homes, real listeners — and wrote the walkthrough by transcribing what actually
happened (`avalonia/README-SHARING.md`). **The flow works**: mount → share → accept into a
differently-named directory → existing files transfer → live changes propagate. Real bytes,
correct content, no manual mount on the receiving side.

Running it also found three things a full green sweep had not.

**A verb was still printing the instructions for a flow we had deleted.** `share` told the
operator on the other machine to `mount` first and then `accept` without a directory — the
pre-S3 sequence. Following it reintroduces the exact coupling S3 removed, because an `accept`
with no directory falls back to a mount named after the sender's folder. The operator doc had
been corrected; the program had not. That is now **AP71**, and the rule is that **printed
guidance is a surface with no reader in the test suite** — when you delete a step, grep the
program's output for it, and run the flow and read what it says.

**The address an operator types did not survive the process.** `connect` put it in shell state
and the kernel's pool, both process memory, so after a restart the reconciler had nothing to
dial with. It is now written to the peer's declaration — and **updates only, never creates**,
because connecting is a means and not a relationship. Alongside it, the loop now opens *our own*
outbound connection once per process and says so, since a connection has a direction for
authority and none for display: the pool reports "connected" over a route we cannot dispatch on.
The REPL's startup pass also prints what it *did*, not only what failed; re-establishing things
at every start is the entire point of the loop and it was doing it silently.

**And one defect we could not fix, characterised precisely rather than hand-waved.** After
either peer restarts, the **first** change is not delivered; every change after it is. No error
on either side, `status` reporting `settled` throughout. It is not a delay — the change is gone,
and only a pull recovers it. Two hypotheses were tested and both refuted by measurement, and the
second fix was **reverted rather than kept**, because a disconnect on every startup justified by
a dead hypothesis is a cost with no benefit. Routed with the full reproduction, the counted
runs, and the questions that remain, in
`docs/architecture/reviews/FIRST-CHANGE-AFTER-RESTART-IS-LOST-2026-09-03.md`. The product states
the limitation and the workaround rather than hiding it — a flow that looks reliable and is not
is the same failure as a surface reporting `settled` while nothing works.

The honest summary against the bar we set ourselves: **good enough to use deliberately, not yet
good enough to forget about.** The relationship model is right and is genuinely order-free; the
restart behaviour and silent conflict loss are what stand between this and something you would
trust a directory to unattended.

---

## §0U NEW (2026-09-03) — the control loop got a face, and what it is allowed to claim

**S4.** `Sharing Status (declared vs. actual)` — the third Network panel, and the honest
description of it is not "a new panel" but **the missing half of the control loop**. Before it
there was no bridge export for devices, folders or the reconciler at all: everything the
declared-state layer does was reachable from one shell verb and from no pixel. Meanwhile the GUI
*runs* a reconcile pass at every startup and reported its problems to **stderr** — a run log
nobody has a reason to open. An operator whose accepted folder had lost its mount was told so,
correctly and in detail, somewhere they would never see, while every panel on screen looked fine.

**The reachability sweep passed throughout and was right to.** It asks whether a
`workbench/*_model.go` has a surface; the reconciler is `shellcmd`'s and it has a verb. That is
the second instance of this blind spot in a different shape — the first was a cross-peer handler
built, tested and registered nowhere — so it is now **AP69**, and the question it says to ask
when scoping any non-model layer is *which shipped binary reaches this, and by pressing what*.

**Reading and re-checking are separate operations, and the panel says which it is showing.** A
reconcile pass dials every declared peer, so a panel that reconciled on refresh would be a
dialer somebody leaves running overnight. `StatusSnapshot` reads records and observes the
substrate; `Reconcile` runs the loop; they share one observation pair so a read and a pass cannot
describe the same device — or the same fault — differently. Which one produced a table rides in
the **outcome**, not in the caller's memory, because a surface that has to recall which function
it called in order to caption its table will eventually caption it wrong, always in the confident
direction.

**Outbound authority is stated exactly; inbound is not stated at all.** What we grant a peer is
our own capability row, so it is printed verbatim. What they grant us is in *their* table, which
this peer cannot read — it is only ever observed, through deliveries arriving. So the inbound
direction renders as what has actually landed (both layers of the mount, and an explicit
*unknown* when there is no mount to count) and **never as a health dot**. A dot there asserts
something about another machine we have no way to check, and it is wrong in exactly the case that
matters: they revoked us and we have not tried since. There is a standing gate on that, and its
instruction on failure is to delete the offending field rather than update the test.

Two verbs came with it, because a panel with no verb in it is the tell for a read-only surface
over a read-write model: **Pause/Resume**, which writes the *declaration* so the loop obeys it
rather than being fought by it, and **Remount**, which is the one action the reconciler names and
deliberately refuses to take — it will not write to a directory on somebody's disk. Remount takes
no directory argument: the path comes from the declaration, which is the only record left that
can say where a folder's files are once the mount that knew is gone.

Gates: `shellcmd/status_test.go` (eight, four verified red with the behaviour removed — the
read-does-not-write one runs the reconcile pass as its control arm on the same fixture, because
otherwise it is satisfied by a function that does nothing) and
`avalonia/tests/.../SharingStatusPanelTests.cs` (nine, including the geometry check that
distinguishes a rendered button from a clipped one).

Not built, and named rather than left implied: **pending offers still have to be pulled.** A card
that appears unprompted needs either a poll of every declared device on a cadence (which looks
like a hang when a device is asleep) or a subscription to their offer records, which needs a grant
we do not currently ask for. Shared Folders keeps the pull, and this panel does not imply a
liveness it does not have.

---

## §0T NEW (2026-09-03) — accept takes a directory, and the coupling nobody had written down

**S3.** `accept <peer> <root> [<directory>] [-anyway]` now creates the directory, mounts it,
authorizes the sender's deliveries, subscribes and pulls everything already in the folder — one
action. The GUI asks on the offer row, pre-filled with a fresh path under `~/entity-shared/`.

Two steps of the nine-step flow are gone. The obvious one is the receiver's separate `mount`.
The other was not written down anywhere: **a mount root is derived from its directory's
basename, and the sync was keyed on a single shared root name, so the receiving directory had
to be named exactly what the sender happened to call theirs.** Nothing said so, and picking a
sensible local name produced a relationship that established cleanly, reported the right target
prefix, and delivered nothing. A received folder now records both names —
`FolderData.LocalRoot` beside the sender's `Root`, read through `ReceivingRoot()`.

**The refusal is the interesting part of the design.** Accepting into a directory that already
has files in it is destructive: incoming files overwrite same-named local ones and remote
deletes propagate. So the operation refuses, as a typed value carrying the path and the count,
and the GUI renders that as a choice with the consequence stated — not as a failure, and not as
a pre-authorized default. The pre-filled path is deliberately one that does not exist, because
a default that trips its own safety check on every first use teaches people to click past it.

Gates: `shellboot/accept_directory_e2e_test.go` — two peers, receiver mounts nothing, receives
into a differently-named directory, and the assertion is **bytes on disk**, because every
intermediate signal here is one this repo has already shipped a green version of over an empty
folder. Verified red two ways: without the target-root wiring the accept refuses, and with the
backfill disabled the file never arrives (the byte assertion polls its full 30 s and fails).
Panel side: `SharePanelAcceptDirectoryTests`, which asserts the field is laid out inside the
panel and that a typed path **reaches the model** — checked by requiring the directory to exist
afterwards, since asserting on the panel's own accessor would be circular.

Note for the next session: those are the first tests here to drive a `Perform*Async` panel
method, and the first attempt deadlocked the headless dispatcher —
`.GetAwaiter().GetResult()` on the test thread blocks the UI thread that the continuation needs.
Use `[AvaloniaFact] public async Task` and `await`.

---

## §0S NEW (2026-09-03) — the loop was right and three verbs had not been told

A review pass over §0R's work, before committing it. The design holds and the sweep is
green; three live defects were found in the seam between the new control loop and the
verbs that predate it, all the same shape, and all invisible to the tests that existed.

**The reconciler computed the policy union; the verbs still wrote their own half.** §0R
identified AP68 — `system/capability/policy/{peer}` is one row, `share` writes the sender
grants and `accept` writes the receiver grants over the top — and fixed it *in the
reconciler*, which recomputes the union on its next pass. The verbs were left alone. So the
clobber still happened and was merely **healed later**: at the next `status`, or the next
launch. That window is entered by performing the product's two gestures in one session,
which is the case the fix was written for. `shellcmd.ApplyDeclaredPolicy` is now the single
writer and both verbs derive from the declaration. The lesson worth keeping is general:
**a control loop that corrects a bad write is not the same as removing the writer**, and no
test that runs the loop can tell the two apart — which is why the existing gate seeded the
row "the way the verbs leave it" and stayed green.

**`unshare` did not survive its own reconciler.** It removed the policy row and never
touched the folder declaration, so the next pass read `offered` and wrote the grant back:
the withdrawal held until the operator restarted and then silently reversed itself. This is
a defect the loop *creates*, and it generalizes — **once a reconciler exists, every verb
that changes what the operator wants must write a declaration, or the loop will undo it.**

**And `unshare` dropped the whole row.** Its test for whether to revoke was "is this peer in
any remaining offer", and offers describe only the outgoing direction — so unsharing the
last folder from a peer who is also *sending* us one deleted their delivery grant too. The
predicate was deleted rather than left beside its replacement.

Gates: `shellboot/share_union_e2e_test.go` (drives the two shipped verbs and reads the row
they leave — asserting the reconciler's output would have tested the healer) and
`shellcmd/declare_test.go`. Every one verified red with the fix genuinely removed. **One of
them was vacuous first** and the control arm is what caught it: it drove `Share` with an
unmounted root, which is refused *before* anything is declared, so the unwind path it was
written for never ran.

Also fixed: `make test-each` did not clear `.test-logs/` before a run, so a sweep in
progress left a mixture of this run's logs and the last one's with nothing to tell them
apart. Read mid-sweep during this review, a stale `FAIL` from a defect fixed hours earlier
read as a live failure — and it is the more alarming reading, so it wins. Same rule as
`bin/`: build output is not the tree.

Verified: 10/10 Go suites, Avalonia 160/160, vet clean, reachability clean, `make build`
clean. (`shellcmd`, `shellboot` and `shell` re-run after the `unshare` fix, which touches no
other package.)

---

## §0R NEW (2026-09-03) — four reasons nothing survived a restart, and the reconnect engine we never called

An operator spent two days trying to share a folder between two machines and never got a file
across. The previous session found one hard defect and wrote the redesign brief. This session
priced the redesign against the substrate first, and the pricing is the finding: **most of what
we were about to build already exists, and the reason the flow does not work is that almost
nothing in it was designed to survive a restart.**

### 1. Four independent restart killers, and they stack

Each of these alone makes the whole flow fail after a relaunch. Fixing any one in isolation
would have changed nothing observable, which is why two days of correct button-pressing
produced nothing.

1. **The GUI's default peer was ephemeral.** No identity, memory store, fresh keypair per
   launch. The tree is peer-id-namespaced, so this was not "some features off" — the app was
   **a different peer every time it started**, and every grant, mount, offer and accepted share
   from the last session named a peer-id that no longer existed. An operator debugging by
   relaunching was destroying the state they were debugging, on both machines, every time.
2. **The GUI's default peer did not listen or announce**, and discovery is disabled entirely
   without a listener. The default configuration could not be reached, found, or shared with.
3. **`localfiles.Handler.Load` restored no mounts** — the kernel AP58 instance found last
   session, worked around in `workbench/localfiles_root_restore.go`, routed in
   `reviews/LOCALFILES-LOAD-RESTORES-NOTHING-2026-09-03.md`.
4. **Nothing re-established a peer relationship at startup**, because nothing called
   `maintain-peer` (§2).

And a fifth, found while pricing: **`entity-shell -listen ADDR` bound no socket at all** (AP67).
The address went `shell.Config` → `shellboot.Config` → `entitysdk.PeerConfig` →
`peer.WithListenAddr`, and core-go reads that field in exactly one place — `Peer.Listen` —
which only `PeerManager.Create` ever called, and `entity-shell` does not use the manager. A
documented flag, step 1 of the operator recipe, doing nothing, for months, **with every e2e
suite green**: each stands its own listener up with a local helper rather than going through
the frontend's startup. That is AP63's shape at the level of a startup step — *a fixture that
reimplements part of the product's startup deletes that part from the suite.*

### 2. The reconnect engine has been in the kernel the whole time

`system/network:maintain-peer` connects, installs the §4.1 reconnect continuation graph,
**retries forever** with derived backoff, and restores subscriptions on reconnect. The handler
is registered by default. `entitysdk.NetworkClient.MaintainPeer` wraps it.

**It had zero callers in shipped code** — ten references in this repo, nine inside its own file
and one in its own test — while three of our verbs called `AppPeer.Connect` once and treated
that as a relationship. So *"why do I have to press connect again"* has a one-line answer, and
the answer is not that the feature is hard.

Two more of the same shape, confirmed by grep rather than assumed: `ext/signaling/peerwiring`
(symmetric establishment, unused) and `localfiles.RootConfigData.ReadOnly` (Syncthing's "Send
Only", honoured by the kernel's writeback path, settable from nothing we ship). D20 says price
against the substrate; the correction this session adds is **to point it at our own SDK too.**

### 3. The structural error: a wizard where the substrate wanted a controller

`Share` → `Accept` → `CompleteShare` are one-shot sequences, each encoding the four AP63
authorization facts inside a button press. That is order-dependent, non-idempotent, and has no
representation of what it established — so nothing can re-establish it. Underneath were **five
disconnected durable records** (roster entry, policy row, offer record, mount binding, sync
binding) written by five verbs and restored by four different startup paths, and **none of them
was the relationship.**

Now: two declared records — `app/workbench/devices/{peer-id}` and
`app/workbench/folders/{folder-id}` (`workbench/desired_state.go`) — and one idempotent
reconciler (`shellcmd/reconcile.go`) that derives everything else. `share` and `accept` keep
doing what they did and additionally declare; startup, the new `status` verb, and the GUI's
background pass all run the same loop. Full argument in
`docs/architecture/SHARING-DIRECTION.md`, including the Syncthing model this follows and why
Dropbox's is not available to us.

### 4. What writing the reconciler found: two-way sharing could not authorize (AP68)

`system/capability/policy/{peer}` is one row per peer. `Share` writes the sender grants to it,
`Accept` writes the receiver grant to the same path, and `SaveAccessPolicy` replaces rather than
merges — **correctly, by its own documented contract**, which reasons from "one caller, one
statement of intent". With two callers that reasoning is false: share a folder to a peer AND
accept one from them, and the second write silently revokes the first. Invisible because
`sync_twoway_e2e_test.go` — the test written specifically for two-way — runs under
`OpenAccess: true` and says so in its own header, and a wildcard never consults the row. **AP63
one instance on from the instance that named AP63.**

The fix is not a merge. It is that a shared row gets exactly one writer which derives it from
every declaration that contributes to it.

### 5. Landed, with its gates

- **Iteration cost.** Podman cache mounts on every compile step in `avalonia/Containerfile`; a
  rebuild after a source edit is **~13 s** where it previously re-downloaded the Go module graph
  and the NuGet graph, including the self-contained runtime packs, every time.
- **Build stamp.** `git describe` plus a hash over the working-tree diff — stable for an
  unchanged tree so the layer cache still hits, distinct for any edit. Compiled into **both**
  the frontend and the bridge, printed at startup, written to the crash trail, and **in the
  window title**, because the way this information actually travels is a screenshot. A
  frontend/bridge disagreement is reported as a MISMATCH: that is the one state `extract` can
  produce and nothing could previously report, and it cost a day on 2026-09-03.
- **Defaults.** The GUI is a persistent, listening, announcing peer; `--ephemeral` restores the
  old throwaway. `BringUpListener` (`shellboot/listener.go`) is the single bring-up — bind,
  advertise, announce — shared by every frontend. A wildcard bind now advertises this host's LAN
  address instead of publishing no profile at all.
- **The loop**, with `status` as its shell surface.

Gates, each verified red-then-green with the fix genuinely removed:
`shell/listen_test.go` (dials the reported address from a second peer; **two** control arms —
bring-up removed, and bring-up faked with a plausible address and no bind),
`workbench/desired_state_test.go` (peer-backed store; the AP58 control arm turns it red),
`shellcmd/reconcile_test.go` (the union, and idempotence — the control forces the grant
comparison to always differ, which reconnects every peer on every pass), and
`shellboot/reconcile_restart_test.go`, which is the one that matters: **across a process
boundary, on the derived structure, asserting not-connected before the pass and connected
after**, because only that pair of facts distinguishes re-establishment from a connection that
never dropped. Its control arm swaps `maintain-peer` for the old single `Connect` and it fails.

### 6. Not done, and named

- **The Sync panel** (`SHARING-DIRECTION.md` §7, S4). The loop has a shell surface and no GUI
  one yet. The three existing panels stay — they are how we understand the system — and what
  becomes of them is a question for after the flow works, not before.
- **Accept-with-directory** (S3): one action that mounts, accepts, syncs and backfills.
- **Cross-NAT.** Everything here assumes the machines can reach each other. `peerwiring` plus a
  signalling carrier is the answer for the internet case and is deliberately not scheduled: it
  needs infrastructure deployed, and it would not have fixed any part of the reported failure.
  The previous handoff nominated it as *the* lever — right about the mechanism, wrong about the
  priority, because on a LAN the reconciler plus discovery removes the address problem without
  it.
- **Peer identity on every panel**, and the `gui-layout.json` / `browser.json` migration into
  the tree. The layout file's stated reason for being a file was a consequence of the ephemeral
  default and no longer holds.

### 7. A process note worth keeping

`make test-each` and `make -C avalonia test` were run concurrently during this session. AGENTS.md
says never to, and the reason is exactly what happened: both bind-mount the tree with `:Z`, the
second relabel revokes the first container's access mid-run, and three suites reported
`Permission denied` on their own log files while the tests inside them had passed. **A sweep
that goes red for a reason outside the tree is worse than no sweep** — it is indistinguishable
from a real failure at a glance, and it cost a re-run. The rule was already written down.

## §0Q (2026-09-02, fourth pass) — the file share that shared no files, and the kernel line nobody read

Two defects, found from one operator report — *"we're trying to debug this on two computers and
it keeps crashing, and I can't get the mounts working"*. They are unrelated in mechanism and
identical in shape: **in both, the artifact that named the cause was already on disk and had been
read past.**

### 1. The crash: the render thread was on 16 KB, and the kernel said so

`journalctl` for the crashing pid:

```
kernel: signal: entity-avalonia[3746566] overflowed sigaltstack
```

One line, unambiguous, and it settles what §0J left open. The process pid is 3746552; **3746566
is a secondary thread** carrying `ServerCompositionContainerVisual::Update` and `libSkiaSharp`
frames — the render thread. A SIGSEGV arrived there, CoreCLR's handler chain ran on the PAL's
stock **16 KB** alternate signal stack, overflowed it, and the process died unrecoverably: no
minidump, no crash log, and a breadcrumb trail that just stops.

`CrashDiagnostics.cs` had predicted this in a header comment since 2026-09-01 — *"the 2026-09-01
fault landed on a background thread, which still has the PAL's stock 16 KB … **Nothing in this
file fixes that**"* — and `AGENTS.md` carried it as a known open gap. So the diagnosis cost
nothing and the fix was never scheduled, because the paragraph explaining why the limitation was
correct had closed the question. **That is AP45's lesson pointed at ourselves for the second
time**: a `TODO` invites work, a well-written rationale ends the discussion permanently.

It also retires §0J's *sharpest open question*. That entry recorded, as an untested hypothesis,
that "createdump is on and produces nothing for this fault class … by the time the process faults
there is no stack left for createdump to run on." That is now **measured**, by a field nobody had
looked at, in a journal that had been sitting there for ninety seconds when the session started.

**The fix.** `sigaltstack` is per-thread and must be called **on** the thread it covers, so
something has to run on the render thread. Nothing tidy is available in Avalonia 11.2 —
`IRenderTimer.Tick` is raised there and is `internal`, `Compositor`'s update callbacks run on the
UI thread, and `AvaloniaLocator.Current` is gone. What does work is an `ICustomDrawOperation`:
serialized into the render command stream and executed by the compositor during the render pass,
i.e. **in the same call stack the crash was taken in**, by construction rather than by inference.
`AltStackProbe` is a zero-size, hit-test-invisible control that does exactly that and nothing
else. Measured on a real X11 run, before and after, on a different thread from the UI thread:

```
altstack: at startup:  sp=0x7f34ff4dd000 size=16384    flags=0   (UI thread)
altstack: render thread (managed tid 4): before: size=16384      <- the crashing configuration
altstack: render thread: after:  sp=0x7f30401a2000 size=1048576
```

**Gate:** `run-xvfb-smoke.sh` now fails (exit 3) if that line is absent, because this is the only
harness in the repo that can see it — the headless suite has no render thread, and there is no
cross-thread query for another thread's alt stack, so *the install having run there* is the only
evidence obtainable. The control arm is real: the same grep against the crashed run's own log
(`run-20260902-201920.log`) finds nothing, so the gate would have failed the exact build that
died. `WB_ALTSTACK_BYTES=0` is exempted, since it is the documented A/B arm.

**What is still NOT fixed, and must not be read as fixed.** The *original* fault inside the
compositor walk is still unidentified, exactly as §0J left it. This makes the fault
**survivable and legible** — a 16 KB overflow produces a corpse, a 1 MB stack produces a handler
that can run — and the next occurrence should finally arrive as something with a stack trace. It
is not a fix for whatever faults. And only threads we can reach are covered; the external
`createdump` path stays armed for the rest.

### 2. The file share did not share files

The operator's other report — *"I haven't been able to get the mounts working, the
synchronization"* — was correct, and the cause is one line of `Sync`:

```go
entitysdk.SubscribeOpts{Events: []string{"created", "updated"}, IncludePayload: true}
```

**A subscription is a future tense.** It reports what happens after it exists. So a folder that
already had files in it transferred **nothing**, because no file in it ever changed again — and
the gesture the product is named after (pick a directory, share it with a peer) produced an empty
folder, no error on either side, and a `syncs` row reporting the relationship as healthy.

This was **documented**, which is why it survived. `USAGE-SHARE-A-FOLDER.md` listed *"history
replay — files already sitting in A's folder arrive when they next change, or when A remounts"*
under what-this-does-not-cover; the `sync` verb printed the same sentence; and
`sync_e2e_test.go` writes its file **after** the sync, deliberately, with a comment explaining
that the subscription does not replay history. Every artifact was accurate about the
implementation and every one of them described a product that does not do the thing it is named
after. **A known-limitations entry is not a substitute for the feature**, and prose that explains
why an absence is correct is read as a decision rather than as a defect.

**The fix reuses the live path rather than paralleling it.** `shellcmd/sync_backfill.go`
enumerates the remote prefix and, per file, synthesizes the notification the subscription engine
would have delivered, dispatching it at the same `workbench/blob-resolve:receive` handler through
`Executor.ExecuteWithIncluded` — which is the shape a live delivery already arrives in, the
changed entity presented in `HandlerContext.Included`. Mount lookup, the F9 already-current
short-circuit, the capability-checked cross-peer blob closure pull and the `local/files:write`
materialization are therefore shared **by construction**. Ordering is load-bearing: subscribe
first, then backfill, so a write landing during the catch-up is carried by the subscription; the
overlap is harmless because F9 makes a duplicate a no-op.

**A correction to the flow's own story, and it is not a softening.** `accept` used to end with
*"until they dial you, this folder stays empty"*. The backfill runs entirely on authority the
receiver holds — it dispatches to the publisher for the listing and the closure, which `share`
already grants — so **nothing in a first transfer requires the publisher's dial**. What requires
it is *delivery of future changes*. The old text was telling operators their files would not
arrive while their files were, by then, already on disk.

**New verbs.** `resync <peer> <root>` re-runs the catch-up without touching the subscription —
the "pull now" affordance, and the only positive confirmation available anywhere in this flow:
run it twice and the second pass reports everything **already current**, where otherwise "no
errors" and "nothing happened" render identically. `forget <peer>` / `forget --all` drops syncs,
offers, authorization and the connection, because everything the flow establishes is deliberately
durable and the sum of that is a flow that **cannot be re-tested** — a second run is
indistinguishable from a stale grant, and that failure presents as a success nobody can trust. It
deletes no files and unmounts nothing. Both are in the GUI: **Pull now** and **Forget peer** on
each received folder's row, **Forget all peers** in *This peer*.

**Gate:** `shellboot/sync_backfill_e2e_test.go`, through `shellboot.Bootstrap` and the shipped
verbs, seeding files **before** the mount so they enter the tree via the watcher's initial scan
and are never the subject of a change event. It has a **control arm** —
`TestBackfill_ControlArm_WithoutItNothingArrives` runs the same scenario with `SkipBackfill` and
asserts the folder stays empty — so the positive test cannot pass on some other mechanism
replaying history, which is precisely the vacuity the sibling test had.

### Two-way was the composition of two tested halves, and nothing tested the composition

The operator's model is Dropbox — one folder, two machines, contents converge. Everything gated
here was **one direction**: `sync_e2e_test.go` establishes A→B and asserts a file written on A
lands on B. Two-way was *supported* — `workbench/blob_resolve.go`'s F9 content-hash short-circuit
exists specifically to stop the notification loop the "bidirectional symmetric topology" creates —
and exercised only by `shellcmd/cmd_local_files_bidirectional_test.go`, which hand-assembles the
chain instead of going through the verbs. So the thing the product is *for* was the composition of
two separately-tested halves, and the composition had no gate.

`shellboot/sync_twoway_e2e_test.go` closes that: both peers mount, both seed files **before**
either sync exists, both `Sync`, and every file must land on both disks with live delivery still
working in both directions afterwards. It passes.

**It also caught something on its first run, and the finding is the assertion rather than the
code.** The test expected each backfill to scan 2 — the other side's files. Bob's scanned **4**.
That is correct: by the time bob syncs, alice's backfill has already pulled bob's files into
alice's folder, so bob enumerates alice's folder and sees both sets. The reported outcome was
`4 file(s) found, 2 transferred, 2 already current` — the F9 short-circuit recognising bob's own
bytes and declining to pull them back. **The loop guard is visible in a count**, and the test now
asserts what must hold (each side sees at least the other's files; the two backfills materialize
no more than the number of distinct files that exist) rather than a number that depends on which
sync won a race. A regression in F9 shows up in that bound before it shows up as churn.

The audit also added the case that would have shipped broken: a file in a **subdirectory**. The
remote enumeration is a breadth-first walk that must descend, and a walk returning only the top
level would have passed every other assertion — a partial transfer reported as a complete one,
which is the original defect wearing a smaller hat. The gate asserts the nested file arrives *at
its nested path*, and distinguishes "missing" from "arrived flattened into the mount root",
because those two failures have different causes and the same symptom.

### The finding that spans both

Neither defect needed an experiment. The crash needed `journalctl`; the sync needed the last
section of our own usage doc. Both had a **written, accurate, in-tree artifact naming the cause**,
and in both the artifact was phrased as settled — a rationale in a header comment, a
known-limitations bullet. D25 says *a field that prints is not a field that answers*. The
extension this pair earns: **an explanation that closes a question is a claim with no expiry
date, and nothing re-opens it.** Grep your own tree for sentences that explain why something
cannot or need not be done, and treat each as unmeasured until it is re-measured — they are the
cheapest place in the repo to find a live defect, and the least likely to be looked at.

**State.** Avalonia headless **160/160** (was 157 — three new boundary tests).
`smoke-xvfb-click` green with the new gate (200 gestures, drag 40%, exit 0).
`make test-each` **9/10 — `sdk` is RED**, and it is not this session's:

```
sdk FAIL 329s · inspect PASS · shell PASS · shellboot PASS 97s · shellcmd PASS 307s
shellpanel PASS · workbench PASS · programs PASS · publish PASS · fetch PASS
```

Run twice, on a settled tree both times, with byte-identical failures — the first attempt was
discarded because it was started while the tree was still being edited, and a sweep over a
changing tree is not a measurement.

```
--- FAIL: TestAxis1Equivalence_Differential
--- FAIL: TestAxis1Equivalence_ContainedErrorPositions/map-element-contains
--- FAIL: TestAxis1Equivalence_ContainedErrorPositions/fold-accumulator-contains
    stage1: error(index_out_of_range: …)   axis1: value:uint64:04
```

**Not bisected to a commit, and deliberately not called "pre-existing" on vibes** — what is
established is narrower and sufficient: `entitysdk` is **unmodified** in this working tree
(`git status`), it imports neither `shellcmd` nor `shellboot` (the dependency direction forbids
it), and the failure reproduces **alone, unloaded, in 0.02 s**, so it is not the load-dependent
class this file warns about. Neither of the two commits since §0P's "10/10" claim touched
`entitysdk/axis1/`, so **§0P's green line and this red are not reconcilable** and one of them is
wrong about the tree; that is worth someone's attention on its own terms.

The shape is **AP43's exact territory** — the CONTAINED positions, where `map`'s output element
and `fold`'s accumulator must *place* an error as a value rather than short-circuit. Axis-1 is
returning the plain value where Stage-1 raises. The commit that fixed this class is the one whose message reads
*"fix(axis1): contained-error position semantics, and an admission gate that was never
running"* — cited by content rather than by hash, because this file publishes and an internal
SHA resolves to nothing on the public mirror ([ADR-0012] Am. 1).
**Left alone on purpose**: it is the compute engine, it is nowhere near a folder share, and
opening it at the end of a session whose subject is the file-sharing flow is how the sweep ends
up red in two places instead of one. Flagged, reproduced, and handed on.

## §0P NEW (2026-09-02, third pass) — the whole flow, and the wildcard that was hiding a stage

The ask was to make discover → share → permission → mount → change/sync one seamless process, and
validate it before multi-device testing. It is validated, end to end, with **no wildcard grants
anywhere** — and turning the wildcard off is what made the afternoon interesting.

### Every cross-peer test we have ever run was under a wildcard

Twenty-four of them, all `peer.OpenAccessGrants()`. That authorizes everything, so the entire
cross-peer suite establishes that the bytes move and says **nothing** about permission — the one
stage an operator actually has to perform. The kernel's per-peer mechanism, the V7 v7.62 §8
handshake policy table at `system/capability/policy/{peer}`, had **zero uses in this repo**. What
we had instead was `shellboot.Config.OpenAccess`, whose own doc says development-only.

The generalisation is the part to carry: **a permissive fixture does not weaken a test, it deletes
a stage of the product from the suite** — and the deletion is invisible, because everything
downstream of the disabled mechanism passes. The count of tests covering that mechanism is zero
however many run through it. AP63.

### Four things the wildcard was hiding

All measured (`shellboot/policy_probe_test.go`), each with its own arm, and each one an afternoon
lost if you meet it on real hardware instead:

1. **A sync is MUTUAL authorization.** The receiver dispatches *into* the publisher to subscribe
   and to fetch blobs; the publisher's subscription engine dispatches the notification *back*
   into the receiver's `blob-resolve`. Both cross a capability boundary. Grant one direction and
   the subscription is **accepted** and no file ever arrives — indistinguishable from a working
   share until someone opens the folder. Every prior test hid this because the receiver was
   always wildcard, **including the one written specifically to close the capability-delegation
   gap**, which scopes the sender and leaves the receiver open on purpose.
2. **The grant is assembled at HANDSHAKE.** A policy written on a live connection is inert until
   that connection is re-established.
3. **The peer that DISPATCHES is the peer that must reconnect.** A reconnect by the granter does
   nothing for the grantee, who dispatches over the connection *it* opened.
4. **A dial-by-address authorizes the DIALER ONLY.** `sendReciprocalGrant` is gated on
   `EstablishedViaRendezvousKey()`, and the kernel says why: *"a dial-by-address is asymmetric —
   one party requested service."* So a two-way sync needs **both** peers to dial, each after the
   other's policy exists — and the final dial is a step neither verb can perform for the other.
   `accept` prints the exact command for the other machine, because the symptom otherwise is
   silence on the side that cannot see the refusal.

### What shipped

`peers` (discover), `share` / `unshare` / `shares`, `offers`, `accept`, `access`. Two of these
stages had no surface at all before today: `entitysdk/share.go` implements APP-CONVENTION-SHARE in
full and was called from `share_test.go` and nowhere else, and the policy table was untouched.

Three decisions worth recording:

- **The authorization is the policy table, not `AuthorShare`'s minted token.**
  `ShareWithdrawalNotice` states that a `system/capability:request`-minted token is **not
  recallable** — no tree write, so the granter never holds its hash and `revoke` cannot name it.
  Built on that, `unshare` could not end access and would be lying by its name. A policy entry is
  a tree write we own.
- **The offer record is a label, not an authority**, per §2.2 — so seeing an offer and still
  getting a 403 is the two things being correctly separate rather than a defect.
- **`access` marks the peer's own row.** The kernel seeds a `*:*` entry keyed on the peer's own
  identity hash (§6.9a). Unlabelled it reads as a wildcard grant to a 66-character stranger;
  hidden it would conceal a real grant. It is shown and named.

### Two bugs found in our own new code, both by the test rather than by review

**A reconnect that could leave the peer disconnected.** The first version tore the connection
down and *then* discovered it had nothing to dial — strictly worse than doing nothing, because it
turned a share into an outage. It now resolves an address first and leaves the connection alone
if it has none.

**`ConnectedPeers()` is not a source of dialable addresses.** Its `Address` is the connection's
*observed* remote address, which for an **inbound** connection is the dialer's ephemeral source
port. It looks exactly like an address — `127.0.0.1:50026` — and dialling it is refused. The two
honest sources are the address we recorded when *we* dialled (`connect` writes it) and the peer's
own mDNS announcement, which advertises the port it listens on.

### One extraction

`chooseDialAddr` / `pickDialHost` / `parseTXTPairs` lived in the Avalonia bridge. The shell's
`peers` verb was about to be the second consumer, and the dial-address choice is a substrate
judgement about mDNS announcements rather than anything a renderer owns — so it moved to
`entitysdk` and the bridge calls it. *DRY the integration, not the renderer.*

### Verified

`make test-each` 10/10. `lint`, `textual`, `reachability`, `gofmt` clean.
`shellboot/flow_e2e_test.go` runs the whole flow across two peers with **no OpenAccess on either
side** — discover, share, inspect the permission, read the offer from the other peer, mount,
accept, write a file, **edit** it, then revoke. Its companion `TestFlow_ShareAloneDeliversNothing`
is the control arm and the more valuable of the two: it pins that the sender's half alone yields
an accepted subscription and an empty folder, so a later "simplification" of `accept` into a plain
sync fails there with the reason attached.

**Not done, named rather than implied:** no GUI surface for any of these verbs — the Local Files
panel manages mounts and says nothing about shares or syncs. Still one LAN or manual addressing
(`ext/relay` unlanded upstream). Concurrent edits to one file on two machines remain M3 and
remain unsafe to assume. The operator recipe is
`docs/architecture/USAGE-SHARE-A-FOLDER.md`.

## §0O NEW (2026-09-02, second pass) — M2, and the subscription that was dead after every restart

Two peers, one folder, end to end, through the shipped verbs. And on the way, a live defect that
had been in every build since the subscription wiring was written.

### `sync` — and the engine was already there

`sync <peer> <root>`, `unsync`, `syncs`. The receiving chain is *subscribe to their
`local/files/{root}/*` with the payload included → `workbench/blob-resolve` → pull the blob
closure across → dispatch `local/files:write` locally*. Durable half at
`app/workbench/syncs/{peerID}.{root}`, restored by `shellboot`.

**The reason this was a wiring job and not a build is the finding.**
`workbench.BlobResolveHandler` is the entire cross-peer materialization pipeline and it had
**twelve test files** behind it — one-way, bidirectional, burst writes, a 4 MB file, capability
delegation, late join, self-loop. Every one of those registrations was in a `_test.go`.
`shellboot` registered ingest and chain-errors and not this one, and `subscription` is
`ls|inspect|rm` with no create, so **the whole thing was built, tested, and reachable from
nothing a user could run.**

That is D23 at the **handler** layer, and `make reachability` passes because the sweep asks
whether a *model* has a surface. A handler is not a model. §0L already noted that a read-only
surface over a read-write model is invisible to that sweep; this is one further out — no surface
at all, and nothing looking for it. The question the sweep will not ask for you: *what registers
this in a shipped binary, and what verb causes it to be used?*

`sync` **refuses without a local mount to receive into.** A sync writes into a mount; it does not
create one. Choosing where an operator's files land is not a default, and a relationship that
establishes cleanly and then 404s on every delivery is the failure this repo has now shipped
twice — the ingest mapping, then the mount binding. The third time it becomes a precondition.

### Every subscription was dead after a restart, and every mount with it

Found while scoping the above, not by any suite.

`subscription.Engine.Load()` rebuilds the engine's runtime index from the
`system/subscription/{id}` entities in the tree. Its own doc comment says it **must** be called
after `SetLocationIndex` and before `StartDelivery`, and says why — *"subscriptions are classified
as PERSISTENT extension state, so the durable copy in the tree is authoritative and the runtime
index is a derived cache that must be rebuilt on boot."* `entity-core-go`'s daemon calls it.
`entitysdk.assembleAppPeer` made both neighbouring calls and not that one.

So a reopened peer had every subscription it had ever made sitting in its tree, and none of them
live. **Every mount in this repo is driven by a subscription**, so the shipped behaviour was: the
watcher resumes, `mounts` lists it healthy, and no document ever appears again.

That is precisely the end state `workbench/mount_binding.go` was written to fix six days ago,
reached by a **second independent route the fix could not close** — and the mount-binding work was
inside this code and did not see it, because it was hunting a fact held in one process's memory
while this is a fact held in the tree that nothing reads back. Two failures wearing one symptom;
finding either is no evidence about the other.

**AP39's shape one extension over, and worse in kind.** There the volatile index was the query
index and `find`/`grep` answered wrongly. Here the volatile index is the subscription engine's,
and a subscription is not a read path — it is what makes a write *cause* something, so the
failure is not a wrong answer but no answer.

Two instances of one shape is the ladder's promotion trigger: **D26** is ratified — *a persistent
store does not make a derived runtime index persistent; name what rebuilds it at open, and gate it
across a process boundary.* Both instances were **adopt, not build**: the kernel shipped the
answer and we had never called it, which is D20 pointed at initialization.

**The gate had to be built twice to be worth anything.** Its first version asserted the
subscription entity was in the reopened tree — which it is, in both builds, always. Only the
reopened *engine's* subscriber count separates them. And the pattern key is now read back off the
stored entity rather than reconstructed, because the engine indexes on `sub.Pattern` verbatim and
the hand-built key was simply wrong: the test's own premise check caught that on the first run and
reported it as a broken test rather than a broken engine, which is the only reason it did not
become a false finding.

### A stale status line that said the opposite of the truth

`TestStage3_Case2_Bidirectional`'s header said **"CURRENTLY SKIPPED pending core-team / arch
investigation"** and *"fully symmetric peer-to-peer does not [work]"*. F9 was closed by core-go at
`8ad52bc`, the skip came off, and the test has been passing ever since — with that paragraph
above it. So the file asserted in its own header that the default deployment shape was broken
while the code beneath it proved nightly that it was not, and the constraint outlived its fix in
our planning documents. Corrected in place, history kept and marked as history.
`FILE-REPLICATION-LANDSCAPE.md` §6 inherited the same error and is corrected too.

### What M2 does not cover, named rather than implied

Two peers on **one host** over loopback TCP. Two physical machines is untouched — the addressing
and NAT story is not started and `ext/relay` is unlanded upstream. **No GUI affordance**: the
verbs are shell-only, and the Local Files panel still manages mounts and says nothing about syncs.
And a sync delivers changes from the moment it is established; it does not replay history, which
the verb prints rather than leaving an operator to watch an empty directory.

### Verified

`make test-each` **10/10** with M2 and the subscription fix in.
`lint`, `gofmt`, `reachability` clean. The M2 end-to-end gate builds both peers through the real
`shellboot.Bootstrap` and was **verified to fail before it was trusted** — deleting the
blob-resolve registration fails it with *"shellboot did not wire the blob-resolve handler"*, which
is the pre-M2 state reproduced. An earlier draft of that test assembled its own handler list and
would have stayed green through exactly that deletion; that is the same defect as AP61 one layer
up, caught in review rather than by a suite.

## §0N NEW (2026-09-02) — the panel threw on every file, and this time the forensics worked

§0M shipped the Files panel and closed by naming what had not been done: *"nobody has driven this
by hand. It is headless plus the Go suites."* An operator drove it the next morning. The report:
mounted a folder, saw the files, clicked them, **the preview pane never updated**, and after a few
more clicks the app died.

Both symptoms are one defect, and it was diagnosed from the run log in a single read.

### One exception, nine times, then the ninth killed it

`FileExplorerPanel.OnRowSelected` formatted the file's mtime with
`DateTimeOffset.FromUnixTimeSeconds`. The field holds Unix **milliseconds** — every producer
writes `info.ModTime().UnixMilli()`, in the kernel's watcher, both sites in its operations, and
our own `mount_sweep.go` — and `FromUnixTimeSeconds` throws above year 9999, which every real
mtime in milliseconds is. So the handler threw on **every file row**, before reaching
`LoadPreview` three lines later. That is the dead preview pane, exactly.

The crash is the same fault counted. `MaxContainedUiFaults` is 8; the log holds nine
`Dispatcher.UnhandledException` records and then an `AppDomain.UnhandledException (terminating)`.
The operator's *"eventually it just crashed"* is the ninth click.

The field is now `ModifiedAtMillis` end to end — model, bridge DTO, panel — because **a doc
comment is invisible from the far side of cgo and JSON**, and this one said "a Unix second" while
every writer disagreed with it. Separately, `FormatMtime` now range-checks: an mtime is a number a
*filesystem* chose, not one we did, and a row that cannot be dated should lose its date, not the
operator their session. The unit was the cause; the missing guard is why it was fatal instead of
ugly.

### Why 138/138 could not see it — two reasons, both worth carrying

**The end-to-end test drove data all the way to the row and never touched a row.**
`A_Mounted_Directory_Of_Mixed_Kinds_Becomes_Browsable_Files` is a real-session test in the strong
sense — real directory, real watcher, real subscription, real ingest, real bridge — and every
assertion in it reads a **row viewmodel**. `OnRowSelected` is reachable only from
`SelectionChanged`. The test stopped one method call short of the code that renders what it had
spent seven layers producing, and that gap reads as thorough coverage rather than as a hole.

**And the envelope test's fixture carried a value no producer emits.** It hard-coded
`"modifiedAt": 1700000000` — plausible-looking, in the seconds range, and written by nobody in
either tree. So it asserted the decode and could never catch the unit. That is **AP58's shape at
the level of a value rather than a wrapper**: a fixture that is not what production produces
cannot fail on the difference. AP58 was measured six days ago on a store wrapper; this is the
second instance, one layer over, which is what the promotion ladder calls a different shape.

Now: the mount test **selects every row**, and asserts the detail line carries the current year
(so a unit swap fails rather than silently printing 1970) and that the preview pane updated — the
operator's own sentence as an assertion. `FileExplorerMtimeTests` covers the formatter directly
including the `long.MaxValue` arm. **Verified to fail before trusted**: restoring
`FromUnixTimeSeconds` fails all three with the operator's exception, through the same
`SelectionChanged → OnRowSelected` route the production stack shows. AP61.

**A near-miss in the gate itself, recorded because it nearly cost the whole exercise.** Inserting
the new test class immediately above `FileExplorerPanelMountTests` silently stole its
`[Collection(nameof(BridgeCollection))]` attribute. The first falsification run duly reported
3 failures — and one of them was *"constructor parameters did not have matching fixture data"*,
meaning the row-selection arm had never executed. The failure **count** was right and the failure
**reason** was wrong, and the count alone would have been read as proof. A control run that fails
for the wrong reason is not a control run.

### The instrument question, answered

The operator asked whether the diagnostic work is paying off. On this one it paid completely,
and it is worth being precise about which pieces did it:

- **The run log survived.** AP54 moved run logs out of `dist-native/`, which `extract` deletes on
  every build. The 2026-09-01 crash lost its stderr to exactly that. This one is 3,285 lines at
  `avalonia/run-logs/run-20260902-083012.log`.
- **`run-with-dump.sh` was armed without anyone choosing it** — until 2026-09-01 `gui-run` exec'd
  the bare binary and whether a session had diagnostics depended on which target you typed.
- **The breadcrumb ring named the click.** The last entry before the fault is
  `input: press @547,690 on TextBlock "  ENTITY-COMPUTATION-MODEL.html"`.
- **The fault channel printed `file:line` on the first fault**, so no coredump was opened, no
  `si_code` was read, and `crash-stack` was not needed. The doctrine's whole point is that the
  intuitive order wastes days; here the first signal was sufficient and the rest stayed unused.
- **Containment turned one fatal click into eight recorded ones.** Nine identical stacks is
  strictly more evidence than dying on the first, which is what the bound was for.

**What did not work is the suite**, and that is the finding. The forensics chain caught a defect
the tests were structurally unable to reach.

**One honest observation, not acted on.** Containment is silent *to the operator*. Eight faults
went to the log and the crash trail and nothing appeared on screen, so from the user's seat the
preview pane was simply dead until the app vanished. That is arguably correct — a UI that spams
exception dialogs is worse — but "contained" currently also means "invisible", and AP49's rule
that a surface must say so points the other way. Flagged for a decision rather than changed.

### Verified

`make test-each` **10/10**. Avalonia headless **141/141** (was 138 — the three new tests).
`lint`, `textual`, `reachability`, `gofmt` clean.

## §0M NEW (2026-09-01, fifth pass) — a mount you can actually use

§0L shipped the ability to mount a directory from the GUI. The operator's report on it, verbatim:
*"I mounted the thing. Wasn't able to really do much with it after that."* That was accurate, and
tracing why turned up four defects, two of them shipped and invisible.

### The Local Files panel showed no mounts at all on a real peer

The worst of the four, and it had been there since the panel was written. **`Store.List(prefix)`
takes a tree-relative prefix and returns PEER-QUALIFIED paths** — the prefix is canonicalized on
the way in, the entries come back carrying the `/{peer-id}/` they are stored under. So
`strings.TrimPrefix(e.Path, "system/config/local/files/")` trimmed nothing, the
`strings.Contains(root, "/")` guard after it rejected every row, and the panel rendered
**"no filesystem mounts on this peer"** for a peer that had just mounted one. The mount had
succeeded, written its config and started its watcher. The shell's `mounts` verb had the same
defect one shade milder: it printed the qualified path as the root *name*, which is not a name
`unmount` accepts.

**Why nothing caught it is the transferable half.** The cheap test store —
`NewStore(memory, memory)` — has no namespacing index, so `List` returns bare relative paths and
the arithmetic works. Every model test in the package used it; the namespacing appears only on a
store from `CreatePeer`, which is what every real peer has and what no unit test had. **A fixture
that omits a wrapper the production object always has cannot fail on anything the wrapper
changes**, and this wrapper changes the return-value *shape* of the most-used read in the
codebase. Now: `workbench.TreeRelative` / `RelativeUnder` (one helper, replacing two private
copies that between them showed the class had already bitten twice without being named), five
call sites corrected, and `workbench/tree_path_test.go` — a separate file **on a peer-backed
store**, because the store being real is the entire point. That test asserts its own premise out
loud, so if the store ever stops namespacing it reports the fix as dead code rather than passing
for a new reason. AP58.

### Everything that was not markdown was dropped on the floor, silently

The ingest handler classified with one predicate, `isMarkdownPath`, and returned
`type_not_handled` for everything else: no typed entity, no tree binding, no surface told. So a
mount of a directory of code, images or plain text produced a full source layer and **not one
document anyone could open**.

And the mount row's file count came from the *source* layer — where the kernel's watcher writes a
record for everything it admits, the layer that has no failure mode. A mount of 400 photographs
and one README read as **"401 entities in tree"**. A number that goes up is the most reassuring
thing a panel can show. AP59.

`workbench/doc_types.go` is the registry the old comment promised: markdown, text, code, image,
and `doc/binary-file` as an honest fallthrough. **There is no "unhandled" outcome any more** —
`doc/binary-file` says *this is in your tree, addressable, and we are not going to pretend to
render it*, which is a fact an operator can act on; `type_not_handled` was a fact only we could
act on, and we didn't. Three decisions worth flagging:

- **Classification is by name, never by content.** Reading a 4 GB video's first chunk to learn
  what its extension already said is a cost with no answer at the end of it. A misclassified file
  is a wrong label on a row that still opens; a slow mount is not.
- **Markdown keeps its own struct, byte for byte.** Widening it with three optional fields would
  almost certainly have preserved the encoding — and "almost certainly" is the wrong confidence
  level for a change that re-hashes every document in every existing mount if it is wrong. A test
  pins the four-field shape rather than leaving that as a paragraph.
- **Mount validation had to widen with it.** It refused a target prefix holding types the mount
  did not own, and that set was the single `doc/markdown-file`. Left alone, the first remount of a
  mixed directory would have conflicted with what its own predecessor wrote.

### A mount stopped ingesting after a restart, and looked healthy

A mount is two records. The kernel persists its half and rehydrates the watcher at startup. Ours
— the source→target mapping the ingest handler routes on — lived **only in process memory**. So
after a restart the watcher resumed, wrote its file entities, and every delivery answered
`404 no_mount_for_uri`. `mounts` still listed the mount, because that reads the kernel's record.
Nothing was wrong except that no new document ever appeared again.

Now at `app/workbench/mounts/{root}`, restored by `shellboot`, removed by `unmount` (or the
unmount undoes itself at the next launch), unwound by every failure path in between. Worth noting
how it was found: **not by a restart, but by a second consumer** — the new explorer needed the
target prefix and the only source was a handler's private map. **A fact held in exactly one
process's memory reads as an internal detail until something else asks for it.**

### The panel that shows files

`Files (browse a mounted folder)` — folder tree, breadcrumbs, size column, text preview, and a
**status on every row**: the kind if it became a document, and the reason if it did not. The
summary reports total, ingested and not-ingested separately, because one count cannot tell a
healthy mount from one with nothing openable in it.

`Local Files` keeps the mount administration and is renamed to say so. Two panels, one question
each — the rule the browser trio already earned.

Avalonia has no built-in file explorer control; the framework's own sample is built on a separate
`TreeDataGrid` package, and we deliberately did not take it. A new templating surface is the
container-teardown hazard re-opened somewhere our one guard does not look.

### One setting that existed in the substrate and in no surface we ship

`ReadOnly` has been in the kernel's mount config from the start. Nothing in this repo could set
it: the request struct hard-coded `false`, the verb had no flag, the form had no checkbox. So
*send-only* — the mode an operator reaches for the first time they mount a directory they do not
want written to — was expressible in the substrate and unreachable from the product. Now
`mount -readonly` and a checkbox, and **every mount states its write policy**, not only the
unusual ones. `FILE-REPLICATION-LANDSCAPE.md` §2a is the new inventory this came out of: what a
folder-sync tool exposes, against what we do.

### A deadlock the tests could not report

Found the way these are always found: the sweep stopped. `make test-each` sat on the `workbench`
suite for **sixteen minutes** with no output, and would eventually have reported a 30-minute
timeout naming no test and no reason.

`Store.OnPrefixChange`'s cancel closes the watch and then *waits* for its delivery goroutine to
exit — and that goroutine is inside the model's event handler, taking the model's lock. A `Close`
holding that lock across the cancel is the two of them waiting on each other. No panic, no race
report, nothing. The rule the model now states in one line: **never call into the store while
holding the model's lock.** The attach side is the same hazard and is louder only by luck, because
the subscription delivers its seed synchronously on the caller's goroutine when the store has no
watch hub.

**The second-order finding cost more than the bug.** The first regression test written for it
*passed against the deadlocking version*, and was about two minutes from being committed as a gate
that proved nothing — "verified" by a green run on the already-fixed code. One close under churn
does not reliably catch the deliverer mid-contention; forty open/close cycles against a
continuously-written store deadlocks the broken version in seconds. **A race gate needs repetition
and a control arm**, and the rule that a probe which does not reach the mechanism refutes nothing
applies just as hard to a probe that cannot fail.

The fix had its own trap, caught by a second test: the first correction cleared the renderer's
wake callback on every re-bind, which would have left the panel live-updating for exactly one
mount and silently static for every mount chosen after it.

### Verified

Avalonia headless **138/138** (was 134), including an end-to-end test that mounts a real directory
of mixed kinds through the shipped bridge and asserts each file reaches its own `doc/*` type.
**That test was verified to fail before it was trusted** — restoring the markdown-only gate makes
it report `blob.bin(no-doc), notes.txt(no-doc), readme.md(doc)` and *"3 with no document yet"*,
which is the pre-registry behaviour reproduced. A probe that does not reach the mechanism refutes
nothing, and a probe that cannot fail proves nothing either.

**Not done, and named rather than implied:** nobody has driven this by hand. It is headless plus
the Go suites. And the panel has no image preview — an image row says what it is and its size,
and shows nothing, which is the honest version of a gap rather than a fix.

## §0L (2026-09-01, fourth pass) — the GUI can mount a directory, and it remembers your workspace

Two pieces of the application surface, both entirely ours, neither waiting on anything.

### The Local Files panel could list mounts and not make one

M0 landed the panel; the verbs it was supposed to come with did not. So the only way to mount a
directory in the shipped GUI was to open a Shell panel and type the sentence — with the mount
pipeline (validate the target, mint the scoped chain capability, register the source→target
mapping, persist the RootConfig, subscribe, start the watcher, record the subscription) living
inside `cmdMount`, reachable only from an argv.

**`make reachability` passed the whole time, and was right to.** The sweep asks whether a model has
*a* surface, not whether the surface can do what the model does. **A read-only surface over a
read-write model is a D23 violation the sweep cannot see**, and that is the transferable part: the
tell is a panel with no verb in it. Worth a second look across the other seventeen.

The fix is the extraction `AGENTS.md` already asks for — *DRY the integration, not the renderer*.
`shellcmd/mount_op.go` holds `ShellWorkspace.Mount`/`Unmount`; `cmdMount` is now flag parsing and
phrasing over it, and the bridge calls the same function. Ordering is preserved verbatim, including
the two comments that explain why (subscribe *before* the watcher, because its initial scan writes
synchronously; unwind in reverse on every failure).

One thing changed shape rather than moving. A target-prefix conflict was a formatted multi-line
string — fine for a terminal, useless to a panel that wants to list what is in the way and offer to
proceed. It is now a typed `MountConflict` whose `Error()` renders the exact text the verb used to
produce, so the shell surface is unchanged and the panel gets the structure. The GUI shows the
offending types and a **Mount anyway** button.

**Two things the panel says out loud because it would otherwise be lying by omission.** Unmount
does **not** stop the watcher — the kernel exposes `StartWatching` and no `StopWatching`, so the
fsnotify watcher survives until the process exits (bounded, not leaking: remounting the same root
replaces it). And a sweep names every binding it removed rather than counting them, because a count
alone asks the operator to trust a destructive operation they cannot inspect.

### The workspace reset itself on every launch

`new PanelStack(peerH, this, "site-view", "detail", "shell")` — three hard-coded names. Open the
Browser and Local Files, close Detail, restart: the original three, forever, with no indication
that anything had been remembered or forgotten. That was the largest single piece of friction in
the app and it is the operator complaint from §0E that had gone untouched longest.

Now persisted, and **the interesting decisions are about where and what**:

- **A file, not the tree.** The entity-shaped answer is `app/state/…`, and it is wrong here for a
  blunt reason: with no flags the GUI is an ephemeral in-memory peer and loses everything on exit,
  so a tree-stored layout would vanish at exactly the moment it is meant to help — the default
  configuration. It would also have quietly settled the open §0G design fork about per-window
  state identity. `~/.entity/gui-layout.json` avoids both.
- **Keyed by alias, not peer-id.** Peer-id is the better identifier nearly everywhere else in this
  codebase and the wrong one here: an ephemeral peer gets a fresh keypair every launch, so a
  peer-id-keyed layout would never match itself twice and the feature would silently do nothing
  by default.
- **The policy is in Go** (`workbench/layout_config.go`) even though panel names mean nothing to
  it. Where the file lives, precedence, boundedness, and that a file which exists and does not
  parse is an **error** rather than a silent reset (AP33) are not renderer-specific; only the
  strings are, and the model never interprets them. It also means the rules are covered by a suite
  that runs in milliseconds rather than by a GUI test someone remembers to write.
- **A corrupt layout opens on the defaults AND says why.** Silently substituting them is
  indistinguishable from *"it forgot again"* — which is the complaint the feature exists to answer.
  Same for a panel kind this build does not register: skipped so a file written by a newer build
  still opens, and named on the status line so the panel does not just disappear.

The signal is `PanelStack.LayoutChanged`, fired on add, close **and swap-in-place**. The swap is
the one that is easy to miss — it changes no slot count — and missing it gives a workspace that
remembers the panels you added and forgets the ones you changed your mind about, which is a
stranger thing to diagnose than forgetting everything.

**A hazard caught before it shipped, and it is the AGENTS.md note pointed at new code.**
`WB_LAYOUT` **cannot be set from a managed test**: Go captures its environment once at process
start, so a C# `SetEnvironmentVariable` calls libc `setenv` and never reaches `os.Getenv` in the
bridge. A headless test of this feature would therefore have written to the developer's real
`~/.entity/gui-layout.json`, which is the "no suite may depend on state outside the tree" rule
broken in a new place. The answer is the one this repo already established for
`BrowserPanel.AutoPinOnOpen`: an in-process switch (`LayoutSetPath`), not an env var.

The end-to-end coverage is `PeerViewLayoutPersistenceTests` — arrange, dispose, re-open, assert the
arrangement came back — and it crosses the whole seam, because everything underneath was already
green while nothing connected it, which is the shape this repo keeps finding by audit.

**State:** Avalonia headless **134/134** (was 117). `make test-each` 10/10, exit 0. `make lint`,
`make textual`, `gofmt`, `make reachability` clean.

## §0K (2026-09-01, third pass) — M1: the reverse-write clock, and the defect was one direction over from where we published it

M1 on the file-replication plan is *"replace the clock with content"* in `ext/localfiles` — a
correctness fix in the sibling kernel, so our half is the reproducer and the ask. Both are done:
`workbench/localfiles_reverse_window_test.go` — six arms, ~8 s, no network, green under `-race` —
and the ask is routed with it.

### What the clock actually does

`reverseWriteLoop` drops any **tree change** event for a path the handler wrote to disk inside
`recentWriteWindow = 5 * time.Second`. The event is **discarded, not deferred** — no queue, no
retry, no second look. Measured:

| Arm | Result |
|---|---|
| write A, then write **B ≠ A** to the same path inside the window | disk holds A, tree commits B, nothing errors |
| the same, then wait 6 s with no further events | still A — and re-emitting the **identical** event after expiry delivers it |
| write, then **delete** the binding inside the window | tree unbinds, **file stays on disk** |

The delete arm is the sharp one: `reverseWriteLoop` routes `ChangeDeleted` before any content is
fetched and `reverseDelete` has no circuit breaker, so for a delete the clock is the *only* guard.

### The finding that makes it a small fix

**The content check already exists** — `currentDiskBlobHash(fsPath, chunkSize) ==
fileData.Content`, twenty lines below the clock in `reverseWrite`. Seeding the bytes on disk
directly, so the clock is provably never armed for that path, the echo is still refused: the
content check is the guard. And `markWritten` sits **downstream** of that check, so an echo returns
before reaching it — **the clock cannot arm on an echo.** It arms only after a real tree→disk
write, which is exactly the moment when the next event is most likely to be a genuine follow-up
update. Armed when it is harmful, disarmed when it would be redundant.

The differential is the evidence, not the reading: two tests emit the *same* second event at the
same distance from the first, and it is delivered or lost purely according to whether the first
event caused a write.

### The correction, which is the part worth carrying

**§0H's description of this defect was wrong, and it was wrong in the direction that flattered
us.** It said the window drops a *filesystem* event, so an operator saving their own edit inside
the window has it "discarded as an echo — silent data loss". `isRecentlyWritten` is consulted in
exactly one place, which handles **tree** events; the watcher's `flush` never consults the tracker
at all. Arm 6 measures it with real inotify rather than arguing it from the source: with the clock
armed, the operator's bytes reach the tree 2.0 s later.

The real defect is **worse than the one we published**. A lost operator edit is at least visible to
the operator, who still has the file open. A tree update that never reaches disk is visible to
nobody on either side.

How it happened is the familiar shape: a correct-sounding reading of one function, generalised to
an operation nobody ran (D19 / AP43). §0H even wrote *"it needs a reproducer, because per AP43 a
probe that does not reach the mechanism refutes nothing"* — and then stated the mechanism anyway,
in a bullet a reader has no way to tell apart from the measured ones beside it. **A sentence that
names its own missing evidence still asserts the claim.** Where the conclusion has not been run,
say what is unknown rather than what is probably true.

### And a reach finding that changes M2

**`StartReverseWrite` has no caller in this repo.** One non-test caller exists in either tree and
it is core-go's own `cmd/entity-peer/main.go`. Our peers replicate through the subscription chain
into `local/files:write` instead. So the tree→disk direction this whole defect sits on **is never
started by anything entity-workbench-go ships** — the window is live for `entity-peer` and dormant
for us, which is why routing it did not wait on our adopting anything.

It also means `FILE-REPLICATION-LANDSCAPE.md`'s inventory row calling that direction "live,
**bidirectional**" was a statement about the kernel being read as a statement about us. Corrected
there. M2 (*two machines, one folder*) now has a decision in front of it that the plan did not know
it was making: wire `StartReverseWrite`, or keep replicating through dispatch and say so.

### What we did not establish

Removing the gate is a **hypothesis**, not a verified fix, and the ask says so in its own section.
The clock was originally added to close an unbounded same-path loop under a dual-watch topology,
and **we did not reconstruct that topology.** Arms 4 and 5 support the argument on the tree-event
path; supporting an argument is not running the operation (D19). If removing the gate re-opens
that loop, that is the more interesting result and we want it back.

**State:** `make test-each` **10/10 green, exit 0** — `shellcmd` 290s · `sdk` 220s · `programs`
160s · `shellboot` 15s · `workbench` 12s (the six new arms included) · rest ≤4s. `make lint`,
`make textual`, `gofmt`, `make reachability` clean. **No product code changed this pass** — the
diff is one test file, one routed ask, and the corrections the reproducer forced.

## §0J (2026-09-01, second pass) — it was not a stack overflow, and how we came to say it was is the finding

§0I, written hours earlier, classified the SIGSEGV as a managed stack overflow. **That is
retracted.** The retraction came from the same coredump, and getting it took one pass over the
stack rather than a new experiment — which is the part worth carrying, because it means the
refuting evidence was in hand the whole time.

### The measurement

`coredumpctl info` printed 45 consecutive frames returning to one address and stopped. §0I
reasoned: systemd truncates a backtrace, so the real depth is larger, so this is a call site
recursing without end, so it is a stack overflow. Scanning the stack instead of counting the
printed frames:

| | |
|---|---|
| occurrences of the repeating return address | **exactly 45** — not truncated |
| frame stride | **448 bytes, uniform across all 44 gaps** |
| span of the recursion | 19,712 bytes (19.2 KB) |
| depth of the whole frame chain below the thread descriptor | **24,424 bytes (23.9 KB)** |
| stack available to any thread CoreCLR creates | ≥ 1 MB |

**24 KB of a megabyte is not an overflow.** A uniform-stride run 45 deep is an ordinary
recursive walk — a UI tree is tens of levels deep by construction. Two further facts kill the
attribution outright: the faulting thread is a **background** thread (LWP 1722860; the UI thread
is 1722846 and was blocked in a syscall), and it carries **`libSkiaSharp`** frames, so it is the
render side. `MarkdownRenderer`'s inline walkers — the unbounded recursion §0I bounded — only
ever run on the UI thread.

`MaxInlineDepth = 64` **stays**, because an unbounded recursion over a remote page body is a real
defect on its own terms. It is hardening, and it fixes no observed crash. The source comment and
the AP53 catalog entry both said otherwise and have been corrected in place, because a wrong
conclusion left in a doc comment is invisible to review forever (AP45's own lesson, pointed at
ourselves).

### What is actually established, and what is not

**Established.** The thread was 45 levels into a uniform recursive walk. The repeating frame's
machine code iterates a children array, copies a 72-byte by-value struct (nine doubles — the
shape of a 3×3 transform matrix) onto the stack per child, dispatches through an interface slot,
and OR-accumulates two booleans out of the returned struct. The innermost frame computes min/max
over an array of 16-byte (double, double) pairs — a bounding box over points. Stack words nearby
are layout-scale doubles (≈495.0, ≈1064.2). That is a geometry walk over a visual tree on the
render thread.

**Not established: what faulted.** And here the artifact is worse than §0I assumed. `si_code` is
128 (SI_KERNEL) with `si_addr` 0 — the AP34 re-raise signature, already known. Beyond that: the
recorded `rip` disassembles to `vucomisd %xmm5,%xmm0`, a **register-to-register compare that
cannot fault at all**, and the recorded `rsp` points into memory **the core does not contain** —
it is on the alternate signal stack, which systemd does not dump. So on a systemd core for a .NET
crash, the register set does not merely describe the handler; it describes memory that is absent.
Believe the stack geometry; believe nothing else.

### The reason every crash costs days, which is a defect in our tooling and not in the crash

Two of them, both ours, both found by asking why the artifact was so thin.

**The evidence existed and the build deleted it.** `make up` tees the run's stderr — which for
this crash carried `WB_PANEL_LOG=1`, i.e. **the complete breadcrumb stream up to the fault** — to
`dist-native/run.log`. `make extract` does `rm -rf` on `dist-native`, and every build target runs
`extract`. So the log was destroyed by the next build, and it was. Our own crash doctrine already
says *"write artifacts outside `dist-native/` or copy them immediately"* — we wrote that rule
about smoke-run artifacts while the most important log in the repo sat in the directory the rule
names. Run logs now go to `avalonia/run-logs/`, timestamped rather than overwritten, because the
interesting run is rarely the most recent one.

**A first attempt at this paragraph claimed something stronger and false, and the correction is
worth more than the claim was.** It said *every operator-reported crash was captured with
createdump switched off*, reasoning from a real code fact — `run-with-dump.sh` was copied into
place by `make up` alone, so `make host-run` (`make gui-run` at the root, which this file's own
`AGENTS.md` calls the documented fast loop) exec'd the bare binary. Plausible, and wrong: the
crashed process's environment is **in the coredump**, and it says `DOTNET_DbgEnableMiniDump=1`
and `WB_PANEL_LOG=1`. The operator used `make gui`. **createdump was enabled and did not fire** —
the same outcome as 2026-08-21, which `CrashDiagnostics`'s own header records and which nobody
re-read. That is D25 again, inside the entry ratifying D25, caught only because the claim was
measured before it was committed.

What survives, and is fixed: `host-run` really was unarmed, so whether a session had diagnostics
depended on which target the operator happened to type and **nothing recorded which**. Both
targets now share one wrapper, which also sets `DOTNET_PerfMapEnabled` — genuinely absent from
the crashed process, and without a perf map a JIT frame can only be identified by
hand-disassembling it, which is exactly what this session spent its first hours doing.

What is **not** fixed, and is now the sharpest open question on this crash: **createdump is on
and produces nothing for this fault class.** `CrashDiagnostics`'s hypothesis is that by the time
the process faults there is no stack left for createdump to run on. That is a hypothesis, not a
finding, and it has never been tested.

**The breadcrumb guarantee described a code path that did not exist.** The ring reached disk only
from `WriteFatal`, the *managed* fault handler — precisely the path a hard SIGSEGV does not take —
while the class comment promised "the breadcrumb ring on disk up to the last flushed line".
Nothing flushed it. Measured cost: the crash log's last breadcrumb is timestamped **seventeen
hours** before the fault. Breadcrumbs now write through to `~/.entity/crash/<pid>.trail` as they
are recorded, line-buffered onto the fd so the bytes survive a signal.

All three are **AP54**. Every crash artifact now also states which instruments were armed —
`external diagnostics: minidump=… perfmap=… panel-log=…`, on stderr at startup and in the trail —
because a missing minidump reads identically whether createdump was off or whether it ran and
found nothing, and those two send the next session to different questions. This session spent
real time on the wrong one of those two, and the app could have answered it in a line.

### The reach gap, named and closed

Nothing in this repo could perform the gesture the operator was making. `smoke-xvfb-click` has a
real compositor and a real X11 backend and could only **press and release at one point** — no
pointer capture, no motion stream to a captured element, no scrollbar thumb. The headless harness
can drag (`TreeViewScrollDragTests`, added in §0I) but has no X11 backend and no render thread.
The crash is on the render thread during a drag, i.e. in the intersection neither one covered.
`smoke-xvfb-click` now does real drags — `DRAG_PCT` of gestures, half of them near-vertical
because that is the scrollbar-thumb shape and uniformly random endpoints almost never produce
one — and `crash-hunt` sweeps them. `DRAG_PCT=0` is the control arm.

### New instrument: `make -C avalonia crash-stack`

`avalonia/crash-stack-report.py`, run automatically inside `make crash`. Reads an ELF core with
no debugger, reports per-thread frame stride, uniformity, span and depth below the thread
descriptor, and prints the overflow / not-overflow verdict **with the numbers behind it**. It
also detects the case above — a recorded `rsp` absent from the core — names it as the
handler-on-altstack signature, and falls back to `fs_base` to find the real stack. Validated
against the 2026-09-01 core: it reproduces the hand measurement and returns the opposite verdict
to the one that was published.

### Ratified: D25

**A field that PRINTS is not a field that ANSWERS.** Second instance of the same failure, which
is what promotes it out of the catalog: AP34 read `si_addr` off a re-raised signal; AP55 read a
recursion depth off a truncating pretty-printer. Both fields render in a tidy table beside fields
that *are* authoritative, so the artifact reads as one coherent statement when it is several of
different strength. The rule: name what produced a number and what it is guaranteed to describe,
before it classifies anything. Prefer a derived measurement to a printed count — "45 frames" is a
printout, "45 × 448 bytes against ≥1 MB" is a measurement, and only the second one classifies.

### Still open

The specific fault is **unidentified**, and this entry does not pretend otherwise.

The next run that crashes will have, for the first time, a **JIT symbol map** and a **breadcrumb
trail that reaches the fault**, and its run log will survive the next build. Those three are
measured, not hoped for. What is **not** promised is a managed stack: createdump is enabled and
did not fire for either of the two crashes we have checked, so the minidump channel is
**believed broken for this fault class and untested**. Finding out why is the next piece of work
on this crash, ahead of any further theorising about the fault itself — a working createdump
turns this class from archaeology into a stack trace, and nothing else on the list does.

The drag fuzz is the instrument to point at the fault once there is a channel to read it with.
Eight seeds × 140 gestures with drags at 40% ran clean today; per D24 that is a bound on the
search, not a result — 45 s of synthetic gestures is not 17 hours of a real session, and the
operator's crash followed a long idle.

**State:** Go `make test-each` **10/10 green, exit 0**. Avalonia headless suite green (three new
`CrashTrailTests`). `make textual`, `gofmt`, `make reachability` ok.

## §0I SUPERSEDED (2026-09-01) — the SIGSEGV was a stack overflow, and the coredump said so before any symbols

> **This section's conclusion is wrong and is kept for the record.** The measurement in §0J
> refutes it: 45 frames at a uniform 448-byte stride is 19.2 KB, not a truncated runaway, and the
> faulting thread is a background render thread the markdown walkers never run on. Read §0J.
> What survives from here: the depth bound is still correct as hardening, and the note about the
> DBus exception not being the crash is still right.

The operator crashed the GUI adjusting the tree's vertical scrollbar. Exit 139, no crash log.

**`coredumpctl info` classified it in one read.** The faulting thread carries **45 consecutive
frames returning to a single address** (`0x7f34190c139e`), with systemd truncating the trace — so
the real depth is larger. One repeating return address is not a corrupted stack; it is **one call
site recursing without end**. That reclassifies the bug from "native memory fault" to "managed
infinite recursion → stack overflow" before any symbolication, and it is why nothing was logged:
**a stack overflow on .NET is uncatchable**, the runtime fails fast at the guard page, so there is
no exception, no `Dispatcher.UnhandledException`, and no stack left to run a handler on. AP46's UI
fault containment cannot help — it catches exceptions, and this raises none. Recorded as **AP53**.

Note the earlier `TaskScheduler.UnobservedTaskException` in that log
(`org.freedesktop.DBus.Error.ServiceUnknown`) is **not** the crash: the process kept handling
input for three more minutes after it. The log timestamps are UTC, so the apparent jump from
`21:55` to `14:33` is a day boundary — the app was idle ~17 hours before the fatal click.

**Reach, per the doctrine, and the negative with it.** `smoke-xvfb-click` **cannot reach a drag**
— its own row in the doctrine's table says so, it presses and releases at one point. The headless
harness can: `MouseMove` runs the genuine route with pointer capture. `TreeViewScrollDragTests`
now drives four real drags on the tree's vertical thumb — a sweep, six reversals, a drag off the
window in four directions including an absurd finite coordinate, and unbalanced press/release
pairs. **All four pass**, and the reach that negative carries is: this harness runs the real
managed input stack but **no X11 backend, no window manager, no compositor**. So it rules out the
managed ScrollBar/Thumb logic and nothing below it.

**What we fixed, and what remains unproven.** `MarkdownRenderer.EmitInline` and `AppendText` both
recursed over a parsed inline tree with **no depth bound**, and that tree comes from a *remote
page body* — nesting depth is whoever wrote the page. That is a genuine defect on its own terms:
a hostile or merely pathological document could take the browser down with no catchable error and
no log, which is `AssetNameFromRef`'s hazard class reached through document structure instead of a
URL. Bounded now at `MaxInlineDepth = 64`, refusing to descend and emitting an ellipsis rather
than truncating in silence.

**It is NOT established that this recursion caused that crash.** The JIT frames carry no module
and a systemd ELF core yields no managed stack, so the coredump names the shape and never the
method. What is established: the crash was a stack overflow, and we had an unbounded recursion
over untrusted input. Those are two facts, not one — do not let the next session collapse them.

**Still open on this crash:** the specific recursion is unidentified. The next instrument is
either an xvfb drag harness (`smoke-xvfb-click` extended past press-release, which the doctrine
already names as the missing capability) or catching the first SIGSEGV live under gdb with
`make -C avalonia smoke-xvfb-click GDB=1`, which is the only way to get a faulting-thread register
context that has not been through CoreCLR's re-raise (AP34).

**State:** Avalonia **114/114** (was 106), Go green on touched packages, `make reachability` ok,
`make textual` ok, `gofmt` clean.

## §0H NEW (2026-09-01) — M0: the filesystem mounts have a surface, and the sync landscape is mapped

**The landscape analysis is `docs/architecture/FILE-REPLICATION-LANDSCAPE.md`.** Short version:
`ext/localfiles` in core-go is already most of a sync engine — bidirectional filesystem/tree
sync, fsnotify watcher, a stat cache implementing Git's racy-clean rule, path containment with a
leaf-symlink refusal, FastCDC chunking, tombstones, restart-equivalent mount config, and a
1022-line normative spec. The gap is not plumbing. It is `DOMAIN-LOCAL-FILES` §35: concurrent
same-path writes are **last-arrival-wins with no merge**, which as a product statement means an
edit made on two offline machines silently loses one. Syncthing has not lost that write since
version vectors. The composition nobody has built is `localfiles x revision` — merge instead of
surrender — and that is the only milestone on the plan that is not catch-up.

**M0 is done.** `workbench/local_files_model.go` + `LocalFilesRender` on the bridge +
`avalonia/frontend/Panels/LocalFilesPanel.cs`, registered under "This peer". Before this,
`grep LocalFiles avalonia/ console/` returned **nothing** — a complete kernel extension with no
user-reachable edge, D23's shape for the fourth time. Rows come from the persisted
`system/config/local/files/{root}` configs, which is the same source restart-equivalence reads,
so there is no second workbench-side mount list to drift.

Two things the model deliberately refuses to do. It does **not** claim watcher liveness:
`WatcherConfigData` is built as the response to a `watch` op and never written to a tree path, and
the handler's live set is unexported, so every row carries `WatcherObservable=false` and the panel
prints *"watcher: unknown"*. Inferring "configured" means "running" would be the AP45 move. And a
mount whose config does not decode **still renders**, with the reason attached — dropping it would
report a broken mount as no mount at all.

**The core-go tracker is `reviews/CORE-GO-TRACKER.md`**, a living index of all seven open asks
plus what they have already closed. Two new ones came out of M0, both in `ext/localfiles`:

- **The reverse-write loop guard is a clock, not a content check.** `recentWriteWindow = 5 *
  time.Second` drops an event for a path the handler wrote to disk in the last five seconds.
  > **Corrected 2026-09-01 by the M1 reproducer — read §0K.** This bullet went on to say the
  > dropped event is a *filesystem* event, so an operator's own edit inside the window is
  > "discarded as an echo — silent data loss". **That is wrong in both mechanism and consequence**,
  > and it is wrong in the direction that flatters us: the real defect is worse. The tracker gates
  > **tree** events only; the watcher's ingest never consults it. What is lost is tree→disk.
  The fix is still to ask whether the bytes on disk are the bytes we wrote — and it turns out that
  check already exists twenty lines below the clock. Routed with a six-arm reproducer.
- **No way to observe watcher state**, per above. Persisting `WatcherConfigData` per root composes
  better than an accessor: it makes watcher liveness a tree fact any impl can read.

**Open, not investigated: a SIGSEGV in the GUI.** pid 1722846, 2026-09-01 09:33 CDT, core present
in `coredumpctl` (28.3 MB), log at `~/.entity/crash/entity-avalonia-1722846.log`. The app had been
up ~17 hours idle; a single click on a `Border` took it down while the operator was rearranging
panel framing. Note the log timestamps are **UTC** — the apparent jump from `21:55` to `14:33` is
a day boundary, not a clock fault. The `TaskScheduler.UnobservedTaskException` earlier in that log
(`org.freedesktop.DBus.Error.ServiceUnknown`) is **not** the crash: the process kept running and
handled input for three more minutes. Deliberately not chased yet — `DOCTRINE-CRASH-FORENSICS`
says reach before forensics, and the first question is whether `make -C avalonia crash-hunt`
reproduces it from a seeded click sweep.

**State:** Go suites green on the touched packages, Avalonia **106/106** (was 101), `make
reachability` ok, `make textual` ok, `gofmt` clean.

## §0G NEW (2026-09-01) — browser-rust read our tree and found two spec defects we could not see

`entity-browser-rust` reviewed `entitysdk/workspace_state.go` and sent three findings. All three
verified against `GUIDE-ENTITY-WORKBENCH-APP` in the arch repo; two are fixed here, one is a
design fork and is below.

**We were accusing conformant peers of non-conformance (AP52).** §5.4 rule 3 retires exactly
three Selection fields — `source_window`, `source_panel`, `content_type` — and MUSTs a WARN when
one is read. Our `legacySelectionFields` had **four**; the fourth was `paths`, which the same
section's schema block declares **live and optional** (*"the wider selection set when the user has
shift-clicked / ctrl-clicked"*). So a correct multi-select emitter got named NON-CONFORMANT on the
one channel the ecosystem has for finding emitters that actually are. The unit test **pinned it**
— it looped over all four names asserting each produced a WARN, so green meant conformant to a
rule nobody wrote. That test is now the regression guard, inverted.

**`app/state/window` MUSTs a `content_type` field and we wrote `content-type`.** §4.2's slot
table is explicit, and the hyphen meant a reader looking for the field the spec names found
nothing — the MUST satisfied in spirit and failed in fact. Now written with the underscore, with
the legacy spelling read as a fallback and **dropped on the next save**, so a tree written by an
older build converges instead of carrying two fields that disagree.

The transferable half is one sentence: **an implementation does not get to retire a field the
schema still declares, or rename one the schema spells.** When a list or a key mirrors a normative
document, diffing it against that document *is* the review.

**Still open — §8's persist-arm obligation, and it is a design fork, not a fix.** An application
persisting per-window state MUST be able to say at startup which window each persisted entity
belongs to, satisfied by *either* an `app/state/window-index` *or* a startup sweep before
allocating any id. We satisfy neither: `window-index` has **zero hits** in the tree, the only
reference to `workspace/windows/` is the path builder, and the sole id allocator is `ws.nextID++`
(`console/workspace.go:88`), a per-process counter that restarts at zero. We persist
(`workspace_state.go:429`) and read back live on a session ordinal (`workbench/log_model.go:143`),
so session 2's window 1 inherits session 1's window 1 state — precisely the third case the rule
exists to prevent. Bites with `-storage sqlite`; the default in-memory peer is unaffected.

It is not a one-liner because the two arms mean different products: the sweep arm makes per-window
state *safely non-resuming* (and log display level stops persisting, which today it does
incorrectly), the index arm keeps resumption and costs a new persisted entity. **Sequencing that
is an operator call**, so it is here and not guessed at.

## §0F NEW (2026-08-31) — an operator drove the browser for real, and it was slow, silent and picture-less

The links landed in the morning (AP47). By afternoon someone used the panel to actually read a
site, and every complaint was a defect we had not measured.

**It was slow because it re-downloaded a verified tree on every click.** Measured against the
live federation with a counting transport:

| | before | after |
|---|---|---|
| open `billslab.com` | 7.3 s / 61 requests | **2.4 s / 61** |
| click a link in it | 6.2 s / 60 requests | **0.36 s / 4** |
| Back | 5.9 s / 60 requests | **0.23 s / 3** |

51 of every 60 requests were the *same* CHAMP nodes, fetched serially at ~90 ms each. The
refusal to cache was justified by a sentence that is true of the **manifest** — the one mutable
pointer in the chain — and false of everything it points at: a content URL's path *is* the
SHA-256 of the body it returns, and a trie rooted at H has exactly one key set forever. The
publisher's own transport profile even says `freshness: "static-immutable+signed-pointer"`, in a
field we parse and never read. `entity-browser-rust` had written the answer in a doc comment:
*"5 fetches for the first page, 2 for the next."*

**The paranoid shape turned out to be the weaker one**, which is the part worth carrying: the
same comment names the `seq` floor, and a `Consumer` rebuilt per navigation cannot enforce it —
so an origin could serve `seq 5` for one page and `seq 3` for the next, a validly-signed replay
of a previous publish, page by page, undetected. We now hold one consumer per publisher per
session and refuse a rollback (`fetch.ErrSeqRollback`). Recorded as **AP48**.

**It jumped around because `Refresh` rebuilt everything, every time.** Three columns re-laid out
per click, the page losing its scroll offset because its content had been replaced. Fixed with
remembered signatures. The trust rail is now **dimmed and labelled** during a navigation instead
of emptied: the page does not become fresh when a navigation *starts*, so the old chain is
exactly what describes the page still on screen, and clearing it deleted a true statement. A
*refused* navigation still clears the page, and that distinction has its own test.

**It did not show the pin, and the reason is nastier than the symptom.** The model computes
`RegistryPinFromOrigin` (AP45's trust-on-first-use flag) and `RegistryRebasedFrom` (AP44's third
provenance state); the bridge sent both; **the panel's DTO declared neither**, and
`System.Text.Json` discards an undeclared member in total silence. `entity-shell` met the
obligation in full and the GUI met none of it, with every test green. The registry identity *was*
rendered — below an expanded seven-field pin form, i.e. below the fold. Identity is now first,
the form is collapsed, and the pin is in the chrome. **AP49.**

**There were no images because we had implemented two thirds of the site convention.** A site is
`manifest` + `pages/` + **`assets/`**, and we had no path helper, no resolver branch and no
renderer for the third. On billslab's methodology site that is **665 of 966 committed keys**.
Two independent reasons it was invisible: the wire grammar is a directive,
`::embed[caption]{ref=assets/figures/x.png}`, which every markdown parser renders as literal
text; and Markdig models an image as a `LinkInline` with `IsImage=true`, so a figure that *did*
parse would have become a clickable link to a page no site commits. Now live: **13/13 figures on
`gallery/biology-chain-1` render as PNGs**, and the three hostile refs a page body could write
(`https://tracker/…`, `assets/../../secret`, `/etc/passwd`) are refused by `AssetNameFromRef`,
whose vectors are the reference's own. **AP50.**

**The "unsupported markdown block" placeholders were mostly bugs, not TODOs.** A link-reference
definition group renders as *nothing* in every markdown implementation — printing a placeholder
announced a failure where the correct output is silence. An HTML block is lowered to its text.
Tables are built as real `Grid`s inside an `InlineUIContainer`, with cell content going through
the ordinary inline emitters, so a link or a figure inside a cell still works.

**The raw HTML was the papers, and they are big.** `SitePage.format` admits `html` — the
web-tier escape hatch for a pre-rendered document — and billslab publishes 23 of them, the
largest **8.27 MB**. The panel's DTO never declared `BodyFormat`, so HTML went into Markdig and
rendered as its own source, and the raw body crossed cgo as an 8 MB JSON string on every render.
It is now lowered to text with an honest note, capped at 256 KiB, and the bridge sends only the
projection. Why not a WebView — and what to build instead, which is a *structured* lowering into
markdown so HTML pages reuse the table/link/figure renderer — is
`docs/architecture/HTML-PAGE-RENDERING.md`.

**A cold open is still 61 requests, and that is the completeness proof, not overhead.** The
operator's read after the fix was *"still a little slow"*, and the honest answer is a design
choice rather than a defect. `entity-browser-rust`'s *"5 fetches for the first page"* is a
**targeted CHAMP descent**: follow the key's hash path from the root, four or five nodes deep,
and fetch the page. Ours walks the whole committed trie — 51 nodes — because
`nav.record("target walk", …)` is the one step in the chain *a withholding origin cannot pass*.
Every other step is satisfiable by an origin serving a correctly-signed root that commits to
nothing. A descent proves the page it returns is committed; only an enumeration proves the
origin is not holding the rest back, and the panel's site list needs the enumeration anyway.
So the reference is faster on the first page **because it verifies something weaker there**, and
the comparison is not like-for-like. The cost is paid once per publisher per session — the
second page is 4 requests.

There is a real design option here and it is written down rather than built: render on the
targeted descent and finish the walk in the background, downgrading the trust rail if the
completeness check then fails. That trades a *provisional* claim on screen for latency, so it
needs the rail to be able to say "committed, completeness pending" — a third state, and AP49 is
the standing warning about what happens when a provenance state exists in the model and no
surface says it. Not started.

**Two things this session left alone, both small, neither on anything's critical path.**
1. **The shell does not auto-pin.** `workbench.LoadBrowseConfig` — precedence
   `WB_REGISTRY_ORIGIN`/`WB_REGISTRY_PEER` > `~/.entity/browser.json` > the built-in origin —
   has exactly one caller, `avalonia/bridge/browse.go`. So the GUI opens pre-pinned and
   `entity-shell` still needs an explicit `registry pin <origin>` first. `make reachability`
   passes and is right to: the model *has* a surface. This is a consistency gap between two
   shipped surfaces, which is a shape the D23 sweep does not look for.
2. **The saved panel layout is still the old default** (Local Site / Detail / Shell), so the
   Browser panel is not on screen until it is picked from the picker.

**A hygiene defect caught in the working tree, before it was committed — the mechanism is
invisible and the guard for it was blind at the first attempt.** This session's uncommitted
`BrowserPanel.cs` had grown **17 raw control bytes** — NUL, SOH and STX typed *literally* into
C# string and char literals as list-signature separators. Semantically fine, compiler happy; but
`git` calls any file with a NUL in its first 8000 bytes **binary**, so the 1513-line centrepiece
of this work had no diff, no blame and no merge resolution, and `git diff --stat` reported
`Bin 36537 -> 64640 bytes`. The committed HEAD version was clean, so nothing shipped. Rewritten
as `"\u0000"` / `'\u0002'` escapes — identical characters to the compiler, textual file,
643/51 diff restored. The tell is a source file that `file(1)` calls `data`.

Two things about it are worth more than the fix. **The obvious guard does not work:** GNU grep
cannot match a NUL in a pattern *at all*, so `grep -P '\x00'` over the tree reported it clean
while `od -c` showed the NUL sitting there — a false negative that reads exactly like a pass.
`grep -I` (which classifies rather than matches) and a byte scan both see it; the pattern does
not. And **it recurred inside this same session**: writing the paragraph above into `STATUS.md`
put two real control bytes into a *published* document, because the escape text was interpreted
on the way in. That is the second shape in one afternoon, so the guard is
`scripts/no-control-bytes.py` behind **`make textual`**, validated in both directions — it
fails on the reconstructed pre-fix `BrowserPanel.cs` (all seventeen bytes, at the offsets the
original scan reported) and passes the current tree.

**State:** Go 10/10 suites green (`make test-each`, exit 0), Avalonia 101/101, `make
reachability` ok, `gofmt` clean.

## §0E NEW (2026-08-30, second pass) — the operator used the GUI, and it found the two things the first pass missed

§0D fixed the *refusal* that made a co-hosted registry unreachable. Within the hour the
operator drove the Avalonia **Browser** panel against the live registry and hit two failures
the fix did not touch. Both are recorded because the first pass had already declared the area
done.

**1. "0 names" was a confident wrong answer.** With no pin supplied we adopt whichever peer the
origin features. At the live registry that is the *site* peer, whose signed root honestly
commits **zero** registry bindings — so the panel drew an empty list and said nothing else.
This is AP44's root cause in the shape that **needs no refusal to go wrong**, which is why the
first fix missed it and why it is the worse of the two: a refusal at least names itself.
`fetch.NameSet` now carries `NotARegistry`, `Diagnosis` and `OtherPrefixes`, and every surface
renders the diagnosis instead of an empty list — *"peer 2KEbBKup… is NOT a registry; its root
commits 39 keys — apps/ (34), sites/ (5) — and not one by-name binding. This is almost always
one origin hosting several peers."*

**2. The operator objected to being made to type a peer-id at all — and was right, because the
fact was already on the wire.** `{origin}/entity-deployment.json` is the cohort's deployment
descriptor, and it carries both `name_registry_pin` (origin + peer-id) and `home_site`. Reading
it means typing a bare domain now works: **`entitychurchregistry.org` → 5 names**, no key.

A pin taken from there is **trust-on-first-use and is labelled as such at every surface.** An
operator's pin is the one fact the origin did not choose; this one the origin chose. It cannot
forge a binding for a key it does not hold — but it can hand you one it does. Refusing to look
would have been AP44 a third time: protecting an invariant at the cost of the feature, when the
honest move is to do it and say what it rests on.

**How we missed it is the durable part, and it is now AP45.** `SiteDefaulted`'s own doc comment
named `entity-deployment.json`, described what it holds, and concluded *"so the choice here is
first-in-byte-order."* Every clause true, the conclusion wrong, and the comment names the file
that holds the answer. **A dismissal recorded as a doc comment is invisible to review forever**
— a `TODO` invites work; a paragraph explaining why a limitation is correct closes the question
for every future reader, including its author. Consequence measured today: `billslab.com`
declares `home_site: billslab-main` and we were opening `billslab-entity-system`, purely because
it sorts first. Now we read the declaration — and, because it is unsigned, it may only **select
among sites the signed root already commits**; an origin naming a site the walk does not carry
is ignored, never followed.

**The panels themselves were the third complaint and it was fair.** Eighteen panels in one flat
picker with no ordering, and three of them — `Browser`, `Site`, `Publisher Verify` — read the
same bytes under names that do not distinguish them. `PanelRegistry.Register` now takes a
**category** and a **blurb**; the picker groups by category with the blurb as a tooltip; and the
three are renamed to say what they answer: **Browser — registry + sites on the network**,
**Local Site (this peer's own)**, **Origin Inspector (is it serving what it signed?)**. The
Browser's address-bar example was `docs.entitychurch.org/demo/index`, a domain that does not
exist; it is now the live registry.

**Verified end to end through the panel's own model** (`workbench.BrowseModel`, which is exactly
what `BrowsePin`/`BrowseNames`/`BrowseGo` drive): bare domain → pin discovered from the origin →
layout re-based → 5 names from the walk → open `billslab.com` → `billslab-main/index`, three
sites enumerated, chain green, freshness scoped to the target's `published_at`.

**Tree:** `make test-each` 10/10 · `make lint` clean · `gofmt` 0 · Avalonia headless **74/74** ·
`make consume-live` green.

**Still open from this pass, deliberately:** the **Local Site** panel still opens the bundled
demo, because there is no bridge export listing the local peer's own sites. It is now labelled
rather than fixed, and that is a stopgap. The panel *layout* complaints — one vertical column,
no persistence, no tabs — are untouched; the picker is grouped but `PanelStack` is unchanged.

## §0D NEW (2026-08-30) — we consumed the live federation, and it found a refusal that was protecting nothing

The naming chain works end to end against the live public federation, for the first time:
enumerate a registry by **walking its signed root**, resolve every name through
`EXTENSION-REGISTRY` §6a.4 in full (signature · the `binding.name == asked` association
check · a finite unexpired `ttl` · a revocation probe inside the signed key set), follow a
binding's transport to a **second domain**, verify that peer's own root under a **different
key**, and read page bytes that hash to what that root committed. Five names, all resolving,
plus the follow-through leg.

**Getting there took most of the session, because our own tooling refused.** Every naming
surface this repo ships read the well-known `{origin}/transport-profile`'s `peer_id` as *the*
peer that origin serves, and refused when the operator's pin named a different one. The
registry origin **hosts two peers** — the well-known object features the *site* peer, and the
*registry* peer sits beside it. So the refusal made the cohort's only public registry
unreachable from `entity-fetch`, from `entity-shell`'s `registry`/`browse`/`open`, from
`workbench.BrowseModel` (hence the Avalonia **Browser** panel), and from
`workbench.ConsumeModel` (the **Publisher Verify** panel). Four sites, each independently
phrased as a security property, none ever questioned.

`EXTENSION-NETWORK` §6.5.3 makes that object a **cold-start entry point**, not an exclusivity
claim. We had invented the invariant and then enforced it.

**Two properties made the class invisible, and they are the durable part (AP44).**

- **Every fixture in this tree serves one peer per origin.** Not under-coverage — a
  *mis-shaped* fixture. A fixture that models one instance of a plural relationship cannot
  fail on the plural case, and its greenness is not evidence about it.
- **A false refusal reads as rigor and leaves no wrong answer to catch.** It yields *no*
  answer, which at a consumer is indistinguishable from a broken origin, so the failure is
  attributed outward every time. The tell to grep for is a refusal whose message asserts a
  fact about the world — *"advertises"*, *"is"*, *"would not be the same publisher's"* — then
  ask which spec sentence makes it exclusive.

**The fix is a split, not a relaxation: a derivation is safe exactly when someone else's key
checks it.** `fetch.Layout.RebaseTo` re-points an origin's advertised layout at a co-hosted
peer — origin-level fields (`content_url_prefix`, `content_layout`, both suffixes) verbatim,
and whole path **segments** equal to the featured peer-id substituted in `tree_url_prefix` /
`manifest_url_prefix`. Segment-exact, never substring. Consumer paths take it, because the
next fetch is that peer's own signed root and a wrong substitution cannot produce a verifying
one — it fails closed. **`registry issue` still refuses**, because there the derived reach
goes into a binding *we* sign, our signature would be the only thing asserting it, and
nothing downstream could catch a bad one.

Layout provenance is now **three** states everywhere — discovered / re-based / pinned. A
surface that collapses re-based into discovered tells an operator the origin advertised a
layout it never did.

One live-tree defect fell out on the way: `BrowseModel`'s hop-2 peer-id-address branch loaded
the origin's featured layout and never compared it to the peer the user addressed, so against
a co-hosting origin it would have walked the **wrong peer's** root and rendered it as the
answer — silently. Now re-based onto the addressed peer.

**New instrument: `make consume-live`** — the whole naming chain against the live federation.
Deliberately **outside `test-native`**, same rule as `crossimpl-go` and one step stronger: it
reaches the public internet, and a sweep that can go red for a domain's reasons teaches
people to ignore the sweep. The offline half is `fetch/rebase_test.go`, pins transcribed from
the live wire, so the mechanism stays covered by `make test-fetch` with no network.

Routed as `reviews/COHOSTED-PEER-DISCOVERY-2026-08-30.md`: one ask (is the well-known profile
singular per origin, and if not, how is a co-hosted peer cold-started?) plus the finding that
the live registry publishes at `system/`, which is what makes §6a.3a's corrected MUST
implementable — the narrow prefix we argued against on 2026-08-21 would have broken this
deployment.

**Tree, measured today:** `make test-each` 10/10 green · `make lint` vet clean · `gofmt -l`
0 files · `make reachability` clean · `make consume-live` green.

**AE-5, re-measured by hand this session** (PR-D still has no target): corpus `8d2f55c8`, 362
vectors — **334 agree · 0 diverge · 28 INCOMPLETE · NOT LOCKED**, byte-identical to the
2026-08-25 reading. Itemised from the run: **12** `v325-corner` primitive vectors (PR-C),
**11** value-form error vectors (PR-E), **5** scope-fence vectors (the AE-6 question).

**Two rows corrected as stale:** `PeerLiveness` is **not** the remaining renderer gap — the
Avalonia peer panel consumes the liveness exports and `make reachability` is clean (closed by
`8383326`, never struck through). And `USAGE-PROTOTYPE-FILESYSTEM-SYNC.md` §10 still points
readers at the removed `canvas/` renderer.

## §0A — 2026-08-24, the 0.9.0 release preparation

**Working notes for this repo. Release-process coordination is not recorded here** — it belongs
to the seats that own it, and our side of it is in `docs/status/HANDOFF-2026-08-24-release-readiness.md`,
which does not publish. What follows is what changed in this tree and why.

### Tree state — the line this release is cut on

`make test-each` to completion, run twice today, the second time on the release tip, against
`entity-core-go` `13a42ea`:

**ALL 10 SUITES GREEN, exit 0** — `shellcmd` 290s · `sdk` 206s · `programs` 155s ·
`shellboot` 14s · `shell` 4s · `inspect`/`workbench`/`publish` ≤4s · `fetch`/`shellpanel` ≤2s.

**Re-run 2026-08-25 on the Axis-1 fix (§0A), same result** — `sdk` 209s · `shellcmd` 290s ·
`programs` 157s · `shellboot` 14s · rest ≤4s. `make lint` clean, `gofmt` clean. Green here now
means the defect is gone, not waived.
`make build` green (five binaries) · `make lint` clean · `gofmt` clean · `make reachability`
clean · Avalonia headless **74/74**.

**This is the first fully green `test-each` this tree has had**, and as of the Axis-1 fix below
it is green because the defect is **gone**, not waived. The intermediate state — waived on blast
radius with the cause recorded as unknown — lasted part of one day and is written up below,
because how it got there is the reusable part.

**The kernel moved 10 commits under us since the previous recorded sweep**, two of them
load-bearing here — `e5b3efd` (§6.9's "all resources" narrowed to own-namespace, the rule
`MintMirrorCapability` exists for) and `fb6d461` (published-root convergence under load). Same
single failure, same seed, so neither reached us. **The green line was re-measured against what
we actually ship on rather than inherited from yesterday.**

**The one failing suite is now green because the defect is fixed. Getting there took a wrong
count, a wrong retraction, and finally an instrument. All three are worth recording.**

`TestAxis1Equivalence_Differential` sweeps 300 generated graphs across the reference compute
engine and the experimental Axis-1 engine.

**First: the count was wrong.** The sweep ended at `t.Fatalf` on the first divergence, so for
three days this file, the CHANGELOG and every number routed outward said **one** failing case.
Removing the early exit shows **three** — 9, 28 and 79 — because 28 and 79 had never been
evaluated. That is **AP15**, one level below where we had already fixed it. `t.Errorf` now.

**Second: the diagnosis was right, and we retracted it on a probe that could not test it.**
The row said *"Axis-1 has not adopted `EXTENSION-COMPUTE` v3.26's contained-error semantics"* —
correct, and §0 below had even cited the six core-go commits that are the spec of the change.
It was retracted on two probes that indexed a 2-element array out of range, bare and
`Construct`-wrapped, and found both engines byte-identical. They are: **a bare index is a
CONSUMED position, where both engines were already right.** The divergence lives only at a
*closure-result* position inside a collection primitive. A probe that does not reproduce the
shape refutes nothing — and reporting it as a refutation cost more than the original error,
because it replaced a correct explanation with "cause unknown" in a published CHANGELOG.

**Third: the instrument settled it in one run.** Regenerate the same seed, dump each diverging
case's IR, then evaluate **every subnode on both engines, children first**, and print the
deepest node where they disagree. All three cases turned out to be the same shape:

```
length( map(arr, λe. e + index(<2-elt literal>, <out-of-range>)) )
```

Every subnode agreed; `map` was the first that did not. **Stage-1 CONTAINS each element's error
as a value and `length` answers 4; Axis-1 propagated and answered `index_out_of_range`.**

### The fix — a position model, not a patch

The real gap was that Axis-1 had **no notion of an error as a value**. §1.5 makes a
`compute/error` an ordinary value, so every result site has two representations to handle
(minted, and value-form) and the decision is keyed on the *position*, never on which
representation showed up. `entitysdk/axis1/contain.go` is the transcription of all three
position kinds, and each call site now names which one it is:

- **CONSUMED** — the result is READ (arith/compare/logic operand, `if` condition, cast value,
  field target, construct field, index and its array, any collection operand, a filter
  predicate). Both representations short-circuit. New chokepoint `evaluator.operand`, mirroring
  the reference's `evalOperand`.
- **CONTAINED** — the result is PLACED without being read (`map`'s output element, `fold`'s
  accumulator and `initial`). Both become a value in that slot — the §1.5 NaN model — except
  `budget_exhausted` / `cascade_limit`, whose counters are not restored on unwind. `depth`
  *is* restored, so `depth_exceeded` contains like anything else.
- **BOUNDARY** — a contained error element materializes **code-only**, so two implementations
  that word the same failure differently still produce the same array bytes.

Three of those were outright wrong before; the filter predicate was half-right (it propagated a
minted error but ran a value-form one through `truthy()`, whose default arm returns `true` — so
an element whose predicate *failed* was silently **kept**). That one was never reachable in the
sweep and is the kind of thing this class of bug hides.

**Gate:** `TestAxis1Equivalence_ContainedErrorPositions` — five vectors, one per position, each
asserting the *exact* outcome both engines must produce rather than merely that they agree (two
engines can agree on the wrong answer, and before the fix several of these agreed on a
propagated error), plus a structural check that a contained error materializes code-only with no
`message`/`at`/`expression`. Verified to fail on the pre-fix engine before being kept. The
generator reached this class by luck of the draw; the vectors do not depend on that.

**What to carry.** Two things, and the second is the expensive one:

1. **A wrong probe is worse than no probe.** "I measured it and the explanation is dead" is a
   much stronger claim than "nobody has measured this", and it is the one that gets copied
   forward. Before a refutation retires an explanation, show the probe *reproduces the failing
   shape* — ours did not go anywhere near a closure-result position.
2. **When two implementations disagree on generated input, the generated input is the evidence.**
   Dump the graph, walk it bottom-up, evaluate every subnode on both engines. It took one
   throwaway test file and one run to convert three days of "unknown" into a named position in
   a spec. Reach for it first, not after a round of hypotheses.

### `make build` now refuses early when the sibling kernel is missing

`build` / `test` / `test-each` / `lint` / `gui` / `gui-build` / `shell-build` (so also `run`,
`demo`, `shell`) depend on a new **`preflight`** target that names the missing
`../entity-core-go` and the `git clone` that fixes it, in one sentence. `make doctor` had that
check from the day it was written and **nothing called it**, so a clone without the sibling
produced forty lines of module-resolution spew and no cause.

The predicate is defined once as `SIBLING_PRESENT` and shared with `doctor`, so the reporting
path and the refusing path cannot drift. Verified both ways: silent exit 0 with the sibling,
and the full message plus exit 1 under `make preflight PARENT=/tmp/no-such-parent`.

**No suite in this repo can regress this** — a suite that runs at all is running in a tree where
the sibling resolved, which is also why it survived so long. Filed as **AP41**: *an instrument
nothing calls is indistinguishable from an instrument you do not have* — D23 (*a model with no
shipped surface is not shipped*) in a second domain.

**And its second instance arrived the same day.** `IN_CONTAINER` hard-coded
`-w /src/entity-systems/entity-workbench-go`, so the checkout had to be *named* that — true for
every developer, false for a git worktree. In a worktree named anything else: `make preflight` →
**OK**, `make build` → **`No rule to make target 'build-native'`**. podman *creates* the missing
workdir instead of refusing, so make lands in an empty tree and the error names our Makefile
while the defect is the placement. Fixed by deriving it — `REPO_DIR := $(notdir $(CURDIR))` —
and verified with a differently-named worktree: exit 0, five binaries. **The check passed
because the sibling really was there: it answered the question it was asked, and the question
was the wrong one.** That sharpening is in the charter.

### The published document surface was the weak half

None of this was in the code. `CANONICAL-DOCS.toml` is the declaration of what this project
publishes, and nobody had read it since the disciplines grew.

- **Blurbs advertised counts that had gone false** — *"D1–D23"*, *"AP1–AP27"*, a *"six-boundary
  map"*, *"P0–P6"*, every one of them behind the document it described. A blurb is not a comment:
  it is the prose a reader is shown **instead of** the document. Rewritten to describe rather
  than count, after the same blurb went stale again inside the same day's diff. *(This bullet
  used to restate the then-current numbers, which made it the very thing AP42 is about; a
  catalog entry added on 2026-08-25 falsified it. The counts live in the charter and nowhere
  else.)*
- **The GitHub URL named an organisation that does not exist.** Fixed to match every remote and
  the README.
- **`DOCTRINE-CRASH-FORENSICS.md` was undeclared** while public `AGENTS.md` instructs the reader
  to open it at the start of any crash investigation. Declared.
- **The nine root documents were undeclared** — `README`, `CHANGELOG`, `CONTRIBUTING`,
  `CODE_OF_CONDUCT`, `SECURITY`, `AGENTS`, `AGENTS-STANDARD`, `METHODOLOGY`, `CLAUDE`. Seven were
  already published, and the keep-list is a keep-list: undeclared means dropped. **Left alone the
  next release would have deleted `SECURITY.md` — the vulnerability-reporting address — from a
  public repo.** All declared.
- **`CHANGELOG.md` said nothing about 156 commits.** It carried *"Initial public research-preview
  release"* under `[Unreleased]`, written before `v0.8.0` was cut and never touched. Now written:
  Added / Changed / Fixed at a thematic altitude, plus **Known limitations** stating the sibling
  requirement, the one known differential failure, both flakes, and the unresolvable pin count.

Both halves are **AP42**: *a manifest is published prose and it goes false silently*, because a
`.toml` is read as configuration and skipped by review. The second half is the sharper one — **an
omission in a keep-list is an act of deletion against anything already published**, so the
manifest wants reviewing against what is currently public, not only against the tree.

### The rolling log moved to `docs/STATUS.md`

Operator ruling, matching the fleet. `docs/status/` is stripped from the published tree, so a
rolling canonical log left inside it is one edit away from being swept up by a rule that is
otherwise correct. **The path is now the declaration** — outside that directory is canonical,
inside it is working memory. The dated `STATUS-*` snapshots and `HANDOFF-*` stay behind and stay
unpublished; `.release-removals` records that the old path is a **move, not a withdrawal**.

**The cost, stated rather than discovered:** publishing this log took the repo's unreachable
short-SHA citations from **43 to 117**, 72 of them here — `dev` SHAs that by [ADR-0027] resolve
for no public reader. It gates nothing, but `CHANGELOG.md` had disclosed "43" and would have
shipped a false number. Corrected there with the split named. Backlog **PR-5**.

**And the rule that comes with the move:** this file is read by strangers now. Write it for the
next session — that is what makes it useful — but a verbatim quote in it is a published quote,
and anything you would not want read by someone outside belongs in `docs/status/`, one directory
away, which exists for exactly that. Two things went wrong here before that sank in: a frank
quote tripped the release profanity gate, and then the write-up of a resolved credential-shaped
finding **reproduced it** by pasting the literal. **Describe a finding; do not reproduce it.**

### The front door claimed two things that were not true

Both were true when written, and neither had a reason to be re-read:

- *"The published vanity module path is wired up … the final cutover is a one-line change per
  module."* **`go.entitychurch.org` has no DNS record** — confirmed here, `getent` rc=2, while
  the apex resolves. There is no module identity to fetch, so **`go get` of this SDK is not
  available**, and the sibling `replace` is not a shortcut we took but the only thing available.
  Rewritten, and the limitation is stated outright in README and CHANGELOG.
- *"The repo is not git-tagged yet."* `v0.8.0` is tagged and public.

**The pattern is worth more than the two fixes:** the sentences that rot on a front-door document
are the ones describing a *transitional* state — "not yet", "still local", "the final cutover
is" — because the transition completes somewhere else and nobody revisits the paragraph. The
README's durable claims (the sibling requirement, stated three times) were all correct.

### The number is 0.9.0

Stamped in `CHANGELOG.md`, `README.md` § Versioning, the shell's unstamped-build fallback and
the GUI's `.csproj`. **The reasoning, so no session re-derives it:** the implementations
(`entity-core-go` / `-py` / `-rust`) moved to semantic versioning at 0.9.0 and this repo sits
directly on them. The core protocol's own number is a different scheme and is not one to copy —
the shared `0.8` across the ecosystem was an accident of adoption, not a coupling. No `go.mod`
needed touching: those `require … v0.8.0` lines name *core-go's* version, and the shipped
binary's version comes from `git describe --tags`.

### A waived skip was quoting a measurement that had gone false

`TestStorage_SqliteIdentityBundle_RebootstrapGrowsBoundedly` is skipped under a conscious waiver
— a real linear leak in the kernel's identity ceremony, routed long ago, not ours to fix. Its
text said *"WAIVED for the 0.8.0 preview"*, and a release cut is exactly when that stops being
true. **So it was re-measured rather than re-worded:** Skip removed, test run against `13a42ea`.

| | when waived | 2026-08-24 |
|---|---|---|
| `ΔpathCount` per reload | 1 | **0** |
| `ΔentityCount` per reload | 4 | **2** |
| bootstrap → reload-4 | 347/338 → 351/354 | 374/365 → **374/373** |

**The path leak is gone and the entity leak is halved** — the kernel has fixed part of this, and
the numbers justifying our waiver had silently become false. Still linear, so the waiver stands
on substance; re-scoped to 0.9.0 with today's figures inline, and the assertion re-arms by
deleting one `t.Skip`. Routed upstream.

**The general form:** *a waiver cites evidence, and evidence expires.* A skip whose justification
names a release has a built-in expiry, and the honest act at the next cut is to re-run it, not to
bump the number in the string.

### One credential-shaped constant renamed

`entitysdk/rendezvous_test.go` bound the well-known xkcd passphrase to a constant named `secret`.
No secret by any reading, but the **shape** is a credential assignment and a scanner cannot tell
the difference. Renamed to `passphrase`; `secret` remains the word wherever it is load-bearing —
`RendezvousModeSecret`, the test name, the SIGNALING §3.2 reasoning. We declined the alternative
of having the detector taught to ignore canonical placeholder values: widening a credential rule
to suit one test is a worse trade than naming a variable accurately.

**A related one to carry:** `AGENTS.md` is written for an internal audience and is now published.
Internal infrastructure paths are what such a document is *made of*, and three of them had to be
rewritten to say what they mean without naming machine-local or internal locations — those belong
in the git-ignored `AGENTS.local.md` / `.agents/` ([ADR-0020]). **Declaring a document changes
what "internal" means about it; the manifest edit is not finished until the file has been re-read
as a stranger.**

## §0c NEW (2026-08-25) — the gate that would have named the bug on day one exists, is arch's, and we do not run it

Written immediately after §0A, because looking for coverage of the `filter` half of that defect
found something bigger than the defect.

**`TestAxis1Admission_*` skips unless `AXIS1_ADMISSION_CORPUS` is set, and no `make` target sets
it.** So it has skipped in every sweep since it was written, and a green `make test-sdk` has never
attested anything about AE-5. That is **AP41 in a second domain** — an opt-in check is not a gate —
and it is why §0's first correction paragraph could claim the admission was re-quotable without
anyone noticing nothing had run.

### What it says when you actually run it

Corpus `8d2f55c8…`, profile `inproc`, **362 vectors**, generated from core-go `13a42ea`;
reference emission from core-go in-process; Axis-1 emission at `6ab42c6`:

| | |
|---|---|
| agree | **334** |
| diverge | **0** |
| INCOMPLETE (deopted to Stage-1) | **28** |
| verdict | **NOT LOCKED** |

Two readings, and both matter:

- **The good half is genuinely good.** Every vector Axis-1 *answers* is byte-identical to the
  reference — 334 of them, including the whole v3.26 contained-error family. That is far stronger
  evidence for the §0A fix than the five vectors we hand-wrote, and it is independent of us.
- **The bad half is that AE-5 is not green and has not been for some time.** §11's **AE-6** is
  explicit: *"the alternate engine MUST run every vector; no per-vector fallback … deopt during an
  admission run voids the evidence for that vector."* 28 deopts means 28 voided vectors. **Stop
  quoting the 2026-07-23 admission as current** — it lapsed when the v3.24/v3.25 primitives landed
  and nothing told us, because nothing ran.

### The corpus would have named this bug on day one

Re-run against the **pre-fix** engine, it produces **five two-way divergences**, and the vector IDs
are the diagnosis:

```
cv8a-map-contains-minted-error
cv8c-filter-predicate-error-shortcircuit
cv9a-map-depth-exceeded-contains
cv9c-map-valueform-budget-exhausted-shortcircuits
sweep/0281
```

Our home-grown 300-case fuzz found three anonymous cases and cost three days plus a wrong
retraction to explain. Arch's corpus **names the class in the vector ID**. It also carries eight
`worked/value-error/*` vectors — one per consumed position — which is the exact table §0A's fix
had to derive by reading core-go's source.

### And 12 of those vectors cannot currently reach the code they test

Of the 28 deopts, **12 are value-form-error vectors that fail one node too early**: Axis-1's
decoder sends a `compute/error` **leaf** to the Stage-1 fallback (`decode.go`'s `default` arm —
"value types … go to Stage-1"), so the vector never reaches the CONSUMED/CONTAINED logic it was
written to test. Measured: adding a single `case types.TypeComputeError → litNode{value: ent}`
takes deopts **28 → 16** and every recovered vector cross-blesses byte-identical. **That change is
not in `6ab42c6`** — it is measured, not landed, because it wants its own diff and its own review.

The remaining 16 are honest gaps, not a decode artifact: 11 are the v3.24/v3.25 primitives
(`assoc` / `concat` / `group-by` / `range`) that Axis-1 has never implemented — that is backlog
**PR-C**, whose real size this measures for the first time — and 5 are dispatch-mode `apply`,
which is deopt **by design** (`doc.go`'s scope fence). Those 5 need arch's ruling, not code: AE-6
admits no fallback, and Axis-1's declared scope excludes dispatch. Either the corpus profile grows
a pure-only subset or the scope fence moves.

### Rows this opens

- **PR-D — wire the admission corpus into a `make` target.** The one-line runner is in `AGENTS.md`
  now; a target that generates, emits, and cross-blesses is the actual fix. Deliberately not
  `test-native`: it needs a buildable sibling, and a sweep that can go red for a neighbour's
  reasons teaches people to ignore the sweep (same rule as `crossimpl-go`).
- **PR-E — decode `compute/error` as a value leaf.** Measured above: 12 vectors, 0 divergences.
- **PR-C is now sized** — 11 corpus vectors, named.
- **Ask arch:** how does an engine with a declared scope fence satisfy AE-6? (§0c, routed in
  `reviews/AXIS1-ADMISSION-LAPSED-2026-08-25.md`.)

### Operator ruling, 2026-08-25 — the engine's home is the compute extension

Recorded here so it is not relitigated. **`entitysdk/axis1` does not belong in an SDK**: an SDK is
an interface layer, and the only thing that implements compute is the compute extension. An
alternate engine is not a research toy — it is **what makes compute practical**, because without
the collapse to a condensed handler you re-hash every intermediate on every tick. So it belongs
with the extension it makes usable, **under the same vectors and the same standards, exercised on
both engines whenever compute changes**. Someone re-implementing for their own deployment reasons
is fine and expected — under those same vectors.

**The objection this repo had been carrying is withdrawn.** `doc.go`'s *"sharing code with the
reference would make the equivalence oracle circular"* defends **code** independence — which is
precisely what produced this drift — while the independence that actually catches errors is
**vector** independence, i.e. arch's corpus, which does not care where the engine lives. Nothing
in the proposal asks the two engines to share code; it asks one corpus to run over both, in the
repo where the semantics change.

Structural evidence, measured: `axis1` is **3,028 lines with zero dependencies on this repo**
(`go list -deps ./axis1` is core-go only), implements core-go's `handler.Handler`, and is consumed
by `programs/` through a **handler-path string** rather than an import. It is not integrated here;
it is parked here, because that is where the prototype was typed. Routed as
`reviews/PROPOSAL-AXIS1-HOME-IS-THE-COMPUTE-EXTENSION-2026-08-25.md`.

*And the honest frame: this was a prototype built fast to prove entity-compute could carry a real
interactive workload. It did. The placement was a deliberate corner cut, and both halves of that
trade are now visible — the capability and the maintenance arrangement nobody would design.*

## §0aa NEW (2026-08-23) — the re-sync against core-go's compute work found two live defects, neither of them in compute

**Why the session ran.** `entity-core-go` moved 25 commits after the close-out — most of it
compute (`COMPUTE v3.26`, the §3.5 Corner-1 rulings, the eval-limit carve-out, the corpus
re-freeze 359→362, and `0e34e3e`'s §5.2 depth-budget fix). Every `go.mod` here resolves the
kernel through a `replace` to the sibling working tree, so "core-go moved" and "our build
inputs moved" are the same event. The ask was narrow: **confirm the tree still builds and the
compute surface still works — not to extend it.**

**The compute answer is clean.** `make build` green against core-go `3b4b1e9`; `make test-each`
9 PASS / 1 FAIL with the *same single* known failure (`TestAxis1Equivalence_Differential`
case 9), so none of core-go's six compute changes introduced a new divergence. `programs`
(158s) passes, which is the generic host over `system/compute` at their HEAD.

**But driving the shipped compute verb by hand found two defects, and they were not compute's.**
D19 earned its keep here — the suites were green and the source read fine; only running the
operation surfaced these.

**1 — the query index did not survive a restart, so three verbs went blind.** `find`, `grep`
and `compute aggregate` all route through `system/query`. core-go ships only in-memory query
indexes (`MemoryTypeIndex` / `MemoryReverseHashIndex` / `MemoryPathLinkIndex`), populated
solely by the `"query"` sync hook — i.e. only by writes made *during this process*. With
`-storage sqlite` the tree persists and the index does not:

```
session 1:  put …/files/a … ;  find files a  →  1 match
session 2:  (same DB, new process)  find files a  →  (no paths under files match "a")
            ls …/files                          →  a          ← the entity is right there
```

That asymmetry is why it reads as *"the entity is gone"* rather than *"the index is empty"*,
and why no single-process test in `entitysdk` could see it. **The kernel already had the
answer and we never called it** — `query.IndexMaintainer.Rebuild(li)`, whose own doc comment
says *"use for recovery or startup with persisted stores."* This is **D20 exactly**: the fix
was one call, and the D20 grep is what found it rather than a plan to build an index. Landed in
`assembleAppPeer`, unconditional (an empty tree makes the scan free, and a caller supplying a
persistent `LocationIndex` via `RawOptions` gets the guarantee without knowing to ask).
Gate: `entitysdk/query_index_restart_test.go` — **mutation-checked**, reports 0 matches with
the `Rebuild` call disabled.

**No packet is owed to core-go for this, and the check that established that is the point.**
The obvious next move was to route it — "your query indexes go blind on a persistent store" is
a serious claim about a reference implementation days before a release. It is also **false**:
`cmd/entity-peer/main.go` already calls `queryMaintainer.Rebuild(p.LocationIndex())`,
unconditionally, with the same rationale we arrived at and a citation to
`DESIGN-SQLITE-PERSISTENCE.md §4.3` we had not read. **Our wiring had diverged from the
reference wiring at a step nobody diffed.** Reading their call site cost a minute and stopped a
wrong packet (D19: route the measurement, not the argument) — the standing lesson being that
when we re-implement a kernel assembly the kernel also performs, an omission is **ours by
default** until their call site says otherwise.

**2 — `compute aggregate` could not read a size written by the shell's own `put`.** `put`
decodes its payload with `json.Unmarshal` into an `interface{}`, so every JSON number becomes a
`float64`, and CBOR core-deterministic encoding does not fold an integral float back to an
integer. `extractNumericSize` accepted only the integer kinds. The result, on the most obvious
gesture in the verb's own usage line:

```
put …/files/a app/file '{"name":"a","size":100}'   ×3
compute aggregate files
  → (no entities with a numeric .size field under files; 3 entities scanned, 3 skipped)
```

The verb whose stated purpose is *"demonstrate compute usefully aggregates over real workbench
entities"* could not aggregate over one. **Its own unit coverage was green throughout** because
those cases build Go ints directly — the bug lives exactly at the seam between two verbs, which
is the part neither verb's tests own. Non-integral sizes are now **refused, not truncated**
(AP33; also what the fold requires, its accumulator being a `uint64` literal).
Gate: `shellcmd/cmd_compute_size_test.go`, which encodes through `put`'s own path and carries a
premise-guard so a future encoder change cannot make it vacuous.

**Re-measured end to end through the shipped binary, across the process boundary:**

```
session 2 (fresh process):  find files a           → 1 match(es) of 3 entities under files
                            compute aggregate files → count = 3 · total_bytes = 999
```

`999 = 100+250+649` — a real `LowerFold`/`add` dispatched to core-go's `system/compute` at HEAD.

**Two doc facts corrected while re-syncing** (both were true when written, both had expired):

- **`types.BuildContentURL` is fixed upstream.** `AGENTS.md` said it emits digest-only hex and
  told us not to delegate to it. core-go `7f39eb3` landed the full-wire-form; it now hexes
  `h.Bytes()` (33-byte, format byte included). `fetch/crossimpl_test.go`'s tripwire was written
  to notice exactly this and now logs the agreement. The delegation itself still owes a
  round-trip measurement before `fetch.contentURL` changes — **noticing is not adopting.**
- **Axis-1's constraint reader is now a stale mirror** — see §0b below.

**Nothing was routed to us.** D21 sweep of `../entity-core-go/docs/status/` by subject: the
newest documents naming this repo are the 08-20/08-21 handoffs already read. `0e34e3e`'s ask
(*"the cross-impl re-drive of the 362 corpus"*) names **rust/py, not us**, in its own text.
## §0b NEW (2026-08-23) — Axis-1's constraint reader drifted, and it is recorded rather than fixed

core-go `0e34e3e` replaced `ext/compute/handler.go::extractComputeConstraints`, which
`entitysdk/axis1/handler.go` transcribes verbatim by design. Per `EXTENSION-COMPUTE` §5.2 the
eval limits come from the **matching grant's** `constraints["system/compute"]`; a capability
token carries no top-level `constraints` field at all, so the old reader — ours, still — means
**no compute constraint has ever reached the Axis-1 evaluator.** Their replacements are
`computeConstraintsOfGrant` / `computeConstraintsOfToken`.

**Deliberately not fixed this session**, on two grounds: Axis-1 is registered by tests only (no
binary, bridge or panel reaches it — `make reachability` agrees), so it changes nothing a user
runs; and adopting it is engine-semantics work that belongs with the v3.26 contained-error
adoption in §0, not ahead of a release. **What did land is the honesty**: the transcription's
doc comment now names the drift and cites `0e34e3e`, because *the transcription is the
contract* and a comment claiming a lapsed fidelity is worse than no comment.

**Carry both rows together into the post-release backlog (PR-B + PR-3)** — they are one piece of work
(Axis-1 catches up to COMPUTE v3.26 *and* to §5.2), not two.

> **2026-08-25:** the v3.26 half (PR-B) is done — §0A. This half is not, and it turned out to be
> genuinely separable: the fix was a position model over error *values*, and it does not go near
> the constraint reader. The pairing was a good bet on shared context, not a real dependency.

## §0a NEW (2026-08-22) — Regen was sliding one fixed pattern, and the operator's phrasing was the measurement

**Reported:** *"Regen should just basically pick a random seed… right now it just seems to
iterate this weird ladder. If I keep hitting Regen it seems to just move it one or two over."*
That is not an impression. It is a description of a **translation**, and it was exactly right.

**The defect (AP38).** `programs/life_edit.go`'s regen soup hashed `(gen, i)` as
`LCG(gen·C + i)` and read bits 16..18. Under a power-of-two modulus, bit *k* of an LCG step
depends only on bits 0..*k* of its input — so those three bits depend on nothing but
`(gen·C + i) mod 2^19`, and **changing the generation counter is arithmetically
indistinguishable from changing the cell index by a constant.** The 256-cell board was a
window into one fixed pattern; Regen only slid the window. Measured before the fix, best
agreement under a cyclic shift:

| generation gap | agreement | shift |
|---|---|---|
| 1 | 0.980 | 9 cells |
| 2 | 0.965 | 18 cells |
| 6 | 0.961 | 2 cells |
| 60 | **1.000** | 2 cells |

The board is 16 wide, so a 9-cell shift is half a row — *"one or two over"* is the arithmetic
read off the screen.

**Why every test was green.** `TestLifeEdit_RegenReplacesBoard` had an explicit anti-vacuity
clause — `if lifeCellsEqual(before, after) { t.Fatal("regen did nothing") }`. **A translation
is never equal**, so the clause passes on every one of these boards. The test asserted
*different* where the property that mattered was *independent*. The second tell was free and
unread: a translation preserves the live-cell count, so population across regens had σ = 0.76
where an independent draw at 3/8 density gives σ = 7.75 — every board it ever produced had
exactly 96 cells alive.

**The fix.** One nonlinear round: square the mixed value (so the counter's contribution depends
on the index it is mixed with — no shift can reproduce that), then fold the square's high bits
down before the LCG step, because squaring mod 2^k leaves the low bits weak. After: agreement
0.578–0.734 under any cyclic shift, cell-for-cell agreement 0.530 (chance for two independent
boards at this density is 0.53125), population mean 96.5 with σ 7.33 against a theoretical 7.75.

**And no, compute has no randomness — that is correct and load-bearing.** Reproducible state
hashes are the whole point of the programs track; a mirror that re-derives a program's state has
to get the same bytes. The entropy is the **tick counter at the moment of the press**, which is
unpredictable to a human hand and exactly replayable to a machine. The bug was never determinism;
it was a hash too weak to look like one.

**Gate:** `TestLifeEdit_RegenIsNotATranslation` — two regens through the **running host**, an
oracle sweep across generation gaps 1…1000, and the population-σ check, with `lifeBestCyclicMatch`
as the instrument. Thresholds calibrated over 2500 board pairs (defective 0.953–0.992, fixed
0.578–0.734, cut at 0.85). **The gate was run against the old hash and shown to fire on all seven
gaps before it was committed** — a regression test never shown red is decoration.

**Ratchet:** AP38 in the charter (with its enforcement paragraph), and a `programs/` entry in
`AGENTS.md` — *compute has no randomness, so a program's "randomizer" is a hash, and a hash
linear in its varying input is a translation.*

**Reachability note.** The GUI embeds the Go program authoring, so an operator sees this only
after `make gui` (image rebuild) — `make gui-run` alone will keep running the old soup.

## §0b Close-out review (2026-08-22) — the D21 sweep is clean, and one decision is left

**Run at close-out, so "cap it unless something major comes through" is a measurement and not
an assumption.**

- **Arch: nothing new addressed to us.** The newest packet naming this repo is still
  `ROUTING-2026-08-21-k` (read and answered last session; pin replied in
  `reviews/COMMIT-PIN-REGISTRY-1.21-PROVENANCE-2026-08-21.md`). Arch has three commits since —
  a `METHODOLOGY.md` overlay re-sync and two on their own publication/crawl-path track — and
  **the diff of `COHORT-OPEN-ITEMS.md` and `WORKSTREAMS.md` since then adds no line naming
  workbench-go.** Checked by subject and by diff, not by filename (AP28).
- **`METHODOLOGY.md` overlay is in sync** — byte-identical to arch's, and tracked/committed here.
- **`entity-browser-rust`: nothing routed to us.** They have moved to connectivity, rendezvous
  and acquisition; no commit since 2026-08-21 names this repo. Our
  `reviews/GENERIC-HOST-SUBTICK-INPUT-2026-08-21.md` is committed and pushed (`7729cb5`), which
  is what delivery means; **no reply is owed to us** — it was routed as a design result.
- **Nothing is blocked on us.** Every row in "Waiting on" below carries an explicit
  *blocked on us: nothing*, and the one row that used to read "blocked on arch" while never
  having been sent (pointer input) is delivered and sitting on arch's board as W-1.

**Axis-1's drift does not touch the shipped surface.** `entitysdk/axis1` is imported by nothing
outside its own package and tests — no binary, no bridge, no panel. So §0 was a **stale
conformance claim** (AE-5, `EXTENSION-COMPUTE` §11, LOCKED 2026-07-23), not a defect in anything
a user runs. Per ADR-0012 that distinction is the whole point: **the release must not restate the
AE-5 admission as current** until the engine adopts v3.26 contained-error semantics.

***The v3.26 semantics are RESOLVED 2026-08-25 (§0A). The AE-5 admission is NOT, and the first
version of this paragraph said it was.***

**Correction.** This paragraph originally read *"the AE-5 corpus still runs byte-exact with the
new semantics — `make test-sdk` green under `-race`, 330 vectors — so the admission is
re-quotable."* Every clause of that is wrong, and it was written without running anything:
`TestAxis1Admission_*` is **gated on `AXIS1_ADMISSION_CORPUS` and skips by default**, so
`make test-sdk` green attests nothing about AE-5; the corpus is **362** vectors now, not 330; and
when actually run, **the admission does not lock.** See §0c.

The same reflex as AP43, one day later and pointed at a different fact: a green suite was read as
covering a gate that suite does not run. **A skip counts as a failure** (AGENTS-STANDARD), and
this one had been skipping in every sweep since the harness was written.

**~~The one decision left: `dev` is ahead of `master`.~~ RETRACTED 2026-08-23 — there was never
a decision here, and carrying one was the error.** `master` is the **public canonical mirror**
and is **reset to public after each release**; promotion `dev → master` is the *release act*,
performed by maintainers / the release team through the central runbook, per ADR-0015 and its
2026-06-30 amendment. **`dev` ahead of `master` is the normal steady state** — it is simply
unreleased work. Framing it as *"an operator call"* pending
in our status file put a release-team act on our board and invited a future session to act on
it. Nothing here is owed, and the row is closed rather than answered. The rule now lives in
`AGENTS.md` under **Boundaries — do NOT modify**, which is where it can actually stop someone.

## §0 — CLOSED 2026-08-25. Axis-1 had drifted from COMPUTE v3.26, and our own differential gate caught it

> **Closed by the fix in §0A above.** Kept verbatim because **this section was right**: it named
> the cause and cited the six core-go commits that are the spec of the change, on 2026-08-23. It
> was overturned on 2026-08-24 by a probe that could not reach the failing position, and the
> retraction — not the original — is what had to be undone. If you are reading this because a
> written-down explanation is being challenged, the question to ask is whether the challenge
> *reproduces the shape*, not whether it ran cleanly.

`TestAxis1Equivalence_Differential` (300-case fuzz, fixed seed 20260716) diverges at case 9:

```
stage1: entity:ecf-sha256:c71e6433776fe58280ee9cfa69bebf6e07bce058546dd7eb75a3f2e5df013f95
axis1:  error(index_out_of_range: index 4 out of range for array of length 2)
```

**Stage-1 CONTAINS the error and returns a value; Axis-1 short-circuits.** That is the exact
direction of six `entity-core-go` compute commits dated **2026-08-21** — `7f39eb3` (COMPUTE
v3.26 contained-error), `ded9ea0` / `4519554` (a consumed operand short-circuits an error, not
`type_mismatch`), `eea0a6e` / `9ad0110` / `d5e318d` (§3.5 Corner 1: filter/fold/map closure
results contain and recover). Our last green `sdk` sweep was **2026-08-20**, before all six.

**Why this matters more than a red suite.** Axis-1 is not a research toy — it is a
**conformance-admitted alternate engine** (AE-5 / `EXTENSION-COMPUTE` §11, 330 vectors, LOCKED
2026-07-23), and it is **workbench-owned**, so this is ours to fix and nobody else's. The
admission is stale until it adopts the new semantics.

> *2026-08-25: the "conformance-admitted" clause above is no longer true and the sentence after it
> understated the problem — the admission did not go stale pending v3.26, it had already **lapsed**
> when the v3.24/v3.25 primitives landed. **WITHDRAWN**, see §0c and §1a. The "not a research toy"
> half stands and got stronger: it is what makes compute practical, which is why the operator ruled
> its home is the compute extension rather than an SDK.*

**Not started, deliberately.** It is a real piece of work (error containment touches every
collection primitive and the fold accumulator) and it was not this session's ask. Two things
that will save the next session time:
- The five core-go commits above are the spec of the change, and each names its arch ruling.
  *(2026-08-25: this was the right pointer. The fix was transcribed from exactly these, and the
  estimate above — "touches every collection primitive and the fold accumulator" — was accurate,
  though it under-counted: the consumed positions outside the collection builtins needed the
  matching short-circuit, and the boundary needed the code-only reduction.)*
- **Do not read arch's "compute stays sequenced / deferred for you" as covering this.** That
  deferral is about the compute-floor research track (T5). This is an admitted engine drifting
  from a landed spec revision, surfaced by our own gate — a different thing that happens to
  share the word.

**Previously (2026-08-20, after the consume leg — §12).** `make test-each` was run **twice** this
session and both runs are reported, because the difference between them is the point:

| run | result |
|---|---|
| before the last edits | **all ten PASS** — sdk 201s · shellcmd 288s · programs 138s · rest ≤13s |
| **final tree** | **nine PASS, `shellcmd` FAIL** — sdk 194s · shellcmd 307s · programs 142s · rest ≤13s |

**The one failure is `TestE2E_Bidirectional_BurstWrites_NoFS`, and it is the known one** — the
terminal last-burst-write loss documented below and routed as
`reviews/CORE-GO-LAST-BURST-WRITE-LOSS-2026-08-20.md`. Our own classifier named it at the moment of
failure: **VERDICT (B1) — NEVER CAPTURED**, `archives/notes/a-4.md` still held by the writer and in
no version. It is the documented load-dependent failure, it is the only failing test in the suite,
and **nothing in this session's diff is in its path** (the reproducer builds its peers straight
through `entitysdk` with no filesystem, no localfiles and no workbench/fetch code). Per AGENTS.md a
passing targeted re-run would **not** be evidence it was spurious, so none is quoted here — one of
two full sweeps hit it, which is the same shape the bug has had since it was diagnosed.

`make lint` clean · `gofmt -l` empty · `make reachability` clean · **Avalonia 63/63 headless**
(three new `PublisherVerifyPanelTests`) · **`make crossimpl-go` green** (live, cross-impl).

**Previous merge gate, for the record:** green as of `2efeb9d` — all ten suites to completion, zero
failures, `sdk` 196s · `shellcmd` 287s · `programs` 140s. That was the sweep that preceded
`dev` → `master`.

**Avalonia: 60/60 headless** (`make -C avalonia test`, 2026-08-20 — three new liveness tests),
plus `make smoke-xvfb-handlers` green under real X11 + software Skia — 21 handlers walked,
exit 0 — and the new `make smoke-xvfb-connections` (§8).

**The burst flake is diagnosed, and it is a terminal write loss — not a flake and not saturation.**
`TestE2E_Bidirectional_BurstWrites_NoFS`, reproduced 5× under load (0-in-10 idle). The
`heads_equal=true` signature the last session flagged as a lead was one, and it pointed here:

- The heads AGREE, and the agreed head's trie commits to **9 of 10** paths. The lost path is in
  **no version at all**.
- **The peer that WROTE it still holds it; the counterpart never gets it.** That discriminator
  rules out a merge wipe — a wipe takes the writer's copy too. The write was **never captured**.
- Always the **last write** of one peer's burst, symmetric between peers, and **terminal**: the
  head is settled, nothing further is emitted, and no later merge can recover a write no version
  ever named.

**core-go's own source names this failure and says it was fixed.** `ext/revision/auto_version.go`'s
CAS-retry comment calls it *"the symmetric last-burst-write loss pattern… diagnosed by the
workbench in the F10 part-3 results."* It still reproduces. The structural hole is that **every
give-up path in `fire()` recovers via "the next sync-hook event will fire() again", and the last
write of a burst has no next event** — which is exactly why it is always the last write and never
an interior one, and why it is the write under maximum contention.

Routed: `docs/architecture/reviews/CORE-GO-LAST-BURST-WRITE-LOSS-2026-08-20.md`. **Not fixed here**
— the reproducer builds its peers straight through `entitysdk` with no filesystem, no localfiles
and no workbench model layer, so the workbench is not in the failing path.

**What we could not determine, recorded rather than guessed:** which give-up path fires.
`AutoVersioner.debugf` would say in one line, but its output never appeared in ours — the
`PeerConfig.DebugLog` we set does not reach the revision AutoVersioner's logger. Asked for
alongside the main finding. The §3 argument holds whichever path fires, and it is routed as that
rather than as a measurement (D19/AP10).

**The harness now classifies at the moment of failure** instead of leaving it to a reader.
`classifyConvergedHeadFailure` walks the agreed head's trie and prints one of four verdicts —
(A) apply/projection gap, (B1) never captured, (B2) captured then wiped, or inconclusive — so the
next occurrence arrives as evidence. The old message printed `heads_equal=%v` on one line, which
read as a paradox and got the failure shelved under a label that could not explain it.
**`applyBindings` was our prime suspect from source reading and the measurement ruled it out** for
this failure; the B1/B2 split is what keeps that honest.

**Latest arch packets read: `ROUTING-2026-08-21-c` / `-f` / `-i` / `-k` (all addressed to us) plus
`-n`, at arch `b61bafb`** — read 2026-08-21-c. Four packets naming us had landed since the previous
marker and none had been opened (D21). What they move:

| packet | disposition |
|---|---|
| **`-k`** §1/§3 — *arch folded `EXTENSION-REGISTRY` 1.21 on a document that exists in no commit*: our whole registry session was untracked, so the provenance chain for a normative spec revision terminated in a working tree | **Closed, first action of this session.** Three commits, `dev` at **`a6b5e9d`**, pushed. The packet arch folded on is `reviews/REGISTRY-BINDING-TRANSPORTS-DIVERGENCE-2026-08-21.md` at `a6b5e9d`; the code that produced it is `64dcc81`. Replied with the pin: `reviews/COMMIT-PIN-REGISTRY-1.21-PROVENANCE-2026-08-21.md`. Our own fixture README had stated this exact rule about **someone else's** gitignored artifact hours before we left our own packet uncommitted — folded into AGENTS.md as *a packet that is not committed has not been routed*. |
| **`-f`** / **`-c`** — `transports` is **RULED our way and folded**, REGISTRY 1.20 → **1.21**: `[<system/hash, BARE>]` at all five declaration sites, plus D8a (a publishing registry MUST serve what its bindings reference) and D8b (**our finding #3**, the unimplementable §6a.3a prefix, corrected) | **Closed in our favour, three of four findings folded.** No code change owed yet; the consequences are the re-cut below. Arch records our posture — liberal decode, forms kept distinguishable, *"did not make our SDK succeed where the reference implementation fails"* — as the reference one. |
| **`-i`** §2 — **re-cut `fetch/testdata/crossimpl-rust-federation/`** from `entity-browser-rust`'s now-**committed** `tests/fixtures/registry-federation/` at `54f31a7` | **OPEN, unblocked, and the next item on this track.** `TestKernelCannotDecodeARustBinding` should flip (honour its own in-file guard rather than deleting it), the inline branch of `fetch.NameBinding`'s decoder goes, `TransportRef.Kind` **stays** (a decoder that can name the rejected shape is the better diagnostic), and the README's provenance caveat is discharged. Also: their `.list` files are now `system/tree/listing` ECF entities with a **canonical-CBOR** `entries` map — length-first then lexicographic — so **sort in the reader**. |
| **`-i`** §2.1 | Arch had our step and core-go's in series; they are parallel. **Nothing is waiting on us.** |
| **`-k`** §5 / **`-i`** §3 — compute | T5 stays **DEFERRED**; `EXTENSION-COMPUTE`'s fold to v3.27 is explicitly **not addressed to us**. *(Note §0 above: that deferral does **not** cover Axis-1's drift from v3.26's contained-error semantics, which is a different thing sharing a word.)* |

**Previously: `ROUTING-2026-08-21-b` (addressed to us), at arch `bded94c`** — read
mid-session on 2026-08-21, *before* the build it concerns had landed, and it moved three things:

| what | disposition |
|---|---|
| §1 — the **content-URL shape** we filed is **RULED our way**, `EXTENSION-SUBSTITUTE` 1.2 → 1.3: `{hash}` is `hex(H.Bytes())`, format byte included, fleet-wide | **Closed in our favour.** `TestContentURLUsesWireHexNotDigestHex` stops being a local pin and becomes the spec's rule. No code change here; `fetch` already did this. |
| §3 — **build `EnumerateNames`**: §6a.3a is *specified, one producer, **zero consumers***, and arch records that browse was cut from v1 on a **false premise** (*"the shipping application has the browse surface"*) — corrected to *"`entity-browser-rust`'s browse surface has never walked a registry"* | **Built this session.** `fetch.Registry.Enumerate`. Arch's only ask — *"say which produced a row **in the artifact**, not only in the code"* — is satisfied: `RegistryRow.Committed`/`.Listed` + `BrowseOutput.NamesAuthority`/`.NamesNote`, and every surface prints it. Arch is explicit this does **not** reopen v1: a new consumer is a build, not a finding. |
| §5 — **R-9 (`hints` round-trip) goes LIVE the moment `PinRegistry` writes chain entries**, which it now does | **Answered both halves.** Our `InstallResolverConfig` takes the whole decoded struct and re-encodes it — no field-by-field rebuild — and `Hints` is `map[string]cbor.RawMessage`, so an unknown key survives untouched; `addPeerIssuedChainEntry` **appends** rather than reconstructing. And the *authoring* side, which genuinely was absent, now exists: `PinnedRegistry.MaxTTLMillis` / `.NegTTLMillis`. |

§5 also confirms **W-1 (the pointer proposal) is still arch's and still sequenced** — nothing
blocked on us.

**Previously: `ROUTING-2026-08-20-l` (addressed to us), at arch `f3e81e2`** — plus
`-k` / `-m` / `-n` / `-o` and the `WORKSTREAMS` rows naming us in the same pass (D21: a `cc` is a
packet). **`-l` closes the AP28 loop from the other end** and nothing in it is blocked on us; see
§12. **Before that: `ROUTING-2026-08-20-e` + `-h` + `-i` + `STATUS-2026-08-20-c`,
at arch `8dd5689`** (§11). `-e` is addressed to us and carries the compute disposition; `-h`/`-i`
are core-go's and name us in their fold order (D21 — a `cc` is a packet), and neither asks anything.
**Before that: the whole earlier 2026-08-20 set** — `STATUS-2026-08-20-b`,
`HANDOFF-2026-08-20` (arch), `ROUTING-2026-08-20-a/-b/-c/-d`, plus `ROUTING-2026-08-19-k` and
`STATUS-2026-08-19`, all at arch `a5dfff3`. Eight documents naming this repo had landed since the
previous marker and none had been opened — see §9. **Previously: `ROUTING-2026-08-19-j`** (arch `05faaa5`, carrying **REGISTRY 1.18**);
`-19-i` at arch `3dd5800` (**REGISTRY 1.17**) read in the same pass. browser-rust's
`ROUTING-2026-08-19-d` read at their `fbc2c5c`. D21 — this line is the subtraction that tells the
next session what it has not opened. Read *every* document naming this repo, `cc` included:
`grep -ril 'workbench-go' ../entity-system-architecture/docs/status/`.

**Inbound, all answered:** `-19-b` §2 (validator widening → §6c), `-19-c` (the registry board is
closed; our only row was the widening), browser-rust's `-19-d` (consume-us ask → §6d, plus their
F6 correction folded in as AP22), arch's `-19-d` (→ §6e), and **`-19-i` + `-19-j` (→ §6f — both
addressed to core-go, both moving a MUST we had shipped the day before; one live defect)**.

## Where it is

The Go **reference application** built on the Entity Core Protocol — an opinionated,
worked example of an entity-native app, **not** a conformance implementation and not a
mandate. It is a leaf in the stack (depends on the `entity-core-go` kernel — `core` + `ext`
— via local `replace`; nothing depends on it). It ships the in-tree `entitysdk/` (the de
facto reference Go SDK: typed wrappers, storage, identity bundles, revision/continuation/
discovery helpers), **`entity-shell`** (the primary CLI and leading edge of feature work),
an **Avalonia/.NET desktop GUI** driven through a Go c-shared bridge (podman-only build),
a frozen **`console`** (tview) TUI kept as a renderer-neutrality enforcer, the
**`programs/`** entity-native programs track (generic compute host + descriptors), and a
small CDN corridor (`entity-publish` / `entity-vcs` / `entity-fetch`, plus
`entity-seed-site` / `entity-serve-cors`). Maturity: **v0.8.0 research preview**.

`master` carries v0.8.0. **`dev` is well ahead of it** and is where the compute work lives;
it is deliberately unmerged (see the guardrail below).

## Where we left off

**Latest handoff:** `docs/status/HANDOFF-2026-08-25-contained-errors-and-a-gate-we-were-not-running.md`
— **start there** for the current tip: the Axis-1 fix, the lapsed AE-5 admission, the recommended
order (PR-D first), and the traps that will bite a fresh session. Then
`docs/status/HANDOFF-2026-08-20-bearings-and-audit.md` for the wider bearings. It is
the audit, not a session log: tree state, where every arc stands, what is owed each way, the one
remaining renderer gap, and a recommended order. `HANDOFF-2026-08-20-piece-four-and-two-real-bugs.md`
is that session's log; `HANDOFF-2026-08-19-reachability-front-door-and-the-rendezvous-hold.md` the
one before. **Piece 4 — the rendezvous DISCOVERY backend — is BUILT**
(`021e5c2`), so the four-piece connectivity list is complete; §3 piece 4 has the result and what
it routed. Also this session: a live conformance defect fixed under D21 (§6f — our boot refused
to start on a config REGISTRY 1.17 says it MUST run under).

Three threads: the share arc (open, moving), the compute floor (**deferred by operator decision,
resumes after the release** — §11), and a closed stabilization pass.

### 1. The compute floor (the primary arc — Doom-class realtime) — WORKBENCH SIDE IS DRY

Six sessions (2026-07-15 → 07-19) characterized parallel compute on the entity model
end-to-end. **Everything on the workbench side is built, measured, and green; every
remaining lever is gated on arch or core-go.**

The one doc to read is
**`docs/architecture/reviews/COMPUTE-SHARDING-INTO-HOST-2026-07-18.md`** — §10 is the ledger
of all five routed levers and who owns each; §11 is the latest result. Not duplicated here.

Headlines, so this doc stands alone:
- **Host-managed static-k sharding** is a real mounted-program mode, not a test rig. The
  gate proves *parallel == serial == unsharded* state hashes, generation for generation.
- **64×64 reach** through the generic host against an independent Go Life oracle
  (~850 ms/tick on Stage-1 — correct, not realtime).
- **The ~1.8× parallel ceiling was diagnosed, not accepted.** A compute-bound control shows
  speedup climbing with op-cost (1.70 → 2.60×), so the flat 1.8× was *store reads*, not a
  limit on entity-compute parallelism.
- **Axis-1-engine host**: ~20–28× wall-time; the bottleneck then *moved off* the evaluator
  onto store I/O.
- **The time-axis negative**: the floor parallelizes **maps, not folds**; the serial floor
  is k+1 barriers, O(1) in N.
- **The whole-state O(N)-per-tick floor is a representation choice, and it collapses.** Arch
  reframed it; the probe (`programs/subtree_test.go`) confirmed per-tile subtree state +
  content dedup gives an **O(changed)** tick — sparse collapses **21.6×**, dense stays O(N).
  Doom-class sims are sparse, so they never hit this floor. The former Doom blocker is now a
  co-design (a subtree-state descriptor/host convention), not a wall.

**Waiting on arch** for the subtree-state descriptor shape, the collection primitive
(`concat`), and `PROPOSAL-CONTINUATION-STANDING-MODEL` §4. Nothing unblocked remains here.

### 1a. AE-5 Axis-1 conformance admission — ~~**GREEN (LOCKED)**, 2026-07-23~~ **LAPSED — re-measured 2026-08-25, NOT LOCKED**

> **Do not quote this section as current.** Re-run on 2026-08-25 against the live corpus (362
> vectors, `8d2f55c8…`): **334 agree, 0 diverge, 28 INCOMPLETE — NOT LOCKED.** The lock below was
> real on 2026-07-23 and lapsed when the v3.24/v3.25 primitives landed. **Nothing told us, because
> the harness skips unless `AXIS1_ADMISSION_CORPUS` is set and no target sets it** — so it has been
> reported as a passing suite ever since. §0c has the full measurement and the resulting rows; the
> withdrawal is routed in `reviews/AXIS1-ADMISSION-LAPSED-2026-08-25.md`. Kept below verbatim as
> the record of what was true then.

Core-go's AE-5 packet (`entity-core-go/docs/status/ROUTING-2026-07-23-ae5-axis1-inproc-admission.md`)
asked workbench to run the frozen 330-vector inproc compute corpus in-process through **Axis-1** (the
only alternate engine, and it lives here) and hand back the alternate-engine emission; a green run
folds EXTENSION-COMPUTE §11. **Done and green.** Full write-up:
`docs/architecture/reviews/COMPUTE-AE5-AXIS1-ADMISSION-RESULT-2026-07-23.md`.

- Harness `entitysdk/axis1_admission_test.go` mirrors core-go's reference emit loop, swapping in
  Axis-1; corpus + reference emission reproduce byte-exact (`9131a93d` / `419ca55d`); the emission is
  accepted by core-go's `verify`/`cross-bless` with no adapter.
- The run found **two real Axis-1 bugs, both fixed** (Axis-1 is workbench-owned): the uint-index
  `type_mismatch`/`index_out_of_range` divergence (F-2 ruling never transcribed — `axis1/arith.go`),
  and the missing native `compute/apply` closure application that forced a tail-recursion deopt
  (`axis1/{node,decode,eval}.go`). After both: `verify --require-alternate` 7/7 guards pass,
  `cross-bless` **330 agree, LOCKED**.
- **Owed:** core-go re-runs verify + cross-bless on its side to bless, then arch folds §11.
- **Rider for arch:** AE-5/§11 is Axis-1's graduation to a conformance-admitted engine — the
  Axis-1-home question (workbench research engine vs a second `ext/compute` engine) is surfaced in the
  review doc §5.

### 1b. Avalonia program-chrome catch-up (2026-07-27) — CLOSED

Closed the C# side of `docs/status/HANDOFF-2026-07-27-avalonia-program-chrome-catchup.md`
(the workbench half — interactive Life + the pointer-gap proposal — shipped the same day;
that handoff is left as the historical ask). All four items landed in `ProgramPanel.cs`,
verified pixel-for-pixel via `make smoke-xvfb-program PROGRAM={life,snake,asteroids,life-edit}`
(real X11 + Skia, not just headless) and the 51-test headless suite (`make test`), both green:

- **Item C (was a live bug):** `BitForKey` only parsed the legacy string keymap form, so
  keyboard input was dead for Asteroids and interactive Life since their 2026-07-24 roled
  re-declaration. Fixed to accept both forms (mirrors `programs/controls.go::ParseKeymap`).
  Added the on-screen standard controller (d-pad + labelled action buttons, built once per
  key-set input from `scene.keymap`) — buttons pair glyph+label text since the podman
  runtime's font set lacks colour-emoji coverage (bare glyphs render as tofu).
- **Item B (was a live bug):** views were keyed by shape, so the `status` port drew on top
  of the `display` port for every program. Rekeyed by port name; `status` now renders as a
  caption docked above the board. Caught a second latent bug fixing this: the caption's
  custom-drawn `Control` has no `MeasureOverride`, so a bare `DockPanel.Dock.Top` child
  collapsed to zero height — wrapped it in a fixed-height `Panel` (a `Panel` arranges
  children to its own bounds regardless of their `DesiredSize`, same reason `_stage` already
  worked for the board views).
- **Item A:** `DisplayListShapeView` now reads `scene.render` and fills closed quads
  (skipping the reserved `DisplayKindBackground` kind) when `render == "fill"` (Life/Snake);
  Asteroids' default `stroke` wireframe is unchanged.
- **§2:** `life-edit` wired into `avalonia/bridge/program.go`'s dispatch and registered as
  `program-life-edit` in `Program.cs` (+ `SmokeDriver`'s program-cycle map).

Nothing here touches the still-open pointer/click gap
(`docs/architecture/reviews/PROPOSAL-GENERIC-HOST-POINTER-INPUT-DEVICE-2026-07-27.md`) —
that stays blocked on arch, as before.

### 1c. Full-repo health sweep (2026-08-13) — CLOSED

A ~2.5-week gap (07-27 → 08-13) had `dev` untouched but the sibling `entity-core-go` moved
dozens of commits, which had silently broken `make test` here — the earlier catch-up sessions
only ran `make test-programs`, not the full sweep. Ran `make test` / `make lint` /
`avalonia/bridge` smoke-compile / `avalonia make test` end to end for the first time since;
three real defects found and fixed, all on our side of the fence (core-go itself untouched):

- **Build break:** core-go's coordinated rename `types.InboxNotificationData` →
  `types.SubscriptionNotificationData` (+ wire type `system/protocol/inbox/notification` →
  `system/subscription/notification`, `d7e44f6`, a single-round MUST with no dual-kind
  window) had never been adopted here. Updated the 5 call sites
  (`entitysdk/subscription.go`, `entitysdk/subscription_handler_direct_test.go`,
  `workbench/notification_ingest.go`, `workbench/blob_resolve.go`,
  `workbench/test_helpers_test.go`) plus stale example code in
  `docs/architecture/APPLICATION-HANDLER-INTEGRATION.md`.
- **Two capability-check regressions** (`shellcmd` `TestStage3_CapDelegation_Positive`,
  `TestStage4_CaseH_RestrictedCapsMesh3`): root-caused to core-go's `43573d3` (Aug 5) —
  connect-time capability assembly now applies real advertisement-discipline filtering
  (`ConnectHandler.AssembleInboundGrants` → `filterAdvertisedGrants`), and under the §PR-8
  canonicalization a self-issued grant can only ever advertise coverage over **its own**
  peer namespace (`"*"` → `/{peerID}/*`). Both tests' scoped grants listed `Resources:
  ["*", "/*/*"]` — the `"/*/*"` (cross-peer) entry can never be covered by a same-peer
  advertisement, and since coverage requires *every* Include member to match, the **whole
  grant entry** silently dropped, not just that member. Fixed by dropping the redundant
  `"/*/*"` (the operations under test all target the granting peer's own namespace, so bare
  `"*"` is sufficient) — confirmed empirically (reverted, reproduced the 403, re-applied).
  Documented in both tests' doc comments so the next drift doesn't re-diagnose this from
  scratch.
- **Makefile hygiene:** `test-inspect` was missing from `.PHONY` (its sibling
  `test-programs` wasn't) — file-existence-based tracking on that target name was tripping
  a stat/permission error under the containerized build, failing `make test` at the last
  step. Added it.

Full sweep green after: `make test` (all 9 native modules + `inspect`), `make lint` (vet
clean across every module), `avalonia/bridge` smoke-compile, `avalonia make test` (51/51
headless). No code changes to `entity-core-go` — read-only archaeology (`git log -S`, diff
review) to root-cause, per the sibling-repo boundary.

### 2. Stabilization pass (2026-07-22) — CLOSED

Structural cleanup so the next arch ask lands on solid ground:
- **`programs/` extracted into its own module.** The 21 `workbench/program_*.go` files (the
  generic host, descriptors, authoring, Life/Snake/Asteroids/heavyfield/chain/shapes) moved
  out of the app's renderer-neutral model layer into a sibling module, and lost the now-
  redundant `program_` filename prefix. Verified zero coupling before the move: of the 316
  top-level symbols in non-`program_` workbench files, the only ones the cluster referenced
  were `entitysdk`-qualified. Dep direction is `programs → entitysdk`; `avalonia/bridge`
  repointed. New `make test-programs`; wired into `test-native` and `LINT_MODULES`.
- **perfreview rot guard landed** (the backlog item). `make lint-perfreview` runs
  `go vet -tags=perfreview`, and `lint-native` depends on it. Compile-only on purpose —
  the benches take ~20m and must never run under `-race`. It is currently **clean**, so the
  module is *not* rotted; the guard is what keeps it that way.
- **The minimize crash got a headless repro suite** —
  `avalonia/tests/Workbench.Headless.Tests/PanelStackZeroCollapseTests.cs` (see below).

### 3. The share / multi-peer arc (the live thread) — steps 3a and 3 DONE

The one doc to read is
**`docs/architecture/reviews/REVIEW-SHARE-AND-CONNECTIVITY-ALIGNMENT-2026-08-17.md`** —
§5.1 is the ordered plan, §7 is the N1 ruling, §8 is what landed 2026-08-18 and the
correction it forced. Companion packet: `reviews/CORE-GO-ASKS-2026-08-17.md`.

Where the six steps stand:

| step | what | status |
|---|---|---|
| 1 | Commit + send the review | done (`ee96c5f`, `0e86a9f`, `ca2e0cb`) |
| 2 | Foreign-namespace subscription gate on the Go arm | done (`b3848c1`) — and it found the watch-hub `send on closed channel` |
| **3a** | **Consumer-side `published-root` reader** | **done** — `entitysdk/published_root.go` |
| **3** | **Target prefix on the sync surface (source ≠ target)** | **done** — `MirrorSinceLastSeen` + `InstallRevisionMirrorChain` + `revision mirror` |
| 4 | Follow vocabulary settled with browser-rust | **open — needs browser-rust.** Arch confirms the two pieces (one follow verb with a `strategy` field; a per-follow minted capability) need no arch ruling |
| 5 | The first `app/share/*` record | **arch half DELIVERED, our half STARTED (`146f9a4`).** `APP-CONVENTION-SHARE` v0.1 authored (`bb86cd1`, `ROUTING-i` §4); `entitysdk/share.go` ships the `app/share/*` type vocabulary, the tagged target union, `ShareGrants` (with `peers` omitted), `ValidateShareGrants` (the §1.1 MUST as a refusal), `AuthorShare` and `ShareWithdrawalNotice`. Vectors **SHARE-4** and **SHARE-6** pass, plus three shape pins. The authoring/validation layer is pure, so it is green **through** the kernel block; persisting the record + delivering tokens is the half that waits. `strategy` still open on browser-rust (step 4) |
| 6 | `APP-CONVENTION-CHAT` review as a consumer | **DONE 2026-08-20** — `reviews/APP-CONVENTION-CHAT-CONSUMER-REVIEW-2026-08-20.md`. Three findings, each grounded in something this tree already hit: §4's `.list`/subscribe over `/{P}/…` needs a **mirror capability the proposal never names** (AP11 + the §PR-8 403 we shipped wrong first); the per-author `prev` chain has **no gap rule** and it is the same construction browser-rust already flagged in our follow; `attachments` is a **Layer-2 chunking contract** wearing a Layer-1 spelling. Plus one unlisted substrate dependency (content-store GC — a conversation is the purest unbounded-append workload there is) and a measured datum for `[ASK-ARCH-CHAT-2]` (the 20.3 MB/peer delivery ring). **Not building it** — the proposal is DRAFT, not `RULED` |

**What 3a/3 mean in practice.** A peer can now read another peer's signed
`system/peer/published-root` (full verification: content-hash recompute, signature against
the key derived from the Base58 peer-id, `prefix` §3.3a discipline, monotonic seq floor),
and mirror that peer's subtree into `/{them}/{their path}` — the V7 §1.4 cached-remote
shape browser-rust's F1 is about — instead of into our own namespace. Both the one-shot
pull and the standing follow chain. The destination is **derived from the publisher's
signed prefix**, never caller-chosen, per arch's amendment.

**The correction worth carrying.** W3 told core-go the mirror needed "no wire change — the
blocker is entirely ours, in an SDK signature." Half right. `tree:merge` pre-checks put
authorization on every target path, and a self-issued `Resources: ["*"]` is peer-local under
§PR-8, so every `/{them}/…` merge 403s. The fix is a capability that names the publisher's
namespace (`MintMirrorCapability`) plus a per-call caller-cap seam on the executor — not a
signature change. **No ask on core-go**; their behavior is correct. Full account in §8.2;
ratified as **D19** with **AP10/AP11** in the charter.

**Axis B (connectivity validation)** is unstarted, **confirmed unblocked by arch**, and the three
constraints are confirmed correct as stated (§5.2 sizes it). One caveat added 08-18: **no TURN
credential mechanism is specified anywhere in the corpus** (arch queue Q18). Build against static
config and do not invent a credential shape — hitting that wall and routing it is the forcing
function.

**Scoped with browser-rust 2026-08-18** —
`docs/architecture/reviews/CONNECTIVITY-CONVERGENCE-2026-08-18.md`. They named the four app-tier
pieces our arm is missing and we verified all four against our own tree: no `ext/signaling`
consumer, no `system/peer/status` read-model (`ConnectedPeers()` is a pool snapshot), no
`maintain-peer`, no connector/`meet`. **The transport is not the gap** — `AppPeer`
listens and dials WebSocket already (`ListenWebSocketReady`, `Connect("ws://")`), and `shellboot`
routes a `ws://` `ListenAddr` at peer creation. What is missing is **self-publication**: nothing
writes `system/peer/transport/{our-peer-id}/*`, so a browser cannot learn we accept a socket.
That inverts the order — profile publish is the precondition, not a peer, of the other three.
**No WebRTC on the Go arm** (§6.5.2d is latent without a native terminator; browser↔Go is `wss`,
browser↔browser is WebRTC).

**Piece 1 of 4 landed 2026-08-18 — self-publication.** `AppPeer.AdvertiseTransport(dialURL)`
writes a `system/peer/transport/*` profile under **our own** peer-id, and `PeerManager.Create`
calls it when the listener binds (new `Config.AdvertiseURL` for when the routable address differs
from the bound one). Wildcard and port-0 advertisements are **refused, non-fatally** — the peer
still listens, `HostedPeer.AdvertiseErr` says why, and nothing false goes in the tree. Pinned by a
real-session test through `Create`, mutation-checked. Found on the way: the Avalonia bridge's
`PeerListenAddr` reported `listening: false` for **every** WebSocket peer, because it gated on
core-go's `Peer.Addr()` and only `ListenReady` sets `p.listener`; fixed to gate on `ListenScheme`.
**Pieces 2 and 3 landed the same day.** The lesson from AP13 applied immediately: **core-go
already had both halves.** `core/peer` writes every liveness transition (`connected` at handshake,
`suspect` at the dispatch seam, `disconnected` on keepalive miss) whether or not anything is
listening, and `ext/network` implements maintain-peer / release-peer / status / close plus the
§4.1 reconnect continuation graph. Neither needed authoring — they needed *registering* and a
consumer.

- **Liveness read-model** — `Store.PeerLivenessOf` / `PeerLivenessAll` / `OnPeerLivenessChange`
  (+ `AppPeer` wrappers) over `system/peer/status`, and `workbench.PeerLivenessModel` as the
  renderer-neutral view with connected/suspect/disconnected counts. Prefix-subscribed, never
  scan-and-filter. This replaces reading `ConnectedPeers()` in a renderer: the pool snapshot
  cannot express `suspect`, cannot say *why* a peer went, and disagrees with the tree whenever a
  connection is evicted without a demotion. Three properties the model carries deliberately:
  absence is reported as absence (a stranger is not a goodbye), `LastSeen` is a transition
  snapshot and **not** a heartbeat (§5.4.1), and the enum is three-state — `reconnecting` is not
  a status the tree can hold.
- **`system/network` handler wired** (`ExtensionsConfig.Network`, default-on) with the
  post-construction `Bind`, plus `entitysdk.NetworkClient` — `MaintainPeer`, `ReleasePeer`,
  `Status`, `MaintainedPeers`, `Close`. Registering it starts nothing: the continuation graph is
  installed per-peer by a maintain-peer call.

**Spec finding, found by running it:** §2.7's `maintained_peers` is **not** the maintained set.
§4.3's own pseudocode enumerates every entity under `system/peer/status/`, so a released peer
keeps its row and loses only its `session_id` — verified against a real handler when the obvious
assertion failed. Routed to arch; `NetworkClient.MaintainedPeers()` is the `session_id != ""`
filter in the meantime.

**Remaining on the four: connector registry + `meet`** — and both of its gates have now returned,
so the next session starts here rather than re-scoping it. **D20 pre-check done (2026-08-18), so
nobody prices this against our own tree again:**

- **The shape is ruled.** Our `CONNECTIVITY-CONVERGENCE` §2 said we would take piece 4 earlier *"if
  your `meet`-as-DISCOVERY-backend question (arch Q8) returns in a shape that makes 4 cheap."* It
  returned on **2026-08-17** — `ROUTING-2026-08-17-c` §1: **confirmed, token minted `rendezvous`**,
  `pair` mode is **not** discovery, and the candidate is the §2.2 successor pair with
  `identity_hint` **absent** (TOFU). `meet` is a DISCOVERY backend implemented on the SIGNALING
  carrier — *the key introduces; it never authorizes* (SIGNALING §1.2) meeting DISCOVERY §2's
  *discovery is the initiator of the grant, never the authority*. Read
  `PROPOSAL-DISCOVERY-RENDEZVOUS-BACKEND`, not the summary, before building.
- **The carrier already exists in the substrate.** `../entity-core-go/ext/signaling/` ships
  `HandlerPattern = "system/signaling"`, ops `offer` / `collect` / `advertise` (`const.go`), a
  `Client` (`client.go`), plus `punch.go`, `webrtc.go`, `reflection.go`, `pool.go`, `coordination.go`
  and the rendezvous `key.go`. **There is no `meet` op and there should not be** — `meet` is the
  DISCOVERY-side logic over this carrier, which is the piece that is genuinely ours.
- So piece 4 is the same shape pieces 2 and 3 turned out to be (AP13/D20): **registration and a
  consumer, not authoring.** What is absent in our tree is an `ext/signaling` consumer — grep:
  `grep -rn 'ext/signaling' entitysdk/ workbench/ shellcmd/ shellboot/` returns nothing.
- Constraints unchanged and confirmed: SIGNALING §3.4 same-provider is a **MUST** (both arms on the
  same pool or silent never-meet); `data_relay` is `policy: open` only; **no TURN credential
  mechanism exists anywhere in the corpus** (arch Q18) — build against static config and route the
  wall rather than invent a credential shape.

**Piece 4, started 2026-08-19 — the carrier landed; the backend is READY TO BUILD.** The D20
pre-check said "registration + a consumer" and that is right; what it did not check is whether the
DISCOVERY surface it registers into exists in landed spec. It does not — which turned out to be
worth knowing and not worth stopping for:

```
$ grep -rn 'rendezvous' ../entity-system-architecture/specs/extensions/EXTENSION-DISCOVERY.md
   (no matches)          # arch c984f93 — Version 1.0, enum still <"mdns" | "qr" | ...>
```

`PROPOSAL-DISCOVERY-RENDEZVOUS-BACKEND` is stamped **RULED 2026-08-17** and **none of its §6 fold
has landed** — no `rendezvous` enum token, no §5.5, no version bump. §5.5 is where the mode split
and the TOFU + successor requirements live, so the backend's whole normative content exists only in
the proposal, and `AGENTS-STANDARD` says implement against the **landed spec**. Defining the token
locally is AP20's exact shape one step earlier. `entity-core-go` matches the spec, not the proposal
(`DiscoveryBackendMDNS` and nothing else) — the gap is upstream of them. Ask routed:
`docs/architecture/reviews/DISCOVERY-RENDEZVOUS-FOLD-ASK-2026-08-19.md`. **A RULED stamp is a
decision, not a normative surface** — we read it as landed and had to grep to find out otherwise.

**HOLD LIFTED by operator ruling, 2026-08-19. Piece 4 is READY TO BUILD — build it against the
proposal.** The hold was mine and it was wrong; recording the correction so no session re-derives
it.

Re-checked against arch HEAD (`05faaa5`): arch's `docs/COHORT-OPEN-ITEMS.md` carries this as
**R-10, owner `arch`, OPEN** — *"ruled, not folded"*. The facts:

- **Nothing here is undecided.** The proposal header reads `Status: RULED 2026-08-17`, and its §0
  states the normative delta is **one enum value** in `EXTENSION-DISCOVERY` §2.1 plus a
  composition subsection — **no wire change, no new entity type**. The mode split, TOFU, and the
  successor chain are fully written in §2/§5. It is buildable precisely, not guessed.
- **What is missing is arch writing that ruling into spec text.** Editorial, R-10, theirs.

**The operator's ruling: implementing a RULED proposal is normal practice in this cohort, and is
one of the ways a spec gets validated before it is folded.** `AGENTS-STANDARD`'s *"implement
against the landed spec, not in-flight proposals"* is aimed at **unruled** proposals — building on
a shape nobody has decided. A proposal stamped RULED is a decision; treating it as in-flight is
over-reading the rule, and the cost of that over-read here was a self-inflicted stop on work whose
content was fully determined.

**AP20 does not apply and I mis-cited it.** AP20 is about inventing a constant to satisfy a spec
table *whose referent does not exist* — the tell being that you must write a doc comment
explaining the absence. Here the referent exists and is named in a ruling; we are implementing
ahead of the fold, which is a different act with a different failure mode.

**Build it, and route what building teaches.** The known cost is small and stated up front: if the
token's spelling moves in the fold, we re-cut a few hundred lines of backend. The enum is open
(`<"mdns" | "qr" | ...>`), so an undeclared token is not a conformance violation for a consumer.
The commit says it is built against `PROPOSAL-DISCOVERY-RENDEZVOUS-BACKEND` at arch `05faaa5` and
not against landed `EXTENSION-DISCOVERY`, so the coupling is greppable when the fold lands — and
anything the implementation surfaces goes back to arch as feedback on the proposal, which is the
point of doing it in this order.

The routed ask (`docs/architecture/reviews/DISCOVERY-RENDEZVOUS-FOLD-ASK-2026-08-19.md`) stands as
a fold request, **not** as a blocker on us; it should be re-framed as "here is what we learned
building it" when piece 4 lands.

**PIECE 4 IS BUILT — 2026-08-19, `021e5c2`.** `entitysdk/rendezvous.go`, against the RULED
proposal at arch `05faaa5`, not against landed `EXTENSION-DISCOVERY` v1.0. **The four-piece
connectivity list is complete.**

The D20 pre-check held: **registration and a consumer, not authoring.** `ext/signaling` ships the
ops, the client, the key derivation and the §6.3 signed container; `ext/discovery` ships the
substrate, the candidate store, the watchable prefix and `PromoteSuccessor`. **There is no `meet`
op and there should not be one** — now confirmed from the implementing side rather than the
reading.

The rules that are not obvious, each pinned and each mutation-checked:

- **`peer_id` and `identity_hint` are both ABSENT on `candidate_0`** — and we hold a
  *cryptographically verified* counterpart identity at that moment (core-go's §6.3 container
  returns a `VerifiedSigner`). The backend does not use it. §2.2 step 1 puts the peer-id after
  IDENTIFY over the **admitted channel**, and SIGNALING §1.2 is why: reaching a key proves
  someone derived that key, never that the channel admission opens belongs to them. This is the
  half an implementation gets wrong by doing the helpful thing.
- **`pair` mode is refused** (proposal §2.1) — both peer-ids are inputs, so there is nothing to
  surface and a candidate would be the caller's own input presented to a user as a stranger.
- **`endpoint_hint` locates a DEPOSIT, not a bucket.** The proposal's "bucket/key locator" cannot
  tell two peers at one tag from one peer seen twice. For `secret` mode it carries neither the
  secret nor the derived key — a candidate exists to be *shown to a user*, which is the last
  place a credential belongs.
- **Skip-own, departure-reaping, one candidate per standing peer.** A deposit falling out of the
  bucket at the node's TTL is a real departure signal (§3.0.1 rule 3's shape, not rule 4's
  one-shot waiver), and the candidate entity embeds `observed_at`, so re-emitting per poll would
  show one peer N times.

**Scope boundary:** `candidate_0` only. The §2.2 successor promotion, the §2 grant decision and
IDENTIFY are the admission path, not the backend.

**`Extensions.Discovery` is new and exists because of this** — see AP26. The substrate was wired
only for **listening** peers, which was right while mDNS was the only backend and exactly wrong
here: a rendezvous peer stands at a mailbox *because* it has no reachable listener, so the gate
excluded precisely the peers the backend serves. Default unchanged; a control arm asserts that.

**Routed, which is the other half of building ahead of a fold:**
`docs/architecture/reviews/RENDEZVOUS-BACKEND-BUILD-RESULT-2026-08-19.md` — four things §5.5
should carry (the TOFU rule's reason **and its cost**: §2.2.1's fail-closed IDENTIFY comparison is
structurally unavailable to every rendezvous candidate, so TOFU is the ceiling and not a fallback;
deposit granularity; the mode-dependent locator; a reap rule for a non-mDNS departure signal).
**None is a defect in the ruling** — the layering, the token and the composition all held up under
construction. The earlier fold-ask keeps its §3 ask and has its §2 (the stop) withdrawn in place.

**Not cross-impl.** Everything above is Go meeting Go over its own node — cohort-consistent at
best. The interop claim needs browser-rust's `src/rendezvous.rs` on the other end of **one shared
node** (§3.4), which we have not run and are not asserting.

**Ratchet: AP26** — *the first consumer's precondition became the substrate's.* Not promoted; it
has bitten once. Charter is D1–D22 / **AP1–AP26**.

**The carrier half is landed spec (EXTENSION-SIGNALING v1.1) and is done.** `entitysdk/signaling.go`
— `AppPeer.Signaling(nodePeerID)` with `Offer` / `Collect` / `Advertise` through `extDispatch` (so
non-2xx maps to `*entitysdk.Error`, not core-go's proto-SDK), key derivation **delegated** to the
kernel's `ext/signaling` because it is a Layer-2 algorithm — two peers whose key bytes differ
silently never meet, and no same-impl test can see it. Plus `ExtensionsConfig.SignalingNode`, an
opt-in hosted node: every other extension in that struct is a capability the peer *has*, this one is
a service it runs *for other people*.

Gates: three peers on the wire (node + two clients, TCP, pooled connections) meeting at a tag —
non-destructive collect, arrival order, content-hash dedup, empty-not-404; the derivation properties
that have no error path (mode separation, pair symmetry, determinism); and §3.4's same-provider MUST
shown as two nodes with the same key never meeting. **The meeting gate uses two real peers on
purpose** — one peer offering into its own node proves the bucket and not the meeting, which is the
shape we just promoted to D22.

### 4. Publisher conformance — CLOSED 2026-08-18, the corridor emits a real signed root

`publish/publish.go` advertised `signed_pointer: "system/peer/published-root"` +
`freshness: "static-immutable+signed-pointer"` and did no signing at all; the artifact at
`{manifest_url_prefix}` was the http-poll *transport profile*, which `EXTENSION-NETWORK` §6.5.3.1
rules out in as many words. Step 3a made it self-refuting — our own `ReadPublishedRoot` rejected
our own publisher's output on gate one. Arch routed it 08-17 and again 08-18.

**Exit B taken (emit a real signed root), not Exit A.** The handoff priced B as its own arc on the
strength of *"our closure walk is shallow and D3 requires the trie closure"* — true about
`publish/`'s walker over bound entities, and irrelevant to the obligation, because
`tree.CollectNodeClosure` in core-go already implements D3 exactly and cites §6.5.6 Amendment 10
as the reason it exists. **An estimate that prices a spec obligation off our own code's shape,
without checking whether the substrate already implements it, is an estimate of the wrong thing.**

What ships (`publish/signed_root.go`, new):

| object | now |
|---|---|
| `{out}/manifest` | the signed `system/peer/published-root` (3-key wire entity, `content_hash` + `prefix`) |
| `{out}/transport-profile` | the http-poll profile, out-of-band per §6.5.4 / proposal D5 |
| `{out}/content/…` | + the transitive trie closure of `root_hash`, the published-root, and its signature — the §6.5.3 publish-side MUST |
| `{out}/{peer}/system/signature/{hex}.bin` | the §5.2 invariant pointer, as an ordinary two-hop TREE_GET leaf |
| `{out}/{peer}/system/peer/published-root/{peer}.bin` | `signed_pointer` names a path, so the path resolves too |

Acceptance test is written **as a consumer** — no peer, only the emitted files: recompute the
manifest hash from its own bytes, resolve the signature two-hop, verify against the key derived
from the Base58 peer-id, walk the CHAMP trie from `root_hash` over the emitted shard asserting no
404 (`TestPublish_SignedRootVerifiesFromTheEmittedFiles`).

**Three things fell out of building it**, all in
`docs/architecture/reviews/archive/PUBLISHER-CONFORMANCE-RESULT-2026-08-18.md`:

1. **A signed root and a filtered publish are incompatible — we refuse at the emitter.** The
   closure obligation would upload the `IncludeType`/`IncludePath`-excluded entities' bytes under
   `content_url_prefix` anyway (a leak, and *because* we advertised a signed pointer); withholding
   them instead is the silent short walk browser-rust measured (`9a9c0f5`). §6.5.3 states the
   broken-walk direction; the leak direction is unstated. Routed as a candidate.
2. **core-go ask: `published-root` `seq`/`predecessor` are process-memory only.** Measured — three
   publishes of three *different* roots through `ext/publishedroot.Publisher` emitted
   `seq=1 / predecessor=nil` every time. §6.5.6 makes both a MUST. We source them from the store
   ourselves and would rather not own a second minting site.
3. **arch ask: the content-only mirror §6.5.6 sanctions has no expressible `freshness`.** The enum
   is `live | async | static-immutable+signed-pointer`, and the MUST list requires `signed_pointer`
   for both non-live values. Found while pricing Exit A; we emit nothing into the gap.

**Unblocked by this:** the Go-published / Rust-consumed cross-check with `entity-browser-rust`
(they emit and walk signed roots already, rust↔rust) — arch calls it the most valuable interop
result on this track, and the only one that is not cohort-consistent.

**Process (AP12, ours).** Arch routed the finding on 08-17 addressed to us by name; we did not open
a row and shipped three commits past it. `AGENTS.md` now makes the sibling-arch read a session-start
step. Catalogued, not ratified — first time in this shape.

### 5. R3 — the resolver-config ships (2026-08-18)

Arch's `096fa96` folded the default `name_format_dispatch` globs into
`EXTENSION-REGISTRY` §4.1a. That was **R1**, the item R3 was waiting on, so R3 — ours jointly
with browser-rust — is unblocked and now done on our side.

D20 check first, and this time the substrate did **not** have it: core-go reads the config and
applies the dispatch list, but nothing in the cohort writes a default one (construction exists
only in the `validate` harness). Shipping it is app-tier work, as arch said.

`entitysdk/resolver_config.go` — `DefaultNameFormatDispatch` (the six §4.1a rules, in order),
`DefaultResolverConfig` (that list + a local-name-only chain), `ValidateResolverConfig`, and
`AppPeer.InstallResolverConfig` / `ResolverConfig` / `EnsureResolverConfig`.
`EnableLocalNameResolver` now writes the dispatch list too — it used to write the chain alone,
which was harmless only by accident (no dispatch list ⇒ every name consults every backend, and the
chain happened to be local-only).

- **Order is the contract.** First-match-wins, and rules 4/5 overlap on every dotted authority. A
  glob cannot say "undotted", so `*@*.*` must precede `*@*`. The pin asserts the sequence — a
  set-membership test passes on the reversed list, which routes every domain-scoped name to
  peer-issued and then toward a catch-all that must not see it.
- **The catch-all MUST be local-only** (§4.1 step 2 — "the primary privacy mechanism"). We
  **refuse** rather than normalize: §11.1 permits either, but silently rewriting an operator's
  privacy config into a different one means they never learn they did not get what they asked for.
- Two cohort observations we filed as inert. **They were not** — see §6b.

**Operator surface:** `peer status` (new) renders the tree's lifecycle record —
connected/suspect/disconnected, the transition reason, and a coarse age — deliberately a
*different* answer from `peer ls`, which lists this session's alias table. A peer that connected
to **us**, or one released an hour ago, appears in the first and not the second, and only the
first can say `suspect`. Empty output says *"nothing has ever transitioned"* rather than showing a
blank table that reads as "nothing is connected". The SINCE column shows `failing_since` or
`connected_at` and **never `last_seen`** — rendering a transition snapshot as "last heard from"
would tell an operator a healthy peer had gone quiet for hours.

### 6. CLEARED — the tree was red across 6 of 9 suites, and it was core-go's (2026-08-18)

> **RESOLVED same day. core-go `7593618` — `DispatchLocalExecute` now passes
> `handler.WithResource(req.Resource)`.** Re-run here against it: **`make test` exit 0, zero
> failures** across its eight suites (`entitysdk`, `shell`, `shellboot`, `shellcmd`, `shellpanel`,
> `workbench`, `programs`, `inspect`), plus **`make test-publish` green** separately — `publish` is
> not in the `make test` target. `make lint` clean. **323 → 0.** Our call sites were correct the
> whole way down and nothing here changed to accommodate the defect — the decision not to work
> around it in the app repo is what kept the fix a one-liner in the right tree.
>
> *(Per AP15: the count above is from a run that completed. A first attempt exited 2 on a transient
> `cd: can't cd to shell` while other container jobs were touching the tree concurrently; re-run
> clean with nothing else running, `shell` passes in 2.27s. Reported rather than quietly dropped.)*
>
> **The reproducer became their regression test.** `TestDispatchLocalExecute_CarriesResourceToHandler`
> is our kernel-level shape landed in `core/protocol/local_entry_resource_test.go` — red pre-fix at
> status 200 with `Resource == nil`, green after.
>
> **Their ratchet, worth carrying here too:** `dispatch_equivalence_test.go` asserted *result*
> equality between the wire and in-process entry paths but never the handler-visible *context*, and
> `subdispatch_resource_dimension_test.go` drove both sub-dispatch directions without ever entering
> through `DispatchLocalExecute`. **Two entry paths claiming equivalence need a test that asserts
> the handler-visible context, not just the result.** The SDK path exercised it; theirs did not.
> Folded as **AP17**.

The history below is kept because the *shape* is the lesson, not the outage.

### 6 (historical). BLOCKED — `make test` is red across the tree, and it is core-go's (2026-08-18)

**Not ours, not worked around, routed — and it has LANDED in core-go (`0b9e261`), not merely
in-flight.** The change removes resource inheritance in sub-dispatch per arch
`ROUTING-2026-08-18-g` §5 (**ruled, normative** — `ENTITY-CORE-PROTOCOL` §5.2 at arch `980ddf1`). The ruling is right. Removing the inheritance also removed the only channel by which
the in-process **entry point** delivered a resource it was explicitly given:
`DispatchLocalExecute` sets `rootCtx.Resource` and then dispatches with `WithCapability` alone.

**Measured tree-wide in the 2026-08-18 audit — 323 failures across 6 of 9 suites**, which is
substantially worse than first reported. `make test` stops at the first failing package, so the
earlier per-suite numbers were taken through a keyhole; each suite must be run on its own
(`make test-sdk`, `test-shellcmd`, …) to see the radius:

| suite | failures | |
|---|---:|---|
| `entitysdk` | **171** | first reported as 5 |
| `shellcmd` | **71** | |
| `programs` | **46** | 52 with subtests |
| `shell` | **14** | first reported green — it was not |
| `shellboot` | **12** | first reported green — it was not |
| `inspect` | **9** | not previously reported |
| `workbench`, `shellpanel`, `publish` | 0 | genuinely green |

**It wears five faces, and that is the part worth remembering** — the same defect will not present
the same way twice, because each handler validates its resource independently and says so in its
own words: `resource target path is required` (tree, 306), `bind_cap` (37, downstream of a failed
put), `resource target is required for subscribe` (17), `ambiguous_resource: install requires
exactly one resource` (17), `missing_resource_path` (role, 5). Do not diagnose these separately.
The rest are **cascade** — a test whose setup `Put` was refused then reads an empty tree and
reports a wrong count, a missing path, a surviving roster entry. All one defect.

**The mechanism is now isolated, not just argued** (the measurement the previous handoff flagged as
owed). A kernel-level reproducer — `protocol.NewDispatcher` + `DispatchLocalExecute` with a
wildcard grant and a handler that records `req.Context.Resource`, **no workbench code in the
path** — shows the handler running at status 200 and seeing `Resource == nil`. It is in the packet
verbatim, as the test core-go is missing.

Full packet, including the one-line fix, the reproducer, and the coverage gap that let it through:
`docs/architecture/reviews/archive/CORE-GO-LOCAL-DISPATCH-RESOURCE-2026-08-18.md`.
**Re-run `make test` once it lands.** Do not work around it here — the call sites are correct.

**Historical note, corrected 2026-08-20:** this section once ended *"the packet is still unsent —
it does not reach them until `dev` is pushed."* **It went out with the 2026-08-19 push and core-go
landed the fix** (`7593618`). Kept because the shape is the lesson, not the outage.

### 5a. The cross-impl publish/consume check — three surfaces align, the front door does not (2026-08-18)

The ADR-0012 result: every signed-root result either arm holds is **same-language**, so ours and
browser-rust's agreeing with themselves is cohort-consistent, not independent convergence.
`publish/cmd/crossimpl-fixture` emits a deterministic Go site (pinned seed, so peer-id and every
hash below it are stable) to hand their reader.

Measured against browser-rust `a0145a7`'s `DirFetcher`:

- **Aligned, with no shared code:** content sharded `{aa}/{bb}/{hex}` on the 66-char wire hex,
  bare-hashable bodies, and the two-hop signature at `system/signature/{root_hex}.bin` keyed on the
  published-root entity hash. Their doc calls the latter two *"divergences from upstream"* — they
  are **not** divergences from us. That is the part worth keeping.
- **Not aligned — and it is hop 0.** `DirFetcher::manifest()` reads
  `{base}/{peer_id}/system/peer/published-root`. **Half of this is now FIXED in the kernel:**
  core-go `2bd2380` (ruled by arch *from this run*) dropped the `/{base58_peer_id}` segment that
  `PublishedRootStoragePath` appended — the qualified binding had named the peer twice, and
  core-go's writer and reader shared the helper, so Go-on-Go passed deceptively. Adopted here in
  `00b92c7`; the directory collision is gone.
  **The remainder, re-measured:** we emit `…/published-root**.bin**` (our advertised
  `tree_leaf_suffix`, since the head pointer *is* a tree leaf) holding a 2-key `system/hash`
  pointer; their `manifest()` reads the suffix-less path and expects the 3-key wire entity, which
  we emit at `{out}/manifest`. `ENOENT` now rather than `EISDIR` — still hop 0, and now a narrow
  question about which artifact belongs at which path.

**RULED 2026-08-18 in our favour — `ROUTING-2026-08-18-p` §3, `EXTENSION-NETWORK` 1.8.** The
manifest's location is **discovered** from `manifest_url_prefix`, never derived by convention from
the tree path, and *a consumer MUST NOT join `signed_pointer` onto an origin*. The two fields answer
different questions: `manifest_url_prefix` is where to GET it, `signed_pointer` is what the origin
is asserting. `{origin}/manifest` and `{origin}/{peer}/system/peer/published-root` are equally
conformant; only the advertised one is findable. **`DirFetcher::manifest()` is the defect and the
fix is browser-rust's.** Arch rejected "serve it at both paths" — *"two front doors is not
compatibility; it is the divergence, ratified"* — and upheld the decision not to move our layout.
Nothing owed here; our `manifest_url_prefix` advertisement was conformant throughout.

**Fixture re-cut and handed over (`d940ce0`), and the re-cut found a defect of ours.** The packet
told browser-rust the emission was byte-identical on their machine. It was not: `published_at` is a
field **of** the published-root entity, so a fresh clock moved the root's content hash, the
`system/signature/{root_hex}.bin` binding named after it, two content shards and `{out}/manifest` —
every artifact their reader enters through. Only the trie root and the entities beneath it were ever
stable. `publish.Opts.At` now pins the instant (zero still means `time.Now()`), the fixture pins it,
and two fresh runs diff clean. **AP18** — a claim about emitted bytes settled by reading the emitter
instead of emitting twice and diffing.

Corrected fixture facts (core-go `7593618`): `peer_id 2KLv2nhwtPrL…`, trie
`ecf-sha256:f567bfbd…`, published-root `00e0138dbeb374…`, signature `000ef5f255803…`.

**Still not run end to end:** executing their reader against our fixture needs a test in *their*
tree. Per D19/AP10 everything above except the emitted bytes ships as a prediction with a
reproducer attached.
Packet: `docs/architecture/reviews/CROSSIMPL-PUBLISH-CONSUME-2026-08-18.md` (UPDATE 2 carries the
ruling and the corrected hashes).

### 6a. CORRECTED — R3 shipped against a spec sentence arch withdrew 78 minutes later (2026-08-18)

`24169b9` shipped the resolver-config against `EXTENSION-REGISTRY` **1.6** §4 (*"an ORDERED list,
first-match-wins (MUST)"*). Arch withdrew that in `3670283` → **1.7**: the list is a **filter**, a
name matching several entries is eligible at the **union**, and precedence is
`resolver_chain[].priority`. The `#` column is reference numbering, not evaluation order.

**One live defect came out of it and is fixed.** `ValidateResolverConfig` refused a config whose
catch-all was not the final entry (`catchall_not_last`) — correct under 1.6, where everything below
a catch-all was dead config; under 1.7 those entries stay eligible, so the refusal **rejected a
deployment the spec permits**. Removed, with a regression pin naming the withdrawal
(`TestValidateResolverConfig_CatchAllPositionIsNotADefect`). The order pin became
`TestDefaultNameFormatDispatch_MatchesTheSpecTable` — it pins the six rows and their
`backend_kinds`, not a sequence.

**What did not change:** the six default rows, emitted verbatim in the table's sequence (now
documented as presentational), and **§4.1 step 2's catch-all local-only MUST, still enforced at the
write as a refusal**. 1.7 makes that the load-bearing rule explicitly — the same conclusion resting
on the right sentence.

Routed: `docs/architecture/reviews/archive/RESOLVER-CONFIG-FILTER-CORRECTION-2026-08-18.md`, which also
carries the two cohort observations (`did-key` vs `self-certifying`; `pinned` has no constant) and
the `ROUTING-2026-08-18-i` acknowledgement.

### 6b. REGISTRY v1.13 adopted — both "inert" observations were live defects (2026-08-18)

`f79cc4a`. Arch's `-p` §5 came back on the two cohort observations §5 filed as inert: **they were
dead config in every conformant peer**, because §4.2 makes an unknown `backend_kind` MUST-skip with
a warning, so our shipped default list contained rows a conformant implementation is *required to
discard*.

| row | was | now (v1.13) |
|---|---|---|
| 2 `did:key:*` | `["did-key"]` | `["self-certifying"]` |
| 6 `*` | `["local-name", "pinned"]` | `["local-name", "self-certifying", "out-of-band", "peer-issued"]` |

Row 6 is **not** what `-p` said — v1.12 removed the undeclared `pinned` without naming the declared
token that does the job, and `-q` §2 corrects it to `out-of-band` (§4.1.2: the kind a pin's
synthesized binding carries; §6a.4 makes it dispatchable where `pinned` is not). The row moved three
times in one day and browser-rust pinned the middle version. **Our pin now names the spec revision
it was taken at**, so the next move presents as a red test with a version to compare.

**The tell was in our own source: we had to invent both constants.** `BackendKindPinned` and
`backendKindDIDKey` existed only because §4.1a named strings core-go's enum does not declare, each
with a doc comment explaining the absence. We wrote that explanation twice and still filed it as an
observation. **AP20** — a constant you have to invent locally to satisfy a spec table is a defect in
one of the two documents, never a naming gap.

**The catch-all MUST is re-keyed, and our guard had been refusing a legal config.** `-l` §1
(REGISTRY 1.8) was cc'd to us and unopened: the banned property is **name transmission, not
remoteness**. The two come apart exactly at `peer-issued`, which §6a.4 resolves by content address
through a signed root so the queried name never appears in a request. Our allow-list was
`{local-name, pinned}` — it refused `self-certifying` and `out-of-band`, which dial nobody, and once
rows 2/6 were corrected `DefaultResolverConfig()` failed `ValidateResolverConfig()`: the helper that
ships the default could no longer install it. Now a deny-list over the four disclosing kinds
(`dns-txt`, `well-known-url`, `did-web`, `consensus-anchored`), because §4.2 makes an unrecognized
kind inert and refusing on account of one rejects a config a newer vocabulary permits. Code renamed
`catchall_not_local` → `catchall_transmits_name`.

**The old pin passed under both rules** — it tried exactly one forbidden kind, `peer-issued`, which
the re-key moved from forbidden to permitted. Green was our only evidence the guard was right and it
was compatible with the guard being backwards. **AP19.** The replacement enumerates all four
disclosing kinds and all four admitted ones.

**Finding routed to arch:** §11.1's `REG-DISPATCH-CATCHALL-LOCAL-1` was **not** moved with §4.1
step 2. It still says a catch-all naming *"a remote backend"* MUST be refused and that resolving a
bare name MUST produce *"no read against any remote registry"* — both false against row 6, which now
ships `peer-issued`. **An implementation passing that vector literally refuses the default list the
same document tells it to ship.** Same failure as rows 2/6 one layer out: the rule was re-keyed and
the artifact that tests it stayed on the old property. Nobody copies §11.1, so it drifted silently.

Also pinned: REG-DISPATCH-GRAMMAR-1's refusal half (`-q` §1). The grammar is closed and every non-`*`
byte is a literal, so **no pattern is invalid** and a registry MUST NOT reject one for `?`, `[`, `\`.
We author patterns and never match them, so that is our whole exposure — pinned rather than assumed,
because "closed grammar" has meant "reject at write" everywhere else in this corpus.

Packet: `docs/architecture/reviews/REGISTRY-V113-ADOPTION-2026-08-18.md`.
**D21 earned** (AP12 promoted): a *cc'd* packet is a packet. Session start now greps the arch repo
for every document naming this repo, and STATUS carries the last letter read.

### 6c. REGISTRY v1.14 — the MUST binds the configuration, and our validator saw one row (2026-08-19)

`b9e99e7`, answering `ROUTING-2026-08-19-b` §2 (arch read our tree at `0ba80c6`, source, this
session). Arch widened §4.1 step 2 from the **catch-all row** to the **configuration** — D4, because
a rule that binds one row is evaded by not writing it. Ours enforced the row: `if d.Pattern !=
CatchAllPattern { continue }`.

| door | case | code |
|---|---|---|
| 1 | a **broad** pattern that is not the catch-all — `al*` naming `dns-txt` | `broad_pattern_transmits_name` |
| 1 | the catch-all itself (unchanged, so existing diagnostics still resolve) | `catchall_transmits_name` |
| 2 | **absent/empty** `name_format_dispatch` while a name-transmitting kind sits in the chain | `filter_disabled_transmits_name` |

Door 2 is the one the loop body could not reach — with no rules there is **no row to inspect**, and
`eligible_kinds` returns ALL, so every kind is eligible for every name. `EnableLocalNameResolver`
has carried a doc comment naming this exact hazard since 2026-08-18 with nothing enforcing it: **a
named hazard with no gate is a comment.** Both doors mutation-checked against the pre-widening code.

**§11.1's placement, which we had never implemented:** *"refused or normalized at load"*. Ours ran
on author only. `AppPeer.ResolverConfig` now validates on read, returns the config **anyway**
(non-zero, beside the error — a load-time refusal denies use, not sight), and `EnsureResolverConfig`
refuses rather than reinstalling the default over an operator's config. The case is not
hypothetical: what a config *means* depends on a vocabulary outside it, so an entry that is inert
under §4.2 today becomes disclosing the moment core-go declares that kind.

**Open, routed to arch:** the widened MUST says *"any rule whose pattern matches unscoped names"*
and **supplies no decision procedure**. It cannot be read literally — a name is a flat string, so
`alice.eth` is bare and §4.1a row 3 (`*.eth` → `consensus-anchored`) would violate the MUST the same
table recommends. We read "unscoped" as *carrying no explicit authority marker*, and our
`matchesUnscopedNames` parts from browser-rust's `is_broad` on patterns like `*e` (theirs: narrow;
ours: broad). We took the strict side — it is what their own doc sentence argues for, and refusing
an exotic config costs an error message while admitting one costs every name a user types. A `§11.1`
row is needed; everything in §4.1a is grammar-identical under both readings, which is the same shape
that let `*.lab` survive review in the `name_constraints` case.

Packet: `docs/architecture/reviews/REGISTRY-V114-VALIDATOR-2026-08-19.md`.

### 6d. The CDN corridor runs in both directions — and ours was broken at our own end (2026-08-19)

`c13dfe2`, answering browser-rust's `ROUTING-2026-08-19-d` §5, the one thing they asked for:
**consume us.** Doing it found our half broken first, and worse than theirs — **`fetch` could not
read `publish`.** Four divergences at once:

| `fetch` derived | `publish` emits |
|---|---|
| `{base}/{peer}/tree/{path}.bin` | `{base}/{peer}/{path}.bin` (§6.5.3.1 has **no `tree/` reserved word**) |
| a raw 33-byte hash at the leaf | `ECF({type:"system/hash", data:H})` — Amendment 6, two-hop |
| `sharded-2-flat` | `sharded-2-4` — *and the profile declares it* |
| hex of the 32-byte digest | hex of the **33-byte wire form** (§6.5.3.1 MUST) |

**Both suites were green the whole time**, because each half asserted its own idea of the layout and
nothing asserted they were the same one. Structurally: **`fetch` had no `make` test target and
`publish` was not in `test-native`** — "run everything" ran neither end. Both are in the sweep now,
plus `fetch` in `LINT_MODULES`.

What replaced the derivation is `fetch.Layout` — the Go counterpart of browser-rust's
`PublishLayout`. Peer-id, all three URL prefixes, content layout and both suffixes come from the
publisher's http-poll profile, decoded with **core-go's own type**. One convention is left on
purpose: the well-known `{origin}/transport-profile` a cold-start consumer enters at.

Gates, both mutation-checked and both in the sweep:
- `publish/consume_test.go::TestPublishThenFetch_TheTwoHalvesOfOurOwnCorridor` — our publisher →
  our consumer over `httptest`, cold start, signed root through `manifest_url_prefix`, every page
  hash-verified, absence reported as `404` rather than as unreachability.
- `fetch/crossimpl_test.go::TestConsumeBrowserRustSite` — **their** emission, frozen at
  `fetch/testdata/crossimpl-rust-site/` (their `dev` @ `fbc2c5c`, provenance in its README), four
  entities across two of their sites. Drop the peer-rooted bridge and it 404s at hop 0 — their
  reported failure, reproduced from our side.

**Findings routed, not worked around.** (1) core-go's `types.BuildContentURL` hexes
`EffectiveDigest()` — the digest-only form §6.5.3.1 excludes **by MUST** — so it builds a URL that
resolves against neither publisher in the cohort. (2) `entity.Validate()` cannot be used on a
`CONTENT_GET` body: those are the bare 2-key hashable form, so it reads the absent `content_hash` as
a zero hash and fails against it. That was our own **AP22** instance — a guard that refuses
everything unfamiliar — the same shape as browser-rust's audit F6, which they reversed the same day.

`tree_url_prefix` stays arch's. We consume **both** joins (last segment exactly the peer-id ⇒
peer-rooted; otherwise append), which is a bridge, not a third convention.

**Ratchet: AP21 → D22.** *A contract between two components is only tested by a test that crosses
it; per-side tests are evidence about each side.* Second shape of AP17 (core-go's
`DispatchLocalExecute` equivalence claim, asserted on return values) — different repo, different
layer, one lesson. Charter is now D1–D22 / AP1–AP22.

Packet: `docs/architecture/reviews/CROSSIMPL-CONSUME-RESULT-2026-08-19.md` (to browser-rust, cc arch
+ core-go).

### 6e. REGISTRY 1.16 read — both rulings land outside our code (2026-08-19)

Arch `d3752ca`, routed as `ROUTING-2026-08-19-d` to **core-go**, not to us. Read under D21 because
its own commit message says it moves a MUST and withdraws a vector row — *"a packet that says it
changes a table, a default, or a MUST is read the same session regardless of who it is addressed
to."* §7 confirms our items (R-8/R-9/R-10) are tracked and that nothing in it waits on us.

Two rulings, both verified against our tree rather than assumed:

- **`REG-NAME-CONSTRAINTS-GRAMMAR-1` row 3 withdrawn.** v1.15 required `x/y/z` admitted while
  §6a name-path safety refuses `/` three subsections earlier — unsatisfiable by every conformant
  impl. Issuer-side; `name_constraints` is core-go's `cmd/entity-peer` + `validate` surface and
  appears nowhere in our tree. Nothing owed.
- **The resolver-side TTL ceiling gets a config site: `resolver_chain[].hints.max_ttl`.** Four
  seats had built three different keys; py+rust's site is ratified, plus durable-config-read-at-
  resolution, `0` is undeclared, and a ceiling against a no-ttl binding yields `local_max`. The
  non-conformant "go" in that finding is **core-go**, and they have already landed it
  (`ext/registry/localname.Handler.Resolve(hctx, name, localMaxTTL)`, `validate`'s
  `v4c_ttl_resolver_ceiling`). Our surface is the config we *write* and *validate*:
  `DefaultResolverConfig` sets no `hints`, and `ValidateResolverConfig` rules only on §4.1 step 2,
  so a config carrying `max_ttl` passes untouched. **Nothing owed — and R-9 got more load-bearing
  than it was when we closed it**, since `hints` is now the ratified home of a spec'd control and
  `19786fb`'s round-trip pin is what keeps a tidy refactor from dropping it.

We deliberately do **not** put a `max_ttl` in `DefaultResolverConfig`. A ceiling is a deployment
choice, `0` is undeclared, and inventing a default here would ship an opinion the spec does not
carry.

### 6f. REGISTRY 1.17 + 1.18 — the load-time refusal we shipped is now a MUST NOT (2026-08-19)

`2d312f1`, answering arch `ROUTING-2026-08-19-i` (arch `3dd5800`) and `-19-j` (`05faaa5`). **Both
are addressed to `entity-core-go` and §7 of the first says explicitly "not yours to chase."** Read
under D21 anyway, because each moves a MUST — and one of them moves a MUST we had implemented the
previous day. This is D21 doing the exact job AP12 earned it for, on the session-start step rather
than by feature work tripping over it.

**§11.1's *"refused or normalized at load"* is WITHDRAWN, and we had shipped the refusal.** §4.1
step 2 binds *a distribution shipping* a config and *a peer storing* one. §6a.9.2's store-first rule
puts an operator's deliberate edit and a distribution's seed **in one entity at one path**, so a
loading resolver cannot observe which act produced the bytes; it necessarily over-enforces, and the
over-enforcement **deleted the operator `MAY` the same paragraph grants**. Replacement, all three
halves a MUST at 1.17: **surface it, never normalize it, never refuse to start.**

Ours refused, and `shellboot` turned that into a fatal — **`entity-shell` would not start** on a
name-disclosing stored config. Fixed:

- `EnsureResolverConfig` records a disclosure condition as a diagnostic and returns success;
  `AppPeer.ResolverConfigDiagnostic` is where the surfacing lands, and `shellboot` prints it to
  stderr at boot. `name config` already printed it beside the config and needed no change.
- **Every other error still returns.** `IsNameDisclosureRefusal` is the classifier that makes the
  split expressible: a policy decision an operator is allowed to have made, versus a config the
  peer cannot read. A reversal like this overshoots in exactly one direction and that is the fence.
- Nothing normalizes — the operator's bytes survive the boot verbatim, which 1.17 generalizes into
  its own MUST (*a resolver MUST NOT rewrite stored configuration as a side effect of reading it*).

**Verified rather than assumed on the rest of both packets:**

- **§4.1 step 2 is KIND-SCOPED** (ruled 1.17, re-derived 1.18): validity is a function of
  `name_format_dispatch` alone, never of `resolver_chain`. **Ours already was**, by construction —
  door 1 never read the chain. Pinned anyway as `REG-DISPATCH-CONFIG-REFUSED-1` row (b): a broad
  rule naming `did-web` with **no** `did-web` chain entry, which a chain-scoped implementation
  accepts. AP19 is why — green under both readings was our only evidence.
- **arch withdrew its own published rationale for that ruling at 1.18** (the "silent arming"
  argument, refuted by §4.1's own whole-config sentence) and kept the conclusion on monotonicity.
  Our doc comment carries the replacement reason and says the pin asserts the behaviour, not the
  rationale — a correct conclusion resting on a withdrawn reason is what gets cited later.
- **§4.3's `set-resolver-config` / `get-resolver-config` `[v1.18]` are new and unimplemented in
  every seat, core-go included.** That is where the operator override
  (`acknowledge_name_disclosure`) lives. **We do not invent a local acknowledgement parameter** —
  the spec says in as many words it MUST NOT become a field of the entity, because a field is
  written by whoever writes the bytes and would move a content-addressed type's hash to carry an
  unsecurable claim. The path an operator has today is the one §4.3 keeps open: a direct tree
  write, which carries no acknowledgement and is therefore surfaced at every load. **The fix above
  is what makes that path work** — before it, the peer refused to boot instead.
- `REG-NAME-CONSTRAINTS-GRAMMAR-1`, the `hints.max_ttl` ceiling, R-4, R-13: all confirmed to land
  outside our tree.

Three pins, all mutation-checked against the pre-fix code — including
`TestBootstrap_ANameDisclosingConfigDoesNotStopTheBoot`, two `Bootstrap`s over one SQLite file
under one keypair, because **the SDK call was correct in isolation and the fatal lived in the
caller** (D22).

**Ratchet: AP25 — an enforcement point that cannot observe the rule's subject.** *Before
implementing a rule, ask whether the point you are implementing it at can see the thing the rule
is about.* The tell is a check whose subject names an actor or an act — *did a **distribution**
ship this?*, *was this **latched** or re-read?* — that the data at that point does not carry.
**Deliberately not promoted:** this shape has bitten once. AP19 is a *test* that could not tell two
rules apart and AP22 is a guard keyed on familiarity; neither is this. Charter is D1–D22 /
**AP1–AP25**.

### 7. The name arc reaches a user, and the handler browser closes the parity gap (2026-08-19)

`ddde4c7` + `27874ad` + `6204630`. Two gaps that were both *reachability*, not features.

**EXTENSION-REGISTRY §11.2 lists "UI / CLI surface for local-name bind / unbind / list" as a
SHOULD.** We had none, and could not have had one: `shellboot` never set `Extensions.Registry`, so
**no shipped binary carried the handler a verb would dispatch to.** ResolveName, BindLocalName, the
resolver-config validator, the v1.13 adoption, the 1.14 re-key, the hints pin — all reachable only
from unit tests. AP21's shape again: green at every unit boundary, broken at the one seam no test
crossed.

**The default was guarding a cost that is not there.** `app.go` justified registry-default-OFF with
a claim about the sibling — local-name default-grant caps "re-minted on every bootstrap", linear
growth. Priced against the substrate (D20): **zero marginal per-restart cost on both counters**;
the whole cost is **+8 paths / +8 entities, once**. The growth originally seen is the +2
entities/restart a **registry-less** peer pays too (core-go's, same family as the waived
identity-rebootstrap leak). `entitysdk/registry_bootstrap_cost_test.go` asserts the differential,
never an absolute.

Landed: the three missing local-name SDK ops (`ListLocalNames`, `UnbindLocalName`,
`UpdateLocalNameTransports`, plus `WithNotes`) — the kernel declared all four since the handler
landed and we had wrapped one; registry ON by default in `shellboot` with `DisableRegistry` /
`-disable-registry` to opt out; §4.1a's default resolver-config shipped via `EnsureResolverConfig`
(a config failing the §4.1 step 2 MUST is fatal at boot, not silently resolved through); and the
`name` verb — `ls` / `resolve` / `bind` / `unbind` / `config`, with `@alias` targets and failures
that name the **rung** they stopped at.

**Driven through `bin/entity-shell` across separate processes**, not only in tests. That found a
pre-existing property worth knowing: **without `-identity` the peer-id is regenerated per
invocation**, so each process writes a different namespace of the same SQLite DB and nothing
appears to persist. Affects every persistent surface, not just names. Under `-identity` the whole
cycle round-trips.

**The handler browser** (`27874ad`) closes the last console→Avalonia parity gap.
`wb.HandlerBrowserModel` was complete and renderer-neutral all along; only tview drove it. The
bridge is handle lifecycle + a JSON projection, the panel is controls. A real-X11 driver
(`make smoke-xvfb-handlers`) found a crash headless was structurally blind to — see AP24. 21
handlers walked, 53 output rows, exit 0; `system/registry` and `system/registry/local-name` appear
in that walk, which is the shellboot default confirmed live in the GUI rather than argued from
source.

**Ratchet: AP23 + AP24.** *A measurement with no control arm* (the probe that manufactured a
Δ370/restart leak in both arms), and *a test that asserts on the first item cannot see a bug that
needs a second one* (the headless suite that never changed the selection). Charter is now
D1–D22 / **AP1–AP24**.

Also this session: **60 files of gofmt drift** cleared in its own commit (`6204630`) — `make lint`
is `go vet` only and has never gated formatting.

**Owed, not done:** `UpdateLocalNameTransports` has no verb, because a binding's transports are
hash references to transport-profile entities and no surface hands a user one. A `-transports`
flag that can only take a hash nobody can obtain is worse than none. Wants the profile-hash story
first.

### 8. The last renderer gap closes, and the front door gets documented (2026-08-20)

Three things, one session, all follow-ons from the bearings audit.

**`dev` is pushed.** 14 commits to `origin/dev`. The four review packets that existed only
locally — two of them naming live core-go bugs (`CORE-GO-LAST-BURST-WRITE-LOSS`,
`CORE-GO-SUBSCRIPTION-DELIVERY-RING`) — are readable by their addressees now. That was the
cheapest item on the list and it had been sitting.

**`PeerLiveness` reaches a user.** The audit's one remaining renderer gap: the model was
tested, the bridge exported it, and **no C# file referenced the export**, while
`PeerConnectionsPanel` rendered `ConnectionsOpen` — the connection-pool snapshot the liveness
model exists to replace. The GUI showed a strictly weaker answer with the correct one one
unused export away.

What landed is **not** the one-shot export wired to a button. That shape was wrong twice over:
it re-paid the model's O(N) seed on every call, and having no wake it could never deliver the
one transition the pool cannot express — **a demotion to `suspect` writes `system/peer/status`
and nothing else**, so a panel refreshing on connection events would never see it. So
`avalonia/bridge/liveness.go` is the standard handle lifecycle (`LivenessOpen` /
`RegisterWake` / `Render` / `Close`, cascade-on-peer-destroy), holding one long-lived
prefix-subscribed model; the one-shot `PeerLiveness` export is gone.

The panel now shows **both** surfaces, labelled apart: *"Liveness (tree)"* with per-status
colour (`suspect` gets its own — collapsing it into green or grey discards the whole reason
the section exists) and the three counts, above *"Connections (local pool)"* which keeps the
dial/drop buttons. `last_seen` is still not exposed anywhere: transition-written (§5.4.1),
so rendering it as freshness would invent a contract the protocol declines to offer. The
empty state says **"No lifecycle transitions recorded"**, not "nothing connected" — absence
means no transition was ever written, and a test asserts that wording.

**Avalonia 60/60 headless** (was 57), plus a new tier-3 gate: `make -C avalonia
smoke-xvfb-connections` drives the panel under real X11 and churns both renders — the
clear-an-ObservableCollection-under-selection shape that killed the handler browser in X11 and
nowhere else (AP24). Its log states what it cannot prove: one peer writes no transitions, so
row *content* is not under test.

**The front door is documented and has a fast rung.** `make gui` rebuilds the image before
launching, which is right after a code change and wrong when you just want to look at the app —
and there was no other verb, so "start the thing I built five minutes ago" cost a full podman
build. Added **`make gui-run`** (launch, rebuild nothing) and `ARGS` passthrough on both
(`make gui-run ARGS="--identity me --storage sqlite"` — the .NET frontend takes double-dash
flags, not Go's `flag` spelling; **with no flags the GUI is an ephemeral in-memory peer that
loses everything on exit**). `avalonia/README.md` was rewritten: it had described the renderer
as a three-spike POC, pointed at a `PHASE-I-DESKTOP-RENDERER-PLAN.md` that does not exist, and
listed 7 bridge symbols when there are 118 — the same *doc-points-at-a-missing-file* failure
the audit found in `AGENTS.md`, on the one page a newcomer reads first.

**Ratchet: D23** — *a model with no shipped surface is not shipped.* Third instance of one
shape (name arc, handler browser, this), two of the three found by audit because no test
crosses "can a user reach this". Enforcement is real, not aspirational: **`make reachability`**
runs both sweeps (bridge exports no C# consumes; workbench models no renderer or verb drives)
and exits non-zero on the first orphan. Both are empty as of this commit. Charter is now
**D1–D23** / AP1–AP26.

**Owed:** `make reachability` is not in `check` yet — one clean sweep is not enough evidence
that it will not false-positive on a legitimately internal model. Join it after a few sessions.

### 9. The D21 sweep, run late: eight unread packets, and one of them is a hold on us (2026-08-20)

`grep -ril 'workbench-go' ../entity-system-architecture/docs/status/` returned **eight documents
newer than our last-read marker** — the entire 2026-08-20 set plus `ROUTING-2026-08-19-k` and
`STATUS-2026-08-19`. All read this session. This is the discipline working late rather than not at
all, and it is worth noting that the previous session's audit did not run it either.

**Nothing in them assigns us work.** Three findings, in order of how much they change:

**1. T5 · COMPUTE is `HELD BY DECISION` — and arch's own §4 says the hold was never delivered to its
driver. We are the driver.** `PROPOSAL-COMPUTE-COLLECTION-PRIMITIVES` (the `concat` family) is
**parked**, not queued. We had it recorded here as *"gated on arch"*, which reads as a queue
position; a decision is a different thing and we were carrying the wrong one. Arch calls it *"the
highest-value unaddressed item on the board"* and says it needs an **operator call**: does compute
reactivate this cycle, or does the hold stand and get told to its driver? Five of the 39 active
proposals sit downstream of it.
**Why it lands here and not only on the compute tier:** the hold is upstream of the HUD text port →
which is upstream of retiring the legacy panels → which is why `dev` cannot cleanly release (see the
guardrail). Routed with the cost measured, and with the uncertainty stated — the HUD may be
expressible on today's `map`/`fold`/arith vocabulary without the parked primitives, and we have not
run that spike: `reviews/COMPUTE-HOLD-IMPACT-2026-08-20.md`.

**2. R-10 is arch's next item, and their board lists us as blocked behind it. We are not.**
`STATUS-2026-08-20-b` §3 and their handoff both name `entity-workbench-go` as *"blocked on it
today"* and put the fold first *"before more design"*. We built against the ruling and shipped;
the fold's absence holds no code here. Corrected in the reply so the fold gets sequenced on R-25's
need rather than on a seat that is not actually waiting.

**3. The transport-set round opened, and §2.3 asks every seat to grep its tree.**
`ROUTING-2026-08-20-d` — `PROPOSAL-PEER-TRANSPORT-SET` fourth pass, answering R-22 (*a consumer
holding only a `peer_id` cannot learn where that peer is*). Not ruled, no work assigned; arch wants
implementation evidence before ruling. We are the seat shipping **both ends** of the profile path,
so we answered with measurements rather than opinion:
`reviews/TRANSPORT-SET-IMPLEMENTATION-EVIDENCE-2026-08-20.md`. The three findings worth repeating
here: `transport.set|transport_set|TransportSet` is **zero occurrences** in our tree (nothing to
collide with); our three self-published profiles are **coexisting singletons, not a set**, and our
one signed aggregate (the published root) is not over profiles; and on R7's confirm-before-publish
`SHOULD`, **our missing half is not the confirmation, it is the expiry** — we refuse structurally
undialable addresses at the publish site with no network, but a profile has no TTL, so a rule that
binds only the publish instant buys less than it looks like.

**Three packets went out on the strength of this sweep:** the transport-set evidence, the
compute-hold impact, and — separately, closing share-arc step 6 — the
`APP-CONVENTION-CHAT` consumer review. All three carry measurements from this tree rather than
positions; none of them asks for a ruling.

**Also noted, no action:** arch records that our v1.17 refusal-at-load crash independently validated
their withdrawal (`ROUTING-2026-08-20-a` §6), and corrects the record that our posture matching
browser-rust's on the signed-root path is **cohort-consistency, not convergence**
(`ROUTING-2026-08-20-b` §3) — the same distinction we owe in the other direction.

### 10. The console/TUI review, asked for and run: it is not behind — it is bounded (2026-08-20)

Operator asked whether the TUI has fallen behind and whether a catch-up is owed. **Measured, not
estimated** — every `workbench/*_model.go` against both renderers:

| Model | console | Avalonia |
|---|---|---|
| detail · handler · log · markdown_files · markdown_view · peer_info · query · site · tree | **yes** (9) | yes |
| `peer_connections` | no | yes |
| `peer_liveness` | no | yes (new today) |

**Nine of eleven, and the two absences are the policy line, not drift.** Both missing models are
**multi-peer connectivity** surfaces, and console is **frozen and single-peer by policy**
(`AGENTS.md`: multiple renderers are a discipline enforcer, not a parity obligation). A single-peer
TUI has no connections list to render and no second peer to hold a lifecycle transition for. The
compute/program panels are absent for the same reason — that track is Avalonia's.

**So there is no catch-up debt**, and the D18 compile gate that keeps it honest is green:
`make build` produces `entity-shell` + `entity-console` + the three corridor binaries at `1398139`.
The thing to watch is not console falling behind; it is a *model* change that breaks console, which
is the signal that the abstraction was wrong — that is what the gate is for.

**If console ever goes multi-peer**, `peer_liveness` is the one worth taking first: it is a flat
sorted list with three states, which is the cheapest possible tview surface, and the CLI already has
the same answer under `peer status`.

### 11. Compute is sequenced, not stalled — and the cross-impl program gate is measured clean (2026-08-20)

Arch's `ROUTING-2026-08-20-e` read at arch `8dd5689`, tree green at `2d79fa2`. **The compute
disposition arrived, and it closes the item §9 opened.**

**The decision.** T5 compute is **deferred by operator decision and resumes after the release** — not
doubted, not dropped. Two stated reasons: the reachability stack (registry / routing / relay /
network / transport) is **cross-peer observable** and therefore the part that cannot be fixed later,
while compute's outstanding refinements are mostly *within* a peer; and compute is opt-out in a way
the network layer is not (an app that never touches `system/compute` is a complete app; one that
cannot reach a peer is not). **The five levers stay arch's and stay on the board.** What changes here
is only the reading: they were **paused**, not unanswered, and §1's ledger now says so.

**The two packets crossed.** `-e` was written against our tree at `2f285f7` — *before*
`COMPUTE-HOLD-IMPACT-2026-08-20.md` landed (`42bd8c1`, corrected `f887f3d`) — and arch says outright
it is not claiming to know our current state. Both documents reached the same answer independently:
the hold costs this seat nothing today, and **the defect was the delivery, not the decision**.

**Lever 1 re-measured, since arch asked to be told if our tree moved.** It has not moved on compute:
`programs/` carries no functional change since `968190b` (2026-07-27) — only `6204630` (gofmt) and
`a5752e2` (the frozen Life oracle, test-only). **`concat`/`range`/`group-by`/`assoc` are implemented
nowhere**, independently confirmed at core-go `0332c90`: the builtin set is still
`{arithmetic, compare, logic, field, construct, map, filter, fold, store}`. We do not ship builtins —
the seat is core-go's — so the fold (`EXTENSION-COMPUTE` 3.23 → **3.24**) changes nothing in this tree
today. Note `assoc` folded with **`assoc` MUST NOT be an implicit lowering target**, which binds
`compute_lower.go` if we ever lower a fold; and the naming gate re-spelled `group_by` → **`group-by`**
(kebab in a path segment). Arch's `WORKSTREAMS.md` lever-1 row cites this fold as "`EXTENSION-COMPUTE`
1.4→1.5" where the spec header says 3.23→3.24 — cosmetic, theirs, flagged not filed.

**The cross-impl program gate: measured, not assumed.** Operator asked whether `entity-browser-rust`
had fallen behind on compute. **It has not**, and this was settled by regenerating rather than reading:

- Their host is real and Mount-only — `src/program_host/` (2916 lines: bundle, controls, descriptor,
  host, input, sexpr, shapes) with a DOM/SVG `display-list` driver that fills **closed quads**, i.e.
  our de-facto ABI. Their 07-19 review's **F2** (`display-list` arity — spec says variable polylines,
  we shipped fixed quads) is therefore resolved *by adoption* and still **unruled**; **F1** they closed
  with the fetch-by-hash → materialize → eval-by-path sequence they proposed.
- Their fixtures are generated from **our** tree: `tools/program-dump` (theirs, `make
  program-fixtures`) runs the unchanged `programs` authoring through a `replace` onto this repo.
- **We re-ran it against `2d79fa2` into a scratch dir** (their tree untouched, verified clean before
  and after) and diffed: **the per-tick oracle is byte-identical for all three programs** —
  life 12/12, snake 12/12, asteroids 16/16 ticks. Entity counts match exactly (130 / 191 / 477).

**The only diffs are the dump tool's per-run random `origin_peer`**, which re-keys the handful of
entities whose IR embeds a peer-qualified path (4 same-path hash changes + 3–6 content-addressed
`_expr/{hash}` renames per program). So a regeneration **always** produces a dirty `git diff`, which
means "are these fixtures stale?" cannot be answered by looking — it has to be answered by comparing
oracles, as here. **Worth routing to browser-rust as a suggestion** (seed the dump keypair and the
fixture regen becomes reproducible), not a defect: their build-time `all_embedded_entities_hash_verify`
+ `corrupted_entity_is_refused` + `oracle_tests` already make a stale fixture a build failure on the
Rust side. Their fixtures were last regenerated at their `98671db` (2026-07-28) — the day after our
last functional `programs/` commit, which is why they are current.

**One thing we own and got wrong.** `PROPOSAL-GENERIC-HOST-POINTER-INPUT-DEVICE-2026-07-27.md` is
addressed *"To: arch + entity-browser-rust"* and this file has recorded it as **"blocked on arch"**
for 24 days. **It never left this repo.** Exhaustive search across both sibling trees — by filename
and by five distinctive phrases (`third input device`, `shapeDrivers`, `life-edit`,
`AuthorLifeInteractive`, `click a cell`) — returns **zero** hits, and arch's `WORKSTREAMS.md` generic-
host block lists the five compute levers with no pointer item. `pointer` is named in
`PROPOSAL-APP-CONVENTION-COMPUTE-PROGRAM` §-taxonomy as a held-state device, but that proposal is
DRAFT since 07-13 and carries no ABI for it. **This is AP12's mirror image** — that one was a routed
packet nobody opened; this is an unrouted packet we recorded as routed, and the failure mode is worse
because the ledger reads *waiting on them*. Not urgent (it is a post-release host gap, and the d-pad
cursor in `programs/life_edit.go` is the honest maximum on the two devices that exist), but the
delivery is owed and costs nothing — the packet is already written. **AP28**, with the outbound
sweep as its enforcement point (`AGENTS.md`, beside the inbound one).

**And the sweep's own first run corrected how it should be written.** Grepping siblings for our
packet *filenames* flags **twelve of the packets in `reviews/`** — and **eleven of the twelve had
plainly landed**: AE-5 is folded into `EXTENSION-COMPUTE` §11, `EXTENSION-DISCOVERY` §-mDNS names
this repo, four `WORKSTREAMS` rows carry our transport findings, and R-10's fold carries all four of
our rendezvous items. **Siblings cite our commits and our claims, never our file paths**, so the
filename test fires on almost everything and means almost nothing. The **subject** test — grep their
board and specs for what the packet is *about* — fired exactly once, on the pointer proposal. That is
the version that went into `AGENTS.md`; a gate that flags twelve to catch one gets ignored by the
third session that runs it.

**Tree state at the time of this entry:** `make test-each` **all ten suites green to completion**
(sdk 199s · shellcmd 287s · programs 144s · the rest under 15s), `make lint` clean, `gofmt -l` empty,
`make reachability` clean, working tree clean at `2d79fa2`.

### 12. The cross-impl consume leg — BUILT, RUN LIVE, and shipped to a UI (2026-08-20)

**`entity-core-go` handed us WS-A and it is done.** Their
`HANDOFF-2026-08-20-workbench-go-federation-consume-leg.md` asked for the axis
`entity-browser-rust` structurally cannot cover: **a reader in a different language than the
emitter**, over a live host boundary. `make crossimpl-go` is green.

Result packet, with everything below in full:
**`docs/architecture/reviews/CROSSIMPL-CONSUME-LEG-RESULT-2026-08-20.md`**.

**The operation that was missing, stated once so nobody re-derives it.** We had both halves and
neither walked the trie: `entitysdk.ReadPublishedRoot` verified a signature but over the **local
store**; `fetch.Fetch` crossed a wire but resolved by the **advertised leaf path**. Joined, the
chain is manifest → signature → **CHAMP trie walk from the signed root** → leaves, and **only the
walk can tell a complete origin from a withholding one** — every other step is satisfiable by an
origin serving a correctly-signed root that commits to nothing. That is not hypothetical: core-go
shipped exactly that for a week (a `0xC0C1C2…` literal root, fixed at their `dabd076`) and every
per-leaf consumer in this cohort reported green against it.

| measured | result |
|---|---|
| **live**, core-go's publisher on a podman bridge, our consumer in a second container | signature VERIFIED (ed25519, key from their peer-id) · 3 keys · **3/3 reconciled** · absent control fired |
| **offline**, browser-rust's frozen emission (their `fbc2c5c`) | signature VERIFIED · 15 keys · **15/15 reconciled** |
| **our own** publisher, 64 keys, 5 CHAMP nodes | exact enumeration + three mutation controls |

**Reconciliation is ours and nobody else in the cohort does it.** Every committed key is resolved
**twice** — through the signed trie and through the publisher's own advertised tree-leaf URL — and
the two must agree. *An origin that answers differently on the two paths is serving two trees and
only one of them is signed.* Green on all three emissions.

**What ships, in one line each:**
- `entitysdk/publishedroot/` — the seven published-root gates, extracted so the store-side reader
  and the wire consumer apply the **same** checks. Dependency-light so `entity-fetch` links no peer.
- `fetch/consume.go` — `Consumer`, the **strict** walk, `PinnedLayout`, `AbsolutePrefix`, the
  reconcile pass, typed `ErrIncompleteWalk` / `ErrAbsent` / `ErrEmptyEnumeration` / `ErrContractMismatch`.
- `scripts/crossimpl-go.sh` + `make crossimpl-go` — outside `test-native` on purpose (podman + a
  sibling checkout).
- **Three operator surfaces (D23):** `entity-fetch -verify [-json]`, `entity-shell`'s **`site
  verify`**, and the Avalonia **Publisher Verify** panel over `workbench.ConsumeModel`.

**The UI is deliberately the opposite of browser-rust's.** Their Site Browser renders the *pages* —
the reader's surface. Ours renders the **chain**: seven steps, each with its verdict *and what a
green verdict on that step actually proves*, because five of the seven are true of an origin that is
lying. A UI that collapses this into one tick teaches an operator that "verified" is one fact. It is
seven, and one of them (`published_at`) is a **moment** rather than a state — which is what the
freshness line under a green verdict says, on every surface, and never omits.
**That contrast is the UX research, not a duplication of theirs.**

**Two things building it found, both routed:**
- **AP29 — the handoff's own advice would have produced a blind consumer.** It said to walk with
  `core/tree.CollectAllBindings`. That helper (and `CollectNodeClosure`) is documented best-effort —
  *"Missing nodes are skipped"* — which over an HTTP origin turns a withholding origin into a
  **smaller site**, silently. Ours re-implements the traversal over their `core/types` and fails
  closed. Mutation-checked: `publish`'s `TestConsumeWithholdingInteriorNode` removes one **interior**
  node and requires `tree/incomplete-walk`; a best-effort walk passes it with fewer keys and no error.
- **AP30 — §3.3's prefix, and we were right by luck.** The wire field is the **configured** prefix
  and all three admissible shapes are live at once (browser-rust `/{peer}/`, ours `docs/`, core-go
  `system/`). Concatenating it verbatim is correct for two of the three, for two different reasons,
  neither of them the rule. **A mis-joined path is a 404, and at a consumer a 404 is
  indistinguishable from a withholding origin** — the bug reports as the other side's defect. Fixed
  as `fetch.AbsolutePrefix`, pinned across all three rows, and confirmed live against core-go's
  `system/` shape.

**Charter is now D1–D23 / AP1–AP30.** Neither is promoted; each has bitten once.

**Not claimed:** two physical machines, the public internet, TLS, a CDN, NAT. And **B5**
(name → binding → transports → fetch) is not in this leg — core-go's origin publishes no registry
or bindings, exactly as `ROUTING-2026-08-20-m` §3 says.

### 12a. What `ROUTING-2026-08-20-l` settled (2026-08-20)

- **The pointer proposal was delivered by pushing `dev`** and arch read it at `98ff6de`. It is
  `COHORT-OPEN-ITEMS` §1d row **W-1, owner arch, blocked on us: nothing**. Not ruled this session
  **by stated decision** (the T5/T4 seam; the design axis stays deferred) — which is the
  confirmable kind of deferral, and the whole point of AP28.
- **Arch adopted AP28 verbatim** and filed their half: for 24 days nobody noticed a driver had gone
  silent, because *a blocked-on-us row and a paused-by-us row look identical from the outside*.
- **R-10 is folded — `EXTENSION-DISCOVERY` 1.0 → 1.1**, with all four of our rendezvous build
  findings landed in full. `entitysdk/rendezvous.go` now sits on landed spec; the fold-ask closes.
- **R-9 (`hints` round-trip) — answered here, both halves.** Arch's worry was an SDK that rebuilds
  `resolver_chain` entries field-by-field and drops the new `max_ttl` / `neg_ttl` keys, disarming a
  security control with every test green. **Refuted for this seat:** `ResolverConfig()` decodes
  straight into `types.ResolverChainEntry` (whose `Hints` is `map[string]cbor.RawMessage`,
  `registry_ext.go:303`) and `InstallResolverConfig` encodes the caller's struct — nothing
  reconstructs an entry. **Their other half confirmed:** we author no ceiling and consume none.
- **Compute, corrected:** `concat` / `range` / `group-by` / `assoc` **are** implemented in core-go
  (`ext/compute/builtins_v324.go`, `eb80750`, corrected at v3.25 `4cd1ee8`). §11's *"implemented
  nowhere, at core-go `0332c90`"* was measured **before** that commit — `0332c90` is an ancestor of
  `eb80750`. **Lever 1 is no longer waiting on anybody.** Compute stays post-release by operator
  decision; what changed is the *state of the lever*, and two consequences are now ledger rows:
  `programs/` still routes around `concat` in four places, and **Axis-1 — our conformance-admitted
  alternate engine — implements none of the four**, against a corpus that has grown to 350 vectors.

### 13. The naming hop — BUILT both directions, and four cross-impl findings (2026-08-21)

**The journey is closed.** Consume: pin a name authority → **walk** it for its names (§6a.3a)
→ resolve one with every §6a.4 check → follow the binding to its publisher → verify *that*
peer's signed root → walk it → render the page. Serve: `registry issue` mints and signs a
binding into this peer's tree, and publishing `system/` emits it as a static registry another
peer can pin and browse (§6a.8 / §7.4). Ran end to end by hand, both directions.

**Three surfaces (D23):** `entity-shell`'s `registry` / `browse` / `open`, the Avalonia
**Browser** panel (page in the middle, the ten-step trust chain in the right column), and
`entity-fetch -registry -names / -name`.

**D20 paid for itself before a line was written.** `entity-core-go`'s
`ext/registry/peerissued` already has all of §6a.4 including the association check plus an
`HTTPPollReader`, so the gap was **wiring, not a backend** — `entitysdk/registry_pin.go`
implements no spec logic. What the kernel lacks is the §6a.3a **enumeration**, which arch
(`ROUTING-2026-08-21-b` §3) records as *the one registry surface in the corpus with a producer
and zero consumers*; ours is the first.

**Routed:** `reviews/REGISTRY-BINDING-TRANSPORTS-DIVERGENCE-2026-08-21.md` — four findings.

| § | finding | who |
|---|---|---|
| §0 | `transports` has two live readings (`Vec<Value>` inline vs `[]hash.Hash`); **core-go's backend cannot decode any binding in the cohort's only live federation** | arch to rule; core-go or core-rust changes |
| §6 | **§6a.3a's recommended publishing prefix is unimplementable** — a binding's signature is at `system/signature/{hex}`, outside every `system/registry/…` prefix, so a conforming registry enumerates fine and resolves nothing | arch |
| §4 | core-go's `httplive.Outbound` implements one branch of the §6.5.3.1 tree-URL join, so fed browser-rust's own profile it builds `/{peer}/{peer}/…` | arch's existing AP30 ruling |
| §7 | the `.list` artifact is a `system/tree/listing` **entity** per spec; browser-rust emits newline text | browser-rust |

Plus one declinable ask to core-go (§5): **export `normalizeName`** — a Layer-2
canonicalization we had to transcribe.

**Held together by a differential test, not by intent.** `workbench/registry_differential_test.go`
runs core-go's backend and ours over the same frozen bytes and requires the same verdict; it
currently **records the divergence as measured state** and fails if it widens *or* silently
closes.

**Frozen fixture:** `fetch/testdata/crossimpl-rust-federation/` — browser-rust's `make
federation` emission (a registry signing four names + the four domains, 784K). It exercises
both layout modes in one run: the registry serves no `transport-profile` (conformant, §6.5.4,
R-28) so it is pinned; the domains advertise one so they are discovered.

**Three anti-patterns earned.** AP31 (an async cgo export read a C string after the P/Invoke
freed it — no crash, it reads as `""`), AP32 (a derived UI property used as a completion
signal: four headless tests green while measuring nothing), and **AP33** — two pre-existing
`put` bugs found by seeding a site by hand for the demo, not by any test.

## Open bugs

- **~~Managed stack overflow on window minimize~~ — RESOLVED 2026-08-21** (it was never
  minimize; see the dated block at the end of this entry). Kept in full because the *way* it
  stayed open for a month is the lesson, and D24/AP34/AP35 were earned on it. A tight
  alternating A↔B JIT recursion into the .NET guard page, **zero GPU/GL modules in the
  faulting thread** — so it is ours, not the driver. Full forensics:
  `docs/status/HANDOFF-2026-07-18-avalonia-64x64-segfault.md`.
  **New this session:** `PanelStack`'s class doc records **two earlier SIGSEGVs with the
  identical signature** (PIDs 4033208, 4035311) from a GridSplitter drag driving a
  star-weighted row to **zero size** — an upstream Avalonia layout-engine recursion, fixed
  structurally with `RowDefinition.MinHeight`/`MaxHeight` pins. Minimize is the *same
  zero-size condition arriving from above* (the window collapses the ScrollViewer viewport),
  which those row pins do not cover. That is the working hypothesis.
  **The headless repro did NOT fire** (all three tests pass): 25× collapse-to-0×0-and-back,
  25× `WindowState.Minimized`/restore, and 40× collapsing the 64×64 Life panel with a live
  repaint stream in flight are all survivable under headless Skia. That is a *negative*
  result kept as a regression fence — it narrows the search without closing it, since
  headless does not run the X11 backend at all, which is where both documented predecessors
  lived. **Both original leads are now closed:** the other one — symbolizing the core dump —
  died with rotation; the `entity-avalonia` dumps are gone from
  `/var/lib/systemd/coredump`.
  **2026-08-19 — the X11 rung was built and it is a third negative, but a much sharper one.**
  `make smoke-xvfb-window` drives the window geometry itself under Xvfb while a program
  paints, which is the rung between headless (no X11 at all) and the desktop (crashes,
  dump rotated). Two runs, both survived, both with the transitions **verified to have
  actually taken effect** rather than assumed:
  - `MODE=resize` (no WM): 39/40 transitions, ClientSize alternating 1400x900 ↔ 1x1 with the
    64×64 Life panel painting. Survived.
  - `MODE=minimize WM=1` (openbox on the virtual display): 39/40 transitions,
    `Normal ↔ Minimized`, real iconify. Survived.
  **And the second run damaged the working hypothesis.** The theory was that minimize
  collapses the ScrollViewer viewport to zero from above — but under openbox, `ClientSize`
  stayed `1280x900` across every minimize. Minimize did not collapse anything. So either the
  zero-size condition comes from the **compositor** specifically (mutter/kwin, not openbox),
  or the viewport-collapse theory is wrong and the fault is elsewhere in the iconify path.
  Next rung: a compositing WM in the harness, or a capture on the real desktop.
  **The instrumentation caught itself, which is the reason to trust it.** The first version
  read ClientSize back in the same tick it requested the change; an X11 geometry change lands
  asynchronously, so it reported *"zero transitions"* for a run whose own log showed the
  window collapsing twenty times. A gate that miscounts in the reassuring direction is worse
  than no gate. It now compares against the previous tick's observation and prints, for every
  run, how many transitions actually took effect — with an explicit "this run is NOT evidence"
  line when that count is zero.
  **2026-08-20 — a fourth negative, and an accounting of what is actually left.** Operator
  challenged whether this bug is still real or is being carried forward on old text. It is
  being carried, and here is exactly what stands behind it:
  - **The original evidence was real** — three core dumps (PIDs 3239906 / 3331097 / 3513953),
    analyzed, with the discriminator recorded as a count anyone could re-run
    (`coredumpctl info | grep -icE 'gallium|GLX_mesa|libGL\.|swrast'` → 21 for the GPU crash,
    **0** for this one). That is what makes it "ours, not the driver."
  - **No artifact remains.** `coredumpctl list | grep -c entity-avalonia` → **0** today. The
    dumps rotated out of `/var/lib/systemd/coredump`. Nothing is left to re-examine.
  - **Four repro attempts, four negatives**, the newest on today's binary:
    `make -C avalonia smoke-xvfb-window MODE=both` — 60 iterations, **59 transitions verified
    to have taken effect**, collapse to 1×1 and restore with paint in flight, exit 0.
  - **The stress coverage is real and specific**, not a claim: 25× width→0, 25×
    `Minimized`/restore, 40× collapse-while-ticking (`PanelStackZeroCollapseTests`), plus the
    400× real-Skia rasterize in `ProgramPanelStressTests`. All in the 60/60 headless run.
  **Disposition, so this stops being carried by default:** it stays open only as *last seen
  2026-07-18, unreproduced since, no artifact*. The next step is an operator capture on the
  real desktop (compositing WM — mutter/kwin, which is the one condition the harness has never
  had). **If the next occurrence produces no dump, close it as unreproducible** rather than
  keeping a month-old symptom on an open-bug list, which is how a stale entry starts steering
  work it can no longer justify.

  ### 2026-08-21 — REPRODUCED, CHARACTERIZED, FIXED. Closing.

  The operator capture arrived (two desktop SIGSEGVs, PIDs 619966 and 1462191, both preserved
  before rotation this time). It was **not** the minimize path, and the harness never had a
  chance of finding it, for a reason worth keeping:

  **Root cause: the alternate signal stack overflows.** The PAL gives the UI thread a
  **16384-byte** alternate signal stack (measured at startup, not inferred:
  `CrashDiagnostics.ReportAltStack`). Under real pointer input the handler chain on that stack
  exceeds 16 KB and the next `call` pushes its return address into the guard page. Caught live
  under gdb with `nopass`, twice, identical:

  ```
  Thread 1 received signal SIG34, Real-time event 34       <- runtime thread-suspend injection
  Thread 1 received signal SIGSEGV
    si_code = 2 (SEGV_ACCERR)   si_addr = rsp - 8   rip on a `call`
    rsp inside a PROT_NONE page; the mapping is the PAL altstack, not the managed stack
  ```

  **Every previous reading of this bug was taken from the wrong signal.** CoreCLR's handler
  cannot classify the fault, so it **re-raises** — and the re-raised signal is what lands in
  the coredump, carrying `si_code 128` (SI_KERNEL) and `si_addr 0`. That artefact is why the
  cores read as a null dereference and why the runtime never printed `Stack overflow.` and
  createdump never fired: by the time it faults there is no stack left to report on. Rule:
  **on a .NET Linux crash, check `si_code` before believing `si_addr`.**

  **Fix:** install a 1 MB alternate signal stack on the UI thread at startup, default on
  (`CrashDiagnostics.EnlargeAltStack`, `WB_ALTSTACK_BYTES=0` restores stock for re-measuring).
  A `PRIVATE|ANONYMOUS` mapping commits lazily, so the cost is the pages a handler actually
  touches, against an unrecoverable process kill.

  **Evidence — A/B, same binary, same seeds, only the env differs:**

  | arm | crashes |
  |---|---|
  | control, stock 16 KB (`WB_ALTSTACK_BYTES=0`) | **6 / 8 seeds** |
  | 1 MB altstack (default) | **0 / 8 seeds** |

  Plus 5/5 → 0/5 on an earlier pass over the seeds that had failed 5/5.

  **Still open, narrowly:** *what* consumes more than 16 KB is not identified — nested signal
  delivery on the altstack is the leading candidate, and `GODEBUG=asyncpreemptoff=1` (5/5 still
  crashed), `DOTNET_gcConcurrent=0` and `DOTNET_TieredCompilation=0` are all ruled out as the
  trigger. The enlargement removes the crash without explaining the appetite. Also **only the
  UI thread is protected** — every other managed thread still runs the stock 16 KB, and a
  crash on one of those would look identical.

  **Why four harness attempts missed it, which is the part that generalizes:** every driver in
  this repo called the model method *under* the control
  (`SiteViewPanel.NavigateForTests`, `HandlerBrowserModel`, the window driver). None of them
  ever produced an X11 pointer event, so none could execute input dispatch, hit-testing or
  focus transfer — and this bug lives only there. `make -C avalonia smoke-xvfb-click` (real
  `xdotool` clicks, seeded and logged) reproduces in under 45 s; `make crash-hunt` sweeps
  seeds unattended and stops at the first hit with a replay command.
- **GPU-driver SIGSEGV** (distinct, older): the mesa hardware-GL path crashes under
  sustained compositor load. **The product call is made (2026-08-19): software Skia is the
  DEFAULT, hardware GL is opt-in via `WB_GPU_RENDER=1`.** Auto-detect was the other
  candidate and was rejected — detecting a bad driver from in-process means fingerprinting
  mesa versions, which is a guess that ages badly. The two outcomes are not symmetric: the
  cost of software render is frames per second on an app that is mostly static text and
  small grids, and the cost of the GPU path on an affected driver is a hard crash. A slow
  window beats a dead one. `WB_SOFTWARE_RENDER=1` is still honored (now a no-op), and the
  chosen mode is announced on stderr because it is the first thing a crash report needs and
  the last thing a reporter includes.
- **Asteroids "keys" report** — flagged by the operator, never reproduced. Key wiring was
  checked and is correct; suspected to be the GPU crash hit while interacting. A headless
  key-injection repro is still owed.

## Backlog

**Release / dependency cutover**
- **Module-path cutover.** Every `go.mod` requires the kernel by its vanity path
  (`go.entitychurch.org/entity-core-go/{core,ext}` @ v0.8.0) but resolves it through a
  local `replace` to the sibling `../../entity-core-go/{core,ext}` (offline; no network, no
  tag). When the vanity path is published + tagged, the cutover is **one line per module**:
  drop the `replace`, let `require … @v0.8.0` fetch. Then run the deferred no-siblings,
  clone-fresh `make build` to prove it.
- **`USAGE-PROTOTYPE-FILESYSTEM-SYNC.md` §10 points at a renderer that does not exist.** It
  sends the reader to the `canvas/` application for a graphical tree browser; canvas was
  removed. The section's substance still holds — there is no GUI surface tied to the mount
  prototype — but it names the wrong absent thing. Found 2026-08-30.
- **Path-syntax migration.** User-facing surfaces still use `alias:path`; the pinned
  substitution sigil is `@alias` (`:` is reserved for `<handler-path>:<op>`). Prefer
  `@alias` in new docs/examples now to minimize churn when the code change lands.
- **`entitysdk` spin-out.** Stewarded in-tree as the authoritative Go SDK.
  - **Known blocker, found 2026-07-22:** the compute probe cluster in `entitysdk`
    (~9k lines: `exp_compute_*`, `axis1_*`, `continuation_shard_test.go`) **cannot be
    lifted out** as-is. It is one densely interconnected web — a shared Life workload
    fixture, the `boundary` CBOR helper, the shard rigs — with no clean cut, and 5 of the 12
    files reach `export_test.go`'s deliberately test-only engine accessors, which are
    reachable only from *inside* `entitysdk`'s own directory. Moving them would force those
    accessors into the shipped API, which that file explicitly forbids (it exists so app
    code cannot bypass protocol-first access). Untangling it means extracting the Life
    workload fixture first; until then the probes stay put. Attempted and reverted this
    session — the `programs/` extraction is the part that *was* clean.
  - Relatedly: `axis1_{cost,equivalence,tick}_test.go` are arguably API pins for the shipped
    `entitysdk/axis1` subpackage rather than probes, and should travel with the SDK.

**Known waivers / kernel-blocked (not workbench bugs — sibling-impl rule)**
- **Identity-rebootstrap storage leak.** The bounded-rebootstrap pin in `entitysdk/` is
  `t.Skip`'d with its assertion intact. Re-applying an identity bundle leaks ~+1 path /
  +4 entities **per reload** (linear, unbounded). Root cause is the kernel's identity
  *ceremony* re-issuing the local-peer→controller cap + sibling signature with
  ceremony-time-varying material instead of reproducing prior content hashes. Delete the
  Skip to re-arm the regression the moment the kernel's re-apply is idempotent.
- **Subscription slow-consumer head-of-line block.** The producer queue is bounded with
  drop-on-full; the gap is a missing **per-delivery deadline** on the consumer-side
  synchronous `Deliver`, which pins one shard worker when a consumer stalls. Kernel-side fix.
- **Subscription delivery saturation.** 100% delivery below ~2K notifs/sec; a cliff to
  ~47–49% at 5K+/sec, dropped silently. Typical workbench heartbeat is far below saturation.
  Endorsed fix is **parallel delivery workers** — kernel-side after cross-impl alignment.
- **Revision auto-version is O(N)/Put.** Per-Put latency under auto-version grows linearly
  in the existing path count under the prefix, *regardless of trie shape*, because the trie
  is rebuilt from scratch on every Put. Fix is incremental update — a kernel/spec concern.
- **Content-store GC contract.** Path-overwrite accumulates orphaned content entities
  indefinitely; a naive "delete unreferenced + VACUUM" sweep is unsafe because hash
  references are encoded across many typed sites. Awaiting a cross-team reachability/GC
  contract built on the kernel's reverse-hash index; workbench ships nothing until it lands.

**Discovery follow-ups (post-"Nearby Peers" close)**
- N-panel × M-peer scan multiplier — one scan loop per discovery handle, no dedup by handle.
- TXT-pair parsing in the Avalonia bridge duplicates the kernel's private parser (drift risk).
- Scan interval is hardcoded ~5s; no "Scan now" affordance.
- No headless test exercises a *populated* nearby list (would need mDNS in the test container).

**UI / renderer**
- ~~**Handler-browser panel**~~ — **DONE 2026-08-19** (`27874ad`). The console→Avalonia parity
  gap is closed; `console` is no longer ahead on any surface (re-verified by sweep 2026-08-20 —
  console's `execute_console.go` IS its handler browser, and `HandlerBrowserPanel` matches it).
- ~~**`PeerLiveness` has a bridge export and no panel — the ONE remaining renderer gap.**~~ —
  **CLOSED by `8383326`, struck 2026-08-30.** `PeerConnectionsPanel` consumes
  `LivenessOpen`/`LivenessRender`/`LivenessRegisterWake`/`LivenessClose`, and `make reachability`
  reports clean. The row survived five days after its own fix because nothing re-reads a backlog
  against the tree; the sweep that would have caught it existed and was not run against this row.
  Kept below as the record of what the gap was, since D23 was earned on it.
- **(historical, as written)** Found by
  audit 2026-08-20, by two sweeps that each return exactly one name: bridge exports no C# consumes,
  and `workbench/*_model.go` files with no Avalonia panel. `workbench.PeerLivenessModel` is built
  and tested, `avalonia/bridge/main.go:492` exports `PeerLiveness`, and **no C# file references
  it** — `PeerConnectionsPanel` still reads `ConnectionsOpen`, the connection-pool snapshot the
  liveness model exists to replace. Not cosmetic: the pool snapshot **cannot express `suspect`**,
  cannot say why a peer went, and disagrees with the tree whenever a connection is evicted without
  a demotion, so the GUI shows a strictly weaker and occasionally wrong answer while the correct
  one sits one unused export away. The CLI already has it (`peer status`). **This is the third
  instance of one shape** — the name arc, the handler browser, this — where a renderer-neutral
  model was green and no shipped surface reached it; two of the three were found by audit rather
  than by a test, because no test crosses "is there a user-reachable path to this." A fourth earns
  a discipline.
- **Console multi-peer UX** (deferred): peer-picker modal, status bar, `peer create`/`destroy`.
- **Manifest-driven panel registration** (deferred from the multi-peer plan).
- Avalonia drives feature work and may outpace the frozen `console` renderer; console-parity
  is explicitly *not* an obligation.

**SDK ergonomics / compute**
- **SDK ergonomic helpers (compute "S4–S8").** Owed a research-first session.
- **Compute DSL parser.** Deferred; built *on top of* the S4–S8 helpers, only once an
  authoring workflow actually needs one.
- ~~**Wire a real consumer of the `resolve()` seam**~~ — **DONE 2026-08-19** (`ddde4c7`). The
  `name` verb is that consumer, and it needed the substrate turned on in `shellboot` before it
  could be one.

**Hardening / cleanup**
- Revision-recovery diagnostic: hub-spoke fetch-diff recovery with auto-version *off* logs
  an independent transport failure mode; captured as an observation, the test passes.
- Selection-state reader hardening: replace the silent legacy-tolerance path in
  `entitysdk/workspace_state.go` with log-on-violation or reject-on-decode.
- ~~**An ephemeral shell silently re-namespaces a persistent store.**~~ — **FIXED 2026-08-20**
  (`6c72dfa`). `-storage sqlite` with no `-identity` now uses the `default` identity, created on
  first use: a plain nameable one that shows up in `identity ls`, with the store at the
  GUIDE-PERSISTENCE §1.1 path derived from it. Create-if-absent and never overwrite (regenerating
  over an existing keypair would orphan everything under the old peer-id — the same
  re-namespacing, made permanent); the 409-exists race resolves to the winner's keypair rather
  than an error. **Memory storage keeps its ephemeral keypair, with a control arm pinning that** —
  nothing survives the process either way. The old `-storage sqlite` + no-path + no-identity
  refusal is dropped and its pin replaced rather than deleted: it guarded against an orphan store
  and missed the case where the *identity*, not the path, was unknown. Verified through
  `bin/entity-shell` across separate processes, which is the only place the bug existed.
- **Registry restart baseline (core-go, observed 2026-08-19).** A peer with NO registry extension
  and NO identity ceremony still accretes **+2 entities per restart** of the same SQLite DB
  (paths stay flat). Measured as the control arm in
  `entitysdk/registry_bootstrap_cost_test.go`. Same family as the waived identity-rebootstrap
  leak; not ours, and owed a routing packet to core-go.

## `master` — the guardrail is cleared, and here is what cleared it

**History, kept because the reasoning is the useful part.** This section used to read *"do not merge
`dev` to `master` yet"*, on the grounds that the legacy hard-coded panels were the oracle for
`TestMount_LifeMatchesHardCodedModel` and Avalonia registered both the legacy and the generic-host
copy of all three programs. Both halves are now gone, and neither was gone the way the guardrail
predicted:

1. **The oracle was re-pinned** (2026-08-20). `programs/oracle_vectors_test.go` freezes the twelve
   state hashes the two sides agreed on, so the mounted program is gated against a **recorded**
   reference rather than against live legacy code. Strictly stronger than the mutual comparison,
   which cannot see a kernel-side encoding change — both sides shift together and stay green.
   Verified to fail (one character flipped → red at tick 2). The mutual test stays while the legacy
   Go models do; it is no longer what blocks deleting them.
2. **The product regression that was the real blocker had already closed a month earlier, and
   nothing was tracking that.** The generic panel *can* show program-specific status — the `text`
   status port shipped 2026-07-27 (Life `POP nnnn`, Snake `LEN nnn`, Asteroids `SCORE nnnnn`),
   projected in the tree so every renderer shows the byte-identical line, and it needed no `concat`.
   We spent an hour on 2026-08-20 pricing this against the July review instead of against the tree —
   **D20 failing in its usual direction** — and the correction is in
   `reviews/COMPUTE-HOLD-IMPACT-2026-08-20.md` §2 plus an annotation on the July packet itself, so
   the next reader does not pay it again.

**The legacy trio is retired** (2026-08-20): three Avalonia panels, three bridge surfaces (23 of the
118 exports), three registry entries, three smoke targets and three smoke-driver modes — deleted.
`avalonia/bridge/program.go` + `ProgramPanel` drive every program from a descriptor, including the
input ports and the status readout. **The legacy Go models in `programs/` stay** as the mutual
test's oracle: they cost nothing, nothing else consumes them, and keeping a second independent
implementation of Life around is the cheapest oracle we will ever have.

What that removes from the shipped app: nothing a user can do. What it removes from the tree: the
duplicate path, which is what made `dev` a comparison surface instead of a release.

## Open — the compute/programs track (operator-named 2026-08-21, rewritten 2026-08-22)

**Where this arc actually stands, as of the release close-out.** Interactive Life is *working*
— the operator has driven it. Three defects were found and fixed across two sessions, and the
resolution of the original "isn't working" report is recorded here because the two hypotheses it
was split into are now both answered:

| the 2026-08-21 hypothesis | answer |
|---|---|
| **(b)** the d-pad/action bits are not reaching the program | **YES, and twice over** — AP36 (the host sampled its ports only at tick time) and AP37 (`btn.PointerPressed +=` on an Avalonia `Button` never runs). Both fixed, both gated. |
| **(a)** the operator expected to *click cells* | **Still true, still unimplemented** — see the pointer-input row below. The toggle interface is the d-pad + Toggle button, which the operator has now used and called "clunky, but it works." |

Plus AP38 this session (Regen was a translation, §0a). Nothing in the list below is started;
**the operator's call is to resume after the release.**

### Backlog — post-release

> **This is the canonical backlog. Rows carry stable `PR-n` ids** so the charter, `AGENTS.md`
> and commit messages can cite one without restating it (one canonical home per fact).
> **PR-A…PR-C are the operator-named order and keep their priority**; PR-1…PR-3 were added
> 2026-08-23 and are *not* ranked against them — sequencing them is an operator call, not a
> thing a session decides for itself.

**PR-1 — a SQLite-backed query index (the shape fix behind AP39).** **This is the big one.**
2026-08-23 landed the *correctness* fix — `IndexMaintainer.Rebuild` at peer open — and that is
all it is. The three query indexes remain **in-memory**, so every peer open pays a full rebuild:
`li.List("")` over every tree-bound entity, then a content-store `Get` and a CBOR decode each.
Measured (`entitysdk/query_index_rebuild_bench_test.go`, `-race` off per the perf rule):

| entities | peer open | size-dependent component |
|---|---|---|
| 100 | 15.7 ms | — (baseline) |
| 1,000 | 23.1 ms | ~8 µs/entity |
| 10,000 | 102.5 ms | ~8.7 µs/entity |

Linear, ~8.7 µs/entity — **~0.9 s at 100k, ~9 s at 1M**. It is a startup tax that grows with the
store forever, and it is paid to rebuild state the database was already holding.

**The operator's objection is the correct one and is recorded verbatim:** *"the whole point of
SQLite is that it does queries easier than anything else."* Exactly — and today none of the query
work reaches SQL. `system/query` answers from three Go maps that a SQL `WHERE` clause could serve
directly, against a `SqliteStore` that is already open. The seam is deliberate and documented:
`SqliteStore.DB()` exists *"so co-located extensions can create their own tables in the same
database"*, which is precisely this. **Prove the negative before scoping** (done, 2026-08-23):
`MemoryTypeIndex` / `MemoryReverseHashIndex` / `MemoryPathLinkIndex` in `ext/query/index.go` are
the **only** implementations of the three `core/store` interfaces in `entity-core-go` — no
SQL-backed variant exists, and none has been added since the v0.8.0 public release
(`git log --since` on `ext/query/` is 5 commits, all unrelated). So this is **build, not adopt** —
and note it is a *kernel* extension shape, which makes it a design conversation with core-go
before it is a diff here, not a thing to land unilaterally in `entitysdk`.
*(Loose end found while scoping: core-go's own comment at `cmd/entity-peer/main.go` cites
`DESIGN-SQLITE-PERSISTENCE.md §4.3`, and that file exists nowhere in their tree. Worth asking for
when the conversation opens — it likely holds the reasoning this row needs.)*

**PR-2 — restart-cover every read path a store serves** (the generalization AP39 earned). The
gap was never a missing restart test; it was that `TestStorage_Sqlite_LargeCorpusSurvivesRestart`
reads the reopened peer only through `Get`/`List` — the persistent location index — and never
through `system/query`. Enumerate the read paths (tree/location, query, revision log, subscription
tracking, published-root seq, discovery) and confirm each survives a reopen. Cheap, and it is how
we find the *next* one of these instead of shipping it.

**PR-3 — Axis-1's §5.2 constraint reader** (§0b). **Now stands alone** — PR-B, the other half of
the pairing, is done (2026-08-25). Still open, still unreached by any binary, and the position
model that closed PR-B does not touch it: this is the constraint *reader*, not the evaluator's
error semantics. **Note for whoever takes it:** PR-B's cause was named correctly the day it was
found and then talked out of existence — so treat §0b's diagnosis, which names
`0e34e3e` and both replacement functions, as the strong starting point it is, and require any
refutation to reproduce the failing shape.

**PR-5 — 43 unresolvable commit pins in the canonical docs** ([ADR-0012] Amendment 1, landed
2026-08-23 with the ADR injection). Published commits are authored fresh at the release boundary
([ADR-0027]), so **public `master` is a different history from `dev`** and an internal SHA in a
published doc resolves to nothing *by construction* — it was never valid, rather than broken by
the release. Measured, not estimated:

```
python3 ../entity-system-arch-tools/spec-tool/cli.py pins --root .
  21 declared doc(s), 48 short-SHA citations — 43 unreachable by a reader of `master`
  DISCIPLINE-CHARTER.md 17 · MODEL-AVALONIA-RUNTIME.md 14 · DIAGNOSTIC-DIRECTION.md 4 ·
  GUIDE-AVALONIA-PANEL-PATTERNS.md 3 · DEPLOYMENT-DIRECTION.md 2 · TESTING-STRATEGY.md 1 ·
  LOGGING-CONVENTIONS.md 1 · CROSS-IMPL-HELPER-REFERENCE.md 1
```

Every one is an anti-pattern row or a model note citing the commit that earned it, so the repair
is per-citation judgement (cite by content — *"the commit that fixed X"* — or a release tag, or a
content digest), not a sed. **Run the checker before any release cut** and after adding any commit
citation to a canonical doc; it is reader-mode by default, `--gate` to enforce. Content hashes
(64 hex) are never flagged — they are the fix, not the defect.

*Already banked, at zero cost:* dropping `docs/STATUS.md` from `CANONICAL-DOCS.toml` per
[ADR-0031] took **67 more unique short-SHAs** out of the published surface — this file is the
densest SHA carrier in the repo, and it was the one declared exception. It is now internal, which
is also why it can stay frank.

**PR-4 — name the Avalonia headless flake.** 1 failure in 8 runs, still unnamed. The *method* is
fixed (runs are logged now, not tailed), so this needs runs, not a new approach — and three clean
runs in a row do not close it (~0.51 chance of missing a 1-in-5 fault). Next occurrence carries a
name; until then the row stays open and the suite is quoted as **"74/74, one unexplained failure
in 8 runs"** rather than "74/74".

1. **PR-A. Pointer input for the generic host (click-a-cell).** The capability gap, not a Life bug:
   neither input shape we have (`key-set`, a 64-bit mask; `direction`, a 0..3 enum) can carry a
   coordinate. `PROPOSAL-GENERIC-HOST-POINTER-INPUT-DEVICE-2026-07-27.md` is **delivered** and
   arch has it as `COHORT-OPEN-ITEMS` §1d row **W-1, owner arch**, sequenced with T5's resume by
   stated decision. This is what upgrades the toggle interface from "clunky but works" to direct
   manipulation, and it is the honest blocker on shipping interactive Life anywhere as a
   showcase.
2. ~~**PR-B. Three UNDIAGNOSED divergences between the two compute engines.**~~ **DONE
   2026-08-25** — diagnosed and fixed, see §0A. All three were one shape:
   `length(map(arr, λe. e + <out-of-range index>))`, where `map`'s output element is a
   **CONTAINED** position and Axis-1 propagated instead of containing. The cause was
   contained-error semantics after all; the 2026-08-24 refutation was the error, not the
   original diagnosis. Fixed as a position model (`axis1/contain.go`) rather than a patch, which
   also caught a filter predicate that **kept** elements whose predicate had failed. Gate:
   `TestAxis1Equivalence_ContainedErrorPositions`, verified failing pre-fix. Sweep 300/300.
   **§5.2 (PR-3) is still open and is now the whole of that pairing** — see below.
3. **PR-C. The `programs/` ↔ `concat` catch-up.** `programs/` routes around `concat` in four places
   and **Axis-1 implements none of the four v3.24 primitives** against a 350-vector corpus.
   Nothing is waiting on anybody for this (core-go shipped them at `eb80750`).
   **Sized 2026-08-25 (§0c): exactly 11 corpus vectors** — `cv1-group-by-shape`, `cv2-assoc-oob-*`,
   `cv3-range-*`, `cv4a/4b-assoc-*`, `cv5-concat-error-transparent`, `cv6-group-by-error-key`,
   `cv7a/7b/7c-*-collection-error-shortcircuit`. They are 11 of the 28 vectors currently voiding
   AE-5, so PR-C is now on the admission's critical path rather than a nice-to-have.
4. **PR-D. Wire arch's differential compute corpus into a `make` target** (§0c). The gate that
   would have caught PR-B's defect on day one already exists and **skips by default**; the runner
   is in `AGENTS.md`. Not in `test-native` — it needs a buildable sibling, same rule as
   `crossimpl-go`. **Highest value-per-hour row on this list.**
5. **PR-E. Decode `compute/error` as a value leaf** (§0c). One `case` arm in `axis1/decode.go`;
   measured at 12 recovered vectors, 28 → 16 deopts, 0 divergences. Wants its own diff.
6. **Waiting on arch — AE-6 vs a declared scope fence.** Routed as
   `reviews/AXIS1-ADMISSION-LAPSED-2026-08-25.md` §3, with the "should this engine live here at
   all" question in §4. Not blocking: Axis-1 ships in no binary.

### Coordination with `entity-browser-rust` — where we left it

- **They serve `app/life`, the old non-interactive Life.** That is the phase-1 falsification
  fixture — pure Life, deliberately boring, correct as such. Publishing `app/life-edit` to them
  rides on the CDN corridor, which works; but it **inherits row 1 above**, because shipping an
  interactive program to a renderer that cannot deliver the input is half a feature (D23).
  The AP38 fix does not change this — a translated soup and a good one are equally unreachable
  without a controller.
- **The sub-tick input finding is routed and mutual.** `reviews/GENERIC-HOST-SUBTICK-INPUT-2026-08-21.md`
  (committed at `7729cb5`, pushed) — they found AP36 independently, in a different language, and
  fixed it at a **different layer** (`MomentaryGuard`: delay the release by one tick period)
  where we fixed it in the host (an input queue). Routed as a **design result, not a defect
  report**: the open question is whether sub-tick input belongs in the driver or the host, and
  the convention that specifies `rate_hint` says nothing about the interval between ticks —
  which is the hole both implementations fell into. **Blocked on us: nothing. No reply is
  owed to us either** — this is a question for the convention, not a bug in either tree.
- **The `.list` artifact format** and the **fixture re-cut** are the two other live browser-rust
  threads; both are in "Waiting on" below, both cost us nothing today.
- **Not attempted this session, by decision:** no further browser-rust coordination before the
  release. Parity work resumes next week per the operator, then the more complex features.

### The instrument note worth keeping

Three defects in this arc (AP36, AP37, AP38) shipped through green suites, and each was found by
a different thing: AP36 and AP37 by **the first test that crossed the seam** with real input,
AP38 by **an operator playing with the shipped program**. None was found by the layer's own
tests, all of which were complete and correct about their own layer. That is D10 stated as a
measurement rather than a slogan — and for AP38 specifically, the missing instrument was not
coverage but a **strong enough property**: the test asserted the board *changed* when what it
needed to assert was that the boards were *independent*.

## Waiting on

- **⚠ meta / release team — the release NUMBER, and it is the last thing our `CHANGELOG` is
  missing.** Arch cut **0.8.2** tracking the protocol; `entity-core-py`/`rust`/`go` went
  **0.9.0**; we are public at `v0.8.0` and our heading is still `[Unreleased]`. **Deliberately
  not guessed** — core-go spent a commit on exactly this failure mode (`13a42ea`, *"stop
  restating the release version in status prose — it went false on py/rust the moment they
  cut"*). Sub-question in the same breath: the CHANGELOG's *"Initial public research-preview
  release"* line predates the `v0.8.0` tag and had never been filed under a heading; we attached
  it to `[0.8.0]` as a **reconstruction from the tag**, not from a record. If the release record
  says otherwise, one line fixes it. **Blocked on us: nothing** — the entry is written and the
  number is a heading edit. Routed in `reviews/RELEASE-READINESS-REPLY-2026-08-24.md` §6.
- **⚠ operator / arch (ledger M-4) — [ADR-0030]'s disposition.** It is **retracted** and its own
  text still reads `Status: Accepted`; arch reverted their copy, we kept ours and annotated it
  (§0A). We are stable either way. The only thing we are asking is that it not sit indefinitely
  in the state where the text asserts a decision that was withdrawn — that state is precisely
  what the retraction is about. **Blocked on us: nothing.**
- **arch — the generic host's third input device (pointer/`click`). DELIVERED; sequenced, not
  blocked.** `PROPOSAL-GENERIC-HOST-POINTER-INPUT-DEVICE-2026-07-27.md` is AGENTS.md's worked
  AP28 example — for 24 days this row read "blocked on arch" while the packet had **no filename
  hit and no subject hit** in either sibling tree. Pushing `dev` was the delivery; arch read it
  at `98ff6de` and opened `COHORT-OPEN-ITEMS` §1d row **W-1, owner arch**, deferred by stated
  decision alongside T5's resume. **The content is undecided, so it stays on this list** — but
  the reason is arch's sequencing, not our non-delivery, and the difference is the whole of
  AP28. It is row 1 of the post-release programs backlog above: the answer to hypothesis (a),
  and what turns interactive Life's toggle interface into direct manipulation. *(Duplicate of
  the struck-through row further down; kept here because this is where a reader looks first.)*

> **"Waiting on" means the content is undecided.** A ruling that has not been folded into spec
> text is **not** on this list — that is an editorial queue item on the authoring repo's board,
> and we build against the ruling and route what it teaches (operator ruling 2026-08-19, see §3
> piece 4 and `AGENTS.md`). Putting a decided-but-unfolded surface here is how a self-inflicted
> stop gets laundered into a dependency.

- **⚠ arch — `transports` on a §3 registry binding: rule the sentence.** Two live readings,
  both conformant to the text, and the consequence is not latent: **`entity-core-go`'s
  peer-issued backend cannot decode any binding in `entity-browser-rust`'s federation**, for
  every name. Routed 2026-08-21 as
  `reviews/REGISTRY-BINDING-TRANSPORTS-DIVERGENCE-2026-08-21.md` §0/§2 (cc core-go, core-rust,
  browser-rust). **Blocked on us: nothing** — our consumer reads both shapes and keeps them
  distinguishable, and our emitter writes inline and says so. What is blocked is Go peers
  resolving Rust-issued names. *(Per AP28: delivery to be established by grepping arch's board
  for the SUBJECT, not this filename.)*
- **⚠ arch — §6a.3a's recommended publishing prefix is unimplementable** (same packet, §6).
  A binding's signature lives at `system/signature/{hex}`, outside every `system/registry/…`
  prefix, so a registry that follows the SHOULD **enumerates fine and resolves nothing**.
  One added sentence fixes it. Measured, with a control:
  `publish/registry_roundtrip_test.go::TestNarrowRegistryPrefixOmitsTheSignatures`.
  **Blocked on us: nothing** — we publish at `system/` and say why.
- **browser-rust — the `.list` artifact's format** (same packet, §7). The spec names a
  `system/tree/listing` entity; their static emitter writes newline text. Costs us nothing now
  (we read both), and it is worth knowing which of us is wrong. **Blocked on us: nothing.**
- **core-go — export `normalizeName`** (same packet, §5). Declinable; a doc sentence naming it
  as a cross-impl contract would also do. **Blocked on us: nothing** — transcribed and pinned.
- ~~**⚠ arch / operator — the compute hold needs a call**~~ — **ANSWERED 2026-08-20**
  (`ROUTING-2026-08-20-e` §1, §11 here). **The hold stands and was delivered**: compute is deferred
  by operator decision and **resumes after the release**. It is off this list because the content is
  decided, not because it is done — the five levers stay arch's and stay on their board. Lever 1's
  ruling is **folded** (`EXTENSION-COMPUTE` 3.24) and the builtin is implemented nowhere, ours
  included; nothing here is waiting on it. **That last clause is now wrong and is corrected in
  §12a — core-go shipped all four v3.24 primitives at `eb80750`, after the commit we measured.**
- **arch (post-release, not waiting on us or blocking us):** the subtree-state descriptor/host
  convention (the successor rung, and the same rung as Doom-realtime);
  `PROPOSAL-CONTINUATION-STANDING-MODEL` §4 (the continuation join-failure policy); whether a
  scan/up-sweep orchestration is in scope; **§4's fairness clause, which the 3.24 fold left
  explicitly not ruled** — adopting `concat` does not bless an in-compute sharded step.
- ~~**⚠ OURS, not theirs — deliver `PROPOSAL-GENERIC-HOST-POINTER-INPUT-DEVICE-2026-07-27.md`**~~
  — **DELIVERED, and arch has it.** Pushing `dev` was the delivery; arch read it at `98ff6de` and
  opened `COHORT-OPEN-ITEMS` §1d row **W-1, owner arch** (`ROUTING-2026-08-20-l` §1/§3, §12a here).
  Not ruled this session by **stated decision** — sequenced with T5's resume — which is the kind of
  deferral a counterpart can confirm. **Blocked on us: nothing.**
- ~~**arch — the `rendezvous` fold (R-10)**~~ — **FOLDED 2026-08-20.** `EXTENSION-DISCOVERY`
  1.0 → 1.1 carries all four of our build findings in full (`-l` §4). `entitysdk/rendezvous.go`
  now stands on landed spec; nothing owed either way.
- **arch — R-9, the `hints` round-trip:** answered from our side in §12a (refuted for this seat,
  their second observation confirmed). Listed only so the reply is greppable; nothing is owed to us.
- **Lever 1 is NOT waiting on anybody, corrected 2026-08-20.** `concat` and the other three v3.24
  primitives are implemented in core-go (`ext/compute/builtins_v324.go`, `eb80750`); §11's
  "implemented nowhere" was measured at an ancestor commit. Two rows this opens, both post-release:
  `programs/` still routes around `concat` in four places, and **Axis-1 implements none of the four**
  against a 350-vector corpus.
- **`entity-core-go` kernel:** published + tagged vanity module path; an idempotent
  identity-ceremony re-apply; a per-delivery deadline + parallel delivery workers;
  incremental revision-trie update.
- **Cross-team:** the content-store GC / reachability contract.
- **`entity-browser-rust`:** the follow vocabulary — one record + verb, `strategy` with a
  value that does not assume ordered delivery (share-review §3.2). Step 4 of the share arc,
  and step 5 waits on it by construction. **Also: the fixture run.** `DirFetcher::manifest()`
  reads `manifest_url_prefix` per `-p` §3, then runs our re-cut fixture (`d940ce0`, byte-stable)
  and tells us where it actually stops.
- **arch:** `REG-DISPATCH-CATCHALL-LOCAL-1` re-keyed or explicitly scoped — §11.1's vector still
  bans remoteness while §4.1 step 2 bans name transmission, so it contradicts §4.1a row 6's
  `peer-issued` (§6b). Blocks nobody today; misleads everybody later.
- ~~**arch:** `APP-CONVENTION-SHARE` authored~~ — **DELIVERED 2026-08-18.**
  `specs/applications/APP-CONVENTION-SHARE.md` v0.1 exists (arch `bb86cd1`, routed as
  `ROUTING-2026-08-18-i` §4). **`app/share/*` is no longer blocked by arch.** Four constraints to
  build against: a share is a titled grant (`resources` = what is shared, audience = the minted
  token's `grantee`); **`peers` MUST be omitted** (populating it 403s every cross-peer presentation
  and still passes local testing, because our single-identity tests collapse root/grantee/granter);
  type tags are `app/share/*` (the index key for cross-peer aggregation — a tag under our own
  prefix breaks browser↔go interop); mirrors write the publisher's paths verbatim. Withdrawal is
  asymmetric — `request`-minted tokens are **not** recallable and a UI **MUST NOT** imply otherwise.
  Not ratifiable (zero vectors); `SHARE-4` and `SHARE-6` are the two owed vectors that fail loudly
  on the intuitive-but-wrong reading, and are where we start.
  **Step 5 is now blocked only on the core-go kernel defect (§6), not on arch.**
