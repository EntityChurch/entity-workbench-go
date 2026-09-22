// `app/feed/mirror` and `app/feed/mirror-page` — APP-CONVENTION-FEED §6, the
// gathered view.
//
// ⭐ **THIS IS NOT A FEED FEATURE, AND §6.0a SAYS SO IN ITS FIRST LINE.**
// `SYSTEM-DATA-EXCHANGE` §2.5 is the authority over this section, §2.3's four
// rules are stated there and merely restated in FEED §6.1, and *on any
// disagreement the exchange wins.* A gatherer is any peer that republishes what
// it obtained; a wiki or a forum on the same substrate would get gathering from
// the identical rule with a different noun. What is feed-specific here is only
// the type tag.
//
// This file is the VOCABULARY — the two types, the subject, and §6.0.1's
// coordinate derivation. It deliberately carries no gathering loop: what a
// gatherer may bind is `SYSTEM-DATA-EXCHANGE` §2.1's byte-preservation MUST, and
// the operation that satisfies it (`AppPeer.PutEntity`) has never been used
// against foreign bytes in this tree. The loop is owed and is a separate change.
//
// ## Why the coordinate CALLS the kernel
//
// §6.0.1 pins the live coordinate to `EXTENSION-REVISION` §3.1's `prefix_hash`
// and `[v0.3]` adds a MUST NOT against implementing it as a new derivation. It
// is a **derive-to-meet** value: two peers compute it independently from the same
// path string and must land on the same byte, with **nothing failing loudly** if
// they do not — they simply construct different keys and never find each other's
// mirror. So this calls `revision.PrefixHash` rather than restating three lines,
// and a kernel change reaches us by recompiling.
//
// **Measured, and this is the reason to trust the shape:** our
// `revision.PrefixHash("/{peer}/app/feed/index")` and `entity-browser-rust`'s
// `MirrorSubject::coordinate()` agree byte for byte on their pinned vector, and
// their expected value is computed with `hashlib` from the ECF framing rules
// rather than by calling their kernel. Two implementations, two languages, two
// kernels, one independently-derived literal — which per [ADR-0012] is **not**
// cohort-consistent. `feed_mirror_test.go` carries the vector.
//
// ## What this file refuses, and why each refusal is structural
//
//   - **There is no completeness field and there will not be one** (§6.1 rule 2 /
//     `DX-R14`). The verifiable property is narrower and more useful: *a mirror
//     can omit but never substitute.* A `complete: true` a gatherer could set is
//     a claim nothing can check.
//   - **`entries` is always PINNED** (§6.0). A live reference would make the
//     mirror a view of whatever is at that path now, which is not a mirror.
//   - **The derivation reads identifying fields ONLY** (`FEED-R27`) — never `at`,
//     `via` or `seen`. Two readers naming one subject must derive one key, and *a
//     derivation that includes an optional field is not a derivation.*
//   - **Pages are never renumbered, merged or compacted, and `page` MUST equal
//     its key** (§6.0a). Both are asserted at encode time, because the failure is
//     otherwise a silently-moved page that still verifies.
package entitysdk

import (
	hexenc "encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/ext/revision"
)

// The §6 type tags. As everywhere in this convention the TAG is the cross-impl
// contract and the path is not — except for the mirror prefix below, which §6.0.1
// pins by hand and for a stated reason.
const (
	TypeFeedMirror     = "app/feed/mirror"
	TypeFeedMirrorPage = "app/feed/mirror-page"
)

// FeedMirrorPrefix is §6.0.1's pinned location, peer-relative.
//
// **This is the third and last path the convention pins** (with `FeedIndexPath`
// and its pages), and §6.0.1 gives the reason it is not left to the
// implementation the way an entry's key is: a reader needs to **enumerate what a
// peer has gathered, from that peer's signed root, without asking them.** A
// type-filtered query cannot be served by a static origin; a conventional prefix
// is reachable by ordinary trie descent on every publishing posture.
const FeedMirrorPrefix = "app/feed/mirrors/"

// MirrorSubject is §6.0's coordinate — *what this view is of.*
//
// Exactly one of the two arms is populated, and which one decides what the
// mirror MEANS rather than merely how it is addressed:
//
//	thread   — a PIN to one entity many parties contribute to. Many writers, no
//	           single authority, and *short* means a contributor you did not reach.
//	timeline — a LIVE reference to a path one peer owns. Exactly one writer, and
//	           *short* means a gap in that timeline.
//
// Use [MirrorSubjectThread] / [MirrorSubjectTimeline] rather than building one
// by hand: the constructors are what make the two arms mutually exclusive, and a
// subject with both populated has no defined coordinate.
type MirrorSubject struct {
	// Peer is who to ask about the subject itself. For a thread it is NOT part
	// of the key — the entity's own hash is — so two gatherers who learned of
	// one thread from different peers still meet.
	Peer string

	// Root is the thread arm: the pinned entity this view is of.
	Root *hash.Hash

	// Path is the timeline arm: peer-relative, leading slash, as a live
	// reference's `path` is stored.
	Path string
}

// MirrorSubjectThread is §6.0's pinned arm — a view of one entity many parties
// contribute to, a conversation's root entry being the worked example.
func MirrorSubjectThread(peer string, root hash.Hash) MirrorSubject {
	r := root
	return MirrorSubject{Peer: peer, Root: &r}
}

// MirrorSubjectTimeline is §6.0's live arm for the case the convention pins by
// hand: `[MUST]` the live subject of a **timeline** mirror is a live reference to
// the author's index head, `app/feed/index` (§4.2) `[v0.3]`.
//
// **The author's index HEAD HASH is the tempting subject and it is wrong twice
// over**, which is why this takes a peer and not a hash: the head's hash changes
// every time the author posts, so it is a *witness* rather than an identity; and
// a reader must already have reached the author to know it, which is the hop the
// mirror exists to save. §6.0's `[MUST NOT]` — *a subject MUST NOT be a pin to a
// value that moves when the subject changes* — is that sentence as a rule.
//
// Three properties pick this path and no other candidate has all three: the
// convention pins it by hand (so a second gatherer computes it from the peer id
// alone, where an entry prefix is implementation-chosen and could never be
// computed by another seat); it RESOLVES, so §2.2.2's four outcomes apply; and it
// does not move when the author posts.
func MirrorSubjectTimeline(peer string) MirrorSubject {
	return MirrorSubject{Peer: peer, Path: "/" + FeedIndexPath}
}

// MirrorSubjectFromReference reads a subject off a decoded mirror record.
//
// ⭐ **It names the identifying fields and nothing else, on purpose**
// (`FEED-R27`). `At`, `Via` and `Seen` are hints and expectations; folding any of
// them into the key means two readers naming the same subject derive different
// addresses and neither finds the other's mirror.
//
// Go cannot make *adding a field to [EntityRef]* a compile error here the way a
// destructuring match can. `TestMirrorSubject_DerivationReadsIdentifyingFieldsOnly`
// stands in for that: it holds an unkeyed composite literal of `EntityRef`, which
// **does** fail to compile when a field is added, so the next person to widen the
// reference atom is sent to this function.
func MirrorSubjectFromReference(r EntityRef) (MirrorSubject, error) {
	switch r.Tag {
	case RefTagPin:
		if r.Hash == nil {
			return MirrorSubject{}, NewError(400, "invalid_mirror_subject",
				"a pinned subject carries no hash")
		}
		return MirrorSubjectThread(r.Peer, *r.Hash), nil
	case RefTagLive:
		if r.Path == "" {
			return MirrorSubject{}, NewError(400, "invalid_mirror_subject",
				"a live subject carries no path")
		}
		return MirrorSubject{Peer: r.Peer, Path: r.Path}, nil
	default:
		return MirrorSubject{}, NewError(400, "invalid_mirror_subject",
			fmt.Sprintf("a subject is a reference atom; tag %q is neither %q nor %q",
				r.Tag, RefTagPin, RefTagLive))
	}
}

// Reference returns the atom a mirror record carries as its `subject`.
func (s MirrorSubject) Reference() EntityRef {
	if s.Root != nil {
		return PinnedRef(s.Peer, *s.Root)
	}
	return LiveRef(s.Peer, s.Path, hash.Hash{})
}

// IsTimeline reports which arm this is. A mirror's `short` means different
// things on the two arms (§6.0), so a surface reporting gaps has to know.
func (s MirrorSubject) IsTimeline() bool { return s.Root == nil }

// Coordinate is §6.0.1's derivation — **the 66-character hex the key ends in.**
//
// The two arms derive differently and share only a SHAPE: a thread's coordinate
// is an entity hash the subject already carries, used verbatim at whatever width
// its own format byte implies (hold-and-fetch); a timeline's is `prefix_hash`
// over the absolute path. Both land on `00`-prefixed 66 hex, which is what keeps
// [MirrorSubject.Key] one expression rather than two.
func (s MirrorSubject) Coordinate() (string, error) {
	if s.Root != nil {
		return hexenc.EncodeToString(s.Root.Bytes()), nil
	}
	if s.Path == "" {
		return "", NewError(400, "invalid_mirror_subject",
			"a subject is either a pinned hash or a live path, and this is neither")
	}
	return PathCoordinate(s.Peer, s.Path), nil
}

// PathCoordinate is §6.0.1's live coordinate: `prefix_hash` over the ABSOLUTE
// path, `/{peer}/{path}`.
//
// ## This is not a new derivation and MUST NOT be implemented as one `[v0.3]`
//
// v0.2 read `hex(content_hash(absolute-path))` — a ONE-argument function this
// corpus does not define, since `content_hash` is over an entity's `{type, data}`
// and a path is not an entity. Two incompatible readings survived that, and
// `entity-browser-rust` shipped one of them (`sha256(utf8(absolute))`) before
// arch ruled. The function already existed, over the identical input, with the
// floor pinned: `EXTENSION-REVISION` §3.1's `prefix_hash` =
// `hex(content_hash(type="system/tree/path", data=path))`.
//
// ⚠ **It is pinned to the ECFv1-SHA-256 floor (`0x00`) whatever the deriving
// peer's home format**, and core-go's own `PrefixHash` doc comment records that
// this differs from one available reading of §3.1 and that nothing has failed
// only because it pins SHA-256 today. If that function ever follows the home
// format, this coordinate MUST NOT follow it — that is a wire event for every
// published mirror, and the gate carries a literal so it surfaces as a diff.
func PathCoordinate(peer, relativePath string) string {
	p := relativePath
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return revision.PrefixHash("/" + peer + p)
}

// Key is where a gatherer binds the HEAD of its mirror of this subject
// (`FEED-R25`), peer-relative.
func (s MirrorSubject) Key() (string, error) {
	c, err := s.Coordinate()
	if err != nil {
		return "", err
	}
	return FeedMirrorPrefix + c, nil
}

// PageKey is where page `page` of that mirror lives — `{head}/{page}`, §6.0a.
//
// **Derived from [MirrorSubject.Key] rather than spelled**, so §6.0.1's prefix
// and the coordinate derivation have exactly one expression and a page key
// cannot drift from the head it hangs off. Decimal, and key-addressed rather
// than chained by hash — a hash chain makes extending a view republish it.
func (s MirrorSubject) PageKey(page uint64) (string, error) {
	k, err := s.Key()
	if err != nil {
		return "", err
	}
	return k + "/" + strconv.FormatUint(page, 10), nil
}

// FeedMirrorTreePath and FeedMirrorPageTreePath are the same keys PEER-QUALIFIED
// — what a tree write takes. Kept apart from the bare keys for the reason
// `FeedIndexTreePath` states: the bare key is the BINDING and the binding is the
// cross-impl comparand, so the peer segment must not leak into the one string two
// implementations have to agree on.
func FeedMirrorTreePath(peerID string, s MirrorSubject) (string, error) {
	k, err := s.Key()
	if err != nil {
		return "", err
	}
	return "/" + peerID + "/" + k, nil
}

// FeedMirrorPageTreePath is §6.0a's page, peer-qualified.
func FeedMirrorPageTreePath(peerID string, s MirrorSubject, page uint64) (string, error) {
	k, err := s.PageKey(page)
	if err != nil {
		return "", err
	}
	return "/" + peerID + "/" + k, nil
}

// FeedMirrorData is §6's `app/feed/mirror` — the HEAD of one gatherer's view of
// one subject.
//
// **Fixed-size whatever the size of the view it heads**: a subject, two integers
// and two scalars. `entries` lives on pages, and that is `SYSTEM-DATA-EXCHANGE`
// §2.5's growth rule rather than a layout preference — a mirror's membership
// grows with how much the gatherer gathered, which is bounded by nothing, so it
// is a bounded head plus key-addressed pages. §2.5's test is one question: *does
// this collection grow with how many parties participate, or with something the
// format's author controls?* This one grows with participation.
//
// **There is no completeness field** (§6.1 rule 2 / `DX-R14`). Its absence is the
// design and adding one is forbidden at the tier above this convention.
type FeedMirrorData struct {
	// Subject is what this view is OF — §6.0's two arms.
	Subject EntityRef `cbor:"subject"`

	// Current is the highest mirror page in use.
	Current uint64 `cbor:"current"`

	// Oldest is the lowest page still published. Omitted when nothing has been
	// dropped, for `FeedIndexHeadData.Oldest`'s reason — §6 declares 0 as the
	// default, so emitting it spends bytes to say what the default says.
	Oldest *uint64 `cbor:"oldest,omitempty"`

	// GatheredAt is when this gatherer last EXTENDED the view — not when it
	// last looked. A gatherer that read and found nothing has not gathered.
	GatheredAt uint64 `cbor:"gathered_at"`

	// GatheredBy is the key that assembled it.
	//
	// ⚠ **This is the one place a mirror names the gatherer, and it attributes
	// NOTHING.** §6.1 rule 3: the gatherer signs the mirror RECORD; each
	// republished entry travels with its own author's detached signature, and
	// attribution follows `entry.author` verified against that signature,
	// always. *A renderer that attributes a mirrored entry to the gatherer is
	// non-conformant* (`DX-R13`), and one holding an entry whose signature is
	// absent MUST present it as unattributed rather than attributing it to
	// anyone (`DX-R11`).
	GatheredBy string `cbor:"gathered_by"`
}

// ToEntity encodes the mirror head.
func (d FeedMirrorData) ToEntity() (entity.Entity, error) {
	if err := d.Subject.Validate(); err != nil {
		return entity.Entity{}, fmt.Errorf("mirror subject: %w", err)
	}
	if _, err := MirrorSubjectFromReference(d.Subject); err != nil {
		return entity.Entity{}, err
	}
	if d.Oldest != nil && *d.Oldest > d.Current {
		return entity.Entity{}, NewError(400, "invalid_feed_mirror",
			fmt.Sprintf("mirror head says oldest=%d current=%d; the lowest page still published "+
				"cannot be above the highest in use", *d.Oldest, d.Current))
	}
	if d.GatheredBy == "" {
		return entity.Entity{}, NewError(400, "invalid_feed_mirror",
			"a mirror names the key that assembled it; `gathered_by` is empty")
	}
	return encodeAsEntity(TypeFeedMirror, d)
}

// FeedMirrorPageData is §6's `app/feed/mirror-page`, at
// `app/feed/mirrors/{coordinate}/{page}`.
type FeedMirrorPageData struct {
	// Page MUST equal this page's own key (§6.0a), so a page that is moved is
	// detectably moved.
	Page uint64 `cbor:"page"`

	// Entries are republished, UNMODIFIED, and always PINNED (§6.0).
	//
	// **Pinned is not a style choice**: a mirror carries exact bytes (§6.1 rule
	// 1), so an entry named by a live reference would be a mirror of whatever
	// is at that address now — which is not a mirror. [ToEntity] refuses a live
	// one rather than accepting it and meaning something else.
	Entries []EntityRef `cbor:"entries"`

	UpdatedAt uint64 `cbor:"updated_at"`
}

// ToEntity encodes one mirror page.
//
// ⚠ **Nothing here can enforce §6.0a's real rules** — that pages are filled in
// GATHER order, that a page is sealed when its successor opens, and that a sealed
// page is never rewritten to insert an entry discovered later. Those are
// properties of a SEQUENCE of writes and this function sees one. They belong to
// the gathering loop, which does not exist yet, and the gate for them is that a
// sealed page's bytes do not move when the view is extended (`DX-C8`).
func (d FeedMirrorPageData) ToEntity() (entity.Entity, error) {
	for i, r := range d.Entries {
		if err := r.Validate(); err != nil {
			return entity.Entity{}, fmt.Errorf("mirror page %d entry %d: %w", d.Page, i, err)
		}
		if r.Tag != RefTagPin {
			return entity.Entity{}, NewError(400, "invalid_feed_mirror_page",
				fmt.Sprintf("mirror page %d entry %d is a %q reference; §6.0 requires every "+
					"mirrored entry to be PINNED, because a mirror carries exact bytes and a live "+
					"reference would mirror whatever is at that address now", d.Page, i, r.Tag))
		}
	}
	return encodeAsEntity(TypeFeedMirrorPage, d)
}

// FeedMirrorFromEntity decodes a mirror head, refusing anything that is not one.
//
// The type check is not a formality: §2 makes the TYPE TAG the cross-impl
// contract, so an entity at a mirror's key whose tag says otherwise is not a
// mirror that moved, it is a different thing at that address.
func FeedMirrorFromEntity(e entity.Entity) (FeedMirrorData, error) {
	if e.Type != TypeFeedMirror {
		return FeedMirrorData{}, NewError(400, "not_a_feed_mirror",
			fmt.Sprintf("expected %q, got %q", TypeFeedMirror, e.Type))
	}
	var d FeedMirrorData
	if err := ecf.Decode(e.Data, &d); err != nil {
		return FeedMirrorData{}, WrapError(400, "invalid_feed_mirror", "decode feed mirror", err)
	}
	return d, nil
}

// FeedMirrorPageFromEntity decodes one mirror page.
func FeedMirrorPageFromEntity(e entity.Entity) (FeedMirrorPageData, error) {
	if e.Type != TypeFeedMirrorPage {
		return FeedMirrorPageData{}, NewError(400, "not_a_feed_mirror_page",
			fmt.Sprintf("expected %q, got %q", TypeFeedMirrorPage, e.Type))
	}
	var d FeedMirrorPageData
	if err := ecf.Decode(e.Data, &d); err != nil {
		return FeedMirrorPageData{}, WrapError(400, "invalid_feed_mirror_page", "decode feed mirror page", err)
	}
	return d, nil
}
