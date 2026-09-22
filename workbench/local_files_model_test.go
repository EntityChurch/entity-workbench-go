package workbench

import (
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/store"
	"go.entitychurch.org/entity-core-go/ext/localfiles"
)

// newMountTestStore builds an empty in-memory store for mount tests.
func newMountTestStore() *Store {
	return NewStore(store.NewMemoryContentStore(), store.NewMemoryLocationIndex())
}

// putMount persists a RootConfig exactly where Handler.AddRoot puts it,
// so these tests read the same bytes the real handler writes rather than
// a shape invented here. If AddRoot's path ever moves, this stops
// matching and the model's read is wrong — which is the point.
func putMount(t *testing.T, st *Store, root string, cfg localfiles.RootConfigData) {
	t.Helper()
	ent, err := cfg.ToEntity()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Put(MountConfigPrefix+root, ent.Type, cfg); err != nil {
		t.Fatal(err)
	}
}

// An empty config namespace is a claim, and the two claims a renderer
// could make about it are different. The model must say which.
func TestLocalFilesModel_EmptyIsExplained(t *testing.T) {
	m := NewLocalFilesModel(newMountTestStore())
	out := m.Render()

	if len(out.Mounts) != 0 {
		t.Fatalf("expected no mounts, got %d", len(out.Mounts))
	}
	if out.Note == "" {
		t.Error("an empty mount list with no note renders identically to a broken read; " +
			"AP44's lesson is that an empty result is a claim")
	}
}

// A nil store must not panic a panel. Renderers construct models before
// a peer exists more often than anyone intends.
func TestLocalFilesModel_NilStoreIsNoted(t *testing.T) {
	m := NewLocalFilesModel(nil)
	out := m.Render()
	if len(out.Mounts) != 0 || out.Note == "" {
		t.Errorf("nil store should render zero rows with a note; got %d rows, note %q",
			len(out.Mounts), out.Note)
	}
}

// The whole point of the model: every field an operator needs comes off
// the persisted config, not out of a second workbench-side list.
func TestLocalFilesModel_DecodesThePersistedConfig(t *testing.T) {
	st := newMountTestStore()
	putMount(t, st, "notes", localfiles.RootConfigData{
		Prefix:             "local/files/notes/",
		FilesystemRoot:     "/home/me/notes",
		ReadOnly:           true,
		Include:            []string{"*.md"},
		Exclude:            []string{"vendor", "*.log"},
		PublishDescriptors: true,
	})

	out := mountModel(st).Render()
	if len(out.Mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d (note %q)", len(out.Mounts), out.Note)
	}
	row := out.Mounts[0]

	if row.Root != "notes" {
		t.Errorf("Root = %q, want notes", row.Root)
	}
	if row.FilesystemRoot != "/home/me/notes" {
		t.Errorf("FilesystemRoot = %q", row.FilesystemRoot)
	}
	if row.Prefix != "local/files/notes/" {
		t.Errorf("Prefix = %q", row.Prefix)
	}
	if !row.ReadOnly {
		t.Error("ReadOnly lost — a read-only mount rendered as writable is a lie about what " +
			"the peer will do to the operator's disk")
	}
	if strings.Join(row.Include, ",") != "*.md" {
		t.Errorf("Include = %v", row.Include)
	}
	if strings.Join(row.Exclude, ",") != "vendor,*.log" {
		t.Errorf("Exclude = %v", row.Exclude)
	}
	if !row.PublishDescriptors {
		t.Error("PublishDescriptors lost")
	}
	if row.ConfigPath != MountConfigPrefix+"notes" {
		t.Errorf("ConfigPath = %q", row.ConfigPath)
	}
	if row.Err != "" {
		t.Errorf("unexpected Err %q", row.Err)
	}
}

// FileCount is scoped to the mount's own prefix. A count that leaked
// entities from a sibling mount would overstate every row.
func TestLocalFilesModel_FileCountIsScopedToTheMount(t *testing.T) {
	st := newMountTestStore()
	putMount(t, st, "a", localfiles.RootConfigData{
		Prefix: "local/files/a/", FilesystemRoot: "/tmp/a",
	})
	putMount(t, st, "b", localfiles.RootConfigData{
		Prefix: "local/files/b/", FilesystemRoot: "/tmp/b",
	})
	for _, p := range []string{"local/files/a/one", "local/files/a/two", "local/files/b/only"} {
		if _, err := st.Put(p, "doc/markdown-file", map[string]interface{}{"body": "x"}); err != nil {
			t.Fatal(err)
		}
	}

	out := mountModel(st).Render()
	if len(out.Mounts) != 2 {
		t.Fatalf("expected 2 mounts, got %d", len(out.Mounts))
	}
	// Sorted by root name, so a then b — which is itself the assertion
	// that ordering is stable rather than map-iteration order.
	if out.Mounts[0].Root != "a" || out.Mounts[1].Root != "b" {
		t.Fatalf("rows not sorted by root: %q, %q", out.Mounts[0].Root, out.Mounts[1].Root)
	}
	if out.Mounts[0].FileCount != 2 {
		t.Errorf("mount a FileCount = %d, want 2", out.Mounts[0].FileCount)
	}
	if out.Mounts[1].FileCount != 1 {
		t.Errorf("mount b FileCount = %d, want 1", out.Mounts[1].FileCount)
	}
}

// A mount whose config will not decode is exactly what an operator needs
// shown. Dropping the row reports a broken mount as no mount at all.
func TestLocalFilesModel_UndecodableConfigStillRenders(t *testing.T) {
	st := newMountTestStore()
	if _, err := st.Put(MountConfigPrefix+"broken", "local/files/root-config",
		map[string]interface{}{"prefix": 42}); err != nil {
		t.Fatal(err)
	}

	out := mountModel(st).Render()
	if len(out.Mounts) != 1 {
		t.Fatalf("a mount with an unreadable config must still appear; got %d rows", len(out.Mounts))
	}
	if out.Mounts[0].Err == "" {
		t.Error("expected the row to carry the decode failure rather than render as a healthy mount")
	}
	if out.Mounts[0].Root != "broken" {
		t.Errorf("Root = %q, want broken", out.Mounts[0].Root)
	}
}

// putWatch persists a WatcherConfig exactly where Handler.persistWatcherState
// puts it, so these tests read the bytes the real handler writes rather than a
// shape invented here.
func putWatch(t *testing.T, st *Store, root string, wc localfiles.WatcherConfigData) {
	t.Helper()
	ent, err := wc.ToEntity()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Put(MountWatchPrefix+root, ent.Type, wc); err != nil {
		t.Fatal(err)
	}
}

// Watcher liveness IS in the tree now (core-go row 3), and this is the arm
// that would have caught the four months in which this file asserted the
// opposite: it fails against a model that never learned to read the record.
//
// It replaces `TestLocalFilesModel_DoesNotClaimWatcherLiveness`, which pinned
// the blocked state and therefore kept passing after the blocker lifted — the
// one failure mode a blocked-state gate must not have.
func TestLocalFilesModel_ReportsWatcherLivenessFromTheTree(t *testing.T) {
	st := newMountTestStore()
	putMount(t, st, "notes", localfiles.RootConfigData{
		Prefix: "local/files/notes/", FilesystemRoot: "/home/me/notes",
	})
	putWatch(t, st, "notes", localfiles.WatcherConfigData{RootName: "notes", Status: "active"})

	row := mountModel(st).Render().Mounts[0]
	if !row.WatcherObservable {
		t.Fatal("a root with a persisted watch record must report as observable; " +
			"core-go's Handler.persistWatcherState writes system/config/local/files/watch/{root} " +
			"on every start/stop/error path")
	}
	if row.WatcherStatus != "active" {
		t.Errorf("WatcherStatus = %q, want the kernel's own value \"active\"", row.WatcherStatus)
	}
}

// An `error` status is the only thing that says WHY a mount stopped
// producing documents, so the message has to survive to the row.
func TestLocalFilesModel_CarriesTheWatcherErrorMessage(t *testing.T) {
	st := newMountTestStore()
	putMount(t, st, "notes", localfiles.RootConfigData{
		Prefix: "local/files/notes/", FilesystemRoot: "/home/me/notes",
	})
	putWatch(t, st, "notes", localfiles.WatcherConfigData{
		RootName: "notes", Status: "error", ErrorMessage: "inotify watch limit reached",
	})

	row := mountModel(st).Render().Mounts[0]
	if row.WatcherStatus != "error" {
		t.Fatalf("WatcherStatus = %q, want error", row.WatcherStatus)
	}
	if !strings.Contains(row.WatcherError, "inotify") {
		t.Errorf("WatcherError = %q; an error status with no message tells an operator "+
			"a watcher failed and nothing about what to do", row.WatcherError)
	}
}

// ⭐ The control arm, and the load-bearing one: NO record is a third state,
// not `stopped`. A mount whose watcher never started and one deliberately
// stopped send an operator to different places.
//
// Without this arm the two tests above are satisfied by a model that reports
// `observable` unconditionally — which is the invented fact the old
// hard-`false` existed to prevent, arriving from the opposite direction.
func TestLocalFilesModel_AbsentWatchRecordIsNotStopped(t *testing.T) {
	st := newMountTestStore()
	putMount(t, st, "notes", localfiles.RootConfigData{
		Prefix: "local/files/notes/", FilesystemRoot: "/home/me/notes",
	})

	row := mountModel(st).Render().Mounts[0]
	if row.WatcherObservable {
		t.Error("a root with NO watch record must not report as observable")
	}
	if row.WatcherStatus != "" {
		t.Errorf("WatcherStatus = %q, want empty: an absent record is not a status, and "+
			"defaulting it to one turns \"never started\" into \"stopped\"", row.WatcherStatus)
	}
}

// A watch record is not a mount. The nested-path filter is what stops every
// `watch/{root}` entity rendering as a mount of its own — it sits inside the
// same config namespace, so this is the filter earning its keep rather than
// being defensive.
func TestLocalFilesModel_WatchRecordsDoNotRenderAsMounts(t *testing.T) {
	st := newMountTestStore()
	putMount(t, st, "notes", localfiles.RootConfigData{
		Prefix: "local/files/notes/", FilesystemRoot: "/home/me/notes",
	})
	putWatch(t, st, "notes", localfiles.WatcherConfigData{RootName: "notes", Status: "active"})

	out := mountModel(st).Render()
	if len(out.Mounts) != 1 || out.Mounts[0].Root != "notes" {
		roots := make([]string, 0, len(out.Mounts))
		for _, m := range out.Mounts {
			roots = append(roots, m.Root)
		}
		t.Fatalf("expected exactly the one real mount; got roots %v", roots)
	}
}

// A nested path under the config namespace is not a mount. Rendering
// strangers is how a panel starts showing rows nobody can act on.
func TestLocalFilesModel_IgnoresNestedNonMountPaths(t *testing.T) {
	st := newMountTestStore()
	putMount(t, st, "real", localfiles.RootConfigData{
		Prefix: "local/files/real/", FilesystemRoot: "/tmp/real",
	})
	if _, err := st.Put(MountConfigPrefix+"real/extra/thing", "doc/markdown-file",
		map[string]interface{}{"body": "x"}); err != nil {
		t.Fatal(err)
	}

	out := mountModel(st).Render()
	if len(out.Mounts) != 1 || out.Mounts[0].Root != "real" {
		t.Fatalf("expected only the real mount; got %+v", out.Mounts)
	}
}

// mountModel is a tiny constructor alias to keep the assertions above readable.
func mountModel(st *Store) *LocalFilesModel { return NewLocalFilesModel(st) }
