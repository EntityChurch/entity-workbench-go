package workbench

import (
	"sort"
	"strings"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/ext/localfiles"
)

// local_files_model.go — the renderer-neutral view of this peer's
// filesystem mounts.
//
// # Why this file exists
//
// `ext/localfiles` is one of the most complete things in the kernel —
// filesystem→tree ingest, tree→filesystem writeback, an fsnotify watcher,
// a stat cache implementing Git's racy-clean rule, path containment with a
// leaf-symlink refusal — and until this model landed **no renderer in this
// repo could see any of it**. `grep LocalFiles avalonia/ console/` returned
// nothing. That is D23's exact shape for the fourth time: a complete model
// with no user-reachable edge, every layer green, and the defect being the
// absence of a connection between two correct layers.
//
// # Where the rows come from
//
// `Handler.AddRoot` persists each mount's [localfiles.RootConfigData] at
// `system/config/local/files/{root}` — the handler's own config namespace,
// which is also what gives mounts their restart-equivalence. We read that
// namespace rather than keeping a parallel workbench-level mount list,
// because a second list is a second thing to get out of sync.
//
// This is an L0 store read of **our own** config namespace, which is the
// sanctioned back door (D1) and the same thing `shellcmd`'s `mounts` verb
// does. It is not a dispatched read: a peer-qualified path would route to
// that peer and answer about *their* tree (AP11), which is not the question.
//
// # Watcher status — a tree fact since 2026-09-17, and this file said
// otherwise for as long as it was not
//
// [localfiles.WatcherConfigData] carries exactly the field a panel wants
// (`active` / `stopped` / `error` plus a message). It used to be built only
// as the *response* to a `watch` operation, with the live set in an
// unexported map and no accessor — so a mount row could say what was
// configured and not whether it was running, and [MountRow.WatcherObservable]
// was hard `false` so a renderer could say *unknown* rather than imply *fine*.
// Reporting "active" because a config row exists would have invented a fact
// the tree did not carry.
//
// core-go closed that ask (their tracker row 3) the better way than we asked
// for: `Handler.persistWatcherState` writes the same shape to
// `system/config/local/files/watch/{root}` on **every** path that starts,
// stops or faults a watcher — the auto/restart path included — so watcher
// liveness is a tree fact any implementation reads, not a Go-API affordance.
// We read it here.
//
// ⚠ **The stale half is the part worth keeping in view.** The paragraph above
// survived in this doc comment, and in a test asserting `WatcherObservable`
// stays false, for as long as it took someone to look — the test kept passing
// *because* it pinned the blocked state, which is the one failure a
// blocked-state gate must not have (a gate that goes on reporting "still
// blocked" after the blocker lifts). It is replaced with the property, and the
// arm that would have caught the drift is the one asserting a row DOES report
// a status when the tree carries one.
//
// # Three states, not two
//
// `WatcherObservable` false means **no watch record exists for this root** —
// which is a real and different thing from `stopped`. A mount whose handler
// never started a watcher and a mount whose watcher was deliberately stopped
// send an operator to different places, and collapsing them into one "not
// running" is how the first gets diagnosed as the second. A renderer must not
// default an absent record to any status value.

// MountRow is one filesystem mount as the tree records it.
//
// Every field except FileCount and Err is read straight out of the
// persisted RootConfig, so a row describes what a restart would restore.
type MountRow struct {
	// Root is the mount's name — the last segment of its config path.
	Root string
	// FilesystemRoot is the absolute on-disk directory, cleaned by
	// AddRoot before it was persisted.
	FilesystemRoot string
	// Prefix is the tree prefix the directory ingests into, e.g.
	// "local/files/notes/".
	Prefix string
	// ReadOnly suppresses tree→filesystem writeback for this mount.
	ReadOnly bool
	// Include admits only matching files; empty means "everything not
	// excluded". Exclude applies to files *and* directories, so an
	// excluded directory is never descended into.
	Include []string
	Exclude []string
	// PublishDescriptors emits system/content/descriptor entities on
	// read (DOMAIN-LOCAL-FILES §10.5 V3).
	PublishDescriptors bool

	// ConfigPath is where this row was read from, so an operator can go
	// look at the entity behind it.
	ConfigPath string

	// FileCount is how many entities currently live under Prefix.
	// See [LocalFilesModel.Render] for what it costs and what it means
	// when Prefix is empty.
	FileCount int

	// WatcherObservable reports whether the tree carries a watch record
	// for this root at all. False means **no record**, which is not the
	// same claim as `stopped` — see the file note. A renderer that reads
	// WatcherStatus without checking this one first turns "never started"
	// into whatever the zero value happens to look like.
	WatcherObservable bool
	// WatcherStatus is the kernel's own value — `active`, `stopped` or
	// `error` — and is empty exactly when WatcherObservable is false.
	WatcherStatus string
	// WatcherError is the kernel's message on an `error` status, and is
	// the only thing that says *why* a mount stopped producing documents.
	WatcherError string

	// Err is set when this row's config entity did not decode. The row
	// still appears — a mount whose config is unreadable is exactly the
	// thing an operator needs shown, and dropping it would report a
	// broken mount as no mount at all.
	Err string
}

// LocalFilesOutput is the renderer-neutral output of the model.
type LocalFilesOutput struct {
	// Mounts is sorted by root name so a re-render does not reorder the
	// list under the reader (AP49 — an unconditional reshuffle is a
	// correctness surface, not a cosmetic one).
	Mounts []MountRow

	// Note explains an empty list instead of leaving the renderer to
	// present "no mounts" and "the extension is off" identically. An
	// empty result is a claim, and the two claims are different.
	Note string
}

// LocalFilesModel renders this peer's filesystem mounts.
//
// Stateless by construction: it holds a store handle and reads the config
// namespace on demand. There is no subscription and no cached snapshot,
// because mounts change when an operator runs a verb — on the order of
// once a session — and a subscription would be machinery maintained for an
// event that effectively does not arrive. `MarkdownFilesModel` subscribes
// because *file* entities churn; mount *configs* do not.
type LocalFilesModel struct {
	store *Store
}

// NewLocalFilesModel binds a model to a peer's store.
func NewLocalFilesModel(store *Store) *LocalFilesModel {
	return &LocalFilesModel{store: store}
}

// MountConfigPrefix is the localfiles handler's own config namespace.
// Named once here because three call sites reconstructing the same string
// literal is how one of them ends up spelled differently.
const MountConfigPrefix = "system/config/local/files/"

// MountWatchPrefix is where `ext/localfiles` persists each root's watcher
// liveness (`Handler.persistWatcherState`). It sits *inside*
// [MountConfigPrefix], which is why [LocalFilesModel.Render]'s "a nested
// path under our namespace is not a mount" filter is load-bearing rather
// than defensive: without it every watch record would render as a mount
// named `watch/{root}`.
const MountWatchPrefix = MountConfigPrefix + "watch/"

// Render reads the config namespace and returns one row per mount.
//
// Cost: one prefix-scoped List for the mount set, then per mount one Get
// (its config entity) and one prefix-scoped List (its file count). Both
// Lists are location-index prefix scans, not whole-tree iterations — the
// anti-pattern this repo names is scan-and-filter, and neither call does
// that. FileCount is nonetheless O(files under the mount) and is the one
// thing here that grows with the mount's size; a renderer that wants to
// poll rather than refresh on demand should reconsider it first.
func (m *LocalFilesModel) Render() LocalFilesOutput {
	if m.store == nil {
		return LocalFilesOutput{Note: "no peer store — the model has nothing to read"}
	}

	entries := m.store.List(MountConfigPrefix)
	if len(entries) == 0 {
		return LocalFilesOutput{
			Note: "no filesystem mounts on this peer — `mount <dir> <tree-prefix>` creates one",
		}
	}

	rows := make([]MountRow, 0, len(entries))
	for _, e := range entries {
		// TreeRelative first: List hands back PEER-QUALIFIED paths, so a
		// bare TrimPrefix leaves the whole `/{peer-id}/...` string and the
		// Contains check below then rejects every real mount. That was
		// live — the panel reported "no filesystem mounts" for a peer
		// that had one. See workbench/tree_path.go.
		root, under := RelativeUnder(e.Path, MountConfigPrefix)
		if !under || root == "" || strings.Contains(root, "/") {
			// Not a mount config: the namespace is ours, but a nested
			// path under it is something else's business and guessing
			// at its shape is how a panel starts rendering strangers.
			continue
		}
		row := MountRow{Root: root, ConfigPath: e.Path}

		ent, ok := m.store.Get(e.Path)
		if !ok {
			row.Err = "config entity listed but not resolvable"
			rows = append(rows, row)
			continue
		}
		cfg, err := localfiles.RootConfigDataFromEntity(ent)
		if err != nil {
			row.Err = "config did not decode: " + err.Error()
			rows = append(rows, row)
			continue
		}

		row.FilesystemRoot = cfg.FilesystemRoot
		row.Prefix = cfg.Prefix
		row.ReadOnly = cfg.ReadOnly
		row.Include = cfg.Include
		row.Exclude = cfg.Exclude
		row.PublishDescriptors = cfg.PublishDescriptors
		if cfg.Prefix != "" {
			row.FileCount = len(m.store.List(cfg.Prefix))
		}
		if wc, ok := m.watcherStateFor(root); ok {
			row.WatcherObservable = true
			row.WatcherStatus = wc.Status
			row.WatcherError = wc.ErrorMessage
		}
		rows = append(rows, row)
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].Root < rows[j].Root })
	return LocalFilesOutput{Mounts: rows}
}

// watcherStateFor reads one root's persisted watcher liveness.
//
// The decode is done here rather than through a kernel helper because
// `ext/localfiles` exports `RootConfigDataFromEntity` and has no
// `WatcherConfigDataFromEntity` beside it — the type was originally only ever
// an operation *response*, which needed no decoder. Filed as a small ask
// rather than worked around silently; `ecf.Decode` into their own struct is
// their idiom verbatim (`ext/localfiles/types.go:132-138`), so this cannot
// drift from the shape they write.
//
// A record that does not decode reports ABSENT rather than a zero-valued
// status: a malformed watch record tells us nothing about the watcher, and
// inventing `stopped` from it would be the tolerant-fallback shape (AP33)
// applied to a liveness claim.
func (m *LocalFilesModel) watcherStateFor(root string) (localfiles.WatcherConfigData, bool) {
	ent, ok := m.store.Get(MountWatchPrefix + root)
	if !ok {
		return localfiles.WatcherConfigData{}, false
	}
	var wc localfiles.WatcherConfigData
	if err := ecf.Decode(ent.Data, &wc); err != nil {
		return localfiles.WatcherConfigData{}, false
	}
	if wc.Status == "" {
		return localfiles.WatcherConfigData{}, false
	}
	return wc, true
}

// MountFilesystemRoot reads the on-disk directory one mount is bound to,
// straight from the kernel's config entity.
//
// Exported because a Folder record has to carry the path an operator
// chose, and the ONLY durable source for it is this config — the
// workbench-side mount binding stores prefixes, not paths. A record that
// omitted the path would be unable to say where a folder's files are
// after the mount that knew is gone, which is precisely the state a
// reader needs it in.
func MountFilesystemRoot(st *Store, root string) (string, bool) {
	if st == nil || root == "" {
		return "", false
	}
	ent, ok := st.Get(MountConfigPrefix + root)
	if !ok {
		return "", false
	}
	cfg, err := localfiles.RootConfigDataFromEntity(ent)
	if err != nil {
		return "", false
	}
	return cfg.FilesystemRoot, cfg.FilesystemRoot != ""
}
