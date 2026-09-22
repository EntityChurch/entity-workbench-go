package fetch

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"go.entitychurch.org/entity-core-go/core/entity"
	"go.entitychurch.org/entity-core-go/core/hash"
	"go.entitychurch.org/entity-core-go/core/types"
)

// transports.go — a binding's `transports` become a RANKED candidate
// list, by the specification's rule rather than by one of ours.
//
// # What was here before this file, and why it was wrong
//
// [Registry.OriginFor] walked `res.Binding.Transports` in array order and
// returned the first entry that decoded as an http-poll profile. Two
// defects in that one line, and neither of them announces itself:
//
//   - **Array order is not the selection rule.** EXTENSION-NETWORK
//     §6.5.1a D1 (Amendment 8, Q1) orders a peer's candidate profiles by
//     `(priority asc, profile-id lex)`, and EXTENSION-REGISTRY §4.1.1
//     points a binding's `transports` straight at it: *"the same
//     `target_peer_id` may publish multiple `transports` (per NETWORK
//     §6.5 priority-selection) covering distinct addresses, and a
//     binding's `transports` field MAY enumerate several."* We read
//     `priority` in no place at all, so a publisher that marked its
//     preferred origin `priority: 0` and a slow mirror `priority: 100`
//     got whichever the registry happened to list first — silently, with
//     no wrong answer for anyone to catch, and looking like the
//     publisher's own misconfiguration.
//   - **A live profile read as a decode failure.** Everything that was
//     not http-poll went through `httpPollProfile` and came back
//     *"transport is type system/peer/transport/tcp, not
//     system/peer/transport/http-poll"*, landing in `Skipped` as though
//     the binding were malformed. It is not malformed: it is a peer
//     saying *dial me*, which is the whole input to the mode choice this
//     package's second [Source] exists to make.
//
// # The class split, and why this package does not make the choice
//
// A profile is in one of two classes as far as a consumer is concerned,
// and the classes are not a spec vocabulary — they are what a consumer
// would DO with the thing:
//
//	static  http-poll             fetch a signed root off an HTTP origin
//	live    tcp / http / websocket  dial the peer and dispatch at it
//
// **Ranking within a class is the specification's; choosing between the
// classes is the caller's.** D1 sorts profiles *"of the wanted
// `transport_type`"* — it answers *which of these mirrors* and says
// nothing about *static or live*, because that depends on what the
// consumer can actually do. `entity-fetch` links no peer and can only
// ever take the static road; a peer-backed browser can take either. So
// this file ranks and reports, and [TransportOptions.Static] /
// [TransportOptions.Live] are how a caller says which roads it has.
//
// # The tie-break key is not carried by either of the specification's
// own carriages, and that is a finding rather than a limitation of ours
//
// D1's second key is `profile-id`, defined as *"the final path segment of
// `system/peer/transport/{peer_id}/{profile-id}`"*. A profile entity
// carries no `profile_id` field — the §6.5.1 schema block has ten fields
// and that is not one of them — so the key exists only for a consumer
// reading profiles **at their own paths in the peer's tree**.
//
// Neither carriage that moves profiles between parties preserves it. A
// registry binding carries bare content hashes (REGISTRY §3, *"`transports`
// carries hashes, not endpoint objects"*, a `[MUST, v1.21]`), and a
// content-addressed fetch returns the entity and no path. A
// `system/peer/transport-set` (§6.5.1c) carries members **inline**, which
// is likewise pathless — and that section states outright that *"array
// order is NOT significant: §6.5.1a D1's (priority asc, profile-id lex)
// remains the only selection rule"*, while its consumer rule 6 requires
// consumers to *"attempt profiles in §6.5.1a D1 order"*. So a consumer
// holding two equal-`priority` profiles through either carriage has, by
// the text, no defined order and no way to obtain one.
//
// **What we do about it, and the part that matters is that we say so.**
// The sort is stable, so equal-priority candidates keep the order the
// binding listed them in. That is deterministic for one binding and it is
// NOT the specification's tie-break; [TransportOptions.TieBreak] reports
// which of the two happened, so a surface can name it instead of
// implying the publisher's preference was honored in full. Routed to arch
// as A-35 — and note that inventing a local tie-break (hash order, say)
// would be worse than this: it is deterministic per consumer and
// *different per implementation*, which is the failure the rule exists to
// prevent, wearing the appearance of having followed it.

// TransportClass is what a consumer would do with a profile, which is a
// coarser question than its `transport_type` and the one that decides
// which [Source] reads it.
type TransportClass string

const (
	// ClassStatic is an http-poll profile: a signed root and a content
	// store on an HTTP origin, read by [HTTPSource].
	ClassStatic TransportClass = "static"
	// ClassLive is a dialable peer transport — tcp, http, websocket —
	// read by dispatching at the peer itself. The [Source] for it needs a
	// peer and therefore lives in `workbench`, not here.
	ClassLive TransportClass = "live"
)

// DefaultTransportPriority is §6.5.1a D1's default for a profile that
// declares none: **100, and lower is more preferred** (DNS-SRV
// semantics). `priority` is OPTIONAL (Amendment 8, Q1) and pointer-typed
// on the wire precisely so an omitted value is distinguishable from an
// explicit zero.
const DefaultTransportPriority uint64 = 100

// TransportCandidate is one of a binding's transports, decoded, classed
// and ready to be used or reported.
type TransportCandidate struct {
	// Class is which road this is.
	Class TransportClass
	// Type is the profile's entity type, e.g. `system/peer/transport/tcp`.
	// **The entity type is authoritative and the `transport_type` field is
	// not** (§6.5.1a D5); core-go's decoders fail closed on a mismatch and
	// we let them.
	Type string
	// Priority is §6.5.1a D1's key, defaulted per [DefaultTransportPriority].
	Priority uint64
	// PriorityDeclared is false when the profile omitted `priority` and
	// the default was applied. Carried because *"this publisher expressed
	// no preference"* and *"this publisher asked for 100"* are different
	// facts, and only the first one makes our stable-order tie-break
	// harmless.
	PriorityDeclared bool
	// Ref is the binding entry this came from, including which of the two
	// cohort carriage shapes it arrived in.
	Ref TransportRef
	// Hash is the profile's content hash, set when it was carried by one.
	Hash hash.Hash

	// Layout is the reachable endpoint, set for [ClassStatic] only.
	Layout Layout
	// Profile is the decoded http-poll body, set for [ClassStatic] only.
	Profile types.HTTPPollProfileData

	// Address is the dialable endpoint URL, set for [ClassLive] only —
	// `endpoint.url`, e.g. `tcp://host:port`. It is the profile's own
	// single-field §6.5.1 D-14 shape and is passed along verbatim; this
	// package neither parses nor dials it.
	Address string
	// PeerID is the peer this profile claims to describe.
	//
	// **It is checked against the binding's `target_peer_id` and is never
	// trusted on its own.** §6.5.1a D7 is explicit that the inner field is
	// self-description and not evidence: anyone can mint a profile naming
	// any peer. What makes this usable is the registry's signature over
	// the binding that carries it, plus the handshake at the far end —
	// never the field.
	PeerID string
}

// TransportOptions is everything a binding's `transports` offered, ranked
// and with the declines kept.
//
// **`Skipped` is not debris.** *"This binding carries no transport"* and
// *"this binding carries three and we speak none of them"* are different
// operator problems with different next steps, and a first-fit loop that
// returns one error for both erases the difference — which is why the
// refusal below prints what was on offer.
type TransportOptions struct {
	// Candidates is every usable transport, sorted by §6.5.1a D1.
	Candidates []TransportCandidate
	// Skipped is every entry that was not usable, each with its reason.
	Skipped []SkippedTransport
	// TieBreak names how equal-priority candidates were ordered. Empty
	// when nothing tied. See the file note: the specification's `profile-id`
	// key is not carried by a registry binding, so this is where the
	// substitute is disclosed rather than assumed.
	TieBreak string
}

// Static is the candidates a consumer with only an HTTP client can use.
func (o TransportOptions) Static() []TransportCandidate { return o.byClass(ClassStatic) }

// Live is the candidates a consumer with a peer can dial.
func (o TransportOptions) Live() []TransportCandidate { return o.byClass(ClassLive) }

func (o TransportOptions) byClass(c TransportClass) []TransportCandidate {
	var out []TransportCandidate
	for _, cand := range o.Candidates {
		if cand.Class == c {
			out = append(out, cand)
		}
	}
	return out
}

// Offered is a one-line summary of what the binding carried, for a
// refusal that has to say what it saw.
func (o TransportOptions) Offered() string {
	if len(o.Candidates) == 0 && len(o.Skipped) == 0 {
		return "the binding carries no transports at all"
	}
	parts := make([]string, 0, len(o.Candidates)+1)
	for _, c := range o.Candidates {
		parts = append(parts, fmt.Sprintf("%s (%s, priority %d)", c.Type, c.Class, c.Priority))
	}
	if len(o.Skipped) > 0 {
		parts = append(parts, fmt.Sprintf("%d unusable: %s", len(o.Skipped), skippedSummary(o.Skipped)))
	}
	return strings.Join(parts, "; ")
}

// TransportsFor resolves a binding's `transports` into the ranked
// candidate list of [TransportOptions].
//
// `origin` is where an http-poll candidate's origin-relative URLs are
// rooted; see [Registry.OriginFor] for the reasoning, which is unchanged.
// It has no bearing on a live candidate, whose address is absolute in its
// own profile.
//
// **It does not fail when nothing is usable.** An empty candidate list
// with populated `Skipped` is a complete and useful answer — it is what
// lets a caller say *"this peer offers only transports we do not speak"*
// rather than *"no transport"*. Only a caller that needed a particular
// class turns that into an error, and [Registry.OriginFor] is the one
// that does.
func (r *Registry) TransportsFor(ctx context.Context, res NameResolution, origin string) TransportOptions {
	if origin == "" {
		origin = OriginRoot(r.Layout.Origin)
	}
	var out TransportOptions
	for _, ref := range res.Binding.Transports {
		cand, err := r.candidateFor(ctx, ref, res.Binding.TargetPeerID, origin)
		if err != nil {
			out.Skipped = append(out.Skipped, SkippedTransport{Hash: ref.Hash, Reason: err.Error()})
			continue
		}
		out.Candidates = append(out.Candidates, cand)
	}
	rankCandidates(&out)
	return out
}

// rankCandidates applies §6.5.1a D1 and records how ties were settled.
//
// **`sort.SliceStable` is doing normative work here**, not tidying: a
// stable sort leaves equal-priority candidates in the order the binding
// listed them, which is the only ordering available once `profile-id` is
// gone (file note). An unstable sort would make the choice depend on Go's
// pivot selection, i.e. on nothing, and two runs of the same consumer
// over the same binding could disagree.
//
// **`advertised_at` is not consulted, and that is a MUST** (§6.5.1a D3):
// it is wall-clock and skew-prone, so a publisher whose clock runs fast
// would otherwise outrank one whose clock is right.
func rankCandidates(o *TransportOptions) {
	sort.SliceStable(o.Candidates, func(i, j int) bool {
		return o.Candidates[i].Priority < o.Candidates[j].Priority
	})

	tied := false
	for i := 1; i < len(o.Candidates); i++ {
		if o.Candidates[i].Priority == o.Candidates[i-1].Priority {
			tied = true
			break
		}
	}
	if !tied {
		return
	}
	o.TieBreak = "equal-priority transports were left in the order the binding lists them: " +
		"§6.5.1a D1's `profile-id` tie-break is the final segment of a tree path, and a binding " +
		"carries profiles by content hash, so the key does not survive the carriage (arch A-35)"
}

// candidateFor decodes one `transports` entry into a usable candidate.
func (r *Registry) candidateFor(ctx context.Context, ref TransportRef, targetPeerID, origin string) (TransportCandidate, error) {
	switch ref.Kind {
	case TransportInline:
		// The pre-ruling inline shape, which this package still reads
		// (see [NameBinding]).
		//
		// **An inline entry has no entity type, so the `transport_type`
		// field is the only discriminator there is** — the exact
		// situation §6.5.1a D5 calls demoted, and the reason REGISTRY §3
		// now `[MUST, v1.21]`s the by-hash form. `decodeTransportRef`
		// admits any entry that decodes into the http-poll struct with a
		// non-empty `transport_type`, and a live profile decodes into it
		// perfectly well with an empty endpoint — so without this check a
		// `tcp` entry would be labelled http-poll and then fail three
		// layers down with *"advertises no manifest_url_prefix"*, which
		// reads as a broken publisher rather than as a shape we declined.
		// There has never been an inline live profile in this cohort;
		// this refuses one by name rather than mislabelling it.
		if tt := ref.Profile.TransportType; tt != "" && tt != "http-poll" {
			return TransportCandidate{}, fmt.Errorf(
				"inline transport declares transport_type %q — only the http-poll shape is "+
					"carriable inline, and a live profile must be referenced by hash so its "+
					"entity type can be the authority (§6.5.1a D5)", tt)
		}
		return staticCandidate(ref, ref.Profile, types.TypePeerTransportHTTPPoll, targetPeerID, origin)

	case TransportByHash:
		// The by-hash branch reads from the **registry's** content store.
		// The version of this code that moved here carried a doc comment
		// calling that a hole — *"nothing in §3 says the registry holds
		// the entity a binding's hash names, so a by-hash transport can
		// dead-end at a registry that published the reference and not the
		// referent"* — and it was one of the arguments in our own
		// divergence packet. **The specification has since closed it**:
		// EXTENSION-REGISTRY §6a.3's publish obligation now covers *"every
		// `system/peer/transport/*` profile entity named by a published
		// binding's `transports`, fetchable by its content hash"*, on the
		// stated reasoning that a hash the consumer cannot resolve
		// *"reinstates exactly that gap one indirection later, satisfying
		// the rule in letter while delivering nothing."* So this failing
		// is a registry defect, and the message says so rather than
		// hedging about whether it was ever promised.
		ent, err := r.Blob(ctx, ref.Hash)
		if err != nil {
			return TransportCandidate{}, fmt.Errorf(
				"the binding names transport-profile %s and the registry does not serve it: %w", ref.Hash, err)
		}
		return r.candidateFromEntity(ref, ent, targetPeerID, origin)

	default:
		return TransportCandidate{}, fmt.Errorf("%s", ref.Note)
	}
}

// candidateFromEntity classes a decoded profile entity.
//
// The switch is on the **entity type**, never on the `transport_type`
// field: §6.5.1a D5 makes the type authoritative and demotes the field to
// self-description, and core-go's `*FromEntity` decoders fail closed when
// the two disagree. A profile family we do not speak is skipped by name
// rather than reported as malformed.
func (r *Registry) candidateFromEntity(ref TransportRef, ent entity.Entity, targetPeerID, origin string) (TransportCandidate, error) {
	switch ent.Type {
	case types.TypePeerTransportHTTPPoll:
		prof, err := types.HTTPPollProfileDataFromEntity(ent)
		if err != nil {
			return TransportCandidate{}, err
		}
		c, err := staticCandidate(ref, prof, ent.Type, targetPeerID, origin)
		if err != nil {
			return TransportCandidate{}, err
		}
		c.Hash = ref.Hash
		return c, nil

	case types.TypePeerTransportTCP:
		prof, err := types.TCPProfileDataFromEntity(ent)
		if err != nil {
			return TransportCandidate{}, err
		}
		return liveCandidate(ref, ent, prof.PeerID, prof.Endpoint.URL, prof.Priority, targetPeerID)

	case types.TypePeerTransportHTTP:
		prof, err := types.HTTPProfileDataFromEntity(ent)
		if err != nil {
			return TransportCandidate{}, err
		}
		return liveCandidate(ref, ent, prof.PeerID, prof.Endpoint.URL, prof.Priority, targetPeerID)

	case types.TypePeerTransportWebSocket:
		prof, err := types.WebSocketProfileDataFromEntity(ent)
		if err != nil {
			return TransportCandidate{}, err
		}
		return liveCandidate(ref, ent, prof.PeerID, prof.Endpoint.URL, prof.Priority, targetPeerID)

	default:
		return TransportCandidate{}, fmt.Errorf(
			"transport-profile %s is type %q, which is not a transport family this consumer speaks",
			ref.Hash, ent.Type)
	}
}

// staticCandidate turns an http-poll profile into a reachable layout.
//
// The peer-id cross-check and the origin-adoption rule are lifted from
// [Registry.OriginFor] unchanged — they were correct and are the reason
// this is a move rather than a rewrite.
func staticCandidate(ref TransportRef, prof types.HTTPPollProfileData, entType, targetPeerID, origin string) (TransportCandidate, error) {
	if err := checkProfilePeer(prof.PeerID, targetPeerID); err != nil {
		return TransportCandidate{}, err
	}
	if prof.PeerID == "" {
		// A bare endpoint carries no peer-id; the binding's target is the
		// only identity in play, and it is the registry's signed
		// assertion. Adopt it explicitly rather than letting Layout fail
		// on an empty one.
		prof.PeerID = targetPeerID
	}
	at := origin
	if abs, ok := absoluteOrigin(prof); ok {
		// A profile with absolute URL prefixes carries its own host and
		// does not need ours (§6.5.3 lets the three prefixes sit on
		// entirely separate origins).
		at = abs
	}
	layout, err := LayoutFromProfile(at, prof)
	if err != nil {
		return TransportCandidate{}, err
	}
	prio, declared := effectivePriority(prof.Priority)
	return TransportCandidate{
		Class:            ClassStatic,
		Type:             entType,
		Priority:         prio,
		PriorityDeclared: declared,
		Ref:              ref,
		Layout:           layout,
		Profile:          prof,
		PeerID:           prof.PeerID,
	}, nil
}

// liveCandidate turns a dialable profile into an address and nothing more.
//
// **It does not dial and does not check reachability**, which is not
// laziness: §6.5.1c consumer rule 6 says a profile in a signed set *"is
// not a promise that it currently answers"* and consumers fall through on
// failure. Deciding a candidate is dead belongs to whoever holds the
// socket, and this package holds none.
func liveCandidate(ref TransportRef, ent entity.Entity, profPeer, addr string, priority *uint64, targetPeerID string) (TransportCandidate, error) {
	if err := checkProfilePeer(profPeer, targetPeerID); err != nil {
		return TransportCandidate{}, err
	}
	if addr == "" {
		// An endpoint-less live profile is a real and deliberate shape —
		// §6.5.2d's *"reachable by negotiation"* advertisement — and it is
		// not something this consumer can act on, because we dial and do
		// not negotiate. Named as its own reason so it does not read as a
		// malformed profile.
		return TransportCandidate{}, fmt.Errorf(
			"transport-profile %s (%s) advertises no endpoint URL — reachability by negotiation "+
				"(§6.5.2d), which this consumer cannot drive", ref.Hash, ent.Type)
	}
	if profPeer == "" {
		profPeer = targetPeerID
	}
	prio, declared := effectivePriority(priority)
	return TransportCandidate{
		Class:            ClassLive,
		Type:             ent.Type,
		Priority:         prio,
		PriorityDeclared: declared,
		Ref:              ref,
		Hash:             ref.Hash,
		Address:          addr,
		PeerID:           profPeer,
	}, nil
}

// checkProfilePeer refuses a profile that names a peer other than the one
// the registry signed a binding to.
//
// The registry signed a binding to peer X carrying a profile that says
// peer Y. Refuse: the profile's peer-id is the key the target's content
// signature gets checked against, so following it would verify the wrong
// publisher perfectly.
func checkProfilePeer(profilePeer, targetPeerID string) error {
	if profilePeer != "" && profilePeer != targetPeerID {
		return fmt.Errorf("profile is for peer %s, binding targets %s", profilePeer, targetPeerID)
	}
	return nil
}

// effectivePriority applies §6.5.1a D1's default and reports whether it
// had to.
//
// **The `primary`-unset-means-zero half of the rule cannot be applied
// here and is deliberately not approximated.** That reserved id is a
// profile-id, i.e. a tree-path segment, and a binding carries profiles by
// content hash — see the file note. Guessing at it from any other field
// would be inventing a preference the publisher did not express, in the
// one direction (promoting to 0) that outranks everything they did.
func effectivePriority(p *uint64) (uint64, bool) {
	if p == nil {
		return DefaultTransportPriority, false
	}
	return *p, true
}
