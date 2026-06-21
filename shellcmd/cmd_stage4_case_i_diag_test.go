package shellcmd_test

// Diagnostic — minimal 2-peer mesh through the Stage 4 harness with a
// single one-direction file write. Isolates whether the harness can do
// what Stage 3 case 2 does directly.

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"
	"go.entitychurch.org/entity-core-go/core/types"
	"go.entitychurch.org/entity-core-go/ext/localfiles"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"
)

func TestStage4_Diag_TwoPeerConcurrentDifferentFiles(t *testing.T) {
	const rootName = "sync"
	const sourcePrefix = "local/files/" + rootName + "/"

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	peers := stage4Setup(t, ctx, 2, []string{"alice", "bob"}, rootName, sourcePrefix)
	stage4ConnectAllToAll(t, ctx, peers)
	stage4StartWatchers(t, ctx, peers, rootName, sourcePrefix)
	stage4WireMesh(t, peers, rootName, sourcePrefix)

	// Concurrent writes of DIFFERENT files. This is the case 2 shape
	// (different filenames) but with the timing where both peers write
	// AT THE SAME TIME (instead of sequentially as in case 2). Probes
	// whether the 2-peer mesh handles concurrent-different-path writes.
	aliceContent := "# Alice\n"
	bobContent := "# Bob\n"
	alicePath := filepath.Join(peers[0].fsRoot, "alice.md")
	bobPath := filepath.Join(peers[1].fsRoot, "bob.md")
	if err := os.WriteFile(alicePath, []byte(aliceContent), 0600); err != nil {
		t.Fatalf("alice write: %v", err)
	}
	if err := os.WriteFile(bobPath, []byte(bobContent), 0600); err != nil {
		t.Fatalf("bob write: %v", err)
	}

	// WB-28 fix verification — core-go 6ebdd78 added connection-level
	// multiplexing in core/peer/connection.go. The pre-fix shape was:
	// both blob-resolve handlers spin up, both try to fetch back to
	// source synchronously, deadlock, time out at 15s with each peer
	// keeping only its own content. Post-fix: 2-peer concurrent
	// writes converge cleanly. Promoted from diagnostic-only to
	// assertion-mode in round-2 to lock the fix in CI.
	deadline := time.Now().Add(20 * time.Second)
	if !stage4AwaitFile(peers[0], "bob.md", deadline) {
		t.Fatalf("WB-28 REGRESSION: alice did not receive bob.md within 20s; reentrant-RPC deadlock is back")
	}
	if !stage4AwaitFile(peers[1], "alice.md", deadline) {
		t.Fatalf("WB-28 REGRESSION: bob did not receive alice.md within 20s; reentrant-RPC deadlock is back")
	}
	t.Logf("WB-28 pin: 2-peer concurrent-different-files converged ✓")
}

func TestStage4_Diag_TwoPeerSequentialDifferentFiles(t *testing.T) {
	const rootName = "sync"
	const sourcePrefix = "local/files/" + rootName + "/"

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	peers := stage4Setup(t, ctx, 2, []string{"alice", "bob"}, rootName, sourcePrefix)
	stage4ConnectAllToAll(t, ctx, peers)
	stage4StartWatchers(t, ctx, peers, rootName, sourcePrefix)
	stage4WireMesh(t, peers, rootName, sourcePrefix)

	// Sequential writes — alice writes first, wait for delivery, THEN
	// bob writes. Matches Stage 3 case 2's pattern. This should work.
	alicePath := filepath.Join(peers[0].fsRoot, "alice-seq.md")
	if err := os.WriteFile(alicePath, []byte("# Alice seq\n"), 0600); err != nil {
		t.Fatalf("alice write: %v", err)
	}
	deadline1 := time.Now().Add(20 * time.Second)
	if !stage4AwaitFile(peers[1], "alice-seq.md", deadline1) {
		t.Fatalf("bob did not receive alice's file")
	}
	t.Logf("phase 1 ok")

	bobPath := filepath.Join(peers[1].fsRoot, "bob-seq.md")
	if err := os.WriteFile(bobPath, []byte("# Bob seq\n"), 0600); err != nil {
		t.Fatalf("bob write: %v", err)
	}
	deadline2 := time.Now().Add(20 * time.Second)
	if !stage4AwaitFile(peers[0], "bob-seq.md", deadline2) {
		t.Fatalf("alice did not receive bob's file")
	}
	t.Logf("phase 2 ok — 2-peer sequential-different-files converged")
}

// TestStage4_Diag_WB28_RootCauseProbe is a debug-log-enabled variant of
// TwoPeerConcurrentDifferentFiles that surfaces subscription engine
// messages during the failure window. Narrows root cause: did the engine
// MATCH the events? Did delivery FIRE? Did blob-resolve EXECUTE?
//
// Diagnostic-only; doesn't fail. Pipe `make test-shellcmd ARGS="-run
// TestStage4_Diag_WB28_RootCauseProbe -v"` and inspect the log output
// for "subscription:" lines around the write timestamps.
func TestStage4_Diag_WB28_RootCauseProbe(t *testing.T) {
	const rootName = "sync"
	const sourcePrefix = "local/files/" + rootName + "/"

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Construct peers WITH DebugLog enabled — bypasses the harness which
	// disables logs by default. Mirrors the Stage 3 case 2 pattern but
	// with truly concurrent writes (goroutines + WaitGroup).
	dbg := log.New(os.Stderr, "", log.Lmicroseconds)
	makePeer := func(name string) *entitysdk.AppPeer {
		t.Helper()
		br := workbench.NewBlobResolveHandler()
		br.RegisterMount(sourcePrefix, sourcePrefix)
		ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
			ListenAddr: "127.0.0.1:0",
			DebugLog:   dbg,
			RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
			Handlers: []entitysdk.HandlerRegistration{
				{Pattern: workbench.BlobResolvePattern, Handler: br},
				{Pattern: workbench.ChainErrorsPattern, Handler: workbench.NewChainErrorsHandler()},
			},
		})
		if err != nil {
			t.Fatalf("CreatePeer %s: %v", name, err)
		}
		t.Cleanup(func() { _ = ap.Close() })
		bringUpListener(t, ctx, ap, name)
		return ap
	}

	alice := makePeer("alice")
	bob := makePeer("bob")
	aliceDir := t.TempDir()
	bobDir := t.TempDir()

	if _, err := alice.Connect(ctx, bob.Addr().String()); err != nil {
		t.Fatalf("alice→bob connect: %v", err)
	}
	if _, err := bob.Connect(ctx, alice.Addr().String()); err != nil {
		t.Fatalf("bob→alice connect: %v", err)
	}

	for _, p := range []struct {
		name string
		ap   *entitysdk.AppPeer
		dir  string
	}{{"alice", alice, aliceDir}, {"bob", bob, bobDir}} {
		lf := p.ap.LocalFilesHandler()
		if err := lf.AddRoot(rootName, localfiles.RootConfigData{
			Prefix: sourcePrefix, FilesystemRoot: p.dir,
		}, p.ap.RawContentStore(), p.ap.RawLocationIndex()); err != nil {
			t.Fatalf("%s AddRoot: %v", p.name, err)
		}
		if err := lf.StartWatching(ctx, rootName, p.ap.RawContentStore(),
			p.ap.RawLocationIndex(), p.ap.IdentityHash()); err != nil {
			t.Fatalf("%s StartWatching: %v", p.name, err)
		}
	}

	for _, p := range []struct {
		name   string
		ap     *entitysdk.AppPeer
		other  *entitysdk.AppPeer
		thisID string
	}{{"alice", alice, bob, alice.PeerID()}, {"bob", bob, alice, bob.PeerID()}} {
		chainGrants := []types.GrantEntry{{
			Handlers:   types.CapabilityScope{Include: []string{workbench.BlobResolvePattern}},
			Operations: types.CapabilityScope{Include: []string{"receive"}},
		}}
		if _, err := p.ap.MintChainCapabilityBound(chainGrants,
			"system/capability/grants/chain/blob-resolve/"+rootName); err != nil {
			t.Fatalf("%s mint cap: %v", p.name, err)
		}
		deliverURI := "entity://" + p.thisID + "/" + workbench.BlobResolvePattern
		if _, err := p.ap.SubscribeRawAt(p.other.PeerID(), sourcePrefix+"*",
			deliverURI, "receive", entitysdk.SubscribeOpts{
				Events: []string{"created", "updated"}, IncludePayload: true,
			}); err != nil {
			t.Fatalf("%s subscribe: %v", p.name, err)
		}
	}

	// Truly concurrent writes — both goroutines, WaitGroup barrier so
	// the writes happen as close to simultaneously as the scheduler allows.
	t.Log("=== STAGE: about to fire concurrent writes ===")
	var wg sync.WaitGroup
	wg.Add(2)
	barrier := make(chan struct{})
	go func() {
		defer wg.Done()
		<-barrier
		_ = os.WriteFile(filepath.Join(aliceDir, "alice-probe.md"), []byte("# Alice\n"), 0600)
	}()
	go func() {
		defer wg.Done()
		<-barrier
		_ = os.WriteFile(filepath.Join(bobDir, "bob-probe.md"), []byte("# Bob\n"), 0600)
	}()
	close(barrier)
	wg.Wait()
	t.Log("=== STAGE: writes fired; waiting 30s for convergence ===")

	time.Sleep(30 * time.Second)

	t.Log("=== STAGE: final state ===")
	aliceFiles, _ := os.ReadDir(aliceDir)
	bobFiles, _ := os.ReadDir(bobDir)
	aliceNames := []string{}
	for _, e := range aliceFiles {
		aliceNames = append(aliceNames, e.Name())
	}
	bobNames := []string{}
	for _, e := range bobFiles {
		bobNames = append(bobNames, e.Name())
	}
	t.Logf("alice FS: %v", aliceNames)
	t.Logf("bob FS:   %v", bobNames)
}

func TestStage4_Diag_TwoPeerOneWrite(t *testing.T) {
	const rootName = "sync"
	const sourcePrefix = "local/files/" + rootName + "/"

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	peers := stage4Setup(t, ctx, 2, []string{"alice", "bob"}, rootName, sourcePrefix)
	stage4ConnectAllToAll(t, ctx, peers)
	stage4StartWatchers(t, ctx, peers, rootName, sourcePrefix)
	stage4WireMesh(t, peers, rootName, sourcePrefix)

	// Only alice writes. Bob should receive.
	content := "# Hello\n\nAlice only.\n"
	path := filepath.Join(peers[0].fsRoot, "hello.md")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("alice write: %v", err)
	}

	deadline := time.Now().Add(30 * time.Second)
	if !stage4AwaitFile(peers[1], "hello.md", deadline) {
		stage4DumpDiagnostics(t, peers, sourcePrefix)
		t.Fatalf("bob did not receive alice's hello.md within 30s")
	}
	t.Logf("OK: harness 2-peer one-direction works")
}
