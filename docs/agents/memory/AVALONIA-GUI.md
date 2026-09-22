# Avalonia GUI

Panel construction, the framework traps that shipped defects at full green, the render/DTO boundary, and which panel answers which question.

> Memory, per `AGENTS.md`. Every entry below was paid for by a defect that shipped or
> a measurement that contradicted what we believed. Find by symptom; the mechanism is
> stated before the fix, and `file:line` references point at the code, not at prose.

---

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
