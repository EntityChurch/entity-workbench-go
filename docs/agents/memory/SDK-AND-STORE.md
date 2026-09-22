# SDK & store

Addressing, path qualification, what routes remotely versus locally, derived indexes that need rebuilding at open, and the SDK shape rules.

> Memory, per `AGENTS.md`. Every entry below was paid for by a defect that shipped or
> a measurement that contradicted what we believed. Find by symptom; the mechanism is
> stated before the fix, and `file:line` references point at the code, not at prose.

---

- **A tolerant fallback that turns malformed input into a well-formed entity is a bug, not
  leniency** (AP33). `put`'s `// Not valid JSON — treat as literal string` wrote an entity
  that decodes nowhere, and the failure surfaced an hour later in a consumer as *the
  consumer's* bug. Be liberal about input that was never trying to be structured; refuse
  input that plainly was. Note also that the shell's `SplitArgs` strips quotes as **shell**
  quoting — JSON payloads must be single-quoted (`put P T '{"a":1}'`), which the usage doc's
  examples show and which is easy to miss.
- **A `.list` artifact is a `system/tree/listing` ENTITY, not text** (`ENTITY-SYSTEM-REFERENCE`
  §168). `entity-browser-rust` emits newline text; `fetch.parseListing` reads both and
  guards on printability, because feeding a CBOR body to a newline splitter yields fragments
  that then "disagree" with the signed key set — a false alarm about the other side's honesty.
- **Read the source before asserting** path shape / addressing / namespace claims (see
  AGENTS-STANDARD). The whole tree is peer-id-namespaced, so **"peer-id keyed" is almost
  never a valid distinguishing claim** — if you reach for it to explain why something
  matters, you're probably about to mislead. Cite `file:line` in test comments and doc
  explanations.
- The project measures everything against the **27 disciplines (D1–D27)**, ten review
  questions, and anti-pattern catalog (AP1–AP117) in `docs/architecture/DISCIPLINE-CHARTER.md`.
- **A COPY OF A LIVE SQLITE STORE IS NOT THE STORE, AND THE MISSING WRITES READ AS ZERO ROWS**
  (AP76). File-backed `SqliteStore` opens **WAL** (`core/store/sqlite.go`, `buildSqliteDSN`
  defaults `JournalMode` to `"WAL"`), so everything since the last checkpoint is in the `-wal`
  sidecar. `podman cp /data/store.db` then `sqlite3` on the copy therefore answers a question
  about a store that does not exist — **measured at the same instant on the same store: 0 rows
  naming the file from the main file alone, 2 with `-wal` alongside.** That is how *"the
  receiving peer binds no file entity"* was reported as the blocker ahead of M3, with four
  further conclusions built on it, when the receiver binds correctly in every configuration.
  Copy the sidecars, read the file in place, or ask the running peer. Two generalisations,
  both worth more than the recipe: **silence from a new instrument is a claim about the
  instrument first** — before an absence becomes a finding, point the same instrument at a
  case you know is populated (the publisher's own binding was right there and would have
  failed identically) — and **when you have just finished proving that every surface lies, the
  replacement you reach for needs its own control arm**, because the reasoning that retired
  the surfaces is exactly what makes the new instrument feel beyond question.
- **A model with no shipped surface is not shipped** (D23). Landing a renderer-neutral model
  is half a feature; the other half is a verb, panel, or menu entry a user can reach, in the
  same session. Three times now — the name arc, the handler browser, `PeerLiveness` — every
  layer was green and no edge connected them, and two of the three were found by audit
  because no test crosses "can a user reach this". **`make reachability`** is the sweep.
  **And a READ-ONLY surface over a READ-WRITE model is the same violation, one the sweep cannot
  see** (AP57): it asks whether a model has *a* surface, and one `Render` export satisfies it
  completely. The Local Files panel shipped able to list mounts and unable to make one, with
  everything green. **The tell is a panel with no verb in it** — a bridge area whose exports are
  all `Open`/`Render`/`RegisterWake`/`Close` while the model behind it has operations. When you
  find one, extract the operation so the verb and the panel share it (`shellcmd/mount_op.go` is
  the worked example) rather than reimplementing it in the renderer.
- **`Store.List` RETURNS PEER-QUALIFIED PATHS — never `TrimPrefix` one with a relative prefix**
  (AP58). The prefix you pass in is canonicalized by `NamespacedIndex`; the entries come back
  carrying the `/{peer-id}/` they are stored under. `strings.TrimPrefix(e.Path, "system/config/...")`
  therefore trims **nothing**, and the `== ""` / `Contains(root, "/")` / map-key test after it gets
  a confidently wrong answer. This shipped: the Local Files panel filtered out every row and
  rendered *"no filesystem mounts on this peer"* for a peer that had one, and `mounts` printed the
  qualified path as the root NAME. Use **`workbench.TreeRelative` / `RelativeUnder`**, which are
  idempotent and safe on a path you built yourself, so applying them is never wrong.
  **The reason no test caught it is the part to carry:** `NewStore(memory, memory)` has no
  `NamespacedIndex`, so the cheap scaffolding returns bare relative paths and the arithmetic works
  — every model test in `workbench/` used it, and namespacing only appears on a `CreatePeer`
  store, which is what every real peer has. **A fixture that omits a wrapper the production object
  always has cannot fail on anything the wrapper changes.** Peer-backed assertions live in
  `workbench/tree_path_test.go`; put new ones there rather than beside the model, because the store
  being real is the whole point.
- **NEVER CALL INTO THE STORE WHILE HOLDING A MODEL'S LOCK** (AP60). `Store.OnPrefixChange`'s
  cancel waits for its delivery goroutine to exit, and that goroutine is inside your event handler
  taking your mutex — so a `Close` holding the lock across the cancel deadlocks, silently. No
  panic, no race report: `make test-each` **sat on the workbench suite for sixteen minutes**. The
  attach side is the same hazard and is louder only by luck, because `OnPrefixChange` delivers its
  seed *synchronously on the caller's goroutine* when the store has no watch hub. Grab the cancel
  funcs under the lock, release, then call them. And when you gate a race, **loop it and run a
  control arm** — the first version of that gate passed against the deadlocking code, because one
  close under churn does not reliably catch the deliverer mid-contention.
- **A DERIVED RUNTIME INDEX over a persistent store must be rebuilt at open, and nothing in the
  read path will tell you it wasn't** (D26, AP62). `subscription.Engine.Load` rebuilds the
  engine's `pathIndex` from `system/subscription/{id}` entities; its doc comment says it "must be
  called after SetLocationIndex and before StartDelivery", core-go's own daemon calls it, and
  `entitysdk.assembleAppPeer` made both neighbouring calls and not that one. So **every
  subscription a peer ever made was in its tree and dead after a restart** — and since every
  mount here is driven by a subscription, a restarted peer resumed its watcher, listed the mount
  as healthy, and never produced another document. That is the same end state
  `workbench/mount_binding.go` fixes, by a second route that fix could not close, and the
  mount-binding work was in this code and missed it. **This is AP39 one extension over** — same
  shape as the query index, worse in kind, because a subscription is not a read path: it is what
  makes a write *cause* something. Two rules. **Grep the dependency for a `Load`/`Rebuild`/
  `Restore` you are not calling** before scoping a build — both instances were adopt, not build.
  **And gate it across a process boundary, asserting on the DERIVED structure**: the tree keeps
  its copy either way, so a test that re-reads the entity passes against the broken build
  (`entitysdk/subscription_restart_test.go`).
- **"identity" vs "keypair" — keep these distinct** (the word is overloaded across the
  codebase; full discussion `DEPLOYMENT-DIRECTION.md §3`):
  - **keypair** — the bare Ed25519 keypair (what `crypto.LoadIdentity/SaveIdentity`
    load/save — upstream misnomer; don't propagate it).
  - **identity bundle** — the on-disk directory shape
    (`entitysdk/identity_bundle.go::IdentityBundle`).
  - **identity entity** — the V7 hash-addressed public-key entity (`peer.Identity()`).
  - **identity extension** — the attestation + quorum + identity stack (`ext/identity/`).
- **`shell.WD` is stored canonical `/{peerID}/...`, never `/@alias/...`.** The alias form is
  display-only (`shellpanel.Prompt()` applies `AliasFor` at render). Store-side surfaces
  (`Store.List`, `NamespacedIndex.canonicalize`) **panic** on a leading `@alias`. When
  seeding WD in a renderer/test, use `shellcmd.Path("/" + peerID + "/")`.
- **Path syntax migration owed:** `alias:path` ships today, but `@alias` is the pinned
  peer-id substitution sigil (`:` is reserved for `<handler-path>:<op>`), so prefer `@alias`
  in new user-facing docs/examples to avoid re-churn when the migration lands.
- **SDK shape:** `entitysdk.AppPeer` **is** a peer (always has a tree + full handler set +
  dispatcher + pool); `entitysdk.Client` is **not** (bare TCP wrapper, deferred). The
  keypair you operate under picks the surface; never open a fresh client connection to a
  peer your AppPeer already pooled under the same identity. Don't add `PeerSurface` /
  `*From` variants — URIs (`entity://{peer-id}/...`) encode the target.
- **Build the typed SDK clients against `core/types`** via `entitysdk/extdispatch.go`'s
  `extDispatch` — do **not** consume core-go's `ext/{identity,role}/sdk` proto-SDK (Go-only,
  no SDK error mapping; would puncture `*entitysdk.Error` predicates). **Layer-2 algorithm
  contract:** any operation whose bytes land in the tree (chunking params, slug/path
  canonicalization, CBOR canonical encoding, subscription pattern matching, …) must be
  byte-identical across impls — extract named constants + reference vectors. Layer-1
  ergonomics may vary freely.
- **`AppPeer.Get`/`List`/`Has` ROUTE BY PEER-ID, so a peer-qualified path on THOSE
  helpers is a remote read** (AP11). `List("/{them}/…")` dispatches to *that peer* and
  returns *their* tree, not our cached mirror of it. To assert on a mirror — or on anything
  we hold in another peer's namespace — read `AppPeer.Store()` (L0) instead. A test that
  gets this wrong is green whether or not the mirror was ever written.
  ⛔ **AND THE ENTRY'S OWN WORST INSTANCE WAS IN THIS TREE THE WHOLE TIME, FOUND 2026-09-17.**
  `entitysdk/tree_follow_deep_test.go` asks whether a follower materializes a 50-leaf subtree and
  read the mirror with `bob.List("/{aliceID}/deep/sub-N/")` — so it dispatched to **alice**, counted
  **alice's** tree, and reported `bob materialized 50/50` within **0.2 s of alice's own commit**, on
  every run since the day it was written. It also logged *"G3 RESOLVED"*, so a vacuous measurement
  became a recorded conclusion another session could cite.
  **Measured, and it is what makes this more than a style note: it passes identically with the
  cross-peer credential scoped to an operation that does not exist.** Pointed at bob's own index it
  reports **0/50** and a `403`, which is the blocker three other tests were already showing — so the
  vacuous test was hiding the third instance of a live defect, and the one it hid was the one whose
  subject is *"does cross-peer continuation work at all"*.
  ⭐ **The tell is cheap and is the part to carry: an assertion about B's state that NAMES A's
  namespace is a remote read unless you went out of your way to make it local.** A rule written
  down is not a rule applied — this entry has existed since August, in as many words, and the
  instance survived every reading of it. When you next open this bullet, **grep the suite you are
  in** (`\.List\("/%s/`, `\.Get\("/%s/`) rather than only nodding at the rule; that grep is what
  finds these, and it takes a minute. **It was run across the tree on 2026-09-17 and the corpus is
  otherwise clean** — two other hits, both correct: one reads a peer's *own* namespace (dispatch to
  self is a local read), and `publish/feed_live_test.go:359` is a *deliberate* remote read whose
  comment says so, because the arm's whole point is that the signature is reachable live and not in
  the committed set. **A rule with two legitimate-looking exceptions is why the grep has to be read,
  not just run.**
  ⭐ **RE-SCOPED 2026-09-17, and the old wording was a fact about these three helpers stated
  as a fact about the protocol.** It read *"a dispatched read of a peer-qualified path is a
  REMOTE read"*, full stop, and we then generalised from it — in a routed packet — to *"a
  peer-qualified path is ambiguous in both directions and the corpus has no way to say
  which."* **That is false, and arch argued it down with `ENTITY-CORE-PROTOCOL` §1.4, whose
  cross-peer worked example is exactly the operation we said no road expressed.** The
  handler URI names **who you ask**; the resource target names **what you ask about**; they
  are two separate fields of one EXECUTE. An inbound EXECUTE is never re-routed (§6.5
  canonicalizes before handler resolution), so a request arriving at B is answered out of
  B's own view definitionally, and *"B goes and asks A"* is a **locally-originated outbound
  sub-dispatch**, which §1.4 distinguishes by name. **Two operations, not one ambiguous
  path.** So: AP11 is true of our own sub-dispatch and of these three helpers, and false as
  a general rule. Use `AppPeer.GetObtainedEntity` / `PutObtainedEntity` to say the other
  thing. **Had we carried the framing we routed, this tier would have grown a road
  qualifier, a second signature locator and a second trust argument to route around an
  addressing model that was working** — which is the cost of an over-general catalogue
  entry, and the reason the scope of one is worth as much care as its claim.
- **AN ALREADY-ABSOLUTE PATH PASSES THROUGH UNCHANGED, AND RE-QUALIFYING ONE IS THE COHORT'S
  MOST-RECURRING CROSS-IMPL BUG** (`ENTITY-CORE-PROTOCOL` §1.4, which says so in as many
  words and names `/{local}//{other}/…` as the signature; there is a
  `universal_address_space` conformance category for it). It bit here twice in one
  afternoon, in **two functions and one class**: `fetch.Consumer.SignatureEntityOver`
  derived §2.2's invariant pointer under `c.src.PeerID()` — *the peer being read from* —
  and both `Source.Leaf` implementations prepended the serving peer to whatever they were
  handed. Fixed by naming the **signer** (a required parameter, never defaulting to the
  source: the default is right on a direct read and wrong on the only case where the
  distinction exists) and passing the absolute path through in both Sources.
  ⚠ **Neither failure looked like a path bug.** On dispatch, `AppPeer.Get` routed the
  foreign path to **the author**, a peer the reader of a mirror has never spoken to, so the
  answer was *"unreachable"* — the mirror reading as incomplete while holding exactly the
  right bytes. On HTTP, `types.BuildTreeLeafURL` does a `TrimLeft(treePath, "/")`, so the
  path lost its leading slash and was appended to a base already ending in the serving peer:
  a syntactically perfect URL, under two peer-ids, for an object nobody publishes. **A 404
  from that reads as a withholding origin** — an accusation against the publisher, caused by
  the reader. Gates: `fetch/absolute_leaf_path_test.go` (with the peer-relative control arm,
  because the fix must not change what every existing path means) and
  `publish/feed_gather_signer_test.go`.
- **Two directions of transport profile — keep them straight.** `AppPeer.Connect` registers a
  profile for the peer we **dialed** (the address-book direction, core-go's `RegisterRemote*`).
  `AppPeer.AdvertiseTransport` publishes one for **us**, under our own peer-id — §6.5.1a D1
  self-publication, wired into `shellboot`'s listener bind. Path segment is the peer
  **identity-hash hex**, not the Base58 id (`core/types/crypto.go` pins hex for non-root path
  positions); the Base58 form is the profile's `peer_id` field. **Never advertise a wildcard or
  port-0 address** — a durable profile nobody can dial is worse than none, and the SDK refuses it.
- **Liveness comes from the tree, not the pool.** `AppPeer.ConnectedPeers()` is a connection-pool
  snapshot; the lifecycle signal is `system/peer/status`, read via `Store.PeerLivenessOf` /
  `PeerLivenessAll` / `OnPeerLivenessChange` and rendered through `workbench.PeerLivenessModel`.
  The pool cannot express `suspect`, cannot say *why*, and diverges whenever a connection is
  evicted without a demotion. **The status entity is transition-written, not a heartbeat** —
  `LastSeen` is a snapshot taken at the transition, so ageing rows off it invents a contract the
  protocol does not offer (§5.4.1). Absence means "no transition ever recorded", never
  "disconnected". `system/network`'s §2.7 `maintained_peers` is **not** the maintained set (it
  enumerates every status entity); `session_id` is the discriminator, and
  `NetworkClient.MaintainedPeers()` is the filter.
- **Subscribe by prefix; never scan-and-filter.** Consumer refresh uses `Store.Watch`/
  `OnSelectionChange` (or `OnPrefixChange`) — listing the whole tree and filtering in-proc
  is a structural anti-pattern (O(store size) per render). Don't build a workspace-level
  polling dispatcher over the SDK; the SDK is the dispatcher.
- **App handler integration:** the app registers a handler at `workspace/app`; targeted
  refresh flows subscription engine → inbox → app handler → Go channel → UI (not
  "refresh-all on every tree event").
- **Spec discipline:** workbench-application logic (workbench-owned handlers /
  `app/workbench/`, `archives/` namespaces) extends freely; **spec-adjacent** behavior
  (anything `system/*` or a documented domain handler) needs a spec read + cross-impl
  coordination first. To check whether an op is spec'd, read the `EXTENSION-*.md` section
  outline + manifest YAML (`pull: {input_type: ...}`) — grep with impl-style patterns misses
  unquoted YAML declarations.
- **A `RULED` proposal is buildable. Build it.**
  `AGENTS-STANDARD`'s *"implement against the landed spec, not in-flight proposals"* targets
  **unruled** proposals — a shape nobody has decided yet. A proposal stamped `RULED` in its
  header **is** a decision, and implementing ahead of the editorial fold is normal practice
  here: it is one of the ways a spec gets validated before it hardens, and what the build
  surfaces goes back to the authoring repo as feedback.
  **Do not treat "the fold has not landed" as a blocker** — that reading cost us a
  self-inflicted stop on `PROPOSAL-DISCOVERY-RENDEZVOUS-BACKEND`, whose whole normative
  content (one enum value + a composition subsection) was already determined.
  What the implementer owes instead: **name the source in the commit** — "built against
  `PROPOSAL-X` at arch `<sha>`, not against landed `EXTENSION-Y`" — so the coupling is
  greppable when the fold lands and the diff is re-derivable if a token's spelling moves.
  *(Not the same as AP20: that one is about inventing a constant whose referent exists in no
  document. A ruling is a referent.)*
- **A RULE DESCRIBED AS "GATED" IS GATED OVER SOMEBODY'S CORPUS, AND OURS WAS NOT IT** (AP110,
  `A-44`, fixed 2026-09-16 — volunteered against ourselves). `STYLE-NAMING-CONVENTIONS` §3 makes an
  entity-type path segment **kebab** and §7 marks it *gated (high precision): snake = violation*.
  The gate reads `specs/` and `guides/`. **Entity type names are string literals in source**, so
  the rule was unenforced everywhere it is actually written, in every implementation at once — 15
  in `entity-browser-rust`'s `app/state/*`, **1 in ours**, and neither seat could see its own.
  Nobody re-asks whether the thing being scanned is the thing they are writing.
  Ours was `app/state/peer_roster_entry` (`shellboot/peer_roster.go`), wrong twice: snake, and in
  the namespace `GUIDE-ENTITY-WORKBENCH-APP` §4.1.1 makes **cross-impl portable canonical**
  (*"consumers MUST follow the canonical schema"*) — minted there to match ONE other
  implementation, which runs §4.1.1's ladder backwards (app-owned first; promote *when a type
  proves convergeable*). **The file's own comment said both things outright**, and had been read
  for months. It is `app/entity-workbench/peer-roster-entry` now, beside
  `entity-browser-rust`'s `app/entity-browser/peer-roster-entry`, which is where the ladder can
  actually run forward from.
  **No migration code and that is checked, not assumed**: `ListRosterEntries` lists a path prefix
  and decodes, so it never consults the type, and `RestoreFromRoster` → `Create` →
  `WriteRosterEntry` rewrites every entry on the first launch after the change. The app-id in the
  type is a **literal**, not `PeerManager.appID` — §4.1.1's disambiguation keeps the type-name
  namespace separate from the instance-path one, the schema is shellboot's single contract, and
  deriving it from the path would give two frontends two names for one shape.
  Gate: `shellboot/naming_convention_test.go`, in `test-each` rather than in a lint nobody runs.
  Two things it had to get right. **A source-walking gate has two ways to pass while measuring
  nothing** — wrong tree, or a detector that flags nothing — so it asserts a floor on literals
  scanned (474 at writing) *and* runs its detector over a synthetic violation. And **the one
  character that makes the naming split legible on sight is what makes the scan precise: a slash
  means the namespace axis**, so a `cbor:"peer_id"` tag cannot match and will never be "waived".
  It excludes its own source, named, because its fixture must contain a violation.
- **A persistent store does not make the query index persistent** (AP39). core-go ships only
  in-memory query indexes, fed solely by the `"query"` sync hook — i.e. by *this process's*
  writes. `entitysdk`'s `assembleAppPeer` now calls `query.IndexMaintainer.Rebuild` at startup,
  which is the kernel's own answer (*"use for recovery or startup with persisted stores"*) and
  which we simply never called. Before it, `-storage sqlite` gave you a tree that survived a
  restart and an index that did not, so **`find` / `grep` / `compute aggregate` were blind to
  everything written before the process started** while `ls` listed it happily. If you add
  another derived index, the question to answer at the same change is *what rebuilds it on
  open* — and the gate has to cross a process boundary
  (`entitysdk/query_index_restart_test.go`), because in-process it is green either way.
  **This is a correctness fix, not the shape fix** — the rebuild is O(store), measured at
  ~8.7 µs/entity (`query_index_rebuild_bench_test.go`), so it is a startup tax that grows with
  the store forever. The shape fix is backlog row **PR-1**: a SQLite-backed query index in the
  store's own database, which `SqliteStore.DB()` exists to allow ("*so co-located extensions can
  create their own tables in the same database*"). Do not let "it's fixed" close PR-1.
- **A persistence test that reads back through one path certifies that path, not persistence**
  (AP39, and the reason it shipped). `TestStorage_Sqlite_LargeCorpusSurvivesRestart` is a strong
  restart test — 500 entities, hash byte-equality, revision log, `List` cardinality — and it
  reads the reopened peer **only** through `Get`/`List`, which the persistent location index
  serves. The volatile query path had no restart coverage at all. When a store serves more than
  one read path, enumerate them and restart-cover **each**: in-process they answer identically,
  and they diverge only on reopen.
- **`put` stores JSON numbers as floats, so any reader of a numeric field needs the float case**
  (AP40). `json.Unmarshal` into an `interface{}` makes every JSON number a `float64` and CBOR
  core-deterministic encoding keeps it one. `compute aggregate` accepted only integer kinds and
  therefore skipped **every** entity created the documented way, reporting "3 entities scanned,
  3 skipped" while its own unit tests — which build Go ints directly — stayed green. Refuse a
  non-integral value rather than truncating it (AP33). When a fixture has to prove a cross-verb
  type contract, encode it through the *writing* verb's path, not by hand.
