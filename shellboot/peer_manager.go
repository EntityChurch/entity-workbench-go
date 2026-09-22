// peer_manager.go — renderer-agnostic multi-peer registry.
//
// PeerManager owns the in-process peer set for a workbench-go
// application. First peer added is the system peer (it owns the
// application-state namespace + the roster); subsequent peers are
// ordinary roster entries. Concurrency-safe; methods are callable
// from any goroutine.
//
// Per PHASE-I-MULTI-PEER-PLAN.md §12 — moved from avalonia/bridge/
// where it shouldn't have been, into shellboot/ where the shared
// peer-construction logic already lives. Avalonia + console both
// construct a PeerManager and consume the same surface; renderer-
// specific resource cleanup (cgo tree+watch handle maps for the
// bridge, tview model wrappers for console) hooks in via
// OnPeerDestroyed.
//
// Compare with godot core/app_state.gd (SIBLING-FRONTEND-SURVEY §3.1) —
// same shape: dict keyed by peer-id-equivalent, system peer slot,
// signals/hooks for add/remove.

package shellboot

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/shellcmd"
)

// HostedPeer is the public per-peer struct callers receive from
// PeerManager. The struct is a snapshot — callers should not mutate
// fields; for live state, call back through the manager.
//
// Fields:
//
//	Handle    — opaque int64 assigned by the manager at Create time.
//	            Stable for the peer's lifetime.
//	AppPeer   — the entitysdk peer (Store, PeerContext, Put/Get/...).
//	Workspace — the shellcmd workspace owning the local-peer alias
//	            + remote-peer connection pool.
//	Shell     — a pre-constructed shellcmd.Shell, WD set to the
//	            peer's canonical root path.
//	Config    — the shellboot.Config the peer was Create'd with.
//	AddedAt   — unix-millis timestamp of Create.
//	IsSystem  — true iff this peer is the manager's system peer at
//	            the snapshot moment (system handle can shift on
//	            Destroy + handover).
type HostedPeer struct {
	Handle    int64
	AppPeer   *entitysdk.AppPeer
	Workspace *shellcmd.ShellWorkspace
	Shell     *shellcmd.Shell
	Config    Config
	AddedAt   int64
	IsSystem  bool

	// ListenScheme is "tcp" or "ws" when Config.ListenAddr is set and
	// the listener bound successfully. Empty when the peer is
	// outbound-only or the listen attempt failed. Set at Create time;
	// stable for the peer's lifetime.
	ListenScheme string

	// AdvertisedURL is the dial address self-published as this peer's
	// transport profile once the listener bound, or empty when nothing
	// was advertised. AdvertiseErr carries why, when the reason is not
	// simply "no listener": it is NOT fatal to Create, because a peer
	// that listens but does not advertise is exactly the pre-2026-08-18
	// behaviour — reachable by anyone told out-of-band, invisible to
	// anyone who was not.
	AdvertisedURL string
	AdvertiseErr  error

	// Listener is the full bring-up result — bound address, whether the
	// port fell back, and both non-fatal failures — for surfaces that
	// want more than the three flattened fields above. nil when the peer
	// is outbound-only.
	//
	// The flattened fields are kept because two renderers and the roster
	// already read them; this is the one an operator-facing surface
	// should prefer, because ListenScheme and AdvertisedURL together
	// still cannot say "the port you asked for was taken".
	Listener *ListenerInfo

	// listenCancel cancels the auto-Listen goroutine. nil for
	// outbound-only peers. Called during Destroy, before AppPeer.Close.
	listenCancel func()
}

// PeerManager is the per-process multi-peer registry. One per
// frontend; not a global. Concurrent-safe.
type PeerManager struct {
	appID string

	mu           sync.Mutex
	peers        map[int64]*HostedPeer
	counter      int64
	systemHandle int64

	// onDestroyHooks fire during Destroy AFTER the peer is removed
	// from the registry but BEFORE its AppPeer is closed. Renderers
	// use this to cascade-clean their own resources (bridge: trees +
	// watches keyed by peer-handle; console: tview model wrappers).
	//
	// Hooks fire WITHOUT the manager's mutex held — they can safely
	// call back into the manager.
	onDestroyHooks []func(handle int64)
}

// NewPeerManager constructs an empty manager for app-id appID
// (e.g. "entity-workbench"). The app-id namespaces every roster
// entry under the system peer's tree at app/{appID}/system/peers/.
func NewPeerManager(appID string) *PeerManager {
	return &PeerManager{
		appID: appID,
		peers: map[int64]*HostedPeer{},
	}
}

// AppID returns the namespace the manager writes roster entries
// under.
func (m *PeerManager) AppID() string { return m.appID }

// Create boots a new peer per cfg and registers it. The first
// successful Create establishes the system peer (handle 1 if no
// prior Creates have happened).
//
// Returns the assigned handle. On Bootstrap failure returns
// (0, error). On roster-write failure returns (handle, error) —
// the peer is live in-memory; only the durable mirror failed.
// Callers that want strict success should treat (handle != 0, err)
// as a warning.
func (m *PeerManager) Create(cfg Config) (int64, error) {
	ap, ws, err := Bootstrap(context.Background(), cfg)
	if err != nil {
		return 0, fmt.Errorf("shellboot.Bootstrap: %w", err)
	}

	sh := shellcmd.NewShellInWorkspace(ws)
	// Pre-cd to the local peer's canonical root so relative paths
	// work without a leading `cd @<alias>`. WD stays in canonical
	// peer-id form (`/{peerID}/`); alias substitution happens at
	// display time. Per feedback-shell-wd-canonical-form: never
	// store the alias-display form here.
	sh.SetWD(shellcmd.Path("/" + ws.Local.PeerID + "/"))

	h := atomic.AddInt64(&m.counter, 1)
	now := time.Now().UnixMilli()

	hp := &HostedPeer{
		Handle:    h,
		AppPeer:   ap,
		Workspace: ws,
		Shell:     sh,
		Config:    cfg,
		AddedAt:   now,
	}

	// Bind, advertise and announce — all three in BringUpListener, which
	// is shared with the frontends that do NOT go through this manager.
	// It used to live here in full, and `entity-shell` (which calls
	// Bootstrap directly) therefore accepted a -listen flag and bound
	// nothing at all. See shellboot/listener.go.
	if li, err := BringUpListener(context.Background(), ap, cfg); err != nil {
		_ = ap.Close()
		return 0, err
	} else if li != nil {
		hp.Listener = li
		hp.ListenScheme = li.Scheme
		hp.listenCancel = li.Cancel
		hp.AdvertisedURL = li.AdvertisedURL
		hp.AdvertiseErr = li.AdvertiseErr
	}

	var systemPeerForRoster *entitysdk.AppPeer
	m.mu.Lock()
	m.peers[h] = hp
	firstPeer := m.systemHandle == 0
	if firstPeer {
		m.systemHandle = h
	}
	if sp, ok := m.peers[m.systemHandle]; ok {
		systemPeerForRoster = sp.AppPeer
	}
	m.mu.Unlock()

	// Write the roster entry under the system peer's tree. Done with
	// the manager mutex released — tree writes hit dispatch + could
	// otherwise deadlock if a hook fired during the write tried to
	// re-enter the manager.
	if systemPeerForRoster != nil {
		entry := RosterEntry{
			PeerID:      ap.PeerID(),
			Label:       ws.Local.Alias,
			AddedAt:     now,
			IsFavorite:  false,
			Identity:    cfg.Identity,
			StorageKind: cfg.StorageKind,
			StoragePath: cfg.StoragePath,
			ListenAddr:  cfg.ListenAddr,
		}
		if werr := WriteRosterEntry(systemPeerForRoster, m.appID, entry); werr != nil {
			return h, fmt.Errorf("peer created (handle %d) but roster write failed: %w", h, werr)
		}
	}

	// Re-establish everything this peer declared. In the BACKGROUND, and
	// that is the whole design decision here: a pass dials, a declared
	// peer being switched off is the normal case rather than the
	// exceptional one, and doing it inline would make how long the app
	// takes to open depend on whether another machine happens to be
	// awake. Nothing on screen needs the result to render.
	if cfg.ReconcileOnStart {
		go reconcileAtStartup(ws, ws.Local.Alias)
	}
	return h, nil
}

// reconcileAtStartup runs one pass and reports it on stderr.
//
// stderr rather than a return value because this runs after Create has
// answered, and stderr is where the frontends' run logs already are —
// `avalonia/run-logs/`, which is the artifact anyone diagnosing this
// reads. Problems are printed individually and actions are summarized:
// an operator needs to know that eight things could not be established
// far more than which four were re-established silently and correctly.
func reconcileAtStartup(ws *shellcmd.ShellWorkspace, alias string) {
	out, err := ws.ReconcileWithTimeout(startupReconcileTimeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: [%s] startup reconcile failed: %v\n", alias, err)
		return
	}
	if len(out.Devices) == 0 && len(out.Folders) == 0 {
		return // nothing declared: say nothing
	}
	fmt.Fprintf(os.Stderr, "[%s] reconciled %d peer(s), %d folder(s)\n",
		alias, len(out.Devices), len(out.Folders))
	for _, a := range out.Actions {
		fmt.Fprintf(os.Stderr, "[%s]   %s\n", alias, a)
	}
	for _, p := range out.Problems {
		fmt.Fprintf(os.Stderr, "warning: [%s] %s\n", alias, p)
	}
}

// startupReconcileTimeout bounds the background pass. Generous, because
// it is off the critical path and a slow LAN is not an error; bounded,
// because an unbounded goroutine holding a dial gate is how a shutdown
// hangs.
const startupReconcileTimeout = 60 * time.Second

// Destroy tears down peer h. Cascade order:
//  1. Remove from in-memory registry; demote system-peer if it was h.
//  2. Promote a replacement system peer if any survivors remain.
//  3. Fire OnPeerDestroyed hooks (renderer cascades own resources).
//  4. Remove the roster entry from the (possibly-new) system peer's tree.
//  5. Close the AppPeer.
//
// Idempotent — unknown handles return nil.
func (m *PeerManager) Destroy(h int64) error {
	m.mu.Lock()
	hp, ok := m.peers[h]
	if ok {
		delete(m.peers, h)
	}
	wasSystem := h == m.systemHandle
	if wasSystem {
		m.systemHandle = 0
		// Map iter order is random; at single-survivor scope this is
		// fine. Multi-survivor handover policy can be made
		// deterministic if a use case demands it.
		for id := range m.peers {
			m.systemHandle = id
			break
		}
	}
	hooks := append([]func(int64){}, m.onDestroyHooks...)
	var newSystemPeer *entitysdk.AppPeer
	if sp, ok := m.peers[m.systemHandle]; ok {
		newSystemPeer = sp.AppPeer
	}
	m.mu.Unlock()
	if !ok {
		return nil
	}

	// Fire cascade hooks BEFORE closing the AppPeer — renderers
	// touching the peer's store during cleanup need it live.
	for _, cb := range hooks {
		cb(h)
	}

	// Cancel the auto-Listen goroutine if one is running. AppPeer.Close
	// also tears down the listener, but cancelling first lets the
	// listen goroutine return cleanly (avoids the "use of closed
	// network connection" log spam).
	if hp.listenCancel != nil {
		hp.listenCancel()
	}

	// Stop the catch-up supervisor, and stop it BEFORE the AppPeer
	// closes underneath it.
	//
	// Bootstrap starts this loop on context.Background() on purpose — it
	// is process-lifetime and must not be cancelled by whatever context
	// happened to be in scope at startup — which means nothing cancels
	// it when a peer is destroyed rather than when the process exits.
	// So until this line existed, destroying a peer left a goroutine
	// running passes against a closed store every 5 s to 10 min, for the
	// life of the process, once per peer the operator ever removed. It
	// is a leak that gets worse the more an operator uses the feature,
	// and nothing reports it: a pass that errors is not recorded, so the
	// supervisor of a dead peer is invisible in `status` too.
	//
	// StopCatchUp WAITS for an in-flight pass, which is the behaviour we
	// want here even though it makes Destroy take as long as one pass: a
	// pass reading a store that closes mid-read is the alternative. A
	// settled pass costs 0.24 ms/file, so this is sub-second on any
	// folder an operator would recognise (SYNC-LIMITS-AND-FAILURE-MODES).
	hp.Workspace.StopCatchUp()

	// And the recording guard's prefix watch, for the same reason one
	// line up: it is attached to a store that is about to close, and
	// nothing else cancels it. Cheaper to leak than the supervisor — a
	// watch with no writer delivers nothing — but a leak that is only
	// quiet is still a leak, and it holds the destroyed peer's store
	// alive through the SDK's watch hub.
	hp.Workspace.StopHistoryBudget()

	// Remove the roster entry. If the destroyed peer WAS the system
	// peer and there's no replacement, skip (the tree we'd write to
	// is going away anyway). If there IS a replacement, the old roster
	// entries currently live on the dying peer's tree — that's a known
	// limitation (the new system peer doesn't inherit them in-memory
	// here; RestoreFromRoster on next boot reconstructs them).
	if !wasSystem && newSystemPeer != nil {
		_ = RemoveRosterEntry(newSystemPeer, m.appID, hp.AppPeer.PeerID())
	}

	if hp.AppPeer != nil {
		_ = hp.AppPeer.Close()
	}
	return nil
}

// Get returns a snapshot of peer h, or nil if h is 0 / unknown.
// The returned pointer is a fresh allocation; mutating it doesn't
// affect the manager's state. AppPeer / Workspace / Shell pointers
// inside the struct point to the live underlying objects — those
// remain valid until Destroy(h) is called.
func (m *PeerManager) Get(h int64) *HostedPeer {
	if h == 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	hp, ok := m.peers[h]
	if !ok {
		return nil
	}
	out := *hp
	out.IsSystem = hp.Handle == m.systemHandle
	return &out
}

// List returns a snapshot of every live peer. Order is map-iteration
// (unstable); callers sort if they want a presentation order.
func (m *PeerManager) List() []*HostedPeer {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*HostedPeer, 0, len(m.peers))
	for _, hp := range m.peers {
		copy := *hp
		copy.IsSystem = hp.Handle == m.systemHandle
		out = append(out, &copy)
	}
	return out
}

// SystemPeer returns the current system peer (snapshot) or nil.
func (m *PeerManager) SystemPeer() *HostedPeer {
	return m.Get(m.SystemHandle())
}

// SystemHandle returns the current system-peer handle (0 if empty).
func (m *PeerManager) SystemHandle() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.systemHandle
}

// ShutdownAll destroys every peer. Call before process exit.
func (m *PeerManager) ShutdownAll() {
	m.mu.Lock()
	handles := make([]int64, 0, len(m.peers))
	for id := range m.peers {
		handles = append(handles, id)
	}
	m.mu.Unlock()
	for _, h := range handles {
		_ = m.Destroy(h)
	}
}

// OnPeerDestroyed appends a cleanup hook fired during Destroy after
// the peer is unregistered + system-handover happens but BEFORE its
// AppPeer is closed. Hooks fire serially in registration order. The
// manager's mutex is NOT held during hook execution.
//
// Renderer-specific resource registries (cgo tree+watch maps in the
// bridge; tview model wrappers in console) register here so a
// PeerDestroy call cleanly tears down everything attached to the peer.
func (m *PeerManager) OnPeerDestroyed(cb func(handle int64)) {
	if cb == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onDestroyHooks = append(m.onDestroyHooks, cb)
}

// RestoreFromRoster reads the system peer's roster and respawns
// every non-ephemeral peer that isn't already hosted. Filters:
//   - the system peer itself (already up; that's our reader)
//   - ephemeral peers (Identity == "" — keypair can't be reloaded)
//   - already-hosted peer-ids (idempotent retry)
//
// Returns the handles of the newly-spawned peers plus any error
// encountered during enumeration or per-entry spawn. A per-entry
// failure does NOT abort the rest — successful spawns appear in
// handles; the err carries the first failure with a count.
func (m *PeerManager) RestoreFromRoster() ([]int64, error) {
	sp := m.SystemPeer()
	if sp == nil {
		return nil, fmt.Errorf("RestoreFromRoster: no system peer (call Create first)")
	}
	entries, err := ListRosterEntries(sp.AppPeer, m.appID)
	if err != nil {
		return nil, fmt.Errorf("RestoreFromRoster: %w", err)
	}
	systemPeerID := sp.AppPeer.PeerID()
	alreadyHosted := m.liveHostedPeerIDs()

	handles := make([]int64, 0, len(entries))
	failures := 0
	var firstErr error
	for _, e := range entries {
		if e.PeerID == systemPeerID {
			continue
		}
		if e.Identity == "" {
			continue
		}
		if _, dup := alreadyHosted[e.PeerID]; dup {
			continue
		}
		cfg := Config{
			Identity:    e.Identity,
			LocalAlias:  e.Label,
			StorageKind: e.StorageKind,
			StoragePath: e.StoragePath,
			ListenAddr:  e.ListenAddr,
			// OpenAccess isn't persisted — defaults to false on
			// restore. Re-enable through a future config UI if needed.
		}
		h, cerr := m.Create(cfg)
		if cerr != nil && h == 0 {
			failures++
			if firstErr == nil {
				firstErr = cerr
			}
			continue
		}
		handles = append(handles, h)
	}
	if failures > 0 {
		return handles, fmt.Errorf("RestoreFromRoster: %d of %d entries failed (first: %w)",
			failures, len(entries), firstErr)
	}
	return handles, nil
}

// parseListenAddr classifies a Config.ListenAddr into a transport
// scheme + a bind address suitable for the matching listener call.
// Three accepted shapes:
//
//   - "host:port"            → ("tcp", "host:port", "")
//   - "tcp://host:port"      → ("tcp", "host:port", "")
//   - "ws://host:port[/path]" → ("ws", "host:port", "/path"; default "/ws")
//   - "wss://host:port[/path]" → ("ws", "host:port", "/path"; default "/ws")
//
// wss:// downgrades to ws-with-no-TLS for now — production wss is
// expected to terminate TLS at a reverse proxy per
// peer.ListenWebSocketReady's doc.
func parseListenAddr(raw string) (scheme, bindAddr, wsPath string, err error) {
	switch {
	case strings.HasPrefix(raw, "ws://"), strings.HasPrefix(raw, "wss://"):
		u, perr := url.Parse(raw)
		if perr != nil {
			return "", "", "", perr
		}
		if u.Host == "" {
			return "", "", "", fmt.Errorf("missing host:port in %q", raw)
		}
		path := u.Path
		if path == "" {
			path = "/ws"
		}
		return "ws", u.Host, path, nil
	case strings.HasPrefix(raw, "tcp://"):
		return "tcp", strings.TrimPrefix(raw, "tcp://"), "", nil
	default:
		return "tcp", raw, "", nil
	}
}

// liveHostedPeerIDs returns the set of peer-ids currently in the
// in-memory registry. Internal — RestoreFromRoster uses it for dedup.
func (m *PeerManager) liveHostedPeerIDs() map[string]struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]struct{}, len(m.peers))
	for _, hp := range m.peers {
		out[hp.AppPeer.PeerID()] = struct{}{}
	}
	return out
}

// advertiseListener moved to listener.go, beside the bind it belongs to.
// It grew a LAN-address derivation for a wildcard bind at the same time;
// the reasoning is in that file.
