# Live-peer direction — reading a site from the machine that wrote it

**Status:** direction, living. Edit in place.
**Written:** 2026-09-10, after a session that measured the consume path end to end against the live
federation and found it strong, session-scoped, and pointed entirely at static origins.

---

## 1. The problem with what we are currently good at

The consume journey works. Measured today, one REPL session, live network:

```
registry pin https://entitychurchregistry.org   → TOFU pin, labelled as origin-nominated, layout re-based
registry ls                                     → 5 names from the WALK of a signed root
open billslab.com                               → 10 chain steps, 51 CHAMP nodes, 966 committed keys, the page
```

Every step of that is HTTP GET against a static origin. `fetch.Layout` is built from an **http-poll**
transport profile; `fetch.Registry.OriginFor` refuses a binding that commits to no usable http-poll
transport (`fetch/registry.go:1147`). There is one `ContentResolver` implementation for remote
content and it is constructed from a completed static walk (`workbench/remote_site_resolver.go`).

**So what this application does with a published site is read what somebody else emitted, and check
it.** That is worth building — a second independent reader is a large part of what makes a format
real — but it is the smaller half of what this program is in a position to do, because this program
is not a document viewer. It is a peer: it listens, it is discoverable on a local network, it holds
capabilities, it dispatches, it runs a subscription engine and a content store.

Everything below is an obligation in the published content conventions that **cannot be exercised
without a second live participant** — and a static origin is not one.

## 2. The two modes, side by side

| | **Static origin** | **Live peer** |
|---|---|---|
| A site is read from | a host serving files over HTTP, against a signed root | the peer that authored it, over an authenticated connection |
| A reference resolves against | a frozen emission | a tree that can change underneath the reader |
| Following someone is | polling an origin on a timer | a subscription that delivers |
| A reply reaches an author | not at all — nothing carries it | over a delivery grant the author issued |
| A second opinion comes from | another origin, if one happens to exist | any peer that kept a copy |
| Publishing is | emit a directory and upload it | serve it, from the machine that wrote it |
| Reaches | anything with a network connection | a local network, a peer with an address, a rendezvous |

Neither column is the real one. The conventions are written so both work, and a format that only
works in one of them is not finished. **What this program has never done is the right-hand
column.**

## 3. The five obligations only a live peer can reach

Each of these is published normative text, and each is unexercised here. The claim that no
implementation anywhere has exercised them is a reading of the published corpus rather than a
measurement, and is stated as such.

**(1) `FEED` §2.2.2 rows 2, 3 and 4 — and `FEED-R7` is a MUST.**
A live reference (`{tag:"live", peer, path, ?seen}`) has four resolution outcomes, and *"a reader
MUST be able to tell which one it got"*. Row 2 is *the path resolved and the hash differs* — the
document evolved. Row 3 is *the path 404s, fall back to `seen` fetched from anywhere*. **Against a
frozen fixture rows 2 and 3 cannot occur.** A publisher who never changes cannot produce a
mismatch, and a static bundle cannot 404 a key it committed to. Two live peers produce all four on
demand: edit the target, unpublish the target, both. This is the highest-value thing on the list
because it is a MUST that is structurally untestable in the other column.

**(2) `FEED` §3 — reply delivery is grant-scoped, and a grant needs somebody to issue it.**
*"A reply published in the replier's own namespace produces no notification anywhere by itself…
Inbound delivery is granted, not ambient."* The spec names two honest routes: the author **polls**
for entries whose `reply.root` names one of theirs, or **the replier holds a delivery grant the
author issued**. The second route is the capability machinery this application already runs —
`system/capability/policy/{peer}`, assembled at handshake, and mutual by construction, because a
one-directional grant gives you a relationship that establishes cleanly and carries nothing.
`FEED-R9` (MUST NOT describe replies as notifying the author absent a grant) is a rule an
implementation is most likely to get right after having tried the alternative.

**(3) `app/feed/follow` as a subscription, not a poll.**
The type is *"a reader's durable subscription to a peer's feed"*. We have a subscription engine, a
delivery path, a restart-safe binding layer, an adaptive catch-up supervisor, and measured numbers
for what delivery costs (~2.3 s fixed + ~0.36 ms/file; 6.5–8.2 queue slots per item). A follow over
that path is a feed that arrives. A follow over HTTP is a timer.

**(4) `FEED` §7.1's one honest hole — the second source.**
*"A quiet publisher and a withholding origin are byte-identical at the consumer… Distinguishing the
two requires a second source — which is exactly what a mirror provides, and which is why §6 is on
the critical path for honesty rather than only for convenience."* `app/feed/mirror` is a
**reader-assembled** type. Two live peers, one mirroring the other, is the smallest configuration in
which that sentence can be demonstrated rather than asserted — and two- and three-peer harnesses
asserting on bytes at the far end are already how this repository tests replication.

**(5) `FEED` §7.3/§7.5 — unpublication is not erasure, and it is demonstrable.**
*"A tree from which an entry was removed is byte-identical to a tree that never contained it"*, and
removal **MUST NOT** be presented as deletion. With two live peers: A removes a binding and
republishes; B, holding the old signed root, can still show what A served. That is a *product*
behaviour — the honest sentence at the moment of the action — and it needs a second party who kept
something.

## 4. The architecture — one verification stack, two byte sources

**Both modes ship. That is a requirement, not a preference, and each one covers the other's hole.**
A laptop is not reachable from the public internet: no inbound port, no stable address, asleep half
the day. A CDN origin is reachable and cannot tell you anything about *now* — §7.1's honest hole is
that a quiet publisher and a withholding origin are byte-identical at the consumer. So the static
corridor is how you reach the world and the live path is how you get an answer with a timestamp on
it. A design that picks one is wrong in the case the other exists for.

### 4.1 The seam — `fetch.Source`

`fetch.Consumer` today bundles three things, and only one of them is about HTTP:

| | What | HTTP-specific? |
|---|---|---|
| **Byte source** | `Layout` + `Client` + `httpGet` | **yes** — and it is reached through exactly two primitives |
| **Verification** | `decodeVerified` (hash), the two-hop root signature, the `minSeq` floor, the CHAMP walk | no |
| **Caching** | `Cache`, keyed by content hash and by root hash | no |

So the seam goes under the byte source and nowhere else:

```go
// Source is where a consumer's bytes come from. It answers "give me this"
// and nothing else: no method on it can express a verdict, because every
// check lives above.
type Source interface {
    PeerID() string
    Root(ctx context.Context) (ent entity.Entity, locator string, err error)
    Leaf(ctx context.Context, treePath string) (h hash.Hash, locator string, err error)
    Blob(ctx context.Context, h hash.Hash) (ent entity.Entity, locator string, err error)
}
```

**Every one of those three signatures said `raw []byte` when the seam landed, and all three
were wrong — which is what a second implementation is for.** W1 shipped the interface with one
implementation and said in as many words that *"everything above it is asserted to be
transport-neutral and that claim is untested until a second one exists"*. Building `PeerSource`
tested it and it failed three times in one shape: **`ecf.Decode` was never a check, it was
HTTP's framing**, sitting above the seam because HTTP was the only thing under it. A dispatched
read hands back a decoded entity — the wire bytes are the protocol's own framing and never reach
this package — so a byte-shaped seam would have forced the second source to *re-encode an entity
in order for the layer above to decode it again*, which is manufacturing bytes in order to check
them.

`Leaf` is the one worth arguing about, because moving it moved a **conformance check**.
EXTENSION-NETWORK Amendment 6 makes an HTTP tree leaf the bound hash *pointer* and not the
dereferenced entity — but that is a rule about the **HTTP projection**, and a dispatched
`system/tree:get` returning the entity is the protocol behaving correctly rather than a one-hop
publisher cutting a corner. A check that fires on conformant behaviour on another transport is
not a stricter check, it is a false refusal (AP44). So `crackPointer` moved into `HTTPSource`,
where the obligation lives, and the seam answers the question both callers were actually asking:
*what hash does this publisher bind at this path*.

What did **not** move: the recomputed content hash, the two-hop signature against the key in the
peer-id, the `seq` floor, the fail-closed walk. None of them can be satisfied by a `Source`, and
`consume.go` now imports no encoding package at all — which is the one mechanical check a reader
can run on whether the seam holds.

**Three primitives, not two, and building it is what established that.** An earlier draft of this
section sketched two — content-addressed bytes, and the invariant-pointer leaves that live *outside*
the trie, since V7 §5.2 puts a signature at an invariant pointer rather than at a trie key, so a
consumer must hold both resolution paths. That is true of the **tree** and it is one short: the
**published-root manifest is reached a third way**, served as an entity at a location §6.5.3
*reserves* and does not make derivable. It is neither a content hash nor a tree-leaf pointer, and
folding it into either would mean deriving its address — which is the one thing this layer refuses
to do, because a derived address and a withholding origin are byte-identical at a consumer.

**`PeerID` is on the seam because it is the key every signature verifies against**, not a routing
detail. A source that cannot name its publisher leaves the verification above it with no subject.

**Every method returns a locator** — the transport's own name for what it fetched, a URL here and an
address elsewhere. The chain an operator reads names where each step looked, and a verification layer
that cannot see a URL cannot print one.

There is no `Describe()` yet. A description of *which mode answered* belongs with the freshness
sentence in §4.3, and until that sentence exists it would be a method with no reader — the shape this
project keeps catching in its own models.

`HTTPSource` is today's `Layout`+`Client`, moved behind it with no behaviour change. ✅ *Landed
2026-09-11; it is the only implementation, and a seam with one side is justified by what goes on
the other rather than by itself.*
`workbench.PeerSource` dispatches at a connected peer. ✅ *Landed 2026-09-12 (W2).* **Both were
adoption, not construction** — `AppPeer.Get` already routes a peer-qualified path to *that peer's*
tree (a dispatched read of a peer-qualified path is a remote read) and `AppPeer.ContentAt` already
does the cross-peer `system/content:get` the sync leg has used since M2. D20 came back positive for
the fifth time.

**It lives in `workbench`, not in `fetch`.** `fetch` is deliberately peer-free — `entity-fetch`
links a content decoder and an HTTP client and no peer, no store, no location index, which is also
why `fetch.Registry` exists rather than calling the kernel's resolver. A `Source` needing an
`AppPeer` would pull the whole peer stack into that binary. `workbench` already imports both.

**`decodeVerified` stays above the seam.** A live peer gets the identical hash check, the identical
seq floor and the identical walk. A second verification path is the failure this repo keeps hitting
in other forms, and it would be worse here than anywhere: two code paths for one trust argument,
with the weaker one wearing the same UI.

### 4.2 The trust argument, which is where a live read can quietly become weaker

**An authenticated connection proves WHO, not WHAT.** The handshake establishes that the peer on
the far end holds the key the peer-id names. It says nothing about whether the bytes it hands you
are bytes it ever committed to — a peer can serve whatever it likes over its own connection.

So **live mode verifies both**: read by dispatch, *and* check the key against the publisher's signed
`system/peer/published-root` exactly as the static path does. `RemoteSiteResolver`'s invariant —
*structure comes from the signed key set, never from what the far side says it has* — carries over
unchanged, and that is the keystone. Without it, "live" is a downgrade in trust presented as an
upgrade in freshness.

**The corollary is a third state and it needs a name, not a silent fallback.** A peer that has never
published a root can still answer `tree:get`. That is a real and useful thing — your own LAN, a
draft, a peer that never intends to publish — and it is *committed to nothing*.

**Q3 decided 2026-09-12: v1 does NOT read one through this stack, and it refuses by its own name.**
`fetch.ErrNoPublishedRoot` passes through `VerifiedRoot` unwrapped, for the same reason
`errNoManifestPrefix` does — wrapped in *"fetch manifest:"* it reads as a network failure, and no
amount of retrying changes it. The three states send an operator to three different places:
*unreachable* (the peer did not answer), *withholding* (it answered and will not serve what its own
root commits to), *unpublished* (it answered and has committed to nothing). Only the middle one is
the publisher misbehaving.

Admitting the third state here would mean every check hanging off a signed root that does not
exist, i.e. structure coming from what the far side says it has — the exact inversion §4.2 is
about. Reading an unpublished peer is a **different operation** and wants a different name; it does
not want this one made lenient. Gated: `TestLiveRead_UnpublishedPeerIsItsOwnState`.

### 4.3 Which mode answers, and the surface says which

A registry binding commits to `transports` — a list of hashes of transport-profile entities, never
inlined bodies (`EXTENSION-REGISTRY` §246, `[MUST, v1.21]`). Today `fetch.Registry.OriginFor` filters
that list for a usable **http-poll** profile and refuses if there is none. It becomes a **ranking**:

1. a dialable peer transport, when we are a peer and the target advertises one — *fresher, and it
   can be subscribed to*
2. http-poll against the origin — *always available, reaches anything on the web*
3. refuse, naming what was on offer and why none of it worked

**The chain gains a row and the freshness sentence changes.** Static says *"verified as of
`published_at`"* and must, because a quiet publisher and a withholding origin are indistinguishable.
Live says *"asked the publisher directly at `<t>`"*, which is a strictly stronger statement and is
the whole product difference. **Neither sentence may be printed by the other mode**, and
`ReconcileOutcome`'s discipline applies: which mode answered is a property of the *reading*, carried
in the result, never remembered by the caller.

### 4.4 Serving live is a grant, and it is the part most likely to be got wrong

Reading a stranger's site by dispatch requires that stranger to have authorized it. The mechanism
exists: the V7 §8 policy table at `system/capability/policy/{peer}` is keyed on the Base58 peer-id,
`hex(identityHash)`, **or `default`** — and `default` is what a public read is.

**Scope it to the site prefix and to two operations, never to `*`.** This repository shipped a
sharing grant carrying `Resources: ["*"]` for months, under a doc comment claiming the set was
minimal: sharing one folder authorized a read of every entity and every mounted file on the machine.
A public site grant is `system/tree:get` + `system/content:get` over `sites/{site_id}/*` and
the content namespace those bytes live in, and nothing else.

⚠ **And it is NOT only the site prefix, which W2 measured.** The published-root lives at
`system/peer/published-root` and its signature at `system/signature/{hex}` — both **outside** the
prefix they commit to. A grant scoped to the site alone yields a peer that serves every page and
cannot be verified at all, and the failure does not present as *permission denied on the manifest*:
it presents as **a publisher that has never published**, i.e. as the other machine's fault, with
the grant looking complete. Gated in both directions by
`TestLiveRead_NeedsTheSignedRootPathsNotJustTheSite`. **The blast radius of getting this wrong
is larger than that one's** — a per-peer grant names one reader; this names everybody.

### 4.5 Publishing is one act with two projections

The site lives in the tree. `entitysdk.PutSiteManifest` / `PutSitePage` write `app/site-manifest` /
`app/site-page` under **`sites/{site_id}/`**, on the final type tags.

⚠ **They wrote `content/sites/{site_id}/` until 2026-09-12**, which is the placement SITE v0.5 §2
drops by name as a layer violation — so the SDK, the surface an application developer reaches for,
authored sites where neither of this repo's own resolvers looks. Found by W2 pointing our own
writer at our own reader for the first time. Every path in this document inherited it.
Everything else is a projection of those entities:

| Projection | What it is | Reaches |
|---|---|---|
| **static emit** | `entity-publish`'s signed root + closure into a directory | anything with HTTP |
| **live serve** | advertise the transport, write the `default` grant, be dialable | a LAN, a rendezvous, a peer with an address |

A `publish` verb does the authoring and then either or both. **The registry binding is orthogonal
and stays static** — a registry is a signed root served over HTTP, and it can commit a binding whose
`transports` names a peer profile without itself being live.

## 5. Build order

Forced where it says forced; otherwise chosen and revisable.

**Step 0 — the pin survives the process, and both surfaces share it.** ✅ *done 2026-09-10.*
The browser was process state on the workspace, so `registry pin` reported success and the next
command reported *"nothing pinned"*; `~/.entity/browser.json` was read by the desktop app and
written by nothing, so the two surfaces could not share a trust decision either. The pin and the
unpin now both persist, and the shell opens pre-pinned. Small, and it is the difference between a
demonstration and a browser.

**Step 1 — `LivePeerResolver`: a third `ContentResolver`, over dispatch.**
`ContentResolver` (`workbench/site_resolver.go:111`) is already the transport seam, with two
implementations: the local tree and a verified static origin. A third — reading another peer's site
subtree by dispatched `tree:get`, verified against their `system/peer/published-root` — makes every
site surface we ship work against a live peer with **no renderer change**. This is the p2p read
path, and it is the prerequisite for everything in §3.
*Design constraint carried from the static resolver: structure comes from the signed key set, never
from what the far side says it has. A live peer can answer a listing query any way it likes; the
signature is what commits.*

**Step 2 — publish from the running peer.** ✅ *done 2026-09-12.*
Until then there was **no `publish` verb** (40+ shell verbs; measured) and no GUI panel: publishing
was `entity-publish`, a separate binary that reads a store off disk. For the static corridor that is
fine. For the live one it was the wrong shape entirely — a peer that authored a site should serve
it, and a reader should reach it by dialing.

There is one publishing ACT — signing a root over a prefix — and the projections are flags on it.
`publish` signs; `-out`/`-origin` also emit the static directory; `-public` writes the `default`
grant so a peer nobody named may read the site and verify it; `-private` withdraws it. The *Local
Site* panel carries the same act and, always on screen, the sentence that says who may read this.
The static emit stays: it is how you reach the web tier, and it has stopped being the only way to
exist.

Three things the build surfaced that are worth carrying into anything that grants:

- **A peer has exactly ONE published root**, so publishing a narrower prefix stops committing to
  everything outside it — under a valid signature, which a reader cannot tell from a site that was
  never there. The publisher is the only party who can see that coming, so it says so.
- **A catch-all authorization row is shadowed by every specific one.** The policy table resolves
  identity-hash → peer-id → `default` and returns at the *first* match, so a public site reached
  everybody except the peers already named by a share — the only peers an operator can test with.
  The general grant is now derived into every specific row.
- **A public grant needs the two paths outside the prefix it commits to** — the published root and
  its signature — or the site serves every page and cannot be verified, which reads to a reader as
  *"this publisher has never published"*.

**Step 3 — `Embed`, then the four feed types.** Forced order: the embed convention's child payload
carries a reference (we have the atom), and a feed entry's `body` is an `embed-node`. Built on
step 1's rails, so each type gets its live test as it lands rather than a fixture round trip and a
promise. A-21 (the naming ask) does not block this; if the word changes, it is a rename.

**Step 4 — the live obligations in §3, in order 1 → 3 → 4 → 2 → 5.**
Live references first because that is the MUST with no other route to it.

## 6. What this deliberately is not

- **Not a second static publisher.** The web tier has that covered and does it better.
- **Not a notification mechanism.** `FEED` §8's F-4 records that a mention list is a delivery
  instruction and does not belong in a content vocabulary. If we build delivery it is the granted
  route, named as such.
- **Not a parallel media path.** EMBED §3: *"a photo post and a text post are one shape with a
  different embed inside… nobody should build a parallel media surface for feeds."*
- **Not a fourth collection type.** Three answers to *"who assembled this list"* and there is no
  fourth.
- **Not conformance tooling.** A failure in a cross-impl vector is routed, not locally corrected.

## 7. The standing risk in this direction

A live counterparty makes tests non-deterministic, and this repository already has load-dependent
failures. Every live obligation above needs a harness that is **two real peers on a real network with
assertions on bytes at the far end** — `twopeer-sync` and `threepeer-sync` are the shape — and none
of them belongs in the default test sweep, for the same reason the existing cross-implementation
gates sit outside it: a sweep that can go red for reasons outside the tree teaches people to ignore
the sweep.
