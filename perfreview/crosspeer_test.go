//go:build perfreview

package perfreview

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"
	coretypes "go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

// TestCrossPeer_ConnectAndDispatch characterizes the cost of cross-peer
// operations. Zero baseline data existed for this before round 5.
//
// Measurements:
//  1. Connect-handshake latency (one-time per connection)
//  2. Cross-peer Get latency vs local Get
//  3. Cross-peer dispatch latency (executing a remote op)
//
// Setup: alice listens, bob connects. Both peers in-process; the
// network hop is loopback so this measures the protocol + serialization
// cost, NOT real-network round-trips.
func TestCrossPeer_ConnectAndDispatch(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	alice, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		Storage:    entitysdk.StorageConfig{Kind: "sqlite", Path: filepath.Join(dir, "alice.db")},
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("CreatePeer alice: %v", err)
	}
	defer alice.Close()

	bob, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		Storage: entitysdk.StorageConfig{Kind: "sqlite", Path: filepath.Join(dir, "bob.db")},
	})
	if err != nil {
		t.Fatalf("CreatePeer bob: %v", err)
	}
	defer bob.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ready := make(chan struct{})
	listenErr := make(chan error, 1)
	go func() { listenErr <- alice.ListenReady(ctx, ready) }()
	select {
	case <-ready:
	case err := <-listenErr:
		t.Fatalf("alice listen: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("alice listen timeout")
	}

	// --- 1) Connect-handshake latency ---
	connectStart := time.Now()
	if _, err := bob.Connect(ctx, alice.Addr().String()); err != nil {
		t.Fatalf("bob connect: %v", err)
	}
	connectDur := time.Since(connectStart)
	t.Logf("connect-handshake (loopback): %s", short(connectDur))

	// --- 2) Seed alice with some content ---
	const seed = 1_000
	for i := 0; i < seed; i++ {
		path := fmt.Sprintf("shared/%05d", i)
		if _, err := alice.Put(path, "perfreview/entity",
			map[string]interface{}{"tick": i, "time": "x"}); err != nil {
			t.Fatalf("alice Put: %v", err)
		}
	}
	t.Logf("alice seeded with %d entities", seed)

	// --- 3) Cross-peer Get latency: bob reads from alice's namespace ---
	// Bob's view of alice's path: @alicePeerID/shared/00001
	const N = 200
	localLatencies := make([]time.Duration, 0, N)
	remoteLatencies := make([]time.Duration, 0, N)

	// Local Get baseline via AppPeer.Get (goes through dispatcher,
	// same code path as cross-peer Get for apples-to-apples).
	for i := 0; i < N; i++ {
		path := fmt.Sprintf("shared/%05d", i)
		start := time.Now()
		_, ok, _ := alice.Get(path)
		if !ok {
			t.Fatalf("alice local Get %s not found", path)
		}
		localLatencies = append(localLatencies, time.Since(start))
	}

	// Cross-peer Get (bob → alice via dispatcher).
	// SDK path syntax is /<peer-id>/path (the @alias form is shell-
	// specific; see entitysdk/executor_remote_test.go for the canonical
	// pattern).
	for i := 0; i < N; i++ {
		path := fmt.Sprintf("/%s/shared/%05d", alice.PeerID(), i)
		start := time.Now()
		_, ok, err := bob.Get(path)
		if err != nil {
			t.Fatalf("bob Get %s: %v", path, err)
		}
		if !ok {
			t.Fatalf("bob Get %s not found", path)
		}
		remoteLatencies = append(remoteLatencies, time.Since(start))
	}

	sort.Slice(localLatencies, func(i, j int) bool { return localLatencies[i] < localLatencies[j] })
	sort.Slice(remoteLatencies, func(i, j int) bool { return remoteLatencies[i] < remoteLatencies[j] })

	t.Logf("local Get  (N=%d): p50=%s p95=%s p99=%s",
		N,
		short(localLatencies[len(localLatencies)*50/100]),
		short(localLatencies[len(localLatencies)*95/100]),
		short(localLatencies[len(localLatencies)*99/100]))
	t.Logf("remote Get (N=%d): p50=%s p95=%s p99=%s",
		N,
		short(remoteLatencies[len(remoteLatencies)*50/100]),
		short(remoteLatencies[len(remoteLatencies)*95/100]),
		short(remoteLatencies[len(remoteLatencies)*99/100]))

	remoteP50 := remoteLatencies[len(remoteLatencies)*50/100]
	localP50 := localLatencies[len(localLatencies)*50/100]
	if localP50 > 0 {
		t.Logf("cross-peer overhead ratio: %.1fx (p50)", float64(remoteP50)/float64(localP50))
	}
}

// TestCrossPeer_RevisionSyncCost measures the wall time to sync a
// modest corpus between two peers. Combines:
//   - alice puts N entities + commits a revision
//   - bob fetches the revision + tree-merges it
//
// Production case: a follower peer catching up after going offline.
func TestCrossPeer_RevisionSyncCost(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	alice, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		Storage:    entitysdk.StorageConfig{Kind: "sqlite", Path: filepath.Join(dir, "alice.db")},
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	})
	if err != nil {
		t.Fatalf("CreatePeer alice: %v", err)
	}
	defer alice.Close()

	bob, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		Storage: entitysdk.StorageConfig{Kind: "sqlite", Path: filepath.Join(dir, "bob.db")},
	})
	if err != nil {
		t.Fatalf("CreatePeer bob: %v", err)
	}
	defer bob.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ready := make(chan struct{})
	go func() { _ = alice.ListenReady(ctx, ready) }()
	<-ready

	if _, err := bob.Connect(ctx, alice.Addr().String()); err != nil {
		t.Fatalf("bob connect: %v", err)
	}

	// Alice seeds + commits.
	const N = 5_000
	seedStart := time.Now()
	for i := 0; i < N; i++ {
		path := fmt.Sprintf("shared/%05d", i)
		if _, err := alice.Store().Put(path, "perfreview/entity",
			map[string]interface{}{"tick": i, "time": "x"}); err != nil {
			t.Fatalf("alice Put: %v", err)
		}
	}
	seedDur := time.Since(seedStart)

	commitStart := time.Now()
	aliceCommit, err := alice.Revision().Commit(ctx, "shared/", "initial")
	if err != nil {
		t.Fatalf("alice commit: %v", err)
	}
	commitDur := time.Since(commitStart)

	t.Logf("alice: seed %d entities in %s, commit in %s (head=%s)",
		N, short(seedDur), short(commitDur), aliceCommit.Version.String()[:12])

	// Bob fetches alice's head + diff.
	syncStart := time.Now()
	if _, err := bob.RevisionAt(alice.PeerID()).Fetch(ctx, coretypes.RevisionFetchParamsData{Prefix: "shared/"}); err != nil {
		t.Fatalf("bob fetch: %v", err)
	}
	fetchDur := time.Since(syncStart)
	t.Logf("bob fetch alice's revision: %s", short(fetchDur))
}
