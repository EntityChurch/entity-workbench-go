package shellcmd_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/peer"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
)

// TestE2E_Bidirectional_BurstWrites_NoFS isolates whether the burst
// convergence bug is in the cross-peer merge cascade or somewhere in
// the filesystem/ingest stack. Identical convergence-checking shape
// to TestE2E_Bidirectional_BurstWrites but writes go via AppPeer.Put
// directly to archives/notes/ — no fsnotify, no localfiles, no
// workbench ingest handler. Just: two peers, follow chain in both
// directions, each peer writes 5 entities concurrently via Put,
// expect 10/10 entries to converge on both sides.
//
// If this test PASSES: the bug is somewhere in fsnotify / localfiles
// / ingest-from-notification, not in cross-peer merge.
//
// If this test FAILS the same way: the bug is in cross-peer merge
// itself, and the fastForward/checkout fixes were necessary but not
// sufficient.
func TestE2E_Bidirectional_BurstWrites_NoFS(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const targetPrefix = "archives/notes/"
	const burst = 5

	a, b := bringUpNoFSPair(t, ctx, targetPrefix)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < burst; i++ {
			path := fmt.Sprintf("%sa-%d.md", targetPrefix, i)
			if _, err := a.ap.Put(path, "doc/markdown-file", map[string]interface{}{
				"path":    path,
				"title":   fmt.Sprintf("a %d", i),
				"content": fmt.Sprintf("# a %d\n", i),
			}); err != nil {
				t.Errorf("alice put a-%d: %v", i, err)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < burst; i++ {
			path := fmt.Sprintf("%sb-%d.md", targetPrefix, i)
			if _, err := b.ap.Put(path, "doc/markdown-file", map[string]interface{}{
				"path":    path,
				"title":   fmt.Sprintf("b %d", i),
				"content": fmt.Sprintf("# b %d\n", i),
			}); err != nil {
				t.Errorf("bob put b-%d: %v", i, err)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	wg.Wait()

	expectedPaths := make([]string, 0, burst*2)
	for i := 0; i < burst; i++ {
		expectedPaths = append(expectedPaths,
			fmt.Sprintf("%sa-%d.md", targetPrefix, i),
			fmt.Sprintf("%sb-%d.md", targetPrefix, i))
	}

	allConverged := func() bool {
		for _, p := range expectedPaths {
			if !a.ap.Store().Has(p) || !b.ap.Store().Has(p) {
				return false
			}
		}
		return true
	}
	headConverged := waitFor(20*time.Second, func() bool {
		aH, _ := a.ap.Revision().Status(ctx, targetPrefix)
		bH, _ := b.ap.Revision().Status(ctx, targetPrefix)
		return !aH.Head.IsZero() && aH.Head == bH.Head && allConverged()
	})
	if !headConverged {
		aHead, _ := a.ap.Revision().Status(ctx, targetPrefix)
		bHead, _ := b.ap.Revision().Status(ctx, targetPrefix)
		missing := make([]string, 0)
		for _, p := range expectedPaths {
			if !a.ap.Store().Has(p) {
				missing = append(missing, "alice missing "+p)
			}
			if !b.ap.Store().Has(p) {
				missing = append(missing, "bob missing "+p)
			}
		}
		t.Logf("alice has %d / %d expected entries; bob has %d / %d",
			len(a.ap.Store().List(targetPrefix)), len(expectedPaths),
			len(b.ap.Store().List(targetPrefix)), len(expectedPaths))
		if len(missing) > 0 {
			t.Logf("missing %d:", len(missing))
			for _, m := range missing {
				t.Logf("  %s", m)
			}
		}
		t.Fatalf("CONVERGENCE FAILED (heads_equal=%v) — alice=%s bob=%s",
			aHead.Head == bHead.Head, aHead.Head, bHead.Head)
	}

	if !allConverged() {
		missing := make([]string, 0)
		for _, p := range expectedPaths {
			if !a.ap.Store().Has(p) {
				missing = append(missing, "alice missing "+p)
			}
			if !b.ap.Store().Has(p) {
				missing = append(missing, "bob missing "+p)
			}
		}
		aHead, _ := a.ap.Revision().Status(ctx, targetPrefix)
		bHead, _ := b.ap.Revision().Status(ctx, targetPrefix)
		t.Logf("HEAD CONVERGED to %s but DATA INCOMPLETE", aHead.Head)
		t.Logf("alice has %d / %d expected entries; bob has %d / %d",
			len(a.ap.Store().List(targetPrefix)), len(expectedPaths),
			len(b.ap.Store().List(targetPrefix)), len(expectedPaths))
		t.Logf("missing %d:", len(missing))
		for _, m := range missing {
			t.Logf("  %s", m)
		}
		dumpRevisionDAG(t, "alice", a.ap, aHead.Head, targetPrefix)
		dumpRevisionDAG(t, "bob  ", b.ap, bHead.Head, targetPrefix)
		t.Fatalf("burst writes lost data during merge (no-FS path)")
	}
	t.Logf("burst OK: %d entries converged on both peers (no-FS path)", burst*2)
}

// bringUpNoFSPair stands up two peers configured for revision sync
// on targetPrefix, but with the localfiles extension DISABLED (no
// watcher, no ingest handler wired). Just the revision/converge
// plumbing.
func bringUpNoFSPair(t *testing.T, ctx context.Context, targetPrefix string) (*bidiPeer, *bidiPeer) {
	t.Helper()
	a := newNoFSPeer(t, "alice")
	b := newNoFSPeer(t, "bob")
	t.Cleanup(func() { _ = a.ap.Close() })
	t.Cleanup(func() { _ = b.ap.Close() })

	for _, p := range []*bidiPeer{a, b} {
		ready := make(chan struct{})
		errCh := make(chan error, 1)
		go func(ap *entitysdk.AppPeer) {
			errCh <- ap.ListenReady(ctx, ready)
		}(p.ap)
		select {
		case <-ready:
		case err := <-errCh:
			t.Fatalf("%s listen: %v", p.rootName, err)
		case <-time.After(2 * time.Second):
			t.Fatalf("%s listen timeout", p.rootName)
		}
	}
	if _, err := b.ap.Connect(ctx, a.ap.Addr().String()); err != nil {
		t.Fatalf("bob→alice connect: %v", err)
	}
	if _, err := a.ap.Connect(ctx, b.ap.Addr().String()); err != nil {
		t.Fatalf("alice→bob connect: %v", err)
	}

	autoTrue := true
	for _, p := range []*bidiPeer{a, b} {
		if _, err := p.ap.Revision().ConfigPut(ctx, "notes", types.RevisionConfigData{
			Prefix:      targetPrefix,
			AutoVersion: &autoTrue,
		}, nil); err != nil {
			t.Fatalf("%s auto-version config: %v", p.rootName, err)
		}
	}
	a.installFollow(t, b, targetPrefix)
	b.installFollow(t, a, targetPrefix)
	return a, b
}

func newNoFSPeer(t *testing.T, rootName string) *bidiPeer {
	t.Helper()
	logger := log.New(os.Stderr, "["+rootName+"] ", 0)
	cfg := entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		DebugLog:   logger,
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
	}
	ap, err := entitysdk.CreatePeer(cfg)
	if err != nil {
		t.Fatalf("%s create: %v", rootName, err)
	}
	return &bidiPeer{
		ap:       ap,
		rootName: rootName,
		id:       ap.PeerID(),
	}
}
