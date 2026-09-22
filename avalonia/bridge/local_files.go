package main

// Local-files bridge surface. Wraps workbench's LocalFilesModel — this
// peer's filesystem mounts, read out of the localfiles handler's own
// config namespace (`system/config/local/files/{root}`).
//
// **Why this is one call and not an Open/Wake/Render/Close handle.**
// Every other panel surface here holds a handle because its model owns a
// subscription and needs a wake: a liveness transition, a connection
// event, a tree change. Mounts are not that. A mount config is written
// when an operator runs `mount` and then does not move — on the order of
// once a session — and the model is stateless, so a render is a
// prefix-scoped List plus one Get per mount. Holding a handle open for
// that would be machinery maintained for an event that does not arrive.
//
// If mounts ever gain a real event source — a watcher-status feed is the
// obvious candidate, and see below for why that does not exist yet — this
// should grow the handle shape rather than start polling.
//
// **What this surface cannot tell you: whether a watcher is running.**
// `localfiles.WatcherConfigData` carries exactly that (`active` /
// `stopped` / `error` plus a message) and is built as the *response* to a
// `watch` operation, never written to a tree path; the handler keeps the
// live set in an unexported map with no accessor. So a row says what was
// configured, not what is running, and `watcherObservable` is false on
// every row. A panel must render that as *unknown* rather than let a
// reader infer that a configured mount is a live one — the AP45 rule that
// a surface must say what it does not know, rather than let the absence
// read as fine.
//
// Note on the preamble below: cgo compiles the comment block
// IMMEDIATELY preceding `import "C"` as C. Prose there is fed to gcc,
// which reports "stray '`' in program" on backticks and chokes on an
// em-dash — so package documentation must be separated from the import
// by the real preamble (or a blank line), and the preamble itself has to
// carry stdint.h for C.int64_t and stdlib.h for C.CString.

/*
#include <stdlib.h>
#include <stdint.h>
*/
import "C"

import (
	"encoding/json"
	"strings"

	"entity-workbench-go/shellcmd"
	wb "entity-workbench-go/workbench"
)

// localFilesMountDTO is one mount as the C# panel receives it.
//
// Explicit and flat rather than marshalling wb.MountRow directly: the
// wire shape between Go and the panel is a contract, and AP49 is what
// happens when it drifts silently — the panel there declared two fewer
// fields than the model sent and System.Text.Json discarded them without
// a word. A named DTO makes the field set reviewable in one place.
type localFilesMountDTO struct {
	Root               string   `json:"root"`
	FilesystemRoot     string   `json:"filesystemRoot"`
	Prefix             string   `json:"prefix"`
	ReadOnly           bool     `json:"readOnly"`
	Include            []string `json:"include"`
	Exclude            []string `json:"exclude"`
	PublishDescriptors bool     `json:"publishDescriptors"`
	ConfigPath         string   `json:"configPath"`
	FileCount          int      `json:"fileCount"`
	WatcherObservable  bool     `json:"watcherObservable"`
	Err                string   `json:"err"`
}

type localFilesRenderDTO struct {
	OK     bool                 `json:"ok"`
	Mounts []localFilesMountDTO `json:"mounts"`
	Note   string               `json:"note"`
}

//export LocalFilesRender
func LocalFilesRender(peerHandle C.int64_t) (result *C.char) {
	defer recoverToErrorEnvelope("LocalFilesRender", &result)
	if manager == nil {
		return C.CString(errNotInit)
	}
	hp := manager.Get(int64(peerHandle))
	if hp == nil {
		return C.CString(errBadPeer)
	}

	out := wb.NewLocalFilesModel(hp.AppPeer.Store()).Render()

	// Non-nil so the panel receives `[]` rather than `null` for an empty
	// mount set. A null here deserializes to a null List<T> and the
	// panel's first foreach is a NullReferenceException — which, per
	// AP46, would surface as a process abort rather than an exception.
	dto := localFilesRenderDTO{OK: true, Mounts: []localFilesMountDTO{}, Note: out.Note}
	for _, mnt := range out.Mounts {
		inc, exc := mnt.Include, mnt.Exclude
		if inc == nil {
			inc = []string{}
		}
		if exc == nil {
			exc = []string{}
		}
		dto.Mounts = append(dto.Mounts, localFilesMountDTO{
			Root:               mnt.Root,
			FilesystemRoot:     mnt.FilesystemRoot,
			Prefix:             mnt.Prefix,
			ReadOnly:           mnt.ReadOnly,
			Include:            inc,
			Exclude:            exc,
			PublishDescriptors: mnt.PublishDescriptors,
			ConfigPath:         mnt.ConfigPath,
			FileCount:          mnt.FileCount,
			WatcherObservable:  mnt.WatcherObservable,
			Err:                mnt.Err,
		})
	}

	b, err := json.Marshal(dto)
	if err != nil {
		return C.CString(`{"ok":false,"error":"marshal local-files render"}`)
	}
	return C.CString(string(b))
}

// --- Mutating surface -----------------------------------------------
//
// Until this landed, `LocalFilesRender` above was the whole local-files
// bridge: the panel could list mounts and could not make one, so the
// only way to mount a directory in the shipped GUI was to open a Shell
// panel and type the verb. That is D23 with the model half-surfaced,
// and `make reachability` cannot see it — the sweep asks whether a model
// has *a* surface, not whether the surface can do what the model does.
//
// The operation itself is `shellcmd.ShellWorkspace.Mount`/`Unmount`, one
// implementation shared with the verb (see shellcmd/mount_op.go). None
// of the pipeline is reimplemented here; this file is a JSON envelope
// and nothing else, which is the "renderers are thin I/O" rule applied
// to the bridge rather than only to the panel.
//
// These are SYNCHRONOUS exports. Mount does real work — a filesystem
// walk at watcher start, a subscription, a capability mint — but it is
// operator-initiated, once per session, against a directory the operator
// just chose, and the alternative is the async handle shape whose whole
// justification is an event source that arrives without being asked for.
// Note the AP31 rule that governs the async form: an async cgo export
// must copy every C-owned argument into Go memory BEFORE launching the
// goroutine. Being synchronous is exactly why the strings below can be
// read directly.

// localFilesMountResultDTO is the mount reply. It carries the mount that
// now exists rather than echoing the request, so a panel renders facts.
type localFilesMountResultDTO struct {
	OK             bool     `json:"ok"`
	Error          string   `json:"error"`
	RootName       string   `json:"rootName"`
	FilesystemRoot string   `json:"filesystemRoot"`
	SourcePrefix   string   `json:"sourcePrefix"`
	TargetPrefix   string   `json:"targetPrefix"`
	Include        []string `json:"include"`
	Exclude        []string `json:"exclude"`
	CapabilityPath string   `json:"capabilityPath"`
	HandlerPattern string   `json:"handlerPattern"`
	SubscriptionID string   `json:"subscriptionId"`
	ReadOnly       bool     `json:"readOnly"`

	// Conflict is populated instead of a bare Error when the target
	// prefix already holds bindings this mount does not own. The panel
	// needs the structure, not the sentence: it shows what is in the way
	// and offers to proceed, which a formatted string cannot support.
	// This is why shellcmd.MountConflict is a typed error.
	Conflict *localFilesConflictDTO `json:"conflict"`
}

type localFilesConflictDTO struct {
	TargetPrefix   string         `json:"targetPrefix"`
	SourcePrefix   string         `json:"sourcePrefix"`
	ExpectedTypes  []string       `json:"expectedTypes"`
	TargetTotal    int            `json:"targetTotal"`
	TargetExpected int            `json:"targetExpected"`
	SourceTotal    int            `json:"sourceTotal"`
	ForeignOrder   []string       `json:"foreignOrder"`
	Foreign        map[string]int `json:"foreign"`
}

//export LocalFilesMount
func LocalFilesMount(peerHandle C.int64_t, fsDir *C.char, treePrefix *C.char,
	includeCSV *C.char, excludeCSV *C.char, excludeSet C.int64_t, force C.int64_t,
	readOnly C.int64_t) (result *C.char) {
	defer recoverToErrorEnvelope("LocalFilesMount", &result)
	if manager == nil {
		return C.CString(errNotInit)
	}
	hp := manager.Get(int64(peerHandle))
	if hp == nil {
		return C.CString(errBadPeer)
	}
	if hp.Workspace == nil {
		return C.CString(`{"ok":false,"error":"peer has no shell workspace"}`)
	}

	req := shellcmd.MountRequest{
		FilesystemDir: C.GoString(fsDir),
		TargetPrefix:  C.GoString(treePrefix),
		Include:       splitCSVPatterns(C.GoString(includeCSV)),
		Force:         int64(force) != 0,
		// Send-only, in Syncthing's vocabulary. The kernel has carried
		// this field from the start and no surface here could set it.
		ReadOnly: int64(readOnly) != 0,
	}
	// "said nothing" and "said none" are different intentions and the
	// request type keeps them apart; the panel decides which it means.
	if int64(excludeSet) != 0 {
		req.Exclude = splitCSVPatterns(C.GoString(excludeCSV))
		req.ExcludeSet = true
	}

	out, err := hp.Workspace.Mount(req)
	if err != nil {
		dto := localFilesMountResultDTO{OK: false, Error: err.Error()}
		if mc, ok := shellcmd.AsMountConflict(err); ok {
			dto.Conflict = &localFilesConflictDTO{
				TargetPrefix:   mc.TargetPrefix,
				SourcePrefix:   mc.SourcePrefix,
				ExpectedTypes:  nonNilStrings(mc.ExpectedTypes),
				TargetTotal:    mc.Result.TargetTotal,
				TargetExpected: mc.Result.TargetExpected,
				SourceTotal:    mc.Result.SourceTotal,
				ForeignOrder:   mc.Result.ForeignTypeOrder(),
				Foreign:        mc.Result.TargetForeign,
			}
		}
		return C.CString(marshalOrError(dto, "local-files mount"))
	}

	return C.CString(marshalOrError(localFilesMountResultDTO{
		OK:             true,
		RootName:       out.RootName,
		FilesystemRoot: out.FilesystemRoot,
		SourcePrefix:   out.SourcePrefix,
		TargetPrefix:   out.TargetPrefix,
		Include:        nonNilStrings(out.Include),
		Exclude:        nonNilStrings(out.Exclude),
		CapabilityPath: out.CapabilityPath,
		HandlerPattern: out.HandlerPattern,
		SubscriptionID: out.SubscriptionID,
		ReadOnly:       out.ReadOnly,
	}, "local-files mount"))
}

// localFilesUnmountResultDTO reports teardown INCLUDING what it could
// not do. WatcherStillRunning is not a detail to be tidied away: the
// kernel exposes StartWatching and no StopWatching, so the fsnotify
// watcher survives the unmount, and a panel that renders "unmounted"
// with nothing further has told the operator something untrue about
// where their edits will go.
type localFilesUnmountResultDTO struct {
	OK                   bool   `json:"ok"`
	Error                string `json:"error"`
	RootName             string `json:"rootName"`
	SubscriptionCloseErr string `json:"subscriptionCloseError"`
	WatcherStillRunning  bool   `json:"watcherStillRunning"`
}

//export LocalFilesUnmount
func LocalFilesUnmount(peerHandle C.int64_t, rootName *C.char) (result *C.char) {
	defer recoverToErrorEnvelope("LocalFilesUnmount", &result)
	if manager == nil {
		return C.CString(errNotInit)
	}
	hp := manager.Get(int64(peerHandle))
	if hp == nil {
		return C.CString(errBadPeer)
	}
	if hp.Workspace == nil {
		return C.CString(`{"ok":false,"error":"peer has no shell workspace"}`)
	}

	out, err := hp.Workspace.Unmount(C.GoString(rootName))
	if err != nil {
		return C.CString(marshalOrError(localFilesUnmountResultDTO{OK: false, Error: err.Error()}, "local-files unmount"))
	}
	dto := localFilesUnmountResultDTO{
		OK:                  true,
		RootName:            out.RootName,
		WatcherStillRunning: out.WatcherStillRunning,
	}
	if out.SubscriptionCloseErr != nil {
		dto.SubscriptionCloseErr = out.SubscriptionCloseErr.Error()
	}
	return C.CString(marshalOrError(dto, "local-files unmount"))
}

// localFilesSweepResultDTO is the reconciliation report. The removed
// path lists are carried in full rather than counted: a sweep DELETES
// bindings, and a surface that says "removed 12" without saying which
// twelve is asking the operator to trust a destructive operation they
// cannot inspect.
type localFilesSweepResultDTO struct {
	OK              bool     `json:"ok"`
	Error           string   `json:"error"`
	RootName        string   `json:"rootName"`
	FilesystemFiles int      `json:"filesystemFiles"`
	SourcePresent   int      `json:"sourcePresent"`
	SourceRemoved   []string `json:"sourceRemoved"`
	TargetRemoved   []string `json:"targetRemoved"`
	Added           int      `json:"added"`
	AddErrors       []string `json:"addErrors"`
}

//export LocalFilesSweep
func LocalFilesSweep(peerHandle C.int64_t, rootName *C.char, addMissing C.int64_t) (result *C.char) {
	defer recoverToErrorEnvelope("LocalFilesSweep", &result)
	if manager == nil {
		return C.CString(errNotInit)
	}
	hp := manager.Get(int64(peerHandle))
	if hp == nil {
		return C.CString(errBadPeer)
	}
	if hp.Workspace == nil {
		return C.CString(`{"ok":false,"error":"peer has no shell workspace"}`)
	}

	root := C.GoString(rootName)
	res, err := wb.SweepMount(hp.AppPeer, hp.Workspace.NotificationIngest, root)
	if err != nil {
		return C.CString(marshalOrError(localFilesSweepResultDTO{OK: false, Error: err.Error(), RootName: root}, "local-files sweep"))
	}

	dto := localFilesSweepResultDTO{
		OK:              true,
		RootName:        root,
		FilesystemFiles: res.FilesystemFiles,
		SourcePresent:   res.SourcePresent,
		SourceRemoved:   nonNilStrings(res.SourceRemoved),
		TargetRemoved:   nonNilStrings(res.TargetRemoved),
		AddErrors:       []string{},
	}
	if int64(addMissing) != 0 {
		added, errs, err := wb.IngestMissingFiles(hp.AppPeer, root)
		if err != nil {
			dto.OK = false
			dto.Error = err.Error()
			return C.CString(marshalOrError(dto, "local-files sweep"))
		}
		dto.Added = added
		dto.AddErrors = nonNilStrings(errs)
	}
	return C.CString(marshalOrError(dto, "local-files sweep"))
}

//export LocalFilesDefaultExclude
func LocalFilesDefaultExclude() (result *C.char) {
	defer recoverToErrorEnvelope("LocalFilesDefaultExclude", &result)
	b, err := json.Marshal(struct {
		OK       bool     `json:"ok"`
		Patterns []string `json:"patterns"`
	}{OK: true, Patterns: shellcmd.DefaultMountExclude()})
	if err != nil {
		return C.CString(`{"ok":false,"error":"marshal default exclude"}`)
	}
	return C.CString(string(b))
}

// splitCSVPatterns mirrors the shell's own comma-separated filter parse
// so a pattern list typed into the panel and one typed at the prompt
// mean the same thing. Trims, drops empties, returns nil for "".
func splitCSVPatterns(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// nonNilStrings guarantees `[]` rather than `null` on the wire. A null
// deserializes to a null List<T> and the panel's first foreach is a
// NullReferenceException, which per AP46 surfaces as a process abort
// rather than as an exception anyone can catch.
func nonNilStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func marshalOrError(v any, what string) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"ok":false,"error":"marshal ` + what + `"}`
	}
	return string(b)
}
