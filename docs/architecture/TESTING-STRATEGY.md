# Testing strategy — entity-workbench-go

Canonical. Living doc — edit in place.

This doc names the test tiers, what each tier proves (and doesn't prove),
and the gate rules per change type. Charter `DISCIPLINE-CHARTER.md`
disciplines D10 (real-session coverage) and D16 (test-depth honesty)
are operationalized here.

The hard lesson from the Phase I crash-hunt: "tests pass" was a true
statement about the wrong tier for three commits in a row (AP4, AP5,
AP6). Naming the tier closes that gap.

---

## 1. The four tiers

| Tier | Boots | Tests | Failure class caught | Speed |
|------|-------|-------|----------------------|-------|
| **Go unit / package** | Single Go package | Pure logic | Algorithmic bugs, contract round-trips, parser/decoder behavior | Fast (~50 s full sweep with race) |
| **C# unit** | No Avalonia app | Pure C# logic (parsers, DTO shaping, view-model state) | C#-side logic bugs | Fast (~5 s) |
| **Avalonia headless** | Avalonia app with `UseHeadlessDrawing=false` + `UseSkia()` | Real bridge, real Skia in-process, no X11 | Bridge contract bugs, panel lifecycle bugs, GC-pinning bugs, render-pipeline contract bugs | Medium (~3 min full sweep) |
| **Xvfb smoke** | Real `entity-avalonia` binary inside Xvfb virtual framebuffer | Real X11, real Skia paint pipeline, real font shaping, real window manager | X11 paint bugs, font/HarfBuzz bugs, real-dispatcher races, HiDPI bugs | Slow (~15 s minimum + screenshot) |
| **Xvfb stress** *(within smoke)* | Same as smoke, driven by `SmokeDriver` (ingest docs, cycle paths) | Multi-cycle render under real X11 | Accumulation bugs, race classes that only appear under repeated stress | Slow (proportional to cycle count) |

Each tier catches something the cheaper tier cannot. **None
substitutes for the next.**

---

## 2. What each tier proves — and explicitly does not

### Go unit / package (`make test`)

**Proves:**
- Substrate behavior (entitysdk, shellcmd, shellboot logic).
- Bridge JSON envelope contracts on the Go side.
- Protocol round-trips.

**Does not prove:**
- Anything about Avalonia, .NET, Skia, or the rendering pipeline.
- That the bridge actually loads in a C# host.
- That panels render correctly.

### C# unit (`make test-cs-unit` or similar)

**Proves:**
- C#-side parsing (e.g., `MarkdownRendererTests`).
- DTO transformations.
- ViewModel state machines.

**Does not prove:**
- Anything that requires the Avalonia app to be running.
- Bridge calls (those need at least headless).

### Avalonia headless (`cd avalonia && make test`)

**Proves:**
- The bridge loads, peer boots, panels mount.
- Wake callbacks fire and re-render.
- GC-pinning of delegates works (`MarkdownViewPanelStressTests`).
- Panel render contracts hold on synthetic inputs
  (`AdaptiveRenderTests`).
- Click-around flows work on real docs
  (`RealMarkdownClickAroundTests`, `ActualArchitectureDocsTests`).
- Tree expand/collapse, search, log filter, etc. behave.

**Does not prove (the AP5 trap):**
- That Skia's real paint pipeline survives the load.
- That X11 message-loop interleaving doesn't race the render.
- That HarfBuzz font shaping handles the input.
- That real-display scaling / HiDPI works.

**Why:** `UseHeadlessDrawing=false` + `UseSkia()` is closer to real than
the pre-`f25e7cb` setup (which stubbed Skia entirely), but Avalonia's
**headless windowing platform** is still a stub. There is no X11
message loop, no real WM interaction, no real DPI. Several bug classes
live in exactly that gap.

### Xvfb smoke (`cd avalonia && make smoke-xvfb`)

**Proves:**
- The real `entity-avalonia` binary boots under X11 (Xvfb).
- The chrome renders (tab strip, panels, shell input).
- The peer system comes up via the bridge.
- Idle paint doesn't crash.
- Clean shutdown via `WB_SMOKE_EXIT_AFTER_SEC`.
- Screenshot artifact captures regressions visibly.

**Does not prove (yet):**
- That rapid panel switching works.
- That ingestion + content browsing works.
- Wayland-specific paths (Xvfb is X11-only).

### Xvfb stress (`cd avalonia && make smoke-xvfb-driver`)

**Proves:**
- The driven scenario (ingest → cycle paths → quit) survives N cycles.
- Per-frame screenshots in `xvfb-smoke-out/frames/` capture pre-crash
  state on failure.
- The HiDPI variant (`make smoke-xvfb-hidpi`) amplifies accumulation
  bugs (~1 iter vs ~18 at default res).

**Does not prove:**
- Anything until the accumulation bug (PHASE-I-RELIABILITY-PLAN
  follow-on #4) is understood and fixed. Currently *finds* the bug;
  doesn't *certify against* it.

---

## 3. Tier nomenclature in commit messages and PRs

The charter D16 enforcement: any claim of "passes" or "fixed" **must**
name the tier.

| Phrase | Means |
|--------|-------|
| "passes" *(unqualified)* | Smell — ask the author which tier. Likely Go unit only. |
| "passes unit" | Go / C# unit only. **Not enough** for any change touching the renderer or the bridge. |
| "passes headless" | Avalonia headless tier green. Sufficient for C# logic and bridge contract changes. **Not enough** for render pipeline changes. |
| "passes headless + xvfb-smoke" | Both tiers. Sufficient for most renderer changes (chrome, panel mount, basic interaction). |
| "passes headless + xvfb-smoke + xvfb-driver" | All three real tiers. Required for changes to the render pipeline (`MarkdownViewPanel.SwapBody`, any new P1 adaptive emit). |
| "passes nightly stress" | Has survived a 24h cycled-render stress run. Aspirational; we don't have this gate today. |

The intent is not bureaucratic — it's that **the reader knows what
the test result actually means.**

---

## 4. Gate rules per change type

| Change touches | Required tiers |
|-----------------|----------------|
| `entitysdk/`, `shellcmd/`, `shellboot/` (Go-only) | Go unit |
| `avalonia/bridge/` (Go-side bridge) | Go unit + Avalonia headless |
| `avalonia/frontend/*.cs` (non-panel C# logic) | Avalonia headless |
| `avalonia/frontend/Panels/*.cs` (panel render path) | Avalonia headless + xvfb-smoke |
| `avalonia/frontend/Panels/MarkdownViewPanel.cs` *or* any new P1 adaptive emit | All three (headless + xvfb-smoke + xvfb-driver) |
| `avalonia/Containerfile`, `avalonia/Makefile` (build/infra) | xvfb-smoke (proves the container still boots) |

**These are minimums.** Always run the tier that would have caught the
class of bug you're fixing.

---

## 5. The viewport-driven test idea (Godot's leverage, our gap)

The Godot sibling project has `Viewport.push_input(event)` — synthetic
input that travels the **same chain** real OS events do. This catches
input-routing, focus, and modal-precedence bugs that handler-bypass
tests miss.

Avalonia has analogous APIs (`InputManager`, `KeyboardDevice`,
`MouseDevice`, dispatched `RawInputEventArgs`). We have one click-test
fixture (`RealMarkdownClickAroundTests`) but no formal layer that
"all UI tests use synthetic dispatched input, never call handler
methods directly."

**Status:** open work. The current headless tests sometimes call
`panel.OnClick(...)` directly. The discipline is to push the
synthetic event through Avalonia's input pipeline so routing is
exercised.

**[DEEP-DIVE PENDING]** — the right input-injection layer for Avalonia
headless tests. Worth investigating before the next round of UI tests.

---

## 6. Real-fixture rule

The AP6 trap (`1467985`): synthetic test docs didn't match the docs
users actually read.

**Rule:** real-bug-classes get real fixtures. Specifically:

- `ActualArchitectureDocsTests` ingests `docs/architecture/*.md` —
  the actual docs the user opens. Run on every PR.
- Synthetic fixtures (e.g., `Big_Doc_Emits_In_Multiple_Batches` —
  600-section synthetic) are for **pinning thresholds**, not for
  proving feature correctness.
- New panels rendering domain content add a real-fixture test that
  loads from `docs/`, the user's `~/.config/entity-shell/`, or
  similar.

---

## 7. Failure-aware artifact capture

Tests must leave forensic evidence on failure.

- Xvfb runs capture one frame per 2 s into `dist-native/xvfb-smoke-out/frames/`.
  All frames survive on disk; the last captured frame survives a crash.
- Headless tests pipe stderr breadcrumbs (`WB_PANEL_LOG=1`) into the
  test output stream on failure.
- Go-side test failures include the surrounding `make test` output
  (race detector, etc.).

**Owed:** structured "test failed → dump last 100 PanelLog lines" hook
on the C# side (mirror of Godot's `DebugLog.start_capture` +
`drain_captured` pattern from `TESTING-STRATEGY.md §F`). Today we
read the breadcrumb manually from stderr; the dump-on-failure
automation would close the loop.

---

## 8. Current test inventory

(Snapshot — `ls avalonia/tests/Workbench.Headless.Tests/` for current.)

| File | Tier | What it pins |
|------|------|--------------|
| `MarkdownRendererTests.cs` | C# unit | Markdown → inline parsing correctness |
| `BridgeFixture.cs` | (test infra) | Shared bridge bootstrap for Avalonia tests |
| `TestAppBuilder.cs` | (test infra) | Avalonia headless app config |
| `HeadlessPump.cs` | (test infra) | Dispatcher pump helper |
| `AdaptiveRenderTests.cs` | Headless | The three layers of `SwapBody` (small / big / above-ceiling) |
| `MarkdownViewPanelStressTests.cs` | Headless | Wake-callback GC-pinning under stress |
| `TreeViewPanelSmokeTests.cs` | Headless | Tree mount / expand / search |
| `TreeViewPanelRegressionTests.cs` | Headless | Specific TreeView bug pins |
| `RealMarkdownClickAroundTests.cs` | Headless | Real-doc click flow |
| `ActualArchitectureDocsTests.cs` | Headless | Real `docs/architecture/*.md` ingest |
| `ClickAroundStressTests.cs` | Headless | Rapid click-cycle synthetic |
| `run-xvfb-smoke.sh` + `make smoke-xvfb` | Xvfb smoke | Boot + chrome + clean exit |
| `make smoke-xvfb-driver` | Xvfb stress | Ingest + cycle paths (currently finds #4) |
| `make smoke-xvfb-hidpi` | Xvfb stress | HiDPI amplification |

---

## 9. What this strategy does NOT do

- It does not specify which test framework lives where (xUnit / NUnit /
  Go's `testing`). That's already chosen.
- It does not mandate watch-mode / visual test reviews. Useful but
  optional.
- It does not extend to the `entity-shell` / `entity-console`
  renderers' UI tier — they have different harnesses
  (TUI snapshot tests in console, shell command tests in shellcmd).
  This doc is **Avalonia-tier**.

---

## 10. Pinning

Linked from:

- `DISCIPLINE-CHARTER.md §6` — D16 enforcement table cites this doc.
- `MODEL-AVALONIA-RUNTIME.md` — the boundary map's "forensic record"
  rows quote which tier caught what.

Edit in place when new tiers are added or gates change.
