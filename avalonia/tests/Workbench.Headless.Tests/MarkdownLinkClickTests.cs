using System;
using System.Collections.Generic;
using System.Linq;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Controls.Documents;
using Avalonia.Headless;
using Avalonia.Headless.XUnit;
using Avalonia.Input;
using Avalonia.VisualTree;
using EntityAvalonia.Panels;
using Xunit;

namespace EntityAvalonia.Tests;

// Tier-3 coverage for markdown link clicks.
//
// # The defect
//
// Links were rendered as a styled `Span` with the URL appended in
// parentheses, under a comment reading "Click-to-open is a future
// concern". An Avalonia `Inline` is not an `InputElement` — it has no
// pointer events of any kind — so that markup could never be clicked no
// matter what handler was later attached to it. On the live federation
// this meant every link on every page was dead, which a user reads as a
// broken browser rather than a missing feature.
//
// # Why these press the mouse instead of invoking the handler
//
// The whole failure was that input could not REACH the link. A test that
// calls the click delegate directly would have passed against the broken
// `Span` implementation, because the delegate was never the problem —
// hit-testing was. So these lay the document out in a real window, find
// the link's control, and press it at its hit-tested centre, the way
// `ProgramPanelInputTests` does for the on-screen d-pad and for the same
// reason (AP37: the on-screen controller rendered perfectly and set no
// bit for three weeks).
public sealed class MarkdownLinkClickTests
{
    private static (Window window, SelectableTextBlock block) Layout(string markdown, Action<string>? onLink)
    {
        var block = new SelectableTextBlock { Width = 600 };
        foreach (var inline in MarkdownRenderer.BuildInlines(markdown, onLink))
        {
            block.Inlines!.Add(inline);
        }
        var window = new Window { Content = block, Width = 640, Height = 400 };
        window.Show();
        window.UpdateLayout();
        HeadlessPump.Flush();
        return (window, block);
    }

    // FindLinkControl locates the control a link was rendered as. If the
    // renderer regresses to an inline-only Span this returns null, which
    // is the assertion that matters.
    private static TextBlock? FindLinkControl(Visual root, string text) =>
        root.GetVisualDescendants()
            .OfType<TextBlock>()
            .FirstOrDefault(t => t.Text == text);

    [AvaloniaFact]
    public void An_InSite_Link_Is_Clickable_And_Reports_Its_Raw_Href()
    {
        var clicked = new List<string>();
        var (window, block) = Layout(
            "See [Support Bill's Lab](support.md) for more.", clicked.Add);

        var link = FindLinkControl(block, "Support Bill's Lab");
        Assert.True(link != null,
            "the link did not render as a control — an Avalonia Inline receives no pointer input, " +
            "so a Span-only link can never be clicked");

        window.UpdateLayout();
        HeadlessPump.Flush();
        Assert.True(link!.Bounds.Width > 0 && link.Bounds.Height > 0,
            $"link has no layout bounds ({link.Bounds}); it cannot be hit-tested");

        var point = link.TranslatePoint(
            new Point(link.Bounds.Width / 2, link.Bounds.Height / 2), window);
        Assert.True(point.HasValue, "link is not positioned relative to the window");

        window.MouseDown(point!.Value, MouseButton.Left);
        HeadlessPump.Flush();
        window.MouseUp(point.Value, MouseButton.Left);
        HeadlessPump.Flush();

        // The RAW href, not a resolved slug. Resolution is Layer-2
        // contract and lives in workbench/site_model.go; a renderer that
        // "helpfully" stripped the .md here would be a second, drifting
        // copy of that rule.
        Assert.Equal(new[] { "support.md" }, clicked.ToArray());
    }

    [AvaloniaFact]
    public void A_Link_Inside_Bold_Is_Still_Clickable()
    {
        // The live corpus writes `**[Entity System Research](site:...)**`.
        // Markdig nests the link inside an emphasis inline, so the link
        // text has to be flattened out of the children rather than read
        // from one literal.
        var clicked = new List<string>();
        var (window, block) = Layout(
            "Two ways in:\n\n- **[Entity System Research](site:billslab-entity-system)** — the corpus.\n",
            clicked.Add);

        var link = FindLinkControl(block, "Entity System Research");
        Assert.True(link != null, "a link nested inside bold did not render as a clickable control");

        window.UpdateLayout();
        HeadlessPump.Flush();
        var point = link!.TranslatePoint(
            new Point(link.Bounds.Width / 2, link.Bounds.Height / 2), window)!.Value;
        window.MouseDown(point, MouseButton.Left);
        HeadlessPump.Flush();
        window.MouseUp(point, MouseButton.Left);
        HeadlessPump.Flush();

        Assert.Equal(new[] { "site:billslab-entity-system" }, clicked.ToArray());
    }

    [AvaloniaFact]
    public void An_External_Link_Is_Clickable_And_Passed_Through_Unchanged()
    {
        // External links are NOT filtered out in the renderer. The model
        // decides what to do with one (it declines and posts a notice) —
        // deciding here would put the rule in two places.
        var clicked = new List<string>();
        var (window, block) = Layout("A [Ko-fi page](https://ko-fi.com/billslab) exists.", clicked.Add);

        var link = FindLinkControl(block, "Ko-fi page");
        Assert.True(link != null, "external link did not render as a control");

        window.UpdateLayout();
        HeadlessPump.Flush();
        var point = link!.TranslatePoint(
            new Point(link.Bounds.Width / 2, link.Bounds.Height / 2), window)!.Value;
        window.MouseDown(point, MouseButton.Left);
        HeadlessPump.Flush();
        window.MouseUp(point, MouseButton.Left);
        HeadlessPump.Flush();

        Assert.Equal(new[] { "https://ko-fi.com/billslab" }, clicked.ToArray());
    }

    [AvaloniaFact]
    public void Without_A_Handler_The_Old_Inline_Rendering_Is_Kept()
    {
        // MarkdownViewPanel and the file viewer render markdown with no
        // navigation available. Those must keep the URL visible inline,
        // because there is nothing to click and a bare underlined phrase
        // would hide where it points.
        var (_, block) = Layout("See [docs](https://example.com/x)", null);

        Assert.Null(FindLinkControl(block, "docs"));
        // SelectableTextBlock.Text is null when the content came from
        // Inlines, so read the inline tree rather than the property.
        var text = InlineText(block.Inlines);
        Assert.Contains("https://example.com/x", text, StringComparison.Ordinal);
    }

    // InlineText flattens an inline tree to its text.
    private static string InlineText(InlineCollection? inlines)
    {
        if (inlines == null) return "";
        var sb = new System.Text.StringBuilder();
        Walk(inlines, sb);
        return sb.ToString();

        static void Walk(InlineCollection items, System.Text.StringBuilder sb)
        {
            foreach (var i in items)
            {
                switch (i)
                {
                    case Run r: sb.Append(r.Text); break;
                    case Span s when s.Inlines != null: Walk(s.Inlines, sb); break;
                    case LineBreak: sb.Append('\n'); break;
                }
            }
        }
    }

    [AvaloniaFact]
    public void Clicking_Is_The_Only_Thing_That_Fires_The_Handler()
    {
        // Anti-vacuity: if the handler fired on render, every assertion
        // above would pass without any input being dispatched at all.
        var clicked = new List<string>();
        Layout("See [Support](support.md) here.", clicked.Add);
        Assert.Empty(clicked);
    }
}
