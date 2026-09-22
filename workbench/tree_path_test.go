package workbench

import (
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/ext/localfiles"
)

// tree_path_test.go — the gate for the qualified-path defect.
//
// Tier: model, and deliberately on a **peer-backed** store. Every test in
// this package that uses `NewStore(memory, memory)` runs without a
// NamespacedIndex, where `List` returns bare relative paths and the
// prefix arithmetic these tests exercise cannot fail. That is precisely
// how the defect shipped: the fixture omitted a wrapper the production
// object always has, and the wrapper changes the return-value shape of
// the most-used read in the codebase.
//
// **Anything asserting on a path that came out of the store belongs
// here, not beside its model** — the point of the file is that the store
// is real.

func TestTreeRelative(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/2K64hzCLexvM/system/config/local/files/notes", "system/config/local/files/notes"},
		{"/p/a/b/c", "a/b/c"},
		{"/p/", ""},
		// Already relative — unchanged, which is what makes the helper
		// safe to apply without knowing where a path came from.
		{"system/config/local/files/notes", "system/config/local/files/notes"},
		{"", ""},
		// One segment is not a namespaced path; emptying it would turn a
		// real path into nothing.
		{"/onlysegment", "/onlysegment"},
	}
	for _, c := range cases {
		if got := TreeRelative(c.in); got != c.want {
			t.Errorf("TreeRelative(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRelativeUnder(t *testing.T) {
	cases := []struct {
		path, prefix, want string
		under              bool
	}{
		{"/p/docs/a.md", "docs/", "a.md", true},
		{"docs/a.md", "docs/", "a.md", true},
		{"/p/other/a.md", "docs/", "", false},
		{"/p/docs/a.md", "", "docs/a.md", true},
	}
	for _, c := range cases {
		got, under := RelativeUnder(c.path, c.prefix)
		if got != c.want || under != c.under {
			t.Errorf("RelativeUnder(%q, %q) = (%q, %v), want (%q, %v)",
				c.path, c.prefix, got, under, c.want, c.under)
		}
	}
	// The trap a bare TrimPrefix falls into: a non-matching prefix
	// returns the whole string, which reads as a valid relative path.
	if got, under := RelativeUnder("/p/other/a.md", "docs/"); under || got != "" {
		t.Errorf("a path outside the prefix must report not-under, got (%q, %v)", got, under)
	}
}

// TestLocalFilesModel_SeesMountsOnAPeerBackedStore — the regression gate.
//
// Before the fix this returned ZERO mounts for a peer that had one, and
// the Local Files panel rendered "no filesystem mounts on this peer".
// The mount had succeeded, written its config and started its watcher;
// the only surface built to show it filtered it out on a string test
// against a path shape it did not expect.
func TestLocalFilesModel_SeesMountsOnAPeerBackedStore(t *testing.T) {
	st := liveStore(t)
	if _, err := st.Put(MountConfigPrefix+"notes", localfiles.TypeRootConfig,
		localfiles.RootConfigData{
			Prefix:         "local/files/notes/",
			FilesystemRoot: "/tmp/notes",
		}); err != nil {
		t.Fatal(err)
	}

	// State the premise the whole file rests on, so a future reader does
	// not have to take it on trust: List returns QUALIFIED paths.
	entries := st.List(MountConfigPrefix)
	if len(entries) != 1 {
		t.Fatalf("List returned %d entries, want 1", len(entries))
	}
	if !strings.HasPrefix(entries[0].Path, "/") {
		t.Fatalf("premise broken: List returned a relative path %q — if the store "+
			"stopped namespacing, TreeRelative is now dead code rather than a fix",
			entries[0].Path)
	}

	out := NewLocalFilesModel(st).Render()
	if len(out.Mounts) != 1 {
		t.Fatalf("LocalFilesModel reports %d mounts on a peer-backed store, want 1 "+
			"(note=%q)", len(out.Mounts), out.Note)
	}
	if out.Mounts[0].Root != "notes" {
		t.Errorf("Root = %q, want %q — a mangled root is not a name unmount accepts",
			out.Mounts[0].Root, "notes")
	}
	if out.Mounts[0].FilesystemRoot != "/tmp/notes" {
		t.Errorf("FilesystemRoot = %q", out.Mounts[0].FilesystemRoot)
	}
}

// TestMountBindings_RoundTripOnAPeerBackedStore — the durable half of a
// mount reads back with a usable root name, which is what
// RestoreMountBindings routes on at startup.
func TestMountBindings_RoundTripOnAPeerBackedStore(t *testing.T) {
	st := liveStore(t)
	if err := SaveMountBinding(st, MountBindingData{
		Root:         "notes",
		SourcePrefix: "local/files/notes/",
		TargetPrefix: "archives/notes/",
	}); err != nil {
		t.Fatal(err)
	}

	one, ok := LoadMountBinding(st, "notes")
	if !ok || one.TargetPrefix != "archives/notes/" {
		t.Fatalf("LoadMountBinding = (%+v, %v)", one, ok)
	}

	all, problems := LoadMountBindings(st)
	if len(problems) != 0 {
		t.Fatalf("problems: %v", problems)
	}
	if len(all) != 1 || all[0].Root != "notes" {
		t.Fatalf("LoadMountBindings = %+v, want one row rooted at notes", all)
	}

	ingest := NewNotificationIngestHandler(nil)
	restored, problems := RestoreMountBindings(st, ingest)
	if restored != 1 || len(problems) != 0 {
		t.Fatalf("RestoreMountBindings = (%d, %v), want (1, none)", restored, problems)
	}
	// The restore is only meaningful if the handler can now ROUTE — a
	// registration nobody can dispatch against is the same silence it
	// replaced.
	if got := ingest.LookupMount("local/files/notes/"); got != "archives/notes/" {
		t.Errorf("after restore, LookupMount = %q, want the target prefix", got)
	}

	if !RemoveMountBinding(st, "notes") {
		t.Error("RemoveMountBinding reported nothing to remove")
	}
	if _, ok := LoadMountBinding(st, "notes"); ok {
		t.Error("binding survived removal — an unmount would undo itself at the next restart")
	}
}
