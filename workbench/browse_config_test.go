package workbench

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The browser's start-up configuration is policy, so it is tested here
// rather than through a panel. A GUI test of this would need the network
// and would be measuring a remote host.

func TestLoadBrowseConfig_DefaultPinsThePublicRegistryWithNoKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("WB_REGISTRY_ORIGIN", "")
	t.Setenv("WB_REGISTRY_PEER", "")
	t.Setenv("WB_NO_AUTOPIN", "")

	cfg, err := LoadBrowseConfig()
	if err != nil {
		t.Fatalf("LoadBrowseConfig with no file: %v", err)
	}
	if cfg.RegistryOrigin != DefaultRegistryOrigin {
		t.Errorf("origin = %q; want %q", cfg.RegistryOrigin, DefaultRegistryOrigin)
	}
	// The peer being empty is the whole design: shipping a peer-id would
	// bake a key into the binary that no operator agreed to and that
	// cannot be rotated without a release.
	if cfg.RegistryPeer != "" {
		t.Errorf("the built-in default must not carry a peer-id; got %q", cfg.RegistryPeer)
	}
	if !cfg.ShouldAutoPin() {
		t.Error("the default must auto-pin — a browser that opens with no name authority can do nothing")
	}
}

func TestLoadBrowseConfig_FileOverridesDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WB_REGISTRY_ORIGIN", "")
	t.Setenv("WB_REGISTRY_PEER", "")
	t.Setenv("WB_NO_AUTOPIN", "")

	writeConfig(t, home, `{"registry_origin":"https://ops.example","registry_peer":"2KABC"}`)

	cfg, err := LoadBrowseConfig()
	if err != nil {
		t.Fatalf("LoadBrowseConfig: %v", err)
	}
	if cfg.RegistryOrigin != "https://ops.example" || cfg.RegistryPeer != "2KABC" {
		t.Errorf("file did not take effect: origin=%q peer=%q", cfg.RegistryOrigin, cfg.RegistryPeer)
	}
	if !strings.Contains(cfg.Source, "browser.json") {
		t.Errorf("Source must name the file it came from; got %q", cfg.Source)
	}
}

func TestLoadBrowseConfig_FileMayDisableAutoPin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WB_REGISTRY_ORIGIN", "")
	t.Setenv("WB_NO_AUTOPIN", "")

	writeConfig(t, home, `{"auto_pin":false}`)

	cfg, err := LoadBrowseConfig()
	if err != nil {
		t.Fatalf("LoadBrowseConfig: %v", err)
	}
	if cfg.ShouldAutoPin() {
		t.Error(`"auto_pin":false must be honoured — an operator who wants to start unpinned gets to`)
	}
}

func TestLoadBrowseConfig_EnvBeatsFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WB_NO_AUTOPIN", "")
	writeConfig(t, home, `{"registry_origin":"https://from-file","registry_peer":"FILEPEER"}`)
	t.Setenv("WB_REGISTRY_ORIGIN", "https://from-env")
	t.Setenv("WB_REGISTRY_PEER", "ENVPEER")

	cfg, err := LoadBrowseConfig()
	if err != nil {
		t.Fatalf("LoadBrowseConfig: %v", err)
	}
	if cfg.RegistryOrigin != "https://from-env" || cfg.RegistryPeer != "ENVPEER" {
		t.Errorf("env must win: origin=%q peer=%q", cfg.RegistryOrigin, cfg.RegistryPeer)
	}
}

// A config file that exists and does not parse must FAIL, not be
// silently ignored (AP33). The operator believes a setting is in effect;
// the expensive outcome is the browser quietly using a different
// registry than the one they configured.
func TestLoadBrowseConfig_MalformedFileIsRefusedNotIgnored(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WB_REGISTRY_ORIGIN", "")
	t.Setenv("WB_REGISTRY_PEER", "")
	t.Setenv("WB_NO_AUTOPIN", "")

	writeConfig(t, home, `{"registry_origin": "https://x",}`) // trailing comma

	_, err := LoadBrowseConfig()
	if err == nil {
		t.Fatal("a malformed browser.json must be reported, not ignored")
	}
	if !strings.Contains(err.Error(), "browser.json") {
		t.Errorf("the error must name the file so it can be fixed; got %v", err)
	}
}

func writeConfig(t *testing.T, home, body string) {
	t.Helper()
	dir := filepath.Join(home, ".entity")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "browser.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}
