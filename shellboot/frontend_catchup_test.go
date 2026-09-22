// frontend_catchup_test.go — does the catch-up supervisor actually reach
// the frontends an operator leaves running?
//
// Tier: integration (TESTING-STRATEGY §3).
//
// # Why this file exists
//
// The supervisor is wired in Bootstrap rather than per-frontend (AP67),
// off ReconcileOnStart, precisely so no frontend has to remember it. But
// "wired centrally" is a claim about a code path, and D19 says a claim
// about a path is not a claim about the operation. It had already gone
// wrong once in the other direction: the REPL reconciles by its own route
// and never sets that flag, so entity-shell — the surface most likely to
// be open while a directory is copying — was the one long-running
// frontend without catch-up, and the central wiring is exactly what made
// that invisible.
//
// The GUI reaches Bootstrap by a longer route still: C# decides the
// default, serializes it, and the bridge unmarshals a JSON blob into
// shellboot.Config. That is three links, and the middle one is AP49's
// shape — an undeclared or renamed DTO field is discarded in silence, by
// both System.Text.Json and encoding/json, with no warning of any kind.
// Nothing downstream would notice: a peer with no supervisor looks
// exactly like a peer that never needed one.
//
// So this asserts on the SERIALIZED FORM the frontend actually sends,
// not on a Config built in Go. A rename on either side breaks it.
package shellboot

import (
	"encoding/json"
	"testing"
	"time"

	"entity-workbench-go/shellcmd"
)

// guiConfigBlob is the JSON the Avalonia frontend sends to BridgeInit for
// a DEFAULT (persistent) launch — the shape produced by
// avalonia/frontend/Program.cs::ApplyDefaults + JsonSerializer.
//
// Storage is overridden to memory here; the flag under test is
// reconcile_on_start, and a test has no business writing to the
// developer's real ~/.entity. Every other field is as the frontend emits
// it, including the ones this test does not read, because a blob that has
// been trimmed to what the test cares about cannot fail on a field the
// frontend sends and Go has stopped accepting.
const guiConfigBlob = `{
  "identity": "",
  "create_identity": false,
  "alias": "",
  "storage": "memory",
  "storage_path": "",
  "listen": "",
  "listen_fallback": true,
  "advertise": "",
  "reconcile_on_start": true,
  "open_access": false
}`

func TestGUIConfigBlob_CarriesReconcileOnStartAcrossTheBridge(t *testing.T) {
	var cfg Config
	if err := json.Unmarshal([]byte(guiConfigBlob), &cfg); err != nil {
		t.Fatalf("the frontend's own config blob does not unmarshal into shellboot.Config: %v", err)
	}
	if !cfg.ReconcileOnStart {
		t.Fatal("reconcile_on_start did not survive the bridge's json.Unmarshal — " +
			"the frontend sends it and Go dropped it in silence (AP49). " +
			"Check the json tag on shellboot.Config.ReconcileOnStart against " +
			"the JsonPropertyName in avalonia/frontend/Program.cs::BridgeConfig.")
	}
}

// The GUI's route to a supervisor is PeerManager.Create, not Bootstrap
// directly — so the assertion is made through Create, which is what the
// bridge calls.
func TestPeerManagerCreate_StartsAndStopsTheCatchUpSupervisor(t *testing.T) {
	t.Run("a frontend that asks for reconcile-on-start gets the supervisor", func(t *testing.T) {
		var cfg Config
		if err := json.Unmarshal([]byte(guiConfigBlob), &cfg); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		cfg.LocalAlias = "gui-like"
		cfg.OpenAccess = true

		m := NewPeerManager(testAppID)
		defer m.ShutdownAll()

		h, err := m.Create(cfg)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		hp := m.Get(h)
		if hp == nil {
			t.Fatal("Get returned nil immediately after Create")
		}
		interval, running := hp.Workspace.CatchUpInterval()
		if !running {
			t.Fatal("no catch-up supervisor on a peer created with reconcile_on_start — " +
				"this frontend would lose files to a burst and never recover them, " +
				"silently, which is exactly the failure AP77 catalogues")
		}
		// The interval is deliberately NOT asserted against
		// DefaultCatchUpInterval here, and the first draft of this test
		// did exactly that. It failed 2 runs in 3 under full-suite load
		// and passed every time in isolation, which reads like a memory
		// or scheduling regression and is nothing of the kind: the
		// supervisor's first pass runs IMMEDIATELY, and a completed pass
		// re-derives the interval. So "the interval is still its
		// starting value" is a statement about whether a goroutine has
		// been scheduled yet — a race the adaptive rate is designed to
		// win, asserted as if it were a configuration fact.
		//
		// The ramp's starting point is a property of nextCatchUpInterval
		// and is gated there, as a pure function, without a clock.
		if interval <= 0 {
			t.Errorf("supervisor reports a non-positive interval (%v) — a surface "+
				"rendering this would tell an operator the folder is checked never",
				interval)
		}
	})

	// The control arm. Without it the positive assertion above is
	// satisfied by a supervisor that runs unconditionally, which would
	// mean --ephemeral and outbound-only tools quietly run a loop nobody
	// asked for.
	t.Run("control: a frontend that does not ask gets no supervisor", func(t *testing.T) {
		cfg := memCfg("no-reconcile")
		cfg.ReconcileOnStart = false

		m := NewPeerManager(testAppID)
		defer m.ShutdownAll()

		h, err := m.Create(cfg)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if _, running := m.Get(h).Workspace.CatchUpInterval(); running {
			t.Error("a supervisor is running on a peer that asked for neither " +
				"reconcile-on-start nor a catch-up interval")
		}
	})

	// An explicit negative interval is the documented way to get
	// reconcile-on-start WITHOUT the loop. If that stops working, a
	// caller who deliberately opted out is silently opted back in.
	t.Run("a negative interval turns it off explicitly", func(t *testing.T) {
		cfg := memCfg("reconcile-no-loop")
		cfg.ReconcileOnStart = true
		cfg.CatchUpInterval = -1

		m := NewPeerManager(testAppID)
		defer m.ShutdownAll()

		h, err := m.Create(cfg)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if _, running := m.Get(h).Workspace.CatchUpInterval(); running {
			t.Error("CatchUpInterval: -1 did not turn the supervisor off")
		}
	})
}

// Destroying a peer must stop its supervisor.
//
// Until 2026-09-07 it did not, and the leak is the kind that only shows
// up in the configuration the feature exists for: the loop is started on
// context.Background() so that a startup timeout cannot kill it, which
// also means nothing cancels it when a peer goes away. One goroutine per
// peer the operator ever removed, running passes against a closed store
// forever, invisible in `status` because a pass that errors is not
// recorded.
func TestPeerDestroy_StopsTheCatchUpSupervisor(t *testing.T) {
	cfg := memCfg("destroyed")
	cfg.ReconcileOnStart = true

	m := NewPeerManager(testAppID)
	defer m.ShutdownAll()

	h, err := m.Create(cfg)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	hp := m.Get(h)
	if _, running := hp.Workspace.CatchUpInterval(); !running {
		t.Fatal("precondition: no supervisor to stop — this test cannot see " +
			"what it is named after")
	}

	// Hold the workspace pointer: Destroy unregisters the handle, so
	// after it m.Get(h) is nil and there is nothing left to ask.
	ws := hp.Workspace

	if err := m.Destroy(h); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if _, running := ws.CatchUpInterval(); running {
		t.Error("the catch-up supervisor is still running after the peer that " +
			"owns it was destroyed — it is now taking passes against a closed " +
			"store, once per removed peer, for the life of the process")
	}
}

// ShutdownAll goes through Destroy, so it inherits the fix — but the
// process-exit path is the one every frontend uses and it is worth one
// assertion of its own, because a future Destroy that skips workspaces
// for a good reason would leave this uncovered.
func TestShutdownAll_StopsEverySupervisor(t *testing.T) {
	m := NewPeerManager(testAppID)

	var workspaces []*shellcmd.ShellWorkspace
	for _, alias := range []string{"one", "two", "three"} {
		cfg := memCfg(alias)
		cfg.ReconcileOnStart = true
		h, err := m.Create(cfg)
		if err != nil {
			t.Fatalf("Create %s: %v", alias, err)
		}
		workspaces = append(workspaces, m.Get(h).Workspace)
	}
	for i, ws := range workspaces {
		if _, running := ws.CatchUpInterval(); !running {
			t.Fatalf("precondition: peer %d has no supervisor", i)
		}
	}

	m.ShutdownAll()

	for i, ws := range workspaces {
		if _, running := ws.CatchUpInterval(); running {
			t.Errorf("peer %d's supervisor survived ShutdownAll", i)
		}
	}
}

// A supervisor that is stopped and restarted must not leave two loops
// running. EnableCatchUp documents that it replaces rather than adds;
// the manager now calls StopCatchUp during Destroy, so the two have to
// compose without the second call hanging on a stop that already ran.
func TestStopCatchUp_IsIdempotent(t *testing.T) {
	cfg := memCfg("idempotent")
	cfg.ReconcileOnStart = true

	m := NewPeerManager(testAppID)
	defer m.ShutdownAll()

	h, err := m.Create(cfg)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	ws := m.Get(h).Workspace

	done := make(chan struct{})
	go func() {
		defer close(done)
		ws.StopCatchUp()
		ws.StopCatchUp()
		ws.StopCatchUp()
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("StopCatchUp hung on a repeat call — a Destroy after an " +
			"explicit stop would block the whole shutdown")
	}
}

// LongRunning is the flag that answers the supervisor's actual question,
// and its whole reason for existing is the configuration ReconcileOnStart
// cannot reach: an in-memory peer.
//
// That peer has nothing to re-establish at startup — correctly, so
// ReconcileOnStart is false — and it can still accept a share mid-session,
// take a burst, and lose files. It is also the ONE configuration in which
// the loss is final, because there is no next launch to recover in.
//
// Three arms, because the additive rule is the part that can go wrong:
// LongRunning alone must start it, an explicit negative interval must
// still win, and a peer that asks for neither must still get nothing —
// the last one is what keeps the several hundred peers the suites build
// through Bootstrap out of a loop nobody asked for.
func TestLongRunning_StartsTheSupervisorWithoutDeclarations(t *testing.T) {
	t.Run("an in-memory long-running frontend gets the supervisor", func(t *testing.T) {
		cfg := memCfg("ephemeral-gui")
		cfg.ReconcileOnStart = false
		cfg.LongRunning = true

		m := NewPeerManager(testAppID)
		defer m.ShutdownAll()

		h, err := m.Create(cfg)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if _, running := m.Get(h).Workspace.CatchUpInterval(); !running {
			t.Fatal("no supervisor on a peer that declared itself long-running — " +
				"this is the --ephemeral GUI, the one configuration where a " +
				"burst loses files permanently because there is no next launch")
		}
	})

	t.Run("an explicit negative interval still wins", func(t *testing.T) {
		cfg := memCfg("long-no-loop")
		cfg.LongRunning = true
		cfg.CatchUpInterval = -1

		m := NewPeerManager(testAppID)
		defer m.ShutdownAll()

		h, err := m.Create(cfg)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if _, running := m.Get(h).Workspace.CatchUpInterval(); running {
			t.Error("LongRunning overrode an explicit CatchUpInterval: -1 — " +
				"a caller who deliberately opted out is silently opted back in")
		}
	})

	// The control arm for the ADDITIVE choice. Widening the rule to "every
	// peer" would satisfy the first arm and put a supervisor behind every
	// test peer in the tree, which is the change this flag exists to avoid
	// making.
	t.Run("control: neither flag still means no supervisor", func(t *testing.T) {
		cfg := memCfg("neither")

		m := NewPeerManager(testAppID)
		defer m.ShutdownAll()

		h, err := m.Create(cfg)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if _, running := m.Get(h).Workspace.CatchUpInterval(); running {
			t.Error("a supervisor is running on a peer that asked for neither " +
				"long-running nor reconcile-on-start — the catch-up loop has " +
				"become unconditional and every suite in this repo now runs one")
		}
	})
}

// The GUI's blob must carry it. Same AP49 hazard as reconcile_on_start:
// C# names the field, System.Text.Json serializes it, encoding/json
// unmarshals it, and a mismatch at either end is silent on both sides.
//
// The blob below is what ApplyDefaults produces for an `--ephemeral`
// launch — the case the flag exists for, so a test using the persistent
// blob would pass while the configuration under test stayed broken.
const guiEphemeralConfigBlob = `{
  "identity": "",
  "create_identity": false,
  "alias": "",
  "storage": "memory",
  "storage_path": "",
  "listen": "",
  "listen_fallback": false,
  "advertise": "",
  "reconcile_on_start": false,
  "long_running": true,
  "open_access": false
}`

func TestGUIEphemeralBlob_CarriesLongRunningAcrossTheBridge(t *testing.T) {
	var cfg Config
	if err := json.Unmarshal([]byte(guiEphemeralConfigBlob), &cfg); err != nil {
		t.Fatalf("the frontend's ephemeral config blob does not unmarshal: %v", err)
	}
	if cfg.ReconcileOnStart {
		t.Error("precondition: the ephemeral blob sets reconcile_on_start — " +
			"this test cannot see what it is named after")
	}
	if !cfg.LongRunning {
		t.Fatal("long_running did not survive the bridge's json.Unmarshal — " +
			"the frontend sends it and Go dropped it in silence (AP49). " +
			"Check the json tag on shellboot.Config.LongRunning against the " +
			"JsonPropertyName in avalonia/frontend/Program.cs::BridgeConfig.")
	}
}

// The recording growth guard is attached for EVERY peer, and detaches
// when one is destroyed.
//
// Unlike the supervisor it hangs off no frontend flag, because unbounded
// recording is not work a peer schedules — it is a consequence of a mount
// existing, accruing at whatever rate something else writes. A peer that
// runs for an hour with a log file in a shared folder has written a
// gigabyte whether or not it meant to stay up.
func TestPeerManagerCreate_AttachesTheRecordingGuard(t *testing.T) {
	m := NewPeerManager(testAppID)
	defer m.ShutdownAll()

	h, err := m.Create(memCfg("guarded"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	hp := m.Get(h)
	st := hp.Workspace.HistoryBudget()
	if !st.Running {
		t.Fatal("no recording guard on a freshly created peer — a file something " +
			"rewrites continuously will grow the tree without bound and nothing " +
			"will notice")
	}
	if st.Budget == 0 {
		t.Errorf("guard is running with a zero budget — every path would trip at once")
	}

	ws := hp.Workspace
	if err := m.Destroy(h); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if ws.HistoryBudget().Running {
		t.Error("the recording guard is still attached after the peer that owns " +
			"it was destroyed — its prefix watch holds the closed store alive " +
			"through the SDK's watch hub, once per removed peer")
	}
}

// A negative budget turns it off, which is the only way to re-measure the
// unbounded growth the guard exists to stop. A guard with no off switch
// makes its own A/B impossible.
func TestHistoryPathBudget_NegativeTurnsTheGuardOff(t *testing.T) {
	cfg := memCfg("unguarded")
	cfg.HistoryPathBudget = -1

	m := NewPeerManager(testAppID)
	defer m.ShutdownAll()

	h, err := m.Create(cfg)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if m.Get(h).Workspace.HistoryBudget().Running {
		t.Error("HistoryPathBudget: -1 did not turn the recording guard off")
	}
}
