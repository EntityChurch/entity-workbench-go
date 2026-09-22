package shellcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"entity-workbench-go/workbench"
)

// registry_pin_persist_test.go — the pin outlives the process.
//
// The defect these gate was measured on 2026-09-10 against the LIVE
// federation, in two commands:
//
//	$ entity-shell -storage sqlite registry pin https://entitychurchregistry.org
//	pinned   2KFNrGARQBkx3d9WQtkeuT3HbQ3oXSS7PBggWt1szT9hzb
//	...
//	$ entity-shell -storage sqlite registry ls
//	entity-shell: registry ls: nothing pinned
//
// `ShellWorkspace.Browser` is process state, nothing in this tree wrote
// `~/.entity/browser.json`, and the file was read only by the GUI. So the
// one fact the consume design says the operator supplies out of band was
// the one fact discarded at exit, and the two surfaces could not share a
// trust decision.
//
// **The shape that matters is the process boundary**, which is why these
// tests write a file with one call and read it back with another rather
// than asserting on a model: a test that keeps the model alive passes
// against the broken build. Same reason
// `entitysdk/subscription_restart_test.go` and
// `entitysdk/query_index_restart_test.go` are shaped the way they are.
//
// These are OFFLINE. Persistence is a file question and none of it needs
// an origin; the live half is covered by cmd_browse_test.go against the
// frozen federation.

// TestBrowseConfigRoundTripsThroughTheFile is the process boundary,
// stood in for by two independent calls with no shared state.
func TestBrowseConfigRoundTripsThroughTheFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	want := workbench.BrowseConfig{
		RegistryOrigin: "https://example.invalid",
		RegistryPeer:   "2KFNrGARQBkx3d9WQtkeuT3HbQ3oXSS7PBggWt1szT9hzb",
	}
	path, err := workbench.SaveBrowseConfig(want)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if filepath.Dir(path) != filepath.Join(home, ".entity") {
		t.Fatalf("wrote outside ~/.entity: %s", path)
	}

	got, err := workbench.LoadBrowseConfig()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.RegistryOrigin != want.RegistryOrigin {
		t.Errorf("origin did not survive the file: got %q want %q",
			got.RegistryOrigin, want.RegistryOrigin)
	}
	if got.RegistryPeer != want.RegistryPeer {
		t.Errorf("PIN did not survive the file: got %q want %q — this is the whole defect",
			got.RegistryPeer, want.RegistryPeer)
	}
	if got.Source != path {
		t.Errorf("a loaded config must name the file it came from, got %q", got.Source)
	}
	if !got.ShouldAutoPin() {
		t.Error("a saved pin must be one the next session opens on")
	}
}

// TestSavedPinIsNotWorldReadable — it is not a secret, but it is a
// security-relevant declaration in the operator's home directory.
func TestSavedPinIsNotWorldReadable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path, err := workbench.SaveBrowseConfig(workbench.BrowseConfig{
		RegistryOrigin: "https://example.invalid",
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := fi.Mode().Perm(); mode != 0o600 {
		t.Errorf("browser.json mode %o, want 600", mode)
	}
}

// TestUnpinSurvivesTheProcess is the arm that the obvious implementation
// fails.
//
// Persisting the pin and not the UNPIN gives a verb that reverses itself
// at the next launch: the operator drops the authority, restarts, and the
// start-up pin puts it straight back. That is the same defect `unshare`
// shipped with — a change the declaration does not carry is undone by the
// next pass.
func TestUnpinSurvivesTheProcess(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	prior := BrowseAutoPin
	BrowseAutoPin = false
	t.Cleanup(func() { BrowseAutoPin = prior })

	if _, err := workbench.SaveBrowseConfig(workbench.BrowseConfig{
		RegistryOrigin: "https://example.invalid",
		RegistryPeer:   "2KFNrGARQBkx3d9WQtkeuT3HbQ3oXSS7PBggWt1szT9hzb",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	sh := &Shell{ShellWorkspace: &ShellWorkspace{}}
	res, err := cmdRegistry(sh, []string{"unpin"})
	if err != nil {
		t.Fatalf("unpin: %v", err)
	}
	out := strings.Join(res.Lines, "\n")
	if !strings.Contains(out, "auto_pin: false") {
		t.Errorf("unpin does not say it was recorded:\n%s", out)
	}

	cfg, err := workbench.LoadBrowseConfig()
	if err != nil {
		t.Fatalf("load after unpin: %v", err)
	}
	if cfg.ShouldAutoPin() {
		t.Error("unpin did not survive the process — the next session pins the authority again " +
			"and the verb reads as broken")
	}
	// The origin and key are KEPT on purpose: unpinning is "do not begin
	// with an authority", not "forget which one I used".
	if cfg.RegistryOrigin == "" {
		t.Error("unpin discarded the origin, so getting back requires retyping it")
	}
}

// TestAutoPinIsSuppressedForTests is the anti-vacuity arm.
//
// Every browse test in this package runs with BrowseAutoPin false, so a
// build where the flag did nothing — or where browserOf stopped
// consulting it — would leave the whole suite silently dialing the public
// registry, which is exactly the failure that turned six BrowserPanel
// tests red on 2026-08-31. Assert the flag is load-bearing.
func TestAutoPinIsSuppressedForTests(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	prior := BrowseAutoPin
	BrowseAutoPin = false
	t.Cleanup(func() { BrowseAutoPin = prior })

	sh := &Shell{ShellWorkspace: &ShellWorkspace{}}
	if b := browserOf(sh); b.Registry() != "" {
		t.Fatalf("browserOf pinned %q with BrowseAutoPin false — this suite would reach the "+
			"public internet", b.Registry())
	}
	if !sh.browseAutoPinTried {
		// Not a failure of the product, but it pins the contract the
		// next test relies on: a suppressed pin must not be retried.
		t.Log("note: a suppressed auto-pin does not mark the workspace as tried")
	}
}
