# Sharing a folder — run it, test it, validate it

A practical guide to the two-machine folder share, for someone who wants
to **try it and find out whether it works**. Every command here was run,
in order, on 2026-09-03, and the outputs are transcribed from that run
rather than composed.

- The **operator recipe** (both routes, all the reasoning) is
  `../docs/architecture/USAGE-SHARE-A-FOLDER.md`.
- The **design** — why this is a control loop and not a wizard — is
  `../docs/architecture/SHARING-DIRECTION.md`.
- This file is the **how do I actually drive it** layer, plus §6, which
  is an honest list of what is still wrong.

---

## 0. The shape, in one paragraph

Two objects are declared: a **device** (a peer you have paired with) and
a **folder** (a directory, and who it is shared with). Everything else —
the authorization row, the mount, the subscription, the connection — is
*derived* by a **reconciler** that runs at every start, after every
change, and whenever you ask. So there is no correct order to do things
in, and a restart is just the reconciler running again.

Two gestures: **share a folder with a peer**, and **accept a folder
someone offered you**. That is the whole model. It is Syncthing's shape,
not Dropbox's — there is no server, so both sides approve once, and that
approval is the thing that is durable.

---

## 1. The fastest way to see it work (one machine, two peers)

You do not need two computers to test this. Two shells with separate
`HOME`s are two genuinely different peers.

**Terminal 1 — peer A, the one that shares:**

```bash
make build                     # bin/ is build output, not the tree. Rebuild first, always.
mkdir -p /tmp/peerA/shared
echo "hello from A" > /tmp/peerA/shared/notes.txt
HOME=/tmp/peerA ./bin/entity-shell -storage sqlite -alias a -listen 127.0.0.1:9401
```

**Terminal 2 — peer B, the one that receives:**

```bash
mkdir -p /tmp/peerB
HOME=/tmp/peerB ./bin/entity-shell -storage sqlite -alias b -listen 127.0.0.1:9402
```

Both print their identity on the first line:

```
entity-shell: listening on 127.0.0.1:9401; dialable at tcp://127.0.0.1:9401
Local peer: a (peer-id 2K6DQNP9jEPU...)
```

Get the **full** peer-ids — you need them, and the startup line is
truncated. In each shell:

```
info
```

```
Alias:   a
Address: (self)
PeerID:  2K6DQNP9jEPUa9wvfmMn9hYkQXVUXP6VxsPxxu65M3BeVV
```

### The four commands

**On A** — bridge the directory, then offer it:

```
mount /tmp/peerA/shared archives/shared/
share shared with <B's full peer-id>
```

**On B** — dial A once, then accept into a directory of your choosing:

```
connect a 127.0.0.1:9401
accept <A's full peer-id> shared /tmp/peerB/from-a
```

Accept prints what it did, and the line that matters is the last one:

```
accepted shared from a (2K6DQNP9jEPUa9wvfmMn9hYkQXVUXP6VxsPxxu65M3BeVV)
  files land in: /tmp/peerB/from-a (mounted as "from-a")
  granted them: workbench/blob-resolve:receive
  policy:       system/capability/policy/2K6DQNP9jEPU…
  their prefix: local/files/shared/
  our prefix:   local/files/from-a/
  subscription: sub-1788487213124220603
  the connection was re-established, so the grant is in force now.

existing files: 1 file(s) found, 1 transferred

ONE STEP LEFT, AND IT IS ON THEIR MACHINE:
    connect <B's peer-id> <this-peer's host:port>   (run on <A's peer-id>)
```

**`1 transferred` is the whole point.** Files that were already in the
folder come across on accept — you do not have to touch them to make them
move. And note the two names: their `shared` lands in your `from-a`, and
nothing had to be called the same thing on both machines.

**On A** — dial back, once. `share` and `accept` both print this
instruction, naming the machine to run it on:

```
connect b 127.0.0.1:9402
```

That last step is not ceremony. A dial-by-address authorizes the *dialer*
only, so A must open its own connection to B before A's future changes
can be delivered to B. Accept's output says so explicitly, naming the
command and the machine to run it on.

### Now watch it sync

```bash
echo "written live" > /tmp/peerA/shared/live.txt
sleep 5
ls /tmp/peerB/from-a/
```

```
live.txt
notes.txt
```

That is the flow working. Measured end to end, real bytes, correct
content, into a directory named nothing like the sender's.

---

## 2. The same thing in the desktop app

```bash
make gui
```

**That is the whole command. Corrected 2026-09-08 — this line used to read
`make gui ARGS="--identity me --storage sqlite --listen 0.0.0.0:9110"`, and
the first of those three flags makes startup FAIL.**

`--identity NAME` means *use the EXISTING identity called NAME*, and it
refuses if there is none — deliberately, because a peer-id is what every
grant, mount and offer on the other machine names, so a typo must not
quietly become a different peer. On a machine that has never run this,
there is no identity called `me`, so the documented first command could not
work on a first run. `--new-identity NAME` is the flag that creates one.

The other two flags were noise: `sqlite` is already the default storage,
and `0.0.0.0:9110` is already the default listen address. The program's own
`--help` says so plainly — *"By default this is a PERSISTENT, REACHABLE
peer: the same peer-id every launch, an on-disk store, an inbound listener,
and an mDNS announcement so peers on your LAN can find it without being
told an address"* — and this file spent that sentence's whole meaning on
three flags that re-stated two defaults and broke the third.

`make gui` rebuilds the image (do this after any code change);
`make gui-run` launches what is already extracted. Use `--new-identity
NAME` only if you deliberately want a *second, different* peer on the same
machine.

Add panels with **+ Add panel**. Only two of them are under **Network**,
and that is deliberate — the rest were demoted to **Diagnostics** because
five panels had grown around this one job and the sum was unusable.

| Panel | Where | The question it answers |
|---|---|---|
| **Sync — share a folder with a peer** | Network | **the flow. Start here.** |
| **Peer Connections** | Network | who am I connected to right now |
| **Sharing Status (declared vs. actual)** | Diagnostics | is what I declared actually working, and what is stopping it |
| **Shared Folders (every control, one stage at a time)** | Diagnostics | the per-stage controls — resync, forget, unsync |

**Corrected 2026-09-08: this table used to send you to Shared Folders and
did not mention Sync at all.** Sync is the front door and has been since
2026-09-04; Shared Folders is what you open when a share will not
establish and you need to drive one stage at a time.

**Sync is two gestures and nothing else.** *Pick a folder → pick a peer →
Share.* And, when a card appears: *pick a directory → Accept.* Share
creates the mount for you — you never say "mount", because that is
mechanism and it had leaked into the UI.

Accept asks for a directory on the offer row, pre-filled with a fresh path
under `~/entity-shared/`. **Check that box actually holds what you typed
before you press Accept** — a harness run on 2026-09-08 typed a path,
reported success, and the app accepted into the pre-filled default; the
driver could not tell whether the app ignored the input or the input never
arrived, and the same ambiguity is available to you at a keyboard. Accept
refuses a directory that already has files in it, and offers to proceed
anyway with the consequence stated (their writes overwrite yours, their
deletes remove yours).

### The flow on a real LAN, and what you do NOT have to do

**You do not type an address.** The peer picker in Share is the *reachable*
peers — the connection pool **unioned with mDNS discovery** — and each row
says which of the two it came from, because a connection is a fact and an
announcement is an advertisement. On one LAN, with both machines launched
as above, the other machine appears by itself.

**You do not reconnect after sharing, either.** A grant is assembled at the
handshake, so a policy written on a live connection is inert until that
connection is replaced — but replacing it is the **reconciler's** job, not
yours: it tracks which peers had a policy change on this pass and re-dials
exactly those (`shellcmd/reconcile.go`, `refreshGrantConnection`), and it
opens this peer's own outbound route to every declared device. Earlier
versions of this guide told you to press Connect again afterwards. That was
this project describing its own test harness, which dials explicitly for
determinism, as though it were the operator's flow.

That the *mechanism* costs a reconnect at all is a design smell and is
routed as such (`reviews/LIVE-GRANT-REFRESH-2026-09-08.md`): a permission
change should not require destroying a working connection, and it should
certainly never surface as an instruction.

**One of these WAS measured on 2026-09-08, and discovery did not carry it —
because of a defect, now fixed.** This section used to list it as merely
unverified; here is what the first real two-machine run found.

Discovery announced correctly and the Peer Connections panel showed the
other machine the whole time. But three consumers that needed a peer-id
from an announcement read `CandidateData.PeerID`, which is **empty on
every mDNS candidate** — the claimed id travels in a `peer_id_hint` TXT
key, and the field is only filled in by an IDENTIFY step nothing calls. So
the reconciler could never learn an address from discovery, and the shell's
`peers` verb had never listed a discovered peer at all. The visible cost
was an app dialling a port the other machine had moved off, for 45
minutes, while that machine announced its real one on the LAN.

Fixed 2026-09-09 (`entitysdk.CandidatePeerID`, and a dial ladder that tries
the announced address first and writes back whichever answers). **Discovery
alone should now carry the flow with no address ever typed — and that is a
claim from one operator session plus unit gates, not from a harness**, so
it is the thing to check hardest on the next real run.

Still genuinely NOT measured:

- whether the publisher's dial is still needed at all, or whether the
  reconciler's outbound route covers it, is unverified **because every
  harness performs that dial** — a stage a fixture always performs is a
  stage with no coverage, the same shape as a fixture that always disables
  one.

If the flow works for you without touching Peer Connections at all, that is
the answer to both, and it is worth writing down.

**Sharing Status** is where you go when something is wrong. Two things
about it are worth knowing before you need them:

- **"Re-check now" is not "Refresh."** Re-check runs one pass of the
  control loop — it reconnects, writes any authorization your
  declarations require, and subscribes accepted folders that have a
  mount. Refresh only re-reads. The panel captions which of the two you
  are looking at, because *"these are the records"* and *"this is what a
  pass established"* are different claims.
- **It states what you grant a peer and refuses to guess what they grant
  you.** Your side is your own capability row, so it is exact. Their side
  is in *their* table, which your machine cannot read. So instead of a
  green light you get the only evidence that exists: how many files have
  actually arrived. If that number is zero and everything else looks
  established, the missing step is on the other machine.

A folder whose mount has gone gets a **Remount** button. The control loop
deliberately will not do that for you — creating a mount writes to a
directory on your disk, and that stays your decision.

---

## 3. How to check it is working

**`status`, in either shell.** It runs a real reconcile pass and then
reports, so what it says is verified rather than remembered:

```
peers: 1
  connected    127.0.0.1:9402             2KHHdEbxFL…
               you grant: local/files:read  system/content:get  system/subscription:*  system/tree:get

folders: 1
  shared out mounted                shared
               2 file(s) on disk, 2 readable as documents
               2KHHdEbxFLoBGgrTWRJZkdJdyHoL8Tp23whNYYxhWWD1R3 (offered)

settled — everything declared is established.
```

A pass that changed something lists it after the tables, in place of `settled`:

```
this pass changed:
  2KHHdEbxFL…: opened our outbound connection to 127.0.0.1:9402 so deliveries can reach them
```

Read it like this:

| Line | Means |
|---|---|
| `connected 127.0.0.1:9402` | we hold a route and we know an address to re-dial after a restart |
| `connected (no address; by peer-id only)` | working now, and **nothing will re-dial it later** — dial them once so the address is recorded |
| `you grant: …` | exactly what your capability row lets them do. There is no inbound counterpart, on purpose |
| `N on disk, M readable as documents` | both sides of the mount. `M` lower than `N` means files are bridged but not yet ingested |
| `NO MOUNT` | the folder is declared and there is nowhere for its bytes to live |
| `settled` | this pass changed nothing and found nothing wrong |

`status` reports **relationships**, not mounts. A peer with a mounted
folder it has never shared correctly prints *"nothing declared on this
peer"* — use `mounts` for that. Declaring is what `share` and `accept` do.

**The end-to-end check that cannot lie** is the filesystem:

```bash
ls -la /tmp/peerB/from-a/ && cat /tmp/peerB/from-a/notes.txt
```

**`resync` is the positive confirmation.** On the receiver:

```
resync <A's peer-id> shared
```

It re-pulls the folder's current state and tells you what it moved. On a
folder that has fallen behind:

```
existing files: 4 file(s) found, 1 transferred, 3 already current
```

and on one that is up to date:

```
re-pulled shared from 2K6DQNP9jEPUa9wvfmMn9hYkQXVUXP6VxsPxxu65M3BeVV
existing files: 2 file(s) found, 2 already current
```

**`already current` is the point.** It is the only signal this flow offers
that distinguishes *"it worked"* from *"nothing happened"* — without it,
success and silence render identically. It is also the recovery for §6.1,
so it is worth running after any restart.

---

## 4. Re-testing from a clean slate

Everything the flow establishes is durable on purpose, which means a
second run cannot be told apart from a stale grant that was already
there. That failure presents as a **success you cannot trust**, so there
is a verb for it:

```
forget <their-peer-id>     # drop syncs, offers, authorization, connection
forget --all
```

It **deletes no files and unmounts nothing** — received bytes are yours —
and it does not touch identity. **Run it on both machines**; a peer
cannot reach into another peer's tree, so forgetting on one side leaves
the other still remembering you. In the GUI: **Forget peer** per received
folder, **Forget all peers** under *This peer*.

For a genuinely clean test, throw the homes away:

```bash
rm -rf /tmp/peerA /tmp/peerB
```

---

## 5. Running the test suites

Serially. **Never both at once** — both bind-mount the tree with `:Z`,
and the second relabel revokes the first container's access mid-run, so
every suite after the first reports a permission error while its own log
says `ok`.

```bash
make test-each              # all ten Go suites to completion (~12 min), logs in .test-logs/
make -C avalonia test       # the headless UI suite (~7 min)
make lint                   # go vet
make reachability           # every bridge export has a consumer; every model a surface
make build                  # all shipped binaries
```

`make test` stops at the first failing package, so a count from a red
`make test` covers one package and is a lower bound. `make test-each`
exists so the correct thing is also the easy thing.

The gates specific to this feature:

```bash
make test-shellcmd ARGS="-run 'TestStatus|TestReconcile|TestRemount|TestSetDevicePaused' -v"
make -C avalonia test        # SharingStatusPanelTests is in here
```

---

## 6. What is still wrong — read this before you conclude it is broken

### 6.1 The first change after a restart is late — not lost

**Corrected 2026-09-08. This section used to say the change was lost and
that only `resync` recovered it. Both halves were wrong**, and they were
written before the catch-up supervisor existed.

**What is true:** after either peer restarts, the **first** file you change
is not delivered *live*. Every change after it is:

```
restart peer A
  write w1.txt  ->  lost
  write w2.txt  ->  delivered
  write w3.txt  ->  delivered
```

Four single-write restart cycles alternated fail / pass / fail / pass,
which is the same fact seen once per cycle. It happens on a **receiver**
restart too, identically.

**And then it arrives by itself.** Measured: **~107 seconds** after it was
written, with no `resync` and nobody touching anything. The catch-up
supervisor takes a pass at startup and then doubles its wait after every
pass that finds nothing, so from a restart the passes land at roughly
t=0, t≈120s, t≈360s — and every test window in this project was 90
seconds, which expires between the first two. That is how a delay got
written down as a loss, in this file and five others.

**No bound is promised.** The interval grows with idle time to a ten-minute
ceiling, so "a couple of minutes" is what you should expect and not what
you are owed.

**If you do not want to wait:** `resync <peer> <root>` on the receiver, or
**Pull now** on the folder's row in the GUI. It re-pulls the current state
and reports what it moved. This is now a way to *hurry* recovery rather
than the only route to it.

Two hypotheses for the live miss were tested and **both refuted by
measurement**: it is not a missing outbound dial (the loop now opens one at
every start and says so), and it is not a stale entry in our own connection
pool (evicting before dialing changed nothing, so that change was reverted
rather than kept as a plausible-looking no-op). The cause is not yet known
and is being routed rather than guessed at.

### 6.2 A folder can be mounted and publish nothing openable

If `status` says `N file(s) on disk, 0 readable as documents`, the files
are bridged into the tree but were never turned into documents. It happens
when a mount is created by a process that exits straight afterwards: the
ingest is asynchronous and nothing re-runs it on the next launch.

**Sharing still works** — the transfer moves the bridged layer, which is
why the walkthrough in §1 succeeds regardless. What you lose is the
ability to *open* those files on that peer.

The fix is to change the files with the app running; the watcher picks the
change up and ingests. Verified: a file left at `0 readable` gained its
document the moment it was modified in a live session. **There is no
button for it** — the Local Files panel's *Sweep* is the opposite
direction (it removes tree entries whose file is gone from disk), so do
not reach for it expecting this.

### 6.3 Both sides must dial once, and the prompt has to be relayed

This one is better than it looks and still not good. `share` tells the
sending operator when the grant is not yet in force and names the
`connect` command. `accept` tells the receiving operator, under **ONE STEP
LEFT, AND IT IS ON THEIR MACHINE**, the exact command and the peer to run
it on.

But those are two different people at two different keyboards, and the
second message is addressed to someone who is not reading it. Nothing
reaches the sending operator *after* the accept to say "they are waiting
on you now". On a LAN with discovery the addresses resolve themselves and
this mostly disappears; on loopback or across subnets it does not, and the
symptom is a folder that received its existing files and then went quiet.

### 6.4 A concurrent edit is not merged, and the other machine is not told

**Corrected 2026-09-08. This section used to say a concurrent edit loses a
write silently, and since 2026-09-07 that is no longer true.** A delivery
that lands on a file you edited is detected, the version it replaced is
recorded, and the *Sharing Status* panel lists it with **Restore mine** /
**Keep theirs** / **Keep both** on the row. The shell verbs are `conflicts`
and `resolve`.

What is still true, and is the remaining gap:

- **Nothing is merged.** The arriving version wins on disk and yours is
  recoverable; there is no three-way merge and none is planned at this tier.
- **The other machine is not told.** Each side notices only what landed on
  its own edit. In a one-way share the owner never learns their change
  replaced somebody's work.
- **A folder can be declared `keep-both`** (`conflicts -folder <id> -policy
  keep-both`), which writes your version beside theirs as
  `{name}.keep-both-{8 hex}` — the Syncthing/Dropbox shape. It is opt-in
  rather than the default because in a one-way share it stops that folder
  converging and the owner is never told, so the two machines diverge
  permanently. In a two-way folder it is the better setting, and the sibling
  is an ordinary file so it replicates to the other machine by itself.

`../docs/architecture/SYNC-LIMITS-AND-FAILURE-MODES.md` §5 is the full
discussion.

### 6.5 Offers are pulled, not pushed

"Device X wants to share folder Y" does not appear by itself — you press
**Check their offers**. A push feed needs a grant we do not currently ask
for; a poll would look like a hang whenever a device is asleep.

---

## 7. Honestly, against Syncthing and Dropbox

Worth stating plainly, because "as good as Syncthing" is the bar.

**Where this matches.** The relationship model is the right one and is
now genuinely order-free: share before the peer is reachable, accept
while the other side is offline, restart either machine — the loop closes
when the facts are present. Accept takes a directory and creates it.
Files already in the folder come across. Both sides approve once, and the
approval is durable. `status` answers *"is it working"* in one command,
which is more than Syncthing's UI gives you in one place.

**Where it does not, yet.** §6.1 is the one you will notice: the first
change after a restart misses live delivery, cause still unknown.
Syncthing's answer — a scan plus a rescan interval — converges without
anyone asking; **we have the same shape** in the catch-up supervisor, and
it is now measured doing exactly that (~107s, unattended). So the gap
against Syncthing here is latency, not durability. And Dropbox's whole advantage — a third party that is always up,
so two devices never have to be awake together — is not available to us by
construction, which is a design choice rather than a defect but changes
what "it just works" can mean.

**§6.4 has moved, and it is worth being precise about where.** Syncthing
detects a conflict and keeps both sides as `.sync-conflict-…` files. We
detect it, keep both — one on disk and one addressable on the chain — and
will write the sibling too if the folder asks for it, using the same naming
the revision extension specifies. Where Syncthing is still ahead is that
**both peers find out**; ours is a local notice. That is now the gap, and it
is a smaller one than "we lose a write with no record", which is what this
section said until 2026-09-08.

**So: good enough to use deliberately, not yet good enough to forget
about.** Drive it, watch `status`, and run `resync` after a restart rather
than waiting for the supervisor.
