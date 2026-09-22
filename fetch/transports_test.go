package fetch

import (
	"context"
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/types"
)

// transports_test.go — the ranking, driven end to end through the real
// decoders and a real content fetch.
//
// **These run through [Registry.TransportsFor] rather than against
// `rankCandidates` directly**, on purpose. The rule under test is a
// specification's (EXTENSION-NETWORK §6.5.1a D1), and a test that reaches
// past the decode to sort a hand-built slice would be asserting on our
// copy of the rule while the thing the product calls — decode the entity,
// class it, pull `priority` out of whichever of four profile shapes it
// is — went uncovered. That is the shape `TestAcceptSeqIsTheRealFloor`
// was written to correct, one file over.
//
// The blob server is cache_test.go's, so the profile entities are served
// over real HTTP at real content URLs and come back through
// `Consumer.Blob`'s hash check.
//
// Tier: integration (real HTTP, real content-addressed fetch).

// transportTarget is the peer a binding targets. It never has to be
// identity-form here — nothing in these tests verifies a root, and
// borrowing the registry's would hide a peer-id cross-check failure
// behind an accidental match.
const transportTarget = "2KFNrGARQBkx3d9WQtkeuT3HbQ3oXSS7PBggWt1szT9hzb"

// transportRig is a registry whose content store serves whatever profile
// entities a test puts in it.
type transportRig struct {
	blobs *blobServer
	reg   *Registry
}

func newTransportRig(t *testing.T) *transportRig {
	t.Helper()
	b := newBlobServer(t)
	layout := Layout{
		Origin: b.srv.URL,
		PeerID: srcTestPeer,
		Endpoint: types.TransportEndpoint{
			ContentURLPrefix: "/content",
			ContentLayout:    types.ContentLayoutFlat,
			TreeURLPrefix:    "/tree",
			TreeLeafSuffix:   ".bin",
		},
	}
	reg, err := NewRegistry(layout, b.srv.Client())
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return &transportRig{blobs: b, reg: reg}
}

// serve files a profile entity in the registry's content store and
// returns the by-hash `transports` entry that names it — the shape
// EXTENSION-REGISTRY §3's `[MUST, v1.21]` requires.
func (r *transportRig) serve(t *testing.T, ent entity.Entity, err error) TransportRef {
	t.Helper()
	if err != nil {
		t.Fatalf("building profile entity: %v", err)
	}
	h := r.blobs.put(t, ent.Type, ent.Data)
	return TransportRef{Kind: TransportByHash, Hash: h}
}

// servePoll and serveTCP are `serve` with a profile builder folded in.
// Go will not expand a two-value call into a call that already has a
// leading argument, so the pairing has to happen here rather than at
// every call site.
func (r *transportRig) servePoll(t *testing.T, tag string, priority *uint64, advertisedAt uint64) TransportRef {
	t.Helper()
	ent, err := pollProfile(tag, priority, advertisedAt)
	return r.serve(t, ent, err)
}

func (r *transportRig) serveTCP(t *testing.T, addr string, priority *uint64) TransportRef {
	t.Helper()
	ent, err := tcpProfile(addr, priority)
	return r.serve(t, ent, err)
}

func (r *transportRig) options(t *testing.T, refs ...TransportRef) TransportOptions {
	t.Helper()
	res := NameResolution{
		Name:       "example.test",
		Normalized: "example.test",
		Binding: NameBinding{
			Name:         "example.test",
			Kind:         types.BackendKindPeerIssued,
			TargetPeerID: transportTarget,
			Transports:   refs,
		},
	}
	return r.reg.TransportsFor(context.Background(), res, r.blobs.srv.URL)
}

func (r *transportRig) originFor(t *testing.T, refs ...TransportRef) (Origin, error) {
	t.Helper()
	res := NameResolution{
		Name:       "example.test",
		Normalized: "example.test",
		Binding: NameBinding{
			Name:         "example.test",
			TargetPeerID: transportTarget,
			Transports:   refs,
		},
	}
	return r.reg.OriginFor(context.Background(), res, r.blobs.srv.URL)
}

func prio(v uint64) *uint64 { return &v }

// pollProfile is an http-poll profile that differs from its siblings only
// in the fields under test. The tree prefix varies so a test can tell
// which one it got back.
func pollProfile(tag string, priority *uint64, advertisedAt uint64) (entity.Entity, error) {
	return types.HTTPPollProfileData{
		PeerID:        transportTarget,
		TransportType: "http-poll",
		Endpoint: types.TransportEndpoint{
			ManifestURLPrefix: "/" + tag + "/manifest",
			TreeURLPrefix:     "/" + tag + "/tree",
			ContentURLPrefix:  "/" + tag + "/content",
			ContentLayout:     types.ContentLayoutFlat,
		},
		SupportedOps:  []string{types.OpTreeGet, types.OpContentGet, types.OpManifestGet},
		Freshness:     "static-immutable+signed-pointer",
		NonceRequired: false,
		CapFlow:       "egress",
		Priority:      priority,
		AdvertisedAt:  advertisedAt,
	}.ToEntity()
}

func tcpProfile(addr string, priority *uint64) (entity.Entity, error) {
	return types.TCPProfileData{
		PeerID:        transportTarget,
		TransportType: "tcp",
		Endpoint:      types.TransportEndpointURL{URL: addr},
		SupportedOps:  []string{types.OpExecute},
		Freshness:     "live",
		NonceRequired: true,
		CapFlow:       "both",
		Priority:      priority,
	}.ToEntity()
}

func treePrefixes(cands []TransportCandidate) []string {
	out := make([]string, len(cands))
	for i, c := range cands {
		out[i] = c.Profile.Endpoint.TreeURLPrefix
	}
	return out
}

// TestTransportsAreRankedByPriorityNotByBindingOrder is the conformance
// fix this file exists for.
//
// EXTENSION-NETWORK §6.5.1a D1 (Amendment 8, Q1) orders candidates by
// `priority` ascending, lower being preferred, and EXTENSION-REGISTRY
// §4.1.1 points a binding's `transports` at that rule by name. Until this
// landed we read `priority` nowhere at all and took whichever entry the
// registry happened to list first — so a publisher who marked its
// preferred origin `0` and a slow mirror `100` got the mirror, silently,
// with no wrong answer anywhere for anyone to catch.
//
// **The binding order is deliberately the reverse of the priority
// order**, which is this test's anti-vacuity arm: with the two agreeing,
// a build that ignores `priority` entirely passes.
func TestTransportsAreRankedByPriorityNotByBindingOrder(t *testing.T) {
	rig := newTransportRig(t)
	slow := rig.servePoll(t, "slow", prio(100), 0)
	mid := rig.servePoll(t, "mid", prio(50), 0)
	fast := rig.servePoll(t, "fast", prio(0), 0)

	opts := rig.options(t, slow, mid, fast)
	if len(opts.Skipped) != 0 {
		t.Fatalf("three good http-poll profiles and %d were skipped: %v", len(opts.Skipped), opts.Skipped)
	}
	got := treePrefixes(opts.Candidates)
	want := []string{"/fast/tree", "/mid/tree", "/slow/tree"}
	for i := range want {
		if i >= len(got) || got[i] != want[i] {
			t.Fatalf("ranked order = %v, want %v — §6.5.1a D1 is (priority asc), and the binding "+
				"lists these in the opposite order on purpose", got, want)
		}
	}
	// The chosen static origin is the ranked head, not the listed head.
	origin, err := rig.originFor(t, slow, mid, fast)
	if err != nil {
		t.Fatalf("OriginFor: %v", err)
	}
	if origin.Layout.Endpoint.TreeURLPrefix != "/fast/tree" {
		t.Errorf("OriginFor chose %q; want the priority-0 profile /fast/tree",
			origin.Layout.Endpoint.TreeURLPrefix)
	}
}

// TestAdvertisedAtIsNotASelectionKey pins a MUST NOT.
//
// §6.5.1a D3: `advertised_at` is wall-clock, skew-prone and
// **informational only** — *"Not a selection key"*. A consumer that let
// it break ties would rank publishers by whose clock runs fastest, and
// the ordering would change under a peer that did nothing but re-assert
// an unchanged profile.
//
// The two profiles are otherwise identical in every field D1 reads, so a
// reorder here can only have come from the one field that must not
// produce one.
func TestAdvertisedAtIsNotASelectionKey(t *testing.T) {
	rig := newTransportRig(t)
	first := rig.servePoll(t, "first", prio(10), 1_000)
	// Far newer, and listed second. If `advertised_at` were consulted at
	// all it would come first.
	second := rig.servePoll(t, "second", prio(10), 9_999_999_999)

	opts := rig.options(t, first, second)
	got := treePrefixes(opts.Candidates)
	if len(got) != 2 || got[0] != "/first/tree" {
		t.Fatalf("order = %v; a much later advertised_at reordered the list, which §6.5.1a D3 "+
			"makes a MUST NOT", got)
	}
}

// TestAbsentPriorityTakesTheDefaultAndSaysItWasAbsent covers the other
// half of D1's key.
//
// `priority` is OPTIONAL and pointer-typed on the wire precisely so an
// omitted value is distinguishable from an explicit `0`; absent means
// 100. [TransportCandidate.PriorityDeclared] keeps that distinction,
// because *"expressed no preference"* is what makes our stable-order
// tie-break harmless and *"asked for 100"* is not the same statement.
func TestAbsentPriorityTakesTheDefaultAndSaysItWasAbsent(t *testing.T) {
	rig := newTransportRig(t)
	ahead := rig.servePoll(t, "ahead", prio(50), 0)
	silent := rig.servePoll(t, "silent", nil, 0)
	behind := rig.servePoll(t, "behind", prio(200), 0)

	opts := rig.options(t, silent, behind, ahead)
	got := treePrefixes(opts.Candidates)
	want := []string{"/ahead/tree", "/silent/tree", "/behind/tree"}
	for i := range want {
		if i >= len(got) || got[i] != want[i] {
			t.Fatalf("order = %v, want %v — an absent priority is %d, not 0 and not last",
				got, want, DefaultTransportPriority)
		}
	}
	for _, c := range opts.Candidates {
		declared := c.Profile.Endpoint.TreeURLPrefix != "/silent/tree"
		if c.PriorityDeclared != declared {
			t.Errorf("%s: PriorityDeclared = %v, want %v",
				c.Profile.Endpoint.TreeURLPrefix, c.PriorityDeclared, declared)
		}
	}
}

// TestLiveProfilesAreCandidatesAndNotDecodeFailures is the second half of
// the defect.
//
// Every non-http-poll profile used to go through a decoder that insisted
// on http-poll and came back *"transport is type
// system/peer/transport/tcp, not system/peer/transport/http-poll"*,
// landing in `Skipped` as though the binding were malformed. It is not
// malformed: it is a peer saying **dial me**, and that is the entire
// input to the mode choice.
func TestLiveProfilesAreCandidatesAndNotDecodeFailures(t *testing.T) {
	rig := newTransportRig(t)
	live := rig.serveTCP(t, "tcp://peer.example:9110", prio(0))
	static := rig.servePoll(t, "cdn", prio(100), 0)

	opts := rig.options(t, static, live)
	if len(opts.Skipped) != 0 {
		t.Fatalf("a well-formed tcp profile was skipped: %v", opts.Skipped)
	}
	if len(opts.Live()) != 1 || len(opts.Static()) != 1 {
		t.Fatalf("class split = %d live / %d static, want 1 and 1",
			len(opts.Live()), len(opts.Static()))
	}
	got := opts.Live()[0]
	if got.Address != "tcp://peer.example:9110" {
		t.Errorf("live address = %q", got.Address)
	}
	if got.Type != types.TypePeerTransportTCP {
		t.Errorf("live type = %q, want %q", got.Type, types.TypePeerTransportTCP)
	}
	// The ranked head is the live one, because the publisher said so —
	// ranking is priority-ordered across the whole list and the CLASS
	// choice belongs to the caller, not to the sort.
	if opts.Candidates[0].Class != ClassLive {
		t.Errorf("ranked head is %s; the live profile declared priority 0", opts.Candidates[0].Class)
	}
}

// TestOriginForNamesTheLiveTransportItCannotUse is the refusal arm.
//
// A static-only consumer (`entity-fetch` links no peer) meeting a
// live-only binding must say what it saw. Reporting *"no usable
// transport"* sends an operator to the registry to fix a binding that is
// correct; the honest sentence is that this reader cannot dial.
func TestOriginForNamesTheLiveTransportItCannotUse(t *testing.T) {
	rig := newTransportRig(t)
	live := rig.serveTCP(t, "tcp://peer.example:9110", nil)

	_, err := rig.originFor(t, live)
	if err == nil {
		t.Fatal("a binding with no http-poll transport produced an http-poll origin")
	}
	for _, want := range []string{"tcp://peer.example:9110", "no way to dial", types.TypePeerTransportTCP} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not mention %q:\n  %v", want, err)
		}
	}

	// Anti-vacuity: the same consumer against a binding that commits BOTH
	// still gets the static one, unchanged. Without this arm the test
	// above is satisfied by an OriginFor that refuses everything.
	static := rig.servePoll(t, "cdn", nil, 0)
	origin, err := rig.originFor(t, live, static)
	if err != nil {
		t.Fatalf("a binding committing both refused the static one: %v", err)
	}
	if origin.Layout.Endpoint.TreeURLPrefix != "/cdn/tree" {
		t.Errorf("static origin = %q, want /cdn/tree", origin.Layout.Endpoint.TreeURLPrefix)
	}
}

// TestEqualPrioritiesDiscloseTheSubstituteTieBreak is the disclosure this
// repository owes rather than a behaviour it wants.
//
// D1's second key is `profile-id`, *"the final path segment of
// `system/peer/transport/{peer_id}/{profile-id}`"* — and a registry
// binding carries profiles by content hash, so the key does not survive
// the carriage. We fall back to the order the binding lists, which is
// deterministic and is **not the specification's rule**, so the surface
// is told rather than left to assume the publisher's preference was
// honored in full. Routed as arch A-35.
func TestEqualPrioritiesDiscloseTheSubstituteTieBreak(t *testing.T) {
	rig := newTransportRig(t)
	a := rig.servePoll(t, "a", prio(10), 0)
	b := rig.servePoll(t, "b", prio(10), 0)

	tied := rig.options(t, a, b)
	if tied.TieBreak == "" {
		t.Error("two equal-priority transports were ordered with no disclosure of how")
	}
	if !strings.Contains(tied.TieBreak, "profile-id") {
		t.Errorf("the disclosure does not name the key that went missing: %q", tied.TieBreak)
	}

	// The control arm: distinct priorities are settled by the rule
	// itself, and claiming a substitute tie-break there would be noise
	// that trains a reader to ignore the real one.
	c := rig.servePoll(t, "c", prio(20), 0)
	distinct := rig.options(t, a, c)
	if distinct.TieBreak != "" {
		t.Errorf("nothing tied and a tie-break was reported: %q", distinct.TieBreak)
	}
}

// TestAProfileNamingAnotherPeerIsRefused keeps a check that predates this
// file and now runs through it.
//
// The registry signed a binding to peer X carrying a profile that says
// peer Y. The profile's peer-id is the key the target's content
// signature is checked against, so following it would verify the wrong
// publisher perfectly — a green chain about somebody else.
func TestAProfileNamingAnotherPeerIsRefused(t *testing.T) {
	rig := newTransportRig(t)
	other, err := types.HTTPPollProfileData{
		PeerID:        srcTestPeer, // not the binding's target
		TransportType: "http-poll",
		Endpoint:      types.TransportEndpoint{TreeURLPrefix: "/other/tree"},
		SupportedOps:  []string{types.OpTreeGet},
		Freshness:     "static-immutable+signed-pointer",
	}.ToEntity()
	ref := rig.serve(t, other, err)

	opts := rig.options(t, ref)
	if len(opts.Candidates) != 0 {
		t.Fatalf("a profile for a different peer was accepted: %+v", opts.Candidates)
	}
	if len(opts.Skipped) != 1 || !strings.Contains(opts.Skipped[0].Reason, "binding targets") {
		t.Fatalf("skip reason does not name the mismatch: %+v", opts.Skipped)
	}
}

// TestEndpointlessLiveProfileIsNamedRatherThanCalledMalformed covers a
// real shape we cannot drive.
//
// §6.5.2d admits a live profile with **no endpoint** — an advertisement
// of *reachability by negotiation* rather than an address, which the
// §10.3 `establish_live` seam is the consumer of. We dial and do not
// negotiate, so it is correctly unusable here; what it must not do is
// read as a corrupt profile, which would send an operator to fix
// something that is conformant.
func TestEndpointlessLiveProfileIsNamedRatherThanCalledMalformed(t *testing.T) {
	rig := newTransportRig(t)
	ref := rig.serveTCP(t, "", nil)

	opts := rig.options(t, ref)
	if len(opts.Candidates) != 0 {
		t.Fatalf("a profile with no address became a candidate: %+v", opts.Candidates)
	}
	if len(opts.Skipped) != 1 {
		t.Fatalf("skipped = %+v", opts.Skipped)
	}
	if !strings.Contains(opts.Skipped[0].Reason, "negotiation") {
		t.Errorf("the reason does not name what the profile actually is: %q", opts.Skipped[0].Reason)
	}
}

// TestAnUnknownTransportFamilyIsSkippedByName is the forward-compatibility
// arm.
//
// A profile family from a newer vocabulary is not a malformed binding.
// The distinction the catalogue keeps insisting on: *"three transports,
// none of them ones we speak"* and *"no transports"* send an operator to
// two different places.
func TestAnUnknownTransportFamilyIsSkippedByName(t *testing.T) {
	rig := newTransportRig(t)
	ent, err := entity.NewEntity("system/peer/transport/carrier-pigeon",
		enc(t, map[string]string{"peer_id": transportTarget, "transport_type": "carrier-pigeon"}))
	ref := rig.serve(t, ent, err)

	opts := rig.options(t, ref)
	if len(opts.Candidates) != 0 {
		t.Fatalf("an unknown family became a candidate: %+v", opts.Candidates)
	}
	if len(opts.Skipped) != 1 || !strings.Contains(opts.Skipped[0].Reason, "carrier-pigeon") {
		t.Fatalf("skip reason does not name the family: %+v", opts.Skipped)
	}
	// And the summary a refusal prints distinguishes it from an empty
	// binding, which is the whole point of keeping the entry.
	if got := opts.Offered(); !strings.Contains(got, "1 unusable") {
		t.Errorf("Offered() = %q; an unusable transport must not read as no transport", got)
	}
}

// TestOfferedDistinguishesAnEmptyBindingFromAnUnspeakableOne is the
// control for the sentence above.
func TestOfferedDistinguishesAnEmptyBindingFromAnUnspeakableOne(t *testing.T) {
	rig := newTransportRig(t)
	empty := rig.options(t)
	if !strings.Contains(empty.Offered(), "no transports at all") {
		t.Errorf("an empty binding reads as %q", empty.Offered())
	}
}

// TestAnInlineLiveProfileIsRefusedRatherThanMislabelled covers the one
// place a shape can be misread instead of merely declined.
//
// An inline entry carries no entity type, so `transport_type` — the field
// §6.5.1a D5 demotes precisely because it is self-description — is the
// only discriminator available. The decoder admits anything that fits the
// http-poll struct with a non-empty `transport_type`, and a live profile
// fits it with an empty endpoint. Labelled http-poll, that entry becomes
// a static candidate whose layout has no manifest prefix, and the failure
// surfaces three layers down as *"this publisher advertises no signed
// entry point"* — the publisher's fault, on our screen, for a shape we
// were the ones to decline.
func TestAnInlineLiveProfileIsRefusedRatherThanMislabelled(t *testing.T) {
	rig := newTransportRig(t)
	inline := TransportRef{Kind: TransportInline, Profile: types.HTTPPollProfileData{
		PeerID:        transportTarget,
		TransportType: "tcp",
	}}

	opts := rig.options(t, inline)
	if len(opts.Candidates) != 0 {
		t.Fatalf("an inline tcp entry became a %s candidate: %+v",
			opts.Candidates[0].Class, opts.Candidates[0])
	}
	if len(opts.Skipped) != 1 || !strings.Contains(opts.Skipped[0].Reason, "transport_type") {
		t.Fatalf("the refusal does not name what it declined: %+v", opts.Skipped)
	}

	// Anti-vacuity: a genuine inline http-poll entry — the shape the only
	// live federation actually publishes — still works. Without this arm
	// the check above is satisfied by refusing every inline transport,
	// which would make this repository unable to resolve a name in the
	// one federation it can reach.
	good := TransportRef{Kind: TransportInline, Profile: types.HTTPPollProfileData{
		PeerID:        transportTarget,
		TransportType: "http-poll",
		Endpoint: types.TransportEndpoint{
			ManifestURLPrefix: "/legacy/manifest",
			TreeURLPrefix:     "/legacy/tree",
			ContentURLPrefix:  "/legacy/content",
			ContentLayout:     types.ContentLayoutFlat,
		},
	}}
	ok := rig.options(t, good)
	if len(ok.Candidates) != 1 || ok.Candidates[0].Class != ClassStatic {
		t.Fatalf("a legitimate inline http-poll transport was refused: %+v", ok.Skipped)
	}
}
