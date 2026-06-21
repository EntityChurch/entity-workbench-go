package shellcmd_test

// Stage 4 Case I — late-join mesh.
//
// Topology evolution: start with a 2-peer mesh (alice + bob), write
// files, wait for convergence. THEN introduce a 3rd peer (carol) that
// joins the mesh: carol connects to alice + bob, mounts, and subscribes
// to both. After the late-join, observe what carol sees.
//
// What this probes:
//
//  1. Late-joiner state catch-up. The subscription engine spec
//     (EXTENSION-SUBSCRIPTION) describes notifications as fired on
//     tree change events. Files written BEFORE carol's subscription
//     existed are not "changes" from carol's perspective — they're
//     pre-existing state. Does the substrate provide a mechanism for
//     carol to receive them?
//
//  2. Realistic production deployment. New machines join an existing
//     workspace cluster. They expect to see the cluster's state, not
//     just changes from this-moment-forward. If carol sees nothing,
//     the substrate requires an explicit "catch up" mechanism.
//
//  3. Whether subsequent writes after carol joins propagate normally.
//     Even if catch-up doesn't happen, new writes should still flow
//     through the full 3-peer mesh.
//
// What we expect to observe (one of these):
//   (a) Carol sees pre-existing files within deadline → substrate has
//       implicit catch-up (probably via subscription on PATTERN that
//       triggers initial scan; unlikely per spec but possible).
//   (b) Carol sees NEW files only → substrate is purely event-driven;
//       catch-up needs a separate "pull existing state" mechanism.
//   (c) Carol sees nothing → bigger finding; subscription is broken
//       at the new-peer-mid-flight point.
//
// Observation (b) is the most likely; if so, it's a documented
// production-readiness gap and a candidate for an SDK helper.

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

// fsList returns the filenames in a directory (for diagnostics).
func fsList(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{"<read-err: " + err.Error() + ">"}
	}
	out := []string{}
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func TestStage4_CaseI_LateJoin(t *testing.T) {
	const rootName = "sync"
	const sourcePrefix = "local/files/" + rootName + "/"

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Phase 1: bring up alice + bob in 2-peer mesh.
	peers := stage4Setup(t, ctx, 2, []string{"alice", "bob"}, rootName, sourcePrefix)
	stage4ConnectAllToAll(t, ctx, peers)
	stage4StartWatchers(t, ctx, peers, rootName, sourcePrefix)
	stage4WireMesh(t, peers, rootName, sourcePrefix)

	alice := peers[0]
	bob := peers[1]

	// Phase 2: alice + bob each write a file BEFORE carol joins.
	// SEQUENTIAL writes — alice first + wait, then bob + wait. This
	// works around WB-28 (2-peer concurrent-write silent failure).
	preJoinContent := []struct {
		filename string
		content  string
		author   int
	}{
		{"alice-prejoin.md", "# Alice prejoin\n\nWritten before carol joined.\n", 0},
		{"bob-prejoin.md", "# Bob prejoin\n\nWritten before carol joined.\n", 1},
	}
	for _, s := range preJoinContent {
		p := peers[s.author]
		path := filepath.Join(p.fsRoot, s.filename)
		if err := os.WriteFile(path, []byte(s.content), 0600); err != nil {
			t.Fatalf("%s write %s: %v", p.name, s.filename, err)
		}
		// Wait for cross-peer materialization before the next write.
		// Both peers must see this file before we proceed.
		stepDeadline := time.Now().Add(30 * time.Second)
		for _, op := range []stage4Peer{alice, bob} {
			if !stage4AwaitFile(op, s.filename, stepDeadline) {
				t.Fatalf("pre-join sequential: %s did not receive %s. FS: alice=%v bob=%v",
					op.name, s.filename, fsList(alice.fsRoot), fsList(bob.fsRoot))
			}
		}
	}
	t.Logf("phase 1: alice + bob converged on 2 pre-join files (sequentially written)")

	// Phase 3: spin up carol, join the mesh symmetrically.
	carolDir := t.TempDir()
	carolBR := workbench.NewBlobResolveHandler()
	carolBR.RegisterMount(sourcePrefix, sourcePrefix)
	carol, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		ListenAddr: "127.0.0.1:0",
		RawOptions: []peer.Option{peer.WithConnectionGrants(peer.OpenAccessGrants())},
		Handlers: []entitysdk.HandlerRegistration{
			{Pattern: workbench.BlobResolvePattern, Handler: carolBR},
			{Pattern: workbench.ChainErrorsPattern, Handler: workbench.NewChainErrorsHandler()},
		},
	})
	if err != nil {
		t.Fatalf("CreatePeer carol: %v", err)
	}
	t.Cleanup(func() { _ = carol.Close() })
	bringUpListener(t, ctx, carol, "carol")

	carolPeer := stage4Peer{name: "carol", ap: carol, fsRoot: carolDir, br: carolBR}

	// Carol connects to alice + bob, AND alice + bob connect to carol.
	for _, p := range []stage4Peer{alice, bob} {
		if _, err := carol.Connect(ctx, p.ap.Addr().String()); err != nil {
			t.Fatalf("carol→%s connect: %v", p.name, err)
		}
		if _, err := p.ap.Connect(ctx, carol.Addr().String()); err != nil {
			t.Fatalf("%s→carol connect: %v", p.name, err)
		}
	}

	// Carol mounts + starts watcher.
	carolLF := carol.LocalFilesHandler()
	if err := carolLF.AddRoot(rootName, localfiles.RootConfigData{
		Prefix:         sourcePrefix,
		FilesystemRoot: carolDir,
	}, carol.RawContentStore(), carol.RawLocationIndex()); err != nil {
		t.Fatalf("carol AddRoot: %v", err)
	}
	if err := carolLF.StartWatching(ctx, rootName, carol.RawContentStore(),
		carol.RawLocationIndex(), carol.IdentityHash()); err != nil {
		t.Fatalf("carol StartWatching: %v", err)
	}

	// Mint chain cap on carol. Then carol subscribes to alice + bob,
	// and alice + bob subscribe to carol — wires carol fully into the
	// mesh. (Carol can't catch up on alice + bob's pre-join files via
	// new subscriptions alone — pure subscription is event-driven; only
	// FUTURE changes will fire.)
	chainGrants := []types.GrantEntry{{
		Handlers:   types.CapabilityScope{Include: []string{workbench.BlobResolvePattern}},
		Operations: types.CapabilityScope{Include: []string{"receive"}},
	}}
	if _, err := carol.MintChainCapabilityBound(chainGrants,
		"system/capability/grants/chain/blob-resolve/"+rootName); err != nil {
		t.Fatalf("carol mint chain cap: %v", err)
	}
	for _, p := range []stage4Peer{alice, bob} {
		stage4Subscribe(t, carolPeer, p, sourcePrefix)
		stage4Subscribe(t, p, carolPeer, sourcePrefix)
	}

	t.Logf("phase 3: carol joined the mesh")

	// Phase 4: probe. Wait the watcher debounce + a settle window and
	// check: did carol receive the pre-join files via any mechanism?
	probeDeadline := time.Now().Add(20 * time.Second)
	stillMissing := []string{}
	for _, s := range preJoinContent {
		if !stage4AwaitFile(carolPeer, s.filename, probeDeadline) {
			stillMissing = append(stillMissing, s.filename)
		}
	}
	if len(stillMissing) == 0 {
		t.Logf("OBSERVATION (a): carol caught up on pre-join files — substrate has implicit catch-up")
	} else {
		t.Logf("OBSERVATION (b/c): carol does NOT have pre-join files after late-join: %v", stillMissing)
		t.Logf("(this is expected if subscriptions are event-driven only; finding documented inline)")
	}

	// Phase 5: post-join write — does NEW activity propagate through
	// the full 3-peer mesh, including carol writing?
	postJoinContent := []struct {
		filename string
		content  string
		author   int // 0=alice, 1=bob, 2=carol
	}{
		{"alice-postjoin.md", "# Alice postjoin\n", 0},
		{"bob-postjoin.md", "# Bob postjoin\n", 1},
		{"carol-postjoin.md", "# Carol postjoin\n", 2},
	}
	allPeers := []stage4Peer{alice, bob, carolPeer}
	for _, s := range postJoinContent {
		p := allPeers[s.author]
		path := filepath.Join(p.fsRoot, s.filename)
		if err := os.WriteFile(path, []byte(s.content), 0600); err != nil {
			t.Fatalf("%s write %s: %v", p.name, s.filename, err)
		}
	}

	postJoinDeadline := time.Now().Add(45 * time.Second)
	postJoinConverged := 0
	postJoinExpected := len(postJoinContent) * 3
	for _, s := range postJoinContent {
		for _, p := range allPeers {
			label := fmt.Sprintf("%s has %s", p.name, s.filename)
			if !stage4AwaitFile(p, s.filename, postJoinDeadline) {
				t.Errorf("post-join: %s — did not materialize within deadline", label)
				continue
			}
			postJoinConverged++
		}
	}
	t.Logf("phase 5: post-join converged %d/%d (peer × file) pairs",
		postJoinConverged, postJoinExpected)

	// Final state summary
	for _, p := range allPeers {
		files, _ := os.ReadDir(p.fsRoot)
		names := []string{}
		for _, f := range files {
			names = append(names, f.Name())
		}
		t.Logf("FINAL %s fs: %v", p.name, names)
	}

	// Loop-prevention probe
	time.Sleep(3 * time.Second)
	stage4AssertNoChainErrors(t, allPeers)
}
