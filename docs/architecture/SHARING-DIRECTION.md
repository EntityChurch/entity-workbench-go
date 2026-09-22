# Sharing direction — the relationship is the object, and the rest is derived

**Status:** long-lived belief; edit in place.
**Companion:** `FILE-REPLICATION-LANDSCAPE.md` compares the *engine* (chunking,
merge, conflict semantics) against rsync / Unison / Syncthing. This document is
about the layer above it — the **relationship model and the control loop** — and
about the shape that lets the next shared thing (a revision project, a
collaborative document, a compute job) reuse all of it.

---

## 0. The report this exists to answer

An operator ran the two-machine share flow across two days and never got a file
across. The verdict was *"scrap this whole flow"*, and the audit
(`docs/status/HANDOFF-2026-09-03-share-flow-audit.md`) found one hard defect that
made every attempt fail. Fixing that defect does not make the flow good. Nine
steps, of which two are the user's intent, is not a bug in a panel.

The second half of the report is the one this document is really about:

> *"We're not leveraging the SDK. We're making up workflows, making up calls,
> rebuilding stuff. We don't want a one-off interface for every little thing we
> build. If I want to sync something else, do a revision project, do a CRDT for
> a text app — that pattern should be pretty much the same."*

That is correct, and it is measurable rather than a matter of taste. §2 is the
measurement.

---

## 1. The landscape, at the level that decides the UX

`FILE-REPLICATION-LANDSCAPE.md` §1 covers the engines. What it does not cover —
and what turns out to matter more for whether the product is usable — is that
the two obvious reference points have **fundamentally different relationship
models**, and only one of them is available to us.

**Dropbox is an account model.** There is a server. Identity is an account, trust
is "logged in", and a share is a row in someone else's database. Two devices
never need to find each other, agree on anything, or be online at the same time.
Almost none of its interaction design transfers to a peer-to-peer system,
because every hard problem it solves has been solved by having a third party
that is always up.

**Syncthing is a peer model, and it is the right reference.** Its whole
configuration is two object kinds:

| Object | What it is | Approval |
|---|---|---|
| **Device** | a remote peer, named by its device ID | **mutual** — each side must add the other, once |
| **Folder** | a local directory, shared *with* a set of devices | the receiving side is prompted, picks a path, accepts |

Everything else an operator sees — connection state, sync progress, out-of-sync
items, last-seen, per-folder errors — is **derived and displayed, never
configured**. And the two config objects have exactly the property that makes it
feel like it works: **order does not matter.** Add the folder first or the device
first, from either machine, in any sequence, while the other side is offline —
it converges when both facts exist and both machines are up.

Two of its properties are worth naming because we have been treating them as
accidents rather than as the design:

- **Mutual authorization is not friction, it is the model.** Syncthing also
  requires both sides to add each other. Nobody experiences this as a nine-step
  dance, because it is *one* approval per relationship, it is durable, and the
  UI presents the other side's pending request rather than instructing you to go
  press a button on another computer.
- **A pending offer is a first-class UI object.** "Device X wants to share folder
  Y" appears as a card with an Accept button and a directory picker. It is not a
  message telling you to run a command.

We are closer to Syncthing than to Dropbox in every respect, including the ones
where we are ahead (content-defined chunking; capability-scoped sharing that is
finer-grained than device-level trust). The gap is not the engine.

---

## 2. What we actually built, measured

### 2.1 The kernel facility we hand-rolled around

`entitysdk.NetworkClient.MaintainPeer` wraps `system/network:maintain-peer`,
which is a **relationship lifecycle**: connect, install a reconnect continuation
graph, retry with derived backoff **forever by default**, re-validate and restore
subscriptions on reconnect, and write `system/peer/status` transitions the tree
can be watched for.

It has **zero callers in shipped code.** Ten references in this repo: nine inside
its own file, one in its own test. What the product calls instead is
`AppPeer.Connect` — a single dial that establishes one connection and promises
nothing about it afterwards — from three places.

So: *"why do I have to hit connect again, why doesn't it reconnect on startup"*
has a one-line answer. The reconnect engine is in the kernel, is wrapped in our
own SDK, and nothing runs it.

Two more of the same shape, both confirmed by grep rather than assumed:

- `ext/signaling/peerwiring.Coordinator` — establishment that marks a connection
  rendezvous-established on **both** sides, which is what makes authority
  symmetric. Unused here. (`entitysdk/rendezvous.go` is the *discovery* backend —
  a different layer, and easy to mistake for coverage.)
- `localfiles.RootConfigData.ReadOnly` — Syncthing's "Send Only", the most-used
  non-default folder type. Honoured by the kernel's writeback path, settable from
  nothing we ship.

This is D20 pointed at ourselves: **price the work against the substrate, not
against our own tree.** Every one of these was found by grepping the kernel for
the thing we were about to build.

### 2.2 Four independent reasons nothing survived a restart

The operator restarted constantly — because iterating is what you do — and every
one of these silently destroys the flow across a restart. They stack, so fixing
any one of them alone would have changed nothing observable.

1. **The GUI's default peer was ephemeral.** No identity, memory store → a fresh
   keypair every launch. The whole tree is peer-id-namespaced, so the app was a
   **different peer** every time it started. Every grant, offer, mount and
   accepted share from the previous session named a peer that no longer existed.
2. **The GUI's default peer did not listen or announce**, and discovery is
   disabled entirely without a listener. The default configuration could not be
   reached, could not be found, and could not share.
3. **`localfiles.Handler.Load` restored no mounts** (AP58, in the kernel — routed
   in `reviews/LOCALFILES-LOAD-RESTORES-NOTHING-2026-09-03.md`, worked around in
   `workbench/localfiles_root_restore.go`).
4. **Nothing re-established a peer relationship at startup**, per §2.1 — and even
   with `MaintainPeer` called, the kernel's session map is in-memory with no
   rebuild at open, so the app has to re-issue it per launch. That is AP62's
   shape for the third time in this repo and the fourth in this dependency.

And a fifth, found while writing this and fixed in the same session:
**`entity-shell -listen ADDR` bound no socket at all.** The address was carried
into `peer.WithListenAddr`; core-go reads that field in exactly one place
(`Peer.Listen`); nothing in the shell's startup called it. A documented flag,
step 1 of the operator recipe, doing nothing — and every e2e suite passing
throughout, because each stands up its own listener with a local helper instead
of going through the frontend's startup path.

### 2.3 The structural error

**We wrote a wizard where the substrate wanted a controller.**

`Share` → `Accept` → `CompleteShare` are imperative one-shot sequences. Each one
encodes, inside a single button press, the four authorization facts AP63 cost us
a day to learn (a sync is mutual; the grant is assembled at handshake; the peer
that dispatches is the one that must reconnect; a dial-by-address authorizes only
the dialer). That makes the flow **order-dependent, non-idempotent, and dead
after a restart**: press the buttons in the wrong order, or relaunch, and there
is no path back except doing all nine steps again in the right sequence on two
machines.

Underneath, the durable state is **five disconnected records**, each written by a
different verb and restored by a different startup path:

| Record | Written by | Restored at startup by |
|---|---|---|
| roster entry `app/{app}/system/peers/{peer}` | `PeerCreate` | `RestorePeers` |
| policy row `system/capability/policy/{peer}` | `share` / `accept` | the kernel, at handshake |
| offer record | `share` | nothing — read on demand |
| mount binding `app/workbench/mounts/{root}` | `mount` | `RestoreMountBindings` |
| sync binding `app/workbench/syncs/{peer}.{root}` | `sync` | `RestoreSyncBindings` |

**None of them is the relationship.** There is no object in this system that says
"this peer and I share this folder"; there are five artefacts from which that can
be *inferred*, written at five different moments, and any one of them going
missing produces a different silent failure. Every restart bug in §2.2 is a
consequence of that.

---

## 3. The model — two declared objects, one loop

Adopt Syncthing's shape, on our substrate.

### 3.1 Declared, in the tree

```
app/workbench/devices/{peerID}      a peer we know
app/workbench/shares/{shareID}      a thing we share, and with whom
```

A **device** is: peer-id, label, last-known addresses (from discovery, from a
manual entry, or from their advertised transport profile), when we added it, and
whether we auto-connect. Adding one is the single approval that establishes a
relationship.

A **share** is: what is shared (a local path and mount root, for `kind: files`),
the mode (send / receive / both), and the set of devices it is shared with, each
with a state — `offered`, `accepted`, `declined`, `withdrawn`.

These two are **desired state**. They are written by the operator's two real
gestures and by nothing else. They are in the **tree**, not in a JSON file — see
§6.

### 3.2 Derived, by a reconciler

Everything in §2.3's table stops being a thing anyone writes directly and becomes
output of one idempotent function:

```
reconcile(desired) → { maintain-peer sessions,
                       capability policy rows,
                       local-files mounts,
                       subscriptions + sync bindings,
                       observed status }
```

It runs at **startup**, on **every change to desired state**, and on every
**connection or discovery event**. It is idempotent, so running it twice is free
and running it in the wrong order is impossible — there is no order, only a
target.

This is what buys the properties the flow does not have today:

- **Restart-safe by construction.** Startup is just the first reconcile. There is
  no separate restore path per record type to forget — which is how §2.2 items 3
  and 4, and the two AP62 instances before them, each happened.
- **Order-free.** Share before the peer is added, accept while the other side is
  offline, mount later — the loop closes when the facts are all present.
- **Self-healing.** A grant that was written on a live connection (and is
  therefore inert until the next handshake) is not a permanent trap; the next
  reconcile notices the connection predates the grant and re-establishes it.
- **The AP63 facts live in exactly one place** — the reconciler — instead of
  inside three verbs and, if we are not careful, a renderer.

`MaintainPeer` is the reconciler's connection primitive. We do not write a
reconnect loop; the kernel has one, with backoff, subscription restoration and
tree-visible status.

### 3.3 Observed, and rendered

The status an operator reads is **read from the substrate, never remembered**:
`system/peer/status` for liveness and reason, the mount's own counts for
progress, chain-errors for delivery failures. The panel renders observed state
beside desired state, and the difference between them is the whole diagnostic
surface — *"you want this shared; here is what is actually true; here is what is
stopping it."*

Note the distinction the current Connections panel gets wrong: it reports socket
state. A connection is directional for **authority** and undirectional for
display, so "connected" can be true and useless. The relationship view must show
**authority in each direction**, because that is the thing that is actually
broken when nothing arrives.

---

## 4. Why this generalizes — the adapter seam

The reason to build a relationship layer rather than a better share button:
**almost none of it is about files.**

Decompose what a share needs:

| Concern | Files | Revision project | Collaborative doc | Compute job |
|---|---|---|---|---|
| know the peer | same | same | same | same |
| mutual authorization | same | same | same | same |
| durable desired state | same | same | same | same |
| reconnect + restore | same | same | same | same |
| offer / accept | same | same | same | same |
| status + errors | same | same | same | same |
| **what a notification means** | write bytes to disk | apply a commit | merge an operation | enqueue work |
| **what "up to date" means** | file counts | head equality | causal delivery | queue depth |

Only the last two rows differ. So the seam is a **share kind adapter**:

```go
type ShareKind interface {
    Kind() string                                  // "files", "revision", ...
    ValidateTarget(local string) error             // is this a thing we can receive into?
    SubscriptionPattern(peer, root string) string  // what to subscribe to
    Materialize(ctx, notification) error           // what an arriving change does
    Status(local string) ShareStatus               // what "up to date" means here
}
```

`kind: files` is `local/files` + `workbench/blob-resolve`, which already exists
and already works. `kind: revision` is `ext/revision` — the composition
`FILE-REPLICATION-LANDSCAPE.md` §4 calls the only genuinely novel thing here —
and it inherits the entire relationship layer for free.

**This is the answer to "we don't want to rewrite 80 flows."** The flow is
written once. A new shared thing is an adapter and a row in a registry, not a
panel.

Two honest caveats. The interface above is a **sketch derived from one
implemented kind**, and an interface generalized from one instance is a guess —
it gets ratified when `kind: revision` is built against it and not before
(promotion ladder: this is a candidate, not a discipline). And the seam must not
be built speculatively ahead of that: build the files kind through the seam,
prove the seam by writing the second kind, then claim it.

---

## 5. What the operator does

**Machine A:** pick a folder → pick a peer → Share.
**Machine B:** a card appears — *"desk-2 wants to share `downloads`"* → pick a
directory → Accept.

Nothing else. No address typing, no offers button, no separate mount step, no
"now go press Complete on the other machine". Two gestures, which is what the
nine-step table always had underneath it.

Everything removed is removed because the reconciler does it:

| Was | Now |
|---|---|
| launch both with `--listen` | default |
| connect by address | discovery + device record + `MaintainPeer` |
| mount on the sender | implied by choosing a folder to share |
| mount on the receiver | the directory picker in Accept |
| "check their offers" | offers arrive as cards |
| "complete the connection" | the reconciler dials from both sides |

The permission stage is **not** removed. It is still two grants, both still
required (AP63), and pairing is still a deliberate act — it is one approval per
peer and one per folder, both durable, neither ever asked twice.

---

## 6. The file/tree boundary — answered

*"I noticed we're saving stuff and then on startup reading a JSON file. Why isn't
that in the tree?"*

Correct question. Today `~/.entity/gui-layout.json` and `~/.entity/browser.json`
are on-disk config. The stated reason for the layout file
(`workbench/layout_config.go`) is that it is keyed by **alias, not peer-id**,
because with an ephemeral default peer a peer-id-keyed layout would never match
itself twice — the feature would silently do nothing in the default
configuration.

**That argument was a consequence of the ephemeral default, and the default is
now persistent.** So it no longer holds, and the rule that replaces it is:

> **A file holds what you need in order to find the tree. The tree holds
> everything else.**

- **Pre-peer, therefore a file:** which identity, where the store is, what to
  listen on. You cannot read these out of a store whose location they determine.
- **Post-peer, therefore the tree:** panel layout, registry pin, devices, shares,
  and everything in §3. This is per-peer state, and putting it in the tree means
  it is addressable, watchable, revision-able, and it travels with the peer when
  the peer is backed up or moved — which a file in `$HOME` does not.

Devices and shares are being built in the tree from the start. Migrating the two
existing JSON files is a follow-up, not a prerequisite, and it needs a read-both
/ write-tree transition so an existing install does not lose its layout.

---

## 7. Sequence

**S0 — make iteration cheap and observation trustworthy. DONE 2026-09-03.**
Podman cache mounts on every compile step (a rebuild after a source edit went
from re-downloading the Go and NuGet worlds to ~13 s); a build stamp derived from
`git describe` plus a working-tree hash, compiled into **both** the frontend and
the bridge, printed at startup, written to the crash trail, and **in the window
title** — because the way this information actually travels is a screenshot. A
frontend/bridge stamp disagreement is reported as a mismatch, which is the one
state `extract` can produce and nothing could previously report.

**S1 — a default configuration that can work. DONE 2026-09-03.**
The GUI defaults to a persistent identity, an on-disk store, a listener on
`0.0.0.0:9110` (ephemeral fallback if taken — a second instance must still
start), an mDNS announcement, and a transport profile derived from this host's
LAN address rather than skipped for a wildcard bind. `--ephemeral` keeps the old
throwaway peer. `BringUpListener` is now shared by every frontend, so
`entity-shell -listen` binds a socket for the first time, gated by a test that
dials it from a second peer.

**S2 — the relationship object and the reconciler.** Headless first, and gated
across a **process boundary** on the **derived** structure, per D26/AP62 — this
repo has now shipped that same defect three times and each one was invisible
in-process.

**S3 — accept-with-directory. DONE 2026-09-03.**
`accept <peer> <root> <directory>` creates the directory, mounts it,
authorizes, subscribes and backfills, in one action. It refuses a
directory that already has files in it — their writes overwrite yours and
their deletes remove yours — and `-anyway` overrules that; the GUI
renders the refusal as a choice rather than a failure, pre-filled with a
fresh path under `~/entity-shared/`. Two steps of the nine went with it:
the receiver's separate `mount`, and the **unwritten rule that the
receiving directory had to be named the same as the sender's folder**,
which nothing stated and which silently produced a relationship that
could not deliver. That coupling is why `FolderData` now records a
`LocalRoot` beside the sender's `Root`, read through `ReceivingRoot()`.

**S4 — the panel over the declared state. DONE 2026-09-03, and it is NOT the
flow in §5.** Read §9 before planning further work here: the operator who asked
for a streamlined flow got a diagnostic window, correctly built and answering the
wrong question. S4 is a good *instrument*; it is not the product.
`Sharing Status (declared vs. actual)` — devices and folders, observed state
beside desired state, and what is stopping the rest. Built as
`shellcmd.StatusSnapshot` (a read) beside the existing `Reconcile` (a pass),
four bridge exports, and `SharingStatusPanel`. It does not replace the Local
Files / Shared Folders / Peer Connections panels — those stay as the surfaces we
learn the system from, and the question of what they become is answered *after*
the flow is understood, not before.

Two verbs came with it rather than a read-only view, because a panel with no verb
in it is AP57's tell: **Pause / Resume** a device (which writes the
*declaration*, so the loop obeys it rather than being fought by it) and
**Remount**, which is the action the reconciler NAMES and refuses to take. That
refusal is the loop's bargain — it will not write to a directory on somebody's
disk — and it only works if some surface offers the other half. The remount takes
no directory argument: the path comes from the declaration, which is the only
record left that can say where a folder's files are once the mount that knew is
gone.

What it does **not** offer is a "pending offers" feed. Offers are pulled today,
and the honest first version of a push feed needs a grant we do not currently ask
for; polling every declared device on a cadence would look like a hang whenever a
device is asleep. Shared Folders keeps the pull, and this panel does not imply a
liveness it does not have.

It is **not** "a Sync panel", and that correction is worth more than a name.
Three points, settled 2026-09-03 while reviewing S2/S3:

- **It is the missing half of S2, not a new feature.** There is no bridge export
  for devices, folders or the reconciler — none — so the entire declared-state
  layer is unreachable from the GUI. Meanwhile the GUI *runs* a reconcile pass at
  every startup and reports its problems to **stderr**, which for a desktop app
  is a log file nobody opens: an operator whose accepted folder has lost its
  mount is told so correctly, in a place they will never look, while every panel
  on screen looks fine. `make reachability` cannot raise this — it asks whether a
  `workbench/*_model.go` has *a* surface, and the reconciler is `shellcmd`'s and
  has a verb.
- **Name it for the question it answers.** With Peer Connections ("who am I
  connected to") and Shared Folders ("share and receive") already in the Network
  category, a third panel called "Sync" repeats the Browser / Site / Publisher
  Verify mistake an operator called incomprehensible. The question here is the
  `status` verb's — *is what I declared actually working, and what is stopping
  it* — so the name says that.
- **Authority is only half knowable, and the panel must not fake the other
  half.** What we grant them is our own policy row, exactly known. What they
  grant us is in *their* capability table, which we cannot read: it is only ever
  observed, through deliveries arriving or through chain-errors. So the inbound
  direction renders as an observation with a time — "last received 12:04",
  "nothing yet" — never as a green dot. A dot there asserts something about
  another machine that this peer has no way to check, and it is wrong in exactly
  the case that matters: they revoked us and we have not tried since.

One structural consequence: **render and reconcile are separate exports.** The
`status` verb runs the loop on purpose — a read-only report has the same blind
spot as the five verbs it replaced. A panel cannot copy that, because it
refreshes on wakes and a reconcile pass *dials*; wake-driven reconciling turns a
status panel into a dialer. So: a cheap read for wakes, and an explicit pass on
open, on a re-check button, and after any mutation the panel makes.

Built as `StatusSnapshot` / `Reconcile`, sharing one `observeDevice` /
`observeFolder` pair so a read and a pass cannot describe the same device
differently, and one `FolderStatus.problems()` so they cannot describe the same
fault differently either. The distinction rides in the **outcome**
(`Reconciled bool`), not in the caller's memory: "this reading was verified by a
pass" is a property of the reading, and a surface that has to recall which
function produced its table in order to caption it will eventually caption it
wrong — and the wrong caption is always the confident one.

**S6 — ONE FOLDER, TWO PEERS.** The identity gap in §9.2. Until a folder is a
single object both peers can name, "share this with them" cannot mean what
everybody assumes it means, and no amount of panel work fixes it.

**S7 — the flow in §5, for real.** Two gestures, one surface. The audit in §9.3
first: every button, what it is for, and what it costs a first-time operator to
have it there.

**S5 — the second kind.** `kind: revision` through the adapter seam, which is
what ratifies §4 or refutes it. **Deliberately after S6/S7 now.** The reason is
in §9.5: this repo is the furthest along, so the shape it settles on is the one
the other implementations will copy. Shipping the seam before the model is right
exports the model being wrong.
## 8. What this document does not claim

- **Cross-NAT is not addressed.** Everything above assumes the two machines can
  reach each other — same LAN, or a routable address. `peerwiring` plus a
  signaling carrier is the answer for the internet case and is deliberately not
  in the sequence: it needs infrastructure to be deployed, and it would not have
  fixed any part of the reported failure. The previous handoff nominated it as
  *the* lever; that was right about the mechanism and wrong about the priority,
  because on a LAN the reconciler plus discovery removes the address problem
  without it.
- **Conflict semantics are unchanged.** `FILE-REPLICATION-LANDSCAPE.md` §3 still
  decides whether this becomes a tool people install. A concurrent edit still
  loses a write silently. Nothing here improves that; it makes the layer above it
  usable enough that the question can be reached.
- **The adapter interface in §4 is a sketch**, generalized from one implemented
  kind. It is a candidate until a second kind is built against it.



---

## 9. The second operator report — 2026-09-04

The flow works now. A file was shared between two machines and arrived. That
is the first time that sentence has been true, and everything below is about
what the operator said next.

What they reported, in their words rendered here as findings:

- **The panel was confusing and offered no obvious order of operations** — buttons
  everywhere, no indication of which to press first, so every one of them got pressed.
- **It did not behave as bi-directional.**
- **Inbound and outbound went to different places** — content from the external peer was
  written into one directory and content from here was uploaded into another.
- **Nothing came back the other way**, so the two sides were doing bilateral transfer to
  different locations without any shared understanding of what the pairing meant.
- **This is the diagnostic panel, not the one sync panel we agreed to build.**

Every one of those is correct, and three of them are the same defect.

### 9.1 We built the instrument, not the product

§5 of this document specifies the flow: *pick a folder → pick a peer → Share*,
and on the other machine *a card appears → pick a directory → Accept*. **Nothing
else.** S4 shipped `Sharing Status (declared vs. actual)` — which answers *is
what I declared actually working*, a question §5 does not ask and an operator
only reaches for once something has gone wrong.

It is a good instrument and it stays. But it was scheduled as the next step
toward §5 and it is not on that path, and calling it S4 disguised that. The
streamlined flow is **unbuilt**, and the sequence now says so.

### 9.2 A folder has no identity across peers — this is the real defect

*"Their sources and their destinations are different... they don't have the same
understanding."* That is not a UI problem. Measured in the code:

- `share` creates `app/workbench/folders/{root}` on the sender, `Origin: local`.
- `accept` creates `app/workbench/folders/{peer}.{their-root}` on the receiver,
  `Origin: {sender}`, `Mode: receive`.
- **Nothing joins those two records.** They have different ids, different roots,
  different paths, and neither carries a name the other would recognise.
- **`FolderData.Mode` is written and never read.** `grep` finds exactly two
  writers (`declare.go`) and two readers, both of which put it in a status DTO
  for display. No code branches on it. So `send` / `receive` / `both` is a
  vocabulary the product does not implement — **AP67 again**: a configuration
  field that nothing reads is a fiction, and this is its second instance.

So "share this folder with them" today means *"publish my directory; they
subscribe to it"*. One direction, full stop. Getting the reverse means running a
second, unrelated `share` on the other machine, which produces a second folder
object with a different id, pointed at a different directory. Two one-way pipes
that share nothing but the operator's intention — which is exactly what was
described, and exactly what it feels like.

**Syncthing's model is the opposite and is the one to adopt.** There, a folder
has a **Folder ID** that is the same string on every device. Each device chooses
its own local path. Direction is a per-device *property of one shared object*,
not a different object per direction. That single fact is what makes "share a
folder with a device" mean what everyone expects, and it is what we do not have.

**S6 is that.** Until a folder is one object both peers can name, no panel can
present it as one thing, because it is not one thing.

### 9.3 Five panels, twenty-two buttons, eighteen verbs — for one job

*"Buttons all over the place. I just kept clicking them all."* Counted, rather
than sympathised with:

| Surface | Buttons |
|---|---|
| Shared Folders | 10 |
| Sharing Status | 5 |
| Local Files | 5 |
| Files | 2 |
| Peer Connections | 0 (fields + a dial) |

Five panels touch this one job — Peer Connections, Shared Folders, Sharing
Status, Local Files, Files — and the shell exposes **eighteen** verbs for it:
`mount unmount mounts share unshare shares offers accept sync unsync syncs
resync forget access status peers connect disconnect`.

Every one of them was added for a real reason and most are the *seam* of some
defect this document records. That is the trap: **each was locally justified and
the sum is unusable.** The audit S7 opens with is not "remove buttons" — it is,
for each control, *which of the two gestures in §5 does this serve, and what does
its presence cost someone doing this for the first time?* A control that serves
neither belongs in a diagnostic surface or behind an "advanced" disclosure, not
in the flow.

### 9.4 The name collision is a real trap, not a nitpick

*"This entity-shared directory overlapping — because I had downloads, so I had to
name an entity downloads."*

Accept pre-fills `~/entity-shared/{their-root}`. When their folder is called
`downloads` and you already have `~/Downloads`, the operator is asked to reason
about a name that means two different things in two different places, at the
moment they are least equipped to. The pre-fill is defensible (a fresh directory
that will not trip the non-empty refusal) and the *label* is wrong: the row
should say whose folder it is and where it will land in one sentence, and the
default should be qualified by the peer — `~/entity-shared/{peer-label}/{root}` —
so two peers sharing a folder of the same name do not collide either.

### 9.5 Why this has to be right before it is copied

This repo is the furthest along, and the plan is for `entity-browser-rust`, the
Godot frontend and the Python implementation to carry the same pattern. **A
model that is wrong here gets exported four times.** Concretely: if `Mode` stays
a fiction and a folder stays peer-local, every implementation inherits
"bidirectional means two unrelated one-way pipes", and the conformance surface
will encode it.

That is why S5 (the second kind) now sits *after* S6 and S7. The adapter seam is
supposed to be the thing that lets a revision project or a CRDT document reuse
the whole relationship layer — and that is only worth generalising once the
relationship layer means what it says. Building the seam on top of a folder model
with no shared identity would generalise the defect, not the design.

### 9.6 What is genuinely established

Said plainly, because the list above is long and the progress is real:

- Two machines, one folder, files across, live changes propagating.
- Accept creates the directory, mounts it, authorizes, subscribes and backfills
  in one action.
- Restart survives, mostly — with §6.1 of `avalonia/README-SHARING.md` the known
  exception, characterised and routed.
- The declared-state model and the reconciler are the right shape and are not in
  question. Everything in §9.2 is a gap *in* that model, not an argument against
  it.

The foundation holds. What is missing is that the object the operator thinks
they are manipulating does not exist yet.

## 10. Resolution — 2026-09-04, second session

§9.2 and §9.3 are closed. §9.4's trap is closed by naming. What follows is
what the fix actually was, because the scoping in §9 over-estimated it and the
reason is worth carrying.

### 10.1 It was two changes, and grounding it first is why

§9.2 proposed adopting Syncthing's model. Reading Syncthing's configuration
documentation **before** designing collapsed the scope: their `<device>`
element is `id` / `name` / `address` / `paused` — which is `DeviceData`
already — and their `<folder>` is `id` / `path` / a per-folder device list /
a type, which is `FolderData` already. Their `path` is explicitly *"not sent
to other devices"*, which is our `Path`, and their folder `id` is the same
string on every device, which is the one thing we did not have.

So: **one divergence and one dead field**, not a model to adopt. The lesson
is not about Syncthing. It is that *"adopt X's model"* is a scoping claim,
and checking it against X's actual documentation is cheap and changed the
size of the work by an order of magnitude.

### 10.2 The id is derived, not minted

`workbench.FolderID(owner, root)`. Syncthing mints a random folder ID and
copies it between devices; we do not have to, because the pair (owner
peer-id, the owner's root name) already identifies the folder and both sides
already hold both halves at the moment they need it.

That matters for a specific reason: minting would mean carrying a new field
in the offer record, and the offer record's type lives in `app/share/*`,
which is APP-CONVENTION-SHARE's namespace. A field there is a cross-impl
coordination, not a local edit. Deriving it makes the id impossible to get
wrong by construction and costs nothing on the wire.

**The cost, stated so nobody rediscovers it as a surprise:** renaming the
owner's root renames the folder. That is already true of the subscription and
the sync binding, which are keyed on the same name, so this inherits an
existing fragility rather than adding one. If rename-stability is ever
needed, mint at the owner and carry it in the offer — do not paper over it by
re-deriving somewhere else.

### 10.3 Direction is `Mode`, and `IsLocal()` was standing in for it

The reconciler branched on `IsLocal()` — *who created the folder* —
everywhere it meant *which way bytes flow*. Origin is immutable and binary,
so direction was immutable and binary. That is the mechanical reason `both`
was inexpressible and why the only way to get bytes moving both ways was a
second, unrelated share.

`Publishes()` / `Receives()` are the readers now. `reconcileFolder`
subscribes to every peer it receives from rather than to a single origin, so
a folder we own and set to `both` pulls their changes back — without that,
`both` would still have been half a fiction.

**An absent `Mode` means the pre-S6 behaviour and never `both`.** This is the
part that nearly went wrong. The first implementation defaulted an absent
mode to `both`, and `TestReconcile_SaysNothingAboutDialingAPeerWeOnlyRECEIVEFrom`
failed — it passes a record with no `Mode`, precisely because that is what
every record already on an operator's disk looks like. That default would
have begun publishing folders an operator had only ever *accepted*, over a
grant that already existed.

The generalisation, which is worth more than the fix: **a field that gains
meaning must default to the OLD behaviour, and when the two directions of the
mistake are not equally recoverable, that decides it.** One direction quietly
sends someone else's files out of their machine; the other merely keeps doing
what the record already did.

### 10.4 One panel, and what was left out of it

§9.3's count — five panels, twenty-two buttons, eighteen verbs — is answered
by `SyncPanel`: the two gestures and nothing else. Share creates the mount
itself, because "mount" is a mechanism that leaked into the UI and is the step
no operator could explain; `accept` had already done this for the same reason.
It is the *surface* creating a mount, never the reconciler, which refuses on
purpose because that writes to somebody's disk.

The other panels are **demoted to Diagnostics, not deleted**. Each answers a
real question you reach for after something breaks. §9.3's rule — *anything
serving neither gesture belongs behind a disclosure, not in the bin* — is what
was applied.

What is **not** built, so no one reads this section as completion: a directory
picker (the field takes a typed path), an "as of" timestamp on the offer list
(offers are a remote read and a stale list currently looks live), a stop-sharing
verb on the row, and discovery in the peer chooser.

### 10.5 The refresh buttons were one missing subscription

Not listed in §9 at all, and the operator raised it first. Measured: twelve of
fifteen panels held a tree subscription and the three that did not were the
three sharing panels — the only three with Refresh buttons. `share.go` had nine
bridge exports and no `RegisterWake`.

The mechanism was never missing; seven models in `workbench/` already used
`OnPrefixChange`. Each button was locally reasonable when it was added, and the
sum was a feature the operator had to hand-crank while the rest of the
application was live. AP73 carries the general form.
