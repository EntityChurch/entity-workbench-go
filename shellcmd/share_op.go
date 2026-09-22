package shellcmd

// Share / Accept as workspace OPERATIONS — the seam that turns
// discover → share → permission → mount → sync from five disconnected
// capabilities into one flow.
//
// # What was missing, stage by stage
//
// Every stage's machinery existed. None of them were joined, and two were
// reachable from nothing at all:
//
//	discover     SDK has DiscoverPeers / ReadDiscoveredCandidates and
//	             shellboot auto-announces on the listener's scheme. The
//	             Avalonia panel consumed it; the shell had no verb.
//	share        entitysdk/share.go implements APP-CONVENTION-SHARE in
//	             full — AuthorShare, ShareGrants, PrefixTarget, audience
//	             entries, the withdrawal notice. Called from share_test.go
//	             and nothing else, in either renderer.
//	permission   `system/capability/policy/{peer}` is the kernel's
//	             per-peer handshake policy table. ZERO uses in this repo.
//	             What we had instead was shellboot's OpenAccess flag — a
//	             process-wide wildcard its own doc calls development-only —
//	             which is the only reason cross-peer sync ever worked here.
//	mount        done.
//	sync         done (M2), but the receiver had to be told a peer-id and
//	             a root name out of band.
//
// # Three measured facts these operations are built around
//
// From `shellboot/policy_probe_test.go`, none of them obvious:
//
//  1. **A sync is MUTUAL authorization.** Both peers need a policy entry
//     naming the other. Grant one direction and you get an accepted
//     subscription and no files — which is indistinguishable from a
//     working share until someone looks in the folder.
//  2. **The grant is fixed at HANDSHAKE.** Writing a policy while already
//     connected changes nothing. Both Share and Accept therefore
//     re-establish the connection, and report whether they managed to.
//  3. **The peer-id-keyed policy path works**, so an operator who has a
//     peer-id from `peers` needs nothing else.
//
// # Why the policy table and not AuthorShare's minted tokens
//
// `entitysdk.ShareWithdrawalNotice` states that a
// `system/capability:request`-minted token is **not recallable**: it is
// returned inline with no tree write, so the granter never holds its hash
// and `revoke` cannot name it. Building `unshare` on that would make the
// verb unable to do the thing it is named after. A policy entry is a tree
// write we own, so removing it means the next handshake does not carry
// the grant — a revocation that is real, with a stated limit (it does not
// reach into a connection that already holds one).

import (
	"context"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/entitysdk"
	"entity-workbench-go/workbench"
)

// ShareRequest is the renderer-neutral input to a share.
type ShareRequest struct {
	// Root is the local mount root being offered.
	Root string
	// Peer is who it is offered to — an alias or a bare peer-id.
	Peer string
	// Title is an optional human label; defaults to the root name.
	Title string
	// NowMillis is the authoring timestamp. Passed in rather than read
	// from the clock so a caller can make the record deterministic.
	NowMillis uint64
}

// ShareOutcome reports what the share established.
type ShareOutcome struct {
	Root         string
	TargetPrefix string
	PeerID       string
	PeerAlias    string
	PolicyPath   string
	GrantSummary string
	Audience     []string

	// Reconnected reports whether the connection was re-established so
	// the new grant is actually in force.
	//
	// This field exists because the alternative is a share that reports
	// success and does nothing until an unrelated restart. If it is
	// false, ReconnectNote says why, and the operator has to act.
	Reconnected   bool
	ReconnectNote string
}

// Share offers a mounted folder to one peer: writes the handshake policy
// that authorizes them, publishes the offer record that tells them what
// is on offer, and re-establishes the connection so the grant is live.
func (ws *ShellWorkspace) Share(req ShareRequest) (ShareOutcome, error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return ShareOutcome{}, fmt.Errorf("workspace has no local peer")
	}
	local := ws.Local.Peer

	root := sanitizeRootName(strings.TrimSpace(req.Root))
	if root == "" {
		return ShareOutcome{}, fmt.Errorf("no root name given")
	}
	peerID, alias, err := ws.resolvePeerRef(req.Peer)
	if err != nil {
		return ShareOutcome{}, err
	}
	if peerID == local.PeerID() {
		return ShareOutcome{}, fmt.Errorf("cannot share with this peer itself (%s)", peerID)
	}

	// The folder must actually be mounted here. Sharing a root we do not
	// publish would write an offer nobody can consume and a grant that
	// authorizes reading a prefix with nothing under it.
	if !hasLocalRoot(local, root) {
		return ShareOutcome{}, fmt.Errorf(
			"no local mount named %q to share — run `mount <dir> <prefix>` first", root)
	}
	targetPrefix := ""
	if b, ok := workbench.LoadMountBinding(local.Store(), root); ok {
		targetPrefix = b.TargetPrefix
	}

	// The offer's audience is cumulative across calls: sharing the same
	// folder with a second peer must not un-share it from the first.
	audience := []string{peerID}
	if existing, ok := findOffer(local.Store(), root); ok {
		audience = mergeAudience(existing.Audience, peerID)
	}

	if err := workbench.SaveAccessPolicy(local.Store(), peerID,
		workbench.SyncSenderGrants(),
		"share: "+root); err != nil {
		return ShareOutcome{}, err
	}

	title := req.Title
	if title == "" {
		title = root
	}
	if err := workbench.SaveShareOffer(local.Store(), workbench.ShareOffer{
		Root:            root,
		Title:           title,
		TargetPrefix:    targetPrefix,
		Audience:        audience,
		CreatedAtMillis: req.NowMillis,
	}); err != nil {
		// Unwind the grant: an authorization with no offer beside it is
		// authority nobody asked for and nothing records the reason for.
		workbench.RemoveAccessPolicy(local.Store(), peerID)
		return ShareOutcome{}, err
	}

	reconnected, note := ws.refreshGrantConnection(peerID)

	return ShareOutcome{
		Root:          root,
		TargetPrefix:  targetPrefix,
		PeerID:        peerID,
		PeerAlias:     alias,
		PolicyPath:    workbench.AccessPolicyPrefix + peerID,
		GrantSummary:  workbench.SummarizeGrants(workbench.SyncSenderGrants()),
		Audience:      audience,
		Reconnected:   reconnected,
		ReconnectNote: note,
	}, nil
}

// UnshareOutcome reports what withdrawal actually did — and what it did
// not, which is the part a surface must not omit.
type UnshareOutcome struct {
	Root          string
	PeerID        string
	PolicyRemoved bool
	OfferRemoved  bool
	// StillOffered is the remaining audience for this root, so a surface
	// can say "withdrawn from A, still shared with B" rather than
	// implying the folder is now private.
	StillOffered []string
	// Caveat is the sentence a surface MUST print. Withdrawal stops the
	// NEXT handshake carrying the grant; it does not reach into a live
	// connection that already holds one.
	Caveat string
}

// Unshare withdraws one peer's access to one root.
func (ws *ShellWorkspace) Unshare(root, peer string) (UnshareOutcome, error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return UnshareOutcome{}, fmt.Errorf("workspace has no local peer")
	}
	local := ws.Local.Peer

	root = sanitizeRootName(strings.TrimSpace(root))
	peerID, _, err := ws.resolvePeerRef(peer)
	if err != nil {
		return UnshareOutcome{}, err
	}
	if root == "" {
		return UnshareOutcome{}, fmt.Errorf("unshare needs a root name")
	}

	out := UnshareOutcome{
		Root:   root,
		PeerID: peerID,
		Caveat: "the grant is removed from the policy table, so the NEXT handshake " +
			"will not carry it; a connection that already holds it keeps it until " +
			"it is re-established.",
	}

	// Narrow the offer's audience rather than deleting the record, unless
	// this was the last member.
	if existing, ok := findOffer(local.Store(), root); ok {
		remaining := make([]string, 0, len(existing.Audience))
		for _, a := range existing.Audience {
			if a != peerID {
				remaining = append(remaining, a)
			}
		}
		out.StillOffered = remaining
		if len(remaining) == 0 {
			out.OfferRemoved = workbench.RemoveShareOffer(local.Store(), root)
		} else {
			existing.Audience = remaining
			if err := workbench.SaveShareOffer(local.Store(), existing); err != nil {
				return out, err
			}
		}
	}

	// Only drop the peer's policy if they are no longer in ANY of our
	// offers. The policy is per-peer and a share is per-root, so revoking
	// on the first unshare would silently cut a folder the operator did
	// not mention.
	if !peerIsStillOffered(local.Store(), peerID) {
		out.PolicyRemoved = workbench.RemoveAccessPolicy(local.Store(), peerID)
	}
	return out, nil
}

// AcceptOutcome reports what accepting an offer established.
type AcceptOutcome struct {
	PeerID        string
	PeerAlias     string
	Root          string
	PolicyPath    string
	GrantSummary  string
	Sync          SyncOutcome
	Reconnected   bool
	ReconnectNote string

	// PublisherMustDial is the step the RECEIVER cannot perform and the
	// flow does not work without.
	//
	// Over a dial-by-address connection, authorization is one-directional
	// by design. `Connection.sendReciprocalGrant` is gated on
	// `EstablishedViaRendezvousKey()`, and the kernel states the reason:
	// *"a dial-by-address is asymmetric — one party requested service —
	// and §6.6's one-directional mint stands alone there."* The dialer
	// gains authority to originate to the acceptor; the acceptor gains
	// nothing.
	//
	// So accepting gets us authority to subscribe and fetch, and gives
	// the publisher nothing — and DELIVERY runs publisher→us. They must
	// dial us once, after this policy exists, or their notifications are
	// refused 403 and the folder stays empty with no error on our side.
	//
	// Carried as a field rather than left to prose because it is the one
	// step in the whole flow that neither verb can complete on its own.
	PublisherMustDial string
}

// Accept is the receiving side's one step: write the delivery policy that
// lets the publisher reach our blob-resolve handler, re-establish the
// connection so it is in force, then sync.
//
// The three actions are one verb because they are not independently
// useful. A policy with no sync grants a stranger a handler they will
// never call; a sync with no policy is accepted and delivers nothing.
func (ws *ShellWorkspace) Accept(peer, root string) (AcceptOutcome, error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return AcceptOutcome{}, fmt.Errorf("workspace has no local peer")
	}
	local := ws.Local.Peer

	peerID, alias, err := ws.resolvePeerRef(peer)
	if err != nil {
		return AcceptOutcome{}, err
	}
	root = sanitizeRootName(strings.TrimSpace(root))
	if root == "" {
		return AcceptOutcome{}, fmt.Errorf("accept needs a root name (try `offers %s`)", peer)
	}

	if err := workbench.SaveAccessPolicy(local.Store(), peerID,
		workbench.SyncReceiverGrants(),
		"accept: deliveries for "+root); err != nil {
		return AcceptOutcome{}, err
	}

	reconnected, note := ws.refreshGrantConnection(peerID)

	syncOut, err := ws.Sync(SyncRequest{Remote: peerID, Root: root})
	if err != nil {
		// Leave the policy in place on failure and say so in the error:
		// the usual cause is a missing local mount, the operator is about
		// to fix that and retry, and removing the grant would make the
		// retry fail for a second, different reason.
		return AcceptOutcome{}, fmt.Errorf(
			"%w\n(the delivery grant for %s was written and is left in place; "+
				"re-run accept once the cause above is fixed)", err, peerID)
	}

	return AcceptOutcome{
		PeerID:        peerID,
		PeerAlias:     alias,
		Root:          root,
		PolicyPath:    workbench.AccessPolicyPrefix + peerID,
		GrantSummary:  workbench.SummarizeGrants(workbench.SyncReceiverGrants()),
		Sync:          syncOut,
		Reconnected:   reconnected,
		ReconnectNote: note,
		PublisherMustDial: fmt.Sprintf(
			"connect %s <this-peer's host:port>   (run on %s)", local.PeerID(), peerID),
	}, nil
}

// DialableAddressFor reports an address that can actually be dialled to
// reach a peer, and where it came from ("connection-table" or
// "discovery"), or ("", "") when nothing is known.
//
// Exported so a surface can show the reciprocal-dial step as a state it
// can act on — with an address already resolved, or with a field to type
// one into — rather than discovering at click time that there was never
// anything to dial. The strength of the claim differs by source, which
// is why the source is returned rather than just the string.
func (ws *ShellWorkspace) DialableAddressFor(peerID string) (string, string) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return "", ""
	}
	for _, pc := range ws.Conns {
		if pc != nil && pc.PeerID == peerID && pc.Address != "" {
			return pc.Address, "connection-table"
		}
	}
	if addr := ws.dialableAddressFor(peerID); addr != "" {
		return addr, "discovery"
	}
	return "", ""
}

// CompleteShareOutcome reports the reciprocal dial — the last step of a
// share, and the only one that cannot be performed by the peer who needs
// it done.
type CompleteShareOutcome struct {
	PeerID    string
	PeerAlias string
	// Address is what was actually dialled, so a surface can show the
	// operator which of the several possible sources won.
	Address string
	// AddressSource is "given", "connection-table" or "discovery".
	// Distinguished because they are different strengths of claim: an
	// address we dialled before is known-good, an mDNS announcement is
	// what the peer says about itself, and a typed one is the operator's
	// assertion.
	AddressSource string
	Connected     bool

	// NeedsAddress is the one failure a surface must handle differently
	// from an error: there is nothing wrong, we simply do not know where
	// this peer listens. The remedy is an address field, not a retry.
	NeedsAddress bool
	Note         string
}

// CompleteShare performs the reciprocal dial from the SHARING side.
//
// # Why this operation exists at all
//
// Over a dial-by-address connection the kernel's reciprocal grant is
// gated on `EstablishedViaRendezvousKey()` — *"a dial-by-address is
// asymmetric — one party requested service"*. So when the receiver dials
// us to subscribe, they gain the right to originate to us and we gain
// nothing; and DELIVERY runs publisher→receiver. Until we dial them, our
// notifications are refused and their folder stays empty, with no error
// on either side.
//
// # Why the receiver cannot do it for us, and why we cannot do it silently
//
// The addresses are asymmetric too, and that asymmetry is the whole
// difficulty. The receiver always has ours (they dialled it). We
// frequently have nothing for them: `ConnectedPeers()` reports the
// OBSERVED remote address, which for their inbound connection is an
// ephemeral source port that looks dialable and is not (see
// dialableAddressFor). And we cannot read their self-published transport
// profile, because `SyncReceiverGrants` deliberately grants us exactly
// one thing — `workbench/blob-resolve:receive` — and reading their tree
// is not it. Widening that grant to make this automatic is a real option
// and a separate decision; it trades a scoped read on the receiver for
// removing this step.
//
// So the honest shape is: resolve an address if we can, and if we cannot,
// say so as a distinct outcome (`NeedsAddress`) rather than as a failure,
// so a surface can ask for one instead of reporting that something broke.
//
// A successful dial is REMEMBERED in the connection table, which is what
// makes this a one-time step per peer rather than a recurring chore.
func (ws *ShellWorkspace) CompleteShare(peer, address string) (CompleteShareOutcome, error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return CompleteShareOutcome{}, fmt.Errorf("workspace has no local peer")
	}
	local := ws.Local.Peer

	peerID, alias, err := ws.resolvePeerRef(peer)
	if err != nil {
		return CompleteShareOutcome{}, err
	}
	if peerID == local.PeerID() {
		return CompleteShareOutcome{}, fmt.Errorf("cannot dial this peer itself (%s)", peerID)
	}

	out := CompleteShareOutcome{PeerID: peerID, PeerAlias: alias}

	addr := strings.TrimSpace(address)
	out.AddressSource = "given"
	if addr == "" {
		addr = ws.dialableAddressFor(peerID)
		// dialableAddressFor prefers the connection table over discovery,
		// so report which one answered rather than guessing.
		out.AddressSource = "discovery"
		for _, pc := range ws.Conns {
			if pc != nil && pc.PeerID == peerID && pc.Address == addr && addr != "" {
				out.AddressSource = "connection-table"
				break
			}
		}
	}
	if addr == "" {
		out.NeedsAddress = true
		out.AddressSource = ""
		out.Note = "no address is known for this peer — we have never dialled them and they " +
			"are not announcing on the local network. Their own app shows the address it " +
			"listens on."
		return out, nil
	}
	out.Address = addr

	// Disconnect first so the next handshake re-assembles the grant set.
	// This is the whole point of the operation: the policy the receiver
	// wrote is inert on a connection established before it existed.
	//
	// Ordering matters and was a live bug once: an earlier version of the
	// neighbouring reconnect tore the connection down BEFORE checking it
	// had somewhere to redial, which turned a share into an outage. The
	// address is resolved above, so by here there is something to dial.
	local.Disconnect(peerID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := local.Connect(ctx, addr)
	if err != nil {
		out.Note = fmt.Sprintf("dial %s failed: %v — the share is written and applies at "+
			"the next successful connection either side makes", addr, err)
		return out, nil
	}

	// Verify we reached the peer we meant to. An address can be stale or
	// simply wrong when it was typed, and connecting to the wrong peer
	// and reporting the share complete is worse than failing: the folder
	// stays empty and the surface says it should not be.
	if state := conn.ConnState(); state == nil || string(state.RemotePeerID) != peerID {
		got := "(no peer-id)"
		if state != nil && state.RemotePeerID != "" {
			got = string(state.RemotePeerID)
		}
		out.Note = fmt.Sprintf("dialled %s but reached %s, not this peer — check the address", addr, got)
		return out, nil
	}

	out.Connected = true
	ws.rememberAddress(peerID, addr)
	return out, nil
}

// rememberAddress records a known-good dial address for a peer so a
// later CompleteShare (or any reconnect) does not have to ask again.
// Updates an existing alias binding in place; otherwise binds a new one
// derived from the peer-id, and gives up quietly if that name is taken
// rather than inventing a second alias for a peer that already has one.
func (ws *ShellWorkspace) rememberAddress(peerID, addr string) {
	for _, pc := range ws.Conns {
		if pc != nil && pc.PeerID == peerID {
			pc.Address = addr
			return
		}
	}
	short := peerID
	if len(short) > 8 {
		short = short[:8]
	}
	alias, err := NormalizeAlias(strings.ToLower(short))
	if err != nil || alias == "" || IsReservedAlias(alias) {
		return
	}
	if _, taken := ws.Conns[alias]; taken {
		return
	}
	ws.addConn(&PeerConn{
		Alias:   alias,
		Address: addr,
		PeerID:  peerID,
		Peer:    ws.Local.Peer,
	})
}

// Offers reads what a remote peer is publishing to us.
//
// A DISPATCHED read of a peer-qualified path, which is a remote read
// (AP11): `List("/{them}/app/share/records/")` routes to that peer and
// returns their tree, which is the point — we want their offers, not our
// mirror of them.
func (ws *ShellWorkspace) Offers(peer string) (offers []workbench.ShareOffer, err error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return nil, fmt.Errorf("workspace has no local peer")
	}
	local := ws.Local.Peer
	peerID, _, err := ws.resolvePeerRef(peer)
	if err != nil {
		return nil, err
	}

	// Refresh OUR outbound connection first.
	//
	// The grant we hold on them was assembled during OUR handshake, and
	// reading their offers needs the `system/tree:get` their `share`
	// wrote into their policy table afterwards. Their own reconnect does
	// not help: we dispatch over the connection WE opened, and that one
	// still carries whatever we were granted when we opened it.
	//
	// The transferable rule, measured the hard way: **the peer that
	// DISPATCHES is the peer that must re-establish its connection.** A
	// reconnect performed by the grantER refreshes the reciprocal
	// direction, not the pooled outbound connection the grantEE actually
	// uses. Without this, `offers` returns 403 for a share that was
	// correctly written moments earlier.
	ws.refreshGrantConnection(peerID)

	prefix := "/" + peerID + "/" + workbench.ShareOfferPrefix
	entries, lerr := local.List(prefix)
	if lerr != nil {
		return nil, fmt.Errorf("read %s's offers: %w", peerID, lerr)
	}
	me := local.PeerID()
	for _, e := range entries {
		ent, ok, gerr := local.Get(e.Path)
		if gerr != nil || !ok {
			continue
		}
		offer, ok := workbench.DecodeRemoteShareOffer(e.Path, ent)
		if !ok {
			continue
		}
		// Only offers that name us. An offer to someone else is not ours
		// to see listed as available — and since the record is a label
		// rather than an authority, showing it would promise nothing the
		// grant backs.
		if offer.OfferedTo(me) {
			offers = append(offers, offer)
		}
	}
	sort.Slice(offers, func(i, j int) bool { return offers[i].Root < offers[j].Root })
	return offers, nil
}

// DiscoveredPeer is one candidate as a surface should show it.
type DiscoveredPeer struct {
	PeerID    string
	Address   string
	Alias     string
	Connected bool
	// Source is how we know about this peer — "discovery" (announced on
	// the local network) or "connected" (we hold a connection). Stated
	// because they are different claims: one is an advertisement, the
	// other is a fact.
	Source string
}

// Peers is the discover stage: everything we can currently see, from
// mDNS announcements and from the connection pool, merged.
func (ws *ShellWorkspace) Peers() ([]DiscoveredPeer, error) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return nil, fmt.Errorf("workspace has no local peer")
	}
	local := ws.Local.Peer

	byID := map[string]*DiscoveredPeer{}

	for _, c := range local.ConnectedPeers() {
		if c.PeerID == "" {
			continue
		}
		byID[c.PeerID] = &DiscoveredPeer{
			PeerID:    c.PeerID,
			Address:   c.Address,
			Alias:     ws.AliasFor(c.PeerID),
			Connected: true,
			Source:    "connected",
		}
	}

	if local.DiscoveryEnabled() {
		for _, cand := range local.ReadDiscoveredCandidates() {
			id := string(cand.PeerID)
			if id == "" || id == local.PeerID() {
				continue
			}
			if existing, ok := byID[id]; ok {
				if existing.Address == "" {
					existing.Address = candidateAddress(cand)
				}
				continue
			}
			byID[id] = &DiscoveredPeer{
				PeerID:  id,
				Address: candidateAddress(cand),
				Alias:   ws.AliasFor(id),
				Source:  "discovery",
			}
		}
	}

	out := make([]DiscoveredPeer, 0, len(byID))
	for _, p := range byID {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Connected != out[j].Connected {
			return out[i].Connected
		}
		return out[i].PeerID < out[j].PeerID
	})
	return out, nil
}

// --- internals ------------------------------------------------------

// resolvePeerRef turns an alias or peer-id into (peerID, alias).
func (ws *ShellWorkspace) resolvePeerRef(ref string) (peerID, alias string, err error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", "", fmt.Errorf("no peer given")
	}
	if pc, ok := ws.Conns[ref]; ok && pc != nil {
		return pc.PeerID, pc.Alias, nil
	}
	return ref, ws.AliasFor(ref), nil
}

// refreshGrantConnection re-establishes the connection to a peer so a
// freshly written policy is actually in force.
//
// The grant a peer holds is assembled during the handshake, so a policy
// written afterwards has no effect until the next one. Measured:
// `shellboot/policy_probe_test.go::TestProbe_PolicyWrittenAfterConnect`
// — sync is refused 403 with both policies present and written late.
//
// Returns whether it managed it and why not. It does NOT return an error:
// failing to reconnect leaves the policy correctly written and the grant
// pending, which is a state worth reporting rather than one worth
// unwinding a share for.
func (ws *ShellWorkspace) refreshGrantConnection(peerID string) (bool, string) {
	local := ws.Local.Peer

	addr := ws.dialableAddressFor(peerID)
	if addr == "" {
		// **Deliberately does not disconnect.** An earlier version tore
		// the connection down first and then discovered it had nothing to
		// dial, which left the peer disconnected — strictly worse than
		// doing nothing, and it turned a share into an outage.
		return false, "no dialable address is known for this peer, so the connection " +
			"was left alone — the grant applies to the next connection either side " +
			"makes (`connect <alias> <host:port>` forces one now)"
	}

	local.Disconnect(peerID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := local.Connect(ctx, addr); err != nil {
		return false, fmt.Sprintf(
			"reconnect to %s failed (%v) — the policy is written and the grant "+
				"applies at the next successful connection", addr, err)
	}
	return true, ""
}

// dialableAddressFor returns an address that can actually be dialled to
// reach a peer, or "".
//
// **`AppPeer.ConnectedPeers()` is not a source for this**, which is the
// trap: its Address is the connection's OBSERVED remote address, and for
// an INBOUND connection that is the dialer's ephemeral source port. It
// looks exactly like a real address — `127.0.0.1:50026` — and dialling it
// is refused. Measured while writing the flow test, where the reconnect
// failed against a port that had never been listening.
//
// The two honest sources, in preference order:
//
//  1. the address the workspace recorded when WE dialled them, via the
//     `connect` verb. Known-dialable because it worked once.
//  2. the peer's own mDNS announcement, which advertises the address it
//     LISTENS on rather than one it happened to send from.
func (ws *ShellWorkspace) dialableAddressFor(peerID string) string {
	for _, pc := range ws.Conns {
		if pc != nil && pc.PeerID == peerID && pc.Address != "" {
			return pc.Address
		}
	}
	local := ws.Local.Peer
	if local.DiscoveryEnabled() {
		for _, cand := range local.ReadDiscoveredCandidates() {
			if cand.PeerID != peerID {
				continue
			}
			if addr := entitysdk.DialAddressForCandidate(cand); addr != "" {
				return addr
			}
		}
	}
	return ""
}

func findOffer(st *workbench.Store, root string) (workbench.ShareOffer, bool) {
	offers, _ := workbench.LoadShareOffers(st)
	for _, o := range offers {
		if o.Root == root {
			return o, true
		}
	}
	return workbench.ShareOffer{}, false
}

func peerIsStillOffered(st *workbench.Store, peerID string) bool {
	offers, _ := workbench.LoadShareOffers(st)
	for _, o := range offers {
		if o.OfferedTo(peerID) {
			return true
		}
	}
	return false
}

func mergeAudience(existing []string, add string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(existing)+1)
	for _, a := range append(append([]string(nil), existing...), add) {
		if a == "" || seen[a] {
			continue
		}
		seen[a] = true
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

func candidateAddress(c types.CandidateData) string {
	// The dial-address choice lives in the SDK, shared with the Avalonia
	// bridge — it is a substrate judgement about mDNS announcements, not
	// something either renderer owns.
	return entitysdk.DialAddressForCandidate(c)
}

// SharesOffered lists this peer's own outgoing offers.
func (ws *ShellWorkspace) SharesOffered() ([]workbench.ShareOffer, []string) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return nil, nil
	}
	return workbench.LoadShareOffers(ws.Local.Peer.Store())
}

// AccessPolicies lists every peer this peer has authorized.
func (ws *ShellWorkspace) AccessPolicies() ([]workbench.AccessPolicy, []string) {
	if ws == nil || ws.Local == nil || ws.Local.Peer == nil {
		return nil, nil
	}
	// The peer's own identity hash in hex is the key the kernel's §6.9a
	// seed entry uses, and passing it is what lets a surface say "this
	// row is you" instead of showing a wildcard grant to an unexplained
	// hex string.
	return workbench.ListAccessPolicies(ws.Local.Peer.Store(),
		hex.EncodeToString(ws.Local.Peer.IdentityHash().Bytes()))
}
