package workbench

import "testing"

// The distinction the whole file turns on: an ABSENT observation is
// unknown, and an observation carrying an EMPTY rule is a real answer
// meaning the default.
//
// Collapsing the two is not a cosmetic slip — it is the difference
// between refusing to resolve a collision and silently applying our own
// rule to somebody else's folder, which is the divergence the ruling
// exists to prevent. It is also the easiest possible mistake to make,
// because `Conflict == ""` reads as "nothing here" at every call site.
//
// Tier: unit (TESTING-STRATEGY §1).
func TestObservedFolder_AbsentIsUnknownAndEmptyIsTheDefault(t *testing.T) {
	st := liveStore(t)

	if _, ok := LoadObservedFolder(st, "owner.folder"); ok {
		t.Fatal("an unwritten observation reported as known")
	}

	// They declared nothing, which IS a declaration: the default.
	if err := SaveObservedFolder(st, ObservedFolderData{
		FolderID:    "owner.folder",
		OwnerPeerID: "owner",
		Conflict:    "",
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	o, ok := LoadObservedFolder(st, "owner.folder")
	if !ok {
		t.Fatal("a written observation reported as unknown — the refusal path would " +
			"then fire for every folder whose owner declared no policy, which is most " +
			"of them")
	}
	if got := o.OwnerConflictPolicy(); got != ConflictPolicyRecord {
		t.Errorf("an empty declared rule resolved to %q, want %q", got, ConflictPolicyRecord)
	}

	// And it must agree with the local reading of the same field, because
	// they are the same field read from two machines.
	if got, want := o.OwnerConflictPolicy(), (FolderData{Conflict: o.Conflict}).ConflictPolicy(); got != want {
		t.Errorf("the owner's rule resolves to %q locally and %q remotely — one field, "+
			"two answers", want, got)
	}
}

// An observation with no owner names nobody, and a rule attributed to
// nobody is worse than no rule: it would satisfy the `known` test at the
// delivery path while carrying no accountable source.
func TestSaveObservedFolder_RefusesAnOwnerlessObservation(t *testing.T) {
	st := liveStore(t)
	if err := SaveObservedFolder(st, ObservedFolderData{FolderID: "f", Conflict: "keep-both"}); err == nil {
		t.Error("an observation with no owner was accepted")
	}
}

func TestObservedFolder_RoundTripsTheRuleVerbatim(t *testing.T) {
	st := liveStore(t)
	if err := SaveObservedFolder(st, ObservedFolderData{
		FolderID:         "owner.folder",
		OwnerPeerID:      "owner",
		Conflict:         ConflictPolicyKeepBoth,
		ObservedAtMillis: 1700000000000,
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	o, ok := LoadObservedFolder(st, "owner.folder")
	if !ok {
		t.Fatal("not found")
	}
	if o.Conflict != ConflictPolicyKeepBoth {
		t.Errorf("rule round-tripped as %q, want %q", o.Conflict, ConflictPolicyKeepBoth)
	}
	if o.OwnerPeerID != "owner" {
		t.Errorf("owner round-tripped as %q", o.OwnerPeerID)
	}
	if o.ObservedAtMillis != 1700000000000 {
		t.Errorf("timestamp round-tripped as %d", o.ObservedAtMillis)
	}
	if all := LoadObservedFolders(st); len(all) != 1 {
		t.Errorf("LoadObservedFolders returned %d, want 1", len(all))
	}
	if !RemoveObservedFolder(st, "owner.folder") {
		t.Error("remove reported nothing removed")
	}
	if _, ok := LoadObservedFolder(st, "owner.folder"); ok {
		t.Error("still present after remove")
	}
}
