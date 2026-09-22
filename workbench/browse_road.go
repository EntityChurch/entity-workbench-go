package workbench

import (
	"context"
	"fmt"
	"strings"

	"entity-workbench-go/fetch"
)

// browse_road.go — choosing between the roads a binding offers, and
// walking them in order until one answers.
//
// # What this closes
//
// `fetch.TransportsFor` ranks a binding's `transports` by §6.5.1a D1 and
// reports the live ones. `workbench.PeerSource` reads a publisher by
// dispatch. Between them sat nothing: `BrowseModel.goTo` called
// `Registry.OriginFor`, the static half, so **every shipped surface took
// the static road however loudly a publisher advertised itself as
// reachable.** The ranking was conformant and the live source was
// verified end to end, and no user could reach either.
//
// # Ranking is the specification's; CHOOSING is ours, and here is the
// choice
//
// D1 orders profiles *of the wanted transport_type*. It answers "which of
// these mirrors" and says nothing about static-vs-live, because that
// depends on what the consumer can do — `entity-fetch` links no peer and
// can only ever take the static road. So this file decides, and the
// decision is **live first when we have a peer to dial with**:
//
//   - It removes a party. A static origin is a third party that can serve
//     an arbitrarily old correctly-signed root and say nothing; the
//     publisher cannot be in that position about its own root. That is
//     the whole content of the freshness difference (fetch/freshness.go)
//     and it is the only thing that legitimately differs between the two.
//   - It is not a stronger verification and must never be sold as one.
//     An authenticated connection proves WHO, not WHAT. Every check runs
//     identically whichever road answered, by construction — there is one
//     [fetch.Consumer].
//
// Within each class the order is D1's, untouched.
//
// # The ladder, and where it stops
//
// A road is a hypothesis: §6.5.1c consumer rule 6 says a profile in a
// signed set *"is not a promise that it currently answers"* and has
// consumers fall through on failure. So the roads are tried in order and
// a failure moves to the next one — the same shape as the reconciler's
// `dialLadderFor`, and for the same reason a remembered address is a
// hypothesis and a live announcement is an observation.
//
// **It stops the moment a root verifies.** Falling through after that
// would let a stale third party paper over a publisher contradicting its
// own signed root — an incomplete walk on the live road is a finding
// ABOUT THE PUBLISHER, and answering it with an origin's older copy
// destroys the finding and shows the reader bytes while doing it. Before
// that point, a decline is an ordinary fact (they are asleep, they have
// not authorized us, the origin 404s) and the reader wants the other
// road.
//
// **And the declines are kept.** *"No transport"* and *"three transports,
// all of which declined, here is what each said"* are different operator
// problems with different next steps, and a ladder that returns one error
// for both erases the difference.

// browseRoad is one way this browser can reach a publisher's bytes.
//
// Exactly one of Live/Static is meaningful, and Class says which. Kept as
// one type rather than two so the ladder is one loop over one ordered
// slice — the order across classes IS the choice this file makes, and
// splitting the slice would put that choice back at the call site where
// it cannot be tested on its own.
type browseRoad struct {
	Class     fetch.TransportClass
	Candidate fetch.TransportCandidate
}

// Describe is the road in one phrase, for a chain row.
func (r browseRoad) Describe() string {
	switch r.Class {
	case fetch.ClassLive:
		return fmt.Sprintf("live peer at %s (priority %d)", r.Candidate.Address, r.Candidate.Priority)
	default:
		return fmt.Sprintf("static origin at %s (priority %d)",
			r.Candidate.Layout.Origin, r.Candidate.Priority)
	}
}

// dialableLive reports whether this consumer can actually open a
// connection for a live candidate, and why not when it cannot.
//
// **The families are the ones `entitysdk.AppPeer.Connect` dials, which is
// TCP and WebSocket — core-go implements `Connect` and `ConnectWebSocket`
// and nothing else.** An `http` transport profile is a legal, conformant
// thing for a peer to advertise and we cannot drive it: handing
// `http://host:port` to `Connect` falls into the TCP arm and dies inside
// `net.SplitHostPort` with *"too many colons"*, which reads as a
// malformed profile, i.e. as the publisher's fault for advertising
// something correct.
//
// That is AP44's shape — a refusal that asserts a fact about the world —
// so this one asserts a fact about US instead, by name.
func dialableLive(c fetch.TransportCandidate) (string, bool) {
	addr := strings.TrimSpace(c.Address)
	switch {
	case strings.HasPrefix(addr, "ws://"), strings.HasPrefix(addr, "wss://"):
		return "", true
	case strings.HasPrefix(addr, "tcp://"):
		return "", true
	case addr == "":
		return "the profile carries no endpoint URL", false
	case !strings.Contains(addr, "://"):
		// A bare host:port is what `Connect` treats as TCP, and §6.5.1
		// D-14's endpoint URL is expected to carry a scheme. Accepting it
		// is the lenient reading and it is the right one here: the
		// alternative refuses a profile we can certainly dial.
		return "", true
	default:
		scheme := addr[:strings.Index(addr, "://")]
		return fmt.Sprintf(
			"this browser dials tcp and websocket; %q is a transport family it has no client for "+
				"(the profile is fine — core-go implements Connect and ConnectWebSocket and no other)",
			scheme), false
	}
}

// roadsFor turns a ranked [fetch.TransportOptions] into the ordered list
// of roads THIS browser can drive, plus what it put aside and why.
//
// `havePeer` is the caller's statement that it can dispatch. It is passed
// in rather than read off the model so the choice is a pure function and
// can be gated without standing up a peer — the arm that matters most is
// *"a browser with no peer takes the static road even when live is
// offered and ranked first"*, and a model-bound version of this would
// need a whole peer to assert the absence of one.
func roadsFor(opts fetch.TransportOptions, havePeer bool) (roads []browseRoad, declined []string) {
	for _, c := range opts.Live() {
		if !havePeer {
			declined = append(declined, fmt.Sprintf(
				"%s: this browser holds no peer, so it cannot dispatch — a live read means asking "+
					"the publisher, and asking requires being one", c.Address))
			continue
		}
		if why, ok := dialableLive(c); !ok {
			declined = append(declined, c.Address+": "+why)
			continue
		}
		roads = append(roads, browseRoad{Class: fetch.ClassLive, Candidate: c})
	}
	for _, c := range opts.Static() {
		roads = append(roads, browseRoad{Class: fetch.ClassStatic, Candidate: c})
	}
	for _, s := range opts.Skipped {
		declined = append(declined, s.Reason)
	}
	return roads, declined
}

// offeredSummary is what to tell an operator when no road worked.
//
// It names every road that was tried and what it said, then the ones that
// were never tried and why. A refusal that says only *"could not reach
// this publisher"* sends them to check their network when the answer may
// be that the publisher advertises only a family we do not speak, or that
// they have not authorized us.
func offeredSummary(attempts []string, declined []string) string {
	var b strings.Builder
	if len(attempts) > 0 {
		b.WriteString("tried " + strings.Join(attempts, "; "))
	}
	if len(declined) > 0 {
		if b.Len() > 0 {
			b.WriteString(". ")
		}
		b.WriteString("not tried: " + strings.Join(declined, "; "))
	}
	if b.Len() == 0 {
		b.WriteString("the binding carries no transport this browser can use")
	}
	return b.String()
}

// travel walks the roads in order and returns the first verified root.
//
// The returned consumer is the one that answered, and the caller uses it
// for the rest of the navigation — the walk and every page fetch — so the
// chain describes one road and not a mixture. `attempts` is what each
// tried road said, for the refusal and for the chain row, in order.
//
// A road that fails is recorded and the next is tried; see the file note
// for why the fall-through stops at the first verified root and not one
// step later.
func (m *BrowseModel) travel(ctx context.Context, roads []browseRoad) (
	c *fetch.Consumer, root fetch.VerifiedRoot, used browseRoad, attempts []string, err error) {

	for _, r := range roads {
		cand, cerr := m.consumerForRoad(ctx, r)
		if cerr != nil {
			attempts = append(attempts, r.Describe()+" — "+cerr.Error())
			err = cerr
			continue
		}
		vr, rerr := cand.VerifiedRoot(ctx)
		if rerr != nil {
			attempts = append(attempts, r.Describe()+" — "+rerr.Error())
			err = rerr
			continue
		}
		attempts = append(attempts, r.Describe()+" — answered")
		return cand, vr, r, attempts, nil
	}
	if err == nil {
		err = fmt.Errorf("no transport this browser can use")
	}
	return nil, fetch.VerifiedRoot{}, browseRoad{}, attempts, err
}

// consumerForRoad builds (or reuses) the verifying reader for one road.
//
// The live arm dials before it reads. A navigation is an operator act —
// they typed an address or clicked a link — so dialing here is not the
// hazard that dialing from a wake is (`shellcmd/status.go`: a pass dials,
// a read must not). It is still a dial, which is why it happens once per
// publisher per session rather than once per navigation: the consumer is
// cached, and a cached consumer is also what gives the `seq` floor
// something to compare against.
//
// **The live cache is keyed by PUBLISHER, not by road**, so two live
// candidates for one peer collapse to one reader after the first one
// builds. That is deliberate and it is worth knowing what it costs: if
// address A dials but its root read fails, address B for the same peer
// returns the cached reader and fails identically rather than being
// tried. Correct in the case that actually happens — a 403 or a
// withholding publisher is about the peer and not about which socket
// reached it — and a real limitation for the mirror case, where two
// addresses front genuinely different machines. Named rather than
// guessed at: nothing in this cohort publishes two live profiles for one
// peer yet, so there is no measurement saying which way to resolve it.
// A failed BUILD (the dial itself) is never cached, so a dead address
// followed by a live one works.
func (m *BrowseModel) consumerForRoad(ctx context.Context, r browseRoad) (*fetch.Consumer, error) {
	if r.Class != fetch.ClassLive {
		return m.consumerFor(r.Candidate.Layout), nil
	}
	peerID := r.Candidate.PeerID
	key := "entity://" + peerID
	m.mu.Lock()
	cached, ok := m.consumers[key]
	ap := m.peer
	m.mu.Unlock()
	if ok {
		return cached, nil
	}
	if ap == nil {
		return nil, fmt.Errorf("this browser holds no peer, so it cannot dispatch")
	}
	// Dial only when there is an address to dial and it is not us.
	// Reading our OWN published site live is a real case and the cheapest
	// control arm there is; dialing ourselves is not. An EMPTY address is
	// the peer-id-address case (`canReachLive`): the connection already
	// exists, so there is nothing to open and nothing to guess — dialing
	// "" would turn a working read into a transport error.
	if peerID != ap.PeerID() && strings.TrimSpace(r.Candidate.Address) != "" {
		if _, err := ap.Connect(ctx, r.Candidate.Address); err != nil {
			return nil, fmt.Errorf("dial %s: %w", r.Candidate.Address, err)
		}
	}
	c, err := NewPeerConsumer(ap, peerID, m.cache)
	if err != nil {
		return nil, err
	}
	c.UseFloor(m.floorFor(peerID))

	m.mu.Lock()
	defer m.mu.Unlock()
	// Re-check under the lock: two navigations can race here, and two
	// consumers for one publisher is exactly what the floor is shared to
	// survive — but keeping one is still cheaper and keeps the cache
	// stats honest.
	if again, ok := m.consumers[key]; ok {
		return again, nil
	}
	m.consumers[key] = c
	return c, nil
}

// canReachLive answers whether a peer-id address can be read live with
// no origin and no binding, and says why not when it cannot.
//
// **A peer-id carries no address** (NETWORK §6.5.4 puts profile
// distribution out of band in v1), so this is not "can we find them" —
// it is "are we already in touch". Two cases qualify and no others:
//
//   - It is US. A peer reading its own published site takes the identical
//     dispatched path, which is what makes it the cheapest control arm
//     there is.
//   - We hold a connection to them. Then the address exists: it is the
//     socket. Note the pool holds inbound sessions too and tags neither
//     direction (AP82's neighbour), so this can be true for a peer we
//     have never dialled — and that is fine here, because the worst case
//     is a dispatch that fails and a chain row saying so. It would NOT be
//     fine for minting a grant or binding an identity, which is why this
//     predicate is named for the one question it answers.
//
// Anything else declines. **It does not dial a guess**: there is nothing
// to guess from, and inventing an address would be the one move that can
// put a stranger's bytes behind a peer-id the operator typed.
func (m *BrowseModel) canReachLive(peerID string) (why string, ok bool) {
	if peerID == "" {
		return "no peer-id in the address", false
	}
	m.mu.Lock()
	ap := m.peer
	m.mu.Unlock()
	if ap == nil {
		return "this browser holds no peer, so it cannot dispatch", false
	}
	if peerID == ap.PeerID() {
		return "", true
	}
	for _, p := range ap.ConnectedPeers() {
		if p.PeerID == peerID {
			return "", true
		}
	}
	return "this peer holds no connection to " + peerID +
		", and a peer-id carries no address to dial", false
}

// floorFor is the §3-RES.4 monotonicity memory for one publisher.
//
// **Keyed by peer-id and nothing else**, which is the whole point: the
// floor is a statement about a publisher's published-root sequence, and
// the road that carried a root is not part of that statement. Two
// consumers for one publisher — a live one and a static one, or the
// re-based and discovered layouts `consumerFor` already keys apart —
// share this, so a reader that verified seq=6 on one road cannot be
// handed a correctly-signed seq=3 on the other.
func (m *BrowseModel) floorFor(peerID string) *fetch.SeqFloor {
	m.mu.Lock()
	defer m.mu.Unlock()
	if f, ok := m.floors[peerID]; ok {
		return f
	}
	f := fetch.NewSeqFloor()
	m.floors[peerID] = f
	return f
}
