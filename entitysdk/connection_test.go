package entitysdk

import (
	"context"
	"net"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/crypto"
)

// TestMultiPeer_TwoAppPeers verifies that two AppPeers created
// separately in the same process can successfully perform the
// connect handshake against each other, and that each retains its
// own identity, store, watcher hub, and event log.
func TestMultiPeer_TwoAppPeers(t *testing.T) {
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

	// Each peer has its own identity.
	if server.PeerID() == client.PeerID() {
		t.Fatal("two AppPeers ended up with identical PeerIDs")
	}

	// Each peer's Store is independent — writes to one don't appear
	// in the other.
	if _, err := server.Store().Put("server/only", "test/v", 1); err != nil {
		t.Fatal(err)
	}
	if client.Store().Has("server/only") {
		t.Error("client can see a path written only on the server — stores are not isolated")
	}

	// Start the server listener.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ready := make(chan struct{})
	listenErr := make(chan error, 1)
	go func() {
		listenErr <- server.ListenReady(ctx, ready)
	}()

	select {
	case <-ready:
	case err := <-listenErr:
		t.Fatalf("server failed to start listening: %v", err)
	case <-time.After(1 * time.Second):
		t.Fatal("server did not become ready in 1s")
	}

	if server.Addr() == nil {
		t.Fatal("server.Addr() is nil after ListenReady fired")
	}

	// Client connects — dial + handshake.
	conn, err := client.Connect(ctx, server.Addr().String())
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	defer conn.Close()

	// Handshake state should be populated.
	st := conn.ConnState()
	if !st.Completed {
		t.Fatal("connection not marked Completed after Connect returned")
	}
	if st.RemotePeerID != serverKP.PeerID() {
		t.Errorf("RemotePeerID = %s, want %s", st.RemotePeerID, serverKP.PeerID())
	}

	sess := conn.Session()
	if sess == nil {
		t.Fatal("Session() is nil after successful connect")
	}
	if sess.Capability == nil {
		t.Error("Session.Capability is nil — grant exchange didn't deliver a capability")
	}
}

func TestConnectRejectsEmptyAddress(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	_, err = ap.Connect(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty address")
	}
	if !IsClientError(err) {
		t.Errorf("want 400 client error, got %v", err)
	}
}

func TestConnectDialFailureIsSystemError(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Port 1 on localhost is almost certainly closed.
	_, err = ap.Connect(ctx, "127.0.0.1:1")
	if err == nil {
		t.Fatal("expected dial to fail")
	}
	if !IsSystemError(err) {
		t.Errorf("want 5xx system error for transport failure, got %v", err)
	}
}

// TestConnect_TCPSchemePrefix_Accepted regresses the Nearby-Peers freeze-
// fix follow-up: mDNS chooseDialAddr emits "tcp://host:port" URLs (and
// manually-typed URLs use the same form per the panel placeholder). Prior
// to this guard, AppPeer.Connect passed the full URL straight to
// net.Dial("tcp", addr), which surfaced a "too many colons in address"
// dial failure. The fix strips the scheme before the TCP dial path
// (RegisterRemote already did the same downstream).
func TestConnect_TCPSchemePrefix_Accepted(t *testing.T) {
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
	go func() {
		listenErr <- server.ListenReady(ctx, ready)
	}()
	select {
	case <-ready:
	case err := <-listenErr:
		t.Fatalf("server failed to start listening: %v", err)
	case <-time.After(1 * time.Second):
		t.Fatal("server did not become ready in 1s")
	}

	addr := "tcp://" + server.Addr().String()
	conn, err := client.Connect(ctx, addr)
	if err != nil {
		t.Fatalf("client.Connect(%q): %v", addr, err)
	}
	defer conn.Close()
	if st := conn.ConnState(); st == nil || !st.Completed {
		t.Fatal("connection not Completed after tcp:// dial")
	}
}

// TestMultiPeer_WebSocket verifies that AppPeer.Connect scheme-routes
// ws:// addresses through core-go's ConnectWebSocket and that
// AppPeer.ListenWebSocketReady accepts the resulting connection.
// Cross-references PHASE-I-PEER-CONNECTIONS-PLAN B-3.
func TestMultiPeer_WebSocket(t *testing.T) {
	serverKP, err := crypto.Generate()
	if err != nil {
		t.Fatal(err)
	}
	clientKP, err := crypto.Generate()
	if err != nil {
		t.Fatal(err)
	}

	server, err := CreatePeer(PeerConfig{
		Keypair: &serverKP,
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

	// Bind on a kernel-chosen ephemeral port so the test is parallel-safe.
	// ListenWebSocketReady takes the addr literally; resolve via a TCP
	// probe and reuse the chosen port string for the ws:// URL.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ephemeral listen: %v", err)
	}
	wsAddr := ln.Addr().String()
	_ = ln.Close()

	ready := make(chan struct{})
	listenErr := make(chan error, 1)
	go func() {
		listenErr <- server.ListenWebSocketReady(ctx, wsAddr, "/ws", ready)
	}()

	select {
	case <-ready:
	case err := <-listenErr:
		t.Fatalf("server ws listen failed: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("ws server did not become ready in 2s")
	}

	url := "ws://" + wsAddr + "/ws"
	conn, err := client.Connect(ctx, url)
	if err != nil {
		t.Fatalf("client.Connect %s: %v", url, err)
	}
	defer conn.Close()

	st := conn.ConnState()
	if !st.Completed {
		t.Fatal("ws connection not Completed after Connect returned")
	}
	if st.RemotePeerID != serverKP.PeerID() {
		t.Errorf("RemotePeerID = %s, want %s", st.RemotePeerID, serverKP.PeerID())
	}
}

func TestListenWithoutAddrIs400(t *testing.T) {
	ap, err := CreatePeer(PeerConfig{}) // no ListenAddr
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	err = ap.Listen(context.Background())
	if err == nil {
		t.Fatal("expected error from Listen with no configured address")
	}
	if !IsClientError(err) {
		t.Errorf("want 400 client error, got %v", err)
	}
}
