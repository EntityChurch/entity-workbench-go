package shellcmd_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"
	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/localfiles"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"
)

// TestStage3_Case2_Bidirectional documents F9 — cross-peer
// subscription delivery is asymmetric under bidirectional setup.
// CURRENTLY SKIPPED pending core-team / arch investigation.
//
// Surfaced in round 4 closeout. Setup: both peers run
// watchers + subscribe to each other's local/files/sync/* prefix +
// have blob-resolve handlers wired. Expected: alice writes file A
// → bob receives; bob writes file B → alice receives; both converge.
// Observed: alice writes → bob receives ✓ (direction α→β). Bob
// writes → alice does NOT receive ✗ (direction β→α).
//
// Diagnostics ruled out:
//   - Subscription registration: both records present on both trees
//     with correct patterns, deliver URIs, include_payload=true.
//   - Cap discipline: test uses OpenAccessGrants; if caps were the
//     issue we'd see 403, not silence.
//   - Transport routing: both peers have transport addresses for
//     each other (alice.Connect(bob) + bob.Connect(alice)) verified
//     by alice's successful subscribe to bob in the first place.
//   - Connect order: reversing alice.Connect / bob.Connect order
//     leaves the failure direction unchanged. NOT tied to who-
//     connected-first.
//   - Chain-error markers: 0 on alice, meaning alice's handler was
//     never invoked. Inbox on alice: 0 entries — notification never
//     arrived.
//
// Net: bob's subscription engine, when alice's subscription record
// fires for bob's own tree changes, either doesn't observe the
// change or fails to dispatch the cross-peer notification. Case
// 1.5 succeeded with the same shape but only one direction; case 2
// adds the symmetric setup and the second direction breaks.
//
// **Production impact:** bidirectional cross-peer sync is the
// default deployment shape (every node writes + receives). Until
// this is resolved, only one-way mirror topologies work cleanly.
// Hub-and-spoke would work with bob-as-hub if bob only publishes
// (single-direction); fully symmetric peer-to-peer does not.
//
// Owner: core-go subscription engine. Coordination needed with
// arch + sibling impls — Rust + Python may share the shape.
//
// What this validates beyond case 1.5:
//
//  1. Bidirectional subscription topology composes — each peer is
//     simultaneously a writer (watcher → tree) and a subscriber
//     (chain step → materialize). No coordination required.
//  2. Loop prevention works under cross-peer materialization.
//     When bob materializes alice's file via local/files:write,
//     bob's watcher will fire on the disk write. The watcher's
//     stat-cache (Amendment 1 L7) detects the same blob hash and
//     skips re-chunking; the reverseTracker (§5.5) suppresses the
//     loopback reverse-write. The full system must not spin.
//  3. Cap discipline: the chain capabilities each peer mints are
//     bound to their own blob-resolve handler; cross-peer
//     subscription delivery authorizes through standard V7 §5.10.
//     (Test uses OpenAccessGrants so the cap shape isn't deeply
//     exercised; Q3 ask to arch remains open for production
//     cap-chain design.)
//
// What this does NOT test (case 4 territory):
//   - Concurrent same-file edits (last-writer-wins semantics).
//   - Hot-spot concurrency on the same path under churn.
//
// Wall time budget is generous (30 s) to absorb worst-case watcher
// debounce on both ends + cross-peer round-trip + race-detector
// overhead.
func TestStage3_Case2_Bidirectional(t *testing.T) {
	// F9 closed by core-go 8ad52bc: handleWrite now calls
	// reverseTracker.markWritten symmetric to reverseWrite. Skip removed.
	aliceDir := t.TempDir()
	bobDir := t.TempDir()
	const rootName = "sync"
	const sourcePrefix = "local/files/" + rootName + "/"

	aliceBR := workbench.NewBlobResolveHandler()
	aliceBR.RegisterMount(sourcePrefix, sourcePrefix)
	bobBR := workbench.NewBlobResolveHandler()
	bobBR.RegisterMount(sourcePrefix, sourcePrefix)

	alice, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
		Handlers: []entitysdk.HandlerRegistration{
			{Pattern: workbench.BlobResolvePattern, Handler: aliceBR},
			{Pattern: workbench.ChainErrorsPattern, Handler: workbench.NewChainErrorsHandler()},
		},
	})
	if err != nil {
		t.Fatalf("CreatePeer alice: %v", err)
	}
	t.Cleanup(func() { _ = alice.Close() })

	bob, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
		Handlers: []entitysdk.HandlerRegistration{
			{Pattern: workbench.BlobResolvePattern, Handler: bobBR},
			{Pattern: workbench.ChainErrorsPattern, Handler: workbench.NewChainErrorsHandler()},
		},
	})
	if err != nil {
		t.Fatalf("CreatePeer bob: %v", err)
	}
	t.Cleanup(func() { _ = bob.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	bringUpListener(t, ctx, alice, "alice")
	bringUpListener(t, ctx, bob, "bob")
	// Reversed Connect order — test whether the failing direction
	// is tied to who-connected-first.
	if _, err := alice.Connect(ctx, bob.Addr().String()); err != nil {
		t.Fatalf("alice→bob connect: %v", err)
	}
	if _, err := bob.Connect(ctx, alice.Addr().String()); err != nil {
		t.Fatalf("bob→alice connect: %v", err)
	}

	aliceID := alice.PeerID()
	bobID := bob.PeerID()

	// Both peers mount the same prefix at their own filesystem root.
	// Both watchers run (this is what makes the loop-prevention test
	// real — when bob materializes alice's file, bob's watcher will
	// fire on the disk write).
	for _, p := range []struct {
		name   string
		ap     *entitysdk.AppPeer
		fsRoot string
	}{
		{"alice", alice, aliceDir},
		{"bob", bob, bobDir},
	} {
		lf := p.ap.LocalFilesHandler()
		if lf == nil {
			t.Fatalf("%s local/files handler not wired", p.name)
		}
		if err := lf.AddRoot(rootName, localfiles.RootConfigData{
			Prefix:         sourcePrefix,
			FilesystemRoot: p.fsRoot,
		}, p.ap.RawContentStore(), p.ap.RawLocationIndex()); err != nil {
			t.Fatalf("%s AddRoot: %v", p.name, err)
		}
		if err := lf.StartWatching(ctx, rootName, p.ap.RawContentStore(),
			p.ap.RawLocationIndex(), p.ap.IdentityHash()); err != nil {
			t.Fatalf("%s StartWatching: %v", p.name, err)
		}
	}

	// Mint chain caps + subscribe both directions symmetrically.
	for _, p := range []struct {
		name   string
		ap     *entitysdk.AppPeer
		other  *entitysdk.AppPeer
		thisID string
	}{
		{"alice", alice, bob, aliceID},
		{"bob", bob, alice, bobID},
	} {
		chainGrants := []types.GrantEntry{{
			Handlers:   types.CapabilityScope{Include: []string{workbench.BlobResolvePattern}},
			Operations: types.CapabilityScope{Include: []string{"receive"}},
		}}
		if _, err := p.ap.MintChainCapabilityBound(chainGrants,
			"system/capability/grants/chain/blob-resolve/"+rootName); err != nil {
			t.Fatalf("%s mint chain cap: %v", p.name, err)
		}
		otherID := p.other.PeerID()
		deliverURI := fmt.Sprintf("entity://%s/%s", p.thisID, workbench.BlobResolvePattern)
		if _, err := p.ap.SubscribeRawAt(otherID, sourcePrefix+"*", deliverURI, "receive",
			entitysdk.SubscribeOpts{
				Events:         []string{"created", "updated"},
				IncludePayload: true,
			}); err != nil {
			t.Fatalf("%s subscribe to %s: %v", p.name, otherID, err)
		}
	}

	// PHASE 1: alice writes alice-file.md → expect bob to receive.
	aliceContent := "# Alice's file\n\nWritten by alice; should appear on bob.\n"
	aliceFSPath := filepath.Join(aliceDir, "alice-file.md")
	if err := os.WriteFile(aliceFSPath, []byte(aliceContent), 0600); err != nil {
		t.Fatalf("alice write file: %v", err)
	}

	// Wait specifically for bob to receive before triggering alice's
	// receive. This bisects which direction is failing.
	bobReceivedPath := filepath.Join(bobDir, "alice-file.md")
	bobReceivedDeadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(bobReceivedDeadline) {
		if _, err := os.Stat(bobReceivedPath); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if _, err := os.Stat(bobReceivedPath); err != nil {
		t.Logf("DIAGNOSTIC: bob did NOT receive alice-file.md within 15 s — direction α→β broken")
	} else {
		t.Logf("DIAGNOSTIC: bob received alice-file.md ✓ — direction α→β works")
	}

	// PHASE 2: bob writes bob-file.md → expect alice to receive.
	bobContent := "# Bob's file\n\nWritten by bob; should appear on alice.\n"
	bobFSPath := filepath.Join(bobDir, "bob-file.md")
	if err := os.WriteFile(bobFSPath, []byte(bobContent), 0600); err != nil {
		t.Fatalf("bob write file: %v", err)
	}

	aliceReceivedPath := filepath.Join(aliceDir, "bob-file.md")
	aliceReceivedDeadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(aliceReceivedDeadline) {
		if _, err := os.Stat(aliceReceivedPath); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if _, err := os.Stat(aliceReceivedPath); err != nil {
		t.Logf("DIAGNOSTIC: alice did NOT receive bob-file.md within 15 s — direction β→α broken")
		// Inspect the actual subscription records on both peers.
		for _, p := range []struct {
			name string
			ap   *entitysdk.AppPeer
		}{{"alice", alice}, {"bob", bob}} {
			subs := listPrefix(p.ap, "system/subscription/")
			for _, s := range subs {
				ent, ok, err := p.ap.Get(stripPeer(s))
				if err != nil || !ok {
					t.Logf("β→α DIAG %s sub read fail: %v ok=%v", p.name, err, ok)
					continue
				}
				sub, derr := types.SubscriptionDataFromEntity(ent)
				if derr != nil {
					t.Logf("β→α DIAG %s sub decode fail: %v", p.name, derr)
					continue
				}
				t.Logf("β→α DIAG %s sub: id=%s pattern=%q events=%v deliver=%q op=%q include_payload=%v",
					p.name, sub.SubscriptionID, sub.Pattern, sub.Events, sub.DeliverURI, sub.DeliverOperation, sub.IncludePayload)
			}
		}
	} else {
		t.Logf("DIAGNOSTIC: alice received bob-file.md ✓ — direction β→α works")
	}

	// Wait for both files to converge on both peers' filesystems.
	// Expected paths after convergence:
	//   alice's disk: alice-file.md (own), bob-file.md (received)
	//   bob's disk:   alice-file.md (received), bob-file.md (own)
	convergenceTargets := []struct {
		who    string
		fsPath string
		want   string
	}{
		{"alice has alice-file.md", filepath.Join(aliceDir, "alice-file.md"), aliceContent},
		{"alice has bob-file.md", filepath.Join(aliceDir, "bob-file.md"), bobContent},
		{"bob has alice-file.md", filepath.Join(bobDir, "alice-file.md"), aliceContent},
		{"bob has bob-file.md", filepath.Join(bobDir, "bob-file.md"), bobContent},
	}

	deadline := time.Now().Add(30 * time.Second)
	for _, target := range convergenceTargets {
		landed := false
		for time.Now().Before(deadline) {
			if _, err := os.Stat(target.fsPath); err == nil {
				landed = true
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if !landed {
			// Surface full diagnostics from both peers.
			for _, p := range []struct {
				name string
				ap   *entitysdk.AppPeer
			}{{"alice", alice}, {"bob", bob}} {
				errPaths := listPrefix(p.ap, "system/runtime/chain-errors/")
				t.Logf("%s chain-error markers: %d", p.name, len(errPaths))
				for _, ep := range errPaths {
					t.Logf("  %s: %s", p.name, ep)
				}
				subs := listPrefix(p.ap, "system/subscription/")
				t.Logf("%s subscriptions: %d", p.name, len(subs))
				for _, s := range subs {
					t.Logf("  %s", s)
				}
				files := listPrefix(p.ap, sourcePrefix)
				t.Logf("%s tree entries under %s: %d", p.name, sourcePrefix, len(files))
				for _, f := range files {
					t.Logf("  %s", f)
				}
				// List the FS contents of each peer's dir too.
				dir := aliceDir
				if p.name == "bob" {
					dir = bobDir
				}
				entries, _ := os.ReadDir(dir)
				t.Logf("%s filesystem dir %s: %d entries", p.name, dir, len(entries))
				for _, e := range entries {
					t.Logf("  %s", e.Name())
				}
			}
			t.Fatalf("%s — did not materialize within deadline", target.who)
		}
		gotBytes, err := os.ReadFile(target.fsPath)
		if err != nil {
			t.Fatalf("%s read: %v", target.who, err)
		}
		if string(gotBytes) != target.want {
			t.Errorf("%s content mismatch:\n  got:  %q\n  want: %q",
				target.who, string(gotBytes), target.want)
		} else {
			t.Logf("%s ✓ (%d bytes)", target.who, len(gotBytes))
		}
	}

	// Loop-prevention probe: wait an extra debounce window and check
	// chain-error markers + that we haven't seen unbounded growth in
	// the file entity bindings under sourcePrefix on either peer.
	// (Each file should land exactly once per peer; no ping-pong.)
	time.Sleep(3 * time.Second)
	aliceFiles := listPrefix(alice, sourcePrefix)
	bobFiles := listPrefix(bob, sourcePrefix)
	t.Logf("post-convergence: alice has %d entities under %s, bob has %d",
		len(aliceFiles), sourcePrefix, len(bobFiles))
	if len(aliceFiles) > 2 {
		t.Errorf("alice has more than the expected 2 file entities under %s: %d", sourcePrefix, len(aliceFiles))
		for _, p := range aliceFiles {
			t.Logf("  %s", p)
		}
	}
	if len(bobFiles) > 2 {
		t.Errorf("bob has more than the expected 2 file entities under %s: %d", sourcePrefix, len(bobFiles))
		for _, p := range bobFiles {
			t.Logf("  %s", p)
		}
	}

	for _, p := range []struct {
		name string
		ap   *entitysdk.AppPeer
	}{{"alice", alice}, {"bob", bob}} {
		errPaths := listPrefix(p.ap, "system/runtime/chain-errors/")
		if len(errPaths) > 0 {
			t.Logf("%s chain-error markers AT END: %d", p.name, len(errPaths))
			for _, ep := range errPaths {
				t.Logf("  %s", ep)
			}
		}
	}
}
