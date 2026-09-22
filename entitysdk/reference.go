package entitysdk

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"go.entitychurch.org/entity-core-go/core/hash"
)

// reference.go — `APP-CONVENTION-REFERENCE` v0.1: the reference atom (§2.1) and
// its string form (§3).
//
// **This convention mints no entity type** (§6.3). It is one shape for *"this
// points at that"*, carried inside other conventions' types — `APP-CONVENTION-EMBED`
// §3's pointer slots and `APP-CONVENTION-FEED`'s `reply` / `context` /
// `attachments`. It adds no kernel feature and no wire form; the only thing it
// asks of the substrate is content addressing.
//
// **It is FIRST in the build order and that order is forced, not preferred.**
// EMBED's `child` payload arm carries an `entity-ref`, so it is unwritable until
// this exists, and FEED §2.3 declares `body: embed-node`. `entity-browser-rust`
// measured the consequence and it is worth restating because it is the sizing
// error we would otherwise have repeated: *"a plan that sizes FEED by analogy
// with the site convention will be wrong — the site convention imports no atoms
// and FEED imports two."*
//
// # This is a shared shape, and every encoding choice below is matched deliberately
//
// The bytes and the strings must equal `entity-browser-rust`'s. They shipped
// first (`src/entity_ref.rs`), so where §3 leaves an encoding unspecified their
// choice is the baseline and ours follows it — not because it is better, but
// because *whatever publishes first becomes the corpus* and two seats each
// picking reasonably is how a family diverges with nothing to catch it.
// Routed to us as `W-5` in
// `ROUTING-2026-09-10-i-workbench-go-THE-FEED-TYPES-ARE-BUILT-OUR-SIDE-…` §4;
// accepted in `ROUTING-2026-09-10-f-…` §5. §3.2's losslessness MUST names no
// encoding for `via`, for the `at` fragment, or for query parameter ORDER, so
// all three are conventions we are inheriting rather than deriving:
//
//   - `via={tag}:{value}`, each half percent-encoded as a component
//   - the anchor `/`-joined percent-encoded field names
//   - `hash` / `seen` first in the query, then `via` in list order
//   - `hash` / `seen` values are the **invariant-pointer hex** — the full wire
//     form with the format-code byte, `ENTITY-CORE-PROTOCOL` §3.5. That one they
//     did not invent and neither did we: it is the spelling the corpus already
//     publishes, and the same form `EXTENSION-CONTENT` §6.4.2's `{hex(H)}` uses.
//
// If arch rules differently on any of them (their `A-33`/`A-34`/`A-36`), one
// file moves on each side and neither seat is the corpus alone.

// The atom's discriminator values (§2.1). **There is no untagged form.**
const (
	// RefTagPin names *these exact bytes*. Self-verifying: anyone holding
	// them satisfies it, and the answer can never change.
	RefTagPin = "pin"
	// RefTagLive names *whatever is at this address now*. Only the publisher
	// is authoritative, and the answer is expected to change.
	RefTagLive = "live"
)

// RefScheme is §3.1's scheme, with the `//` that says it has an authority.
//
// **`entity://` is the wire DISPATCH scheme** (`ENTITY-CORE-PROTOCOL` §1.4) and
// following a link is not dispatching to a handler, so §4 makes emitting
// `entity://` in a link position a MUST NOT once an implementation emits the
// atom at all. Consumers SHOULD keep resolving it — see RefIsLegacyDispatchURI.
const RefScheme = "entity+ref://"

// The four hint kinds (§2.3). All four answer one question: *where might I find
// this?* **An unknown tag is IGNORED, never an error** — see RefHint.
const (
	RefHintOrigin = "origin" // where the publisher serves bytes from
	RefHintMirror = "mirror" // somewhere else that had them
	RefHintPeer   = "peer"   // somebody likely to hold them
	RefHintPath   = "path"   // where in the publisher's tree it was placed
)

// Errors. **Distinguished rather than collapsed**, because §2.2, §3.1 and §3.3
// each refuse for a different reason and a caller acts differently on each:
// ErrRefNotAReferenceURI is a *classification* (the string is an external link,
// REF-R20), a shape violation is an authoring bug, and a bad escape is a
// transport or copy-paste fault.
//
// **ErrRefShapeExcludesField is kept apart from ErrRefMalformed on purpose**,
// matching browser-rust's third `W-5` choice: §3.1 refuses the string form of a
// `"pin"` carrying a path and §2.1 is silent about the atom, so refusing it is a
// *schema* judgement we are making in step with them, and a reader needs to be
// able to tell that from bytes that did not decode.
var (
	ErrRefNotAReferenceURI  = errors.New("not an entity+ref:// reference")
	ErrRefNoAuthority       = errors.New("reference has no authority (peer id)")
	ErrRefBothIdentityTerms = errors.New("reference carries both a hash and a path")
	ErrRefNoIdentityTerm    = errors.New("reference carries neither a hash nor a path")
	ErrRefEmptyParam        = errors.New("query parameter is present with an empty value")
	ErrRefDotSegment        = errors.New("absolute-form path contains a . or .. segment")
	ErrRefBadEscape         = errors.New("malformed percent-escape")
	ErrRefBadHash           = errors.New("parameter is not a content hash")
	ErrRefTagMismatch       = errors.New("reference tag and identity term disagree")
	ErrRefShapeExcludes     = errors.New("reference carries a field its shape excludes")
	ErrRefUnknownTag        = errors.New("reference tag is an intent this build does not implement")
	ErrRefMalformed         = errors.New("malformed reference")
)

// RefHint is one `via` entry (§2.3): advisory, ordered, droppable.
//
// **An unknown Tag is carried and never acted on.** REF-R7 and REF-R8 pull in
// the same direction here — an unknown kind must be *ignored* rather than
// refused, and a re-serializer that dropped it would *"silently rewrite other
// people's references"* (§3.2). So Tag is a plain string with a Known()
// predicate rather than an enum with no room in it, and the ignoring happens at
// resolution (RefActionableHints) rather than at parse.
type RefHint struct {
	Tag   string `cbor:"tag"`
	Value string `cbor:"value"`
}

// Known reports whether this build understands the hint kind.
func (h RefHint) Known() bool {
	switch h.Tag {
	case RefHintOrigin, RefHintMirror, RefHintPeer, RefHintPath:
		return true
	}
	return false
}

// RefAnchor is §2.4's `at` — a path of field names into the referenced entity.
// **Absent means the whole entity**, so nothing changes for a consumer that
// does not use it.
//
// `Field` is an OPAQUE sequence of names in v0.1 and this build must not read
// meaning into it. §2.4: the core's address primitives are content hash, tree
// path, type name and peer id, and an intra-entity field path is none of them —
// *"a slot with no consumer does not mint a grammar."*
type RefAnchor struct {
	Field []string `cbor:"field"`
}

// EntityRef is §2.1's atom.
//
// The CDDL declares two maps sharing four terms. One Go struct with `omitempty`
// on the arm-specific fields encodes byte-identically to either arm, and
// Validate is what enforces that exactly one arm is inhabited — a shape a type
// system cannot express here has to be a check, and the check has to be called.
//
// Four terms, and only the first two are ever required:
//
//   - **who** — Peer. A reference is routable if and only if it names one;
//     bytes alone name no holder you can go and ask.
//   - **what** — Hash or Path. *Which of the two it is IS the intent*, and §1.1
//     is about why that is not a formatting detail: a consumer that collapses
//     them cannot express *"the version I read"* and *"the current version"* as
//     different things, and both are needed.
//   - **which part** — At. Optional; absent is the whole entity.
//   - **where to look first** — Via. Optional, advisory, droppable.
type EntityRef struct {
	Tag  string     `cbor:"tag"`
	Peer string     `cbor:"peer"`
	Hash *hash.Hash `cbor:"hash,omitempty"` // pin: identity AND expectation
	Path string     `cbor:"path,omitempty"` // live: the address of record
	Seen *hash.Hash `cbor:"seen,omitempty"` // live: what the linker saw — an EXPECTATION only
	At   *RefAnchor `cbor:"at,omitempty"`
	Via  []RefHint  `cbor:"via,omitempty"`
}

// PinnedRef builds a reference to exact bytes.
func PinnedRef(peer string, h hash.Hash) EntityRef {
	return EntityRef{Tag: RefTagPin, Peer: peer, Hash: &h}
}

// LiveRef builds a reference to an address of record. `seen` is optional — pass
// the zero Hash to omit it.
//
// **The `seen` hash is an expectation and never the identity.** A consumer that
// resolves the path to different bytes has not failed; it has learned the thing
// changed, and §4's *name your own provenance* obligation says it must surface
// that rather than silently serve either one.
func LiveRef(peer, path string, seen hash.Hash) EntityRef {
	r := EntityRef{Tag: RefTagLive, Peer: peer, Path: refAbsolutePath(path)}
	if !seen.IsZero() {
		s := seen
		r.Seen = &s
	}
	return r
}

// WithHints returns a copy carrying `via`, in descending confidence.
func (r EntityRef) WithHints(hints ...RefHint) EntityRef {
	r.Via = append(append([]RefHint(nil), r.Via...), hints...)
	return r
}

// WithAnchor returns a copy addressing one field path inside the referent.
func (r EntityRef) WithAnchor(field ...string) EntityRef {
	r.At = &RefAnchor{Field: append([]string(nil), field...)}
	return r
}

// IsPinned / IsLive read the TAG. **Never infer the intent from which of Hash /
// Path is populated** (§2.2): the discriminator is a value, not a scan, so a
// malformed atom fails at the first field instead of only at a reader that
// happens to implement the presence rule.
func (r EntityRef) IsPinned() bool { return r.Tag == RefTagPin }

// IsLive reports whether this is a live reference. See IsPinned.
func (r EntityRef) IsLive() bool { return r.Tag == RefTagLive }

// RefActionableHints returns the hints this build may act on — the known kinds,
// in order.
//
// **This is where REF-R7's "ignore" happens, and it is deliberately not at
// parse.** Dropping an unknown hint on decode would satisfy "ignore" and break
// REF-R8's lossless round-trip in the same move, so the atom keeps everything
// and resolution filters. A resolver MUST reach the same answer using these as
// it would using none, or fail (REF-R5).
func RefActionableHints(hints []RefHint) []RefHint {
	out := make([]RefHint, 0, len(hints))
	for _, h := range hints {
		if h.Known() {
			out = append(out, h)
		}
	}
	return out
}

// Validate enforces §2.1's shape and §2.2's tag discipline.
//
// **A tag does not make an identity term optional.** §2.2: were the hash merely
// optional, a reference arriving without one would be indistinguishable between
// *the author wants the live version*, *the author's implementation did not
// populate it*, and *the author only ever had an address* — one intent and two
// defects, with nothing to separate them.
func (r EntityRef) Validate() error {
	if r.Peer == "" {
		// §1: routable if and only if it names a publisher.
		return fmt.Errorf("%w: reference names no peer", ErrRefMalformed)
	}
	switch r.Tag {
	case RefTagPin:
		if r.Hash == nil || r.Hash.IsZero() {
			return fmt.Errorf("%w: a %q reference with no hash (REF-R2)", ErrRefTagMismatch, RefTagPin)
		}
		// §3.1 refuses the string form of a pin carrying a path and §2.1 is
		// silent about the atom. Refused here as a SCHEMA violation, kept apart
		// from a decode failure — matched with browser-rust rather than decided
		// alone, because "one field, one job" (§2.3) is the whole reason `path`
		// is authoritative only where it is the identity term.
		if r.Path != "" {
			return fmt.Errorf("%w: a %q reference carrying path (§2.3: a path on a pinned reference is a hint and belongs in via)", ErrRefShapeExcludes, RefTagPin)
		}
		if r.Seen != nil {
			return fmt.Errorf("%w: a %q reference carrying seen (identity and expectation already coincide)", ErrRefShapeExcludes, RefTagPin)
		}
	case RefTagLive:
		if !refIsAddressablePath(r.Path) {
			return fmt.Errorf("%w: a %q reference with no path (REF-R2)", ErrRefTagMismatch, RefTagLive)
		}
		if r.Hash != nil {
			return fmt.Errorf("%w: a %q reference carrying hash (its identity is the path; what the linker saw is seen)", ErrRefShapeExcludes, RefTagLive)
		}
	default:
		return fmt.Errorf("%w: %q", ErrRefUnknownTag, r.Tag)
	}
	for _, h := range r.Via {
		if h.Tag == "" {
			return fmt.Errorf("%w: a via hint with no tag", ErrRefMalformed)
		}
	}
	return nil
}

// --- §3, the string form ---

// URI renders §3.1's string form.
//
// Fallible, and the reason is worth stating: the atom can hold a `path` the
// *string* form must refuse. The CDDL does not exclude a dot segment from a
// `tree-path` and §3.3 does exclude it from an absolute-form URI, so the
// refusal belongs on this projection rather than on the type.
//
// Query order is `hash` / `seen` first, then `via` in list order — §3.2's
// losslessness MUST names no order, so this one is inherited (see the file
// header) and is load-bearing for byte-identity across seats.
func (r EntityRef) URI() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(RefScheme)
	// **The authority is a peer id and is CASE-SENSITIVE** (§3.3, REF-R12):
	// written verbatim, never normalized. This is the single most likely
	// implementation error in this file — a general URL library lowercases the
	// host by default, and a peer id that survives such a parser names a
	// DIFFERENT peer, failing as a clean 404 at a well-formed address rather
	// than as a parse error. REF-V3 exists because of it.
	b.WriteString(r.Peer)

	var query []string
	switch r.Tag {
	case RefTagPin:
		// §3.1: a pinned reference has no path-shaped identity, so the path is
		// empty and the hash is a required query parameter. That reads oddly
		// and it is the honest encoding — putting the hash in the path would
		// make the two shapes structurally indistinguishable to a generic URI
		// parser and hand the discriminator back to a scan.
		b.WriteByte('/')
		query = append(query, "hash="+refHashParam(*r.Hash))
	case RefTagLive:
		if err := refRefuseDotSegments(r.Path); err != nil {
			return "", err
		}
		b.WriteString(refEncodePath(r.Path))
		if r.Seen != nil {
			query = append(query, "seen="+refHashParam(*r.Seen))
		}
	}
	for _, h := range r.Via {
		query = append(query, "via="+refEncodeComponent(h.Tag)+":"+refEncodeComponent(h.Value))
	}
	if len(query) > 0 {
		b.WriteByte('?')
		b.WriteString(strings.Join(query, "&"))
	}
	if r.At != nil {
		b.WriteByte('#')
		parts := make([]string, 0, len(r.At.Field))
		for _, f := range r.At.Field {
			parts = append(parts, refEncodeComponent(f))
		}
		b.WriteString(strings.Join(parts, "/"))
	}
	return b.String(), nil
}

// ParseRefURI parses §3.1's string form.
//
// Returns ErrRefNotAReferenceURI for anything that is not an `entity+ref://`
// string. **That is a classification and not a fault** (REF-R20): a caller
// resolving a link body treats it as leaving the system, because a tolerant
// re-anchoring scan produces a well-formed wrong location and cannot report
// that it did.
//
// **The tag is recovered from the string without a lookup table** (§3.1):
// `hash` present ⇒ pin, non-empty path ⇒ live. Both ⇒ refuse (REF-R9), neither
// ⇒ refuse (REF-R10). Those two refusals are what keep the discriminator a rule
// rather than a convention.
func ParseRefURI(s string) (EntityRef, error) {
	rest, ok := refStripScheme(s)
	if !ok {
		return EntityRef{}, ErrRefNotAReferenceURI
	}

	// Fragment before query. **Order matters**: a `#` appearing before a `?`
	// makes the `?` part of the fragment, per RFC 3986 §3. Splitting on `?`
	// first would silently move an anchor's contents into the query.
	beforeFragment, fragment := rest, ""
	hasFragment := false
	if i := strings.IndexByte(rest, '#'); i >= 0 {
		beforeFragment, fragment, hasFragment = rest[:i], rest[i+1:], true
	}
	authorityAndPath, rawQuery := beforeFragment, ""
	hasQuery := false
	if i := strings.IndexByte(beforeFragment, '?'); i >= 0 {
		authorityAndPath, rawQuery, hasQuery = beforeFragment[:i], beforeFragment[i+1:], true
	}

	peer, rawPath := authorityAndPath, ""
	if i := strings.IndexByte(authorityAndPath, '/'); i >= 0 {
		peer, rawPath = authorityAndPath[:i], authorityAndPath[i:]
	}
	if peer == "" {
		// §3.1: the authority is mandatory, and it is the term that makes the
		// reference routable at all.
		return EntityRef{}, ErrRefNoAuthority
	}

	var (
		h, seen *hash.Hash
		via     []RefHint
	)
	if hasQuery {
		for _, param := range strings.Split(rawQuery, "&") {
			name, value, found := strings.Cut(param, "=")
			// REF-R16: an absent parameter and one present with an empty value
			// are different things, and the second is malformed.
			if !found || value == "" {
				return EntityRef{}, fmt.Errorf("%w: %q", ErrRefEmptyParam, name)
			}
			switch name {
			case "hash", "seen":
				parsed, err := hash.ParseHex(value)
				if err != nil {
					return EntityRef{}, fmt.Errorf("%w: %q: %v", ErrRefBadHash, name, err)
				}
				if name == "hash" {
					h = &parsed
				} else {
					seen = &parsed
				}
			case "via":
				tag, val, ok := strings.Cut(value, ":")
				if !ok {
					return EntityRef{}, fmt.Errorf("%w: via hint %q is not tag:value", ErrRefMalformed, value)
				}
				// Split on the RAW `:` then decode each half — `:` is outside
				// the unreserved set, so a literal one inside either half is
				// `%3A` and the split is unambiguous.
				dt, err := refDecode(tag)
				if err != nil {
					return EntityRef{}, err
				}
				dv, err := refDecode(val)
				if err != nil {
					return EntityRef{}, err
				}
				via = append(via, RefHint{Tag: dt, Value: dv})
			default:
				// Unknown parameter: MUST-ignore, the substrate's own
				// discipline for unknown fields. Refusing here would make the
				// query component closed, which nothing says it is.
			}
		}
	}

	pathIsIdentity := refIsAddressablePath(rawPath)
	switch {
	case h != nil && pathIsIdentity:
		return EntityRef{}, ErrRefBothIdentityTerms
	case h == nil && !pathIsIdentity:
		return EntityRef{}, ErrRefNoIdentityTerm
	}

	ref := EntityRef{Peer: peer, Via: via}
	if h != nil {
		ref.Tag = RefTagPin
		ref.Hash = h
		if seen != nil {
			return EntityRef{}, fmt.Errorf("%w: a %q reference carrying seen", ErrRefShapeExcludes, RefTagPin)
		}
	} else {
		ref.Tag = RefTagLive
		if err := refRefuseDotSegments(rawPath); err != nil {
			return EntityRef{}, err
		}
		decoded, err := refDecodePath(rawPath)
		if err != nil {
			return EntityRef{}, err
		}
		ref.Path = decoded
		ref.Seen = seen
	}

	if hasFragment {
		// An empty fragment is not an anchor. `#` with nothing after it would
		// otherwise round-trip to a one-element field list containing "",
		// which is a different atom.
		if fragment == "" {
			return EntityRef{}, fmt.Errorf("%w: empty fragment", ErrRefMalformed)
		}
		fields := make([]string, 0, 2)
		for _, seg := range strings.Split(fragment, "/") {
			d, err := refDecode(seg)
			if err != nil {
				return EntityRef{}, err
			}
			fields = append(fields, d)
		}
		ref.At = &RefAnchor{Field: fields}
	}
	return ref, ref.Validate()
}

// RefIsLegacyDispatchURI reports whether a link-position string uses the wire
// dispatch scheme.
//
// §4's asymmetry is the mechanism and it is worth keeping visible: a **MUST**
// on the producer stops the population of ambiguous strings growing, while a
// **SHOULD** on the consumer keeps every already-published document resolving.
// **No flag day is declared** — when the ambiguous form stops being accepted is
// a decision for the parties holding the corpus of published documents.
//
// A consumer that resolves one SHOULD surface that it did (REF-R19) — the same
// *name your own provenance* obligation as a live reference resolving to
// something other than what the linker saw.
func RefIsLegacyDispatchURI(s string) bool {
	return strings.HasPrefix(strings.ToLower(s), "entity://")
}

// --- internals ---

// refStripScheme accepts the scheme case-insensitively (REF-R11) while leaving
// everything after it untouched — the authority must survive verbatim.
func refStripScheme(s string) (string, bool) {
	if len(s) < len(RefScheme) {
		return "", false
	}
	if !strings.EqualFold(s[:len(RefScheme)], RefScheme) {
		return "", false
	}
	return s[len(RefScheme):], true
}

// refHashParam is the invariant-pointer hex — the FULL wire form with the
// format-code byte included (`ENTITY-CORE-PROTOCOL` §3.5), not the digest-only
// form. 66 chars beginning `00` under ECFv1-SHA-256, 98 beginning `01` under
// SHA-384; the length is implied by the leading code and is never assumed.
//
// `hash.ParseHex` is the exact inverse and fails closed when a string's length
// disagrees with its own format byte, which rejects the 64-char digest-only
// form as well as a mislabelled one.
func refHashParam(h hash.Hash) string { return hex.EncodeToString(h.Bytes()) }

// refIsAddressablePath is the ONE predicate for *"is this path an identity"*,
// shared by the emitter, the parser and Validate. A caller can still build a
// live ref with `Path: "/"` by hand; this is where that stops, so the three
// cannot disagree about what an address is.
func refIsAddressablePath(p string) bool { return p != "" && p != "/" }

// refAbsolutePath normalizes a constructor's path to the leading-slash form the
// string projection emits, so an atom built from a peer-relative path and one
// built from the absolute form are equal (§3.2's second direction).
func refAbsolutePath(p string) string {
	if p == "" || strings.HasPrefix(p, "/") {
		return p
	}
	return "/" + p
}

// refRefuseDotSegments is REF-R15, and **only for the absolute form.**
//
// The scoping is load-bearing: §3.4's relative form is directory-relative, so
// `..` is both meaningful and expected there. Relative resolution consumes the
// dot segments and what it produces is an absolute reference in which none
// survive — a reader that applies this refusal to an unresolved relative string
// rejects ordinary correct links, which is what REF-V9 is the arm against.
func refRefuseDotSegments(path string) error {
	for _, seg := range strings.Split(path, "/") {
		if seg == "." || seg == ".." {
			return fmt.Errorf("%w: %q", ErrRefDotSegment, path)
		}
	}
	return nil
}

// refEncodePath percent-encodes a `/`-joined path segment by segment (REF-R14).
// Reserved characters *within* a segment are encoded; the `/` delimiter never
// is.
func refEncodePath(path string) string {
	segs := strings.Split(path, "/")
	for i, s := range segs {
		segs[i] = refEncodeComponent(s)
	}
	return strings.Join(segs, "/")
}

// refDecodePath decodes segment by segment. **Comparison is after
// percent-decoding** (REF-R13) — an implementation must not compare raw — so
// the atom holds the decoded form and the string holds the encoded one.
func refDecodePath(path string) (string, error) {
	segs := strings.Split(path, "/")
	for i, s := range segs {
		d, err := refDecode(s)
		if err != nil {
			return "", err
		}
		segs[i] = d
	}
	return strings.Join(segs, "/"), nil
}

// refEncodeComponent escapes one component — a single path segment, a query
// value, one anchor field name — so a delimiter inside a component is data
// rather than structure. **A `/` IS encoded here**; the caller joins.
//
// Deliberately NOT `net/url`. `url.QueryEscape` encodes a space as `+`, which
// is `application/x-www-form-urlencoded` and not RFC 3986, and `url.PathEscape`
// leaves sub-delimiters literal. Either would produce a string that is
// well-formed, decodes to the right value, and **is not byte-identical to the
// other implementation's** — which REF-R8 makes a conformance failure rather
// than a cosmetic difference.
//
// The set is RFC 3986 §2.3's *unreserved*: ALPHA / DIGIT / `-` `.` `_` `~`,
// with UPPERCASE hex escapes (§6.2.2.1 prefers uppercase).
func refEncodeComponent(s string) string {
	if refAllUnreserved(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if refIsUnreserved(c) {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(refUpperHex[c>>4])
		b.WriteByte(refUpperHex[c&0x0f])
	}
	return b.String()
}

const refUpperHex = "0123456789ABCDEF"

func refIsUnreserved(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	}
	return c == '-' || c == '.' || c == '_' || c == '~'
}

func refAllUnreserved(s string) bool {
	for i := 0; i < len(s); i++ {
		if !refIsUnreserved(s[i]) {
			return false
		}
	}
	return true
}

// refDecode reverses refEncodeComponent. Accepts either hex case on input; a
// truncated or non-hex escape is ErrRefBadEscape rather than being passed
// through, because a tolerant decode invents a value.
func refDecode(s string) (string, error) {
	if !strings.ContainsRune(s, '%') {
		return s, nil
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			b.WriteByte(s[i])
			continue
		}
		if i+2 >= len(s) {
			return "", fmt.Errorf("%w: truncated escape in %q", ErrRefBadEscape, s)
		}
		hi, ok1 := refHexVal(s[i+1])
		lo, ok2 := refHexVal(s[i+2])
		if !ok1 || !ok2 {
			return "", fmt.Errorf("%w: %q", ErrRefBadEscape, s[i:i+3])
		}
		b.WriteByte(hi<<4 | lo)
		i += 2
	}
	return b.String(), nil
}

func refHexVal(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}
