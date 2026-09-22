# Cross-repo routing

Delivery versus intention, the counterpart trackers, packet form, marking which sentences are measured, and pricing work against the substrate rather than our own tree.

> Memory, per `AGENTS.md`. Every entry below was paid for by a defect that shipped or
> a measurement that contradicted what we believed. Find by symptom; the mechanism is
> stated before the fix, and `file:line` references point at the code, not at prose.

---

- **An artifact that exists only in your working tree does not exist.** Commit and push before the
  session that produced it ends, and cite the hash. We once had a specification revision folded
  upstream — and implemented elsewhere — on the strength of a document that was in no commit in any
  repo, so the provenance chain for a normative change terminated in one machine's working
  directory. We had written that exact rule *outward* hours earlier and could not see it pointed at
  ourselves.
- **Delivery is a fact; addressing something is an intention.** Before carrying a *"waiting on
  them"* row forward, establish the other side actually has it. Search for the **subject**, not the
  filename — people cite your commits and your claims, never your file paths, so a filename miss
  means nothing. One row here sat blocked for 24 days on a document that had never arrived.
  **And ARCHIVING is not delivering** — the corollary, earned 2026-09-09. We fixed a publisher
  defect, wrote the result packet, filed it in `reviews/archive/` as done, and never sent it; three
  weeks later arch's board still carried a **⛔ blocker against us for the defect it reports as
  fixed**, source-read at a commit that predates the fix by hours. Archive on **delivery plus
  reply**, never on our own side of the work being finished.
  **THIS RULE HAS A RECEIVING DIRECTION AND WE HAD ONLY EVER POINTED IT OUTWARD** (AP102,
  2026-09-13). Every instance above is a packet of *ours* that never reached *them*. On 2026-09-12
  `entity-browser-rust` routed us three, and the next day **none of the three was in this tree** —
  measured, `grep -rl` over all of `docs/` for the three stems, zero hits — including the one
  carrying a joint fixture **built, routed, and waiting on the seat that had accepted the producer
  role.** The symptom was work not happening, on our side, with every gate green; nothing was
  broken, because a missing packet is a *discovery you have not made yet*, which is the one
  category a green tree cannot report on. **Reconcile against the counterpart's TRACKER on a
  schedule you keep, not when you happen to be writing to them.** The tell is their
  *last reconciled* line being newer than yours — theirs read our tip that morning, ours was three
  days stale — and the structural cost is that *a party that reads more often than it is read
  becomes the only one who knows the state.*
  ⛔ **AND THE BACKLOG THIS RULE PRODUCES HAS ITS OWN COST, WHICH LANDS ON SOMEBODY ELSE** (2026-09-17,
  and it is the third direction). Every instance above is about a packet not arriving. The one this
  file had never written down is what happens when the whole backlog arrives **at once**: fourteen
  rows on `TRACKER-entity-core-go.md` sat at delivery *"not established"* for weeks — several filed
  in August — and were reconciled to that seat in a **single 22-row tranche immediately before a
  release cut**. They worked all of it, and worked it well; that is not the point. **We chose the
  moment, and we chose it by not choosing it for two months.** A tranche is not a neutral way to
  deliver a backlog — it is a scheduling decision imposed on a seat that had no say in it, at the
  one time of the cycle when their cost of interruption is highest.
  Two rules. **A row worth filing is worth routing the week it is filed** — the batch is the defect,
  not the rows, and a row routed on the day it is found is a cheap question while fifteen routed
  together are an audit. **And a counterpart in a release freeze gets NOTHING that does not block
  you now** — check their `STATUS.md` for release mode before opening a packet, hold what can wait,
  and say in the row that it is *held under their freeze*, which is a recorded decision rather than
  a thing you forgot. `TRACKER-entity-core-go.md` row 23 is the worked example: measured, falsifier
  ready, deliberately unsent.
- **There are exactly three counterpart seats, and each has ONE tracker at a PREDICTABLE PATH:
  `docs/status/TRACKER-<counterpart-repo>.md`** — `TRACKER-entity-core-go.md` (upstream substrate:
  a defect in an implementation of something already decided), `TRACKER-entity-system-architecture.md`
  (the specs: anything whose answer is a sentence in a document),
  `TRACKER-entity-browser-rust.md` (the peer application tier: shared shapes and interop, where
  neither side can rule anything). The path and the four sections — *Open — asks · Corrections we
  owe them · Filed, nothing owed back to us · Closed* — are the **ecosystem-wide** convention from
  arch's `SEAT-CLEANUP-INSTRUCTIONS-2026-09-09`, adopted 2026-09-09; ours started in
  `reviews/` and moved. **Other seats reconcile against the tracker, not against the directory**,
  so a packet that is not on one does not exist: the `localfiles.Handler.Load` packet was written,
  was missing from the index, and read as unwritten from two directions at once — including from
  arch, who had opened a row offering to carry it for us.
  Four rules carry the weight. **Stable ids, never renumbered** (a closed ask keeps its id).
  **An ask is ONE SENTENCE naming what must be decided**, not a summary — the packet carries the
  detail. **"Filed, nothing owed back to us" is a real section and most documents belong in it**;
  counting a for-information review as an open ask is how one seat's private notes were read as a
  44-item inbox when the true number was eleven. **Say what state delivery is in** — *filed* ≠
  *routed* ≠ *answered*, default *not established*.
  ⭐ **EACH TRACKER CARRIES A WATERMARK, AND IT IS A DIFFERENT CLAIM FROM "RECONCILED"** (adopted
  2026-09-17): `_Last read <counterpart>'s outbox through <date>, at <branch> @ <sha>._` Fetch,
  list their `docs/outbox/` for a filename dated after the line, read the header, act or ignore,
  then move the line and record the tip you scanned at. **Reconciling happens when you happen to
  be writing to them; the watermark is a claim about a PERIOD, and only the second one has an
  answer to *"what have I not seen?"*** Measured on its first run: it found
  `entity-browser-rust`'s `ROUTING-2026-09-17-c-…-OUR-HALF-OF-FEED-12-IS-CUT-…`, addressed to us,
  in no form in this tree, **one day after our own tracker said nothing inbound was missing —
  which was true when written.**
  Two rules keep it from becoming a control that lies. **Go by the date in the FILENAME, never
  file mtime**, and say whether you fetched: an unpulled checkout lists nothing new and is
  indistinguishable from a clean scan, after which the watermark advances *past* packets nobody
  saw — a permanent miss, not a late one. (From this tree another repo's git is read-only, so
  *scanned at the tip on disk* is the honest wording and it is weaker than *at their published
  tip*.) **And if you cannot reach a counterpart's tree, write that down** — *could not look* is
  not *nothing to see*, and an omitted row reads as clean.
  ⭐ **Scan the DIRECTORY BY DATE; do not grep for our own name.** Arch's
  `ROUTING-2026-09-17-c-…` has a `To:` naming only `entity-browser-rust` while its **filename**
  names both seats — so a recipient-by-header grep misses it and a recipient-by-filename grep
  finds it. Either grep is defeated by how the sender happened to spell you, and `cc:` is
  invisible to both.
  **The split that is easy to get wrong is core-go vs arch**, and the error runs one way: filing
  an implementation bug about behaviour a spec already decided. Grep
  `../entity-system-architecture/docs/proposals/` — **including `implemented/`** — before deciding
  a gate is a defect.
- **IN A PACKET, MARK WHICH SENTENCES ARE MEASURED — an unmeasured claim in the grammar of a
  measured one is the failure mode of this whole channel** (2026-09-09, three instances in one
  day, all in documents whose subject *was* measurement). We routed a naming collision found by
  measurement — file, symbol, type tag, all real — and in the same paragraph ranked alternatives
  on the claim that one *"collides with nothing in either tree"*, **which we did not check and
  which is false**: it is `EXTENSION-TREE`'s own `get` return type (`entity? | listing`), 292
  occurrences in one sibling and 70 in ours. Hours earlier we wrote *"what we need first: a
  WebSocket listener"* into a tracker about a listener **this repo has shipped, wired and gated**
  since before the row was written. The reader cannot tell the two voices apart, and neither can
  the next session.
  Two rules. **The party that MOVES a claim owns re-checking it, however short the move** — arch
  moved ours one document and it failed there; that is the check that caught it, and it is the one
  to run on anything inherited. **And a negative claim about your OWN tree needs the same evidence
  as one about a sibling** — *"we don't have X"* feels like recall and is a search, which is why
  D20's sixth payout was scored against ourselves rather than against the kernel.
- **A new outbound packet is `docs/outbox/ROUTING-<date>-<letter>-<recipient>-<slug>.md`**, opening
  with an addressee block whose fields are each **on their own line** — `**To:**` naming
  **repositories** (never a person or a nickname; a brace list is fine), `**From:**`, `**cc:**`,
  `**Re:**` (the full stem of what it answers) and `**Tip:**` (`dev` @ sha). `To:`, `From:` and
  `Tip:` are required: **a packet whose claim cannot be re-derived is an opinion.** `cc:` means
  *you are not on the hook*: if it needs acting on, it goes in `To:`. Never reuse a letter within
  a day. **Our addressable name is `entity-workbench-go`** — this repo's directory name, and
  nothing else.
  **Acknowledged packets move to `docs/archive/outbox/`** — on delivery **plus reply**, never on
  our own side of the work being finished. Neither directory is ever declared in
  `CANONICAL-DOCS.toml`; publishing the routing corpus is the expensive mistake.
  ⚠ **This entry used to end *"the packets already in `docs/architecture/reviews/` stay exactly
  where they are — do not move or rename history"*, and that was SUPERSEDED on 2026-09-17.** The
  rule was right about its own subject — do not re-file to flush a backlog, do not churn — and it
  did not anticipate the failure that actually cost us: **a recipient could not find a packet
  addressed to them**, because packets sat mixed into several hundred status files under names
  they would not have guessed. All 79 live packets and 14 archived ones are one corpus now, moved
  with `git mv`. **The packet BODIES were not edited** — only the pointers *into* them — because a
  dated packet is a record of what was true when it was sent. *A rule against churn is not a rule
  against reorganising once, for a reason the rule did not consider.*
  **Cite a packet by its FULL stem.** `ROUTING-2026-09-06-b` names a day and a letter, which is
  unique to one repo on one day and therefore not unique — three such ids in this ecosystem
  already reach three different packets each. A packet is cited far more often than it is opened,
  so a citation the reader cannot resolve is the failure that matters.
- **When you read another repo, its git is read-only.** `git status` first, stage specific paths,
  never `git add -A` outside your own working directory.

*(Sibling repositories named in this file sit beside this one under a shared parent. A `../` path
is relative to the repo root; the `../../` form is relative to a Go module directory.)*

Task start: open the matching doctrine. Every
feature/audit ends by feeding its lessons back into the disciplines — the ratchet (*a feature
must make us stronger, not weaker*).

**Doctrines — one exists now.** `docs/architecture/DOCTRINE-CRASH-FORENSICS.md` is this
repo's first, earned on the 2026-08-21 hunt: the procedure for a crash that leaves no managed
dump, in the order that actually converges (reach before forensics; first signal before any
dump; `si_code` before `si_addr`). Open it at the *start* of any crash investigation — its
whole point is that the intuitive order wastes days.
The rest are still owed: for Feature / Audit / Foundation work, run the procedures as
`METHODOLOGY.md` §7 states them and codify the substrate-native steps here when a run surfaces
one — name the recurring cycle first, then let each step own one lever of it.

- **A "no change needed" claim about another layer or repo is a hypothesis until the
  operation has been run end to end** (D19, AP10). Reading the code path establishes what
  that path does, not what the operation does — the two claims we routed on the strength of
  a correct source reading both missed a layer underneath (a lock released before a send; a
  capability pre-check). Route the measurement, not the argument.
- **Price work against the substrate, not against our own tree** (D20). An absence in
  `entitysdk/` is evidence about `entitysdk/`, not about the system. Before estimating
  anything that names a spec obligation or protocol surface, **grep `../entity-core-go` for
  it by name** — twice on 2026-08-18 the kernel already had what we were about to plan
  (`tree.CollectNodeClosure` for the §6.5.3 closure; `core/peer` + `ext/network` for two of
  the four connectivity pieces). The error always over-estimates, so it never surfaces as a
  surprise — only as work that quietly did not happen. Every "we need to build X" line
  carries the search that established the absence.
- **Not the conformance team.** When a cross-impl wire bug surfaces during perf/feature
  work, capture `file:line` + reproducer and route it (Python encoder → Python team, spec
  ambiguity → arch, conformance test-gap → core-go) — don't extend the probe into a
  validation harness. That's core-go's `validate-peer`.
