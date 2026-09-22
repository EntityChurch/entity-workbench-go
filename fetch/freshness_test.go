package fetch

import (
	"strings"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/types"
)

// freshness_test.go — the two sentences, and the assertion that neither
// can be produced by the other's mode.
//
// **This is a gate on prose, which is unusual here and is the point.**
// Both sentences are plausible English about a correctly verified root,
// so a live claim pasted under a static result is invisible to every
// other test in this repository: the chain is green, the timestamps are
// real, the seq is right, and the only thing wrong is a statement about
// who could have been withholding. `make textual` cannot see it and no
// type can either.
//
// Tier: unit.

func rootAt(mode Mode, publishedAt uint64, seq uint64, observed time.Time) VerifiedRoot {
	return VerifiedRoot{
		Mode:       mode,
		ObservedAt: observed,
		Data: types.PublishedRootData{
			PublishedAt: publishedAt,
			Seq:         seq,
		},
	}
}

// TestFreshnessSentencesCannotCross is the gate the plan asked for.
//
// The distinguishing clause of each mode must be absent from the other.
// Not "the strings differ" — they would differ on the timestamp alone,
// so that assertion passes against a build where both modes emit the
// static claim.
func TestFreshnessSentencesCannotCross(t *testing.T) {
	at := time.Date(2026, 9, 12, 10, 4, 5, 0, time.UTC)
	static := rootAt(ModeStaticOrigin, 1_757_671_445_000, 7, at).Freshness()
	live := rootAt(ModeLivePeer, 1_757_671_445_000, 7, at).Freshness()

	// What only a static read has to confess: there is a third party in
	// the middle who could be serving an old root.
	const staticOnly = "withholding origin"
	// What only a live read has earned: the publisher answered for
	// itself, so there is nobody in that position.
	const liveOnly = "answered for itself"

	if !strings.Contains(static, staticOnly) {
		t.Errorf("the static sentence does not confess the withholding case:\n  %s", static)
	}
	if strings.Contains(static, liveOnly) {
		t.Errorf("the static sentence claims the publisher answered for itself:\n  %s", static)
	}
	if !strings.Contains(live, liveOnly) {
		t.Errorf("the live sentence does not say the publisher answered for itself:\n  %s", live)
	}
	if strings.Contains(live, staticOnly) {
		t.Errorf("the live sentence confesses a party that was not in the exchange:\n  %s", live)
	}
}

// TestLiveFreshnessDoesNotClaimTheRootIsCurrent is the overclaim this is
// most likely to drift into, so it is pinned rather than trusted to
// review.
//
// A live read removes the ORIGIN from the trust set. It does not make the
// publisher diligent: a peer that has not republished in a year answers
// instantly with a year-old root, and nothing about the exchange looks
// different. *Quiet publisher* survives both modes; only *withholding
// origin* is removed by one of them.
func TestLiveFreshnessDoesNotClaimTheRootIsCurrent(t *testing.T) {
	live := rootAt(ModeLivePeer, 1_700_000_000_000, 3, time.Now()).Freshness()

	if !strings.Contains(live, "has not republished") {
		t.Errorf("the live sentence does not keep the quiet-publisher caveat:\n  %s", live)
	}
	for _, overclaim := range []string{"current", "up to date", "latest", "newest root"} {
		if strings.Contains(strings.ToLower(live), overclaim) {
			t.Errorf("the live sentence claims %q, which a live read does not establish:\n  %s",
				overclaim, live)
		}
	}
	// It must still name BOTH moments, because they answer different
	// questions: when we asked, and when the publisher signed.
	if !strings.Contains(live, "published_at") {
		t.Errorf("the live sentence drops the signed moment:\n  %s", live)
	}
}

// TestAnUnnamedModeProducesNeitherClaim covers the default arm.
//
// A Source that names no mode is a bug in that Source. Answering it with
// the static sentence would be the tempting safe-looking choice and is
// wrong: it would bury the bug under a true-sounding claim, and the next
// implementer would never learn their Describe was never called.
func TestAnUnnamedModeProducesNeitherClaim(t *testing.T) {
	got := rootAt("", 1_700_000_000_000, 3, time.Now()).Freshness()
	for _, claim := range []string{"withholding origin", "answered for itself", "verified as of"} {
		if strings.Contains(got, claim) {
			t.Errorf("a root with no mode produced the %q claim:\n  %s", claim, got)
		}
	}
	if !strings.Contains(got, "cannot be stated") {
		t.Errorf("a root with no mode did not say so:\n  %s", got)
	}
}

// TestHTTPSourceNamesTheStaticMode is the small half that stops the
// sentences being right about a mode nothing sets.
//
// The live half is `PeerSource` in `workbench`, and the arm that matters
// for both is `publish/live_and_static_test.go`, where two real sources
// read one published act and the two sentences come out of real
// verifications rather than a struct literal.
func TestHTTPSourceNamesTheStaticMode(t *testing.T) {
	src := NewHTTPSource(Layout{PeerID: srcTestPeer, Origin: "https://example.test"}, nil)
	got := src.Describe()
	if got.Mode != ModeStaticOrigin {
		t.Errorf("HTTPSource.Describe().Mode = %q, want %q", got.Mode, ModeStaticOrigin)
	}
	if got.Authority != "https://example.test" {
		t.Errorf("Authority = %q, want the origin", got.Authority)
	}
}
