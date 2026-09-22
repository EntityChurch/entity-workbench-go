package workbench

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The workspace layout an operator arranged, so it is still there next
// launch.
//
// # Why this is here and not in the renderer
//
// A layout is a list of panel names, and those names mean something only
// to the renderer that registers them. The temptation is therefore to
// keep the whole thing in C#. Two reasons not to.
//
// First, the rule: workbench is the brain and renderers are thin I/O. The
// *policy* in this file — where the file lives, what precedence applies,
// what happens when it does not parse, what a layout is allowed to
// contain — is not renderer-specific at all. Only the panel names are,
// and to this model they are opaque strings it never interprets.
//
// Second, the reason that matters more: this way it is covered by a Go
// suite that runs in seconds. A config format tested only through a GUI
// is a config format tested when someone remembers.
//
// # Why a file and not the tree
//
// The obvious entity-shaped answer is to persist under `app/state/...`,
// and it is the wrong one here for a blunt reason: **with no flags the
// GUI is an ephemeral in-memory peer and loses everything on exit.** A
// layout stored in that peer's tree would be gone at precisely the moment
// it is supposed to help, which is the default configuration. There is
// also a live design fork about per-window state identity that this must
// not quietly settle. A file under ~/.entity survives both.
//
// # Precedence
//
//  1. WB_LAYOUT (env, highest — scripts and tests; a path to a layout file)
//  2. ~/.entity/gui-layout.json                  (the operator's arrangement)
//  3. DefaultPanelLayout below
//
// # On a file that does not parse
//
// It is an ERROR, never a silent reset (AP33). A layout is the operator's
// arrangement of their own workspace; discarding it because of a stray
// comma and silently substituting the defaults would look exactly like
// "the app forgot again", which is the complaint this file exists to
// answer. The caller is expected to surface the error and keep running on
// the defaults — losing the arrangement is survivable, not being told why
// is not.

// DefaultPanelLayout is the arrangement a peer with no saved layout
// opens with. It is a starting point, not a recommendation: content,
// a place for selection to land, and a prompt.
func DefaultPanelLayout() []string {
	return []string{"site-view", "detail", "shell"}
}

// LayoutKeyDefault is the layout applied to a peer that has no entry of
// its own. Stored under this key in the file, so an operator can edit
// "what every new peer opens with" by hand.
const LayoutKeyDefault = "*"

// MaxLayoutPanels bounds a single layout. A layout is read from a file
// an operator can edit, and a renderer that mounts every entry in it
// will happily mount ten thousand panels. The cap is generous relative
// to any real arrangement and small enough that a malformed file cannot
// wedge the app before it draws a frame.
const MaxLayoutPanels = 64

// PeerLayout is one peer's arrangement.
type PeerLayout struct {
	// Panels is the panel-kind names in display order, top to bottom.
	// Opaque to this model — it never interprets them, which is what
	// keeps a renderer free to add a panel kind without touching Go.
	Panels []string `json:"panels"`

	// NavWidth is the width in pixels of the left-hand navigator column,
	// or 0 for "the renderer's default". Persisted because dragging that
	// splitter back every launch is the same complaint as re-adding a
	// panel every launch.
	NavWidth float64 `json:"nav_width,omitempty"`
}

// LayoutConfig is the whole file: one layout per peer key, plus the
// default under LayoutKeyDefault.
type LayoutConfig struct {
	// Peers maps a peer key (its alias) to that peer's layout.
	//
	// Keyed by ALIAS rather than peer-id on purpose. A peer-id is the
	// better identifier in almost every other context in this codebase,
	// and here it is the wrong one: an ephemeral peer gets a fresh
	// keypair every launch, so a peer-id-keyed layout would never match
	// itself twice and the feature would silently do nothing in the
	// default configuration. The alias is what the operator typed and
	// what the tab shows.
	Peers map[string]PeerLayout `json:"peers"`

	// Source names where this came from, for display. Never read from
	// the file.
	Source string `json:"-"`
}

// LayoutConfigPath is the operator's layout file.
func LayoutConfigPath() (string, error) {
	if p := strings.TrimSpace(os.Getenv("WB_LAYOUT")); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".entity", "gui-layout.json"), nil
}

// LoadLayoutConfig reads the layout file.
//
// A missing file is not an error — absence is the normal first launch and
// means "use the defaults". A file that EXISTS and does not parse is an
// error, and the returned config is still usable (defaults), so a caller
// can report the problem and carry on rather than choosing between the
// two.
func LoadLayoutConfig() (LayoutConfig, error) {
	cfg := LayoutConfig{Peers: map[string]PeerLayout{}, Source: "built-in default"}

	path, err := LayoutConfigPath()
	if err != nil {
		return cfg, nil
	}
	b, rerr := os.ReadFile(path)
	if rerr != nil {
		// Missing is normal. Anything else — a directory, a permission
		// error — is reported, because "we could not read your layout"
		// and "you have no layout" are different facts.
		if os.IsNotExist(rerr) {
			return cfg, nil
		}
		return cfg, errors.New(path + ": " + rerr.Error())
	}

	var fileCfg LayoutConfig
	if jerr := json.Unmarshal(b, &fileCfg); jerr != nil {
		return cfg, errors.New(path + ": " + jerr.Error() +
			" — the file exists, so your layout is not being ignored by accident; fix it or remove it")
	}
	if fileCfg.Peers != nil {
		cfg.Peers = fileCfg.Peers
	}
	cfg.Source = path
	return cfg, nil
}

// For resolves the layout for a peer alias: the peer's own entry, else
// the file's default entry, else the built-in default.
//
// The returned Panels slice is always non-empty and always a copy. Empty
// is not a layout: a saved arrangement with no panels would open the app
// on a blank column with a "+ Add panel" button and no indication that
// anything had been restored, which reads as a broken launch rather than
// as an empty workspace. An operator who genuinely wants no panels can
// close them, and that state is not worth persisting into a dead start.
func (c LayoutConfig) For(alias string) PeerLayout {
	if l, ok := c.Peers[alias]; ok && len(l.Panels) > 0 {
		return PeerLayout{Panels: sanitizeLayoutPanels(l.Panels), NavWidth: l.NavWidth}
	}
	if l, ok := c.Peers[LayoutKeyDefault]; ok && len(l.Panels) > 0 {
		return PeerLayout{Panels: sanitizeLayoutPanels(l.Panels), NavWidth: l.NavWidth}
	}
	return PeerLayout{Panels: DefaultPanelLayout()}
}

// sanitizeLayoutPanels drops blanks and applies MaxLayoutPanels. It does
// NOT validate that a name is a registered panel kind — this model has no
// panel registry and inventing one here would put the renderer's catalog
// in two places. A renderer skips a name it does not know, which is also
// what lets a layout survive a panel being renamed or an older build
// reading a newer file.
func sanitizeLayoutPanels(in []string) []string {
	out := make([]string, 0, len(in))
	for _, p := range in {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
		if len(out) >= MaxLayoutPanels {
			break
		}
	}
	if len(out) == 0 {
		return DefaultPanelLayout()
	}
	return out
}

// Remember records a peer's layout in the config, in memory. Call
// SaveLayoutConfig to persist.
//
// An empty panel list REMOVES the peer's entry rather than storing an
// empty one, so the peer falls back to the default on next launch. See
// For for why an empty layout is not a layout.
func (c *LayoutConfig) Remember(alias string, layout PeerLayout) {
	if c.Peers == nil {
		c.Peers = map[string]PeerLayout{}
	}
	if alias == "" {
		return
	}
	panels := sanitizeLayoutPanelsAllowEmpty(layout.Panels)
	if len(panels) == 0 {
		delete(c.Peers, alias)
		return
	}
	c.Peers[alias] = PeerLayout{Panels: panels, NavWidth: layout.NavWidth}
}

func sanitizeLayoutPanelsAllowEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, p := range in {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
			if len(out) >= MaxLayoutPanels {
				break
			}
		}
	}
	return out
}

// SaveLayoutConfig writes the config, creating ~/.entity if needed.
//
// The write is atomic — temp file in the destination directory, then
// rename. A layout file is written on every panel add, close and swap,
// so it is written far more often than it is read, and a crash or a
// full disk partway through a direct write leaves a truncated file that
// LoadLayoutConfig then correctly refuses. Refusing to load the file
// this app corrupted itself would be a poor way to keep a promise about
// remembering the operator's workspace.
func SaveLayoutConfig(cfg LayoutConfig) error {
	path, err := LayoutConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	// Marshal with sorted keys for a stable file: an operator may read
	// or diff this, and a map iteration order that changes every write
	// makes it look like something changed when nothing did.
	out := struct {
		Peers map[string]PeerLayout `json:"peers"`
	}{Peers: map[string]PeerLayout{}}
	keys := make([]string, 0, len(cfg.Peers))
	for k := range cfg.Peers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out.Peers[k] = cfg.Peers[k]
	}

	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), ".gui-layout.*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}
