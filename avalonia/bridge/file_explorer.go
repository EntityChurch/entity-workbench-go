package main

// File-explorer bridge surface. Wraps workbench's FileExplorerModel —
// the contents of one mount, directory at a time, joining the source
// layer (what the watcher ingested) against the document layer (what a
// viewer can open).
//
// **This one IS a handle, unlike LocalFilesRender beside it, and the
// difference is not stylistic.** A mount's CONFIG changes when an
// operator runs a verb, once a session; that surface is stateless and
// re-reads on demand. A mount's CONTENTS change on every save in a
// watched directory, and during a watcher's initial scan they change
// hundreds of times a second. The model owns two prefix subscriptions
// and needs a wake, so this file carries the full
// Open/RegisterWake/Render/Close shape.
//
// Wakes are coalesced into a ONE-DEEP channel. An initial scan of a
// 40k-file directory would otherwise post 40k dispatcher callbacks at
// the UI thread, each triggering a render, and the panel would be
// unusable for exactly as long as the mount was interesting.
//
// Note the cgo preamble rule: the comment block IMMEDIATELY preceding
// `import "C"` is compiled as C, so prose with backticks or em-dashes
// there is fed to gcc and reported as a stray token. Package prose lives
// above, separated by the real preamble.

/*
#include <stdlib.h>
#include <stdint.h>

// Local copy of the wake trampoline, per the convention every other
// handle-shaped area in this bridge follows (browse.go, handlers.go,
// discovery.go). cgo will not share a static inline across files.
static inline void invoke_tree_wake_explorer(void* cb, int64_t handle) {
    ((void(*)(int64_t))cb)(handle);
}
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"

	wb "entity-workbench-go/workbench"
)

type explorerHandle struct {
	peerHandleID int64
	model        *wb.FileExplorerModel
	wakeCh       chan struct{}
	doneCh       chan struct{}
	wakeDoneCh   chan struct{}
}

var (
	explorerMu      sync.Mutex
	explorers       = map[int64]*explorerHandle{}
	explorerCounter int64
)

func lookupExplorer(h C.int64_t) (*explorerHandle, bool) {
	explorerMu.Lock()
	defer explorerMu.Unlock()
	eh, ok := explorers[int64(h)]
	return eh, ok
}

// explorerEntryDTO is one row as the panel receives it.
//
// Flat and explicit rather than marshalling the model struct: the wire
// shape is a contract, and AP49 is what happens when it drifts in
// silence — the panel there declared two fewer fields than the bridge
// sent and System.Text.Json dropped them without a word. Every field
// here is declared on the C# side and asserted to arrive.
type explorerEntryDTO struct {
	Name       string `json:"name"`
	RelPath    string `json:"relPath"`
	IsDir      bool   `json:"isDir"`
	ChildFiles int    `json:"childFiles"`
	ChildBytes int64  `json:"childBytes"`
	Size       int64  `json:"size"`
	ModifiedAt int64  `json:"modifiedAt"`
	Kind       string `json:"kind"`
	Language   string `json:"language"`
	MediaType  string `json:"mediaType"`
	SourcePath string `json:"sourcePath"`
	TargetPath string `json:"targetPath"`
	EntityType string `json:"entityType"`
	Ingested   bool   `json:"ingested"`
	Status     string `json:"status"`
}

type explorerRenderDTO struct {
	OK             bool               `json:"ok"`
	Error          string             `json:"error"`
	Root           string             `json:"root"`
	FilesystemRoot string             `json:"filesystemRoot"`
	SourcePrefix   string             `json:"sourcePrefix"`
	TargetPrefix   string             `json:"targetPrefix"`
	Dir            string             `json:"dir"`
	Crumbs         []string           `json:"crumbs"`
	Entries        []explorerEntryDTO `json:"entries"`
	TotalFiles     int                `json:"totalFiles"`
	TotalBytes     int64              `json:"totalBytes"`
	Ingested       int                `json:"ingested"`
	NotIngested    int                `json:"notIngested"`
	KindCounts     map[string]int     `json:"kindCounts"`
	Note           string             `json:"note"`
}

//export FileExplorerOpen
func FileExplorerOpen(peerHandle C.int64_t) (result *C.char) {
	defer recoverToErrorEnvelope("FileExplorerOpen", &result)
	if manager == nil {
		return C.CString(errNotInit)
	}
	hp := manager.Get(int64(peerHandle))
	if hp == nil {
		return C.CString(errBadPeer)
	}

	eh := &explorerHandle{
		peerHandleID: hp.Handle,
		model:        wb.NewFileExplorerModel(hp.AppPeer.Store()),
		wakeCh:       make(chan struct{}, 1),
		doneCh:       make(chan struct{}),
	}
	// Coalesce: a full one-deep channel already means "something
	// changed, re-render", and a second notification adds nothing.
	eh.model.OnChange(func() {
		select {
		case eh.wakeCh <- struct{}{}:
		default:
		}
	})

	h := atomic.AddInt64(&explorerCounter, 1)
	explorerMu.Lock()
	explorers[h] = eh
	explorerMu.Unlock()
	return C.CString(fmt.Sprintf(`{"ok":true,"handle":%d}`, h))
}

//export FileExplorerRegisterWake
func FileExplorerRegisterWake(h C.int64_t, cb unsafe.Pointer) (result *C.char) {
	defer recoverToErrorEnvelope("FileExplorerRegisterWake", &result)
	eh, ok := lookupExplorer(h)
	if !ok {
		return C.CString(`{"ok":false,"error":"unknown explorer handle"}`)
	}
	handle := int64(h)
	eh.wakeDoneCh = make(chan struct{})
	go func() {
		defer close(eh.wakeDoneCh)
		for {
			select {
			case <-eh.doneCh:
				return
			case <-eh.wakeCh:
				C.invoke_tree_wake_explorer(cb, C.int64_t(handle))
			}
		}
	}()
	return C.CString(`{"ok":true}`)
}

// FileExplorerSetRoot binds the handle to a named mount.
//
// Synchronous, and the AP31 hazard therefore does not apply: rootName is
// C-owned memory belonging to the .NET marshaller and is freed when the
// P/Invoke returns, so reading it on a goroutine would be a
// use-after-free that reads as the empty string rather than crashing.
// Every export in this file is synchronous for that reason plus the
// simpler one — none of them block.
//
//export FileExplorerSetRoot
func FileExplorerSetRoot(h C.int64_t, rootName *C.char) (result *C.char) {
	defer recoverToErrorEnvelope("FileExplorerSetRoot", &result)
	eh, ok := lookupExplorer(h)
	if !ok {
		return C.CString(`{"ok":false,"error":"unknown explorer handle"}`)
	}
	if err := eh.model.SetRoot(C.GoString(rootName)); err != nil {
		return C.CString(marshalOrError(struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}{OK: false, Error: err.Error()}, "explorer set-root"))
	}
	return C.CString(`{"ok":true}`)
}

//export FileExplorerSetDir
func FileExplorerSetDir(h C.int64_t, dir *C.char) (result *C.char) {
	defer recoverToErrorEnvelope("FileExplorerSetDir", &result)
	eh, ok := lookupExplorer(h)
	if !ok {
		return C.CString(`{"ok":false,"error":"unknown explorer handle"}`)
	}
	eh.model.SetDir(C.GoString(dir))
	return C.CString(`{"ok":true}`)
}

//export FileExplorerUp
func FileExplorerUp(h C.int64_t) (result *C.char) {
	defer recoverToErrorEnvelope("FileExplorerUp", &result)
	eh, ok := lookupExplorer(h)
	if !ok {
		return C.CString(`{"ok":false,"error":"unknown explorer handle"}`)
	}
	eh.model.Up()
	return C.CString(`{"ok":true}`)
}

//export FileExplorerRender
func FileExplorerRender(h C.int64_t) (result *C.char) {
	defer recoverToErrorEnvelope("FileExplorerRender", &result)
	eh, ok := lookupExplorer(h)
	if !ok {
		return C.CString(`{"ok":false,"error":"unknown explorer handle"}`)
	}
	out := eh.model.Render()

	// Non-nil slices and maps so the panel receives `[]` / `{}` rather
	// than `null`. A null deserializes to a null List<T> and the panel's
	// first foreach is a NullReferenceException — which per AP46 surfaces
	// as a process abort, not as an exception anyone catches.
	dto := explorerRenderDTO{
		OK:             out.Error == "",
		Error:          out.Error,
		Root:           out.Root,
		FilesystemRoot: out.FilesystemRoot,
		SourcePrefix:   out.SourcePrefix,
		TargetPrefix:   out.TargetPrefix,
		Dir:            out.Dir,
		Crumbs:         nonNilStrings(out.Crumbs),
		Entries:        []explorerEntryDTO{},
		TotalFiles:     out.TotalFiles,
		TotalBytes:     out.TotalBytes,
		Ingested:       out.Ingested,
		NotIngested:    out.NotIngested,
		KindCounts:     out.KindCounts,
		Note:           out.Note,
	}
	if dto.KindCounts == nil {
		dto.KindCounts = map[string]int{}
	}
	for _, e := range out.Entries {
		dto.Entries = append(dto.Entries, explorerEntryDTO{
			Name:       e.Name,
			RelPath:    e.RelPath,
			IsDir:      e.IsDir,
			ChildFiles: e.ChildFiles,
			ChildBytes: e.ChildBytes,
			Size:       e.Size,
			ModifiedAt: e.ModifiedAt,
			Kind:       e.Kind,
			Language:   e.Language,
			MediaType:  e.MediaType,
			SourcePath: e.SourcePath,
			TargetPath: e.TargetPath,
			EntityType: e.EntityType,
			Ingested:   e.Ingested,
			Status:     e.Status,
		})
	}

	b, err := json.Marshal(dto)
	if err != nil {
		return C.CString(`{"ok":false,"error":"marshal explorer render"}`)
	}
	return C.CString(string(b))
}

type explorerPreviewDTO struct {
	OK        bool   `json:"ok"`
	Error     string `json:"error"`
	RelPath   string `json:"relPath"`
	Kind      string `json:"kind"`
	Language  string `json:"language"`
	Text      string `json:"text"`
	Textual   bool   `json:"textual"`
	Truncated bool   `json:"truncated"`
	Size      int64  `json:"size"`
}

// FileExplorerPreview returns a file's bytes for inline display, capped
// at workbench.MaxPreviewBytes.
//
// The cap is why this is safe to call on any row. The browser panel's
// lesson is the precedent: an 8.27 MB pre-rendered HTML page crossed
// this boundary as a JSON string on every render, and the fix was to cap
// at the model rather than to hope no page was large.
//
//export FileExplorerPreview
func FileExplorerPreview(h C.int64_t, relPath *C.char) (result *C.char) {
	defer recoverToErrorEnvelope("FileExplorerPreview", &result)
	eh, ok := lookupExplorer(h)
	if !ok {
		return C.CString(`{"ok":false,"error":"unknown explorer handle"}`)
	}
	p := eh.model.Preview(C.GoString(relPath))
	return C.CString(marshalOrError(explorerPreviewDTO{
		OK:        p.Error == "",
		Error:     p.Error,
		RelPath:   p.RelPath,
		Kind:      p.Kind,
		Language:  p.Language,
		Text:      p.Text,
		Textual:   p.Textual,
		Truncated: p.Truncated,
		Size:      p.Size,
	}, "explorer preview"))
}

//export FileExplorerClose
func FileExplorerClose(h C.int64_t) {
	handle := int64(h)
	explorerMu.Lock()
	eh, ok := explorers[handle]
	if ok {
		delete(explorers, handle)
	}
	explorerMu.Unlock()
	if !ok {
		return
	}
	closeExplorerHandle(eh)
}

// closeExplorerHandle tears one handle down in the order that makes the
// C# side's delegate safe to drop.
//
// Clear the model's callback FIRST, then stop the wake goroutine, and
// only return once that goroutine has exited: the goroutine holds a
// function pointer into a marshaled .NET delegate, and if C# frees the
// delegate while Go still intends to call it, the process aborts with "a
// callback was made on a garbage collected delegate". Same constraint
// TreeClose documents.
func closeExplorerHandle(eh *explorerHandle) {
	eh.model.OnChange(nil)
	close(eh.doneCh)
	if eh.wakeDoneCh != nil {
		<-eh.wakeDoneCh
	}
	eh.model.Close()
}

// cascadeExplorers tears down every explorer handle tagged with peer h.
// Registered as an OnPeerDestroyed hook, the same way trees and watches
// are — a handle outliving its peer is a use-after-free waiting for a
// render.
func cascadeExplorers(h int64) {
	explorerMu.Lock()
	victims := []*explorerHandle{}
	for id, eh := range explorers {
		if eh.peerHandleID == h {
			victims = append(victims, eh)
			delete(explorers, id)
		}
	}
	explorerMu.Unlock()
	for _, eh := range victims {
		closeExplorerHandle(eh)
	}
}
