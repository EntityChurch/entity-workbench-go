using System;
using System.Runtime.InteropServices;

namespace EntityAvalonia;

// P/Invoke surface for libbridge.so — must match the //export
// declarations in ../bridge/main.go exactly.
//
// Phase I Session 2 multi-peer pivot: every per-peer op now takes a
// leading `long peerHandle`. The C# side acquires its peer handle from
// `Bridge.DefaultPeer()` after `Bridge.Init(...)` and threads it through
// every subsequent call. Session 3 will add a peer manager UI (tab
// strip + new-peer modal) that uses PeerCreate / PeerDestroy / PeerList
// / PeerConfig directly to manage multiple peers concurrently.
public static class Bridge
{
    private const string Lib = "bridge";

    // void cb(int64_t handle, const char* event_json)
    [UnmanagedFunctionPointer(CallingConvention.Cdecl)]
    public delegate void WatchCallback(long handle, IntPtr eventJsonPtr);

    // BridgeInit boots a default peer per the JSON config. Returns NULL
    // on success; on failure returns an error string the caller must
    // release with FreeString.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "BridgeInit")]
    public static extern IntPtr Init([MarshalAs(UnmanagedType.LPStr)] string configJson);

    // BridgeDefaultPeer returns the handle of the peer booted by
    // BridgeInit (or 0 if init hasn't run). Call once post-init.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "BridgeDefaultPeer")]
    public static extern long DefaultPeer();

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "BridgeShutdown")]
    public static extern void Shutdown();

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "Hello")]
    public static extern IntPtr Hello();

    // BridgeBuildStamp returns the stamp of the tree libbridge.so was
    // compiled from. Compared against the frontend's own stamp at startup:
    // they are produced by the same podman build, so a disagreement means
    // dist-native/ holds a stale .so next to a fresh executable.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "BridgeBuildStamp")]
    public static extern IntPtr BuildStamp();

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "FreeString")]
    public static extern void FreeString(IntPtr p);

    // --- Peer manager (Phase I S2) --------------------------------------
    //
    // PeerCreate boots a new peer. Envelope: {ok, handle} or {ok:false, error}.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "PeerCreate")]
    public static extern IntPtr PeerCreate([MarshalAs(UnmanagedType.LPStr)] string configJson);

    // PeerDestroy tears down peer h. Cascades through trees + watches.
    // Envelope: {ok} or {ok:false, error}.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "PeerDestroy")]
    public static extern IntPtr PeerDestroy(long peerHandle);

    // PeerList enumerates live peers. Envelope:
    // {ok:true, peers:[{handle,peer_id,alias,identity,storage_kind,
    //                   listen,is_system,connections,added_at},...]}
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "PeerList")]
    public static extern IntPtr PeerList();

    // PeerConfig returns the bootstrap config snapshot for peer h.
    // Envelope: {ok, config:{identity,alias,storage,storage_path,listen,open_access}}.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "PeerConfig")]
    public static extern IntPtr PeerConfig(long peerHandle);

    // PeerListenAddr returns the bound listener address (if any) for
    // peer h. Envelope:
    //   {ok:true, result:{listening:true|false, scheme:"tcp"|"ws", addr:"..."}}
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "PeerListenAddr")]
    public static extern IntPtr PeerListenAddr(long peerHandle);

    // BridgeRestorePeers reads the system peer's roster and respawns
    // every non-ephemeral peer that isn't already hosted. Call after
    // Init. Envelope:
    //   {ok:true, restored:[handle,...]}
    //   {ok:true, restored:[...], warning:"N of M failed (...)"}
    //   {ok:false, error:"..."}
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "BridgeRestorePeers")]
    public static extern IntPtr RestorePeers();

    // --- Per-peer ops ---------------------------------------------------

    // DispatchLine runs a single shell input line through peer h's
    // shell. Envelope: { ok, lines: [{text,kind}...], prompt: "entity:…:/… > " }
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "DispatchLine")]
    public static extern IntPtr DispatchLine(long peerHandle, [MarshalAs(UnmanagedType.LPStr)] string line);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ShellPrompt")]
    public static extern IntPtr ShellPrompt(long peerHandle);

    // Complete returns candidate completions for the last token of the
    // line on peer h's shell. Envelope: { ok, candidates: [..], tokenStart }.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "Complete")]
    public static extern IntPtr Complete(long peerHandle, [MarshalAs(UnmanagedType.LPStr)] string line);

    // EntityGet runs `get <path>` against peer h, no prompt-echo line.
    // Envelope: { ok, lines: [{text,kind}, ...] }.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "EntityGet")]
    public static extern IntPtr EntityGet(long peerHandle, [MarshalAs(UnmanagedType.LPStr)] string path);

    // PeerSummary returns the per-peer status snapshot for h.
    // Envelope: { ok, alias, peer_id, identity, connections }.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "PeerSummary")]
    public static extern IntPtr PeerSummary(long peerHandle);

    // WatchSubscribe attaches a watch to peer h's store. Returns
    // {ok:true, handle:N} — the WATCH handle is distinct from the peer
    // handle and is what WatchUnsubscribe consumes.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "WatchSubscribe")]
    public static extern IntPtr WatchSubscribe(long peerHandle, [MarshalAs(UnmanagedType.LPStr)] string pattern, IntPtr callback);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "WatchUnsubscribe")]
    public static extern void WatchUnsubscribe(long watchHandle);

    // --- Tree-browser ops -----------------------------------------------
    //
    // Tree handles also live in a flat namespace (distinct from peer
    // handles + watch handles). TreeOpen takes the peer it's bound to;
    // every subsequent tree op takes only the tree handle.

    [UnmanagedFunctionPointer(CallingConvention.Cdecl)]
    public delegate void TreeWakeCallback(long handle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "TreeOpen")]
    public static extern IntPtr TreeOpen(long peerHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "TreeRegisterWake")]
    public static extern IntPtr TreeRegisterWake(long treeHandle, IntPtr callback);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "TreeRender")]
    public static extern IntPtr TreeRender(long treeHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "TreeToggleExpand")]
    public static extern IntPtr TreeToggleExpand(long treeHandle, int index);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "TreeSetSearch")]
    public static extern IntPtr TreeSetSearch(long treeHandle, [MarshalAs(UnmanagedType.LPStr)] string text);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "TreeClose")]
    public static extern void TreeClose(long treeHandle);

    // --- PeerInfo panel -------------------------------------------------
    //
    // Per-peer statistics (entity count, path count, sorted path list).
    // Same handle pattern as the tree: Open → handle, RegisterWake →
    // wake-fanout goroutine wired to a C callback, Render → snapshot,
    // Close → tear down. Cascades on peer destroy.

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "PeerInfoOpen")]
    public static extern IntPtr PeerInfoOpen(long peerHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "PeerInfoRegisterWake")]
    public static extern IntPtr PeerInfoRegisterWake(long peerInfoHandle, IntPtr callback);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "PeerInfoRender")]
    public static extern IntPtr PeerInfoRender(long peerInfoHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "PeerInfoClose")]
    public static extern void PeerInfoClose(long peerInfoHandle);

    // --- Log viewer panel ----------------------------------------------
    //
    // Wraps wb.LogFilterModel. Wake fires per EventLog append (via
    // EventLog.OnAppend). LogCycleDisplayLevel cycles the per-panel
    // filter; LogCycleCollectionLevel cycles the global verbosity.

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "LogOpen")]
    public static extern IntPtr LogOpen(long peerHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "LogRegisterWake")]
    public static extern IntPtr LogRegisterWake(long logHandle, IntPtr callback);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "LogRender")]
    public static extern IntPtr LogRender(long logHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "LogCycleDisplayLevel")]
    public static extern IntPtr LogCycleDisplayLevel(long logHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "LogCycleCollectionLevel")]
    public static extern IntPtr LogCycleCollectionLevel(long logHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "LogClose")]
    public static extern void LogClose(long logHandle);

    // --- Markdown view panel -------------------------------------------
    //
    // Read-mode renderer for doc/markdown-file entities. LoadPath binds
    // a path + rebinds the per-path Store.Watch. Wake fires on path
    // change + on entity content mutation. Edit/save not yet exposed.

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "MarkdownViewOpen")]
    public static extern IntPtr MarkdownViewOpen(long peerHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "MarkdownViewRegisterWake")]
    public static extern IntPtr MarkdownViewRegisterWake(long markdownHandle, IntPtr callback);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "MarkdownViewLoadPath")]
    public static extern IntPtr MarkdownViewLoadPath(long markdownHandle, [MarshalAs(UnmanagedType.LPStr)] string path);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "MarkdownViewRender")]
    public static extern IntPtr MarkdownViewRender(long markdownHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "MarkdownViewClose")]
    public static extern void MarkdownViewClose(long markdownHandle);

    // --- Markdown files panel ------------------------------------------
    //
    // Tree-shaped browser filtered to doc/markdown-file under "docs/".
    // Same open/wake/render/toggleExpand/close shape as the main tree.
    // C# fires the host's PublishSelectedPath when a leaf is selected.

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "MarkdownFilesOpen")]
    public static extern IntPtr MarkdownFilesOpen(long peerHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "MarkdownFilesRegisterWake")]
    public static extern IntPtr MarkdownFilesRegisterWake(long mdFilesHandle, IntPtr callback);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "MarkdownFilesRender")]
    public static extern IntPtr MarkdownFilesRender(long mdFilesHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "MarkdownFilesToggleExpand")]
    public static extern IntPtr MarkdownFilesToggleExpand(long mdFilesHandle, int index);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "MarkdownFilesClose")]
    public static extern void MarkdownFilesClose(long mdFilesHandle);

    // --- Query browser panel -------------------------------------------
    //
    // Pull-only — no wake source. Set filters, call Execute, render
    // result page. SelectNext/Prev navigate; NextPage advances.

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "QueryOpen")]
    public static extern IntPtr QueryOpen(long peerHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "QuerySetFilters")]
    public static extern IntPtr QuerySetFilters(long queryHandle,
        [MarshalAs(UnmanagedType.LPStr)] string typeFilter,
        [MarshalAs(UnmanagedType.LPStr)] string pathPrefix);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "QueryExecute")]
    public static extern IntPtr QueryExecute(long queryHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "QuerySelectNext")]
    public static extern IntPtr QuerySelectNext(long queryHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "QuerySelectPrev")]
    public static extern IntPtr QuerySelectPrev(long queryHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "QueryNextPage")]
    public static extern IntPtr QueryNextPage(long queryHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "QueryRender")]
    public static extern IntPtr QueryRender(long queryHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "QueryClose")]
    public static extern void QueryClose(long queryHandle);

    // --- Handler browser panel -----------------------------------------
    //
    // Wake-driven on the `system/handler/` prefix: a handler registered
    // at runtime shows up without a poll. Execution is always explicit —
    // selecting a handler or an operation dispatches nothing, only
    // ExecuteSelected / ExecuteCustom do.

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "HandlersOpen")]
    public static extern IntPtr HandlersOpen(long peerHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "HandlersRegisterWake")]
    public static extern IntPtr HandlersRegisterWake(long handlersHandle, IntPtr callback);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "HandlersRender")]
    public static extern IntPtr HandlersRender(long handlersHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "HandlersSelectHandler")]
    public static extern IntPtr HandlersSelectHandler(long handlersHandle, long index);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "HandlersSelectOperation")]
    public static extern IntPtr HandlersSelectOperation(long handlersHandle, long index);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "HandlersExecuteSelected")]
    public static extern IntPtr HandlersExecuteSelected(long handlersHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "HandlersExecuteCustom")]
    public static extern IntPtr HandlersExecuteCustom(long handlersHandle,
        [MarshalAs(UnmanagedType.LPStr)] string uri,
        [MarshalAs(UnmanagedType.LPStr)] string op,
        [MarshalAs(UnmanagedType.LPStr)] string resource);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "HandlersClose")]
    public static extern void HandlersClose(long handlersHandle);

    // --- Site view panel -----------------------------------------------
    //
    // Read-projection of the SITE convention (app/site-manifest +
    // app/site-page, v0.5). Single-shot per Navigate; wake fires on
    // model state change (Navigate, GoBack, Invalidate). Navigate's
    // envelope carries `kind` — "navigated" (model state moved) or
    // "external" (target is an http/mailto link the OS should handle).

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "SiteOpen")]
    public static extern IntPtr SiteOpen(long peerHandle,
        [MarshalAs(UnmanagedType.LPStr)] string peerId,
        [MarshalAs(UnmanagedType.LPStr)] string siteId);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "SiteRegisterWake")]
    public static extern IntPtr SiteRegisterWake(long siteHandle, IntPtr callback);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "SiteNavigate")]
    public static extern IntPtr SiteNavigate(long siteHandle,
        [MarshalAs(UnmanagedType.LPStr)] string target);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "SiteGoBack")]
    public static extern IntPtr SiteGoBack(long siteHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "SiteRender")]
    public static extern IntPtr SiteRender(long siteHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "SiteClose")]
    public static extern void SiteClose(long siteHandle);

    // Publisher verification (the CDN corridor's consume stack).
    //
    // VerifyOpen takes NO peer handle, and that is deliberate: a Mode A2
    // consumer is not a peer (EXTENSION-NETWORK §6.5.3) — no dispatch,
    // no ingest, no store. The corridor's whole point is that a stranger
    // with a URL can check a publisher's work.
    //
    // VerifyStart returns immediately and the wake fires ONCE, on
    // completion. Every other panel here wakes on tree events; this one
    // wakes on an operation the operator started. Never call
    // VerifyStart's work on the UI thread — the bridge already moved it
    // to a goroutine, which is why Start is not Render.

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "VerifyOpen")]
    public static extern IntPtr VerifyOpen();

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "VerifyRegisterWake")]
    public static extern IntPtr VerifyRegisterWake(long verifyHandle, IntPtr callback);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "VerifyConfigure")]
    public static extern IntPtr VerifyConfigure(long verifyHandle,
        [MarshalAs(UnmanagedType.LPStr)] string configJson);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "VerifyStart")]
    public static extern IntPtr VerifyStart(long verifyHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "VerifyRender")]
    public static extern IntPtr VerifyRender(long verifyHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "VerifyClose")]
    public static extern IntPtr VerifyClose(long verifyHandle);

    // The consume-side BROWSER (registry pin -> name -> page).
    //
    // Same no-peer rule as Verify above, and the same
    // operation-triggered wake. The difference worth knowing at this
    // seam: the single-flight guard is PER OPERATION, not per panel.
    // BrowseNames and BrowseGo do not block each other — clicking a name
    // while the list is still loading is a reasonable thing to do — but a
    // second BrowseGo while one is in flight is refused, because two
    // chains interleaved into one step list read as one journey.
    //
    // BrowsePin is synchronous: pinning does at most one profile fetch
    // and verifies nothing, so there is no wake to wait for. Everything
    // that actually checks something is async.

    // peerHandle is OPTIONAL: 0 opens a browser that reads static
    // origins and nothing else, which is a complete consumer and not a
    // degraded one. A real handle additionally lets a binding's live
    // transports be taken — asking the publisher directly, which is a
    // dispatch and therefore needs a peer. It buys the removal of a
    // third party, never a stronger check: the same verifier runs on
    // both roads.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "BrowseOpen")]
    public static extern IntPtr BrowseOpen(long peerHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "BrowseRegisterWake")]
    public static extern IntPtr BrowseRegisterWake(long browseHandle, IntPtr callback);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "BrowsePin")]
    public static extern IntPtr BrowsePin(long browseHandle,
        [MarshalAs(UnmanagedType.LPStr)] string pinJson);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "BrowseNames")]
    public static extern IntPtr BrowseNames(long browseHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "BrowseGo")]
    public static extern IntPtr BrowseGo(long browseHandle,
        [MarshalAs(UnmanagedType.LPStr)] string address);

    // BrowseFollow takes the RAW href from the rendered markdown. Do not
    // resolve it here: `support.md` -> page `support`, `../notes/x.md`
    // relative to the current page's directory, `site:other` -> a
    // different site on the same peer. Those rules are Layer-2 contract
    // shared with entity-browser-rust and live in workbench/site_model.go.
    // BrowseAutoPin pins the configured registry and enumerates it.
    // Async — it does network I/O, so it must NOT be called on the
    // constructor path synchronously. Config precedence:
    // WB_REGISTRY_ORIGIN/PEER > ~/.entity/browser.json > built-in.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "BrowseAutoPin")]
    public static extern IntPtr BrowseAutoPin(long browseHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "BrowseFollow")]
    public static extern IntPtr BrowseFollow(long browseHandle,
        [MarshalAs(UnmanagedType.LPStr)] string target);

    // BrowseAsset resolves one embedded figure of the page on screen and
    // returns {ok, media_type, bytes(base64)}.
    //
    // Like BrowseFollow, the ref goes across UNINTERPRETED. Deciding
    // whether `assets/figures/x.png` may be fetched — and that
    // `https://tracker/x.png` may not — is workbench.AssetNameFromRef,
    // which is the security gate and is Layer-2 contract shared with
    // entity-browser-rust. A C# copy of it would be a second
    // implementation of a rule whose failure mode is this process
    // fetching a URL a page body chose.
    //
    // SYNCHRONOUS and does I/O on a cache miss. Call it from a worker,
    // never from the UI thread — MarkdownRenderer hands the panel a
    // callback and the panel is what puts it on a Task.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "BrowseAsset")]
    public static extern IntPtr BrowseAsset(long browseHandle,
        [MarshalAs(UnmanagedType.LPStr)] string reference);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "BrowseBack")]
    public static extern IntPtr BrowseBack(long browseHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "BrowseForward")]
    public static extern IntPtr BrowseForward(long browseHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "BrowseRender")]
    public static extern IntPtr BrowseRender(long browseHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "BrowseClose")]
    public static extern IntPtr BrowseClose(long browseHandle);

    // The three per-program panels (Snake / Life / Asteroids) that used to
    // sit here were retired 2026-08-20 along with their bridge surfaces.
    // The generic host below drives all three from descriptors, including
    // the per-program status readout, so the per-program exports were 21
    // functions expressing what ProgramMount expresses in one.

    // --- The generic compute-program host -------------------------------
    //
    // ONE seam for every compute program, replacing what the three blocks
    // above do per-program. The split is deliberate and visible:
    //
    //   ProgramAuthor(peer, name) -> descriptor path   ; per-program, once
    //   ProgramMount(peer, path)  -> handle            ; generic, always
    //
    // A front-end holding a descriptor fetched from another peer by hash
    // calls ProgramMount alone and never touches ProgramAuthor. That is the
    // transferable-compute story in two signatures.
    //
    // ProgramRender returns every output port decoded BY SHAPE, so the C#
    // side binds shapes (text, display-list) and never a program.

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ProgramAuthor")]
    public static extern IntPtr ProgramAuthor(long peerHandle,
        [MarshalAs(UnmanagedType.LPUTF8Str)] string name);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ProgramMount")]
    public static extern IntPtr ProgramMount(long peerHandle,
        [MarshalAs(UnmanagedType.LPUTF8Str)] string descriptorPath);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ProgramRegisterWake")]
    public static extern IntPtr ProgramRegisterWake(long programHandle, IntPtr callback);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ProgramStart")]
    public static extern IntPtr ProgramStart(long programHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ProgramStop")]
    public static extern IntPtr ProgramStop(long programHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ProgramRestart")]
    public static extern IntPtr ProgramRestart(long programHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ProgramRender")]
    public static extern IntPtr ProgramRender(long programHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ProgramInputKeys")]
    public static extern IntPtr ProgramInputKeys(long programHandle,
        [MarshalAs(UnmanagedType.LPUTF8Str)] string portName, long keys);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ProgramInputDirection")]
    public static extern IntPtr ProgramInputDirection(long programHandle,
        [MarshalAs(UnmanagedType.LPUTF8Str)] string portName, long dir);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ProgramClose")]
    public static extern void ProgramClose(long programHandle);

    // --- Per-panel shell ------------------------------------------------
    //
    // PHASE-I-DESKTOP-RENDERER-PLAN §I.5 — each ShellPanel owns its
    // own shellcmd.Shell over the peer's shared workspace, so multiple
    // panels can have independent WD / history / completion. The peer-
    // keyed DispatchLine / Complete / ShellPrompt above remain for
    // programmatic ingress paths (smoke driver, tests).

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ShellOpen")]
    public static extern IntPtr ShellOpen(long peerHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ShellClose")]
    public static extern void ShellClose(long shellHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ShellDispatchLine")]
    public static extern IntPtr ShellDispatchLine(long shellHandle,
        [MarshalAs(UnmanagedType.LPStr)] string line);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ShellComplete")]
    public static extern IntPtr ShellComplete(long shellHandle,
        [MarshalAs(UnmanagedType.LPStr)] string line);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ShellPromptForHandle")]
    public static extern IntPtr ShellPromptForHandle(long shellHandle);

    // --- Peer connections panel ---------------------------------------
    //
    // PHASE-I-PEER-CONNECTIONS-PLAN B-1. Wakes fire whenever the
    // peer's `system/peer/transport/` prefix changes (i.e. any
    // Connect/Disconnect from this panel, a shell panel, or any other
    // surface). Connect/Disconnect dispatch through the shared shell
    // workspace, so alias bindings are visible to ShellPanels on the
    // same peer.

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ConnectionsOpen")]
    public static extern IntPtr ConnectionsOpen(long peerHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ConnectionsRegisterWake")]
    public static extern IntPtr ConnectionsRegisterWake(long connsHandle, IntPtr callback);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ConnectionsRender")]
    public static extern IntPtr ConnectionsRender(long connsHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ConnectionsConnect")]
    public static extern IntPtr ConnectionsConnect(long connsHandle,
        [MarshalAs(UnmanagedType.LPStr)] string alias,
        [MarshalAs(UnmanagedType.LPStr)] string address);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ConnectionsDisconnect")]
    public static extern IntPtr ConnectionsDisconnect(long connsHandle,
        [MarshalAs(UnmanagedType.LPStr)] string alias);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ConnectionsClose")]
    public static extern void ConnectionsClose(long connsHandle);

    // --- Discovery (mDNS) -----------------------------------------------
    // Handle lifecycle mirrors Connections* — Open returns {ok,handle};
    // RegisterWake hooks the C# OnWake delegate; Render returns the
    // current "Nearby peers" snapshot; Close tears down. The handle is
    // scoped to one peer; auto-cleaned when the peer is destroyed via
    // the bridge's cascadeDiscoveries OnPeerDestroyed hook.

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "DiscoveryOpen")]
    public static extern IntPtr DiscoveryOpen(long peerHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "DiscoveryRegisterWake")]
    public static extern IntPtr DiscoveryRegisterWake(long discoveryHandle, IntPtr callback);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "DiscoveryRender")]
    public static extern IntPtr DiscoveryRender(long discoveryHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "DiscoveryClose")]
    public static extern void DiscoveryClose(long discoveryHandle);

    // --- Peer liveness (system/peer/status) ------------------------------
    // The TREE's lifecycle record, not the connection pool. Handle
    // lifecycle mirrors Connections*/Discovery*. Wakes fire on every
    // lifecycle transition (EXTENSION-NETWORK §3.13) — including the
    // demotion to `suspect`, which writes the status entity and nothing
    // else, so a Connections wake would never see it.
    //
    // Rows carry status/reason/last_error/connected_at/failing_since.
    // There is deliberately NO last_seen: the status entity is
    // transition-written (§5.4.1 MUST), so the field is a snapshot taken
    // at the transition, and rendering it as "last heard from" would
    // invent a freshness contract the protocol does not offer.

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "LivenessOpen")]
    public static extern IntPtr LivenessOpen(long peerHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "LivenessRegisterWake")]
    public static extern IntPtr LivenessRegisterWake(long livenessHandle, IntPtr callback);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "LivenessRender")]
    public static extern IntPtr LivenessRender(long livenessHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "LivenessClose")]
    public static extern void LivenessClose(long livenessHandle);

    // Local files: one call, no handle. The model is stateless and mounts
    // have no event source to wake on — see avalonia/bridge/local_files.go
    // for why this surface deliberately does not follow the
    // Open/RegisterWake/Render/Close shape the panels above use.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "LocalFilesRender")]
    public static extern IntPtr LocalFilesRender(long peerHandle);

    // Mutating local-files surface. Synchronous on purpose: the work is
    // operator-initiated and once-per-session, and the async cgo shape
    // carries AP31's use-after-free hazard (a *C.char belongs to the .NET
    // marshaller and is freed when the P/Invoke returns, so an export that
    // reads it on a goroutine reads freed memory — and does NOT crash, it
    // reads as the empty string and surfaces as a plausible user error).
    // Being synchronous is what makes passing these strings safe.
    //
    // excludeSet distinguishes "the caller named no exclude patterns" from
    // "the caller asked for no exclusions". Collapsing them would silently
    // ingest a .git directory on a mount that meant to take the defaults.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "LocalFilesMount", CharSet = CharSet.Ansi)]
    public static extern IntPtr LocalFilesMount(long peerHandle, string fsDir, string treePrefix,
        string includeCsv, string excludeCsv, long excludeSet, long force, long readOnly);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "LocalFilesUnmount", CharSet = CharSet.Ansi)]
    public static extern IntPtr LocalFilesUnmount(long peerHandle, string rootName);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "LocalFilesSweep", CharSet = CharSet.Ansi)]
    public static extern IntPtr LocalFilesSweep(long peerHandle, string rootName, long addMissing);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "LocalFilesDefaultExclude")]
    public static extern IntPtr LocalFilesDefaultExclude();

    // --- Share / sync (the folder-sharing flow) --------------------------
    //
    // One export per ShellWorkspace operation, same operation the shell
    // verb calls. No handle: shares and syncs are config, written when an
    // operator acts and then still — the same reason LocalFilesRender is
    // handle-free. Mount CONTENTS churn and have a handle; mount
    // AGREEMENTS do not.
    //
    // ShareOffers and ShareAccept reach the NETWORK — they re-establish a
    // connection and dispatch to a remote peer, so they can take seconds.
    // Both are still synchronous, per AP31 (an async cgo export would have
    // to copy every C-owned argument into Go memory before launching its
    // goroutine, and a `*C.char` read on the goroutine is a use-after-free
    // that reads as the empty string). SharePanel keeps the UI alive by
    // calling them on a thread-pool worker instead.

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ShareRender")]
    public static extern IntPtr ShareRender(long peerHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ShareOffers", CharSet = CharSet.Ansi)]
    public static extern IntPtr ShareOffers(long peerHandle, string peer);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ShareCreate", CharSet = CharSet.Ansi)]
    public static extern IntPtr ShareCreate(long peerHandle, string root, string peer, string title);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ShareRevoke", CharSet = CharSet.Ansi)]
    public static extern IntPtr ShareRevoke(long peerHandle, string root, string peer);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ShareAccept", CharSet = CharSet.Ansi)]
    public static extern IntPtr ShareAccept(long peerHandle, string peer, string root,
        string directory, int allowNonEmpty);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ShareUnsync", CharSet = CharSet.Ansi)]
    public static extern IntPtr ShareUnsync(long peerHandle, string peer, string root);

    // ShareResync reaches the NETWORK — it lists the remote folder and
    // pulls blob closures — so it belongs off the UI thread with the
    // other two, not with the local render calls.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ShareResync", CharSet = CharSet.Ansi)]
    public static extern IntPtr ShareResync(long peerHandle, string peer, string root);

    // ShareForget is local-only (it drops policy, bindings and the
    // connection) but is run off-thread anyway: Unsync inside it closes
    // subscriptions, and a subscription close waits on a delivery
    // goroutine (AP60).
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ShareForget", CharSet = CharSet.Ansi)]
    public static extern IntPtr ShareForget(long peerHandle, string peer);

    // The reciprocal dial. Pass address="" to let the workspace resolve
    // one (a peer we have dialled before, or an mDNS announcement); pass
    // a typed address when it reports needsAddress. Network-bound.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "ShareComplete", CharSet = CharSet.Ansi)]
    public static extern IntPtr ShareComplete(long peerHandle, string peer, string address);

    // --- Sharing status (declared state vs. what is actually true) -------
    //
    // Two exports, and the split is the whole design. StatusRender READS
    // the declarations and observes the substrate — no write, no dial, so
    // it is safe from a refresh or a wake. StatusReconcile runs one pass
    // of the control loop, which DIALS every declared peer; wiring that to
    // anything automatic would turn a status panel into a dialer an
    // operator leaves running. It is called on open, on Re-check, and
    // after a mutation this panel performed, and never otherwise.
    //
    // Both return the same envelope, carrying `reconciled` — so the
    // surface says which of the two it is showing rather than implying the
    // stronger one.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "StatusRender")]
    public static extern IntPtr StatusRender(long peerHandle);

    // Network-bound (it dials): run it on a thread-pool worker, like
    // ShareOffers / ShareAccept and for the same AP31 reason.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "StatusReconcile")]
    public static extern IntPtr StatusReconcile(long peerHandle);

    // StatusCatchUp runs ONE backfill pass over every folder this peer
    // receives, then returns the same envelope as StatusRender.
    //
    // **Not the same hazard as StatusReconcile, and that is why it is a
    // separate export.** A catch-up uses the sync binding and the pooled
    // connection: it does not dial, does not mount, does not delete and
    // does not write policy. It transfers files, though, so it is
    // I/O-bound and belongs on a thread-pool worker for AP31's reason.
    //
    // It is the GUI's half of the `catchup` verb. Until it existed, the
    // machinery that turns a dropped delivery into a delay rather than a
    // loss was reachable from the shell and from no pixel — on the one
    // failure an operator is most likely to meet.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "StatusCatchUp")]
    public static extern IntPtr StatusCatchUp(long peerHandle);

    // Decide one conflict: keep is "mine", "theirs" or "both". Returns the
    // same envelope as StatusRender, so the table and the sentence come
    // from one reading — a note-only reply would leave the resolved row on
    // screen until the next refresh, which reads as the button doing
    // nothing.
    //
    // Local work, no dial. Thread-pool worker anyway: it writes a file
    // whose size nobody here chose.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "StatusResolveConflict", CharSet = CharSet.Ansi)]
    public static extern IntPtr StatusResolveConflict(long peerHandle, string key, string keep);

    // Declare what happens when another peer's change lands on a file you
    // edited: policy is "record" or "keep-both", with no default at this
    // boundary for the reason above — the two outcomes differ in whether
    // the folder keeps converging, which is not a choice to make silently.
    //
    // REFUSES on a folder this peer does not own. A shared folder names
    // one rule and it is the owner's, so the Go side answers with the
    // sentence naming the machine to run it on rather than writing a
    // field nothing reads. Returns the StatusRender envelope, same as
    // StatusResolveConflict and for the same reason.
    //
    // A declaration only: no dial, no re-handshake, no reconcile pass.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "StatusSetConflictRule", CharSet = CharSet.Ansi)]
    public static extern IntPtr StatusSetConflictRule(long peerHandle, string folderId, string policy);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "StatusPauseDevice", CharSet = CharSet.Ansi)]
    public static extern IntPtr StatusPauseDevice(long peerHandle, string peer, int paused);

    // No directory parameter, on purpose: a remount uses the path the
    // DECLARATION remembers, because that record is the only thing left
    // that can say where a folder's files are once the mount that knew is
    // gone.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "StatusRemountFolder", CharSet = CharSet.Ansi)]
    public static extern IntPtr StatusRemountFolder(long peerHandle, string folderId);

    // Which way a shared folder flows on THIS peer: send / receive / both.
    //
    // S6 made FolderData.Mode the field the reconciler branches on. Before
    // it, Mode had two writers, two readers, and nothing consulting it —
    // both readers put it in a status DTO. The tell worth remembering is a
    // field whose only readers are serializers: stored faithfully,
    // displayed faithfully, invisible to every test at every layer.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "StatusSetFolderDirection", CharSet = CharSet.Ansi)]
    public static extern IntPtr StatusSetFolderDirection(long peerHandle, string folderId, string mode);

    // --- Publishing (the produce side) -----------------------------------
    //
    // Same split as the pair above and for a sharper reason. StatusRender
    // is safe on a wake because it does not dial; PublishRender is safe on
    // a wake because it does not MINT. A publish signs a new root and
    // increments `seq`, so wiring it to a wake would make an open window
    // announce a new release of the site every time it was focused.
    //
    // Both return the same envelope, carrying `minted`, so the surface
    // says which it is showing rather than implying the stronger one.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "PublishRender")]
    public static extern IntPtr PublishRender(long peerHandle);

    // public: 1 writes the public grant, -1 removes it, 0 leaves it alone.
    //
    // A tri-state and not a bool, because a bool makes every re-publish
    // restate a disclosure decision — so pressing "Publish" to pick up a
    // new page would silently un-publish the site, which is a change to
    // who can read it made by a button that does not say so.
    //
    // feed: non-zero asks for A-38 ruling (D)'s curated binding set —
    // publish at the peer root, the only prefix containing both the feed
    // and the `system/signature/…` keys that attribute an entry, and commit
    // to exactly the entries, the index and each entry's signature.
    //
    // A SECOND PARAMETER and not a mode enum: "what does this root commit
    // to" and "who may read it" are independent questions, and folding them
    // together would make "publish my feed" also restate a disclosure
    // decision — the mistake the tri-state above exists to avoid.
    //
    // Walks the tree and, with a grant change, re-handshakes every
    // declared peer. Thread-pool worker, like StatusReconcile and for the
    // same AP31 reason.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "PublishNow")]
    public static extern IntPtr PublishNow(long peerHandle, int makePublic, int feed);

    // --- Feeds: following, reading a timeline, resolving a reference ------
    //
    // THREE SAFETY CLASSES, and the split is load-bearing. `status.go`
    // draws the same line and this one is sharper, because two of the
    // three reach other machines.
    //
    //   FeedFollowsRender   reads THIS peer's tree. Dials nobody. Safe on
    //                       a wake, and it IS on one — follows are tree
    //                       data, and a refresh button on tree data is a
    //                       bug report about a missing subscription.
    //   FeedTimelineRead    DIALS every followed publisher. Must never be
    //                       on a wake or a timer: an open panel would
    //                       become a thing that contacts everyone you
    //                       follow whenever the window is focused.
    //   FeedTimelineCatchUp dials AND moves durable cursors. Separate from
    //                       the read so a surface somebody refreshes
    //                       cannot quietly change durable state.
    //
    // `advanced` in the reply means a position MOVED, not that the
    // catch-up entry point was the one called.
    //   FeedOwnRender       reads THIS peer's OWN feed and whether anyone
    //                       else can read it. Dials nobody, mints nothing.
    //                       Safe on a wake — and note it deliberately does
    //                       not publish: a surface that refreshed by
    //                       minting would bump `seq` every time somebody
    //                       looked at it. The act is PublishNow(…, feed: 1).
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "FeedOwnRender")]
    public static extern IntPtr FeedOwnRender(long peerHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "FeedFollowsRender")]
    public static extern IntPtr FeedFollowsRender(long peerHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "FeedFollowPeer")]
    public static extern IntPtr FeedFollowPeer(long peerHandle, string subject, string label);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "FeedUnfollowPeer")]
    public static extern IntPtr FeedUnfollowPeer(long peerHandle, string subject);

    // subject empty = every follow; limit 0 = everything the read reaches.
    // Thread-pool worker: one round trip per followed publisher, and a
    // followed peer that is switched off is the normal case.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "FeedTimelineRead")]
    public static extern IntPtr FeedTimelineRead(long peerHandle, string subject, int limit);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "FeedTimelineCatchUp")]
    public static extern IntPtr FeedTimelineCatchUp(long peerHandle, string subject, int limit);

    // One reference, and WHICH of APP-CONVENTION-FEED §2.2.2's outcomes it
    // got. FEED-R7 is a MUST about the ability to TELL, so the reply
    // carries `row` and `moved` both — an enum a caller can forget a case
    // of, and a boolean a provenance line reads directly.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "FeedResolveRef")]
    public static extern IntPtr FeedResolveRef(long peerHandle, string reference);

    // --- The declared-state wake -----------------------------------------
    //
    // The sharing surfaces were the only three panels in the app with no
    // tree subscription, so they were the only three that needed a Refresh
    // button — 12 of 15 panels updated themselves and these did not. The
    // state behind them is ordinary watchable tree entities.
    //
    // The callback receives the PEER handle, so a panel calls StatusRender
    // or ShareRender with the value it already holds. Only the READ is
    // wired: StatusReconcile dials, and it writes to the tree, so wiring it
    // here would both make a dialer out of a status surface and wake itself
    // forever.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "SharingRegisterWake")]
    public static extern IntPtr SharingRegisterWake(long peerHandle, IntPtr callback);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "SharingUnregisterWake")]
    public static extern IntPtr SharingUnregisterWake(long peerHandle, long registration);

    // File explorer: one mount's CONTENTS, and unlike LocalFilesRender
    // above this one IS a handle. The distinction is the event source. A
    // mount's config is written once and does not move; a mount's
    // contents change on every save in a watched directory and hundreds
    // of times a second during a watcher's initial scan. The model owns
    // two prefix subscriptions, so it needs the wake shape.
    //
    // Wakes reuse TreeWakeCallback — the signature is identical and a
    // second delegate type would be two things to keep in step. The
    // delegate MUST be held in a GCHandle for the handle's lifetime: the
    // field reference alone is not enough, and a collected delegate makes
    // the process abort rather than throw.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "FileExplorerOpen")]
    public static extern IntPtr FileExplorerOpen(long peerHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "FileExplorerRegisterWake")]
    public static extern IntPtr FileExplorerRegisterWake(long explorerHandle, IntPtr callback);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "FileExplorerSetRoot", CharSet = CharSet.Ansi)]
    public static extern IntPtr FileExplorerSetRoot(long explorerHandle, string rootName);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "FileExplorerSetDir", CharSet = CharSet.Ansi)]
    public static extern IntPtr FileExplorerSetDir(long explorerHandle, string dir);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "FileExplorerUp")]
    public static extern IntPtr FileExplorerUp(long explorerHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "FileExplorerRender")]
    public static extern IntPtr FileExplorerRender(long explorerHandle);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "FileExplorerPreview", CharSet = CharSet.Ansi)]
    public static extern IntPtr FileExplorerPreview(long explorerHandle, string relPath);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "FileExplorerClose")]
    public static extern void FileExplorerClose(long explorerHandle);

    // Workspace layout. Takes an ALIAS, not a peer handle: the layout has
    // to be readable before a peer's panels exist, and it is keyed by
    // alias because an ephemeral peer gets a fresh peer-id every launch —
    // a peer-id-keyed layout would never match itself twice and the
    // feature would silently do nothing in the default configuration.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "LayoutLoad", CharSet = CharSet.Ansi)]
    public static extern IntPtr LayoutLoad(string alias);

    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "LayoutSave", CharSet = CharSet.Ansi)]
    public static extern IntPtr LayoutSave(string alias, string panelsJson, double navWidth);

    // In-process redirect of the layout file. Setting WB_LAYOUT from
    // managed code does NOT work — Go captures its environment at process
    // start, so setenv never reaches os.Getenv in the bridge — which is
    // why this export exists and why the headless suite uses it instead
    // of an environment variable to stay out of the real ~/.entity.
    [DllImport(Lib, CallingConvention = CallingConvention.Cdecl, EntryPoint = "LayoutSetPath", CharSet = CharSet.Ansi)]
    public static extern IntPtr LayoutSetPath(string path);

    // TakeString copies a C-string allocated by Go into a managed
    // string and immediately frees the Go-side allocation. Returns
    // empty string when given IntPtr.Zero (Go's NULL return).
    public static string TakeString(IntPtr p)
    {
        if (p == IntPtr.Zero) return "";
        var s = Marshal.PtrToStringAnsi(p) ?? "";
        FreeString(p);
        return s;
    }
}
