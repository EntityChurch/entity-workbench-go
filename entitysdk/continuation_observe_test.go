package entitysdk_test

import (
	"context"
	"testing"

	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

// TestContinuationClient_ListAt_AfterInstall: install a couple of
// continuations under a known inbox prefix and verify ListAt
// returns them with parsed views.
func TestContinuationClient_ListAt_AfterInstall(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	capHash := ap.OwnerCapability().ContentHash
	cc := ap.Continuation()
	ctx := context.Background()

	// Install two distinct forward continuations under a common
	// inbox prefix.
	pathA := entitysdk.InboxPath("test", "alpha", "fetch")
	pathB := entitysdk.InboxPath("test", "alpha", "merge")

	contA, _ := types.ContinuationData{
		Target:             "system/tree",
		Operation:          "get",
		Resource:           &types.ResourceTarget{Targets: []string{"a/"}},
		DispatchCapability: capHash,
	}.ToEntity()
	contB, _ := types.ContinuationData{
		Target:             "system/tree",
		Operation:          "get",
		Resource:           &types.ResourceTarget{Targets: []string{"b/"}},
		DispatchCapability: capHash,
	}.ToEntity()

	if _, err := cc.Install(ctx, pathA, contA); err != nil {
		t.Fatalf("install A: %v", err)
	}
	if _, err := cc.Install(ctx, pathB, contB); err != nil {
		t.Fatalf("install B: %v", err)
	}

	views, err := cc.ListAt(ctx, entitysdk.InboxPath("test", "alpha", ""))
	if err != nil {
		t.Fatalf("ListAt: %v", err)
	}
	if len(views) != 2 {
		t.Fatalf("expected 2 continuations, got %d: %v", len(views), summaryStrings(views))
	}

	// Every view should be Kind=forward and have target/operation
	// populated.
	for _, v := range views {
		if v.Kind != entitysdk.ContinuationKindForward {
			t.Errorf("view %q kind = %v, want forward", v.Path, v.Kind)
		}
		if v.Target == "" || v.Operation == "" {
			t.Errorf("view %q has empty target/operation", v.Path)
		}
		if v.DispatchCapability.IsZero() {
			t.Errorf("view %q has empty dispatch_capability", v.Path)
		}
		if !v.IsStanding() {
			t.Errorf("view %q should be standing (RemainingExecutions = nil)", v.Path)
		}
	}
}

// TestContinuationClient_Inspect_RoundtripFields: install with a
// specific shape, inspect, verify every field made it through the
// decode.
func TestContinuationClient_Inspect_RoundtripFields(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	capHash := ap.OwnerCapability().ContentHash
	const path = "system/inbox/test/inspect/probe"

	cont := types.ContinuationData{
		Target:    "system/tree",
		Operation: "get",
		Resource:  &types.ResourceTarget{Targets: []string{"obs/x"}},
		DeliverTo: &types.DeliverySpec{
			URI:       "system/inbox/test/inspect/next",
			Operation: "receive",
		},
		OnError: &types.DeliverySpec{
			URI:       "system/inbox/test/inspect/err",
			Operation: "receive",
		},
		DispatchCapability: capHash,
	}
	contEnt, _ := cont.ToEntity()
	if _, err := ap.Continuation().Install(context.Background(), path, contEnt); err != nil {
		t.Fatalf("install: %v", err)
	}

	view, ok, err := ap.Continuation().Inspect(context.Background(), path)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if !ok {
		t.Fatal("inspect returned not found")
	}
	if view.Target != "system/tree" {
		t.Errorf("target = %q", view.Target)
	}
	if view.Operation != "get" {
		t.Errorf("operation = %q", view.Operation)
	}
	if view.DeliverTo == nil || view.DeliverTo.URI != "system/inbox/test/inspect/next" {
		t.Errorf("deliver_to = %+v", view.DeliverTo)
	}
	if view.OnError == nil || view.OnError.URI != "system/inbox/test/inspect/err" {
		t.Errorf("on_error = %+v", view.OnError)
	}
	if view.DispatchCapability != capHash {
		t.Errorf("dispatch_capability mismatch")
	}
}

// TestContinuationClient_Inspect_AbsentPath: inspect on a path with
// no entity returns (_, false, nil).
func TestContinuationClient_Inspect_AbsentPath(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	_, ok, err := ap.Continuation().Inspect(context.Background(), "system/inbox/nope/here")
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if ok {
		t.Errorf("expected absent, got found")
	}
}

// TestSubscriptionClient_ListAfterSubscribe: subscribe locally,
// verify the subscription entity shows up in List.
func TestSubscriptionClient_ListAfterSubscribe(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	sub, err := ap.Subscribe("local/data/*", entitysdk.SubscribeOpts{})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(func() { _ = sub.Close() })

	views, err := ap.Subscriptions().List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(views) == 0 {
		t.Fatal("expected at least one subscription, got 0")
	}
	var found bool
	for _, v := range views {
		if v.SubscriptionID == sub.ID() {
			found = true
			if v.DeliverOperation != "receive" {
				t.Errorf("deliver_operation = %q, want receive", v.DeliverOperation)
			}
			break
		}
	}
	if !found {
		t.Errorf("subscription %s not in list", sub.ID())
	}
}

func summaryStrings(views []entitysdk.ContinuationView) []string {
	out := make([]string, len(views))
	for i, v := range views {
		out[i] = v.Summary()
	}
	return out
}
