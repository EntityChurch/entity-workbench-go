package shellcmd

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"entity-workbench-go/workbench"
)

// catchup.go — the half of the sync loop that makes a dropped delivery a
// delay instead of a loss.
//
// # The failure
//
// Measured 2026-09-07 (`shellboot/bigcopy_load_test.go`): 2000 files
// dropped into a shared folder at once. The sender ingested and bound all
// 2000; **676 reached the receiver**; the copy then stopped for good, with
// no error on either side and `syncs` listing healthy throughout. The
// sender's subscription engine had discarded **2327** notifications
// because its delivery shards were full — a deliberate, counted,
// deadlock-avoiding drop in `ext/subscription/engine.go`.
//
// # Why the receiver has to be the one to fix it, and why on a timer
//
// The drop happens on the SENDER, before anything goes on the wire. The
// receiver is not behind — it was never told. There is no failed delivery
// to retry, no chain error to collect, and no counter on this side that
// moves. So a receiver cannot *detect* this at all; it can only re-derive
// the truth by asking the sender what it actually has, which is exactly
// what a backfill pass does.
//
// That is the whole justification for a periodic pass rather than an
// event-driven one. It is not polling for lack of a better idea; it is
// that the only local signal available is the absence of something we
// were never promised.
//
// # And the specification says so, which we found out afterwards
//
// EXTENSION-SUBSCRIPTION §5.5 (Gap Detection) ends: *"For guaranteed
// consistency, subscribers SHOULD periodically reconcile via GET on
// subscribed paths."* §6.3 property (ii) MUSTs the same outcome where a
// peer offers the mirror recipe — *"Convergence MUST hold under delivery
// loss."* **Delivery is best-effort by design; this loop is the SHOULD.**
// It was designed here from first principles over three days before
// anyone read that subsection, which is D20 Amendment 1's source.
//
// Reading it also produced the one finding worth routing, because §5.5's
// *primary* mechanism does not work for this workload. The
// `previous_hash` chain check detects a gap when a LATER notification on
// the same URI arrives with a mismatched predecessor — and a folder sync
// writes most paths exactly once, so a dropped notification is the only
// one that chain will ever carry and no successor arrives to mismatch.
// Of the 2327 drops above, the number `previous_hash` could have caught
// is zero. Routed as
// reviews/SUBSCRIPTION-SATURATION-AND-THE-LAYER-BOUNDARY-2026-09-07.md.
//
// # Why this is affordable
//
// Because the backfill already short-circuits. `blob_resolve`'s F9 check
// compares the local file entity's blob hash against the incoming one and
// skips when they match, so a pass over an up-to-date folder transfers
// nothing. Measured over 2000 files: a recovering pass took ~700 ms and
// moved 1324 files; a pass with **nothing to do took 472 ms** — 0.24 ms
// per file, no writes, no disk churn.
//
// # Why the interval is measured from the END of a pass
//
// A ticker would stack passes on a folder big enough to take longer than
// the interval, and the failure mode of that is a peer that spends all its
// time catching up and reports itself busy forever. Sleeping *after* a
// pass makes a large folder self-space: the loop degrades to back-to-back
// passes rather than to unbounded concurrency.
//
// # What it deliberately does NOT do
//
// It does not dial. `Resync` uses the sync binding and the pooled
// connection, so this is safe to leave running in a GUI — unlike
// `Reconcile`, which dials every declared device and must never be wired
// to a timer or a panel refresh (see status.go). It also does not create,
// delete, or unmount anything: a catch-up is a read of the sender's folder
// plus writes into a mount the operator already agreed to.

// DefaultCatchUpInterval is the pause between the end of one catch-up
// pass and the start of the next.
//
// A minute is a compromise with its terms stated: long enough that a pass
// over a large folder is a rounding error on this peer's load, short
// enough that "it stopped part way" is a delay an operator does not
// notice rather than a support request. Nothing depends on the exact
// value — the loop is idempotent at any interval.
const DefaultCatchUpInterval = 60 * time.Second

// The adaptive band. The supervisor does not run at a fixed rate — it
// runs FAST while it is finding lost files and BACKS OFF once it stops.
//
// # Why adaptive rather than a fixed minute
//
// The two regimes are three orders of magnitude apart in urgency and
// identical in cost. A folder that has just taken a 2000-file burst is
// missing two thirds of itself and every second of delay is an operator
// staring at an incomplete directory; a folder that has been idle since
// yesterday needs a pass only to notice a change that delivery dropped,
// which is rare. A single interval has to be wrong for one of them.
//
// # The signal, and why the receiver has one at all
//
// The receiving peer cannot see the sender's drop counter, so it cannot
// detect "I am behind" directly. But it can read its own PASSES: a pass
// that recovered files means delivery is losing things right now, and a
// pass that recovered nothing means it is not. That is a purely local
// signal, needs no protocol change, and is exactly as accurate as the
// thing it is used for.
//
// # Why backing off is not just a cost saving
//
// Converging fast after a burst matters more than the average rate, and
// hammering during one is counterproductive: a pass taken in the middle
// of a burst reads a moving target and re-does most of its work on the
// next pass. Settling first and then reading the FINAL state is both
// cheaper and more correct — one pass over a settled folder gets
// everything, because a catch-up reads current state rather than
// replaying a change stream.
const (
	// MinCatchUpInterval is the floor while actively recovering.
	MinCatchUpInterval = 5 * time.Second
	// MaxCatchUpInterval is the HARD ceiling — the longest this loop will
	// ever wait, whatever a pass costs. It is not the resting rate; see
	// settledCeiling, which is usually much lower.
	MaxCatchUpInterval = 10 * time.Minute
	// catchUpBackoffFactor is how fast a settled loop backs off.
	catchUpBackoffFactor = 2
	// catchUpIdleDutyDivisor bounds what an IDLE folder is allowed to
	// cost: the resting interval is at least this many times the duration
	// of a pass, i.e. a settled folder occupies at most 1/N of the wall
	// clock. 100 → 1%.
	catchUpIdleDutyDivisor = 100
)

// settledCeiling is how far this loop is allowed to back off for a folder
// whose passes cost `passDuration` — DERIVED, where it used to be the flat
// MaxCatchUpInterval constant.
//
// # Why a constant ceiling was wrong, and it is the operator who said so
//
// The intervals double, so the PASSES land at t=0, 2 min, 6 min, 14 min,
// 24 min, and every 10 minutes after that. Four empty passes — about
// fourteen minutes of quiet — and the worst-case time to notice a change
// that missed live delivery is TEN MINUTES. On a folder where a pass costs
// a quarter of a second. The operator's words were *"doubling exponentially
// is not the right frequency, it grows too quickly"*, and the numbers agree:
// the back-off was buying a saving that was not needed and paying for it in
// the one currency this product is judged in.
//
// `SYNC-LIMITS` already named the underlying flaw as a known limitation —
// *"the catch-up rate adapts to whether it is finding anything, NOT to
// folder SIZE; a 100k-file folder uses the same ladder as a 10-file one"*.
// This is that limitation closed from the other end: the ladder is
// unchanged, and what it may climb TO is now a function of measured cost.
//
// An idle pass costs ~0.24 ms/file (SYNC-LIMITS §1), so:
//
//	     files   pass    ceiling  steady-state duty
//	       10    ~0 s      60 s   negligible
//	    1,000   0.24 s     60 s   0.4 %
//	   10,000   2.4 s       4 min 1 %
//	  100,000    24 s      10 min 4 %   (hard cap)
//
// So an ordinary folder now rests at the base rate — which is also
// Syncthing's default rescan interval, and that is not a coincidence: it is
// the rate this class of tool has already converged on — while a folder big
// enough for a pass to actually cost something still backs away from it.
//
// The floor is DefaultCatchUpInterval and not something smaller because
// below that the loop stops being a safety net and starts being a poller.
func settledCeiling(passDuration time.Duration) time.Duration {
	ceiling := passDuration * catchUpIdleDutyDivisor
	if ceiling < DefaultCatchUpInterval {
		ceiling = DefaultCatchUpInterval
	}
	if ceiling > MaxCatchUpInterval {
		ceiling = MaxCatchUpInterval
	}
	return ceiling
}

// nextCatchUpInterval is the adaptive rule, as a pure function so it can
// be tested without a clock.
//
// recovered > 0  → drop to the floor: we are behind and more is coming.
// recovered == 0 → back off geometrically toward the ceiling, where the
//                  ceiling is DERIVED from what a pass costs
//                  (settledCeiling), not a flat constant.
//
// The ramp is a GRADIENT and the configured rate is only where it starts,
// not a floor it snaps back to. An earlier version clamped the way up at
// `base`, which made the loop jump 5 s → 60 s in one step the moment a
// burst ended — losing every rate in between, which are exactly the ones
// worth having while a copy is still trickling in. Recovery is
// deliberately asymmetric: back off gently, return to the floor in ONE
// step (see the convergence test), because being slow to notice a burst
// is the failure an operator feels and being slow to relax is not.
//
// The anti-churn floor is the part that matters on a big folder: the
// next wait is never shorter than the pass that just ran, so the
// supervisor can never occupy more than half the wall clock however far
// behind it is. Without it, a folder whose pass takes 20 s would be
// passing back-to-back forever, which is the "don't churn on it"
// failure — it burns the peer, and it re-reads a moving target instead
// of letting the burst settle.
func nextCatchUpInterval(current, passDuration time.Duration, recovered int) time.Duration {
	var next time.Duration
	if recovered > 0 {
		next = MinCatchUpInterval
	} else {
		next = current * catchUpBackoffFactor
		if next < MinCatchUpInterval {
			next = MinCatchUpInterval
		}
		// The ceiling is DERIVED from what a pass costs, not a flat ten
		// minutes — see settledCeiling. A cheap folder rests at the base
		// rate instead of climbing to a ten-minute blind spot.
		if ceiling := settledCeiling(passDuration); next > ceiling {
			next = ceiling
		}
	}
	// Never spend more than half the wall clock catching up.
	if next < passDuration {
		next = passDuration
	}
	return next
}

// CatchUpResult is what one pass over every sync binding did.
type CatchUpResult struct {
	// Folders is the number of sync bindings the pass covered.
	Folders int
	// Recovered is the number of files this pass actually transferred —
	// i.e. how much had been silently lost. Zero is the healthy steady
	// state and is the number worth watching.
	Recovered int
	// AlreadyCurrent is what the F9 check skipped. It is the bulk of any
	// healthy pass and is reported so a reader can tell "nothing to do"
	// from "nothing happened".
	AlreadyCurrent int
	// Failed is per-file failures across all folders.
	Failed int
	// Problems name folders the pass could not cover, and why.
	Problems []string
	// Duration is how long the pass took.
	Duration time.Duration
	// At is when the pass finished, Unix milliseconds.
	AtMillis uint64
}

// Summary is one operator-facing line.
func (r CatchUpResult) Summary() string {
	if r.Folders == 0 {
		return "catch-up: no folders are being received"
	}
	s := fmt.Sprintf("catch-up over %d folder(s) in %s: %d recovered, %d already current",
		r.Folders, r.Duration.Round(time.Millisecond), r.Recovered, r.AlreadyCurrent)
	if r.Failed > 0 {
		s += fmt.Sprintf(", %d failed", r.Failed)
	}
	if len(r.Problems) > 0 {
		s += fmt.Sprintf(", %d problem(s)", len(r.Problems))
	}
	return s
}

// CatchUp runs one backfill pass over every sync binding this peer holds.
//
// Idempotent and safe to call at any time. It is the same operation the
// `resync` verb performs, over every folder instead of one — deliberately
// the SAME call and not a parallel implementation, so a fix to one is a
// fix to both.
func (ws *ShellWorkspace) CatchUp(ctx context.Context) (CatchUpResult, error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return CatchUpResult{}, fmt.Errorf("workspace has no local peer")
	}
	start := time.Now()
	res := CatchUpResult{}

	bindings, problems := workbench.LoadSyncBindings(ws.Local.Peer.Store())
	res.Problems = append(res.Problems, problems...)

	// Stable order, so two passes over the same state produce the same
	// report and a reader can diff them.
	sort.Slice(bindings, func(i, j int) bool {
		if bindings[i].RemotePeerID != bindings[j].RemotePeerID {
			return bindings[i].RemotePeerID < bindings[j].RemotePeerID
		}
		return bindings[i].Root < bindings[j].Root
	})

	for _, b := range bindings {
		if err := ctx.Err(); err != nil {
			res.Problems = append(res.Problems, "catch-up cancelled part way")
			break
		}
		res.Folders++
		out, err := ws.Resync(b.RemotePeerID, b.Root)
		if err != nil {
			// A peer that is offline is the ordinary case, not a fault:
			// the next pass covers it. Recorded, not escalated.
			res.Problems = append(res.Problems,
				fmt.Sprintf("folder %q from %s: %v", b.Root, shortPeer(b.RemotePeerID), err))
			continue
		}
		res.Recovered += out.Materialized
		res.AlreadyCurrent += out.AlreadyCurrent
		res.Failed += out.Failed
	}

	res.Duration = time.Since(start)
	res.AtMillis = uint64(time.Now().UnixMilli())
	return res, nil
}

// catchUpState is the workspace's memory of the supervisor.
type catchUpState struct {
	mu       sync.Mutex
	last     CatchUpResult
	haveOne  bool
	running  bool
	stop     func()
	interval time.Duration
}

// LastCatchUp returns the most recent pass, and whether one has happened.
//
// A surface needs both: "no drops recovered" and "nothing has run yet"
// are different claims, and rendering the second as the first is how a
// stalled supervisor reads as a healthy one.
func (ws *ShellWorkspace) LastCatchUp() (CatchUpResult, bool) {
	if ws == nil {
		return CatchUpResult{}, false
	}
	ws.catchUp.mu.Lock()
	defer ws.catchUp.mu.Unlock()
	return ws.catchUp.last, ws.catchUp.haveOne
}

// RunCatchUpOnce runs a pass and records it, unless one is already in
// flight — in which case it reports that rather than stacking.
func (ws *ShellWorkspace) RunCatchUpOnce(ctx context.Context) (CatchUpResult, bool) {
	ws.catchUp.mu.Lock()
	if ws.catchUp.running {
		ws.catchUp.mu.Unlock()
		return CatchUpResult{}, false
	}
	ws.catchUp.running = true
	ws.catchUp.mu.Unlock()

	res, err := ws.CatchUp(ctx)

	ws.catchUp.mu.Lock()
	ws.catchUp.running = false
	if err == nil {
		ws.catchUp.last = res
		ws.catchUp.haveOne = true
	}
	ws.catchUp.mu.Unlock()
	return res, err == nil
}

// StartCatchUp runs adaptive catch-up passes until ctx is done, and
// returns a stop function.
//
// The FIRST pass runs immediately, which is the restart case: a peer that
// was down while the other side was writing has no notification coming for
// anything it missed, so without an opening pass those changes wait for
// the next unrelated write to that path — which may be never.
//
// `base` (<= 0 means DefaultCatchUpInterval) is where the adaptive ramp
// STARTS, not a rate it holds: the interval drops to MinCatchUpInterval
// while passes are recovering files and backs off toward
// MaxCatchUpInterval once they stop. See nextCatchUpInterval.
//
// The pause is taken AFTER a pass completes, so a folder large enough to
// take longer than the interval spaces itself instead of stacking.
func (ws *ShellWorkspace) StartCatchUp(ctx context.Context, base time.Duration) (stop func()) {
	if base <= 0 {
		base = DefaultCatchUpInterval
	}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	ws.catchUp.mu.Lock()
	ws.catchUp.interval = base
	ws.catchUp.mu.Unlock()

	go func() {
		defer close(done)
		timer := time.NewTimer(0) // fire immediately
		defer timer.Stop()
		interval := base
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			res, ran := ws.RunCatchUpOnce(ctx)
			if ran {
				interval = nextCatchUpInterval(interval, res.Duration, res.Recovered)
			}
			ws.catchUp.mu.Lock()
			ws.catchUp.interval = interval
			ws.catchUp.mu.Unlock()
			timer.Reset(interval)
		}
	}()

	return func() {
		cancel()
		<-done
	}
}

// CatchUpInterval is the supervisor's current wait, and whether one is
// running.
//
// Surfaced because the adaptive rate is the difference between "this
// folder is being watched closely" and "this folder is checked twice an
// hour", and an operator looking at a stale folder needs to know which
// they are in. A rate a surface cannot report is one nobody can trust.
func (ws *ShellWorkspace) CatchUpInterval() (time.Duration, bool) {
	if ws == nil {
		return 0, false
	}
	ws.catchUp.mu.Lock()
	defer ws.catchUp.mu.Unlock()
	return ws.catchUp.interval, ws.catchUp.stop != nil
}

// EnableCatchUp starts the supervisor for the lifetime of the process
// and remembers how to stop it.
//
// context.Background() and not a caller's context on purpose: this is a
// process-lifetime loop owned by the workspace, and binding it to
// whatever context happened to be in scope at bootstrap is how a
// background loop ends up cancelled by a startup timeout and nobody
// notices for a week. Callers that need it stopped call StopCatchUp.
//
// Calling it twice replaces the running supervisor rather than adding a
// second one.
func (ws *ShellWorkspace) EnableCatchUp(interval time.Duration) {
	ws.StopCatchUp()
	stop := ws.StartCatchUp(context.Background(), interval)
	ws.catchUp.mu.Lock()
	ws.catchUp.stop = stop
	ws.catchUp.mu.Unlock()
}

// StopCatchUp stops the supervisor if one is running, and waits for the
// in-flight pass to finish. Safe to call when none is.
func (ws *ShellWorkspace) StopCatchUp() {
	if ws == nil {
		return
	}
	ws.catchUp.mu.Lock()
	stop := ws.catchUp.stop
	ws.catchUp.stop = nil
	ws.catchUp.mu.Unlock()
	if stop != nil {
		stop()
	}
}
