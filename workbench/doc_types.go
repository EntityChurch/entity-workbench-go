package workbench

import (
	"path/filepath"
	"sort"
	"strings"
)

// doc_types.go — the extension→entity-type registry that decides what an
// ingested file BECOMES.
//
// # Why this file exists
//
// Until now the answer was "a markdown file, or nothing". The ingest
// handler carried a single predicate — `isMarkdownPath` — and every file
// that failed it returned `skipped / type_not_handled` with no typed
// entity and no tree binding. The comment above that branch named this
// file as the intended replacement ("the long-term direction is a
// per-extension type registry ... until then we declare the markdown POC
// explicitly here"), and it stayed the POC for as long as it did because
// nothing surfaced the consequence: an operator who mounted a directory
// of anything else got a mount that reported a healthy file count and
// produced not one document they could open.
//
// **The skip was invisible at every surface**, which is the part worth
// keeping. The count on a mount row came from the *source* prefix, where
// the watcher writes a `local/files/file` for everything it admits, so a
// mount of 400 photographs and one README read as "401 entities in tree"
// and had exactly one openable document in it. A number that goes up is
// the most reassuring thing a panel can show, and it was measuring the
// layer that never fails.
//
// # What a class is for
//
// Two consumers, and they want different halves:
//
//   - The ingest handler needs the ENTITY TYPE, so the file lands in the
//     tree as something a viewer can dispatch on, and Textual, so it
//     knows whether peeking the first chunk for a title is meaningful or
//     is about to read a JPEG looking for a `# heading`.
//   - The explorer needs Kind, Language and MediaType, which are what a
//     human reads off a row.
//
// # The deliberate limits
//
// Extension-only, with a small basename table for the extensionless
// files that are unambiguous by name (Makefile, Dockerfile). We do NOT
// sniff content. Sniffing means reading the first chunk of every file to
// classify it, which is the cost the markdown path pays only because it
// needs the bytes anyway for a title — paying it for a 4 GB video to
// learn what its name already said is a bad trade. A misclassified file
// is a wrong label on a row that still opens; an unreadable mount is not.
//
// And every file gets a class. There is no "unhandled" outcome any more:
// the fallthrough is DocKindBinary, which is a real classification with a
// real entity type, not a refusal wearing one. That is the whole point —
// `doc/binary-file` says "this is in your tree, addressable, and we are
// not going to pretend to render it", which is a fact an operator can act
// on. `type_not_handled` was a fact only we could act on, and we didn't.

// DocKind is the coarse family a file belongs to. It is what a renderer
// groups and icons by; EntityType is what the tree dispatches on.
type DocKind string

const (
	// DocKindMarkdown is markdown proper — the only kind with a title
	// extracted from a heading rather than from the filename.
	DocKindMarkdown DocKind = "markdown"
	// DocKindText is plain prose text with no structure we parse.
	DocKindText DocKind = "text"
	// DocKindCode is source and structured-data text (JSON, YAML, TOML
	// and friends live here — they are text a person edits, and the
	// distinction from source code buys a renderer nothing).
	DocKindCode DocKind = "code"
	// DocKindImage is a raster or vector image.
	DocKindImage DocKind = "image"
	// DocKindBinary is the honest fallthrough: bytes in the tree that we
	// decline to interpret.
	DocKindBinary DocKind = "binary"
)

// Entity types produced by the registry. MarkdownFileType is declared in
// ingest_tree.go and predates this file; it is listed here so the set is
// enumerable in one place.
const (
	TextFileType   = "doc/text-file"
	CodeFileType   = "doc/code-file"
	ImageFileType  = "doc/image-file"
	BinaryFileType = "doc/binary-file"
)

// DocClass is the registry's answer for one path.
type DocClass struct {
	// Kind is the coarse family.
	Kind DocKind
	// EntityType is the tree type the ingested document is written as.
	EntityType string
	// Language is a syntax hint for code ("go", "python", "yaml"), empty
	// for every other kind. Advisory: no consumer may depend on a
	// particular spelling, because the table is ours and will grow.
	Language string
	// MediaType is a best-effort IANA type, empty when we do not know
	// one. The watcher's own MediaType wins over this when it has one —
	// it saw the file, we saw its name.
	MediaType string
	// Textual reports whether the bytes are UTF-8 text that a viewer can
	// put on screen and a title can be read out of. False for image and
	// binary, which is what stops the ingest handler peeking into them.
	Textual bool
}

// IsMarkdown reports whether this class is markdown, which is the one
// kind with behavior of its own (heading-derived titles, and the
// MarkdownFileData shape that predates the registry).
func (c DocClass) IsMarkdown() bool { return c.Kind == DocKindMarkdown }

// DocEntityTypes returns every entity type the registry can produce,
// sorted, for callers that need the whole set rather than one answer.
//
// The caller this exists for is ValidateMountTarget: a mount refuses to
// land on a prefix that already holds bindings of a type it does not
// own, and before the registry that set was the single element
// {doc/markdown-file}. Left alone, the first mount of a mixed directory
// would have conflicted with the documents its own predecessor wrote.
func DocEntityTypes() []string {
	return []string{
		BinaryFileType,
		CodeFileType,
		ImageFileType,
		MarkdownFileType,
		TextFileType,
	}
}

// IsDocEntityType reports whether t is one of the registry's types.
func IsDocEntityType(t string) bool {
	for _, known := range DocEntityTypes() {
		if t == known {
			return true
		}
	}
	return false
}

// ClassifyDocPath classifies a file path. Never fails: an unknown
// extension is DocKindBinary, which is a classification and not an error.
//
// path may be a bare filename, a relative path or an absolute one; only
// the basename and its extension are read.
func ClassifyDocPath(path string) DocClass {
	base := filepath.Base(filepath.ToSlash(path))

	// Basename table first — an extensionless file whose NAME is its
	// type. Checked before the extension split because "Makefile" has no
	// extension and ".gitignore" has nothing but one.
	if c, ok := docByBasename[strings.ToLower(base)]; ok {
		return c
	}

	ext := strings.ToLower(filepath.Ext(base))
	if c, ok := docByExt[ext]; ok {
		return c
	}
	return DocClass{Kind: DocKindBinary, EntityType: BinaryFileType}
}

// ClassifyDocKinds returns the kind counts for a set of paths. Used by
// the explorer's summary line, which reports what a mount is made of
// rather than a single total — "412 files" and "412 files: 400 image, 11
// code, 1 markdown" answer different questions, and only the second one
// tells an operator why nothing opens.
func ClassifyDocKinds(paths []string) map[DocKind]int {
	out := make(map[DocKind]int, 5)
	for _, p := range paths {
		out[ClassifyDocPath(p).Kind]++
	}
	return out
}

// DocKindOrder is the stable presentation order for kind tallies, most
// interpretable first. Renderers use it so two panels showing the same
// mount do not order the same counts differently.
func DocKindOrder() []DocKind {
	return []DocKind{DocKindMarkdown, DocKindText, DocKindCode, DocKindImage, DocKindBinary}
}

func text(media string) DocClass {
	return DocClass{Kind: DocKindText, EntityType: TextFileType, MediaType: media, Textual: true}
}

func code(lang, media string) DocClass {
	return DocClass{Kind: DocKindCode, EntityType: CodeFileType, Language: lang, MediaType: media, Textual: true}
}

func image(media string) DocClass {
	return DocClass{Kind: DocKindImage, EntityType: ImageFileType, MediaType: media}
}

func binary(media string) DocClass {
	return DocClass{Kind: DocKindBinary, EntityType: BinaryFileType, MediaType: media}
}

// markdownClass is the one kind that predates the registry. Its entity
// type and on-tree shape are unchanged by this file existing.
var markdownClass = DocClass{
	Kind:       DocKindMarkdown,
	EntityType: MarkdownFileType,
	MediaType:  "text/markdown",
	Textual:    true,
}

// docByExt maps a lowercased extension (with its dot) to a class.
//
// Not exhaustive and not trying to be — an unknown extension has a
// correct answer (binary) rather than a wrong one, so the cost of an
// omission is a coarser row, not a broken mount. Add entries when a real
// mount surfaces one.
var docByExt = map[string]DocClass{
	// --- markdown ---
	".md":       markdownClass,
	".markdown": markdownClass,

	// --- prose text ---
	".txt":  text("text/plain"),
	".text": text("text/plain"),
	".log":  text("text/plain"),
	".rst":  text("text/x-rst"),
	".org":  text("text/plain"),
	".adoc": text("text/plain"),
	".tex":  text("text/x-tex"),
	".csv":  text("text/csv"),
	".tsv":  text("text/tab-separated-values"),

	// --- code: languages ---
	".go":    code("go", "text/x-go"),
	".rs":    code("rust", "text/x-rust"),
	".c":     code("c", "text/x-c"),
	".h":     code("c", "text/x-c"),
	".cc":    code("cpp", "text/x-c++"),
	".cpp":   code("cpp", "text/x-c++"),
	".hpp":   code("cpp", "text/x-c++"),
	".cs":    code("csharp", "text/plain"),
	".java":  code("java", "text/x-java"),
	".kt":    code("kotlin", "text/plain"),
	".swift": code("swift", "text/plain"),
	".py":    code("python", "text/x-python"),
	".rb":    code("ruby", "text/x-ruby"),
	".pl":    code("perl", "text/x-perl"),
	".lua":   code("lua", "text/x-lua"),
	".php":   code("php", "text/x-php"),
	".js":    code("javascript", "text/javascript"),
	".mjs":   code("javascript", "text/javascript"),
	".cjs":   code("javascript", "text/javascript"),
	".ts":    code("typescript", "text/plain"),
	".tsx":   code("typescript", "text/plain"),
	".jsx":   code("javascript", "text/plain"),
	".sh":    code("shell", "text/x-shellscript"),
	".bash":  code("shell", "text/x-shellscript"),
	".zsh":   code("shell", "text/x-shellscript"),
	".fish":  code("shell", "text/x-shellscript"),
	".ps1":   code("powershell", "text/plain"),
	".sql":   code("sql", "text/x-sql"),
	".r":     code("r", "text/plain"),
	".scm":   code("scheme", "text/plain"),
	".el":    code("elisp", "text/plain"),
	".hs":    code("haskell", "text/plain"),
	".ml":    code("ocaml", "text/plain"),
	".ex":    code("elixir", "text/plain"),
	".exs":   code("elixir", "text/plain"),
	".erl":   code("erlang", "text/plain"),
	".zig":   code("zig", "text/plain"),
	".proto": code("protobuf", "text/plain"),

	// --- code: structured data and markup ---
	".json": code("json", "application/json"),
	".yaml": code("yaml", "application/yaml"),
	".yml":  code("yaml", "application/yaml"),
	".toml": code("toml", "application/toml"),
	".ini":  code("ini", "text/plain"),
	".cfg":  code("ini", "text/plain"),
	".conf": code("ini", "text/plain"),
	".xml":  code("xml", "application/xml"),
	".html": code("html", "text/html"),
	".htm":  code("html", "text/html"),
	".css":  code("css", "text/css"),
	".scss": code("scss", "text/css"),
	".svg":  image("image/svg+xml"),

	// --- images ---
	".png":  image("image/png"),
	".jpg":  image("image/jpeg"),
	".jpeg": image("image/jpeg"),
	".gif":  image("image/gif"),
	".webp": image("image/webp"),
	".bmp":  image("image/bmp"),
	".ico":  image("image/vnd.microsoft.icon"),
	".tif":  image("image/tiff"),
	".tiff": image("image/tiff"),
	".avif": image("image/avif"),
	".heic": image("image/heic"),

	// --- named binaries: a known type is worth more than a bare
	//     "binary", because it is the difference between "we do not
	//     render this" and "we do not know what this is". ---
	".pdf":    binary("application/pdf"),
	".zip":    binary("application/zip"),
	".gz":     binary("application/gzip"),
	".bz2":    binary("application/x-bzip2"),
	".xz":     binary("application/x-xz"),
	".zst":    binary("application/zstd"),
	".tar":    binary("application/x-tar"),
	".7z":     binary("application/x-7z-compressed"),
	".mp3":    binary("audio/mpeg"),
	".flac":   binary("audio/flac"),
	".wav":    binary("audio/wav"),
	".ogg":    binary("audio/ogg"),
	".m4a":    binary("audio/mp4"),
	".mp4":    binary("video/mp4"),
	".mkv":    binary("video/x-matroska"),
	".mov":    binary("video/quicktime"),
	".webm":   binary("video/webm"),
	".ttf":    binary("font/ttf"),
	".otf":    binary("font/otf"),
	".woff":   binary("font/woff"),
	".so":     binary("application/x-sharedlib"),
	".a":      binary("application/x-archive"),
	".o":      binary("application/x-object"),
	".exe":    binary("application/vnd.microsoft.portable-executable"),
	".dll":    binary("application/x-msdownload"),
	".wasm":   binary("application/wasm"),
	".db":     binary("application/vnd.sqlite3"),
	".sqlite": binary("application/vnd.sqlite3"),
}

// docByBasename covers files whose whole name is their type. Keyed
// lowercased; matched before the extension split.
var docByBasename = map[string]DocClass{
	"makefile":       code("make", "text/x-makefile"),
	"gnumakefile":    code("make", "text/x-makefile"),
	"dockerfile":     code("dockerfile", "text/plain"),
	"containerfile":  code("dockerfile", "text/plain"),
	"jenkinsfile":    code("groovy", "text/plain"),
	"vagrantfile":    code("ruby", "text/plain"),
	"rakefile":       code("ruby", "text/plain"),
	"gemfile":        code("ruby", "text/plain"),
	"procfile":       code("yaml", "text/plain"),
	".gitignore":     text("text/plain"),
	".gitattributes": text("text/plain"),
	".dockerignore":  text("text/plain"),
	".editorconfig":  code("ini", "text/plain"),
	".env":           code("ini", "text/plain"),
	"license":        text("text/plain"),
	"licence":        text("text/plain"),
	"copying":        text("text/plain"),
	"notice":         text("text/plain"),
	"authors":        text("text/plain"),
	"codeowners":     text("text/plain"),
	"go.mod":         code("gomod", "text/plain"),
	"go.sum":         text("text/plain"),
}

// SortedDocExtensions lists every registered extension, sorted. Exists
// for the gate test that pins the registry's shape, and for a `doctor`
// style surface that wants to print what we know.
func SortedDocExtensions() []string {
	out := make([]string, 0, len(docByExt))
	for ext := range docByExt {
		out = append(out, ext)
	}
	sort.Strings(out)
	return out
}
