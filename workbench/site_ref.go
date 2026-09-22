package workbench

import (
	"strings"

	"entity-workbench-go/entitysdk"
)

// site_ref.go — the reference GRAMMAR: which of the four spellings
// `APP-CONVENTION-REFERENCE` §3.4 admits a raw string is written in, and
// whether it addresses anything inside this system at all.
//
// # The defect this file exists to fix
//
// [AssetNameFromRef] answers exactly one question — *"is this a
// directory-relative ref naming a file in this site's own `assets/`
// subgraph"* — and every caller read its `false` as *"refused for
// security"*, because that is what its own documentation says it means.
// Three of §3.4's four forms fall into that `false`:
//
//	entity+ref://{peer}/sites/lab/assets/x.png   caught by the `://` arm
//	/assets/figures/x.png                        caught by the leading-`/` arm
//	site:other-site/assets/x.png                 never reached at all
//
// Each is a form the convention says MUST resolve, declined by a
// function reporting a security property. That is AP44's shape — *a
// false refusal reads as rigor and leaves no wrong answer to catch* —
// in its more durable form, because a refusal phrased as protection is
// the one nobody goes back and questions.
//
// # The split, which is the whole design
//
// One predicate answers *can this system resolve this at all*
// ([ClassifyRefForm], §3.4's forms). A second answers *does this leave
// the system* ([RefLeavesSystem]: another scheme, protocol-relative,
// `data:`), and it is **defined off the first** rather than beside it,
// so the two cannot drift into disagreeing about one string.
//
// The refusals are kept exactly as they were. `https://tracker/x.png`,
// `//tracker/x.png` and `data:…` are still how a hostile page body
// steers a renderer at a URL the publisher never committed, and every
// one of them still refuses. What changes is that they stop catching
// bystanders.
//
// # What did NOT change, deliberately
//
// [AssetNameFromRef] is untouched and stays byte-identical with
// `entity-browser-rust`'s `content_site/paths.rs::asset_name_from_ref`.
// It is Layer-2 algorithm contract, its vectors are lifted verbatim in
// site_asset_crossimpl_test.go, and a divergence there is routed rather
// than locally corrected. It is now the **relative arm** of
// [ClassifyAssetRef] — one voice among four instead of the only one —
// and it is reused by the other three arms for the containment check, so
// there is one implementation of *"is this inside the site's assets"* and
// not four.
//
// The base for an asset ref is the **site root**, not the referring
// page's directory: `assets/figures/x.png` names the same bytes from
// every page in the site, which is what the live corpus publishes and
// what the reference implementation resolves. So the root-absolute form
// `/assets/figures/x.png` resolves identically to the relative one, and
// a `..` segment in an asset ref can only ever escape — which is why the
// blanket `..` refusal is correct *here* while `REF-V9` requires it not
// be applied at the link position, where [resolveInSitePage] consumes
// dot segments with root clamping.

// The schemes this system answers for. Scheme names are compared
// case-insensitively (§3.3, `REF-R11`); everything after the scheme,
// the authority above all, is compared verbatim.
const (
	refSchemeEntityRef = "entity+ref"
	refSchemeEntity    = "entity"
	refSchemeSite      = "site"
)

// SiteRefScheme is §3.4's same-peer short form: `site:{site-id}/{rest}`
// names something in a different site on the **implied** authority, the
// current peer. It is opaque — no `//`, no authority component, because
// it has none to carry.
const SiteRefScheme = refSchemeSite + ":"

// legacyDispatchScheme is `ENTITY-CORE-PROTOCOL` §1.4's wire dispatch
// scheme. §4 forbids a producer emitting it in a link position
// (`REF-R18`) and asks a consumer to keep resolving it (`REF-R19`),
// which is the asymmetry that stops the ambiguous population growing
// without breaking every document already published.
const legacyDispatchScheme = refSchemeEntity + "://"

// RefForm names which of §3.4's spellings a raw reference string is
// written in.
type RefForm int

const (
	// RefFormExternal — the string addresses something outside this
	// system, or parses as none of the forms below. §3.4 makes those the
	// same answer on purpose: *"a tolerant re-anchoring scan produces a
	// well-formed wrong location and cannot report that it did"*
	// (`REF-R20`).
	RefFormExternal RefForm = iota
	// RefFormRelative — a bare string, resolved against the referring
	// entity's own location. The overwhelmingly common case.
	RefFormRelative
	// RefFormRootAbsolute — a leading `/`: root-absolute within the
	// current site. §3.4 SHOULDs this form for application-generated
	// links, so that one resolves identically from whatever page it is
	// rendered on.
	RefFormRootAbsolute
	// RefFormSite — `site:{site-id}/{rest}`, the same-peer short form.
	RefFormSite
	// RefFormEntityRef — `entity+ref://{peer}/…`, the absolute form and
	// the one a conformant producer emits in a link position
	// (`REF-R17`).
	RefFormEntityRef
	// RefFormLegacyDispatch — `entity://{peer}/…`. Resolved, and a
	// surface that resolves one SHOULD say so (`REF-R19`), which is why
	// this is its own form rather than folded into RefFormEntityRef.
	RefFormLegacyDispatch
)

// String names the form for a diagnostic. A refusal that cannot say
// which form it refused is the thing this file is fixing.
func (f RefForm) String() string {
	switch f {
	case RefFormRelative:
		return "relative"
	case RefFormRootAbsolute:
		return "root-absolute"
	case RefFormSite:
		return "site:"
	case RefFormEntityRef:
		return "entity+ref://"
	case RefFormLegacyDispatch:
		return "entity:// (legacy dispatch form)"
	default:
		return "external"
	}
}

// Resolvable reports whether a reference of this form addresses
// something this system can go and look for.
func (f RefForm) Resolvable() bool { return f != RefFormExternal }

// ClassifyRefForm reports which of §3.4's forms `ref` is written in.
//
// It answers a question about the STRING and never about the world: a
// resolvable form may still name a site nobody published, and that is a
// different answer arriving from a different layer. Keeping the two
// apart is the point — collapsing them is how *"this system has no route
// to that peer"* came to be reported as *"that reference is hostile"*.
//
// Order matters and is not stylistic. `entity+ref://` is tested before
// anything generic about `://`, because the two entity schemes share a
// prefix and a classifier that reaches a generic test first turns the
// current form into precisely the outcome `REF-R20` forbids — a
// well-formed wrong in-site slug, produced in silence.
func ClassifyRefForm(ref string) RefForm {
	r := strings.TrimSpace(ref)
	if r == "" {
		return RefFormExternal
	}
	// Protocol-relative. It carries no scheme of its own and inherits
	// the reader's, so it is an arbitrary origin wearing no scheme name.
	if strings.HasPrefix(r, "//") {
		return RefFormExternal
	}
	if i := refSchemeEnd(r); i > 0 {
		switch strings.ToLower(r[:i]) {
		case refSchemeEntityRef:
			// §3.1: the authority is mandatory, and the scheme is spelled
			// with `//` because it has one. `entity+ref:sites/x` is not
			// the scheme at all — and re-anchoring it as a relative path
			// would be the guess `REF-R20` rules out.
			if hasPrefixFold(r, entitysdk.RefScheme) {
				return RefFormEntityRef
			}
			return RefFormExternal
		case refSchemeEntity:
			if entitysdk.RefIsLegacyDispatchURI(r) {
				return RefFormLegacyDispatch
			}
			return RefFormExternal
		case refSchemeSite:
			return RefFormSite
		}
		// Any other scheme — `https:`, `data:`, `mailto:`, `javascript:`
		// — leaves the system. Note this covers a bare relative string
		// whose FIRST segment contains a colon, which RFC 3986 §4.2 says
		// cannot be used as a relative-path reference for exactly this
		// reason: it is indistinguishable from a scheme.
		return RefFormExternal
	}
	if strings.HasPrefix(r, "/") {
		return RefFormRootAbsolute
	}
	return RefFormRelative
}

// RefLeavesSystem reports whether a reference string addresses something
// outside this system — an arbitrary origin, inline bytes, another
// application's scheme — or parses as none of §3.4's forms.
//
// This is the second half of the split, and it is deliberately DERIVED
// from [ClassifyRefForm] rather than written beside it. Two predicates
// maintained independently are two chances to disagree about one string,
// and the disagreement would be invisible: each would be individually
// defensible and the pair would admit something neither meant to.
func RefLeavesSystem(ref string) bool {
	return ClassifyRefForm(ref) == RefFormExternal
}

// AssetRefRefusal says why a reference resolves to no asset. Zero value
// means it resolved.
type AssetRefRefusal int

const (
	// AssetRefOK — the reference names an asset.
	AssetRefOK AssetRefRefusal = iota
	// AssetRefLeavesSystem — an arbitrary origin, protocol-relative
	// bytes, `data:`, another scheme. This is the security refusal and
	// it is the ONLY one of the four that is: a page body must not be
	// able to make this process fetch something the publisher never
	// committed.
	AssetRefLeavesSystem
	// AssetRefOutsideSubgraph — a resolvable form addressing something
	// that is not an asset of any site: not under `assets/`, or a `..`
	// segment escaping the subgraph.
	AssetRefOutsideSubgraph
	// AssetRefPinned — a well-formed pinned reference. It names bytes by
	// hash rather than a place in a site, so there is no asset name to
	// derive; resolving one is content-addressed retrieval, which this
	// position does not do. Refused, and named, rather than guessed at.
	AssetRefPinned
	// AssetRefMalformed — the right shape and it does not parse.
	AssetRefMalformed
)

// String names the refusal for a diagnostic.
func (r AssetRefRefusal) String() string {
	switch r {
	case AssetRefOK:
		return "resolved"
	case AssetRefLeavesSystem:
		return "the reference leaves the entity system"
	case AssetRefOutsideSubgraph:
		return "the reference addresses nothing in a site's assets subgraph"
	case AssetRefPinned:
		return "a pinned reference names bytes, not a place in a site"
	default:
		return "the reference is malformed"
	}
}

// AssetRef is where an embed's `ref` points, once classified.
//
// PeerID and SiteID are already defaulted from the referring location
// for the forms that inherit them, so a caller never re-derives the
// inheritance rule — which is the kind of thing two call sites get
// subtly different.
type AssetRef struct {
	// Form is which spelling the publisher wrote. Carried on the result
	// so a surface can say it resolved a legacy `entity://`
	// (`REF-R19`'s *name your own provenance*) without the caller having
	// to remember which branch it took.
	Form RefForm
	// PeerID / SiteID address the site holding the asset.
	PeerID string
	SiteID string
	// Name is the site-local asset name — `figures/x.png`, the suffix
	// after `assets/`, exactly what [AssetPath] takes.
	Name string
	// Refusal says why there is no Name. Zero value when there is one.
	Refusal AssetRefRefusal
}

// ClassifyAssetRef resolves an embed reference at the ASSET position,
// across all four of §3.4's forms, relative to the page it was read
// from.
//
// It decides *where the bytes would be*, never *whether anyone has
// them*: a resolver bound to one publisher's signed root answers the
// second question, and it needs the first one answered honestly to do
// it. An [AssetRef] naming another peer is not a refusal — it is a
// destination this resolver cannot reach, which is a different sentence
// and belongs to whoever can say it.
func ClassifyAssetRef(ref string, cur Location) (AssetRef, bool) {
	r := strings.TrimSpace(ref)
	out := AssetRef{Form: ClassifyRefForm(r), PeerID: cur.PeerID, SiteID: cur.SiteID}

	switch out.Form {
	case RefFormExternal:
		out.Refusal = AssetRefLeavesSystem
		if r == "" {
			out.Refusal = AssetRefMalformed
		}

	case RefFormRelative:
		out.Name, out.Refusal = assetNameOrRefusal(r)

	case RefFormRootAbsolute:
		// Root-absolute WITHIN THE SITE. The base of an asset ref is the
		// site root either way, so this is the same address as the
		// relative form and resolves to the same bytes — which is what
		// makes the old refusal a pure loss. `/etc/passwd` still refuses,
		// on the containment rule that was doing the work all along.
		out.Name, out.Refusal = assetNameOrRefusal(strings.TrimPrefix(r, "/"))

	case RefFormSite:
		siteID, rest := splitFirstSlash(r[len(SiteRefScheme):])
		if siteID == "" {
			out.Refusal = AssetRefMalformed
			break
		}
		out.SiteID = siteID
		out.Name, out.Refusal = assetNameOrRefusal(rest)

	case RefFormEntityRef:
		er, err := entitysdk.ParseRefURI(r)
		if err != nil {
			out.Refusal = AssetRefMalformed
			break
		}
		out.PeerID = er.Peer
		if er.IsPinned() {
			out.Refusal = AssetRefPinned
			break
		}
		out.SiteID, out.Name, out.Refusal = assetInPeerPath(er.Path)

	case RefFormLegacyDispatch:
		peer, tail := splitFirstSlash(r[len(legacyDispatchScheme):])
		if peer == "" {
			out.Refusal = AssetRefMalformed
			break
		}
		out.PeerID = peer
		out.SiteID, out.Name, out.Refusal = assetInPeerPath(tail)
	}

	return out, out.Refusal == AssetRefOK
}

// assetNameOrRefusal is the containment check, and there is one of it.
//
// Every arm above funnels through [AssetNameFromRef], so *"is this
// inside the site's assets subgraph"* has a single implementation — the
// one that is byte-identical with the reference implementation and
// carries its vectors.
func assetNameOrRefusal(rest string) (string, AssetRefRefusal) {
	name, ok := AssetNameFromRef(rest)
	if !ok {
		return "", AssetRefOutsideSubgraph
	}
	return name, AssetRefOK
}

// assetInPeerPath reads `…/sites/{site-id}/assets/{name}` out of a
// peer-relative tree path.
func assetInPeerPath(path string) (siteID, name string, refusal AssetRefRefusal) {
	site, rest, ok := refSiteSubpath(path)
	if !ok {
		return "", "", AssetRefOutsideSubgraph
	}
	name, refusal = assetNameOrRefusal(rest)
	return site, name, refusal
}

// refSiteSubpath locates the `sites/{site-id}` pair in a peer-relative
// tree path and returns the site id with everything after it.
//
// Tolerant about what precedes `sites`, for the same reason
// [parseEntityURI] is: the pre-v0.5 emission put sites under
// `content/sites/…`, those documents are published and unreachable by
// their authors, and a reader that insists on the current layout stops
// resolving them for no gain.
func refSiteSubpath(path string) (siteID, rest string, ok bool) {
	segs := splitNonEmpty(path, "/")
	for i, s := range segs {
		if s != SitesSubpath {
			continue
		}
		if i+1 >= len(segs) {
			return "", "", false
		}
		return segs[i+1], strings.Join(segs[i+2:], "/"), true
	}
	return "", "", false
}

// refSchemeEnd returns the index of the `:` terminating an RFC 3986 §3.1
// scheme at the head of s, or -1 when s does not begin with one.
//
// The colon must precede any `/`: `assets/a:b.png` carries a colon in a
// path segment and is not scheme-bearing, while `data:image/png` is. A
// scheme must begin with ALPHA, so `2:x` is not one either.
func refSchemeEnd(s string) int {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == ':':
			return i // i == 0 is an empty scheme; the caller tests i > 0
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
			// Always legal, in any position.
		case c >= '0' && c <= '9', c == '+', c == '-', c == '.':
			if i == 0 {
				return -1 // a scheme MUST begin with ALPHA
			}
		default:
			return -1
		}
	}
	return -1
}

// hasPrefixFold is HasPrefix with an ASCII-case-insensitive comparison
// of the prefix only.
//
// **Only the prefix.** §3.3 makes the scheme case-insensitive and the
// authority CASE-SENSITIVE, and calls mixing the two up *"the single
// most likely implementation error"* — a peer id that survives a
// host-normalizing parser names a different peer and fails as a clean
// 404 at a well-formed address.
func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}
