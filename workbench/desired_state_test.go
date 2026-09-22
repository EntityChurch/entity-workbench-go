package workbench

import (
	"strings"
	"testing"
)

// desired_state_test.go — the declared objects, on a store that
// NAMESPACES.
//
// Every one of these uses liveStore (a CreatePeer-backed store) rather
// than NewStore(memory, memory). That is not belt-and-braces: the cheap
// scaffolding has no NamespacedIndex, so List returns bare relative paths
// and any prefix arithmetic works by accident. Every model test in this
// package used the cheap one, and the one time it mattered — AP58 — the
// Local Files panel shipped rendering "no filesystem mounts on this peer"
// for a peer that had one, with a green suite. A fixture that omits a
// wrapper the production object always has cannot fail on anything the
// wrapper changes.

func TestDevices_RoundTripOnAPeerBackedStore(t *testing.T) {
	st := liveStore(t)

	const pid = "2KHyyndBo6LW3FigmdHPQdkvscNpngeNS3FAGoRfoMoYwN"
	if err := SaveDevice(st, DeviceData{
		PeerID:        pid,
		Label:         "desk-2",
		Addresses:     []string{"tcp://192.168.1.20:9110"},
		AddedAtMillis: 1_700_000_000_000,
	}); err != nil {
		t.Fatal(err)
	}

	one, ok := LoadDevice(st, pid)
	if !ok {
		t.Fatal("LoadDevice found nothing it had just written")
	}
	if one.Label != "desk-2" || one.PreferredAddress() != "tcp://192.168.1.20:9110" {
		t.Errorf("device read back as %+v", one)
	}

	all, problems := LoadDevices(st)
	if len(problems) != 0 {
		t.Fatalf("problems: %v", problems)
	}
	if len(all) != 1 || all[0].PeerID != pid {
		// The AP58 failure mode lands here: with a relative TrimPrefix the
		// remainder keeps its slashes, the guard skips every row, and this
		// reads zero devices on a peer that has one.
		t.Fatalf("LoadDevices = %+v, want exactly the one device", all)
	}

	if !RemoveDevice(st, pid) {
		t.Error("RemoveDevice reported nothing to remove")
	}
	if _, ok := LoadDevice(st, pid); ok {
		t.Error("device survived removal")
	}
}

func TestFolders_RoundTripOnAPeerBackedStore(t *testing.T) {
	st := liveStore(t)

	const them = "2KLf7osYcMLEdmSnYLx3Bg6TScaGJefLZJdKaL1tPNHAbR"
	f := FolderData{
		ID:    "downloads",
		Label: "Downloads",
		Kind:  "files",
		Path:  "/home/me/Downloads",
		Root:  "downloads",
		Mode:  FolderModeBoth,
	}
	f = f.WithPeerState(them, FolderStateOffered, 1, "")
	if err := SaveFolder(st, f); err != nil {
		t.Fatal(err)
	}

	got, ok := LoadFolder(st, "downloads")
	if !ok {
		t.Fatal("LoadFolder found nothing it had just written")
	}
	if got.Origin != "local" || !got.IsLocal() {
		t.Errorf("a folder saved with no Origin should default to local; got %q", got.Origin)
	}
	if ps, ok := got.PeerState(them); !ok || ps.State != FolderStateOffered {
		t.Errorf("peer state = (%+v, %v), want offered", ps, ok)
	}
	if len(got.AcceptedPeers()) != 0 {
		t.Error("an offered folder must not report an accepted peer — the reconciler acts only on accepted")
	}

	all, problems := LoadFolders(st)
	if len(problems) != 0 || len(all) != 1 {
		t.Fatalf("LoadFolders = (%+v, %v), want one row", all, problems)
	}
}

// TestFolderPeerState_ReplacesRatherThanAppends is the one piece of
// arithmetic here that is easy to get wrong by hand, and it is wrong in a
// way that survives every happy-path test: a peer who declines and later
// accepts ends up with TWO entries, and whether the folder is shared then
// depends on which one a reader looks at first.
func TestFolderPeerState_ReplacesRatherThanAppends(t *testing.T) {
	const them = "2KLf7osYcMLE"
	f := FolderData{ID: "notes"}
	f = f.WithPeerState(them, FolderStateOffered, 1, "")
	f = f.WithPeerState(them, FolderStateDeclined, 2, "not now")
	f = f.WithPeerState(them, FolderStateAccepted, 3, "")

	if len(f.SharedWith) != 1 {
		t.Fatalf("SharedWith has %d entries for one peer: %+v", len(f.SharedWith), f.SharedWith)
	}
	ps, _ := f.PeerState(them)
	if ps.State != FolderStateAccepted || ps.AtMillis != 3 {
		t.Errorf("latest state = %+v, want accepted at 3", ps)
	}
	if ps.Note != "" {
		t.Errorf("note %q survived a state change that did not set one — a stale reason is worse than none", ps.Note)
	}
	if got := f.AcceptedPeers(); len(got) != 1 || got[0] != them {
		t.Errorf("AcceptedPeers = %v", got)
	}
}

// TestDeviceAddresses_PromoteDedupeAndBound covers the address list the
// reconciler dials from. The failure it guards is a roaming laptop whose
// device record grows an address per network forever, and the subtler one
// where a discovery pass that learned nothing erases what we knew.
func TestDeviceAddresses_PromoteDedupeAndBound(t *testing.T) {
	d := DeviceData{PeerID: "p"}
	for _, a := range []string{"a:1", "b:2", "c:3", "d:4", "e:5", "f:6"} {
		d = d.WithAddress(a)
	}
	if len(d.Addresses) != MaxDeviceAddresses {
		t.Fatalf("kept %d addresses, want the bound of %d: %v", len(d.Addresses), MaxDeviceAddresses, d.Addresses)
	}
	if d.Addresses[0] != "f:6" {
		t.Errorf("freshest address is %q, want the last one offered", d.Addresses[0])
	}

	before := append([]string(nil), d.Addresses...)
	d = d.WithAddress("   ")
	if len(d.Addresses) != len(before) || d.Addresses[0] != before[0] {
		t.Errorf("an empty address changed the list: %v -> %v", before, d.Addresses)
	}

	d = d.WithAddress("d:4")
	seen := map[string]int{}
	for _, a := range d.Addresses {
		seen[a]++
	}
	for a, n := range seen {
		if n > 1 {
			t.Errorf("address %q appears %d times after re-offering it", a, n)
		}
	}
}

// TestSaveRefusesAnUnusableID — AP33. A tolerant fallback that rewrites a
// malformed id into a well-formed one writes a record at a path the
// caller did not ask for, and the failure surfaces later, somewhere else,
// as "the folder I created is not there".
func TestSaveRefusesAnUnusableID(t *testing.T) {
	st := liveStore(t)
	for _, bad := range []string{"", "   ", "a/b", ".", ".."} {
		if err := SaveFolder(st, FolderData{ID: bad}); err == nil {
			t.Errorf("SaveFolder(%q) was accepted; it must be refused", bad)
		}
		if err := SaveDevice(st, DeviceData{PeerID: bad}); err == nil {
			t.Errorf("SaveDevice(%q) was accepted; it must be refused", bad)
		}
	}
}

// TestReceivedFolderIDIsTheSyncBindingKey pins the join. A received
// folder and its sync binding are two records that must name the same
// thing; deriving one id from the other is what keeps them from drifting
// on the single field that connects them.
func TestReceivedFolderIDIsTheSyncBindingKey(t *testing.T) {
	const them, root = "2KLf7osYcMLE", "downloads"
	id := ReceivedFolderID(them, root)
	if id != SyncBindingKey(them, root) {
		t.Fatalf("ReceivedFolderID = %q, SyncBindingKey = %q", id, SyncBindingKey(them, root))
	}
	if strings.Contains(id, "/") {
		t.Fatalf("folder id %q contains a path separator and cannot be a path segment", id)
	}
	gotPeer, gotRoot, ok := SplitSyncBindingKey(id)
	if !ok || gotPeer != them || gotRoot != root {
		t.Fatalf("round trip = (%q, %q, %v)", gotPeer, gotRoot, ok)
	}
}
