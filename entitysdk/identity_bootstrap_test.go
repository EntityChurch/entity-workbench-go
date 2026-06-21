package entitysdk_test

import (
	"context"
	"testing"

	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

// TestBootstrapIdentity_OneOfOne validates the simplest valid
// identity bootstrap: 1-member quorum, threshold 1, local peer as
// controller. After BootstrapIdentity returns, the peer-config
// entity is bound and the local→controller cap is issued.
func TestBootstrapIdentity_OneOfOne(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	res, err := ap.BootstrapIdentity(context.Background(), entitysdk.BootstrapOpts{
		QuorumName: "one-of-one",
	})
	if err != nil {
		t.Fatalf("BootstrapIdentity: %v", err)
	}

	if res.QuorumID.IsZero() {
		t.Errorf("QuorumID is zero")
	}
	if res.ControllerCertHash.IsZero() {
		t.Errorf("ControllerCertHash is zero")
	}
	if res.PeerConfigPath == "" {
		t.Errorf("PeerConfigPath empty")
	}
	if len(res.LocalToControllerCaps) == 0 {
		t.Errorf("LocalToControllerCaps empty — expected at least one cap")
	}
	if len(res.QuorumMembers) != 1 {
		t.Errorf("expected 1 quorum member, got %d", len(res.QuorumMembers))
	}

	// peer-config must be bound at the canonical path.
	if !ap.Store().Has(res.PeerConfigPath) {
		t.Errorf("peer-config not bound at %s", res.PeerConfigPath)
	}
}

// TestBootstrapIdentity_ThreeOfThree validates the user-stated
// "three quorum" deployment shape — 3-member quorum, threshold 3,
// local peer as controller. Equivalent to a "majority requires all"
// minimal team setup before the K=2 default kicks in for real
// deployments.
func TestBootstrapIdentity_ThreeOfThree(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	res, err := ap.BootstrapIdentity(context.Background(), entitysdk.BootstrapOpts{
		QuorumMembers:   3,
		QuorumThreshold: 3,
		QuorumName:      "three-of-three",
	})
	if err != nil {
		t.Fatalf("BootstrapIdentity: %v", err)
	}
	if len(res.QuorumMembers) != 3 {
		t.Errorf("expected 3 quorum members, got %d", len(res.QuorumMembers))
	}
	if res.PeerConfigPath == "" || res.ControllerCertHash.IsZero() {
		t.Errorf("incomplete BootstrapResult: %+v", res)
	}
}

// TestBootstrapIdentity_TwoOfThree exercises the more interesting
// K-of-N case: 3 members, threshold 2 (not all sign). Verifies that
// the partial signature set is sufficient for identity.Startup's
// K-of-N verification to issue the cap.
func TestBootstrapIdentity_TwoOfThree(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	res, err := ap.BootstrapIdentity(context.Background(), entitysdk.BootstrapOpts{
		QuorumMembers:   3,
		QuorumThreshold: 2,
		QuorumName:      "two-of-three",
	})
	if err != nil {
		t.Fatalf("BootstrapIdentity: %v", err)
	}
	if res.PeerConfigPath == "" {
		t.Errorf("PeerConfigPath empty")
	}
	if len(res.LocalToControllerCaps) == 0 {
		t.Errorf("LocalToControllerCaps empty")
	}
}

// TestBootstrapIdentity_RejectsBadThreshold ensures the helper
// rejects threshold > members early rather than relying on the
// underlying handler's validation.
func TestBootstrapIdentity_RejectsBadThreshold(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	_, err = ap.BootstrapIdentity(context.Background(), entitysdk.BootstrapOpts{
		QuorumMembers:   2,
		QuorumThreshold: 5,
	})
	if err == nil {
		t.Fatal("expected threshold>members to be rejected")
	}
	if entitysdk.StatusOf(err) != 400 {
		t.Errorf("expected 400, got status %d (%v)", entitysdk.StatusOf(err), err)
	}
}

// TestBootstrapIdentity_RoleStillWorksAfter validates that the role
// extension stays operational after the identity bootstrap. RL2 sees
// the peer-owner self-cap on local L1 dispatch (unchanged by
// bootstrap) and accepts role definitions / assignments. This is
// the cross-extension integration check — Cuts 1 and 2 compose.
func TestBootstrapIdentity_RoleStillWorksAfter(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	if _, err := ap.BootstrapIdentity(context.Background(), entitysdk.BootstrapOpts{
		QuorumMembers: 1,
	}); err != nil {
		t.Fatalf("BootstrapIdentity: %v", err)
	}

	rc := ap.Role()
	defineRes, err := rc.Define(context.Background(), "post-bootstrap-ctx", "reader",
		[]types.GrantEntry{{
			Handlers:   types.CapabilityScope{Include: []string{"system/tree"}},
			Operations: types.CapabilityScope{Include: []string{"get"}},
			Resources:  types.CapabilityScope{Include: []string{"*"}},
		}}, nil)
	if err != nil {
		t.Fatalf("Role.Define after bootstrap: %v", err)
	}
	if defineRes.RolePath == "" {
		t.Errorf("Define returned empty role path")
	}

	asn, err := rc.Assign(context.Background(), "post-bootstrap-ctx",
		ap.IdentityHash(), "reader")
	if err != nil {
		t.Fatalf("Role.Assign after bootstrap: %v", err)
	}
	if len(asn.DerivedTokens) == 0 {
		t.Errorf("Role.Assign returned no derived tokens")
	}
}

// TestBootstrapIdentity_RejectsWhenIdentityStackDisabled checks the
// guard returns 503 when the IdentityStack extension is opted out.
func TestBootstrapIdentity_RejectsWhenIdentityStackDisabled(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		Extensions: entitysdk.ExtensionsConfig{
			IdentityStack: &entitysdk.IdentityStackConfig{Disabled: true},
		},
	})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	_, err = ap.BootstrapIdentity(context.Background(), entitysdk.BootstrapOpts{})
	if err == nil {
		t.Fatal("expected BootstrapIdentity to fail when IdentityStack is disabled")
	}
	if entitysdk.StatusOf(err) != 503 {
		t.Errorf("expected 503, got status %d (%v)", entitysdk.StatusOf(err), err)
	}
}
