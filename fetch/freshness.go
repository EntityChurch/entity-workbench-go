package fetch

import (
	"fmt"
	"time"
)

// freshness.go — which kind of party answered, and the one sentence that
// differs because of it.
//
// # The two sentences are not interchangeable, and that IS the product
// difference
//
// Everything else about a live read and a static read is identical by
// construction: the same [Consumer], the same recomputed hashes, the same
// two-hop signature against the key in the peer-id, the same `seq` floor,
// the same fail-closed walk. W2 measured one published site both ways and
// got byte-identical page bodies, an identical committed key set and an
// identical root. **The only thing that legitimately differs is what a
// reader may be told about age**, which is why it gets a file and a gate
// rather than a format string at the call site.
//
//	static   we fetched bytes from an ORIGIN. The publisher signed them at
//	         `published_at`. The origin is a different party and may be
//	         serving a root arbitrarily older than the publisher's current
//	         one — §6.5.3.1, D6/D7: a withholding origin and a quiet
//	         publisher are byte-identical from here.
//	live     we asked the PUBLISHER and the publisher answered. What that
//	         adds is not fresher bytes, it is the removal of a party: there
//	         is no third party in a position to withhold a newer root,
//	         because the party that would be doing the withholding is the
//	         one whose root it is.
//
// **The live sentence must not be allowed to grow past that.** It does
// NOT establish that this is the newest root the publisher could have
// made — a publisher that has not republished in a year answers instantly
// with a year-old root and nothing about the exchange looks different.
// *Quiet publisher* survives both modes; only *withholding origin* is
// removed. Every word in `Freshness` is chosen to say the second without
// implying the first.
//
// # Why it is carried in the outcome and not remembered by the caller
//
// [VerifiedRoot.Mode] is set by [Consumer.VerifiedRoot] from the [Source]
// that answered, so a surface cannot caption a chain by recalling which
// constructor it used. That is the same rule `shellcmd/status.go` follows
// for `Reconciled` and for the same reason: a surface that has to
// remember which function it called in order to caption its result will
// eventually caption it wrong, and always in the confident direction.

// Mode is which kind of party answered for a publisher's bytes.
//
// It is a property of the [Source], not of the publisher: the same
// publisher's same published act is readable both ways, which is the
// thing W2 measured.
type Mode string

const (
	// ModeStaticOrigin is an HTTP origin serving a published snapshot.
	// The origin is a party distinct from the publisher and is trusted
	// for nothing — but it is in a position to WITHHOLD, which is what
	// the static freshness sentence has to confess.
	ModeStaticOrigin Mode = "static-origin"
	// ModeLivePeer is the publishing peer itself, answering a dispatch.
	// **It is not a stronger verification** — an authenticated connection
	// proves who, not what, and every check runs identically either way.
	// It is one fewer party who could be hiding a newer root.
	ModeLivePeer Mode = "live-peer"
)

// Description is how a [Source] names itself to the layer above.
//
// Deliberately tiny. It exists for the freshness sentence and for the
// chain row that says where a step looked; a Source that could describe
// more would be tempted to describe whether its bytes were good, which is
// the one thing the seam forbids it (see source.go).
type Description struct {
	// Mode is which kind of party answers here.
	Mode Mode
	// Authority is that party, addressed the way this mode addresses it:
	// an origin (`https://host`) or a peer (`entity://{peer-id}`).
	Authority string
}

// Freshness is the honest scope of this root, in one sentence, composed
// from the mode that produced it.
//
// **There is one composition and it switches on the mode**, so neither
// sentence can be produced by the other's path — which is exactly what
// `TestFreshnessSentencesCannotCross` asserts. A per-caller format string
// would let the wrong one be typed next to the right value, and the
// failure would be invisible: both are plausible English about a
// correctly verified root.
func (v VerifiedRoot) Freshness() string {
	return FreshnessFor(v.Mode, v.Data.PublishedAt, v.Data.Seq, v.ObservedAt)
}

// FreshnessFor is [VerifiedRoot.Freshness] over loose fields, for the
// surfaces that carry a root's facts flattened into a view struct rather
// than carrying the root.
//
// **It exists because the first version of this did not, and the shape it
// forced was wrong.** `ConsumeOutput` held the whole [VerifiedRoot] in an
// unexported field and asked it — which is right on the product path and
// leaves the struct **unconstructible** anywhere else: a renderer test
// building an output literal got a verified-looking result whose mode was
// the zero value, i.e. the refusal branch, silently. A view struct that
// only the model can fill correctly is a trap for every other caller, and
// it was caught by a test that had been asserting the sentence for weeks.
//
// So the mode travels as an ordinary field alongside `published_at` and
// `seq`, exactly as those do, and there is still **one composition**.
func FreshnessFor(mode Mode, publishedAt, seq uint64, observedAt time.Time) string {
	signed := epochMillis(publishedAt)
	switch mode {
	case ModeStaticOrigin:
		return fmt.Sprintf("verified as of published_at %s (seq %d) — never simply \"verified\": "+
			"an origin is a different party from the publisher, and a withholding origin looks "+
			"exactly like a quiet publisher from here",
			signed, seq)
	case ModeLivePeer:
		return fmt.Sprintf("the publisher answered for itself at %s and served published_at %s "+
			"(seq %d) — no third party was in a position to withhold a newer root; a publisher "+
			"that simply has not republished still looks exactly like this",
			observedAtText(observedAt), signed, seq)
	default:
		// Not a fallback to the weaker sentence, on purpose. A Source that
		// names no mode is a bug in that Source, and answering it with a
		// real freshness claim would bury the bug under a plausible
		// sentence — which is the failure this whole file is shaped to
		// avoid.
		return fmt.Sprintf("published_at %s (seq %d) — the byte source did not say what kind of "+
			"party answered, so the scope of this verification cannot be stated",
			signed, seq)
	}
}

func epochMillis(ms uint64) string {
	if ms == 0 {
		return "(unset)"
	}
	return time.UnixMilli(int64(ms)).UTC().Format(time.RFC3339)
}

func observedAtText(t time.Time) string {
	if t.IsZero() {
		return "(unrecorded)"
	}
	return t.UTC().Format(time.RFC3339)
}
