package workbench

import "testing"

func TestFilterCommands_Empty(t *testing.T) {
	Init()
	results := FilterCommands("")
	if len(results) != len(Registry) {
		t.Fatalf("expected all %d, got %d", len(Registry), len(results))
	}
}

func TestFilterCommands_Match(t *testing.T) {
	Init()
	results := FilterCommands("tree")
	found := false
	for _, idx := range results {
		if Registry[idx].Name == "new-tree-browser" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected 'new-tree-browser' in results")
	}
}

func TestFilterCommands_NoMatch(t *testing.T) {
	Init()
	results := FilterCommands("zzzzz")
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}
