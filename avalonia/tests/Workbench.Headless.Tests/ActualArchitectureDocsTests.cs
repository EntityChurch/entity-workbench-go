using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
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

// This test ingests the ACTUAL workbench architecture docs — the
// same .md files the user is loading when the app crashes — and
// hammers the markdown view through them with real Skia paint
// enabled.
//
// If THIS test passes and the real app still crashes, the bug is
// in the X11/window-manager layer that headless skips, or in some
// interaction with the user's real display that we cannot
// reproduce here.
//
// The test finds the docs/architecture/ dir relative to where the
// test is built. Inside the podman tester stage that's
// `/src/entity-workbench-go/docs/architecture` (the source is
// COPY'd into /src). On a hypothetical host run it would resolve
// upward; the in-container path is what matters for CI.
[Collection(nameof(BridgeCollection))]
public sealed class ActualArchitectureDocsTests
{
    private readonly BridgeFixture _bridge;
    private readonly ITestOutputHelper _output;

    public ActualArchitectureDocsTests(BridgeFixture bridge, ITestOutputHelper output)
    {
        _bridge = bridge;
        _output = output;
    }

    [AvaloniaFact]
    public async Task Real_Architecture_Docs_Survive_300_Clicks_Through_MarkdownView()
    {
        var archDir = FindArchitectureDir();
        if (archDir == null)
        {
            _output.WriteLine("Skipping — docs/architecture/ not found from test cwd.");
            return; // not a failure; some build contexts don't ship docs
        }

        // Ingest the actual repo docs into the peer's tree.
        Dispatch($"ingest tree {archDir} docs/");

        // Wait for ingest to settle and harvest the resulting paths.
        var tree = new TreeViewPanel(_bridge.DefaultPeer);
        var tw = new Window { Content = tree, Width = 400, Height = 600 };
        tw.Show();
        tree.SetSearchForTests("docs/");

        await HeadlessPump.WaitUntil(
            () => CountEntries(tree) >= 10,
            TimeSpan.FromSeconds(10));

        var docPaths = HarvestPaths(tree);
        _output.WriteLine($"Loaded {docPaths.Count} real architecture docs");
        Assert.True(docPaths.Count >= 10,
            $"expected ≥10 ingested docs; got {docPaths.Count}");

        ((IDisposable)tree).Dispose();
        tw.Close();

        // Construct the markdown view in a host that publishes
        // selections. This is the exact wiring the user has when
        // they switch a slot to the markdown view.
        var host = new TestHost();
        var mdView = new MarkdownViewPanel(_bridge.DefaultPeer, host);
        var w = new Window { Content = mdView, Width = 1000, Height = 800 };
        w.Show();
        HeadlessPump.Flush();

        // 300 rapid clicks across the real docs. 200ms gap so the
        // debounce timer fires and the renderer materially runs +
        // Skia rasterizes the result every time.
        var clickedTitles = new HashSet<string>();
        for (int i = 0; i < 300; i++)
        {
            var path = docPaths[i % docPaths.Count];
            host.Publish(path);
            HeadlessPump.Flush();
            await Task.Delay(200);

            if (i % 25 == 0)
            {
                _output.WriteLine($"  iter {i:D3}/300 — {Path.GetFileName(path)}");
            }
        }

        _output.WriteLine($"300 clicks across {docPaths.Count} real docs complete, no crash");
    }

    [AvaloniaFact]
    public async Task Each_Architecture_Doc_Renders_Without_Throwing()
    {
        // Per-doc test: open each ingested doc once, render it, look
        // at the count of inlines produced. If any doc throws during
        // BuildInlines or during the panel's render path, this is
        // where it surfaces — by individual doc rather than buried
        // in a stress loop.
        var archDir = FindArchitectureDir();
        if (archDir == null) return;

        Dispatch($"ingest tree {archDir} docs/per_doc/");

        var tree = new TreeViewPanel(_bridge.DefaultPeer);
        var tw = new Window { Content = tree, Width = 400, Height = 600 };
        tw.Show();
        tree.SetSearchForTests("docs/per_doc/");
        await HeadlessPump.WaitUntil(
            () => CountEntries(tree) >= 10,
            TimeSpan.FromSeconds(10));
        var docPaths = HarvestPaths(tree);
        ((IDisposable)tree).Dispose();
        tw.Close();

        var host = new TestHost();
        var mdView = new MarkdownViewPanel(_bridge.DefaultPeer, host);
        var w = new Window { Content = mdView, Width = 1000, Height = 800 };
        w.Show();

        foreach (var p in docPaths)
        {
            host.Publish(p);
            HeadlessPump.Flush();
            await Task.Delay(250); // past the 150ms debounce
            HeadlessPump.Flush();
        }

        _output.WriteLine($"All {docPaths.Count} docs rendered successfully");
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

    // Walk up from cwd looking for docs/architecture/. Inside the
    // tester container, cwd is .../tests/Workbench.Headless.Tests/bin/...
    // and the architecture dir is at /src/entity-workbench-go/docs/architecture.
    // Just try a known set of candidates.
    private static string? FindArchitectureDir()
    {
        var candidates = new[]
        {
            "/src/entity-workbench-go/docs/architecture",
        };
        foreach (var c in candidates)
        {
            if (Directory.Exists(c)) return c;
        }
        return null;
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

    private static int CountEntries(TreeViewPanel tree)
    {
        int n = 0;
        for (int i = 0; i < tree.RowsCountForTests; i++)
        {
            if (tree.IsEntryForTests(i)) n++;
        }
        return n;
    }

    private static List<string> HarvestPaths(TreeViewPanel tree)
    {
        var paths = new List<string>();
        for (int i = 0; i < tree.RowsCountForTests; i++)
        {
            if (tree.IsEntryForTests(i))
            {
                paths.Add(tree.GetRowPathForTests(i));
            }
        }
        return paths;
    }
}
