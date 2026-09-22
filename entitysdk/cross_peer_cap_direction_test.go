package entitysdk

import (
	"context"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/capability"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/peer"
	"go.entitychurch.org/entity-core-go/core/types"
)

// The cross-peer chain cap MUST root at the TARGET's conferred authority,
// in the topology where BOTH peers have dialled each other.
//
// # Why this exists beside cross_peer_cap_mint_test.go
//
// That file already asserts the root-granter property — and it dials **one**
// direction, so the pool holds exactly one connection whose remote is alice
// and the lookup cannot pick a wrong one. Every continuation test that
// actually fails dials **both**, because the chain needs a route each way.
// That is the plural case, and a fixture modelling one instance of a plural
// relationship cannot fail on it (AP44). The two capabilities really are
// different, and only one of them is the one
// `findConnectionCapability`'s doc comment names:
//
//   - outbound (`PerformConnect`, core/peer/connection.go:1035) — `Capability`
//     is extracted from alice's AUTHENTICATE_RESPONSE, so its granter is
//     **alice**. This is "the B-conferred connection grant".
//   - inbound (server accept, core/peer/connection.go:905-908) — `Capability`
//     is `cs.GrantedCapability`, the cap **bob** granted alice on the way in.
//     Its granter is **bob**.
//
// Root a chain at the second and V7 §1.4's presented arm refuses it exactly as
// specified: `presentedAuthorizes` runs `VerifyChain(cap, included, target)`,
// whose root-granter check requires the chain ROOT's granter to be the target
// (core/protocol/outbound_authz.go:156-158). A bob-rooted chain is not a
// credential alice minted, so it relaxes nothing and Dimension 4 falls back to
// the executing handler's own grant.
//
// # What was measured, so nobody re-runs it
//
// This started as a hypothesis for core-go tracker row 23 — that
// `findConnectionCapability` ranges `Peer.Connections()` with no direction
// filter and picks the inbound session. **It is REFUTED, and the probe reached
// the mechanism rather than missing it** (AP43): with both directions dialled,
// bob's pool holds two connections to alice, one `IsOutbound()==true` carrying
// an alice-granted cap and one `IsOutbound()==false` carrying a bob-granted
// one — and the helper returned the alice-granted one. Row 23's cause is
// elsewhere and is still unknown.
//
// Also measured at the same time, and it is why these tests are kept rather
// than deleted with the hypothesis: over a real mint in this topology, **all
// six of the presented arm's predicates pass** (grantee, `VerifyChain` against
// the target, single-sig root, leaf-granter resolution, not-revoked, and
// `CheckPermission` coverage of `system/tree:extract` at alice), and
// `defaultHandlerSelfGrant` is `handlers:* operations:* resources:/*/*`, so the
// ambient arm covers Dimensions 1-3. The invariant below is therefore load
// bearing and nothing else in this tree asserts it in the plural topology.
//
// ⚠ The ordering these depend on is NOT guaranteed by anything.
// `findConnectionCapability` matches on `RemotePeerID` alone; it returns the
// right capability here because of the order `Peer.Connections()` happens to
// assemble its two slices in. core-go exposes `Connection.IsOutbound()` /
// `Direction()` as of their row 13. Filtering on it would make the property
// hold by construction instead of by luck — and these tests are what would
// catch it if the ordering ever moves first.

// Both peers dialled: the minted chain must still root at the target.
func TestMintCrossPeerChainCapability_RootsAtTheTargetWhenBothPeersHaveDialled(t *testing.T) {
	alice, bob := twoWayConnectedPair(t)

	capEnt, err := bob.MintCrossPeerChainCapability(string(alice.PeerID()), treeExtractGrants(alice), nil)
	if err != nil {
		t.Fatalf("MintCrossPeerChainCapability: %v", err)
	}

	rootGranter := rootGranterOf(t, bob, capEnt)

	if rootGranter == bob.IdentityHash() {
		t.Fatalf("the chain rooted at BOB's own grant, so it is not a credential the target minted: "+
			"V7 §1.4's presented arm refuses it at VerifyChain's root-granter check, Dimension 4 falls "+
			"back to the executing handler's grant, and a cross-peer sub-dispatch is denied at the "+
			"SENDER. root granter = %s (bob), want %s (alice). The pool holds both directions for "+
			"alice; use Connection.IsOutbound() rather than RemotePeerID alone", rootGranter, alice.IdentityHash())
	}
	if rootGranter != alice.IdentityHash() {
		t.Fatalf("root granter = %s, want alice %s", rootGranter, alice.IdentityHash())
	}
}

// One direction only — the topology every other cross-peer cap test uses.
// It passes whatever the pool ordering is, which is exactly why it could
// never have caught the case above; it is kept as the control arm that
// isolates *the second connection* as the variable.
func TestMintCrossPeerChainCapability_RootsAtTheTargetWithOneDirectionDialled(t *testing.T) {
	alice, bob := oneWayConnectedPair(t)

	capEnt, err := bob.MintCrossPeerChainCapability(string(alice.PeerID()), treeExtractGrants(alice), nil)
	if err != nil {
		t.Fatalf("MintCrossPeerChainCapability: %v", err)
	}

	if got := rootGranterOf(t, bob, capEnt); got != alice.IdentityHash() {
		t.Fatalf("root granter = %s, want alice %s", got, alice.IdentityHash())
	}
}

func treeExtractGrants(alice *AppPeer) []types.GrantEntry {
	return []types.GrantEntry{{
		Handlers:   types.CapabilityScope{Include: []string{"system/tree"}},
		Operations: types.CapabilityScope{Include: []string{"extract"}},
		Resources:  types.CapabilityScope{Include: []string{"/" + string(alice.PeerID()) + "/*"}},
	}}
}

// rootGranterOf walks the minted cap's authority chain and returns the ROOT's
// granter identity hash — the single value V7 §1.4 reads to decide whether a
// credential was minted by the target.
func rootGranterOf(t *testing.T, minter *AppPeer, capEnt entity.Entity) hash.Hash {
	t.Helper()

	bundle, err := minter.BundleCrossPeerChain(capEnt)
	if err != nil {
		t.Fatalf("BundleCrossPeerChain: %v", err)
	}
	chain, err := capability.CollectAuthorityChain(capEnt, capability.IncludedResolver(bundle))
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
		t.Fatal("root cap must have a single-sig granter (the target's connection grant)")
	}
	return rootGranter
}

// oneWayConnectedPair asserts that it really is one-way. Without this the
// control arm is indistinguishable from the experimental one and the pair
// isolates nothing.
func oneWayConnectedPair(t *testing.T) (alice, bob *AppPeer) {
	t.Helper()
	alice, bob, ctx := listeningPair(t)
	if _, err := bob.Connect(ctx, alice.Addr().String()); err != nil {
		t.Fatalf("bob.Connect(alice): %v", err)
	}
	if in, out := bob.connectionDirectionsTo(string(alice.PeerID())); in || !out {
		t.Fatalf("control arm is not the one-way case: inbound=%v outbound=%v", in, out)
	}
	return alice, bob
}

// twoWayConnectedPair leaves bob's pool holding two connections whose remote
// is alice. The premise is asserted rather than assumed — without it these
// tests are the one-way arm wearing a longer setup.
func twoWayConnectedPair(t *testing.T) (alice, bob *AppPeer) {
	t.Helper()
	alice, bob, ctx := listeningPair(t)
	if _, err := bob.Connect(ctx, alice.Addr().String()); err != nil {
		t.Fatalf("bob.Connect(alice): %v", err)
	}
	if _, err := alice.Connect(ctx, bob.Addr().String()); err != nil {
		t.Fatalf("alice.Connect(bob): %v", err)
	}

	aliceID := string(alice.PeerID())
	deadline := time.Now().Add(3 * time.Second)
	for {
		if in, out := bob.connectionDirectionsTo(aliceID); in && out {
			return alice, bob
		}
		if time.Now().After(deadline) {
			t.Fatal("PREMISE NOT MET: bob does not hold both an inbound and an outbound " +
				"connection to alice, so this arm is the one-way arm with extra setup")
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func listeningPair(t *testing.T) (alice, bob *AppPeer, ctx context.Context) {
	t.Helper()

	mk := func(name string) *AppPeer {
		p, err := CreatePeer(PeerConfig{
			ListenAddr: "127.0.0.1:0",
			RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
		})
		if err != nil {
			t.Fatalf("CreatePeer %s: %v", name, err)
		}
		t.Cleanup(func() { _ = p.Close() })
		return p
	}
	alice, bob = mk("alice"), mk("bob")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)

	for _, p := range []*AppPeer{alice, bob} {
		ready := make(chan struct{})
		errCh := make(chan error, 1)
		go func(ap *AppPeer) { errCh <- ap.ListenReady(ctx, ready) }(p)
		select {
		case <-ready:
		case err := <-errCh:
			t.Fatalf("listen: %v", err)
		case <-time.After(2 * time.Second):
			t.Fatal("listen timeout")
		}
	}
	return alice, bob, ctx
}

// connectionDirectionsTo reports whether this peer's pool holds an inbound
// and/or an outbound connection whose remote is peerID. Used only to assert
// the premise of the two-way arm above.
func (a *AppPeer) connectionDirectionsTo(peerID string) (inbound, outbound bool) {
	for _, c := range a.peer.Connections() {
		sess := c.Session()
		if sess == nil || string(sess.RemotePeerID) != peerID {
			continue
		}
		if c.IsOutbound() {
			outbound = true
		} else {
			inbound = true
		}
	}
	return inbound, outbound
}
