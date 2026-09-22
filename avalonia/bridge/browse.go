package main

// Browser panel bridge surface — BrowseOpen / BrowseRegisterWake /
// BrowsePin / BrowseNames / BrowseGo / BrowseBack / BrowseForward /
// BrowseRender / BrowseClose on the cgo envelope (D14).
//
// **Same two departures as verify.go, for the same reasons, and a third
// that is new.**
//
//  1. *No peer handle.* A Mode A2 consumer is not a peer (§6.5.3). The
//     browser derives every key it verifies against from a peer-id and
//     dispatches nothing.
//  2. *Operation-triggered wake.* Every other panel wakes on tree events
//     at a rate we do not control; this one wakes when a navigation
//     finishes, because a navigation is something an operator started.
//
//  3. NEW — **the single-flight guard is per-operation, not per-panel.**
//     `verify.go` refuses a second run outright: two verifications would
//     interleave two origins' steps into one list. Here, enumerating the
//     registry and navigating are *different* operations against
//     *different* state (the name list vs. the page + chain), and a user
//     who clicks a name while the list is still loading is doing
//     something reasonable. So each has its own guard and neither
//     blocks the other. What is still refused is a second navigation
//     while one is in flight — that one has the verify.go failure mode
//     exactly: two chains, one list, read as one journey.

/*
#include <stdlib.h>
#include <stdint.h>

static inline void invoke_tree_wake_browse(void* cb, int64_t handle) {
    if (cb != NULL) {
        ((void(*)(int64_t))cb)(handle);
    }
}
*/
import "C"

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sync"
	"sync/atomic"
	"unsafe"

	"go.entitychurch.org/entity-core-go/core/types"

	wb "entity-workbench-go/workbench"
)

type browseHandle struct {
	model *wb.BrowseModel

	// ops counts COMPLETED async operations on this handle. It is on the
	// render envelope for two reasons: a panel can ignore a wake that
	// arrived for an operation it has already drawn, and a test can wait
	// for a specific completion instead of guessing from a button's
	// enabled state — which is a race, because the goroutine may not
	// have entered the operation by the time the first Render lands.
	ops int64

	mu      sync.Mutex
	navving bool
	listing bool
	// autopinning is true while the start-up pin is resolving. An
	// explicit BrowsePin during that window is REFUSED rather than
	// allowed to interleave: both write the same model fields around
	// network I/O, so last-writer-wins would be decided by which HTTP
	// round trip finished first. Same single-flight rule the rest of
	// this file uses, and for the same reason.
	autopinning bool
	wakeCb      unsafe.Pointer
	cancelNav   context.CancelFunc

	// wakeWG tracks goroutines that may still invoke wakeCb.
	//
	// NILLING wakeCb UNDER THE MUTEX IS NOT ENOUGH, and that is what this
	// handle did until 2026-09-10. Every async site below reads the
	// pointer into a LOCAL, releases the lock, and then calls into .NET —
	// so a Close that nils the field and returns leaves an in-flight
	// goroutine holding a pointer to a delegate the caller is about to
	// free. `_wakeCallbackHandle.Free()` runs on the next line of
	// BrowserPanel.Dispose, and a callback landing after it does not
	// throw: the runtime prints "a callback was made on a garbage
	// collected delegate" and ABORTS THE PROCESS.
	//
	// Check-then-act with no wait narrows the window; it does not close
	// it. Same rule as wake_pump.go's stop(): a cancel that does not wait
	// is a request, not a cancel.
	wakeWG sync.WaitGroup
}

// beginWake registers an in-flight goroutine that may invoke wakeCb.
// Call it BEFORE `go`, never inside — Add racing Wait is the bug this
// exists to prevent.
func (bh *browseHandle) beginWake() { bh.wakeWG.Add(1) }

// fireWake invokes the registered callback if there still is one.
func (bh *browseHandle) fireWake(handle C.int64_t) {
	bh.mu.Lock()
	cb := bh.wakeCb
	bh.mu.Unlock()
	if cb != nil {
		C.invoke_tree_wake_browse(cb, handle)
	}
}

var (
	browseCounter int64
	browseMu      sync.Mutex
	browsers      = map[int64]*browseHandle{}
)

// browsePin is the C#-facing pin payload. Any pin_* field set means the
// layout is PINNED and discovery is skipped — the same all-or-nothing
// rule verify.go uses, so an operator who has learned one has learned
// both.
type browsePin struct {
	Origin string `json:"origin"`
	PeerID string `json:"peer_id"`

	PinTree     string `json:"pin_tree"`
	PinContent  string `json:"pin_content"`
	PinManifest string `json:"pin_manifest"`
	PinLayout   string `json:"pin_layout"`
	PinLeaf     string `json:"pin_leaf"`
	PinListing  string `json:"pin_listing"`

	// TargetOrigin is where a resolved binding's origin-relative URLs
	// are rooted, and where a peer-id address is fetched from. Empty
	// means "the registry's own origin", which is right for every
	// single-host deployment.
	TargetOrigin string `json:"target_origin"`
}

func (p browsePin) pinned() bool {
	return p.PinTree != "" || p.PinContent != "" || p.PinManifest != "" || p.PinLayout != ""
}

// BrowseOpen creates a browser panel handle.
//
//export BrowseOpen
func BrowseOpen() (result *C.char) {
	defer recoverToErrorEnvelope("BrowseOpen", &result)

	bh := &browseHandle{model: wb.NewBrowseModel(nil)}
	h := atomic.AddInt64(&browseCounter, 1)
	browseMu.Lock()
	browsers[h] = bh
	browseMu.Unlock()

	b, _ := json.Marshal(map[string]any{"ok": true, "handle": h})
	return C.CString(string(b))
}

func lookupBrowse(h int64) *browseHandle {
	browseMu.Lock()
	defer browseMu.Unlock()
	return browsers[h]
}

// BrowseRegisterWake registers the completion callback.
//
//export BrowseRegisterWake
func BrowseRegisterWake(handle C.int64_t, cb unsafe.Pointer) (result *C.char) {
	defer recoverToErrorEnvelope("BrowseRegisterWake", &result)

	bh := lookupBrowse(int64(handle))
	if bh == nil {
		return C.CString(`{"ok":false,"error":"unknown browse handle"}`)
	}
	bh.mu.Lock()
	bh.wakeCb = cb
	bh.mu.Unlock()
	return C.CString(`{"ok":true}`)
}

// BrowsePin sets the trust root. Synchronous: pinning does one optional
// profile fetch and no verification, so there is nothing worth a wake.
//
//export BrowsePin
func BrowsePin(handle C.int64_t, cJSON *C.char) (result *C.char) {
	defer recoverToErrorEnvelope("BrowsePin", &result)

	bh := lookupBrowse(int64(handle))
	if bh == nil {
		return C.CString(`{"ok":false,"error":"unknown browse handle"}`)
	}
	bh.mu.Lock()
	if bh.autopinning {
		bh.mu.Unlock()
		return C.CString(`{"ok":false,"error":"the start-up registry pin is still resolving — ` +
			`try again in a moment, or set WB_NO_AUTOPIN=1 to start unpinned"}`)
	}
	bh.mu.Unlock()

	var p browsePin
	if cJSON != nil {
		if err := json.Unmarshal([]byte(C.GoString(cJSON)), &p); err != nil {
			b, _ := json.Marshal(map[string]any{"ok": false, "error": "bad pin: " + err.Error()})
			return C.CString(string(b))
		}
	}
	bh.model.SetTargetOrigin(p.TargetOrigin)

	var err error
	if p.pinned() {
		leaf, listing := p.PinLeaf, p.PinListing
		if leaf == "" {
			leaf = ".bin"
		}
		if listing == "" {
			listing = ".list"
		}
		err = bh.model.PinRegistry(context.Background(), p.Origin, p.PeerID, &types.TransportEndpoint{
			TreeURLPrefix:     p.PinTree,
			ContentURLPrefix:  p.PinContent,
			ManifestURLPrefix: p.PinManifest,
			ContentLayout:     p.PinLayout,
			TreeLeafSuffix:    leaf,
			TreeListingSuffix: listing,
		})
	} else {
		err = bh.model.PinRegistry(context.Background(), p.Origin, p.PeerID, nil)
	}
	if err != nil {
		b, _ := json.Marshal(map[string]any{"ok": false, "error": err.Error()})
		return C.CString(string(b))
	}
	return C.CString(`{"ok":true}`)
}

// BrowseNames enumerates the pinned registry — the §6a.3a walk. Async;
// wakes on completion.
//
//export BrowseNames
func BrowseNames(handle C.int64_t) (result *C.char) {
	defer recoverToErrorEnvelope("BrowseNames", &result)

	bh := lookupBrowse(int64(handle))
	if bh == nil {
		return C.CString(`{"ok":false,"error":"unknown browse handle"}`)
	}
	bh.mu.Lock()
	if bh.listing {
		bh.mu.Unlock()
		return C.CString(`{"ok":false,"error":"the registry is already being enumerated"}`)
	}
	bh.listing = true
	bh.mu.Unlock()

	bh.beginWake()
	go func() {
		defer bh.wakeWG.Done()
		defer func() { _ = recover() }()
		_ = bh.model.RefreshNames(context.Background())
		atomic.AddInt64(&bh.ops, 1)
		bh.mu.Lock()
		bh.listing = false
		bh.mu.Unlock()
		bh.fireWake(C.int64_t(handle))
	}()
	return C.CString(`{"ok":true}`)
}

// BrowseAutoPin pins the configured registry and enumerates it, so the
// panel opens on a usable browser instead of an empty one. Async.
//
// Configuration and precedence live in `workbench.LoadBrowseConfig`
// (env > ~/.entity/browser.json > built-in default). Nothing about the
// choice is decided here — this is the call site, not the policy.
//
// **Async on purpose.** Pinning does a `entity-deployment.json` fetch and
// a `transport-profile` fetch, and enumerating walks a signed root. Doing
// that in `BrowseOpen` — which the panel calls from its constructor,
// on the UI thread — would block the window's first paint on the network,
// and on a slow or unreachable origin would hang the app before it drew
// anything. The panel calls this after it is built and wakes when done.
//
// A failure here is not fatal and not silent: the model records it and
// the operator can pin by hand, which is the pre-2026-08-31 flow.
//
//export BrowseAutoPin
func BrowseAutoPin(handle C.int64_t) (result *C.char) {
	defer recoverToErrorEnvelope("BrowseAutoPin", &result)

	bh := lookupBrowse(int64(handle))
	if bh == nil {
		return C.CString(`{"ok":false,"error":"unknown browse handle"}`)
	}

	cfg, err := wb.LoadBrowseConfig()
	if err != nil {
		// A malformed config file is reported, never ignored — see
		// LoadBrowseConfig. The browser still opens, unpinned.
		b, _ := json.Marshal(map[string]any{"ok": false, "error": err.Error()})
		return C.CString(string(b))
	}
	if !cfg.ShouldAutoPin() {
		return C.CString(`{"ok":true,"skipped":true}`)
	}

	bh.mu.Lock()
	if bh.listing || bh.autopinning {
		bh.mu.Unlock()
		return C.CString(`{"ok":false,"error":"the registry is already being enumerated"}`)
	}
	bh.listing = true
	bh.autopinning = true
	bh.mu.Unlock()

	origin, peer := cfg.RegistryOrigin, cfg.RegistryPeer
	bh.beginWake()
	go func() {
		defer bh.wakeWG.Done()
		defer func() { _ = recover() }()
		ctx := context.Background()
		// nil endpoint = discover the layout from the origin, and (when
		// peer is empty) adopt the registry the origin nominates.
		if perr := bh.model.PinRegistry(ctx, origin, peer, nil); perr == nil {
			_ = bh.model.RefreshNames(ctx)
		}
		atomic.AddInt64(&bh.ops, 1)
		bh.mu.Lock()
		bh.listing = false
		bh.autopinning = false
		bh.mu.Unlock()
		bh.fireWake(C.int64_t(handle))
	}()
	return C.CString(`{"ok":true}`)
}

// BrowseGo navigates. Async; wakes on completion.
//
// A second navigation while one is in flight is refused rather than
// queued or cancelled — two chains interleaved into one step list would
// be read as one journey, which is verify.go's failure mode exactly.
//
//export BrowseGo
func BrowseGo(handle C.int64_t, cAddr *C.char) (result *C.char) {
	defer recoverToErrorEnvelope("BrowseGo", &result)

	// **Copy the C string HERE, not inside the goroutine.**
	//
	// `cAddr` is a buffer the .NET marshaller allocated for the duration
	// of the P/Invoke and frees the moment this function returns. The
	// goroutine below outlives that by construction — the whole point of
	// this call is that it returns immediately — so a `C.GoString(cAddr)`
	// down there reads freed memory. It does not crash: it reads as an
	// EMPTY STRING, and an empty address parses as "an address needs at
	// least a name or a peer-id", which reports as a user error in a
	// panel the user typed an address into.
	//
	// Found by the headless panel tests on 2026-08-21, after the first
	// version did exactly this. Nothing about the Go side is unsafe on
	// its own; the lifetime that matters belongs to the caller and ends
	// at the return.
	addr := C.GoString(cAddr)
	return browseNavigate(handle, func(m *wb.BrowseModel, ctx context.Context) {
		_ = m.Open(ctx, addr)
	})
}

// BrowseFollow follows a link written in the page on screen. Async;
// wakes on completion, exactly like BrowseGo.
//
// The renderer passes the link's raw href — `support.md`,
// `site:billslab-entity-system`, `../notes/x.md`, `https://…` — and does
// no interpretation of it whatsoever. Resolving an href to a page slug is
// Layer-2 algorithm contract that must stay byte-identical with
// `entity-browser-rust`; a copy of those rules in C# would be a second
// implementation nobody diffs, and its failure mode is a silently dead
// link rather than an error.
//
//export BrowseFollow
func BrowseFollow(handle C.int64_t, cTarget *C.char) (result *C.char) {
	defer recoverToErrorEnvelope("BrowseFollow", &result)

	// Copy the C string HERE, not inside the goroutine — see BrowseGo's
	// note. The same use-after-free applies verbatim, and its symptom
	// here would be an empty href, which classifies as an in-site link to
	// the site's root page: a click that plausibly "went somewhere".
	target := C.GoString(cTarget)
	return browseNavigate(handle, func(m *wb.BrowseModel, ctx context.Context) {
		_ = m.Follow(ctx, target)
	})
}

// BrowseBack / BrowseForward move through history, re-running the chain.
//
//export BrowseBack
func BrowseBack(handle C.int64_t) (result *C.char) {
	defer recoverToErrorEnvelope("BrowseBack", &result)
	return browseNavigate(handle, func(m *wb.BrowseModel, ctx context.Context) { _ = m.Back(ctx) })
}

// BrowseForward is Back's mirror.
//
//export BrowseForward
func BrowseForward(handle C.int64_t) (result *C.char) {
	defer recoverToErrorEnvelope("BrowseForward", &result)
	return browseNavigate(handle, func(m *wb.BrowseModel, ctx context.Context) { _ = m.Forward(ctx) })
}

func browseNavigate(handle C.int64_t, run func(*wb.BrowseModel, context.Context)) *C.char {
	bh := lookupBrowse(int64(handle))
	if bh == nil {
		return C.CString(`{"ok":false,"error":"unknown browse handle"}`)
	}
	bh.mu.Lock()
	if bh.navving {
		bh.mu.Unlock()
		return C.CString(`{"ok":false,"error":"a navigation is already in flight"}`)
	}
	ctx, cancel := context.WithCancel(context.Background())
	bh.navving, bh.cancelNav = true, cancel
	bh.mu.Unlock()

	bh.beginWake()
	go func() {
		defer bh.wakeWG.Done()
		defer func() { _ = recover() }()
		run(bh.model, ctx)
		atomic.AddInt64(&bh.ops, 1)

		bh.mu.Lock()
		bh.navving, bh.cancelNav = false, nil
		bh.mu.Unlock()
		bh.fireWake(C.int64_t(handle))
	}()
	return C.CString(`{"ok":true}`)
}

// BrowseRender returns the current view as JSON. Safe at any time,
// including mid-navigation — the result carries `Running`.
//
//export BrowseRender
func BrowseRender(handle C.int64_t) (result *C.char) {
	defer recoverToErrorEnvelope("BrowseRender", &result)

	bh := lookupBrowse(int64(handle))
	if bh == nil {
		return C.CString(`{"ok":false,"error":"unknown browse handle"}`)
	}
	view := bh.model.Render()
	// The RAW body does not cross this boundary.
	//
	// `Content.BodyMarkdown` is the verified bytes at full size, and on
	// the live federation those reach **8.27 MB** for a single page
	// (billslab's `papers/full-corpus`). Marshalling that into a JSON
	// string, copying it through cgo, and handing it to a markdown
	// parser on the UI thread is most of the "fifteen seconds" an
	// operator reported — and unlike the network half, caching does not
	// help, because the cost is paid again on every display.
	//
	// `view.Body` is the display projection: HTML lowered to text,
	// `::embed` directives lowered to markdown images, capped at
	// MaxDisplayBytes with a note saying so, and carrying FullBytes so
	// the honest size is still available. Everything a renderer needs;
	// nothing it cannot draw. The shell keeps reading the full body
	// because a terminal is a different medium with a pager behind it.
	view.Content.BodyMarkdown = ""

	b, err := json.Marshal(map[string]any{
		"ok":   true,
		"view": view,
		"ops":  atomic.LoadInt64(&bh.ops),
	})
	if err != nil {
		return C.CString(`{"ok":false,"error":"marshal view failed"}`)
	}
	return C.CString(string(b))
}

// BrowseAsset resolves one embedded asset of the page currently on
// screen and returns its bytes base64-encoded.
//
// **Synchronous, and that is a deliberate departure from every other
// navigation export here.** An asset is a leaf fetch against a cache
// that the walk has usually already filled, so the common case is a map
// lookup; the async machinery (a goroutine, an ops bump, a wake, a full
// re-render) would cost more than the work and would re-render the page
// once per figure. A caller on a UI thread must still not call this
// directly — the miss path is an HTTP round trip — and BrowserPanel
// drives it off a worker.
//
// The ref is passed through UNINTERPRETED, for the same reason
// BrowseFollow does it: [wb.AssetNameFromRef] is the security gate that
// decides whether a string in someone else's page body may cause a
// fetch, it is Layer-2 contract shared with entity-browser-rust, and a
// C# copy of it would be a second implementation of a rule whose failure
// mode is "the renderer fetched a tracking URL".
//
// The site is NOT a parameter. It is the page on screen, read inside the
// model — see [wb.BrowseModel.Asset].
//
//export BrowseAsset
func BrowseAsset(handle C.int64_t, cRef *C.char) (result *C.char) {
	defer recoverToErrorEnvelope("BrowseAsset", &result)

	// Copy before anything else — AP31. This one is synchronous so the
	// pointer is live for the whole call, but the habit is the rule: the
	// next person to make an export async does not re-derive it.
	ref := C.GoString(cRef)

	bh := lookupBrowse(int64(handle))
	if bh == nil {
		return C.CString(`{"ok":false,"error":"unknown browse handle"}`)
	}
	asset, ok := bh.model.Asset(ref)
	if !ok {
		// One answer for three facts — refused ref, uncommitted asset,
		// failed fetch — because the renderer's move is the same in all
		// three: draw the fallback text, never a broken image. The
		// distinction that matters to an operator is on the chain, which
		// says what the signed root committed.
		b, _ := json.Marshal(map[string]any{"ok": false,
			"error": "no committed asset for that reference in the site on screen"})
		return C.CString(string(b))
	}
	b, err := json.Marshal(map[string]any{
		"ok":         true,
		"media_type": asset.MediaType,
		"bytes":      base64.StdEncoding.EncodeToString(asset.Bytes),
	})
	if err != nil {
		return C.CString(`{"ok":false,"error":"marshal asset failed"}`)
	}
	return C.CString(string(b))
}

// BrowseClose releases the handle and cancels any navigation in flight.
//
//export BrowseClose
func BrowseClose(handle C.int64_t) (result *C.char) {
	defer recoverToErrorEnvelope("BrowseClose", &result)

	h := int64(handle)
	browseMu.Lock()
	bh := browsers[h]
	delete(browsers, h)
	browseMu.Unlock()
	if bh == nil {
		return C.CString(`{"ok":true}`) // idempotent
	}
	bh.mu.Lock()
	bh.wakeCb = nil
	cancel := bh.cancelNav
	bh.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	// WAIT for anything that might still be about to call into .NET.
	// Outside the mutex: fireWake takes it.
	bh.wakeWG.Wait()
	return C.CString(`{"ok":true}`)
}
