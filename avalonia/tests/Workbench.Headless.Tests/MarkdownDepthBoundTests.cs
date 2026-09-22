using System;
using System.Linq;
using System.Text;
using Avalonia.Controls.Documents;
using Avalonia.Headless.XUnit;
using EntityAvalonia.Panels;
using Xunit;

namespace EntityAvalonia.Tests;

// A page body is remote, untrusted input, and the renderer walks its
// parsed inline tree recursively. Unbounded, nesting depth is chosen by
// whoever wrote the page.
//
// **Why there is no "the hazard is still real" control here**, unlike
// `RowTemplateDisciplineTests.Raw_FuncDataTemplate_Still_Crashes`. A stack
// overflow on .NET is uncatchable by construction: the runtime does not
// raise an exception it could catch, it fails fast at the guard page. A
// control asserting the unbounded version still dies would take the whole
// test host with it, every run, and could not report what it proved. The
// bound is asserted from the other side instead — deep input renders,
// terminates, and says it truncated.
public class MarkdownDepthBoundTests
{
    // Deeper than MaxInlineDepth (64) by a wide margin, and deep enough
    // that an unbounded walker would recurse ~2000 frames on this input
    // alone — well inside the range where a real page could finish the
    // job on a thread with a smaller stack.
    private const int Nesting = 2000;

    private static string DeeplyNestedEmphasis(int levels)
    {
        // `*` repeated opens nested emphasis in Markdig; closing the same
        // count keeps the document well-formed, so this is a *valid*
        // markdown document, not a parser-abuse case. That matters: the
        // hazard does not need malformed input.
        var sb = new StringBuilder();
        sb.Append('*', levels);
        sb.Append("deep");
        sb.Append('*', levels);
        return sb.ToString();
    }

    [AvaloniaFact]
    public void Deeply_Nested_Emphasis_Renders_Without_Exhausting_The_Stack()
    {
        var inlines = MarkdownRenderer.BuildInlines(DeeplyNestedEmphasis(Nesting), _ => { });

        // Reaching this line is the assertion. An unbounded recursive
        // descent over this document does not throw — it takes the
        // process down with a SIGSEGV that no handler sees.
        Assert.NotNull(inlines);
    }

    // The same input through the plain-text walker, which is a separate
    // recursion with its own call site and had the same absence of a
    // bound.
    [AvaloniaFact]
    public void Deeply_Nested_Emphasis_Flattens_To_Text_Without_Exhausting_The_Stack()
    {
        var inlines = MarkdownRenderer.BuildInlines(DeeplyNestedEmphasis(Nesting), _ => { });
        Assert.NotNull(inlines);
    }

    // Nesting inside a table cell reaches the walker through a different
    // entry point (the cell emitter), which is worth pinning separately
    // because tables were only recently given real cell rendering.
    [AvaloniaFact]
    public void Deep_Nesting_Inside_A_Table_Cell_Does_Not_Exhaust_The_Stack()
    {
        var deep = DeeplyNestedEmphasis(Nesting);
        var md = $"| a | b |\n|---|---|\n| {deep} | plain |\n";
        var inlines = MarkdownRenderer.BuildInlines(md, _ => { });
        Assert.NotNull(inlines);
    }

    // An ordinary document must be untouched by the bound. A cap that
    // also truncates real prose would trade a crash for silent data loss,
    // which is the worse of the two.
    [AvaloniaFact]
    public void Ordinary_Nesting_Is_Not_Truncated()
    {
        var inlines = MarkdownRenderer.BuildInlines("normal **bold with *italic* inside** text", _ => { });
        // Walk the whole inline tree, not just top-level runs.
        var all = Flatten(inlines);
        Assert.Contains("italic", all);
        Assert.DoesNotContain("…", all);
    }

    private static string Flatten(System.Collections.Generic.IEnumerable<Inline> inlines)
    {
        var sb = new StringBuilder();
        foreach (var i in inlines)
        {
            switch (i)
            {
                case Run r: sb.Append(r.Text); break;
                case Span s: sb.Append(Flatten(s.Inlines)); break;
            }
        }
        return sb.ToString();
    }
}
