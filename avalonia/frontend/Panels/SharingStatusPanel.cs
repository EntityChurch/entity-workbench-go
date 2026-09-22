using System;
using System.Collections.Generic;
using System.Collections.ObjectModel;
using System.Runtime.InteropServices;
using System.Text.Json;
using System.Text.Json.Serialization;
using System.Threading.Tasks;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Controls.Primitives;
using Avalonia.Layout;
using Avalonia.Media;
using Avalonia.Threading;

namespace EntityAvalonia.Panels;

// SharingStatusPanel answers ONE question: **is what I declared actually
// working, and if not, what is stopping it?**
//
// # Why it is not called "Sync"
//
// Two panels in this category already exist — Peer Connections ("who am I
// connected to") and Shared Folders ("share and receive") — and a third
// called "Sync" would repeat the Browser / Local Site / Origin Inspector
// mistake an operator called incomprehensible. Three panels reading the
// same bytes must each say in their NAME which question they answer. This
// one is the `status` verb's question, so the name is the question.
//
// # What it is, structurally
//
// Not a new feature: the missing half of the declared-state layer. There
// was no bridge export for devices, folders or the reconciler at all, so
// everything S2 built was reachable from a shell verb and from no pixel.
// Meanwhile the GUI *runs* a reconcile pass at every startup and reports
// its problems to stderr — a file in `avalonia/run-logs/` nobody opens —
// so an operator whose accepted folder had lost its mount was told so
// correctly somewhere they would never look, while every panel on screen
// looked fine.
//
// # Read and re-check are DIFFERENT buttons, and that is the design
//
// A reconcile pass DIALS every declared peer. So this panel refreshes with
// `StatusRender` (reads records, observes the substrate, writes nothing,
// dials nobody) and runs `StatusReconcile` only when a person asks: on
// open, on "Re-check now", and after a mutation it performed. Wiring the
// pass to a wake or a timer would make a status panel into a dialer that
// an operator leaves running overnight.
//
// The panel says which of the two it is showing. "Read" and "verified by a
// pass" are different claims, and only one of them means the relationship
// was actually established.
//
// # Inbound authority is NOT rendered as a health state
//
// What we grant a peer is our own policy row and is exactly knowable, so
// it is printed. What THEY grant US is in their capability table, which
// this peer cannot read; it is only ever observed, through deliveries
// arriving. So the inbound direction renders as an observation — how many
// files have actually landed, and whether a subscription exists — and
// never as a green dot. A dot there asserts something about another
// machine that we have no way to check, and it is wrong in exactly the
// case that matters: they revoked us and we have not tried since.
public sealed class SharingStatusPanel : UserControl, IPanelPreferredHeight
{
    // Chrome floor (AP64): identity block + caption + three sections, each
    // with a bounded list. Every panel declares one; a panel that does not
    // claims the 200px default and clips every panel in the stack.
    public double PreferredSlotMinHeight => 620;

    private readonly long _peerHandle;
    private long _wakeRegistration = -1;
    private SharingWake? _wakeCallback;
    private delegate void SharingWake(long handle);
    private GCHandle _wakeHandle;
    private bool _closed;

    private readonly SelectableTextBlock _identityLine;
    private readonly TextBlock _caption;
    private readonly Button _recheckBtn;
    private readonly SelectableTextBlock _status;

    private readonly ObservableCollection<DeviceVm> _devices = new();
    private readonly ObservableCollection<FolderVm> _folders = new();
    private readonly ObservableCollection<string> _problems = new();
    private readonly ObservableCollection<string> _actions = new();

    private readonly TextBlock _devicesEmpty;
    private readonly TextBlock _foldersEmpty;
    private readonly TextBlock _problemsEmpty;
    private readonly Control _actionsSection;

    private bool _busy;

    // AutoReconcileOnOpen is on in the app and off under test.
    //
    // The panel runs a real pass when it opens, which is right: the
    // question it answers is not answerable from records alone, and
    // opening it IS the operator asking. But a pass DIALS, and no suite in
    // this repo may reach anything — and worse for a test, the pass
    // completes asynchronously and then REPLACES the row collections,
    // silently wiping anything a test seeded a moment earlier. That is a
    // race a test cannot see: it would pass or fail on dispatcher timing
    // and be blamed on the assertion.
    //
    // Same shape and same reason as SharePanel.AutoDialOnConnect.
    internal static bool AutoReconcileOnOpen = true;

    // --- Test/driver surface ---------------------------------------------
    //
    // Counts and captions rather than control-walking, for the assertions
    // that are about data. The assertions that are about REACHABILITY walk
    // the visual tree in the test itself, because that is the only thing
    // that can tell a rendered button from a clipped one.
    public int DeviceCount => _devices.Count;
    public int FolderCount => _folders.Count;
    public int ProblemCount => _problems.Count;
    public string StatusText => _status.Text ?? "";
    public string CaptionText => _caption.Text ?? "";
    public string IdentityText => _identityLine.Text ?? "";

    // SeedForTests renders rows without a second live peer.
    //
    // It fills the SAME view-model collections the real ItemTemplates read,
    // so a row is built by BuildDeviceRow / BuildFolderRow exactly as it is
    // in the app. That is the part worth gating: a test that asserts on a
    // view model passes against a row whose buttons are clipped, or absent,
    // or wired to nothing.
    internal void SeedDeviceForTests(string peerId, string label, bool maintained,
        bool connected, bool paused, string outboundGrant)
    {
        _devices.Add(new DeviceVm(peerId, label, "", maintained, connected, paused,
            outboundGrant, ""));
        _devicesEmpty.IsVisible = false;
    }

    // root and localRoot are parameters and not blanks, because the
    // difference between them is a thing this panel has to render: the
    // subscription is keyed on the SENDER's root and the mount is named
    // after the directory the operator picked. A seed that left both empty
    // could not exercise that clause at all, and the test named after it
    // would pass on the strength of a row belonging to some other test.
    internal void SeedFolderForTests(string id, string label, bool local, string origin,
        string root, string localRoot,
        bool mounted, bool syncing, bool accepted, string path,
        int filesPresent, int filesIngested, bool filesObservable)
    {
        _folders.Add(new FolderVm(id, label, local, origin, root, localRoot, path, mounted,
            syncing, accepted, filesPresent, filesIngested, filesObservable, "",
            new List<FolderPeerVm>()));
        _foldersEmpty.IsVisible = false;
    }

    public SharingStatusPanel(long peerHandle)
    {
        _peerHandle = peerHandle;

        _identityLine = new SelectableTextBlock
        {
            FontFamily = new FontFamily("monospace"),
            FontSize = 11,
            Opacity = 0.75,
            TextWrapping = TextWrapping.Wrap,
        };
        _caption = new TextBlock
        {
            FontSize = 11,
            Opacity = 0.6,
            TextWrapping = TextWrapping.Wrap,
            Margin = new Thickness(0, 2, 0, 0),
        };

        _recheckBtn = new Button { Content = "Re-check now", FontSize = 12 };
        ToolTip.SetTip(_recheckBtn,
            "Run one pass of the control loop: re-establish connections to declared peers, "
            + "write any authorization the declarations require, and subscribe to accepted "
            + "folders that have a mount. Reaches the network.");
        _recheckBtn.Click += (_, _) => _ = ReconcileAsync();

        _status = new SelectableTextBlock
        {
            Text = "",
            FontSize = 12,
            TextWrapping = TextWrapping.Wrap,
            Foreground = Brushes.Gainsboro,
            Margin = new Thickness(12, 6, 12, 10),
        };

        _devicesEmpty = Hint("No peers declared yet. Sharing a folder with someone, or "
            + "accepting one from them, declares them here — from then on this peer "
            + "re-establishes the relationship by itself at every start.");
        _foldersEmpty = Hint("No folders declared yet. Use Shared Folders to offer one, "
            + "or to accept one that has been offered to you.");
        _problemsEmpty = Hint("Nothing is wrong that this peer can see.");

        var body = new StackPanel { Orientation = Orientation.Vertical };
        body.Children.Add(BuildIdentitySection());
        body.Children.Add(BuildDevicesSection());
        body.Children.Add(BuildFoldersSection());
        body.Children.Add(BuildProblemsSection());
        _actionsSection = BuildActionsSection();
        body.Children.Add(_actionsSection);

        // Status docked to the bottom, body in a fill ScrollViewer: the
        // operator's last action is the thing they are reading, and a
        // status line that scrolls away with the body is one they have to
        // go looking for. It also makes the panel internally scrollable
        // regardless of the declared floor above, so an under-estimate
        // degrades to scrolling rather than to an unreachable button.
        var root = new DockPanel { LastChildFill = true };
        DockPanel.SetDock(_status, Dock.Bottom);
        root.Children.Add(_status);
        root.Children.Add(new ScrollViewer
        {
            Content = body,
            HorizontalScrollBarVisibility = ScrollBarVisibility.Disabled,
            VerticalScrollBarVisibility = ScrollBarVisibility.Auto,
        });
        Content = root;

        // Read on construction so the panel has content immediately, then
        // run one real pass — because a reading is only a reading, and the
        // question this panel answers ("is it working?") is not answerable
        // without one. On open is exactly where a pass belongs: the
        // operator asked, by opening it.
        Refresh();
        OpenWake();
        if (AutoReconcileOnOpen)
        {
            _ = ReconcileAsync();
        }
    }

    // --- Reactivity --------------------------------------------------------
    //
    // This panel shipped with NO tree subscription and a Refresh button,
    // which is AP73's exact tell — and the mechanism it needed already
    // existed, because SyncPanel was using it. So an operator watching a
    // share land saw the row update nowhere and the file counts freeze,
    // while a second panel they opened later showed different numbers.
    //
    // Only the READ is wired. `StatusReconcile` dials every declared
    // device; hanging that off a wake would make a dialer out of an open
    // panel and it would wake itself forever. Re-check now stays a button
    // on purpose — it reaches the network, and the operator asks for it.
    private void OpenWake()
    {
        _wakeCallback = OnSharingWake;
        _wakeHandle = GCHandle.Alloc(_wakeCallback);
        var ptr = Marshal.GetFunctionPointerForDelegate(_wakeCallback);
        var reply = Bridge.TakeString(Bridge.SharingRegisterWake(_peerHandle, ptr));
        _wakeRegistration = ParseWakeRegistration(reply);
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

    private static long ParseWakeRegistration(string json)
    {
        try
        {
            using var doc = JsonDocument.Parse(json);
            if (doc.RootElement.TryGetProperty("registration", out var r)
                && r.TryGetInt64(out var id))
            {
                return id;
            }
        }
        catch (JsonException) { }
        return -1;
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

    // --- Sections ---------------------------------------------------------

    private Control BuildIdentitySection()
    {
        var stack = Section("This peer");
        stack.Children.Add(_identityLine);
        stack.Children.Add(_caption);

        var row = new StackPanel
        {
            Orientation = Orientation.Horizontal,
            Spacing = 6,
            Margin = new Thickness(0, 6, 0, 0),
        };
        // No Refresh button: this panel holds a SharingRegisterWake
        // subscription over the declarations AND the file layers it
        // counts, so every number here moves by itself. A Refresh control
        // on tree data is a bug report about a missing subscription
        // (AP73), and this panel was the bug report.
        row.Children.Add(_recheckBtn);
        stack.Children.Add(row);
        return stack;
    }

    private Control BuildDevicesSection()
    {
        var stack = Section("Peers you have paired with");
        stack.Children.Add(_devicesEmpty);
        stack.Children.Add(new ListBox
        {
            ItemsSource = _devices,
            Background = Brushes.Transparent,
            BorderThickness = new Thickness(0),
            ItemTemplate = Rows.Of<DeviceVm>((vm, _) => BuildDeviceRow(vm)),
        });
        return stack;
    }

    private Control BuildFoldersSection()
    {
        var stack = Section("Folders — what you declared, and what is actually true");
        stack.Children.Add(_foldersEmpty);
        stack.Children.Add(new ListBox
        {
            ItemsSource = _folders,
            Background = Brushes.Transparent,
            BorderThickness = new Thickness(0),
            ItemTemplate = Rows.Of<FolderVm>((vm, _) => BuildFolderRow(vm)),
        });
        return stack;
    }

    private Control BuildProblemsSection()
    {
        var stack = Section("What is stopping it");
        stack.Children.Add(_problemsEmpty);
        stack.Children.Add(new ListBox
        {
            ItemsSource = _problems,
            Background = Brushes.Transparent,
            BorderThickness = new Thickness(0),
            // Bounded, per the other half of AP64's rule: an unbounded list
            // in a stacked region pushes everything below it out of reach.
            // Bounded, it scrolls within itself instead.
            MaxHeight = 160,
            ItemTemplate = Rows.Of<string>((s, _) => new SelectableTextBlock
            {
                Text = s,
                FontSize = 12,
                Foreground = Brushes.IndianRed,
                TextWrapping = TextWrapping.Wrap,
            }),
        });
        return stack;
    }

    private Control BuildActionsSection()
    {
        var stack = Section("What the last pass changed");
        stack.Children.Add(new ListBox
        {
            ItemsSource = _actions,
            Background = Brushes.Transparent,
            BorderThickness = new Thickness(0),
            MaxHeight = 140,
            ItemTemplate = Rows.Of<string>((s, _) => new SelectableTextBlock
            {
                Text = s,
                FontSize = 12,
                Opacity = 0.8,
                TextWrapping = TextWrapping.Wrap,
            }),
        });
        // Hidden until a pass actually changes something. A permanently
        // empty section is furniture, and a settled system is the normal
        // state — most passes change nothing, which is the good outcome.
        stack.IsVisible = false;
        return stack;
    }

    // --- Rows -------------------------------------------------------------

    private Control BuildDeviceRow(DeviceVm vm)
    {
        var stack = new StackPanel { Margin = new Thickness(4, 4, 4, 4), Spacing = 1 };
        stack.Children.Add(new TextBlock
        {
            Text = vm.Label,
            FontWeight = FontWeight.SemiBold,
            FontSize = 12,
        });
        stack.Children.Add(Line(vm.StateLine, vm.StateBrush));
        if (!string.IsNullOrEmpty(vm.Address))
        {
            stack.Children.Add(Line($"address {vm.Address}", Brushes.Gray));
        }

        // The two directions, said differently on purpose.
        stack.Children.Add(Line($"you grant them: {vm.OutboundGrantOrNothing}", Brushes.Gray));
        stack.Children.Add(new TextBlock
        {
            // NOT a status. This peer cannot read another peer's
            // capability table, so the only honest inbound statement is
            // where to look for evidence — which is the folder rows, where
            // a file count is a thing that actually happened.
            Text = "what they grant you: not knowable from here — the folder rows below show "
                   + "what has actually arrived, which is the only evidence this peer has",
            FontSize = 11,
            Opacity = 0.5,
            FontStyle = FontStyle.Italic,
            TextWrapping = TextWrapping.Wrap,
        });
        if (!string.IsNullOrEmpty(vm.Note))
        {
            stack.Children.Add(Line(vm.Note, Brushes.Goldenrod));
        }

        var buttons = new StackPanel
        {
            Orientation = Orientation.Horizontal,
            Spacing = 6,
            Margin = new Thickness(0, 4, 0, 0),
        };
        buttons.Children.Add(vm.Paused
            ? RowButton("Resume",
                "Start maintaining a connection to this peer again from the next pass.",
                () => _ = PauseAsync(vm.PeerId, false))
            : RowButton("Pause",
                "Stop keeping this relationship alive, without forgetting it. Nothing is "
                + "disconnected now and no files are touched; the next pass simply leaves "
                + "this peer alone until it is resumed.",
                () => _ = PauseAsync(vm.PeerId, true)));
        stack.Children.Add(buttons);
        return stack;
    }

    private Control BuildFolderRow(FolderVm vm)
    {
        var stack = new StackPanel { Margin = new Thickness(4, 4, 4, 4), Spacing = 1 };
        stack.Children.Add(new TextBlock
        {
            Text = vm.Label,
            FontWeight = FontWeight.SemiBold,
            FontSize = 12,
        });
        stack.Children.Add(Line(vm.DirectionLine, Brushes.Gainsboro));
        stack.Children.Add(Line(vm.EstablishedLine, vm.EstablishedBrush));
        stack.Children.Add(Line(vm.FilesLine, Brushes.Gray));
        if (!string.IsNullOrEmpty(vm.Path))
        {
            stack.Children.Add(Line(vm.Path, Brushes.Gray));
        }
        foreach (var p in vm.Peers)
        {
            stack.Children.Add(Line($"    {p.Label} — {p.State}", Brushes.Gray));
        }
        if (!string.IsNullOrEmpty(vm.Note))
        {
            stack.Children.Add(Line(vm.Note, Brushes.Goldenrod));
        }

        // The action the reconciler NAMES and refuses to take, offered
        // here — which is the bargain the loop's design depends on. It
        // takes no directory: the path comes from the declaration, which
        // is the only record left that can say where this folder's files
        // are once the mount that knew is gone.
        if (vm.CanRemount)
        {
            var buttons = new StackPanel
            {
                Orientation = Orientation.Horizontal,
                Spacing = 6,
                Margin = new Thickness(0, 4, 0, 0),
            };
            buttons.Children.Add(RowButton("Remount",
                $"Bridge {vm.Path} again, at the root this folder is declared under. "
                + "This is the one thing the control loop will not do for you, because "
                + "it writes to a directory on your disk.",
                () => _ = RemountAsync(vm.Id)));
            stack.Children.Add(buttons);
        }
        return stack;
    }

    // --- Bridge calls -----------------------------------------------------

    // Refresh READS. No write, no dial — safe to call from anywhere,
    // including a wake if one is ever added.
    public void Refresh()
    {
        var reply = Bridge.TakeString(Bridge.StatusRender(_peerHandle));
        Apply(reply, "status render");
    }

    // ReconcileAsync runs ONE pass, off the UI thread.
    //
    // `StatusReconcile` is a synchronous cgo export that dials (AP31 is
    // why it is not async on the Go side), so it goes on a thread-pool
    // worker — the same shape SharePanel uses for offers and accept.
    public async Task ReconcileAsync()
    {
        if (_busy) return;
        _busy = true;
        _recheckBtn.IsEnabled = false;
        SetStatus("re-checking — connecting to declared peers…", Brushes.Gainsboro);
        try
        {
            var reply = await Task.Run(() =>
                Bridge.TakeString(Bridge.StatusReconcile(_peerHandle)));
            Apply(reply, "re-check");
        }
        finally
        {
            _busy = false;
            _recheckBtn.IsEnabled = true;
        }
    }

    // PerformRecheckForTests is the driver seam. Tests await this rather
    // than synthesizing a click: `.GetAwaiter().GetResult()` on the test
    // thread deadlocks the headless dispatcher, because the continuation
    // resumes on the UI thread (measured — the suite hung).
    public Task PerformRecheckForTests() => ReconcileAsync();

    private async Task PauseAsync(string peerId, bool paused)
    {
        if (_busy) return;
        _busy = true;
        try
        {
            var reply = await Task.Run(() =>
                Bridge.TakeString(Bridge.StatusPauseDevice(_peerHandle, peerId, paused ? 1 : 0)));
            var dto = Decode<ActionReply>(reply, out var err);
            if (dto == null || !dto.Ok)
            {
                SetStatus($"could not change that peer: {(dto?.Error is { Length: > 0 } e ? e : err)}",
                    Brushes.IndianRed);
                return;
            }
            SetStatus(dto.Note, Brushes.Gainsboro);
        }
        finally { _busy = false; }
        // A read, not a pass: pausing changes a declaration and nothing
        // that needs dialing to observe. Re-checking here would dial every
        // OTHER declared peer as a side effect of pausing one.
        Refresh();
    }

    private async Task RemountAsync(string folderId)
    {
        if (_busy) return;
        _busy = true;
        try
        {
            var reply = await Task.Run(() =>
                Bridge.TakeString(Bridge.StatusRemountFolder(_peerHandle, folderId)));
            var dto = Decode<ActionReply>(reply, out var err);
            if (dto == null || !dto.Ok)
            {
                SetStatus($"remount failed: {(dto?.Error is { Length: > 0 } e ? e : err)}",
                    Brushes.IndianRed);
                return;
            }
            SetStatus(dto.Note, Brushes.Gainsboro);
        }
        finally { _busy = false; }

        // A pass, here, and deliberately: a remount is exactly the missing
        // precondition the loop was waiting on, so the useful next thing
        // is for it to establish everything that mount unblocks. This is
        // the "after any mutation the panel performs" case.
        await ReconcileAsync();
    }

    private void Apply(string reply, string what)
    {
        var dto = Decode<RenderEnvelope>(reply, out var err);
        if (dto == null || !dto.Ok)
        {
            SetStatus($"{what} failed: {(dto?.Error is { Length: > 0 } e ? e : err)}",
                Brushes.IndianRed);
            return;
        }

        _identityLine.Text =
            $"{(string.IsNullOrEmpty(dto.LocalAlias) ? "this peer" : dto.LocalAlias)}   {dto.LocalPeerId}";

        // The caption is the honesty control. "Read" and "verified by a
        // pass" are different claims and the panel must not let the weaker
        // one wear the stronger one's clothes.
        var when = DateTime.Now.ToString("HH:mm:ss");
        _caption.Text = dto.Reconciled
            ? $"re-checked at {when} — this is what the pass established."
            : $"read at {when} — these are the records and what this peer can observe. "
              + "Nothing has been re-established; \"Re-check now\" does that.";

        _devices.Clear();
        foreach (var d in dto.Devices ?? new List<DeviceDto>())
        {
            _devices.Add(new DeviceVm(d.PeerId, d.Label, d.Address, d.Maintained, d.Connected,
                d.Paused, d.OutboundGrant, d.Note));
        }
        _devicesEmpty.IsVisible = _devices.Count == 0;

        _folders.Clear();
        foreach (var f in dto.Folders ?? new List<FolderDto>())
        {
            var peers = new List<FolderPeerVm>();
            foreach (var p in f.Peers ?? new List<FolderPeerDto>())
            {
                peers.Add(new FolderPeerVm(p.PeerId, p.Label, p.State));
            }
            _folders.Add(new FolderVm(f.Id, f.Label, f.Local, f.Origin, f.Root, f.LocalRoot,
                f.Path, f.Mounted, f.Syncing, f.Accepted, f.FilesPresent, f.FilesIngested,
                f.FilesObservable, f.Note, peers));
        }
        _foldersEmpty.IsVisible = _folders.Count == 0;

        _problems.Clear();
        foreach (var p in dto.Problems ?? new List<string>()) _problems.Add(p);
        _problemsEmpty.IsVisible = _problems.Count == 0;

        _actions.Clear();
        foreach (var a in dto.Actions ?? new List<string>()) _actions.Add(a);
        _actionsSection.IsVisible = _actions.Count > 0;

        if (dto.Reconciled)
        {
            SetStatus(
                dto.Settled
                    ? "settled — everything declared is established."
                    : $"{_actions.Count} change(s), {_problems.Count} problem(s) — see below.",
                dto.Settled ? Brushes.DarkSeaGreen : Brushes.Goldenrod);
        }
    }

    private void SetStatus(string text, IBrush brush)
    {
        _status.Text = text;
        _status.Foreground = brush;
    }

    // --- Helpers ----------------------------------------------------------

    private static string Short(string peerId)
        => string.IsNullOrEmpty(peerId) || peerId.Length <= 12 ? peerId : peerId[..12] + "…";

    private static StackPanel Section(string header)
    {
        var stack = new StackPanel
        {
            Orientation = Orientation.Vertical,
            Margin = new Thickness(12, 8, 12, 2),
        };
        stack.Children.Add(new TextBlock
        {
            Text = header,
            FontWeight = FontWeight.SemiBold,
            FontSize = 13,
            Opacity = 0.85,
            Margin = new Thickness(0, 0, 0, 4),
        });
        return stack;
    }

    private static TextBlock Hint(string text) => new()
    {
        Text = text,
        FontSize = 11,
        FontStyle = FontStyle.Italic,
        Opacity = 0.5,
        TextWrapping = TextWrapping.Wrap,
        Margin = new Thickness(0, 2, 0, 2),
    };

    private static TextBlock Line(string text, IBrush brush) => new()
    {
        Text = text,
        FontSize = 12,
        Foreground = brush,
        TextWrapping = TextWrapping.Wrap,
    };

    // AP37/P7: pointer input on a Button is never wired with `+=`. Click is
    // the routed-command surface and is safe; it is the press events Button
    // marks handled in its own override.
    private static Button RowButton(string label, string tip, Action onClick)
    {
        var b = new Button { Content = label, FontSize = 11, Padding = new Thickness(8, 2) };
        ToolTip.SetTip(b, tip);
        b.Click += (_, _) => onClick();
        return b;
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

    private static readonly JsonSerializerOptions JsonOpts = new()
    {
        PropertyNameCaseInsensitive = true,
    };

    // --- View models ------------------------------------------------------

    private sealed record DeviceVm(
        string PeerId, string LabelRaw, string Address, bool Maintained, bool Connected,
        bool Paused, string OutboundGrant, string Note)
    {
        public string Label => string.IsNullOrEmpty(LabelRaw) || LabelRaw == PeerId
            ? Short(PeerId)
            : $"{LabelRaw}  ({Short(PeerId)})";

        // "connected" and "maintained" are different claims and this panel
        // exists because collapsing them hid a restart defect for months: a
        // peer can be connected by something else entirely while nothing is
        // keeping the relationship alive, and after a restart that is the
        // normal state until a pass runs.
        public string StateLine => Paused
            ? "paused — nothing is being kept alive for this peer"
            : (Connected, Maintained) switch
            {
                (true, true) => "connected, and the relationship is being maintained",
                (true, false) => "connected, but nothing is keeping it alive — a re-check installs that",
                (false, true) => "offline — being retried automatically",
                _ => "offline, and nothing is retrying it — run a re-check",
            };

        public IBrush StateBrush => Paused
            ? Brushes.Gray
            : (Connected && Maintained) ? Brushes.DarkSeaGreen
            : Maintained ? Brushes.Goldenrod
            : Brushes.IndianRed;

        public string OutboundGrantOrNothing =>
            string.IsNullOrWhiteSpace(OutboundGrant) ? "(nothing)" : OutboundGrant;
    }

    private sealed record FolderPeerVm(string PeerId, string Label, string State);

    private sealed record FolderVm(
        string Id, string LabelRaw, bool Local, string Origin, string Root, string LocalRoot,
        string Path, bool Mounted, bool Syncing, bool Accepted,
        int FilesPresent, int FilesIngested, bool FilesObservable, string Note,
        List<FolderPeerVm> Peers)
    {
        public string Label => string.IsNullOrEmpty(LabelRaw) ? Id : LabelRaw;

        public string DirectionLine => Local
            ? "shared out from this peer"
            : $"received from {Short(Origin)}"
              + (string.IsNullOrEmpty(LocalRoot) || LocalRoot == Root
                  ? ""
                  : $" — their \"{Root}\" lands in your \"{LocalRoot}\"");

        // Declared vs. actual, as one line, because the difference between
        // them IS the diagnostic.
        public string EstablishedLine
        {
            get
            {
                if (Local)
                {
                    return Mounted
                        ? "mounted — this folder can publish"
                        : "NO MOUNT — nothing is published from this folder";
                }
                if (!Accepted) return "not accepted — nothing is established for it";
                if (!Mounted) return "NO MOUNT — every delivery will be refused until one exists";
                return Syncing
                    ? "mounted and subscribed"
                    : "mounted, but not subscribed — a re-check establishes it";
            }
        }

        public IBrush EstablishedBrush
        {
            get
            {
                if (Local) return Mounted ? Brushes.DarkSeaGreen : Brushes.IndianRed;
                if (!Accepted) return Brushes.Gray;
                if (!Mounted) return Brushes.IndianRed;
                return Syncing ? Brushes.DarkSeaGreen : Brushes.Goldenrod;
            }
        }

        // Both sides of the mount's lossy stage (AP59), and an explicit
        // *unknown* when there is no mount to count. A confident "0 files"
        // for a folder that has nowhere to put them is the wrong answer to
        // the operator's actual question.
        //
        // **Neither number is "on disk", and saying so was a live defect.**
        // FilesPresent counts tree entries under `local/files/{root}/` —
        // the SOURCE layer, which is what the watcher has admitted, not
        // what the filesystem holds. The Local Files panel's sweep prints
        // a real `filepath.Walk` count under the same words, so the two
        // panels showed different numbers both labelled "on disk" and an
        // operator reasonably read that as one of them being broken. AP59
        // is about reporting both sides of a lossy stage; this is the same
        // rule applied to the NAMES, which the original fix did not do.
        public string FilesLine => !FilesObservable
            ? "files: unknown — there is no mount to count"
            : $"files: {FilesPresent} admitted by the watcher, {FilesIngested} readable as documents";

        // Offered only when the loop cannot proceed AND the declaration
        // remembers where the directory was. Without a path there is
        // nothing to remount and a button would produce a refusal.
        public bool CanRemount => !Mounted && !string.IsNullOrWhiteSpace(Path)
                                  && (Local || Accepted);
    }

    // --- DTOs -------------------------------------------------------------
    //
    // AP49: an undeclared member is discarded by System.Text.Json in TOTAL
    // silence. This panel's whole content is fields that mean "something is
    // wrong", so a dropped one renders as "everything is fine" — the worst
    // available failure for a surface whose only job is to say otherwise.
    // SharingStatusPanelTests asserts each one arrives.

    public sealed class RenderEnvelope
    {
        [JsonPropertyName("ok")] public bool Ok { get; set; }
        [JsonPropertyName("error")] public string Error { get; set; } = "";
        [JsonPropertyName("reconciled")] public bool Reconciled { get; set; }
        [JsonPropertyName("localPeerId")] public string LocalPeerId { get; set; } = "";
        [JsonPropertyName("localAlias")] public string LocalAlias { get; set; } = "";
        [JsonPropertyName("devices")] public List<DeviceDto>? Devices { get; set; }
        [JsonPropertyName("folders")] public List<FolderDto>? Folders { get; set; }
        [JsonPropertyName("actions")] public List<string>? Actions { get; set; }
        [JsonPropertyName("problems")] public List<string>? Problems { get; set; }
        [JsonPropertyName("settled")] public bool Settled { get; set; }
    }

    public sealed class DeviceDto
    {
        [JsonPropertyName("peerId")] public string PeerId { get; set; } = "";
        [JsonPropertyName("label")] public string Label { get; set; } = "";
        [JsonPropertyName("address")] public string Address { get; set; } = "";
        [JsonPropertyName("maintained")] public bool Maintained { get; set; }
        [JsonPropertyName("connected")] public bool Connected { get; set; }
        [JsonPropertyName("paused")] public bool Paused { get; set; }
        [JsonPropertyName("note")] public string Note { get; set; } = "";
        [JsonPropertyName("outboundGrant")] public string OutboundGrant { get; set; } = "";
        [JsonPropertyName("addedAtMillis")] public ulong AddedAtMillis { get; set; }
        [JsonPropertyName("lastSeenMillis")] public ulong LastSeenMillis { get; set; }
    }

    public sealed class FolderPeerDto
    {
        [JsonPropertyName("peerId")] public string PeerId { get; set; } = "";
        [JsonPropertyName("label")] public string Label { get; set; } = "";
        [JsonPropertyName("state")] public string State { get; set; } = "";
        [JsonPropertyName("atMillis")] public ulong AtMillis { get; set; }
        [JsonPropertyName("note")] public string Note { get; set; } = "";
    }

    public sealed class FolderDto
    {
        [JsonPropertyName("id")] public string Id { get; set; } = "";
        [JsonPropertyName("label")] public string Label { get; set; } = "";
        [JsonPropertyName("local")] public bool Local { get; set; }
        [JsonPropertyName("root")] public string Root { get; set; } = "";
        [JsonPropertyName("localRoot")] public string LocalRoot { get; set; } = "";
        [JsonPropertyName("path")] public string Path { get; set; } = "";
        [JsonPropertyName("origin")] public string Origin { get; set; } = "";
        [JsonPropertyName("mode")] public string Mode { get; set; } = "";
        [JsonPropertyName("mounted")] public bool Mounted { get; set; }
        [JsonPropertyName("syncing")] public bool Syncing { get; set; }
        [JsonPropertyName("accepted")] public bool Accepted { get; set; }
        [JsonPropertyName("acceptedAtMillis")] public ulong AcceptedAtMillis { get; set; }
        [JsonPropertyName("note")] public string Note { get; set; } = "";
        [JsonPropertyName("filesPresent")] public int FilesPresent { get; set; }
        [JsonPropertyName("filesIngested")] public int FilesIngested { get; set; }
        [JsonPropertyName("filesObservable")] public bool FilesObservable { get; set; }
        [JsonPropertyName("peers")] public List<FolderPeerDto>? Peers { get; set; }
    }

    public sealed class ActionReply
    {
        [JsonPropertyName("ok")] public bool Ok { get; set; }
        [JsonPropertyName("error")] public string Error { get; set; } = "";
        [JsonPropertyName("note")] public string Note { get; set; } = "";
    }
}
