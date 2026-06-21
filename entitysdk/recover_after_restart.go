package entitysdk

// One-call post-restart recovery sugar. Composes the documented
// three-step sequence (Connect + RestorePriorSubscriptions +
// ReconcileSinceLastSeen) into a single API call so applications
// don't need to reimplement the order + error-handling boilerplate
// each time.
//
// See HANDOFF-WORKBENCH-STAGE-5-FOLLOWUPS Lane 5 and
// GUIDE-CONTINUATIONS-WORKBENCH §6.5 for the full contract.

import (
	"context"
	"fmt"
	"strings"

	"go.entitychurch.org/entity-core-go/core/hash"
)

// RecoveryStepError captures a single failure during recovery without
// halting the rest of the sequence. Best-effort recovery means the
// helper keeps going past per-step errors and reports everything.
type RecoveryStepError struct {
	// Step identifies where the failure happened: one of
	// "connect", "restore", or "reconcile".
	Step string

	// Detail describes the specific item that failed (e.g. publisher
	// peer-id, restored-subscription pattern, reconcile prefix). Empty
	// when the step failure isn't item-specific (e.g. the initial
	// Connect call).
	Detail string

	// Err is the underlying error.
	Err error
}

func (e RecoveryStepError) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("%s: %v", e.Step, e.Err)
	}
	return fmt.Sprintf("%s [%s]: %v", e.Step, e.Detail, e.Err)
}

func (e RecoveryStepError) Unwrap() error { return e.Err }

// RecoveryResult bundles the outcomes of all three recovery steps.
type RecoveryResult struct {
	// Connected is true iff Connect to the publisher succeeded.
	// When false, Restored + Reconciled are empty (no subsequent
	// steps run).
	Connected bool

	// Restored is the list of subscriptions re-registered during
	// RestorePriorSubscriptions. Each entry pairs a live Subscription
	// handle with the metadata used to create it. Callers MUST drain
	// each handle's Events() channel (typically in a goroutine) the
	// same way they would for a fresh SubscribeAt.
	Restored []RestoredSubscription

	// Reconciled is the list of per-subscription reconcile results.
	// Index-aligned with Restored when a reconcile attempt was made;
	// length may be shorter than Restored if some entries weren't
	// against the supplied publisher (those are skipped — the helper
	// only reconciles against the explicitly-connected publisher).
	Reconciled []ReconcileResult

	// Errors collects per-step failures. Non-empty Errors does NOT
	// imply RecoveryResult is unusable: the helper proceeds past
	// per-item failures and reports each in this slice.
	Errors []RecoveryStepError
}

// RecoverAfterRestart re-establishes consumer state against publisher
// after a process restart, in three steps:
//
//  1. Connect(ctx, publisherAddr) — re-establish transport.
//  2. RestorePriorSubscriptions() — re-register inbox handlers for
//     every tracked subscription (including those against publishers
//     other than `publisherPeerID`; those get restored but skip the
//     reconcile step).
//  3. ReconcileSinceLastSeen(ctx, publisherPeerID, prefix, zero-hash)
//     for each restored subscription whose RemotePeer equals
//     publisherPeerID — pulls the full current closure (bootstrap
//     pull) since this version of the helper doesn't yet track
//     per-subscription last-seen hashes.
//
// Best-effort: a failure in step 1 short-circuits (transport is the
// hard prerequisite); failures in steps 2 or 3 don't halt the rest
// of the sweep — each is collected into RecoveryResult.Errors.
//
// `publisherPeerID` and `publisherAddr` identify the single
// publisher to reconnect to. Multi-publisher recovery is a future
// extension; for now, callers with subscriptions against multiple
// publishers should call RecoverAfterRestart once per publisher
// (each call's RestorePriorSubscriptions sweep is idempotent —
// re-registering an already-restored subscription is a no-op modulo
// a transient new-subscription-ID).
func (a *AppPeer) RecoverAfterRestart(ctx context.Context, publisherPeerID, publisherAddr string) (RecoveryResult, error) {
	if publisherPeerID == "" {
		return RecoveryResult{}, NewError(400, "invalid_peer", "publisherPeerID is empty")
	}
	if publisherAddr == "" {
		return RecoveryResult{}, NewError(400, "invalid_addr", "publisherAddr is empty")
	}

	res := RecoveryResult{}

	// Step 1: Connect. A connect failure short-circuits — there's no
	// way to restore + reconcile without transport.
	if _, err := a.Connect(ctx, publisherAddr); err != nil {
		res.Errors = append(res.Errors, RecoveryStepError{
			Step: "connect",
			Err:  err,
		})
		return res, NewError(503, "connect_failed",
			fmt.Sprintf("connect to %s at %s: %v", publisherPeerID, publisherAddr, err))
	}
	res.Connected = true

	// Step 2: Restore. Per-item errors collected; sweep continues.
	restored, restoreErrs := a.RestorePriorSubscriptions()
	res.Restored = restored
	for _, err := range restoreErrs {
		res.Errors = append(res.Errors, RecoveryStepError{
			Step: "restore",
			Err:  err,
		})
	}

	// Step 3: Reconcile. Only for subscriptions against the connected
	// publisher — other publishers' subscriptions are restored above
	// but skip the reconcile step (no live connection to their peer).
	for _, r := range restored {
		if r.RemotePeer != publisherPeerID {
			continue
		}
		prefix := prefixFromPattern(r.Pattern)
		if prefix == "" {
			// Pattern doesn't map to a reconcile prefix (e.g. "*"
			// alone). Skip with a soft error so callers know.
			res.Errors = append(res.Errors, RecoveryStepError{
				Step:   "reconcile",
				Detail: fmt.Sprintf("%s pattern=%s", r.RemotePeer, r.Pattern),
				Err:    fmt.Errorf("cannot derive reconcile prefix from pattern %q", r.Pattern),
			})
			continue
		}

		reconcileRes, err := a.ReconcileSinceLastSeen(ctx, r.RemotePeer, prefix, hash.Hash{})
		if err != nil {
			// "no revision head bound" means the publisher doesn't have
			// auto-version configured on this prefix — there's nothing
			// to reconcile, which is a NORMAL state for non-versioned
			// subscriptions. Treat as soft success with a zero-entity
			// result. Distinguish from real failures (network, decode,
			// etc.) which should propagate.
			if isNoLocalStateError(err) {
				res.Reconciled = append(res.Reconciled, ReconcileResult{
					Prefix:           prefix,
					RemotePeerID:     r.RemotePeer,
					BaseHash:         hash.Hash{},
					EntitiesIngested: 0,
				})
				continue
			}
			res.Errors = append(res.Errors, RecoveryStepError{
				Step:   "reconcile",
				Detail: fmt.Sprintf("%s prefix=%s", r.RemotePeer, prefix),
				Err:    err,
			})
			continue
		}
		res.Reconciled = append(res.Reconciled, reconcileRes)
	}

	return res, nil
}

// isNoLocalStateError returns true if err signals the publisher has
// no revision head bound at the requested prefix — i.e. the prefix
// isn't auto-versioned. This is distinct from real recovery failures
// (transport, decode, etc.) and should be silenced into "nothing to
// reconcile" rather than reported as an error.
func isNoLocalStateError(err error) bool {
	if err == nil {
		return false
	}
	// The error chain wraps revision:fetch-diff's 404 no_local_state
	// inside a higher-level fetch_diff_failed (also 500-coded). Probe
	// the message; SDK errors are stable text per WrapError shape.
	msg := err.Error()
	return strings.Contains(msg, "no_local_state") ||
		strings.Contains(msg, "no revision head bound")
}

// prefixFromPattern converts a subscription pattern to a reconcile
// prefix. Subscription patterns use trailing `*` as wildcard
// ("watched/*"); reconcile prefixes are directory-style ("watched/").
// Returns empty string when no clean conversion is possible (e.g.
// "*" alone), letting the caller raise a soft error.
func prefixFromPattern(pattern string) string {
	if pattern == "" || pattern == "*" {
		return ""
	}
	p := strings.TrimSuffix(pattern, "*")
	if p == "" {
		return ""
	}
	if !strings.HasSuffix(p, "/") {
		p += "/"
	}
	return p
}
