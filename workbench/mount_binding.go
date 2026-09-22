package workbench

import (
	"fmt"
	"sort"
	"strings"

	"go.entitychurch.org/entity-core-go/core/ecf"
)

// mount_binding.go — the durable half of a mount that the kernel does
// not hold.
//
// # The gap this closes
//
// A mount is two records in two places, and only one of them survived a
// restart:
//
//	system/config/local/files/{root}   KERNEL. Filesystem root, source
//	                                   prefix, include/exclude. Persisted
//	                                   by localfiles.Handler.AddRoot and
//	                                   rehydrated by its Load(), which
//	                                   shellboot calls at startup — so the
//	                                   watcher comes back by itself.
//	app/workbench/mounts/{root}        OURS. The source→target mapping the
//	                                   ingest handler routes on. Until this
//	                                   file, it existed ONLY in
//	                                   NotificationIngestHandler's memory,
//	                                   written by Mount and by nothing else.
//
// So after a restart the watcher resumed, wrote `local/files/file`
// entities for every change, the subscription delivered them — and the
// ingest handler answered `404 no_mount_for_uri`, because the map it
// routes on was empty. Every document stopped appearing, and the only
// visible symptom was that nothing new showed up. `mounts` still listed
// the mount, because that reads the kernel's record. The mount looked
// completely healthy and had lost half of itself.
//
// The kernel could not fix this for us and should not: the target prefix
// is a workbench-application concept that `ext/localfiles` has never
// heard of. The record belongs in our namespace, and `app/workbench/` is
// the namespace AGENTS.md reserves for exactly this.
//
// # Why a second consumer made it urgent rather than the restart
//
// Worth being honest about how this was found: not by a restart, but by
// [FileExplorerModel] needing the target prefix in order to say whether a
// file had become a document. The only source for it was the ingest
// handler's memory, which a model has no business reaching into. The
// choice was to guess the prefix, thread it through every caller, or
// persist it — and once persisted, the restart bug it also fixes became
// visible. **A fact held in exactly one process's memory reads as
// "internal detail" until a second consumer asks for it.**

// MountBindingPrefix is where the workbench-application half of each
// mount lives. Under `app/workbench/` — the application namespace, which
// extends freely, as opposed to anything `system/*` which is
// spec-adjacent and would need cross-impl coordination.
const MountBindingPrefix = "app/workbench/mounts/"

// MountBindingType is the entity type at MountBindingPrefix.
const MountBindingType = "workbench/mount-binding"

// MountBindingData is the workbench-owned record of one mount.
//
// SourcePrefix is duplicated from the kernel's RootConfig on purpose: it
// is the key the ingest handler routes on, and a record that carried only
// the target would force every reader to join against the kernel's config
// to find out what it applied to. The two are written together and by one
// caller, so they cannot drift within a mount's lifetime — and if the
// kernel's config is gone, this row describing a mount that no longer
// exists is itself the useful signal.
type MountBindingData struct {
	Root         string `cbor:"root"`
	SourcePrefix string `cbor:"source_prefix"`
	TargetPrefix string `cbor:"target_prefix"`
}

// SaveMountBinding persists the workbench half of a mount.
func SaveMountBinding(st *Store, b MountBindingData) error {
	if st == nil {
		return fmt.Errorf("no store")
	}
	if b.Root == "" {
		return fmt.Errorf("mount binding needs a root name")
	}
	b.SourcePrefix = ensureSlash(b.SourcePrefix)
	b.TargetPrefix = ensureSlash(b.TargetPrefix)
	if _, err := st.Put(MountBindingPrefix+b.Root, MountBindingType, b); err != nil {
		return fmt.Errorf("persist mount binding %s: %w", b.Root, err)
	}
	return nil
}

// LoadMountBinding reads one mount's binding.
func LoadMountBinding(st *Store, root string) (MountBindingData, bool) {
	if st == nil || root == "" {
		return MountBindingData{}, false
	}
	ent, ok := st.Get(MountBindingPrefix + root)
	if !ok || ent.Type != MountBindingType {
		return MountBindingData{}, false
	}
	var b MountBindingData
	if err := ecf.Decode(ent.Data, &b); err != nil {
		return MountBindingData{}, false
	}
	return b, true
}

// RemoveMountBinding drops one mount's binding. Reports whether
// something was there.
func RemoveMountBinding(st *Store, root string) bool {
	if st == nil || root == "" {
		return false
	}
	return st.Remove(MountBindingPrefix + root)
}

// LoadMountBindings reads every binding, sorted by root.
//
// Returns the rows it could decode and a separate list of the ones it
// could not, rather than failing the whole read on one bad row. A single
// corrupt binding must not take every other mount down with it — but it
// must also not vanish, which is why the problems come back as values
// instead of being logged and dropped (AP33).
func LoadMountBindings(st *Store) (bindings []MountBindingData, problems []string) {
	if st == nil {
		return nil, nil
	}
	for _, e := range st.List(MountBindingPrefix) {
		root, under := RelativeUnder(e.Path, MountBindingPrefix)
		if !under || root == "" || strings.Contains(root, "/") {
			continue
		}
		ent, ok := st.Get(e.Path)
		if !ok {
			problems = append(problems, root+": listed but not resolvable")
			continue
		}
		if ent.Type != MountBindingType {
			problems = append(problems, root+": unexpected type "+ent.Type)
			continue
		}
		var b MountBindingData
		if err := ecf.Decode(ent.Data, &b); err != nil {
			problems = append(problems, root+": did not decode: "+err.Error())
			continue
		}
		if b.Root == "" {
			b.Root = root
		}
		bindings = append(bindings, b)
	}
	sort.Slice(bindings, func(i, j int) bool { return bindings[i].Root < bindings[j].Root })
	sort.Strings(problems)
	return bindings, problems
}

// RestoreMountBindings re-registers every persisted mount with the
// ingest handler. Call once at startup, after the localfiles handler's
// own Load has rehydrated the kernel side.
//
// Returns how many were restored and a human-readable line per binding
// that could not be. The problems are RETURNED rather than logged
// because the caller is startup code with a stderr an operator reads —
// and a mount that silently fails to come back is precisely the failure
// this whole file exists to end.
func RestoreMountBindings(st *Store, ingest *NotificationIngestHandler) (restored int, problems []string) {
	if st == nil || ingest == nil {
		return 0, nil
	}
	bindings, problems := LoadMountBindings(st)
	for _, b := range bindings {
		if b.SourcePrefix == "" || b.TargetPrefix == "" {
			problems = append(problems,
				b.Root+": binding has an empty prefix, cannot route (source="+
					b.SourcePrefix+" target="+b.TargetPrefix+")")
			continue
		}
		ingest.RegisterMount(b.SourcePrefix, b.TargetPrefix)
		restored++
	}
	return restored, problems
}
