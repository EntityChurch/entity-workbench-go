package shellcmd

import (
	"testing"
	"time"
)

// The adaptive rate is a pure function of (current interval, how long the
// last pass took, how much it recovered), so it is tested without a clock
// — no sleeps, no flakes, and the anti-churn property is provable rather
// than sampled.
//
// Tier: unit (TESTING-STRATEGY §1).
func TestNextCatchUpInterval(t *testing.T) {
	base := DefaultCatchUpInterval

	tests := []struct {
		name      string
		current   time.Duration
		pass      time.Duration
		recovered int
		want      time.Duration
		why       string
	}{
		{
			name: "recovering drops straight to the floor",
			// Not a gradual ramp down: a pass that found lost files means
			// delivery is dropping RIGHT NOW, and the operator is looking
			// at an incomplete folder. Converging fast matters more than
			// the average rate.
			current: MaxCatchUpInterval, pass: time.Second, recovered: 42,
			want: MinCatchUpInterval,
			why:  "a pass that recovered files must not wait ten minutes to try again",
		},
		{
			name: "settled backs off geometrically",
			// A 3 s pass — a folder big enough that backing off buys
			// something. The ceiling is derived from the pass now
			// (settledCeiling), so this case has to be in the regime
			// where a doubling is actually permitted, or it is asserting
			// the cap rather than the ramp.
			current: base, pass: 3 * time.Second, recovered: 0,
			want: base * catchUpBackoffFactor,
			why:  "an idle folder should cost less over time, not the same forever",
		},
		{
			name: "back-off is capped by the HARD ceiling for an expensive folder",
			// 30 s per pass -> derived ceiling 50 min, clamped to the hard
			// 10-minute cap. This is the only regime the flat constant was
			// ever right for.
			current: MaxCatchUpInterval, pass: 30 * time.Second, recovered: 0,
			want: MaxCatchUpInterval,
			why:  "a folder must still be checked eventually, however long it has been quiet",
		},
		{
			name: "A CHEAP FOLDER RESTS AT THE BASE RATE, it does not climb to ten minutes",
			// The operator-visible one, and the reason settledCeiling
			// exists. A quarter-second pass is a ~1000-file folder. Under
			// the old flat ceiling this returned 120 s and kept climbing to
			// 10 minutes over the next few passes — so a change that missed
			// live delivery could sit unnoticed for ten minutes on a folder
			// where looking costs 0.24 s.
			current: base, pass: 240 * time.Millisecond, recovered: 0,
			want: base,
			why:  "backing off past the base rate must buy a real saving, and here it buys none",
		},
		{
			name: "the derived ceiling scales WITH the folder",
			// 2.4 s -> 10,000 files. Ceiling 4 minutes: it may climb past
			// the base rate, and not to ten minutes.
			current: 2 * time.Minute, pass: 2400 * time.Millisecond, recovered: 0,
			want: 4 * time.Minute,
			why:  "a folder expensive enough to be worth backing off from should back off further",
		},
		{
			name: "the ramp up from the floor is GRADUAL, not a snap to the base rate",
			// The gradient. Clamping the way up at `base` would jump
			// 5s -> 60s the instant a burst ended, discarding every rate
			// in between — and those are the useful ones while a copy is
			// still trickling in. `base` sets where the ramp starts; it is
			// not a floor.
			current: MinCatchUpInterval, pass: time.Second, recovered: 0,
			want: MinCatchUpInterval * catchUpBackoffFactor,
			why:  "a settled loop should ease off, not snap to the resting rate",
		},
		{
			name: "ANTI-CHURN: the wait is never shorter than the pass",
			// The load case. A 20 s pass with more still to recover would
			// otherwise re-run immediately, forever: it burns the peer and
			// it re-reads a moving target instead of letting the burst
			// settle. Capped at a 50% duty cycle.
			current: base, pass: 20 * time.Second, recovered: 100,
			want: 20 * time.Second,
			why:  "a supervisor must never occupy more than half the wall clock",
		},
		{
			name:    "ANTI-CHURN applies to the settled path too",
			current: base, pass: 5 * time.Minute, recovered: 0,
			want: 5 * time.Minute,
			why:  "a pass slower than the interval spaces itself in both regimes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextCatchUpInterval(tt.current, tt.pass, tt.recovered)
			if got != tt.want {
				t.Errorf("nextCatchUpInterval(%v, %v, %d) = %v, want %v\n  %s",
					tt.current, tt.pass, tt.recovered, got, tt.want, tt.why)
			}
		})
	}
}

// TestNextCatchUpInterval_ConvergesFromIdleToBusyInOneStep is the
// property the whole gradient exists for, stated as a property rather
// than a case: however far the loop has backed off, ONE recovering pass
// puts it back at the floor.
//
// The alternative design — symmetric ramps — is what makes a sync tool
// feel broken: it has backed off to ten minutes, a burst arrives, and it
// takes six passes and an hour to get interested again.
func TestNextCatchUpInterval_ConvergesFromIdleToBusyInOneStep(t *testing.T) {
	interval := DefaultCatchUpInterval

	// Back all the way off. The pass has to be EXPENSIVE (6 s -> a derived
	// ceiling of exactly MaxCatchUpInterval) or the loop now correctly
	// rests at the base rate and never reaches the hard ceiling at all —
	// which is settledCeiling working, not the ramp failing.
	for i := 0; i < 30; i++ {
		interval = nextCatchUpInterval(interval, 6*time.Second, 0)
	}
	if interval != MaxCatchUpInterval {
		t.Fatalf("after 30 idle passes the interval is %v, want the %v ceiling",
			interval, MaxCatchUpInterval)
	}

	// One pass that finds something.
	interval = nextCatchUpInterval(interval, 10*time.Millisecond, 1)
	if interval != MinCatchUpInterval {
		t.Errorf("one recovering pass left the interval at %v; it must return to the "+
			"%v floor in a single step, or a burst after a quiet spell waits out "+
			"an entire back-off ramp", interval, MinCatchUpInterval)
	}
}

// TestNextCatchUpInterval_DutyCycleIsBoundedUnderSustainedLoad is the
// anti-churn guarantee as an invariant over a long run, not one row.
//
// It is the property the operator asked for in plain words — "if you
// detect you're falling behind, don't churn on it" — so it is asserted as
// a property: over any sequence of passes, the loop is never scheduled to
// spend more than half its time passing.
func TestNextCatchUpInterval_DutyCycleIsBoundedUnderSustainedLoad(t *testing.T) {
	interval := DefaultCatchUpInterval
	// A folder big enough that every pass is slow, always recovering —
	// the sustained-burst regime.
	const pass = 30 * time.Second
	for i := 0; i < 50; i++ {
		interval = nextCatchUpInterval(interval, pass, 500)
		if interval < pass {
			t.Fatalf("pass %d: interval %v is shorter than the %v pass that "+
				"produced it — the supervisor would run back-to-back", i, interval, pass)
		}
	}
}
