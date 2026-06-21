package entitysdk_test

import (
	"context"
	"testing"

	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

// TestRevisionClient_CommitLogStatusDiff exercises the manual-commit
// happy path: write data, commit, write more, commit again, then
// verify Log returns both versions newest-first, Status reports the
// head, and Diff between the two versions surfaces the change.
//
// Manual commits do not require a tracking config — that's only
// needed for the auto-versioner. This test runs entirely in
// "manual" mode.
func TestRevisionClient_CommitLogStatusDiff(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	ctx := context.Background()
	rc := ap.Revision()
	prefix := "workspace/"

	// Initial commit — single entity under the tracked prefix.
	if _, err := ap.Put("workspace/note", "test/v", "v1"); err != nil {
		t.Fatalf("first Put: %v", err)
	}
	first, err := rc.Commit(ctx, prefix, "initial commit")
	if err != nil {
		t.Fatalf("first Commit: %v", err)
	}
	if first.Version.IsZero() {
		t.Fatal("first commit returned zero version hash")
	}

	// Second commit after a write.
	if _, err := ap.Put("workspace/note", "test/v", "v2"); err != nil {
		t.Fatalf("second Put: %v", err)
	}
	second, err := rc.Commit(ctx, prefix, "second commit")
	if err != nil {
		t.Fatalf("second Commit: %v", err)
	}
	if second.Version == first.Version {
		t.Fatal("second commit returned same version as first — content didn't change?")
	}

	// Log: newest first.
	logResult, err := rc.Log(ctx, types.RevisionLogParamsData{Prefix: prefix})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(logResult.Versions) < 2 {
		t.Fatalf("expected ≥2 versions, got %d", len(logResult.Versions))
	}
	if logResult.Versions[0] != second.Version {
		t.Errorf("log[0] = %s, want second %s", logResult.Versions[0], second.Version)
	}
	if logResult.Versions[1] != first.Version {
		t.Errorf("log[1] = %s, want first %s", logResult.Versions[1], first.Version)
	}

	// Status: head should match second.
	status, err := rc.Status(ctx, prefix)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Head != second.Version {
		t.Errorf("status.Head = %s, want %s", status.Head, second.Version)
	}

	// Diff: first → second should report the changed path.
	diff, err := rc.Diff(ctx, prefix, first.Version, second.Version)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	totalChanges := len(diff.Added) + len(diff.Removed) + len(diff.Changed)
	if totalChanges == 0 {
		t.Errorf("diff reports no changes between v1 and v2")
	}
}

// TestRevisionClient_BranchTagFlow exercises Branch + Tag listing
// and creation against a fresh peer with one committed version.
// Confirms the action-routing helpers route correctly and the
// result envelopes round-trip.
func TestRevisionClient_BranchTagFlow(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	ctx := context.Background()
	rc := ap.Revision()
	prefix := "workspace/"

	if _, err := ap.Put("workspace/note", "test/v", "v1"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	commit, err := rc.Commit(ctx, prefix, "initial")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// BranchCreate with from=commit.Version.
	if _, err := rc.BranchCreate(ctx, prefix, "feature", commit.Version); err != nil {
		t.Fatalf("BranchCreate: %v", err)
	}

	// BranchList should now include "feature".
	branches, err := rc.BranchList(ctx, prefix)
	if err != nil {
		t.Fatalf("BranchList: %v", err)
	}
	if _, ok := branches.Branches["feature"]; !ok {
		t.Errorf("BranchList missing 'feature'; got %+v", branches.Branches)
	}

	// TagCreate at the commit.
	if _, err := rc.TagCreate(ctx, prefix, "v1.0", commit.Version); err != nil {
		t.Fatalf("TagCreate: %v", err)
	}
	tags, err := rc.TagList(ctx, prefix)
	if err != nil {
		t.Fatalf("TagList: %v", err)
	}
	if _, ok := tags.Tags["v1.0"]; !ok {
		t.Errorf("TagList missing 'v1.0'; got %+v", tags.Tags)
	}
}

// TestRevisionClient_FindAncestor confirms a linear two-commit
// chain has the first commit as the LCA of (first, second).
func TestRevisionClient_FindAncestor(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	ctx := context.Background()
	rc := ap.Revision()
	prefix := "workspace/"

	if _, err := ap.Put("workspace/x", "test/v", 1); err != nil {
		t.Fatal(err)
	}
	first, err := rc.Commit(ctx, prefix, "")
	if err != nil {
		t.Fatalf("first Commit: %v", err)
	}
	if _, err := ap.Put("workspace/x", "test/v", 2); err != nil {
		t.Fatal(err)
	}
	second, err := rc.Commit(ctx, prefix, "")
	if err != nil {
		t.Fatalf("second Commit: %v", err)
	}

	ancestor, err := rc.FindAncestor(ctx, first.Version, second.Version)
	if err != nil {
		t.Fatalf("FindAncestor: %v", err)
	}
	// In a linear chain first..second, first is ancestor of second.
	if ancestor != first.Version {
		t.Errorf("ancestor = %s, want %s", ancestor, first.Version)
	}
}
