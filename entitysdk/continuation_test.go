package entitysdk_test

import (
	"context"
	"testing"

	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

// TestContinuationClient_InstallRoundtrip: install a forward
// continuation, verify the entity is bound at the chosen path, and
// confirm the install handler's returned path matches.
//
// Scope: ContinuationClient wrapper correctness. Downstream
// advance-dispatch behavior is the responsibility of the continuation
// handler in core-go and is exercised end-to-end by the Phase C
// revision-follow integration test.
func TestContinuationClient_InstallRoundtrip(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	// Use the peer-owner self-cap as the dispatch_capability. It's
	// already persisted to the local content store at peer construction
	// and authority-walkable from the local identity, so install's R1
	// chain-root check succeeds without further setup.
	capHash := ap.OwnerCapability().ContentHash

	// Build a minimal forward continuation. No deliver_to, no
	// result_field, no params — the continuation handler accepts this
	// shape (trigger-only continuation that fires its target+op when
	// advanced).
	const installPath = "system/inbox/test/probe"
	contEnt, err := types.ContinuationData{
		Target:             "system/tree",
		Operation:          "get",
		Resource:           &types.ResourceTarget{Targets: []string{"observed/marker"}},
		DispatchCapability: capHash,
	}.ToEntity()
	if err != nil {
		t.Fatal(err)
	}

	cc := ap.Continuation()
	gotPath, err := cc.Install(context.Background(), installPath, contEnt)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if gotPath != installPath {
		t.Errorf("install path = %q, want %q", gotPath, installPath)
	}

	// The handler put the continuation entity at the install path; the
	// tree now resolves to a system/continuation entity.
	got, ok, err := ap.Get(installPath)
	if err != nil {
		t.Fatalf("get installed continuation: %v", err)
	}
	if !ok {
		t.Fatal("installed continuation not found at install path")
	}
	if got.Type != types.TypeContinuation {
		t.Errorf("installed entity type = %q, want %q", got.Type, types.TypeContinuation)
	}
}

// TestContinuationClient_AbandonNoError: smoke-tests the Abandon
// wrapper — confirms the params encode + dispatch path is correct
// (does not panic, returns nil or a typed sdk error).
//
// Spec §3.8 says abandon over a non-existent path is
// implementation-defined; we accept nil or 404 here.
func TestContinuationClient_AbandonNoError(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	const path = "system/continuation/suspended/test-noop"
	if err := ap.Continuation().Abandon(context.Background(), path); err != nil {
		sdkErr, ok := err.(*entitysdk.Error)
		if !ok || sdkErr.Status != 404 {
			t.Errorf("abandon on missing path: got %v, want nil or 404", err)
		}
	}
}

// TestContinuationClient_InvalidParamsRejected: Install with an
// entity that's neither system/continuation nor system/continuation/join
// must be rejected client-side (no dispatch) with a 400.
func TestContinuationClient_InvalidParamsRejected(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	// A plain capability token is not a valid install payload.
	junkCap := ap.OwnerCapability()
	_, err = ap.Continuation().Install(context.Background(), "system/inbox/x", junkCap)
	if err == nil {
		t.Fatal("expected error for non-continuation entity, got nil")
	}
	sdkErr, ok := err.(*entitysdk.Error)
	if !ok || sdkErr.Status != 400 {
		t.Errorf("got %v, want 400", err)
	}
}
