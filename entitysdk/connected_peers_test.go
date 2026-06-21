package entitysdk

import (
	"context"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/crypto"
)

func TestConnectedPeersEmptyByDefault(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	if got := ap.ConnectedPeers(); len(got) != 0 {
		t.Errorf("ConnectedPeers on fresh peer: len=%d, want 0; got %+v", len(got), got)
	}
}

func TestConnectedPeersReportsHandshakedConnection(t *testing.T) {
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

	client, err := CreatePeer(PeerConfig{Keypair: &clientKP})
	if err != nil {
		t.Fatalf("client CreatePeer: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ready := make(chan struct{})
	listenErr := make(chan error, 1)
	go func() { listenErr <- server.ListenReady(ctx, ready) }()
	select {
	case <-ready:
	case err := <-listenErr:
		t.Fatalf("server listen: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("server did not become ready")
	}

	conn, err := client.Connect(ctx, server.Addr().String())
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	defer conn.Close()

	// Client sees one connected peer — the server.
	clientPeers := client.ConnectedPeers()
	if len(clientPeers) != 1 {
		t.Fatalf("client ConnectedPeers: len=%d, want 1; got %+v", len(clientPeers), clientPeers)
	}
	if clientPeers[0].PeerID != string(serverKP.PeerID()) {
		t.Errorf("client side PeerID = %q, want %q", clientPeers[0].PeerID, serverKP.PeerID())
	}
	if clientPeers[0].Address == "" {
		t.Error("client side Address is empty")
	}

	// Server may take a brief moment to register the accepted connection
	// on its side; poll until it appears or the deadline hits.
	deadline := time.Now().Add(2 * time.Second)
	var serverPeers []PeerInfo
	for time.Now().Before(deadline) {
		serverPeers = server.ConnectedPeers()
		if len(serverPeers) >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(serverPeers) != 1 {
		t.Fatalf("server ConnectedPeers: len=%d, want 1; got %+v", len(serverPeers), serverPeers)
	}
	if serverPeers[0].PeerID != string(clientKP.PeerID()) {
		t.Errorf("server side PeerID = %q, want %q", serverPeers[0].PeerID, clientKP.PeerID())
	}
}
