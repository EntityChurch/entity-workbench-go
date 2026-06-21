package shellcmd_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"
	"go.entitychurch.org/entity-core-go/ext/localfiles"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"
)

// TestStage3_Case3_BurstWriter validates the substrate under
// burst-write conditions: alice writes N files in rapid succession
// (parallel goroutines), bob receives all N via the same subscription
// chain. Tests the handler's concurrency story + the subscription
// engine's notification fan-out.
//
// What this validates beyond case 1.5:
//   - Each notification is independent: a burst of N writes produces
//     N independent subscription deliveries, each triggering its own
//     blob-resolve invocation.
//   - The §7.2 chunk-fetch path is reentrant: multiple in-flight
//     EnsureClosure calls don't trip over each other's content-store
//     writes (idempotent by content hash).
//   - The atomic-write-to-disk path is safe under concurrent
//     materialize dispatches: each file lands at its own target path
//     so there's no path-level contention; the test exercises this.
//
// What this does NOT test (case 4 territory):
//   - Concurrent writes to the SAME path (would test last-writer-wins).
func TestStage3_Case3_BurstWriter(t *testing.T) {
	const burstSize = 20
	const fileBytes = 16 * 1024 // 16 KiB — small enough to keep chunked-eligibility predictable

	aliceDir := t.TempDir()
	bobDir := t.TempDir()
	const rootName = "sync"
	const sourcePrefix = "local/files/" + rootName + "/"

	bobBR := workbench.NewBlobResolveHandler()
	bobBR.RegisterMount(sourcePrefix, sourcePrefix)

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
			{Pattern: workbench.BlobResolvePattern, Handler: bobBR},
			{Pattern: workbench.ChainErrorsPattern, Handler: workbench.NewChainErrorsHandler()},
		},
	})
	if err != nil {
		t.Fatalf("CreatePeer bob: %v", err)
	}
	t.Cleanup(func() { _ = bob.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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

	aliceLF := alice.LocalFilesHandler()
	if err := aliceLF.AddRoot(rootName, localfiles.RootConfigData{
		Prefix: sourcePrefix, FilesystemRoot: aliceDir,
	}, alice.RawContentStore(), alice.RawLocationIndex()); err != nil {
		t.Fatalf("alice AddRoot: %v", err)
	}
	if err := aliceLF.StartWatching(ctx, rootName, alice.RawContentStore(),
		alice.RawLocationIndex(), alice.IdentityHash()); err != nil {
		t.Fatalf("alice StartWatching: %v", err)
	}

	bobLF := bob.LocalFilesHandler()
	if err := bobLF.AddRoot(rootName, localfiles.RootConfigData{
		Prefix: sourcePrefix, FilesystemRoot: bobDir,
	}, bob.RawContentStore(), bob.RawLocationIndex()); err != nil {
		t.Fatalf("bob AddRoot: %v", err)
	}
	// Sink only — no bob watcher (case 1.5 convention).

	deliverURI := fmt.Sprintf("entity://%s/%s", bobID, workbench.BlobResolvePattern)
	if _, err := bob.SubscribeRawAt(aliceID, sourcePrefix+"*", deliverURI, "receive",
		entitysdk.SubscribeOpts{
			Events:         []string{"created", "updated"},
			IncludePayload: true,
		}); err != nil {
		t.Fatalf("bob subscribe: %v", err)
	}

	// Burst write: N files in parallel from goroutines. Each file
	// has unique content so each chunks to a unique blob hash.
	payloads := make([][]byte, burstSize)
	names := make([]string, burstSize)
	for i := 0; i < burstSize; i++ {
		payloads[i] = makeProbePayload(int64(i+1000), fileBytes)
		names[i] = fmt.Sprintf("burst-%02d.bin", i)
	}

	burstStart := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < burstSize; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			p := filepath.Join(aliceDir, names[idx])
			if err := os.WriteFile(p, payloads[idx], 0600); err != nil {
				t.Errorf("burst write %s: %v", names[idx], err)
			}
		}(i)
	}
	wg.Wait()
	writeFanoutWall := time.Since(burstStart)

	// Wait for all N files to materialize on bob. Generous budget:
	// debounce + per-file chain overhead × burstSize is the worst
	// case (~2 s + 20 × ~50 ms = ~3 s expected).
	deadline := time.Now().Add(45 * time.Second)
	pending := make(map[string]bool, burstSize)
	for _, n := range names {
		pending[n] = true
	}
	convergenceStart := time.Now()
	for len(pending) > 0 && time.Now().Before(deadline) {
		for n := range pending {
			if _, err := os.Stat(filepath.Join(bobDir, n)); err == nil {
				delete(pending, n)
			}
		}
		if len(pending) == 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	convergenceWall := time.Since(convergenceStart)

	if len(pending) > 0 {
		errPaths := listPrefix(bob, "system/runtime/chain-errors/")
		t.Logf("bob chain-error markers: %d", len(errPaths))
		for _, p := range errPaths {
			t.Logf("  %s", p)
		}
		missing := make([]string, 0, len(pending))
		for n := range pending {
			missing = append(missing, n)
		}
		t.Fatalf("%d of %d burst files did not materialize on bob within deadline: %v",
			len(pending), burstSize, missing)
	}

	// Verify byte content for each file.
	mismatch := 0
	for i, n := range names {
		got, err := os.ReadFile(filepath.Join(bobDir, n))
		if err != nil {
			t.Errorf("read %s: %v", n, err)
			continue
		}
		if len(got) != len(payloads[i]) {
			t.Errorf("%s size mismatch: got %d want %d", n, len(got), len(payloads[i]))
			mismatch++
			continue
		}
		for j := range got {
			if got[j] != payloads[i][j] {
				mismatch++
				break
			}
		}
	}
	if mismatch > 0 {
		t.Errorf("%d of %d files had content mismatch", mismatch, burstSize)
	}

	t.Logf("=== Case 3 burst writer ===")
	t.Logf("  burst size:           %d files × %d bytes", burstSize, fileBytes)
	t.Logf("  alice write fan-out:  %s", writeFanoutWall.Round(time.Millisecond))
	t.Logf("  bob convergence:      %s", convergenceWall.Round(time.Millisecond))
	t.Logf("  per-file convergence: %s", (convergenceWall / burstSize).Round(time.Millisecond))
}
