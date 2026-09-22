package shellcmd

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"entity-workbench-go/workbench"

	"go.entitychurch.org/entity-core-go/core/types"
)

// history_budget.go — the guard that stops one runaway path from filling
// the disk, and says so.
//
// # The failure
//
// A folder that receives has change recording on for its whole mount
// prefix (folder_history.go), because that is what makes an overwritten
// local edit recoverable. Recording is flat and cheap per write —
// ~1.2 KB, 2 entities, latency that does not grow from a thousand writes
// to a hundred thousand (SYNC-LIMITS-AND-FAILURE-MODES §2). **And nothing
// prunes it.** The cost is per WRITE, so the number that matters is the
// write rate and not the file count:
//
//	a documents folder      ~100 writes/day     ~120 KB/day
//	an active project       ~5,000 writes/day   ~6 MB/day
//	one appended log file   ~1,000,000/day      ~1.2 GB/day
//
// The third row has no ceiling, produces no error, and ends with a full
// disk. It has been this repo's top open item since the load work.
//
// # Why this is a guard and not a bound
//
// D20 first, and it came back negative: `HistoryConfigData.MaxDepth` is
// documented as a per-path transition bound and prunes nothing, measured
// (`shellboot/history_maxdepth_probe_test.go`). It cannot easily do
// otherwise — transitions are immutable and content-addressed, so
// severing a link cascades a rewrite of the retained chain — and there is
// no garbage collector in the cohort to reclaim what a sever would orphan.
// So there is no mechanism anywhere below us that makes an existing chain
// smaller. The only lever is to stop adding.
//
// **State the trade rather than hiding it.** Stopping keeps the OLDEST
// positions and loses the newest, which is backwards for recovery. That
// is acceptable for the workload this catches — nobody wants the version
// history of a log file — and it is why a tripped limit is a Problem on
// every status reading rather than a silent internal event. An operator
// who disagrees has two answers, both named in the message: take the file
// out of the shared folder, or re-enable the config by hand.
//
// # The signal, and why it costs nothing
//
// Every recorded transition writes a head pointer at
// `system/history/head/{tracked-path}` (`ext/history/recorder.go`), and
// that write is an ordinary tree mutation. So a prefix watch on
// `system/history/head/` delivers **exactly one event per recorded
// transition**, with the tracked path on it, for the price of a map
// increment. No polling, no store scan, and nothing to keep in step with
// the recorder's internals: if recording is off for a path the events
// stop by themselves.
//
// # Why the depth is derived lazily, and what that costs
//
// A counter in process memory says how much a path grew SINCE THIS
// PROCESS STARTED, and the chain it is guarding is durable. Deriving the
// true depth for every tracked path at attach would be O(files) dispatched
// queries on every launch — paid by every peer, forever, to answer a
// question about a handful of pathological paths.
//
// So the depth is derived exactly once per path, and only after that path
// has shown itself busy in this process ([historyProbeThreshold] events).
// Cold paths — which is nearly all of them — cost one map entry and never
// a query. A path that was already deep when the process started is
// measured the moment it proves it is still being written to, which is
// the only case where the carried-over depth changes an answer.
//
// The known limit, stated because it is invisible from the code: a path
// that accumulated a deep chain in an earlier run and is written fewer
// than [historyProbeThreshold] times in this one is never measured and
// never trips. That is the path that is not growing, so the miss costs
// nothing that matters — but a report of "no limits tripped" means "none
// in this process's view", not "no deep chains exist".

// DefaultHistoryPathBudget is how many recorded versions of ONE path we
// keep before stopping.
//
// The number is chosen against the two workloads it has to separate, not
// picked for roundness. At ~1.2 KB per transition, 2,000 versions of one
// file is ~2.4 MB — more revisions of a single document than any conflict
// recovery has ever needed, and cheap. The workload on the other side
// writes about that many times every three minutes, so the two are three
// orders of magnitude apart and no plausible setting of this constant
// confuses them.
//
// What it does NOT bound is the aggregate: a folder of N files can still
// reach N × budget, because the guard acts on the shape that grows
// without limit in TIME and a per-file chain grows with the data. That
// case is proportional to what the operator copied in and is stated in
// SYNC-LIMITS-AND-FAILURE-MODES §2 rather than guarded here.
const DefaultHistoryPathBudget = 2000

// historyProbeThreshold is how many transitions a path must take in THIS
// process before its true chain depth is worth a dispatched query.
//
// A tenth of the budget in force, and DERIVED from it rather than a
// constant beside it. A fixed threshold is wrong the moment the budget is
// configured lower than it: nothing would ever be measured, so nothing
// would ever trip, and the guard would report itself running while doing
// nothing at all — which is the failure mode that reads as "no runaway
// paths" (TestHistoryBudget_ProbeThresholdTracksTheBudget).
//
// At the default that is 200: low enough that a path carrying a deep
// chain from a previous run is measured within seconds of becoming busy
// again, high enough that ordinary editing never triggers a query.
func historyProbeThreshold(budget uint64) uint64 {
	if n := budget / 10; n > 0 {
		return n
	}
	return 1
}

// historyHeadPrefix is where the recorder writes one pointer per tracked
// path. Mirrors `ext/history/recorder.go`'s headPrefix; not imported
// because that constant is unexported, and a spec-adjacent path we
// restate is asserted by TestHistoryBudget_HeadPrefixMatchesTheRecorder.
const historyHeadPrefix = "system/history/head/"

// historyBudgetState is the workspace's memory of the guard.
type historyBudgetState struct {
	mu      sync.Mutex
	budget  uint64
	paths   map[string]*historyPathCount
	cancel  func()
	running bool

	// observed is every transition this process has seen, across all
	// paths. Reported so a surface can say what the guard is actually
	// watching — a guard with a zero here is not quiet, it is deaf.
	observed uint64
	// tripped counts limits applied in this process, as opposed to the
	// durable records, which include earlier runs.
	tripped int
}

// historyPathCount is one tracked path's accounting.
type historyPathCount struct {
	// seen is events observed in this process, including the one-shot
	// seed the SDK delivers at attach.
	seen uint64
	// total is the chain depth once derived, then incremented per event.
	// Meaningless until derived.
	total uint64
	// derived is whether total has been measured against the tree.
	derived bool
	// probing guards against two derivations racing for one path.
	probing bool
	// done means a limit has been applied, or one already existed when
	// the guard started. Either way the guard has said what it has to say
	// and never acts on this path again — which is what lets an operator
	// re-enable the config without the guard immediately undoing them.
	done bool
}

// EnableHistoryBudget starts the guard and returns immediately.
//
// budget <= 0 means DefaultHistoryPathBudget. Calling it twice replaces
// the running guard rather than adding a second one.
func (ws *ShellWorkspace) EnableHistoryBudget(budget uint64) {
	ws.StopHistoryBudget()
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return
	}
	if budget == 0 {
		budget = DefaultHistoryPathBudget
	}
	st := ws.Local.Peer.Store()

	// Every path we have already acted on, so a restart neither re-derives
	// a depth we measured last time nor re-applies a limit an operator may
	// have since overridden.
	known := map[string]*historyPathCount{}
	limits, _ := workbench.LoadHistoryLimits(st)
	for _, l := range limits {
		known[l.Path] = &historyPathCount{done: true, derived: true, total: l.Transitions}
	}

	ws.historyBudget.mu.Lock()
	ws.historyBudget.budget = budget
	ws.historyBudget.paths = known
	ws.historyBudget.observed = 0
	ws.historyBudget.tripped = 0
	ws.historyBudget.mu.Unlock()

	// Attach OUTSIDE our lock. OnPrefixChange delivers its seed
	// synchronously on the calling goroutine when the store has no watch
	// hub, so attaching under the lock deadlocks on the first seeded
	// event — a hang with no panic and no race report (AP60).
	cancel := st.OnPrefixChange(historyHeadPrefix, ws.onHistoryHeadChange)

	ws.historyBudget.mu.Lock()
	ws.historyBudget.cancel = cancel
	ws.historyBudget.running = true
	ws.historyBudget.mu.Unlock()
}

// StopHistoryBudget detaches the watch. Safe to call when none is running.
func (ws *ShellWorkspace) StopHistoryBudget() {
	if ws == nil {
		return
	}
	// Take the cancel under the lock and CALL IT OUTSIDE. A cancel waits
	// for its delivery goroutine to exit, and that goroutine may be inside
	// onHistoryHeadChange taking this same mutex — cancelling under the
	// lock deadlocks silently (AP60, and it once sat on the workbench
	// suite for sixteen minutes).
	ws.historyBudget.mu.Lock()
	cancel := ws.historyBudget.cancel
	ws.historyBudget.cancel = nil
	ws.historyBudget.running = false
	ws.historyBudget.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// HistoryBudgetStats is what the guard has seen in this process.
type HistoryBudgetStats struct {
	// Running is whether the watch is attached. False means no path is
	// being counted at all, which a surface must not render as "nothing
	// has grown".
	Running bool
	// Budget is the per-path limit in force.
	Budget uint64
	// Paths is how many distinct tracked paths this process has observed.
	Paths int
	// Transitions is every recorded transition this process has observed.
	Transitions uint64
	// Tripped is limits applied in this process. The durable set is
	// LoadHistoryLimits and is the larger number after a restart.
	Tripped int
}

// HistoryBudget reports the guard's state.
func (ws *ShellWorkspace) HistoryBudget() HistoryBudgetStats {
	if ws == nil {
		return HistoryBudgetStats{}
	}
	ws.historyBudget.mu.Lock()
	defer ws.historyBudget.mu.Unlock()
	return HistoryBudgetStats{
		Running:     ws.historyBudget.running,
		Budget:      ws.historyBudget.budget,
		Paths:       len(ws.historyBudget.paths),
		Transitions: ws.historyBudget.observed,
		Tripped:     ws.historyBudget.tripped,
	}
}

// onHistoryHeadChange is the watch handler: one call per recorded
// transition.
//
// It runs on an SDK-owned goroutine that also carries every other event
// on this prefix, so it does the cheap accounting inline and hands
// anything that touches the store to a goroutine. A dispatched history
// query inline would backpressure the watch channel behind it — and the
// events queued behind it are the ones telling us the path is still
// growing.
func (ws *ShellWorkspace) onHistoryHeadChange(ev workbench.ChangeEvent) {
	tracked, root, ok := trackedPathFromHead(ev.Path)
	if !ok {
		return
	}

	ws.historyBudget.mu.Lock()
	if ws.historyBudget.paths == nil {
		ws.historyBudget.mu.Unlock()
		return
	}
	budget := ws.historyBudget.budget
	ws.historyBudget.observed++
	pc := ws.historyBudget.paths[tracked]
	if pc == nil {
		pc = &historyPathCount{}
		ws.historyBudget.paths[tracked] = pc
	}
	if pc.done {
		ws.historyBudget.mu.Unlock()
		return
	}
	pc.seen++
	var (
		probe uint64 // non-zero: derive the depth, bounded by this many
		trip  uint64 // non-zero: apply a limit at this measured depth
	)
	switch {
	case pc.derived:
		pc.total++
		if pc.total > budget {
			pc.done = true
			ws.historyBudget.tripped++
			trip = pc.total
		}
	case !pc.probing && pc.seen >= historyProbeThreshold(budget):
		pc.probing = true
		probe = budget + 1
	}
	ws.historyBudget.mu.Unlock()

	switch {
	case trip > 0:
		go ws.applyHistoryLimit(tracked, root, trip, budget)
	case probe > 0:
		go ws.deriveHistoryDepth(tracked, root, probe, budget)
	}
}

// deriveHistoryDepth measures one path's real chain depth and folds it
// into the running count.
//
// Bounded by limit, so the cost of measuring a runaway path is the budget
// and not the chain — the whole point is that the chain has no ceiling.
func (ws *ShellWorkspace) deriveHistoryDepth(tracked, root string, limit, budget uint64) {
	depth, ok := ws.historyChainDepth(tracked, limit)

	ws.historyBudget.mu.Lock()
	pc := ws.historyBudget.paths[tracked]
	if pc == nil || pc.done {
		if pc != nil {
			pc.probing = false
		}
		ws.historyBudget.mu.Unlock()
		return
	}
	pc.probing = false
	if !ok {
		// The query failed. Fall back to what this process counted, which
		// is a floor rather than an estimate: it under-reports a carried
		// chain and never over-reports one, so the guard stays late rather
		// than becoming wrong.
		depth = pc.seen
	}
	if depth < pc.seen {
		// The query is a snapshot and events kept arriving while it ran.
		depth = pc.seen
	}
	pc.total = depth
	pc.derived = true
	var trip uint64
	if pc.total > budget {
		pc.done = true
		ws.historyBudget.tripped++
		trip = pc.total
	}
	ws.historyBudget.mu.Unlock()

	if trip > 0 {
		ws.applyHistoryLimit(tracked, root, trip, budget)
	}
}

// historyChainDepth asks the history handler how deep a path's chain is,
// bounded by limit. Reports false when the question could not be answered
// at all, which is different from "the chain is empty".
func (ws *ShellWorkspace) historyChainDepth(tracked string, limit uint64) (uint64, bool) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return 0, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), historyProbeTimeout)
	defer cancel()
	n := limit
	res, err := ws.Local.Peer.History().Query(ctx, types.HistoryQueryParamsData{
		Path:  tracked,
		Limit: &n,
	})
	if err != nil {
		return 0, false
	}
	return uint64(len(res.Transitions)), true
}

// historyProbeTimeout bounds one depth measurement. Generous: it is a
// local dispatched read that returns up to `budget` transitions, and a
// slow answer is far better than an unmeasured path.
const historyProbeTimeout = 30 * time.Second

// applyHistoryLimit stops recording for one path and records why.
//
// The order matters. The CONFIG goes first, because it is what stops the
// growth and every millisecond after the decision is more of it; the
// record follows and is what makes the stop visible. A crash between the
// two leaves a path silently unrecorded — so the config's own name
// carries the marker prefix, and reconcileHistoryLimits reports a
// guard-written config that has no record beside it rather than letting
// it pass as an operator's own.
func (ws *ShellWorkspace) applyHistoryLimit(tracked, root string, depth, budget uint64) {
	st := ws.Local.Peer.Store()
	name := historyLimitConfigName(tracked)

	pattern, ok := historyExclusionPattern(tracked)
	if !ok {
		return
	}
	cfg := types.HistoryConfigData{Pattern: pattern, Enabled: false}
	if _, err := st.Put("system/history/config/"+name, "system/history/config", cfg); err != nil {
		return
	}
	_ = workbench.SaveHistoryLimit(st, workbench.HistoryLimitData{
		Path:        tracked,
		Root:        root,
		ConfigName:  name,
		Transitions: depth,
		Budget:      budget,
		AtMillis:    uint64(time.Now().UnixMilli()),
	})
}

// historyLimitConfigNamePrefix marks a config this guard wrote.
//
// Greppable on purpose: an operator looking at `system/history/config/`
// must be able to tell the three configs apart — the folder's own
// `folder-{root}`, one they wrote by hand, and one the guard installed
// because a path ran away.
const historyLimitConfigNamePrefix = "budget-stopped-"

func historyLimitConfigName(tracked string) string {
	return historyLimitConfigNamePrefix + workbench.HistoryLimitKey(tracked)
}

// historyExclusionPattern turns a qualified tracked path into the config
// pattern that stops recording for exactly it.
//
// SHORT form — `local/files/{root}/{rel}`, no peer segment — because the
// recorder canonicalizes an unrooted pattern against the LOCAL peer
// (`ext/history/config.go`, canonicalizePattern). That is the same choice
// FolderHistoryPattern makes, for the same reason: a pattern carrying our
// peer-id would be a claim about a namespace, and this one is about our
// own tree.
//
// It outranks the folder's `local/files/{root}/*` under §6.2's ordering
// because it has strictly more literal segments, and `configCache.find`
// returns nil when the most specific match is disabled — so this stops
// one path and leaves the rest of the folder recording.
func historyExclusionPattern(tracked string) (string, bool) {
	_, bare, ok := splitQualified(tracked)
	if !ok || !strings.HasPrefix(bare, workbench.LocalFilesSourcePrefix) {
		return "", false
	}
	return bare, true
}

// trackedPathFromHead recovers (tracked path, mount root) from a head
// pointer path, and reports whether the path is one we guard.
//
// A head pointer lives at `system/history/head/{tracked-without-leading-
// slash}` and the tracked path is itself qualified, so the event path
// carries the peer-id twice:
//
//	/{us}/system/history/head/{us}/local/files/{root}/{rel}
//	                          └──────── tracked, minus its slash ────┘
//
// Only `local/files/` paths are guarded. A config an operator installed
// over some other prefix is theirs, and applying a budget to it would be
// this loop deciding something nobody declared.
func trackedPathFromHead(headPath string) (tracked, root string, ok bool) {
	rel, under := workbench.RelativeUnder(headPath, historyHeadPrefix)
	if !under || rel == "" {
		return "", "", false
	}
	tracked = "/" + rel
	_, bare, ok := splitQualified(tracked)
	if !ok {
		return "", "", false
	}
	if !strings.HasPrefix(bare, workbench.LocalFilesSourcePrefix) {
		return "", "", false
	}
	remainder := bare[len(workbench.LocalFilesSourcePrefix):]
	i := strings.IndexByte(remainder, '/')
	if i <= 0 || i == len(remainder)-1 {
		// `local/files/{root}` with nothing under it is the root's own
		// entity, not a file in it.
		return "", "", false
	}
	return tracked, remainder[:i], true
}

// splitQualified splits `/{peer}/{bare}` into its two halves.
func splitQualified(path string) (peer, bare string, ok bool) {
	if !strings.HasPrefix(path, "/") {
		return "", "", false
	}
	rest := path[1:]
	i := strings.IndexByte(rest, '/')
	if i <= 0 || i == len(rest)-1 {
		return "", "", false
	}
	return rest[:i], rest[i+1:], true
}

// HistoryLimits reads the durable records of every path whose recording
// was stopped, with the problems from reading them.
func (ws *ShellWorkspace) HistoryLimits() ([]workbench.HistoryLimitData, []string) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return nil, nil
	}
	return workbench.LoadHistoryLimits(ws.Local.Peer.Store())
}

// noteHistoryLimits records stopped paths on an outcome — the ONE place
// either entry point learns about them.
//
// Shared by StatusSnapshot and Reconcile for observeDevice's reason: a
// read and a pass must not be able to describe the same condition
// differently. It reports through Problems and not Actions, because a
// stopped path is not something the pass just did — it is a standing
// state of the peer that stays true until an operator acts, and a line
// that appears once and then goes quiet is how this became invisible in
// the first place.
func (ws *ShellWorkspace) noteHistoryLimits(out *ReconcileOutcome) {
	limits, problems := ws.HistoryLimits()
	out.HistoryLimits = limits
	for _, p := range problems {
		out.Problems = append(out.Problems,
			fmt.Sprintf("history limit record %s — a path may be silently unrecorded", p))
	}
	for _, l := range limits {
		out.Problems = append(out.Problems, l.Summary())
	}
}
