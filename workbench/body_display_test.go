package workbench

import (
	"strings"
	"testing"
)

// body_display_test.go — Tier 2. Three properties, each named for the
// live defect it prevents.

// TestBodyView_HTMLIsNotFedToAMarkdownParser is the operator's report,
// as an assertion.
//
// `SitePage.format` has always carried `"html"` and every renderer
// ignored it, so a pre-rendered Pandoc paper was handed to Markdig as
// though it were markdown and drawn as its own source. IsMarkdown=false
// is the whole fix: a renderer that respects it cannot make that
// mistake again, and one that ignores it fails this test.
func TestBodyView_HTMLIsNotFedToAMarkdownParser(t *testing.T) {
	v := NewBodyView("html", `<h1>Title</h1><p>Hello <em>world</em>.</p>`)
	if v.IsMarkdown {
		t.Fatal("an html page was marked as markdown — this is the defect verbatim")
	}
	if strings.Contains(v.Text, "<") {
		t.Errorf("tags survived the lowering: %q", v.Text)
	}
	if !strings.Contains(v.Text, "Title") || !strings.Contains(v.Text, "Hello world.") {
		t.Errorf("the reading text did not survive: %q", v.Text)
	}
	if v.Note == "" {
		t.Error("an html page was lowered silently; a reader must be told structure was dropped")
	}
}

// TestBodyView_CapsAPageThatIsADocument.
//
// billslab publishes `papers/full-corpus` at 8,267,316 bytes of HTML.
// Before the cap that crossed the cgo bridge as a JSON string and went
// into a markdown parser on the UI thread.
func TestBodyView_CapsAPageThatIsADocument(t *testing.T) {
	huge := strings.Repeat("word ", 3_000_000) // ~15 MB
	v := NewBodyView("markdown", huge)
	if !v.Truncated {
		t.Fatal("a 15 MB body was passed through whole")
	}
	if len(v.Text) > MaxDisplayBytes {
		t.Fatalf("capped text is %d bytes, over the %d cap", len(v.Text), MaxDisplayBytes)
	}
	if v.FullBytes != len(huge) {
		t.Errorf("FullBytes = %d, want %d — the honest size must survive the cap", v.FullBytes, len(huge))
	}
	if !strings.Contains(v.Note, "verified and fetched") {
		t.Errorf("truncation was not explained: %q", v.Note)
	}
}

// TestBodyView_TruncationNeverSplitsARune.
//
// A cap that cuts mid-UTF-8 hands the renderer an invalid string, which
// on the .NET side is a marshalling problem rather than a display one.
func TestBodyView_TruncationNeverSplitsARune(t *testing.T) {
	// Every rune is 3 bytes, so a byte cap lands mid-rune constantly.
	v := NewBodyView("markdown", strings.Repeat("あ", MaxDisplayBytes))
	if !v.Truncated {
		t.Fatal("expected truncation")
	}
	if !isValidUTF8(v.Text) {
		t.Fatal("the cap split a rune")
	}
}

func isValidUTF8(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}

// TestBodyView_MarkdownGetsItsEmbedsLowered joins the two halves: a
// markdown page's `::embed` directives become markdown images here, so
// the renderer has exactly one image code path.
func TestBodyView_MarkdownGetsItsEmbedsLowered(t *testing.T) {
	v := NewBodyView("markdown", "::embed[Fig]{ref=assets/figures/x.png}")
	if !v.IsMarkdown {
		t.Fatal("markdown was not marked as markdown")
	}
	if v.Text != "![Fig](assets/figures/x.png)" {
		t.Errorf("embed not lowered: %q", v.Text)
	}
}

// TestBodyView_UnknownFormatIsNotGuessed is AP33 on this path.
//
// A format we do not speak is shown as text and SAID to be unknown.
// Rendering it as markdown would be a tolerant fallback presenting
// unknown bytes as a format we claim to have interpreted.
func TestBodyView_UnknownFormatIsNotGuessed(t *testing.T) {
	v := NewBodyView("asciidoc", "= Title\n\nBody.")
	if v.IsMarkdown {
		t.Fatal("an unknown format was parsed as markdown")
	}
	if !strings.Contains(v.Note, "asciidoc") {
		t.Errorf("the unknown format was not named to the reader: %q", v.Note)
	}
}

// TestPlainTextFromHTML_DropsScriptAndStyleContents.
//
// Showing the source of a `<script>` is worse than showing nothing: it
// is noise that looks like content, and on a page of any size it is most
// of what a reader sees.
func TestPlainTextFromHTML_DropsScriptAndStyleContents(t *testing.T) {
	got := PlainTextFromHTML(
		`<html><head><style>body{color:red}</style></head>` +
			`<body><script>alert('x')</script><p>Real text.</p></body></html>`)
	if strings.Contains(got, "alert") || strings.Contains(got, "color:red") {
		t.Errorf("script/style contents leaked into the reading text: %q", got)
	}
	if !strings.Contains(got, "Real text.") {
		t.Errorf("the reading text was lost: %q", got)
	}
}

// TestPlainTextFromHTML_DecodesEntitiesLast.
//
// Order matters and is a safety property: if entities were decoded
// first, `&lt;script&gt;` in the source would become a real tag on the
// way through and its contents would then be dropped as markup — text
// the author wrote, silently deleted.
func TestPlainTextFromHTML_DecodesEntitiesLast(t *testing.T) {
	got := PlainTextFromHTML(`<p>Write &lt;script&gt;here&lt;/script&gt; to embed.</p>`)
	if !strings.Contains(got, "<script>here</script>") {
		t.Errorf("escaped markup did not survive as text: %q", got)
	}
}

// TestPlainTextFromHTML_ALoneAngleBracketIsText.
//
// `a < b` is not a tag. A lowering that treated every `<` as markup
// would eat the rest of the document from that point.
func TestPlainTextFromHTML_ALoneAngleBracketIsText(t *testing.T) {
	got := PlainTextFromHTML("<p>if a < b then keep reading</p>")
	if !strings.Contains(got, "keep reading") {
		t.Errorf("an unmatched < swallowed the document: %q", got)
	}
}
