using System;
using System.Collections.Generic;
using System.Collections.ObjectModel;
using System.Text.Json;
using System.Text.Json.Serialization;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Controls.Templates;
using Avalonia.Layout;
using Avalonia.Media;

namespace EntityAvalonia.Panels;

// DetailPanel renders the formatted output of `get <path>` for the
// currently-selected entity. Driven externally by ShowEntity(path) —
// MainWindow forwards TreeViewPanel.EntitySelected events here.
// Pure read-only; no input controls. Selectable text so users can
// copy hashes, type names, etc.
public sealed class DetailPanel : UserControl, IDisposable
{
    private readonly long _peerHandle;
    private readonly IPanelHost? _host;
    private readonly Action<string>? _selectedPathHandler;
    private readonly TextBlock _pathLabel;
    private readonly ListBox _lines;
    private readonly ObservableCollection<DetailLineVm> _rows = new();
    private bool _disposed;

    public DetailPanel(long peerHandle) : this(peerHandle, null) { }

    public DetailPanel(long peerHandle, IPanelHost? host)
    {
        _peerHandle = peerHandle;
        _host = host;
        if (host != null)
        {
            _selectedPathHandler = path => ShowEntity(path);
            host.SelectedPath += _selectedPathHandler;
        }
        _pathLabel = new TextBlock
        {
            Text = "(no entity selected)",
            FontFamily = new FontFamily("monospace"),
            FontSize = 13,
            Opacity = 0.7,
            Margin = new Thickness(0, 0, 0, 6),
        };

        _lines = new ListBox
        {
            ItemsSource = _rows,
            FontFamily = new FontFamily("monospace"),
            FontSize = 13,
            Background = Brushes.Transparent,
            BorderThickness = new Thickness(0),
            ItemTemplate = new FuncDataTemplate<DetailLineVm>((vm, _) =>
                new SelectableTextBlock
                {
                    Text = vm.Text,
                    Foreground = ColorForKind(vm.Kind),
                    TextWrapping = TextWrapping.NoWrap,
                }, supportsRecycling: true),
        };

        var header = new TextBlock
        {
            Text = "Detail",
            FontWeight = FontWeight.SemiBold,
            FontSize = 14,
            Margin = new Thickness(0, 0, 0, 6),
            Opacity = 0.8,
        };

        var dock = new DockPanel
        {
            LastChildFill = true,
            Margin = new Thickness(8),
        };
        DockPanel.SetDock(header, Dock.Top);
        DockPanel.SetDock(_pathLabel, Dock.Top);
        dock.Children.Add(header);
        dock.Children.Add(_pathLabel);
        dock.Children.Add(new ScrollViewer { Content = _lines });
        Content = dock;

        PanelLog.Write("detail", $"Mount peerHandle={_peerHandle}");

        // If a path is already selected, seed the panel with it so a
        // mid-session swap doesn't show "(no entity selected)" until
        // the user clicks something new.
        if (host?.CurrentSelectedPath is string seed && !string.IsNullOrEmpty(seed))
        {
            ShowEntity(seed);
        }
    }

    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        PanelLog.Write("detail", "Dispose");
        if (_host != null && _selectedPathHandler != null)
        {
            _host.SelectedPath -= _selectedPathHandler;
        }
    }

    public void ShowEntity(string path)
    {
        if (string.IsNullOrEmpty(path)) return;
        PanelLog.Write("detail", $"ShowEntity path={path}");
        _pathLabel.Text = path;
        _pathLabel.Opacity = 1.0;
        var reply = Bridge.TakeString(Bridge.EntityGet(_peerHandle, path));
        EntityGetRespDto? dto = null;
        try
        {
            dto = JsonSerializer.Deserialize<EntityGetRespDto>(reply);
        }
        catch { }

        _rows.Clear();
        if (dto?.Lines == null)
        {
            _rows.Add(new DetailLineVm($"<unparseable reply> {reply}", "error"));
            return;
        }
        foreach (var l in dto.Lines)
        {
            _rows.Add(new DetailLineVm(l.Text, l.Kind));
        }
    }

    private static IBrush ColorForKind(string kind) => kind switch
    {
        "path" => new SolidColorBrush(Color.FromRgb(0x7e, 0xc5, 0xff)),
        "hash" => new SolidColorBrush(Color.FromRgb(0xc8, 0xa8, 0xff)),
        "key" => new SolidColorBrush(Color.FromRgb(0xff, 0xd6, 0x7a)),
        "string" => new SolidColorBrush(Color.FromRgb(0xb5, 0xe8, 0x99)),
        "number" => new SolidColorBrush(Color.FromRgb(0xff, 0xb3, 0x86)),
        "error" => Brushes.IndianRed,
        "null" => new SolidColorBrush(Color.FromArgb(0xaa, 0xff, 0xff, 0xff)),
        _ => new SolidColorBrush(Color.FromArgb(0xdd, 0xff, 0xff, 0xff)),
    };

    private sealed class EntityGetRespDto
    {
        [JsonPropertyName("ok")] public bool Ok { get; set; }
        [JsonPropertyName("lines")] public List<EntityGetLineDto>? Lines { get; set; }
    }

    private sealed class EntityGetLineDto
    {
        [JsonPropertyName("text")] public string Text { get; set; } = "";
        [JsonPropertyName("kind")] public string Kind { get; set; } = "";
    }

    private sealed class DetailLineVm
    {
        public string Text { get; }
        public string Kind { get; }
        public DetailLineVm(string text, string kind) { Text = text; Kind = kind; }
    }
}
