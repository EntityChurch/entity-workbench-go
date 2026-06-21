using System;
using System.Collections.Generic;
using System.Collections.ObjectModel;
using System.Linq;
using System.Text.Json;
using System.Text.Json.Serialization;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Controls.Templates;
using Avalonia.Layout;
using Avalonia.Media;
using Avalonia.Threading;

namespace EntityAvalonia;

// MainWindow is the application chrome — bridge status, tab strip,
// "+" button to spawn new peers, diagnostic footer. The per-peer UI
// lives in PeerView (one per tab). Per PHASE-I-MULTI-PEER-PLAN.md
// §5.1, §5.3.
//
// Tab model: TabControl + ObservableCollection<PeerTab>. Each PeerTab
// wraps a PeerView + its TabItem chrome. Tab Header shows the peer's
// alias; context menu → Close destroys the peer + removes the tab.
//
// Lifecycle:
//   1. Bridge.Init boots the default peer (from --identity et al. argv).
//   2. RestorePeers() reads the persisted roster + respawns prior peers.
//   3. PeerList() enumerates the full peer set into tabs.
//   4. Default peer's tab is selected; window opens; PeerView focuses input.
//   5. "+" tab opens NewPeerDialog → on submit add tab.
//   6. Right-click tab → Close peer → Bridge.PeerDestroy → tab removed.
//   7. Closing window → Bridge.Shutdown (cascades all peers).
public class MainWindow : Window
{
    private readonly TextBlock _bridgeStatus;
    private readonly SelectableTextBlock _diagStatus;
    // Bridge-init failure path early-returns without constructing
    // _tabs; nothing in that path reads it, so null! is the honest
    // "always initialized before any reachable read" annotation.
    private readonly TabControl _tabs = null!;
    private readonly ObservableCollection<object> _tabItems = new();
    private readonly List<PeerTab> _peerTabs = new();

    // Marker class for the trailing "+" tab item — it's a TabItem with
    // special handling so SelectionChanged opens the dialog instead of
    // showing content.
    private sealed class NewPeerTabMarker
    {
        public override string ToString() => "+";
    }

    // Long-form Avalonia tab item factory. We bind a TabControl with
    // an ItemTemplate that distinguishes PeerTab (a PeerView wrapped
    // with a Header) from the NewPeerTabMarker (the "+" tab).

    public MainWindow()
    {
        Title = "entity-avalonia";
        // PHASE-I-SITE-VIEW-PLAN §5.1 — open maximized so the chrome
        // fills its canvas. The prior 1100×720 default sat tiny on a
        // 1280×1024 (or larger) display and made the panel grid look
        // broken. Width/Height kept as a defensive fallback for
        // environments where WindowState=Maximized is a no-op (some
        // headless WMs); Maximized is the surface-level intent.
        Width = 1400;
        Height = 900;
        WindowState = WindowState.Maximized;
        Background = new SolidColorBrush(Color.FromRgb(0x2b, 0x2d, 0x33));

        _bridgeStatus = new TextBlock
        {
            Text = "(bridge not initialized)",
            Opacity = 0.6,
            Margin = new Thickness(12, 6),
            FontSize = 12,
        };
        _diagStatus = new SelectableTextBlock
        {
            Text = "",
            FontSize = 12,
            Opacity = 0.6,
        };

        // Init bridge first — every PeerView depends on a live default
        // peer handle to construct against.
        var errPtr = Bridge.Init(Program.ConfigJson);
        if (errPtr != IntPtr.Zero)
        {
            _bridgeStatus.Text = $"bridge init failed: {Bridge.TakeString(errPtr)}";
            _bridgeStatus.Foreground = Brushes.IndianRed;
            _bridgeStatus.Opacity = 1.0;

            // Render an empty shell + diagnostic; tabs are non-functional
            // without a bridge.
            var failRoot = new DockPanel { LastChildFill = true };
            DockPanel.SetDock(_bridgeStatus, Dock.Top);
            failRoot.Children.Add(_bridgeStatus);
            failRoot.Children.Add(BuildDiagBar());
            Content = failRoot;
            return;
        }

        var systemPeer = Bridge.DefaultPeer();

        // Replay the roster from disk so non-ephemeral peers from prior
        // sessions reappear. Idempotent w.r.t. the already-booted default
        // peer (the restore loop skips entries matching the system peer's
        // own peer-id; see peerRestoreFromRoster in bridge/peers.go).
        Bridge.TakeString(Bridge.RestorePeers());

        // Build tabs from the current peer set.
        _tabs = new TabControl
        {
            Padding = new Thickness(0),
            ItemsSource = _tabItems,
            ItemTemplate = new FuncDataTemplate<object>((vm, _) =>
            {
                if (vm is NewPeerTabMarker)
                {
                    return new TextBlock
                    {
                        Text = "+",
                        FontSize = 14,
                        FontWeight = FontWeight.SemiBold,
                        Padding = new Thickness(10, 2),
                    };
                }
                if (vm is PeerTab pt)
                {
                    return BuildTabHeader(pt);
                }
                return new TextBlock { Text = vm?.ToString() ?? "" };
            }, supportsRecycling: false),
            ContentTemplate = new FuncDataTemplate<object>((vm, _) =>
            {
                if (vm is PeerTab pt) return pt.View;
                if (vm is NewPeerTabMarker)
                {
                    return new TextBlock
                    {
                        Text = "(click + to add a peer)",
                        HorizontalAlignment = HorizontalAlignment.Center,
                        VerticalAlignment = VerticalAlignment.Center,
                        Opacity = 0.5,
                    };
                }
                return new TextBlock();
            }, supportsRecycling: false),
        };

        RefreshTabsFromBridge(systemPeer, focusDefault: true);

        // Selection change → if user clicked "+", open the new-peer
        // dialog. Otherwise focus the selected tab's input.
        _tabs.SelectionChanged += OnTabSelectionChanged;

        // Window layout: bridge status (top), diag bar (bottom), tabs (center).
        // diagBar is built ONCE and reused — calling BuildDiagBar twice
        // would try to re-parent the shared _diagStatus field (visual-
        // parent invariant violation; crashes in DockPanel.Children.Add).
        var diagBar = BuildDiagBar();
        var root = new DockPanel { LastChildFill = true };
        DockPanel.SetDock(_bridgeStatus, Dock.Top);
        DockPanel.SetDock(diagBar, Dock.Bottom);
        root.Children.Add(_bridgeStatus);
        root.Children.Add(diagBar);
        root.Children.Add(_tabs);
        Content = root;

        // Set the bridge status to system-peer summary.
        RefreshBridgeStatus(systemPeer);

        Closing += (_, _) => Bridge.Shutdown();

        // Smoke mode: WB_SMOKE_EXIT_AFTER_SEC=N makes the window
        // auto-close after N seconds. Used by `make smoke-xvfb` to
        // run the real X11 paint path inside a virtual framebuffer
        // and assert the app boots + renders cleanly without
        // requiring a human at the keyboard. The wrapper script
        // grabs a screenshot before the timer fires.
        var smokeSec = Environment.GetEnvironmentVariable("WB_SMOKE_EXIT_AFTER_SEC");
        if (int.TryParse(smokeSec, out var sec) && sec > 0)
        {
            Console.Error.WriteLine($"==> smoke mode: window will auto-close after {sec}s");
            var timer = new DispatcherTimer { Interval = TimeSpan.FromSeconds(sec) };
            timer.Tick += (_, _) =>
            {
                timer.Stop();
                Console.Error.WriteLine("==> smoke mode: timer fired, closing window");
                Close();
            };
            timer.Start();
        }

        // Smoke driver: if WB_SMOKE_INGEST is set, drive the active
        // PeerView programmatically (ingest a corpus, swap a slot to
        // markdown-view, cycle through ingested paths). The driver
        // exists to reproduce the rapid-click-large-doc scenario the
        // adaptive renderer was built for, under real X11. See
        // SmokeDriver.cs + PHASE-I-RELIABILITY-PLAN.md.
        if (_peerTabs.Count > 0)
        {
            var driveTimer = new DispatcherTimer { Interval = TimeSpan.FromMilliseconds(500) };
            driveTimer.Tick += (_, _) =>
            {
                driveTimer.Stop();
                SmokeDriver.MaybeStart(this, _peerTabs[0].View);
            };
            driveTimer.Start();
        }
    }

    private Control BuildDiagBar()
    {
        var helloBtn = new Button { Content = "ping bridge", FontSize = 12 };
        helloBtn.Click += (_, _) => _diagStatus.Text = Bridge.TakeString(Bridge.Hello());
        var diagBar = new Grid
        {
            ColumnDefinitions = new ColumnDefinitions("Auto,*"),
            Margin = new Thickness(12, 4),
        };
        Grid.SetColumn(helloBtn, 0);
        Grid.SetColumn(_diagStatus, 1);
        _diagStatus.Margin = new Thickness(8, 0, 0, 0);
        _diagStatus.VerticalAlignment = VerticalAlignment.Center;
        diagBar.Children.Add(helloBtn);
        diagBar.Children.Add(_diagStatus);
        return diagBar;
    }

    // BuildTabHeader builds the chrome shown in the tab strip — alias
    // text + a context menu with "Close peer." System peer can't be
    // closed (UI signals this by disabling the menu item).
    private Control BuildTabHeader(PeerTab pt)
    {
        var label = new TextBlock
        {
            Text = pt.View.Alias.Length > 0 ? pt.View.Alias : "(loading…)",
            FontSize = 13,
            Padding = new Thickness(4, 2),
        };
        // Decorate system peer with a marker so the user can tell
        // which tab owns the roster.
        if (pt.IsSystemPeer)
        {
            label.Text = "* " + label.Text;
            label.FontWeight = FontWeight.SemiBold;
        }

        var menu = new ContextMenu();
        var closeItem = new MenuItem { Header = "Close peer" };
        closeItem.IsEnabled = !pt.IsSystemPeer;
        closeItem.Click += (_, _) => ClosePeerTab(pt);
        menu.Items.Add(closeItem);
        label.ContextMenu = menu;

        return label;
    }

    private void OnTabSelectionChanged(object? sender, SelectionChangedEventArgs e)
    {
        if (_tabs.SelectedItem is NewPeerTabMarker)
        {
            // Snap back to the previously-selected real tab BEFORE
            // opening the dialog — if the user cancels, focus stays on
            // a peer tab rather than the marker.
            var prev = _peerTabs.LastOrDefault();
            if (prev != null) _tabs.SelectedItem = prev;
            _ = OpenNewPeerDialogAsync();
            return;
        }
        if (_tabs.SelectedItem is PeerTab pt)
        {
            pt.View.FocusInput();
        }
    }

    private async System.Threading.Tasks.Task OpenNewPeerDialogAsync()
    {
        var dlg = new NewPeerDialog();
        await dlg.ShowDialog(this);
        if (dlg.CreatedPeerHandle != 0)
        {
            AddTabForPeer(dlg.CreatedPeerHandle, Bridge.DefaultPeer(), isSystem: false, focus: true);
            RefreshBridgeStatus(Bridge.DefaultPeer());
        }
    }

    private void ClosePeerTab(PeerTab pt)
    {
        if (pt.IsSystemPeer) return;
        // Drop the tab first so the UI doesn't try to render through a
        // peer that's about to disappear, then destroy + dispose.
        var idx = _tabItems.IndexOf(pt);
        _tabItems.Remove(pt);
        _peerTabs.Remove(pt);
        Bridge.TakeString(Bridge.PeerDestroy(pt.PeerHandle));
        pt.View.Dispose();
        // Pick a sane new selection — the tab to the left if any, else
        // the first remaining peer tab.
        if (_peerTabs.Count > 0)
        {
            var newIdx = Math.Max(0, Math.Min(idx, _peerTabs.Count - 1));
            _tabs.SelectedItem = _peerTabs[newIdx];
        }
    }

    // RefreshTabsFromBridge enumerates Bridge.PeerList and constructs a
    // tab for each live peer. Idempotent — clears + rebuilds. Used on
    // first open + after BridgeRestorePeers.
    private void RefreshTabsFromBridge(long systemPeer, bool focusDefault)
    {
        _tabItems.Clear();
        _peerTabs.Clear();

        var listReply = Bridge.TakeString(Bridge.PeerList());
        PeerListDto? dto = null;
        try { dto = JsonSerializer.Deserialize<PeerListDto>(listReply); } catch { }
        if (dto?.Peers == null)
        {
            // No peers — just an empty + tab. Shouldn't happen if Init
            // succeeded, but defensive.
            _tabItems.Add(new NewPeerTabMarker());
            return;
        }

        // Stable order: system peer first, then by added_at ascending.
        var ordered = dto.Peers
            .OrderByDescending(p => p.IsSystem)
            .ThenBy(p => p.AddedAt)
            .ToList();

        foreach (var p in ordered)
        {
            AddTabForPeer(p.Handle, systemPeer, p.IsSystem, focus: false);
        }
        _tabItems.Add(new NewPeerTabMarker());

        if (focusDefault && _peerTabs.Count > 0)
        {
            _tabs.SelectedItem = _peerTabs[0];
        }
    }

    private void AddTabForPeer(long peerHandle, long systemPeer, bool isSystem, bool focus)
    {
        var view = new PeerView(peerHandle, systemPeer);
        var pt = new PeerTab(peerHandle, view, isSystem);
        _peerTabs.Add(pt);
        // Insert before the trailing "+" marker if present, else append.
        var markerIdx = -1;
        for (int i = 0; i < _tabItems.Count; i++)
        {
            if (_tabItems[i] is NewPeerTabMarker) { markerIdx = i; break; }
        }
        if (markerIdx >= 0) _tabItems.Insert(markerIdx, pt);
        else _tabItems.Add(pt);
        if (focus) _tabs.SelectedItem = pt;
    }

    private void RefreshBridgeStatus(long systemPeer)
    {
        var listReply = Bridge.TakeString(Bridge.PeerList());
        PeerListDto? dto = null;
        try { dto = JsonSerializer.Deserialize<PeerListDto>(listReply); } catch { }
        int n = dto?.Peers?.Count ?? 0;
        _bridgeStatus.Text = $"bridge alive — {n} peer{(n == 1 ? "" : "s")} hosted · system handle {systemPeer}";
        _bridgeStatus.Opacity = 0.75;
        _bridgeStatus.ClearValue(TextBlock.ForegroundProperty);
    }

    // PeerTab pairs a PeerView with the bookkeeping the tab strip
    // needs (handle + system-peer flag). Acts as the TabControl item.
    private sealed class PeerTab
    {
        public long PeerHandle { get; }
        public PeerView View { get; }
        public bool IsSystemPeer { get; }
        public PeerTab(long handle, PeerView view, bool isSystem)
        {
            PeerHandle = handle; View = view; IsSystemPeer = isSystem;
        }
    }

    // DTOs for PeerList envelope.
    private sealed class PeerListDto
    {
        [JsonPropertyName("ok")] public bool Ok { get; set; }
        [JsonPropertyName("peers")] public List<PeerInfoDto>? Peers { get; set; }
    }

    private sealed class PeerInfoDto
    {
        [JsonPropertyName("handle")] public long Handle { get; set; }
        [JsonPropertyName("peer_id")] public string PeerId { get; set; } = "";
        [JsonPropertyName("alias")] public string Alias { get; set; } = "";
        [JsonPropertyName("identity")] public string Identity { get; set; } = "";
        [JsonPropertyName("storage_kind")] public string StorageKind { get; set; } = "";
        [JsonPropertyName("listen")] public string Listen { get; set; } = "";
        [JsonPropertyName("is_system")] public bool IsSystem { get; set; }
        [JsonPropertyName("connections")] public int Connections { get; set; }
        [JsonPropertyName("added_at")] public long AddedAt { get; set; }
    }
}
