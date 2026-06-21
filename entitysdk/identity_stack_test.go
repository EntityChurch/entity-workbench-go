package entitysdk_test

import (
	"strings"
	"testing"

	"entity-workbench-go/entitysdk"
)

// TestIdentityStack_DefaultOn confirms attestation + quorum + identity
// handlers are present after default CreatePeer. Per the user's
// "we want maximal" stance — every stable extension wires by default;
// minimized peers explicitly opt out.
func TestIdentityStack_DefaultOn(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	handlers := entitysdk.DiscoverHandlers(ap.PeerContext())
	want := map[string]bool{
		"system/attestation": false,
		"system/quorum":      false,
		"system/identity":    false,
		"system/role":        false,
	}
	for _, h := range handlers {
		if _, ok := want[h.Pattern]; ok {
			want[h.Pattern] = true
		}
	}
	for pattern, present := range want {
		if !present {
			t.Errorf("expected handler %q to be registered by default; not found", pattern)
		}
	}
}

// TestStableExtensions_AllDefaultOn confirms the full default-on
// extension set is registered after a zero-config CreatePeer. This
// is the conformance test for the "we want maximal" stance — a
// minimal AppPeer ships with every stable extension wired.
//
// Default-on includes the entire stable extension set:
// tree + query (always), identity stack (4 handlers),
// plus clock / continuation / content / handler / history /
// revision / compute. Subscription wires when not explicitly
// disabled but registers system/inbox + system/subscription rather
// than its own pattern, so it isn't asserted here.
func TestStableExtensions_AllDefaultOn(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	handlers := entitysdk.DiscoverHandlers(ap.PeerContext())
	want := map[string]bool{
		// Always-on.
		"system/tree":  false,
		"system/query": false,
		// Identity stack.
		"system/attestation": false,
		"system/quorum":      false,
		"system/identity":    false,
		"system/role":        false,
		// Stable extensions.
		"system/clock":        false,
		"system/continuation": false,
		"system/content":      false,
		"system/handler":      false,
		"system/history":      false,
		"system/revision":     false,
		"system/compute":      false,
	}
	for _, h := range handlers {
		if _, ok := want[h.Pattern]; ok {
			want[h.Pattern] = true
		}
	}
	for pattern, present := range want {
		if !present {
			t.Errorf("expected handler %q to be registered by default", pattern)
		}
	}
}

// TestIdentityStack_DisabledRemovesAll confirms the all-or-nothing
// opt-out: setting IdentityStack.Disabled = true unwires attestation,
// quorum, and identity together.
func TestIdentityStack_DisabledRemovesAll(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		Extensions: entitysdk.ExtensionsConfig{
			IdentityStack: &entitysdk.IdentityStackConfig{Disabled: true},
		},
	})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	handlers := entitysdk.DiscoverHandlers(ap.PeerContext())
	for _, h := range handlers {
		if strings.HasPrefix(h.Pattern, "system/attestation") ||
			strings.HasPrefix(h.Pattern, "system/quorum") ||
			strings.HasPrefix(h.Pattern, "system/identity") {
			t.Errorf("handler %q present despite IdentityStack disabled", h.Pattern)
		}
	}
}
