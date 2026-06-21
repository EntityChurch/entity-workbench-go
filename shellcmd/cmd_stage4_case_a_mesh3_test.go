package shellcmd_test

// Stage 4 Case A — 3-peer symmetric mesh.
//
// Topology: peers {alice, bob, carol}. Every peer subscribes to every
// other peer's local/files/sync/* prefix. Every peer writes one distinct
// file. Expected: all 3 files materialize on all 3 peers' filesystems,
// with no chain-error markers and bounded entity count under the prefix
// (one binding per file per peer).
//
// What this stresses beyond Stage 3 case 2 (2-peer bidirectional):
//
//  1. Subscription engine fan-out at N=3. Each peer is publisher to 2 and
//     subscriber to 2 (6 active subscription edges, 3 watchers).
//  2. Loop prevention under multi-peer cross-fertilization. When carol
//     materializes alice's file, carol's watcher fires; bob is subscribed
//     to carol's prefix and could re-fetch the same blob alice produced.
//     The idempotency check in blob-resolve (workbench/blob_resolve.go,
//     F9 fix 84269c4) must hold across this triangular path.
//  3. Cap discipline across peer triplet. Each peer mints its own chain
//     cap; cross-peer subscription delivery authorizes against V7 §5.10.
//     Test uses OpenAccessGrants so caps aren't deeply exercised, but
//     three distinct caps must coexist.
//
// What this does NOT yet test (Case B/C/D territory):
//   - Higher fan-out (5+ peers; WB-21 saturation territory).
//   - Asymmetric publish patterns (hub-and-spoke).
//   - Transitive cap delegation (cascade).
//   - Concurrent same-file edits (last-writer-wins).

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStage4_CaseA_Mesh3(t *testing.T) {
	const rootName = "sync"
	const sourcePrefix = "local/files/" + rootName + "/"

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	peers := stage4Setup(t, ctx, 3, []string{"alice", "bob", "carol"}, rootName, sourcePrefix)
	stage4ConnectAllToAll(t, ctx, peers)
	stage4StartWatchers(t, ctx, peers, rootName, sourcePrefix)
	stage4WireMesh(t, peers, rootName, sourcePrefix)

	// Each peer writes one distinct file. Filenames encode the author so
	// we can verify provenance after convergence (every peer should have
	// all 3 files even though it only wrote one).
	type fileSpec struct {
		filename string
		content  string
		author   int // index in peers
	}
	specs := []fileSpec{
		{"alice-file.md", "# Alice\n\nWritten by alice; expect on bob + carol.\n", 0},
		{"bob-file.md", "# Bob\n\nWritten by bob; expect on alice + carol.\n", 1},
		{"carol-file.md", "# Carol\n\nWritten by carol; expect on alice + bob.\n", 2},
	}

	for _, s := range specs {
		p := peers[s.author]
		path := filepath.Join(p.fsRoot, s.filename)
		if err := os.WriteFile(path, []byte(s.content), 0600); err != nil {
			t.Fatalf("%s write %s: %v", p.name, s.filename, err)
		}
	}

	// Each file must land on every peer. Wall-time budget: 30s per
	// convergence target (mesh delivery should be sub-second under
	// nominal load; budget absorbs watcher debounce + race detector).
	startConvergence := time.Now()
	deadline := time.Now().Add(45 * time.Second)
	convergedCount := 0

	for _, s := range specs {
		for i := range peers {
			p := peers[i]
			label := fmt.Sprintf("%s has %s", p.name, s.filename)
			if !stage4AwaitFile(p, s.filename, deadline) {
				t.Errorf("%s — did not materialize within deadline", label)
				stage4DumpDiagnostics(t, peers, sourcePrefix)
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
			t.Logf("%s ✓ (%d bytes)", label, len(got))
			convergedCount++
		}
	}
	convergenceLatency := time.Since(startConvergence)
	t.Logf("converged %d/%d (peer × file) pairs in %s", convergedCount, len(specs)*len(peers), convergenceLatency)

	// Loop-prevention probe: wait an extra settle window, then assert
	// bounded entity count + zero chain-error markers across all peers.
	// Each peer should have exactly 3 file bindings under sourcePrefix
	// (one per file across the mesh). >3 would indicate runaway rebinding.
	time.Sleep(3 * time.Second)
	stage4AssertEntityCountBound(t, peers, sourcePrefix, 3)
	stage4AssertNoChainErrors(t, peers)
}
