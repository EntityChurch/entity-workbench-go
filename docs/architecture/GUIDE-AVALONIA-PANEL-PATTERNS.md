# Guide — Avalonia panel patterns

Canonical. Living doc — edit in place.

The recipes. The charter (`DISCIPLINE-CHARTER.md`) names the rules;
the runtime model (`MODEL-AVALONIA-RUNTIME.md`) names what the platform
does. **This doc names how to write a panel that respects both.**

Every pattern is grounded in working code. The reference implementation
is named per pattern. New panels should lift the reference shape rather
than re-invent.

---

## 1. The standard panel shape

Every panel in `avalonia/frontend/Panels/` follows the same skeleton:

```
public sealed class FooPanel : UserControl, IDisposable
{
    private long _peerHandle;      // injected at construction
    private long _handle;          // returned by FooOpen
    private Bridge.WakeCallback _wakeCallback;  // pinned via GCHandle
    private GCHandle _wakeCallbackHandle;       // explicit pinning

    public FooPanel(long peerHandle) { ... }

    public void Mount() {
        _handle = FromBridge(Bridge.FooOpen(_peerHandle));
        _wakeCallback = OnWake;
        _wakeCallbackHandle = GCHandle.Alloc(_wakeCallback);
        var cbPtr = Marshal.GetFunctionPointerForDelegate(_wakeCallback);
        FromBridge(Bridge.FooRegisterWake(_handle, cbPtr));
        Render();
    }

    private void OnWake(long handle) {
        // Called from Go's wake goroutine — marshal to UI thread.
        Dispatcher.UIThread.Post(Render);
    }

    private void Render() { ... }

    public void Dispose() {
        if (_handle != 0) Bridge.FooClose(_handle);
        if (_wakeCallbackHandle.IsAllocated) _wakeCallbackHandle.Free();
    }
}
```

That's the structure. The patterns below are the discipline applied
to the body of `Render()` and the surrounding lifecycle.

**Reference: every panel in `avalonia/frontend/Panels/`.**

---

## 2. P1 — Adaptive emit (chunked render with backpressure)

**When to use:** rendering content that scales with input size — markdown
documents, log streams, large lists, query results.

**The rule (D13 + D15):** never block the UI thread for more than ~one
frame. Emit work in batches; yield the dispatcher between batches at
`Background` priority. Adapt batch size to hold a frame budget.

**Shape:**

1. Parse / fetch off the UI thread (or quickly on it).
2. **Pre-compute** the structural split: how many blocks / rows /
   chunks will exist, and where the boundaries fall. Do this *before*
   touching the visual tree.
3. **Pre-allocate** all blocks / row containers. Add them to a
   **persistent** parent container (not a fresh one each render).
4. **Emit** content into the pre-allocated containers in batches. After
   each batch, `Dispatcher.UIThread.Post(EmitBatch, Background)`.
5. **Adapt** batch size: if a batch ran > 20 ms, halve next batch
   (floor 50). If < 6 ms, double (ceiling 1000). Else hold.
6. **Cancel** on a new render: every render carries a
   `CancellationTokenSource`; starting a new render cancels the
   in-flight one before clearing.

**Why each step:**

- Pre-compute + pre-allocate: mutating visual-tree *structure*
  (Add/Remove `Children`) during a Background-priority emit interleaves
  with Avalonia's Render-priority measure pass and crashes Skia. Only
  mutating `Inlines` on already-attached blocks is safe. Headless paint
  hides this; the Xvfb driver caught it.
- Persistent container: recreating the `ScrollViewer` / `StackPanel`
  per render is what bit us — the detach/reattach raced the paint
  pipeline.
- Cancellation: rapid path clicks should not stack up multiple
  in-flight emits competing for the dispatcher.

**Reference:** `avalonia/frontend/Panels/MarkdownViewPanel.cs:304-501`
(`SwapBody` + `EmitBatch`).

**Companion safety net (P1.5):** above an absolute byte ceiling
(MarkdownView uses 1 MiB), bypass the rich path entirely and attach a
plain `TextBox` with a banner. Loud, reversible degradation.

---

## 3. P2 — Persistent container + recycle

**When to use:** any panel that re-renders on wake (almost all of them).

**The rule (D13):** the outermost container is created once at panel
construction and **never** detached / reattached. Only its children's
collections mutate. The `ScrollViewer` in particular survives every
re-render.

**Shape:**

```csharp
private ScrollViewer _bodyScroll;   // created in InitializeComponent, NEVER replaced
private StackPanel _bodyStack;      // child of _bodyScroll, NEVER replaced

private void Render() {
    _bodyStack.Children.Clear();    // OK — mutating the stack's collection
    foreach (var item in items) {
        _bodyStack.Children.Add(BuildRow(item));
    }
}
```

**Anti-pattern (don't do this):**

```csharp
private void Render() {
    var scroll = new ScrollViewer { Content = new StackPanel { ... } };
    Content = scroll;   // DETACH old, ATTACH new — races the paint pipeline
}
```

**Reference:** `MarkdownViewPanel.cs:419-432` (`_bodyScroll` /
`_bodyStack` persistent, only `Children.Add` runs per render).

**P2 companion — in-place item replace.** When the row set is
identical render-to-render (the common steady-state case), replace
items in place via `_rows[i] = newVm` instead of `_rows.Clear() +
foreach Add`. Replace fires `NotifyCollectionChanged.Replace` per
index; Clear+Add fires `Reset`. `Reset` makes the ListBox discard
every recycled container and re-template from scratch. Replace lets
the ListBox keep the recycled containers. Reference:
`MarkdownFilesPanel.RowsPathSetMatches` + the diff path in
`RerenderFromBridge`; `TreeViewPanel` uses the same shape.

---

## 4. P3 — Wake debounce / coalesce

**When to use:** panels whose underlying data fires many wake events
in rapid succession (a log viewer during a burst; a tree during a
roster sync).

**The rule (D13):** one render per dispatcher tick is enough; don't
queue ten if ten wakes fire in one frame.

**Shape:** check if a render is already pending; if so, drop the new
wake. `Dispatcher.UIThread.Post` with a guard flag, or a `Volatile`
counter, or an explicit timer.

```csharp
private int _renderPending;  // 0 = idle, 1 = render queued

private void OnWake(long handle) {
    if (Interlocked.CompareExchange(ref _renderPending, 1, 0) != 0) return;
    Dispatcher.UIThread.Post(() => {
        Volatile.Write(ref _renderPending, 0);
        Render();
    });
}
```

**Two flavors in use:**

- **Single-flight guard** (`_renderQueued`-style): catches the
  multiple-wake-in-one-dispatcher-tick case. Cheap. Doesn't help if
  the wakes arrive across separate ticks. `LogViewerPanel`,
  `PeerInfoPanel`.
- **Time-based debounce** (`DispatcherTimer`): coalesces a burst of
  wakes spread over an N-millisecond window into a single render at
  the end of the burst. Use for bursty wake sources (ingest, roster
  sync). 150 ms is the current default for live-feeling panels.
  Reference: `MarkdownFilesPanel.OnWakeFromGo` /
  `TreeViewPanel.OnWakeFromGo` (both stack the timer on top of
  single-flight; the 400 ms `MarkdownView.LoadPathDebounce` is the
  longer-window variant for user-driven path clicks).

**Reference:** `MarkdownFilesPanel.cs` and `TreeViewPanel.cs` for
the time-debounce shape; `MarkdownViewPanel.LoadPathDebounce` for
the user-input shape.

---

## 5. P4 — Bounded list

**When to use:** any panel rendering a list whose length is determined
by external data (entities, paths, log lines, query results, peer
roster).

**The rule (D15):** no unbounded `ObservableCollection`. No
`ItemsControl` without virtualization. No `Children.Add` in a loop
without a cap.

**Pick one explicitly:**

- **(a) Virtualize:** `ItemsControl` with `VirtualizingStackPanel`.
  Cheapest if the source allows random access. Avalonia's virtualization
  has its own gotchas (item size estimation, scroll-restore). **[DEEP-DIVE
  PENDING]** before recommending unconditionally.
- **(b) Per-block cap:** like MarkdownView's `MaxInlinesPerBlock = 500`.
  Use when the items are inline (no inherent row structure).
- **(c) Top-N + paginate:** show first N, "More..." button or scroll-to-load.
  Best when ordering is meaningful and the user reaches the bottom rarely.
- **(d) Server-side `HasMore`:** the bridge returns a truncated list
  with a `HasMore` flag; the client renders a "showing N of many"
  banner. Already in `QueryBrowserPanel` DTO (line 289) — just needs
  client-side cap wired up.

**Anti-pattern:** `ObservableCollection.Clear() + foreach { Add }` over
unbounded data, with no cap. **Resolved** in
`LogViewerPanel` (cap 1000, drop oldest), `PeerInfoPanel` (cap 1000,
show first), `QueryBrowserPanel` (1000-row defensive cap below the
server-side HasMore pagination).

**Reference:** `MarkdownViewPanel` per-block split as (b);
`LogViewerPanel.MaxDisplayRows = 1000` as (b) tail-truncated;
`QueryBrowserPanel.MaxClientRows = 1000` plus DTO `HasMore` flow
as (d) — the page-server is upstream of the visible row cap.

---

## 6. P5 — Differential update

**When to use:** panels rendering ordered lists where most items don't
change render-to-render (tree views, peer rosters).

**The rule (D13 + D15):** don't clear-and-rebuild when you can diff.
Clear-and-add forces Avalonia to remeasure / rearrange / repaint the
entire list. Diff-and-patch lets it touch only what changed.

**Shape (sketch):**

```csharp
private void Render(IReadOnlyList<RowDto> newRows) {
    // Trivial cases first.
    if (_lastRows.Count == 0) { FullRender(newRows); return; }
    if (newRows.Count == 0) { _stack.Children.Clear(); _lastRows = newRows; return; }

    // Identity-keyed diff: prefer (a) ordered-keys-match (just update
    // contents in place), (b) longest common subsequence (move minimal).
    // For most panels (a) is enough: a wake usually changes contents,
    // rarely the row set.
    if (RowsAreSameSet(newRows, _lastRows)) {
        for (int i = 0; i < newRows.Count; i++) UpdateRow(_stack.Children[i], newRows[i]);
    } else {
        FullRender(newRows);  // fallback
    }
    _lastRows = newRows;
}
```

**Reference:** `TreeViewPanel.RerenderFromBridge` — when
`RowsPathSetMatches` returns true (the common steady-state case),
replace items in place; otherwise fall back to clear-and-rebuild.
`MarkdownFilesPanel` uses the same shape against its own DTO.
Cheap `SameDisplayAs` equality on the VM keeps no-change indices
from firing a `Replace` event at all.

---

## 7. P6 — Pinned delegate (cross-language lifetime)

**When to use:** every time a C# delegate is handed to Go via
`Marshal.GetFunctionPointerForDelegate`. **Mandatory** for every wake
registration.

**The rule (D12):** every delegate handed to Go is **`GCHandle.Alloc`-
pinned** for the lifetime of Go's use. Free only after the goroutine
has joined (which happens inside `FooClose` on the Go side).

**Shape:**

```csharp
private Bridge.WakeCallback _wakeCallback;
private GCHandle _wakeCallbackHandle;

public void Mount() {
    _wakeCallback = OnWake;                                // store the delegate field, not just a lambda
    _wakeCallbackHandle = GCHandle.Alloc(_wakeCallback);   // pin it
    var cbPtr = Marshal.GetFunctionPointerForDelegate(_wakeCallback);
    Bridge.FooRegisterWake(_handle, cbPtr);
}

public void Dispose() {
    if (_handle != 0) Bridge.FooClose(_handle);            // Go joins its wake goroutine
    if (_wakeCallbackHandle.IsAllocated) _wakeCallbackHandle.Free();
}
```

**Why every line matters:**

- Storing `_wakeCallback` as a field (not just `Marshal.GetFunctionPointerForDelegate(OnWake)`) keeps a managed reference; otherwise the delegate is born and dies in the same expression.
- `GCHandle.Alloc` is belt-and-suspenders: even if the field reference is somehow lost (refactor, future code), the GCHandle keeps it pinned.
- `FooClose` **must** be called before `GCHandle.Free` — Go joins its wake goroutine inside `Close`; if we free the handle first, the goroutine's last call dereferences a freed delegate → SIGSEGV.

**Anti-pattern (AP2 — `958b3fe`):** passing `Marshal.GetFunctionPointerForDelegate(OnWake)` directly without keeping a strong reference. The delegate is GC'd between mount and first wake; the next callback SIGSEGVs.

**Reference:** `MarkdownViewPanel.cs:170-171` (allocation),
`Dispose()` for the release order.

**Enforcement:** every panel with a wake callback **must** follow this
pattern. Cross-cutting audit (AP3 — `f95c0c6`) confirmed every panel
got fixed; new panels must lift the shape from a reference panel, not
write their own.

---

## 8. P0 — The diagnostic breadcrumb

**Not optional.** Every panel writes a breadcrumb at every long-running
op via `PanelLog.Write(category, message)`. When the process dies
(stack overflow, SIGSEGV in native code), the **last line of stderr
names the dying op**. This is how we get any forensic information at
all from a crash that bypasses managed exception handling (AP7 —
`8b3963e`).

**Shape:**

```csharp
PanelLog.Write("markdown-view", $"SwapBody h={_handle} bytes={markdown.Length}");
// ... work that might crash ...
PanelLog.Write("markdown-view", $"SwapBody h={_handle} parsed {inlines.Count} inlines");
```

Category names are pre-registered in `LOGGING-CONVENTIONS.md`. Don't
invent new ones inline.

**Reference:** every long-running op in `MarkdownViewPanel.cs`
(grep `PanelLog.Write`).

---

## 9. Cross-pattern table: where each panel stands today

**Scope (D18).** This table is Avalonia-specific by design. The
TUI (`entity-console`) consumes the same `wb.*Model` instances via
`console/*.go` but doesn't have an equivalent table — tview's
primitives (`TextView` scrollback, `TreeView` widget) don't share
Avalonia's failure modes (no compositor batch pipeline, no per-block
paint recursion, no GC-handle race across cgo). Patterns P1–P6
are presentation-tier responses to Avalonia's specific dispatcher
+ visual-tree + Skia stack; they don't transfer. See
`DISCIPLINE-CHARTER.md §1` for the renderer-asymmetry rationale.


| Panel | P1 adaptive | P2 persistent | P3 debounce | P4 bounded | P5 diff | P6 pinned | P0 breadcrumb |
|-------|-------------|---------------|-------------|------------|---------|-----------|---------------|
| `MarkdownViewPanel` | ✅ ref impl | ✅ ref impl | — (single-shot per LoadPath) | ✅ (b) | n/a (single body) | ✅ ref impl | ✅ |
| `TreeViewPanel` | n/a (small per-row payload) | ✅ | ✅ 150ms timer | n/a (filtered) | ✅ ref impl | ✅ | ✅ |
| `LogViewerPanel` | n/a (per-row) | ✅ | ✅ single-flight | ✅ (b) 1000 cap | n/a (append) | ✅ | ✅ |
| `MarkdownFilesPanel` | n/a (no big payload) | ✅ in-place replace | ✅ 150ms timer | n/a (small list today) | ✅ (companion) | ✅ | ✅ |
| `PeerInfoPanel` | n/a | ✅ | ✅ single-flight | ✅ (c) top-1000 | n/a | ✅ | ✅ |
| `QueryBrowserPanel` | n/a | ✅ | n/a (pull-only) | ✅ (d) HasMore + 1000 floor | n/a | n/a (no wake) | ✅ |
| `DetailPanel` | n/a | ✅ | n/a (externally driven) | n/a | n/a | n/a (no wake) | ✅ |
| `SiteViewPanel` | deferred (curated content; revisit if >5K-inline page surfaces) | ✅ | ✅ single-flight + 150ms timer | ✅ (b) 500/block | deferred (small sidebar/nav lists) | ✅ | ✅ |

**P2 criterion:** the outermost container (`DockPanel` / `Grid`) and
its persistent children (`ScrollViewer`, `ListBox`, named TextBlocks)
are constructed once in the panel constructor and never reassigned;
only child collections (`ObservableCollection<T>`) mutate. Every panel
above qualifies — `Content = ...` runs exactly once per panel.

The cross-panel reliability sweep (follow-ons #5–#9) has landed: each
panel's collection mutation is bounded and covered by the verified
test-tier trail.

---

## 10. Order of operations for a new panel

1. **Copy the standard skeleton** (§1) from a reference panel.
2. **Apply P6** (pinned delegate) if it has a wake.
3. **Apply P0** (breadcrumb) at every long-running op.
4. **Apply P4** (bounded list) if it renders any list whose length is
   data-driven. Pick (a/b/c/d) explicitly in the panel's comment block.
5. **Apply P1 + P2** if its render touches > ~100 elements or fetches
   payload >10 KB. Lift the shape from `MarkdownViewPanel.SwapBody`.
6. **Apply P3** if its wake source is bursty.
7. **Apply P5** if its renders are mostly-the-same lists.

The pre-merge checklist in `DISCIPLINE-CHARTER.md §3` runs the nine
review questions; this guide's patterns are how you answer "yes" to
questions 5 / 6 / 7 / 8.

---

## 11. Pinning

Linked from:

- `DISCIPLINE-CHARTER.md §8` — the reading order points here.
- `MODEL-AVALONIA-RUNTIME.md §6` — boundary A and B fixes use these patterns.

Edit in place. When a pattern is added or a reference moves, update
both the section and the cross-pattern table.
