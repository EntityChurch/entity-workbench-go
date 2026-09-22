# Folder sharing & sync

The declare-then-reconcile control loop, direction, conflicts, delivery saturation and catch-up, connection reachability, and the verbs that drive all of it.

> Memory, per `AGENTS.md`. Every entry below was paid for by a defect that shipped or
> a measurement that contradicted what we believed. Find by symptom; the mechanism is
> stated before the fix, and `file:line` references point at the code, not at prose.

---

- **The flow verbs are `peers` / `share` / `offers` / `accept` / `shares` / `unshare` /
  `access`** (`shellcmd/share_op.go` + `workbench/access_policy.go`, `workbench/share_offer.go`).
  `share` writes the permission AND the offer record; `accept` writes the delivery permission and
  syncs. **The offer is a LABEL, not an authority** — `APP-CONVENTION-SHARE` §2.2 forbids
  inferring authorization from it, so seeing an offer and still getting a 403 is the two things
  being correctly separate. **The authorization is the policy table and not `AuthorShare`'s
  minted token**, because `ShareWithdrawalNotice` states a `request`-minted token is *not
  recallable* — building `unshare` on it would make the verb unable to do what it is named after.
  **`access` marks the peer's own kernel-seeded `*:*` row**: unlabelled it reads as a wildcard
  grant to a stranger, and hidden it would conceal a real grant.
- **A FOLDER IS ONE OBJECT ACROSS TWO PEERS, AND DIRECTION IS A PROPERTY OF IT** (S6, AP72
  + AP67's second instance). `workbench.FolderID(owner, root)` is the same string on every
  peer that participates — derived from facts both sides already hold, so it cannot be typed
  twice and needs no wire change (`app/share/*` is APP-CONVENTION-SHARE's namespace; a field
  there is a cross-impl coordination, not a local edit). Before it, `share` wrote
  `folders/{root}` and `accept` wrote `folders/{owner}.{their-root}`: **no field in common,
  so there was no object either side could name**, and every symptom the operator reported
  was that one gap. `MigrateFolderIDs` moves pre-S6 records once, at bootstrap, not in the
  loop — a control loop that rewrites declarations on every pass is a different and worse
  thing than one that reconciles substrate to them.
  **`Mode` is what the reconciler branches on now, and `IsLocal()` is not.** Origin says who
  ORIGINATED a folder; it is immutable and binary, and using it for direction is why `both`
  was inexpressible. Read direction through `Publishes()` / `Receives()`, never through
  `IsLocal()`. **An absent `Mode` means the PRE-S6 behaviour** (`EffectiveMode`: local
  publishes, received receives) and never `both` — defaulting it to `both` starts publishing
  folders an operator only ever *accepted*, over a grant that already exists, and that
  mistake is not symmetric. Set it through `ShellWorkspace.SetFolderMode` (verb: `direction`;
  panel: the Sync row's one button), never by writing the field.
  **`Mode: both` RUNS BOTH LEGS as of 2026-09-10, and the two peers DO NOT have to name the
  directory the same thing.** This entry said the opposite for four days and the correction is
  the interesting part, so both halves are kept. What was true: the owner never built the
  reverse leg, because `receiveFromPeers` admitted only peers whose state is `Accepted` while
  `declareLocalShare` writes `Offered` and acceptance is recorded in the *receiver's* tree. What
  was **wrong** was the conclusion drawn from it — that this needed a receiver→owner channel and
  was therefore one piece of work with conflict propagation. It needed neither.
  Two fixes, and both are the same move. `offered` on a folder **we own** is *our own act of
  sharing*, not a stranger's proposal, so requiring `accepted` there required a fact that
  structurally cannot arrive; and **the owner READS the receiver's record over the wire**
  (`ShellWorkspace.ObserveRemoteFolder`, `shellcmd/remote_declaration.go`) rather than waiting to
  be told — an AP11 dispatched read, authorized exactly when it is worth asking, because the
  reverse leg only exists when the receiver **publishes** and a publishing peer has already
  granted us `system/tree:get` (`SyncSenderGrants`). That read answers the root name too, which
  is the fact nothing could ever have inferred.
  **A failed read REFUSES rather than falling back to our own root name.** The binding a guess
  creates is durable and silent — a subscription to a prefix that does not exist on the far side
  is accepted, reports healthy, and delivers nothing forever. Not creating one is recoverable at
  the next pass.
  **BOTH SIDES MUST DECLARE `both`, and this is a requirement rather than a bug.** Direction is
  per-peer; a receive-only counterpart publishes nothing and grants no sender authority, so the
  owner's subscribe answers 403. `direction` now says so, on the machine the operator is looking
  at, naming the command to run on the other one. Gates:
  `shellboot/mode_both_asymmetric_roots_test.go` (bytes on disk, asymmetric roots, with the
  symmetric arm as the control that isolates the name as the variable) and
  `mode_both_reverse_leg_test.go`, whose `ReceiveOnlyCounterpart` arm is what stops the fix being
  re-broken by a helpful fallback.
  **What is STILL owed: conflict propagation.** The two were never one piece of work — that was
  the wrong inference above — but the receiver→owner channel is still genuinely needed for
  *"your change landed on my edit"* (`reviews/CONFLICT-PROPAGATION-OPTIONS-2026-09-08.md` §8).
  Note the verb takes a folder-id of `{owner-peer-id}.{sender-root}`, so an operator who accepted
  into a directory of their own choosing sees an id built from a root they never typed.
- **`direction` RECONNECTS AND RECONCILES, and until 2026-09-10 it did neither while its own doc
  comment said the reconciler had already forced the reconnect.** It rewrote the policy row and
  stopped. Grants are assembled at handshake (AP63), so the new authority was inert; and nothing
  ran a pass, so no leg was established. Two correct declarations on two machines, and the
  feature did nothing until some later pass happened to run — indistinguishable from it being
  broken. `share` and `accept` had both steps from the start; this verb was the odd one out
  because nobody ran the flow (AP71). **A false sentence in a doc comment is worse than none** —
  it is the sentence the next reader checks the behaviour against, and this one had been read at
  least twice.
- **`make threepeer-sync` runs the topologies two peers CANNOT EXPRESS.** Two peers are one edge,
  so the whole class of *"and then the third machine…"* questions had never been asked. Three
  containers, one real TCP network, every file assertion on bytes on disk at the far end.
  **Fan-out** (one folder, two receivers) reaches the per-peer policy row under a second writer —
  and the assertion that catches a clobber is **a change reaching B *after* C was added**, not
  "both got the backfill", because a second share overwriting the first row makes the *first*
  receiver go quiet. **Chain** (A→B→C) reaches whether an *ingested* file is observable to the
  receiving peer's own watcher; B forwards by publishing **its own mount**, never by republishing
  A's folder, so a forward is an operator act rather than an emergent property of receiving, and
  the safety half (a write at the middle must not reach the origin) is asserted. Both green.
  **`Mode: both` is measured with `note()`, never `ok()`** — a measurement dressed as a check is
  how an undesigned behaviour gets recorded as a passing requirement. Four peers and a triangle
  are still untested.
- **A REFRESH BUTTON ON TREE DATA IS A BUG REPORT ABOUT A MISSING SUBSCRIPTION** (AP73).
  Measured 2026-09-04: 12 of 15 panels held a tree subscription, and the 3 that did not were
  the 3 sharing panels — the only ones with Refresh buttons. `share.go` had nine exports and
  no `RegisterWake`. `workbench.WatchDeclarations` watches all four declaration prefixes;
  `SharingRegisterWake` hangs it off the **peer** handle and fans out, because `StatusRender`
  and `ShareRender` already take one and are already wake-safe. **Only the READ is wired** —
  `StatusReconcile` dials every declared device *and* writes to the tree, so wiring it to a
  wake makes a dialer out of an open panel and wakes itself forever. Before adding a refresh
  control, name the prefix and say why it cannot be watched; the one honest case here is a
  peer's offers TO us, which live in their tree and need a dispatched remote read (AP11).
- **`Sync` IS THE FRONT DOOR; the other four sharing panels are DIAGNOSTICS.** Five panels
  touched this one job (Shared Folders 10 buttons, Sharing Status 5, Local Files 5, Files 2,
  plus Peer Connections) over eighteen shell verbs — each added for a real reason, most of
  them the scar tissue of a defect this repo actually hit, and the sum unusable. `SyncPanel`
  is the two gestures and nothing else: *pick a folder → pick a peer → Share*, and *a card
  appears → pick a directory → Accept*. **Share is one gesture over two substrate steps** —
  it creates the mount itself, because the operator never says "mount"; that is a mechanism
  that leaked into the UI and it is why the flow had a step nobody could explain. Note it is
  the SURFACE creating a mount, never the reconciler, which refuses on purpose. Do not add a
  control here without asking which of the two gestures it serves; anything else belongs in
  the diagnostic panels, which are kept and not deleted.
- **THE FLOW IS A CONTROL LOOP NOW, NOT A SEQUENCE — declare, then reconcile.** Two records are
  written on purpose (`workbench/desired_state.go`): `app/workbench/devices/{peer-id}` and
  `app/workbench/folders/{folder-id}`. Everything else — the policy row, the mount, the
  subscription, the sync binding, the connection — is OUTPUT of `ShellWorkspace.Reconcile`
  (`shellcmd/reconcile.go`), which is idempotent and runs at startup, after any change, and from
  the `status` verb. `share` / `accept` / `unshare` **declare** (`shellcmd/declare.go`) and then
  DERIVE the policy row through `ApplyDeclaredPolicy` / `WithdrawDeclaredPolicy` — they do not
  write it. Read `docs/architecture/SHARING-DIRECTION.md` before touching this
  area — the argument is that a **wizard** (order-dependent, non-idempotent, with no
  representation of what it established) cannot survive a restart, and every restart defect in
  this flow is that one sentence. Four rules the loop owns and nothing else may duplicate:
  the policy row is a **union across both directions** and has exactly **one writer** (AP68 —
  it was fixed in the reconciler first, which only *healed* the clobber a pass later, and
  "the loop corrects it" is not the same as "nothing else writes it"); **a verb that changes
  what the operator wants MUST write a declaration**, because a change the declaration does not
  carry is undone by the next pass — that is how `unshare` silently reversed itself at the next
  launch; a policy change and **only** a policy change forces a re-handshake, because grants are
  assembled at handshake but reconnecting on every pass is an outage generator; and the loop
  **names** what it will not do rather than doing it — it never creates a mount (that writes to
  somebody's disk) and never deletes (that removes their files or their access).
- **A READ AND A PASS ARE DIFFERENT OPERATIONS, AND THE READING MUST SAY WHICH ONE MADE IT.**
  `ShellWorkspace.Reconcile` dials every declared peer; `StatusSnapshot` (`shellcmd/status.go`)
  reads the declarations and observes the substrate and does neither. The shell's `status` verb
  runs the loop **on purpose** — a read-only report has the same blind spot as the five verbs it
  replaced — but a *panel* refreshes, so wiring a pass to a wake or a timer turns a status
  surface into a dialer an operator leaves running overnight. Hence `StatusRender` /
  `StatusReconcile` as separate bridge exports, one shape between them, and `Reconciled` carried
  **in the outcome** rather than remembered by the caller: "verified by a pass" is a property of
  the reading, and a surface that has to recall which function it called in order to caption its
  table will eventually caption it wrong — always in the confident direction. Both entry points
  share `observeDevice` / `observeFolder` and `FolderStatus.problems()`, so a read and a pass
  cannot describe the same device, or the same fault, differently.
- **OUTBOUND AUTHORITY IS EXACTLY KNOWABLE; INBOUND IS NOT, AND NO SURFACE MAY FAKE IT.** What we
  grant a peer is our own `system/capability/policy/{peer}` row, so `DeviceStatus.OutboundGrant`
  is a fact and is printed. What *they* grant *us* lives in **their** capability table, which
  this peer cannot read — it is only ever OBSERVED, through deliveries arriving or through
  chain-errors. So the inbound direction renders as an observation (what has actually landed:
  `FilesPresent` / `FilesIngested`, and whether a subscription exists) and **never as a
  health dot**. A dot there asserts something about another machine we have no way to check, and
  it is wrong in exactly the case that matters — they revoked us and we have not tried since.
  `SharingStatusPanelTests.Inbound_Authority_Is_Stated_As_Unknowable_Not_Drawn_As_A_State` is the
  enforcement point; if it fails because someone added an inbound status field, **delete the
  field, not the test.**
- **RUN THE FLOW AND READ WHAT IT SAYS — a verb's printed guidance is a surface with no reader in
  the suite** (AP71). `share` went on telling the operator on the other machine to `mount` first
  and to `accept` without a directory for as long as S3 had been shipped, i.e. it instructed them
  to reintroduce the exact coupling S3 removed. Every gate was green; the operator doc had been
  corrected and the program had not. **When you delete or add an operator step, grep the
  PROGRAM's output for it**, not just `docs/`. `avalonia/README-SHARING.md` is the standing
  artifact for this — it is written by transcription from a real two-peer session, so writing it
  is running it.
  **AND GREP THE PUBLISHED DOCS FOR THE SENTENCE THAT USED TO BE TRUE** (AP80) — this is AP71 one
  layer out and it is the half we keep missing. A feature ships in code, in the one guide the
  session had open, and in a **plan** that now describes the past, and the plan is what a stranger
  opens to find out what the product does. Four published claims were false on 2026-09-08, each
  written by a session that shipped its feature correctly: the operator guide said concurrent
  edits were unhandled *the day after* they shipped, and the landscape doc still carried the
  provenance claim that had been retracted in `STATUS.md`, in this file and in the test, in the
  very section a reader goes to for what M3 is. **Nothing in the tree can fail on prose** — start
  from `CANONICAL-DOCS.toml`'s declared list, which is short, and search the **claim**, not the
  filename. Corollary: **a supersession recorded only in `STATUS.md` has not been recorded**,
  because nobody arrives at a topic through the rolling log; banner the document the next session
  will actually open.
- **THE FIRST CHANGE AFTER A RESTART IS NOT DELIVERED LIVE, AND THE CAUSE IS UNKNOWN — BUT IT IS
  A DELAY, NOT A LOSS, AND WE CALLED IT A LOSS FOR FIVE DAYS.**
  Measured 2026-09-03, both directions, no error on either side, `status` reporting `settled`
  throughout: restart a peer, and the next file changed never arrives while every one after it
  does. **That much still holds. The sentence that followed — *"it is not a delay, the change is
  gone, and only `resync` recovers it"* — was WRONG, and it was repeated in six documents.** It
  was written on 2026-09-03, *before the catch-up supervisor existed*, and nobody re-measured it
  after. `make twopeer-gui` PHASE 13 now does: the waived file lands **~107 s after it was
  written, with no resync and no operator action.** The mechanism predicts the number — the
  supervisor doubles its interval after every empty pass, so from a restart the passes fall at
  roughly t=0, t≈120 s, t≈360 s, and **every harness in this tree asserted on a 90-second window,
  which expires between the first two by construction.** *A window shorter than the mechanism's
  period turns a latency into a loss, and the write-up is then confidently about the wrong
  defect.* No bound is promised: the interval grows with idle time to a 10-minute ceiling, and
  `resync` / **Pull now** forces it. **Two obvious
  hypotheses are already refuted** (a missing outbound dial; a stale entry in our own pool — the
  evict-then-dial change was REVERTED rather than kept, because a cost justified by a dead
  hypothesis is not a fix). Read
  `docs/architecture/reviews/FIRST-CHANGE-AFTER-RESTART-IS-LOST-2026-09-03.md` before touching
  this — §4 lists what has not been ruled out, and the first question to answer is whether the
  loss is on the send side or the receive side, which nobody has instrumented.
- **`CandidateData.PeerID` IS EMPTY ON EVERY mDNS CANDIDATE — USE `entitysdk.CandidatePeerID`**
  (AP82, fixed 2026-09-09). Per EXTENSION-DISCOVERY §2.1 the field is null until IDENTIFY, and the
  only writer of the populated form — `discovery.Handler.PromoteSuccessor` — has **zero callers in
  either tree**. The claimed peer-id travels in the `peer_id_hint` TXT key and nowhere else. So
  reading the field is not a stricter check, it is a **guaranteed miss**, and three consumers here
  joined on it: the reconciler's address refresh, `dialableAddressFor`, and the `peers` verb.
  All three did nothing at all, for every peer, on every pass, since they were written — **the
  `peers` verb had never listed a single discovered peer.** The Nearby panel read the TXT hint and
  worked, which is why discovery looked healthy while every consumer of it was inert.
  **Two readers of one announcement using different keys, and only one of the keys is ever
  populated.** The tell is a join that silently yields nothing rather than failing; a fixture that
  fills the field in cannot reproduce it, which is AP58's shape at the level of a value. Trusting
  the hint is correct **only** for deciding where to dial a peer we have already declared — the
  peer-id is matched against a record we hold and the dial authenticates, so a false hint costs a
  failed handshake, never a wrong peer. It is not sufficient to admit a peer, mint a grant or bind
  an identity. Gates: `entitysdk/candidate_peerid_test.go`, `shellcmd/discovery_address_test.go`,
  both with an anti-vacuity arm asserting the field really is still empty.
- **A REMEMBERED ADDRESS IS A HYPOTHESIS; A LIVE ANNOUNCEMENT IS AN OBSERVATION** (fixed
  2026-09-09, found 2026-09-08 on the first real two-machine run). Measured: 45 minutes of
  `connection refused` at a port the peer had moved off, **zero** successful connections, and the
  operator's folder syncing one way because inbound worked and outbound never came up — while
  that peer sat on the LAN announcing its real address the whole time. The intent was right — a
  stored address is the only source that survives a restart — and **the error was treating
  *durable* as *authoritative*.** Now: `dialLadderFor` (`shellcmd/reconcile.go`) puts **discovery
  first and the declaration behind it**, `ensureOutboundRoute` tries EVERY address rather than the
  preferred one, and the address that answered is written back through `RememberDeviceAddress`.
  Walking the ladder is also what makes trusting the mDNS claim safe: a spoofed announcement costs
  one failed handshake and we fall through, instead of replacing a working address. Keep the
  fallback — dropping it turns "discovery first" into "discovery only" and breaks every peer that
  is asleep or on a network with no multicast.
- **A PEER WE CANNOT DISPATCH TO MUST NOT RENDER AS "CONNECTED"** (fixed 2026-09-09, same run).
  `ConnectedPeers()` is the connection **pool**: it holds sessions in both directions and tags
  neither, so an inbound-only session — they dialled us, we never dialled them — was
  indistinguishable from a working one. That is the exact state in which sharing is half-broken
  and the most likely one, since a dial-by-address authorizes only the dialer (AP63). **The panel
  and the run log contradicted each other in the same session and the reassuring one was on
  screen.** `DeviceStatus.OutboundRoute` now carries the direction, sourced from
  `dialedThisProcess`; the shell prints `inbound only`, the Sharing Status panel says *"they can
  reach us — we have no connection to them"* in Goldenrod and never DarkSeaGreen, and
  `DeviceStatus.directionProblem` is the **one writer** of the sentence, shared by the pass and
  the read so the two surfaces cannot drift. **It fires for a RECEIVE-only folder too** — that
  was a live error while writing it, and AP63 already had the answer: a sync is mutual, the
  receiver dispatches out to subscribe and pull the closure, so an unreachable peer breaks an
  incoming folder just as completely and merely presents as *"nothing is arriving"*. The
  consequence sentence differs by direction because those two symptoms send an operator to
  opposite machines.
  ✅ **Direction IS observable as of 2026-09-17 and the workaround is retired** — this bullet said
  *"not observable from core-go"*, which was true (`Connections()` concatenated and tagged nothing;
  `IsConnected` conflates the pool with the §6.11 reentry map), was routed as
  `reviews/CONNECTION-DIRECTION-AND-BILATERAL-REACH-2026-09-09.md`, and core-go answered it with
  `Connection.IsOutbound()` / `Direction()`, recorded once in `PerformConnect` (their row 13).
  `entitysdk.PeerInfo.Direction` — **declared and deliberately left empty from the day it was
  written** — is populated, and `AppPeer.HasOutboundConnection` is the question every caller was
  actually asking. `ShellWorkspace.dialedThisProcess` is **gone**.
  ⭐ **Adopting it was not tidying, and the reason generalises: a REMEMBERED ACT and a CURRENT
  FACT are different claims, and they diverge in the direction that reassures.** *"We dialled
  this peer"* stays true forever; *"we hold a connection to this peer"* stops being true the
  moment it drops. So `OutboundRoute` — the field whose entire purpose is to stop a surface
  asserting a route it does not have — went on asserting one, for the rest of the process, after
  a mid-session drop. **The workaround reproduced the defect it was built to fix, one layer in.**
  The tell is a set named after something *we did*, standing in for something that *is*.
  Second consequence, and it is a behaviour change worth knowing: the memory also suppressed
  re-dialling, so the loop could see the route was gone and decline to act. What remains is
  `dialFailedThisProcess`, a **cost bound** (do not re-walk a ten-second address ladder for a peer
  that is switched off) and not a fact about the relationship — so a dropped connection is now
  re-established by the next pass instead of waiting for an operator to type `connect`.
  Gate: `entitysdk/connection_direction_test.go`, and **the pair is the assertion, not either
  half** — both arms run against one pair of peers in one pool, so bob's view of the wire must be
  outbound and alice's view of the *same wire* inbound. A field hard-coded to either value passes
  a single-direction test; mutation-checked, and it fires.
- **AN UNREACHABLE PEER WRITES A PERMANENT ENTITY PER STATUS TRANSITION, AND THE CAUSE IS THAT
  `maintain-peer` READS AN IN-MEMORY MAP TO DECIDE WHETHER A RELATIONSHIP EXISTS** (measured
  2026-09-09; core-go's, routed as
  `reviews/CORE-GO-MAINTAIN-SESSION-DIES-AND-THE-GRAPH-DOES-NOT-2026-09-09.md`).
  The reconnect lifecycle has two halves with **different lifetimes**: the continuation graph
  (`system/inbox/network/{peer}/*`) and the lifecycle subscriptions are tree-resident and restored
  at open; the maintain **session** is a map on the handler and dies with the process. So
  `existed` is false on the first call of every new process, `maintain-peer` takes
  `dropSession` + 502 instead of the arm-the-retry + 200 branch core-go added *specifically to
  stop this marker family*, and every later peer-status transition dispatches
  `system/network:restore-subscriptions` into an empty map → **404 not_found** → one permanent
  `chain-errors/lost/…/notif-sub-…` marker, forever.
  **The op is `restore-subscriptions` and NOT `reconnect`, and the marker's own body is what
  says so** (corrected 2026-09-09 by audit, after the first write-up named `reconnect` in three
  documents). `advance.go` binds this marker **only** when the continuation has no `on_error`,
  and sets `reason` to the failed op's `code` verbatim — so `reason=not_found` reaches the
  resubscribe continuation and nothing else: `reconnect`'s continuation *has* an `on_error`
  (routing to the backoff path, which is exactly the fix that works), and the backoff's own
  `maintain-peer` answers 502 `connection_failed`. **Consequence worth more than the correction:
  the trigger is the far peer RECONNECTING TO US, not going away** — both lifecycle
  subscriptions fire on `created`/`updated` with no filter on the status value, so an
  inbound-only peer whose retry loop keeps succeeding writes one marker per success. That is why
  the cadence matched their dial interval.
  **Four arms, and the two NEGATIVES are the load-bearing ones** (`shellboot/chain_error_growth_probe_test.go`):
  a peer never reachable → 0 markers (no session, so no graph); established-then-closed in one
  process → 0; **restart, then one status transition → the operator's marker, exact shape**;
  and `CollectExpiredMarkers` on expired markers → 1→0, a **real** removal. That last arm is not
  optional — `HistoryConfigData.MaxDepth` looks identical and prunes nothing, and a second no-op
  knob would have inverted the conclusion. **So the growth is BOUNDED at one retention window**
  (24 h default, knob at `system/config/chain-errors` → `retention_ms`), ~6,000 entities at the
  observed rate — noisy, not disk-filling. Do not set a local retention override to mask it: that
  discards real chain forensics for someone else's bug, and the trigger is the peer we cannot
  reach, which the dial ladder above is the actual fix for.
  **Every arm that runs in ONE process measures zero**, and one process is what every test in both
  trees uses — the defect lives exactly at the boundary a suite does not cross.
- **THE CONNECTION IS BILATERAL IN THE PROTOCOL, AND BOTH HALVES ARE GATED OUT OF OUR TOPOLOGY**
  (measured 2026-09-09; the operator's question, and a better one than our bug).
  The kernel has **§6.11 reentry** — `registerInboundForReentry` caches every accepted connection
  by peer-id so a handler can originate back over it — and **§6.5(b) reciprocal grant**, whose own
  comment says *"that single reciprocal grant is what makes the pair symmetric"*. So "why do we
  need two one-way connections" has a real answer: **we should not, and the mechanism exists.**
  Both are gated on predicates a LAN peering never satisfies. Reentry is consulted only when
  transport-profile resolution **errors** (`establishRemote`, `core/peer/remote.go:935`) — so a
  profile that resolves to a *dead* address dials the corpse forever while the peer's live inbound
  connection sits one map lookup away, which is exactly the operator's 45 minutes. The reciprocal
  grant is sent only when `EstablishedViaRendezvousKey()`, and two laptops on a LAN dial by
  address, so it is never sent in either direction and each peer must independently dial the
  other. **Do not build around this locally** — the ask is routed in the review above; our own
  outbound dial per process is the correct workaround meanwhile.
- **THE ADDRESS AN OPERATOR TYPES IS A DURABLE FACT AND BELONGS IN THE DECLARATION.** `connect`
  used to put it in `ShellWorkspace.Conns` and the kernel's pool — both process memory — so it
  died with the process and the reconciler had nothing to dial after a restart.
  `RememberDeviceAddress` writes it to `app/workbench/devices/{peer}`, and **updates only, never
  creates**: connecting is a means, not a relationship, and dialing a peer to look at its tree
  must not enroll it in one the loop then maintains forever. Related: **our own outbound
  connection is derived runtime state that must be re-established at open** (AP62's shape again)
  — the loop does it once per process, because a connection has a direction for *authority* and
  none for *display*, so `ConnectedPeers()` says "connected" over a route we cannot dispatch on.
- **`system/network:maintain-peer` IS the reconnect engine, and we hand-rolled around it for
  months.** It connects, installs the §4.1 continuation graph, retries **forever** with derived
  backoff, and restores subscriptions on reconnect; `entitysdk.NetworkClient.MaintainPeer` wraps
  it and the handler is registered by default. It had **zero callers in shipped code** — ten
  references, nine in its own file and one in its own test — while three verbs called
  `AppPeer.Connect` once and called that a relationship. Its session map is in-memory with no
  rebuild at open, so the **caller** re-issues it per launch; that is the reconciler's job.
  `MaintainOpts.Address` is bare `host:port` — `RegisterRemote` refuses a scheme and builds a
  TCP profile, so a `ws://` address cannot be passed here at all. This is D20 aimed at our own
  SDK: **grep `../entity-core-go` AND `entitysdk/` for the thing you are about to build.**
- **A file holds what you need in order to find the tree; the tree holds everything else.**
  Identity, store path and listen address are pre-peer facts and must be on disk. Everything
  after that — devices, folders, and eventually `gui-layout.json` and `browser.json` — belongs in
  `app/workbench/`, where it is addressable, watchable and travels with the peer. The layout
  file's stated reason for being a file (a peer-id key never matches twice under an ephemeral
  default) died with the ephemeral default; migrating it is owed, and needs a read-both /
  write-tree transition so an existing install does not lose its layout.
- **A CONFLICT IS DETECTED, RECORDED, LISTED AND UNDOABLE — and the DEFAULT does not put a
  second file in the folder** (`workbench/conflict_detect.go`, `conflict_record.go`,
  `shellcmd/conflict_op.go`; verbs `conflicts` / `resolve`; the *Sharing Status* panel's fifth
  section). Before it, a delivery that landed on your edit replaced it silently — nothing was
  destroyed, the chain kept both, and the replaced version sat where no surface rendered and no
  verb reached. **A recoverable loss nobody is told about is an unrecoverable one.**
  **`EXTENSION-REVISION` §2.3's keep-both is the wrong DEFAULT here and the reason only shows
  up in the topology this product ships**: it keeps local at the path and puts the incoming
  version in a sibling, so in a one-way share the receiver stops converging to the owner's
  version and *the owner is never told*. The default converges and records what it replaced;
  `keep-both` is a per-folder declaration (`FolderData.Conflict`, absent means record — never
  keep-both, for `EffectiveMode`'s reason). **Both versions are recoverable either way; the
  difference is whether both are PRESENT** — keep the word `keep-both` for the sibling form
  only, because it is a cross-impl term and `KeepBothSuffix` is byte-identical to
  `ext/revision/strategy.go`'s naming by obligation.
  **`resolve -keep mine` writes through the FILESYSTEM, and the obvious route is a silent
  regression**: restoring through `local/files:write` records with a DELIVERY's provenance, so
  the next catch-up pass reads that head, concludes the copy is stale, and undoes the
  operator's choice minutes later with nothing said. Writing into the mounted directory makes
  the watcher record `local/files:watch`, which is the truth. It also **declines that exact
  delivery** — the resolved record is keyed on (path, mine, theirs), which is the identity of
  one collision — so a pass cannot re-materialize it. Gated by restoring and then running two
  `resync` passes.
  **THE RULE IS THE FOLDER OWNER'S, AND EACH SIDE USED TO READ ITS OWN COPY** (AP94, fixed
  2026-09-11). `FolderData.Conflict` is per peer, travels on no wire field, and was read locally by
  whichever side the collision landed on — so A could declare `record` while B declared `keep-both`,
  for one folder, and **neither machine could notice**: each side's surfaces are correct about its own
  declaration and blind to the other's. Found by arguing for the rule that forbids it; the ask we
  routed came back as *a `shared` subject MUST name its reconciliation rule*, and the first thing it
  indicted was us. `FolderID(owner, root)` already designates the owner on every peer, including
  under `Mode: both`, so no wire change was needed. Now: the reconciler reads the owner's declaration
  and records it as an **observation** (`workbench/observed_state.go` — its own prefix, its own type,
  **never a field on the declaration**, because hearsay merged into a declaration means two different
  things depending on which side of the folder you read it from), and the delivery handler reads what
  the reconciler recorded rather than dialing. **A rule we have never read HOLDS the collision**
  (`409 conflict_rule_unknown`) — nothing overwritten, deliveries unaffected, released by the next
  pass. **The receiving side's verb REFUSES** and names the machine to run it on: its copy is not
  consulted, so accepting the write would hand an operator a success line for an instruction the
  product will not carry out. **Scope the refusal to RESOLVING, never to delivering** — a hold that
  stopped the folder is a worse failure than the divergence and is indistinguishable from it in the
  moment, which is why the release is gated as carefully as the hold. **And the sentence has ONE
  writer**, on `FolderStatus.problems()`, so a read and a pass cannot describe the state differently;
  the pass adds only the fact it alone has, in the Note. **A SAFETY OUTCOME MUST NOT CONSUME A SAFETY
  BUDGET** — a held delivery was first accounted against the conflict-storm burst limiter, which is
  backwards: a hold writes nothing, so spending budget on it does not make the check stricter, it
  makes the diagnosis worse (an unreachable peer fills the window and the operator is told
  *"conflict storm"*, a fault on their own machine, when the cause is another machine). The tell is
  a counter documented as *things this process acted on*. **And ask the owner by the CANONICAL
  folder id** (`FolderID(owner, root)`), not by our local record's id — they differ only for a
  pre-S6 record `MigrateFolderIDs` could not move, and there the local id asks for a path the owner
  does not have, holding every collision **forever** while files keep flowing.
  ~~Known gap, named rather than left to be found: **there is no GUI control for the rule at
  all.**~~ — **CLOSED 2026-09-16.** `StatusSetConflictRule` (bridge) + the *Sharing Status* panel's
  per-folder rule line and toggle. It was `D23`/`AP57` in its purest form — `StatusResolveConflict`
  was exported so *resolving* was reachable, `SetFolderConflictPolicy` was not, so the read-write
  model wore a read-only surface — and `AP49` underneath it: `FolderStatus.OwnerRuleKnown` and
  `OwnerConflictPolicy` were computed on **every** reading and declared by nothing in the DTO, so
  `System.Text.Json` dropped both one field short of the screen.
  Three rules the control obeys, each already earned elsewhere in this file.
  **The control is offered on OWNED folders only** and the row otherwise names who owns it: the
  verb refuses on a folder this peer does not own, so an enabled button would be a surface
  accepting an instruction it cannot carry out, and a *disabled* one would read as a permission
  fault. **`ruleSettableHere` is its own DTO field rather than the renderer reading `local`** —
  the two are the same predicate (`OwnerOf(self) != self` ⟺ `!IsLocal()`) asked for different
  reasons, and conflating those two questions is what `AP94` is the entry about.
  **Three states, not two:** `OwnerConflictPolicy` is empty exactly when `OwnerRuleKnown` is false,
  and a renderer defaulting that to `record` deletes the one sentence that explains the symptom —
  while the rule is unread a collision is **HELD**, not applied. Gates:
  `SharingStatusPanelTests.The_Conflict_Rule_Is_Settable_Only_On_A_Folder_This_Peer_Owns` and
  `An_Unread_Owner_Rule_Says_Held_Rather_Than_Defaulting_To_Record`, each mutation-checked and each
  caught by exactly the mutation it is named for; plus
  `SyncPanelTwoPeerTests.The_Status_Envelope_Carries_The_Conflict_Rule_And_The_Export_Changes_It`,
  which is the only arm that reads what Go actually emits — **both panel tests drive
  `SeedFolderForTests` and would pass with all three fields renamed on the Go side**, rendering
  *"rule not yet read"* for every folder forever. It runs on a **test-scoped** peer because the
  fields are per folder and `BridgeFixture.DefaultPeer` is shared (`AP70`), and it carries the
  anti-vacuity arm that sets the rule **back** — without it an export that writes `keep-both`
  unconditionally passes, which is the one value that stops a folder converging.
  ⚠ **The height floor moved 900 → 960 and that is not a fix.** The two earlier raises were new
  *sections* — fixed chrome. This is a line, and sometimes a button, on every row of the **folders
  list, which has no height bound**, so the growth is per folder and no constant bounds it. The
  open item below is now one row of text closer to firing.
  **A path with no chain is NOT a conflict**, and that is the storm rule, not caution: recording
  is what the classification reads, so treating an absence as a collision conflicts a whole
  folder on the first pass after an upgrade. Carried on the record as `recoverable: false`.
  **The anti-vacuity arm is what caught the head-provenance bug** — one delivery to an edited
  file and one to an untouched file, in the same pass, asserting **exactly one** conflict.
  Without it `return conflict` passes. `SYNC-LIMITS-AND-FAILURE-MODES` §5 has all four storm
  rules and how each is met; the burst limiter is a **sliding** window (10 per 2 min) because a
  resetting counter lets 2×limit through across two adjacent windows.
- **`history query` IS the recovery surface, so it has to take the paths people type and say
  who wrote each position.** Both were broken and both were found by running the flow, not by
  reading it (AP71's shape). It passed its argument to the handler **raw**, so `@alias/…`
  matched nothing and the operator got *"(no transitions recorded — is a config installed?)"*
  — a confident diagnosis pointing at the one thing that was fine. A **bare** path must keep
  passing through untouched (the store canonicalizes it against the local peer, which works
  from any WD; `sh.Resolve` on a bare path at the REPL root yields `/local/files/…` with no
  peer in it and matches nothing), so `historyPath` resolves the `@` form **only**. And the
  renderer dropped `Handler`/`Operation`, which the recorder has always carried: on a shared
  folder that column is the whole operator story — `local/files:watch` is *you edited this*,
  `local/files:write` is *their copy replaced yours*. Note the harness consequence: a chain
  assertion that only counts positions is satisfied by four DELIVERIES, so assert that both
  provenances appear.
- **THE LIVE PATH LOSES FILES UNDER BURST, AND A FILE COSTS ~8 DELIVERY-QUEUE SLOTS, NOT ONE**
  (AP77, AP78). Measured 2026-09-07 (`make loadtest`): 2000 files into a shared folder delivers
  **676 and stops forever**, because the SENDING peer's subscription shards saturate and drop
  before anything reaches the wire — no failed delivery, no chain error, no counter that moves
  on the receiving side, both peers reporting healthy.
  **Two causes, and the second was ours.** The ring is configurable, and `entitysdk` sets 4096
  slots against core-go's 65536 on a real memory measurement (~20.3 MB/peer, eager, never
  released). 4096 was justified as core-go's *"sized for 1000+-file mount bursts"* plus 4×
  margin — **but that counted FILES and the queue counts NOTIFICATIONS**, and one mounted file
  emits the watcher's file entity, the ingest document and the blob bindings. Swept
  (`TestLoad_QueueDepthSweep`, ring × burst, supervisor off): 4096 → 675/2000; 16384 → 2000 ✓
  but 2949/10000; **65536 (core-go's own default) → 9100/10000**; 262144 → 10000 ✓. Bracketing
  those puts demand at **6.5–8.2 slots per file**. So `shellboot.DefaultDeliveryQueueSize` is
  65536 — the application tier making the opposite call to the library, exactly as it already
  does for `DisableRegistry` — and `Config.DeliveryQueueSize` exposes it, because it had been a
  documented mitigation reachable from **no frontend at all**. **And no value fixes it**: the
  cliff moves with the ring and never goes, since the producer is a person with a file manager.
  **CORRECTED: live delivery is ~2.3 s fixed + ~0.36 ms/file, and the old "~90 files/s / catch-up
  is 20× faster" was wrong** — the 90 came from a 200-file run that is almost entirely fixed
  setup, i.e. an operation's overhead divided by its file count. Leaning on catch-up is still
  right, but for the other reason: `EXTENSION-SUBSCRIPTION` §5.5 makes delivery **best-effort**
  and SHOULDs periodic reconciliation, so a subscriber that does not reconcile loses things at
  any speed. **We built that supervisor without reading §5.5 — D20 aimed at the SPEC for the
  first time**, having aimed it at the kernel four times. Routed with the layering question in
  `reviews/SUBSCRIPTION-SATURATION-AND-THE-LAYER-BOUNDARY-2026-09-07.md`; the part we cannot fix
  here is that a subscriber can neither discover the publisher's ring size nor see its drop
  counter, which is why the adaptive rate is a blind heuristic and not a feedback loop.
  A backfill pass runs ~1,900 files/s and a pass with nothing to do costs **0.24 ms/file**
  (2000 files in 472 ms), because F9 already short-circuits on an equal blob hash.
  Hence `shellcmd/catchup.go`: a 60 s supervisor that
  re-derives the truth by asking each sender what it holds. It is **on by default whenever
  `ReconcileOnStart` is** — wired in `Bootstrap` and not per-frontend, for AP67's reason — and
  it does **not dial**, which is what makes it safe on a timer where `Reconcile` is not.
  **`PeerManager.Destroy` stops it**; until 2026-09-07 it did not, and a destroyed peer left a
  goroutine taking passes against a closed store for the life of the process. **And
  `ReconcileOnStart` answers the wrong question** — it means *"I have durable declarations"*
  where the loop needs *"am I long-running"*, so an in-memory peer that accepts a share
  mid-session is still uncovered. Which surface runs it, and why defaulting it on for every peer
  is not free, is `SYNC-LIMITS-AND-FAILURE-MODES.md` §7.
  `shellcmd/delivery_health.go` surfaces the kernel's own `DroppedDeliveries()` through
  `status`; it had zero readers in this repo before that, which is **D20 aimed at the kernel
  for the fourth time.** The catch-up makes a drop a DELAY, not a loss — it is not
  backpressure, and real backpressure belongs in the kernel's queue.
  **The RATE ADAPTS, and the four properties are load-bearing** (`nextCatchUpInterval`, a pure
  function so it is gated without a clock). **The CEILING IS DERIVED FROM WHAT A PASS COSTS, not
  a constant** (`settledCeiling`, added 2026-09-08 on an operator report). The intervals double,
  so the PASSES land at t=0, 2 min, 6 min, 14 min, 24 min — four empty passes and every folder
  was on a ten-minute check, *whatever it cost to look*, which on a 1,000-file folder is 0.24 s.
  The resting interval is now at least 100× a pass (≈1% duty), floored at the base rate and still
  capped at the 10-minute hard ceiling: a ~1k-file folder rests at **60 s** (which is also
  Syncthing's default rescan interval), a 10k-file folder at 4 min, a 100k-file folder at the cap
  as before. `SYNC-LIMITS` had already named this as a known limitation — *"adapts to whether it
  is finding anything, NOT to folder SIZE"* — and it sat as a limitation rather than a bug
  because nobody had multiplied the ladder out into wall-clock latency. **A back-off whose
  ceiling is not derived from a measured cost is a latency budget nobody signed off.** The receiver cannot see the sender's counter, so
  the only local signal is *its own passes*: recovered something → we are behind; recovered
  nothing → we are not. **Recovery is asymmetric on purpose** — back off by doubling to a
  10 min ceiling, return to the 5 s floor in ONE step, because being slow to notice a burst is
  a failure an operator feels and being slow to relax is not. **The ramp up is a gradient, not
  a snap to the configured rate** — clamping it at `base` jumped 5 s → 60 s the instant a burst
  ended and discarded exactly the rates worth having while a copy trickles in, so `base` sets
  where the ramp STARTS and is not a floor. And **the wait is never shorter than the pass that
  produced it**: without that, a folder whose pass takes 20 s runs back-to-back forever, which
  burns the peer *and* re-reads a moving target. Settling first is not just cheaper, it is more
  correct — a catch-up reads CURRENT STATE rather than replaying a change stream, so one pass
  over a settled folder gets everything.
- **A BOUND WITH NO MEMORY BOUNDS THE FOLDER, NOT THE PASS** (AP93, fixed 2026-09-11). The backfill
  walk stopped at `backfillWalkLimit` (20,000) for a real reason — an unbounded walk driven by a
  remote response is a denial of service with our own CPU — reported the truncation honestly, and
  restarted from the base prefix every pass. The traversal is deterministic, so **every pass returned
  the identical prefix**: 25,000 entities, two passes, **0 new paths, 5,000 unreachable** for as long
  as they did not change again, with every gate green and the limit shown in the UI. **The tell is a
  cap on a REPEATED operation with no cursor beside it**, and the giveaway is a disclosure in the
  present tense (*"this is a prefix of the folder"*) where the honest sentence needs a future one.
  The walk now resumes after a cursor (`shellcmd/backfill_cursor.go`). Three things it cost, each
  worth knowing before touching it: **a directory must sort under its own name plus a separator, not
  its bare name** — leaves under `d` all begin with `d + "/"`, so with siblings `a` and `a.txt` the
  bare-name key emits `a/x.txt` before `a.txt` while `a.txt < a/x.txt`, and one transposed pair is a
  file the cursor skips forever, invisible in any fixture whose names do not collide at a separator;
  **the subtree prune is what makes a resumed pass cheap in ROUND TRIPS**, and its off-by-one drops a
  whole subtree while reporting a clean, complete, shorter folder, so the gate asserts the tail
  property at EVERY position rather than one; and **the supervisor had to learn the same
  distinction** — it backs off on *"recovered nothing"*, which the first segment of an oversized
  folder legitimately reports, so `CatchUpResult.Incomplete` holds the floor. Without that last part
  the fix delivers progress on every pass and an hour between passes, which is the defect rebuilt on
  top of its own repair. The cursor is **process memory** (restart re-walks from the top, files
  short-circuit on F9) because persisting it means one tree write per truncated pass on a watched
  prefix. Control arm: `TestProbe_BoundedWalkMakesNoProgress` stays pointed at the cursorless entry
  point and still measures 5,000 unreachable.
- **THE SYNC LEG HAS NO ROLLBACK FLOOR, MEASURED, AND IT IS STILL OPEN.** `fetch.Consumer.acceptSeq`
  refuses a published root whose `seq` went backwards; the sync leg has nothing. Measured
  2026-09-11 (`shellboot/sync_rollback_probe_test.go`): capture a v1 delivery while current, let v2
  land, re-dispatch v1 — **status 200, no error, and v2 is gone from disk.** What is measured is that
  the HANDLER has no ordering check, at the entry point a live delivery and a backfill both reach, so
  an **out-of-order live delivery lands here by accident with no attacker**. What is NOT measured is
  that an unauthorized remote party can trigger it — the probe does not cross the wire, and the two
  sentences are kept apart in the source because only the first is run. The probe **pins** the
  measurement rather than logging it, because the claim is carried in the specification seat's text
  (their floor row is now per-leg, partly on our reading). **Do not build the floor on content or on
  "older mtime" alone**: the control arm is a sender genuinely reverting its own file, which must
  still be followed, and a restored backup carries an old mtime.
- **RECORDING IS FLAT AND UNBOUNDED; AUTO-VERSIONING IS NEITHER — and the difference decides an
  API.** Measured (`make perfreview ARGS="-run TestFeatureCost"`). History recording: **2
  entities and ~1.2 KB per write, with latency that does not grow** (p50 141 µs at 1k writes,
  165 µs at 100k). Affordable, which is why a receiving folder turns it on. **But nothing
  prunes it**, and the cost is per WRITE not per file — a continuously-rewritten file (a log, a
  database, an editor swap file) is ~1.2 GB/day, with no error, until the disk fills.
  Revision **auto-versioning** is the opposite: ~4.5 entities per write and p50 452 µs → 7 ms
  over 5,000 writes, because each write recomputes a trie root over the whole prefix. So
  **auto-version must never be a per-folder toggle** — it is correct on a small curated prefix
  an operator points at deliberately, and a trap as a checkbox beside a shared folder. The
  numbers, the failure modes and the operator's triage order are in
  `docs/architecture/SYNC-LIMITS-AND-FAILURE-MODES.md`; read it before promising anything about
  load.
  **AND THE BOUND THE SUBSTRATE APPEARS TO OFFER IS A NO-OP — do not plan around it.**
  `types.HistoryConfigData.MaxDepth` is documented *"Max transitions per path"* and the recorder
  calls `prune` after every transition, so it reads like the fix. Measured
  (`shellboot/history_maxdepth_probe_test.go`): `max_depth=3`, twelve writes, **twelve
  transitions still reachable**. `prune` walks to the nth transition and returns having written
  nothing — and it cannot easily do otherwise, since transitions are immutable and
  content-addressed, so severing a link cascades a rewrite of the whole retained chain. Its
  comment's *"GC handles cleanup"* names a garbage collector that **does not exist anywhere in
  the cohort**. Setting it is a pure cost: an O(max_depth) walk per write, for nothing. Routed as
  `reviews/CORE-GO-HISTORY-MAXDEPTH-PRUNES-NOTHING-2026-09-07.md`; core-go's own
  `TestRecorderMaxDepthPruning` asserts `count >= maxDepth`, which passes with the feature
  deleted. **This is D20's fifth payout and the first that came back negative** — which is the
  point of running it: *"the substrate does not have this"* is worth as much as *"it does"*, and
  only one of the two is free.
- **SO THE GUARD IS TO STOP, AND STOPPING KEEPS THE OLDEST VERSIONS AND LOSES THE NEWEST — say
  that out loud rather than shipping it quietly** (`shellcmd/history_budget.go`). With no
  pruning below us and no GC, the only lever that bounds disk is to stop adding, and the trade
  is backwards for recovery. It is right for the workload it catches (nobody wants a log file's
  version history) and it is why a tripped limit is a **problem** line on every status reading
  rather than an internal event: the two remaining answers — move the file out, or re-enable the
  named config — are the operator's. The guard acts **once per path and never re-applies**, so
  an operator who overrides it is not undone by the next pass.
  **The signal is free and it is the head pointer.** Every recorded transition writes
  `system/history/head/{tracked-path}`, an ordinary tree mutation, so a prefix watch is exactly
  one event per transition for the price of a map increment — no polling, no scan, and it goes
  quiet by itself when recording stops. **The depth is derived LAZILY**, once per path and only
  after that path has taken 10% of the budget in this process: a cold path costs a map entry and
  never a query, and O(files) dispatched queries at every launch would be a tax every peer pays
  forever to answer a question about a handful of pathological paths. Known limit, invisible
  from the code: a path deep from an earlier run and barely written in this one is never
  measured, so *"no limits tripped"* means "none in this process's view".
  **The exclusion is surgical because the KERNEL's rule makes it so** — `EXTENSION-HISTORY` §6.2
  orders configs by literal-segment count and `configCache.find` returns nil when the most
  specific match is disabled, so a disabled exact-path config outranks the folder's `…/*` and
  stops one file while its neighbours keep recording. That is a reading of a sibling repo, so
  per D19 it is a hypothesis until run: `shellboot/history_budget_e2e_test.go` runs it, **with a
  control arm** that does the same workload unguarded and asserts the chain keeps growing —
  without which the test passes against a build where recording never worked.
  **What it does NOT bound is the aggregate** (N files × budget), which is proportional to the
  data rather than to time. Off with a negative `Config.HistoryPathBudget`, which is the only
  way to re-measure the growth.
- **`ReconcileOnStart` MEANT TWO QUESTIONS AND ANSWERED ONE — `Config.LongRunning` is the other.**
  The flag means *"I have durable declarations to re-establish"*; the catch-up supervisor needs
  *"am I going to be around to take another pass"*. They coincide for the default configurations
  and diverge for in-memory ones, so `--ephemeral` was the one configuration where a burst lost
  files **permanently** — no next launch to recover in — and the one without the loop. The two
  are **or'd, not merged**: making the loop unconditional would put a supervisor behind the
  several hundred peers the suites build through `Bootstrap` and change the load profile of a
  tree that already has load-dependent failures. **The recording guard hangs off neither**, and
  the distinction is worth keeping: a catch-up pass is work a peer schedules, and unbounded
  recording is a consequence of a mount existing that accrues at whatever rate something else is
  writing.
- **Delivery saturation, the catch-up supervisor and the recording guard now reach a PIXEL** —
  the *Sharing Status* panel's third section, plus **Catch up now**. Until 2026-09-07 all three
  were reachable from `entity-shell` and from nothing in the GUI, on the failure an operator is
  most likely to meet and least able to diagnose; `ReconcileOutcome.Delivery` in particular was
  computed on every reading and dropped by an undeclared bridge field (AP49) one step short of
  the screen. **`StatusCatchUp` is a separate export from `StatusReconcile` on purpose**: a
  catch-up does not dial, mount, delete or write policy, so it is safe from a button where a
  reconcile is not — and it is still captioned as a **read**, because only a pass turns
  "declared" into "verified". The discipline every line in that section follows is that
  **measured-zero and not-measured must not render the same**: "nothing dropped" and "nothing
  counted drops" are the same words and opposite facts, and the second is the state in which the
  failure is silently in progress.
- **A SUBSCRIPTION IS A FUTURE TENSE — `sync` now BACKFILLS, and before it did the share
  transferred every file except the ones in the folder** (AP65). `Sync` subscribed on
  `created`/`updated` only, so a folder that already had files in it delivered **nothing**: no
  file in it ever changed again. The operator gesture the product is named after produced an empty
  folder, no error on either side, and a healthy-looking `syncs` row. **It survived because it was
  documented** — `USAGE-SHARE-A-FOLDER.md` listed it under known limitations, the verb printed the
  same sentence, and `sync_e2e_test.go` writes its file *after* the sync **on purpose**, so the
  suite encoded the defect as a premise. `shellcmd/sync_backfill.go` closes it by synthesizing the
  notification the engine would have delivered and dispatching it at the same
  `workbench/blob-resolve:receive` handler via `Executor.ExecuteWithIncluded` — **do not add a
  second materialization path**; mount lookup, the F9 already-current short-circuit, the blob
  closure pull and `local/files:write` are shared by construction. Subscribe **then** backfill, so
  a write during catch-up is carried by the subscription. Gate:
  `shellboot/sync_backfill_e2e_test.go` **plus its control arm**, which runs the same scenario
  with `SkipBackfill` and asserts the folder stays empty — without that arm the positive test
  could be satisfied by anything else replaying history.
  **A first transfer needs NO dial from the publisher.** The backfill runs on authority the
  receiver holds (`share` grants it the listing and the closure), so `accept`'s old line — *"until
  they dial you this folder stays empty"* — was telling operators their files had not arrived
  while the files were on disk. The publisher's dial is for **future change notifications** only.
- **`resync` and `forget` exist for TESTABILITY, and that is a product requirement not a
  convenience.** `resync <peer> <root>` re-runs the catch-up without touching the subscription;
  run it twice and the second pass reports everything **already current**, which is the only
  positive confirmation this flow offers — otherwise "no errors" and "nothing happened" render
  identically. `forget <peer>` / `forget --all` drops syncs, offers, authorization and the
  connection. It exists because everything the flow establishes is deliberately durable, and the
  sum of that is a flow that **cannot be re-tested**: a second run is indistinguishable from a
  stale grant, and that failure presents as a *success nobody can trust*. It **deletes no files
  and unmounts nothing** — received bytes are the operator's — and it does not touch identity.
  Run it on **both** machines; a peer cannot reach into another peer's tree. GUI: **Pull now** and
  **Forget peer** per received folder, **Forget all peers** in *This peer*.
- **`accept` TAKES A DIRECTORY, and a received folder therefore has TWO root names.**
  `accept <peer> <root> [<directory>] [-anyway]` creates the directory, mounts it, authorizes,
  subscribes and backfills in one action (`prepareReceivingMount` in `shellcmd/share_op.go`); the
  GUI asks on the offer row, pre-filled with a fresh `~/entity-shared/{root}`. It **refuses a
  non-empty directory** — their writes overwrite yours and their deletes remove yours — as a typed
  `NonEmptyDirectory` so a panel can offer the override instead of printing prose. **The trap is
  the two names:** the subscription and the sync binding are keyed on the SENDER's root, the mount
  is named after the directory the operator picked, and before this they had to be identical with
  nothing saying so — pick a sensible local name and you got a relationship that established
  cleanly and delivered nothing. `FolderData.LocalRoot` records ours; **read it through
  `ReceivingRoot()`, never `Root` directly**, because `Root` is right in the symmetric case and
  silently wrong in exactly the case the field exists for, which survives every test whose two
  peers happen to agree on a name.
- **`mount` bridges a local directory; `sync` attaches to a REMOTE peer's mount** (M2,
  2026-09-02). `shellcmd/sync_op.go` holds `ShellWorkspace.Sync`/`Unsync`/`Syncs`; the verbs are
  `sync <peer> <root> [-as <local-root>]`, `unsync`, `syncs`. The receiving chain is
  *subscribe to their `local/files/{root}/*` with `include_payload` → `workbench/blob-resolve`
  → pull the blob closure cross-peer → dispatch `local/files:write` locally*, and the durable
  half is `app/workbench/syncs/{peerID}.{root}` (`workbench/sync_binding.go`), restored by
  `shellboot`. **`sync` REFUSES without a local mount to receive into** — a sync writes into a
  mount and does not create one, because a relationship that establishes cleanly and then 404s on
  every delivery is the failure this repo shipped twice before it became a check.
  **The finding that made M2 small: `BlobResolveHandler` had twelve test files and no
  registration outside them.** `shellboot` wired ingest and chain-errors and not this, and
  `subscription` has no create verb, so the entire cross-peer pipeline was built, tested, and
  reachable from nothing a user could run. That is **D23 at the HANDLER layer, where
  `make reachability` cannot see it** — the sweep asks whether a *model* has a surface, and a
  handler is not a model. When you add a handler, the question the sweep will not ask for you is
  *what registers this in a shipped binary, and what verb causes it to be used*. The gate is
  `shellboot/sync_e2e_test.go`, which builds both peers through the real `shellboot.Bootstrap`
  rather than assembling its own handler list — an earlier draft did the latter and would have
  stayed green with the registration deleted.
- **THE SYNC LEG SAYS OUT LOUD THAT IT HAS NO ROLLBACK WITNESS** (`A-33`, ruled 2026-09-12).
  `FolderStatus.RollbackWitness` is `not_supported` for every folder with an incoming leg, with
  one sentence written once (`FolderStatus.RollbackWitnessNote`) and rendered by the shell's
  `status` and by the *Sharing Status* panel's delivery section. The ruling has two MUSTs: **MUST
  NOT synthesize a floor from a quantity minted by neither the writer nor the content** — which is
  why `modified_at` is not it, the filesystem being a third party — and **MUST report the leg's
  witness as `not_supported` while MUST NOT presenting it as rollback-protected.** We met the
  second half by saying nothing, and *saying nothing* is how a reader concludes a leg is fine: the
  static leg next door DOES refuse a rollback, so one defended leg and one silent leg reads as two
  defended legs. **Carrying a witness is the subscription tier's, not ours — do not build a floor
  here.** The quantity, when it exists, is a per-`(sender, subject)` counter from the sender's own
  durable state: a genuine revert is a new write (counter advances, bytes go backwards → follow
  it), a replay is the same write twice (counter stale → refuse).
  **`BY-12` is MEASURED** (`shellboot/delivery_entry_authorization_probe_test.go`): a peer with no
  policy row gets **403 `capability_denied`** at `workbench/blob-resolve:receive`; a peer with one
  gets **404 `no_mount_for_uri`**, i.e. past capability and answered on the merits. So the missing
  floor is reachable by an **authorized** sender and by accidental out-of-order delivery, and not
  by a stranger. The second arm is the anti-vacuity one and it is not optional — without it the
  probe is satisfied by a receiver that refuses everything. It is also the only cross-peer test in
  this repo with **no `OpenAccess` anywhere**, which is the whole experiment (AP63).
