package entitysdk

import (
	"context"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/crypto"
	"go.entitychurch.org/entity-core-go/core/peer"
)

// resolve_chain_test.go — the proof for PROPOSAL-UNIVERSAL-RESOLUTION's
// SDK claim: that `resolve()` is a real, working composition of the four
// landed rungs, returning a typed Outcome at whichever rung stops it.
//
// Per the proposal's meta-rule (PRIMER): "a resolution claim is not
// validated until a cross-impl conformance test exercises the *composed*
// chain. Prose does not catch resolution bugs." This is the Go-side
// RESOLVE-CHAIN-* walk against two real peers. It proves the seam
// composes today on the BUILT rungs (local-name resolve + connect +
// tree:get + content); the peer-issued backend is a drop-in at the name
// rung that changes only *where the binding comes from* — the downstream
// chain is identical (MODEL §7).

// twoPeers stands up a listening server (with a published page) and a
// client, returning both and the server's dial address.
func twoPeers(t *testing.T) (client, server *AppPeer, serverAddr string) {
	t.Helper()
	serverKP, err := crypto.Generate()
	if err != nil {
		t.Fatal(err)
	}
	clientKP, err := crypto.Generate()
	if err != nil {
		t.Fatal(err)
	}

	server, err = CreatePeer(PeerConfig{
		Keypair:    &serverKP,
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("server CreatePeer: %v", err)
	}
	t.Cleanup(func() { server.Close() })

	// The resolution substrate is opt-in; the client is the resolver.
	client, err = CreatePeer(PeerConfig{
		Keypair:    &clientKP,
		Extensions: ExtensionsConfig{Registry: &RegistryConfig{}},
	})
	if err != nil {
		t.Fatalf("client CreatePeer: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	// Publish a page on the server (the content the chain fetches).
	if _, err := server.Put("demo/page", "test/scalar", 42); err != nil {
		t.Fatalf("server publish: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	ready := make(chan struct{})
	listenErr := make(chan error, 1)
	go func() { listenErr <- server.ListenReady(ctx, ready) }()
	select {
	case <-ready:
	case err := <-listenErr:
		t.Fatalf("server listen: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("server not ready in 2s")
	}
	return client, server, server.Addr().String()
}

// TestResolveChain_HappyPath walks the whole chain a SITE link triggers:
// name → peer_id (registry resolve) → reach (connect) → path → hash →
// bytes (tree:get + content). This is RESOLVE-CHAIN-HAPPY-1.
func TestResolveChain_HappyPath(t *testing.T) {
	client, server, addr := twoPeers(t)
	serverID := string(server.PeerID())

	// --- rung A: name → peer_id (the rung that had no SDK surface) ---
	if err := client.EnableLocalNameResolver(); err != nil {
		t.Fatalf("EnableLocalNameResolver: %v", err)
	}
	if _, err := client.BindLocalName("bills-lab", serverID, nil); err != nil {
		t.Fatalf("BindLocalName: %v", err)
	}
	res, err := client.ResolveName("bills-lab")
	if err != nil {
		t.Fatalf("ResolveName: %v", err)
	}
	if res.PeerID != serverID {
		t.Fatalf("resolved peer_id = %q, want %q", res.PeerID, serverID)
	}
	t.Logf("rung A: bills-lab → %s (status %s)", res.PeerID, res.Status)

	// --- rung B: reach (Connect to the resolved peer) ---
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := client.Connect(ctx, addr)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer conn.Close()

	// --- rungs C+D: path → hash → bytes (over the reached peer) ---
	ent, err := client.BrowseFetch(res.PeerID, "demo/page")
	if err != nil {
		t.Fatalf("BrowseFetch: %v", err)
	}
	if ent.Type != "test/scalar" {
		t.Fatalf("fetched entity type = %q, want test/scalar", ent.Type)
	}
	t.Logf("rungs C+D: /%s/demo/page → entity %s (hash %s)", res.PeerID, ent.Type, ent.ContentHash.String())
	t.Log("PROVEN: name → peer_id → reach → path → hash → bytes composes end-to-end on the BUILT rungs")
}

// TestResolveChain_NotFoundName: a name absent from the chain fails
// closed at the name rung (RESOLVE-CHAIN-NOTFOUND-NAME-1).
func TestResolveChain_NotFoundName(t *testing.T) {
	client, _, _ := twoPeers(t)
	if err := client.EnableLocalNameResolver(); err != nil {
		t.Fatalf("EnableLocalNameResolver: %v", err)
	}
	_, err := client.ResolveName("no-such-name")
	o := AsOutcome(err)
	if o == nil {
		t.Fatalf("want *Outcome, got %T: %v", err, err)
	}
	if o.Kind != OutcomeNotFound || o.Rung != RungName {
		t.Fatalf("got Outcome{%s,%s}, want {not_found,name}", o.Kind, o.Rung)
	}
}

// TestResolveChain_PinMismatch: name@peer_id where the resolved peer-id
// != the pin returns PinMismatch, not silent acceptance
// (NAME-GRAMMAR-PIN-1; the outcome PROPOSAL-UNIVERSAL-RESOLUTION §3.1
// omits).
func TestResolveChain_PinMismatch(t *testing.T) {
	client, server, _ := twoPeers(t)
	if err := client.EnableLocalNameResolver(); err != nil {
		t.Fatalf("EnableLocalNameResolver: %v", err)
	}
	if _, err := client.BindLocalName("bills-lab", string(server.PeerID()), nil); err != nil {
		t.Fatalf("BindLocalName: %v", err)
	}
	_, err := client.ResolveNamePinned("bills-lab", "z6MkWrongPeerIdThatDoesNotMatch")
	o := AsOutcome(err)
	if o == nil {
		t.Fatalf("want *Outcome, got %T: %v", err, err)
	}
	if o.Kind != OutcomePinMismatch {
		t.Fatalf("got Outcome{%s}, want pin_mismatch", o.Kind)
	}
}

// TestResolveChain_Unreachable: a resolved-but-never-connected peer
// dead-ends at the transport rung (RESOLVE-CHAIN-UNREACHABLE-1).
func TestResolveChain_Unreachable(t *testing.T) {
	client, _, _ := twoPeers(t)
	ghost, err := crypto.Generate()
	if err != nil {
		t.Fatal(err)
	}
	_, ferr := client.BrowseFetch(string(ghost.PeerID()), "demo/page")
	o := AsOutcome(ferr)
	if o == nil {
		t.Fatalf("want *Outcome, got %T: %v", ferr, ferr)
	}
	if o.Kind != OutcomeUnreachable || o.Rung != RungTransport {
		t.Fatalf("got Outcome{%s,%s}, want {unreachable,transport}", o.Kind, o.Rung)
	}
}

// TestResolveChain_Classifier: the typed-Outcome classifier maps the
// SDK's existing status codes onto the §3.1 taxonomy on the F1 axes —
// 404→NotFound (reachability), 403→Denied (authorization), 503→Pending
// (transient). This is the §8 confidentiality contract proven at the
// unit level: a peer's 404 is never upgraded to Denied.
func TestResolveChain_Classifier(t *testing.T) {
	cases := []struct {
		status uint
		want   OutcomeKind
	}{
		{404, OutcomeNotFound},
		{403, OutcomeDenied},
		{503, OutcomePending},
	}
	for _, c := range cases {
		o := classifyError(RungContent, NewError(c.status, "x", "y"))
		if o == nil {
			t.Fatalf("status %d: want Outcome, got nil", c.status)
		}
		if o.Kind != c.want {
			t.Fatalf("status %d: got %s, want %s", c.status, o.Kind, c.want)
		}
	}
	// A 500 is not a clean rung dead-end: classifier returns nil so the
	// caller propagates the raw error.
	if o := classifyError(RungContent, NewError(500, "x", "y")); o != nil {
		t.Fatalf("status 500: want nil (raw error propagates), got %v", o)
	}
}
