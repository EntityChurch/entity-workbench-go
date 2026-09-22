package workbench

import (
	"go.entitychurch.org/entity-core-go/core/ecf"

	"entity-workbench-go/entitysdk"
)

// publish_record.go — what this peer's live published root was minted FROM.
//
// # Why a durable record exists at all
//
// `A-38` (arch, ruled 2026-09-16) separates the `prefix` a root DECLARES from
// the binding set it COMMITS TO: a prefix is a bound, `EXTENSION-TREE` §3.3a
// says so three times, and a publisher may declare `/{peer_id}/` and publish a
// small subset of it. That is the whole fix — it takes a feed publish from 386
// committed keys to 9 while still putting every `FEED-R2` signature inside the
// signed set.
//
// **The published root does not record which it was.** A curated root and a
// scanned root over one prefix are both one hash over one trie, and nothing in
// `system/peer/published-root` distinguishes them — correctly, because a
// consumer is forbidden to read the prefix as a completeness claim and so has
// no business inferring the set from it either.
//
// So the PUBLISHER has to remember, and the reason is a surface rather than a
// protocol obligation: `publish.RootNow` answers *"does what I published still
// describe what I have"* by re-deriving the root, and re-deriving it with the
// wrong set reports **"your root is behind" on a root that is exactly
// current** — permanently, because nothing an operator can do will make a
// curated root equal a scanned one. A standing false line in a problems list
// is how an operator learns to skip the list, and this tree has already paid
// for that once, when a trimmed trailing slash made `feed` report its root
// stale forever.
//
// # Absent means SCAN, and that is a fact rather than a default
//
// The curated mode did not exist before 2026-09-16, so every root this tree
// has ever published was a prefix scan. An absent record is therefore not an
// unknown to be guessed at — it is a period during which only one answer was
// possible. Same shape as `FolderData.Mode`'s absent-means-pre-S6 reading, and
// the same reason it is written down here rather than inferred at each call
// site.

// PublishRecordPath is where the publisher's own note about its live root
// lives. One record, because a peer has exactly one published root.
const PublishRecordPath = "app/workbench/publish/record"

// PublishRecordType is the entity type at [PublishRecordPath].
const PublishRecordType = "workbench/publish-record"

// Content-set names. These are the STORED spellings and are not display
// strings: renaming one silently makes every existing record read as the
// scan, which is the reading that reports a curated root permanently stale.
const (
	// PublishContentScan is the prefix scan — every binding the prefix
	// bounds. The empty string on purpose, so an absent record and an
	// explicit scan are the same value and no caller has to know which
	// it got.
	PublishContentScan = ""

	// PublishContentFeed is `A-38` ruling (D)'s feed set: the entries,
	// the index head and pages, and the signature held for each entry.
	PublishContentFeed = "feed"
)

// PublishRecordData is the note. Deliberately one field: it answers one
// question, and a struct that accumulates "everything about the last publish"
// becomes a second, staler copy of the published root itself.
type PublishRecordData struct {
	// ContentSet is one of the PublishContent* constants above.
	ContentSet string `cbor:"content_set" json:"content_set"`
}

// RecordPublishContentSet stores how the root that was just minted chose its
// bindings.
//
// Written on every successful publish INCLUDING a scan, so the record tracks
// the live root rather than only the interesting case: a peer that publishes a
// feed and later re-publishes a site must go back to reading the scan, and a
// record that were only written for the curated case would leave it claiming
// otherwise forever.
func RecordPublishContentSet(st *entitysdk.Store, name string) error {
	_, err := st.Put(PublishRecordPath, PublishRecordType, PublishRecordData{ContentSet: name})
	return err
}

// ReadPublishContentSet returns the content-set name the live published root
// was minted with, or [PublishContentScan] when there is no record.
//
// An entity at the path that does not decode, or that carries a name this
// build does not know, also reads as the scan. That is the deliberately
// un-AP33 choice in this one place and the reason is the consequence: this
// value feeds a *staleness indicator*, so refusing would take out the whole
// `feed` and `publish status` surface over a note, while guessing wrong costs
// one wrong line on one row. A name this build does not know is also exactly
// what a downgrade looks like, and a downgraded build cannot mint the set it
// cannot name.
func ReadPublishContentSet(st *entitysdk.Store) string {
	ent, ok := st.Get(PublishRecordPath)
	if !ok || ent.Type != PublishRecordType {
		return PublishContentScan
	}
	var d PublishRecordData
	if err := ecf.Decode(ent.Data, &d); err != nil {
		return PublishContentScan
	}
	switch d.ContentSet {
	case PublishContentFeed:
		return PublishContentFeed
	default:
		return PublishContentScan
	}
}
