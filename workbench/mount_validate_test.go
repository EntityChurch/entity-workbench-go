package workbench

import (
	"testing"

	"entity-workbench-go/entitysdk"
)

// TestValidateMountTarget_FreshIsClean — a target prefix with no
// existing bindings reports zero conflict; mount can proceed.
func TestValidateMountTarget_FreshIsClean(t *testing.T) {
	ap := mustNewPeer(t)
	defer ap.Close()

	res := ValidateMountTarget(ap, "local/files/x/", "archives/notes/", []string{MarkdownFileType})

	if res.TargetTotal != 0 {
		t.Errorf("TargetTotal = %d, want 0", res.TargetTotal)
	}
	if res.HasConflict() {
		t.Errorf("fresh prefix should not conflict; got foreign=%v", res.TargetForeign)
	}
	if res.SourceTotal != 0 {
		t.Errorf("SourceTotal = %d, want 0", res.SourceTotal)
	}
}

// TestValidateMountTarget_ExpectedTypeIsClean — a target prefix with
// pre-existing doc/markdown-file bindings (expected type) does not
// conflict; this is the remount-over-prior-content case.
func TestValidateMountTarget_ExpectedTypeIsClean(t *testing.T) {
	ap := mustNewPeer(t)
	defer ap.Close()

	md := MarkdownFileData{Path: "x.md", Title: "X"}
	if _, err := ap.Store().Put("archives/notes/x.md", MarkdownFileType, md); err != nil {
		t.Fatal(err)
	}

	res := ValidateMountTarget(ap, "local/files/x/", "archives/notes/", []string{MarkdownFileType})

	if res.TargetTotal != 1 {
		t.Errorf("TargetTotal = %d, want 1", res.TargetTotal)
	}
	if res.TargetExpected != 1 {
		t.Errorf("TargetExpected = %d, want 1", res.TargetExpected)
	}
	if res.HasConflict() {
		t.Errorf("expected-type bindings should not conflict; got %v", res.TargetForeign)
	}
}

// TestValidateMountTarget_ForeignTypeConflicts — a target prefix with
// entities of a different type reports conflict; operator must
// override with -force at the shell layer.
func TestValidateMountTarget_ForeignTypeConflicts(t *testing.T) {
	ap := mustNewPeer(t)
	defer ap.Close()

	if _, err := ap.Store().Put("archives/notes/legacy",
		"doc/text-file", map[string]interface{}{"body": "x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ap.Store().Put("archives/notes/other",
		"app/state/setting", map[string]interface{}{"v": 1}); err != nil {
		t.Fatal(err)
	}
	md := MarkdownFileData{Path: "y.md", Title: "Y"}
	if _, err := ap.Store().Put("archives/notes/y.md", MarkdownFileType, md); err != nil {
		t.Fatal(err)
	}

	res := ValidateMountTarget(ap, "local/files/x/", "archives/notes/", []string{MarkdownFileType})

	if res.TargetTotal != 3 {
		t.Errorf("TargetTotal = %d, want 3", res.TargetTotal)
	}
	if res.TargetExpected != 1 {
		t.Errorf("TargetExpected = %d, want 1 (the markdown-file)", res.TargetExpected)
	}
	if !res.HasConflict() {
		t.Fatalf("expected conflict with non-md types, got TargetForeign=%v", res.TargetForeign)
	}
	if res.TargetForeign["doc/text-file"] != 1 {
		t.Errorf("doc/text-file count = %d, want 1", res.TargetForeign["doc/text-file"])
	}
	if res.TargetForeign["app/state/setting"] != 1 {
		t.Errorf("app/state/setting count = %d, want 1", res.TargetForeign["app/state/setting"])
	}

	// Foreign-type ordering is stable + alphabetical.
	order := res.ForeignTypeOrder()
	if len(order) != 2 || order[0] != "app/state/setting" || order[1] != "doc/text-file" {
		t.Errorf("ForeignTypeOrder = %v, want [app/state/setting doc/text-file]", order)
	}
}

// TestValidateMountTarget_SourcePrefixCount — the source prefix
// count is reported separately; lets the shell verb tell the
// operator about prior-mount residue at local/files/{root}/.
func TestValidateMountTarget_SourcePrefixCount(t *testing.T) {
	ap := mustNewPeer(t)
	defer ap.Close()

	if _, err := ap.Store().Put("local/files/oldmount/a", "any/type",
		map[string]interface{}{"x": 1}); err != nil {
		t.Fatal(err)
	}

	res := ValidateMountTarget(ap, "local/files/oldmount/", "archives/notes/", []string{MarkdownFileType})

	if res.SourceTotal != 1 {
		t.Errorf("SourceTotal = %d, want 1", res.SourceTotal)
	}
}

func mustNewPeer(t *testing.T) *entitysdk.AppPeer {
	t.Helper()
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	return ap
}
