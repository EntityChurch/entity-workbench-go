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
// daemon. When core-go expands its decoder to surface IPs, this mirror
// goes away. [unverified: this comment used to cite a packet stem
// `FEEDBACK-CORE-GO-DECODE-ENDPOINT-HINT-IPV4-*` that resolves to no
// file in this repo's outbox or archive, and to nothing in git history
// — checked 2026-09-17. The ask may have been routed under another
// name or never sent; a citation the reader cannot resolve is worse
// than none, so it is removed rather than re-pointed on a guess.]
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

// CandidatePeerID is THE answer to "which peer is this announcement
// from", and every consumer must use it rather than reading a field.
//
// # Why this exists — a whole feature was dead because of one field read
//
// `CandidateData.PeerID` is **empty for every mDNS candidate that exists
// in this product**, and reading it is not a stricter check, it is a
// guaranteed miss. Per EXTENSION-DISCOVERY §2.1 the field is null
// pre-IDENTIFY; core-go's `candidateFromServiceEntry`
// (`ext/discovery/mdns/mdns.go`) constructs the candidate with no PeerID
// at all, and the only writer of the populated form is
// `discovery.Handler.PromoteSuccessor` — which, measured 2026-09-09, has
// **zero callers in either tree**. The claimed peer-id travels in the
// `peer_id_hint` TXT key and nowhere else.
//
// So there were two readers of one announcement, using different keys:
// the Nearby panel read the TXT hint and worked, while the reconciler's
// address refresh and `dialableAddressFor` both read `cd.PeerID`, matched
// nothing, and silently did nothing — for every peer, on every pass,
// since they were written. That is why an operator watched their app dial
// a stale port for 45 minutes while the panel showed the peer sitting on
// the LAN announcing the right one: discovery HAD found it, and the code
// that could have used it was comparing against a field that is never
// filled in. AP58's shape at the level of a field rather than a wrapper —
// the cheap fixture (and the panel) read the populated channel, so
// nothing failed.
//
// # Why trusting the hint is correct here, and where it would not be
//
// The hint is a CLAIM. Anything on the LAN can announce any peer-id. That
// is survivable for exactly one use — deciding **where to dial a peer we
// have already declared** — because the peer-id is not what we are
// learning, it is what we are matching against a device record we already
// hold, and the dial then authenticates against that identity. A false
// hint therefore costs a failed handshake, never a wrong peer: it can
// waste a dial, it cannot redirect a relationship.
//
// It would NOT be sufficient to admit a peer, mint a grant, or bind an
// identity — those need the IDENTIFY ceremony, which is exactly what
// `PromoteSuccessor` is for and what nothing yet calls. Do not widen this
// function's use to that class of decision.
//
// Prefers the substrate's own populated field when it is there, so this
// becomes a no-op the day the ceremony is wired.
func CandidatePeerID(cd types.CandidateData) string {
	if cd.PeerID != "" {
		return cd.PeerID
	}
	return peerIDHintFromCandidate(cd)
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

// DialAddressForCandidate turns a discovery candidate into the URL form
// AppPeer.Connect accepts, or "" when the announcement carries nothing
// dialable.
//
// Extracted from the Avalonia bridge on 2026-09-02, when the shell's
// `peers` verb became the second consumer. AGENTS.md's rule is *DRY the
// integration, not the renderer* — the same closure in two renderers is
// an extraction, and the dial-address choice is a substrate judgement
// about mDNS announcements rather than anything either renderer owns.
//
// Protocol: the hint carries a `proto` TXT key with comma-separated
// names per EXTENSION-DISCOVERY §3.2 — workbench v1 ships profile_refs
// "tcp" and "ws", which are also the protocol names. Prefer ws for
// browser interop; fall back to TCP.
//
// Host preference: a routable IPv4 from the hint beats the announced
// mDNS HostName, because the HostName is the canonical `.local.` form
// (`peer-host.lan.local.`) and resolving it requires nss-mdns /
// avahi-daemon on the DIALING host — a dependency a peer cannot check
// and should not assume. HostName is the fallback when no IPv4 was
// announced, which keeps loopback and IPv6-only LANs working. IPv6 is
// third: many home routers fail IPv6 LAN reachability.
func DialAddressForCandidate(c types.CandidateData) string {
	if len(c.EndpointHint) == 0 {
		return ""
	}
	hint, err := DecodeMDNSEndpointHint(c.EndpointHint)
	if err != nil {
		return ""
	}
	return DialAddressForHint(hint)
}

// DialAddressForHint is DialAddressForCandidate for an already-decoded
// hint, which is what the bridge holds (it renders other fields of the
// hint beside the address).
func DialAddressForHint(hint MDNSEndpointHint) string {
	host := DialHostForHint(hint)
	if host == "" || hint.Port == 0 {
		return ""
	}
	proto := ParseTXTPairs(hint.Text)["proto"]
	if proto == "ws" || proto == "wss" {
		return fmt.Sprintf("ws://%s:%d/ws", host, hint.Port)
	}
	return fmt.Sprintf("tcp://%s:%d", host, hint.Port)
}

// DialHostForHint selects the most dial-friendly host string:
// first non-empty IPv4 → first non-empty IPv6 → HostName.
func DialHostForHint(hint MDNSEndpointHint) string {
	for _, ip := range hint.IPv4 {
		if ip != "" {
			return ip
		}
	}
	for _, ip := range hint.IPv6 {
		if ip != "" {
			// Bracketed per net.Dial's host:port grammar.
			return "[" + ip + "]"
		}
	}
	return hint.HostName
}

// ParseTXTPairs splits "key=value" entries (RFC 6763 §6 / §3.2).
func ParseTXTPairs(txt []string) map[string]string {
	out := make(map[string]string, len(txt))
	for _, t := range txt {
		i := strings.Index(t, "=")
		if i < 0 {
			continue
		}
		out[t[:i]] = t[i+1:]
	}
	return out
}
