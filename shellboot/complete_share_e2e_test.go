package shellboot_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"entity-workbench-go/shellcmd"
)

// The GUI's "Complete connection" button, end to end.
//
// `TestFlow_DiscoverShareAuthorizeMountSync_NoOpenAccess` already proves
// the flow delivers bytes, but it performs the final reciprocal dial with
// `connectVerb(t, alice, "bob", bob.ap.Addr().String())` — the address
// handed in from the test's own knowledge. That covers the dial and
// covers NOTHING about how an operator's machine is supposed to obtain
// that address, which is the entire difficulty of this step and the part
// the GUI has to answer.
//
// `ShellWorkspace.CompleteShare` is what the button calls. Its job is
// address RESOLUTION plus the dial, and its three outcomes are three
// different things a surface must render differently:
//
//   - resolved from the connection table (we dialled them once) → dial,
//     no operator involvement
//   - nothing known → NeedsAddress, which is a QUESTION, not an error
//   - an address supplied → dial, and remember it so it is asked once
//
// The middle one is why this test exists. It is the common case on the
// sharing side — the sharer is frequently the peer holding an INBOUND
// connection, whose observed remote address is the dialer's ephemeral
// source port (V7 §6.7.1 states it MUST NOT be treated as dialable) — and
// a surface that renders it as a failure tells the operator something
// broke when the truth is that nobody has said where the peer listens.
func TestCompleteShare_ResolvesOrAsks_AndDeliversAfterwards(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	alice := newFlowPeer(t, ctx, "alice")
	bob := newFlowPeer(t, ctx, "bob")

	// Bob dials alice. This is the realistic asymmetry: bob now holds a
	// dialable address for alice, and alice holds an inbound connection
	// and nothing she can dial back.
	connectVerb(t, bob, "alice", alice.ap.Addr().String())

	const folder = "completetest"
	aliceDir := filepath.Join(t.TempDir(), folder)
	bobDir := filepath.Join(t.TempDir(), folder)
	if err := os.MkdirAll(aliceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bobDir, 0o755); err != nil {
		t.Fatal(err)
	}

	aliceMount, err := alice.ws.Mount(shellcmd.MountRequest{
		FilesystemDir: aliceDir, TargetPrefix: "archives/" + folder + "/",
	})
	if err != nil {
		t.Fatalf("alice mount: %v", err)
	}
	if _, err := bob.ws.Mount(shellcmd.MountRequest{
		FilesystemDir: bobDir, TargetPrefix: "archives/" + folder + "/",
	}); err != nil {
		t.Fatalf("bob mount: %v", err)
	}
	root := aliceMount.RootName

	if _, err := alice.ws.Share(shellcmd.ShareRequest{
		Root: root, Peer: bob.ap.PeerID(), NowMillis: 1_756_000_000_000,
	}); err != nil {
		t.Fatalf("share: %v", err)
	}

	// --- the state the panel renders BEFORE the operator acts ---------
	//
	// The button's label and whether an address field appears beside it
	// are both decided by this, so it is asserted directly rather than
	// inferred from a click outcome. There is no mDNS in this test (no
	// suite here may depend on the host's avahi) and alice has never
	// dialled bob, so the honest answer is "nothing known".
	if addr, src := alice.ws.DialableAddressFor(bob.ap.PeerID()); addr != "" {
		t.Fatalf("alice reports a dialable address %q (source %q) for a peer she has "+
			"never dialled and cannot hear announcing — if this is bob's ephemeral "+
			"source port it will be refused, and the panel will offer a button that "+
			"cannot work instead of asking for an address", addr, src)
	}

	if _, err := bob.ws.Accept(alice.ap.PeerID(), root); err != nil {
		t.Fatalf("accept: %v", err)
	}

	// --- CompleteShare with nothing to go on --------------------------
	//
	// Must be a question, not a failure: ok, no error, NeedsAddress set,
	// and a note the panel can show. A non-nil error here would make the
	// panel render "could not complete" in red for a situation in which
	// nothing is wrong.
	ask, err := alice.ws.CompleteShare(bob.ap.PeerID(), "")
	if err != nil {
		t.Fatalf("CompleteShare with no address returned an ERROR (%v) — the panel "+
			"renders that as a failure, but not knowing where a peer listens is a "+
			"question to ask the operator", err)
	}
	if !ask.NeedsAddress {
		t.Fatalf("CompleteShare did not report NeedsAddress; got %+v", ask)
	}
	if ask.Connected {
		t.Fatal("CompleteShare reported connected without an address")
	}
	if strings.TrimSpace(ask.Note) == "" {
		t.Error("NeedsAddress carries no note — the panel has nothing to show the " +
			"operator about what it is asking for or why")
	}

	// --- CompleteShare with the address the operator supplies ---------
	done, err := alice.ws.CompleteShare(bob.ap.PeerID(), bob.ap.Addr().String())
	if err != nil {
		t.Fatalf("CompleteShare with an address: %v", err)
	}
	if !done.Connected {
		t.Fatalf("CompleteShare did not connect: note=%q addr=%q", done.Note, done.Address)
	}
	if done.NeedsAddress {
		t.Error("CompleteShare reported both connected and NeedsAddress")
	}
	if done.AddressSource != "given" {
		t.Errorf("address source %q, want \"given\" — the panel shows this to say "+
			"whether an address was resolved or typed", done.AddressSource)
	}

	// --- and now the bytes actually cross -----------------------------
	//
	// The assertion that matters. Everything above is state; this is
	// whether pressing the button in the GUI makes a shared folder work.
	const body = "delivered after Complete connection\n"
	if err := os.WriteFile(filepath.Join(aliceDir, "note.md"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	landed := filepath.Join(bobDir, "note.md")
	if !awaitFileContent(landed, body, 45*time.Second) {
		got, rerr := os.ReadFile(landed)
		t.Fatalf("note.md never reached bob after CompleteShare (read %q, err %v) — "+
			"the button completes the flow or it is decoration", string(got), rerr)
	}

	// --- the address is REMEMBERED ------------------------------------
	//
	// Which is what makes this a one-time step per peer rather than a
	// recurring chore, and what lets a later reconnect resolve without
	// asking again. Asserted through the public accessor the panel reads,
	// not through the connection table's internals.
	addr, src := alice.ws.DialableAddressFor(bob.ap.PeerID())
	if addr == "" {
		t.Fatal("after a successful CompleteShare, alice still has no dialable " +
			"address for bob — the operator would be asked for it again every time")
	}
	if src != "connection-table" {
		t.Errorf("remembered address source is %q, want \"connection-table\"", src)
	}
}
