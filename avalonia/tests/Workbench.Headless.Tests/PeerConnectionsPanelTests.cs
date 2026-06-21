using System;
using System.Threading.Tasks;
using Avalonia.Controls;
using Avalonia.Headless;
using Avalonia.Headless.XUnit;
using Avalonia.Threading;
using EntityAvalonia.Panels;
using Xunit;

namespace EntityAvalonia.Tests;

// Headless tests for PeerConnectionsPanel (PHASE-I-PEER-CONNECTIONS-PLAN
// B-1). Covers panel mount, input validation, scheme rejection, and
// graceful failure on unreachable addresses. Round-trip connect/
// disconnect against a paired listening peer is deferred until
// shellboot auto-Listen wiring lands (covers the B-5 path).
[Collection(nameof(BridgeCollection))]
public sealed class PeerConnectionsPanelTests
{
    private readonly BridgeFixture _bridge;

    public PeerConnectionsPanelTests(BridgeFixture bridge)
    {
        _bridge = bridge;
    }

    [AvaloniaFact]
    public async Task Mount_Shows_Local_Entry_Only()
    {
        var panel = new PeerConnectionsPanel(_bridge.DefaultPeer);
        var window = new Window { Content = panel, Width = 400, Height = 300 };
        window.Show();

        var deadline = DateTime.UtcNow.AddSeconds(3);
        while (panel.ConnectionCountForTests == 0 && DateTime.UtcNow < deadline)
        {
            AvaloniaHeadlessPlatform.ForceRenderTimerTick();
            Dispatcher.UIThread.RunJobs();
            await Task.Delay(10);
        }

        Assert.True(
            panel.HandleForTests > 0,
            $"connections handle not allocated (got {panel.HandleForTests})");
        Assert.True(
            panel.ConnectionCountForTests >= 1,
            $"expected at least the local entry; got {panel.ConnectionCountForTests}");
        // First entry is sorted-local-first per bridge buildConnectionsRender.
        Assert.True(
            panel.IsLocalAtForTests(0),
            $"first entry not marked local; alias={panel.AliasAtForTests(0)}");
    }

    [AvaloniaFact]
    public void Mount_Without_ListenAddr_Hides_Nearby_Section()
    {
        // BridgeFixture's default peer is constructed with Listen="" so
        // discovery is disabled; the panel should mark the discovery
        // handle as -1 and have zero nearby entries.
        var panel = new PeerConnectionsPanel(_bridge.DefaultPeer);
        var window = new Window { Content = panel, Width = 400, Height = 300 };
        window.Show();

        Assert.True(panel.DiscoveryHandleForTests < 0,
            $"expected discovery handle to be -1 (disabled); got {panel.DiscoveryHandleForTests}");
        Assert.Equal(0, panel.NearbyCountForTests);
    }

    [AvaloniaFact]
    public void Mount_Surfaces_Listen_Address_Or_NotListening()
    {
        var panel = new PeerConnectionsPanel(_bridge.DefaultPeer);
        var window = new Window { Content = panel, Width = 400, Height = 300 };
        window.Show();

        var line = panel.ListenLineForTests;
        Assert.False(string.IsNullOrEmpty(line), "listen line should be populated");
        // BridgeFixture's default peer has no ListenAddr in its boot
        // config, so the panel should report "Not listening". This also
        // protects against a regression where PeerListenAddr starts
        // returning a malformed envelope.
        Assert.Contains("Not listening", line, StringComparison.OrdinalIgnoreCase);
    }

    [AvaloniaFact]
    public async Task Connect_With_Empty_Address_Shows_Error_And_Does_Not_Crash()
    {
        var panel = new PeerConnectionsPanel(_bridge.DefaultPeer);
        var window = new Window { Content = panel, Width = 400, Height = 300 };
        window.Show();

        panel.SetAddressForTests("");
        panel.SetAliasForTests("remote");
        await panel.TriggerConnectForTests();

        Assert.Contains("address", panel.StatusTextForTests, StringComparison.OrdinalIgnoreCase);
    }

    [AvaloniaFact]
    public async Task Connect_With_Empty_Alias_Shows_Error()
    {
        var panel = new PeerConnectionsPanel(_bridge.DefaultPeer);
        var window = new Window { Content = panel, Width = 400, Height = 300 };
        window.Show();

        panel.SetAddressForTests("127.0.0.1:9999");
        panel.SetAliasForTests("");
        await panel.TriggerConnectForTests();

        Assert.Contains("alias", panel.StatusTextForTests, StringComparison.OrdinalIgnoreCase);
    }

    [AvaloniaFact]
    public async Task Connect_To_Unreachable_Address_Shows_Error_And_Survives()
    {
        var panel = new PeerConnectionsPanel(_bridge.DefaultPeer);
        var window = new Window { Content = panel, Width = 400, Height = 300 };
        window.Show();

        // 127.0.0.1:1 — well-known "nothing listening here" port; the
        // dial gets refused in milliseconds. DoConnectAsync dispatches
        // the bridge call on a thread-pool worker and awaits back to
        // the UI thread, so the status is populated by the time the
        // Task completes.
        panel.RerenderForTests();
        Assert.True(panel.ConnectionCountForTests >= 1, "local entry missing pre-dial");

        panel.SetAddressForTests("127.0.0.1:1");
        panel.SetAliasForTests("dead");
        await panel.TriggerConnectForTests();

        Assert.False(string.IsNullOrEmpty(panel.StatusTextForTests),
            "expected non-empty status after failed dial");
        // Local entry should still be present after the failed dial.
        Assert.True(panel.ConnectionCountForTests >= 1,
            "panel lost the local entry after a failed dial");
    }
}
