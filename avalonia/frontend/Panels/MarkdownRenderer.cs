using System.Collections.Generic;
using Avalonia;
using Avalonia.Controls.Documents;
using Avalonia.Media;
using Markdig;
using Markdig.Syntax;
// Don't import Markdig.Syntax.Inlines globally — its `Inline` type
// collides with Avalonia.Controls.Documents.Inline. Alias each
// Markdig inline type we actually use.
using MdContainerInline = Markdig.Syntax.Inlines.ContainerInline;
using MdLiteralInline = Markdig.Syntax.Inlines.LiteralInline;
using MdEmphasisInline = Markdig.Syntax.Inlines.EmphasisInline;
using MdCodeInline = Markdig.Syntax.Inlines.CodeInline;
using MdLinkInline = Markdig.Syntax.Inlines.LinkInline;
using MdLineBreakInline = Markdig.Syntax.Inlines.LineBreakInline;
using MdLeafInline = Markdig.Syntax.Inlines.LeafInline;

namespace EntityAvalonia.Panels;

// MarkdownRenderer converts a markdown string into Avalonia Inlines
// suitable for a single SelectableTextBlock. Why one TextBlock instead
// of a StackPanel-of-blocks:
//
//   - Drag selection works across the entire document (paragraph
//     boundaries don't reset the selection). Copy-to-clipboard returns
//     the full text the user dragged through.
//   - Skia has one continuous text layout to manage — no per-block
//     visual tree churn on content change.
//   - Replaces Markdown.Avalonia, which segfaulted on rapid content
//     changes and broke selection at paragraph boundaries even when
//     it didn't crash.
//
// We sacrifice some visual fidelity for this:
//   - Code blocks render as monospace runs, no background fill.
//   - Lists render as "• " or "1. " prefixes inline.
//   - Horizontal rules render as a line of "─" characters.
//   - Tables, images, footnotes are not handled (yet).
//
// The renderer is a pure function — string in, immutable inline list
// out. Tests can hit it directly without spinning up a UI.
public static class MarkdownRenderer
{
    private const double BaseFontSize = 14;
    private const string SerifFontFamily = "Inter, sans-serif";
    private const string MonoFontFamily = "monospace";

    private static readonly MarkdownPipeline Pipeline =
        new MarkdownPipelineBuilder()
            .UseAdvancedExtensions()
            .Build();

    public static List<Inline> BuildInlines(string markdown)
    {
        var result = new List<Inline>();
        if (string.IsNullOrEmpty(markdown)) return result;

        var doc = Markdig.Markdown.Parse(markdown, Pipeline);
        bool first = true;
        foreach (var block in doc)
        {
            if (!first) result.Add(new LineBreak());
            first = false;
            EmitBlock(block, result);
        }
        return result;
    }

    private static void EmitBlock(Block block, List<Inline> result)
    {
        switch (block)
        {
            case HeadingBlock h:
                EmitHeading(h, result);
                break;
            case ParagraphBlock p:
                if (p.Inline != null) EmitInline(p.Inline, result, regular: true);
                result.Add(new LineBreak());
                break;
            case FencedCodeBlock fcb:
                EmitFencedCode(fcb, result);
                break;
            case CodeBlock cb:
                EmitIndentedCode(cb, result);
                break;
            case ListBlock list:
                EmitList(list, result);
                break;
            case QuoteBlock q:
                EmitQuote(q, result);
                break;
            case ThematicBreakBlock:
                result.Add(new Run
                {
                    Text = new string('─', 40),
                    Foreground = new SolidColorBrush(Color.FromArgb(0x66, 0xff, 0xff, 0xff)),
                });
                result.Add(new LineBreak());
                break;
            default:
                // Unknown block — emit a placeholder line so the doc
                // doesn't silently swallow content.
                result.Add(new Run
                {
                    Text = $"[unsupported markdown block: {block.GetType().Name}]",
                    Foreground = new SolidColorBrush(Color.FromArgb(0x88, 0xff, 0x88, 0x88)),
                    FontStyle = FontStyle.Italic,
                });
                result.Add(new LineBreak());
                break;
        }
    }

    private static void EmitHeading(HeadingBlock h, List<Inline> result)
    {
        var size = h.Level switch
        {
            1 => BaseFontSize + 8,
            2 => BaseFontSize + 5,
            3 => BaseFontSize + 3,
            4 => BaseFontSize + 2,
            5 => BaseFontSize + 1,
            _ => BaseFontSize,
        };
        var run = new Span
        {
            FontWeight = FontWeight.Bold,
            FontSize = size,
        };
        if (h.Inline != null)
        {
            var headingInlines = new List<Inline>();
            EmitInline(h.Inline, headingInlines, regular: true);
            foreach (var inline in headingInlines) run.Inlines.Add(inline);
        }
        result.Add(run);
        result.Add(new LineBreak());
    }

    private static void EmitFencedCode(FencedCodeBlock fcb, List<Inline> result)
    {
        var text = fcb.Lines.ToString();
        result.Add(new Run
        {
            Text = text,
            FontFamily = new FontFamily(MonoFontFamily),
            FontSize = BaseFontSize - 1,
            Foreground = new SolidColorBrush(Color.FromRgb(0xb5, 0xe8, 0x99)),
        });
        result.Add(new LineBreak());
    }

    private static void EmitIndentedCode(CodeBlock cb, List<Inline> result)
    {
        var text = cb.Lines.ToString();
        result.Add(new Run
        {
            Text = text,
            FontFamily = new FontFamily(MonoFontFamily),
            FontSize = BaseFontSize - 1,
            Foreground = new SolidColorBrush(Color.FromRgb(0xb5, 0xe8, 0x99)),
        });
        result.Add(new LineBreak());
    }

    private static void EmitList(ListBlock list, List<Inline> result)
    {
        int index = 1;
        foreach (var child in list)
        {
            if (child is not ListItemBlock item) continue;
            var prefix = list.IsOrdered
                ? $"{index}. "
                : "• ";
            result.Add(new Run { Text = prefix, FontWeight = FontWeight.SemiBold });
            // Emit nested blocks inline. For simple lists this is just
            // a paragraph per item; for nested lists it recurses.
            bool firstChild = true;
            foreach (var sub in item)
            {
                if (!firstChild) result.Add(new Run { Text = "  " });
                firstChild = false;
                if (sub is ParagraphBlock p && p.Inline != null)
                {
                    EmitInline(p.Inline, result, regular: true);
                    result.Add(new LineBreak());
                }
                else
                {
                    EmitBlock(sub, result);
                }
            }
            index++;
        }
    }

    private static void EmitQuote(QuoteBlock q, List<Inline> result)
    {
        // Emit each contained block prefixed with "│ " to visually mark
        // the quoted region.
        foreach (var child in q)
        {
            result.Add(new Run
            {
                Text = "│ ",
                Foreground = new SolidColorBrush(Color.FromArgb(0xaa, 0xff, 0xff, 0xff)),
            });
            EmitBlock(child, result);
        }
    }

    private static void EmitInline(MdContainerInline container, List<Inline> result, bool regular)
    {
        foreach (var node in container)
        {
            switch (node)
            {
                case MdLiteralInline lit:
                    result.Add(new Run { Text = lit.Content.ToString() });
                    break;
                case MdEmphasisInline em:
                    var span = new Span();
                    if (em.DelimiterCount >= 2) span.FontWeight = FontWeight.Bold;
                    else span.FontStyle = FontStyle.Italic;
                    var nested = new List<Inline>();
                    EmitInline(em, nested, regular: true);
                    foreach (var n in nested) span.Inlines.Add(n);
                    result.Add(span);
                    break;
                case MdCodeInline ci:
                    result.Add(new Run
                    {
                        Text = ci.Content,
                        FontFamily = new FontFamily(MonoFontFamily),
                        Foreground = new SolidColorBrush(Color.FromRgb(0xff, 0xd6, 0x7a)),
                    });
                    break;
                case MdLinkInline link:
                    var linkSpan = new Span
                    {
                        Foreground = new SolidColorBrush(Color.FromRgb(0x7e, 0xc5, 0xff)),
                        TextDecorations = TextDecorations.Underline,
                    };
                    var linkInlines = new List<Inline>();
                    EmitInline(link, linkInlines, regular: true);
                    foreach (var li in linkInlines) linkSpan.Inlines.Add(li);
                    // Surface the URL inline so users can see + copy
                    // it. Click-to-open is a future concern.
                    if (!string.IsNullOrEmpty(link.Url))
                    {
                        linkSpan.Inlines.Add(new Run
                        {
                            Text = $" ({link.Url})",
                            Foreground = new SolidColorBrush(Color.FromArgb(0x88, 0xff, 0xff, 0xff)),
                            TextDecorations = null,
                        });
                    }
                    result.Add(linkSpan);
                    break;
                case MdLineBreakInline:
                    result.Add(new Run { Text = " " });
                    break;
                case MdContainerInline ctr:
                    EmitInline(ctr, result, regular);
                    break;
                default:
                    if (node is MdLeafInline leaf)
                    {
                        result.Add(new Run { Text = leaf.ToString() ?? "" });
                    }
                    break;
            }
        }
    }
}
