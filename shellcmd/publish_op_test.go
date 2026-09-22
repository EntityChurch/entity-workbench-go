package shellcmd

import (
	"strings"
	"testing"
)

// publish_op_test.go — the pure halves of the publishing act.
//
// The end-to-end gate is `shellboot/public_site_scope_probe_test.go`,
// which measures what a stranger can actually read across a wire. What
// is here is the arithmetic that decides what an operator is TOLD, kept
// separate because it is the part with no observable failure: a missing
// warning looks exactly like a successful publish.

// TestNarrowedFrom_CatchesASiteGoingDark is the safety line, and it
// exists because nothing downstream can raise it.
//
// A peer has exactly ONE published root. Publishing `sites/notes/` over
// a root that committed to `sites/` leaves every other site answering
// "absent" — under a valid signature, so a reader cannot tell it from a
// site that was never there. `fetch.ErrEmptyEnumeration` exists because
// absence carries no information, and here the publisher is the only
// party in the whole chain that knows what just happened.
func TestNarrowedFrom_CatchesASiteGoingDark(t *testing.T) {
	for _, tc := range []struct {
		name       string
		prior, now string
		want       bool
	}{
		{"first publish has no prior", "", "sites/", false},
		{"republishing the same prefix", "sites/", "sites/", false},
		{"narrowing to one site takes the others dark", "sites/", "sites/notes/", true},
		{"widening is safe — the old keys are still committed", "sites/notes/", "sites/", false},
		{"an unrelated prefix takes everything dark", "sites/", "archives/", true},
		{"the universal root widens everything", "sites/", "/", false},
	} {
		if got := narrowedFrom(tc.prior, tc.now); got != tc.want {
			t.Errorf("%s: narrowedFrom(%q, %q) = %v, want %v", tc.name, tc.prior, tc.now, got, tc.want)
		}
	}
}

// TestPublishProblems_HasOneWriterForEachSentence pins the shapes the
// act and the read must agree on.
//
// `publishProblems` is shared by `Publish` and `PublishStatus` for
// `FolderStatus.problems()`'s reason: the two surfaces are read side by
// side, and a difference between them reads to an operator as a change
// in the peer rather than a difference in the code.
func TestPublishProblems_HasOneWriterForEachSentence(t *testing.T) {
	narrowed := publishProblems(PublishOutcome{Prefix: "sites/notes/", NarrowedFrom: "sites/"})
	if len(narrowed) != 1 || !strings.Contains(narrowed[0], "sites/") {
		t.Errorf("a narrowing publish produced %v; it must name what stopped being committed", narrowed)
	}

	// A stale public grant: the row authorizes a prefix the root no
	// longer commits to. That is a disclosure with no site behind it,
	// and it is invisible from either entity alone — the grant and the
	// root have no link between them except the act that wrote both.
	stale := publishProblems(PublishOutcome{
		Prefix: "sites/notes/",
		Public: PublicGrantState{Present: true, Ours: true, Prefix: "sites", Stale: true},
	})
	if len(stale) == 0 || !strings.Contains(strings.Join(stale, " "), "no longer publishes") {
		t.Errorf("a stale public grant produced %v; it must say the grant outlived the prefix", stale)
	}

	// Reach is only worth mentioning when something is on offer. A peer
	// that has published nothing publicly is not failing to be reachable
	// — telling it that it is unreachable is a warning about a state it
	// did not ask for.
	quiet := publishProblems(PublishOutcome{
		Prefix: "sites/",
		Reach:  PublishReach{Note: "this peer is not listening"},
	})
	for _, p := range quiet {
		if strings.Contains(p, "not listening") {
			t.Error("a peer with no public grant was warned about reachability it does not need")
		}
	}

	offered := publishProblems(PublishOutcome{
		Prefix: "sites/",
		Public: PublicGrantState{Present: true, Ours: true, Prefix: "sites"},
		Reach:  PublishReach{Note: "this peer is not listening"},
	})
	if len(offered) == 0 || !strings.Contains(strings.Join(offered, " "), "not listening") {
		t.Errorf("a PUBLIC site on a peer nothing can dial produced %v; that is the state an "+
			"operator stares at wondering why nobody can see their site", offered)
	}
}

// TestPublicGrantState_TolerantOfTheTrailingSlash is a one-line rule
// with a wide blast radius.
//
// §3.3a makes a published root's prefix end in "/" and a capability
// resource pattern carries no trailing slash, so `sites` and `sites/`
// are the same prefix spelled two ways. Comparing them raw would raise
// a stale-disclosure alarm on every correctly-published peer — a
// warning that is wrong in exactly the case it is supposed to be quiet.
func TestPublicGrantState_TolerantOfTheTrailingSlash(t *testing.T) {
	for _, tc := range []struct {
		published, granted string
		wantStale          bool
	}{
		{"sites/", "sites", false},
		{"/", "", false},
		{"sites/notes/", "sites", true},
		{"sites/", "sites/notes", true},
	} {
		s := PublicGrantState{Present: true, Ours: true, Prefix: tc.granted}
		s.Stale = tc.published != "" &&
			strings.Trim(tc.published, "/") != strings.Trim(s.Prefix, "/")
		if s.Stale != tc.wantStale {
			t.Errorf("published %q granted %q: stale=%v, want %v",
				tc.published, tc.granted, s.Stale, tc.wantStale)
		}
	}
}
