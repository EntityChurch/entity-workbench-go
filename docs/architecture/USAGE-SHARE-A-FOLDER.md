# Share a folder between two devices

The whole flow, in the order it works, with the reason for each step that
is not obvious. Everything below is exercised end to end by
`shellboot/flow_e2e_test.go` with **no wildcard grants on either side**.

> **Two things changed on 2026-09-03 and this page has not been rewritten
> around them yet.**
>
> **`-listen` now binds a socket.** It did not, in any version of
> `entity-shell` before that date: the flag was accepted, the address was
> stored on the peer, and nothing ever called `Peer.Listen` (AP67). If you
> followed this page before and step 1 appeared to do nothing, it did
> nothing. The shell now prints what it bound and what it advertised as
> its first line, so you can see it.
>
> **The relationship is durable and re-establishes itself.** `share` and
> `accept` now also record a declaration
> (`app/workbench/devices/…`, `app/workbench/folders/…`), and the shell
> re-establishes everything declared before its first prompt. So the
> `connect` steps below are needed for the FIRST contact and not on any
> later run — a restart no longer costs you the relationship. The new
> **`status`** verb shows what is declared, whether it is actually
> established, and what is stopping it; run it first when something looks
> wrong. See `docs/architecture/SHARING-DIRECTION.md`.
>
> **Accept takes a directory, so the receiver no longer mounts first.**
> `accept <peer> <root> <directory>` creates that directory, mounts it,
> authorizes their deliveries, subscribes and pulls what is already
> there. Two steps of the flow below are gone with it — the separate
> `mount` on the receiving side, and the unwritten rule that the
> receiving directory had to be *named* the same as the sender's folder.
> It refuses a directory that already has files in it, because their
> writes overwrite yours and their deletes remove yours; `-anyway`
> overrules that.

---

## The short version

On the machine that has the folder (call it **A**):

```
mount ~/notes archives/notes/
share notes with <B's peer-id>
```

On the machine that wants it (**B**):

```
peers                                  # find A, note its peer-id + address
connect a <A's host:port>
offers <A's peer-id>                   # see what A is offering you
accept <A's peer-id> notes ~/notes-from-a
```

`accept` creates `~/notes-from-a`, mounts it, and receives into it. The
directory may be called anything — it is not required to match what A
called their folder, which it used to be, silently.

**B now has A's files.** `accept` pulls whatever is already in the folder
before it returns, and reports how many files came across. That transfer
runs entirely on authority B holds, so it needs nothing further from A.

Back on **A**, once B has accepted:

```
connect b <B's host:port>
```

That last step is the one people miss. It is **not** needed for the files
above — it is what lets A tell B about *future* changes, and the rest of
this page is mostly about why it exists.

---

## Two-way: both machines edit, both converge

Everything above is **one direction** — B receives A's folder. The
Dropbox-shaped thing, where either machine can edit and both converge, is
that same flow run **once in each direction**. There is no separate
bidirectional verb, and there deliberately is not one: each direction is
an independent grant, and a single verb that established both would be
minting authority in a direction the operator did not name.

On **A**:

```
mount ~/notes archives/notes/
share notes with <B's peer-id>
```

On **B**:

```
mount ~/notes archives/notes/     # same folder NAME — see below
share notes with <A's peer-id>    # the second direction
connect a <A's host:port>
accept a notes                    # into the mount above; B now has A's files
```

Here the mount comes first *on purpose*: two-way means B publishes the
same directory back, and only `mount` establishes the publishing half.
Giving `accept` a directory is the one-way shape.

Back on **A**:

```
connect b <B's host:port>
accept b notes                    # A now has B's files
```

Both `accept`s report what came across. After the second one, both
folders hold the union of what each had.

**This is gated**, end to end through the verbs, with files seeded on
*both* sides before either sync existed:
`shellboot/sync_twoway_e2e_test.go`.

### One counter-intuitive thing you will see, which is correct

The second `accept` usually reports **more files than the other machine
started with**. By the time A accepts, B has already pulled A's files
into B's folder — so A enumerates B's folder and legitimately sees both
sets. The report reads like:

```
existing files: 4 file(s) found, 2 transferred, 2 already current
```

The *already current* half is A recognising its own bytes by content hash
and declining to pull them back. That short-circuit is what stops the two
subscriptions from feeding each other in a loop, and seeing it in the
count is the loop guard working, not a double transfer.

### Several shared folders

One mount per folder, and **the root name is the directory's basename**
(`shellcmd/mount_op.go`, `filepath.Base`). So `~/notes` is root `notes`
and `~/photos` is root `photos`, and each is shared, accepted and synced
by that name independently. Two consequences worth knowing before you
lay out directories:

- **The two machines do NOT have to use the same folder name.** `accept`
  takes a directory and you choose it; the receiving side records what it
  called the folder, and the sending side reads that when it needs it.
  This section said the opposite until 2026-09-10, and it was describing a
  real defect rather than a design: two-way delivery genuinely did depend
  on the names matching. It no longer does.
- **Two different directories with the same basename collide** on one
  machine — `~/work/notes` and `~/personal/notes` are both root `notes`.
  The mount is refused rather than silently merged.

### Two-way with `direction <folder-id> both`

**This works as of 2026-09-10, and both machines have to ask for it.**
This section previously told you not to use it, because the receiver's
writes were never carried back to the owner — that was true, measured, and
is fixed.

Direction is a property of *one folder on one machine*, so setting it on
your side means *"I will accept their changes"* and says nothing about
whether they send any. Run it on both:

```
# on the machine that owns the folder
direction <folder-id> both

# on the machine that accepted it — same folder id, both sides
direction <folder-id> both
```

Two things to expect:

- **The folder id is built from the OWNER's root name**, so if you
  accepted into a directory you named yourself, the id contains a name you
  never typed. Run `direction` with no arguments to list the ids.
- **If only one side has asked for two-way, the other side says so**, and
  names the command to run on the far machine. A quiet folder that reports
  healthy is the failure this message exists to prevent.

Two one-way shares still work and are still gated; use them if you want
the two directions to be independently revocable.

### Concurrent edits to the same file on both machines

**Since 2026-09-07 this is noticed, recorded and undoable.** It used to be
silent, which is what this section said.

The substrate is last-arrival-wins and does not merge — `DOMAIN-LOCAL-FILES`
§1.1a rules that deliberately, and it is the rsync/git-checkout model. What
changed is that the version being replaced is no longer lost to you:

```
conflicts                          # what a delivery replaced, and when
resolve <key> -keep mine           # put your version back
resolve <key> -keep theirs         # record that you looked and chose theirs
resolve <key> -keep both           # keep yours beside it as a second file
resolve --all -keep mine           # or the whole list at once
```

**The default converges and does not put a second file in your folder.**
The arriving version wins on disk, as it always has, and a durable record
names the version it replaced; `-keep mine` writes yours back and declines
that one delivery, so a catch-up pass will not undo your choice.

If you would rather have both versions **present** — the Dropbox /
Syncthing "conflicted copy" behaviour — declare it per folder:

```
conflicts -folder <folder-id> -policy keep-both
```

The replaced version is then written beside the original as
`{name}.keep-both-{8 hex}`. It is opt-in rather than the default because in
a **one-way** share it stops that folder converging and the owner is never
told: they still hold their version, you now hold both, and nothing brings
the two back together. In a two-way folder it is the better choice.

Two limits worth knowing before you rely on this:

- **Recovery rests on change recording**, which a receiving folder turns on
  automatically. A folder shared before 2026-09-07, or a path whose
  recording hit its growth budget, is reported as **not recoverable** on the
  record rather than silently offering a restore that would fail.
- **The other machine is not told.** Each side notices only what landed on
  its own edit. There is no message back to the sender, so if you and
  someone else edit the same file, only the receiving side sees a conflict
  row.

### Deletes

Deletes propagate while a sync is live; a delete that happens while the
other machine is offline does not replay when it returns.

---

## The same flow in the desktop app

Every verb above is also a button, in the **Shared Folders (share and
receive)** panel — add it from *+ Add panel → Network*. The sections are
numbered in the order you perform them.

**Including the catch-up and the clean slate.** Each received folder's row
has a **Pull now** button (`resync`) and a **Forget peer** button; the
*This peer* section has **Forget all peers**. Accept reports how many
existing files it brought across, in the panel, on the way through.

**Including the last dial.** The step that reads as `connect b <B's
addr>` above is a **Complete connection** button on the share's row, on
the machine that has to do the dialling. It resolves the address itself
from a peer you have dialled before or from an mDNS announcement; when
neither is available it shows a field for the address once and remembers
it. The panel never asks you to go and type a command somewhere else —
an instruction to use another surface is not a surface.

The panel also renders whether the connection was actually
re-established, which the verbs report and prose tends to drop: a grant
written on a live connection is inert until the next handshake.

Two things the GUI needs that the shell does not:

- **Just launch it.** Corrected 2026-09-08: this step used to prescribe
  `make gui ARGS="--identity me --storage sqlite --listen 0.0.0.0:9000"`
  and claimed the no-flag default could not share. Both halves are wrong
  now — the default has been a persistent, listening, mDNS-announcing peer
  with an on-disk store since 2026-09-03, and `--identity me` makes startup
  **fail** on any machine that has no identity called `me` (it loads an
  existing one; `--new-identity` creates).

  ```
  make gui
  ```

  Use `--listen` only to choose a different port — two peers on ONE machine
  need two ports. `--ephemeral` asks for the old outbound-only in-memory
  peer on purpose; in that configuration discovery is off and the Peer
  Connections panel says so rather than hiding the section.

- **Nothing else.** The receiver used to have to create a mount first,
  named exactly what the sender happened to call their folder. Accept now
  asks for a directory on its own row — pre-filled with a fresh path under
  `~/entity-shared/` — and creates it, mounts it and receives into it. It
  refuses a directory that already has files in it, because their writes
  overwrite yours and their deletes remove yours, and offers to proceed
  anyway with that consequence stated.

The panel is a thin surface over `ShellWorkspace.Share`/`Accept`/`Sync` —
the same methods the verbs call (`shellcmd/share_op.go`,
`shellcmd/sync_op.go`). There is no second implementation of the flow, so
the rest of this page describes both routes.

**When it does not work, the panel to open is *Sharing Status (declared
vs. actual)*** — the `status` verb's question, in the same Network
category. It lists every peer and folder you have declared beside what is
actually established, and names what is stopping the rest. Two things on
it are worth knowing before you need them:

- **"Re-check now" is not "Refresh".** Re-check runs one pass of the
  control loop: it reconnects to declared peers, writes any authorization
  your declarations require, and subscribes to accepted folders that have
  a mount. Refresh only re-reads. The panel captions which of the two you
  are looking at, because "these are the records" and "this is what a pass
  established" are different claims.
- **It tells you what you grant a peer, and it will not pretend to know
  what they grant you.** Your side is your own capability row, so it is
  printed exactly. Their side is in *their* table, which your machine
  cannot read — so instead of a green light you get the only evidence that
  exists: how many files have actually arrived in the folder. If that
  number is zero and everything else looks established, the missing step
  is almost always on the other machine.

A folder whose mount has gone gets a **Remount** button there, which
bridges the directory the declaration remembers. The control loop
deliberately will not do that for you: creating a mount writes to a
directory on your disk, and that stays your decision.

---

## Why A has to dial B at the end

**A dial-by-address authorizes the dialer only.** This is deliberate in
the protocol, not an oversight in the product:
`Connection.sendReciprocalGrant` is gated on
`EstablishedViaRendezvousKey()`, and the kernel's comment gives the
reason — *"a dial-by-address is asymmetric — one party requested service
— and §6.6's one-directional mint stands alone there."*

So when B dials A, B gains authority to originate to A. A gains nothing.

And a sync runs in **both** directions:

| direction | who dispatches | what it needs |
|---|---|---|
| subscribe to the folder | B → A | A's grant to B (`share` writes it) |
| fetch each file's blocks | B → A | same |
| deliver "a file changed" | **A → B** | **B's grant to A** (`accept` writes it) |

`accept` writes B's half. It cannot make A dial, and until A dials, A's
delivery notifications are refused. **The symptom is silence**: B's
subscription is accepted, B's folder stays empty, and nothing on B's
machine reports an error — the refusal happens on A.

`accept` prints the exact command A must run, for this reason.

---

## Why the order matters

A grant is assembled during the **handshake**. Writing a policy on a live
connection changes nothing until that connection is re-established.

Two consequences:

- **Sharing before connecting is the simplest order.** If A shares with B
  before B ever dials, B's first handshake already carries the grant and
  no reconnect is needed anywhere.
- **The peer that dispatches is the peer that must reconnect.** A
  reconnect performed by the granter does not help the grantee: the
  grantee dispatches over the connection *it* opened, which still carries
  whatever it was granted at the time. `offers` and `accept` therefore
  re-establish B's own connection before they use it.

---

## What each stage actually writes

| stage | where it lands | what it is |
|---|---|---|
| `share` | `system/capability/policy/{B}` | the **permission**: subscription, content:get, local/files:read, tree:get |
| `share` | `app/share/records/{root}` | the **label** — what is on offer, and to whom |
| `accept` | `system/capability/policy/{A}` | the delivery permission: `workbench/blob-resolve:receive` |
| `accept` | `app/workbench/syncs/{A}.{root}` | the sync binding, restored at startup |
| `mount` | `system/config/local/files/{root}` + `app/workbench/mounts/{root}` | the folder bridge, both halves |

`accept` and `sync` also run a **catch-up pass** before returning: every
file already under A's `local/files/{root}/` is pulled across and written
into B's mount. It goes through the same
`workbench/blob-resolve:receive` handler the live deliveries use — the
notification is synthesized, nothing downstream is duplicated — so the
subscription and the catch-up cannot drift apart.

Without it, sharing a folder that already had files in it transferred
nothing at all and reported success, because a subscription is a future
tense and no file in that folder ever "changed" again.

The offer record is a **label, not an authority** —
`APP-CONVENTION-SHARE` §2.2 is explicit that a consumer must not infer
authorization from it. If you see an offer and still get a 403, that is
the two things being correctly separate, not a bug.

---

## Checking and undoing

```
access          # every peer authorized on this machine, and for what
shares          # folders this machine offers, and to whom
syncs           # folders this machine is receiving
resync a notes  # pull their current folder NOW, without waiting for a change
unshare notes <B's peer-id>
forget a        # drop every relationship with one peer
forget --all    # ... with every peer
```

`resync` is also the answer to *"did it actually work?"*. Run it twice:
the second pass reports everything as **already current**, which is a
positive confirmation. Without it, "no errors" and "nothing happened"
render identically, which is the state this flow spent a week in.

`forget` exists because everything the flow establishes is deliberately
durable — the policy survives a restart, the sync binding is restored at
startup, the alias and the discovered candidate persist. That is correct
individually and it makes the flow impossible to re-test: a second run
cannot be told apart from a stale grant that was already in place, and
that failure presents as a **success you cannot trust**. It drops
relationships only: **no file is ever deleted and nothing is unmounted**,
and it does not change this peer's identity. Run it on **both** machines
— a peer cannot reach into another peer's tree, so one side forgetting
leaves the other still remembering.

`access` lists one row that is **this peer itself** — a wildcard entry the
kernel seeds at construction (V7 v7.74 §6.9a). It is marked as such. It is
not a grant to anyone else, and hiding it would be concealing a real
grant, so it is shown and labelled.

`unshare` removes the policy entry, which means **the next handshake will
not carry the grant**. It does not reach into a connection that already
holds one. It also leaves every file that already arrived exactly where it
is — those are the other machine's files now.

---

## When it does not work

**B's folder is empty and nothing is wrong on B.** First run `resync b
<root>` on B — that pulls A's current folder on B's own authority and
needs nothing from A. If files appear, the relationship is fine and only
*change notification* is missing: A has not dialled B since B accepted,
so run `connect` on A.

If `resync` also brings nothing, the grant has not reached B — see the
403 row below.

**`offers` shows nothing.** The offer names a specific audience. Check A's
`share` used B's peer-id, and that B is connected.

**`sync`/`accept` is refused 403.** A's grant has not reached B yet —
either A never shared, or the connection predates the share and B has not
reconnected.

**`peers` is empty.** Discovery announces on the listener's scheme, so a
peer started without `-listen` is not announcing. mDNS is also link-local:
peers on different networks never appear and must be reached with
`connect <alias> <host:port>`.

---

## What this does not cover yet

- **Cross-network.** mDNS is link-local and `ext/relay` is unlanded
  upstream, so today this is one LAN or manual addressing.
- **A merge.** A concurrent edit is detected, recorded and undoable (above),
  and it is never three-way merged. `EXTENSION-REVISION` is where merge
  semantics belong and nothing at this tier attempts them.
- **Telling the other machine a conflict happened.** Each side sees only
  what landed on its own edit.
- **More than three machines**, and any topology with a cycle. Three is the
  most that has been run (`make threepeer-sync`: one folder to two
  receivers, and a two-hop chain).
- **A folder whose files depend on each other** — see the warning below.
  This is the important one.

> **`direction … both` used to be on this list and is not any more** (fixed
> 2026-09-10). One folder record carries two-way now, and the two machines do
> not have to name the directory the same thing. **Both sides must declare
> `both`**: direction is per-peer, and a receive-only counterpart publishes
> nothing and grants no sender authority, so the owner's subscribe answers
> 403. The verb says so on the machine you ran it on, and names the command
> to run on the other one.

## Do not share a folder that contains a live repository

**A shared folder converges one file at a time. There is no notion of a set of
files that must arrive together**, and that is the whole answer to why this
class of tool has always misbehaved over a `.git` directory, a database, or
anything else with strict inter-file invariants.

The failure is not about volume and it is not a bug we have yet to find. A
repository's index, its refs and its object files are only meaningful *with
respect to each other*. Propagate them independently — which is exactly what a
per-file replication stream does — and there is a window on the receiving side
where the set is internally inconsistent. A half-arrived repository is worse
than either version alone, because both machines now believe they have one.

The same reasoning covers a live SQLite database (the `-wal` sidecar and the
main file are one object), an application's profile directory, and a virtual
machine image being written to.

**What to do instead:** share the working files and let the repository's own
tooling move the repository — that is what a remote is for. If you want the
history on both machines, push and pull; if you want the files on both
machines, share the files.

**This is a stated limitation, not a diagnosis of your specific problem.** We
have not measured the failure in this product — what we have measured is that
nothing in the design prevents it. If you do point a share at a repository and
it survives, that is luck about timing, not a guarantee, and it will not
survive a burst.

The GUI covers all of this now — the **Sync** panel is the two-gesture front
door, and *Shared Folders* / *Sharing Status* are the diagnostics. That row
used to say there was no GUI surface at all.
