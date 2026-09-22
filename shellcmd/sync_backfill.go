package shellcmd

// Backfill — the files that were ALREADY in the folder when the sync
// was established.
//
// # The defect this closes
//
// `Sync` subscribes with Events{"created","updated"}. A subscription is
// a future tense: it reports what happens after it exists. So a folder
// that already had files in it transferred NOTHING, because no file in
// it ever "changed" again — and the operator, who had done exactly what
// the product is named after (pick a directory, share it with a peer),
// watched an empty folder and correctly concluded that sync was broken.
//
// `USAGE-SHARE-A-FOLDER.md` disclosed this, in its last section, as
// "history replay ... files already sitting in A's folder arrive when
// they next change, or when A remounts." That is an accurate sentence
// about the implementation and a wrong one about the product: a file
// share whose first act is not to share the files is not a file share.
// A known-limitations entry is not a substitute for the feature, and
// the fact that it was written down is why nobody re-derived it from
// the symptom for a week.
//
// # Why this is not a second implementation of the flow
//
// The rule in AGENTS.md is that the share flow has one implementation
// and renderers are thin over it; the same rule binds this. Backfill
// does NOT re-derive "pull a blob and write a file". It synthesizes the
// notification the subscription engine would have delivered had the
// file changed, and dispatches it at the SAME handler
// (`workbench/blob-resolve:receive`) through `ExecuteWithIncluded`,
// which is the same shape the live delivery arrives in — the changed
// entity presented in HandlerContext.Included.
//
// Everything downstream is therefore shared with the live path by
// construction, not by inspection: the mount lookup, the F9
// already-current short-circuit, the capability-checked cross-peer blob
// closure pull, the `local/files:write` materialization, and the
// deletion branch's absence (a backfill has nothing to delete). If the
// live path is correct, this is; if it regresses, both regress
// together, which is the only kind of sharing worth having here.

import (
	"fmt"
	"sort"
	"strings"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"
)

// maxBackfillErrors bounds what we carry back to a surface. The count
// is always exact; only the transcript is capped, because a folder that
// fails 4,000 files has one cause and a renderer does not need 4,000
// lines of it to say so.
const maxBackfillErrors = 12

// backfillWalkLimit bounds the remote enumeration. A sync points at
// somebody else's machine, and "how many entries are under this prefix"
// is their answer, not ours — an unbounded walk driven by a remote
// response is a denial of service with our own CPU. Hitting the limit
// is REPORTED, never silently truncated: a partial backfill that claims
// to be complete is the failure mode this whole file exists to fix.
const backfillWalkLimit = 20000

// BackfillResult is what a catch-up pass actually did. Every field is a
// count of an outcome that occurred, so a surface can state the result
// rather than implying one from the absence of an error.
type BackfillResult struct {
	// Scanned is remote entries examined (files only; directory nodes
	// are walked but not counted).
	Scanned int

	// Materialized is files whose bytes were pulled across and written
	// locally by this pass.
	Materialized int

	// AlreadyCurrent is files the local tree already had at the same
	// content hash — the F9 short-circuit. On a re-run this is the
	// number that should be large, and it is the cheap proof that a
	// second pass is idempotent rather than a second transfer.
	AlreadyCurrent int

	// Skipped is entries under the prefix that are not file entities.
	Skipped int

	// Failed is files this pass could not materialize.
	Failed int

	// Errors is a bounded transcript. Len may be < Failed.
	Errors []string

	// Truncated reports that the remote enumeration hit
	// backfillWalkLimit and this result describes a PREFIX of the
	// folder, not the folder.
	Truncated bool

	// Unreachable reports that the remote folder could not be LISTED at
	// all, so this result describes nothing about their folder.
	//
	// It exists because the alternative is the trap this repo already
	// named once about empty registries: `Scanned == 0` has two causes —
	// their folder is empty, and we could not ask — and rendering them
	// identically turns "we do not know" into the confident claim
	// "there is nothing there". The common cause is the grant not
	// having reached us yet, which is a state the operator can fix and
	// will never think to if the surface says the folder is empty.
	Unreachable bool

	// ListError is why the enumeration failed, verbatim.
	ListError string
}

// Complete reports whether the pass covered everything it found.
func (r BackfillResult) Complete() bool {
	return r.Failed == 0 && !r.Truncated && !r.Unreachable
}

// Summary is a one-line rendering for a surface that has room for one
// line. It always states the failures, because a summary that reads as
// success while files are missing is the shape of the original bug.
func (r BackfillResult) Summary() string {
	if r.Unreachable {
		// Never "the folder is empty". We did not see the folder.
		return "could not read their folder — " + r.ListError +
			" (usually their grant has not reached this peer yet; the subscription is " +
			"established, so retry with a resync)"
	}
	if r.Scanned == 0 {
		return "no files in the remote folder yet"
	}
	parts := []string{fmt.Sprintf("%d file(s) found", r.Scanned)}
	if r.Materialized > 0 {
		parts = append(parts, fmt.Sprintf("%d transferred", r.Materialized))
	}
	if r.AlreadyCurrent > 0 {
		parts = append(parts, fmt.Sprintf("%d already current", r.AlreadyCurrent))
	}
	if r.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d not a file", r.Skipped))
	}
	if r.Failed > 0 {
		parts = append(parts, fmt.Sprintf("%d FAILED", r.Failed))
	}
	if r.Truncated {
		parts = append(parts, fmt.Sprintf("stopped at the %d-entry walk limit — this is a PREFIX of the folder", backfillWalkLimit))
	}
	return strings.Join(parts, ", ")
}

// backfill drives every file already present under the remote's
// sourcePrefix through the live materialization path.
//
// It is deliberately synchronous and deliberately best-effort: a
// failure here does not undo the subscription, because the subscription
// is the durable half and a folder that catches up on the next write is
// strictly better than no relationship at all. The caller reports what
// happened; it does not unwind.
func (ws *ShellWorkspace) backfill(
	remotePeerID, sourcePrefix, targetPrefix, subscriptionID string,
) BackfillResult {
	var res BackfillResult
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		res.Failed++
		res.Errors = append(res.Errors, "workspace has no local peer")
		return res
	}
	local := ws.Local.Peer

	// AP11: a dispatched read of a peer-qualified path is a REMOTE
	// read. That is exactly what is wanted here — this must be their
	// current folder, not our cached idea of it — but it is also why
	// every one of these calls can fail for their reasons, and why the
	// error text names the peer.
	paths, truncated, listErr := ws.walkRemoteFiles(local, remotePeerID, sourcePrefix)
	res.Truncated = truncated
	if listErr != nil {
		// The BASE prefix failed, so we learned nothing about their
		// folder. Reported as its own state rather than folded into
		// Failed, because "one file did not transfer" and "we never saw
		// the folder" send an operator to different places.
		res.Unreachable = true
		res.ListError = listErr.Error()
		return res
	}

	for _, remotePath := range paths {
		res.Scanned++

		ent, found, err := local.Get(remotePath)
		if err != nil {
			res.Failed++
			ws.noteBackfillError(&res, fmt.Sprintf("read %s from %s: %v",
				remotePath, shortPeer(remotePeerID), err))
			continue
		}
		if !found {
			// Raced with a delete on their side. Not an error: the
			// subscription is live by now and will carry the deletion
			// if it matters.
			res.Skipped++
			continue
		}

		outcome, err := ws.materializeOne(remotePeerID, remotePath, sourcePrefix, ent, subscriptionID)
		switch {
		case err != nil:
			res.Failed++
			ws.noteBackfillError(&res, fmt.Sprintf("%s: %v", trimPeerQualified(remotePath), err))
		case outcome == materializedAlreadyCurrent:
			res.AlreadyCurrent++
		case outcome == materializedSkipped:
			res.Skipped++
		default:
			res.Materialized++
		}
	}

	sort.Strings(res.Errors)
	return res
}

// walkRemoteFiles enumerates leaf paths under a remote prefix,
// descending into subdirectories. Returns the paths and whether the
// walk was cut short by backfillWalkLimit.
//
// Listing is breadth-first with an explicit queue rather than
// recursion: the depth is chosen by the remote peer, and a recursive
// walk over a remote-supplied tree is a stack overflow with someone
// else's finger on the trigger.
func (ws *ShellWorkspace) walkRemoteFiles(
	local *entitysdk.AppPeer, remotePeerID, sourcePrefix string,
) ([]string, bool, error) {
	base := "/" + remotePeerID + "/" + sourcePrefix
	queue := []string{base}
	var out []string
	seen := map[string]bool{base: true}

	for len(queue) > 0 {
		if len(out) >= backfillWalkLimit {
			return out, true, nil
		}
		dir := queue[0]
		queue = queue[1:]

		entries, err := local.List(dir)
		if err != nil {
			// The BASE prefix is special: failing it means we never saw
			// their folder, and a caller that reports that as "zero
			// files" states a fact about their machine that it does not
			// have. A SUBDIRECTORY that fails mid-walk is different —
			// the rest of the walk is still real — so it contributes
			// nothing and does not abort.
			if dir == base {
				return nil, false, err
			}
			continue
		}
		for _, e := range entries {
			// Normalize to the peer-qualified form explicitly rather
			// than trusting whatever shape List happened to return.
			// This is AP58's neighbourhood: entries come back qualified
			// today, but a RELATIVE path handed to `local.Get` would
			// read our OWN tree instead of theirs — a local read
			// wearing a remote read's clothes, which would "succeed",
			// materialize nothing new, and report every file as
			// already-current. Cheap to make unambiguous; expensive to
			// debug if it ever changes.
			p := qualifyTo(remotePeerID, e.Path)
			if p == "" || seen[p] {
				continue
			}
			seen[p] = true
			if e.HasChildren {
				queue = append(queue, strings.TrimSuffix(p, "/")+"/")
				continue
			}
			out = append(out, p)
			if len(out) >= backfillWalkLimit {
				return out, true, nil
			}
		}
	}
	sort.Strings(out)
	return out, false, nil
}

type materializeOutcome int

const (
	materializedWritten materializeOutcome = iota
	materializedAlreadyCurrent
	materializedSkipped
)

// materializeOne synthesizes the delivery the subscription engine would
// have produced for this file and dispatches it at the real handler.
//
// The URI shape is load-bearing and is the handler's own contract:
// `splitPeerIDFromURI` expects `/{sourcePeerID}/{relativePath}` and the
// mount table is keyed on the relative part, so a URI built any other
// way produces a 404 no_mount_for_uri that looks like a routing bug.
func (ws *ShellWorkspace) materializeOne(
	remotePeerID, remotePath, sourcePrefix string, ent entity.Entity, subscriptionID string,
) (materializeOutcome, error) {
	local := ws.Local.Peer

	entHash := ent.ContentHash
	if entHash.IsZero() {
		return materializedSkipped, fmt.Errorf("remote entity has no content hash")
	}

	relative := trimPeerQualified(remotePath)
	if !strings.HasPrefix(relative, sourcePrefix) {
		return materializedSkipped, fmt.Errorf(
			"remote path %q is not under the synced prefix %q", relative, sourcePrefix)
	}

	notif := types.SubscriptionNotificationData{
		SubscriptionID: subscriptionID,
		Event:          "created",
		URI:            "/" + remotePeerID + "/" + relative,
		Hash:           entHash,
	}
	notifEnt, err := notif.ToEntity()
	if err != nil {
		return materializedSkipped, fmt.Errorf("build notification: %w", err)
	}

	resp, err := local.Executor().ExecuteWithIncluded(
		workbench.BlobResolvePattern, "receive", notifEnt, nil,
		map[hash.Hash]entity.Entity{entHash: ent},
	)
	if err != nil {
		return materializedSkipped, err
	}
	if resp == nil {
		return materializedSkipped, fmt.Errorf("blob-resolve returned no response")
	}
	if resp.Status >= 400 {
		return materializedSkipped, fmt.Errorf("blob-resolve status %d", resp.Status)
	}

	// The handler reports its own short-circuits in the ack body, and
	// they are the difference between "we transferred this" and "this
	// was already here" — a distinction the operator is specifically
	// trying to see when they run a second pass to check the first one.
	reason, skipped := ackSkipReason(resp)
	if skipped {
		if reason == "already_current" {
			return materializedAlreadyCurrent, nil
		}
		return materializedSkipped, nil
	}
	return materializedWritten, nil
}

// ackSkipReason reads the handler's ack for its skip flag. Best-effort:
// an ack we cannot decode is treated as a successful write, which is
// the direction that over-reports transfers rather than under-reporting
// them — a wrong "transferred" is caught by the next pass showing
// "already current", while a wrong "skipped" hides a real file.
func ackSkipReason(resp *entitysdk.Response) (string, bool) {
	if resp == nil {
		return "", false
	}
	var fields map[string]interface{}
	if err := ecf.Decode(resp.Data, &fields); err != nil || fields == nil {
		return "", false
	}
	if v, ok := fields["skipped"]; !ok || v != true {
		return "", false
	}
	reason, _ := fields["reason"].(string)
	return reason, true
}

func (ws *ShellWorkspace) noteBackfillError(res *BackfillResult, msg string) {
	if len(res.Errors) < maxBackfillErrors {
		res.Errors = append(res.Errors, msg)
	}
}

// trimPeerQualified drops a leading `/{peerID}/` from a store path.
//
// It exists rather than a TrimPrefix at each call site because of AP58:
// entries come back peer-qualified and trimming them with a relative
// prefix silently removes nothing, and the comparison after it then
// gets a confidently wrong answer.
func trimPeerQualified(p string) string {
	if !strings.HasPrefix(p, "/") {
		return p
	}
	rest := p[1:]
	i := strings.Index(rest, "/")
	if i < 0 {
		return rest
	}
	return rest[i+1:]
}

// qualifyTo returns path in `/{peerID}/{relative}` form.
//
// Idempotent: a path already qualified to this peer is returned
// unchanged, so applying it is never wrong. A path qualified to a
// DIFFERENT peer is returned unchanged too — it is not ours to rewrite,
// and silently re-homing it would manufacture a read against a peer the
// listing never named.
func qualifyTo(peerID, path string) string {
	if path == "" || peerID == "" {
		return path
	}
	if strings.HasPrefix(path, "/") {
		return path
	}
	return "/" + peerID + "/" + path
}

func shortPeer(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12] + "…"
}
