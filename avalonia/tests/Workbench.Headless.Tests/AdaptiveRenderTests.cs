using System;
using System.IO;
using System.Runtime.InteropServices;
using System.Text;
using System.Text.Json;
using System.Threading.Tasks;
using Avalonia.Controls;
using Avalonia.Headless.XUnit;
using EntityAvalonia.Panels;
using Xunit;
using Xunit.Abstractions;

namespace EntityAvalonia.Tests;

// AdaptiveRenderTests pin the three layers of the MarkdownViewPanel
// render pipeline (see SwapBody comment block):
//
//   1. Small doc            → single batch, single block, rich render
//   2. Big doc              → multiple batches AND multiple blocks
//   3. Doc above ceiling    → plain-text fallback path
//
// Tests drive the panel exactly the way the user does: ingest real
// markdown files into the peer, then publish their tree paths to the
// panel via IPanelHost. Render state is observed via the panel's
// internal test surfaces (LastRenderBatch/Block counters + fallback
// flag).
[Collection(nameof(BridgeCollection))]
public sealed class AdaptiveRenderTests
{
    private readonly BridgeFixture _bridge;
    private readonly ITestOutputHelper _output;

    public AdaptiveRenderTests(BridgeFixture bridge, ITestOutputHelper output)
    {
        _bridge = bridge;
        _output = output;
    }

    [AvaloniaFact]
    public async Task Small_Doc_Renders_In_One_Batch_One_Block()
    {
        var dir = MakeTempDir("adaptive-small");
        File.WriteAllText(Path.Combine(dir, "small.md"),
            "# small\n\nHello world.\n\nA paragraph.\n");
        Dispatch($"ingest tree {dir} adaptive_small/");

        var (panel, window, host) = NewPanel();
        try
        {
            var path = await WaitForFirstPath(panel, host, "adaptive_small/");
            host.Publish(path);
            await WaitForRenderComplete(panel, TimeSpan.FromSeconds(5));

            _output.WriteLine(
                $"small: batches={panel.LastRenderBatchCountForTests} " +
                $"blocks={panel.LastRenderBlockCountForTests} " +
                $"fallback={panel.LastRenderUsedPlainTextFallbackForTests}");

            Assert.False(panel.LastRenderUsedPlainTextFallbackForTests);
            Assert.Equal(1, panel.LastRenderBatchCountForTests);
            Assert.Equal(1, panel.LastRenderBlockCountForTests);
        }
        finally
        {
            ((IDisposable)panel).Dispose();
            window.Close();
        }
    }

    [AvaloniaFact]
    public async Task Big_Doc_Emits_In_Multiple_Batches_And_Splits_Across_Blocks()
    {
        // ~600 sections × 4 lines/each ≈ 2500+ inlines, well past
        // MaxInlinesPerBlock (500). Should trigger BOTH multi-batch
        // emit AND multi-block split. Stays well under the 1MB
        // fallback ceiling.
        var dir = MakeTempDir("adaptive-big");
        var sb = new StringBuilder();
        for (int i = 0; i < 600; i++)
        {
            sb.Append("## Section ").Append(i).Append('\n').Append('\n');
            sb.Append("Line one with some **bold** and *italic*.\n");
            sb.Append("Line two with `inline code` and [a link](https://example.com).\n");
            sb.Append("Line three plain text content.\n\n");
        }
        File.WriteAllText(Path.Combine(dir, "big.md"), sb.ToString());
        _output.WriteLine($"big.md size: {sb.Length} bytes");
        Dispatch($"ingest tree {dir} adaptive_big/");

        var (panel, window, host) = NewPanel();
        try
        {
            var path = await WaitForFirstPath(panel, host, "adaptive_big/");
            host.Publish(path);
            await WaitForRenderComplete(panel, TimeSpan.FromSeconds(15));

            _output.WriteLine(
                $"big: batches={panel.LastRenderBatchCountForTests} " +
                $"blocks={panel.LastRenderBlockCountForTests} " +
                $"fallback={panel.LastRenderUsedPlainTextFallbackForTests}");

            Assert.False(panel.LastRenderUsedPlainTextFallbackForTests);
            Assert.True(panel.LastRenderBatchCountForTests > 1,
                $"expected multiple batches; got {panel.LastRenderBatchCountForTests}");
            Assert.True(panel.LastRenderBlockCountForTests > 1,
                $"expected multiple SelectableTextBlock chunks; got {panel.LastRenderBlockCountForTests}");
        }
        finally
        {
            ((IDisposable)panel).Dispose();
            window.Close();
        }
    }

    [AvaloniaFact]
    public async Task Doc_Above_Ceiling_Falls_Back_To_Plain_Text()
    {
        // Build a >1MB markdown payload. We don't try to make it
        // pretty — the path under test is the pre-parse guard, which
        // only looks at byte count.
        var dir = MakeTempDir("adaptive-huge");
        var sb = new StringBuilder(capacity: 2 * 1024 * 1024);
        const string line = "This is a line of content. Many of them. Bytes accumulate without bound.\n";
        while (sb.Length < 1_100_000)
        {
            sb.Append(line);
        }
        File.WriteAllText(Path.Combine(dir, "huge.md"), sb.ToString());
        _output.WriteLine($"huge.md size: {sb.Length} bytes");
        Dispatch($"ingest tree {dir} adaptive_huge/");

        var (panel, window, host) = NewPanel();
        try
        {
            var path = await WaitForFirstPath(panel, host, "adaptive_huge/");
            host.Publish(path);
            // Fallback completes synchronously inside SwapBody — no
            // dispatcher round-trips. Just pump once.
            await Task.Delay(250); // past debounce
            HeadlessPump.Flush();
            await HeadlessPump.WaitUntil(
                () => panel.LastRenderUsedPlainTextFallbackForTests,
                TimeSpan.FromSeconds(5));

            _output.WriteLine(
                $"huge: fallback={panel.LastRenderUsedPlainTextFallbackForTests}");

            Assert.True(panel.LastRenderUsedPlainTextFallbackForTests,
                "expected plain-text fallback for >1MB document");
        }
        finally
        {
            ((IDisposable)panel).Dispose();
            window.Close();
        }
    }

    // --- helpers -----------------------------------------------------

    private (MarkdownViewPanel panel, Window window, TestHost host) NewPanel()
    {
        var host = new TestHost();
        var panel = new MarkdownViewPanel(_bridge.DefaultPeer, host);
        var window = new Window { Content = panel, Width = 1000, Height = 800 };
        window.Show();
        HeadlessPump.Flush();
        return (panel, window, host);
    }

    private async Task<string> WaitForFirstPath(
        MarkdownViewPanel panel, TestHost host, string prefix)
    {
        // After ingest the tree-side wake propagates async; harvest
        // the path by listing entries via the tree panel briefly.
        var tree = new TreeViewPanel(_bridge.DefaultPeer);
        var tw = new Window { Content = tree, Width = 400, Height = 600 };
        tw.Show();
        tree.SetSearchForTests(prefix);
        var ok = await HeadlessPump.WaitUntil(
            () => HarvestFirstPath(tree, prefix) != null,
            TimeSpan.FromSeconds(10));
        Assert.True(ok, $"no ingested path under {prefix} appeared in tree");
        var path = HarvestFirstPath(tree, prefix);
        ((IDisposable)tree).Dispose();
        tw.Close();
        Assert.NotNull(path);
        return path!;
    }

    private static string? HarvestFirstPath(TreeViewPanel tree, string prefix)
    {
        for (int i = 0; i < tree.RowsCountForTests; i++)
        {
            if (!tree.IsEntryForTests(i)) continue;
            var p = tree.GetRowPathForTests(i);
            if (p != null && p.Contains(prefix)) return p;
        }
        return null;
    }

    // Wait for the adaptive emit to finish. We poll: batch count
    // stops growing for two consecutive checks → render is done.
    private static async Task WaitForRenderComplete(
        MarkdownViewPanel panel, TimeSpan timeout)
    {
        var deadline = DateTime.UtcNow + timeout;
        int lastBatch = -1;
        int stable = 0;
        while (DateTime.UtcNow < deadline)
        {
            HeadlessPump.Flush();
            await Task.Delay(50);
            var batches = panel.LastRenderBatchCountForTests;
            if (panel.LastRenderUsedPlainTextFallbackForTests) return;
            if (batches > 0 && batches == lastBatch) { stable++; if (stable >= 2) return; }
            else stable = 0;
            lastBatch = batches;
        }
    }

    private void Dispatch(string line)
    {
        var replyPtr = Bridge.DispatchLine(_bridge.DefaultPeer, line);
        var reply = Marshal.PtrToStringAnsi(replyPtr) ?? "(null)";
        Bridge.FreeString(replyPtr);
        using var doc = JsonDocument.Parse(reply);
        if (!doc.RootElement.GetProperty("ok").GetBoolean())
        {
            throw new InvalidOperationException($"dispatch `{line}` failed: {reply}");
        }
    }

    private static string MakeTempDir(string label)
    {
        var dir = Path.Combine(Path.GetTempPath(), $"{label}-{Guid.NewGuid():N}");
        Directory.CreateDirectory(dir);
        return dir;
    }

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
}
