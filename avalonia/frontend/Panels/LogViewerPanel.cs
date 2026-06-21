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

// LogViewerPanel renders wb.LogFilterModel — per-peer event stream
// with collection + display level filtering. Wake fires per EventLog
// append via the bridge's LogOnAppend hook.
//
// Tab key cycles the per-panel display level; Ctrl+L cycles the
// global collection level. Matches console/log_viewer.go's bindings
// so muscle memory carries over.
public sealed class LogViewerPanel : UserControl, IDisposable
{
    // P4 (bounded list) — logs are append-mostly; cap the visible row
    // count so an unbounded EventLog (100k events over a long session)
    // can never blow the visual tree. Trim from the front, keeping the
    // most recent entries — the bottom is where new events appear and
    // is what the user is reading. See DISCIPLINE-CHARTER.md D15 and
    // GUIDE-AVALONIA-PANEL-PATTERNS.md §5 (b).
    private const int MaxDisplayRows = 1000;

    private readonly long _peerHandle;
    private readonly long _handle;
    private readonly TextBlock _titleLine;
    private readonly ListBox _list;
    private readonly ObservableCollection<LogRowVm> _rows = new();

    private Bridge.TreeWakeCallback? _wakeCallback;
    // Explicit GC root — see TreeViewPanel for full rationale.
    private GCHandle _wakeCallbackHandle;
    private bool _renderQueued;
    private bool _disposed;

    public LogViewerPanel(long peerHandle)
    {
        _peerHandle = peerHandle;
        var openReply = Bridge.TakeString(Bridge.LogOpen(peerHandle));
        _handle = ParseHandle(openReply);
        if (_handle < 0)
        {
            Content = new SelectableTextBlock
            {
                Text = $"log open failed: {openReply}",
                Foreground = Brushes.IndianRed,
                Margin = new Thickness(12),
                FontSize = 14,
            };
            return;
        }

        _titleLine = new TextBlock
        {
            Text = "(loading log…)",
            FontFamily = new FontFamily("monospace"),
            FontSize = 12,
            FontWeight = FontWeight.SemiBold,
            Opacity = 0.85,
            Margin = new Thickness(0, 0, 0, 6),
        };

        _list = new ListBox
        {
            ItemsSource = _rows,
            FontFamily = new FontFamily("monospace"),
            FontSize = 12,
            Background = Brushes.Transparent,
            BorderThickness = new Thickness(0),
            ItemTemplate = new FuncDataTemplate<LogRowVm>((vm, _) =>
                new SelectableTextBlock
                {
                    Text = vm.Display,
                    Foreground = ColorForLevel(vm.Level),
                    TextWrapping = TextWrapping.NoWrap,
                }, supportsRecycling: true),
        };

        // Keyboard bindings parallel console/log_viewer.go.
        KeyDown += (_, e) =>
        {
            if (e.Key == Key.Tab)
            {
                Bridge.TakeString(Bridge.LogCycleDisplayLevel(_handle));
                e.Handled = true;
            }
            else if (e.Key == Key.L && e.KeyModifiers == KeyModifiers.Control)
            {
                Bridge.TakeString(Bridge.LogCycleCollectionLevel(_handle));
                e.Handled = true;
            }
        };
        // Click-to-focus so keyboard shortcuts work without a global
        // hotkey route.
        Focusable = true;

        var dock = new DockPanel
        {
            LastChildFill = true,
            Margin = new Thickness(8),
        };
        DockPanel.SetDock(_titleLine, Dock.Top);
        dock.Children.Add(_titleLine);
        dock.Children.Add(new ScrollViewer { Content = _list });
        Content = dock;

        _wakeCallback = OnWakeFromGo;
        _wakeCallbackHandle = GCHandle.Alloc(_wakeCallback);
        var cbPtr = Marshal.GetFunctionPointerForDelegate(_wakeCallback);
        Bridge.TakeString(Bridge.LogRegisterWake(_handle, cbPtr));
        PanelLog.Write("log-viewer", $"Mount h={_handle}");
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
        PanelLog.Write("log-viewer", $"Render h={_handle}");
        var reply = Bridge.TakeString(Bridge.LogRender(_handle));
        LogOutputDto? dto = null;
        try
        {
            using var doc = JsonDocument.Parse(reply);
            if (!doc.RootElement.TryGetProperty("ok", out var ok) || !ok.GetBoolean())
            {
                _titleLine.Text = reply;
                _titleLine.Foreground = Brushes.IndianRed;
                return;
            }
            dto = doc.RootElement.GetProperty("result").Deserialize<LogOutputDto>();
        }
        catch (Exception ex)
        {
            _titleLine.Text = $"log render parse failed: {ex.Message}";
            _titleLine.Foreground = Brushes.OrangeRed;
            return;
        }
        if (dto == null)
        {
            _rows.Clear();
            _titleLine.Text = "(empty log)";
            return;
        }
        _titleLine.ClearValue(TextBlock.ForegroundProperty);

        _rows.Clear();
        int totalEntries = dto.Entries?.Count ?? 0;
        bool truncated = totalEntries > MaxDisplayRows;
        if (dto.Entries != null)
        {
            int start = truncated ? totalEntries - MaxDisplayRows : 0;
            for (int i = start; i < totalEntries; i++)
            {
                _rows.Add(new LogRowVm(dto.Entries[i]));
            }
        }
        _titleLine.Text = truncated
            ? $"{dto.Title}  (showing last {MaxDisplayRows} of {totalEntries})"
            : dto.Title;

        Dispatcher.UIThread.Post(() =>
        {
            if (_rows.Count > 0)
            {
                _list.ScrollIntoView(_rows[_rows.Count - 1]);
            }
        }, DispatcherPriority.Background);
    }

    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        PanelLog.Write("log-viewer", $"Dispose h={_handle}");
        if (_handle >= 0)
        {
            Bridge.LogClose(_handle);
        }
        _wakeCallback = null;
        if (_wakeCallbackHandle.IsAllocated) _wakeCallbackHandle.Free();
    }

    // Color palette mirrors console/log_viewer.go's level tags.
    private static IBrush ColorForLevel(int level) => level switch
    {
        // LogInfo
        0 => new SolidColorBrush(Color.FromArgb(0xee, 0xff, 0xff, 0xff)),
        // LogVerbose
        1 => new SolidColorBrush(Color.FromRgb(0x6e, 0xb5, 0xc5)),
        // LogDebug
        2 => new SolidColorBrush(Color.FromArgb(0x99, 0xff, 0xff, 0xff)),
        _ => new SolidColorBrush(Color.FromArgb(0xee, 0xff, 0xff, 0xff)),
    };

    private sealed class LogOutputDto
    {
        [JsonPropertyName("Title")] public string Title { get; set; } = "";
        [JsonPropertyName("Entries")] public List<LogEntryDto>? Entries { get; set; }
        [JsonPropertyName("DisplayLevel")] public int DisplayLevel { get; set; }
        [JsonPropertyName("CollectionLevel")] public int CollectionLevel { get; set; }
    }

    private sealed class LogEntryDto
    {
        [JsonPropertyName("Seq")] public ulong Seq { get; set; }
        [JsonPropertyName("Time")] public DateTime Time { get; set; }
        [JsonPropertyName("Level")] public int Level { get; set; }
        [JsonPropertyName("Message")] public string Message { get; set; } = "";
    }

    private sealed class LogRowVm
    {
        public string Display { get; }
        public int Level { get; }
        public LogRowVm(LogEntryDto e)
        {
            Display = $"{e.Time.ToLocalTime():HH:mm:ss}  {e.Message}";
            Level = e.Level;
        }
    }
}
