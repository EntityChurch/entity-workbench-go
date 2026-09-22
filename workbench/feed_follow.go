package workbench

import (
	"fmt"
	"sort"
	"strings"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/hash"

	"entity-workbench-go/entitysdk"
)

// feed_follow.go — the reader's own two records: who they follow, and how
// far they have read.
//
// # A follow is a DECLARATION and it is private
//
// `APP-CONVENTION-FEED` §2.4: a feed-follow follows a **namespace** —
// *"public, pull-only, requiring no grant and no permission, and the
// publisher does not know the follower exists"* — which is exactly what
// distinguishes it from `app/share/follow`, where following a **grant**
// means the publisher authorized you and therefore knows. Two
// authorization models cannot share a record without one lying about what
// it grants.
//
// So a follow establishes nothing at the far end. It records an interest,
// and a read derives from it — the same declare/derive split `share` and
// `accept` run, with the difference that this loop's whole output is a
// READ and there is nothing to reconcile on anybody else's machine.
//
// ⚠ **THE PATH IS NOT UNDER `app/feed/`, AND THAT IS THE POINT.**
// §2.4 makes a follow record *the reader's private data*, and publishing
// a follow list *"a separate, voluntary act"*. A feed publish commits to
// `app/feed/` — so a follow stored under the convention's own prefix is
// **published by the ordinary act of publishing your feed**, turning the
// voluntary act into the default and telling every reader on earth who
// this peer reads. §2's cross-impl contract is the **type tag, not the
// path**, so keeping these under `app/workbench/` costs nothing and keeps
// them out of whatever the operator signs.
//
// # The cursor is local state and v0.3 removed it from the record
//
// §4.4: the cursor is `{page, applied}`, it is held wherever the reader
// keeps its own bookkeeping, and **nothing publishes it.** v0.2 carried
// `? cursor: content-hash` on the follow record; a bare hash cannot
// express `{page, applied}`, so the field was simultaneously the declared
// one and the wrong shape. It is a separate record here, under the same
// private prefix, for the same reason and one more: a cursor moves on
// every read and a follow does not, so merging them would rewrite a
// declaration every time somebody looked at a feed.

// FeedFollowPrefix is where this reader keeps its follows. The ENTITIES
// are `app/feed/follow` (§2.4's type, which is the cross-impl contract);
// only the placement is ours. See the file note for why it is not under
// `app/feed/`.
const FeedFollowPrefix = "app/workbench/feed/follows/"

// FeedCursorPrefix is where §4.4's reader position lives, one record per
// subject.
const FeedCursorPrefix = "app/workbench/feed/cursors/"

// FeedCursorType is the entity type at FeedCursorPrefix. **Workbench-owned
// and deliberately not a convention type**: the convention has no record
// for a reader's position, on purpose, and inventing one under its
// namespace would be minting a shape two implementations might then
// disagree about for no reason (AP20).
const FeedCursorType = "workbench/feed-cursor"

// FeedCursorRecord is one subject's read position, persisted.
type FeedCursorRecord struct {
	// Subject is whose feed this is a position in.
	Subject string `cbor:"subject"`
	// Page is the index page the reader reached. **Carried as well as the
	// hash** so §4.4's MUST — resume from `page` when `applied` no longer
	// resolves — is answerable at all.
	Page uint64 `cbor:"page"`
	// Applied is the newest entry taken from that page.
	Applied hash.Hash `cbor:"applied"`
	// ReadAt is when this position was recorded, ms since epoch. A fact
	// about the READING, never about the publisher — nothing here is
	// evidence of when they last posted.
	ReadAt uint64 `cbor:"read_at"`
}

// FeedFollowRow is one follow plus whatever local state goes with it.
type FeedFollowRow struct {
	Follow entitysdk.FeedFollowData
	// Cursor is the reader's position, and Read reports whether there is
	// one. **A follow with no cursor and a follow at position zero are
	// different facts** — never read versus read and nothing kept.
	Cursor entitysdk.FeedCursor
	Read   bool
	ReadAt uint64
}

// SaveFeedFollow writes (or updates) one follow declaration.
//
// Updating preserves `since`: the follow is the same relationship and
// re-typing it with a petname is not a new one. A caller that wants the
// original must read it first — which is what [LoadFeedFollow] is for.
func SaveFeedFollow(st *Store, f entitysdk.FeedFollowData) error {
	if st == nil {
		return fmt.Errorf("no store")
	}
	if strings.TrimSpace(f.Subject) == "" {
		return fmt.Errorf("a follow names no subject; §2.4 makes it the peer namespace being followed")
	}
	ent, err := f.ToEntity()
	if err != nil {
		return err
	}
	if _, err := st.Put(FeedFollowPrefix+f.Subject, ent.Type, f); err != nil {
		return fmt.Errorf("persist follow of %s: %w", f.Subject, err)
	}
	return nil
}

// LoadFeedFollow reads one follow.
func LoadFeedFollow(st *Store, subject string) (entitysdk.FeedFollowData, bool) {
	if st == nil || subject == "" {
		return entitysdk.FeedFollowData{}, false
	}
	ent, ok := st.Get(FeedFollowPrefix + subject)
	if !ok || ent.Type != entitysdk.TypeFeedFollow {
		return entitysdk.FeedFollowData{}, false
	}
	var f entitysdk.FeedFollowData
	if err := ecf.Decode(ent.Data, &f); err != nil {
		return entitysdk.FeedFollowData{}, false
	}
	if f.Subject == "" {
		f.Subject = subject
	}
	return f, true
}

// RemoveFeedFollow drops a follow and its cursor, reporting whether a
// follow was there.
//
// **The cursor goes with it.** A position in a feed nobody follows is
// bookkeeping about a relationship that no longer exists, and leaving it
// behind means re-following silently resumes from a stale position
// instead of showing the operator the feed.
func RemoveFeedFollow(st *Store, subject string) bool {
	if st == nil || subject == "" {
		return false
	}
	existed := st.Remove(FeedFollowPrefix + subject)
	st.Remove(FeedCursorPrefix + subject)
	return existed
}

// LoadFeedFollows reads every follow with its cursor, sorted by subject.
//
// Rows that do not decode are reported rather than dropped (AP33): a
// follow that vanishes silently is indistinguishable from one that was
// never made.
func LoadFeedFollows(st *Store) (rows []FeedFollowRow, problems []string) {
	if st == nil {
		return nil, nil
	}
	for _, e := range st.List(FeedFollowPrefix) {
		// RelativeUnder, never TrimPrefix — Store.List returns
		// peer-qualified paths (AP58).
		key, under := RelativeUnder(e.Path, FeedFollowPrefix)
		if !under || key == "" || strings.Contains(key, "/") {
			continue
		}
		f, ok := LoadFeedFollow(st, key)
		if !ok {
			problems = append(problems, key+": a follow record that did not decode")
			continue
		}
		row := FeedFollowRow{Follow: f}
		if cur, readAt, ok := LoadFeedCursor(st, key); ok {
			row.Cursor, row.ReadAt, row.Read = cur, readAt, true
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Follow.Subject < rows[j].Follow.Subject })
	sort.Strings(problems)
	return rows, problems
}

// SaveFeedCursor records where a read of `subject` reached.
func SaveFeedCursor(st *Store, subject string, cur entitysdk.FeedCursor, nowMillis uint64) error {
	if st == nil {
		return fmt.Errorf("no store")
	}
	if subject == "" {
		return fmt.Errorf("a cursor with no subject is a position in nothing")
	}
	rec := FeedCursorRecord{Subject: subject, Page: cur.Page, Applied: cur.Applied, ReadAt: nowMillis}
	if _, err := st.Put(FeedCursorPrefix+subject, FeedCursorType, rec); err != nil {
		return fmt.Errorf("persist read position for %s: %w", subject, err)
	}
	return nil
}

// LoadFeedCursor reads one subject's position.
func LoadFeedCursor(st *Store, subject string) (cur entitysdk.FeedCursor, readAt uint64, ok bool) {
	if st == nil || subject == "" {
		return entitysdk.FeedCursor{}, 0, false
	}
	ent, found := st.Get(FeedCursorPrefix + subject)
	if !found || ent.Type != FeedCursorType {
		return entitysdk.FeedCursor{}, 0, false
	}
	var rec FeedCursorRecord
	if err := ecf.Decode(ent.Data, &rec); err != nil {
		return entitysdk.FeedCursor{}, 0, false
	}
	return entitysdk.FeedCursor{Page: rec.Page, Applied: rec.Applied}, rec.ReadAt, true
}

// FeedPrivacyProblem reports a published prefix that would carry this
// reader's own follows or cursors, in the operator's words.
//
// **§2.4 makes publishing a follow list a separate voluntary act**, so a
// peer that publishes a prefix covering these records has done it without
// being asked. Nothing else in the chain will mention it: the records are
// well-formed, the publish is correct, and the only thing wrong is that
// somebody can now read who this peer reads.
//
// ⚠ **A PREFIX IS A BOUND AND NOT A CONTENT SET, AND THIS FUNCTION READ IT AS
// ONE.** `A-38` ruling (D) is exactly that distinction, and it arrived here one
// function later than it arrived in the publisher: a `publish -feed` declares
// the PEER ROOT — which bounds `app/workbench/feed/` along with everything else
// — and commits to nine keys, none of them a follow record. Reading the prefix
// alone therefore accused the operator who took the ruling's advice of
// publishing their follow list, in the same breath as telling them to fix it by
// doing what they had just done. A warning that fires on the one action that
// cannot cause the harm is worse than no warning: it is the line an operator
// learns to skip, on the surface where the real disclosure would appear.
//
// So it takes the content set. Empty (the scan) means the prefix IS the set and
// the old reading is correct; [PublishContentFeed] admits only
// `app/feed/index*`, `app/feed/entries*` and the signature held for each entry
// (`publish.FeedContent`), so a follow record is outside it by construction and
// not by luck — the selector cannot widen to reach one.
//
// Returns "" when the published root does not commit to them.
func FeedPrivacyProblem(publishedPrefix, contentSet string) string {
	if contentSet == PublishContentFeed {
		return ""
	}
	p := strings.Trim(strings.TrimSpace(publishedPrefix), "/")
	covers := func(key string) bool {
		if p == "" {
			return true // the whole tree
		}
		return strings.HasPrefix(strings.Trim(key, "/")+"/", p+"/")
	}
	if !covers(FeedFollowPrefix) && !covers(FeedCursorPrefix) {
		return ""
	}
	scope := publishedPrefix
	if p == "" {
		scope = "the whole tree"
	}
	return "this peer's published root commits to " + scope + ", which contains its own follow list " +
		"and read positions — FEED §2.4 makes publishing a follow list a separate, voluntary act, and " +
		"this publishes it. `publish -feed` commits to the feed's entries, index and signatures and " +
		"to nothing about who reads it"
}
