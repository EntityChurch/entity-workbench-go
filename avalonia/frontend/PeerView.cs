using System;
using System.Collections.Generic;
using System.Linq;
using System.Runtime.InteropServices;
using System.Text.Json;
using System.Text.Json.Serialization;
using Avalonia;
using Avalonia.Automation;
using Avalonia.Controls;
using Avalonia.Layout;
using Avalonia.Media;
using Avalonia.Threading;
using EntityAvalonia.Panels;

namespace EntityAvalonia;

// PeerView is the per-peer UserControl — one instance per tab. Owns
// everything that lives per-peer except the dispatch surface, which
// since I.5 wave 2 is owned by ShellPanel (one shell per panel
// instance via shellcmd.NewShellInWorkspace).
//
// PHASE-I-MULTI-PEER-PLAN.md §5.3 — extracted from MainWindow's
// single-peer flat layout. Each PeerView constructs its own
// PeerResolver with the peer's handle as the active override; panels
// inside this view bind to that override via the resolver (not by
// receiving the handle as a free-floating argument).
//
// PHASE-I-DESKTOP-RENDERER-PLAN §I.5 wave 2 — the right column is
// now just the PanelStack. Default boot includes a shell panel so
// the dispatch surface is still present at startup; the user can
// close it, open more, or rearrange via the stack chrome.
//
// IPanelHost surface — broadcasts tree selection to detail-shaped
// panels, and accepts peer-status refresh requests from shells.
//
// Lifetime: created when a peer's tab opens; disposed when the tab
// closes. Disposing tears down the tree + panel stack. PeerDestroy
// on the underlying handle is the caller's responsibility (MainWindow
// owns peer lifecycle).
public sealed class PeerView : UserControl, IDisposable, IPanelHost
{
    public long PeerHandle { get; }
    public string Alias { get; private set; } = "";
    public PeerResolver Resolver { get; }

    // IPanelHost — broadcast tree selection to any DetailPanel-shaped
    // panel currently mounted in a switchable slot.
    public event Action<string>? SelectedPath;
    public string? CurrentSelectedPath { get; private set; }

    private readonly TreeViewPanel _tree;
    private readonly PanelStack _panelStack;

    // The arrangement this view opened on, and the column whose width is
    // part of it. Both are read once at construction and written back on
    // every change.
    private readonly LayoutDto _layout;
    private readonly ColumnDefinition _navColumn = null!;

    // _layoutNote carries anything that went wrong with the saved layout —
    // a file that would not parse, a panel kind this build does not know,
    // a save that failed. It is rendered on the peer status line rather
    // than logged, because the operator is the only person who can act on
    // it and a workspace that quietly forgets is the exact complaint this
    // feature answers.
    private string _layoutNote = "";

    // Per-peer status bar — shows alias, peer-id, identity, connections.
    private readonly SelectableTextBlock _peerStatus;

    // THE PROBLEM BANNER — always present, independent of which panels the
    // operator has open.
    //
    // Why it exists (2026-09-10, from a real two-machine session). The
    // reconciler diagnosed the fault correctly and completely at startup:
    // "could not open our own connection to this peer — nothing we write to
    // a shared folder will reach them". That sentence went to STDERR, i.e.
    // to `avalonia/run-logs/`, where nobody looks. The operator's layout
    // held `tree-view` and `peer-connections` — the two panels that cannot
    // say anything about sharing — so the app's own correct diagnosis was
    // on screen nowhere, while the peer status line said "1 remote" and the
    // Nearby list said "Connected". The operator concluded, reasonably,
    // that the app was fine and the files were being eaten.
    //
    // This is D23/AP73's shape aimed at the APPLICATION rather than at a
    // model: we fixed "the reconciler's output reaches no pixel" for the
    // Sharing Status PANEL and left it true for anyone who does not have
    // that panel open. A diagnosis whose visibility depends on the
    // operator's layout is not a surface.
    //
    // It is deliberately NOT dismissible and NOT collapsed-by-default: the
    // conditions it reports are exactly the ones that are silent otherwise.
    private readonly Border _problemBanner;
    private readonly StackPanel _problemLines;
    private readonly TextBlock _problemHeading;
    private Bridge.TreeWakeCallback? _problemWakeCallback;
    private GCHandle _problemWakeHandle;
    // The registration id, so Dispose can actually UNREGISTER.
    //
    // It was discarded until 2026-09-10, which meant Dispose could not
    // unregister even in principle — it freed the pinned delegate and
    // left Go holding the pointer. The next sharing-tree change then
    // called into a collected delegate and the runtime ABORTED THE
    // PROCESS: "a callback was made on a garbage collected delegate".
    // That is what had been killing `make -C avalonia test` part way
    // through, taking an unknown number of tests with it.
    private long _problemWakeRegistration = -1;

    private bool _disposed;

    public PeerView(long peerHandle, long systemPeerHandle)
    {
        PeerHandle = peerHandle;
        Resolver = new PeerResolver(systemPeerHandle, activePeerOverride: peerHandle);

        _peerStatus = new SelectableTextBlock
        {
            Text = "(loading peer status…)",
            Opacity = 0.6,
            Margin = new Thickness(12, 8),
            FontSize = 13,
        };

        _problemHeading = new TextBlock
        {
            FontWeight = FontWeight.Bold,
            FontSize = 13,
            Foreground = Brushes.White,
            Margin = new Thickness(0, 0, 0, 4),
        };
        _problemLines = new StackPanel { Orientation = Orientation.Vertical };
        _problemBanner = new Border
        {
            Background = new SolidColorBrush(Color.FromRgb(0x8B, 0x27, 0x27)),
            Padding = new Thickness(12, 8),
            IsVisible = false,
            Child = new StackPanel
            {
                Orientation = Orientation.Vertical,
                Children = { _problemHeading, _problemLines },
            },
        };
        AutomationProperties.SetAutomationId(_problemBanner, "PeerProblemBanner");

        // Resolve the alias BEFORE the panel stack is built, because the
        // saved layout is keyed by it. RefreshPeerStatus used to run only
        // at the end of the constructor; the stack cannot wait for it.
        RefreshPeerStatus();
        _layout = LoadLayout(Alias);

        // Tree stays hard-mounted as the left-column navigator — it's
        // structurally privileged. Tree selection feeds IPanelHost.
        // SelectedPath so any DetailPanel-shaped slot can subscribe
        // without holding a direct reference to the tree.
        _tree = new TreeViewPanel(Resolver.ResolveForPanel(PanelScope.Peer));
        _tree.EntitySelected += OnTreeEntitySelected;

        // Right column is a dynamic PanelStack, opened on the layout this
        // peer was last left in.
        //
        // It used to be three hard-coded names, and that was the single
        // largest piece of friction in the app: an operator who opened
        // the Browser and the Local Files panel and closed Detail got the
        // original three back on every launch, forever, with no
        // indication that anything had been remembered or forgotten. A
        // workspace you have to rebuild each session is not a workspace.
        //
        // Unknown names are skipped rather than refused (see MountPanels),
        // which is what lets a layout written by a newer build open in an
        // older one instead of failing the launch.
        var peerH = Resolver.ResolveForPanel(PanelScope.Peer);
        _panelStack = new PanelStack(peerH, this, MountablePanels(_layout.Panels));
        _panelStack.LayoutChanged += SaveLayout;

        // --- Left+right horizontal split.
        //
        // MinWidth/MaxWidth pin both columns — same structural
        // mitigation applied in PanelStack (see its class doc). The
        // crashes included a splitter drag here too; an
        // unbounded star column can be driven to zero by the splitter,
        // triggering the same Avalonia layout-engine self-recursion
        // that crashes the CLR.
        var split = new Grid();
        // Restored width, clamped into the same [MinWidth, MaxWidth] band
        // the column pins. The clamp is not politeness: an unbounded star
        // column driven to zero by a splitter drag is what triggered the
        // Avalonia layout-engine self-recursion that crashed the CLR, and
        // a width read from a file an operator can edit is exactly the
        // input that could reintroduce it.
        _navColumn = new ColumnDefinition(new GridLength(ClampNavWidth(_layout.NavWidth)))
        {
            MinWidth = 200,
            MaxWidth = 700,
        };
        split.ColumnDefinitions.Add(_navColumn);
        split.ColumnDefinitions.Add(new ColumnDefinition(new GridLength(4)));
        split.ColumnDefinitions.Add(new ColumnDefinition(GridLength.Star)
        {
            MinWidth = 400,
        });
        var treeContainer = new Border
        {
            Child = _tree,
            BorderBrush = new SolidColorBrush(Color.FromArgb(0x33, 0xff, 0xff, 0xff)),
            BorderThickness = new Thickness(0, 0, 1, 0),
        };
        var splitter = new GridSplitter
        {
            Background = new SolidColorBrush(Color.FromArgb(0x33, 0xff, 0xff, 0xff)),
            ResizeDirection = GridResizeDirection.Columns,
        };
        Grid.SetColumn(treeContainer, 0);
        Grid.SetColumn(splitter, 1);
        Grid.SetColumn(_panelStack, 2);
        split.Children.Add(treeContainer);
        split.Children.Add(splitter);
        split.Children.Add(_panelStack);

        // A splitter drag has no completion event, so the width is
        // captured when the drag settles the column — DragCompleted is
        // the one signal that means "the operator stopped", and saving on
        // every pixel of a drag would rewrite the file a hundred times a
        // second.
        splitter.DragCompleted += (_, _) => SaveLayout();

        var root = new DockPanel { LastChildFill = true };
        DockPanel.SetDock(_peerStatus, Dock.Top);
        DockPanel.SetDock(_problemBanner, Dock.Top);
        root.Children.Add(_peerStatus);
        root.Children.Add(_problemBanner);
        root.Children.Add(split);
        Content = root;

        // The banner is wired to the declaration watch, not to a timer and
        // not to a reconcile: StatusRender is a READ (shellcmd/status.go —
        // it observes and never dials), so refreshing it on a wake cannot
        // turn an open window into a dialer. See the export's own comment
        // for why the pass and the read are two exports.
        RegisterProblemWake();
        RefreshProblems();

        // Second status refresh, and it is not redundant. The first ran
        // before the layout was read, because the alias it resolves is
        // the layout's key. Anything the layout had to say — a file that
        // would not parse, a panel kind this build does not know — was
        // therefore recorded AFTER the line was drawn, and without this
        // the operator would never see it. That is the failure mode this
        // whole feature is about, reproduced one level down.
        RefreshPeerStatus();
    }

    // --- Layout persistence ---------------------------------------------

    // LoadLayout reads this peer's saved arrangement. A layout file that
    // exists and will not parse is reported on the status line and the
    // peer opens on the defaults: losing an arrangement is survivable,
    // not being told why is the thing that reads as "it forgot again".
    private LayoutDto LoadLayout(string alias)
    {
        try
        {
            var reply = Bridge.TakeString(Bridge.LayoutLoad(alias));
            var dto = JsonSerializer.Deserialize<LayoutDto>(reply, LayoutJsonOpts);
            if (dto == null) return LayoutDto.Fallback();
            if (!string.IsNullOrEmpty(dto.Error))
            {
                _layoutNote = $"saved layout not loaded — {dto.Error}";
            }
            if (dto.Panels == null || dto.Panels.Count == 0) return LayoutDto.Fallback();
            return dto;
        }
        catch (JsonException ex)
        {
            _layoutNote = $"saved layout not loaded — {ex.Message}";
            return LayoutDto.Fallback();
        }
    }

    // MountablePanels filters the saved names down to kinds this build
    // registers.
    //
    // Skipping an unknown name rather than refusing the layout is what
    // lets a file written by a newer build open in an older one. The
    // dropped names are NOT silently forgotten, though: they are noted on
    // the status line, because a panel that vanishes with no explanation
    // is the same complaint as a layout that resets with no explanation.
    // They are also not written back out until the operator changes
    // something, at which point the arrangement genuinely is what is on
    // screen.
    private string[] MountablePanels(List<string>? names)
    {
        var known = new List<string>();
        var dropped = new List<string>();
        foreach (var n in names ?? new List<string>())
        {
            if (PanelRegistry.Get(n) != null) known.Add(n);
            else dropped.Add(n);
        }
        if (dropped.Count > 0)
        {
            var note = $"layout referenced {dropped.Count} unknown panel kind(s): {string.Join(", ", dropped)}";
            _layoutNote = string.IsNullOrEmpty(_layoutNote) ? note : _layoutNote + " · " + note;
        }
        // An arrangement that filtered down to nothing opens on the
        // defaults rather than on a blank column — a workspace with no
        // panels and no message reads as a broken launch.
        return known.Count > 0 ? known.ToArray() : LayoutDto.Fallback().Panels!.ToArray();
    }

    // SaveLayout persists the current arrangement. Best-effort by design:
    // a workspace that refuses to close a panel because ~/.entity is
    // read-only would be trading a real capability for a convenience.
    // The failure is surfaced on the status line rather than swallowed.
    private void SaveLayout()
    {
        if (_disposed) return;
        try
        {
            var panels = JsonSerializer.Serialize(_panelStack.PanelNames);
            var width = _navColumn.Width.IsAbsolute ? _navColumn.Width.Value : _navColumn.ActualWidth;
            var reply = Bridge.TakeString(Bridge.LayoutSave(Alias, panels, ClampNavWidth(width)));
            var dto = JsonSerializer.Deserialize<LayoutSaveDto>(reply, LayoutJsonOpts);
            if (dto != null && !dto.Ok && !string.IsNullOrEmpty(dto.Error))
            {
                _layoutNote = $"layout not saved — {dto.Error}";
                RefreshPeerStatus();
            }
        }
        catch (JsonException ex)
        {
            _layoutNote = $"layout not saved — {ex.Message}";
        }
    }

    // ClampNavWidth keeps a restored or measured width inside the band
    // the column pins, and rejects the non-finite values a measurement
    // can produce before first layout. 360 is the historical default.
    private static double ClampNavWidth(double w)
    {
        if (double.IsNaN(w) || double.IsInfinity(w) || w <= 0) return 360;
        return Math.Clamp(w, 200, 700);
    }

    // FocusInput delegates to the first shell panel in the stack, if
    // any. Tab-open puts the user at a shell prompt by default; with
    // per-panel shells we focus the topmost one.
    public void FocusInput()
    {
        for (int i = 0; i < _panelStack.SlotCountForTests; i++)
        {
            if (_panelStack.SlotAtForTests(i).CurrentPanelControlForSmoke
                is ShellPanel sp)
            {
                sp.FocusInput();
                return;
            }
        }
    }

    // Smoke-driver hooks. Used by SmokeDriver to drive the panel
    // pipeline programmatically under Xvfb (see SmokeDriver.cs +
    // PHASE-I-RELIABILITY-PLAN.md). Not for general use.
    internal TreeViewPanel TreeForSmoke => _tree;
    internal PanelStack PanelStackForSmoke => _panelStack;

    // The rendered status line, including any layout note. Exposed so a
    // test can assert the operator was TOLD about a layout that would not
    // load — asserting only that the defaults appeared would pass equally
    // well against a silent reset, which is the behaviour being ruled out.
    internal string StatusLineForTests => _peerStatus.Text ?? "";

    // SwitchMiddleSlotForSmoke / SwitchBottomSlotForSmoke retained for
    // the existing markdown-cycle driver — both delegate to the first
    // and last slots of the stack respectively. If the user has closed
    // a slot, the driver finds the nearest live slot (idx 0 or -1).
    internal void SwitchMiddleSlotForSmoke(string panelName)
    {
        if (_panelStack.SlotCountForTests > 0)
            _panelStack.SlotAtForTests(0).SwitchTo(panelName);
    }
    internal void SwitchBottomSlotForSmoke(string panelName)
    {
        var n = _panelStack.SlotCountForTests;
        if (n > 0)
            _panelStack.SlotAtForTests(n - 1).SwitchTo(panelName);
    }

    // SITE smoke surface — yields the live SiteViewPanel from whichever
    // slot is hosting one, or null if no slot holds a site-view.
    internal Panels.SiteViewPanel? SiteForSmoke
    {
        get
        {
            for (int i = 0; i < _panelStack.SlotCountForTests; i++)
            {
                if (_panelStack.SlotAtForTests(i).CurrentPanelControlForSmoke
                    is Panels.SiteViewPanel sp)
                {
                    return sp;
                }
            }
            return null;
        }
    }

    // HANDLER-BROWSER smoke surface (WB_SMOKE_HANDLERS). Same shape as
    // the others: find the mounted panel so the driver can select and
    // execute against it under real X11.
    internal Panels.HandlerBrowserPanel? HandlersForSmoke
    {
        get
        {
            for (int i = 0; i < _panelStack.SlotCountForTests; i++)
            {
                if (_panelStack.SlotAtForTests(i).CurrentPanelControlForSmoke
                    is Panels.HandlerBrowserPanel hp)
                {
                    return hp;
                }
            }
            return null;
        }
    }

    // PEER-CONNECTIONS smoke surface (WB_SMOKE_CONNECTIONS). Reaches the
    // panel that renders BOTH connection surfaces — the local pool and
    // the tree's liveness record — under real X11.
    internal Panels.PeerConnectionsPanel? ConnectionsForSmoke
    {
        get
        {
            for (int i = 0; i < _panelStack.SlotCountForTests; i++)
            {
                if (_panelStack.SlotAtForTests(i).CurrentPanelControlForSmoke
                    is Panels.PeerConnectionsPanel cp)
                {
                    return cp;
                }
            }
            return null;
        }
    }

    // GENERIC-HOST smoke surface — the one accessor for ALL programs mounted
    // through ProgramPanel (WB_SMOKE_PROGRAM=life|snake|asteroids).
    //
    // Note there is exactly one of these, where the three above are one per
    // program. That collapse is the rung, visible in the test surface.
    internal Panels.ProgramPanel? ProgramForSmoke
    {
        get
        {
            for (int i = 0; i < _panelStack.SlotCountForTests; i++)
            {
                if (_panelStack.SlotAtForTests(i).CurrentPanelControlForSmoke
                    is Panels.ProgramPanel pp)
                {
                    return pp;
                }
            }
            return null;
        }
    }

    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        _tree.Dispose();
        _panelStack.Dispose();

        // UNREGISTER FIRST, THEN FREE. This used to free the pinned
        // delegate and stop, on the argument that `_disposed` protects the
        // callback — but that flag is checked INSIDE the managed callback,
        // which can only run if the delegate still exists. The crash
        // happens at the call itself, before any C# of ours runs, and it
        // is not catchable: the runtime aborts the process.
        //
        // A correct-sounding argument for an unsafe thing is worse than no
        // comment, because it stops the next reader looking.
        //
        // SharingUnregisterWake now joins the wake goroutine before it
        // returns (avalonia/bridge/wake_pump.go), so once it has, nothing
        // can be in flight and freeing is safe.
        if (_problemWakeRegistration >= 0)
        {
            Bridge.TakeString(Bridge.SharingUnregisterWake(PeerHandle, _problemWakeRegistration));
            _problemWakeRegistration = -1;
        }
        if (_problemWakeHandle.IsAllocated) _problemWakeHandle.Free();
        _problemWakeCallback = null;
        // The underlying peer handle is NOT destroyed here — MainWindow
        // owns peer lifecycle and calls Bridge.PeerDestroy on tab close.
    }

    private void OnTreeEntitySelected(string path) => PublishSelectedPath(path);

    public void PublishSelectedPath(string path)
    {
        if (string.IsNullOrEmpty(path)) return;
        CurrentSelectedPath = path;
        SelectedPath?.Invoke(path);
    }

    // IPanelHost — ShellPanel calls this after every dispatch because
    // connect/disconnect/cd/identity commands mutate peer state shared
    // across all shells.
    public void RequestPeerStatusRefresh()
    {
        RefreshPeerStatus();
        RefreshProblems();
    }

    // Exposed for the headless suite: the banner is the only surface that
    // reports a peer we cannot dial when no sharing panel is open, so a
    // test that asserts on it is asserting on the thing that actually
    // failed the operator.
    internal void ApplyProblemsForTests(IReadOnlyList<string> problems, bool reconciled)
        => ShowProblems(problems, reconciled);

    internal bool ProblemBannerVisibleForTests => _problemBanner.IsVisible;
    internal string ProblemBannerTextForTests =>
        _problemHeading.Text + "\n" + string.Join("\n",
            _problemLines.Children.OfType<TextBlock>().Select(t => t.Text ?? ""));

    // RegisterProblemWake hangs the banner off the declaration watch that
    // already fans out to the sharing panels. It is a READ-side wake: the
    // callback calls StatusRender, which observes and never dials.
    private void RegisterProblemWake()
    {
        try
        {
            // The delegate is held in a field AND pinned: a collected
            // callback is a use-after-free the moment Go wakes us, and it
            // presents as a crash in unrelated code.
            _problemWakeCallback = OnProblemWakeFromGo;
            _problemWakeHandle = GCHandle.Alloc(_problemWakeCallback);
            var fn = Marshal.GetFunctionPointerForDelegate(_problemWakeCallback);
            var reply = Bridge.TakeString(Bridge.SharingRegisterWake(PeerHandle, fn));
            // KEEP THE ID. Without it there is nothing to unregister with,
            // and the delegate below outlives its own registration.
            _problemWakeRegistration = ParseRegistration(reply);
        }
        catch (Exception ex)
        {
            // A banner that cannot subscribe still refreshes on every
            // RequestPeerStatusRefresh, so degrade rather than fail — but
            // say so, because a silently un-waking warning surface is the
            // same defect one level down.
            PanelLog.Write("peer-view", $"problem banner wake not registered: {ex.Message}");
        }
    }

    // Go calls this on ITS goroutine; every touch of a control has to hop
    // to the UI thread.
    private void OnProblemWakeFromGo(long _)
    {
        if (_disposed) return;
        Dispatcher.UIThread.Post(RefreshProblems);
    }

    // RefreshProblems reads the reconciler's OBSERVATION — never a pass.
    //
    // The caption distinguishes the two, because "verified by a pass just
    // now" and "this is what the last pass established" are different
    // claims and the second is the one a read can make. Carrying it in the
    // outcome rather than remembering which export we called is the same
    // rule the Sharing Status panel follows.
    private void RefreshProblems()
    {
        if (_disposed) return;
        string reply;
        try
        {
            reply = Bridge.TakeString(Bridge.StatusRender(PeerHandle));
        }
        catch (Exception ex)
        {
            ShowProblems(new[] { $"could not read sharing status: {ex.Message}" }, reconciled: false);
            return;
        }

        StatusProblemsDto? dto = null;
        try
        {
            dto = JsonSerializer.Deserialize<StatusProblemsDto>(reply, LayoutJsonOpts);
        }
        catch { }

        if (dto == null || !dto.Ok)
        {
            ShowProblems(new[] { $"sharing status unavailable: {reply}" }, reconciled: false);
            return;
        }
        ShowProblems(dto.Problems ?? Array.Empty<string>(), dto.Reconciled);
    }

    private void ShowProblems(IReadOnlyList<string> problems, bool reconciled)
    {
        _problemLines.Children.Clear();
        if (problems.Count == 0)
        {
            _problemBanner.IsVisible = false;
            return;
        }

        var n = problems.Count;
        var verb = n == 1 ? "problem" : "problems";
        // Measured-vs-observed, stated rather than implied.
        var basis = reconciled
            ? "verified by a pass just now"
            : "as established by the last pass — press Check in Sharing Status to re-verify";
        _problemHeading.Text = $"⚠  {n} sharing {verb} — {basis}";

        foreach (var p in problems)
        {
            _problemLines.Children.Add(new SelectableTextBlock
            {
                Text = "• " + p,
                Foreground = Brushes.White,
                FontSize = 12,
                TextWrapping = TextWrapping.Wrap,
                Margin = new Thickness(0, 1, 0, 1),
            });
        }
        _problemBanner.IsVisible = true;
    }

    private void RefreshPeerStatus()
    {
        var reply = Bridge.TakeString(Bridge.PeerSummary(PeerHandle));
        PeerSummaryDto? dto = null;
        try
        {
            dto = JsonSerializer.Deserialize<PeerSummaryDto>(reply);
        }
        catch { }
        if (dto == null || !dto.Ok)
        {
            _peerStatus.Text = $"peer status unavailable: {reply}";
            _peerStatus.Foreground = Brushes.IndianRed;
            return;
        }
        Alias = dto.Alias;
        var identity = string.IsNullOrEmpty(dto.Identity) ? "ephemeral" : $"identity={dto.Identity}";
        var peerShort = dto.PeerId.Length > 12 ? dto.PeerId.Substring(0, 12) + "…" : dto.PeerId;
        var sys = (PeerHandle == Resolver.SystemPeerHandle) ? " · SYSTEM" : "";
        // "known", not "remote" / "connected". PeerSummary.connections is
        // `len(Shell.Conns)-1` (avalonia/bridge/main.go) — the alias map,
        // i.e. the address book. It counts peers we have a name for, not
        // peers we can reach, and it does not drop when one goes away. It
        // read as "1 remote" for a peer that had been refusing connections
        // all morning (2026-09-10).
        var known = dto.Connections == 1 ? "1 peer known" : $"{dto.Connections} peers known";
        var line = $"@{dto.Alias} · {peerShort} · {identity} · {known}{sys}";
        if (!string.IsNullOrEmpty(_layoutNote))
        {
            line += "  ·  " + _layoutNote;
        }
        _peerStatus.Text = line;
        _peerStatus.Opacity = 0.85;
        if (string.IsNullOrEmpty(_layoutNote))
        {
            _peerStatus.ClearValue(TextBlock.ForegroundProperty);
        }
        else
        {
            _peerStatus.Foreground = Brushes.Orange;
        }
    }

    // --- DTOs --------------------------------------------------------

    // AP49 again: an undeclared member is dropped in silence, and this DTO
    // carries the two fields the banner exists for. If `problems` is ever
    // renamed on the Go side, the banner goes quiet and reports "no
    // problems" for a peer that has them — which is the exact failure this
    // whole surface was built to end. `PeerViewProblemBannerTests` asserts
    // both fields arrive.
    private static long ParseRegistration(string json)
    {
        try
        {
            using var doc = JsonDocument.Parse(json);
            if (doc.RootElement.TryGetProperty("registration", out var r)) return r.GetInt64();
        }
        catch (JsonException) { }
        return -1;
    }

    private sealed class StatusProblemsDto
    {
        [JsonPropertyName("ok")] public bool Ok { get; set; }
        [JsonPropertyName("reconciled")] public bool Reconciled { get; set; }
        [JsonPropertyName("problems")] public string[]? Problems { get; set; }
    }

    private static readonly JsonSerializerOptions LayoutJsonOpts = new()
    {
        PropertyNameCaseInsensitive = true,
    };

    // AP49: an undeclared member is discarded by System.Text.Json in
    // silence. `error` in particular is the one that must not be dropped —
    // without it a layout file that failed to parse is indistinguishable
    // from a peer that never had a layout, which is precisely the "it
    // forgot again" reading this whole feature exists to prevent.
    private sealed class LayoutDto
    {
        [JsonPropertyName("ok")] public bool Ok { get; set; }
        [JsonPropertyName("panels")] public List<string>? Panels { get; set; }
        [JsonPropertyName("navWidth")] public double NavWidth { get; set; }
        [JsonPropertyName("source")] public string Source { get; set; } = "";
        [JsonPropertyName("error")] public string Error { get; set; } = "";

        // Fallback mirrors workbench.DefaultPanelLayout. It exists for
        // the case where the bridge itself could not answer — the panel
        // set is duplicated here deliberately and only here, because the
        // alternative is a peer view that cannot open at all when the
        // layout surface is unavailable.
        public static LayoutDto Fallback() => new()
        {
            Ok = true,
            Panels = new List<string> { "site-view", "detail", "shell" },
        };
    }

    private sealed class LayoutSaveDto
    {
        [JsonPropertyName("ok")] public bool Ok { get; set; }
        [JsonPropertyName("error")] public string Error { get; set; } = "";
    }

    private sealed class PeerSummaryDto
    {
        [JsonPropertyName("ok")] public bool Ok { get; set; }
        [JsonPropertyName("alias")] public string Alias { get; set; } = "";
        [JsonPropertyName("peer_id")] public string PeerId { get; set; } = "";
        [JsonPropertyName("identity")] public string Identity { get; set; } = "";
        [JsonPropertyName("connections")] public int Connections { get; set; }
    }
}
