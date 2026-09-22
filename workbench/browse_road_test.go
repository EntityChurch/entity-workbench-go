package workbench

import (
	"strings"
	"testing"

	"entity-workbench-go/fetch"
)

// browse_road_test.go — the choice between the roads a binding offers.
//
// `roadsFor` takes `havePeer` as an argument rather than reading it off a
// model, and this file is why: the arm that matters most is *"a browser
// with no peer takes the static road even when live is offered and ranked
// first"*, and a model-bound version would need a whole peer stood up in
// order to assert the absence of one.
//
// **The ladder above it (`travel`) is exercised where a real published
// act exists** — `publish/live_and_static_test.go` drives both roads over
// one signed root. What is under test here is the ORDER and the
// declines, which is the part with no bytes in it.
//
// Tier: unit.

func liveCandidate(addr string, priority uint64) fetch.TransportCandidate {
	return fetch.TransportCandidate{
		Class:            fetch.ClassLive,
		Type:             "system/peer/transport/tcp",
		Priority:         priority,
		PriorityDeclared: true,
		Address:          addr,
		PeerID:           "2KFRBJ9feEPCZZiNaCEKsCAVkGkp1htWZk9a8jz5n5D2sS",
	}
}

func staticCandidate(origin string, priority uint64) fetch.TransportCandidate {
	return fetch.TransportCandidate{
		Class:            fetch.ClassStatic,
		Type:             "system/peer/transport/http-poll",
		Priority:         priority,
		PriorityDeclared: true,
		Layout:           fetch.Layout{Origin: origin, PeerID: "2KFRBJ9feEPCZZiNaCEKsCAVkGkp1htWZk9a8jz5n5D2sS"},
	}
}

// options builds a ranked TransportOptions the way `TransportsFor`
// would, i.e. already sorted. Sorting is `fetch`'s and is gated there;
// what this file asserts is what happens to an already-ranked list.
func options(cands ...fetch.TransportCandidate) fetch.TransportOptions {
	return fetch.TransportOptions{Candidates: cands}
}

// TestBindingOfferingBothPicksLiveAndKeepsStaticBehindIt is the plan's
// first arm.
//
// Live first is THIS consumer's choice and not the specification's: D1
// ranks within a transport type and says nothing about static-vs-live.
// The reason is that a live read removes a party — a static origin can
// serve an arbitrarily old correctly-signed root and say nothing —
// which is the entire content of the freshness difference.
//
// The static road must still be PRESENT and behind it: §6.5.1c consumer
// rule 6 says a profile is not a promise that it currently answers, so
// dropping the other road on preferring one would make an asleep peer
// into an unreachable site.
func TestBindingOfferingBothPicksLiveAndKeepsStaticBehindIt(t *testing.T) {
	// Deliberately ranked the WRONG way round for the outcome: the static
	// profile carries the better D1 priority. Class preference has to win
	// across classes or this arm passes on the ranking alone.
	opts := options(
		staticCandidate("https://origin.test", 0),
		liveCandidate("tcp://peer.test:9110", 50),
	)
	roads, declined := roadsFor(opts, true)
	if len(roads) != 2 {
		t.Fatalf("got %d roads, want 2 (live then static): %v", len(roads), roads)
	}
	if roads[0].Class != fetch.ClassLive {
		t.Fatalf("first road is %s, want live — a live read removes the party that could be "+
			"withholding a newer root, which is the only thing that legitimately differs",
			roads[0].Class)
	}
	if roads[1].Class != fetch.ClassStatic {
		t.Fatalf("second road is %s, want the static fallback kept: a profile in a signed set is "+
			"not a promise that it currently answers (§6.5.1c rule 6)", roads[1].Class)
	}
	if len(declined) != 0 {
		t.Fatalf("nothing should have been declined, got %v", declined)
	}
}

// TestBindingOfferingOnlyHTTPPollIsUnchanged is the plan's second arm:
// the road every shipped surface has taken until now must still be taken,
// identically, whether or not a peer is present.
func TestBindingOfferingOnlyHTTPPollIsUnchanged(t *testing.T) {
	opts := options(staticCandidate("https://origin.test", 100))
	for _, havePeer := range []bool{false, true} {
		roads, declined := roadsFor(opts, havePeer)
		if len(roads) != 1 || roads[0].Class != fetch.ClassStatic {
			t.Fatalf("havePeer=%v: got %v, want exactly the static road", havePeer, roads)
		}
		if len(declined) != 0 {
			t.Fatalf("havePeer=%v: declined %v, want nothing", havePeer, declined)
		}
	}
}

// TestBindingOfferingNeitherRefusesAndSaysWhatItSaw is the plan's third
// arm, and the assertion is on the SENTENCE.
//
// "No transport" and "three transports, all of which we declined" are
// different operator problems with different next steps. A refusal that
// collapses them sends someone to check their network when the answer is
// that the publisher advertises a family we do not speak.
func TestBindingOfferingNeitherRefusesAndSaysWhatItSaw(t *testing.T) {
	opts := fetch.TransportOptions{
		Skipped: []fetch.SkippedTransport{
			{Reason: "transport-profile ecf-sha256:aa is type \"system/peer/transport/quic\", " +
				"which is not a transport family this consumer speaks"},
		},
	}
	roads, declined := roadsFor(opts, true)
	if len(roads) != 0 {
		t.Fatalf("got %d roads from a binding carrying none: %v", len(roads), roads)
	}
	summary := offeredSummary(nil, declined)
	for _, want := range []string{"quic", "not a transport family"} {
		if !strings.Contains(summary, want) {
			t.Errorf("the refusal does not mention %q, so the operator cannot tell a missing "+
				"transport from an unspoken one: %s", want, summary)
		}
	}
}

// TestNoPeerTakesTheStaticRoadAndBlamesItselfForIt is the arm the
// argument-not-a-field shape exists for.
//
// A peer-less browser is `entity-fetch`'s configuration and it is a
// complete consumer, not a degraded one. When it declines a live road the
// sentence has to say so about ITSELF: a message phrased as a fact about
// the publisher would send an operator to the other machine, and there is
// nothing wrong with the other machine.
func TestNoPeerTakesTheStaticRoadAndBlamesItselfForIt(t *testing.T) {
	opts := options(
		liveCandidate("tcp://peer.test:9110", 0),
		staticCandidate("https://origin.test", 100),
	)
	roads, declined := roadsFor(opts, false)
	if len(roads) != 1 || roads[0].Class != fetch.ClassStatic {
		t.Fatalf("got %v, want only the static road", roads)
	}
	if len(declined) != 1 {
		t.Fatalf("got %d declines, want the live road named: %v", len(declined), declined)
	}
	if !strings.Contains(declined[0], "this browser") {
		t.Errorf("the decline reads as a fact about the publisher rather than about us: %q",
			declined[0])
	}
}

// TestALiveFamilyWeCannotDialIsDeclinedByNameNotMisdialled is AP44's
// shape caught before it ships.
//
// core-go implements `Connect` (TCP) and `ConnectWebSocket` and no other,
// so an `http` transport profile is a perfectly conformant thing we
// cannot drive. Handing `http://host:port` to `Connect` falls into the
// TCP arm and dies inside `net.SplitHostPort` with "too many colons",
// which reads as a malformed profile — a false accusation, and one with
// no wrong answer left behind for anyone to catch.
func TestALiveFamilyWeCannotDialIsDeclinedByNameNotMisdialled(t *testing.T) {
	http1 := liveCandidate("http://peer.test:9110", 0)
	http1.Type = "system/peer/transport/http"
	opts := options(http1, staticCandidate("https://origin.test", 100))

	roads, declined := roadsFor(opts, true)
	if len(roads) != 1 || roads[0].Class != fetch.ClassStatic {
		t.Fatalf("got %v, want the undialable live road dropped and the static one kept", roads)
	}
	if len(declined) != 1 || !strings.Contains(declined[0], "tcp and websocket") {
		t.Fatalf("the decline does not name the families this browser has a client for: %v", declined)
	}
	if strings.Contains(declined[0], "malformed") || strings.Contains(declined[0], "invalid") {
		t.Errorf("the decline blames the profile: %q — the profile is fine", declined[0])
	}

	// Anti-vacuity: the same shape with a scheme we DO dial must produce a
	// live road. Without this the assertions above are satisfied by a
	// `dialableLive` that refuses everything.
	ws := liveCandidate("ws://peer.test:9110", 0)
	ws.Type = "system/peer/transport/websocket"
	roads, _ = roadsFor(options(ws), true)
	if len(roads) != 1 || roads[0].Class != fetch.ClassLive {
		t.Fatalf("a websocket profile produced %v — the refusal above is refusing everything", roads)
	}
}

// TestD1OrderIsPreservedWithinEachClass: the class preference reorders
// the two groups and must not reorder inside one.
//
// Ranking within a class is the specification's (§6.5.1a D1, priority
// asc) and arrives already sorted from `TransportsFor`. This asserts the
// chooser is a partition and not a re-sort — a publisher's declared
// preference between two mirrors survives it.
func TestD1OrderIsPreservedWithinEachClass(t *testing.T) {
	opts := options(
		liveCandidate("tcp://preferred.test:9110", 0),
		liveCandidate("tcp://mirror.test:9110", 10),
		staticCandidate("https://preferred-origin.test", 20),
		staticCandidate("https://mirror-origin.test", 30),
	)
	roads, _ := roadsFor(opts, true)
	got := make([]string, 0, len(roads))
	for _, r := range roads {
		if r.Class == fetch.ClassLive {
			got = append(got, r.Candidate.Address)
			continue
		}
		got = append(got, r.Candidate.Layout.Origin)
	}
	want := []string{
		"tcp://preferred.test:9110",
		"tcp://mirror.test:9110",
		"https://preferred-origin.test",
		"https://mirror-origin.test",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("road order %v, want %v — the chooser partitions by class and must not re-sort "+
			"inside one, or a publisher's preference between two mirrors is discarded", got, want)
	}
}

// TestOneFloorPerPublisherAcrossBothRoads is the WIRING half of the
// shared seq floor.
//
// The mechanism is gated in `fetch` (`TestConsumer_SeqFloorIsPerPublisher
// NotPerRoad`, with the control arm that proves an unshared floor takes
// the rollback). What that cannot see is whether this model actually
// hands the same floor to both kinds of consumer — a correct `SeqFloor`
// reached by two `floorFor` calls with different keys is the defect
// wearing the fix.
//
// So this asserts on identity: the static consumer for a layout and the
// live consumer for the same peer-id must enforce the SAME object. Keyed
// on peer-id and nothing else, because the road that carried a root is
// not part of the statement §3-RES.4 makes.
func TestOneFloorPerPublisherAcrossBothRoads(t *testing.T) {
	const pub = "2KFRBJ9feEPCZZiNaCEKsCAVkGkp1htWZk9a8jz5n5D2sS"
	m := NewBrowseModel(nil)

	staticA := m.consumerFor(fetch.Layout{PeerID: pub, Origin: "https://a.test"})
	// A SECOND static layout for the same publisher: a re-based layout and
	// a discovered one are different readers and are keyed apart on
	// purpose, which is exactly the case the old per-consumer floor got
	// wrong quietly.
	staticB := m.consumerFor(fetch.Layout{PeerID: pub, Origin: "https://b.test"})
	live := m.floorFor(pub)

	if staticA == staticB {
		t.Fatal("two origins for one publisher returned one consumer — they are different " +
			"readers and the test below would then prove nothing")
	}
	if staticA.Floor() != staticB.Floor() {
		t.Error("two static roads to one publisher hold different floors")
	}
	if staticA.Floor() != live {
		t.Error("the live road's floor is not the static road's floor — a reader that verified " +
			"a root on one road starts from no memory on the other")
	}

	// Anti-vacuity: a DIFFERENT publisher must get a different floor.
	// Without this, returning one process-wide floor would pass every
	// assertion above and would refuse one publisher's root because
	// another publisher's seq happened to be higher.
	other := m.floorFor("2KBLkCxvkgobuauPA6zPfKarpuRRnnWHL98n8Gv1GNmybr")
	if other == live {
		t.Error("two publishers share one floor — monotonicity is per peer, and a global floor " +
			"refuses a legitimate root from whoever publishes more slowly")
	}
}
