package shellcmd_test

// Stage 4 Case B — 5-peer symmetric mesh.
//
// Scales Case A (3-peer mesh) to 5 peers: 5 × 4 = 20 directed subscription
// edges, 5 active watchers. Each peer writes one distinct file; all 5 files
// must converge on all 5 peers (25 peer×file pairs total).
//
// What this stresses beyond Case A:
//
//  1. Subscription engine fan-out at higher cardinality. Each Put on any
//     peer fires 4 cross-peer notifications. With 5 simultaneous writes
//     that's 20 cross-peer deliveries in the burst — still well under the
//     ~2K notifs/sec saturation cliff (WB-21), but a 6× larger pulse than
//     Case A's 6-edge mesh.
//  2. Triangle-loop frequency goes up. In Case A there are 3 distinct
//     3-peer cycles (A→B→C→A and rotations) where idempotency must hold;
//     at N=5 there are C(5,3)=10 distinct triangles. The blob-resolve
//     idempotency short-circuit is exercised more aggressively.
//  3. Entity-count bound tighter. With 5 files and 5 peers, total
//     populated mesh = 25 bindings; runaway re-binding would be obvious.
//  4. Convergence latency baseline at moderate fan-out. Establishes a
//     per-pair-convergence ceiling for comparison with hub-and-spoke
//     (Case C, which has the same write load but fewer edges).
//
// Wall-time budget: 60s for convergence. Case A converged 9 pairs in
// 17.15s under race detector (~1.9s/pair); Case B has 25 pairs so
// proportional ceiling is ~48s; budget rounds up.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStage4_CaseB_Mesh5(t *testing.T) {
	const rootName = "sync"
	const sourcePrefix = "local/files/" + rootName + "/"

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	names := []string{"p0", "p1", "p2", "p3", "p4"}
	peers := stage4Setup(t, ctx, 5, names, rootName, sourcePrefix)
	stage4ConnectAllToAll(t, ctx, peers)
	stage4StartWatchers(t, ctx, peers, rootName, sourcePrefix)
	stage4WireMesh(t, peers, rootName, sourcePrefix)

	// Each peer writes one distinct file (filename encodes author index
	// so post-convergence assertions can verify per-file provenance).
	type fileSpec struct {
		filename string
		content  string
		author   int
	}
	specs := make([]fileSpec, len(peers))
	for i := range peers {
		specs[i] = fileSpec{
			filename: fmt.Sprintf("%s-file.md", peers[i].name),
			content:  fmt.Sprintf("# %s\n\nAuthored by %s; expect on all peers.\n", peers[i].name, peers[i].name),
			author:   i,
		}
	}

	for _, s := range specs {
		p := peers[s.author]
		path := filepath.Join(p.fsRoot, s.filename)
		if err := os.WriteFile(path, []byte(s.content), 0600); err != nil {
			t.Fatalf("%s write %s: %v", p.name, s.filename, err)
		}
	}

	startConvergence := time.Now()
	deadline := time.Now().Add(90 * time.Second)
	convergedCount := 0
	expectedCount := len(specs) * len(peers)

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
				t.Errorf("%s content mismatch:\n  got:  %q\n  want: %q",
					label, string(got), s.content)
				continue
			}
			convergedCount++
		}
	}
	convergenceLatency := time.Since(startConvergence)
	t.Logf("converged %d/%d (peer × file) pairs in %s (%.2fs/pair avg)",
		convergedCount, expectedCount, convergenceLatency,
		convergenceLatency.Seconds()/float64(expectedCount))

	if convergedCount < expectedCount {
		stage4DumpDiagnostics(t, peers, sourcePrefix)
	}

	// Settle + bounded check. Each peer should have exactly len(peers)
	// bindings; runaway loops would explode this.
	time.Sleep(3 * time.Second)
	stage4AssertEntityCountBound(t, peers, sourcePrefix, len(peers))
	stage4AssertNoChainErrors(t, peers)
}
