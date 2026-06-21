using System;
using System.Collections.Generic;
using System.Collections.ObjectModel;
using System.Runtime.InteropServices;
using System.Text.Json;
using System.Text.Json.Serialization;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Controls.Templates;
using Avalonia.Data;
using Avalonia.Layout;
using Avalonia.Media;
using Avalonia.Threading;

namespace EntityAvalonia.Panels;

// TreeViewPanel renders the local peer's tree via wb.TreeBrowserModel
// (running inside the Go bridge). The panel holds no tree state of its
// own — every render is a fresh snapshot pulled via Bridge.TreeRender.
//
// Wake → render pipeline:
//   Go side fires TreeWakeCallback per coalesced dirty-window.
//   We post a single render request to the UI thread per wake, with a
//   _renderQueued bool to dedupe rapid wakes (Avalonia's Dispatcher
//   doesn't auto-coalesce identical posts).
//
// Lifecycle: TreeOpen on construction, TreeClose on Dispose. The wake
// callback delegate is held in a field — if it GCs, Go's invoke
// segfaults (same constraint as MainWindow's _watchCallback).
public sealed class TreeViewPanel : UserControl, IDisposable
{
    // P3 (wake debounce) — tree wakes are bursty during ingest and
    // roster sync. Coalesce a burst to a single render. Reference
    // shape: MarkdownViewPanel.LoadPathDebounce. 150ms keeps the tree
    // feeling live while paying one render per burst rather than N.
    private static readonly TimeSpan WakeDebounce = TimeSpan.FromMilliseconds(150);

    private readonly long _peerHandle;
    private readonly long _handle;
    private readonly ListBox _list;
    private readonly TextBox _searchBox;
    private readonly TextBlock _statusLine;
    private readonly ObservableCollection<RowVm> _rows = new();

    private Bridge.TreeWakeCallback? _wakeCallback;
    // Explicit GC root for _wakeCallback. The field reference alone is
    // NOT sufficient — the .NET runtime can collect the delegate any
    // time the field isn't visibly used, even if the panel itself is
    // still reachable. The bridge holds a function pointer to the
    // delegate's marshaled stub; if the stub is freed while Go still
    // intends to call it, we get
    //   "A callback was made on a garbage collected delegate"
    // and the host process aborts. GCHandle.Alloc(Normal) creates a
    // strong root that survives until we explicitly Free() it in
    // Dispose, AFTER Bridge.TreeClose has joined the wake goroutine.
    private GCHandle _wakeCallbackHandle;
    private DispatcherTimer? _wakeTimer;
    private bool _renderQueued;
    private bool _disposed;
    // True while RerenderFromBridge is mutating _list.SelectedIndex to
    // restore the user's selection after a wake. Suppresses the
    // SelectionChanged → EntitySelected fan-out so wakes don't cascade
    // every subscribed panel into a reload they didn't ask for.
    private bool _restoringSelection;

    // Fires when the user selects a row that points to a leaf entity
    // (HasEntry=true). The payload is the entity's full path. Folder
    // rows (no entry) DO NOT fire — the detail panel only cares about
    // selectable entities.
    public event Action<string>? EntitySelected;

    // Test-only surface — Workbench.Headless.Tests pokes at row count,
    // selection state, and per-row metadata without taking a public
    // dependency on the ObservableCollection or ListBox. `internal` is
    // opened to the test assembly via [InternalsVisibleTo] on
    // frontend.csproj.
    internal int RowsCountForTests => _rows.Count;
    internal int SelectedIndexForTests => _list.SelectedIndex;
    internal ListBox ListForTests => _list;
    internal string GetRowPathForTests(int idx) => _rows[idx].Path;
    internal bool IsEntryForTests(int idx) => _rows[idx].HasEntry;
    internal void SetSearchForTests(string text) => _searchBox.Text = text;

    public TreeViewPanel(long peerHandle)
    {
        _peerHandle = peerHandle;
        var openReply = Bridge.TakeString(Bridge.TreeOpen(peerHandle));
        var handle = ParseHandle(openReply);
        if (handle < 0)
        {
            _handle = -1;
            Content = new SelectableTextBlock
            {
                Text = $"tree open failed: {openReply}",
                Foreground = Brushes.IndianRed,
                Margin = new Thickness(12),
                FontSize = 14,
            };
            return;
        }
        _handle = handle;

        _searchBox = new TextBox
        {
            Watermark = "filter — path substring, or `type:foo`",
            Margin = new Thickness(8, 8, 8, 4),
        };
        _searchBox.TextChanged += (_, _) => OnSearchChanged();

        _statusLine = new TextBlock
        {
            Text = "(no entries)",
            Opacity = 0.6,
            Margin = new Thickness(10, 0, 10, 4),
            FontSize = 12,
        };

        _list = new ListBox
        {
            ItemsSource = _rows,
            FontFamily = new FontFamily("monospace"),
            FontSize = 14,
            ItemTemplate = new FuncDataTemplate<RowVm>((vm, _) => BuildRow(vm), supportsRecycling: true),
        };
        _list.DoubleTapped += (_, _) => ToggleSelectedRow();
        _list.KeyDown += (_, e) =>
        {
            if (e.Key == Avalonia.Input.Key.Enter || e.Key == Avalonia.Input.Key.Space)
            {
                ToggleSelectedRow();
                e.Handled = true;
            }
        };
        _list.SelectionChanged += (_, _) =>
        {
            if (_restoringSelection) return;
            if (_list.SelectedIndex < 0 || _list.SelectedIndex >= _rows.Count) return;
            var row = _rows[_list.SelectedIndex];
            if (row.HasEntry && !string.IsNullOrEmpty(row.Path))
            {
                EntitySelected?.Invoke(row.Path);
            }
        };

        var grid = new Grid
        {
            RowDefinitions = new RowDefinitions("Auto,Auto,*"),
        };
        Grid.SetRow(_searchBox, 0);
        Grid.SetRow(_statusLine, 1);
        Grid.SetRow(_list, 2);
        grid.Children.Add(_searchBox);
        grid.Children.Add(_statusLine);
        grid.Children.Add(_list);
        Content = grid;

        // Pin the delegate via GCHandle so the marshaled stub survives
        // for the lifetime of the bridge handle, then register the
        // wake and pull the seed render.
        _wakeCallback = OnWakeFromGo;
        _wakeCallbackHandle = GCHandle.Alloc(_wakeCallback);
        var cbPtr = Marshal.GetFunctionPointerForDelegate(_wakeCallback);
        Bridge.TakeString(Bridge.TreeRegisterWake(_handle, cbPtr));
        PanelLog.Write("tree-view", $"Mount h={_handle}");

        // TreeOpen already fires one wake to draw the seed; nothing to do
        // here — it'll arrive on the wake-fanout goroutine.
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
        // Called from the wake-fanout goroutine. Hop to UI thread first
        // so DispatcherTimer.Start touches its owning thread, then
        // restart the debounce — a burst of N wakes within WakeDebounce
        // produces a single render at the end of the burst.
        if (_disposed) return;
        Dispatcher.UIThread.Post(() =>
        {
            if (_disposed) return;
            if (_wakeTimer == null)
            {
                _wakeTimer = new DispatcherTimer { Interval = WakeDebounce };
                _wakeTimer.Tick += OnWakeTimerTick;
            }
            _wakeTimer.Stop();
            _wakeTimer.Start();
        });
    }

    private void OnWakeTimerTick(object? sender, EventArgs e)
    {
        _wakeTimer?.Stop();
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
        PanelLog.Write("tree-view", $"Render h={_handle}");
        var reply = Bridge.TakeString(Bridge.TreeRender(_handle));
        TreeRenderDto? dto = null;
        try
        {
            using var doc = JsonDocument.Parse(reply);
            if (!doc.RootElement.TryGetProperty("ok", out var ok) || !ok.GetBoolean())
            {
                _statusLine.Text = reply;
                _statusLine.Foreground = Brushes.IndianRed;
                _statusLine.Opacity = 1.0;
                return;
            }
            dto = doc.RootElement.GetProperty("result").Deserialize<TreeRenderDto>();
        }
        catch (Exception ex)
        {
            _statusLine.Text = $"tree render parse failed: {ex.Message}";
            _statusLine.Foreground = Brushes.OrangeRed;
            return;
        }
        if (dto?.Rows == null)
        {
            _rows.Clear();
            _statusLine.Text = "(empty tree)";
            _statusLine.ClearValue(TextBlock.ForegroundProperty);
        _statusLine.Opacity = 0.6;
            return;
        }

        // Preserve selection by PATH (not index) — when the search
        // filter changes or tree contents shift, the entity at a given
        // numeric index changes, so index-restore silently switches
        // what's selected and re-fires EntitySelected for the wrong
        // path. Path-restore keeps the user's intended selection
        // across re-renders.
        var selectedPath = (_list.SelectedIndex >= 0 && _list.SelectedIndex < _rows.Count)
            ? _rows[_list.SelectedIndex].Path
            : null;
        _restoringSelection = true;
        try
        {
            int restoreIdx = -1;
            if (RowsPathSetMatches(_rows, dto.Rows))
            {
                // P5 (differential update) — same path set in same order
                // is the steady-state case: a wake fires for an unrelated
                // entity, the tree filter is unchanged. Replace items
                // in place where they differ; this fires per-index
                // Replace events instead of a Reset, so the ListBox
                // keeps every recycled container instead of throwing
                // them away. Massive paint-pipeline win on long trees.
                for (int i = 0; i < dto.Rows.Count; i++)
                {
                    var r = dto.Rows[i];
                    var newVm = new RowVm(r);
                    if (!_rows[i].SameDisplayAs(newVm))
                    {
                        _rows[i] = newVm;
                    }
                    if (selectedPath != null && r.Path == selectedPath)
                    {
                        restoreIdx = i;
                    }
                }
            }
            else
            {
                _rows.Clear();
                for (int i = 0; i < dto.Rows.Count; i++)
                {
                    var r = dto.Rows[i];
                    _rows.Add(new RowVm(r));
                    if (selectedPath != null && r.Path == selectedPath)
                    {
                        restoreIdx = i;
                    }
                }
            }
            if (restoreIdx >= 0)
            {
                _list.SelectedIndex = restoreIdx;
            }
        }
        finally
        {
            _restoringSelection = false;
        }
        _statusLine.Text = string.IsNullOrEmpty(dto.SearchText)
            ? $"{dto.Rows.Count} row(s)"
            : $"{dto.MatchCount} match(es) for \"{dto.SearchText}\"";
        _statusLine.ClearValue(TextBlock.ForegroundProperty);
        _statusLine.Opacity = 0.6;
    }

    private void ToggleSelectedRow()
    {
        var idx = _list.SelectedIndex;
        if (idx < 0 || _handle < 0) return;
        Bridge.TakeString(Bridge.TreeToggleExpand(_handle, idx));
    }

    private static bool RowsPathSetMatches(IReadOnlyList<RowVm> existing, IReadOnlyList<TreeRowDto> incoming)
    {
        if (existing.Count != incoming.Count) return false;
        for (int i = 0; i < existing.Count; i++)
        {
            if (existing[i].Path != incoming[i].Path) return false;
        }
        return true;
    }

    private void OnSearchChanged()
    {
        if (_handle < 0) return;
        Bridge.TakeString(Bridge.TreeSetSearch(_handle, _searchBox.Text ?? ""));
    }

    private static Control BuildRow(RowVm vm)
    {
        var stack = new StackPanel
        {
            Orientation = Orientation.Horizontal,
            Spacing = 2,
        };
        // Depth indent.
        stack.Children.Add(new Border
        {
            Width = vm.Depth * 16,
        });
        // Expand arrow placeholder (▸ ▾ or two spaces for leaves so
        // labels align in the column).
        stack.Children.Add(new TextBlock
        {
            Text = vm.HasChildren ? (vm.Expanded ? "▾ " : "▸ ") : "   ",
            FontSize = 14,
            Opacity = 0.5,
            VerticalAlignment = VerticalAlignment.Center,
            Width = 20,
        });
        // Segment label. Plain TextBlock — SelectableTextBlock here
        // intercepts pointer input for text selection and fights the
        // parent ListBox's selection model (single clicks get eaten,
        // jostle on multi-click). Path copying is available via the
        // detail panel, which shows the full path as a selectable
        // string.
        var label = new TextBlock
        {
            Text = vm.Segment,
            FontSize = 14,
            VerticalAlignment = VerticalAlignment.Center,
        };
        if (vm.HasEntry)
        {
            label.Foreground = new SolidColorBrush(Color.FromRgb(0x7e, 0xc5, 0xff));
        }
        stack.Children.Add(label);
        // Collapsed-folder leaf count hint.
        if (vm.HasChildren && !vm.Expanded && vm.LeafCount > 0)
        {
            stack.Children.Add(new TextBlock
            {
                Text = $"  ({vm.LeafCount})",
                FontSize = 12,
                Opacity = 0.5,
                VerticalAlignment = VerticalAlignment.Center,
            });
        }
        return stack;
    }

    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        PanelLog.Write("tree-view", $"Dispose h={_handle}");
        if (_wakeTimer != null)
        {
            _wakeTimer.Stop();
            _wakeTimer.Tick -= OnWakeTimerTick;
            _wakeTimer = null;
        }
        if (_handle >= 0)
        {
            // TreeClose joins the wake goroutine before returning, so
            // after this point no Go code holds the function pointer.
            Bridge.TreeClose(_handle);
        }
        _wakeCallback = null;
        // Safe to release the GC root now that Go is no longer using
        // the function pointer (TreeClose joined the goroutine).
        if (_wakeCallbackHandle.IsAllocated) _wakeCallbackHandle.Free();
    }

    // --- DTOs ----------------------------------------------------------

    private sealed class TreeRenderDto
    {
        [JsonPropertyName("Rows")] public List<TreeRowDto>? Rows { get; set; }
        [JsonPropertyName("SearchText")] public string SearchText { get; set; } = "";
        [JsonPropertyName("MatchCount")] public int MatchCount { get; set; }
    }

    private sealed class TreeRowDto
    {
        [JsonPropertyName("Path")] public string Path { get; set; } = "";
        [JsonPropertyName("Segment")] public string Segment { get; set; } = "";
        [JsonPropertyName("Depth")] public int Depth { get; set; }
        [JsonPropertyName("HasChildren")] public bool HasChildren { get; set; }
        [JsonPropertyName("Expanded")] public bool Expanded { get; set; }
        [JsonPropertyName("HasEntry")] public bool HasEntry { get; set; }
        [JsonPropertyName("LeafCount")] public int LeafCount { get; set; }
    }

    private sealed class RowVm
    {
        public string Path { get; }
        public string Segment { get; }
        public int Depth { get; }
        public bool HasChildren { get; }
        public bool Expanded { get; }
        public bool HasEntry { get; }
        public int LeafCount { get; }

        public RowVm(TreeRowDto dto)
        {
            Path = dto.Path;
            Segment = string.IsNullOrEmpty(dto.Segment) ? "/" : dto.Segment;
            Depth = dto.Depth;
            HasChildren = dto.HasChildren;
            Expanded = dto.Expanded;
            HasEntry = dto.HasEntry;
            LeafCount = dto.LeafCount;
        }

        // Cheap structural equality on the rendered fields, used by
        // the P5 diff path to skip _rows[i] = newVm assignments that
        // would fire a Replace event with no visible change.
        public bool SameDisplayAs(RowVm other)
        {
            return Segment == other.Segment
                && Depth == other.Depth
                && HasChildren == other.HasChildren
                && Expanded == other.Expanded
                && HasEntry == other.HasEntry
                && LeafCount == other.LeafCount;
        }
    }
}
