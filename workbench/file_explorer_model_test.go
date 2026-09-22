package workbench

import (
	"strings"
	"sync"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/store"
	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/content"
	"go.entitychurch.org/entity-core-go/ext/content/chunker"
	"go.entitychurch.org/entity-core-go/ext/localfiles"
)

// Tier: model (TESTING-STRATEGY §2) — a real Store over memory
// content/location, no peer, no network, no filesystem.

const (
	testFSRoot = "/home/op/notes"
	testRoot   = "notes"
	testSource = "local/files/notes/"
	testTarget = "archives/notes/"
)

// explorerFixture builds a mount whose shape is the one that broke: a
// directory of mixed kinds where only SOME files reached the document
// layer. Returns the store and the model, already bound.
//
// The mount is registered the way a real one is — the kernel's RootConfig
// plus our MountBindingData — because SetRoot reads both, and a fixture
// that seeded only the first would test a mount that cannot exist.
func explorerFixture(t *testing.T) (*Store, *FileExplorerModel) {
	t.Helper()
	return explorerFixtureOn(t, NewStore(store.NewMemoryContentStore(), store.NewMemoryLocationIndex()))
}

// explorerFixtureOn seeds the same mount into a caller-supplied store, so
// a test that needs live subscription delivery can hand in a peer-backed
// one without duplicating the fixture.
func explorerFixtureOn(t *testing.T, st *Store) (*Store, *FileExplorerModel) {
	t.Helper()
	if _, err := st.Put(MountConfigPrefix+testRoot, localfiles.TypeRootConfig,
		localfiles.RootConfigData{
			Prefix:         testSource,
			FilesystemRoot: testFSRoot,
			Exclude:        []string{".git"},
		}); err != nil {
		t.Fatal(err)
	}
	if err := SaveMountBinding(st, MountBindingData{
		Root:         testRoot,
		SourcePrefix: testSource,
		TargetPrefix: testTarget,
	}); err != nil {
		t.Fatal(err)
	}

	// Source layer: every admitted file. Document layer: only some.
	putSourceFile(t, st, "readme.md", []byte("# Readme\n\nbody"))
	putSourceFile(t, st, "todo.txt", []byte("buy milk"))
	putSourceFile(t, st, "photo.png", []byte{0x89, 'P', 'N', 'G', 0, 1, 2, 3})
	putSourceFile(t, st, "src/main.go", []byte("package main\n"))
	putSourceFile(t, st, "src/deep/util.go", []byte("package deep\n"))

	// readme.md and main.go made it to documents; the rest did not —
	// exactly the gap an operator sees and cannot explain.
	putDoc(t, st, "readme.md", MarkdownFileType)
	putDoc(t, st, "src/main.go", CodeFileType)

	m := NewFileExplorerModel(st)
	if err := m.SetRoot(testRoot); err != nil {
		t.Fatalf("SetRoot: %v", err)
	}
	t.Cleanup(m.Close)
	return st, m
}

func putSourceFile(t *testing.T, st *Store, rel string, body []byte) {
	t.Helper()
	ranges := chunker.ChunkFastCDC(body, types.DefaultChunkSize)
	blobHash, err := content.IngestBlob(body, ranges, types.ChunkingFastCDC,
		types.DefaultChunkSize, st.ContentStore())
	if err != nil {
		t.Fatalf("ingest blob for %s: %v", rel, err)
	}
	mtime := uint64(1_700_000_000)
	if _, err := st.Put(testSource+rel, localfiles.TypeFile, localfiles.FileData{
		Path:       rel,
		Size:       uint64(len(body)),
		Content:    blobHash,
		ModifiedAt: &mtime,
	}); err != nil {
		t.Fatalf("put source %s: %v", rel, err)
	}
}

func putDoc(t *testing.T, st *Store, rel, entityType string) {
	t.Helper()
	if _, err := st.Put(testTarget+rel, entityType, DocFileData{
		Path:  rel,
		Title: rel,
		Size:  1,
		Kind:  string(ClassifyDocPath(rel).Kind),
	}); err != nil {
		t.Fatalf("put doc %s: %v", rel, err)
	}
}

func entryByName(out ExplorerOutput, name string) (ExplorerEntry, bool) {
	for _, e := range out.Entries {
		if e.Name == name {
			return e, true
		}
	}
	return ExplorerEntry{}, false
}

// TestExplorer_ListsMountRoot — folders first, then files, each
// name-sorted, and a folder is inferred from paths rather than being an
// entity of its own.
func TestExplorer_ListsMountRoot(t *testing.T) {
	_, m := explorerFixture(t)
	out := m.Render()

	if out.Error != "" {
		t.Fatalf("unexpected error: %s", out.Error)
	}
	var names []string
	for _, e := range out.Entries {
		names = append(names, e.Name)
	}
	want := []string{"src", "photo.png", "readme.md", "todo.txt"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("root listing = %v, want %v (folders first, then files, each sorted)", names, want)
	}

	src, ok := entryByName(out, "src")
	if !ok || !src.IsDir {
		t.Fatal("src should be an inferred folder row")
	}
	// Recursive, not immediate: src holds one file directly and one
	// under deep/.
	if src.ChildFiles != 2 {
		t.Errorf("src.ChildFiles = %d, want 2 (recursive count)", src.ChildFiles)
	}
}

// TestExplorer_StatusNamesTheReason — the row that made the whole panel
// worth building. A file with no document must say WHY, never just fail
// to appear or read as blank.
func TestExplorer_StatusNamesTheReason(t *testing.T) {
	_, m := explorerFixture(t)
	out := m.Render()

	readme, _ := entryByName(out, "readme.md")
	if !readme.Ingested {
		t.Fatal("readme.md has a document and should report as ingested")
	}
	if readme.TargetPath != testTarget+"readme.md" {
		t.Errorf("readme.md TargetPath = %q, want %q", readme.TargetPath, testTarget+"readme.md")
	}
	if readme.EntityType != MarkdownFileType {
		t.Errorf("readme.md EntityType = %q, want %q", readme.EntityType, MarkdownFileType)
	}

	png, _ := entryByName(out, "photo.png")
	if png.Ingested {
		t.Fatal("photo.png has no document and must not report as ingested")
	}
	if png.TargetPath != "" {
		t.Errorf("photo.png TargetPath = %q, want empty", png.TargetPath)
	}
	if png.Status == "" || !strings.Contains(png.Status, "no document") {
		t.Errorf("photo.png Status = %q — a row with no document must name the reason", png.Status)
	}
	// The kind is knowable from the name even with no document, which is
	// what tells the operator what to expect.
	if png.Kind != string(DocKindImage) {
		t.Errorf("photo.png Kind = %q, want %q", png.Kind, DocKindImage)
	}
}

// TestExplorer_SummarySeparatesIngestedFromTotal — a single count cannot
// distinguish a healthy mount from one that ingested nothing openable.
// This is the number the old mount row got wrong.
func TestExplorer_SummarySeparatesIngestedFromTotal(t *testing.T) {
	_, m := explorerFixture(t)
	out := m.Render()

	if out.TotalFiles != 5 {
		t.Fatalf("TotalFiles = %d, want 5", out.TotalFiles)
	}
	if out.Ingested != 2 {
		t.Errorf("Ingested = %d, want 2", out.Ingested)
	}
	if out.NotIngested != 3 {
		t.Errorf("NotIngested = %d, want 3 — this is the number that answers "+
			"'why can't I open anything'", out.NotIngested)
	}
	if out.KindCounts[string(DocKindImage)] != 1 || out.KindCounts[string(DocKindCode)] != 2 {
		t.Errorf("KindCounts = %v, want 1 image and 2 code", out.KindCounts)
	}
	// Sizes come from the source FileData, decoded lazily and cached.
	if out.TotalBytes == 0 {
		t.Error("TotalBytes = 0, want the sum of the seeded file sizes")
	}
}

func TestExplorer_Navigation(t *testing.T) {
	_, m := explorerFixture(t)

	m.SetDir("src")
	out := m.Render()
	if out.Dir != "src" {
		t.Fatalf("Dir = %q, want src", out.Dir)
	}
	var names []string
	for _, e := range out.Entries {
		names = append(names, e.Name)
	}
	if strings.Join(names, ",") != "deep,main.go" {
		t.Fatalf("src listing = %v, want [deep main.go]", names)
	}

	m.SetDir("src/deep")
	out = m.Render()
	if len(out.Crumbs) != 2 || out.Crumbs[0] != "src" || out.Crumbs[1] != "deep" {
		t.Fatalf("Crumbs = %v, want [src deep]", out.Crumbs)
	}

	m.Up()
	if got := m.Dir(); got != "src" {
		t.Fatalf("after Up, Dir = %q, want src", got)
	}
	m.Up()
	if got := m.Dir(); got != "" {
		t.Fatalf("after second Up, Dir = %q, want the mount root", got)
	}
	m.Up() // no-op at the root, must not panic or go negative
	if got := m.Dir(); got != "" {
		t.Fatalf("Up at the root moved to %q", got)
	}
}

// TestExplorer_SetDirCannotEscapeTheMount — a browser that accepts
// "../.." and renders whatever it finds is a containment hole with a
// friendly face. The kernel's own path containment does not cover this
// because the read never touches the disk.
func TestExplorer_SetDirCannotEscapeTheMount(t *testing.T) {
	_, m := explorerFixture(t)
	for _, attempt := range []string{"..", "../..", "/../etc", "src/../..", "\\..\\.."} {
		m.SetDir(attempt)
		if got := m.Dir(); strings.Contains(got, "..") {
			t.Errorf("SetDir(%q) left Dir = %q — escaped the mount", attempt, got)
		}
	}
}

// TestExplorer_UnknownRootIsAnError — "no such mount" and "a mount with
// no files" are different claims (AP33). A model that returned an empty
// listing for a bad root would render them identically.
func TestExplorer_UnknownRootIsAnError(t *testing.T) {
	st, _ := explorerFixture(t)
	m := NewFileExplorerModel(st)
	defer m.Close()

	if err := m.SetRoot("no-such-mount"); err == nil {
		t.Fatal("expected an error for an unknown root")
	}
	out := m.Render()
	if out.Error == "" {
		t.Fatal("Render must carry the bind error, not present as an empty mount")
	}
	if len(out.Entries) != 0 {
		t.Errorf("a failed bind must list nothing, got %d entries", len(out.Entries))
	}
}

// TestExplorer_NoTargetPrefixReportsUnknownNotAbsent — AP45. A mount
// predating the durable binding has no known target, and rendering every
// file as un-ingested would be a confident wrong answer shaped exactly
// like the real defect.
func TestExplorer_NoTargetPrefixReportsUnknownNotAbsent(t *testing.T) {
	st, _ := explorerFixture(t)
	RemoveMountBinding(st, testRoot)

	m := NewFileExplorerModel(st)
	defer m.Close()
	if err := m.SetRoot(testRoot); err != nil {
		t.Fatal(err)
	}
	out := m.Render()
	if out.TargetPrefix != "" {
		t.Fatalf("TargetPrefix = %q, want empty with no binding", out.TargetPrefix)
	}
	if !strings.Contains(out.Note, "unknown") {
		t.Errorf("Note = %q — must say status is unknown rather than imply absent", out.Note)
	}
	readme, _ := entryByName(out, "readme.md")
	if !strings.Contains(readme.Status, "unknown") {
		t.Errorf("row Status = %q, want it to say unknown", readme.Status)
	}
}

// TestExplorer_PreviewReadsThroughTheSourceLayer — a preview must work
// for a file that has NO document, because those are the rows an operator
// clicks on to find out what went wrong.
func TestExplorer_PreviewReadsThroughTheSourceLayer(t *testing.T) {
	_, m := explorerFixture(t)

	// todo.txt is textual and has no document.
	p := m.Preview("todo.txt")
	if p.Error != "" {
		t.Fatalf("preview error: %s", p.Error)
	}
	if !p.Textual || p.Text != "buy milk" {
		t.Fatalf("preview = %+v, want the file's bytes", p)
	}

	// A binary is not an error and not mojibake — it is a stated absence.
	p = m.Preview("photo.png")
	if p.Error != "" {
		t.Errorf("binary preview reported an error (%s); not showing bytes is the ANSWER", p.Error)
	}
	if p.Textual || p.Text != "" {
		t.Errorf("binary preview produced text: %+v", p)
	}
	if p.Kind != string(DocKindImage) {
		t.Errorf("binary preview Kind = %q, want image", p.Kind)
	}

	// A path that is not in the mount is an error, not empty text.
	p = m.Preview("nope.txt")
	if p.Error == "" {
		t.Error("preview of a file outside the mount must be an error")
	}
}

// TestExplorer_LiveUpdateOnNewFile — the subscription, not a re-List, is
// what keeps the listing current.
//
// Runs against a LIVE peer store (liveStore, peer_liveness_model_test.go)
// rather than the bare NewStore scaffolding the tests above use. That is
// not an incidental choice: without a watch hub, OnPrefixChange degrades
// to seed-only, and this test would pass on the construction-time seed
// while the subscription it claims to exercise was never wired. Same
// shape as the note on liveStore itself — a model whose subscription is
// dead looks identical to a working one under the cheap harness.
func TestExplorer_LiveUpdateOnNewFile(t *testing.T) {
	st, m := explorerFixtureOn(t, liveStore(t))

	if _, ok := entryByName(m.Render(), "later.txt"); ok {
		t.Fatal("fixture should not contain later.txt")
	}
	putSourceFile(t, st, "later.txt", []byte("added after open"))

	out := awaitEntry(t, m, "later.txt")
	e, _ := entryByName(out, "later.txt")
	if e.Size != int64(len("added after open")) {
		t.Errorf("later.txt Size = %d, want %d", e.Size, len("added after open"))
	}
	if out.TotalFiles != 6 {
		t.Errorf("TotalFiles = %d, want 6 after the addition", out.TotalFiles)
	}
}

// TestExplorer_SizeCacheInvalidatedOnRewrite — the size cache is keyed by
// path, and a re-saved file keeps its path. Without the explicit
// invalidation in onSourceEvent, a file would render its first-seen size
// forever. Live store, for the reason above.
func TestExplorer_SizeCacheInvalidatedOnRewrite(t *testing.T) {
	st, m := explorerFixtureOn(t, liveStore(t))

	first, _ := entryByName(m.Render(), "todo.txt")
	if first.Size != int64(len("buy milk")) {
		t.Fatalf("initial size = %d", first.Size)
	}

	grown := "buy milk and eggs and bread"
	putSourceFile(t, st, "todo.txt", []byte(grown))

	deadline := time.After(3 * time.Second)
	for {
		e, _ := entryByName(m.Render(), "todo.txt")
		if e.Size == int64(len(grown)) {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("size still %d after 3s, want %d — the cache was not invalidated",
				e.Size, len(grown))
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// awaitEntry polls Render until a named entry appears. Bounded, so a dead
// subscription fails rather than hanging (AP32: wait on the thing that
// actually changes, with a deadline).
func awaitEntry(t *testing.T, m *FileExplorerModel, name string) ExplorerOutput {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		out := m.Render()
		if _, ok := entryByName(out, name); ok {
			return out
		}
		select {
		case <-deadline:
			t.Fatalf("%q never appeared within 3s — the subscription is not maintaining the index", name)
			return out
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// TestExplorer_CloseDoesNotDeadlockUnderDelivery — the regression gate
// for a hang, which is the failure mode that does not announce itself.
//
// The SDK's cancel closes the watch and then WAITS for its delivery
// goroutine to exit. Cancelling while holding the model's lock deadlocks
// against a delivery already sitting in onSourceEvent, and Go mutexes are
// not reentrant. The first version of this model did that, and it did not
// fail — `make test-each` sat on the workbench suite for sixteen minutes
// and would eventually have reported a 30-minute timeout naming no test.
//
// **The window is narrow, so this closes it by repetition, not by one
// attempt.** A single close-under-churn passes against the deadlocking
// version — measured, with that version restored — because the deliverer
// has to be blocked on the mutex at the instant the closer takes it.
// Opening and closing many models against a continuously-written store
// hits it. Each iteration is bounded, so a deadlock reports in a second
// with a sentence rather than stalling the sweep.
func TestExplorer_CloseDoesNotDeadlockUnderDelivery(t *testing.T) {
	st, seed := explorerFixtureOn(t, liveStore(t))
	seed.Close()

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			putSourceFile(t, st, "churn.txt", []byte(strings.Repeat("x", i%64+1)))
		}
	}()
	defer func() { close(stop); <-done }()

	for i := 0; i < 40; i++ {
		m := NewFileExplorerModel(st)
		if err := m.SetRoot(testRoot); err != nil {
			t.Fatalf("iteration %d: SetRoot: %v", i, err)
		}
		// Give the subscription a moment to have a delivery in flight,
		// so Close lands while the deliverer is contending for the lock
		// rather than against an idle subscription.
		time.Sleep(time.Millisecond)

		closed := make(chan struct{})
		go func() { defer close(closed); m.Close() }()
		select {
		case <-closed:
		case <-time.After(2 * time.Second):
			t.Fatalf("iteration %d: Close did not return within 2s — deadlocked against "+
				"the subscription's delivery goroutine (cancelling while holding m.mu)", i)
		}
	}
}

// TestExplorer_WakeSurvivesAMountSwitch — SetRoot re-binds the
// subscriptions, and a renderer's wake registration must outlive the
// mount it was watching. Clearing it there would leave a panel
// live-updating for exactly one mount and silently static for every mount
// the operator picked afterwards — a defect with no symptom except that
// the view stops being current.
func TestExplorer_WakeSurvivesAMountSwitch(t *testing.T) {
	st, m := explorerFixtureOn(t, liveStore(t))

	var mu sync.Mutex
	woke := 0
	m.OnChange(func() { mu.Lock(); woke++; mu.Unlock() })

	// Re-bind to the same root: the subscriptions are torn down and
	// rebuilt, which is the operation that used to drop the callback.
	if err := m.SetRoot(testRoot); err != nil {
		t.Fatal(err)
	}

	putSourceFile(t, st, "after-switch.txt", []byte("hello"))

	deadline := time.After(3 * time.Second)
	for {
		mu.Lock()
		n := woke
		mu.Unlock()
		if n > 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("no wake fired after a re-bind — the renderer's callback was dropped by SetRoot")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
