# Feed & social conventions

EMBED, FEED and the data-exchange mirror: what is built, what agrees cross-implementation, and which reader obligations have no gate here.

> Memory, per `AGENTS.md`. Every entry below was paid for by a defect that shipped or
> a measurement that contradicted what we believed. Find by symptom; the mechanism is
> stated before the fix, and `file:line` references point at the code, not at prose.

---

- **THE SOCIAL VOCABULARY IS BUILT — `entitysdk/embed.go` + `feed.go` — AND IT AGREES WITH THE
  OTHER SEAT BYTE-FOR-BYTE** (W5, 2026-09-13). `APP-CONVENTION-EMBED` §3 (the node, `embed-data`,
  the three-arm tagged payload union, the `.size (1..16384)` bound, the passive-only **render**
  refusal) and `APP-CONVENTION-FEED` (`entry`, `index-head`, `index-page`, `follow`, the index
  builder, `FEED-R2`'s detached signature). **Build order is forced by the documents, not chosen:**
  reference → embed → feed, because a feed entry's `body` *is* an embed node and an embed's `child`
  arm carries a reference atom.
  `entitysdk/feed_crossimpl_test.go` produces against `entity-browser-rust`'s `J-4` fixture —
  **five of five on the first run, no correction to either side**: peer id from the pinned seed, 5
  entry hashes, 5 detached signatures *and* their invariant-pointer keys, 4 index bindings key by
  key, and the trie root over the §4.2-pinned keys. Two code bases, two languages, one authored
  input, so per [ADR-0012] this is **not cohort-consistent**.
  Four rules, each of which costs a session if missed. **`signer` is the identity ENTITY's content
  hash, never the peer-id string** — FEED's shorthand reads *"signer = author"* and
  `system/signature` is the kernel's type; `MintEntrySignature` is the one place. **Pages fill
  oldest-first and read newest-first WITHIN a page, and reversing BOTH round-trips perfectly**, so
  a harness that re-reads its own index cannot see it — implemented as a reversal and never a sort,
  because sorting consults `created_at`, which §2.3.2 forbids relying on. **A page's `updated_at`
  is their reading of a field with no stated semantics** (arch `A-41`) and we match it
  *deliberately and say so* — one seat becomes the baseline either way. **The fixture pins a SEED,
  not a peer id**, because `FEED-R1` puts the author in the bytes; the SHARE fixture is the exact
  inverse (no peer id anywhere — §4 makes the namespace the publisher). Two conventions, opposite
  answers on one axis.
  ⛔ **THE READ SIDE ENFORCES §3's MANDATORY `fallback` AS OF 2026-09-15, AND IT DID NOT BEFORE**
  (AP104, `C-6`). `EmbedData.Validate` refused a missing fallback and **`EmbedNodeFromEntity` never
  called it**, so the rule held against embeds this tree authored and against nobody else's — which
  is backwards, since our own emitter is the one producer fixable by other means, and every gate was
  green because every fixture was ours. Second half, unfiled by anyone: the feed reader's non-inline
  branch renders `fallback` and nothing else, so a missing one produced **a blank row carrying no
  problem**, reading as an author who posted nothing; the entry is now KEPT with the fault stated on
  it, for `FEED-R1`'s reason. **MISSING and EMPTY are two refusals**, adopted from
  `entity-browser-rust` — different producers, different next actions — and the split **cannot be
  recovered after decoding** (one Go zero value, two CBOR encodings), so `EmbedFallbackPresence`
  reads the raw bytes and the gate encodes its inputs by hand. Use `ValidateDecodedNested` for a node
  carried as a field of something else: passing the enclosure to the flat probe looks for `fallback`
  at the top level, never finds it, and reports MISSING for every entry including conformant ones.
  **§4 `EmbedOutput` is deliberately NOT built** and the file header says why: an entry stores what
  was *authored* and the handler runs at the *reader*, so storing the output surface fixes the
  rendition choice for every reader forever — and an output vocabulary with no renderer is D23's
  violation carrying a closed enum we would then have to keep.
  ⚠ **We have a producer and NO READER, so four reader-side `[MUST]`s have no gate here at all**
  (§1.1's namespace check on receipt — `ValidateInNamespace` has no caller outside its own test;
  §2.2.2's four resolution outcomes; §4.3 rule 6's fall-back-to-enumeration; §4.4's
  resume-from-page). The evidence is one-directional and a green fixture does not change that.
- **A PEER PUBLISHES A FEED NOW — `post` / `feed`, `entitysdk/feed_author.go` — AND THE THING THAT
  ATTRIBUTES AN ENTRY IS OUTSIDE EVERY PREFIX A PUBLISHER CAN COMMIT TO** (2026-09-14; arch's
  `AZ-s4` and our own surface question, which are one item from two sides). W5 shipped the
  vocabulary with no verb and no pixel; this is the production path. **A post APPENDS** — entry →
  its `FEED-R2` signature → the current index page → the head, in that order, because a head naming
  a page whose entries are not bound yet is a feed that overstates itself and a reader cannot tell
  that from a withholding origin. One post touches two index keys and **a full page is never read
  or re-encoded again**, which is §4.3 rules 1 and 3 holding by construction. `BuildFeedIndex`
  stays the reference and `TestFeedAuthor_AppendEqualsFullRebuild` asserts the appended index is
  **byte-identical** to a full build across two page boundaries — the fixture measures a builder
  and the product uses an appender, and nothing else in either tree compares them.
  ⛔ **The finding: `FEED-R2`'s detached signature lives at `system/signature/{hex(entry_hash)}`
  (V7 §3.5) and a feed publish commits to `app/feed/`.** Measured, two arms
  (`publish/feed_live_test.go`): absent from the committed key set; reachable by a **live** reader
  only because the grant names `system/signature/*` separately. **A static reader has no second
  channel**, so every entry arrives unattributable and `FEED-R4` cannot distinguish that from an
  author who never signed. The other seat mints the same signature at the same key under the same
  prefix, so it is a property of the convention — routed as our `A-36`, and **`feed` prints it as a
  standing caveat** because the operator who publishes is the only party who can act on it.
  ⚠ **This entry used to add *"no prefix contains both except the whole tree"*. That is WRONG and
  arch corrected it** (`ROUTING-2026-09-15-a` §1.3, read in `entity-browser-rust`'s
  `signed_root.rs`): the containing prefix is **this peer's own namespace**, which is one peer's
  subtree and not the universal tree. We made a negative claim about the corpus on recall — the
  rule this file already states about our own tree, one level out. `PublicSiteGrants`'s
  universal-tree refusal is confirmed correct and stays.
  ✅ **`A-36` AND `A-38` ARE BOTH RULED AND SHIPPED (2026-09-16) — `publish -feed`.** Publishing
  over the peer root does put every signature in the committed key set
  (`publish/a36_peer_root_probe_test.go`, with `FEED-14`'s negative arm as its anti-vacuity half).
  It also moved the committed set from **4 keys to 386** and the static emit from **7 entities to
  400 across 379 paths**, putting an operator's folder path, another peer's LAN address and the body
  of a document from a folder nobody shared into the upload directory — so we filed `A-38` offering
  three answers and did not ship.
  ⭐ **Arch ruled NONE of the three, and the correction is the part to carry: all three rested on
  the premise that widening the `prefix` widens what is PUBLISHED, and `EXTENSION-TREE` §3.3a denies
  it three separate times.** The prefix *"bounds the publication's scope; it is NOT a completeness
  claim"*; a publisher *"MAY declare `/{peer_id}/` and publish a small subset"*; and the
  negative-scoping `[MUST]` lets one *"serve different subsets to different audiences from the same
  prefix"*. A completeness `[MUST]` that would have made our premise true landed in v4.1 and was
  **withdrawn in full.** ⇒ **Ruling (D): move the PREFIX, do not move the CONTENT.** The 4→386 was
  never a cost of the ruling; it was the cost of **deriving the content set from the prefix**.
  Measured after: **9 committed keys against the 399 the peer root bounds** on the gate's fixture,
  and **71 vs 438** on the 34-entry corridor cut — entries + their signatures + the index head and
  pages, exactly.
  ⭐ **Why it survived review, and this is the transferable half:** nothing in `publish/` ever chose
  a binding set, because **the easy helper implements the reading the ruling rejects.**
  `tree.BuildTrieForPrefix` takes a prefix and scans; `tree.BuildTrie(cs, bindings)` takes an
  explicit list and is exported, and core-go's own publish path and `root_tracker` both reach for the
  scanning form. Arch said so in our favour and routed it to `entity-core-go` themselves — *the
  reference API makes the prefix the subset selector*. **When a defect is "we used the obvious
  helper", the finding is about the helper.**
  ⚠ **The structural statement we filed — `system/signature/` is kernel-placed, so a root committing
  to ONE contiguous prefix can never commit to an artifact *and* its evidence without containing
  everything between them — is TRUE about the PREFIX and was wrong about the PUBLISH.** Keeping it
  visible because the error is the reusable one: *a bound and a content set are different things,
  and we collapsed them because one function took only the bound.*
  **Shape:** `publish.ContentSet` + `publish.FeedContent()` (`publish/content_set.go`); nil is the
  scan and stays the default, because a site's prefix genuinely IS its content. `mintSignedRoot`
  takes the binding set and never scans. **The disclosure guard moved with it** —
  `disclosureAcrossSystem` is about SCANNING, not about the prefix's spelling, so `-feed` needs no
  `-whole-peer`: the acknowledgement exists for keys a scan sweeps along and a curated set sweeps
  none.
  ⚠ **A ROOT DOES NOT RECORD HOW ITS SET WAS CHOSEN, so the publisher must**
  (`workbench.PublishRecordPath`). A curated root and a scanned root over one prefix are two hashes
  and nothing distinguishes them — correctly, since a consumer is forbidden to read the prefix as a
  completeness claim. But `publish.RootNow` answers *"does what I published still describe what I
  have"* by RE-DERIVING, and re-deriving with the wrong set reports **"your root is behind" on a
  root that is exactly current, permanently**, which is the identical symptom the trimmed trailing
  slash produced. **Absent means SCAN and that is a fact, not a default** — the curated mode did not
  exist before 2026-09-16, so there was only one possible answer (`FolderData.Mode`'s shape).
  Gates, and they are deliberately two because neither sees the other's failure:
  `publish/a38_curated_content_set_test.go` is the **mechanism** — including the no-regression arm
  asserted against `tree.BuildTrieForPrefix` itself, the function the publisher no longer calls, so
  it cannot pass by agreeing with itself (AP96) — and `shellboot/feed_publish_curated_test.go` is the
  **wiring**, because the mechanism gate builds its own content set and hands it in, which is AP108
  exactly. Both mutation-checked: unshare the name→selector mapping and the wiring gate fires on
  *"reported as behind immediately after `publish -feed`"*; make the selector re-scan and the
  mechanism gate names the leaked declarations one by one.
  ~~⚠ **Owed: no GUI control.**~~ — **CLOSED 2026-09-16.** `PublishNow(handle, public, feed)` +
  `FeedOwnRender`, and the *Feed* panel's **your feed** section with a **Publish feed** button.
  It was D23 at field granularity in the purest form measured here: the act that makes a feed
  attributable reached a shell verb and nothing else, and `SignatureNote` — `FEED-R2`'s caveat,
  the whole point — had never crossed the bridge in **any** form.
  Four rules, each already earned elsewhere in this file and each paid for again.
  **`feed` is a SECOND PARAMETER and not a mode enum**: *what does this root commit to* and *who
  may read it* are independent questions, and folding them together makes "publish my feed" also
  restate a disclosure decision — which is exactly what the `public` tri-state exists to avoid one
  field over. **The publish is an ACT and the section is a READ**, two exports, because a surface
  that refreshed by minting bumps `seq` every time somebody looks at it. **`contentSet` crosses
  the bridge too and the Local Site panel renders it**, because `PublishOutcome.ContentSet` already
  said a surface MUST — after `A-38` a peer-root root is 9 keys or 399 and the prefix does not
  record which.
  ⭐ **Three states, not two — and empty is a real answer on BOTH sides of the interesting line.**
  `signatureNote` empty means *attributable* when a root is published and covers the feed, and it
  is ALSO what an unpublished peer produces, because there is no root to be wrong about. A renderer
  short-circuiting on the empty string therefore tells an operator who has published nothing that
  every entry is attributable — the confident direction of wrong.
  `An_Unpublished_Feed_Does_Not_Render_As_Attributable` drives all three arms through
  `ApplyOwnFeedForTests` (the fixture peer is shared, so two of the three are unreachable from the
  bridge — AP70), and `The_Own_Feed_Envelope_Does_Not_Drop_The_Attribution_Fields` asserts the
  names against what Go actually emits. Both mutation-checked and each fired on exactly its own
  mutation: renaming `signatureNote` on the Go side, and collapsing the renderer to two states.
  ⛔ **The wake is the SHARING wake, so a post made from `entity-shell` while the panel is open
  does not refresh it.** Entries live under `app/feed/`, which nothing here subscribes to — and by
  this file's own rule (AP73) the honest fix is a subscription, not a refresh button, so the gap is
  named in the panel and left open rather than papered over with a control that would read as one.
  ⚠ **And `A-38`'s lesson arrived in `FeedPrivacyProblem` one function later than in the
  publisher.** §2.4's *publishing a follow list is a separate, voluntary act* warning read the
  published PREFIX — so a `publish -feed`, which declares the peer root and commits to nine keys,
  **accused the operator of publishing their follow list in the same sentence that told them to fix
  it by doing what they had just done.** It takes the content set now. *A warning that fires on the
  one action that cannot cause the harm is worse than no warning*: it is the line an operator
  learns to skip, on the surface where the real disclosure would appear. Gate:
  `TestFeedPrivacyProblem_TheCuratedFeedSetIsSilentAtEveryPrefixTheScanWarnsAt`, whose scan arm is
  the control — without it the test passes against a check that has been deleted rather than
  narrowed.
  ⚠ **Also corrected: `feedProblems` still named `publish -prefix app/feed/`**, which the ruling
  made the *weaker* instruction — same entries, no signatures, so every entry stays unattributable
  to a static reader while the next line the operator reads is the caveat saying so. AP80's shape:
  guidance that survives the ruling that superseded it, still true-sounding, quietly costing the
  reader the thing they came for.
  ⚠ **Still owed: no way to POST from the GUI.** A composer is in flight between arch and
  `entity-browser-rust`, so building one here unilaterally would be inventing the surface they are
  ruling on; the panel publishes what the shell authored.
  Three rules the build earned. **POSTING IS NOT PUBLISHING and nothing in the substrate says so**
  — a published root commits to a trie root taken at mint time, so a post is invisible to every
  reader on both roads and looks exactly like not having posted; `publish.RootNow` is the check and
  **the binding count is not**, because one post rewrites the index head in place and changes no
  count at all.
  ⭐ **"On both roads" was written from the STATIC road and is now measured on the live one**
  (2026-09-15, `publish/feed_post_reach_live_test.go`): two real peers, 3 entries visible after the
  mint, **3 after a fourth is authored with no re-mint**, 4 after re-minting — the third row being
  the control arm without which the first two are satisfied by a harness that can never see a fourth
  entry. The reader does not error, does not warn and does **not** take the enumeration fallback: it
  answers **via the index** with the old set, which is byte-identical to an author who never posted.
  ⚠ **This entry said `APP-CONVENTION-FEED` §7.6 "is why it cannot be otherwise" and that is an
  OVERCLAIM — corrected 2026-09-15 on re-reading the convention rather than our quote of it.**
  §7.6 is titled *the light-client property* and §7.2 says *"verification is root-anchored"*, both
  unqualified prose, and **no `FEED-Rn` row requires a reader to anchor a read on a root** — checked,
  all 34. Worse for the old framing, §1.1 rules the opposite on the axis it names: an entry *"should
  verify alone … **without a root that may be many publishes stale**"*, and the root answers a
  different question the section calls **anti-omission** — *was this in their published tree, as of
  sequence N?* So **attribution needs no root and the other seat is right about that half**; what is
  root-anchored is **discovery**, and only as-implemented.
  ⭐ **The sharp finding, and it is better than the one we routed: BOTH of a reader's two paths are
  root-scoped, so `FEED-R13`'s fallback — the rule that exists precisely so the index is never the
  authority — inherits the same anchor and cannot rescue an un-minted post.** Ours enumerates *"every
  key the signed root commits to"* (`workbench/feed_read.go:73-76`, `:470`), and the entry is outside
  that set by construction; `entity-browser-rust` measured **34 via index / 0 via enumeration** on the
  corridor cut, and their `FeedSource::list` defaults to *cannot enumerate*. **On the live road there
  IS a second channel a static reader does not have** — a `tree:list` at the author's own peer — and
  nothing in the convention says whether rule 6 may use it. So *"the live road needs nothing further"*
  is **not refuted, it is undecided**: it is achievable by a reader whose rule-6 fallback is not
  root-scoped, which neither seat has built. That is `C-7`'s corrected ask. Measured because another
  seat was about to have a composer built on it (`ROUTING-2026-09-15-d-…`); the distinction that holds
  either way is that authoring
  is local, the **mint** is the act a reader can observe and is *also* local, and only the origin
  emit needs a network — collapse the first two and the one step whose absence is silent is the one
  you dropped. **A trie's keys are relative to its prefix**, so `app/feed` and `app/feed/` give
  different roots over identical bytes — trimming the slash made `feed` report the root as stale
  forever, which is a permanent line in a problems list, which is how an operator learns to skip
  the list. And ⚠ **a walk's keys are relative to the PUBLISHED PREFIX while §4.2's pinned address
  is not**: a root over `app/feed/` commits to `index`, not `app/feed/index` (§3.3a:
  *`prefix + relative_key`*). Our own live gate asserted the wrong one and failed against a correct
  feed; a reader that gets this wrong sees **an empty feed with a valid signature over it**, which
  is the most confident wrong answer available.
  `ValidateInNamespace` has callers now — the emitting side and the live gate's reading side — so
  one of §40's four ungated reader `[MUST]`s is gated and **the other three are not**. `feed` is
  **not a reader**: it reads this peer's own tree with this peer's own authority and says so.
- **THE MIRROR VOCABULARY IS BUILT — `entitysdk/feed_mirror.go` — AND IT IS NOT A FEED FEATURE**
  (2026-09-16). `app/feed/mirror` + `app/feed/mirror-page`, the subject's two arms, and §6.0.1's
  coordinate. **`APP-CONVENTION-FEED` §6.0a's first line hands authority to `SYSTEM-DATA-EXCHANGE`
  §2.5 and says the exchange wins on any disagreement** — a gatherer is any peer that republishes
  what it obtained, and a wiki on this substrate would inherit gathering from the identical rule
  with a different noun. What is feed-specific is the type tag.
  ⭐ **The coordinate agrees with `entity-browser-rust` byte for byte, against a literal neither of
  us computed with the function under test.** Their `the_live_coordinate_is_pinned_to_a_literal`
  derives it with `hashlib` from the ECF framing rules; ours comes out of the kernel's
  `revision.PrefixHash`. Two implementations, two languages, two kernels, one independently-derived
  value ⇒ per [ADR-0012] this is **not** cohort-consistent. Vector:
  `2AliceExamplePeerIdForKeyVectors` → `004bba2675…8856d`.
  **Call the kernel, never restate the three lines.** §6.0.1 pins the live coordinate to
  `EXTENSION-REVISION` §3.1's `prefix_hash` and `[v0.3]` adds a MUST NOT against implementing it as
  a new derivation — it is **derive-to-meet**, so a drift fails *nothing*: the two peers write
  mirrors at different keys and never meet. ⚠ It is pinned to the ECFv1-SHA-256 floor whatever the
  peer's home format, and **core-go's own `PrefixHash` comment records that this differs from one
  available reading of §3.1** and that nothing has failed only because it pins SHA-256 today. If
  that function ever follows the home format, this coordinate MUST NOT follow it.
  Four refusals, each structural rather than a validation: **no completeness field and there will
  not be one** (`DX-R14` — the verifiable property is *omit but never substitute*); **`entries` is
  always PINNED**, because a live reference mirrors whatever is at that address now, which is not a
  mirror; **the derivation reads identifying fields only** (`FEED-R27`) — the Go stand-in for their
  destructuring match is an **unkeyed composite literal of `EntityRef`** in the gate, which stops
  compiling the moment the atom grows a field; and **`page` MUST equal its key**.
  ⛔ **What is NOT built: the gathering loop, and the reason is `DX-R8`.** A gatherer's write is
  §2.1's byte-preservation MUST, `AppPeer.PutEntity` is this repo's operation for it, and **it has
  never been used against foreign bytes anywhere in this tree** — all twenty callers are in
  `programs/`, writing their own state. §6.0a's real rules are properties of a SEQUENCE of writes
  (gather order; a page sealed when its successor opens; a sealed page never rewritten) and
  `ToEntity` sees one, so they belong to the loop and their gate is `DX-C8`. Also still missing:
  `app/feed/collection`, the seventh type. **Measured 2026-09-16: FEED declares 7 types,
  `entity-browser-rust` implements 7, we implement 6.**
- **THE GATHERING LOOP IS BUILT — `workbench/feed_gather.go` — AND `DX-C1`'s THIRD HOP CARRIES
  INTEGRITY WITHOUT AUTHORSHIP** (2026-09-16). `GatherTimeline` → `PlanMirror` → `WriteMirror`,
  plus `LoadMirror` for the prior view. It is the first place in this tree where `PutEntity` meets
  bytes we did not author — the audit had measured **twenty callers, all in `programs/`, all
  writing their own state**, so §2.1's required operation existed and was unproven against foreign
  input. Read through `BrowseModel`'s road (`feedConsumerFor`, shared with `ReadFeedOf`) because
  **a gatherer that read the projection some other way would republish bytes nobody verified**, and
  the artifact is durable, signed by us and indistinguishable from a good one (AP108, aimed at a
  write).
  Three outcomes per entry and **only one is a drop**: carried with its signature; **carried
  WITHOUT one** (`DX-C5` — §2.3 rule 3 binds the *reader* to say unattributed and does not licence
  a gatherer to drop, because **a mirror's only lie is omission**); refused, for an entry the reader
  already rejected under `FEED-R1`, since republishing it propagates a forgery we had just finished
  detecting. Foreign bytes land **under the AUTHOR's namespace** (`Carried{peer, key, entity}`,
  the shape `entity-browser-rust` reached independently), never ours — `DX-R4`/`DX-C6` hold by
  construction because every key is derived from an entry hash.
  ⛔ **`AppPeer.PutEntity` COULD NOT EXPRESS THE WRITE, AND ITS ERROR NAMES THE WRONG MACHINE**
  (AP112). `resolveDispatchTarget` routes by peer segment, so
  `PutEntity("/{author}/app/feed/entries/{hex}", …)` resolves to `entity://{author}/system/tree`
  and asks **the author** to accept a write; they refuse, correctly, with `403 capability_denied`,
  and the sentence an operator reads is indistinguishable from *"that publisher revoked us"* when
  the author is not involved at all. `AppPeer.PutObtainedEntity` pins the handler LOCAL and lets the
  peer-qualified path travel as the resource; the capability is a parameter because V7 §1.4 layer 1
  grants the authority and §PR-8 stops the owner self-cap from *saying* so
  (`MintMirrorCapability`).
  ⛔ **The finding, measured** (`publish/feed_gather_reach_test.go`, three peers, C a stranger to
  A): view addressable at the derived coordinate ✓, 3 entries all referencing A ✓, B serves all 3
  by hash ✓, **0 of 3 attributable by C** against a control of **3 of 3 read directly from A**.
  ⚠ **Ruled out and measured so nobody reaches for it:** B's root commits to **0 keys under A's
  namespace**, which is expected (a detached signature is read outside the committed set by
  design), so *"carry it in the root"* is not the fix and is `A-38`(D) pushed past where moving a
  prefix can go. Asks `A-45`/`A-46`, packet `ROUTING-2026-09-16-c-…`.
  ✅ **BOTH WALLS ARE DOWN AND LEG 4 CLOSES 3 OF 3 (2026-09-17-b).** `DX-C1`'s third hop carries
  authorship on a live transport: a stranger to the author verifies every mirrored entry out of the
  republisher's tree. It took two fixes in two trees, each invisible from the other side, and the
  history below is kept because **the intermediate state read as one bug and was two.**
  ⭐ **`A-45` RULED, OUR HALF FIXED — AND FOR TWO DAYS THE COUNT STAYED 0 AND IT WAS NOT THE SAME 0**,
  which is the part to carry, because the sweep looks identical either way.
  `SYSTEM-DATA-EXCHANGE` v0.3 §2.2.1 rules it on `ENTITY-CORE-PROTOCOL` §1.4's authority: bind at
  the signer-rooted absolute path, resolve it **against the peer serving the object**, MUST NOT
  re-qualify. Ours asked under the peer it was reading from; it now names the signer and passes
  the absolute path through (see the AP11 re-scope above). Leg 4's detail line moved from
  *"nothing bound at this path"* to **`?resource=/{A}/system/signature/{hex}: 403
  capability_denied`** — the right question, refused.
  ✅ **The second wall was not ours, nobody had measured it, and core-go fixed it in hours
  (`3df98f6`).** Arch's §1.3 offered the repair as one character — a bare `system/signature/*` is
  peer-relative under §PR-8, so name the author, `/{A}/system/signature/*`. **The grammar was right
  and the conclusion did not hold.** Measured, three arms: the granter's OWN namespace (`/{B}/…`)
  accepted; a THIRD peer's refused; `/*/…` refused. And the refusal was worse than a refusal —
  `AssembleInboundGrants` → `filterAdvertisedGrants` keeps an entry only if this peer's advertised
  served-scope covers it and *"an uncovered entry is DROPPED, not narrowed"*, while
  `advertisedServedScope` gave `system/tree` a bare `*`. ⇒ **adding the row did not widen the
  grant, it DELETED the entry the row was added to**, and the reader lost reads that worked before:
  an operator widening a grant made it strictly narrower, silently, with the policy row written and
  accepted. **The fix is the cross-peer `/*/*` on the derived resources axis** (`MaxScope`, when a
  handler declares one, untouched). Keep the shape in mind rather than the incident: **ask of any
  filter that admits by coverage whether it NARROWS or DROPS**, because the two are
  indistinguishable at the call site and only one of them is safe to widen into.
  ⭐ **core-go's own `defaultHandlerSelfGrant` documented this exact class forty lines from
  `advertisedServedScope`** — bare `*` is own-namespace-only, the cross-peer form is `/*/*` — and
  recorded fixing it there *because* a peer *"could no longer write the foreign-namespace subtrees
  its store legitimately holds under V7 §1.4's universal address space."* The advertised scope was
  the same ceiling facing **outward**, still carrying the old spelling: **the third surface of one
  class, and the first two were fixed by people who could not see the third.** It was **not shimmed
  here** — a local workaround would have hidden a cohort-wide question — and that call is why the
  fix landed in the tree that owns it.
  ⭐ **What made it turn around in hours was routing a FALSIFIER, not a bug report.** Four arms with
  the control included, so the other seat could reproduce the finding *and its negative case*
  without re-deriving either. A report says *this is broken*; a falsifier hands over the experiment.
  ⭐ **So the two walls were gated SEPARATELY, and that is the transferable move.** A gate that only
  runs the full hop reports our fixed half as broken for as long as somebody else's blocker is open,
  and the next session re-fixes what is already fixed.
  `publish/feed_gather_signer_test.go` removes the transport by construction — a Source reporting
  the republisher while serving the author's bytes — and asserts the address, the key and a
  wrong-signer control arm; it fails on the pre-fix consumer. Leg 4 is an **assertion** now, ordered
  **after** its control arm so a broken harness can never present as a regression in the mirror.
  ⭐ **AND THE TRIPWIRE WAS REPLACED RATHER THAN DELETED OR KEPT** — the step most likely to be
  skipped. `…GrantDeletesTheGrantItWasAddedTo` pinned the blocked state and went red the moment the
  blocker lifted, exactly as designed (*a blocked-state gate that keeps passing after the blocker
  lifts is the one failure it must not have*). But **a gate that only ever says "still blocked"
  cannot then protect the fix**, so it is now
  `…TheAuthorNamespacedGrantWidensRatherThanDeleting`: the property, plus the anti-vacuity arm that
  removes exactly that row and requires the 403 back. Without that arm the positive arm passes
  against a harness with no authorization in it at all.
  ⭐ **The generalisation is bidirectional and is the half to carry: a peer-qualified path means
  both *"ask A for x"* and *"my own copy of A's x"*, and republication is the first operation in
  this cohort that needs the second on the READ and the WRITE side.** AP11 has covered the read
  half since August; the exchange tier is what makes the other reading load-bearing.
  ⭐ **AND OUR OWN LIVE GATE WAS VACUOUS, found by mutation, which is `DX-R9` happening inside the
  gate written to honour it.** The pure gates use a fixture carrying a field this build does not
  declare; the live one used the publisher's entries, which our own encoder authored — so
  decode-and-re-encode is lossless over them and **a gatherer mutated to do exactly that passed the
  whole live suite.** §2.1.1 says it in as many words (*a round trip through bytes your own encoder
  produced proves nothing*) and `DX-C2a` is a separate conformance row precisely because this check
  *"has a documented history of passing while measuring nothing"*. It has one more instance now.
  Fixed by `undeclaredFieldEntry`, which asserts its own premise at the point of planting.
  **Every gate here is mutation-checked and each fired on exactly its own mutation** (re-encode;
  self-generated fixture; drop-the-unsigned; restamp-every-page; wall-clock stamp).
  **`gathered_at` is the gathered set's HIGH-WATER MARK, not a clock** — §6 gives the field no
  semantics, `entity-browser-rust` made the same call and routed the question, and we **match them
  deliberately and say so** (`A-41`'s rule: one seat is the baseline either way). A wall clock moves
  the entity, the trie and the signed root on every run, and invalidates every consumer's cached
  copy of a view whose content did not change.
  ⭐ **One place the substrate puts us ahead, and it is not better code:** their `--gather` passes
  `&[]` for prior pages by stated bound, so a second gather re-pages from scratch and `FEED-R32`'s
  **cross-run** sealed-page property is gated natively only. We write into a tree, so `LoadMirror`
  reads the prior view back — second gather carries **0** entities and the head hash does not move.
  **Still owed:** the pointer-body blob closure (an embed over EMBED §3's 16 KiB ceiling
  republishes with its body unreachable — named in `MirrorPlan.Notes` at plan time rather than left
  to be found), a thread gather, and `app/feed/collection`.
- **THE BODY CALL: A FEED ENTRY'S BODY IS AN EMBED NODE AND WE RENDERED EVERY ONE OF THEM AS ITS
  FALLBACK** (`workbench/feed_body.go`, 2026-09-17 — arch's `ROUTING-2026-09-17-a` §4, and the one
  thing they asked either app seat to align on). `APP-CONVENTION-FEED` §2.3 makes `body` an
  **Embed node**, `SITE` §3.1/§3.2 already shares EMBED as the vocabulary ⇒ **a feed entry
  carrying markdown, drawn by the same code that draws a site page, is conformant today with no
  spec change.** Ours read the inline bytes into a text field whatever the media type said, and
  rendered `fallback` for every other arm.
  ⭐ **Why no gate could see it: the old reader was CONFORMANT.** Showing a fallback is exactly
  what §6 step 2 says to do; the defect was doing it when step 1 was available. Nothing errors,
  nothing is malformed, and *every assertion phrased as "is the output valid" passes*. Arch put
  it in one line — **the ladder puts `fallback` LAST, and implementing only the last rung
  produces a conformant reader that displays an image post as its alt text.** So the gates are
  phrased as **which rung**, which is the only question that separates the two.
  **It was an ALIGNMENT defect, not only a local one**: `entity-browser-rust` had taken the first
  pass (`feed_body.rs`), so one seat rendered markdown and the other rendered alt text for the
  same bytes — two products, one convention, **invisible from both sides**, since each is
  internally consistent and neither reads the other's output.
  §6's three steps, and what this tier can actually reach: step 1's full form **exists in no tree**
  (the §5 handler/renderer registry has no implementation anywhere and §4 `EmbedOutput` is
  deliberately not built here), so what is implemented is §7's stated **v1 floor** — *"an
  inline-payload passive embed renders at the floor tier"* — which for a text media type is: draw
  the bytes in the form the type declares. Step 3 has nothing to do without §4, and is **named
  rather than silently skipped**.
  ⚠ **§6 step 2's anti-poisoning rule is a security rule and the natural implementation breaks
  it** (S-8): a fallback is rendered with embed directives **DISABLED** at depth 1 — shown as
  visible text, *not re-expanded and not silently stripped*. So `EmbedsToMarkdownImages` runs on
  the markdown rung and **MUST NOT** run on the fallback, however much sharing the line looks like
  tidying: the author of an unrenderable entry controls that string, and expanding a directive
  there lets them make every reader that **degraded** fetch an asset of their choosing — the path
  for readers that can do less acquiring the wider reach.
  **`Rung` is a field because the string cannot carry it.** `rendered` is the post; `fallback` is
  the author's *description* of a post that is not on screen; they are the same Go type and the
  same JSON string, and a short alt text reads as a short post. Three states, not two —
  `unrenderable` is EMBED §3's mandatory-`fallback` violation and renders the fault, never a blank
  row (`C-6`'s finding, kept).
  Also fixed while here: the `ValidateDecodedNested` check now runs on the **fallback rungs only**
  — a markdown body renders at step 1, where the fallback is never consulted, so validating it
  there puts a producer-defect problem line on a row displaying the author's words perfectly.
  **Authoring exists too, or the tree could render what it cannot produce**: `post -markdown`,
  `entitysdk.FeedMediaMarkdown`/`FeedMediaPlain` spelled **once beside the author** (a reader
  holding its own copy of a producer's dispatch key is the shape of the divergence this closes).
  `DefaultPostMediaType` stays `text/plain` **deliberately** — defaulting a bare `post` to markdown
  silently reinterprets every post already written, and the one thing a default may not do is
  change what existing bytes mean.
  Surfaces in the same change (D23): `timeline` marks the rung and prints the note; the Feed panel
  draws markdown through the **same `MarkdownRenderer` a site page uses** and captions the fallback
  in Goldenrod. Gates: `workbench/feed_body_test.go` (eight arms, mutation-checked — dropping the
  markdown rung and lowering directives in the fallback each fire on exactly their own arm).
  ⭐ **And the envelope gate went where it could actually run.** `FeedPanelTests` had named
  per-ENTRY fields as a hole it could not cover (a populated timeline needs a reachable publisher
  and no suite here may reach the network; AP70 forbids writing to the shared fixture peer) — but
  **the hole was about the LAYER, not the fixture**: *"does Go emit the key C# reads"* needs no
  peer at all. `avalonia/bridge/feed_envelope_test.go` builds the model struct, runs the real
  projection, and asserts the key names as **literals** (a test that derives the expected name
  from the tag it checks agrees with itself for any value of the tag). It covers `via`/`listed`
  too, so that gap is closed rather than extended. **`avalonia/bridge` was a `go.work` module with
  no suite target** — a gate there would have run for whoever typed `go test` in that directory
  and nobody else, which is `publish`/`fetch` before 2026-08-19 (AP21/D22) — so `make test-bridge`
  exists and `TEST_SUITES` is **eleven**. ⚠ Still true and named: a rename on the **C# side alone**
  is invisible to it. The two halves are gated in two languages and neither can see the other's,
  which is AP49's own shape.
- **A LIVE REFERENCE RESOLVES NOW, AND THE ABSENCE IT REPORTS HAS TWO CAUSES THAT MUST NOT BE ONE**
  (`workbench/ref_resolve.go`, verb `ref`, 2026-09-15). `APP-CONVENTION-FEED` §2.2.2 gives a live
  reference four outcomes and `FEED-R7` **[MUST]s that a reader be able to tell which one it got**.
  The normative half is the **ability to tell**, not the policy — strict and lenient are both
  legitimate — so the resolver returns a typed outcome and keeps its error return for faults about
  the **publisher** (unreachable, never published, root unverifiable, committed bytes withheld). **A
  `(entity, error)` pair cannot express this**: it collapses rows 2, 3 and 4 into *"something went
  wrong"*, and **row 2 is not a failure at all** — a document that evolved since somebody linked to
  it is the ordinary case, and the honest answer is the current bytes *plus* the fact they moved.
  `Moved` is carried as its own field beside `Row`, because a caller switching on an enum can forget
  a case and a caller rendering provenance reads one boolean.
  ⭐ **The fifth state, and it is the transferable part: a reference names a `(peer, path)` and a
  root commits to a PREFIX.** So *"the key is not in the committed set"* has two causes — the
  publisher unpublished it (row 3/4), or **this root never covered that region of the tree at all**,
  in which case the publisher has unpublished nothing and the root says nothing in either direction.
  Folding them together answers *"that document is gone"* about a document that is fine and blames
  the machine that is behaving correctly. `RefNotCommitted` keeps them apart and its sentence names
  what the root **does** commit to. Same shape one layer up from `fetch.ErrEmptyEnumeration`, and an
  **empty** signed root takes that outcome too rather than `dangling` — a fact about the root is not
  a finding about the path. The prefix test is **segment-exact**, or `app/feed` swallows
  `app/feedback/…` (§3.3a makes reconstruction pure concatenation, so the inverse is a trim and not
  a path join).
  **Resolution goes through the verified walk and nothing else** — a dispatched `tree:get` is right
  there and proves the wrong thing (an authenticated connection proves WHO, not WHAT), and the verb
  goes through the browser's road chooser and consumer cache so the `seq` floor stays one per
  publisher (AP100). **Row 3's fallback is safe from anywhere and both legs are gated**, because
  `seen` is a hash and the bytes are self-validating: a document this reader had already read comes
  back from its own store, one it never saw comes back from the publisher's content store by hash
  *after* the root stopped committing to the path. That is how a live reference survives its author
  unpublishing it with no link database anywhere. Pins resolve too and **need no published root** —
  §2.2.1 makes `reply.root` and `reply.parent` pins, so a live-only resolver would refuse every
  reference a feed actually carries today.
  Also fixed here: `post -reply` now says a reply **notifies nobody** (`FEED-R9`, a MUST NOT). It
  claimed no notification and so was not yet a violation — but *"reply"* means notification
  everywhere else a person has used the word, and silence was the wrong amount to say.
  ⚠ **Owed: no GUI control.** The outcome reaches a shell verb and no pixel, and a resolver whose
  fact a renderer drops satisfies `FEED-R7` nowhere — D23 at field granularity, AP49's shape one
  boundary out.
- **A PEER READS ANOTHER PEER'S FEED NOW — `follow` / `unfollow` / `follows` / `timeline`,
  `workbench/feed_read.go` — AND THE PLAN SAID TO BUILD IT THE ONE WAY THE CONVENTION RULES OUT**
  (2026-09-15). `LIVE-PEER-DIRECTION` §3 obligation 3 said *"`app/feed/follow` as a subscription,
  not a poll"* and named the kernel subscription engine, quoting the convention's **one-line role**
  for the type (*"a reader's durable subscription to a peer's feed"*). **§2.4 of the same document
  rules the opposite model**: a feed-follow follows a *namespace* — public, pull-only, **no grant
  and no permission**, and *"the publisher does not know the follower exists"* — which is the entire
  discriminator against `app/share/follow`, where following a **grant** means they do know. A kernel
  subscription registers AT the publisher, so it needs authorization and announces a follower.
  ⭐ **The transferable rule: a plan that quotes a one-line role descriptor has quoted the summary,
  not the rule.** The section that defines a type is where its authorization model lives, and a
  summary line cannot contradict it because it was never making that claim. The trap here was a word
  — *subscription* names a protocol extension in this corpus **and** means a standing interest — so
  the mechanism got read into ordinary English. §7.6 is the loop the convention actually specifies:
  verify one signature, read the index head, read down to your cursor, stop. Delivery stays a
  legitimate **optimization for a publisher who granted one** and must never be described as how
  following works. The direction doc is corrected in place with the error kept visible (AP80).
  Four reader `[MUST]`s, each gated by the condition that defines it
  (`publish/feed_reader_live_test.go`): **`FEED-R1`** rejects an entry whose `author` is not the
  namespace it was found under — the row is KEPT and the body is not rendered, because dropping it
  makes the reader's list disagree with the index it came from; **`FEED-R4`** presents an entry with
  no verified detached signature as *unattributed*, with both arms asserted, since a positive-only
  test passes against a reader that attributes everything; **`FEED-R13`** (§4.3 rule 6) removes the
  head and every page and asserts the enumeration returns **the same set** — *slower, same answer* is
  the rule, and a shorter answer is the publisher lying by omission with our help; **`FEED-R14`**
  removes the entry the reader held as its position and requires a resume from the page number.
  **The fallback filters by TYPE, never by key prefix** — where entries live is this implementation's
  choice (§2 makes the tag the contract), so a prefix scan finds another implementation's feed empty
  and calls it an absence.
  Three distinctions the build had to make and the spec does not state. **A position is a LISTING,
  not a fetch**: a cursor entry still named by a page but whose bytes are withheld leaves the
  position intact, and resuming there would hand a publisher a way to make every reader re-show old
  entries (gated as its own arm). **A read and a catch-up are different operations** — `timeline`
  does not touch the cursor, `timeline -new` advances it — and *"advanced"* means a position MOVED,
  not that the flag was passed. ⚠ **And a follow record under `app/feed/` is published by the
  ordinary act of publishing your feed**, turning §2.4's *separate, voluntary act* into the default;
  ours live under `app/workbench/feed/`, and `follows` checks the peer's real published prefix and
  says so, because the records are well-formed and the publish is correct so nothing else will.
  Two of §40's four ungated reader `[MUST]`s were closed by the reference resolver and **these close
  the other two**. What is still owed: no mirror, no removal verb, and nothing
  cross-implementation — one reader, ours, against one publisher, ours.
  ⭐ **EVERY ONE OF THOSE READER GATES WAS GREEN WHILE THE ROAD A USER TAKES WAS BROKEN** (AP108,
  2026-09-15). `BrowseModel.ReadFeedOf` is the only entry point a feed surface has — `timeline`
  reaches it through `bareBrowserOf`, the GUI through the bridge — and it wraps the road chooser,
  the per-publisher consumer memo, the shared `seq` floor and `canReachLive`. **Measured by
  mutation:** ask for the static road instead of the live one, one token, and `TestFeedReader_*`
  (including rule 6 driven end to end), both corridor ① cuts and `TestFeedLive_*`/`TestFeedPost_*`
  all stay **green**. Every one of them builds its own `fetch.Consumer` and calls `ReadFeed` with
  it, and **a test that constructs the reader's transport cannot fail on the transport the product
  chooses.** ⚠ **This bullet said *"no test in this tree named `Timeline` at all"* and that is
  FALSE — corrected 2026-09-16, and the true version is sharper.** One test named it
  (`FeedPanelTests.cs:134`, `Bridge.FeedTimelineRead`) and **could not reach `ReadFeedOf`**: it
  passes `Subject == ""` against `BridgeFixture.DefaultPeer`, which no test ever follows with, so
  `FeedTimeline`'s target loop runs **zero** times and the call under test is never made — it
  asserts three JSON key names, and it stays green under the mutation. So: **no GO test drove
  `FeedTimeline` or the verb, and the one test that named it was structurally incapable of
  exercising it.** *A test can NAME the thing it does not exercise*, so a grep for coverage finds
  it and stops looking — which is why the enumeration was offered as evidence in the first place
  and why it has to be read at the call and not at the name. Gates:
  `publish/feed_road_wiring_test.go` (the road, with the enumeration control arm that also proves
  the memoized consumer sees a re-minted root) and `shellboot/feed_timeline_test.go` (the verb,
  through `Dispatch`, asserting the discrimination reaches the **rendered** source block; plus the
  exclusion row and `Advanced`). **AP106 says assert WHICH path answered; AP108 says WHERE** — the
  fallback rescues a broken road exactly as it rescues a broken lookup, so `Via`/`Listed` belong on
  the road gate and not only on the reader gate. Two by-products worth keeping. The **live** rule-6
  enumeration is real and pinned — index unbound, re-minted, identical entry set, `Listed` false
  throughout — which is the number `entity-browser-rust` cannot produce, their live source
  implementing none, so *cannot enumerate* and *wired to the wrong road* are indistinguishable
  there. And the new harness uses **no `OpenAccess`**: `publish -public` is the authorization, and
  that is checked rather than assumed — remove it and the read is `403 capability_denied` (AP63).
  ⛔ **Still uncovered: `via` and `listed` crossing the BRIDGE.** The GUI envelope gate asserts
  `row`/`moved` (added 2026-09-15, proven by renaming the Go DTO field) and cannot reach the other
  two — `via` needs a follow and `listed` needs a reachable publisher, and the headless fixture may
  not reach the network nor write shared-peer state (AP70). So a Go-side rename of either still
  downgrades the GUI silently; the honest fix needs a second peer in `BridgeFixture`.
- **A LIST OF ENTITIES IS NOT A LIST OF MAPS, AND THE CDDL SAYS WHICH ONE BLOCK APART** (AP101,
  fixed 2026-09-13). `APP-CONVENTION-SHARE` §2.2's `audience: [* audience-entry]` plus §2.3's
  `audience-entry = { type: "app/share/audience-entry", data: {…} }` make each element a **whole
  entity map**; we emitted the bare `data` map, on every `app/share/record` this product has ever
  authored. **The same file's `share-target` arms are bare inline maps with no `type` key**, so the
  convention does distinguish the two shapes in neighbouring declarations — which makes this our
  non-conformance and not a cohort disagreement, so it was **fixed here and reported**, the
  opposite call from AP92 where the text genuinely does not decide. The tell: **a CDDL production
  carrying a `type:` key, used as a field's element type.**
  **Why every gate was green is the half to carry.** `workbench/share_publication_crossimpl_test.go`
  builds its bodies by hand from the CDDL and from their field spellings — and our hand-built
  audience and our decoder were wrong in the *same direction*, so the pair agreed with itself
  indefinitely. *A test population you generated cannot contain the shape you are missing.* Found
  in the first hour of vendoring their own emission (`entitysdk/testdata/crossimpl-rust-share/`,
  ask `B-7`), which we had filed **naming exactly this asymmetry**.
  Migration: the conformant shape is written, the legacy bare-data shape is **read-only** and gone
  at the next save — those bytes are on real machines — and an element in neither shape is an
  **error**, never a zero-valued entry, because a silent zero puts a member nobody named into an
  audience.
