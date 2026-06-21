package shellcmd_test

// Stage 4 Case D — cascade A→B→C.
//
// Topology: peers[0] (head) → peers[1] (middle) → peers[2] (tail). Only
// peers[1] subscribes to peers[0]; only peers[2] subscribes to peers[1].
// No reverse subscriptions, no fan-out. Single linear pipeline.
//
// What this stresses (qualitatively different from mesh + hub-spoke):
//
//  1. Transitive materialization. When A writes, B receives + materializes
//     to disk. B's watcher fires on that disk write, rebinding the same
//     content under B's tree (with B as the writer). C then receives via
//     its B-subscription and materializes locally. The cascade exercises
//     the "B re-publishes after materializing" path that's invisible in
//     hub-and-spoke (spokes don't re-publish) and statistically averaged
//     out in mesh (every peer is both publisher and subscriber).
//
//  2. Content-address transparency through the chain. C's blob-resolve
//     does content.EnsureClosure against B (its subscription source) for
//     the blob hash B re-published. The chunks originated on A and were
//     content-addressed; B now has them by virtue of having received from
//     A. So B→C transfer is satisfied without ever consulting A. This is
//     a property of content addressing, not the chain primitive — but
//     cascade is where it gets exercised end-to-end.
//
//  3. Reverse-write boundary. Each middle peer's localfiles handler writes
//     a file to disk (downstream of receiving from upstream). The
//     watcher's reverseTracker (F9 fix, core-go 8ad52bc) must mark that
//     write as reverse-originated so it doesn't loop back upstream. In
//     mesh, this matters but is intertwined with N-way fan-out; in
//     cascade, it's the dominant correctness mechanism.
//
//  4. Cap-chain provenance under transitive delivery. NOTE: with
//     OpenAccessGrants, cap discipline is not exercised here. A future
//     restricted-cap variant would verify that B's chain cap to C is
//     independent of A's chain cap to B (each pair has its own cap; no
//     A-rooted cap is required for C's reception). That variant is owed
//     but parked — the OpenAccessGrants version validates the wiring
//     shape end-to-end.
//
// Expected: A writes; B materializes; C materializes. C never echoes
// back to B; B never echoes back to A. Each peer ends with exactly
// numFiles bindings under sourcePrefix.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStage4_CaseD_Cascade(t *testing.T) {
	const rootName = "sync"
	const sourcePrefix = "local/files/" + rootName + "/"

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	names := []string{"head", "middle", "tail"}
	peers := stage4Setup(t, ctx, 3, names, rootName, sourcePrefix)
	stage4ConnectAllToAll(t, ctx, peers)
	stage4StartWatchers(t, ctx, peers, rootName, sourcePrefix)
	stage4WireCascade(t, peers, rootName, sourcePrefix)

	// Head writes K files. Middle + tail should pick them up through
	// the cascade.
	const numFiles = 3
	type fileSpec struct {
		filename string
		content  string
	}
	specs := make([]fileSpec, numFiles)
	for i := 0; i < numFiles; i++ {
		specs[i] = fileSpec{
			filename: fmt.Sprintf("file-%d.md", i),
			content:  fmt.Sprintf("# File %d\n\nAuthored by head; expect cascading to middle then tail.\n", i),
		}
	}

	head := peers[0]
	for _, s := range specs {
		path := filepath.Join(head.fsRoot, s.filename)
		if err := os.WriteFile(path, []byte(s.content), 0600); err != nil {
			t.Fatalf("head write %s: %v", s.filename, err)
		}
	}

	// Cascade convergence: tail receives last (it depends on middle
	// having materialized + re-published). Convergence time should be
	// roughly 2× single-hop latency.
	startConvergence := time.Now()
	deadline := time.Now().Add(60 * time.Second)
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
	t.Logf("cascade converged %d/%d (peer × file) pairs in %s",
		convergedCount, expectedCount, time.Since(startConvergence))

	if convergedCount < expectedCount {
		stage4DumpDiagnostics(t, peers, sourcePrefix)
	}

	// Loop-prevention probe + bounded count.
	time.Sleep(3 * time.Second)
	stage4AssertEntityCountBound(t, peers, sourcePrefix, numFiles)
	stage4AssertNoChainErrors(t, peers)
}
