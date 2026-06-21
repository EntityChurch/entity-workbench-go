using System;
using System.Collections.Generic;
using System.Collections.ObjectModel;
using System.Runtime.InteropServices;
using System.Text.Json;
using System.Text.Json.Serialization;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Controls.Templates;
using Avalonia.Input;
using Avalonia.Layout;
using Avalonia.Media;
using Avalonia.Threading;

namespace EntityAvalonia.Panels;

// MarkdownFilesPanel renders wb.MarkdownFilesModel — tree-shaped
// browser filtered to doc/markdown-file entities under "docs/".
//
// Selecting a leaf row broadcasts the path via IPanelHost.
// PublishSelectedPath so any DetailPanel / MarkdownViewPanel mounted
// in a slot follows along. Folder rows broadcast nothing (the
// markdown viewer's NotFound state isn't useful UX here).
public sealed class MarkdownFilesPanel : UserControl, IDisposable
{
    // P3 (wake debounce) — file-list wakes are bursty during peer
    // bring-up (every discovered markdown-file fires one). Coalesce a
    // burst to a single render by restarting a 150ms timer per wake.
    // Reference shape: MarkdownViewPanel.LoadPathDebounce.
    private static readonly TimeSpan WakeDebounce = TimeSpan.FromMilliseconds(150);

    private readonly long _peerHandle;
    private readonly long _handle;
    private readonly IPanelHost? _host;
    private readonly ListBox _list;
    private readonly TextBlock _statusLine;
    private readonly ObservableCollection<RowVm> _rows = new();

    private Bridge.TreeWakeCallback? _wakeCallback;
    // Explicit GC root — see TreeViewPanel for full rationale. Pinning
    // via GCHandle is what prevents the "callback was made on a
    // garbage collected delegate" host abort under panel-lifecycle stress.
    private GCHandle _wakeCallbackHandle;
    private DispatcherTimer? _wakeTimer;
    private bool _renderQueued;
    private bool _disposed;

    public MarkdownFilesPanel(long peerHandle, IPanelHost? host)
    {
        _peerHandle = peerHandle;
        _host = host;
        var openReply = Bridge.TakeString(Bridge.MarkdownFilesOpen(peerHandle));
        _handle = ParseHandle(openReply);
        if (_handle < 0)
        {
            Content = new SelectableTextBlock
            {
                Text = $"markdown-files open failed: {openReply}",
                Foreground = Brushes.IndianRed,
                Margin = new Thickness(12),
                FontSize = 14,
            };
            return;
        }

        _statusLine = new TextBlock
        {
            Text = "(no markdown files)",
            Opacity = 0.6,
            Margin = new Thickness(10, 6, 10, 4),
            FontSize = 12,
        };

        _list = new ListBox
        {
            ItemsSource = _rows,
            FontFamily = new FontFamily("monospace"),
            FontSize = 13,
            ItemTemplate = new FuncDataTemplate<RowVm>((vm, _) => BuildRow(vm), supportsRecycling: true),
        };
        _list.DoubleTapped += (_, _) => ToggleSelectedRow();
        _list.KeyDown += (_, e) =>
        {
            if (e.Key == Key.Enter || e.Key == Key.Space)
            {
                ToggleSelectedRow();
                e.Handled = true;
            }
        };
        _list.SelectionChanged += (_, _) =>
        {
            if (_list.SelectedIndex < 0 || _list.SelectedIndex >= _rows.Count) return;
            var row = _rows[_list.SelectedIndex];
            if (row.HasEntry && !string.IsNullOrEmpty(row.Path) && _host != null)
            {
                _host.PublishSelectedPath(row.Path);
            }
        };

        var grid = new Grid
        {
            RowDefinitions = new RowDefinitions("Auto,*"),
        };
        Grid.SetRow(_statusLine, 0);
        Grid.SetRow(_list, 1);
        grid.Children.Add(_statusLine);
        grid.Children.Add(_list);
        Content = grid;

        _wakeCallback = OnWakeFromGo;
        _wakeCallbackHandle = GCHandle.Alloc(_wakeCallback);
        var cbPtr = Marshal.GetFunctionPointerForDelegate(_wakeCallback);
        Bridge.TakeString(Bridge.MarkdownFilesRegisterWake(_handle, cbPtr));
        PanelLog.Write("markdown-files", $"Mount h={_handle}");
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
        // Hop to UI thread first so DispatcherTimer.Start touches its
        // owning thread; this method runs on Go's wake-fanout goroutine.
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
        PanelLog.Write("markdown-files", $"Render h={_handle}");
        var reply = Bridge.TakeString(Bridge.MarkdownFilesRender(_handle));
        MarkdownFilesOutputDto? dto = null;
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
            dto = doc.RootElement.GetProperty("result").Deserialize<MarkdownFilesOutputDto>();
        }
        catch (Exception ex)
        {
            _statusLine.Text = $"markdown-files render parse failed: {ex.Message}";
            _statusLine.Foreground = Brushes.OrangeRed;
            return;
        }
        if (dto?.Rows == null)
        {
            _rows.Clear();
            _statusLine.Text = string.IsNullOrEmpty(dto?.Error)
                ? "(no markdown files)"
                : dto.Error;
            _statusLine.ClearValue(TextBlock.ForegroundProperty);
            _statusLine.Opacity = 0.6;
            return;
        }

        // Same path-based selection preservation as TreeViewPanel.
        var selectedPath = (_list.SelectedIndex >= 0 && _list.SelectedIndex < _rows.Count)
            ? _rows[_list.SelectedIndex].Path
            : null;

        // P2-spirit: when the path set is identical render-to-render
        // (the common case — wake fires for an unrelated entity but
        // markdown-files filter result is unchanged), replace items
        // in place. Replace fires Replace events per index, not Reset;
        // the ListBox keeps every recycled container. A Clear+Add
        // sequence forces Avalonia to discard every recycled row and
        // re-template from scratch.
        int restoreIdx = -1;
        if (RowsPathSetMatches(_rows, dto.Rows))
        {
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
        _statusLine.Text = $"{dto.Rows.Count} row(s)";
        _statusLine.ClearValue(TextBlock.ForegroundProperty);
        _statusLine.Opacity = 0.6;
    }

    private static bool RowsPathSetMatches(IReadOnlyList<RowVm> existing, IReadOnlyList<RowDto> incoming)
    {
        if (existing.Count != incoming.Count) return false;
        for (int i = 0; i < existing.Count; i++)
        {
            if (existing[i].Path != incoming[i].Path) return false;
        }
        return true;
    }

    private void ToggleSelectedRow()
    {
        var idx = _list.SelectedIndex;
        if (idx < 0 || _handle < 0) return;
        Bridge.TakeString(Bridge.MarkdownFilesToggleExpand(_handle, idx));
    }

    private static Control BuildRow(RowVm vm)
    {
        var stack = new StackPanel
        {
            Orientation = Orientation.Horizontal,
            Spacing = 2,
        };
        stack.Children.Add(new Border { Width = vm.Depth * 16 });
        stack.Children.Add(new TextBlock
        {
            Text = vm.HasChildren ? (vm.Expanded ? "▾ " : "▸ ") : "   ",
            FontSize = 14,
            Opacity = 0.5,
            VerticalAlignment = VerticalAlignment.Center,
            Width = 20,
        });

        // Leaf rows show "segment — title" when there's a distinct title.
        var label = vm.Segment;
        if (vm.HasEntry && !string.IsNullOrEmpty(vm.Title) && vm.Title != vm.Segment)
        {
            label = $"{vm.Segment} — {vm.Title}";
        }
        var text = new SelectableTextBlock
        {
            Text = label,
            FontSize = 14,
            VerticalAlignment = VerticalAlignment.Center,
        };
        if (vm.HasEntry)
        {
            text.Foreground = new SolidColorBrush(Color.FromRgb(0x7e, 0xc5, 0xff));
        }
        stack.Children.Add(text);
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
        PanelLog.Write("markdown-files", $"Dispose h={_handle}");
        if (_wakeTimer != null)
        {
            _wakeTimer.Stop();
            _wakeTimer.Tick -= OnWakeTimerTick;
            _wakeTimer = null;
        }
        if (_handle >= 0)
        {
            Bridge.MarkdownFilesClose(_handle);
        }
        _wakeCallback = null;
        if (_wakeCallbackHandle.IsAllocated) _wakeCallbackHandle.Free();
    }

    private sealed class MarkdownFilesOutputDto
    {
        [JsonPropertyName("Rows")] public List<RowDto>? Rows { get; set; }
        [JsonPropertyName("Error")] public string Error { get; set; } = "";
    }

    private sealed class RowDto
    {
        [JsonPropertyName("Path")] public string Path { get; set; } = "";
        [JsonPropertyName("Segment")] public string Segment { get; set; } = "";
        [JsonPropertyName("Depth")] public int Depth { get; set; }
        [JsonPropertyName("HasChildren")] public bool HasChildren { get; set; }
        [JsonPropertyName("Expanded")] public bool Expanded { get; set; }
        [JsonPropertyName("HasEntry")] public bool HasEntry { get; set; }
        [JsonPropertyName("LeafCount")] public int LeafCount { get; set; }
        [JsonPropertyName("Title")] public string Title { get; set; } = "";
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
        public string Title { get; }
        public RowVm(RowDto dto)
        {
            Path = dto.Path;
            Segment = string.IsNullOrEmpty(dto.Segment) ? "docs" : dto.Segment;
            Depth = dto.Depth;
            HasChildren = dto.HasChildren;
            Expanded = dto.Expanded;
            HasEntry = dto.HasEntry;
            LeafCount = dto.LeafCount;
            Title = dto.Title;
        }

        // Cheap structural equality on the rendered fields, used by
        // RerenderFromBridge to skip _rows[i] = newVm assignments that
        // would fire a Replace event with no visible change.
        public bool SameDisplayAs(RowVm other)
        {
            return Segment == other.Segment
                && Depth == other.Depth
                && HasChildren == other.HasChildren
                && Expanded == other.Expanded
                && HasEntry == other.HasEntry
                && LeafCount == other.LeafCount
                && Title == other.Title;
        }
    }
}
