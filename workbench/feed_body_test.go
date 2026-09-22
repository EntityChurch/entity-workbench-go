package workbench

import (
	"strings"
	"testing"

	"entity-workbench-go/entitysdk"
	"go.entitychurch.org/entity-core-go/core/hash"
)

// feed_body_test.go — `APP-CONVENTION-EMBED` §6's ladder.
//
// Every arm here fails on the reader this replaced, and the reason they are
// worth spelling out is that the old reader was **conformant**: showing a
// fallback is what §6 step 2 says to do. The defect was doing it when step 1
// was available, and no assertion phrased as *"is the output valid"* can see
// that. So the arms are phrased as *which rung*, which is the only question
// that separates the two.

func inlineNode(t *testing.T, media, body, fallback string) entitysdk.EmbedNode {
	t.Helper()
	return entitysdk.NewEmbedNode(media, entitysdk.InlinePayload([]byte(body)), fallback)
}

// A markdown body renders AS MARKDOWN, at step 1.
//
// The old reader produced byte-identical `Text` here and marked nothing, so a
// renderer showed the source. That is why `IsMarkdown` and `Rung` are asserted
// and `Text` alone is not: the string is not where the bug was.
func TestFeedBody_MarkdownRendersAtStepOne(t *testing.T) {
	b := FeedBodyView(inlineNode(t, entitysdk.FeedMediaMarkdown,
		"# Title\n\nsome **bold** prose", "a post"))

	if b.Rung != FeedBodyRendered {
		t.Errorf("rung = %q, want %q — a markdown body is drawable at §7's v1 floor, so "+
			"degrading it to the authored fallback skips a rung that was available",
			b.Rung, FeedBodyRendered)
	}
	if !b.IsMarkdown {
		t.Error("IsMarkdown = false for a text/markdown body, so a renderer shows the source")
	}
	if !strings.Contains(b.Text, "**bold**") {
		t.Errorf("the author's own bytes are not in Text: %q", b.Text)
	}
	if b.Note != "" {
		t.Errorf("a rendered body carries a note (%q). A note on the top rung trains a reader "+
			"to ignore the notes that mean something", b.Note)
	}
}

// A plain body renders at step 1 too, and MUST NOT be marked as markdown.
//
// This is the arm that stops the fix above being applied too widely: making
// everything markdown would render at the right rung and silently eat the
// asterisks out of prose nobody meant as markup.
func TestFeedBody_PlainIsRenderedAndIsNotMarkdown(t *testing.T) {
	const body = "2 * 3 * 4 is _not_ emphasis"
	b := FeedBodyView(inlineNode(t, entitysdk.FeedMediaPlain, body, "a post"))

	if b.Rung != FeedBodyRendered {
		t.Errorf("rung = %q, want %q", b.Rung, FeedBodyRendered)
	}
	if b.IsMarkdown {
		t.Error("a text/plain body is marked as markdown — a parser will eat the author's " +
			"asterisks and underscores, which is a silent edit to somebody else's words")
	}
	if b.Text != body {
		t.Errorf("Text = %q, want the bytes verbatim %q", b.Text, body)
	}
}

// The media type is matched per RFC 9110 — case-insensitively, parameters
// ignored. A reader that string-compares the whole header falls to the
// fallback rung for a body it can draw perfectly, which is this file's own
// subject one layer down.
func TestFeedBody_TheMediaTypeIsMatchedWithoutItsParameters(t *testing.T) {
	for _, mt := range []string{
		"text/markdown; charset=utf-8",
		"Text/Markdown",
		"  text/markdown  ",
	} {
		b := FeedBodyView(inlineNode(t, mt, "# hi", "a post"))
		if b.Rung != FeedBodyRendered || !b.IsMarkdown {
			t.Errorf("%q degraded to rung %q (markdown=%v); the type and subtype are "+
				"case-insensitive and the parameters are not part of the identity",
				mt, b.Rung, b.IsMarkdown)
		}
	}
}

// A media type this tier cannot draw takes step 2 — AND SAYS SO.
//
// ⭐ The `Rung`/`Note` assertions are the point. The old reader produced the
// same `Text` for this entry and for a rendered one, so an image post and a
// text post were indistinguishable on screen: the alt text simply looked like
// a short post. Asserting the text alone passes against exactly that.
func TestFeedBody_AnUndrawableMediaTypeFallsToStepTwoAndSaysSo(t *testing.T) {
	b := FeedBodyView(inlineNode(t, "image/png", "\x89PNG...", "a photo of a cat"))

	if b.Rung != FeedBodyFallback {
		t.Fatalf("rung = %q, want %q", b.Rung, FeedBodyFallback)
	}
	if b.Text != "a photo of a cat" {
		t.Errorf("Text = %q, want the authored fallback", b.Text)
	}
	if b.Note == "" {
		t.Fatal("no note on a fallback. The reader is showing the author's DESCRIPTION of a " +
			"post instead of the post, and nothing on screen says so")
	}
	if !strings.Contains(b.Note, "image/png") {
		t.Errorf("the note does not name the media type, so a reader cannot tell which "+
			"capability is missing: %q", b.Note)
	}
}

// ⭐ §6 step 2's anti-poisoning rule (S-8), and it is a security arm rather
// than a formatting one.
//
// A fallback is rendered with embed directives DISABLED at depth 1: shown as
// visible text, never re-expanded and never silently stripped. The author of
// an entry that cannot be drawn controls this string, so expanding a directive
// here would let them make every reader that DEGRADED fetch an asset of their
// choosing — the path taken by readers that can do less acquiring the wider
// reach.
//
// The natural implementation is the bug: the markdown rung lowers directives
// with `EmbedsToMarkdownImages`, and sharing that line with this one looks
// like tidying.
func TestFeedBody_AFallbackDoesNotExpandEmbedDirectives(t *testing.T) {
	const poison = "see ::embed[tracker]{ref=assets/beacon.png} for details"
	b := FeedBodyView(inlineNode(t, "image/png", "\x89PNG...", poison))

	if b.Rung != FeedBodyFallback {
		t.Fatalf("rung = %q, want %q", b.Rung, FeedBodyFallback)
	}
	if b.Text != poison {
		t.Errorf("the fallback was rewritten:\n  got  %q\n  want %q (verbatim)", b.Text, poison)
	}
	// Both halves of the rule are affirmative. Stripping it is as wrong as
	// expanding it — a reader that silently removed it would hide from the
	// operator that the author tried.
	if !strings.Contains(b.Text, "::embed[tracker]") {
		t.Error("the directive was stripped. §6 step 2 says it is rendered as VISIBLE TEXT: " +
			"not re-expanded and not silently stripped")
	}
	if strings.Contains(b.Text, "![") || strings.Contains(b.Text, "](") {
		t.Errorf("the directive was lowered into markdown's image grammar, so a renderer "+
			"will fetch it: %q", b.Text)
	}
}

// A pointer body names the missing CAPABILITY, not the author.
//
// The bytes exist and this reader did not fetch them — which is a fact about
// this tier (the pointer-body blob closure is owed) and reads, if unsaid, as
// an author who posted a caption.
func TestFeedBody_APointerBodySaysTheBytesWereNotFetched(t *testing.T) {
	node := entitysdk.NewEmbedNode("image/png",
		entitysdk.PointerPayload(hash.Hash{}), "a photo of a cat")
	b := FeedBodyView(node)

	if b.Rung != FeedBodyFallback {
		t.Fatalf("rung = %q, want %q", b.Rung, FeedBodyFallback)
	}
	if !strings.Contains(b.Note, "reference") && !strings.Contains(b.Note, "pointer") {
		t.Errorf("the note does not say the body is stored by reference and was not "+
			"fetched, so the gap reads as the author's: %q", b.Note)
	}
}

// No drawable body and no fallback is NOT an empty row.
//
// `C-6`'s finding, kept: a missing `fallback` is non-conformant (EMBED §3,
// mandatory and non-empty), and rendering it as blank presents a producer's
// defect as an author who posted nothing — the one reading that blames the
// wrong party and gives the reader nothing to act on.
func TestFeedBody_NoFallbackIsStatedRatherThanBlank(t *testing.T) {
	b := FeedBodyView(inlineNode(t, "image/png", "\x89PNG...", "   "))

	if b.Rung != FeedBodyUnrenderable {
		t.Errorf("rung = %q, want %q", b.Rung, FeedBodyUnrenderable)
	}
	if b.Note == "" {
		t.Fatal("an entry with nothing to draw and nothing to degrade to renders as an " +
			"empty row carrying no problem")
	}
	if !strings.Contains(b.Note, "fallback") {
		t.Errorf("the note does not name the missing field: %q", b.Note)
	}
}

// The display bound applies here too. An entry is short by convention, not by
// rule — `body` is an embed node like any other — and the measured cost of
// letting a large one through is in body_display.go.
func TestFeedBody_ALargeBodyIsCappedAndSaysSo(t *testing.T) {
	big := strings.Repeat("a", MaxDisplayBytes+4096)
	b := FeedBodyView(inlineNode(t, entitysdk.FeedMediaPlain, big, "a post"))

	if len(b.Text) > MaxDisplayBytes {
		t.Errorf("Text is %d bytes, over the %d bound", len(b.Text), MaxDisplayBytes)
	}
	if b.Note == "" {
		t.Error("a truncated body carries no note, so a short display looks like a short post")
	}
}
