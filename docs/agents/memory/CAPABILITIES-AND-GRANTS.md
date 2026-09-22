# Capabilities & grants

The four grant dimensions, what an absent one defaults to, the per-peer policy table, and the fixture wildcard that deletes a whole stage of the product from the suite.

> Memory, per `AGENTS.md`. Every entry below was paid for by a defect that shipped or
> a measurement that contradicted what we believed. Find by symptom; the mechanism is
> stated before the fix, and `file:line` references point at the code, not at prose.

---

- **AN ABSENT CAPABILITY DIMENSION IS A DEFAULT, NOT AN ABSENCE — AND HERE THE DEFAULT IS "THIS
  PEER ONLY"** (AP85–AP87, 2026-09-10). The kernel made the executing handler's own grant the
  gate on outbound sub-dispatch (0.8.2.19 Delta E1 / F67). §5.2 Dimension 4 defaults an absent
  `peers` scope to `{include:[local_peer_id]}` and **still checks it**, so `blob-resolve` — which
  had never declared one, correctly, because nothing used to consult a handler's internal scope
  outbound — became able to fetch only from itself. **Every file transfer in the product
  stopped**: `make twopeer-sync` 35 checks / 18 failed, not a byte in either direction, *with
  `OpenAccess: true` as well as without* — `peer.OpenAccessGrants()` has the same hole, so the
  cohort's development wildcard is not open access under E1.
  Four things to carry, each of which cost real time:
  **The scope lives in ONE place** — `workbench.BlobResolveInternalScope`, read by the manifest,
  by the chain capability `Sync` mints, and by the migration. Widen it there or not at all.
  **A DELIVERY AND A BACKFILL RUN THE SAME HANDLER UNDER TWO DIFFERENT AMBIENT GRANTS.** A
  catch-up originates locally, under the peer's installed handler grant; a subscription delivery
  runs under the subscription's `dispatch_capability`. Fix one and the symptom is *"`resync`
  works, live delivery does not"*, which reads as a subscription fault and sends you to the wrong
  half of the system. When a handler has more than one entry point, enumerate the ambient
  authority of each.
  **HANDLER GRANTS ARE INSTALL-ONCE, so a manifest change reaches no peer that already exists** —
  and **every test in this repo runs on a memory store, which always mints fresh**, so the
  manifest fix alone is green everywhere and inert on every real machine.
  `entitysdk.AppPeer.RemintHandlerGrant` runs from `Bootstrap`, idempotent **by content** (the
  mint embeds a `CreatedAt`; an unconditional re-mint moves the grant's content hash every
  launch). Gate it across a **process** boundary, with a control arm that installs the OLD shape
  and asserts the failure — without which it passes against a build where the field does nothing.
  Generalise past capabilities: **anything the kernel writes once at construction is invisible to
  a suite whose fixtures are all freshly constructed.**
  **When a whole suite goes red at once with an authorization code, suspect the authority model
  changed before you suspect your diff.** *Everything* is not the shape of a code change.
  Still open and not ours: E1 also strands `system/revision:pull` and the tree-follow fetch-diff
  (4 `entitysdk` tests, confirmed pre-existing by stashing our fix). That handler declares no
  `InternalScope`, so it takes the kernel's `defaultHandlerSelfGrant`, which omits `Peers` on
  purpose — while `pull`'s whole job is to reach another peer. Routed:
  `docs/status/ROUTING-2026-09-10-a-entity-core-go-e1-breaks-cross-peer-ops.md`, core-go tracker
  rows 15–16. **Do not shim it locally** — that hides a cohort-wide question.
  ⛔ **IT IS 20 TESTS AND NOT 4, AND SIXTEEN OF THEM DO NOT CARRY THE E1 CODE** (measured
  2026-09-15). The sweep is **8/10, 20 failures**: 4 in `entitysdk` showing `403 capability_denied`
  outright, and **16 in `shellcmd`** — the `TestE2E_*` local-files replication family — showing
  `502 remote_fetch_failed` at `failed_uri=system/revision` with **zero `capability_denied` in the
  entire log**. Same cause. `ext/revision/pull.go:91-93` formats the downstream failure as
  `status=%d` and **drops the code**, and `ChainErrorLostData` has no message field at all, so the
  403 survives only in a response nothing logs: `revision:pull` answers
  `502 (remote_fetch_failed): revision/fetch on <peer>: status=403`, recoverable **only by running
  one test by hand**. ⭐ **This session re-verified the prior handoff's "all 20 are E1", concluded
  from the logs that the 16 were NOT, and held that until the hand-run refuted it** — so the
  artifacts actively support the wrong conclusion, which is why it is routed
  (`ROUTING-2026-09-15-h-…`, core-go row **21**) rather than just noted. **And they are NOT
  load-dependent**: 4 of the 16 run alone, 56 s, 4/4 fail, one reporting the receiver's revision
  head still at `ecf-sha256:0000…`. Do not reach for this file's three documented load-dependent
  tests to explain them. `[not measured: that all sixteen share the one cause — four were re-run,
  twelve are inferred from the same suite, handler and code.]`
  ✅ **CLOSED BY CORE-GO 2026-09-17, and the inference above was right: 16 of the 16 went green on
  one fix.** `ext/revision` declares an `InternalScope` (entry 1 reproduces the default self-grant
  verbatim; entry 2 gives `system/revision` `fetch`/`fetch-entities` a peers wildcard — narrow in the
  three dimensions a handler can name, wildcard only in the one it cannot), `OpenAccessGrants` grew
  its `Peers:["*"]`, and `pull`'s 502 wrapper now carries the downstream code. **Measured here against
  their tree: `shellcmd` 16 → 1, `entitysdk` 4 → 2, sweep 20 → 6.** *Do not shim it locally* was the
  right call and this is the payout.
  ⛔ **WHAT REMAINS IS A DIFFERENT HANDLER AND A BIGGER QUESTION — core-go tracker row 23.** Entry 2
  deliberately does not carry `fetch-diff`, and that is not the gap: a **continuation** step whose
  target is remote is denied whatever the operation, because the advance executes as
  `ext/continuation`'s handler, which declares no `InternalScope` at all. Measured: widening the
  step's own minted credential to `Handlers:["*"] Operations:["*"]` changes nothing, so the credential
  is not the limiting factor — under E1 a target-minted credential relaxes Dimension 4 and **cannot
  supply a grant**, which is exactly what `EXTENSION-CONTINUATION` §4.2 case 3's `dispatch_capability`
  was the mechanism for. **Three tests measured with that shape** (all `entitysdk`). A fourth red,
  `shellcmd`'s `TestInstallRevisionMirrorChain_…`, is a continuation mirror chain that delivers
  nothing and, hand-run, carries **no code and no chain-error marker at all** — consistent, *not
  established*, and deliberately not folded in: this file's own row-21 entry is about that exact
  inference going wrong in the opposite direction. **Held, not routed** — it blocks nothing shipped
  and that tier is at a release cut.
- **SHARING ONE FOLDER USED TO GRANT A READ OF THE WHOLE TREE** (AP90, fixed 2026-09-10).
  `workbench.SyncSenderGrants` carried `Resources: ["*"]` on three of its four entries, and the
  reconciler writes that row verbatim — so *"share this folder"* authorized every entity and
  every mounted file on the machine. Measured across the wire
  (`shellboot/share_scope_probe_test.go`): a file from an unshared folder came back, **including
  its `content` hash**, which is the next thing `system/content:get` needs. Its doc comment
  claimed the set was "the minimum established by" a delegation test — true of the **handler
  list**, false of the resources, because that test's negative arm drops a whole handler and
  never narrows a cell. **A grant has four dimensions and "minimal" is a claim about all four.**
  Now derived per folder from `workbench.SharedScope`, which carries **this peer's `LocalRoot`
  and the OWNER's `FolderID`** — derive either from the other and you name an id nobody holds on
  any folder received and republished under `both`.
  **`system/content` IS scopeable and we are not scoping it** — an earlier version of this bullet
  said the opposite and was wrong. `EXTENSION-CONTENT` §6.4.2 binds each hash into the tree at
  `{namespace}/{hex(H)}` (lookup is one `tree:get`), and §6.4.1 makes namespace-scoped topology a
  **MUST for multi-party deployments**; the flat mode we run — bare `system/content` namespace, no
  `system/content:ingest` call anywhere in this tree — is the opt-in single-trust-domain one that
  the same section says MUST NOT be the default and calls **"out-of-spec and security-defective"**
  for multi-party. **Not fixable here alone:** core-go implements the ingest binding
  (`bindHashTreePresence`) and **not** the get consult (`handleGet` is a bare store lookup), so
  scoping our grant narrows which label we may claim, not which bytes we may get. Both halves are
  routed.
  **AND FOR MOUNTED FILE BYTES IT IS NOT OURS AT ALL — `local/files` IS THE CHUNKER AND IT NEVER
  CALLS `ingest`** (measured 2026-09-10 on an operator question; the framing was the finding).
  `ext/localfiles/watcher.go:327-345` runs FastCDC and `contentStore.Put`s the blob and every chunk
  **directly**, so `bindHashTreePresence` — the only writer of the §6.4.2 binding — is *unreachable
  for file bytes whatever the app tier does*; and `DOMAIN-LOCAL-FILES` §3.1 pins that handler's
  internal scope to the **bare** namespace, so it is conformant while §6.4.1 says that topology MUST
  NOT be the default. **The party that holds the path→grant relation is the party doing the
  chunking**, which is why the namespace is `local/files`'s to derive (from the mount root — already
  the boundary a share names) and not an application's to remember. Asks: arch **A-20**, core-go row
  **20**, packet `ROUTING-2026-09-10-e-…`. **Do not scope our own grants before A-20 answers** — a
  product that looks namespace-scoped while every file byte stays unbound has deleted the only signal
  that the boundary is missing. We *do* chunk for our own documents at three sites
  (`workbench/markdown_view_model.go:178`, `mount_sweep.go:209`, `ingest_tree.go:140`) using their
  `chunker.ChunkFastCDC` at `types.DefaultChunkSize`, so the bytes agree by construction; what those
  sites cannot do is know a namespace.
  **Until all of that lands the tree grant is the operative boundary** — widen it and you have
  re-opened this. `ShareOfferPrefix+"*"` is the one wildcard left and is named in the source as a
  known disclosure. Note `APP-CONVENTION-SHARE` §2.2 had already ruled this — *"`target` is what
  the grant's `resources` scope covers"* — so it was non-conformance, not just a leak; A-14 asks
  arch whether that rule is conformance-checkable, since nothing anywhere compared the two.
- **A WILDCARD TEST FIXTURE DELETES A STAGE OF THE PRODUCT FROM THE SUITE** (AP63). Every
  cross-peer test in this repo — twenty-four of them — runs under `peer.OpenAccessGrants()`, so
  the whole suite establishes that the transport works and **nothing at all** about permission.
  The kernel's per-peer mechanism is the V7 v7.62 §8 policy table at
  `system/capability/policy/{peer}`, unioned into the grant set by `AssembleInboundGrants`, keyed
  on `hex(identityHash)` **or the Base58 peer-id** or `default`; it had zero uses here. Turning
  the wildcard off surfaced four things, all measured in `shellboot/policy_probe_test.go`:
  **(1) a sync is MUTUAL** — the receiver dispatches in to subscribe and fetch, the publisher
  dispatches back to deliver, so both need an entry naming the other, and one direction alone
  gives you an accepted subscription and an empty folder; **(2) the grant is assembled at
  HANDSHAKE**, so a policy written on a live connection is inert until it is re-established;
  **(3) the peer that DISPATCHES is the peer that must reconnect** — a granter's reconnect does
  nothing for the grantee, who uses its own pooled outbound connection; **(4) a dial-by-address
  authorizes the DIALER ONLY** (`sendReciprocalGrant` is gated on
  `EstablishedViaRendezvousKey()`; *"a dial-by-address is asymmetric — one party requested
  service"*), so a two-way sync needs both peers to dial, each after the other's policy exists.
  The operator recipe is `docs/architecture/USAGE-SHARE-A-FOLDER.md`. The generalisation to
  carry: **when a fixture disables a mechanism wholesale, that mechanism has zero coverage
  however many tests run through it**, and the gap is invisible because everything downstream
  passes.
- **Mirroring another peer's subtree needs a capability that names their namespace.** The
  owner self-cap's `Resources: ["*"]` is peer-**local** under §PR-8, so `tree:merge` 403s on
  every `/{them}/…` target. Use `AppPeer.MintMirrorCapability` +
  `Executor.executeAs`; the destination itself comes from the publisher's signed
  published-root via `AppPeer.MirrorDestination`, never from a caller-chosen string.
