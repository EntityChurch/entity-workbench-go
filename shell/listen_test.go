package shell_test

import (
	"context"
	"testing"
	"time"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/shell"
)

// TestShellListenIsActuallyDialable is the gate on a flag that shipped
// doing nothing.
//
// `entity-shell -listen ADDR` carried its address into
// `peer.WithListenAddr` and no code path in this repo ever called
// `Peer.Listen` for it — core-go reads that field in exactly one place —
// so the shell could dial out and could never be dialled. The help text
// said "TCP listener for inbound peer connections"; step 1 of
// USAGE-SHARE-A-FOLDER.md says to use it; and the entire two-machine
// share flow starts by depending on it.
//
// **The assertion has to be a completed handshake from a second peer.**
// Anything cheaper passes against the broken build:
//
//   - `app.Listener() != nil` passes against a bring-up that returns a
//     struct and binds nothing.
//   - reading back the configured address passes trivially — the config
//     was always correct; it was never acted on.
//   - every e2e suite in this repo already "covers" listening and misses
//     this, because each stands its own listener up with a local helper
//     rather than through the frontend's own path. That is the whole
//     mechanism of the miss and it is AP62's shape one layer out: the
//     product's startup sequence is the thing under test, and a fixture
//     that reimplements a step of it cannot fail on that step.
func TestShellListenIsActuallyDialable(t *testing.T) {
	t.Parallel()

	app, err := shell.New(shell.Config{
		LocalAlias: "listener",
		ListenAddr: "127.0.0.1:0", // port 0: no fixed port to collide under -race
	})
	if err != nil {
		t.Fatalf("shell.New with -listen: %v", err)
	}
	defer app.Close()

	li := app.Listener()
	if li == nil {
		t.Fatal("shell.New(ListenAddr set) produced no listener — the flag is inert again")
	}
	if li.BoundAddr == "" || li.BoundAddr == "127.0.0.1:0" {
		t.Fatalf("listener reports BoundAddr %q — the bound port was never read back", li.BoundAddr)
	}

	// The dial. A second, listener-less peer connects to the address the
	// shell says it is on; Connect performs the full handshake, so a
	// non-error here means a socket was accepted and a session
	// established, which is exactly the claim `-listen` makes.
	dialer, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("create dialing peer: %v", err)
	}
	defer dialer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := dialer.Connect(ctx, li.BoundAddr); err != nil {
		t.Fatalf("dial the shell's advertised listener at %s: %v", li.BoundAddr, err)
	}
}

// TestShellOutboundOnlyStaysOutbound is the control arm: with no
// ListenAddr there is no listener and nothing claims otherwise. Without
// it, the test above could be satisfied by a bring-up that binds
// unconditionally, which would be a different bug (a CLI that opens a
// port nobody asked for) reported as a pass.
func TestShellOutboundOnlyStaysOutbound(t *testing.T) {
	t.Parallel()

	app, err := shell.New(shell.Config{LocalAlias: "quiet"})
	if err != nil {
		t.Fatalf("shell.New with no listen addr: %v", err)
	}
	defer app.Close()

	if li := app.Listener(); li != nil {
		t.Fatalf("no ListenAddr was configured yet a listener bound at %s", li.BoundAddr)
	}
}
