package shellboot_test

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"go.entitychurch.org/entity-core-go/core/ecf"
	"go.entitychurch.org/entity-core-go/core/protocol"
	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/shellboot"
)

// PROBE — an unreachable peer writes a permanent entity into our tree every
// ~15 seconds. Why, and is it bounded?
//
// # What was observed, and what was NOT established
//
// On the 2026-09-08 two-machine session, one peer was unreachable for 45
// minutes. In that window the local tree gained **188** chain-error markers,
// one every ~15 s, every one of them:
//
//	path       system/runtime/chain-errors/lost/chain-<id>/notif-sub-<id>/not_found/<hash>
//	failed_uri system/network
//	status     404   code "not_found"
//
// Extrapolated: ~5,700 overnight, ~40,000 for a laptop shut for a week. That
// much is established. **Why `system/network` answers 404 is NOT**, and the
// handoff was explicit that it must not be routed to core-go as a defect
// until somebody instruments it. This is that instrument.
//
// # Three questions, and why each needs the answer written down
//
//  1. **Is it real tree growth, or just log noise?** The operator saw it in a
//     log, and a log line is cheap. `bindLostMarker` does `store.Put` plus a
//     location-index bind, so the hypothesis is that these are real entities —
//     but "I read the code" is not a measurement (D19). Count the tree.
//
//  2. **Why 404?** Two hypotheses were already killed by reading, which is
//     exactly why they had to be: an unknown operation on the network handler
//     returns **501 unsupported_operation**, not 404, so neither the
//     `on_error` route to the backoff path nor a stray operation name can
//     produce this code. The marker body carries `target_uri`, `status` and
//     `code` verbatim — so the probe prints them rather than guessing again.
//
//  3. **Is it bounded?** core-go ships a real collector
//     (`protocol.CollectExpiredMarkers`, 24 h default, operator knob at
//     `system/config/chain-errors` → `retention_ms`) — and, unlike the history
//     `max_depth` case which pruned nothing, this one removes. The catch is
//     that collection is **bind-time and self-triggered**: the design is
//     stated as *"the path that grows this tree is exactly the path that reaps
//     it."* There are THREE binders and only two call a sweep. So the question
//     is whether the binder that fires here is a sweeping one.
//
// # This probe asserts almost nothing
//
// It is a measurement, not a gate. It prints what it finds and fails only on
// the one thing already established (that markers accumulate at all) — because
// a probe that asserts a number nobody has justified turns an observation into
// a requirement, which is the mistake AP81 catalogues. Run it with `-v`.
func TestProbe_UnreachablePeerChainErrorGrowth(t *testing.T) {
	if testing.Short() {
		t.Skip("probe: needs a real maintain-peer lifecycle and wall-clock time")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	ap, _, err := shellboot.Bootstrap(ctx, shellboot.Config{
		LocalAlias: "observer", ListenAddr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	defer ap.Close()

	before := countMarkers(t, ap)
	t.Logf("markers before maintain-peer: %d", before)

	// A peer-id that names nobody, at a port nothing listens on. This is the
	// operator's state exactly: a declared relationship with a machine that is
	// switched off.
	//
	// The peer-id must be well-formed or the failure happens in parsing rather
	// than in the lifecycle, which is a different code path and would measure
	// the wrong thing.
	const deadPeer = "2KLf7osYcMLEdmSnYLx3Bg6TScaGJefLZJdKaL1tPNHAbR"
	res, err := ap.Network().MaintainPeer(ctx, deadPeer, entitysdk.MaintainOpts{
		Address: "127.0.0.1:1",
	})
	t.Logf("maintain-peer returned: session=%q err=%v", sessionOf(res), err)

	// THE finding of this arm, asserted rather than waited for: a peer that
	// was never reachable gets no session, therefore no continuation graph and
	// no lifecycle subscriptions — so it is structurally incapable of
	// producing the operator's markers. That is what sends the investigation
	// to the restart boundary in arm 3.
	if sessionOf(res) != "" {
		t.Fatalf("maintain-peer created session %q against a peer that was never reachable — "+
			"if this now succeeds, core-go changed the lifecycle and arm 3's root cause "+
			"needs re-measuring, not just re-reading", sessionOf(res))
	}

	// Short on purpose. Arm 1's finding is in the RETURN VALUE — no session id
	// means no lifecycle installed, so there is nothing that could bind a
	// marker and no amount of waiting changes that. An earlier version waited
	// 75 s to observe the same zero, which is 75 s of sweep time buying a
	// weaker form of an assertion already available synchronously.
	//
	// The loop sleeps BEFORE each sample, so a 20 s budget takes two passes and
	// spends 30 s of wall clock. Named rather than tuned: the number in the log
	// line below is the budget, not the elapsed time, and a reader comparing the
	// two should not have to re-derive that.
	const observe = 20 * time.Second
	deadline := time.Now().Add(observe)
	var samples []int
	for time.Now().Before(deadline) {
		time.Sleep(15 * time.Second)
		n := countMarkers(t, ap)
		samples = append(samples, n)
		t.Logf("  t+%-3.0fs  markers=%d", time.Since(deadline.Add(-observe)).Seconds(), n)
	}

	after := countMarkers(t, ap)
	grew := after - before
	t.Logf("markers after %s: %d (grew by %d)", observe, after, grew)

	// What the markers actually SAY. This is the part the handoff asked for:
	// the body carries target_uri / status / code verbatim, so the 404 either
	// names itself here or the hypothesis that this is the same family is
	// wrong — and either outcome is worth more than another reading of the
	// source.
	for _, line := range describeMarkers(t, ap, 8) {
		t.Logf("  marker: %s", line)
	}

	if grew == 0 {
		t.Logf("NO GROWTH in this configuration — the 2026-09-08 shape did not reproduce. " +
			"That is a finding, not a pass: it means the trigger needs something this " +
			"fixture lacks (an established-then-lost connection, a live subscription " +
			"delivering to the lifecycle inbox, or a declared folder). Do not conclude " +
			"the growth is fixed.")
	}
}

// PROBE arm 2 — ESTABLISHED, then LOST. This is the operator's shape.
//
// Arm 1 (above) measured zero growth and named why: `maintain-peer` against a
// peer that was never reachable returns **502 connection_failed with no
// session id**, so the lifecycle never installs — no continuation graph, no
// lifecycle subscriptions, nothing that could bind a marker. A peer that has
// never been reachable is not the case that grows the tree.
//
// The 2026-09-08 machine was different: the two peers HAD been talking, so the
// graph and its subscriptions were tree-resident and (since the subscription
// restore fix) survive a restart. The trigger is therefore a session that
// exists and then stops working, which is what this arm builds.
func TestProbe_EstablishedThenLostChainErrorGrowth(t *testing.T) {
	if testing.Short() {
		t.Skip("probe: needs two real peers and wall-clock time")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	a, _, err := shellboot.Bootstrap(ctx, shellboot.Config{
		LocalAlias: "observer", ListenAddr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("bootstrap observer: %v", err)
	}
	defer a.Close()

	b, _, err := shellboot.Bootstrap(ctx, shellboot.Config{
		LocalAlias: "target", ListenAddr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("bootstrap target: %v", err)
	}

	// The listener needs explicit bring-up — Bootstrap wires the config but
	// does not bind in-process, which is why b.Addr() was nil on the first
	// run of this probe.
	bringUpListener(t, ctx, b, "target")
	bAddr := b.Addr().String()
	t.Logf("target listening at %s (peer %s)", bAddr, b.PeerID())

	// Establish for real, so the lifecycle graph and its subscriptions exist.
	res, err := a.Network().MaintainPeer(ctx, b.PeerID(), entitysdk.MaintainOpts{Address: bAddr})
	if err != nil {
		t.Fatalf("maintain-peer while the target is UP failed (%v) — without an established "+
			"session this arm measures the same nothing arm 1 did", err)
	}
	if res.SessionID == "" {
		t.Fatal("maintain-peer returned no session id while the target was up — the lifecycle " +
			"did not install, so this arm cannot reproduce the operator's shape")
	}
	t.Logf("session established: %s", res.SessionID)

	established := countMarkers(t, a)
	t.Logf("markers with the session HEALTHY: %d", established)
	dumpLifecycle(t, a, "while HEALTHY")

	// Now take the target away. This is "the other laptop was switched off".
	if err := b.Close(); err != nil {
		t.Logf("closing target: %v", err)
	}
	t.Log("target closed — observing the observer's tree")

	// 30 s, not 90. The finding is that a status TRANSITION happened (the
	// peer-status entity's hash changes across the dump either side) and no
	// marker was bound — which is visible as soon as the disconnect is
	// noticed, not after three minutes of confirming the same zero.
	const observe = 30 * time.Second
	start := time.Now()
	for time.Since(start) < observe {
		time.Sleep(15 * time.Second)
		t.Logf("  t+%-3.0fs  markers=%d", time.Since(start).Seconds(), countMarkers(t, a))
	}

	dumpLifecycle(t, a, "after the peer went away")

	after := countMarkers(t, a)
	grew := after - established
	rate := float64(grew) / observe.Seconds() * 60
	t.Logf("markers after %s with the peer gone: %d (grew by %d — %.1f/min)", observe, after, grew, rate)

	for _, line := range describeMarkers(t, a, 10) {
		t.Logf("  marker: %s", line)
	}

	switch {
	case grew == 0:
		t.Log("STILL NO GROWTH. The operator's 188-in-45-minutes did not reproduce from " +
			"establish-then-lose alone. Something else is required — most likely a " +
			"SUBSCRIPTION delivering across the dead link (a shared folder), which is the " +
			"one ingredient this fixture still lacks. Do not close the question on this.")
	default:
		t.Logf("REPRODUCED at ~%.1f markers/min. Operator observed ~4.2/min (188 in 45min).", rate)
	}
}

// dumpLifecycle prints the PRECONDITIONS for a marker, so "no growth" can be
// told apart from "the machinery never ran".
//
// This is the control the first two arms lacked. Silence from a new instrument
// is a claim about the instrument first (AP76): before "no markers" becomes a
// finding, show that the thing which produces markers was actually present and
// firing. Subscriptions installed, a peer-status entity to trigger them, and an
// inbox continuation to deliver into are the three, and if any is absent the
// zero above measures nothing at all.
func dumpLifecycle(t *testing.T, ap *entitysdk.AppPeer, when string) {
	t.Helper()
	st := ap.Store()
	for _, prefix := range []string{
		"system/subscription/",
		"system/peer/status/",
		"system/inbox/network/",
		"system/network/peers/",
	} {
		entries := st.List(prefix)
		t.Logf("  [%s] %-26s %d", when, prefix, len(entries))
		for i, e := range entries {
			if i >= 3 {
				t.Logf("  [%s]   … and %d more", when, len(entries)-3)
				break
			}
			// The HASH, not just the path. A peer-status entity is a
			// SAME-PATH OVERWRITE, so counting paths cannot see a
			// transition at all — the count is 1 before and 1 after
			// however many times the status changed. The first version of
			// this dump printed only the count and I read "1 and 1" as
			// "nothing fired", which is the instrument answering a
			// different question than the one asked (AP76).
			t.Logf("  [%s]   %s  hash=%x", when, shortPath(e.Path), e.Hash.Bytes()[:6])
		}
	}
}

// countMarkers counts entities bound under the chain-error marker root.
//
// Store(), not a dispatched read: these are OUR markers in OUR tree, and a
// peer-qualified path would be a remote read (AP11) that dispatches to
// ourselves through a different authority path.
func countMarkers(t *testing.T, ap *entitysdk.AppPeer) int {
	t.Helper()
	return len(ap.Store().List("system/runtime/chain-errors/"))
}

// describeMarkers decodes up to max markers into one line each, so the
// question "why 404" is answered by the record rather than by inference.
func describeMarkers(t *testing.T, ap *entitysdk.AppPeer, max int) []string {
	t.Helper()
	st := ap.Store()
	entries := st.List("system/runtime/chain-errors/")
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })

	var out []string
	for _, e := range entries {
		if len(out) >= max {
			out = append(out, "… and more")
			break
		}
		ent, ok := st.Get(e.Path)
		if !ok {
			continue
		}
		var body types.ChainErrorLostData
		if err := ecf.Decode(ent.Data, &body); err != nil {
			out = append(out, e.Path+"  (undecodable: "+err.Error()+")")
			continue
		}
		out = append(out, strings.Join([]string{
			"reason=" + body.Reason,
			"target_uri=" + body.TargetURI,
			"code=" + body.Code,
			"step=" + body.StepIndex,
			"path=" + shortPath(e.Path),
		}, "  "))
	}
	return out
}

// assertOperatorMarkerShape checks that at least one marker in the tree is the
// one the 2026-09-08 operator log carried, field by field.
//
// Deliberately "at least one" and never "all": the fixture's own bootstrap can
// legitimately bind other chain-error markers, and a test that fails because
// something ELSE also went wrong is a test that reports the wrong defect.
func assertOperatorMarkerShape(t *testing.T, ap *entitysdk.AppPeer) {
	t.Helper()
	st := ap.Store()
	var seen []string
	for _, e := range st.List("system/runtime/chain-errors/") {
		ent, ok := st.Get(e.Path)
		if !ok {
			continue
		}
		var body types.ChainErrorLostData
		if err := ecf.Decode(ent.Data, &body); err != nil {
			continue
		}
		seen = append(seen, body.TargetURI+"/"+body.Code+"/"+body.StepIndex)
		if body.TargetURI == "system/network" &&
			body.Code == "not_found" &&
			strings.HasPrefix(body.StepIndex, "notif-") {
			return
		}
	}
	t.Errorf("a marker was bound after the restart, but none has the operator's shape "+
		"(target_uri=system/network code=not_found step=notif-*). Saw: %v.\n"+
		"This is not a pass with a cosmetic difference — the three fields ARE the "+
		"identification of the mechanism, and the packet routed to core-go quotes them.", seen)
}

func shortPath(p string) string {
	// The marker hash is 66 chars of no interest when reading a list.
	if i := strings.LastIndex(p, "/"); i > 0 && len(p)-i > 20 {
		return p[:i] + "/…"
	}
	return p
}

func sessionOf(res types.MaintainResultData) string { return res.SessionID }

// PROBE arm 3 — THE CAUSE. A durable subscription firing at a volatile session.
//
// Arms 1 and 2 both measured zero, and between them they name the missing
// ingredient precisely.
//
// Reading the two 404 sites in the network handler
// (`ext/network/maintain.go:456` and `:525`) shows they are the SAME
// condition, and it is the only `404 not_found` the handler can produce:
//
//	sess := h.getSession(peerID)
//	if sess == nil {
//	    return handler.NewErrorResponse(404, "not_found", "no maintain session for peer "+…)
//	}
//
// and `h.sessions` is an in-memory map on the handler
// (`ext/network/handler.go:63`).
//
// So the two halves of the reconnect lifecycle have different lifetimes:
//
//	the continuation graph      system/inbox/network/{peer}/*   TREE — durable
//	the lifecycle subscriptions system/subscription/{id}        TREE — durable
//	                                                            (and restored
//	                                                            at open since
//	                                                            the Engine.Load fix)
//	the maintain SESSION        h.sessions                      MEMORY — dies
//	                                                            with the process
//
// After a restart the durable half is fully alive and the thing it dispatches
// into is empty. Every peer-status transition then fires a subscription →
// advances a continuation → dispatches `system/network:reconnect` → 404
// not_found → and `advance.go` binds a permanent `lost` marker under the
// notification's request id, which is why every marker in the operator's log
// was `notif-sub-*` / `system/network` / 404 / `not_found`.
//
// **This is D26 / AP62 one layer out** — a derived runtime index over a
// persistent store that nothing rebuilds at open — except that here the
// unrebuilt state is not merely unread: a durable subscription keeps firing at
// it and every miss is written down forever.
//
// It also explains why neither earlier arm reproduced: both ran in ONE process
// with a warm session map, which is the one configuration where this cannot
// happen.
func TestProbe_RestartMakesTheLifecycleFireIntoAHole(t *testing.T) {
	if testing.Short() {
		t.Skip("probe: restarts a real peer against a persistent store")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	dir := t.TempDir()
	storePath := dir + "/observer.db"
	// HOME is redirected because `Identity` is a NAME resolved under
	// ~/.entity/identities, not a path — without this the probe writes a
	// keypair into the developer's real identity store.
	t.Setenv("HOME", dir)

	// The target stays up the whole time. The defect is on the OBSERVER's
	// side and has nothing to do with the far peer being gone — which is
	// worth establishing, because "the other machine was off" is the obvious
	// and wrong explanation.
	b, _, err := shellboot.Bootstrap(ctx, shellboot.Config{
		LocalAlias: "target", ListenAddr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("bootstrap target: %v", err)
	}
	defer b.Close()
	bringUpListener(t, ctx, b, "target")
	bAddr := b.Addr().String()

	// --- launch 1: establish, so the graph and the subscriptions land ---
	a1, _, err := shellboot.Bootstrap(ctx, shellboot.Config{
		LocalAlias: "observer", ListenAddr: "127.0.0.1:0",
		StorageKind: "sqlite", StoragePath: storePath,
		CreateIdentity: true, Identity: "probe-observer",
	})
	if err != nil {
		t.Fatalf("bootstrap observer (launch 1): %v", err)
	}
	bringUpListener(t, ctx, a1, "observer-1")

	res, err := a1.Network().MaintainPeer(ctx, b.PeerID(), entitysdk.MaintainOpts{Address: bAddr})
	if err != nil || res.SessionID == "" {
		_ = a1.Close()
		t.Fatalf("maintain-peer in launch 1 did not establish (err=%v session=%q) — "+
			"without a session the graph never installs and this arm measures nothing",
			err, res.SessionID)
	}
	t.Logf("launch 1: session %s established", res.SessionID)
	dumpLifecycle(t, a1, "launch 1")
	beforeRestart := countMarkers(t, a1)
	t.Logf("launch 1 markers: %d", beforeRestart)

	if err := a1.Close(); err != nil {
		t.Logf("closing launch 1: %v", err)
	}

	// --- launch 2: same store, same identity, NO maintain-peer re-issued ---
	//
	// This is the operator relaunching the app. The reconciler does re-issue
	// maintain-peer per launch, but only for DECLARED devices and only once a
	// pass runs — and in the window before that, or for any peer the pass
	// cannot reach, the graph is live and the session is not.
	a2, _, err := shellboot.Bootstrap(ctx, shellboot.Config{
		LocalAlias: "observer", ListenAddr: "127.0.0.1:0",
		StorageKind: "sqlite", StoragePath: storePath,
		Identity: "probe-observer",
	})
	if err != nil {
		t.Fatalf("bootstrap observer (launch 2): %v", err)
	}
	defer a2.Close()
	bringUpListener(t, ctx, a2, "observer-2")

	dumpLifecycle(t, a2, "launch 2 (before any transition)")
	restarted := countMarkers(t, a2)
	t.Logf("launch 2 markers at open: %d", restarted)

	// Now make a peer-status transition happen, which is what fires the
	// restored lifecycle subscription. The target dialling US is exactly the
	// operator's configuration: inbound worked, outbound did not.
	for i := 0; i < 3; i++ {
		if _, err := b.Connect(ctx, a2.Addr().String()); err != nil {
			t.Logf("  target dial %d: %v", i, err)
		}
		time.Sleep(5 * time.Second)
		t.Logf("  after target dial %d: markers=%d", i, countMarkers(t, a2))
	}
	time.Sleep(10 * time.Second)

	after := countMarkers(t, a2)
	t.Logf("markers after restart + %d inbound dials: %d (grew by %d)", 3, after, after-restarted)
	for _, line := range describeMarkers(t, a2, 10) {
		t.Logf("  marker: %s", line)
	}

	if after > restarted {
		t.Logf("REPRODUCED: the restored lifecycle dispatched into an empty session map. " +
			"Every marker should read target_uri=system/network code=not_found.")
		// The COUNT stays unasserted on purpose — nobody has justified a
		// number and pinning one turns an observation into a requirement
		// (AP81). The SHAPE is a different thing: three documents quote it as
		// measured, so it is asserted here rather than only printed.
		//
		// It is also the discriminator for WHICH 404 site fired.
		// `advance.go` binds this marker only when the continuation has no
		// on_error and sets {reason} to the failed op's `code` verbatim, so
		// reason=not_found reaches `restore-subscriptions` (no on_error) and
		// not `reconnect` (which has one, routing to the backoff path) or the
		// backoff itself (whose maintain-peer answers 502 connection_failed).
		// If this starts failing with reason=connection_failed or
		// on_error_dispatch_failed, the mechanism moved and the packet's §3
		// needs re-deriving, not re-wording.
		assertOperatorMarkerShape(t, a2)
	} else {
		t.Logf("NOT reproduced in this arm either. The reading of the two 404 sites still " +
			"stands on its own, but it is a reading and not a measurement — say so " +
			"before routing it anywhere.")
	}
}

// PROBE arm 4 — does the collector actually COLLECT?
//
// core-go ships `protocol.CollectExpiredMarkers` with a 24 h default and an
// operator knob at `system/config/chain-errors` → `retention_ms`. Reading it,
// it removes. **That is not sufficient evidence and this repo has been burned
// on exactly this shape**: `types.HistoryConfigData.MaxDepth` is documented
// *"max transitions per path"*, the recorder calls `prune` after every write,
// and it removes **nothing** — twelve writes at `max_depth=3` left twelve
// transitions reachable, and core-go's own test asserted `count >= maxDepth`,
// which passes with the feature deleted.
//
// So before any mitigation is built on this knob, run it: set a retention of
// effectively zero, bind a marker, and assert the earlier ones are gone.
//
// It matters because the two possible worlds are very different. If collection
// works, the growth is BOUNDED at one retention window and the operator knob is
// a real lever. If it does not, the growth is unbounded and the only remaining
// lever is not producing the markers at all.
func TestProbe_ChainErrorCollectorActuallyCollects(t *testing.T) {
	if testing.Short() {
		t.Skip("probe: needs a real restart to produce markers")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	dir := t.TempDir()
	t.Setenv("HOME", dir)
	storePath := dir + "/observer.db"

	b, _, err := shellboot.Bootstrap(ctx, shellboot.Config{
		LocalAlias: "target", ListenAddr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("bootstrap target: %v", err)
	}
	defer b.Close()
	bringUpListener(t, ctx, b, "target")

	a1, _, err := shellboot.Bootstrap(ctx, shellboot.Config{
		LocalAlias: "observer", ListenAddr: "127.0.0.1:0",
		StorageKind: "sqlite", StoragePath: storePath,
		CreateIdentity: true, Identity: "probe-collector",
	})
	if err != nil {
		t.Fatalf("bootstrap observer (launch 1): %v", err)
	}
	bringUpListener(t, ctx, a1, "observer-1")
	res, err := a1.Network().MaintainPeer(ctx, b.PeerID(), entitysdk.MaintainOpts{Address: b.Addr().String()})
	if err != nil || res.SessionID == "" {
		_ = a1.Close()
		t.Fatalf("launch 1 did not establish (err=%v session=%q)", err, res.SessionID)
	}
	_ = a1.Close()

	a2, _, err := shellboot.Bootstrap(ctx, shellboot.Config{
		LocalAlias: "observer", ListenAddr: "127.0.0.1:0",
		StorageKind: "sqlite", StoragePath: storePath,
		Identity: "probe-collector",
	})
	if err != nil {
		t.Fatalf("bootstrap observer (launch 2): %v", err)
	}
	defer a2.Close()
	bringUpListener(t, ctx, a2, "observer-2")

	// Produce a marker the way arm 3 established.
	if _, err := b.Connect(ctx, a2.Addr().String()); err != nil {
		t.Logf("target dial: %v", err)
	}
	time.Sleep(8 * time.Second)
	produced := countMarkers(t, a2)
	t.Logf("markers produced after restart: %d", produced)
	if produced == 0 {
		// A skip counts as a failure (AGENTS-STANDARD), and this one has a
		// consequence worth naming rather than swallowing: this arm is the ONLY
		// measurement behind "the growth is bounded at one retention window",
		// which STATUS.md, AGENTS.md and the routed packet all state as fact.
		// A silent skip retires that claim without anybody noticing.
		t.Skip("SKIPPED, AND THE BOUNDEDNESS CLAIM IS UNMEASURED ON THIS RUN: no marker was " +
			"produced, so there is nothing to collect. Arm 3 is this arm's precondition and " +
			"it did not hold. Do not read a green suite as re-confirming that retention_ms " +
			"is a real lever — re-run arm 3 first and find out why it stopped reproducing.")
	}

	// Retention of 1 ms: everything already bound is expired.
	if _, err := a2.Store().Put("system/config/chain-errors", "system/config/chain-errors",
		map[string]interface{}{"retention_ms": uint64(1)}); err != nil {
		t.Fatalf("write retention config: %v", err)
	}
	t.Log("wrote system/config/chain-errors retention_ms=1")

	// Call the collector DIRECTLY rather than trying to trigger it.
	//
	// The first version of this arm produced more peer-status *dials* and
	// waited out the throttle — and returned INCONCLUSIVE, because a peer that
	// is already connected does not transition, so no further marker bound and
	// no bind-time sweep ran. That is a test whose negative result means
	// nothing, which is the thing to avoid when the question is "does this
	// mechanism work at all".
	//
	// `protocol.CollectExpiredMarkers` is exported and is the exact function
	// both in-tree binders call, so invoking it answers the question with no
	// clock and no trigger: given expired markers, does it remove them?
	nowMs := uint64(time.Now().UnixMilli()) + 60_000
	collected := protocol.CollectExpiredMarkers(a2.Store().ContentStore(), a2.RawLocationIndex(), 1, nowMs)
	after := countMarkers(t, a2)
	t.Logf("CollectExpiredMarkers(retention=1ms) reported %d collected; markers %d → %d",
		collected, produced, after)

	switch {
	case collected > 0 && after < produced:
		t.Logf("COLLECTOR WORKS: it reported %d and the tree shrank %d → %d. The growth is "+
			"BOUNDED at one retention window, and retention_ms is a real lever the "+
			"application tier can set.", collected, produced, after)
	case collected > 0 && after >= produced:
		t.Errorf("collector REPORTED %d collected and the tree did not shrink (%d → %d) — "+
			"a count that is not a removal is the history max_depth shape exactly, and "+
			"nothing may be built on this knob", collected, produced, after)
	default:
		t.Errorf("collector removed NOTHING with every marker expired (retention 1ms, now "+
			"pushed 60s forward). If this reproduces, retention_ms is a no-op like "+
			"history max_depth and the only remaining lever is not producing the "+
			"markers at all: %d markers still present", after)
	}
}
