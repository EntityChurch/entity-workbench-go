package shellcmd_test

// Stage 4 Case C — hub-and-spoke fan-out.
//
// Topology: peers[0] = hub (publisher), peers[1..N-1] = spokes. Each
// spoke subscribes to the hub's local/files/sync/* prefix. The hub
// does NOT subscribe to any spoke (asymmetric — pure fan-out).
//
// What this stresses:
//
//  1. Pure subscription engine fan-out cost. A single write on the hub
//     produces N-1 simultaneous cross-peer notifications, all from a
//     single source's subscription engine. Different shape than Case
//     A/B (mesh) where notifications come from many sources at once.
//     This is the "broadcast" workload — one writer, many readers.
//  2. No loop-prevention question. Spokes don't re-publish to the hub,
//     so the F9 / F12 idempotency code path is exercised once per file
//     per spoke and that's it. Cleaner failure isolation.
//  3. Establishes a fan-out baseline for comparison with mesh: same
//     write count, but mesh fires N×(N-1) edges versus hub-and-spoke's
//     (N-1) edges. Latency difference measures the per-source engine
//     amortization.
//
// Expected (hub writes K files): each spoke ends up with K files; hub
// keeps its K files. Hub never receives anything; spoke filesystems
// only contain hub-authored files.
//
// What this does NOT test (deferred):
//   - Higher write rates approaching WB-21 saturation cliff (~2K/sec)
//     — that's a separate dedicated burst test.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStage4_CaseC_HubSpoke(t *testing.T) {
	const rootName = "sync"
	const sourcePrefix = "local/files/" + rootName + "/"
	const numPeers = 5 // 1 hub + 4 spokes
	const hubIdx = 0

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	names := []string{"hub", "spoke1", "spoke2", "spoke3", "spoke4"}
	peers := stage4Setup(t, ctx, numPeers, names, rootName, sourcePrefix)
	stage4ConnectAllToAll(t, ctx, peers)
	stage4StartWatchers(t, ctx, peers, rootName, sourcePrefix)
	stage4WireHubSpoke(t, peers, hubIdx, rootName, sourcePrefix)

	// Hub writes K files in rapid succession. K chosen modestly so we
	// stay well below the saturation cliff and isolate fan-out behavior
	// from queue-pressure behavior.
	const numFiles = 4
	type fileSpec struct {
		filename string
		content  string
	}
	specs := make([]fileSpec, numFiles)
	for i := 0; i < numFiles; i++ {
		specs[i] = fileSpec{
			filename: fmt.Sprintf("file-%d.md", i),
			content:  fmt.Sprintf("# File %d\n\nAuthored by hub; expect on all spokes.\n", i),
		}
	}

	hub := peers[hubIdx]
	startWrite := time.Now()
	for _, s := range specs {
		path := filepath.Join(hub.fsRoot, s.filename)
		if err := os.WriteFile(path, []byte(s.content), 0600); err != nil {
			t.Fatalf("hub write %s: %v", s.filename, err)
		}
	}
	t.Logf("hub wrote %d files in %s", numFiles, time.Since(startWrite))

	// Each spoke must receive every hub-authored file. Hub's own dir
	// already has them (written there directly). Total expected
	// convergence: numFiles × numPeers (hub + spokes all have all files).
	startConvergence := time.Now()
	deadline := time.Now().Add(60 * time.Second)
	convergedCount := 0
	expectedCount := numFiles * numPeers

	for _, s := range specs {
		for i := range peers {
			p := peers[i]
			label := fmt.Sprintf("%s has %s", p.name, s.filename)
			if !stage4AwaitFile(p, s.filename, deadline) {
				t.Errorf("%s — did not materialize within deadline", label)
				continue
			}
			target := filepath.Join(p.fsRoot, s.filename)
			got, err := os.ReadFile(target)
			if err != nil {
				t.Errorf("%s read: %v", label, err)
				continue
			}
			if string(got) != s.content {
				t.Errorf("%s content mismatch", label)
				continue
			}
			convergedCount++
		}
	}
	convergenceLatency := time.Since(startConvergence)
	t.Logf("converged %d/%d (peer × file) pairs in %s (%.3fs/pair avg)",
		convergedCount, expectedCount, convergenceLatency,
		convergenceLatency.Seconds()/float64(expectedCount))

	if convergedCount < expectedCount {
		stage4DumpDiagnostics(t, peers, sourcePrefix)
	}

	// Hub-and-spoke asymmetry: spokes' tree should ONLY contain hub-
	// authored files (no echo-back). Hub's own tree should have just
	// its writes. Each peer expects exactly numFiles bindings.
	time.Sleep(3 * time.Second)
	stage4AssertEntityCountBound(t, peers, sourcePrefix, numFiles)
	stage4AssertNoChainErrors(t, peers)
}
