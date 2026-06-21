package entitysdk

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"go.entitychurch.org/entity-core-go/core/types"

	"github.com/fxamacker/cbor/v2"
)

// DiscoveryCandidateMaxAge is the freshness window for candidate
// entities returned by ReadDiscoveredCandidates. Anything observed
// longer ago is treated as stale (peer departed / went offline) and
// is both omitted from the snapshot and removed from the store on
// the next ReapStaleDiscoveredCandidates pass.
//
// Set to 3× the bridge's scanLoop cadence (5s) so a single dropped
// scan doesn't false-positive a still-live peer. Core-go's mDNS
// backend doesn't fire its reapCb today, so the workbench side owns
// staleness (routed to upstream).
//
// Declared as a var so tests can override; not safe to mutate at
// runtime in production code.
var DiscoveryCandidateMaxAge = 15 * time.Second

// MDNSEndpointHint is the workbench-side view of the opaque
// `endpoint_hint` blob the mDNS backend writes into a CandidateData
// (per EXTENSION-DISCOVERY §2.1). Mirrors the private struct in
// `entity-core-go/ext/discovery/mdns/mdns.go` (`mdnsEndpointHint`).
//
// We mirror it because core-go's exported `DecodeEndpointHint` drops
// the IPv4/IPv6 lists from its return signature, and we need them to
// dial cross-LAN peers whose announced HostName (e.g.
// `peer-host.lan.local.`) only resolves under nss-mdns / avahi-
// daemon. When core-go expands its decoder to surface IPs (routed to
// upstream in reviews/FEEDBACK-CORE-GO-DECODE-ENDPOINT-HINT-IPV4-*),
// this mirror goes away.
//
// CBOR field tags MUST match the upstream struct verbatim or decoding
// silently drops fields.
type MDNSEndpointHint struct {
	HostName string   `cbor:"host_name"`
	Port     int      `cbor:"port"`
	IPv4     []string `cbor:"ipv4,omitempty"`
	IPv6     []string `cbor:"ipv6,omitempty"`
	Text     []string `cbor:"text,omitempty"`
}

// DecodeMDNSEndpointHint decodes the opaque `endpoint_hint` blob into
// the full MDNSEndpointHint shape — including IPv4/IPv6 lists that
// core-go's `mdns.DecodeEndpointHint` drops. Used by the bridge's
// `chooseDialAddr` so cross-LAN dials can target an IP instead of a
// `.local.` hostname that requires avahi-side resolution.
func DecodeMDNSEndpointHint(raw []byte) (MDNSEndpointHint, error) {
	var h MDNSEndpointHint
	if err := cbor.Unmarshal(raw, &h); err != nil {
		return MDNSEndpointHint{}, fmt.Errorf("decode mdns endpoint_hint: %w", err)
	}
	return h, nil
}

// DiscoveryEnabled reports whether the discovery substrate is wired on
// this peer. False when no ListenAddr was configured at construction.
func (a *AppPeer) DiscoveryEnabled() bool {
	return a.discoveryHandler != nil
}

// Announce advertises this peer on the given mDNS profile so other
// peers on the LAN can discover it. profileRef is the §3.2 "service
// kind" hint — workbench v1 supports "tcp" and "ws" (the local
// peer's listener scheme). Idempotent on repeat calls for the same
// profile.
//
// Returns 400 when discovery is disabled or the listener has not yet
// bound; 5xx on backend errors. Safe to call after auto-Listen has
// completed (shellboot.PeerManager.Create waits for ready before
// returning).
func (a *AppPeer) Announce(ctx context.Context, profileRef string) error {
	if a.discoveryHandler == nil {
		return NewError(400, "discovery_disabled",
			"discovery not configured (peer constructed without ListenAddr)")
	}
	b, ok := a.discoveryHandler.Backend("mdns")
	if !ok {
		return NewError(400, "no_backend", "mdns backend not registered")
	}
	if err := b.Announce(ctx, profileRef); err != nil {
		return WrapError(500, "announce_failed",
			"mdns announce profile="+profileRef, err)
	}
	return nil
}

// AnnounceStop ends an active announce session for profileRef.
// Idempotent on already-stopped sessions.
func (a *AppPeer) AnnounceStop(ctx context.Context, profileRef string) error {
	if a.discoveryHandler == nil {
		return NewError(400, "discovery_disabled",
			"discovery not configured")
	}
	b, ok := a.discoveryHandler.Backend("mdns")
	if !ok {
		return NewError(400, "no_backend", "mdns backend not registered")
	}
	if err := b.AnnounceStop(ctx, profileRef); err != nil {
		return WrapError(500, "announce_stop_failed",
			"mdns announce-stop profile="+profileRef, err)
	}
	return nil
}

// DiscoverPeers returns the current mDNS scan snapshot. Equivalent to
// dispatching `system/discovery:scan(mdns)` but bypasses the protocol
// layer — workbench scopes are always local so the grant-check
// ceremony is unnecessary noise here. The returned slice excludes the
// local peer (filtered by peer-id-hint match against the live peer).
//
// Blocks until the backend completes its mDNS browse window (default
// 1s); ctx cancellation aborts the scan early with whatever the
// backend has observed by that point.
func (a *AppPeer) DiscoverPeers(ctx context.Context) ([]types.CandidateData, error) {
	if a.discoveryHandler == nil {
		return nil, NewError(400, "discovery_disabled",
			"discovery not configured (peer constructed without ListenAddr)")
	}
	b, ok := a.discoveryHandler.Backend("mdns")
	if !ok {
		return nil, NewError(400, "no_backend", "mdns backend not registered")
	}
	cands, err := b.Scan(ctx, nil)
	if err != nil {
		return nil, WrapError(500, "scan_failed", "mdns scan", err)
	}
	localPID := string(a.peer.PeerID())
	out := cands[:0]
	for _, c := range cands {
		if c.PeerID == localPID {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

// ReadDiscoveredCandidates returns the current set of mDNS candidates
// materialized into the store, WITHOUT triggering a fresh scan. Pairs
// with OnDiscoveredPeerChange: the subscription notifies on add/remove,
// this method reads the resulting state.
//
// Two filters apply that DiscoverPeers does not:
//
//   - **Freshness.** Each scan writes a new candidate entity (the hash
//     embeds ObservedAt, so the path differs every scan). Entries
//     older than DiscoveryCandidateMaxAge are dropped — that's how a
//     peer that goes offline disappears from the Nearby list.
//   - **Dedup.** Multiple snapshot entries per peer collapse to one
//     (the freshest), keyed on the `peer_id_hint` TXT key. Without
//     this the panel would show the same peer N times after N scans.
//
// The local peer is filtered out by PeerID match (matches DiscoverPeers'
// own self-filter); pre-IDENTIFY candidates carry an empty PeerID, so
// the TXT-hint check below catches that path too.
//
// Returns nil when discovery is disabled. Safe to call concurrently.
func (a *AppPeer) ReadDiscoveredCandidates() []types.CandidateData {
	if a.discoveryHandler == nil || a.store == nil {
		return nil
	}
	entries := a.store.List(types.CandidatePrefix(types.DiscoveryBackendMDNS))
	if len(entries) == 0 {
		return nil
	}
	cutoff := time.Now().Add(-DiscoveryCandidateMaxAge).UnixMilli()
	localPID := string(a.peer.PeerID())
	freshest := make(map[string]types.CandidateData, len(entries))
	for _, e := range entries {
		ent, ok := a.store.Get(e.Path)
		if !ok {
			continue
		}
		cd, err := types.CandidateDataFromEntity(ent)
		if err != nil {
			continue
		}
		if cd.PeerID == localPID {
			continue
		}
		if int64(cd.ObservedAt) < cutoff {
			continue
		}
		pidHint := peerIDHintFromCandidate(cd)
		if pidHint == "" || pidHint == localPID {
			continue
		}
		if prev, seen := freshest[pidHint]; !seen || cd.ObservedAt > prev.ObservedAt {
			freshest[pidHint] = cd
		}
	}
	out := make([]types.CandidateData, 0, len(freshest))
	for _, cd := range freshest {
		out = append(out, cd)
	}
	return out
}

// ReapStaleDiscoveredCandidates removes candidate entities older than
// DiscoveryCandidateMaxAge from the store. Returns the number removed.
// Called opportunistically by the bridge's scanLoop after each Scan so
// the candidate prefix doesn't grow unboundedly — every Scan creates a
// fresh entity (hash includes ObservedAt), and without reaping the
// store accumulates one entry per peer per scan interval indefinitely.
//
// No-op if discovery is disabled. Safe to call concurrently.
func (a *AppPeer) ReapStaleDiscoveredCandidates() int {
	if a.discoveryHandler == nil || a.store == nil {
		return 0
	}
	entries := a.store.List(types.CandidatePrefix(types.DiscoveryBackendMDNS))
	if len(entries) == 0 {
		return 0
	}
	cutoff := time.Now().Add(-DiscoveryCandidateMaxAge).UnixMilli()
	removed := 0
	for _, e := range entries {
		ent, ok := a.store.Get(e.Path)
		if !ok {
			continue
		}
		cd, err := types.CandidateDataFromEntity(ent)
		if err != nil {
			continue
		}
		if int64(cd.ObservedAt) >= cutoff {
			continue
		}
		if a.store.Remove(e.Path) {
			removed++
		}
	}
	return removed
}

// peerIDHintFromCandidate extracts the `peer_id_hint` TXT key from a
// candidate's endpoint_hint, returning empty string when absent or on
// decode failure.
func peerIDHintFromCandidate(cd types.CandidateData) string {
	hint, err := DecodeMDNSEndpointHint(cd.EndpointHint)
	if err != nil {
		return ""
	}
	for _, t := range hint.Text {
		if rest, ok := strings.CutPrefix(t, "peer_id_hint="); ok {
			return rest
		}
	}
	return ""
}

// OnDiscoveredPeerChange subscribes to mutations under
// `system/discovery/candidate/mdns/`. Returns a cancel func. The
// callback fires when new candidates land via the mDNS observe-callback
// path (the substrate writes candidate entities into the store under
// this prefix; the watcher fans out per the standard Store.OnPrefixChange
// contract).
//
// Returns a nil cancel func when discovery is disabled — callers can
// safely store and invoke the return value either way.
func (a *AppPeer) OnDiscoveredPeerChange(cb func(ChangeEvent)) func() {
	if a.discoveryHandler == nil || a.store == nil {
		return func() {}
	}
	return a.store.OnPrefixChange(
		types.CandidatePrefix("mdns"),
		cb,
	)
}

// --- helpers (referenced from app.go's discovery resolver) ----------

// splitHostPort wraps net.SplitHostPort with a clearer error wrap.
func splitHostPort(addr string) (host, port string, err error) {
	host, port, err = net.SplitHostPort(addr)
	if err != nil {
		return "", "", fmt.Errorf("split %q: %w", addr, err)
	}
	return host, port, nil
}

// atoiPort parses a port string and validates the range.
func atoiPort(s string) (int, error) {
	p, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	if p <= 0 || p > 65535 {
		return 0, fmt.Errorf("port %d out of range", p)
	}
	return p, nil
}
