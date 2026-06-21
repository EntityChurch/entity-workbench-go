package main

// Peer connections panel bridge surface. Wraps the existing shell
// connect/disconnect verbs so a panel can dial remote peers without
// the user typing into a shell. State (alias → peer-id → address)
// lives in shellcmd.ShellWorkspace.Conns and is shared with every
// shell panel hosted on the same peer — connecting via this panel
// is immediately visible to ShellPanels on the same peer, and vice
// versa.
//
// Handle lifecycle parity with the other panels (TreeOpen, PeerInfo,
// SiteOpen): Open → handle, RegisterWake → wake-fanout goroutine,
// Render → snapshot, Close → tear down. Wakes flow from a
// Store.OnPrefixChange subscription against `system/peer/transport/`
// — that's the canonical signal that a Connect/Disconnect mutated
// workspace state, regardless of which panel kicked it off.
//
// Address parsing today: accepts whatever entitysdk.AppPeer.Connect
// accepts (bare host:port, tcp://host:port, ws://host:port/ws, or
// wss://host:port/ws). Scheme routing lives in
// entitysdk/connection.go — the bridge passes addr through as-is.

/*
#include <stdlib.h>
#include <stdint.h>

// Local copy of invoke_tree_wake — see site.go for the same pattern.
static inline void invoke_tree_wake_conns(void* cb, int64_t handle) {
    if (cb != NULL) {
        ((void(*)(int64_t))cb)(handle);
    }
}
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"entity-workbench-go/shellboot"
	"entity-workbench-go/shellcmd"
	wb "entity-workbench-go/workbench"
)

type connectionsHandle struct {
	peerHandleID int64
	hp           *shellboot.HostedPeer

	wakeCh     chan struct{}
	doneCh     chan struct{}
	wakeDoneCh chan struct{}
	cancelEv   func()
}

var (
	connsCounter int64
	connsMu      sync.Mutex
	conns        = map[int64]*connectionsHandle{}
)

// ConnectionsOpen returns a handle scoped to a peer. The handle owns
// a Store.OnPrefixChange subscription on `system/peer/transport/` so
// any Connect/Disconnect (from this panel, a shell panel, or any
// other surface) fires a wake. Initial wake is seeded so the panel
// paints the existing connection list on mount.
//
//export ConnectionsOpen
func ConnectionsOpen(peerHandle C.int64_t) (result *C.char) {
	defer recoverToErrorEnvelope("ConnectionsOpen", &result)
	if manager == nil {
		return C.CString(errNotInit)
	}
	hp := manager.Get(int64(peerHandle))
	if hp == nil {
		return C.CString(errBadPeer)
	}
	ch := &connectionsHandle{
		peerHandleID: hp.Handle,
		hp:           hp,
		wakeCh:       make(chan struct{}, 1),
		doneCh:       make(chan struct{}),
	}
	// Subscribe to the transport-profile prefix — that's where
	// AppPeer.Connect / Disconnect write/erase entries. Any change
	// under this prefix means workspace.Conns may have changed
	// (RegisterRemote is called inline with addConn).
	ch.cancelEv = hp.AppPeer.Store().OnPrefixChange("system/peer/transport/", func(_ wb.ChangeEvent) {
		select {
		case ch.wakeCh <- struct{}{}:
		default:
		}
	})
	h := atomic.AddInt64(&connsCounter, 1)
	connsMu.Lock()
	conns[h] = ch
	connsMu.Unlock()
	// Seed initial wake so the panel paints existing entries.
	select {
	case ch.wakeCh <- struct{}{}:
	default:
	}
	return C.CString(fmt.Sprintf(`{"ok":true,"handle":%d}`, h))
}

//export ConnectionsRegisterWake
func ConnectionsRegisterWake(h C.int64_t, cb unsafe.Pointer) *C.char {
	handle := int64(h)
	connsMu.Lock()
	ch, ok := conns[handle]
	connsMu.Unlock()
	if !ok {
		return C.CString(`{"ok":false,"error":"unknown connections handle"}`)
	}
	ch.wakeDoneCh = make(chan struct{})
	go func() {
		defer close(ch.wakeDoneCh)
		for {
			select {
			case <-ch.doneCh:
				return
			case <-ch.wakeCh:
				C.invoke_tree_wake_conns(cb, C.int64_t(handle))
			}
		}
	}()
	return C.CString(`{"ok":true}`)
}

// ConnectionsRender returns the current alias → connection list
// snapshot. Local peer is first; remote entries are sorted by alias.
//
//export ConnectionsRender
func ConnectionsRender(h C.int64_t) (result *C.char) {
	defer recoverToErrorEnvelope("ConnectionsRender", &result)
	handle := int64(h)
	connsMu.Lock()
	ch, ok := conns[handle]
	connsMu.Unlock()
	if !ok {
		return C.CString(`{"ok":false,"error":"unknown connections handle"}`)
	}
	render := buildConnectionsRender(ch.hp)
	b, err := json.Marshal(render)
	if err != nil {
		return C.CString(fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error()))
	}
	return C.CString(fmt.Sprintf(`{"ok":true,"result":%s}`, string(b)))
}

func buildConnectionsRender(hp *shellboot.HostedPeer) wb.PeerConnectionsRender {
	if hp == nil || hp.Workspace == nil {
		return wb.PeerConnectionsRender{}
	}
	ws := hp.Workspace
	localAlias := ""
	if ws.Local != nil {
		localAlias = ws.Local.Alias
	}
	entries := make([]wb.ConnectionEntry, 0, len(ws.Conns))
	for alias, pc := range ws.Conns {
		entries = append(entries, wb.ConnectionEntry{
			Alias:   alias,
			PeerID:  pc.PeerID,
			Address: pc.Address,
			IsLocal: alias == localAlias,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsLocal != entries[j].IsLocal {
			return entries[i].IsLocal // local first
		}
		return entries[i].Alias < entries[j].Alias
	})
	return wb.PeerConnectionsRender{Entries: entries}
}

// ConnectionsConnect dispatches `connect <alias> <addr>` through the
// hosted peer's primary shell. Workspace.Conns is shared across every
// shell, so the resulting alias binding is immediately visible from
// ShellPanels too. Address may be any scheme entitysdk.AppPeer.Connect
// accepts (bare host:port, tcp://, ws://, wss://).
//
//export ConnectionsConnect
func ConnectionsConnect(h C.int64_t, cAlias, cAddr *C.char) (result *C.char) {
	defer recoverToErrorEnvelope("ConnectionsConnect", &result)
	handle := int64(h)
	connsMu.Lock()
	ch, ok := conns[handle]
	connsMu.Unlock()
	if !ok {
		return C.CString(`{"ok":false,"error":"unknown connections handle"}`)
	}
	if registry == nil {
		return C.CString(errNotInit)
	}
	alias := strings.TrimSpace(C.GoString(cAlias))
	addr := strings.TrimSpace(C.GoString(cAddr))
	if alias == "" {
		return C.CString(`{"ok":false,"error":"alias is required"}`)
	}
	if addr == "" {
		return C.CString(`{"ok":false,"error":"address is required"}`)
	}
	if _, err := registry.Dispatch(ch.hp.Shell, "connect", []string{alias, addr}); err != nil {
		return C.CString(fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error()))
	}
	// Normalize before the workspace lookup — cmdConnect stored under
	// NFC, the user-supplied alias may be NFD or carry case differences
	// the normalizer collapses. Without this, Unicode aliases would
	// pass through the Dispatch and then miss on the readback below.
	normalizedAlias, _ := shellcmd.NormalizeAlias(alias)
	pc, ok := ch.hp.Workspace.Conns[normalizedAlias]
	if !ok {
		return C.CString(`{"ok":false,"error":"connect dispatched but alias not registered"}`)
	}
	payload := map[string]any{
		"ok": true,
		"result": map[string]any{
			"alias":   pc.Alias,
			"peer_id": pc.PeerID,
			"address": pc.Address,
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return C.CString(fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error()))
	}
	return C.CString(string(b))
}

// ConnectionsDisconnect dispatches `disconnect <alias>` through the
// peer's primary shell. The shell verb closes the pooled connection,
// removes the transport-address entry (which fires the OnPrefixChange
// wake for every open ConnectionsHandle), and removes the alias from
// the workspace.
//
//export ConnectionsDisconnect
func ConnectionsDisconnect(h C.int64_t, cAlias *C.char) (result *C.char) {
	defer recoverToErrorEnvelope("ConnectionsDisconnect", &result)
	handle := int64(h)
	connsMu.Lock()
	ch, ok := conns[handle]
	connsMu.Unlock()
	if !ok {
		return C.CString(`{"ok":false,"error":"unknown connections handle"}`)
	}
	if registry == nil {
		return C.CString(errNotInit)
	}
	alias := strings.TrimSpace(C.GoString(cAlias))
	if alias == "" {
		return C.CString(`{"ok":false,"error":"alias is required"}`)
	}
	if _, err := registry.Dispatch(ch.hp.Shell, "disconnect", []string{alias}); err != nil {
		return C.CString(fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error()))
	}
	return C.CString(`{"ok":true}`)
}

//export ConnectionsClose
func ConnectionsClose(h C.int64_t) {
	handle := int64(h)
	connsMu.Lock()
	ch, ok := conns[handle]
	if ok {
		delete(conns, handle)
	}
	connsMu.Unlock()
	if !ok {
		return
	}
	if ch.cancelEv != nil {
		ch.cancelEv()
	}
	close(ch.doneCh)
	if ch.wakeDoneCh != nil {
		<-ch.wakeDoneCh
	}
}

// cascadeConnections tears down every connections handle tagged with
// peer h. Registered as an OnPeerDestroyed hook in BridgeInit.
func cascadeConnections(h int64) {
	connsMu.Lock()
	victims := []*connectionsHandle{}
	for id, ch := range conns {
		if ch.peerHandleID == h {
			victims = append(victims, ch)
			delete(conns, id)
		}
	}
	connsMu.Unlock()
	for _, ch := range victims {
		if ch.cancelEv != nil {
			ch.cancelEv()
		}
		close(ch.doneCh)
		if ch.wakeDoneCh != nil {
			<-ch.wakeDoneCh
		}
	}
}
