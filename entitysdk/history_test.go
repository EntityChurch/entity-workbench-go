package entitysdk_test

import (
	"context"
	"testing"

	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

// TestHistoryClient_QueryRecordsTransitions exercises the happy path:
// install a history config matching workspace/*, drive two writes at
// the same path, verify the history query surfaces both transitions
// (newest first), and rollback to the first hash to confirm the path
// rebinds.
//
// Recording is opt-in: the recorder consults a HistoryConfigData at
// system/history/config/{name} to decide which paths to track.
// Without a matching config the recorder no-ops, which is also why
// the recorder runs default-on without flooding the tree.
func TestHistoryClient_QueryRecordsTransitions(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	ctx := context.Background()

	// Install a history config matching the path under test. The
	// recorder's config cache hot-reloads when system/history/config/*
	// paths change, so subsequent writes will be recorded.
	cfg := types.HistoryConfigData{Pattern: "workspace/*", Enabled: true}
	cfgEnt, err := cfg.ToEntity()
	if err != nil {
		t.Fatalf("config ToEntity: %v", err)
	}
	if _, err := ap.Store().Put("system/history/config/test", cfgEnt.Type, cfg); err != nil {
		t.Fatalf("install history config: %v", err)
	}

	// Two L1 writes at the same path — created, then updated.
	path := "workspace/note"
	first, err := ap.Put(path, "test/v", "v1")
	if err != nil {
		t.Fatalf("first Put: %v", err)
	}
	second, err := ap.Put(path, "test/v", "v2")
	if err != nil {
		t.Fatalf("second Put: %v", err)
	}
	if first == second {
		t.Fatalf("v1 and v2 produced the same hash; test fixture is broken")
	}

	// Query history. Path is bare; the handler canonicalizes against
	// the local peer's id.
	result, err := ap.History().Query(ctx, types.HistoryQueryParamsData{Path: path})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(result.Transitions) < 2 {
		t.Fatalf("expected at least 2 transitions, got %d", len(result.Transitions))
	}

	// Newest first: head should reference v2; second-most-recent v1.
	// Walk ordering is "newest → oldest" per handler doc.
	if result.Transitions[0].Hash != second {
		t.Errorf("transition[0].Hash = %s, want v2 hash %s",
			result.Transitions[0].Hash, second)
	}
	if result.Transitions[1].Hash != first {
		t.Errorf("transition[1].Hash = %s, want v1 hash %s",
			result.Transitions[1].Hash, first)
	}

	// Rollback to v1.
	rb, err := ap.History().Rollback(ctx, path, first)
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if rb.Restored != first {
		t.Errorf("Restored = %s, want v1 hash %s", rb.Restored, first)
	}

	// The rollback rebinds path to first via the normal tree path —
	// confirm the resolved entity is back at v1.
	got, ok, err := ap.Get(path)
	if err != nil {
		t.Fatalf("Get after rollback: %v", err)
	}
	if !ok {
		t.Fatalf("path missing after rollback")
	}
	if got.ContentHash != first {
		t.Errorf("after rollback path resolves to %s, want %s", got.ContentHash, first)
	}
}

// TestHistoryClient_QueryEmptyPath verifies query on a path with no
// recorded history returns an empty transition list (not an error).
func TestHistoryClient_QueryEmptyPath(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	result, err := ap.History().Query(context.Background(),
		types.HistoryQueryParamsData{Path: "never/written"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(result.Transitions) != 0 {
		t.Errorf("transitions = %d, want 0", len(result.Transitions))
	}
}

// TestHistoryClient_RollbackUnknownHash verifies the handler rejects a
// rollback to a hash that doesn't appear in the path's history with a
// 404 not_in_history.
func TestHistoryClient_RollbackUnknownHash(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	// Put something at one path, then try to rollback a different path
	// to that hash.
	target, err := ap.Put("a", "test/v", 1)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	_, err = ap.History().Rollback(context.Background(), "b", target)
	if err == nil {
		t.Fatal("expected error for rollback to hash not in path's history")
	}
	if !entitysdk.IsNotFound(err) {
		t.Errorf("expected 404 not-found, got %v", err)
	}
}
