package shellcmd

import (
	"context"
	"testing"

	"entity-workbench-go/workbench"
)

// declare_test.go — the unwind, which is the edge the policy union
// creates and the one a careless fix breaks.
//
// # Why this test is in the shellcmd package and not beside the e2e gate
//
// The first version of it lived in `shellboot` and drove the real `Share`
// verb with an unmounted root, expecting the refusal to exercise the
// unwind. It passed against the broken code, because `Share` refuses at
// the `hasLocalRoot` check — which is BEFORE anything is declared, so the
// unwind never ran. A probe that does not reach the mechanism refutes
// nothing (AP43), and the tell was that the control arm stayed green.
//
// The real unwind fires only when `SaveShareOffer` fails, which is a
// store failure with no injection point from outside the package. So it
// is called directly here. That is the honest trade: an internal test
// that reaches the code, rather than an end-to-end one that does not.
func TestUnwindShare_LeavesTheOtherDirectionAuthorized(t *testing.T) {
	ws, st := reconcileFixture(t)
	const them = "2KLf7osYcMLEdmSnYLx3Bg6TScaGJefLZJdKaL1tPNHAbR"

	// The state a share-then-accept pair leaves: one folder out to them,
	// one folder in from them, and the union of both grants in the row.
	declareBothDirections(t, st, them)
	if _, err := ws.ApplyDeclaredPolicy(them, "test seed"); err != nil {
		t.Fatalf("seed policy: %v", err)
	}
	pol, ok := workbench.LoadAccessPolicy(st, them)
	if !ok || !grantsInclude(pol.Grants, "system/subscription") ||
		!grantsInclude(pol.Grants, workbench.BlobResolvePattern) {
		t.Fatalf("premise broken: the seeded row is not the union (%+v)", pol.Grants)
	}

	// Now unwind the OUTGOING folder, as a failed `Share` does. The
	// incoming one is untouched and must keep working.
	if _, ok := workbench.LoadFolder(st, "photos"); !ok {
		t.Fatal("fixture did not create the outgoing folder")
	}
	// hadPrior=false is the common case: the share that failed is the one
	// that created the record, so the unwind removes it entirely.
	ws.undeclareLocalShare("photos", them, workbench.FolderData{}, false)

	pol, ok = workbench.LoadAccessPolicy(st, them)
	if !ok {
		t.Fatal("the unwind deleted the policy row outright — the folder we accepted FROM " +
			"this peer can no longer be delivered, because an unrelated share failed to " +
			"record its offer")
	}
	if !grantsInclude(pol.Grants, workbench.BlobResolvePattern) {
		t.Errorf("the unwind revoked the delivery grant for a folder accepted from the same "+
			"peer.\n  grants now: %+v", pol.Grants)
	}
	// And the half it WAS supposed to withdraw is gone: an unwind that
	// leaves the sender grant behind is authority for an offer that was
	// never recorded, which is the thing the unwind exists to prevent.
	if grantsInclude(pol.Grants, "system/subscription") {
		t.Errorf("the unwind left the sender grant in place for a share that was rolled "+
			"back.\n  grants now: %+v", pol.Grants)
	}
	if _, still := workbench.LoadFolder(st, "photos"); still {
		t.Error("the unwind left the folder declaration behind")
	}
}

// TestUnshare_IsNotResurrectedByTheNextReconcile.
//
// The defect a control loop CREATES if a verb changes the substrate
// without changing the declaration. `Unshare` removed the policy row and
// left the folder record saying `offered`, so the next reconcile — at the
// following `status`, or simply the next launch — read the declaration
// and wrote the grant back. The withdrawal held until the operator
// restarted the app and then silently reversed itself, which is worse
// than failing outright because nothing reports it.
//
// Asserted by running the loop, because that is the thing that undoes it:
// a test that only reads the row after `Unshare` passes against the bug.
func TestUnshare_IsNotResurrectedByTheNextReconcile(t *testing.T) {
	ws, st := reconcileFixture(t)
	const them = "2KLf7osYcMLEdmSnYLx3Bg6TScaGJefLZJdKaL1tPNHAbR"

	declareBothDirections(t, st, them)
	if _, err := ws.ApplyDeclaredPolicy(them, "test seed"); err != nil {
		t.Fatalf("seed policy: %v", err)
	}

	// Withdraw the outgoing folder the way `Unshare` does.
	if err := ws.declareWithdrawnShare("photos", them); err != nil {
		t.Fatalf("declareWithdrawnShare: %v", err)
	}
	if _, err := ws.WithdrawDeclaredPolicy(them, "unshare: photos"); err != nil {
		t.Fatalf("WithdrawDeclaredPolicy: %v", err)
	}

	pol, ok := workbench.LoadAccessPolicy(st, them)
	if !ok {
		t.Fatal("the whole row went, taking the delivery grant for the folder we " +
			"accepted FROM this peer with it")
	}
	if grantsInclude(pol.Grants, "system/subscription") {
		t.Fatalf("premise broken: unshare did not withdraw the sender grant (%+v)", pol.Grants)
	}

	// The loop runs. It must not put back what was just withdrawn.
	if _, err := ws.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	pol, _ = workbench.LoadAccessPolicy(st, them)
	if grantsInclude(pol.Grants, "system/subscription") {
		t.Errorf("the reconciler re-granted a folder the operator unshared — the withdrawal "+
			"reverses itself at the next `status` or launch.\n  grants now: %+v", pol.Grants)
	}
	// And the incoming half is untouched throughout: unsharing one folder
	// must not disturb a folder coming the other way.
	if !grantsInclude(pol.Grants, workbench.BlobResolvePattern) {
		t.Errorf("unshare removed the delivery grant for a folder accepted from the same "+
			"peer.\n  grants now: %+v", pol.Grants)
	}
}
