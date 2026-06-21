package entitysdk_test

import (
	"testing"

	"entity-workbench-go/entitysdk"
)

// TestLocalFilesExtension_WiredByDefault confirms the local/files
// handler is registered out of the box. Phase E mount verbs depend
// on AppPeer.LocalFilesHandler() being non-nil.
func TestLocalFilesExtension_WiredByDefault(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	defer ap.Close()
	if ap.LocalFilesHandler() == nil {
		t.Error("LocalFilesHandler is nil — extension didn't wire by default")
	}
}

// TestLocalFilesExtension_DisabledOptOut confirms the explicit
// opt-out path. Useful for deeply minimal peers that don't need
// the filesystem bridge.
func TestLocalFilesExtension_DisabledOptOut(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		Extensions: entitysdk.ExtensionsConfig{
			LocalFiles: &entitysdk.LocalFilesConfig{Disabled: true},
		},
	})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	defer ap.Close()
	if ap.LocalFilesHandler() != nil {
		t.Error("LocalFilesHandler is non-nil despite Disabled=true")
	}
}
