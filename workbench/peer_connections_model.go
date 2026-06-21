package workbench

// ConnectionEntry is the renderer-friendly snapshot of one alias →
// peer-id binding in a shell workspace. Built by the bridge / shellboot
// adapter layer from shellcmd.Shell.Conns; workbench/ holds only the
// shape so panels can deserialize without depending on shellcmd.
//
// IsLocal marks the in-process peer entry — panels typically hide or
// visually distinguish it from remote connections.
type ConnectionEntry struct {
	Alias   string `json:"alias"`
	PeerID  string `json:"peer_id"`
	Address string `json:"address"`
	IsLocal bool   `json:"is_local"`
}

// PeerConnectionsRender is the JSON payload returned by the bridge's
// ConnectionsRender to the renderer. Entries are sorted with local
// first, then remote alphabetically by alias.
type PeerConnectionsRender struct {
	Entries []ConnectionEntry `json:"entries"`
}

// NearbyPeerEntry is the renderer-friendly snapshot of one mDNS-discovered
// candidate. PeerID carries the candidate's peer_id_hint TXT key (pre-
// IDENTIFY hint; CandidateData.PeerID is empty until the trust ceremony
// completes — see EXTENSION-DISCOVERY §2.1). DialURL is the suggested
// address for AppPeer.Connect (ws:// preferred when advertised).
type NearbyPeerEntry struct {
	PeerID    string `json:"peer_id"`
	DialURL   string `json:"dial_url"`
	Backend   string `json:"backend"`
	IsLocal   bool   `json:"is_local"`
	Connected bool   `json:"connected"`
}

// NearbyPeersRender is the JSON payload returned by the bridge's
// DiscoveryRender. Entries exclude the local peer and entries that are
// already in the workspace's Conns map (connected via this or another
// surface) are marked Connected=true so the panel can label them
// distinctly from connectable rows.
type NearbyPeersRender struct {
	Entries []NearbyPeerEntry `json:"entries"`
}
