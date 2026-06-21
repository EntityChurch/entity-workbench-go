package shellcmd

import (
	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"
)

// Result is the renderer-neutral output of a shell command. Like the
// panel models' Output structs, it carries everything a presentation
// backend (stdout/JSON formatter, panel OutputLine adapter) needs to
// render the result without reaching back into command internals.
//
// Each command populates exactly one result variant per call. Empty
// results are valid (e.g. cd succeeds silently).
type Result struct {
	// Kind identifies which payload field is populated. Empty Kind
	// means "no output" (e.g. successful cd, pwd echoes WD via Path).
	Kind ResultKind

	// One of the following payloads is set per Kind:
	Message  string         // KindMessage: free-form line (info/help/etc.)
	Path     Path           // KindPath: a path (pwd, post-cd echo)
	Listing  []ListingRow   // KindListing: ls / list-of-peers
	Entity   *EntityPayload // KindEntity: cat output
	Tree     []TreeRow      // KindTree: tree output, flat with depth
	Dispatch *DispatchResp  // KindDispatch: exec response
	Info     *PeerInfo      // KindInfo: peer connection details
	Lines    []string       // KindLines: pre-formatted multiline (help screen)
}

// ResultKind tags which payload variant of Result is populated.
type ResultKind int

const (
	KindNone     ResultKind = iota
	KindMessage             // simple text
	KindPath                // a Path value
	KindListing             // []ListingRow
	KindEntity              // *EntityPayload
	KindTree                // []TreeRow
	KindDispatch            // *DispatchResp
	KindInfo                // *PeerInfo (or list)
	KindLines               // []string preformatted
)

// ListingRow is a row in the output of `ls` (or the connection list
// at root). For the root listing, Path is empty and Kind is
// "connection"; the alias is in Name and the addr in Detail.
type ListingRow struct {
	Name        string // last segment / alias
	Path        string // full bare path within the peer (empty at root)
	Kind        string // "dir" | "entity" | "dir+entity" | "connection" | ""
	Detail      string // address (root-level connections) or empty
	HasChildren bool
	Hash        hash.Hash // entity hash if Kind contains "entity"
}

// EntityPayload is the result of `cat <path>`. Decoded carries the
// CBOR-decoded data (any Go type); raw access is via Entity. When
// Diag is true, the formatter should emit the diagnostic-hash form
// instead of the type/hash/data display.
type EntityPayload struct {
	Entity  entity.Entity
	Decoded interface{}
	Diag    bool
}

// TreeRow is one line of `tree` output. Depth is 0-indexed from the
// root passed to tree.
type TreeRow struct {
	Path        string
	Name        string
	Depth       int
	Kind        string // "dir" | "entity" | "dir+entity"
	HasChildren bool
	Hash        hash.Hash
	// When verbose, Entity / Decoded are populated for entity rows.
	Entity  *entity.Entity
	Decoded interface{}
}

// DispatchResp is the result of `exec`. Status is the response
// status code from the handler (matches the entitysdk Response
// uint shape); Result is the result entity; Decoded is the
// CBOR-decoded data of the result entity. Included counts any extra
// entities returned alongside.
type DispatchResp struct {
	Status   int
	Result   entity.Entity
	Decoded  interface{}
	Included int
}

// PeerInfo is the result of `info`. When listing all peers, the
// shell returns multiple Results of KindInfo in sequence (or one
// Result with Lines pre-formatted — implementor's choice).
type PeerInfo struct {
	Alias   string
	Address string
	PeerID  string
	Grants  []types.GrantEntry
}

// MessageResult is a short-hand for KindMessage results.
func MessageResult(msg string) Result {
	return Result{Kind: KindMessage, Message: msg}
}

// PathResult is short-hand for KindPath results.
func PathResult(p Path) Result {
	return Result{Kind: KindPath, Path: p}
}

// LinesResult is short-hand for KindLines results.
func LinesResult(lines []string) Result {
	return Result{Kind: KindLines, Lines: lines}
}
