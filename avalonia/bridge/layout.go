package main

// Workspace-layout bridge surface — the panel arrangement an operator
// made, so it is still there next launch.
//
// **Why this crosses the bridge at all**, when a layout is a list of
// Avalonia panel names and nothing else in Go will ever interpret one:
// the POLICY is not renderer-specific. Where the file lives, what
// precedence applies, that a file which exists and does not parse is an
// error rather than a silent reset (AP33), that an empty layout is not a
// layout, that the list is bounded because an operator can hand-edit it —
// none of that is about Avalonia. Only the strings are, and the model
// treats them as opaque. Keeping it here also means it is covered by a Go
// suite that runs in milliseconds instead of by a GUI test somebody
// remembers to write.
//
// **No peer handle.** These take an ALIAS, not a handle. A layout has to
// be readable before the peer's panels are built and writable after a tab
// is closed, and it is keyed by alias precisely because an ephemeral peer
// gets a fresh peer-id every launch — a peer-id-keyed layout would never
// match itself twice and the whole feature would silently do nothing in
// the default configuration. See workbench/layout_config.go.
//
// Synchronous, for the reason local_files.go states: the async cgo shape
// carries AP31's use-after-free on C-owned arguments, and there is
// nothing here worth a goroutine.

/*
#include <stdlib.h>
#include <stdint.h>
*/
import "C"

import (
	"encoding/json"
	"os"

	wb "entity-workbench-go/workbench"
)

// LayoutSetPath redirects the layout file, in process.
//
// It exists because **`WB_LAYOUT` cannot be set from a managed test.**
// Go captures its environment once at process start, so a C#
// `Environment.SetEnvironmentVariable` calls libc `setenv` and never
// reaches `os.Getenv` in this library — the same trap that made
// `WB_NO_AUTOPIN` useless from the browser tests and produced
// `BrowserPanel.AutoPinOnOpen`. Env vars are for launching the app;
// in-process control needs an in-process switch.
//
// It is not test-only machinery bolted on: it is the in-process spelling
// of a knob the model already has, and it is what keeps the headless
// suite from writing to the developer's real ~/.entity — no suite in
// this repo may depend on, or scribble on, state outside the tree.
//
// Go's own os.Setenv updates the runtime's cached copy, which is why
// setting it from HERE works where setting it from C# does not.
//
//export LayoutSetPath
func LayoutSetPath(path *C.char) (result *C.char) {
	defer recoverToErrorEnvelope("LayoutSetPath", &result)
	if err := os.Setenv("WB_LAYOUT", C.GoString(path)); err != nil {
		return C.CString(`{"ok":false,"error":"set layout path"}`)
	}
	return C.CString(`{"ok":true}`)
}

type layoutLoadDTO struct {
	OK       bool     `json:"ok"`
	Panels   []string `json:"panels"`
	NavWidth float64  `json:"navWidth"`
	Source   string   `json:"source"`

	// Error is set when the layout file exists and could not be read.
	// Note OK stays TRUE in that case and Panels carries the defaults:
	// the app must still open. The renderer's job is to show the reason
	// AND draw the window — an operator whose layout file has a stray
	// comma should lose the arrangement, learn why, and keep working,
	// not face a dead launch or a silent reset.
	Error string `json:"error"`
}

//export LayoutLoad
func LayoutLoad(alias *C.char) (result *C.char) {
	defer recoverToErrorEnvelope("LayoutLoad", &result)

	cfg, err := wb.LoadLayoutConfig()
	l := cfg.For(C.GoString(alias))
	dto := layoutLoadDTO{
		OK:       true,
		Panels:   l.Panels,
		NavWidth: l.NavWidth,
		Source:   cfg.Source,
	}
	if err != nil {
		dto.Error = err.Error()
	}
	if dto.Panels == nil {
		dto.Panels = []string{}
	}
	b, merr := json.Marshal(dto)
	if merr != nil {
		return C.CString(`{"ok":false,"error":"marshal layout load"}`)
	}
	return C.CString(string(b))
}

type layoutSaveDTO struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

// LayoutSave records one peer's arrangement and writes the file.
//
// It is read-modify-write on every call rather than holding the config in
// bridge memory, and that is deliberate: several peer tabs are open at
// once and each saves independently, so a cached copy would let the last
// writer clobber every other tab's entry. The file is small and this
// happens on operator actions, not on a frame.
//
// panelsJSON is a JSON array of panel names — an array rather than a
// delimited string because the names come from a renderer's registry and
// picking a delimiter is inventing a constraint on what a future panel
// may be called.
//
//export LayoutSave
func LayoutSave(alias *C.char, panelsJSON *C.char, navWidth C.double) (result *C.char) {
	defer recoverToErrorEnvelope("LayoutSave", &result)

	var panels []string
	if raw := C.GoString(panelsJSON); raw != "" {
		if err := json.Unmarshal([]byte(raw), &panels); err != nil {
			return C.CString(`{"ok":false,"error":"layout save: panels is not a JSON array"}`)
		}
	}

	// A load error here is NOT fatal to the save, and treating it as
	// fatal would be the worse bug: an unparseable file would then be
	// permanently unfixable from the UI, since every attempt to save over
	// it would refuse first. Load returns a usable empty config alongside
	// its error, so saving replaces the broken file with a good one.
	cfg, _ := wb.LoadLayoutConfig()
	cfg.Remember(C.GoString(alias), wb.PeerLayout{
		Panels:   panels,
		NavWidth: float64(navWidth),
	})
	if err := wb.SaveLayoutConfig(cfg); err != nil {
		b, _ := json.Marshal(layoutSaveDTO{OK: false, Error: err.Error()})
		return C.CString(string(b))
	}
	b, err := json.Marshal(layoutSaveDTO{OK: true})
	if err != nil {
		return C.CString(`{"ok":false,"error":"marshal layout save"}`)
	}
	return C.CString(string(b))
}
