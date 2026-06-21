package entitysdk

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// DefaultAppID is the {app-id} used by NewWorkspaceState when the
// caller doesn't specify one. The workbench convention.
const DefaultAppID = "workbench"

// WorkspaceState provides typed access to entity-backed application
// state. All state lives in the entity tree (not in Go structs), so
// multiple renderers can share it.
//
// WorkspaceState operates at Level 0 (direct store) — the application
// is writing its own state under its own app-id, so capability
// dispatch is unnecessary overhead. The Store accessor is visible at
// construction time to make the level explicit.
//
// Path conventions follow GUIDE-ENTITY-WORKBENCH-APP.md and
// GUIDE-PEER-CONCERNS-AND-NAMESPACES §5:
//
//	app/{app-id}/workspace/windows/{id}/state          bundled per-window state
//	app/{app-id}/workspace/screens/active              active screen index
//	app/{app-id}/workspace/screens/{idx}/selection     per-screen selection
//	app/{app-id}/settings/{key}                        global setting
//
// Per-window state is bundled into a single CBOR map entity at the
// windows/{id}/state path (type app/state/window). Renderers that
// want fine-grained subscriptions can still observe just that one
// path. The bundled shape matches the cross-impl convention and
// avoids a per-key entity explosion as the workbench grows.
//
// Selection is per-presentation-context (per-screen): each screen
// owns its own selection so multi-screen workspaces have independent
// navigation history. SaveSelection/ReadSelection are scoped by
// screen index.
//
// The {app-id} scoping lets multiple applications coexist on one peer
// without colliding. Type names (app/state/window, app/state/selection,
// app/state/setting) are language-neutral.
type WorkspaceState struct {
	store *Store
	appID string
}

// NewWorkspaceState creates a workspace state accessor scoped under
// app/{DefaultAppID}/ (app/workbench/ by default).
func NewWorkspaceState(store *Store) *WorkspaceState {
	return NewWorkspaceStateFor(store, DefaultAppID)
}

// NewWorkspaceStateFor creates a workspace state accessor scoped
// under app/{appID}/. Use this when embedding the SDK in an
// application with a different app ID.
func NewWorkspaceStateFor(store *Store, appID string) *WorkspaceState {
	if appID == "" {
		appID = DefaultAppID
	}
	return &WorkspaceState{store: store, appID: appID}
}

// AppID returns the {app-id} used for path scoping.
func (ws *WorkspaceState) AppID() string { return ws.appID }

// --- Per-window bundled state ---

// SaveWindowContent records what content type a window is showing.
func (ws *WorkspaceState) SaveWindowContent(windowID uint32, contentType string) {
	ws.updateWindowState(windowID, "content-type", contentType)
}

// SaveWindowScreen records which screen a window belongs to.
func (ws *WorkspaceState) SaveWindowScreen(windowID uint32, screenIdx int) {
	ws.updateWindowState(windowID, "screen", fmt.Sprintf("%d", screenIdx))
}

// SaveWindowSetting writes a per-window setting.
func (ws *WorkspaceState) SaveWindowSetting(windowID uint32, key, value string) {
	ws.updateWindowState(windowID, key, value)
}

// ReadWindowSetting reads a per-window setting. Returns "" if not found.
func (ws *WorkspaceState) ReadWindowSetting(windowID uint32, key string) string {
	state := ws.readWindowState(windowID)
	v, _ := state[key].(string)
	return v
}

// WindowStatePath returns the absolute tree path of a window's
// bundled state entity. Useful for tests and for renderers that want
// to subscribe to a specific window's state.
func (ws *WorkspaceState) WindowStatePath(windowID uint32) string {
	return ws.windowsStatePath(windowID)
}

// --- Screen state ---

// SaveActiveScreen records which screen is active.
func (ws *WorkspaceState) SaveActiveScreen(screenIdx int) {
	ws.store.Put(ws.screensActivePath(), "app/state/setting", map[string]interface{}{
		"key":   "active-screen",
		"value": uint64(screenIdx),
	})
}

// ReadActiveScreen reads the active screen index. Returns 0 if not found.
func (ws *WorkspaceState) ReadActiveScreen() int {
	r, ok := ws.resolve(ws.screensActivePath())
	if !ok {
		return 0
	}
	m, ok := r.Decoded.(map[interface{}]interface{})
	if !ok {
		return 0
	}
	switch v := m["value"].(type) {
	case uint64:
		return int(v)
	case int64:
		return int(v)
	}
	return 0
}

// --- Per-screen selection ---

// Selection is the per-presentation-context selection payload. Slim
// schema following the per-panel-slot + per-context-aggregate model in
// SHELL-DIRECTION.md §8.4. Diverges from GUIDE-ENTITY-WORKBENCH-APP.md
// §5.4 (`content_type`, `source_window`, `paths` dropped — vestigial under
// per-panel-slot wiring; ReadSelection still tolerates legacy records
// that carry them).
//
//   - Path: focused single path the user is attending to. Empty when no
//     selection.
//   - Type: type of the selected *thing* — today always "entity";
//     forward-compat for query-result rows, event-log rows, etc. The
//     slot path identifies the source panel; this names the kind of
//     pointee.
//   - PeerID: which peer's tree the Path refers to. Empty means the
//     host peer.
//   - UpdatedAt: epoch milliseconds; staleness signal for last-writer
//     tie-breaking on aggregate slots. SaveSelection auto-fills from
//     time.Now if zero.
type Selection struct {
	Path      string
	Type      string
	PeerID    string
	UpdatedAt uint64
}

// SaveSelection records the selection for a specific screen aggregate
// slot. Auto-fills UpdatedAt from time.Now if sel.UpdatedAt == 0.
// Optional fields are written only when non-empty, matching the guide's
// "absence = unset" convention.
func (ws *WorkspaceState) SaveSelection(screenIdx int, sel Selection) {
	ws.savePanelSelectionAt(ws.screenSelectionPath(screenIdx), sel)
}

// SavePanelSelection records a selection in a panel's own slot at
// app/{app-id}/workspace/panels/{panelID}/selection. Per the per-panel-
// slot model (SHELL-DIRECTION.md §8.4): publisher panels typically
// write both their own slot and the screen aggregate; consumer panels
// default to watching the aggregate but can opt into a specific panel
// slot instead.
func (ws *WorkspaceState) SavePanelSelection(panelID uint32, sel Selection) {
	ws.savePanelSelectionAt(ws.panelSelectionPath(panelID), sel)
}

// ReadPanelSelection reads the selection from a specific panel's own
// slot. Returns (Selection{}, false) if no record is present.
func (ws *WorkspaceState) ReadPanelSelection(panelID uint32) (Selection, bool) {
	return ws.readSelectionAt(ws.panelSelectionPath(panelID))
}

// PanelSelectionPath returns the absolute tree path of a panel's own
// selection slot. Useful for tests, debug surfaces, and for callers
// that want to subscribe via OnSelectionChange.
func (ws *WorkspaceState) PanelSelectionPath(panelID uint32) string {
	return ws.panelSelectionPath(panelID)
}

// ScreenSelectionPath returns the absolute tree path of a screen's
// aggregate selection slot. Useful for OnSelectionChange subscribers
// that watch the per-context aggregate.
func (ws *WorkspaceState) ScreenSelectionPath(screenIdx int) string {
	return ws.screenSelectionPath(screenIdx)
}

// savePanelSelectionAt is the shared encode-and-write used by
// SaveSelection (screen aggregate) and SavePanelSelection (per-panel
// slot in step 3).
func (ws *WorkspaceState) savePanelSelectionAt(path string, sel Selection) {
	if sel.UpdatedAt == 0 {
		sel.UpdatedAt = uint64(time.Now().UnixMilli())
	}
	payload := map[string]interface{}{
		"path":       sel.Path,
		"updated_at": sel.UpdatedAt,
	}
	if sel.Type != "" {
		payload["type"] = sel.Type
	}
	if sel.PeerID != "" {
		payload["peer_id"] = sel.PeerID
	}
	ws.store.Put(path, "app/state/selection", payload)
}

// ReadSelection reads the selection for a specific screen. Returns
// (Selection{}, false) if no record is present. Tolerates records
// written under the prior {path, has_entry} schema — the Path field is
// still populated.
func (ws *WorkspaceState) ReadSelection(screenIdx int) (Selection, bool) {
	return ws.readSelectionAt(ws.screenSelectionPath(screenIdx))
}

// legacySelectionFields enumerates the retired Selection fields per
// GUIDE-ENTITY-WORKBENCH-APP §5.4 post-absorption. Reading an
// entity that carries any of these emits a violation log per arch's
// landed Amendment A (Option 2: MUST log violation pre-publication; silent
// tolerance is NON-CONFORMANT). See feedback_no_legacy_pre_release in
// workbench-go auto-memory for the stance.
var legacySelectionFields = []string{"content_type", "source_window", "source_panel", "paths"}

// readSelectionAt reads + decodes a Selection at any tree path. Shared
// between ReadSelection (screen aggregate), ReadPanelSelection
// (per-panel slot), and OnSelectionChange's event handler.
//
// Pre-publication legacy-field handling per GUIDE-ENTITY-WORKBENCH-APP §5.4
// (absorption): the retired fields (content_type, source_window,
// source_panel, paths) trigger a WARN-level violation log naming the path
// + offending field(s). The entity is still decoded (MAY-reject permitted
// by spec; we choose log-only at this layer to keep readers permissive for
// in-flight migration probing). Post-publication, behavior shifts to spec-
// V7 §2.6 skip-unknown-fields per a published cutover date.
func (ws *WorkspaceState) readSelectionAt(path string) (Selection, bool) {
	r, ok := ws.resolve(path)
	if !ok {
		return Selection{}, false
	}
	m, ok := r.Decoded.(map[interface{}]interface{})
	if !ok {
		return Selection{}, false
	}
	logLegacyFieldViolations(path, m)
	sel := Selection{}
	sel.Path, _ = m["path"].(string)
	sel.Type, _ = m["type"].(string)
	sel.PeerID, _ = m["peer_id"].(string)
	switch v := m["updated_at"].(type) {
	case uint64:
		sel.UpdatedAt = v
	case int64:
		sel.UpdatedAt = uint64(v)
	}
	return sel, true
}

// logLegacyFieldViolations emits a single WARN line per legacy field
// detected. Per arch's landed Amendment A: MUST log violation pre-
// publication; silent tolerance is NON-CONFORMANT. Each violation cites
// the entity path so the offending emitter can be tracked down.
func logLegacyFieldViolations(path string, m map[interface{}]interface{}) {
	for _, field := range legacySelectionFields {
		if _, present := m[field]; present {
			log.Printf("entitysdk: WARN: legacy field %q present in app/state/selection at %q — emitter is NON-CONFORMANT per GUIDE-ENTITY-WORKBENCH-APP §5.4", field, path)
		}
	}
}

// OnPrefixChange is a thin re-export of Store.OnPrefixChange for
// callers that already have a WorkspaceState handle. The implementation
// lives on Store — this method exists so callers can subscribe by
// prefix without separately threading the underlying Store.
//
// See Store.OnPrefixChange for the full contract.
func (ws *WorkspaceState) OnPrefixChange(prefix string, handler func(ChangeEvent)) (cancel func()) {
	return ws.store.OnPrefixChange(prefix, handler)
}

// OnSelectionChange subscribes to selection-state changes at the given
// tree path. The handler fires on each ChangePut, decoded as a
// Selection. ChangeRemove events deliver a zero Selection so handlers
// can react to clears.
//
// The returned cancel function stops the subscription: it closes the
// underlying Store.Watch and the SDK-internal goroutine. Safe to call
// more than once; safe to call after the peer is closed.
//
// Threading: the handler runs on an SDK-owned goroutine. Callers that
// touch panel-render state from the handler must marshal back to their
// render thread themselves — sync.Mutex for raylib/canvas, tview's
// app.QueueUpdateDraw for tview/console, etc.
//
// Panics if path is empty or the Store has no watch hub (peer
// misconfiguration — peer must be constructed via CreatePeer or
// NewAppPeer).
func (ws *WorkspaceState) OnSelectionChange(path string, handler func(Selection)) (cancel func()) {
	if path == "" {
		panic("entitysdk: OnSelectionChange requires a non-empty path")
	}
	w, err := ws.store.Watch(path)
	if err != nil {
		panic(fmt.Sprintf("entitysdk: OnSelectionChange watch failed: %v", err))
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ev := range w.Events() {
			switch ev.EventType {
			case ChangePut:
				if sel, ok := ws.readSelectionAt(path); ok {
					handler(sel)
				}
			case ChangeRemove:
				handler(Selection{})
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			w.Close()
			<-done
		})
	}
}

// --- Shell connection aliases ---

// ShellAlias is the persisted binding of a human-friendly alias name
// to a remote peer's id + transport address. Aliases live in
// app/{app-id}/workspace/shells/aliases/{alias}; the transport address
// is duplicated here so the alias can be re-bound on startup without
// also reading system/peer/transport (which is keyed by peer-id, not
// alias name).
type ShellAlias struct {
	Alias   string
	PeerID  string
	Address string
}

// SaveAlias persists a shell connection alias under the workspace.
// Today the slot is write-only; future "restore last session" or
// "list known aliases" features read from it.
func (ws *WorkspaceState) SaveAlias(alias, peerID, address string) {
	ws.store.Put(ws.shellAliasPath(alias), "app/state/shell-alias", map[string]interface{}{
		"alias":   alias,
		"peer_id": peerID,
		"address": address,
	})
}

// RemoveAlias deletes a persisted alias. Mirrors removeConn.
func (ws *WorkspaceState) RemoveAlias(alias string) {
	ws.store.Remove(ws.shellAliasPath(alias))
}

// ReadAlias reads a persisted alias, or returns (ShellAlias{}, false)
// if no record is present.
func (ws *WorkspaceState) ReadAlias(alias string) (ShellAlias, bool) {
	r, ok := ws.resolve(ws.shellAliasPath(alias))
	if !ok {
		return ShellAlias{}, false
	}
	m, ok := r.Decoded.(map[interface{}]interface{})
	if !ok {
		return ShellAlias{}, false
	}
	out := ShellAlias{}
	out.Alias, _ = m["alias"].(string)
	out.PeerID, _ = m["peer_id"].(string)
	out.Address, _ = m["address"].(string)
	return out, true
}

// --- Global Settings ---

// SaveSetting writes a global setting.
func (ws *WorkspaceState) SaveSetting(key, value string) {
	ws.putSetting(ws.settingsPath(key), key, value)
}

// ReadSetting reads a global setting. Returns "" if not found.
func (ws *WorkspaceState) ReadSetting(key string) string {
	return ws.readSettingValue(ws.settingsPath(key))
}

// --- Path helpers ---

func (ws *WorkspaceState) windowsStatePath(windowID uint32) string {
	return fmt.Sprintf("app/%s/workspace/windows/%d/state", ws.appID, windowID)
}

func (ws *WorkspaceState) screensActivePath() string {
	return fmt.Sprintf("app/%s/workspace/screens/active", ws.appID)
}

func (ws *WorkspaceState) screenSelectionPath(screenIdx int) string {
	return fmt.Sprintf("app/%s/workspace/screens/%d/selection", ws.appID, screenIdx)
}

func (ws *WorkspaceState) panelSelectionPath(panelID uint32) string {
	return fmt.Sprintf("app/%s/workspace/panels/%d/selection", ws.appID, panelID)
}

func (ws *WorkspaceState) settingsPath(key string) string {
	return fmt.Sprintf("app/%s/settings/%s", ws.appID, key)
}

func (ws *WorkspaceState) shellAliasPath(alias string) string {
	return fmt.Sprintf("app/%s/workspace/shells/aliases/%s", ws.appID, alias)
}

// --- Internal helpers ---

// updateWindowState reads the window's bundled state, sets the given
// key, and writes the updated bundle back. The bundle is a single
// entity of type app/state/window holding all per-window state in a
// CBOR map.
func (ws *WorkspaceState) updateWindowState(windowID uint32, key, value string) {
	state := ws.readWindowState(windowID)
	state[key] = value
	ws.store.Put(ws.windowsStatePath(windowID), "app/state/window", state)
}

func (ws *WorkspaceState) readWindowState(windowID uint32) map[string]interface{} {
	r, ok := ws.resolve(ws.windowsStatePath(windowID))
	if !ok {
		return make(map[string]interface{})
	}
	return mapFromDecoded(r.Decoded)
}

// mapFromDecoded coerces a CBOR-decoded map (which may use
// interface{} or string keys depending on decoder choice) into a
// map[string]interface{}. Non-string keys are dropped.
func mapFromDecoded(decoded interface{}) map[string]interface{} {
	out := make(map[string]interface{})
	switch m := decoded.(type) {
	case map[interface{}]interface{}:
		for k, v := range m {
			if ks, ok := k.(string); ok {
				out[ks] = v
			}
		}
	case map[string]interface{}:
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

func (ws *WorkspaceState) putSetting(path, key, value string) {
	ws.store.Put(path, "app/state/setting", map[string]interface{}{
		"key":   key,
		"value": value,
	})
}

func (ws *WorkspaceState) readSettingValue(path string) string {
	r, ok := ws.resolve(path)
	if !ok {
		return ""
	}
	m, ok := r.Decoded.(map[interface{}]interface{})
	if !ok {
		return ""
	}
	v, ok := m["value"].(string)
	if !ok {
		return ""
	}
	return v
}

func (ws *WorkspaceState) resolve(path string) (ResolvedEntity, bool) {
	ent, ok := ws.store.Get(path)
	if !ok {
		return ResolvedEntity{}, false
	}
	var decoded interface{}
	decodeEntityData(ent.Data, &decoded)
	return ResolvedEntity{
		Path:    path,
		Hash:    ent.ContentHash,
		Entity:  ent,
		Decoded: decoded,
	}, true
}
