package entitysdk

import (
	"context"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/crypto"
	discoverymdns "go.entitychurch.org/entity-core-go/ext/discovery/mdns"
)

// TestDiscovery_AnnounceThenScan_FindsLocalPeer verifies the workbench
// mDNS substrate is wired end-to-end: a peer with ListenAddr set can
// Announce("tcp"), and a sibling peer's DiscoverPeers() returns the
// announced candidate. Both peers run in the same process so the test
// is hermetic — `lo` is included in announceInterfaces() to support
// same-host cross-impl convergence per ext/discovery/mdns/mdns.go:31-43.
func TestDiscovery_AnnounceThenScan_FindsLocalPeer(t *testing.T) {
	if testing.Short() {
		t.Skip("mDNS test uses real multicast; skip in -short mode")
	}

	serverKP, err := crypto.Generate()
	if err != nil {
		t.Fatal(err)
	}
	clientKP, err := crypto.Generate()
	if err != nil {
		t.Fatal(err)
	}

	server, err := CreatePeer(PeerConfig{
		Keypair:    &serverKP,
		ListenAddr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("server CreatePeer: %v", err)
	}
	defer server.Close()

	client, err := CreatePeer(PeerConfig{
		Keypair:    &clientKP,
		ListenAddr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("client CreatePeer: %v", err)
	}
	defer client.Close()

	if !server.DiscoveryEnabled() {
		t.Fatal("server.DiscoveryEnabled() = false, expected true (ListenAddr set)")
	}
	if !client.DiscoveryEnabled() {
		t.Fatal("client.DiscoveryEnabled() = false")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	// Start both peers listening so the resolver's port lookup succeeds.
	serverReady := make(chan struct{})
	go func() { _ = server.ListenReady(ctx, serverReady) }()
	<-serverReady

	clientReady := make(chan struct{})
	go func() { _ = client.ListenReady(ctx, clientReady) }()
	<-clientReady

	if err := server.Announce(ctx, "tcp"); err != nil {
		t.Fatalf("server.Announce: %v", err)
	}

	// Give the mDNS announcement a moment to propagate before scanning.
	// grandcat/zeroconf's announce path is asynchronous; the spec gives
	// no immediate-visibility guarantee. ~500ms is enough on `lo`.
	time.Sleep(500 * time.Millisecond)

	cands, err := client.DiscoverPeers(ctx)
	if err != nil {
		t.Fatalf("client.DiscoverPeers: %v", err)
	}

	// CandidateData.PeerID is post-IDENTIFY (§2.1) — pre-IDENTIFY scan
	// results carry the peer-id in the TXT-key `peer_id_hint` channel,
	// which the mDNS backend preserves inside endpoint_hint.
	serverPID := string(serverKP.PeerID())
	found := false
	for _, c := range cands {
		if c.Backend != "mdns" {
			continue
		}
		_, _, txt, decErr := discoverymdns.DecodeEndpointHint(c.EndpointHint)
		if decErr != nil {
			t.Logf("decode endpoint_hint: %v", decErr)
			continue
		}
		if txt["peer_id_hint"] == serverPID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("server peer-id %s not in client's scan (%d candidates returned)",
			serverPID, len(cands))
		for i, c := range cands {
			_, _, txt, _ := discoverymdns.DecodeEndpointHint(c.EndpointHint)
			t.Logf("  candidate[%d] peer_id_hint=%s backend=%s txt=%v",
				i, txt["peer_id_hint"], c.Backend, txt)
		}
	}

	if err := server.AnnounceStop(ctx, "tcp"); err != nil {
		t.Errorf("server.AnnounceStop: %v", err)
	}
}

// TestDiscovery_DisabledWhenNoListenAddr verifies the substrate is
// off-by-default for outbound-only peers.
func TestDiscovery_DisabledWhenNoListenAddr(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	if ap.DiscoveryEnabled() {
		t.Error("DiscoveryEnabled() = true on outbound-only peer; want false")
	}
	if err := ap.Announce(context.Background(), "tcp"); err == nil {
		t.Error("Announce on outbound-only peer returned nil error; want 400")
	} else if !IsClientError(err) {
		t.Errorf("want 400 client error, got %v", err)
	}
}
