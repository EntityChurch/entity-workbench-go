using System;
using System.Collections.Generic;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Controls.Documents;
using Avalonia.Input;
using Avalonia.Interactivity;
using Avalonia.Layout;
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
using MdTable = Markdig.Extensions.Tables.Table;
using MdTableRow = Markdig.Extensions.Tables.TableRow;
using MdTableCell = Markdig.Extensions.Tables.TableCell;

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
//   - Footnotes are still unhandled.
//
// # The placeholder that was a bug, not a TODO (2026-08-31)
//
// Every block type this switch did not know printed
// "[unsupported markdown block: X]" into the page. Two of the three an
// operator hit on the live corpus were not unimplemented features:
//
//   - `LinkReferenceDefinitionGroup` is the `[ref]: url` definitions
//     block. It renders as NOTHING in every markdown implementation
//     there is — it is metadata the inline parser has already consumed.
//     Printing a placeholder for it announced a failure where the
//     correct output was silence.
//   - `HtmlBlock` is inline HTML inside a markdown page. Shouting
//     "unsupported" at it and dropping the content is worse than
//     lowering it to its text, which is what the body projection does
//     for a whole HTML page anyway.
//   - `Table` was genuinely unbuilt. It is built now, and it is the
//     reason InlineUIContainer appears twice in this file: a Grid
//     cannot live in an inline flow any other way.
//
// # Images
//
// A site's figures are `::embed[alt]{ref=assets/…}` on the wire;
// workbench lowers those into markdown images before the body reaches
// here, so this file sees one grammar. `OnAsset` fetches the bytes —
// see [EmitImage] for why it is asynchronous and what happens when it
// returns nothing.
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

    // OnLink, when non-null, makes markdown links CLICKABLE and is
    // invoked with the link's raw href exactly as the publisher wrote it
    // (`support.md`, `site:other`, `../notes/x.md`, `https://…`).
    //
    // Resolving that href is deliberately not this renderer's job — it is
    // Layer-2 contract shared with entity-browser-rust and it lives in
    // `workbench/site_model.go::ClassifyTarget`. Pass it through.
    //
    // Threading: invoked on the UI thread from a click handler.
    [ThreadStatic]
    private static Action<string>? _onLink;

    // OnAsset, when non-null, makes markdown IMAGES render as pictures.
    // It is handed the raw `ref` from the page body and an Image control
    // to fill; it is expected to return immediately and fill the control
    // later, from the UI thread. See EmitImage.
    [ThreadStatic]
    private static Action<string, Image>? _onAsset;

    public static List<Inline> BuildInlines(string markdown) => BuildInlines(markdown, null);

    public static List<Inline> BuildInlines(string markdown, Action<string>? onLink)
        => BuildInlines(markdown, onLink, null);

    public static List<Inline> BuildInlines(string markdown, Action<string>? onLink,
        Action<string, Image>? onAsset)
    {
        var result = new List<Inline>();
        if (string.IsNullOrEmpty(markdown)) return result;
        _onAsset = onAsset;

        // [ThreadStatic] rather than threading a parameter through the
        // eight Emit* methods: the emitters form a recursive descent over
        // Markdig's tree and every one of them would need the parameter
        // to reach the single site that uses it. Build is synchronous and
        // single-threaded per call, so the field's lifetime is this call.
        _onLink = onLink;
        try
        {
            return BuildInlinesCore(markdown, result);
        }
        finally
        {
            _onLink = null;
            _onAsset = null;
        }
    }

    private static List<Inline> BuildInlinesCore(string markdown, List<Inline> result)
    {

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
            case MdTable table:
                EmitTable(table, result);
                break;
            case LinkReferenceDefinitionGroup:
            case LinkReferenceDefinition:
                // NOTHING, deliberately. `[ref]: https://…` is a
                // definition block: the inline parser has already used it
                // to resolve `[text][ref]`, and every markdown renderer
                // there is emits no output for it. The old default case
                // printed "[unsupported markdown block:
                // LinkReferenceDefinitionGroup]" into the middle of a
                // live page, announcing a failure where the correct
                // behaviour is silence.
                break;
            case HtmlBlock html:
                EmitHtmlBlock(html, result);
                break;
            default:
                // A block type we genuinely do not handle. Keep the
                // placeholder — swallowing content silently is worse —
                // but say what a reader can do about it.
                result.Add(new Run
                {
                    Text = $"[this browser does not render a {block.GetType().Name} yet]",
                    Foreground = new SolidColorBrush(Color.FromArgb(0x88, 0xff, 0x88, 0x88)),
                    FontStyle = FontStyle.Italic,
                });
                result.Add(new LineBreak());
                break;
        }
    }

    // EmitHtmlBlock lowers raw HTML inside a markdown page to its text.
    //
    // Same posture as workbench's PlainTextFromHTML and for the same
    // reason: these bytes came from a remote publisher, so the job is to
    // show what the document says without interpreting anything it asks
    // for. No DOM, no fetches, no styling. If nothing readable is left —
    // a bare `<div>` wrapper, a comment — emit nothing rather than an
    // empty line.
    private static void EmitHtmlBlock(HtmlBlock html, List<Inline> result)
    {
        var text = StripTags(html.Lines.ToString());
        if (string.IsNullOrWhiteSpace(text)) return;
        result.Add(new Run { Text = text.Trim() });
        result.Add(new LineBreak());
    }

    // StripTags is the C#-side lowering for an HTML *fragment*.
    //
    // Whole HTML PAGES are lowered in Go (workbench.PlainTextFromHTML),
    // where the format decision is made once for every renderer. This is
    // the fragment case, which only arises for an HtmlBlock inside a
    // markdown body and so cannot be done there — the body is markdown.
    // It follows the same rule that matters: a `<` only opens a tag when
    // what follows can begin one, so `if a < b` in prose survives.
    private static string StripTags(string s)
    {
        var sb = new System.Text.StringBuilder(s.Length);
        for (int i = 0; i < s.Length;)
        {
            if (s[i] == '<' && i + 1 < s.Length && OpensTag(s[i + 1]))
            {
                int gt = s.IndexOf('>', i);
                if (gt < 0) { sb.Append(s, i, s.Length - i); break; }
                i = gt + 1;
                continue;
            }
            sb.Append(s[i]);
            i++;
        }
        return System.Net.WebUtility.HtmlDecode(sb.ToString());
    }

    private static bool OpensTag(char c) =>
        (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '/' || c == '!' || c == '?';

    // EmitTable draws a pipe table as a real Grid.
    //
    // # Why this needs InlineUIContainer
    //
    // The whole document is one SelectableTextBlock so drag-selection
    // crosses paragraph boundaries (see the class note). A Grid is a
    // Control, not an Inline, and the only supported way to put a
    // Control into an inline flow is InlineUIContainer — the same lever
    // clickable links use. The cost is the same too: the table's text is
    // a separate control, so it does not participate in the surrounding
    // drag-selection.
    //
    // Cells are rendered through the ordinary inline emitters, so a link
    // or an image inside a table cell works exactly as it does anywhere
    // else. That is worth more than it sounds: a corpus that puts
    // figures in tables would otherwise lose them at this boundary.
    private static void EmitTable(MdTable table, List<Inline> result)
    {
        var grid = new Grid { Margin = new Thickness(0, 4, 0, 6) };
        int cols = 0;
        foreach (var rowObj in table)
        {
            if (rowObj is MdTableRow r) cols = Math.Max(cols, r.Count);
        }
        if (cols == 0) return;
        for (int c = 0; c < cols; c++)
        {
            grid.ColumnDefinitions.Add(new ColumnDefinition(GridLength.Auto));
        }

        int rowIndex = 0;
        foreach (var rowObj in table)
        {
            if (rowObj is not MdTableRow row) continue;
            grid.RowDefinitions.Add(new RowDefinition(GridLength.Auto));
            for (int c = 0; c < row.Count; c++)
            {
                if (row[c] is not MdTableCell cell) continue;
                var cellInlines = new List<Inline>();
                foreach (var sub in cell)
                {
                    if (sub is ParagraphBlock p && p.Inline != null)
                    {
                        EmitInline(p.Inline, cellInlines, regular: true);
                    }
                    else
                    {
                        EmitBlock(sub, cellInlines);
                    }
                }
                var tb = new TextBlock
                {
                    FontSize = BaseFontSize - 1,
                    FontWeight = row.IsHeader ? FontWeight.SemiBold : FontWeight.Normal,
                    TextWrapping = TextWrapping.Wrap,
                    MaxWidth = 420,
                    Padding = new Thickness(6, 3),
                };
                foreach (var inl in cellInlines) tb.Inlines!.Add(inl);
                if (row.IsHeader)
                {
                    tb.Background = new SolidColorBrush(Color.FromArgb(0x1a, 0xff, 0xff, 0xff));
                }
                Grid.SetRow(tb, rowIndex);
                Grid.SetColumn(tb, c);
                grid.Children.Add(tb);
            }
            rowIndex++;
        }

        var border = new Border
        {
            Child = grid,
            BorderThickness = new Thickness(1),
            BorderBrush = new SolidColorBrush(Color.FromArgb(0x33, 0xff, 0xff, 0xff)),
            CornerRadius = new CornerRadius(3),
        };
        result.Add(new InlineUIContainer(border));
        result.Add(new LineBreak());
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

    // EmitLink renders one markdown link.
    //
    // # Why a control and not a styled Span
    //
    // An Avalonia `Inline` is not an `InputElement`: it has no pointer
    // events at all, so a `Span` can be blue and underlined and can never
    // be clicked. That is precisely what shipped — links were styled,
    // the URL was appended in parentheses "so users can see + copy it",
    // and a comment recorded click-to-open as "a future concern". On the
    // live federation every page's links were therefore dead, which reads
    // to a user as a broken browser rather than an unimplemented feature.
    //
    // `InlineUIContainer` embeds a real control in the text flow, which
    // is the supported way to get input on something inline. The cost is
    // that the link text no longer participates in the surrounding
    // drag-selection (it is a separate control), so we keep the href
    // visible beside it when there is no click handler and drop the
    // parenthetical when there is one — a clickable link does not need
    // its URL spelled out mid-sentence.
    // EmitImage renders `![alt](ref)` as an actual picture.
    //
    // # Why this case existed and did nothing
    //
    // Markdig models an image as a LinkInline with IsImage=true, so
    // before 2026-08-31 every figure fell through EmitLink and rendered
    // as a clickable link labelled with its alt text. Clicking one asked
    // the model to navigate to a page named `assets/figures/x.png`,
    // which no site commits — a dead link where a figure should be. On
    // billslab's methodology site that is thirteen per gallery page,
    // across 665 committed figures.
    //
    // # Why it is asynchronous, and what happens meanwhile
    //
    // The bytes come from the verifying consumer. After the trie walk
    // they are usually a cache hit, but a miss is an HTTP round trip and
    // this method runs on the UI thread during a layout pass. So the
    // control is returned empty with the alt text under it, and the
    // callback fills it later. The alt text is NOT removed when the
    // image lands — it is the publisher's caption, and the reference
    // renderer keeps it too.
    //
    // # When there is no image
    //
    // No OnAsset (the plain markdown viewer), a ref the security gate
    // refused, an asset the signed root does not commit, or a format
    // Avalonia cannot decode — all four render the alt text and say what
    // happened. **Never a broken-image glyph**: this browser's contract
    // is that what is on screen came with a chain, and a placeholder
    // that means "something should be here" with no explanation is the
    // opposite of that.
    private static Inline EmitImage(MdLinkInline link)
    {
        var reference = link.Url ?? "";
        var alt = LinkText(link);

        var handler = _onAsset;
        if (handler == null || string.IsNullOrEmpty(reference))
        {
            return new Span
            {
                Inlines =
                {
                    new Run
                    {
                        Text = $"🖼 {alt}",
                        FontStyle = FontStyle.Italic,
                        Foreground = new SolidColorBrush(Color.FromArgb(0xaa, 0xff, 0xff, 0xff)),
                    },
                },
            };
        }

        var image = new Image
        {
            Stretch = Stretch.Uniform,
            // A figure is a figure, not a wallpaper. Bounded so one
            // oversized asset cannot push the rest of the page off
            // screen, and so a page of thirteen of them is scrollable.
            MaxWidth = 720,
            MaxHeight = 540,
            HorizontalAlignment = HorizontalAlignment.Left,
        };
        var caption = new TextBlock
        {
            Text = alt,
            FontSize = BaseFontSize - 2,
            FontStyle = FontStyle.Italic,
            Opacity = 0.7,
            TextWrapping = TextWrapping.Wrap,
            MaxWidth = 720,
            Margin = new Thickness(0, 2, 0, 0),
        };
        var box = new StackPanel
        {
            Orientation = Orientation.Vertical,
            Margin = new Thickness(0, 6, 0, 6),
            Children = { image, caption },
        };
        handler(reference, image);
        return new InlineUIContainer(box);
    }

    private static Inline EmitLink(MdLinkInline link)
    {
        if (link.IsImage) return EmitImage(link);

        var url = link.Url ?? "";
        var text = LinkText(link);

        var handler = _onLink;
        if (handler == null || string.IsNullOrEmpty(url))
        {
            // No navigation available (plain rendering, e.g. the
            // markdown file viewer): keep the old behaviour exactly,
            // including the URL in parentheses so it can be read+copied.
            var span = new Span
            {
                Foreground = new SolidColorBrush(Color.FromRgb(0x7e, 0xc5, 0xff)),
                TextDecorations = TextDecorations.Underline,
            };
            var inner = new List<Inline>();
            EmitInline(link, inner, regular: true);
            foreach (var li in inner) span.Inlines.Add(li);
            if (!string.IsNullOrEmpty(url))
            {
                span.Inlines.Add(new Run
                {
                    Text = $" ({url})",
                    Foreground = new SolidColorBrush(Color.FromArgb(0x88, 0xff, 0xff, 0xff)),
                    TextDecorations = null,
                });
            }
            return span;
        }

        var external = url.StartsWith("http://", StringComparison.OrdinalIgnoreCase)
                    || url.StartsWith("https://", StringComparison.OrdinalIgnoreCase)
                    || url.StartsWith("mailto:", StringComparison.OrdinalIgnoreCase);

        var label = new TextBlock
        {
            Text = text,
            FontSize = BaseFontSize,
            TextWrapping = TextWrapping.Wrap,
            // External links are visibly a different thing, because
            // clicking one does NOT navigate — the model reports it
            // instead. Colouring them identically would make that
            // refusal look like a failure.
            Foreground = external
                ? new SolidColorBrush(Color.FromRgb(0x9a, 0xa4, 0xb0))
                : new SolidColorBrush(Color.FromRgb(0x7e, 0xc5, 0xff)),
            TextDecorations = TextDecorations.Underline,
            Cursor = new Avalonia.Input.Cursor(StandardCursorType.Hand),
            // The href is the tooltip rather than inline parentheses:
            // still inspectable, no longer shouted mid-paragraph.
            [ToolTip.TipProperty] = external ? url + "  (leaves the entity system)" : url,
        };

        // AddHandler, never `+=` — AP37/P7.
        //
        // TUNNEL ONLY, and both halves of that matter:
        //
        //   * Tunnel, because the link sits inside a SelectableTextBlock,
        //     which handles PointerPressed itself to start a drag
        //     selection. A bubble-phase handler would be racing a class
        //     handler that marks the event handled.
        //   * ONLY tunnel — registering `Tunnel | Bubble` on the same
        //     element invokes the handler on BOTH passes, i.e. twice per
        //     click. That is what the first version did, and
        //     `An_InSite_Link_Is_Clickable_And_Reports_Its_Raw_Href`
        //     caught it: two navigations per click, which in the browser
        //     would push two history entries and make Back feel broken.
        label.AddHandler(InputElement.PointerPressedEvent, (_, e) =>
        {
            handler(url);
            e.Handled = true;
        }, RoutingStrategies.Tunnel, handledEventsToo: true);

        return new InlineUIContainer(label) { BaselineAlignment = BaselineAlignment.TextBottom };
    }

    // LinkText flattens a link's children to their text. Markdig nests
    // emphasis inside links (`**[Support](support.md)**` is common in the
    // live corpus), so this must walk rather than read one literal.
    private static string LinkText(MdContainerInline container)
    {
        var sb = new System.Text.StringBuilder();
        AppendText(container, sb);
        return sb.Length == 0 ? "(link)" : sb.ToString();
    }

    // MaxInlineDepth bounds every recursive descent over a parsed inline
    // tree.
    //
    // Both walkers below recurse once per nesting level of a document we
    // did not write: a site page arrives as remote bytes, Markdig parses
    // it, and nesting depth is whatever the author (or an attacker) put
    // there. Unbounded, that is a stack overflow reachable from a page
    // body — and a stack overflow on .NET is **uncatchable**: no
    // exception, no `Dispatcher.UnhandledException`, no crash log,
    // because there is no stack left to run a handler on. It surfaces as
    // a bare SIGSEGV when the guard page is hit.
    //
    // THIS BOUND WAS ADDED FOR THE WRONG REASON, AND THE REASON IS
    // RETRACTED. It was written on 2026-09-01 believing it might be the
    // 2026-09-01 SIGSEGV: the coredump's faulting thread showed 45
    // consecutive frames returning to one address, systemd truncates a
    // backtrace, so "a single call site recursing without end" looked
    // like the safe reading. Measuring the stack instead of counting the
    // printed frames refutes it — the frames are a uniform 448 bytes,
    // there are exactly 45 of them, and the chain reaches 24 KB below
    // the thread descriptor. That is an ordinary recursive walk, not an
    // overflow. The faulting thread was also a BACKGROUND thread with
    // libSkiaSharp frames on it, and this walker only ever runs on the
    // UI thread, so it is doubly not the one. `make crash-stack` now
    // prints that measurement automatically.
    //
    // The bound STAYS, on its own merits: a site page arrives as remote
    // bytes and nesting depth is whoever wrote the page, so an unbounded
    // recursion here is a real defect in the same hazard class as
    // AssetNameFromRef — a hostile page body reaching something it
    // should not. It is kept as hardening, and it is NOT a fix for any
    // observed crash. Do not let a future session re-derive the
    // retracted claim from the fact that the bound exists.
    //
    // 64 is far beyond any real document. Markdown that nests emphasis
    // deeper than this is not prose; truncating it loses nothing a reader
    // wanted and keeps the process alive.
    private const int MaxInlineDepth = 64;

    private static void AppendText(MdContainerInline container, System.Text.StringBuilder sb, int depth = 0)
    {
        foreach (var node in container)
        {
            switch (node)
            {
                case MdLiteralInline lit:
                    sb.Append(lit.Content.ToString());
                    break;
                case MdCodeInline ci:
                    sb.Append(ci.Content);
                    break;
                case MdContainerInline inner:
                    // Refuse to descend past the bound rather than
                    // truncating silently — the ellipsis is the reader's
                    // only signal that the document said more.
                    if (depth >= MaxInlineDepth) { sb.Append('\u2026'); break; }
                    AppendText(inner, sb, depth + 1);
                    break;
                case MdLeafInline leaf:
                    sb.Append(leaf.ToString());
                    break;
            }
        }
    }

    private static void EmitInline(MdContainerInline container, List<Inline> result, bool regular, int depth = 0)
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
                    if (depth >= MaxInlineDepth) { result.Add(new Run { Text = "\u2026" }); break; }
                    var nested = new List<Inline>();
                    EmitInline(em, nested, regular: true, depth + 1);
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
                    result.Add(EmitLink(link));
                    break;
                case MdLineBreakInline:
                    result.Add(new Run { Text = " " });
                    break;
                case MdContainerInline ctr:
                    if (depth >= MaxInlineDepth) { result.Add(new Run { Text = "\u2026" }); break; }
                    EmitInline(ctr, result, regular, depth + 1);
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
