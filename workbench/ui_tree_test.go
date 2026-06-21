package workbench

import (
	"testing"

	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/store"
)

func makeEntries(paths ...string) []store.LocationEntry {
	entries := make([]store.LocationEntry, len(paths))
	for i, p := range paths {
		entries[i] = store.LocationEntry{Path: p, Hash: hash.Hash{}}
	}
	return entries
}

func TestBuildTree_Empty(t *testing.T) {
	root := BuildTree(nil)
	if len(root.Children) != 0 {
		t.Fatalf("expected 0 children, got %d", len(root.Children))
	}
}

func TestBuildTree_SingleEntry(t *testing.T) {
	root := BuildTree(makeEntries("test/hello"))
	if len(root.Children) != 1 || root.Children[0].Segment != "test" {
		t.Fatal("expected one child 'test'")
	}
	leaf := root.Children[0].Children[0]
	if leaf.Segment != "hello" || !leaf.HasEntry || leaf.FullPath != "test/hello" {
		t.Fatalf("unexpected leaf: %+v", leaf)
	}
}

func TestBuildTree_SharedPrefix(t *testing.T) {
	root := BuildTree(makeEntries("a/b/c", "a/b/d", "a/x"))
	a := root.Children[0]
	if a.Segment != "a" || len(a.Children) != 2 {
		t.Fatalf("expected 'a' with 2 children, got %q with %d", a.Segment, len(a.Children))
	}
	if a.Children[0].Segment != "b" || a.Children[1].Segment != "x" {
		t.Fatal("children should be sorted: b, x")
	}
}

func TestFlattenVisible_AllExpanded(t *testing.T) {
	root := BuildTree(makeEntries("a/b", "a/c"))
	ExpandToDepth(root, 10)
	rows := FlattenVisible(root)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
}

func TestFlattenVisible_Collapsed(t *testing.T) {
	root := BuildTree(makeEntries("a/b", "a/c"))
	root.Children[0].Expanded = false
	rows := FlattenVisible(root)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
}

func TestCountLeaves(t *testing.T) {
	root := BuildTree(makeEntries("a/b/c", "a/b/d", "a/x"))
	if c := CountLeaves(root.Children[0]); c != 3 {
		t.Fatalf("expected 3, got %d", c)
	}
}

func TestCollectAndRestoreExpanded(t *testing.T) {
	root := BuildTree(makeEntries("a/b/c", "a/d"))
	ExpandToDepth(root, 10)
	expanded := CollectExpanded(root)
	root2 := BuildTree(makeEntries("a/b/c", "a/d"))
	RestoreExpanded(root2, expanded)
	if !root2.Children[0].Expanded {
		t.Fatal("'a' should be expanded after restore")
	}
}
