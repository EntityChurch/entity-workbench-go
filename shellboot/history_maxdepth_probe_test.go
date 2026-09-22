package shellboot_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/types"
)

// DOES `max_depth` BOUND ANYTHING? Measured, because we were about to
// build on it.
//
// Tier: real-session (TESTING-STRATEGY §4).
//
// # Why this exists
//
// Change recording on a shared folder is **unbounded** — 2 entities and
// ~1.2 KB per write, per WRITE and not per file, so a continuously
// rewritten file costs ~1.2 GB/day with no error until the disk fills
// (SYNC-LIMITS-AND-FAILURE-MODES §2). It is this repo's top open item.
//
// D20 says price the work against the substrate first, and the substrate
// appears to have the answer already: `types.HistoryConfigData` carries
// `MaxDepth *uint64`, documented **"Max transitions per path; nil = no
// limit"**, and `ext/history/recorder.go` calls `prune(path, maxDepth)`
// after every recorded transition. That reads like a bound we simply had
// not set.
//
// **Read the prune, though.** It walks the chain to the max_depth'th
// transition and then returns, having written nothing — and it cannot do
// otherwise, because transitions are immutable content-addressed
// entities: severing the link would mean rewriting the retained
// transition with a zeroed `previous`, which changes its hash, which
// changes its successor's `previous`, cascading a rewrite of the whole
// retained chain. Its closing comment says the old transitions are
// "no longer reachable from the head" and that "GC handles cleanup".
// Neither is the case in this cohort: nothing was mutated, so the head
// still reaches every transition through `previous`, and there is no
// garbage collector in `entity-core-go` to reclaim them if there were.
//
// AP43 says a reading is not a measurement and a probe that does not
// reach the mechanism refutes nothing. So this drives the real recorder,
// through a real peer, at a path that is really being watched, and
// counts what the head reaches.
//
// # Why it lives here and not in a routing packet
//
// We are not the conformance team, and this is not a conformance test —
// it is the evidence for a limit we have to DESIGN AROUND. If `max_depth`
// starts working, this test fails and tells the next session that the
// cheapest fix to the growth problem just became available. That is the
// only reason to keep it.
func TestSubstrateLimit_HistoryMaxDepthDoesNotBoundTheChain(t *testing.T) {
	const writes = 12
	const maxDepth = 3

	dir := t.TempDir()
	p := newSyncTestPeer(t, "recorder")

	const folder = "bounded"
	mount := filepath.Join(dir, folder)
	mkdirOrFail(t, mount)
	out := mountOrFail(t, p, mount, "archives/"+folder+"/")
	root := out.RootName

	// The config under test: recording ON, with the documented bound set.
	depth := uint64(maxDepth)
	cfg := types.HistoryConfigData{
		Pattern:  "local/files/" + root + "/*",
		Enabled:  true,
		MaxDepth: &depth,
	}
	if _, err := p.ap.Store().Put("system/history/config/bounded", "system/history/config", cfg); err != nil {
		t.Fatalf("install history config: %v", err)
	}

	// Rewrite ONE file many times. This is the shape that matters — the
	// cost is per write, so a log file or an editor swap file is a single
	// path with an unbounded chain, not many paths with short ones.
	name := "rewritten.txt"
	path := filepath.Join(mount, name)
	treePath := "/" + p.id + "/local/files/" + root + "/" + name

	for i := 0; i < writes; i++ {
		body := fmt.Sprintf("revision %d\n", i)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
		// One at a time, and waited for. A burst would coalesce at the
		// watcher and measure fewer transitions than writes for a reason
		// that has nothing to do with max_depth.
		if !awaitChainDepth(t, p, root, name, i+1, 20*time.Second) {
			got := len(historyAt(t, p, treePath, 200))
			t.Fatalf("write %d did not reach the chain (depth %d, want %d) — "+
				"this probe cannot see what it is named after", i, got, i+1)
		}
	}

	reached := len(historyAt(t, p, treePath, 200))

	// The control arm. If the chain is SHORTER than the number of writes
	// for some reason other than pruning — coalescing, a dropped watch,
	// a query limit — then a "max_depth does nothing" verdict would be
	// unearned, and a "max_depth works" verdict would be unearned too.
	// The loop above already waits for each write, so reaching `writes`
	// here is the expected outcome and anything less is a broken probe.
	if reached < maxDepth {
		t.Fatalf("only %d transitions recorded for %d writes — the probe never "+
			"built a chain long enough for a bound of %d to bite",
			reached, writes, maxDepth)
	}

	if reached <= maxDepth {
		t.Errorf("max_depth=%d BOUNDED the chain to %d transitions after %d writes.\n"+
			"That is the documented behaviour and it did NOT hold when this test "+
			"was written (2026-09-07), so the substrate has changed: "+
			"ext/history/recorder.go's prune() now truncates rather than walking.\n"+
			"This is GOOD NEWS and this test is what should tell you: setting "+
			"MaxDepth on the folder history config is now the cheapest fix for "+
			"the unbounded-growth item in SYNC-LIMITS-AND-FAILURE-MODES §2. "+
			"Delete this test and take it.",
			maxDepth, reached, writes)
		return
	}

	t.Logf("MEASURED: max_depth=%d, %d writes, %d transitions still reachable from "+
		"the head — the bound is not enforced. prune() walks the chain and mutates "+
		"nothing; transitions are immutable, so severing a link would mean "+
		"rewriting every retained transition. There is also no GC in the cohort "+
		"to reclaim them if it did.",
		maxDepth, writes, reached)
}
