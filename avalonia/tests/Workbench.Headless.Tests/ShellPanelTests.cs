using System;
using System.Threading.Tasks;
using Avalonia.Controls;
using Avalonia.Headless.XUnit;
using EntityAvalonia.Panels;
using Xunit;

namespace EntityAvalonia.Tests;

// Tier-3 coverage for ShellPanel (PHASE-I-DESKTOP-RENDERER-PLAN §I.5
// wave 2 — per-panel shell). Each panel owns its own
// shellcmd.Shell handle via Bridge.ShellOpen; close on Dispose; two
// panels in the same process produce two independent handles.
//
// Contracts pinned:
//   1. Mount opens a valid shell handle (>= 0).
//   2. Dispatching a no-error command produces scrollback output AND
//      asks the host to refresh peer status (per the §I.5 contract
//      that `connect`/`disconnect`/`cd`/identity mutate shared peer
//      state).
//   3. Two ShellPanels in parallel have DISTINCT handles — the bridge
//      is per-panel, not per-peer.
//   4. Open-close cycle doesn't leak handles or hang.
[Collection(nameof(BridgeCollection))]
public sealed class ShellPanelTests
{
    private readonly BridgeFixture _bridge;

    public ShellPanelTests(BridgeFixture bridge)
    {
        _bridge = bridge;
    }

    [AvaloniaFact]
    public void Mount_Opens_Valid_Shell_Handle()
    {
        var host = new TestHost();
        using var panel = new ShellPanel(_bridge.DefaultPeer, host);
        Assert.True(
            panel.HandleForTests >= 0,
            $"expected non-negative shell handle on mount; got {panel.HandleForTests}");
    }

    [AvaloniaFact]
    public void Dispatch_Produces_Scrollback_And_Refreshes_Peer_Status()
    {
        var host = new TestHost();
        using var panel = new ShellPanel(_bridge.DefaultPeer, host);
        var window = new Window { Content = panel, Width = 600, Height = 400 };
        window.Show();

        var baseline = panel.ScrollbackRowCountForTests;
        var baselineRefreshes = host.PeerStatusRefreshCount;

        // `pwd` is a no-side-effects command that produces a single
        // path line in the scrollback. The command-echo line (prompt
        // + line) lands too, so we expect ≥2 new rows.
        panel.SubmitLineForTests("pwd");
        HeadlessPump.Flush();

        Assert.True(
            panel.ScrollbackRowCountForTests >= baseline + 1,
            $"expected scrollback to grow after dispatch; was {baseline}, " +
            $"now {panel.ScrollbackRowCountForTests}");

        // Every dispatch must ask the host to refresh peer status —
        // even pwd, because the contract is "we don't know what the
        // command did so refresh anyway." Cheap; the host de-dupes
        // if it wants.
        Assert.True(
            host.PeerStatusRefreshCount > baselineRefreshes,
            "expected RequestPeerStatusRefresh to fire on dispatch");
    }

    [AvaloniaFact]
    public void Two_Panels_Have_Distinct_Handles()
    {
        var host = new TestHost();
        using var a = new ShellPanel(_bridge.DefaultPeer, host);
        using var b = new ShellPanel(_bridge.DefaultPeer, host);

        Assert.True(a.HandleForTests >= 0);
        Assert.True(b.HandleForTests >= 0);
        Assert.NotEqual(a.HandleForTests, b.HandleForTests);
    }

    [AvaloniaFact]
    public void Repeated_Open_Close_Does_Not_Hang_Or_Crash()
    {
        // Same shape as MarkdownViewPanelStressTests' open-close cycle
        // test — surfaces a handle leak or dispose-race in the bridge.
        // 50 mount/dispose cycles in a tight loop; if any cycle leaks,
        // the next ShellOpen returns -1 (no-handle path) and the
        // assertion fails.
        var host = new TestHost();
        const int iterations = 50;
        for (int i = 0; i < iterations; i++)
        {
            var panel = new ShellPanel(_bridge.DefaultPeer, host);
            Assert.True(panel.HandleForTests >= 0,
                $"iter {i}: shell handle was {panel.HandleForTests}");
            panel.Dispose();
        }
    }

    // TestHost counts RequestPeerStatusRefresh calls so the dispatch
    // test can verify the contract holds.
    private sealed class TestHost : IPanelHost
    {
        public event Action<string>? SelectedPath;
        public string? CurrentSelectedPath { get; private set; }
        public int PeerStatusRefreshCount { get; private set; }

        public void PublishSelectedPath(string path)
        {
            CurrentSelectedPath = path;
            SelectedPath?.Invoke(path);
        }
        public void RequestPeerStatusRefresh() => PeerStatusRefreshCount++;
    }
}
