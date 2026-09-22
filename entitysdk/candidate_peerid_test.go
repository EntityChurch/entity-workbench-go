package entitysdk

import (
	"testing"

	"github.com/fxamacker/cbor/v2"
	"go.entitychurch.org/entity-core-go/core/types"
)

// candidate_peerid_test.go — the gate on CandidatePeerID, and the reason
// it exists.
//
// The defect it fixes was not a wrong answer, it was a permanent absence:
// two consumers read `CandidateData.PeerID`, which no mDNS candidate in
// this product has ever carried, so both did nothing at all for every
// peer on every pass. Nothing failed, because the third consumer — the
// Nearby panel — read the TXT hint and worked.
//
// So the important arm here is not `Reads_The_Hint`. It is
// `The_Substrate_Really_Does_Leave_PeerID_Empty`: without it, the day
// core-go starts populating the field, this whole file passes while
// asserting nothing, and the next person reads a green suite as evidence
// that reading the field was fine all along.

// mdnsCandidate builds a CandidateData the way core-go's
// `candidateFromServiceEntry` does: no PeerID, claimed id in the TXT
// channel only. Kept byte-shaped rather than hand-waved because the whole
// bug was a disagreement about which channel carries the id.
func mdnsCandidate(t *testing.T, peerIDHint string, extraTXT ...string) types.CandidateData {
	t.Helper()
	txt := []string{"version=1"}
	if peerIDHint != "" {
		txt = append(txt, "peer_id_hint="+peerIDHint)
	}
	txt = append(txt, extraTXT...)
	raw, err := cbor.Marshal(MDNSEndpointHint{
		HostName: "peer-host.lan.local.",
		Port:     9110,
		IPv4:     []string{"192.168.68.160"},
		Text:     txt,
	})
	if err != nil {
		t.Fatalf("encode endpoint hint: %v", err)
	}
	return types.CandidateData{
		// Deliberately NOT set — this is the shape the substrate emits.
		PeerID:       "",
		Backend:      "mdns",
		ObservedAt:   1_700_000_000_000,
		EndpointHint: raw,
	}
}

// The_Substrate_Really_Does_Leave_PeerID_Empty is the anti-vacuity arm:
// it asserts the hazard is still real, so the guard below cannot pass
// against a world where the old code would also have worked.
func TestCandidatePeerID_The_Substrate_Really_Does_Leave_PeerID_Empty(t *testing.T) {
	cd := mdnsCandidate(t, "2KLUxPXmcQx1")
	if cd.PeerID != "" {
		t.Fatalf("CandidateData.PeerID is populated (%q) — if core-go now runs the IDENTIFY "+
			"ceremony, CandidatePeerID's fallback may be retired, but verify PromoteSuccessor "+
			"has a caller before touching it", cd.PeerID)
	}
	// The old reading, spelled out. This is what shipped, and this is the
	// value it produced for every peer, forever.
	if got := cd.PeerID; got == "2KLUxPXmcQx1" {
		t.Fatal("unreachable: kept so the diff shows what the broken read was")
	}
}

func TestCandidatePeerID_ResolvesFromTheTXTHint(t *testing.T) {
	const want = "2KLUxPXmcQx1"
	if got := CandidatePeerID(mdnsCandidate(t, want)); got != want {
		t.Fatalf("CandidatePeerID = %q, want %q — an announcement carrying a peer_id_hint "+
			"must resolve, or the reconciler cannot correct a stale address", got, want)
	}
}

// The control arm. A candidate that names nobody must resolve to nobody —
// otherwise a hint-less announcement would join itself to whichever
// device record happened to be asking, which is a worse bug than the one
// being fixed.
func TestCandidatePeerID_NoHintResolvesToNothing(t *testing.T) {
	if got := CandidatePeerID(mdnsCandidate(t, "")); got != "" {
		t.Fatalf("CandidatePeerID = %q for an announcement with no peer_id_hint, want \"\"", got)
	}
	if got := CandidatePeerID(types.CandidateData{Backend: "mdns"}); got != "" {
		t.Fatalf("CandidatePeerID = %q for a candidate with no endpoint hint at all, want \"\"", got)
	}
}

// The populated field wins, so this function becomes a no-op rather than
// a competing source the day the IDENTIFY ceremony is wired.
func TestCandidatePeerID_PrefersThePopulatedFieldOverTheHint(t *testing.T) {
	cd := mdnsCandidate(t, "claimed-by-txt")
	cd.PeerID = "established-by-identify"
	if got := CandidatePeerID(cd); got != "established-by-identify" {
		t.Fatalf("CandidatePeerID = %q, want the IDENTIFY-established id to win over the claim", got)
	}
}
