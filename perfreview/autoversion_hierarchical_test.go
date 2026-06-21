//go:build perfreview

package perfreview

import (
	"context"
	"fmt"
	"testing"
	"time"

	coretypes "go.entitychurch.org/entity-core-go/core/types"
)

// TestAutoVersion_HierarchicalWorkload tests the §6.11 hypothesis: the
// O(N)/Put pathology surfaced by Investigation 11 is specific to flat-
// growing namespaces (one trie node accumulates N siblings). A
// hierarchical path shape (bounded fanout at each level) should NOT
// hit the pathology because no single node grows wide.
//
// Method: configure auto-version on a prefix, write N entities under
// a hierarchical path shape (docs/{aa}/{bb}/{cc}/file-{n}, where aa,
// bb, cc have ~10 children each). Compare per-Put latency growth with
// the flat-namespace baseline.
//
// Expected: per-Put latency stays roughly flat (constant per Put, not
// growing linearly with N).
//
// If this confirms the hypothesis, the framing for the feedback memo
// shifts from "auto-version is broken" to "auto-version assumes the
// hierarchical workload shape that the type system already encourages."
func TestAutoVersion_HierarchicalWorkload(t *testing.T) {
	h := NewHarness(t, HarnessOptions{})
	defer h.Close()

	yes := true
	cfg := coretypes.RevisionConfigData{
		Prefix:      "docs/",
		AutoVersion: &yes,
	}
	params := coretypes.RevisionConfigParamsData{
		Action: "set",
		Name:   "perfreview-hier",
		Config: &cfg,
	}
	if _, err := h.Peer().Revision().Config(context.Background(), params); err != nil {
		t.Fatalf("install revision config: %v", err)
	}

	storeAPI := h.Peer().Store()
	const N = 500

	// Hierarchical path shape: docs/{level1}/{level2}/{level3}/file-{idx}
	// With 10 children at each level (00..09) and a 3-level deep prefix,
	// 1000 leaves can fit before any node would have more than 10
	// siblings. For 500 entities we definitely stay within that bound.
	pathOf := func(i int) string {
		l1 := (i / 100) % 10
		l2 := (i / 10) % 10
		l3 := i % 10
		// The /file-N segment makes each leaf path unique even though
		// the directory structure is shared. Each "directory" node
		// (l1, l2, l3) has at most 10 children — the bounded fanout.
		return fmt.Sprintf("docs/%02d/%02d/%02d/file-%d", l1, l2, l3, i)
	}

	t.Logf("starting %d Puts with auto-version on docs/ prefix (hierarchical)", N)
	type sample struct {
		I        int
		Elapsed  time.Duration
		Entities int
		Locs     int
		HeapMiB  float64
	}
	samples := []sample{}

	for i := 0; i < N; i++ {
		path := pathOf(i)
		start := time.Now()
		if _, err := storeAPI.Put(path, "perfreview/entity",
			map[string]interface{}{"tick": i, "time": "x"}); err != nil {
			t.Fatalf("Put %d: %v", i, err)
		}
		elapsed := time.Since(start)
		if i%50 == 0 {
			m := h.Snapshot("", 0, 0, 0, 0, 0)
			samples = append(samples, sample{
				I: i, Elapsed: elapsed, Entities: m.EntityCount,
				Locs:    m.LocationCount,
				HeapMiB: float64(m.HeapInUseBytes) / 1024 / 1024,
			})
			t.Logf("put %3d: %s (entities=%d locs=%d heap=%.1fMiB)",
				i, short(elapsed), m.EntityCount, m.LocationCount,
				float64(m.HeapInUseBytes)/1024/1024)
		}
	}

	// Linear-regression sanity: compute first-vs-last per-Put time.
	if len(samples) >= 2 {
		first := samples[0].Elapsed
		last := samples[len(samples)-1].Elapsed
		t.Logf("per-Put latency: i=%d → %s; i=%d → %s (growth: %.2fx)",
			samples[0].I, short(first), samples[len(samples)-1].I, short(last),
			float64(last)/float64(first))
	}
}
