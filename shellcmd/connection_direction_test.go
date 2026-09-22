package shellcmd

import (
	"strings"
	"testing"
)

// connection_direction_test.go — the gate on "we never render a peer we
// cannot reach as simply connected".
//
// An operator spent 45 minutes on 2026-09-08 watching a row that said
// **connected** while nothing they wrote left the machine. It was not a
// lie about the pool: the peer had dialled THEM, so a session genuinely
// existed. It was a claim the pool cannot support — a connection has a
// direction for authority and none for display, and a dial-by-address
// authorizes only the dialer (AP63), so the connection they held was one
// they could not dispatch over.
//
// The run log said so, correctly, at startup. The panel disagreed, and
// **the reassuring one was the one on screen.**
//
// This is the same discipline the Sharing Status panel already applies to
// inbound authority — state what is knowable, never draw a health dot over
// what is not — one field across.

// The case that cost the 45 minutes: a live session, no outbound route,
// and a folder we publish to them.
func TestDirection_InboundOnlyIsNotReportedAsConnected(t *testing.T) {
	st := DeviceStatus{Label: "desk-2", Connected: true, OutboundRoute: false}

	got := st.directionProblem(true)
	if got == "" {
		t.Fatal("an inbound-only peer we publish to produced no problem line — this is the " +
			"exact state where sharing is half-broken, and it renders as `connected`")
	}
	if !strings.Contains(got, "they can reach us") || !strings.Contains(got, "nothing we write") {
		t.Fatalf("problem = %q; it must say the direction and the consequence, because "+
			"`not maintained` was already on screen and told nobody what was wrong", got)
	}
}

// The control arm. Without it, `return "a problem"` passes the test above
// and every healthy peer is flagged forever — which is how a warning
// becomes something operators learn to ignore.
func TestDirection_AnOutboundRouteIsSilent(t *testing.T) {
	st := DeviceStatus{Label: "desk-2", Connected: true, OutboundRoute: true}
	if got := st.directionProblem(true); got != "" {
		t.Fatalf("a peer we hold an outbound route to produced %q, want silence", got)
	}
}

// A peer we only RECEIVE from is flagged too, and naming the right
// consequence is the point.
//
// The first draft of directionProblem stayed silent here, reasoning that
// receiving needs their dial rather than ours. **AP63 already says that is
// wrong: a sync is MUTUAL.** The receiver dispatches out to subscribe and
// to pull the blob closure, so an unreachable peer breaks an incoming
// folder just as completely — it simply presents as "nothing is arriving"
// instead of "nothing is being sent", which sends an operator looking in
// the opposite place.
func TestDirection_ReceiveOnlyPeerIsFlaggedWithTheRightConsequence(t *testing.T) {
	st := DeviceStatus{Label: "desk-2", Connected: true, OutboundRoute: false}
	got := st.directionProblem(false)
	if got == "" {
		t.Fatal("a peer we only receive from produced no problem — the receiver dispatches " +
			"out to subscribe and fetch, so this folder has stopped updating")
	}
	if !strings.Contains(got, "receive") {
		t.Fatalf("problem = %q; a receive-only peer must name the symptom the operator "+
			"will actually see, which is a folder that stops updating", got)
	}
	if strings.Contains(got, "nothing we write") {
		t.Fatalf("problem = %q; that is the PUBLISH consequence and it points the operator "+
			"at the wrong machine", got)
	}
}

// A paused peer is deliberately not maintained. Reporting it would make
// the operator's own choice look like a fault.
func TestDirection_PausedIsSilent(t *testing.T) {
	st := DeviceStatus{Label: "desk-2", Connected: true, OutboundRoute: false, Paused: true}
	if got := st.directionProblem(true); got != "" {
		t.Fatalf("a paused peer produced %q, want silence", got)
	}
}

// The offline case still names the consequence, and carries whatever the
// dial ladder learned. The Note is the part that says WHICH addresses were
// refused, which is what turns "it doesn't work" into a port number.
func TestDirection_OfflinePeerCarriesTheDialDetail(t *testing.T) {
	st := DeviceStatus{
		Label: "desk-2",
		Note:  "could not dial 192.168.68.160:9000 (connection refused)",
	}
	got := st.directionProblem(true)
	if !strings.Contains(got, "192.168.68.160:9000") {
		t.Fatalf("problem = %q, want the refused address in it — without the detail the "+
			"operator cannot tell a stale port from a sleeping machine", got)
	}
}
