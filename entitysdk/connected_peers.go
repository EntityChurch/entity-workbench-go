package entitysdk

import (
	"go.entitychurch.org/entity-core-go/core/peer"
)

// PeerInfo describes a peer currently connected to this AppPeer.
// Matches SDK-OPERATIONS §7.3 PeerInfo.
type PeerInfo struct {
	PeerID  string
	Address string
	// Direction is "inbound" | "outbound", core-go's own value.
	//
	// It is TOTAL from that source — `Connection.Direction()` branches on a
	// bool and always returns one of the two — so a caller does not have to
	// handle a third case. The empty string therefore means *this PeerInfo
	// did not come from a connection*, which is a bug in whoever built it
	// rather than a state of the world, and `Outbound()` reports false for it
	// because the safe reading of "we do not know" is "we cannot dispatch".
	Direction string
}

// Outbound reports whether WE opened this connection.
//
// A predicate rather than a string comparison at each call site: comparing by
// hand fails silently on a typo, and the whole point of the field is that a
// caller deciding whether it can dispatch to a peer gets the right answer.
func (p PeerInfo) Outbound() bool { return p.Direction == DirectionOutbound }

// Direction values, spelled once. These are core-go's strings
// (`Connection.Direction()`), not ours to choose.
const (
	DirectionInbound  = "inbound"
	DirectionOutbound = "outbound"
)

// ConnectedPeers returns a snapshot of peers currently connected to
// this AppPeer — inbound and outbound. Matches SDK-OPERATIONS §7.3
// (SHOULD).
//
// # Direction
//
// `Direction` was declared and left **empty** from the day this file was
// written, with a note saying core-go's `Connections()` concatenates both
// directions without tagging them. That was true, it was routed
// (`docs/outbox/CONNECTION-DIRECTION-AND-BILATERAL-REACH-2026-09-09.md`), and
// core-go answered it with `Connection.IsOutbound()` / `Direction()`,
// recorded once in `PerformConnect` and therefore dialer-only (their tracker
// row 13). It is populated now.
//
// ⚠ **Why the distinction is load-bearing and not descriptive.** A
// dial-by-address authorizes the DIALER only (AP63), so an inbound-only
// session is a peer that can reach us while we cannot dispatch to them — the
// state in which sharing is half-broken, and the most likely one. Reported as
// a flat "connected" it is indistinguishable from a working relationship,
// which is exactly what let an operator's app spend 45 minutes looking at
// three surfaces that said `Connected` while the run log correctly said the
// outbound connection had never come up.
func (a *AppPeer) ConnectedPeers() []PeerInfo {
	conns := a.peer.Connections()
	out := make([]PeerInfo, 0, len(conns))
	for _, c := range conns {
		out = append(out, peerInfoFromConnection(c))
	}
	return out
}

// HasOutboundConnection reports whether the pool holds a connection WE
// opened to peerID.
//
// The question every caller of `ConnectedPeers` was actually asking when it
// checked for a peer's presence, and the one a flat pool snapshot could not
// answer. Kept on the SDK rather than in one frontend so a second frontend
// cannot reach a different conclusion about the same pool.
func (a *AppPeer) HasOutboundConnection(peerID string) bool {
	if peerID == "" {
		return false
	}
	for _, info := range a.ConnectedPeers() {
		if info.PeerID == peerID && info.Outbound() {
			return true
		}
	}
	return false
}

func peerInfoFromConnection(c *peer.Connection) PeerInfo {
	info := PeerInfo{}
	if cs := c.ConnState(); cs != nil {
		info.PeerID = string(cs.RemotePeerID)
	}
	if addr := c.RemoteAddr(); addr != nil {
		info.Address = addr.String()
	}
	info.Direction = c.Direction()
	return info
}
