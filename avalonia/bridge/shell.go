package main

// Per-shell bridge surface. PHASE-I-DESKTOP-RENDERER-PLAN §I.5 line:
// "Each Panel owns its own per-panel Shell instance (per
// shellcmd.NewShellInWorkspace — the workspace is per-process, shells
// are per-panel)."
//
// Each ShellOpen creates a fresh shellcmd.Shell over the peer's
// shared ShellWorkspace. Per-shell state: WD, internally history is
// renderer-side (the C# ShellPanel owns it). Workspace state
// (connections, aliases, identity, handlers) is shared across all
// shells belonging to the same peer.
//
// Mirrors the SITE pattern (handle map + mutex + cascade-on-peer-
// destroy) but the dispatch surface itself reuses encodeDispatchResp,
// promptFor, pathCandidates from main.go — no logic duplication.

/*
#include <stdlib.h>
#include <stdint.h>
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"entity-workbench-go/shellboot"
	"entity-workbench-go/shellcmd"
	"entity-workbench-go/shellpanel"
	wb "entity-workbench-go/workbench"
)

type shellHandle struct {
	peerHandleID int64
	sh           *shellcmd.Shell
}

var (
	shellCounter int64
	shellMu      sync.Mutex
	shells       = map[int64]*shellHandle{}
)

// ShellOpen creates a fresh shellcmd.Shell over the given peer's
// workspace and returns its handle. The new shell's WD is seeded to
// the peer's canonical root (`/{peerID}/`) so relative paths work
// from the start — matches the per-peer-default shell's seed in
// PeerManager.Create.
//
//export ShellOpen
func ShellOpen(peerHandle C.int64_t) (result *C.char) {
	defer recoverToErrorEnvelope("ShellOpen", &result)
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

	sh := shellcmd.NewShellInWorkspace(hp.Workspace)
	sh.SetWD(shellcmd.Path("/" + hp.Workspace.Local.PeerID + "/"))

	h := atomic.AddInt64(&shellCounter, 1)
	shellMu.Lock()
	shells[h] = &shellHandle{
		peerHandleID: hp.Handle,
		sh:           sh,
	}
	shellMu.Unlock()
	return C.CString(fmt.Sprintf(`{"ok":true,"handle":%d}`, h))
}

// ShellClose tears down a shell handle. Shells own no goroutines or
// external resources beyond their own state, so close is just a map
// delete — no join needed (unlike SiteClose's wake-goroutine join).
//
//export ShellClose
func ShellClose(h C.int64_t) {
	handle := int64(h)
	shellMu.Lock()
	delete(shells, handle)
	shellMu.Unlock()
}

// ShellDispatchLine dispatches a command line through the per-handle
// shell. Same envelope shape as the peer-keyed DispatchLine but
// addressed by shell handle so multiple panels' shells can dispatch
// independently without sharing WD/state.
//
//export ShellDispatchLine
func ShellDispatchLine(h C.int64_t, cLine *C.char) (result *C.char) {
	defer recoverToErrorEnvelope("ShellDispatchLine", &result)
	if manager == nil || registry == nil {
		return C.CString(errNotInit)
	}
	handle := int64(h)
	shellMu.Lock()
	entry, ok := shells[handle]
	shellMu.Unlock()
	if !ok {
		return C.CString(`{"ok":false,"error":"unknown shell handle"}`)
	}
	sh := entry.sh

	line := strings.TrimSpace(C.GoString(cLine))
	out := []wb.OutputLine{{Text: promptFor(sh) + line, Kind: wb.KindPath}}

	if line == "" {
		return encodeDispatchResp(sh, out)
	}
	args := shellcmd.SplitArgs(line)
	if len(args) == 0 {
		return encodeDispatchResp(sh, out)
	}
	cmd := args[0]
	switch cmd {
	case "clear":
		out = []wb.OutputLine{{Text: "(clear)", Kind: wb.KindNull}}
	case "quit", "exit":
		out = append(out, wb.OutputLine{
			Text: "(panel shell does not exit; close the panel)",
			Kind: wb.KindNull,
		})
	default:
		res, err := registry.Dispatch(sh, cmd, args[1:])
		if err != nil {
			out = append(out, shellpanel.RenderError(err))
		} else {
			out = append(out, shellpanel.RenderResult(res)...)
		}
	}
	return encodeDispatchResp(sh, out)
}

// ShellComplete returns completion candidates for the per-handle shell.
// Same shape as the peer-keyed Complete but addressed by shell handle.
//
//export ShellComplete
func ShellComplete(h C.int64_t, cLine *C.char) (result *C.char) {
	defer recoverToErrorEnvelope("ShellComplete", &result)
	if manager == nil || registry == nil {
		return C.CString(errNotInit)
	}
	handle := int64(h)
	shellMu.Lock()
	entry, ok := shells[handle]
	shellMu.Unlock()
	if !ok {
		return C.CString(`{"ok":false,"error":"unknown shell handle"}`)
	}
	hp := manager.Get(entry.peerHandleID)
	if hp == nil {
		return C.CString(errBadPeer)
	}
	sh := entry.sh
	line := C.GoString(cLine)

	tokenStart := len(line)
	for i := len(line) - 1; i >= 0; i-- {
		if line[i] == ' ' || line[i] == '\t' {
			tokenStart = i + 1
			break
		}
		if i == 0 {
			tokenStart = 0
		}
	}
	prefix := line[tokenStart:]

	leading := strings.TrimSpace(line[:tokenStart])
	candidates := []string{}
	if leading == "" {
		for _, c := range registry.Commands() {
			if strings.HasPrefix(c.Name, prefix) {
				candidates = append(candidates, c.Name)
			}
		}
	} else {
		// pathCandidates resolves against the SHELL's WD, but the
		// existing helper is keyed by HostedPeer. We need a per-shell
		// variant — inline it here so the prefix matches against
		// this shell's WD, not the peer-default shell's.
		candidates = pathCandidatesForShell(hp, sh, prefix)
	}

	payload := map[string]any{
		"ok":         true,
		"candidates": candidates,
		"tokenStart": tokenStart,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return C.CString(fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error()))
	}
	return C.CString(string(b))
}

// ShellPromptForHandle returns the prompt string for the shell handle.
// Used by ShellPanel to seed its prompt label on mount.
//
//export ShellPromptForHandle
func ShellPromptForHandle(h C.int64_t) *C.char {
	handle := int64(h)
	shellMu.Lock()
	entry, ok := shells[handle]
	shellMu.Unlock()
	if !ok {
		return C.CString("")
	}
	return C.CString(promptFor(entry.sh))
}

// pathCandidatesForShell is a per-shell variant of main.go's
// pathCandidates. The original keyed completion off hp.Shell (the
// peer-default shell); per-panel shells need completion relative to
// THEIR OWN WD.
func pathCandidatesForShell(hp *shellboot.HostedPeer, sh *shellcmd.Shell, prefix string) []string {
	if hp == nil || sh == nil || hp.AppPeer == nil {
		return nil
	}
	if strings.HasPrefix(prefix, "/") || strings.HasPrefix(prefix, "@") {
		return nil
	}
	const maxCandidates = 50

	canonical := sh.Resolve(prefix)
	entries := hp.AppPeer.Store().List(string(canonical))

	wdCanonical := string(sh.WD)
	if !strings.HasSuffix(wdCanonical, "/") {
		wdCanonical += "/"
	}

	out := make([]string, 0, len(entries))
	seen := map[string]struct{}{}
	for _, e := range entries {
		if !strings.HasPrefix(e.Path, wdCanonical) {
			continue
		}
		display := e.Path[len(wdCanonical):]
		if _, ok := seen[display]; ok {
			continue
		}
		seen[display] = struct{}{}
		out = append(out, display)
		if len(out) >= maxCandidates {
			break
		}
	}
	return out
}

// cascadeShells tears down every shell handle tagged with peer h.
// Registered in BridgeInit's OnPeerDestroyed chain.
func cascadeShells(h int64) {
	shellMu.Lock()
	for id, sh := range shells {
		if sh.peerHandleID == h {
			delete(shells, id)
		}
	}
	shellMu.Unlock()
}
