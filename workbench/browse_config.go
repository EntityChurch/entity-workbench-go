package workbench

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Where a browser starts.
//
// # Why there is a built-in default at all
//
// A browser with no name authority pinned can do nothing: it cannot
// resolve a name, so the address bar is inert and the name list is empty.
// Until 2026-08-31 that was the state every session began in, and the
// first thing an operator had to do was paste an origin and — because the
// well-known profile features that origin's *site* peer, not its registry
// peer (AP44) — often a peer-id as well, obtained out of band. Requiring a
// correct 46-character key before the first useful keystroke is not a
// security property; it is a blank screen.
//
// So there is a default, and the honesty obligation moves rather than
// disappears: **a default pin is still a pin somebody else chose**, and it
// is reported on every render exactly like an origin-nominated one. The
// difference between "you pinned this" and "we shipped this" is the kind
// of difference this browser exists to keep visible.
//
// # Precedence
//
//  1. WB_REGISTRY_ORIGIN / WB_REGISTRY_PEER    (env, highest — scripts, tests)
//  2. ~/.entity/browser.json                   (the operator's file)
//  3. the built-in default below
//
// Any level may set the origin and leave the peer empty, which means
// *adopt whatever the origin's `entity-deployment.json` nominates* —
// trust-on-first-use, labelled as such downstream.
const (
	// DefaultRegistryOrigin is the cohort's public registry. It is a
	// default, not a blessing: it is here so the browser opens on
	// something rather than nothing.
	DefaultRegistryOrigin = "https://entitychurchregistry.org"

	// DefaultRegistryPeer is deliberately EMPTY.
	//
	// Hard-coding the peer-id would be a stronger default and a worse
	// one: it would bake a key into the binary that no operator agreed
	// to and that cannot be rotated without a release. Empty means we
	// read `{origin}/entity-deployment.json` and adopt what it
	// nominates, which is TOFU — but it is TOFU the operator can see,
	// labelled on every render, and it is honest about who chose the
	// key. Put a peer-id in ~/.entity/browser.json to make it a real pin.
	DefaultRegistryPeer = ""
)

// BrowseConfig is the browser's start-up configuration.
type BrowseConfig struct {
	// RegistryOrigin is the origin serving the registry's bytes.
	RegistryOrigin string `json:"registry_origin"`
	// RegistryPeer is the pin. Empty means adopt the origin's nomination.
	RegistryPeer string `json:"registry_peer"`
	// AutoPin false suppresses the start-up pin entirely, for an
	// operator who wants the browser to begin with no name authority.
	// Pointer so that "absent" and "false" are distinguishable.
	AutoPin *bool `json:"auto_pin,omitempty"`

	// Source names where these values came from, for display. Never
	// read from the file.
	Source string `json:"-"`
}

// ShouldAutoPin reports whether to pin at start-up.
func (c BrowseConfig) ShouldAutoPin() bool {
	if c.AutoPin != nil && !*c.AutoPin {
		return false
	}
	return c.RegistryOrigin != ""
}

// BrowseConfigPath is the operator's override file.
func BrowseConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".entity", "browser.json"), nil
}

// LoadBrowseConfig resolves the start-up configuration.
//
// It never fails on a missing file — absence is the normal case and
// means "use the default". It DOES fail on a file that exists and does
// not parse: a config file that is silently ignored because of a stray
// comma is worse than no config file, because the operator believes a
// setting is in effect (AP33 — refuse input that was plainly trying to be
// structured).
func LoadBrowseConfig() (BrowseConfig, error) {
	cfg := BrowseConfig{
		RegistryOrigin: DefaultRegistryOrigin,
		RegistryPeer:   DefaultRegistryPeer,
		Source:         "built-in default",
	}

	if path, err := BrowseConfigPath(); err == nil {
		b, rerr := os.ReadFile(path)
		switch {
		case rerr == nil:
			var fileCfg BrowseConfig
			if jerr := json.Unmarshal(b, &fileCfg); jerr != nil {
				return cfg, errors.New(path + ": " + jerr.Error() +
					" — the file exists, so it is not being ignored; fix it or remove it")
			}
			if fileCfg.RegistryOrigin != "" {
				cfg.RegistryOrigin = strings.TrimSpace(fileCfg.RegistryOrigin)
			}
			// An explicitly empty peer in the file is meaningful: it
			// says "adopt the origin's nomination", overriding nothing
			// but also not inheriting a peer from the default.
			cfg.RegistryPeer = strings.TrimSpace(fileCfg.RegistryPeer)
			cfg.AutoPin = fileCfg.AutoPin
			cfg.Source = path
		case errors.Is(rerr, fs.ErrNotExist):
			// normal
		default:
			return cfg, rerr
		}
	}

	// Env wins over both — it is what a test or a script sets, and it
	// must not require writing to the operator's home directory.
	if v := strings.TrimSpace(os.Getenv("WB_REGISTRY_ORIGIN")); v != "" {
		cfg.RegistryOrigin = v
		cfg.RegistryPeer = strings.TrimSpace(os.Getenv("WB_REGISTRY_PEER"))
		cfg.Source = "WB_REGISTRY_ORIGIN"
	} else if v := strings.TrimSpace(os.Getenv("WB_REGISTRY_PEER")); v != "" {
		cfg.RegistryPeer = v
		cfg.Source += " + WB_REGISTRY_PEER"
	}

	if strings.TrimSpace(os.Getenv("WB_NO_AUTOPIN")) != "" {
		no := false
		cfg.AutoPin = &no
	}
	return cfg, nil
}

// SaveBrowseConfig writes the operator's pin to ~/.entity/browser.json.
//
// # Why the pin belongs in a FILE and not in the tree
//
// The rule this repo works to is that a file holds what you need in
// order to find the tree, and the tree holds everything else. A registry
// pin looks like tree material and is not: the browser **holds no peer**
// (see the note on `ShellWorkspace.Browser`). A Mode A2 consumer reading
// a static origin never dispatches anything, so there is no tree to read
// the pin out of at the moment the pin is needed, and a peer-id-keyed
// record would key the operator's trust decision to whichever identity
// happened to be loaded. The pin is a pre-peer fact, so it is a file.
//
// # Why this exists at all
//
// Until 2026-09-10 nothing in this tree wrote this file — it was read by
// the GUI at start-up, read by nothing else, and written by no one. The
// consequence was measured: `registry pin` reported four lines of
// success, and the next one-shot command in the same HOME reported
// *"nothing pinned"*, because the model behind it was process state on
// the workspace. The one fact the whole consume design says you supply
// out of band was the one fact we discarded at exit — and because the
// shell had no writer and the GUI had no other reader, the two surfaces
// could not share a trust decision either.
//
// Written 0600: it is not a secret, but it is a security-relevant
// declaration in the operator's home directory and the file mode should
// say who it belongs to.
func SaveBrowseConfig(cfg BrowseConfig) (string, error) {
	path, err := BrowseConfigPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	// Source is display-only and must never round-trip into the file;
	// it is `json:"-"`, so this is a statement of intent rather than a
	// filter, and the test asserts a written file re-reads as itself.
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	b = append(b, '\n')
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return "", err
	}
	return path, nil
}
