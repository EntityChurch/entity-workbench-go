package shellcmd

import (
	"context"
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

// TestCmdContinuation_LsAndInspect: install a continuation via the
// SDK, then drive the shell verbs against the same peer. Verifies
// the wiring + rendering path.
func TestCmdContinuation_LsAndInspect(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	sh := NewShell(ap, "local", "")

	// Install a continuation directly via the SDK.
	const path = "system/inbox/test/shellprobe/fetch"
	contEnt, err := types.ContinuationData{
		Target:             "system/tree",
		Operation:          "get",
		Resource:           &types.ResourceTarget{Targets: []string{"a/"}},
		DispatchCapability: ap.OwnerCapability().ContentHash,
	}.ToEntity()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ap.Continuation().Install(context.Background(), path, contEnt); err != nil {
		t.Fatalf("install: %v", err)
	}

	// `continuation ls system/inbox/test/shellprobe/` returns 1 line.
	res, err := cmdContinuation(sh, []string{"ls", "system/inbox/test/shellprobe/"})
	if err != nil {
		t.Fatalf("continuation ls: %v", err)
	}
	if res.Kind != KindLines {
		t.Fatalf("expected KindLines, got %v", res.Kind)
	}
	joined := strings.Join(res.Lines, "\n")
	if !strings.Contains(joined, "forward") || !strings.Contains(joined, path) {
		t.Errorf("ls output doesn't mention forward + path:\n%s", joined)
	}

	// `continuation inspect <path>` shows detailed fields.
	res, err = cmdContinuation(sh, []string{"inspect", path})
	if err != nil {
		t.Fatalf("continuation inspect: %v", err)
	}
	if res.Kind != KindLines {
		t.Fatalf("expected KindLines, got %v", res.Kind)
	}
	joined = strings.Join(res.Lines, "\n")
	for _, want := range []string{"target:", "operation:", "system/tree", "get"} {
		if !strings.Contains(joined, want) {
			t.Errorf("inspect output missing %q:\n%s", want, joined)
		}
	}
}

// TestCmdSubscription_LsAfterSubscribe: subscribe via SDK, verify
// `subscription ls` lists it.
func TestCmdSubscription_LsAfterSubscribe(t *testing.T) {
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ap.Close() })

	sh := NewShell(ap, "local", "")

	sub, err := ap.Subscribe("local/test/*", entitysdk.SubscribeOpts{})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(func() { _ = sub.Close() })

	res, err := cmdSubscription(sh, []string{"ls"})
	if err != nil {
		t.Fatalf("subscription ls: %v", err)
	}
	if res.Kind != KindLines {
		t.Fatalf("expected KindLines, got %v", res.Kind)
	}
	joined := strings.Join(res.Lines, "\n")
	if !strings.Contains(joined, "local/test/*") {
		t.Errorf("ls output missing pattern:\n%s", joined)
	}
}
