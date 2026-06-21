package main

// Site view panel bridge surface. Adds SiteOpen / SiteRegisterWake /
// SiteNavigate / SiteGoBack / SiteRender / SiteClose to the cgo
// envelope (D14). Wakes flow through invoke_tree_wake (declared in
// main.go's cgo preamble, shared across the package).
//
// Mirrors the MarkdownView surface in main.go — same handle map +
// wake-goroutine shape, same recoverToErrorEnvelope discipline, same
// cascade-on-peer-destroy story.

/*
#include <stdlib.h>
#include <stdint.h>

// invoke_tree_wake — local copy of main.go's helper. cgo compiles
// each Go file's preamble into its own translation unit, so the
// static inline in main.go isn't visible here. Same signature,
// same semantics — Avalonia tree-wake callback dispatch.
static inline void invoke_tree_wake_site(void* cb, int64_t handle) {
    if (cb != NULL) {
        ((void(*)(int64_t))cb)(handle);
    }
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

// siteHandle bundles a SiteModel with the goroutine + channels
// driving its wake-coalescing. Tagged with peerHandleID for cascade.
// LocalTreeResolver lives behind the model; we hold no extra refs.
type siteHandle struct {
	peerHandleID int64
	model        *wb.SiteModel
	cancelChange func()

	wakeCh     chan struct{}
	doneCh     chan struct{}
	wakeDoneCh chan struct{}
}

var (
	siteCounter int64
	siteMu      sync.Mutex
	sites       = map[int64]*siteHandle{}
)

// SiteOpen opens a SiteModel bound to a peer's store for one site id.
// Returns `{"ok":true,"handle":N}` on success, error envelope otherwise.
//
//export SiteOpen
func SiteOpen(peerHandle C.int64_t, cPeerID *C.char, cSiteID *C.char) (result *C.char) {
	defer recoverToErrorEnvelope("SiteOpen", &result)

	if manager == nil {
		return C.CString(errNotInit)
	}
	hp := manager.Get(int64(peerHandle))
	if hp == nil {
		return C.CString(errBadPeer)
	}
	peerID := ""
	if cPeerID != nil {
		peerID = C.GoString(cPeerID)
	}
	siteID := ""
	if cSiteID != nil {
		siteID = C.GoString(cSiteID)
	}
	if siteID == "" {
		return C.CString(`{"ok":false,"error":"SiteOpen requires non-empty siteID"}`)
	}
	// Empty peerID → the bound peer (the resolver's default).
	boundPeer := hp.AppPeer.PeerID()
	if peerID == "" {
		peerID = boundPeer
	}

	// Preload the bundled demo site on the bound peer if requested.
	// Idempotent — gated on the manifest's presence inside the helper.
	// "For now" affordance: lets a fresh peer with no published sites
	// surface something usable. When a real published-site flow lands,
	// the demo becomes opt-in / a CLI command instead of automatic.
	if siteID == wb.DemoSiteID && peerID == boundPeer {
		if err := wb.EnsureDemoSite(hp.AppPeer.Store(), boundPeer); err != nil {
			return C.CString(fmt.Sprintf(`{"ok":false,"error":"demo seed failed: %s"}`, err.Error()))
		}
	}

	resolver := wb.NewLocalTreeResolver(hp.AppPeer.Store(), boundPeer)
	model := wb.NewSiteModel(resolver, wb.Location{PeerID: peerID, SiteID: siteID})

	sh := &siteHandle{
		peerHandleID: hp.Handle,
		model:        model,
		wakeCh:       make(chan struct{}, 1),
		doneCh:       make(chan struct{}),
	}
	// Model OnChange → push to wakeCh non-blocking. The registered
	// wake-fan goroutine (SiteRegisterWake) reads wakeCh and invokes
	// the C# callback. Drop-on-full is the P3 single-flight guard.
	sh.cancelChange = model.OnChange(func() {
		select {
		case sh.wakeCh <- struct{}{}:
		default:
		}
	})

	h := atomic.AddInt64(&siteCounter, 1)
	siteMu.Lock()
	sites[h] = sh
	siteMu.Unlock()
	return C.CString(fmt.Sprintf(`{"ok":true,"handle":%d}`, h))
}

// SiteRegisterWake registers a C# wake callback for a site handle.
// The bridge-owned goroutine drains wakeCh and invokes the callback
// from there — the C# side MUST keep the delegate GCHandle-pinned
// for the lifetime of Go's use (P6). Returns `{"ok":true}` or error.
//
//export SiteRegisterWake
func SiteRegisterWake(h C.int64_t, cb unsafe.Pointer) (result *C.char) {
	defer recoverToErrorEnvelope("SiteRegisterWake", &result)
	handle := int64(h)
	siteMu.Lock()
	sh, ok := sites[handle]
	siteMu.Unlock()
	if !ok {
		return C.CString(`{"ok":false,"error":"unknown site handle"}`)
	}
	sh.wakeDoneCh = make(chan struct{})
	go func() {
		defer close(sh.wakeDoneCh)
		for {
			select {
			case <-sh.doneCh:
				return
			case <-sh.wakeCh:
				C.invoke_tree_wake_site(cb, C.int64_t(handle))
			}
		}
	}()
	return C.CString(`{"ok":true}`)
}

// SiteNavigate dispatches a raw link target through the model's
// classifier. External links → ok=true with kind=external; the C#
// side hands those off to the OS. Malformed → ok=false.
//
//export SiteNavigate
func SiteNavigate(h C.int64_t, cTarget *C.char) (result *C.char) {
	defer recoverToErrorEnvelope("SiteNavigate", &result)
	handle := int64(h)
	siteMu.Lock()
	sh, ok := sites[handle]
	siteMu.Unlock()
	if !ok {
		return C.CString(`{"ok":false,"error":"unknown site handle"}`)
	}
	target := ""
	if cTarget != nil {
		target = C.GoString(cTarget)
	}
	cur := sh.model.Current()
	loc, kind, ok := wb.ClassifyTarget(target, cur)
	if !ok {
		return C.CString(`{"ok":false,"error":"malformed target"}`)
	}
	if kind == wb.LinkExternal {
		// External link — surface back to C# so it can hand off to OS.
		// Model state untouched.
		return C.CString(`{"ok":true,"kind":"external"}`)
	}
	sh.model.Navigate(loc)
	return C.CString(`{"ok":true,"kind":"navigated"}`)
}

// SiteGoBack pops the model's back-history. Envelope carries
// `popped`: true if anything popped, false if history was empty.
//
//export SiteGoBack
func SiteGoBack(h C.int64_t) (result *C.char) {
	defer recoverToErrorEnvelope("SiteGoBack", &result)
	handle := int64(h)
	siteMu.Lock()
	sh, ok := sites[handle]
	siteMu.Unlock()
	if !ok {
		return C.CString(`{"ok":false,"error":"unknown site handle"}`)
	}
	popped := sh.model.GoBack()
	return C.CString(fmt.Sprintf(`{"ok":true,"popped":%t}`, popped))
}

// SiteRender returns the current SiteRenderOutput as JSON.
//
//export SiteRender
func SiteRender(h C.int64_t) (result *C.char) {
	defer recoverToErrorEnvelope("SiteRender", &result)
	handle := int64(h)
	siteMu.Lock()
	sh, ok := sites[handle]
	siteMu.Unlock()
	if !ok {
		return C.CString(`{"ok":false,"error":"unknown site handle"}`)
	}
	out := sh.model.Render()
	b, err := json.Marshal(out)
	if err != nil {
		return C.CString(fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error()))
	}
	return C.CString(fmt.Sprintf(`{"ok":true,"result":%s}`, string(b)))
}

// SiteClose tears down a site handle. Joins the wake goroutine before
// returning so the C# side can safely Free its GCHandle on the pinned
// wake delegate immediately after (P6 release-order discipline).
//
//export SiteClose
func SiteClose(h C.int64_t) {
	handle := int64(h)
	siteMu.Lock()
	sh, ok := sites[handle]
	if ok {
		delete(sites, handle)
	}
	siteMu.Unlock()
	if !ok {
		return
	}
	if sh.cancelChange != nil {
		sh.cancelChange()
		sh.cancelChange = nil
	}
	close(sh.doneCh)
	if sh.wakeDoneCh != nil {
		<-sh.wakeDoneCh
	}
}

// cascadeSites tears down every site handle tagged with peer h.
// Registered in BridgeInit's OnPeerDestroyed chain.
func cascadeSites(h int64) {
	siteMu.Lock()
	victims := []*siteHandle{}
	for id, sh := range sites {
		if sh.peerHandleID == h {
			victims = append(victims, sh)
			delete(sites, id)
		}
	}
	siteMu.Unlock()
	for _, sh := range victims {
		if sh.cancelChange != nil {
			sh.cancelChange()
			sh.cancelChange = nil
		}
		close(sh.doneCh)
		if sh.wakeDoneCh != nil {
			<-sh.wakeDoneCh
		}
	}
}
