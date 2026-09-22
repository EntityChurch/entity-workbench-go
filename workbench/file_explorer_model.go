package workbench

import (
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/store"
	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/localfiles"
)

// file_explorer_model.go — a directory-at-a-time browser over a mount.
//
// # The question this answers
//
// "I mounted a folder. Where are my files?" Before this model there was
// no surface in the repo that answered it. The Local Files panel listed
// MOUNTS — root name, filesystem root, target prefix, a count — and never
// a file. The raw tree panel showed entity paths with CBOR behind them.
// The Markdown Files panel showed markdown, from the hard-coded prefix
// `docs/`, so a mount to any other prefix was invisible there even for
// the one kind that did ingest.
//
// So the mount worked, the count went up, and nothing an operator could
// open appeared anywhere. That is D23 with every layer green.
//
// # Two layers, and the gap between them is the interesting part
//
// A mount writes into two places, and this model reads both:
//
//	local/files/{root}/{rel}   the SOURCE layer — one `local/files/file`
//	                           per admitted file, written by the kernel's
//	                           watcher. This is ground truth for "what is
//	                           on disk and got in".
//	{targetPrefix}{rel}        the DOCUMENT layer — one `doc/*` entity per
//	                           file the ingest chain classified and lifted.
//	                           This is what a viewer can open.
//
// A file present in the first and missing from the second is the whole
// failure mode that went unnoticed for as long as it did: before the
// ingest registry, that gap was *every non-markdown file in the mount*.
// It is why [ExplorerEntry.Status] exists and why the summary counts
// `NotIngested` separately instead of reporting one total. **A single
// count cannot distinguish a healthy mount from a mount that ingested
// nothing openable, and the source layer — the one that never fails — is
// the one a naive count reads.**
//
// # Cost
//
// The two prefix maps are seeded with one prefix-scoped List each and
// then maintained by subscription, so a render is O(entries in the
// current directory), not O(mount). Sizes and types are decoded lazily
// at render time for the visible directory only: a 40k-file mount costs
// two index scans at open and a few dozen Gets per navigation, and never
// decodes an entity the operator is not looking at.
//
// This is deliberately NOT the shape LocalFilesModel uses (stateless, re-read
// on demand). Mount *configs* change once a session; mount *contents*
// churn on every save in a watched directory, which is exactly the case
// where a subscription earns its complexity.

// ExplorerEntry is one row — a folder or a file — in the current
// directory.
type ExplorerEntry struct {
	// Name is the display name: the folder segment, or the filename.
	Name string
	// RelPath is the path relative to the mount root, forward-slashed.
	// Folders carry no trailing slash.
	RelPath string
	// IsDir distinguishes the two row shapes. A folder is not an entity
	// — it is inferred from the paths beneath it, exactly as a
	// filesystem browser infers one from a listing.
	IsDir bool

	// --- folder rows ---

	// ChildFiles is the number of files anywhere beneath this folder.
	// Recursive rather than immediate, because "3 items" for a folder
	// holding three folders holding a thousand files is the less useful
	// of the two answers.
	ChildFiles int
	// ChildBytes is the total size beneath this folder.
	ChildBytes int64

	// --- file rows ---

	// Size is the file's size in bytes, from the source FileData.
	Size int64
	// ModifiedAt is the source file's mtime as a Unix second, 0 when the
	// watcher did not record one.
	ModifiedAt int64
	// Kind is the registry's coarse classification ("markdown", "text",
	// "code", "image", "binary"). Present even when the file has no
	// document yet — it is derived from the name, so it is knowable
	// before ingest and is what tells an operator what to expect.
	Kind string
	// Language is the syntax hint for code kinds, empty otherwise.
	Language string
	// MediaType is the best available IANA type, empty when unknown.
	MediaType string
	// SourcePath is the full tree path of the `local/files/file` entity.
	SourcePath string
	// TargetPath is the full tree path of the document entity, empty
	// when nothing is bound there. **This is the path a renderer
	// publishes as the selection** — the source entity is a file record,
	// the target entity is the document.
	TargetPath string
	// EntityType is the document's tree type, empty when not ingested.
	EntityType string
	// Ingested reports whether a document exists at TargetPath.
	Ingested bool
	// Status is a short operator-facing phrase, always set. For an
	// ingested file it names the kind; otherwise it names the REASON, so
	// a row never reads as merely absent. See statusFor.
	Status string
}

// ExplorerOutput is the renderer-neutral snapshot of one directory.
type ExplorerOutput struct {
	// Root is the mount's root name, empty when no mount is bound.
	Root string
	// FilesystemRoot is the on-disk directory this mount bridges.
	FilesystemRoot string
	// SourcePrefix / TargetPrefix are the two tree prefixes.
	SourcePrefix string
	TargetPrefix string

	// Dir is the current directory relative to the mount root — "" at
	// the top, else "a/b" with no trailing slash.
	Dir string
	// Crumbs is Dir split into segments, for a clickable breadcrumb.
	Crumbs []string
	// Entries is the current directory's contents: folders first, then
	// files, each group name-sorted. Stable across renders, because a
	// listing that reorders under the reader is a correctness surface
	// and not a cosmetic one (AP49).
	Entries []ExplorerEntry

	// --- whole-mount summary, not directory-scoped ---

	// TotalFiles is every file in the source layer under this mount.
	TotalFiles int
	// TotalBytes is their combined size. Costs a decode per file, so it
	// is computed from the cached sizes maintained alongside the index
	// rather than re-read per render.
	TotalBytes int64
	// Ingested is how many of them have a document at the target.
	Ingested int
	// NotIngested is TotalFiles - Ingested. Reported separately and
	// deliberately: it is the number that answers "why can't I open
	// anything", and a single total hides it completely.
	NotIngested int
	// KindCounts tallies the mount by kind, keyed by DocKind string.
	KindCounts map[string]int

	// Note explains an empty listing rather than leaving the renderer to
	// present "nothing here" and "nothing read" identically.
	Note string
	// Error is set when the mount could not be bound at all.
	Error string
}

// FileExplorerModel browses one mount's contents.
//
// Bind a mount with [FileExplorerModel.SetRoot], navigate with
// [FileExplorerModel.SetDir], read with [FileExplorerModel.Render], and
// [FileExplorerModel.Close] when done. Safe for concurrent use: the
// subscription callbacks run on SDK-owned goroutines.
type FileExplorerModel struct {
	store *Store

	mu sync.Mutex

	root           string
	filesystemRoot string
	sourcePrefix   string
	targetPrefix   string
	include        []string
	exclude        []string
	bindErr        string

	dir string

	// source and target are PRESENCE sets keyed by mount-relative path,
	// seeded by List and maintained by subscription. Presence only: the
	// hash is deliberately not cached, because a cached hash goes stale
	// the moment a file is edited and would render a size from the
	// previous revision. Everything else is read through Store.Get at
	// the moment it is needed, for the current directory only.
	source map[string]struct{}
	target map[string]struct{}

	// sizes caches the decoded size per relative path so the whole-mount
	// byte total does not require decoding every file on every render.
	// Populated on first decode; invalidated by the subscription.
	sizes map[string]int64

	// Two subscriptions, held as named fields rather than a slice.
	// Positional bookkeeping ("the target one is at index 1") was the
	// first shape here and it is the kind of invariant that survives
	// exactly until someone adds a third.
	cancelSource func()
	cancelTarget func()

	// onChange is the renderer's wake. Fired on every index mutation,
	// which is bursty during a watcher's initial scan — the callback is
	// expected to coalesce (the bridge does, into a one-deep channel).
	// Called WITHOUT the lock held: a renderer that re-entered Render
	// from inside it would otherwise deadlock, and that is exactly what
	// a naive "just refresh" handler does.
	onChange func()
}

// OnChange registers a callback fired whenever the mount's contents
// change underneath the model. Pass nil to clear. The callback runs on an
// SDK-owned goroutine and must not touch a renderer directly.
func (m *FileExplorerModel) OnChange(fn func()) {
	m.mu.Lock()
	m.onChange = fn
	m.mu.Unlock()
}

// NewFileExplorerModel binds a model to a peer's store. No mount is
// selected until SetRoot is called.
func NewFileExplorerModel(st *Store) *FileExplorerModel {
	if st == nil {
		panic("workbench: NewFileExplorerModel requires non-nil Store")
	}
	return &FileExplorerModel{
		store:  st,
		source: map[string]struct{}{},
		target: map[string]struct{}{},
		sizes:  map[string]int64{},
	}
}

// SetRoot binds the model to the named mount, reading its persisted
// RootConfig for the prefixes and filters, seeding both indexes and
// attaching the subscriptions. Re-binding to a different root tears the
// previous subscriptions down first.
//
// A root that does not exist is an error, not an empty listing: "no such
// mount" and "a mount with no files" are different claims and a surface
// that renders them identically is lying about one of them (AP33).
func (m *FileExplorerModel) SetRoot(rootName string) error {
	// Detach BEFORE the lock — see Close for why cancelling under it
	// deadlocks against the SDK's delivery goroutine.
	m.detachSubscriptions()

	m.mu.Lock()
	m.root = rootName
	m.dir = ""
	m.filesystemRoot = ""
	m.sourcePrefix = ""
	m.targetPrefix = ""
	m.include, m.exclude = nil, nil
	m.source = map[string]struct{}{}
	m.target = map[string]struct{}{}
	m.sizes = map[string]int64{}
	m.bindErr = ""

	if rootName == "" {
		m.mu.Unlock()
		return nil
	}

	cfgPath := MountConfigPrefix + rootName
	ent, ok := m.store.Get(cfgPath)
	if !ok {
		m.bindErr = "no mount named " + rootName + " (nothing at " + cfgPath + ")"
		err := fmt.Errorf("%s", m.bindErr)
		m.mu.Unlock()
		return err
	}
	cfg, decErr := localfiles.RootConfigDataFromEntity(ent)
	if decErr != nil {
		m.bindErr = "mount config did not decode: " + decErr.Error()
		err := fmt.Errorf("%s", m.bindErr)
		m.mu.Unlock()
		return err
	}

	m.filesystemRoot = cfg.FilesystemRoot
	m.sourcePrefix = ensureSlash(cfg.Prefix)
	m.include = cfg.Include
	m.exclude = cfg.Exclude
	sourcePrefix := m.sourcePrefix

	// Seed from the index under the lock, so the model is populated by
	// the time SetRoot returns and a caller can Render immediately
	// without waiting on a subscription's delivery.
	for _, e := range m.store.List(sourcePrefix) {
		if rel, under := RelativeUnder(e.Path, sourcePrefix); under && rel != "" {
			m.source[rel] = struct{}{}
		}
	}

	// The target prefix is NOT in the kernel's RootConfig — `ext/localfiles`
	// has never heard of the workbench-application mapping laid over it.
	// It comes from our own durable record (mount_binding.go). A mount
	// written before that record existed has none, and the model reports
	// "unknown" rather than guessing `docs/`: a guessed prefix renders
	// every file as un-ingested, which is indistinguishable from the real
	// defect this model exists to show.
	binding, hasBinding := LoadMountBinding(m.store, rootName)
	m.mu.Unlock()

	// **Subscribe OUTSIDE the lock.** Store.OnPrefixChange delivers its
	// seed SYNCHRONOUSLY on the calling goroutine when the store has no
	// watch hub (unit-test scaffolding), so attaching under the lock
	// deadlocks the moment the first seeded event calls back into
	// onSourceEvent. Go mutexes are not reentrant and the failure is a
	// hang, not a panic — it would have looked like a slow test.
	//
	// The gap between unlocking and attaching loses nothing: the
	// subscription's own seed re-delivers current state at attach, which
	// is exactly what that seed is for.
	cancel := m.store.OnPrefixChange(sourcePrefix, m.onSourceEvent)
	m.mu.Lock()
	m.cancelSource = cancel
	m.mu.Unlock()

	if hasBinding && binding.TargetPrefix != "" {
		m.SetTargetPrefix(binding.TargetPrefix)
	}
	return nil
}

// SetTargetPrefix supplies the workbench-application target prefix for
// the bound mount, overriding the persisted binding.
//
// Separate from SetRoot because the two facts live in different places:
// the kernel persists the RootConfig (filesystem root, source prefix,
// filters) and we persist the source→target mapping. SetRoot reads the
// latter automatically; this exists for a caller that knows better —
// a test, or a mount predating the durable binding.
func (m *FileExplorerModel) SetTargetPrefix(prefix string) {
	// Detach the previous target subscription outside the lock, for
	// Close's reason. The source subscription is untouched.
	m.mu.Lock()
	ct := m.cancelTarget
	m.cancelTarget = nil
	m.mu.Unlock()
	if ct != nil {
		ct()
	}

	m.mu.Lock()
	m.targetPrefix = ensureSlash(prefix)
	m.target = map[string]struct{}{}
	targetPrefix := m.targetPrefix
	if targetPrefix == "" {
		m.mu.Unlock()
		return
	}
	for _, e := range m.store.List(targetPrefix) {
		if rel, under := RelativeUnder(e.Path, targetPrefix); under && rel != "" {
			m.target[rel] = struct{}{}
		}
	}
	m.mu.Unlock()

	// Outside the lock, for SetRoot's reason.
	cancel := m.store.OnPrefixChange(targetPrefix, m.onTargetEvent)
	m.mu.Lock()
	m.cancelTarget = cancel
	m.mu.Unlock()
}

// onSourceEvent maintains the source presence set. Runs on an SDK-owned
// goroutine — it must not touch a renderer; renderers see the effect at
// their next Render.
func (m *FileExplorerModel) onSourceEvent(ev ChangeEvent) {
	m.mu.Lock()
	if m.sourcePrefix == "" {
		m.mu.Unlock()
		return
	}
	rel, under := RelativeUnder(ev.Path, m.sourcePrefix)
	if !under || rel == "" {
		m.mu.Unlock()
		return
	}
	switch ev.EventType {
	case ChangePut:
		m.source[rel] = struct{}{}
		// The size cache is keyed by path and the path's CONTENT just
		// changed. Dropping the entry is what stops a re-saved file
		// rendering its previous size forever.
		delete(m.sizes, rel)
	case ChangeRemove:
		delete(m.source, rel)
		delete(m.sizes, rel)
	}
	fn := m.onChange
	m.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// onTargetEvent maintains the document presence set.
func (m *FileExplorerModel) onTargetEvent(ev ChangeEvent) {
	m.mu.Lock()
	if m.targetPrefix == "" {
		m.mu.Unlock()
		return
	}
	rel, under := RelativeUnder(ev.Path, m.targetPrefix)
	if !under || rel == "" {
		m.mu.Unlock()
		return
	}
	switch ev.EventType {
	case ChangePut:
		m.target[rel] = struct{}{}
	case ChangeRemove:
		delete(m.target, rel)
	}
	fn := m.onChange
	m.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// Close cancels every subscription. Idempotent.
//
// **Cancels OUTSIDE the lock, and that is load-bearing.** The SDK's
// cancel closes the underlying watch and then WAITS for its delivery
// goroutine to exit. If that goroutine is mid-delivery it is sitting in
// onSourceEvent trying to take m.mu — so cancelling while holding m.mu
// is a deadlock between the closer and the deliverer, and Go mutexes are
// not reentrant. It hangs rather than panicking, which means it presents
// as a slow test rather than a broken one: this was found because the
// full suite sat on `workbench` for sixteen minutes.
//
// Same hazard as SetRoot's attach, from the other side. The rule for
// this model is one line: **never call into the store while holding
// m.mu.**
func (m *FileExplorerModel) Close() {
	// Drop the wake first: a callback fired between here and the cancel
	// would wake a renderer for a model that is going away.
	m.mu.Lock()
	m.onChange = nil
	m.mu.Unlock()
	m.detachSubscriptions()
}

// detachSubscriptions cancels both prefix subscriptions and returns once
// the SDK's delivery goroutines have stopped. Idempotent.
//
// It deliberately does NOT touch onChange. Re-binding to a different
// mount goes through here, and a renderer's wake registration outlives
// the mount it was watching — clearing it on SetRoot would leave the
// panel live-updating for exactly one mount and silently static for
// every one the operator picked afterwards.
func (m *FileExplorerModel) detachSubscriptions() {
	m.mu.Lock()
	cs, ct := m.cancelSource, m.cancelTarget
	m.cancelSource, m.cancelTarget = nil, nil
	m.mu.Unlock()

	if cs != nil {
		cs()
	}
	if ct != nil {
		ct()
	}
}

// Dir returns the current directory, relative to the mount root.
func (m *FileExplorerModel) Dir() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.dir
}

// SetDir navigates to a directory relative to the mount root. "" is the
// mount root. Cleans the path and refuses to escape the mount — a
// browser that accepts "../.." and then renders whatever it finds is a
// containment hole with a friendly face, and the kernel's own localfiles
// containment check does not protect a read that never touches the disk.
func (m *FileExplorerModel) SetDir(rel string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dir = cleanExplorerDir(rel)
}

// Up navigates to the parent directory. A no-op at the mount root.
func (m *FileExplorerModel) Up() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dir == "" {
		return
	}
	idx := strings.LastIndex(m.dir, "/")
	if idx < 0 {
		m.dir = ""
		return
	}
	m.dir = m.dir[:idx]
}

// cleanExplorerDir normalizes a directory argument and clamps it inside
// the mount. Returns "" for anything that would escape.
func cleanExplorerDir(rel string) string {
	rel = strings.Trim(strings.TrimSpace(strings.ReplaceAll(rel, "\\", "/")), "/")
	if rel == "" {
		return ""
	}
	cleaned := path.Clean(rel)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return ""
	}
	return cleaned
}

// Render snapshots the current directory.
func (m *FileExplorerModel) Render() ExplorerOutput {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := ExplorerOutput{
		Root:           m.root,
		FilesystemRoot: m.filesystemRoot,
		SourcePrefix:   m.sourcePrefix,
		TargetPrefix:   m.targetPrefix,
		Dir:            m.dir,
		KindCounts:     map[string]int{},
	}
	if m.dir != "" {
		out.Crumbs = strings.Split(m.dir, "/")
	}
	if m.bindErr != "" {
		out.Error = m.bindErr
		return out
	}
	if m.root == "" {
		out.Note = "no mount selected"
		return out
	}

	// Whole-mount summary first: one pass over the source index, which
	// is a map walk and not a store read except for sizes we have not
	// cached yet.
	for rel := range m.source {
		out.TotalFiles++
		out.KindCounts[string(ClassifyDocPath(rel).Kind)]++
		out.TotalBytes += m.sizeOfLocked(rel)
		if _, ok := m.target[rel]; ok {
			out.Ingested++
		}
	}
	out.NotIngested = out.TotalFiles - out.Ingested

	dirPrefix := ""
	if m.dir != "" {
		dirPrefix = m.dir + "/"
	}

	// Immediate children of the current directory. A path with a further
	// "/" after the prefix contributes to a folder row; one without is a
	// file row.
	folders := map[string]*ExplorerEntry{}
	var files []ExplorerEntry

	for rel := range m.source {
		if dirPrefix != "" && !strings.HasPrefix(rel, dirPrefix) {
			continue
		}
		tail := strings.TrimPrefix(rel, dirPrefix)
		if tail == "" {
			continue
		}
		if idx := strings.Index(tail, "/"); idx >= 0 {
			seg := tail[:idx]
			f, ok := folders[seg]
			if !ok {
				f = &ExplorerEntry{
					Name:    seg,
					RelPath: dirPrefix + seg,
					IsDir:   true,
				}
				folders[seg] = f
			}
			f.ChildFiles++
			f.ChildBytes += m.sizeOfLocked(rel)
			continue
		}
		files = append(files, m.fileEntryLocked(rel, tail))
	}

	entries := make([]ExplorerEntry, 0, len(folders)+len(files))
	folderNames := make([]string, 0, len(folders))
	for name := range folders {
		folderNames = append(folderNames, name)
	}
	sort.Strings(folderNames)
	for _, name := range folderNames {
		entries = append(entries, *folders[name])
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	entries = append(entries, files...)
	out.Entries = entries

	if len(entries) == 0 {
		if out.TotalFiles == 0 {
			out.Note = "this mount holds no files yet — the watcher ingests on its first scan " +
				"and on every change after it"
		} else {
			out.Note = "nothing at " + m.dir
		}
	}
	if m.targetPrefix == "" && out.TotalFiles > 0 {
		// Say it rather than render every row as un-ingested. Without a
		// target prefix we cannot know, and "unknown" is the honest
		// answer (AP45).
		out.Note = strings.TrimSpace(out.Note + " · no target prefix bound for this mount, " +
			"so document status is unknown rather than absent")
	}
	return out
}

// fileEntryLocked builds one file row. Caller holds the lock.
func (m *FileExplorerModel) fileEntryLocked(rel, name string) ExplorerEntry {
	class := ClassifyDocPath(name)
	e := ExplorerEntry{
		Name:       name,
		RelPath:    rel,
		Size:       m.sizeOfLocked(rel),
		Kind:       string(class.Kind),
		Language:   class.Language,
		MediaType:  class.MediaType,
		SourcePath: m.sourcePrefix + rel,
	}
	if ent, found := m.store.Get(m.sourcePrefix + rel); found {
		if fd, err := localfiles.FileDataFromEntity(ent); err == nil {
			if fd.ModifiedAt != nil {
				e.ModifiedAt = int64(*fd.ModifiedAt)
			}
			// The watcher saw the file; the registry saw its name.
			if fd.MediaType != nil && *fd.MediaType != "" {
				e.MediaType = *fd.MediaType
			}
		}
	}
	if m.targetPrefix != "" {
		if _, ok := m.target[rel]; ok {
			e.Ingested = true
			e.TargetPath = m.targetPrefix + rel
			if ent, found := m.store.Get(e.TargetPath); found {
				e.EntityType = ent.Type
			}
		}
	}
	e.Status = m.statusForLocked(rel, e, class)
	return e
}

// statusForLocked produces the short phrase on a file row. Every branch
// is a REASON, never a bare absence: "not ingested" with no explanation
// is the exact shape that let the markdown-only gate hide for as long as
// it did.
func (m *FileExplorerModel) statusForLocked(rel string, e ExplorerEntry, class DocClass) string {
	if e.Ingested {
		if class.Language != "" {
			return class.Language
		}
		return string(class.Kind)
	}
	if m.targetPrefix == "" {
		return "unknown — no target prefix bound"
	}
	if !passesMountFilter(rel, m.include, m.exclude) {
		return "excluded by this mount's filters"
	}
	return "no document yet — ingest pending or failed"
}

// sizeOfLocked returns the cached size for a relative path, decoding the
// source entity once on first ask. Caller holds the lock.
func (m *FileExplorerModel) sizeOfLocked(rel string) int64 {
	if sz, ok := m.sizes[rel]; ok {
		return sz
	}
	if _, ok := m.source[rel]; !ok {
		return 0
	}
	ent, found := m.store.Get(m.sourcePrefix + rel)
	if !found {
		return 0
	}
	fd, err := localfiles.FileDataFromEntity(ent)
	if err != nil {
		return 0
	}
	sz := int64(fd.Size)
	m.sizes[rel] = sz
	return sz
}

// MaxPreviewBytes caps an inline preview. Chosen to be large enough for
// any source file a person wrote and small enough that the whole payload
// crosses the cgo boundary as a JSON string without the 8 MB problem the
// browser panel hit with pre-rendered HTML pages.
const MaxPreviewBytes = 256 * 1024

// ExplorerPreview is the inline view of one file's bytes.
type ExplorerPreview struct {
	// RelPath is the file, relative to the mount root.
	RelPath string
	// Kind is the registry classification.
	Kind string
	// Language is the syntax hint for code, empty otherwise.
	Language string
	// Text is the payload, valid only when Textual is true.
	Text string
	// Textual reports whether Text was populated. False for image and
	// binary kinds, which are not rendered as mojibake on the grounds
	// that something is better than nothing — it is not.
	Textual bool
	// Truncated reports that Text stops at MaxPreviewBytes.
	Truncated bool
	// Size is the file's full size regardless of truncation.
	Size int64
	// Error explains why Text is empty when it should not have been.
	Error string
}

// Preview reads a file's bytes for inline display.
//
// Reads through the SOURCE layer's content hash rather than the
// document's, on purpose: the source entity exists for every admitted
// file, and the document may not exist at all — a preview that only
// worked for ingested files would be blind to exactly the rows an
// operator is most likely to click on to find out what went wrong.
func (m *FileExplorerModel) Preview(rel string) ExplorerPreview {
	m.mu.Lock()
	defer m.mu.Unlock()

	class := ClassifyDocPath(rel)
	p := ExplorerPreview{
		RelPath:  rel,
		Kind:     string(class.Kind),
		Language: class.Language,
		Size:     m.sizeOfLocked(rel),
	}
	if _, ok := m.source[rel]; !ok {
		p.Error = "no file at " + rel + " in this mount"
		return p
	}
	if !class.Textual {
		// Not an error. A binary has nothing to show and saying so is
		// the answer, not a failure to produce one.
		return p
	}
	ent, found := m.store.Get(m.sourcePrefix + rel)
	if !found {
		p.Error = "source entity missing from the store"
		return p
	}
	fd, err := localfiles.FileDataFromEntity(ent)
	if err != nil {
		p.Error = "source entity did not decode: " + err.Error()
		return p
	}
	body, truncated, err := loadBlobCapped(m.store.ContentStore(), fd.Content, MaxPreviewBytes)
	if err != nil {
		p.Error = err.Error()
		return p
	}
	p.Textual = true
	p.Truncated = truncated
	p.Text = string(body)
	return p
}

// loadBlobCapped reassembles a content blob, stopping once max bytes have
// been collected.
//
// Chunk-at-a-time with an early exit rather than LoadMarkdownContent's
// full buffer: a preview of a 400 MB log file must not allocate 400 MB to
// show the first screenful, and the chunk boundary is where that decision
// can actually be made.
func loadBlobCapped(cs store.ContentStore, blobHash hash.Hash, max int) ([]byte, bool, error) {
	blobEnt, ok := cs.Get(blobHash)
	if !ok {
		return nil, false, fmt.Errorf("content blob not in the local store yet")
	}
	var blob types.ContentBlobData
	if err := ecf.Decode(blobEnt.Data, &blob); err != nil {
		return nil, false, fmt.Errorf("decode blob: %w", err)
	}
	buf := make([]byte, 0, min64(int64(blob.TotalSize), int64(max)))
	for i, chunkHash := range blob.Chunks {
		if len(buf) >= max {
			return buf[:max], true, nil
		}
		ent, ok := cs.Get(chunkHash)
		if !ok {
			return nil, false, fmt.Errorf("chunk %d missing from the local content store", i)
		}
		var chunk types.ContentChunkData
		if err := ecf.Decode(ent.Data, &chunk); err != nil {
			return nil, false, fmt.Errorf("decode chunk %d: %w", i, err)
		}
		buf = append(buf, chunk.Payload...)
	}
	if len(buf) > max {
		return buf[:max], true, nil
	}
	return buf, false, nil
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// ensureSlash appends a trailing "/" to a non-empty prefix.
func ensureSlash(p string) string {
	if p == "" || strings.HasSuffix(p, "/") {
		return p
	}
	return p + "/"
}
