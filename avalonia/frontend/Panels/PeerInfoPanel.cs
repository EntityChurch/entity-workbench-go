using System;
using System.Collections.Generic;
using System.Collections.ObjectModel;
using System.Runtime.InteropServices;
using System.Text.Json;
using System.Text.Json.Serialization;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Controls.Templates;
using Avalonia.Layout;
using Avalonia.Media;
using Avalonia.Threading;

namespace EntityAvalonia.Panels;

// PeerInfoPanel renders the per-peer summary stats (entity count, path
// count, sorted path list). Driven by wb.PeerInfoModel inside the Go
// bridge — opens a peer-info handle on construction, registers a wake
// callback, pulls a fresh snapshot every wake.
//
// Mirrors TreeViewPanel's open/registerWake/render/close lifecycle.
// Cascades cleanly when the peer is destroyed (bridge-side
// cascadePeerInfos closes our handle before we get to dispose).
public sealed class PeerInfoPanel : UserControl, IDisposable
{
    // P4 (bounded list) — `Paths` is unbounded peer-side. A long-lived
    // peer can accumulate tens of thousands of distinct paths; rendering
    // them all is both pointless and a D15 violation. Show the first
    // MaxPathsShown and surface the truncation in the stats line.
    private const int MaxPathsShown = 1000;

    private readonly long _peerHandle;
    private readonly long _handle;
    private readonly TextBlock _statsLine;
    private readonly TextBlock _header;
    private readonly ListBox _pathList;
    private readonly ObservableCollection<string> _paths = new();

    private Bridge.TreeWakeCallback? _wakeCallback;
    // Explicit GC root — see TreeViewPanel for full rationale.
    private GCHandle _wakeCallbackHandle;
    private bool _renderQueued;
    private bool _disposed;

    public PeerInfoPanel(long peerHandle)
    {
        _peerHandle = peerHandle;
        var openReply = Bridge.TakeString(Bridge.PeerInfoOpen(peerHandle));
        _handle = ParseHandle(openReply);
        if (_handle < 0)
        {
            Content = new SelectableTextBlock
            {
                Text = $"peer-info open failed: {openReply}",
                Foreground = Brushes.IndianRed,
                Margin = new Thickness(12),
                FontSize = 14,
            };
            return;
        }

        _header = new TextBlock
        {
            Text = "Peer Info",
            FontWeight = FontWeight.SemiBold,
            FontSize = 14,
            Margin = new Thickness(0, 0, 0, 6),
            Opacity = 0.8,
        };

        _statsLine = new TextBlock
        {
            Text = "(loading…)",
            FontFamily = new FontFamily("monospace"),
            FontSize = 13,
            Opacity = 0.85,
            Margin = new Thickness(0, 0, 0, 8),
        };

        _pathList = new ListBox
        {
            ItemsSource = _paths,
            FontFamily = new FontFamily("monospace"),
            FontSize = 12,
            Background = Brushes.Transparent,
            BorderThickness = new Thickness(0),
            ItemTemplate = new FuncDataTemplate<string>((path, _) =>
                new SelectableTextBlock
                {
                    Text = path,
                    Opacity = 0.7,
                    TextWrapping = TextWrapping.NoWrap,
                }, supportsRecycling: true),
        };

        var dock = new DockPanel
        {
            LastChildFill = true,
            Margin = new Thickness(8),
        };
        DockPanel.SetDock(_header, Dock.Top);
        DockPanel.SetDock(_statsLine, Dock.Top);
        dock.Children.Add(_header);
        dock.Children.Add(_statsLine);
        dock.Children.Add(new ScrollViewer { Content = _pathList });
        Content = dock;

        _wakeCallback = OnWakeFromGo;
        _wakeCallbackHandle = GCHandle.Alloc(_wakeCallback);
        var cbPtr = Marshal.GetFunctionPointerForDelegate(_wakeCallback);
        Bridge.TakeString(Bridge.PeerInfoRegisterWake(_handle, cbPtr));
        PanelLog.Write("peer-info", $"Mount h={_handle}");
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
        PanelLog.Write("peer-info", $"Render h={_handle}");
        var reply = Bridge.TakeString(Bridge.PeerInfoRender(_handle));
        PeerInfoOutputDto? dto = null;
        try
        {
            using var doc = JsonDocument.Parse(reply);
            if (!doc.RootElement.TryGetProperty("ok", out var ok) || !ok.GetBoolean())
            {
                _statsLine.Text = reply;
                _statsLine.Foreground = Brushes.IndianRed;
                return;
            }
            dto = doc.RootElement.GetProperty("result").Deserialize<PeerInfoOutputDto>();
        }
        catch (Exception ex)
        {
            _statsLine.Text = $"peer-info render parse failed: {ex.Message}";
            _statsLine.Foreground = Brushes.OrangeRed;
            return;
        }
        if (dto == null)
        {
            _statsLine.Text = "(empty)";
            _paths.Clear();
            return;
        }
        _statsLine.ClearValue(TextBlock.ForegroundProperty);

        _paths.Clear();
        int totalPaths = dto.Paths?.Count ?? 0;
        bool truncated = totalPaths > MaxPathsShown;
        if (dto.Paths != null)
        {
            int take = truncated ? MaxPathsShown : totalPaths;
            for (int i = 0; i < take; i++)
            {
                _paths.Add(dto.Paths[i]);
            }
        }
        _statsLine.Text = truncated
            ? $"entities {dto.EntityCount}  ·  paths {dto.PathCount}  ·  showing first {MaxPathsShown} of {totalPaths}"
            : $"entities {dto.EntityCount}  ·  paths {dto.PathCount}";
    }

    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        PanelLog.Write("peer-info", $"Dispose h={_handle}");
        if (_handle >= 0)
        {
            Bridge.PeerInfoClose(_handle);
        }
        _wakeCallback = null;
        if (_wakeCallbackHandle.IsAllocated) _wakeCallbackHandle.Free();
    }

    private sealed class PeerInfoOutputDto
    {
        [JsonPropertyName("EntityCount")] public int EntityCount { get; set; }
        [JsonPropertyName("PathCount")] public int PathCount { get; set; }
        [JsonPropertyName("Paths")] public List<string>? Paths { get; set; }
    }
}
