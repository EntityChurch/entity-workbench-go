# Publishing & consuming

The CDN corridor and the live road: minting a signed root, the transport ladder, the verification stack both roads share, and the refusals that were wrong.

> Memory, per `AGENTS.md`. Every entry below was paid for by a defect that shipped or
> a measurement that contradicted what we believed. Find by symptom; the mechanism is
> stated before the fix, and `file:line` references point at the code, not at prose.

---

- **A CONTENT HASH IS AN IDENTITY, SO CACHING ONE IS NOT A FRESHNESS CLAIM** (AP48). `fetch`
  re-fetched everything on every navigation, justified by a sentence that is true of the
  *manifest* — the mutable pointer — and false of the whole content-addressed tree underneath it.
  Measured on the live federation: **7.3 s / 61 requests to open a page, 6.2 s / 60 to click a
  link in it**, 51 of every 60 the same CHAMP nodes, serial, while the publisher's profile
  declared `freshness: "static-immutable+signed-pointer"` in a field we parse and never read.
  `entity-browser-rust` had the answer in a doc comment — *"5 fetches for the first page, 2 for
  the next"*. **The paranoid shape was also the weaker one**: a `Consumer` rebuilt per navigation
  has no `seq` floor, so a correctly-signed rollback replayed page by page was undetectable.
  Now: `fetch.Cache` (blobs by hash, walks by root hash — a trie rooted at H has one key set
  forever), one `Consumer` per publisher per session, a real seq floor (`ErrSeqRollback`), a
  `WalkConcurrency`-wide walk, and `fetch.NewHTTPClient`, because `http.DefaultTransport` holds
  **two** idle connections per host and silently re-handshakes six of every eight concurrent
  fetches. After: **2.4 s / 61, 0.36 s / 4, 0.23 s / 3.** Two rules: only a **completed** walk is
  memoized (a partial one would manufacture a withholding origin locally, permanently), and the
  cache is filled in exactly one place — `Consumer.Blob`, past `decodeVerified`. A test that
  re-uses a warm consumer to measure a withholding origin is testing the cache, not the walk;
  `publish/consume_walk_test.go` says so and uses a cold one.
  ⭐ **THE CACHE HAD A GATE FOR THE MECHANISM AND NONE FOR THE WIRING, AND AN AGGREGATE COUNTER
  CANNOT TELL YOU WHICH** (AP107, fixed 2026-09-15 by sweeping AP106's shape across the tree).
  Measured by mutation: unshare the cache in `BrowseModel.consumerFor` —
  `NewConsumerWithCache(layout, client, m.cache)` → `NewConsumer(layout, client)` — and the
  **`workbench` and `publish` suites stay entirely green**, while a reader that verified a page
  live and fell back to the static road re-fetches every byte it just proved. That is AP100's
  finding one field over: the chooser makes two consumers per publisher deliberate, the cache is
  handed to both for that exact reason, and only the `seq` floor ever got a wiring gate.
  **The part to carry is how the first gate failed.** It asserted on `CacheStats.Hits`/`Misses`
  and passed under the mutation, because one cache is shared by the registry reader and every
  site reader — *registry traffic alone kept the counters moving while every site read went
  uncached.* **A counter summed over N users goes on moving while N−1 of them are dead**, so
  assert on an entry that is discriminated per user: here the **walk**, one per publisher, which
  is also the entry this file calls the one whose absence costs 51 round-trips. Gates:
  `workbench/browse_cache_test.go` (wiring — `Back` re-runs the whole chain, so a re-visit must
  serve a cached walk, take no new misses, and fetch **nothing** that is not a published root or
  a signature over one) and `publish/road_cache_test.go` (mechanism, across the live/static seam,
  **with the separate-caches control arm** that proves the measurement can move). Two notes worth
  more than the fix: the page-is-correct assertion is last in both files and labelled as the only
  one the defect also passes; and a second mutation — disabling the consumer memo while leaving
  the cache shared — **correctly does not fire**, because the property survives it, and a gate
  that fired there would be pinning the implementation rather than the property.
- **`publish/` (the CDN corridor) emits a real signed root as of `0a7423d`, 2026-08-25.**
  `{out}/manifest`
  is the signed `system/peer/published-root`; the http-poll transport profile moved to
  `{out}/transport-profile` (§6.5.4 / D5); the §6.5.3 closure of `root_hash` is uploaded in full.
  **This line said "as of 2026-08-18" until 2026-09-09 and that was the DRAFTING date, not the
  landing date** — the result packet was written against a working tree and the commit followed a
  week later. Three documents inherited the wrong date and a correction packet to another seat
  repeated it; *they* found the real commit. **Date a capability by the commit that carries it**,
  and note that this is our own *"an artifact that exists only in your working tree does not
  exist"* rule failing in the one direction it is hard to see: the artifact did land, so nothing
  ever came back to flag it.
  Three rules that came out of building it:
  - **A signed root and a filtered publish are incompatible** — `Opts.IncludePath`/`IncludeType`
    now **refuse**. The closure obligation would upload the filtered-out entities' bytes anyway
    (a leak the operator did not ask for), and withholding them instead shortens a consumer's
    walk silently. Narrowing the published set is `-prefix`'s job.
  - **`seq`/`predecessor` come from the store, not from the engine.** `ext/publishedroot.Publisher`
    keeps both in process memory and never seeds them, so a batch publisher restarts at `seq=1`
    forever (measured). Ours reads the prior root; the ask is routed to core-go.
  - **A workbench-published site is "verified as of `published_at`", never "verified"** — a quiet
    publisher and a withholding origin are indistinguishable at the consumer (§6.5.3.1, D6/D7).
  Result packet: `docs/architecture/reviews/archive/PUBLISHER-CONFORMANCE-RESULT-2026-08-18.md`.
- **A SITE LIVES AT `/{peer}/sites/{id}/`, AND THE SDK WROTE IT SOMEWHERE ELSE FOR FOUR MONTHS**
  (AP96, fixed 2026-09-12). `APP-CONVENTION-SEMANTIC-CONTENT-SITE` v0.5 §2 **drops
  `content/sites/` by name** — `system/content/*` is the CONTENT extension's namespace, where the
  leaf is always `{hex(H)}` — and registers `sites` as the convention's reserved first segment.
  `workbench`'s two resolvers, `entity-browser-rust` and the live corpus were all on `sites/`;
  `entitysdk.PutSiteManifest` / `PutSitePage` / `SitePrefix` — the surface an application
  developer reaches for, and what `entity-seed-site` calls — were on the retired one. **So a site
  authored through our own SDK was invisible to the Local Site panel and to the Browser panel,
  with every suite green.** There is ONE definition now: `workbench.SitesSubpath` is
  `entitysdk.SitesSubpath`. No migration is owed — anything at the old placement was already
  unreadable by every surface that renders a site.
  Three things to carry, each worth more than the fix. **A round trip through your own constant is
  not a check on the constant** — each package's tests composed the expected path from its own
  copy, so both halves agreed with themselves indefinitely; the gate
  (`workbench/site_paths_agree_test.go`) asserts both against **spelled-out literals**, because
  composing from the now-shared constant could not fail on the segment however wrong it was.
  **A cross-impl fixture measures the half of a corridor that faces it** — the remote resolver's
  only end-to-end exercise is browser-rust's frozen emission, which uses the correct path, so it
  proved the READER right and said nothing whatever about the writer. And **the divergence had
  already been noticed and filed as a coordination detail** in `publish/site_root_scope_test.go`'s
  comment, as a note about scoping a future joint fixture rather than a question about which side
  was conformant — AP45's shape, and the reason that comment now carries the retraction.
- **`workbench.PeerSource` IS THE SECOND `fetch.Source`: read a site by DISPATCHING at the peer
  that wrote it** (W2, 2026-09-12). Same `fetch.Consumer`, same recomputed hashes, same two-hop
  signature, same `seq` floor, same fail-closed walk — **a second verification path is the thing
  that must not exist**, because it would be two code paths for one trust argument with the weaker
  one wearing the same UI. An authenticated connection proves WHO, not WHAT.
  **It lives in `workbench` and not in `fetch`** — `fetch` is deliberately peer-free so
  `entity-fetch` links no peer, no store and no location index, which is also why `fetch.Registry`
  exists at all. Gate: `publish/live_and_static_test.go`, which drives **both projections of one
  published act** and requires byte-identical bodies plus an identical committed key set, with the
  locators the only thing allowed to differ.
  **The seam changed shape and that is AP95.** `Source` was `raw []byte` in three of four
  primitives; a dispatched read hands back a decoded entity, so `Root`/`Blob` now return
  `entity.Entity` and `Leaf` returns the **binding** (`hash.Hash`). `crackPointer` moved into
  `HTTPSource`, because Amendment 6 binds the HTTP projection and a dispatched `tree:get`
  returning the entity is the protocol behaving correctly — a check that fires on conformant
  behaviour on another transport is AP44's false refusal with a citation attached. The mechanical
  check that the seam holds: **`fetch/consume.go` imports no encoding package.**
  Two decisions not to re-litigate. **A peer that has never published is a third state and v1
  refuses it by name** (`fetch.ErrNoPublishedRoot`, passed through `VerifiedRoot` unwrapped, for
  `errNoManifestPrefix`'s reason): *unreachable*, *withholding* and *committed to nothing* send an
  operator to three different machines. **And a grant for a live site is NOT the site prefix** —
  the published-root and its signature live at `system/peer/published-root` and
  `system/signature/*`, outside the prefix they commit to, so a grant scoped to `sites/*` alone
  yields a peer that serves every page and cannot be verified at all, presenting as *"this
  publisher has never published"*, i.e. as the other machine's fault. Both arms are gated.
  Known and named rather than hidden: one dispatch per CHAMP node (`Source.Blob` takes one hash
  while `content:get` takes an array), and `leafAt` costs an extra round trip because the one door
  into the content store stays one door.
- **`publish` WOULD SIGN A ROOT COMMITTING TO NOTHING, AND ITS OWN GUARD FOR THAT COULD NEVER
  FIRE** (AP97, fixed 2026-09-12). `mintSignedRoot` refuses when `trieRoot.IsZero()`;
  `tree.BuildTrieForPrefix` over a prefix with no bindings returns the hash of the canonical
  **empty** CHAMP node, which is non-zero and identical under every identity. So a mistyped
  `-prefix` emitted a well-formed, correctly-signed, entirely empty origin — and an empty root
  answers *absent* to every key with a valid signature over it, which is exactly what
  `fetch.ErrEmptyEnumeration` exists to say carries no information. The refusal is now in
  `Publish`, **on the binding count it already computed and printed**, not on the root hash. Guard
  on the fact, not on a proxy for it; and a guard with no control arm asserting its condition is
  reachable is a handled case that was never handled.
- **The consume side is a JOURNEY now, not an inspector** (2026-08-21). `fetch.Registry` +
  `workbench.BrowseModel` do `name → binding → transports → the target's signed root → walk →
  page`, and the three surfaces are `entity-shell`'s `registry` / `browse` / `open`, the Avalonia
  **Browser** panel, and `entity-fetch -registry`. Four rules from building it:
  - **Do not re-implement §6a.4.** `entity-core-go`'s `ext/registry/peerissued` has the whole
    algorithm *and* an `HTTPPollReader`. `entitysdk.AppPeer.PinRegistry` registers **their**
    backend; `fetch.Registry` exists only because their `Resolve` needs a store + location index
    and `entity-fetch` links neither. The split is held by
    `workbench/registry_differential_test.go`, which runs both over the same frozen bytes — **if
    that test goes, `fetch`'s resolver goes with it.**
  - **`transports` on a §3 binding — CLOSED 2026-09-09, and our reading was the normative one.**
    It carries **hashes**: `EXTENSION-REGISTRY` §246 is a `[MUST, v1.21]` — *"An implementation MUST
    NOT inline an endpoint object, a profile body, or any other map in this field"* — ruled
    2026-08-21 and folded, which we did not know because **a ruling that lands in a spec is
    invisible to the seat that asked for it unless someone says so** (arch's words; the third
    instance here). core-rust is conformant, and **the live federation now serves the hash form**:
    `make consume-live` prints `transport carried hash in the binding` where it used to print
    *"carried inline"*, on a binding reissued 2026-08-24, three days after the ruling. Keep
    `TransportRef.Kind` — it is what let a one-command check answer this, and an origin can still
    serve a pre-ruling binding until its ttl runs out.
    **What is NOT closed is our gate.** `workbench/registry_differential_test.go` pins the
    divergence against a **frozen** fixture, so its own stated exit condition — *"if core-go
    resolves, delete this test"* — is **unreachable by construction**: those bytes carry the inline
    form forever. Re-cut the fixture from a current emission, restore the full agreement
    assertions, and drop the vacuity caveat on `TestBothResolversRefuseASubstitutedBinding`. **A
    gate whose success condition cannot occur is not a gate, it is a monument** — and the tell is a
    self-retiring test whose retirement depends on data it owns.
  - **A CROSS-IMPL CLAIM IN A DOC COMMENT IS INVISIBLE TO REVIEW FOREVER, AND A "DETERMINISTIC
    ENCODING" TEST IS NOT THE GATE FOR IT** (AP83, 2026-09-09). `entitysdk/site.go`'s header said we
    *"hold to byte-equivalence with [browser-rust's] `to_entity` output"*, which reads as settled and
    was never run. The nearest test, `TestSiteManifest_DeterministicEncoding`, encodes **twice and
    compares** — self-consistency, green for any encoder that is merely stable, and green with the
    other implementation deleted from the universe. The two files that *do* carry their vectors
    verbatim cover link classification and asset refs, so the one shape
    `APP-CONVENTION-SEMANTIC-CONTENT-SITE` §9 turns into a ratification gate was the one nothing
    measured. **The tell is a test whose name states a cross-impl property and whose body names only
    our own types.** Now gated: `fetch/site_entity_crossimpl_test.go` decodes every site entity in
    their frozen emission through our types and re-encodes it (byte-identical, 3 manifests + 11
    pages), and rebuilds **their** CHAMP root from **their** bindings in a **fresh** store.
    Two things worth more than the fix. **The round trip is its own anti-vacuity arm** — `ecf.Decode`
    into a Go struct silently drops undeclared fields (AP49's shape), so a dropped field cannot
    survive the re-encode; it returns as a byte difference. And **before designing a joint rig with
    another seat, ask what the frozen fixture you already hold answers on its own**: the question
    browser-rust had scoped as needing a live two-peer run — *"can two independent publishers produce
    a byte-identical root at all?"* — was answerable offline, from bytes both seats had been sitting
    on since August, and two of its three links came back green in an afternoon. That is D20 pointed
    sideways at a counterpart's artifacts rather than down at the kernel.
  - **The walk is the authority; a served listing is a menu** (§6a.3a). `Registry.Enumerate` walks
    the signed root over the `by-name/` prefix and reports both sets *and their disagreement*.
    The spec authors' standing ask: say which one produced a row **in the artifact**, not only
    in the code.
  - **An origin-relative transport prefix resolves against a scheme://host:port, never against the
    path the profile was fetched under** (`fetch.OriginRoot`). A registry served at `host/registry`
    names domains at `host/docs`.
- **A BINDING'S `transports` IS A RANKED LIST AND WE WERE TAKING THE FIRST ONE** (AP98, fixed
  2026-09-12). `priority` is landed normative text — `EXTENSION-NETWORK` §6.5.1a D1, Amendment 8 Q1,
  lower preferred, **default 100**, gated 3-way green — and `EXTENSION-REGISTRY` §4.1.1 points a
  binding's `transports` at it by name. We read the field **nowhere**: `OriginFor` walked the array
  and took the first entry that decoded as `http-poll`. A publisher's declared preference was
  discarded in silence, with a correct page at the end of it, and the symptom (a reader on the slow
  mirror) reads as *the publisher's* misconfiguration. `fetch/transports.go` is the fix —
  `TransportsFor` returns a ranked `TransportOptions` with the declines kept, `OriginFor` is its
  static half and keeps its signature. **`advertised_at` is a MUST NOT** (D3, wall-clock and
  skew-prone) and is read by nothing; the gate proves it with two profiles differing only in that
  field. **Ranking within a class is the spec's; choosing between static and live is the CALLER's**
  — D1 sorts profiles *of the wanted `transport_type`*, and `entity-fetch` links no peer, so `fetch`
  ranks and reports while the caller says which roads it has.
  Three things to carry. **A wire field your code never names is either dead or a rule you have not
  implemented, and the two look identical from inside the code** — grep the struct's fields against
  your own source. **A rule you are about to design is a rule to search for first**: D20 aimed at
  the *spec* for the second time, and it keeps not happening because a missing rule and an unread
  rule feel the same while you are writing the replacement. And **D1's tie-break is unreachable
  here, which is routed and not patched** — `profile-id` is *the final path segment* of the
  profile's tree path (core-go gets it from `path.Base(e.Path)`, correctly), and **neither carriage
  that moves profiles between parties carries a path**: a registry binding carries content hashes
  (REGISTRY §3 `[MUST, v1.21]`), a `system/peer/transport-set` carries members inline and says
  *"array order is NOT significant"* while its rule 6 requires D1 order. We stable-sort and
  **disclose** it (`TransportOptions.TieBreak`); a locally-invented tie-break is deterministic per
  implementation and *different per implementation*, which is the failure the rule prevents wearing
  the look of the rule. Ask **A-35**, packet `ROUTING-2026-09-12-b-…-the-d1-tie-break-key-does-not-survive-either-carriage`.
  **And a live profile is no longer a decode failure** — every non-`http-poll` family used to land
  in `Skipped` as *"type …/tcp, not …/http-poll"*, so a peer saying *dial me* read as a malformed
  binding; a static-only reader now refuses by naming what it saw.
- **THE FRESHNESS SENTENCE IS CARRIED IN THE OUTCOME, AND THE TWO MODES MAY NOT BORROW EACH OTHER'S**
  (`fetch/freshness.go`, 2026-09-12). `Source.Describe()` puts a `Mode` on `VerifiedRoot`, and
  `VerifiedRoot.Freshness()` is the **one** composition that switches on it — both hand-written
  copies are gone (`BrowseModel.goTo`, `ConsumeOutput.FreshnessNote`). A surface that has to recall
  which consumer it built in order to caption a chain will eventually caption it wrong, always in
  the confident direction; that is `shellcmd/status.go`'s `Reconciled` rule one corridor over.
  **What a live read buys is the REMOVAL OF A PARTY, not fresher bytes.** A static origin is a third
  party that can serve an arbitrarily old correctly-signed root and say nothing; asking the
  publisher removes anyone in that position. It does **not** establish the root is current — a
  publisher that has not republished in a year answers instantly and the exchange looks no
  different, so *quiet publisher* survives both modes and only *withholding origin* is removed.
  `TestLiveFreshnessDoesNotClaimTheRootIsCurrent` exists to fail when that sentence drifts into the
  other one. **An unnamed mode produces NEITHER claim** — falling back to the static sentence would
  be the safe-looking choice and is wrong, because it buries a Source that never implemented
  `Describe` under a true-sounding claim. Gates: `fetch/freshness_test.go` for cross-exclusion, and
  the arm that matters in `publish/live_and_static_test.go`, where both sentences come out of **real
  verifications of one published act** rather than struct literals — the hand-built one cannot fail
  if `Describe` is never called on a real read.
  ~~⛔ **Owed: nothing a user can reach takes the live road.**~~ — **CLOSED 2026-09-13**, see the
  chooser bullet below. It was correctly sequenced behind W4's public grant: a road that exists and
  leads to a publisher who authorizes nobody fails as *"that machine is broken"* on the reader's
  screen.
- **THE CHOOSER: RANKING IS THE SPEC'S, PICKING A CLASS IS OURS, AND THE LADDER STOPS AT THE FIRST
  VERIFIED ROOT** (`workbench/browse_road.go`, 2026-09-13). `TransportsFor` ranked and `PeerSource`
  read, and between them sat nothing — `BrowseModel.goTo` called `OriginFor`, the static half, so
  **every shipped surface took the static road however loudly a publisher advertised itself as
  reachable.** Now `roadsFor` partitions the ranked candidates into roads this browser can drive and
  `travel` walks them. Four rules, each earned:
  **Live first, and it is not a stronger check.** D1 orders profiles *of the wanted transport_type*
  and says nothing about static-vs-live, because that depends on what the consumer can do —
  `entity-fetch` links no peer. So the class preference is ours, and its whole justification is that
  a live read removes a party who could be withholding a newer root. An authenticated connection
  proves WHO, not WHAT; one `fetch.Consumer` verifies both roads identically.
  **Within a class the order is D1's and the chooser is a PARTITION, not a re-sort** — otherwise a
  publisher's declared preference between two mirrors is discarded by the fix for discarding it.
  **The ladder falls through on a decline and stops at the first VERIFIED root.** §6.5.1c rule 6
  makes a profile *"not a promise that it currently answers"*, so a dial that fails moves to the next
  road; but answering an incomplete walk on the live road with an origin's older copy would destroy a
  finding **about the publisher** and put bytes on screen while doing it.
  **A family we cannot dial is declined BY NAME** — core-go implements `Connect` (TCP) and
  `ConnectWebSocket` and no other, so an `http` transport profile is conformant and undrivable;
  handing it to `Connect` dies in `net.SplitHostPort` with *"too many colons"*, which reads as the
  publisher's fault. The decline says *"this browser dials tcp and websocket"* — a fact about us.
  **A peer-id address with no origin now works when we already hold a connection**
  (`canReachLive`). The old refusal — *"a peer-id address needs an origin to fetch from"* — was true
  about peer-ids (§6.5.4) and false about that situation, i.e. AP44 again, and it is the
  configuration a laptop is permanently in. It never dials a guess and never invents an address; the
  two qualifying cases are *it is us* and *we hold a connection*, and the pool tags neither direction
  (AP82's neighbour) — acceptable here because the worst case is a dispatch that fails and a chain
  row saying so, and **not** acceptable for minting a grant or binding an identity.
  Surfaces, same change (D23): the shell's browser takes the workspace peer (`bareBrowserOf`) and
  `BrowseOpen` takes a peer handle, **0 meaning none and an unknown one being an ERROR** (AP33) —
  falling back to peer-less would make the live road silently unavailable and the symptom land on
  the other machine. Gates: `workbench/browse_road_test.go` (order, declines, the no-peer arm, and
  the undialable-family arm with its websocket anti-vacuity control) and
  `publish/live_and_static_test.go`'s `TestBrowseModel_PeerIDAddressTakesTheLiveRoadWithNoOrigin`,
  which drives the model every surface drives, over a real published act, with **no origin in
  existence** and a peer-less control arm that must refuse.
- **A CHECK'S MEMORY MUST BE KEYED BY WHAT THE RULE IS ABOUT, NOT BY THE OBJECT THAT PERFORMS IT**
  (AP100, fixed 2026-09-13). The §3-RES.4 `seq` floor was a field on `fetch.Consumer`, which is a
  per-publisher floor for exactly as long as a publisher is reachable one way. The chooser makes two
  consumers per publisher deliberate, and **the split points the wrong way**: a reader that verified
  seq=7 live and falls back to a static origin would start from no memory and accept a replayed
  seq=3 in silence — the fallback being the road a chooser reaches for, and a third party serving a
  stale root being what the static mode exists to confess. Now `fetch.SeqFloor`, shared through
  `BrowseModel.floorFor`, keyed by peer-id **and nothing else**. Note the hazard had been named and
  routed around rather than fixed — `consumerFor`'s own doc comment said *"two consumers for one peer
  would each hold their own seq floor, which is how a floor stops being one"* and keyed defensively
  against a narrower version of it, which is AP45's shape. Two gates because one cannot see the
  other's failure: the mechanism in `fetch` (with the control arm proving an unshared pair still
  takes the rollback) and the **wiring** in `workbench` — a correct `SeqFloor` reached by two
  different keys is the defect wearing the fix.
- **`publish` IS ONE ACT WITH TWO PROJECTIONS, AND THE GRANT IS THE DANGEROUS HALF** (W4, 2026-09-12).
  Publishing is signing a `system/peer/published-root` over a prefix; the static directory and the
  live serve are projections of that one act, which is why `publish.MintRoot` and `publish.Publish`
  run the same code past the same refusal. **Both go through `prepareMint`** — the AP97 empty-prefix
  guard was in `Publish` alone, and adding a second entry point beside it would have re-opened the
  defect on the newer road. Surfaces: the `publish` verb (`shellcmd/publish_op.go` + `cmd_publish.go`)
  and the *Local Site* panel's docked publish bar, in the same change (D23 — the act the whole
  consume side exists to read had no verb and no pixel).
  **The public grant is `workbench.PublicSiteGrants`, derived from the prefix the root committed
  to**, and it refuses an empty prefix by name: `MintRoot` will sign over the whole tree, which is
  legal, and a PUBLIC grant over the whole tree is `Resources: ["*"]` in a different spelling — AP90
  aimed at everybody instead of at one named peer. It carries `system/peer/published-root` and
  `system/signature/*` **as well as** the prefix, because those two live outside the prefix they
  commit to and a site-shaped grant that omits them serves every page and cannot be verified at all,
  presenting as *"this publisher has never published"* — an accusation against the publisher for
  something the reader's own grant caused.
  Four things this cost, each worth more than the feature:
  **A CATCH-ALL AUTHORIZATION ROW IS SHADOWED BY EVERY SPECIFIC ONE** (AP99). The V7 §8 table
  resolves `hex(identityHash)` → Base58 peer-id → `default` and **returns at the first match**
  (`readHandshakePolicyGrants`); it is not a union. So a public site was readable by every peer on
  earth **except the ones already named by a share** — the only peers an operator has to test with,
  measured as `403 capability_denied` at `system/peer/published-root`. Fixed by deriving the public
  grant into every per-peer row (`desiredGrantsByPeer`), and **deliberately not** by propagating a
  hand-written `default` row, which would be widening somebody else's grant on their behalf.
  Generalise: *a fallback rule is not a floor* — ask of any catch-all, **who is excluded by being
  known to us?**
  **A PEER HAS EXACTLY ONE PUBLISHED ROOT**, so publishing a narrower prefix stops committing to
  everything outside it, under a valid signature — a site going dark that a reader cannot tell from
  a site that never existed (`fetch.ErrEmptyEnumeration`'s whole reason). `SignedRoot.PriorPrefix` +
  `narrowedFrom` say so. Note the edge its own test caught: §3.3a spells the universal tree `"/"`
  and everything else without a leading slash, so an unstripped `HasPrefix` reports *publishing the
  whole tree* as taking a site dark — a warning firing on the one move that cannot lose a key.
  **A VERB THAT CHANGES AUTHORIZATION MUST RE-DERIVE AND RECONNECT** — grants are assembled at
  handshake (AP63), so `-public` runs `ApplyDeclaredPolicy` + `refreshGrantConnection` for every
  declared peer. `direction` shipped without that for four days with a doc comment claiming
  otherwise.
  **AND THE PUBLIC ROW WIDENS THE CONTENT EXPOSURE FROM ONE NAMED PEER TO EVERYBODY.** Measured, not
  argued (`shellboot/public_site_scope_probe_test.go`, logged rather than asserted for AP65's
  reason): a stranger fetched a file from an unshared folder through `system/content:get` by knowing
  its hash. That is §6.4.1's unimplemented get-half, already routed; what holds the line is that the
  **tree** grant is what discloses hashes, which is why the tree boundary is the assertion and the
  content probe is a log. `publish -private` removes the row; `access` marks it *NOT A PEER*.
  **Three of the four defects above were found by TYPING THE COMMANDS, not by reading the code**
  (AP71, and every suite was green throughout). (1) With nothing published, the access block printed
  *"this root is signed and in the tree"* — the renderer branched on the grant and not on whether a
  root existed. (2) `prepareMint`'s refusal ended *"check the prefix"* when the prefix was correct
  and the STORE was empty: `entity-shell` defaults to an **in-memory** store, so `-identity NAME`
  alone yields the right peer-id and a blank tree, and the message sent an operator to re-read the
  one thing that was right (AP44). It now names the tree's actual top-level prefixes, which
  separates *wrong prefix* from *wrong store* at a glance — when you refuse, say what was on offer.
  (3) **`publish -private` re-published at the CONSTANT default**, silently moving the committed
  prefix away from the one the operator had chosen. The default is now **sticky** — the prefix the
  current root already commits to, falling back to `sites/` only on a first publish
  (`currentPublishPrefix`). *A default is what to do when nothing is known, and after the first
  publish something IS known*; a constant one made the commonest action (re-publish after adding a
  page) quietly change what the peer commits to, with no error anywhere.
- **ONE ORIGIN MAY HOST SEVERAL PEERS, and the well-known `transport-profile` names exactly
  one of them** (AP44). `{origin}/transport-profile` is a **cold-start entry point**
  (NETWORK §6.5.3, Mode A2), never an exclusivity claim. We read it as *the* peer that origin
  serves and refused on a mismatch — in **four** places, each phrased as a security property —
  which made the cohort's only public registry unreachable from every naming surface we ship
  (`entity-fetch`, the shell's `registry`/`browse`/`open`, the Avalonia Browser and Publisher
  Verify panels). The live registry origin co-hosts its site peer and its registry peer, and
  the well-known object features the site.
  **`fetch.Layout.RebaseTo` is the answer, and the rule it encodes is the transferable part: a
  derivation is safe exactly when SOMEONE ELSE'S KEY CHECKS IT.** Consumers re-base (the pinned
  peer's own root signature fails closed on a wrong substitution, one hop later);
  **`registry issue` still refuses**, because there the derived reach goes into a binding *we*
  sign and nothing downstream could catch a bad one. Substitution is whole-path-**segment**
  exact, never substring — same discriminator `treeBase()` uses. Layout provenance is **three**
  states (discovered / re-based / pinned) and a surface that collapses the middle one into
  "discovered" claims the origin advertised something it did not.
  Two generalisations worth more than the fix: **a fixture that models one instance of a plural
  relationship cannot fail on the plural case** — every fixture in this tree serves one peer per
  origin, so the whole class was untestable and green — and **a false refusal reads as rigor and
  leaves no wrong answer to catch**, so it is always attributed to the other side. Grep your
  refusals for messages that assert a fact about the world and ask which sentence makes each
  one exclusive.
- **`{origin}/entity-deployment.json` carries the two facts an operator would otherwise have to
  be told** (AP45) — `name_registry_pin` (origin + peer-id of the registry this deployment uses)
  and `home_site` (which site is the front door). `fetch.LoadDeployment` reads it; it is
  **unsigned, origin-supplied, and a HINT**. Two rules: a pin taken from here is
  **trust-on-first-use and every surface must say so** (an operator's pin is the one fact the
  origin did not choose; this one the origin chose for you, and while it cannot forge a binding
  for a key it does not hold, it can hand you one it does), and `home_site` may only **select
  among sites the signed root already commits** — an origin naming a site the walk does not
  carry is ignored, never followed.
  **The reason this is a rule and not a footnote:** we had already found this file, written its
  name and contents into a doc comment, and concluded from it that there was nothing to read.
  Meanwhile we opened the wrong front page on a domain that declares one, and made an operator
  hand-type a peer-id the origin publishes. *A dismissal recorded as a doc comment is invisible
  to review forever* — a `TODO` invites work, a paragraph explaining why a limitation is correct
  closes the question permanently. Treat **"there is no way to know X" about your own tree** as
  a claim needing the same evidence as one about a sibling repo, and prefer a flag that says
  *unknown* over prose that says *unknowable*.
- **An empty result is a claim, and "0 names" is often a confident wrong answer.** With no pin
  supplied we adopt whichever peer the origin features; if that is a site peer, the registry walk
  honestly commits zero bindings and the surface reports an empty registry. `fetch.NameSet` now
  carries `NotARegistry` + `Diagnosis` + `OtherPrefixes`, and **a surface must render the
  diagnosis instead of an empty list.** This is AP44's shape in the form that needs no refusal to
  go wrong, and it is the one the operator hit first.
- **`fetch/` enters through the publisher's advertised layout and derives nothing** (2026-08-19).
  `fetch.Layout` is built from the http-poll transport profile at `{origin}/transport-profile` —
  peer-id, all three URL prefixes, content layout, both suffixes. The one convention left is that
  well-known object. **Never re-derive a URL here**: the pre-2026-08-19 version derived all three
  and could not fetch a byte from our own publisher (AP21). Two live joins exist in this cohort and
  we consume both — if the `tree_url_prefix`'s **last segment is exactly the peer-id** it is
  peer-rooted (browser-rust), otherwise the peer-id is ours to append (ours); arch owns the ruling
  that will kill one branch. Content URLs use the **33-byte wire hex** (66 chars, `00`-prefixed),
  which §6.5.3.1 MUSTs. core-go's `types.BuildContentURL` **used to** hex the digest only; as of
  their `7f39eb3` it hexes `h.Bytes()` and agrees with us — but we still build our own, because
  noticing is not adopting: `fetch/crossimpl_test.go::TestContentURLUsesWireHexNotDigestHex` is
  the tripwire that logs the agreement, and the delegation owes a round-trip measurement against
  **both** live joins before `fetch.contentURL` becomes a call. Cross-impl gate:
  `fetch/testdata/crossimpl-rust-site/` (their bytes, frozen, provenance in its README).
