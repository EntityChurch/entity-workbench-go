package shellcmd

import (
	"strings"
	"testing"
	"time"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"

	"github.com/fxamacker/cbor/v2"
	"go.entitychurch.org/entity-core-go/core/types"
)

// discovery_address_test.go — the gate on "a live announcement can correct
// a stale stored address".
//
// # What went wrong, and why nothing caught it
//
// An operator's two machines were on one LAN, discovering each other, with
// shares already established. One had moved to a different port. The app
// dialled the remembered address for 45 minutes, logged `connection
// refused` every 15 seconds, and never once tried the address the peer was
// announcing — while the Nearby panel showed that peer, with that address,
// the whole time.
//
// The reason was one field. `CandidateData.PeerID` is empty on every mDNS
// candidate (EXTENSION-DISCOVERY §2.1: null until IDENTIFY, and nothing in
// either tree calls PromoteSuccessor), and THREE consumers here joined on
// it: the reconciler's address refresh, `dialableAddressFor`, and the
// `peers` verb. All three matched nothing, always, for every peer. The
// Nearby panel read the `peer_id_hint` TXT key instead and worked, which
// is why discovery looked healthy.
//
// So these tests seed candidates in the shape the substrate really emits —
// **PeerID empty, claim in the TXT channel** — because a fixture that
// populates the field cannot fail on the bug. That is the same lesson as
// AP58: the cheap fixture omitted what production always has, so the
// arithmetic worked in the test and nowhere else.

// discoveryFixture is reconcileFixture with the discovery substrate wired
// on but nothing announcing — candidates are seeded into the store
// directly, so these tests need no multicast, no timing and no network.
func discoveryFixture(t *testing.T) (*ShellWorkspace, *workbench.Store) {
	t.Helper()
	ap, err := entitysdk.CreatePeer(entitysdk.PeerConfig{
		Extensions: entitysdk.ExtensionsConfig{Discovery: &entitysdk.DiscoveryConfig{}},
	})
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	t.Cleanup(func() { _ = ap.Close() })
	if !ap.DiscoveryEnabled() {
		t.Fatal("discovery substrate did not come up — the seeded candidates below would " +
			"be invisible and every assertion here would pass vacuously")
	}
	return NewShellWorkspace(ap, "self", ""), ap.Store()
}

// announce seeds one mDNS candidate exactly as core-go's
// candidateFromServiceEntry builds it: no PeerID, claimed id in the
// `peer_id_hint` TXT key only.
func announce(t *testing.T, st *workbench.Store, peerID, ip string, port int) {
	t.Helper()
	hint, err := cbor.Marshal(entitysdk.MDNSEndpointHint{
		HostName: "peer-host.lan.local.",
		Port:     port,
		IPv4:     []string{ip},
		Text:     []string{"version=1", "peer_id_hint=" + peerID, "proto=tcp"},
	})
	if err != nil {
		t.Fatalf("encode endpoint hint: %v", err)
	}
	cd := types.CandidateData{
		// Deliberately empty. This is the whole point of the fixture.
		PeerID:       "",
		Backend:      "mdns",
		ObservedAt:   uint64(time.Now().UnixMilli()),
		EndpointHint: hint,
	}
	ent, err := cd.ToEntity()
	if err != nil {
		t.Fatalf("candidate to entity: %v", err)
	}
	path := types.CandidateStoragePath("mdns", ent.ContentHash)
	if _, err := st.Put(path, types.TypeDiscoveryCandidate, cd); err != nil {
		t.Fatalf("seed candidate: %v", err)
	}
}

// The anti-vacuity arm. If this ever fails, the substrate has started
// populating PeerID and the fallback in CandidatePeerID may be retired —
// but check PromoteSuccessor has a caller first, because the alternative
// explanation is that somebody "fixed" the fixture.
func TestDiscovery_TheSeededCandidateReallyHasNoPeerID(t *testing.T) {
	ws, st := discoveryFixture(t)
	announce(t, st, themPeer, "192.168.68.160", 9110)
	cands := ws.Local.Peer.ReadDiscoveredCandidates()
	if len(cands) != 1 {
		t.Fatalf("seeded 1 candidate, ReadDiscoveredCandidates returned %d — the rest of "+
			"this file asserts nothing if the candidate is not visible", len(cands))
	}
	if cands[0].PeerID != "" {
		t.Fatalf("candidate carries PeerID %q; the field the broken code read is populated, "+
			"so these tests no longer reproduce the defect", cands[0].PeerID)
	}
}

func TestDiscovery_ResolvesAnAddressForAnAnnouncedPeer(t *testing.T) {
	ws, st := discoveryFixture(t)
	announce(t, st, themPeer, "192.168.68.160", 9110)

	got := ws.discoveredAddressFor(themPeer)
	if !strings.Contains(got, "192.168.68.160:9110") {
		t.Fatalf("discoveredAddressFor = %q, want the announced 192.168.68.160:9110 — "+
			"this is the join that was dead, and with it dead a stale address can never "+
			"be corrected", got)
	}
}

// The control arm: an announcement from someone else must not answer for
// the peer we asked about. Without this, "return the first candidate"
// passes the test above.
func TestDiscovery_DoesNotAnswerWithAnotherPeersAnnouncement(t *testing.T) {
	ws, st := discoveryFixture(t)
	announce(t, st, "2SomeOtherPeerEntirely", "192.168.68.99", 9110)

	if got := ws.discoveredAddressFor(themPeer); got != "" {
		t.Fatalf("discoveredAddressFor(%s) = %q from an announcement by a different peer",
			themPeer, got)
	}
}

// The operator's actual case: a device record holding an address the peer
// has moved off, and a live announcement of the real one.
func TestDiscovery_ALiveAnnouncementOutranksAStaleStoredAddress(t *testing.T) {
	ws, st := discoveryFixture(t)
	if err := workbench.SaveDevice(st, workbench.DeviceData{
		PeerID:    themPeer,
		Label:     "desk-2",
		Addresses: []string{"192.168.68.160:9000"}, // where they used to be
	}); err != nil {
		t.Fatal(err)
	}
	announce(t, st, themPeer, "192.168.68.160", 9110) // where they are

	d, ok := workbench.LoadDevice(st, themPeer)
	if !ok {
		t.Fatal("device record vanished")
	}
	ladder := ws.dialLadderFor(d)
	if len(ladder) < 2 {
		t.Fatalf("dial ladder = %v, want the announced address AND the stored one — "+
			"dropping the stored address would break every peer that is asleep or on a "+
			"network with no multicast", ladder)
	}
	if !strings.Contains(ladder[0], "9110") {
		t.Fatalf("dial ladder = %v, want the ANNOUNCED address first: a stored address is "+
			"a hypothesis about where a peer is, a live announcement is an observation, "+
			"and treating durable as authoritative is the whole defect", ladder)
	}
	if !strings.Contains(strings.Join(ladder, " "), "9000") {
		t.Fatalf("dial ladder = %v, want the stored address retained as a fallback — it is "+
			"also what makes trusting the mDNS claim safe, since a spoofed announcement "+
			"then costs one failed handshake rather than the relationship", ladder)
	}
}

// A peer that is announcing nothing must still be dialled at what we
// remember. The regression this guards is a "discovery first" change that
// quietly becomes "discovery only".
func TestDiscovery_TheStoredAddressStillWorksWithNoAnnouncement(t *testing.T) {
	ws, st := discoveryFixture(t)
	if err := workbench.SaveDevice(st, workbench.DeviceData{
		PeerID:    themPeer,
		Addresses: []string{"192.168.68.160:9000"},
	}); err != nil {
		t.Fatal(err)
	}
	d, _ := workbench.LoadDevice(st, themPeer)
	ladder := ws.dialLadderFor(d)
	if len(ladder) != 1 || !strings.Contains(ladder[0], "9000") {
		t.Fatalf("dial ladder = %v, want just the stored address", ladder)
	}
	_ = ws
}
