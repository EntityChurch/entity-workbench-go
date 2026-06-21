package entitysdk_test

import (
	"context"
	"testing"

	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/role"

	"entity-workbench-go/entitysdk"
)

// TestRoleClient_DefineAssignUnassign exercises the happy path: define a
// role, assign a peer to it, verify the assignment + role-derived cap
// are bound in the tree, then unassign and verify they're gone.
//
// Conformance check: the test runs on a default-config AppPeer (no
// explicit ExtensionsConfig.Role set), which means role is wired by
// default per the documented convention.
func TestRoleClient_DefineAssignUnassign(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	ctx := context.Background()
	rc := ap.Role()

	// Identity hash for the assignee — we use the local peer's own
	// identity hash for simplicity. In practice this would be a remote
	// peer's identity hash.
	assigneeHash := ap.IdentityHash()
	if assigneeHash.IsZero() {
		t.Fatalf("local peer has zero identity hash; check Identity() wiring")
	}

	contextName := "test-ctx"
	roleName := "reader"
	grants := []types.GrantEntry{
		{
			Handlers:   types.CapabilityScope{Include: []string{"system/tree"}},
			Operations: types.CapabilityScope{Include: []string{"get", "list"}},
			Resources:  types.CapabilityScope{Include: []string{"public/*"}},
		},
	}

	// 1. Define the role.
	defineResult, err := rc.Define(ctx, contextName, roleName, grants, nil)
	if err != nil {
		t.Fatalf("Define: %v", err)
	}
	if defineResult.RolePath == "" {
		t.Fatalf("Define returned empty role path")
	}

	// 2. Verify the role definition is bound.
	defPath := role.RoleDefinitionPath(contextName, roleName)
	if !ap.Store().Has(defPath) {
		t.Fatalf("role definition not bound at %s", defPath)
	}

	// 3. Assign the local peer's identity to the role.
	assignResult, err := rc.Assign(ctx, contextName, assigneeHash, roleName)
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if assignResult.AssignmentPath == "" {
		t.Fatalf("Assign returned empty assignment path")
	}
	if len(assignResult.DerivedTokens) == 0 {
		t.Fatalf("Assign returned no derived tokens")
	}

	// 4. Verify the assignment + at least one role-derived cap.
	asnPath := role.AssignmentPath(contextName, assigneeHash, roleName)
	if !ap.Store().Has(asnPath) {
		t.Fatalf("assignment not bound at %s", asnPath)
	}
	capPath := role.RoleDerivedTokenPath(contextName, assigneeHash, assignResult.DerivedTokens[0])
	if !ap.Store().Has(capPath) {
		t.Fatalf("role-derived cap not bound at %s", capPath)
	}

	// 5. Unassign and verify both the assignment and cap are gone.
	if _, err := rc.Unassign(ctx, contextName, assigneeHash, roleName); err != nil {
		t.Fatalf("Unassign: %v", err)
	}
	if ap.Store().Has(asnPath) {
		t.Errorf("assignment still bound after unassign: %s", asnPath)
	}
	if ap.Store().Has(capPath) {
		t.Errorf("role-derived cap still bound after unassign: %s", capPath)
	}
}

// TestRoleClient_ExcludeSweepsCaps verifies the layer-1 sweep semantic
// of EXTENSION-ROLE §6.1: when a peer is excluded in a context, all
// role-derived caps issued to that peer in that context are revoked.
func TestRoleClient_ExcludeSweepsCaps(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	ctx := context.Background()
	rc := ap.Role()

	assigneeHash := ap.IdentityHash()
	contextName := "exclude-ctx"
	roleName := "writer"
	grants := []types.GrantEntry{
		{
			Handlers:   types.CapabilityScope{Include: []string{"system/tree"}},
			Operations: types.CapabilityScope{Include: []string{"put"}},
			Resources:  types.CapabilityScope{Include: []string{"scratch/*"}},
		},
	}

	if _, err := rc.Define(ctx, contextName, roleName, grants, nil); err != nil {
		t.Fatalf("Define: %v", err)
	}
	asn, err := rc.Assign(ctx, contextName, assigneeHash, roleName)
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	capPath := role.RoleDerivedTokenPath(contextName, assigneeHash, asn.DerivedTokens[0])
	if !ap.Store().Has(capPath) {
		t.Fatalf("role-derived cap not bound at %s before exclude", capPath)
	}

	// Exclude — the layer-1 sweep should revoke the cap.
	if _, err := rc.Exclude(ctx, contextName, assigneeHash); err != nil {
		t.Fatalf("Exclude: %v", err)
	}
	excPath := role.ExclusionPath(contextName, assigneeHash)
	if !ap.Store().Has(excPath) {
		t.Errorf("exclusion entity not bound at %s", excPath)
	}
	if ap.Store().Has(capPath) {
		t.Errorf("role-derived cap still bound after exclude (sweep failed): %s", capPath)
	}

	// Re-assigning while excluded should fail per layer-2 check (§6.5 IA12).
	_, err = rc.Assign(ctx, contextName, assigneeHash, roleName)
	if err == nil {
		t.Errorf("Assign of excluded peer succeeded; expected failure")
	} else if !entitysdk.IsForbidden(err) {
		t.Errorf("Assign of excluded peer: expected 403, got %v (status %d)",
			err, entitysdk.StatusOf(err))
	}

	// Unexclude does NOT auto-restore caps (§6.4); re-assignment is required.
	if _, err := rc.Unexclude(ctx, contextName, assigneeHash); err != nil {
		t.Fatalf("Unexclude: %v", err)
	}
	if ap.Store().Has(excPath) {
		t.Errorf("exclusion entity still bound after unexclude: %s", excPath)
	}
	if ap.Store().Has(capPath) {
		t.Errorf("role-derived cap reappeared after unexclude (should require re-assign): %s",
			capPath)
	}
}

// TestRoleClient_DefaultOff confirms that ExtensionsConfig.Role with
// Disabled: true opts the role extension out of CreatePeer's default-on
// posture. After opt-out, role ops return 404 (handler not registered;
// longest-prefix-miss).
func TestRoleClient_DefaultOff(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		Extensions: entitysdk.ExtensionsConfig{
			Role: &entitysdk.RoleConfig{Disabled: true},
		},
	})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	rc := ap.Role()
	_, err = rc.Define(context.Background(), "ctx", "role", nil, nil)
	if err == nil {
		t.Fatalf("Define succeeded with role disabled; expected 404")
	}
	if entitysdk.StatusOf(err) != 404 {
		t.Errorf("expected 404 for disabled role, got status %d (%v)",
			entitysdk.StatusOf(err), err)
	}
}
