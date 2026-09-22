package workbench

import (
	"fmt"
	"html"
	"strings"
	"unicode"
)

// body_display.go — the projection between "the verified bytes" and
// "what a renderer can actually put on a screen", and the two facts
// nobody was carrying across it.
//
// # Fact one: `format` is a real field and every renderer ignored it
//
// `SitePage.format` is part of the site convention and `"html"` is a
// permitted value — the web-tier escape hatch for a pre-rendered
// document (a Pandoc paper), stored verbatim. [SiteRenderOutput] has
// carried `BodyFormat` since it was written. The Avalonia panel's DTO
// never declared the field, so the panel could not tell HTML from
// markdown and fed both to Markdig. An operator's report of it: *"I
// guess this is the raw HTML. That makes sense. That looks bad."*
//
// # Fact two: some of these pages are megabytes
//
// Measured on billslab.com, 2026-08-31:
//
//	papers/full-corpus       html   8,267,316 bytes
//	papers/narrative-order   html   8,267,821 bytes
//	papers/tree-order        html   8,267,511 bytes
//	papers/methodology-path  html   4,458,626 bytes
//	papers/12-abiogenesis…   html   2,498,516 bytes
//
// There was no size guard anywhere on this path. Eight megabytes went
// through the cgo bridge as a JSON string, into Markdig, and out as an
// Avalonia inline list on the UI thread. The P4 bounded-block rule
// splits at 500 inlines *per block* and does not bound the total, so it
// does not help here. This is a large part of the "fifteen seconds"
// report, and unlike the network half it is not fixed by caching —
// a cached 8 MB page is 8 MB of parsing, every time it is displayed.
//
// # Why the cap is honest rather than lossy
//
// The chain step already states the **full** size of the verified body
// ("N bytes of html"), so a truncated display beside a chain that names
// the real number is not a page pretending to be complete. What would be
// dishonest is silence, so [BodyView.Note] says what was dropped and the
// verified byte count stays available. Truncation is a display decision
// and is never allowed to look like a publisher decision.

// MaxDisplayBytes bounds the text a renderer is handed.
//
// 256 KiB is roughly 40k words — past any length a person reads in one
// pane, and two orders of magnitude below the corpus dumps above.
const MaxDisplayBytes = 256 << 10

// PageFormatMarkdown / PageFormatHTML are the two `format` values live
// in this cohort.
const (
	PageFormatMarkdown = "markdown"
	PageFormatHTML     = "html"
)

// BodyView is a page body prepared for display.
//
// It is a projection, never a substitute: the verified bytes stay on
// [SiteRenderOutput.BodyMarkdown] for anything that wants them.
type BodyView struct {
	// Format is the page's declared format, verbatim.
	Format string
	// Text is what to draw.
	Text string
	// IsMarkdown says whether Text should be parsed as markdown. False
	// means it is plain text and MUST NOT be run through a markdown
	// parser — HTML lowered to text is full of characters markdown would
	// re-interpret.
	IsMarkdown bool
	// Truncated is true when Text is shorter than the body it came from.
	Truncated bool
	// FullBytes is the size of the verified body.
	FullBytes int
	// Note explains any lowering or truncation, in a sentence for a
	// person. Empty when Text is the whole body in its own format.
	Note string
}

// NewBodyView projects a verified page body for display.
func NewBodyView(format, body string) BodyView {
	v := BodyView{Format: format, FullBytes: len(body)}
	var notes []string

	switch strings.ToLower(strings.TrimSpace(format)) {
	case PageFormatHTML:
		v.Text = PlainTextFromHTML(body)
		v.IsMarkdown = false
		notes = append(notes, fmt.Sprintf(
			"This page is published as HTML (%s), which this browser does not render as a "+
				"document — the tags have been stripped to their text. Structure, tables and "+
				"images are lost; the bytes on the chain are the publisher's, unmodified.",
			byteSize(len(body))))
	case "", PageFormatMarkdown:
		// The canonical embed directive is not markdown, so lower it into
		// markdown's own image grammar before anything parses this.
		v.Text = EmbedsToMarkdownImages(body)
		v.IsMarkdown = true
		v.Format = PageFormatMarkdown
	default:
		// An unknown format is shown as plain text and SAID to be
		// unknown. Guessing markdown would render a format we do not
		// speak as though we did (AP33).
		v.Text = body
		v.IsMarkdown = false
		notes = append(notes, fmt.Sprintf(
			"This page declares format %q, which this browser does not know. It is shown as "+
				"plain text rather than interpreted as a format it may not be.", format))
	}

	if len(v.Text) > MaxDisplayBytes {
		v.Text = truncateAtRune(v.Text, MaxDisplayBytes)
		v.Truncated = true
		notes = append(notes, fmt.Sprintf(
			"Showing the first %s of %s. The rest is verified and fetched, and is not displayed "+
				"— a page this size is a document, not a screen.",
			byteSize(len(v.Text)), byteSize(v.FullBytes)))
	}
	v.Note = strings.Join(notes, " ")
	return v
}

// truncateAtRune cuts at or below n bytes without splitting a rune.
func truncateAtRune(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !isRuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }

func byteSize(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d bytes", n)
	}
}

// PlainTextFromHTML lowers an HTML document to readable text.
//
// **It is a lowering, not a parse.** There is no DOM here and there must
// not be one on this path: the input is bytes from a remote publisher,
// and the job is to show a person what the document says without this
// process interpreting anything the document asks for. So `script` and
// `style` contents are dropped rather than shown, block-level tags
// become line breaks, everything else becomes its text, and entities are
// decoded last so a decoded `&lt;script&gt;` cannot become a tag.
//
// What is lost is real and is stated in [BodyView.Note] rather than
// hidden: tables collapse to their cell text, images to nothing, links
// to their label. A reader who needs the document as a document needs an
// HTML renderer, which is a different and much larger decision (see
// docs/architecture/DEPLOYMENT-DIRECTION.md — the WebView question).
func PlainTextFromHTML(s string) string {
	var b strings.Builder
	b.Grow(len(s) / 2)

	i := 0
	for i < len(s) {
		c := s[i]
		// A `<` only opens a tag when what follows can begin one. This is
		// the HTML spec's own tokenizer rule and it is load-bearing, not
		// pedantry: prose contains `if a < b`, and a lowering that took
		// every `<` as markup would scan forward to the next `>` — the
		// close of some later element — and delete every word in between.
		// Caught by TestPlainTextFromHTML_ALoneAngleBracketIsText, which
		// failed on the first draft of this function.
		if c != '<' || !opensTag(s, i) {
			b.WriteByte(c)
			i++
			continue
		}
		// An unterminated tag at EOF is the rest of the document as text.
		end := strings.IndexByte(s[i:], '>')
		if end < 0 {
			b.WriteString(s[i:])
			break
		}
		tag := s[i+1 : i+end]
		i += end + 1

		name := tagName(tag)
		switch name {
		case "script", "style", "head", "svg", "noscript":
			// Drop the whole element. These carry no reading text and
			// showing their source is worse than showing nothing.
			if closeAt := findCloseTag(s, i, name); closeAt >= 0 {
				i = closeAt
			}
		case "br", "p", "div", "li", "tr", "h1", "h2", "h3", "h4", "h5", "h6",
			"section", "article", "header", "footer", "blockquote", "pre", "hr",
			"table", "ul", "ol", "figure", "figcaption":
			b.WriteByte('\n')
		case "td", "th":
			b.WriteString("\t")
		}
	}
	return collapseBlankRuns(html.UnescapeString(b.String()))
}

// opensTag reports whether the `<` at i begins markup.
//
// A tag name starts with an ASCII letter; `/` closes; `!` is a comment
// or doctype; `?` is a processing instruction. Anything else — a space,
// a digit, an operator — is prose.
func opensTag(s string, i int) bool {
	if i+1 >= len(s) {
		return false
	}
	c := s[i+1]
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		return true
	case c == '/' || c == '!' || c == '?':
		return true
	default:
		return false
	}
}

// tagName is the lowercase element name of a tag body, with any leading
// `/` for a close tag stripped.
func tagName(tag string) string {
	tag = strings.TrimPrefix(strings.TrimSpace(tag), "/")
	end := strings.IndexFunc(tag, func(r rune) bool {
		return unicode.IsSpace(r) || r == '>' || r == '/'
	})
	if end >= 0 {
		tag = tag[:end]
	}
	return strings.ToLower(tag)
}

// findCloseTag returns the index just past `</name>` at or after i, or
// -1. Unclosed is not an error: the caller simply keeps reading, which
// degrades to showing the element's text rather than to losing the rest
// of the document.
func findCloseTag(s string, i int, name string) int {
	want := "</" + name
	for {
		rel := indexFold(s[i:], want)
		if rel < 0 {
			return -1
		}
		at := i + rel
		if gt := strings.IndexByte(s[at:], '>'); gt >= 0 {
			return at + gt + 1
		}
		return -1
	}
}

// indexFold is a case-insensitive IndexOf for ASCII needles.
func indexFold(hay, needle string) int {
	return strings.Index(strings.ToLower(hay), strings.ToLower(needle))
}

// collapseBlankRuns trims trailing spaces per line and squeezes runs of
// blank lines to one. An HTML document lowered naively is mostly
// whitespace — the markup was carrying the structure.
func collapseBlankRuns(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	for _, ln := range lines {
		ln = strings.TrimRight(ln, " \t\r")
		if strings.TrimSpace(ln) == "" {
			blank++
			if blank > 1 {
				continue
			}
			out = append(out, "")
			continue
		}
		blank = 0
		out = append(out, ln)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
