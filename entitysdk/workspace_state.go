package entitysdk

import (
	"fmt"
	"log"
	"strconv"
	"strings"
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

// WindowContentTypeField is the field name the generic per-window
// fallback carries, per GUIDE-ENTITY-WORKBENCH-APP §4.2's slot table:
// `app/state/window` **MUST carry a `content_type` field** naming what the
// window is showing.
//
// It is spelled with an underscore, like every other field in this schema.
// We wrote `content-type` with a hyphen from the day this was added until
// 2026-09-01, which satisfies the MUST in spirit and fails it in fact —
// a reader looking for the field the spec names finds nothing.
//
// Note this is *not* the `content_type` §5.4 retires from
// `app/state/selection`. That one was source attribution and is gone; this
// one names the window's own content type and is REQUIRED, because under
// the fallback every window's state entity carries the identical entity
// type, so the payload is the only thing that can say what it is.
const WindowContentTypeField = "content_type"

// legacyWindowContentTypeField is the misspelling above, retained for one
// purpose: recognising it on read so a bundle written by an older build is
// migrated rather than left carrying both spellings forever.
const legacyWindowContentTypeField = "content-type"

// SaveWindowContent records what content type a window is showing.
//
// Writes the spec's field name and drops the legacy misspelling from the
// bundle in the same put, so a tree written by an older build converges on
// first save rather than accumulating two fields that disagree.
func (ws *WorkspaceState) SaveWindowContent(windowID uint32, contentType string) {
	state := ws.readWindowState(windowID)
	delete(state, legacyWindowContentTypeField)
	state[WindowContentTypeField] = contentType
	ws.store.Put(ws.windowsStatePath(windowID), "app/state/window", state)
}

// ReadWindowContent returns the content type recorded for a window, or ""
// if none is recorded.
//
// Reads the spec spelling and falls back to the legacy one, because a
// bundle persisted by an older build still carries the hyphen until
// something saves that window again. The fallback is a read-side courtesy
// to our own old data and nothing more — we never write it.
func (ws *WorkspaceState) ReadWindowContent(windowID uint32) string {
	state := ws.readWindowState(windowID)
	if v, ok := state[WindowContentTypeField].(string); ok && v != "" {
		return v
	}
	v, _ := state[legacyWindowContentTypeField].(string)
	return v
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
// tolerance is NON-CONFORMANT).
//
// **`paths` is NOT one of them, and adding it was a live defect** (found by
// entity-browser-rust, 2026-09-01). §5.4's schema block declares
// `paths [text]?` a *current optional* field — "the wider selection set when
// the user has shift-clicked / ctrl-clicked ... multi-select-aware renderers
// populate" — and rule 3's read-side MUST names only `source_window`,
// `source_panel` and `content_type`. Carrying `paths` here meant a perfectly
// conformant multi-select emitter was told it "is NON-CONFORMANT", in the one
// channel the ecosystem has for finding emitters that actually are. A false
// accusation on a conformance channel is worse than silence: it is acted on.
//
// The list is exactly rule 3's three. Extending it is a spec change, not a
// local judgement call — an implementation does not get to retire a field the
// schema still declares.
var legacySelectionFields = []string{"content_type", "source_window", "source_panel"}

// readSelectionAt reads + decodes a Selection at any tree path. Shared
// between ReadSelection (screen aggregate), ReadPanelSelection
// (per-panel slot), and OnSelectionChange's event handler.
//
// Pre-publication legacy-field handling per GUIDE-ENTITY-WORKBENCH-APP §5.4
// (absorption): the retired fields (content_type, source_window,
// source_panel) trigger a WARN-level violation log naming the path
// + offending field(s). `paths` is a live optional field and is NOT one of
// them — see [legacySelectionFields]. The entity is still decoded (MAY-reject
// permitted by spec; we choose log-only at this layer to keep readers
// permissive for in-flight migration probing). Post-publication, behavior
// shifts to spec-V7 §2.6 skip-unknown-fields per a published cutover date.
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

// --- The §8 persist-arm obligation ---

// HighestPersistedWindowID sweeps `app/{app-id}/workspace/windows/` and
// returns the largest window id that has a persisted state entity, or 0 if
// none has. A renderer that persists per-window state seeds its window-id
// counter ABOVE this value, before allocating any window.
//
// This is `GUIDE-ENTITY-WORKBENCH-APP` §8's second arm — *"sweeping
// `app/{app-id}/workspace/windows/` at startup, before allocating any window
// id"* — and taking one of the two arms is a **MUST** for any application that
// persists per-window state. The first arm is an `app/state/window-index`
// (§4.2a), which is the right answer for an application that wants to *restore*
// a window's state; the sweep is the right answer for one that only wants to
// avoid handing the next session's window a stranger's state.
//
// **Why the obligation exists, in the words of the failure it prevents:**
// `{window_id}` is a session-scoped slot address (§3), so an in-memory counter
// starting at zero makes the next launch's first window `1` — and it then reads
// back whatever the *previous* session's first window wrote. That is not a lost
// session, it is a wrong one, and §8 names it as the third case the rule exists
// to prevent: *"persisting, restoring, and silently restoring the wrong
// thing."* Found by `entity-browser-rust` reading our source (their `W-3`,
// 2026-09-01); `console.workspace.nextID` was exactly that counter, and
// `workbench.LogModel` reads a window setting back by ordinal.
//
// **`Store.List` returns PEER-QUALIFIED paths** (AP58), so the id is parsed by
// locating the `workspace/windows/` segment rather than by trimming the prefix
// we passed in — a `TrimPrefix` with a relative prefix removes nothing and the
// arithmetic after it is then confidently wrong. `entitysdk` cannot import
// `workbench`, so `TreeRelative` is not available here; segment-scanning is the
// equivalent and is idempotent on either path form.
//
// A malformed or non-numeric id segment is skipped rather than erroring: this
// is a floor for allocation, and refusing to start because one stray binding
// sits under the prefix would be worse than allocating above the ids we could
// read.
func (ws *WorkspaceState) HighestPersistedWindowID() uint32 {
	const marker = "workspace/windows/"
	var highest uint32
	for _, e := range ws.store.List(ws.windowsPrefix()) {
		i := strings.Index(e.Path, marker)
		if i < 0 {
			continue
		}
		rest := e.Path[i+len(marker):]
		seg := rest
		if j := strings.IndexByte(rest, '/'); j >= 0 {
			seg = rest[:j]
		}
		n, err := strconv.ParseUint(seg, 10, 32)
		if err != nil {
			continue
		}
		if uint32(n) > highest {
			highest = uint32(n)
		}
	}
	return highest
}

// --- Path helpers ---

// windowsPrefix is the sweep prefix for [HighestPersistedWindowID]. It is the
// directory the §8 obligation names, with no trailing id.
func (ws *WorkspaceState) windowsPrefix() string {
	return fmt.Sprintf("app/%s/workspace/windows/", ws.appID)
}

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
