# Share a folder between two devices

The whole flow, in the order it works, with the reason for each step that
is not obvious. Everything below is exercised end to end by
`shellboot/flow_e2e_test.go` with **no wildcard grants on either side**.

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
mount ~/notes-from-a archives/notes/   # a sync writes INTO a mount
accept <A's peer-id> notes
```

Back on **A**, once B has accepted:

```
connect b <B's host:port>
```

That last step is the one people miss, and the rest of this page is
mostly about why it exists.

---

## The same flow in the desktop app

Every verb above is also a button, in the **Shared Folders (share and
receive)** panel — add it from *+ Add panel → Network*. The sections are
numbered in the order you perform them.

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

- **Launch both peers with a listener**, or there is nothing to dial and
  no mDNS announcement:

  ```
  make gui ARGS="--identity me --storage sqlite --listen 0.0.0.0:9000"
  ```

  With no `--listen` the app is an outbound-only, in-memory peer. It is a
  fine default for looking around and cannot participate in a share.
  Discovery is off in that configuration and the Peer Connections panel
  says so rather than hiding the section.

- **Mount on the receiving side too, before you press Accept.** A sync
  writes into a mount and does not create one — same refusal as `sync`,
  for the reason in the next section. Use a *Local Files (manage mounts)*
  panel; Accept names that panel in its error when the mount is missing.

The panel is a thin surface over `ShellWorkspace.Share`/`Accept`/`Sync` —
the same methods the verbs call (`shellcmd/share_op.go`,
`shellcmd/sync_op.go`). There is no second implementation of the flow, so
the rest of this page describes both routes.

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
unshare notes <B's peer-id>
```

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

**B's folder is empty and nothing is wrong on B.** Almost always the last
step: A has not dialled B since B accepted. Run `connect` on A.

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
- **A GUI surface.** These are shell verbs. The Local Files panel manages
  mounts and says nothing about shares or syncs.
- **Concurrent edits to one file on two machines.** Not covered, and not
  safe to assume: the substrate is last-arrival-wins and does not merge.
  That is milestone M3 in `FILE-REPLICATION-LANDSCAPE.md`, and it is the
  one that needs `ext/revision` composed in.
- **History replay.** A sync delivers changes from the moment it is
  established. Files already sitting in A's folder arrive when they next
  change, or when A remounts.
