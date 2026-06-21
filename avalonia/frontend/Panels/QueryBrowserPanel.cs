using System;
using System.Collections.Generic;
using System.Collections.ObjectModel;
using System.Text.Json;
using System.Text.Json.Serialization;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Controls.Templates;
using Avalonia.Input;
using Avalonia.Layout;
using Avalonia.Media;

namespace EntityAvalonia.Panels;

// QueryBrowserPanel renders wb.QueryModel — type-filter + path-prefix
// query against the local peer. Pull-only (no wake): user edits the
// filters, hits Enter or the Search button, results render. Selecting
// a result publishes its path via IPanelHost.PublishSelectedPath so a
// Detail panel mounted elsewhere can follow.
//
// Console binding parity: Enter on either input executes; n key on
// the result list advances pagination; Up/Down navigate selection.
public sealed class QueryBrowserPanel : UserControl, IDisposable
{
    // P4 (bounded list) — the bridge paginates server-side via
    // HasMore, but a single page that returns more than MaxClientRows
    // would still blow the visual tree. Defensive client-side cap
    // sits below the HasMore flow: if the page itself overflows, we
    // render the first MaxClientRows and surface the truncation in
    // the status line. Normal page sizes (a couple hundred) never
    // hit this; the cap is the floor against an unexpectedly large
    // page from a future model change.
    private const int MaxClientRows = 1000;

    private readonly long _peerHandle;
    private readonly long _handle;
    private readonly IPanelHost? _host;
    private readonly TextBox _typeInput;
    private readonly TextBox _pathInput;
    private readonly Button _searchBtn;
    private readonly TextBlock _statusLine;
    private readonly ListBox _matchList;
    private readonly ObservableCollection<MatchVm> _matches = new();

    private bool _disposed;
    private string _currentTypeFilter = "";
    private string _currentPathPrefix = "";

    public QueryBrowserPanel(long peerHandle, IPanelHost? host)
    {
        _peerHandle = peerHandle;
        _host = host;

        var openReply = Bridge.TakeString(Bridge.QueryOpen(peerHandle));
        _handle = ParseHandle(openReply);
        if (_handle < 0)
        {
            Content = new SelectableTextBlock
            {
                Text = $"query open failed: {openReply}",
                Foreground = Brushes.IndianRed,
                Margin = new Thickness(12),
                FontSize = 14,
            };
            return;
        }

        _typeInput = new TextBox
        {
            Watermark = "type (e.g. doc/markdown-file)",
            FontFamily = new FontFamily("monospace"),
            FontSize = 13,
            Margin = new Thickness(0, 0, 0, 4),
        };
        _typeInput.KeyDown += OnFilterKey;

        _pathInput = new TextBox
        {
            Watermark = "path prefix (e.g. docs/)",
            FontFamily = new FontFamily("monospace"),
            FontSize = 13,
            Margin = new Thickness(0, 0, 0, 4),
        };
        _pathInput.KeyDown += OnFilterKey;

        _searchBtn = new Button
        {
            Content = "Search",
            FontSize = 12,
            Padding = new Thickness(10, 4),
            HorizontalAlignment = HorizontalAlignment.Right,
            Margin = new Thickness(0, 0, 0, 6),
        };
        _searchBtn.Click += (_, _) => ExecuteQuery();

        _statusLine = new TextBlock
        {
            Text = "(no query yet — set filters, hit Enter or Search)",
            FontFamily = new FontFamily("monospace"),
            FontSize = 12,
            Opacity = 0.6,
            Margin = new Thickness(0, 0, 0, 4),
        };

        _matchList = new ListBox
        {
            ItemsSource = _matches,
            FontFamily = new FontFamily("monospace"),
            FontSize = 13,
            Background = Brushes.Transparent,
            BorderThickness = new Thickness(0),
            ItemTemplate = new FuncDataTemplate<MatchVm>((vm, _) =>
            {
                var stack = new StackPanel
                {
                    Orientation = Orientation.Horizontal,
                    Spacing = 8,
                };
                stack.Children.Add(new SelectableTextBlock
                {
                    Text = string.IsNullOrEmpty(vm.Path) ? "(no path)" : vm.Path,
                    FontSize = 13,
                });
                stack.Children.Add(new TextBlock
                {
                    Text = vm.TypeName,
                    FontSize = 12,
                    Opacity = 0.6,
                    VerticalAlignment = VerticalAlignment.Center,
                });
                return stack;
            }, supportsRecycling: true),
        };
        _matchList.SelectionChanged += (_, _) =>
        {
            var idx = _matchList.SelectedIndex;
            if (idx < 0 || idx >= _matches.Count) return;
            // Broadcast the path so detail / markdown-view in another
            // slot follows. The bridge's selection cursor is for the
            // model's PublishSelection wiring (which we don't use here
            // — we route via IPanelHost instead).
            var path = _matches[idx].Path;
            if (!string.IsNullOrEmpty(path) && _host != null)
            {
                _host.PublishSelectedPath(path);
            }
        };
        _matchList.KeyDown += (_, e) =>
        {
            if (e.Key == Key.N)
            {
                Bridge.TakeString(Bridge.QueryNextPage(_handle));
                RerenderFromBridge();
                e.Handled = true;
            }
        };

        var inputsRow = new StackPanel
        {
            Orientation = Orientation.Vertical,
        };
        inputsRow.Children.Add(LabeledRow("type", _typeInput));
        inputsRow.Children.Add(LabeledRow("path", _pathInput));
        inputsRow.Children.Add(_searchBtn);
        inputsRow.Children.Add(_statusLine);

        var dock = new DockPanel
        {
            LastChildFill = true,
            Margin = new Thickness(10),
        };
        DockPanel.SetDock(inputsRow, Dock.Top);
        dock.Children.Add(inputsRow);
        dock.Children.Add(new ScrollViewer { Content = _matchList });
        Content = dock;
        PanelLog.Write("query-browser", $"Mount h={_handle}");
    }

    private void OnFilterKey(object? sender, KeyEventArgs e)
    {
        if (e.Key == Key.Enter)
        {
            ExecuteQuery();
            e.Handled = true;
        }
    }

    private void ExecuteQuery()
    {
        _currentTypeFilter = _typeInput.Text ?? "";
        _currentPathPrefix = _pathInput.Text ?? "";
        PanelLog.Write("query-browser",
            $"Execute h={_handle} type='{_currentTypeFilter}' path='{_currentPathPrefix}'");
        Bridge.TakeString(Bridge.QuerySetFilters(_handle, _currentTypeFilter, _currentPathPrefix));
        Bridge.TakeString(Bridge.QueryExecute(_handle));
        RerenderFromBridge();
    }

    private void RerenderFromBridge()
    {
        if (_handle < 0) return;
        PanelLog.Write("query-browser", $"Render h={_handle}");
        var reply = Bridge.TakeString(Bridge.QueryRender(_handle));
        QueryOutputDto? dto = null;
        try
        {
            using var doc = JsonDocument.Parse(reply);
            if (!doc.RootElement.TryGetProperty("ok", out var ok) || !ok.GetBoolean())
            {
                _statusLine.Text = reply;
                _statusLine.Foreground = Brushes.IndianRed;
                return;
            }
            dto = doc.RootElement.GetProperty("result").Deserialize<QueryOutputDto>();
        }
        catch (Exception ex)
        {
            _statusLine.Text = $"query render parse failed: {ex.Message}";
            _statusLine.Foreground = Brushes.OrangeRed;
            return;
        }
        if (dto == null)
        {
            _matches.Clear();
            _statusLine.Text = "(empty)";
            return;
        }

        _matches.Clear();
        int totalMatches = dto.Matches?.Count ?? 0;
        bool clientCapped = totalMatches > MaxClientRows;
        if (dto.Matches != null)
        {
            int take = clientCapped ? MaxClientRows : totalMatches;
            for (int i = 0; i < take; i++)
            {
                var m = dto.Matches[i];
                _matches.Add(new MatchVm(m.Path, m.TypeName));
            }
        }

        string baseStatus = string.IsNullOrEmpty(dto.Status)
            ? $"{totalMatches} match(es)"
            : dto.Status;
        if (clientCapped)
        {
            baseStatus += $"  (showing first {MaxClientRows} of {totalMatches})";
        }
        if (dto.HasMore)
        {
            baseStatus += "  (press n in result list for next page)";
        }
        _statusLine.Text = baseStatus;
        _statusLine.ClearValue(TextBlock.ForegroundProperty);
        _statusLine.Opacity = 0.7;

        if (dto.Selected >= 0 && dto.Selected < _matches.Count)
        {
            _matchList.SelectedIndex = dto.Selected;
        }
    }

    private static Control LabeledRow(string label, Control input)
    {
        var grid = new Grid
        {
            ColumnDefinitions = new ColumnDefinitions("64,*"),
            Margin = new Thickness(0, 0, 0, 2),
        };
        var labelControl = new TextBlock
        {
            Text = label,
            FontSize = 12,
            Opacity = 0.6,
            VerticalAlignment = VerticalAlignment.Center,
            Margin = new Thickness(0, 0, 6, 0),
        };
        Grid.SetColumn(labelControl, 0);
        Grid.SetColumn(input, 1);
        grid.Children.Add(labelControl);
        grid.Children.Add(input);
        return grid;
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

    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        PanelLog.Write("query-browser", $"Dispose h={_handle}");
        if (_handle >= 0)
        {
            Bridge.QueryClose(_handle);
        }
    }

    private sealed class QueryOutputDto
    {
        [JsonPropertyName("type_filter")] public string TypeFilter { get; set; } = "";
        [JsonPropertyName("path_prefix")] public string PathPrefix { get; set; } = "";
        [JsonPropertyName("matches")] public List<MatchDto>? Matches { get; set; }
        [JsonPropertyName("total")] public ulong Total { get; set; }
        [JsonPropertyName("has_more")] public bool HasMore { get; set; }
        [JsonPropertyName("selected")] public int Selected { get; set; }
        [JsonPropertyName("status")] public string Status { get; set; } = "";
        [JsonPropertyName("has_executed")] public bool HasExecuted { get; set; }
    }

    private sealed class MatchDto
    {
        [JsonPropertyName("path")] public string Path { get; set; } = "";
        [JsonPropertyName("type")] public string TypeName { get; set; } = "";
    }

    private sealed class MatchVm
    {
        public string Path { get; }
        public string TypeName { get; }
        public MatchVm(string path, string typeName) { Path = path; TypeName = typeName; }
    }
}
