# entity-workbench-go

Read **AGENTS-STANDARD.md** first. This file adds entity-workbench-go specifics.

> **Reference an ADR; do not copy one.** `[ADR-NNNN]` unqualified means the *ecosystem* ADR —
> read it at its source rather than keeping a copy here, because a copy goes stale silently.
> Cite this repo's own as `[<repo>-ADR-NNNN]`. Two that change daily work: **[ADR-0031]**
> (`docs/status/` publishes nothing — so it is written for the next session, with no scrub
> obligation) and **[ADR-0012] Am. 1** (in a *canonical* doc, cite by content or a release tag,
> never a branch SHA, because published history is authored fresh at the release boundary and an
> internal SHA resolves to nothing for a reader).
>
> **`git add -A` is not safe in this tree.** `AGENTS-STANDARD.md` and `METHODOLOGY.md` are shared
> files maintained outside this repo and updated in place, so they can change underneath you
> mid-session — and a blanket `add -A` then sweeps thousands of lines you did not write into a
> commit about something else. It has happened here. **Stage explicit paths, or read
> `git status` before staging.** The same habit generalizes: when a tool or a teammate tells you
> what landed in your tree, measure it in your tree before repeating the number.

## Overview

Application + performance layer for the entity ecosystem — **not** a conformance
implementation. Ships **entity-shell** (the primary CLI / leading edge of feature
development) and an **Avalonia** desktop frontend, plus a frozen **console** (tview)
renderer, all over the Go workbench stack / V7 protocol / `entity-core-go` store. The
in-tree `entitysdk/` is the **de facto reference SDK** (stewarded here until it spins out
— treat it as the authoritative Go SDK impl, not workbench-internal glue).

## How we work here — Disciplines & Doctrines · tier **FULL**

This repo runs the entity-OS methodology at the **Full** tier for the Avalonia/.NET UI runtime
— held where conformance alone can't reach a GUI. The framework is `METHODOLOGY.md` (maintained
upstream, identical in every repo); the charter below carries the local grounding, and **this repo is one of
the worked instances the framework was reconciled from** — D1–D11 there are inherited verbatim,
D12–D27 here are ours, earned on the eight crash-hunt commits, two feedback episodes, the
2026-08-18 publisher/connectivity pair, the v1.13 adoption trio, the 2026-08-19 cross-impl
consume run, the 2026-08-20 reachability audit, and the 2026-08-21 crash hunt that found a
month-old fatal bug the moment an instrument could reach it.
- **Disciplines** (invariants — the *what*): `docs/architecture/DISCIPLINE-CHARTER.md` —
  D1–D27, the ten review questions, the anti-pattern catalog AP1–AP113, and the promotion
  criteria (§5) that the ecosystem ladder generalizes.
- **Substrate model** (ground truth): `docs/architecture/MODEL-AVALONIA-RUNTIME.md` — what
  the Avalonia/.NET/Skia/X11 runtime actually does (stack diagram, lifecycle matrix, the
  seven-boundary map — **Boundary G is the POSIX signal layer, added 2026-08-21** — and the
  invariants). Read before any layout/lifetime/render/**crash** work.
- **Recipes & conventions** (patterns that respect the rules):
  `docs/architecture/GUIDE-AVALONIA-PANEL-PATTERNS.md` (P0–P7, new panels lift these) +
  `TESTING-STRATEGY.md` + `LOGGING-CONVENTIONS.md`.

Session start: read the charter → the substrate model → anything newer than the point
`docs/STATUS.md` records as last read, and update that marker. A specification change that moves a
*table*, a *default*, or a **MUST** is read the same session it is found, **before feature work** —
it has twice been the case here that a landed spec change sat unread while we shipped past it, once
leaving a validator rejecting a configuration that had become legal.

Three rules about work that crosses a repo boundary, each earned the hard way:

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
- **A new outbound packet is `docs/status/ROUTING-<date>-<letter>-<recipient>-<slug>.md`**, opening
  with an addressee block whose three fields are each **on their own line** — `**To:**` naming
  **repositories** (never a person or a nickname; a brace list is fine), `**From:**`, `**cc:**`.
  `cc:` means *you are not on the hook*: if it needs acting on, it goes in `To:`. Never reuse a
  letter within a day. This is `AGENTS-STANDARD`'s existing convention, unadopted here until
  2026-09-09; **the packets already in `docs/architecture/reviews/` stay exactly where they are**
  — do not move or rename history, and do not re-file anything to flush a backlog.
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

## Setup / environment

- **Go pinned to 1.25.1** (forced by core-go's `ext/go.mod` `go 1.25.0`) — the Makefile
  pins it. Per AGENTS-STANDARD, never set `GOTOOLCHAIN=` inline.
- **Sibling `../entity-core-go/` is required.** Every `go.mod` uses `replace` directives
  resolving to `../../entity-core-go/core` and `../../entity-core-go/ext`; without the
  sibling, `go build` fails at module resolution. README documents the layout.

## Build & test

`make` is the build interface (see AGENTS-STANDARD). Full target catalogue is in the
`Makefile` header.

- **`make test-each` is the target to reach for when you want to know the state of the
  tree.** It runs all ten suites **to completion** regardless of failures, prints a pass/fail
  table with per-suite timings, leaves logs in `.test-logs/`, and exits non-zero if any
  failed. ~12 min.
- `make test` — full sweep (`-race -count=1`); `entitysdk`/`shellcmd` slowest; pin tests
  live in `entitysdk`.
- **AP15 is about fail-fast REPORTING, not about `make`** — the same defect lives inside any
  test that loops. `TestAxis1Equivalence_Differential` swept 300 cases and `t.Fatalf`'d on the
  first divergence, so the tree reported **one** for three days when there were **three**. The
  tell is `Fatalf`/`break`/`return` inside a range over cases, corpora or fixtures: **the count
  it yields is a lower bound.** Collect, then report. And if you waive a known failure, waive a
  *signature* and pin the instance set, so a fourth instance is not silently absorbed and the
  waiver fails when the defect goes away.
- **A probe that does not reproduce the failing shape refutes nothing** (AP43). The same three
  divergences had a correct diagnosis, which we then retracted on two probes that ran cleanly
  and were about the wrong position — an out-of-range index in a *consumed* position, where both
  engines were already correct, when the defect lived only at a *closure-result* position. The
  cause went back to "unknown" in a published CHANGELOG for a day. **Before a refutation retires
  an explanation, show the probe reaches the mechanism**: name the position/branch the hypothesis
  predicts will fail, and demonstrate a failing run before the fix rather than only a passing one
  after. *"I measured it and the explanation is dead"* is a far more expensive claim than
  *"nobody has measured this"*, and it travels faster because it sounds more rigorous.
- **When two implementations disagree on GENERATED input, the generated input is the evidence** —
  reach for this first, not after a round of hypotheses. Regenerate at the fixed seed, dump the
  IR of each diverging case, then **evaluate every subnode on both engines, children first, and
  print the deepest disagreement.** One throwaway test file converted three days of "cause
  unknown" into a named position in a spec, in one run. The scaffolding is deliberately not kept
  in the tree; it is twenty minutes to rewrite and it is wrong to maintain a debugger as a test.
- **`make test` STOPS at the first failing package, and `-k` does not cross the container**
  (`test` = `IN_CONTAINER make test-native`, so `make -k test` still aborts at the first
  sub-target). **Never quote a failure count taken from a red `make test` run** — it is a
  lower bound covering one package. This is **AP15** — we routed "shell and shellboot are
  green" to another repo when they had 26 failures between them, and reported `entitysdk` as
  5 when it was 171. `make test-each` exists precisely so the correct thing is also the easy
  thing; it replaces the hand-typed `for t in sdk inspect …; do make test-$t; done` loop this
  file used to make you remember.
- **NOTHING ELSE MAY RUN A PODMAN TARGET WHILE A SWEEP IS RUNNING — not `make -C avalonia
  test`, and not `lint`, `textual`, `build` or any other `:Z` target either.** The rule below
  used to name the avalonia suite only, and on 2026-09-07 a session read that as "the avalonia
  suite is the collider", ran `make textual` and `make lint` during a `test-each`, and got a
  sweep reporting **9 of 10 suites `FAIL 0s` with no log files at all** — a different
  presentation from the one documented below, same cause, and it reads as a catastrophic
  regression rather than as an environment fault. **The collider is the second `:Z` mount, not
  the target that happens to make it.** The tell that it is not your code: a suite that
  "failed" in **0s**, or a missing `.test-logs/<suite>.log`. Start a sweep, then keep your
  hands off podman until it prints its table.
- **Never run `make test-each` and `make -C avalonia test` at the same time.** Both bind-mount
  this tree into podman with `:Z` (private SELinux relabel), and the second relabel revokes the
  first container's access mid-run: every suite after the first reports as failed with
  `.test-logs/<suite>.log: Permission denied` while the log itself says `ok`. **A sweep that
  reports red for a reason outside the tree is worse than no sweep**, and this one is
  indistinguishable from a real failure at a glance. Run them serially. (Measured 2026-08-21.)
- **Some failures are load-dependent** — `TestE2E_Bidirectional_BurstWrites_NoFS` fails under
  full-suite load and passes when run alone, so a targeted re-run is **not** evidence a
  `test-each` failure was spurious. Check `docs/STATUS.md`'s green line before
  attributing a red suite to your own diff.
  **`TestStage3_F9_SelfLoop_SinglePeer` is the second one** (added 2026-09-11), and it is the
  instructive one because *the failing arm is a real assertion about a real invariant*: it reports
  `Δentities=11 Δbindings=1 … RUNAWAY LOOP. F9 has regressed`, which reads as a serious regression
  rather than as a flake, and its own message sends you to two named source lines that are fine.
  **`TestM3Baseline_ConcurrentEditLeavesBothWritesOnTheChain` is the third** (added 2026-09-15), and
  it is the one whose failure text is most likely to send you to a specification. It reports
  *"the receiver's chain has 2 transitions; §1.1a requires both writes to be recorded at DISTINCT
  chain positions"* — a named normative clause, apparently violated. What is measured: it failed in
  one `test-each` and **passed in a second full-suite run at the same commit**, `-count=6` alone is
  6/6, and the whole `shellboot` suite alone is green. Two full-suite runs at one commit disagreeing
  is this tree's documented discriminator, so treat it as load-dependent **and note that the
  mechanism is NOT established** — what we know is the disagreement, not the cause. The likely
  shape is already written down two bullets from here: *a fact established by reading a mutable
  structure once is a fact about that instant*, and this test reads the chain immediately after the
  delivery it is about, so a delivery still in flight reads as a missing chain position. **Do not
  quote it as a §1.1a finding without a second full-suite run.**
  **`-count=12` reproduces it on a clean tree**, which is the check that settles it in a minute —
  and note that *passing alone* did not, because one run of a 1-in-12 flake is not a measurement.
  **The procedure that actually converged: loop the suspect test with `-count=N` at your HEAD AND
  at `HEAD~1`, and compare the failure SIGNATURE, not the pass/fail.** A single full-suite run at
  each is one observation each way and settles nothing; two full-suite runs at the same commit
  disagreed with each other here. **So a red count taken from one run is a lower bound even when
  the suite completed** — that is AP15 with no `Fatalf` in sight, and both numbers in a scorecard
  row got quoted as facts before a second run contradicted them.
- **`make lint` is `go vet` only — it does not check formatting.** Nothing gates gofmt, so
  drift accumulates silently (60 files at the 2026-08-18 audit). Run `make fmt` as its own
  commit, never folded into a feature diff.
- **`make textual` refuses a raw C0 control byte in a tracked source** (AP51). Seventeen of them
  — NUL, SOH, STX typed *literally* into C# string literals as separators — made
  `BrowserPanel.cs` **binary to git**: `Bin 36537 -> 64640 bytes`, no diff, no blame, no merge,
  on the 1513-line centre of that session's work. Nothing about the program was wrong, so every
  test stayed green. **Do not reach for `grep` here**: GNU grep matches on NUL-terminated C
  strings, so a NUL cannot appear in a pattern and `grep -P '\x00'` reports a file clean that
  `od -c` shows it sitting in — a false negative shaped like a pass. The fix is always escapes
  (`"\u0000"`), never a `.gitattributes` binary marker, which keeps the bytes and discards the
  diff. The tell is a source file `file(1)` calls `data`.
- `make test ARGS="-run X -v"` — single test / forwarded flags (`ARGS=` is the only
  passthrough syntax `make` accepts).
- `make test-sdk` / `test-shell` / `test-shellcmd` / `test-workbench` — per-package suites.
- **`test-publish` and `test-fetch` are the CDN corridor's two ends**, and both joined
  `test-native` on 2026-08-19. Before that `fetch` had no target and `publish` was outside the
  sweep, which is how the two halves drifted four ways apart with every suite green (AP21/D22).
- **`make crossimpl-go` is the LIVE cross-impl consume leg** (`scripts/crossimpl-go.sh`) — it
  stands `entity-core-go`'s federation publisher up in its own container on a podman bridge
  (their script, unmodified) and drives our verifying consumer at it from a second container on
  that bridge: manifest → signature → CHAMP trie walk → leaves. **Deliberately outside
  `test-native`** — it needs podman and a buildable sibling checkout, and a sweep target that
  can go red for a neighbour's reasons teaches people to ignore the sweep. First green
  2026-08-20. What a green run claims (and, more importantly, what it does not) is in the
  script header; do not restate it looser anywhere else.
- **`make consume-live` drives the naming chain at the LIVE public federation**
  (`scripts/consume-live.sh`) — enumerate a registry by walking its signed root, resolve every
  name through §6a.4 in full, follow one through to verified page bytes on a second domain
  under a different key. **Outside `test-native`** for `crossimpl-go`'s reason, one step
  stronger: it reaches the public internet. The registry pin is the one fact supplied out of
  band and is **not** read from the origin — that is the whole point of a pin. What a green run
  claims (and does not) is in the script header; do not restate it looser. First green
  2026-08-30, and it is the only gate in this repo that can see AP44's shape at all.
- **`make -C avalonia smoke-xvfb-click` is the real-input gate under the X11 platform**, and
  `make -C avalonia crash-hunt` is its unattended form (sweeps seeds, stops at the first
  crash, prints the replay command). Most drivers in this repo call the model method
  *under* the control — `SiteViewPanel.NavigateForTests`, `HandlerBrowserModel`, the window
  driver — so **none of them can execute input dispatch, hit-testing, focus transfer, or any
  handler that runs before a panel's own code.** That blind spot hid a fatal crash for a
  month across four "negative" repro attempts (STATUS, 2026-08-21). Clicks are seeded and
  every coordinate is logged, so a crashing run replays: `CLICK_SEED=n`.
  **It does real DRAGS as of 2026-09-01** (`DRAG_PCT`, default 40, half of them near-vertical
  because that is the scrollbar-thumb shape). Before that it pressed and released at one point,
  so pointer capture, a motion stream to a captured element, and a scrollbar thumb were all
  outside its reach — and the 2026-09-01 SIGSEGV happened on the render thread during a
  scrollbar drag, i.e. in the one region neither this harness nor the headless drag tests
  covered. `DRAG_PCT=0` is the control arm.
- **`make gui-drive` presses a NAMED button in the real app and reads the window back** — the
  rung between the click fuzz (real input, random coordinates, no assertions) and the headless
  suite (assertions, no X11, no render thread). `scripts/gui-drive.sh` runs the shipped binary
  under Xvfb + openbox and drives it over a socket; `avalonia/frontend/UiDriver.cs` is the
  automation server, enabled only by `WB_UI_DRIVER` and announced on stderr when it is.
  **There is no Selenium for this stack and it is measured, not assumed**: Avalonia 11.2.3's X11
  backend ships no AT-SPI bridge (`strings Avalonia.X11.dll | grep -ci atspi` = 0), so the
  accessibility tree `dogtail`/`pyatspi` would drive does not exist; Appium and FlaUI are
  Windows-only. **The driver RESOLVES and READS; it does not synthesise input** — it turns
  *"the Share button"* into a screen rectangle and **xdotool** does the pressing, through the same
  X server the fuzz uses, because a fabricated pointer event skips the dispatch/hit-test/capture
  layers where the 2026-08-21 SIGSEGV landed. A missing xdotool is an **error**, never a fallback
  to invoking the handler. Address controls by `AutomationProperties.AutomationId` (real
  accessibility metadata, not a test channel); text selectors work without them, so adoption is
  incremental. **An ambiguous selector is refused rather than resolved to the first match** —
  `Button:Share` matches both "Share a folder…" and "Share", and guessing would make a scenario's
  meaning depend on visual-tree order. Two rules the spike earned the hard way: a subtree-text
  read must **skip what is not effectively visible**, or a prose assertion passes on text nobody
  can see (it did, on a section the Sync panel hides until an offer arrives); and a failed
  redirection on `exec` is **fatal** to a non-interactive shell, so `if exec 3<>/dev/tcp/…` cannot
  be retried in a loop — probe in a subshell, and believe the port only when a `ping` comes back,
  because rootless podman accepts a connection on a published port before anything is listening.
  **What a green run does NOT claim: two peers, a network, permissions, a restart, or anything
  about sharing.** That is `scripts/twopeer-gui.sh`, which is owed and does not exist.
- **`make twopeer-gui` is the SHARING GATE FOR THE BINARY THE OPERATOR OPENS** — `twopeer-sync`
  aimed at the GUI. Two real Avalonia apps, two containers, one real TCP network, real grants (no
  `--open-access`; a wildcard deletes the permission stage while everything downstream stays green,
  AP63). The address is **typed** into the Connections panel, the share and the accept are **real
  clicks**, and every file assertion is on **bytes on disk on the receiving side**. 74 checks.
  Three rules it earned: **pin the layout** (`WB_LAYOUT` takes a *path* to a layout file) so the
  panels under test are not below the fold — a scrolled-out control still has layout and still
  reports a screen coordinate, so an unguarded driver clicks the chrome at that pixel and says
  `ok`; **scope a waiver to the instance** — phase 11 restarts BOTH peers, which is the case
  documented to *work*, so the first-change waiver belongs only to phase 12's asymmetric restart,
  and `known_ok` shouts if it ever starts passing; and **say when the thing under test died** —
  a peer that SIGSEGVs mid-run otherwise presents as a dozen unrelated panel bugs.
  **`type` REPORTS WHAT THE CONTROL HOLDS, NOT WHAT XDOTOOL SENT** (D25, third instance —
  added 2026-09-08). It used to return `{"typed": "<what we sent>"}`, which is a receipt for
  keystrokes leaving the driver: on 2026-09-08 a run typed a receiving directory into an offer
  card, reported success, and the app accepted into its pre-filled default — **9 of 74 failed**,
  every one a file missing from a path nobody had ever accepted into, and the transcript could
  not distinguish *"the app ignored the operator"* from *"the input never arrived"*. The driver
  now reads the control back and returns `text`/`mismatch`; the scenario asserts it **before
  pressing Accept**, so the failure is named where it happens. A mismatch is **advisory in the
  driver and asserted by the caller** — a control may legitimately transform its input, and a
  driver refusing on that blocks scenarios that work. **The re-run was 74 · 0 · 1 known-open,
  and nothing was changed that affects whether keystrokes land** — so treat this shape of red as
  the harness first, and know the intermittency is still there.
- **For anything above the platform, the headless suite can drive real input too, and it is
  much cheaper** (added 2026-08-21). `Avalonia.Headless`'s `MouseDown` / `MouseUp` /
  `KeyPressQwerty` run the genuine route — hit test, capture, class **and** instance handlers —
  in ordinary xunit. `ProgramPanelInputTests` is the worked example: it clicks the on-screen
  d-pad at its hit-tested coordinates and asserts on **program state** (the cursor's cell index
  read out of the rendered display list), and it found two shipped defects in its first run.
  Reach for xvfb when the suspect is *below* Avalonia (Boundary D/G); reach for headless first
  otherwise.
- **Never wire a control's pointer input with `+=`** (AP37, P7). Avalonia delivers class
  handlers before instance handlers at the same element, and `Button` marks `PointerPressed`
  **and** `PointerReleased` handled in its own override — so `btn.PointerPressed += …` is
  accepted, never invoked, and warns about nothing. Use `AddHandler(…, Tunnel | Bubble,
  handledEventsToo: true)`. The on-screen game controller shipped in the `+=` form on
  2026-07-27, rendered perfectly, and set no bit for three weeks.
- **Never build a row template with `new FuncDataTemplate<T>` — use `Panels/Rows.cs::Rows.Of<T>`**
  (AP46). Avalonia types the builder's parameter `T` and **calls it with `null`** during container
  teardown: clearing an `ObservableCollection` a virtualizing panel has realized runs
  `Clear → RecycleElementOnItemRemoved → ClearContainerForItemOverride → ClearValue →
  TemplateBinding.PublishValue → ContentPresenter.ContentChanged → Build(null) → your lambda`.
  `<Nullable>enable</Nullable>` is on and cannot see it, so **all nineteen** sites in the frontend
  shipped the same null dereference, and the symptom was SIGABRT rather than an exception because
  `Dispatcher.UnhandledException` declined to set `Handled`. `Rows.Of` guards once at the framework
  boundary; `RowTemplateDisciplineTests` fails the build if a raw construction reappears, and its
  `Raw_FuncDataTemplate_Still_Crashes` control asserts the hazard is *still real* so the guard test
  cannot pass vacuously. **UI faults are now contained but only `MaxContainedUiFaults` (8) of them**
  — past that the process dies as before, and `WB_UI_FAULTS_FATAL=1` restores the old behaviour,
  which is the only way to re-measure a fault as a hard crash.
- **A link in a rendered page is followed by the MODEL, never the renderer** (AP47). A click
  passes the raw href to `BrowseModel.Follow`, which classifies it with `ClassifyTarget` +
  `resolveInSitePage` — the same rules `entity-browser-rust` uses, byte-identical by obligation
  because a slug addresses bytes in a content-addressed tree. Two traps, both live defects until
  2026-08-31: our resolver ignored the current page's directory and **did not strip `.md`**, so
  billslab.com's own `[Support](support.md)` asked for a key the signed root does not commit and
  every in-page link was dead; and links rendered as a styled `Span`, which **cannot receive
  pointer input at all** in Avalonia — use `InlineUIContainer` around a real control. When you
  wire that control, register `RoutingStrategies.Tunnel` **only** (a `SelectableTextBlock` parent
  handles `PointerPressed` for drag-selection, so bubble is too late; and `Tunnel | Bubble` on one
  element fires the handler *twice*, which is two navigations and two history entries per click).
  Gates: `workbench/site_link_crossimpl_test.go` carries the reference's own vectors verbatim —
  **a failure there is routed, not locally corrected** — and `MarkdownLinkClickTests` dispatches
  real `MouseDown`/`MouseUp` at hit-tested coordinates, because a test that invokes the handler
  passes against the unclickable version.
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
- **An undeclared DTO field is discarded in silence, and an unconditional redraw is a
  correctness surface** (AP49). `BrowserPanel.View` did not declare `RegistryPinFromOrigin` or
  `RegistryRebasedFrom`; the model computed both, the bridge sent both, `System.Text.Json` dropped
  both with no warning of any kind — so **AP45's "every surface must say so" was met by the shell
  and by nothing in the GUI**, and no test noticed because none read a value that had quietly
  become `false`. When a field crosses this boundary, assert it *arrives*
  (`The_Render_Envelope_Does_Not_Drop_The_Provenance_Fields`). Separately: `Refresh` rebuilt three
  lists and the whole body on every wake, so the columns re-laid out and the page lost its scroll
  offset on every click — the operator's *"it jumps around"*. Signatures now gate each rebuild.
  **The rail is dimmed during a navigation, not cleared**: the page does not become fresh when a
  navigation starts, so the old chain is exactly what describes the page still on screen, and
  emptying it deleted a true statement. A *refused* navigation still clears the page — that
  distinction is gated, because over-applying the correction puts unverified bytes on screen.
- **A site has THREE nouns — `manifest`, `pages/`, `assets/` — and we had implemented two**
  (AP50). **665 of billslab's 966 committed keys are figures** and none were reachable. Two
  reasons, both silent: the wire grammar is a directive, `::embed[caption]{ref=assets/figures/x.png}`,
  which every markdown parser renders as literal text (`![alt](src)` is the lightweight form and
  is lowered up into it at ingest); and Markdig models an image as a `LinkInline` with
  `IsImage=true`, so a figure that *did* parse would have become a clickable link to a page no
  site commits. Now: `workbench.ParseEmbeds` + `EmbedsToMarkdownImages` lower to one grammar
  before the renderer sees anything, `AssetResolver` is an **optional** interface (a resolver that
  serves no assets is a real thing — widening `ContentResolver` would force every impl to grow a
  method returning "no"), and the bridge exports `BrowseAsset` lazily. **`AssetNameFromRef` is a
  security gate, not a path helper**: it refuses `https://`, `//`, `/`, `data:` and any `..`
  segment, so a hostile page body cannot make the renderer fetch a tracking URL. Reference
  vectors are in `workbench/site_asset_crossimpl_test.go` — **a failure there is routed.**
- **`format: html` is a real value and the panel could not see it.** `SitePage.format` admits
  `markdown` and `html` (the web-tier escape hatch for a pre-rendered document); billslab
  publishes 23 HTML papers, the largest **8.27 MB**. The panel's DTO never declared the field, so
  HTML went into Markdig and rendered as its own source, and the raw body crossed cgo as an 8 MB
  JSON string on every render. `workbench.NewBodyView` now lowers HTML to text, lowers embeds,
  caps at `MaxDisplayBytes`, and says which of those it did; `BrowseRender` blanks
  `Content.BodyMarkdown` (the shell still reads it — a terminal has a pager). Why not a WebView,
  and what to build instead, is `docs/architecture/HTML-PAGE-RENDERING.md` — the short version is
  that a browser engine *executes* a document and would fetch subresources nobody in the chain
  vouched for, which is `AssetNameFromRef`'s hole re-opened at a much larger surface.
- **`Show()` is not a layout pass, and a headless test that skips one cannot see container
  teardown** (AP46's transferable half). `BrowserPanelTests` mounted a `Window`, called `Show()`,
  populated a list and cleared it — and never realized a container, so the crash path was
  *unreachable* from the suite while it reported 74/74. If a test's assertion depends on
  virtualization, recycling, or anything a panel does when items go away, call `UpdateLayout()`
  between the populate and the clear (`BrowserPanel.Settle` now does this for every browser test).
  Generalise: **constructing the world is not running it** — cf. AP36, where writing an input port
  and calling `tickOnce()` yourself could not see the queue.
- **`bin/` is build output, not the tree — rebuild before you measure behaviour with it.**
  On 2026-08-31 `bin/entity-shell` was six days stale and still carried the pre-AP44 refusal,
  so a live check "reproduced" a bug that had been fixed and was one sentence from being
  reported as a regression in a shipped surface. `bin/entity-fetch` *had* been rebuilt, which
  made it worse: two binaries from the same tree disagreed, which reads as a code difference
  between `fetch` and `shellcmd`. The tell is a refusal message you cannot `grep` in the
  source — if the string is not in the tree, you are running an old binary. `make build` first,
  every time, before attributing anything to source.
- **No suite in this repo may reach the public internet.** The Avalonia headless container has
  egress, and on 2026-08-31 a start-up registry pin turned six BrowserPanel tests red against
  the **live** registry's peer-id where the frozen fixture's was expected. A suite whose result
  depends on a remote host is not measuring this tree, and it fails for someone else's reasons —
  the same argument that keeps `crossimpl-go` and `consume-live` out of `test-native`. The
  browser's start-up pin is suppressed assembly-wide in `BridgeFixture`; the policy itself is
  covered in `workbench/browse_config_test.go`, where it lives. Note the trap that makes this
  easy to get wrong: **`WB_NO_AUTOPIN` and friends do not work from a test.** Go captures its
  environment once at process start, so a C# `Environment.SetEnvironmentVariable` never reaches
  `os.Getenv` in the bridge — env vars are for launching the app, and in-process control needs an
  in-process flag (`BrowserPanel.AutoPinOnOpen`).
- **The browser opens pre-pinned, and the config file is `~/.entity/browser.json`.** Precedence
  is `WB_REGISTRY_ORIGIN`/`WB_REGISTRY_PEER` > that file > the built-in
  `https://entitychurchregistry.org` with **no peer-id** (empty peer = adopt the origin's
  `entity-deployment.json` nomination, TOFU, labelled on every render). Shipping a peer-id in the
  binary would be a stronger default and a worse one — no operator agreed to it and it cannot be
  rotated without a release. A `browser.json` that exists and does not parse is an **error**
  (AP33), never a silent fallback. `workbench.LoadBrowseConfig` is the one place this is decided.
- **The GUI remembers its panel arrangement in `~/.entity/gui-layout.json`**, keyed by peer
  **alias** (`workbench/layout_config.go`; precedence `WB_LAYOUT` > that file > the built-in
  `site-view`/`detail`/`shell`). Alias and not peer-id **on purpose**: with no flags the GUI is an
  ephemeral in-memory peer with a fresh keypair every launch, so a peer-id-keyed layout would never
  match itself twice and the feature would silently do nothing in the default configuration — which
  is also why the layout is a file and not an `app/state/…` entity. A file that exists and does not
  parse is an **error** (AP33): the app opens on the defaults *and says why*, because a silent
  reset is indistinguishable from "it forgot again". A panel kind the build does not register is
  skipped and named, so a layout written by a newer build still opens.
  **`WB_LAYOUT` does not work from a test** — same trap as `WB_NO_AUTOPIN`, Go captures its
  environment at process start — so the headless suite redirects the file through the
  `LayoutSetPath` export instead. Without that, these tests would write to the developer's real
  `~/.entity`.
- `make build` — all shipped Go binaries (entity-shell + entity-console).
- **Every build/test target now refuses early if the sibling kernel is missing** (`preflight`,
  added 2026-08-24, AP41). `doctor` had that check from the day it was written and **nothing
  called it**, so cloning this repo on its own produced forty lines of module-resolution spew and
  no cause — measured by someone doing exactly that. The predicate lives once as
  `SIBLING_PRESENT` and is shared with `doctor`. **No suite in this repo can regress
  this**: a suite that runs at all is running in a tree where the sibling resolved, so if you
  touch it, test it the only way that works — `make preflight PARENT=/tmp/no-such-parent`.
- **Entry points, for when you need to actually run the thing:** `make doctor` (prerequisites
  + the sibling kernel — run this first on a strange machine), `make run` (build + REPL;
  `ARGS=` for one-shot), `make gui` / `gui-run` / `gui-build` / `gui-test` (root-level
  passthroughs to `avalonia/`), `make demo` (scripted CLI tour in a throwaway HOME — the
  fastest end-to-end validation that the shipped binary works).
- **`make gui` rebuilds the image; `make gui-run` does not.** Use `gui` after any Go or C#
  change, `gui-run` to launch what is already extracted. Both forward the app's own flags —
  `make gui-run ARGS="--new-identity second-peer"` (double-dash: the .NET frontend does
  not use Go's `flag` spelling). **With no flags the GUI is a PERSISTENT, REACHABLE peer** —
  same peer-id, on-disk SQLite, a listener on `0.0.0.0:9110`, an mDNS announcement — and that
  is the configuration to run; `--ephemeral` asks for the throwaway one. **This bullet said the
  opposite until 2026-09-08**, five months of sessions copied `--identity me` out of it, and
  `--identity` LOADS an existing identity and **fails** when there is none — so the example
  every doc inherited could not work on a first run. `--new-identity` is the creating form.
  `avalonia/README.md` is the full entry-path doc.
  **Both now launch through `run-with-dump.sh`, and the rebuild is the only difference left.**
  Until 2026-09-01 `gui-run` exec'd the bare binary, so whether a session had diagnostics
  depended on which target you happened to type and nothing recorded which. The app now prints
  `external diagnostics: minidump=… perfmap=…` at startup and writes it to the crash trail; **if
  that line says OFF, relaunch armed before analysing anything.**
- **Run logs live in `avalonia/run-logs/`, NOT in `dist-native/`** (AP54). `extract` does
  `rm -rf dist-native`, and every build target runs `extract`, so a log written there is deleted
  by the next build — which is what happened to the 2026-09-01 crash's stderr, and it had
  `WB_PANEL_LOG=1`, meaning the complete breadcrumb stream up to the fault was on disk and gone
  before anyone looked. `DOCTRINE-CRASH-FORENSICS` §1 already said *"write artifacts outside
  `dist-native/`"*; we had written that about smoke artifacts and put the run log inside anyway.
  Logs are timestamped, never overwritten: the interesting run is rarely the most recent one.
- `make go ARGS="..."` — escape hatch; `ARGS` carries the subcommand (`vet ./...`,
  `mod tidy`, `env`, …).
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
- **An async cgo export must copy every C-owned argument into Go memory BEFORE launching the
  goroutine** (AP31). A `*C.char` belongs to the .NET marshaller and is freed when the P/Invoke
  returns; reading it on the goroutine is a use-after-free that **does not crash** — it reads as
  the empty string and surfaces as a plausible user error. The synchronous exports beside it are
  safe for a reason that does not transfer.
- **On a .NET Linux crash, read `si_code` before you believe `si_addr`** (AP34). CoreCLR's
  handler **re-raises** any fault it cannot classify, so the signal that reaches the coredump
  carries `si_code 128` (SI_KERNEL) and `si_addr 0` — and a register context that is the
  handler's, not the fault's. Both 2026-08-21 desktop dumps read as null dereferences on that
  basis and were nothing of the kind. The real fault (`si_code 2`, SEGV_ACCERR, `si_addr =
  rsp-8`, rip on a `call`) is only visible by catching the FIRST SIGSEGV live —
  `make -C avalonia smoke-xvfb-click GDB=1`, which runs under gdb with `nopass`. Corollary:
  **a systemd ELF core cannot give you a managed stack** (the DAC rejects it, `0x80004002`),
  and `dist-native/tools/dotnet-dump` cannot run on the host at all — it is
  framework-dependent beside a self-contained publish. `make -C avalonia crash` now does the
  managed half inside the builder image.
- **A FIELD THAT PRINTS IS NOT A FIELD THAT ANSWERS** (D25, AP55) — the same failure as AP34, one
  field over, and the reason D25 is ratified. `coredumpctl info` **truncates** a backtrace, so a
  run of identical return addresses is a *lower bound* and can never say "unbounded". On
  2026-09-01 we read 45 such frames as a runaway recursion and shipped a depth bound for it; the
  same core says uniform 448-byte stride, exactly 45 frames, 19.2 KB, in a thread with ≥1 MB of
  stack — **an ordinary tree walk.** Run **`make -C avalonia crash-stack`** (now inside
  `make crash`) before believing any classification: it prints stride, uniformity, span, depth
  below the thread descriptor, and the verdict with its numbers. Two things it will tell you that
  nothing else does — a **uniform** stride is a single recursing call site while a mixed one is
  not a recursion at all, and a recorded `rsp` **absent from the core** means the fault went
  through CoreCLR's handler on an alternate signal stack systemd does not dump, so the registers
  describe memory the core lacks. Prefer a derived measurement to a printed count, always.
- **The UI thread's alternate signal stack is 1 MB, on purpose, and it is load-bearing.** The
  PAL default is **16 KB**, which is not enough for this process's handler chain under real
  pointer input: the click fuzz crashed **6/8 seeds** at 16 KB and **0/8** at 1 MB, same
  binary, same seeds. Installed at startup by `CrashDiagnostics.EnlargeAltStack`;
  `WB_ALTSTACK_BYTES=0` restores stock, which is the only way to re-measure the bug. Only the
  UI thread is covered by *that* call — a crash on another managed thread would look identical.
  **The render thread is covered too as of 2026-09-02, and it took the crash to do it** (AP66).
  `sigaltstack` is per-thread and must be called *on* the thread it covers, so something has to
  run there; Avalonia 11.2 offers no tidy hook (`IRenderTimer.Tick` is **internal**, `Compositor`
  callbacks run on the UI thread, `AvaloniaLocator.Current` is gone). **`AltStackProbe` is the
  answer**: a zero-size, hit-test-invisible control whose `ICustomDrawOperation` is executed by
  the compositor during the render pass — the same call stack the fault was taken in. It
  re-invalidates until the install lands, because a control that is never dirty is never drawn and
  the crashing session had been idle for 17 s. **Gate: `run-xvfb-smoke.sh` exits 3 if the run log
  lacks the render-thread line** — the only harness that can see it, since the headless suite has
  no render thread and there is no cross-thread query for another thread's alt stack. Other
  managed threads are still uncovered, so `run-with-dump.sh` stays non-optional.
  **Read `journalctl` FIRST on any hard crash.** The 2026-09-02 death was named outright by
  `kernel: signal: entity-avalonia[<tid>] overflowed sigaltstack`, which also converted §0J's
  "createdump produces nothing for this fault class" from an untested hypothesis into a
  measurement. Note the tid there is the **thread**, not the pid — if they differ, the fault is
  not on the main thread and every per-thread mitigation you installed at startup missed it.
  **That kernel line is not reliable, though — its absence proves nothing.** The 2026-09-06
  crashes were the same fault class with no journal line at all, because the message is printed
  at signal-frame setup and this overflow happened *inside* an already-running handler.
- **EVERY thread now gets a 1 MB alt stack, and the mechanism is an `LD_PRELOAD` interposer —
  not the managed installs** (`avalonia/altstack-preload.c`, loaded by `run-with-dump.sh` and
  `run-xvfb-smoke.sh`). The per-thread managed installs reach the UI thread and the render
  thread and **cannot reach anything else**, because `sigaltstack` must be called ON the thread
  it covers and no managed hook runs on a fresh thread-pool, finalizer or timer thread. On
  2026-09-06 the GUI died twice in four `make twopeer-gui` runs on exactly such a thread, with
  **16 PAL-shaped stock 16 KiB alt stacks still present per core** while both enumerated threads were correctly
  covered and every gate was green (AP74). Measured from the cores' `PT_LOAD` headers: 12,288
  bytes usable, **13,088 consumed**, 800 into the guard page, one 6,960-byte frame, *identical
  in both cores*. The extra demand is the **CPU** — this AVX-512 host reports
  `sysconf(_SC_MINSIGSTKSZ)` 3376 against a compile-time `SIGSTKSZ` of 8192 — so the overflow
  is **deterministic** and only the arrival of a signal is intermittent. `WB_ALTSTACK_BYTES=0`
  disables the interposer *and* the managed installs, which keeps the A/B honest.
  **The gate is a population sample, not an install count**: the app starts one ordinary thread
  and asks the kernel what it got (`CrashDiagnostics.ProbeAltStackCoverage`);
  `AltStackCoverageTests` asserts on it, and `run-xvfb-smoke.sh` exits 3 without
  `coverage=ALL-THREADS`. **A dlopen'd library does not interpose** — `DllImport` resolves
  `libaltstack.so` happily in a process that never preloaded it, so "the symbol is there" is not
  the check; the interposer's own call counter is.
- **A derived UI property is not a completion signal** (AP32). A headless test that settles on
  "the button re-enabled" returns in the window between the bridge call returning and the
  goroutine entering the operation, and then asserts against an empty view — green, measuring
  nothing. Wait on the bridge handle's monotonic `ops` counter instead.
- **Avalonia builds go through podman, always** — `cd avalonia && make build && make
  extract`, then `make host-run`. The host has no .NET and never needs it; never `dnf
  install dotnet`. Bridge-only smoke check: `cd avalonia/bridge && CGO_ENABLED=1 go build
  -buildmode=c-shared -o /tmp/libbridge-test.so .`.

## Code style

- **Read the source before asserting** path shape / addressing / namespace claims (see
  AGENTS-STANDARD). The whole tree is peer-id-namespaced, so **"peer-id keyed" is almost
  never a valid distinguishing claim** — if you reach for it to explain why something
  matters, you're probably about to mislead. Cite `file:line` in test comments and doc
  explanations.
- The project measures everything against the **27 disciplines (D1–D27)**, ten review
  questions, and anti-pattern catalog (AP1–AP113) in `docs/architecture/DISCIPLINE-CHARTER.md`.
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
- **A DIAGNOSIS WHOSE VISIBILITY DEPENDS ON THE OPERATOR'S LAYOUT IS NOT A SURFACE** (AP84,
  2026-09-10, found by an operator losing a morning to it). The reconciler named the fault at
  startup, correctly and completely — *"could not open our own connection to this peer — nothing we
  write to a shared folder will reach them"* — and printed it to **stderr**, i.e. to
  `avalonia/run-logs/`. The saved layout held `tree-view` and `peer-connections`, neither of which
  says anything about sharing, and every warning we had built the day before lives in the *Sharing
  Status* panel, which was not open. So the app's own correct diagnosis was on screen nowhere.
  **Worse, three surfaces actively reassured**, all reading `Workspace.Conns` — which is the
  **address book**, not a connection: the peer status line said *"1 remote"*, the Nearby row said
  *"Connected"*, and `PeerSummary.connections` is `len(Shell.Conns)-1`. None of them drops when the
  far peer is switched off, so all three were confidently wrong in exactly the case the operator was
  looking at them for. **`PeerView`'s problem banner is the fix** — docked, always present, fed by
  `StatusRender` (a READ; it observes and never dials, so hanging it off the declaration wake cannot
  turn an open window into a dialer), captioned *verified just now* vs *as of the last pass* from
  `Reconciled` in the outcome. Gate: `PeerProblemBannerTests`, whose third arm crosses the bridge
  because the other two drive the renderer directly and would both pass if `problems` were renamed
  in Go (AP49 — a dropped field renders as *"everything is fine"*, the worst available failure for a
  surface whose only job is to say otherwise).
  Two rules. **`make reachability` cannot see this** — it asks whether a model has *a* surface, and
  the reconciler has a verb and a panel; the question it will not ask for you is *what does the
  operator see when the panel that renders this is closed*. And **when you name a fact the operator
  must act on, say where it renders with no panel open** — stderr is not a surface. (An
  earlier draft of this entry added *"and `run-logs/` is deleted by `extract`"* — that is
  **false and backwards**: run-logs live outside `dist-native/` precisely so they survive, which
  is AP54's whole fix. The log was on disk the entire time. Nobody looked, which is the point,
  and the invented embellishment made the finding sound worse while making it wrong.)
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
  opposite machines. Direction is **not observable from core-go** (`Connections()` concatenates
  and tags nothing; `IsConnected` conflates the pool with the §6.11 reentry map) — routed as
  `reviews/CONNECTION-DIRECTION-AND-BILATERAL-REACH-2026-09-09.md`.
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
- **A PANEL TEST ASSERTS INSIDE ITS OWN ROW, because the fixture peer is SHARED and other tests
  write real state to it** (AP70). Every headless panel test runs against
  `BridgeFixture.DefaultPeer` — one peer for the whole assembly — and `SharePanelAcceptDirectoryTests`
  accepts a folder into a temp directory on it. So any panel that reads real declarations renders
  rows your test did not create, and an assertion phrased over *the panel* is really phrased over
  everything every other test in the assembly has ever done to that peer. It bit both ways in one
  run: a negative assertion failed on somebody else's row (which reads as a product defect and is
  not), and a positive one passed on somebody else's row while its own seed could not express the
  case the test was named after. Use `RowTextsFor` / `FindButtonInRow`, and **when a seed helper
  cannot express the distinction the test name claims, fix the helper** — otherwise the test is
  decoration. The tell: only the NEGATIVE assertion can catch this, because contamination only
  ever adds matching rows.
- **`Sharing Status (declared vs. actual)` is the third Network panel, and it is the missing half
  of the control loop rather than a new feature.** Before it there was **no bridge export for
  devices, folders or the reconciler at all** — the whole declared-state layer was reachable from
  one shell verb and from no pixel — while the GUI *ran* a reconcile pass at every startup and
  reported its problems to **stderr**, i.e. to `avalonia/run-logs/`, where nobody looks. An
  operator whose accepted folder had lost its mount was told so, correctly, somewhere they would
  never see, with every panel on screen looking fine. **`make reachability` cannot raise this**:
  it asks whether a `workbench/*_model.go` has a surface, and the reconciler is `shellcmd`'s and
  has a verb. The question the sweep will not ask for you is *which frontend can reach this, and
  by pressing what*. It also carries two verbs — Pause/Resume (writes the **declaration**, so the
  loop obeys it) and Remount (the action the loop names and refuses) — because a panel with no
  verb in it is AP57's tell.
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
- **The GUI's default peer is persistent, listening and announcing, and it was none of those.**
  Until 2026-09-03 the default was an in-memory peer with a fresh keypair per launch, no
  listener and therefore no discovery. The tree is peer-id-namespaced, so that default was not
  "some features off" — it made the app **a different peer on every start**, silently
  invalidating every grant, mount, offer and accepted share from the previous session. An
  operator debugging a two-machine share by relaunching was destroying the state they were
  debugging, on both machines, every time. `--ephemeral` restores it deliberately.
  **`BringUpListener` (`shellboot/listener.go`) is the ONE bring-up** — bind, advertise, announce
  — shared by every frontend, because the version that lived in `PeerManager.Create` left
  `entity-shell -listen` binding nothing (AP67). A wildcard bind now advertises this host's LAN
  address rather than publishing no profile at all; a peer with no profile cannot be reconnected
  to by peer-id, which is the manual step the whole redesign exists to delete.
- **A file holds what you need in order to find the tree; the tree holds everything else.**
  Identity, store path and listen address are pre-peer facts and must be on disk. Everything
  after that — devices, folders, and eventually `gui-layout.json` and `browser.json` — belongs in
  `app/workbench/`, where it is addressable, watchable and travels with the peer. The layout
  file's stated reason for being a file (a peer-id key never matches twice under an ephemeral
  default) died with the ephemeral default; migrating it is owed, and needs a read-both /
  write-tree transition so an existing install does not lose its layout.
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
- **A test that drives data to the row and never SELECTS one has not tested the panel** (AP61).
  The file explorer's end-to-end test crosses mount → watcher → subscription → ingest → model →
  bridge → panel, and asserted on row viewmodels — one method call short of the code that renders
  a row. So `OnRowSelected` shipped throwing on **every** file an operator clicked, at 138/138
  green: the mtime field carried Unix **milliseconds** (all four producers write `UnixMilli()`),
  was documented as "a Unix second", and `FromUnixTimeSeconds` throws for every date past year
  9999. The preview pane never updated because the throw is upstream of `LoadPreview`, and the
  **ninth** click killed the process — `MaxContainedUiFaults` is 8. The envelope test could not
  catch it either: it hard-coded `1700000000`, a seconds value **no producer emits**, which is
  AP58's shape at the level of a value rather than a wrapper. Three rules. **Put the unit in the
  field name when it crosses a boundary** (`ModifiedAtMillis`) — a doc comment is invisible from
  the far side of cgo and JSON. **A data value must never be able to end the process**: an mtime
  is a number a *filesystem* chose, so the render path guards its range and prints `mtime out of
  range (n)`. And **select the row in the test** — `SelectRowForTests` drives the real
  `SelectionChanged` route, and the gate asserts the detail line carries the current year, so a
  unit swap fails rather than silently printing 1970.
- **A mount has TWO layers and a single count reads the one that cannot fail** (AP59). The
  watcher writes `local/files/{root}/{rel}` for every admitted file; the ingest chain lifts each
  into a `doc/*` at the target prefix. The mount row's `FileCount` read the *source* layer, so a
  directory of 400 photographs and one README displayed "401 entities in tree" with exactly one
  openable document. Report **both sides of a lossy stage** — `FileExplorerModel` carries
  `TotalFiles` / `Ingested` / `NotIngested` — and make a per-item absence carry its **reason**,
  never a blank column.
  **What a file becomes is `workbench/doc_types.go`**, an extension→type registry:
  `doc/markdown-file` (unchanged and byte-identical to what it always was — see
  `doc_file_data.go` for why markdown keeps its own struct), plus `doc/text-file`,
  `doc/code-file`, `doc/image-file`, and `doc/binary-file` as the honest fallthrough. There is no
  "unhandled" outcome any more. Classification is by **name only, never by content**: reading a
  4 GB video's first chunk to learn what its extension already said is a cost with no answer at
  the end of it. `ValidateMountTarget` must be passed `DocEntityTypes()` — the whole set — or a
  remount conflicts with its own predecessor's output.
- **The workbench half of a mount is persisted at `app/workbench/mounts/{root}`**
  (`workbench/mount_binding.go`) and restored by `shellboot` at startup. The kernel persists the
  RootConfig and rehydrates the watcher; the **source→target mapping the ingest handler routes on**
  is ours, and until this landed it existed only in `NotificationIngestHandler`'s memory. After a
  restart the watcher came back, wrote its file entities, and every delivery answered
  `404 no_mount_for_uri` — a mount that listed as healthy and had silently stopped producing
  documents. `Mount` writes it, `Unmount` removes it (or the unmount undoes itself at the next
  launch), and every failure path in between unwinds it.
- **HISTORY RECORDING IS OPT-IN PER PATH, AND NOTHING IN THE MOUNT/SYNC/SHARE PATH TURNS IT
  ON** — which is the whole distance between where M3 is and where it needs to be. The
  recorder tracks a path only when a `system/history/config` entity matches it
  (`ext/history/config.go`, `configCache.find`); with no config a query returns **empty and no
  error**, which reads exactly like "the tree kept nothing". `DOMAIN-LOCAL-FILES` §1.1a — our
  own WB-25 closure — rules that a concurrent same-path write is last-arrival-wins at the FS
  surface with **both writes recorded at distinct chain positions**, and *the substrate already
  delivers that in full the moment recording is enabled.* Measured
  (`shellboot/concurrent_edit_baseline_test.go`), receiver's chain after a concurrent edit:
  `[0] updated local/files:write` (the delivery, which won on disk) · `[1] updated
  local/files:watch` (the receiver's own edit, **preserved and byte-recoverable**) ·
  `[2] created local/files:write` (the seed). **The `handler`/`operation` on a transition is
  the provenance discriminator conflict detection needs** — a local edit arrives through the
  WATCHER, a delivered one through `blob_resolve`'s dispatch. So M3 is **not** "build a merge
  engine": the tree-side guarantee is already met and had simply never been switched on.
  **BUT THAT READING IS TRUE AT THE INSTANT OF DELIVERY AND ERASED SHORTLY AFTERWARDS, AND THE
  PARAGRAPH ABOVE USED TO SAY OTHERWISE.** Every delivered file acquires a `local/files:watch`
  transition a moment later: the receiver's own watcher ingests the file `blob_resolve` just
  wrote to disk, and a file entity carries `modified_at`, which the watcher reads from the
  filesystem rather than from the write — same bytes, different entity, real transition. So the
  HEAD says `watch` for **every** file in a received folder, and the obvious detector built on
  it flagged all of them. The baseline test's `trans[0].Operation == "write"` assertion passes
  because it reads within a second of the delivery; it is a measurement taken at one moment,
  read as a property. **The question that survives is *who last changed the BYTES***:
  `workbench.localEditAwaitsDelivery` walks the run of consecutive transitions carrying the
  current content hash and reads the OLDEST member's operation, because an mtime-only echo
  lengthens the run and cannot change its oldest member. Known limit, unchanged: a local caller
  dispatching `local/files:write` directly records as a delivery — which is why `resolve -keep
  mine` writes through the **filesystem**. The generalisation is the part to carry: **a fact
  established by reading a mutable structure once is a fact about that instant**, and the tell
  is a test that reads immediately after the event it is about. **`shellcmd/folder_history.go` switches
  it on**, as a derived output of the reconciler — a folder that `Receives()` and is mounted
  gets a config for its mount prefix. `Receives()` and **not** `IsLocal()`, and here the two
  genuinely differ: `share` declares the OWNER's folder `both`, so either side of a shared
  folder can be overwritten and both record, while keying on `IsLocal()` would leave the owner
  — whose files these actually are — as the one side with no chain. A **send-only** folder
  gets none, because one writer means an entity per save forever answering no question. What
  is left of M3 after 2026-09-07 is `EXTENSION-REVISION`, and nothing else in this list. This is
  D20 aimed at the kernel one more time: **grep `../entity-core-go` for the mechanism before
  pricing the build.**
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
- **`Files` browses a mount; `Local Files` manages mounts.** Two panels, one question each, same
  rule as the browser trio. `FileExplorerPanel` joins the two layers above and is the only surface
  that shows an actual file; it holds a wake handle because mount *contents* churn, unlike mount
  *configs*. **Avalonia has no built-in file explorer** — the framework's own sample uses the
  separate `Avalonia.Controls.TreeDataGrid` package, and we deliberately do not take it: a new
  templating surface is AP46 re-opened where `Rows.Of`'s guard does not look.
- **`Shared Folders` is the flow panel** (`SharePanel`, category Network) — discover, share out,
  read a peer's offers, accept, and see what is arriving, in the order you do them. It is a thin
  envelope over `ShellWorkspace.Share`/`Unshare`/`Accept`/`Offers`/`Sync`/`Unsync` via
  `avalonia/bridge/share.go`; **do not reimplement any stage in the renderer** — the four
  mutual-authorization facts in AP63 would then live in two places, and they are exactly the
  ones nobody rediscovers by reading code. It renders three fields a paraphrase drops:
  `reconnected` (a grant written on a live connection is inert), `publisherMustDial` (the step
  neither side's verb can perform), and `caveat` (withdrawal binds the NEXT handshake).
  Until 2026-09-02 the whole flow was shell-only and an operator reported, correctly, that
  there was nowhere to share a folder.
- **EVERY PANEL DECLARES `IPanelPreferredHeight`, AND A NEW ONE THAT FORGETS IS A LAYOUT BUG**
  (AP64). `PanelStack` sizes its Grid to `max(viewport, sum-of-slot-minimums)`; a slot's minimum
  is what its panel declares. `ProgramPanel` was the only implementer for six weeks, so every
  other panel claimed the 200px default, three panels summed to less than the viewport, and each
  got ~297px — **less than its own fixed chrome, clipped, with nothing to scroll** while the
  stack's always-visible scrollbar sat inert. The only workaround was to close panels until one
  was left, which is what an operator did before any test noticed. The interface's own doc had
  told implementers to skip it; **a default that is wrong for every caller is a bug with a
  docstring.** Pair the floor with the other half: never put an unbounded list in a docked
  region — bound it (`MaxHeight`) so an under-estimate degrades to scrolling rather than to an
  unreachable button. Gate: `PanelStackScrollTests`, which asserts `Extent > Viewport` at a real
  window size (the only measurement that tells a scrollbar from a picture of one) and collects
  every floorless panel before failing.
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
- **Logging:** `PanelLog` breadcrumb discipline + category list per
  `docs/architecture/LOGGING-CONVENTIONS.md`; the pre-crash breadcrumb is the forensic surface.
- **Testing:** four tiers per `docs/architecture/TESTING-STRATEGY.md` — naming the tier is
  the discipline.

## Project structure

Architecture is a **dependency graph, not a strict stack** — five layers, where the panel
framework and the application are **siblings** (the panel framework could work without
entities):

1. **Entity Core** — peer, store, protocol (the `../entity-core-go` sibling).
2. **Entity Developer Framework** — Executor, PeerContext, Resolve, Format.
3. **Panel Framework** — panel, focus, actions, content contracts (entity-independent).
4. **Application** — content models, panel declarations, state persistence.
5. **Renderers** — medium-specific ordering + manifestation.

- Go packages: `entitysdk`, `workbench` (renderer-neutral models + business logic),
  `programs` (the entity-native programs track), `shellcmd` / `shell` (entity-shell verb-ops
  + REPL), `shellboot` (shared bootstrap + `PeerManager`/multi-peer lifecycle), `avalonia/`
  (bridge + C# frontend), `console/`. `ext/identity/` is the identity extension.
  Dep direction: `workbench → entitysdk`; `programs → entitysdk`;
  `shellcmd → entitysdk, workbench`; `shellboot → entitysdk, shellcmd, workbench`.
  **`workbench` cannot import `shellcmd`.**
- **`programs/` is a sibling of `workbench`, not a layer of it.** It holds the generic
  compute host, program descriptors, and the Life/Snake/Asteroids/heavyfield programs —
  extracted from `workbench/program_*.go` on 2026-07-22 because a research track does not
  belong inside the app's renderer-neutral model layer. It depends on `entitysdk` **only**;
  it must never import `workbench` (that dependency was zero at extraction — keep it zero).
  `avalonia/bridge` imports it as `pg`. Tests: `make test-programs`.
- **`Host.Input` ENQUEUES; it does not write through** (AP36). A clock-driven program reads its
  input ports at tick time and only then, so a value superseded before the next tick was never
  observed by anything — at Life's 6 Hz that window is **167 ms** and a mouse click is ~25 ms.
  Measured: **1 of 6 d-pad clicks moved the cursor**; 6 of 6 when held past a tick. The contract
  now is *every value offered to a port is observed by exactly one tick, in order* — bounded at
  `inputQueueMax`, coalescing at the tail on overflow, deduping an identical consecutive value,
  and **shape-agnostic** (the host still never decodes a program's bytes). The known limit is
  stated in the doc comment: a bit released and re-pressed between two ticks reads as one
  continuous hold, because the queue carries port *values*, not an event stream.
  **A test that writes the input and then calls `tickOnce()` itself cannot see any of this** —
  that is what every pre-2026-08-21 program test does. The region test is
  `programs/host_input_queue_test.go`, which drives a **running clock** from outside.
- **Compute has no randomness, on purpose — so a program's "randomizer" is a hash, and a
  hash that is linear in its varying input is a TRANSLATION** (AP38). Interactive Life's
  Regen slid one fixed pattern for a month: under a power-of-two modulus, bit *k* of an LCG
  step depends only on bits 0..*k* of its input, so bumping the generation counter is
  arithmetically the same as shifting the cell index. Measured at 0.98–1.00 agreement under
  a cyclic shift, and the operator's report was *"it moves the same map one or two over."*
  **One nonlinear round** (square, then fold the high bits back down — squaring mod 2^k
  leaves the low bits weak) is the minimum. Two things to carry: `!equal(before, after)` is
  the wrong assertion — a translation is never equal, so an anti-vacuity clause passes on
  every one of these boards — and **population/variance is the cheap tell**, since a
  translation preserves the count (σ 0.76 where an independent draw gives 7.75). Gate:
  `TestLifeEdit_RegenIsNotATranslation`. Determinism itself is correct and load-bearing —
  reproducible state hashes are the whole point; the entropy is the tick counter at the
  moment of the press.
- `docs/architecture/` — canonical framework (charter, `MODEL-AVALONIA-RUNTIME.md`,
  `GUIDE-AVALONIA-PANEL-PATTERNS.md` recipes P0–P7, `TESTING-STRATEGY.md`,
  `LOGGING-CONVENTIONS.md`, `DEPLOYMENT-DIRECTION.md`, `SHELL-DIRECTION.md`,
  `CROSS-IMPL-HELPER-REFERENCE.md`, `PERFORMANCE-CHARACTERISTICS.md`). These are **undated /
  living** — edit in place.
  - **`docs/STATUS.md` — the rolling status log, and it lives OUTSIDE `docs/status/` on
    purpose** (moved there 2026-08-24, to match the rest of the ecosystem).
    `docs/status/` is stripped wholesale from the published tree under [ADR-0031], so a
    rolling log left inside it is a canonical document sitting in a directory whose whole
    meaning is "none of this publishes". The path *is* the declaration: out of that
    directory means canonical, in it means working memory. **`docs/STATUS.md` is published
    — write it for the next session, but know a stranger can read it.**
  - `docs/status/` — the ephemeral area: dated `STATUS-YYYY-MM-DD.md` snapshots (immutable
    once published) and `HANDOFF-*`. Never published, no scrub obligation.
  - `docs/architecture/reviews/` — dated cross-team exchanges (`reviews/{TOPIC}-{DATE}.md`);
    closed ones move to `reviews/archive/`.
- **`CANONICAL-DOCS.toml` is a published artifact, not configuration** (AP42). It is the whole
  interface to the release pipeline — we declare, it publishes ([ADR-0031]) — and its `blurb` is the prose a
  public reader gets **instead of** the document. Two rules, both earned on 2026-08-24: **(1) a
  blurb describes, it does not count** — every numeric range in it had gone false (D1–D23 for
  D1–D24, AP1–AP27 for AP1–AP40, "six-boundary" for seven, P0–P6 for P0–P7) plus a `github =`
  org that does not exist, because a `.toml` gets read as config and skipped by review; **(2)
  declaring a doc is part of adding it** — `AGENTS.md` is public and told the reader to open
  `DOCTRINE-CRASH-FORENSICS.md`, which was undeclared, so the instruction shipped broken. If a
  diff changes a count a blurb restates, or adds a doc a published doc cites, the manifest is
  part of that diff. **(3) An omission in a keep-list is an act of DELETION** — undeclared means
  dropped, so a root doc that is already on public `master` and missing from this file gets
  *withdrawn* at the next release. That was live on 2026-08-24 for seven files including
  `SECURITY.md`. Review the manifest against **what is currently published**, not just against
  the tree — the two move independently, so the answer changes without this repo changing.
  Compare a filtered export of `dev` against the published remote (**never a local `master`** —
  it drifts from what is actually published and answers this question wrong). The procedure and
  the local tool paths belong in your git-ignored `AGENTS.local.md` / `.agents/`, not here.

  **(4) Declaring a doc changes what "internal" means about it.** `AGENTS.md` is written for us
  and is *published*, and internal infrastructure names are exactly what an internal-audience
  document is made of — several had to be rewritten out of this file on the day it was declared,
  **and more had to come out on 2026-08-25**, which is the part worth learning from. What
  survived the first pass was everything that read as *engineering*: another team's incident
  history, their process, packet identifiers, who miscounted what. It was all true and all
  useless to the reader it was being shipped to. **A machine-local or internal path belongs in
  the git-ignored `AGENTS.local.md` / `.agents/`** ([ADR-0020]), never here. Assume you
  **cannot** self-check this category in one pass — the second reading is the one that finds it,
  and the test is not "is this true" but "is this the reader's business."

  Note rule (5) below already said this and was written to be applied to `docs/STATUS.md`. It
  binds every declared file, this one included. **A rule stated in a document does not exempt
  that document.**

  **(5) The rolling log is about this project.** `docs/STATUS.md` publishes; it carries our tree
  state, our defects, our decisions. Coordination with other teams — their processes, their
  tooling, their internal state — is **theirs**, and goes in `docs/status/` or a `reviews/`
  packet, neither of which publishes. "It explains why we changed a file" is not an exception;
  say what we changed and why it is right for this repo.
- **Don't synthesize project state from `git log` or top-down code reading** — use the
  framework + latest status snapshot + the newest `docs/status/HANDOFF-*`. **There is no
  roadmap doc and there has never been one** — this file named a
  `REPOSITORY-WORKSPACE-ROADMAP.md` and `PHASE-*-PLAN.md` for months and neither exists, which
  sent every new session looking for a file to orient from. "What's next" lives in the newest
  handoff's recommended-order section and in `STATUS.md`'s "Waiting on"; one direction doc per
  topic covers the rest. Corrected 2026-08-20, by audit.

## Boundaries — do NOT modify

- **You work on `dev`. You do not touch `master`, ever.** `master` is the **public canonical
  mirror** and is republished at each release — it is not a branch this repo's working sessions
  advance, propose advancing, or reason about. Promotion `dev → master` is the **release act**,
  performed by maintainers; [ADR-0015] and its amendment are the authority, [ADR-0022] covers the
  pipeline. **`dev` being ahead of `master` is the normal, expected steady state** — not a pending
  decision, not a status-file row, simply unreleased work. This file used to carry *"the one
  decision left: `dev` is N commits ahead of `master`"* as an open item across sessions, which
  presented someone else's act as our decision and invited a future session to act on it.
  **A session that finds itself weighing a merge to `master` has already gone wrong; there is
  nothing to weigh.**
- **`../entity-core-go/` is a sibling dependency, not part of this repo.** Read it for
  protocol/store behavior; never edit it from here (route cross-impl changes via `reviews/`
  per AGENTS-STANDARD).
- **Status snapshots (`docs/status/STATUS-YYYY-MM-DD.md`) are immutable once published** — never
  re-open a closed snapshot to add work; write a new dated one.
- **Avalonia is podman-only** — don't touch the host package set for the .NET toolchain.

## Repo-specific gotchas

- **"identity" vs "keypair" — keep these distinct** (the word is overloaded across the
  codebase; full discussion `DEPLOYMENT-DIRECTION.md §3`):
  - **keypair** — the bare Ed25519 keypair (what `crypto.LoadIdentity/SaveIdentity`
    load/save — upstream misnomer; don't propagate it).
  - **identity bundle** — the on-disk directory shape
    (`entitysdk/identity_bundle.go::IdentityBundle`).
  - **identity entity** — the V7 hash-addressed public-key entity (`peer.Identity()`).
  - **identity extension** — the attestation + quorum + identity stack (`ext/identity/`).
- **Three Avalonia panels read the same bytes and answer different questions — say which in
  the name.** `Browser` is the journey (registry → name → site → page, remote); `Local Site` is
  a site published by **this** peer and opens the bundled demo; `Origin Inspector`
  (`PublisherVerifyPanel`) renders the verification **chain** and deliberately never renders a
  page. An operator called the set incomprehensible and was right: they were named `Browser`,
  `Site` and `Publisher Verify`. `PanelRegistry.Register` now takes a **category** and a
  **blurb**, the picker groups by category with the blurb as a tooltip, and a new panel that
  registers without them lands in "This peer" with no explanation — which is a bug, not a
  default.
- **workbench is the brain; renderers are thin I/O.** All business logic (entity
  resolution, CBOR/markdown formatting, handler discovery, tree/selection state, the
  content models — `tree_model`, `detail_model`, `shell_model`, `peer_info_model`,
  `log_model`, `handler_model`, …) lives renderer-neutral in `workbench/`; `Render()`
  returns a plain struct that any renderer drives. **Never reimplement model logic in C#
  or tview.** Treat `workbench` as the Go "standard library" for entity apps —
  protocol-first (execute / tree get-put), never direct store/index access from app code.
- **Multiple renderers are a discipline enforcer, not a parity obligation.** Console is
  kept (frozen, single-peer) purely to keep the renderer-neutral core honest; Avalonia
  drives all feature work and may outpace it. If a model-layer change breaks console,
  that's a signal the abstraction was wrong — fix the model, don't gate Avalonia on console
  parity. (The canvas/raylib renderer has been removed.)
- **DRY the integration, not the renderer.** Shared shell↔workspace wiring lives once in
  `shellcmd/integration.go` (e.g. `PersistAliases`, `PublishWDTo`); renderers add one line
  of wiring. Same closure in two renderers = extract it.
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
- **A dispatched read of a peer-qualified path is a REMOTE read** (AP11). `AppPeer.Get` /
  `List` / `Has` route by peer-id: `List("/{them}/…")` dispatches to *that peer* and
  returns *their* tree, not our cached mirror of it. To assert on a mirror — or on anything
  we hold in another peer's namespace — read `AppPeer.Store()` (L0) instead. A test that
  gets this wrong is green whether or not the mirror was ever written.
- **Mirroring another peer's subtree needs a capability that names their namespace.** The
  owner self-cap's `Resources: ["*"]` is peer-**local** under §PR-8, so `tree:merge` 403s on
  every `/{them}/…` target. Use `AppPeer.MintMirrorCapability` +
  `Executor.executeAs`; the destination itself comes from the publisher's signed
  published-root via `AppPeer.MirrorDestination`, never from a caller-chosen string.
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
- **Perf measurement:** `modernc.org/sqlite` under `-race` is ~17× slower — perf benches
  must override the default flags (`GOTEST_FLAGS="-count=1"`); never trust SQL bench numbers
  taken with `-race`. To reconcile store/index count discrepancies, `SqliteStore.DB()` lets
  you `SELECT` the `entities` table directly.
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
  §2.2 names `/{signer_peer_id}/system/signature/{hex}`; every reader resolves it relative to the
  peer it is **reading from**, and those coincide on a direct read only. **B HOLDS the signature**
  at the right key — nothing is missing, it is looked for in the wrong place. ⚠ **Ruled out and
  measured so nobody reaches for it:** B's root commits to **0 keys under A's namespace**, which is
  expected (a detached signature is read outside the committed set by design), so *"carry it in the
  root"* is not the fix and is `A-38`(D) pushed past where moving a prefix can go. **Not patched
  here** — it changes the verification path, and a second way to locate a signature is a second
  trust argument. Asks `A-45`/`A-46`, packet `ROUTING-2026-09-16-c-…`.
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
- **THE FOLDERS AND PEERS LISTS IN THE SHARING STATUS PANEL HAVE NO HEIGHT BOUND** (found
  2026-09-12; **the reachability half is CLOSED 2026-09-16 and the premise was partly wrong**).
  Conflicts, delivery and recording are bounded; these two are not.
  What the original entry recorded: one extra line of text above the folders list pushed the
  Remount button outside its clipping ancestor and turned
  `Remount_Is_Offered_Only_When_The_Loop_Is_Stuck_On_A_Missing_Mount` red — read as AP64's other
  half, *never put an unbounded list in a docked region*, one row of honest text from an operator
  meeting it. The conflict-rule line reproduced it exactly on 2026-09-16, and **both failures were
  the TEST**.
  ⭐ **`SharingStatusPanel` puts its whole body in a working `ScrollViewer`, on purpose, so an
  under-estimated floor degrades to scrolling rather than to an unreachable control — and the
  predicate could not see the difference.** `IsWithinAllClippingAncestors` was three byte-identical
  private copies asking *is this inside every clipping ancestor's bounds*, which is right for AP64
  (where the stack's scrollbar sat **inert** and there was nothing to scroll) and wrong in general.
  One copy now: `avalonia/tests/…/Reachable.cs`.
  ⭐ **The non-obvious half, and the first fix still failed on it.** Walking every ancestor is the
  wrong shape: a control inside an UNSCROLLED viewport has a position that is only true while it is
  unscrolled, so measuring it against ancestors above the viewport measures a position the operator
  is about to change. Measured — a button 560px down a 600px stack in a 100px `ScrollViewer` passed
  both scroll containers and was then rejected by the **`Window`**, `y=894` against `h=768`. So on
  leaving a scrollable viewport the subject is **substituted**: the question becomes *is the
  viewport reachable*. That also keeps AP64 caught at the right level — a panel whose own
  `ScrollViewer` is clipped out of its slot reveals nothing by scrolling, and the substituted check
  fails.
  Gate on the gate: `ReachableTests`, whose **negative** arm (clipped, nothing to scroll) is the
  one that matters — a widened predicate is one step from a predicate that never fails.
  ⚠ **What is genuinely still open is smaller than the entry claimed.** Controls are reachable;
  the lists are still unbounded, so a peer with many folders pushes the sections *below* them
  behind a long scroll. That is ergonomic, not unreachable. **Adding `MaxHeight` remains not a
  one-line change** — a bounded list becomes its own clipping ancestor and is NOT a scroll
  container unless it is given one, so `Reachable` would correctly call its overflow unreachable.
  The floor is 960 (900 → 960 with the conflict-rule line) to reduce scrolling in the common case,
  which is comfort and not correctness.
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
- **A compute error is a VALUE, and what happens to it depends on the POSITION, not on how it
  arrived** (`entitysdk/axis1/contain.go`, AP43). Two representations exist at every result site —
  *minted* (the evaluator raised it) and *value-form* (a `compute/error` entity arrived as an
  ordinary result), and §2.4 forbids them taking different paths. Three positions, and each call
  site in `axis1/eval.go` names its own: **CONSUMED** — the result is READ (arith/compare/logic
  operand, `if` condition, cast value, field target, construct field, index and its array, any
  collection operand, a filter predicate) → both short-circuit, via `evaluator.operand`;
  **CONTAINED** — the result is PLACED without being read (`map`'s output element, `fold`'s
  accumulator and `initial`) → both become a value in that slot, except `budget_exhausted` /
  `cascade_limit`, whose counters are not restored on unwind (`depth` *is*, so `depth_exceeded`
  contains like anything else); **BOUNDARY** — a contained error element materializes
  **code-only**, because `message` is prose no spec pins and containing it forks the array's bytes
  cross-impl. Getting the split wrong is silent under any test whose closures cannot fail — the
  filter half of the defect **kept** elements whose predicate had failed rather than erroring.
  Gates: `TestAxis1Equivalence_ContainedErrorPositions` (one vector per position) and, the real
  one, **arch's differential corpus below.**
- **`TestAxis1Admission_*` SKIPS unless you set `AXIS1_ADMISSION_CORPUS`, so a green
  `make test-sdk` says NOTHING about AE-5** — and a skip counts as a failure (AGENTS-STANDARD).
  This is the gate that matters and it is not wired into any target. Run it:

  ```
  (cd ../entity-core-go && go run ./cmd/internal/compute-corpus generate --profile inproc --out /tmp/c.cbor \
     && go run ./cmd/internal/compute-corpus emit --corpus /tmp/c.cbor --out /tmp/ref.cbor)
  (cd entitysdk && AXIS1_ADMISSION_CORPUS=/tmp/c.cbor AXIS1_ADMISSION_OUT=/tmp/a1.cbor go test -run TestAxis1Admission .)
  (cd ../entity-core-go && go run ./cmd/internal/compute-corpus cross-bless --corpus /tmp/c.cbor \
     --emission /tmp/ref.cbor --emission /tmp/a1.cbor)
  ```

  **Measured 2026-08-25 (corpus `8d2f55c8`, 362 vectors, core-go `13a42ea`): 334 agree, 0 diverge,
  28 INCOMPLETE — NOT LOCKED.** Every vector Axis-1 *answers* is byte-identical to the reference;
  the 28 deopt to Stage-1, and under **AE-6 a deopted vector's evidence is void**, so **AE-5 is not
  green and has not been since the v3.24/v3.25 primitives landed.** Do not quote the 2026-07-23
  admission as current. The corpus names the divergence-prone classes in its vector IDs
  (`cv8a-map-contains-minted-error`, `cv8c-filter-predicate-error-shortcircuit`,
  `cv9a-map-depth-exceeded-contains`, `worked/value-error/*`) — **it catches the AP43 bug in five
  vectors**, measured by re-running it against the pre-fix engine. Our home-grown 300-case fuzz
  found three cases and no cause; this corpus names them. **Reach for it first.**
- **Nothing in this repo starts the tree→disk direction of a `local/files` mount** (M1 finding,
  2026-09-01). `localfiles.Handler.StartReverseWrite` has **one** non-test caller in either tree
  and it is the kernel's own `cmd/entity-peer`. Our peers replicate through the subscription chain
  into `local/files:write` instead, so a mount here is watcher-ingest plus dispatched writes — not
  the kernel's reverse-write loop. Do not read the kernel's *"bidirectional"* as a statement about
  us; it was, in a doc of ours, for a day. **A capability that exists in a dependency is not a
  capability of your product until something in your tree calls it** — D23 aimed one layer out,
  and the D23 sweep does not look for it because there is no unreached model of *ours* to find.
  The reverse-write loop's own loop guard is a five-second clock with a live correctness defect;
  the reproducer is `workbench/localfiles_reverse_window_test.go` and the finding that matters if
  you touch this area is that **the content check the clock is standing in for already exists**,
  and `markWritten` sits downstream of it, so the clock can only arm when it is harmful. Read the
  file header before re-deriving any of it.
- **Not the conformance team.** When a cross-impl wire bug surfaces during perf/feature
  work, capture `file:line` + reproducer and route it (Python encoder → Python team, spec
  ambiguity → arch, conformance test-gap → core-go) — don't extend the probe into a
  validation harness. That's core-go's `validate-peer`.
