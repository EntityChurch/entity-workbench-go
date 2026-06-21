package shellcmd_test

// Stage 4 Case E — ring topology A→B→C→A.
//
// Topology: closed-cycle 3-peer ring. peers[0] (A) → peers[1] (B) →
// peers[2] (C) → peers[0] (A). Each peer subscribes to its predecessor.
// Distinct from Case D (cascade, open A→B→C) because the back-edge
// closes the cycle: when A writes, B materializes, then B re-publishes,
// then C materializes, then C re-publishes back TO A. The F9
// idempotency check must terminate the loop at A's blob-resolve handler
// (A already has the file content; short-circuit).
//
// What this stresses (qualitatively):
//
//  1. Cycle-completion loop prevention. The hardest topology for
//     loop-prevention: every published file traverses every peer in
//     order and returns to the origin. Without idempotency, this is
//     infinite. With idempotency (F9 fix), the origin must observe
//     same-hash-same-path and reject the rebind. This is the most
//     direct closed-graph test of F9.
//
//  2. Authentic continuation chain composition. Each delivery hop is
//     a Stage 3 chain in its own right (subscribe → blob-resolve →
//     local/files:write); 3 chains compose into the ring. No special
//     ring primitive — emerges from the substrate.
//
//  3. Convergence latency under linear hop-by-hop pipeline. Expected:
//     wall-clock ≈ (N-1) × per-hop-debounce + small constants. Compare
//     against mesh (Case A) which would deliver in parallel.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStage4_CaseE_Ring(t *testing.T) {
	const rootName = "sync"
	const sourcePrefix = "local/files/" + rootName + "/"

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	names := []string{"A", "B", "C"}
	peers := stage4Setup(t, ctx, 3, names, rootName, sourcePrefix)
	stage4ConnectAllToAll(t, ctx, peers)
	stage4StartWatchers(t, ctx, peers, rootName, sourcePrefix)
	stage4WireRing(t, peers, rootName, sourcePrefix)

	// Only A writes. The file must traverse A→B→C and then loop back
	// to A via the closed cycle. A must idempotently reject the
	// loopback delivery (it already has the content).
	const numFiles = 2
	type fileSpec struct {
		filename string
		content  string
	}
	specs := make([]fileSpec, numFiles)
	for i := 0; i < numFiles; i++ {
		specs[i] = fileSpec{
			filename: fmt.Sprintf("ring-file-%d.md", i),
			content:  fmt.Sprintf("# Ring File %d\n\nWritten on A; expected to circumnavigate to B and C.\n", i),
		}
	}

	A := peers[0]
	for _, s := range specs {
		path := filepath.Join(A.fsRoot, s.filename)
		if err := os.WriteFile(path, []byte(s.content), 0600); err != nil {
			t.Fatalf("A write %s: %v", s.filename, err)
		}
	}

	startConvergence := time.Now()
	deadline := time.Now().Add(90 * time.Second)
	convergedCount := 0
	expectedCount := numFiles * 3

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
	t.Logf("ring converged %d/%d (peer × file) pairs in %s",
		convergedCount, expectedCount, time.Since(startConvergence))

	if convergedCount < expectedCount {
		stage4DumpDiagnostics(t, peers, sourcePrefix)
	}

	// Loop-prevention check is the WHOLE POINT of this case. Wait an
	// extended settle window then verify entity-count is bounded and
	// no chain-error markers accumulated.
	time.Sleep(5 * time.Second)
	stage4AssertEntityCountBound(t, peers, sourcePrefix, numFiles)
	stage4AssertNoChainErrors(t, peers)
}
