using System;
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

// Reproduces the user's actual flow:
//   1. `ingest tree <tmpdir> docs/` to load real markdown-file entities
//   2. Construct tree + markdown-view + slot
//   3. Click through the docs RAPIDLY until something breaks
//
// Previous ClickAroundStressTests used `put X data ...` which gives
// the bridge a path that resolves to a Data entity, and MarkdownView
// renders NotFound for those — so the renderer never built a real
// document. This test gets the renderer + real content into the
// stress loop. If the segfault lives in MarkdownRenderer / Avalonia
// layout of long Inline lists, this is the test that will catch it.
[Collection(nameof(BridgeCollection))]
public sealed class RealMarkdownClickAroundTests
{
    private readonly BridgeFixture _bridge;
    private readonly ITestOutputHelper _output;

    public RealMarkdownClickAroundTests(BridgeFixture bridge, ITestOutputHelper output)
    {
        _bridge = bridge;
        _output = output;
    }

    [AvaloniaFact]
    public async Task Ingested_Markdown_Files_Survive_300_Rapid_Clicks_Through_MarkdownView()
    {
        // Stage 1: spread real-shape markdown content on disk so
        // `ingest tree` has something to read.
        var tmpDir = Path.Combine(Path.GetTempPath(),
            $"wb_md_stress_{Guid.NewGuid():N}");
        Directory.CreateDirectory(tmpDir);
        try
        {
            for (int i = 0; i < 10; i++)
            {
                File.WriteAllText(Path.Combine(tmpDir, $"doc_{i:D2}.md"),
                    BuildRealMarkdownDoc(i));
            }

            // Stage 2: dispatch ingest so the peer's tree has
            // doc/markdown-file entities at docs/...
            Dispatch($"ingest tree {tmpDir} docs/");

            // Stage 3: harvest the actual tree paths for our docs.
            // Real tree paths are /<peer-id>/docs/doc_NN.md — we
            // discover them by listing the tree.
            var tree = new TreeViewPanel(_bridge.DefaultPeer);
            var tw = new Window { Content = tree, Width = 400, Height = 600 };
            tw.Show();
            tree.SetSearchForTests("docs/doc_");

            await HeadlessPump.WaitUntil(
                () => CountMatchingRows(tree) >= 10,
                TimeSpan.FromSeconds(5));

            var docPaths = HarvestMatchingPaths(tree);
            _output.WriteLine($"harvested {docPaths.Count} real doc paths");
            Assert.True(docPaths.Count >= 10,
                $"expected ≥10 ingested docs in tree, got {docPaths.Count}");

            // Stage 4: construct the markdown-view in a host that
            // forwards publishes. This is what mounts in the user's
            // slot. Drop the tree from the window — we don't need it
            // for the cascade.
            ((IDisposable)tree).Dispose();
            tw.Close();

            var host = new TestHost();
            var mdView = new MarkdownViewPanel(_bridge.DefaultPeer, host);
            var w = new Window { Content = mdView, Width = 800, Height = 600 };
            w.Show();
            HeadlessPump.Flush();

            // Stage 5: click around 300 times across the 10 docs.
            // The debounce timer collapses adjacent clicks, but
            // changing path + flushing forces the renderer to run.
            // We deliberately wait between clicks so the debounce
            // fires (otherwise we never exercise the render path).
            for (int i = 0; i < 300; i++)
            {
                var path = docPaths[i % docPaths.Count];
                host.Publish(path);
                HeadlessPump.Flush();
                // 200ms delay forces the debounce to fire and the
                // renderer to materially run on each click.
                await Task.Delay(200);
                if (i % 25 == 0)
                {
                    _output.WriteLine($"  iter {i}, current = {path}");
                }
            }

            _output.WriteLine("300 real-markdown clicks complete, no crash");
        }
        finally
        {
            try { Directory.Delete(tmpDir, recursive: true); } catch { }
        }
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

    // BuildRealMarkdownDoc produces a LARGE non-trivial markdown
    // document. The user's actual docs (architecture md files) are
    // hundreds of lines with many sections; small test docs missed
    // the layout/render bug that surfaces at real-doc scale.
    private static string BuildRealMarkdownDoc(int i)
    {
        var sb = new System.Text.StringBuilder();
        sb.AppendLine($"# Document {i:D2} — large-shape stress doc");
        sb.AppendLine();

        // Many sections, each with paragraphs + nested formatting.
        for (int section = 0; section < 20; section++)
        {
            sb.AppendLine($"## Section {section} of doc {i}");
            sb.AppendLine();

            // 4-6 paragraphs per section.
            for (int para = 0; para < 5; para++)
            {
                sb.AppendLine(
                    $"Paragraph {para} of section {section}: this contains **bold runs** " +
                    "and *italic* and `inline code` and [a link to docs](https://example.com/" +
                    $"{i}/{section}/{para}) and more text after the link to test wrapping.");
                sb.AppendLine();
            }

            // A list every other section.
            if (section % 2 == 0)
            {
                for (int li = 1; li <= 8; li++)
                {
                    sb.AppendLine($"{li}. list item {li} with **emphasis** and `code`");
                }
                sb.AppendLine();
            }
            else
            {
                for (int li = 0; li < 6; li++)
                {
                    sb.AppendLine($"- bullet {li} carrying [a link](https://x.y/{li}) and content");
                }
                sb.AppendLine();
            }

            // Code block.
            sb.AppendLine("```");
            for (int cl = 0; cl < 5; cl++)
            {
                sb.AppendLine($"code line {cl} of section {section}");
            }
            sb.AppendLine("```");
            sb.AppendLine();

            // Blockquote every fourth section.
            if (section % 4 == 0)
            {
                sb.AppendLine($"> A quoted statement in section {section}.");
                sb.AppendLine("> Spanning multiple lines.");
                sb.AppendLine();
            }

            sb.AppendLine("---");
            sb.AppendLine();
        }

        return sb.ToString();
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

    private static int CountMatchingRows(TreeViewPanel tree)
    {
        int n = 0;
        for (int i = 0; i < tree.RowsCountForTests; i++)
        {
            if (tree.IsEntryForTests(i)) n++;
        }
        return n;
    }

    private static System.Collections.Generic.List<string> HarvestMatchingPaths(TreeViewPanel tree)
    {
        var paths = new System.Collections.Generic.List<string>();
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
