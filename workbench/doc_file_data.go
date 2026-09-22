package workbench

import (
	"fmt"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"

	"github.com/fxamacker/cbor/v2"
)

// doc_file_data.go — the typed payload for every non-markdown document
// the ingest registry produces.
//
// # Why this is a second struct and not a widened MarkdownFileData
//
// It very nearly was. The reason it is not comes down to one property
// worth protecting: **the bytes of an existing `doc/markdown-file` entity
// do not change because this file exists.** A markdown document's hash is
// its identity — it is what the tree binds, what a revision history is
// keyed on, and what a second peer will compare against when M2 lands.
// Widening the markdown struct with three omitempty fields would almost
// certainly have preserved those bytes (canonical CBOR omits an empty
// omitempty field, so the encoded map would be identical), and "almost
// certainly" is the wrong confidence level for a change that silently
// re-hashes every document in every existing mount if it is wrong.
//
// So markdown keeps its struct, byte-for-byte, and the new kinds get
// this one. `TestMarkdownEntityBytesUnchangedByRegistry` pins that claim
// rather than leaving it as the paragraph you are reading.
//
// # The shape
//
// DocFileData is a superset of MarkdownFileData's four fields, which is
// what lets [DocFileDataFromEntity] decode *any* registry type including
// markdown — the explorer needs one decode path across a directory
// listing that mixes kinds, and giving it a type switch over five structs
// would have been five places to forget a field.
type DocFileData struct {
	// Path is the file's path relative to its mount root, as the
	// watcher recorded it. Forward-slashed on the wire regardless of
	// host OS.
	Path string `cbor:"path"`
	// Title is a display label. For textual kinds with no better
	// source, and for every non-textual kind, this is the filename
	// without its extension — never a guess derived from the bytes.
	Title string `cbor:"title"`
	// Content is a hash into system/content/blob. The entity carries
	// the reference; the bytes live in the content substrate, so a 4 GB
	// video ingests without materializing anywhere.
	Content hash.Hash `cbor:"content"`
	// Size is the payload size in bytes, mirrored from the source
	// FileData so a listing can show sizes without touching the blob.
	Size int64 `cbor:"size"`
	// MediaType is a best-effort IANA type. Sourced from the watcher's
	// own MediaType when it set one, else from the registry's table,
	// else absent — never invented.
	MediaType string `cbor:"media_type,omitempty"`
	// Language is a syntax hint for code kinds, absent otherwise.
	// Advisory: the spellings are ours and the table will grow, so no
	// consumer may branch on an exact value in a way that breaks when
	// one is added.
	Language string `cbor:"language,omitempty"`
	// Kind is the coarse family ("text", "code", "image", "binary").
	// Denormalized from EntityType on purpose: a renderer walking a
	// mixed listing groups and icons by kind, and deriving it back out
	// of the entity type at every row is a mapping maintained in the
	// renderer — which is exactly what workbench exists to prevent.
	Kind string `cbor:"kind,omitempty"`
}

// ToEntityOfType encodes the payload as an entity of the given registry
// type.
//
// The type is a parameter rather than derived from Kind because the
// caller has already classified the path and holds a DocClass; deriving
// it again here would be a second mapping to keep in step with the
// first. Refuses a type the registry does not produce — writing a
// `doc/*` entity of an unregistered type would put a row in the tree
// that no viewer dispatches on and no mount validation expects, which is
// AP33's shape (a tolerant path that produces a well-formed thing nobody
// can consume).
func (d DocFileData) ToEntityOfType(entityType string) (entity.Entity, error) {
	if !IsDocEntityType(entityType) {
		return entity.Entity{}, fmt.Errorf("not a document entity type: %q", entityType)
	}
	raw, err := ecf.Encode(d)
	if err != nil {
		return entity.Entity{}, fmt.Errorf("encode %s: %w", entityType, err)
	}
	return entity.NewEntity(entityType, cbor.RawMessage(raw))
}

// DocFileDataFromEntity decodes any registry-typed document entity,
// markdown included.
//
// Rejects anything outside the registry. A caller holding an arbitrary
// entity should check [IsDocEntityType] first rather than relying on the
// error text.
func DocFileDataFromEntity(ent entity.Entity) (DocFileData, error) {
	if !IsDocEntityType(ent.Type) {
		return DocFileData{}, fmt.Errorf("expected a document entity type, got %s", ent.Type)
	}
	var d DocFileData
	if err := ecf.Decode(ent.Data, &d); err != nil {
		return DocFileData{}, fmt.Errorf("decode %s: %w", ent.Type, err)
	}
	// A markdown entity predates the Kind field and will not carry one.
	// Fill it from the type so a consumer never has to know which
	// vintage it is holding.
	if d.Kind == "" {
		d.Kind = string(kindForEntityType(ent.Type))
	}
	return d, nil
}

// kindForEntityType maps a registry entity type back to its kind. The
// inverse direction is DocClass.EntityType; this one exists only for
// entities written before Kind was a field.
func kindForEntityType(t string) DocKind {
	switch t {
	case MarkdownFileType:
		return DocKindMarkdown
	case TextFileType:
		return DocKindText
	case CodeFileType:
		return DocKindCode
	case ImageFileType:
		return DocKindImage
	default:
		return DocKindBinary
	}
}
