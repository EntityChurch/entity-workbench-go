package entitysdk_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/types"
)

// TestRevisionPullOp_DirectDispatch exercises `revision:pull` as a
// handler op per EXTENSION-REVISION §4.4.8. The op folds the
// fetch + iterative fetch-entities trie walk + local merge into a
// single dispatchable operation, which is what makes DAG-mirror
// chain-expressible (single dynamic field via `base` or
// `since=$notification.previous_hash` in a future shape; for the
// direct-dispatch validation here we just call it imperatively).
//
// Setup: alice writes a 50-leaf workspace + commits. bob calls
// `revision:pull` against alice via direct cross-peer dispatch
// (constructing a fetch-params entity with `remote=aliceID`).
// Expected: bob's revision DAG advances to alice's head, AND bob's
// local tree under his own prefix mirrors all 50 leaves under
// `bob/deep/...` (DAG-advancing semantics, not just content mirror).
//
// This is the workbench-side validation that the op is implemented
// correctly on go. Cross-impl validation (rust, python) lives in
// `cross_impl_validate_ratified_test.go`.
func TestRevisionPullOp_DirectDispatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	alice, bob := setupPeerPair(t, ctx)
	const prefix = "deep/"
	aliceID := alice.PeerID()
	bobID := bob.PeerID()

	// Alice writes 50 leaves + commits.
	const numSubdirs = 5
	const leavesPerSub = 10
	const totalLeaves = numSubdirs * leavesPerSub
	for sub := 0; sub < numSubdirs; sub++ {
		for leaf := 0; leaf < leavesPerSub; leaf++ {
			path := fmt.Sprintf("deep/sub-%d/leaf-%02d", sub, leaf)
			body := fmt.Sprintf("alice's leaf at %s", path)
			if _, err := alice.Put(path, "test/note", body); err != nil {
				t.Fatalf("alice put %s: %v", path, err)
			}
		}
	}
	aliceCommit, err := alice.Revision().Commit(ctx, prefix, "initial")
	if err != nil {
		t.Fatalf("alice commit: %v", err)
	}
	t.Logf("alice head: %s", aliceCommit.Version)

	// Bob dispatches `system/revision:pull` LOCALLY (on himself) with
	// `Remote = aliceID` to tell the handler which peer to talk to.
	// The pull handler runs on bob, makes outbound EXECUTEs to alice
	// for fetch + iterative fetch-entities, then locally merges.
	pullParams := types.RevisionFetchParamsData{
		Prefix: prefix,
		Remote: aliceID,
	}
	paramEnt, err := pullParams.ToEntity()
	if err != nil {
		t.Fatalf("encode pull params: %v", err)
	}
	resp, err := bob.Executor().ExecuteWithParams("system/revision", "pull", paramEnt)
	if err != nil {
		t.Fatalf("revision:pull dispatch: %v", err)
	}
	if resp == nil || resp.Status >= 400 {
		t.Fatalf("revision:pull status=%d type=%s", statusOf(resp), resp.Type)
	}
	t.Logf("revision:pull returned status=%d type=%s", resp.Status, resp.Type)

	// Validation 1: bob's revision DAG advanced to alice's head at
	// bob's local prefix.
	bobStatus, err := bob.Revision().Status(ctx, prefix)
	if err != nil {
		t.Fatalf("bob status after pull: %v", err)
	}
	t.Logf("bob head after pull: %s", bobStatus.Head)
	if bobStatus.Head.IsZero() {
		t.Fatal("bob's local revision head is zero after pull — DAG did NOT advance")
	}
	// After a fast-forward pull from an empty DAG, bob's head equals
	// alice's commit version. (After a merge — when bob had local
	// commits — bob's head would be a NEW merge version. Here bob's
	// DAG was empty so it's fast-forward.)
	if bobStatus.Head != aliceCommit.Version {
		t.Fatalf("bob's head %s != alice's commit %s — pull did not fast-forward",
			bobStatus.Head, aliceCommit.Version)
	}

	// Validation 2: bob's local tree mirrors alice's leaves under
	// bob's own namespace. revision:merge applies the fetched closure
	// at bob's local prefix.
	mirrored := 0
	for sub := 0; sub < numSubdirs; sub++ {
		ents, _ := bob.List(fmt.Sprintf("/%s/deep/sub-%d/", bobID, sub))
		mirrored += len(ents)
	}
	t.Logf("bob mirrored %d/%d leaves under /%s/deep/sub-*/", mirrored, totalLeaves, bobID)
	if mirrored < totalLeaves {
		t.Fatalf("revision:pull did not materialize all leaves: %d/%d under bob's namespace",
			mirrored, totalLeaves)
	}
}

// TestRevisionPullOp_UpToDate validates that a second `revision:pull`
// against the same remote head is a no-op merge ("already_in_sync"),
// not an error. Important for idempotency under standing-chain
// repeat firings.
func TestRevisionPullOp_UpToDate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	alice, bob := setupPeerPair(t, ctx)
	const prefix = "deep/"
	aliceID := alice.PeerID()

	if _, err := alice.Put("deep/leaf", "test/note", "hello"); err != nil {
		t.Fatalf("alice put: %v", err)
	}
	if _, err := alice.Revision().Commit(ctx, prefix, "initial"); err != nil {
		t.Fatalf("alice commit: %v", err)
	}

	pullParams := types.RevisionFetchParamsData{Prefix: prefix, Remote: aliceID}
	paramEnt, _ := pullParams.ToEntity()

	// First pull — fast-forward.
	resp1, err := bob.Executor().ExecuteWithParams("system/revision", "pull", paramEnt)
	if err != nil {
		t.Fatalf("first pull: %v", err)
	}
	if resp1 == nil || resp1.Status >= 400 {
		t.Fatalf("first pull status=%d", statusOf(resp1))
	}

	// Second pull — should report already_in_sync, not error.
	resp2, err := bob.Executor().ExecuteWithParams("system/revision", "pull", paramEnt)
	if err != nil {
		t.Fatalf("second pull: %v", err)
	}
	if resp2 == nil || resp2.Status >= 400 {
		t.Fatalf("second pull status=%d type=%s", statusOf(resp2), resp2.Type)
	}
	mergeResult, err := types.RevisionMergeResultDataFromEntity(resp2.Entity())
	if err != nil {
		t.Fatalf("decode second merge result: %v", err)
	}
	if mergeResult.Status != "already_in_sync" && mergeResult.Status != "already_ahead" {
		t.Fatalf("expected already_in_sync/already_ahead on repeat pull, got %q",
			mergeResult.Status)
	}
	t.Logf("idempotency OK: second pull status=%q", mergeResult.Status)
}

// TestRevisionPullOp_MissingRemote checks the error path for an
// unparseable / missing remote peer-id.
func TestRevisionPullOp_MissingRemote(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, bob := setupPeerPair(t, ctx)

	pullParams := types.RevisionFetchParamsData{Prefix: "anything/"}
	paramEnt, _ := pullParams.ToEntity()
	// The SDK Executor maps >=400 responses to Go errors, so we
	// inspect the error rather than the response.
	_, err := bob.Executor().ExecuteWithParams("system/revision", "pull", paramEnt)
	if err == nil {
		t.Fatal("expected error for missing remote, got nil")
	}
	t.Logf("got expected error: %v", err)
}
