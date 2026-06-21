package entitysdk

import (
	"context"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/crypto"
	"go.entitychurch.org/entity-core-go/core/peer"
	"go.entitychurch.org/entity-core-go/core/tree"
)

// TestExecutor_RemoteDispatch verifies the Phase-2 wiring (§4.4 of
// SHELL-DIRECTION.md): when an Executor is asked to dispatch to a
// handler URI whose peer-id is non-local, the call routes through the
// dispatcher's remote-execute path — using the local peer's pooled
// connection set up by AppPeer.Connect (which now seeds RegisterRemote).
func TestExecutor_RemoteDispatch(t *testing.T) {
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
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
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

	// Seed a known entity in the server's tree.
	const remotePath = "demo/value"
	if _, err := server.Store().Put(remotePath, "test/scalar", 42); err != nil {
		t.Fatalf("server seed Put: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ready := make(chan struct{})
	listenErr := make(chan error, 1)
	go func() { listenErr <- server.ListenReady(ctx, ready) }()
	select {
	case <-ready:
	case err := <-listenErr:
		t.Fatalf("server listen: %v", err)
	case <-time.After(1 * time.Second):
		t.Fatal("server not ready in 1s")
	}

	conn, err := client.Connect(ctx, server.Addr().String())
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	defer conn.Close()

	// Dispatch GET via the client's Executor against a peer-qualified
	// handler URI. The dispatcher's RemoteExecute callback should kick
	// in (URI peer-id ≠ local) and route over the pooled connection.
	serverPeerID := string(serverKP.PeerID())
	handlerURI := "entity://" + serverPeerID + "/system/tree"
	resourceURI := "entity://" + serverPeerID + "/" + remotePath

	getReq, resource, err := tree.CreateGetRequest(resourceURI, "entity")
	if err != nil {
		t.Fatalf("CreateGetRequest: %v", err)
	}

	resp, err := client.Executor().ExecuteOnResource(handlerURI, "get", getReq, resource)
	if err != nil {
		t.Fatalf("remote ExecuteOnResource: %v", err)
	}
	if resp == nil {
		t.Fatal("nil response from remote ExecuteOnResource")
	}
	if resp.Status != 200 {
		t.Fatalf("status = %d, want 200", resp.Status)
	}

	got := resp.Entity()
	if got.Type != "test/scalar" {
		t.Errorf("entity Type = %q, want %q", got.Type, "test/scalar")
	}

	// A subsequent local dispatch on the same Executor must still work
	// — the URI-aware branch should not have leaked into local paths.
	if _, err := client.Store().Put("local/v", "test/scalar", 99); err != nil {
		t.Fatalf("local Put: %v", err)
	}
	if _, ok := client.Store().Get("local/v"); !ok {
		t.Error("local store round-trip broken after remote dispatch")
	}
}

// TestAppPeer_RemoteGetPutList verifies the high-level surface
// (AppPeer.Get/Put/List) routes remote when the path names a non-
// local peer-id, end-to-end against an in-process server peer.
func TestAppPeer_RemoteGetPutList(t *testing.T) {
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
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	client, err := CreatePeer(PeerConfig{Keypair: &clientKP})
	if err != nil {
		t.Fatal(err)
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
	case <-time.After(1 * time.Second):
		t.Fatal("server not ready")
	}

	conn, err := client.Connect(ctx, server.Addr().String())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer conn.Close()

	serverID := string(serverKP.PeerID())

	// Put a value on the server via the high-level surface using a
	// peer-qualified path.
	remotePath := "/" + serverID + "/demo/value"
	if _, err := client.Put(remotePath, "test/scalar", 123); err != nil {
		t.Fatalf("remote Put: %v", err)
	}

	// Confirm the entity actually landed on the server's tree.
	if !server.Store().Has("demo/value") {
		t.Fatal("server tree did not receive the remote Put")
	}

	// Get it back via the same surface.
	got, ok, err := client.Get(remotePath)
	if err != nil {
		t.Fatalf("remote Get: %v", err)
	}
	if !ok {
		t.Fatal("remote Get: not found after Put")
	}
	if got.Type != "test/scalar" {
		t.Errorf("got type %q want %q", got.Type, "test/scalar")
	}

	// List the parent directory remotely.
	rows, err := client.List("/" + serverID + "/demo")
	if err != nil {
		t.Fatalf("remote List: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("remote List returned no rows")
	}
	foundValue := false
	for _, r := range rows {
		if r.Name == "value" {
			foundValue = true
		}
	}
	if !foundValue {
		t.Errorf("remote List did not return 'value' entry; got %+v", rows)
	}

	// Local Get on the same client AppPeer must still work — bare
	// paths take the local branch.
	if _, err := client.Put("local/v", "test/scalar", 7); err != nil {
		t.Fatalf("local Put: %v", err)
	}
	if _, ok, err := client.Get("local/v"); err != nil || !ok {
		t.Fatalf("local Get round-trip broken: ok=%v err=%v", ok, err)
	}
}

// TestExecutor_NoRemoteWiringRejects verifies that an Executor without
// a RemoteExecute callback returns a clear error when handed a non-local
// URI, rather than silently misrouting through the local registry.
func TestExecutor_NoRemoteWiringRejects(t *testing.T) {
	kp, err := crypto.Generate()
	if err != nil {
		t.Fatal(err)
	}
	ap, err := CreatePeer(PeerConfig{Keypair: &kp})
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()

	// Detach the remote-execute callback to simulate a bare Executor.
	ap.Executor().setRemoteExecute(nil)

	otherKP, err := crypto.Generate()
	if err != nil {
		t.Fatal(err)
	}
	otherID := string(otherKP.PeerID())

	resourceURI := "entity://" + otherID + "/foo"
	handlerURI := "entity://" + otherID + "/system/tree"
	getReq, resource, err := tree.CreateGetRequest(resourceURI, "entity")
	if err != nil {
		t.Fatal(err)
	}

	_, err = ap.Executor().ExecuteOnResource(handlerURI, "get", getReq, resource)
	if err == nil {
		t.Fatal("expected error when remote dispatch unwired")
	}
}
