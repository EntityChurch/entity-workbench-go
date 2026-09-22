package workbench

import (
	"testing"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/hash"
)

// Tier: unit (TESTING-STRATEGY §1). Pure functions over a table; no
// store, no peer, no clock.

func TestClassifyDocPath_KindsAndTypes(t *testing.T) {
	cases := []struct {
		path     string
		kind     DocKind
		entity   string
		language string
		textual  bool
	}{
		// Markdown keeps its identity — this is the row that must never
		// move, because the entity type behind it is bound in trees that
		// already exist.
		{"notes.md", DocKindMarkdown, MarkdownFileType, "", true},
		{"NOTES.MD", DocKindMarkdown, MarkdownFileType, "", true},
		{"a/b/c.markdown", DocKindMarkdown, MarkdownFileType, "", true},

		{"readme.txt", DocKindText, TextFileType, "", true},
		{"server.log", DocKindText, TextFileType, "", true},
		{"rows.csv", DocKindText, TextFileType, "", true},

		{"main.go", DocKindCode, CodeFileType, "go", true},
		{"app.py", DocKindCode, CodeFileType, "python", true},
		{"config.yaml", DocKindCode, CodeFileType, "yaml", true},
		{"pkg.json", DocKindCode, CodeFileType, "json", true},

		{"photo.jpg", DocKindImage, ImageFileType, "", false},
		{"icon.PNG", DocKindImage, ImageFileType, "", false},
		{"chart.svg", DocKindImage, ImageFileType, "", false},

		{"paper.pdf", DocKindBinary, BinaryFileType, "", false},
		{"clip.mp4", DocKindBinary, BinaryFileType, "", false},

		// The fallthrough. An unknown extension is a CLASSIFICATION, not
		// a refusal — this is the row that replaced `type_not_handled`.
		{"data.qqq", DocKindBinary, BinaryFileType, "", false},
		{"noextension", DocKindBinary, BinaryFileType, "", false},

		// Basename table: the whole name is the type.
		{"Makefile", DocKindCode, CodeFileType, "make", true},
		{"src/Dockerfile", DocKindCode, CodeFileType, "dockerfile", true},
		{".gitignore", DocKindText, TextFileType, "", true},
		{"LICENSE", DocKindText, TextFileType, "", true},
		{"go.mod", DocKindCode, CodeFileType, "gomod", true},
	}
	for _, c := range cases {
		got := ClassifyDocPath(c.path)
		if got.Kind != c.kind || got.EntityType != c.entity ||
			got.Language != c.language || got.Textual != c.textual {
			t.Errorf("ClassifyDocPath(%q) = {kind:%s type:%s lang:%q textual:%v}, "+
				"want {kind:%s type:%s lang:%q textual:%v}",
				c.path, got.Kind, got.EntityType, got.Language, got.Textual,
				c.kind, c.entity, c.language, c.textual)
		}
	}
}

// TestClassifyDocPath_NeverEmpty — the registry has no "I don't know"
// answer. Every classification names an entity type, because the whole
// point of the change is that a file always becomes SOMETHING.
//
// Collects rather than Fatalf-ing on the first miss (AP15): the count a
// loop that stops at one failure yields is a lower bound.
func TestClassifyDocPath_NeverEmpty(t *testing.T) {
	var bad []string
	for _, ext := range SortedDocExtensions() {
		c := ClassifyDocPath("file" + ext)
		if c.EntityType == "" || c.Kind == "" {
			bad = append(bad, ext)
			continue
		}
		if !IsDocEntityType(c.EntityType) {
			bad = append(bad, ext+" (type not in DocEntityTypes)")
		}
	}
	if len(bad) > 0 {
		t.Fatalf("%d extension(s) classified to nothing usable: %v", len(bad), bad)
	}
}

// TestDocEntityTypes_CoversEveryKind — DocEntityTypes is what
// ValidateMountTarget uses to decide which bindings a mount owns. If the
// registry can WRITE a type that the set omits, a remount conflicts with
// its own predecessor's output. This is the gate for that.
func TestDocEntityTypes_CoversEveryKind(t *testing.T) {
	probes := []string{
		"a.md", "a.txt", "a.go", "a.png", "a.bin", "Makefile", "unknown.zzz",
	}
	for _, p := range probes {
		c := ClassifyDocPath(p)
		if !IsDocEntityType(c.EntityType) {
			t.Errorf("%s classifies to %s which DocEntityTypes() omits — "+
				"a mount would refuse to re-mount over its own output",
				p, c.EntityType)
		}
	}
}

// TestMarkdownEntityShapeUnchangedByRegistry — the claim doc_file_data.go
// makes in prose, pinned.
//
// A doc/markdown-file entity's bytes are its hash, which is what the tree
// binds and what a second peer compares. Adding kinds must not re-encode
// them. The invariant checked is the strong form: the encoded map has
// EXACTLY the four historic keys, so a future field added to the wrong
// struct fails here rather than silently re-hashing every document in
// every existing mount.
func TestMarkdownEntityShapeUnchangedByRegistry(t *testing.T) {
	ent, err := MarkdownFileData{
		Path:    "notes/a.md",
		Title:   "A",
		Content: hash.Hash{},
		Size:    12,
	}.ToEntity()
	if err != nil {
		t.Fatal(err)
	}
	if ent.Type != MarkdownFileType {
		t.Fatalf("type = %q, want %q", ent.Type, MarkdownFileType)
	}

	var decoded map[string]interface{}
	if err := ecf.Decode(ent.Data, &decoded); err != nil {
		t.Fatalf("decode markdown entity: %v", err)
	}
	want := map[string]bool{"path": true, "title": true, "content": true, "size": true}
	for k := range decoded {
		if !want[k] {
			t.Errorf("doc/markdown-file grew a field %q — every existing "+
				"markdown document's hash just changed", k)
		}
	}
	for k := range want {
		if _, ok := decoded[k]; !ok {
			t.Errorf("doc/markdown-file lost field %q", k)
		}
	}
}

// TestDocFileData_RoundTripsEveryNonMarkdownKind — one decode path has to
// work across a mixed listing, which is what the explorer walks.
func TestDocFileData_RoundTripsEveryNonMarkdownKind(t *testing.T) {
	for _, path := range []string{"a.txt", "a.go", "a.png", "a.bin"} {
		class := ClassifyDocPath(path)
		ent, err := DocFileData{
			Path:      path,
			Title:     "t",
			Size:      7,
			MediaType: class.MediaType,
			Language:  class.Language,
			Kind:      string(class.Kind),
		}.ToEntityOfType(class.EntityType)
		if err != nil {
			t.Fatalf("%s: build: %v", path, err)
		}
		back, err := DocFileDataFromEntity(ent)
		if err != nil {
			t.Fatalf("%s: decode: %v", path, err)
		}
		if back.Path != path || back.Size != 7 || back.Kind != string(class.Kind) {
			t.Errorf("%s: round-trip lost data: %+v", path, back)
		}
	}
}

// TestDocFileDataFromEntity_ReadsMarkdownToo — markdown predates the Kind
// field, so a decode of one has to fill it from the type rather than
// hand back an empty string a renderer would show as a blank column.
func TestDocFileDataFromEntity_ReadsMarkdownToo(t *testing.T) {
	ent, err := MarkdownFileData{Path: "a.md", Title: "A", Size: 3}.ToEntity()
	if err != nil {
		t.Fatal(err)
	}
	d, err := DocFileDataFromEntity(ent)
	if err != nil {
		t.Fatalf("decode markdown as DocFileData: %v", err)
	}
	if d.Kind != string(DocKindMarkdown) {
		t.Errorf("Kind = %q, want %q (backfilled from the entity type)",
			d.Kind, DocKindMarkdown)
	}
	if d.Path != "a.md" || d.Title != "A" {
		t.Errorf("markdown fields lost through the general decoder: %+v", d)
	}
}

// TestToEntityOfType_RefusesUnregisteredType — AP33: a tolerant path that
// writes a well-formed entity nobody dispatches on is a bug, not
// leniency.
func TestToEntityOfType_RefusesUnregisteredType(t *testing.T) {
	if _, err := (DocFileData{Path: "a"}).ToEntityOfType("doc/invented-file"); err == nil {
		t.Fatal("expected a refusal for a type outside the registry")
	}
}
