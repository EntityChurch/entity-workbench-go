package shellcmd

import (
	"fmt"
	"sort"
	"strings"

	"entity-workbench-go/workbench"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/types"
)

// delivery_failures.go — THE FAILURES THE TREE ALREADY RECORDED AND
// NOTHING READ.
//
// # The morning this exists for
//
// On 2026-09-10 a kernel change made every cross-peer blob fetch answer
// `403 capability_denied`. Each failure bound a `system/runtime/
// chain-error-lost` marker into this peer's own tree, with the status,
// the code, the target peer and the URI. Hundreds of them.
//
// **Every surface in the product said nothing.** The folder row drew a
// two-way arrow, the file counts were healthy (they count the local
// mount), the peer was connected, and the operator's summary of the
// experience was that nothing in the product told them what was
// happening. They were right, and the diagnosis was sitting in their
// own store the whole time.
//
// The only reader of that subtree was `inspect watch`, a raw tree-walking
// debug verb. `status.go` even names chain-errors in a comment as one of
// the two ways an inbound failure is *observed* — a comment describing a
// capability nothing implemented, which is AP45's shape: a sentence that
// closes the question permanently by making it sound answered.
//
// # Why this is a READ
//
// It is a local prefix walk over our own tree. It dials nothing and asks
// nobody, so a status panel may run it on every refresh, which is the
// constraint that decides where sharing diagnostics are allowed to live.
//
// # What it deliberately filters, and why filtering is stated
//
// A peer we cannot reach writes one marker per status transition, from a
// separate defect in the reconnect lifecycle that is routed and not ours
// (`system/network:restore-subscriptions` answering 404 into a
// continuation with no `on_error`). At the observed rate that is
// thousands of markers a day, all of them saying the same thing, none of
// them about a file.
//
// Surfacing those would bury the ones that matter — which is the failure
// mode of every alert system ever built, and the reason a banner nobody
// reads is worse than no banner. So they are excluded BY TARGET, and the
// count of what was excluded is reported rather than dropped: an operator
// told "3 deliveries failed" when 4,000 markers exist has been given a
// number they cannot reconcile with anything else they might look at.

// DeliveryFailure is one recorded failure to move a file across.
type DeliveryFailure struct {
	// Code is the failure's own code — `capability_denied`, `not_found`.
	// The single most diagnostic field, and the one that distinguishes
	// "they revoked us" from "the folder moved".
	Code string
	// Status is the HTTP-shaped status the far end answered.
	Status uint
	// TargetPeerID is who we were talking to — when the marker says, which
	// as of 2026-09-10 is NEVER.
	//
	// §3.10.6 reserves `target_peer_id` on the `lost` kind for exactly this
	// ("sender-side capture: the peer the dispatch was aimed at"), and no
	// writer in the kernel populates it: the only assignments to a field of
	// that name in `../entity-core-go` are `PinnedEntry.TargetPeerID`, an
	// unrelated registry type. Both marker writers leave it zero.
	//
	// So this is read hopefully and rendered only when present. It is kept
	// rather than deleted because the field is the spec's, the gap is the
	// implementation's, and a marker that named its peer would turn "a
	// transfer failed" into "a transfer to THAT machine failed" — which is
	// the difference between a diagnosis and a mood. Routed; until it lands,
	// a surface must not imply it knows.
	TargetPeerID string
	// TargetURI is what we were asking for.
	TargetURI string
	// AtMillis is when it happened. Unix milliseconds, captured at
	// failure-origination time by the kernel.
	AtMillis uint64
}

// DeliveryFailureReport is the bounded summary a surface renders.
//
// A COUNT plus a few examples, never the whole set: the retention window
// is 24 hours and the observed marker rate makes the full list useless to
// a human and expensive to render. The count is the fact; the examples
// are what makes it actionable.
type DeliveryFailureReport struct {
	// Total is how many file-transfer failures are recorded.
	Total int
	// Recent is the newest few, freshest first, capped at
	// maxDeliveryFailureExamples.
	Recent []DeliveryFailure
	// Excluded is how many markers were skipped as reconnect-lifecycle
	// noise. Reported so a number here can be reconciled with what
	// `inspect` shows, rather than silently disagreeing with it.
	Excluded int
	// Observable is false when the subtree could not be read at all.
	// "No failures" and "we could not look" are different claims and a
	// surface that renders them identically is lying about one of them —
	// the same rule mountFileCounts follows.
	Observable bool
}

// chainErrorPrefix is where the kernel binds lost-error markers.
const chainErrorPrefix = "system/runtime/chain-errors/"

// maxDeliveryFailureExamples bounds what a surface shows. Small on
// purpose: three is enough to see whether they are all the same fault,
// which is the only question a summary can answer.
const maxDeliveryFailureExamples = 3

// maxDeliveryFailureScan bounds the walk. A retention window's worth of
// markers can be thousands, and this runs on every status read.
const maxDeliveryFailureScan = 2000

// DeliveryFailures reads the chain-error markers that mean a file did not
// arrive.
//
// Never returns an error: a diagnostic surface that can itself fail is
// one more thing to diagnose. Unreadable state comes back as
// Observable=false, which a renderer must say out loud.
func DeliveryFailures(st *workbench.Store) DeliveryFailureReport {
	rep := DeliveryFailureReport{}
	if st == nil {
		return rep
	}
	entries := st.List(chainErrorPrefix)
	rep.Observable = true

	scanned := 0
	for _, e := range entries {
		if scanned >= maxDeliveryFailureScan {
			break
		}
		scanned++
		ent, ok := st.Get(e.Path)
		if !ok || ent.Type != types.TypeChainErrorLost {
			continue
		}
		var d types.ChainErrorLostData
		if err := ecf.Decode(ent.Data, &d); err != nil {
			continue
		}
		if isReconnectLifecycleNoise(d) {
			rep.Excluded++
			continue
		}
		rep.Total++
		rep.Recent = append(rep.Recent, DeliveryFailure{
			Code:         firstNonEmpty(d.Code, d.Reason),
			Status:       d.Status,
			TargetPeerID: d.TargetPeerID,
			TargetURI:    d.TargetURI,
			AtMillis:     d.Timestamp,
		})
	}

	// Freshest first: the current fault is the one worth showing, and a
	// day-old one that has since been fixed is actively misleading.
	sort.Slice(rep.Recent, func(i, j int) bool { return rep.Recent[i].AtMillis > rep.Recent[j].AtMillis })
	if len(rep.Recent) > maxDeliveryFailureExamples {
		rep.Recent = rep.Recent[:maxDeliveryFailureExamples]
	}
	return rep
}

// networkHandlerPath is the handler the reconnect lifecycle dispatches at.
const networkHandlerPath = "system/network"

// isReconnectLifecycleNoise reports whether a marker came from the
// reconnect lifecycle rather than from a file transfer.
//
// Matched on the TARGET, not on the reason: `not_found` is a perfectly
// real answer about a file, and excluding by code would hide a genuine
// fault to suppress a known one. The noise is specifically a dispatch at
// `system/network` — the resubscribe continuation whose 404 has nowhere
// to go — and nothing about a file is ever aimed there.
//
// # This filter was DEAD for its first day, and the shape is the lesson
//
// It matched `"/system/network"`, with a leading slash, because the test
// fixture beside it wrote `entity://{peer}/system/network` and the
// qualified form was assumed to be the only one. The kernel emits the
// BARE handler path: an operator's run on 2026-09-10 shows
// `failed_uri=system/network` in every marker, and neither writer
// (`ext/continuation/advance.go`, `ext/subscription/chain_error_lost.go`)
// qualifies it — TargetURI is whatever the dispatch named.
//
// So every reconnect marker was counted as a failed file transfer. The
// operator was told **"20 file transfer(s) failed"** with `(system/network)`
// printed as the URI, followed by an invented explanation — *"the content
// was not there when we asked"* — about content that was never involved.
// A brand-new surface, built to end a morning of being told nothing, spent
// its first day confidently saying something false instead.
//
// The fixture is why nothing caught it: it did not omit what production
// has, it ADDED what production does not. That is AP58 inverted and it is
// harder to see, because the invented value makes the code look more
// careful rather than less. Both forms are handled below and both are
// tested, but the arm that matters carries the byte-exact string off the
// operator's own log.
func isReconnectLifecycleNoise(d types.ChainErrorLostData) bool {
	return uriTargetsHandler(d.TargetURI, networkHandlerPath)
}

// uriTargetsHandler reports whether uri addresses handler, in either the
// bare form the kernel emits or the peer-qualified form.
//
// Segment-exact on the tail, so `system/networking` does not match
// `system/network` — the same discriminator every path comparison in this
// tree uses, and the reason a substring test is not good enough here even
// though it would pass every case we have seen.
func uriTargetsHandler(uri, handler string) bool {
	uri = strings.TrimSpace(uri)
	if uri == handler {
		return true
	}
	if strings.HasPrefix(uri, handler+"/") {
		return true
	}
	i := strings.Index(uri, "/"+handler)
	if i < 0 {
		return false
	}
	rest := uri[i+len(handler)+1:]
	return rest == "" || strings.HasPrefix(rest, "/")
}

// Problems renders the report as the sentences a surface shows.
//
// One line, not one per marker. A banner with four thousand identical
// entries is a banner an operator scrolls past, and the whole reason this
// file exists is that they had nothing to read at all.
func (r DeliveryFailureReport) Problems() []string {
	if !r.Observable || r.Total == 0 {
		return nil
	}
	f := r.Recent[0]

	who := f.TargetPeerID
	if who == "" {
		who = "another peer"
	}
	what := f.Code
	if what == "" {
		what = fmt.Sprintf("status %d", f.Status)
	}

	// The CODE is the actionable half and it is named first. An operator
	// can do something different about `capability_denied` (they revoked
	// us, or a grant has not been re-handshaked) than about `not_found`
	// (the file moved), and a message that says only "delivery failed"
	// sends them to us instead of to the answer.
	line := fmt.Sprintf(
		"%d file transfer(s) failed and were recorded — most recent: %s from %s",
		r.Total, what, who)
	if f.TargetURI != "" {
		line += " (" + f.TargetURI + ")"
	}
	switch {
	case strings.Contains(what, "capability"):
		line += ". They have not authorized us for this, or the grant has not " +
			"reached a handshake yet — reconnect, and check the folder is shared to us."
	case strings.Contains(what, "not_found"):
		line += ". The content was not there when we asked; a catch-up pass retries."
	}
	if r.Excluded > 0 {
		// Say what was filtered. A number an operator cannot reconcile
		// with `inspect` is a number they stop believing.
		line += fmt.Sprintf(" (%d unrelated reconnect markers not counted)", r.Excluded)
	}
	return []string{line}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
