# entity-workbench-go — status

_Updated: 2026-09-17 · public: 0.9.0 (master) · working branch: `dev` (ahead of `master`)_

> **STARTING WORK? Read
> `docs/status/HANDOFF-2026-09-17-c-the-substrate-closed-thirteen-rows-and-the-test-that-should-have-caught-the-rest-read-the-wrong-peer.md`**
> — the newest: the Go kernel under us closed 13 tracker rows in one session, our sweep went 20
> failures to 4, the three that were our own tripwires are replaced with the property, and the test
> that should have caught the rest had been green since the day it was written while measuring the
> wrong peer. §4 is why nothing was routed — a decision, not an omission — and §5 is the ordered
> next list, whose first line is the joint feed run. **Nothing is blocked on us for a cut and there
> is nothing to send anyone.** Then
> `docs/status/HANDOFF-2026-09-17-b-both-walls-are-down-and-the-third-hop-closes-3-of-3.md`
> — republication now carries authorship end to end (the leg that read 0 of 3 for two
> days reads 3 of 3), the one move worth carrying — how to retire a gate that was built to fail —
> the measured tree state from a single sweep, and §6's ordered next list, whose first line is
> still that **nothing is blocked on us for a cut**. Then
> `docs/status/HANDOFF-2026-09-17-a-the-third-hop-is-half-fixed-and-the-grant-that-would-close-it-deletes-itself.md`
> — what shipped against the last ruling packet before release mode, and the falsifier that found
> the second cause; note its §1 and §5 are **superseded** by -b (the cause is fixed, and the sweep
> it reports was run in two parts). Then
> `docs/status/HANDOFF-2026-09-16-b-the-gatherer-is-built-and-the-evidence-does-not-survive-the-hop.md`
> — the gathering loop, and note its §2 cause is **superseded**: the consumer half named there is
> fixed, and what remains is a second wall in another tree. Then
> `docs/status/HANDOFF-2026-09-16-a-the-naming-rule-was-gated-somewhere-else-and-the-feeds-produce-side-reached-no-pixel.md`**
> — the newest: what shipped, the four things not to re-litigate about the feed publish control,
> the second instance of the bound-versus-content-set confusion, and §6's ordered next list (the
> first item is somebody else's sequencing and their reason for it is the good one). Then
> **`docs/status/HANDOFF-2026-09-15-c-the-release-check-two-gates-a-correction-routed-and-the-changelog-had-stopped-two-weeks-back.md`**
> — the release-readiness pass: the measured tree state, the one thing the release still needs from
> us (a version number, which is not ours to guess), and `C-7`, a correction routed to two seats
> about what it takes for a post to be readable. Then
> `docs/status/HANDOFF-2026-09-15-b-the-inbox-is-clear-the-corridor-is-cut-and-one-thing-is-parked-on-arch.md`
> — what landed, the one item parked on somebody else, the three packets owed routing, and the
> finding worth carrying (a conformance fallback will hide a broken primary path). Then
> `docs/status/HANDOFF-2026-09-15-a-two-of-five-obligations-done-and-the-plan-was-wrong-about-one-of-them.md`
> — its §0 and §3 are history; **§2 is still the plan** for W6's remaining obligations and §4's
> traps are all still live. Then
> `docs/status/HANDOFF-2026-09-13-a-the-vocabulary-agrees-and-the-fixture-found-our-defect.md`
> — its §0 is the scorecard for the vocabulary work. Then
> `docs/status/HANDOFF-2026-09-10-d-the-live-peer-implementation-plan.md`, which is still the work
> list (**W0–W5 done, W6 in progress — obligations 1 and 3 landed 2026-09-15**); the architecture it
> assumes is `docs/architecture/LIVE-PEER-DIRECTION.md` §4, whose §5 step 4 carries the state of all
> five obligations.
>
> **CONTINUING THE FEED WORK?** `HANDOFF-2026-09-10-c`'s **§0 scorecard is superseded** (bannered
> there) — pieces 2, 3 and 4 landed 2026-09-13 and the joint fixture agrees 5/5. **Its §2 is still
> the right reading for HOW**: the forced build order, the four encodings to match rather than
> re-derive, and the sizing trap. What is still not built is a **publisher** and a **reader**, and
> the reader-side `[MUST]`s have no gate here at all.
>
> **For the file-sync worklist, read
> `docs/status/HANDOFF-2026-09-10-file-sync-closeout.md`** — but note its §2.1 has been
> **corrected** — it splits every open item
> into ours / waiting-on-architecture / waiting-on-core-go, in the order to do them, with the
> one piece of work that is deliberately sequenced behind somebody else named as such.
>
> **Start here:** **§56 — a republished entry carries its author's signature all the way to a
> stranger** (both causes are fixed, the leg closes 3 of 3, and the part to keep is what you do
> with a gate that existed to pin a blocked state once the block lifts). Then **§55 — a signature
> is looked up under the peer that SIGNED it, and a feed body is drawn at the rung it deserves** (the address fix, a generalisation of ours that was wrong and
> is retracted, the remaining cause that is not in this repository, and the embed ladder — where
> the old behaviour was *conformant*, which is why no test could see it). Then **§54 — a peer
> republishes another peer's feed now, and the evidence does not survive the hop** (the gathering
> loop; **its cause paragraph is superseded by §55**). Then **§53 — a rule described as "gated" is gated over somebody's corpus, and ours
> was not it** (the check ran where the rule is written down, not where it is obeyed; plus the
> feed's produce side, which reached a command line and no pixel — and the §52 distinction biting
> a second time, one function over, in a warning that fired on the operator who had done the right
> thing), then
> **§52 — a publish prefix is a BOUND and we had been reading it as a content
> set** (the fix an architecture ruling asked for cost 386 committed keys when we measured it, and
> the ruling was that the cost was never the ruling's: it was ours, for deriving the set from the
> bound), then
> **§51 — the rule deciding what happens to your edit was reachable from the
> command line and from no pixel** (three states and the third is the one that matters; a field
> nobody declares arrives as a default, not as an error), then
> **§50 — every feed reader test was green while the road a user takes was
> broken** (a test that builds the reader's transport cannot fail on the transport the product
> chooses; assert which path produced the answer, and gate the chooser separately), then
> **§48 — posting is not publishing on the live road either** (measured against a
> neighbouring project's correction, and against a sentence of our own that had never been run),
> then
> **§47 — the speed-up had a test for the mechanism and none for the
> wiring** (a counter summed over several users keeps moving while all but one are dead; assert
> on something belonging to one user at a time), then
> **§46 — following a peer reaches a pixel** (three actions with three
> different costs; one verifying reader per peer, because that is what remembers the highest
> version it has accepted), then
> **§45 — the joint corridor is cut, and reading it back found a defect a conformance rule
> was hiding** (a fallback built to survive a hostile publisher will equally survive your own
> broken primary path — assert which path answered), then
> **§44 — the fix for our own finding works, and it would put the operator's
> private tree on a CDN** (the prescribed scope change moves the committed set from 4 keys to
> 386 and puts private state in the upload directory; a ruling that answers the objection as
> filed can still miss the defect), then
> **§43 — a peer reads another peer's feed, and the plan said to build it the
> one way the convention rules out** (`follow` / `timeline` over a real reader; four reader
> MUSTs with a gate each; a one-line role descriptor was read as naming a protocol mechanism
> and the plan of record is corrected in place), then
> **§42 — a live reference resolves now, and the absence it reports has two
> causes that must never be one** (the four outcomes are typed, named and gated against a
> publisher that republishes between reads; a root commits to a prefix, so *"not in the key
> set"* and *"not covered by this root"* are different answers), then
> **§41 — a peer publishes a feed now, and the thing that attributes an
> entry is outside everything a publisher can commit to** (the vocabulary reached a verb and a
> signed root; a second peer reads it back over the wire; the per-entry signature is in no
> published prefix), then
> **§40 — the social vocabulary agrees with another implementation on
> the first run, and the fixture that proved it found a defect one convention over** (EMBED and
> FEED built; five of five against a Rust publisher's bytes; our share `audience` had been
> non-conformant since the type was written), then
> **§39 — the live road reaches a user, and a floor that resets when the road
> changes is not a floor** (a peer-id address with no origin now works; the seq floor became
> per-publisher), then
> **§38 — publishing reached a surface, and "public" reached everyone except the
> people we knew** (one act with two projections; a catch-all authorization row is shadowed by every
> specific one), then
> **§37 — the ranking we were about to invent was already law, and its tie-break
> cannot be carried** (a publisher's declared preference was being discarded silently; the freshness
> sentence now says which road answered; one specification question routed), then
> **§36 — a site can now be read from the machine that wrote it, and the seam we
> built for it was wrong in three of four places** (plus two live defects that fell out: the SDK
> wrote sites where nothing reads them, and the publisher would sign a root committing to nothing),
> then
> **§35 — three defects the specification round handed back, and one of them ate
> 5,000 files** (two fixed, one measured and still open), then
> **§34 — the closure path holds, and the witness we were handed costs 1000× the
> spec's claim** (three measurements against the revision-3 draft), then
> **§33 — seven requests over fifty thousand entries, and an axis with a missing
> value** (the measurement that answers the specification seat's first review round), then
> **§32 — we built the same loop three times, and each one is missing a
> different layer** (the design pass it came out of), then
> **§31 — the gate that refused three legal reference forms while
> reporting a security property** (the plan's W0, landed), then
> **§30 — the pin died with the process, and the whole consume path
> points at static origins** (and the plan that came out of it,
> `docs/architecture/LIVE-PEER-DIRECTION.md`), then
> **§29 — the social vocabulary starts here, and three findings
> had been fixed and never delivered**, then
> **§28 — it worked on two real machines, and the first surface
> to read the failures was lying**, then
> **§27 — sharing was dead all morning, and the directory names
> never had to match**, then
> **§26 — the site bytes and the trie root already agree, and
> the claim that said so had never been run**, then
> **§25 — the reply pass, and the listener we had already
> built**, then
> **§24 — the specification seat answered, and one of our
> asks had been law for three weeks**, then
> **§23 — the audit of §22, and the outbound work now has an
> index instead of a directory listing**, then
> **§22 — the discovery join was dead, the connection is
> bilateral in the protocol, and the marker storm is a session that died with
> the process**, then
> **§21 — the first real two-machine session: the app diagnosed itself and
> three surfaces hid it**, then
> **§20 — the audit before live testing: what a stranger reading our docs
> would have been told wrong, and why `Mode: both` never worked**, then
> **§19 — the growth guard, the machinery that reached no pixel, and the
> conflict detector that flagged every file**, then
> **§18 — a file costs eight queue slots, two of our own numbers were wrong,
> and the bound we were about to build on turned out to be a no-op**, then
> **§17 — what it does under load, and the copy that stopped at 676**, then
> **§16 — the blocker was the instrument, and M3 is much smaller than we
> priced it**, then **§15 — the receiver stores nothing (SUPERSEDED by §16)**, then
> **§14 — three peers**, then
> **§13 — the SIGSEGV, named**, then
> **§12 — the two-peer flow, by pressing buttons**, then
> **§11 — the GUI can be driven now**, then
> **§10 — the validation audit: we are not testing the thing being tested**, then
> **§0Z — the L5 review, and the ladder that never runs**, then
> **§0Y — the flow, run; and the capability surface**, then
> **§0X — one folder, one panel, no refresh buttons**, then
> **§0W — it works, and a folder is still not one object**, then
> **§0V — driving the flow for real, and what it found**, then
> **§0U — the control loop got a face**, then
> **§0T — accept takes a directory**, then
> **§0S — the loop was right and three verbs had not been told**, then
> **§0R — four reasons nothing survived a restart, and the reconnect engine we
> never called**, then **§0Q** (the file share that shared no files, and the
> kernel line nobody read), **§0P** (the whole flow, and the wildcard that was hiding the
> permission stage), **§0O** (M2, and the subscription that was dead after every
> restart), **§0N** (the panel threw on every file, and the forensics worked),
> **§0M** (a mount you can actually use), **§0L** (the GUI can mount a directory, and it
> remembers your workspace) and **§0J** (it was not a stack overflow, and the reason we said it
> was is the real finding). Everything after those is the running history. **§0I is superseded by
> §0J and is kept only so the error is legible.**
>
> **What this file is.** The rolling engineering log for entity-workbench-go — our tree, our
> defects, our decisions — and it is **published**. Working memory that is not about this
> project's own state lives in `docs/status/`, which publishes nothing: dated snapshots,
> handoffs, and cross-team coordination. Write here for the next session, but a stranger reads
> it.

## §57 NEW (2026-09-17) — the substrate fixed thirteen defects under us, and the test that should have caught the last one was reading the wrong peer

The Go kernel this application layer sits on landed a large batch of fixes for defects found here.
Re-measured in this tree rather than taken from the report: **the full sweep went from 20 failures
across two suites to 6 across three** — the file-replication family alone went **16 → 1**. File
mounts now rehydrate after a restart; a watcher's liveness is a fact in the tree instead of a Go-API
affordance; a connection can say which side dialled it; and the reverse-write path's echo guard is
content identity rather than a five-second clock.

**Three of the remaining six were our own tripwires going off correctly.** After replacing them and
fixing the vacuous test below, the tree stands at **9 of 11 suites green, 4 failures in 2 suites**,
none of them ours to fix. They had asserted that
clock defect *on purpose*, each with instructions in its own failure message for the day it stopped
reproducing. They are now **replaced, not deleted** — each asserts the property the fix delivers,
against the same rig. That is the step most likely to be skipped in both directions: a tripwire that
keeps passing after its blocker lifts is the one failure it must not have, and one deleted on the
day it fires leaves the fix with no gate at all, in the only tree whose product depends on it. The
timing arm is deliberately separate from the arrival arm, because a build that reintroduced
suppression *with a retry queue* would deliver the second write five seconds late and an
arrival-only test that waits ten would pass.

### The finding, and it is about a test rather than about code

Two failures remained with one shape: a cross-peer **continuation** step refused with
`403 capability_denied`. Chasing them turned up a third instance that had been **green since the day
it was written**.

`TestTreeFollow_DeepTreeConvergence` asks whether a follower materialises a 50-leaf subtree. It read
the follower's mirror with `peer.List("/{other-peer}/…")` — and that call **routes by peer-id**, so
it dispatched to the *other* peer and counted *their* tree as the mirror. It reported 50/50 within
0.2 s of the source peer's own commit, every run.

**Measured, and this is what makes it more than a style point: it passes identically with the
cross-peer credential scoped to an operation that does not exist.** The assertion the test is named
after could not fail. Pointed at the follower's own index it reports **0/50** and the same refusal.

⭐ **The anti-pattern catalogue has carried this since August, with the exact sentence — *"a test
that gets this wrong is green whether or not the mirror was ever written"* — and this file was the
entry's own worst instance.** A rule written down is not a rule applied. The tell is cheap and
worth internalising: **an assertion about peer B's state that names peer A's namespace is a remote
read unless you went out of your way to make it local.**

So the blocker's measured size is three tests and one cause, where the tree said two — and the one
it hid was the one whose entire subject is *does cross-peer continuation work at all*.

⚠ **A fourth test is red and is deliberately NOT attributed to that cause.** It is a mirror chain
that delivers nothing, and hand-running it produces no error code and no failure marker of any kind
— consistent with the same blocker, not evidence of it. This tree has already been caught once
inferring a shared cause from a log that could not name one, and the correction cost a day; the
discipline applies just as much when the inference would be convenient.

### What is and is not established about the cause

**Established:** the refusal is not the presented credential — widening it to every handler and
every operation changes nothing. The gate is the *executing handler's* grant, and the handler
executing a continuation advance declares no scope of its own, so it takes a default that authorises
the local peer only.

**Not established, and it decides which of two fixes is right:** whether the step's own dispatch
credential fails to reach that check, or reaches it and is rejected. Recorded as unknown rather than
inferred.

⚠ **It is deliberately not routed.** Nothing shipped depends on it — folder sharing, the two-peer
sync gate and the publish/consume corridor are all green — and the tier that owns the fix is at a
release cut. Which is also the correction this session owes itself: a batch of long-filed asks went
out in one tranche immediately before that cut, and the cost landed on somebody else. **A finding
worth filing is worth delivering the week it is filed**; the batching was the defect, not the
findings.

Also cleared: the one file with formatting drift, as its own commit. Nothing gates formatting here,
so it accumulates silently.

## §56 NEW (2026-09-17) — a republished entry carries its author's signature all the way to a stranger

Republication works end to end now. A peer that has never spoken to an author can read that
author's entries out of a **republisher's** tree and verify every one of them against the author's
own detached signature. That was the whole point of the gathering work, and until today it was the
one leg that did not answer.

### It needed two fixes, in two layers, and each was invisible from the other side

The reader was asking at the wrong address — it looked for a signature under *the peer it was
reading from*, which is the author on a direct read and the republisher on a mirror. That was fixed
here yesterday (§55).

Fixing it did not change the count. The leg still reported **0 of 3**, and it was **not the same
0**: the detail line moved from *"nothing bound at this path"* to *"403, capability denied"* at
exactly the right path. The right question, refused — which is a different defect wearing an
identical summary line.

The second cause was underneath us, in the substrate: a peer advertises what it serves, and that
advertisement covered only the peer's **own** namespace. A republisher's grant has to name the
*author's* namespace, so the grant was not narrowed to fit — it was **dropped**. Adding a
permission row deleted the row it was added to, with the policy written and accepted and nothing
logged. That has been fixed in the substrate; we confirmed it against their tree rather than
against their report, and the leg now closes **3 of 3**, with the control arm — the same entries
read straight from the author — also 3 of 3.

The transferable half is the question, not the incident: **ask of any filter that admits by
coverage whether it NARROWS or DROPS.** The two are indistinguishable at the call site, and only
one of them is safe to widen into.

### Replacing a gate that was built to fail

While the leg was blocked, it was left as a **measurement** rather than an assertion, with a
separate tripwire pinning the blocked state. That was deliberate: an end-to-end assertion would
have reported our own fixed half as broken for as long as the other half was open, and the next
session would have re-fixed what was already fixed.

The tripwire did its job — it went red the moment the blocker lifted, and said so in its failure
message. **The step most likely to be skipped is what happens next.** A gate that only ever says
*"still blocked"* cannot then protect the fix, so it was neither deleted nor kept: it was replaced
by one that pins the property, with an anti-vacuity arm that strips out exactly the permission row
and requires the refusal back. Without that arm the passing test would be satisfied by a harness
with no permissions in it at all.

The leg-4 assertion is also ordered **after** its control arm, so a broken harness can never
present as a regression in the thing under test.

### Two pieces of housekeeping, both the same shape

The bridge test suite was added to the "run everything and report" sweep and **not** to the
fail-fast one that `make test` runs — so the gate it carries was absent from the check that gates a
change. It is in both lists now. The same directory also drops a 27 MB stray binary when built
without an output flag, in a tree whose own guidance warns against blanket staging; it is ignored
and swept like the other dir-named strays.

Both are the same failure: **a thing that exists in one list and not its twin.** Neither was
found by a test, because neither is the kind of thing a test looks at.

## §55 NEW (2026-09-17) — a signature is looked up under the peer that SIGNED it, and a feed body is drawn at the rung it deserves

Two things landed, and the first one is a correction to a claim this project made.

### The signature address, and a generalisation we got wrong

A republished entry travels with its author's detached signature, and a reader has to find it. Ours
looked for it under **the peer it was reading from**, which is the author on a direct read and the
republisher on a mirror — so an entry read through a mirror came back unattributable, with the
signature sitting at the correct address the whole time.

That much was already measured here. What was *also* filed, and was wrong, is the generalisation:
that a peer-qualified path is ambiguous — meaning both *"ask that peer"* and *"my own copy of that
peer's subtree"* — with nothing able to say which. It is not ambiguous. A request names the peer it
is **asked of** and the resource it is **about** in two separate fields, and an arriving request is
never re-routed, so the two readings are two different operations rather than one overloaded path.
Reading the protocol document settled it; we had read one layer up and concluded the question was
open.

Fixed: the signature's address is rooted at the **signer**, named explicitly by the caller rather
than defaulted (the default any reader reaches for is right on a direct read and wrong on the only
case where the distinction exists), and an already-absolute path is passed through unchanged by
both byte sources instead of being re-qualified. The HTTP half of that had been failing silently —
a path lost its leading slash and was appended to a base already naming a different peer, producing
a perfectly well-formed URL for an object nobody publishes, whose 404 reads as *the publisher is
withholding*.

> ✅ **SUPERSEDED BY §56 the following day — the remaining cause below was fixed in the substrate
> and a mirrored entry IS attributable to a third party now, 3 of 3.** The paragraph is kept as
> written because the intermediate state is the instructive part: the count did not move when we
> fixed our half, and the two zeros meant different things.

⛔ **A mirrored entry is still not attributable to a third party, and the remaining cause is not in
this repository.** A republisher cannot express — in the grant it hands a reader — that it serves
its own copy of the author's subtree. Worse, attempting it is destructive: the grant entry naming a
foreign namespace is dropped whole rather than narrowed, so *widening* a grant makes it strictly
narrower and the reader loses access it already had, with no error anywhere. Measured with four
arms, reported upstream, and deliberately not worked around here — a local shim would hide a
question the whole cohort shares.

⭐ **The two causes are gated separately, and that is the part worth copying.** A single end-to-end
gate would report the half we fixed as broken for as long as somebody else's half is open, and the
next session would re-fix what is already fixed. So the consumer's behaviour is gated with the
transport removed by construction, and the blocked end-to-end path stays a **measurement** with a
tripwire beside it that goes red — and says so — when the blocker lifts. *A gate pinning a blocked
state must not keep passing after the state changes.*

### A feed body is an embed, and we were rendering every one of them as its alt text

An entry's body is an embed node, and the embed vocabulary defines a three-step degradation ladder
ending in the author's mandatory fallback text. We had implemented the last step and nothing above
it: a markdown post showed its source, and an image post showed its alt text as though that were
the post.

⭐ **Nothing caught it because the old behaviour was conformant.** Showing a fallback is exactly
what the ladder says to do; the defect was doing it when a better rung was available. The output is
always well-formed, nothing errors, and every test phrased as *"is this valid"* passes. The gates
are now phrased as **which rung**, which is the only question that separates the two.

So: markdown renders through the same code that renders a site page, plain text renders and is
deliberately **not** handed to a markdown parser, and any lower rung is named on the row — because
a rendered body and a fallback are the same string type, and a short alt text reads as a short
post. Three states rather than two: the third is an entry with nothing to draw and no fallback
either, which is a producer error and must say so instead of rendering as an author who posted
nothing. `post -markdown` is the authoring half, so this project does not render what it cannot
produce.

⚠ **One rule in there is a security rule and the tidy implementation breaks it.** The fallback is
bounded: an embed directive inside it is shown as visible text, never expanded. The markdown rung
lowers those directives, and sharing that line between the two looks like cleanup — it would let
the author of an unrenderable entry make every reader that **degraded** fetch an asset of their
choosing. The path taken by readers that can do less must not acquire the wider reach.

### Also

The cgo bridge had no test target, so a gate written beside it would have run for whoever typed the
command in that directory and for nobody else. It has one now, and the sweep covers eleven suites.
The first thing it gates is the field names crossing into the desktop frontend, which is where a
dropped field renders as a confident zero value.

## §54 NEW (2026-09-16) — a peer republishes another peer's feed now, and the evidence does not survive the hop

The gathering loop is built. A peer reads somebody else's published feed through the verifying
reader it already has, and republishes what it obtained so the next reader does not have to gather
it again. That is the piece this repo's own audit named as owed a few days ago, and it is the first
place in the tree where the operation for binding **obtained bytes** meets bytes we did not author
— until now every one of its twenty callers was a program writing its own state.

### The rule everything else hangs off

A republished entity is bound **byte-identically** to the form it arrived in. It is never decoded
and re-encoded, not even through a type we fully declare, and the reason is sharper than a lost
field: an entity's address is the hash of its bytes, and the signature that attributes it binds at
a pointer derived from that hash. Move the hash and the signature stops naming the entity — the
result is a complete, correctly-walked, entirely verifiable publication **in which nobody wrote
anything.**

The trap is that this is not a careless mistake. Decoding a body into a local structure and binding
it back is what anybody writes first, and it is lossless over exactly the types you fully declare.
A gatherer aggregates types it did not write, so the case that breaks it is the normal case and it
raises no error anywhere.

### What we found by running it end to end

Three peers: **A** authors and publishes, **B** gathers and republishes, **C** has never spoken to A
and never will — which is the whole proposition, one verification instead of five hundred. Measured:

    the view is addressable at the coordinate C computes for itself   ✓
    it names three entries, every one attributed to A and not to B    ✓
    B serves all three of A's entries, by hash, from its own store     ✓
    C can attribute                                                0 of 3
    control: the same entries read straight from A                 3 of 3

**So a mirror we produce carries integrity and not authorship** — the one state the rules say must
never be presented as attributed. The bytes are right, the references are right, the view is
complete, and the thing the whole layer exists for does not survive the hop.

The cause is narrow and it is ours: a signature's address names **the peer who signed it**, and our
reader looks for it under **the peer it is reading from**. Those are the same peer on a direct read
and different peers on exactly this one. B is holding the signature at the correct address the whole
time — nothing is missing, it is being looked for in the wrong place.

It is not a one-line repair, which is why it is written up and routed rather than patched: asking
correctly means a read aimed at one peer for something under another peer's name, and that request
cannot currently be expressed. A second way to locate a signature is a second trust argument, and
that belongs in its own change.

### The same gap, found the same afternoon from the other end

The write side had it too, and its error message pointed at the wrong machine. Binding a gathered
entry under its author's name resolved to *the author*, over the network, asking them to accept a
write — and their refusal is indistinguishable from *"that publisher revoked us"* when the author is
not involved at all. A peer-qualified path means both *"ask them for this"* and *"my own copy of
theirs"*, and republication is the first thing here that needs the second meaning in both
directions.

### And our own new gate was measuring nothing

The pure tests use an entry carrying a field this build has never heard of, because a round trip
through bytes your own encoder produced proves nothing. The end-to-end test did not — its publisher
authored everything through our own encoder — so a version of the gatherer deliberately broken to
re-encode every entry **passed the entire live suite.** Found by mutation, fixed by planting a
hostile entry there too, and worth recording because the live harness is the one that *looks* more
real: two peers, a network, a signed root, and the weaker measurement of the property that matters.

Every gate here was then mutation-checked, and each failed on exactly the defect it is named for.

---

## §53 (2026-09-16) — a rule described as "gated" is gated over somebody's corpus, and ours was not it, plus the feed's produce side reaches a pixel

Two pieces, and they turned out to share one shape: **a check that runs somewhere other than where
the thing it checks is written.**

### The naming rule nothing was checking

The specification says an entity-type path segment is kebab-case, and it marks the rule *gated
(high precision): snake = violation*. The gate reads the specification corpus. **Entity type names
are string literals in program source** — which that gate cannot see and was never meant to. So a
rule everyone cites as mechanically enforced was unenforced everywhere it is actually written.

Measured here: **474 distinct entity-namespace path literals across 771 source files, one
violation.** It was `app/state/peer_roster_entry`, and both halves of that string were wrong. The
casing, against the gated rule. And the namespace: `app/state/` is the cross-impl portable one,
where *"consumers MUST follow the canonical schema"*, and this type was minted there to match one
other implementation — which runs the guide's own promotion ladder backwards, since it prescribes
an app-owned name first and promotion only when a type proves convergeable. The file's comment said
both things outright, four lines above a struct field documented as specific to this repo. It had
been read for months.

It is `app/entity-workbench/peer-roster-entry` now. No migration code, and that is checked rather
than assumed: the read side lists a path prefix and decodes, so it never consults the type, and the
restore path rewrites every entry on the first launch after the change.

**The rule worth carrying is not the rename.** When you adopt a rule described as gated, find the
gate and read what it walks; if your artifacts are outside it, the gate you are relying on is
somebody else's. Nobody re-asks whether the thing being scanned is the thing they are writing.

Ours exists now and runs in the ordinary sweep. Two things it had to get right, because a
source-walking gate has two ways to pass while measuring nothing — it can walk the wrong tree and
find no files, and its detector can be broken so that it flags nothing, and neither is
distinguishable from a clean result. So it asserts a floor on how many literals it scanned, *and*
runs its detector over a synthetic source containing a known violation. Its precision comes from
one character: a literal is on the namespace axis if and only if it contains a slash, which is
exactly what keeps a correctly snake-cased field key from matching — the difference between a gate
that gets fixed and a gate that gets waived.

### The feed's produce side reached a verb and no pixel

Publishing a feed so that a reader can attribute its entries was reachable from the command line
and from nothing in the window. Worse, the sentence that says whether entries *are* attributable
had never crossed into the frontend in any form — so an operator's feed could be unattributable to
every reader who fetches it as static files, with nothing on screen saying so and no control that
would have fixed it.

Closed: the publish call takes the curated-content-set choice, the attribution state crosses the
boundary, and the feed panel has a **your feed** section with a **Publish feed** button.

**Three states, not two, and the reason is a value that is a real answer on both sides of the
interesting line.** An empty attribution note means *every entry is attributable* — when a root is
published and covers the feed. It is also what a peer that has published nothing produces, because
there is no root to be wrong about. A renderer that short-circuits on the empty string therefore
tells the operator who has published nothing that everything is fine. Gated across all three arms
and mutation-checked by collapsing it to two.

### And the same distinction bit again, one function over

The ruling behind §52 — a published prefix *bounds* a publication and does not state its contents —
had a second instance in this tree, found while building the control above. A privacy warning about
publishing your own follow list was reading the published prefix as a statement about content. After
the curated publish, the prefix is the peer root, so the warning fired: it told the operator they
had published their follow list **in the same sentence that told them to fix it by doing what they
had just done**, while the root committed to nine keys and not one of them a follow record.

*A warning that fires on the one action that cannot cause the harm is worse than no warning* — it is
the line an operator learns to skip, on the surface where a real disclosure would appear. It reads
the content set now, with the unchanged case kept as the control arm, because without it the gate
passes against a check that has been deleted rather than narrowed. Same round: the feed's own
problem list still recommended the older, weaker publish — same entries, no signatures, so every
entry stays unattributable — directly above the caveat saying exactly that.

**Still owed and named rather than left to be found:** there is no way to compose a post from the
window; the panel publishes what the command line authored. And the panel's refresh signal does not
cover feed entries, so a post made elsewhere while it is open does not appear — the honest fix there
is a subscription, not a refresh button.

## §52 NEW (2026-09-16) — a publish prefix is a BOUND, and we had been reading it as a content set

A feed's per-entry signatures live at `system/signature/{hex}` and a feed publish commits to
`app/feed/`, so a reader who fetches a feed as static files cannot attribute a single entry. The
fix we were asked for was to publish over the peer root — the shortest prefix containing both. We
built it, measured it, and did not ship it: the committed key set went from **4 keys to 386**, and
the directory an operator uploads then contained their folder paths, another machine's address and
the body of a document from a folder nobody had shared.

So we filed the problem upstream with three possible answers. **All three were rejected, and the
reason is the thing worth carrying: every one of them assumed that widening the prefix widens what
gets published.** The specification says three separate times that it does not — a prefix *bounds*
a publication and is explicitly not a claim to have published everything under it, and a publisher
may declare the whole peer and publish a handful of keys. A rule that would have made our
assumption true had been added a version earlier and withdrawn in full.

⇒ **Move the bound, not the content.** Publishing at the peer root with a chosen binding set now
commits to **9 keys where the peer root bounds 399** — the entries, the index, and the one
signature that attributes each entry. On the 34-entry interoperability fixture it is **71 against
438**. The 386 was never the cost of the fix. It was the cost of deriving the content from the
bound.

**Why it survived review is the transferable half, and it is not about care.** Nothing in our
publisher ever chose a binding set, because the obvious library call does not let you: one function
takes a prefix and scans, and the one that takes an explicit list is exported and goes unused by
the reference implementation's own publish path. *The easy helper implements the reading the ruling
rejects.* A green tree never asks why the bound is also the selector.

Three things the build decided that the ruling does not state:

- **The disclosure guard is about SCANNING, not about the prefix.** A peer-root publish is refused
  without an explicit acknowledgement; the curated one needs none, because the acknowledgement
  exists for keys a scan sweeps along and a chosen set sweeps none. Making the fix reachable only
  through the flag that means *publish my private tree* would have inverted it.
- ⚠ **A published root does not record how its set was chosen, so the publisher has to.** Two roots
  over one prefix are two hashes and nothing distinguishes them — correctly, since a reader is
  forbidden to infer the set from the prefix. But *"does what I published still describe what I
  have"* is answered by re-deriving, and re-deriving the wrong way reports **"your root is
  behind" on a root that is exactly current, permanently.** That is a standing false line in a
  problems list, which is how an operator learns to skip the list; this tree has already paid for
  it once.
- **An empty chosen set is refused** for the same reason an empty prefix is: signing it produces a
  valid, correctly-signed origin that answers *absent* to every key, which a reader cannot tell
  from a feed they asked the wrong question of.

**Two gates, because neither can see the other's failure.** The mechanism gate asserts the
unchanged case against the library function the publisher no longer calls — so it cannot pass by
agreeing with itself — and the wiring gate drives the actual command, because a test that builds
the thing under test's input cannot fail on the input the product chooses. Both were
mutation-checked, and each was caught by exactly the assertion it is named for.

⚠ **Owed: none of this reaches the desktop app.** The publish button takes one option and the
attribution caveat has never crossed into the interface at all, so an operator there sees a feed
they cannot make attributable and is not told why.

---

## §51 NEW (2026-09-16) — the rule deciding what happens to your edit was reachable from the command line and from no pixel

When two machines share a folder and a change arrives on a file you had just edited, something has
to decide what happens. This project supports two answers: let the arriving version win and keep
yours recoverable, or keep both and stop converging until a person chooses. The choice is a
declaration, it is consulted on every collision, and **it could only be made by typing a command.**
The desktop panel that lists your shared folders showed neither what the rule was nor any way to
change it.

Two things were wrong and only one of them was the missing button. The panel's model had been
computing the rule on *every* reading — which folder, which rule, and whether we had read it at all
— and the layer that carries data across to the interface never declared those fields, so they were
discarded silently on the way. A field nobody declares does not arrive as an error; it arrives as a
default. That has now happened often enough here to have its own entry, and the interesting part is
that the fix is never the hard part: the fields existed, the panel wanted them, and nothing anywhere
would have reported the loss.

⭐ **Three states, not two, and the third is the one that matters.** A shared folder names one rule
and it belongs to whoever owns the folder — so the other machine's rule is something you have to
have *read*, and until you have, an arriving change that lands on your edit is **held**: nothing
overwritten, nothing lost, released on the next pass that reads their declaration. Rendering that
as the ordinary "their version wins" would have been the natural shortcut, and it would delete the
one sentence explaining why someone's file is not where they expect it. The panel says *held*, and
a test exists whose only job is to fail if that word is ever replaced by the reassuring one.

The control appears **only on folders this machine owns**. On a folder you received, the row names
who owns it instead. An enabled button that then refuses would be an interface accepting an
instruction it cannot carry out; a greyed-out one would read as a permissions problem. Neither is
true — the rule is simply somebody else's to set, and saying so is the whole of what that row needs.

Both behaviours were then verified by deliberately breaking them: collapse the third state into the
second, and offer the control on a folder we do not own. Each break was caught by exactly the test
named after it and by nothing else.

⚠ Stated rather than smoothed over: the folder list this line was added to has no height limit, and
a panel whose content outgrows its space puts controls where they cannot be clicked. That was
already a known weakness here and this change moves one row closer to it. The height budget was
raised to cover the ordinary case — a machine with a folder or two — which buys time and is not a
fix.

## §50 NEW (2026-09-15) — every feed reader test was green while the road a user actually takes was broken

Reading somebody else's feed has two halves that are easy to mistake for one. There is the
**reader** — resolve the index, check who signed each entry, fall back to a slower complete method
when the index is missing, resume when the position you were holding has been deleted. And there is
the **road**: deciding how to reach that publisher at all, remembering the highest version of their
data you have already accepted, and refusing when you are not in touch with them.

The reader had a gate for every rule. The road had none. Every one of those tests constructed its
own connection to the publisher and handed it to the reader — so **a test that builds the reader's
transport cannot fail on the transport the product chooses.**

Measured rather than argued. We changed one token so that the only call any feed surface makes
asks for the wrong kind of connection, and re-ran everything: the reader tests, both halves of the
cross-implementation fixture, and the publish-and-read pair **all stayed green**. Only a gate
written this session went red.

Counted the other way — and this is the sharper half, corrected the next day after the first
version of this paragraph overstated it. It is **not** true that nothing mentioned the feature:
one desktop test called it. What is true is that **the one test naming it could not reach the code
it appeared to cover.** It asks for a view of every publisher the operator follows, on a fixture
that follows nobody, so the loop runs zero times and the call under test is never made; it then
checks that three field names are present in the reply. Under the deliberate breakage it stays
green, which is the measurement, and it is a better example of the point than an absence would
have been: **a test can name the thing it does not exercise**, and a file-name search for coverage
will find it and stop looking.

⭐ **The reason it stayed invisible is worth more than the fix.** The slower complete method exists
precisely so a publisher cannot lie by omission about their own posts — so it is built to survive a
publisher who withholds the index. **It survives your own broken index lookup identically**, and
returns the right answer while doing it. A count of entries therefore proves nothing. The fields
that discriminate are *which path produced this list*, and both the command line and the desktop
panel already showed them; what was missing was anything that would notice if they stopped.

So the rule this project now applies: **assert which path produced the answer, not the answer, and
gate the chooser separately from the thing it chooses for.** Three separate breakages were
introduced deliberately to confirm the new gates fail on each — a wrong road, a reader that claims
the index named entries it found by the slow method, and a view that gives up on the first
unreachable publisher instead of naming it and carrying on.

Two smaller things fell out of it. The new two-peer harness authorises its reader with the real
operator gesture rather than a test-only wildcard, and that was checked rather than assumed: remove
the gesture and the read is refused for lack of permission, so the permission stage is genuinely
part of the test. And a desktop test that guards against fields being dropped between the two
languages opened by naming three fields as its dangerous category and then asserted none of them —
a criterion written in prose, never applied, and invisible to review because it reads as a
description of what the test does. Two of the three are now asserted, proven by renaming the field
and watching it fail. **The other two are still not covered there and the comment now says so**
rather than implying otherwise: they need a reachable second machine, which that test environment
deliberately does not have.

## §49 NEW (2026-09-15) — where a convention existed the two file implementations agree exactly, and every place they diverge is a place nothing was written

This project builds file **synchronisation**; an independently written application on the same
protocol builds a file **manager**. Both put files in a tree, both name them, and until now nobody
had measured where the two vocabularies meet. Measured this session in both code bases, by
enumerating every type name each one writes.

**Where a shared convention exists, they agree completely.** The sharing vocabulary is five type
names, and both implementations carry those five, spelled identically, with no local additions and no
casing drift — two application tiers built separately, converging with no coordination beyond the
convention's text and one exchanged test fixture. The one defect that comparison did surface was
**ours**, found by decoding the other implementation's own bytes rather than bytes we had built to
our own reading. The filesystem-facing namespace agrees too, because it belongs to the layer
underneath and neither of us chose it.

**Every divergence is on an axis where no convention was ever written.** Three of them:

- **What a file *is*, once ingested, was invented twice.** We classify into five buckets by file
  extension; the other implementation has its own, unrelated set. Neither name appears in any
  specification, and neither code base references the other's. Ours sits in a **top-level namespace
  we minted**, which is the more exposed of the two placements — the other implementation put its
  set under its own application prefix, which is what that prefix is for.
- **"I am offering you a file" exists in both, under two different authorities.** That may be
  deliberate: the convention is explicit that an offer is a *label* and not a permission, so a user
  interface's own offer card is legitimately not the same object as a recorded share. But both
  projects reached for the same word, which is usually a sign that one sentence is missing.
- **A "peer roster entry" now exists in three places, in two namespaces, under two spelling
  conventions** — and ours is the misplaced one. We put it in the namespace that is meant to be
  *portable and canonical*, where consumers are told to follow the schema, in order to match one
  other implementation. The file manager put the same idea under its own application prefix.
  Whichever placement is right, the two cannot read each other, and ours is the one that made the
  portability promise. It also uses the wrong word separator for a rule that is supposed to be
  enforced.

⭐ **The inference worth more than any of the three:** the checks that would catch a misplaced or
misspelled type name read the specifications, not the source, so they cannot see any of this. The
portability promise is made *in code*, and nothing reads the code to see whether it was kept. And
since the one axis with a written convention is the one where two independent implementations landed
on identical names, the problem may not be that names get promoted to shared status too readily —
it is that **nothing proposes a name for sharing until two projects have already shipped their own.**

We are flagging all three rather than fixing them. This release ships feeds and individual entries;
the alignment work is real, it is small, and it is not this week's. Saying so now beats discovering
later that both sides assumed the other had raised it.

---

## §48 NEW (2026-09-15) — posting is not publishing on the live road either, and we had asserted that without measuring it

A neighbouring project corrected itself this week on what it takes to publish a post: that writing
the entry into your own tree **is** publishing, and that a peer people can connect to therefore needs
nothing further — the exported copy being an extra step rather than the definition. The first half is
a fair description of authoring. The second does not hold for the readers that exist.

> **⚠ Corrected the same day, and the correction is ours.** This section first said the second half
> was *"false for any reader that checks signatures, which is every reader the convention
> describes."* **That overstates the convention and is withdrawn.** No conformance requirement
> obliges a reader to anchor a read on a signed root — we checked every one of them — and the
> convention says the opposite on the axis it names: an entry *"should verify alone … without a root
> that may be many publishes stale"*, the root answering a separate question it calls
> **anti-omission** — *was this in their published tree, as of sequence N?* **So an entry needs no
> root to be attributable, and the neighbouring project was right about that half.** What is
> root-anchored is **discovery**, and only as built. We were reading a prose sentence about
> verification as a requirement about reading — which is the same error as an unmeasured claim in the
> grammar of a measured one, one category over, and we had a rule for the second and none for the
> first. The check is thirty seconds: look for the conformance row before attributing a sentence to
> a specification.

Measured here, two real machines, real verification: a reader sees **three** entries after the author
publishes, **three** after a fourth entry is written with no new signature over the tree, and **four**
once that signature is made. The middle reading is the dangerous one. The reader does not fail, does
not warn, and does not fall back to its slower path — it answers from the index with the previous set,
which is indistinguishable from an author who never posted. Everything on the writing side reports
success.

**The act has three steps and the middle one is the one that publishes.** Writing the entry is local
and needs nobody. Signing a new root over your feed is *also* local and cheap — and it is the only
one of the three a reader can observe. Exporting the directory to a host is the third, and applies
only to the exported road. Collapsing the first two is easy precisely because neither touches the
network, and it drops the one step whose absence says nothing.

**The part that is ours:** this project's own notes already said a post is invisible "on both roads".
That sentence was written from the exported road and had never been measured on the live one. It is
measured now, and the note says which of its claims were run — an assertion in the grammar of a
measurement is the failure mode this project keeps writing rules about.

The finding was routed the same day rather than filed, because another implementation is building a
composer against the corrected sentence this week.

**What survives the correction is sharper than what it replaced.** A reader has two ways to find a
feed's entries: the index, and a fallback that exists precisely so the index is never treated as the
authority. **In both implementations the fallback is scoped to the signed root as well** — so the
rule written to escape one anchor inherits it, and cannot recover a post the root does not cover.
Ours enumerates the keys the root commits to; the other implementation reached all its entries by
index and none by fallback. A reader talking directly to an author has a channel an exported-copy
reader does not — it can ask the author's peer to list — and nothing in the convention says whether
the fallback may use it. That question is now open and correctly stated, rather than answered wrongly
in our favour.

**And it has since been answered, against us, on a better argument than ours.** There is no
*published* flag on an entry. So if merely writing an entry into your own tree made it published,
an author could never **draft** — the act of typing would be the act of publishing, with nothing in
the data model able to express the difference. Signing the root is what separates them. We had argued
from what readers happen to do; the answer argues from what the author loses, and it settles the
open question as a by-product: if signing is the act, the set a reader may enumerate is the set the
signature covers.

One practical rule came out of it and is worth stating on its own: **compute "your published copy is
behind" from the root, never from a count of what is published.** One post rewrites the index page in
place and changes no count at all, so a count-based indicator reads clean on exactly the case it
exists to catch.

Worth recording alongside it: the same conflation has now appeared three times in one week at three
different layers — a signature whose *path* is computable but whose *bytes* are unreachable; a rule
answered for one road that only bites on the other; and bytes that are in a tree but not covered by
anything a reader anchors on. Every one is "the artifact exists" standing in for "a reader can get
there". Two of the three were caught by the seat that made them.

---

## §47 NEW (2026-09-15) — the speed-up had a test for the mechanism and none for the wiring, and the counter that looked like proof was measuring somebody else

Two weeks ago this project stopped re-downloading pages it had already verified. A page is
addressed by a hash of its own bytes, so the second read of one is free; only the small mutable
pointer at the top of the chain has to be re-checked, and that is what makes a second visit fast
without making it a stale claim. That work had a test proving the cache itself holds bytes. It had
nothing proving the **browser actually reaches it**.

The difference is not academic. Deleting the connection between the browser and its cache — one
argument, in one function — leaves every test in this project green and every page on screen
correct, while a reader that checked a page against the publisher directly and then fell back to
the published copy pays the full download again for bytes it had just proved. Nothing fails,
because nothing about the answer changes. This is the same finding as yesterday's, one layer down:
when there are two legitimate ways to get an answer, the answer cannot tell you which one you got.

**The part worth carrying is how the first version of the new test failed.** It asserted on the
cache's own hit and miss counters, which is the obvious thing to reach for, and it passed with the
connection deleted. One cache is shared between the part that looks up names and the part that
reads pages — so the name lookups alone kept the counters climbing while every page read went
uncached. *A counter added up across several users keeps moving while all but one of them are
dead.* The fix was to assert on something that belongs to one user at a time: a page's traversal of
the publisher's tree, one per publisher, which is the expensive thing the cache exists to avoid
repeating.

Two gates now, because neither can see the other's failure — one that a second visit re-downloads
nothing, one that checking a publisher directly and then reading its published copy does not pay
twice, each with a control arm proving the measurement can move at all. Both were checked by
breaking the code on purpose and confirming they go red. In both, the assertion that *the page is
correct* is written last and labelled as the only one the defect also passes.

A second deliberate break — removing an unrelated shortcut while leaving the cache connected —
correctly does **not** trip either gate, because the property still holds. A test that fired there
would be pinning how the code is written rather than what it has to do.

The same sweep cleared two other places where a fallback could have been hiding a dead path. The
name-resolution chain already reports which of its two legal reads produced a binding, and says in
plain words which is stronger — that one was already right, and is the model. The recovery pass
that re-checks shared folders already counts and displays what it had to recover, which is exactly
the signal that the live path missed something. Reporting those as clean is part of the result: an
instrument that finds nothing has to be shown capable of finding something.

---

## §46 NEW (2026-09-15) — following a peer reaches a pixel, and one of the rules is about being able to tell outcomes apart

Following a peer's feed, reading a timeline and resolving a reference all shipped as shell commands
with nothing in the desktop app behind them. That is the fourth time a model has landed here with no
surface. For the reference resolver it is sharper than the usual case, because the obligation is
**that a reader be able to tell four outcomes apart** — so a resolver whose outcome no renderer
shows satisfies it nowhere at all. There is a panel now, with the verbs in it.

**Three actions, three different costs, and collapsing them would be the defect.** Listing who you
follow reads this machine's own tree and contacts nobody, so it updates itself. Reading a timeline
**dials every publisher you follow**, so it is a button and never a timer — an open panel that
contacted everyone you follow on each window focus is a thing somebody leaves running overnight.
Catching up dials *and* moves your saved reading positions, so it is a separate control, and the
result says whether a position actually moved rather than that the button was pressed.

One verifying reader per peer, for the whole process, because that reader is what remembers the
highest version number it has accepted from each publisher. Building one per read would give every
read a fresh memory, and a correctly-signed **rollback** replayed between two reads would be
undetectable — the same defect we fixed two days ago, reintroduced at a different seam.

Two things the panel is deliberate about. Whether a followed peer is currently reachable renders as
an observation and never as an error: following requires nothing of the far end, so following
someone whose laptop is shut is a perfectly good follow. And a view assembled from several
publishers always shows what it was assembled from, including when every source is fine — a
provenance block that only appears on failure teaches an operator that its absence means one source.

---

## §45 NEW (2026-09-15) — the joint corridor is cut, and reading it back found a defect a conformance rule was hiding

The other application implementation asked for a fixture in a specific shape: a static tree we
publish and their reader reads, **34 entries against a 32-entry page size**, because every feed
fixture either side had produced so far was one page — so the multi-page index rules had been
unfalsifiable on both of us at once. It is cut, deterministic (byte-identical across two runs; an
entry's timestamp is inside its hashed bytes, so an unpinned clock moves all 34 entry hashes and all
34 signature paths), and it is cut **twice**: once over the peer root, where the per-entry
signatures are inside what the root commits to, and once over the feed prefix, where they are not.

| cut | committed bindings | entry signatures committed |
|---|---|---|
| peer root | 438 | **34 / 34** |
| feed prefix | 37 | **0 / 34** |

That pair is the check neither side had, and it is the point of cutting twice: a single-prefix
fixture passes against both rules and measures neither. The emitter prints those two numbers itself,
rather than a README asserting them — a README claim about emitted bytes is what goes stale
silently.

⭐ **Then reading it back with our own reader found the better finding.** The reader joined a
committed key to its published prefix by concatenation, which is exactly what the specification
says — and the universal tree is the one prefix spelled with a trailing-slash-only form, so the
join produced a leading slash and **every index lookup missed**. The reader then did precisely what
the standard requires of it: an index is an optimization, its absence is a cost rather than an
answer, so it fell back to enumerating the whole committed set and returned **the same 34 entries**.
Right answer. No error. Nothing a caller would read as a defect.

Measured: **0 of 34 found by index and 34 by fallback**, against 34 and 0 on the narrow cut.

So the rule worth keeping is not *"get the prefix join right"*. It is that **a conformance fallback
built to survive a hostile publisher will equally survive your own broken primary path, and hide it
completely** — no assertion phrased over the *answer* can fail, because the answer was right. Assert
which path produced it. The field that says so had existed, unasserted, since the reader was
written; the new assertion was mutation-tested against the pre-fix code rather than merely observed
to pass after it. Two further teeth: a helper that already resolved all three prefix shapes was
sitting in the consumer package with a doc comment recording this same lesson, so the defect is a
reimplementation of something that existed; and the fallback was not even answer-identical, because
the publisher's newest-first ordering lives on an index page, so the *set* matched and the *order*
did not.

Recorded as `AP106`. It was unreachable until this fixture existed, because every feed this tree had
ever read was published over one prefix — **a fixture that models one value of a parameter cannot
fail on the others**, which is the third time that shape has cost us something.

---

## §44 (2026-09-15) — the fix for our own finding works, and it would put the operator's private tree on a CDN

The per-entry signature problem reported in §41 was ruled on: **our objection was wrong.** We had
said no prefix contains both a feed and its signatures except the whole tree; the prefix that
contains both is the **publishing peer's own namespace**, which is one peer's subtree and not the
universal tree. That correction stands and the refusal it was used to justify — a public grant over
everything — is confirmed correct and unchanged.

**So we built the prescribed fix and measured it** (`publish/a36_peer_root_probe_test.go`, three
arms). It works: publishing over the peer root puts every detached entry signature inside the
committed key set, which is exactly what a statically-published feed needs and what it had no route
to. The arm that says so carries its own negative control — the same feed published over `app/feed/`
commits to no signature key — so it is measuring the prefix and not something else.

**And it is not shipped.** Same peer, same feed, two prefixes:

| published prefix | committed keys | entities in the upload directory |
|---|---|---|
| `app/feed/` | **4** | **7** |
| the peer root | **386** | **400**, across 379 paths |

The wide emit contains a filesystem path on the operator's machine, another peer's LAN address out
of a device declaration, and the body of a document from a folder nobody shared — as bytes, in the
directory whose next step is an upload.

⭐ **The reason the gap survived a careful ruling is worth more than the finding.** We filed the
problem and argued it from the **capability grant**, because that is where this repo's last two
disclosure bugs were. The ruling refuted that argument correctly. But the obligation is about the
**static** road, and on the static road there is no grant at all — the closure is written to a
directory and copied to an origin, so there the disclosure control *is* the published prefix, which
is the one thing the fix moves. **A ruling that answers the objection as filed can still miss the
defect, because the filing party chose which objection to raise.** Recorded as `AP103`.

The structural version, which we think is the real finding: **the signature location is fixed by the
protocol, not chosen by the author.** A published root that commits to exactly one contiguous prefix
therefore cannot commit to an artifact *and* its evidence unless it widens far enough to contain
everything between them — here, an application prefix and a system prefix, i.e. the whole peer. That
is not specific to feeds and not specific to this implementation. Three options are on the table and
the question is open; a publisher-side guard that makes a whole-peer publish enumerate what it is
about to disclose is right under all three and is being built regardless.

**Also measured while reviewing a proposed schema change** (`AP105`): a canonical CBOR map's key
order is the bytewise order of the *encoded* keys, so for short text keys it is **length-first**,
not lexicographic. Where a specification says a set is ordered by a key that is also a map key,
iterating the map is the obvious implementation and it picks a different winner — deterministically,
silently, and differently in each implementation, which is the exact failure the ordering rule
exists to prevent. Measured with ids chosen so the two orders disagree. In the same probe: an
array-of-pairs carriage gives **different bytes for the same logical set** depending on the
producer's insertion order, while the map is byte-identical — so the map is the right shape and what
is missing is a sentence, not a redesign.

**And a read side that was more permissive than our write side** (`AP104`): the mandatory
degradation text on an embedded object was enforced when we authored one and not when we read one,
so the rule held against our own output and against nobody else's — green everywhere, because every
fixture was ours. Fixed, with the two ways of failing it reported as two different refusals, because
*the key is absent* and *the key is present and blank* name different producers and different next
actions. The distinction cannot be recovered after decoding, so the check reads the raw bytes.

---

## §43 (2026-09-15) — a peer reads another peer's feed, and the plan said to build it the one way the convention rules out

**This peer can now follow another peer and read what they posted**, through their signed root:
`follow` / `unfollow` / `follows` / `timeline`, over `workbench.ReadFeed`. Driven end to end on two
real peers on a loopback network — publish, connect, follow, read, catch up, read again and get
nothing, post, read again and get exactly the new one.

### 1. The correction, which is worth more than the feature

`docs/architecture/LIVE-PEER-DIRECTION.md` §3 obligation 3 said **"`app/feed/follow` as a
subscription, not a poll"**, and proposed the kernel subscription engine as the mechanism. It quoted
the convention's one-line role for the type — *"a reader's durable subscription to a peer's feed"*.

**§2.4 of the same document says the opposite about the model.** A feed-follow follows a
**namespace**: *public, pull-only, requiring no grant and no permission*, and *"the publisher does
not know the follower exists."* That is the entire discriminator against `app/share/follow`, which
follows a **grant** and therefore does tell the publisher who you are. A kernel subscription is
registered **at** the publisher — it needs authorization and it announces a follower — so building a
follow on one would have made the feed-follow the thing §2.4 says it is not, and collapsed a
distinction the convention draws deliberately.

**The mechanism was read into an ordinary English word.** "Subscription" names a protocol extension
in this corpus and also means *a standing interest in something*, and the role line means the second.
The lesson generalises past this instance: **a plan that quotes a one-line role descriptor has quoted
the summary, not the rule** — the section that defines the type is where the authorization model
lives, and a summary line cannot contradict it because it was never making that claim.

What a live peer is actually for here: §7.6's reader loop is *verify one signature, read the index
head, read down to your cursor, stop*. Over a static origin a host stands between reader and author
and can serve an arbitrarily old correctly-signed root while saying nothing. Asking the peer directly
removes that party. It does not make the read a delivery. Delivery stays available as an
**optimization for a publisher who granted one**, which is what `share` already runs on, and which
must never be described as how following works.

### 2. What the reader does, and the four rules with a gate each

`workbench.ReadFeed` reads through a `fetch.Consumer` — verified root, two-hop signature, `seq`
floor, fail-closed CHAMP walk — so nothing reaches a surface that the publisher's own signed root did
not commit to.

| rule | what it requires | how it is driven |
|---|---|---|
| `FEED-R1` | reject an entry whose `author` is not the namespace it was found under | an entry authored by a third peer, planted in the publisher's tree and named by their own index |
| `FEED-R4` | an entry with no verified signature is **unattributed**, not attributed | an unsigned entry beside signed ones; both arms asserted |
| `FEED-R13` | the index is an optimization and **MUST NOT** be the authority | remove the head and every page, republish, and assert the enumeration returns **the same set** |
| `FEED-R14` | if the cursor's `applied` no longer resolves, resume from `page` | remove the entry the reader was holding as its position, as an author removes one |

**`FEED-R13`'s arm is the comparison, not the count.** *Slower, same answer* is the rule; a fallback
that returned fewer entries would satisfy a did-not-error check while leaving the publisher able to
lie by omission, which is the exact hole the rule closes. And the fallback filters by **type**, not
by key prefix — where entries live is this implementation's choice and not normative, so a prefix
scan would find another implementation's feed empty and report it as an absence.

**A rejected entry keeps its row and loses its body.** Dropping it would make the reader's list
disagree with the index it was read from, and the rejected row is the interesting one.

### 3. Three distinctions the build had to make, none of them in the spec's text

**A read and a catch-up are different operations.** `timeline` reads and does not touch the cursor;
`timeline -new` reads from the stored position and advances it. Same split as `status` versus the
reconcile pass: a surface somebody refreshes must not quietly change durable state. *And "advanced"
means a position moved, not that the flag was passed* — a read that found nothing new and reported
"positions were advanced" is a surface describing its own mode instead of what happened.

**A position is a listing, not a fetch.** If the cursor's entry is still named by a page but its
bytes are withheld, the reader stops there and reports nothing unusual; `FEED-R14` is about the row
being **gone**. Treating a withheld body as a lost position would hand a publisher a way to make
every reader re-show old entries. Both arms are gated, and the distinction is the reason the second
one exists.

**⚠ A follow record stored under `app/feed/` is published by the ordinary act of publishing your
feed.** §2.4 makes a follow the reader's private data and publishing a follow list *"a separate,
voluntary act"*; a feed publish commits to `app/feed/`. The type tag is the cross-implementation
contract and the path is not, so the records live under `app/workbench/feed/` — and `follows` checks
what this peer actually publishes and says so if the prefix covers them, because nothing else in the
chain will: the records are well-formed and the publish is correct.

### 4. What is not done

No GUI for any of it. No mirror (obligation 4) and no removal verb (obligation 5). The reply-delivery
grant (obligation 2) is not built — only the sentence saying a reply notifies nobody. Nothing here is
cross-implementation: one reader, ours, against one publisher, ours.

## §42 (2026-09-15) — a live reference resolves now, and the absence it reports has two causes that must never be one

**A reference is the atom every social vocabulary in this cohort is built out of, and until this
change nothing in this tree resolved one.** `entitysdk.LiveRef` had a constructor and no consumer;
`APP-CONVENTION-FEED` §2.2.2's four outcomes existed as a table nobody had run.

`workbench/ref_resolve.go` is the resolver, `ref <entity+ref://…>` is the verb, and
`publish/ref_outcomes_live_test.go` drives every outcome against a real publisher that republishes
between reads.

### 1. The deliverable is the outcome, not the entity

§2.2.2 gives a live reference four outcomes and **`FEED-R7` MUSTs that a reader be able to tell
which one it got** — *the normative half is the ability to tell, not which policy it picks.* A
`(entity, error)` signature cannot carry that. It collapses rows 2, 3 and 4 into *"something went
wrong"*, and **row 2 is not a failure at all**: a document that has evolved since somebody linked
to it is the ordinary case, and the honest answer is the current bytes plus the fact that they
moved. So the resolver returns a typed outcome and keeps its error return for faults about the
**publisher** — unreachable, never published, root did not verify, committed bytes not served.

| outcome | condition |
|---|---|
| `current` | the path resolves and matches `seen`, or the link carried no expectation |
| `moved` | the path resolves to something else — **`FEED-R7`'s fact**, carried as its own field |
| `fell-back-to-seen` | the path is gone from the signed root; `seen` named bytes something still holds |
| `dangling` | nothing resolves and nothing falls back |
| `not-committed` | the publisher's root commits to a different part of its tree |

**Resolution goes through the verified walk.** A dispatched `system/tree:get` would answer in one
round trip and prove the wrong thing — an authenticated connection proves WHO, not WHAT — and a
second verification path is the thing that must not exist. The verb goes through the browser's own
road chooser and consumer cache, which is also what keeps the `seq` floor one per publisher (AP100):
a resolution and a navigation against the same peer share a floor.

### 2. The finding: a root commits to a PREFIX, so "the key is not there" has two causes

§2.2.2's table has four rows and the world has five states. A reference names a `(peer, path)`; a
published root commits to a **prefix**. A path outside that prefix is not row 3 — **the publisher
has unpublished nothing.** It minted its one root over a different region of its tree, and that root
says nothing whatever about the path, in either direction.

Folding the two together produces a confident wrong answer in the direction that costs most: *"that
document is gone"* about a document that is fine, blamed on the machine that is behaving correctly.
The two send an operator to different places — one is *"ask the author what happened to it"*, the
other is *"you asked the wrong root"* — so `not-committed` is its own outcome and its sentence names
what the root **does** commit to. This is `fetch.ErrEmptyEnumeration`'s argument one layer up:
absence carries information only once you know the question was in scope.

**The same rule caught a second case.** An empty signed root answers *absent* to every key, so a
reference resolved against one reports `not-committed` and not `dangling` — reporting a fact about
the root as a finding about the path is how a publisher's mistake becomes a link's obituary.

### 3. Row 3 is the elegant one, and both of its legs are measured

`seen` is a hash, so the bytes are self-validating: **any** store that has them satisfies the link
and none can substitute for it. That is why a live reference survives its author unpublishing the
path with no link database anywhere. Both legs are driven in one run and asserted apart, because one
passing says nothing about the other: a document this reader had already read comes back **from its
own store**, and a document it had never seen comes back **from the publisher's content store, by
hash, after the signed root stopped committing to the path.**

### 4. What was found by typing the commands, and what is still owed

`post -reply` said nothing about what a reply does or does not reach. It claimed no notification, so
it was not yet a `FEED-R9` violation — but *"reply"* means notification everywhere else a person has
used the word, and silence was the wrong amount to say. Both the verb's help and its output now say
it: a reply is published in **this** peer's namespace, and the peer replied to sees it only if they
poll or have granted delivery. Inbound delivery is granted, never ambient.

⚠ **Still owed, named rather than left to be discovered.** The outcome reaches a shell verb and **no
GUI control** — a resolver whose fact a renderer drops satisfies `FEED-R7` nowhere, and that half is
only done for one of the two frontends. Nothing here is cross-implementation: one resolver, ours.
And of the four reader-side `[MUST]`s §40 listed as ungated, **two now have gates** (§1.1's namespace
check, §2.2.2's outcomes) and two do not — §4.3 rule 6's fall-back-to-enumeration and §4.4's
resume-from-page, both of which need a feed **reader**, which this seat still has not built.

## §41 (2026-09-14) — a peer publishes a feed now, and the thing that attributes an entry is outside everything a publisher can commit to

**The feed vocabulary reached a verb, a signed root and a second peer.** `post` and `feed` are
shipped shell verbs; `entitysdk.FeedAuthor` is the authoring path behind them; and
`publish/feed_live_test.go` stands up two peers on a real connection, publishes a feed under a
scoped grant with no wildcard anywhere, and reads every entry back **through the signed root's own
walk** — verified root, two-hop signature, `seq` floor, fail-closed CHAMP walk, our decoder at the
far end.

**Why it was the next thing rather than the nice thing.** A vocabulary with no verb is a model with
no shipped surface, and the joint fixture that agreed five-of-five last session is an **encoder
comparison over authored input**: both seats had a producer, neither had published anything, so
nothing either of them wrote had ever been asked for by a reader. The publish path is what turns
that into something checkable, and it is the same item the specification seat had open from the
other direction — their ledger says *a feed publish verb; the per-entry signing capability exists
on the second application seat and is called by nothing but tests.*

### 1. What is built

`entitysdk/feed_author.go` — `Post` (entry → its `FEED-R2` detached signature → the index page →
the head, **in that order**), `ReplyTo`, `Read`, `SignatureCoverage`. `shellcmd/feed_op.go` +
`cmd_feed.go` — the two verbs and the one rendering of who can read the result. `publish.RootNow`
— what a root over a prefix *would* commit to right now, signing nothing.

**A post appends; it does not rebuild.** §4.3 rule 1 makes pages key-addressed so that rewriting
page 12 changes page 12 and nothing else, and rule 3 forbids renumbering — both only pay if a new
entry lands on the **last** page, which is why pages fill oldest-first while entries read
newest-first within a page. So one post touches exactly two index keys and a full page is never
read or re-encoded again. `BuildFeedIndex` stays the reference:
`TestFeedAuthor_AppendEqualsFullRebuild` asserts the appended index is byte-identical to a full
build over the same entries across two page boundaries, so the shipped path cannot drift from the
one the cross-implementation fixture measures.

**`ValidateInNamespace` has a caller outside its own test now** — §1.1's `[MUST]` applied at the
emitting side, where it is nearly a tautology and costs nothing, plus on the reading side of the
live gate where the bytes arrived over a wire.

### 2. ⛔ The finding: a published root cannot commit to the thing that attributes an entry

`FEED-R2` requires a detached `system/signature` per entry. V7 §3.5 fixes where it goes:
`system/signature/{hex(entry_hash)}`. A feed publish commits to `app/feed/`. **There is no prefix
containing both except the whole tree**, which a public grant must refuse for obvious reasons.

Measured, both arms, in `TestFeedLive_TheSignaturesAreNotInTheSignedRoot`:

- the signature key is **not** in the committed set — so a **static** reader, whose only authority
  is the root, has no route to it at all, and every entry arrives unattributable in a way it cannot
  distinguish from an author who never signed;
- a **live** reader gets it anyway, because the grant names `system/signature/*` as a separate
  resource. Without that second arm this would read as *"the signature is unreachable"*, which is
  false on the road we ship and would send somebody to fix the wrong thing.

The other seat mints the same signature at the same key and publishes over the same prefix, so this
is a property of the convention rather than of either implementation. Routed; `feed` prints it as a
standing caveat rather than leaving it in a doc comment, because the operator who publishes is the
only party who can act on it and nothing else in the chain will mention it.

### 3. Two defects found by typing the commands, and neither was reachable by reading

**A permanent false alarm.** `feed` reported *"the published root is BEHIND this tree"* on a peer
that had published seconds earlier and posted nothing. A trie's keys are relative to its prefix, so
`app/feed` and `app/feed/` produce different roots over identical bytes, and the staleness check
had trimmed the trailing slash. **A problems list that is always wrong is how an operator learns to
skip the problems list.** Gate: `shellboot/feed_reach_test.go`, both arms — the second one (a post
*does* move it) is the one that would have been skipped, and a hard-wired *current* is the worse
defect.

**A walk's keys are relative to the published prefix, and the convention's pinned address is not.**
§4.2 pins `app/feed/index`; a root published over `app/feed/` commits to `index`. §3.3a says so
outright — a consumer rebuilds absolute paths as `prefix + relative_key` — and the first version of
the live gate asserted on the peer-relative form and failed against a perfectly correct feed. Both
forms are right and a reader has to hold both. Left as a loud comment in the test, because a reader
implementation that gets it wrong sees an empty feed with a valid signature over it.

### 4. What this still does not claim

**One encoder, one decoder, both ours.** The live gate is not cross-implementation evidence; it is
evidence that what we emit is reachable by the verification stack we ship. The four reader-side
`[MUST]`s named in §40 are still ungated on this seat: §1.1's namespace check now runs, but §2.2.2's
four live-reference outcomes, §4.3 rule 6's enumeration fallback and §4.4's resume are not built.
**`feed` is not a reader** — it reads this peer's own tree with this peer's own authority, and the
verb says so.

Also not built: a GUI surface (the verbs are the surface; the *Local Site* panel's publish bar is
the obvious next home), the §6 mirror, and `collection`.

## §40 (2026-09-13) — the social vocabulary agrees with another implementation on the first run, and the fixture that proved it found a defect one convention over

**Two application conventions were built here — `APP-CONVENTION-EMBED` §3 and
`APP-CONVENTION-FEED` — and driven against an authored fixture whose expected values were computed
by an independent implementation in another language. Every step agreed, with no correction to
either side. The more useful finding came from the other fixture in the same session, and it is a
defect of ours that had been shipping since the type was written.**

### 1. The vocabulary, and why the build order was not a preference

`entitysdk/embed.go` and `entitysdk/feed.go`. The order — reference → embed → feed — is forced by
the documents rather than chosen: a feed entry's `body` **is** an embed node, and an embed's `child`
payload carries a reference atom. Nothing in the third file could be written before the second
existed.

Two absences in it are decisions and are recorded as such in the source:

- **EMBED §4's output surface is not built.** An entry stores what was *authored* and the handler
  runs at the *reader*; storing the output vocabulary would fix the rendition choice at authoring
  time for every reader forever and leave the dispatch and the degradation ladder with nothing to
  operate on. It would also be a model with no shipped surface, which this repo has a discipline
  against.
- **A follow record carries no cursor.** The specification's own two answers for a reader's position
  currently contradict each other, and one of them is under review. Emitting nothing commits to
  neither, and costs nothing while that is true.

### 2. Five of five, first run

The other implementation's fixture pins two Ed25519 seeds and five authored entries; their half
computes every entry hash, every detached signature, the index and its root. Ours produces the same
input through our own encoder and compares.

| step | result |
|---|---|
| peer id derived from the pinned seed | exact |
| five entry content hashes | 5/5 byte-identical |
| five detached signatures, and their invariant-pointer keys | 5/5 byte-identical |
| four index bindings, key by key | 4/4 byte-identical |
| the trie root over the convention-pinned keys | identical |

**Three neuters, each falsified, each landing on a distinct row** — and the middle one is the
interesting one. The convention pages entries oldest-first and lists them newest-first *within* a
page; the two directions run against each other, and **reversing BOTH round-trips perfectly**, so
any harness that only re-reads its own index passes with the pair swapped. Reversing both reds the
comparison here, because the comparand is somebody else's bytes.

**Two implementations, two languages, one authored input, zero shared lines.** That is the property
worth recording: a cohort all passing one author's vectors is cohort-consistent, and this is not
that.

**Four encodings were matched rather than re-derived**, each because the other seat published first
and the specification is silent: the signer field being an identity entity's content hash rather
than a peer-id string, the two page orderings, a page's `updated_at` being a witness rather than a
publish instant, and an omitted key where a default already says the same thing. The third is their
reading of a field with no stated semantics, it is under review upstream, and **we say out loud that
we matched it** — one implementation becomes the baseline either way and it should be on purpose.

### 3. The finding: a list of entities encoded as a list of maps

Vendoring the *other* fixture — bodies from their encoder for the sharing vocabulary — turned three
of five rows red within the hour, and the fault was ours.

`APP-CONVENTION-SHARE` declares a share record's `audience` as a list of **audience-entry**, and
declares an audience-entry as a whole entity: a two-key map carrying a type tag and a data map. **We
emitted the bare data map**, and had since the type was written.

**The convention decides it one block apart, and the contrast is the whole argument:** a share
*target*'s arms are bare inline maps with no type key; an audience-entry is not one. So the document
does distinguish an inline structure from an inlined entity, in adjacent declarations, and we
treated the second like the first. That makes it our non-conformance rather than a disagreement
between implementations — so it was corrected here and reported, which is the opposite call from the
reference-resolution question in the same exchange, where the text genuinely does not decide and
**neither side is moving.**

**Why every gate was green.** The nearest existing test builds its bodies by hand from the
specification and from the other implementation's field spellings — and our hand-built fixture and
our decoder were wrong in the *same direction*, so the two agreed with each other indefinitely. A
test population you generated cannot contain the shape you are missing, and this is the asymmetry
the fixture was requested to close, closing on the first run.

The conformant shape is now written and the old one is read-only, gone at the next save: every share
record an operator has authored is in the old shape, and those bytes are on real machines.

### 4. The delivery failure, and it is ours in a direction we had not considered

**Three packets were routed to us on 2026-09-12 and none of them was in this tree** — including the
one that unblocked work we had accepted responsibility for. Measured, not inferred: a search across
the whole documentation tree for all three returned nothing.

We carry a rule that *delivery is a fact and addressing something is an intention*, and we have
logged it three times. **Every one of those was a document of ours that never reached them.** It had
not occurred to us that the same failure has a receiving direction, and it is the more consequential
one: the other seat had built a fixture, a runner and a consumer, and was waiting on us.

The correction is cheap and is the transferable part: **reconcile against a counterpart's own status
file on a schedule you keep, not when you happen to be writing to them.** A party that reads more
often than it is read becomes the only one who knows the state, which is not a stable arrangement
and is not a property either side chose.

### 5. What this does not establish

**We have a producer and no reader.** Nothing here consumes a feed, renders an entry, or resolves a
live reference at read time. Four of the convention's reader-side obligations therefore have no gate
in this tree at all, and one of our own validation functions has no caller outside its own test. The
evidence is one-directional and the fixture being green does not change that.

Nothing has been published, either: these are encoder gates over an authored fixture, not a peer
standing up and serving a feed.

## §39 (2026-09-13) — the live road reaches a user, and a floor that resets when the road changes is not a floor

**Two things shipped as one: a publisher that says "you can ask me directly" is now actually asked,
and the check that refuses a replayed publication stopped being per-connection. The second was
invisible until the first existed, and it is the one worth reading.**

### 1. The half that was owed

The previous two sessions built a conformant transport ranking and a verified live reader, and
connected neither to anything a person can press. The browse model called the static-only resolver,
so **every shipped surface took the static road however loudly a publisher advertised itself as
reachable.** That was named at the time rather than absorbed, which is the only reason it was still
findable.

`workbench/browse_road.go` is the join. A binding's ranked transports become the roads this browser
can actually drive; the roads are walked in order until one answers.

Four decisions in it, and only the first is the specification's:

- **Ranking within a transport family is the spec's rule. Choosing between a static origin and a
  live peer is ours.** The rule orders profiles *of the wanted type* — it answers *which of these
  mirrors*, not *static or live*, because that depends on what the consumer can do. A reader that
  links no peer can only ever take the static road.
- **Live first, and it is not a stronger check.** An authenticated connection proves *who*, not
  *what*: the same verifier runs on both roads, by construction. What asking the publisher removes
  is a third party who could be sitting on a newer root. That is the entire difference, and it is
  the entire justification for the preference.
- **The chooser is a partition, not a re-sort.** A publisher who ranks two mirrors has expressed a
  preference, and a fix for discarding preferences must not discard that one.
- **The ladder falls through on a decline and stops at the first verified root.** A profile is not
  a promise that it currently answers, so a dial that fails moves on. But once a root has verified,
  a later failure is a finding *about that publisher* — answering it with a third party's older
  copy would delete the finding and put bytes on screen while doing it.

### 2. The refusal that became false

Typing a peer-id with no origin used to be refused: *"a peer-id address needs an origin to fetch
from — nothing in a peer-id says where its bytes are served."* Every clause of that is true about
peer-ids. None of it was true about the situation once this browser holds a peer that is **already
connected** to the one being addressed. The address exists; it is the socket.

That is a refusal asserting a fact about the world, made false by a capability on our own side —
and the shape is one this repo has been bitten by before, because a false refusal reads as rigor
and leaves no wrong answer behind for anyone to catch. It also happens to be the configuration a
laptop is permanently in: a machine on a home network has no origin and never will.

It refuses to guess. The two qualifying cases are *it is us* and *we already hold a connection*;
there is no third case in which an address is invented, because inventing one is the single move
that could put a stranger's bytes behind a peer-id somebody typed.

### 3. The finding: a floor keyed to the road is not a floor

A publisher signs each publication with a sequence number, and a reader remembers the highest it
has accepted so that a **correctly signed older** publication — a replay — is refused. That memory
lived on the reader object.

Which is the same thing as a per-publisher memory for exactly as long as a publisher is reachable
one way. Making two roads available makes two readers, and two readers had two memories.

**The split points the wrong way in the only case that matters.** A reader that verified
publication 7 by asking the publisher, and then falls back to a cached copy on a third-party
origin, would start from *no memory at all* and accept a replayed publication 3 in silence. The
fallback is precisely the road a chooser reaches for when the preferred one fails, and a third
party serving something stale is precisely what the fallback road exists to warn about. Nothing
fails; the refusal simply never happens.

The memory is now keyed by the publisher and nothing else, which is what the rule always said it
was about. The general form is worth more than the fix: **ask of any accumulated check what it is a
statement about, and whether that is what it is keyed by.**

One detail that is the real lesson. The hazard had already been *named* — a comment on the reader
cache said, in so many words, that two readers for one publisher would each hold their own floor
and that this is how a floor stops being one — and the response at the time was to key the cache
defensively so the situation would not arise. A paragraph explaining why a limitation is acceptable
closes the question permanently, and this one closed it right up until the limitation stopped
being true.

### 4. Surfaces, in the same change

The shell's browser takes the workspace's peer. The desktop browser panel takes a peer handle,
where **none** is a valid and complete configuration and an **unknown** one is an error — falling
back to the peer-less mode would leave the live road silently unavailable, and the symptom would
appear on the other machine as a publisher that looks unreachable.

### 5. Gates

- The choice itself, as a pure function: order, the declines, a browser with no peer taking the
  static road and **blaming itself rather than the publisher** for it, and a transport family we
  have no client for being declined by name instead of misdialled into an error that reads as the
  publisher's fault. With a control arm, because a refusal that refuses everything satisfies all of
  the above.
- The live road end to end, through the model both surfaces drive, over one real published act,
  with **no origin in existence** — plus a peer-less arm that must refuse.
- The floor, twice: the mechanism (with a control arm proving an unshared pair still takes the
  replay) and separately the **wiring**, because a correct implementation reached by two different
  keys is the defect wearing the fix.

## §38 (2026-09-12) — publishing reached a surface, and "public" reached everyone except the people we knew

**This peer could read anybody's site and could not publish its own from anywhere a person could
press. Fixing that took an afternoon. The hour that mattered went on the grant — and on discovering
that the obvious way to make a site public makes it readable by every stranger in the world and by
none of your own machines.**

### 1. The act, and why it had to be one act

Publishing is signing a statement: *at this moment, this peer commits to these keys, and here is my
signature over it.* Everything else — a directory of files for a web server, a peer answering
questions over a connection — is a projection of that one statement. Both projections were already
implemented; only one of them had a way in, and that way was a separate command-line program that
opens a peer's storage off disk while the peer is not running.

So there is now a `publish` verb and a strip along the bottom of the Local Site panel, both over the
same act. Ask for the static directory and you get it; ask for nothing and you get a signed root
that nobody is allowed to read, which is the right default. The signing code has exactly one
entrance, shared by both, and the refusal that stops it signing an empty prefix sits on that shared
path rather than beside it — the previous version of that guard protected one caller of one, and a
second caller would have walked straight past it.

### 2. The finding: a fallback rule is not a floor

Authorization here is a small table. A connecting peer is looked up by identity, then by peer
identifier, then under a catch-all entry named `default`; making a site public means writing that
catch-all entry.

**The lookup returns at the first match. It does not combine them.**

Which means: any peer that already has an entry of its own — every peer you have ever shared a
folder with — never reaches the catch-all. Publishing a site publicly made it readable by every
stranger who could dial the machine, and unreadable by the one other machine the operator owns.
Which is, of course, the only machine they have to test with. The site would look broken to the
single reader available and fine to everybody they could not ask.

Measured, not reasoned: a peer that had just been granted a shared folder got a flat refusal at the
publisher's signed root. The fix is to derive the public grant into every specific row, so being
known to us stops being a way to be excluded. What was deliberately **not** done is propagate a
catch-all entry somebody wrote by hand — a rule whose intent we cannot read is not one to widen on
its author's behalf.

The generalisation is worth more than the fix and it is not about this system. Any mechanism with a
"default" arm behaves this way, and the failure always has the same shape: adding a specific case
silently removes the general one, so the feature works for the population you cannot observe and
fails for the one in front of you — which reads as the feature being broken rather than as the rule
working exactly as written.

### 3. Three smaller things, each of which could have shipped quietly

**A peer has exactly one published root.** Publishing a narrower prefix stops committing to
everything outside it — with a valid signature over the replacement, so a reader asking for one of
the vanished pages gets a correctly-signed *"absent"*, which is indistinguishable from a page that
never existed. Nobody downstream can raise this; the publisher is the only party that knows what it
just stopped committing to, so it now says so. The test for that arithmetic immediately caught an
edge where publishing the *whole* tree — the one move that cannot possibly lose a key — was being
reported as taking a site dark.

**A grant scoped to the site cannot verify the site.** The signed root and its signature live
outside the prefix they commit to. Omit them and the publisher serves every page and can prove none
of them, which the reader's software reports as *"this publisher has never published"* — an
accusation against the publisher for something the reader's own configuration caused.

**Changing who may read something is not in force until the next handshake.** Authorization is read
when a connection is established, so the flag that opens a site also re-derives and re-establishes
every affected connection. A verb in this codebase shipped without that step for four days, with a
comment claiming it had been taken.

### 4. What is measured, and what is only logged

The gate runs four arms with no wildcard grant anywhere: a stranger verifies the site end to end; the
same read **fails** with the grant removed (without that arm the first proves nothing); the stranger
can read nothing else — not a file from an unshared folder, not the peer's own records of who it is
paired with; and a peer we share a folder with can read it too, which is the arm that found the
defect above and was confirmed by removing the fix and watching it fail.

One thing is measured and deliberately **not** asserted: making a site public widens an existing,
already-reported weakness from one named peer to anybody who can reach the machine. A stranger can
fetch any stored blob whose content hash it already knows. What stands in the way is that hashes are
not discoverable, and the readable-paths grant is what discloses them — which is why the paths
boundary is an assertion and the blob probe is a log. Asserting either outcome would be wrong: one
fails today, the other would fail on the day somebody fixes it.

### 5. Three of the defects came from typing the commands

Not from reading the code, and every test was green the whole time. This keeps happening, and the
cost of the habit is about ten minutes.

The status output, with nothing published yet, said *"this root is signed and in the tree"* — it was
deciding what to print from who was authorized, and never asking whether there was anything to
authorize. The refusal for an empty prefix ended with *"check the prefix"*, and the case an operator
actually hits is the one where the prefix is right and the **store** is empty: the command-line shell
defaults to an in-memory store, so naming an identity gets you the correct peer identity and a blank
tree. That message sent someone to re-read the one thing that was correct. It now reports what the
tree does hold, which separates *wrong prefix* from *wrong store* in one glance — the same rule that
applies everywhere else here: when you refuse, say what was on offer.

And withdrawing public access silently re-published at the constant default prefix, moving what the
peer commits to away from the one the operator had deliberately chosen. The default is now the
prefix already published, falling back to the constant only the first time. *A default is what to do
when nothing is known, and after the first publish something is known.* A constant one made the
commonest action of all — re-publish after adding a page — quietly change the shape of what the peer
commits to, with nothing anywhere reporting it.

### 6. Owed, and named rather than absorbed

No graphical control for the static directory projection. No public-read equivalent for a name
registry, which is still published as a batch from storage and is correctly a different operation.
And the chooser from §37 — nothing a user can reach takes the live road yet — is still owed, but it
is no longer blocked: the road now leads to a publisher who will answer.

## §37 (2026-09-12) — the ranking we were about to invent was already law, and its tie-break cannot be carried

**A published site is reachable through more than one road, and something has to choose. We sat down
to design that choice, read the specification first, and found the rule already written, already
agreed across three implementations, and implemented here in no place at all.**

A transport profile carries a `priority` — lower is preferred, DNS-SRV semantics, with a documented
default for an absent one — and the ordering rule that reads it is normative text. Our name resolver
read that field nowhere. It walked the list of ways to reach a publisher in whatever order they were
written down and took the first one it recognised. So a publisher who marked a preferred origin
first-choice and a slow mirror last-choice got whichever the registry happened to list first,
**silently**, with a perfectly good page at the end of it and nothing anywhere to indicate a
preference had been discarded — and the symptom, a reader on the slow mirror, looks like the
publisher's own misconfiguration.

That is the third time this year the answer to *"how should we decide X"* has been *"someone already
decided, go and read it"*, and the second time the someone was the specification rather than the
substrate. The habit that keeps paying is cheap: **before designing a rule, spend twenty minutes
finding out whether it exists.** The reason it keeps not happening is that a missing rule and an
unread rule feel identical from inside the code.

### 1. What a live read actually buys, said carefully

The same site is now readable two ways, and the only thing that may legitimately differ between them
is what a reader is told about **age**. That sentence is now composed in one place, from the kind of
party that answered, and it is worth writing down what the difference is — because the tempting
version of it is wrong.

Fetching from a static origin puts a third party between the reader and the publisher. That party
cannot forge anything: everything is signed and every byte is checked against a hash the publisher
committed to. What it **can** do is serve an older signed snapshot and say nothing, and from the
reader's position that is indistinguishable from a publisher who simply has not published since.
Asking the publisher directly removes that party. Nobody is in a position to withhold a newer
statement, because the party who would be doing the withholding is the one whose statement it is.

**What it does not buy is freshness.** A publisher that has not republished in a year answers
instantly with a year-old root and the exchange looks no different. One of the two doubts is
removed; the other survives both roads. There is a test whose entire job is to fail if that sentence
ever grows into the other one, because both readings are plausible English about a correctly
verified result and nothing else in the tree can tell them apart.

### 2. The tie-break is a path segment, and the two ways a profile travels have no paths ⚠ ROUTED

Implementing the rule properly meant implementing all of it, and the second half does not work.

Ordering is *by priority, then by the profile's name* — where the name is defined as the last segment
of the location the profile is stored at. That is available to a peer reading another peer's profiles
out of its own copy of their tree, which is how the substrate implements it, correctly.

It is not available anywhere else. The two mechanisms the specification defines for moving profiles
between parties both discard the location: one carries content hashes, which resolve to the record
and not to where it lived, and the other carries the records inline — and that second one states
outright that the order they appear in is not significant, while separately requiring consumers to
use the ordering rule. For two profiles of equal priority, that is not an under-specified rule. It is
an **unsatisfiable** one, in precisely the situation the mechanism exists to serve.

It has not bitten anyone because it needs a publisher advertising two equal-priority routes of the
same kind — the mirror case — and the one live federation in this ecosystem has one route per
publisher. **Every fixture models one instance of a plural relationship, so the plural case is
untestable and green**, which is a failure shape already in our catalogue under a different name.

We ship a stable order, which is deterministic, and the surface is **told** that the real tie-break
went missing rather than left to assume the publisher's preference was honored. The alternative —
quietly picking our own tie-break — is the trap: deterministic for us, different for the next
implementation, and wearing the appearance of having followed the rule. Routed as one question, with
four possible shapes listed and none of them recommended, because the trade-offs sit in a layer that
is not ours to weigh.

### 3. A road that leads somewhere nobody can go yet ⛔ OPEN

Stated plainly rather than absorbed: the ranking now reports that a publisher offers a direct route,
and **nothing a user can reach will take it.** The browser holds an HTTP client and no peer, so every
shipped surface still goes the static way. The choice function, the peer handle, and the three-way
gate that proves the choice is made correctly are owed — and are deliberately sequenced behind the
permission work, because a road that exists and leads to a publisher who authorizes nobody is worse
than no road: it fails as *"that machine is broken"* on the reader's screen.

This is the failure mode this project has a standing rule against — a capability that is complete at
every layer with no edge connecting it to a person — and the rule is being followed by naming it
here, not by having avoided it.

### 4. The suites, and which run each number came from

`make test-each`, full sweep: **eight of ten green** — the file explorer, the shell, the panel
layer, the programs track, the publish corridor, the consume corridor and the two inspectors — with
the two red ones being the sets already on record as belonging to an upstream capability change,
matched name for name.

**One number in that sweep was ours and is fixed.** The sweep was started before the last change
landed, so its table includes a renderer test this work broke, and the break is worth more than the
fix: the verification result had been given a field that only the model could fill, so **any view
built outside the model produced a verified-looking result whose scope sentence had quietly
degraded.** A test that had been asserting that sentence for weeks caught it in a hundredth of a
second. The repair was to the design rather than to the test — the field now travels as an ordinary
one alongside the sequence number and the timestamp it belongs with, and there is still exactly one
place the sentence is composed.

Two habits did the work there and both are cheap. **A count taken from one run is a lower bound**,
so the two red suites were checked against the recorded sets by name rather than by number — and an
earlier native run of the same suite produced a different count from the sweep's, which is the
reason that rule exists. And **an intermittent failure that did not fire is not a passing test**:
the one known flake in that suite stayed quiet this time, which is information about this run and
not about the tree.

## §36 (2026-09-12) — a site read from the machine that wrote it, and a seam that was wrong in three of four places

**A published site is now readable two ways from one act of publishing: over HTTP against a signed
root, and by dispatching at the peer that authored it.** Same verification stack — recomputed
content hashes, the two-hop signature against the key carried in the peer-id, the monotonic `seq`
floor, a CHAMP walk that fails closed. Measured over a 63-key site and a 5-node trie: **byte-identical
page bodies, identical committed key set, identical root and `seq`**, and locators that differ
(`entity://…` against `http://…`), which is what the freshness sentence will hang off next.

That is the feature. The three things worth a reader's time are what it cost to find out.

### 1. A seam with one implementation is a hypothesis, and this one was false

The byte-source seam shipped the day before with a single implementation and a note saying, in as
many words, that its transport-neutrality was untested until a second one existed. It was untested
and it was wrong in **three of its four primitives**, all in the same way: they returned `raw
[]byte`, because **decoding was never a check — it was HTTP's framing**, sitting above the seam
because HTTP was the only thing below it. A dispatched read hands back an already-decoded entity;
the wire bytes are the protocol's own framing and never reach that layer. A byte-shaped seam would
have forced the second implementation to **re-encode an entity purely so the layer above could
decode it again**, which is manufacturing bytes in order to check them.

The interesting one is the third primitive, because moving it moved a **conformance check**. Over
HTTP a tree leaf must serve the bound hash *pointer* and not the dereferenced entity; over a
dispatch, returning the entity is the protocol behaving correctly. A check that fires on conformant
behaviour on another transport is not a stricter check, it is a **false refusal** — a failure mode
this project has a catalogue entry for, because it reads as rigour and leaves no wrong answer for
anyone to catch. The check moved down to the transport that carries the obligation, and the seam now
answers the question both callers were actually asking: *what hash does this publisher bind here?*

Nothing that decides admissibility moved. The verification file now imports no encoding package at
all, which is the one mechanical check a reader can run on whether the seam holds. **The existing
suites were the net and not one test file was edited to accommodate the change**, including the
frozen cross-implementation fixture.

### 2. The SDK wrote sites where nothing in this repository reads them ✅ FIXED

Pointing our own writer at our own reader for the first time found that they disagreed about where a
site lives. The SDK — the surface an application developer reaches for, and what the site-seeding
tool calls — used a placement the site convention **drops by name** as a layer violation; both
resolvers here, the sibling implementation and the live corpus use the current one. So a site
authored through the SDK was invisible to every surface in this program that renders a site.

**Nothing failed, and the reason generalises.** Each half round-trips through its own copy of the
constant, so both agreed with themselves; the remote resolver's only end-to-end exercise is a frozen
fixture from the other implementation, which uses the correct path and therefore proved the *reader*
right while saying nothing about the writer. **A round trip through your own constant is not a check
on the constant.**

Worse, the divergence had already been *noticed* — written into a test's comment as a note about how
to scope a future joint comparison, rather than as a question about which of the two was conformant.
A paragraph explaining a difference closes the question permanently, where a `TODO` would have
invited the work. There is one definition now, at the lower layer, with the upper one an alias of
it; the gate asserts both packages against **spelled-out literals**, because composing the expected
path from the shared constant could not fail on the segment however wrong the segment was — which is
precisely the vacuity that hid this. The seeding tool's printed operator instructions named the old
prefix too, which is its own recurring lesson: when you move something, grep what the *program*
prints, not only what the docs say.

### 3. The publisher would sign and emit a root committing to nothing ✅ FIXED

Found by a *failing* run of the new gate, in its output rather than its assertions: `— 0 paths`,
immediately followed by a signed root. The publisher has an explicit refusal for an empty prefix and
it **cannot fire** — building a trie over a prefix with no bindings returns the hash of the canonical
*empty* node, which is a perfectly good non-zero hash. So a mistyped prefix emitted a well-formed,
correctly-signed, entirely empty origin.

The failure mode is the one this corridor works hardest to avoid: an empty root answers *absent* to
every key, with a valid signature over it, which is indistinguishable from a large site nobody asked
the right question of. The publisher is the one party that can tell those apart for free, because it
knows it bound nothing. The refusal is now on the binding count, and the gate carries a control arm
asserting the substrate fact the old guard was wrong about — without it the test would pass against a
build where nothing was ever fixed.

### 4. Two decisions, recorded because they will be re-litigated otherwise

**A peer that has never published is its own state, and this path refuses it by name.** It is not
unreachable — it answered — and not withholding, because there is nothing to withhold. Those three
send an operator to three different places and only the middle one is the publisher misbehaving.
Admitting it here would mean every check hanging off a signed root that does not exist, i.e.
structure coming from whatever the far side says it has, which is the exact inversion the design
exists to prevent. Reading an unpublished peer is a different operation; it wants a different name,
not this one made lenient.

**A grant for a published site is not the site prefix.** The signed root and its signature live
outside the prefix they commit to, so a grant scoped to the site alone yields a peer that serves
every page and **cannot be verified at all** — and the failure does not present as a permission
error. It presents as *"this publisher has never published"*: the other machine's fault, on your
screen, with the grant looking complete. Measured as its own arm, and it is the thing the public
serving grant would otherwise have got wrong.

### 5. One line of honest text made a button unreachable ⚠ OPEN

A delivered file carries no quantity a receiver can order deliveries by, so a stale one arriving
after a newer one is applied. That is now **said out loud** on the status surfaces rather than left
silent — the publish side next door does refuse a rollback, and a product with one defended path and
one silent path reads as a product with two defended paths.

Saying it cost a line, and the line broke a test: one extra row of text above the folders list
pushed a *Remount* button outside its clipping region. The sentence moved to the section it actually
belongs to — it describes the delivery mechanism, not any one folder — which fixed the symptom.
**The finding underneath is not fixed and is recorded here rather than quietly absorbed:** of the
five lists in that panel, the two with no height bound are the two that grow with what an operator
actually does. They were one line of text away from an unreachable control, and adding the bound is
not a one-line edit, because a bounded list becomes its own clipping region and the reachability
check has to learn the difference between *below the fold of something scrollable* and *unreachable*.

## §35 (2026-09-11) — three defects the specification round handed back, and one of them ate 5,000 files

**The specification seat ruled all six of our open asks in our favour and named three consequences as
ours to fix.** All three are shipped or measured here. They are unrelated to each other except in
provenance: each one is a place where a rule we argued for turned out to indict our own code.

### 1. A bounded walk with no memory bounds the FOLDER, not the pass ✅ FIXED

`walkRemoteFiles` restarted from the base prefix every pass and stopped at 20,000 entries. The
traversal is deterministic, so a folder over the cap converged to a **fixed, permanently incomplete
prefix**. Measured: 25,000 entities, two passes, **0 new paths on the second, 5,000 unreachable** —
truncation honestly reported, loop making no progress, every gate green, for as long as those files
did not change again.

The cap is not the bug and was not raised: an unbounded walk driven by a remote response is a denial
of service with our own CPU. The walk now **resumes after a cursor**, and the ordering subtlety is
the whole fix — leaves under a directory `d` all begin with `d + "/"`, so a directory must sort under
*that* key and not its bare name. With siblings `a` (a directory) and `a.txt`, sorting by bare name
emits `a/x.txt` before `a.txt`, and `a.txt < a/x.txt` because `.` is `0x2E` and `/` is `0x2F`. **One
transposed pair is a file the cursor skips forever.** Mutation-tested: with the bare-name key the
resume gate loses `a.txt`.

⚠ **And the loop had to learn the difference too.** The catch-up supervisor backs off on *"recovered
nothing"*, which the first segment of a huge folder can legitimately report while the folder is
thousands of files short. Left alone it would have taken the fix and rebuilt the defect on top of it:
progress on every pass, an hour between passes. A truncated pass now holds the floor.

20,500 entries, 2 passes, none unreachable. The control arm still measures 5,000 unreachable through
the cursorless entry point, so the fix cannot pass vacuously.

### 2. The sync leg has no rollback floor — MEASURED, not yet fixed

`fetch.Consumer.acceptSeq` refuses a published root whose `seq` went backwards. The sync leg has
nothing. We routed that as a source reading and marked it **unperformed**, and the specification seat
then narrowed a clause partly on the strength of it — so our unrun reading became load-bearing for
somebody else's text. Performed:

> `REPLAY: dispatch status=200, err=nil` · file on disk after the replay: **`version one`**

Two peers, a real connection. v1 propagates, the v1 delivery is captured while current, v2
propagates, the captured v1 is re-dispatched — and it lands. The newer file is gone, silently, with a
200.

**What is measured is that the HANDLER has no ordering check.** It is the same entry point a live
delivery and a backfill both reach, so an out-of-order live delivery lands here **by accident, with
no attacker**. **What is NOT measured is that an unauthorized remote party can trigger it** — the
probe does not cross the wire. The two sentences are kept apart in the source, because only the first
is run.

The control arm is what stops a naive fix: a sender that genuinely reverts its own file must still be
followed, so *"refuse content we have seen before"* passes the probe and breaks the product. The
sender's mtime is the only local monotone scalar and a restored backup carries an old one — which is
why the floor is not built blind. **Still ours, still open.**

### 3. A shared folder had two reconciliation rules ✅ FIXED

`FolderData.Conflict` was stored per peer, carried by no wire field, and read by each side from its
own record — so A could declare `record` while B declared `keep-both`, for one folder, and diverge
with nothing noticing. Our own ask is what exposed it, and the ruling is our own proposal: the
authority declaration names a party, that party's copy of the rule **is** the subject's, and a reader
that cannot read it **refuses rather than guesses**.

`FolderID(owner, root)` already designates the owner on every peer, including under `Mode: both`, so
nothing had to be invented. Now: `RemoteFolderView` carries the rule, the reconciler reads the
owner's declaration and records it as an **observation** — kept in its own prefix and its own type,
never merged into a declaration, because hearsay inside a declaration is a field that means two
different things depending on which side of the folder you read it from. The delivery handler reads
what the reconciler recorded; it never dials.

**A rule we have never read HOLDS a collision** (`409 conflict_rule_unknown`). Nothing is
overwritten, deliveries keep arriving, and the next pass releases it. Gated both ways — the hold and
the release — and mutation-tested: disabling the refusal fails the hold arm in 6 s.

Two consequences worth stating plainly. **The receiving side's verb now refuses**, naming the machine
to run it on: it would otherwise write a field nothing reads and hand the operator a success line for
an instruction the product will not carry out. And **the sentence has one writer** — it is on
`FolderStatus.problems()`, so a read and a pass cannot describe the state differently, with the pass
adding only the one fact it alone has (what happened when it just tried). An earlier draft put it in
the pass only, which is the exact divergence this repository forbids.

**Two more the review pass caught, and both were ours.** A held delivery was being accounted against
the conflict-storm burst limiter, on the reasoning that an unknown rule should not bypass storm
accounting — **wrong, and backwards**: a hold writes nothing and overwrites nothing, so it is already
the safest outcome the function has, and spending storm budget on it means an unreachable peer fills
the window and the operator is told *"conflict storm"* — a fault on **their own** machine that clears
by itself — when the cause is another machine they need to go and switch on. Two diagnoses, opposite
destinations, and the wrong one is the reassuring one. ⇒ **A safety outcome must not consume a safety
budget.** Gated, and mutation-checked. Separately, the owner's record is now asked for by the
**canonical** folder id derived from (owner, root) rather than by our local record's id: they differ
only for a pre-S6 record `MigrateFolderIDs` could not move, and there the local id asks for a path
the owner does not have — answered *"no record"*, holding every collision **forever** while files
keep flowing. A rare state made recoverable instead of permanent.

⚠ **Known gap, named rather than discovered:** there is no GUI control for the conflict rule at all.
It is shell-only, and the Sharing Status panel lists conflicts it cannot set the rule for.

**Suites:** `shellboot` fully green (335 s). `workbench` green. `shellcmd` 17 failures, signature
identical to the documented set — the 16 E1-stranded tests and the `F9_SelfLoop` intermittent.

---

## §34 (2026-09-11) — the closure path holds, and the witness we were handed costs 1000× the spec's claim

**Revision 3 of the data-exchange proposal restored the property the whole thing turns on, and asked
us to build it first.** Built. Four answers went back, three of them against the text.

⭐⭐ **THE CLOSURE PATH HOLDS — run, and it is the first time anyone has.** The claim is *what a peer
obtains it may publish, as the same kind of object, consumable by the identical code path* — the
argument being that every system which centralized did so because its aggregator's output was a
different **type** from its input, so there could only be one of them.
`publish/closure_probe_test.go`: A authors three entries and publishes a signed root; B consumes it
and **republishes byte-preserving into its own namespace** via `AppPeer.PutEntity`; C consumes B with
the same `fetch.Consumer`, no branch, no knowledge that B wrote none of it. **Hashes byte-identical
at both hops.**

⭐ **And the unplanned result is the better one: both peers published the IDENTICAL trie root**
(`ecf-sha256:fa5887f3e37281b4…`) under different peer-ids, because trie keys are prefix-relative and
CHAMP canonicalization is permutation-invariant. That matters because the draft says *"witnesses from
two sources are not comparable"* while another of its `MUST`s requires that two sources both serving
**be** comparable. The measurement gives the split that resolves it: a **content-derived** witness (a
trie root) is comparable across sources; a **source-minted** one (a signed pointer carrying `seq`, an
entity-tag) is not. Note the trap inside our own chain — the *manifest* differs per peer and the
`root_hash` it commits to does not, so a reader comparing manifests sees disagreement where there is
none.

⛔ **The control arm is what earns it: the naive republish moved 3 of 3 hashes.** Decode the body
through a struct that does not declare one of the publisher's fields — **the realistic gatherer, which
aggregates types it has never heard of** — and the field is dropped in silence (AP49), the entity
re-encodes, the hash moves. **The consequence is not a lost field, it is attribution**: a detached
signature is bound at `system/signature/{hex(entry_hash)}`, so every republished entry becomes
*unattributed*, which the feed convention then obliges a renderer to say. A complete, verifiable,
correctly-walked publication in which nobody wrote anything. Filed as A-27: the closure property needs
a byte-preservation `MUST`, and the SDK needs to name the bind-an-obtained-entity operation, because
the ordinary `Put(path, type, data)` shape is the one a developer reaches for and it is the broken one.

⛔⛔ **The witness they recommended is three orders of magnitude more expensive than the spec claims.**
They pointed us at `EXTENSION-TREE` §3.7.1 — reconstruct a trie over the bindings under a prefix,
*"O(n log n), microseconds to low milliseconds"* — as the answer to our live-peer gap, and rejected
§3.7.2's maintained sidecar as an over-priced proof. Measured (`shellcmd/exchange_probe_test.go`, best
of 5, core-go's only builder): **34 ms / 455 ms / 2.62 s over 1,000 / 10,000 / 50,000 bindings.** The
asymptotic is right and the constant is ~10⁴ out, because **each of those operations is a CBOR encode
plus a SHA-256 plus a content-store put, not an arithmetic step** — per-binding cost grows 34 → 46 →
52 µs exactly as n log n predicts. ⇒ **as a per-pass witness the trade inverts**: the source pays
seconds of CPU per reader per pass where today it pays one B-tree range scan, and the sidecar
(O(log n)/write, O(1)/read) is the cheap shape. Reconstruction is right for a **one-off** comparison.
⚠ Stated as a limit on our own number: that is core-go's incremental builder, not a lower bound — a
bulk bottom-up builder would be faster and **nobody has written one anywhere.** Our build order is
unchanged and still right for our topology: let a live peer publish a root for the prefix it shares,
and the witness is a fixed-key pointer at 2 requests.

✅ **C15 is a measurement now.** 25,000 entities, cap 20,000, two passes: **identical sets, 0 new
paths, 5,000 unreachable** until they change again. The prediction we routed as unperformed is
performed, and it is ours to fix.

⛔ **`FolderData.Conflict` does not survive contact, and we are non-conformant.** The new `MUST` says a
`shared` subject's reconciliation rule belongs to the **subject**. Ours is written at
`app/workbench/folders/{folder-id}` **in each peer's own tree**, and `RemoteFolderView`
(`shellcmd/remote_declaration.go:81`) — the only thing that reads the other side's record — carries
`Root`, `Mode`, `Publishes`, `OurState` and **not `Conflict`**. So A can declare `record` while B
declares `keep-both`, one delivery lands on an edit at each, **A ends with one file and B with two, and
they do not converge** — the exact failure the `MUST` exists to prevent. The fix is available because
`FolderID` is `{owner-peer-id}.{sender-root}` and therefore designates a party even under
`Mode: both`: the owner's declaration is the subject's rule, receivers read it over the channel that
already exists, and a receiver who cannot read it **refuses rather than guessing**. A-29.

⛔ **Three more against the text, each one clause.** The six source outcomes **have no row whose owner
is the reader** — `fetch`'s `ErrSeqRollback` is served, authentic, verified and refused on *our*
monotonicity policy, and by their own generative rule it merges with none of the six (A-30). §6.3.3's
three clauses — ours, adopted verbatim — name trie nodes in a section whose opacity rule exists so a
plain web origin with entity-tags can be a source, which cannot satisfy them. And *"a monotonic floor
is not a valid witness for `shared`"* **deletes the only rollback defence**: a `shared` subject's legs
are each `owned` by one writer, which is precisely the granularity a floor is meaningful at, so as
written the row makes our own missing sync-leg floor *conformant* (A-31).

**Four of our asks closed in our favour** — the third authority value is adopted as `shared`, D6 lands
with the feed correction as `D6a`, the submit path is in scope after all (only multi-peer *atomic*
commit is out), and A-13's placement question is answered by their four-document layer map. Packet:
`docs/status/ROUTING-2026-09-11-d-…-the-closure-path-holds-…`. **Nothing blocks us; `W2`/`W3`
continues, and their §15.6 confirms the rung work and this mechanism are one arc rather than two.**

## §33 (2026-09-11) — seven requests over fifty thousand entries, and an axis with a missing value

**The specification seat turned §32's derivation into a drafted mechanism and sent it back for
review with seven questions.** This section is what we measured in order to answer them, and the
two things we are asking them to change.

**The measurement, because nobody had one.** The largest tree anywhere in this repository is a
51-node site, which is too small to tell an expensive design from a cheap one. A published feed
prefix, three scales, a reader holding the previous walk's nodes in the content-addressed cache:

| entries | trie nodes | cold walk | no-op currency check | **1-entry delta** |
|---|---|---|---|---|
| 1,000 | 53 | 56 requests | **2** | **5 requests** |
| 10,000 | 1,054 | 1,057 requests | **2** | **6 requests** |
| 50,000 | 3,342 | 3,345 requests | **2** | **7 requests** |

⭐ **A returning reader pays the root-to-leaf path, not the tree.** Fifty times more data costs two
more requests. **The control arm is what makes that a measurement**: the same one-entry delta read
by a cacheless reader costs **3,346 requests**. Gate: `publish/feedscale_probe_test.go` — it is kept
rather than thrown away because the whole result rests on the blob cache surviving a root move, and
a 51-node fixture cannot tell 5 requests from 56.

**What it decides.** *Answering "what is new?" by comparison* is affordable at social scale, so a
publication index does not need to be the change-detection mechanism. It also makes one landed
sentence false: `APP-CONVENTION-FEED` §4.1's *"discovering what is new under a prefix costs the whole
tree … the index is not a convenience added after the fact"* is true of a first read and of local
traversal, and **false of the case it was written about.** Filed as A-25, because that sentence is
what someone will cite in two years to re-add the index as a change detector.

**The cost that is still O(n) is local traversal of already-cached nodes** — 63 ms over 3,343 nodes
at 50k entries, because a walk is memoized per root hash and a moved root re-walks. Network is what
a reader is charged for. Do not quote the good number for the other thing.

⛔ **The axis with a missing value, and we ship a counterexample to it.** The drafted mechanism
classifies every subject as **owned** (exactly one writer, convergence required) or **ownerless**
(many writers, convergence *forbidden*). **A folder shared `Mode: both` is neither** — two writers,
and convergence is the entire feature. So is one person's notes across their own laptop and phone,
and so is a turn-based game with an authoritative host and N submitters. **The third value is
*many writers, convergence REQUIRED, and the subject declares its reconciliation rule* — and we
already ship the declaration**, as `FolderData.Conflict` (`workbench/desired_state.go:326`). Filed
as A-23. The social convention's own `F-10` records the same empty cell from the other tier.

⚠ **A defect of ours, found by answering their question about walk bounds, and NOT yet run.**
`walkRemoteFiles` (`shellcmd/sync_backfill.go:228`) starts from the base prefix every pass and stops
at 20,000 entries, deterministically — so **a folder over the cap converges to a fixed, permanently
incomplete prefix**, truncation honestly reported, loop making no progress. The predicted failing
case (two passes over a >20,000-file folder, assert the second reaches paths the first did not) is
**a source reading and has not been performed.** Ours to run.

**Also answered:** comparison-primary is right for our file sync and does not reintroduce the
saturation blind spot — but its *cheap* arm assumes a published root, and neither peer in a LAN
share publishes one, so the affordable check is unavailable in the topology the product ships in.
And their generalization of our *every field is MINE, THEIRS or OURS* rule into a per-subject owner
axis is **not faithful**: our actual defect was a THEIRS field inside a subject their axis classifies
correctly as owned, so the two are orthogonal and both are needed.

Packet: `docs/status/ROUTING-2026-09-11-c-entity-system-architecture-seven-requests-over-fifty-thousand-entries-and-the-owner-axis-has-a-third-value-we-ship.md`.
**Nothing in it blocks us and nothing in it changes what we are building** — `W2`/`W3`, the
live-peer consume chain, continues.

## §32 (2026-09-11) — we built the same loop three times, and each one is missing a different layer

**Implementation paused for a design pass, on purpose.** The question is whether the layer we keep
building by hand is at the right altitude, or whether one application's vocabulary has been standing
in for a general mechanism. This section is the derivation, and the evidence turned out to be
already in the tree.

**Three instances, not one.** This repository contains three independent implementations of *"keep
a local view current against a remote one"* — **static consume** (`fetch/`, against a signed root
over HTTP), **file sync** (`shellcmd/` + `workbench/`, against a live peer over subscriptions), and
**discovery** (a LAN scan). Three sessions, three transports, three meanings-of-arrival, and
**they share no code at all.** They were never compared until now.

**They converged on the same answer to the only question that matters** — *am I current?* All three
**re-derive current state and compare by hash**; none of them replays a change log. That is not a
style preference, it is what content-addressing makes cheapest: when identity is a hash, currency is
answerable by comparison, and a change stream becomes an optimization rather than a requirement.
**The practical consequence is that a cursor is a liveness accelerator, not a correctness
primitive** — file sync has never had one and converges anyway, which is a sentence worth being able
to say about any such design.

⭐ **The finding is that each instance is missing a different piece, and each one's known defects are
exactly that absence.** Laid against a seven-layer reading — byte source · commitment · identity of
the replicated thing · position · intent · convergence · lowering:

- **Static consume** has commitment and position (a signed root, a monotonic floor that refuses a
  rollback) and **no durable intent and nothing that reconciles.** Its defect family is the registry
  pin that died with the process (§30): success reported, nothing persisted, two surfaces unable to
  share one trust decision.
- **File sync** has intent, convergence and lowering — the declare-then-reconcile loop, the catch-up
  supervisor, the adapter that turns an arriving change into bytes on disk — and **no ordering
  primitive at all.** Measured this session: the receive path's currency check is an *equality* test,
  so an older version arriving is indistinguishable from a newer one. The static side refuses that
  case by design; the sync side cannot see it. *(A source read of the path, not a run — it predicts
  a specific failing case nobody has performed, and it is the next thing to measure.)*
- **Discovery** **fuses identity with position**: the observation timestamp is a field of the
  observed entity, so every re-observation of an unchanged peer mints a new content hash. *Nothing
  changed* and *everything changed* become byte-identical, which is why no amount of downstream
  deduplication helped the render churn.

**The rule that came out of it, earned on three instances rather than argued:** *position belongs to
the reader/thing pair. It may live inside the thing only when the thing has exactly one writer and
the position is that writer's own.* A published root's sequence number satisfies that and is correct.
An observer's timestamp written into the observed thing multiplies the thing's identity. A reader's
cursor written into the reader's own declaration turns a declaration into a log — and that one is the
shape this repository already removed once, when the sharing flow became declare-then-reconcile.

**And the second rule, which a layer diagram will not give you.** Layers say *what job*; they do not
say *who can know it*. Every defect in the sharing area for a month was one move: **a fact that
neither peer can hold alone, stored as a private one, then read as authoritative.** We had written
that rule down and enforced it only where things are *rendered*, never where they are *recorded*,
which is where it decides behaviour.

**Nothing in the product moved.** Measured at `ddf99a0`: the application vocabulary under discussion
appears **zero** times in this repository's Go code. The one built piece — the reference atom, eleven
of eleven vectors — is the piece that is not application-specific. So this is a design pass with no
migration attached, taken at a checkpoint deliberately.

**One correction landed with it, and it is the uncomfortable kind.** A review this repository wrote
on 2026-09-04 warns that we cannot guarantee delivery. **That severity was retracted four days later**
— it is a delay of about 107 seconds, not a loss, and no operator action is needed — **and the
retraction reached six documents and not that one**, which was the copy addressed to another team and
aimed at a requirement that could have hardened around a defect we do not have. The document is now
corrected at the top and at the claim. What survives is narrower and still true: we can meet
*delivered*; we cannot yet bound *delivered within N*.

## §31 (2026-09-11) — the gate that refused three legal reference forms while reporting a security property

**`APP-CONVENTION-REFERENCE` §3.4 names four spellings a reference may be written in. Both places
this program reads one admitted a single form**, and the function doing the refusing is documented —
correctly, as far as it went — as the security gate that stops a page body steering the renderer at
a tracking URL. So three legal forms were declined by a refusal phrased as protection, which is the
one nobody goes back and questions.

```
entity+ref://{peer}/sites/lab/assets/x.png   caught by the `://` arm
/assets/figures/x.png                        caught by the leading-`/` arm
site:other-site/assets/x.png                 never reached at all
```

The middle one is the one that bites: §3.4 **SHOULDs** root-absolute form for
application-generated links, precisely so a link resolves identically from whatever page it is
rendered on. A publisher following that SHOULD got a fallback caption and no way to find out why.

**At the link position it was worse than a refusal — it was a wrong answer.** `entity+ref://` matched
no arm and fell through to the relative resolver, so
`entity+ref://PEER/sites/lab/pages/intro` came out as the in-site page slug
`entity+ref:/PEER/sites/lab/pages/intro` — a well-formed address of something nobody published,
reported to the operator as *page missing*, i.e. as the publisher's fault. That is §3.4's own named
failure: *"a tolerant re-anchoring scan produces a well-formed wrong location and cannot report that
it did."* The same fall-through caught `data:`, `ftp:` and `javascript:`, which the old code let
past by naming only `http`, `https` and `mailto` as external.

**The fix is a split, not a loosening.** One predicate answers *which of §3.4's forms is this*
(`ClassifyRefForm`); a second answers *does this leave the system* (`RefLeavesSystem`) and is
**derived from** the first rather than written beside it, because two predicates maintained
independently are two chances to disagree about one string and the disagreement would be invisible.
`ClassifyAssetRef` is the gate now, and every one of its four arms funnels through the existing
containment check, so *"is this inside the site's assets subgraph"* still has exactly one
implementation.

**What was deliberately not touched: `AssetNameFromRef`.** It is cross-implementation algorithm
contract — the output selects bytes in a content-addressed tree — and its vectors are carried
verbatim from the reference implementation. The fix went *around* it. A test asserts it still
refuses all three forms on its own, so a future session cannot satisfy the new gate by widening the
shared function, which would convert a fixed bug into a silent divergence.

**Every refusal that was doing real work still fires**, and that half is gated explicitly, because
*"stop refusing three legal forms"* has an implementation that refuses nothing and it would pass
everything else. `/etc/passwd` is still refused — now for not being under `assets/` rather than for
being absolute, which is the rule that was carrying the argument all along.

Two things carried out of it. **A pinned reference is a third state**: well-formed, neither hostile
nor malformed, naming bytes rather than a place in a site — so it refuses under its own name rather
than being folded into either of the other two. And **a colon is only a scheme when it precedes any
`/` and follows an `ALPHA` start**, or the fix for over-refusal invents its own: a published figure
named `a:b.png` would otherwise stop rendering.

**One thing measured on the way out, because it will otherwise be read as somebody's regression.**
The `shellcmd` suite's failing set has been recorded as a flat *16, none ours* since the kernel's
capability change. Two full runs at the same commit on the same machine disagreed: one 16, one 17.
The extra is `TestStage3_F9_SelfLoop_SinglePeer`, it passes when run alone, and `-count=12`
reproduces it on a tree that predates this change — **byte-identical signature**, so it is
pre-existing and load-dependent. Two things worth keeping. Its failure message says *"RUNAWAY LOOP.
F9 has regressed"* and names two source lines to go and check, so an intermittent reads as a
serious regression and sends the reader somewhere correct and irrelevant; and **a red count taken
from one completed run is a lower bound**, which is the familiar anti-pattern arriving with no
early-exit anywhere in it. The settled set is still 16.

**Open and stated rather than assumed:** we now resolve asset refs the other application-tier
implementation refuses, so one page could render different figures in the two readers. That is filed
as an ask the day it landed rather than left to be discovered, along with a question neither seat can
settle — §3.4 says a relative reference is directory-relative to the current *page*, while both
implementations and the live corpus resolve an asset ref against the **site root**, and those agree
only when the page is at the root.

## §30 (2026-09-10) — the pin died with the process, and the whole consume path points at static origins

**Written after running the product instead of reading it**, which is the only reason any of this
is here.

**The journey works, live, and it is the strongest thing we ship.** One session against the public
federation: `registry pin` → TOFU pin, labelled as origin-nominated, layout re-based onto it;
`registry ls` → five names from the **walk** of a signed root; `open billslab.com` → ten chain
steps, 51 CHAMP nodes, 966 committed keys, the page. Every step says what it proves.

**And the pin did not survive the command that made it.** Measured in two invocations with the same
`HOME` and `-storage sqlite`: `registry pin` printed four lines of success, and the next command
printed *"nothing pinned"*. `ShellWorkspace.Browser` is process state; **nothing in this tree wrote
`~/.entity/browser.json`** (zero writers, measured) and the file was read only by the GUI. So the
one fact the consume design says the operator supplies out of band was the one fact discarded at
exit, and the shell and the desktop app could not share a trust decision either. That is D26/AP62 —
derived state re-established at open — landing on the single most important durable fact on the
consume side, in a repo that has now hit that shape four times.

Fixed: `workbench.SaveBrowseConfig` is the writer, `registry pin` persists origin and key (never a
`-pin-*` layout — a hand-tuned probe must not become the durable default), `registry unpin`
persists `auto_pin: false` so the verb does not reverse itself at the next launch, and the shell
applies the start-up pin lazily on the first browse verb, the same as the Browser panel.
`BrowseAutoPin` is an in-process flag for `AutoPinOnOpen`'s reason — a start-up pin is a network
fetch and no suite here may reach the public internet. **The gates cross the file boundary rather
than asserting on a model**, because a test that keeps the model alive passes against the broken
build; the unpin arm is the one the obvious implementation fails.

**The larger finding is what the whole path points at.** `fetch.Layout` is built from an http-poll
profile, `OriginFor` refuses a binding committing to no usable http-poll transport, and the only
remote `ContentResolver` is constructed from a completed **static** walk. There is also **no
`publish` verb** — 40-odd shell verbs, none of them publishes, and the GUI has no panel; publishing
is a separate binary that reads a store off disk. So this seat currently *reads what somebody else
emitted, and agrees*. That is the smaller half of what a running peer can do, and it duplicates the
web tier instead of complementing it.

`docs/architecture/LIVE-PEER-DIRECTION.md` is what came out of it: the two modes side by side, the
five obligations in the published conventions that **cannot be exercised without a second live
participant** — `FEED` §2.2.2's rows 2/3/4 behind the `FEED-R7` MUST, §3's grant-scoped reply route,
`app/feed/follow` as a real subscription, §7.1's second source, §7.3/§7.5's
unpublication-is-not-erasure — and the architecture that lets both modes share one trust argument.

**Three design calls are made there and should not be re-litigated.** The seam goes **under the byte
source and nowhere else**: `fetch.Consumer` keeps verification, the seq floor, the walk and the
cache, and `Source` has two methods, so a live read gets the identical hash check. **An
authenticated connection proves WHO, not WHAT** — so a live read is *also* checked against the
publisher's signed root, or "live" is a downgrade in trust wearing an upgrade in freshness. And
**publishing is one act with two projections** — the site lives in the tree either way; static emit
and live serve are what you do with it.

**The implementation plan is
`docs/status/HANDOFF-2026-09-10-d-the-live-peer-implementation-plan.md`** — seven work items in
forced order with their gates, five open questions with who answers each, and the traps. W0 is the
ref-grammar fix above; the naming question blocks none of it and **nothing gets renamed here until
arch rules**.

**Suite state for this change, measured rather than inferred.** `workbench` green (14.5 s).
`shellcmd` **16 failures, and all sixteen are the E1 stranding §27 already records** — confirmed
not ours by stashing this diff and re-running three of them at `e44337b~1`, where they fail
identically. The browse and registry tests are green with the diff. Verified live across four
separate processes: pin in one, `registry ls` in the next, `open billslab.com` in a third with no
pin command in it, `registry unpin` persisting into a fourth.

**And the word "embed" names three things.** `Embed`/`embed-node` (EMBED §3/§3.1), the `::embed`
directive (SITE §3.2, whose grammar the *site* convention owns), and `app/site-asset` (SITE §4,
declared yesterday). We implemented the third, named the code after the first, and a handoff of ours
attributed the lowering MUST to EMBED §3 when it is SITE §3.2's. Our `AssetNameFromRef` admits one
ref form and refuses `://` and a leading `/` — which also refuses `entity+ref://`, `site:` and
root-absolute-within-the-site, three of the four forms `APP-CONVENTION-REFERENCE` §3.4 says MUST
resolve, while reporting a security property. Ours to fix. The naming call is routed as ask A-21.

## §29 (2026-09-10) — the social vocabulary starts here, and three findings had been fixed and never delivered

File sync is closed out (§28) and the next build is the **application-tier social vocabulary** —
the reference atom, embeds, and feeds. This section is the pivot: what was answered, what was
found on the way in, and what is being built.

**One of the six pieces is built, and the count belongs in the first sentence.** The reference atom
is done: `APP-CONVENTION-REFERENCE` §2.1's atom and §3's string form, with all **eleven** of §6.2's
required checks exercised, one test named after each. **The embed vocabulary and every feed type are
untouched — zero lines, zero tests**, so an external vocabulary census still reports this repository
at zero on feeds, and that reading is correct. What changed is that the atom the other two import now
exists, which is the piece that unblocks writing them. The atom mints no entity type; it is one shape
for *"this points at that"* that the other conventions carry inside their own types.
**The build order is forced rather than chosen:** the embed convention's child-payload arm carries a
reference, and the feed convention's entry body is an embed node, so reference → embed → feed is the
only sequence that compiles. The peer application tier had already measured the sizing trap and told
us: *the site convention imports no atoms and the feed convention imports two*, so pricing the feed
work by analogy with the site work is wrong. That warning saved the estimate.

**Where the specification is deliberately silent, we matched the other implementation instead of
deriving an answer.** The losslessness rule names no encoding for the hint list, for the anchor
fragment, or for query-parameter **order**. They shipped first, so their choices are the baseline —
not because they are better, but because whatever publishes first becomes the corpus, and two seats
each picking reasonably is how a format family diverges with nothing to catch it. Their pinned
literals are transcribed into our tests verbatim; a failure there is **routed, not locally
corrected**, because correcting it here would turn a disagreement into a divergence with our name on
it.

**One thing we will not use: the standard library's URL type.** Its query escaper spells a space as
`+`, its path escaper leaves sub-delimiters literal, and its serializer **lowercases the host** — and
the authority component here is a peer id, which is case-sensitive. The specification calls that the
single most likely implementation error, and the failure mode is why: a peer id that survives a
host-normalizing parser names a **different peer**, so it fails as a clean *not found* at a
well-formed address rather than as a parse error. The highest-value vector in the set exists for
exactly that, and the test fixture asserts it is genuinely mixed-case so a normalizing build cannot
pass it.

**Two defects found by answering other people's questions rather than by testing our own code.**

*A persisted per-window state entity was being handed to the wrong window.* The workbench guide makes
it a MUST: persist per-window state only if you can tell at startup which window each entity belongs
to, by keeping an index or by sweeping the directory before allocating an id. We did neither, and the
console's window counter starts at zero every launch — so every session's first window read back the
**previous** session's first window, including a display setting bound by ordinal. That is the guide's
own third case: *persisting, restoring, and silently restoring the wrong thing.* The sweep now lives
in the SDK rather than in the renderer, so any frontend taking the persist arm gets it, and the
seeding **refuses and logs** if a window already exists rather than moving the counter under a live
one — ordering is the whole correctness argument. It does not make the state restorable and does not
pretend to; an old bundle is orphaned rather than mis-read.

*Our reader called another implementation's public share malformed.* The share convention has two
types — a record with an audience and an audience-less publication — and we knew only the first, so a
conformant publication was rejected as *"unexpected type"* and filed as a **problem** against the
peer that sent it. A diagnosis pointing at the wrong party is the expensive direction, and they had
no way to see it from their side. The trap in the fix is the interesting part: the obvious
implementation decodes a publication into a record with an empty audience, and an empty audience
already means *authored, with no members yet* — self-only. **Those are opposites**: one is fetchable
by nobody and the other by everybody. So the two are separate types, separate structs, and the
"public" flag is read from the type and never derived from the audience length, with an explicit
assertion that the two forms are distinguishable at all so a later simplification cannot quietly
collapse them.

**And the process finding, which cost nine days.** Three conformance findings against our source
arrived on 2026-09-01. Two were fixed the same day. **None was ever delivered back**, and the other
seat's own notes recorded them as delivered on the grounds that they were on our tracker — they were
not, and never had been, because that tracker did not exist until eight days later and nobody added
them when it did. So the one file either side would consult said nothing, in both directions at once.
**A packet is not delivered by the recipient having a tracker; it is delivered when it appears on
one, and the sender cannot establish that from their own side.** Third instance in four days, third
direction.

The same shape, one layer down, is why a retraction now starts at the tracker row: the correction of
a false security claim reached five documents on the day it was found and survived as the **premise of
an open question** pointed at the specification seat, because a tracker row reads as an index entry
rather than as prose making an assertion.

## §28 (2026-09-10) — it worked on two real machines, and the first surface to read the failures was lying

An operator shared a folder between two of their own machines, in the GUI, by pressing buttons,
and the files arrived. Both directions. Then a directory. **That is the first time this product
has done the thing it is named after outside a test harness**, and everything below is from the
log of that session, because a working run is the cheapest audit available and we had never had
one to read.

**A surface built that morning to end the silence spent its first day lying.** The
delivery-failure reader filters reconnect-lifecycle markers — a known kernel defect that writes
one marker per peer-status transition and is about a session, not a file. It matched
`"/system/network"`, with a leading slash. The kernel writes the **bare** handler path
(`failed_uri=system/network`, verbatim in the log). So the filter matched nothing, and the
operator was told **"20 file transfer(s) failed"** — about twenty markers with no file in them —
followed by an invented explanation, *"the content was not there when we asked"*, about content
that was never involved.

The fixture is why nothing caught it, and the shape is worth more than the fix: **it did not
omit what production has, it added what production does not.** It seeded the peer-qualified
`entity://{peer}/system/network` because that form looked more careful. That is AP58 inverted
and it is harder to see for exactly that reason. Now matched segment-exact against both forms,
gated with the byte-exact string off the operator's log and a near-miss control arm
(`system/networking-…` must still be counted), and verified by reverting the predicate: 3
counted before, 0 after. **AP88.**

**SHARING ONE FOLDER GRANTED A READ OF THE WHOLE MACHINE, and the operator found it by asking
the right question.** They looked at `system/content:get` in the leverage review and said *"we
just need to make sure it's not having access to stuff outside the scope."* It was.
`workbench.SyncSenderGrants` carried `Resources: ["*"]` on three of its four entries and the
reconciler writes that row verbatim, so the gesture *"share this folder"* authorized the
receiving peer to read **every entity in the tree and every mounted file**. Measured across the
wire before the fix (`shellboot/share_scope_probe_test.go`, written to fail): share one folder,
read a file from an unshared one, **126 bytes back including the `content` hash** — which is
exactly what the next `system/content:get` needs. Enumerate everything, fetch anything.

**The doc comment is the finding.** It said the set was "the minimum established by
`cmd_stage3_cap_delegation_test.go`" — true of the **handler list** and false of everything
else, because that test's negative arm drops a whole handler and never narrows a resource.
Dropping a handler shows the handler is necessary; it says nothing about its cells. **A grant
has four dimensions and "minimal" is a claim about all four.** AP90.

And it was **non-conformance, not just an oversight**: `APP-CONVENTION-SHARE` §2.2 already says
*"`target` is what the grant's `resources` scope covers"*. Nothing in any suite compared our
share record's target against the grant, so they disagreed for months at full green — which is
now ask A-14, because a boundary the spec states and nothing enforces will drift in every
implementation, not just ours.

Fixed: resources derive per folder from `workbench.SharedScope`. **The first version of the fix
was wrong and the suite caught it** — it derived the folder id from the local peer, which names
an id nobody holds on any folder received and republished under `both`; both reverse-leg tests
failed, and a symmetric-names fixture would have passed. Full `shellboot` green with the narrowing in place.

**And then the operator corrected us on the part we had got wrong.** We wrote — in five places —
that `system/content:get` "cannot be scoped in any implementation" because a hash does not belong
to a folder. **That is false.** `EXTENSION-CONTENT` §6.4.2 binds each hash into the tree at
`{namespace}/{hex(H)}`, lookup is a single `tree:get` probe, and §6.4.1 makes namespace-scoped
topology a **MUST for any multi-party deployment** — get "consults the tree binding and serves
only when the hash is bound under the requested namespace". The flat behaviour we described as
inherent is the **single-trust-domain** topology, which that section says MUST NOT be the default
and which it calls **"out-of-spec and security-defective"** when run multi-party. We run it: bare
`system/content` namespace, and `system/content:ingest` is called nowhere in this tree, so no hash
is bound under any namespace.

**It is not fixable on our side alone** — core-go implements the ingest half
(`bindHashTreePresence`) and not the get half (`handleGet` is a bare store lookup with no
namespace consult), so scoping our grant would narrow which label we may claim and not which bytes
we may get. Both halves routed. Until they land, the tree grant is the operative boundary, which
is a fact about this build and not about the architecture.

**And then a second operator question moved the fix off our side almost entirely.** *"Local files
does the content chunks — and because they know the permissions of the paths, they can handle the
chunk permissioning into the content store."* Measured, that is exactly what the code says. The
`local/files` watcher runs FastCDC and puts the blob and every chunk **straight into the content
store**, without dispatching `system/content:ingest` — so the only writer of the §6.4.2 namespace
binding is unreachable for file bytes *whatever an application does*. And the domain specification
that owns file bytes pins that handler's own grant to the **bare** namespace, which is the topology
`EXTENSION-CONTENT` §6.4.1 says must not be the default for multiple parties. Two conforming
documents, in tension, and the component that could resolve it is the one holding both halves:
`local/files` is the chunker **and** the only holder of the path→grant relation, since a mount root
is already the boundary a share names. So the namespace is its to derive, not an application's to
remember. Asked as A-20 and kernel row 20. **Our own scoping work is deliberately not done yet** —
narrowing our grants while every file byte stays unbound would make the product look scoped and
delete the only signal that it is not.

**The lesson is the one this repo already has a rule for and we broke anyway:** a negative claim
about the system needs the same evidence as one about a sibling repo. We asserted an
impossibility, wrote it into source, charter, AGENTS, a review and a routed packet, and never
opened `EXTENSION-CONTENT`. A dismissal recorded as a doc comment is invisible to review forever
(AP45) — this one survived exactly one reader who knew the capability system. **A sixth copy
outlived the correction by a day: the tracker row that carries the ask to the specification seat
still had the retracted sentence as its premise.** Fixing a claim in the prose and leaving it in the
tracker is not fixing it, because a counterpart reconciles against the tracker — so a retraction
starts there.

**Two findings routed to the kernel, both about its own diagnostics.**
`ChainErrorLostData.TargetPeerID` is reserved by §3.10.6 for the peer a failed dispatch was
aimed at, and **no writer in the kernel populates it** — measured: every assignment to a field
of that name is an unrelated registry type. So a failure surface can say *a transfer failed* and
never *a transfer to that machine failed*, which is the field that decides which of two
computers the operator walks over to. And a discovery candidate's content hash embeds
`ObservedAt`, so an unchanged peer is a brand-new entity every scan: measured from the run log,
the Nearby panel re-rendered **~2.6 times a second, continuously**, against tree-view's 0.51.
*Nothing changed* and *everything changed* are byte-identical to any consumer, so no downstream
dedup can fix it. Both in
`docs/status/ROUTING-2026-09-10-b-entity-core-go-marker-peer-and-candidate-churn.md`.

**The Nearby row is a dead end in the one state that needs a verb.** A discovered peer already
in the address book renders as `known` with no control — correct labelling, and hard-won, since
`known` is an address-book fact and `Connected` would assert reachability nothing checked. But
the operator kept clicking it, because *we know this peer and cannot reach them* is exactly when
you want an action, and the action lives in a different panel. **AP89**: a row that renders state
and offers no verb is AP57 at row granularity, and it bites hardest in the failure state. Not
fixed — it is a UI decision and the notification surface is being designed as a whole.

**The leverage question, answered with numbers.**
`docs/architecture/reviews/DESIGN-REVIEW-SYNC-LEVERAGE-AND-PORTABILITY-2026-09-10.md` measures
how much of file sync is ours: **656 of 4,633 non-comment lines (14.2%) are the data path**, and
neither of those two files moves a byte — they dispatch `system/content:get` (the kernel's
§6.5.3 closure walk) and `local/files:write`. **The kernel moves the files; we decide which
files, to whom, into which directory, and we say so when it fails.** The other 85.8% is
declaration, reconciliation and diagnosis. The review also names what a Rust or Godot or
browser-tier peer must implement to interoperate — five things, of which `workbench/blob-resolve`
is a cross-impl requirement currently wearing a private name — and flags the one place we
deliberately diverge from a landed spec (`EXTENSION-REVISION` §2.3's keep-both default), which
is the most likely cross-impl divergence in the feature and is a one-sentence ruling for arch.

**Why it took as long as it did**, from the track: 74 commits since 2026-09-01, 43 on the
sharing path, and almost none of them building the mechanism. They went to things that were
built and reachable from nothing, fixtures that could not express the failure, surfaces that
were confidently wrong, and numbers read off the wrong thing. **This was an integration problem
wearing a construction problem's clothes** — every session that opened by asking *what do we
need to build* lost time, and every session that opened by grepping the kernel found it already
there.

**The atomic-set limitation is now stated where an operator reads it.** A shared folder
converges file by file with no notion of a set that must arrive together, which is the whole
reason this class of tool has always misbehaved over a live `.git` directory, a SQLite database
with its `-wal` sidecar, or a VM image. `USAGE-SHARE-A-FOLDER.md` now says so plainly, says what
to do instead, and is explicit that this is a property of the design rather than a defect we
have yet to find — we have measured that nothing prevents it, not the failure itself. Designing
a set boundary remains open and unstarted.

**And that same edit removed a sentence that had stopped being true** (AP80, again): the guide
still told operators that `direction … both` was unsupported and to run the share twice, the day
after two-way on one folder record landed. Nothing in the tree can fail on prose — the only
defence is to grep the published claim when the behaviour changes.

## §27 (2026-09-10) — sharing was dead all morning, and the directory names never had to match

An operator came back from a real two-machine session with a folder that would
not sync two ways, and the session that answered them spent its first hour on
the wrong defect. What follows is in the order it was measured, because the
order is the finding.

**The reported problem was not the blocking one.** The known limitation was
that a two-way folder's reverse leg subscribes to *our* root name on a peer
whose files sit under *their* root name — accepted, healthy, permanently
empty. Real, reproduced, fixed below. But the first live gate run said
`make twopeer-sync` — 2 peers, real TCP, real grants — was **35 checks, 18
failed, with no file crossing in either direction**. Nothing was syncing at
all, for anyone, in any configuration.

**The cause was a capability dimension that defaults to something.** The
kernel had, that morning, made the executing handler's own grant the gate on
outbound sub-dispatch. §5.2's fourth dimension — *peers* — defaults an absent
scope to *the local peer* and still checks it. So `blob-resolve`'s manifest,
which had never declared one because nothing used to consult a handler's
internal scope on an outbound dispatch, became a handler that could fetch only
from itself. `Peers: nil` reads as *not narrowed* and means *narrowed to the
smallest thing*. It failed under the development wildcard too, which is how it
was found before anything about our own manifests was suspected: a fixture
named "open access" had silently stopped covering a dimension.

**Fixing it once was not enough, twice.** A backfill runs the handler under
the peer's own installed grant; a subscription delivery runs it under the
subscription's dispatch capability. Repairing the first made `resync` work
while live delivery still refused — a symptom that reads as a subscription
fault and sends you to the wrong half of the system. Three probes went inside
the handler before anyone asked what was different about the *caller*, and the
discriminating measurement had been available from the first run.

**And handler grants are install-once, so the fix reached no existing
machine.** The kernel skips minting for a pattern already bound — correct, and
its own comment says a change reaches new peers only. What no comment can say
is that every cross-peer test here runs on a memory store, which always mints
fresh: the manifest fix was green across the whole tree and inert on every
machine an operator runs. That needed a migration, idempotent by content
because the mint embeds a timestamp, gated across a process boundary, with a
control arm that installs the old shape and asserts the failure — without
which the migration test passes against a build where the changed field does
nothing.

**Then the reported bug, which turned out to be smaller than its write-up.**
The reverse leg's root-name problem had been sitting in the source as a
comment that wrote out its own fix — *a dispatched remote read of their folder
record would work; they already grant us the read* — and did not do it. It
does work, and the authority is present exactly when the question is worth
asking, because the reverse leg only exists when the receiver publishes and a
publishing peer has already granted that read. So the owner **looks** instead
of waiting to be told. No new channel, no new field in a shared namespace, and
the same read answers what the other side's state is — the fact a
four-day-old entry in our own agent guidance had called structurally
unavailable. A failed read now **refuses** rather than falling back to our own
root name: the binding a guess creates is durable and silent, and not creating
one is recoverable thirty seconds later.

**The gate that was passing on this was asserting the wrong thing.** It
declared two-way on one side only and checked that a sync binding appeared. It
did appear, and it could not have carried a byte — a receive-only peer
publishes nothing and grants no sender authority, so the subscribe answers
403. A gate on *the existence of a mechanism*, on a case where the mechanism
cannot work. Rewritten to require both sides to declare it, keyed on the
receiver's root, with a new control arm asserting a receive-only counterpart
gets no binding **and a reason the operator can act on**.

**One more, found by running the flow.** `direction` — the verb the whole
two-way feature depends on — rewrote the authorization and then neither
reconnected nor reconciled, while its own doc comment said the reconciler had
already forced the reconnect. Grants are assembled at handshake, so the new
authority was inert, and no pass ran, so no leg was built: two correct
declarations on two machines and nothing happened. `share` and `accept` had
both steps from the start. A false sentence in a doc comment is worse than no
sentence, because it is the one the next reader checks the behaviour against.

**After:** `twopeer-sync` 35 checks, 1 failed — the documented
first-change-after-restart delay, which is a latency and not a loss.
`threepeer-sync` 27 checks, 0 failed. Two peers with differently named
directories now sync in both directions, measured on bytes on disk, with the
same-name case kept as the control that isolates the name as the variable.

**`shellcmd` was NOT green, and this paragraph said it was.** The claim came
from `go test … | tail`, which reports **`tail`'s** exit code — zero however
the run ended. Re-run to a file, `-race`, full package: **17 failures.**
Sixteen are the kernel change below and are not ours. The seventeenth was, and
is fixed at `42fdafc`: the discovery fixture seeded one mDNS candidate and
asserted the substrate reported exactly one, while the peer it builds runs a
live mDNS browser that also collects this package's own E2E peers and, on a
developer's LAN, their other machine. It passed alone and failed in the full
package — which reads as a substrate regression and is not one. Its own doc
comment claimed the tests needed "no multicast, no timing and no network";
that was false, and it is the second false doc comment in this section.
**Now 16, all accounted for.** Never read a pass/fail from a command that ends
in a pipe; `make test-each` exists so the correct thing is also the easy one.

**Still broken and deliberately not worked around here:** the same kernel
change strands the revision pull and the tree-follow fetch-diff — **20 tests
across two suites** (4 in `entitysdk`, 16 in `shellcmd`), every one
`failed_uri=system/revision status=502`, confirmed pre-existing by stashing our
own fix and re-running. The first count of this was four, taken before the
suite that holds the other sixteen had been read correctly. That handler
declares no internal scope, so it takes the kernel's default self-grant, which
omits the peers dimension on purpose while the operation's whole job is to
reach another peer. Both cannot be right. `system/revision` is the propagation
path for `archives/*`, so cross-peer deletes, bidirectional convergence and the
mirror chain are all down for that reason. Routed rather than shimmed, because
a local workaround would hide a question that is probably cohort-wide.

**The gate for the binary an operator opens is green: `make twopeer-gui`, 74
checks · 0 failed · 0 known-open**, all 13 phases, nothing skipped. Two real
Avalonia apps in two containers, one real TCP network, the address typed into
the panel, the share and the accept as real clicks, every file assertion on
bytes on disk at the receiving end: connect both ways, share, accept into a
directory the receiver chooses, backfill of the files already in the folder,
add/modify/delete, restart both peers, restart the receiver alone.

Two things in that run are worth carrying rather than celebrating. **Phase 12's
known defect did not reproduce** — the first change after a receiver-only
restart arrived — and the harness said so out loud (`the known defect did NOT
reproduce; re-check the waiver`), which is what a waiver scoped to an instance
is for. **One non-reproduction does not retire it**; the intermittency is the
documented state. Phase 13 measured the waived change converging on its own
~90 s after it was written, no resync, no operator action, which is the same
delay-not-loss finding and is now measured twice.

**The Avalonia headless suite had been aborting mid-run, and now completes:
213/213, exit 0.** It previously printed `Passed: 160 … Total tests: Unknown …
Aborted` — so an uncounted number of tests at the tail had never been running,
and the pass count on its own hid it. `PeerView` registered a wake, discarded
the registration id, and freed the delegate on close without unregistering; Go
kept calling into collected memory, which the runtime aborts uncatchably. The
audit outward from there found twelve more of the same class — ten wake pumps
written by hand ten times, five of which never waited for their goroutine at
all — now one pump whose `stop` blocks until the goroutine is gone. **Read the
suite's `Total tests` line, not its pass count:** `Aborted` with an unknown
total is a truncated run wearing a green number.

**What this cost, stated plainly:** the operator lost a morning, and then an
hour of the session that was supposed to help. Three of the five defects above
were written down somewhere in this repo before they bit — as a known
limitation, as a doc comment, and as a passing test. None of them was written
down anywhere that could fail.

## §26 (2026-09-09) — the site bytes and the trie root already agree, and the claim that said so had never been run

Fourth pass of the day, scoping the cross-implementation site gate the content-site convention
makes a precondition of ratification. **The gate turned out to be two-thirds already passing, and
the way we found out is that nothing had ever measured it.**

### 1. The claim was in a header comment, and the test beside it measured the wrong thing

`entitysdk/site.go` has said since the types landed that we *"hold to byte-equivalence with
[the other application tier's] `to_entity` output"*. It reads as settled. It had never been run.

The nearest test, `TestSiteManifest_DeterministicEncoding`, **encodes the same value twice and
compares** — self-consistency, green for any encoder that is merely stable, and green if the other
implementation did not exist. The two files that *do* carry another impl's vectors verbatim
(`site_link_crossimpl_test.go`, `site_asset_crossimpl_test.go`) cover link classification and asset
refs. So the one shape the convention turns into a ratification gate was the one shape nothing
measured, and a confident sentence in a doc comment is what kept anyone from noticing. **AP83** —
AP45's *"a dismissal recorded as a doc comment is invisible to review forever"* with the sign
flipped. The tell is a test whose **name** states a cross-impl property and whose **body** names
only our own types.

### 2. What the measurement says — two of three links agree

We have held another implementation's frozen static emission at `fetch/testdata/crossimpl-rust-site/`
since August. Pointing our own types and our own trie builder at it decomposes the reproducible-publish
gate into three links:

| link | state |
|---|---|
| source fixture (markdown + frontmatter) → site entity | **unmeasured — the whole remaining risk; no fixture exists on either side** |
| site entity → canonical bytes | **agrees, byte-identical** — 3 `app/site-manifest` + 11 `app/site-page`, including 5-entry nested nav with a section header and a non-ASCII title |
| binding set → CHAMP site root | **agrees** — our reader walked their signed root, collected 15 bindings (exactly their 15 leaves) and rebuilt it in a **fresh** content store |

Gates: `fetch/site_entity_crossimpl_test.go`. Both were checked against a deliberate break — perturb
one re-encoded byte, drop one binding — before being trusted.

**The round trip is its own anti-vacuity arm**, which is why it is the right shape: `ecf.Decode` into
a Go struct silently drops undeclared fields, so a dropped field cannot survive the re-encode. It
comes back as a byte difference, loudly.

### 3. Three facts the joint gate has to be designed around, none of which were written down

- **The site root is independent of the publishing peer** — two identities, one root. It holds
  because `BuildTrieForPrefix` trims the qualified prefix, so the trie keys are relative and the
  peer-id never enters the hash. **Without this the gate is impossible as specified**, and in a tree
  that is peer-id-namespaced everywhere the opposite is what you would assume until you measure it.
  `publish/site_root_scope_test.go`.
- **The comparison must be scoped to the site subtree, never the peer subtree.** Our sites sit under
  `content/sites/{id}/` and the other impl's under `sites/{id}/`; at peer scope the same content sits
  under different keys, the roots cannot agree, and the red says nothing about conformance.
- **`site_id` enters the root**, via the manifest body — so a shared fixture has to pin it, or two
  publishers diverge on a field neither thinks of as content.

### 4. What the gate does not exercise, and it is the clause most likely to be assumed green

**No chunking, at all.** The only asset in the fixture is a **732-byte inline tree entity** — three
orders of magnitude under the 16 KiB inline threshold. So the convention's chunker-agreement caveat
and its canonical `chunk_size` MUST are untouched by every artifact either seat holds. A
markdown-only fixture keeps it that way and would report green on a clause it never ran. Any shared
fixture needs one asset above the threshold.

Related, and a false alarm we nearly filed: the spec's *"min/avg/max = 256 KiB / 1 MiB / 2 MiB"*
**is** what the shipped chunker does — `DeriveFastCDC` sets `min = target/4`, `max = target*2` from a
1 MiB target. The trap is that `core/types` also defines a `MinChunkSize` of 64 KiB, which is the
inline-include threshold and **not** the FastCDC minimum. Reading that constant as the chunker's min
produces a confident divergence report against a spec that is correct.

### 5. The transferable rule

**Before designing a joint rig with another seat, ask what the frozen fixture you already hold
answers alone.** The question that had been scoped as needing a live two-peer run — *can two
independent publishers produce a byte-identical root at all?* — was answerable offline, from bytes
that had been in this tree since August, and two of its three links came back green in an afternoon.
That is D20 pointed sideways: at a counterpart's artifacts rather than down at the kernel.

**Owed next, and it is the one real build:** a source-**directory** ingest. `entity-seed-site`
constructs entities in Go rather than reading `.md` files, so the unmeasured link is the piece we
have to write — and it waits on an agreed fixture by design, because building our own layout first is
how two seats end up comparing two different things.

## §25 (2026-09-09) — the reply pass, and the listener we had already built

Third pass of the day. The specification seat replied twice, both replies hold, and one of them
corrected us correctly. Checking that correction turned up two more of our own — and the third is
the expensive one.

### 1. Their rulings hold, and they found a defect in their own delta after both seats cleared it

The public-share type is ruled and it is our second-ranked name. They verified our usage at source
rather than taking our word, then read the other application seat and found **it ships the same
word for the opposite case** — so the finding is not *"the noun is taken by us"* but *"the noun is
already ambiguous across both seats on the exact axis the new type exists to separate."* That is
what a two-seat convergence is supposed to produce, and neither of us had it alone.

Then, after both seats had confirmed their claims and found nothing wrong, they opened their own
routed schema and found a defect in it: the field they had narrowed would have expressed a single
blob where the landed convention expresses a blob *or a subtree* — which would have unblocked one
of the other seat's cases and immediately re-blocked the next one. **Their rule out of it is worth
taking whole: a peer's confirmation of your claim is not a review of your delta.** We confirm
claims constantly and review deltas almost never.

### 2. Three unmeasured claims of ours in one day

Ranking naming alternatives, we wrote that our first choice *"collides with nothing in either
tree."* It is a **core protocol return type**, with hundreds of occurrences in one sibling and
dozens in ours. The failure is not a mis-scoped search — **there was no search.** A measured claim
(our own usage, with file, symbol and type tag) sat in the same paragraph and the same voice as an
assertion nobody had run, and a reader cannot tell them apart.

They caught it by re-measuring a claim *on moving it*, which is the rule worth keeping: the party
that moves a claim owns re-checking it, however short the move.

### 3. And the one that cost the most: we already had the thing we said we needed to build

This morning we wrote, into our own index of what the other application seat is waiting for, that
the blocker between here and the first browser-to-workbench peer-to-peer gate was *a WebSocket
listener we would have to add*.

**It has been shipped, wired and gated in this repo for months** — the listener, the address form,
the advertised transport profile, the discovery announcement, and a green test across all of it.
The remaining item on their list is a browser-side affordance that is not on the path to a first
run, because in that topology **they dial us**.

So the joint gate needs no build here at all. It is a launch flag.

This is the sixth time this discipline has paid out — *price work against the substrate, not
against your own assumptions* — and **the first time it has been scored against ourselves**. The
capability was not sitting unadopted in a dependency; it was sitting finished in our own tree, and
we wrote a build estimate over it. **A negative claim about your own tree needs the same evidence
as one about somebody else's**: *"we don't have X"* feels like recall and is a search.

### 4. Where the board stands

Every ask filed to the specification seat this cycle has been answered; eight remain open there,
none blocked on us. The substrate seat's thirteen are unchanged and none has been handed over yet —
that channel is quiet because nothing has left, not because nothing came back. **Nothing anywhere
is blocked on us**, which is the first time that has been true and measurable here.

### 5. Gates

WebSocket and registry-differential suites green; `go vet` and the textual sweep clean. No shipped
behaviour changed.

---

## §24 (2026-09-09) — the specification seat answered, and one of our asks was already law

Same day, second pass. The spec seat read our tracker and replied; this is what their answers
change here.

### 1. A-1 was ruled three weeks ago and we could not see it

Our first-rated ask — which of two readings of a registry binding's transport field is normative —
**was ruled on 2026-08-21, folded as a MUST, and our reading is the normative one.** The ruling
landed in a spec revision between our filing and their reply, and nothing in either tree pointed at
it.

**We confirmed it one step past their source read.** They said *"if you can still reproduce it,
that is a new finding — but against current trees we cannot."* Current trees are not the place that
settles it: a conformant emitter does not retroactively change bindings already published. Our
live-federation gate does settle it, and the answer is no — the chain now prints `transport carried
hash in the binding` where it printed *"carried inline"*, on a binding reissued three days after the
ruling. Five names walked out of a signed root, five resolved in full, one followed through to
verified page bytes under a different key.

The reason a one-command check could answer this at all is that our consumer reads both shapes and
**keeps them distinguishable** rather than normalising them away. A resolver that quietly accepted
either would have had nothing to print.

**Their framing of the failure is better than ours would have been:** *"the pattern is not that you
should grep harder; a ruling that lands in a spec is invisible to the seat that asked for it unless
someone says so."*

### 2. The gate we built for it can never retire itself

Our differential test pins the old shape against a **frozen** fixture, and its own exit condition is
*"if the kernel resolves these, delete this test."* Those bytes carry the superseded form forever,
so the condition is **unreachable by construction** — the test passes in perpetuity as a monument to
a defect that no longer exists, while the full agreement assertions it disabled, which are the only
place two independent resolvers are required to agree on one set of bytes, stay switched off.

Re-cutting the fixture rather than deleting the test, so the harness is never left with no coverage.
**A self-retiring test whose retirement depends on data the test itself owns can never retire** —
the exit condition has to be checkable against something that moves.

### 3. A noun collision worth catching before it lands

The convention is gaining a type for a *public* share — audience-less, grant-less, pull-only — and
the proposed name is `offer`. **In shipped product language here, `offer` already means the opposite
case**: a share naming specific peers, with a derived permission, pending until accepted. One word,
two meanings, differing on the exact axis the new type exists to separate.

Filed while the proposal is open, because they asked the sibling seat for naming evidence and we are
the second datapoint. If the name stays, we rename our surface language — the point is that the
collision be decided rather than discovered.

### 4. We are a fourth consumer of a loop being promoted to substrate

They ruled that a follow loop's cursor is substrate, on evidence of three shipped consumers that all
fetch a foreign subtree through one chokepoint with no content cursor — noting that what differs
across them is the cursor and *the retry policy*.

Our catch-up supervisor is a fourth, and it is the one that has had to design the retry policy under
load. Offered as input, not as a position: the resting interval derived from what a pass costs
rather than from a constant; recovery asymmetric on purpose; the ramp a gradient rather than a snap;
and the wait never shorter than the pass that produced it. Plus the finding that motivates all of
it — **a doubling back-off makes a latency look like a loss**, which is how "the first change after
a restart is lost" was published in six documents and believed for five days about a delay.

### 5. Two connectivity asks confirmed genuine, and infrastructure moves their priority

Both scope questions we posed against a folded proposal came back *"not covered, not re-asks, both
want rulings"*, to be taken as one conversation. Meanwhile a rendezvous deployment is being stood up
elsewhere in the ecosystem — and where a rendezvous node is reachable, the symmetric-pair mechanism
fires with **no ruling needed** and the shipped adapter is ours to adopt. **The asks survive for the
case that deployment cannot reach**, which is this product's default: two laptops on a home LAN with
no reachable node, possibly no internet at all.

### 6. Gates

Registry differential suite green; `go vet`, `gofmt` and the textual sweep clean; the live-federation
chain green end to end (5 names, one followed to verified bytes). No shipped behaviour changed.

---

## §23 (2026-09-09) — auditing our own finding, and the outbound work gets an index

No feature work. An audit of §22 before any of it left the tree, and the consolidation that
audit made unavoidable.

### 1. The marker storm names the wrong operation, and the marker's own body says so

§22.7 and the routed packet both describe the restored lifecycle dispatching
`system/network:reconnect` into an empty session map. **It cannot be that operation**, and the
correction is worth more than the error.

The marker is bound by the continuation handler's forward-dispatch path, which binds **only when
the continuation carries no `on_error`**, and which sets `reason` to the failed operation's `code`
verbatim. Our markers all read `reason=not_found`. The `reconnect` continuation *has* an
`on_error` — added by core-go specifically to stop this family, and it works. The backoff
continuation has none, but its operation answers `502 connection_failed`. Exactly one continuation
in the graph has no `on_error` **and** can answer `404 not_found`: the one that dispatches
**`restore-subscriptions`**.

**So the trigger is the far peer RECONNECTING TO US, not going away.** Both lifecycle
subscriptions fire on `created`/`updated` with no filter on the status value, so a peer we cannot
dial but which keeps successfully dialling *us* writes one marker per success — which is why the
operator's cadence matched their machine's retry interval rather than anything on ours.

The root cause is unchanged and still measured: a session map that dies with the process while
the graph that dispatches into it is tree-resident. Only the op name and the trigger sentence
were wrong. Both are corrected in the packet, in `AGENTS.md`, and here.

**Two probe gaps closed at the same time.** The restart arm printed the marker's shape and
asserted nothing while three documents quoted that shape as measured — it asserts it now, and
the assertion is also the discriminator for *which* 404 site fired. And the collector arm skips
when the restart arm does not reproduce, which silently retires the only measurement behind
*"bounded at one retention window"*; the skip now says so in as many words.

### 2. Twenty-four packets, indexed by `ls`

Counting what is actually outstanding: **thirteen** asks to the kernel seat, **eleven** to the
spec seat, **three** open in each direction with the other application-tier seat. One index
existed, it covered one seat, and **four routed packets were missing from it** — including one
that another team had opened a row offering to carry for us, on the belief that it had never been
written.

There are now three, one per counterpart, and the split is the relationship rather than the
topic: the substrate seat (defects in an implementation of a decided thing), the spec seat
(anything whose answer is a sentence in a document), the peer application seat (shared shapes and
interop, where neither side can rule anything). Each row carries its ask, its packet, and an
explicit delivery state that **defaults to not established**.

**Same day, the specification seat published a cleanup standard for every seat, and the shape it
asks for is the one we had just built.** One tracker per counterpart at a predictable path, four
named sections, stable ids never renumbered, an ask stated as one sentence rather than a summary,
and a real section for documents that are filed with nothing owed back — because treating a
for-information review as an open ask is how one seat's notes were read as a 44-item inbox when the
true number was eleven. We adopted it the same day: the trackers moved to
`docs/status/TRACKER-<counterpart-repo>.md`, ids unchanged, and new outbound packets follow the
standard `ROUTING-<date>-<letter>-<recipient>-<slug>` shape with a three-line addressee block.
Existing packets stay where they are. Two of our conventions went the other way and are now in the
ecosystem-wide instructions, *archived is not delivered* among them.

**They also caught a date we had wrong for three weeks.** Our correction packet said the publisher
fix landed 2026-08-18; the commit is `0a7423d`, **2026-08-25**. The write-up was drafted against a
working tree and committed a week later, so *"an artifact that exists only in your working tree
does not exist"* had failed in the direction that leaves no trace — the artifact did land, so
nothing ever came back to flag the gap, and three documents inherited the drafting date. **Date a
capability by the commit that carries it.**

**The rule that was missing has a name now: archiving is not delivering.** We fixed a publisher
defect on 2026-08-18, wrote the result up, filed it in the archive as done — and never sent it.
Three weeks later another team's board still carried it as an open blocker against us, correctly,
because the last evidence they had was a source read from hours before the fix. Closing a packet
against our own completion rather than against the other side's receipt is how a fixed thing stays
broken in everybody else's model of us. Archive on delivery plus reply.

### 3. One of our own asks was re-asking a decided question

The bilateral-connection packet from §22.5 named two gates and asked about both. Checking before
routing: both are the subject of a proposal that was **folded five weeks ago**, and the fold says
in as many words that the one-directional mint is correct for dial-by-address and that the
mechanism is deliberately not broadened to it. One of the two asks is genuinely narrower than what
was ruled — two peers that each hold a durable declaration naming the other and have each dialled
is not *"one party requested service"* — and it survives, cited against the ruling. The other was
re-aimed: it is a scope question for the spec seat, not a defect report for the kernel seat.

Also missed on the first pass: the mechanism that would make our topology symmetric today is
**shipped in the kernel and unused by us**, and the spec seat's own board already carried that as
an item about us. It needs a rendezvous node, so it is not free — but it is a product decision
here rather than a gap anywhere else, and *"why do we need two one-way connections?"* has a real
answer: because we dial by address, and meeting at a rendezvous key is what makes a pair
symmetric.

**The generalisable half:** a search of a dependency's *implementation* is not a search of the
*specifications*, and those live in different repositories. D20 has paid out five times against
the kernel and once against a spec; this is the first time it has paid out against a **ruling**.

### 4. Gates

`shellboot` restart + collector arms re-run green (33 s) with the new shape assertion live;
`go vet` clean; the two pre-existing gofmt drifts are untouched and still owed a `make fmt` commit
of their own.

---

## §22 (2026-09-09) — the discovery join was dead, and the connection is bilateral

Fixes for §21's three defects. The cause of the first turned out to be **worse than §21 said**,
and the operator's architecture question turned out to have a real answer.

### 1. The cause was not precedence — discovery could never identify a peer at all

§21 diagnosed a precedence bug: the stored address consulted before discovery. That is true and
it is not the cause. The cause is one field.

`CandidateData.PeerID` is **empty on every mDNS candidate that exists** — EXTENSION-DISCOVERY
§2.1 makes it null until IDENTIFY, core-go's `candidateFromServiceEntry` never sets it, and the
only writer of the populated form (`discovery.Handler.PromoteSuccessor`) has **zero callers in
either tree**. The claimed peer-id travels in the `peer_id_hint` TXT key and nowhere else.

Three consumers here joined on that field:

| | |
|---|---|
| `refreshDeviceAddresses` (reconciler) | matched nothing, every pass, every peer |
| `dialableAddressFor` (discovery arm) | matched nothing |
| `Peers()` (the `peers` verb) | **had never listed a single discovered peer** |

So discovery was not out-ranked. **It was inert at every precedence.** It survived because a
fourth consumer — the Nearby panel — reads the TXT hint directly and works, so the one surface
anybody checked showed discovery in perfect health while every consumer of it did nothing.

Fixed: `entitysdk.CandidatePeerID` is now the single reader, preferring the substrate's field and
falling back to the hint, so it becomes a no-op the day the ceremony is wired. Trusting the hint
is correct for **this one decision only** — where to dial a peer we have already declared, whose
identity the dial then authenticates — so a false announcement costs a failed handshake and never
a wrong peer. Gated with an anti-vacuity arm asserting the field really is still empty, without
which the whole file passes the day core-go changes.

**The transferable part: a join on a never-populated field does nothing forever and reads as a
stricter check.** Two readers of one announcement, using different keys, and only one of the keys
is ever filled in. Catalogued as AP82.

### 2. Discovery now outranks the declaration, and the ladder is what makes that safe

`dialLadderFor` puts the **announced** address first and keeps the stored one behind it;
`ensureOutboundRoute` tries every address rather than the preferred one and writes back whichever
answered. Discovery first because a candidate is at most 15 s old by construction — it is an
observation, where a stored address is a hypothesis. The fallback is not politeness: it is what
makes trusting an unauthenticated mDNS claim safe, since a spoofed announcement then costs one
failed handshake and we fall through, and it is what keeps a sleeping peer or a
multicast-less network working. Dropping it turns "discovery first" into "discovery only".

### 3. Direction is on the reading now, and it fires for RECEIVE-only folders too

`DeviceStatus.OutboundRoute` carries what the pool cannot express. The shell prints
`inbound only`; the Sharing Status panel says *"they can reach us — we have no connection to
them"* in Goldenrod, never DarkSeaGreen; `directionProblem` is the **one writer** of the
sentence, shared by the pass and the read so the two surfaces cannot drift.

**The first draft got this wrong, and so had an existing test.** It stayed silent for a folder we
only receive, reasoning that receiving needs their dial rather than ours —
`TestReconcile_SaysNothingAboutDialingAPeerWeOnlyRECEIVEFrom` asserted exactly that, with the
premise written out. Refuted by our own measurements: AP63 (*"a sync is MUTUAL — the receiver
dispatches in to subscribe and fetch"*) and `walkRemoteFiles`, which lists
`/{remotePeerID}/{prefix}` — a peer-qualified path is a remote read (AP11), so the receiver
dispatches to the sender to enumerate and again to pull the closure. An unreachable peer breaks an
incoming folder just as completely; it merely presents as *"nothing is arriving"* instead of
*"nothing is being sent"*, and suppressing it is why *"my folder just stopped updating"* had no
surface. The consequence sentence differs by direction because those two symptoms send an
operator to opposite machines. The test now asserts the corrected behaviour with its old premise
kept visible.

### 4. The reconciler's diagnosis reaches the Sync panel — it was being dropped at the boundary

§21 blamed placement: the diagnosis went to stderr. There was a second, independent reason it
could not arrive. **`SyncPanel`'s own `StatusView` never declared `problems`**, so
`System.Text.Json` dropped it in silence at the bridge boundary (AP49, the same failure as the
browser's provenance fields). The panel that owns the flow could not have shown the answer even
if it had wanted to. Now declared, rendered in a bounded "Needs attention" section above the two
gestures, and hidden when empty — a permanently-present warning heading is chrome, and chrome is
what an operator learns to skip.

### 5. The operator's architecture question has an answer: the connection IS bilateral

*"If I have a connection established with the peer, why do we open another one-way?"*

The protocol already agrees with them. The kernel has **§6.11 reentry**
(`registerInboundForReentry` caches every accepted connection by peer-id so a handler can
originate back over it) and **§6.5(b) reciprocal grant**, whose own comment reads *"that single
reciprocal grant is what makes the pair symmetric"*. It is not an oversight; it is built.

**Both halves are gated on predicates a LAN peering never satisfies.** Reentry is consulted only
when transport-profile resolution **errors** — so a profile resolving to a *dead* address dials
the corpse forever while the peer's live inbound connection sits one map lookup away, which is
exactly the operator's 45 minutes. The reciprocal grant is sent only on a rendezvous
establishment, and two laptops dial by address, so it is never sent in either direction and each
peer must independently dial the other.

Routed as `reviews/CONNECTION-DIRECTION-AND-BILATERAL-REACH-2026-09-09.md` with three asks, the
smallest and most useful being an accessor for a connection's **direction** — core-go's
`Connections()` concatenates inbound and outbound and tags neither, which is why our SDK's
`PeerInfo.Direction` has been a permanently-empty field since it was written, and why our
workaround (tracking our own dials per process) is honest but narrow.

### 6. Gates

`test-each` **10/10** · Avalonia headless **207/207** (was 203) · `twopeer-gui` **74 · 0 failed ·
0 known-open** · `reachability`, `lint`, `textual` clean.

**The `0 known-open` is not a fix.** The waived first-change-after-restart check passed in both
restart phases and the harness said so itself — that defect is known-intermittent, and nothing
here touches delivery after a restart (the harness types the address, so the new dial ladder has
exactly one rung). The waiver stays. PHASE 13 measured convergence at **~90 s** against ~107 s on
2026-09-08; both are consistent with the supervisor's doubling interval and neither is a bound.

### 7. The chain-error growth (§21.5) — root cause measured, and it is not what it looked like

§21 recorded 188 markers in 45 minutes as **established**, and *why* `system/network` answered 404
as **not established**, with an instruction not to route it until somebody instrumented it. That
is now done — `shellboot/chain_error_growth_probe_test.go`, four arms.

**It is real tree growth, not log noise** (each marker is a `store.Put` plus an index bind), and
**it is bounded at one retention window** — ~6,000 entities at the observed rate, not the
unbounded growth §21 implied.

**The cause: the reconnect lifecycle has two halves with different lifetimes.**

| | where | survives a restart |
|---|---|---|
| continuation graph, lifecycle subscriptions | tree | **yes** |
| the maintain **session** | a map on the handler | **no** |

`maintain-peer` reads that map to decide whether a relationship already exists. After a restart it
says no — for a relationship whose graph is sitting in the tree — so it drops the session and
returns 502 instead of taking the arm-the-retry branch core-go added *specifically to kill this
marker family*. Every later peer-status transition then dispatches into an empty map, gets
**404 not_found**, and a permanent marker is written.

> **Corrected by §23.1:** the operation is `restore-subscriptions`, not `reconnect` as this
> section first said, and the trigger is the far peer **reconnecting to us** rather than going
> away. The root cause below is unchanged.

**The two negative arms are the load-bearing ones.** A peer that was never reachable produces
**zero** markers, and so does establish-then-close inside one process. Both obvious explanations —
*"the other machine was off"*, *"the connection dropped"* — are refuted. The trigger is our own
**process boundary**, which is why no test in either tree has ever seen it: every arm that runs in
one process measures zero.

**And the collector genuinely collects** — arm 4 measures 1 → 0. Checked deliberately, because
`HistoryConfigData.MaxDepth` looks identical and prunes nothing; a second no-op knob would have
inverted the conclusion from "noisy" to "disk-filling".

Routed as `reviews/CORE-GO-MAINTAIN-SESSION-DIES-AND-THE-GRAPH-DOES-NOT-2026-09-09.md`. **No local
mitigation shipped, on purpose**: a retention override would discard real chain forensics to mask
someone else's bug, and the trigger is a peer we cannot reach — which §22.2's dial ladder is the
actual fix for.

This is the **fourth** instance of one shape here — a durable structure whose dependency is derived
runtime state nothing rebuilds at open (the query index, the subscription path index, the mount
binding, now this). The first three were read paths that silently returned nothing; this one is
worse in kind, because something keeps *writing* on every miss.

### 8. Still open from §21

The "start over" affordance (§21.6) is untouched.

---

## §21 (2026-09-08) — the first real two-machine run, and the app was right the whole time

Two machines, one LAN, no harness. It found more than the previous three sessions of gate work,
every gate stayed green throughout, and none of it is exotic.

**What happened:** a file created on one machine arrived; an edit made on the other never did;
the panel said **connected** the whole time. The operator's reading was *"it's one way"*, which
is reasonable and wrong.

### 1. The cause was a refused TCP connection, and the app said so at startup

```
warning: could not open our own connection to 192.168.68.160:9000
  (connection refused) — until it succeeds, anything we write to a
  shared folder will not reach them
```

Zero occurrences of "connected" or "established" in the whole 4,700-line log. Inbound worked —
they dial us, we listen, their file lands. Outbound was dead, so nothing we wrote ever left.
Not `Mode: both`, not stale tree state, and nothing to do with the change being an edit.

**The product diagnosed itself correctly, in plain language, and three surfaces stopped anyone
reading it.** That is the finding.

### 2. Discovery cannot correct a stale address — and that is the whole point of discovery

> **Superseded by §22.1 — the diagnosis below is right about the symptom and WRONG about the
> cause.** The precedence order described here is real, and fixing it alone would have changed
> nothing: discovery could not identify a peer at *any* precedence, because all three consumers
> read a field that is never populated. Kept so the error is legible.

The operator asked the right question: *"why aren't they transferring the port? Isn't that what
discovery is supposed to do?"*

`dialableAddressFor` consults, in order: this session's connections, **the stored declaration**,
then discovery. So once an address is recorded, a live announcement can never override it — we
dial a months-old address forever while the peer announces its real one on the LAN. The reconcile
path is stricter still: it reads the stored declaration **only**, with discovery nowhere in it.

The intent is sound — a stored address is the only source that survives a restart. **The error is
treating "durable" as "authoritative". A remembered address is a HYPOTHESIS about where a peer
is; an announcement is an observation, and when the hypothesis has just been refused the
observation should win.** Worth reaching for anywhere a cached fact outranks a live one.

### 3. "Connected" is rendered for a peer we cannot dispatch to

> **Fixed — see §22.3.** The diagnosis below is accurate; `DeviceStatus.OutboundRoute` now
> carries the direction and no surface renders a peer we cannot reach as plain "connected".

`st.Connected` comes from the connection **pool**, and `AGENTS.md` already warns in our own words
that a pooled session *"says connected over a route we cannot dispatch on"*. An inbound-only
session — they dialled us, we never dialled them — renders as plain connected. It is the exact
state where sharing is half-broken, and the most likely one, since a dial-by-address authorizes
only the dialer.

**The panel and the log contradicted each other in the same session, and the reassuring one was
on screen.** Direction has to be on the reading, the way inbound authority already is.

### 4. Fixed: 95% of the log was four render breadcrumbs

`peer-connections: NearbyRender h=1` alone was 2,383 lines, one every 1.2 s, carrying a handle
that never changes; with three siblings, 4,489 of 4,700. The bounded crash ring was being evicted
by it too, which every crash investigation here has depended on.

**The obvious fix was written, measured against the operator's actual file, and deleted:**
collapsing consecutive duplicates saves 14%, because the four offenders interleave and nothing
repeats back to back. What shipped is per-key suppression on a power-of-two ladder — 4,700 → 251,
no clock, and a rare event is never suppressed. AP43 applies to fixes exactly as it does to
refutations: a change that does not reproduce the failing shape fixes nothing.

### 5. A peer being switched off grows your tree forever

188 chain-error markers in 45 minutes, one every ~15 s, coincident with the refused dial —
~5,700 overnight, ~40,000 for a laptop shut for a week. Established. **Why they carry
`failed_uri=system/network status=404` is NOT established**, and it is not being routed until
somebody instruments it.

### 6. What an operator cannot do: start over

"Clear this peer and start fresh" has no answer that does not require a mental model of the tree,
and the easiest-to-reach form (`rm -rf ~/.entity`) changes the peer-id and invalidates every
grant on every other machine. That is a missing feature, not a documentation gap.

---

## §20 (2026-09-08) — the audit before live testing, and the reason `Mode: both` never worked

No feature work. An audit ahead of putting the app on real machines, plus one measurement that
closes an open cause. Sweep: **10/10 green.**

### 1. `Mode: both` — the cause is that the owner never learns the receiver accepted

Open since 2026-09-06 and measured three ways over a real network with no cause. It is now
measured (`shellboot/mode_both_cause_test.go`):

```
folder id — the same string on both peers:   {owner}.photos
  the receiver's record for it says:         accepted
  the OWNER's record for it says:            offered
  the owner's reverse leg admits only:       accepted
  owner's sync binding after mode=both
    plus a full reconcile:                   none
```

Acceptance is recorded by the receiver **in the receiver's own tree**, and there is no message
back. So the owner's filter cannot match, no reverse subscription is created, and the receiver's
writes have nothing to travel on — while the declaration takes and the owner truthfully reports
`mode=both`, which is why it read as a delivery problem. The probe runs with authorization out
of the picture, so it is not the grant stage; and it carries a control arm that fails if the
accept did not happen, without which *"the owner does not see accepted"* passes on a fixture
where nobody accepted anything.

**The consequence that changes planning: this and conflict propagation are ONE piece of work.**
Whatever carries *"I accepted your folder"* back to the owner is the same channel that carries
*"your change landed on my edit"*. Building either alone builds it twice, or wrong.

**Two-way sharing is not blocked by this** — it is the flow run once in each direction, which is
supported, gated end to end, and the documented way. The `direction … both` verb is the thing
that does nothing, and the operator doc now says so.

### 2. What our published docs would have told a stranger, and what was wrong with it

The audit's actual yield. **Six published claims across four declared documents** had gone false,
and none of them would have been caught by a test, because they are prose:

- **"Concurrent edits: not handled, that is milestone M3."** Shipped the previous day. The
  operator guide described the state of the world before the feature and named no verb for it —
  AP71's shape exactly, one document over: we corrected the doc when we shipped the *flow* and
  did not re-read the guide when we shipped the *feature*.
- **"A GUI surface: these are shell verbs."** There have been three sharing panels for a week.
- **"The provenance discriminator exists on every transition at zero cost."** The refuted claim
  from §19, still stated as fact in the replication landscape, in the section a reader goes to
  for what M3 *is*.
- **The M3 plan's step 3, "write a keep-both copy on conflict", read as the shipped design.** It
  is not: keep-both is opt-in and converge-and-record is the default, for a reason that is
  invisible in a two-way folder and decisive in a one-way one.
- **"A concurrent edit still loses a write, silently" — twice**, in the sharing walkthrough and
  in the direction doc, one of them in a section comparing us honestly against Syncthing. Those
  two were found only by *running* the rule this section is about, after the first four had
  already been fixed. A fresh anti-pattern rarely gets to prove itself in the same session.

**The generalisation, and it is a discipline candidate rather than a note.** Every one of these
was written by a session that shipped the thing correctly and updated the document *it* had open.
A feature lands in code, in one guide, and in a plan that now describes the past — and the plan
is the one a stranger reads to find out what the product does. **When a feature ships, grep the
published set for the sentence that used to be true**, not just the file you were editing.

### 3. The conflict-propagation question, answered as options rather than a work item

*"If I have four or five peers and a conflict with one, and then a conflict with another — these
should be propagated somehow."* Right, and the shape it wants is *put it in the tree and let
replication carry it*. Written up with the trade-offs rather than picked:

- **One form already works.** A `keep-both` sibling is a real file in the shared folder, so in a
  two-way folder it replicates by the path that already exists — no new namespace, grant or wire
  change. That is exactly how Syncthing, Dropbox, OneDrive and iCloud tell you, and the classifier
  already knows a sibling is an ordinary file. What it cannot do is carry metadata, or reach the
  owner of a one-way share.
- **The disciplined version needs a grant widening that is not small.** Today a receiver grants
  the publisher exactly one operation. For the owner to subscribe to the receiver's conflict
  prefix, the receiver must grant subscription and tree-read rights — and the publisher's own
  grant set does that with `Resources: ["*"]`, i.e. the whole namespace, in exchange for a
  conflict notice. A scoped form is the only acceptable one and has not been measured.
- **The spec's own home for this is priced out**, on our own numbers: revision auto-versioning is
  ~4.5 entities per write with latency growing 15× over 5,000 writes, which is why it must never
  be a per-folder toggle.
- **Recommendation: not yet.** The case this product ships — drop a file here, pick it up there —
  does not generate conflicts, and the receipt-plus-undo that shipped is the right amount of
  machinery for the rare one.

### 4. The GUI gate went red, and it was the instrument again — but only the *reporting* is fixed

`make twopeer-gui` came back **9 of 74 failed**, on checks that all read
`…/received/<file> never appeared`. It looked like the share had stopped working. It had not:
the panel reported *"4 of 4 files readable"* the whole time, at
`/data/entity-shared/a/photos` — the pre-filled default — while every assertion was pointed at
`/received`, the directory the harness had typed.

The transcript could not settle it, and that is the finding. `type` returned
`{"typed":"/received","realInput":true}`, which is **a receipt for the keystrokes leaving
xdotool** and says nothing about where they landed. So two very different faults — *the app
ignored the operator's directory* and *the operator's directory never reached the box* — produced
an identical log line, and the first evidence of either was nine failures about files at a path
nobody had ever accepted into. **D25's third instance: a field that prints is not a field that
answers**, now in the harness rather than in a coredump or a status surface.

`type` now reads the control back and returns what it actually holds, and the scenario asserts
that **before pressing Accept**. The re-run is **74 · 0 failed · 1 known-open** — the phase-12
waiver, reproducing exactly as documented.

**Say the uncomfortable half plainly: nothing was fixed that would change whether the keystrokes
land.** The change is to the reporting, and the two runs differed. So there is an intermittent
input-delivery failure in this harness — observed once in two runs — and it is now *legible*
rather than *absent*. A future red run of this shape should be read as the harness first.

### 5. "The first change after a restart is LOST" was wrong for five days — it is a DELAY

The scariest open item in this repo, quoted in six documents including two published ones and a
packet to another team, said the change was *gone* and that *only `resync` recovers it*.

**Measured** (`make twopeer-gui` PHASE 13, receiver restarts while the sender stays up):

```
asym1.txt, written just after the receiver came back
  absent at 90s   ← every gate this project has ever had asked exactly this
  present at ~107s ← no resync, nobody touching anything
```

**The characterisation was written on 2026-09-03. The catch-up supervisor landed on 2026-09-07.
Nothing re-measured it in between**, and in the meantime the sentence was copied forward into
five more documents.

**The number was inside the instrument's blind spot the whole time, and that is the part to
carry.** The supervisor doubles its interval after every pass that recovers nothing, so from a
restart the passes fall at roughly t=0, t≈120 s, t≈360 s — and **every harness here waits 90
seconds.** That window expires between the first two passes *by construction*.
**A test window shorter than the recovery mechanism's period turns a latency into a loss**, and
the write-up is then confidently about the wrong defect, at the wrong severity, routed to the
wrong people.

Still open and unchanged: live delivery of that one change misses, cause unknown, and nobody has
instrumented whether the miss is on the send side or the receive side. What changed is that this
is a latency defect in a flow that heals itself, not a data-loss defect — so it does not
disqualify unattended use. No bound is promised: the interval grows with idle time to a ten-minute
ceiling, and `resync` / **Pull now** forces it.

### 6. Where conflict detection lives, since it was reasonable to ask

It is **entirely ours** — `blob_resolve.go`'s hash comparison plus a walk of `system/history`
transitions — and `ext/revision` is not involved at any point. Three reasons, and the third is
the one worth knowing:

1. The spec's conflict entity requires `version_local` / `version_remote` and a project prefix
   hash. A mounted folder commits no versions, so there is no place to put one and no values to
   fill it with.
2. `DOMAIN-LOCAL-FILES` §1.1a rules concurrent same-path writes explicitly *not* a CRDT case for
   the domain, and points at the revision extension for merge semantics. Detection over a mount
   is application-tier by construction.
3. **`EXTENSION-REVISION` §2.2 rules that conflict entities do NOT sync** — *"peer-local… NOT
   included in version snapshots and NOT synced to other peers"*. So the intuition that putting
   a conflict in the revision namespace would make it replicate is refuted by the spec, and our
   unreplicated record is **aligned with the specified model** rather than a departure from it.

Where we did have a choice we spent it on compatibility: the keep-both sibling name is
byte-identical to the kernel's and gated against its vectors. One alignment item is owed — our
record's field names should move toward the spec's (`local`/`remote`/`base`) while nothing depends
on them.

**And a coverage gap surfaced by the question: every conflict test runs the ONE-WAY topology.**
Nothing covers both peers publishing the same folder and both editing, which is what an operator
gets from the two-way recipe — and it is the case where whether *both* sides notice depends on
which delivery lands after which edit. Not measured, and worth more than any propagation design.

### 7. Also corrected: a review whose headline finding was withdrawn and did not say so

The 2026-09-06 conflict-semantics landscape still led with *"the receiving peer binds no file
entity at all"* — the `podman cp` of a WAL store, withdrawn on 2026-09-07 (AP76) — with four
conclusions resting on it and no banner. The rolling log had recorded the withdrawal; the
document a session would actually open on this topic had not. **A supersession recorded only in
the status log has not been recorded**, because nobody arrives at a topic through the status log.

---

## §19 (2026-09-07) — the growth we could not bound, the machinery nobody could see, and a conflict detector that flagged every file

§18 left three things at the top of the list: bound the unbounded change recording, put the
catch-up machinery somewhere an operator can see it, and finish M3. All three are done. The
finding worth reading is the third one, because it was wrong in a way that looked right.

### 1. Recording growth is bounded per path now, and the bound is "stop"

Change recording costs a flat ~1.2 KB per WRITE and nothing prunes it, so one continuously
rewritten file — a log, a database, an editor swap file — is ~1.2 GB/day until the disk fills.
§18 established that the substrate's own `max_depth` prunes nothing and that there is no
collector in the cohort, so **there is no mechanism below us that makes an existing chain
smaller.** The only lever is to stop adding.

So the guard stops, and says so. Past 2,000 recorded versions of one path it writes a disabled
exact-path history config and a durable record naming the path, the depth, the budget and the
time; `status` and the *Sharing Status* panel both report it.

**The trade is stated rather than buried: stopping keeps the OLDEST versions and loses the
newest**, which is backwards for recovery. It is right for the workload it catches and it is
why a tripped limit is a problem line on every reading rather than an internal event.

Two things worth carrying. **The signal was free**: every recorded transition writes a head
pointer, which is an ordinary tree mutation, so one prefix watch is exactly one event per
transition for the price of a map increment — and it goes quiet by itself when recording stops.
And **the exclusion is surgical because the kernel's own specificity rule makes it so**: an
exact-path config disabled outranks the folder's `…/*`, so one file stops and its neighbours
keep recording. That was a reading of a sibling repo, so it was a hypothesis until the
end-to-end test ran it — with a control arm that does the same workload unguarded and asserts
the chain keeps growing, without which the test passes against a build where recording never
worked at all.

Measured: budget 5, `server.log` stopped at 6 transitions with `notes.txt` beside it still
recording; unguarded, 14 writes and a 14-deep chain still growing.

### 2. Delivery saturation, catch-up and the growth guard reached a pixel

All three were reachable from `entity-shell` and from nothing in the GUI — the wrong way round,
because the failure they describe (a big copy stops part way, no error, every row green) is the
one an operator hits first and can diagnose least.

`ReconcileOutcome.Delivery` was the sharpest instance: computed on every reading since the load
work, carried across the model boundary, and dropped by an undeclared bridge field one step
short of the screen. That is AP49, and nothing would have noticed.

The discipline the new section follows, and the one worth generalising: **measured-zero and
not-measured must not render the same.** "Nothing dropped" and "nothing counted drops", "no
pass recovered anything" and "no supervisor is running", "no path hit its budget" and "nothing
is watching growth" — same words, opposite facts, and in each pair the second is the state in
which the failure is silently in progress.

### 3. The conflict detector flagged every file in the folder, and the reason is the finding

M3's last step: notice when a delivery lands on a file you have edited, record what it replaced,
and let you put it back.

The discriminator was supposed to be free. The recorder writes who authored each chain position
— a local edit arrives through the watcher (`local/files:watch`), a delivered one through
blob-resolve (`local/files:write`) — and §16 measured exactly that. Read the head, compare.

**Every file in a received folder has a `watch` head.** The receiver's own watcher ingests the
file blob-resolve just wrote to disk; the bytes are identical but a file entity carries an
mtime, and the watcher reads the filesystem's rather than the one the write recorded. Different
entity, real transition, landing a second or so after every delivery.

The baseline test asserts `trans[0].Operation == "write"` and passes — because it reads within a
second of the delivery, before the echo arrives. **It is a measurement taken at one moment, read
as a property**, and it had been written into a doc comment and into `AGENTS.md` as a fact about
the chain. The generalisation: *a fact established by reading a mutable structure once is a fact
about that instant*, and the tell is a test that reads immediately after the event it is about.

What survives is the question *who last changed the BYTES* — walk the run of consecutive
transitions carrying the current content hash and read its oldest member's operation. An
mtime-only echo lengthens the run and cannot move its oldest member.

**The anti-vacuity arm is what caught it**, not review. One delivery to an edited file and one
to an untouched file in the same pass, asserting exactly one conflict. Without that arm,
`return conflict` unconditionally passes — which is precisely the detection bug our own
`SYNC-LIMITS` §5 says is the realistic cause of a conflict storm, and it is not self-limiting.

### 4. What a conflict does now, and why the default creates no files

`EXTENSION-REVISION` §2.3's `keep-both` keeps the local version at the path and writes the
incoming one to a sibling. That is right for a two-way folder and wrong as a **default** here,
for a reason that only appears in the topology this product ships most of: in a one-way share
the receiver would stop converging to the owner's version and the owner would never be told.

So the default converges — the arriving version wins on disk, as always — and records what it
replaced, which stays recoverable from the chain. `keep-both` is a per-folder declaration.
**Both versions are recoverable either way; the difference is whether both are PRESENT.**

`resolve -keep mine` writes through the **filesystem**, and the obvious route is a silent
regression: restoring through `local/files:write` records with a delivery's provenance, so the
next catch-up pass reads that head, concludes the copy is stale, and undoes the operator's
choice minutes later with nothing said. Gated by restoring and then running two passes.

Verbs: `conflicts`, `resolve`. Panel: a row per file with **Restore mine** / **Keep theirs** /
**Keep both**, and Restore offered only when the replaced version was actually kept. All four
of `SYNC-LIMITS` §5's storm rules are met and the section says how.

### 5. One flag was answering two questions

`ReconcileOnStart` means *"I have declarations to re-establish"*; the catch-up supervisor needs
*"am I going to be around to take another pass"*. They diverge for in-memory peers, so the GUI's
`--ephemeral` peer — the one configuration where a lost file has no next launch to be recovered
in — was the one without the loop. `Config.LongRunning` is the second question, or'd rather than
merged so the several hundred peers the suites build measure exactly as they did.

## §18 (2026-09-07) — a file costs eight queue slots, two of our published numbers were wrong, and two bounds we would have built on were already decided elsewhere

§17 found that a big directory copy stops part way and said the cause was a saturating delivery
queue. This is what happened when we stopped reasoning about that queue and swept it.

### The cliff was partly a number we chose, in the wrong unit

The subscription engine's delivery ring is configurable. Our SDK sets **4096** slots where
core-go defaults to **65536** — a deliberate choice on a real memory measurement (~20.3 MB per
peer, allocated eagerly, never released; a test binary holding ~100 peers paid ~2 GB). It was
justified in our own source against core-go's stated sizing, *"sized for 1000+-file mount
bursts"*, plus a four-fold margin.

**That arithmetic counted files. The queue counts notifications.** One mounted file emits the
watcher's file entity, the ingest chain's document, and the blob bindings. Swept with the
catch-up supervisor off, so this measures the delivery path and not the mitigation:

| ring slots | 2000 files | 10,000 files |
|---|---|---|
| **4096** (our old default) | **675**, 2350 dropped | — |
| 16384 | 2000 ✓ in 3.0 s | **2949**, 12535 dropped |
| **65536** (core-go's default) | 2000 ✓ in 3.0 s | **9100**, 900 dropped |
| 262144 | 2000 ✓ in 3.3 s | 10,000 ✓ in 5.9 s |

Bracketing those rows puts real demand at **6.5–8.2 slots per file**. A documented, four-fold
margin was short by most of an order of magnitude because the unit was wrong, and the failure
mode is silent data loss.

**Both readings turned out to be true**, which is not what we expected: we set out to
distinguish *"our default is too small"* from *"any finite queue fills"*. The default was too
small — `shellboot.DefaultDeliveryQueueSize` is now 65536, the application tier making the
opposite call to the library for the same reason it already does about the registry extension,
and `Config.DeliveryQueueSize` exposes it, because it had been a documented mitigation
reachable from no frontend at all. **And no value fixes it**: core-go's own default still loses
900 of 10,000. The producer is a person with a file manager, and nothing in the delivery path
can slow them down. The ring decides how big a burst finishes at live speed; the supervisor is
what makes every larger one a delay rather than a loss.

### Two numbers we published were wrong, and one of them was wrong in our own favour

**"Live delivery runs ~90 files/s"** came from a 200-file run. Measured across two burst sizes,
live delivery is **~2.3 s fixed cost plus ~0.36 ms per file** — and 2.3 s is essentially the
whole of that 200-file run. It was never a throughput; it was an operation's setup cost divided
by its file count.

That retires **"the catch-up path is ~20× faster than the live path"**, which we had published
as *the reason* the design leans on catch-up. The real gap is small. **Leaning on catch-up is
still right, for the other reason** — and that is where this gets uncomfortable.

### The specification had already said it, in a subsection we had not read

`EXTENSION-SUBSCRIPTION` §5.5 ends: *"For guaranteed consistency, subscribers SHOULD
periodically reconcile via GET on subscribed paths."* That sentence is the catch-up supervisor
we designed from first principles over three days. §6.3 already MUSTs the outcome for mirrors.

Our standing rule is *grep the dependency before you price the build*. We have applied it to the
kernel four times and had never once applied it to the **specification**.

**But when we did read it, §5.5's primary mechanism turns out not to work for this workload** —
and that is worth more than the embarrassment. Gap detection is stated per-URI and works by a
*later* notification arriving with a mismatched `previous_hash`. A folder sync writes most paths
exactly once, so a dropped notification is the only one that chain will ever carry and **no
successor arrives to mismatch**. Of 2350 dropped notifications, the number a conformant
`previous_hash` implementation could have detected is **zero**. For bulk-create workloads the
periodic reconciliation is not the belt-and-braces measure §5.5 presents it as; it is the only
mechanism that works.

### What went to architecture

`reviews/SUBSCRIPTION-SATURATION-AND-THE-LAYER-BOUNDARY-2026-09-07.md`, with the layering
question the operator asked and we cannot answer from here: *how much of flow control belongs in
the extension?* Our position, held with varying confidence — the policy (ring size, backoff,
whether to run a supervisor at all) is correctly ours; the **one** thing we would argue for is
that publisher saturation be observable to a subscriber, because it is the single fact an
application cannot obtain for itself, and every weakness in what we built traces to its absence.
Our adaptive rate is a blind timer standing in for a feedback loop for exactly that reason.

### The top open item got priced, and the substrate's own bound is a no-op

Our standing top item is that change recording is **unbounded**: 2 entities and ~1.2 KB per
*write*, so a shared folder containing a log or a database grows at ~1.2 GB/day with no error
until the disk fills. Recording is on by default for a folder that receives, because the chain
is what keeps an overwritten local edit recoverable.

Priced against the substrate first, and it looked like a config change:
`types.HistoryConfigData` carries `MaxDepth *uint64`, documented **"Max transitions per path;
nil = no limit"**, and the recorder calls `prune(path, maxDepth)` after every transition.

**It prunes nothing.** Measured through the real recorder at a real watched path:
`max_depth = 3`, twelve serialized writes, **twelve transitions still reachable from the head.**
`prune` walks to the nth transition and returns having written nothing — and cannot easily do
otherwise, since transitions are immutable and content-addressed, so severing a link cascades a
rewrite of the whole retained chain. Its comment's *"GC handles cleanup"* names a garbage
collector that does not exist anywhere in the cohort. Setting `MaxDepth` today is a **pure
cost**: an O(max_depth) walk per write, for nothing.

Routed with the reproducer. The probe is kept and **inverts** — it fails if `max_depth` ever
starts working, and says to take the cheap fix. The planning consequence, which is the reason
this is worth a section: **a workbench-tier guard is real work, not a config field.**

This is the fifth time we have priced a build against the substrate before starting and the
first time the answer came back negative. That is the point of running it — *"the substrate does
not have this"* is worth exactly as much as *"it does"*, and only one of the two is free.

### Also this session

- **`PeerManager.Destroy` never stopped the catch-up supervisor.** The loop runs on
  `context.Background()` so a startup timeout cannot kill it, which also meant nothing cancelled
  it when a peer was destroyed — one goroutine per removed peer, taking passes against a closed
  store for the life of the process, invisible in `status` because a failed pass is not
  recorded.
- **The console had never set `ReconcileOnStart`**, so it ran neither the startup reconcile nor
  the supervisor. Third instance of that shape on one flag. And the flag answers the wrong
  question — it means *"I have durable declarations"* where the loop needs *"am I
  long-running"*, so an in-memory peer that accepts a share mid-session is still uncovered.
  Recorded rather than closed, with the reason: `Bootstrap` is what the test suites build peers
  with, so defaulting the loop on for every peer changes the load profile of the whole sweep.
- **A test we wrote this session failed 2 runs in 3 under suite load and passed every time in
  isolation** — beside an unrelated change that raised per-peer memory 2.4×, which is exactly
  what a memory regression looks like. It was our test: it asserted on the *starting* value of
  an interval the adaptive rate is designed to change, so it was really asking whether a
  goroutine had been scheduled yet. Six control runs located it. Worth recording because the
  wrong conclusion arrives first and is the one nobody argues with.

---

## §17 (2026-09-07) — what folder sync does under load, and the copy that stopped at 676

The load and failure-mode questions, asked for the first time. Full numbers, triage order and
known limitations are in `docs/architecture/SYNC-LIMITS-AND-FAILURE-MODES.md`; this is the
summary and what changed because of it.

### The failure an operator hits first, and it was live

Drop 2000 files into a shared folder at once. The sender ingests and binds all 2000. **676
reach the receiver, and it stops there forever.** No error on either side, no chain error,
`syncs` and `status` healthy throughout.

The cause is not a bug in the kernel: the subscription engine's delivery queue is bounded and
**drops on full**, deliberately, to avoid a deadlock. It happens on the **SENDING** peer,
before anything reaches the wire — measured `sender dropped=2327, receiver dropped=0` — so the
receiver is not behind, it was never told. There is no failed delivery to retry and no local
counter that moves. Both peers are, from their own point of view, correct.

**The defect was ours: nothing had ever read the kernel's counter.** `DroppedDeliveries()` and
`DeliveryQueueDepth()` exist precisely so saturation is visible, and they had zero readers in
this repo — not in shipped code, not in a test. D20 aimed at the kernel for the fourth time.

And the suite could not have caught it. **Every cross-peer test in this tree moved between 1
and 10 files**, so the burst regime was not under-tested, it was unreachable — no fixture was
within two orders of magnitude of the bound. Catalogued as **AP77**.

### What changed

- **`shellcmd/delivery_health.go`** surfaces saturation through `status`, on the peer that
  dropped. A silent stall is now a visible one.
- **`shellcmd/catchup.go`** — a 60 s supervisor that re-derives the truth by asking each
  sending peer what it actually holds and pulling what is missing. **On by default wherever
  `ReconcileOnStart` is**, wired in `Bootstrap` rather than per-frontend (AP67). It does not
  dial, which is what makes it safe on a timer where `Reconcile` is not.
- **`catchup`** is a verb, and `status` reports the last pass — a background loop that silently
  repairs things is half a feature, because an operator who cannot see it cannot tell "the
  system healed itself" from "nothing was ever wrong".

Gate: `shellboot/catchup_e2e_test.go`. Supervisor off, a 1500-file burst delivers **621** and
stops; on, **1500 of 1500**. The control arm runs FIRST and the test *skips* rather than
passing if the burst did not saturate on that machine — "1500 of 1500" is equally true of a
working supervisor and a burst that never needed one.

### The numbers that change design decisions

**CORRECTED in §18 — both figures below are wrong.** Live delivery is ~2.3 s fixed + ~0.36 ms
per file; the "~90 files/s" was a 200-file run's fixed setup cost divided by its file count,
and the "20× faster" conclusion drawn from it does not survive. Backfill: ~1,900 files/s. A pass with nothing to do: **0.24 ms/file** (2000 files in 472 ms),
because F9 already short-circuits on an equal blob hash. That asymmetry is the whole reason the
design leans on catch-up rather than on making delivery more reliable.

**Change recording is flat and unbounded.** 2 entities and ~1.2 KB per write, with p50 latency
that does not move between 1k and 100k writes (141 µs → 165 µs). Affordable — but nothing
prunes it, and the cost is per WRITE, not per file. A continuously-rewritten file is **~1.2
GB/day**, silently, until the disk fills. **Do not share a directory containing a log, a
database, or an editor's swap files.** Guarding this is owed.

**Revision auto-versioning is the opposite and must not become a per-folder toggle.** ~4.5
entities per write, and p50 goes 452 µs → 7 ms over 5,000 writes because each write recomputes
a trie root over the whole prefix. Correct on a small curated prefix an operator points at
deliberately; a trap as a checkbox beside a shared folder.

### Conflict storms — designed against before they are reachable

The realistic cause of a thousand conflict entities is a bug in "changed since delivery", not
operator behaviour: a genuine concurrent edit is rare and self-limiting, a comparison bug fires
on every file every pass. So three rules bind before conflict handling ships — a conflict pass
is **rate-limited and fails closed**; conflicts are **listable and bulk-resolvable in the same
change** that makes them creatable; and keep-both never destroys, so recovery is always
"delete the copies". `SYNC-LIMITS-AND-FAILURE-MODES.md` §5.

### The rate adapts, because a fixed period is wrong for one of the two regimes

A folder that just took a burst is missing two thirds of itself; a folder idle since yesterday
needs a pass only to notice something rare. Three orders of magnitude apart in urgency,
identical in cost.

The receiver cannot see the sender's counter, so the only local signal is **its own passes**:
recovered files → delivery is losing things now; recovered nothing → it is not. No protocol
change, and exactly as accurate as the use it is put to.

| Signal | Next interval |
|---|---|
| The pass recovered files | **5 s** — to the floor, in one step |
| The pass recovered nothing | **double**, to a 10 min ceiling |
| Any pass | **never shorter than the pass itself** |

`nextCatchUpInterval` is a pure function, so it is gated without a clock. Three properties, each
earned: **recovery is asymmetric** (back off gently, return to the floor in one step — symmetric
ramps are what make a sync tool feel broken, relaxing to ten minutes and then needing six passes
to get interested again); **the ramp is a gradient, not a snap** (an earlier version clamped the
way up at the configured rate and jumped 5 s → 60 s the instant a burst ended, discarding exactly
the rates worth having while a copy trickles in, so the configured rate sets where the ramp
*starts*); and **anti-churn — the wait is never shorter than the pass that produced it**, bounded
at a 50% duty cycle and asserted as an invariant over a 50-pass sustained-load run.

That last one is the one that matters, and it is why settling first is not merely cheaper but
*more correct*: a catch-up reads **current state** rather than replaying a change stream, so one
pass over a settled folder gets everything, while a pass taken mid-burst re-does most of its work.

`status` reports the interval and whether the loop is recovering or settled — the two regimes
produce an identical folder listing, and an operator looking at a stale folder needs to know
which one they are in.

### Still open

Backpressure (the sender discards rather than slowing — belongs in the kernel's queue; catch-up
makes it a delay, not a loss). Unbounded recording. First-change-after-restart, now mitigated by
the startup catch-up pass but still uncaused. `Mode: both` receiver → owner. Four+ peers. No GUI
surface for any of the catch-up machinery — it is reachable from `status` and `catchup` and from
no pixel, which is D23's shape.

---

## §16 NEW (2026-09-07) — the blocker was the instrument, and the milestone it blocked is mostly already built

Three things, and the third is the one to carry forward.

### 1. There is no M2b. The receiver binds correctly, and always did.

§15 reported *"the receiving peer binds no file entity"* as the thing standing between us and
concurrent-edit work, and inserted a milestone ahead of it. **It is not a defect.** The
measurement was a `podman cp` of a live SQLite store followed by `sqlite3` on the copy.
File-backed stores open **WAL**, so every write since the last checkpoint lives in the `-wal`
sidecar, which did not come along — and the missing rows come back as **zero rows with no
error**. Measured on one store at one instant: **0 rows naming the file from the main file
alone, 2 with `-wal` beside it.**

The asymmetry that made it convincing is exactly what WAL predicts. The publisher's binding
was older and checkpointed; the receiver's was seconds old and still in the log. So the
artifact presented as a clean structural finding about one side of the pipeline.

Refuted three independent ways: the receiver's live location index holds the binding in **all
four** configurations that differ from the in-process tests (memory/sqlite × symmetric and
asymmetric root names); the measurement error reproduces exactly on demand; and an **already
green** gate — `TestBackfill_FilesAlreadyInTheFolderArrive`, asserting `AlreadyCurrent == 4`
after a resync — can only pass if the receiver binds. It had been passing the whole time.

**Four conclusions built on it are void with it.** F9's already-current check is not dead
code. `resync`'s "everything already current" does fire. And neither `Mode: both`'s missing
receiver → owner leg nor first-change-after-restart is explained by this; both are open again
on their own terms, which for `Mode: both` means the measurement §14 already recorded — the
owner holds no sync binding naming the receiver (`receiveFromPeers`, `shellcmd/reconcile.go`).

The thing that was real: **every sync test in this tree asserted bytes on disk and none read
the receiver's tree.** The whole tree-side half of the receive path had zero coverage, which
is why a claim that it did not work at all was consistent with a fully green suite.
`shellboot/receive_binds_e2e_test.go` closes that, and additionally asserts the two peers'
blob hashes are **equal** — the property everything downstream actually needs — plus that the
`ls` verb agrees with the index, since the containerised harness has no way to read a peer's
store (the alpine image ships no `sqlite3`) and has to ask the shell.

Catalogued as **AP76**. The two generalisations are worth more than the recipe: **silence from
a new instrument is a claim about the instrument first** — point it at a case you know is
populated before an absence becomes a finding; the publisher's own binding was right there and
would have failed identically — and **when you have just finished proving that every surface
lies, the replacement needs its own control arm**, because the reasoning that retired the
surfaces is what makes the new instrument feel beyond question. That is precisely how it
happened: the session had correctly established that `info`, `mounts`, `subscription ls` and
`inspect errors` each gave a wrong answer that day, and reached for "read the store directly"
as the trustworthy floor. Right instinct, wrong floor.

### 2. The concurrent-edit guarantee is already met by the substrate. It was never switched on.

The measurement nobody had run, and it reframes the milestone
(`shellboot/concurrent_edit_baseline_test.go`). With history recording enabled on a mount
prefix, a concurrent same-path edit already produces exactly what `DOMAIN-LOCAL-FILES` §1.1a
rules — last arrival wins on disk, **both writes at distinct chain positions**, and the
overwritten bytes byte-recoverable from the chain:

```
[0] updated  local/files:write   ← the delivered edit; won on disk
[1] updated  local/files:watch   ← the receiver's own edit, preserved
[2] created  local/files:write   ← the seed
```

Two things follow. **Recording is opt-in per path** (`ext/history/config.go`,
`configCache.find`) and **nothing in the mount / sync / share path installed a config** — so
the chain the ruling depends on was not being written for the one namespace whose whole point
it is. With no config a query returns **empty and no error**, which reads as "the tree kept
nothing"; that is the same failure shape as AP76, one layer up, and it is why this went
unnoticed. And **the provenance discriminator conflict detection needs is already on every
transition**: a local edit arrives through the WATCHER (`local/files:watch`), a delivered one
through `blob_resolve`'s dispatch (`local/files:write`), so *"they edited this"* and *"I am
behind"* are distinguishable today at no cost. Known limit, invisible from the field name: a
local caller dispatching `local/files:write` directly records as a delivery — nothing in the
shipped flow does that.

### 3. Landed: the chain is now a derived output of the control loop.

`shellcmd/folder_history.go`. A folder that `Receives()` and is mounted gets a
`system/history/config` for its mount prefix, written by `Reconcile` — so it is idempotent,
restart-safe, and automatically correct when a folder's direction changes later. Not by
`mount`, for the same reason the policy row and the sync binding are not: a verb that wrote it
directly would be undone by the next pass.

`Receives()` and **not** `IsLocal()`, and here the two genuinely differ: `share` declares the
owner's folder `both`, so either side of a shared folder can be overwritten and both record.
Keying on `IsLocal()` would have left the owner — the peer whose files these are — as the one
side with no chain. The first version of the test asserted the opposite and was wrong; the
control arm now sets `send` explicitly and asserts a send-only folder gets **no** config,
because a folder with one writer records an entity per save forever and answers no question.

**M3 is therefore not "build a merge engine".** In order: install the config (done); branch at
`blob_resolve.go`'s existing F9 *different* arm; write a keep-both copy under the spec's own
`{path}.keep-both-{hash8}` naming; then layer `EXTENSION-REVISION` for real three-way merge,
whose commit/log/status half §15 already proved works over a `local/files` prefix. The
paper-worthy claim survives: a content-addressed tree makes both parents of a conflict
permanently addressable with no side-car format. We are wiring it up rather than inventing it.

`FILE-REPLICATION-LANDSCAPE.md` §6 is corrected in place — M2b withdrawn, M3 rescoped.

### 4. And it is asserted in the real environment, not only in-process

`twopeer-sync.sh` reads the receiver's tree now. Two real containers, real TCP, real SQLite:
**35 checks / 2 failed**, where the two are the pre-existing asymmetric-restart defect
(baseline 30 checks; the 5 new ones are all green).

```
PHASE 5b — the receiver's TREE, not just its disk
  ok  tree: the receiver BOUND one.txt, not only wrote it
  ok  tree: a second delivered file is bound
PHASE 8b — rename, and a concurrent edit
  ok  conflict: both writes are on the chain (4 positions) — the losing edit is addressable
  ok  conflict: peer-b's OWN edit is on the chain (local/files:watch)
  ok  conflict: the DELIVERED edit is on the chain (local/files:write)
```

Nothing in the script enables recording — the reconciler does. That is §1.1a, end to end, in
the environment the false finding came from.

Four defects turned up on the way there, all found by *using* the thing rather than reading it:

- **`ls` needs `@alias/…`, not a bare relative path.** At the REPL root the working directory
  is `/`, which is the connection list and not a peer namespace, so `ls local/files/received/`
  resolves to `/local/files/…` and fails with `no connection for path`.
- **`expect` greps the WHOLE log, so a listing assertion written with it is vacuous** — a
  filename any earlier phase printed satisfies it. `log_lines` + `expect_after` scope an
  assertion to one command's output; every listing assertion needs that form.
- **`history query` refused the `@alias` form and blamed the config for it.** It passed its
  argument to the handler raw, so the path matched nothing and the operator was told *"(no
  transitions recorded — is a config installed?)"*: a confidently wrong diagnosis pointing at
  the one thing that was fine. A bare path still passes through untouched — the store
  canonicalizes it against the local peer, which works from any working directory — and
  `@alias/…` now resolves like everywhere else.
- **`history query` did not say WHO wrote each position.** The recorder has carried
  `Handler`/`Operation` on every transition since it was written and the renderer dropped both.
  On an ordinary tree path that column reads `system/tree:put` throughout and says little; on a
  shared folder it is the whole operator story — `local/files:watch` is *you* edited this,
  `local/files:write` is *their copy arrived and replaced yours*. It is also what let the
  harness's assertion go from "four things happened" (satisfied by four deliveries) to "both
  sides are represented", which is the claim §1.1a actually makes.

**Still owed:** the same assertions in `threepeer-sync.sh`, and a GUI surface — the chain is
reachable from `history query` and from no pixel, which is D23's shape.

---

## §15 SUPERSEDED (2026-09-06) — the receiving peer stores nothing, and the conflict question was ruled long ago

> **Superseded by §16.** The central finding below — that the receiving peer binds no file
> entity — is **false**, and it was an artifact of reading a copied WAL SQLite store (AP76).
> The conflict-question half of this section stands. Kept unedited so the error is legible.

Two corrections and one new defect, and the defect is the one that matters.

### The conflict question was never open

`DOMAIN-LOCAL-FILES` §1.1a specifies concurrent same-path writes exactly: last-arrival wins at
the filesystem surface, **both writes recorded in the tree at distinct chain positions**, no
automatic merge, and `EXTENSION-REVISION` is where collaborative-edit semantics live. The spec
attributes the ruling to *"workbench-go **WB-25** closure"* — **our own case validated it.**

`entity-core-go` implements **all 19** revision operations, writes conflict entities at
`system/revision/{H}/conflicts/{path}`, and offers `keep-both` (what Syncthing, Dropbox, OneDrive
and iCloud all do) and `manual` (conflict tracking, on demand). We invoke none of it, because we
never commit versions. **Our gap — not a spec gap and not a core gap.**

`docs/architecture/FILE-REPLICATION-LANDSCAPE.md` had already established all of this on
2026-09-01, including the milestone sequence in which this is **M3**. It was re-derived rather
than read. `twopeer-sync.sh` PHASE 8b still carries the comment *"there is no specified behaviour
to assert against yet"* — false, and the reason it stayed parked (D27).

### Verified: `revision` works over a `local/files` mount prefix

Nobody had run M3's feasibility check. Live, one peer, real mount: `revision commit` over
`local/files/src/` commits with a root hash, a file edit produces a second commit with a
different root, `revision log` shows a real DAG, and **`revision status` reports a conflict
count**. The composition the landscape doc calls novel is reachable from the shipped shell today.

### The blocker: the receiving peer binds NO file entity

Measured by reading the receiver's SQLite store directly rather than through any surface:

| | |
|---|---|
| publisher store | `local/files/src/f.txt` bound correctly by its watcher |
| receiver store | **zero** rows matching the filename |
| receiver `archives/` | **empty** |
| receiver disk | correct bytes, present |

**A delivered file reaches the receiver's disk and produces no entity anywhere in its tree.** Not
backfill-specific: a file present before the share and one created after behave identically.

That single fact explains the shape of several open items — conflict tracking is impossible (no
local entity to compare against), `revision` has nothing to version on the receiver, the F9
`already_current` short-circuit is dead code so every delivery re-fetches the whole blob closure,
and `resync`'s *"everything already current"* — documented as the only positive confirmation this
flow offers — comes from a check that never fires. It is also a candidate cause for both
first-change-after-restart-is-lost and `Mode: both` not running backwards.

**The cause is not established and is deliberately not guessed at.** `local/files:write` binds
unconditionally and reports failure if the bind fails; the receiver reports neither. Also found:
`inspect errors` counts the bare `system/runtime/chain-errors` namespace entity as a marker, so
it reports one error with every column blank when there is none — a phantom that was very nearly
written up as the cause.

### What this session got wrong

Three readings were retracted mid-investigation — namespace pollution, swapped subscriptions, and
a chain error causing the bind failure — all from trusting a surface. A peer-id read out of
`info` turned out to be the **connected** peer's rather than self, and every conclusion built on
it was confidently wrong. **The store is ground truth; `info`, `mounts`, `subscription ls` and
`inspect errors` were each wrong at least once in one session.**

## §14 (2026-09-06) — three peers: fan-out and a chain work, `Mode: both` does not run backwards

Two peers are one edge, so every sharing gate here has been blind to the question that begins
*"and then the third machine…"*. **`make threepeer-sync` — 27 checks, 0 failed**, three peers in
three containers on one real TCP network, every file assertion on **bytes on disk at the far end**.

**FAN-OUT works.** One folder, one publisher, two receivers. The mechanism under test is the
per-peer policy row: `system/capability/policy/{peer}` is a union with exactly one writer (AP68),
and this is the first time two declarations touch two rows in one reconcile pass. The assertion
that would catch a clobber is not "both receivers got the backfill" — it is **a change reaching B
*after* C was added**, because a second share overwriting the first peer's row makes the *first*
receiver go quiet. It does not.

**CHAIN works.** A → B → C. A file peer-b never authored reaches peer-c, and a change at the
origin traverses both hops. B forwards by **publishing its own mount**, not by republishing A's
folder — which is `mode=receive` and deliberately does not republish — so a forward is an explicit
operator act rather than an emergent property of receiving. The safety half holds too: **a write
at the middle does not travel back to the origin.**

**`Mode: both` does not run receiver → owner.** Shipped, and never exercised two-way over a
network until today. Measured three ways:

| arm | result |
|---|---|
| one side declares `both` | b's own write reaches c; c's does not come back |
| **both** sides declare `both` | c's write still does not come back |
| both sides, **symmetric root names** | still does not come back |

The third arm is the discriminator and it **refutes the obvious hypothesis.** A received folder
has two root names — the sender's keys the binding and the folder id, the local directory is the
receiver's choice — and that trap survives any test whose two peers happen to agree on a name. It
is *not* the cause here: asymmetric and symmetric behave identically.

Which leg is missing is recorded rather than guessed: **`direction` takes** (the owner reports
`mode=both` for its own folder) and **the owner holds no sync binding naming the receiver**, so
the owner-side receive leg is never established. `receiveFromPeers` (`shellcmd/reconcile.go`) says
a local folder that `Receives()` should pull from every peer whose state is `Accepted`, so that is
where the next session starts. Measured with `note()`, never `ok()` — **a measurement dressed as a
check is how an undesigned behaviour gets recorded as a passing requirement.**

One thing that landed on the operator surface: `direction` takes a folder-id of
`{owner-peer-id}.{sender-root}`, so an operator who accepted into a directory of their own
choosing sees an id built from a root they never typed. The first version of the harness looked up
the local name and found nothing.

**Still untested:** four or more peers, and a triangle.

### Coordination — the packet that was owed

`APP-CONVENTION-SHARE` §5 marks `[OPEN-CONVENTION-1]` and says arch will not rule it: whether
retrieval is a term of the cross-impl contract *"has a real answer and this document does not know
it"*, to be resolved by the implementing peers. We had built the retrieval leg and never answered.

`reviews/APP-TIER-FILE-SHARING-PATTERN-2026-09-06.md` answers it: **retrieval is follower-local**
— a publisher does nothing differently for a closure-pull follower than for a diff follower, which
the fan-out and the chain both demonstrate — **but our form has a precondition the convention's own
floor permits a peer to lack**: a subscription engine, and file entities at a pattern-matchable
prefix. So the cross-impl term is a *publisher capability*, not the follower's `strategy`. The
packet also carries the four mutual-authorization facts (AP63), which are properties of the kernel
rather than of our app and which every seat building file sharing will meet.

**Two delivery findings, both ours:**

- **`dev` was six commits behind `origin`** — spanning the previous two sessions. Pushed. An
  artifact that exists only in a working tree does not exist, and this file has said so for weeks.
- **AC-2 was filed and never delivered.** Our `localfiles.Handler.Load` finding is written and
  committed (`586661d`) and a subject search of `entity-core-go`'s tree finds **no trace of it**.
  Filing is not routing. Arch's row reads *"the review packet is theirs and is not yet written"*
  — half right, and the half that is wrong is the half that matters.
- **OP-1 closed on our side.** Routed to us 2026-09-03 and not started: five non-test sites
  emitting `400 unknown_operation` are now `501 unsupported_operation`. The status is the half
  that matters — 400 tells a caller to fix its request, 501 tells it to degrade. **The cross-impl
  validator was widened, not switched**: it accepts both spellings, because narrowing it on the
  day we moved our own emitters would report every peer that had not yet moved as failing to
  implement the operation — a confident wrong answer about somebody else's conformance, produced
  by our own release timing.

## §13 (2026-09-06) — the SIGSEGV, named: every thread but two was on a 16 KB signal stack

**§12 left one open item and called it the most valuable thing on the list. It is closed.**
The crash is an **alternate-signal-stack overflow on a thread nothing covered**, it is
**deterministic rather than rare**, and the reason it read as rare for a month is that only
*whether a signal arrives* was ever the random part.

### What the two cores actually say

Both crashes from §12 were still on disk. Read from the cores' own **`PT_LOAD` program
headers**, they are the same event to the byte:

| | core 726963 | core 1053759 |
|---|---|---|
| faulting `rip` | `libcoreclr+0x3af3da` | `libcoreclr+0x3af3da` |
| `rbp - rsp` | `0x1b30` (6,960 bytes) | `0x1b30` (6,960 bytes) |
| `rsp` vs the usable base | **800 bytes below** | **800 bytes below** |
| consumed / available | **13,088 / 12,288** | **13,088 / 12,288** |
| `si_code` / `si_addr` | 128 (SI_KERNEL) / 0 | 128 (SI_KERNEL) / 0 |
| PAL-shaped stock 16 KiB alt stacks still present | **15** | **16** |

`rsp` sits inside a 4 KiB `PROT_NONE` guard page immediately below a 12 KiB `rw-` region —
the shape of the PAL's per-thread alternate signal stack, which maps 16 KiB and guards the
low page. The chain on it is **three return addresses**, so it is not a recursion and not
nested delivery: it is **one 6,960-byte frame** (CoreCLR building a `CONTEXT`) landing when
~6.1 KiB had already gone to the kernel signal frame and the handler prologue.

**The missing term was the CPU.** This host is an AVX-512 machine, where the kernel's XSAVE
signal frame is far larger than the PAL's compile-time `SIGSTKSZ` assumed:

    compile-time SIGSTKSZ    = 8192
    compile-time MINSIGSTKSZ = 2048
    sysconf(_SC_MINSIGSTKSZ) = 3376

So on this hardware **any** signal taken on a stock-alt-stack thread overflows, by exactly
800 bytes, every time. Which also retires three artifacts that had gone unexplained since
2026-08-21: `createdump` produces nothing (no stack left to run it on), the dump carries
`si_code 128`/`si_addr 0` (the kernel's `force_sigsegv` when it cannot build a frame — a
**second** cause of that signature, and the opposite of a re-raise), and there is no
`overflowed sigaltstack` line in the journal (that message is printed at frame setup; this
overflow happens inside an already-running handler).

### Why every gate was green while this was live

The mitigation had been applied **per thread**: the UI thread on 2026-08-21, the render
thread on 2026-09-02 after the render thread crashed. Both were correctly covered, both were
asserted, and both assertions were true. The 2026-09-06 fault landed on a **third** thread —
one no managed hook can reach, because `sigaltstack` must be called on the thread it covers
and nothing runs on a fresh thread-pool, finalizer or timer thread.

**The check's subject was the mitigation, not the process.** That is AP74, and the file's own
header had already written the gap down as prose (*"Other managed threads are still
uncovered"*) — D27's shape, a dismissal recorded where review never re-reads it.

### The fix, and how it is held

`avalonia/altstack-preload.c` interposes **`sigaltstack(2)` itself** under `LD_PRELOAD`, so
every thread gets 1 MB at the moment the runtime creates it — no enumeration, no list to keep
current. It only ever enlarges, never fails a call the real one would have satisfied, and
frees its mapping at thread exit so a long-lived GUI does not leak address space.

**The gate samples the population rather than counting installs**: the app starts one ordinary
thread and asks the kernel what that thread actually got
(`CrashDiagnostics.ProbeAltStackCoverage`). `AltStackCoverageTests` asserts on it,
`run-xvfb-smoke.sh` exits 3 without `coverage=ALL-THREADS`, and `WB_ALTSTACK_BYTES=0` still
restores the crashing configuration in full — in the interposer as well as in the managed
installs, so the A/B is of one variable.

Measured, same binary, three arms:

| arm | a fresh thread's alt stack |
|---|---|
| no preload (every launch before this) | **16,384 bytes** — the crashing config |
| interposer | **1,048,576 bytes** |
| `WB_ALTSTACK_BYTES=0` | 16,384 bytes — control arm restores the bug |

And the mechanism is **demonstrated, not inferred**: a 40-line reproducer builds the PAL's
exact stack shape, installs an `SA_ONSTACK` handler with the measured 6,960-byte frame, and
dies with `rsp` in the guard page — the same structural signature as both production cores.
Same binary under the interposer: survives.

### Two things to carry that are not about signals

- **`info proc mappings` on a coredump cannot see a stack.** It is served from the `NT_FILE`
  note, which lists only *file-backed* mappings, so every stack, heap and guard page reads as
  *"not present in core"* — which looks exactly like a wild pointer. Reading the `PT_LOAD`
  table instead turned "rsp points at nothing" into the whole diagnosis in one query. The
  previous session's *"not a stack overflow"* was taken with an instrument that could not see
  one (AP75).
- **A dlopen'd library does not interpose.** The first version of the coverage report said
  *"libaltstack.so IS loaded but did not enlarge — investigate"* about a process that had
  simply never preloaded it: `DllImport` had `dlopen`'d the file sitting beside the binary.
  A confidently wrong diagnostic pointed at the wrong layer. The check is now the interposer's
  own call counter, and the message names the actual cause.

### Still open

`4 of 3 files readable` and the asymmetric-restart first-change loss are both unchanged from
§12 — this session did not touch either.

## §12 (2026-09-06) — the two-peer sharing flow, by pressing buttons

**`make twopeer-gui`: 74 checks, 0 failed, 1 known-open.** Two real Avalonia binaries, two
containers, one real TCP network, real capability grants — connected by an address **typed into a
text box**, a folder shared by **pressing Share**, accepted by **pressing Accept** on the card that
appeared, then create / modify / delete propagating, then both peers restarted on the same stores,
then one peer restarted alone. Every file assertion is on **bytes on disk on the receiving side**.
No `--open-access`, so the permission stage is real (AP63).

That closes §0 of the validation audit. The operator's own path now has a gate.

### Three product defects it found, all fixed

- **`--identity NAME` demanded an identity that already existed, while the app's own `--help` said
  it was "created on first launch".** `ensureDefaultIdentity` only ever ran for the name `default`;
  any other name failed with a 404. In the shell that is an inconvenience with a documented next
  step (`identity create`); **in the GUI it is a dead end**, because the bridge fails to
  initialise and no surface exists from which to create the identity that would let the app start.
  Fixed with a **second flag** — `--new-identity NAME` — rather than by making `--identity`
  permissive: loading names a peer that exists, creating brings a new one into being, and since the
  tree is peer-id-namespaced a typo under create-if-absent would silently abandon every entity,
  mount and grant the intended peer owns, including the ones other machines wrote naming it.
  Refusing an unknown name was right; having no way to say *"yes, a new one"* was the defect.
  Gated three ways in `shellboot/named_identity_test.go`, including that the refusal still refuses
  and that creating never overwrites.
- **The share form offered no peers.** Connect to a machine, press "Share a folder…", get an empty
  dropdown and *"Which peer?"* — with no control anywhere that would populate it, so the only escape
  is restarting the app. The panel is wake-driven and has no Refresh button by design, but the wake
  watches the **declaration** prefixes, and connecting deliberately writes no declaration
  (`RememberDeviceAddress` updates, never creates). So nothing wakes and the list stays as it was at
  panel-open: empty. **This is AP73 in the mirror** — that rule says a Refresh button on tree data
  is a missing subscription; the converse is that state which is *not* in the tree cannot be
  subscribed to, so a surface reading it must re-read at the moment of use. Opening the form is that
  moment.
- **The receiving operator was never told anything.** Same shape, worse: an offer *to* us lives in
  **their** tree, so no local subscription can ever fire on it. peer-a shared a folder, peer-a's
  panel confirmed it, and peer-b's Sync panel — open the whole time, on the machine the share was
  addressed to — stayed empty indefinitely. AP73's own text names this as the single honest
  exception, so the panel now polls known peers every 15s while open. It is a **read** over an
  existing connection, never a pass: a reconcile dials and writes, and an operator leaving a panel
  open overnight must not turn their window into a dialer. The rebuild is gated on a signature,
  because the offer card holds a TextBox the operator types a path into and an unconditional
  15-second rebuild would discard it mid-typing (AP49's second half, at a timer's tempo).

### What is open, and precisely how open

- **The GUI SIGSEGVs.** Twice in four full runs, both times on peer-a, both times with the
  breadcrumb trail ending in Peer Connections render churn (`NearbyRender` / `LivenessRender`)
  rather than in an input handler. Two cores captured. Not the Go bridge (0 `libbridge.so` frames),
  not the mesa driver (0 GPU modules), **not a stack overflow** (every repeated-address run has a
  *mixed* stride, and per D25 a mixed stride is not a recursion), and no `overflowed sigaltstack` in
  the kernel log. `createdump` again produced nothing. It is intermittent and it is real, and it is
  the most valuable thing on the list.
- **`4 of 3 files readable`** on the publisher's own folder row after a delete. The source layer
  drops the file and the ingested document survives, so the panel prints a count that is nonsense on
  its face. Both sides of a lossy stage are reported on purpose (AP59) — this is the other half of
  that being wrong.
- **The asymmetric restart still loses the first change**, now reproduced *in the GUI*: restart the
  receiver alone, the next file changed never arrives, the one after it does. It is waived by name
  in phase 12 and nowhere else — phase 11 restarts **both**, which is the case documented to work,
  and waiving it there would have hidden a real regression behind somebody else's defect.

### And a note on the instruments, because three of them lied

Every one of these was found by running the thing, and each is a shape worth recognising. **A
harness printed its green banner after a fatal abort** — zero checks, zero failures, and a sentence
claiming two peers had shared a folder; an empty result set satisfying "nothing failed" is the
oldest way a harness lies. **A failed redirection on `exec` is fatal to a non-interactive shell**,
so an `if exec 3<>/dev/tcp/…; then` retry loop cannot retry — it killed the harness mid-line with
the container still running and no verdict, which from outside reads as the *app* hanging. And
**`exec 3<&- 2>/dev/null` closes fd 3 and then permanently gags the shell's stderr**, because `exec`
with no command makes its redirections permanent: the run executed its whole restart phase, reached
its verdict, printed nothing, and exited 0. That one read exactly like a crash and was the opposite
— a clean green run that had silenced itself.

## §11 (2026-09-05) — the GUI can be driven now

§10 said the top of the list was a harness that drives the app an operator actually opens. It
exists: **`make gui-drive`** presses a named button in the real Avalonia binary, under a real X
server with a window manager and a real render thread, and reads back what the window then says.
Fifteen checks, green, in about a minute.

**There is no Selenium for this stack, and that is measured rather than assumed.** Avalonia 11.2.3's
X11 backend ships **no AT-SPI bridge** — `strings Avalonia.X11.dll | grep -ci atspi` is 0, and
`Avalonia.FreeDesktop` carries none either — so the accessibility tree that `dogtail`/`pyatspi`
would drive on any GTK or Qt application does not exist to be driven. Appium and FlaUI are
Windows-only. Writing the driver is the standard answer for this toolkit on Linux, not a
workaround for having failed to find the tool.

**The division of labour is the design, and it is what makes the result trustworthy.**
`avalonia/frontend/UiDriver.cs` is an automation server inside the app that **resolves and reads**;
it does not synthesise input. It turns *"the Share button"* into a screen rectangle — the one thing
an outside harness cannot work out for itself — and **xdotool does the pressing, through the same
real X server the click fuzz uses.** A fabricated pointer event would be cheaper and would skip the
dispatch, hit-test and capture layers where the 2026-08-21 SIGSEGV actually landed, i.e. it would
be the thing under test lying about itself. If xdotool is missing the command **fails**; it never
falls back to invoking the handler, because a harness that silently stops testing input keeps
passing while covering nothing.

Controls are addressed by `AutomationProperties.AutomationId` — real accessibility metadata rather
than a private test channel, so the same names become useful the day a bridge exists. Text
selectors work against a frontend that has none, which is what makes adoption incremental instead
of a fifty-file prerequisite. **An ambiguous selector is refused, never guessed at**: `Button:Share`
matches both "Share a folder…" and "Share", and a driver that quietly takes the first match makes
a scenario's meaning depend on visual-tree order — that refusal is itself one of the fifteen checks.

**Three defects the spike found in its own instruments**, which is the point of running a thing
rather than reasoning about it:

- **A prose assertion was reading text nobody can see.** The first `alltext` walked the whole
  subtree, so it read the Sync panel's *"nothing is being offered to you right now"* out of a
  section that is deliberately hidden until an offer arrives. That is not a weaker assertion, it is
  a wrong one — it would pass against a panel that had stopped displaying the thing entirely. The
  walk now skips any subtree that is not effectively visible.
- **A failed redirection on `exec` is fatal to a non-interactive shell**, so the harness's
  `if exec 3<>/dev/tcp/…; then` retry loop could not retry: the first refused connection killed the
  script mid-line, with the container still running and no verdict printed. From outside it looked
  like the *app* hanging. Rootless podman accepts a connection on a published port before anything
  inside is listening, so that race is the normal case, not a rare one. The probe now runs in a
  subshell and is only believed when a `ping` comes back.
- **Artifacts were being written where the next build deletes them** (AP54 again). The run log,
  frames and crash record defaulted into `dist-native/`, and `extract` — which every build target
  runs — does `rm -rf` on it. The run directory is now bind-mounted into `avalonia/run-logs/`.

**What a green run claims:** the real shipped binary, driven by real pointer and keyboard input at
coordinates it reported for named controls; panel state changing as a result; the operator's prose
read back out of the process; and a clean shutdown through the window's own `Closing` handler
afterwards. **What it does not claim:** two peers, a network, permissions, a restart, or anything
whatsoever about sharing. This is the *driver's* proof, not the flow's — `scripts/twopeer-gui.sh`
owes those and does not exist yet. Four of §10's uncovered layers are now reachable; none of them
are yet covered.

## §10 (2026-09-05) — we are not testing the thing being tested

**`make twopeer-sync` exercises `entity-shell`. The operator opens the GUI.** That is the audit in
one line. The new harness runs two peers in two containers on a real TCP network with real grants
and a real restart — and the Avalonia app has never been run that way. The GUI's own two-peer test
is same-process, loopback, and uses a **wildcard grant**, which per AP63 means it establishes that
the transport works and nothing at all about permission.

The mitigating measurement: `avalonia/bridge/share.go` is a thin envelope, so the business logic the
GUI runs *is* what the harness covers. What is uncovered is the cgo/JSON boundary (AP49's home,
which has shipped twice), the panel layer, the GUI's process lifecycle, and its default peer
config. **A containerised GUI harness is the top of the next session's list.**

**What the harness found and we fixed** (`64605f4`): deletes were never subscribed, so
`BlobResolveHandler`'s entire deletion branch was unreachable and a removed file stayed on the
receiver forever; the delete branch computed the *sender's* path where the write branch uses the
target's, which is the two-root-names trap in a third place; no sharing panel woke when a file
arrived, because the watch set covered declarations and not the layers the counts read; and
`FilesPresent` was printed as "on disk" when it is a tree count, beside a sweep that prints a real
filesystem walk under the same words. Each delete fix was control-armed alone and each alone is
still red.

**What it found and we did NOT fix** — all live: **an asymmetric restart loses the first change**
(restart ONE peer and the next file changed never arrives; restart BOTH and nothing is lost — which
narrows a two-day-old "cause unknown" to the *surviving* peer, and means the old stale-pool
refutation was made without a reproducer that reaches the shape); **a concurrent edit silently
destroys the receiver's file**, no conflict copy, no warning, where Dropbox and Syncthing both keep
one; **30 GB of unpruned state** on the developer machine, 27 GB of it logs, with no reset path
until now; and **the layout file admits duplicate panels**, which is why the operator's screen had
`sharing-status` on it twice.

**`bash scripts/entity-state.sh`** inventories every byte this product leaves on a machine and, on
an explicit `reset`, removes it — never touching a mounted directory, because a received folder's
bytes are the operator's. Run it with `--keep-identities` to empty the tree and stay the same peer.

The survey, the scenario catalogue (topology, lifecycle, file semantics, capability, GUI,
robustness), and a landscape comparison against fifteen years of shipped file-sync are in the
2026-09-05 handoff under `docs/status/`. **The honest summary of coverage: two peers, one folder,
one direction, one file operation at a time. Three peers has never been run. `Mode: both` has never
been run over a network. The GUI has never been run two-peer at all.**

## §0Z (2026-09-04) — nobody asks our renderers whether they can draw an embed

**Analysis session, nothing built.** Answering the L5 social-vocabulary review as the non-web
seat turned into measurements about **our own tree**, and the headline one is a defect we have
been shipping since the figure work landed.

**First, the thing that is NOT a defect, because getting this backwards would send us building the
wrong fix.** A renderer that cannot draw an HTML embed, or an SVG, is **fine**. Declining is a
correct outcome: an HTML embed is an HTML document with everything that implies, and nobody should
be obliged to become a browser engine to participate. The symmetry is the point — we could ship an
embed type a browser cannot render either, and it would decline, and that would also be fine.
**`format: "html"` on a site page already works exactly this way here**: we lower it to text and
*say on screen* that we did and what was lost (`workbench/body_display.go:93-100`). That case is
closed and correct, and the publisher's reason for HTML is worth recording since it was nowhere in
our tree — it was chosen for the book as the **lighter** option against PDF, a priced tradeoff
rather than a web default leaking in.

**The defect is that for an embed, nothing is ever asked.**

**A figure never reaches `APP-CONVENTION-EMBED`'s degradation ladder, because it stops being an
embed one layer above the renderer.** `EmbedsToMarkdownImages` rewrites
`::embed[fallback]{ref=x}` into `![fallback](x)` (`workbench/site_embed.go:120`) before anything
else looks at the body. After that there is no embed, no `media_type` and no handler dispatch —
there is a markdown image node. The spec's three-step ladder is unreachable by construction.
We did not invent this: we transcribed it from `entity-browser-rust`'s `embed_to_markdown_image`
on purpose, and **their doc comment names the reason as the HTML renderer** — the lowering exists
to make `pulldown_cmark` emit `<img alt src>`. It works in our GUI by luck, because Markdig models
an image as a `LinkInline` we happen to intercept.

**In the terminal there is nothing left to decline, so it prints the source.** `entity-shell`'s
`open` writes the display body verbatim (`shellcmd/cmd_browse.go:583`), and every figure comes out
as the literal string `![Entity Demo Figure — …](assets/figures/demo.svg)`. EMBED §6 step 3 says
*never dump raw source as visible text*; we do, for every figure, on every page. **The fallback
text itself is fine and would read well — the terminal is never offered the choice.**

**Three smaller ones from the same pass, all measured:**

- **`renderer-caps` and `renditions` are implemented nowhere in the cohort** (zero hits in our Go,
  our C# and `entity-browser-rust`'s Rust), and the cohort's own demo asset is an **SVG**, which
  Avalonia's raster-only `Bitmap` cannot decode (`BrowserPanel.cs:1179`). So on the reference demo
  site our GUI shows a caption and no picture. **Not drawing the SVG is fine** — adding an SVG
  package is a dependency decision we have deliberately declined. What is wrong is that §5.3 is
  the mechanism by which a publisher could have offered a PNG and a renderer could have declined
  the SVG *for a stated reason*, and it is implemented by nobody — so what happens instead is a
  silent absence, and an operator cannot tell it from a break. `V-CAPS-DECLINE` is **our** vector
  and nothing in our tree exercises it.
- **`fallback` is mandatory-non-empty in the spec and our parser accepts empty**, and says so in
  the struct field (`workbench/site_embed.go:42`). Same tolerance in the reference, transcribed
  deliberately. Routed rather than fixed unilaterally — a parser stricter than the reference is
  how a figure appears in one browser and not the other.
- **A narrowed publish drops entry signatures.** If the feed convention lands as drafted, an entry
  needs a detached `system/signature` at `/{author}/system/signature/{hex}` — a **different
  top-level prefix** from the entries. `Publish` narrows by `-prefix` and nothing else
  (`publish/publish.go:195`; `IncludePath`/`IncludeType` refuse, `:225-231`), and "detached" means
  the closure has no edge to follow. So the entries travel and their authorship does not. Routed
  as the substrate item.

**One thing that came out well, worth recording because it says the spec's basis is right.**
`MarkdownRenderer.EmitImage` builds `StackPanel{Image, caption}` — which is the spec's
`box{layout:"figure", children:[image, text(caption)]}`, arrived at independently by someone not
reading the spec, with no broken-image glyph on any of its four failure paths. **The output basis
survives contact with a non-web front end; the input lowering does not.**

**Priced while we were there** (throwaway harness, deleted): a detached signature is **14.2 µs**
and **255 bytes**, and `types.SignatureData`/`LocalSignaturePath` are already called in four
places in our SDK. Per-entry signing is not a cost worth designing around.

**The property to build against, and it is the whole scope of the fix we owe:** *a renderer that
will not draw an embed must be able to say which embed it declined and why.* That is unsatisfiable
today at any point in our pipeline, because `![alt](ref)` carries a reference and **no type** — so
everything downstream guesses from a file extension, which is literally what our GUI does
(`BrowserPanel.cs:1187`, a substring match on `"svg"`). The fix is to stop flattening the directive
and carry a typed node to the renderer. **In a terminal the right answer is usually "HTML embed —
not rendered here", printed, and that is a good outcome rather than a degraded one.**

**Two capability rows closed in the same pass.** The reciprocal-grant gate *can* never open — zero
calls to `MarkEstablishedViaRendezvousKey` in our tree, confirmed — **but marking our own path
would be a lie and, worse, asymmetric**: our rendezvous backend is discovery (its header says it
"surfaces candidates and stops"), the connection that follows is a dial-by-address, and the
acceptor would never mark itself, which is the one-sided-claim shape the design forbids. Of the
three dependencies a `Coordinator` needs, one ships, one is small, and one — `SRFLXGatherer` —
**exists ready-made with every dependency exported and sits under `cmd/internal/`, where no other
module can import it.** Routed to core-go. **The better question, routed to arch, is whether a
punch is even the right fix**: it is NAT traversal, and the problem is two laptops on a LAN that
can already reach each other. And on rings: all three variables measure **ring 1** while the
vocabulary is ring 2; one mechanism for both is correct and we are not splitting it, but
`Mode: both` is a ring-1 gesture wearing a ring-2 label.

The findings went back to the specification authors as an internal review packet (not published;
`docs/architecture/reviews/`, dated today). **Nothing here is built.** The embed declination gap is
the one that is a live defect in a shipped surface, and it is not fixed.

## §0Y (2026-09-04) — the two-peer flow found four defects, and the manual step may not need to exist

**Running the flow through the panel found four things no gate could see**, which is the whole
argument for running it. The one that matters most: **the first share between two machines was
invisible to the receiver.** The panel asked for offers from the DECLARED devices, and connecting
deliberately does not create a declaration — so a peer you had just connected to was not in that
list. It broke both gestures at once: you could not share with someone until you had already
shared with them, and a receiver saw no offer from anyone they had not already shared with. That
is the only case that matters on day one. The other three: an `await` inside a loop over a
collection the wake rebuilds (reliable on the accept path, contained only 8 times before the
process dies); **both gestures completing in total silence**, because the panel read a `note`
field the bridge replies do not have; and the panel **pasting a shell command at a GUI operator** —
`connect <peer-id> <this-peer's host:port>` — which is AP71's shape inside the GUI, guidance
correct for one surface shipped to another where nobody can act on it. Gate:
`SyncPanelTwoPeerTests`, two real peers on loopback, driven through the panel's own handlers and
**reading its prose**, which nothing else in the suite does.

**And then the finding that reframes it.** Absorbing arch's capability-surface review turned up
their A-5, which is about this seat: `sendReciprocalGrant` is gated on
`EstablishedViaRendezvousKey()`, `core-go` ships `ext/signaling/peerwiring.Coordinator` which marks
**both** halves (`coordinator.go:179` and `:211`), and nothing here uses it. Re-measured rather
than carried: **`MarkEstablishedViaRendezvousKey` has zero call sites in `entitysdk/`, so the gate
cannot open on any path we ship** — even though our rendezvous machinery
(`entitysdk/rendezvous.go`, `signaling.go`) is substantial and ours. **So the manual dial I spent
the session explaining may not need to exist.** That is D20 aimed at explanation rather than
construction: we grep the kernel before we build, and did not grep it before documenting a
limitation as permanent. AGENTS.md already carries the sentence for this — *a paragraph explaining
why a limitation is correct closes the question permanently* — and this is its second instance.
The fix is **not sized**; `peerwiring` needs three things injected by the caller. The finding is
that the gate can never open, which is certain.

**The vocabulary the operator said we lack, we have — it is just on no surface.** The V7 §3.6
four-tuple `handlers × operations × resources × peers` is *which extension · which verb · what ·
who*, the unit of management is a named bundle of them (a role, which
`entitysdk/role.go` already wraps), and no fifth axis may be added — that is ruled, because three
implementations had degraded named caps to *"has any token → allow"*. The sentence to design
against is **"[Alice] may [read] [Holiday photos] via [content + tree]"**, with the test that a
surface which cannot be expressed that way is reaching outside the authority model. Two
constraints to know before anyone builds a capabilities panel: grants and signatures are
**sensitive** and must not be rendered outside the operator role, and subscriptions under
`system/capability/` should be **rejected** unless the scope is operator-class — so a live
capabilities window is an operator-class consumer or a poll loop. **Checked: our watcher touches
`app/workbench/*` and `app/share/records/` only, so it is clear today.** Absorption, the routing
packet addressed to us that we had not read, and the tracking rows are in
`docs/status/ABSORB-2026-09-04-the-capability-surface-and-a-routing-packet-we-had-not-read.md`.

Green: `make -C avalonia test` 180/180.

## §0X (2026-09-04) — one folder across two peers, one panel for the job, and no refresh buttons

§0W's three findings are closed. The scoping in it over-estimated the first by a lot, and the
reason is the transferable part.

**A folder is one object now.** `workbench.FolderID(owner, root)` is the same string on every
participating peer. **Derived, not minted** — both sides already hold both halves, so there is
no wire change and no new field in `app/share/*`, which is APP-CONVENTION-SHARE's namespace
where a field would be a cross-impl coordination rather than a local edit. The receiving side
had been computing exactly this string all along; only the sharing side wrote the bare root.
Pre-S6 records migrate once, at bootstrap — not in the loop, because a control loop that
rewrites declarations on every pass is a different and worse thing than one that reconciles
substrate to them.

**It was two changes, not a rewrite, because we read the source first.** §0W proposed adopting
Syncthing's model. Reading Syncthing's own configuration docs *before* designing collapsed the
scope: their `<device>` is `id`/`name`/`address`/`paused`, which is our `DeviceData`; their
`<folder>` is `id`/`path`/a device list/a type, which is our `FolderData`; their `path` is
explicitly *"not sent to other devices"*, which is ours. One divergence and one dead field. The
lesson is not about Syncthing — it is that **"adopt X's model" is a scoping claim, and checking
it against X's actual documentation is cheap.**

**`Mode` is read now, and `IsLocal()` was standing in for it.** The reconciler branched on
*who created the folder* everywhere it meant *which way bytes flow*; origin is immutable and
binary, which is the mechanical reason `both` was inexpressible. `Publishes()`/`Receives()` are
the readers, `reconcileFolder` subscribes to every peer it receives from rather than to one
origin, and `direction <folder-id> <send|receive|both>` sets it. **An absent `Mode` means the
pre-S6 behaviour, never `both`** — the first draft defaulted to `both` and a test caught it,
because that would have started publishing folders an operator had only ever *accepted*, over a
grant that already existed. Generalised: **a field that gains meaning defaults to the OLD
behaviour, and when the two directions of the mistake are not equally recoverable, that decides
it.**

**The refresh buttons were one missing subscription, and the operator raised it before we did.**
Measured: **12 of 15 panels held a tree subscription; the 3 that did not were the 3 sharing
panels**, and they were the only ones with Refresh buttons. `share.go` had nine bridge exports
and no `RegisterWake`; `SharePanel`'s only wake was one it borrowed from the *peer-connections*
model for its discovery list, so the one thing that updated itself was the one thing not about
sharing. We never lacked the mechanism — seven models already used `OnPrefixChange`. **AP73: a
Refresh button on data that lives in the tree is a bug report about a missing subscription.**
Only the read is wired to the wake; the reconcile pass dials *and* writes to the tree, so wiring
it would make a dialer out of an open panel and wake itself forever.

**And the panel.** `Sync` is the two gestures and nothing else. Share creates the mount itself,
because "mount" is a mechanism that leaked into the UI and is the step no operator could
explain. Shared Folders and Sharing Status are **demoted to Diagnostics, not deleted** — each
answers a real question you reach for after something breaks. Not built, and stated so this does
not read as completion: a directory picker (the field takes a typed path), an "as of" timestamp
on the offer list (offers are a remote read and a stale list currently looks live), a
stop-sharing verb on the row, and discovery in the peer chooser.

Green: **`make test-each` all 10 suites**, and `make -C avalonia test` at 178/178.
`SHARING-DIRECTION.md` §10 is the full argument; AP72 and AP73 are in the charter.

## §0W (2026-09-04) — an operator shared a file, and told us what is still wrong

A file was shared between two machines by an operator working unassisted, and it arrived.
That is a first. What they said next is the work item, and three of their complaints turn out
to be one defect.

**A folder has no identity across peers.** `share` writes a folder record on the sender;
`accept` writes a different folder record, with a different id, a different root and a
different path, on the receiver — and **nothing joins them**. `FolderData.Mode` declares
`send` / `receive` / `both` and is **written and never read**: two writers, two readers, both
of which only put it in a status view. Nothing branches on it. So "share a folder with a peer"
means *publish my directory, they subscribe* — one direction — and the reverse is a second,
unrelated share pointing at a different directory. The operator's words were *"bilateral
transfer to different locations, but they don't have the same understanding"*, which is exactly
what the records say. Syncthing's answer is one idea: a **Folder ID that is the same string on
every device**, each device choosing its own local path, direction being a property of one
shared object rather than a different object per direction. That is the next piece of work, and
everything else is downstream of it. The dead `Mode` field is **AP67's second instance** — a
configuration field that nothing reads is a fiction — and the tell worth carrying is *a field
whose only readers are serializers*.

**And we built the instrument rather than the product.** `Sharing Status (declared vs. actual)`
answers *is what I declared actually working* — a question you reach for once something has
gone wrong. The flow this was supposed to be heading toward is two gestures and nothing else.
The operator noticed at once: *"thought we were going to build the one sync panel; looks like
you just built the diagnostic panel."* Correct. The panel is good and it stays; scheduling it
as the next step toward the flow disguised that it was not on that path, and the sequence now
says so.

The scale of the surface, counted rather than sympathised with: **five panels touch this one
job, twenty-two buttons across them, and eighteen shell verbs.** Every one was added for a real
reason — most are the seam of a defect this project actually hit — and that is the trap: each
was locally justified and the sum is unusable. The next piece of work opens with an audit that
asks, per control, which of the two gestures it serves and what its presence costs someone
doing this for the first time.

Why the bar is this high: the plan is for the Rust browser, the Godot frontend and the Python
implementation to carry this pattern, and this repo is the furthest along. **A model that is
wrong here gets exported four times.** So the second share kind — the one that would prove the
adapter seam generalises to a revision project or a CRDT document — has been moved to *after*
the model is fixed, on the grounds that generalising a layer before it means what it says
generalises the defect rather than the design.

What is genuinely established, said plainly because the list above is long: two machines, one
folder, files across, live changes propagating, accept creating and mounting the directory in
one action, and restart survival with one known and characterised exception. The foundation
holds. What is missing is that the object the operator thinks they are manipulating does not
exist yet. `docs/architecture/SHARING-DIRECTION.md` §9 is the full argument.

---

## §0V NEW (2026-09-03) — we drove the whole flow for real, and it found three things

S4 shipped green. Then we ran the two-machine share end to end with the shipped binary — two
peers, separate homes, real listeners — and wrote the walkthrough by transcribing what actually
happened (`avalonia/README-SHARING.md`). **The flow works**: mount → share → accept into a
differently-named directory → existing files transfer → live changes propagate. Real bytes,
correct content, no manual mount on the receiving side.

Running it also found three things a full green sweep had not.

**A verb was still printing the instructions for a flow we had deleted.** `share` told the
operator on the other machine to `mount` first and then `accept` without a directory — the
pre-S3 sequence. Following it reintroduces the exact coupling S3 removed, because an `accept`
with no directory falls back to a mount named after the sender's folder. The operator doc had
been corrected; the program had not. That is now **AP71**, and the rule is that **printed
guidance is a surface with no reader in the test suite** — when you delete a step, grep the
program's output for it, and run the flow and read what it says.

**The address an operator types did not survive the process.** `connect` put it in shell state
and the kernel's pool, both process memory, so after a restart the reconciler had nothing to
dial with. It is now written to the peer's declaration — and **updates only, never creates**,
because connecting is a means and not a relationship. Alongside it, the loop now opens *our own*
outbound connection once per process and says so, since a connection has a direction for
authority and none for display: the pool reports "connected" over a route we cannot dispatch on.
The REPL's startup pass also prints what it *did*, not only what failed; re-establishing things
at every start is the entire point of the loop and it was doing it silently.

**And one defect we could not fix, characterised precisely rather than hand-waved.** After
either peer restarts, the **first** change is not delivered; every change after it is. No error
on either side, `status` reporting `settled` throughout. It is not a delay — the change is gone,
and only a pull recovers it. Two hypotheses were tested and both refuted by measurement, and the
second fix was **reverted rather than kept**, because a disconnect on every startup justified by
a dead hypothesis is a cost with no benefit. Routed with the full reproduction, the counted
runs, and the questions that remain, in
`docs/architecture/reviews/FIRST-CHANGE-AFTER-RESTART-IS-LOST-2026-09-03.md`. The product states
the limitation and the workaround rather than hiding it — a flow that looks reliable and is not
is the same failure as a surface reporting `settled` while nothing works.

The honest summary against the bar we set ourselves: **good enough to use deliberately, not yet
good enough to forget about.** The relationship model is right and is genuinely order-free; the
restart behaviour and silent conflict loss are what stand between this and something you would
trust a directory to unattended.

---

## §0U NEW (2026-09-03) — the control loop got a face, and what it is allowed to claim

**S4.** `Sharing Status (declared vs. actual)` — the third Network panel, and the honest
description of it is not "a new panel" but **the missing half of the control loop**. Before it
there was no bridge export for devices, folders or the reconciler at all: everything the
declared-state layer does was reachable from one shell verb and from no pixel. Meanwhile the GUI
*runs* a reconcile pass at every startup and reported its problems to **stderr** — a run log
nobody has a reason to open. An operator whose accepted folder had lost its mount was told so,
correctly and in detail, somewhere they would never see, while every panel on screen looked fine.

**The reachability sweep passed throughout and was right to.** It asks whether a
`workbench/*_model.go` has a surface; the reconciler is `shellcmd`'s and it has a verb. That is
the second instance of this blind spot in a different shape — the first was a cross-peer handler
built, tested and registered nowhere — so it is now **AP69**, and the question it says to ask
when scoping any non-model layer is *which shipped binary reaches this, and by pressing what*.

**Reading and re-checking are separate operations, and the panel says which it is showing.** A
reconcile pass dials every declared peer, so a panel that reconciled on refresh would be a
dialer somebody leaves running overnight. `StatusSnapshot` reads records and observes the
substrate; `Reconcile` runs the loop; they share one observation pair so a read and a pass cannot
describe the same device — or the same fault — differently. Which one produced a table rides in
the **outcome**, not in the caller's memory, because a surface that has to recall which function
it called in order to caption its table will eventually caption it wrong, always in the confident
direction.

**Outbound authority is stated exactly; inbound is not stated at all.** What we grant a peer is
our own capability row, so it is printed verbatim. What they grant us is in *their* table, which
this peer cannot read — it is only ever observed, through deliveries arriving. So the inbound
direction renders as what has actually landed (both layers of the mount, and an explicit
*unknown* when there is no mount to count) and **never as a health dot**. A dot there asserts
something about another machine we have no way to check, and it is wrong in exactly the case that
matters: they revoked us and we have not tried since. There is a standing gate on that, and its
instruction on failure is to delete the offending field rather than update the test.

Two verbs came with it, because a panel with no verb in it is the tell for a read-only surface
over a read-write model: **Pause/Resume**, which writes the *declaration* so the loop obeys it
rather than being fought by it, and **Remount**, which is the one action the reconciler names and
deliberately refuses to take — it will not write to a directory on somebody's disk. Remount takes
no directory argument: the path comes from the declaration, which is the only record left that
can say where a folder's files are once the mount that knew is gone.

Gates: `shellcmd/status_test.go` (eight, four verified red with the behaviour removed — the
read-does-not-write one runs the reconcile pass as its control arm on the same fixture, because
otherwise it is satisfied by a function that does nothing) and
`avalonia/tests/.../SharingStatusPanelTests.cs` (nine, including the geometry check that
distinguishes a rendered button from a clipped one).

Not built, and named rather than left implied: **pending offers still have to be pulled.** A card
that appears unprompted needs either a poll of every declared device on a cadence (which looks
like a hang when a device is asleep) or a subscription to their offer records, which needs a grant
we do not currently ask for. Shared Folders keeps the pull, and this panel does not imply a
liveness it does not have.

---

## §0T NEW (2026-09-03) — accept takes a directory, and the coupling nobody had written down

**S3.** `accept <peer> <root> [<directory>] [-anyway]` now creates the directory, mounts it,
authorizes the sender's deliveries, subscribes and pulls everything already in the folder — one
action. The GUI asks on the offer row, pre-filled with a fresh path under `~/entity-shared/`.

Two steps of the nine-step flow are gone. The obvious one is the receiver's separate `mount`.
The other was not written down anywhere: **a mount root is derived from its directory's
basename, and the sync was keyed on a single shared root name, so the receiving directory had
to be named exactly what the sender happened to call theirs.** Nothing said so, and picking a
sensible local name produced a relationship that established cleanly, reported the right target
prefix, and delivered nothing. A received folder now records both names —
`FolderData.LocalRoot` beside the sender's `Root`, read through `ReceivingRoot()`.

**The refusal is the interesting part of the design.** Accepting into a directory that already
has files in it is destructive: incoming files overwrite same-named local ones and remote
deletes propagate. So the operation refuses, as a typed value carrying the path and the count,
and the GUI renders that as a choice with the consequence stated — not as a failure, and not as
a pre-authorized default. The pre-filled path is deliberately one that does not exist, because
a default that trips its own safety check on every first use teaches people to click past it.

Gates: `shellboot/accept_directory_e2e_test.go` — two peers, receiver mounts nothing, receives
into a differently-named directory, and the assertion is **bytes on disk**, because every
intermediate signal here is one this repo has already shipped a green version of over an empty
folder. Verified red two ways: without the target-root wiring the accept refuses, and with the
backfill disabled the file never arrives (the byte assertion polls its full 30 s and fails).
Panel side: `SharePanelAcceptDirectoryTests`, which asserts the field is laid out inside the
panel and that a typed path **reaches the model** — checked by requiring the directory to exist
afterwards, since asserting on the panel's own accessor would be circular.

Note for the next session: those are the first tests here to drive a `Perform*Async` panel
method, and the first attempt deadlocked the headless dispatcher —
`.GetAwaiter().GetResult()` on the test thread blocks the UI thread that the continuation needs.
Use `[AvaloniaFact] public async Task` and `await`.

---

## §0S NEW (2026-09-03) — the loop was right and three verbs had not been told

A review pass over §0R's work, before committing it. The design holds and the sweep is
green; three live defects were found in the seam between the new control loop and the
verbs that predate it, all the same shape, and all invisible to the tests that existed.

**The reconciler computed the policy union; the verbs still wrote their own half.** §0R
identified AP68 — `system/capability/policy/{peer}` is one row, `share` writes the sender
grants and `accept` writes the receiver grants over the top — and fixed it *in the
reconciler*, which recomputes the union on its next pass. The verbs were left alone. So the
clobber still happened and was merely **healed later**: at the next `status`, or the next
launch. That window is entered by performing the product's two gestures in one session,
which is the case the fix was written for. `shellcmd.ApplyDeclaredPolicy` is now the single
writer and both verbs derive from the declaration. The lesson worth keeping is general:
**a control loop that corrects a bad write is not the same as removing the writer**, and no
test that runs the loop can tell the two apart — which is why the existing gate seeded the
row "the way the verbs leave it" and stayed green.

**`unshare` did not survive its own reconciler.** It removed the policy row and never
touched the folder declaration, so the next pass read `offered` and wrote the grant back:
the withdrawal held until the operator restarted and then silently reversed itself. This is
a defect the loop *creates*, and it generalizes — **once a reconciler exists, every verb
that changes what the operator wants must write a declaration, or the loop will undo it.**

**And `unshare` dropped the whole row.** Its test for whether to revoke was "is this peer in
any remaining offer", and offers describe only the outgoing direction — so unsharing the
last folder from a peer who is also *sending* us one deleted their delivery grant too. The
predicate was deleted rather than left beside its replacement.

Gates: `shellboot/share_union_e2e_test.go` (drives the two shipped verbs and reads the row
they leave — asserting the reconciler's output would have tested the healer) and
`shellcmd/declare_test.go`. Every one verified red with the fix genuinely removed. **One of
them was vacuous first** and the control arm is what caught it: it drove `Share` with an
unmounted root, which is refused *before* anything is declared, so the unwind path it was
written for never ran.

Also fixed: `make test-each` did not clear `.test-logs/` before a run, so a sweep in
progress left a mixture of this run's logs and the last one's with nothing to tell them
apart. Read mid-sweep during this review, a stale `FAIL` from a defect fixed hours earlier
read as a live failure — and it is the more alarming reading, so it wins. Same rule as
`bin/`: build output is not the tree.

Verified: 10/10 Go suites, Avalonia 160/160, vet clean, reachability clean, `make build`
clean. (`shellcmd`, `shellboot` and `shell` re-run after the `unshare` fix, which touches no
other package.)

---

## §0R NEW (2026-09-03) — four reasons nothing survived a restart, and the reconnect engine we never called

An operator spent two days trying to share a folder between two machines and never got a file
across. The previous session found one hard defect and wrote the redesign brief. This session
priced the redesign against the substrate first, and the pricing is the finding: **most of what
we were about to build already exists, and the reason the flow does not work is that almost
nothing in it was designed to survive a restart.**

### 1. Four independent restart killers, and they stack

Each of these alone makes the whole flow fail after a relaunch. Fixing any one in isolation
would have changed nothing observable, which is why two days of correct button-pressing
produced nothing.

1. **The GUI's default peer was ephemeral.** No identity, memory store, fresh keypair per
   launch. The tree is peer-id-namespaced, so this was not "some features off" — the app was
   **a different peer every time it started**, and every grant, mount, offer and accepted share
   from the last session named a peer-id that no longer existed. An operator debugging by
   relaunching was destroying the state they were debugging, on both machines, every time.
2. **The GUI's default peer did not listen or announce**, and discovery is disabled entirely
   without a listener. The default configuration could not be reached, found, or shared with.
3. **`localfiles.Handler.Load` restored no mounts** — the kernel AP58 instance found last
   session, worked around in `workbench/localfiles_root_restore.go`, routed in
   `reviews/LOCALFILES-LOAD-RESTORES-NOTHING-2026-09-03.md`.
4. **Nothing re-established a peer relationship at startup**, because nothing called
   `maintain-peer` (§2).

And a fifth, found while pricing: **`entity-shell -listen ADDR` bound no socket at all** (AP67).
The address went `shell.Config` → `shellboot.Config` → `entitysdk.PeerConfig` →
`peer.WithListenAddr`, and core-go reads that field in exactly one place — `Peer.Listen` —
which only `PeerManager.Create` ever called, and `entity-shell` does not use the manager. A
documented flag, step 1 of the operator recipe, doing nothing, for months, **with every e2e
suite green**: each stands its own listener up with a local helper rather than going through
the frontend's startup. That is AP63's shape at the level of a startup step — *a fixture that
reimplements part of the product's startup deletes that part from the suite.*

### 2. The reconnect engine has been in the kernel the whole time

`system/network:maintain-peer` connects, installs the §4.1 reconnect continuation graph,
**retries forever** with derived backoff, and restores subscriptions on reconnect. The handler
is registered by default. `entitysdk.NetworkClient.MaintainPeer` wraps it.

**It had zero callers in shipped code** — ten references in this repo, nine inside its own file
and one in its own test — while three of our verbs called `AppPeer.Connect` once and treated
that as a relationship. So *"why do I have to press connect again"* has a one-line answer, and
the answer is not that the feature is hard.

Two more of the same shape, confirmed by grep rather than assumed: `ext/signaling/peerwiring`
(symmetric establishment, unused) and `localfiles.RootConfigData.ReadOnly` (Syncthing's "Send
Only", honoured by the kernel's writeback path, settable from nothing we ship). D20 says price
against the substrate; the correction this session adds is **to point it at our own SDK too.**

### 3. The structural error: a wizard where the substrate wanted a controller

`Share` → `Accept` → `CompleteShare` are one-shot sequences, each encoding the four AP63
authorization facts inside a button press. That is order-dependent, non-idempotent, and has no
representation of what it established — so nothing can re-establish it. Underneath were **five
disconnected durable records** (roster entry, policy row, offer record, mount binding, sync
binding) written by five verbs and restored by four different startup paths, and **none of them
was the relationship.**

Now: two declared records — `app/workbench/devices/{peer-id}` and
`app/workbench/folders/{folder-id}` (`workbench/desired_state.go`) — and one idempotent
reconciler (`shellcmd/reconcile.go`) that derives everything else. `share` and `accept` keep
doing what they did and additionally declare; startup, the new `status` verb, and the GUI's
background pass all run the same loop. Full argument in
`docs/architecture/SHARING-DIRECTION.md`, including the Syncthing model this follows and why
Dropbox's is not available to us.

### 4. What writing the reconciler found: two-way sharing could not authorize (AP68)

`system/capability/policy/{peer}` is one row per peer. `Share` writes the sender grants to it,
`Accept` writes the receiver grant to the same path, and `SaveAccessPolicy` replaces rather than
merges — **correctly, by its own documented contract**, which reasons from "one caller, one
statement of intent". With two callers that reasoning is false: share a folder to a peer AND
accept one from them, and the second write silently revokes the first. Invisible because
`sync_twoway_e2e_test.go` — the test written specifically for two-way — runs under
`OpenAccess: true` and says so in its own header, and a wildcard never consults the row. **AP63
one instance on from the instance that named AP63.**

The fix is not a merge. It is that a shared row gets exactly one writer which derives it from
every declaration that contributes to it.

### 5. Landed, with its gates

- **Iteration cost.** Podman cache mounts on every compile step in `avalonia/Containerfile`; a
  rebuild after a source edit is **~13 s** where it previously re-downloaded the Go module graph
  and the NuGet graph, including the self-contained runtime packs, every time.
- **Build stamp.** `git describe` plus a hash over the working-tree diff — stable for an
  unchanged tree so the layer cache still hits, distinct for any edit. Compiled into **both**
  the frontend and the bridge, printed at startup, written to the crash trail, and **in the
  window title**, because the way this information actually travels is a screenshot. A
  frontend/bridge disagreement is reported as a MISMATCH: that is the one state `extract` can
  produce and nothing could previously report, and it cost a day on 2026-09-03.
- **Defaults.** The GUI is a persistent, listening, announcing peer; `--ephemeral` restores the
  old throwaway. `BringUpListener` (`shellboot/listener.go`) is the single bring-up — bind,
  advertise, announce — shared by every frontend. A wildcard bind now advertises this host's LAN
  address instead of publishing no profile at all.
- **The loop**, with `status` as its shell surface.

Gates, each verified red-then-green with the fix genuinely removed:
`shell/listen_test.go` (dials the reported address from a second peer; **two** control arms —
bring-up removed, and bring-up faked with a plausible address and no bind),
`workbench/desired_state_test.go` (peer-backed store; the AP58 control arm turns it red),
`shellcmd/reconcile_test.go` (the union, and idempotence — the control forces the grant
comparison to always differ, which reconnects every peer on every pass), and
`shellboot/reconcile_restart_test.go`, which is the one that matters: **across a process
boundary, on the derived structure, asserting not-connected before the pass and connected
after**, because only that pair of facts distinguishes re-establishment from a connection that
never dropped. Its control arm swaps `maintain-peer` for the old single `Connect` and it fails.

### 6. Not done, and named

- **The Sync panel** (`SHARING-DIRECTION.md` §7, S4). The loop has a shell surface and no GUI
  one yet. The three existing panels stay — they are how we understand the system — and what
  becomes of them is a question for after the flow works, not before.
- **Accept-with-directory** (S3): one action that mounts, accepts, syncs and backfills.
- **Cross-NAT.** Everything here assumes the machines can reach each other. `peerwiring` plus a
  signalling carrier is the answer for the internet case and is deliberately not scheduled: it
  needs infrastructure deployed, and it would not have fixed any part of the reported failure.
  The previous handoff nominated it as *the* lever — right about the mechanism, wrong about the
  priority, because on a LAN the reconciler plus discovery removes the address problem without
  it.
- **Peer identity on every panel**, and the `gui-layout.json` / `browser.json` migration into
  the tree. The layout file's stated reason for being a file was a consequence of the ephemeral
  default and no longer holds.

### 7. A process note worth keeping

`make test-each` and `make -C avalonia test` were run concurrently during this session. AGENTS.md
says never to, and the reason is exactly what happened: both bind-mount the tree with `:Z`, the
second relabel revokes the first container's access mid-run, and three suites reported
`Permission denied` on their own log files while the tests inside them had passed. **A sweep
that goes red for a reason outside the tree is worse than no sweep** — it is indistinguishable
from a real failure at a glance, and it cost a re-run. The rule was already written down.

## §0Q (2026-09-02, fourth pass) — the file share that shared no files, and the kernel line nobody read

Two defects, found from one operator report — *"we're trying to debug this on two computers and
it keeps crashing, and I can't get the mounts working"*. They are unrelated in mechanism and
identical in shape: **in both, the artifact that named the cause was already on disk and had been
read past.**

### 1. The crash: the render thread was on 16 KB, and the kernel said so

`journalctl` for the crashing pid:

```
kernel: signal: entity-avalonia[3746566] overflowed sigaltstack
```

One line, unambiguous, and it settles what §0J left open. The process pid is 3746552; **3746566
is a secondary thread** carrying `ServerCompositionContainerVisual::Update` and `libSkiaSharp`
frames — the render thread. A SIGSEGV arrived there, CoreCLR's handler chain ran on the PAL's
stock **16 KB** alternate signal stack, overflowed it, and the process died unrecoverably: no
minidump, no crash log, and a breadcrumb trail that just stops.

`CrashDiagnostics.cs` had predicted this in a header comment since 2026-09-01 — *"the 2026-09-01
fault landed on a background thread, which still has the PAL's stock 16 KB … **Nothing in this
file fixes that**"* — and `AGENTS.md` carried it as a known open gap. So the diagnosis cost
nothing and the fix was never scheduled, because the paragraph explaining why the limitation was
correct had closed the question. **That is AP45's lesson pointed at ourselves for the second
time**: a `TODO` invites work, a well-written rationale ends the discussion permanently.

It also retires §0J's *sharpest open question*. That entry recorded, as an untested hypothesis,
that "createdump is on and produces nothing for this fault class … by the time the process faults
there is no stack left for createdump to run on." That is now **measured**, by a field nobody had
looked at, in a journal that had been sitting there for ninety seconds when the session started.

**The fix.** `sigaltstack` is per-thread and must be called **on** the thread it covers, so
something has to run on the render thread. Nothing tidy is available in Avalonia 11.2 —
`IRenderTimer.Tick` is raised there and is `internal`, `Compositor`'s update callbacks run on the
UI thread, and `AvaloniaLocator.Current` is gone. What does work is an `ICustomDrawOperation`:
serialized into the render command stream and executed by the compositor during the render pass,
i.e. **in the same call stack the crash was taken in**, by construction rather than by inference.
`AltStackProbe` is a zero-size, hit-test-invisible control that does exactly that and nothing
else. Measured on a real X11 run, before and after, on a different thread from the UI thread:

```
altstack: at startup:  sp=0x7f34ff4dd000 size=16384    flags=0   (UI thread)
altstack: render thread (managed tid 4): before: size=16384      <- the crashing configuration
altstack: render thread: after:  sp=0x7f30401a2000 size=1048576
```

**Gate:** `run-xvfb-smoke.sh` now fails (exit 3) if that line is absent, because this is the only
harness in the repo that can see it — the headless suite has no render thread, and there is no
cross-thread query for another thread's alt stack, so *the install having run there* is the only
evidence obtainable. The control arm is real: the same grep against the crashed run's own log
(`run-20260902-201920.log`) finds nothing, so the gate would have failed the exact build that
died. `WB_ALTSTACK_BYTES=0` is exempted, since it is the documented A/B arm.

**What is still NOT fixed, and must not be read as fixed.** The *original* fault inside the
compositor walk is still unidentified, exactly as §0J left it. This makes the fault
**survivable and legible** — a 16 KB overflow produces a corpse, a 1 MB stack produces a handler
that can run — and the next occurrence should finally arrive as something with a stack trace. It
is not a fix for whatever faults. And only threads we can reach are covered; the external
`createdump` path stays armed for the rest.

### 2. The file share did not share files

The operator's other report — *"I haven't been able to get the mounts working, the
synchronization"* — was correct, and the cause is one line of `Sync`:

```go
entitysdk.SubscribeOpts{Events: []string{"created", "updated"}, IncludePayload: true}
```

**A subscription is a future tense.** It reports what happens after it exists. So a folder that
already had files in it transferred **nothing**, because no file in it ever changed again — and
the gesture the product is named after (pick a directory, share it with a peer) produced an empty
folder, no error on either side, and a `syncs` row reporting the relationship as healthy.

This was **documented**, which is why it survived. `USAGE-SHARE-A-FOLDER.md` listed *"history
replay — files already sitting in A's folder arrive when they next change, or when A remounts"*
under what-this-does-not-cover; the `sync` verb printed the same sentence; and
`sync_e2e_test.go` writes its file **after** the sync, deliberately, with a comment explaining
that the subscription does not replay history. Every artifact was accurate about the
implementation and every one of them described a product that does not do the thing it is named
after. **A known-limitations entry is not a substitute for the feature**, and prose that explains
why an absence is correct is read as a decision rather than as a defect.

**The fix reuses the live path rather than paralleling it.** `shellcmd/sync_backfill.go`
enumerates the remote prefix and, per file, synthesizes the notification the subscription engine
would have delivered, dispatching it at the same `workbench/blob-resolve:receive` handler through
`Executor.ExecuteWithIncluded` — which is the shape a live delivery already arrives in, the
changed entity presented in `HandlerContext.Included`. Mount lookup, the F9 already-current
short-circuit, the capability-checked cross-peer blob closure pull and the `local/files:write`
materialization are therefore shared **by construction**. Ordering is load-bearing: subscribe
first, then backfill, so a write landing during the catch-up is carried by the subscription; the
overlap is harmless because F9 makes a duplicate a no-op.

**A correction to the flow's own story, and it is not a softening.** `accept` used to end with
*"until they dial you, this folder stays empty"*. The backfill runs entirely on authority the
receiver holds — it dispatches to the publisher for the listing and the closure, which `share`
already grants — so **nothing in a first transfer requires the publisher's dial**. What requires
it is *delivery of future changes*. The old text was telling operators their files would not
arrive while their files were, by then, already on disk.

**New verbs.** `resync <peer> <root>` re-runs the catch-up without touching the subscription —
the "pull now" affordance, and the only positive confirmation available anywhere in this flow:
run it twice and the second pass reports everything **already current**, where otherwise "no
errors" and "nothing happened" render identically. `forget <peer>` / `forget --all` drops syncs,
offers, authorization and the connection, because everything the flow establishes is deliberately
durable and the sum of that is a flow that **cannot be re-tested** — a second run is
indistinguishable from a stale grant, and that failure presents as a success nobody can trust. It
deletes no files and unmounts nothing. Both are in the GUI: **Pull now** and **Forget peer** on
each received folder's row, **Forget all peers** in *This peer*.

**Gate:** `shellboot/sync_backfill_e2e_test.go`, through `shellboot.Bootstrap` and the shipped
verbs, seeding files **before** the mount so they enter the tree via the watcher's initial scan
and are never the subject of a change event. It has a **control arm** —
`TestBackfill_ControlArm_WithoutItNothingArrives` runs the same scenario with `SkipBackfill` and
asserts the folder stays empty — so the positive test cannot pass on some other mechanism
replaying history, which is precisely the vacuity the sibling test had.

### Two-way was the composition of two tested halves, and nothing tested the composition

The operator's model is Dropbox — one folder, two machines, contents converge. Everything gated
here was **one direction**: `sync_e2e_test.go` establishes A→B and asserts a file written on A
lands on B. Two-way was *supported* — `workbench/blob_resolve.go`'s F9 content-hash short-circuit
exists specifically to stop the notification loop the "bidirectional symmetric topology" creates —
and exercised only by `shellcmd/cmd_local_files_bidirectional_test.go`, which hand-assembles the
chain instead of going through the verbs. So the thing the product is *for* was the composition of
two separately-tested halves, and the composition had no gate.

`shellboot/sync_twoway_e2e_test.go` closes that: both peers mount, both seed files **before**
either sync exists, both `Sync`, and every file must land on both disks with live delivery still
working in both directions afterwards. It passes.

**It also caught something on its first run, and the finding is the assertion rather than the
code.** The test expected each backfill to scan 2 — the other side's files. Bob's scanned **4**.
That is correct: by the time bob syncs, alice's backfill has already pulled bob's files into
alice's folder, so bob enumerates alice's folder and sees both sets. The reported outcome was
`4 file(s) found, 2 transferred, 2 already current` — the F9 short-circuit recognising bob's own
bytes and declining to pull them back. **The loop guard is visible in a count**, and the test now
asserts what must hold (each side sees at least the other's files; the two backfills materialize
no more than the number of distinct files that exist) rather than a number that depends on which
sync won a race. A regression in F9 shows up in that bound before it shows up as churn.

The audit also added the case that would have shipped broken: a file in a **subdirectory**. The
remote enumeration is a breadth-first walk that must descend, and a walk returning only the top
level would have passed every other assertion — a partial transfer reported as a complete one,
which is the original defect wearing a smaller hat. The gate asserts the nested file arrives *at
its nested path*, and distinguishes "missing" from "arrived flattened into the mount root",
because those two failures have different causes and the same symptom.

### The finding that spans both

Neither defect needed an experiment. The crash needed `journalctl`; the sync needed the last
section of our own usage doc. Both had a **written, accurate, in-tree artifact naming the cause**,
and in both the artifact was phrased as settled — a rationale in a header comment, a
known-limitations bullet. D25 says *a field that prints is not a field that answers*. The
extension this pair earns: **an explanation that closes a question is a claim with no expiry
date, and nothing re-opens it.** Grep your own tree for sentences that explain why something
cannot or need not be done, and treat each as unmeasured until it is re-measured — they are the
cheapest place in the repo to find a live defect, and the least likely to be looked at.

**State.** Avalonia headless **160/160** (was 157 — three new boundary tests).
`smoke-xvfb-click` green with the new gate (200 gestures, drag 40%, exit 0).
`make test-each` **9/10 — `sdk` is RED**, and it is not this session's:

```
sdk FAIL 329s · inspect PASS · shell PASS · shellboot PASS 97s · shellcmd PASS 307s
shellpanel PASS · workbench PASS · programs PASS · publish PASS · fetch PASS
```

Run twice, on a settled tree both times, with byte-identical failures — the first attempt was
discarded because it was started while the tree was still being edited, and a sweep over a
changing tree is not a measurement.

```
--- FAIL: TestAxis1Equivalence_Differential
--- FAIL: TestAxis1Equivalence_ContainedErrorPositions/map-element-contains
--- FAIL: TestAxis1Equivalence_ContainedErrorPositions/fold-accumulator-contains
    stage1: error(index_out_of_range: …)   axis1: value:uint64:04
```

**Not bisected to a commit, and deliberately not called "pre-existing" on vibes** — what is
established is narrower and sufficient: `entitysdk` is **unmodified** in this working tree
(`git status`), it imports neither `shellcmd` nor `shellboot` (the dependency direction forbids
it), and the failure reproduces **alone, unloaded, in 0.02 s**, so it is not the load-dependent
class this file warns about. Neither of the two commits since §0P's "10/10" claim touched
`entitysdk/axis1/`, so **§0P's green line and this red are not reconcilable** and one of them is
wrong about the tree; that is worth someone's attention on its own terms.

The shape is **AP43's exact territory** — the CONTAINED positions, where `map`'s output element
and `fold`'s accumulator must *place* an error as a value rather than short-circuit. Axis-1 is
returning the plain value where Stage-1 raises. The commit that fixed this class is the one whose message reads
*"fix(axis1): contained-error position semantics, and an admission gate that was never
running"* — cited by content rather than by hash, because this file publishes and an internal
SHA resolves to nothing on the public mirror ([ADR-0012] Am. 1).
**Left alone on purpose**: it is the compute engine, it is nowhere near a folder share, and
opening it at the end of a session whose subject is the file-sharing flow is how the sweep ends
up red in two places instead of one. Flagged, reproduced, and handed on.

## §0P NEW (2026-09-02, third pass) — the whole flow, and the wildcard that was hiding a stage

The ask was to make discover → share → permission → mount → change/sync one seamless process, and
validate it before multi-device testing. It is validated, end to end, with **no wildcard grants
anywhere** — and turning the wildcard off is what made the afternoon interesting.

### Every cross-peer test we have ever run was under a wildcard

Twenty-four of them, all `peer.OpenAccessGrants()`. That authorizes everything, so the entire
cross-peer suite establishes that the bytes move and says **nothing** about permission — the one
stage an operator actually has to perform. The kernel's per-peer mechanism, the V7 v7.62 §8
handshake policy table at `system/capability/policy/{peer}`, had **zero uses in this repo**. What
we had instead was `shellboot.Config.OpenAccess`, whose own doc says development-only.

The generalisation is the part to carry: **a permissive fixture does not weaken a test, it deletes
a stage of the product from the suite** — and the deletion is invisible, because everything
downstream of the disabled mechanism passes. The count of tests covering that mechanism is zero
however many run through it. AP63.

### Four things the wildcard was hiding

All measured (`shellboot/policy_probe_test.go`), each with its own arm, and each one an afternoon
lost if you meet it on real hardware instead:

1. **A sync is MUTUAL authorization.** The receiver dispatches *into* the publisher to subscribe
   and to fetch blobs; the publisher's subscription engine dispatches the notification *back*
   into the receiver's `blob-resolve`. Both cross a capability boundary. Grant one direction and
   the subscription is **accepted** and no file ever arrives — indistinguishable from a working
   share until someone opens the folder. Every prior test hid this because the receiver was
   always wildcard, **including the one written specifically to close the capability-delegation
   gap**, which scopes the sender and leaves the receiver open on purpose.
2. **The grant is assembled at HANDSHAKE.** A policy written on a live connection is inert until
   that connection is re-established.
3. **The peer that DISPATCHES is the peer that must reconnect.** A reconnect by the granter does
   nothing for the grantee, who dispatches over the connection *it* opened.
4. **A dial-by-address authorizes the DIALER ONLY.** `sendReciprocalGrant` is gated on
   `EstablishedViaRendezvousKey()`, and the kernel says why: *"a dial-by-address is asymmetric —
   one party requested service."* So a two-way sync needs **both** peers to dial, each after the
   other's policy exists — and the final dial is a step neither verb can perform for the other.
   `accept` prints the exact command for the other machine, because the symptom otherwise is
   silence on the side that cannot see the refusal.

### What shipped

`peers` (discover), `share` / `unshare` / `shares`, `offers`, `accept`, `access`. Two of these
stages had no surface at all before today: `entitysdk/share.go` implements APP-CONVENTION-SHARE in
full and was called from `share_test.go` and nowhere else, and the policy table was untouched.

Three decisions worth recording:

- **The authorization is the policy table, not `AuthorShare`'s minted token.**
  `ShareWithdrawalNotice` states that a `system/capability:request`-minted token is **not
  recallable** — no tree write, so the granter never holds its hash and `revoke` cannot name it.
  Built on that, `unshare` could not end access and would be lying by its name. A policy entry is
  a tree write we own.
- **The offer record is a label, not an authority**, per §2.2 — so seeing an offer and still
  getting a 403 is the two things being correctly separate rather than a defect.
- **`access` marks the peer's own row.** The kernel seeds a `*:*` entry keyed on the peer's own
  identity hash (§6.9a). Unlabelled it reads as a wildcard grant to a 66-character stranger;
  hidden it would conceal a real grant. It is shown and named.

### Two bugs found in our own new code, both by the test rather than by review

**A reconnect that could leave the peer disconnected.** The first version tore the connection
down and *then* discovered it had nothing to dial — strictly worse than doing nothing, because it
turned a share into an outage. It now resolves an address first and leaves the connection alone
if it has none.

**`ConnectedPeers()` is not a source of dialable addresses.** Its `Address` is the connection's
*observed* remote address, which for an **inbound** connection is the dialer's ephemeral source
port. It looks exactly like an address — `127.0.0.1:50026` — and dialling it is refused. The two
honest sources are the address we recorded when *we* dialled (`connect` writes it) and the peer's
own mDNS announcement, which advertises the port it listens on.

### One extraction

`chooseDialAddr` / `pickDialHost` / `parseTXTPairs` lived in the Avalonia bridge. The shell's
`peers` verb was about to be the second consumer, and the dial-address choice is a substrate
judgement about mDNS announcements rather than anything a renderer owns — so it moved to
`entitysdk` and the bridge calls it. *DRY the integration, not the renderer.*

### Verified

`make test-each` 10/10. `lint`, `textual`, `reachability`, `gofmt` clean.
`shellboot/flow_e2e_test.go` runs the whole flow across two peers with **no OpenAccess on either
side** — discover, share, inspect the permission, read the offer from the other peer, mount,
accept, write a file, **edit** it, then revoke. Its companion `TestFlow_ShareAloneDeliversNothing`
is the control arm and the more valuable of the two: it pins that the sender's half alone yields
an accepted subscription and an empty folder, so a later "simplification" of `accept` into a plain
sync fails there with the reason attached.

**Not done, named rather than implied:** no GUI surface for any of these verbs — the Local Files
panel manages mounts and says nothing about shares or syncs. Still one LAN or manual addressing
(`ext/relay` unlanded upstream). Concurrent edits to one file on two machines remain M3 and
remain unsafe to assume. The operator recipe is
`docs/architecture/USAGE-SHARE-A-FOLDER.md`.

## §0O NEW (2026-09-02, second pass) — M2, and the subscription that was dead after every restart

Two peers, one folder, end to end, through the shipped verbs. And on the way, a live defect that
had been in every build since the subscription wiring was written.

### `sync` — and the engine was already there

`sync <peer> <root>`, `unsync`, `syncs`. The receiving chain is *subscribe to their
`local/files/{root}/*` with the payload included → `workbench/blob-resolve` → pull the blob
closure across → dispatch `local/files:write` locally*. Durable half at
`app/workbench/syncs/{peerID}.{root}`, restored by `shellboot`.

**The reason this was a wiring job and not a build is the finding.**
`workbench.BlobResolveHandler` is the entire cross-peer materialization pipeline and it had
**twelve test files** behind it — one-way, bidirectional, burst writes, a 4 MB file, capability
delegation, late join, self-loop. Every one of those registrations was in a `_test.go`.
`shellboot` registered ingest and chain-errors and not this one, and `subscription` is
`ls|inspect|rm` with no create, so **the whole thing was built, tested, and reachable from
nothing a user could run.**

That is D23 at the **handler** layer, and `make reachability` passes because the sweep asks
whether a *model* has a surface. A handler is not a model. §0L already noted that a read-only
surface over a read-write model is invisible to that sweep; this is one further out — no surface
at all, and nothing looking for it. The question the sweep will not ask for you: *what registers
this in a shipped binary, and what verb causes it to be used?*

`sync` **refuses without a local mount to receive into.** A sync writes into a mount; it does not
create one. Choosing where an operator's files land is not a default, and a relationship that
establishes cleanly and then 404s on every delivery is the failure this repo has now shipped
twice — the ingest mapping, then the mount binding. The third time it becomes a precondition.

### Every subscription was dead after a restart, and every mount with it

Found while scoping the above, not by any suite.

`subscription.Engine.Load()` rebuilds the engine's runtime index from the
`system/subscription/{id}` entities in the tree. Its own doc comment says it **must** be called
after `SetLocationIndex` and before `StartDelivery`, and says why — *"subscriptions are classified
as PERSISTENT extension state, so the durable copy in the tree is authoritative and the runtime
index is a derived cache that must be rebuilt on boot."* `entity-core-go`'s daemon calls it.
`entitysdk.assembleAppPeer` made both neighbouring calls and not that one.

So a reopened peer had every subscription it had ever made sitting in its tree, and none of them
live. **Every mount in this repo is driven by a subscription**, so the shipped behaviour was: the
watcher resumes, `mounts` lists it healthy, and no document ever appears again.

That is precisely the end state `workbench/mount_binding.go` was written to fix six days ago,
reached by a **second independent route the fix could not close** — and the mount-binding work was
inside this code and did not see it, because it was hunting a fact held in one process's memory
while this is a fact held in the tree that nothing reads back. Two failures wearing one symptom;
finding either is no evidence about the other.

**AP39's shape one extension over, and worse in kind.** There the volatile index was the query
index and `find`/`grep` answered wrongly. Here the volatile index is the subscription engine's,
and a subscription is not a read path — it is what makes a write *cause* something, so the
failure is not a wrong answer but no answer.

Two instances of one shape is the ladder's promotion trigger: **D26** is ratified — *a persistent
store does not make a derived runtime index persistent; name what rebuilds it at open, and gate it
across a process boundary.* Both instances were **adopt, not build**: the kernel shipped the
answer and we had never called it, which is D20 pointed at initialization.

**The gate had to be built twice to be worth anything.** Its first version asserted the
subscription entity was in the reopened tree — which it is, in both builds, always. Only the
reopened *engine's* subscriber count separates them. And the pattern key is now read back off the
stored entity rather than reconstructed, because the engine indexes on `sub.Pattern` verbatim and
the hand-built key was simply wrong: the test's own premise check caught that on the first run and
reported it as a broken test rather than a broken engine, which is the only reason it did not
become a false finding.

### A stale status line that said the opposite of the truth

`TestStage3_Case2_Bidirectional`'s header said **"CURRENTLY SKIPPED pending core-team / arch
investigation"** and *"fully symmetric peer-to-peer does not [work]"*. F9 was closed by core-go at
`8ad52bc`, the skip came off, and the test has been passing ever since — with that paragraph
above it. So the file asserted in its own header that the default deployment shape was broken
while the code beneath it proved nightly that it was not, and the constraint outlived its fix in
our planning documents. Corrected in place, history kept and marked as history.
`FILE-REPLICATION-LANDSCAPE.md` §6 inherited the same error and is corrected too.

### What M2 does not cover, named rather than implied

Two peers on **one host** over loopback TCP. Two physical machines is untouched — the addressing
and NAT story is not started and `ext/relay` is unlanded upstream. **No GUI affordance**: the
verbs are shell-only, and the Local Files panel still manages mounts and says nothing about syncs.
And a sync delivers changes from the moment it is established; it does not replay history, which
the verb prints rather than leaving an operator to watch an empty directory.

### Verified

`make test-each` **10/10** with M2 and the subscription fix in.
`lint`, `gofmt`, `reachability` clean. The M2 end-to-end gate builds both peers through the real
`shellboot.Bootstrap` and was **verified to fail before it was trusted** — deleting the
blob-resolve registration fails it with *"shellboot did not wire the blob-resolve handler"*, which
is the pre-M2 state reproduced. An earlier draft of that test assembled its own handler list and
would have stayed green through exactly that deletion; that is the same defect as AP61 one layer
up, caught in review rather than by a suite.

## §0N NEW (2026-09-02) — the panel threw on every file, and this time the forensics worked

§0M shipped the Files panel and closed by naming what had not been done: *"nobody has driven this
by hand. It is headless plus the Go suites."* An operator drove it the next morning. The report:
mounted a folder, saw the files, clicked them, **the preview pane never updated**, and after a few
more clicks the app died.

Both symptoms are one defect, and it was diagnosed from the run log in a single read.

### One exception, nine times, then the ninth killed it

`FileExplorerPanel.OnRowSelected` formatted the file's mtime with
`DateTimeOffset.FromUnixTimeSeconds`. The field holds Unix **milliseconds** — every producer
writes `info.ModTime().UnixMilli()`, in the kernel's watcher, both sites in its operations, and
our own `mount_sweep.go` — and `FromUnixTimeSeconds` throws above year 9999, which every real
mtime in milliseconds is. So the handler threw on **every file row**, before reaching
`LoadPreview` three lines later. That is the dead preview pane, exactly.

The crash is the same fault counted. `MaxContainedUiFaults` is 8; the log holds nine
`Dispatcher.UnhandledException` records and then an `AppDomain.UnhandledException (terminating)`.
The operator's *"eventually it just crashed"* is the ninth click.

The field is now `ModifiedAtMillis` end to end — model, bridge DTO, panel — because **a doc
comment is invisible from the far side of cgo and JSON**, and this one said "a Unix second" while
every writer disagreed with it. Separately, `FormatMtime` now range-checks: an mtime is a number a
*filesystem* chose, not one we did, and a row that cannot be dated should lose its date, not the
operator their session. The unit was the cause; the missing guard is why it was fatal instead of
ugly.

### Why 138/138 could not see it — two reasons, both worth carrying

**The end-to-end test drove data all the way to the row and never touched a row.**
`A_Mounted_Directory_Of_Mixed_Kinds_Becomes_Browsable_Files` is a real-session test in the strong
sense — real directory, real watcher, real subscription, real ingest, real bridge — and every
assertion in it reads a **row viewmodel**. `OnRowSelected` is reachable only from
`SelectionChanged`. The test stopped one method call short of the code that renders what it had
spent seven layers producing, and that gap reads as thorough coverage rather than as a hole.

**And the envelope test's fixture carried a value no producer emits.** It hard-coded
`"modifiedAt": 1700000000` — plausible-looking, in the seconds range, and written by nobody in
either tree. So it asserted the decode and could never catch the unit. That is **AP58's shape at
the level of a value rather than a wrapper**: a fixture that is not what production produces
cannot fail on the difference. AP58 was measured six days ago on a store wrapper; this is the
second instance, one layer over, which is what the promotion ladder calls a different shape.

Now: the mount test **selects every row**, and asserts the detail line carries the current year
(so a unit swap fails rather than silently printing 1970) and that the preview pane updated — the
operator's own sentence as an assertion. `FileExplorerMtimeTests` covers the formatter directly
including the `long.MaxValue` arm. **Verified to fail before trusted**: restoring
`FromUnixTimeSeconds` fails all three with the operator's exception, through the same
`SelectionChanged → OnRowSelected` route the production stack shows. AP61.

**A near-miss in the gate itself, recorded because it nearly cost the whole exercise.** Inserting
the new test class immediately above `FileExplorerPanelMountTests` silently stole its
`[Collection(nameof(BridgeCollection))]` attribute. The first falsification run duly reported
3 failures — and one of them was *"constructor parameters did not have matching fixture data"*,
meaning the row-selection arm had never executed. The failure **count** was right and the failure
**reason** was wrong, and the count alone would have been read as proof. A control run that fails
for the wrong reason is not a control run.

### The instrument question, answered

The operator asked whether the diagnostic work is paying off. On this one it paid completely,
and it is worth being precise about which pieces did it:

- **The run log survived.** AP54 moved run logs out of `dist-native/`, which `extract` deletes on
  every build. The 2026-09-01 crash lost its stderr to exactly that. This one is 3,285 lines at
  `avalonia/run-logs/run-20260902-083012.log`.
- **`run-with-dump.sh` was armed without anyone choosing it** — until 2026-09-01 `gui-run` exec'd
  the bare binary and whether a session had diagnostics depended on which target you typed.
- **The breadcrumb ring named the click.** The last entry before the fault is
  `input: press @547,690 on TextBlock "  ENTITY-COMPUTATION-MODEL.html"`.
- **The fault channel printed `file:line` on the first fault**, so no coredump was opened, no
  `si_code` was read, and `crash-stack` was not needed. The doctrine's whole point is that the
  intuitive order wastes days; here the first signal was sufficient and the rest stayed unused.
- **Containment turned one fatal click into eight recorded ones.** Nine identical stacks is
  strictly more evidence than dying on the first, which is what the bound was for.

**What did not work is the suite**, and that is the finding. The forensics chain caught a defect
the tests were structurally unable to reach.

**One honest observation, not acted on.** Containment is silent *to the operator*. Eight faults
went to the log and the crash trail and nothing appeared on screen, so from the user's seat the
preview pane was simply dead until the app vanished. That is arguably correct — a UI that spams
exception dialogs is worse — but "contained" currently also means "invisible", and AP49's rule
that a surface must say so points the other way. Flagged for a decision rather than changed.

### Verified

`make test-each` **10/10**. Avalonia headless **141/141** (was 138 — the three new tests).
`lint`, `textual`, `reachability`, `gofmt` clean.

## §0M NEW (2026-09-01, fifth pass) — a mount you can actually use

§0L shipped the ability to mount a directory from the GUI. The operator's report on it, verbatim:
*"I mounted the thing. Wasn't able to really do much with it after that."* That was accurate, and
tracing why turned up four defects, two of them shipped and invisible.

### The Local Files panel showed no mounts at all on a real peer

The worst of the four, and it had been there since the panel was written. **`Store.List(prefix)`
takes a tree-relative prefix and returns PEER-QUALIFIED paths** — the prefix is canonicalized on
the way in, the entries come back carrying the `/{peer-id}/` they are stored under. So
`strings.TrimPrefix(e.Path, "system/config/local/files/")` trimmed nothing, the
`strings.Contains(root, "/")` guard after it rejected every row, and the panel rendered
**"no filesystem mounts on this peer"** for a peer that had just mounted one. The mount had
succeeded, written its config and started its watcher. The shell's `mounts` verb had the same
defect one shade milder: it printed the qualified path as the root *name*, which is not a name
`unmount` accepts.

**Why nothing caught it is the transferable half.** The cheap test store —
`NewStore(memory, memory)` — has no namespacing index, so `List` returns bare relative paths and
the arithmetic works. Every model test in the package used it; the namespacing appears only on a
store from `CreatePeer`, which is what every real peer has and what no unit test had. **A fixture
that omits a wrapper the production object always has cannot fail on anything the wrapper
changes**, and this wrapper changes the return-value *shape* of the most-used read in the
codebase. Now: `workbench.TreeRelative` / `RelativeUnder` (one helper, replacing two private
copies that between them showed the class had already bitten twice without being named), five
call sites corrected, and `workbench/tree_path_test.go` — a separate file **on a peer-backed
store**, because the store being real is the entire point. That test asserts its own premise out
loud, so if the store ever stops namespacing it reports the fix as dead code rather than passing
for a new reason. AP58.

### Everything that was not markdown was dropped on the floor, silently

The ingest handler classified with one predicate, `isMarkdownPath`, and returned
`type_not_handled` for everything else: no typed entity, no tree binding, no surface told. So a
mount of a directory of code, images or plain text produced a full source layer and **not one
document anyone could open**.

And the mount row's file count came from the *source* layer — where the kernel's watcher writes a
record for everything it admits, the layer that has no failure mode. A mount of 400 photographs
and one README read as **"401 entities in tree"**. A number that goes up is the most reassuring
thing a panel can show. AP59.

`workbench/doc_types.go` is the registry the old comment promised: markdown, text, code, image,
and `doc/binary-file` as an honest fallthrough. **There is no "unhandled" outcome any more** —
`doc/binary-file` says *this is in your tree, addressable, and we are not going to pretend to
render it*, which is a fact an operator can act on; `type_not_handled` was a fact only we could
act on, and we didn't. Three decisions worth flagging:

- **Classification is by name, never by content.** Reading a 4 GB video's first chunk to learn
  what its extension already said is a cost with no answer at the end of it. A misclassified file
  is a wrong label on a row that still opens; a slow mount is not.
- **Markdown keeps its own struct, byte for byte.** Widening it with three optional fields would
  almost certainly have preserved the encoding — and "almost certainly" is the wrong confidence
  level for a change that re-hashes every document in every existing mount if it is wrong. A test
  pins the four-field shape rather than leaving that as a paragraph.
- **Mount validation had to widen with it.** It refused a target prefix holding types the mount
  did not own, and that set was the single `doc/markdown-file`. Left alone, the first remount of a
  mixed directory would have conflicted with what its own predecessor wrote.

### A mount stopped ingesting after a restart, and looked healthy

A mount is two records. The kernel persists its half and rehydrates the watcher at startup. Ours
— the source→target mapping the ingest handler routes on — lived **only in process memory**. So
after a restart the watcher resumed, wrote its file entities, and every delivery answered
`404 no_mount_for_uri`. `mounts` still listed the mount, because that reads the kernel's record.
Nothing was wrong except that no new document ever appeared again.

Now at `app/workbench/mounts/{root}`, restored by `shellboot`, removed by `unmount` (or the
unmount undoes itself at the next launch), unwound by every failure path in between. Worth noting
how it was found: **not by a restart, but by a second consumer** — the new explorer needed the
target prefix and the only source was a handler's private map. **A fact held in exactly one
process's memory reads as an internal detail until something else asks for it.**

### The panel that shows files

`Files (browse a mounted folder)` — folder tree, breadcrumbs, size column, text preview, and a
**status on every row**: the kind if it became a document, and the reason if it did not. The
summary reports total, ingested and not-ingested separately, because one count cannot tell a
healthy mount from one with nothing openable in it.

`Local Files` keeps the mount administration and is renamed to say so. Two panels, one question
each — the rule the browser trio already earned.

Avalonia has no built-in file explorer control; the framework's own sample is built on a separate
`TreeDataGrid` package, and we deliberately did not take it. A new templating surface is the
container-teardown hazard re-opened somewhere our one guard does not look.

### One setting that existed in the substrate and in no surface we ship

`ReadOnly` has been in the kernel's mount config from the start. Nothing in this repo could set
it: the request struct hard-coded `false`, the verb had no flag, the form had no checkbox. So
*send-only* — the mode an operator reaches for the first time they mount a directory they do not
want written to — was expressible in the substrate and unreachable from the product. Now
`mount -readonly` and a checkbox, and **every mount states its write policy**, not only the
unusual ones. `FILE-REPLICATION-LANDSCAPE.md` §2a is the new inventory this came out of: what a
folder-sync tool exposes, against what we do.

### A deadlock the tests could not report

Found the way these are always found: the sweep stopped. `make test-each` sat on the `workbench`
suite for **sixteen minutes** with no output, and would eventually have reported a 30-minute
timeout naming no test and no reason.

`Store.OnPrefixChange`'s cancel closes the watch and then *waits* for its delivery goroutine to
exit — and that goroutine is inside the model's event handler, taking the model's lock. A `Close`
holding that lock across the cancel is the two of them waiting on each other. No panic, no race
report, nothing. The rule the model now states in one line: **never call into the store while
holding the model's lock.** The attach side is the same hazard and is louder only by luck, because
the subscription delivers its seed synchronously on the caller's goroutine when the store has no
watch hub.

**The second-order finding cost more than the bug.** The first regression test written for it
*passed against the deadlocking version*, and was about two minutes from being committed as a gate
that proved nothing — "verified" by a green run on the already-fixed code. One close under churn
does not reliably catch the deliverer mid-contention; forty open/close cycles against a
continuously-written store deadlocks the broken version in seconds. **A race gate needs repetition
and a control arm**, and the rule that a probe which does not reach the mechanism refutes nothing
applies just as hard to a probe that cannot fail.

The fix had its own trap, caught by a second test: the first correction cleared the renderer's
wake callback on every re-bind, which would have left the panel live-updating for exactly one
mount and silently static for every mount chosen after it.

### Verified

Avalonia headless **138/138** (was 134), including an end-to-end test that mounts a real directory
of mixed kinds through the shipped bridge and asserts each file reaches its own `doc/*` type.
**That test was verified to fail before it was trusted** — restoring the markdown-only gate makes
it report `blob.bin(no-doc), notes.txt(no-doc), readme.md(doc)` and *"3 with no document yet"*,
which is the pre-registry behaviour reproduced. A probe that does not reach the mechanism refutes
nothing, and a probe that cannot fail proves nothing either.

**Not done, and named rather than implied:** nobody has driven this by hand. It is headless plus
the Go suites. And the panel has no image preview — an image row says what it is and its size,
and shows nothing, which is the honest version of a gap rather than a fix.

## §0L (2026-09-01, fourth pass) — the GUI can mount a directory, and it remembers your workspace

Two pieces of the application surface, both entirely ours, neither waiting on anything.

### The Local Files panel could list mounts and not make one

M0 landed the panel; the verbs it was supposed to come with did not. So the only way to mount a
directory in the shipped GUI was to open a Shell panel and type the sentence — with the mount
pipeline (validate the target, mint the scoped chain capability, register the source→target
mapping, persist the RootConfig, subscribe, start the watcher, record the subscription) living
inside `cmdMount`, reachable only from an argv.

**`make reachability` passed the whole time, and was right to.** The sweep asks whether a model has
*a* surface, not whether the surface can do what the model does. **A read-only surface over a
read-write model is a D23 violation the sweep cannot see**, and that is the transferable part: the
tell is a panel with no verb in it. Worth a second look across the other seventeen.

The fix is the extraction `AGENTS.md` already asks for — *DRY the integration, not the renderer*.
`shellcmd/mount_op.go` holds `ShellWorkspace.Mount`/`Unmount`; `cmdMount` is now flag parsing and
phrasing over it, and the bridge calls the same function. Ordering is preserved verbatim, including
the two comments that explain why (subscribe *before* the watcher, because its initial scan writes
synchronously; unwind in reverse on every failure).

One thing changed shape rather than moving. A target-prefix conflict was a formatted multi-line
string — fine for a terminal, useless to a panel that wants to list what is in the way and offer to
proceed. It is now a typed `MountConflict` whose `Error()` renders the exact text the verb used to
produce, so the shell surface is unchanged and the panel gets the structure. The GUI shows the
offending types and a **Mount anyway** button.

**Two things the panel says out loud because it would otherwise be lying by omission.** Unmount
does **not** stop the watcher — the kernel exposes `StartWatching` and no `StopWatching`, so the
fsnotify watcher survives until the process exits (bounded, not leaking: remounting the same root
replaces it). And a sweep names every binding it removed rather than counting them, because a count
alone asks the operator to trust a destructive operation they cannot inspect.

### The workspace reset itself on every launch

`new PanelStack(peerH, this, "site-view", "detail", "shell")` — three hard-coded names. Open the
Browser and Local Files, close Detail, restart: the original three, forever, with no indication
that anything had been remembered or forgotten. That was the largest single piece of friction in
the app and it is the operator complaint from §0E that had gone untouched longest.

Now persisted, and **the interesting decisions are about where and what**:

- **A file, not the tree.** The entity-shaped answer is `app/state/…`, and it is wrong here for a
  blunt reason: with no flags the GUI is an ephemeral in-memory peer and loses everything on exit,
  so a tree-stored layout would vanish at exactly the moment it is meant to help — the default
  configuration. It would also have quietly settled the open §0G design fork about per-window
  state identity. `~/.entity/gui-layout.json` avoids both.
- **Keyed by alias, not peer-id.** Peer-id is the better identifier nearly everywhere else in this
  codebase and the wrong one here: an ephemeral peer gets a fresh keypair every launch, so a
  peer-id-keyed layout would never match itself twice and the feature would silently do nothing
  by default.
- **The policy is in Go** (`workbench/layout_config.go`) even though panel names mean nothing to
  it. Where the file lives, precedence, boundedness, and that a file which exists and does not
  parse is an **error** rather than a silent reset (AP33) are not renderer-specific; only the
  strings are, and the model never interprets them. It also means the rules are covered by a suite
  that runs in milliseconds rather than by a GUI test someone remembers to write.
- **A corrupt layout opens on the defaults AND says why.** Silently substituting them is
  indistinguishable from *"it forgot again"* — which is the complaint the feature exists to answer.
  Same for a panel kind this build does not register: skipped so a file written by a newer build
  still opens, and named on the status line so the panel does not just disappear.

The signal is `PanelStack.LayoutChanged`, fired on add, close **and swap-in-place**. The swap is
the one that is easy to miss — it changes no slot count — and missing it gives a workspace that
remembers the panels you added and forgets the ones you changed your mind about, which is a
stranger thing to diagnose than forgetting everything.

**A hazard caught before it shipped, and it is the AGENTS.md note pointed at new code.**
`WB_LAYOUT` **cannot be set from a managed test**: Go captures its environment once at process
start, so a C# `SetEnvironmentVariable` calls libc `setenv` and never reaches `os.Getenv` in the
bridge. A headless test of this feature would therefore have written to the developer's real
`~/.entity/gui-layout.json`, which is the "no suite may depend on state outside the tree" rule
broken in a new place. The answer is the one this repo already established for
`BrowserPanel.AutoPinOnOpen`: an in-process switch (`LayoutSetPath`), not an env var.

The end-to-end coverage is `PeerViewLayoutPersistenceTests` — arrange, dispose, re-open, assert the
arrangement came back — and it crosses the whole seam, because everything underneath was already
green while nothing connected it, which is the shape this repo keeps finding by audit.

**State:** Avalonia headless **134/134** (was 117). `make test-each` 10/10, exit 0. `make lint`,
`make textual`, `gofmt`, `make reachability` clean.

## §0K (2026-09-01, third pass) — M1: the reverse-write clock, and the defect was one direction over from where we published it

M1 on the file-replication plan is *"replace the clock with content"* in `ext/localfiles` — a
correctness fix in the sibling kernel, so our half is the reproducer and the ask. Both are done:
`workbench/localfiles_reverse_window_test.go` — six arms, ~8 s, no network, green under `-race` —
and the ask is routed with it.

### What the clock actually does

`reverseWriteLoop` drops any **tree change** event for a path the handler wrote to disk inside
`recentWriteWindow = 5 * time.Second`. The event is **discarded, not deferred** — no queue, no
retry, no second look. Measured:

| Arm | Result |
|---|---|
| write A, then write **B ≠ A** to the same path inside the window | disk holds A, tree commits B, nothing errors |
| the same, then wait 6 s with no further events | still A — and re-emitting the **identical** event after expiry delivers it |
| write, then **delete** the binding inside the window | tree unbinds, **file stays on disk** |

The delete arm is the sharp one: `reverseWriteLoop` routes `ChangeDeleted` before any content is
fetched and `reverseDelete` has no circuit breaker, so for a delete the clock is the *only* guard.

### The finding that makes it a small fix

**The content check already exists** — `currentDiskBlobHash(fsPath, chunkSize) ==
fileData.Content`, twenty lines below the clock in `reverseWrite`. Seeding the bytes on disk
directly, so the clock is provably never armed for that path, the echo is still refused: the
content check is the guard. And `markWritten` sits **downstream** of that check, so an echo returns
before reaching it — **the clock cannot arm on an echo.** It arms only after a real tree→disk
write, which is exactly the moment when the next event is most likely to be a genuine follow-up
update. Armed when it is harmful, disarmed when it would be redundant.

The differential is the evidence, not the reading: two tests emit the *same* second event at the
same distance from the first, and it is delivered or lost purely according to whether the first
event caused a write.

### The correction, which is the part worth carrying

**§0H's description of this defect was wrong, and it was wrong in the direction that flattered
us.** It said the window drops a *filesystem* event, so an operator saving their own edit inside
the window has it "discarded as an echo — silent data loss". `isRecentlyWritten` is consulted in
exactly one place, which handles **tree** events; the watcher's `flush` never consults the tracker
at all. Arm 6 measures it with real inotify rather than arguing it from the source: with the clock
armed, the operator's bytes reach the tree 2.0 s later.

The real defect is **worse than the one we published**. A lost operator edit is at least visible to
the operator, who still has the file open. A tree update that never reaches disk is visible to
nobody on either side.

How it happened is the familiar shape: a correct-sounding reading of one function, generalised to
an operation nobody ran (D19 / AP43). §0H even wrote *"it needs a reproducer, because per AP43 a
probe that does not reach the mechanism refutes nothing"* — and then stated the mechanism anyway,
in a bullet a reader has no way to tell apart from the measured ones beside it. **A sentence that
names its own missing evidence still asserts the claim.** Where the conclusion has not been run,
say what is unknown rather than what is probably true.

### And a reach finding that changes M2

**`StartReverseWrite` has no caller in this repo.** One non-test caller exists in either tree and
it is core-go's own `cmd/entity-peer/main.go`. Our peers replicate through the subscription chain
into `local/files:write` instead. So the tree→disk direction this whole defect sits on **is never
started by anything entity-workbench-go ships** — the window is live for `entity-peer` and dormant
for us, which is why routing it did not wait on our adopting anything.

It also means `FILE-REPLICATION-LANDSCAPE.md`'s inventory row calling that direction "live,
**bidirectional**" was a statement about the kernel being read as a statement about us. Corrected
there. M2 (*two machines, one folder*) now has a decision in front of it that the plan did not know
it was making: wire `StartReverseWrite`, or keep replicating through dispatch and say so.

### What we did not establish

Removing the gate is a **hypothesis**, not a verified fix, and the ask says so in its own section.
The clock was originally added to close an unbounded same-path loop under a dual-watch topology,
and **we did not reconstruct that topology.** Arms 4 and 5 support the argument on the tree-event
path; supporting an argument is not running the operation (D19). If removing the gate re-opens
that loop, that is the more interesting result and we want it back.

**State:** `make test-each` **10/10 green, exit 0** — `shellcmd` 290s · `sdk` 220s · `programs`
160s · `shellboot` 15s · `workbench` 12s (the six new arms included) · rest ≤4s. `make lint`,
`make textual`, `gofmt`, `make reachability` clean. **No product code changed this pass** — the
diff is one test file, one routed ask, and the corrections the reproducer forced.

## §0J (2026-09-01, second pass) — it was not a stack overflow, and how we came to say it was is the finding

§0I, written hours earlier, classified the SIGSEGV as a managed stack overflow. **That is
retracted.** The retraction came from the same coredump, and getting it took one pass over the
stack rather than a new experiment — which is the part worth carrying, because it means the
refuting evidence was in hand the whole time.

### The measurement

`coredumpctl info` printed 45 consecutive frames returning to one address and stopped. §0I
reasoned: systemd truncates a backtrace, so the real depth is larger, so this is a call site
recursing without end, so it is a stack overflow. Scanning the stack instead of counting the
printed frames:

| | |
|---|---|
| occurrences of the repeating return address | **exactly 45** — not truncated |
| frame stride | **448 bytes, uniform across all 44 gaps** |
| span of the recursion | 19,712 bytes (19.2 KB) |
| depth of the whole frame chain below the thread descriptor | **24,424 bytes (23.9 KB)** |
| stack available to any thread CoreCLR creates | ≥ 1 MB |

**24 KB of a megabyte is not an overflow.** A uniform-stride run 45 deep is an ordinary
recursive walk — a UI tree is tens of levels deep by construction. Two further facts kill the
attribution outright: the faulting thread is a **background** thread (LWP 1722860; the UI thread
is 1722846 and was blocked in a syscall), and it carries **`libSkiaSharp`** frames, so it is the
render side. `MarkdownRenderer`'s inline walkers — the unbounded recursion §0I bounded — only
ever run on the UI thread.

`MaxInlineDepth = 64` **stays**, because an unbounded recursion over a remote page body is a real
defect on its own terms. It is hardening, and it fixes no observed crash. The source comment and
the AP53 catalog entry both said otherwise and have been corrected in place, because a wrong
conclusion left in a doc comment is invisible to review forever (AP45's own lesson, pointed at
ourselves).

### What is actually established, and what is not

**Established.** The thread was 45 levels into a uniform recursive walk. The repeating frame's
machine code iterates a children array, copies a 72-byte by-value struct (nine doubles — the
shape of a 3×3 transform matrix) onto the stack per child, dispatches through an interface slot,
and OR-accumulates two booleans out of the returned struct. The innermost frame computes min/max
over an array of 16-byte (double, double) pairs — a bounding box over points. Stack words nearby
are layout-scale doubles (≈495.0, ≈1064.2). That is a geometry walk over a visual tree on the
render thread.

**Not established: what faulted.** And here the artifact is worse than §0I assumed. `si_code` is
128 (SI_KERNEL) with `si_addr` 0 — the AP34 re-raise signature, already known. Beyond that: the
recorded `rip` disassembles to `vucomisd %xmm5,%xmm0`, a **register-to-register compare that
cannot fault at all**, and the recorded `rsp` points into memory **the core does not contain** —
it is on the alternate signal stack, which systemd does not dump. So on a systemd core for a .NET
crash, the register set does not merely describe the handler; it describes memory that is absent.
Believe the stack geometry; believe nothing else.

### The reason every crash costs days, which is a defect in our tooling and not in the crash

Two of them, both ours, both found by asking why the artifact was so thin.

**The evidence existed and the build deleted it.** `make up` tees the run's stderr — which for
this crash carried `WB_PANEL_LOG=1`, i.e. **the complete breadcrumb stream up to the fault** — to
`dist-native/run.log`. `make extract` does `rm -rf` on `dist-native`, and every build target runs
`extract`. So the log was destroyed by the next build, and it was. Our own crash doctrine already
says *"write artifacts outside `dist-native/` or copy them immediately"* — we wrote that rule
about smoke-run artifacts while the most important log in the repo sat in the directory the rule
names. Run logs now go to `avalonia/run-logs/`, timestamped rather than overwritten, because the
interesting run is rarely the most recent one.

**A first attempt at this paragraph claimed something stronger and false, and the correction is
worth more than the claim was.** It said *every operator-reported crash was captured with
createdump switched off*, reasoning from a real code fact — `run-with-dump.sh` was copied into
place by `make up` alone, so `make host-run` (`make gui-run` at the root, which this file's own
`AGENTS.md` calls the documented fast loop) exec'd the bare binary. Plausible, and wrong: the
crashed process's environment is **in the coredump**, and it says `DOTNET_DbgEnableMiniDump=1`
and `WB_PANEL_LOG=1`. The operator used `make gui`. **createdump was enabled and did not fire** —
the same outcome as 2026-08-21, which `CrashDiagnostics`'s own header records and which nobody
re-read. That is D25 again, inside the entry ratifying D25, caught only because the claim was
measured before it was committed.

What survives, and is fixed: `host-run` really was unarmed, so whether a session had diagnostics
depended on which target the operator happened to type and **nothing recorded which**. Both
targets now share one wrapper, which also sets `DOTNET_PerfMapEnabled` — genuinely absent from
the crashed process, and without a perf map a JIT frame can only be identified by
hand-disassembling it, which is exactly what this session spent its first hours doing.

What is **not** fixed, and is now the sharpest open question on this crash: **createdump is on
and produces nothing for this fault class.** `CrashDiagnostics`'s hypothesis is that by the time
the process faults there is no stack left for createdump to run on. That is a hypothesis, not a
finding, and it has never been tested.

**The breadcrumb guarantee described a code path that did not exist.** The ring reached disk only
from `WriteFatal`, the *managed* fault handler — precisely the path a hard SIGSEGV does not take —
while the class comment promised "the breadcrumb ring on disk up to the last flushed line".
Nothing flushed it. Measured cost: the crash log's last breadcrumb is timestamped **seventeen
hours** before the fault. Breadcrumbs now write through to `~/.entity/crash/<pid>.trail` as they
are recorded, line-buffered onto the fd so the bytes survive a signal.

All three are **AP54**. Every crash artifact now also states which instruments were armed —
`external diagnostics: minidump=… perfmap=… panel-log=…`, on stderr at startup and in the trail —
because a missing minidump reads identically whether createdump was off or whether it ran and
found nothing, and those two send the next session to different questions. This session spent
real time on the wrong one of those two, and the app could have answered it in a line.

### The reach gap, named and closed

Nothing in this repo could perform the gesture the operator was making. `smoke-xvfb-click` has a
real compositor and a real X11 backend and could only **press and release at one point** — no
pointer capture, no motion stream to a captured element, no scrollbar thumb. The headless harness
can drag (`TreeViewScrollDragTests`, added in §0I) but has no X11 backend and no render thread.
The crash is on the render thread during a drag, i.e. in the intersection neither one covered.
`smoke-xvfb-click` now does real drags — `DRAG_PCT` of gestures, half of them near-vertical
because that is the scrollbar-thumb shape and uniformly random endpoints almost never produce
one — and `crash-hunt` sweeps them. `DRAG_PCT=0` is the control arm.

### New instrument: `make -C avalonia crash-stack`

`avalonia/crash-stack-report.py`, run automatically inside `make crash`. Reads an ELF core with
no debugger, reports per-thread frame stride, uniformity, span and depth below the thread
descriptor, and prints the overflow / not-overflow verdict **with the numbers behind it**. It
also detects the case above — a recorded `rsp` absent from the core — names it as the
handler-on-altstack signature, and falls back to `fs_base` to find the real stack. Validated
against the 2026-09-01 core: it reproduces the hand measurement and returns the opposite verdict
to the one that was published.

### Ratified: D25

**A field that PRINTS is not a field that ANSWERS.** Second instance of the same failure, which
is what promotes it out of the catalog: AP34 read `si_addr` off a re-raised signal; AP55 read a
recursion depth off a truncating pretty-printer. Both fields render in a tidy table beside fields
that *are* authoritative, so the artifact reads as one coherent statement when it is several of
different strength. The rule: name what produced a number and what it is guaranteed to describe,
before it classifies anything. Prefer a derived measurement to a printed count — "45 frames" is a
printout, "45 × 448 bytes against ≥1 MB" is a measurement, and only the second one classifies.

### Still open

The specific fault is **unidentified**, and this entry does not pretend otherwise.

The next run that crashes will have, for the first time, a **JIT symbol map** and a **breadcrumb
trail that reaches the fault**, and its run log will survive the next build. Those three are
measured, not hoped for. What is **not** promised is a managed stack: createdump is enabled and
did not fire for either of the two crashes we have checked, so the minidump channel is
**believed broken for this fault class and untested**. Finding out why is the next piece of work
on this crash, ahead of any further theorising about the fault itself — a working createdump
turns this class from archaeology into a stack trace, and nothing else on the list does.

The drag fuzz is the instrument to point at the fault once there is a channel to read it with.
Eight seeds × 140 gestures with drags at 40% ran clean today; per D24 that is a bound on the
search, not a result — 45 s of synthetic gestures is not 17 hours of a real session, and the
operator's crash followed a long idle.

**State:** Go `make test-each` **10/10 green, exit 0**. Avalonia headless suite green (three new
`CrashTrailTests`). `make textual`, `gofmt`, `make reachability` ok.

## §0I SUPERSEDED (2026-09-01) — the SIGSEGV was a stack overflow, and the coredump said so before any symbols

> **This section's conclusion is wrong and is kept for the record.** The measurement in §0J
> refutes it: 45 frames at a uniform 448-byte stride is 19.2 KB, not a truncated runaway, and the
> faulting thread is a background render thread the markdown walkers never run on. Read §0J.
> What survives from here: the depth bound is still correct as hardening, and the note about the
> DBus exception not being the crash is still right.

The operator crashed the GUI adjusting the tree's vertical scrollbar. Exit 139, no crash log.

**`coredumpctl info` classified it in one read.** The faulting thread carries **45 consecutive
frames returning to a single address** (`0x7f34190c139e`), with systemd truncating the trace — so
the real depth is larger. One repeating return address is not a corrupted stack; it is **one call
site recursing without end**. That reclassifies the bug from "native memory fault" to "managed
infinite recursion → stack overflow" before any symbolication, and it is why nothing was logged:
**a stack overflow on .NET is uncatchable**, the runtime fails fast at the guard page, so there is
no exception, no `Dispatcher.UnhandledException`, and no stack left to run a handler on. AP46's UI
fault containment cannot help — it catches exceptions, and this raises none. Recorded as **AP53**.

Note the earlier `TaskScheduler.UnobservedTaskException` in that log
(`org.freedesktop.DBus.Error.ServiceUnknown`) is **not** the crash: the process kept handling
input for three more minutes after it. The log timestamps are UTC, so the apparent jump from
`21:55` to `14:33` is a day boundary — the app was idle ~17 hours before the fatal click.

**Reach, per the doctrine, and the negative with it.** `smoke-xvfb-click` **cannot reach a drag**
— its own row in the doctrine's table says so, it presses and releases at one point. The headless
harness can: `MouseMove` runs the genuine route with pointer capture. `TreeViewScrollDragTests`
now drives four real drags on the tree's vertical thumb — a sweep, six reversals, a drag off the
window in four directions including an absurd finite coordinate, and unbalanced press/release
pairs. **All four pass**, and the reach that negative carries is: this harness runs the real
managed input stack but **no X11 backend, no window manager, no compositor**. So it rules out the
managed ScrollBar/Thumb logic and nothing below it.

**What we fixed, and what remains unproven.** `MarkdownRenderer.EmitInline` and `AppendText` both
recursed over a parsed inline tree with **no depth bound**, and that tree comes from a *remote
page body* — nesting depth is whoever wrote the page. That is a genuine defect on its own terms:
a hostile or merely pathological document could take the browser down with no catchable error and
no log, which is `AssetNameFromRef`'s hazard class reached through document structure instead of a
URL. Bounded now at `MaxInlineDepth = 64`, refusing to descend and emitting an ellipsis rather
than truncating in silence.

**It is NOT established that this recursion caused that crash.** The JIT frames carry no module
and a systemd ELF core yields no managed stack, so the coredump names the shape and never the
method. What is established: the crash was a stack overflow, and we had an unbounded recursion
over untrusted input. Those are two facts, not one — do not let the next session collapse them.

**Still open on this crash:** the specific recursion is unidentified. The next instrument is
either an xvfb drag harness (`smoke-xvfb-click` extended past press-release, which the doctrine
already names as the missing capability) or catching the first SIGSEGV live under gdb with
`make -C avalonia smoke-xvfb-click GDB=1`, which is the only way to get a faulting-thread register
context that has not been through CoreCLR's re-raise (AP34).

**State:** Avalonia **114/114** (was 106), Go green on touched packages, `make reachability` ok,
`make textual` ok, `gofmt` clean.

## §0H NEW (2026-09-01) — M0: the filesystem mounts have a surface, and the sync landscape is mapped

**The landscape analysis is `docs/architecture/FILE-REPLICATION-LANDSCAPE.md`.** Short version:
`ext/localfiles` in core-go is already most of a sync engine — bidirectional filesystem/tree
sync, fsnotify watcher, a stat cache implementing Git's racy-clean rule, path containment with a
leaf-symlink refusal, FastCDC chunking, tombstones, restart-equivalent mount config, and a
1022-line normative spec. The gap is not plumbing. It is `DOMAIN-LOCAL-FILES` §35: concurrent
same-path writes are **last-arrival-wins with no merge**, which as a product statement means an
edit made on two offline machines silently loses one. Syncthing has not lost that write since
version vectors. The composition nobody has built is `localfiles x revision` — merge instead of
surrender — and that is the only milestone on the plan that is not catch-up.

**M0 is done.** `workbench/local_files_model.go` + `LocalFilesRender` on the bridge +
`avalonia/frontend/Panels/LocalFilesPanel.cs`, registered under "This peer". Before this,
`grep LocalFiles avalonia/ console/` returned **nothing** — a complete kernel extension with no
user-reachable edge, D23's shape for the fourth time. Rows come from the persisted
`system/config/local/files/{root}` configs, which is the same source restart-equivalence reads,
so there is no second workbench-side mount list to drift.

Two things the model deliberately refuses to do. It does **not** claim watcher liveness:
`WatcherConfigData` is built as the response to a `watch` op and never written to a tree path, and
the handler's live set is unexported, so every row carries `WatcherObservable=false` and the panel
prints *"watcher: unknown"*. Inferring "configured" means "running" would be the AP45 move. And a
mount whose config does not decode **still renders**, with the reason attached — dropping it would
report a broken mount as no mount at all.

**The core-go tracker is `reviews/CORE-GO-TRACKER.md`**, a living index of all seven open asks
plus what they have already closed. *(Moved 2026-09-09 to
`docs/status/TRACKER-entity-core-go.md` under the ecosystem-wide seat convention — §23.)* Two new ones came out of M0, both in `ext/localfiles`:

- **The reverse-write loop guard is a clock, not a content check.** `recentWriteWindow = 5 *
  time.Second` drops an event for a path the handler wrote to disk in the last five seconds.
  > **Corrected 2026-09-01 by the M1 reproducer — read §0K.** This bullet went on to say the
  > dropped event is a *filesystem* event, so an operator's own edit inside the window is
  > "discarded as an echo — silent data loss". **That is wrong in both mechanism and consequence**,
  > and it is wrong in the direction that flatters us: the real defect is worse. The tracker gates
  > **tree** events only; the watcher's ingest never consults it. What is lost is tree→disk.
  The fix is still to ask whether the bytes on disk are the bytes we wrote — and it turns out that
  check already exists twenty lines below the clock. Routed with a six-arm reproducer.
- **No way to observe watcher state**, per above. Persisting `WatcherConfigData` per root composes
  better than an accessor: it makes watcher liveness a tree fact any impl can read.

**Open, not investigated: a SIGSEGV in the GUI.** pid 1722846, 2026-09-01 09:33 CDT, core present
in `coredumpctl` (28.3 MB), log at `~/.entity/crash/entity-avalonia-1722846.log`. The app had been
up ~17 hours idle; a single click on a `Border` took it down while the operator was rearranging
panel framing. Note the log timestamps are **UTC** — the apparent jump from `21:55` to `14:33` is
a day boundary, not a clock fault. The `TaskScheduler.UnobservedTaskException` earlier in that log
(`org.freedesktop.DBus.Error.ServiceUnknown`) is **not** the crash: the process kept running and
handled input for three more minutes. Deliberately not chased yet — `DOCTRINE-CRASH-FORENSICS`
says reach before forensics, and the first question is whether `make -C avalonia crash-hunt`
reproduces it from a seeded click sweep.

**State:** Go suites green on the touched packages, Avalonia **106/106** (was 101), `make
reachability` ok, `make textual` ok, `gofmt` clean.

## §0G NEW (2026-09-01) — browser-rust read our tree and found two spec defects we could not see

`entity-browser-rust` reviewed `entitysdk/workspace_state.go` and sent three findings. All three
verified against `GUIDE-ENTITY-WORKBENCH-APP` in the arch repo; two are fixed here, one is a
design fork and is below.

**We were accusing conformant peers of non-conformance (AP52).** §5.4 rule 3 retires exactly
three Selection fields — `source_window`, `source_panel`, `content_type` — and MUSTs a WARN when
one is read. Our `legacySelectionFields` had **four**; the fourth was `paths`, which the same
section's schema block declares **live and optional** (*"the wider selection set when the user has
shift-clicked / ctrl-clicked"*). So a correct multi-select emitter got named NON-CONFORMANT on the
one channel the ecosystem has for finding emitters that actually are. The unit test **pinned it**
— it looped over all four names asserting each produced a WARN, so green meant conformant to a
rule nobody wrote. That test is now the regression guard, inverted.

**`app/state/window` MUSTs a `content_type` field and we wrote `content-type`.** §4.2's slot
table is explicit, and the hyphen meant a reader looking for the field the spec names found
nothing — the MUST satisfied in spirit and failed in fact. Now written with the underscore, with
the legacy spelling read as a fallback and **dropped on the next save**, so a tree written by an
older build converges instead of carrying two fields that disagree.

The transferable half is one sentence: **an implementation does not get to retire a field the
schema still declares, or rename one the schema spells.** When a list or a key mirrors a normative
document, diffing it against that document *is* the review.

**Still open — §8's persist-arm obligation, and it is a design fork, not a fix.** An application
persisting per-window state MUST be able to say at startup which window each persisted entity
belongs to, satisfied by *either* an `app/state/window-index` *or* a startup sweep before
allocating any id. We satisfy neither: `window-index` has **zero hits** in the tree, the only
reference to `workspace/windows/` is the path builder, and the sole id allocator is `ws.nextID++`
(`console/workspace.go:88`), a per-process counter that restarts at zero. We persist
(`workspace_state.go:429`) and read back live on a session ordinal (`workbench/log_model.go:143`),
so session 2's window 1 inherits session 1's window 1 state — precisely the third case the rule
exists to prevent. Bites with `-storage sqlite`; the default in-memory peer is unaffected.

It is not a one-liner because the two arms mean different products: the sweep arm makes per-window
state *safely non-resuming* (and log display level stops persisting, which today it does
incorrectly), the index arm keeps resumption and costs a new persisted entity. **Sequencing that
is an operator call**, so it is here and not guessed at.

## §0F NEW (2026-08-31) — an operator drove the browser for real, and it was slow, silent and picture-less

The links landed in the morning (AP47). By afternoon someone used the panel to actually read a
site, and every complaint was a defect we had not measured.

**It was slow because it re-downloaded a verified tree on every click.** Measured against the
live federation with a counting transport:

| | before | after |
|---|---|---|
| open `billslab.com` | 7.3 s / 61 requests | **2.4 s / 61** |
| click a link in it | 6.2 s / 60 requests | **0.36 s / 4** |
| Back | 5.9 s / 60 requests | **0.23 s / 3** |

51 of every 60 requests were the *same* CHAMP nodes, fetched serially at ~90 ms each. The
refusal to cache was justified by a sentence that is true of the **manifest** — the one mutable
pointer in the chain — and false of everything it points at: a content URL's path *is* the
SHA-256 of the body it returns, and a trie rooted at H has exactly one key set forever. The
publisher's own transport profile even says `freshness: "static-immutable+signed-pointer"`, in a
field we parse and never read. `entity-browser-rust` had written the answer in a doc comment:
*"5 fetches for the first page, 2 for the next."*

**The paranoid shape turned out to be the weaker one**, which is the part worth carrying: the
same comment names the `seq` floor, and a `Consumer` rebuilt per navigation cannot enforce it —
so an origin could serve `seq 5` for one page and `seq 3` for the next, a validly-signed replay
of a previous publish, page by page, undetected. We now hold one consumer per publisher per
session and refuse a rollback (`fetch.ErrSeqRollback`). Recorded as **AP48**.

**It jumped around because `Refresh` rebuilt everything, every time.** Three columns re-laid out
per click, the page losing its scroll offset because its content had been replaced. Fixed with
remembered signatures. The trust rail is now **dimmed and labelled** during a navigation instead
of emptied: the page does not become fresh when a navigation *starts*, so the old chain is
exactly what describes the page still on screen, and clearing it deleted a true statement. A
*refused* navigation still clears the page, and that distinction has its own test.

**It did not show the pin, and the reason is nastier than the symptom.** The model computes
`RegistryPinFromOrigin` (AP45's trust-on-first-use flag) and `RegistryRebasedFrom` (AP44's third
provenance state); the bridge sent both; **the panel's DTO declared neither**, and
`System.Text.Json` discards an undeclared member in total silence. `entity-shell` met the
obligation in full and the GUI met none of it, with every test green. The registry identity *was*
rendered — below an expanded seven-field pin form, i.e. below the fold. Identity is now first,
the form is collapsed, and the pin is in the chrome. **AP49.**

**There were no images because we had implemented two thirds of the site convention.** A site is
`manifest` + `pages/` + **`assets/`**, and we had no path helper, no resolver branch and no
renderer for the third. On billslab's methodology site that is **665 of 966 committed keys**.
Two independent reasons it was invisible: the wire grammar is a directive,
`::embed[caption]{ref=assets/figures/x.png}`, which every markdown parser renders as literal
text; and Markdig models an image as a `LinkInline` with `IsImage=true`, so a figure that *did*
parse would have become a clickable link to a page no site commits. Now live: **13/13 figures on
`gallery/biology-chain-1` render as PNGs**, and the three hostile refs a page body could write
(`https://tracker/…`, `assets/../../secret`, `/etc/passwd`) are refused by `AssetNameFromRef`,
whose vectors are the reference's own. **AP50.**

**The "unsupported markdown block" placeholders were mostly bugs, not TODOs.** A link-reference
definition group renders as *nothing* in every markdown implementation — printing a placeholder
announced a failure where the correct output is silence. An HTML block is lowered to its text.
Tables are built as real `Grid`s inside an `InlineUIContainer`, with cell content going through
the ordinary inline emitters, so a link or a figure inside a cell still works.

**The raw HTML was the papers, and they are big.** `SitePage.format` admits `html` — the
web-tier escape hatch for a pre-rendered document — and billslab publishes 23 of them, the
largest **8.27 MB**. The panel's DTO never declared `BodyFormat`, so HTML went into Markdig and
rendered as its own source, and the raw body crossed cgo as an 8 MB JSON string on every render.
It is now lowered to text with an honest note, capped at 256 KiB, and the bridge sends only the
projection. Why not a WebView — and what to build instead, which is a *structured* lowering into
markdown so HTML pages reuse the table/link/figure renderer — is
`docs/architecture/HTML-PAGE-RENDERING.md`.

**A cold open is still 61 requests, and that is the completeness proof, not overhead.** The
operator's read after the fix was *"still a little slow"*, and the honest answer is a design
choice rather than a defect. `entity-browser-rust`'s *"5 fetches for the first page"* is a
**targeted CHAMP descent**: follow the key's hash path from the root, four or five nodes deep,
and fetch the page. Ours walks the whole committed trie — 51 nodes — because
`nav.record("target walk", …)` is the one step in the chain *a withholding origin cannot pass*.
Every other step is satisfiable by an origin serving a correctly-signed root that commits to
nothing. A descent proves the page it returns is committed; only an enumeration proves the
origin is not holding the rest back, and the panel's site list needs the enumeration anyway.
So the reference is faster on the first page **because it verifies something weaker there**, and
the comparison is not like-for-like. The cost is paid once per publisher per session — the
second page is 4 requests.

There is a real design option here and it is written down rather than built: render on the
targeted descent and finish the walk in the background, downgrading the trust rail if the
completeness check then fails. That trades a *provisional* claim on screen for latency, so it
needs the rail to be able to say "committed, completeness pending" — a third state, and AP49 is
the standing warning about what happens when a provenance state exists in the model and no
surface says it. Not started.

**Two things this session left alone, both small, neither on anything's critical path.**
1. **The shell does not auto-pin.** `workbench.LoadBrowseConfig` — precedence
   `WB_REGISTRY_ORIGIN`/`WB_REGISTRY_PEER` > `~/.entity/browser.json` > the built-in origin —
   has exactly one caller, `avalonia/bridge/browse.go`. So the GUI opens pre-pinned and
   `entity-shell` still needs an explicit `registry pin <origin>` first. `make reachability`
   passes and is right to: the model *has* a surface. This is a consistency gap between two
   shipped surfaces, which is a shape the D23 sweep does not look for.
2. **The saved panel layout is still the old default** (Local Site / Detail / Shell), so the
   Browser panel is not on screen until it is picked from the picker.

**A hygiene defect caught in the working tree, before it was committed — the mechanism is
invisible and the guard for it was blind at the first attempt.** This session's uncommitted
`BrowserPanel.cs` had grown **17 raw control bytes** — NUL, SOH and STX typed *literally* into
C# string and char literals as list-signature separators. Semantically fine, compiler happy; but
`git` calls any file with a NUL in its first 8000 bytes **binary**, so the 1513-line centrepiece
of this work had no diff, no blame and no merge resolution, and `git diff --stat` reported
`Bin 36537 -> 64640 bytes`. The committed HEAD version was clean, so nothing shipped. Rewritten
as `"\u0000"` / `'\u0002'` escapes — identical characters to the compiler, textual file,
643/51 diff restored. The tell is a source file that `file(1)` calls `data`.

Two things about it are worth more than the fix. **The obvious guard does not work:** GNU grep
cannot match a NUL in a pattern *at all*, so `grep -P '\x00'` over the tree reported it clean
while `od -c` showed the NUL sitting there — a false negative that reads exactly like a pass.
`grep -I` (which classifies rather than matches) and a byte scan both see it; the pattern does
not. And **it recurred inside this same session**: writing the paragraph above into `STATUS.md`
put two real control bytes into a *published* document, because the escape text was interpreted
on the way in. That is the second shape in one afternoon, so the guard is
`scripts/no-control-bytes.py` behind **`make textual`**, validated in both directions — it
fails on the reconstructed pre-fix `BrowserPanel.cs` (all seventeen bytes, at the offsets the
original scan reported) and passes the current tree.

**State:** Go 10/10 suites green (`make test-each`, exit 0), Avalonia 101/101, `make
reachability` ok, `gofmt` clean.

## §0E NEW (2026-08-30, second pass) — the operator used the GUI, and it found the two things the first pass missed

§0D fixed the *refusal* that made a co-hosted registry unreachable. Within the hour the
operator drove the Avalonia **Browser** panel against the live registry and hit two failures
the fix did not touch. Both are recorded because the first pass had already declared the area
done.

**1. "0 names" was a confident wrong answer.** With no pin supplied we adopt whichever peer the
origin features. At the live registry that is the *site* peer, whose signed root honestly
commits **zero** registry bindings — so the panel drew an empty list and said nothing else.
This is AP44's root cause in the shape that **needs no refusal to go wrong**, which is why the
first fix missed it and why it is the worse of the two: a refusal at least names itself.
`fetch.NameSet` now carries `NotARegistry`, `Diagnosis` and `OtherPrefixes`, and every surface
renders the diagnosis instead of an empty list — *"peer 2KEbBKup… is NOT a registry; its root
commits 39 keys — apps/ (34), sites/ (5) — and not one by-name binding. This is almost always
one origin hosting several peers."*

**2. The operator objected to being made to type a peer-id at all — and was right, because the
fact was already on the wire.** `{origin}/entity-deployment.json` is the cohort's deployment
descriptor, and it carries both `name_registry_pin` (origin + peer-id) and `home_site`. Reading
it means typing a bare domain now works: **`entitychurchregistry.org` → 5 names**, no key.

A pin taken from there is **trust-on-first-use and is labelled as such at every surface.** An
operator's pin is the one fact the origin did not choose; this one the origin chose. It cannot
forge a binding for a key it does not hold — but it can hand you one it does. Refusing to look
would have been AP44 a third time: protecting an invariant at the cost of the feature, when the
honest move is to do it and say what it rests on.

**How we missed it is the durable part, and it is now AP45.** `SiteDefaulted`'s own doc comment
named `entity-deployment.json`, described what it holds, and concluded *"so the choice here is
first-in-byte-order."* Every clause true, the conclusion wrong, and the comment names the file
that holds the answer. **A dismissal recorded as a doc comment is invisible to review forever**
— a `TODO` invites work; a paragraph explaining why a limitation is correct closes the question
for every future reader, including its author. Consequence measured today: `billslab.com`
declares `home_site: billslab-main` and we were opening `billslab-entity-system`, purely because
it sorts first. Now we read the declaration — and, because it is unsigned, it may only **select
among sites the signed root already commits**; an origin naming a site the walk does not carry
is ignored, never followed.

**The panels themselves were the third complaint and it was fair.** Eighteen panels in one flat
picker with no ordering, and three of them — `Browser`, `Site`, `Publisher Verify` — read the
same bytes under names that do not distinguish them. `PanelRegistry.Register` now takes a
**category** and a **blurb**; the picker groups by category with the blurb as a tooltip; and the
three are renamed to say what they answer: **Browser — registry + sites on the network**,
**Local Site (this peer's own)**, **Origin Inspector (is it serving what it signed?)**. The
Browser's address-bar example was `docs.entitychurch.org/demo/index`, a domain that does not
exist; it is now the live registry.

**Verified end to end through the panel's own model** (`workbench.BrowseModel`, which is exactly
what `BrowsePin`/`BrowseNames`/`BrowseGo` drive): bare domain → pin discovered from the origin →
layout re-based → 5 names from the walk → open `billslab.com` → `billslab-main/index`, three
sites enumerated, chain green, freshness scoped to the target's `published_at`.

**Tree:** `make test-each` 10/10 · `make lint` clean · `gofmt` 0 · Avalonia headless **74/74** ·
`make consume-live` green.

**Still open from this pass, deliberately:** the **Local Site** panel still opens the bundled
demo, because there is no bridge export listing the local peer's own sites. It is now labelled
rather than fixed, and that is a stopgap. The panel *layout* complaints — one vertical column,
no persistence, no tabs — are untouched; the picker is grouped but `PanelStack` is unchanged.

## §0D NEW (2026-08-30) — we consumed the live federation, and it found a refusal that was protecting nothing

The naming chain works end to end against the live public federation, for the first time:
enumerate a registry by **walking its signed root**, resolve every name through
`EXTENSION-REGISTRY` §6a.4 in full (signature · the `binding.name == asked` association
check · a finite unexpired `ttl` · a revocation probe inside the signed key set), follow a
binding's transport to a **second domain**, verify that peer's own root under a **different
key**, and read page bytes that hash to what that root committed. Five names, all resolving,
plus the follow-through leg.

**Getting there took most of the session, because our own tooling refused.** Every naming
surface this repo ships read the well-known `{origin}/transport-profile`'s `peer_id` as *the*
peer that origin serves, and refused when the operator's pin named a different one. The
registry origin **hosts two peers** — the well-known object features the *site* peer, and the
*registry* peer sits beside it. So the refusal made the cohort's only public registry
unreachable from `entity-fetch`, from `entity-shell`'s `registry`/`browse`/`open`, from
`workbench.BrowseModel` (hence the Avalonia **Browser** panel), and from
`workbench.ConsumeModel` (the **Publisher Verify** panel). Four sites, each independently
phrased as a security property, none ever questioned.

`EXTENSION-NETWORK` §6.5.3 makes that object a **cold-start entry point**, not an exclusivity
claim. We had invented the invariant and then enforced it.

**Two properties made the class invisible, and they are the durable part (AP44).**

- **Every fixture in this tree serves one peer per origin.** Not under-coverage — a
  *mis-shaped* fixture. A fixture that models one instance of a plural relationship cannot
  fail on the plural case, and its greenness is not evidence about it.
- **A false refusal reads as rigor and leaves no wrong answer to catch.** It yields *no*
  answer, which at a consumer is indistinguishable from a broken origin, so the failure is
  attributed outward every time. The tell to grep for is a refusal whose message asserts a
  fact about the world — *"advertises"*, *"is"*, *"would not be the same publisher's"* — then
  ask which spec sentence makes it exclusive.

**The fix is a split, not a relaxation: a derivation is safe exactly when someone else's key
checks it.** `fetch.Layout.RebaseTo` re-points an origin's advertised layout at a co-hosted
peer — origin-level fields (`content_url_prefix`, `content_layout`, both suffixes) verbatim,
and whole path **segments** equal to the featured peer-id substituted in `tree_url_prefix` /
`manifest_url_prefix`. Segment-exact, never substring. Consumer paths take it, because the
next fetch is that peer's own signed root and a wrong substitution cannot produce a verifying
one — it fails closed. **`registry issue` still refuses**, because there the derived reach
goes into a binding *we* sign, our signature would be the only thing asserting it, and
nothing downstream could catch a bad one.

Layout provenance is now **three** states everywhere — discovered / re-based / pinned. A
surface that collapses re-based into discovered tells an operator the origin advertised a
layout it never did.

One live-tree defect fell out on the way: `BrowseModel`'s hop-2 peer-id-address branch loaded
the origin's featured layout and never compared it to the peer the user addressed, so against
a co-hosting origin it would have walked the **wrong peer's** root and rendered it as the
answer — silently. Now re-based onto the addressed peer.

**New instrument: `make consume-live`** — the whole naming chain against the live federation.
Deliberately **outside `test-native`**, same rule as `crossimpl-go` and one step stronger: it
reaches the public internet, and a sweep that can go red for a domain's reasons teaches
people to ignore the sweep. The offline half is `fetch/rebase_test.go`, pins transcribed from
the live wire, so the mechanism stays covered by `make test-fetch` with no network.

Routed as `reviews/COHOSTED-PEER-DISCOVERY-2026-08-30.md`: one ask (is the well-known profile
singular per origin, and if not, how is a co-hosted peer cold-started?) plus the finding that
the live registry publishes at `system/`, which is what makes §6a.3a's corrected MUST
implementable — the narrow prefix we argued against on 2026-08-21 would have broken this
deployment.

**Tree, measured today:** `make test-each` 10/10 green · `make lint` vet clean · `gofmt -l`
0 files · `make reachability` clean · `make consume-live` green.

**AE-5, re-measured by hand this session** (PR-D still has no target): corpus `8d2f55c8`, 362
vectors — **334 agree · 0 diverge · 28 INCOMPLETE · NOT LOCKED**, byte-identical to the
2026-08-25 reading. Itemised from the run: **12** `v325-corner` primitive vectors (PR-C),
**11** value-form error vectors (PR-E), **5** scope-fence vectors (the AE-6 question).

**Two rows corrected as stale:** `PeerLiveness` is **not** the remaining renderer gap — the
Avalonia peer panel consumes the liveness exports and `make reachability` is clean (closed by
`8383326`, never struck through). And `USAGE-PROTOTYPE-FILESYSTEM-SYNC.md` §10 still points
readers at the removed `canvas/` renderer.

## §0A — 2026-08-24, the 0.9.0 release preparation

**Working notes for this repo. Release-process coordination is not recorded here** — it belongs
to the seats that own it, and our side of it is in `docs/status/HANDOFF-2026-08-24-release-readiness.md`,
which does not publish. What follows is what changed in this tree and why.

### Tree state — the line this release is cut on

`make test-each` to completion, run twice today, the second time on the release tip, against
`entity-core-go` `13a42ea`:

**ALL 10 SUITES GREEN, exit 0** — `shellcmd` 290s · `sdk` 206s · `programs` 155s ·
`shellboot` 14s · `shell` 4s · `inspect`/`workbench`/`publish` ≤4s · `fetch`/`shellpanel` ≤2s.

**Re-run 2026-08-25 on the Axis-1 fix (§0A), same result** — `sdk` 209s · `shellcmd` 290s ·
`programs` 157s · `shellboot` 14s · rest ≤4s. `make lint` clean, `gofmt` clean. Green here now
means the defect is gone, not waived.
`make build` green (five binaries) · `make lint` clean · `gofmt` clean · `make reachability`
clean · Avalonia headless **74/74**.

**This is the first fully green `test-each` this tree has had**, and as of the Axis-1 fix below
it is green because the defect is **gone**, not waived. The intermediate state — waived on blast
radius with the cause recorded as unknown — lasted part of one day and is written up below,
because how it got there is the reusable part.

**The kernel moved 10 commits under us since the previous recorded sweep**, two of them
load-bearing here — `e5b3efd` (§6.9's "all resources" narrowed to own-namespace, the rule
`MintMirrorCapability` exists for) and `fb6d461` (published-root convergence under load). Same
single failure, same seed, so neither reached us. **The green line was re-measured against what
we actually ship on rather than inherited from yesterday.**

**The one failing suite is now green because the defect is fixed. Getting there took a wrong
count, a wrong retraction, and finally an instrument. All three are worth recording.**

`TestAxis1Equivalence_Differential` sweeps 300 generated graphs across the reference compute
engine and the experimental Axis-1 engine.

**First: the count was wrong.** The sweep ended at `t.Fatalf` on the first divergence, so for
three days this file, the CHANGELOG and every number routed outward said **one** failing case.
Removing the early exit shows **three** — 9, 28 and 79 — because 28 and 79 had never been
evaluated. That is **AP15**, one level below where we had already fixed it. `t.Errorf` now.

**Second: the diagnosis was right, and we retracted it on a probe that could not test it.**
The row said *"Axis-1 has not adopted `EXTENSION-COMPUTE` v3.26's contained-error semantics"* —
correct, and §0 below had even cited the six core-go commits that are the spec of the change.
It was retracted on two probes that indexed a 2-element array out of range, bare and
`Construct`-wrapped, and found both engines byte-identical. They are: **a bare index is a
CONSUMED position, where both engines were already right.** The divergence lives only at a
*closure-result* position inside a collection primitive. A probe that does not reproduce the
shape refutes nothing — and reporting it as a refutation cost more than the original error,
because it replaced a correct explanation with "cause unknown" in a published CHANGELOG.

**Third: the instrument settled it in one run.** Regenerate the same seed, dump each diverging
case's IR, then evaluate **every subnode on both engines, children first**, and print the
deepest node where they disagree. All three cases turned out to be the same shape:

```
length( map(arr, λe. e + index(<2-elt literal>, <out-of-range>)) )
```

Every subnode agreed; `map` was the first that did not. **Stage-1 CONTAINS each element's error
as a value and `length` answers 4; Axis-1 propagated and answered `index_out_of_range`.**

### The fix — a position model, not a patch

The real gap was that Axis-1 had **no notion of an error as a value**. §1.5 makes a
`compute/error` an ordinary value, so every result site has two representations to handle
(minted, and value-form) and the decision is keyed on the *position*, never on which
representation showed up. `entitysdk/axis1/contain.go` is the transcription of all three
position kinds, and each call site now names which one it is:

- **CONSUMED** — the result is READ (arith/compare/logic operand, `if` condition, cast value,
  field target, construct field, index and its array, any collection operand, a filter
  predicate). Both representations short-circuit. New chokepoint `evaluator.operand`, mirroring
  the reference's `evalOperand`.
- **CONTAINED** — the result is PLACED without being read (`map`'s output element, `fold`'s
  accumulator and `initial`). Both become a value in that slot — the §1.5 NaN model — except
  `budget_exhausted` / `cascade_limit`, whose counters are not restored on unwind. `depth`
  *is* restored, so `depth_exceeded` contains like anything else.
- **BOUNDARY** — a contained error element materializes **code-only**, so two implementations
  that word the same failure differently still produce the same array bytes.

Three of those were outright wrong before; the filter predicate was half-right (it propagated a
minted error but ran a value-form one through `truthy()`, whose default arm returns `true` — so
an element whose predicate *failed* was silently **kept**). That one was never reachable in the
sweep and is the kind of thing this class of bug hides.

**Gate:** `TestAxis1Equivalence_ContainedErrorPositions` — five vectors, one per position, each
asserting the *exact* outcome both engines must produce rather than merely that they agree (two
engines can agree on the wrong answer, and before the fix several of these agreed on a
propagated error), plus a structural check that a contained error materializes code-only with no
`message`/`at`/`expression`. Verified to fail on the pre-fix engine before being kept. The
generator reached this class by luck of the draw; the vectors do not depend on that.

**What to carry.** Two things, and the second is the expensive one:

1. **A wrong probe is worse than no probe.** "I measured it and the explanation is dead" is a
   much stronger claim than "nobody has measured this", and it is the one that gets copied
   forward. Before a refutation retires an explanation, show the probe *reproduces the failing
   shape* — ours did not go anywhere near a closure-result position.
2. **When two implementations disagree on generated input, the generated input is the evidence.**
   Dump the graph, walk it bottom-up, evaluate every subnode on both engines. It took one
   throwaway test file and one run to convert three days of "unknown" into a named position in
   a spec. Reach for it first, not after a round of hypotheses.

### `make build` now refuses early when the sibling kernel is missing

`build` / `test` / `test-each` / `lint` / `gui` / `gui-build` / `shell-build` (so also `run`,
`demo`, `shell`) depend on a new **`preflight`** target that names the missing
`../entity-core-go` and the `git clone` that fixes it, in one sentence. `make doctor` had that
check from the day it was written and **nothing called it**, so a clone without the sibling
produced forty lines of module-resolution spew and no cause.

The predicate is defined once as `SIBLING_PRESENT` and shared with `doctor`, so the reporting
path and the refusing path cannot drift. Verified both ways: silent exit 0 with the sibling,
and the full message plus exit 1 under `make preflight PARENT=/tmp/no-such-parent`.

**No suite in this repo can regress this** — a suite that runs at all is running in a tree where
the sibling resolved, which is also why it survived so long. Filed as **AP41**: *an instrument
nothing calls is indistinguishable from an instrument you do not have* — D23 (*a model with no
shipped surface is not shipped*) in a second domain.

**And its second instance arrived the same day.** `IN_CONTAINER` hard-coded
`-w /src/entity-systems/entity-workbench-go`, so the checkout had to be *named* that — true for
every developer, false for a git worktree. In a worktree named anything else: `make preflight` →
**OK**, `make build` → **`No rule to make target 'build-native'`**. podman *creates* the missing
workdir instead of refusing, so make lands in an empty tree and the error names our Makefile
while the defect is the placement. Fixed by deriving it — `REPO_DIR := $(notdir $(CURDIR))` —
and verified with a differently-named worktree: exit 0, five binaries. **The check passed
because the sibling really was there: it answered the question it was asked, and the question
was the wrong one.** That sharpening is in the charter.

### The published document surface was the weak half

None of this was in the code. `CANONICAL-DOCS.toml` is the declaration of what this project
publishes, and nobody had read it since the disciplines grew.

- **Blurbs advertised counts that had gone false** — *"D1–D23"*, *"AP1–AP27"*, a *"six-boundary
  map"*, *"P0–P6"*, every one of them behind the document it described. A blurb is not a comment:
  it is the prose a reader is shown **instead of** the document. Rewritten to describe rather
  than count, after the same blurb went stale again inside the same day's diff. *(This bullet
  used to restate the then-current numbers, which made it the very thing AP42 is about; a
  catalog entry added on 2026-08-25 falsified it. The counts live in the charter and nowhere
  else.)*
- **The GitHub URL named an organisation that does not exist.** Fixed to match every remote and
  the README.
- **`DOCTRINE-CRASH-FORENSICS.md` was undeclared** while public `AGENTS.md` instructs the reader
  to open it at the start of any crash investigation. Declared.
- **The nine root documents were undeclared** — `README`, `CHANGELOG`, `CONTRIBUTING`,
  `CODE_OF_CONDUCT`, `SECURITY`, `AGENTS`, `AGENTS-STANDARD`, `METHODOLOGY`, `CLAUDE`. Seven were
  already published, and the keep-list is a keep-list: undeclared means dropped. **Left alone the
  next release would have deleted `SECURITY.md` — the vulnerability-reporting address — from a
  public repo.** All declared.
- **`CHANGELOG.md` said nothing about 156 commits.** It carried *"Initial public research-preview
  release"* under `[Unreleased]`, written before `v0.8.0` was cut and never touched. Now written:
  Added / Changed / Fixed at a thematic altitude, plus **Known limitations** stating the sibling
  requirement, the one known differential failure, both flakes, and the unresolvable pin count.

Both halves are **AP42**: *a manifest is published prose and it goes false silently*, because a
`.toml` is read as configuration and skipped by review. The second half is the sharper one — **an
omission in a keep-list is an act of deletion against anything already published**, so the
manifest wants reviewing against what is currently public, not only against the tree.

### The rolling log moved to `docs/STATUS.md`

Operator ruling, matching the fleet. `docs/status/` is stripped from the published tree, so a
rolling canonical log left inside it is one edit away from being swept up by a rule that is
otherwise correct. **The path is now the declaration** — outside that directory is canonical,
inside it is working memory. The dated `STATUS-*` snapshots and `HANDOFF-*` stay behind and stay
unpublished; `.release-removals` records that the old path is a **move, not a withdrawal**.

**The cost, stated rather than discovered:** publishing this log took the repo's unreachable
short-SHA citations from **43 to 117**, 72 of them here — `dev` SHAs that by [ADR-0027] resolve
for no public reader. It gates nothing, but `CHANGELOG.md` had disclosed "43" and would have
shipped a false number. Corrected there with the split named. Backlog **PR-5**.

**And the rule that comes with the move:** this file is read by strangers now. Write it for the
next session — that is what makes it useful — but a verbatim quote in it is a published quote,
and anything you would not want read by someone outside belongs in `docs/status/`, one directory
away, which exists for exactly that. Two things went wrong here before that sank in: a frank
quote tripped the release profanity gate, and then the write-up of a resolved credential-shaped
finding **reproduced it** by pasting the literal. **Describe a finding; do not reproduce it.**

### The front door claimed two things that were not true

Both were true when written, and neither had a reason to be re-read:

- *"The published vanity module path is wired up … the final cutover is a one-line change per
  module."* **`go.entitychurch.org` has no DNS record** — confirmed here, `getent` rc=2, while
  the apex resolves. There is no module identity to fetch, so **`go get` of this SDK is not
  available**, and the sibling `replace` is not a shortcut we took but the only thing available.
  Rewritten, and the limitation is stated outright in README and CHANGELOG.
- *"The repo is not git-tagged yet."* `v0.8.0` is tagged and public.

**The pattern is worth more than the two fixes:** the sentences that rot on a front-door document
are the ones describing a *transitional* state — "not yet", "still local", "the final cutover
is" — because the transition completes somewhere else and nobody revisits the paragraph. The
README's durable claims (the sibling requirement, stated three times) were all correct.

### The number is 0.9.0

Stamped in `CHANGELOG.md`, `README.md` § Versioning, the shell's unstamped-build fallback and
the GUI's `.csproj`. **The reasoning, so no session re-derives it:** the implementations
(`entity-core-go` / `-py` / `-rust`) moved to semantic versioning at 0.9.0 and this repo sits
directly on them. The core protocol's own number is a different scheme and is not one to copy —
the shared `0.8` across the ecosystem was an accident of adoption, not a coupling. No `go.mod`
needed touching: those `require … v0.8.0` lines name *core-go's* version, and the shipped
binary's version comes from `git describe --tags`.

### A waived skip was quoting a measurement that had gone false

`TestStorage_SqliteIdentityBundle_RebootstrapGrowsBoundedly` is skipped under a conscious waiver
— a real linear leak in the kernel's identity ceremony, routed long ago, not ours to fix. Its
text said *"WAIVED for the 0.8.0 preview"*, and a release cut is exactly when that stops being
true. **So it was re-measured rather than re-worded:** Skip removed, test run against `13a42ea`.

| | when waived | 2026-08-24 |
|---|---|---|
| `ΔpathCount` per reload | 1 | **0** |
| `ΔentityCount` per reload | 4 | **2** |
| bootstrap → reload-4 | 347/338 → 351/354 | 374/365 → **374/373** |

**The path leak is gone and the entity leak is halved** — the kernel has fixed part of this, and
the numbers justifying our waiver had silently become false. Still linear, so the waiver stands
on substance; re-scoped to 0.9.0 with today's figures inline, and the assertion re-arms by
deleting one `t.Skip`. Routed upstream.

**The general form:** *a waiver cites evidence, and evidence expires.* A skip whose justification
names a release has a built-in expiry, and the honest act at the next cut is to re-run it, not to
bump the number in the string.

### One credential-shaped constant renamed

`entitysdk/rendezvous_test.go` bound the well-known xkcd passphrase to a constant named `secret`.
No secret by any reading, but the **shape** is a credential assignment and a scanner cannot tell
the difference. Renamed to `passphrase`; `secret` remains the word wherever it is load-bearing —
`RendezvousModeSecret`, the test name, the SIGNALING §3.2 reasoning. We declined the alternative
of having the detector taught to ignore canonical placeholder values: widening a credential rule
to suit one test is a worse trade than naming a variable accurately.

**A related one to carry:** `AGENTS.md` is written for an internal audience and is now published.
Internal infrastructure paths are what such a document is *made of*, and three of them had to be
rewritten to say what they mean without naming machine-local or internal locations — those belong
in the git-ignored `AGENTS.local.md` / `.agents/` ([ADR-0020]). **Declaring a document changes
what "internal" means about it; the manifest edit is not finished until the file has been re-read
as a stranger.**

## §0c NEW (2026-08-25) — the gate that would have named the bug on day one exists, is arch's, and we do not run it

Written immediately after §0A, because looking for coverage of the `filter` half of that defect
found something bigger than the defect.

**`TestAxis1Admission_*` skips unless `AXIS1_ADMISSION_CORPUS` is set, and no `make` target sets
it.** So it has skipped in every sweep since it was written, and a green `make test-sdk` has never
attested anything about AE-5. That is **AP41 in a second domain** — an opt-in check is not a gate —
and it is why §0's first correction paragraph could claim the admission was re-quotable without
anyone noticing nothing had run.

### What it says when you actually run it

Corpus `8d2f55c8…`, profile `inproc`, **362 vectors**, generated from core-go `13a42ea`;
reference emission from core-go in-process; Axis-1 emission at `6ab42c6`:

| | |
|---|---|
| agree | **334** |
| diverge | **0** |
| INCOMPLETE (deopted to Stage-1) | **28** |
| verdict | **NOT LOCKED** |

Two readings, and both matter:

- **The good half is genuinely good.** Every vector Axis-1 *answers* is byte-identical to the
  reference — 334 of them, including the whole v3.26 contained-error family. That is far stronger
  evidence for the §0A fix than the five vectors we hand-wrote, and it is independent of us.
- **The bad half is that AE-5 is not green and has not been for some time.** §11's **AE-6** is
  explicit: *"the alternate engine MUST run every vector; no per-vector fallback … deopt during an
  admission run voids the evidence for that vector."* 28 deopts means 28 voided vectors. **Stop
  quoting the 2026-07-23 admission as current** — it lapsed when the v3.24/v3.25 primitives landed
  and nothing told us, because nothing ran.

### The corpus would have named this bug on day one

Re-run against the **pre-fix** engine, it produces **five two-way divergences**, and the vector IDs
are the diagnosis:

```
cv8a-map-contains-minted-error
cv8c-filter-predicate-error-shortcircuit
cv9a-map-depth-exceeded-contains
cv9c-map-valueform-budget-exhausted-shortcircuits
sweep/0281
```

Our home-grown 300-case fuzz found three anonymous cases and cost three days plus a wrong
retraction to explain. Arch's corpus **names the class in the vector ID**. It also carries eight
`worked/value-error/*` vectors — one per consumed position — which is the exact table §0A's fix
had to derive by reading core-go's source.

### And 12 of those vectors cannot currently reach the code they test

Of the 28 deopts, **12 are value-form-error vectors that fail one node too early**: Axis-1's
decoder sends a `compute/error` **leaf** to the Stage-1 fallback (`decode.go`'s `default` arm —
"value types … go to Stage-1"), so the vector never reaches the CONSUMED/CONTAINED logic it was
written to test. Measured: adding a single `case types.TypeComputeError → litNode{value: ent}`
takes deopts **28 → 16** and every recovered vector cross-blesses byte-identical. **That change is
not in `6ab42c6`** — it is measured, not landed, because it wants its own diff and its own review.

The remaining 16 are honest gaps, not a decode artifact: 11 are the v3.24/v3.25 primitives
(`assoc` / `concat` / `group-by` / `range`) that Axis-1 has never implemented — that is backlog
**PR-C**, whose real size this measures for the first time — and 5 are dispatch-mode `apply`,
which is deopt **by design** (`doc.go`'s scope fence). Those 5 need arch's ruling, not code: AE-6
admits no fallback, and Axis-1's declared scope excludes dispatch. Either the corpus profile grows
a pure-only subset or the scope fence moves.

### Rows this opens

- **PR-D — wire the admission corpus into a `make` target.** The one-line runner is in `AGENTS.md`
  now; a target that generates, emits, and cross-blesses is the actual fix. Deliberately not
  `test-native`: it needs a buildable sibling, and a sweep that can go red for a neighbour's
  reasons teaches people to ignore the sweep (same rule as `crossimpl-go`).
- **PR-E — decode `compute/error` as a value leaf.** Measured above: 12 vectors, 0 divergences.
- **PR-C is now sized** — 11 corpus vectors, named.
- **Ask arch:** how does an engine with a declared scope fence satisfy AE-6? (§0c, routed in
  `reviews/AXIS1-ADMISSION-LAPSED-2026-08-25.md`.)

### Operator ruling, 2026-08-25 — the engine's home is the compute extension

Recorded here so it is not relitigated. **`entitysdk/axis1` does not belong in an SDK**: an SDK is
an interface layer, and the only thing that implements compute is the compute extension. An
alternate engine is not a research toy — it is **what makes compute practical**, because without
the collapse to a condensed handler you re-hash every intermediate on every tick. So it belongs
with the extension it makes usable, **under the same vectors and the same standards, exercised on
both engines whenever compute changes**. Someone re-implementing for their own deployment reasons
is fine and expected — under those same vectors.

**The objection this repo had been carrying is withdrawn.** `doc.go`'s *"sharing code with the
reference would make the equivalence oracle circular"* defends **code** independence — which is
precisely what produced this drift — while the independence that actually catches errors is
**vector** independence, i.e. arch's corpus, which does not care where the engine lives. Nothing
in the proposal asks the two engines to share code; it asks one corpus to run over both, in the
repo where the semantics change.

Structural evidence, measured: `axis1` is **3,028 lines with zero dependencies on this repo**
(`go list -deps ./axis1` is core-go only), implements core-go's `handler.Handler`, and is consumed
by `programs/` through a **handler-path string** rather than an import. It is not integrated here;
it is parked here, because that is where the prototype was typed. Routed as
`reviews/PROPOSAL-AXIS1-HOME-IS-THE-COMPUTE-EXTENSION-2026-08-25.md`.

*And the honest frame: this was a prototype built fast to prove entity-compute could carry a real
interactive workload. It did. The placement was a deliberate corner cut, and both halves of that
trade are now visible — the capability and the maintenance arrangement nobody would design.*

## §0aa NEW (2026-08-23) — the re-sync against core-go's compute work found two live defects, neither of them in compute

**Why the session ran.** `entity-core-go` moved 25 commits after the close-out — most of it
compute (`COMPUTE v3.26`, the §3.5 Corner-1 rulings, the eval-limit carve-out, the corpus
re-freeze 359→362, and `0e34e3e`'s §5.2 depth-budget fix). Every `go.mod` here resolves the
kernel through a `replace` to the sibling working tree, so "core-go moved" and "our build
inputs moved" are the same event. The ask was narrow: **confirm the tree still builds and the
compute surface still works — not to extend it.**

**The compute answer is clean.** `make build` green against core-go `3b4b1e9`; `make test-each`
9 PASS / 1 FAIL with the *same single* known failure (`TestAxis1Equivalence_Differential`
case 9), so none of core-go's six compute changes introduced a new divergence. `programs`
(158s) passes, which is the generic host over `system/compute` at their HEAD.

**But driving the shipped compute verb by hand found two defects, and they were not compute's.**
D19 earned its keep here — the suites were green and the source read fine; only running the
operation surfaced these.

**1 — the query index did not survive a restart, so three verbs went blind.** `find`, `grep`
and `compute aggregate` all route through `system/query`. core-go ships only in-memory query
indexes (`MemoryTypeIndex` / `MemoryReverseHashIndex` / `MemoryPathLinkIndex`), populated
solely by the `"query"` sync hook — i.e. only by writes made *during this process*. With
`-storage sqlite` the tree persists and the index does not:

```
session 1:  put …/files/a … ;  find files a  →  1 match
session 2:  (same DB, new process)  find files a  →  (no paths under files match "a")
            ls …/files                          →  a          ← the entity is right there
```

That asymmetry is why it reads as *"the entity is gone"* rather than *"the index is empty"*,
and why no single-process test in `entitysdk` could see it. **The kernel already had the
answer and we never called it** — `query.IndexMaintainer.Rebuild(li)`, whose own doc comment
says *"use for recovery or startup with persisted stores."* This is **D20 exactly**: the fix
was one call, and the D20 grep is what found it rather than a plan to build an index. Landed in
`assembleAppPeer`, unconditional (an empty tree makes the scan free, and a caller supplying a
persistent `LocationIndex` via `RawOptions` gets the guarantee without knowing to ask).
Gate: `entitysdk/query_index_restart_test.go` — **mutation-checked**, reports 0 matches with
the `Rebuild` call disabled.

**No packet is owed to core-go for this, and the check that established that is the point.**
The obvious next move was to route it — "your query indexes go blind on a persistent store" is
a serious claim about a reference implementation days before a release. It is also **false**:
`cmd/entity-peer/main.go` already calls `queryMaintainer.Rebuild(p.LocationIndex())`,
unconditionally, with the same rationale we arrived at and a citation to
`DESIGN-SQLITE-PERSISTENCE.md §4.3` we had not read. **Our wiring had diverged from the
reference wiring at a step nobody diffed.** Reading their call site cost a minute and stopped a
wrong packet (D19: route the measurement, not the argument) — the standing lesson being that
when we re-implement a kernel assembly the kernel also performs, an omission is **ours by
default** until their call site says otherwise.

**2 — `compute aggregate` could not read a size written by the shell's own `put`.** `put`
decodes its payload with `json.Unmarshal` into an `interface{}`, so every JSON number becomes a
`float64`, and CBOR core-deterministic encoding does not fold an integral float back to an
integer. `extractNumericSize` accepted only the integer kinds. The result, on the most obvious
gesture in the verb's own usage line:

```
put …/files/a app/file '{"name":"a","size":100}'   ×3
compute aggregate files
  → (no entities with a numeric .size field under files; 3 entities scanned, 3 skipped)
```

The verb whose stated purpose is *"demonstrate compute usefully aggregates over real workbench
entities"* could not aggregate over one. **Its own unit coverage was green throughout** because
those cases build Go ints directly — the bug lives exactly at the seam between two verbs, which
is the part neither verb's tests own. Non-integral sizes are now **refused, not truncated**
(AP33; also what the fold requires, its accumulator being a `uint64` literal).
Gate: `shellcmd/cmd_compute_size_test.go`, which encodes through `put`'s own path and carries a
premise-guard so a future encoder change cannot make it vacuous.

**Re-measured end to end through the shipped binary, across the process boundary:**

```
session 2 (fresh process):  find files a           → 1 match(es) of 3 entities under files
                            compute aggregate files → count = 3 · total_bytes = 999
```

`999 = 100+250+649` — a real `LowerFold`/`add` dispatched to core-go's `system/compute` at HEAD.

**Two doc facts corrected while re-syncing** (both were true when written, both had expired):

- **`types.BuildContentURL` is fixed upstream.** `AGENTS.md` said it emits digest-only hex and
  told us not to delegate to it. core-go `7f39eb3` landed the full-wire-form; it now hexes
  `h.Bytes()` (33-byte, format byte included). `fetch/crossimpl_test.go`'s tripwire was written
  to notice exactly this and now logs the agreement. The delegation itself still owes a
  round-trip measurement before `fetch.contentURL` changes — **noticing is not adopting.**
- **Axis-1's constraint reader is now a stale mirror** — see §0b below.

**Nothing was routed to us.** D21 sweep of `../entity-core-go/docs/status/` by subject: the
newest documents naming this repo are the 08-20/08-21 handoffs already read. `0e34e3e`'s ask
(*"the cross-impl re-drive of the 362 corpus"*) names **rust/py, not us**, in its own text.
## §0b NEW (2026-08-23) — Axis-1's constraint reader drifted, and it is recorded rather than fixed

core-go `0e34e3e` replaced `ext/compute/handler.go::extractComputeConstraints`, which
`entitysdk/axis1/handler.go` transcribes verbatim by design. Per `EXTENSION-COMPUTE` §5.2 the
eval limits come from the **matching grant's** `constraints["system/compute"]`; a capability
token carries no top-level `constraints` field at all, so the old reader — ours, still — means
**no compute constraint has ever reached the Axis-1 evaluator.** Their replacements are
`computeConstraintsOfGrant` / `computeConstraintsOfToken`.

**Deliberately not fixed this session**, on two grounds: Axis-1 is registered by tests only (no
binary, bridge or panel reaches it — `make reachability` agrees), so it changes nothing a user
runs; and adopting it is engine-semantics work that belongs with the v3.26 contained-error
adoption in §0, not ahead of a release. **What did land is the honesty**: the transcription's
doc comment now names the drift and cites `0e34e3e`, because *the transcription is the
contract* and a comment claiming a lapsed fidelity is worse than no comment.

**Carry both rows together into the post-release backlog (PR-B + PR-3)** — they are one piece of work
(Axis-1 catches up to COMPUTE v3.26 *and* to §5.2), not two.

> **2026-08-25:** the v3.26 half (PR-B) is done — §0A. This half is not, and it turned out to be
> genuinely separable: the fix was a position model over error *values*, and it does not go near
> the constraint reader. The pairing was a good bet on shared context, not a real dependency.

## §0a NEW (2026-08-22) — Regen was sliding one fixed pattern, and the operator's phrasing was the measurement

**Reported:** *"Regen should just basically pick a random seed… right now it just seems to
iterate this weird ladder. If I keep hitting Regen it seems to just move it one or two over."*
That is not an impression. It is a description of a **translation**, and it was exactly right.

**The defect (AP38).** `programs/life_edit.go`'s regen soup hashed `(gen, i)` as
`LCG(gen·C + i)` and read bits 16..18. Under a power-of-two modulus, bit *k* of an LCG step
depends only on bits 0..*k* of its input — so those three bits depend on nothing but
`(gen·C + i) mod 2^19`, and **changing the generation counter is arithmetically
indistinguishable from changing the cell index by a constant.** The 256-cell board was a
window into one fixed pattern; Regen only slid the window. Measured before the fix, best
agreement under a cyclic shift:

| generation gap | agreement | shift |
|---|---|---|
| 1 | 0.980 | 9 cells |
| 2 | 0.965 | 18 cells |
| 6 | 0.961 | 2 cells |
| 60 | **1.000** | 2 cells |

The board is 16 wide, so a 9-cell shift is half a row — *"one or two over"* is the arithmetic
read off the screen.

**Why every test was green.** `TestLifeEdit_RegenReplacesBoard` had an explicit anti-vacuity
clause — `if lifeCellsEqual(before, after) { t.Fatal("regen did nothing") }`. **A translation
is never equal**, so the clause passes on every one of these boards. The test asserted
*different* where the property that mattered was *independent*. The second tell was free and
unread: a translation preserves the live-cell count, so population across regens had σ = 0.76
where an independent draw at 3/8 density gives σ = 7.75 — every board it ever produced had
exactly 96 cells alive.

**The fix.** One nonlinear round: square the mixed value (so the counter's contribution depends
on the index it is mixed with — no shift can reproduce that), then fold the square's high bits
down before the LCG step, because squaring mod 2^k leaves the low bits weak. After: agreement
0.578–0.734 under any cyclic shift, cell-for-cell agreement 0.530 (chance for two independent
boards at this density is 0.53125), population mean 96.5 with σ 7.33 against a theoretical 7.75.

**And no, compute has no randomness — that is correct and load-bearing.** Reproducible state
hashes are the whole point of the programs track; a mirror that re-derives a program's state has
to get the same bytes. The entropy is the **tick counter at the moment of the press**, which is
unpredictable to a human hand and exactly replayable to a machine. The bug was never determinism;
it was a hash too weak to look like one.

**Gate:** `TestLifeEdit_RegenIsNotATranslation` — two regens through the **running host**, an
oracle sweep across generation gaps 1…1000, and the population-σ check, with `lifeBestCyclicMatch`
as the instrument. Thresholds calibrated over 2500 board pairs (defective 0.953–0.992, fixed
0.578–0.734, cut at 0.85). **The gate was run against the old hash and shown to fire on all seven
gaps before it was committed** — a regression test never shown red is decoration.

**Ratchet:** AP38 in the charter (with its enforcement paragraph), and a `programs/` entry in
`AGENTS.md` — *compute has no randomness, so a program's "randomizer" is a hash, and a hash
linear in its varying input is a translation.*

**Reachability note.** The GUI embeds the Go program authoring, so an operator sees this only
after `make gui` (image rebuild) — `make gui-run` alone will keep running the old soup.

## §0b Close-out review (2026-08-22) — the D21 sweep is clean, and one decision is left

**Run at close-out, so "cap it unless something major comes through" is a measurement and not
an assumption.**

- **Arch: nothing new addressed to us.** The newest packet naming this repo is still
  `ROUTING-2026-08-21-k` (read and answered last session; pin replied in
  `reviews/COMMIT-PIN-REGISTRY-1.21-PROVENANCE-2026-08-21.md`). Arch has three commits since —
  a `METHODOLOGY.md` overlay re-sync and two on their own publication/crawl-path track — and
  **the diff of `COHORT-OPEN-ITEMS.md` and `WORKSTREAMS.md` since then adds no line naming
  workbench-go.** Checked by subject and by diff, not by filename (AP28).
- **`METHODOLOGY.md` overlay is in sync** — byte-identical to arch's, and tracked/committed here.
- **`entity-browser-rust`: nothing routed to us.** They have moved to connectivity, rendezvous
  and acquisition; no commit since 2026-08-21 names this repo. Our
  `reviews/GENERIC-HOST-SUBTICK-INPUT-2026-08-21.md` is committed and pushed (`7729cb5`), which
  is what delivery means; **no reply is owed to us** — it was routed as a design result.
- **Nothing is blocked on us.** Every row in "Waiting on" below carries an explicit
  *blocked on us: nothing*, and the one row that used to read "blocked on arch" while never
  having been sent (pointer input) is delivered and sitting on arch's board as W-1.

**Axis-1's drift does not touch the shipped surface.** `entitysdk/axis1` is imported by nothing
outside its own package and tests — no binary, no bridge, no panel. So §0 was a **stale
conformance claim** (AE-5, `EXTENSION-COMPUTE` §11, LOCKED 2026-07-23), not a defect in anything
a user runs. Per ADR-0012 that distinction is the whole point: **the release must not restate the
AE-5 admission as current** until the engine adopts v3.26 contained-error semantics.

***The v3.26 semantics are RESOLVED 2026-08-25 (§0A). The AE-5 admission is NOT, and the first
version of this paragraph said it was.***

**Correction.** This paragraph originally read *"the AE-5 corpus still runs byte-exact with the
new semantics — `make test-sdk` green under `-race`, 330 vectors — so the admission is
re-quotable."* Every clause of that is wrong, and it was written without running anything:
`TestAxis1Admission_*` is **gated on `AXIS1_ADMISSION_CORPUS` and skips by default**, so
`make test-sdk` green attests nothing about AE-5; the corpus is **362** vectors now, not 330; and
when actually run, **the admission does not lock.** See §0c.

The same reflex as AP43, one day later and pointed at a different fact: a green suite was read as
covering a gate that suite does not run. **A skip counts as a failure** (AGENTS-STANDARD), and
this one had been skipping in every sweep since the harness was written.

**~~The one decision left: `dev` is ahead of `master`.~~ RETRACTED 2026-08-23 — there was never
a decision here, and carrying one was the error.** `master` is the **public canonical mirror**
and is **reset to public after each release**; promotion `dev → master` is the *release act*,
performed by maintainers / the release team through the central runbook, per ADR-0015 and its
2026-06-30 amendment. **`dev` ahead of `master` is the normal steady state** — it is simply
unreleased work. Framing it as *"an operator call"* pending
in our status file put a release-team act on our board and invited a future session to act on
it. Nothing here is owed, and the row is closed rather than answered. The rule now lives in
`AGENTS.md` under **Boundaries — do NOT modify**, which is where it can actually stop someone.

## §0 — CLOSED 2026-08-25. Axis-1 had drifted from COMPUTE v3.26, and our own differential gate caught it

> **Closed by the fix in §0A above.** Kept verbatim because **this section was right**: it named
> the cause and cited the six core-go commits that are the spec of the change, on 2026-08-23. It
> was overturned on 2026-08-24 by a probe that could not reach the failing position, and the
> retraction — not the original — is what had to be undone. If you are reading this because a
> written-down explanation is being challenged, the question to ask is whether the challenge
> *reproduces the shape*, not whether it ran cleanly.

`TestAxis1Equivalence_Differential` (300-case fuzz, fixed seed 20260716) diverges at case 9:

```
stage1: entity:ecf-sha256:c71e6433776fe58280ee9cfa69bebf6e07bce058546dd7eb75a3f2e5df013f95
axis1:  error(index_out_of_range: index 4 out of range for array of length 2)
```

**Stage-1 CONTAINS the error and returns a value; Axis-1 short-circuits.** That is the exact
direction of six `entity-core-go` compute commits dated **2026-08-21** — `7f39eb3` (COMPUTE
v3.26 contained-error), `ded9ea0` / `4519554` (a consumed operand short-circuits an error, not
`type_mismatch`), `eea0a6e` / `9ad0110` / `d5e318d` (§3.5 Corner 1: filter/fold/map closure
results contain and recover). Our last green `sdk` sweep was **2026-08-20**, before all six.

**Why this matters more than a red suite.** Axis-1 is not a research toy — it is a
**conformance-admitted alternate engine** (AE-5 / `EXTENSION-COMPUTE` §11, 330 vectors, LOCKED
2026-07-23), and it is **workbench-owned**, so this is ours to fix and nobody else's. The
admission is stale until it adopts the new semantics.

> *2026-08-25: the "conformance-admitted" clause above is no longer true and the sentence after it
> understated the problem — the admission did not go stale pending v3.26, it had already **lapsed**
> when the v3.24/v3.25 primitives landed. **WITHDRAWN**, see §0c and §1a. The "not a research toy"
> half stands and got stronger: it is what makes compute practical, which is why the operator ruled
> its home is the compute extension rather than an SDK.*

**Not started, deliberately.** It is a real piece of work (error containment touches every
collection primitive and the fold accumulator) and it was not this session's ask. Two things
that will save the next session time:
- The five core-go commits above are the spec of the change, and each names its arch ruling.
  *(2026-08-25: this was the right pointer. The fix was transcribed from exactly these, and the
  estimate above — "touches every collection primitive and the fold accumulator" — was accurate,
  though it under-counted: the consumed positions outside the collection builtins needed the
  matching short-circuit, and the boundary needed the code-only reduction.)*
- **Do not read arch's "compute stays sequenced / deferred for you" as covering this.** That
  deferral is about the compute-floor research track (T5). This is an admitted engine drifting
  from a landed spec revision, surfaced by our own gate — a different thing that happens to
  share the word.

**Previously (2026-08-20, after the consume leg — §12).** `make test-each` was run **twice** this
session and both runs are reported, because the difference between them is the point:

| run | result |
|---|---|
| before the last edits | **all ten PASS** — sdk 201s · shellcmd 288s · programs 138s · rest ≤13s |
| **final tree** | **nine PASS, `shellcmd` FAIL** — sdk 194s · shellcmd 307s · programs 142s · rest ≤13s |

**The one failure is `TestE2E_Bidirectional_BurstWrites_NoFS`, and it is the known one** — the
terminal last-burst-write loss documented below and routed as
`reviews/CORE-GO-LAST-BURST-WRITE-LOSS-2026-08-20.md`. Our own classifier named it at the moment of
failure: **VERDICT (B1) — NEVER CAPTURED**, `archives/notes/a-4.md` still held by the writer and in
no version. It is the documented load-dependent failure, it is the only failing test in the suite,
and **nothing in this session's diff is in its path** (the reproducer builds its peers straight
through `entitysdk` with no filesystem, no localfiles and no workbench/fetch code). Per AGENTS.md a
passing targeted re-run would **not** be evidence it was spurious, so none is quoted here — one of
two full sweeps hit it, which is the same shape the bug has had since it was diagnosed.

`make lint` clean · `gofmt -l` empty · `make reachability` clean · **Avalonia 63/63 headless**
(three new `PublisherVerifyPanelTests`) · **`make crossimpl-go` green** (live, cross-impl).

**Previous merge gate, for the record:** green as of `2efeb9d` — all ten suites to completion, zero
failures, `sdk` 196s · `shellcmd` 287s · `programs` 140s. That was the sweep that preceded
`dev` → `master`.

**Avalonia: 60/60 headless** (`make -C avalonia test`, 2026-08-20 — three new liveness tests),
plus `make smoke-xvfb-handlers` green under real X11 + software Skia — 21 handlers walked,
exit 0 — and the new `make smoke-xvfb-connections` (§8).

**The burst flake is diagnosed, and it is a terminal write loss — not a flake and not saturation.**
`TestE2E_Bidirectional_BurstWrites_NoFS`, reproduced 5× under load (0-in-10 idle). The
`heads_equal=true` signature the last session flagged as a lead was one, and it pointed here:

- The heads AGREE, and the agreed head's trie commits to **9 of 10** paths. The lost path is in
  **no version at all**.
- **The peer that WROTE it still holds it; the counterpart never gets it.** That discriminator
  rules out a merge wipe — a wipe takes the writer's copy too. The write was **never captured**.
- Always the **last write** of one peer's burst, symmetric between peers, and **terminal**: the
  head is settled, nothing further is emitted, and no later merge can recover a write no version
  ever named.

**core-go's own source names this failure and says it was fixed.** `ext/revision/auto_version.go`'s
CAS-retry comment calls it *"the symmetric last-burst-write loss pattern… diagnosed by the
workbench in the F10 part-3 results."* It still reproduces. The structural hole is that **every
give-up path in `fire()` recovers via "the next sync-hook event will fire() again", and the last
write of a burst has no next event** — which is exactly why it is always the last write and never
an interior one, and why it is the write under maximum contention.

Routed: `docs/architecture/reviews/CORE-GO-LAST-BURST-WRITE-LOSS-2026-08-20.md`. **Not fixed here**
— the reproducer builds its peers straight through `entitysdk` with no filesystem, no localfiles
and no workbench model layer, so the workbench is not in the failing path.

**What we could not determine, recorded rather than guessed:** which give-up path fires.
`AutoVersioner.debugf` would say in one line, but its output never appeared in ours — the
`PeerConfig.DebugLog` we set does not reach the revision AutoVersioner's logger. Asked for
alongside the main finding. The §3 argument holds whichever path fires, and it is routed as that
rather than as a measurement (D19/AP10).

**The harness now classifies at the moment of failure** instead of leaving it to a reader.
`classifyConvergedHeadFailure` walks the agreed head's trie and prints one of four verdicts —
(A) apply/projection gap, (B1) never captured, (B2) captured then wiped, or inconclusive — so the
next occurrence arrives as evidence. The old message printed `heads_equal=%v` on one line, which
read as a paradox and got the failure shelved under a label that could not explain it.
**`applyBindings` was our prime suspect from source reading and the measurement ruled it out** for
this failure; the B1/B2 split is what keeps that honest.

**Latest arch packets read: `ROUTING-2026-08-21-c` / `-f` / `-i` / `-k` (all addressed to us) plus
`-n`, at arch `b61bafb`** — read 2026-08-21-c. Four packets naming us had landed since the previous
marker and none had been opened (D21). What they move:

| packet | disposition |
|---|---|
| **`-k`** §1/§3 — *arch folded `EXTENSION-REGISTRY` 1.21 on a document that exists in no commit*: our whole registry session was untracked, so the provenance chain for a normative spec revision terminated in a working tree | **Closed, first action of this session.** Three commits, `dev` at **`a6b5e9d`**, pushed. The packet arch folded on is `reviews/REGISTRY-BINDING-TRANSPORTS-DIVERGENCE-2026-08-21.md` at `a6b5e9d`; the code that produced it is `64dcc81`. Replied with the pin: `reviews/COMMIT-PIN-REGISTRY-1.21-PROVENANCE-2026-08-21.md`. Our own fixture README had stated this exact rule about **someone else's** gitignored artifact hours before we left our own packet uncommitted — folded into AGENTS.md as *a packet that is not committed has not been routed*. |
| **`-f`** / **`-c`** — `transports` is **RULED our way and folded**, REGISTRY 1.20 → **1.21**: `[<system/hash, BARE>]` at all five declaration sites, plus D8a (a publishing registry MUST serve what its bindings reference) and D8b (**our finding #3**, the unimplementable §6a.3a prefix, corrected) | **Closed in our favour, three of four findings folded.** No code change owed yet; the consequences are the re-cut below. Arch records our posture — liberal decode, forms kept distinguishable, *"did not make our SDK succeed where the reference implementation fails"* — as the reference one. |
| **`-i`** §2 — **re-cut `fetch/testdata/crossimpl-rust-federation/`** from `entity-browser-rust`'s now-**committed** `tests/fixtures/registry-federation/` at `54f31a7` | **OPEN, unblocked, and the next item on this track.** `TestKernelCannotDecodeARustBinding` should flip (honour its own in-file guard rather than deleting it), the inline branch of `fetch.NameBinding`'s decoder goes, `TransportRef.Kind` **stays** (a decoder that can name the rejected shape is the better diagnostic), and the README's provenance caveat is discharged. Also: their `.list` files are now `system/tree/listing` ECF entities with a **canonical-CBOR** `entries` map — length-first then lexicographic — so **sort in the reader**. |
| **`-i`** §2.1 | Arch had our step and core-go's in series; they are parallel. **Nothing is waiting on us.** |
| **`-k`** §5 / **`-i`** §3 — compute | T5 stays **DEFERRED**; `EXTENSION-COMPUTE`'s fold to v3.27 is explicitly **not addressed to us**. *(Note §0 above: that deferral does **not** cover Axis-1's drift from v3.26's contained-error semantics, which is a different thing sharing a word.)* |

**Previously: `ROUTING-2026-08-21-b` (addressed to us), at arch `bded94c`** — read
mid-session on 2026-08-21, *before* the build it concerns had landed, and it moved three things:

| what | disposition |
|---|---|
| §1 — the **content-URL shape** we filed is **RULED our way**, `EXTENSION-SUBSTITUTE` 1.2 → 1.3: `{hash}` is `hex(H.Bytes())`, format byte included, fleet-wide | **Closed in our favour.** `TestContentURLUsesWireHexNotDigestHex` stops being a local pin and becomes the spec's rule. No code change here; `fetch` already did this. |
| §3 — **build `EnumerateNames`**: §6a.3a is *specified, one producer, **zero consumers***, and arch records that browse was cut from v1 on a **false premise** (*"the shipping application has the browse surface"*) — corrected to *"`entity-browser-rust`'s browse surface has never walked a registry"* | **Built this session.** `fetch.Registry.Enumerate`. Arch's only ask — *"say which produced a row **in the artifact**, not only in the code"* — is satisfied: `RegistryRow.Committed`/`.Listed` + `BrowseOutput.NamesAuthority`/`.NamesNote`, and every surface prints it. Arch is explicit this does **not** reopen v1: a new consumer is a build, not a finding. |
| §5 — **R-9 (`hints` round-trip) goes LIVE the moment `PinRegistry` writes chain entries**, which it now does | **Answered both halves.** Our `InstallResolverConfig` takes the whole decoded struct and re-encodes it — no field-by-field rebuild — and `Hints` is `map[string]cbor.RawMessage`, so an unknown key survives untouched; `addPeerIssuedChainEntry` **appends** rather than reconstructing. And the *authoring* side, which genuinely was absent, now exists: `PinnedRegistry.MaxTTLMillis` / `.NegTTLMillis`. |

§5 also confirms **W-1 (the pointer proposal) is still arch's and still sequenced** — nothing
blocked on us.

**Previously: `ROUTING-2026-08-20-l` (addressed to us), at arch `f3e81e2`** — plus
`-k` / `-m` / `-n` / `-o` and the `WORKSTREAMS` rows naming us in the same pass (D21: a `cc` is a
packet). **`-l` closes the AP28 loop from the other end** and nothing in it is blocked on us; see
§12. **Before that: `ROUTING-2026-08-20-e` + `-h` + `-i` + `STATUS-2026-08-20-c`,
at arch `8dd5689`** (§11). `-e` is addressed to us and carries the compute disposition; `-h`/`-i`
are core-go's and name us in their fold order (D21 — a `cc` is a packet), and neither asks anything.
**Before that: the whole earlier 2026-08-20 set** — `STATUS-2026-08-20-b`,
`HANDOFF-2026-08-20` (arch), `ROUTING-2026-08-20-a/-b/-c/-d`, plus `ROUTING-2026-08-19-k` and
`STATUS-2026-08-19`, all at arch `a5dfff3`. Eight documents naming this repo had landed since the
previous marker and none had been opened — see §9. **Previously: `ROUTING-2026-08-19-j`** (arch `05faaa5`, carrying **REGISTRY 1.18**);
`-19-i` at arch `3dd5800` (**REGISTRY 1.17**) read in the same pass. browser-rust's
`ROUTING-2026-08-19-d` read at their `fbc2c5c`. D21 — this line is the subtraction that tells the
next session what it has not opened. Read *every* document naming this repo, `cc` included:
`grep -ril 'workbench-go' ../entity-system-architecture/docs/status/`.

**Inbound, all answered:** `-19-b` §2 (validator widening → §6c), `-19-c` (the registry board is
closed; our only row was the widening), browser-rust's `-19-d` (consume-us ask → §6d, plus their
F6 correction folded in as AP22), arch's `-19-d` (→ §6e), and **`-19-i` + `-19-j` (→ §6f — both
addressed to core-go, both moving a MUST we had shipped the day before; one live defect)**.

## Where it is

The Go **reference application** built on the Entity Core Protocol — an opinionated,
worked example of an entity-native app, **not** a conformance implementation and not a
mandate. It is a leaf in the stack (depends on the `entity-core-go` kernel — `core` + `ext`
— via local `replace`; nothing depends on it). It ships the in-tree `entitysdk/` (the de
facto reference Go SDK: typed wrappers, storage, identity bundles, revision/continuation/
discovery helpers), **`entity-shell`** (the primary CLI and leading edge of feature work),
an **Avalonia/.NET desktop GUI** driven through a Go c-shared bridge (podman-only build),
a frozen **`console`** (tview) TUI kept as a renderer-neutrality enforcer, the
**`programs/`** entity-native programs track (generic compute host + descriptors), and a
small CDN corridor (`entity-publish` / `entity-vcs` / `entity-fetch`, plus
`entity-seed-site` / `entity-serve-cors`). Maturity: **v0.8.0 research preview**.

`master` carries v0.8.0. **`dev` is well ahead of it** and is where the compute work lives;
it is deliberately unmerged (see the guardrail below).

## Where we left off

**Latest handoff:** `docs/status/HANDOFF-2026-08-25-contained-errors-and-a-gate-we-were-not-running.md`
— **start there** for the current tip: the Axis-1 fix, the lapsed AE-5 admission, the recommended
order (PR-D first), and the traps that will bite a fresh session. Then
`docs/status/HANDOFF-2026-08-20-bearings-and-audit.md` for the wider bearings. It is
the audit, not a session log: tree state, where every arc stands, what is owed each way, the one
remaining renderer gap, and a recommended order. `HANDOFF-2026-08-20-piece-four-and-two-real-bugs.md`
is that session's log; `HANDOFF-2026-08-19-reachability-front-door-and-the-rendezvous-hold.md` the
one before. **Piece 4 — the rendezvous DISCOVERY backend — is BUILT**
(`021e5c2`), so the four-piece connectivity list is complete; §3 piece 4 has the result and what
it routed. Also this session: a live conformance defect fixed under D21 (§6f — our boot refused
to start on a config REGISTRY 1.17 says it MUST run under).

Three threads: the share arc (open, moving), the compute floor (**deferred by operator decision,
resumes after the release** — §11), and a closed stabilization pass.

### 1. The compute floor (the primary arc — Doom-class realtime) — WORKBENCH SIDE IS DRY

Six sessions (2026-07-15 → 07-19) characterized parallel compute on the entity model
end-to-end. **Everything on the workbench side is built, measured, and green; every
remaining lever is gated on arch or core-go.**

The one doc to read is
**`docs/architecture/reviews/COMPUTE-SHARDING-INTO-HOST-2026-07-18.md`** — §10 is the ledger
of all five routed levers and who owns each; §11 is the latest result. Not duplicated here.

Headlines, so this doc stands alone:
- **Host-managed static-k sharding** is a real mounted-program mode, not a test rig. The
  gate proves *parallel == serial == unsharded* state hashes, generation for generation.
- **64×64 reach** through the generic host against an independent Go Life oracle
  (~850 ms/tick on Stage-1 — correct, not realtime).
- **The ~1.8× parallel ceiling was diagnosed, not accepted.** A compute-bound control shows
  speedup climbing with op-cost (1.70 → 2.60×), so the flat 1.8× was *store reads*, not a
  limit on entity-compute parallelism.
- **Axis-1-engine host**: ~20–28× wall-time; the bottleneck then *moved off* the evaluator
  onto store I/O.
- **The time-axis negative**: the floor parallelizes **maps, not folds**; the serial floor
  is k+1 barriers, O(1) in N.
- **The whole-state O(N)-per-tick floor is a representation choice, and it collapses.** Arch
  reframed it; the probe (`programs/subtree_test.go`) confirmed per-tile subtree state +
  content dedup gives an **O(changed)** tick — sparse collapses **21.6×**, dense stays O(N).
  Doom-class sims are sparse, so they never hit this floor. The former Doom blocker is now a
  co-design (a subtree-state descriptor/host convention), not a wall.

**Waiting on arch** for the subtree-state descriptor shape, the collection primitive
(`concat`), and `PROPOSAL-CONTINUATION-STANDING-MODEL` §4. Nothing unblocked remains here.

### 1a. AE-5 Axis-1 conformance admission — ~~**GREEN (LOCKED)**, 2026-07-23~~ **LAPSED — re-measured 2026-08-25, NOT LOCKED**

> **Do not quote this section as current.** Re-run on 2026-08-25 against the live corpus (362
> vectors, `8d2f55c8…`): **334 agree, 0 diverge, 28 INCOMPLETE — NOT LOCKED.** The lock below was
> real on 2026-07-23 and lapsed when the v3.24/v3.25 primitives landed. **Nothing told us, because
> the harness skips unless `AXIS1_ADMISSION_CORPUS` is set and no target sets it** — so it has been
> reported as a passing suite ever since. §0c has the full measurement and the resulting rows; the
> withdrawal is routed in `reviews/AXIS1-ADMISSION-LAPSED-2026-08-25.md`. Kept below verbatim as
> the record of what was true then.

Core-go's AE-5 packet (`entity-core-go/docs/status/ROUTING-2026-07-23-ae5-axis1-inproc-admission.md`)
asked workbench to run the frozen 330-vector inproc compute corpus in-process through **Axis-1** (the
only alternate engine, and it lives here) and hand back the alternate-engine emission; a green run
folds EXTENSION-COMPUTE §11. **Done and green.** Full write-up:
`docs/architecture/reviews/COMPUTE-AE5-AXIS1-ADMISSION-RESULT-2026-07-23.md`.

- Harness `entitysdk/axis1_admission_test.go` mirrors core-go's reference emit loop, swapping in
  Axis-1; corpus + reference emission reproduce byte-exact (`9131a93d` / `419ca55d`); the emission is
  accepted by core-go's `verify`/`cross-bless` with no adapter.
- The run found **two real Axis-1 bugs, both fixed** (Axis-1 is workbench-owned): the uint-index
  `type_mismatch`/`index_out_of_range` divergence (F-2 ruling never transcribed — `axis1/arith.go`),
  and the missing native `compute/apply` closure application that forced a tail-recursion deopt
  (`axis1/{node,decode,eval}.go`). After both: `verify --require-alternate` 7/7 guards pass,
  `cross-bless` **330 agree, LOCKED**.
- **Owed:** core-go re-runs verify + cross-bless on its side to bless, then arch folds §11.
- **Rider for arch:** AE-5/§11 is Axis-1's graduation to a conformance-admitted engine — the
  Axis-1-home question (workbench research engine vs a second `ext/compute` engine) is surfaced in the
  review doc §5.

### 1b. Avalonia program-chrome catch-up (2026-07-27) — CLOSED

Closed the C# side of `docs/status/HANDOFF-2026-07-27-avalonia-program-chrome-catchup.md`
(the workbench half — interactive Life + the pointer-gap proposal — shipped the same day;
that handoff is left as the historical ask). All four items landed in `ProgramPanel.cs`,
verified pixel-for-pixel via `make smoke-xvfb-program PROGRAM={life,snake,asteroids,life-edit}`
(real X11 + Skia, not just headless) and the 51-test headless suite (`make test`), both green:

- **Item C (was a live bug):** `BitForKey` only parsed the legacy string keymap form, so
  keyboard input was dead for Asteroids and interactive Life since their 2026-07-24 roled
  re-declaration. Fixed to accept both forms (mirrors `programs/controls.go::ParseKeymap`).
  Added the on-screen standard controller (d-pad + labelled action buttons, built once per
  key-set input from `scene.keymap`) — buttons pair glyph+label text since the podman
  runtime's font set lacks colour-emoji coverage (bare glyphs render as tofu).
- **Item B (was a live bug):** views were keyed by shape, so the `status` port drew on top
  of the `display` port for every program. Rekeyed by port name; `status` now renders as a
  caption docked above the board. Caught a second latent bug fixing this: the caption's
  custom-drawn `Control` has no `MeasureOverride`, so a bare `DockPanel.Dock.Top` child
  collapsed to zero height — wrapped it in a fixed-height `Panel` (a `Panel` arranges
  children to its own bounds regardless of their `DesiredSize`, same reason `_stage` already
  worked for the board views).
- **Item A:** `DisplayListShapeView` now reads `scene.render` and fills closed quads
  (skipping the reserved `DisplayKindBackground` kind) when `render == "fill"` (Life/Snake);
  Asteroids' default `stroke` wireframe is unchanged.
- **§2:** `life-edit` wired into `avalonia/bridge/program.go`'s dispatch and registered as
  `program-life-edit` in `Program.cs` (+ `SmokeDriver`'s program-cycle map).

Nothing here touches the still-open pointer/click gap
(`docs/architecture/reviews/PROPOSAL-GENERIC-HOST-POINTER-INPUT-DEVICE-2026-07-27.md`) —
that stays blocked on arch, as before.

### 1c. Full-repo health sweep (2026-08-13) — CLOSED

A ~2.5-week gap (07-27 → 08-13) had `dev` untouched but the sibling `entity-core-go` moved
dozens of commits, which had silently broken `make test` here — the earlier catch-up sessions
only ran `make test-programs`, not the full sweep. Ran `make test` / `make lint` /
`avalonia/bridge` smoke-compile / `avalonia make test` end to end for the first time since;
three real defects found and fixed, all on our side of the fence (core-go itself untouched):

- **Build break:** core-go's coordinated rename `types.InboxNotificationData` →
  `types.SubscriptionNotificationData` (+ wire type `system/protocol/inbox/notification` →
  `system/subscription/notification`, `d7e44f6`, a single-round MUST with no dual-kind
  window) had never been adopted here. Updated the 5 call sites
  (`entitysdk/subscription.go`, `entitysdk/subscription_handler_direct_test.go`,
  `workbench/notification_ingest.go`, `workbench/blob_resolve.go`,
  `workbench/test_helpers_test.go`) plus stale example code in
  `docs/architecture/APPLICATION-HANDLER-INTEGRATION.md`.
- **Two capability-check regressions** (`shellcmd` `TestStage3_CapDelegation_Positive`,
  `TestStage4_CaseH_RestrictedCapsMesh3`): root-caused to core-go's `43573d3` (Aug 5) —
  connect-time capability assembly now applies real advertisement-discipline filtering
  (`ConnectHandler.AssembleInboundGrants` → `filterAdvertisedGrants`), and under the §PR-8
  canonicalization a self-issued grant can only ever advertise coverage over **its own**
  peer namespace (`"*"` → `/{peerID}/*`). Both tests' scoped grants listed `Resources:
  ["*", "/*/*"]` — the `"/*/*"` (cross-peer) entry can never be covered by a same-peer
  advertisement, and since coverage requires *every* Include member to match, the **whole
  grant entry** silently dropped, not just that member. Fixed by dropping the redundant
  `"/*/*"` (the operations under test all target the granting peer's own namespace, so bare
  `"*"` is sufficient) — confirmed empirically (reverted, reproduced the 403, re-applied).
  Documented in both tests' doc comments so the next drift doesn't re-diagnose this from
  scratch.
- **Makefile hygiene:** `test-inspect` was missing from `.PHONY` (its sibling
  `test-programs` wasn't) — file-existence-based tracking on that target name was tripping
  a stat/permission error under the containerized build, failing `make test` at the last
  step. Added it.

Full sweep green after: `make test` (all 9 native modules + `inspect`), `make lint` (vet
clean across every module), `avalonia/bridge` smoke-compile, `avalonia make test` (51/51
headless). No code changes to `entity-core-go` — read-only archaeology (`git log -S`, diff
review) to root-cause, per the sibling-repo boundary.

### 2. Stabilization pass (2026-07-22) — CLOSED

Structural cleanup so the next arch ask lands on solid ground:
- **`programs/` extracted into its own module.** The 21 `workbench/program_*.go` files (the
  generic host, descriptors, authoring, Life/Snake/Asteroids/heavyfield/chain/shapes) moved
  out of the app's renderer-neutral model layer into a sibling module, and lost the now-
  redundant `program_` filename prefix. Verified zero coupling before the move: of the 316
  top-level symbols in non-`program_` workbench files, the only ones the cluster referenced
  were `entitysdk`-qualified. Dep direction is `programs → entitysdk`; `avalonia/bridge`
  repointed. New `make test-programs`; wired into `test-native` and `LINT_MODULES`.
- **perfreview rot guard landed** (the backlog item). `make lint-perfreview` runs
  `go vet -tags=perfreview`, and `lint-native` depends on it. Compile-only on purpose —
  the benches take ~20m and must never run under `-race`. It is currently **clean**, so the
  module is *not* rotted; the guard is what keeps it that way.
- **The minimize crash got a headless repro suite** —
  `avalonia/tests/Workbench.Headless.Tests/PanelStackZeroCollapseTests.cs` (see below).

### 3. The share / multi-peer arc (the live thread) — steps 3a and 3 DONE

The one doc to read is
**`docs/architecture/reviews/REVIEW-SHARE-AND-CONNECTIVITY-ALIGNMENT-2026-08-17.md`** —
§5.1 is the ordered plan, §7 is the N1 ruling, §8 is what landed 2026-08-18 and the
correction it forced. Companion packet: `reviews/CORE-GO-ASKS-2026-08-17.md`.

Where the six steps stand:

| step | what | status |
|---|---|---|
| 1 | Commit + send the review | done (`ee96c5f`, `0e86a9f`, `ca2e0cb`) |
| 2 | Foreign-namespace subscription gate on the Go arm | done (`b3848c1`) — and it found the watch-hub `send on closed channel` |
| **3a** | **Consumer-side `published-root` reader** | **done** — `entitysdk/published_root.go` |
| **3** | **Target prefix on the sync surface (source ≠ target)** | **done** — `MirrorSinceLastSeen` + `InstallRevisionMirrorChain` + `revision mirror` |
| 4 | Follow vocabulary settled with browser-rust | **open — needs browser-rust.** Arch confirms the two pieces (one follow verb with a `strategy` field; a per-follow minted capability) need no arch ruling |
| 5 | The first `app/share/*` record | **arch half DELIVERED, our half STARTED (`146f9a4`).** `APP-CONVENTION-SHARE` v0.1 authored (`bb86cd1`, `ROUTING-i` §4); `entitysdk/share.go` ships the `app/share/*` type vocabulary, the tagged target union, `ShareGrants` (with `peers` omitted), `ValidateShareGrants` (the §1.1 MUST as a refusal), `AuthorShare` and `ShareWithdrawalNotice`. Vectors **SHARE-4** and **SHARE-6** pass, plus three shape pins. The authoring/validation layer is pure, so it is green **through** the kernel block; persisting the record + delivering tokens is the half that waits. `strategy` still open on browser-rust (step 4) |
| 6 | `APP-CONVENTION-CHAT` review as a consumer | **DONE 2026-08-20** — `reviews/APP-CONVENTION-CHAT-CONSUMER-REVIEW-2026-08-20.md`. Three findings, each grounded in something this tree already hit: §4's `.list`/subscribe over `/{P}/…` needs a **mirror capability the proposal never names** (AP11 + the §PR-8 403 we shipped wrong first); the per-author `prev` chain has **no gap rule** and it is the same construction browser-rust already flagged in our follow; `attachments` is a **Layer-2 chunking contract** wearing a Layer-1 spelling. Plus one unlisted substrate dependency (content-store GC — a conversation is the purest unbounded-append workload there is) and a measured datum for `[ASK-ARCH-CHAT-2]` (the 20.3 MB/peer delivery ring). **Not building it** — the proposal is DRAFT, not `RULED` |

**What 3a/3 mean in practice.** A peer can now read another peer's signed
`system/peer/published-root` (full verification: content-hash recompute, signature against
the key derived from the Base58 peer-id, `prefix` §3.3a discipline, monotonic seq floor),
and mirror that peer's subtree into `/{them}/{their path}` — the V7 §1.4 cached-remote
shape browser-rust's F1 is about — instead of into our own namespace. Both the one-shot
pull and the standing follow chain. The destination is **derived from the publisher's
signed prefix**, never caller-chosen, per arch's amendment.

**The correction worth carrying.** W3 told core-go the mirror needed "no wire change — the
blocker is entirely ours, in an SDK signature." Half right. `tree:merge` pre-checks put
authorization on every target path, and a self-issued `Resources: ["*"]` is peer-local under
§PR-8, so every `/{them}/…` merge 403s. The fix is a capability that names the publisher's
namespace (`MintMirrorCapability`) plus a per-call caller-cap seam on the executor — not a
signature change. **No ask on core-go**; their behavior is correct. Full account in §8.2;
ratified as **D19** with **AP10/AP11** in the charter.

**Axis B (connectivity validation)** is unstarted, **confirmed unblocked by arch**, and the three
constraints are confirmed correct as stated (§5.2 sizes it). One caveat added 08-18: **no TURN
credential mechanism is specified anywhere in the corpus** (arch queue Q18). Build against static
config and do not invent a credential shape — hitting that wall and routing it is the forcing
function.

**Scoped with browser-rust 2026-08-18** —
`docs/architecture/reviews/CONNECTIVITY-CONVERGENCE-2026-08-18.md`. They named the four app-tier
pieces our arm is missing and we verified all four against our own tree: no `ext/signaling`
consumer, no `system/peer/status` read-model (`ConnectedPeers()` is a pool snapshot), no
`maintain-peer`, no connector/`meet`. **The transport is not the gap** — `AppPeer`
listens and dials WebSocket already (`ListenWebSocketReady`, `Connect("ws://")`), and `shellboot`
routes a `ws://` `ListenAddr` at peer creation. What is missing is **self-publication**: nothing
writes `system/peer/transport/{our-peer-id}/*`, so a browser cannot learn we accept a socket.
That inverts the order — profile publish is the precondition, not a peer, of the other three.
**No WebRTC on the Go arm** (§6.5.2d is latent without a native terminator; browser↔Go is `wss`,
browser↔browser is WebRTC).

**Piece 1 of 4 landed 2026-08-18 — self-publication.** `AppPeer.AdvertiseTransport(dialURL)`
writes a `system/peer/transport/*` profile under **our own** peer-id, and `PeerManager.Create`
calls it when the listener binds (new `Config.AdvertiseURL` for when the routable address differs
from the bound one). Wildcard and port-0 advertisements are **refused, non-fatally** — the peer
still listens, `HostedPeer.AdvertiseErr` says why, and nothing false goes in the tree. Pinned by a
real-session test through `Create`, mutation-checked. Found on the way: the Avalonia bridge's
`PeerListenAddr` reported `listening: false` for **every** WebSocket peer, because it gated on
core-go's `Peer.Addr()` and only `ListenReady` sets `p.listener`; fixed to gate on `ListenScheme`.
**Pieces 2 and 3 landed the same day.** The lesson from AP13 applied immediately: **core-go
already had both halves.** `core/peer` writes every liveness transition (`connected` at handshake,
`suspect` at the dispatch seam, `disconnected` on keepalive miss) whether or not anything is
listening, and `ext/network` implements maintain-peer / release-peer / status / close plus the
§4.1 reconnect continuation graph. Neither needed authoring — they needed *registering* and a
consumer.

- **Liveness read-model** — `Store.PeerLivenessOf` / `PeerLivenessAll` / `OnPeerLivenessChange`
  (+ `AppPeer` wrappers) over `system/peer/status`, and `workbench.PeerLivenessModel` as the
  renderer-neutral view with connected/suspect/disconnected counts. Prefix-subscribed, never
  scan-and-filter. This replaces reading `ConnectedPeers()` in a renderer: the pool snapshot
  cannot express `suspect`, cannot say *why* a peer went, and disagrees with the tree whenever a
  connection is evicted without a demotion. Three properties the model carries deliberately:
  absence is reported as absence (a stranger is not a goodbye), `LastSeen` is a transition
  snapshot and **not** a heartbeat (§5.4.1), and the enum is three-state — `reconnecting` is not
  a status the tree can hold.
- **`system/network` handler wired** (`ExtensionsConfig.Network`, default-on) with the
  post-construction `Bind`, plus `entitysdk.NetworkClient` — `MaintainPeer`, `ReleasePeer`,
  `Status`, `MaintainedPeers`, `Close`. Registering it starts nothing: the continuation graph is
  installed per-peer by a maintain-peer call.

**Spec finding, found by running it:** §2.7's `maintained_peers` is **not** the maintained set.
§4.3's own pseudocode enumerates every entity under `system/peer/status/`, so a released peer
keeps its row and loses only its `session_id` — verified against a real handler when the obvious
assertion failed. Routed to arch; `NetworkClient.MaintainedPeers()` is the `session_id != ""`
filter in the meantime.

**Remaining on the four: connector registry + `meet`** — and both of its gates have now returned,
so the next session starts here rather than re-scoping it. **D20 pre-check done (2026-08-18), so
nobody prices this against our own tree again:**

- **The shape is ruled.** Our `CONNECTIVITY-CONVERGENCE` §2 said we would take piece 4 earlier *"if
  your `meet`-as-DISCOVERY-backend question (arch Q8) returns in a shape that makes 4 cheap."* It
  returned on **2026-08-17** — `ROUTING-2026-08-17-c` §1: **confirmed, token minted `rendezvous`**,
  `pair` mode is **not** discovery, and the candidate is the §2.2 successor pair with
  `identity_hint` **absent** (TOFU). `meet` is a DISCOVERY backend implemented on the SIGNALING
  carrier — *the key introduces; it never authorizes* (SIGNALING §1.2) meeting DISCOVERY §2's
  *discovery is the initiator of the grant, never the authority*. Read
  `PROPOSAL-DISCOVERY-RENDEZVOUS-BACKEND`, not the summary, before building.
- **The carrier already exists in the substrate.** `../entity-core-go/ext/signaling/` ships
  `HandlerPattern = "system/signaling"`, ops `offer` / `collect` / `advertise` (`const.go`), a
  `Client` (`client.go`), plus `punch.go`, `webrtc.go`, `reflection.go`, `pool.go`, `coordination.go`
  and the rendezvous `key.go`. **There is no `meet` op and there should not be** — `meet` is the
  DISCOVERY-side logic over this carrier, which is the piece that is genuinely ours.
- So piece 4 is the same shape pieces 2 and 3 turned out to be (AP13/D20): **registration and a
  consumer, not authoring.** What is absent in our tree is an `ext/signaling` consumer — grep:
  `grep -rn 'ext/signaling' entitysdk/ workbench/ shellcmd/ shellboot/` returns nothing.
- Constraints unchanged and confirmed: SIGNALING §3.4 same-provider is a **MUST** (both arms on the
  same pool or silent never-meet); `data_relay` is `policy: open` only; **no TURN credential
  mechanism exists anywhere in the corpus** (arch Q18) — build against static config and route the
  wall rather than invent a credential shape.

**Piece 4, started 2026-08-19 — the carrier landed; the backend is READY TO BUILD.** The D20
pre-check said "registration + a consumer" and that is right; what it did not check is whether the
DISCOVERY surface it registers into exists in landed spec. It does not — which turned out to be
worth knowing and not worth stopping for:

```
$ grep -rn 'rendezvous' ../entity-system-architecture/specs/extensions/EXTENSION-DISCOVERY.md
   (no matches)          # arch c984f93 — Version 1.0, enum still <"mdns" | "qr" | ...>
```

`PROPOSAL-DISCOVERY-RENDEZVOUS-BACKEND` is stamped **RULED 2026-08-17** and **none of its §6 fold
has landed** — no `rendezvous` enum token, no §5.5, no version bump. §5.5 is where the mode split
and the TOFU + successor requirements live, so the backend's whole normative content exists only in
the proposal, and `AGENTS-STANDARD` says implement against the **landed spec**. Defining the token
locally is AP20's exact shape one step earlier. `entity-core-go` matches the spec, not the proposal
(`DiscoveryBackendMDNS` and nothing else) — the gap is upstream of them. Ask routed:
`docs/architecture/reviews/DISCOVERY-RENDEZVOUS-FOLD-ASK-2026-08-19.md`. **A RULED stamp is a
decision, not a normative surface** — we read it as landed and had to grep to find out otherwise.

**HOLD LIFTED by operator ruling, 2026-08-19. Piece 4 is READY TO BUILD — build it against the
proposal.** The hold was mine and it was wrong; recording the correction so no session re-derives
it.

Re-checked against arch HEAD (`05faaa5`): arch's `docs/COHORT-OPEN-ITEMS.md` carries this as
**R-10, owner `arch`, OPEN** — *"ruled, not folded"*. The facts:

- **Nothing here is undecided.** The proposal header reads `Status: RULED 2026-08-17`, and its §0
  states the normative delta is **one enum value** in `EXTENSION-DISCOVERY` §2.1 plus a
  composition subsection — **no wire change, no new entity type**. The mode split, TOFU, and the
  successor chain are fully written in §2/§5. It is buildable precisely, not guessed.
- **What is missing is arch writing that ruling into spec text.** Editorial, R-10, theirs.

**The operator's ruling: implementing a RULED proposal is normal practice in this cohort, and is
one of the ways a spec gets validated before it is folded.** `AGENTS-STANDARD`'s *"implement
against the landed spec, not in-flight proposals"* is aimed at **unruled** proposals — building on
a shape nobody has decided. A proposal stamped RULED is a decision; treating it as in-flight is
over-reading the rule, and the cost of that over-read here was a self-inflicted stop on work whose
content was fully determined.

**AP20 does not apply and I mis-cited it.** AP20 is about inventing a constant to satisfy a spec
table *whose referent does not exist* — the tell being that you must write a doc comment
explaining the absence. Here the referent exists and is named in a ruling; we are implementing
ahead of the fold, which is a different act with a different failure mode.

**Build it, and route what building teaches.** The known cost is small and stated up front: if the
token's spelling moves in the fold, we re-cut a few hundred lines of backend. The enum is open
(`<"mdns" | "qr" | ...>`), so an undeclared token is not a conformance violation for a consumer.
The commit says it is built against `PROPOSAL-DISCOVERY-RENDEZVOUS-BACKEND` at arch `05faaa5` and
not against landed `EXTENSION-DISCOVERY`, so the coupling is greppable when the fold lands — and
anything the implementation surfaces goes back to arch as feedback on the proposal, which is the
point of doing it in this order.

The routed ask (`docs/architecture/reviews/DISCOVERY-RENDEZVOUS-FOLD-ASK-2026-08-19.md`) stands as
a fold request, **not** as a blocker on us; it should be re-framed as "here is what we learned
building it" when piece 4 lands.

**PIECE 4 IS BUILT — 2026-08-19, `021e5c2`.** `entitysdk/rendezvous.go`, against the RULED
proposal at arch `05faaa5`, not against landed `EXTENSION-DISCOVERY` v1.0. **The four-piece
connectivity list is complete.**

The D20 pre-check held: **registration and a consumer, not authoring.** `ext/signaling` ships the
ops, the client, the key derivation and the §6.3 signed container; `ext/discovery` ships the
substrate, the candidate store, the watchable prefix and `PromoteSuccessor`. **There is no `meet`
op and there should not be one** — now confirmed from the implementing side rather than the
reading.

The rules that are not obvious, each pinned and each mutation-checked:

- **`peer_id` and `identity_hint` are both ABSENT on `candidate_0`** — and we hold a
  *cryptographically verified* counterpart identity at that moment (core-go's §6.3 container
  returns a `VerifiedSigner`). The backend does not use it. §2.2 step 1 puts the peer-id after
  IDENTIFY over the **admitted channel**, and SIGNALING §1.2 is why: reaching a key proves
  someone derived that key, never that the channel admission opens belongs to them. This is the
  half an implementation gets wrong by doing the helpful thing.
- **`pair` mode is refused** (proposal §2.1) — both peer-ids are inputs, so there is nothing to
  surface and a candidate would be the caller's own input presented to a user as a stranger.
- **`endpoint_hint` locates a DEPOSIT, not a bucket.** The proposal's "bucket/key locator" cannot
  tell two peers at one tag from one peer seen twice. For `secret` mode it carries neither the
  secret nor the derived key — a candidate exists to be *shown to a user*, which is the last
  place a credential belongs.
- **Skip-own, departure-reaping, one candidate per standing peer.** A deposit falling out of the
  bucket at the node's TTL is a real departure signal (§3.0.1 rule 3's shape, not rule 4's
  one-shot waiver), and the candidate entity embeds `observed_at`, so re-emitting per poll would
  show one peer N times.

**Scope boundary:** `candidate_0` only. The §2.2 successor promotion, the §2 grant decision and
IDENTIFY are the admission path, not the backend.

**`Extensions.Discovery` is new and exists because of this** — see AP26. The substrate was wired
only for **listening** peers, which was right while mDNS was the only backend and exactly wrong
here: a rendezvous peer stands at a mailbox *because* it has no reachable listener, so the gate
excluded precisely the peers the backend serves. Default unchanged; a control arm asserts that.

**Routed, which is the other half of building ahead of a fold:**
`docs/architecture/reviews/RENDEZVOUS-BACKEND-BUILD-RESULT-2026-08-19.md` — four things §5.5
should carry (the TOFU rule's reason **and its cost**: §2.2.1's fail-closed IDENTIFY comparison is
structurally unavailable to every rendezvous candidate, so TOFU is the ceiling and not a fallback;
deposit granularity; the mode-dependent locator; a reap rule for a non-mDNS departure signal).
**None is a defect in the ruling** — the layering, the token and the composition all held up under
construction. The earlier fold-ask keeps its §3 ask and has its §2 (the stop) withdrawn in place.

**Not cross-impl.** Everything above is Go meeting Go over its own node — cohort-consistent at
best. The interop claim needs browser-rust's `src/rendezvous.rs` on the other end of **one shared
node** (§3.4), which we have not run and are not asserting.

**Ratchet: AP26** — *the first consumer's precondition became the substrate's.* Not promoted; it
has bitten once. Charter is D1–D22 / **AP1–AP26**.

**The carrier half is landed spec (EXTENSION-SIGNALING v1.1) and is done.** `entitysdk/signaling.go`
— `AppPeer.Signaling(nodePeerID)` with `Offer` / `Collect` / `Advertise` through `extDispatch` (so
non-2xx maps to `*entitysdk.Error`, not core-go's proto-SDK), key derivation **delegated** to the
kernel's `ext/signaling` because it is a Layer-2 algorithm — two peers whose key bytes differ
silently never meet, and no same-impl test can see it. Plus `ExtensionsConfig.SignalingNode`, an
opt-in hosted node: every other extension in that struct is a capability the peer *has*, this one is
a service it runs *for other people*.

Gates: three peers on the wire (node + two clients, TCP, pooled connections) meeting at a tag —
non-destructive collect, arrival order, content-hash dedup, empty-not-404; the derivation properties
that have no error path (mode separation, pair symmetry, determinism); and §3.4's same-provider MUST
shown as two nodes with the same key never meeting. **The meeting gate uses two real peers on
purpose** — one peer offering into its own node proves the bucket and not the meeting, which is the
shape we just promoted to D22.

### 4. Publisher conformance — CLOSED 2026-08-18, the corridor emits a real signed root

`publish/publish.go` advertised `signed_pointer: "system/peer/published-root"` +
`freshness: "static-immutable+signed-pointer"` and did no signing at all; the artifact at
`{manifest_url_prefix}` was the http-poll *transport profile*, which `EXTENSION-NETWORK` §6.5.3.1
rules out in as many words. Step 3a made it self-refuting — our own `ReadPublishedRoot` rejected
our own publisher's output on gate one. Arch routed it 08-17 and again 08-18.

**Exit B taken (emit a real signed root), not Exit A.** The handoff priced B as its own arc on the
strength of *"our closure walk is shallow and D3 requires the trie closure"* — true about
`publish/`'s walker over bound entities, and irrelevant to the obligation, because
`tree.CollectNodeClosure` in core-go already implements D3 exactly and cites §6.5.6 Amendment 10
as the reason it exists. **An estimate that prices a spec obligation off our own code's shape,
without checking whether the substrate already implements it, is an estimate of the wrong thing.**

What ships (`publish/signed_root.go`, new):

| object | now |
|---|---|
| `{out}/manifest` | the signed `system/peer/published-root` (3-key wire entity, `content_hash` + `prefix`) |
| `{out}/transport-profile` | the http-poll profile, out-of-band per §6.5.4 / proposal D5 |
| `{out}/content/…` | + the transitive trie closure of `root_hash`, the published-root, and its signature — the §6.5.3 publish-side MUST |
| `{out}/{peer}/system/signature/{hex}.bin` | the §5.2 invariant pointer, as an ordinary two-hop TREE_GET leaf |
| `{out}/{peer}/system/peer/published-root/{peer}.bin` | `signed_pointer` names a path, so the path resolves too |

Acceptance test is written **as a consumer** — no peer, only the emitted files: recompute the
manifest hash from its own bytes, resolve the signature two-hop, verify against the key derived
from the Base58 peer-id, walk the CHAMP trie from `root_hash` over the emitted shard asserting no
404 (`TestPublish_SignedRootVerifiesFromTheEmittedFiles`).

**Three things fell out of building it**, all in
`docs/architecture/reviews/archive/PUBLISHER-CONFORMANCE-RESULT-2026-08-18.md`:

1. **A signed root and a filtered publish are incompatible — we refuse at the emitter.** The
   closure obligation would upload the `IncludeType`/`IncludePath`-excluded entities' bytes under
   `content_url_prefix` anyway (a leak, and *because* we advertised a signed pointer); withholding
   them instead is the silent short walk browser-rust measured (`9a9c0f5`). §6.5.3 states the
   broken-walk direction; the leak direction is unstated. Routed as a candidate.
2. **core-go ask: `published-root` `seq`/`predecessor` are process-memory only.** Measured — three
   publishes of three *different* roots through `ext/publishedroot.Publisher` emitted
   `seq=1 / predecessor=nil` every time. §6.5.6 makes both a MUST. We source them from the store
   ourselves and would rather not own a second minting site.
3. **arch ask: the content-only mirror §6.5.6 sanctions has no expressible `freshness`.** The enum
   is `live | async | static-immutable+signed-pointer`, and the MUST list requires `signed_pointer`
   for both non-live values. Found while pricing Exit A; we emit nothing into the gap.

**Unblocked by this:** the Go-published / Rust-consumed cross-check with `entity-browser-rust`
(they emit and walk signed roots already, rust↔rust) — arch calls it the most valuable interop
result on this track, and the only one that is not cohort-consistent.

**Process (AP12, ours).** Arch routed the finding on 08-17 addressed to us by name; we did not open
a row and shipped three commits past it. `AGENTS.md` now makes the sibling-arch read a session-start
step. Catalogued, not ratified — first time in this shape.

### 5. R3 — the resolver-config ships (2026-08-18)

Arch's `096fa96` folded the default `name_format_dispatch` globs into
`EXTENSION-REGISTRY` §4.1a. That was **R1**, the item R3 was waiting on, so R3 — ours jointly
with browser-rust — is unblocked and now done on our side.

D20 check first, and this time the substrate did **not** have it: core-go reads the config and
applies the dispatch list, but nothing in the cohort writes a default one (construction exists
only in the `validate` harness). Shipping it is app-tier work, as arch said.

`entitysdk/resolver_config.go` — `DefaultNameFormatDispatch` (the six §4.1a rules, in order),
`DefaultResolverConfig` (that list + a local-name-only chain), `ValidateResolverConfig`, and
`AppPeer.InstallResolverConfig` / `ResolverConfig` / `EnsureResolverConfig`.
`EnableLocalNameResolver` now writes the dispatch list too — it used to write the chain alone,
which was harmless only by accident (no dispatch list ⇒ every name consults every backend, and the
chain happened to be local-only).

- **Order is the contract.** First-match-wins, and rules 4/5 overlap on every dotted authority. A
  glob cannot say "undotted", so `*@*.*` must precede `*@*`. The pin asserts the sequence — a
  set-membership test passes on the reversed list, which routes every domain-scoped name to
  peer-issued and then toward a catch-all that must not see it.
- **The catch-all MUST be local-only** (§4.1 step 2 — "the primary privacy mechanism"). We
  **refuse** rather than normalize: §11.1 permits either, but silently rewriting an operator's
  privacy config into a different one means they never learn they did not get what they asked for.
- Two cohort observations we filed as inert. **They were not** — see §6b.

**Operator surface:** `peer status` (new) renders the tree's lifecycle record —
connected/suspect/disconnected, the transition reason, and a coarse age — deliberately a
*different* answer from `peer ls`, which lists this session's alias table. A peer that connected
to **us**, or one released an hour ago, appears in the first and not the second, and only the
first can say `suspect`. Empty output says *"nothing has ever transitioned"* rather than showing a
blank table that reads as "nothing is connected". The SINCE column shows `failing_since` or
`connected_at` and **never `last_seen`** — rendering a transition snapshot as "last heard from"
would tell an operator a healthy peer had gone quiet for hours.

### 6. CLEARED — the tree was red across 6 of 9 suites, and it was core-go's (2026-08-18)

> **RESOLVED same day. core-go `7593618` — `DispatchLocalExecute` now passes
> `handler.WithResource(req.Resource)`.** Re-run here against it: **`make test` exit 0, zero
> failures** across its eight suites (`entitysdk`, `shell`, `shellboot`, `shellcmd`, `shellpanel`,
> `workbench`, `programs`, `inspect`), plus **`make test-publish` green** separately — `publish` is
> not in the `make test` target. `make lint` clean. **323 → 0.** Our call sites were correct the
> whole way down and nothing here changed to accommodate the defect — the decision not to work
> around it in the app repo is what kept the fix a one-liner in the right tree.
>
> *(Per AP15: the count above is from a run that completed. A first attempt exited 2 on a transient
> `cd: can't cd to shell` while other container jobs were touching the tree concurrently; re-run
> clean with nothing else running, `shell` passes in 2.27s. Reported rather than quietly dropped.)*
>
> **The reproducer became their regression test.** `TestDispatchLocalExecute_CarriesResourceToHandler`
> is our kernel-level shape landed in `core/protocol/local_entry_resource_test.go` — red pre-fix at
> status 200 with `Resource == nil`, green after.
>
> **Their ratchet, worth carrying here too:** `dispatch_equivalence_test.go` asserted *result*
> equality between the wire and in-process entry paths but never the handler-visible *context*, and
> `subdispatch_resource_dimension_test.go` drove both sub-dispatch directions without ever entering
> through `DispatchLocalExecute`. **Two entry paths claiming equivalence need a test that asserts
> the handler-visible context, not just the result.** The SDK path exercised it; theirs did not.
> Folded as **AP17**.

The history below is kept because the *shape* is the lesson, not the outage.

### 6 (historical). BLOCKED — `make test` is red across the tree, and it is core-go's (2026-08-18)

**Not ours, not worked around, routed — and it has LANDED in core-go (`0b9e261`), not merely
in-flight.** The change removes resource inheritance in sub-dispatch per arch
`ROUTING-2026-08-18-g` §5 (**ruled, normative** — `ENTITY-CORE-PROTOCOL` §5.2 at arch `980ddf1`). The ruling is right. Removing the inheritance also removed the only channel by which
the in-process **entry point** delivered a resource it was explicitly given:
`DispatchLocalExecute` sets `rootCtx.Resource` and then dispatches with `WithCapability` alone.

**Measured tree-wide in the 2026-08-18 audit — 323 failures across 6 of 9 suites**, which is
substantially worse than first reported. `make test` stops at the first failing package, so the
earlier per-suite numbers were taken through a keyhole; each suite must be run on its own
(`make test-sdk`, `test-shellcmd`, …) to see the radius:

| suite | failures | |
|---|---:|---|
| `entitysdk` | **171** | first reported as 5 |
| `shellcmd` | **71** | |
| `programs` | **46** | 52 with subtests |
| `shell` | **14** | first reported green — it was not |
| `shellboot` | **12** | first reported green — it was not |
| `inspect` | **9** | not previously reported |
| `workbench`, `shellpanel`, `publish` | 0 | genuinely green |

**It wears five faces, and that is the part worth remembering** — the same defect will not present
the same way twice, because each handler validates its resource independently and says so in its
own words: `resource target path is required` (tree, 306), `bind_cap` (37, downstream of a failed
put), `resource target is required for subscribe` (17), `ambiguous_resource: install requires
exactly one resource` (17), `missing_resource_path` (role, 5). Do not diagnose these separately.
The rest are **cascade** — a test whose setup `Put` was refused then reads an empty tree and
reports a wrong count, a missing path, a surviving roster entry. All one defect.

**The mechanism is now isolated, not just argued** (the measurement the previous handoff flagged as
owed). A kernel-level reproducer — `protocol.NewDispatcher` + `DispatchLocalExecute` with a
wildcard grant and a handler that records `req.Context.Resource`, **no workbench code in the
path** — shows the handler running at status 200 and seeing `Resource == nil`. It is in the packet
verbatim, as the test core-go is missing.

Full packet, including the one-line fix, the reproducer, and the coverage gap that let it through:
`docs/architecture/reviews/archive/CORE-GO-LOCAL-DISPATCH-RESOURCE-2026-08-18.md`.
**Re-run `make test` once it lands.** Do not work around it here — the call sites are correct.

**Historical note, corrected 2026-08-20:** this section once ended *"the packet is still unsent —
it does not reach them until `dev` is pushed."* **It went out with the 2026-08-19 push and core-go
landed the fix** (`7593618`). Kept because the shape is the lesson, not the outage.

### 5a. The cross-impl publish/consume check — three surfaces align, the front door does not (2026-08-18)

The ADR-0012 result: every signed-root result either arm holds is **same-language**, so ours and
browser-rust's agreeing with themselves is cohort-consistent, not independent convergence.
`publish/cmd/crossimpl-fixture` emits a deterministic Go site (pinned seed, so peer-id and every
hash below it are stable) to hand their reader.

Measured against browser-rust `a0145a7`'s `DirFetcher`:

- **Aligned, with no shared code:** content sharded `{aa}/{bb}/{hex}` on the 66-char wire hex,
  bare-hashable bodies, and the two-hop signature at `system/signature/{root_hex}.bin` keyed on the
  published-root entity hash. Their doc calls the latter two *"divergences from upstream"* — they
  are **not** divergences from us. That is the part worth keeping.
- **Not aligned — and it is hop 0.** `DirFetcher::manifest()` reads
  `{base}/{peer_id}/system/peer/published-root`. **Half of this is now FIXED in the kernel:**
  core-go `2bd2380` (ruled by arch *from this run*) dropped the `/{base58_peer_id}` segment that
  `PublishedRootStoragePath` appended — the qualified binding had named the peer twice, and
  core-go's writer and reader shared the helper, so Go-on-Go passed deceptively. Adopted here in
  `00b92c7`; the directory collision is gone.
  **The remainder, re-measured:** we emit `…/published-root**.bin**` (our advertised
  `tree_leaf_suffix`, since the head pointer *is* a tree leaf) holding a 2-key `system/hash`
  pointer; their `manifest()` reads the suffix-less path and expects the 3-key wire entity, which
  we emit at `{out}/manifest`. `ENOENT` now rather than `EISDIR` — still hop 0, and now a narrow
  question about which artifact belongs at which path.

**RULED 2026-08-18 in our favour — `ROUTING-2026-08-18-p` §3, `EXTENSION-NETWORK` 1.8.** The
manifest's location is **discovered** from `manifest_url_prefix`, never derived by convention from
the tree path, and *a consumer MUST NOT join `signed_pointer` onto an origin*. The two fields answer
different questions: `manifest_url_prefix` is where to GET it, `signed_pointer` is what the origin
is asserting. `{origin}/manifest` and `{origin}/{peer}/system/peer/published-root` are equally
conformant; only the advertised one is findable. **`DirFetcher::manifest()` is the defect and the
fix is browser-rust's.** Arch rejected "serve it at both paths" — *"two front doors is not
compatibility; it is the divergence, ratified"* — and upheld the decision not to move our layout.
Nothing owed here; our `manifest_url_prefix` advertisement was conformant throughout.

**Fixture re-cut and handed over (`d940ce0`), and the re-cut found a defect of ours.** The packet
told browser-rust the emission was byte-identical on their machine. It was not: `published_at` is a
field **of** the published-root entity, so a fresh clock moved the root's content hash, the
`system/signature/{root_hex}.bin` binding named after it, two content shards and `{out}/manifest` —
every artifact their reader enters through. Only the trie root and the entities beneath it were ever
stable. `publish.Opts.At` now pins the instant (zero still means `time.Now()`), the fixture pins it,
and two fresh runs diff clean. **AP18** — a claim about emitted bytes settled by reading the emitter
instead of emitting twice and diffing.

Corrected fixture facts (core-go `7593618`): `peer_id 2KLv2nhwtPrL…`, trie
`ecf-sha256:f567bfbd…`, published-root `00e0138dbeb374…`, signature `000ef5f255803…`.

**Still not run end to end:** executing their reader against our fixture needs a test in *their*
tree. Per D19/AP10 everything above except the emitted bytes ships as a prediction with a
reproducer attached.
Packet: `docs/architecture/reviews/CROSSIMPL-PUBLISH-CONSUME-2026-08-18.md` (UPDATE 2 carries the
ruling and the corrected hashes).

### 6a. CORRECTED — R3 shipped against a spec sentence arch withdrew 78 minutes later (2026-08-18)

`24169b9` shipped the resolver-config against `EXTENSION-REGISTRY` **1.6** §4 (*"an ORDERED list,
first-match-wins (MUST)"*). Arch withdrew that in `3670283` → **1.7**: the list is a **filter**, a
name matching several entries is eligible at the **union**, and precedence is
`resolver_chain[].priority`. The `#` column is reference numbering, not evaluation order.

**One live defect came out of it and is fixed.** `ValidateResolverConfig` refused a config whose
catch-all was not the final entry (`catchall_not_last`) — correct under 1.6, where everything below
a catch-all was dead config; under 1.7 those entries stay eligible, so the refusal **rejected a
deployment the spec permits**. Removed, with a regression pin naming the withdrawal
(`TestValidateResolverConfig_CatchAllPositionIsNotADefect`). The order pin became
`TestDefaultNameFormatDispatch_MatchesTheSpecTable` — it pins the six rows and their
`backend_kinds`, not a sequence.

**What did not change:** the six default rows, emitted verbatim in the table's sequence (now
documented as presentational), and **§4.1 step 2's catch-all local-only MUST, still enforced at the
write as a refusal**. 1.7 makes that the load-bearing rule explicitly — the same conclusion resting
on the right sentence.

Routed: `docs/architecture/reviews/archive/RESOLVER-CONFIG-FILTER-CORRECTION-2026-08-18.md`, which also
carries the two cohort observations (`did-key` vs `self-certifying`; `pinned` has no constant) and
the `ROUTING-2026-08-18-i` acknowledgement.

### 6b. REGISTRY v1.13 adopted — both "inert" observations were live defects (2026-08-18)

`f79cc4a`. Arch's `-p` §5 came back on the two cohort observations §5 filed as inert: **they were
dead config in every conformant peer**, because §4.2 makes an unknown `backend_kind` MUST-skip with
a warning, so our shipped default list contained rows a conformant implementation is *required to
discard*.

| row | was | now (v1.13) |
|---|---|---|
| 2 `did:key:*` | `["did-key"]` | `["self-certifying"]` |
| 6 `*` | `["local-name", "pinned"]` | `["local-name", "self-certifying", "out-of-band", "peer-issued"]` |

Row 6 is **not** what `-p` said — v1.12 removed the undeclared `pinned` without naming the declared
token that does the job, and `-q` §2 corrects it to `out-of-band` (§4.1.2: the kind a pin's
synthesized binding carries; §6a.4 makes it dispatchable where `pinned` is not). The row moved three
times in one day and browser-rust pinned the middle version. **Our pin now names the spec revision
it was taken at**, so the next move presents as a red test with a version to compare.

**The tell was in our own source: we had to invent both constants.** `BackendKindPinned` and
`backendKindDIDKey` existed only because §4.1a named strings core-go's enum does not declare, each
with a doc comment explaining the absence. We wrote that explanation twice and still filed it as an
observation. **AP20** — a constant you have to invent locally to satisfy a spec table is a defect in
one of the two documents, never a naming gap.

**The catch-all MUST is re-keyed, and our guard had been refusing a legal config.** `-l` §1
(REGISTRY 1.8) was cc'd to us and unopened: the banned property is **name transmission, not
remoteness**. The two come apart exactly at `peer-issued`, which §6a.4 resolves by content address
through a signed root so the queried name never appears in a request. Our allow-list was
`{local-name, pinned}` — it refused `self-certifying` and `out-of-band`, which dial nobody, and once
rows 2/6 were corrected `DefaultResolverConfig()` failed `ValidateResolverConfig()`: the helper that
ships the default could no longer install it. Now a deny-list over the four disclosing kinds
(`dns-txt`, `well-known-url`, `did-web`, `consensus-anchored`), because §4.2 makes an unrecognized
kind inert and refusing on account of one rejects a config a newer vocabulary permits. Code renamed
`catchall_not_local` → `catchall_transmits_name`.

**The old pin passed under both rules** — it tried exactly one forbidden kind, `peer-issued`, which
the re-key moved from forbidden to permitted. Green was our only evidence the guard was right and it
was compatible with the guard being backwards. **AP19.** The replacement enumerates all four
disclosing kinds and all four admitted ones.

**Finding routed to arch:** §11.1's `REG-DISPATCH-CATCHALL-LOCAL-1` was **not** moved with §4.1
step 2. It still says a catch-all naming *"a remote backend"* MUST be refused and that resolving a
bare name MUST produce *"no read against any remote registry"* — both false against row 6, which now
ships `peer-issued`. **An implementation passing that vector literally refuses the default list the
same document tells it to ship.** Same failure as rows 2/6 one layer out: the rule was re-keyed and
the artifact that tests it stayed on the old property. Nobody copies §11.1, so it drifted silently.

Also pinned: REG-DISPATCH-GRAMMAR-1's refusal half (`-q` §1). The grammar is closed and every non-`*`
byte is a literal, so **no pattern is invalid** and a registry MUST NOT reject one for `?`, `[`, `\`.
We author patterns and never match them, so that is our whole exposure — pinned rather than assumed,
because "closed grammar" has meant "reject at write" everywhere else in this corpus.

Packet: `docs/architecture/reviews/REGISTRY-V113-ADOPTION-2026-08-18.md`.
**D21 earned** (AP12 promoted): a *cc'd* packet is a packet. Session start now greps the arch repo
for every document naming this repo, and STATUS carries the last letter read.

### 6c. REGISTRY v1.14 — the MUST binds the configuration, and our validator saw one row (2026-08-19)

`b9e99e7`, answering `ROUTING-2026-08-19-b` §2 (arch read our tree at `0ba80c6`, source, this
session). Arch widened §4.1 step 2 from the **catch-all row** to the **configuration** — D4, because
a rule that binds one row is evaded by not writing it. Ours enforced the row: `if d.Pattern !=
CatchAllPattern { continue }`.

| door | case | code |
|---|---|---|
| 1 | a **broad** pattern that is not the catch-all — `al*` naming `dns-txt` | `broad_pattern_transmits_name` |
| 1 | the catch-all itself (unchanged, so existing diagnostics still resolve) | `catchall_transmits_name` |
| 2 | **absent/empty** `name_format_dispatch` while a name-transmitting kind sits in the chain | `filter_disabled_transmits_name` |

Door 2 is the one the loop body could not reach — with no rules there is **no row to inspect**, and
`eligible_kinds` returns ALL, so every kind is eligible for every name. `EnableLocalNameResolver`
has carried a doc comment naming this exact hazard since 2026-08-18 with nothing enforcing it: **a
named hazard with no gate is a comment.** Both doors mutation-checked against the pre-widening code.

**§11.1's placement, which we had never implemented:** *"refused or normalized at load"*. Ours ran
on author only. `AppPeer.ResolverConfig` now validates on read, returns the config **anyway**
(non-zero, beside the error — a load-time refusal denies use, not sight), and `EnsureResolverConfig`
refuses rather than reinstalling the default over an operator's config. The case is not
hypothetical: what a config *means* depends on a vocabulary outside it, so an entry that is inert
under §4.2 today becomes disclosing the moment core-go declares that kind.

**Open, routed to arch:** the widened MUST says *"any rule whose pattern matches unscoped names"*
and **supplies no decision procedure**. It cannot be read literally — a name is a flat string, so
`alice.eth` is bare and §4.1a row 3 (`*.eth` → `consensus-anchored`) would violate the MUST the same
table recommends. We read "unscoped" as *carrying no explicit authority marker*, and our
`matchesUnscopedNames` parts from browser-rust's `is_broad` on patterns like `*e` (theirs: narrow;
ours: broad). We took the strict side — it is what their own doc sentence argues for, and refusing
an exotic config costs an error message while admitting one costs every name a user types. A `§11.1`
row is needed; everything in §4.1a is grammar-identical under both readings, which is the same shape
that let `*.lab` survive review in the `name_constraints` case.

Packet: `docs/architecture/reviews/REGISTRY-V114-VALIDATOR-2026-08-19.md`.

### 6d. The CDN corridor runs in both directions — and ours was broken at our own end (2026-08-19)

`c13dfe2`, answering browser-rust's `ROUTING-2026-08-19-d` §5, the one thing they asked for:
**consume us.** Doing it found our half broken first, and worse than theirs — **`fetch` could not
read `publish`.** Four divergences at once:

| `fetch` derived | `publish` emits |
|---|---|
| `{base}/{peer}/tree/{path}.bin` | `{base}/{peer}/{path}.bin` (§6.5.3.1 has **no `tree/` reserved word**) |
| a raw 33-byte hash at the leaf | `ECF({type:"system/hash", data:H})` — Amendment 6, two-hop |
| `sharded-2-flat` | `sharded-2-4` — *and the profile declares it* |
| hex of the 32-byte digest | hex of the **33-byte wire form** (§6.5.3.1 MUST) |

**Both suites were green the whole time**, because each half asserted its own idea of the layout and
nothing asserted they were the same one. Structurally: **`fetch` had no `make` test target and
`publish` was not in `test-native`** — "run everything" ran neither end. Both are in the sweep now,
plus `fetch` in `LINT_MODULES`.

What replaced the derivation is `fetch.Layout` — the Go counterpart of browser-rust's
`PublishLayout`. Peer-id, all three URL prefixes, content layout and both suffixes come from the
publisher's http-poll profile, decoded with **core-go's own type**. One convention is left on
purpose: the well-known `{origin}/transport-profile` a cold-start consumer enters at.

Gates, both mutation-checked and both in the sweep:
- `publish/consume_test.go::TestPublishThenFetch_TheTwoHalvesOfOurOwnCorridor` — our publisher →
  our consumer over `httptest`, cold start, signed root through `manifest_url_prefix`, every page
  hash-verified, absence reported as `404` rather than as unreachability.
- `fetch/crossimpl_test.go::TestConsumeBrowserRustSite` — **their** emission, frozen at
  `fetch/testdata/crossimpl-rust-site/` (their `dev` @ `fbc2c5c`, provenance in its README), four
  entities across two of their sites. Drop the peer-rooted bridge and it 404s at hop 0 — their
  reported failure, reproduced from our side.

**Findings routed, not worked around.** (1) core-go's `types.BuildContentURL` hexes
`EffectiveDigest()` — the digest-only form §6.5.3.1 excludes **by MUST** — so it builds a URL that
resolves against neither publisher in the cohort. (2) `entity.Validate()` cannot be used on a
`CONTENT_GET` body: those are the bare 2-key hashable form, so it reads the absent `content_hash` as
a zero hash and fails against it. That was our own **AP22** instance — a guard that refuses
everything unfamiliar — the same shape as browser-rust's audit F6, which they reversed the same day.

`tree_url_prefix` stays arch's. We consume **both** joins (last segment exactly the peer-id ⇒
peer-rooted; otherwise append), which is a bridge, not a third convention.

**Ratchet: AP21 → D22.** *A contract between two components is only tested by a test that crosses
it; per-side tests are evidence about each side.* Second shape of AP17 (core-go's
`DispatchLocalExecute` equivalence claim, asserted on return values) — different repo, different
layer, one lesson. Charter is now D1–D22 / AP1–AP22.

Packet: `docs/architecture/reviews/CROSSIMPL-CONSUME-RESULT-2026-08-19.md` (to browser-rust, cc arch
+ core-go).

### 6e. REGISTRY 1.16 read — both rulings land outside our code (2026-08-19)

Arch `d3752ca`, routed as `ROUTING-2026-08-19-d` to **core-go**, not to us. Read under D21 because
its own commit message says it moves a MUST and withdraws a vector row — *"a packet that says it
changes a table, a default, or a MUST is read the same session regardless of who it is addressed
to."* §7 confirms our items (R-8/R-9/R-10) are tracked and that nothing in it waits on us.

Two rulings, both verified against our tree rather than assumed:

- **`REG-NAME-CONSTRAINTS-GRAMMAR-1` row 3 withdrawn.** v1.15 required `x/y/z` admitted while
  §6a name-path safety refuses `/` three subsections earlier — unsatisfiable by every conformant
  impl. Issuer-side; `name_constraints` is core-go's `cmd/entity-peer` + `validate` surface and
  appears nowhere in our tree. Nothing owed.
- **The resolver-side TTL ceiling gets a config site: `resolver_chain[].hints.max_ttl`.** Four
  seats had built three different keys; py+rust's site is ratified, plus durable-config-read-at-
  resolution, `0` is undeclared, and a ceiling against a no-ttl binding yields `local_max`. The
  non-conformant "go" in that finding is **core-go**, and they have already landed it
  (`ext/registry/localname.Handler.Resolve(hctx, name, localMaxTTL)`, `validate`'s
  `v4c_ttl_resolver_ceiling`). Our surface is the config we *write* and *validate*:
  `DefaultResolverConfig` sets no `hints`, and `ValidateResolverConfig` rules only on §4.1 step 2,
  so a config carrying `max_ttl` passes untouched. **Nothing owed — and R-9 got more load-bearing
  than it was when we closed it**, since `hints` is now the ratified home of a spec'd control and
  `19786fb`'s round-trip pin is what keeps a tidy refactor from dropping it.

We deliberately do **not** put a `max_ttl` in `DefaultResolverConfig`. A ceiling is a deployment
choice, `0` is undeclared, and inventing a default here would ship an opinion the spec does not
carry.

### 6f. REGISTRY 1.17 + 1.18 — the load-time refusal we shipped is now a MUST NOT (2026-08-19)

`2d312f1`, answering arch `ROUTING-2026-08-19-i` (arch `3dd5800`) and `-19-j` (`05faaa5`). **Both
are addressed to `entity-core-go` and §7 of the first says explicitly "not yours to chase."** Read
under D21 anyway, because each moves a MUST — and one of them moves a MUST we had implemented the
previous day. This is D21 doing the exact job AP12 earned it for, on the session-start step rather
than by feature work tripping over it.

**§11.1's *"refused or normalized at load"* is WITHDRAWN, and we had shipped the refusal.** §4.1
step 2 binds *a distribution shipping* a config and *a peer storing* one. §6a.9.2's store-first rule
puts an operator's deliberate edit and a distribution's seed **in one entity at one path**, so a
loading resolver cannot observe which act produced the bytes; it necessarily over-enforces, and the
over-enforcement **deleted the operator `MAY` the same paragraph grants**. Replacement, all three
halves a MUST at 1.17: **surface it, never normalize it, never refuse to start.**

Ours refused, and `shellboot` turned that into a fatal — **`entity-shell` would not start** on a
name-disclosing stored config. Fixed:

- `EnsureResolverConfig` records a disclosure condition as a diagnostic and returns success;
  `AppPeer.ResolverConfigDiagnostic` is where the surfacing lands, and `shellboot` prints it to
  stderr at boot. `name config` already printed it beside the config and needed no change.
- **Every other error still returns.** `IsNameDisclosureRefusal` is the classifier that makes the
  split expressible: a policy decision an operator is allowed to have made, versus a config the
  peer cannot read. A reversal like this overshoots in exactly one direction and that is the fence.
- Nothing normalizes — the operator's bytes survive the boot verbatim, which 1.17 generalizes into
  its own MUST (*a resolver MUST NOT rewrite stored configuration as a side effect of reading it*).

**Verified rather than assumed on the rest of both packets:**

- **§4.1 step 2 is KIND-SCOPED** (ruled 1.17, re-derived 1.18): validity is a function of
  `name_format_dispatch` alone, never of `resolver_chain`. **Ours already was**, by construction —
  door 1 never read the chain. Pinned anyway as `REG-DISPATCH-CONFIG-REFUSED-1` row (b): a broad
  rule naming `did-web` with **no** `did-web` chain entry, which a chain-scoped implementation
  accepts. AP19 is why — green under both readings was our only evidence.
- **arch withdrew its own published rationale for that ruling at 1.18** (the "silent arming"
  argument, refuted by §4.1's own whole-config sentence) and kept the conclusion on monotonicity.
  Our doc comment carries the replacement reason and says the pin asserts the behaviour, not the
  rationale — a correct conclusion resting on a withdrawn reason is what gets cited later.
- **§4.3's `set-resolver-config` / `get-resolver-config` `[v1.18]` are new and unimplemented in
  every seat, core-go included.** That is where the operator override
  (`acknowledge_name_disclosure`) lives. **We do not invent a local acknowledgement parameter** —
  the spec says in as many words it MUST NOT become a field of the entity, because a field is
  written by whoever writes the bytes and would move a content-addressed type's hash to carry an
  unsecurable claim. The path an operator has today is the one §4.3 keeps open: a direct tree
  write, which carries no acknowledgement and is therefore surfaced at every load. **The fix above
  is what makes that path work** — before it, the peer refused to boot instead.
- `REG-NAME-CONSTRAINTS-GRAMMAR-1`, the `hints.max_ttl` ceiling, R-4, R-13: all confirmed to land
  outside our tree.

Three pins, all mutation-checked against the pre-fix code — including
`TestBootstrap_ANameDisclosingConfigDoesNotStopTheBoot`, two `Bootstrap`s over one SQLite file
under one keypair, because **the SDK call was correct in isolation and the fatal lived in the
caller** (D22).

**Ratchet: AP25 — an enforcement point that cannot observe the rule's subject.** *Before
implementing a rule, ask whether the point you are implementing it at can see the thing the rule
is about.* The tell is a check whose subject names an actor or an act — *did a **distribution**
ship this?*, *was this **latched** or re-read?* — that the data at that point does not carry.
**Deliberately not promoted:** this shape has bitten once. AP19 is a *test* that could not tell two
rules apart and AP22 is a guard keyed on familiarity; neither is this. Charter is D1–D22 /
**AP1–AP25**.

### 7. The name arc reaches a user, and the handler browser closes the parity gap (2026-08-19)

`ddde4c7` + `27874ad` + `6204630`. Two gaps that were both *reachability*, not features.

**EXTENSION-REGISTRY §11.2 lists "UI / CLI surface for local-name bind / unbind / list" as a
SHOULD.** We had none, and could not have had one: `shellboot` never set `Extensions.Registry`, so
**no shipped binary carried the handler a verb would dispatch to.** ResolveName, BindLocalName, the
resolver-config validator, the v1.13 adoption, the 1.14 re-key, the hints pin — all reachable only
from unit tests. AP21's shape again: green at every unit boundary, broken at the one seam no test
crossed.

**The default was guarding a cost that is not there.** `app.go` justified registry-default-OFF with
a claim about the sibling — local-name default-grant caps "re-minted on every bootstrap", linear
growth. Priced against the substrate (D20): **zero marginal per-restart cost on both counters**;
the whole cost is **+8 paths / +8 entities, once**. The growth originally seen is the +2
entities/restart a **registry-less** peer pays too (core-go's, same family as the waived
identity-rebootstrap leak). `entitysdk/registry_bootstrap_cost_test.go` asserts the differential,
never an absolute.

Landed: the three missing local-name SDK ops (`ListLocalNames`, `UnbindLocalName`,
`UpdateLocalNameTransports`, plus `WithNotes`) — the kernel declared all four since the handler
landed and we had wrapped one; registry ON by default in `shellboot` with `DisableRegistry` /
`-disable-registry` to opt out; §4.1a's default resolver-config shipped via `EnsureResolverConfig`
(a config failing the §4.1 step 2 MUST is fatal at boot, not silently resolved through); and the
`name` verb — `ls` / `resolve` / `bind` / `unbind` / `config`, with `@alias` targets and failures
that name the **rung** they stopped at.

**Driven through `bin/entity-shell` across separate processes**, not only in tests. That found a
pre-existing property worth knowing: **without `-identity` the peer-id is regenerated per
invocation**, so each process writes a different namespace of the same SQLite DB and nothing
appears to persist. Affects every persistent surface, not just names. Under `-identity` the whole
cycle round-trips.

**The handler browser** (`27874ad`) closes the last console→Avalonia parity gap.
`wb.HandlerBrowserModel` was complete and renderer-neutral all along; only tview drove it. The
bridge is handle lifecycle + a JSON projection, the panel is controls. A real-X11 driver
(`make smoke-xvfb-handlers`) found a crash headless was structurally blind to — see AP24. 21
handlers walked, 53 output rows, exit 0; `system/registry` and `system/registry/local-name` appear
in that walk, which is the shellboot default confirmed live in the GUI rather than argued from
source.

**Ratchet: AP23 + AP24.** *A measurement with no control arm* (the probe that manufactured a
Δ370/restart leak in both arms), and *a test that asserts on the first item cannot see a bug that
needs a second one* (the headless suite that never changed the selection). Charter is now
D1–D22 / **AP1–AP24**.

Also this session: **60 files of gofmt drift** cleared in its own commit (`6204630`) — `make lint`
is `go vet` only and has never gated formatting.

**Owed, not done:** `UpdateLocalNameTransports` has no verb, because a binding's transports are
hash references to transport-profile entities and no surface hands a user one. A `-transports`
flag that can only take a hash nobody can obtain is worse than none. Wants the profile-hash story
first.

### 8. The last renderer gap closes, and the front door gets documented (2026-08-20)

Three things, one session, all follow-ons from the bearings audit.

**`dev` is pushed.** 14 commits to `origin/dev`. The four review packets that existed only
locally — two of them naming live core-go bugs (`CORE-GO-LAST-BURST-WRITE-LOSS`,
`CORE-GO-SUBSCRIPTION-DELIVERY-RING`) — are readable by their addressees now. That was the
cheapest item on the list and it had been sitting.

**`PeerLiveness` reaches a user.** The audit's one remaining renderer gap: the model was
tested, the bridge exported it, and **no C# file referenced the export**, while
`PeerConnectionsPanel` rendered `ConnectionsOpen` — the connection-pool snapshot the liveness
model exists to replace. The GUI showed a strictly weaker answer with the correct one one
unused export away.

What landed is **not** the one-shot export wired to a button. That shape was wrong twice over:
it re-paid the model's O(N) seed on every call, and having no wake it could never deliver the
one transition the pool cannot express — **a demotion to `suspect` writes `system/peer/status`
and nothing else**, so a panel refreshing on connection events would never see it. So
`avalonia/bridge/liveness.go` is the standard handle lifecycle (`LivenessOpen` /
`RegisterWake` / `Render` / `Close`, cascade-on-peer-destroy), holding one long-lived
prefix-subscribed model; the one-shot `PeerLiveness` export is gone.

The panel now shows **both** surfaces, labelled apart: *"Liveness (tree)"* with per-status
colour (`suspect` gets its own — collapsing it into green or grey discards the whole reason
the section exists) and the three counts, above *"Connections (local pool)"* which keeps the
dial/drop buttons. `last_seen` is still not exposed anywhere: transition-written (§5.4.1),
so rendering it as freshness would invent a contract the protocol declines to offer. The
empty state says **"No lifecycle transitions recorded"**, not "nothing connected" — absence
means no transition was ever written, and a test asserts that wording.

**Avalonia 60/60 headless** (was 57), plus a new tier-3 gate: `make -C avalonia
smoke-xvfb-connections` drives the panel under real X11 and churns both renders — the
clear-an-ObservableCollection-under-selection shape that killed the handler browser in X11 and
nowhere else (AP24). Its log states what it cannot prove: one peer writes no transitions, so
row *content* is not under test.

**The front door is documented and has a fast rung.** `make gui` rebuilds the image before
launching, which is right after a code change and wrong when you just want to look at the app —
and there was no other verb, so "start the thing I built five minutes ago" cost a full podman
build. Added **`make gui-run`** (launch, rebuild nothing) and `ARGS` passthrough on both
(`make gui-run ARGS="--identity me --storage sqlite"` — the .NET frontend takes double-dash
flags, not Go's `flag` spelling; **with no flags the GUI is an ephemeral in-memory peer that
loses everything on exit**). `avalonia/README.md` was rewritten: it had described the renderer
as a three-spike POC, pointed at a `PHASE-I-DESKTOP-RENDERER-PLAN.md` that does not exist, and
listed 7 bridge symbols when there are 118 — the same *doc-points-at-a-missing-file* failure
the audit found in `AGENTS.md`, on the one page a newcomer reads first.

**Ratchet: D23** — *a model with no shipped surface is not shipped.* Third instance of one
shape (name arc, handler browser, this), two of the three found by audit because no test
crosses "can a user reach this". Enforcement is real, not aspirational: **`make reachability`**
runs both sweeps (bridge exports no C# consumes; workbench models no renderer or verb drives)
and exits non-zero on the first orphan. Both are empty as of this commit. Charter is now
**D1–D23** / AP1–AP26.

**Owed:** `make reachability` is not in `check` yet — one clean sweep is not enough evidence
that it will not false-positive on a legitimately internal model. Join it after a few sessions.

### 9. The D21 sweep, run late: eight unread packets, and one of them is a hold on us (2026-08-20)

`grep -ril 'workbench-go' ../entity-system-architecture/docs/status/` returned **eight documents
newer than our last-read marker** — the entire 2026-08-20 set plus `ROUTING-2026-08-19-k` and
`STATUS-2026-08-19`. All read this session. This is the discipline working late rather than not at
all, and it is worth noting that the previous session's audit did not run it either.

**Nothing in them assigns us work.** Three findings, in order of how much they change:

**1. T5 · COMPUTE is `HELD BY DECISION` — and arch's own §4 says the hold was never delivered to its
driver. We are the driver.** `PROPOSAL-COMPUTE-COLLECTION-PRIMITIVES` (the `concat` family) is
**parked**, not queued. We had it recorded here as *"gated on arch"*, which reads as a queue
position; a decision is a different thing and we were carrying the wrong one. Arch calls it *"the
highest-value unaddressed item on the board"* and says it needs an **operator call**: does compute
reactivate this cycle, or does the hold stand and get told to its driver? Five of the 39 active
proposals sit downstream of it.
**Why it lands here and not only on the compute tier:** the hold is upstream of the HUD text port →
which is upstream of retiring the legacy panels → which is why `dev` cannot cleanly release (see the
guardrail). Routed with the cost measured, and with the uncertainty stated — the HUD may be
expressible on today's `map`/`fold`/arith vocabulary without the parked primitives, and we have not
run that spike: `reviews/COMPUTE-HOLD-IMPACT-2026-08-20.md`.

**2. R-10 is arch's next item, and their board lists us as blocked behind it. We are not.**
`STATUS-2026-08-20-b` §3 and their handoff both name `entity-workbench-go` as *"blocked on it
today"* and put the fold first *"before more design"*. We built against the ruling and shipped;
the fold's absence holds no code here. Corrected in the reply so the fold gets sequenced on R-25's
need rather than on a seat that is not actually waiting.

**3. The transport-set round opened, and §2.3 asks every seat to grep its tree.**
`ROUTING-2026-08-20-d` — `PROPOSAL-PEER-TRANSPORT-SET` fourth pass, answering R-22 (*a consumer
holding only a `peer_id` cannot learn where that peer is*). Not ruled, no work assigned; arch wants
implementation evidence before ruling. We are the seat shipping **both ends** of the profile path,
so we answered with measurements rather than opinion:
`reviews/TRANSPORT-SET-IMPLEMENTATION-EVIDENCE-2026-08-20.md`. The three findings worth repeating
here: `transport.set|transport_set|TransportSet` is **zero occurrences** in our tree (nothing to
collide with); our three self-published profiles are **coexisting singletons, not a set**, and our
one signed aggregate (the published root) is not over profiles; and on R7's confirm-before-publish
`SHOULD`, **our missing half is not the confirmation, it is the expiry** — we refuse structurally
undialable addresses at the publish site with no network, but a profile has no TTL, so a rule that
binds only the publish instant buys less than it looks like.

**Three packets went out on the strength of this sweep:** the transport-set evidence, the
compute-hold impact, and — separately, closing share-arc step 6 — the
`APP-CONVENTION-CHAT` consumer review. All three carry measurements from this tree rather than
positions; none of them asks for a ruling.

**Also noted, no action:** arch records that our v1.17 refusal-at-load crash independently validated
their withdrawal (`ROUTING-2026-08-20-a` §6), and corrects the record that our posture matching
browser-rust's on the signed-root path is **cohort-consistency, not convergence**
(`ROUTING-2026-08-20-b` §3) — the same distinction we owe in the other direction.

### 10. The console/TUI review, asked for and run: it is not behind — it is bounded (2026-08-20)

Operator asked whether the TUI has fallen behind and whether a catch-up is owed. **Measured, not
estimated** — every `workbench/*_model.go` against both renderers:

| Model | console | Avalonia |
|---|---|---|
| detail · handler · log · markdown_files · markdown_view · peer_info · query · site · tree | **yes** (9) | yes |
| `peer_connections` | no | yes |
| `peer_liveness` | no | yes (new today) |

**Nine of eleven, and the two absences are the policy line, not drift.** Both missing models are
**multi-peer connectivity** surfaces, and console is **frozen and single-peer by policy**
(`AGENTS.md`: multiple renderers are a discipline enforcer, not a parity obligation). A single-peer
TUI has no connections list to render and no second peer to hold a lifecycle transition for. The
compute/program panels are absent for the same reason — that track is Avalonia's.

**So there is no catch-up debt**, and the D18 compile gate that keeps it honest is green:
`make build` produces `entity-shell` + `entity-console` + the three corridor binaries at `1398139`.
The thing to watch is not console falling behind; it is a *model* change that breaks console, which
is the signal that the abstraction was wrong — that is what the gate is for.

**If console ever goes multi-peer**, `peer_liveness` is the one worth taking first: it is a flat
sorted list with three states, which is the cheapest possible tview surface, and the CLI already has
the same answer under `peer status`.

### 11. Compute is sequenced, not stalled — and the cross-impl program gate is measured clean (2026-08-20)

Arch's `ROUTING-2026-08-20-e` read at arch `8dd5689`, tree green at `2d79fa2`. **The compute
disposition arrived, and it closes the item §9 opened.**

**The decision.** T5 compute is **deferred by operator decision and resumes after the release** — not
doubted, not dropped. Two stated reasons: the reachability stack (registry / routing / relay /
network / transport) is **cross-peer observable** and therefore the part that cannot be fixed later,
while compute's outstanding refinements are mostly *within* a peer; and compute is opt-out in a way
the network layer is not (an app that never touches `system/compute` is a complete app; one that
cannot reach a peer is not). **The five levers stay arch's and stay on the board.** What changes here
is only the reading: they were **paused**, not unanswered, and §1's ledger now says so.

**The two packets crossed.** `-e` was written against our tree at `2f285f7` — *before*
`COMPUTE-HOLD-IMPACT-2026-08-20.md` landed (`42bd8c1`, corrected `f887f3d`) — and arch says outright
it is not claiming to know our current state. Both documents reached the same answer independently:
the hold costs this seat nothing today, and **the defect was the delivery, not the decision**.

**Lever 1 re-measured, since arch asked to be told if our tree moved.** It has not moved on compute:
`programs/` carries no functional change since `968190b` (2026-07-27) — only `6204630` (gofmt) and
`a5752e2` (the frozen Life oracle, test-only). **`concat`/`range`/`group-by`/`assoc` are implemented
nowhere**, independently confirmed at core-go `0332c90`: the builtin set is still
`{arithmetic, compare, logic, field, construct, map, filter, fold, store}`. We do not ship builtins —
the seat is core-go's — so the fold (`EXTENSION-COMPUTE` 3.23 → **3.24**) changes nothing in this tree
today. Note `assoc` folded with **`assoc` MUST NOT be an implicit lowering target**, which binds
`compute_lower.go` if we ever lower a fold; and the naming gate re-spelled `group_by` → **`group-by`**
(kebab in a path segment). Arch's `WORKSTREAMS.md` lever-1 row cites this fold as "`EXTENSION-COMPUTE`
1.4→1.5" where the spec header says 3.23→3.24 — cosmetic, theirs, flagged not filed.

**The cross-impl program gate: measured, not assumed.** Operator asked whether `entity-browser-rust`
had fallen behind on compute. **It has not**, and this was settled by regenerating rather than reading:

- Their host is real and Mount-only — `src/program_host/` (2916 lines: bundle, controls, descriptor,
  host, input, sexpr, shapes) with a DOM/SVG `display-list` driver that fills **closed quads**, i.e.
  our de-facto ABI. Their 07-19 review's **F2** (`display-list` arity — spec says variable polylines,
  we shipped fixed quads) is therefore resolved *by adoption* and still **unruled**; **F1** they closed
  with the fetch-by-hash → materialize → eval-by-path sequence they proposed.
- Their fixtures are generated from **our** tree: `tools/program-dump` (theirs, `make
  program-fixtures`) runs the unchanged `programs` authoring through a `replace` onto this repo.
- **We re-ran it against `2d79fa2` into a scratch dir** (their tree untouched, verified clean before
  and after) and diffed: **the per-tick oracle is byte-identical for all three programs** —
  life 12/12, snake 12/12, asteroids 16/16 ticks. Entity counts match exactly (130 / 191 / 477).

**The only diffs are the dump tool's per-run random `origin_peer`**, which re-keys the handful of
entities whose IR embeds a peer-qualified path (4 same-path hash changes + 3–6 content-addressed
`_expr/{hash}` renames per program). So a regeneration **always** produces a dirty `git diff`, which
means "are these fixtures stale?" cannot be answered by looking — it has to be answered by comparing
oracles, as here. **Worth routing to browser-rust as a suggestion** (seed the dump keypair and the
fixture regen becomes reproducible), not a defect: their build-time `all_embedded_entities_hash_verify`
+ `corrupted_entity_is_refused` + `oracle_tests` already make a stale fixture a build failure on the
Rust side. Their fixtures were last regenerated at their `98671db` (2026-07-28) — the day after our
last functional `programs/` commit, which is why they are current.

**One thing we own and got wrong.** `PROPOSAL-GENERIC-HOST-POINTER-INPUT-DEVICE-2026-07-27.md` is
addressed *"To: arch + entity-browser-rust"* and this file has recorded it as **"blocked on arch"**
for 24 days. **It never left this repo.** Exhaustive search across both sibling trees — by filename
and by five distinctive phrases (`third input device`, `shapeDrivers`, `life-edit`,
`AuthorLifeInteractive`, `click a cell`) — returns **zero** hits, and arch's `WORKSTREAMS.md` generic-
host block lists the five compute levers with no pointer item. `pointer` is named in
`PROPOSAL-APP-CONVENTION-COMPUTE-PROGRAM` §-taxonomy as a held-state device, but that proposal is
DRAFT since 07-13 and carries no ABI for it. **This is AP12's mirror image** — that one was a routed
packet nobody opened; this is an unrouted packet we recorded as routed, and the failure mode is worse
because the ledger reads *waiting on them*. Not urgent (it is a post-release host gap, and the d-pad
cursor in `programs/life_edit.go` is the honest maximum on the two devices that exist), but the
delivery is owed and costs nothing — the packet is already written. **AP28**, with the outbound
sweep as its enforcement point (`AGENTS.md`, beside the inbound one).

**And the sweep's own first run corrected how it should be written.** Grepping siblings for our
packet *filenames* flags **twelve of the packets in `reviews/`** — and **eleven of the twelve had
plainly landed**: AE-5 is folded into `EXTENSION-COMPUTE` §11, `EXTENSION-DISCOVERY` §-mDNS names
this repo, four `WORKSTREAMS` rows carry our transport findings, and R-10's fold carries all four of
our rendezvous items. **Siblings cite our commits and our claims, never our file paths**, so the
filename test fires on almost everything and means almost nothing. The **subject** test — grep their
board and specs for what the packet is *about* — fired exactly once, on the pointer proposal. That is
the version that went into `AGENTS.md`; a gate that flags twelve to catch one gets ignored by the
third session that runs it.

**Tree state at the time of this entry:** `make test-each` **all ten suites green to completion**
(sdk 199s · shellcmd 287s · programs 144s · the rest under 15s), `make lint` clean, `gofmt -l` empty,
`make reachability` clean, working tree clean at `2d79fa2`.

### 12. The cross-impl consume leg — BUILT, RUN LIVE, and shipped to a UI (2026-08-20)

**`entity-core-go` handed us WS-A and it is done.** Their
`HANDOFF-2026-08-20-workbench-go-federation-consume-leg.md` asked for the axis
`entity-browser-rust` structurally cannot cover: **a reader in a different language than the
emitter**, over a live host boundary. `make crossimpl-go` is green.

Result packet, with everything below in full:
**`docs/architecture/reviews/CROSSIMPL-CONSUME-LEG-RESULT-2026-08-20.md`**.

**The operation that was missing, stated once so nobody re-derives it.** We had both halves and
neither walked the trie: `entitysdk.ReadPublishedRoot` verified a signature but over the **local
store**; `fetch.Fetch` crossed a wire but resolved by the **advertised leaf path**. Joined, the
chain is manifest → signature → **CHAMP trie walk from the signed root** → leaves, and **only the
walk can tell a complete origin from a withholding one** — every other step is satisfiable by an
origin serving a correctly-signed root that commits to nothing. That is not hypothetical: core-go
shipped exactly that for a week (a `0xC0C1C2…` literal root, fixed at their `dabd076`) and every
per-leaf consumer in this cohort reported green against it.

| measured | result |
|---|---|
| **live**, core-go's publisher on a podman bridge, our consumer in a second container | signature VERIFIED (ed25519, key from their peer-id) · 3 keys · **3/3 reconciled** · absent control fired |
| **offline**, browser-rust's frozen emission (their `fbc2c5c`) | signature VERIFIED · 15 keys · **15/15 reconciled** |
| **our own** publisher, 64 keys, 5 CHAMP nodes | exact enumeration + three mutation controls |

**Reconciliation is ours and nobody else in the cohort does it.** Every committed key is resolved
**twice** — through the signed trie and through the publisher's own advertised tree-leaf URL — and
the two must agree. *An origin that answers differently on the two paths is serving two trees and
only one of them is signed.* Green on all three emissions.

**What ships, in one line each:**
- `entitysdk/publishedroot/` — the seven published-root gates, extracted so the store-side reader
  and the wire consumer apply the **same** checks. Dependency-light so `entity-fetch` links no peer.
- `fetch/consume.go` — `Consumer`, the **strict** walk, `PinnedLayout`, `AbsolutePrefix`, the
  reconcile pass, typed `ErrIncompleteWalk` / `ErrAbsent` / `ErrEmptyEnumeration` / `ErrContractMismatch`.
- `scripts/crossimpl-go.sh` + `make crossimpl-go` — outside `test-native` on purpose (podman + a
  sibling checkout).
- **Three operator surfaces (D23):** `entity-fetch -verify [-json]`, `entity-shell`'s **`site
  verify`**, and the Avalonia **Publisher Verify** panel over `workbench.ConsumeModel`.

**The UI is deliberately the opposite of browser-rust's.** Their Site Browser renders the *pages* —
the reader's surface. Ours renders the **chain**: seven steps, each with its verdict *and what a
green verdict on that step actually proves*, because five of the seven are true of an origin that is
lying. A UI that collapses this into one tick teaches an operator that "verified" is one fact. It is
seven, and one of them (`published_at`) is a **moment** rather than a state — which is what the
freshness line under a green verdict says, on every surface, and never omits.
**That contrast is the UX research, not a duplication of theirs.**

**Two things building it found, both routed:**
- **AP29 — the handoff's own advice would have produced a blind consumer.** It said to walk with
  `core/tree.CollectAllBindings`. That helper (and `CollectNodeClosure`) is documented best-effort —
  *"Missing nodes are skipped"* — which over an HTTP origin turns a withholding origin into a
  **smaller site**, silently. Ours re-implements the traversal over their `core/types` and fails
  closed. Mutation-checked: `publish`'s `TestConsumeWithholdingInteriorNode` removes one **interior**
  node and requires `tree/incomplete-walk`; a best-effort walk passes it with fewer keys and no error.
- **AP30 — §3.3's prefix, and we were right by luck.** The wire field is the **configured** prefix
  and all three admissible shapes are live at once (browser-rust `/{peer}/`, ours `docs/`, core-go
  `system/`). Concatenating it verbatim is correct for two of the three, for two different reasons,
  neither of them the rule. **A mis-joined path is a 404, and at a consumer a 404 is
  indistinguishable from a withholding origin** — the bug reports as the other side's defect. Fixed
  as `fetch.AbsolutePrefix`, pinned across all three rows, and confirmed live against core-go's
  `system/` shape.

**Charter is now D1–D23 / AP1–AP30.** Neither is promoted; each has bitten once.

**Not claimed:** two physical machines, the public internet, TLS, a CDN, NAT. And **B5**
(name → binding → transports → fetch) is not in this leg — core-go's origin publishes no registry
or bindings, exactly as `ROUTING-2026-08-20-m` §3 says.

### 12a. What `ROUTING-2026-08-20-l` settled (2026-08-20)

- **The pointer proposal was delivered by pushing `dev`** and arch read it at `98ff6de`. It is
  `COHORT-OPEN-ITEMS` §1d row **W-1, owner arch, blocked on us: nothing**. Not ruled this session
  **by stated decision** (the T5/T4 seam; the design axis stays deferred) — which is the
  confirmable kind of deferral, and the whole point of AP28.
- **Arch adopted AP28 verbatim** and filed their half: for 24 days nobody noticed a driver had gone
  silent, because *a blocked-on-us row and a paused-by-us row look identical from the outside*.
- **R-10 is folded — `EXTENSION-DISCOVERY` 1.0 → 1.1**, with all four of our rendezvous build
  findings landed in full. `entitysdk/rendezvous.go` now sits on landed spec; the fold-ask closes.
- **R-9 (`hints` round-trip) — answered here, both halves.** Arch's worry was an SDK that rebuilds
  `resolver_chain` entries field-by-field and drops the new `max_ttl` / `neg_ttl` keys, disarming a
  security control with every test green. **Refuted for this seat:** `ResolverConfig()` decodes
  straight into `types.ResolverChainEntry` (whose `Hints` is `map[string]cbor.RawMessage`,
  `registry_ext.go:303`) and `InstallResolverConfig` encodes the caller's struct — nothing
  reconstructs an entry. **Their other half confirmed:** we author no ceiling and consume none.
- **Compute, corrected:** `concat` / `range` / `group-by` / `assoc` **are** implemented in core-go
  (`ext/compute/builtins_v324.go`, `eb80750`, corrected at v3.25 `4cd1ee8`). §11's *"implemented
  nowhere, at core-go `0332c90`"* was measured **before** that commit — `0332c90` is an ancestor of
  `eb80750`. **Lever 1 is no longer waiting on anybody.** Compute stays post-release by operator
  decision; what changed is the *state of the lever*, and two consequences are now ledger rows:
  `programs/` still routes around `concat` in four places, and **Axis-1 — our conformance-admitted
  alternate engine — implements none of the four**, against a corpus that has grown to 350 vectors.

### 13. The naming hop — BUILT both directions, and four cross-impl findings (2026-08-21)

**The journey is closed.** Consume: pin a name authority → **walk** it for its names (§6a.3a)
→ resolve one with every §6a.4 check → follow the binding to its publisher → verify *that*
peer's signed root → walk it → render the page. Serve: `registry issue` mints and signs a
binding into this peer's tree, and publishing `system/` emits it as a static registry another
peer can pin and browse (§6a.8 / §7.4). Ran end to end by hand, both directions.

**Three surfaces (D23):** `entity-shell`'s `registry` / `browse` / `open`, the Avalonia
**Browser** panel (page in the middle, the ten-step trust chain in the right column), and
`entity-fetch -registry -names / -name`.

**D20 paid for itself before a line was written.** `entity-core-go`'s
`ext/registry/peerissued` already has all of §6a.4 including the association check plus an
`HTTPPollReader`, so the gap was **wiring, not a backend** — `entitysdk/registry_pin.go`
implements no spec logic. What the kernel lacks is the §6a.3a **enumeration**, which arch
(`ROUTING-2026-08-21-b` §3) records as *the one registry surface in the corpus with a producer
and zero consumers*; ours is the first.

**Routed:** `reviews/REGISTRY-BINDING-TRANSPORTS-DIVERGENCE-2026-08-21.md` — four findings.

| § | finding | who |
|---|---|---|
| §0 | `transports` has two live readings (`Vec<Value>` inline vs `[]hash.Hash`); **core-go's backend cannot decode any binding in the cohort's only live federation** *(**ruled 2026-08-21, hashes, our reading — see §24.1**; true as measured on this date)* | arch to rule; core-go or core-rust changes |
| §6 | **§6a.3a's recommended publishing prefix is unimplementable** — a binding's signature is at `system/signature/{hex}`, outside every `system/registry/…` prefix, so a conforming registry enumerates fine and resolves nothing | arch |
| §4 | core-go's `httplive.Outbound` implements one branch of the §6.5.3.1 tree-URL join, so fed browser-rust's own profile it builds `/{peer}/{peer}/…` | arch's existing AP30 ruling |
| §7 | the `.list` artifact is a `system/tree/listing` **entity** per spec; browser-rust emits newline text | browser-rust |

Plus one declinable ask to core-go (§5): **export `normalizeName`** — a Layer-2
canonicalization we had to transcribe.

**Held together by a differential test, not by intent.** `workbench/registry_differential_test.go`
runs core-go's backend and ours over the same frozen bytes and requires the same verdict; it
currently **records the divergence as measured state** and fails if it widens *or* silently
closes.

**Frozen fixture:** `fetch/testdata/crossimpl-rust-federation/` — browser-rust's `make
federation` emission (a registry signing four names + the four domains, 784K). It exercises
both layout modes in one run: the registry serves no `transport-profile` (conformant, §6.5.4,
R-28) so it is pinned; the domains advertise one so they are discovered.

**Three anti-patterns earned.** AP31 (an async cgo export read a C string after the P/Invoke
freed it — no crash, it reads as `""`), AP32 (a derived UI property used as a completion
signal: four headless tests green while measuring nothing), and **AP33** — two pre-existing
`put` bugs found by seeding a site by hand for the demo, not by any test.

## Open bugs

- **~~Managed stack overflow on window minimize~~ — RESOLVED 2026-08-21** (it was never
  minimize; see the dated block at the end of this entry). Kept in full because the *way* it
  stayed open for a month is the lesson, and D24/AP34/AP35 were earned on it. A tight
  alternating A↔B JIT recursion into the .NET guard page, **zero GPU/GL modules in the
  faulting thread** — so it is ours, not the driver. Full forensics:
  `docs/status/HANDOFF-2026-07-18-avalonia-64x64-segfault.md`.
  **New this session:** `PanelStack`'s class doc records **two earlier SIGSEGVs with the
  identical signature** (PIDs 4033208, 4035311) from a GridSplitter drag driving a
  star-weighted row to **zero size** — an upstream Avalonia layout-engine recursion, fixed
  structurally with `RowDefinition.MinHeight`/`MaxHeight` pins. Minimize is the *same
  zero-size condition arriving from above* (the window collapses the ScrollViewer viewport),
  which those row pins do not cover. That is the working hypothesis.
  **The headless repro did NOT fire** (all three tests pass): 25× collapse-to-0×0-and-back,
  25× `WindowState.Minimized`/restore, and 40× collapsing the 64×64 Life panel with a live
  repaint stream in flight are all survivable under headless Skia. That is a *negative*
  result kept as a regression fence — it narrows the search without closing it, since
  headless does not run the X11 backend at all, which is where both documented predecessors
  lived. **Both original leads are now closed:** the other one — symbolizing the core dump —
  died with rotation; the `entity-avalonia` dumps are gone from
  `/var/lib/systemd/coredump`.
  **2026-08-19 — the X11 rung was built and it is a third negative, but a much sharper one.**
  `make smoke-xvfb-window` drives the window geometry itself under Xvfb while a program
  paints, which is the rung between headless (no X11 at all) and the desktop (crashes,
  dump rotated). Two runs, both survived, both with the transitions **verified to have
  actually taken effect** rather than assumed:
  - `MODE=resize` (no WM): 39/40 transitions, ClientSize alternating 1400x900 ↔ 1x1 with the
    64×64 Life panel painting. Survived.
  - `MODE=minimize WM=1` (openbox on the virtual display): 39/40 transitions,
    `Normal ↔ Minimized`, real iconify. Survived.
  **And the second run damaged the working hypothesis.** The theory was that minimize
  collapses the ScrollViewer viewport to zero from above — but under openbox, `ClientSize`
  stayed `1280x900` across every minimize. Minimize did not collapse anything. So either the
  zero-size condition comes from the **compositor** specifically (mutter/kwin, not openbox),
  or the viewport-collapse theory is wrong and the fault is elsewhere in the iconify path.
  Next rung: a compositing WM in the harness, or a capture on the real desktop.
  **The instrumentation caught itself, which is the reason to trust it.** The first version
  read ClientSize back in the same tick it requested the change; an X11 geometry change lands
  asynchronously, so it reported *"zero transitions"* for a run whose own log showed the
  window collapsing twenty times. A gate that miscounts in the reassuring direction is worse
  than no gate. It now compares against the previous tick's observation and prints, for every
  run, how many transitions actually took effect — with an explicit "this run is NOT evidence"
  line when that count is zero.
  **2026-08-20 — a fourth negative, and an accounting of what is actually left.** Operator
  challenged whether this bug is still real or is being carried forward on old text. It is
  being carried, and here is exactly what stands behind it:
  - **The original evidence was real** — three core dumps (PIDs 3239906 / 3331097 / 3513953),
    analyzed, with the discriminator recorded as a count anyone could re-run
    (`coredumpctl info | grep -icE 'gallium|GLX_mesa|libGL\.|swrast'` → 21 for the GPU crash,
    **0** for this one). That is what makes it "ours, not the driver."
  - **No artifact remains.** `coredumpctl list | grep -c entity-avalonia` → **0** today. The
    dumps rotated out of `/var/lib/systemd/coredump`. Nothing is left to re-examine.
  - **Four repro attempts, four negatives**, the newest on today's binary:
    `make -C avalonia smoke-xvfb-window MODE=both` — 60 iterations, **59 transitions verified
    to have taken effect**, collapse to 1×1 and restore with paint in flight, exit 0.
  - **The stress coverage is real and specific**, not a claim: 25× width→0, 25×
    `Minimized`/restore, 40× collapse-while-ticking (`PanelStackZeroCollapseTests`), plus the
    400× real-Skia rasterize in `ProgramPanelStressTests`. All in the 60/60 headless run.
  **Disposition, so this stops being carried by default:** it stays open only as *last seen
  2026-07-18, unreproduced since, no artifact*. The next step is an operator capture on the
  real desktop (compositing WM — mutter/kwin, which is the one condition the harness has never
  had). **If the next occurrence produces no dump, close it as unreproducible** rather than
  keeping a month-old symptom on an open-bug list, which is how a stale entry starts steering
  work it can no longer justify.

  ### 2026-08-21 — REPRODUCED, CHARACTERIZED, FIXED. Closing.

  The operator capture arrived (two desktop SIGSEGVs, PIDs 619966 and 1462191, both preserved
  before rotation this time). It was **not** the minimize path, and the harness never had a
  chance of finding it, for a reason worth keeping:

  **Root cause: the alternate signal stack overflows.** The PAL gives the UI thread a
  **16384-byte** alternate signal stack (measured at startup, not inferred:
  `CrashDiagnostics.ReportAltStack`). Under real pointer input the handler chain on that stack
  exceeds 16 KB and the next `call` pushes its return address into the guard page. Caught live
  under gdb with `nopass`, twice, identical:

  ```
  Thread 1 received signal SIG34, Real-time event 34       <- runtime thread-suspend injection
  Thread 1 received signal SIGSEGV
    si_code = 2 (SEGV_ACCERR)   si_addr = rsp - 8   rip on a `call`
    rsp inside a PROT_NONE page; the mapping is the PAL altstack, not the managed stack
  ```

  **Every previous reading of this bug was taken from the wrong signal.** CoreCLR's handler
  cannot classify the fault, so it **re-raises** — and the re-raised signal is what lands in
  the coredump, carrying `si_code 128` (SI_KERNEL) and `si_addr 0`. That artefact is why the
  cores read as a null dereference and why the runtime never printed `Stack overflow.` and
  createdump never fired: by the time it faults there is no stack left to report on. Rule:
  **on a .NET Linux crash, check `si_code` before believing `si_addr`.**

  **Fix:** install a 1 MB alternate signal stack on the UI thread at startup, default on
  (`CrashDiagnostics.EnlargeAltStack`, `WB_ALTSTACK_BYTES=0` restores stock for re-measuring).
  A `PRIVATE|ANONYMOUS` mapping commits lazily, so the cost is the pages a handler actually
  touches, against an unrecoverable process kill.

  **Evidence — A/B, same binary, same seeds, only the env differs:**

  | arm | crashes |
  |---|---|
  | control, stock 16 KB (`WB_ALTSTACK_BYTES=0`) | **6 / 8 seeds** |
  | 1 MB altstack (default) | **0 / 8 seeds** |

  Plus 5/5 → 0/5 on an earlier pass over the seeds that had failed 5/5.

  **Still open, narrowly:** *what* consumes more than 16 KB is not identified — nested signal
  delivery on the altstack is the leading candidate, and `GODEBUG=asyncpreemptoff=1` (5/5 still
  crashed), `DOTNET_gcConcurrent=0` and `DOTNET_TieredCompilation=0` are all ruled out as the
  trigger. The enlargement removes the crash without explaining the appetite. Also **only the
  UI thread is protected** — every other managed thread still runs the stock 16 KB, and a
  crash on one of those would look identical.

  **Why four harness attempts missed it, which is the part that generalizes:** every driver in
  this repo called the model method *under* the control
  (`SiteViewPanel.NavigateForTests`, `HandlerBrowserModel`, the window driver). None of them
  ever produced an X11 pointer event, so none could execute input dispatch, hit-testing or
  focus transfer — and this bug lives only there. `make -C avalonia smoke-xvfb-click` (real
  `xdotool` clicks, seeded and logged) reproduces in under 45 s; `make crash-hunt` sweeps
  seeds unattended and stops at the first hit with a replay command.
- **GPU-driver SIGSEGV** (distinct, older): the mesa hardware-GL path crashes under
  sustained compositor load. **The product call is made (2026-08-19): software Skia is the
  DEFAULT, hardware GL is opt-in via `WB_GPU_RENDER=1`.** Auto-detect was the other
  candidate and was rejected — detecting a bad driver from in-process means fingerprinting
  mesa versions, which is a guess that ages badly. The two outcomes are not symmetric: the
  cost of software render is frames per second on an app that is mostly static text and
  small grids, and the cost of the GPU path on an affected driver is a hard crash. A slow
  window beats a dead one. `WB_SOFTWARE_RENDER=1` is still honored (now a no-op), and the
  chosen mode is announced on stderr because it is the first thing a crash report needs and
  the last thing a reporter includes.
- **Asteroids "keys" report** — flagged by the operator, never reproduced. Key wiring was
  checked and is correct; suspected to be the GPU crash hit while interacting. A headless
  key-injection repro is still owed.

## Backlog

**Release / dependency cutover**
- **Module-path cutover.** Every `go.mod` requires the kernel by its vanity path
  (`go.entitychurch.org/entity-core-go/{core,ext}` @ v0.8.0) but resolves it through a
  local `replace` to the sibling `../../entity-core-go/{core,ext}` (offline; no network, no
  tag). When the vanity path is published + tagged, the cutover is **one line per module**:
  drop the `replace`, let `require … @v0.8.0` fetch. Then run the deferred no-siblings,
  clone-fresh `make build` to prove it.
- **`USAGE-PROTOTYPE-FILESYSTEM-SYNC.md` §10 points at a renderer that does not exist.** It
  sends the reader to the `canvas/` application for a graphical tree browser; canvas was
  removed. The section's substance still holds — there is no GUI surface tied to the mount
  prototype — but it names the wrong absent thing. Found 2026-08-30.
- **Path-syntax migration.** User-facing surfaces still use `alias:path`; the pinned
  substitution sigil is `@alias` (`:` is reserved for `<handler-path>:<op>`). Prefer
  `@alias` in new docs/examples now to minimize churn when the code change lands.
- **`entitysdk` spin-out.** Stewarded in-tree as the authoritative Go SDK.
  - **Known blocker, found 2026-07-22:** the compute probe cluster in `entitysdk`
    (~9k lines: `exp_compute_*`, `axis1_*`, `continuation_shard_test.go`) **cannot be
    lifted out** as-is. It is one densely interconnected web — a shared Life workload
    fixture, the `boundary` CBOR helper, the shard rigs — with no clean cut, and 5 of the 12
    files reach `export_test.go`'s deliberately test-only engine accessors, which are
    reachable only from *inside* `entitysdk`'s own directory. Moving them would force those
    accessors into the shipped API, which that file explicitly forbids (it exists so app
    code cannot bypass protocol-first access). Untangling it means extracting the Life
    workload fixture first; until then the probes stay put. Attempted and reverted this
    session — the `programs/` extraction is the part that *was* clean.
  - Relatedly: `axis1_{cost,equivalence,tick}_test.go` are arguably API pins for the shipped
    `entitysdk/axis1` subpackage rather than probes, and should travel with the SDK.

**Known waivers / kernel-blocked (not workbench bugs — sibling-impl rule)**
- **Identity-rebootstrap storage leak.** The bounded-rebootstrap pin in `entitysdk/` is
  `t.Skip`'d with its assertion intact. Re-applying an identity bundle leaks ~+1 path /
  +4 entities **per reload** (linear, unbounded). Root cause is the kernel's identity
  *ceremony* re-issuing the local-peer→controller cap + sibling signature with
  ceremony-time-varying material instead of reproducing prior content hashes. Delete the
  Skip to re-arm the regression the moment the kernel's re-apply is idempotent.
- **Subscription slow-consumer head-of-line block.** The producer queue is bounded with
  drop-on-full; the gap is a missing **per-delivery deadline** on the consumer-side
  synchronous `Deliver`, which pins one shard worker when a consumer stalls. Kernel-side fix.
- **Subscription delivery saturation.** 100% delivery below ~2K notifs/sec; a cliff to
  ~47–49% at 5K+/sec, dropped silently. Typical workbench heartbeat is far below saturation.
  Endorsed fix is **parallel delivery workers** — kernel-side after cross-impl alignment.
- **Revision auto-version is O(N)/Put.** Per-Put latency under auto-version grows linearly
  in the existing path count under the prefix, *regardless of trie shape*, because the trie
  is rebuilt from scratch on every Put. Fix is incremental update — a kernel/spec concern.
- **Content-store GC contract.** Path-overwrite accumulates orphaned content entities
  indefinitely; a naive "delete unreferenced + VACUUM" sweep is unsafe because hash
  references are encoded across many typed sites. Awaiting a cross-team reachability/GC
  contract built on the kernel's reverse-hash index; workbench ships nothing until it lands.

**Discovery follow-ups (post-"Nearby Peers" close)**
- N-panel × M-peer scan multiplier — one scan loop per discovery handle, no dedup by handle.
- TXT-pair parsing in the Avalonia bridge duplicates the kernel's private parser (drift risk).
- Scan interval is hardcoded ~5s; no "Scan now" affordance.
- No headless test exercises a *populated* nearby list (would need mDNS in the test container).

**UI / renderer**
- ~~**Handler-browser panel**~~ — **DONE 2026-08-19** (`27874ad`). The console→Avalonia parity
  gap is closed; `console` is no longer ahead on any surface (re-verified by sweep 2026-08-20 —
  console's `execute_console.go` IS its handler browser, and `HandlerBrowserPanel` matches it).
- ~~**`PeerLiveness` has a bridge export and no panel — the ONE remaining renderer gap.**~~ —
  **CLOSED by `8383326`, struck 2026-08-30.** `PeerConnectionsPanel` consumes
  `LivenessOpen`/`LivenessRender`/`LivenessRegisterWake`/`LivenessClose`, and `make reachability`
  reports clean. The row survived five days after its own fix because nothing re-reads a backlog
  against the tree; the sweep that would have caught it existed and was not run against this row.
  Kept below as the record of what the gap was, since D23 was earned on it.
- **(historical, as written)** Found by
  audit 2026-08-20, by two sweeps that each return exactly one name: bridge exports no C# consumes,
  and `workbench/*_model.go` files with no Avalonia panel. `workbench.PeerLivenessModel` is built
  and tested, `avalonia/bridge/main.go:492` exports `PeerLiveness`, and **no C# file references
  it** — `PeerConnectionsPanel` still reads `ConnectionsOpen`, the connection-pool snapshot the
  liveness model exists to replace. Not cosmetic: the pool snapshot **cannot express `suspect`**,
  cannot say why a peer went, and disagrees with the tree whenever a connection is evicted without
  a demotion, so the GUI shows a strictly weaker and occasionally wrong answer while the correct
  one sits one unused export away. The CLI already has it (`peer status`). **This is the third
  instance of one shape** — the name arc, the handler browser, this — where a renderer-neutral
  model was green and no shipped surface reached it; two of the three were found by audit rather
  than by a test, because no test crosses "is there a user-reachable path to this." A fourth earns
  a discipline.
- **Console multi-peer UX** (deferred): peer-picker modal, status bar, `peer create`/`destroy`.
- **Manifest-driven panel registration** (deferred from the multi-peer plan).
- Avalonia drives feature work and may outpace the frozen `console` renderer; console-parity
  is explicitly *not* an obligation.

**SDK ergonomics / compute**
- **SDK ergonomic helpers (compute "S4–S8").** Owed a research-first session.
- **Compute DSL parser.** Deferred; built *on top of* the S4–S8 helpers, only once an
  authoring workflow actually needs one.
- ~~**Wire a real consumer of the `resolve()` seam**~~ — **DONE 2026-08-19** (`ddde4c7`). The
  `name` verb is that consumer, and it needed the substrate turned on in `shellboot` before it
  could be one.

**Hardening / cleanup**
- Revision-recovery diagnostic: hub-spoke fetch-diff recovery with auto-version *off* logs
  an independent transport failure mode; captured as an observation, the test passes.
- Selection-state reader hardening: replace the silent legacy-tolerance path in
  `entitysdk/workspace_state.go` with log-on-violation or reject-on-decode.
- ~~**An ephemeral shell silently re-namespaces a persistent store.**~~ — **FIXED 2026-08-20**
  (`6c72dfa`). `-storage sqlite` with no `-identity` now uses the `default` identity, created on
  first use: a plain nameable one that shows up in `identity ls`, with the store at the
  GUIDE-PERSISTENCE §1.1 path derived from it. Create-if-absent and never overwrite (regenerating
  over an existing keypair would orphan everything under the old peer-id — the same
  re-namespacing, made permanent); the 409-exists race resolves to the winner's keypair rather
  than an error. **Memory storage keeps its ephemeral keypair, with a control arm pinning that** —
  nothing survives the process either way. The old `-storage sqlite` + no-path + no-identity
  refusal is dropped and its pin replaced rather than deleted: it guarded against an orphan store
  and missed the case where the *identity*, not the path, was unknown. Verified through
  `bin/entity-shell` across separate processes, which is the only place the bug existed.
- **Registry restart baseline (core-go, observed 2026-08-19).** A peer with NO registry extension
  and NO identity ceremony still accretes **+2 entities per restart** of the same SQLite DB
  (paths stay flat). Measured as the control arm in
  `entitysdk/registry_bootstrap_cost_test.go`. Same family as the waived identity-rebootstrap
  leak; not ours, and owed a routing packet to core-go.

## `master` — the guardrail is cleared, and here is what cleared it

**History, kept because the reasoning is the useful part.** This section used to read *"do not merge
`dev` to `master` yet"*, on the grounds that the legacy hard-coded panels were the oracle for
`TestMount_LifeMatchesHardCodedModel` and Avalonia registered both the legacy and the generic-host
copy of all three programs. Both halves are now gone, and neither was gone the way the guardrail
predicted:

1. **The oracle was re-pinned** (2026-08-20). `programs/oracle_vectors_test.go` freezes the twelve
   state hashes the two sides agreed on, so the mounted program is gated against a **recorded**
   reference rather than against live legacy code. Strictly stronger than the mutual comparison,
   which cannot see a kernel-side encoding change — both sides shift together and stay green.
   Verified to fail (one character flipped → red at tick 2). The mutual test stays while the legacy
   Go models do; it is no longer what blocks deleting them.
2. **The product regression that was the real blocker had already closed a month earlier, and
   nothing was tracking that.** The generic panel *can* show program-specific status — the `text`
   status port shipped 2026-07-27 (Life `POP nnnn`, Snake `LEN nnn`, Asteroids `SCORE nnnnn`),
   projected in the tree so every renderer shows the byte-identical line, and it needed no `concat`.
   We spent an hour on 2026-08-20 pricing this against the July review instead of against the tree —
   **D20 failing in its usual direction** — and the correction is in
   `reviews/COMPUTE-HOLD-IMPACT-2026-08-20.md` §2 plus an annotation on the July packet itself, so
   the next reader does not pay it again.

**The legacy trio is retired** (2026-08-20): three Avalonia panels, three bridge surfaces (23 of the
118 exports), three registry entries, three smoke targets and three smoke-driver modes — deleted.
`avalonia/bridge/program.go` + `ProgramPanel` drive every program from a descriptor, including the
input ports and the status readout. **The legacy Go models in `programs/` stay** as the mutual
test's oracle: they cost nothing, nothing else consumes them, and keeping a second independent
implementation of Life around is the cheapest oracle we will ever have.

What that removes from the shipped app: nothing a user can do. What it removes from the tree: the
duplicate path, which is what made `dev` a comparison surface instead of a release.

## Open — the compute/programs track (operator-named 2026-08-21, rewritten 2026-08-22)

**Where this arc actually stands, as of the release close-out.** Interactive Life is *working*
— the operator has driven it. Three defects were found and fixed across two sessions, and the
resolution of the original "isn't working" report is recorded here because the two hypotheses it
was split into are now both answered:

| the 2026-08-21 hypothesis | answer |
|---|---|
| **(b)** the d-pad/action bits are not reaching the program | **YES, and twice over** — AP36 (the host sampled its ports only at tick time) and AP37 (`btn.PointerPressed +=` on an Avalonia `Button` never runs). Both fixed, both gated. |
| **(a)** the operator expected to *click cells* | **Still true, still unimplemented** — see the pointer-input row below. The toggle interface is the d-pad + Toggle button, which the operator has now used and called "clunky, but it works." |

Plus AP38 this session (Regen was a translation, §0a). Nothing in the list below is started;
**the operator's call is to resume after the release.**

### Backlog — post-release

> **This is the canonical backlog. Rows carry stable `PR-n` ids** so the charter, `AGENTS.md`
> and commit messages can cite one without restating it (one canonical home per fact).
> **PR-A…PR-C are the operator-named order and keep their priority**; PR-1…PR-3 were added
> 2026-08-23 and are *not* ranked against them — sequencing them is an operator call, not a
> thing a session decides for itself.

**PR-1 — a SQLite-backed query index (the shape fix behind AP39).** **This is the big one.**
2026-08-23 landed the *correctness* fix — `IndexMaintainer.Rebuild` at peer open — and that is
all it is. The three query indexes remain **in-memory**, so every peer open pays a full rebuild:
`li.List("")` over every tree-bound entity, then a content-store `Get` and a CBOR decode each.
Measured (`entitysdk/query_index_rebuild_bench_test.go`, `-race` off per the perf rule):

| entities | peer open | size-dependent component |
|---|---|---|
| 100 | 15.7 ms | — (baseline) |
| 1,000 | 23.1 ms | ~8 µs/entity |
| 10,000 | 102.5 ms | ~8.7 µs/entity |

Linear, ~8.7 µs/entity — **~0.9 s at 100k, ~9 s at 1M**. It is a startup tax that grows with the
store forever, and it is paid to rebuild state the database was already holding.

**The operator's objection is the correct one and is recorded verbatim:** *"the whole point of
SQLite is that it does queries easier than anything else."* Exactly — and today none of the query
work reaches SQL. `system/query` answers from three Go maps that a SQL `WHERE` clause could serve
directly, against a `SqliteStore` that is already open. The seam is deliberate and documented:
`SqliteStore.DB()` exists *"so co-located extensions can create their own tables in the same
database"*, which is precisely this. **Prove the negative before scoping** (done, 2026-08-23):
`MemoryTypeIndex` / `MemoryReverseHashIndex` / `MemoryPathLinkIndex` in `ext/query/index.go` are
the **only** implementations of the three `core/store` interfaces in `entity-core-go` — no
SQL-backed variant exists, and none has been added since the v0.8.0 public release
(`git log --since` on `ext/query/` is 5 commits, all unrelated). So this is **build, not adopt** —
and note it is a *kernel* extension shape, which makes it a design conversation with core-go
before it is a diff here, not a thing to land unilaterally in `entitysdk`.
*(Loose end found while scoping: core-go's own comment at `cmd/entity-peer/main.go` cites
`DESIGN-SQLITE-PERSISTENCE.md §4.3`, and that file exists nowhere in their tree. Worth asking for
when the conversation opens — it likely holds the reasoning this row needs.)*

**PR-2 — restart-cover every read path a store serves** (the generalization AP39 earned). The
gap was never a missing restart test; it was that `TestStorage_Sqlite_LargeCorpusSurvivesRestart`
reads the reopened peer only through `Get`/`List` — the persistent location index — and never
through `system/query`. Enumerate the read paths (tree/location, query, revision log, subscription
tracking, published-root seq, discovery) and confirm each survives a reopen. Cheap, and it is how
we find the *next* one of these instead of shipping it.

**PR-3 — Axis-1's §5.2 constraint reader** (§0b). **Now stands alone** — PR-B, the other half of
the pairing, is done (2026-08-25). Still open, still unreached by any binary, and the position
model that closed PR-B does not touch it: this is the constraint *reader*, not the evaluator's
error semantics. **Note for whoever takes it:** PR-B's cause was named correctly the day it was
found and then talked out of existence — so treat §0b's diagnosis, which names
`0e34e3e` and both replacement functions, as the strong starting point it is, and require any
refutation to reproduce the failing shape.

**PR-5 — 43 unresolvable commit pins in the canonical docs** ([ADR-0012] Amendment 1, landed
2026-08-23 with the ADR injection). Published commits are authored fresh at the release boundary
([ADR-0027]), so **public `master` is a different history from `dev`** and an internal SHA in a
published doc resolves to nothing *by construction* — it was never valid, rather than broken by
the release. Measured, not estimated:

```
python3 ../entity-system-arch-tools/spec-tool/cli.py pins --root .
  21 declared doc(s), 48 short-SHA citations — 43 unreachable by a reader of `master`
  DISCIPLINE-CHARTER.md 17 · MODEL-AVALONIA-RUNTIME.md 14 · DIAGNOSTIC-DIRECTION.md 4 ·
  GUIDE-AVALONIA-PANEL-PATTERNS.md 3 · DEPLOYMENT-DIRECTION.md 2 · TESTING-STRATEGY.md 1 ·
  LOGGING-CONVENTIONS.md 1 · CROSS-IMPL-HELPER-REFERENCE.md 1
```

Every one is an anti-pattern row or a model note citing the commit that earned it, so the repair
is per-citation judgement (cite by content — *"the commit that fixed X"* — or a release tag, or a
content digest), not a sed. **Run the checker before any release cut** and after adding any commit
citation to a canonical doc; it is reader-mode by default, `--gate` to enforce. Content hashes
(64 hex) are never flagged — they are the fix, not the defect.

*Already banked, at zero cost:* dropping `docs/STATUS.md` from `CANONICAL-DOCS.toml` per
[ADR-0031] took **67 more unique short-SHAs** out of the published surface — this file is the
densest SHA carrier in the repo, and it was the one declared exception. It is now internal, which
is also why it can stay frank.

**PR-4 — name the Avalonia headless flake.** 1 failure in 8 runs, still unnamed. The *method* is
fixed (runs are logged now, not tailed), so this needs runs, not a new approach — and three clean
runs in a row do not close it (~0.51 chance of missing a 1-in-5 fault). Next occurrence carries a
name; until then the row stays open and the suite is quoted as **"74/74, one unexplained failure
in 8 runs"** rather than "74/74".

1. **PR-A. Pointer input for the generic host (click-a-cell).** The capability gap, not a Life bug:
   neither input shape we have (`key-set`, a 64-bit mask; `direction`, a 0..3 enum) can carry a
   coordinate. `PROPOSAL-GENERIC-HOST-POINTER-INPUT-DEVICE-2026-07-27.md` is **delivered** and
   arch has it as `COHORT-OPEN-ITEMS` §1d row **W-1, owner arch**, sequenced with T5's resume by
   stated decision. This is what upgrades the toggle interface from "clunky but works" to direct
   manipulation, and it is the honest blocker on shipping interactive Life anywhere as a
   showcase.
2. ~~**PR-B. Three UNDIAGNOSED divergences between the two compute engines.**~~ **DONE
   2026-08-25** — diagnosed and fixed, see §0A. All three were one shape:
   `length(map(arr, λe. e + <out-of-range index>))`, where `map`'s output element is a
   **CONTAINED** position and Axis-1 propagated instead of containing. The cause was
   contained-error semantics after all; the 2026-08-24 refutation was the error, not the
   original diagnosis. Fixed as a position model (`axis1/contain.go`) rather than a patch, which
   also caught a filter predicate that **kept** elements whose predicate had failed. Gate:
   `TestAxis1Equivalence_ContainedErrorPositions`, verified failing pre-fix. Sweep 300/300.
   **§5.2 (PR-3) is still open and is now the whole of that pairing** — see below.
3. **PR-C. The `programs/` ↔ `concat` catch-up.** `programs/` routes around `concat` in four places
   and **Axis-1 implements none of the four v3.24 primitives** against a 350-vector corpus.
   Nothing is waiting on anybody for this (core-go shipped them at `eb80750`).
   **Sized 2026-08-25 (§0c): exactly 11 corpus vectors** — `cv1-group-by-shape`, `cv2-assoc-oob-*`,
   `cv3-range-*`, `cv4a/4b-assoc-*`, `cv5-concat-error-transparent`, `cv6-group-by-error-key`,
   `cv7a/7b/7c-*-collection-error-shortcircuit`. They are 11 of the 28 vectors currently voiding
   AE-5, so PR-C is now on the admission's critical path rather than a nice-to-have.
4. **PR-D. Wire arch's differential compute corpus into a `make` target** (§0c). The gate that
   would have caught PR-B's defect on day one already exists and **skips by default**; the runner
   is in `AGENTS.md`. Not in `test-native` — it needs a buildable sibling, same rule as
   `crossimpl-go`. **Highest value-per-hour row on this list.**
5. **PR-E. Decode `compute/error` as a value leaf** (§0c). One `case` arm in `axis1/decode.go`;
   measured at 12 recovered vectors, 28 → 16 deopts, 0 divergences. Wants its own diff.
6. **Waiting on arch — AE-6 vs a declared scope fence.** Routed as
   `reviews/AXIS1-ADMISSION-LAPSED-2026-08-25.md` §3, with the "should this engine live here at
   all" question in §4. Not blocking: Axis-1 ships in no binary.

### Coordination with `entity-browser-rust` — where we left it

- **They serve `app/life`, the old non-interactive Life.** That is the phase-1 falsification
  fixture — pure Life, deliberately boring, correct as such. Publishing `app/life-edit` to them
  rides on the CDN corridor, which works; but it **inherits row 1 above**, because shipping an
  interactive program to a renderer that cannot deliver the input is half a feature (D23).
  The AP38 fix does not change this — a translated soup and a good one are equally unreachable
  without a controller.
- **The sub-tick input finding is routed and mutual.** `reviews/GENERIC-HOST-SUBTICK-INPUT-2026-08-21.md`
  (committed at `7729cb5`, pushed) — they found AP36 independently, in a different language, and
  fixed it at a **different layer** (`MomentaryGuard`: delay the release by one tick period)
  where we fixed it in the host (an input queue). Routed as a **design result, not a defect
  report**: the open question is whether sub-tick input belongs in the driver or the host, and
  the convention that specifies `rate_hint` says nothing about the interval between ticks —
  which is the hole both implementations fell into. **Blocked on us: nothing. No reply is
  owed to us either** — this is a question for the convention, not a bug in either tree.
- **The `.list` artifact format** and the **fixture re-cut** are the two other live browser-rust
  threads; both are in "Waiting on" below, both cost us nothing today.
- **Not attempted this session, by decision:** no further browser-rust coordination before the
  release. Parity work resumes next week per the operator, then the more complex features.

### The instrument note worth keeping

Three defects in this arc (AP36, AP37, AP38) shipped through green suites, and each was found by
a different thing: AP36 and AP37 by **the first test that crossed the seam** with real input,
AP38 by **an operator playing with the shipped program**. None was found by the layer's own
tests, all of which were complete and correct about their own layer. That is D10 stated as a
measurement rather than a slogan — and for AP38 specifically, the missing instrument was not
coverage but a **strong enough property**: the test asserted the board *changed* when what it
needed to assert was that the boards were *independent*.

## Waiting on

> **Reconciled 2026-09-09, and the shape of this section changed.** It had not been touched since
> 2026-08-24 while the outbound set roughly doubled, so it described a different month's work. The
> full per-counterpart index — every ask, its packet, and whether delivery has been *established*
> rather than assumed — now lives in three trackers at `docs/status/TRACKER-<counterpart-repo>.md`,
> the ecosystem-wide convention as of 2026-09-09, in a directory that is not a published surface.
> What stays here is the part a reader of *this* project needs: what is undecided elsewhere that
> we care about, and what it costs us meanwhile.
>
> **The honest summary is one line: twenty-four packets written, none confirmed delivered.**
> Nothing in that set blocks us — every row has a workbench-side answer that works today — and
> the correct response is to make them handable, not to write more of them.
>
> The three standing counterparts, and what each is for: the **kernel/substrate seat**, for
> defects in an implementation of something already decided; the **specification seat**, for
> anything whose answer is a sentence in a document; the **other application-tier seat**, for
> shared shapes and interop, where neither of us can rule anything and the exchange is running
> each other's bytes.

- **⚠ meta / release team — the release NUMBER for the next cut.** ⚠ **This row said we were
  public at `v0.8.0`, and that went false without anybody noticing: the published branch carries
  a written-out `## [0.9.0]` section.** Checked against the **published remote**, which is the
  only thing that answers it — a local `master` drifts from what is actually published and gives
  the wrong answer confidently. The row is corrected rather than deleted, because *how* it went
  stale is the reusable part: it described a state, the state changed elsewhere, and nothing in
  this tree can fail on a sentence. What is actually open is the **next** number. Our
  `[Unreleased]` section is written and now covers the whole span since 0.9.0 (sharing, the file
  explorer, publishing, the live road, the feed, conflicts, catch-up), so the remaining act is a
  heading edit and it is **deliberately not guessed** — core-go spent a commit on exactly this
  failure mode (`13a42ea`, *"stop restating the release version in status prose — it went false
  on py/rust the moment they cut"*). **Blocked on us: nothing.**
- **⚠ operator / arch (ledger M-4) — [ADR-0030]'s disposition.** It is **retracted** and its own
  text still reads `Status: Accepted`; arch reverted their copy, we kept ours and annotated it
  (§0A). We are stable either way. The only thing we are asking is that it not sit indefinitely
  in the state where the text asserts a decision that was withdrawn — that state is precisely
  what the retraction is about. **Blocked on us: nothing.**
- **arch — the generic host's third input device (pointer/`click`). DELIVERED; sequenced, not
  blocked.** `PROPOSAL-GENERIC-HOST-POINTER-INPUT-DEVICE-2026-07-27.md` is AGENTS.md's worked
  AP28 example — for 24 days this row read "blocked on arch" while the packet had **no filename
  hit and no subject hit** in either sibling tree. Pushing `dev` was the delivery; arch read it
  at `98ff6de` and opened `COHORT-OPEN-ITEMS` §1d row **W-1, owner arch**, deferred by stated
  decision alongside T5's resume. **The content is undecided, so it stays on this list** — but
  the reason is arch's sequencing, not our non-delivery, and the difference is the whole of
  AP28. It is row 1 of the post-release programs backlog above: the answer to hypothesis (a),
  and what turns interactive Life's toggle interface into direct manipulation. *(Duplicate of
  the struck-through row further down; kept here because this is where a reader looks first.)*

> **"Waiting on" means the content is undecided.** A ruling that has not been folded into spec
> text is **not** on this list — that is an editorial queue item on the authoring repo's board,
> and we build against the ruling and route what it teaches (operator ruling 2026-08-19, see §3
> piece 4 and `AGENTS.md`). Putting a decided-but-unfolded surface here is how a self-inflicted
> stop gets laundered into a dependency.

- ~~**⚠ arch — `transports` on a §3 registry binding: rule the sentence.**~~ — **CLOSED
  2026-09-09, and it had been ruled since 2026-08-21.** The field carries **hashes**, as a MUST;
  our reading was the normative one. We could not see the ruling — it landed between our filing
  and the reply — and the cohort had already converged on it. **Confirmed at the live federation,
  not only at source:** the chain now reports the transport as carried *by hash* in the binding,
  on a binding reissued three days after the ruling. Nothing was ever blocked on us. §24.1.
- **⚠ arch — §6a.3a's recommended publishing prefix is unimplementable** (same packet, §6).
  A binding's signature lives at `system/signature/{hex}`, outside every `system/registry/…`
  prefix, so a registry that follows the SHOULD **enumerates fine and resolves nothing**.
  One added sentence fixes it. Measured, with a control:
  `publish/registry_roundtrip_test.go::TestNarrowRegistryPrefixOmitsTheSignatures`.
  **Blocked on us: nothing** — we publish at `system/` and say why.
- **browser-rust — the `.list` artifact's format** (same packet, §7). The spec names a
  `system/tree/listing` entity; their static emitter writes newline text. Costs us nothing now
  (we read both), and it is worth knowing which of us is wrong. **Blocked on us: nothing.**
- **core-go — export `normalizeName`** (same packet, §5). Declinable; a doc sentence naming it
  as a cross-impl contract would also do. **Blocked on us: nothing** — transcribed and pinned.
- ~~**⚠ arch / operator — the compute hold needs a call**~~ — **ANSWERED 2026-08-20**
  (`ROUTING-2026-08-20-e` §1, §11 here). **The hold stands and was delivered**: compute is deferred
  by operator decision and **resumes after the release**. It is off this list because the content is
  decided, not because it is done — the five levers stay arch's and stay on their board. Lever 1's
  ruling is **folded** (`EXTENSION-COMPUTE` 3.24) and the builtin is implemented nowhere, ours
  included; nothing here is waiting on it. **That last clause is now wrong and is corrected in
  §12a — core-go shipped all four v3.24 primitives at `eb80750`, after the commit we measured.**
- **arch (post-release, not waiting on us or blocking us):** the subtree-state descriptor/host
  convention (the successor rung, and the same rung as Doom-realtime);
  `PROPOSAL-CONTINUATION-STANDING-MODEL` §4 (the continuation join-failure policy); whether a
  scan/up-sweep orchestration is in scope; **§4's fairness clause, which the 3.24 fold left
  explicitly not ruled** — adopting `concat` does not bless an in-compute sharded step.
- ~~**⚠ OURS, not theirs — deliver `PROPOSAL-GENERIC-HOST-POINTER-INPUT-DEVICE-2026-07-27.md`**~~
  — **DELIVERED, and arch has it.** Pushing `dev` was the delivery; arch read it at `98ff6de` and
  opened `COHORT-OPEN-ITEMS` §1d row **W-1, owner arch** (`ROUTING-2026-08-20-l` §1/§3, §12a here).
  Not ruled this session by **stated decision** — sequenced with T5's resume — which is the kind of
  deferral a counterpart can confirm. **Blocked on us: nothing.**
- ~~**arch — the `rendezvous` fold (R-10)**~~ — **FOLDED 2026-08-20.** `EXTENSION-DISCOVERY`
  1.0 → 1.1 carries all four of our build findings in full (`-l` §4). `entitysdk/rendezvous.go`
  now stands on landed spec; nothing owed either way.
- **arch — R-9, the `hints` round-trip:** answered from our side in §12a (refuted for this seat,
  their second observation confirmed). Listed only so the reply is greppable; nothing is owed to us.
- **Lever 1 is NOT waiting on anybody, corrected 2026-08-20.** `concat` and the other three v3.24
  primitives are implemented in core-go (`ext/compute/builtins_v324.go`, `eb80750`); §11's
  "implemented nowhere" was measured at an ancestor commit. Two rows this opens, both post-release:
  `programs/` still routes around `concat` in four places, and **Axis-1 implements none of the four**
  against a 350-vector corpus.
- **`entity-core-go` kernel:** published + tagged vanity module path; an idempotent
  identity-ceremony re-apply; a per-delivery deadline + parallel delivery workers;
  incremental revision-trie update.
- **Cross-team:** the content-store GC / reachability contract.
- **`entity-browser-rust`:** the follow vocabulary — one record + verb, `strategy` with a
  value that does not assume ordered delivery (share-review §3.2). Step 4 of the share arc,
  and step 5 waits on it by construction. **Also: the fixture run.** `DirFetcher::manifest()`
  reads `manifest_url_prefix` per `-p` §3, then runs our re-cut fixture (`d940ce0`, byte-stable)
  and tells us where it actually stops.
- **arch:** `REG-DISPATCH-CATCHALL-LOCAL-1` re-keyed or explicitly scoped — §11.1's vector still
  bans remoteness while §4.1 step 2 bans name transmission, so it contradicts §4.1a row 6's
  `peer-issued` (§6b). Blocks nobody today; misleads everybody later.
- ~~**arch:** `APP-CONVENTION-SHARE` authored~~ — **DELIVERED 2026-08-18.**
  `specs/applications/APP-CONVENTION-SHARE.md` v0.1 exists (arch `bb86cd1`, routed as
  `ROUTING-2026-08-18-i` §4). **`app/share/*` is no longer blocked by arch.** Four constraints to
  build against: a share is a titled grant (`resources` = what is shared, audience = the minted
  token's `grantee`); **`peers` MUST be omitted** (populating it 403s every cross-peer presentation
  and still passes local testing, because our single-identity tests collapse root/grantee/granter);
  type tags are `app/share/*` (the index key for cross-peer aggregation — a tag under our own
  prefix breaks browser↔go interop); mirrors write the publisher's paths verbatim. Withdrawal is
  asymmetric — `request`-minted tokens are **not** recallable and a UI **MUST NOT** imply otherwise.
  Not ratifiable (zero vectors); `SHARE-4` and `SHARE-6` are the two owed vectors that fail loudly
  on the intuitive-but-wrong reading, and are where we start.
  **Step 5 is now blocked only on the core-go kernel defect (§6), not on arch.**
