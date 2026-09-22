using System;
using System.Collections.Generic;
using System.Collections.ObjectModel;
using System.Runtime.InteropServices;
using System.Text.Json;
using System.Text.Json.Serialization;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Layout;
using Avalonia.Media;
using Avalonia.Threading;

namespace EntityAvalonia.Panels;

// LocalFilesPanel renders — and now OPERATES — this peer's filesystem
// mounts: the directories wired into the tree by `mount`, read out of the
// localfiles handler's own config namespace.
//
// **Why this panel exists at all.** `ext/localfiles` is one of the most
// complete things in the kernel — filesystem->tree ingest, tree->filesystem
// writeback, an fsnotify watcher, a stat cache implementing Git's
// racy-clean rule, path containment with a leaf-symlink refusal — and
// until this panel landed no renderer in the repo could see any of it.
// `grep LocalFiles avalonia/ console/` returned nothing. That is D23's
// exact shape: complete layers, every test green, and the defect being the
// absence of an edge between them.
//
// **And the first version of this panel was half that edge**, which is the
// more interesting half of the lesson. It could LIST mounts and not make
// one, so the only way to mount a directory in the shipped GUI was to open
// a Shell panel and type the verb. `make reachability` passed throughout,
// correctly: it asks whether a model has *a* surface, not whether the
// surface can do what the model does. A read-only surface over a
// read-write model is a D23 violation the sweep cannot see, and the tell
// is a panel with no verb in it.
//
// **One call, no handle — still true, and now for a second reason.**
// Neighbouring panels hold an Open/RegisterWake/Render/Close handle
// because their models own a subscription. Mounts have no event source: a
// config is written when an operator mounts and then does not move. The
// mutating calls are synchronous for the same reason plus a sharper one —
// AP31: an async cgo export must copy every C-owned argument into Go
// memory before launching its goroutine, and a synchronous export is the
// shape where that hazard does not exist.
//
// **What this panel must not imply.** Two things it is careful to say
// rather than let the reader assume:
//
//   * Watcher liveness is NOT knowable from here. `WatcherConfigData` is
//     built as the response to a `watch` operation and never written to a
//     tree path, and the handler's live set is unexported. Every row
//     carries `watcherObservable: false` and prints "watcher: unknown".
//   * **Unmount does not stop the watcher.** The kernel exposes
//     StartWatching and no StopWatching, so the fsnotify watcher survives
//     an unmount. The bridge returns `watcherStillRunning` and this panel
//     renders it, because "unmounted" with nothing further tells an
//     operator something untrue about where their edits will go.
//
// Both are AP45: a surface must say what it does not know, because an
// absence reads as fine.
public sealed class LocalFilesPanel : UserControl, IPanelPreferredHeight
{
    // Chrome floor: summary + the four-field mount form + verb row + status + the mount list.
    // Declared because the 200px stack default clipped this panel the
    // moment a second one was open — see IPanelPreferredHeight.
    public double PreferredSlotMinHeight => 440;
    // P4 (bounded list). A peer can hold many mounts in principle; in
    // practice it holds a handful. The cap exists so a pathological
    // config namespace cannot make the panel the slow part.
    private const int MaxMountsShown = 200;

    private readonly long _peerHandle;
    private long _wakeRegistration = -1;
    private SharingWake? _wakeCallback;
    private GCHandle _wakeHandle;
    private bool _closed;

    private delegate void SharingWake(long handle);
    private readonly TextBlock _summary;
    private readonly ObservableCollection<MountRow> _mounts = new();

    private readonly TextBox _dirBox;
    private readonly TextBox _prefixBox;
    private readonly TextBox _includeBox;
    private readonly TextBox _excludeBox;
    private readonly CheckBox _readOnlyBox;
    private readonly Button _mountBtn;
    private readonly Button _forceBtn;
    private readonly SelectableTextBlock _opStatus;

    // Test/driver surface. The form fields are the operation's inputs, so
    // a headless test sets them and calls PerformMount rather than
    // synthesizing a click — the click path itself is covered by the
    // real-input harness, and AP32 is the standing warning against
    // settling on a derived UI property as a completion signal.
    public string DirectoryText { get => _dirBox.Text ?? ""; set => _dirBox.Text = value; }
    public string PrefixText { get => _prefixBox.Text ?? ""; set => _prefixBox.Text = value; }
    public string IncludeText { get => _includeBox.Text ?? ""; set => _includeBox.Text = value; }
    public string ExcludeText { get => _excludeBox.Text ?? ""; set => _excludeBox.Text = value; }
    public bool ReadOnlyChecked { get => _readOnlyBox.IsChecked == true; set => _readOnlyBox.IsChecked = value; }
    public string OperationStatus => _opStatus.Text ?? "";
    public int MountCount => _mounts.Count;

    public LocalFilesPanel(long peerHandle)
    {
        _peerHandle = peerHandle;

        _summary = new TextBlock
        {
            Margin = new Thickness(12, 10, 12, 6),
            FontSize = 13,
            TextWrapping = TextWrapping.Wrap,
            Foreground = Brushes.Gainsboro,
        };

        _dirBox = FormBox("/absolute/path/to/a/directory");
        _prefixBox = FormBox("archives/notes/");
        _includeBox = FormBox("*.md, *.txt   (empty = everything not excluded)");
        // Prefilled from the shell's own default list rather than a
        // literal retyped here. A filter applied silently is a filter the
        // operator is surprised by later, and two copies of the list is
        // how the panel and the verb come to disagree about what a
        // default mount ingests.
        _excludeBox = FormBox("");
        _excludeBox.Text = string.Join(", ", LoadDefaultExclude());

        // Syncthing calls this folder type "Send Only" and it is the
        // most-used non-default one. The kernel's RootConfigData has
        // carried the field from the start; until now no surface in this
        // repo could set it, so the mode existed in the substrate and was
        // unreachable from the product.
        _readOnlyBox = new CheckBox
        {
            Content = "Read-only (disk → tree only)",
            FontSize = 12,
            IsChecked = false,
        };
        ToolTip.SetTip(_readOnlyBox,
            "Changes on disk flow into the tree. Nothing the tree receives is written back "
            + "out to disk. Syncthing calls this \"Send Only\".");

        _mountBtn = new Button { Content = "Mount", FontSize = 12 };
        _mountBtn.Click += (_, _) => PerformMount(force: false);

        _forceBtn = new Button
        {
            Content = "Mount anyway",
            FontSize = 12,
            IsVisible = false,
            Background = new SolidColorBrush(Color.FromRgb(0x7a, 0x3b, 0x1e)),
        };
        ToolTip.SetTip(_forceBtn, "Proceed past the target-prefix type conflict shown above");
        _forceBtn.Click += (_, _) => PerformMount(force: true);

        _opStatus = new SelectableTextBlock
        {
            Text = "",
            FontSize = 12,
            Margin = new Thickness(12, 2, 12, 6),
            TextWrapping = TextWrapping.Wrap,
            Foreground = Brushes.Gainsboro,
        };

        var list = new ListBox
        {
            ItemsSource = _mounts,
            Background = Brushes.Transparent,
            // AP46: never `new FuncDataTemplate<T>` — Avalonia calls the
            // builder with null during container teardown.
            ItemTemplate = Rows.Of<MountRow>((row, _) => BuildRow(row)),
        };

        // Built once and held in a local. A control constructed twice and
        // added twice violates Avalonia's visual-parent invariant and
        // throws inside DockPanel.Children.Add — the same trap MainWindow
        // documents about its diag bar.
        var form = BuildMountForm();

        var root = new DockPanel();
        DockPanel.SetDock(_summary, Dock.Top);
        DockPanel.SetDock(form, Dock.Top);
        DockPanel.SetDock(_opStatus, Dock.Top);
        root.Children.Add(_summary);
        root.Children.Add(form);
        root.Children.Add(_opStatus);
        root.Children.Add(list);
        Content = root;

        Refresh();
        OpenWake();
    }

    // --- Reactivity --------------------------------------------------------

    private void OpenWake()
    {
        _wakeCallback = OnSharingWake;
        _wakeHandle = GCHandle.Alloc(_wakeCallback);
        var ptr = Marshal.GetFunctionPointerForDelegate(_wakeCallback);
        var reply = Bridge.TakeString(Bridge.SharingRegisterWake(_peerHandle, ptr));
        try
        {
            using var doc = JsonDocument.Parse(reply);
            if (doc.RootElement.TryGetProperty("registration", out var r)
                && r.TryGetInt64(out var id))
            {
                _wakeRegistration = id;
            }
        }
        catch (JsonException) { }
    }

    // Runs on a Go-owned goroutine — must not touch a control from here.
    private void OnSharingWake(long handle)
    {
        if (_closed) return;
        Dispatcher.UIThread.Post(() =>
        {
            if (_closed) return;
            Refresh();
        });
    }

    protected override void OnDetachedFromVisualTree(VisualTreeAttachmentEventArgs e)
    {
        _closed = true;
        if (_wakeRegistration >= 0)
        {
            Bridge.TakeString(Bridge.SharingUnregisterWake(_peerHandle, _wakeRegistration));
            _wakeRegistration = -1;
        }
        if (_wakeHandle.IsAllocated) _wakeHandle.Free();
        _wakeCallback = null;
        base.OnDetachedFromVisualTree(e);
    }

    // BuildMountForm lays out the four inputs plus the verb row.
    //
    // There used to be a Refresh button threaded in here. It is gone: the
    // panel holds a SharingRegisterWake subscription covering the mount
    // namespaces and the file layers, so the list and its counts move by
    // themselves. A Refresh control on tree data is a bug report about a
    // missing subscription (AP73), and this panel showed a stale entity
    // count beside a Sharing Status panel showing a different one —
    // which an operator reasonably read as the product being broken.
    private Control BuildMountForm()
    {
        var grid = new Grid
        {
            Margin = new Thickness(12, 0, 12, 4),
            ColumnDefinitions = new ColumnDefinitions("Auto,*"),
            RowDefinitions = new RowDefinitions("Auto,Auto,Auto,Auto,Auto"),
        };

        void Row(int r, string label, Control field)
        {
            var lbl = new TextBlock
            {
                Text = label,
                FontSize = 12,
                Foreground = Brushes.DarkGray,
                VerticalAlignment = VerticalAlignment.Center,
                Margin = new Thickness(0, 3, 8, 3),
            };
            Grid.SetRow(lbl, r); Grid.SetColumn(lbl, 0);
            Grid.SetRow(field, r); Grid.SetColumn(field, 1);
            grid.Children.Add(lbl);
            grid.Children.Add(field);
        }

        Row(0, "Directory", _dirBox);
        Row(1, "Tree prefix", _prefixBox);
        Row(2, "Include", _includeBox);
        Row(3, "Exclude", _excludeBox);

        var buttons = new StackPanel
        {
            Orientation = Orientation.Horizontal,
            Spacing = 6,
            Margin = new Thickness(0, 6, 0, 2),
        };
        buttons.Children.Add(_readOnlyBox);
        buttons.Children.Add(_mountBtn);
        buttons.Children.Add(_forceBtn);
        Grid.SetRow(buttons, 4); Grid.SetColumn(buttons, 1);
        grid.Children.Add(buttons);

        return grid;
    }

    private static TextBox FormBox(string watermark) => new()
    {
        Watermark = watermark,
        FontSize = 12,
        Margin = new Thickness(0, 2),
    };

    // --- Operations ---------------------------------------------------

    // PerformMount reads the form and mounts. Public so a headless test
    // drives the operation directly; the click path is the same call.
    //
    // Exclude is sent with excludeSet = 1 unconditionally, because the
    // form ALWAYS shows the operator a value — prefilled with the
    // defaults. Whatever is in that box when they press Mount is what
    // they asked for, including an empty box, which means "no exclusions"
    // and not "apply the defaults behind my back".
    public MountResult PerformMount(bool force)
    {
        var reply = Bridge.TakeString(Bridge.LocalFilesMount(
            _peerHandle,
            DirectoryText.Trim(),
            PrefixText.Trim(),
            IncludeText,
            ExcludeText,
            excludeSet: 1,
            force: force ? 1 : 0,
            readOnly: ReadOnlyChecked ? 1 : 0));

        var res = Decode<MountResult>(reply, out var decodeErr);
        if (res == null)
        {
            SetStatus($"mount reply did not decode: {decodeErr}", Brushes.IndianRed);
            _forceBtn.IsVisible = false;
            return new MountResult { Error = decodeErr };
        }

        if (res.Ok)
        {
            _forceBtn.IsVisible = false;
            SetStatus(
                $"mounted {res.FilesystemRoot} → {res.TargetPrefix} (root={res.RootName})\n"
                // Stated on every mount, not only the read-only ones: a
                // surface that mentions the write policy only when it is
                // unusual leaves the usual case to be inferred from silence.
                + (res.ReadOnly
                    ? "read-only — disk → tree only; nothing is written back to disk\n"
                    : "read-write — disk → tree, and tree → disk\n")
                + $"source {res.SourcePrefix} · subscription {res.SubscriptionId} · cap {res.CapabilityPath}",
                Brushes.PaleGreen);
            Refresh();
            return res;
        }

        if (res.Conflict != null)
        {
            // Show what is in the way, not just that something is. The
            // typed conflict exists precisely so this can be a list the
            // operator reads rather than a sentence they have to parse.
            var lines = new List<string>
            {
                $"{res.Conflict.TargetTotal} existing binding(s) at {res.Conflict.TargetPrefix} "
                + $"have a type this mount does not own:",
            };
            foreach (var t in res.Conflict.ForeignOrder ?? new List<string>())
            {
                var n = res.Conflict.Foreign != null && res.Conflict.Foreign.TryGetValue(t, out var c) ? c : 0;
                lines.Add($"    {n}  {t}");
            }
            lines.Add($"    ({res.Conflict.TargetExpected} matching "
                + $"{string.Join(" / ", res.Conflict.ExpectedTypes ?? new List<string>())} already present)");
            if (res.Conflict.SourceTotal > 0)
                lines.Add($"    also {res.Conflict.SourceTotal} binding(s) under {res.Conflict.SourcePrefix} from a prior mount");
            SetStatus(string.Join("\n", lines), Brushes.Orange);
            _forceBtn.IsVisible = true;
            return res;
        }

        _forceBtn.IsVisible = false;
        SetStatus($"mount failed: {res.Error}", Brushes.IndianRed);
        return res;
    }

    public UnmountResult PerformUnmount(string rootName)
    {
        var reply = Bridge.TakeString(Bridge.LocalFilesUnmount(_peerHandle, rootName));
        var res = Decode<UnmountResult>(reply, out var decodeErr);
        if (res == null)
        {
            SetStatus($"unmount reply did not decode: {decodeErr}", Brushes.IndianRed);
            return new UnmountResult { Error = decodeErr };
        }
        if (!res.Ok)
        {
            SetStatus($"unmount failed: {res.Error}", Brushes.IndianRed);
            return res;
        }

        // The watcher note is not decoration. It is the difference
        // between "this directory is disconnected" and "this directory
        // still has a live fsnotify watcher on it" — which is what is
        // actually true, and what the operator needs in order to
        // understand why edits keep showing up.
        var msg = $"unmounted {res.RootName} — ingest mapping and subscription cleared";
        if (!string.IsNullOrEmpty(res.SubscriptionCloseError))
            msg += $"\nsubscription close reported: {res.SubscriptionCloseError}";
        if (res.WatcherStillRunning)
            msg += "\nthe filesystem watcher is STILL RUNNING for this root — the kernel offers no stop, "
                 + "so it ends with the process. Bounded, not leaking: remounting the same root replaces it.";
        SetStatus(msg, res.WatcherStillRunning ? Brushes.Orange : Brushes.PaleGreen);
        Refresh();
        return res;
    }

    public SweepResult PerformSweep(string rootName, bool addMissing)
    {
        var reply = Bridge.TakeString(Bridge.LocalFilesSweep(_peerHandle, rootName, addMissing ? 1 : 0));
        var res = Decode<SweepResult>(reply, out var decodeErr);
        if (res == null)
        {
            SetStatus($"sweep reply did not decode: {decodeErr}", Brushes.IndianRed);
            return new SweepResult { Error = decodeErr };
        }
        if (!res.Ok)
        {
            SetStatus($"sweep failed: {res.Error}", Brushes.IndianRed);
            return res;
        }

        // A sweep DELETES bindings. Naming which ones is not verbosity —
        // a count alone asks the operator to trust a destructive
        // operation they cannot inspect.
        var lines = new List<string>
        {
            $"sweep {res.RootName}: {res.FilesystemFiles} file(s) on disk, {res.SourcePresent} path(s) in tree",
            $"removed {res.SourceRemoved?.Count ?? 0} source binding(s)"
                + (res.TargetRemoved is { Count: > 0 } ? $", {res.TargetRemoved.Count} target binding(s)" : ""),
        };
        foreach (var p in res.SourceRemoved ?? new List<string>()) lines.Add($"    - {p}");
        foreach (var p in res.TargetRemoved ?? new List<string>()) lines.Add($"    - {p}");
        if (addMissing)
        {
            lines.Add($"ingested {res.Added} file(s) missing from the tree");
            foreach (var e in res.AddErrors ?? new List<string>()) lines.Add($"    ! {e}");
        }
        SetStatus(string.Join("\n", lines), Brushes.Gainsboro);
        Refresh();
        return res;
    }

    // Refresh pulls a fresh snapshot. Public so a headless test can drive
    // the panel without reaching for a wake it does not have (AP32 — a
    // derived UI property is not a completion signal).
    public void Refresh()
    {
        var reply = Bridge.TakeString(Bridge.LocalFilesRender(_peerHandle));
        RenderEnvelope view;
        try
        {
            view = JsonSerializer.Deserialize<RenderEnvelope>(reply, JsonOpts) ?? new RenderEnvelope();
        }
        catch (JsonException ex)
        {
            _mounts.Clear();
            _summary.Text = $"local-files render did not decode: {ex.Message}";
            _summary.Foreground = Brushes.IndianRed;
            return;
        }

        _mounts.Clear();
        var shown = 0;
        foreach (var m in view.Mounts ?? new List<MountRow>())
        {
            if (shown++ >= MaxMountsShown) break;
            _mounts.Add(m);
        }

        if (_mounts.Count == 0)
        {
            // An empty list is a claim. Render the model's reason rather
            // than a bare "no mounts", which reads identically to a
            // failed read.
            _summary.Text = string.IsNullOrEmpty(view.Note)
                ? "no filesystem mounts on this peer — use the form below to bridge a directory into the tree"
                : view.Note;
            _summary.Foreground = Brushes.Gainsboro;
            return;
        }

        var truncated = (view.Mounts?.Count ?? 0) > _mounts.Count
            ? $" (showing {_mounts.Count} of {view.Mounts!.Count})"
            : "";
        _summary.Text = $"{_mounts.Count} mount{(_mounts.Count == 1 ? "" : "s")}{truncated} "
                      + "— watcher state is not reported by the tree, so \"running\" is unknown here";
        _summary.Foreground = Brushes.Gainsboro;
    }

    private void SetStatus(string text, IBrush brush)
    {
        _opStatus.Text = text;
        _opStatus.Foreground = brush;
    }

    private static T? Decode<T>(string json, out string error) where T : class
    {
        error = "";
        try
        {
            var v = JsonSerializer.Deserialize<T>(json, JsonOpts);
            if (v == null) error = "empty reply";
            return v;
        }
        catch (JsonException ex)
        {
            error = ex.Message;
            return null;
        }
    }

    private static List<string> LoadDefaultExclude()
    {
        var reply = Bridge.TakeString(Bridge.LocalFilesDefaultExclude());
        try
        {
            var dto = JsonSerializer.Deserialize<DefaultExcludeEnvelope>(reply, JsonOpts);
            return dto?.Patterns ?? new List<string>();
        }
        catch (JsonException)
        {
            // A default list we could not read is a prefill we do not
            // make up. An empty box means "no exclusions" and the
            // operator can see that it is empty — inventing a plausible
            // list here would be a filter nobody chose.
            return new List<string>();
        }
    }

    private Control BuildRow(MountRow row)
    {
        var stack = new StackPanel { Margin = new Thickness(4, 6, 4, 6), Spacing = 2 };

        var title = new TextBlock
        {
            Text = row.Root,
            FontWeight = FontWeight.SemiBold,
            FontSize = 14,
            Foreground = string.IsNullOrEmpty(row.Err) ? Brushes.White : Brushes.IndianRed,
        };
        stack.Children.Add(title);

        if (!string.IsNullOrEmpty(row.Err))
        {
            // A mount whose config will not decode is exactly what an
            // operator needs shown — rendering it as healthy, or dropping
            // it, reports a broken mount as no mount at all.
            stack.Children.Add(Line($"config unreadable: {row.Err}", Brushes.IndianRed));
            stack.Children.Add(Line($"at {row.ConfigPath}", Brushes.Gray));
            return stack;
        }

        stack.Children.Add(Line($"{row.FilesystemRoot}  →  {row.Prefix}", Brushes.Gainsboro));

        var flags = new List<string> { row.ReadOnly ? "read-only" : "read-write" };
        if (row.PublishDescriptors) flags.Add("publishes descriptors");
        flags.Add($"{row.FileCount} entit{(row.FileCount == 1 ? "y" : "ies")} in tree");
        // Stated on every row rather than once in the header, because a
        // row is what gets screenshotted and quoted.
        //
        // THREE states, not two. `watcherObservable` false means the tree
        // carries no watch record for this root — which is where a mount whose
        // watcher never started lands, and is a different thing from one that
        // was stopped. Collapsing them sends an operator to the wrong place.
        flags.Add(row.WatcherObservable
            ? $"watcher: {row.WatcherStatus}"
            : "watcher: no record");
        stack.Children.Add(Line(string.Join("  ·  ", flags),
            row.WatcherStatus == "error" ? Brushes.Goldenrod : Brushes.DarkGray));

        // The only line that says WHY a mount stopped producing documents.
        // Rendered on its own row because it is the thing to act on, and a
        // message folded into the flag strip reads as a label.
        if (row.WatcherStatus == "error" && !string.IsNullOrWhiteSpace(row.WatcherError))
            stack.Children.Add(Line($"watcher error: {row.WatcherError}", Brushes.IndianRed));

        if (row.Include is { Count: > 0 })
            stack.Children.Add(Line($"include: {string.Join(", ", row.Include)}", Brushes.DarkGray));
        if (row.Exclude is { Count: > 0 })
            stack.Children.Add(Line($"exclude: {string.Join(", ", row.Exclude)}", Brushes.DarkGray));

        var rootName = row.Root;
        var verbs = new StackPanel
        {
            Orientation = Orientation.Horizontal,
            Spacing = 6,
            Margin = new Thickness(0, 4, 0, 0),
        };
        verbs.Children.Add(RowButton("Sweep", "Remove tree entries whose file is gone from disk",
            () => PerformSweep(rootName, addMissing: false)));
        verbs.Children.Add(RowButton("Sweep + ingest", "Also ingest files on disk that are missing from the tree",
            () => PerformSweep(rootName, addMissing: true)));
        verbs.Children.Add(RowButton("Unmount", "Drop the ingest mapping and cancel the subscription. Does NOT stop the filesystem watcher.",
            () => PerformUnmount(rootName)));
        stack.Children.Add(verbs);

        return stack;
    }

    private static Button RowButton(string label, string tip, Action onClick)
    {
        var b = new Button { Content = label, FontSize = 11, Padding = new Thickness(8, 2) };
        ToolTip.SetTip(b, tip);
        b.Click += (_, _) => onClick();
        return b;
    }

    private static TextBlock Line(string text, IBrush brush) => new()
    {
        Text = text,
        FontSize = 12,
        Foreground = brush,
        TextWrapping = TextWrapping.Wrap,
    };

    private static readonly JsonSerializerOptions JsonOpts = new()
    {
        PropertyNameCaseInsensitive = true,
    };

    // AP49: an undeclared DTO member is discarded by System.Text.Json in
    // total silence. Every field the bridge sends is declared here, and
    // LocalFilesPanelTests asserts they arrive.
    public sealed class RenderEnvelope
    {
        [JsonPropertyName("ok")] public bool Ok { get; set; }
        [JsonPropertyName("mounts")] public List<MountRow>? Mounts { get; set; }
        [JsonPropertyName("note")] public string? Note { get; set; }
    }

    public sealed class MountRow
    {
        [JsonPropertyName("root")] public string Root { get; set; } = "";
        [JsonPropertyName("filesystemRoot")] public string FilesystemRoot { get; set; } = "";
        [JsonPropertyName("prefix")] public string Prefix { get; set; } = "";
        [JsonPropertyName("readOnly")] public bool ReadOnly { get; set; }
        [JsonPropertyName("include")] public List<string>? Include { get; set; }
        [JsonPropertyName("exclude")] public List<string>? Exclude { get; set; }
        [JsonPropertyName("publishDescriptors")] public bool PublishDescriptors { get; set; }
        [JsonPropertyName("configPath")] public string ConfigPath { get; set; } = "";
        [JsonPropertyName("fileCount")] public int FileCount { get; set; }
        [JsonPropertyName("watcherObservable")] public bool WatcherObservable { get; set; }
        // Declared, not inferred: an undeclared field is discarded in silence
        // by System.Text.Json (AP49), and the failure mode here is a watcher
        // reporting `error` that renders as though nothing were wrong.
        [JsonPropertyName("watcherStatus")] public string WatcherStatus { get; set; } = "";
        [JsonPropertyName("watcherError")] public string WatcherError { get; set; } = "";
        [JsonPropertyName("err")] public string Err { get; set; } = "";
    }

    public sealed class MountResult
    {
        [JsonPropertyName("ok")] public bool Ok { get; set; }
        [JsonPropertyName("error")] public string Error { get; set; } = "";
        [JsonPropertyName("rootName")] public string RootName { get; set; } = "";
        [JsonPropertyName("filesystemRoot")] public string FilesystemRoot { get; set; } = "";
        [JsonPropertyName("sourcePrefix")] public string SourcePrefix { get; set; } = "";
        [JsonPropertyName("targetPrefix")] public string TargetPrefix { get; set; } = "";
        [JsonPropertyName("include")] public List<string>? Include { get; set; }
        [JsonPropertyName("exclude")] public List<string>? Exclude { get; set; }
        [JsonPropertyName("capabilityPath")] public string CapabilityPath { get; set; } = "";
        [JsonPropertyName("handlerPattern")] public string HandlerPattern { get; set; } = "";
        [JsonPropertyName("subscriptionId")] public string SubscriptionId { get; set; } = "";
        [JsonPropertyName("readOnly")] public bool ReadOnly { get; set; }
        [JsonPropertyName("conflict")] public MountConflictDto? Conflict { get; set; }
    }

    public sealed class MountConflictDto
    {
        [JsonPropertyName("targetPrefix")] public string TargetPrefix { get; set; } = "";
        [JsonPropertyName("sourcePrefix")] public string SourcePrefix { get; set; } = "";
        // Plural since the ingest registry: a mount owns every doc/* type
        // it can write, and naming one of five would tell an operator
        // their code files were foreign to a mount that writes code files.
        [JsonPropertyName("expectedTypes")] public List<string>? ExpectedTypes { get; set; }
        [JsonPropertyName("targetTotal")] public int TargetTotal { get; set; }
        [JsonPropertyName("targetExpected")] public int TargetExpected { get; set; }
        [JsonPropertyName("sourceTotal")] public int SourceTotal { get; set; }
        [JsonPropertyName("foreignOrder")] public List<string>? ForeignOrder { get; set; }
        [JsonPropertyName("foreign")] public Dictionary<string, int>? Foreign { get; set; }
    }

    public sealed class UnmountResult
    {
        [JsonPropertyName("ok")] public bool Ok { get; set; }
        [JsonPropertyName("error")] public string Error { get; set; } = "";
        [JsonPropertyName("rootName")] public string RootName { get; set; } = "";
        [JsonPropertyName("subscriptionCloseError")] public string SubscriptionCloseError { get; set; } = "";
        [JsonPropertyName("watcherStillRunning")] public bool WatcherStillRunning { get; set; }
    }

    public sealed class SweepResult
    {
        [JsonPropertyName("ok")] public bool Ok { get; set; }
        [JsonPropertyName("error")] public string Error { get; set; } = "";
        [JsonPropertyName("rootName")] public string RootName { get; set; } = "";
        [JsonPropertyName("filesystemFiles")] public int FilesystemFiles { get; set; }
        [JsonPropertyName("sourcePresent")] public int SourcePresent { get; set; }
        [JsonPropertyName("sourceRemoved")] public List<string>? SourceRemoved { get; set; }
        [JsonPropertyName("targetRemoved")] public List<string>? TargetRemoved { get; set; }
        [JsonPropertyName("added")] public int Added { get; set; }
        [JsonPropertyName("addErrors")] public List<string>? AddErrors { get; set; }
    }

    private sealed class DefaultExcludeEnvelope
    {
        [JsonPropertyName("ok")] public bool Ok { get; set; }
        [JsonPropertyName("patterns")] public List<string>? Patterns { get; set; }
    }
}
