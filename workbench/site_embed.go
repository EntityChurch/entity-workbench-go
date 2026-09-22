package workbench

import "strings"

// site_embed.go — how a picture is written in a site page, which is not
// how anyone guesses.
//
// # The grammar, and why we were rendering it as prose
//
// The site convention's canonical wire form for an embedded asset is a
// **directive**, not a markdown image:
//
//	::embed[Abiogenesis comprehensive]{ref=assets/figures/abiogenesis-comprehensive-68a03a1e.png}
//
// Plain `![alt](src)` is also legal and is the lightweight authoring
// form; `entity-browser-rust`'s ingest lowers it *up* into `::embed` so
// the stored body speaks one grammar (`content_site/embed.rs`). Both
// therefore appear in the wild and a reader has to handle both.
//
// Markdig — and any CommonMark parser — has no idea what `::embed[…]{…}`
// is, so it parses as an ordinary paragraph of literal text. That is
// what shipped: on billslab.com's gallery pages every figure rendered as
// the raw directive string, thirteen of them per page, and the operator
// who went looking for the images reported finding no image links
// anywhere. There were none to find. This file is the missing parse.
//
// # Tolerances are the reference's, deliberately
//
// The fallback ends at the first `]{` (so a `]` inside the fallback is
// fine unless immediately followed by `{`); attributes end at the next
// `}`; `ref=` is one space-separated `key=value` token with optional
// surrounding quotes. Transcribed from `embed.rs::scan_embeds` /
// `parse_ref` rather than improved on — a parser that is more
// permissive than the reference accepts bodies the reference renders
// differently, and the divergence shows up as a figure that appears in
// one browser and not the other.

const embedOpen = "::embed["

// Embed is one embedded asset as written in a page body.
type Embed struct {
	// Fallback is the authored alt text / caption. May be empty.
	Fallback string
	// Ref is the raw reference exactly as the publisher wrote it. It has
	// NOT been validated — pass it through [AssetNameFromRef], which is
	// the gate, before it reaches anything that fetches.
	Ref string
	// Start / End are the byte range of the directive in the body, so a
	// renderer can splice around it.
	Start, End int
}

// ParseEmbeds returns every `::embed` directive in body, in document
// order.
func ParseEmbeds(body string) []Embed {
	var out []Embed
	search := 0
	for {
		rel := strings.Index(body[search:], embedOpen)
		if rel < 0 {
			return out
		}
		start := search + rel
		afterOpen := start + len(embedOpen)
		fbRel := strings.Index(body[afterOpen:], "]{")
		if fbRel < 0 {
			return out
		}
		fbEnd := afterOpen + fbRel
		attrsStart := fbEnd + len("]{")
		closeRel := strings.Index(body[attrsStart:], "}")
		if closeRel < 0 {
			return out
		}
		attrsEnd := attrsStart + closeRel
		out = append(out, Embed{
			Fallback: body[afterOpen:fbEnd],
			Ref:      embedRef(body[attrsStart:attrsEnd]),
			Start:    start,
			End:      attrsEnd + 1,
		})
		search = attrsEnd + 1
	}
}

// embedRef pulls the `ref=` value out of a directive's attribute blob.
func embedRef(attrs string) string {
	for _, tok := range strings.Fields(attrs) {
		if v, ok := strings.CutPrefix(tok, "ref="); ok {
			return strings.Trim(v, `"'`)
		}
	}
	return ""
}

// EmbedRefs is the de-duplicated set of references a body pulls in,
// order preserved — the assets a page's closure needs.
func EmbedRefs(body string) []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range ParseEmbeds(body) {
		if e.Ref == "" || seen[e.Ref] {
			continue
		}
		seen[e.Ref] = true
		out = append(out, e.Ref)
	}
	return out
}

// EmbedsToMarkdownImages rewrites every `::embed[alt]{ref=x}` into
// `![alt](x)`.
//
// This is the lowering the reference's renderer does
// (`embed_to_markdown_image`), and it exists here for the same reason:
// it puts figures into the markdown parser's own image lane, so a
// renderer needs one image code path rather than two. The refs are not
// touched — validation stays at [AssetNameFromRef], where the renderer
// applies it at fetch time.
func EmbedsToMarkdownImages(body string) string {
	embeds := ParseEmbeds(body)
	if len(embeds) == 0 {
		return body
	}
	var b strings.Builder
	b.Grow(len(body))
	pos := 0
	for _, e := range embeds {
		if e.Start < pos || e.End > len(body) {
			continue
		}
		b.WriteString(body[pos:e.Start])
		b.WriteString("![")
		b.WriteString(mdEscapeAlt(e.Fallback))
		b.WriteString("](")
		b.WriteString(e.Ref)
		b.WriteString(")")
		pos = e.End
	}
	b.WriteString(body[pos:])
	return b.String()
}

// mdEscapeAlt keeps alt text from unbalancing the `![…]` slot.
func mdEscapeAlt(s string) string {
	r := strings.NewReplacer("[", `\[`, "]", `\]`, "\n", " ")
	return r.Replace(s)
}
