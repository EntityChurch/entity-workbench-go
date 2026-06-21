using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Threading.Tasks;
using Avalonia.Controls;
using Avalonia.Headless.XUnit;
using EntityAvalonia.Panels;
using Xunit;
using Xunit.Abstractions;

namespace EntityAvalonia.Tests;

// Diagnostic stress test for the "click 3 docs in a row → app falls
// over" symptom the user reproduced.
//
// Premise: the segfault is downstream of state accumulation. Each
// click hits MarkdownView.LoadPath, which triggers a bridge-side
// watch rebind + a wake → RerenderFromBridge → Markdown.Avalonia
// re-parse. Some layer in this chain (most likely Markdown.Avalonia
// or its parent visual tree under Skia) doesn't release prior
// state cleanly.
//
// What headless CAN catch:
//   - LoadPath round-trip time growing per iteration (state
//     accumulation on Go bridge or C# logic side)
//   - Bridge calls returning non-ok envelopes (handle exhaustion,
//     watch errors, panic)
//   - Hangs / deadlocks in the cancel-old-watch path
//
// What headless CANNOT catch:
//   - Skia paint segfaults: UseHeadlessDrawing=true stubs the
//     renderer; no paint happens. If this test passes clean but
//     the segfault still reproduces in real use, the root cause
//     is in the paint layer and we need a different mitigation
//     (debounce LoadPath, drop Markdown.Avalonia for a custom
//     renderer, etc.)
//
// The tests intentionally do NOT seed real markdown content — any
// path exercises the watch rebind + wake + render plumbing. NotFound
// is a valid render output. We're stress-testing the cycle, not the
// rendering of resolved content.
[Collection(nameof(BridgeCollection))]
public sealed class MarkdownViewPanelStressTests
{
    private readonly BridgeFixture _bridge;
    private readonly ITestOutputHelper _output;

    public MarkdownViewPanelStressTests(BridgeFixture bridge, ITestOutputHelper output)
    {
        _bridge = bridge;
        _output = output;
    }

    [AvaloniaFact]
    public async Task Rapid_LoadPath_Does_Not_Slow_Down_Or_Fail()
    {
        var host = new TestHost();
        var panel = new MarkdownViewPanel(_bridge.DefaultPeer, host);
        var window = new Window { Content = panel, Width = 600, Height = 600 };
        window.Show();

        var paths = new[] { "stress/a", "stress/b", "stress/c" };

        // Warm up — first LoadPath cost includes JIT + cold caches.
        host.Publish(paths[0]);
        await HeadlessPump.WaitUntil(() => false, TimeSpan.FromMilliseconds(100));

        const int iterations = 100;
        var times = new List<long>(iterations);
        var sw = new Stopwatch();
        for (int i = 0; i < iterations; i++)
        {
            sw.Restart();
            host.Publish(paths[i % paths.Length]);
            HeadlessPump.Flush();
            sw.Stop();
            times.Add(sw.ElapsedMilliseconds);
        }

        var firstHalf = times.GetRange(0, iterations / 2);
        var secondHalf = times.GetRange(iterations / 2, iterations / 2);
        var avgFirst = Avg(firstHalf);
        var avgSecond = Avg(secondHalf);

        _output.WriteLine(
            $"LoadPath timing — first half avg {avgFirst:F2}ms, " +
            $"second half avg {avgSecond:F2}ms, max {Max(times)}ms");

        Assert.True(avgSecond < Math.Max(avgFirst * 5, 100),
            $"LoadPath slowed down progressively. First-half avg {avgFirst:F2}ms, " +
            $"second-half avg {avgSecond:F2}ms. State is accumulating somewhere " +
            $"between iteration 1 and iteration {iterations}.");
    }

    [AvaloniaFact]
    public async Task Rapid_Publish_Debounces_To_Single_LoadPath()
    {
        // Regression test for the rapid-click crash + the
        // structural mitigation for open follow-on #4. The fix
        // debounces LoadPath behind a timer (currently 400ms, raised
        // from 150ms in the evening pivot — see
        // MODEL-AVALONIA-RUNTIME §4). Publishing 5 paths in rapid
        // succession should result in exactly 1 LoadPath crossing
        // the bridge — only the final one. This both prevents the
        // paint-layer crash AND avoids visible flicker through
        // intermediate content.
        var host = new TestHost();
        var panel = new MarkdownViewPanel(_bridge.DefaultPeer, host);
        var window = new Window { Content = panel, Width = 600, Height = 600 };
        window.Show();

        // Flush construction + initial render.
        HeadlessPump.Flush();
        var baseline = panel.LoadPathCallCountForTests;

        // Publish 5 distinct paths back-to-back (no async gap).
        for (int i = 0; i < 5; i++)
        {
            host.Publish($"rapid_publish/{i}");
        }

        // Wait past the debounce window plus margin. 400ms debounce +
        // 200ms margin — generous enough to survive heavy CI without
        // flaking, tight enough that a debounce regression to a
        // multi-second value would fail loudly.
        await HeadlessPump.WaitUntil(() => false, TimeSpan.FromMilliseconds(600));

        var delta = panel.LoadPathCallCountForTests - baseline;
        Assert.Equal(1, delta);
    }

    [AvaloniaFact]
    public void Repeated_Open_Close_Does_Not_Hang_Or_Crash()
    {
        // Construct + dispose markdown view panels in a tight loop
        // to surface bridge-handle leaks or close races.
        // MarkdownViewOpen allocates a handle in mdViews; Close must
        // remove it AND join the wake-forwarding goroutine. If Close
        // is racy with in-flight wakes, this test will hang or crash.
        var host = new TestHost();
        const int iterations = 50;
        for (int i = 0; i < iterations; i++)
        {
            var panel = new MarkdownViewPanel(_bridge.DefaultPeer, host);
            var window = new Window { Content = panel, Width = 400, Height = 400 };
            window.Show();
            host.Publish($"open_close_cycle/{i}");
            HeadlessPump.Flush();
            ((IDisposable)panel).Dispose();
            window.Close();
        }
        // Reaching here without hang/throw is the assertion.
    }

    // --- helpers -----------------------------------------------------

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

    private static double Avg(List<long> xs)
    {
        if (xs.Count == 0) return 0;
        long sum = 0;
        foreach (var x in xs) sum += x;
        return (double)sum / xs.Count;
    }

    private static long Max(List<long> xs)
    {
        long m = 0;
        foreach (var x in xs) if (x > m) m = x;
        return m;
    }
}
