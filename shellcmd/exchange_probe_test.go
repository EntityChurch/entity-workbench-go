package shellcmd

// Two measurements the specification seat asked for, both about the
// enumerating fallback a live source forces.
//
// # 1. A bounded walk that does not resume (the C15 prediction, now run)
//
// `walkRemoteFiles` restarts from the base prefix on every pass and stops
// at `backfillWalkLimit`. The traversal is deterministic, so the claim was
// that a folder over the cap converges to a FIXED, permanently incomplete
// prefix — truncation honestly reported, loop making no progress. It was
// routed as a source reading and marked unperformed. This performs it.
//
// # 2. Subtree-hash by reconstruction (`EXTENSION-TREE` §3.7.1)
//
// Our live-peer problem: neither peer in a LAN share publishes a root, so
// the cheap currency check is unavailable and the backfill enumerates the
// remote prefix every pass. Arch's answer is §3.7.1 — build a fresh trie
// over the bindings under the prefix and compare its root, which needs no
// publication act and no stored sidecar. The spec claims
// "O(|S'| log |S'|) — microseconds to low milliseconds" at our sizes.
//
// **Measured 2026-09-11 and the claim does not hold**, best of 5, the only
// trie builder that exists (core-go `tree.BuildTrie`, which is n incremental
// `TriePut`s):
//
//	  1,000 bindings →   34 ms   (34 µs/binding)
//	 10,000 bindings →  455 ms   (46 µs/binding)
//	 50,000 bindings →  2.62 s   (52 µs/binding)
//
// Three orders of magnitude off at 50k. The error is pricing: O(n log n) is
// right, and **each of those operations is a CBOR encode plus a SHA-256 plus
// a content-store put**, not an arithmetic step. Per-binding cost grows with
// n exactly as n log n predicts.
//
// ⇒ As a PER-PASS witness consulted by N readers this inverts the trade it
// was recommended for: the source pays seconds of CPU per request where
// today it pays one B-tree range scan. An incrementally maintained root
// (O(log n) per write, O(1) per read) is the cheap shape, which is the
// sidecar the same ruling told us we did not want. Reconstruction is right
// for a ONE-OFF comparison, not for a loop.
//
// ⚠ This is core-go's builder, not a lower bound on the algorithm. A bulk
// bottom-up builder would be faster and nobody has written one.
//
// Run: go test ./shellcmd -run 'TestProbe_' -v -count=1
// The 50,000 scale is opt-in: EXCHANGE_PROBE_FULL=1

import (
	"fmt"
	"os"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/tree"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"
)

// seedFlat puts n entities under prefix and returns the peer.
func seedFlat(t *testing.T, prefix string, n int) *entitysdk.AppPeer {
	t.Helper()
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { ap.Close() })
	for i := 0; i < n; i++ {
		if _, err := ap.Store().Put(
			fmt.Sprintf("%s/file-%06d", prefix, i),
			"local/files/file",
			map[string]any{"name": fmt.Sprintf("file-%06d", i), "size": i},
		); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
	return ap
}

// TestProbe_BoundedWalkMakesNoProgress performs the case C15 was written
// from. Two passes over a prefix larger than the cap; the question is
// whether the second reaches anything the first did not.
func TestProbe_BoundedWalkMakesNoProgress(t *testing.T) {
	const over = backfillWalkLimit + 5000

	ap := seedFlat(t, "local/files/big", over)
	ws := NewShellWorkspace(ap, "self", "")

	first, truncated1, err := ws.walkRemoteFiles(ap, ap.PeerID(), "local/files/big")
	if err != nil {
		t.Fatalf("pass 1: %v", err)
	}
	second, truncated2, err := ws.walkRemoteFiles(ap, ap.PeerID(), "local/files/big")
	if err != nil {
		t.Fatalf("pass 2: %v", err)
	}

	// Anti-vacuity: the cap must actually have bitten, or this measures
	// an ordinary complete walk and says nothing.
	if !truncated1 || !truncated2 {
		t.Fatalf("neither pass truncated (%v, %v) over %d entries — the cap did not bite "+
			"and this probe is about nothing", truncated1, truncated2, over)
	}
	if len(first) != backfillWalkLimit || len(second) != backfillWalkLimit {
		t.Fatalf("passes returned %d and %d, want %d each",
			len(first), len(second), backfillWalkLimit)
	}

	inFirst := make(map[string]bool, len(first))
	for _, p := range first {
		inFirst[p] = true
	}
	var fresh int
	for _, p := range second {
		if !inFirst[p] {
			fresh++
		}
	}

	t.Logf("seeded %d · cap %d · pass 1 returned %d (truncated) · pass 2 returned %d (truncated) · "+
		"paths pass 2 reached that pass 1 did not: %d · UNREACHED FOREVER: %d",
		over, backfillWalkLimit, len(first), len(second), fresh, over-backfillWalkLimit)

	if fresh != 0 {
		t.Logf("NOTE: the walk made progress (%d new paths) — C15's prediction is WRONG "+
			"and the routed claim must be retracted", fresh)
	} else {
		t.Logf("CONFIRMED: two passes returned the identical set. %d entries are unreachable by "+
			"backfill for as long as they do not change again", over-backfillWalkLimit)
	}
}

// TestProbe_ReconstructionCost measures EXTENSION-TREE §3.7.1 — build a
// fresh trie over the bindings under a prefix and take its root — at the
// subject sizes this product actually has. This is the candidate witness
// for a live source that has published nothing.
func TestProbe_ReconstructionCost(t *testing.T) {
	for _, n := range []int{1000, 10000, 50000} {
		t.Run(fmt.Sprintf("bindings=%d", n), func(t *testing.T) {
			if n >= 50000 && os.Getenv("EXCHANGE_PROBE_FULL") == "" {
				t.Skip("50k scale is ~17 s; set EXCHANGE_PROBE_FULL=1. Recorded: 2.62 s / 52 µs per binding")
			}
			ap := seedFlat(t, "local/files/m", n)
			st := ap.Store()
			const prefix = "local/files/m/"

			// §3.7.1 is two steps and BOTH are the reader's cost: filter
			// the bindings under the prefix, then build a fresh trie over
			// them. Timing only the build would flatter it.
			reconstruct := func() (string, time.Duration) {
				t0 := time.Now()
				entries := st.List(prefix)
				bindings := make([]tree.Binding, 0, len(entries))
				for _, e := range entries {
					rel, ok := workbench.RelativeUnder(e.Path, prefix)
					if !ok {
						continue
					}
					bindings = append(bindings, tree.Binding{Path: rel, Hash: e.Hash})
				}
				h, err := tree.BuildTrie(st.ContentStore(), bindings)
				if err != nil {
					t.Fatalf("BuildTrie: %v", err)
				}
				return h.String(), time.Since(t0)
			}

			reconstruct() // warm whatever the store caches

			const reps = 5
			var best time.Duration
			var root string
			for i := 0; i < reps; i++ {
				h, d := reconstruct()
				if i == 0 || d < best {
					best = d
				}
				if root == "" {
					root = h
				} else if root != h {
					t.Fatalf("reconstruction is not deterministic: %s then %s", root, h)
				}
			}

			perBinding := best / time.Duration(n)
			t.Logf("RECONSTRUCT %6d bindings · best of %d: %8s · %s/binding · root %s",
				n, reps, best.Round(time.Microsecond), perBinding, root[:24])

			// The COST is reported, never gated — it is another repo's
			// spec claim, and a suite that goes red for a neighbour's
			// reasons teaches people to ignore the suite.
			//
			// What IS gated is the property that makes reconstruction a
			// witness at all, and it is the cross-impl one: the same
			// binding set MUST produce the same root every time. Asserted
			// across reps above; if CHAMP canonicalization ever stops
			// holding, this is the cheapest place it shows up.
			if root == "" {
				t.Fatal("no root computed")
			}
		})
	}
}
