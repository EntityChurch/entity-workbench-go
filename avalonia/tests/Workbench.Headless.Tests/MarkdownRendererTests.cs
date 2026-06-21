using System.Collections.Generic;
using System.Diagnostics;
using Avalonia.Controls.Documents;
using Avalonia.Headless.XUnit;
using EntityAvalonia.Panels;
using Xunit;
using Xunit.Abstractions;

namespace EntityAvalonia.Tests;

// Tests for our custom MarkdownRenderer (replaces the prior
// Markdown.Avalonia probe — that library is gone). The renderer is a
// pure function, so most of this file is direct calls against
// BuildInlines, no UI needed.
public sealed class MarkdownRendererTests
{
    private readonly ITestOutputHelper _output;
    public MarkdownRendererTests(ITestOutputHelper output) { _output = output; }

    [AvaloniaFact]
    public void Empty_Input_Produces_Empty_Output()
    {
        Assert.Empty(MarkdownRenderer.BuildInlines(""));
        Assert.Empty(MarkdownRenderer.BuildInlines((string)null!));
    }

    [AvaloniaFact]
    public void Heading_Produces_Bold_Span_With_Larger_Font()
    {
        var inlines = MarkdownRenderer.BuildInlines("# Hello");
        Assert.NotEmpty(inlines);
        var span = FindFirst<Span>(inlines);
        Assert.NotNull(span);
        Assert.True(span!.FontSize > 14, $"heading font size {span.FontSize} should exceed body size");
        Assert.Equal(Avalonia.Media.FontWeight.Bold, span.FontWeight);
    }

    [AvaloniaFact]
    public void Paragraph_With_Bold_Emits_Bold_Span()
    {
        var inlines = MarkdownRenderer.BuildInlines("This is **bold** text.");
        Assert.NotEmpty(inlines);
        var span = FindFirst<Span>(inlines);
        Assert.NotNull(span);
        Assert.Equal(Avalonia.Media.FontWeight.Bold, span!.FontWeight);
    }

    [AvaloniaFact]
    public void Inline_Code_Emits_Monospace_Run()
    {
        var inlines = MarkdownRenderer.BuildInlines("Use `make test` to run.");
        var run = FindRunWithText(inlines, "make test");
        Assert.NotNull(run);
        Assert.Equal("monospace", run!.FontFamily?.Name);
    }

    [AvaloniaFact]
    public void Fenced_Code_Block_Emits_Monospace_Run()
    {
        var md = "```\nline one\nline two\n```";
        var inlines = MarkdownRenderer.BuildInlines(md);
        Assert.NotEmpty(inlines);
        bool foundMono = false;
        foreach (var i in inlines)
        {
            if (i is Run r && r.FontFamily?.Name == "monospace")
            {
                foundMono = true;
                Assert.Contains("line one", r.Text ?? "");
                break;
            }
        }
        Assert.True(foundMono, "fenced code block should produce a monospace Run");
    }

    [AvaloniaFact]
    public void Bulleted_List_Emits_Bullet_Prefixes()
    {
        var inlines = MarkdownRenderer.BuildInlines("- alpha\n- beta\n");
        var allText = JoinAllText(inlines);
        Assert.Contains("• ", allText);
        Assert.Contains("alpha", allText);
        Assert.Contains("beta", allText);
    }

    [AvaloniaFact]
    public void Numbered_List_Emits_Numbered_Prefixes()
    {
        var inlines = MarkdownRenderer.BuildInlines("1. first\n2. second\n");
        var allText = JoinAllText(inlines);
        Assert.Contains("1. ", allText);
        Assert.Contains("2. ", allText);
    }

    [AvaloniaFact]
    public void Link_Surfaces_URL()
    {
        var inlines = MarkdownRenderer.BuildInlines("See [docs](https://example.com/x)");
        var allText = JoinAllText(inlines);
        Assert.Contains("docs", allText);
        Assert.Contains("https://example.com/x", allText);
    }

    [AvaloniaFact]
    public void Rapid_Renders_Do_Not_Slow_Down()
    {
        // 100 rebuilds across three real-shape documents. If our
        // renderer accumulates state somewhere (it shouldn't — it's
        // a pure function with no statics other than the pipeline),
        // this will surface as growing per-call time.
        var docs = new[]
        {
            "# A\n\nParagraph **bold** and *italic*.\n\n- one\n- two\n\n```\ncode\n```",
            "## B\n\nDifferent shape.\n\n1. first\n2. second\n\n> a quote here.",
            "### C\n\nShort with [a link](https://x.y/z) and `inline code`.\n\n---\n\nfooter.",
        };

        const int iters = 100;
        var times = new List<long>(iters);
        var sw = new Stopwatch();
        for (int i = 0; i < iters; i++)
        {
            sw.Restart();
            var inlines = MarkdownRenderer.BuildInlines(docs[i % docs.Length]);
            sw.Stop();
            Assert.NotEmpty(inlines);
            times.Add(sw.ElapsedMilliseconds);
        }

        long first = Avg(times, 0, iters / 3);
        long last = Avg(times, 2 * iters / 3, iters / 3);
        _output.WriteLine($"BuildInlines: first-third avg {first}ms, last-third avg {last}ms, total {Sum(times)}ms");
        Assert.True(last < System.Math.Max(first * 3, 50),
            $"BuildInlines slowed across {iters} iterations — first {first}ms vs last {last}ms");
    }

    // --- helpers -----------------------------------------------------

    private static T? FindFirst<T>(IEnumerable<Inline> inlines) where T : Inline
    {
        foreach (var i in inlines)
        {
            if (i is T t) return t;
        }
        return null;
    }

    private static Run? FindRunWithText(IEnumerable<Inline> inlines, string substring)
    {
        foreach (var i in inlines)
        {
            if (i is Run r && (r.Text?.Contains(substring) ?? false)) return r;
            if (i is Span s)
            {
                var found = FindRunWithText(s.Inlines, substring);
                if (found != null) return found;
            }
        }
        return null;
    }

    private static string JoinAllText(IEnumerable<Inline> inlines)
    {
        var sb = new System.Text.StringBuilder();
        Walk(inlines, sb);
        return sb.ToString();

        static void Walk(IEnumerable<Inline> xs, System.Text.StringBuilder sb)
        {
            foreach (var i in xs)
            {
                switch (i)
                {
                    case Run r:
                        sb.Append(r.Text);
                        break;
                    case Span s:
                        Walk(s.Inlines, sb);
                        break;
                    case LineBreak:
                        sb.Append('\n');
                        break;
                }
            }
        }
    }

    private static long Avg(List<long> xs, int start, int count)
    {
        if (count == 0) return 0;
        long sum = 0;
        for (int i = 0; i < count && start + i < xs.Count; i++) sum += xs[start + i];
        return sum / count;
    }

    private static long Sum(List<long> xs)
    {
        long s = 0;
        foreach (var x in xs) s += x;
        return s;
    }
}
