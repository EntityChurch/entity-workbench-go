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
make gui ARGS="--identity me --storage sqlite --listen 0.0.0.0:9110"
```

`make gui` rebuilds the image (do this after any code change);
`make gui-run` launches what is already extracted. **With no `--listen`
nothing can dial you**, and a share is a dial.

Add panels with **+ Add panel → Network**:

| Panel | The question it answers |
|---|---|
| **Peer Connections** | who am I connected to right now |
| **Shared Folders (share and receive)** | the flow: offer, see offers, accept |
| **Sharing Status (declared vs. actual)** | is what I declared actually working, and what is stopping it |

The flow lives in **Shared Folders**, whose sections are numbered in the
order you perform them. Accept asks for a directory on the offer row,
pre-filled with a fresh path under `~/entity-shared/`; it refuses a
directory that already has files in it, and offers to proceed anyway with
the consequence stated (their writes overwrite yours, their deletes
remove yours).

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

### 6.1 The first change after a restart is lost

**Measured, reproducible, and the one that will bite you.** After either
peer restarts, the **first** file you change is not delivered. Every
change after it is:

```
restart peer A
  write w1.txt  ->  lost
  write w2.txt  ->  delivered
  write w3.txt  ->  delivered
```

Four single-write restart cycles alternated fail / pass / fail / pass,
which is the same fact seen once per cycle. It happens on a **receiver**
restart too, identically.

Two hypotheses were tested and **both refuted by measurement**: it is not
a missing outbound dial (the loop now opens one at every start and says
so), and it is not a stale entry in our own connection pool (evicting
before dialing changed nothing, so that change was reverted rather than
kept as a plausible-looking no-op). The cause is not yet known and is
being routed rather than guessed at.

**The workaround is reliable:** `resync <peer> <root>` on the receiver, or
**Pull now** on the folder's row in the GUI. It re-pulls the current
state and reports what it moved.

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

### 6.4 A concurrent edit still loses a write, silently

Two peers editing the same file at the same time is not merged and not
reported. `../docs/architecture/FILE-REPLICATION-LANDSCAPE.md` §3 is the
discussion. This is the gap that decides whether this becomes a tool
people install, and nothing here improves it.

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

**Where it does not, yet.** §6.1 is disqualifying for unattended use: a
sync tool whose first post-restart change is silently dropped is not
something to trust a directory to, and Syncthing's answer here — a scan
plus a rescan interval — means it converges without anyone asking. We
converge only when someone runs `resync`. §6.4 is the deeper one:
Syncthing detects conflicts and keeps both sides as
`.sync-conflict-…` files; we lose a write with no record. And Dropbox's
whole advantage — a third party that is always up, so two devices never
have to be awake together — is not available to us by construction, which
is a design choice rather than a defect but changes what "it just works"
can mean.

**So: good enough to use deliberately, not yet good enough to forget
about.** Drive it, watch `status`, and use `resync` after a restart.
