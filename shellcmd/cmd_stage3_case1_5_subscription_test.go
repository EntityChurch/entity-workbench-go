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

// TestStage3_Case1_5_SubscriptionDriven is the subscription-driven
// version of case 1 (TestStage3_Case1_SubstrateExplicit). Where case
// 1 invoked the cross-peer dispatch sequence explicitly to verify
// each substrate surface, case 1.5 wires the same sequence behind a
// subscription on alice's local/files/sync/* and asserts that an
// fs write on alice fans out to a materialized file on bob without
// any explicit caller dispatch.
//
// This is the first Stage 3 cross-peer chain composition test —
// the workbench/blob-resolve handler is the chain step that wraps
// the case 1 dispatch sequence. Per the Stage 3 design response §3,
// the chain shape is:
//
//	subscribe(alice's local/files/sync/*, include_payload=true)
//	  → workbench/blob-resolve handler (single-step collapse, Q2 shape)
//	      a. unwrap delivery → notification → file entity from Included
//	      b. ContentClient-style fetch closure cross-peer (§7.2)
//	      c. local/files:write content-mode → atomic write to bob's disk
//
// What this validates beyond case 1:
//
//  1. EXTENSION-SUBSCRIPTION v3.14 include_payload delivers the file
//     entity to the handler via hctx.Included (no separate tree:get).
//  2. The single-handler shape composes correctly under subscription
//     dispatch — same Q2-collapse pattern as notification_ingest but
//     for the cross-peer substrate path.
//  3. arch's L10 cleaner-path framing materializes: cap-checked
//     cross-peer dispatch through system/content:get under chain cap;
//     no substrate-direct trust-boundary crossings.
//  4. hctx.Execute (the in-handler dispatch capability) is sufficient
//     for blob-resolve's cross-peer + local dispatches — no
//     AppPeer back-reference needed.
//
// Open dependency on arch: Q3 (cap-chain visibility through
// TreeChangeEvent.context for cross-peer subscription delivery
// authorization). For this test we use OpenAccessGrants so the cap
// shape isn't exercised; documenting the dependency for round 4 +
// flagging arch.
func TestStage3_Case1_5_SubscriptionDriven(t *testing.T) {
	aliceDir := t.TempDir()
	bobDir := t.TempDir()
	const rootName = "sync"
	const sourcePrefix = "local/files/" + rootName + "/"

	// blob-resolve handler — sits on bob.
	bobBlobResolve := workbench.NewBlobResolveHandler()
	bobBlobResolve.RegisterMount(sourcePrefix, sourcePrefix) // symmetric mount

	alice, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("CreatePeer alice: %v", err)
	}
	t.Cleanup(func() { _ = alice.Close() })

	bob, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
		Handlers: []entitysdk.HandlerRegistration{
			{Pattern: workbench.BlobResolvePattern, Handler: bobBlobResolve},
			{Pattern: workbench.ChainErrorsPattern, Handler: workbench.NewChainErrorsHandler()},
		},
	})
	if err != nil {
		t.Fatalf("CreatePeer bob: %v", err)
	}
	t.Cleanup(func() { _ = bob.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	bringUpListener(t, ctx, alice, "alice")
	bringUpListener(t, ctx, bob, "bob")
	if _, err := bob.Connect(ctx, alice.Addr().String()); err != nil {
		t.Fatalf("bob→alice connect: %v", err)
	}
	if _, err := alice.Connect(ctx, bob.Addr().String()); err != nil {
		t.Fatalf("alice→bob connect: %v", err)
	}

	aliceID := alice.PeerID()
	bobID := bob.PeerID()

	// Alice mounts her dir + starts the watcher.
	aliceLF := alice.LocalFilesHandler()
	if aliceLF == nil {
		t.Fatal("alice local/files handler not wired")
	}
	if err := aliceLF.AddRoot(rootName, localfiles.RootConfigData{
		Prefix:         sourcePrefix,
		FilesystemRoot: aliceDir,
	}, alice.RawContentStore(), alice.RawLocationIndex()); err != nil {
		t.Fatalf("alice AddRoot: %v", err)
	}
	if err := aliceLF.StartWatching(ctx, rootName, alice.RawContentStore(),
		alice.RawLocationIndex(), alice.IdentityHash()); err != nil {
		t.Fatalf("alice StartWatching: %v", err)
	}

	// Bob mounts the same prefix at his own dir so the materialization
	// dispatch (local/files:write at sourcePrefix+relpath) lands.
	bobLF := bob.LocalFilesHandler()
	if bobLF == nil {
		t.Fatal("bob local/files handler not wired")
	}
	if err := bobLF.AddRoot(rootName, localfiles.RootConfigData{
		Prefix:         sourcePrefix,
		FilesystemRoot: bobDir,
	}, bob.RawContentStore(), bob.RawLocationIndex()); err != nil {
		t.Fatalf("bob AddRoot: %v", err)
	}
	// Bob doesn't start watching — he's the sink in this one-way
	// case; auto-rechunk would create write-loops we don't want to
	// debug here.

	// Mint a chain capability on bob authorizing the
	// workbench/blob-resolve:receive dispatch the subscription
	// notification will trigger.
	chainGrants := []types.GrantEntry{{
		Handlers:   types.CapabilityScope{Include: []string{workbench.BlobResolvePattern}},
		Operations: types.CapabilityScope{Include: []string{"receive"}},
	}}
	if _, err := bob.MintChainCapabilityBound(chainGrants,
		"system/capability/grants/chain/blob-resolve/"+rootName); err != nil {
		t.Fatalf("mint blob-resolve chain cap: %v", err)
	}

	// Subscribe bob to alice's local/files/sync/* with
	// include_payload: true so the notification's payload (the
	// FileData entity) arrives at blob-resolve via hctx.Included.
	deliverURI := fmt.Sprintf("entity://%s/%s", bobID, workbench.BlobResolvePattern)
	if _, err := bob.SubscribeRawAt(aliceID, sourcePrefix+"*", deliverURI, "receive",
		entitysdk.SubscribeOpts{
			Events:         []string{"created", "updated"},
			IncludePayload: true,
		}); err != nil {
		t.Fatalf("bob subscribe: %v", err)
	}

	// Alice writes the file. Watcher fires → FileData binds at
	// sourcePrefix+"hello.md" on alice → subscription delivers
	// notification to bob's blob-resolve handler → handler fetches
	// blob closure from alice → dispatches local/files:write
	// content-mode locally → atomic write to bobDir/hello.md.
	mdContent := "# Stage 3 case 1.5\n\nSubscription-driven cross-peer file sync.\n"
	mdPath := filepath.Join(aliceDir, "hello.md")
	if err := os.WriteFile(mdPath, []byte(mdContent), 0600); err != nil {
		t.Fatalf("alice write file: %v", err)
	}

	// Wait for alice's watcher to bind the FileData.
	wantSourcePath := sourcePrefix + "hello.md"
	if !pollUntilBound(alice, wantSourcePath, 5*time.Second) {
		t.Fatalf("alice's watcher never bound %s", wantSourcePath)
	}

	// Wait for the file to appear on bob's filesystem. The total
	// wall time should be on the order of the debounce window (~2 s)
	// plus the dispatch chain (~ms). Generous deadline absorbs CI
	// jitter.
	bobFSPath := filepath.Join(bobDir, "hello.md")
	deadline := time.Now().Add(15 * time.Second)
	var landed bool
	for time.Now().Before(deadline) {
		if _, err := os.Stat(bobFSPath); err == nil {
			landed = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !landed {
		// Diagnostic: surface any chain-error markers on bob.
		errPaths := listPrefix(bob, "system/runtime/chain-errors/")
		t.Logf("bob chain-error markers: %d", len(errPaths))
		for _, p := range errPaths {
			t.Logf("  %s", p)
		}
		t.Fatalf("bob's hello.md did not appear after 15 s — chain didn't materialize")
	}

	// Content match: byte-for-byte.
	gotBytes, err := os.ReadFile(bobFSPath)
	if err != nil {
		t.Fatalf("read bob's hello.md: %v", err)
	}
	if string(gotBytes) != mdContent {
		t.Errorf("bob's hello.md content mismatch:\n  got:  %q\n  want: %q",
			string(gotBytes), mdContent)
	} else {
		t.Logf("bob's filesystem has hello.md with matching content (%d bytes) — chain composed end-to-end",
			len(gotBytes))
	}

	// Dedup: blob hash present on both peers' content stores.
	fileEnt, ok, err := bob.Get(sourcePrefix + "hello.md")
	if err != nil || !ok {
		t.Fatalf("bob's tree:get for own materialized file failed: ok=%v err=%v", ok, err)
	}
	file, err := localfiles.FileDataFromEntity(fileEnt)
	if err != nil {
		t.Fatalf("decode bob's FileData: %v", err)
	}
	if _, ok := alice.RawContentStore().Get(file.Content); !ok {
		t.Errorf("alice's content store missing blob %s after chain", file.Content)
	}
	if _, ok := bob.RawContentStore().Get(file.Content); !ok {
		t.Errorf("bob's content store missing blob %s after chain", file.Content)
	}
}
