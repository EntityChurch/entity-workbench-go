using System;
using System.Runtime.InteropServices;
using System.Text.Json;
using System.Threading.Tasks;
using Avalonia.Controls;
using Avalonia.Headless.XUnit;
using EntityAvalonia;
using EntityAvalonia.Panels;
using Xunit;
using Xunit.Abstractions;

namespace EntityAvalonia.Tests;

// "Click around like a user" stress tests. These are the tests we
// should have had from the start. Earlier headless tests probed
// individual surfaces (one panel, one event, one stress dimension)
// and missed the cross-panel-state-machine crashes the user kept
// hitting in real use.
//
// Each test below emulates one slice of the user's actual flow at
// high cadence: hammer tree selection, hammer slot swaps, hammer
// the full panel cascade (tree + markdown-view + markdown-files
// all subscribed to the same host).
//
// If a test in this file crashes, we've reproduced the user-visible
// segfault deterministically and can iterate against it without
// any clicking around in the real app. If they all pass clean and
// real use still crashes, the bug is in the Skia paint layer
// (UseHeadlessDrawing=true skips paint) and we need a different
// mitigation strategy.
[Collection(nameof(BridgeCollection))]
public sealed class ClickAroundStressTests
{
    private readonly BridgeFixture _bridge;
    private readonly ITestOutputHelper _output;

    public ClickAroundStressTests(BridgeFixture bridge, ITestOutputHelper output)
    {
        _bridge = bridge;
        _output = output;
    }

    [AvaloniaFact]
    public async Task Cascade_Of_All_Panels_Survives_500_Rapid_Selections()
    {
        // Construct the panel set the user actually has open in the
        // app: tree (publishes selection) + markdown-view + markdown-
        // files + detail (all subscribed via IPanelHost). Same wiring
        // as PeerView. This is the cascade that has been crashing.
        var host = new TestHost();
        var tree = new TreeViewPanel(_bridge.DefaultPeer);
        var mdView = new MarkdownViewPanel(_bridge.DefaultPeer, host);
        var mdFiles = new MarkdownFilesPanel(_bridge.DefaultPeer, host);
        var detail = new DetailPanel(_bridge.DefaultPeer, host);

        tree.EntitySelected += p => host.Publish(p);

        var w1 = new Window { Content = tree, Width = 400, Height = 600 }; w1.Show();
        var w2 = new Window { Content = mdView, Width = 600, Height = 600 }; w2.Show();
        var w3 = new Window { Content = mdFiles, Width = 400, Height = 400 }; w3.Show();
        var w4 = new Window { Content = detail, Width = 400, Height = 400 }; w4.Show();

        // Seed 20 entities so we have varied paths to alternate.
        for (int i = 0; i < 20; i++)
        {
            PutData($"stress_cascade_{i}", $"content {i}");
        }
        HeadlessPump.Flush();

        // 500 rapid publishes cycling through the seeded paths.
        // Each publish triggers MarkdownView.LoadPath (debounced),
        // Detail.Refresh (NOT debounced), MarkdownFiles re-render.
        for (int i = 0; i < 500; i++)
        {
            host.Publish($"/somepath/stress_cascade_{i % 20}");
            HeadlessPump.Flush();
        }

        // Survived. If we got here, the cascade is OK at this cadence.
        _output.WriteLine("500 cascade publishes complete, no crash");
    }

    [AvaloniaFact]
    public async Task PanelSlot_Surviving_100_Rapid_Swaps()
    {
        // PanelSlot.SwitchTo disposes the prior panel and constructs
        // a new one. This is the path that triggers every bridge-handle
        // Open/Close cycle in succession. If any panel's lifecycle is
        // racy, slot swaps will surface it.
        var host = new TestHost();
        var slot = new PanelSlot(_bridge.DefaultPeer, host, "detail");
        var w = new Window { Content = slot, Width = 600, Height = 600 };
        w.Show();
        HeadlessPump.Flush();

        var panels = new[]
        {
            "detail",
            "peer-info",
            "log-viewer",
            "markdown-view",
            "markdown-files",
            "query-browser",
        };

        for (int i = 0; i < 100; i++)
        {
            slot.SwitchTo(panels[i % panels.Length]);
            // Drop a selection event so the just-mounted panel takes
            // a render trip through its bridge.
            host.Publish($"/swap_test/{i}");
            HeadlessPump.Flush();
        }

        _output.WriteLine("100 slot swaps with selection events complete, no crash");
    }

    [AvaloniaFact]
    public async Task Two_Slots_Both_Swapping_And_Publishing_Survives()
    {
        // Two slots side-by-side both swapping panel kinds while the
        // tree publishes selection events. Closest reproduction of
        // the user's actual flow: PeerView holds two PanelSlots, tree
        // fires EntitySelected → both slots' subscribed panels react.
        var host = new TestHost();
        var slotA = new PanelSlot(_bridge.DefaultPeer, host, "detail");
        var slotB = new PanelSlot(_bridge.DefaultPeer, host, "markdown-view");
        var wA = new Window { Content = slotA, Width = 600, Height = 400 }; wA.Show();
        var wB = new Window { Content = slotB, Width = 600, Height = 400 }; wB.Show();

        for (int i = 0; i < 25; i++)
        {
            PutData($"two_slot_{i}", $"body {i}");
        }
        HeadlessPump.Flush();

        var panels = new[] { "detail", "markdown-view", "markdown-files", "peer-info" };
        for (int i = 0; i < 200; i++)
        {
            if (i % 7 == 0) slotA.SwitchTo(panels[(i / 7) % panels.Length]);
            if (i % 11 == 0) slotB.SwitchTo(panels[(i / 11) % panels.Length]);
            host.Publish($"/two_slot/{i % 25}");
            HeadlessPump.Flush();
        }

        _output.WriteLine("200 mixed events complete, no crash");
    }

    [AvaloniaFact]
    public async Task Construct_And_Dispose_All_Panel_Kinds_200_Times()
    {
        // Panel lifecycle stress — construct + dispose every panel
        // kind 200 times each. If any panel's Dispose path leaks state
        // or has a use-after-free, this will catch it. Same shape
        // as MarkdownViewPanelStressTests.Repeated_Open_Close but
        // across every panel kind we ship.
        var host = new TestHost();
        var kinds = new (string Name, Func<Control> Make)[]
        {
            ("tree",          () => new TreeViewPanel(_bridge.DefaultPeer)),
            ("markdown-view", () => new MarkdownViewPanel(_bridge.DefaultPeer, host)),
            ("markdown-files", () => new MarkdownFilesPanel(_bridge.DefaultPeer, host)),
            ("detail",        () => new DetailPanel(_bridge.DefaultPeer, host)),
            ("peer-info",     () => new PeerInfoPanel(_bridge.DefaultPeer)),
            ("log-viewer",    () => new LogViewerPanel(_bridge.DefaultPeer)),
        };

        for (int i = 0; i < 200; i++)
        {
            var (name, mk) = kinds[i % kinds.Length];
            var panel = mk();
            var w = new Window { Content = panel, Width = 400, Height = 400 };
            w.Show();
            host.Publish($"/lifecycle/{i}");
            HeadlessPump.Flush();
            if (panel is IDisposable d) d.Dispose();
            w.Close();
        }

        _output.WriteLine("200 panel lifecycle iterations complete, no crash");
    }

    // --- helpers ---

    private sealed class TestHost : IPanelHost
    {
        public event Action<string>? SelectedPath;
        public string? CurrentSelectedPath { get; private set; }
        public void Publish(string path)
        {
            CurrentSelectedPath = path;
            SelectedPath?.Invoke(path);
        }
        public void PublishSelectedPath(string path) => Publish(path);
        public void RequestPeerStatusRefresh() { }
    }

    private void PutData(string path, string value)
    {
        var jsonArg = JsonSerializer.Serialize(value);
        var line = $"put {path} data {jsonArg}";
        var replyPtr = Bridge.DispatchLine(_bridge.DefaultPeer, line);
        var reply = Marshal.PtrToStringAnsi(replyPtr) ?? "(null)";
        Bridge.FreeString(replyPtr);
        using var doc = JsonDocument.Parse(reply);
        if (!doc.RootElement.GetProperty("ok").GetBoolean())
        {
            throw new InvalidOperationException($"put failed: {reply}");
        }
    }
}
