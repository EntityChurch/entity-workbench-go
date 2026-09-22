package main

// Feed bridge surface — following a peer, reading a timeline, and
// resolving a reference.
//
// # Why this file exists
//
// W6 landed `follow` / `unfollow` / `follows` / `timeline` and `ref`, and
// every one of them reached a shell verb and no pixel. That is D23: a
// model with no shipped surface is not shipped, and this tree has done it
// three times before. It is sharper than the usual case for `ref`,
// because `FEED-R7` is a **[MUST]** that a reader be able to tell
// §2.2.2's four outcomes apart — so a resolver whose outcome a renderer
// drops satisfies the MUST nowhere, which is D23 at field granularity and
// AP49's shape one boundary out.
//
// `shellcmd/follow_op.go` was written for this: the operations are
// renderer-neutral and the verb only renders. Nothing here reimplements a
// stage (AP57) — the four §2.4 facts about what a follow does and does
// not establish would then live in two places, and they are exactly the
// ones nobody rediscovers by reading code.
//
// # ⚠ Three exports, three different safety classes — do not collapse them
//
// This is `status.go`'s split and it is sharper here, because two of the
// three reach other machines.
//
//   - **`FeedFollowsRender` READS THIS PEER'S OWN TREE.** Dials nobody.
//     Safe on a wake and safe on a timer — and it should BE on a wake
//     rather than behind a refresh button, because follows are tree data
//     under `app/workbench/feed/` and **a refresh button on tree data is
//     a bug report about a missing subscription** (AP73).
//   - **`FeedTimelineRead` DIALS EVERY FOLLOWED PUBLISHER.** It must NOT
//     be wired to a wake or a timer: an open panel would become a thing
//     that contacts every peer you follow whenever the window is focused,
//     and an operator would leave it running overnight. It is a button.
//   - **`FeedTimelineCatchUp` dials AND advances durable cursors.** A
//     separate export from the read for the reason `shellcmd` separates
//     them: a surface somebody refreshes must not quietly change durable
//     state, and *"advanced"* must mean a position MOVED rather than that
//     a flag was passed.
//
// # One browser per peer handle, and that is a correctness requirement
//
// A feed is read through the same verifying reader a page is, and
// `BrowseModel` is what holds the per-publisher `seq` floor
// (`fetch.SeqFloor`, AP100). Building a fresh model per call would give
// every read a fresh floor, so a correctly-signed **rollback** replayed
// between two timeline reads would be undetectable — the exact defect
// AP100 was filed for, reintroduced at a different seam. So the handle
// owns one model for its lifetime.
//
// Synchronous exports, like the rest of this surface (AP31: a `*C.char`
// belongs to the .NET marshaller and is freed when the P/Invoke returns,
// so reading it on a goroutine is a use-after-free that reads as the
// empty string). The two that dial are long, so the caller runs them on a
// thread-pool worker.

/*
#include <stdlib.h>
#include <stdint.h>
*/
import "C"

import (
	"context"
	"sync"
	"time"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/shellcmd"
	wb "entity-workbench-go/workbench"
)

// --- DTOs -------------------------------------------------------------
//
// Flat and explicit, per AP49: `System.Text.Json` drops an undeclared
// field in total silence, and on this surface the dropped field is the
// one that says an entry is NOT attributable or that a reference has
// MOVED. A dropped provenance field renders as "everything is fine",
// which is the worst available failure for a surface whose job is to say
// otherwise.

type followRowDTO struct {
	Subject string `json:"subject"`
	Label   string `json:"label"`
	Via     string `json:"via"`

	// Reachable is whether this peer can currently reach the subject.
	// **Not a precondition and must not render as an error** — §2.4
	// requires no permission and no relationship, so a follow of a peer
	// who is asleep is a perfectly good follow.
	Reachable bool   `json:"reachable"`
	Why       string `json:"why"`
}

type followsDTO struct {
	Rows []followRowDTO `json:"rows"`

	// Problems includes §2.4's privacy sentence when it applies: a follow
	// record under `app/feed/` is published by the ordinary act of
	// publishing your feed, which turns a *separate, voluntary act* into
	// the default. Ours live under `app/workbench/feed/`, and this is
	// checked against what the peer actually publishes rather than
	// asserted — nothing else in the chain will mention it, because the
	// records are well-formed and the publish is correct.
	Problems []string `json:"problems"`
	Er       string   `json:"error"`
}

type timelineEntryDTO struct {
	Subject string `json:"subject"`
	Label   string `json:"label"`

	Hash      string `json:"hash"`
	Page      uint64 `json:"page"`
	CreatedAt uint64 `json:"createdAtMillis"`
	Text      string `json:"text"`
	MediaType string `json:"mediaType"`
	IsReply   bool   `json:"isReply"`

	// BodyRung is which step of `APP-CONVENTION-EMBED` §6's ladder produced
	// Text, and IsMarkdown is whether a markdown parser may touch it.
	//
	// **Both cross because a renderer cannot recover either from the
	// string.** `rendered` is the post; `fallback` is the author's
	// DESCRIPTION of a post that is not on screen; they are the same Go
	// type and the same JSON string, and showing them identically is
	// exactly the defect §4 of arch's `ROUTING-2026-09-17-a` named — a
	// conformant reader displaying an image post as its alt text.
	//
	// AP49: an undeclared field is dropped here in silence, and the failure
	// mode for THESE two is the confident direction — every body renders as
	// plain text, at a rung nobody can see.
	BodyRung   string `json:"bodyRung"`
	BodyNote   string `json:"bodyNote"`
	IsMarkdown bool   `json:"isMarkdown"`

	// Listed is whether §4.2's index named this entry, or §4.3 rule 6's
	// enumeration found it. Carried to a pixel because AP106: a
	// conformance fallback built to survive a hostile publisher will
	// equally survive a broken primary path and hide it, and the count
	// alone cannot tell the two apart.
	Listed bool `json:"listed"`

	// Attributed is `FEED-R4`. Attribution is why not, when not, and is
	// always populated when Attributed is false — *"unattributed"* on its
	// own reads as a defect in the reader.
	Attributed  bool   `json:"attributed"`
	Attribution string `json:"attribution"`

	// Rejected is `FEED-R1`: the entry claims an author other than the
	// namespace it was found under. The row is KEPT and its body is not
	// rendered; dropping it would make this list disagree with the index
	// it came from, and the rejected row is the interesting one.
	Rejected bool   `json:"rejected"`
	Problem  string `json:"problem"`
}

type timelineSourceDTO struct {
	Subject string `json:"subject"`
	Label   string `json:"label"`
	Count   int    `json:"count"`

	// Via is `index` or `enumeration` — which road §4.3 took.
	Via string `json:"via"`
	// Published is whether the subject has a published root at all.
	Published bool     `json:"published"`
	Prefix    string   `json:"prefix"`
	Freshness string   `json:"freshness"`
	Notes     []string `json:"notes"`

	// Err is why this source contributed nothing, when it did not. A
	// source that could not be reached is an EXCLUSION and is named as
	// one; dropping the row would make the view claim a completeness it
	// does not have.
	Err string `json:"error"`
}

type timelineDTO struct {
	// Sources is `FEED-R23`: a view of more than one publisher MUST
	// declare what produced it. A list of entries with no provenance is
	// exactly the view that MUST NOT exist, so this is never omitted and
	// the renderer is expected to show it even when every source is fine.
	Sources []timelineSourceDTO `json:"sources"`

	// Entries are grouped by source in Sources order and **deliberately
	// not interleaved by time**: §9.3 says there is no cross-publisher
	// order in the data, and `created_at` is an unverifiable clock
	// (§2.3.2) which sorting by would quietly promote to an authority.
	Entries []timelineEntryDTO `json:"entries"`

	// Advanced reports that this read MOVED a stored cursor — not that
	// the catch-up export was the one called.
	Advanced bool `json:"advanced"`

	Er string `json:"error"`
}

// refDTO is `FEED-R7`'s four outcomes, plus the fifth state.
//
// **`row` and `moved` are both carried, and that is not redundancy.** A
// caller switching on an enum can forget a case; a caller rendering
// provenance reads one boolean. The MUST is about the ABILITY TO TELL,
// so the surface gets both.
type refDTO struct {
	Peer string `json:"peer"`
	Path string `json:"path"`

	Row   string `json:"row"`
	Moved bool   `json:"moved"`

	Have       bool   `json:"have"`
	EntityType string `json:"entityType"`
	Resolved   string `json:"resolved"`
	Seen       string `json:"seen"`

	Provenance string `json:"provenance"`
	Prefix     string `json:"prefix"`
	Note       string `json:"note"`

	// Er is reserved for faults about the PUBLISHER — unreachable, never
	// published, root unverifiable, committed bytes withheld. Every
	// outcome that is about the reference arrives as a row with no error,
	// including the ones that look like failures. Collapsing them is the
	// thing `workbench.ResolveRef` exists to prevent.
	Er string `json:"error"`
}

// ownFeedDTO is THIS peer's own feed and whether anybody else can read it.
//
// # Why every reach field is carried separately
//
// `shellcmd.FeedReach` keeps three facts apart on purpose — unpublished,
// published under a prefix that does not cover the feed, and
// published-and-behind — because they are three different operator
// actions and the middle one is the quietest: everything looks published
// and the feed keys are not in the root. Collapsing them into one boolean
// here would undo that at the last boundary, which is AP49's shape: the
// model computes the distinction, the renderer never sees it, and the
// screen says "published" for all three.
//
// **`signatureNote` is the field this whole export exists for.** It is
// `FEED-R2`'s attribution caveat, and it had never crossed this boundary
// in any form — so a GUI operator's feed could be unattributable to every
// static reader with nothing on screen saying so, and no control to fix
// it. That is D23 at field granularity.
type ownFeedDTO struct {
	PeerID string `json:"peerId"`

	// Posted is whether an index head exists at all, and Entries counts
	// what was read. FALSE with zero entries and TRUE with zero entries are
	// different facts — *never posted* versus *a feed that is empty* — and
	// only the first is a reason to say "nothing here yet".
	Posted  bool `json:"posted"`
	Entries int  `json:"entries"`
	Pages   int  `json:"pages"`

	// Truncated reports that [ownFeedLimit] stopped the read before the
	// oldest page, so Entries is a floor and not a total.
	Truncated bool `json:"truncated"`

	// Published / Prefix / CoversFeed / Current are the three reach facts
	// plus what the root commits to.
	Published  bool   `json:"published"`
	Prefix     string `json:"prefix"`
	CoversFeed bool   `json:"coversFeed"`
	Current    bool   `json:"current"`

	// ContentSet is how the live root chose its bindings — "" for the scan,
	// "feed" for the curated set. It is what decides the other two below,
	// so it is carried rather than re-derived by the renderer.
	ContentSet string `json:"contentSet"`

	// SignatureNote is the attribution sentence, and EMPTY IS A REAL ANSWER:
	// it means the published root commits to the entries' signatures and to
	// little else, i.e. `A-38` ruling (D) has been applied. A renderer must
	// not treat empty as "unknown".
	SignatureNote string `json:"signatureNote"`

	// PublicPresent / PublicOurs are the `default` policy row, reused from
	// the publish surface so the two panels cannot describe one row
	// differently. A feed nobody is authorized to read is published in the
	// signing sense and readable by no stranger.
	PublicPresent bool `json:"publicPresent"`
	PublicOurs    bool `json:"publicOurs"`

	Problems []string `json:"problems"`
	Er       string   `json:"error"`
}

// ownFeedLimit bounds the read behind the count.
//
// The panel renders a summary rather than the entries, so this exists only
// to keep an enormous feed from being decoded for a number. `Truncated` is
// why the count is reported as "at least" past the limit rather than as a
// total — a surface that renders a limited list as the whole feed is
// telling the operator their older posts are gone.
const ownFeedLimit = 500

// --- per-peer browser -------------------------------------------------

var (
	feedMu       sync.Mutex
	feedBrowsers = map[int64]*wb.BrowseModel{}
)

// feedBrowserFor returns the one browser this peer reads feeds through.
//
// See the file header: one model per peer, for the whole process, because
// the model owns the per-publisher `seq` floor. Keyed on the PEER handle
// and not on a panel handle — two open feed panels on one peer must share
// a floor, or closing and reopening a panel would reset the memory of
// what `seq` this reader has already accepted.
func feedBrowserFor(peerHandle int64, ap *entitysdk.AppPeer) *wb.BrowseModel {
	feedMu.Lock()
	defer feedMu.Unlock()
	m := feedBrowsers[peerHandle]
	if m == nil {
		m = wb.NewBrowseModel(nil)
		feedBrowsers[peerHandle] = m
	}
	// Idempotent, and set on every call for `bareBrowserOf`'s reason: the
	// model outlives any one call and the peer is bound at bootstrap.
	m.SetPeer(ap)
	return m
}

// panelFeedReadTimeout bounds one timeline read.
//
// Long, because it is once per followed publisher and a followed peer
// that is switched off is the normal case — §2.4 makes following require
// nothing of the far end, so an unreachable subject is an ordinary row
// rather than an error.
const panelFeedReadTimeout = 120 * time.Second

// --- exports ----------------------------------------------------------

// FeedOwnRender reads THIS peer's own feed and whether anyone can read it.
//
// **A READ throughout.** It reads the local tree, rebuilds the trie over
// the published prefix in memory and reads the policy table; it mints
// nothing, writes nothing and dials nobody — which is what makes it safe
// on a wake, unlike `FeedTimelineRead` one export up. The distinction is
// the whole reason this file has three safety classes rather than one.
//
// It reads rather than publishes on purpose: `PublishStatus`'s rule is that
// a status surface that refreshed by minting would bump `seq` on a timer,
// which is a publisher claiming a new release every time somebody looks at
// it. The act is `PublishNow(handle, 0, feed: 1)` and it is a button.
//
//export FeedOwnRender
func FeedOwnRender(peerHandle C.int64_t) (result *C.char) {
	defer recoverToErrorEnvelope("FeedOwnRender", &result)
	ws, _, errEnv := shareWorkspace(peerHandle)
	if errEnv != "" {
		return C.CString(errEnv)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := ws.Feed(ctx, ownFeedLimit)
	if err != nil {
		return marshalReply(ownFeedDTO{Er: err.Error()}, "feed own")
	}
	return marshalReply(ownFeedToDTO(out), "feed own")
}

func ownFeedToDTO(out shellcmd.FeedOutcome) ownFeedDTO {
	probs := out.Problems
	if probs == nil {
		// Never null across the boundary: a C# `List<string>?` that arrives
		// null and one that arrives empty take different paths in the
		// renderer, and the difference is not meaningful here.
		probs = []string{}
	}
	return ownFeedDTO{
		PeerID:        out.PeerID,
		Posted:        out.Readout.Published,
		Entries:       len(out.Readout.Entries),
		Pages:         int(out.Readout.Pages),
		Truncated:     out.Readout.Truncated,
		Published:     out.Reach.Published,
		Prefix:        out.Reach.Prefix,
		CoversFeed:    out.Reach.CoversFeed,
		Current:       out.Reach.Current,
		ContentSet:    out.Reach.ContentSet,
		SignatureNote: out.Reach.SignatureNote,
		PublicPresent: out.Reach.Public.Present,
		PublicOurs:    out.Reach.Public.Ours,
		Problems:      probs,
	}
}

// FeedFollowsRender lists this peer's follows. Dials nobody.
//
//export FeedFollowsRender
func FeedFollowsRender(peerHandle C.int64_t) (result *C.char) {
	defer recoverToErrorEnvelope("FeedFollowsRender", &result)
	ws, _, errEnv := shareWorkspace(peerHandle)
	if errEnv != "" {
		return C.CString(errEnv)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := ws.FeedFollows(ctx)
	if err != nil {
		return marshalReply(followsDTO{Er: err.Error()}, "feed follows")
	}
	return marshalReply(followsToDTO(out), "feed follows")
}

// FeedFollowPeer follows a subject. Writes one local declaration and
// establishes NOTHING at the far end (§2.4).
//
//export FeedFollowPeer
func FeedFollowPeer(peerHandle C.int64_t, subject, label *C.char) (result *C.char) {
	defer recoverToErrorEnvelope("FeedFollowPeer", &result)
	// Copied before anything else happens. These belong to the .NET
	// marshaller (AP31); these exports are synchronous so the lifetime is
	// safe, and the copy is here so that adding a goroutine later cannot
	// silently reintroduce the use-after-free.
	subj, lbl := C.GoString(subject), C.GoString(label)

	ws, _, errEnv := shareWorkspace(peerHandle)
	if errEnv != "" {
		return C.CString(errEnv)
	}
	if _, err := ws.FeedFollow(shellcmd.FollowRequest{Subject: subj, Label: lbl, Via: subj}); err != nil {
		return marshalReply(followsDTO{Er: err.Error()}, "feed follow")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := ws.FeedFollows(ctx)
	if err != nil {
		return marshalReply(followsDTO{Er: err.Error()}, "feed follow")
	}
	return marshalReply(followsToDTO(out), "feed follow")
}

// FeedUnfollowPeer drops a follow and the cursor with it.
//
//export FeedUnfollowPeer
func FeedUnfollowPeer(peerHandle C.int64_t, subject *C.char) (result *C.char) {
	defer recoverToErrorEnvelope("FeedUnfollowPeer", &result)
	subj := C.GoString(subject)

	ws, _, errEnv := shareWorkspace(peerHandle)
	if errEnv != "" {
		return C.CString(errEnv)
	}
	if _, err := ws.FeedUnfollow(subj); err != nil {
		return marshalReply(followsDTO{Er: err.Error()}, "feed unfollow")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := ws.FeedFollows(ctx)
	if err != nil {
		return marshalReply(followsDTO{Er: err.Error()}, "feed unfollow")
	}
	return marshalReply(followsToDTO(out), "feed unfollow")
}

// FeedTimelineRead reads without touching the cursor. **Dials.**
//
// `subject` empty means every follow. `limit` 0 means everything the read
// reaches.
//
//export FeedTimelineRead
func FeedTimelineRead(peerHandle C.int64_t, subject *C.char, limit C.int) (result *C.char) {
	defer recoverToErrorEnvelope("FeedTimelineRead", &result)
	return feedTimeline(peerHandle, C.GoString(subject), int(limit), false, "feed timeline")
}

// FeedTimelineCatchUp reads from the stored cursor and ADVANCES it.
//
// Separate from the read so that a surface somebody refreshes cannot
// quietly change durable state — and note `advanced` in the reply means a
// position MOVED, not that this export was the one called.
//
//export FeedTimelineCatchUp
func FeedTimelineCatchUp(peerHandle C.int64_t, subject *C.char, limit C.int) (result *C.char) {
	defer recoverToErrorEnvelope("FeedTimelineCatchUp", &result)
	return feedTimeline(peerHandle, C.GoString(subject), int(limit), true, "feed catch up")
}

func feedTimeline(peerHandle C.int64_t, subject string, limit int, advance bool, what string) *C.char {
	ws, hp, errEnv := shareWorkspace(peerHandle)
	if errEnv != "" {
		return C.CString(errEnv)
	}
	ctx, cancel := context.WithTimeout(context.Background(), panelFeedReadTimeout)
	defer cancel()

	out, err := ws.FeedTimeline(ctx, shellcmd.TimelineRequest{
		Subject: subject, Limit: limit, New: advance,
	}, feedBrowserFor(int64(peerHandle), hp.AppPeer))
	if err != nil {
		return marshalReply(timelineDTO{Er: err.Error()}, what)
	}
	return marshalReply(timelineToDTO(out), what)
}

// FeedResolveRef resolves one reference and reports WHICH of §2.2.2's
// outcomes it got — `FEED-R7`, which is a MUST and had reached no pixel.
//
// **Dials**, through the same verifying reader and the same road chooser
// a page takes, so the `seq` floor stays one per publisher.
//
//export FeedResolveRef
func FeedResolveRef(peerHandle C.int64_t, ref *C.char) (result *C.char) {
	defer recoverToErrorEnvelope("FeedResolveRef", &result)
	raw := C.GoString(ref)

	_, hp, errEnv := shareWorkspace(peerHandle)
	if errEnv != "" {
		return C.CString(errEnv)
	}
	parsed, err := entitysdk.ParseRefURI(raw)
	if err != nil {
		return marshalReply(refDTO{Er: err.Error()}, "feed ref")
	}
	ctx, cancel := context.WithTimeout(context.Background(), panelFeedReadTimeout)
	defer cancel()

	out, err := feedBrowserFor(int64(peerHandle), hp.AppPeer).ResolveReference(ctx, parsed)
	if err != nil {
		// A fault about the PUBLISHER. Kept distinct from every outcome
		// about the reference, which is the whole reason this model has a
		// typed outcome instead of an `(entity, error)` pair.
		return marshalReply(refDTO{Peer: parsed.Peer, Path: parsed.Path, Er: err.Error()}, "feed ref")
	}
	return marshalReply(refToDTO(out), "feed ref")
}

// --- mapping ----------------------------------------------------------

func followsToDTO(out shellcmd.FollowsOutcome) followsDTO {
	d := followsDTO{Rows: []followRowDTO{}, Problems: []string{}}
	for _, r := range out.Rows {
		d.Rows = append(d.Rows, followRowDTO{
			Subject:   r.Follow.Subject,
			Label:     r.Follow.Label,
			Via:       r.Follow.Via,
			Reachable: r.Reachable,
			Why:       r.Why,
		})
	}
	if out.Problems != nil {
		d.Problems = out.Problems
	}
	return d
}

func timelineToDTO(out shellcmd.TimelineOutcome) timelineDTO {
	d := timelineDTO{
		Sources:  []timelineSourceDTO{},
		Entries:  []timelineEntryDTO{},
		Advanced: out.Advanced,
	}
	for _, s := range out.Sources {
		notes := s.Read.Notes
		if notes == nil {
			// Never null across the boundary: a C# `string[]?` that
			// arrives null and one that arrives empty take different
			// paths in every renderer, and the difference is not
			// meaningful here.
			notes = []string{}
		}
		d.Sources = append(d.Sources, timelineSourceDTO{
			Subject:   s.Subject,
			Label:     s.Label,
			Count:     len(s.Read.Entries),
			Via:       string(s.Read.Via),
			Published: s.Read.Published,
			Prefix:    s.Read.Prefix,
			Freshness: s.Read.Freshness,
			Notes:     notes,
			Err:       s.Err,
		})
	}
	for _, e := range out.Merged {
		d.Entries = append(d.Entries, timelineEntryDTO{
			Subject:     e.Subject,
			Label:       e.Label,
			Hash:        e.Entry.Hash.String(),
			Page:        e.Entry.Page,
			CreatedAt:   e.Entry.CreatedAt,
			Text:        e.Entry.Text,
			MediaType:   e.Entry.MediaType,
			IsReply:     e.Entry.IsReply,
			BodyRung:    string(e.Entry.BodyRung),
			BodyNote:    e.Entry.BodyNote,
			IsMarkdown:  e.Entry.IsMarkdown,
			Listed:      e.Entry.Listed,
			Attributed:  e.Entry.Attributed,
			Attribution: e.Entry.Attribution,
			Rejected:    e.Entry.Rejected,
			Problem:     e.Entry.Problem,
		})
	}
	return d
}

func refToDTO(out wb.RefOutcome) refDTO {
	d := refDTO{
		Peer:       out.Ref.Peer,
		Path:       out.Ref.Path,
		Row:        string(out.Row),
		Moved:      out.Moved,
		Have:       out.Have,
		Provenance: out.Provenance,
		Prefix:     out.Prefix,
		Note:       out.Note,
	}
	if out.Have {
		d.EntityType = out.Entity.Type
	}
	if !out.Resolved.IsZero() {
		d.Resolved = out.Resolved.String()
	}
	if !out.Seen.IsZero() {
		d.Seen = out.Seen.String()
	}
	return d
}
