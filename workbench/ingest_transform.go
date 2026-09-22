package workbench

import (
	"context"
	"path/filepath"
	"strings"

	"go.entitychurch.org/entity-core-go/core/handler"
	"go.entitychurch.org/entity-core-go/ext/localfiles"
)

// IngestTransformPattern is the URI pattern under which the
// ingest-transform handler registers. Chain step 2 in the
// Phase E watch-driven ingest chain dispatches against this
// to convert a `local/files/file` entity into a
// `doc/markdown-file` entity.
//
// The handler is workbench-internal — it implements the
// transform half of the watch-chain shape from
// PHASE-E-LOCAL-FILES-PLAN §4.2. It has no spec-level URI;
// the workbench owns this namespace.
const IngestTransformPattern = "workbench/ingest-transform"

// IngestTransformHandler converts a localfiles.FileData entity
// (`local/files/file`) into a `doc/markdown-file` entity in the
// shape produced by IngestMarkdownTree. Stateless; the chain
// composition supplies the source entity in EXECUTE params and
// routes the result entity downstream via `deliver_to`.
//
// Classification goes through the same registry the notification
// path uses (doc_types.go), so the two ingest routes cannot answer
// "what does this file become" differently. That was a live risk
// rather than a theoretical one: this handler used to declare that
// non-markdown content "is passed through with an empty title and
// the raw Content as the body", producing a doc/markdown-file for
// a PNG, while the notification path refused the same file. Two
// paths, two answers, and only one of them wired to a panel.
type IngestTransformHandler struct{}

// NewIngestTransformHandler returns the singleton transform
// handler. Pure function; no construction-time state.
func NewIngestTransformHandler() *IngestTransformHandler {
	return &IngestTransformHandler{}
}

func (h *IngestTransformHandler) Name() string { return IngestTransformPattern }

// Handle decodes the params as a localfiles.FileData and emits a
// doc/markdown-file entity. The params entity MUST be of type
// `local/files/file`; any other type is rejected with 400.
//
// Op name is ignored — this handler has a single behavior. We
// don't dispatch on the op for future flexibility (a "transform"
// is one shape; "validate" or "preview" would be additional ops
// when there's a real need).
func (h *IngestTransformHandler) Handle(_ context.Context, req *handler.Request) (*handler.Response, error) {
	if req.Params.Type != localfiles.TypeFile {
		return handler.NewErrorResponse(400, "invalid_input_type",
			"ingest-transform expects "+localfiles.TypeFile+", got "+req.Params.Type)
	}
	file, err := localfiles.FileDataFromEntity(req.Params)
	if err != nil {
		return handler.NewErrorResponse(400, "invalid_input",
			"failed to decode local/files/file: "+err.Error())
	}

	// DOMAIN-LOCAL-FILES v1.3 §2.1 — file.Content is a hash into
	// system/content/blob. The transform passes that hash through into
	// the typed document entity instead of reassembling the payload
	// here; large files round-trip without buffering in memory.
	// First-chunk peek (≤1 MiB) gives us the title without touching
	// downstream chunks — and is skipped entirely for a kind whose
	// bytes carry no title, which is every non-textual one.
	if req.Context == nil || req.Context.Store == nil {
		return handler.NewErrorResponse(500, "internal_error",
			"ingest-transform requires content store in handler context")
	}
	class := ClassifyDocPath(file.Path)
	if !class.Textual {
		if !req.Context.Store.Has(file.Content) {
			return handler.NewErrorResponse(503, "blob_pending_sync",
				"blob "+file.Content.String()+" not yet in local content store")
		}
		return h.emit(file, class, "")
	}
	firstChunk, blobPresent, err := LoadMarkdownFirstChunk(req.Context.Store, file.Content)
	if err != nil {
		return handler.NewErrorResponse(500, "first_chunk_failed",
			"load first chunk for "+file.Content.String()+": "+err.Error())
	}
	if !blobPresent {
		// L12 canonical name (Amendment 2 of v1.3). Chain retries on
		// next sync event when the blob lands.
		return handler.NewErrorResponse(503, "blob_pending_sync",
			"blob "+file.Content.String()+" not yet in local content store")
	}

	// Only markdown has a heading worth reading a title out of. Running
	// the scan over a .go or a .csv finds the first line starting with
	// "#" — a comment, or a shebang-adjacent line — and produces a
	// confident wrong label.
	var title string
	if class.IsMarkdown() {
		title = extractFirstHeading(string(firstChunk))
	}
	return h.emit(file, class, title)
}

// emit builds the typed document entity for one classified file. Shared
// by the textual and non-textual paths so the two cannot drift in what
// they write, only in how much they had to read to get there.
func (h *IngestTransformHandler) emit(file localfiles.FileData, class DocClass, title string) (*handler.Response, error) {
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(file.Path), filepath.Ext(file.Path))
	}

	// Markdown keeps MarkdownFileData verbatim — see doc_file_data.go
	// for why the registry does not widen it.
	if class.IsMarkdown() {
		ent, err := MarkdownFileData{
			Path:    file.Path,
			Title:   title,
			Content: file.Content,
			Size:    int64(file.Size),
		}.ToEntity()
		if err != nil {
			return handler.NewErrorResponse(500, "entity_build_failed",
				"build doc/markdown-file entity: "+err.Error())
		}
		return &handler.Response{Status: 200, Result: ent}, nil
	}

	mediaType := class.MediaType
	if file.MediaType != nil && *file.MediaType != "" {
		mediaType = *file.MediaType
	}
	ent, err := DocFileData{
		Path:      file.Path,
		Title:     title,
		Content:   file.Content,
		Size:      int64(file.Size),
		MediaType: mediaType,
		Language:  class.Language,
		Kind:      string(class.Kind),
	}.ToEntityOfType(class.EntityType)
	if err != nil {
		return handler.NewErrorResponse(500, "entity_build_failed",
			"build "+class.EntityType+" entity: "+err.Error())
	}
	return &handler.Response{Status: 200, Result: ent}, nil
}
