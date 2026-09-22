package shellcmd

import (
	"strings"
	"testing"

	"entity-workbench-go/workbench"

	"go.entitychurch.org/entity-core-go/core/types"
)

// delivery_failures_test.go — the failures the tree recorded and nothing
// read.
//
// Tier: model (TESTING-STRATEGY §2).
//
// # What is being gated
//
// Not "DeliveryFailures returns rows". Three properties, each of which
// was violated by the absence this file's subject replaces:
//
//  1. a refused delivery REACHES a surface at all;
//  2. reconnect-lifecycle noise does NOT, or the real one is buried;
//  3. and the filtering is REPORTED, so the number here can be reconciled
//     with what `inspect` shows instead of quietly disagreeing with it.
//
// Property 2 is the one with teeth. A peer we cannot reach writes one
// marker per status transition from a separate, routed defect; at the
// observed rate that is thousands a day. A diagnostic that surfaces them
// is worse than no diagnostic, because an operator learns to skip it and
// then skips the one line that mattered.

func bindLostMarker(t *testing.T, st *workbench.Store, path string, d types.ChainErrorLostData) {
	t.Helper()
	if _, err := st.Put(path, types.TypeChainErrorLost, d); err != nil {
		t.Fatalf("bind marker at %s: %v", path, err)
	}
}

// TestDeliveryFailures_ARefusedFetchIsReported is the morning of
// 2026-09-10 in one test: every cross-peer blob fetch answered 403, a
// marker was bound for each, and no surface in the product said a word.
func TestDeliveryFailures_ARefusedFetchIsReported(t *testing.T) {
	_, st := reconcileFixture(t)

	bindLostMarker(t, st, chainErrorPrefix+"lost/chain-a/req-1/capability_denied/aa", types.ChainErrorLostData{
		Code:         "capability_denied",
		Status:       403,
		TargetPeerID: themPeer,
		TargetURI:    "entity://" + themPeer + "/system/content",
		Reason:       "capability_denied",
		Timestamp:    1_757_000_000_000,
	})

	rep := DeliveryFailures(st)
	if !rep.Observable {
		t.Fatal("the chain-error subtree could not be read at all")
	}
	if rep.Total != 1 {
		t.Fatalf("expected 1 recorded transfer failure, got %d", rep.Total)
	}

	lines := rep.Problems()
	if len(lines) == 0 {
		t.Fatal("a refused delivery produced NO problem line.\n" +
			"  This is the audited defect: the kernel binds a marker for every " +
			"failed fetch, and until this existed the only reader in the tree " +
			"was `inspect watch`. An operator watched a healthy-looking panel " +
			"for a morning while the diagnosis sat in their own store.")
	}
	line := lines[0]
	if !strings.Contains(line, "capability_denied") {
		t.Errorf("the line does not carry the CODE, which is the actionable half: %q", line)
	}
	if !strings.Contains(line, themPeer) {
		t.Errorf("the line does not name the peer: %q", line)
	}
	// The code decides what an operator should do next, so the line has
	// to say. "delivery failed" sends them to us; "they have not
	// authorized us" sends them to the other machine.
	if !strings.Contains(line, "authoriz") {
		t.Errorf("a capability refusal does not tell the operator what it means: %q", line)
	}
}

// TestDeliveryFailures_ReconnectNoiseIsExcludedAndCounted is the arm that
// keeps the feature useful.
//
// Without it the obvious implementation — surface every marker — passes
// the test above and buries the real fault under thousands of identical
// lines from a defect that is not ours and not about a file.
func TestDeliveryFailures_ReconnectNoiseIsExcludedAndCounted(t *testing.T) {
	_, st := reconcileFixture(t)

	// The shape an unreachable peer produces, once per status transition.
	for _, id := range []string{"aa", "bb", "cc"} {
		bindLostMarker(t, st, chainErrorPrefix+"lost/chain-n"+id+"/req-1/not_found/"+id,
			types.ChainErrorLostData{
				Code:      "not_found",
				Status:    404,
				TargetURI: "entity://" + themPeer + "/system/network",
				Reason:    "not_found",
				Timestamp: 1_757_000_000_001,
			})
	}
	// And one that IS about a file.
	bindLostMarker(t, st, chainErrorPrefix+"lost/chain-real/req-1/capability_denied/zz",
		types.ChainErrorLostData{
			Code:         "capability_denied",
			Status:       403,
			TargetPeerID: themPeer,
			TargetURI:    "entity://" + themPeer + "/system/content",
			Reason:       "capability_denied",
			Timestamp:    1_757_000_000_002,
		})

	rep := DeliveryFailures(st)
	if rep.Total != 1 {
		t.Fatalf("expected exactly 1 file-transfer failure among 4 markers, got %d.\n"+
			"  Three of them are reconnect-lifecycle markers aimed at system/network "+
			"— a routed defect that is not about a file. Counting them makes the "+
			"number meaningless at the rate they are produced.", rep.Total)
	}
	if rep.Excluded != 3 {
		t.Fatalf("expected 3 excluded markers, got %d — the count must be REPORTED, "+
			"or an operator cannot reconcile this number with what `inspect` shows",
			rep.Excluded)
	}
	line := rep.Problems()[0]
	if !strings.Contains(line, "3 unrelated") {
		t.Errorf("the exclusion is not stated in the line an operator reads: %q", line)
	}
	// The surviving one must be the REAL one, not whichever sorted first.
	if !strings.Contains(line, "capability_denied") {
		t.Errorf("the reported failure is not the file-transfer one: %q", line)
	}
}

// TestDeliveryFailures_TheBareURIFormIsTheOneProductionEmits is the arm
// that would have caught the dead filter, and it is written from an
// operator's run rather than from our idea of the shape.
//
// The test above seeds `entity://{peer}/system/network`. The kernel emits
// the BARE handler path, so the original predicate — a substring test for
// `"/system/network"`, with the leading slash — matched the fixture and
// nothing real. Verbatim from the 2026-09-10 run log:
//
//	bound lost-error marker at system/runtime/chain-errors/lost/
//	  chain-93a0b3afd0eea813/notif-sub-1788524890458534194-.../not_found/00e1f9...
//	  (failed_uri=system/network status=404 code="not_found")
//
// Note what the operator was shown instead: "20 file transfer(s) failed
// ... most recent: not_found from another peer (system/network). The
// content was not there when we asked" — a fabricated explanation about
// content, on markers with no content in them, from a surface built that
// same day to stop exactly this.
//
// TargetPeerID is left ZERO here because no kernel writer sets it. Do not
// "fix" this fixture by filling it in; that is what made the other one
// unable to fail.
func TestDeliveryFailures_TheBareURIFormIsTheOneProductionEmits(t *testing.T) {
	_, st := reconcileFixture(t)

	for _, id := range []string{"aa", "bb", "cc"} {
		bindLostMarker(t, st, chainErrorPrefix+"lost/chain-n"+id+"/notif-sub-1788524890458534194/not_found/"+id,
			types.ChainErrorLostData{
				Code:      "not_found",
				Status:    404,
				TargetURI: "system/network", // bare — the observed form
				Reason:    "not_found",
				Timestamp: 1_757_000_000_001,
			})
	}

	rep := DeliveryFailures(st)
	if rep.Total != 0 {
		t.Fatalf("counted %d file-transfer failures from %d reconnect markers carrying the "+
			"BARE target URI.\n"+
			"  This is the shipped defect: the filter matched \"/system/network\" and the "+
			"kernel writes \"system/network\", so every reconnect marker was reported to "+
			"the operator as a failed file transfer — with an invented sentence about "+
			"content that was never involved.", rep.Total, rep.Excluded+rep.Total)
	}
	if rep.Excluded != 3 {
		t.Fatalf("expected 3 markers excluded and counted, got %d", rep.Excluded)
	}
	if lines := rep.Problems(); len(lines) != 0 {
		t.Fatalf("reconnect noise alone produced an operator-facing line: %q", lines)
	}
}

// TestDeliveryFailures_ANearMissHandlerIsNotSwallowed is the control arm
// for the arm above.
//
// Widening the predicate to make the bare form match is one substring test
// away from swallowing a real failure at a handler whose path merely
// STARTS with the noisy one. Without this, `strings.Contains(uri,
// "system/network")` passes everything above and silently hides transfers.
func TestDeliveryFailures_ANearMissHandlerIsNotSwallowed(t *testing.T) {
	_, st := reconcileFixture(t)

	bindLostMarker(t, st, chainErrorPrefix+"lost/chain-nm/req-1/not_found/nm",
		types.ChainErrorLostData{
			Code:      "not_found",
			Status:    404,
			TargetURI: "system/networking-does-not-exist",
			Reason:    "not_found",
			Timestamp: 1_757_000_000_003,
		})

	if rep := DeliveryFailures(st); rep.Total != 1 {
		t.Fatalf("a marker at a DIFFERENT handler that merely shares a prefix was "+
			"excluded as reconnect noise (total=%d excluded=%d) — the filter has to be "+
			"segment-exact, or fixing the bare-form miss trades a false alarm for a "+
			"silent one", rep.Total, rep.Excluded)
	}
}

// TestDeliveryFailures_NoMarkersIsSilent is the control arm.
//
// A diagnostic that fires on a healthy peer is chrome, and chrome is
// exactly what an operator learns to skip — which would reintroduce the
// defect in a new form, one release later.
func TestDeliveryFailures_NoMarkersIsSilent(t *testing.T) {
	_, st := reconcileFixture(t)

	rep := DeliveryFailures(st)
	if rep.Total != 0 {
		t.Fatalf("a peer that has delivered nothing reports %d failures", rep.Total)
	}
	if lines := rep.Problems(); len(lines) != 0 {
		t.Fatalf("a healthy peer produced problem lines: %v", lines)
	}
}

// TestStatusSnapshot_CarriesRecordedDeliveryFailures crosses the seam that
// matters: the READ has to carry this, not only the pass.
//
// A panel refreshes; it does not reconcile. If this only reached the pass
// then the operator's banner would stay silent until they pressed
// something, which is the same silence one layer along.
func TestStatusSnapshot_CarriesRecordedDeliveryFailures(t *testing.T) {
	ws, st := reconcileFixture(t)

	bindLostMarker(t, st, chainErrorPrefix+"lost/chain-x/req-1/capability_denied/aa",
		types.ChainErrorLostData{
			Code:         "capability_denied",
			Status:       403,
			TargetPeerID: themPeer,
			TargetURI:    "entity://" + themPeer + "/system/content",
			Timestamp:    1_757_000_000_000,
		})

	snap, _ := ws.StatusSnapshot()
	found := false
	for _, p := range snap.Problems {
		if strings.Contains(p, "file transfer") {
			found = true
		}
	}
	if !found {
		t.Fatalf("a READ does not report recorded transfer failures: %v\n"+
			"  The panel refreshes and never reconciles, so a diagnostic that only "+
			"reaches the pass is invisible until the operator presses something — "+
			"and they press things when they already suspect a problem, which is "+
			"the moment this was supposed to save them.", snap.Problems)
	}
}
