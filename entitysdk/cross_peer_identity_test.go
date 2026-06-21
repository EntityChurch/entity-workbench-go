package entitysdk_test

import (
	"context"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"
	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/role"

	"entity-workbench-go/entitysdk"
)

// TestCrossPeer_IdentityAndRoleEndToEnd is the load-bearing
// integration test for the multi-peer + identity stack composition.
//
// Scenario: two AppPeers (alice listening, bob connecting), both
// bootstrapped identity-aware on independent bundles. Bob does
// cross-peer role operations against alice's tree. After each step,
// the test asserts state on alice's local store directly (we trust
// the dispatch wire because the existing TestAppPeer_RemoteGetPutList
// already verifies the basic remote-dispatch path).
//
// What this proves end-to-end:
//   - Two identity-aware peers can coexist in one process.
//   - A connecting peer's connection-time cap (open-access in this
//     test) suffices for role.Define on the server side under RL2.
//   - Cross-peer role.Assign minted a role-derived cap on the
//     server's tree, addressed under the server's namespace.
//   - The full Cut 1+2+3 stack composes — identity bootstrap doesn't
//     break role; remote dispatch doesn't break either; the local
//     and remote caller-cap paths agree on what authorization means.
func TestCrossPeer_IdentityAndRoleEndToEnd(t *testing.T) {
	// --- Setup: two peers, both identity-aware ---------------------

	// Alice (the server). Listens with open-access connection grants
	// so any handshaked client gets a wildcard cap — sufficient to
	// satisfy RL2 on subsequent role ops.
	alice, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("CreatePeer alice: %v", err)
	}
	t.Cleanup(func() { _ = alice.Close() })

	if _, err := alice.BootstrapIdentity(context.Background(), entitysdk.BootstrapOpts{
		QuorumName: "alice-quorum",
	}); err != nil {
		t.Fatalf("Alice BootstrapIdentity: %v", err)
	}
	aliceID := alice.PeerID()

	// Bob (the client). Different identity bundle.
	bob, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer bob: %v", err)
	}
	t.Cleanup(func() { _ = bob.Close() })

	bobBootstrap, err := bob.BootstrapIdentity(context.Background(), entitysdk.BootstrapOpts{
		QuorumName: "bob-quorum",
	})
	if err != nil {
		t.Fatalf("Bob BootstrapIdentity: %v", err)
	}

	// Sanity: distinct identities.
	if alice.IdentityHash() == bob.IdentityHash() {
		t.Fatalf("alice and bob have the same identity hash; bootstrap not generating fresh keypairs")
	}
	if len(bobBootstrap.LocalToControllerCaps) == 0 {
		t.Fatalf("bob did not receive a controller cap")
	}

	// --- Bring up alice's listener; bob handshakes -----------------

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

	conn, err := bob.Connect(ctx, alice.Addr().String())
	if err != nil {
		t.Fatalf("bob.Connect: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	// --- Cross-peer role op: bob defines a role on alice -----------

	roleCli := bob.RoleAt(aliceID)
	contextName := "cross-peer-ctx"
	roleName := "remote-reader"
	grants := []types.GrantEntry{{
		Handlers:   types.CapabilityScope{Include: []string{"system/tree"}},
		Operations: types.CapabilityScope{Include: []string{"get", "list"}},
		Resources:  types.CapabilityScope{Include: []string{"public/*"}},
	}}
	defineRes, err := roleCli.Define(ctx, contextName, roleName, grants, nil)
	if err != nil {
		t.Fatalf("cross-peer role.Define: %v", err)
	}
	if defineRes.RolePath == "" {
		t.Errorf("role.Define returned empty role path")
	}

	// Verify on alice's side: role definition is bound under alice's
	// namespace. Store.Has on a bare path canonicalizes to alice's
	// peer-id automatically.
	defPath := role.RoleDefinitionPath(contextName, roleName)
	if !alice.Store().Has(defPath) {
		t.Errorf("role definition not bound on alice at %s", defPath)
	}

	// --- Cross-peer role op: bob assigns himself the role ----------

	bobHash := bob.IdentityHash()
	assignRes, err := roleCli.Assign(ctx, contextName, bobHash, roleName)
	if err != nil {
		t.Fatalf("cross-peer role.Assign: %v", err)
	}
	if assignRes.AssignmentPath == "" {
		t.Errorf("role.Assign returned empty assignment path")
	}
	if len(assignRes.DerivedTokens) == 0 {
		t.Errorf("role.Assign returned no derived tokens")
	}

	// Verify on alice's side: assignment + role-derived cap bound
	// under alice's namespace.
	asnPath := role.AssignmentPath(contextName, bobHash, roleName)
	if !alice.Store().Has(asnPath) {
		t.Errorf("assignment not bound on alice at %s", asnPath)
	}
	capPath := role.RoleDerivedTokenPath(contextName, bobHash, assignRes.DerivedTokens[0])
	if !alice.Store().Has(capPath) {
		t.Errorf("role-derived cap not bound on alice at %s", capPath)
	}

	// --- Cross-peer role op: bob excludes himself ------------------
	//
	// Layer-1 sweep should revoke the role-derived cap. Verifies the
	// fleet-wide sync-hook cascade fires correctly under remote
	// dispatch (the hook runs on alice's side, where the binding
	// lives, after the dispatched op causes the exclusion to bind).

	if _, err := roleCli.Exclude(ctx, contextName, bobHash); err != nil {
		t.Fatalf("cross-peer role.Exclude: %v", err)
	}
	excPath := role.ExclusionPath(contextName, bobHash)
	if !alice.Store().Has(excPath) {
		t.Errorf("exclusion entity not bound on alice at %s", excPath)
	}
	if alice.Store().Has(capPath) {
		t.Errorf("role-derived cap still bound on alice after exclude (sweep failed): %s", capPath)
	}

	// --- Sanity: bob's local tree didn't accidentally absorb alice's writes
	//
	// Cross-peer ops mutate the target peer's tree, not the caller's.
	// If our remote-dispatch path leaked into bob's local store, this
	// would catch it.

	if bob.Store().Has(defPath) {
		t.Errorf("bob's local store has the role definition; remote dispatch leaked")
	}
}

// TestCrossPeer_IdentityHashesAreStableAcrossBootstrap is a
// supporting check: re-running BootstrapIdentity from the same
// bundle (Cut 3 deterministic-rebootstrap path) reproduces the
// same controller-cert hash even when the peer connects and does
// cross-peer ops in between.
//
// This guards against a subtle bug class: if remote dispatch ever
// writes to the local store in a way that affects subsequent
// bootstrap ceremonies (e.g., persisting a cached attestation that
// changes the canonical CBOR encoding of subsequent ops), the
// deterministic-ceremony invariant would silently break.
func TestCrossPeer_IdentityHashesAreStableAcrossBootstrap(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	alice, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("CreatePeer alice: %v", err)
	}
	t.Cleanup(func() { _ = alice.Close() })

	res1, err := alice.BootstrapIdentity(context.Background(), entitysdk.BootstrapOpts{
		QuorumName: "stable-test",
		BundleName: "alice-stable",
	})
	if err != nil {
		t.Fatalf("alice initial bootstrap: %v", err)
	}
	originalCertHash := res1.ControllerCertHash

	// Drive some traffic through alice's listener so its tree state
	// has accumulated something beyond the bootstrap-time entities.
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

	bob, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer bob: %v", err)
	}
	t.Cleanup(func() { _ = bob.Close() })
	conn, err := bob.Connect(ctx, alice.Addr().String())
	if err != nil {
		t.Fatalf("bob.Connect: %v", err)
	}
	defer conn.Close()
	if _, err := bob.RoleAt(alice.PeerID()).Define(ctx, "x", "y",
		[]types.GrantEntry{{
			Handlers:   types.CapabilityScope{Include: []string{"system/tree"}},
			Operations: types.CapabilityScope{Include: []string{"get"}},
			Resources:  types.CapabilityScope{Include: []string{"*"}},
		}}, nil); err != nil {
		t.Fatalf("bob remote Define: %v", err)
	}

	// Close alice and reload from the same bundle.
	_ = alice.Close()
	alice2, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		Identity: &entitysdk.IdentityBindingConfig{Name: "alice-stable"},
	})
	if err != nil {
		t.Fatalf("alice reload: %v", err)
	}
	t.Cleanup(func() { _ = alice2.Close() })

	// The reloaded peer's controller-cert hash must match the original
	// — confirming Cut 3's determinism holds when remote dispatch was
	// in play between bootstraps.
	if !alice2.Store().Has("system/identity/internal/cert/" +
		hexBytes(originalCertHash.Bytes())) {
		t.Errorf("reloaded alice doesn't have the original controller cert at the canonical path")
	}
}
