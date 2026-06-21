using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Runtime.InteropServices;
using System.Text.Json;
using System.Text.Json.Serialization;
using System.Threading;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Controls.Documents;
using Avalonia.Layout;
using Avalonia.Media;
using Avalonia.Threading;
using SkiaSharp;

namespace EntityAvalonia.Panels;

// MarkdownViewPanel renders wb.MarkdownViewModel — selection-driven
// read-only view of doc/markdown-file entities. Subscribes to
// IPanelHost.SelectedPath; on each new path, calls
// Bridge.MarkdownViewLoadPath, which rebinds the per-path Store.Watch
// inside the bridge. Wake fires both on path change and on entity
// content mutation.
//
// Body rendering uses MarkdownRenderer (our own Markdig→Inlines
// transform). The rendered visual tree is a vertical stack of
// SelectableTextBlocks (per-block split keeps Skia's per-block paint
// recursion shallow), built adaptively across multiple dispatcher
// ticks (incremental attach keeps the UI thread responsive on huge
// docs). See the SwapBody comment block for the three-layer pipeline
// (safety net / per-block split / adaptive emit).
//
// We picked our own renderer rather than Markdown.Avalonia because
// the third-party library segfaulted under rapid content changes —
// the visual tree it built didn't release cleanly between renders,
// and Skia eventually crashed on the accumulated state.
//
// Edit mode (Ctrl+S save, Esc cancel) is on the underlying view model
// but not yet exposed through the bridge or this panel.
public sealed class MarkdownViewPanel : UserControl, IDisposable
{
    private readonly long _peerHandle;
    private readonly long _handle;
    private readonly IPanelHost? _host;
    private readonly Action<string>? _selectedPathHandler;

    private readonly TextBlock _header;
    private readonly TextBlock _pathLabel;
    private readonly Grid _bodyHost;
    private readonly TextBlock _placeholder;
    // Persistent body container — created once in the constructor,
    // never replaced. SwapBody mutates _bodyStack.Children rather
    // than detaching/reattaching the whole subtree, which avoids
    // the X11 paint-pipeline crash we hit when ClearBody+Add'd
    // a new ScrollViewer per render (caught by smoke-xvfb-driver
    // on a 723-inline reload).
    private ScrollViewer? _bodyScroll;
    private StackPanel? _bodyStack;
    // Fallback container (plain-text path). Independent of the
    // rich-body so the two paths don't fight over _bodyHost children.
    private Control? _fallbackContainer;
    private SelectableTextBlock? _body;

    // Test surface: counts how many times the body TextBlock was
    // (re)instantiated. Tests verify rapid LoadPath collapses to a
    // single body rebuild via debounce.
    internal int BodyRecreateCountForTests { get; private set; }

    private Bridge.TreeWakeCallback? _wakeCallback;
    // Explicit GC root — see TreeViewPanel for full rationale. Pinning
    // via GCHandle is the only thing that reliably prevents a GC'd-
    // delegate crash; the field reference alone isn't enough.
    private GCHandle _wakeCallbackHandle;
    private bool _renderQueued;
    private bool _disposed;

    // LoadPath debounce. Rapid tree selection (user clicking through
    // several docs quickly) was crashing the paint layer — each
    // LoadPath triggers a full Markdown.Avalonia tree rebuild, and
    // back-to-back rebuilds under Skia segfault in real use even
    // though the bridge plumbing is clean (proven via
    // MarkdownViewPanelStressTests). Holding LoadPath behind a
    // debounce means we only load the final path the user landed
    // on, not every intermediate path flashed past.
    //
    // Evening pivot — raised from 150ms to 400ms as a structural
    // mitigation for the open follow-on #4 accumulation crash. The
    // revised hypothesis (MODEL-AVALONIA-RUNTIME §4) points at
    // Avalonia's compositor/layout retention queue growing across
    // rapid renders. A longer debounce gives the compositor more
    // time to drain its serialization queue between renders. This
    // is the panel-level mitigation; the underlying compositor issue
    // remains under investigation.
    //
    // 400ms is empirical: long enough to outpace the smoke driver's
    // 150ms publish cadence (so the debounce actually catches
    // bursts) without being human-noticeable on real click-through
    // (a deliberate user click lands the path within 400ms anyway).
    private static readonly TimeSpan LoadPathDebounce = TimeSpan.FromMilliseconds(400);
    private DispatcherTimer? _loadPathTimer;
    private string? _pendingPath;
    private string? _lastLoadedPath;

    // Test surface: counts how many times LoadPath actually crossed
    // the bridge after debouncing. The fix's regression test asserts
    // this stays low under rapid Publish.
    internal int LoadPathCallCountForTests { get; private set; }

    public MarkdownViewPanel(long peerHandle, IPanelHost? host)
    {
        _peerHandle = peerHandle;
        _host = host;

        var openReply = Bridge.TakeString(Bridge.MarkdownViewOpen(peerHandle));
        _handle = ParseHandle(openReply);
        if (_handle < 0)
        {
            Content = new SelectableTextBlock
            {
                Text = $"markdown-view open failed: {openReply}",
                Foreground = Brushes.IndianRed,
                Margin = new Thickness(12),
                FontSize = 14,
            };
            return;
        }

        _header = new TextBlock
        {
            Text = "Markdown",
            FontWeight = FontWeight.SemiBold,
            FontSize = 14,
            Margin = new Thickness(0, 0, 0, 4),
            Opacity = 0.85,
        };
        _pathLabel = new TextBlock
        {
            Text = "(no file selected)",
            FontFamily = new FontFamily("monospace"),
            FontSize = 12,
            Opacity = 0.55,
            Margin = new Thickness(0, 0, 0, 8),
        };
        // Placeholder shown when nothing is selected / file is missing.
        // It's the only stable child of bodyHost; the MarkdownScrollViewer
        // is created on-demand per load and removed when there's nothing
        // to show.
        _placeholder = new TextBlock
        {
            Text = "Select a doc/markdown-file entity in the tree.",
            FontSize = 13,
            Opacity = 0.5,
            HorizontalAlignment = HorizontalAlignment.Center,
            VerticalAlignment = VerticalAlignment.Center,
            TextWrapping = TextWrapping.Wrap,
        };
        _bodyHost = new Grid();
        _bodyHost.Children.Add(_placeholder);
        // Persistent rich body — pinned to bodyHost from construction.
        // SwapBody mutates its StackPanel children; the ScrollViewer
        // itself is never detached from the visual tree mid-app.
        _bodyStack = new StackPanel { Orientation = Orientation.Vertical };
        _bodyScroll = new ScrollViewer
        {
            Content = _bodyStack,
            HorizontalScrollBarVisibility = Avalonia.Controls.Primitives.ScrollBarVisibility.Disabled,
            VerticalScrollBarVisibility = Avalonia.Controls.Primitives.ScrollBarVisibility.Auto,
            IsVisible = false,
        };
        _bodyHost.Children.Add(_bodyScroll);

        var dock = new DockPanel
        {
            LastChildFill = true,
            Margin = new Thickness(10),
        };
        DockPanel.SetDock(_header, Dock.Top);
        DockPanel.SetDock(_pathLabel, Dock.Top);
        dock.Children.Add(_header);
        dock.Children.Add(_pathLabel);
        dock.Children.Add(_bodyHost);
        Content = dock;

        _wakeCallback = OnWakeFromGo;
        _wakeCallbackHandle = GCHandle.Alloc(_wakeCallback);
        var cbPtr = Marshal.GetFunctionPointerForDelegate(_wakeCallback);
        Bridge.TakeString(Bridge.MarkdownViewRegisterWake(_handle, cbPtr));

        if (host != null)
        {
            _selectedPathHandler = OnSelectedPathChanged;
            host.SelectedPath += _selectedPathHandler;

            // Seed if a path was already selected when this panel mounted.
            if (host.CurrentSelectedPath is string seed && !string.IsNullOrEmpty(seed))
            {
                LoadPath(seed);
            }
        }
    }

    private void OnSelectedPathChanged(string path)
    {
        // Coalesce rapid selection changes — only load the final
        // path after the user settles. See LoadPathDebounce comment.
        _pendingPath = path;
        if (_loadPathTimer == null)
        {
            _loadPathTimer = new DispatcherTimer { Interval = LoadPathDebounce };
            _loadPathTimer.Tick += OnLoadPathTimerTick;
        }
        _loadPathTimer.Stop();
        _loadPathTimer.Start();
    }

    private void OnLoadPathTimerTick(object? sender, EventArgs e)
    {
        _loadPathTimer?.Stop();
        if (_disposed) return;
        var path = _pendingPath;
        _pendingPath = null;
        if (path == null || path == _lastLoadedPath) return;
        _lastLoadedPath = path;
        LoadPath(path);
    }

    private void LoadPath(string path)
    {
        if (_handle < 0) return;
        LoadPathCallCountForTests++;
        PanelLog.Write("markdown-view", $"LoadPath h={_handle} path={path}");
        Bridge.TakeString(Bridge.MarkdownViewLoadPath(_handle, path));
    }

    private static long ParseHandle(string envelope)
    {
        try
        {
            using var doc = JsonDocument.Parse(envelope);
            var root = doc.RootElement;
            if (root.TryGetProperty("ok", out var ok) && ok.GetBoolean()
                && root.TryGetProperty("handle", out var h))
            {
                return h.GetInt64();
            }
        }
        catch { }
        return -1;
    }

    private void OnWakeFromGo(long handle)
    {
        if (_disposed) return;
        if (_renderQueued) return;
        _renderQueued = true;
        Dispatcher.UIThread.Post(() =>
        {
            _renderQueued = false;
            if (_disposed) return;
            RerenderFromBridge();
        });
    }

    private void RerenderFromBridge()
    {
        if (_handle < 0) return;
        var reply = Bridge.TakeString(Bridge.MarkdownViewRender(_handle));
        MarkdownViewOutputDto? dto = null;
        try
        {
            using var doc = JsonDocument.Parse(reply);
            if (!doc.RootElement.TryGetProperty("ok", out var ok) || !ok.GetBoolean())
            {
                _pathLabel.Text = reply;
                _pathLabel.Foreground = Brushes.IndianRed;
                return;
            }
            dto = doc.RootElement.GetProperty("result").Deserialize<MarkdownViewOutputDto>();
        }
        catch (Exception ex)
        {
            _pathLabel.Text = $"markdown-view render parse failed: {ex.Message}";
            _pathLabel.Foreground = Brushes.OrangeRed;
            return;
        }
        if (dto == null)
        {
            _pathLabel.Text = "(empty)";
            ShowPlaceholder("(no content)");
            return;
        }

        if (dto.Empty)
        {
            _pathLabel.Text = "(no file selected)";
            _pathLabel.Opacity = 0.55;
            _pathLabel.ClearValue(TextBlock.ForegroundProperty);
            ShowPlaceholder("Select a doc/markdown-file entity in the tree.");
            return;
        }
        if (dto.NotFound)
        {
            _pathLabel.Text = $"file not found: {dto.Path}";
            _pathLabel.Opacity = 0.85;
            _pathLabel.Foreground = Brushes.IndianRed;
            ShowPlaceholder("");
            return;
        }

        var titleLine = string.IsNullOrEmpty(dto.Title) ? "(untitled)" : dto.Title;
        _header.Text = titleLine;
        _pathLabel.Text = dto.Path;
        _pathLabel.Opacity = 0.6;
        _pathLabel.ClearValue(TextBlock.ForegroundProperty);
        SwapBody(dto.Content ?? "");
        _placeholder.IsVisible = false;
    }

    // ---------- Adaptive render pipeline ------------------------------
    //
    // The render pipeline has three layers, designed so the panel
    // NEVER blocks the UI thread for more than ~one frame and NEVER
    // hands Skia a payload it can stack-overflow on:
    //
    //   1. Pre-parse safety net. If the markdown is above
    //      MaxRichBytes, skip the rich renderer entirely and show a
    //      plain TextBox with a banner. This is the "I caught it,
    //      no crash" backstop. It triggers only on absurd inputs.
    //
    //   2. Per-block split (MaxInlinesPerBlock). Even when we render
    //      rich, no single SelectableTextBlock holds more than ~500
    //      inlines — Avalonia's measure/arrange + Skia's text
    //      shaping recurse deeply through inline collections, and
    //      one TextBlock with ~3000 inlines (e.g.
    //      REPOSITORY-WORKSPACE-ROADMAP.md at 72KB → 2880 inlines)
    //      blows the call stack inside the paint pipeline on the
    //      next dispatcher tick.
    //
    //   3. Adaptive incremental attach. Inlines are added to the
    //      visual tree in batches; between batches we yield the
    //      dispatcher at Background priority so input, scroll, and
    //      Skia paint all interleave with our build. Batch size
    //      adapts to keep each batch under ~one frame (12ms target,
    //      20ms ceiling, 6ms floor).
    //
    // The trade-off for the per-block split: drag-select doesn't
    // cross block boundaries. Per-block selection still works. Copy
    // through the bridge (when wired) can stitch.
    private const int MaxInlinesPerBlock = 500;
    private const int MaxRichBytes = 1024 * 1024;  // 1 MB
    private const int InitialBatchSize = 200;
    private const int MinBatchSize = 50;
    private const int MaxBatchSize = 1000;
    // Adaptive batch deadbands: if a batch's CPU time crosses MaxBatchMs,
    // halve next batch; if it's below MinBatchMs, double. Between the
    // two we hold steady. The implicit target sits in the middle (~12ms,
    // roughly one 60Hz frame budget).
    private const double MaxBatchMs = 20.0;
    private const double MinBatchMs = 6.0;

    private CancellationTokenSource? _renderCts;

    // Test surface: the most recent render's adaptive batch sizes
    // and the final block count. Lets tests assert that big docs
    // emit in multiple batches (not one giant synchronous pass) and
    // small docs emit in one.
    internal int LastRenderBatchCountForTests { get; private set; }
    internal int LastRenderBlockCountForTests { get; private set; }
    internal bool LastRenderUsedPlainTextFallbackForTests { get; private set; }

    // Diagnostic instrumentation — reports Skia's process-global font
    // cache usage between renders. The number is purely informational
    // (no fix applied here). Two earlier fix attempts
    // (SKGraphics.PurgeFontCache and GC.WaitForPendingFinalizers)
    // both made the open #4 crash WORSE rather than better, falsifying
    // the deep-dive's strike-cache-LRU-eviction hypothesis. See the
    // MODEL-AVALONIA-RUNTIME.md §6 Boundary A/B revision: the crash
    // happens at ~208 KB cache (10% of the 2 MB budget), far before
    // any LRU eviction; GC drains report `before == after`, meaning
    // old blobs are strongly reachable (not finalizer-pending). The
    // pinning source is upstream of Skia — most likely in Avalonia's
    // compositor serialization queue or LayoutManager retention.
    //
    // Keeping the instrumentation in place: it gives every smoke run
    // a per-render trace of strike-cache growth, which will be the
    // forensic baseline for the next investigation.
    private static void LogStrikeCacheState(long handle)
    {
        if (!PanelLog.Enabled) return;
        try
        {
            var bytes = SKGraphics.GetFontCacheUsed();
            PanelLog.Write("markdown-view",
                $"StrikeCacheState h={handle} fontCache={bytes}");
        }
        catch (Exception ex)
        {
            PanelLog.Write("markdown-view",
                $"StrikeCacheState h={handle} probe-failed: {ex.GetType().Name}");
        }
    }

    private void SwapBody(string markdown)
    {
        markdown ??= "";
        PanelLog.Write("markdown-view",
            $"SwapBody h={_handle} bytes={markdown.Length}");

        // Cancel any in-flight incremental render — when the user
        // clicks rapidly through docs we want the previous emit to
        // stop, not finish in the background after we've already
        // started attaching new content.
        _renderCts?.Cancel();
        _renderCts = new CancellationTokenSource();
        var token = _renderCts.Token;

        ClearBody();

        // Diagnostic only — log the strike cache size between
        // renders. (Earlier fix attempts that mutated the cache or
        // forced GC at this seam made the open #4 crash WORSE; see
        // LogStrikeCacheState comment for the falsified hypothesis
        // and the next investigation direction.)
        LogStrikeCacheState(_handle);

        // Layer 1: hard safety net. Above MaxRichBytes we don't even
        // try the rich path — every byte of HTML/markdown the user
        // ever opens stays inside this guard. The fallback is loud
        // (banner) and reversible (just open a smaller doc).
        if (markdown.Length > MaxRichBytes)
        {
            PanelLog.Write("markdown-view",
                $"SwapBody h={_handle} bytes={markdown.Length} > {MaxRichBytes}, plain-text fallback");
            AttachPlainTextFallback(markdown);
            LastRenderUsedPlainTextFallbackForTests = true;
            BodyRecreateCountForTests++;
            return;
        }
        LastRenderUsedPlainTextFallbackForTests = false;

        var inlines = MarkdownRenderer.BuildInlines(markdown);
        PanelLog.Write("markdown-view",
            $"SwapBody h={_handle} parsed {inlines.Count} inlines, planning blocks...");

        // Pre-compute split points (last LineBreak index that closes
        // a block of >= MaxInlinesPerBlock inlines). Doing this before
        // we touch the visual tree means EmitBatch never has to Add a
        // new TextBlock to the StackPanel mid-emit — the structural
        // shape is fixed at start. Mutating visual-tree children during
        // a Background-priority emit interleaves with Avalonia's Render-
        // priority measure pass and crashes Skia in the real X11
        // pipeline (headless paint hides this; the Xvfb driver caught
        // it on a 723-inline doc).
        var splitAfter = new List<int>();
        int sinceSplitPlan = 0;
        for (int i = 0; i < inlines.Count; i++)
        {
            sinceSplitPlan++;
            if (inlines[i] is LineBreak && sinceSplitPlan >= MaxInlinesPerBlock)
            {
                splitAfter.Add(i);
                sinceSplitPlan = 0;
            }
        }
        int blockCount = splitAfter.Count + 1;

        // Pre-allocate every block we'll need + attach them all to the
        // PERSISTENT _bodyStack. After this, EmitBatch only mutates
        // Inlines collections on blocks that are already structurally
        // fixed. The _bodyScroll itself is never detached/reattached
        // — only its child stack's Children collection is mutated.
        // This avoids the X11 paint-pipeline crash we saw when
        // ClearBody+Add'd a new ScrollViewer per render.
        var blocks = new SelectableTextBlock[blockCount];
        for (int b = 0; b < blockCount; b++)
        {
            blocks[b] = NewBlock();
            _bodyStack!.Children.Add(blocks[b]);
        }
        _bodyScroll!.IsVisible = true;
        _body = blocks[blockCount - 1];
        BodyRecreateCountForTests++;
        LastRenderBatchCountForTests = 0;
        LastRenderBlockCountForTests = blockCount;
        PanelLog.Write("markdown-view",
            $"SwapBody h={_handle} pre-allocated {blockCount} block(s), starting adaptive emit...");

        var state = new AttachState
        {
            Blocks = blocks,
            CurrentBlockIndex = 0,
            SplitAfter = splitAfter,
            NextSplitPos = 0,
            Inlines = inlines,
            Index = 0,
            BatchSize = InitialBatchSize,
            Token = token,
            Handle = _handle,
        };
        // Kick off the first batch on the dispatcher. We Post rather
        // than run synchronously so the initial scroll/scaffolding
        // paint happens before we start hammering attach work.
        Dispatcher.UIThread.Post(() => EmitBatch(state), DispatcherPriority.Background);
    }

    private void EmitBatch(AttachState state)
    {
        if (_disposed) return;
        if (state.Token.IsCancellationRequested) return;
        // The visual tree we were attaching into may have been
        // dropped by a subsequent SwapBody. Bail.
        if (state.Blocks.Length == 0) return;
        var anchorBlock = state.Blocks[state.Blocks.Length - 1];
        if (!ReferenceEquals(_body, anchorBlock) && anchorBlock.Parent == null) return;
        if (state.Index == 0)
        {
            PanelLog.Write("markdown-view",
                $"EmitBatch h={state.Handle} batch1 begin (size={state.BatchSize})");
        }

        var sw = Stopwatch.StartNew();
        int emitted = 0;
        int startIdx = state.Index;
        while (state.Index < state.Inlines.Count && emitted < state.BatchSize)
        {
            var idx = state.Index++;
            var inline = state.Inlines[idx];
            state.Blocks[state.CurrentBlockIndex].Inlines!.Add(inline);
            emitted++;
            // Advance to next block when we cross the pre-computed
            // split point. The visual tree is unchanged here — just
            // the index of which already-attached block we're filling.
            if (state.NextSplitPos < state.SplitAfter.Count
                && idx == state.SplitAfter[state.NextSplitPos])
            {
                state.CurrentBlockIndex++;
                state.NextSplitPos++;
                PanelLog.Write("markdown-view",
                    $"EmitBatch h={state.Handle} crossed split at idx={idx} -> block {state.CurrentBlockIndex}");
            }
            if ((emitted % 100) == 0)
            {
                PanelLog.Write("markdown-view",
                    $"EmitBatch h={state.Handle} progress emitted={emitted}/{state.BatchSize} idx={idx}");
            }
        }
        sw.Stop();
        LastRenderBatchCountForTests++;

        if (state.Index >= state.Inlines.Count)
        {
            PanelLog.Write("markdown-view",
                $"SwapBody h={state.Handle} done (recreate#{BodyRecreateCountForTests}, " +
                $"blocks={LastRenderBlockCountForTests}, batches={LastRenderBatchCountForTests}, " +
                $"lastBatchMs={sw.ElapsedMilliseconds})");
            return;
        }

        // Adaptive sizing: keep each batch's CPU cost under one frame.
        var elapsed = sw.Elapsed.TotalMilliseconds;
        if (elapsed > MaxBatchMs)
        {
            state.BatchSize = Math.Max(MinBatchSize, state.BatchSize / 2);
        }
        else if (elapsed < MinBatchMs)
        {
            state.BatchSize = Math.Min(MaxBatchSize, state.BatchSize * 2);
        }
        // Otherwise we're in the sweet spot; keep current size.

        Dispatcher.UIThread.Post(() => EmitBatch(state), DispatcherPriority.Background);
    }

    private void ClearBody()
    {
        // Drop existing block children. We do NOT explicitly clear
        // their Inlines first — that interfered with Avalonia's
        // internal layout state and crashed earlier than the
        // accumulation path we were trying to fix. The blocks
        // become GC-eligible after Children.Clear; Inlines go with
        // them.
        if (_bodyStack != null)
        {
            _bodyStack.Children.Clear();
        }
        if (_bodyScroll != null) _bodyScroll.IsVisible = false;
        _body = null;
        // Drop the fallback if it was up.
        if (_fallbackContainer != null)
        {
            _bodyHost.Children.Remove(_fallbackContainer);
            _fallbackContainer = null;
        }
    }

    private void AttachPlainTextFallback(string markdown)
    {
        var stack = new StackPanel { Orientation = Orientation.Vertical };
        var banner = new Border
        {
            Background = new SolidColorBrush(Color.FromArgb(0x55, 0xff, 0xa6, 0x4a)),
            Padding = new Thickness(10, 6),
            Margin = new Thickness(0, 0, 0, 8),
            Child = new TextBlock
            {
                Text = $"Document is {markdown.Length / 1024.0:F1} KB — above the " +
                       $"{MaxRichBytes / 1024} KB rich-render cap. Showing plain text.",
                FontSize = 12,
                FontWeight = FontWeight.SemiBold,
                Foreground = Brushes.White,
                TextWrapping = TextWrapping.Wrap,
            },
        };
        stack.Children.Add(banner);
        var text = new TextBox
        {
            Text = markdown,
            IsReadOnly = true,
            AcceptsReturn = true,
            TextWrapping = TextWrapping.NoWrap,
            FontFamily = new FontFamily("monospace"),
            FontSize = 12,
            Background = Brushes.Transparent,
            BorderThickness = new Thickness(0),
            Padding = new Thickness(0),
        };
        stack.Children.Add(text);
        var scroll = new ScrollViewer
        {
            Content = stack,
            HorizontalScrollBarVisibility = Avalonia.Controls.Primitives.ScrollBarVisibility.Auto,
            VerticalScrollBarVisibility = Avalonia.Controls.Primitives.ScrollBarVisibility.Auto,
        };
        _fallbackContainer = scroll;
        _bodyHost.Children.Add(scroll);
    }

    private static SelectableTextBlock NewBlock() => new SelectableTextBlock
    {
        FontSize = 14,
        TextWrapping = TextWrapping.Wrap,
        Opacity = 0.92,
        Padding = new Thickness(4, 2, 12, 4),
    };

    private sealed class AttachState
    {
        // Pre-allocated text blocks. EmitBatch only writes Inlines on
        // these; the visual-tree structure is fixed before the first
        // batch fires. See the SwapBody planning loop for why.
        public SelectableTextBlock[] Blocks = Array.Empty<SelectableTextBlock>();
        public int CurrentBlockIndex;
        // Sorted ascending. After consuming inlines[SplitAfter[k]],
        // advance CurrentBlockIndex.
        public List<int> SplitAfter = default!;
        public int NextSplitPos;
        public List<Inline> Inlines = default!;
        public int Index;
        public int BatchSize;
        public CancellationToken Token;
        public long Handle;
    }

    private void ShowPlaceholder(string text)
    {
        _placeholder.Text = text;
        _placeholder.IsVisible = !string.IsNullOrEmpty(text);
        // Cancel any in-flight emit; we're switching to placeholder.
        _renderCts?.Cancel();
        ClearBody();
    }

    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        _renderCts?.Cancel();
        if (_loadPathTimer != null)
        {
            _loadPathTimer.Stop();
            _loadPathTimer.Tick -= OnLoadPathTimerTick;
            _loadPathTimer = null;
        }
        if (_host != null && _selectedPathHandler != null)
        {
            _host.SelectedPath -= _selectedPathHandler;
        }
        if (_handle >= 0)
        {
            Bridge.MarkdownViewClose(_handle);
        }
        _wakeCallback = null;
        if (_wakeCallbackHandle.IsAllocated) _wakeCallbackHandle.Free();
    }

    private sealed class MarkdownViewOutputDto
    {
        [JsonPropertyName("Empty")] public bool Empty { get; set; }
        [JsonPropertyName("NotFound")] public bool NotFound { get; set; }
        [JsonPropertyName("Path")] public string Path { get; set; } = "";
        [JsonPropertyName("Title")] public string Title { get; set; } = "";
        [JsonPropertyName("Content")] public string Content { get; set; } = "";
        [JsonPropertyName("Editing")] public bool Editing { get; set; }
        [JsonPropertyName("Dirty")] public bool Dirty { get; set; }
    }
}
