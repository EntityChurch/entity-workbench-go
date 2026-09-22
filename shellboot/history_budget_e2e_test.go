package shellboot_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"entity-workbench-go/workbench"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/types"
)

// The recording growth guard, driven through the real recorder.
//
// Tier: real-session (TESTING-STRATEGY §4).
//
// # What this has to establish, and why each half needs the other
//
// Change recording on a received folder is unbounded — ~1.2 KB per WRITE,
// nothing prunes it, and one continuously rewritten file is ~1.2 GB/day
// until the disk fills (SYNC-LIMITS-AND-FAILURE-MODES §2). The guard
// stops recording for the runaway path and leaves the rest of the folder
// alone. Both halves are load-bearing and each one alone is a defect:
//
//   - stopping nothing is the bug we started with;
//   - stopping the whole folder takes the chain away from the documents
//     beside the log file, which are exactly the files a conflict is
//     recovered from.
//
// The surgical half rests entirely on a claim about the KERNEL — that an
// exact-path config disabled at `system/history/config/` outranks the
// folder's `…/*` config under EXTENSION-HISTORY §6.2's specificity
// ordering, and that `configCache.find` returns nil rather than falling
// through when the most specific match is disabled. That is read out of
// `ext/history/config.go` and it is a sibling repo's code, so per D19 a
// reading is a hypothesis until the operation runs. This runs it.
//
// # Why the control arm is not optional
//
// A test that writes a file 12 times and finds a chain of 6 has not shown
// that a guard stopped it — the watcher coalesces, a burst is not 1:1
// with transitions, and half a dozen other things produce a short chain.
// So the SAME workload runs with the guard off, in the same process, and
// the chain has to keep growing there. Without that arm this test passes
// against a build where recording never worked at all.
func TestHistoryBudget_StopsARunawayPathAndLeavesTheFolderRecording(t *testing.T) {
	const budget = 5
	const writes = 14

	p := newSyncTestPeer(t, "budgeted")
	dir := t.TempDir()

	const folder = "guarded"
	mount := filepath.Join(dir, folder)
	mkdirOrFail(t, mount)
	root := mountOrFail(t, p, mount, "archives/"+folder+"/").RootName

	// Recording for the whole folder, exactly as the reconciler installs
	// it for a folder that receives.
	if _, err := p.ap.Store().Put(
		"system/history/config/folder-"+root, "system/history/config",
		types.HistoryConfigData{
			Pattern: "local/files/" + root + "/*",
			Enabled: true,
		}); err != nil {
		t.Fatalf("install folder history config: %v", err)
	}

	p.ws.EnableHistoryBudget(budget)
	t.Cleanup(p.ws.StopHistoryBudget)

	const runaway = "server.log"
	const quiet = "notes.txt"
	runawayPath := "/" + p.id + "/local/files/" + root + "/" + runaway
	quietPath := "/" + p.id + "/local/files/" + root + "/" + quiet

	// The quiet file gets a couple of versions BEFORE and one AFTER the
	// runaway trips, so the assertion is that it kept recording across the
	// trip rather than that it happened to have a chain already.
	rewrite(t, p, mount, root, quiet, 2)

	rewrite(t, p, mount, root, runaway, writes)

	limit := awaitHistoryLimit(t, p, runawayPath, 30*time.Second)
	if limit.Path == "" {
		depth := len(historyAt(t, p, runawayPath, 200))
		t.Fatalf("no limit recorded for %s after %d writes at budget %d "+
			"(chain depth %d) — the guard did not act",
			runaway, writes, budget, depth)
	}
	if limit.Budget != budget {
		t.Errorf("limit records budget %d, want %d", limit.Budget, budget)
	}
	if limit.Root != root {
		t.Errorf("limit records root %q, want %q", limit.Root, root)
	}
	if limit.Transitions <= budget {
		t.Errorf("limit records %d transitions at budget %d — a limit that "+
			"trips at or below its budget is measuring something else",
			limit.Transitions, budget)
	}

	// The config that does the stopping exists, is OURS, and is disabled.
	cfgPath := "system/history/config/" + limit.ConfigName
	ent, ok := p.ap.Store().Get(cfgPath)
	if !ok {
		t.Fatalf("limit names config %s and nothing is there — the record "+
			"and the mechanism have come apart", cfgPath)
	}
	var cfg types.HistoryConfigData
	if err := ecf.Decode(ent.Data, &cfg); err != nil {
		t.Fatalf("decode %s: %v", cfgPath, err)
	}
	if cfg.Enabled {
		t.Errorf("%s is ENABLED — the guard wrote a config that stops nothing", cfgPath)
	}
	if !strings.HasSuffix(cfg.Pattern, runaway) {
		t.Errorf("config pattern %q does not name %q — the exclusion is aimed "+
			"at the wrong path", cfg.Pattern, runaway)
	}

	// THE POINT: the chain stops growing for the runaway path.
	//
	// The baseline is taken after a settle, not at the instant the record
	// appears. The guard writes the config first and the record second,
	// and the recorder reloads its cache off the config's own change
	// event — so a write already in the watcher's pipeline can still
	// land one more transition. Baselining before that would fail the
	// assertion on a lag rather than on a defect.
	time.Sleep(time.Second)
	stopped := len(historyAt(t, p, runawayPath, 500))
	rewrite(t, p, mount, root, runaway, 6)
	settleWrites(t, p, quietPath, 3, mount, root, quiet)
	after := len(historyAt(t, p, runawayPath, 500))
	if after > stopped {
		t.Errorf("chain for %s grew from %d to %d AFTER the limit — "+
			"the exclusion config is not taking effect",
			runaway, stopped, after)
	}

	// And the file beside it is still recording. This is the half that
	// tests the kernel's specificity rule: the folder's `…/*` config is
	// still enabled and still matches this path.
	quietDepth := len(historyAt(t, p, quietPath, 50))
	if quietDepth < 3 {
		t.Errorf("chain for %s is %d deep, want at least 3 — the guard took "+
			"the whole folder's recording down with one path, which is the "+
			"outcome the exact-path exclusion exists to avoid",
			quiet, quietDepth)
	}

	// Surfaces say so. Both entry points, because a read and a pass must
	// not describe the same condition differently.
	snap, err := p.ws.StatusSnapshot()
	if err != nil {
		t.Fatalf("status snapshot: %v", err)
	}
	if len(snap.HistoryLimits) == 0 {
		t.Errorf("StatusSnapshot carries no history limits — the guard acted " +
			"and no surface can say so")
	}
	if !anyContains(snap.Problems, runaway) {
		t.Errorf("StatusSnapshot problems do not name %s:\n  %s",
			runaway, strings.Join(snap.Problems, "\n  "))
	}

	stats := p.ws.HistoryBudget()
	if !stats.Running {
		t.Errorf("HistoryBudget reports not running while the guard is attached")
	}
	if stats.Tripped != 1 {
		t.Errorf("HistoryBudget reports %d trips, want 1", stats.Tripped)
	}
	t.Logf("MEASURED: budget %d, %s stopped at %d transitions, %s still "+
		"recording at depth %d, %d transitions observed across %d paths",
		budget, runaway, limit.Transitions, quiet, quietDepth,
		stats.Transitions, stats.Paths)
}

// The control arm. Same workload, guard OFF: the chain must keep growing.
//
// Without this, the test above passes on a build where the recorder never
// ran — a chain that stayed at 5 because nothing ever recorded is
// indistinguishable, at the assertion, from one a guard stopped at 5.
func TestHistoryBudget_ControlArm_WithoutTheGuardTheChainKeepsGrowing(t *testing.T) {
	const budget = 5
	const writes = 14

	p := newSyncTestPeer(t, "unbudgeted")
	dir := t.TempDir()

	const folder = "unguarded"
	mount := filepath.Join(dir, folder)
	mkdirOrFail(t, mount)
	root := mountOrFail(t, p, mount, "archives/"+folder+"/").RootName

	if _, err := p.ap.Store().Put(
		"system/history/config/folder-"+root, "system/history/config",
		types.HistoryConfigData{
			Pattern: "local/files/" + root + "/*",
			Enabled: true,
		}); err != nil {
		t.Fatalf("install folder history config: %v", err)
	}

	// Deliberately NOT enabling the guard. Bootstrap has already enabled
	// one at the default budget, which this workload cannot reach — stop
	// it anyway so the arm is about the absence of a guard and not about
	// the size of one.
	p.ws.StopHistoryBudget()

	const runaway = "server.log"
	runawayPath := "/" + p.id + "/local/files/" + root + "/" + runaway
	rewrite(t, p, mount, root, runaway, writes)

	depth := len(historyAt(t, p, runawayPath, 500))
	if depth <= budget {
		t.Fatalf("chain is %d deep after %d writes with NO guard running — "+
			"a chain this short without a guard means the probe never "+
			"reached the recorder, and the guard test beside this one "+
			"proves nothing", depth, writes)
	}
	if limits, _ := p.ws.HistoryLimits(); len(limits) != 0 {
		t.Errorf("guard is stopped and %d limit(s) were still written", len(limits))
	}
	t.Logf("MEASURED: no guard, %d writes, chain %d deep and growing", writes, depth)
}

// --- helpers -------------------------------------------------------

// rewrite writes a file n more times, waiting for each one to reach the
// tree before the next. A burst coalesces at the watcher and would
// measure fewer transitions than writes for a reason that has nothing to
// do with the guard.
//
// Once one write fails to advance the chain the path is presumed stopped
// and the remaining writes only pause. Waiting the full timeout on each
// of them is the expected outcome after a trip, not a fault, and paying
// it turns a 30-second test into a two-minute one.
func rewrite(t *testing.T, p *syncTestPeer, mount, root, name string, n int) {
	t.Helper()
	path := filepath.Join(mount, name)
	start := existingDepth(t, p, root, name)
	advancing := true
	for i := 0; i < n; i++ {
		body := fmt.Sprintf("%s revision %d\n", name, start+i)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s #%d: %v", name, i, err)
		}
		if !advancing {
			// Still spaced, so the watcher sees distinct writes rather
			// than one coalesced change — the guard is what has to stop
			// the chain here, not the file system.
			time.Sleep(150 * time.Millisecond)
			continue
		}
		advancing = awaitChainDepth(t, p, root, name, start+i+1, 5*time.Second)
	}
}

func existingDepth(t *testing.T, p *syncTestPeer, root, name string) int {
	t.Helper()
	return len(historyAt(t, p, "/"+p.id+"/local/files/"+root+"/"+name, 500))
}

// settleWrites drives an UNGUARDED path a few times so the recorder has
// demonstrably still been running while the guarded path stayed flat.
// Otherwise "the chain stopped growing" is also satisfied by a recorder
// that stopped entirely.
func settleWrites(t *testing.T, p *syncTestPeer, quietPath string, want int, mount, root, quiet string) {
	t.Helper()
	rewrite(t, p, mount, root, quiet, 1)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if len(historyAt(t, p, quietPath, 50)) >= want {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// awaitHistoryLimit polls for the durable record of a stopped path.
func awaitHistoryLimit(t *testing.T, p *syncTestPeer, path string, timeout time.Duration) workbench.HistoryLimitData {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if l, ok := workbench.LoadHistoryLimit(p.ap.Store(), path); ok {
			return l
		}
		time.Sleep(200 * time.Millisecond)
	}
	return workbench.HistoryLimitData{}
}

func anyContains(lines []string, want string) bool {
	for _, l := range lines {
		if strings.Contains(l, want) {
			return true
		}
	}
	return false
}
