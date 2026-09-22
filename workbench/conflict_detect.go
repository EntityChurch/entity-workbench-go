package workbench

import (
	"strings"
	"sync"
	"time"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/handler"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/localfiles"
)

// conflict_detect.go — telling "they changed this and I am behind" apart
// from "they changed this and so did I", and refusing to answer at scale.
//
// # The discriminator, and the version of it that does not work
//
// The recorder has always written who authored each chain position: a
// local edit reaches the tree through the WATCHER and records
// `local/files:watch`, a delivered one is dispatched by blob-resolve and
// records `local/files:write`. That is real and it is what makes the
// question answerable at all.
//
// **Reading it off the HEAD does not work, and the first version of this
// file did exactly that.** Every delivered file acquires a `watch`
// transition shortly after it lands, because the receiver's own watcher
// ingests the file blob-resolve just wrote and the file entity carries an
// mtime the watcher reads from the filesystem rather than from the write.
// Same bytes, different entity, real transition. So the head says `watch`
// for every file in a received folder and the detector flagged all of
// them — §5's storm, produced by the caution meant to prevent it, and
// caught by the anti-vacuity arm of the e2e test rather than by review.
//
// The question that survives is **who last changed the BYTES**:
// [localEditAwaitsDelivery] walks the run of consecutive transitions
// carrying the current content hash and reads the operation of the OLDEST
// member, which is the position that introduced them. An mtime-only echo
// lengthens the run and cannot change its oldest member.
//
// The known limit from the baseline transfers verbatim: a LOCAL caller
// dispatching `local/files:write` directly records as a delivery. Nothing
// in the shipped flow does that — `resolve -keep mine` deliberately
// writes through the filesystem for this reason (shellcmd/conflict_op.go).
//
// # Why "no chain" means overwrite and not conflict
//
// Because failing the other way is the storm.
// `SYNC-LIMITS-AND-FAILURE-MODES` §5 is about exactly this: a detection
// bug is not self-limiting — it fires on every file, every pass — where a
// genuine concurrent edit needs two people editing one file between two
// syncs. A path with no chain is the state of every folder that predates
// the reconciler's history config, so treating it as conflicted would
// conflict an entire folder on the first pass after an upgrade. Silence
// there is a KNOWN blind spot, stated on the record and in the docs,
// rather than a guess dressed as caution.
//
// # Why the limiter exists even so
//
// Same §5 rule, applied to the detection we do have. More than a small
// number of conflicts in one window is far more likely to be our bug than
// their editing, and the cheap wrong answer — carry on — is
// unrecoverable at scale. So past the limit the handler REFUSES the
// delivery: nothing is overwritten, nothing new is written, and the
// condition is reported. A refusal is recoverable by construction, since
// the catch-up supervisor re-derives the truth on its next pass and the
// window will have expired.

// Conflict-storm limits. Deliberately generous for a person and tight for
// a loop: a human editing the same folder as somebody else produces
// conflicts at the rate of one every few minutes, and a comparison bug
// produces one per file per pass.
const (
	// ConflictBurstLimit is how many conflicts may be acted on within one
	// window before the handler starts refusing.
	ConflictBurstLimit = 10
	// ConflictBurstWindow is that window.
	ConflictBurstWindow = 2 * time.Minute
)

// conflictLimiter is the rate limit, as a small sliding window.
//
// A slice of timestamps rather than a counter with a reset, because a
// counter that resets on a boundary lets 2×limit through across two
// adjacent windows — which for this feature means twice as many files
// touched by a bug we were trying to stop.
type conflictLimiter struct {
	mu     sync.Mutex
	events []time.Time
	// refused counts deliveries this limiter has turned away in this
	// process. Reported rather than merely counted: a peer that is
	// refusing deliveries is not healthy and not broken, and no other
	// number on any surface would say so.
	refused int
	// now is injectable so the window is testable without sleeping. Nil
	// means time.Now.
	now func() time.Time
}

func (l *conflictLimiter) clock() time.Time {
	if l.now != nil {
		return l.now()
	}
	return time.Now()
}

// admit reports whether one more conflict may be acted on, and records it
// when so.
func (l *conflictLimiter) admit() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock()
	cutoff := now.Add(-ConflictBurstWindow)
	kept := l.events[:0]
	for _, t := range l.events {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	l.events = kept
	if len(l.events) >= ConflictBurstLimit {
		l.refused++
		return false
	}
	l.events = append(l.events, now)
	return true
}

// ConflictHealth is what the conflict machinery has done in this process.
//
// Peer-level and process-scoped, the same shape and for the same reason
// as DeliveryHealth: it is a property of a running handler, not of any
// one folder, and a restart resets it. The DURABLE half is the conflict
// records in the tree, which is what a surface counts.
type ConflictHealth struct {
	// Detected is conflicts this process acted on.
	Detected int
	// Refused is deliveries turned away because the burst limit was hit.
	// Non-zero means files are NOT arriving, on purpose, and the operator
	// has to be told: it is the one state in this feature where the
	// product has deliberately stopped working.
	Refused int
	// Storming is whether the limiter is currently at its limit.
	Storming bool
	// Limit and WindowSeconds describe the rule in force, so a surface
	// can say why rather than only that.
	Limit         int
	WindowSeconds float64
}

// ConflictHealth reports the limiter's state.
func (h *BlobResolveHandler) ConflictHealth() ConflictHealth {
	h.conflicts.mu.Lock()
	defer h.conflicts.mu.Unlock()
	now := h.conflicts.clock()
	cutoff := now.Add(-ConflictBurstWindow)
	live := 0
	for _, t := range h.conflicts.events {
		if t.After(cutoff) {
			live++
		}
	}
	return ConflictHealth{
		Detected:      h.conflictsDetected,
		Refused:       h.conflicts.refused,
		Storming:      live >= ConflictBurstLimit,
		Limit:         ConflictBurstLimit,
		WindowSeconds: ConflictBurstWindow.Seconds(),
	}
}

// conflictChainWalkLimit bounds the walk in localEditAwaitsDelivery.
//
// A run of same-content transitions longer than this is not a shape any
// known mechanism produces, so hitting the bound means the chain is
// telling us something we do not understand — and the answer to that is
// "not a conflict", never a guess.
const conflictChainWalkLimit = 32

// localEditAwaitsDelivery reports whether the BYTES currently at treePath
// were authored by a LOCAL edit rather than by a delivery, and whether
// the question could be answered at all.
//
// The second return is not a detail. `false` means there is no chain to
// read — recording was never enabled for the path, or the growth guard
// stopped it — and the caller must treat that as "not a conflict" rather
// than as "not local". Collapsing the two is how a folder conflicts
// itself wholesale on the first pass after an upgrade.
//
// # Why this walks instead of reading the head, which is what the
// baseline test appeared to establish
//
// The head transition's operation is NOT the answer, and believing it was
// produced a detector that flagged every file in a received folder.
// Measured 2026-09-07 (`shellboot/conflict_e2e_test.go`, which prints the
// chains): a file the receiver has never touched ends up as
//
//	[0] updated local/files:watch
//	[1] created local/files:write
//
// because blob-resolve's `local/files:write` puts the bytes on disk and
// the receiver's OWN WATCHER then ingests the file it just wrote. The
// bytes are identical, but a file entity carries `modified_at`, and the
// watcher reads the filesystem's mtime rather than the one the write
// recorded — so the entity hash differs and the transition is real.
//
// `TestM3Baseline_ConcurrentEditLeavesBothWritesOnTheChain` asserts
// `trans[0].Operation == "write"` and passes, which is what made this
// look settled. It reads the chain within a second of the delivery, i.e.
// before the echo lands. **The provenance distinction is true at the
// instant of delivery and erased shortly afterwards** — a measurement
// taken at one moment, read as a property.
//
// So the question is not "who wrote the last transition" but "who last
// changed the BYTES". That is the run of consecutive transitions carrying
// the current content hash: the OLDEST member of that run is the one that
// introduced these bytes, and its operation is the author. An mtime-only
// echo extends the run and cannot change its oldest member, which is
// exactly the robustness the head-read lacked.
func localEditAwaitsDelivery(hctx *handler.HandlerContext, treePath string) (local bool, known bool) {
	if hctx == nil || hctx.Store == nil || hctx.LocationIndex == nil {
		return false, false
	}
	qualified := treePath
	if hctx.LocalPeerID != "" {
		qualified = "/" + string(hctx.LocalPeerID) + "/" + treePath
	}
	// The recorder's head pointer path: headPrefix + the tracked path
	// minus its leading slash (ext/history/recorder.go, headPointerPath).
	headPath := HistoryHeadPrefix + strings.TrimPrefix(qualified, "/")
	if hctx.LocalPeerID != "" {
		headPath = "/" + string(hctx.LocalPeerID) + "/" + headPath
	}
	cur, ok := hctx.LocationIndex.Get(headPath)
	if !ok {
		return false, false
	}

	var (
		runContent hash.Hash
		haveRun    bool
		runAuthor  string
	)
	for i := 0; i < conflictChainWalkLimit && !cur.IsZero(); i++ {
		ent, ok := hctx.Store.Get(cur)
		if !ok {
			break
		}
		var td types.TransitionData
		if err := ecf.Decode(ent.Data, &td); err != nil {
			break
		}
		content, ok := transitionContentHash(hctx, td)
		if !ok {
			// A position we cannot read the bytes of. Everything older is
			// then unreadable for this purpose too, so stop rather than
			// skip: skipping would join two runs that are not adjacent.
			break
		}
		if !haveRun {
			runContent, haveRun = content, true
		} else if content != runContent {
			// The previous position introduced the current bytes.
			return runAuthor == "watch", true
		}
		// The operation, not the handler: `local/files` is both sides.
		runAuthor = td.Operation
		cur = td.Previous
	}
	if !haveRun {
		return false, false
	}
	// The run reached the start of the chain (or the walk bound): the
	// oldest position we saw is the one that introduced these bytes.
	return runAuthor == "watch", true
}

// transitionContentHash reads the blob hash the file entity at one chain
// position bound.
//
// Reports false when the position binds no readable file entity — a
// deletion, a non-file entity, or content that is no longer present. The
// caller stops rather than treating it as a change, because "I cannot see
// these bytes" and "these bytes are different" are different facts and
// only one of them means a conflict.
func transitionContentHash(hctx *handler.HandlerContext, td types.TransitionData) (hash.Hash, bool) {
	if td.Hash.IsZero() {
		return hash.Hash{}, false
	}
	ent, ok := hctx.Store.Get(td.Hash)
	if !ok || ent.Type != localfiles.TypeFile {
		return hash.Hash{}, false
	}
	f, err := localfiles.FileDataFromEntity(ent)
	if err != nil {
		return hash.Hash{}, false
	}
	return f.Content, true
}

// HistoryHeadPrefix is where the recorder writes one pointer per tracked
// path.
//
// Restated from `ext/history/recorder.go`'s unexported headPrefix, and
// exported here because two packages now depend on it — this file and
// `shellcmd/history_budget.go`. One definition, asserted against the
// shape the recorder actually writes rather than against itself.
const HistoryHeadPrefix = "system/history/head/"

// operatorDeclinedDelivery reports whether the operator has already
// looked at exactly this collision and chosen their own version.
//
// This is what makes `resolve -keep mine` STICK. Without it the choice
// survives until the next catch-up pass and no longer: the pass asks the
// sender what it holds, gets the same version back, sees a differing
// hash, and materializes it — correctly, by every rule the handler has,
// and against an instruction the operator gave five minutes earlier.
//
// The record does the work, so there is no second mechanism to keep in
// step: its key is (path, mine, theirs), which is precisely the identity
// of one collision, and a resolution of `mine` on that key is the
// operator saying no to that exact delivery. A DIFFERENT version from the
// sender is a different key and is not declined — the operator declined a
// version, not a peer.
func operatorDeclinedDelivery(hctx *handler.HandlerContext,
	qualifiedPath string, mine, theirs hash.Hash) bool {

	if hctx == nil || hctx.Store == nil || hctx.LocationIndex == nil {
		return false
	}
	key := ConflictKey(qualifiedPath, mine, theirs)
	path := ConflictPrefix + key
	if hctx.LocalPeerID != "" {
		path = "/" + string(hctx.LocalPeerID) + "/" + path
	}
	h, ok := hctx.LocationIndex.Get(path)
	if !ok {
		return false
	}
	ent, ok := hctx.Store.Get(h)
	if !ok || ent.Type != ConflictType {
		return false
	}
	var c ConflictData
	if err := ecf.Decode(ent.Data, &c); err != nil {
		return false
	}
	return c.Kept == ConflictKeptMine
}

// conflictOutcome is what the handler decided to do about one delivery.
type conflictOutcome int

const (
	// conflictNone — no local edit is at risk. Materialize as usual.
	conflictNone conflictOutcome = iota
	// conflictRecord — a local edit is being replaced. Materialize, and
	// record what was replaced.
	conflictRecord
	// conflictKeepBoth — as conflictRecord, plus write the replaced
	// version to its sibling path.
	conflictKeepBoth
	// conflictRefuse — the burst limit is hit. Materialize NOTHING and
	// report; the catch-up supervisor re-derives on its next pass.
	conflictRefuse
	// conflictDeclined — the operator has already resolved this exact
	// collision in favour of their own version. Materialize nothing and
	// say so; this is a decision being honoured, not a failure.
	conflictDeclined
)

// folderConflictPolicy reads the declared conflict policy for the folder
// that receives into targetPrefix, and the root it names.
//
// Read from the TREE at delivery time rather than registered alongside
// the mount, and that is the control-loop rule rather than a preference:
// the policy is a DECLARATION, so a copy held in handler memory is a
// second representation that goes stale the moment an operator changes
// it — which is the shape every restart defect in this flow has had. It
// costs one indexed list and a decode, and only on the conflict path, so
// an ordinary delivery pays nothing for it.
//
// An unreadable or absent declaration yields the default policy. There is
// no error return on purpose: a folder we cannot find a declaration for
// still has a delivery to handle, and refusing it would make a missing
// record into a delivery outage.
func folderConflictPolicy(hctx *handler.HandlerContext, targetPrefix string) (policy, root string) {
	root = strings.TrimSuffix(strings.TrimPrefix(targetPrefix, LocalFilesSourcePrefix), "/")
	policy = ConflictPolicyRecord
	if hctx == nil || hctx.Store == nil || hctx.LocationIndex == nil || hctx.LocalPeerID == "" {
		return policy, root
	}
	prefix := "/" + string(hctx.LocalPeerID) + "/" + FolderPrefix
	for _, e := range hctx.LocationIndex.List(prefix) {
		ent, ok := hctx.Store.Get(e.Hash)
		if !ok || ent.Type != FolderType {
			continue
		}
		var f FolderData
		if err := ecf.Decode(ent.Data, &f); err != nil {
			continue
		}
		// ReceivingRoot and never Root: the two differ exactly when an
		// operator accepted a folder into a directory of their own
		// choosing, and reaching for Root gives the answer that is right
		// in the symmetric case and silently wrong in the one the field
		// exists for.
		if f.ReceivingRoot() == root {
			return f.ConflictPolicy(), root
		}
	}
	return policy, root
}

// hashPrefix8 is the 8-hex-character form used in a keep-both sibling
// name and in operator-facing lines. Same four digest bytes the kernel's
// keep-both uses, so the two never disagree about which version a name
// refers to.
func hashPrefix8(h hash.Hash) string { return KeepBothSuffix(h)[len(".keep-both-"):] }
