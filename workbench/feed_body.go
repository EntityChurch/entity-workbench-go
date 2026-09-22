package workbench

import (
	"fmt"
	"strings"

	"entity-workbench-go/entitysdk"
)

// feed_body.go — `APP-CONVENTION-EMBED` §6's degradation ladder, applied to a
// feed entry's body.
//
// # What this replaces, and why it read as working
//
// `APP-CONVENTION-FEED` §2.3 makes an entry's `body` an **Embed node**, and
// `SITE` §3.1/§3.2 already shares EMBED as the vocabulary — so a feed entry
// carrying markdown, rendered by the same code that renders a site page, is
// conformant today with no spec change. This reader did not do that. It read
// the inline bytes and assigned them to a text field whatever the media type
// said, and rendered `fallback` for everything else.
//
// **Both halves of that are the same mistake**, and arch named it in one line
// (`ROUTING-2026-09-17-a` §4): *the ladder puts `fallback` LAST, and
// implementing only the last rung produces a conformant reader that displays
// an image post as its alt text.* Nothing errors. Every gate stays green,
// because a fallback IS what a conformant reader shows when it cannot do
// better — the defect is that it could.
//
// ⚠ **It is also an alignment defect and not only a local one.**
// `entity-browser-rust` took the first pass at this (`feed_body.rs`). Until
// this landed, one seat rendered markdown and the other rendered alt text for
// the same bytes — two products, one convention, and **invisible from both
// sides**, since each is internally consistent and neither reads the other's
// output.
//
// # The ladder, and the rung this tier can actually reach
//
// §6 is three steps: try the handler/renderer (§5); else the authored
// `fallback`; else pass `raw` through with its format label.
//
// **Step 1's full form does not exist anywhere in this cohort** — the §5
// handler/renderer registry has no implementation in any tree, and §4's
// `EmbedOutput` is deliberately not built here (see `entitysdk/embed.go` for
// why: storing an output vocabulary fixes the rendition choice for every
// reader forever). What IS reachable is §7's stated v1 floor — *"an
// inline-payload passive embed renders at the floor tier (tree-readable)"* —
// which for a text media type is just: draw the bytes, in the form the media
// type declares. That is the rung below, and it is a real rung rather than a
// stand-in for one.
//
// Step 3 has nothing to do here: `raw` is an `EmbedOutput` kind, and without
// §4 there is no output to pass through. Named rather than silently skipped.
//
// # The rule in step 2 that is easy to miss and is a real hazard
//
// §6 step 2: *"Fallback is rendered with embed directives DISABLED (depth-1
// bound) — an embed directive inside a fallback is rendered as visible text,
// not re-expanded and not silently stripped (anti-poisoning, S-8)."*
//
// So [EmbedsToMarkdownImages] MUST NOT run over a fallback, however natural it
// looks beside the markdown path. An author whose entry cannot be rendered
// controls the fallback text; expanding a directive there would let them make
// every reader that degraded fetch an asset of their choosing — the
// degradation path, which exists for readers that can do less, becoming the
// one with the wider reach. Both the not-re-expanded and the
// not-silently-stripped halves are affirmative: the directive is SHOWN.
//
// # `Rung` is a field because "what you are looking at" is not inferable
//
// A reader handed only text cannot tell a rendered body from an alt text, and
// the two mean opposite things about whether the post has been seen. Carrying
// which rung produced the text is what lets a surface say *"this is the
// author's description, not the post"* — and it is what stops the fallback
// rung looking like success, which is how it went unnoticed here.

// FeedBodyRung names which step of EMBED §6's ladder produced a body.
type FeedBodyRung string

const (
	// FeedBodyRendered is step 1 at §7's v1 floor: the payload was inline,
	// its media type is one this tier draws, and these are the author's
	// own bytes.
	FeedBodyRendered FeedBodyRung = "rendered"

	// FeedBodyFallback is step 2: the authored degradation text. **The post
	// itself is not on screen.**
	FeedBodyFallback FeedBodyRung = "fallback"

	// FeedBodyUnrenderable is below the ladder and is not a step of it: the
	// entry reached step 2 and had no usable `fallback` either, which
	// `APP-CONVENTION-EMBED` §3 makes non-conformant (mandatory,
	// non-empty). It is a state rather than a rung because the ladder has
	// nothing left to degrade to, and a reader that showed an empty row
	// here would render a producer's defect as an author who posted
	// nothing — which is exactly what `C-6` was.
	FeedBodyUnrenderable FeedBodyRung = "unrenderable"
)

// The media types this tier draws at the §7 floor. Both are text; the split is
// whether a markdown parser may touch the bytes. **Spelled once, in
// `entitysdk`, beside the author that writes them** — a reader holding its own
// copy of a producer's dispatch key is the shape of the divergence this file
// exists to close.
const (
	feedMediaMarkdown = entitysdk.FeedMediaMarkdown
	feedMediaPlain    = entitysdk.FeedMediaPlain
)

// FeedBody is one entry's body prepared for display.
type FeedBody struct {
	// Rung says which step of the ladder produced Text. A surface MUST be
	// able to distinguish them; see the file note.
	Rung FeedBodyRung

	// Text is what to draw. Empty only when Rung is
	// [FeedBodyUnrenderable].
	Text string

	// IsMarkdown says whether Text may be parsed as markdown. **False is
	// not "plain by default"** — a fallback is authored markdown per §6
	// step 2 and is marked true, while a `text/plain` body is marked false
	// because running a markdown parser over prose the author never meant
	// as markdown silently eats their asterisks and underscores.
	IsMarkdown bool

	// MediaType is the node's declared type, verbatim, whatever rung was
	// taken. It is what names the gap when a rung was missed.
	MediaType string

	// Note is a sentence for a person, present exactly when Text is not
	// the author's own body in its own form. Empty on
	// [FeedBodyRendered] — a rendered body needs no apology, and a note
	// there would train a reader to ignore the ones that matter.
	Note string
}

// FeedBodyView applies EMBED §6's ladder to an entry's body node.
//
// It is deliberately a pure function of the node: resolving a `pointer` or
// `child` payload needs a content store and a reader's authority, and the
// blob closure for pointer bodies is owed rather than built (see
// `feed_gather.go`'s header). Those arms therefore take step 2 **and say
// so by media type**, which is what makes the gap legible as a missing
// capability rather than as an author who wrote nothing.
func FeedBodyView(node entitysdk.EmbedNode) FeedBody {
	mt := node.MediaType()
	b := FeedBody{MediaType: mt}

	// ---- Step 1: the §7 v1 floor — an inline passive embed we can draw ----
	if node.Data.Payload.Tag == entitysdk.EmbedPayloadInline {
		switch normalizeMediaType(mt) {
		case feedMediaMarkdown:
			b.Rung = FeedBodyRendered
			// The same lowering a site page gets: the canonical `::embed`
			// directive is not markdown, so it becomes markdown's own image
			// grammar before any parser sees it. Correct HERE and forbidden
			// on the fallback arm below — §6 step 2, S-8.
			b.Text = EmbedsToMarkdownImages(string(node.Data.Payload.Bytes))
			b.IsMarkdown = true
			return b.capped()
		case feedMediaPlain:
			b.Rung = FeedBodyRendered
			b.Text = string(node.Data.Payload.Bytes)
			b.IsMarkdown = false
			return b.capped()
		}
	}

	// ---- Step 2: the authored fallback ----
	//
	// Reached by two different situations that must not be described the
	// same way: a media type this tier cannot draw (the post exists and is
	// not being shown), and a payload arm this tier does not resolve (the
	// post's bytes were never fetched). An operator can act on the second
	// and not on the first.
	fallback := strings.TrimSpace(node.Data.Fallback)
	if fallback == "" {
		b.Rung = FeedBodyUnrenderable
		b.Note = fmt.Sprintf(
			"this entry carries a %s body that this reader does not draw, and no `fallback` — "+
				"which APP-CONVENTION-EMBED §3 makes mandatory and non-empty, so the entry is "+
				"not conformant and there is nothing left to degrade to", describeMedia(mt))
		return b
	}

	b.Rung = FeedBodyFallback
	// NOT lowered. §6 step 2 bounds the fallback at depth 1: a directive in
	// here is shown as text, never re-expanded (and never stripped).
	b.Text = fallback
	b.IsMarkdown = true

	switch node.Data.Payload.Tag {
	case entitysdk.EmbedPayloadPointer:
		b.Note = fmt.Sprintf(
			"showing the author's fallback text. The %s body is stored by reference and this "+
				"reader does not yet fetch pointer bodies, so the post itself is not on screen.",
			describeMedia(mt))
	case entitysdk.EmbedPayloadChild:
		b.Note = fmt.Sprintf(
			"showing the author's fallback text. The %s body is a transcluded child embed and "+
				"this reader does not yet resolve them, so the post itself is not on screen.",
			describeMedia(mt))
	default:
		b.Note = fmt.Sprintf(
			"showing the author's fallback text — this reader cannot draw %s. The post itself "+
				"is not on screen.", describeMedia(mt))
	}
	return b.capped()
}

// capped applies the same display bound a page body gets. An entry is short by
// convention and not by rule: `body` is an embed node like any other, so
// nothing stops one carrying a megabyte of inline markdown, and the cgo
// bridge plus a markdown parser on the UI thread is the measured cost of
// letting one through (see body_display.go).
func (b FeedBody) capped() FeedBody {
	if len(b.Text) <= MaxDisplayBytes {
		return b
	}
	full := len(b.Text)
	b.Text = truncateAtRune(b.Text, MaxDisplayBytes)
	note := fmt.Sprintf("showing the first %s of %s.", byteSize(len(b.Text)), byteSize(full))
	if b.Note == "" {
		b.Note = note
	} else {
		b.Note += " " + note
	}
	return b
}

// normalizeMediaType drops parameters and folds case, so `Text/Markdown;
// charset=utf-8` dispatches as `text/markdown`. RFC 9110 makes the type and
// subtype case-insensitive and the parameters not part of the identity, and a
// reader that string-compared the whole header would fall to the fallback rung
// for a body it can draw perfectly — which is this file's own bug, one layer
// down.
func normalizeMediaType(mt string) string {
	if i := strings.IndexByte(mt, ';'); i >= 0 {
		mt = mt[:i]
	}
	return strings.ToLower(strings.TrimSpace(mt))
}

// describeMedia names a media type for a person, without pretending an empty
// one is a type.
func describeMedia(mt string) string {
	if strings.TrimSpace(mt) == "" {
		return "an untyped"
	}
	return mt
}
