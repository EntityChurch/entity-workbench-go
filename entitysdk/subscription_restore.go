package entitysdk

// Subscription restoration after peer restart — workbench-side
// implementation of EXTENSION-SUBSCRIPTION v3.15 §5.7's
// "subscriber-side restoration is application-level" position.
//
// Substrate boundary: the subscription entity on the publisher is the
// durable contract. The receiver's inbox handler is session-bound
// (registered in-memory at SubscribeAt; cleared on Close); the
// substrate explicitly MUST NOT auto-resume it.
//
// Mechanism: each cross-peer SubscribeAt writes a sidecar tracking
// entity to the LOCAL peer's tree at `sdk/subscription-tracking/{id}`,
// capturing the inputs needed to re-issue (remotePeer, pattern,
// events, includePayload). Subscription.Close removes the sidecar.
// On restart, RestorePriorSubscriptions enumerates + re-issues.
//
// Notes:
//   - The publisher's PRIOR subscription entity may still exist on
//     its tree (orphaned). The substrate's delivery-token-missing
//     terminate path (engine.go:357 in core-go) GCs it on the next
//     delivery attempt.
//   - Connections MUST be re-established (Connect) before
//     RestorePriorSubscriptions, otherwise the dispatch will fail.

import (
	"fmt"

	"go.entitychurch.org/entity-core-go/core/ecf"
)

const (
	// subscriptionTrackingPrefix is the local tree prefix where the
	// SDK records per-cross-peer-subscription sidecar entities. NOT a
	// system/* path — workbench-side ergonomic state, not part of the
	// V7 protocol surface.
	subscriptionTrackingPrefix = "sdk/subscription-tracking/"

	// trackingEntityType is the entity Type for the sidecar. Not a
	// wire type; for local categorization only.
	trackingEntityType = "sdk/subscription-tracking"
)

// subscriptionTrackingData captures the inputs to SubscribeAt that
// are needed to re-create the subscription on restart.
type subscriptionTrackingData struct {
	RemotePeer     string   `json:"remote_peer"`
	Pattern        string   `json:"pattern"`
	Events         []string `json:"events,omitempty"`
	IncludePayload bool     `json:"include_payload,omitempty"`
}

// writeSubscriptionTracking persists the sidecar so the subscription
// can be restored after restart. Called from SubscribeAt on the
// success path, only for cross-peer subscriptions (local subs don't
// need restoration — the inbox handler IS the persistence boundary
// for the local case).
func (a *AppPeer) writeSubscriptionTracking(inboxID, remotePeer, pattern string, opts SubscribeOpts) error {
	if remotePeer == "" || remotePeer == a.PeerID() {
		return nil
	}
	payload := map[string]interface{}{
		"remote_peer":     remotePeer,
		"pattern":         pattern,
		"events":          opts.Events,
		"include_payload": opts.IncludePayload,
	}
	path := subscriptionTrackingPrefix + inboxID
	if _, err := a.Put(path, trackingEntityType, payload); err != nil {
		return fmt.Errorf("write subscription tracking %s: %w", path, err)
	}
	return nil
}

// removeSubscriptionTracking deletes the sidecar. Called from
// Subscription.Close so that explicitly-canceled subscriptions don't
// auto-restore on next restart.
func (a *AppPeer) removeSubscriptionTracking(inboxID string) {
	path := subscriptionTrackingPrefix + inboxID
	_, _ = a.RawLocationIndex().Remove(path)
}

// RestoredSubscription pairs a restored Subscription handle with the
// tracking metadata that produced it. Returned by
// RestorePriorSubscriptions so the caller can route events from each
// handle to the appropriate application logic.
type RestoredSubscription struct {
	Sub            *Subscription
	RemotePeer     string
	Pattern        string
	Events         []string
	IncludePayload bool
}

// RestorePriorSubscriptions enumerates sidecar tracking entities and
// re-issues the cross-peer subscriptions they record. Returns the
// restored handles + any errors per-entry (failures don't halt the
// sweep so a single missing publisher doesn't block restoration of
// the rest).
//
// Callers MUST have re-established connections to the relevant
// publishers (via AppPeer.Connect) before calling this. Restoration
// against an unreachable publisher returns an error in the per-entry
// slice; the application can retry later.
//
// Each restoration produces a NEW subscription ID on the publisher
// side; the previous run's subscription entity is orphaned + GC'd by
// the substrate's delivery-token-missing path on next attempt.
func (a *AppPeer) RestorePriorSubscriptions() ([]RestoredSubscription, []error) {
	entries, err := a.List(subscriptionTrackingPrefix)
	if err != nil {
		return nil, []error{fmt.Errorf("list tracking entries: %w", err)}
	}
	out := make([]RestoredSubscription, 0, len(entries))
	errs := make([]error, 0)
	for _, e := range entries {
		if e.HasChildren {
			continue
		}
		ent, ok, err := a.Get(e.Path)
		if err != nil {
			errs = append(errs, fmt.Errorf("get %s: %w", e.Path, err))
			continue
		}
		if !ok || ent.Type != trackingEntityType {
			continue
		}
		var raw map[string]interface{}
		if err := ecf.Decode(ent.Data, &raw); err != nil {
			errs = append(errs, fmt.Errorf("decode %s: %w", e.Path, err))
			continue
		}
		data := parseTrackingMap(raw)
		if data.RemotePeer == "" || data.Pattern == "" {
			errs = append(errs, fmt.Errorf("invalid tracking %s: missing fields", e.Path))
			continue
		}
		opts := SubscribeOpts{
			Events:         data.Events,
			IncludePayload: data.IncludePayload,
		}
		sub, err := a.SubscribeAt(data.RemotePeer, data.Pattern, opts)
		if err != nil {
			errs = append(errs, fmt.Errorf("restore %s→%s: %w",
				data.RemotePeer, data.Pattern, err))
			continue
		}
		// The new SubscribeAt wrote a NEW tracking entry under a new
		// inboxID. Remove the OLD entry so we don't double-restore on
		// the next restart.
		_, _ = a.RawLocationIndex().Remove(e.Path)

		out = append(out, RestoredSubscription{
			Sub:            sub,
			RemotePeer:     data.RemotePeer,
			Pattern:        data.Pattern,
			Events:         data.Events,
			IncludePayload: data.IncludePayload,
		})
	}
	return out, errs
}

// parseTrackingMap pulls the tracking fields out of a decoded
// map[string]interface{} payload.
func parseTrackingMap(raw map[string]interface{}) subscriptionTrackingData {
	data := subscriptionTrackingData{}
	if s, ok := raw["remote_peer"].(string); ok {
		data.RemotePeer = s
	}
	if s, ok := raw["pattern"].(string); ok {
		data.Pattern = s
	}
	if v, ok := raw["events"].([]interface{}); ok {
		for _, e := range v {
			if s, ok := e.(string); ok {
				data.Events = append(data.Events, s)
			}
		}
	}
	if b, ok := raw["include_payload"].(bool); ok {
		data.IncludePayload = b
	}
	return data
}
