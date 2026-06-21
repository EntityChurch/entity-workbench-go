package entitysdk

import (
	"go.entitychurch.org/entity-core-go/core/peer"
)

// PeerInfo describes a peer currently connected to this AppPeer.
// Matches SDK-OPERATIONS §7.3 PeerInfo.
type PeerInfo struct {
	PeerID    string
	Address   string
	Direction string // "inbound" | "outbound" — empty when core-go cannot disambiguate (see note).
}

// ConnectedPeers returns a snapshot of peers currently connected to
// this AppPeer — inbound and outbound. Matches SDK-OPERATIONS §7.3
// (SHOULD).
//
// Direction: core-go's peer.Peer.Connections() returns a flat slice
// in which inbound and outbound connections are concatenated but not
// individually tagged. Until core-go exposes a direction accessor,
// Direction is left empty. A
// non-empty string is introduced without changing PeerID / Address
// semantics, so callers that switch on Direction remain correct once
// core surfaces the information.
func (a *AppPeer) ConnectedPeers() []PeerInfo {
	conns := a.peer.Connections()
	out := make([]PeerInfo, 0, len(conns))
	for _, c := range conns {
		out = append(out, peerInfoFromConnection(c))
	}
	return out
}

func peerInfoFromConnection(c *peer.Connection) PeerInfo {
	info := PeerInfo{}
	if cs := c.ConnState(); cs != nil {
		info.PeerID = string(cs.RemotePeerID)
	}
	if addr := c.RemoteAddr(); addr != nil {
		info.Address = addr.String()
	}
	return info
}
