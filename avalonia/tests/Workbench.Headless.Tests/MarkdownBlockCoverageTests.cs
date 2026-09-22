using System;
using System.Collections.Generic;
using System.Linq;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Controls.Documents;
using Avalonia.Headless.XUnit;
using Avalonia.Media;
using Avalonia.VisualTree;
using EntityAvalonia.Panels;
using Xunit;

namespace EntityAvalonia.Tests;

// Tier-3 coverage for the block types the renderer used to answer with
// "[unsupported markdown block: X]".
//
// # Why these are worth tests rather than a glance
//
// The placeholder was not a TODO. Two of the three block types an
// operator hit on the live corpus had a CORRECT rendering that was not
// "unsupported":
//
//   * `LinkReferenceDefinitionGroup` renders as nothing, everywhere,
//     always. The placeholder printed a failure notice in the middle of
//     a page whose markdown was completely valid.
//   * `HtmlBlock` has readable text inside it, which was dropped.
//
// Both are the kind of defect that survives review indefinitely because
// the output *looks* like the renderer is being honest about a limit.
// The assertions below are therefore mostly NEGATIVE — the thing that
// must not appear — which is the only shape that catches a regression
// back to a plausible-looking placeholder.
public sealed class MarkdownBlockCoverageTests
{
    private static (Window window, SelectableTextBlock block) Layout(string markdown,
        Action<string, Image>? onAsset = null)
    {
        var block = new SelectableTextBlock { Width = 700 };
        foreach (var inline in MarkdownRenderer.BuildInlines(markdown, _ => { }, onAsset))
        {
            block.Inlines!.Add(inline);
        }
        var window = new Window { Content = block, Width = 760, Height = 600 };
        window.Show();
        window.UpdateLayout();
        HeadlessPump.Flush();
        return (window, block);
    }

    // AllText reads BOTH ways a TextBlock can carry text.
    //
    // `TextBlock.Text` is null when the block was populated through
    // `Inlines` — which is how every table cell is built, because a cell
    // renders its content through the ordinary inline emitters so links
    // and images inside cells keep working. A helper that read only
    // `.Text` reported an empty table and failed this file's own test on
    // its first run.
    private static string AllText(Visual root) =>
        string.Join(" ", root.GetVisualDescendants().OfType<TextBlock>()
            .Select(TextOf).Where(s => s.Length > 0));

    private static string TextOf(TextBlock t)
    {
        if (!string.IsNullOrEmpty(t.Text)) return t.Text!;
        if (t.Inlines == null) return "";
        var sb = new System.Text.StringBuilder();
        foreach (var i in t.Inlines)
        {
            switch (i)
            {
                case Run r: sb.Append(r.Text); break;
                case Span s: sb.Append(SpanText(s)); break;
            }
        }
        return sb.ToString();
    }

    private static string SpanText(Span span)
    {
        var sb = new System.Text.StringBuilder();
        foreach (var i in span.Inlines)
        {
            switch (i)
            {
                case Run r: sb.Append(r.Text); break;
                case Span s: sb.Append(SpanText(s)); break;
            }
        }
        return sb.ToString();
    }

    private static string InlineText(SelectableTextBlock block)
    {
        var sb = new System.Text.StringBuilder();
        Walk(block.Inlines!, sb);
        return sb.ToString();

        static void Walk(InlineCollection inlines, System.Text.StringBuilder sb)
        {
            foreach (var i in inlines)
            {
                switch (i)
                {
                    case Run r: sb.Append(r.Text); break;
                    case Span s: Walk(s.Inlines, sb); break;
                    case LineBreak: sb.Append('\n'); break;
                }
            }
        }
    }

    // A link reference definition group renders as NOTHING.
    //
    // `[ref]: https://example.com` is metadata: the inline parser has
    // already used it to resolve `[text][ref]`, and no markdown
    // implementation emits output for the definition itself.
    [AvaloniaFact]
    public void LinkReferenceDefinitions_Render_As_Nothing()
    {
        var md = "See [the docs][d] for more.\n\n[d]: https://example.com/docs\n[e]: https://example.com/e\n";
        var (window, block) = Layout(md);
        try
        {
            var text = InlineText(block) + " " + AllText(window);
            Assert.DoesNotContain("unsupported", text, StringComparison.OrdinalIgnoreCase);
            Assert.DoesNotContain("LinkReferenceDefinition", text, StringComparison.Ordinal);
            // The prose around it must survive — a fix that dropped the
            // whole block including its siblings would also pass the
            // assertions above, so this is the anti-vacuity clause.
            Assert.Contains("for more", text, StringComparison.Ordinal);
        }
        finally { window.Close(); }
    }

    // An HTML block is lowered to its text, not announced as a failure.
    [AvaloniaFact]
    public void HtmlBlock_Is_Lowered_To_Its_Text()
    {
        var md = "Intro paragraph.\n\n<div class=\"note\">\n  <b>Important</b> detail here.\n</div>\n\nOutro.\n";
        var (window, block) = Layout(md);
        try
        {
            var text = InlineText(block) + " " + AllText(window);
            Assert.DoesNotContain("unsupported", text, StringComparison.OrdinalIgnoreCase);
            Assert.Contains("Important", text, StringComparison.Ordinal);
            Assert.Contains("detail here", text, StringComparison.Ordinal);
            // The markup itself must not be on screen — showing tag
            // source is the "looks bad" half of the operator's report.
            Assert.DoesNotContain("<div", text, StringComparison.Ordinal);
            Assert.DoesNotContain("<b>", text, StringComparison.Ordinal);
        }
        finally { window.Close(); }
    }

    // A pipe table renders as a real Grid with a cell per cell.
    //
    // Asserting on the Grid rather than on the text is deliberate: a
    // renderer that dumped the table's cells as a run of words would
    // satisfy any text assertion while losing the structure that makes a
    // table worth having.
    [AvaloniaFact]
    public void Table_Renders_As_A_Grid_With_Cells()
    {
        var md = "| Name | Count |\n|------|-------|\n| alpha | 1 |\n| beta | 2 |\n";
        var (window, block) = Layout(md);
        try
        {
            var text = InlineText(block) + " " + AllText(window);
            Assert.DoesNotContain("unsupported", text, StringComparison.OrdinalIgnoreCase);

            var grid = window.GetVisualDescendants().OfType<Grid>()
                .FirstOrDefault(g => g.ColumnDefinitions.Count == 2 && g.RowDefinitions.Count == 3);
            Assert.True(grid != null,
                "a 2-column, 3-row Grid was not built — the table lost its structure");

            Assert.Contains("alpha", text, StringComparison.Ordinal);
            Assert.Contains("beta", text, StringComparison.Ordinal);
            Assert.Contains("Count", text, StringComparison.Ordinal);
        }
        finally { window.Close(); }
    }

    // A table cell's inline content still goes through the ordinary
    // emitters, so a link inside a cell is a link.
    //
    // This is the property that stops the table from being a second,
    // impoverished renderer: a corpus that puts figures or links in
    // tables would otherwise lose them at exactly this boundary.
    [AvaloniaFact]
    public void Table_Cells_Keep_Their_Inline_Content()
    {
        var clicked = new List<string>();
        var block = new SelectableTextBlock { Width = 700 };
        foreach (var inline in MarkdownRenderer.BuildInlines(
            "| Doc |\n|-----|\n| [support](support.md) |\n", href => clicked.Add(href)))
        {
            block.Inlines!.Add(inline);
        }
        var window = new Window { Content = block, Width = 760, Height = 400 };
        window.Show();
        window.UpdateLayout();
        HeadlessPump.Flush();
        try
        {
            var link = window.GetVisualDescendants().OfType<TextBlock>()
                .FirstOrDefault(t => t.Text == "support");
            Assert.True(link != null, "a link inside a table cell was not rendered as a control");
            Assert.Equal("support.md", link!.GetValue(ToolTip.TipProperty));
        }
        finally { window.Close(); }
    }

    // An image renders as an Image control, NOT as a clickable link.
    //
    // # The defect this pins
    //
    // Markdig models `![alt](src)` as a LinkInline with IsImage=true, so
    // every figure fell through EmitLink and became a link labelled with
    // its alt text. Clicking one asked the model to navigate to a page
    // named `assets/figures/x.png`, which no site commits — so the live
    // corpus's 665 figures were 665 dead links.
    [AvaloniaFact]
    public void Image_Renders_As_An_Image_Not_A_Link()
    {
        var requested = new List<string>();
        var (window, _) = Layout("![Fig 1](assets/figures/x.png)\n",
            (reference, _) => requested.Add(reference));
        try
        {
            var image = window.GetVisualDescendants().OfType<Image>().FirstOrDefault();
            Assert.True(image != null, "an image was not rendered as an Image control");

            // The raw ref reaches the asset callback UNINTERPRETED —
            // deciding whether it may be fetched is workbench's
            // AssetNameFromRef, the same posture as link hrefs.
            Assert.Equal(new[] { "assets/figures/x.png" }, requested);

            // The caption survives beside the picture. It is the
            // publisher's alt text, not a loading placeholder, so it must
            // still be there whether or not the bytes ever arrive.
            var caption = window.GetVisualDescendants().OfType<TextBlock>()
                .FirstOrDefault(t => t.Text == "Fig 1");
            Assert.True(caption != null, "the figure's caption was dropped");
        }
        finally { window.Close(); }
    }

    // With no asset callback — the plain markdown viewer — an image
    // degrades to its alt text and does not become a navigable link.
    [AvaloniaFact]
    public void Image_Without_An_Asset_Source_Degrades_To_Alt_Text()
    {
        var clicked = new List<string>();
        var block = new SelectableTextBlock { Width = 700 };
        foreach (var inline in MarkdownRenderer.BuildInlines(
            "![Fig 1](assets/figures/x.png)\n", href => clicked.Add(href)))
        {
            block.Inlines!.Add(inline);
        }
        var window = new Window { Content = block, Width = 760, Height = 400 };
        window.Show();
        window.UpdateLayout();
        HeadlessPump.Flush();
        try
        {
            Assert.Empty(window.GetVisualDescendants().OfType<Image>());
            Assert.Contains("Fig 1", InlineText(block), StringComparison.Ordinal);
            // And crucially it is NOT a clickable link control: a click
            // on it would navigate to a page no site commits.
            var asLink = window.GetVisualDescendants().OfType<TextBlock>()
                .FirstOrDefault(t => t.GetValue(ToolTip.TipProperty) as string == "assets/figures/x.png");
            Assert.True(asLink == null,
                "an image was still rendered as a link — clicking it navigates to a dead page");
        }
        finally { window.Close(); }
    }

    // The live corpus writes figures as `::embed[…]{ref=…}`, which is
    // lowered to a markdown image in Go before it reaches this renderer.
    // This is the join, asserted from the renderer's side: given what
    // workbench produces, an Image comes out.
    [AvaloniaFact]
    public void The_Lowered_Embed_Form_Produces_An_Image()
    {
        // Exactly what workbench.EmbedsToMarkdownImages emits for
        // billslab's gallery pages.
        var requested = new List<string>();
        var (window, _) = Layout(
            "## Abiogenesis comprehensive\n\n" +
            "![Abiogenesis comprehensive](assets/figures/abiogenesis-comprehensive-68a03a1e.png)\n\n" +
            "`abiogenesis-comprehensive.png`\n",
            (reference, _) => requested.Add(reference));
        try
        {
            Assert.Single(window.GetVisualDescendants().OfType<Image>());
            Assert.Equal(new[] { "assets/figures/abiogenesis-comprehensive-68a03a1e.png" }, requested);
        }
        finally { window.Close(); }
    }
}
