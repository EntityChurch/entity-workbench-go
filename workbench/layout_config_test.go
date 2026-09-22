package workbench

import (
	"os"
	"path/filepath"
	"testing"
)

// layoutAt points WB_LAYOUT at a scratch file so these tests never touch
// the developer's real ~/.entity/gui-layout.json. Note the env var is
// read at CALL time by LayoutConfigPath, not captured at process start —
// which is why this works here and why the equivalent trick does NOT work
// from a C# test against the bridge (Go captures its environment once at
// process start, so a managed SetEnvironmentVariable never reaches
// os.Getenv; that is why BrowserPanel has an in-process flag instead).
func layoutAt(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "gui-layout.json")
	t.Setenv("WB_LAYOUT", p)
	return p
}

func TestLayout_MissingFileIsNotAnError(t *testing.T) {
	layoutAt(t)

	cfg, err := LoadLayoutConfig()
	if err != nil {
		t.Fatalf("a missing layout file is the normal first launch, not an error: %v", err)
	}
	got := cfg.For("me").Panels
	want := DefaultPanelLayout()
	if len(got) != len(want) {
		t.Fatalf("default layout = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("default layout = %v, want %v", got, want)
		}
	}
}

// AP33: a file that exists and does not parse is an ERROR. Silently
// resetting to the defaults is indistinguishable from "the app forgot
// again", which is the complaint this feature exists to answer — and the
// operator would never learn there was a comma out of place.
func TestLayout_MalformedFileIsAnErrorAndStillUsable(t *testing.T) {
	path := layoutAt(t)
	if err := os.WriteFile(path, []byte(`{"peers": {`), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	cfg, err := LoadLayoutConfig()
	if err == nil {
		t.Fatal("a layout file that does not parse must be reported, not silently discarded")
	}
	// The returned config must still be usable, so a caller can show the
	// error AND open the app rather than choosing one.
	if len(cfg.For("me").Panels) == 0 {
		t.Fatal("the config returned alongside the error must still open the app on the defaults")
	}
}

func TestLayout_RoundTripsThroughTheFile(t *testing.T) {
	layoutAt(t)

	cfg, _ := LoadLayoutConfig()
	cfg.Remember("me", PeerLayout{Panels: []string{"browser", "local-files"}, NavWidth: 420})
	if err := SaveLayoutConfig(cfg); err != nil {
		t.Fatalf("save: %v", err)
	}

	reloaded, err := LoadLayoutConfig()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	l := reloaded.For("me")
	if len(l.Panels) != 2 || l.Panels[0] != "browser" || l.Panels[1] != "local-files" {
		t.Fatalf("panels did not survive the round trip: %v", l.Panels)
	}
	if l.NavWidth != 420 {
		t.Fatalf("nav width did not survive the round trip: %v", l.NavWidth)
	}
}

// Order is the layout. A set would restore the same panels in the wrong
// places, which is a different workspace.
func TestLayout_PreservesOrder(t *testing.T) {
	layoutAt(t)
	cfg, _ := LoadLayoutConfig()
	want := []string{"shell", "browser", "detail", "local-files"}
	cfg.Remember("me", PeerLayout{Panels: want})
	if err := SaveLayoutConfig(cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := LoadLayoutConfig()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	for i, p := range got.For("me").Panels {
		if p != want[i] {
			t.Fatalf("layout order changed: %v, want %v", got.For("me").Panels, want)
		}
	}
}

// A peer with no entry of its own falls through to the file's "*" entry
// before the built-in default, so an operator can set what every new peer
// opens with by editing one key.
func TestLayout_UnknownPeerFallsBackToTheFileDefaultThenBuiltIn(t *testing.T) {
	layoutAt(t)
	cfg, _ := LoadLayoutConfig()
	cfg.Remember(LayoutKeyDefault, PeerLayout{Panels: []string{"shell"}})
	cfg.Remember("alice", PeerLayout{Panels: []string{"browser"}})
	if err := SaveLayoutConfig(cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, _ := LoadLayoutConfig()

	if p := got.For("alice").Panels; len(p) != 1 || p[0] != "browser" {
		t.Fatalf("alice should get her own layout, got %v", p)
	}
	if p := got.For("bob").Panels; len(p) != 1 || p[0] != "shell" {
		t.Fatalf("bob should fall back to the file default, got %v", p)
	}

	// With no "*" entry at all, the built-in default applies.
	delete(got.Peers, LayoutKeyDefault)
	if p := got.For("bob").Panels; len(p) != len(DefaultPanelLayout()) {
		t.Fatalf("with no file default, bob should get the built-in, got %v", p)
	}
}

// An empty layout is not a layout. Restoring one would open the app on a
// blank column with no indication anything had been restored, which reads
// as a broken launch. Remembering empty removes the entry so the peer
// falls back instead.
func TestLayout_EmptyIsNotPersistedAsALayout(t *testing.T) {
	layoutAt(t)
	cfg, _ := LoadLayoutConfig()
	cfg.Remember("me", PeerLayout{Panels: []string{"browser"}})
	cfg.Remember("me", PeerLayout{Panels: []string{}})
	if _, ok := cfg.Peers["me"]; ok {
		t.Fatal("an empty layout must remove the entry, not store an empty one")
	}
	if len(cfg.For("me").Panels) == 0 {
		t.Fatal("For must never return an empty layout")
	}
}

// A file an operator can hand-edit is a file that can contain anything.
// The cap is what stops a malformed or hostile entry from wedging the app
// before it draws a frame.
func TestLayout_IsBoundedAndDropsBlanks(t *testing.T) {
	layoutAt(t)
	cfg, _ := LoadLayoutConfig()

	huge := make([]string, MaxLayoutPanels+50)
	for i := range huge {
		huge[i] = "shell"
	}
	cfg.Remember("me", PeerLayout{Panels: huge})
	if n := len(cfg.Peers["me"].Panels); n != MaxLayoutPanels {
		t.Fatalf("layout not bounded: %d panels stored, cap is %d", n, MaxLayoutPanels)
	}

	cfg.Remember("blanks", PeerLayout{Panels: []string{"  ", "browser", "", "\t"}})
	got := cfg.Peers["blanks"].Panels
	if len(got) != 1 || got[0] != "browser" {
		t.Fatalf("blank entries not dropped: %v", got)
	}
}

// The model must NOT validate panel names against a catalog it does not
// have. Keeping the renderer's registry in one place is what lets a
// layout survive an older build reading a newer file — the renderer skips
// what it does not recognise, rather than the model refusing the file.
func TestLayout_DoesNotValidatePanelNames(t *testing.T) {
	layoutAt(t)
	cfg, _ := LoadLayoutConfig()
	cfg.Remember("me", PeerLayout{Panels: []string{"a-panel-from-the-future"}})
	if err := SaveLayoutConfig(cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := LoadLayoutConfig()
	if err != nil {
		t.Fatalf("an unknown panel name must not make the file unreadable: %v", err)
	}
	if p := got.For("me").Panels; len(p) != 1 || p[0] != "a-panel-from-the-future" {
		t.Fatalf("unknown panel name was not carried through: %v", p)
	}
}

// The saved file is written far more often than it is read — every add,
// close and swap — so a partial write is the realistic failure. The write
// is atomic, and the observable consequence is that no reader ever sees a
// truncated file. Asserted here by checking no temp file survives a
// successful save, which is the part a later reader would trip over.
func TestLayout_SaveLeavesNoTempFileBehind(t *testing.T) {
	path := layoutAt(t)
	cfg, _ := LoadLayoutConfig()
	cfg.Remember("me", PeerLayout{Panels: []string{"shell"}})
	for i := 0; i < 5; i++ {
		if err := SaveLayoutConfig(cfg); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != filepath.Base(path) {
			t.Fatalf("save left %s behind; a stray temp file is litter in the operator's ~/.entity", e.Name())
		}
	}
}
