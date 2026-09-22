package workbench

import (
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/hash"

	"entity-workbench-go/entitysdk"
)

// site_ref_test.go — Tier 1, and the gate for `APP-CONVENTION-REFERENCE`
// §3.4's grammar.
//
// Two halves, and BOTH are load-bearing. The positive half asserts each
// of the four forms resolves, which is what was broken. The negative
// half asserts every refusal that was doing real work still fires —
// without it, "stop refusing three legal forms" has an implementation
// that refuses nothing, and it would pass.
//
// The cross-implementation vectors are NOT here. They live in
// site_asset_crossimpl_test.go, unchanged, because [AssetNameFromRef]
// is unchanged: the fix adds arms around it rather than editing the one
// function whose behaviour is shared with `entity-browser-rust`. If a
// change ever makes that file's expectations need editing, the change is
// wrong — route it instead.

const refTestPeer = "z6MkhaXgBZDvotDkL5257faiztiGiC2QtKLGpbnnEGta2doK"

// TestClassifyRefForm_EveryFormInSection34 is one vector per form,
// which is the shape the work item asked for.
func TestClassifyRefForm_EveryFormInSection34(t *testing.T) {
	for _, c := range []struct {
		name string
		ref  string
		want RefForm
	}{
		// --- §3.4's four forms, each of which MUST resolve ---
		{"relative", "assets/figures/x.png", RefFormRelative},
		{"root-absolute", "/assets/figures/x.png", RefFormRootAbsolute},
		{"site: short form", "site:other-site/assets/x.png", RefFormSite},
		{"entity+ref absolute", refScheme() + refTestPeer + "/sites/lab/assets/x.png", RefFormEntityRef},

		// --- §4's legacy dispatch form: resolved, and named so a surface
		// can say it did (REF-R19) ---
		{"legacy entity://", "entity://" + refTestPeer + "/sites/lab/assets/x.png", RefFormLegacyDispatch},

		// --- leaving the system ---
		{"https", "https://tracker.example/x.png", RefFormExternal},
		{"http", "http://tracker.example/x.png", RefFormExternal},
		{"protocol-relative", "//tracker.example/x.png", RefFormExternal},
		{"data", "data:image/png;base64,AAAA", RefFormExternal},
		{"mailto", "mailto:someone@example.test", RefFormExternal},
		{"an unrelated scheme", "ftp://host/x.png", RefFormExternal},
		{"empty", "", RefFormExternal},

		// --- REF-R20: unparseable is external, never re-anchored ---
		{"entity+ref with no authority", "entity+ref:sites/lab", RefFormExternal},
		{"entity with no authority", "entity:sites/lab", RefFormExternal},

		// --- REF-R11: the scheme is case-insensitive on parse ---
		{"uppercase scheme", "ENTITY+REF://" + refTestPeer + "/sites/lab/x", RefFormEntityRef},
		{"mixed-case site scheme", "Site:other/assets/x.png", RefFormSite},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := ClassifyRefForm(c.ref); got != c.want {
				t.Errorf("ClassifyRefForm(%q) = %v; want %v", c.ref, got, c.want)
			}
			// The second predicate is derived from the first and must
			// stay that way; this is the assertion that notices if
			// someone re-implements it.
			if got, want := RefLeavesSystem(c.ref), c.want == RefFormExternal; got != want {
				t.Errorf("RefLeavesSystem(%q) = %v; want %v", c.ref, got, want)
			}
		})
	}
}

// TestClassifyRefForm_AColonInAPathSegmentIsNotAScheme is the control
// arm for the scheme scanner.
//
// A scheme's colon precedes any `/` (RFC 3986 §3.1) and a scheme begins
// with ALPHA. Without both rules, an ordinary published figure whose
// name carries a colon would be classified as leaving the system and
// would stop rendering — a refusal invented by the fix for refusals.
func TestClassifyRefForm_AColonInAPathSegmentIsNotAScheme(t *testing.T) {
	for _, ref := range []string{
		"assets/figures/a:b.png",
		"assets/2:1-ratio.png",
		"/assets/figures/a:b.png",
	} {
		if got := ClassifyRefForm(ref); got == RefFormExternal {
			t.Errorf("ClassifyRefForm(%q) = external; a colon inside a path segment is not a scheme", ref)
		}
	}
	// And the other side of the same rule: a leading segment that IS a
	// scheme leaves the system. RFC 3986 §4.2 forbids this spelling in a
	// relative-path reference precisely because it cannot be told apart.
	if got := ClassifyRefForm("javascript:alert(1)"); got != RefFormExternal {
		t.Errorf("ClassifyRefForm(javascript:…) = %v; want external", got)
	}
}

// TestClassifyAssetRef_EveryFormResolvesToTheSameAsset is the positive
// half, and the point of the whole work item.
//
// Four spellings of one figure. Before 2026-09-11 exactly one of them
// resolved and the other three were reported as security refusals.
func TestClassifyAssetRef_EveryFormResolvesToTheSameAsset(t *testing.T) {
	cur := Location{PeerID: refTestPeer, SiteID: "lab", Page: "research/model/grounding"}

	for _, c := range []struct {
		name      string
		ref       string
		wantPeer  string
		wantSite  string
		wantForm  RefForm
		wantAsset string
	}{
		{
			name: "relative — the form the live corpus publishes",
			ref:  "assets/figures/x.png", wantPeer: refTestPeer, wantSite: "lab",
			wantForm: RefFormRelative, wantAsset: "figures/x.png",
		},
		{
			name: "root-absolute — §3.4 SHOULDs this for generated links",
			ref:  "/assets/figures/x.png", wantPeer: refTestPeer, wantSite: "lab",
			wantForm: RefFormRootAbsolute, wantAsset: "figures/x.png",
		},
		{
			name: "site: — a different site, the implied (current) peer",
			ref:  "site:other-site/assets/figures/x.png", wantPeer: refTestPeer, wantSite: "other-site",
			wantForm: RefFormSite, wantAsset: "figures/x.png",
		},
		{
			name:     "entity+ref:// — a different peer entirely",
			ref:      refScheme() + refTestPeer + "/sites/lab/assets/figures/x.png",
			wantPeer: refTestPeer, wantSite: "lab",
			wantForm: RefFormEntityRef, wantAsset: "figures/x.png",
		},
		{
			name:     "entity:// — the legacy form, resolved and NAMED (REF-R19)",
			ref:      "entity://" + refTestPeer + "/sites/lab/assets/figures/x.png",
			wantPeer: refTestPeer, wantSite: "lab",
			wantForm: RefFormLegacyDispatch, wantAsset: "figures/x.png",
		},
		{
			name:     "entity:// pre-v0.5 layout still resolves",
			ref:      "entity://" + refTestPeer + "/content/sites/lab/assets/figures/x.png",
			wantPeer: refTestPeer, wantSite: "lab",
			wantForm: RefFormLegacyDispatch, wantAsset: "figures/x.png",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			ar, ok := ClassifyAssetRef(c.ref, cur)
			if !ok {
				t.Fatalf("ClassifyAssetRef(%q) refused: %v — §3.4 says this form MUST resolve",
					c.ref, ar.Refusal)
			}
			if ar.Form != c.wantForm {
				t.Errorf("form = %v; want %v", ar.Form, c.wantForm)
			}
			if ar.PeerID != c.wantPeer {
				t.Errorf("peer = %q; want %q", ar.PeerID, c.wantPeer)
			}
			if ar.SiteID != c.wantSite {
				t.Errorf("site = %q; want %q", ar.SiteID, c.wantSite)
			}
			if ar.Name != c.wantAsset {
				t.Errorf("asset name = %q; want %q", ar.Name, c.wantAsset)
			}
		})
	}
}

// TestAssetNameFromRef_StillRefusesTheThreeFormsOnItsOwn is the
// anti-vacuity arm for the test above, and it is the one that keeps the
// cross-implementation contract honest.
//
// The four forms resolve because [ClassifyAssetRef] grew arms around
// [AssetNameFromRef] — NOT because that function was loosened. It is
// byte-identical with `entity-browser-rust`'s `asset_name_from_ref` and
// a change to it is a divergence in a Layer-2 algorithm contract. If
// this test fails, the fix was applied in the wrong place: move it back
// out and route the question instead.
func TestAssetNameFromRef_StillRefusesTheThreeFormsOnItsOwn(t *testing.T) {
	for _, ref := range []string{
		"/assets/figures/x.png",
		"site:other-site/assets/figures/x.png",
		refScheme() + refTestPeer + "/sites/lab/assets/figures/x.png",
		"entity://" + refTestPeer + "/sites/lab/assets/figures/x.png",
	} {
		if got, ok := AssetNameFromRef(ref); ok {
			t.Errorf("AssetNameFromRef(%q) = (%q, true) — the shared function must stay "+
				"byte-identical with the reference implementation; the four forms are "+
				"admitted by ClassifyAssetRef, not by widening this", ref, got)
		}
	}
	// And the control: the one form it does answer still answers.
	if got, ok := AssetNameFromRef("assets/figures/x.png"); !ok || got != "figures/x.png" {
		t.Errorf("AssetNameFromRef lost its own arm: (%q, %v)", got, ok)
	}
}

// TestClassifyAssetRef_TheHostileRefsStillRefuse is the negative half.
//
// Every entry is a way a page body could otherwise make this process
// fetch something the publisher never committed. **If this test goes
// green because the refusals were removed, the fix failed in the more
// dangerous direction** — and nothing else in the suite would notice,
// because a renderer that fetches a tracking URL renders correctly.
func TestClassifyAssetRef_TheHostileRefsStillRefuse(t *testing.T) {
	cur := Location{PeerID: refTestPeer, SiteID: "lab", Page: "index"}

	for _, c := range []struct {
		ref  string
		want AssetRefRefusal
		why  string
	}{
		{"https://tracker.example/x.png", AssetRefLeavesSystem, "an arbitrary origin — a read receipt for every reader"},
		{"http://tracker.example/x.png", AssetRefLeavesSystem, "the same, unencrypted"},
		{"//tracker.example/x.png", AssetRefLeavesSystem, "the same, protocol-relative"},
		{"data:image/png;base64,AAAA", AssetRefLeavesSystem, "inline bytes bypassing the tree entirely"},
		{"ftp://host/x.png", AssetRefLeavesSystem, "any other scheme"},
		{"", AssetRefMalformed, "nothing"},

		// Containment, which the root-absolute arm must NOT have
		// loosened. `/etc/passwd` was refused for being absolute; it is
		// now refused for not being under assets/, which is the rule
		// that was doing the work all along.
		{"/etc/passwd", AssetRefOutsideSubgraph, "root-absolute, outside the assets subgraph"},
		{"figures/x.png", AssetRefOutsideSubgraph, "not under assets/"},
		{"assets/../../secret", AssetRefOutsideSubgraph, "a parent escape"},
		{"/assets/../../secret", AssetRefOutsideSubgraph, "the same escape, root-absolute"},
		{"assets/", AssetRefOutsideSubgraph, "the prefix alone names no file"},
		{"site:other/secrets/x.png", AssetRefOutsideSubgraph, "cross-site, and not under assets/"},
		{"site:other/assets/../../x", AssetRefOutsideSubgraph, "cross-site escape"},

		// The newly-admitted forms do not admit anything that is not an
		// asset. An absolute reference at a page path is well-formed and
		// is not a figure.
		{refScheme() + refTestPeer + "/sites/lab/pages/intro", AssetRefOutsideSubgraph,
			"an absolute reference to a page is not an asset"},
		{refScheme() + refTestPeer + "/local/files/etc/passwd", AssetRefOutsideSubgraph,
			"an absolute reference outside any site"},

		// REF-R20: an entity scheme that does not parse leaves the
		// system rather than being re-anchored.
		{"entity+ref:sites/lab/assets/x.png", AssetRefLeavesSystem, "no authority — not the scheme at all"},
		{"site:", AssetRefMalformed, "no site id"},
	} {
		t.Run(c.ref+" / "+c.why, func(t *testing.T) {
			ar, ok := ClassifyAssetRef(c.ref, cur)
			if ok {
				t.Fatalf("ClassifyAssetRef(%q) RESOLVED to %s/%s/%s — %s",
					c.ref, ar.PeerID, ar.SiteID, ar.Name, c.why)
			}
			if ar.Refusal != c.want {
				t.Errorf("refusal = %v (%s); want %v", int(ar.Refusal), ar.Refusal, int(c.want))
			}
		})
	}
}

// TestClassifyAssetRef_APinnedReferenceIsNamedNotGuessedAt covers the
// third state.
//
// A pinned reference is well-formed, names a real thing, and has no
// path-shaped identity (§3.1: its path is empty on purpose). It is not
// hostile and it is not malformed, so reporting it as either would be a
// smaller version of the defect this whole file is the fix for.
func TestClassifyAssetRef_APinnedReferenceIsNamedNotGuessedAt(t *testing.T) {
	pin := entitysdk.PinnedRef(refTestPeer, refTestHash(t, "some figure"))
	uri, err := pin.URI()
	if err != nil {
		t.Fatalf("building a pinned reference: %v", err)
	}
	ar, ok := ClassifyAssetRef(uri, Location{PeerID: refTestPeer, SiteID: "lab"})
	if ok {
		t.Fatalf("a pinned reference resolved to an asset name (%q) — there is no path to derive one from", ar.Name)
	}
	if ar.Refusal != AssetRefPinned {
		t.Errorf("refusal = %v; want AssetRefPinned — a pin is neither hostile nor malformed", ar.Refusal)
	}
	if ar.PeerID != refTestPeer {
		t.Errorf("peer = %q; want %q — the peer is known even when the asset is not", ar.PeerID, refTestPeer)
	}
}

// TestClassifyTarget_EntityRefIsNotReAnchoredIntoAPageSlug is the link
// position's half of the same grammar gap, and it is a live defect
// rather than a hypothetical.
//
// Before this, `entity+ref://` fell through every arm to
// resolveInSitePage and became the in-site page
// `entity+ref:/PEER/sites/lab/pages/intro` — a well-formed address of
// something nobody published, reported as `page missing`, i.e. as the
// publisher's fault. That is REF-R20's named failure verbatim.
func TestClassifyTarget_EntityRefIsNotReAnchoredIntoAPageSlug(t *testing.T) {
	cur := Location{PeerID: "OTHER", SiteID: "main", Page: "index"}
	target := refScheme() + refTestPeer + "/sites/lab/pages/intro"

	loc, kind, ok := ClassifyTarget(target, cur)
	if !ok || kind != LinkCrossPeer {
		t.Fatalf("ClassifyTarget(%q) = kind %v ok %v; want LinkCrossPeer true", target, kind, ok)
	}
	if loc.PeerID != refTestPeer || loc.SiteID != "lab" || loc.Page != "intro" {
		t.Errorf("loc = %+v; want peer %s site lab page intro", loc, refTestPeer)
	}
	// The anti-vacuity arm: assert the OLD answer is genuinely gone. A
	// classifier that returned LinkCrossPeer with a re-anchored slug
	// would satisfy everything above.
	if strings.Contains(loc.Page, "entity+ref") || strings.Contains(loc.SiteID, "entity+ref") {
		t.Errorf("the scheme survived into the location: %+v", loc)
	}
}

// TestClassifyTarget_UnknownSchemesLeaveTheSystem is the other arm the
// link position gained.
//
// Anything scheme-bearing that is not ours used to fall through to the
// same re-anchoring: `data:`, `ftp:` and `javascript:` all became in-site
// page slugs. No renderer here executes a URL, so this was not a live
// exploit — it was a mis-classification that would become one the first
// time a surface handed a target to the OS.
func TestClassifyTarget_UnknownSchemesLeaveTheSystem(t *testing.T) {
	cur := Location{PeerID: "PEER", SiteID: "main", Page: "index"}
	for _, target := range []string{
		"javascript:alert(1)",
		"data:text/html,<script>",
		"ftp://host/file",
		"https://example.test/x",
		"mailto:a@b.test",
		"//example.test/x",
	} {
		loc, kind, ok := ClassifyTarget(target, cur)
		if kind != LinkExternal || !ok {
			t.Errorf("ClassifyTarget(%q) = %+v kind %v ok %v; want LinkExternal true",
				target, loc, kind, ok)
		}
	}
	// A pinned reference in a link position is refused rather than
	// turned into a page: ok=false, so the caller neither navigates nor
	// hands it to the OS.
	pin := entitysdk.PinnedRef(refTestPeer, refTestHash(t, "x"))
	uri, err := pin.URI()
	if err != nil {
		t.Fatalf("building a pinned reference: %v", err)
	}
	if loc, kind, ok := ClassifyTarget(uri, cur); ok {
		t.Errorf("a pinned reference navigated to %+v (kind %v) — it names bytes, not a page", loc, kind)
	}
}

// refScheme re-exports the SDK constant so the vectors above read as the
// spelling a publisher writes rather than as a concatenation.
func refScheme() string { return entitysdk.RefScheme }

// refTestHash builds a content hash for a pinned-reference vector.
func refTestHash(t *testing.T, seed string) hash.Hash {
	t.Helper()
	h, err := hash.OfBytes(hash.AlgorithmSHA256, []byte(seed))
	if err != nil {
		t.Fatalf("hashing %q: %v", seed, err)
	}
	return h
}
