# Testing & sweeps

How to get a true reading of the tree's state, which suites lie to you and why, the podman collider, the load-dependent failures, and the harness gates that reach real input.

> Memory, per `AGENTS.md`. Every entry below was paid for by a defect that shipped or
> a measurement that contradicted what we believed. Find by symptom; the mechanism is
> stated before the fix, and `file:line` references point at the code, not at prose.

---

- **`make test-each` is the target to reach for when you want to know the state of the
  tree.** It runs every suite **to completion** regardless of failures, prints a pass/fail
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
- ⏳ **ONE FIXTURE IN THIS TREE EXPIRES ON A WALL CLOCK, and it went off on 2026-09-20.**
  `fetch/testdata/crossimpl-rust-federation/` is another implementation's frozen emission whose
  bindings carry a real **30-day TTL**; it was cut on **2026-08-21**, so from 2026-09-20 the four
  `shellcmd` browse tests (`TestShellOpenPrintsThePageAndTheChain`, `…WhereReportsTheAuthority`,
  `…SitesIsTheSignedKeySet`, `…BackCrossesPublishers`) fail at `binding freshness` **and will keep
  failing until somebody re-cuts the fixture** — it does not heal, and re-running proves nothing.
  So **the sweep baseline is 8 failures in 2 suites, not 4**: the documented 4 (three `entitysdk`
  continuation reds plus `shellcmd`'s `TestInstallRevisionMirrorChain_…`, tracked as core-go row
  23) *plus* these four. Check the date before you attribute any of the four to your diff.
  **The guard is already the right shape** — `requireFixtureUnexpired` (`shellcmd/cmd_browse_test.go`)
  turns the rot into a message naming the fix, and the expiry check itself is measured on a clock
  it controls in `fetch`'s `TestFedExpiredBindingIsRefused`, so nothing here is untested. What is
  missing is **a source of re-cuts**: their `dist-federation/` is a gitignored build artifact, so
  no commit anywhere contains these bytes and the repair needs the counterpart to emit one.
  Routed 2026-09-20. **The general shape worth carrying: a fixture with a TTL is a test that
  passes today and fails on a date nobody wrote down** — and the one who finds it first is
  usually a stranger who just cloned the release.
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
- **`Show()` is not a layout pass, and a headless test that skips one cannot see container
  teardown** (AP46's transferable half). `BrowserPanelTests` mounted a `Window`, called `Show()`,
  populated a list and cleared it — and never realized a container, so the crash path was
  *unreachable* from the suite while it reported 74/74. If a test's assertion depends on
  virtualization, recycling, or anything a panel does when items go away, call `UpdateLayout()`
  between the populate and the clear (`BrowserPanel.Settle` now does this for every browser test).
  Generalise: **constructing the world is not running it** — cf. AP36, where writing an input port
  and calling `tickOnce()` yourself could not see the queue.
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
- **A derived UI property is not a completion signal** (AP32). A headless test that settles on
  "the button re-enabled" returns in the window between the bridge call returning and the
  goroutine entering the operation, and then asserts against an empty view — green, measuring
  nothing. Wait on the bridge handle's monotonic `ops` counter instead.
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
- **Perf measurement:** `modernc.org/sqlite` under `-race` is ~17× slower — perf benches
  must override the default flags (`GOTEST_FLAGS="-count=1"`); never trust SQL bench numbers
  taken with `-race`. To reconcile store/index count discrepancies, `SqliteStore.DB()` lets
  you `SELECT` the `entities` table directly.
