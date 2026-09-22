package workbench

import "testing"

// site_asset_crossimpl_test.go — Tier 2, and a ROUTED gate.
//
// [AssetNameFromRef] and [ParseEmbeds] are Layer-2 algorithm contract in
// the sense AGENTS.md fixes: their output selects bytes in a
// content-addressed tree, so two implementations that disagree render
// the same published page differently. The vectors below are lifted
// verbatim from `entity-browser-rust`:
//
//	src/content_site/paths.rs
//	  ::asset_name_from_ref_accepts_site_local_and_rejects_external
//	src/content_site/embed.rs  (scan_embeds / parse_ref tolerances)
//
// **A failure here is routed, not locally corrected.** The same posture
// as site_link_crossimpl_test.go: if these two disagree, one of us is
// resolving a publisher's figures to a different key set, and the fix
// belongs wherever the ruling lands — not in whichever tree noticed.

// TestAssetNameFromRef_MatchesRustReference runs the reference's own
// accept/reject vectors.
//
// The rejections are the security half and are not stylistic: each one
// is a way a page body could otherwise make this process fetch
// something the publisher never committed. See site_asset.go.
func TestAssetNameFromRef_MatchesRustReference(t *testing.T) {
	cases := []struct {
		ref  string
		want string
		ok   bool
	}{
		// Site-local refs resolve to their name under assets/.
		{"assets/figures/x.png", "figures/x.png", true},
		{"assets/demo.svg", "demo.svg", true},
		// …and the live corpus's actual shape.
		{"assets/figures/abiogenesis-comprehensive-68a03a1e.png",
			"figures/abiogenesis-comprehensive-68a03a1e.png", true},

		// Rejections, each naming what it would otherwise permit.
		{"assets/../../secret", "", false},           // parent escape out of the subgraph
		{"figures/x.png", "", false},                 // not under assets/
		{"assets/", "", false},                       // the prefix alone names no file
		{"", "", false},                              // nothing
		{"https://tracker.example/x.png", "", false}, // arbitrary origin
		{"http://tracker.example/x.png", "", false},
		{"//tracker.example/x.png", "", false}, // protocol-relative, same thing
		{"/etc/passwd", "", false},             // absolute path
		{"data:image/png;base64,AAAA", "", false},
	}
	for _, c := range cases {
		got, ok := AssetNameFromRef(c.ref)
		if ok != c.ok || got != c.want {
			t.Errorf("AssetNameFromRef(%q) = (%q, %v), want (%q, %v)", c.ref, got, ok, c.want, c.ok)
		}
	}
}

// TestAssetNameFromRef_RejectsEveryEscapeSegment guards the loop rather
// than one instance of it.
//
// A `..` anywhere in the path is a rejection, not just a leading one —
// `assets/figures/../../secret` escapes just as well as
// `assets/../secret`, and a check that only looked at the prefix would
// pass the first. This is the anti-vacuity clause for the case above.
func TestAssetNameFromRef_RejectsEveryEscapeSegment(t *testing.T) {
	for _, ref := range []string{
		"assets/../secret",
		"assets/figures/../../secret",
		"assets/a/b/../../../c",
		"assets/..",
	} {
		if got, ok := AssetNameFromRef(ref); ok {
			t.Errorf("AssetNameFromRef(%q) accepted, yielding %q — that escapes the site subgraph", ref, got)
		}
	}
	// And the control: a segment that merely CONTAINS dots is fine. A
	// check that rejected those would break the live corpus, whose every
	// figure name has a hash suffix and an extension.
	if got, ok := AssetNameFromRef("assets/figures/a..b.png"); !ok || got != "figures/a..b.png" {
		t.Errorf("a dotted filename was rejected: (%q, %v)", got, ok)
	}
}

// TestParseEmbeds_LiveCorpusShape parses the exact directive billslab
// publishes, byte for byte.
func TestParseEmbeds_LiveCorpusShape(t *testing.T) {
	body := "## Abiogenesis comprehensive\n\n**Abiogenesis comprehensive**\n\n" +
		"::embed[Abiogenesis comprehensive]{ref=assets/figures/abiogenesis-comprehensive-68a03a1e.png}\n\n" +
		"`abiogenesis-comprehensive.png`\n\n" +
		"::embed[Abiogenesis context inset]{ref=assets/figures/abiogenesis-context-inset-bda62307.png}\n"

	got := ParseEmbeds(body)
	if len(got) != 2 {
		t.Fatalf("parsed %d embeds, want 2 — the live gallery pages carry thirteen of these each "+
			"and every one of them rendered as literal text before this parser existed", len(got))
	}
	if got[0].Fallback != "Abiogenesis comprehensive" {
		t.Errorf("fallback = %q", got[0].Fallback)
	}
	if got[0].Ref != "assets/figures/abiogenesis-comprehensive-68a03a1e.png" {
		t.Errorf("ref = %q", got[0].Ref)
	}
	if body[got[0].Start:got[0].End] !=
		"::embed[Abiogenesis comprehensive]{ref=assets/figures/abiogenesis-comprehensive-68a03a1e.png}" {
		t.Errorf("range %d..%d spans %q, not the directive", got[0].Start, got[0].End,
			body[got[0].Start:got[0].End])
	}
}

// TestParseEmbeds_ReferenceTolerances pins the parser's edges to the
// reference's, including the ones that look like bugs and are not.
func TestParseEmbeds_ReferenceTolerances(t *testing.T) {
	// A `]` inside the fallback is fine unless immediately followed by
	// `{` — scan_embeds ends the fallback at the first `]{`.
	if e := ParseEmbeds("::embed[a ] b]{ref=assets/x.png}"); len(e) != 1 ||
		e[0].Fallback != "a ] b" || e[0].Ref != "assets/x.png" {
		t.Errorf("bracket-in-fallback: %+v", e)
	}
	// Quoted refs are unquoted.
	if e := ParseEmbeds(`::embed[a]{ref="assets/x.png"}`); len(e) != 1 || e[0].Ref != "assets/x.png" {
		t.Errorf("double-quoted ref: %+v", e)
	}
	if e := ParseEmbeds(`::embed[a]{ref='assets/x.png'}`); len(e) != 1 || e[0].Ref != "assets/x.png" {
		t.Errorf("single-quoted ref: %+v", e)
	}
	// `ref=` is one token among space-separated key=value attrs.
	if e := ParseEmbeds("::embed[a]{width=400 ref=assets/x.png alt=hi}"); len(e) != 1 ||
		e[0].Ref != "assets/x.png" {
		t.Errorf("ref among other attrs: %+v", e)
	}
	// A directive with no ref parses with an empty one rather than being
	// dropped: the fallback text is still the author's, and swallowing
	// the node would silently delete a caption.
	if e := ParseEmbeds("::embed[caption]{width=400}"); len(e) != 1 || e[0].Ref != "" {
		t.Errorf("ref-less directive: %+v", e)
	}
	// Unterminated forms yield what was complete before them, never a
	// panic and never a partial splice.
	for _, s := range []string{"::embed[unclosed", "::embed[a]{unclosed", "::embed["} {
		if e := ParseEmbeds(s); len(e) != 0 {
			t.Errorf("ParseEmbeds(%q) = %+v, want none", s, e)
		}
	}
}

// TestEmbedsToMarkdownImages_LowersIntoTheImageLane is the join that
// makes one renderer code path enough.
func TestEmbedsToMarkdownImages_LowersIntoTheImageLane(t *testing.T) {
	in := "before\n::embed[Fig 1]{ref=assets/figures/x.png}\nafter"
	want := "before\n![Fig 1](assets/figures/x.png)\nafter"
	if got := EmbedsToMarkdownImages(in); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	// A body with no directives is returned untouched — no re-encoding,
	// no normalization, nothing that could perturb bytes we verified.
	plain := "# Title\n\nJust text with a [link](x.md).\n"
	if got := EmbedsToMarkdownImages(plain); got != plain {
		t.Errorf("a body with no embeds was modified:\n got  %q\n want %q", got, plain)
	}
	// Alt text is escaped so it cannot unbalance the `![…]` slot.
	if got := EmbedsToMarkdownImages("::embed[a [b] c]{ref=assets/x.png}"); got != `![a \[b\] c](assets/x.png)` {
		t.Errorf("alt escaping: %q", got)
	}
}
