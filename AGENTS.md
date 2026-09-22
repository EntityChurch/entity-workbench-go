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
  D1–D27, the ten review questions, the anti-pattern catalog AP1–AP76, and the promotion
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
  `make gui-run ARGS="--identity me --storage sqlite"` (double-dash: the .NET frontend does
  not use Go's `flag` spelling). **With no flags the GUI is an ephemeral in-memory peer and
  loses everything on exit.** `avalonia/README.md` is the full entry-path doc.
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
  questions, and anti-pattern catalog (AP1–AP76) in `docs/architecture/DISCIPLINE-CHARTER.md`.
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
  **`Mode: both` DOES NOT RUN RECEIVER → OWNER, measured 2026-09-06 by `make threepeer-sync`.**
  Three arms — one side declaring it, both sides declaring it, and both sides with *symmetric*
  root names — and the receiver's write comes back in none of them. The third arm is the
  discriminator and it **refutes** the obvious hypothesis: the two-root-names trap is not the
  cause, because asymmetric and symmetric behave identically. The declaration *takes* (the owner
  reports `mode=both`) and **the owner holds no sync binding naming the receiver**, so the
  owner-side receive leg is never established. `receiveFromPeers` (`shellcmd/reconcile.go`) says a
  local folder that `Receives()` pulls from every peer whose state is `Accepted` — start there.
  Note the verb takes a folder-id of `{owner-peer-id}.{sender-root}`, so an operator who accepted
  into a directory of their own choosing sees an id built from a root they never typed.
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
- **THE FIRST CHANGE AFTER EITHER PEER RESTARTS IS NOT DELIVERED, AND THE CAUSE IS UNKNOWN.**
  Measured 2026-09-03, both directions, no error on either side, `status` reporting `settled`
  throughout: restart a peer, and the next file changed never arrives while every one after it
  does. It is not a delay — the change is gone, and only `resync` recovers it. **Two obvious
  hypotheses are already refuted** (a missing outbound dial; a stale entry in our own pool — the
  evict-then-dial change was REVERTED rather than kept, because a cost justified by a dead
  hypothesis is not a fix). Read
  `docs/architecture/reviews/FIRST-CHANGE-AFTER-RESTART-IS-LOST-2026-09-03.md` before touching
  this — §4 lists what has not been ruled out, and the first question to answer is whether the
  loss is on the send side or the receive side, which nobody has instrumented.
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
  already the provenance discriminator conflict detection needs** — a local edit arrives
  through the WATCHER, a delivered one through `blob_resolve`'s dispatch — so "they edited
  this" and "I am behind" are distinguishable today, at zero cost, and that distinction is
  what the whole milestone turns on. Known limit, invisible from the field name: a local
  caller dispatching `local/files:write` directly records as a delivery; nothing in the
  shipped flow does that. So M3 is **not** "build a merge engine": the tree-side guarantee is
  already met and had simply never been switched on. **`shellcmd/folder_history.go` switches
  it on**, as a derived output of the reconciler — a folder that `Receives()` and is mounted
  gets a config for its mount prefix. `Receives()` and **not** `IsLocal()`, and here the two
  genuinely differ: `share` declares the OWNER's folder `both`, so either side of a shared
  folder can be overwritten and both record, while keying on `IsLocal()` would leave the owner
  — whose files these actually are — as the one side with no chain. A **send-only** folder
  gets none, because one writer means an entity per save forever answering no question. What
  is left of M3 is: branch at `blob_resolve.go`'s existing F9 *different* arm, write keep-both
  under the spec's `{path}.keep-both-{hash8}`, then layer `EXTENSION-REVISION`. This is D20
  aimed at the kernel one more time: **grep `../entity-core-go` for the mechanism before
  pricing the build.**
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
- **`publish/` (the CDN corridor) emits a real signed root as of 2026-08-18.** `{out}/manifest`
  is the signed `system/peer/published-root`; the http-poll transport profile moved to
  `{out}/transport-profile` (§6.5.4 / D5); the §6.5.3 closure of `root_hash` is uploaded in full.
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
  - **`transports` on a §3 binding has two live readings and they do not interoperate.** core-rust
    emits an inline endpoint object, core-go's `BindingData` says `[]hash.Hash`, and core-go's
    backend therefore **cannot decode any binding in the cohort's only live federation**. Ours
    reads both and keeps them distinguishable (`TransportRef.Kind`) rather than normalizing.
    Routed: `reviews/REGISTRY-BINDING-TRANSPORTS-DIVERGENCE-2026-08-21.md`. **Do not "fix" this by
    making our SDK succeed where the reference implementation fails** — that hides it in our tree.
  - **The walk is the authority; a served listing is a menu** (§6a.3a). `Registry.Enumerate` walks
    the signed root over the `by-name/` prefix and reports both sets *and their disagreement*.
    The spec authors' standing ask: say which one produced a row **in the artifact**, not only
    in the code.
  - **An origin-relative transport prefix resolves against a scheme://host:port, never against the
    path the profile was fetched under** (`fetch.OriginRoot`). A registry served at `host/registry`
    names domains at `host/docs`.
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
