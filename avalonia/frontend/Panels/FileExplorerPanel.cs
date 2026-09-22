using System;
using System.Collections.Generic;
using System.Collections.ObjectModel;
using System.Globalization;
using System.Runtime.InteropServices;
using System.Text.Json;
using System.Text.Json.Serialization;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Layout;
using Avalonia.Media;
using Avalonia.Threading;

namespace EntityAvalonia.Panels;

// FileExplorerPanel — the mounted directory, as a file explorer.
//
// **Why this exists.** You could mount a directory and then do nothing
// with it. The Local Files panel listed MOUNTS — root, filesystem root,
// target prefix, a count, three buttons — and never a file. The raw tree
// showed entity paths with CBOR behind them. The Markdown Files panel
// read the hard-coded prefix `docs/`, so a mount to any other prefix was
// invisible there even for the one kind that ingested at all. So the
// mount succeeded, a number went up, and nothing openable appeared
// anywhere. An operator's report, verbatim: *"I mounted the thing.
// Wasn't able to really do much with it after that."*
//
// **Avalonia has no built-in file explorer control.** The framework's own
// sample explorer is built on `Avalonia.Controls.TreeDataGrid`, a
// separate package. This panel deliberately does not take that
// dependency: TreeDataGrid brings its own templating surface, and AP46 —
// `FuncDataTemplate` being invoked with `null` during container teardown,
// which shipped a null dereference at nineteen sites and aborted the
// process — is a hazard we have tamed exactly once, at `Rows.Of`. A new
// templating engine is that hazard re-opened somewhere the guard test
// does not look. Grid + ListBox + Rows.Of is more code here and less
// surface everywhere.
//
// **What a row asserts, and what it refuses to imply.** Every file
// carries a STATUS naming why it does or does not have a document. The
// old surface's single file count came from the SOURCE layer, where the
// watcher writes a record for everything it admits — the layer that never
// fails — so a mount of 400 photographs and one README read as "401
// entities in tree" with exactly one openable document in it. The summary
// here reports ingested and not-ingested separately for that reason, and
// a mount with no known target prefix says *unknown* rather than
// rendering every row as absent (AP45).
public sealed class FileExplorerPanel : UserControl, IDisposable, IPanelPreferredHeight
{
    // Chrome floor: mount picker + file list beside a preview pane.
    // Declared because the 200px stack default clipped this panel the
    // moment a second one was open — see IPanelPreferredHeight.
    public double PreferredSlotMinHeight => 440;
    // P3 (wake debounce). A watcher's initial scan of a large directory
    // fires thousands of tree events; the bridge coalesces them into a
    // one-deep channel and this coalesces the remainder into one render
    // per burst. Same interval as TreeViewPanel, for the same reason.
    private static readonly TimeSpan WakeDebounce = TimeSpan.FromMilliseconds(150);

    // P4 (bounded list). A single directory with more entries than this
    // is a listing nobody reads; the count is stated when it truncates,
    // because a silently short list is a wrong answer (AP15's shape at a
    // render surface).
    private const int MaxRowsShown = 2000;

    private readonly long _peerHandle;
    private readonly long _handle;
    private readonly IPanelHost? _host;

    private readonly ComboBox _mountPicker;
    private readonly Button _upButton;
    private readonly StackPanel _crumbs;
    private readonly TextBlock _summary;
    private readonly ListBox _list;
    private readonly SelectableTextBlock _preview;
    private readonly TextBlock _previewHeader;
    private readonly SelectableTextBlock _detail;
    private readonly ObservableCollection<EntryVm> _rows = new();
    private readonly List<string> _mountRoots = new();

    private Bridge.TreeWakeCallback? _wakeCallback;
    // Explicit GC root. The field reference alone is NOT sufficient — the
    // runtime can collect the delegate whenever the field is not visibly
    // used, and Go holding a function pointer to a freed marshaled stub
    // aborts the process with "a callback was made on a garbage collected
    // delegate". Freed in Dispose only AFTER FileExplorerClose has joined
    // the wake goroutine.
    private GCHandle _wakeCallbackHandle;
    private DispatcherTimer? _wakeTimer;
    private bool _renderQueued;
    private bool _disposed;
    private bool _suppressMountChange;
    private string _selectedRelPath = "";

    // Test/driver surface. Headless tests drive these rather than
    // synthesizing clicks for navigation; the click paths themselves are
    // covered by MouseDown/MouseUp at hit-tested coordinates, because a
    // test that invokes the handler passes against an unclickable
    // control (AP47).
    public int RowCount => _rows.Count;
    public List<EntryVm> VisibleRowsForTests => new(_rows);
    public long ExplorerHandleForTests => _handle;
    public string SummaryText => _summary.Text ?? "";
    public string PreviewText => _preview.Text ?? "";
    public string DetailText => _detail.Text ?? "";
    public string PreviewHeaderText => _previewHeader.Text ?? "";
    public IReadOnlyList<string> MountRoots => _mountRoots;

    // Selects a row the way the ListBox does, so `SelectionChanged` fires
    // and `OnRowSelected` runs for real. Not a shortcut past the UI: the
    // wiring at the other end of this event is a plain `+=` on
    // `SelectionChanged`, which Avalonia does deliver (AP37 is about
    // POINTER events on controls that mark them handled), and the
    // operator's own clicks reached it. What no test reached was the
    // handler's body — every file-explorer test asserted on row
    // viewmodels and never selected one.
    public void SelectRowForTests(int index) => _list.SelectedIndex = index;

    public FileExplorerPanel(long peerHandle, IPanelHost? host)
    {
        _peerHandle = peerHandle;
        _host = host;

        var openReply = Bridge.TakeString(Bridge.FileExplorerOpen(peerHandle));
        _handle = ParseHandle(openReply);
        if (_handle < 0)
        {
            // Construct the failure surface and STOP. Every field below
            // stays null, so nothing else in this class may run — which
            // is why the early return is here and not a flag checked in
            // twelve places.
            _mountPicker = new ComboBox();
            _upButton = new Button();
            _crumbs = new StackPanel();
            _summary = new TextBlock();
            _list = new ListBox();
            _preview = new SelectableTextBlock();
            _previewHeader = new TextBlock();
            _detail = new SelectableTextBlock();
            Content = new SelectableTextBlock
            {
                Text = $"file explorer failed to open: {openReply}",
                Foreground = Brushes.IndianRed,
                Margin = new Thickness(12),
                FontSize = 14,
            };
            return;
        }

        _mountPicker = new ComboBox
        {
            PlaceholderText = "no mounts on this peer",
            MinWidth = 180,
            FontSize = 12,
        };
        _mountPicker.SelectionChanged += (_, _) => OnMountPicked();

        _upButton = new Button { Content = "↑ Up", FontSize = 12, IsEnabled = false };
        _upButton.Click += (_, _) => { Bridge.TakeString(Bridge.FileExplorerUp(_handle)); Refresh(); };

        var refreshButton = new Button { Content = "⟳", FontSize = 12 };
        ToolTip.SetTip(refreshButton, "Re-read this mount from the tree");
        refreshButton.Click += (_, _) => { ReloadMounts(); Refresh(); };

        _crumbs = new StackPanel
        {
            Orientation = Orientation.Horizontal,
            Spacing = 2,
            VerticalAlignment = VerticalAlignment.Center,
        };

        _summary = new TextBlock
        {
            FontSize = 12,
            Foreground = Brushes.DarkGray,
            TextWrapping = TextWrapping.Wrap,
            Margin = new Thickness(12, 2, 12, 6),
        };

        _list = new ListBox
        {
            ItemsSource = _rows,
            Background = Brushes.Transparent,
            FontSize = 13,
            // AP46: never `new FuncDataTemplate<T>` — Avalonia calls the
            // builder with null during container teardown, and the crash
            // is a SIGABRT rather than an exception because
            // Dispatcher.UnhandledException declines to set Handled.
            ItemTemplate = Rows.Of<EntryVm>((vm, _) => BuildRow(vm), supportsRecycling: true),
        };
        _list.DoubleTapped += (_, _) => ActivateSelectedRow();
        _list.KeyDown += (_, e) =>
        {
            if (e.Key == Avalonia.Input.Key.Enter)
            {
                ActivateSelectedRow();
                e.Handled = true;
            }
            else if (e.Key == Avalonia.Input.Key.Back)
            {
                Bridge.TakeString(Bridge.FileExplorerUp(_handle));
                Refresh();
                e.Handled = true;
            }
        };
        _list.SelectionChanged += (_, _) => OnRowSelected();

        _previewHeader = new TextBlock
        {
            Text = "select a file",
            FontSize = 12,
            Foreground = Brushes.DarkGray,
            Margin = new Thickness(8, 6, 8, 4),
            TextWrapping = TextWrapping.Wrap,
        };
        _preview = new SelectableTextBlock
        {
            Text = "",
            FontSize = 12,
            FontFamily = new FontFamily("monospace"),
            Margin = new Thickness(8, 0, 8, 8),
            TextWrapping = TextWrapping.NoWrap,
            Foreground = Brushes.Gainsboro,
        };
        _detail = new SelectableTextBlock
        {
            Text = "",
            FontSize = 11,
            Foreground = Brushes.DarkGray,
            Margin = new Thickness(12, 2, 12, 6),
            TextWrapping = TextWrapping.Wrap,
        };

        Content = BuildLayout(refreshButton);

        ReloadMounts();

        _wakeCallback = OnWakeFromGo;
        _wakeCallbackHandle = GCHandle.Alloc(_wakeCallback);
        Bridge.TakeString(Bridge.FileExplorerRegisterWake(
            _handle, Marshal.GetFunctionPointerForDelegate(_wakeCallback)));
        PanelLog.Write("file-explorer", $"Mount h={_handle}");
    }

    private Control BuildLayout(Button refreshButton)
    {
        var toolbar = new StackPanel
        {
            Orientation = Orientation.Horizontal,
            Spacing = 6,
            Margin = new Thickness(12, 10, 12, 4),
            VerticalAlignment = VerticalAlignment.Center,
        };
        toolbar.Children.Add(new TextBlock
        {
            Text = "Folder",
            FontSize = 12,
            Foreground = Brushes.DarkGray,
            VerticalAlignment = VerticalAlignment.Center,
        });
        toolbar.Children.Add(_mountPicker);
        toolbar.Children.Add(_upButton);
        toolbar.Children.Add(refreshButton);
        toolbar.Children.Add(_crumbs);

        // Listing left, preview right, draggable split between them. The
        // splitter is a real GridSplitter rather than a fixed width
        // because a file name column and a source file want opposite
        // amounts of room and only the operator knows which they are
        // reading.
        var split = new Grid
        {
            ColumnDefinitions = new ColumnDefinitions("2*,4,3*"),
        };
        var listBorder = new Border
        {
            Child = new ScrollViewer { Content = _list },
            BorderBrush = new SolidColorBrush(Color.FromRgb(0x33, 0x33, 0x33)),
            BorderThickness = new Thickness(0, 1, 1, 0),
        };
        var previewPane = new DockPanel();
        DockPanel.SetDock(_previewHeader, Dock.Top);
        previewPane.Children.Add(_previewHeader);
        previewPane.Children.Add(new ScrollViewer
        {
            Content = _preview,
            HorizontalScrollBarVisibility = Avalonia.Controls.Primitives.ScrollBarVisibility.Auto,
        });

        var splitter = new GridSplitter { Background = new SolidColorBrush(Color.FromRgb(0x2a, 0x2a, 0x2a)) };
        Grid.SetColumn(listBorder, 0);
        Grid.SetColumn(splitter, 1);
        Grid.SetColumn(previewPane, 2);
        split.Children.Add(listBorder);
        split.Children.Add(splitter);
        split.Children.Add(previewPane);

        var root = new DockPanel();
        DockPanel.SetDock(toolbar, Dock.Top);
        DockPanel.SetDock(_summary, Dock.Top);
        DockPanel.SetDock(_detail, Dock.Bottom);
        root.Children.Add(toolbar);
        root.Children.Add(_summary);
        root.Children.Add(_detail);
        root.Children.Add(split);
        return root;
    }

    // --- Mount selection ------------------------------------------------

    // ReloadMounts repopulates the picker from the mount list and keeps
    // the current selection if it survived. Public so a test can drive it
    // after mounting through the Local Files panel.
    public void ReloadMounts()
    {
        if (_handle < 0) return;
        var previous = _mountPicker.SelectedIndex >= 0 && _mountPicker.SelectedIndex < _mountRoots.Count
            ? _mountRoots[_mountPicker.SelectedIndex]
            : "";

        _mountRoots.Clear();
        var reply = Bridge.TakeString(Bridge.LocalFilesRender(_peerHandle));
        try
        {
            var view = JsonSerializer.Deserialize<LocalFilesPanel.RenderEnvelope>(reply, JsonOpts);
            foreach (var m in view?.Mounts ?? new List<LocalFilesPanel.MountRow>())
            {
                if (!string.IsNullOrEmpty(m.Root)) _mountRoots.Add(m.Root);
            }
        }
        catch (JsonException ex)
        {
            _summary.Text = $"could not read the mount list: {ex.Message}";
            _summary.Foreground = Brushes.IndianRed;
            return;
        }

        // Suppress the SelectionChanged fan-out while rebuilding, or
        // repopulating the picker re-binds the model and resets the
        // operator's current directory on every wake.
        _suppressMountChange = true;
        _mountPicker.ItemsSource = new List<string>(_mountRoots);
        var restore = _mountRoots.IndexOf(previous);
        _mountPicker.SelectedIndex = restore >= 0 ? restore : (_mountRoots.Count > 0 ? 0 : -1);
        _suppressMountChange = false;

        if (_mountRoots.Count == 0)
        {
            _rows.Clear();
            _summary.Text = "No folders are mounted on this peer yet. Open the "
                          + "\"Local Files\" panel to mount one — pick a directory and a tree "
                          + "prefix, and its contents appear here.";
            _summary.Foreground = Brushes.Gainsboro;
            _upButton.IsEnabled = false;
            _crumbs.Children.Clear();
            return;
        }
        BindSelectedMount();
    }

    private void OnMountPicked()
    {
        if (_suppressMountChange) return;
        BindSelectedMount();
    }

    private void BindSelectedMount()
    {
        var idx = _mountPicker.SelectedIndex;
        if (idx < 0 || idx >= _mountRoots.Count) return;
        var reply = Bridge.TakeString(Bridge.FileExplorerSetRoot(_handle, _mountRoots[idx]));
        if (!IsOk(reply))
        {
            _rows.Clear();
            _summary.Text = reply;
            _summary.Foreground = Brushes.IndianRed;
            return;
        }
        Refresh();
    }

    // --- Navigation -----------------------------------------------------

    // Navigate enters a directory relative to the mount root. Public so a
    // headless test can walk the tree without synthesizing double-taps.
    public void Navigate(string relDir)
    {
        if (_handle < 0) return;
        Bridge.TakeString(Bridge.FileExplorerSetDir(_handle, relDir ?? ""));
        Refresh();
    }

    private void ActivateSelectedRow()
    {
        if (_list.SelectedIndex < 0 || _list.SelectedIndex >= _rows.Count) return;
        var vm = _rows[_list.SelectedIndex];
        if (vm.IsDir)
        {
            Navigate(vm.RelPath);
            return;
        }
        // A file's activation is "show it to the rest of the workspace".
        // The path published is the DOCUMENT, never the source file
        // record: the source entity is a `local/files/file` whose body is
        // a hash and a size, and pointing the Detail panel at it would
        // answer a question nobody asked.
        if (!string.IsNullOrEmpty(vm.TargetPath))
        {
            _host?.PublishSelectedPath(vm.TargetPath);
        }
    }

    private void OnRowSelected()
    {
        if (_list.SelectedIndex < 0 || _list.SelectedIndex >= _rows.Count)
        {
            _detail.Text = "";
            return;
        }
        var vm = _rows[_list.SelectedIndex];
        _selectedRelPath = vm.RelPath;

        if (vm.IsDir)
        {
            _detail.Text = $"{vm.RelPath}/ — {vm.ChildFiles} file(s), {HumanBytes(vm.ChildBytes)}";
            _detail.Foreground = Brushes.DarkGray;
            _previewHeader.Text = "select a file";
            _preview.Text = "";
            return;
        }

        var bits = new List<string> { HumanBytes(vm.Size), vm.Kind };
        if (!string.IsNullOrEmpty(vm.Language)) bits.Add(vm.Language);
        if (!string.IsNullOrEmpty(vm.MediaType)) bits.Add(vm.MediaType);
        var mtime = FormatMtime(vm.ModifiedAtMillis);
        if (mtime.Length > 0) bits.Add(mtime);
        // Both tree paths, always. An operator debugging an ingest needs
        // to know where the source record is as well as where the
        // document is or is not.
        bits.Add(vm.Ingested ? $"document at {vm.TargetPath} ({vm.EntityType})" : vm.Status);
        bits.Add($"source {vm.SourcePath}");
        _detail.Text = string.Join("  ·  ", bits);
        _detail.Foreground = vm.Ingested ? Brushes.DarkGray : Brushes.Orange;

        LoadPreview(vm);
    }

    // An mtime is a number a FILESYSTEM chose, not one we did, and it
    // reaches this method having crossed the kernel's `*uint64` (which
    // names no unit), CBOR, cgo and JSON. Rendering it must not be able
    // to end the process.
    //
    // It could, and it did. The field was documented as seconds and every
    // producer writes `UnixMilli()`, so `FromUnixTimeSeconds` threw
    // `ArgumentOutOfRangeException` on every file row an operator clicked
    // — nine of them, which is `MaxContainedUiFaults` + 1, and the ninth
    // took the app down (`run-logs/run-20260902-083012.log`). The unit is
    // fixed at the source and named in the field; this guard is the
    // second half, because the next bad value will come from a real
    // filesystem rather than from us. Returns "" when there is nothing
    // honest to print — the row then simply carries no date, which is
    // what a missing mtime already meant.
    internal static string FormatMtime(long millis)
    {
        if (millis <= 0) return "";
        if (millis < DateTimeOffset.MinValue.ToUnixTimeMilliseconds() ||
            millis > DateTimeOffset.MaxValue.ToUnixTimeMilliseconds())
            return $"mtime out of range ({millis})";
        return DateTimeOffset.FromUnixTimeMilliseconds(millis).LocalDateTime
            .ToString("yyyy-MM-dd HH:mm", CultureInfo.InvariantCulture);
    }

    private void LoadPreview(EntryVm vm)
    {
        var reply = Bridge.TakeString(Bridge.FileExplorerPreview(_handle, vm.RelPath));
        PreviewDto? dto;
        try
        {
            dto = JsonSerializer.Deserialize<PreviewDto>(reply, JsonOpts);
        }
        catch (JsonException ex)
        {
            _previewHeader.Text = $"preview did not decode: {ex.Message}";
            _previewHeader.Foreground = Brushes.IndianRed;
            _preview.Text = "";
            return;
        }
        if (dto == null || !string.IsNullOrEmpty(dto.Error))
        {
            _previewHeader.Text = dto?.Error ?? "empty preview reply";
            _previewHeader.Foreground = Brushes.IndianRed;
            _preview.Text = "";
            return;
        }

        if (!dto.Textual)
        {
            // Not a failure. Rendering a JPEG's bytes as text would be
            // "something rather than nothing", and something wrong is
            // worse than a stated absence.
            _previewHeader.Text = $"{vm.Name} — {dto.Kind}, {HumanBytes(dto.Size)}. "
                                + "No text preview for this kind.";
            _previewHeader.Foreground = Brushes.DarkGray;
            _preview.Text = "";
            return;
        }

        _previewHeader.Text = dto.Truncated
            ? $"{vm.Name} — first {HumanBytes(dto.Text.Length)} of {HumanBytes(dto.Size)} (truncated)"
            : $"{vm.Name} — {HumanBytes(dto.Size)}";
        _previewHeader.Foreground = Brushes.DarkGray;
        _preview.Text = dto.Text;
    }

    // --- Render ---------------------------------------------------------

    // Refresh pulls a fresh snapshot. Public so a headless test drives
    // the panel without waiting on a wake it cannot observe (AP32 — a
    // derived UI property is not a completion signal).
    public void Refresh()
    {
        if (_handle < 0) return;
        var reply = Bridge.TakeString(Bridge.FileExplorerRender(_handle));
        RenderDto? dto;
        try
        {
            dto = JsonSerializer.Deserialize<RenderDto>(reply, JsonOpts);
        }
        catch (JsonException ex)
        {
            _rows.Clear();
            _summary.Text = $"explorer render did not decode: {ex.Message}";
            _summary.Foreground = Brushes.IndianRed;
            return;
        }
        if (dto == null)
        {
            _rows.Clear();
            _summary.Text = "empty explorer render";
            _summary.Foreground = Brushes.IndianRed;
            return;
        }
        if (!string.IsNullOrEmpty(dto.Error))
        {
            _rows.Clear();
            _summary.Text = dto.Error;
            _summary.Foreground = Brushes.IndianRed;
            return;
        }

        // Preserve the selected row across a re-render. A wake fires on
        // every ingest event, and a listing that resets the operator's
        // selection on each one is the "it jumps around" complaint AP49
        // names — the same defect the browser panel had.
        var keep = _selectedRelPath;

        _rows.Clear();
        var truncated = false;
        foreach (var e in dto.Entries ?? new List<EntryVm>())
        {
            if (_rows.Count >= MaxRowsShown) { truncated = true; break; }
            _rows.Add(e);
        }

        _upButton.IsEnabled = !string.IsNullOrEmpty(dto.Dir);
        RebuildCrumbs(dto);
        _summary.Text = BuildSummary(dto, truncated);
        _summary.Foreground = dto.NotIngested > 0 ? Brushes.Orange : Brushes.DarkGray;

        if (!string.IsNullOrEmpty(keep))
        {
            for (var i = 0; i < _rows.Count; i++)
            {
                if (_rows[i].RelPath == keep) { _list.SelectedIndex = i; break; }
            }
        }
    }

    private string BuildSummary(RenderDto dto, bool truncated)
    {
        if (!string.IsNullOrEmpty(dto.Note) && dto.TotalFiles == 0)
        {
            return dto.Note;
        }

        var parts = new List<string>
        {
            $"{dto.FilesystemRoot} → {dto.TargetPrefix}",
            $"{dto.TotalFiles} file(s), {HumanBytes(dto.TotalBytes)}",
        };

        // The number that answers "why can't I open anything". Reported
        // separately from the total on purpose — a single count reads the
        // source layer, which never fails.
        if (dto.NotIngested > 0)
        {
            parts.Add($"{dto.NotIngested} with no document yet");
        }

        // What the mount is MADE OF. "412 files" and "412 files: 400
        // image, 11 code, 1 markdown" answer different questions, and
        // only the second explains an empty-looking workspace.
        var kinds = new List<string>();
        foreach (var kind in KindOrder)
        {
            if (dto.KindCounts != null && dto.KindCounts.TryGetValue(kind, out var n) && n > 0)
                kinds.Add($"{n} {kind}");
        }
        if (kinds.Count > 0) parts.Add(string.Join(", ", kinds));

        if (truncated) parts.Add($"showing the first {MaxRowsShown} rows of this directory");
        if (!string.IsNullOrEmpty(dto.Note)) parts.Add(dto.Note);
        return string.Join("  ·  ", parts);
    }

    private void RebuildCrumbs(RenderDto dto)
    {
        _crumbs.Children.Clear();
        _crumbs.Children.Add(CrumbButton(dto.Root.Length > 0 ? dto.Root : "/", ""));
        var acc = "";
        foreach (var seg in dto.Crumbs ?? new List<string>())
        {
            acc = acc.Length == 0 ? seg : acc + "/" + seg;
            _crumbs.Children.Add(new TextBlock
            {
                Text = "/",
                Foreground = Brushes.DimGray,
                FontSize = 12,
                VerticalAlignment = VerticalAlignment.Center,
            });
            _crumbs.Children.Add(CrumbButton(seg, acc));
        }
    }

    private Button CrumbButton(string label, string target)
    {
        var b = new Button
        {
            Content = label,
            FontSize = 12,
            Padding = new Thickness(4, 0),
            Background = Brushes.Transparent,
        };
        b.Click += (_, _) => Navigate(target);
        return b;
    }

    private Control BuildRow(EntryVm vm)
    {
        var grid = new Grid
        {
            ColumnDefinitions = new ColumnDefinitions("*,Auto,Auto"),
            Margin = new Thickness(2, 3),
        };

        var name = new TextBlock
        {
            // A leading glyph rather than an icon set: it survives every
            // font fallback path this app has, and a missing icon asset
            // renders as a box that reads like an error.
            Text = (vm.IsDir ? "▸ " : "  ") + vm.Name,
            FontSize = 13,
            TextTrimming = TextTrimming.CharacterEllipsis,
            Foreground = vm.IsDir ? Brushes.SkyBlue : Brushes.White,
            VerticalAlignment = VerticalAlignment.Center,
        };
        var size = new TextBlock
        {
            Text = vm.IsDir ? $"{vm.ChildFiles} file(s)" : HumanBytes(vm.Size),
            FontSize = 11,
            Foreground = Brushes.DarkGray,
            Margin = new Thickness(8, 0),
            VerticalAlignment = VerticalAlignment.Center,
        };
        // The status column is the whole point of the panel: a file with
        // no document says so on its own row rather than looking
        // identical to one that has one.
        var status = new TextBlock
        {
            Text = vm.IsDir ? "" : ShortStatus(vm),
            FontSize = 11,
            Foreground = vm.IsDir ? Brushes.Transparent
                : (vm.Ingested ? Brushes.DarkSeaGreen : Brushes.Orange),
            MinWidth = 90,
            VerticalAlignment = VerticalAlignment.Center,
        };
        if (!vm.IsDir && !vm.Ingested) ToolTip.SetTip(status, vm.Status);

        Grid.SetColumn(name, 0);
        Grid.SetColumn(size, 1);
        Grid.SetColumn(status, 2);
        grid.Children.Add(name);
        grid.Children.Add(size);
        grid.Children.Add(status);
        return grid;
    }

    private static string ShortStatus(EntryVm vm)
    {
        if (vm.Ingested) return string.IsNullOrEmpty(vm.Language) ? vm.Kind : vm.Language;
        return "no document";
    }

    private static string HumanBytes(long n)
    {
        if (n < 1024) return $"{n} B";
        double v = n;
        foreach (var unit in new[] { "KB", "MB", "GB", "TB" })
        {
            v /= 1024;
            if (v < 1024) return v.ToString("0.#", CultureInfo.InvariantCulture) + " " + unit;
        }
        return v.ToString("0.#", CultureInfo.InvariantCulture) + " PB";
    }

    // --- Wake -----------------------------------------------------------

    private void OnWakeFromGo(long handle)
    {
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
        if (_disposed || _renderQueued) return;
        _renderQueued = true;
        Dispatcher.UIThread.Post(() =>
        {
            _renderQueued = false;
            if (_disposed) return;
            Refresh();
        });
    }

    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        PanelLog.Write("file-explorer", $"Dispose h={_handle}");
        if (_wakeTimer != null)
        {
            _wakeTimer.Stop();
            _wakeTimer.Tick -= OnWakeTimerTick;
            _wakeTimer = null;
        }
        if (_handle >= 0)
        {
            // FileExplorerClose joins the wake goroutine before it
            // returns, so after this point no Go code holds the function
            // pointer and the GC root is safe to release.
            Bridge.FileExplorerClose(_handle);
        }
        _wakeCallback = null;
        if (_wakeCallbackHandle.IsAllocated) _wakeCallbackHandle.Free();
    }

    private static bool IsOk(string envelope)
    {
        try
        {
            using var doc = JsonDocument.Parse(envelope);
            return doc.RootElement.TryGetProperty("ok", out var ok) && ok.GetBoolean();
        }
        catch { return false; }
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

    private static readonly string[] KindOrder = { "markdown", "text", "code", "image", "binary" };

    private static readonly JsonSerializerOptions JsonOpts = new()
    {
        PropertyNameCaseInsensitive = true,
    };

    // --- DTOs ------------------------------------------------------------
    //
    // AP49: an undeclared member is discarded by System.Text.Json in total
    // silence — the browser panel lost two provenance fields that way and
    // no test noticed, because none read a value that had quietly become
    // false. Every field the bridge sends is declared here and
    // FileExplorerPanelTests asserts they arrive.

    public sealed class EntryVm
    {
        [JsonPropertyName("name")] public string Name { get; set; } = "";
        [JsonPropertyName("relPath")] public string RelPath { get; set; } = "";
        [JsonPropertyName("isDir")] public bool IsDir { get; set; }
        [JsonPropertyName("childFiles")] public int ChildFiles { get; set; }
        [JsonPropertyName("childBytes")] public long ChildBytes { get; set; }
        [JsonPropertyName("size")] public long Size { get; set; }
        [JsonPropertyName("modifiedAtMillis")] public long ModifiedAtMillis { get; set; }
        [JsonPropertyName("kind")] public string Kind { get; set; } = "";
        [JsonPropertyName("language")] public string Language { get; set; } = "";
        [JsonPropertyName("mediaType")] public string MediaType { get; set; } = "";
        [JsonPropertyName("sourcePath")] public string SourcePath { get; set; } = "";
        [JsonPropertyName("targetPath")] public string TargetPath { get; set; } = "";
        [JsonPropertyName("entityType")] public string EntityType { get; set; } = "";
        [JsonPropertyName("ingested")] public bool Ingested { get; set; }
        [JsonPropertyName("status")] public string Status { get; set; } = "";
    }

    public sealed class RenderDto
    {
        [JsonPropertyName("ok")] public bool Ok { get; set; }
        [JsonPropertyName("error")] public string Error { get; set; } = "";
        [JsonPropertyName("root")] public string Root { get; set; } = "";
        [JsonPropertyName("filesystemRoot")] public string FilesystemRoot { get; set; } = "";
        [JsonPropertyName("sourcePrefix")] public string SourcePrefix { get; set; } = "";
        [JsonPropertyName("targetPrefix")] public string TargetPrefix { get; set; } = "";
        [JsonPropertyName("dir")] public string Dir { get; set; } = "";
        [JsonPropertyName("crumbs")] public List<string>? Crumbs { get; set; }
        [JsonPropertyName("entries")] public List<EntryVm>? Entries { get; set; }
        [JsonPropertyName("totalFiles")] public int TotalFiles { get; set; }
        [JsonPropertyName("totalBytes")] public long TotalBytes { get; set; }
        [JsonPropertyName("ingested")] public int Ingested { get; set; }
        [JsonPropertyName("notIngested")] public int NotIngested { get; set; }
        [JsonPropertyName("kindCounts")] public Dictionary<string, int>? KindCounts { get; set; }
        [JsonPropertyName("note")] public string Note { get; set; } = "";
    }

    public sealed class PreviewDto
    {
        [JsonPropertyName("ok")] public bool Ok { get; set; }
        [JsonPropertyName("error")] public string Error { get; set; } = "";
        [JsonPropertyName("relPath")] public string RelPath { get; set; } = "";
        [JsonPropertyName("kind")] public string Kind { get; set; } = "";
        [JsonPropertyName("language")] public string Language { get; set; } = "";
        [JsonPropertyName("text")] public string Text { get; set; } = "";
        [JsonPropertyName("textual")] public bool Textual { get; set; }
        [JsonPropertyName("truncated")] public bool Truncated { get; set; }
        [JsonPropertyName("size")] public long Size { get; set; }
    }
}
