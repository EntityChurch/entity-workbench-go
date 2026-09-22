package workbench

import (
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/hash"

	"entity-workbench-go/entitysdk"
)

// feed_follow_test.go — the reader's own two records.
//
// Tier: model, on a **peer-backed** store for `tree_path_test.go`'s
// reason: `Store.List` returns peer-qualified paths on a real peer and
// bare ones on `NewStore(memory, memory)`, so a fixture without a
// NamespacedIndex cannot fail on the prefix arithmetic these functions do
// (AP58).

func followStore(t *testing.T) *Store {
	t.Helper()
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { ap.Close() })
	return ap.Store()
}

func TestFeedFollow_RoundTripsAndTakesItsCursorWithIt(t *testing.T) {
	st := followStore(t)
	const subject = "2KFRBJ9feEPCZZiNaCEKsCAVkGkp1htWZk9a8jz5n5D2sS"

	if err := SaveFeedFollow(st, entitysdk.FeedFollowData{
		Subject: subject, Label: "a petname", Via: "@them", Since: 1_700_000_000_000,
	}); err != nil {
		t.Fatalf("SaveFeedFollow: %v", err)
	}
	rows, problems := LoadFeedFollows(st)
	if len(problems) != 0 {
		t.Errorf("problems on a clean store: %v", problems)
	}
	if len(rows) != 1 {
		t.Fatalf("read %d follows, want 1 — a peer-qualified path defeated the prefix arithmetic if "+
			"this is zero", len(rows))
	}
	if rows[0].Follow.Label != "a petname" || rows[0].Follow.Since != 1_700_000_000_000 {
		t.Errorf("follow came back as %+v", rows[0].Follow)
	}

	if rows[0].Read {
		// **A follow with no cursor and a follow at position zero are
		// different facts** — never read versus read and nothing kept —
		// and only the flag can tell them apart.
		t.Error("a follow that has never been read came back as read")
	}

	if err := SaveFeedCursor(st, subject, entitysdk.FeedCursor{Page: 7, Applied: hash.Hash{}}, 99); err != nil {
		t.Fatalf("SaveFeedCursor: %v", err)
	}
	rows, _ = LoadFeedFollows(st)
	if !rows[0].Read || rows[0].Cursor.Page != 7 || rows[0].ReadAt != 99 {
		t.Errorf("cursor came back as read=%v %+v at %d", rows[0].Read, rows[0].Cursor, rows[0].ReadAt)
	}

	// Unfollowing takes the position with it. A position in a feed nobody
	// follows is bookkeeping about a relationship that no longer exists,
	// and leaving it means a later follow silently resumes from a stale
	// place instead of showing the operator the feed.
	if !RemoveFeedFollow(st, subject) {
		t.Fatal("RemoveFeedFollow reported nothing was there")
	}
	if _, _, ok := LoadFeedCursor(st, subject); ok {
		t.Error("the read position outlived the follow")
	}
	if rows, _ := LoadFeedFollows(st); len(rows) != 0 {
		t.Errorf("%d follows after removing the only one", len(rows))
	}
}

// TestFeedPrivacyProblem_FiresExactlyWhenThePublishWouldCarryTheFollowList
// is §2.4's *publishing a follow list is a separate, voluntary act*.
//
// **The negative arms are the load-bearing ones.** A warning that fires on
// every publish is a warning an operator learns to skip, and this one has
// to be silent for the prefix the feed verbs actually tell people to use.
func TestFeedPrivacyProblem_FiresExactlyWhenThePublishWouldCarryTheFollowList(t *testing.T) {
	quiet := []string{"app/feed/", "sites/", "app/site/", "archives/"}
	for _, p := range quiet {
		if got := FeedPrivacyProblem(p); got != "" {
			t.Errorf("publishing %q warned about the follow list, which it does not contain: %s", p, got)
		}
	}
	loud := []string{"app/workbench/", "app/workbench/feed/", "app/", "/", ""}
	for _, p := range loud {
		got := FeedPrivacyProblem(p)
		if got == "" {
			t.Errorf("publishing %q carries this peer's own follow list and said nothing", p)
			continue
		}
		if !strings.Contains(got, "voluntary act") {
			t.Errorf("the warning for %q does not say what rule it is about: %s", p, got)
		}
	}
	// The trap `publish`'s own narrowing warning hit: §3.3a spells the
	// universal tree "/" and everything else without a leading slash, so a
	// naive prefix comparison gets the one case that covers everything
	// backwards.
	if FeedPrivacyProblem("/") == "" {
		t.Error("publishing the whole tree does not carry the follow list, apparently")
	}
	// `app/feedback/` is not under `app/feed/` and neither is under the
	// other — the segment-exactness these helpers all share.
	if got := FeedPrivacyProblem("app/feedback/"); got != "" {
		t.Errorf("a sibling directory warned: %s", got)
	}
}
