package entitysdk_test

import (
	"context"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/capability"
	"go.entitychurch.org/entity-core-go/core/peer"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

// TestMintCrossPeerChainCapability_Shape verifies the SDK helper produces
// a cap that satisfies the EXTENSION-CONTINUATION §4.2 case 3 contract:
// B-rooted authority chain, installer in-chain, grantee = host peer.
// The shape is what makes both the install-time in-chain check (§3.1a)
// and the remote advance-time `VerifyChain` succeed.
func TestMintCrossPeerChainCapability_Shape(t *testing.T) {
	alice, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("CreatePeer alice: %v", err)
	}
	t.Cleanup(func() { _ = alice.Close() })

	bob, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer bob: %v", err)
	}
	t.Cleanup(func() { _ = bob.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ready := make(chan struct{})
	listenErr := make(chan error, 1)
	go func() { listenErr <- alice.ListenReady(ctx, ready) }()
	select {
	case <-ready:
	case err := <-listenErr:
		t.Fatalf("alice listen: %v", err)
	case <-time.After(1 * time.Second):
		t.Fatal("alice not ready")
	}

	if _, err := bob.Connect(ctx, alice.Addr().String()); err != nil {
		t.Fatalf("bob.Connect: %v", err)
	}

	// Bob mints a cross-peer chain cap rooted at alice's conferred grant.
	grants := []types.GrantEntry{{
		Handlers:   types.CapabilityScope{Include: []string{"system/revision"}},
		Operations: types.CapabilityScope{Include: []string{"fetch", "fetch-entities"}},
		Resources:  types.CapabilityScope{Include: []string{"*"}},
	}}
	capEnt, err := bob.MintCrossPeerChainCapability(string(alice.PeerID()), grants, nil)
	if err != nil {
		t.Fatalf("MintCrossPeerChainCapability: %v", err)
	}

	// (1) Chain shape: leaf -> alice-conferred root. Length 2.
	leafData, err := types.CapabilityTokenDataFromEntity(capEnt)
	if err != nil {
		t.Fatalf("decode leaf cap: %v", err)
	}
	bobIDHash := bob.IdentityHash()
	aliceIDHash := alice.IdentityHash()

	// Leaf grantee MUST be bob (the dispatching host peer / EXECUTE author).
	if leafData.Grantee != bobIDHash {
		t.Fatalf("leaf grantee = %s, want bob %s", leafData.Grantee, bobIDHash)
	}
	// Leaf granter MUST be bob (the installer, re-attenuation leaf).
	if g, single := leafData.Granter.SingleHash(); !single || g != bobIDHash {
		t.Fatalf("leaf granter must be bob (single-sig); got single=%v g=%s", single, g)
	}
	// Leaf resources MUST have bare "*" rewritten to "/{remotePeerID}/*"
	// per V7 v7.73 §PR-8 — the helper canonicalizes the caller-provided
	// bare wildcard so the leaf's authority covers the dispatch target's
	// namespace, not the granter's (which would be a no-op for cross-peer).
	wantResource := "/" + string(alice.PeerID()) + "/*"
	if len(leafData.Grants) != 1 || len(leafData.Grants[0].Resources.Include) != 1 ||
		leafData.Grants[0].Resources.Include[0] != wantResource {
		t.Errorf("leaf resources = %v; want bare `*` rewritten to %q (V7 §PR-8 canonicalization)",
			leafData.Grants[0].Resources.Include, wantResource)
	}

	// Parent MUST be set (chain isn't installer-rooted).
	if leafData.Parent == nil || leafData.Parent.IsZero() {
		t.Fatal("leaf parent must be set (chain MUST NOT be installer-rooted)")
	}

	// (2) Bundle covers leaf + signature + identity + parent + parent-sig + parent-id.
	bundle, err := bob.BundleCrossPeerChain(capEnt)
	if err != nil {
		t.Fatalf("BundleCrossPeerChain: %v", err)
	}
	if _, ok := bundle[capEnt.ContentHash]; !ok {
		t.Error("bundle missing leaf cap")
	}
	if _, ok := bundle[*leafData.Parent]; !ok {
		t.Error("bundle missing parent (connection-grant) cap")
	}
	if _, ok := bundle[bobIDHash]; !ok {
		t.Error("bundle missing bob's identity (leaf granter)")
	}
	if _, ok := bundle[aliceIDHash]; !ok {
		t.Error("bundle missing alice's identity (root granter)")
	}

	// (3) Chain walk: root MUST be alice-rooted, NOT bob-rooted.
	resolver := capability.IncludedResolver(bundle)
	chain, err := capability.CollectAuthorityChain(capEnt, resolver)
	if err != nil {
		t.Fatalf("CollectAuthorityChain: %v", err)
	}
	if len(chain) < 2 {
		t.Fatalf("expected chain length >= 2 (leaf -> root); got %d", len(chain))
	}
	rootData, err := types.CapabilityTokenDataFromEntity(chain[len(chain)-1])
	if err != nil {
		t.Fatalf("decode root cap: %v", err)
	}
	rootGranter, single := rootData.Granter.SingleHash()
	if !single {
		t.Fatal("root cap must have a single-sig granter (alice's connection grant)")
	}
	if rootGranter != aliceIDHash {
		t.Errorf("chain MUST be rooted at alice's conferred authority; got root granter %s, want alice %s", rootGranter, aliceIDHash)
	}
	if rootGranter == bobIDHash {
		t.Error("chain MUST NOT be installer-rooted (the cross-peer-breaking shape spec warns against)")
	}

	// (4) §3.1a install-time in-chain check passes for the installer
	//     (bob), so a local continuation install would not reject the
	//     cap as "writer not in chain". (Cross-peer VerifyChain at alice
	//     would also succeed — same chain — but that's exercised by the
	//     end-to-end follow test, not here.)
	includedSigResolver := capability.IncludedSignatureResolver(bundle)
	found, _, err := capability.CheckCreatorAuthority(capEnt, bobIDHash, resolver, includedSigResolver)
	if err != nil {
		t.Fatalf("CheckCreatorAuthority(installer=bob): %v", err)
	}
	if !found {
		t.Fatal("installer (bob) must be in-chain so the §3.1a install check passes")
	}
}

// TestMintCrossPeerChainCapability_NoConnection rejects with 404 when
// the remote isn't connected — the helper needs a live session capability
// as the chain parent.
func TestMintCrossPeerChainCapability_NoConnection(t *testing.T) {
	bob, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer bob: %v", err)
	}
	t.Cleanup(func() { _ = bob.Close() })

	_, err = bob.MintCrossPeerChainCapability("peer-that-isnt-connected", []types.GrantEntry{{
		Handlers:   types.CapabilityScope{Include: []string{"*"}},
		Operations: types.CapabilityScope{Include: []string{"*"}},
		Resources:  types.CapabilityScope{Include: []string{"*"}},
	}}, nil)
	if err == nil {
		t.Fatal("expected error when remote not connected; got nil")
	}
	if se, ok := err.(*entitysdk.Error); !ok || se.Status != 404 {
		t.Fatalf("expected SDK Error status=404, got %T %v", err, err)
	}
}
